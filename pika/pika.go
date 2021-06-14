// Package pika implements a novel BFT SMR protocol, an efficient VID protocol, and a Byzantine Agreement protocol.
package pika

import (
	"encoding/binary"
	"sort"
	"strings"
	"sync"
	"time"
	"sync/atomic"
)

var NagleSize int
var NagleDelay int

var FirstUnconfirmed int64

// PikaPayload is the interface that the application data in Pika should support.
type PikaPayload interface {
	LogPropose() string // returns a string upon proposing the object
	LogConfirm(int) string // returns a string upon confirming the object
}

// PikaBlock is a block in the Pika protocol. It contains a list of references to blocks proposed by
// each node plus the application data.
type PikaBlock struct {
	References []int // the list of epoch numbers until (but not including) which the proposer of the block
	// thinks each node has finished VID
	Payload PikaPayload // the application data
}

// Size returns the size of the PikaBlock in the emulator. It panics if the Payload is not a Sizer.
func (pb *PikaBlock) Size() int {
	ps, isps := pb.Payload.(Sizer)
	if !isps {
		panic("calling Size on a pikaBlock with a payload that is not Sizer")
	}
	return ps.Size()
}

// valid values of PikaEpoch.vState
const (
	inMemory     = iota
	onDisk       = iota
	pagedInClean = iota
	pagedInDirty = iota
)

const (
	BlockedByLowPriorityMessage  Event = FirstCustomEvent << iota // the protocol cannot proceed without receiving more low-priority messages
	BlockedByHighPriorityMessage                                  // the protocol cannot proceed without receiving more high-priority messages
	Dispersed
)

// PikaEpoch is an epoch of Pika. It contains N VIDs and N BAs. Upon termination, it is guaranteed that all BAs
// have terminated with at least N-F BAs outputting true and the corresponding VIDs have terminated.
type PikaEpoch struct {
	payload   *PikaBlock // the block we are trying to propose in this epoch
	v         []*VID     // VID instances
	vStorage  []int      // the location where the VID is stored
	vRefCount []int      // the reference count of VID - how many times the VID is accessed minus how many times
	// it is freed
	b         []*BA  // BA instances
	bInit     []bool // whether we have initiated the BA instance
	requested []bool // whether we have requested the block and released its chunks
	released  []bool // whether we have requested the block and released its chunks
	retrieved []bool // whether we have retrieved the block; this field is managed by PayloadRetrieved and should
	outputted []bool // whether we have outputeed the block in the upstream protocol
	// not be accessed otherwise
	confirmUntil []int // the epoch numbers until (but not including) which we can confirm the blocks from each
	// node; this field is managed by ConfirmUntil and should not be read or written otherwise
	termed bool // if we have terminated; termination happens when all BAs are terminated, and we have decoded
	// all blocks whose BAs output true
	hptermed bool        // if we have high-priority terminated; HP termination happens when all BAs are terminated
	codec    ErasureCode // the erasure code to be used by the VIDs
	requestStarted bool
	ProtocolParams
}

func (p *PikaEpoch) StartRequesting() []Message {
	var msgs []Message
	p.requestStarted = true
	// first, request for those that have terminated
	for i := 0; i < p.N; i++ {
		// release mean that we have see BA or VID terminate
		if p.released[i] {
			msgs = append(msgs, p.Request(i)...)
		}
	}
	return msgs
}

// TODO: we should let the protocols return whether the internal state is modified. For example, when modifying the
// state, the protocol calls defer func() {Event |= Modified}() And we should decide dirty/clean upon freeing.

// MarkOutput marks the block is outputted.
func (p *PikaEpoch) MarkOutput(idx int) {
	p.outputted[idx] = true
}

// Outputted returns if the block is confirmed.
func (p *PikaEpoch) Outputted(idx int) bool {
	return p.outputted[idx]
}

// ReadVID returns the pointer to the VID instance located by the index, and increases the reference counter if
// needed. If the VID is on the disk, it pages it in.
func (p *PikaEpoch) ReadVID(idx int) *VID {
	// if we don't have access to the DB, do nothing
	if p.DBPath == nil {
		return p.v[idx]
	}
	// if the object was originally in memory, simply return the in-memory value
	// we should still increase the counter though, because the user will defer FreeVID anyway
	if p.vStorage[idx] != onDisk {
		p.vRefCount[idx] += 1
		return p.v[idx]
	} else {
		// if the object was on the disk, we should page it in
		p.pageInVID(idx)
		// then we should mark it as pagedInClean
		p.vRefCount[idx] += 1
		p.vStorage[idx] = pagedInClean
		return p.v[idx]
	}
}

// WriteVID returns the pointer to the VID instance located by the index and mark it edited. It increases the
// reference counter if needed, and pages the VID into the memory if it was not in the main memory.
func (p *PikaEpoch) WriteVID(idx int) *VID {
	// if we don't have access to the DB, do nothing
	if p.DBPath == nil {
		return p.v[idx]
	}
	// if the object was originally in memory, return the in-memory value
	if p.vStorage[idx] != onDisk {
		p.vRefCount[idx] += 1
		// if it was paged in, we need to mark it dirty
		if p.vStorage[idx] == pagedInClean {
			p.vStorage[idx] = pagedInDirty
		}
		return p.v[idx]
	} else {
		// if the object was on the disk, we should page it in
		p.pageInVID(idx)
		// then we should mark it as pagedInDirty
		p.vRefCount[idx] += 1
		p.vStorage[idx] = pagedInDirty
		return p.v[idx]
	}
}

