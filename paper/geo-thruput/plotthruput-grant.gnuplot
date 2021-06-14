#!/usr/local/bin/gnuplot

set term pdf size 3.3,2.0
set size ratio 0.618
set output "geo-thruput-grant.pdf"
set datafile separator ","
#set key outside right
set key right top
set ylabel "Throughput (MB/s)"
set xlabel "Node"
set xtics rotate by -45
set noxtics
set notitle
#set title "Confirmation Latency"
set yrange [0:40]

#column 0 is the row number
plot "hb.dat" using ($0):($2/235*250):(0.25):xtic(1) title "HoneyBadgerBFT" with boxes fill solid, \
    "new.dat" using ($0+0.30):($2/235*250):(0.25) title "DispersedLedger" with boxes fill solid
