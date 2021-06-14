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

Clone my fork of [quic-go](https://github.com/yangl1996/quic-go) somewhere
on your machine and switch to the `emu-conns` branch.
Then, modify `go.mod`, line 24,
to point to the `quic-go` fork you have just cloned.

```
cd pikad
go build
./pikad -h  # show help text
```

