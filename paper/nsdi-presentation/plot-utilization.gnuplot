#!/usr/local/bin/gnuplot

set term pdf size 3.2,2.0
set output "fig.pdf"

set yrange [0:20]
set xrange [0:60]
set notitle
set xlabel "Time"
set ylabel "Bandwidth"

set style fill solid 0.7 noborder
unset xtics
unset ytics

plot "uk.trace" using 0:1 notitle with lines, \
     "indonesia.trace" using 0:1 notitle with lines, \
     "brazil.trace" using 0:1 notitle with lines, \
     "singapore.trace" using 0:1 notitle with lines
