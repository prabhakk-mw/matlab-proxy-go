// Copyright 2026 The MathWorks, Inc.

package matlab

import (
	"bufio"
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/mathworks/matlab-proxy-go/internal/config"
	"github.com/mathworks/matlab-proxy-go/internal/display"
)

//go:embed scripts/startup.m scripts/evaluateUserMatlabCode.m
var matlabScripts embed.FS

// Process manages the MATLAB process lifecycle.
type Process struct {
	mu          sync.RWMutex
	cfg         *config.Config
	status      Status
	busyStatus  *BusyStatus
	connector   *EmbeddedConnector
	matlabCmd   *exec.Cmd
	displayMgr  *display.Manager
	stderrLines []string
	errors      []ErrorInfo
	warnings    []string
	extraEnv    map[string]string // Additional env vars (e.g. MHLM credentials)
	mwapikey    string            // API key for the Embedded Connector

	cancel context.CancelFunc
	done   chan struct{}

	logger *slog.Logger
}

type ErrorInfo struct {
	Message string `json:"message"`
	Type    string `json:"type,omitempty"`
	Logs    string `json:"logs,omitempty"`
}

func NewProcess(cfg *config.Config, logger *slog.Logger) *Process {
	return &Process{
		cfg:    cfg,
		status: StatusDown,
		done:   make(chan struct{}),
		logger: logger,
	}
}

// SetExtraEnv sets additional environment variables to pass to MATLAB on startup.
// Used for MHLM licensing credentials.
func (p *Process) SetExtraEnv(env map[string]string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.extraEnv = env
}

func (p *Process) Status() Status {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.status
}

func (p *Process) BusyStatus() *BusyStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.busyStatus
}

func (p *Process) Errors() []ErrorInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return append([]ErrorInfo{}, p.errors...)
}

func (p *Process) Warnings() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return append([]string{}, p.warnings...)
}

func (p *Process) Connector() *EmbeddedConnector {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.connector
}

func (p *Process) Start(restart bool) error {
	p.mu.Lock()

	if p.status == StatusStarting || p.status == StatusUp {
		if !restart {
			p.mu.Unlock()
			return nil
		}
		p.mu.Unlock()
		if err := p.Stop(false); err != nil {
			p.logger.Warn("error stopping MATLAB before restart", "error", err)
		}
		p.mu.Lock()
	}

	if p.cfg.MATLABCommand == "" {
		p.errors = []ErrorInfo{{
			Message: "Unable to find MATLAB on the system PATH. Add MATLAB to the system PATH, and restart matlab-proxy.",
			Type:    "MatlabInstallError",
		}}
		p.mu.Unlock()
		return fmt.Errorf("unable to find MATLAB on the system PATH")
	}

	p.status = StatusStarting
	p.errors = nil
	p.warnings = nil
	p.stderrLines = nil
	p.busyStatus = nil
	p.connector = nil
	p.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.done = make(chan struct{})

	go p.run(ctx)

	return nil
}