// FreeVID decreases the reference counter of the VID specified by the index. If the referece counter reaches
// zero, it pages the VID out if the VID was paged in and modified.
func (p *PikaEpoch) FreeVID(idx int) {
	// if we don't have access to the DB, do nothing
	if p.DBPath == nil {
		return
	}
	// decrease the reference count
	p.vRefCount[idx] -= 1
	// do something when the ref counter reaches zero
	if p.vRefCount[idx] == 0 {
		switch p.vStorage[idx] {
		case inMemory:
			return
		case pagedInClean:
			p.v[idx] = nil
			p.vStorage[idx] = onDisk
		case pagedInDirty:
			p.pageOutVID(idx)
			p.vStorage[idx] = onDisk
		case onDisk:
			// this should not happen
			panic("freeing on-disk VID")
		}
	}
}

// StashVID forces to page out the VID.
func (p *PikaEpoch) StashVID(idx int) {
	// if we don't have access to the DB, do nothing
	if p.DBPath == nil {
		return
	}
	// if no one is referencing the VID, simply page it out
	if p.vRefCount[idx] == 0 {
		p.pageOutVID(idx)
		p.vStorage[idx] = onDisk
	} else {
		// otherwise, force it to be paged out by setting it as dirty
		p.vStorage[idx] = pagedInDirty
	}
}

// vidKey returns the key to store and retrieve the VID instance.
func (p *PikaEpoch) vidKey(idx int) []byte {
	kbuf := make([]byte, 8)
	binary.BigEndian.PutUint64(kbuf, uint64(idx))
	key := make([]byte, len(p.DBPrefix))
	copy(key, p.DBPrefix)
	key = append(key, kbuf...)
	return key
}

// pageOutVID stores the VID instance onto the disk and removes its in-memory presence. It panics
// if the VID is not in memory.
func (p *PikaEpoch) pageOutVID(idx int) {
	if p.v[idx] == nil {
		panic("vid not in memory")
	} else {
		//p.Printf("Paging out VID%v\n", idx)
		buf, err := p.v[idx].MarshalBinary()
		if err != nil {
			panic(err)
		}
		err = p.DBPath.Put(p.vidKey(idx), buf)
		if err != nil {
			panic(err)
		}
		p.v[idx] = nil
		return
	}
}

// pageInVID gets the VID instance stored on the disk and deserializes it into the memory.
func (p *PikaEpoch) pageInVID(idx int) {
	res := &VID{}
	// populate the codec and the protocolparams
	res.codec = p.codec
	res.ProtocolParams = p.ProtocolParams.WithPrefix("VID", idx)
	res.ProtocolParams.Logger = nil
	//p.Printf("Paging in VID%v\n", idx)
	buf, err := p.DBPath.Get(p.vidKey(idx))
	if err != nil {
		panic(err)
	}
	err = res.UnmarshalBinary(buf)
	if err != nil {
		panic(err)
	}
	p.v[idx] = res
}

// PayloadDispersed checks if the VID instance has finished dispersal.
func (p *PikaEpoch) PayloadDispersed(idx int) bool {
	vp := p.ReadVID(idx)
	defer p.FreeVID(idx)
	return vp.PayloadDispersed()
}

// PayloadRetrieved checks if the VID instance has decoded the dispersed file.
func (p *PikaEpoch) PayloadRetrieved(idx int) bool {
	if p.retrieved[idx] {
		return true
	} else {
		_, there := p.Payload(idx)
		if there {
			p.retrieved[idx] = true
			return true
		} else {
			return false
		}
	}
}

// Payload return the block and true if it is decoded, or nil and false if not.
func (p *PikaEpoch) Payload(idx int) (*PikaBlock, bool) {
	vp := p.ReadVID(idx)
	defer p.FreeVID(idx)
	ptr, there := vp.Payload()
	if !there {
		return nil, false
	}
	res, isblock := ptr.(*PikaBlock)
	// TODO: this is gross. we are not sure if the returned type is PikaBlock, because
	// it is possible that the VID is inited, and we are the node to disperse. In such
	// a case, the Payload will be EmptyPayload before we actually disperse. And if this
	// method is called, we have to return nil, false because we have not got our own
	// block.
	if !isblock {
		return nil, false
	} else {
		return res, true
	}
}

// Terminated returns whether the PikaEpoch is terminated.
func (p *PikaEpoch) Terminated() bool {
	return p.termed
}

// PikaEpochMessage is a message sent and handled by a PikaEpoch.
type PikaEpochMessage struct {
	Idx    int         // index of the BA/VID instance this message is for
	BAMsg  *BAMessage  // pointer to the BA message, or nil if it is not a BA message
	VIDMsg *VIDMessage // pointer to the VID message, or nil if it is not a VID message
}

