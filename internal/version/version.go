// Copyright 2026 The MathWorks, Inc.

package version

// These variables are set at build time via -ldflags.
// Example:
//
//	go build -ldflags "-X github.com/mathworks/matlab-proxy-go/internal/version.Version=1.2.3"
var (
	Version = "0.0.0-dev"
)
