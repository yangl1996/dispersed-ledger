#!/usr/local/bin/gnuplot

set term pdf size 3.3,2.0
set size ratio 0.618
set output "coupled-throughput-avg.pdf"
set datafile separator ","
#set key outside right
set ylabel "Throughput (MB/s)"
set notitle
unset key
#set title "Confirmation Latency"

stats 'decoupled-dl.dat' using 2 prefix "dldec" output
stats 'coupled-dl.dat' using 2 prefix "dlcou" output
stats 'hb-linking.dat' using 2 prefix "hblink" output
stats 'hb.dat' using 2 prefix "hb" output

set print $final
print sprintf("%f %f %f DL", dldec_mean, dldec_stddev, dldec_up_quartile)
print sprintf("%f %f %f DL-Coupled", dlcou_mean, dlcou_stddev, dlcou_up_quartile)
print sprintf("%f %f %f HB-Link", hblink_mean, hblink_stddev, hblink_up_quartile)
print sprintf("%f %f %f HB", hb_mean, hb_stddev, hb_up_quartile)

set datafile separator " "

set yrange [0:45]
set xrange[0:2]
#column 0 is the row number
plot $final using ($0 * 0.5 + 0.25):1:(0.15) title "No Variation" with boxes linecolor 2 fill solid, \
    $final using ($0 * 0.5 + 0.25):1:2 notitle with yerrorbars lc rgb 'black' pt 1 lw 1 ps 0.5, \
    $final using ($0 * 0.5 + 0.25):(NaN):xtic(4) notitle with boxes
