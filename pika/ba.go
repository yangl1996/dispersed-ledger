package pika

import (
	"bytes"
	"encoding/binary"
)

// valid values of BAEpochMessage.Type
const (
	viable = iota // Viable means that we know the value is viable. This is sent for our initial estimation, or when we
	// received F+1 Viables.
	agreement // Agreement means that we know that all nodes will eventually know the value is available. We send
	// Agreement upon receiving 2F+1 Viables.
)

// BAEpoch is an epoch of BA.
type BAEpoch struct {
	rcvdVTrue      []bool // true if the node has sent us Viable(true)
	nVTrue         int    // number of Viable(true) we have received
	rcvdVFalse     []bool // true if the node has sent us Viable(false)
	nVFalse        int    // number of Viable(false) we have received
	rcvdA          []bool // whether we have received agreement from this node
	valueA         []bool // the value of agreement that we received from this node
	sentVTrue      bool   // if we have sent out Viable(true)
	sentVFalse     bool   // if we have sent out Viable(false)
	sentA          bool   // if we have sent out Agreement
	believeTrue    bool   // if we believe in true (we believe in true after receiving 2F+1 Viable(true))
	believeFalse   bool   // if we believe in false
	e              bool   // our initial estimation at the beginning of this epoch
	candidateTrue  bool   // Whether we think True is a candidate output of the BA.
	candidateFalse bool   // Whether we think False is a candidate output of the BA.
	termed         bool   // If this BAEpoch has terminated.
	ProtocolParams
}

// BAEpochMessage is a message emitted and handled by a BAEpoch.
type BAEpochMessage struct {
	Type      int  // type of the message represented as an integer
	Value     bool // value of the message
	DestID    int  // the node this message is going to
	FromID    int  // the sending node of the message
	Signature []byte
}

// Dest returns the destination of the message.
func (m BAEpochMessage) Dest() int {
	return m.DestID
}

// From returns the source of the message.
func (m BAEpochMessage) From() int {
	return m.FromID
}

// Priority always returns High. All BAEpochMessage instances are of high priority.
func (m BAEpochMessage) Priority() Priority {
	return High
}

// Size always returns 8. It is the size used in the emulator.
func (m BAEpochMessage) Size() int {
	return 8
}

// broadcast constructs a message for each node with the given type and value, and returns them in a slice.
func (b *BAEpoch) broadcast(mtype int, value bool) []Message {
	var msgs []Message

	for i := 0; i < b.N; i++ {
		msg := &BAEpochMessage{}
		msg.Type = mtype
		msg.Value = value
		msg.DestID = i
		msg.FromID = b.ID
		if mtype == agreement {
			msg.Signature = bytes.Repeat([]byte("a"), 64)
		}
		msgs = append(msgs, msg)
	}
	return msgs
}

// Terminated returns whether the BAEpoch is terminated. A BAEpoch is terminated when we have settled on a set of
// candidate values.
func (b *BAEpoch) Terminated() bool {
	return b.termed
}

// handleViable takes a Viable message with the given source and value, and updates the internal state of the BAEpoch.
// It returns a slice of messages to be sent out. It ignores the message if the BAEpochCoreState is not there.
func (b *BAEpoch) handleViable(from int, value bool) []Message {
	var msgs []Message
	if value {
		// record that we have received from this server
		if !b.rcvdVTrue[from] {
			b.rcvdVTrue[from] = true
			b.nVTrue += 1
		}
		// if we have not sent Viable(true), and we have received F+1 Viable(true), send it
		if (!b.sentVTrue) && (b.nVTrue >= (b.F + 1)) {
			b.sentVTrue = true
			msgs = append(msgs, b.broadcast(viable, true)...)
			b.Println("sending Viable(true) due to enough viable")
		}

		// if we have received 2F+1 Viable(true), we believe in true
		if b.nVTrue >= (b.F*2 + 1) {
			b.believeTrue = true
			// if we have not sent out Agreement, send out Agreement(true)
			if !b.sentA {
				b.sentA = true
				msgs = append(msgs, b.broadcast(agreement, true)...)
				b.Println("sending Agreement(true) due to enough viable")
			}
		}
	} else {
		// record that we have received from this server
		if !b.rcvdVFalse[from] {
			b.rcvdVFalse[from] = true
			b.nVFalse += 1
		}
		// if we have not sent Viable(false), and we have received F+1 Viable(false), send it
		if (!b.sentVFalse) && (b.nVFalse >= (b.F + 1)) {
			b.sentVFalse = true
			msgs = append(msgs, b.broadcast(viable, false)...)
			b.Println("sending Viable(false) due to enough viable")
		}

		// if we have received 2F+1 Viable(false), we believe in false
		if b.nVFalse >= (b.F*2 + 1) {
			b.believeFalse = true
			// if we have not sent out Agreement, send out Agreement(false)
			if !b.sentA {
				b.sentA = true
				msgs = append(msgs, b.broadcast(agreement, false)...)
				b.Println("sending Agreement(false) due to enough viable")
			}
		}
	}
	return msgs
}

