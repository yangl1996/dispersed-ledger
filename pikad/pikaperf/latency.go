package main

import (
	"github.com/golang/snappy"
	"encoding/gob"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func dispatchLatency(args []string) {
	cmd := flag.NewFlagSet("latency", flag.ExitOnError)
	input := cmd.String("i", "", "path of the experiment data directory delimited by commas")
	output := cmd.String("o", "", "path of the output")
	crossTransaction := cmd.Bool("x", false, "also count transactions proposed by other nodes")
	skipStart:= cmd.Int("skip", 0, "skip the first x%")
	cmd.Parse(args)
	inputs := strings.Split(*input, ",")
	runLatency(inputs, *output, *crossTransaction, *skipStart)
}

func runLatency(paths []string, out string, cross bool, skip int) {
	// first, take a look at the first set of servers
	servers, err := ReadServerInfo(paths[0])
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// open the output
	var of io.Writer
	if out != "" {
		ofile, err := os.Create(out)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		defer ofile.Close()
		of = ofile
	} else {
		of = os.Stdout
	}
	owriter := csv.NewWriter(of)
	defer owriter.Flush()

	var results [][]string
	results = make([][]string, len(servers))
	resch := make(chan struct{int; d []string; nt int}, 20)

	// scan for each server
	for idx, v := range servers {
		go func(idx int, v Server) {
			var ifs []io.Reader
			for _, p := range paths {
				in, err := os.Open(filepath.Join(p, fmt.Sprintf("latency-%d.dat", idx)))
				if err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
				defer in.Close()
				ifs = append(ifs, in)
				if cross {
					in2, err := os.Open(filepath.Join(p, fmt.Sprintf("peerlatency-%d.dat", idx)))
					if err != nil {
						fmt.Println(err)
						os.Exit(1)
					}
					defer in2.Close()
					ifs = append(ifs, in2)
				}
			}
			data, nt := CalcLatencyStats(ifs, skip)
			od := []string{v.Location, strconv.Itoa(data[5]), strconv.Itoa(data[50]), strconv.Itoa(data[95])}
			resch <- struct{int; d []string; nt int}{idx, od, nt}
		}(idx, v)
	}

	tot := 0
	for _, _ = range servers {
		d := <-resch
		d.d = append(d.d, strconv.Itoa(d.nt))
		results[d.int] = d.d
		tot += d.nt
	}
	for _, v := range results {
		owriter.Write(v)
	}
	//fmt.Printf("Avg. num of samples: %v\n", tot / len(servers))
}

// calculate the 0-100 percentile, and the number of transactions
func CalcLatencyStats(inputs []io.Reader, skip int) ([]int, int) {
	var nums []int
	for _, input := range inputs {
		var localNums []int
		reader := snappy.NewReader(input)
		r := gob.NewDecoder(reader)
		for {
			var n int64
			err := r.Decode(&n)
			if err != nil && err != io.EOF {
				fmt.Println(err)
				os.Exit(1)
				break
			}
		    localNums = append(localNums, int(n))
			if err == io.EOF {
				break
			}
		}
		nn := len(localNums)
		var start int
		if skip != 0 {
			start = int(float64(skip) / 100.0 * float64(nn))
		}
		toInsert := localNums[start:nn-1]
		nums = append(nums, toInsert...)
	}

	sort.Ints(nums)
	res := make([]int, 101)
	step := float64(len(nums)) / 100.0
	for i := 0; i < 100; i++ {
		idx := int(step * float64(i))
		res[i] = nums[idx]
	}
	res[100] = nums[len(nums)-1]
	return res, len(nums)
}
