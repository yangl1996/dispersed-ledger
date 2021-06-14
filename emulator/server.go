package main

import (
	"fmt"
	"log"
	"runtime"
	"strconv"
	"sync"
	"github.com/dispersed-ledger/dispersed-ledger/pika"
)

type Scheduler struct {
	nServer int
	time     int
	tlock *sync.RWMutex
	tsignal *sync.Cond
	shutdown chan bool	// an external shutdown signal
	gcintv   int
	tickFinish *sync.WaitGroup
	fifos []*FIFO
}

func NewScheduler(n int, gcintv int, fifos []*FIFO) *Scheduler {
	s := &Scheduler{}
	s.nServer = n
	s.tlock = &sync.RWMutex{}
	s.tsignal = sync.NewCond(s.tlock.RLocker())
	s.shutdown = make(chan bool, 1)
	s.tickFinish = &sync.WaitGroup{}
	s.gcintv = gcintv
	s.fifos = fifos
	return s
}

func (s *Scheduler) Run() {
	s.tickFinish.Add(s.nServer)
	// singal every server to init
	s.tlock.Lock()
	s.time += 1
	s.tlock.Unlock()
	s.tsignal.Broadcast()
	// wait for the init
	s.tickFinish.Wait()
	// run the scheduler
	for {
		// deliver the messages
		for _, v := range s.fifos {
			v.Sync()
		}
		// see if we are told to shutdown. if so, close all the ticker channels and return
		select {
		case <-s.shutdown:
			// set the time to -1 so servers know that they should exit
			s.tlock.Lock()
			s.time = -1
			s.tlock.Unlock()
			s.tsignal.Broadcast()
			return
		default:
		}

		// we will wait for them to finish the next tick
		s.tickFinish.Add(s.nServer)

		// send the next ticks
		s.tlock.Lock()
		s.time += 1
		s.tlock.Unlock()
		s.tsignal.Broadcast()

		// wait for the feedbacks
		s.tickFinish.Wait()

		// now no server is running. so whatever we want
		// run gc if told to
		if s.gcintv != 0 && (s.time%s.gcintv == 0) {
			runtime.GC()
		}
	}
}

var UniformInterval int
var UniformFrequency int

func UniformRate(id, time int) int {
	if time%UniformInterval == 0 {
		return UniformFrequency
	} else {
		return 0
	}
}

type LoadGenerator struct {
	ID        int
	probIntv  int // probe every x txs
	q         *TxQueue
	txSize    int
	rate      func(int, int) int // given ID, time, get throughput in terms of txs for this tick
	txCounter int
}

func (l *LoadGenerator) Generate(t int) {
	if l == nil {
		return
	}
	nTxs := l.rate(l.ID, t)
	for i := 0; i < nTxs; i++ {
		if (l.probIntv == 0) || l.txCounter%l.probIntv != 0 {
			l.q.EnqueueTx(l.txSize, false, t)
		} else {
			l.q.EnqueueTx(l.txSize, true, t)
		}
		l.txCounter += 1
	}
}

// we removed medium priority
type Server struct {
	ID      int
	loadGen *LoadGenerator
	feedback *sync.WaitGroup
	p       pika.Protocol
	hpout   []*FIFO
	lpout   []*FIFO
	lpin    *FIFO
	hpin    *FIFO
	nMsg    int
	time    int
	termTime int
	tick    *int
	tsignal *sync.Cond
	msgSize int
	bwTot   int
	bw      func(int, int) int // given ID, time, get throughput in terms of bytes per tick
	term    chan int
	termRes chan struct {
		int
		string
	}
	*log.Logger
	instBwTot    int
	instLPMsgTot int
	instHPMsgTot int
	priortizeLP  bool
}

// dispatchMsg sends messages to other nodes and processes messages that are routed back to the current node
func (s *Server) dispatchMsgs(m []pika.Message) pika.Event {
	e := pika.Event(0)
	msgQueue := m
	for {
		if len(msgQueue) == 0 {
			break
		}
		var v pika.Message
		v, msgQueue = msgQueue[0], msgQueue[1:]
		dst := v.Dest()

		if dst != s.ID {
			switch v.Priority() {
			case pika.High:
				s.hpout[dst].Send(v)
			case pika.Low:
				s.lpout[dst].Send(v)
			}
		} else {
			newMsgs, event := s.p.Recv(v)
			switch {
			case event == 0:
				// do nothing
			case event & pika.Terminate != 0:
				if e == pika.Event(0) {
					e = event
				}
			default:
				// switch priority
				e = event
			}
			msgQueue = append(msgQueue, newMsgs...)
		}
	}
	return pika.Event(e)
}

