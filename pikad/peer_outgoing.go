package main

import (
	"bufio"
	"container/heap"
	"encoding/gob"

	//"io"
	"github.com/eapache/channels"
	quic "github.com/lucas-clemente/quic-go"
	"github.com/yangl1996/gopika/pika"
	"log"
	"sync/atomic"
)

type peerSender struct {
	quic.SendStream
	bufwriter *bufio.Writer
	*gob.Encoder
}

func newPeerSender(conn quic.SendStream) peerSender {
	bufwriter := bufio.NewWriter(conn)
	enc := gob.NewEncoder(bufwriter)
	return peerSender{conn, bufwriter, enc}
}

var activeOutStreams int64
var queuedHPMsgs []int64
var queuedLPMsgs []int64

func (p peerSender) handle(ch <-chan interface{}, highPriority bool) {
	atomic.AddInt64(&activeOutStreams, 1)
	for ms := range ch {
		m := ms.(pika.Message)
		if highPriority {
			atomic.AddInt64(&queuedHPMsgs[m.Dest()], -1)
		} else {
			atomic.AddInt64(&queuedLPMsgs[m.Dest()], -1)
		}
		if !highPriority {
			pm := m.(*pika.PikaMessage)
			ep := pm.Epoch
			msg := pm.Msg
			idx := msg.Idx
			if msg.VIDMsg != nil {
				if msg.VIDMsg.RespondChunk {
					pika.CanceledReqMux.Lock()
					var can bool
					if len(pika.CanceledReq) <= ep {
						can = false
					} else {
						can = pika.CanceledReq[ep][idx][msg.Dest()]
					}
					pika.CanceledReqMux.Unlock()

					if CancelChunkResponse && can {
						continue
					}
				}
			}
		}
		err := p.Encode(m)
		if err != nil {
			log.Fatalln(err)
		}
		p.bufwriter.Flush()
	}
	p.SendStream.Close()
	atomic.AddInt64(&activeOutStreams, -1)
}

type outgoingPeer struct {
	OurID         int
	PeerID        int
	hpSession     quic.Session
	lpSession     quic.Session
	signalSession quic.Session
	channel       channels.Channel
	transmit      chan<- interface{} // chan to collect messages to be sent to this server
}

func newOutgoingPeer(ourid, peerid int, signalsess, hpsess, lpsess quic.Session) *outgoingPeer {
	chn := channels.NewInfiniteChannel()
	peer := &outgoingPeer{
		OurID:         ourid,
		PeerID:        peerid,
		signalSession: signalsess,
		hpSession:     hpsess,
		lpSession:     lpsess,
		channel:       chn,
		transmit:      chn.In(),
	}
	return peer
}

// An IntHeap is a min-heap of ints.
type IntHeap []int

func (h IntHeap) Len() int           { return len(h) }
func (h IntHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h IntHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *IntHeap) Push(x interface{}) {
	// Push and Pop use pointer receivers because they modify the slice's length,
	// not just its contents.
	*h = append(*h, x.(int))
}

func (h *IntHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

func (p *outgoingPeer) connect(n int) {
	// handle the streams
	hpchan := channels.NewInfiniteChannel()
	signalChan := channels.NewInfiniteChannel()
	lpchan := channels.NewInfiniteChannel()

	createStream := func(sess quic.Session, highprio bool) (quic.SendStream, channels.Channel) {
		str, err := sess.OpenUniStream()
		if err != nil {
			panic(err)
		}
		sender := newPeerSender(str)
		ch := channels.NewInfiniteChannel()
		go sender.handle(ch.Out(), highprio)
		return str, ch
	}

	handleEpochPriority := func(dispatch <-chan interface{}, sess quic.Session, highprio bool, id int) {
		chans := make(map[int]channels.Channel)
		chIns := make(map[int]chan<- interface{})
		activeChans := &IntHeap{}
		for m := range dispatch {
			msg := m.(*pika.PikaMessage)
			ep := msg.Epoch
			_, there := chans[ep]
			if !there {
				str, ch := createStream(sess, highprio)
				str.SetWeight(ep)
				chans[ep] = ch
				chIns[ep] = ch.In()
				heap.Push(activeChans, ep)
			}
			chIns[ep] <- m
			for activeChans.Len() > MaxStreams {
				torm := heap.Pop(activeChans).(int)
				chans[torm].Close()
				delete(chans, torm)
				delete(chIns, torm)
			}

			/*
				disp := int(atomic.LoadInt64(&pika.NumContDispersed))
				pika.CanceledReqMux.Lock()
				var nconn int
				if len(pika.FinishedEpochs) == 0 {
					nconn = 1
				} else {
					ret := pika.FinishedEpochs[id]
					if disp <= ret {
						nconn = 1
					} else {
						nconn = disp - ret
					}
				}
				pika.CanceledReqMux.Unlock()
				nconn = nconn * nconn
				if nconn > HighPriorityConns {
					nconn = HighPriorityConns
				}
			*/

			/*
				var nconn int
				var prog int
				pika.CanceledReqMux.Lock()
				if len(pika.FinishedEpochs) == 0 {
					prog = -1
				} else {
					prog = pika.FinishedEpochs[id]
				}
				pika.CanceledReqMux.Unlock()
				if prog != -1 {
					bott := pika.BottleneckEpochs(5)
					if prog > bott {
						nconn = 1
					} else {
						nconn = 15
					}
				} else {
					nconn = 1
				}
			*/

			//sess.SetNumConnections(nconn)
		}
	}

	handleNoPriority := func(dispatch <-chan interface{}, sess quic.Session, highprio bool) {
		str, err := sess.OpenUniStream()
		if err != nil {
			panic(err)
		}
		sender := newPeerSender(str)
		go sender.handle(dispatch, highprio)
	}

	go handleNoPriority(hpchan.Out(), p.hpSession, true)
	go handleNoPriority(signalChan.Out(), p.signalSession, true)
	go handleEpochPriority(lpchan.Out(), p.lpSession, false, p.PeerID)

	// sort the messages
	ch := p.channel.Out()
	signalinchan := signalChan.In()
	hpinchan := hpchan.In()
	lpinchan := lpchan.In()
	for mi := range ch {
		m := mi.(pika.Message)
		// TODO: not handling priority switch here
		switch m.Priority() {
		case pika.High:
			atomic.AddInt64(&queuedHPMsgs[m.Dest()], 1)
			signalinchan <- m
		case pika.Medium:
			atomic.AddInt64(&queuedHPMsgs[m.Dest()], 1)
			hpinchan <- m
		case pika.Low:
			if !NoRetrieve {
				atomic.AddInt64(&queuedLPMsgs[m.Dest()], 1)
				lpinchan <- m
			}
		}
	}
}
