#!/usr/local/bin/gnuplot

set term pdf size 4.5,1.6 font ",14"
#set size ratio 0.618
set output "avid-comparison.pdf"
set key outside right top
set ylabel "Per-node Download /\nBlock Size"
set xlabel "Number of Nodes"
set notitle
#set title "Confirmation Latency"
set xrange [4:140]
set yrange [0:3]
#set logscale y
set grid ytics mytics

# b in and out are KB
m(n, f, b) = (b * 1024 / (n - 2 * f) + log(n) / log(2) * 32 + (2 * n + 1) * 32) / 1024
fp(n, f, b) = (b * 1024 / (n - 2 * f) + (2 * n + 1) * (32 * n + 16 * (n - 2 * f))) / 1024
opt(n, f, b) = (b * 1024 / (n - 2 * f)) / 1024
lb(n, f, b) = b

print m(128, 42, 1000) / lb(128, 42, 1000)
print fp(128, 42, 1000)
print fp(128, 42, 1000) / lb(128, 42, 1000)

#column 0 is the row number
#plot m(16, 5, x)/lb(16, 5, x) title "AVID-M, N=16" with lines lc 1 lw 1.5 dt 1, \
#     fp(16, 5, x)/lb(16, 5, x) title "AVID-FP, N=16" with lines lc 1 lw 1.5 dt 2, \
#     m(127, 42, x)/lb(127, 42, x) title "AVID-M, N=127" with lines lc 2 lw 1.5 dt 1, \
#     fp(127, 42, x)/lb(127, 42, x) title "AVID-FP, N=127" with lines lc 2 lw 1.5 dt 2
plot m(x, x/3, 100)/lb(x, x/3, 100) title "AVID-M, |B|=100 KB" with lines lc 1 lw 1.7 dt 1, \
     m(x, x/3, 1024)/lb(x, x/3, 1024) title "AVID-M, |B|=1 MB" with lines lc 2 lw 1.7 dt 1, \
     fp(x, x/3, 100)/lb(x, x/3, 100) title "AVID-FP, |B|=100 KB" with lines lc 1 lw 1.7 dt 2, \
     fp(x, x/3, 1024)/lb(x, x/3, 1024) title "AVID-FP, |B|=1 MB" with lines lc 2 lw 1.7 dt 2, \
     opt(x, x/3, 1024)/lb(x, x/3, 1024) title "Lowerbound" with lines lc rgb "#30aa0033" lw 1 dt 1
