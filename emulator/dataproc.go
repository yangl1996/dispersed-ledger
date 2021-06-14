package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"github.com/dispersed-ledger/dispersed-ledger/pika"
)

var dpLogger = log.New(os.Stderr, "", 0)
var m = sync.Mutex{}

type SimulationConfig struct {
	N            int
	F            int
	TolerateF    int
	Mode         int
	LatencyProbe bool
	DummyLoad    bool
}

func (c SimulationConfig) Dump(outpath string) {
	of, err := os.Create(outpath)
	if err != nil {
		dpLogger.Println("could not create output file:", err)
		os.Exit(1)
	}
	defer of.Close()

	m, err := json.MarshalIndent(c, "", "    ")
	if err != nil {
		dpLogger.Println("unable to encode JSON:", err)
	}
	of.Write(m)
}

func ParseConfig(inpath string) SimulationConfig {
	var c SimulationConfig
	f, err := ioutil.ReadFile(inpath)
	if err != nil {
		dpLogger.Println("could not read log file:", err)
		os.Exit(1)
	}
	json.Unmarshal(f, &c)
	return c
}

func GetGnuplot() string {
	p, e := exec.LookPath("gnuplot")
	if e != nil {
		dpLogger.Println("could not find gnuplot, did you install it? err:", e)
		os.Exit(1)
	}
	return p
}

func RunGnuplot(tpl string, tplParams interface{}) {
	tmpl, err := template.New("g").Parse(tpl)
	if err != nil {
		dpLogger.Println("failed to compile template:", err)
		os.Exit(1)
	}
	// start gnuplot
	proc := exec.Command(GetGnuplot(), "-")
	stdin, err := proc.StdinPipe()
	if err != nil {
		dpLogger.Println("failed to connect to stdin of gnuplot:", err)
		os.Exit(1)
	}
	proc.Start()
	tmpl.Execute(stdin, tplParams)
	stdin.Close()
	out, err := proc.CombinedOutput()
	err = proc.Wait()
	if err != nil {
		dpLogger.Println("error running gnuplot:", err)
		dpLogger.Println("gnuplot says:")
		dpLogger.Println(out)
		os.Exit(1)
	}
}

func PlotTermination(c SimulationConfig, n int, dataPrefix string, outPath string) {
	const TerminationPlotTemplate = `set term pdf truecolor enhanced
set output "{{ .OutputPath }}"
set key right bottom
set xlabel "Time (ms)"
set ylabel "Epochs"
set title "Epochs Finished"
plot for [i=0:{{ .ActiveNodesMaxIdx }}] "{{ .InputPrefix }}-term.dat" using 1:2 index i with lines lc i notitle, \{{if .PlotDispersion}}
for [i=0:{{ .ActiveNodesMaxIdx }}] "{{ .InputPrefix }}-hpterm.dat" using 1:2 index i with lines dashtype 2 lc i notitle, \{{end}}
NaN with lines dt 1 lc 1 title "Full Block"{{if .PlotDispersion}}, \
NaN with lines dt 2 lc 1 title "Dispersion"{{end}}`
	params := struct {
		OutputPath        string
		InputPrefix       string
		ActiveNodesMaxIdx int
		PlotDispersion    bool
	}{outPath, dataPrefix, n - 1, c.Mode != pika.HoneyBadger}
	RunGnuplot(TerminationPlotTemplate, params)
	dpLogger.Println("Epoch termination time plotted at", outPath)
}

func PlotConfirmDist(n int, dataPrefix string, outPath string) {
	const TerminationPlotTemplate = `set term pdf truecolor enhanced
set output "{{ .OutputPath }}"
set key right bottom
set xlabel "Confirmation Latency (ms)"
set ylabel "Fraction of Transactions (%)"
set title "CDF of Confirmation Latency"
plot for [i=0:{{ .ActiveNodesMaxIdx }}] "{{ .InputPrefix }}-confirm.dat" using 2:1 index i with lines lc i notitle`
	params := struct {
		OutputPath        string
		InputPrefix       string
		ActiveNodesMaxIdx int
	}{outPath, dataPrefix, n - 1}
	RunGnuplot(TerminationPlotTemplate, params)
	dpLogger.Println("Confirmation latency distribution plotted at", outPath)
}

