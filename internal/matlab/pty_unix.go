// Copyright 2026 The MathWorks, Inc.

//go:build linux

package matlab

import (
	"fmt"
	"os"
	"strconv"
	"syscall"
	"unsafe"
)

// openPTY creates a pseudo-terminal pair, returning the master and slave file descriptors.
// This mirrors Python's pty.openpty() used in matlab-proxy.
func openPTY() (master *os.File, slave *os.File, err error) {
	// Open /dev/ptmx to get a master fd
	ptmx, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("opening /dev/ptmx: %w", err)
	}

	// Unlock the slave
	var unlock int
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, ptmx.Fd(), syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlock))); errno != 0 {
		ptmx.Close()
		return nil, nil, fmt.Errorf("unlocking PTY slave: %v", errno)
	}

	// Get the slave PTY number
	var ptyno uint32
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, ptmx.Fd(), syscall.TIOCGPTN, uintptr(unsafe.Pointer(&ptyno))); errno != 0 {
		ptmx.Close()
		return nil, nil, fmt.Errorf("getting PTY number: %v", errno)
	}

	// Open the slave
	slavePath := "/dev/pts/" + strconv.Itoa(int(ptyno))
	slaveFile, err := os.OpenFile(slavePath, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		ptmx.Close()
		return nil, nil, fmt.Errorf("opening slave PTY %s: %w", slavePath, err)
	}

	return ptmx, slaveFile, nil
}
