package main

import (
	"compress/gzip"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"
)

var Debug = false

func TwoStates(htime, hbw, ltime, lbw int) string {
	b := &strings.Builder{}

	// calculate the interval of high-bandwidth state and low-bandwidth state
	// bandwidths are in terms of KB/s
	// time are in terms of ms
	hintv := 1000.0 / (float64(hbw) * 1000.0 / 1500.0)
	lintv := 1000.0 / (float64(lbw) * 1000.0 / 1500.0)

	curtime := 0.0
	for curtime < float64(htime) {
		b.WriteString(strconv.Itoa(int(curtime)))
		b.WriteString("\n")
		curtime += hintv
	}
	for curtime < float64(htime+ltime) {
		b.WriteString(strconv.Itoa(int(curtime)))
		b.WriteString("\n")
		curtime += lintv
	}
	return b.String()
}

// every parameter is ms or KB/s
// X[n+1] = alpha * X[n] + N(0, stddev^2)
func GaussMarkov(duration, sampleIntv int, mean, alpha, stddev float64) string {
	b := &strings.Builder{}

	bw := 0.0
	currentTime := 0.0

	normFactor := math.Sqrt(1 - alpha*alpha)

	for currentTime < float64(duration) {
		// sample for this interval
		currentBw := alpha*bw + rand.NormFloat64()*stddev*normFactor
		bw = currentBw
		realBw := mean + currentBw
		if Debug {
			fmt.Fprintf(os.Stderr, "%d %.2f\n", int(currentTime), realBw)
		}
		if realBw > 0.0 {
			intv := 1500.0 / realBw
			intvEnd := currentTime + float64(sampleIntv)
			t := currentTime
			for t < intvEnd {
				b.WriteString(strconv.Itoa(int(t)))
				b.WriteString("\n")
				t += intv
			}
		}
		currentTime += float64(sampleIntv)
	}
	return b.String()
}

func dispatchMahimahi(args []string) {
	seed := time.Now().UnixNano()
	rand.Seed(seed)

	command := flag.NewFlagSet("mm", flag.ExitOnError)
	pattern := command.String("p", "gm", "pattern of the bandwidth: binary, gm")
	compress := command.Bool("c", true, "compress the output using gzip")
	duration := command.Int("d", 600000, "duration of the trace in ms")
	interval := command.Int("i", 1000, "interval of samples in ms")
	mean := command.Float64("mean", 10000.0, "mean bandwidth in KB/s")
	variance := command.Float64("variance", 1000.0, "variance in KB/s")
	alpha := command.Float64("alpha", 0.99, "corelation parameter that is smaller than 1.0")
	debug := command.Bool("plot", false, "output time-series to stderr")
	highbw := command.Int("high", 10000, "high bandwidth in binary mode in KB/s")
	lowbw := command.Int("low", 0, "low bandwidth in binary mode in KB/s")
	hightime := command.Int("hightime", 10000, "high time in binary mode in ms")
	lowtime := command.Int("lowtime", 0, "low time in binary mode in ms")

	command.Parse(args[0:])

	Debug = *debug
	var result string

	switch *pattern {
	case "binary":
		result = TwoStates(*hightime, *highbw, *lowtime, *lowbw)
	case "gm":
		result = GaussMarkov(*duration, *interval, *mean, *alpha, *variance)
	}

	if !*compress {
		fmt.Print(result)
	} else {
		cr := gzip.NewWriter(os.Stdout)
		cr.Write([]byte(result))
		cr.Flush()
		cr.Close()
	}

}
