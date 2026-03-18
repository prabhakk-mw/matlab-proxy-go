// Copyright 2026 The MathWorks, Inc.

package proxy

import (
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024 * 1024,
	WriteBufferSize: 1024 * 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// WebSocketProxy bidirectionally proxies a WebSocket connection to the MATLAB
// Embedded Connector.
type WebSocketProxy struct {
	logger *slog.Logger
}

func NewWebSocketProxy(logger *slog.Logger) *WebSocketProxy {
	return &WebSocketProxy{logger: logger}
}

// Handle upgrades the client connection and proxies messages to/from the EC.
func (wsp *WebSocketProxy) Handle(w http.ResponseWriter, r *http.Request, ecPort int, mwapikey string) error {
	// Upgrade client connection
	clientConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return fmt.Errorf("upgrading client connection: %w", err)
	}
	defer clientConn.Close()

	clientConn.SetReadLimit(500 * 1024 * 1024)

	// Connect to EC
	ecURL := fmt.Sprintf("wss://127.0.0.1:%d%s", ecPort, r.URL.Path)
	if r.URL.RawQuery != "" {
		ecURL += "?" + r.URL.RawQuery
	}

	dialer := websocket.Dialer{
		TLSClientConfig:  &tls.Config{InsecureSkipVerify: true},
		ReadBufferSize:   1024 * 1024,
		WriteBufferSize:  1024 * 1024,
		EnableCompression: true,
	}

	headers := http.Header{}
	if mwapikey != "" {
		headers.Set("mwapikey", mwapikey)
	}
	ecConn, _, err := dialer.Dial(ecURL, headers)
	if err != nil {
		return fmt.Errorf("connecting to EC WebSocket: %w", err)
	}
	defer ecConn.Close()

	ecConn.SetReadLimit(500 * 1024 * 1024)

	errc := make(chan error, 2)

	// Client -> EC
	go func() {
		errc <- forward(clientConn, ecConn)
	}()

	// EC -> Client
	go func() {
		errc <- forward(ecConn, clientConn)
	}()

	// Wait for either direction to close
	err = <-errc

	// Close both connections
	_ = clientConn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	_ = ecConn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))

	if err != nil && !isNormalClose(err) {
		wsp.logger.Debug("WebSocket proxy closed", "error", err)
	}
	return nil
}

func forward(src, dst *websocket.Conn) error {
	for {
		msgType, msg, err := src.ReadMessage()
		if err != nil {
			return err
		}
		if err := dst.WriteMessage(msgType, msg); err != nil {
			return err
		}
	}
}

func isNormalClose(err error) bool {
	if err == nil || err == io.EOF {
		return true
	}
	return websocket.IsCloseError(err,
		websocket.CloseNormalClosure,
		websocket.CloseGoingAway) ||
		strings.Contains(err.Error(), "use of closed network connection")
}

// IsWebSocketUpgrade checks if the request is a WebSocket upgrade request.
func IsWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Connection"), "upgrade") &&
		strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}