func PlotTxQueueLength(n int, dataPrefix string, outPath string) {
	const TerminationPlotTemplate = `set term pdf truecolor enhanced
set output "{{ .OutputPath }}"
set key right bottom
set xlabel "Time (ms)"
set ylabel "Size (bytes)"
set title "Transaction Queue Size"
plot for [i=0:{{ .ActiveNodesMaxIdx }}] "{{ .InputPrefix }}-qlen.dat" using 1:2 index i with lines lc i notitle`
	params := struct {
		OutputPath        string
		InputPrefix       string
		ActiveNodesMaxIdx int
	}{outPath, dataPrefix, n - 1}
	RunGnuplot(TerminationPlotTemplate, params)
	dpLogger.Println("Transaction queue length plotted at", outPath)
}

func PlotBandwidth(n int, dataPrefix string, outPath string) {
	const TerminationPlotTemplate = `set term pdf truecolor enhanced
set output "{{ .OutputPath }}"
set key right top 
set xlabel "Time (ms)"
set ylabel "Rate (KB/s)"
set title "Bandwidth"
plot for [i=0:{{ .ActiveNodesMaxIdx }}] "{{ .InputPrefix }}-bw.dat" using 1:2 index i with lines lc i notitle, \
for [i=0:{{ .ActiveNodesMaxIdx }}] "{{ .InputPrefix }}-bw.dat" using 1:3 index i with lines dashtype 2 lc i notitle, \
for [i=0:{{ .ActiveNodesMaxIdx }}] "{{ .InputPrefix }}-bw.dat" using 1:4 index i with lines dashtype 5 lc i notitle, \
NaN with lines dt 1 lc 1 title "Provision", \
NaN with lines dt 2 lc 1 title "Low Priority", \
NaN with lines dt 5 lc 1 title "High Priority"`

	params := struct {
		OutputPath        string
		InputPrefix       string
		ActiveNodesMaxIdx int
	}{outPath, dataPrefix, n - 1}
	RunGnuplot(TerminationPlotTemplate, params)
	dpLogger.Println("Bandwidth plotted at", outPath)
}

func DataProcDispatch(args []string) {
	command := flag.NewFlagSet("plot", flag.ExitOnError)
	input := command.String("p", ".log", "prefix of the log files")
	nodesopt := command.String("n", "all", "which nodes to plot (all or comma-delimited indices, e.g. 0,1,2")
	output := command.String("o", "plot", "prefix of the output plots")
	keepfiles := command.Bool("work", false, "keep intermediate files")
	calcStats := command.Bool("stats", false, "print out stats of the experiment")
	plotTerm := command.Bool("term", true, "plot the termination time")
	plotConfirmDist := command.Bool("confirmdist", true, "plot the confirmation time distribution")
	plotQueueLength := command.Bool("qlen", true, "plot the transaction queue length over time")
	plotBandwidth := command.Bool("bw", true, "plot bandwidth provision and usage over time")

	command.Parse(args)

	c := ParseConfig(*input + "-meta.log")
	var nodes []int
	if *nodesopt == "all" {
		for i := 0; i < c.N-c.F; i++ {
			nodes = append(nodes, i)
		}
	} else {
		nlist := strings.Split(*nodesopt, ",")
		for _, v := range nlist {
			nidx, _ := strconv.Atoi(v)
			nodes = append(nodes, nidx)
		}
	}

	var wg sync.WaitGroup

	if *plotTerm {
		wg.Add(1)
		go func() {
			if c.Mode != pika.HoneyBadger {
				FilterPikaTermination(*input, nodes, true, *input+"-hpterm.dat", *calcStats)
			}
			FilterPikaTermination(*input, nodes, false, *input+"-term.dat", *calcStats)
			PlotTermination(c, len(nodes), *input, *output+"-term.pdf")
			defer wg.Done()
		}()
	}

	if *plotConfirmDist {
		wg.Add(1)
		go func() {
			if (!c.LatencyProbe) || c.DummyLoad {
				dpLogger.Println("Skipping confirmation latency distribution because no latency probes were installed")
			} else {
				FilterConfirmation(*input, nodes, *input+"-confirm.dat", *calcStats)
				PlotConfirmDist(len(nodes), *input, *output+"-confirm.pdf")
			}
			defer wg.Done()
		}()
	}

	if *plotQueueLength {
		wg.Add(1)
		go func() {
			if c.DummyLoad {
				dpLogger.Println("Skipping transaction queue length because dummy load were used")
			} else {
				FilterTxQueueLength(*input, nodes, *input+"-qlen.dat")
				PlotTxQueueLength(len(nodes), *input, *output+"-qlen.pdf")
			}
			defer wg.Done()
		}()
	}

	if *plotBandwidth {
		wg.Add(1)
		go func() {
			FilterBandwidth(*input, nodes, *input+"-bw.dat")
			PlotBandwidth(len(nodes), *input, *output+"-bw.pdf")
			defer wg.Done()
		}()
	}

	wg.Wait()

	// clean up
	if !*keepfiles {
		os.Remove(*input + "-hpterm.dat")
		os.Remove(*input + "-term.dat")
		os.Remove(*input + "-confirm.dat")
		os.Remove(*input + "-qlen.dat")
		os.Remove(*input + "-bw.dat")
	}
}

