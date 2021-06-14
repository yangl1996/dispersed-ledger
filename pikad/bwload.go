package main

import (
	"bytes"
	"fmt"
	"hash/adler32"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yangl1996/gopika/pika"
)

const TxSize int = 250

type Block struct {
	DummyData  []byte
	TimeStamps []time.Time
}

func (b *Block) Size() int {
	return len(b.DummyData)
}

func (b *Block) LogPropose() string {
	return fmt.Sprintf("%v bytes (%v)", len(b.DummyData), adler32.Checksum(b.DummyData))
}

func (b *Block) LogConfirm(i int) string {
	atomic.AddInt64(&ConfirmedBytes, int64(len(b.DummyData)))
	BlockChannel <- struct {
		*Block
		int
	}{b, i}
	return fmt.Sprintf(" %v bytes (%v)", len(b.DummyData), adler32.Checksum(b.DummyData))
}

type BlockGenerator struct {
	*sync.Mutex
	queue   []time.Time
	size    int
	stopped bool
	infBacklog bool
}

func NewBlockGenerator(size int, infBacklog bool) *BlockGenerator {
	return &BlockGenerator{
		Mutex: &sync.Mutex{},
		queue: make([]time.Time, 0),
		size:  size,
		infBacklog: infBacklog,
	}
}

func (q *BlockGenerator) QueueLength() int {
	q.Lock()
	defer q.Unlock()
	return len(q.queue)
}

func (q *BlockGenerator) InsertTransaction() {
	if q.infBacklog {
		return
	}
	t := time.Now()
	q.Lock()
	defer q.Unlock()
	if q.stopped {
		return
	}
	q.queue = append(q.queue, t)
}

func (q *BlockGenerator) dequeueTimes(n int) []time.Time {
	maxTxs := n
	q.Lock()
	defer q.Unlock()
	take := 0
	if maxTxs > len(q.queue) {
		take = len(q.queue)
	} else {
		take = maxTxs
	}

	times := make([]time.Time, take)
	copy(times, q.queue[:take])
	// delete the existing timestamps
	q.queue = q.queue[take:]
	return times
}

func (q *BlockGenerator) EmptyBlock() pika.PikaPayload {
	return &Block{
		TimeStamps: []time.Time{},
		DummyData:  []byte{},
	}
}

func (q *BlockGenerator) TakeBlock() pika.PikaPayload {
	if q.infBacklog {
		dummyData := bytes.Repeat([]byte{'A'}, q.size)

		return &Block{
			TimeStamps: []time.Time{},
			DummyData:  dummyData,
		}
	} else {
		m := q.size / TxSize
		ts := q.dequeueTimes(m)

		// add some dummy data: time.Time marshals into 15 bytes
		dummyData := bytes.Repeat([]byte{'A'}, len(ts)*(TxSize-15))

		return &Block{
			TimeStamps: ts,
			DummyData:  dummyData,
		}
	}
}

func (q *BlockGenerator) FillBlock(b pika.PikaPayload) pika.PikaPayload {
	if q.infBacklog {
		dummyData := bytes.Repeat([]byte{'A'}, q.size)

		return &Block{
			TimeStamps: []time.Time{},
			DummyData:  dummyData,
		}
	} else {
		m := q.size / TxSize
		block := b.(*Block)
		existing := len(block.TimeStamps)
		toFill := m - existing
		if toFill <= 0 {
			return block
		}
		ts := q.dequeueTimes(toFill)
		block.TimeStamps = append(block.TimeStamps, ts...)
		block.DummyData = bytes.Repeat([]byte{'A'}, len(block.TimeStamps)*(TxSize-15))
		return block
	}
}

func (q *BlockGenerator) LogQueueStatus() string {
	return "queue normal"
}

func (q *BlockGenerator) Stop() {
	q.Lock()
	q.stopped = true
	q.Unlock()
	return
}
