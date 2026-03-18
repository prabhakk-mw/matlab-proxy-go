// Copyright 2026 The MathWorks, Inc.

package config

import (
	"os"
	"testing"
)

func TestGetEnv(t *testing.T) {
	os.Setenv("TEST_MWI_VAR", "hello")
	defer os.Unsetenv("TEST_MWI_VAR")

	if got := GetEnv("TEST_MWI_VAR", "default"); got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
}

func TestGetEnv_Fallback(t *testing.T) {
	os.Unsetenv("TEST_MWI_MISSING")
	if got := GetEnv("TEST_MWI_MISSING", "fallback"); got != "fallback" {
		t.Errorf("expected 'fallback', got %q", got)
	}
}

func TestGetEnvBool_True(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"true", true},
		{"True", true},
		{"TRUE", true},
		{"1", true},
		{"false", false},
		{"False", false},
		{"0", false},
		{"anything", false},
	}
	for _, tt := range tests {
		os.Setenv("TEST_MWI_BOOL", tt.value)
		got := GetEnvBool("TEST_MWI_BOOL", false)
		if got != tt.want {
			t.Errorf("GetEnvBool(%q) = %v, want %v", tt.value, got, tt.want)
		}
	}
	os.Unsetenv("TEST_MWI_BOOL")
}

func TestGetEnvBool_Fallback(t *testing.T) {
	os.Unsetenv("TEST_MWI_BOOL_MISSING")
	if got := GetEnvBool("TEST_MWI_BOOL_MISSING", true); !got {
		t.Error("expected fallback true")
	}
	if got := GetEnvBool("TEST_MWI_BOOL_MISSING", false); got {
		t.Error("expected fallback false")
	}
}

func TestGetEnvInt(t *testing.T) {
	os.Setenv("TEST_MWI_INT", "42")
	defer os.Unsetenv("TEST_MWI_INT")

	if got := GetEnvInt("TEST_MWI_INT", 0); got != 42 {
		t.Errorf("expected 42, got %d", got)
	}
}

func TestGetEnvInt_Invalid(t *testing.T) {
	os.Setenv("TEST_MWI_INT_BAD", "notanumber")
	defer os.Unsetenv("TEST_MWI_INT_BAD")

	if got := GetEnvInt("TEST_MWI_INT_BAD", 99); got != 99 {
		t.Errorf("expected fallback 99, got %d", got)
	}
}

func TestGetEnvInt_Fallback(t *testing.T) {
	os.Unsetenv("TEST_MWI_INT_MISSING")
	if got := GetEnvInt("TEST_MWI_INT_MISSING", 7); got != 7 {
		t.Errorf("expected fallback 7, got %d", got)
	}
}