func (p *Process) run(ctx context.Context) {
	defer close(p.done)

	logsDir := p.cfg.LogsDir()
	if err := os.MkdirAll(logsDir, 0700); err != nil {
		p.setError(ErrorInfo{Message: fmt.Sprintf("creating logs directory: %v", err), Type: "SetupError"})
		return
	}

	// Start Xvfb on Linux
	if runtime.GOOS == "linux" {
		p.displayMgr = display.NewManager(p.logger)
		if err := p.displayMgr.Start(p.cfg.EnableSimulink); err != nil {
			p.logger.Warn("failed to start display manager", "error", err)
			// Non-fatal: MATLAB may still work without Xvfb
		}
	}

	args := p.buildMATLABArgs(logsDir)
	env := p.buildMATLABEnv(logsDir)

	// Log the exact command that will be executed
	fullCmd := append([]string{p.cfg.MATLABCommand}, args...)
	p.logger.Info("starting MATLAB", "fullCommand", strings.Join(fullCmd, " "), "logsDir", logsDir)

	// Log key environment variables for debugging
	for _, e := range env {
		for _, prefix := range []string{"MATLAB_LOG_DIR=", "MWAPIKEY=", "MW_CONNECTOR_CONTEXT_ROOT=", "DISPLAY=", "MLM_LICENSE_FILE=", "MW_DOCROOT=", "MW_CONTEXT_TAGS="} {
			if strings.HasPrefix(e, prefix) {
				p.logger.Info("MATLAB env", "var", e)
			}
		}
	}

	cmd := exec.CommandContext(ctx, p.cfg.MATLABCommand, args...)
	cmd.Env = env
	cmd.Dir = logsDir

	// On POSIX systems, MATLAB requires a PTY as stdin (same as Python's pty.openpty())
	if runtime.GOOS != "windows" {
		ptmx, slaveFd, ptyErr := openPTY()
		if ptyErr != nil {
			p.logger.Warn("failed to create PTY, using /dev/null as stdin", "error", ptyErr)
			devnull, _ := os.Open(os.DevNull)
			if devnull != nil {
				cmd.Stdin = devnull
				defer devnull.Close()
			}
		} else if slaveFd != nil {
			cmd.Stdin = slaveFd
			defer ptmx.Close()
			defer slaveFd.Close()
		}
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		p.setError(ErrorInfo{Message: fmt.Sprintf("creating stderr pipe: %v", err), Type: "StartupError"})
		return
	}

	if err := cmd.Start(); err != nil {
		p.setError(ErrorInfo{Message: fmt.Sprintf("starting MATLAB: %v", err), Type: "StartupError"})
		return
	}

	p.mu.Lock()
	p.matlabCmd = cmd
	p.mu.Unlock()

	// Read stderr in background
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			p.mu.Lock()
			p.stderrLines = append(p.stderrLines, line)
			p.mu.Unlock()
			p.logger.Info("MATLAB stderr", "line", line)
		}
	}()

	// Wait for process exit in background so we can detect early death
	processExited := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = cmd.Wait()
		close(processExited)
	}()

	// Wait for Embedded Connector to become ready (aborts if process dies)
	p.waitForConnector(ctx, logsDir, processExited)

	// Wait for MATLAB process to exit (may already be done)
	<-processExited
	if waitErr != nil {
		if ctx.Err() == nil {
			// Process exited unexpectedly
			p.mu.Lock()
			stderrLog := strings.Join(p.stderrLines, "\n")
			p.mu.Unlock()
			p.logger.Error("MATLAB exited with error", "error", waitErr, "stderr", stderrLog)
			p.setError(ErrorInfo{
				Message: fmt.Sprintf("MATLAB exited unexpectedly: %v", waitErr),
				Type:    "MATLABError",
				Logs:    stderrLog,
			})
		}
	}

	// Clean up session files created during this MATLAB run
	p.cleanupSessionFiles(logsDir)

	p.mu.Lock()
	p.status = StatusDown
	p.busyStatus = nil
	p.connector = nil
	p.matlabCmd = nil
	if p.displayMgr != nil {
		p.displayMgr.Stop()
		p.displayMgr = nil
	}
	p.mu.Unlock()
}

// cleanupSessionFiles removes artifacts created during a MATLAB session
// that would interfere with subsequent launches from the same port.
func (p *Process) cleanupSessionFiles(logsDir string) {
	// Remove connector.securePort — stale file would cause next launch
	// to read an old port number
	readyFile := filepath.Join(logsDir, config.ConnectorSecurePortFile)
	if err := os.Remove(readyFile); err != nil && !os.IsNotExist(err) {
		p.logger.Warn("failed to remove connector ready file", "path", readyFile, "error", err)
	}

	// Remove extracted startup scripts
	scriptsDir := filepath.Join(logsDir, "matlab_scripts")
	if err := os.RemoveAll(scriptsDir); err != nil {
		p.logger.Warn("failed to remove matlab scripts dir", "path", scriptsDir, "error", err)
	}

	p.logger.Debug("cleaned up MATLAB session files", "logsDir", logsDir)
}

func (p *Process) waitForConnector(ctx context.Context, logsDir string, processDone <-chan struct{}) {
	readyFile := filepath.Join(logsDir, config.ConnectorSecurePortFile)
	timeout := time.After(time.Duration(p.cfg.ProcessStartTimeout) * time.Second)

	// Phase 1: Wait for the ready file with the EC port
	var port int
	for {
		select {
		case <-ctx.Done():
			return
		case <-processDone:
			p.logger.Warn("MATLAB process exited before Embedded Connector was ready")
			return
		case <-timeout:
			p.setError(ErrorInfo{
				Message: "MATLAB startup timed out waiting for Embedded Connector",
				Type:    "TimeoutError",
			})
			return
		case <-time.After(500 * time.Millisecond):
			data, err := os.ReadFile(readyFile)
			if err != nil {
				continue
			}
			content := strings.TrimSpace(string(data))
			if content == "" {
				continue
			}
			_, _ = fmt.Sscanf(content, "%d", &port)
			if port > 0 {
				goto portFound
			}
		}
	}

portFound:
	p.logger.Info("MATLAB Embedded Connector port detected", "port", port)
	p.mu.RLock()
	mwapikey := p.mwapikey
	p.mu.RUnlock()
	ec := NewEmbeddedConnector(port, mwapikey)

	// Phase 2: Wait for EC to respond to pings
	for {
		select {
		case <-ctx.Done():
			return
		case <-processDone:
			p.logger.Warn("MATLAB process exited while waiting for EC ping")
			return
		case <-timeout:
			p.setError(ErrorInfo{
				Message: "MATLAB startup timed out waiting for Embedded Connector to respond",
				Type:    "TimeoutError",
			})
			return
		case <-time.After(config.StatusPollInterval):
			alive, err := ec.Ping()
			if err != nil {
				p.logger.Debug("EC ping failed", "error", err)
				continue
			}
			if alive {
				p.mu.Lock()
				p.connector = ec
				p.status = StatusUp
				p.mu.Unlock()
				p.logger.Info("MATLAB is ready")

				// Start busy status polling
				go p.pollBusyStatus(ctx)
				return
			}
		}
	}
}