// Dest returns the destination of the message.
func (m PikaEpochMessage) Dest() int {
	if m.BAMsg != nil {
		return m.BAMsg.Dest()
	} else if m.VIDMsg != nil {
		return m.VIDMsg.Dest()
	} else {
		panic("both BA and VID are nil")
	}
}

// From returns the source of the message.
func (m PikaEpochMessage) From() int {
	if m.BAMsg != nil {
		return m.BAMsg.From()
	} else if m.VIDMsg != nil {
		return m.VIDMsg.From()
	} else {
		panic("both BA and VID are nil")
	}
}

// Priority returns the priority of the message.
func (m PikaEpochMessage) Priority() Priority {
	if m.BAMsg != nil {
		return m.BAMsg.Priority()
	} else if m.VIDMsg != nil {
		return m.VIDMsg.Priority()
	} else {
		panic("both BA and VID are nil")
	}
}

// Size returns the size of the message in the emulator.
func (m PikaEpochMessage) Size() int {
	if m.BAMsg != nil {
		return m.BAMsg.Size() + 8
	} else if m.VIDMsg != nil {
		return m.VIDMsg.Size() + 8
	} else {
		panic("both BA and VID are nil")
	}
}

// numBATrue returns the number of BA instances that output true.
func (p *PikaEpoch) numBATrue() int {
	n := 0
	for i := 0; i < p.N; i++ {
		if p.b[i].Terminated() && p.b[i].Output() {
			n += 1
		}
	}
	return n
}

// TODO: we should limit the depth of the retrieval pipeline

// RequestAndRelease requests the payload of the given block, and releases the chunks that we have for that block.
// It returns the messages to be sent.
func (p *PikaEpoch) Release(idx int) []Message {
	var msgs []Message
	if p.released[idx] {
		return msgs
	}
	p.released[idx] = true
	vp := p.WriteVID(idx)
	defer p.FreeVID(idx)
	outMsgs := vp.ReleaseChunk()
	msgs = append(msgs, p.forwardMessages(outMsgs, idx)...)
	return msgs
}

func (p *PikaEpoch) Request(idx int) []Message {
	var msgs []Message
	if p.requested[idx] {
		return msgs
	}
	p.requested[idx] = true
	vp := p.WriteVID(idx)
	defer p.FreeVID(idx)
	reqMsgs := vp.RequestPayload()
	msgs = append(msgs, p.forwardMessages(reqMsgs, idx)...)
	return msgs
}

// ConfirmUntil returns the epochs until (but not including) which are confirmed by the reference links of this epoch.
// It panics if the block is not decoded.
func (p *PikaEpoch) ConfirmUntil() []int {
	if p.confirmUntil != nil {
		return p.confirmUntil
	} else {
		cu := make([]int, p.N)
		// calculate confirmed until levels
		for i := 0; i < p.N; i++ {
			// gather the confirmed levels
			levels := make([]int, p.N)
			for j := 0; j < p.N; j++ {
				if p.b[j].Output() {
					vp := p.ReadVID(j)
					defer p.FreeVID(j)
					pld, there := vp.Payload()
					if !there {
						panic("trying to calculate confirmed levels when the payload is not retrieved")
					}
					levels[j] = pld.(*PikaBlock).References[i]
				}
			}
			// sort and get the F+1-largest, or the [N-F-1]
			sort.Ints(levels)
			cu[i] = levels[p.N-p.F-1]
		}
		p.confirmUntil = cu
		return cu
	}
}

// inputBA provides input to the BA and initializes the BA. It is a nop if the BA has been initialized.
func (p *PikaEpoch) inputBA(idx int, input bool) []Message {
	var msgs []Message
	// we should input only once
	if !p.bInit[idx] {
		p.bInit[idx] = true
		p.b[idx].SetInput(input)
		messages, _ := p.b[idx].Init()
		msgs = append(msgs, p.forwardMessages(messages, idx)...)
	}
	return msgs
}

// forwardMessages wraps the messages in the givne slice into BAMessage, and returns the results.
func (p *PikaEpoch) forwardMessages(m []Message, idx int) []Message {
	var msgs []Message
	for _, v := range m {
		msg := &PikaEpochMessage{}
		msg.Idx = idx
		switch vm := v.(type) {
		case *BAMessage:
			msg.BAMsg = vm
		case *VIDMessage:
			msg.VIDMsg = vm
		default:
			panic("forwarding a msg of neither BA nor VID")
		}
		msgs = append(msgs, msg)
	}
	return msgs
}

