package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/tmc/scp"
	"golang.org/x/crypto/ssh"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type RemoteError struct {
	inner   error
	problem string
}

func (e RemoteError) Error() string {
	if e.inner != nil {
		return e.problem + ": " + e.inner.Error()
	} else {
		return e.problem
	}
}

func dispatchBwTest(args []string) {
	command := flag.NewFlagSet("exp", flag.ExitOnError)
	serverListFilePath := command.String("l", "", "path to the server list file")
	outPath := command.String("o", "results", "path for output results")
	overwrite := command.Bool("overwrite", false, "overwrite the result if it is already there")
	forceReinstall := command.Bool("reinstall", false, "force reinstalling the binary on each server")
	durationSecs := command.Int("dur", 600, "duration of the experiment")
	mmlinkfile := command.String("mmlink", "", "mahimahi link file, potentially separated by commas")
	mmdelayms := command.Int("mmdelay", 0, "mahimahi one-way delay")
	nocopylatency := command.Bool("nolatencylog", false, "do not copy back latency log")

	command.Parse(args[0:])

	if *serverListFilePath == "" {
		fmt.Println("missing server list")
		os.Exit(1)
	}
	if *outPath == "" {
		fmt.Println("missing output path")
		os.Exit(1)
	}

	if _, err := os.Stat(*outPath); !os.IsNotExist(err) {
		if *overwrite {
			e := os.RemoveAll(*outPath)
			if e != nil {
				fmt.Printf("error removing existing output: %v\n", e)
				os.Exit(1)
			}
			fmt.Printf("removed existing results at %v\n", *outPath)
		} else {
			fmt.Printf("output path %v already exists\n", *outPath)
			os.Exit(1)
		}
	}

	err := os.MkdirAll(*outPath, 0755)
	if err != nil {
		fmt.Printf("error creating dir at %v: %v\n", *outPath, err)
		os.Exit(1)
	}

	// parse the server list
	servers := ReadServerInfo(*serverListFilePath)

	// dump the server info
	err = copyFile(filepath.Join(*outPath, "servers.json"), *serverListFilePath)
	if err != nil {
		fmt.Printf("error copying server file: %v\n", err)
		os.Exit(1)
	}

	// dump the experiment config
	cf, err := os.Create(filepath.Join(*outPath, "cmdargs.txt"))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer cf.Close()
	flagstr := strings.Join(command.Args(), " ")
	cf.WriteString(flagstr)

	var mmlinkfiles []string
	if *mmlinkfile != "" {
		mmlinkfiles = strings.Split(*mmlinkfile, ",")
	}

	duration := time.Duration(*durationSecs) * time.Second
	BandwidthTest(servers, *outPath, command.Args(), *forceReinstall, duration, mmlinkfiles, *mmdelayms/2, *nocopylatency)
}

func copyFile(out, in string) error {
	i, err := os.Open(in)
	if err != nil {
		return err
	}
	defer i.Close()

	o, err := os.Create(out)
	if err != nil {
		return err
	}
	defer o.Close()

	_, err = io.Copy(o, i)
	if err != nil {
		return err
	}
	return nil
}

