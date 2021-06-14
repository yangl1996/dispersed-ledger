package main

import (
	"fmt"
	"os"
)

func main() {
	// dispatch subcommands
	if len(os.Args) <= 1 {
		printAvailableCommands()
		os.Exit(1)
	}
	switch os.Args[1] {
	case "show":
		dispatchShow(os.Args[2:])
		return
	case "latency":
		dispatchLatency(os.Args[2:])
		return
	default:
		printAvailableCommands()
		os.Exit(1)
	}
}

func printAvailableCommands() {
	fmt.Println("available commands: show, latency")
}
