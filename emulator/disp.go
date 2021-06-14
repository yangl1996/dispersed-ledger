package main

import (
	"fmt"
	c "github.com/ahmetalpbalkan/go-cursor"
	"github.com/logrusorgru/aurora"
	"sort"
	"strings"
)

const padding = 3

// pchans: progress notification
// magPg: max. progress
// width: total width of the pgbar
func ProgressBar(pchans chan struct{ Id, Pg int }, tchans chan struct {
	int
	string
}, n int, maxPg int, width int) {
	// print out the empty bars
	s := strings.Repeat("\n", n)
	fmt.Print(c.Hide())
	fmt.Print(s)
	fmt.Print(c.MoveUp(n))
	for i := 0; i < n; i++ {
		canvas := "["
		canvas += strings.Repeat(" ", width)
		canvas += "]"
		fmt.Print(canvas)
		fmt.Print(c.MoveDown(1))
		fmt.Print(c.MoveLeft(2 + width))
	}
	// put the cursor to the upper left of the empty bars
	fmt.Print(c.MoveUp(n))
	fmt.Print(c.MoveLeft(2 + width))
	fmt.Print(c.SaveAttributes())
	termed := make([]bool, n)
	// start updating progress
	p := make([]int, n)
	for {
		select {
		case v, ok := <-pchans:
			if !ok {
				pchans = nil
			} else if !termed[v.Id] {
				id := v.Id
				pg := v.Pg
				if p[id] < pg {
					p[id] = pg
					ticks := width * pg / maxPg
					//bar :=aurora.Reverse(strings.Repeat(" ", ticks))
					bar := aurora.Faint(strings.Repeat("■", ticks))
					/*
						if ticks != width {
							bar += strings.Repeat("=", ticks)
							bar += ">"
						} else {
							bar += strings.Repeat("=", ticks)
						}
					*/
					if id != 0 {
						fmt.Print(c.MoveDown(id))
					}
					fmt.Print(c.MoveRight(1))
					fmt.Print(bar)
					if ticks != width {
						fmt.Print("|")
					}
				}
			}
		case v, ok := <-tchans:
			if !ok {
				tchans = nil
			} else {
				id := v.int
				state := v.string
				if id != 0 {
					fmt.Print(c.MoveDown(id))
				}
				clear := strings.Repeat(" ", width+2)
				fmt.Print(clear)
				fmt.Print(c.MoveLeft(width + 2))
				fmt.Print(state)
				termed[id] = true
			}
		}
		fmt.Print(c.RestoreAttributes())
		if pchans == nil && tchans == nil {
			break
		}
	}
	//fmt.Print(c.ClearScreenDown())
	fmt.Print(c.MoveDown(n))
	fmt.Print(c.Show())

}

// s1, s2: data series
// legend: legend text for s1 and s2
// anno: annotation of each data point
// sec: number of sections
// secWidth: width of a section (incl. the left tick)
// max: max. number in the data series
func ShowScale(s1 []int, s2 []int, legend []string, anno []string, sec int, secWidth int, max int) {
	// build the scale
	pc := fmt.Sprintf("%%-%vv", secWidth)
	ticks := ""
	scale := "|"
	ticks += fmt.Sprintf(pc, 0)
	for i := 0; i < sec; i++ {
		scale += strings.Repeat("-", secWidth-1)
		scale += "|"
		ticks += fmt.Sprintf(pc, max*(i+1)/sec)
	}

	// assemble the data
	nd := len(s1)
	if len(s2) != nd {
		panic("data series not of equal length")
	}

	maxPos := sec * secWidth
	var records []struct {
		s1 int
		s2 int
		string
	}
	avgS1 := 0
	avgS2 := 0
	for i := 0; i < nd; i++ {
		r := struct {
			s1 int
			s2 int
			string
		}{s1[i], s2[i], ""}
		s1Pos := r.s1 * maxPos / max
		s2Pos := r.s2 * maxPos / max
		canvas := strings.Repeat(" ", maxPos+1+padding)
		disp := []rune(canvas)
		if s1Pos != s2Pos {
			disp[s1Pos] = '*'
			disp[s2Pos] = '+'
		} else {
			disp[s1Pos] = '#'
		}
		canvas = string(disp)
		canvas += anno[i]
		r.string = canvas
		avgS1 += r.s1
		avgS2 += r.s2
		records = append(records, r)
	}
	avgS1 /= nd
	avgS2 /= nd
	s1Pos := avgS1 * maxPos / max
	s2Pos := avgS2 * maxPos / max
	disp := []rune(scale)
	if s1Pos != s2Pos {
		disp[s1Pos] = '*'
		disp[s2Pos] = '+'
	} else {
		disp[s1Pos] = '#'
	}
	scale = string(disp)

	sort.Slice(records, func(i, j int) bool {
		return records[i].s1 > records[j].s1
	})

	fmt.Printf("*: %v, +: %v, #: %v=%v\n", legend[0], legend[1], legend[0], legend[1])
	fmt.Println()
	fmt.Println(ticks)
	fmt.Println(scale)
	for _, v := range records {
		fmt.Println(v.string)
	}
}