func BandwidthTest(servers []Server, outPath string, extraArgs []string, forceReinstall bool, dur time.Duration, mmlinks []string, mmdelayms int, nolatencylog bool) error {
	// assemble the optional args
	pikaFlags := strings.Join(extraArgs, " ")
	fmt.Println("Accepting Pika flags:", pikaFlags)

	// get the connect list
	peerlist := []string{}
	for pid := 0; pid < len(servers); pid++ {
		peer := servers[pid]
		peerlist = append(peerlist, fmt.Sprintf("%d/%s:9000", pid, peer.PublicIP))
	}

	// connect to each of the servers
	connWg := &sync.WaitGroup{}       // wait for the ssh connection
	installWg := &sync.WaitGroup{}    // wait for installation
	experimentWg := &sync.WaitGroup{} // wait for the experiment
	connWg.Add(len(servers))
	installWg.Add(len(servers))
	experimentWg.Add(len(servers))

	// dispatch for each server
	for i, s := range servers {
		go func(i int, s Server) {
			defer connWg.Done()
			// the core logic running on the server
			err := func(idx int, s Server) error {
				client, err := connectSSH(s.User, s.PublicIP, s.Port, s.KeyPath)
				if err != nil {
					return err
				}
				fmt.Printf("Connected to %v\n", s.Location)
				defer client.Close()

				// send the link file
				if len(mmlinks) == 1 {
					err = sendLinkFile(client, mmlinks[0])
					if err != nil {
						return err
					}
				} else if len(mmlinks) > 1 {
					err = sendLinkFile(client, mmlinks[i])
					if err != nil {
						return err
					}
				}

				// check if pika is installed
				if forceReinstall {
					removePika(client)
				}
				installed, err := checkPika(client)
				if err != nil {
					return err
				}
				if !installed {
					// install pika
					err = installPika(client)
					if err != nil {
						return err
					}
					fmt.Printf("Installed pikad on %v\n", s.Location)
				}
				// make sure that pika is not running, and there's no leftover file
				err = killPika(client)
				if err != nil {
					return err
				}
				err = cleanUpPika(client)
				if err != nil {
					return err
				}

				installWg.Done()
				// wait until all servers have installed
				installWg.Wait()

				// start pika and report when the experiment is finished
				logPath := "log-" + strconv.Itoa(i) + ".log"
				logPath = filepath.Join(outPath, logPath)
				latencyPath := "latency-" + strconv.Itoa(i) + ".dat"
				latencyPath = filepath.Join(outPath, latencyPath)
				peerlatencyPath := "peerlatency-" + strconv.Itoa(i) + ".dat"
				peerlatencyPath = filepath.Join(outPath, peerlatencyPath)

				// start copying the files
				ctx, cancel := context.WithCancel(context.Background())
				go channelFile(ctx, client, "/pika/pika.log", logPath)
				//go channelFile(ctx, client, "/pika/latency.dat", latencyPath)
				//go channelFile(ctx, client, "/pika/peerlatency.dat", peerlatencyPath)

				// start the session itself
				hasmmlink := false
				if len(mmlinks) != 0 {
					hasmmlink = true
				}
				sess, err := startPika(client, i, len(servers), (len(servers)-1)/3, strings.Join(peerlist, ","), pikaFlags, s.PrivateIP, mmdelayms, hasmmlink)
				if err != nil {
					return err
				}

				// wait for the duration
				time.Sleep(dur)

				experimentWg.Done()
				// wait until all servers have finished experiment
				experimentWg.Wait()

				// stop the server node and wait for it to exit
				fmt.Printf("Shutting down %v\n", s.Location)

				err = killPika(client)
				if err != nil {
					return err
				}
				//err = sess.Signal(ssh.SIGINT)
				err = sess.Wait()
				// stop copying back files
				cancel()
				// copy back the files
				if !nolatencylog {
					copyBackFile(s, "/pika/latency.dat", latencyPath)
					copyBackFile(s, "/pika/peerlatency.dat", peerlatencyPath)
				}

				fmt.Printf("Server stopped at %v\n", s.Location)

				if err != nil {
					e, is := err.(*ssh.ExitError)
					if !is {
						return err
					} else {
						if e.ExitStatus() != 143 {
							return err
						}
					}
				}
				return nil
			}(i, s)
			if err != nil {
				fmt.Println(err)
			}
		}(i, s)
	}
	// wait for all servers to start
	installWg.Wait()
	fmt.Printf("All servers installed pikad, running pikad\n")

	experimentWg.Wait()
	connWg.Wait()
	return nil
}

func sendLinkFile(c *ssh.Client, source string) error {
	rmsess, err := c.NewSession()
	if err != nil {
		return err
	}
	rmsess.Run("rm -rf /tmp/linkfile*")
	rmsess.Close()

	cpsess, err := c.NewSession()
	if err != nil {
		return err
	}
	err = scp.CopyPath(source, "/tmp/linkfile.gz", cpsess)
	if err != nil {
		return err
	}

	decompsess, err := c.NewSession()
	if err != nil {
		return err
	}
	decompsess.Run("gzip -d /tmp/linkfile.gz && chmod 666 /tmp/linkfile")
	decompsess.Close()
	return nil
}

// TODO: use go-native ssh
func copyBackFile(s Server, from, dest string) error {
	fromStr := fmt.Sprintf("%s@%s:%s", s.User, s.PublicIP, from)
	cmdArgs := []string{"-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null", "-i", s.KeyPath, fromStr, dest}
	proc := exec.Command("scp", cmdArgs...)
	err := proc.Run()
	if err != nil {
		return err
	}
	return nil
}

func killPika(c *ssh.Client) error {
	pkill, err := c.NewSession()
	if err != nil {
		return RemoteError{err, "error creating session"}
	}
	// flush the iptables, kill all mahimahi shells, and kill all pikad
	pkill.Run("sudo pkill pikad && sudo iptables -F -t nat && sudo pkill mm-")
	pkill.Close()
	return nil
}

