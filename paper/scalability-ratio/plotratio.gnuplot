#!/usr/local/bin/gnuplot

set term pdf size 3.3,2.0 font ",16"
set size ratio 0.618
set output "scalability-fraction.pdf"
set datafile separator " "
set key top right
set ylabel "Disperse / Total"
set xlabel "Block Size (KB)"
set notitle
#set title "Confirmation Latency"
set yrange [0:0.2]

#column 0 is the row number
plot "16.dat" using ($1/1000):($2/($3+$2)) title "N=16" with linespoints lw 2 ps 0.7, \
     "32.dat" using ($1/1000):($2/($3+$2)) title "32" with linespoints lw 2 ps 0.7, \
    "64.dat" using ($1/1000):($2/($3+$2)) title "64" with linespoints lw 2 ps 0.7, \
    "128.dat" using ($1/1000):($2/($3+$2)) title "128" with linespoints lw 2 ps 0.7, \
