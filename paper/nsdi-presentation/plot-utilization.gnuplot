#!/usr/local/bin/gnuplot

set term pdf size 3.2,2.0
set output "fig.pdf"

set yrange [0:5]
set xrange [0:55]
set notitle
set xlabel "Time"
set ylabel "Bandwidth"

set style fill solid 0.7 noborder
unset xtics
unset ytics

plot "concurrent-uk.trace" using 0:1 notitle with lines, \
     "concurrent-indonesia.trace" using 0:1 notitle with lines, \
     "concurrent-brazil.trace" using 0:1 notitle with lines, \
     "concurrent-singapore.trace" using 0:1 notitle with lines
