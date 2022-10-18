#!/usr/local/bin/gnuplot

set term pdf size 3.55,2.0
set output "fig.pdf"

set yrange [0:5]
set xrange [0:40]
set notitle
set xlabel "Time (s)"
set ylabel "Bandwidth (Mbps)"

set style fill pattern 10 noborder
set key right top outside

#unset xtics
#unset ytics

plot "threshold.trace" using ($0-15):1 title "P66" with filledcurves x1 lc 0, \
     "concurrent-uk.trace" using ($0-15):1 title "UK" with lines lc 1 lw 2, \
     "concurrent-indonesia.trace" using ($0-15):1 title "Indonesia" with lines lc 2 lw 2, \
     "concurrent-brazil.trace" using ($0-15):1 title "Brazil" with lines lc 3 lw 2, \
     "concurrent-singapore.trace" using ($0-15):1 title "Singapore" with lines lc 4 lw 2, \
#     "threshold.trace" using 0:2 notitle with lines lc 0 lw 2.5
#     "concurrent-dutch.trace" using 0:1 notitle with lines lc 6 lw 2, \
