// Copyright 2026 The MathWorks, Inc.

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
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

	logger.Info("configuration loaded",
		"host", cfg.Host,
		"port", cfg.Port,
		"base_url", cfg.BaseURL,
		"matlab_root", cfg.MATLABRoot,
		"matlab_version", cfg.MATLABVersion,
		"ssl", cfg.EnableSSL,
		"auth", cfg.EnableTokenAuth,
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
	for _, arg := range os.Args[2:] {
		for _, name := range names {
			if arg == name {
				return true
			}
		}
	}
	return false
}

func printUsage() {
	fmt.Printf(`matlab-proxy %s

Usage:
  matlab-proxy                Start the MATLAB proxy server
  matlab-proxy --list         List running MATLAB proxy servers
  matlab-proxy --version      Print version and exit
  matlab-proxy --help         Print this help message

List options:
  --quiet, -q                 Print server URLs only (one per line)
  --json                      Output as JSON

All server configuration is done via environment variables.
See docs/usage.md for the full list of MWI_* variables.
`, version.Version)
}
