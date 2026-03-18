// Copyright 2026 The MathWorks, Inc.

package licensing

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mathworks/matlab-proxy-go/internal/config"
)

// Type represents the licensing method.
type Type string

const (
	TypeNone     Type = ""
	TypeNLM      Type = "nlm"
	TypeMHLM     Type = "mhlm"
	TypeExisting Type = "existing_license"
)

// Info holds the current licensing configuration.
type Info struct {
	Type           Type   `json:"type"`
	ConnectionStr  string `json:"conn_str,omitempty"`         // NLM
	IdentityToken  string `json:"identity_token,omitempty"`   // MHLM
	SourceID       string `json:"source_id,omitempty"`        // MHLM
	Expiry         string `json:"expiry,omitempty"`           // MHLM
	EmailAddr      string `json:"email_addr,omitempty"`       // MHLM
	EntitlementID  string `json:"entitlement_id,omitempty"`   // MHLM
	Entitlements   []Entitlement `json:"entitlements,omitempty"` // MHLM
	FirstName      string `json:"first_name,omitempty"`       // MHLM
	LastName       string `json:"last_name,omitempty"`        // MHLM
	DisplayName    string `json:"display_name,omitempty"`     // MHLM
	UserID         string `json:"user_id,omitempty"`          // MHLM
	ProfileID      string `json:"profile_id,omitempty"`       // MHLM
}

// Entitlement represents a MATLAB license entitlement.
type Entitlement struct {
	ID      string `json:"id"`
	Label   string `json:"label,omitempty"`
	License string `json:"license_number,omitempty"`
}

// Manager handles licensing state and persistence.
type Manager struct {
	mu      sync.RWMutex
	info    *Info
	cfg     *config.Config
	dataDir string
}

func NewManager(cfg *config.Config) *Manager {
	return &Manager{
		cfg:     cfg,
		dataDir: cfg.DataDir,
	}
}

// Init initializes licensing from environment variables or cached config.
func (m *Manager) Init() error {
	m.mu.Lock()

	// Check env vars in priority order
	if m.cfg.UseExistingLicense {
		m.info = &Info{Type: TypeExisting}
		m.mu.Unlock()
		return nil
	}

	if m.cfg.NLMConnStr != "" {
		m.info = &Info{Type: TypeNLM, ConnectionStr: m.cfg.NLMConnStr}
		m.mu.Unlock()
		return nil
	}

	// Try loading cached config
	cached, err := m.loadCachedConfig()
	if err == nil && cached != nil {
		switch cached.Type {
		case TypeMHLM:
			if !m.isMHLMTokenValid(cached) {
				m.removeCachedConfig()
				m.mu.Unlock()
				return nil
			}
			m.info = cached
			m.mu.Unlock()
			// Re-fetch entitlements since the access token is short-lived.
			// Lock is released because fetchAndSetEntitlements acquires it.
			if err := m.fetchAndSetEntitlements(); err != nil {
				m.mu.Lock()
				m.info = nil
				m.removeCachedConfig()
				m.mu.Unlock()
				return fmt.Errorf("refreshing MHLM entitlements from cache: %w", err)
			}
			return nil

		case TypeNLM:
			m.info = cached
			// Propagate cached NLM connection string back to config so that
			// the MATLAB process gets -licmode file and MLM_LICENSE_FILE
			if cached.ConnectionStr != "" {
				m.cfg.NLMConnStr = cached.ConnectionStr
			}

		case TypeExisting:
			m.info = cached

		default:
			m.removeCachedConfig()
		}
	}

	m.mu.Unlock()
	return nil
}

// isMHLMTokenValid checks if a cached MHLM token has more than 1 hour remaining.
// MathWorks returns expiry in the format "2026-03-18T12:00:00.000+0000".
func (m *Manager) isMHLMTokenValid(info *Info) bool {
	if info.Expiry == "" || info.IdentityToken == "" {
		return false
	}

	// Try multiple formats since MathWorks API format may vary
	formats := []string{
		"2006-01-02T15:04:05.000+0000",  // MathWorks typical format
		"2006-01-02T15:04:05.000Z",       // ISO with Z
		"2006-01-02T15:04:05.000-0700",   // with numeric timezone
		time.RFC3339,                      // standard RFC3339
		"2006-01-02T15:04:05.999Z07:00",  // RFC3339 with millis
	}

	for _, format := range formats {
		if expiry, err := time.Parse(format, info.Expiry); err == nil {
			return time.Until(expiry) > 1*time.Hour
		}
	}

	// If we can't parse the expiry, treat as invalid
	return false
}

func (m *Manager) GetInfo() *Info {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.info == nil {
		return &Info{Type: TypeNone}
	}
	return m.info
}

func (m *Manager) IsLicensed() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.info == nil {
		return false
	}
	switch m.info.Type {
	case TypeExisting:
		return true
	case TypeNLM:
		return m.info.ConnectionStr != ""
	case TypeMHLM:
		return m.info.EntitlementID != ""
	default:
		return false
	}
}

func (m *Manager) SetLicensing(info *Info) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.validateInfo(info); err != nil {
		return err
	}

	m.info = info

	return m.persistConfig()
}

