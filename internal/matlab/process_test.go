// Copyright 2026 The MathWorks, Inc.

package matlab

import (
	"log/slog"
	"testing"

	"github.com/mathworks/matlab-proxy-go/internal/config"
)

func TestStartWithNoMATLABCommand(t *testing.T) {
	cfg := &config.Config{
		MATLABCommand: "",
	}
	logger := slog.New(slog.NewTextHandler(nil, nil))
	p := NewProcess(cfg, logger)

	err := p.Start(false)
	if err == nil {
		t.Fatal("expected error when MATLABCommand is empty, got nil")
	}

	if p.Status() != StatusDown {
		t.Errorf("expected status %q, got %q", StatusDown, p.Status())
	}

	errors := p.Errors()
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errors))
	}
	if errors[0].Type != "MatlabInstallError" {
		t.Errorf("expected error type %q, got %q", "MatlabInstallError", errors[0].Type)
	}
	expectedMsg := "Unable to find MATLAB on the system PATH. Add MATLAB to the system PATH, and restart matlab-proxy."
	if errors[0].Message != expectedMsg {
		t.Errorf("expected error message %q, got %q", expectedMsg, errors[0].Message)
	}
}

func TestStartWithNoMATLABCommandRepeated(t *testing.T) {
	cfg := &config.Config{
		MATLABCommand: "",
	}
	logger := slog.New(slog.NewTextHandler(nil, nil))
	p := NewProcess(cfg, logger)

	// Calling Start multiple times should consistently produce the error.
	for i := 0; i < 3; i++ {
		err := p.Start(false)
		if err == nil {
			t.Fatalf("iteration %d: expected error, got nil", i)
		}
		errors := p.Errors()
		if len(errors) != 1 {
			t.Fatalf("iteration %d: expected 1 error, got %d", i, len(errors))
		}
	}
}

func TestNewProcessDefaultState(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.NewTextHandler(nil, nil))
	p := NewProcess(cfg, logger)

	if p.Status() != StatusDown {
		t.Errorf("expected initial status %q, got %q", StatusDown, p.Status())
	}
	if p.BusyStatus() != nil {
		t.Error("expected nil BusyStatus initially")
	}
	if len(p.Errors()) != 0 {
		t.Errorf("expected no errors initially, got %d", len(p.Errors()))
	}
	if len(p.Warnings()) != 0 {
		t.Errorf("expected no warnings initially, got %d", len(p.Warnings()))
	}
	if p.Connector() != nil {
		t.Error("expected nil Connector initially")
	}
}