func (s *Server) processMsg(m pika.Message, terminated bool) pika.Event {
	if !terminated {
		s.nMsg += 1
		s.msgSize += m.Size()
	}
	if m.Priority() == pika.Low {
		s.instLPMsgTot += m.Size()
	} else {
		s.instHPMsgTot += m.Size()
	}
	msgs, e := s.p.Recv(m)
	newE := s.dispatchMsgs(msgs)
	switch {
	case newE == 0:
		// do nothing
	case newE & pika.Terminate != 0:
		if e == pika.Event(0) {
			e = newE
		}
	default:
		// switch priority
		e = newE
	}
	return e
}

func (s *Server) Run() {
	var t int
	if s.Logger != nil {
		s.Logger.SetPrefix("[INIT] ")
	}
	terminated := false
	// wait for the time to change from zero to one
	s.tsignal.L.Lock()
	for *s.tick <= s.time && *s.tick != -1 {
		s.tsignal.Wait()
	}
	s.time = *s.tick
	s.tsignal.L.Unlock()
	// initialize the protocol
	msgs, e := s.p.Init()
	e2 := s.dispatchMsgs(msgs)
	s.feedback.Done()	// signal init is complete

	if e & pika.Terminate != 0 || e2 & pika.Terminate != 0 {
		terminated = true
	}

	remaining := 0      // remaining message to download
	var nextMsg pika.Message // the message that we are downloading
	signalSent := false
	for {
		s.tsignal.L.Lock()
		for *s.tick <= s.time && *s.tick != -1 {
			s.tsignal.Wait()
		}
		newtime := *s.tick
		s.tsignal.L.Unlock()
		if newtime != -1 {
			s.time = newtime
			t = s.time
		} else {
			break
		}
		if terminated {
			if !signalSent {
				s.termTime = s.time
				s.term <- s.ID
				signalSent = true
				s.termRes <- struct {
					int
					string
				}{s.ID, fmt.Sprintf("Node %v: terminating after %v ticks, %v msgs processed (%v Bytes), %v KB/s", s.ID, s.time, s.nMsg, s.msgSize, s.msgSize/s.time)}
			}
		}
		// proceed to the next time tick
		// if we have terminated, stop updating time, bandwidth, and data gauges
		if !terminated {
			s.time = t
			s.loadGen.Generate(t)
		}
		if s.Logger != nil {
			s.Logger.SetPrefix("[" + strconv.Itoa(t) + "] ")
		}
		bwCredit := s.bw(s.ID, t)
		s.instBwTot += bwCredit
		if !terminated {
			s.bwTot += bwCredit
		}
		for {
			if bwCredit <= 0 {
				break
			}
			if remaining > 0 {
				// if we are downloading the previous message
				if bwCredit >= remaining {
					// can finish downloading
					bwCredit -= remaining
					remaining = 0
					e = s.processMsg(nextMsg, terminated)
					//fmt.Println("Node", s.ID, "processing msg at time", s.time)
					if e & pika.Terminate != 0 {
						terminated = true
					}
					if e & pika.BlockedByLowPriorityMessage != 0 {
						s.priortizeLP = true
					} else if e & pika.BlockedByHighPriorityMessage != 0 {
						s.priortizeLP = false
					}
				} else {
					// cannot finish downloading
					remaining -= bwCredit
					bwCredit = 0
				}
			} else {
				// if no message to download, peek the queue
				if !s.priortizeLP {
					nm, there := s.hpin.Recv()
					if there {
						nextMsg = nm
						remaining = nextMsg.Size()
					} else {
						nm, there = s.lpin.Recv()
						if there {
							nextMsg = nm
							remaining = nextMsg.Size()
						} else {
							remaining = 0
							bwCredit = 0
							break
						}
					}
				} else {
					nm, there := s.lpin.Recv()
					if there {
						nextMsg = nm
						remaining = nextMsg.Size()
					} else {
						nm, there = s.hpin.Recv()
						if there {
							nextMsg = nm
							remaining = nextMsg.Size()
						} else {
							remaining = 0
							bwCredit = 0
							break
						}
					}
				}
			}
		}
		if (t % 250) == 249 {
			s.Logger.Printf("last 250 ticks bandwidth provision %v bytes, lp %v bytes, hp %v bytes\n", s.instBwTot, s.instLPMsgTot, s.instHPMsgTot)
			s.instBwTot = 0
			s.instLPMsgTot = 0
			s.instHPMsgTot = 0
		}
		// send the feedback
		s.feedback.Done()
	}
	s.Println("exiting")
}

