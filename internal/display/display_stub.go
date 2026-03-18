// Copyright 2026 The MathWorks, Inc.

//go:build !linux

package display

import "log/slog"

// Manager is a no-op on non-Linux platforms (no Xvfb needed).
type Manager struct {
	logger *slog.Logger
}

func NewManager(logger *slog.Logger) *Manager {
	return &Manager{logger: logger}
}

func (m *Manager) Display() string { return "" }

func (m *Manager) Start(enableSimulink bool) error {
	return nil
}

func (m *Manager) Stop() {}
