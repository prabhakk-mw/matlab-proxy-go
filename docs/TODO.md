# TODO

Known gaps and planned improvements for matlab-proxy-go.

## High Priority

### Testing
- [x] Unit tests for `internal/auth` — token generation, validation, middleware, stale cookie expiry
- [x] Unit tests for `internal/config` — env var parsing (MATLAB discovery and SSL cert generation still TODO)
- [ ] Unit tests for `internal/licensing` — all three license types, MHLM API mocking, persistence, cache restore with expiry validation
- [x] Unit tests for `internal/session` — concurrency control (active client expiry, transfer, clear), idle timeout countdown
- [ ] Unit tests for `internal/matlab` — state transitions, EC communication mocking, session file cleanup
- [ ] Integration tests — start server, hit API endpoints, verify responses
- [ ] WebSocket proxy tests — bidirectional message forwarding, error handling

### Windows process management
- [ ] Use `taskkill /T /F /PID` for process tree termination on Windows (currently uses basic `Process.Kill` which may leave child processes orphaned)
- [ ] Test MATLAB startup flags on Windows (`-noDisplayDesktop`, `-wait`, `-log`)
- [ ] Handle Windows-specific path separators in MATLAB command construction

### macOS support
- [ ] Verify MATLAB startup behavior on macOS (no Xvfb needed)
- [ ] Test with MATLAB R2022b+ where macOS support is best

## Medium Priority

### MHLM improvements
- [ ] Handle MHLM token refresh — when the identity token expires during a long session, re-prompt the user to log in
- [ ] Add `wsEnv` support for integration environments (currently hardcoded to `prod`)
- [ ] Parse and surface entitlement error details from MathWorks API responses in the UI

### Frontend improvements
- [ ] Entitlement selector dropdown — when MHLM returns multiple entitlements, show a picker (currently the template only supports auto-select for single entitlements)
- [ ] Error log display — show collapsible MATLAB stderr logs in the error section
- [ ] Warning display — show warning banners with dismiss capability
- [x] ~~Concurrent session UI — show a dialog when another client is active, with "Transfer Session" button~~ (implemented: timestamp-based active client model with "Open MATLAB Here" / "Session Moved" dialogs)
- [ ] Mobile-responsive layout adjustments

### Server info file
- [ ] Write the `mwi_server.info` file atomically (write to temp file, then rename) to prevent partial reads
- [ ] Include the MATLAB version in the browser title written to the info file (format: `<session> - MATLAB <version>`)
- [ ] Clean up stale info files from crashed servers on startup

### NLM validation
- [ ] Validate NLM connection string format (`port@hostname` or `hostname`) before passing to MATLAB
- [ ] Parse NLM-specific errors from MATLAB stderr (e.g., "License checkout failed")

### MHLM error parsing
- [ ] Parse MHLM-specific errors from MATLAB stderr (e.g., "License Manager Error")
- [ ] Surface these as typed errors in the `/get_status` response

### Logging
- [ ] Add JSON log format option for structured log aggregation
- [ ] Add request ID middleware for correlating log entries across a request lifecycle
- [ ] Rotate log files when `MWI_LOG_FILE` is set

### Cleanup
- [ ] Remove debug `console.log` statements from `app.js`
- [ ] Remove debug `fmt.Printf` statements from `auth/token.go`

## Low Priority

### Performance
- [ ] Connection pooling for EC HTTP requests (currently creates new connections)
- [ ] Benchmark WebSocket proxy throughput vs direct connection
- [ ] Profile memory usage under sustained load (many WebSocket messages)

### Operational
- [ ] Health check endpoint (`/healthz`) for Kubernetes liveness/readiness probes
- [ ] Prometheus metrics endpoint (`/metrics`) — request latency, MATLAB state, connection counts
- [ ] Graceful drain on shutdown — wait for in-flight WebSocket messages before closing

### Packaging
- [x] GitHub Actions CI pipeline — build, test, vet, lint for Linux/macOS/Windows
- [x] GitHub Actions release pipeline — cross-compile and upload binaries on tag push
- [ ] Homebrew formula for macOS
- [ ] Systemd unit file for Linux
- [ ] Dockerfile with multi-stage build published to a container registry

### Feature parity with Python version
- [ ] Development/test mode with a fake MATLAB server for integration testing
- [ ] Cookie jar caching (`MWI_USE_COOKIE_CACHE`)
- [ ] Custom configuration plugins (Go plugin system or YAML config file as alternative to Python entry points)
- [ ] Jupyter integration helper (script or documentation for wiring into JupyterHub)

### Code quality
- [ ] Linting with `golangci-lint` (errcheck, gosec, govet, staticcheck)
- [ ] Race condition testing (`go test -race ./...`)
- [ ] Fuzz testing for token validation and URL parsing
- [ ] Document all exported types and functions (godoc)

## Completed

- [x] Shutdown cleanup — `connector.securePort` and `matlab_scripts/` removed on exit, MATLAB logs preserved
- [x] MHLM cache restore — expiry validation with multiple date formats, entitlement re-fetch, proper error handling
- [x] MHLM auto-start — shared `prepareMATLABEnv()` fetches fresh access token before MATLAB starts
- [x] Dynamic UI updates — licensing info refreshed from polled `/get_status` data
- [x] Auth token flow — URL token → cookie check → token input screen → cookie set on success
- [x] Port-scoped session cookies — `mwi-auth-session-<port>` prevents cross-session cookie bleed
- [x] Stale cookie middleware — expires cookies from previous server sessions on same port
- [x] Concurrent browser session handling — timestamp-based active client with 10-second expiry, transfer dialogs

## Out of Scope

These features are intentionally not planned:

- **matlab-proxy-manager** — The multi-instance manager is out of scope for this project. Use the Python version if you need it.
- **Jupyter entry-point integration** — Go binaries cannot register as Python entry points. Alternative approaches (documentation, wrapper scripts) may be provided.
- **Python API compatibility** — This is not a drop-in replacement at the Python import level. It is a replacement at the HTTP API and CLI level.

---

Copyright 2026 The MathWorks, Inc.
