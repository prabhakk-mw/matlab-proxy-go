# Architecture

This document describes the internal architecture of matlab-proxy-go.

## High-Level Overview

matlab-proxy-go is an HTTP server that sits between a web browser and a MATLAB process. It:

1. Manages the MATLAB process lifecycle (start, stop, restart)
2. Handles licensing (three types)
3. Serves a web UI for status and control
4. Proxies HTTP and WebSocket traffic to MATLAB's Embedded Connector

```
                          Browser
                            |
                       HTTP / WebSocket
                            |
               +------------+------------+
               |   matlab-proxy-go       |
               |   (chi router)          |
               |                         |
               |  +-------------------+  |
               |  | API Handlers      |  |
               |  | /get_status       |  |
               |  | /start_matlab     |  |
               |  | /stop_matlab      |  |
               |  | /set_licensing    |  |
               |  | /get_env_config   |  |
               |  | /authenticate     |  |
               |  | ...               |  |
               |  +-------------------+  |
               |                         |
               |  +-------------------+  |
               |  | Proxy Layer       |  |
               |  | HTTP forwarding   |  |
               |  | WebSocket bridge  |  |
               |  +-------------------+  |
               |           |             |
               +-----------|-------------+
                           |
                     HTTPS / WSS
                           |
               +-----------+-----------+
               | MATLAB Process        |
               | Embedded Connector    |
               | (port from .securePort|
               |  file)                |
               +-----------+-----------+
                           |
               +-----------+-----------+
               | Xvfb      | Fluxbox   |
               | (Linux)   | (Simulink)|
               +-----------+-----------+
```

## Package Structure

```
matlab-proxy-go/
├── cmd/
│   ├── matlab-proxy/main.go         Entry point for the proxy server
│   └── list-servers/main.go         Entry point for the server discovery CLI
│
├── internal/
│   ├── config/                      Configuration and environment variables
│   │   ├── config.go                Settings struct, MATLAB discovery, SSL, port allocation
│   │   └── env.go                   Environment variable names and helper functions
│   │
│   ├── auth/
│   │   └── token.go                 Token generation, validation, HTTP middleware, session cookies
│   │
│   ├── matlab/
│   │   ├── state.go                 Status and BusyStatus type definitions
│   │   ├── process.go               MATLAB process lifecycle (start/stop/restart), PTY, env setup
│   │   ├── connector.go             Embedded Connector communication (ping, busy, exit)
│   │   ├── pty_unix.go              PTY allocation for POSIX systems (MATLAB requires a PTY as stdin)
│   │   ├── pty_windows.go           No-op PTY stub for Windows
│   │   └── scripts/                 Embedded MATLAB scripts (startup.m, evaluateUserMatlabCode.m)
│   │
│   ├── licensing/
│   │   ├── licensing.go             License manager, persistence, validation, cached config restore
│   │   └── mhlm.go                  MathWorks API calls (expand token, access token, entitlements)
│   │
│   ├── display/
│   │   ├── display.go               Xvfb and Fluxbox management (Linux)
│   │   └── display_windows.go       No-op stub for Windows
│   │
│   ├── proxy/
│   │   ├── http.go                  HTTP reverse proxy to Embedded Connector
│   │   └── websocket.go             Bidirectional WebSocket proxy
│   │
│   ├── session/
│   │   └── session.go               Client concurrency control and idle timeout
│   │
│   ├── terminal/
│   │   ├── handler.go               WebSocket handler, PTY-shell bridge
│   │   ├── pty_unix.go              PTY allocation for Linux
│   │   ├── pty_darwin.go            PTY allocation for macOS
│   │   └── pty_windows.go           No-op stub for Windows
│   │
│   └── server/
│       ├── server.go                HTTP server, route setup, all API handlers
│       ├── serverinfo.go            Server info file write/remove for discovery
│       ├── templates.go             Go html/template renderer
│       ├── templates/index.html     Server-rendered UI with HTML templates
│       └── static/                  CSS, JavaScript, icons (embedded in binary)
│
├── docs/                            Documentation
├── go.mod
└── go.sum
```

