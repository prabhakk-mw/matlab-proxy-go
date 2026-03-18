// Copyright 2026 The MathWorks, Inc.

package proxy

import (
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// HTTPProxy forwards HTTP requests to the MATLAB Embedded Connector.
type HTTPProxy struct {
	client *http.Client
	logger *slog.Logger
}

func NewHTTPProxy(logger *slog.Logger) *HTTPProxy {
	return &HTTPProxy{
		client: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
			// No timeout - some MATLAB requests can take a long time
			Timeout: 0,
		},
		logger: logger,
	}
}

// Forward proxies an HTTP request to the Embedded Connector and writes
// the response back to the client.
func (hp *HTTPProxy) Forward(w http.ResponseWriter, r *http.Request, ecPort int, mwapikey string) {
	targetURL := fmt.Sprintf("https://127.0.0.1:%d%s", ecPort, r.URL.Path)
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
	if err != nil {
		hp.logger.Error("creating proxy request", "error", err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}

	// Copy request headers
	copyHeaders(r.Header, proxyReq.Header)
	proxyReq.Header.Set("X-Forwarded-Proto", "http")

	// Inject MWAPIKEY for EC authentication
	if mwapikey != "" {
		proxyReq.Header.Set("mwapikey", mwapikey)
	}

	// Transform client type in request body for JSD requests
	if strings.Contains(r.URL.Path, "messageservice") {
		proxyReq.Header.Del("Content-Length")
	}

	resp, err := hp.client.Do(proxyReq)
	if err != nil {
		hp.logger.Debug("proxy request failed", "error", err, "url", targetURL)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	copyHeaders(resp.Header, w.Header())

	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func copyHeaders(src, dst http.Header) {
	for key, values := range src {
		for _, v := range values {
			dst.Add(key, v)
		}
	}
}

// WaitForReady polls the EC until it responds, with a timeout.
func WaitForReady(port int, timeout time.Duration) error {
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 2 * time.Second,
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		url := fmt.Sprintf("https://127.0.0.1:%d/messageservice/json/state", port)
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("EC not ready after %v", timeout)
}
