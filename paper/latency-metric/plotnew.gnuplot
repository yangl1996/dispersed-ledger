#!/usr/local/bin/gnuplot

set term pdf size 3.3,2.0
set size ratio 0.618
set output "latency-metric-new.pdf"
set datafile separator ","
#set key outside right
set key right top
set ylabel "Latency (ms)"
set xlabel "Node"
set xtics rotate by -45
set noxtics
set notitle
#set title "Confirmation Latency"
set yrange [0:10000]

#column 0 is the row number
plot "new-nocross.dat" using ($0):3:(0.25):xtic(1) title "Local Tx Only" with boxes fill solid, \
    "new-nocross.dat" using ($0):3:2:4 notitle with yerrorbars lc rgb 'black' pt 1 lw 1 ps 0.5, \
    "new-cross.dat" using ($0+0.30):3:(0.25) title "All Tx" with boxes fill solid, \
    "new-cross.dat" using ($0+0.30):3:2:4 notitle with yerrorbars lc rgb 'black' pt 1 lw 1 ps 0.5