## Component Details

### Config (`internal/config`)

Loaded once at startup. Reads all `MWI_*` environment variables, discovers MATLAB on the system (via `MWI_CUSTOM_MATLAB_ROOT` or `$PATH`), detects the MATLAB version from `VersionInfo.xml`, allocates a free port if none specified, and optionally generates a self-signed TLS certificate.

Key design decision: configuration is immutable after `Load()` returns, with the exception of `NLMConnStr` which can be updated when the user sets NLM licensing at runtime.

### Auth (`internal/auth`)

Generates a cryptographically random token at startup (or uses `MWI_AUTH_TOKEN`). Validates tokens from three sources in order: session cookie, URL query parameter, HTTP header. Uses constant-time comparison (`hmac.Equal`) to prevent timing attacks. Accepts both the raw token and its SHA-256 hash.

**Session cookies:** After successful authentication, a session cookie (`mwi-auth-session-<port>`) is set. The cookie name includes the server port to prevent cross-session cookie bleed when multiple instances run on the same host. A `ClearStaleCookieMiddleware` runs on every request to expire cookies from previous server sessions that used the same port with a different token.

**Frontend auth flow:** When a user visits without a token in the URL, the frontend calls `POST /authenticate` to check if a valid session cookie exists. If not, an auth token input screen is shown. The token is validated server-side, and a session cookie is set on success.

The `Middleware()` function returns a standard `http.Handler` middleware compatible with chi.

### MATLAB Process (`internal/matlab`)

The process manager implements a state machine:

```
     Start()              EC ready
down --------> starting ----------> up
  ^                |                |
  |    Stop()      |    Stop()      |
  +----------------+----------------+
```

**Start flow:**
1. Set state to `starting`, clear errors
2. Start Xvfb (Linux only)
3. Start Fluxbox if Simulink enabled (Linux only)
4. Allocate a PTY for MATLAB's stdin (POSIX only — MATLAB requires a pseudo-terminal)
5. Generate an `MWAPIKEY` UUID for the Embedded Connector
6. Write embedded `startup.m` and `evaluateUserMatlabCode.m` scripts to the logs directory
7. Spawn MATLAB subprocess with appropriate flags (`-nosplash`, `-nodesktop`, `-softwareopengl`, `-externalUI`, `-r <startup code>`) and environment variables (`MATLAB_LOG_DIR`, `MWAPIKEY`, `MW_CONNECTOR_CONTEXT_ROOT`, etc.)
8. Read stderr in a background goroutine
9. Monitor process exit in a background goroutine (detects early death)
10. Wait for `connector.securePort` file to appear (contains EC port)
11. Poll the EC with ping requests until it responds
12. Set state to `up`, begin busy status polling

**Stop flow:**
1. If MATLAB is `up`, send an exit command to the EC via HTTP
2. If that fails or MATLAB is still `starting`, cancel the context and force-kill the process
3. Wait for the run goroutine to finish
4. Clean up display processes

**Shutdown cleanup:** When MATLAB exits (or the server shuts down), session artifacts are cleaned up:
- `connector.securePort` is removed (stale file would confuse the next launch on the same port)
- `matlab_scripts/` directory is removed (extracted startup scripts)
- MATLAB log files are preserved for debugging

The `EmbeddedConnector` type handles all communication with MATLAB's built-in HTTP server, including ping, busy status queries, and exit commands. It injects the `MWAPIKEY` as an HTTP header on all requests to the EC.

### Licensing (`internal/licensing`)

Three licensing types are supported:

| Type | How It Works |
|---|---|
| **Existing License** | MATLAB is already activated on the machine. No additional credentials needed. |
| **NLM** (Network License Manager) | User provides a `port@hostname` connection string. Passed to MATLAB via `MLM_LICENSE_FILE` env var and `-licmode file` flag. |
| **MHLM** (MathWorks Hosted License Manager) | User logs in via MathWorks iframe. Server exchanges identity token for access token and fetches entitlements from MathWorks APIs. |