// Recv handles the given message.
func (p *PikaEpoch) Recv(mg Message) ([]Message, Event) {
	m := mg.(*PikaEpochMessage)
	var msgs []Message
	var event Event

	// process according to message type
	if m.VIDMsg != nil {
		pm := m.VIDMsg
		vp := p.WriteVID(m.Idx)
		defer p.FreeVID(m.Idx)
		outMsgs, e := vp.Recv(pm)
		msgs = append(msgs, p.forwardMessages(outMsgs, m.Idx)...)

		// if the VID is terminated, we should try input true into the BA
		if (e & Terminate) != 0 {
			msgs = append(msgs, p.inputBA(m.Idx, true)...)
			msgs = append(msgs, p.Release(m.Idx)...)
			if p.requestStarted {
				msgs = append(msgs, p.Request(m.Idx)...)
			}
		}
	} else if m.BAMsg != nil {
		pm := m.BAMsg
		outMsgs, e := p.b[m.Idx].Recv(pm)
		msgs = append(msgs, p.forwardMessages(outMsgs, m.Idx)...)

		// if BA just terminated
		if (e & Terminate) != 0 {
			// if BA outputs true, request the chunks and release the chunks
			if p.b[m.Idx].Output() {
				msgs = append(msgs, p.Release(m.Idx)...)
				if p.requestStarted {
					msgs = append(msgs, p.Request(m.Idx)...)
				}
			}

			// check how many BA has output true. if N-F BA has output true, input false to the remaining ones
			if p.numBATrue() >= (p.N - p.F) {
				for i := 0; i < p.N; i++ {
					msgs = append(msgs, p.inputBA(i, false)...)
				}
			}
		}
	} else {
		panic("unknown message type")
	}

	// check if all BA has terminated. If so, we have a chance to terminate
	allBATerm := true
	for i := 0; i < p.N; i++ {
		if !p.b[i].Terminated() {
			allBATerm = false
		}
	}
	// if not all BA has terminated, no need to progress further
	if !allBATerm {
		return msgs, 0
	}

	// check if all VIDs with BA terminated true can be decoded
	chunkRcvd := true
	for i := 0; i < p.N; i++ {
		if p.b[i].Output() {
			if !p.PayloadRetrieved(i) {
				chunkRcvd = false
			}
		}
	}

	// calculate how many blocks we are accepting, for logging purpose
	numAcc := 0
	for i := 0; i < p.N; i++ {
		if p.b[i].Output() {
			numAcc += 1
		}
	}

	// decide whether we can terminate - if we have decoded all blocks, we can terminate
	if chunkRcvd {
		if !p.hptermed {
			p.hptermed = true
			p.Printf("high-priority terminating while accepting %v blocks\n", numAcc)
			event |= Dispersed
		}
		if !p.termed {
			p.termed = true
			p.Printf("terminating while accepting %v blocks\n", numAcc)
			p.Printf("confirming before these levels: %v\n", p.ConfirmUntil())
			event |= Terminate
		}
	} else {
		// NOTE: we can HP terminate as long as all BA terminates. This is because if BA outputs 1, we will
		// eventually see VID terminate. Otherwise, we don't care anyway. Here HP terminate indicates the fate
		// of this epoch is sealed.
		if !p.hptermed {
			p.hptermed = true
			p.Printf("high-priority terminating while accepting %v blocks\n", numAcc)
			event |= Dispersed
		}
	}
	return msgs, event
}

// SetPayload sets the block for this PikaEpoch.
func (p *PikaEpoch) SetPayload(b *PikaBlock) {
	p.payload = b
}

// NewPikaEpoch constructs a new PikaEpoch.
func NewPikaEpoch(codec ErasureCode, pr ProtocolParams) *PikaEpoch {
	p := &PikaEpoch{
		codec:          codec,
		ProtocolParams: pr,
	}
	// create VID and BA instances
	var v []*VID
	var b []*BA
	for i := 0; i < p.N; i++ {
		vid := NewVID(i, p.ProtocolParams.WithPrefix("VID", i), p.codec)
		vid.ProtocolParams.Logger = nil
		v = append(v, vid)
		ba := NewBA(p.ProtocolParams.WithPrefix("BA", i))
		ba.ProtocolParams.Logger = nil
		b = append(b, ba)
	}
	p.v = v
	p.b = b
	p.vStorage = make([]int, p.N)
	p.vRefCount = make([]int, p.N)
	p.requested = make([]bool, p.N)
	p.released = make([]bool, p.N)
	p.retrieved = make([]bool, p.N)
	p.outputted = make([]bool, p.N)
	p.bInit = make([]bool, p.N)
	return p
}

// Init initializes the dispersion of our block, and send true into the corresponding BA.
func (p *PikaEpoch) Init() ([]Message, Event) {
	// set our vid with proper payload
	p.v[p.ID].SetPayload(p.payload)

	var msgs []Message

	// init our VID
	outMsgs, _ := p.v[p.ID].Init()
	msgs = append(msgs, p.forwardMessages(outMsgs, p.ID)...)

	// release all chunks
	for i := 0; i < p.N; i++ {
		msgs = append(msgs, p.Release(i)...)
	}

	// since we are honest, we can input true into our BA at the same time
	msgs = append(msgs, p.inputBA(p.ID, true)...)
	return msgs, 0
}

// PikaMessage is a message handled and sent by Pika.
type PikaMessage struct {
	Sent time.Time
	Epoch int
	Msg   *PikaEpochMessage
	Dummy bool
	Finish bool
	FromID int
	DestID int
}

// Dest returns the destination of the message.
func (m PikaMessage) Dest() int {
	if m.Finish {
		return m.DestID
	}
	return m.Msg.Dest()
}

// From returns the source of the message.
func (m PikaMessage) From() int {
	if m.Finish {
		return m.FromID
	}
	return m.Msg.From()
}

