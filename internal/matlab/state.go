// Copyright 2026 The MathWorks, Inc.

package matlab

// Status represents the MATLAB process state.
type Status string

const (
	StatusDown     Status = "down"
	StatusStarting Status = "starting"
	StatusUp       Status = "up"
	StatusStopping Status = "stopping"
)

// BusyStatus represents whether MATLAB is busy executing user code.
type BusyStatus string

const (
	BusyStatusBusy BusyStatus = "busy"
	BusyStatusIdle BusyStatus = "idle"
)
