// Copyright 2026 The MathWorks, Inc.

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/mathworks/matlab-proxy-go/internal/config"
	"github.com/mathworks/matlab-proxy-go/internal/listservers"
	mwilog "github.com/mathworks/matlab-proxy-go/internal/logging"
	"github.com/mathworks/matlab-proxy-go/internal/server"
	"github.com/mathworks/matlab-proxy-go/internal/version"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v":
			fmt.Println("matlab-proxy " + version.Version)
			os.Exit(0)
		case "--list":
			quiet := hasFlag("--quiet", "-q")
			jsonOut := hasFlag("--json")
			listservers.Run(quiet, jsonOut)
			os.Exit(0)
		case "--help", "-h":
			printUsage()
			os.Exit(0)
		}
	}

	// Setup logging
	logLevel := slog.LevelInfo
	switch os.Getenv("MWI_LOG_LEVEL") {
	case "DEBUG":
		logLevel = slog.LevelDebug
	case "WARNING", "WARN":
		logLevel = slog.LevelWarn
	case "ERROR":
		logLevel = slog.LevelError
	}

	var handler slog.Handler
	logFile := os.Getenv("MWI_LOG_FILE")
	if logFile != "" {
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error opening log file: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		handler = mwilog.NewFileHandler(f, logLevel)
	} else {
		handler = mwilog.NewConsoleHandler(logLevel)
	}
	logger := slog.New(handler)

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	// CLI flags override env vars for attach mode
	if v := getFlagValue("--ec-port"); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			logger.Error("invalid --ec-port value", "value", v, "error", err)
			os.Exit(1)
		}
		cfg.AttachECPort = port
	}
	if v := getFlagValue("--mwapikey"); v != "" {
		cfg.AttachMWAPIKey = v
	}

	// Validate attach mode: both ec-port and mwapikey must be provided together
	if (cfg.AttachECPort > 0) != (cfg.AttachMWAPIKey != "") {
		logger.Error("attach mode requires both --ec-port and --mwapikey (or MWI_ATTACH_EC_PORT and MWI_ATTACH_MWAPIKEY)")
		os.Exit(1)
	}

	logger.Info("configuration loaded",
		"host", cfg.Host,
		"port", cfg.Port,
		"base_url", cfg.BaseURL,
		"matlab_root", cfg.MATLABRoot,
		"matlab_version", cfg.MATLABVersion,
		"ssl", cfg.EnableSSL,
		"auth", cfg.EnableTokenAuth,
		"attach_mode", cfg.IsAttachMode(),
	)

	if cfg.MATLABCommand == "" {
		logger.Warn("Unable to find MATLAB on the system PATH. Add MATLAB to the system PATH, and restart matlab-proxy.")
	}

	// Create and start server
	srv, err := server.New(cfg, logger)
	if err != nil {
		logger.Error("failed to create server", "error", err)
		os.Exit(1)
	}

	// Handle shutdown signals
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		logger.Info("received signal, shutting down", "signal", sig)
		cancel()
		_ = srv.Shutdown(context.Background())
	}()

	if err := srv.Start(ctx); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}

func hasFlag(names ...string) bool {
	for _, arg := range os.Args[1:] {
		for _, name := range names {
			if arg == name {
				return true
			}
		}
	}
	return false
}

// getFlagValue returns the value for a --flag value pair, or "" if not found.
func getFlagValue(name string) string {
	for i, arg := range os.Args[1:] {
		if arg == name && i+1 < len(os.Args)-1 {
			return os.Args[i+2]
		}
	}
	return ""
}

func printUsage() {
	fmt.Printf(`matlab-proxy %s

Usage:
  matlab-proxy                           Start the MATLAB proxy server
  matlab-proxy --ec-port PORT --mwapikey KEY
                                         Attach to an existing MATLAB session
  matlab-proxy --list                    List running MATLAB proxy servers
  matlab-proxy --version                 Print version and exit
  matlab-proxy --help                    Print this help message

Attach mode:
  --ec-port PORT              MATLAB Embedded Connector port (env: MWI_ATTACH_EC_PORT)
  --mwapikey KEY              MATLAB API key for EC auth    (env: MWI_ATTACH_MWAPIKEY)

  Run scripts/enable_connect.m in your MATLAB session to get these values,
  or run these commands manually:

    logDir = fullfile(tempdir, 'matlab-proxy-attach');
    if ~exist(logDir,'dir'), mkdir(logDir); end
    setenv('MATLAB_LOG_DIR', logDir);
    key = lower(sprintf('%%s-%%s-%%s-%%s-%%s', ...
      dec2hex(randi([0 2^32-1]),8), dec2hex(randi([0 2^16-1]),4), ...
      dec2hex(randi([0 2^16-1]),4), dec2hex(randi([0 2^16-1]),4), ...
      dec2hex(randi([0 2^48-1]),12)));
    setenv('MWAPIKEY', key);
    setenv('MW_CONNECTOR_CONTEXT_ROOT', '');
    setenv('MW_DOCROOT', fullfile('ui','webgui','src'));
    setenv('MW_CD_ANYWHERE_ENABLED', 'true');
    setenv('MW_CD_ANYWHERE_DISABLED', 'false');
    evalc('connector.internal.Worker.start');
    key = getenv('MWAPIKEY');  %% recover after workspace clear
    logDir = getenv('MATLAB_LOG_DIR');
    portFile = fullfile(logDir, 'connector.securePort');
    for i = 1:60, if isfile(portFile), break; end, pause(0.5); end
    fprintf('--ec-port %%s --mwapikey %%s\n', strip(fileread(portFile)), key);

List options:
  --quiet, -q                 Print server URLs only (one per line)
  --json                      Output as JSON

All server configuration is done via environment variables.
See docs/usage.md for the full list of MWI_* variables.
`, version.Version)
}
