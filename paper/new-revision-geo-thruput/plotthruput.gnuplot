#!/usr/local/bin/gnuplot

set term pdf size 5.3,2.0 font ",16"
#set size ratio 0.618
set output "geo-thruput.pdf"
set datafile separator ","
#set key outside right
set key horizontal outside right top
set ylabel "Throughput (MB/s)"
set xlabel "Node"
set noxtics
set notitle
#set title "Confirmation Latency"
set yrange [0:50]

#column 0 is the row number
plot "hb.dat" using ($0):($2/250*250):(0.15):xtic(1) title "HB" with boxes fill solid, \
    "hb-linking.dat" using ($0+0.20):($2/250*250):(0.15) title "HB-Link" with boxes fill solid, \
    "coupled-dl.dat" using ($0+0.40):($2/250*250):(0.15) title "DL-Coupled" with boxes fill solid, \
    "decoupled-dl.dat" using ($0+0.60):($2/250*250):(0.15) title "DL" with boxes fill solid
