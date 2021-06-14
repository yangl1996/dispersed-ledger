package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"
)

var dpLogger = log.New(os.Stderr, "", 0)

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
	//proc.Start()
	go func() {
		tmpl.Execute(stdin, tplParams)
		stdin.Close()
	}()
	out, err := proc.CombinedOutput()
	//err = proc.Wait()
	if err != nil {
		dpLogger.Println("error running gnuplot:", err)
		dpLogger.Printf("gnuplot says: %s\n", out)
		//os.Exit(1)
	}
}

func PlotBandwidth(dataPath string, outPath string, serverloc string) {
	const TerminationPlotTemplate = `set term svg
	set size ratio 0.618
set output "{{ .OutputPath }}"
set datafile separator ","
set key outside right
set xlabel "Time"
set ylabel "Rate (MB/s)"
set title "Bandwidth, {{ .ServerLoc }}"
set xdata time
set timefmt "%Y/%m/%d %H:%M:%S"
plot "{{ .InputPath }}" using 1:2 with lines lc 1 notitle, \
"{{ .InputPath }}" using 1:3 with lines lc 2 notitle, \
"{{ .InputPath }}" using 1:4 with lines lc 3 notitle, \
"{{ .InputPath }}" using 1:5 with lines lc 8 notitle, \
"{{ .InputPath }}" using 1:6 with lines lc rgb '#66C2A5' notitle, \
NaN with lines lc 1 title "Total", \
NaN with lines lc 2 title "High Prio", \
NaN with lines lc 3 title "Low Prio", \
NaN with lines lc 8 title "Total (60s)", \
NaN with lines lc rgb '#66C2A5' title "App (60s)"`

	params := struct {
		OutputPath string
		InputPath  string
		ServerLoc  string
	}{outPath, dataPath, serverloc}
	RunGnuplot(TerminationPlotTemplate, params)
	dpLogger.Println("Bandwidth plotted at", outPath)
}

func PlotProgress(dataPath string, outPath string, serverloc string) {
	const TerminationPlotTemplate = `set term svg
	set size ratio 0.618
set output "{{ .OutputPath }}"
set datafile separator ","
set key outside right
set xlabel "Time"
set ylabel "Rounds"
set ytics nomirror
set y2tics
set y2label "Relative Rounds"
set title "Protocol, {{ .ServerLoc }}"
set xdata time
set timefmt "%Y/%m/%d %H:%M:%S"
plot "{{ .InputPath }}" using 1:2 with lines lc 4 notitle, \
"{{ .InputPath }}" using 1:3 with lines lc 5 notitle, \
"{{ .InputPath }}" using 1:4 with lines lc 6 notitle, \
"{{ .InputPath }}" using 1:6 with lines lc 3 notitle, \
"{{ .InputPath }}" using 1:5 with lines lc 7 notitle, \
"{{ .InputPath }}" using 1:7 with lines lc 1 notitle axis x1y2, \
"{{ .InputPath }}" using 1:8 with lines lc 2 notitle axis x1y2, \
NaN with lines lc 4 title "Dispersed", \
NaN with lines lc 5 title "Retrieved", \
NaN with lines lc 6 title "Initiated", \
NaN with lines lc 3 title "Disp-cont", \
NaN with lines lc 7 title "Confirmed", \
NaN with lines lc 1 title "Cfm (Rel)", \
NaN with lines lc 2 title "Init (Rel)"`

	params := struct {
		OutputPath string
		InputPath  string
		ServerLoc  string
	}{outPath, dataPath, serverloc}
	RunGnuplot(TerminationPlotTemplate, params)
	dpLogger.Println("Progress plotted at", outPath)
}

