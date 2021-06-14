package pika

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
)

// TODO: we should pass the pointer to the on-disk shard in the message, and let the
// message encoder to retrieve the data.

// ErasureCode is the interface that wraps the methods that an erasure code should support.
type ErasureCode interface {
	// Encode the given object into a slice of ErasureCodeChunk.
	Encode(input VIDPayload) ([]ErasureCodeChunk, error)

	// Decode a slice of ErasureCodeChunk into the original object. Note that the chunks may be unordered, but
	// all chunks in the slice should be valid.
	Decode(shards []ErasureCodeChunk, v *VIDPayload) error
}

// VIDChunk is the interface that an erasure coded chunk should implement.
type ErasureCodeChunk interface {
	Sizer
}

// StoredErasureCodeChunk is an erasure chunk that is stored. It could be in the memory
// or on the database.
type StoredErasureCodeChunk struct {
	InMemoryValue ErasureCodeChunk
	DBKey         []byte
	IsStored      bool
}

// BUG(leiy): StoredErasureCodeChunk does not have a "delete" or "update" method.

func (s *StoredErasureCodeChunk) LoadPointer(db KVStore) *InMessageChunk {
	if s.inMemory() {
		// TODO
		panic("can't load in-memory chunk into pointer")
	}
	return &InMessageChunk{
		db:  db,
		key: s.DBKey,
	}
}

func (s *StoredErasureCodeChunk) StorePointer(c *InMessageChunk, db KVStore) {
	if c.decoded {
		err := db.Put(s.DBKey, c.data)
		if err != nil {
			panic(err)
		}
	} else {
		d, err := db.Get(c.key)
		if err != nil {
			panic(err)
		}
		err = db.Put(s.DBKey, d)
		if err != nil {
			panic(err)
		}
	}
	s.IsStored = true
}

func (s *StoredErasureCodeChunk) Stored() bool {
	return s.IsStored
}

// inMemory returns if the value is currently on the disk
func (s *StoredErasureCodeChunk) inMemory() bool {
	// if the DBKey is set, it is definitely not in the memory
	if s.DBKey != nil {
		return false
	} else {
		return true
	}
}

func (s *StoredErasureCodeChunk) storeOnDisk(val ErasureCodeChunk, db KVStore) {
	//bm, is := val.(encoding.BinaryMarshaler)
	var b []byte
	/*
		// we can't use marshalbinary here because we don't have the concrete type
		// to decode into when retrieving from the disk
		if is {
			b, err := bm.MarshalBinary()
			if err != nil {
				panic(err)
			}
			s.bmarshaller = true
		} else {
	*/
	buf := &bytes.Buffer{}
	enc := gob.NewEncoder(buf)
	err := enc.Encode(&val)
	if err != nil {
		panic(err)
	}
	b = buf.Bytes()
	//}
	err = db.Put(s.DBKey, b)
	if err != nil {
		panic(err)
	}
	return
}

func (s *StoredErasureCodeChunk) loadFromDisk(db KVStore) ErasureCodeChunk {
	var val ErasureCodeChunk
	data, err := db.Get(s.DBKey)
	if err != nil {
		panic(err)
	}
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)

	err = dec.Decode(&val)
	if err != nil {
		panic(err)
	}
	return val
}

// Store stores the given ErasureCodeChunk to the StoredErasureCodeChunk.
func (s *StoredErasureCodeChunk) Store(val ErasureCodeChunk, db KVStore) {
	s.IsStored = true
	if s.inMemory() {
		s.InMemoryValue = val
	} else {
		s.storeOnDisk(val, db)
	}
}

func (s *StoredErasureCodeChunk) Load(db KVStore) ErasureCodeChunk {
	if s.inMemory() {
		return s.InMemoryValue
	} else {
		return s.loadFromDisk(db)
	}
}