// handleAgreement takes an Agreement message with the given source and value, and updates the internal state of the
// BAEpoch. It always returns nil. It ignores the message if the BAEpochCoreState is not there.
func (b *BAEpoch) handleAgreement(from int, value bool) []Message {
	if !b.rcvdA[from] {
		b.Printf("receiving Agreement(%v) from node %v\n", value, from)
		b.rcvdA[from] = true
		b.valueA[from] = value
	}
	return nil
}

// NewBAEpoch constructs a new instance of BAEpoch.
func NewBAEpoch(p ProtocolParams) *BAEpoch {
	b := &BAEpoch{}
	b.rcvdVTrue = make([]bool, p.N)
	b.rcvdVFalse = make([]bool, p.N)
	b.rcvdA = make([]bool, p.N)
	b.valueA = make([]bool, p.N)
	b.ProtocolParams = p
	b.ProtocolParams.Logger = nil
	return b
}

// SetEstimation sets the initial estimation of the BAEpoch. It is a nop if BAEpochCoreState is nil.
func (b *BAEpoch) SetEstimation(e bool) {
	b.e = e
}

// Init sends out our initial estimation of the BAEpoch. It is a nop if the BAEpoch is already terminated,
// if the BAEpochCoreState is nil, or if our initial estimation has been sent in a Viable message (due to
// receiving F+1 Viable messages). It always returns 0 as the Event.
func (b *BAEpoch) Init() ([]Message, Event) {
	if b.termed {
		// if we have terminated, don't bother
		return nil, 0
	}
	if (b.e && b.sentVTrue) || (!b.e && b.sentVFalse) {
		// if we have sent viable, don't send it again
		return nil, 0
	}
	// send out Viable(b.e) as our initial estimation
	msgs := b.broadcast(viable, b.e)
	b.Printf("sending Viable(%v) as initial estimate\n", b.e)
	if b.e {
		b.sentVTrue = true
	} else {
		b.sentVFalse = true
	}
	return msgs, 0
}

// Recv handles an incoming message to the BAEpoch. It ignores the message if the BAEpoch instance is terminated,
// or if the BAEpochCoreState is nil. It returns a slice of messages to be sent and an integer representing the
// results of the exeuction.
func (b *BAEpoch) Recv(mg Message) ([]Message, Event) {
	m := mg.(*BAEpochMessage)

	// if this epoch has terminated, ignore the message
	if b.termed {
		return nil, 0
	}

	var msgs []Message
	switch m.Type {
	case viable:
		msgs = b.handleViable(m.FromID, m.Value)
	case agreement:
		msgs = b.handleAgreement(m.FromID, m.Value)
	default:
		// unreachable
		panic("invalid BAEpoch message type")
	}

	// see if we can terminate. we can terminate when we get N agreements whose values
	// have been believed by us
	na := 0 // the number of such Agreement messages
	// scan through all Agreement messages
	for from, got := range b.rcvdA {
		// if we have received Agreement(x) and believe in x, then x is a candidate
		if got {
			v := b.valueA[from]
			if v && b.believeTrue {
				b.candidateTrue = true
				na += 1
			}
			if (!v) && b.believeFalse {
				b.candidateFalse = true
				na += 1
			}
		}
	}
	// when we get N-F such values, we can terminate
	if na >= b.N-b.F && !b.termed {
		// log the candidates
		cs := ""
		if b.candidateTrue {
			cs += "(true)"
		}
		if b.candidateFalse {
			cs += "(false)"
		}
		b.Println("terminating with candidates:", cs)
		b.termed = true
		return msgs, Terminate
	} else {
		return msgs, 0
	}

}

