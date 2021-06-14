package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

type Server struct {
	ID       string
	PublicIP string
	PrivateIP string
	Port int
	User     string
	KeyPath string
	Location string
	Tag      string
	Provider string
}

type ServerByLocation []Server

func (s ServerByLocation) Len() int { return len(s) }
func (s ServerByLocation) Swap(i, j int) {s[i], s[j] = s[j], s[i]}
func (s ServerByLocation) Less(i, j int) bool {
	byloc := strings.Compare(s[i].Location, s[j].Location)
	if byloc == 0 {
		return strings.Compare(s[i].ID, s[j].ID) == -1
	} else {
		return byloc == -1
	}
}


func ReadServerInfo(path string) []Server {
	var dt []Server
	f, err := os.Open(path)
	if err != nil {
		fmt.Println("error opening file: ", err)
		os.Exit(1)
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	err = dec.Decode(&dt)
	if err != nil {
		fmt.Println("error decoding json: ", err)
		os.Exit(1)
	}
	return dt
}

func getConfirmation() bool {
	fmt.Print("Are you sure? [y/n] ")
	var input string
	fmt.Scanln(&input)
	if input == "y" || input == "Y" {
		return true
	} else {
		fmt.Println("Aborting")
		return false
	}
}

func dispatchCluster(args []string) {
	command := flag.NewFlagSet("cluster", flag.ExitOnError)
	tag := command.String("t", "", "tag of the servers")
	serverListFilePath := command.String("l", "", "update the server list file")
	yes := command.Bool("y", false, "bypass confirmation for destructive operations")
	serverPrice := command.Float64("p", 80.0, "price of the servers to request")
	loc := command.String("d", "", "filter datacenters to use")
	count := command.Int("n", 1, "number of servers to launch at each datacenter")
	vendor := command.String("vendor", "aws", "cloud vendor to use: aws, vultr")

	// we need one and only one action
	if len(args) < 1 {
		fmt.Println("missing action")
		os.Exit(1)
	}
	action := args[0]

	command.Parse(args[1:])

	if *vendor == "vultr" {
		switch action {
		case "start":
			if *tag == "" {
				fmt.Println("missing tag")
				os.Exit(1)
			}
			if !*yes && !getConfirmation() {
				os.Exit(0)
			}
			s := StartServers(*tag, *serverPrice, *loc, *count)
			if *serverListFilePath != "" {
				DumpServerInfo(*serverListFilePath, s)
				fmt.Printf("Server list written to %v\n", *serverListFilePath)
			}
		case "stop":
			if *tag == "" {
				fmt.Println("missing tag")
				os.Exit(1)
			}
			if !*yes && !getConfirmation() {
				os.Exit(0)
			}
			StopServers(*tag)
		case "reboot":
			if *tag == "" {
				fmt.Println("missing tag")
				os.Exit(1)
			}
			if !*yes && !getConfirmation() {
				os.Exit(0)
			}
			RestartServers(*tag)
		case "status":
			s := CheckServerStatus(*tag)
			if *serverListFilePath != "" {
				DumpServerInfo(*serverListFilePath, s)
				fmt.Printf("Server list written to %v\n", *serverListFilePath)
			}
		default:
			fmt.Println("invalid action")
			os.Exit(1)
		}
	} else if *vendor == "aws" {
		switch action {
		case "start":
			if *tag == "" {
				fmt.Println("missing tag")
				os.Exit(1)
			}
			if !*yes && !getConfirmation() {
				os.Exit(0)
			}
			err := AWSStartInstanceAtRegions(*loc, *count, *tag)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			} else {
				os.Exit(0)
			}
		case "stop":
			if *tag == "" {
				fmt.Println("missing tag")
				os.Exit(1)
			}
			if !*yes && !getConfirmation() {
				os.Exit(0)
			}
			err := AWSStopServersAtAllRegions(*tag)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			} else {
				os.Exit(0)
			}
		case "status":
			s, err := AWSListServersAtAllRegions(*tag)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			if *serverListFilePath != "" {
				AWSDumpServerInfo(*serverListFilePath, s)
				fmt.Printf("Server list written to %v\n", *serverListFilePath)
			}
		default:
			fmt.Println("invalid action")
			os.Exit(1)
		}
	}
}

// restart to make MaxSessions effective
