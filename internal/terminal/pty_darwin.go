// Copyright 2026 The MathWorks, Inc.

//go:build darwin

package terminal

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"unsafe"
)

// startWithPTY starts the command with a PTY as its stdin/stdout/stderr.
// On macOS, posix_openpt is used via /dev/ptmx but the unlock/ptsname
// mechanism differs from Linux.
func startWithPTY(cmd *exec.Cmd) (*os.File, error) {
	ptmx, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("opening /dev/ptmx: %w", err)
	}

	// grantpt + unlockpt via ioctl on macOS
	var u int
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, ptmx.Fd(), uintptr(0x20007461), uintptr(unsafe.Pointer(&u))); errno != 0 {
		// TIOCPTYGRANT = 0x20007461 on macOS
		ptmx.Close()
		return nil, fmt.Errorf("grantpt: %v", errno)
	}
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, ptmx.Fd(), uintptr(0x20007452), 0); errno != 0 {
		// TIOCPTYUNLK = 0x20007452 on macOS
		ptmx.Close()
		return nil, fmt.Errorf("unlockpt: %v", errno)
	}

	// ptsname: TIOCPTYGNAME = 0x40807453 on macOS
	var slaveName [128]byte
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, ptmx.Fd(), uintptr(0x40807453), uintptr(unsafe.Pointer(&slaveName[0]))); errno != 0 {
		ptmx.Close()
		return nil, fmt.Errorf("ptsname: %v", errno)
	}

	slavePath := ""
	for i, b := range slaveName {
		if b == 0 {
			slavePath = string(slaveName[:i])
			break
		}
	}

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
	// TIOCSWINSZ is the same on macOS
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, ptmx.Fd(), syscall.TIOCSWINSZ, uintptr(unsafe.Pointer(&ws)))
	if errno != 0 {
		return fmt.Errorf("TIOCSWINSZ: %v", errno)
	}
	return nil
}
