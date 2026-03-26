// Copyright 2026 The MathWorks, Inc.

//go:build windows

package terminal

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procCreatePseudoConsole               = kernel32.NewProc("CreatePseudoConsole")
	procResizePseudoConsole               = kernel32.NewProc("ResizePseudoConsole")
	procClosePseudoConsole                = kernel32.NewProc("ClosePseudoConsole")
	procInitializeProcThreadAttributeList = kernel32.NewProc("InitializeProcThreadAttributeList")
	procUpdateProcThreadAttribute         = kernel32.NewProc("UpdateProcThreadAttribute")
	procDeleteProcThreadAttributeList     = kernel32.NewProc("DeleteProcThreadAttributeList")
	procCreateProcessW                    = kernel32.NewProc("CreateProcessW")
	procGetProcessId                      = kernel32.NewProc("GetProcessId")
)

const (
	_PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE = 0x00020016
	_EXTENDED_STARTUPINFO_PRESENT        = 0x00080000
)

// Supported reports whether the web terminal is available on this platform.
// ConPTY requires Windows 10 version 1809 or later.
func Supported() bool {
	return procCreatePseudoConsole.Find() == nil
}

// ptySession wraps a ConPTY pseudo console and child process on Windows.
type ptySession struct {
	hPC     uintptr
	proc    syscall.Handle
	thread  syscall.Handle
	pid     int
	inPipe  *os.File // write to this → ConPTY stdin
	outPipe *os.File // read from this → ConPTY stdout
}

func newPTYSession(shell string) (*ptySession, error) {
	if err := procCreatePseudoConsole.Find(); err != nil {
		return nil, fmt.Errorf("ConPTY not available (requires Windows 10 1809+): %w", err)
	}

	// Create pipes for ConPTY I/O.
	// Input pipe: we write to inWrite, ConPTY reads from inRead.
	// Output pipe: ConPTY writes to outWrite, we read from outRead.
	var inRead, inWrite, outRead, outWrite syscall.Handle
	if err := syscall.CreatePipe(&inRead, &inWrite, nil, 0); err != nil {
		return nil, fmt.Errorf("creating input pipe: %w", err)
	}
	if err := syscall.CreatePipe(&outRead, &outWrite, nil, 0); err != nil {
		syscall.CloseHandle(inRead)
		syscall.CloseHandle(inWrite)
		return nil, fmt.Errorf("creating output pipe: %w", err)
	}

	// CreatePseudoConsole expects COORD as a single 32-bit value: low 16 = X (cols), high 16 = Y (rows).
	size := uint32(80) | (uint32(24) << 16)
	var hPC uintptr

	r, _, _ := procCreatePseudoConsole.Call(
		uintptr(size),
		uintptr(inRead),
		uintptr(outWrite),
		0,
		uintptr(unsafe.Pointer(&hPC)),
	)

	// ConPTY now owns inRead and outWrite; close our copies.
	syscall.CloseHandle(inRead)
	syscall.CloseHandle(outWrite)

	if r != 0 {
		syscall.CloseHandle(inWrite)
		syscall.CloseHandle(outRead)
		return nil, fmt.Errorf("CreatePseudoConsole: HRESULT 0x%08x", r)
	}

	// Create process attached to the pseudo console.
	procHandle, threadHandle, pid, err := createProcessWithPC(hPC, shell)
	if err != nil {
		procClosePseudoConsole.Call(hPC)
		syscall.CloseHandle(inWrite)
		syscall.CloseHandle(outRead)
		return nil, err
	}

	return &ptySession{
		hPC:     hPC,
		proc:    procHandle,
		thread:  threadHandle,
		pid:     pid,
		inPipe:  os.NewFile(uintptr(inWrite), "conpty-in"),
		outPipe: os.NewFile(uintptr(outRead), "conpty-out"),
	}, nil
}

// startupInfoEx is STARTUPINFOEXW — extends StartupInfo with an attribute list.
type startupInfoEx struct {
	StartupInfo   syscall.StartupInfo
	AttributeList unsafe.Pointer
}

// processInformation is PROCESS_INFORMATION returned by CreateProcessW.
type processInformation struct {
	Process   syscall.Handle
	Thread    syscall.Handle
	ProcessId uint32
	ThreadId  uint32
}

func createProcessWithPC(hPC uintptr, cmdLine string) (proc, thread syscall.Handle, pid int, err error) {
	// Determine attribute list size.
	var attrSize uintptr
	procInitializeProcThreadAttributeList.Call(0, 1, 0, uintptr(unsafe.Pointer(&attrSize)))
	if attrSize == 0 {
		return 0, 0, 0, fmt.Errorf("failed to query attribute list size")
	}

	attrBuf := make([]byte, attrSize)
	attrPtr := unsafe.Pointer(&attrBuf[0])

	r, _, e := procInitializeProcThreadAttributeList.Call(
		uintptr(attrPtr), 1, 0, uintptr(unsafe.Pointer(&attrSize)),
	)
	if r == 0 {
		return 0, 0, 0, fmt.Errorf("InitializeProcThreadAttributeList: %v", e)
	}
	defer procDeleteProcThreadAttributeList.Call(uintptr(attrPtr))

	// Attach pseudo console to the attribute list.
	r, _, e = procUpdateProcThreadAttribute.Call(
		uintptr(attrPtr),
		0,
		_PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE,
		hPC,
		unsafe.Sizeof(hPC),
		0, 0,
	)
	if r == 0 {
		return 0, 0, 0, fmt.Errorf("UpdateProcThreadAttribute: %v", e)
	}

	// Prepare STARTUPINFOEXW.
	var si startupInfoEx
	si.StartupInfo.Cb = uint32(unsafe.Sizeof(si))
	si.AttributeList = attrPtr

	var pi processInformation
	cmdLineUTF16, _ := syscall.UTF16PtrFromString(cmdLine)

	r, _, e = procCreateProcessW.Call(
		0,
		uintptr(unsafe.Pointer(cmdLineUTF16)),
		0, 0,
		0, // bInheritHandles = FALSE
		_EXTENDED_STARTUPINFO_PRESENT,
		0, 0,
		uintptr(unsafe.Pointer(&si)),
		uintptr(unsafe.Pointer(&pi)),
	)
	if r == 0 {
		return 0, 0, 0, fmt.Errorf("CreateProcessW: %v", e)
	}

	return pi.Process, pi.Thread, int(pi.ProcessId), nil
}

func (s *ptySession) Read(p []byte) (int, error)  { return s.outPipe.Read(p) }
func (s *ptySession) Write(p []byte) (int, error) { return s.inPipe.Write(p) }
func (s *ptySession) Pid() int                     { return s.pid }

func (s *ptySession) Resize(cols, rows int) error {
	size := uint32(cols) | (uint32(rows) << 16)
	r, _, _ := procResizePseudoConsole.Call(s.hPC, uintptr(size))
	if r != 0 {
		return fmt.Errorf("ResizePseudoConsole: HRESULT 0x%08x", r)
	}
	return nil
}

func (s *ptySession) Wait() error {
	_, _ = syscall.WaitForSingleObject(s.proc, syscall.INFINITE)
	return nil
}

func (s *ptySession) Kill() error {
	return syscall.TerminateProcess(s.proc, 1)
}

func (s *ptySession) Close() error {
	procClosePseudoConsole.Call(s.hPC)
	s.inPipe.Close()
	s.outPipe.Close()
	syscall.CloseHandle(s.proc)
	syscall.CloseHandle(s.thread)
	return nil
}
