package main

import (
	"sync/atomic"
	"sync"
	"github.com/dispersed-ledger/dispersed-ledger/pika"
)

// FIFO implements a FIFO queue for network simulation. It supports Send and Recv. Send may be called concurrently,
// but Recv must not (i.e. it is MPSC). Multiple Send and one Recv may be concurrently invoked.
// A message inserted using Send will only be delivered after Sync is called. When Sync is
// being called, no Send or Recv may be called at the same time.
//		[ TX Buffer  |  RX Buffer  |  TX Buffer ]
//					  ^ rxBufStart  ^ txBufStart
type FIFO struct {
	*sync.RWMutex
	// these fields are protected by the lock, they should only be modified when the Writer lock is aquired
	txBufStart int64	// the first offset that we should write to in this timeslot
	rxBufStart int64	// the first offset that we should read in this timeslot
	txBufSize int64	// the number of tx slots from txBufStart until rxBufStart
	bufLen int64
	buf []pika.Message // the main message ring buffer

	// these fields are only modified in Sync call
	rxBufSize int64	// the number of messages to be read

	// these fields are atomic counters
	nRx int64	// the number of messages that we have received in this timeslot
	nRxSuccess int64
	nTx int64	// the number of messages that we have sent in this timeslot

	// these fields are const
	crashed bool
}

func NewFIFO(bufsz int64, crash bool) *FIFO {
	return &FIFO {
		RWMutex: &sync.RWMutex{},
		txBufSize: bufsz,
		bufLen: bufsz,
		buf: make([]pika.Message, bufsz),
		crashed: crash,
	}
}

func (p *FIFO) Recv() (pika.Message, bool) {
	// if we have read every message before this read
	if p.rxBufSize <= p.nRx {
		return nil, false
	}

	// calculate after this read, how many messages are received
	nRead := atomic.AddInt64(&p.nRx, 1)

	// if after this read, more than rxBufSize messages will be received, we should return that the buffer is drained
	if nRead > p.rxBufSize {
		return nil, false
	}

	// acquire the lock for read, so no writer can move the buffer
	p.RLock()
	idx := p.rxBufStart + nRead - 1
	if idx >= p.bufLen {
		idx -= p.bufLen
	}
	msg := p.buf[idx]
	p.RUnlock()

	// return the message and increase the counter for success read messages
	atomic.AddInt64(&p.nRxSuccess, 1)
	if msg == nil {
		panic("returning nil")
	}
	return msg, true
}

func (p *FIFO) Send(m pika.Message) {
	if p.crashed {
		return
	}
	// number of messages sent after this call
	nSend := atomic.AddInt64(&p.nTx, 1)

	// first acquire the reader lock and see if we need to resize the buffer
	p.RLock()
	// if after this write, less or equal to txBufSize messages have been inserted, we are fine
	if nSend <= p.txBufSize {
		// no need to allocate
		idx := p.txBufStart + nSend - 1
		if idx >= p.bufLen {
			idx -= p.bufLen
		}
		p.buf[idx] = m
		p.RUnlock()
		return
	} else {
		// we have run out of the buffer. we may need to allocate some more
		// first release the reader lock. we need to get a writer lock
		p.RUnlock()
		p.Lock()
		// someone else may have allocated the buffer for us between RUnlock and Lock. check again
		if nSend <= p.txBufSize {
			// it appears that the buffer size is enough now
			idx := p.txBufStart + nSend - 1
			if idx >= p.bufLen {
				idx -= p.bufLen
			}
			p.buf[idx] = m
			p.Unlock()
			return
		} else {
			// we still need to allocate
			// to allocate, we insert a slice after the current end of tx buffer (right before rx buffer)
			insertAfter := p.txBufStart + p.txBufSize
			if insertAfter >= p.bufLen {
				insertAfter -= p.bufLen
			}
			// decide how many to allocate: we at least fulfill the number of messages to be sent plus a headroom of 4,
			// or the current buffer size
			nAllocated := nSend - p.txBufSize + 4
			if nAllocated < p.bufLen {
				nAllocated = p.bufLen
			}
			// see go wiki / slicetricks
			p.buf = append(p.buf[:insertAfter], append(make([]pika.Message, nAllocated), p.buf[insertAfter:]...)...)
			// update the starting points of the buffers
			p.rxBufStart += nAllocated
			// don't add if txbufSize == 0!
			if insertAfter <= p.txBufStart && p.txBufSize != 0{
				p.txBufStart += nAllocated
			}
			p.txBufSize += nAllocated
			p.bufLen += nAllocated
			// finally, insert our new message
			idx := p.txBufStart + nSend - 1
			if idx >= p.bufLen {
				idx -= p.bufLen
			}
			p.buf[idx] = m
			p.Unlock()
			return
		}
	}
}

func (p *FIFO) Sync() {
	// get the numebr of read and written messages
	received := p.nRxSuccess
	sent := p.nTx

	// reset the counters
	p.nRx = 0
	p.nTx = 0
	p.nRxSuccess = 0

	// the number of receivable messages is decreased by received an increased by sent
	p.rxBufSize += (sent - received)

	// move rx buffer further by received
	p.rxBufStart += received
	if p.rxBufStart >= p.bufLen {
		p.rxBufStart -= p.bufLen
	}

	// move tx buffer start to be right after the last unreceived message
	p.txBufStart = p.rxBufStart + p.rxBufSize
	if p.txBufStart >= p.bufLen {
		p.txBufStart -= p.bufLen
	}

	// finally, set the tx buffer size to be the size unused by rx buffer
	p.txBufSize = p.bufLen - p.rxBufSize
}
