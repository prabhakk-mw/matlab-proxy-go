// Copyright 2026 The MathWorks, Inc.

package matlab

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"time"
)

// EmbeddedConnector communicates with the MATLAB Embedded Connector (EC).
type EmbeddedConnector struct {
	port     int
	mwapikey string
	client   *http.Client
}

func NewEmbeddedConnector(port int, mwapikey string) *EmbeddedConnector {
	return &EmbeddedConnector{
		port:     port,
		mwapikey: mwapikey,
		client: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
			Timeout: 5 * time.Second,
		},
	}
}

func (ec *EmbeddedConnector) Port() int {
	return ec.port
}

func (ec *EmbeddedConnector) MWAPIKey() string {
	return ec.mwapikey
}

func (ec *EmbeddedConnector) BaseURL() string {
	return fmt.Sprintf("https://127.0.0.1:%d", ec.port)
}

// Ping checks if the Embedded Connector is alive.
func (ec *EmbeddedConnector) Ping() (bool, error) {
	payload := map[string]interface{}{
		"messages": map[string]interface{}{
			"Ping": []map[string]interface{}{{}},
		},
	}
	resp, err := ec.postJSON("/messageservice/json/state", payload)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, nil
	}

	var result struct {
		Messages struct {
			PingResponse []struct {
				MessageFaults []interface{} `json:"messageFaults"`
			} `json:"PingResponse"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}
	if len(result.Messages.PingResponse) > 0 {
		return len(result.Messages.PingResponse[0].MessageFaults) == 0, nil
	}
	return false, nil
}

// GetBusyStatus returns the MATLAB busy/idle status.
func (ec *EmbeddedConnector) GetBusyStatus() (BusyStatus, error) {
	payload := map[string]interface{}{
		"messages": map[string]interface{}{
			"QueryMatlabStatus": []map[string]interface{}{{}},
		},
	}
	resp, err := ec.postJSON("/messageservice/json/state", payload)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result struct {
		Messages struct {
			QueryMatlabStatusResponse []struct {
				Status string `json:"status"`
			} `json:"QueryMatlabStatusResponse"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if len(result.Messages.QueryMatlabStatusResponse) > 0 {
		return BusyStatus(result.Messages.QueryMatlabStatusResponse[0].Status), nil
	}
	return "", fmt.Errorf("no status in response")
}

// Eval sends MATLAB code to be evaluated via the Embedded Connector.
func (ec *EmbeddedConnector) Eval(mcode string) error {
	uuid := generateUUID()
	payload := map[string]interface{}{
		"uuid": uuid,
		"messages": map[string]interface{}{
			"Eval": []map[string]interface{}{
				{"mcode": mcode, "uuid": uuid},
			},
		},
		"computeToken": map[string]string{
			"computeSessionId": "unused",
		},
	}
	resp, err := ec.postJSON("/messageservice/json/secure", payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// SendExit sends an exit command to MATLAB via the Embedded Connector.
func (ec *EmbeddedConnector) SendExit() error {
	uuid := generateUUID()
	payload := map[string]interface{}{
		"uuid": uuid,
		"messages": map[string]interface{}{
			"Eval": []map[string]interface{}{
				{"mcode": "exit", "uuid": uuid},
			},
		},
		"computeToken": map[string]string{
			"computeSessionId": "unused",
		},
	}
	resp, err := ec.postJSON("/messageservice/json/secure", payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (ec *EmbeddedConnector) postJSON(path string, payload interface{}) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	url := ec.BaseURL() + path
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if ec.mwapikey != "" {
		req.Header.Set("mwapikey", ec.mwapikey)
	}
	return ec.client.Do(req)
}

func generateUUID() string {
	const chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, 8)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}

// ProxyHTTPRequest forwards an HTTP request to the Embedded Connector and
// writes the response back to the client.
func (ec *EmbeddedConnector) ProxyHTTPRequest(w http.ResponseWriter, r *http.Request) error {
	targetURL := ec.BaseURL() + r.URL.Path
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
	if err != nil {
		return fmt.Errorf("creating proxy request: %w", err)
	}

	// Copy headers
	for key, values := range r.Header {
		for _, v := range values {
			proxyReq.Header.Add(key, v)
		}
	}
	proxyReq.Header.Set("X-Forwarded-Proto", "http")

	resp, err := ec.client.Do(proxyReq)
	if err != nil {
		return fmt.Errorf("proxying request: %w", err)
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, v := range values {
			w.Header().Add(key, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
	return nil
}
