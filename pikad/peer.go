package main

import (
	"sync"

	"github.com/yangl1996/dispersed-ledger/pika"
)

func loopbackMsgs(m []pika.Message, ourid int, pmux *sync.Mutex, protocol pika.Protocol) ([]pika.Message, pika.Event) {
	e := pika.Event(0)
	msgQueue := m
	var outQueue []pika.Message
	for len(msgQueue) != 0 {
		var v pika.Message
		v, msgQueue = msgQueue[0], msgQueue[1:]
		dst := v.Dest()

		if dst != ourid {
			outQueue = append(outQueue, v)
		} else {
			pmux.Lock()
			newMsgs, event := protocol.Recv(v)
			pmux.Unlock()
			e = e | event
			msgQueue = append(msgQueue, newMsgs...)
		}
	}
	return outQueue, e
}

func initProtocol(ourid int, protocol pika.Protocol, pMux *sync.Mutex) []pika.Message {
	pMux.Lock()
	msgs, _ := protocol.Init()
	pMux.Unlock()
	// TODO: not worrying about the event
	newmsgs, _ := loopbackMsgs(msgs, ourid, pMux, protocol)
	return newmsgs
}
