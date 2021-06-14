package main

import (
	"encoding/gob"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/yangl1996/dispersed-ledger/pika"
)

var NoRetrieve bool
var HighPriorityConns int = 10
var NumPipeline int
var MaxStreams int
var OurNodeID int
var NumNodes int
var LatencyLogPath string
var LatencyOtherLogPath string
var CancelChunkResponse bool

var ConfirmedBytes int64

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	// register the message types to gob
	gob.Register(&ReedSolomonChunk{})
	gob.Register(&Block{})
	gob.Register(&pika.PikaBlock{})
	gob.Register(&pika.EmptyPayload{})
	gob.Register(&pika.InMessageChunk{})

	gcpercentage := flag.Int("gogc", 100, "trigger GC when heap is 1+(gogc/100) times the size of reachable objects")
	cpuprofile := flag.String("cpuprofile", "", "write CPU profile")
	memprofile := flag.String("memprofile", "", "write memory profile")
	showversion := flag.Bool("v", false, "print the version")
	listenaddr := flag.String("s", "127.0.0.1:7452", "sets the address to listen to")
	N := flag.Int("n", 16, "number of servers in the cluster")
	F := flag.Int("f", 5, "number of faulty servers to tolerate")
	peerlist := flag.String("nodes", "", "list of peers in ID/ADDR format split by commas")
	peerlistfile := flag.String("nodefile", "", "path to a list of peers in ID/ADDR format split by newlines")
	blocksize := flag.Int("b", 3000000, "max size of a block")
	ID := flag.Int("id", 0, "ID of this server")
	epochs := flag.Int("e", 10, "number of epochs to run")
	pipeline := flag.Int("p", 1, "number of pipeline stages, 0 for unlimited")
	parallelism := flag.Int("q", 1, "number of parallel dispersions, 0 for unlimited")
	inmemory := flag.Bool("mem", false, "do not use the disk to store blocks")
	noretrieve := flag.Bool("noretrieve", false, "disable retrieval for debugging")
	hpconnsflag := flag.Int("conns", 30, "set the number of connections for high-priority traffic")
	profileServer := flag.Bool("profserver", false, "start the profile server at port 8888")
	chainedmode := flag.Bool("chained", true, "use chained mode")
	maxstreams := flag.Int("streams", 200, "number of streams in each connection")
	txrate := flag.Float64("txrate", 0, "rate of arriving transactions to this server")
	latencylog := flag.String("latencylog", "/pika/latency.dat", "file to log latency")
	peerlatencylog := flag.String("peerlatencylog", "/pika/peerlatency.dat", "file to log peer latency")
	cancellation := flag.Bool("cancelreq", true, "enable chunk request cancellation")
	nagleSize := flag.Int("naglesize", 150, "size threshold of the nagle's algorithm in KB")
	nagleDelay := flag.Int("nagledelay", 100, "delay threshold of the nagle's algorithm in ms")
	coupledValidity := flag.Bool("coupled", false, "enforce coupled validity")
	infbacklog := flag.Bool("infbacklog", false, "infinite backlog for measuring throughput (latency probes will be disabled)")

	flag.Parse()

	CancelChunkResponse = *cancellation
	NoRetrieve = *noretrieve
	HighPriorityConns = *hpconnsflag
	MaxStreams = *maxstreams
	OurNodeID = *ID
	NumNodes = *N
	LatencyLogPath = *latencylog
	LatencyOtherLogPath = *peerlatencylog
	queuedHPMsgs = make([]int64, *N)
	queuedLPMsgs = make([]int64, *N)
	pika.NagleSize = *nagleSize * 1000 / 250
	pika.NagleDelay = *nagleDelay

	if *showversion {
		fmt.Println("pikad v0.0")
		return
	}

	debug.SetGCPercent(*gcpercentage)

	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal("could not create CPU profile: ", err)
		}
		defer f.Close() // error handling omitted
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal("could not start CPU profile: ", err)
		}
	}

	if *profileServer {
		go func() {
			runtime.SetMutexProfileFraction(100)
			log.Println(http.ListenAndServe("0.0.0.0:8888", nil))
		}()
	}

	if *F == 0 {
		log.Fatalln("F must be greater than 0")
	}
	if *N < *F*3+1 {
		log.Fatalln("N must be greater or equal to 3F+1")
	}

	if *peerlist == "" && *peerlistfile == "" {
		log.Fatalln("Missing both peerlist and peerlist file")
	}
	if *peerlist != "" && *peerlistfile != "" {
		log.Fatalln("-p and -pf cannot be set at the same time")
	}

	// parse the peer information
	peers := []struct {
		id   int
		addr string
	}{}
	var peerliststr string
	var peerStrs []string
	if *peerlistfile != "" {
		fc, err := ioutil.ReadFile(*peerlistfile)
		if err != nil {
			log.Fatalln("Error reading node list file")
		}
		peerliststr = string(fc)
		for _, v := range strings.Split(strings.TrimSuffix(peerliststr, "\n"), "\n") {
			if v != "" {
				peerStrs = append(peerStrs, v)
			}
		}

		fmt.Println(peerStrs)
	}
	if *peerlist != "" {
		peerliststr = *peerlist
		peerStrs = strings.Split(peerliststr, ",")
	}
	for _, c := range peerStrs {
		peerInfo := strings.Split(c, "/")
		pid, err := strconv.Atoi(peerInfo[0])
		if err != nil {
			log.Fatalln("Error parsing peer ID:", err)
		}
		peers = append(peers, struct {
			id   int
			addr string
		}{pid, peerInfo[1]})
	}

	if len(peers) != *N {
		log.Fatalln("Number of specified nodes is not equal to N")
	}

	dbpath := ""
	var db pika.KVStore
	if !*inmemory {
		dbpath = fmt.Sprintf("/pika/%d.pikadb", *ID)
		// TODO: we are deleting DB file
		os.RemoveAll(dbpath)
		var err error
		db, err = NewPogrebStore(dbpath)
		if err != nil {
			log.Fatalln("Error creating database:", err)
		}
	}
	q := NewBlockGenerator(*blocksize, *infbacklog)
	codec := NewReedSolomonCode(*N-2*(*F), *N)
	params := pika.ProtocolParams{
		ID:     *ID,
		N:      *N,
		F:      *F,
		Logger: log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds),
		DBPath: db,
	}.WithPrefix("Pika", 0)
	protocol := pika.NewPika(*chainedmode, *epochs, *pipeline, *parallelism, q, codec, nil, *coupledValidity, params)

	// start generating transactions
	go func() {
		for {
			nextDelay := rand.ExpFloat64() / *txrate
			nsSleep := nextDelay * 1000000000
			delay := time.Duration(nsSleep) * time.Nanosecond
			time.Sleep(delay)
			q.InsertTransaction()
		}
	}()
	go AccountLatency()

	server, err := serve(*listenaddr, *N, *F, *ID, protocol)
	if err != nil {
		log.Fatalln("Error starting server:", err)
	}

	for _, c := range peers {
		// don't connect to ourself
		if c.id == *ID {
			continue
		}
		go func(addr string, id int) {
			for {
				err := server.connectPeer(addr, id)
				if err != nil {
					log.Println("Error connecting to peer, will retry in 1 second:", err)
					time.Sleep(1 * time.Second)
				} else {
					break
				}
			}
		}(c.addr, c.id)
	}

	// start reporting memory and bandwidth
	go func() {
		var ms runtime.MemStats
		printstats := func() {
			runtime.ReadMemStats(&ms)
			log.Printf("OS Mem Usage: %v Bytes, Heap: %v Bytes\n", ms.Sys, ms.HeapAlloc)
			hpcount := server.HighPriorityReadCount()
			lpcount := server.LowPriorityReadCount()
			log.Printf("HP Read: %v Bytes, LP Read: %v Bytes, Confirm: %v Bytes\n", hpcount, lpcount, atomic.LoadInt64(&ConfirmedBytes))
			outstrcount := atomic.LoadInt64(&activeOutStreams)
			log.Printf("Active outgoing streams: %v\n", outstrcount)

			hpqueue := make([]int64, 0)
			var hpqueuetot int64
			var lpqueuetot int64
			lpqueue := make([]int64, 0)
			for i := 0; i < *N; i++ {
				hp := atomic.LoadInt64(&queuedHPMsgs[i])
				lp := atomic.LoadInt64(&queuedLPMsgs[i])
				hpqueue = append(hpqueue, hp)
				lpqueue = append(lpqueue, lp)
				hpqueuetot += hp
				lpqueuetot += lp
			}
			txqueue := q.QueueLength()
			log.Printf("Queue HP: %d, LP: %d, Tx: %d, In: %d\n", hpqueuetot, lpqueuetot, txqueue, server.IncomingQueueLength())
			log.Printf("Per-node Queue HP: %d, LP: %d, Tx: %d, In: %d\n", hpqueue, lpqueue, txqueue, server.IncomingQueueLength())

			LatencyLock.Lock()
			totalTx := ConfirmedTx
			totalLatency := TotalLatencyMs
			LatencyLock.Unlock()
			log.Printf("Confirmation Latency: %v txs, %v ms\n", totalTx, totalLatency)

			ooMux.Lock()
			maxDiff := maxMsgEpochDiff
			maxMsgEpochDiff = 0
			minDiff := minMsgEpochDiff
			minMsgEpochDiff = 0
			totDiff := totMsgEpochDiffPos
			numDiff := numMsgEpoch
			totMsgEpochDiffPos = 0
			numMsgEpoch = 0
			ooMux.Unlock()
			var avgDiff int
			if numDiff != 0 {
				avgDiff = int(float64(totDiff) / float64(numDiff))
			}
			log.Printf("Message delay: %v HP, %v ms, %v LP, %v ms, %v HP, %v ms, %v LP, %v ms, %v posdiff, %v negdiff, %v avgpos\n", atomic.LoadInt64(&numHPMessages), atomic.LoadInt64(&totalHPMessageLatency), atomic.LoadInt64(&numLPMessages), atomic.LoadInt64(&totalLPMessageLatency), atomic.LoadInt64(&numHPMessagesProc), atomic.LoadInt64(&totalHPMessageLatencyProc), atomic.LoadInt64(&numLPMessagesProc), atomic.LoadInt64(&totalLPMessageLatencyProc), maxDiff, minDiff, avgDiff)

			// print out logs
			func() {
				pika.CanceledReqMux.Lock()
				defer pika.CanceledReqMux.Unlock()
				if len(pika.FinishedEpochs) != 0 {
					log.Printf("Node progress: %v\n", pika.FinishedEpochs)
				}
			}()

		}
		printstats()
		c := time.Tick(1 * time.Second)
		for _ = range c {
			printstats()
		}
	}()

	// wait for completion
	<-server.statechan
	if *cpuprofile != "" {
		pprof.StopCPUProfile()
	}

	if *memprofile != "" {
		f, err := os.Create(*memprofile)
		if err != nil {
			log.Fatal("could not create memory profile: ", err)
		}
		defer f.Close() // error handling omitted for example
		runtime.GC()    // get up-to-date statistics
		if err := pprof.WriteHeapProfile(f); err != nil {
			log.Fatal("could not write memory profile: ", err)
		}
	}
	hpcount := server.HighPriorityReadCount()
	lpcount := server.LowPriorityReadCount()
	log.Printf("Experiment terminated, %v bytes read (%v HP, %v LP)\n", hpcount+lpcount, hpcount, lpcount)
	select {}
}
