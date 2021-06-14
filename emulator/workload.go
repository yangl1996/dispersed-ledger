package main

import (
	"sync"
	"strings"
	"fmt"
	"strconv"
	"github.com/yangl1996/dispersed-ledger/pika"
)

var bufPool = sync.Pool {
	New: func() interface{} {
		return new(strings.Builder)
	},
}

// Tx represents a transaction in the emulator. It stores the timestamp when it is created to help
// trace the confirmation latency.
type Tx struct {
	pos   int
	t     int
	size  int
	probe bool
}

// Block represents a block in the emulator.
type Block struct {
	size       int
	timestamps []int
}

func (b *Block) New() pika.PikaPayload {
	return &Block{}
}

func (b *Block) MarshalBinary() ([]byte, error) {
	panic("dummy block should not be serialized")
}

func (b *Block) UnmarshalBinary(data []byte) error {
	panic("dummy block should not be deserialized")
}

func (b *Block) Size() int {
	return b.size
}

func (b *Block) LogPropose() string {
	return fmt.Sprintf("proposing %d bytes (%d latency probes)")
}

func (b *Block) LogConfirm() string {
	buf := bufPool.Get().(*strings.Builder)
	buf.Reset()
	for _, v := range b.timestamps {
		buf.WriteByte(' ')
		buf.WriteString(strconv.Itoa(v))
	}
	res := buf.String()
	bufPool.Put(buf)
	return res
}

// fakeErasureCode does not actually encode or decode, but simply produces "shards" that have pointer
// to the input object.
type fakeErasureCode struct {
	d int	// number of data shards
	p int	// number of data + check shards
}

type fakeErasureCodeChunk struct {
	size int
	fullData pika.VIDPayload
}

func (c *fakeErasureCodeChunk) Size() int {
	return c.size
}

func (f *fakeErasureCode) Encode(input pika.VIDPayload) ([]pika.ErasureCodeChunk, error) {
	output := make([]pika.ErasureCodeChunk, f.p)
	sizerInput := input.(pika.Sizer)
	chunkSize := sizerInput.Size() / f.d	// get the size of a data chunk
	for i := 0; i < f.p; i++ {
		output[i] = &fakeErasureCodeChunk {
			size: chunkSize,
			fullData: input,
		}
	}
	return output, nil
}

func (f *fakeErasureCode) Decode(shards []pika.ErasureCodeChunk, v pika.VIDPayload) error {
	c := shards[0].(*fakeErasureCodeChunk)
	v.(Cloneable).CloneFrom(c.fullData)
	return nil
}

type Cloneable interface {
	CloneFrom(s pika.VIDPayload)
}

type TxQueue struct {
	*sync.Mutex
	enqueued int // number of bytes enqueued
	dequeued int // number of bytes dequeued
	txs      []Tx
	stopped  bool
	maxBlockSize int
}

func NewTxQueue(size int, txsize int, maxBlockSize int) *TxQueue {
	n := size / txsize
	q := &TxQueue{
		Mutex: &sync.Mutex{},
		maxBlockSize: maxBlockSize,
	}
	for i := 0; i < n; i++ {
		q.EnqueueTx(txsize, false, 0)
	}
	return q
}

func (q *TxQueue) Stop() {
	q.Lock()
	q.stopped = true
	q.Unlock()
}

func (q *TxQueue) Length() int {
	q.Lock()
	res := q.enqueued - q.dequeued
	q.Unlock()
	return res
}

func (q *TxQueue) EnqueueTx(s int, probe bool, t int) {
	q.Lock()
	defer q.Unlock()
	if q.stopped {
		return
	}
	tx := Tx{
		pos:   q.enqueued,
		t:     t,
		size:  s,
		probe: probe,
	}
	q.txs = append(q.txs, tx)
	q.enqueued += s
}

func (q *TxQueue) takeBlockUpTo(s int) (*Block, int) {
	q.Lock()
	blockSize := 0
	nTx := 0
	var timestamps []int
	for _, v := range q.txs {
		if blockSize+v.size <= s {
			blockSize += v.size
			nTx += 1
			if v.probe {
				timestamps = append(timestamps, v.t)
			}
		} else {
			break
		}
	}

	// remove stuff from the block
	q.dequeued += blockSize
	q.txs = q.txs[nTx:]
	q.Unlock()
	return &Block{
		size:       blockSize,
		timestamps: timestamps,
	}, q.dequeued
}

func (q *TxQueue) TakeBlock() pika.PikaPayload {
	res, _ := q.takeBlockUpTo(q.maxBlockSize)
	return res
}

func (q *TxQueue) FillBlock(p pika.PikaPayload) pika.PikaPayload {
	s := q.maxBlockSize
	b := p.(*Block)
	if b.size >= s {
		return b
	}
	remain := s - b.size
	newb, _ := q.takeBlockUpTo(remain)
	newb.timestamps = append(b.timestamps, newb.timestamps...)
	newb.size += b.size
	return newb
}

func (q *TxQueue) LogQueueStatus() string {
	return fmt.Sprintf("transaction queue length %v", q.Length())
}