func (s *StoredErasureCodeChunk) Stash(db KVStore, key []byte) {
	s.DBKey = key
	if s.IsStored {
		s.storeOnDisk(s.InMemoryValue, db)
		s.InMemoryValue = nil
	}
	return
}

// VIDPayload is the interface that a payload of the VID protocol should implement.
type VIDPayload interface{}

// EmptyPayload is an empty payload for the VID protocol. It is used as a placeholder when there is nothing to be attached
// to the VID instance.
type EmptyPayload struct{}

// Size always returns 0. An empty payload is considered to take no space in the emulator.
func (e *EmptyPayload) Size() int {
	return 0
}

// We are going type embedding here. This is to help us remove unneeded states when the time
// comes. For example, we can remove all received chunks after decoding the payload.

// VIDOutput is the output of a VID instance.
type VIDOutput struct {
	payload           VIDPayload             // the object being dispersed in the VID, nil if it is not yet retrieved or decoded
	associatedData    VIDPayload             // the object being broadcasted in the VID, nil if it is not yet retrieved or decoded
	ourChunk          StoredErasureCodeChunk // the erasure coded chunk of the dispersed file that should be held by us, nil if not received
	ourEcho           StoredErasureCodeChunk // the erasure coded chunk of the broadcasted file that should be held by us, nil if not received
	requestUnanswered []bool                 // nil if we are answering chunk requests right away; otherwise, true if we have not answered the chunk
	// request previously sent by the corresponding node
	canReleaseChunk bool // if we are allowed to release the payload chunk
	canceled        bool
}

// VIDPayloadState is the execution state of the VID that is related to decoding the dispersed file.
type VIDPayloadState struct {
	chunks           []StoredErasureCodeChunk // the chunks that we have received, or nil if we have not received the chunk from that server
	nChunks          int                      // the number of chunks that we have received, which should equal the number of non-nil items in chunks
	payloadScheduled bool                     // if we want to request chunks
	sentRequest      bool                     // if we have requested payload chunks
}

// VIDCoreState is the core execution state of the VID.
type VIDCoreState struct {
	initID int                      // ID of the node which should disperse the file
	echos  []StoredErasureCodeChunk // the chunks of the broadcasted file that we have received in Echo messages,
	// or nil if we have not received Echo from that server
	nEchos    int    // the number of echos that we have received, which should equal the number of non-nil items in echos
	readys    []bool // if we received ready from that server
	nReadys   int    // the number of readys we received, which should equal the number of true's in readys
	sentEcho  bool   // if we have sent out Echo
	sentReady bool   // if we have sent out Ready
}

// VID is an instance of Verifiable Information Dispersal. A VID can disperse an object and broadcast another object. The VID is terminated
// when the two files are guaranteed to be retrievable. The broadcasted file takes fewer network delays to be delivered it does for
// the dispersed file.
type VID struct {
	*VIDPayloadState
	*VIDCoreState
	*VIDOutput
	codec ErasureCode // the codec we want to use
	ProtocolParams
}

