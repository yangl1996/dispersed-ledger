package main

import (
	"bufio"
	"golang.org/x/crypto/ssh"
	"io"
	"io/ioutil"
	"strconv"
	"strings"
)

type DataPoint struct {
	StartTime   int
	Sender      string // the location of the sender (client)
	Receiver    string
	Bps         float64
	TotalBytes  int
	Duration    float64
	Retransmits int
	MaxRTT      int
	MinRTT      int
	MeanRTT     int
}

type IperfOutput struct {
	Start struct {
		Timestamp struct {
			Timesecs int
		}
	}
	End struct {
		Streams []struct {
			Sender struct {
				Bytes           int
				Seconds         float64
				Bits_per_second float64
				Retransmits     int
				Max_rtt         int
				Min_rtt         int
				Mean_rtt        int
			}
		}
	}
}

func killIperf(c *ssh.Client) error {
	pkill, err := c.NewSession()
	if err != nil {
		return RemoteError{err, "error creating session"}
	}
	pkill.Run("pkill iperf3")
	pkill.Close()
	return nil
}

func startIperfServer(c *ssh.Client, n int) error {
	err := killIperf(c)
	if err != nil {
		return err
	}

	for idx := 0; idx < n; idx++ {
		s, err := c.NewSession()
		if err != nil {
			return RemoteError{err, "error creating session"}
		}
		cmd := "iperf3 -s --forceflush -p " + strconv.Itoa(5201+idx) // forceflush or they won't output anything
		// connect to stdout
		stdout, err := s.StdoutPipe()
		if err != nil {
			return RemoteError{err, "error connecting to stdout"}
		}
		// start the server
		err = s.Start(cmd)
		if err != nil {
			return RemoteError{err, "error starting iperf server"}
		}
		// wait for iperf to start
		scanner := bufio.NewScanner(stdout)
		found := false
		for scanner.Scan() {
			t := scanner.Text()
			if strings.Contains(t, "Server listening") {
				found = true
				break
			}
		}
		err = scanner.Err()
		if err != nil {
			return RemoteError{err, "error reading stdout"}
		}
		// start a goroutine that drains the stdout
		go func() {
			io.Copy(ioutil.Discard, stdout)
		}()
		if !found {
			return RemoteError{nil, "iperf server crashed"}
		}
	}
	return nil
}

func installIPerf(c *ssh.Client) error {
	s, err := c.NewSession()
	if err != nil {
		return RemoteError{err, "error creating session"}
	}
	defer s.Close()
	cmd := "apt-get install -y iperf3"
	err = s.Run(cmd)
	if err != nil {
		return RemoteError{err, "error installing iperf3"}
	}
	return nil
}

func checkIPerf(c *ssh.Client) (bool, error) {
	s, err := c.NewSession()
	if err != nil {
		return false, RemoteError{err, "error creating session"}
	}
	defer s.Close()
	cmd := "iperf3 -v"
	err = s.Run(cmd)
	if err != nil {
		return false, nil
	} else {
		return true, nil
	}
}