func cleanUpPika(c *ssh.Client) error {
	rmrf, err := c.NewSession()
	if err != nil {
		return RemoteError{err, "error creating session"}
	}
	rmrf.Run("sudo rm -rf /pika/*.pikadb")
	rmrf.Close()
	rmlog, err := c.NewSession()
	if err != nil {
		return RemoteError{err, "error creating session"}
	}
	rmlog.Run("sudo rm -rf /pika/pika.log")
	rmlog.Close()
	rmlatency, err := c.NewSession()
	if err != nil {
		return RemoteError{err, "error creating session"}
	}
	rmlatency.Run("sudo rm -rf /pika/latency.dat && sudo rm -rf /pika/peerlatency.dat")
	rmlatency.Close()
	return nil
}

func channelFile(ctx context.Context, c *ssh.Client, path, log string) error {
	var lf *os.File
	var ferr error
	lf, ferr = os.Create(log)
	if ferr != nil {
		return ferr
	}
	defer lf.Close()

	s, err := c.NewSession()
	if err != nil {
		return RemoteError{err, "error creating session"}
	}

	// connect to stderr
	output, err := s.StdoutPipe()
	if err != nil {
		return RemoteError{err, "error connecting to stdout"}
	}

	err = s.Start("tail -c +1 -F " + path)
	if err != nil {
		return RemoteError{err, "error cating file"}
	}

	// launch a goroutine to copy the data back to our local file
	res := make(chan struct{})
	go func() {
		io.Copy(lf, output)
		res <- struct{}{}
	}()

	select {
	case <-res:
		// if io.Copy returned by itself, we just return
		return nil
	case <-ctx.Done():
		// if we are told to stop
		s.Signal(ssh.SIGINT)
		<-res
		return nil
	}
	return nil
}

func startPika(c *ssh.Client, id, n, f int, connlist, pikaFlags string, ourip string, mmdelay int, mmlink bool) (*ssh.Session, error) {
	s, err := c.NewSession()
	if err != nil {
		return nil, err
	}
	pikacmd := fmt.Sprintf(`/pika/pikad -s 0.0.0.0:9000 -n %d -f %d -id %d -nodes %s %v &> /pika/pika.log`, n, f, id, connlist, pikaFlags)

	var cmd string
	if mmdelay == 0 && !mmlink {
		cmd = pikacmd
	} else {
		// TODO: we are using infinite queue. Switch to pie or droptail
		cmd = fmt.Sprintf(`sudo su - test -c "sudo iptables -A PREROUTING -t nat -p udp -d %s --dport 9000 -j DNAT --to-destination 100.64.0.2 && mm-delay %d bash -c "\""sudo iptables -A PREROUTING -t nat -p udp -d 100.64.0.2 --dport 9000 -j DNAT --to-destination 100.64.0.4 && mm-link /tmp/linkfile /tmp/linkfile --uplink-queue=droptail --uplink-queue-args='packets=3000' --downlink-queue=droptail --downlink-queue-args='packets=3000' -- /pika/pikad -s 0.0.0.0:9000 -n %d -f %d -id %d -nodes %s %v &> /pika/pika.log"\"`, ourip, mmdelay, n, f, id, connlist, pikaFlags)
	}
	//fmt.Println(cmd)

	// start the server
	err = s.Start(cmd)
	return s, err
}

func checkPika(c *ssh.Client) (bool, error) {
	s, err := c.NewSession()
	if err != nil {
		return false, RemoteError{err, "error creating session"}
	}
	defer s.Close()
	cmd := "/pika/pikad -v"
	err = s.Run(cmd)
	if err != nil {
		return false, nil
	} else {
		return true, nil
	}
}

func removePika(c *ssh.Client) error {
	s, err := c.NewSession()
	if err != nil {
		return RemoteError{err, "error creating session"}
	}
	defer s.Close()
	cmd := "rm -rf /pika/pikad"
	err = s.Run(cmd)
	if err != nil {
		return nil
	} else {
		return nil
	}
}

func installPika(c *ssh.Client) error {
	s, err := c.NewSession()
	if err != nil {
		return RemoteError{err, "error creating session"}
	}
	defer s.Close()
	cmd := "wget 'http://45.63.22.222:8100/pikad' -O /pika/pikad && chmod 777 /pika/pikad"
	err = s.Run(cmd)
	if err != nil {
		return RemoteError{err, "error installing pika"}
	}
	return nil
}
