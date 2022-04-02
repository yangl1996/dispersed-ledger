#!/usr/local/bin/gnuplot

set term pdf size 3.2,2.0
set output "fig.pdf"

set yrange [0:5]
set xrange [15:55]
set notitle
set xlabel "Time"
set ylabel "Bandwidth"

set style fill pattern 10 noborder
unset xtics
unset ytics

plot "threshold.trace" using 0:1 notitle with filledcurves x1 lc 0, \
     "concurrent-uk.trace" using 0:1 notitle with lines lc 1 lw 2, \
     "concurrent-indonesia.trace" using 0:1 notitle with lines lc 2 lw 2, \
     "concurrent-brazil.trace" using 0:1 notitle with lines lc 3 lw 2, \
     "concurrent-singapore.trace" using 0:1 notitle with lines lc 4 lw 2, \
     "threshold.trace" using 0:2 notitle with lines lc 0 lw 2.5
#     "concurrent-dutch.trace" using 0:1 notitle with lines lc 6 lw 2, \
