package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/vultr/govultr"
	"os"
	"strconv"
	"sync"
	"strings"
	"sort"
)

var APIKey = os.Getenv("VULTR_KEY")

func DumpServerInfo(path string, servers []govultr.Server) {
	usrhome, err := os.UserHomeDir()
	if err != nil {
		fmt.Println("error obtaining user home: ", err)
		os.Exit(1)
	}
	keypath := usrhome + "/.ssh/vultr"

	var dt []Server
	for _, s := range servers {
		dt = append(dt, Server{
			ID:       s.InstanceID,
			PrivateIP:       s.MainIP,
			PublicIP:       s.MainIP,
			Location: s.Location,
			Tag:      s.Tag,
			Provider: "vultr",
			User: "root",
			KeyPath: keypath,
			Port: 22,
		})
	}
	sort.Sort(ServerByLocation(dt))
	f, err := os.Create(path)
	if err != nil {
		fmt.Println("error creating file: ", err)
		os.Exit(1)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "    ")
	enc.Encode(dt)
}

func CheckServerStatus(tag string) []govultr.Server {
	// start the client
	c := govultr.NewClient(nil, APIKey)

	// query the servers by tag or simply list them all
	var servers []govultr.Server
	var err error
	if tag == "" {
		servers, err = c.Server.List(context.Background())
	} else {
		servers, err = c.Server.ListByTag(context.Background(), tag)
	}
	if err != nil {
		fmt.Println("error listing servers: ", err)
		os.Exit(1)
	}
	nActive := 0
	nPending := 0
	nOK := 0
	nInstalling := 0
	for _, s := range servers {
		switch s.Status {
		case "active":
			nActive += 1
			switch s.ServerState {
			case "ok":
				nOK += 1
			case "installingbooting":
				nInstalling += 1
			}
		case "pending":
			nPending += 1
		}
	}
	fmt.Printf("%v total, %v active (%v ok, %v installing), %v pending\n", len(servers), nActive, nOK, nInstalling, nPending)
	return servers
}

// StartServers starts one server on each datacenter location. It associates the servers with
// the given tag.
func StartServers(tag string, price float64, location string, count int) []govultr.Server {
	// start the client
	c := govultr.NewClient(nil, APIKey)

	// check the list of vc2 instance types and get the one that costs $5 per month
	plans, err := c.Plan.GetVc2List(context.Background())
	if err != nil {
		fmt.Println("error listing available plans: ", err)
		os.Exit(1)
	}
	var plan govultr.VCPlan
	for _, v := range plans {
		p, err := strconv.ParseFloat(v.Price, 64)
		if err != nil {
			fmt.Println("invalid plan price: ", err)
			os.Exit(1)
		}
		if p == price {
			plan = v
			break
		}
	}
	if plan.PlanID == "" {
		fmt.Println("unable to find the plan")
		os.Exit(1)
	}
	planID, err := strconv.Atoi(plan.PlanID)
	if err != nil {
		fmt.Println("invalid plan id: ", err)
		os.Exit(1)
	}

	// get the list of regions that supports the desired VC2 instance
	regions, err := c.Region.List(context.Background())
	if err != nil {
		fmt.Println("error listing available regions: ", err)
		os.Exit(1)
	}
	var rids []int
	for _, v := range regions {
		rid, err := strconv.Atoi(v.RegionID)
		if err != nil {
			fmt.Println("invalid region id: ", err)
			os.Exit(1)
		}
		availableTypes, err := c.Region.Vc2Availability(context.Background(), rid)
		if err != nil {
			fmt.Println("error listing supported plans: ", err)
			os.Exit(1)
		}
		// look for the plan
		found := false
		for _, t := range availableTypes {
			if t == planID {
				found = true
				break
			}
		}
		if found {
			// if we are matching against location
			if location != "" {
				loc := strings.ToLower(v.Name)
				q := strings.ToLower(location)
				if !strings.Contains(loc, q) {
					continue
				}
			}
			rids = append(rids, rid)
		}
	}
	fmt.Printf("Found %v regions that support the $%.1f plan\n", len(rids), price)

	// get the ssh key
	sshkey, err := c.SSHKey.List(context.Background())
	if err != nil {
		fmt.Println("error listing ssh keys: ", err)
		os.Exit(1)
	}
	kid := ""
	for _, v := range sshkey {
		if v.Name == "macOS Testbed" {
			kid = v.SSHKeyID
			break
		}
	}
	if kid == "" {
		fmt.Println("unable to find the ssh key")
		os.Exit(1)
	}
	fmt.Printf("Using SSH Key %v\n", kid)

	// get the OS
	oses, err := c.OS.List(context.Background())
	if err != nil {
		fmt.Println("error listing os: ", err)
		os.Exit(1)
	}
	osid := 0
	for _, v := range oses {
		if v.Name == "Ubuntu 20.04 x64" {
			osid = v.OsID
			break
		}
	}
	if osid == 0 {
		fmt.Println("unable to find the os")
		os.Exit(1)
	}

	// get the startup script
	scripts, err := c.StartupScript.List(context.Background())
	if err != nil {
		fmt.Println("error listing scripts: ", err)
		os.Exit(1)
	}
	sid := ""
	for _, v := range scripts {
		if v.Name == "increase-sshmaxsessions" {
			sid = v.ScriptID
			break
		}
	}
	if sid == "" {
		fmt.Println("unable to find startup script")
		os.Exit(1)
	}

	// start the servers with the given tag
	opts := &govultr.ServerOptions{
		Tag:       tag,
		SSHKeyIDs: []string{kid},
		ScriptID:  sid,
	}
	var servers []govultr.Server
	for _, r := range rids {
		for t := 0; t < count; t++ {
			s, err := c.Server.Create(context.Background(), r, planID, osid, opts)
			if err != nil {
				fmt.Println("error creating server: ", err)
				os.Exit(1)
			}
			servers = append(servers, *s)
		}
	}

	fmt.Printf("Launched %v servers\n", len(servers))
	return servers
}

