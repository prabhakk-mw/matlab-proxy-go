// Copyright 2026 The MathWorks, Inc.

package version

import (
	"testing"
)

func TestVersion_Default(t *testing.T) {
	if Version == "" {
		t.Error("Version should not be empty")
	}
	if Version != "0.0.0-dev" {
		t.Errorf("expected default version '0.0.0-dev', got %q", Version)
	}
}
