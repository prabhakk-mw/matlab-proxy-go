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

// Supported reports whether the web terminal is available on this platform.
func Supported() bool { return true }

// ptySession wraps a PTY master and child process on macOS.
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

	// grantpt + unlockpt via ioctl on macOS
	var u int
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, ptmx.Fd(), uintptr(0x20007461), uintptr(unsafe.Pointer(&u))); errno != 0 {
		ptmx.Close()
		return nil, fmt.Errorf("grantpt: %v", errno)
	}
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, ptmx.Fd(), uintptr(0x20007452), 0); errno != 0 {
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
