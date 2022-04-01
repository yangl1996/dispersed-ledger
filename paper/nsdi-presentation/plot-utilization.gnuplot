#!/usr/local/bin/gnuplot

set term pdf size 3.2,2.0
set output "fig.pdf"

set yrange [0:30000]
set xrange [0:600000]
set notitle
set xlabel "Time"
set ylabel "Bandwidth"

set style fill solid 0.7 noborder
unset xtics
unset ytics

plot "data.dat" using 1:2 notitle with lines lc "black" lw 0.7, \
     "data.dat" using 1:3 notitle with lines lc "black" lw 0.7, \
     "data.dat" using 1:4 notitle with lines lc "black" lw 0.7, \
     "data.dat" using 1:5 notitle with lines lc "black" lw 0.7