func ScanLogFile(logPath string, lineProc func(l string)) {
	f, err := os.Open(logPath)
	if err != nil {
		dpLogger.Println("could not open log file:", err)
		os.Exit(1)
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	s.Buffer(nil, 8*1024*1024) // support larger files
	for s.Scan() {
		l := s.Text()
		lineProc(l)
	}

	if err := s.Err(); err != nil {
		dpLogger.Println("error reading log file:", err)
		os.Exit(1)
	}
}

func FilterPikaTermination(prefix string, n []int, hpterm bool, outpath string, calc bool) {
	of, err := os.Create(outpath)
	if err != nil {
		dpLogger.Println("could not create output file:", err)
		os.Exit(1)
	}
	defer of.Close()

	var tr *regexp.Regexp
	if !hpterm {
		tr = regexp.MustCompile(`^\[(\d+)\] \/Pika0\/PikaEpoch(\d+) terminating while accepting (\d+) blocks$`)
	} else {
		tr = regexp.MustCompile(`^\[(\d+)\] \/Pika0\/PikaEpoch(\d+) high-priority terminating while accepting (\d+) blocks$`)
	}

	avgTime := 0.0

	for idx, i := range n {
		p := fmt.Sprintf("%s-%d.log", prefix, i)
		of.WriteString(fmt.Sprintf("\"Node %v\"\n", i))
		lastTime := 0.0
		nEpochs := 0

		proc := func(l string) {
			m := tr.FindStringSubmatch(l)
			if len(m) != 0 {
				t, _ := strconv.Atoi(m[1])
				r, _ := strconv.Atoi(m[2])
				of.WriteString(fmt.Sprintln(t, r))

				if calc && !hpterm {
					nEpochs += 1
					lastTime = float64(t)
				}
			}
		}
		ScanLogFile(p, proc)
		if calc && !hpterm {
			avgTime += lastTime / float64(nEpochs)
		}

		if idx != len(n)-1 {
			of.WriteString("\n\n")
		}
	}
	if calc && !hpterm {
		avgTime /= float64(len(n))
		m.Lock()
		fmt.Printf("Average epoch time: %v\n", int(avgTime))
		m.Unlock()
	}
}

func FilterConfirmation(prefix string, n []int, outpath string, calc bool) {
	of, err := os.Create(outpath)
	if err != nil {
		dpLogger.Println("could not create output file:", err)
		os.Exit(1)
	}
	defer of.Close()
	ofbuf := bufio.NewWriter(of)
	defer ofbuf.Flush()

	tr := regexp.MustCompile(`^\[(\d+)\] \/Pika0 confirming (\d+) blocks proposed by node (\d+):(( (\d+))+)$`)
	var allData []int

	var nProbes map[int]int
	var nBlocks map[int]int
	discrepency := false

	for idx, i := range n {
		p := fmt.Sprintf("%s-%d.log", prefix, i)
		ofbuf.WriteString(fmt.Sprintf("\"Node %v\"\n", i))
		var data []int
		cfmProbe := make(map[int]int) // this node has confirmed how many probes from which server
		cfmBlock := make(map[int]int)
		recordData := func(m map[int]int, n int, idx int) {
			_, there := m[idx]
			if !there {
				m[idx] = n
			} else {
				m[idx] += n
			}
		}

		proc := func(l string) {
			// collect all delay samples
			m := tr.FindStringSubmatch(l)
			if len(m) != 0 {
				t, _ := strconv.Atoi(m[1])
				tss := m[4]
				timestamps := strings.Split(tss[1:], " ") // the first char of tss is ' '
				sourceNode, _ := strconv.Atoi(m[3])
				nBlocks, _ := strconv.Atoi(m[2])
				for _, v := range timestamps {
					ctime, _ := strconv.Atoi(v)
					delay := t - ctime
					data = append(data, delay)
				}
				recordData(cfmProbe, len(timestamps), sourceNode)
				recordData(cfmBlock, nBlocks, sourceNode)
			}
		}
		ScanLogFile(p, proc)

		// compare with other nodes
		if nProbes == nil {
			nProbes = cfmProbe
		} else {
			for k, v := range nProbes {
				if cfmProbe[k] != v {
					discrepency = true
				}
			}
		}
		if nBlocks == nil {
			nBlocks = cfmBlock
		} else {
			for k, v := range nBlocks {
				if cfmBlock[k] != v {
					discrepency = true
				}
			}
		}

		// calculate percentiles
		sort.Ints(data)
		if len(data) != 0 {
			for per := 0; per <= 100; per++ {
				idx := int(math.Floor(float64(per) / 100.0 * float64(len(data)-1)))
				ofbuf.WriteString(fmt.Sprintf("%v %v\n", per, data[idx]))
			}
		}
		if calc {
			allData = append(allData, data...)
		}
		if idx != len(n)-1 {
			ofbuf.WriteString("\n\n")
		}
	}
	m.Lock()
	if !discrepency {
		if calc {
			fmt.Printf("Confirmed ")
			for k, _ := range nProbes {
				fmt.Printf("node %v: %v blks %v probes, ", k, nBlocks[k], nProbes[k])
			}
		}
		fmt.Printf("Numbers of confirmed blocks and probes matching\n")
	} else {
		fmt.Println("Nodes have confirmed different numbers of blocks or probes!")
	}
	m.Unlock()
	if calc {
		sort.Ints(allData)
		tot := 0
		for _, v := range allData {
			tot += v
		}
		if len(allData) != 0 {
			fivepct := allData[int(math.Floor(5.0/100.0*float64(len(allData)-1)))]
			ninefivepct := allData[int(math.Floor(95.0/100.0*float64(len(allData)-1)))]
			mean := tot / len(allData)
			m.Lock()
			fmt.Printf("Confirmation latency 90%% interval: %v %v %v\n", fivepct, mean, ninefivepct)
			m.Unlock()
		}
	}
}

func FilterTxQueueLength(prefix string, n []int, outpath string) {
	of, err := os.Create(outpath)
	if err != nil {
		dpLogger.Println("could not create output file:", err)
		os.Exit(1)
	}
	defer of.Close()
	ofbuf := bufio.NewWriter(of)
	defer ofbuf.Flush()

	tr := regexp.MustCompile(`^\[(\d+)\] \/Pika0 transaction queue length (\d+)$`)

	for idx, i := range n {
		p := fmt.Sprintf("%s-%d.log", prefix, i)
		ofbuf.WriteString(fmt.Sprintf("\"Node %v\"\n", i))

		proc := func(l string) {
			m := tr.FindStringSubmatch(l)
			if len(m) != 0 {
				t, _ := strconv.Atoi(m[1])
				l, _ := strconv.Atoi(m[2])
				ofbuf.WriteString(fmt.Sprintln(t, l))
			}
		}
		ScanLogFile(p, proc)
		if idx != len(n)-1 {
			ofbuf.WriteString("\n\n")
		}
	}
}

func FilterBandwidth(prefix string, n []int, outpath string) {
	of, err := os.Create(outpath)
	if err != nil {
		dpLogger.Println("could not create output file:", err)
		os.Exit(1)
	}
	defer of.Close()
	ofbuf := bufio.NewWriter(of)
	defer ofbuf.Flush()

	tr := regexp.MustCompile(`^\[(\d+)\] last (\d+) ticks bandwidth provision (\d+) bytes, lp (\d+) bytes, hp (\d+) bytes$`)

	for idx, i := range n {
		p := fmt.Sprintf("%s-%d.log", prefix, i)
		ofbuf.WriteString(fmt.Sprintf("\"Node %v\"\n", i))

		proc := func(l string) {
			m := tr.FindStringSubmatch(l)
			if len(m) != 0 {
				t, _ := strconv.Atoi(m[1])
				dur, _ := strconv.Atoi(m[2])
				bw, _ := strconv.Atoi(m[3])
				lp, _ := strconv.Atoi(m[4])
				hp, _ := strconv.Atoi(m[5])
				ofbuf.WriteString(fmt.Sprintln(t, float64(bw)/float64(dur), float64(lp)/float64(dur), float64(hp)/float64(dur)))
			}
		}
		ScanLogFile(p, proc)
		if idx != len(n)-1 {
			ofbuf.WriteString("\n\n")
		}
	}
}

