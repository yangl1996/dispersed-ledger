# DispersedLedger

Golang implementation of DispersedLedger.

## Paper

__DispersedLedger: High-Throughput Byzantine Consensus on Variable Bandwidth Networks__

Lei Yang, Seo Jin Park, Mohammad Alizadeh, Sreeram Kannan, David Tse

## This repository

- `pika`: DispersedLedger protocol implemented as an IO automaton.
- `pikad`: DispersedLedger node.
- `pikad/pikaperf`: Real-time performance monitor for `pikad`.
- `testbed`: Testbed controller (in Golang) that controls cloud instances and automates experiments.
- `emulator`: Local emulator used for testing, not working anymore.
- `paper`: Data and Gnuplot scripts for the figures in the paper.

## Build

```
git submodule init
git submodule update  # fetch quic-go with adjustable cubic aggressiveness
cd pikad
go build
./pikad -h  # show help text
```

