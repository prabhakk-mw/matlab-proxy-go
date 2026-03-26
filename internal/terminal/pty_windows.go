// Copyright 2026 The MathWorks, Inc.

//go:build windows

package terminal

import (
	"fmt"
	"os"
	"os/exec"
)

func startWithPTY(cmd *exec.Cmd) (*os.File, error) {
	return nil, fmt.Errorf("terminal is not supported on Windows")
}

func resizePTY(ptmx *os.File, cols, rows int) error {
	return fmt.Errorf("terminal is not supported on Windows")
}
