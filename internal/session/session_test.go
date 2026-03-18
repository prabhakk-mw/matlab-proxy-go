// Copyright 2026 The MathWorks, Inc.

package session

import (
	"log/slog"
	"os"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestSetActiveClient_FirstClientClaims(t *testing.T) {
	m := NewManager(0, testLogger())
	if !m.SetActiveClient("client-1", false) {
		t.Error("first client should claim ownership")
	}
}

func TestSetActiveClient_SameClientRefreshes(t *testing.T) {
	m := NewManager(0, testLogger())
	m.SetActiveClient("client-1", false)
	if !m.SetActiveClient("client-1", false) {
		t.Error("same client should remain active")
	}
}

func TestSetActiveClient_SecondClientBlocked(t *testing.T) {
	m := NewManager(0, testLogger())
	m.SetActiveClient("client-1", false)
	if m.SetActiveClient("client-2", false) {
		t.Error("second client should be blocked")
	}
}

func TestSetActiveClient_TransferSession(t *testing.T) {
	m := NewManager(0, testLogger())
	m.SetActiveClient("client-1", false)
	if !m.SetActiveClient("client-2", true) {
		t.Error("transfer should succeed")
	}
	// Original client should now be blocked
	if m.SetActiveClient("client-1", false) {
		t.Error("original client should be blocked after transfer")
	}
}

func TestSetActiveClient_ExpiredClientReplaced(t *testing.T) {
	m := NewManager(0, testLogger())
	m.SetActiveClient("client-1", false)

	// Manually backdate the last-seen timestamp
	m.mu.Lock()
	m.activeClientSeen = time.Now().Add(-ActiveClientTimeout - time.Second)
	m.mu.Unlock()

	if !m.SetActiveClient("client-2", false) {
		t.Error("new client should take over after timeout")
	}
}

func TestClearClient(t *testing.T) {
	m := NewManager(0, testLogger())
	m.SetActiveClient("client-1", false)
	m.ClearClient("client-1")

	// Another client should now be able to claim
	if !m.SetActiveClient("client-2", false) {
		t.Error("client-2 should claim after client-1 cleared")
	}
}

func TestClearClient_WrongID(t *testing.T) {
	m := NewManager(0, testLogger())
	m.SetActiveClient("client-1", false)
	m.ClearClient("client-999") // wrong ID, should be no-op

	// client-1 should still be active
	if !m.SetActiveClient("client-1", false) {
		t.Error("client-1 should still be active")
	}
	if m.SetActiveClient("client-2", false) {
		t.Error("client-2 should still be blocked")
	}
}

func TestConcurrencyDisabled(t *testing.T) {
	m := NewManager(0, testLogger())
	m.concurrencyEnabled = false

	if !m.SetActiveClient("client-1", false) {
		t.Error("should always return true when concurrency disabled")
	}
	if !m.SetActiveClient("client-2", false) {
		t.Error("should always return true when concurrency disabled")
	}
}

func TestConcurrencyEnabled(t *testing.T) {
	m := NewManager(0, testLogger())
	if !m.ConcurrencyEnabled() {
		t.Error("expected ConcurrencyEnabled() = true by default")
	}
}

func TestGenerateClientID(t *testing.T) {
	id1 := GenerateClientID()
	id2 := GenerateClientID()
	if id1 == id2 {
		t.Error("generated IDs should be unique")
	}
	if len(id1) != 32 { // 16 bytes hex-encoded
		t.Errorf("expected 32-char ID, got %d", len(id1))
	}
}

func TestIdleTimeout_Countdown(t *testing.T) {
	// Use 1-minute timeout (60 seconds)
	m := NewManager(1, testLogger())
	defer close(m.shutdownCh) // prevent goroutine leak if test fails early

	remaining := m.IdleTimeRemaining()
	if remaining != 60 {
		t.Errorf("expected 60 seconds remaining, got %d", remaining)
	}
}

func TestIdleTimeout_Disabled(t *testing.T) {
	m := NewManager(0, testLogger())
	if m.IdleTimeRemaining() != 0 {
		t.Errorf("expected 0 remaining when disabled, got %d", m.IdleTimeRemaining())
	}
}

func TestIdleTimeout_Reset(t *testing.T) {
	m := NewManager(1, testLogger())

	// Wait a bit so countdown decrements
	time.Sleep(2 * time.Second)

	before := m.IdleTimeRemaining()
	m.ResetIdleTimer()

	// Give the timer goroutine time to process the reset
	time.Sleep(100 * time.Millisecond)

	after := m.IdleTimeRemaining()
	if after <= before {
		t.Errorf("reset should restore remaining time: before=%d, after=%d", before, after)
	}
}
