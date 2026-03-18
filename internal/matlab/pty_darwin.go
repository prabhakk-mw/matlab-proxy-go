// Copyright 2026 The MathWorks, Inc.

//go:build darwin

package matlab

import (
	"os"
)

// openPTY on macOS returns nil — MATLAB does not require a PTY on macOS.
// The process will use standard os.Pipe for stdin instead.
func openPTY() (master *os.File, slave *os.File, err error) {
	return nil, nil, nil
}
