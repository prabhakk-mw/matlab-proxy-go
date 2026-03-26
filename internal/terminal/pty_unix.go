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

// Supported reports whether the web terminal is available on this platform.
func Supported() bool { return true }

// ptySession wraps a PTY master and child process on Linux.
type ptySession struct {
	ptmx *os.File
	cmd  *exec.Cmd
}

func newPTYSession(shell string) (*ptySession, error) {
	cmd := exec.Command(shell)
	cmd.Env = os.Environ()

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

	slave.Close()
	return &ptySession{ptmx: ptmx, cmd: cmd}, nil
}

func (s *ptySession) Read(p []byte) (int, error)  { return s.ptmx.Read(p) }
func (s *ptySession) Write(p []byte) (int, error) { return s.ptmx.Write(p) }
func (s *ptySession) Wait() error                 { return s.cmd.Wait() }
func (s *ptySession) Kill() error                  { return s.cmd.Process.Kill() }
func (s *ptySession) Pid() int                     { return s.cmd.Process.Pid }
func (s *ptySession) Close() error                 { return s.ptmx.Close() }

func (s *ptySession) Resize(cols, rows int) error {
	ws := struct {
		Row    uint16
		Col    uint16
		Xpixel uint16
		Ypixel uint16
	}{
		Row: uint16(rows),
		Col: uint16(cols),
	}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, s.ptmx.Fd(), syscall.TIOCSWINSZ, uintptr(unsafe.Pointer(&ws)))
	if errno != 0 {
		return fmt.Errorf("TIOCSWINSZ: %v", errno)
	}
	return nil
}
