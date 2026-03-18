# Contributing to matlab-proxy-go

Thank you for your interest in contributing! This guide covers everything you need to get started.

## Prerequisites

- **Go 1.22+** — Download from [go.dev/dl](https://go.dev/dl/) or use your package manager
- **Git**
- A working **MATLAB** installation is needed to test the full server, but not required for building or running unit tests

Verify your Go installation:

```bash
go version
```

## Getting the Source

1. Fork the repository on GitHub.

2. Clone your fork:
   ```bash
   git clone https://github.com/<your-username>/matlab-proxy-go.git
   cd matlab-proxy-go
   ```

3. Add the upstream remote:
   ```bash
   git remote add upstream https://github.com/mathworks/matlab-proxy-go.git
   ```

There is no need to change the Go module path — it stays as `github.com/mathworks/matlab-proxy-go` regardless of where your fork lives. Go resolves imports from the local filesystem, not from the network, when building locally.

## Building

```bash
go build -o matlab-proxy ./cmd/matlab-proxy
```

This produces a single binary with all assets (templates, CSS, JS) embedded. There is no separate frontend build step.

Verify it works:

```bash
./matlab-proxy --version
./matlab-proxy --help
```

A local build reports `0.0.0-dev` as the version. Release versions are injected at build time by the CI pipeline.

## Running

```bash
# MATLAB must be on PATH, or set MWI_CUSTOM_MATLAB_ROOT
./matlab-proxy

# With debug logging
MWI_LOG_LEVEL=DEBUG ./matlab-proxy

# With a fixed port
MWI_APP_PORT=8080 ./matlab-proxy
```

See [docs/usage.md](docs/usage.md) for the full list of environment variables.

## Project Structure

```
cmd/matlab-proxy/          CLI entry point
internal/
  auth/                    Token authentication and session cookies
  config/                  Environment variable parsing, MATLAB discovery
  display/                 Xvfb/Fluxbox management (Linux)
  licensing/               License types (NLM, MHLM, existing), MathWorks API
  listservers/             Server discovery (--list)
  matlab/                  MATLAB process lifecycle, Embedded Connector
  proxy/                   HTTP and WebSocket reverse proxy to MATLAB
  server/                  HTTP server, routes, handlers
    static/                Embedded CSS and JS
    templates/             Embedded Go html/template
  session/                 Concurrent session tracking, idle timeout
  version/                 Build-time version injection
docs/                      Documentation
.github/workflows/         CI, lint, and release pipelines
```

## Making Changes

1. Create a branch from `main`:
   ```bash
   git checkout -b my-feature
   ```

2. Make your changes. Keep commits focused — one logical change per commit.

3. Run the checks locally before pushing:
   ```bash
   go vet ./...
   go build ./...
   go test -race ./...
   ```

4. Push and open a pull request against `main`:
   ```bash
   git push origin my-feature
   ```

## Code Style

- Follow standard Go conventions. Run `go vet` and `gofmt` (or let your editor handle it).
- Keep functions short and focused.
- Use `slog` for logging — not `fmt.Println` or `log.Println`.
- All new `.go` files must include the copyright header as the first line:
  ```go
  // Copyright 2026 The MathWorks, Inc.
  ```
  Use the current year. For files modified in a later year, update to a range: `// Copyright 2026-2027 The MathWorks, Inc.`

## Testing

```bash
# Run all tests
go test ./...

# Run tests with race detection
go test -race ./...

# Run tests for a specific package
go test ./internal/auth/...

# Run a specific test
go test -run TestTokenValidation ./internal/auth/
```

## CI Pipeline

Every push and pull request triggers:

- **CI** — `go vet`, `go build`, `go test -race` on Linux, macOS, and Windows
- **Lint** — `golangci-lint` for static analysis

Both must pass before a PR can be merged.

## Cross-Compilation

To build for a different platform:

```bash
GOOS=linux   GOARCH=amd64 go build -o matlab-proxy-linux   ./cmd/matlab-proxy
GOOS=darwin  GOARCH=arm64 go build -o matlab-proxy-macos   ./cmd/matlab-proxy
GOOS=windows GOARCH=amd64 go build -o matlab-proxy.exe     ./cmd/matlab-proxy
```

## Documentation

- Architecture and design decisions: [docs/architecture.md](docs/architecture.md)
- Differences from the Python version: [docs/differences.md](docs/differences.md)
- Release process (for maintainers): [docs/releasing.md](docs/releasing.md)

## Reporting Issues

Open an issue on GitHub with:
- What you expected to happen
- What actually happened
- Steps to reproduce
- MATLAB version and OS
- Relevant log output (`MWI_LOG_LEVEL=DEBUG`)

---

Copyright 2026 The MathWorks, Inc.
