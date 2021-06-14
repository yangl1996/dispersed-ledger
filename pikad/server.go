package main

import (
	"context"
	"crypto/tls"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eapache/channels"
	quic "github.com/lucas-clemente/quic-go"
	"github.com/yangl1996/gopika/pika"
)

type server struct {
	N               int
	F               int
	ID              int
	incomingPeers   []*incomingPeerSession
	inPeerMux       *sync.Mutex
	outgoingPeers   []*outgoingPeer
	buffer          [][]pika.Message
	newIncomingPeer chan *incomingPeerSession
	newOutgoingPeer chan *outgoingPeer
	sendChan        channels.Channel
	send            chan<- interface{}
	incoming        chan pika.Message
	protocol        pika.Protocol
	pMux            *sync.Mutex
	statechan       chan bool
}

var quicConfig = &quic.Config{
	KeepAlive:             true,
	MaxIdleTimeout:        time.Hour * 72,
	MaxIncomingUniStreams: 0xffffffff,
}

var quicLPConfig = &quic.Config{
	KeepAlive:             true,
	MaxIdleTimeout:        time.Hour * 72,
	MaxIncomingUniStreams: 0xffffffff,
	StrictPrioritization:  true,
}

// serve starts the main server routine
func serve(addr string, N, F, ID int, p pika.Protocol) (*server, error) {
	listener, err := quic.ListenAddr(addr, generateTLSConfig(), quicConfig)
	if err != nil {
		return nil, err
	}
	ch := channels.NewInfiniteChannel()
	server := &server{
		N:               N,
		F:               F,
		ID:              ID,
		incomingPeers:   nil,
		inPeerMux:       &sync.Mutex{},
		outgoingPeers:   make([]*outgoingPeer, N),
		buffer:          make([][]pika.Message, N),
		newIncomingPeer: make(chan *incomingPeerSession, 30),
		newOutgoingPeer: make(chan *outgoingPeer, 30),
		send:            ch.In(),
		sendChan:        ch,
		protocol:        p,
		pMux:            &sync.Mutex{},
		statechan:       make(chan bool, 1),
		incoming:        make(chan pika.Message, 1000),
	}

	go func() {
		server.dispatch()
	}()
	go func() {
		server.acceptPeer(listener)
	}()

	return server, nil
}

func (s *server) IncomingQueueLength() int {
	return len(s.incoming)
}

func (s *server) HighPriorityReadCount() int64 {
	s.inPeerMux.Lock()
	var tot int64
	for _, p := range s.incomingPeers {
		tot += p.HighPriorityReadCount()
	}
	s.inPeerMux.Unlock()
	return tot
}

func (s *server) LowPriorityReadCount() int64 {
	s.inPeerMux.Lock()
	var tot int64
	for _, p := range s.incomingPeers {
		tot += p.LowPriorityReadCount()
	}
	s.inPeerMux.Unlock()
	return tot
}

func (s *server) dispatch() error {
	sendReader := s.sendChan.Out()
	go func() {
		for {
			select {
			case p := <-s.newIncomingPeer:
				go func(np *incomingPeerSession) {
					err := np.listen()
					if err != nil {
						log.Println("Error handling peer:", err)
					}
				}(p)
				s.inPeerMux.Lock()
				s.incomingPeers = append(s.incomingPeers, p)
				s.inPeerMux.Unlock()
			case p := <-s.newOutgoingPeer:
				go func(np *outgoingPeer) {
					np.connect(s.N)
				}(p)
				s.outgoingPeers[p.PeerID] = p
				// drain the buffer
				log.Printf("Sending %v buffered messages to peer %d\n", len(s.buffer[p.PeerID]), p.PeerID)
				for _, v := range s.buffer[p.PeerID] {
					s.outgoingPeers[p.PeerID].transmit <- v
				}
				s.buffer[p.PeerID] = nil
			case ms := <-sendReader:
				// get where this message goes to
				m := ms.(pika.Message)
				dest := m.Dest()
				if s.outgoingPeers[dest] == nil {
					s.buffer[dest] = append(s.buffer[dest], m)
				} else {
					s.outgoingPeers[dest].transmit <- m
				}
			}
		}
	}()

	// start ourself
	go func() {
		initMsgs := initProtocol(s.ID, s.protocol, s.pMux)
		log.Println("Started protocol")
		for _, v := range initMsgs {
			s.send <- v
		}
		ticker := time.Tick(100 * time.Millisecond)
		for _ = range ticker {
			m := &pika.PikaMessage{
				Dummy: true,
			}
			s.pMux.Lock()
			outmsgs, _ := s.protocol.Recv(m)
			s.pMux.Unlock()
			for _, v := range outmsgs {
				s.send <- v
			}
		}
	}()

	go func() {
		for m := range s.incoming {
			// process the message
			s.pMux.Lock()
			firstBatchMsgs, event := s.protocol.Recv(m)
			s.pMux.Unlock()
			msgs, newevent := loopbackMsgs(firstBatchMsgs, s.ID, s.pMux, s.protocol)
			event = event | newevent
			now := time.Now()
			ms := m.(*pika.PikaMessage)
			timediff := now.Sub(ms.Sent).Milliseconds()
			if ms.Priority() == pika.Low {
				atomic.AddInt64(&numLPMessagesProc, 1)
				atomic.AddInt64(&totalLPMessageLatencyProc, timediff)
			} else {
				atomic.AddInt64(&numHPMessagesProc, 1)
				atomic.AddInt64(&totalHPMessageLatencyProc, timediff)
			}

			for _, v := range msgs {
				s.send <- v
			}

			if event&pika.Terminate != 0 {
				output := atomic.CompareAndSwapInt64(&termReported, 0, 1)
				if output {
					s.statechan <- true
				}
			}

		}
	}()

	select {}
}

func (s *server) connectPeer(addr string, id int) error {
	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"quic-echo-example"},
	}
	var hpsession quic.Session
	var lpsession quic.Session
	var signalsession quic.Session
	var err error
	for {
		signalsession, err = quic.DialAddr(HighPriorityConns, addr, tlsConf, quicConfig)
		if err != nil {
			time.Sleep(1 * time.Second)
		} else {
			break
		}
	}
	for {
		hpsession, err = quic.DialAddr(HighPriorityConns, addr, tlsConf, quicConfig)
		if err != nil {
			time.Sleep(1 * time.Second)
		} else {
			break
		}
	}
	for {
		lpsession, err = quic.DialAddr(2, addr, tlsConf, quicLPConfig)
		if err != nil {
			time.Sleep(1 * time.Second)
		} else {
			break
		}
	}
	p := newOutgoingPeer(s.ID, id, signalsession, hpsession, lpsession)
	s.newOutgoingPeer <- p
	return nil
}

// acceptPeer listens to the listener socket and accepts incoming peers
func (s *server) acceptPeer(listener quic.Listener) error {
	for {
		sess, err := listener.Accept(context.Background())
		if err != nil {
			return err
		}
		p := newIncomingPeerSession(s.ID, sess, s.send, s.protocol, s.pMux, s.statechan, s.incoming)
		s.newIncomingPeer <- p
	}
	return nil
}
