// Copyright 2026 The MathWorks, Inc.

package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/mathworks/matlab-proxy-go/internal/config"
)

const (
	TokenHeader     = "mwi-auth-token"
	TokenQueryParam = "mwi-auth-token"
	cookiePrefix    = "mwi-auth-session"
)

type TokenAuth struct {
	enabled    bool
	token      string
	tokenHash  string
	cookieName string
}

func New(cfg *config.Config) (*TokenAuth, error) {
	ta := &TokenAuth{
		enabled:    cfg.EnableTokenAuth,
		cookieName: fmt.Sprintf("%s-%d", cookiePrefix, cfg.Port),
	}
	if !ta.enabled {
		return ta, nil
	}

	token := cfg.AuthToken
	if token == "" {
		generated, err := generateToken()
		if err != nil {
			return nil, fmt.Errorf("generating auth token: %w", err)
		}
		token = generated
	}
	ta.token = token
	ta.tokenHash = hashToken(token)
	return ta, nil
}

func (ta *TokenAuth) Enabled() bool {
	return ta.enabled
}

func (ta *TokenAuth) Token() string {
	return ta.token
}

func (ta *TokenAuth) TokenHash() string {
	return ta.tokenHash
}

// ClearStaleCookieMiddleware returns middleware that expires any session cookie
// from a previous server instance on the same port. It runs once per client
// by checking if the existing cookie value matches the current session's token hash.
func (ta *TokenAuth) ClearStaleCookieMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ta.enabled {
			if c, err := r.Cookie(ta.cookieName); err == nil && c.Value != "" {
				if !ta.validateToken(c.Value) {
					// Cookie is from a previous session — expire it
					http.SetCookie(w, &http.Cookie{
						Name:     ta.cookieName,
						Value:    "",
						Path:     "/",
						MaxAge:   -1,
						HttpOnly: true,
					})
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (ta *TokenAuth) AccessURL(serverURL string) string {
	if !ta.enabled {
		return serverURL
	}
	sep := "?"
	if strings.Contains(serverURL, "?") {
		sep = "&"
	}
	return fmt.Sprintf("%s%s%s=%s", serverURL, sep, TokenQueryParam, ta.token)
}

// ValidateRequest checks if the request carries a valid auth token.
// It checks (in order): session cookie, URL query parameter, HTTP header.
func (ta *TokenAuth) ValidateRequest(r *http.Request) bool {
	if !ta.enabled {
		return true
	}

	// Check session cookie first (set after initial auth)
	if c, err := r.Cookie(ta.cookieName); err == nil && c.Value != "" {
		if ta.validateToken(c.Value) {
			return true
		}
	}

	// Check URL query parameter
	if qToken := r.URL.Query().Get(TokenQueryParam); qToken != "" {
		return ta.validateToken(qToken)
	}

	// Check HTTP header
	if hToken := r.Header.Get(TokenHeader); hToken != "" {
		return ta.validateToken(hToken)
	}

	return false
}

// SetSessionCookie sets an HTTP-only auth cookie on the response.
// This allows iframe subrequests to authenticate automatically.
func (ta *TokenAuth) SetSessionCookie(w http.ResponseWriter) {
	if !ta.enabled {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     ta.cookieName,
		Value:    ta.tokenHash,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (ta *TokenAuth) validateToken(candidate string) bool {
	if secureCompare(candidate, ta.token) {
		return true
	}
	return secureCompare(candidate, ta.tokenHash)
}

// Middleware returns an HTTP middleware that enforces token authentication.
// On successful auth, it sets a session cookie so that subsequent requests
// (including iframe subrequests) are authenticated automatically.
func (ta *TokenAuth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ta.enabled {
			next.ServeHTTP(w, r)
			return
		}
		if ta.ValidateRequest(r) {
			ta.SetSessionCookie(w)
			next.ServeHTTP(w, r)
			return
		}
		// Log auth failure details for debugging
		cookie, _ := r.Cookie(ta.cookieName)
		cookieVal := ""
		if cookie != nil {
			v := cookie.Value
			if len(v) > 8 {
				v = v[:8]
			}
			cookieVal = v + "..."
		}
		fmt.Printf("AUTH FAIL: %s %s | cookie=%q header=%q query=%q\n",
			r.Method, r.URL.Path, cookieVal, r.Header.Get(TokenHeader), r.URL.Query().Get(TokenQueryParam))
		http.Error(w, "Forbidden", http.StatusForbidden)
	})
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func secureCompare(a, b string) bool {
	if len(a) != len(b) {
		return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
	}
	return hmac.Equal([]byte(a), []byte(b))
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