// Priority returns the priority of the message.
func (m PikaMessage) Priority() Priority {
	if m.Finish {
		return High
	}
	return m.Msg.Priority()
}

// Size returns the size of the message in the emulator.
func (m PikaMessage) Size() int {
	if m.Finish {
		return 8
	}
	return m.Msg.Size() + 8
}

// PikaPayloadQueue is the interface that a queue should implement to supply payload
// data to Pika.
type PikaPayloadQueue interface {
    QueueLength() int
	TakeBlock() PikaPayload              // take a new block
	EmptyBlock() PikaPayload			 // take an empty block
	FillBlock(b PikaPayload) PikaPayload // fill additional data into the given block
	LogQueueStatus() string              // return a message of the queue status to be printed to the log
	Stop()                               // stop accepting or generating new data
}

// Pika contains the state of the Pika consensus protocol.
type Pika struct {
	coupledValidity bool
	maxEpoch          int          // the max. epoch index, -1 for unlimited
	epochs            []*PikaEpoch // epochs of the Pika instance
	numDispersed      int          // the number of epochs that have finished dispersion (high-priority terminate)
	numContinuousDispersed int
	firstUnterminated int          // the first epoch that is not terminated
	firstUninited     int          // the first epoch that we have not initialized (dispersion)
	firstUnoutput     int          // the first epoch that is not output - we must output in sequence, and can output iff all previous epochs are fully terminated
	firstNotStartedRequest int
	pipeline          int          // pipeline depth - the max. number of epochs that have been inited but not terminated, 0 means unlimited
	parallelism       int          // dispersion parallelism - the max. number of epochs that have been inited but not dispersed, 0 means unlimited
	ProtocolParams
	firstUndispersed []int                     // the first epoch of each node that the VID has not finished
	firstUnretrieved []int                     // the first epoch of each node that we have not decoded the dispersed block
	firstUnrequested []int                     // the first epoch of each node that we have not requested the dispersed data
	progress         chan struct{ Id, Pg int } // the channel through which we report the number of terminated epochs for drawing progress bar
	queue            PikaPayloadQueue          // the transaction queue
	chained          bool                      // if we are in chained mode - chained means that a node refers to blocks from other nodes
	codec            ErasureCode               // the codec used in the protocol
}

// NewPika constructs a new instance of Pika.
func NewPika(chained bool, maxEpoch int, pipelineDepth int, maxParallelism int, txQueue PikaPayloadQueue, codec ErasureCode, progressChan chan struct{ Id, Pg int }, cv bool, pr ProtocolParams) *Pika {
	p := &Pika{
		coupledValidity: cv,
		maxEpoch:         maxEpoch,
		pipeline:         pipelineDepth,
		parallelism:      maxParallelism,
		ProtocolParams:   pr,
		progress:         progressChan,
		queue:            txQueue,
		firstUndispersed: make([]int, pr.N),
		firstUnrequested: make([]int, pr.N),
		firstUnretrieved: make([]int, pr.N),
		chained:          chained,
		codec:            codec,
	}
	p.epochs = append(p.epochs, NewPikaEpoch(p.codec, p.ProtocolParams.WithPrefix("PikaEpoch", 0)))
	return p
}

// FirstUndispersed returns the first epoch of node idx where the VID has not terminated.
func (p *Pika) FirstUndispersed(idx int) int {
	for ei := p.firstUndispersed[idx]; ei < len(p.epochs); ei++ {
		// check if this epoch has seen dispersion complete
		if p.epochs[ei].PayloadDispersed(idx) {
			// update the firstUndispersed counter to be the next epoch
			p.firstUndispersed[idx] = ei + 1
		} else {
			// if not dispersed, exit
			break
		}
	}
	return p.firstUndispersed[idx]
}

// FirstUnretrieved returns the first epoch of which we have not decoded the dispersed data.
func (p *Pika) FirstUnretrieved(idx int) int {
	for ei := p.firstUnretrieved[idx]; ei < len(p.epochs); ei++ {
		reted := p.epochs[ei].PayloadRetrieved(idx)
		if reted {
			p.firstUnretrieved[idx] = ei + 1
		} else {
			break
		}
	}
	return p.firstUnretrieved[idx]
}

// FirstUnterminated returns the first epoch which has not terminated.
func (p *Pika) FirstUnterminated() int {
	for ei := p.firstUnterminated; ei < len(p.epochs); ei++ {
		if p.epochs[ei].termed {
			p.firstUnterminated = ei + 1
		} else {
			break
		}
	}
	return p.firstUnterminated
}

func (p *Pika) sendOutFinish(e int) []Message {
	var msgs []Message
	for i := 0; i < p.N; i++ {
		msgs = append(msgs, &PikaMessage {
			Sent: time.Now(),
			Epoch: e,
			Finish: true,
			FromID: p.ID,
			DestID: i,
		})
	}
	return msgs
}

