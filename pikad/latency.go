package main

import (
	"encoding/gob"
	"github.com/golang/snappy"
	"log"
	"os"
	"sync"
	"time"
)

var ConfirmedTx int64 = 0
var TotalLatencyMs int64 = 0
var LatencyLock = &sync.Mutex{}

var BlockChannel = make(chan struct {
	*Block
	int
}, 200)

func AccountLatency() {
	f, err := os.Create(LatencyLogPath)
	if err != nil {
		log.Fatalln("unable to create latency log: ", err)
	}
	defer f.Close()
	w := snappy.NewBufferedWriter(f)
	enc := gob.NewEncoder(w)

	fOther, err := os.Create(LatencyOtherLogPath)
	if err != nil {
		log.Fatalln("unable to create latency log: ", err)
	}
	defer fOther.Close()
	wOther := snappy.NewBufferedWriter(fOther)
	encOther := gob.NewEncoder(wOther)

	for b := range BlockChannel {
		now := time.Now()
		var durMs int64
		for _, t := range b.Block.TimeStamps {
			delay := now.Sub(t)
			ms := delay.Milliseconds()
			durMs += ms
			if b.int == OurNodeID {
				enc.Encode(ms)
			} else {
				encOther.Encode(ms)
			}
		}
		if b.int == OurNodeID {
			LatencyLock.Lock()
			TotalLatencyMs += durMs
			ConfirmedTx += int64(len(b.Block.TimeStamps))
			LatencyLock.Unlock()
			w.Flush()
		} else {
			wOther.Flush()
		}
	}
}