func (p *Process) pollBusyStatus(ctx context.Context) {
	ticker := time.NewTicker(config.StatusPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ec := p.Connector()
			if ec == nil {
				return
			}
			status, err := ec.GetBusyStatus()
			if err != nil {
				// Fall back to ping for older MATLAB versions
				alive, pingErr := ec.Ping()
				if pingErr != nil || !alive {
					p.mu.Lock()
					p.busyStatus = nil
					p.mu.Unlock()
				}
				continue
			}
			p.mu.Lock()
			p.busyStatus = &status
			p.mu.Unlock()
		}
	}
}

func (p *Process) Stop(forceQuit bool) error {
	p.mu.Lock()
	if p.status == StatusDown {
		p.mu.Unlock()
		return nil
	}

	cmd := p.matlabCmd
	ec := p.connector
	wasStarting := p.status == StatusStarting
	p.status = StatusDown
	p.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		if p.cancel != nil {
			p.cancel()
		}
		return nil
	}

	if !forceQuit && !wasStarting && ec != nil {
		// Try graceful exit via EC
		p.logger.Info("sending exit command to MATLAB")
		if err := ec.SendExit(); err != nil {
			p.logger.Warn("graceful exit failed, will force kill", "error", err)
			forceQuit = true
		} else {
			// Wait for MATLAB to exit gracefully
			p.logger.Info("waiting for MATLAB to exit gracefully")
			select {
			case <-p.done:
				p.logger.Info("MATLAB exited gracefully")
				return nil
			case <-time.After(10 * time.Second):
				p.logger.Warn("graceful exit timed out, force killing")
				forceQuit = true
			}
		}
	}

	// Cancel the run context — this terminates the CommandContext process
	if p.cancel != nil {
		p.cancel()
	}

	if forceQuit || wasStarting {
		p.logger.Info("force killing MATLAB process")
		if err := cmd.Process.Kill(); err != nil {
			// Process may have already exited from context cancel
			p.logger.Debug("kill returned error (may already be exited)", "error", err)
		}
	}

	// Wait for the run goroutine to finish
	select {
	case <-p.done:
	case <-time.After(5 * time.Second):
		p.logger.Warn("timed out waiting for MATLAB cleanup to finish")
	}

	return nil
}

// CleanupLogsDir removes all MATLAB session artifacts from the logs directory.
// Called by the server on shutdown to ensure a clean state for subsequent launches.
func (p *Process) CleanupLogsDir() {
	logsDir := p.cfg.LogsDir()
	p.cleanupSessionFiles(logsDir)
}

func (p *Process) buildMATLABArgs(logsDir string) []string {
	args := []string{"-nosplash", "-nodesktop", "-softwareopengl"}

	if runtime.GOOS != "windows" {
		args = append(args, "-externalUI")
	} else {
		args = append(args, "-noDisplayDesktop", "-wait", "-log")
	}

	if p.cfg.ProfileStartup {
		args = append(args, "-timing")
	}

	// Licensing
	if p.cfg.LicModeOverride != "" {
		args = append(args, "-licmode", p.cfg.LicModeOverride)
	} else if p.cfg.NLMConnStr != "" {
		args = append(args, "-licmode", "file")
	}

	// Startup code
	startupCode := p.buildStartupCode(logsDir)
	if startupCode != "" {
		args = append(args, "-r", startupCode)
	}

	return args
}

