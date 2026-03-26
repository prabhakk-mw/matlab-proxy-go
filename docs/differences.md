# Differences from matlab-proxy (Python)

This document describes the differences between this Go implementation and the original [matlab-proxy](https://github.com/mathworks/matlab-proxy) Python package.

## What Is the Same

These aspects are fully compatible between the two implementations:

- **API contract** — All 12 HTTP endpoints use the same paths, methods, request/response formats, and status codes. A frontend built for the Python version would work against the Go version.
- **Environment variables** — All `MWI_*` environment variables are supported with the same names, defaults, and semantics.
- **Licensing types** — NLM, MHLM, and existing license all work the same way, including the MathWorks API calls for MHLM.
- **Server discovery** — Both versions write `mwi_server.info` files in the same format to the same directory structure (`~/.matlab/MWI/hosts/<host>/ports/<port>/`). The Go `matlab-proxy-list-servers` can discover Python servers and vice versa.
- **MATLAB process management** — Same startup flags, same state machine (down/starting/up), same Embedded Connector communication protocol (ping, busy status, exit).
- **Token authentication** — Same token format, same validation (header, query param, SHA-256 hash), same constant-time comparison.
- **Xvfb/Fluxbox** — Same display setup on Linux for headless graphics and Simulink.
- **Idle timeout** — Same behavior: configurable via `MWI_SHUTDOWN_ON_IDLE_TIMEOUT`, countdown in seconds, auto-shutdown when it reaches zero.
- **Concurrency control** — Same single-active-client model with session transfer support.
- **SSL/TLS** — Same self-signed cert generation when no cert files are provided.
- **License persistence** — Same cache file location (`~/.matlab/MWI/proxy_app_config.json`), same JSON format.

## What Changed

### Frontend rewrite

| Aspect | Python version | Go version |
|---|---|---|
| Framework | React 16 + Redux + Redux-Thunk | Go `html/template` + vanilla JS |
| Build toolchain | Node.js + npm + Vite | None (embedded in Go binary) |
| Lines of frontend code | ~3,200 (JSX/JS) + ~500 (CSS) | ~450 (JS) + ~310 (CSS) + ~220 (HTML template) |
| State management | Redux store with 29 action types | Server-side state, polled via fetch |
| Dependencies | react, redux, react-draggable, react-tooltip, reselect | None |

The UI looks and behaves the same from the user's perspective. The same screens, buttons, and workflows are present. The visual design was refreshed with a dark theme.

### Web framework

| Aspect | Python version | Go version |
|---|---|---|
| Framework | aiohttp (asyncio) | chi (net/http) |
| Concurrency model | Python asyncio event loop | Go goroutines |
| Session management | Fernet-encrypted cookies (aiohttp-session) | Port-scoped session cookies with token validation |

### Authentication flow

| Aspect | Python version | Go version |
|---|---|---|
| Session cookies | Generic `aiohttp-session` cookie | Port-scoped: `mwi-auth-session-<port>` |
| Cookie isolation | Single cookie name shared across all instances | Port in cookie name prevents cross-session bleed |
| Stale cookie handling | Not explicitly handled | `ClearStaleCookieMiddleware` expires cookies from previous server sessions on the same port |
| Auth gate | React component checks Redux store | Server-rendered page, JS calls `POST /authenticate` to check for valid session cookie, shows token input if needed |
| Token submission | React form dispatches Redux action | Vanilla JS `POST /authenticate` with token in header, cookie set on success |

The port-scoped cookie approach solves a real problem: when running multiple matlab-proxy instances on the same host (common in shared environments), a cookie set by one instance on `localhost` could be sent to another instance on a different port. The Python version used Fernet-encrypted session cookies which contained instance-specific data, but didn't explicitly prevent cookies from one session interfering with another. The Go version solves this by including the port in the cookie name itself.

### Concurrency control

This is one of the most significant architectural improvements. Both versions prevent MATLAB from being displayed in multiple browser tabs simultaneously (the Embedded Connector does not support concurrent viewers), but the mechanisms are very different.

**Python version — background polling loop:**

The Python version uses a background asyncio task (`detect_active_client_status`) that runs in a loop, checking a boolean flag set by client requests:

1. Each browser tab sends its `clientId` with status polls.
2. The server stores the `clientId` in a `state` dict.
3. A background task (`detect_active_client_status`) runs in an infinite loop, checking if the active client has changed by comparing a boolean flag.
4. Session transfer is implemented by another background task that sets flags and waits for the original client to release.
5. When a tab closes, it calls an endpoint to clear the flag.

Issues with this approach:
- The background loop runs continuously even when no clients are connected.
- There is no timestamp-based expiry — if a client crashes without calling the cleanup endpoint, the session is stuck until another client explicitly requests a transfer.
- The flag-checking loop introduces latency: the new client may have to wait for the next loop iteration to detect the change.
- Multiple layers of boolean flags and background tasks make the state transitions harder to reason about.

**Go version — timestamp-based expiry:**

The Go version uses a simpler timestamp-based model with no background goroutine for concurrency:

1. Each browser tab generates a unique `clientId` and sends it with every `/get_status` poll (every 1 second).
2. The server tracks a single `activeClientID` with a `lastSeen` timestamp.
3. The first tab to poll claims ownership. Subsequent polls from the same client refresh the timestamp.
4. If the active client stops polling for more than 10 seconds (tab closed, browser crashed, network loss), it is automatically expired and the next poll from any client takes over.
5. A tab can forcibly claim the session by polling with `TRANSFER_SESSION=true`.
6. On tab close, `navigator.sendBeacon` sends a `/clear_client_id` request for immediate release.

Advantages:
- **No background goroutine** — expiry is checked lazily on each poll, so there is zero overhead when no clients are connected.
- **Automatic crash recovery** — if a client crashes without calling cleanup, the 10-second timeout ensures the session is released. No manual intervention or explicit transfer needed.
- **Simpler state machine** — one timestamp comparison replaces multiple boolean flags and background loops.
- **Deterministic latency** — the next poll immediately detects the expired client, no waiting for a loop iteration.
- **Cleaner frontend UX** — the UI shows distinct dialogs for "MATLAB is Open Elsewhere" (never had the session) vs "Session Moved" (lost the session), with explicit transfer/reclaim buttons.

### Shutdown cleanup

| Aspect | Python version | Go version |
|---|---|---|
| `connector.securePort` cleanup | Left on disk (can confuse next launch) | Removed on shutdown and process exit |
| Extracted scripts cleanup | Left on disk | `matlab_scripts/` directory removed |
| MATLAB log files | Preserved | Preserved (for debugging) |
| Server info file | Removed on clean shutdown | Removed on clean shutdown |

The Go version cleans up `connector.securePort` and extracted MATLAB scripts when the server shuts down or MATLAB exits. This prevents stale files from confusing subsequent launches on the same port. MATLAB log files are intentionally preserved so users can debug issues like crashes.

### MHLM cache restore

| Aspect | Python version | Go version |
|---|---|---|
| Token expiry validation | Checks expiry before restoring | Checks expiry with >1 hour margin, supports multiple date formats from MathWorks API |
| Entitlement refresh | Unclear if re-fetched on restore | Explicitly re-fetches entitlements using cached identity token on every restore |
| Access token handling | Access token may be cached | Access token is never cached (it's short-lived); always fetched fresh at MATLAB start time |
| Failed restore | Behavior varies | Cache file deleted, user prompted to log in again |

### Packaging

| Aspect | Python version | Go version |
|---|---|---|
| Distribution | pip install (PyPI wheel) | Single binary |
| Runtime deps | Python 3.8+, Node.js (for GUI build) | None |
| Binary size | N/A (interpreted) | ~10 MB |
| Static assets | Served from filesystem | Embedded via `embed.FS` |

### Process management

| Aspect | Python version | Go version |
|---|---|---|
| Subprocess handling | `asyncio.create_subprocess_exec` | `os/exec.CommandContext` |
| PTY allocation | Not explicitly documented | Explicit PTY for MATLAB stdin (POSIX only — MATLAB requires a pseudo-terminal) |
| Windows support | `psutil` for process tree management | `os.Process.Kill` (basic) |
| Xvfb display allocation | `-displayfd` pipe-based | Socket file + TCP port probing |

## What Was Removed

These features from the Python version are **not included** in the Go version:

### matlab-proxy-manager
The multi-instance manager (`matlab_proxy_manager`) that runs shared or isolated MATLAB sessions for Jupyter kernels is not implemented. This was a deliberate scope decision — if you need multi-instance management, use the Python version.

### Development/test mode
The fake MATLAB server (`devel.py`) for testing without a real MATLAB installation is not included. The Go version requires a real MATLAB installation to function.

### Jupyter integration
The Python version registers entry points that allow Jupyter to discover and launch matlab-proxy automatically. The Go version does not integrate with Jupyter's plugin system. It can still be used with Jupyter by running it separately and configuring the URL manually.

### Custom configuration plugins
The Python version supports registering custom configurations via `matlab_proxy_configs` entry points. The Go version uses a fixed default configuration. Custom HTTP headers can still be set via `MWI_CUSTOM_HTTP_HEADERS`.

### Cookie jar caching
The experimental `MWI_USE_COOKIE_CACHE` feature for caching HTTP-only cookies is not implemented.

### Rich console logging
The experimental rich logging feature (formatted console output) is not included. The Go version uses `slog` (structured logging) with plain text or JSON output.

## What Was Added

These are new in the Go version:

### Single-binary deployment
All templates, CSS, JavaScript, and icons are embedded in the binary via Go's `embed.FS`. There are no external files to manage.

### Structured logging
Uses Go's `slog` package for structured, leveled logging. Log output includes timestamps, levels, and key-value pairs by default.

### Simplified frontend
The server-rendered approach eliminates the Node.js build step entirely. There are no `node_modules`, no `package.json`, no Vite config, and no npm scripts.

### Port-scoped session cookies
Authentication cookies include the server port in their name (`mwi-auth-session-<port>`), preventing cookie collisions when multiple instances run on the same host. A middleware automatically expires stale cookies from previous server sessions that reused the same port.

### Automatic crash recovery for concurrent sessions
The timestamp-based concurrency model automatically recovers from client crashes within 10 seconds, without requiring the crashed client to call a cleanup endpoint. The Python version requires an explicit session transfer in this scenario.

### Shutdown artifact cleanup
The server automatically removes `connector.securePort` and extracted MATLAB scripts on shutdown, preventing stale artifacts from affecting subsequent launches. MATLAB log files are preserved for debugging.

### Built-in web terminal
An interactive system shell is available directly in the browser via a bottom-drawer panel. The terminal uses xterm.js with a WebSocket-to-PTY bridge, supporting resize, minimize (shell keeps running), and keyboard shortcut (``Ctrl+` ``). The Python version does not include a terminal. See [Web Terminal](terminal.md) for details.

---

Copyright 2026 The MathWorks, Inc.