The MHLM flow involves three API calls to MathWorks services:
1. **Expand token** — validates the identity token, returns expiry and user details
2. **Fetch access token** — exchanges identity token for a short-lived access token
3. **Fetch entitlements** — retrieves available MATLAB licenses (XML `<describe_entitlements_response>`)

If only one entitlement is returned, it is auto-selected. Otherwise, the user picks from the UI.

**Licensing persistence:** State is saved to `~/.matlab/MWI/proxy_app_config.json` so it survives server restarts. On startup, cached licensing is restored:
- **NLM:** Connection string is propagated back to the config so MATLAB gets `-licmode file` and `MLM_LICENSE_FILE`.
- **MHLM:** The identity token's expiry is validated (must have >1 hour remaining). If valid, entitlements are re-fetched using the cached identity token (the access token is short-lived and cannot be cached). Multiple expiry date formats from the MathWorks API are supported. If the token has expired, the cache is deleted and the user is prompted to log in again.
- **Existing License:** Restored directly.

**MHLM at MATLAB start time:** When MATLAB starts with MHLM licensing, a fresh access token is fetched using the stored identity token. This happens both on auto-start (from cached config) and manual start (from the UI). The access token is passed to MATLAB via `MLM_WEB_USER_CRED`.

### Proxy (`internal/proxy`)

**HTTP proxy:** Forwards all non-API requests to the Embedded Connector. Copies headers bidirectionally. No timeout (MATLAB requests can be long-running). Adds `X-Forwarded-Proto: http` header. Injects the `MWAPIKEY` header so the EC accepts the proxied requests.

**WebSocket proxy:** Upgrades the client connection, opens a parallel WebSocket to the EC (with `MWAPIKEY` header), and runs two goroutines to forward messages in each direction. Supports both text and binary messages. Read limit set to 500 MB per message. Handles normal closure, going-away, and abnormal closure gracefully.

### Session (`internal/session`)