func PlotMemory(dataPath string, outPath string, serverloc string) {
	const TerminationPlotTemplate = `set term svg
	set size ratio 0.618
set output "{{ .OutputPath }}"
set datafile separator ","
set key outside right
set xlabel "Time"
set ylabel "Memory Usage (MB)"
set title "Memory Usage, {{ .ServerLoc }}"
set xdata time
set timefmt "%Y/%m/%d %H:%M:%S"
plot "{{ .InputPath }}" using 1:2 with lines lc 1 notitle, \
"{{ .InputPath }}" using 1:3 with lines lc 2 notitle, \
NaN with lines lc 1 title "OS", \
NaN with lines lc 2 title "Heap"`

	params := struct {
		OutputPath string
		InputPath  string
		ServerLoc  string
	}{outPath, dataPath, serverloc}
	RunGnuplot(TerminationPlotTemplate, params)
	dpLogger.Println("Memory plotted at", outPath)
}

func PlotBlockSize(dataPath string, outPath string, serverloc string) {
	const TerminationPlotTemplate = `set term svg
	set size ratio 0.618
set output "{{ .OutputPath }}"
set datafile separator ","
set key outside right
set xlabel "Round"
set ylabel "Size (Byte)"
set title "Block, {{ .ServerLoc }}"
plot "{{ .InputPath }}" using 1:2 with lines lc 1 notitle, \
NaN with lines lc 1 title "Size"`

	params := struct {
		OutputPath string
		InputPath  string
		ServerLoc  string
	}{outPath, dataPath, serverloc}
	RunGnuplot(TerminationPlotTemplate, params)
	dpLogger.Println("Block plotted at", outPath)
}

func PlotQueue(dataPath string, outPath string, serverloc string) {
	const TerminationPlotTemplate = `set term svg
	set size ratio 0.618
set output "{{ .OutputPath }}"
set datafile separator ","
set key outside right
set xlabel "Time"
set ylabel "Messages"
set ytics nomirror
set y2tics
set y2label "Transactions"
set title "Queue Length, {{ .ServerLoc }}"
set xdata time
set timefmt "%Y/%m/%d %H:%M:%S"
plot "{{ .InputPath }}" using 1:2 with lines lc 1 notitle, \
"{{ .InputPath }}" using 1:3 with lines lc 2 notitle, \
"{{ .InputPath }}" using 1:5 with lines lc 4 notitle, \
"{{ .InputPath }}" using 1:4 with lines lc 3 axis x1y2 notitle, \
NaN with lines lc 1 title "High", \
NaN with lines lc 2 title "Low", \
NaN with lines lc 4 title "Recv", \
NaN with lines lc 3 title "Txn"`

	params := struct {
		OutputPath string
		InputPath  string
		ServerLoc  string
	}{outPath, dataPath, serverloc}
	RunGnuplot(TerminationPlotTemplate, params)
	dpLogger.Println("Queue plotted at", outPath)
}

func PlotConfirmation(dataPath string, outPath string, serverloc string) {
	const TerminationPlotTemplate = `set term svg
	set size ratio 0.618
set output "{{ .OutputPath }}"
set datafile separator ","
set key outside right
set xlabel "Time"
set ylabel "Latency (ms)"
set ytics nomirror
set y2tics
set y2label "Rate"
set title "Confirmation, {{ .ServerLoc }}"
set xdata time
set timefmt "%Y/%m/%d %H:%M:%S"
plot "{{ .InputPath }}" using 1:2 with lines lc 1 notitle, \
"{{ .InputPath }}" using 1:3 with lines lc 2 axis x1y2 notitle, \
NaN with lines lc 1 title "Delay", \
NaN with lines lc 2 title "Rate"`

	params := struct {
		OutputPath string
		InputPath  string
		ServerLoc  string
	}{outPath, dataPath, serverloc}
	RunGnuplot(TerminationPlotTemplate, params)
	dpLogger.Println("Confirmation plotted at", outPath)
}

