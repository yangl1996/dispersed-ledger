#!/usr/local/bin/gnuplot

set term pdf size 4.3,2.0
set size ratio 0.618
set output "geo-latency.pdf"
set datafile separator ","
set key outside right
set ylabel "Latency (ms)"
set xlabel "Load (MB/s)"
set notitle
#set title "Confirmation Latency"
set yrange [0:10000]

#column 0 is the row number
plot     "ap-east-1-hb.csv" using ($1*250/1000000*17):3 notitle with lines lw 2 dt 2 lc rgb "#95afafaf", \
    "ap-northeast-1-hb.csv" using ($1*250/1000000*17):3 notitle with lines lw 2 dt 2 lc rgb "#95afafaf", \
    "ap-northeast-2-hb.csv" using ($1*250/1000000*17):3 notitle with lines lw 2 dt 2 lc rgb "#95afafaf", \
    "ap-southeast-1-hb.csv" using ($1*250/1000000*17):3 notitle with lines lw 2 dt 2 lc rgb "#95afafaf", \
    "ap-southeast-2-hb.csv" using ($1*250/1000000*17):3 notitle with lines lw 2 dt 2 lc rgb "#95afafaf", \
    "ca-central-1-hb.csv" using ($1*250/1000000*17):3 notitle with lines lw 2 dt 2 lc rgb "#95afafaf", \
    "eu-central-1-hb.csv" using ($1*250/1000000*17):3 notitle with lines lw 2 dt 2 lc rgb "#95afafaf", \
    "eu-north-1-hb.csv" using ($1*250/1000000*17):3 notitle with lines lw 2 dt 2 lc rgb "#95afafaf", \
    "eu-west-1-hb.csv" using ($1*250/1000000*17):3 notitle with lines lw 2 dt 2 lc rgb "#95afafaf", \
    "eu-west-2-hb.csv" using ($1*250/1000000*17):3 notitle with lines lw 2 dt 2 lc rgb "#95afafaf", \
    "eu-west-3-hb.csv" using ($1*250/1000000*17):3 notitle with lines lw 2 dt 2 lc rgb "#95afafaf", \
    "sa-east-1-hb.csv" using ($1*250/1000000*17):3 notitle with lines lw 2 dt 2 lc rgb "#95afafaf", \
    "us-east-1-hb.csv" using ($1*250/1000000*17):3 notitle with lines lw 2 dt 2 lc rgb "#95afafaf", \
    "us-west-1-hb.csv" using ($1*250/1000000*17):3 notitle with lines lw 2 dt 2 lc rgb "#95afafaf", \
    "us-west-2-hb.csv" using ($1*250/1000000*17):3 notitle with lines lw 2 dt 2 lc rgb "#95afafaf", \
    "ap-east-1-dl.csv" using ($1*250/1000000*17):3 notitle with lines lw 2 dt 1 lc rgb "#95afafaf", \
    "ap-northeast-1-dl.csv" using ($1*250/1000000*17):3 notitle with lines lw 2 dt 1 lc rgb "#95afafaf", \
    "ap-northeast-2-dl.csv" using ($1*250/1000000*17):3 notitle with lines lw 2 dt 1 lc rgb "#95afafaf", \
    "ap-southeast-1-dl.csv" using ($1*250/1000000*17):3 notitle with lines lw 2 dt 1 lc rgb "#95afafaf", \
    "ap-southeast-2-dl.csv" using ($1*250/1000000*17):3 notitle with lines lw 2 dt 1 lc rgb "#95afafaf", \
    "ca-central-1-dl.csv" using ($1*250/1000000*17):3 notitle with lines lw 2 dt 1 lc rgb "#95afafaf", \
    "eu-central-1-dl.csv" using ($1*250/1000000*17):3 notitle with lines lw 2 dt 1 lc rgb "#95afafaf", \
    "eu-north-1-dl.csv" using ($1*250/1000000*17):3 notitle with lines lw 2 dt 1 lc rgb "#95afafaf", \
    "eu-west-1-dl.csv" using ($1*250/1000000*17):3 notitle with lines lw 2 dt 1 lc rgb "#95afafaf", \
    "eu-west-2-dl.csv" using ($1*250/1000000*17):3 notitle with lines lw 2 dt 1 lc rgb "#95afafaf", \
    "eu-west-3-dl.csv" using ($1*250/1000000*17):3 notitle with lines lw 2 dt 1 lc rgb "#95afafaf", \
    "sa-east-1-dl.csv" using ($1*250/1000000*17):3 notitle with lines lw 2 dt 1 lc rgb "#95afafaf", \
    "us-east-1-dl.csv" using ($1*250/1000000*17):3 notitle with lines lw 2 dt 1 lc rgb "#95afafaf", \
    "us-west-1-dl.csv" using ($1*250/1000000*17):3 notitle with lines lw 2 dt 1 lc rgb "#95afafaf", \
    "us-west-2-dl.csv" using ($1*250/1000000*17):3 notitle with lines lw 2 dt 1 lc rgb "#95afafaf", \
    "ap-south-1-hb.csv" using ($1*250/1000000*17):3 title "Mumbai HB" with linespoints lw 2 ps 0.5 dt 2 lc 2, \
    "us-east-2-hb.csv" using ($1*250/1000000*17):3 title "Ohio HB" with linespoints lw 2 ps 0.5 dt 2 lc 1, \
    "ap-south-1-dl.csv" using ($1*250/1000000*17):3 title "Mumbai DL" with linespoints lw 2 ps 0.5 dt 1 lc 2, \
    "us-east-2-dl.csv" using ($1*250/1000000*17):3 title "Ohio DL" with linespoints lw 2 ps 0.5 dt 1 lc 1, \
    "ap-south-1-hb.csv" using ($1*250/1000000*17):3:2:4 notitle with yerrorbars lw 1 ps 0 dt 1 lc 2, \
    "us-east-2-hb.csv" using ($1*250/1000000*17):3:2:4 notitle with yerrorbars lw 1 ps 0 dt 1 lc 1, \
    "ap-south-1-dl.csv" using ($1*250/1000000*17):3:2:4 notitle with yerrorbars lw 1 ps 0 dt 1 lc 2, \
    "us-east-2-dl.csv" using ($1*250/1000000*17):3:2:4 notitle with yerrorbars lw 1 ps 0 dt 1 lc 1, \
    NaN with lines title "Other HB" lw 2 dt 2 lc rgb "#95afafaf", \
    NaN with lines title "Other DL" lw 2 dt 1 lc rgb "#95afafaf"
