#!/usr/local/bin/gnuplot

set term pdf size 4,1.8 font ",15"
#set size ratio 0.618
set output "geo-thruput.pdf"
set datafile separator ","
#set key outside right
set key outside right top
set ylabel "Throughput (MB/s)"
set xlabel "Node"
set xtics rotate by -45
set noxtics
set notitle
#set title "Confirmation Latency"
set yrange [0:35]

#column 0 is the row number
plot "hb.dat" using ($0):($2/235*250):(0.25):xtic(1) title "HB" with boxes fill solid lc 1, \
    "hblinking.dat" using ($0+0.30):($2/235*250):(0.25) title "HB-Link" with boxes fill solid lc 4, \
    "new.dat" using ($0+0.60):($2/235*250):(0.25) title "DL" with boxes fill solid lc 3
