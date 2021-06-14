package main

import (
	"context"
	"encoding/gob"
	"io"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yangl1996/dispersed-ledger/pika"
	quic "github.com/lucas-clemente/quic-go"
)

var termReported int64 = 0

var numHPMessages int64
var totalHPMessageLatency int64
var ooMux = &sync.Mutex{}
var maxMsgEpochDiff int64
var minMsgEpochDiff int64
var totMsgEpochDiffPos int64
var numMsgEpoch int64
var numLPMessages int64
var totalLPMessageLatency int64
var numHPMessagesProc int64
var totalHPMessageLatencyProc int64
var numLPMessagesProc int64
var totalLPMessageLatencyProc int64

type CountingReader struct {
	io.Reader
	counter *int64
}

func (r *CountingReader) Read(p []byte) (n int, err error) {
	n, err = r.Reader.Read(p)
	atomic.AddInt64(r.counter, int64(n))
	return n, err
}

type peerReceiver struct {
	readCounter *int64
	*gob.Decoder
}

func newPeerReceiver(conn io.Reader, counter *int64) peerReceiver {
	cr := &CountingReader{conn, counter}
	dec := gob.NewDecoder(cr)
	return peerReceiver{
		readCounter: counter,
		Decoder:     dec,
	}
}

type incomingPeerSession struct {
	readCount    *int64
	mMux         *sync.Mutex
	started      bool
	priority     pika.Priority
	OurID        int
	quic.Session                    // we will only accept unidirectional streams into this session
	outgoing     chan<- interface{} // chan back to the server dispatcher
	pMux         *sync.Mutex
	protocol     pika.Protocol
	statechan    chan bool
	incoming     chan pika.Message
}

func newIncomingPeerSession(ourid int, sess quic.Session, outgoing chan<- interface{}, p pika.Protocol, pMux *sync.Mutex, statechan chan bool, incoming chan pika.Message) *incomingPeerSession {
	var c int64
	return &incomingPeerSession{
		readCount: &c,
		mMux:      &sync.Mutex{},
		OurID:     ourid,
		Session:   sess,
		outgoing:  outgoing,
		pMux:      pMux,
		protocol:  p,
		statechan: statechan,
		incoming:  incoming,
	}
}

func (p *incomingPeerSession) HighPriorityReadCount() int64 {
	if p == nil {
		return 0
	}

	p.mMux.Lock()
	defer p.mMux.Unlock()
	if !p.started {
		return 0
	}

	if p.priority == pika.High {
		return atomic.LoadInt64(p.readCount)
	} else {
		return 0
	}
}

func (p *incomingPeerSession) LowPriorityReadCount() int64 {
	if p == nil {
		return 0
	}

	p.mMux.Lock()
	defer p.mMux.Unlock()
	if !p.started {
		return 0
	}

	if p.priority == pika.Low {
		return atomic.LoadInt64(p.readCount)
	} else {
		return 0
	}
}

func (p *incomingPeerSession) listen() error {
	for {
		// get the next stream
		stream, err := p.Session.AcceptUniStream(context.Background())
		if err != nil {
			return err
		}
		conn := newPeerReceiver(stream, p.readCount)
		// start handling the peer
		go func(conn peerReceiver) {
			err := p.receive(conn)
			if err != nil && err != io.EOF {
				log.Fatalln("Incoming peer stream unexpectedly closed:", err)
			}
		}(conn)
	}
}

func (p *incomingPeerSession) receive(c peerReceiver) error {
	for {
		// we can't move this variable (m) out of the loop for god knows reason
		var m *pika.PikaMessage
		err := c.Decode(&m)
		if err != nil {
			return err
		}
		// set the stream type for accounting, if we have not done so
		p.mMux.Lock()
		if !p.started {
			if m.Priority() == pika.Low {
				p.priority = pika.Low
			} else {
				p.priority = pika.High
			}
			p.started = true
		}
		p.mMux.Unlock()

		now := time.Now()
		timediff := now.Sub(m.Sent).Milliseconds()
		if m.Priority() == pika.Low {
			atomic.AddInt64(&numLPMessages, 1)
			atomic.AddInt64(&totalLPMessageLatency, timediff)
			cur := atomic.LoadInt64(&pika.FirstUnconfirmed)
			ooMux.Lock()
			ep := int64(m.Epoch)
			diff := ep - cur
			if diff > 0 && diff > maxMsgEpochDiff {
				maxMsgEpochDiff = diff
				totMsgEpochDiffPos += diff
				numMsgEpoch += 1
			} else if diff < 0 && diff < minMsgEpochDiff {
				minMsgEpochDiff = diff
			}
			ooMux.Unlock()
		} else {
			atomic.AddInt64(&numHPMessages, 1)
			atomic.AddInt64(&totalHPMessageLatency, timediff)
		}

		// process the message
		p.incoming <- m
	}
	return nil
}
