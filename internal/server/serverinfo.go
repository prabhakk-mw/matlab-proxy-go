// Copyright 2026 The MathWorks, Inc.

package server

import (
	"os"
	"path/filepath"
	"strings"
)

// writeServerInfoFile creates the mwi_server.info file that allows
// the list-servers command to discover running instances.
// Format: line 1 = server URL with auth token, line 2 = browser title.
func (s *Server) writeServerInfoFile() error {
	logsDir := s.cfg.LogsDir()
	if err := os.MkdirAll(logsDir, 0700); err != nil {
		return err
	}

	infoFile := filepath.Join(logsDir, "mwi_server.info")

	accessURL := s.auth.AccessURL(s.cfg.ServerURL())

	// Title format: "<session_name> - MATLAB <version>"
	// This is parsed by extractVersionAndSession in the --list command.
	// When SessionName is the auto-generated default (e.g., "MATLAB R2025b"),
	// avoid duplicating it as "MATLAB R2025b - MATLAB R2025b".
	title := s.cfg.SessionName
	if s.cfg.MATLABVersion != "" && !strings.Contains(title, "MATLAB") {
		title += " - MATLAB " + s.cfg.MATLABVersion
	}
	content := accessURL + "\n" + title + "\n"

	s.logger.Info("writing server info file", "path", infoFile)
	return os.WriteFile(infoFile, []byte(content), 0600)
}

// removeServerInfoFile cleans up the info file on shutdown.
func (s *Server) removeServerInfoFile() {
	infoFile := filepath.Join(s.cfg.LogsDir(), "mwi_server.info")
	if err := os.Remove(infoFile); err != nil && !os.IsNotExist(err) {
		s.logger.Warn("failed to remove server info file", "error", err)
	}
}