// writeStartupScripts extracts the embedded .m scripts to the logs directory
// so MATLAB can run them via -r flag.
func (p *Process) writeStartupScripts(logsDir string) (string, string, error) {
	scriptsDir := filepath.Join(logsDir, "matlab_scripts")
	if err := os.MkdirAll(scriptsDir, 0700); err != nil {
		return "", "", fmt.Errorf("creating scripts dir: %w", err)
	}

	startupPath := filepath.Join(scriptsDir, "startup.m")
	data, err := matlabScripts.ReadFile("scripts/startup.m")
	if err != nil {
		return "", "", fmt.Errorf("reading embedded startup.m: %w", err)
	}
	if err := os.WriteFile(startupPath, data, 0600); err != nil {
		return "", "", fmt.Errorf("writing startup.m: %w", err)
	}

	evalCodePath := filepath.Join(scriptsDir, "evaluateUserMatlabCode.m")
	data, err = matlabScripts.ReadFile("scripts/evaluateUserMatlabCode.m")
	if err != nil {
		return "", "", fmt.Errorf("reading embedded evaluateUserMatlabCode.m: %w", err)
	}
	if err := os.WriteFile(evalCodePath, data, 0600); err != nil {
		return "", "", fmt.Errorf("writing evaluateUserMatlabCode.m: %w", err)
	}

	return startupPath, evalCodePath, nil
}

func (p *Process) buildStartupCode(logsDir string) string {
	startupPath, evalCodePath, err := p.writeStartupScripts(logsDir)
	if err != nil {
		p.logger.Warn("failed to write startup scripts", "error", err)
		return ""
	}

	// Always run the proxy's startup.m (starts connector.internal.Worker, etc.)
	code := fmt.Sprintf("try; run('%s'); catch MATLABProxyInitializationError; disp(MATLABProxyInitializationError.message); end;",
		escapeMATLABPath(startupPath))

	// If custom MATLAB code is set, also run evaluateUserMatlabCode.m
	if config.GetEnv(config.EnvMATLABStartupScript, "") != "" {
		code += fmt.Sprintf("try; run('%s'); catch MATLABCustomStartupCodeError; disp(MATLABCustomStartupCodeError.message); end;",
			escapeMATLABPath(evalCodePath))
	}

	return code
}

// escapeMATLABPath escapes single quotes in file paths for use in MATLAB -r code.
func escapeMATLABPath(path string) string {
	return strings.ReplaceAll(path, "'", "''")
}

func (p *Process) buildMATLABEnv(logsDir string) []string {
	env := os.Environ()

	// Helper to set env var only if not already in the environment
	setDefault := func(key, value string) {
		for _, e := range env {
			if strings.HasPrefix(e, key+"=") {
				return
			}
		}
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}

	// MATLAB log directory — MATLAB writes connector.securePort here
	env = append(env, fmt.Sprintf("MATLAB_LOG_DIR=%s", logsDir))

	// API key for the Embedded Connector — required for it to accept requests
	mwapikey := generateMWAPIKey()
	p.mwapikey = mwapikey
	env = append(env, fmt.Sprintf("MWAPIKEY=%s", mwapikey))

	// Connector context root — must match our base URL for proxying to work
	baseURL := p.cfg.BaseURL
	if baseURL == "/" {
		baseURL = ""
	}
	env = append(env, fmt.Sprintf("MW_CONNECTOR_CONTEXT_ROOT=%s", baseURL))

	// Crash handling
	setDefault("MW_CRASH_MODE", "native")

	// Parallel computing
	setDefault("MATLAB_WORKER_CONFIG_ENABLE_LOCAL_PARCLUSTER", "true")

	// Document root for web GUI
	env = append(env, fmt.Sprintf("MW_DOCROOT=%s", filepath.Join("ui", "webgui", "src")))

	// Change directory support across MATLAB versions
	env = append(env, "MW_CD_ANYWHERE_ENABLED=true")
	env = append(env, "MW_CD_ANYWHERE_DISABLED=false")

	// DDUX context tags
	env = append(env, "MW_CONTEXT_TAGS=MATLAB_PROXY:GOLANG:V1")

	// Display (Linux)
	if p.displayMgr != nil && p.displayMgr.Display() != "" {
		env = append(env, fmt.Sprintf("DISPLAY=%s", p.displayMgr.Display()))
	}

	// NLM licensing
	if p.cfg.NLMConnStr != "" {
		env = append(env, fmt.Sprintf("MLM_LICENSE_FILE=%s", p.cfg.NLMConnStr))
	}

	// Extra env vars (e.g. MHLM licensing credentials)
	for k, v := range p.extraEnv {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	return env
}

func generateMWAPIKey() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (p *Process) setError(errInfo ErrorInfo) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.errors = append(p.errors, errInfo)
	p.status = StatusDown
	p.logger.Error("MATLAB error", "message", errInfo.Message, "type", errInfo.Type)
}
