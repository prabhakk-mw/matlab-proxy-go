// Copyright 2026 The MathWorks, Inc.

//go:build linux

package terminal

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"unsafe"
)

// startWithPTY starts the command with a PTY as its stdin/stdout/stderr.
// Returns the master end of the PTY for reading/writing.
func startWithPTY(cmd *exec.Cmd) (*os.File, error) {
	ptmx, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("opening /dev/ptmx: %w", err)
	}

	// Unlock the slave
	var unlock int
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, ptmx.Fd(), syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlock))); errno != 0 {
		ptmx.Close()
		return nil, fmt.Errorf("unlocking PTY slave: %v", errno)
	}

	// Get the slave PTY number
	var ptyno uint32
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, ptmx.Fd(), syscall.TIOCGPTN, uintptr(unsafe.Pointer(&ptyno))); errno != 0 {
		ptmx.Close()
		return nil, fmt.Errorf("getting PTY number: %v", errno)
	}

	slavePath := fmt.Sprintf("/dev/pts/%d", ptyno)
	slave, err := os.OpenFile(slavePath, os.O_RDWR, 0)
	if err != nil {
		ptmx.Close()
		return nil, fmt.Errorf("opening slave PTY %s: %w", slavePath, err)
	}

	cmd.Stdin = slave
	cmd.Stdout = slave
	cmd.Stderr = slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}

	if err := cmd.Start(); err != nil {
		slave.Close()
		ptmx.Close()
		return nil, fmt.Errorf("starting command: %w", err)
	}

	// Slave is now owned by the child process; close our copy.
	slave.Close()

	return ptmx, nil
}

// resizePTY sets the terminal window size on the PTY master.
func resizePTY(ptmx *os.File, cols, rows int) error {
	ws := struct {
		Row    uint16
		Col    uint16
		Xpixel uint16
		Ypixel uint16
	}{
		Row: uint16(rows),
		Col: uint16(cols),
	}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, ptmx.Fd(), syscall.TIOCSWINSZ, uintptr(unsafe.Pointer(&ws)))
	if errno != 0 {
		return fmt.Errorf("TIOCSWINSZ: %v", errno)
	}
	return nil
}
