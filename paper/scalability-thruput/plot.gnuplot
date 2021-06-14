#!/usr/local/bin/gnuplot

set term pdf size 3.3,2.0
set size ratio 0.618
set output "scalability-thruput.pdf"
#set key outside right
set key right top
set ylabel "Throughput (MB/s)"
set xlabel "Cluster Size"
set notitle
set datafile separator ","
#set title "Confirmation Latency"

stats '128-500.csv' using 2 prefix "t_128_500" output
stats '128-1000.csv' using 2 prefix "t_128_1000" output
stats '64-500.csv' using 2 prefix "t_64_500" output
stats '64-1000.csv' using 2 prefix "t_64_1000" output
stats '32-500.csv' using 2 prefix "t_32_500" output
stats '32-1000.csv' using 2 prefix "t_32_1000" output
stats '16-500.csv' using 2 prefix "t_16_500" output
stats '16-1000.csv' using 2 prefix "t_16_1000" output

set print $avgbw500
print sprintf("16,%f,%f", t_16_500_mean, t_16_500_stddev)
print sprintf("32,%f,%f", t_32_500_mean, t_32_500_stddev)
print sprintf("64,%f,%f", t_64_500_mean, t_64_500_stddev)
print sprintf("128,%f,%f", t_128_500_mean, t_128_500_stddev)

set print $avgbw1000
print sprintf("16,%f,%f", t_16_1000_mean, t_16_1000_stddev)
print sprintf("32,%f,%f", t_32_1000_mean, t_32_1000_stddev)
print sprintf("64,%f,%f", t_64_1000_mean, t_64_1000_stddev)
print sprintf("128,%f,%f", t_128_1000_mean, t_128_1000_stddev)

set yrange [0:5]
set xrange [0:150]
set xtics (16, 32, 64, 128)

#column 0 is the row number
plot $avgbw500 using 1:2 title "500 KB" with linespoints lw 2 ps 0.7 lc 1, \
     $avgbw500 using 1:2:3 notitle with yerrorbars ps 0 lc 1, \
     $avgbw1000 using 1:2 title "1 MB" with linespoints lw 2 ps 0.7 lc 2, \
     $avgbw1000 using 1:2:3 notitle with yerrorbars ps 0 lc 2