// MarshalBinary implements encoding.BinaryMarshaler. It marshals the VID instance into binary. It does not marshal the codec the VID
// uses and the ProtocolParams, so the user should supply these values during decoding.
func (v *VID) MarshalBinary() ([]byte, error) {
	// BUG(leiy): we are not marshaling the codec, but the codec is also private, so it is impossible for outside user to manually
	// assign it when decoding.
	buf := &bytes.Buffer{}
	enc := gob.NewEncoder(buf)

	err := enc.Encode(&v.N)
	if err != nil {
		return nil, err
	}
	err = enc.Encode(&v.payload)
	if err != nil {
		return nil, err
	}
	err = enc.Encode(&v.associatedData)
	if err != nil {
		return nil, err
	}
	err = enc.Encode(&v.ourChunk)
	if err != nil {
		return nil, err
	}
	err = enc.Encode(&v.ourEcho)
	if err != nil {
		return nil, err
	}
	err = enc.Encode(&v.requestUnanswered)
	if err != nil {
		return nil, err
	}
	err = enc.Encode(&v.canReleaseChunk)
	if err != nil {
		return nil, err
	}
	err = enc.Encode(&v.canceled)
	if err != nil {
		return nil, err
	}
	err = enc.Encode(v.VIDCoreState != nil)
	if err != nil {
		return nil, err
	}
	if v.VIDCoreState != nil {
		err = enc.Encode(&v.initID)
		if err != nil {
			return nil, err
		}
		err = enc.Encode(&v.echos)
		if err != nil {
			return nil, err
		}
		err = enc.Encode(&v.nEchos)
		if err != nil {
			return nil, err
		}
		err = enc.Encode(&v.readys)
		if err != nil {
			return nil, err
		}
		err = enc.Encode(&v.nReadys)
		if err != nil {
			return nil, err
		}
		err = enc.Encode(&v.sentEcho)
		if err != nil {
			return nil, err
		}
		err = enc.Encode(&v.sentReady)
		if err != nil {
			return nil, err
		}
	}
	err = enc.Encode(v.VIDPayloadState != nil)
	if err != nil {
		return nil, err
	}
	if v.VIDPayloadState != nil {
		err = enc.Encode(&v.chunks)
		if err != nil {
			return nil, err
		}
		err = enc.Encode(&v.nChunks)
		if err != nil {
			return nil, err
		}
		err = enc.Encode(&v.payloadScheduled)
		if err != nil {
			return nil, err
		}
		err = enc.Encode(&v.sentRequest)
		if err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler. It unmarshals binary into the VID state. Note that it does not modify the
// ProtocolParams and the codec used by the VID, so the user must supply these values either before decoding, or after decoding but
// before using the newly decoded value.
func (v *VID) UnmarshalBinary(data []byte) error {
	v.VIDOutput = &VIDOutput{}
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)

	var N int
	err := dec.Decode(&N)
	if err != nil {
		return err
	}
	err = dec.Decode(&v.payload)
	if err != nil {
		return err
	}
	err = dec.Decode(&v.associatedData)
	if err != nil {
		return err
	}
	err = dec.Decode(&v.ourChunk)
	if err != nil {
		return err
	}
	err = dec.Decode(&v.ourEcho)
	if err != nil {
		return err
	}
	v.requestUnanswered = make([]bool, N)
	err = dec.Decode(&v.requestUnanswered)
	if err != nil {
		return err
	}
	err = dec.Decode(&v.canReleaseChunk)
	if err != nil {
		return err
	}
	err = dec.Decode(&v.canceled)
	if err != nil {
		return err
	}
	var coreThere bool
	err = dec.Decode(&coreThere)
	if err != nil {
		return err
	}
	v.VIDCoreState = nil
	if coreThere {
		v.VIDCoreState = &VIDCoreState{}

		err = dec.Decode(&v.initID)
		if err != nil {
			return err
		}
		v.echos = make([]StoredErasureCodeChunk, N)
		err = dec.Decode(&v.echos)
		if err != nil {
			return err
		}
		err = dec.Decode(&v.nEchos)
		if err != nil {
			return err
		}
		v.readys = make([]bool, N)
		err = dec.Decode(&v.readys)
		if err != nil {
			return err
		}
		err = dec.Decode(&v.nReadys)
		if err != nil {
			return err
		}
		err = dec.Decode(&v.sentEcho)
		if err != nil {
			return err
		}
		err = dec.Decode(&v.sentReady)
		if err != nil {
			return err
		}
	}
	var payloadStateThere bool
	err = dec.Decode(&payloadStateThere)
	if err != nil {
		return err
	}
	v.VIDPayloadState = nil
	if payloadStateThere {
		v.VIDPayloadState = &VIDPayloadState{}

		v.chunks = make([]StoredErasureCodeChunk, N)
		err = dec.Decode(&v.chunks)
		if err != nil {
			return err
		}
		err = dec.Decode(&v.nChunks)
		if err != nil {
			return err
		}
		err = dec.Decode(&v.payloadScheduled)
		if err != nil {
			return err
		}
		err = dec.Decode(&v.sentRequest)
		if err != nil {
			return err
		}
	}
	if len(buf.Bytes()) != 0 {
		panic("unread buffer!")
	}
	return nil
}

type InMessageChunk struct {
	db      KVStore // will be set in the outgoing path
	key     []byte  // will be set in the outgoing path
	data    []byte  // will be set in the incoming path
	decoded bool
}

func (c *InMessageChunk) GobEncode() ([]byte, error) {
	data, err := c.db.Get(c.key)
	return data, err
}

func (c *InMessageChunk) GobDecode(data []byte) error {
	c.data = make([]byte, len(data))
	c.decoded = true
	copy(c.data, data)
	return nil
}

// VIDMessage is the message emitted and handled by the VID.
type VIDMessage struct {
	Echo            bool // true if this is an Echo message; an Echo message contains the broadcasted chunk
	Ready           bool // true if this is a Ready message
	Disperse        bool // true this is a Disperse message; a Disperse message contains the dispersed chunk
	RequestChunk    bool // true if this message requests a chunk of the dispersed file
	RespondChunk    bool // true if this message responds with a chunk request; such a message contains a dispersed chunk
	Cancel          bool
	PayloadChunk    interface{} // the chunk of the dispersed file
	AssociatedChunk interface{} // the chunk of the broadcasted file
	DestID          int         // destination of the message
	FromID          int         // source of the message
}

// String formats the VIDMessage for debug output.
func (m VIDMessage) String() string {
	t := ""
	if m.Echo {
		t += "Echo"
	}
	if m.Ready {
		t += "Ready"
	}
	if m.Disperse {
		t += "Disperse"
	}
	if m.RequestChunk {
		t += "Request"
	}
	if m.RespondChunk {
		t += "Respond"
	}
	return fmt.Sprintf("%v from node %d", t, m.FromID)
}

// Dest returns the destination of the message.
func (m VIDMessage) Dest() int {
	return m.DestID
}

// From returns the source of the message.
func (m VIDMessage) From() int {
	return m.FromID
}

// Priority returns Low if the message has RespondChunk marked true, or High if otherwise.
func (m VIDMessage) Priority() Priority {
	if m.RespondChunk {
		return Low
	} else if m.Disperse {
		return Medium
	} else {
		return High
	}
}

// Size returns the size of the message in the emulator. It is equal to the size of the PayloadChunk plus AssociatedChunk.
func (m VIDMessage) Size() int {
	totSize := 0
	if m.PayloadChunk != nil {
		totSize += m.PayloadChunk.(Sizer).Size()
	}
	if m.AssociatedChunk != nil {
		totSize += m.AssociatedChunk.(Sizer).Size()
	}
	return totSize
}

// handleEcho processes an Echo message with the given source and broadcasted chunk. It panics if the broadcasted chunk is nil.
// It is a nop if VIDCoreState is nil.
func (v *VID) handleEcho(from int, ad *InMessageChunk) {
	if v.VIDCoreState == nil {
		// if the core state is dropped, it means that we no longer need to handle echo
		return
	}
	if ad == nil {
		panic("handling echo message with nil associatedChunk")
	}

	// record the message, and we only take the first message
	if !v.echos[from].Stored() {
		v.nEchos += 1
		v.echos[from].StorePointer(ad, v.DBPath)
	}
}

// handleDisperse processes a Disperse message from the given source with the given dispersed and broadcasted chunk. It panics if
// either chunk is nil. It is a nop if VIDCoreState is nil, or if the source is not the initID of the VID.
func (v *VID) handleDisperse(from int, c ErasureCodeChunk, ad ErasureCodeChunk) {
	if v.VIDCoreState == nil {
		// it means that we have terminated, and decoded associated data
		return
	}
	if from != v.initID {
		// not the one who is supposed to send chunk
		return
	}

	if c == nil {
		panic("handling disperse message with nil payloadChunk")
	}
	if ad == nil {
		panic("handling disperse message with nil associatedChunk")
	}

	// record the message, and we only take the first message
	if !v.ourChunk.Stored() {
		// we need to check VIDPayloadState here, because the initiating node will hear its own Disperse message, and at
		// that time, the VIDPayloadState may already been marked as nil
		if v.VIDPayloadState != nil {
			v.chunks[v.ID].Store(c, v.DBPath)
			v.nChunks += 1
		}
		v.ourChunk.Store(c, v.DBPath)
	}
	if !v.ourEcho.Stored() {
		// see the comment above for why we need to check for VIDCoreState here
		if v.VIDCoreState != nil {
			v.echos[v.ID].Store(ad, v.DBPath)
			v.nEchos += 1
		}
		v.ourEcho.Store(ad, v.DBPath)
	}
}

// handleReady processes a Ready message from the given source. It is a nop if VIDCoreState is nil.
func (v *VIDCoreState) handleReady(from int) {
	if v == nil {
		// if the core state is dropped, it means that we no longer need to handle ready
		return
	}
	// record the message
	if !v.readys[from] {
		v.readys[from] = true
		v.nReadys += 1
	}
}

// handleChunkResponse handles a Response message from the given source and dispersed chunk. It is a nop if VIDPayloadState
// is nil, and panics if the dispersed chunk is nil.
func (v *VID) handleChunkResponse(from int, c *InMessageChunk) {
	if v.VIDPayloadState == nil {
		return
	}
	if c == nil {
		panic("handling chunk response message with nil payloadChunk")
	}

	// record the chunk and we only take the first message
	if !v.chunks[from].Stored() {
		v.nChunks += 1
		v.chunks[from].StorePointer(c, v.DBPath)
	}
	v.Printf("receiving chunk from node %v\n", from)
}

// respondRequest handles a Request message from the given source and returns a slice of messages to be sent as the response.
// If we are allowed to respond to the request, the response is sent right away. Otherwise, we record the request and return.
func (v *VID) respondRequest(from, ourid int) []Message {
	var msgs []Message

	// if we can respond to chunk requests
	if v.canReleaseChunk && v.ourChunk.Stored() {
		msg := &VIDMessage{}
		msg.RespondChunk = true
		msg.FromID = ourid
		msg.DestID = from
		msg.PayloadChunk = v.ourChunk.LoadPointer(v.DBPath)
		msgs = append(msgs, msg)
	} else {
		// otherwise buffer the response
		v.requestUnanswered[from] = true
	}
	return msgs
}

func (v *VID) sendOutCancel() []Message {
	var msgs []Message
	for i := 0; i < v.N; i++ {
		msg := &VIDMessage{}
		msg.FromID = v.ID
		msg.DestID = i
		msg.Cancel = true
		msgs = append(msgs, msg)
	}
	return msgs
}

// Recv handles a VID message of type *VIDMessage and returns a slice of messages to send and the execution result.
func (v *VID) Recv(mg Message) ([]Message, Event) {
	m := mg.(*VIDMessage)
	var msgs []Message

	// handle the message
	if m.Echo {
		v.handleEcho(m.FromID, m.AssociatedChunk.(*InMessageChunk))

		// if we have requested the dispersed file, but have not sent the requests, see if we can send
		// we send requests when we have got N-F Echos
		if v.payload == nil {
			if v.payloadScheduled {
				msg := &VIDMessage{}
				msg.RequestChunk = true
				msg.DestID = m.FromID
				msg.FromID = v.ID
				msgs = append(msgs, msg)
				v.Printf("requesting chunk from node %v\n", m.FromID)
			}
		}
	}
	if m.Ready {
		v.handleReady(m.FromID)
	}
	if m.Disperse {
		v.handleDisperse(m.FromID, m.PayloadChunk.(ErasureCodeChunk), m.AssociatedChunk.(ErasureCodeChunk))
	}
	if m.RespondChunk {
		v.handleChunkResponse(m.FromID, m.PayloadChunk.(*InMessageChunk))
	}
	if m.RequestChunk {
		msgs = append(msgs, v.respondRequest(m.FromID, v.ID)...)
	}
	// from now on, the message is not used anymore

	// logics that happens when we are not terminated
	if v.VIDCoreState != nil {
		// if we have received N-F Echos, send out Ready
		if v.nEchos >= (v.N-v.F) && !v.sentReady {
			for i := 0; i < v.N; i++ {
				msg := &VIDMessage{}
				msg.Ready = true
				msg.FromID = v.ID
				msg.DestID = i

				msgs = append(msgs, msg)
			}
			v.sentReady = true
			v.Println("sending out Ready due to enough Echos")
		}

		// if we have received F+1 Readys, send out Ready (if we have not done so)
		if v.nReadys >= (v.F+1) && !v.sentReady {
			for i := 0; i < v.N; i++ {
				msg := &VIDMessage{}
				msg.Ready = true
				msg.FromID = v.ID
				msg.DestID = i
				msgs = append(msgs, msg)
			}
			v.sentReady = true
			v.Println("sending out Ready due to enough Readys")
		}

		// if we have got our chunks, send out Echo
		if !v.sentEcho && v.ourChunk.Stored() && v.ourEcho.Stored() {
			for i := 0; i < v.N; i++ {
				msg := &VIDMessage{}
				msg.Echo = true
				msg.FromID = v.ID
				msg.DestID = i
				msg.AssociatedChunk = v.ourEcho.LoadPointer(v.DBPath)
				msgs = append(msgs, msg)
			}
			v.sentEcho = true
			v.Println("sending out Echos")
		}
	}

	// if we can answer chunk requests, answer the recorded ones now
	if v.requestUnanswered != nil && v.canReleaseChunk && v.ourChunk.Stored() {
		for from, t := range v.requestUnanswered {
			if t {
				msg := &VIDMessage{}
				msg.RespondChunk = true
				msg.FromID = v.ID
				msg.DestID = from
				msg.PayloadChunk = v.ourChunk.LoadPointer(v.DBPath)
				msgs = append(msgs, msg)
			}
		}
		// now that we have answered the requests, we don't need the record anymore
		v.requestUnanswered = nil
	}

	// if we have got N-2F Echos, decode the broadcasted file
	if v.VIDCoreState != nil {
		if v.associatedData == nil && v.nEchos >= v.N-v.F*2 {
			// collect the chunks
			chunks := make([]ErasureCodeChunk, v.N-v.F*2)
			collected := 0
			for _, val := range v.echos {
				if val.Stored() {
					chunks[collected] = val.Load(v.DBPath)
					collected += 1
				}
				if collected >= v.N-v.F*2 {
					break
				}
			}
			if collected < v.N-v.F*2 {
				panic("insufficient shards")
			}
			// decode the broadcasted file
			err := v.codec.Decode(chunks, &v.associatedData)
			if err != nil {
				panic(err)
			}
			v.Println("decoding associated data")
		}
	}

	// if we have got N-2F chunks, decode the dispersed file
	if v.VIDPayloadState != nil {
		if v.payload == nil && v.nChunks >= v.N-v.F*2 {
			// collect the chunks
			chunks := make([]ErasureCodeChunk, v.N-v.F*2)
			collected := 0
			for _, val := range v.chunks {
				if val.Stored() {
					chunks[collected] = val.Load(v.DBPath)
					collected += 1
				}
				if collected >= v.N-v.F*2 {
					break
				}
			}
			if collected < v.N-v.F*2 {
				panic("insufficient shards")
			}
			// decode the dispersed file
			err := v.codec.Decode(chunks, &v.payload)
			if err != nil {
				panic(err)
			}
			v.Println("decoding payload")
		}
		// delete payload state now that we have decoded the payload
		// note that we can't move this into the IF above, because the initiating node will never enter the IF above,
		// because it does not need to decode in order to obtain the payload
		if v.payload != nil {
			// BUG(leiy): We are not deleting the chunks (StoredErasureCodeChunk)
			// on the disk
			v.VIDPayloadState = nil
			if !v.canceled {
				msgs = append(msgs, v.sendOutCancel()...)
				v.canceled = true
			}
		}
	}

	// See if we can remove the core state. We can do it when the dispersed file have been requested (we need
	// the core state to know who have sent us Echos, so that we know who to request chunks from), and after
	// the protocol is terminated.
	// TODO: currently, we are removing it only after decoding the payload
	if (v.VIDPayloadState == nil) && v.Terminated() {
		// BUG(leiy): We are not deleting the echos (StoredErasureCodeChunk)
		// on the disk
		v.VIDCoreState = nil
	}

	if v.Terminated() {
		return msgs, Terminate
	} else {
		return msgs, 0
	}
}

// sendPayloadRequests send out requests for the dispersed file. It searches by the increasing order of node IDs,
// and sends the requests to the first N-F nodes it found to have sent us Echo. It panics when less than N-F nodes
// have sent us Echo.
func (v *VID) sendPayloadRequests() []Message {
	var msgs []Message
	nr := 0
	for idx, ec := range v.echos {
		// if we have received Echo from idx
		if ec.Stored() {
			msg := &VIDMessage{}
			msg.RequestChunk = true
			msg.DestID = idx
			msg.FromID = v.ID
			msgs = append(msgs, msg)
			nr += 1
			v.Printf("requesting chunk from node %v\n", idx)
		}
	}
	return msgs
}

// ReleaseChunk enables the VID to answer requests for its dispersed chunk. If we have got our dispersed chunk,
// it sends out responses to those who have sent us a request before and returns a slice of these messages.
// It is a nop if this function is called before.
func (v *VID) ReleaseChunk() []Message {
	var msgs []Message
	if v.canReleaseChunk {
		return msgs
	}
	v.canReleaseChunk = true
	// if we can answer to the requests
	if v.requestUnanswered != nil && v.ourChunk.Stored() {
		for from, t := range v.requestUnanswered {
			if t {
				msg := &VIDMessage{}
				msg.RespondChunk = true
				msg.FromID = v.ID
				msg.DestID = from
				msg.PayloadChunk = v.ourChunk.LoadPointer(v.DBPath)
				msgs = append(msgs, msg)
			}
		}
		// we don't need the buffer anymore
		v.requestUnanswered = nil
	}
	return msgs
}

// RequestPayload schedules the VID to request the dispersed file, and returns a slice of messages to be sent. If more than N-F
// nodes have sent us Echo, it sends out these requests right away. Otherwise, the request will be sent upon receiving N-F Echos.
// It is a nop if VIDPayloadState is nil, or if we have requested the dispersed file before.
func (v *VID) RequestPayload() []Message {
	if v.VIDPayloadState == nil {
		return nil
	}
	// if we have requested before, do nothing
	if v.payloadScheduled {
		return nil
	}
	var msgs []Message
	v.payloadScheduled = true

	// if we have not got the payload...
	if v.payload == nil {
		msgs = append(msgs, v.sendPayloadRequests()...)
	}

	return msgs
}

// Terminated checks if the file is successfully dispersed and we have decoded the broadcasted file.
func (v *VID) Terminated() bool {
	dispersed := v.PayloadDispersed()
	_, ad := v.AssociatedData()
	return dispersed && ad
}

// PayloadDispersed checks if the files are successfully dispersed, i.e. all nodes will eventually be able to retrieve and files.
// It does not check if the broadcasted file is already decoded by us. For that, the user should use Terminated. It always returns
// true if VIDCoreState is nil.
func (v *VID) PayloadDispersed() bool {
	if v.VIDCoreState == nil {
		return true
	}
	return v.nReadys >= v.F*2+1
}

// AssociatedData returns the broadcasted file and true if it is decoded, or nil and false if not.
func (v *VID) AssociatedData() (VIDPayload, bool) {
	adThere := v.associatedData != nil
	return v.associatedData, adThere
}

// Payload returns the dispersed file and true if it is decoded, or nil and false if not.
func (v *VID) Payload() (VIDPayload, bool) {
	pThere := v.payload != nil
	return v.payload, pThere
}

// Init starts the dispersion of this VID. It is a nop if the caller is not the node which is supposed to initiate the VID.
func (v *VID) Init() ([]Message, Event) {
	var msgs []Message
	// do nothing if we are not supposed to disperse
	if v.initID != v.ID {
		return msgs, 0
	}
	// encode the payload and the associated data
	pldChunks, err := v.codec.Encode(v.payload)
	if err != nil {
		panic("error encoding payload " + err.Error())
	}
	adChunks, err := v.codec.Encode(v.associatedData)
	if err != nil {
		panic("error encoding associated data" + err.Error())
	}
	// send out Disperse and Ready messages
	// can't do Echo here because both Echo and Disperse use the AssociatedChunk field
	for i := 0; i < v.N; i++ {
		msg := &VIDMessage{}
		msg.Disperse = true
		msg.Ready = true
		msg.FromID = v.ID
		msg.DestID = i
		msg.PayloadChunk = pldChunks[i]
		msg.AssociatedChunk = adChunks[i]
		msgs = append(msgs, msg)
	}
	v.Println("dispersing chunks")
	return msgs, 0
}

// NewVID constructs a new VID instance.
func NewVID(initID int, p ProtocolParams, codec ErasureCode) *VID {
	v := VID{
		ProtocolParams:  p,
		VIDCoreState:    &VIDCoreState{},
		VIDPayloadState: &VIDPayloadState{},
		VIDOutput:       &VIDOutput{},
		codec:           codec,
	}
	v.initID = initID
	v.chunks = make([]StoredErasureCodeChunk, p.N)
	v.echos = make([]StoredErasureCodeChunk, p.N)
	v.ourChunk.Stash(v.DBPath, v.dbKey("ourchunk", 0))
	v.ourEcho.Stash(v.DBPath, v.dbKey("ourecho", 0))
	for i := 0; i < p.N; i++ {
		v.chunks[i].Stash(v.DBPath, v.dbKey("ch", i))
		v.echos[i].Stash(v.DBPath, v.dbKey("ec", i))
	}
	v.readys = make([]bool, p.N)
	v.requestUnanswered = make([]bool, p.N)

	// if we are supposed to disperse, set payload and associated data to default to nothing
	if v.ID == v.initID {
		v.payload = &EmptyPayload{}
		v.associatedData = &EmptyPayload{}
	}
	return &v
}

// SetPayload sets the dispersed file of the VID instance. It panics if the caller is not the node supposed to inititate the VID.
func (v *VID) SetPayload(p VIDPayload) {
	if v.ID != v.initID {
		panic("cannot set VID payload when not being the init ID")
	}
	v.payload = p
}

// SetAssociatedData sets the broadcasted file of the VID instance. It panics if the caller is not the node supposed to inititate the VID.
func (v *VID) SetAssociatedData(ad VIDPayload) {
	if v.ID != v.initID {
		panic("cannot set VID associated data when not being the init ID")
	}
	v.associatedData = ad
}

// dbKey assembles the key for the database.
func (v *VID) dbKey(tag string, idx int) []byte {
	kbuf := make([]byte, 8)
	binary.BigEndian.PutUint64(kbuf, uint64(idx))
	buf := &bytes.Buffer{}
	buf.Write(v.DBPrefix)
	buf.WriteString(tag)
	buf.Write(kbuf)
	return buf.Bytes()
}
