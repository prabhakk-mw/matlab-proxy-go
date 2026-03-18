# matlab-proxy-go

A Go implementation of [matlab-proxy](https://github.com/mathworks/matlab-proxy) — a web server that launches MATLAB and provides browser-based access to it.

This project is a from-scratch rewrite of the original Python package. It produces a single, statically-linked binary with no runtime dependencies (no Python, no Node.js, no npm), while preserving API compatibility with the original.

Key features:
- All three licensing types: existing license, Network License Manager (NLM), MathWorks Online License (MHLM)
- Token-based authentication with port-scoped session cookies
- Concurrent browser session protection with automatic crash recovery
- WebSocket proxy for MATLAB's Embedded Connector
- Idle timeout with automatic shutdown
- Xvfb/Fluxbox management for headless Linux environments
- Clean shutdown with artifact cleanup

## Quick Start

Download a pre-built binary from the [Releases](https://github.com/prabhakk-mw/matlab-proxy-go/releases) page, or build from source:

```bash
git clone https://github.com/prabhakk-mw/matlab-proxy-go.git
cd matlab-proxy-go
go build -o matlab-proxy ./cmd/matlab-proxy/

# Run (MATLAB must be on PATH or set MWI_CUSTOM_MATLAB_ROOT)
./matlab-proxy
```

The server prints an access URL on startup. Open it in your browser.

## Documentation

| Document | Description |
|---|---|
| [Architecture](docs/architecture.md) | System design, component map, request flows |
| [Installation](docs/installation.md) | Building from source, cross-compilation, Docker |
| [Usage](docs/usage.md) | Configuration, environment variables, CLI flags |
| [Differences from matlab-proxy](docs/differences.md) | What changed, what was removed, what was added |
| [Releasing](docs/releasing.md) | Version strategy, tagging, pre-releases |
| [TODO](docs/TODO.md) | Known gaps and planned improvements |
| [Contributing](CONTRIBUTING.md) | Development setup, building, testing, code style |

## When to Use This Instead of the Python Version

This Go rewrite may be a better fit than the original [matlab-proxy](https://github.com/mathworks/matlab-proxy) in the following scenarios:

### Single-binary deployment
The Go version compiles to a single ~14 MB binary with all assets embedded. There is no need to install Python, pip, Node.js, or npm. This is particularly useful for:
- Minimal container images (scratch / distroless / Alpine)
- Air-gapped environments where installing Python packages is difficult
- Appliance-style deployments where you want one file to copy and run

### Lower resource footprint
Go's concurrency model (goroutines) and compiled nature result in lower memory usage and faster startup compared to the Python asyncio server. If you are running many proxy instances (e.g., one per user in a shared cluster), the per-instance overhead matters.

### Simplified operations
No virtual environments, no `pip install`, no `package.json`. The binary is self-contained and can be managed with standard process supervisors (systemd, supervisord, Kubernetes).

### Cross-compilation
A single `GOOS=linux GOARCH=amd64 go build` command produces a binary for any target platform. No need to worry about platform-specific Python wheels or native dependencies.

### When the Python version is still the better choice
- You need **matlab-proxy-manager** for multi-instance management (shared/isolated MATLAB sessions for Jupyter kernels). The Go version does not include this component.
- You need the **development/test mode** with the fake MATLAB server for local testing without a real MATLAB installation.
- You need deep integration with **Jupyter** via the Python entry-point plugin system.
- You rely on third-party **Python-based integrations** that register custom configurations via `matlab_proxy_configs` entry points.

## License

See [LICENSE](LICENSE) for details.

---

Copyright 2026 The MathWorks, Inc.
