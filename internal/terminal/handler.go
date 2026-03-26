// Copyright 2026 The MathWorks, Inc.

package terminal

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// resizeMsg is sent as a binary WebSocket message from the client.
type resizeMsg struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

// HandleWebSocket upgrades the connection and bridges a PTY shell session
// over the WebSocket. Text messages carry terminal I/O; binary messages
// carry control commands (resize).
func HandleWebSocket(w http.ResponseWriter, r *http.Request, logger *slog.Logger) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error("websocket upgrade failed", "error", err)
		return
	}
	defer conn.Close()

	shell := getShell()
	cmd := exec.Command(shell)
	cmd.Env = os.Environ()

	ptmx, err := startWithPTY(cmd)
	if err != nil {
		logger.Error("failed to start shell with PTY", "error", err)
		conn.WriteMessage(websocket.TextMessage, []byte("Error: "+err.Error()+"\r\n"))
		return
	}
	defer ptmx.Close()

	logger.Info("terminal session started", "shell", shell, "pid", cmd.Process.Pid)

	var wg sync.WaitGroup

	// PTY stdout → WebSocket
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if err != nil {
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, buf[:n]); err != nil {
				return
			}
		}
	}()

	// WebSocket → PTY stdin (text) or resize (binary)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				// Client disconnected — kill the shell
				_ = cmd.Process.Kill()
				return
			}

			switch msgType {
			case websocket.TextMessage:
				if _, err := ptmx.Write(data); err != nil {
					return
				}
			case websocket.BinaryMessage:
				var msg resizeMsg
				if err := json.Unmarshal(data, &msg); err == nil && msg.Cols > 0 && msg.Rows > 0 {
					if err := resizePTY(ptmx, msg.Cols, msg.Rows); err != nil {
						logger.Debug("PTY resize failed", "error", err)
					}
				}
			}
		}
	}()

	// Wait for the shell process to exit
	_ = cmd.Wait()
	logger.Info("terminal session ended", "pid", cmd.Process.Pid)

	// Close the WebSocket to unblock readers
	conn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, "shell exited"))

	wg.Wait()
}

// getShell returns the user's shell, falling back to common defaults.
func getShell() string {
	if sh := os.Getenv("SHELL"); sh != "" {
		return sh
	}
	if _, err := exec.LookPath("bash"); err == nil {
		return "bash"
	}
	return "sh"
}
