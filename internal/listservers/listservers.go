// Copyright 2026 The MathWorks, Inc.

package listservers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type ServerEntry struct {
	CreatedOn   string    `json:"created_on"`
	MATLABVer   string    `json:"matlab_version"`
	SessionName string    `json:"session_name"`
	ServerURL   string    `json:"server_url"`
	modTime     time.Time
}

// Run executes the list-servers command with the given output mode.
func Run(quiet, jsonOut bool) {
	servers, err := discoverServers()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(servers) == 0 {
		if !quiet {
			fmt.Println("No matlab-proxy servers are currently running.")
		}
		return
	}

	switch {
	case quiet:
		for _, s := range servers {
			fmt.Println(s.ServerURL)
		}
	case jsonOut:
		data, _ := json.MarshalIndent(servers, "", "  ")
		fmt.Println(string(data))
	default:
		printTable(servers)
	}
}

func discoverServers() ([]ServerEntry, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("getting home directory: %w", err)
	}

	portsDir := filepath.Join(home, ".matlab", "MWI", "hosts", "*", "ports")
	hostDirs, _ := filepath.Glob(portsDir)

	var servers []ServerEntry
	for _, hostDir := range hostDirs {
		pattern := filepath.Join(hostDir, "*", "mwi_server.info")
		matches, _ := filepath.Glob(pattern)
		for _, infoFile := range matches {
			entry, err := parseServerInfo(infoFile)
			if err != nil {
				continue
			}
			servers = append(servers, entry)
		}
	}

	sort.Slice(servers, func(i, j int) bool {
		return servers[i].modTime.Before(servers[j].modTime)
	})

	return servers, nil
}

func parseServerInfo(path string) (ServerEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ServerEntry{}, err
	}

	lines := strings.SplitN(string(data), "\n", 3)
	if len(lines) < 2 {
		return ServerEntry{}, fmt.Errorf("invalid server info file: %s", path)
	}

	address := strings.TrimSpace(lines[0])
	title := strings.TrimSpace(lines[1])

	matlabVersion, sessionName := extractVersionAndSession(title)

	info, err := os.Stat(path)
	if err != nil {
		return ServerEntry{}, err
	}

	return ServerEntry{
		CreatedOn:   info.ModTime().Format("02/01/06 15:04:05"),
		MATLABVer:   matlabVersion,
		SessionName: sessionName,
		ServerURL:   address,
		modTime:     info.ModTime(),
	}, nil
}

func extractVersionAndSession(title string) (version, session string) {
	parts := strings.SplitN(title, " - ", 2)
	if len(parts) < 2 {
		return strings.TrimPrefix(title, "MATLAB "), ""
	}
	session = strings.TrimSpace(parts[0])
	version = strings.TrimPrefix(strings.TrimSpace(parts[1]), "MATLAB ")
	return version, session
}

func printTable(servers []ServerEntry) {
	const (
		colCreated = 18
		colVersion = 10
		colSession = 20
		colURL     = 50
	)

	divider := "+" + strings.Repeat("-", colCreated+2) +
		"+" + strings.Repeat("-", colVersion+2) +
		"+" + strings.Repeat("-", colSession+2) +
		"+" + strings.Repeat("-", colURL+2) + "+"

	fmt.Println()
	fmt.Println("  MATLAB Proxy Servers")
	fmt.Println()
	fmt.Println(divider)
	fmt.Printf("| %-*s | %-*s | %-*s | %-*s |\n",
		colCreated, "Created On",
		colVersion, "MATLAB Ver",
		colSession, "Session Name",
		colURL, "Server URL")
	fmt.Println(divider)

	for _, s := range servers {
		url := s.ServerURL
		if len(url) > colURL {
			url = url[:colURL-3] + "..."
		}
		fmt.Printf("| %-*s | %-*s | %-*s | %-*s |\n",
			colCreated, s.CreatedOn,
			colVersion, s.MATLABVer,
			colSession, truncate(s.SessionName, colSession),
			colURL, url)
	}
	fmt.Println(divider)
	fmt.Println()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
