#!/usr/local/bin/gnuplot

set term pdf size 3.555,2.0 enhanced
set output "latency-metric-hb.pdf"
set datafile separator ","
#set key outside right
set key right top
set ylabel "Latency (s)"
set xtics rotate by -45
#set noxtics
set notitle
#set title "Confirmation Latency"
set yrange [0:10]
set ytics ("0" 0, "2" 2, "4" 4, "6" 6, "8" 8, "∞" 10)

set rmargin 5
#column 0 is the row number
plot "hb-nocross.dat" using ($0+0.125):($3/1000):(0.25):xtic(1) title "HB" with boxes fill solid
