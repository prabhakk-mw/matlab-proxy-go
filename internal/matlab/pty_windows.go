// Copyright 2026 The MathWorks, Inc.

//go:build windows

package matlab

import (
	"fmt"
	"os"
)

// openPTY is a no-op on Windows — MATLAB doesn't need a PTY on Windows.
func openPTY() (master *os.File, slave *os.File, err error) {
	return nil, nil, fmt.Errorf("PTY not supported on Windows")
}
