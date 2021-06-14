#!/usr/local/bin/gnuplot

set term pdf size 3,2 font ",15"
set size ratio 0.618
set output "synthetic-thruput-temporal.pdf"
#set key outside right
#set key right top
set key outside top center horizontal
set ylabel "Throughput (MB/s)"
set notitle
#set title "Confirmation Latency"

stats 'new-median.dat' using 1 prefix "new" output
stats 'hblinking-avg.dat' using 1 prefix "hblinking" output
stats 'hb-avg.dat' using 1 prefix "hb" output
stats 'new-novariation.dat' using 1 prefix "new_novar" output
stats 'hblinking-novariation.dat' using 1 prefix "hblinking_novar" output
stats 'hb-novariation.dat' using 1 prefix "hb_novar" output

set print $variation
print sprintf("%f %f %f DL", new_mean, new_stddev, new_up_quartile)
print sprintf("%f %f %f HB-Link", hblinking_mean, hblinking_stddev, hblinking_up_quartile)
print sprintf("%f %f %f HB", hb_mean, hb_stddev, hb_up_quartile)

set print $novar
print sprintf("%f %f %f DL", new_novar_mean, new_novar_stddev, new_novar_up_quartile)
print sprintf("%f %f %f HB-Link", hblinking_novar_mean, hblinking_novar_stddev, hblinking_novar_up_quartile)
print sprintf("%f %f %f HB", hb_novar_mean, hb_novar_stddev, hb_novar_up_quartile)

set yrange [0:4]
set xrange[0:4.3]
#column 0 is the row number
plot $novar using ($0 * 1.5 + 0.45):1:(0.4) title "No Variation" with boxes fill solid, \
    $novar using ($0 * 1.5 + 0.45):1:2 notitle with yerrorbars lc rgb 'black' pt 1 lw 1 ps 0.5, \
    $variation using ($0 * 1.5 + 0.45 + 0.5):1:(0.4) title "With Variation" with boxes fill solid, \
    $variation using ($0 * 1.5 + 0.45 + 0.5):1:2 notitle with yerrorbars lc rgb 'black' pt 1 lw 1 ps 0.5, \
    $novar using ($0 * 1.5 + 0.65):(NaN):xtic(4) notitle with boxes