func StopServers(tag string) {
	// start the client
	c := govultr.NewClient(nil, APIKey)

	// get the list of servers matching the tag
	servers, err := c.Server.ListByTag(context.Background(), tag)
	if err != nil {
		fmt.Println("error listing servers by tag: ", err)
		os.Exit(1)
	}

	// shut down the servers
	for _, s := range servers {
		// first check the state of the server. we can only shutdown when the server is "ok"
		if s.Status == "pending" || s.Status == "closed" {
			fmt.Printf("Skipping %v due to status %v\n", s.InstanceID, s.Status)
			continue
		}
		if s.ServerState == "locked" {
			fmt.Printf("Skipping %v due to state %v\n", s.InstanceID, s.ServerState)
			continue
		}
		err = c.Server.Delete(context.Background(), s.InstanceID)
		if err != nil {
			fmt.Println("error deleting server: ", err)
			os.Exit(1)
		}
	}

	// check the remaining number of servers
	servers, err = c.Server.List(context.Background())
	if err != nil {
		fmt.Println("error listing servers: ", err)
		os.Exit(1)
	}
	fmt.Printf("%v servers remaining\n", len(servers))
}

func RestartServers(tag string) {
	// start the client
	c := govultr.NewClient(nil, APIKey)

	// get the list of servers matching the tag
	servers, err := c.Server.ListByTag(context.Background(), tag)
	if err != nil {
		fmt.Println("error listing servers by tag: ", err)
		os.Exit(1)
	}

	//reboot the servers
	wg := &sync.WaitGroup{}
	for _, s := range servers {
		wg.Add(1)
		go func(s govultr.Server) {
			defer wg.Done()
			// start a new client
			c := govultr.NewClient(nil, APIKey)
			// first check the state of the server. we can only shutdown when the server is "ok"
			if s.Status == "pending" || s.Status == "closed" {
				fmt.Printf("Skipping %v due to status %v\n", s.InstanceID, s.Status)
				return
			}
			if s.ServerState == "locked" {
				fmt.Printf("Skipping %v due to state %v\n", s.InstanceID, s.ServerState)
				return
			}
			fmt.Println("Rebooting ", s.InstanceID)
			err = c.Server.Reboot(context.Background(), s.InstanceID)
			if err != nil {
				fmt.Println("error rebooting server: ", err)
				os.Exit(1)
			}
		}(s)
	}
	wg.Wait()

	fmt.Println("Servers rebooted")
}

