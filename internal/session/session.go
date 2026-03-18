// Copyright 2026 The MathWorks, Inc.

package session

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"sync"
	"time"
)

const (
	// ActiveClientTimeout is how long a client can go without polling
	// before it's considered disconnected and another client can take over.
	ActiveClientTimeout = 10 * time.Second
)

// Manager handles client concurrency control and idle timeout.
type Manager struct {
	mu                 sync.RWMutex
	activeClientID     string
	activeClientSeen   time.Time
	concurrencyEnabled bool
	idleTimeoutSeconds int
	idleTimeRemaining  int
	shutdownCh         chan struct{}
	resetIdleCh        chan struct{}
	logger             *slog.Logger
}

func NewManager(idleTimeoutMinutes int, logger *slog.Logger) *Manager {
	m := &Manager{
		concurrencyEnabled: true,
		idleTimeoutSeconds: idleTimeoutMinutes * 60,
		shutdownCh:         make(chan struct{}),
		resetIdleCh:        make(chan struct{}, 1),
		logger:             logger,
	}
	if m.idleTimeoutSeconds > 0 {
		m.idleTimeRemaining = m.idleTimeoutSeconds
		go m.runIdleTimer()
	}
	return m
}

// GenerateClientID creates a new unique client identifier.
func GenerateClientID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// SetActiveClient handles a poll from a client. Returns whether this client
// is the active client and should show the MATLAB iframe.
//
// Logic:
//   - If no active client, or the active client has timed out, this client takes over.
//   - If this client IS the active client, update its last-seen timestamp.
//   - If transferSession is true, forcibly take over from the current active client.
//   - Otherwise, this client is inactive (another client owns the session).
func (m *Manager) SetActiveClient(clientID string, transferSession bool) (isActive bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.concurrencyEnabled {
		return true
	}

	now := time.Now()

	// Check if the current active client has timed out
	activeExpired := m.activeClientID != "" &&
		!m.activeClientSeen.IsZero() &&
		now.Sub(m.activeClientSeen) > ActiveClientTimeout

	if activeExpired {
		m.logger.Debug("active client timed out", "oldClient", m.activeClientID)
		m.activeClientID = ""
	}

	// No active client — this one claims ownership
	if m.activeClientID == "" {
		m.activeClientID = clientID
		m.activeClientSeen = now
		return true
	}

	// This client is already the active one
	if m.activeClientID == clientID {
		m.activeClientSeen = now
		return true
	}

	// Transfer requested — take over
	if transferSession {
		m.logger.Info("session transferred", "from", m.activeClientID, "to", clientID)
		m.activeClientID = clientID
		m.activeClientSeen = now
		return true
	}

	// Another client is active
	return false
}

// ClearClient removes the active client if it matches the given ID.
func (m *Manager) ClearClient(clientID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeClientID == clientID {
		m.logger.Debug("active client cleared", "client", clientID)
		m.activeClientID = ""
	}
}

// ConcurrencyEnabled returns whether concurrency control is active.
func (m *Manager) ConcurrencyEnabled() bool {
	return m.concurrencyEnabled
}

// ResetIdleTimer resets the idle timeout counter.
func (m *Manager) ResetIdleTimer() {
	if m.idleTimeoutSeconds <= 0 {
		return
	}
	select {
	case m.resetIdleCh <- struct{}{}:
	default:
	}
}

// IdleTimeRemaining returns seconds until idle shutdown, or 0 if disabled.
func (m *Manager) IdleTimeRemaining() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.idleTimeRemaining
}

// ShutdownCh returns a channel that is closed when idle timeout expires.
func (m *Manager) ShutdownCh() <-chan struct{} {
	return m.shutdownCh
}

func (m *Manager) runIdleTimer() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.resetIdleCh:
			m.mu.Lock()
			m.idleTimeRemaining = m.idleTimeoutSeconds
			m.mu.Unlock()

		case <-ticker.C:
			m.mu.Lock()
			m.idleTimeRemaining--
			remaining := m.idleTimeRemaining
			m.mu.Unlock()

			if remaining <= 0 {
				m.logger.Info("idle timeout expired, triggering shutdown")
				close(m.shutdownCh)
				return
			}
		}
	}
}
