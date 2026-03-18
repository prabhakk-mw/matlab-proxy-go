// Copyright 2026 The MathWorks, Inc.

package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"sync"
)

const appName = "MATLABProxyApp"

// ANSI color codes matching the Python _ColoredFormatter
var levelColors = map[slog.Level]string{
	slog.LevelDebug: "\033[94m", // Blue
	slog.LevelInfo:  "\033[32m", // Green
	slog.LevelWarn:  "\033[93m", // Yellow
	slog.LevelError: "\033[91m", // Red
}

const colorReset = "\033[0m"

// Handler implements slog.Handler with the Python matlab-proxy log format:
//
//	Console: [I 2026-03-18 23:10:51.123 MATLABProxyApp] message key=value
//	File:    [INFO 2026-03-18 23:10:51.123 MATLABProxyApp] message key=value
type Handler struct {
	mu       sync.Mutex
	w        io.Writer
	level    slog.Level
	colored  bool
	groups   []string
	preformatted []slog.Attr
}

// NewHandler creates a handler that writes to w in the Python matlab-proxy format.
// If colored is true, output uses ANSI colors (for terminal use).
func NewHandler(w io.Writer, level slog.Level, colored bool) *Handler {
	return &Handler{
		w:       w,
		level:   level,
		colored: colored,
	}
}

// NewConsoleHandler creates a colored handler writing to stderr.
func NewConsoleHandler(level slog.Level) *Handler {
	return NewHandler(os.Stderr, level, isTerminal(os.Stderr))
}

// NewFileHandler creates a non-colored handler writing to the given writer.
func NewFileHandler(w io.Writer, level slog.Level) *Handler {
	return NewHandler(w, level, false)
}

func (h *Handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *Handler) Handle(_ context.Context, r slog.Record) error {
	var buf []byte

	// Format timestamp: 2026-03-18 23:10:51.123
	t := r.Time.Format("2006-01-02 15:04:05.000")
	// Replace last comma with period (Go uses period already, but ensure consistency)

	if h.colored {
		color := levelColors[r.Level]
		if color == "" {
			color = colorReset
		}
		// Short level: first character
		buf = fmt.Appendf(buf, "%s[%s %s %s]%s %s",
			color, shortLevel(r.Level), t, appName, colorReset, r.Message)
	} else {
		buf = fmt.Appendf(buf, "[%s %s %s] %s",
			r.Level.String(), t, appName, r.Message)
	}

	// Append preformatted attrs from WithAttrs
	for _, a := range h.preformatted {
		buf = appendAttr(buf, a)
	}

	// Append record attrs
	r.Attrs(func(a slog.Attr) bool {
		buf = appendAttr(buf, a)
		return true
	})

	buf = append(buf, '\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.w.Write(buf)
	return err
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{
		w:            h.w,
		level:        h.level,
		colored:      h.colored,
		groups:       h.groups,
		preformatted: append(h.preformatted, attrs...),
	}
}

func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{
		w:            h.w,
		level:        h.level,
		colored:      h.colored,
		groups:       append(h.groups, name),
		preformatted: h.preformatted,
	}
}

func shortLevel(l slog.Level) string {
	switch {
	case l < slog.LevelInfo:
		return "D"
	case l < slog.LevelWarn:
		return "I"
	case l < slog.LevelError:
		return "W"
	default:
		return "E"
	}
}

func appendAttr(buf []byte, a slog.Attr) []byte {
	if a.Equal(slog.Attr{}) {
		return buf
	}
	return fmt.Appendf(buf, " %s=%v", a.Key, a.Value)
}

// isTerminal checks if a file descriptor is a terminal.
func isTerminal(f *os.File) bool {
	if runtime.GOOS == "windows" {
		return false // conservative default
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
