#!/usr/local/bin/gnuplot

set term pdf size 3.2,2.0
set output "fig.pdf"

set multiplot layout 4,1

unset border
set border 1
set lmargin 0.1
set bmargin 0.2
set tmargin 0.2
set rmargin 0.1
unset xtics
unset ytics

set yrange [0:10]
set xrange [0:9]
unset ylabel 
unset key
set notitle
unset xlabel

set style fill solid 0.7 noborder

plot "fill.dat" using 1:2 notitle with filledcurves x1 lc "#E1CA96", \
     "trace.dat" using 1:2 notitle with lines lw 2 lc "black"
plot "fill.dat" using 1:3 notitle with filledcurves x1 lc "#ACA885", \
     "trace.dat" using 1:3 notitle with lines lw 2 lc "black"
plot "fill.dat" using 1:4 notitle with filledcurves x1 lc "#918B76", \
     "trace.dat" using 1:4 notitle with lines lw 2 lc "black"
plot "fill.dat" using 1:5 notitle with filledcurves x1 lc "#626C66", \
     "trace.dat" using 1:5 notitle with lines lw 2 lc "black"