// forwardMsgs takes a slice of *PikaEpochMessage and wraps them inside PikaMessage with
// the given epoch number, and returns the slice of resulting messages.
func (p *Pika) forwardMsgs(m []Message, e int) []Message {
	var msgs []Message
	for _, v := range m {
		msg := &PikaMessage{}
		msg.Sent = time.Now()
		msg.Epoch = e
		msg.Msg = v.(*PikaEpochMessage)
		msgs = append(msgs, msg)
	}
	return msgs
}

// Init initializes the protocol.
func (p *Pika) Init() ([]Message, Event) {
	var msgs []Message
	for {
		can, nonempty := p.canInitNext()
		if can {
			msgs = append(msgs, p.initNextEpoch(nonempty)...)
		} else {
			break
		}
	}
	return msgs, 0
}

// RequestRefBlocks requests all the blocks that will be confirmed by the given epoch.
func (p *Pika) RequestRefBlocks(ep int) []Message {
	var msgs []Message
	this := p.epochs[ep].ConfirmUntil()
	for i := 0; i < p.N; i++ {
		for lv := p.firstUnrequested[i]; lv < this[i]; lv++ {
			outM := p.epochs[lv].Request(i)
			msgs = append(msgs, p.forwardMsgs(outM, lv)...)
		}
		if p.firstUnrequested[i] < this[i] {
			p.firstUnrequested[i] = this[i]
		}
	}
	// TODO: mark "for which epoch" so that we will retrieve blocks for later epochs that are referred here early
	return msgs
}

func (p *Pika) StartRequest(ep int) []Message {
	var msgs []Message
	msgs = append(msgs, p.forwardMsgs(p.epochs[ep].StartRequesting(), ep)...)
	return msgs
}

// BlocksReady checks if all the blocks that will be confirmed by the given epoch have been
// decoded by us.
func (p *Pika) BlocksReady(ep int) bool {
	this := p.epochs[ep].ConfirmUntil()
	for i := 0; i < p.N; i++ {
		if p.FirstUnretrieved(i) < this[i] {
			return false
		}
	}
	return true
}

var logBufPool = sync.Pool{
	New: func() interface{} {
		return new(strings.Builder)
	},
}

// PrintProbes prints the confirmation messages of all blocks confirmed by the given epoch.
func (p *Pika) PrintProbes(ep int) {
	if ep > 0 && !p.epochs[ep-1].Terminated() {
		panic("PrintProbes called when the previous epoch is not terminated")
	}
	var lastLevels []int
	if ep != 0 {
		lastLevels = p.epochs[ep-1].ConfirmUntil()
	} else {
		lastLevels = make([]int, p.N)
	}
	thisLevels := p.epochs[ep].ConfirmUntil()

	for i := 0; i < p.N; i++ {
		neps := 0 // number of blocks

		buf := logBufPool.Get().(*strings.Builder)
		buf.Reset()

		// confirm all blocks starting from the last confirmed level
		for lv := lastLevels[i]; lv < thisLevels[i]; lv++ {
			// do nothing if already confirmed
			if !p.epochs[lv].Outputted(i) {
				// mark it as confirmed
				p.epochs[lv].MarkOutput(i)
				// retrieve the payload
				pldPtr, _ := p.epochs[lv].Payload(i)
				buf.WriteString(pldPtr.Payload.LogConfirm(i))
				neps += 1
				// we are doing some dirty hack here: Payload() may have got the VID by paging in
				// so we need to force paging out here
				vidptr := p.epochs[lv].WriteVID(i)
				p.epochs[lv].StashVID(i) // TODO: dirty hack
				defer p.epochs[lv].FreeVID(i)
				pld, _ := vidptr.Payload()
				pld.(*PikaBlock).Payload = nil // TODO: dirty hack
				p.epochs[lv].v[i] = vidptr
			}
		}

		// then see if the current epoch has BA output true
		if p.epochs[ep].b[i].Output() && !p.epochs[ep].Outputted(i) {
			// mark it as confirmed
			p.epochs[ep].MarkOutput(i)
			// retrieve the payload
			pldPtr, _ := p.epochs[ep].Payload(i)
			buf.WriteString(pldPtr.Payload.LogConfirm(i))
			neps += 1
			// we are doing some dirty hack here: Payload() may have got the VID by paging in
			// so we need to force paging out here
			vidptr := p.epochs[ep].WriteVID(i)
			p.epochs[ep].StashVID(i) // TODO: dirty hack
			defer p.epochs[ep].FreeVID(i)
			pld, _ := vidptr.Payload()
			pld.(*PikaBlock).Payload = nil // TODO: dirty hack
			p.epochs[ep].v[i] = vidptr
		}
		p.Printf("confirming %v blocks proposed by node %d:%s", neps, i, buf.String())
		logBufPool.Put(buf)
	}
}

// pipelineFull checks if the number of initialized but non-terminated epochs is greater or equal to the pipeline depth
func (p *Pika) pipelineFull() bool {
	if p.pipeline == 0 {
		// no limit
		return false
	} else {
		// TODO: do we want to block by the NUMBER of unoutput rounds, or the FIRST unoutput round? We are doing the latter now
		// check that the number of inited but unoutputted epochs is smaller than pipeline depth
		if (p.firstUninited - p.firstUnoutput) < p.pipeline {
			return false
		} else {
			return true
		}
	}
}