// SetMHLMLicensing handles the full MHLM login flow: expand the identity token,
// fetch entitlements, and auto-select if only one is available.
func (m *Manager) SetMHLMLicensing(identityToken, sourceID, emailAddr string) error {
	// Step 1: Expand the identity token to get expiry and user details
	tokenData, err := FetchExpandToken(identityToken, sourceID)
	if err != nil {
		// Store partial info so the UI shows the email and license type
		m.mu.Lock()
		m.info = &Info{Type: TypeMHLM, EmailAddr: emailAddr}
		m.mu.Unlock()
		return fmt.Errorf("expanding identity token: %w", err)
	}

	m.mu.Lock()
	m.info = &Info{
		Type:          TypeMHLM,
		IdentityToken: identityToken,
		SourceID:      sourceID,
		Expiry:        tokenData.Expiry,
		EmailAddr:     emailAddr,
		FirstName:     tokenData.FirstName,
		LastName:      tokenData.LastName,
		DisplayName:   tokenData.DisplayName,
		UserID:        tokenData.UserID,
		ProfileID:     tokenData.ProfileID,
		Entitlements:  nil,
		EntitlementID: "",
	}
	m.mu.Unlock()

	// Step 2: Fetch entitlements
	if err := m.fetchAndSetEntitlements(); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	return m.persistConfig()
}

// fetchAndSetEntitlements fetches an access token, then fetches entitlements,
// and auto-selects if only one is returned. Caller should NOT hold m.mu.
func (m *Manager) fetchAndSetEntitlements() error {
	m.mu.RLock()
	identityToken := m.info.IdentityToken
	sourceID := m.info.SourceID
	matlabVersion := m.cfg.MATLABVersion
	m.mu.RUnlock()

	// Get access token
	accessData, err := FetchAccessToken(identityToken, sourceID)
	if err != nil {
		m.clearMHLMTokenFields()
		return fmt.Errorf("fetching access token: %w", err)
	}

	// Get entitlements
	entitlements, err := FetchEntitlements(accessData.Token, matlabVersion)
	if err != nil {
		m.clearMHLMTokenFields()
		return fmt.Errorf("fetching entitlements: %w", err)
	}

	m.mu.Lock()
	m.info.Entitlements = entitlements
	// Auto-select if only one entitlement
	if len(entitlements) == 1 {
		m.info.EntitlementID = entitlements[0].ID
	}
	m.mu.Unlock()

	return nil
}

// RefreshEntitlements re-fetches entitlements from MathWorks. Used when the
// user needs to re-select an entitlement or on session restore.
func (m *Manager) RefreshEntitlements() error {
	m.mu.RLock()
	if m.info == nil || m.info.Type != TypeMHLM {
		m.mu.RUnlock()
		return fmt.Errorf("MHLM licensing must be configured to update entitlements")
	}
	m.mu.RUnlock()

	if err := m.fetchAndSetEntitlements(); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	return m.persistConfig()
}

func (m *Manager) UpdateEntitlement(entitlementID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.info == nil || m.info.Type != TypeMHLM {
		return fmt.Errorf("not using MHLM licensing")
	}

	m.info.EntitlementID = entitlementID
	return m.persistConfig()
}

// MHLMEnvVars fetches a fresh access token and returns the environment
// variables needed by MATLAB for MHLM licensing. This should be called
// at MATLAB start time.
func (m *Manager) MHLMEnvVars() (map[string]string, error) {
	m.mu.RLock()
	info := m.info
	m.mu.RUnlock()

	if info == nil || info.Type != TypeMHLM {
		return nil, fmt.Errorf("not using MHLM licensing")
	}

	accessData, err := FetchAccessToken(info.IdentityToken, info.SourceID)
	if err != nil {
		return nil, fmt.Errorf("fetching access token for MATLAB startup: %w", err)
	}

	return map[string]string{
		"MLM_WEB_LICENSE":  "true",
		"MLM_WEB_USER_CRED": accessData.Token,
		"MLM_WEB_ID":        info.EntitlementID,
		"MHLM_CONTEXT":      mhlmContext,
	}, nil
}

// clearMHLMTokenFields clears sensitive token fields on error while preserving
// the license type and email for UI display.
func (m *Manager) clearMHLMTokenFields() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.info == nil {
		return
	}
	m.info.IdentityToken = ""
	m.info.SourceID = ""
	m.info.Expiry = ""
	m.info.FirstName = ""
	m.info.LastName = ""
	m.info.DisplayName = ""
	m.info.UserID = ""
	m.info.ProfileID = ""
	m.info.Entitlements = nil
	m.info.EntitlementID = ""
}

func (m *Manager) ClearLicensing() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.info = nil
	return m.removeCachedConfig()
}

func (m *Manager) validateInfo(info *Info) error {
	switch info.Type {
	case TypeNLM:
		if info.ConnectionStr == "" {
			return fmt.Errorf("NLM connection string is required")
		}
	case TypeMHLM:
		if info.IdentityToken == "" {
			return fmt.Errorf("MHLM identity token is required")
		}
	case TypeExisting:
		// No validation needed
	default:
		return fmt.Errorf("unknown licensing type: %s", info.Type)
	}
	return nil
}

func (m *Manager) configFilePath() string {
	return filepath.Join(m.dataDir, "proxy_app_config.json")
}

func (m *Manager) persistConfig() error {
	if m.info == nil {
		return nil
	}
	data, err := json.MarshalIndent(m.info, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(m.configFilePath()), 0700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	return os.WriteFile(m.configFilePath(), data, 0600)
}

func (m *Manager) loadCachedConfig() (*Info, error) {
	data, err := os.ReadFile(m.configFilePath())
	if err != nil {
		return nil, err
	}
	var info Info
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func (m *Manager) removeCachedConfig() error {
	err := os.Remove(m.configFilePath())
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