// baTermStatus includes the information related to the termination of the BA protocol on a node.
// Termination of a node at an epoch is considered as Viable and Agreement from that node for all
// future epochs.
type baTermStatus struct {
	NodeID int  // the node terminating the BA
	Epoch  int  // the epoch when this node terminates
	Value  bool // the output of the BA on that node
}

// MarshalBinary implements encoding.BinaryMarshaler. It encodes the baTermStatus into binary.
func (b *baTermStatus) MarshalBinary() ([]byte, error) {
	buf := &bytes.Buffer{}
	err := binary.Write(buf, binary.BigEndian, int64(b.NodeID))
	if err != nil {
		return nil, err
	}
	err = binary.Write(buf, binary.BigEndian, int64(b.Epoch))
	if err != nil {
		return nil, err
	}
	err = binary.Write(buf, binary.BigEndian, b.Value)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler. It decodes the binary into the given instance of baTermStatus.
func (b *baTermStatus) UnmarshalBinary(data []byte) error {
	buf := bytes.NewBuffer(data)

	var nid int64
	err := binary.Read(buf, binary.BigEndian, &nid)
	if err != nil {
		return err
	}
	b.NodeID = int(nid)
	var eid int64
	err = binary.Read(buf, binary.BigEndian, &eid)
	if err != nil {
		return err
	}
	b.Epoch = int(eid)
	err = binary.Read(buf, binary.BigEndian, &b.Value)
	if err != nil {
		return err
	}
	return nil
}

// BACoreState includes the core execution state of a BA.
type BACoreState struct {
	input         bool            // the initial input to the protocol
	epochNum      int             // index of the first epoch that has not terminated
	firstUninited int             // index of the first epoch that we have not sent our estimation
	termStatus    []*baTermStatus // the termination status of each node, nil if the node is not yet terminated
	epochs        []*BAEpoch      // epochs of the BA
}

// BAOutput contains the output of a BA.
type BAOutput struct {
	output bool // the output of the protocol
	termed bool // if we have terminated
}

// An instance of Binary Agreement (BA).
type BA struct {
	ProtocolParams
	*BACoreState
	*BAOutput
}

// valid values for BAMessage.Type
const (
	epoch = iota
	term
)

// BAMessage is the message emitted and handled by a BA instance.
type BAMessage struct {
	Type     int             // the type of the message encoded as an integer, it is either epoch or term
	Epoch    int             // the epoch this message is associated to
	Value    bool            // the value if this message is a Termination message
	EpochMsg *BAEpochMessage // the BAEpochMessage if this message is an Epoch message
	FromID   int             // the source of the message
	DestID   int             // the destination of the message
}

// Dest returns the destination of the message.
func (m BAMessage) Dest() int {
	return m.DestID
}

// From returns the source of the message.
func (m BAMessage) From() int {
	return m.FromID
}

// Size always returns 16. This is the size we assume in the emulator.
func (m BAMessage) Size() int {
	return 8 + 8
}

// Priority always returns High. All BA messages are treated as of high priority.
func (m BAMessage) Priority() Priority {
	return High
}

// Terminated returns whether the BA is terminated.
func (b *BA) Terminated() bool {
	return b.termed
}

// Output returns the output of the BA instance. It panics when the BA is not terminated.
func (b *BA) Output() bool {
	if !b.Terminated() {
		panic("accessing BA output when it is not terminated")
	}
	return b.output
}

// sendEpochMsg takes an epoch number and a slice of *BAEpochMessage, wraps them inside BAMessage, and returns the result.
func (b *BA) sendEpochMsg(e int, out []Message) []Message {
	var msgs []Message
	for _, v := range out {
		m := v.(*BAEpochMessage)
		msg := &BAMessage{}
		msg.Type = epoch
		msg.Epoch = e
		msg.EpochMsg = m
		msg.FromID = m.FromID
		msg.DestID = m.DestID
		msg.Value = false
		msgs = append(msgs, msg)
	}
	return msgs
}

// broadcastTerm constructs Termination messages with the given epoch number and value towards each node.
func (b *BA) broadcastTerm(e int, val bool) []Message {
	var msgs []Message
	for i := 0; i < b.N; i++ {
		msg := &BAMessage{}
		msg.Type = term
		msg.Epoch = e
		msg.EpochMsg = nil
		msg.Value = val
		msg.FromID = b.ID
		msg.DestID = i
		msgs = append(msgs, msg)
	}
	return msgs
}

// NewBA creates a new instance of BA.
func NewBA(p ProtocolParams) *BA {
	ba := &BA{
		BACoreState:    &BACoreState{},
		BAOutput:       &BAOutput{},
		ProtocolParams: p,
	}
	ba.termStatus = make([]*baTermStatus, p.N)
	ba.epochs = append(ba.epochs, NewBAEpoch(ba.ProtocolParams.WithPrefix("BAEpoch", 0)))
	// no Term messages from previous epochs to apply - because there is no previous epoch!
	return ba
}

// SetInput sets the input of the BA. It is a nop if BACoreState is nil.
func (b *BA) SetInput(e bool) {
	if b.BACoreState != nil {
		b.input = e
	}
}

// Init initializes epoch 0 with our input. It is a nop if the BA instance has terminated or if BACoreState is nil.
func (b *BA) Init() ([]Message, Event) {
	var msgs []Message
	// don't bother if we have terminated
	if b.termed || b.BACoreState == nil {
		return msgs, 0
	}

	// set the est of the first epoch to be our input
	// NOTE that we only do this when the first epoch is unterminated TODO this logic, while I think to be correct, is messy
	if b.firstUninited == 0 {
		b.epochs[0].SetEstimation(b.input)

		// initialize the first epoch
		outMsgs, _ := b.epochs[0].Init() // no need to care about if epoch 0 has terminated, since epoch.Init() returns event 0 anyway
		b.firstUninited = 1
		msgs = b.sendEpochMsg(0, outMsgs)
	}
	return msgs, 0
}

// checkOutput checks the given epoch to see whether we can terminate the whole BA. If so, it returns (output, true).
// Otherwise, it returns (estimation for next epoch, false).
func (b *BA) checkOutput(e int) (bool, bool) {
	// BUG(leiy): We are not tossing the coin the BA in the correct way.
	coin := (e % 2) == 0
	// track how many candidates we have got
	numC := 0
	if b.epochs[e].candidateTrue {
		numC += 1
	}
	if b.epochs[e].candidateFalse {
		numC += 1
	}
	if numC == 2 {
		// if this epoch terminates with 2 candidates, set est = coin and we can't terminate
		return coin, false
	} else if numC == 1 {
		// if this epoch terminates with 1 candidate
		candidate := b.epochs[e].candidateTrue // the candidate we have chosen
		if candidate == coin {
			// if candidate == coin, terminate
			return candidate, true
		} else {
			// otherwise, set est = candidate
			return candidate, false
		}
	} else {
		panic("trying to check output when there is no candidate")
	}
}

// allocateEpochs constructs BAEpoch until (and including) the given epoch. It returns the new messages during this process.
func (b *BA) allocateEpochs(until int) []Message {
	var msgs []Message
	// BUG(leiy): Others can easily DDoS the system by sending a message with a very big epoch number.
	for i := len(b.epochs); i <= until; i++ {
		b.epochs = append(b.epochs, NewBAEpoch(b.ProtocolParams.WithPrefix("BAEpoch", i)))

		// apply the termination message from earlier epochs to this new epoch
		for _, tstatus := range b.termStatus {
			if tstatus != nil && tstatus.Epoch < i {
				// simulate Viable and Agreement messages
				nmsgs, _ := b.insertTermMessage(tstatus, i) // we don't care about the output of the epoch, because we will check later
				msgs = append(msgs, nmsgs...)
			}
		}
	}
	return msgs
}

// Recv handles an incoming message of *BAMessage type. It returns the messages to be sent out and the execution result.
// It is a nop if the BA is already terminated, or if BACoreState is nil.
func (b *BA) Recv(mg Message) ([]Message, Event) {
	m := mg.(*BAMessage)
	var msgs []Message

	// if BA has already terminated, ignore the message
	if b.termed || b.BACoreState == nil {
		return msgs, 0
	}

	// make sure we have allocated until epoch m.Epoch
	msgs = append(msgs, b.allocateEpochs(m.Epoch)...)

	// process the message according to its type
	switch m.Type {
	case epoch:
		// pass to the corresponding epoch, we don't really care about the output because we will check later anyway
		outM, _ := b.epochs[m.Epoch].Recv(m.EpochMsg)
		msgs = append(msgs, b.sendEpochMsg(m.Epoch, outM)...)
	case term:
		// record the termination only if we have not got a termination from that node before
		if b.termStatus[m.FromID] == nil {
			tStatus := &baTermStatus{
				NodeID: m.FromID,
				Value:  m.Value,
				Epoch:  m.Epoch,
			}
			b.termStatus[m.FromID] = tStatus
			// apply the message to all future epochs
			for eidx := m.Epoch + 1; eidx < len(b.epochs); eidx++ {
				nmsgs, _ := b.insertTermMessage(tStatus, eidx)
				msgs = append(msgs, nmsgs...)
			}
		}
	default:
		// unreachable
		panic("invalid BA message type")
	}

	// check from the first unterminated epoch until we either hit an unterminated epoch, or we can terminate the while BA
	for i := b.epochNum; i < len(b.epochs); i++ {
		if b.epochs[i].Terminated() {
			// if the epoch has terminated, check if we can terminate the whole protocol
			est, termBA := b.checkOutput(i)
			if termBA {
				// if we can terminate now
				b.output = est
				b.termed = true
				msgs = append(msgs, b.broadcastTerm(i, est)...) // send out term messages
				b.Printf("reached agreement %v\n", b.output)
				b.BACoreState = nil // clear the BACoreState now that we have terminated
				return msgs, Terminate
			} else {
				// otherwise, carry the estimation into the next epoch
				// make sure we have allocated until epoch i+1
				msgs = append(msgs, b.allocateEpochs(i+1)...)
				// set the estimation for epoch i+1
				b.epochs[i+1].SetEstimation(est)
				// we don't init epoch i+1 yet - epoch i+1 will be check in the loop next
			}
		} else {
			// if we hit an epoch that has not terminated, and we have not terminated the whole BA

			// update the first unterminated epoch
			b.epochNum = i // we are at epoch i and it is unterminated
			// Then we try to initialize some epochs. Technically we can initialize all epochs until and including i.
			// Because epoch 0 to i-1 are all terminated, so epoch 0 to i should have their estimations properly set.
			// However, we don't bother initializing epoch 0 to i-1, because they have already terminated and our input
			// will not affect the output. (TODO: is it true?) Instead, we just initialize epoch i.

			// We can't initialize epoch 0 here. Epoch 0 must be kicked off by the user, because at this point maybe the
			// user have not set an estimation!
			if i >= b.firstUninited && i != 0 {
				messages, _ := b.epochs[i].Init() // no point checking for status because BAEpoch.Init() only returns Progress
				b.firstUninited = i + 1
				msgs = append(msgs, b.sendEpochMsg(i, messages)...)
				b.Printf("proceeding to epoch %v\n", b.epochNum)
			}
			return msgs, 0
		}
	}
	// unreachable
	panic("non-continuous BA epochs")
}

// insertTermMessage applies the baTermStatus on a later epoch. It does this by simulate sending a Viable and an Agreement
// message to the specified epoch. It returns the messages to be sent (already wrapped into *BAMessage) and the execution
// result of the BAEpoch. It panics when trying to apply a baTermStatus on an epoch that is not later than the baTermStatus
// itself.
func (b *BA) insertTermMessage(m *baTermStatus, eidx int) ([]Message, Event) {
	if m.Epoch >= eidx {
		panic("calling insertTermMessage for an epoch not later than the message")
	}
	var msgs []Message
	var res Event
	vmsg := &BAEpochMessage{
		Type:   viable,
		Value:  m.Value,
		FromID: m.NodeID,
		DestID: b.ID,
	}
	amsg := &BAEpochMessage{
		Type:   agreement,
		Value:  m.Value,
		FromID: m.NodeID,
		DestID: b.ID,
	}
	nmsgs, ne := b.epochs[eidx].Recv(vmsg)
	msgs = append(msgs, b.sendEpochMsg(eidx, nmsgs)...)
	res = res | ne
	nmsgs, ne = b.epochs[eidx].Recv(amsg)
	msgs = append(msgs, b.sendEpochMsg(eidx, nmsgs)...)
	res = res | ne
	return msgs, res
}