// parallelismCap checks if the number of initialized but non-dispersed (high-priority terminate) epochs is greater or
// equal to the parallelism cap
func (p *Pika) parallelismCap() bool {
	if p.parallelism == 0 {
		return false
	} else {
		if (p.firstUninited - p.numDispersed) < p.parallelism {
			return false
		} else {
			return true
		}
	}
}

var PreviousCheck time.Time

// canInitNext checks if we can initialize the next epoch (the current p.firstUninited)
// It also determines if we should propose a non-empty block (true = nonempty)
func (p *Pika) canInitNext() (bool, bool) {
	// 250byte transactions
	// Nagle's algorithm, size is in transactions
	if p.queue.QueueLength() < NagleSize {
		timeFromLastCheck := time.Now().Sub(PreviousCheck)
		if timeFromLastCheck.Milliseconds() <= int64(NagleDelay) {
			return false, false
		}
	}
	// if epoch 0 is not initialized, we always can
	// TODO: note that this will allow the protocol to initialize itself without user calling Init
	if p.firstUninited == 0 {
		return true, true
	}

	// if p.firstUninited is larger than maxEpoch index, we can't (maxEpoch is unlimited when set to -1)
	if p.maxEpoch != -1 && p.firstUninited > p.maxEpoch {
		return false, false
	}

	// if the pipeline is full, we can't
	if p.pipelineFull() {
		return false, false
	}

	// if we are capped by parallelism. don't
	if p.parallelismCap() {
		return false, false
	}

	PreviousCheck = time.Now()
	// if we should propose an empty block - do so when the pipeline stages is greater than 1
	if p.firstUninited - p.firstUnoutput > 2 {
		return true, false
	} else {
		return true, true
	}
}

func (p *Pika) allocateUntilEpoch(idx int) {
	for i := len(p.epochs); i <= idx; i++ {
		p.epochs = append(p.epochs, NewPikaEpoch(p.codec, p.ProtocolParams.WithPrefix("PikaEpoch", i)))
	}
}

// initNextEpoch initializes the next epoch
func (p *Pika) initNextEpoch(nonempty bool) []Message {
	var msgs []Message

	// construct the block. check if the block was accepted in the previous epoch
	var block *PikaBlock
	if p.chained {
		// we are simply taking to the max. size because we are in referenced mode
		var pld PikaPayload
		if p.coupledValidity && (!nonempty) {
			pld = p.queue.EmptyBlock()
		} else {
			pld = p.queue.TakeBlock()
		}
		// put the level of VIDs
		firstUnfinishedVIDs := make([]int, p.N)
		for i := 0; i < p.N; i++ {
			firstUnfinishedVIDs[i] = p.FirstUndispersed(i)
			// we don't refer beyond the current level
			if firstUnfinishedVIDs[i] > p.firstUninited {
				firstUnfinishedVIDs[i] = p.firstUninited
			}
		}
		block = &PikaBlock{
			References: firstUnfinishedVIDs,
			Payload:    pld,
		}
	} else {
		var pld PikaPayload
		// if we are not in chained mode, see if the block got through in the previous epoch
		if p.coupledValidity && (!nonempty) {
			pld = p.queue.EmptyBlock()
		} else if p.firstUninited != 0 && !p.epochs[p.firstUninited-1].b[p.ID].Output() {
			// if the previous block was rejected, we need to repropose those data
			pld = p.queue.FillBlock(p.epochs[p.firstUninited-1].payload.Payload)
		} else {
			// otherwise, we can propose a brand new block
			pld = p.queue.TakeBlock()
		}
		// plug zeros into the reference list
		block = &PikaBlock{
			References: make([]int, p.N),
			Payload:    pld,
		}
	}
	// now we can free the payload of the previous level
	// TODO: this looks very ugly
	if p.firstUninited != 0 {
		p.epochs[p.firstUninited-1].payload = nil
	}

	// construct the epoch if it is not there
	p.allocateUntilEpoch(p.firstUninited)

	// set the payload and initialize the epoch
	p.epochs[p.firstUninited].SetPayload(block)
	m, _ := p.epochs[p.firstUninited].Init() // don't bother checking output - epoch.Init always returns Progress
	msgs = append(msgs, p.forwardMsgs(m, p.firstUninited)...)

	if p.queue != nil {
		p.Printf("%s\n", p.queue.LogQueueStatus())
	}
	p.Printf("starting Pika epoch %d, %s\n", p.firstUninited, p.epochs[p.firstUninited].payload.Payload.LogPropose())

	p.firstUninited += 1 // increase the counter
	return msgs
}

