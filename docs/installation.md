# Installation

## Prerequisites

- **MATLAB** installed on the target machine (R2020b or later)
- **Linux:** Xvfb (recommended for headless environments)

## Download a Pre-Built Binary

Pre-built binaries for Linux, macOS, and Windows are available on the [GitHub Releases](https://github.com/prabhakk-mw/matlab-proxy-go/releases) page.

1. Download the archive for your platform:

   | Platform | File |
   |---|---|
   | Linux (x86_64) | `matlab-proxy-v*-linux-amd64.tar.gz` |
   | Linux (ARM64) | `matlab-proxy-v*-linux-arm64.tar.gz` |
   | macOS (Apple Silicon) | `matlab-proxy-v*-darwin-arm64.tar.gz` |
   | macOS (Intel) | `matlab-proxy-v*-darwin-amd64.tar.gz` |
   | Windows (x86_64) | `matlab-proxy-v*-windows-amd64.zip` |

2. Extract and run:

   ```bash
   # Linux / macOS
   tar xzf matlab-proxy-v*-linux-amd64.tar.gz
   ./matlab-proxy --version

   # Windows (PowerShell)
   Expand-Archive matlab-proxy-v*-windows-amd64.zip
   .\matlab-proxy.exe --version
   ```

3. Optionally move the binary to a directory on your PATH:

   ```bash
   sudo mv matlab-proxy /usr/local/bin/
   ```

## Install with `go install`

> **Note:** This option will only work once the repository is hosted at `github.com/mathworks/matlab-proxy-go` (matching the Go module path). Until then, use a pre-built binary or build from source.

If you have Go 1.22+ installed, you can install directly:

```bash
go install github.com/mathworks/matlab-proxy-go/cmd/matlab-proxy@latest
```

This downloads the source, compiles it, and places the binary in `$GOPATH/bin` (typically `~/go/bin`). Make sure that directory is on your PATH.

## Building from Source

```bash
git clone https://github.com/mathworks/matlab-proxy-go.git
cd matlab-proxy-go
go build -o matlab-proxy ./cmd/matlab-proxy/
```

This produces a single binary that includes all functionality:
- `./matlab-proxy` — start the server
- `./matlab-proxy --list` — list running servers
- `./matlab-proxy --version` — print version

## Cross-Compilation

Go makes it straightforward to build for any supported platform:

```bash
# Linux (amd64)
GOOS=linux GOARCH=amd64 go build -o matlab-proxy-linux-amd64 ./cmd/matlab-proxy/

# Linux (arm64)
GOOS=linux GOARCH=arm64 go build -o matlab-proxy-linux-arm64 ./cmd/matlab-proxy/

# macOS (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -o matlab-proxy-darwin-arm64 ./cmd/matlab-proxy/

# macOS (Intel)
GOOS=darwin GOARCH=amd64 go build -o matlab-proxy-darwin-amd64 ./cmd/matlab-proxy/

# Windows
GOOS=windows GOARCH=amd64 go build -o matlab-proxy.exe ./cmd/matlab-proxy/
```

## Docker

### Minimal Dockerfile

```dockerfile
FROM golang:1.22 AS builder
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /matlab-proxy ./cmd/matlab-proxy/

FROM ubuntu:22.04
RUN apt-get update && apt-get install -y --no-install-recommends \
    xvfb \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /matlab-proxy /usr/local/bin/

# MATLAB must be mounted or installed in the image
# ENV MWI_CUSTOM_MATLAB_ROOT=/opt/matlab/R2024a

EXPOSE 8888
CMD ["matlab-proxy"]
```

Build and run:

```bash
docker build -t matlab-proxy-go .
docker run -p 8888:8888 \
  -e MWI_APP_PORT=8888 \
  -v /path/to/matlab:/opt/matlab/R2024a \
  -e MWI_CUSTOM_MATLAB_ROOT=/opt/matlab/R2024a \
  matlab-proxy-go
```

### Distroless (smallest image)

For environments where image size matters, you can use a distroless base. Note that Xvfb will not be available, so MATLAB graphics will not work in headless mode.

```dockerfile
FROM golang:1.22 AS builder
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /matlab-proxy ./cmd/matlab-proxy/

FROM gcr.io/distroless/static-debian12
COPY --from=builder /matlab-proxy /
ENTRYPOINT ["/matlab-proxy"]
```

## Verifying the Installation

```bash
# Check the binary runs
./matlab-proxy --version
./matlab-proxy --help

# List running servers (should show none initially)
./matlab-proxy --list
```

## System Dependencies

### Linux
- **Xvfb** — Required for MATLAB graphics in headless (no-display) environments. Install with `apt-get install xvfb` or `yum install xorg-x11-server-Xvfb`.
- **Fluxbox** (optional) — Required only for Simulink Online support. Install with `apt-get install fluxbox`.

### macOS
- No additional system dependencies. MATLAB handles its own display.
- Works best with MATLAB R2022b or later.

### Windows
- No additional system dependencies. The Xvfb/Fluxbox display layer is automatically disabled on Windows.

---

Copyright 2026 The MathWorks, Inc.