**Concurrency control:** Prevents MATLAB from being displayed in multiple browser windows simultaneously (MATLAB's Embedded Connector does not support concurrent viewers). Uses a timestamp-based active client model:

- Each browser tab generates a unique `clientId` and sends it with every `/get_status` poll.
- The server tracks a single `activeClientID` with a `lastSeen` timestamp.
- The first tab to poll claims ownership. Subsequent polls from the same client refresh the timestamp.
- If the active client stops polling for more than 10 seconds (tab closed, browser crashed, network loss), it is automatically expired and the next poll from any client takes over.
- A tab can forcibly claim the session by polling with `TRANSFER_SESSION=true`.
- On tab close, `navigator.sendBeacon` sends a `/clear_client_id` request for immediate release.

This is an improvement over the Python version, which used a background polling loop (`detect_active_client_status`) to check a boolean flag. See [Differences](differences.md#concurrency-control) for details.

**Frontend concurrency UX:**
- **Active tab** — shows the MATLAB iframe normally.
- **Inactive tab (never had the session)** — shows a dialog: "MATLAB is Open Elsewhere" with **"Open MATLAB Here"** (transfers session) and **"Cancel"** buttons.
- **Tab that lost the session** — shows a dialog: "Session Moved" with a **"Reclaim Session"** button.
- Concurrency dialogs cannot be dismissed by clicking the backdrop — the user must explicitly choose an action.

**Idle timeout:** A background goroutine decrements a counter every second. Any API request resets the counter. When it reaches zero, the server shuts down. The timeout is configured via `MWI_SHUTDOWN_ON_IDLE_TIMEOUT` (in minutes).

### Server (`internal/server`)

Uses chi for routing. All templates and static assets are embedded in the binary via `embed.FS`.

**Route table:**

| Method | Path | Auth Required | Description |
|---|---|---|---|
| GET | `/` | No | Serve the web UI |
| GET | `/get_status` | No | MATLAB state, licensing, errors, active client status |
| POST | `/authenticate` | No | Validate auth token, set session cookie |
| GET | `/get_env_config` | No | Frontend configuration |
| GET | `/get_auth_token` | Yes | Return the auth token |
| PUT | `/start_matlab` | Yes | Start or restart MATLAB |
| DELETE | `/stop_matlab` | Yes | Stop MATLAB |
| PUT | `/set_licensing_info` | Yes | Configure licensing |
| PUT | `/update_entitlement` | Yes | Select MHLM entitlement |
| DELETE | `/set_licensing_info` | Yes | Remove licensing |
| DELETE | `/shutdown_integration` | Yes | Shut down the server |
| POST | `/clear_client_id` | Yes | Release active client |
| GET | `/terminal/ws` | Yes | WebSocket terminal (shell over PTY) |
| `*` | `/*` | Yes | Proxy to MATLAB EC (HTTP or WebSocket) |

**Middleware stack** (applied in order):
1. `RealIP` — extract client IP from `X-Forwarded-For`
2. `Recoverer` — catch panics and return 500
3. `Logger` — HTTP access logging (optional, enabled via `MWI_ENABLE_WEB_LOGGING`)
4. `customHeadersMiddleware` — inject custom HTTP headers from `MWI_CUSTOM_HTTP_HEADERS`
5. `ClearStaleCookieMiddleware` — expire auth cookies from previous server sessions on the same port

### Server Discovery (`cmd/list-servers`)

Each running server writes a `mwi_server.info` file to:
```
~/.matlab/MWI/hosts/<hostname>/ports/<port>/mwi_server.info
```

The `matlab-proxy-list-servers` binary scans for these files and displays them. This is compatible with the Python version's `matlab-proxy-app-list-servers` command.

### Terminal (`internal/terminal`)

Provides an interactive system shell in the browser via a WebSocket connection. When a client connects to `/terminal/ws`, the handler:

1. Upgrades the HTTP connection to a WebSocket.
2. Spawns the user's shell (`$SHELL`, `bash`, or `sh`) attached to a PTY.
3. Bridges I/O between the WebSocket and the PTY master fd:
   - **Text messages** carry terminal data bidirectionally.
   - **Binary messages** from the client carry resize commands (JSON `{"cols": N, "rows": N}`), which are applied via `TIOCSWINSZ` ioctl.
4. When the WebSocket closes (tab closed, network loss), the shell process is killed.

PTY allocation is platform-specific: Linux uses `TIOCSPTLCK`/`TIOCGPTN` ioctls, macOS uses `TIOCPTYGRANT`/`TIOCPTYUNLK`/`TIOCPTYGNAME`. Windows is not supported (stub returns an error).

The frontend uses [xterm.js](https://github.com/xtermjs/xterm.js) v5.5.0 (embedded in the binary) with the fit addon for automatic column/row sizing. The terminal is presented as a VS Code-style bottom drawer with three states: closed, open (split-screen with draggable divider), and minimized (thin bar, shell keeps running). Keyboard shortcut: ``Ctrl+` ``.

See [Web Terminal](terminal.md) for user-facing documentation.

### Frontend

The web UI is server-rendered using Go's `html/template` package with vanilla JavaScript (~450 lines). It replaces the Python version's React + Redux frontend (~3,200 lines of JSX/JS). The UI provides:

- **Authentication gate** — token input screen when accessing without a valid token or cookie
- A draggable status trigger button (color-coded: green=up, yellow=starting, red=down)
- A status panel overlay (MATLAB status, version, licensing info — dynamically updated from poll data)
- Control buttons (start, stop, restart, change license, sign out, shutdown)
- A licensing configuration panel with tabs for each license type
- An embedded MATLAB iframe (when MATLAB is running and this tab is the active client)
- MHLM login via MathWorks embedded login iframe with full `postMessage` protocol (init → nonce → load → login)
- **Concurrent session dialogs** — "Open MATLAB Here" / "Session Moved" when another tab/browser owns the session
- Idle timeout countdown warning
- Confirmation dialogs for destructive actions (stop MATLAB, sign out, shutdown)

Status is polled every 1 second via `GET /get_status`. The MATLAB iframe is inserted/removed dynamically based on MATLAB state and active client status.

---

Copyright 2026 The MathWorks, Inc.