func PlotMessageDelay(dataPath string, outPath string, serverloc string) {
	const TerminationPlotTemplate = `set term svg
	set size ratio 0.618
set output "{{ .OutputPath }}"
set datafile separator ","
set key outside right
set xlabel "Time"
set ylabel "Latency (ms)"
set title "Message, {{ .ServerLoc }}"
set xdata time
set timefmt "%Y/%m/%d %H:%M:%S"
set ytics nomirror
set y2tics
set y2label "Epoch Diff"
plot "{{ .InputPath }}" using 1:2 with lines lc 1 notitle, \
"{{ .InputPath }}" using 1:3 with lines lc 2 notitle, \
"{{ .InputPath }}" using 1:4 with lines lc 3 notitle, \
"{{ .InputPath }}" using 1:5 with lines lc 4 notitle, \
"{{ .InputPath }}" using 1:6 with lines lc 5 notitle axis x1y2, \
"{{ .InputPath }}" using 1:8 with lines lc 6 notitle axis x1y2, \
NaN with lines lc 1 title "High", \
NaN with lines lc 2 title "Low", \
NaN with lines lc 3 title "High (E)", \
NaN with lines lc 4 title "Low (E)", \
NaN with lines lc 5 title "Pos", \
NaN with lines lc 6 title "PosAvg"`
	// "{{ .InputPath }}" using 1:7 with lines lc 6 notitle axis x1y2, \
	// NaN with lines lc 6 title "Neg"`

	params := struct {
		OutputPath string
		InputPath  string
		ServerLoc  string
	}{outPath, dataPath, serverloc}
	RunGnuplot(TerminationPlotTemplate, params)
	dpLogger.Println("Delay plotted at", outPath)
}

func PlotConfirmedBytes(nservers int) {
	const TerminationPlotTemplate = `set term pdf size 1.8,1.12
	set size ratio 0.7
	set linetype  1 lc rgb "dark-violet" lw 1 dt 1 pt 0
	set linetype  2 lc rgb "sea-green"   lw 1 dt 1 pt 7
	set linetype  3 lc rgb "cyan"        lw 1 dt 1 pt 6 pi -1
	set linetype  4 lc rgb "dark-red"    lw 1 dt 1 pt 5 pi -1
	set linetype  5 lc rgb "blue"        lw 1 dt 1 pt 8
	set linetype  6 lc rgb "dark-orange" lw 1 dt 1 pt 3
	set linetype  7 lc rgb "black"       lw 1 dt 1 pt 11
	set linetype  8 lc rgb "goldenrod"   lw 1 dt 1
	set linetype 9 lc rgb '#66C2A5' # teal
	set linetype 10 lc rgb '#FC8D62' # orange
	set linetype 11 lc rgb '#8DA0CB' # lilac
	set linetype 12 lc rgb '#E78AC3' # magentat
	set linetype 13 lc rgb '#A6D854' # lime green
	set linetype 14 lc rgb '#FFD92F' # banana
	set linetype 15 lc rgb '#E5C494' # tan
	set linetype 16 lc rgb '#B3B3B3' # grey
	set linetype cycle 16
set output "cfmbytes.pdf" 
set datafile separator ","
set nokey
set xlabel "Time"
unset xtics
set xdata time
set timefmt "%Y/%m/%d %H:%M:%S"
set ylabel "Bytes Confirmed"
set yrange [0:3.5]
set ytics right out rotate ("0" 0, "3.5 G" 3.5) nomirror
plot {{ .PlotCommands }}
`
	plotCommands := []string{}
	for i := 0; i < nservers; i++ {
		cmd := fmt.Sprintf(`"cfmbytes-%d.dat" using 1:($2/1000000000) with lines notitle`, i)
		plotCommands = append(plotCommands, cmd)
	}

	params := struct {
		PlotCommands string
	}{strings.Join(plotCommands, ", ")}
	RunGnuplot(TerminationPlotTemplate, params)
}

