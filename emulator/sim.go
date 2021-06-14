package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"runtime"
	"runtime/pprof"
	"sync"
	"github.com/yangl1996/gopika/pika"
)

var (
	N                   int
	TolerateF           int
	F                   int
	HB                  bool
	Mode                int
	Rounds              int
	PipelineDepth       int
	MaxBandwidth        int
	RandomBandwidth     bool
	RandomBandwidthIntv int
	DummyLoad           bool
	ProbeInterval       int
	MaxBlockSize        int
	PrefillTx           int
	ChainedMode         bool
)

var bwRecord [][]int
var bwActual []int
var bwCapacity []int

func bw(id, time int) int {
	timeSlot := time / RandomBandwidthIntv // changing bandwidth every x ticks, 1 tick = 1ms
	if len(bwRecord[id]) > timeSlot {
		return bwRecord[id][timeSlot]
	} else if len(bwRecord[id]) == timeSlot {
		bwRecord[id] = append(bwRecord[id], rand.Intn(MaxBandwidth)) // max. bw = 2000 Bytes per tick
		return bwRecord[id][timeSlot]
	} else {
		panic("jumping to the next timeslot")
	}
}

func fixedBw(id, time int) int {
	return MaxBandwidth * (id + 1) / N
}

func main() {
	// dispatch subcommands
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "plot":
			DataProcDispatch(os.Args[2:])
			return
		}
	}
	cpuprofile := flag.String("cpuprofile", "", "write CPU profile")
	memprofile := flag.String("memprofile", "", "write memory profile at the end of the simulation")
	logPath := flag.String("log", ".log", "prefix of log files")
	gcinterval := flag.Int("gc", 10000, "run gc every n ticks")
	flag.IntVar(&N, "n", 10, "number of servers (N)")
	flag.IntVar(&TolerateF, "f", 3, "number of faulty servers to tolerate (F)")
	flag.IntVar(&F, "a", 0, "number of actual actual crash fault servers")
	flag.BoolVar(&HB, "hb", false, "simulate HoneyBadgerBFT instead of our protocol")
	flag.IntVar(&Rounds, "e", 500, "number of epochs to simulate")
	flag.IntVar(&PipelineDepth, "p", 0, "max. number of unfinished rounds")
	flag.IntVar(&MaxBandwidth, "bw", 2000, "max. bandwidth in KB/s")
	flag.IntVar(&RandomBandwidthIntv, "bwintv", 1000, "interval of bandwidth change in ms")
	flag.BoolVar(&RandomBandwidth, "r", true, "randomize bandwidth over time")
	flag.BoolVar(&DummyLoad, "dummy", false, "use dummy load to fill blocks")
	flag.IntVar(&ProbeInterval, "pintv", 100, "probe latency every n transactions")
	flag.IntVar(&MaxBlockSize, "b", 10000, "max. block size in bytes")
	flag.IntVar(&PrefillTx, "fill", 0, "prefill the transaction queue with n bytes of transactions")
	flag.BoolVar(&ChainedMode, "c", true, "enabled chained mode (blocks from a node form a chain)")
	distribution := flag.String("dist", "uniform", "set the arrival pattern of new transactions (uniform)")
	newtxratebytes := flag.Float64("rate", 200, "set the rate of new transactions in KB/s in total of all nodes")
	flag.Parse()

	newtxrate := *newtxratebytes / float64(N-F) / 200.0 // convert KB/s to number of txs per tick

	// check the validity of the flags
	if Rounds > 10000 {
		fmt.Println("laptop goes boom boom")
		os.Exit(1)
	}
	if HB && (PipelineDepth != 0) {
		fmt.Println("-p and -hb cannot be used together")
		os.Exit(1)
	}
	if TolerateF*3 >= N {
		fmt.Println("N > 3 x F")
		os.Exit(1)
	}
	if F > TolerateF {
		fmt.Println("number of actual faulty servers greater than F")
		os.Exit(1)
	}
	if (!RandomBandwidth) && (RandomBandwidthIntv != 1000) {
		fmt.Println("-bwintv cannot be used when -r is turned off")
		os.Exit(1)
	}

	if HB {
		Mode = pika.HoneyBadger
		fmt.Println("HoneyBadger mode is currently disabled. Use -p 1 to approximate.")
		os.Exit(1)
	} else {
		Mode = pika.Pipeline
	}

	switch *distribution {
	case "uniform":
		// rationalize a number
		dur := 1.0
		for {
			ntx := dur * newtxrate
			approx := math.Floor(ntx) / dur
			// allow 5% error when rationalizing the number
			if (math.Abs(approx)-newtxrate)/newtxrate < 0.05 {
				if math.Floor(ntx) != 0 {
					UniformInterval = int(math.Floor(dur))
					UniformFrequency = int(math.Floor(ntx))
					break
				}
			}
			dur += 1.0
		}
	}

	// print out simulation config
	configDesc := fmt.Sprintf("N=%v (%v crash fault), F=%v", N, F, TolerateF)
	if Mode == pika.HoneyBadger {
		configDesc += ", HoneyBadgerBFT"
	} else {
		configDesc += ", Pipelined "
		if PipelineDepth == 0 {
			configDesc += " (unlimited stages)"
		} else {
			configDesc += fmt.Sprintf("(%v stages)", PipelineDepth)
		}
	}
	configDesc += fmt.Sprintf(", %v rounds\n", Rounds)
	configDesc += fmt.Sprintf("Max. bandwidth %v KB/s", MaxBandwidth)
	if RandomBandwidth {
		configDesc += fmt.Sprintf(" (randomized every %v ticks)", RandomBandwidthIntv)
	} else {
		configDesc += " (fixed)"
	}
	configDesc += fmt.Sprintf(", Max. block size %v bytes", MaxBlockSize)
	if !ChainedMode {
		configDesc += "\n"
	} else {
		configDesc += " (chained)\n"
	}
	if !DummyLoad {
		configDesc += fmt.Sprintf("New tx arriving at mean rate %v KB/s, queue prefilled to %v bytes when starting\n", *newtxratebytes, PrefillTx)
		configDesc += fmt.Sprintf("Probing confirmation latency every %v txs\n", ProbeInterval)
	} else {
		configDesc += "Blocks filled to max. capacity every time\n"
	}
	fmt.Print(configDesc)

	simConfig := SimulationConfig{
		N:            N,
		F:            F,
		TolerateF:    TolerateF,
		Mode:         Mode,
		LatencyProbe: ProbeInterval != 0,
		DummyLoad:    DummyLoad,
	}
	if *logPath != "" {
		simConfig.Dump(*logPath + "-meta.log")
	}

	// init global vars. we do this after the flags are parsed
	for i := 0; i < N; i++ {
		bwRecord = append(bwRecord, make([]int, 0))
	}
	bwActual = make([]int, N-F)
	bwCapacity = make([]int, N-F)
	servers := make([]*Server, 0)

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

	// make the communication channels
	var hpchans []*FIFO
	var lpchans []*FIFO
	for i := 0; i < N; i++ {
		if i < N-F {
			hpchans = append(hpchans, NewFIFO(256, false))
			lpchans = append(lpchans, NewFIFO(256, false))
		}else {
			hpchans = append(hpchans, NewFIFO(256, true))
			lpchans = append(lpchans, NewFIFO(256, true))
		}
	}
	var allchans []*FIFO
	allchans = append(allchans, hpchans...)
	allchans = append(allchans, lpchans...)

	// make the channel for the progress bar
	progress := make(chan struct{ Id, Pg int }, N*Rounds)
	terminationState := make(chan struct {
		int
		string
	}, N)

	// make the termination signal from the server to this main thread
	term := make(chan int, N)

	// make the wgs that wait for servers and all other goroutines, respectively
	var wg sync.WaitGroup
	var wgAux sync.WaitGroup

	// start the progress bar
	wgAux.Add(1)
	go func() {
		ProgressBar(progress, terminationState, N-F, Rounds-1, 40)
		wgAux.Done()
	}()

	// make the scheduler
	schd := NewScheduler(N-F, *gcinterval, allchans)

	// start the good servers
	for i := 0; i < (N - F); i++ {
		// make the logger
		var logger *log.Logger
		if *logPath == "" {
			logger = nil
		} else {
			path := fmt.Sprintf("%v-%v.log", *logPath, i)
			lf, err := os.Create(path)
			if err != nil {
				log.Fatal("could not create log file: ", err)
			}
			defer lf.Close() // error handling omitted
			buflf := bufio.NewWriter(lf)
			defer buflf.Flush()
			logger = log.New(buflf, "", 0)
		}
		// make the protocolparams
		params := pika.ProtocolParams{
			ID:     i,
			N:      N,
			F:      TolerateF,
			Logger: logger,
		}
		// make the queue
		queue := NewTxQueue(PrefillTx, 200, MaxBlockSize)
		if DummyLoad {
			queue = nil
		}
		// make the protocol
		codec := &fakeErasureCode{N - TolerateF * 2, N}
		p := pika.NewPika(Mode, ChainedMode, Rounds-1, PipelineDepth, queue, &Block{}, codec,progress, params.WithLogPrefix("Pika", 0))
		var bwFunc func(int, int) int
		if RandomBandwidth {
			bwFunc = bw
		} else {
			bwFunc = fixedBw
		}
		// make the server
		var lgen *LoadGenerator
		if !DummyLoad {
			// start the load generator for it
			lgen = &LoadGenerator{
				ID:       i,
				probIntv: ProbeInterval,
				q:        queue,
				txSize:   200,
				rate:     UniformRate,
			}
		}
		s := &Server{
			ID:      i,
			p:       p,
			hpout:   hpchans,
			lpout:   lpchans,
			hpin:    hpchans[i],
			lpin:    lpchans[i],
			feedback: schd.tickFinish,
			loadGen: lgen,
			tsignal:    schd.tsignal,
			tick: &schd.time,
			bw:      bwFunc,
			term:    term,
			termRes: terminationState,
			Logger:  params.Logger,
		}
		servers = append(servers, s)
		wg.Add(1)
		go func(k int) {
			s.Run()
			bwActual[k] = s.msgSize / s.termTime
			bwCapacity[k] = s.bwTot / s.termTime
			wg.Done()
		}(i)
	}

	wgAux.Add(1)
	go func() {
		schd.Run()
		wgAux.Done()
	}()

	termed := 0
	for {
		<-term
		termed += 1
		if termed == N-F {
			break
		}
	}
	schd.shutdown <- true

	wg.Wait()
	close(progress)
	close(terminationState)

	wgAux.Wait()

	// print out the bandwidth usage
	pprof.StopCPUProfile()

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

	anno := make([]string, N-F)

	for i := 0; i < N-F; i++ {
		disp := fmt.Sprintf("Node %v: Avg=%v, Act=%v KB/s", i, bwCapacity[i], bwActual[i])
		anno[i] = disp
	}
	fmt.Println()
	ShowScale(bwCapacity, bwActual, []string{"Average Bandwidth", "Actual Throughput"}, anno, 4, 20, MaxBandwidth)
}
