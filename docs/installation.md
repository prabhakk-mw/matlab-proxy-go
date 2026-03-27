# Installation

## Prerequisites

- **MATLAB** installed on the target machine (R2020b or later)
- **Linux:** Xvfb (recommended for headless environments)

## Install Script

The easiest way to install. The script detects your OS and architecture, fetches the latest release from GitHub, and installs the binary to `~/.local/bin`.

### Linux / macOS

```bash
curl -fsSL https://raw.githubusercontent.com/prabhakk-mw/matlab-proxy-go/main/install.sh | sh
```

Customize with environment variables:

```bash
# Install a specific version
curl -fsSL https://raw.githubusercontent.com/prabhakk-mw/matlab-proxy-go/main/install.sh | VERSION=0.5.1 sh

# Install to a custom directory
curl -fsSL https://raw.githubusercontent.com/prabhakk-mw/matlab-proxy-go/main/install.sh | INSTALL_DIR=/usr/local/bin sh

# Both
curl -fsSL https://raw.githubusercontent.com/prabhakk-mw/matlab-proxy-go/main/install.sh | VERSION=0.5.1 INSTALL_DIR=/usr/local/bin sh
```

The script uses `sudo` only if the install directory is not writable by the current user.

### Windows (PowerShell)

```powershell
powershell -ExecutionPolicy ByPass -c "irm https://raw.githubusercontent.com/prabhakk-mw/matlab-proxy-go/main/install.ps1 | iex"
```

Customize with environment variables:

```powershell
# Install a specific version
$env:VERSION = "0.5.1"; irm https://raw.githubusercontent.com/prabhakk-mw/matlab-proxy-go/main/install.ps1 | iex

# Install to a custom directory
$env:INSTALL_DIR = "C:\Tools"; irm https://raw.githubusercontent.com/prabhakk-mw/matlab-proxy-go/main/install.ps1 | iex

# Don't modify PATH
$env:MWI_NO_MODIFY_PATH = "1"; irm https://raw.githubusercontent.com/prabhakk-mw/matlab-proxy-go/main/install.ps1 | iex
```

The script automatically adds the install directory to your user-level `PATH` via the Windows registry. Restart your shell or run the printed command to pick up the change immediately.

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

## Install with a Package Manager (Linux)

On Linux, `.deb` and `.rpm` packages are published with each release.

### Debian / Ubuntu (apt / dpkg)

```bash
# Download the .deb for your architecture from the latest release
# For amd64:
curl -LO "https://github.com/prabhakk-mw/matlab-proxy-go/releases/latest/download/matlab-proxy_VERSION_amd64.deb"

# Or use the gh CLI:
gh release download --repo prabhakk-mw/matlab-proxy-go --pattern '*.deb'

# Install
sudo dpkg -i matlab-proxy_*_amd64.deb

# Verify
matlab-proxy --version
```

### RHEL / Fedora / Amazon Linux (rpm)

```bash
# Download the .rpm for your architecture from the latest release
curl -LO "https://github.com/prabhakk-mw/matlab-proxy-go/releases/latest/download/matlab-proxy-VERSION.amd64.rpm"

# Or use the gh CLI:
gh release download --repo prabhakk-mw/matlab-proxy-go --pattern '*.rpm'

# Install
sudo rpm -i matlab-proxy-*.amd64.rpm

# Verify
matlab-proxy --version
```

Both packages install the binary to `/usr/local/bin/matlab-proxy`.

### Uninstall

```bash
# Debian / Ubuntu
sudo dpkg -r matlab-proxy

# RHEL / Fedora
sudo rpm -e matlab-proxy
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
go build -ldflags "-s -w" -o bin/matlab-proxy ./cmd/matlab-proxy/
```

This produces a single binary in `bin/` that includes all functionality:
- `./bin/matlab-proxy` — start the server
- `./bin/matlab-proxy --list` — list running servers
- `./bin/matlab-proxy --version` — print version

## Cross-Compilation

Go makes it straightforward to build for any supported platform:

```bash
# Linux (amd64)
GOOS=linux GOARCH=amd64 go build -o bin/matlab-proxy-linux-amd64 ./cmd/matlab-proxy/

# Linux (arm64)
GOOS=linux GOARCH=arm64 go build -o bin/matlab-proxy-linux-arm64 ./cmd/matlab-proxy/

# macOS (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -o bin/matlab-proxy-darwin-arm64 ./cmd/matlab-proxy/

# macOS (Intel)
GOOS=darwin GOARCH=amd64 go build -o bin/matlab-proxy-darwin-amd64 ./cmd/matlab-proxy/

# Windows
GOOS=windows GOARCH=amd64 go build -o bin/matlab-proxy.exe ./cmd/matlab-proxy/
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
./bin/matlab-proxy --version
./bin/matlab-proxy --help

# List running servers (should show none initially)
./bin/matlab-proxy --list
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