func (p *Pika) Recv(mg Message) ([]Message, Event) {
	m := mg.(*PikaMessage)
	em := m.Msg
	var msgs []Message

	if m.Finish {
		CanceledReqMux.Lock()
		if len(FinishedEpochs) == 0 {
			FinishedEpochs = make([]int, p.N)
		}
		if FinishedEpochs[m.From()] < m.Epoch {
			FinishedEpochs[m.From()] = m.Epoch
		}
		CanceledReqMux.Unlock()
		return nil, 0
	}

	if !m.Dummy {
		// if it is a cancellation message
		ep := m.Epoch
		idx := em.Idx
		if em.VIDMsg != nil {
			if em.VIDMsg.Cancel {
				CanceledReqMux.Lock()
				for i := len(CanceledReq); i <= ep; i++ {
					var canceled [][]bool
					for j := 0; j < p.N; j++ {
						canceled = append(canceled, make([]bool, p.N))
					}
					CanceledReq = append(CanceledReq, canceled)
				}
				CanceledReq[ep][idx][m.From()] = true
				CanceledReqMux.Unlock()
				return nil, 0
			}
		}

		// make sure that the have constructed p.epochs[] up until [m.epoch]
		// TODO: this is a DDoS vulnerability
		p.allocateUntilEpoch(m.Epoch)

		// process the message
		out, progress := p.epochs[m.Epoch].Recv(em)
		msgs = append(msgs, p.forwardMsgs(out, m.Epoch)...)

		// update our counters
		if progress&Dispersed != 0 {
			p.numDispersed += 1
		}

		oldncd := p.numContinuousDispersed
		for  i := p.numContinuousDispersed; i < len(p.epochs); i++ {
			if p.epochs[i].hptermed {
				p.numContinuousDispersed = i
			}
		}
		if p.numContinuousDispersed != oldncd {
			p.Printf("continuous dispersed %v epochs\n", p.numContinuousDispersed)
			atomic.StoreInt64(&NumContDispersed, int64(p.numContinuousDispersed))
		}
	}

	// start requesting at most 30 levels beyond the currently output level
	for i := p.firstNotStartedRequest; i < p.firstUnoutput + p.pipeline + 10; i++ {
		if len(p.epochs) <= i {
			break
		}
		msgs = append(msgs, p.StartRequest(i)...)
		p.firstNotStartedRequest = i + 1
	}

	// request referenced blocks for the next130 epochs that are terminated
	for i := p.firstUnoutput; i < p.firstUnoutput + p.pipeline + 10; i++ {
		if len(p.epochs) <= i {
			break
		}
		if p.epochs[i].Terminated() {
			msgs = append(msgs, p.RequestRefBlocks(i)...)
		}
	}

	// see if we can initialize the next epoch (initialize as many as we can)
	prev := p.firstUninited
	for {
		can, nonempty := p.canInitNext()
		if can {
			msgs = append(msgs, p.initNextEpoch(nonempty)...)
		} else {
			break
		}
	}
	if prev != p.firstUninited {
	}

	// output as many epochs as possible, so we search forward
	// check it no matter what. a bug was introduce when checking only when p.firstUnoutput==m.epoch
	// we can't do that because a message to an epoch earlier than firstUnoutput may fulfill chunks
	// of a block not accepted in that epoch, but required by firstUnoutput (in chained mode)
	prevNumUnoutput := p.firstUnoutput
	for epidx := p.firstUnoutput; epidx < len(p.epochs); epidx++ {
		// if the epoch is terminated and all the referred blocks are there, we can output
		if p.epochs[epidx].Terminated() && p.BlocksReady(epidx) {
			p.Println("outputting epoch ", epidx)
			// output this epoch
			p.PrintProbes(epidx)
			// report the progress
			if p.progress != nil {
				p.progress <- struct{ Id, Pg int }{p.ID, p.firstUnoutput}
			}
			// update the first unoutput epoch
			p.firstUnoutput = epidx + 1
		} else {
			break
		}
	}

	if p.firstUnoutput != prevNumUnoutput {
		atomic.StoreInt64(&FirstUnconfirmed, int64(p.firstUnoutput))
		msgs = append(msgs, p.sendOutFinish(p.firstUnoutput)...)
	}


	// check if we have initiated all epochs
	if p.maxEpoch != -1 && len(p.epochs) >= p.maxEpoch+1 {
		if p.firstUninited > p.maxEpoch {
			// stop transaction generation
			if p.queue != nil {
				p.queue.Stop()
			}
		}
	}

	// check if we have output all epochs
	// only check if more than maxEpoch+1 epochs have been constructed
	if p.maxEpoch != -1 && len(p.epochs) >= p.maxEpoch+1 {
		if p.firstUnoutput > p.maxEpoch {
			// stop transaction generation
			if p.queue != nil {
				p.queue.Stop()
			}
			return msgs, Terminate
		}
	}

	// in pipeline mode, if the current pipeline is full, it means that we need to download low-priority
	// data and complete some retrievals. in that case, switch to low-priority
	if p.pipelineFull() {
		return msgs, BlockedByLowPriorityMessage
	} else {
		return msgs, BlockedByHighPriorityMessage
	}
}

var CanceledReq [][][]bool
var FinishedEpochs []int
var CanceledReqMux = &sync.Mutex{}
var NumContDispersed int64

func BottleneckEpochs(f int) int {
	CanceledReqMux.Lock()
	defer CanceledReqMux.Unlock()

	if len(FinishedEpochs) == 0 {
		return int(atomic.LoadInt64(&NumContDispersed))
	}
	nums := make([]int, len(FinishedEpochs))
	copy(nums, FinishedEpochs)
	sort.Ints(nums)
	return nums[f]
}