func dispatchShow(args []string) {
	cmd := flag.NewFlagSet("show", flag.ExitOnError)
	inputDir := cmd.String("i", "", "path of the log bundle")
	nodeIdx := cmd.Int("n", 0, "node to display or plot")
	nodeLoc := cmd.String("l", "", "search node by location")
	output := cmd.String("o", "plot", "prefix of the output plots")
	openSafari := cmd.Int("show", 0, "open the plot in an auto-refreshing tab in Safari with the given interval")
	ignorestart := cmd.Int("skip", 0, "seconds to skip at the beginning [ONLY applies to cfm bandwidth print]")
	cmd.Parse(args)

	servers, err := ReadServerInfo(*inputDir)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if *nodeLoc != "" {
		idx := SearchServer(servers, *nodeLoc)
		if idx == -1 {
			fmt.Printf("No server matching keyword %s\n", *nodeLoc)
			os.Exit(1)
		}
		*nodeIdx = idx
	}

	input := filepath.Join(*inputDir, "log-"+strconv.Itoa(*nodeIdx)+".log")

	loc := servers[*nodeIdx].Location

	plot := func() {
		fmt.Println(loc)
		FilterBandwidth(input, "bw.dat", *ignorestart)
		PlotBandwidth("bw.dat", *output+"-bw.svg", loc)
		FilterProgress(input, "proto.dat")
		PlotProgress("proto.dat", *output+"-proto.svg", loc)
		FilterMemory(input, "mem.dat")
		PlotMemory("mem.dat", *output+"-mem.svg", loc)
		FilterQueue(input, "q.dat")
		PlotQueue("q.dat", *output+"-q.svg", loc)
		FilterConfirmation(input, "cfm.dat")
		PlotConfirmation("cfm.dat", *output+"-cfm.svg", loc)
		FilterBlockSize(input, "blk.dat")
		PlotBlockSize("blk.dat", *output+"-blk.svg", loc)
		FilterMessageDelay(input, "msg.dat")
		PlotMessageDelay("msg.dat", *output+"-msg.svg", loc)
		// clean up
		os.Remove("bw.dat")
		os.Remove("mem.dat")
		os.Remove("q.dat")
		os.Remove("cfm.dat")
		os.Remove("msg.dat")
		os.Remove("blk.dat")
		os.Remove("proto.dat")
	}
	plot()

	if *openSafari != 0 {
		serve() // serve at localhost 8080
		cmd := exec.Command("open", "http://localhost:8080/view")
		err := cmd.Run()
		if err != nil {
			dpLogger.Println(err)
		}
		// run the ticker
		ticker := time.NewTicker(time.Duration(*openSafari) * time.Second)
		for _ = range ticker.C {
			plot()
		}
	} else {
		nservers := len(servers)
		for i := 0; i < nservers; i++ {
			input := filepath.Join(*inputDir, "log-"+strconv.Itoa(i)+".log")
			FilterConfirmedBytes(input, fmt.Sprintf("cfmbytes-%d.dat", i))
		}
		PlotConfirmedBytes(nservers)
		for i := 0; i < nservers; i++ {
			os.Remove(fmt.Sprintf("cfmbytes-%d.dat", i))
		}
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

func FilterProgress(log string, outpath string) {
	of, err := os.Create(outpath)
	if err != nil {
		dpLogger.Println("could not create output file:", err)
		os.Exit(1)
	}
	defer of.Close()
	ofbuf := bufio.NewWriter(of)
	defer ofbuf.Flush()

	tr := regexp.MustCompile(`^(.*) HP Read: (\d+) Bytes, LP Read: (\d+) Bytes, Confirm: (\d+) Bytes$`)
	hr := regexp.MustCompile(`high-priority terminating`)
	ttr := regexp.MustCompile(`[0-9] terminating while`)
	startr := regexp.MustCompile(`starting Pika epoch`)
	outputr := regexp.MustCompile(`outputting epoch`)
	cdr := regexp.MustCompile(`continuous dispersed (\d+) epochs`)

	var hrcounter int
	var termcounter int
	var startcounter int
	var outputcounter int
	var chrcounter int
	proc := func(l string) {
		ss := hr.FindStringSubmatch(l)
		if len(ss) != 0 {
			hrcounter += 1
		}
		ch := cdr.FindStringSubmatch(l)
		if len(ch) != 0 {
			chrcounter, _ = strconv.Atoi(ch[1])
		}
		sss := ttr.FindStringSubmatch(l)
		if len(sss) != 0 {
			termcounter += 1
		}
		ssss := startr.FindStringSubmatch(l)
		if len(ssss) != 0 {
			startcounter += 1
		}
		outputmatch := outputr.FindStringSubmatch(l)
		if len(outputmatch) != 0 {
			outputcounter += 1
		}
		m := tr.FindStringSubmatch(l)
		if len(m) != 0 {
			ofbuf.WriteString(fmt.Sprintf("%v,%v,%v,%v,%v,%v,%v,%v\n", m[1], hrcounter, termcounter, startcounter, outputcounter, chrcounter, outputcounter-hrcounter, startcounter-hrcounter))
		}
	}
	ScanLogFile(log, proc)

	return
}

func FilterConfirmedBytes(log string, outpath string) {
	of, err := os.Create(outpath)
	if err != nil {
		dpLogger.Println("could not create output file:", err)
		os.Exit(1)
	}
	defer of.Close()
	ofbuf := bufio.NewWriter(of)
	defer ofbuf.Flush()

	tr := regexp.MustCompile(`^(.*) HP Read: (\d+) Bytes, LP Read: (\d+) Bytes, Confirm: (\d+) Bytes$`)

	proc := func(l string) {
		m := tr.FindStringSubmatch(l)
		if len(m) != 0 {
			confirm, _ := strconv.Atoi(m[4])
			/*ts, err := time.Parse("2006/01/02 15:04:05.000000", m[1])
			if err != nil {
				dpLogger.Fatalln(err)
			}*/

			ofbuf.WriteString(fmt.Sprintf("%v,%v\n", m[1], confirm))
		}
	}
	ScanLogFile(log, proc)

	return
}

func FilterBandwidth(log string, outpath string, skip int) {
	of, err := os.Create(outpath)
	if err != nil {
		dpLogger.Println("could not create output file:", err)
		os.Exit(1)
	}
	defer of.Close()
	ofbuf := bufio.NewWriter(of)
	defer ofbuf.Flush()

	tr := regexp.MustCompile(`^(.*) HP Read: (\d+) Bytes, LP Read: (\d+) Bytes, Confirm: (\d+) Bytes$`)

	var desiredStartTime time.Time
	var beginConfirm int
	beginConfirm = -1
	var lastTime time.Time
	var lastHp int
	var lastLp int
	var lastConfirm int
	var beginTime time.Time
	var pastRes []float64
	var pastResConfirm []float64
	var tot float64
	var totCfm float64
	proc := func(l string) {
		m := tr.FindStringSubmatch(l)
		if len(m) != 0 {
			hp, _ := strconv.Atoi(m[2])
			lp, _ := strconv.Atoi(m[3])
			confirm, _ := strconv.Atoi(m[4])
			ts, err := time.Parse("2006/01/02 15:04:05.000000", m[1])
			if err != nil {
				dpLogger.Fatalln(err)
			}
			// calculate the different from the last time
			if lastTime.IsZero() {
				lastTime = ts
				lastHp = hp
				lastLp = lp
				lastConfirm = confirm
				beginTime = ts
				desiredStartTime = ts.Add(time.Duration(skip) * time.Second)
			} else {
				timeDiff := ts.Sub(lastTime).Microseconds()
				lastTime = ts
				hpDiff := hp - lastHp
				lastHp = hp
				lpDiff := lp - lastLp
				confirmDiff := confirm - lastConfirm
				lastConfirm = confirm
				lastLp = lp

				bw := float64(hpDiff+lpDiff) / float64(timeDiff)
				cfm := float64(confirmDiff) / float64(timeDiff)
				nSamples := 60
				if len(pastRes) >= nSamples {
					pastRes = pastRes[1:]
					pastResConfirm = pastResConfirm[1:]
				}
				pastRes = append(pastRes, bw)
				pastResConfirm = append(pastResConfirm, cfm)
				tot = 0.0
				totCfm = 0.0
				for _, val := range pastRes {
					tot += val
				}
				for _, val := range pastResConfirm {
					totCfm += val
				}
				if !ts.Before(desiredStartTime) {
					if beginConfirm == -1 {
						beginConfirm = confirm
					}
				}

				ofbuf.WriteString(fmt.Sprintf("%v,%v,%v,%v,%v,%v\n", m[1], float64(hpDiff+lpDiff)/float64(timeDiff), float64(hpDiff)/float64(timeDiff), float64(lpDiff)/float64(timeDiff), tot/float64(len(pastRes)), totCfm/float64(len(pastResConfirm))))
			}
		}
	}
	ScanLogFile(log, proc)

	// print our stats
	duration := lastTime.Sub(beginTime).Microseconds()
	durationForCfm := lastTime.Sub(desiredStartTime).Microseconds()
	dpLogger.Printf("Duration: %d seconds, High-priority: %.3f MB/s, Low-priority: %.3f MB/s, Total: %.3f MB/s (avg. %.3f MB/s), Confirm: %.3f MB/s (avg. %.3f MB/s)\n", duration/1000000, float64(lastHp)/float64(duration), float64(lastLp)/float64(duration), float64(lastHp+lastLp)/float64(duration), tot/float64(len(pastRes)), float64(lastConfirm-beginConfirm)/float64(durationForCfm), totCfm/float64(len(pastResConfirm)))
	return
}

func FilterMemory(log string, outpath string) {
	of, err := os.Create(outpath)
	if err != nil {
		dpLogger.Println("could not create output file:", err)
		os.Exit(1)
	}
	defer of.Close()
	ofbuf := bufio.NewWriter(of)
	defer ofbuf.Flush()

	tr := regexp.MustCompile(`^(.*) OS Mem Usage: (\d+) Bytes, Heap: (\d+) Bytes$`)

	proc := func(l string) {
		m := tr.FindStringSubmatch(l)
		if len(m) != 0 {
			hp, _ := strconv.Atoi(m[2])
			lp, _ := strconv.Atoi(m[3])
			if err != nil {
				dpLogger.Fatalln(err)
			}
			ofbuf.WriteString(fmt.Sprintf("%v,%v,%v\n", m[1], hp/1024/1024, lp/1024/1024))
		}
	}
	ScanLogFile(log, proc)
	return
}

func FilterQueue(log string, outpath string) {
	of, err := os.Create(outpath)
	if err != nil {
		dpLogger.Println("could not create output file:", err)
		os.Exit(1)
	}
	defer of.Close()
	ofbuf := bufio.NewWriter(of)
	defer ofbuf.Flush()

	tr := regexp.MustCompile(`^(.*) Queue HP: (\d+), LP: (\d+), Tx: (\d+), In: (\d+)$`)

	proc := func(l string) {
		m := tr.FindStringSubmatch(l)
		if len(m) != 0 {
			hp, _ := strconv.Atoi(m[2])
			lp, _ := strconv.Atoi(m[3])
			tx, _ := strconv.Atoi(m[4])
			in, _ := strconv.Atoi(m[5])
			if err != nil {
				dpLogger.Fatalln(err)
			}
			ofbuf.WriteString(fmt.Sprintf("%v,%v,%v,%v,%v\n", m[1], hp, lp, tx, in))
		}
	}
	ScanLogFile(log, proc)
	return
}

func FilterConfirmation(log string, outpath string) {
	of, err := os.Create(outpath)
	if err != nil {
		dpLogger.Println("could not create output file:", err)
		os.Exit(1)
	}
	defer of.Close()
	ofbuf := bufio.NewWriter(of)
	defer ofbuf.Flush()

	tr := regexp.MustCompile(`^(.*) Confirmation Latency: (\d+) txs, (\d+) ms$`)
	var lastTxs int
	var lastDelay int
	var lastTime time.Time

	proc := func(l string) {
		m := tr.FindStringSubmatch(l)
		if len(m) != 0 {
			ts, err := time.Parse("2006/01/02 15:04:05.000000", m[1])
			if err != nil {
				dpLogger.Fatalln(err)
			}
			txs, _ := strconv.Atoi(m[2])
			delay, _ := strconv.Atoi(m[3])
			if err != nil {
				dpLogger.Fatalln(err)
			}
			if (!lastTime.IsZero()) && txs != lastTxs {
				avgLatency := float64(delay-lastDelay) / float64(txs-lastTxs)
				timeDiff := ts.Sub(lastTime).Microseconds()
				rate := int(float64(txs-lastTxs) / float64(timeDiff) * 1000000.0)
				ofbuf.WriteString(fmt.Sprintf("%v,%v,%v\n", m[1], avgLatency, rate))
				lastTime = ts
			}
			if lastTime.IsZero() {
				lastTime = ts
			}
			lastTxs = txs
			lastDelay = delay
		}
	}
	ScanLogFile(log, proc)
	totalAvgLatency := float64(lastDelay) / float64(lastTxs)
	dpLogger.Printf("Confirmation Latency: %.3f ms\n", totalAvgLatency)
	return
}

func FilterBlockSize(log string, outpath string) {
	of, err := os.Create(outpath)
	if err != nil {
		dpLogger.Println("could not create output file:", err)
		os.Exit(1)
	}
	defer of.Close()
	ofbuf := bufio.NewWriter(of)
	defer ofbuf.Flush()

	tr := regexp.MustCompile(`starting Pika epoch (\d+), (\d+) bytes`)

	totsize := 0
	num := 0

	proc := func(l string) {
		m := tr.FindStringSubmatch(l)
		if len(m) != 0 {
			epoch, _ := strconv.Atoi(m[1])
			size, _ := strconv.Atoi(m[2])
			if err != nil {
				dpLogger.Fatalln(err)
			}
			TxSize := 250
			realSize := int(float64(size) / float64(TxSize-15) * float64(TxSize))
			totsize += realSize
			num += 1
			ofbuf.WriteString(fmt.Sprintf("%v,%v\n", epoch, realSize))
		}
	}
	ScanLogFile(log, proc)
	dpLogger.Printf("Block Size: %v Bytes\n", totsize/num)
	return
}

func FilterMessageDelay(log string, outpath string) {
	of, err := os.Create(outpath)
	if err != nil {
		dpLogger.Println("could not create output file:", err)
		os.Exit(1)
	}
	defer of.Close()
	ofbuf := bufio.NewWriter(of)
	defer ofbuf.Flush()

	tr := regexp.MustCompile(`^(.*) Message delay: (\d+) HP, (\d+) ms, (\d+) LP, (\d+) ms, (\d+) HP, (\d+) ms, (\d+) LP, (\d+) ms, (\d+) posdiff, (\-?\d+) negdiff, (\d+) avgpos$`)
	var lastHP int
	var lastHPDelay int
	var lastLP int
	var lastLPDelay int
	var lastProcHP int
	var lastProcHPDelay int
	var lastProcLP int
	var lastProcLPDelay int

	proc := func(l string) {
		m := tr.FindStringSubmatch(l)
		if len(m) != 0 {
			hps, _ := strconv.Atoi(m[2])
			hpdelay, _ := strconv.Atoi(m[3])
			lps, _ := strconv.Atoi(m[4])
			lpdelay, _ := strconv.Atoi(m[5])
			prochps, _ := strconv.Atoi(m[6])
			prochpdelay, _ := strconv.Atoi(m[7])
			proclps, _ := strconv.Atoi(m[8])
			proclpdelay, _ := strconv.Atoi(m[9])
			if err != nil {
				dpLogger.Fatalln(err)
			}
			if hps != lastHP && lps != lastLP {
				avgHPLatency := float64(hpdelay-lastHPDelay) / float64(hps-lastHP)
				avgLPLatency := float64(lpdelay-lastLPDelay) / float64(lps-lastLP)
				avgHPLatencyProc := float64(prochpdelay-lastProcHPDelay) / float64(prochps-lastProcHP)
				avgLPLatencyProc := float64(proclpdelay-lastProcLPDelay) / float64(proclps-lastProcLP)
				ofbuf.WriteString(fmt.Sprintf("%v,%v,%v,%v,%v,%v,%v,%v\n", m[1], avgHPLatency, avgLPLatency, avgHPLatencyProc, avgLPLatencyProc, m[10], m[11], m[12]))

				lastHP = hps
				lastLP = lps
				lastHPDelay = hpdelay
				lastLPDelay = lpdelay
				lastProcHP = prochps
				lastProcLP = proclps
				lastProcHPDelay = prochpdelay
				lastProcLPDelay = proclpdelay
			}
		}
	}
	ScanLogFile(log, proc)
	return
}
