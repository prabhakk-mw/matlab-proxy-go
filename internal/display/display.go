// Copyright 2026 The MathWorks, Inc.

//go:build !windows

package display

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

// Manager handles Xvfb and optional Fluxbox window manager for Linux.
type Manager struct {
	xvfbCmd    *exec.Cmd
	fluxboxCmd *exec.Cmd
	display    string
	logger     *slog.Logger
}

func NewManager(logger *slog.Logger) *Manager {
	return &Manager{logger: logger}
}

func (m *Manager) Display() string {
	return m.display
}

// Start launches Xvfb (and optionally Fluxbox for Simulink).
func (m *Manager) Start(enableSimulink bool) error {
	if _, err := exec.LookPath("Xvfb"); err != nil {
		m.logger.Warn("Xvfb not found, skipping display setup")
		return nil
	}

	displayNum, err := findFreeDisplay()
	if err != nil {
		return fmt.Errorf("finding free display: %w", err)
	}
	m.display = fmt.Sprintf(":%d", displayNum)

	m.xvfbCmd = exec.Command("Xvfb", m.display,
		"-screen", "0", "3840x2160x24",
		"-dpi", "100",
		"-nolisten", "tcp",
	)
	m.xvfbCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	m.xvfbCmd.Stdout = os.Stdout
	m.xvfbCmd.Stderr = os.Stderr

	if err := m.xvfbCmd.Start(); err != nil {
		return fmt.Errorf("starting Xvfb: %w", err)
	}
	m.logger.Info("Xvfb started", "display", m.display, "pid", m.xvfbCmd.Process.Pid)

	if enableSimulink {
		if err := m.startFluxbox(); err != nil {
			m.logger.Warn("failed to start Fluxbox", "error", err)
		}
	}

	return nil
}

func (m *Manager) startFluxbox() error {
	if _, err := exec.LookPath("fluxbox"); err != nil {
		return fmt.Errorf("fluxbox not found: %w", err)
	}

	m.fluxboxCmd = exec.Command("fluxbox")
	m.fluxboxCmd.Env = append(os.Environ(), fmt.Sprintf("DISPLAY=%s", m.display))
	m.fluxboxCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := m.fluxboxCmd.Start(); err != nil {
		return fmt.Errorf("starting fluxbox: %w", err)
	}
	m.logger.Info("Fluxbox started", "pid", m.fluxboxCmd.Process.Pid)
	return nil
}

func (m *Manager) Stop() {
	if m.fluxboxCmd != nil && m.fluxboxCmd.Process != nil {
		m.logger.Info("stopping Fluxbox")
		syscall.Kill(-m.fluxboxCmd.Process.Pid, syscall.SIGTERM)
		m.fluxboxCmd.Wait()
	}
	if m.xvfbCmd != nil && m.xvfbCmd.Process != nil {
		m.logger.Info("stopping Xvfb")
		syscall.Kill(-m.xvfbCmd.Process.Pid, syscall.SIGTERM)
		m.xvfbCmd.Wait()
	}
}

// findFreeDisplay finds an available X display number by checking
// if the corresponding Unix socket is in use.
func findFreeDisplay() (int, error) {
	for d := 99; d < 1000; d++ {
		sockPath := fmt.Sprintf("/tmp/.X11-unix/X%d", d)
		if _, err := os.Stat(sockPath); os.IsNotExist(err) {
			// Also verify the TCP port is free (6000 + display)
			addr := fmt.Sprintf("127.0.0.1:%d", 6000+d)
			ln, err := net.Listen("tcp", addr)
			if err != nil {
				continue
			}
			ln.Close()
			return d, nil
		}
	}
	return 0, fmt.Errorf("no free display number found")
}

// findFreeDisplay helper - convert int to string
func itoa(n int) string {
	return strconv.Itoa(n)
}
