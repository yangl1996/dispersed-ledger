package main

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// we want to set tcp_notsent_lowat to cap the max. unsent data in the kernel buffer, so that the
// send buffer closely tracks the BDP
// https://rohanverma.net/blog/2019/01/08/setting-so_reuseport-and-similar-socket-options-in-go-1-11/
// ^ reference on how to set socket option
func setNotSentLowAt(network, address string, c syscall.RawConn) error {
	var opErr error
	err := c.Control(func(fd uintptr) {
		// set the opt
		err := unix.SetsockoptInt(int(fd), unix.IPPROTO_TCP, unix.TCP_NOTSENT_LOWAT, 16384)
		if err != nil {
			opErr = err
			return
		}
		// check if the opt is set
		optval, err := unix.GetsockoptInt(int(fd), unix.IPPROTO_TCP, unix.TCP_NOTSENT_LOWAT)
		if optval != 16384 {
			panic("unable to set socket opt")
		}
		opErr = err
	})
	if err != nil {
		return err
	}
	return opErr
}
