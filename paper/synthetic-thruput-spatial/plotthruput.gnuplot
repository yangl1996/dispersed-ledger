#!/usr/local/bin/gnuplot

set term pdf size 3,2 font ",15"
set size ratio 0.618
set output "synthetic-thruput-spatial.pdf"
set datafile separator ","
#set key outside right
set key right bottom
set ylabel "Throughput (MB/s)"
set xlabel "Node (sorted by increasing bandwidth)"
set xtics rotate by -45
set noxtics
set notitle
#set title "Confirmation Latency"
set yrange [0:10]

#column 0 is the row number
plot "hb.dat" using ($0+0.3):($2/235*250):(0.25):xtic(1) title "HB" with linespoints lw 2 pt 4 ps 0.9, \
    "hblinking.dat" using ($0+0.3):($2/235*250):(0.25) title "HB-Link" with linespoints lw 2 pt 6 ps 0.9, \
    "new.dat" using ($0+0.3):($2/235*250):(0.25) title "DL" with linespoints lw 2 pt 8 ps 0.9
