// Copyright 2026 The MathWorks, Inc.

package listservers

import (
	"testing"
)

func TestExtractVersionAndSession(t *testing.T) {
	tests := []struct {
		title       string
		wantVersion string
		wantSession string
	}{
		{
			title:       "My Session - MATLAB R2025b",
			wantVersion: "R2025b",
			wantSession: "My Session",
		},
		{
			title:       "MATLAB R2024a",
			wantVersion: "R2024a",
			wantSession: "",
		},
		{
			title:       "Custom Name - MATLAB R2023b",
			wantVersion: "R2023b",
			wantSession: "Custom Name",
		},
		{
			title:       "",
			wantVersion: "",
			wantSession: "",
		},
	}

	for _, tt := range tests {
		version, session := extractVersionAndSession(tt.title)
		if version != tt.wantVersion {
			t.Errorf("extractVersionAndSession(%q): version = %q, want %q", tt.title, version, tt.wantVersion)
		}
		if session != tt.wantSession {
			t.Errorf("extractVersionAndSession(%q): session = %q, want %q", tt.title, session, tt.wantSession)
		}
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is too long", 10, "this is..."},
		{"abc", 3, "abc"},
	}

	for _, tt := range tests {
		got := truncate(tt.input, tt.max)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.want)
		}
	}
}

func TestParseServerInfo_InvalidFile(t *testing.T) {
	_, err := parseServerInfo("/nonexistent/path/file.info")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}
