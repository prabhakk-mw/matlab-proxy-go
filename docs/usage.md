# Usage

## Running the Server

```bash
# Simplest invocation (MATLAB must be on PATH)
./matlab-proxy

# With explicit MATLAB path
MWI_CUSTOM_MATLAB_ROOT=/opt/matlab/R2024a ./matlab-proxy

# With a fixed port
MWI_APP_PORT=8080 ./matlab-proxy

# With authentication disabled (not recommended for shared machines)
MWI_ENABLE_TOKEN_AUTH=false ./matlab-proxy
```

On startup, the server prints:

```
Access MATLAB at: http://localhost:8080/?mwi-auth-token=<token>
```

Open this URL in a browser to access MATLAB.

## Attach Mode

Attach mode connects matlab-proxy to an **already-running MATLAB session** instead of spawning a new one. This is useful when you have MATLAB open and want to access it through the browser without restarting it.

### Setup

**Step 1:** Run the setup script in your MATLAB command window:

```matlab
run('/path/to/matlab-proxy-go/scripts/enable_connect.m')
```

The script:
1. Creates a temporary log directory for the Embedded Connector port file
2. Generates a unique API key (`MWAPIKEY`)
3. Configures the Embedded Connector environment (`MW_DOCROOT`, `MW_CONNECTOR_CONTEXT_ROOT`, etc.)
4. Starts the Embedded Connector inside your MATLAB session
5. Prints the `--ec-port` and `--mwapikey` values you need

**Step 2:** Start the proxy with the printed values:

```bash
# Using CLI flags
matlab-proxy --ec-port 31516 --mwapikey d091bb44-e7e1-2082-e9d3-a1e24ba9dc81

# Or using environment variables
MWI_ATTACH_EC_PORT=31516 MWI_ATTACH_MWAPIKEY=d091bb44-e7e1-2082-e9d3-a1e24ba9dc81 matlab-proxy
```

CLI flags take precedence over environment variables. Both `--ec-port` and `--mwapikey` (or their env var equivalents) must be provided together.

### Behavior Differences in Attach Mode

| Behavior | Normal Mode | Attach Mode |
|---|---|---|
| MATLAB lifecycle | Proxy spawns and owns the MATLAB process | Proxy connects to an existing MATLAB; does not own it |
| Licensing | Configured via UI or env vars | Automatically set to "Existing License" (MATLAB is already licensed) |
| Stop / Disconnect | Sends `exit` to MATLAB, kills the process | Disconnects the proxy; MATLAB and the EC keep running |
| Reconnect | Restarts MATLAB from scratch | Re-attaches to the same EC (same port and key) |
| Server shutdown | Stops MATLAB, cleans up session files | Disconnects from EC; MATLAB keeps running; no file cleanup |
| EC health monitoring | Monitors child process exit | Detects EC unreachable after 5 consecutive ping failures → status transitions to `down` |
| UI controls | Start, Stop, Restart, Change License, Sign Out | Disconnect, Reconnect |

### Known Limitations

> **Important:** Once the Embedded Connector's web desktop is activated in a MATLAB session, the original MATLAB desktop command window becomes unresponsive. This is a MATLAB limitation — the web desktop takes exclusive control of the command evaluator. The original command window will **not** recover, even after disconnecting from matlab-proxy or stopping the Embedded Connector. To restore the original MATLAB desktop, you must restart MATLAB.

- Attach mode requires the user to manually run a setup script in MATLAB before connecting.
- The `--ec-port` and `--mwapikey` values are specific to the MATLAB session. If MATLAB is restarted, `enable_connect.m` must be run again.
- MATLAB version detection is not available in attach mode (the proxy does not know the MATLAB installation path). The version will show as empty in the UI.

## Listing Running Servers

```bash
# Table format (default)
./matlab-proxy-list-servers

# URLs only (for scripting)
./matlab-proxy-list-servers --quiet

# JSON output
./matlab-proxy-list-servers --json
```

## Environment Variables

All configuration is done via environment variables. No config files are required.

### Server Settings

| Variable | Default | Description |
|---|---|---|
| `MWI_APP_PORT` | *(random)* | Port to listen on |
| `MWI_APP_HOST` | `0.0.0.0` | Host interface to bind to |
| `MWI_BASE_URL` | `/` | Base URL path (useful when behind a reverse proxy) |

### MATLAB Settings

| Variable | Default | Description |
|---|---|---|
| `MWI_CUSTOM_MATLAB_ROOT` | *(auto-detect)* | Path to MATLAB installation root |
| `MWI_MATLAB_STARTUP_SCRIPT` | *(none)* | MATLAB code to execute on startup |
| `MWI_PROCESS_START_TIMEOUT` | `600` | Seconds to wait for MATLAB to start |
| `MWI_SESSION_NAME` | `MATLAB <version>` | Browser tab title / session name |

### Authentication

| Variable | Default | Description |
|---|---|---|
| `MWI_ENABLE_TOKEN_AUTH` | `true` | Enable token-based authentication |
| `MWI_AUTH_TOKEN` | *(auto-generated)* | Custom auth token (if not set, one is generated) |

### SSL/TLS

| Variable | Default | Description |
|---|---|---|
| `MWI_ENABLE_SSL` | `false` | Enable HTTPS |
| `MWI_SSL_CERT_FILE` | *(none)* | Path to SSL certificate file |
| `MWI_SSL_KEY_FILE` | *(none)* | Path to SSL private key file |

If SSL is enabled but no cert/key files are provided, a self-signed certificate is generated automatically (valid for 365 days).

### Licensing

| Variable | Default | Description |
|---|---|---|
| `MLM_LICENSE_FILE` | *(none)* | NLM connection string (`port@hostname`) |
| `MWI_USE_EXISTING_LICENSE` | `false` | Use an already-activated MATLAB license |
| `MWI_LICMODE_OVERRIDE` | *(none)* | Override MATLAB `-licmode` flag |

These variables pre-configure licensing at startup. If none are set, the user is prompted to configure licensing via the web UI.

### Logging

| Variable | Default | Description |
|---|---|---|
| `MWI_LOG_LEVEL` | `INFO` | Log level: `DEBUG`, `INFO`, `WARNING`, `ERROR` |
| `MWI_LOG_FILE` | *(stderr)* | Path to log file |
| `MWI_ENABLE_WEB_LOGGING` | `false` | Enable HTTP access logging |

### Timeouts and Lifecycle

| Variable | Default | Description |
|---|---|---|
| `MWI_SHUTDOWN_ON_IDLE_TIMEOUT` | *(disabled)* | Minutes of inactivity before auto-shutdown |

### Attach Mode

| Variable | Default | Description |
|---|---|---|
| `MWI_ATTACH_EC_PORT` | *(none)* | Embedded Connector port (CLI: `--ec-port`) |
| `MWI_ATTACH_MWAPIKEY` | *(none)* | API key for EC authentication (CLI: `--mwapikey`) |

Both must be provided together. See [Attach Mode](#attach-mode) above for the full workflow.

### Advanced / Experimental

| Variable | Default | Description |
|---|---|---|
| `MWI_ENABLE_SIMULINK` | `false` | Enable Simulink Online (starts Fluxbox window manager) |
| `MWI_PROFILE_MATLAB_STARTUP` | `false` | Pass `-timing` flag to MATLAB for startup profiling |
| `MWI_CUSTOM_HTTP_HEADERS` | `{}` | JSON string of custom HTTP headers added to all responses |

## Authentication

When token authentication is enabled (the default), access to the server requires a valid token.

### Token sources

The token can be provided in three ways (checked in this order):

1. **Session cookie:** `mwi-auth-session-<port>` — set automatically after successful authentication
2. **URL query parameter:** `?mwi-auth-token=<token>` — used in the access URL printed at startup
3. **HTTP header:** `mwi-auth-token: <token>`

Both the raw token and its SHA-256 hex digest are accepted in all cases.

### Authentication flow

1. **URL with token** — When you open the access URL printed at startup (which includes `?mwi-auth-token=<token>`), the server validates the token, sets a session cookie, and renders the page. Subsequent requests (including iframe subrequests for MATLAB) use the cookie automatically.

2. **URL without token** — When you access the server without a token in the URL, the frontend calls `POST /authenticate` to check if a valid session cookie already exists. If a cookie is present and valid, the app loads normally. If not, a token input screen is shown.

3. **Token input screen** — The user pastes the auth token and submits. The token is sent to `POST /authenticate` via the `mwi-auth-token` header. On success, a session cookie is set and the app loads.

### Session cookie behavior

- Cookies are scoped by port: `mwi-auth-session-<port>`. This prevents cookie collisions when running multiple instances on the same host.
- A `ClearStaleCookieMiddleware` runs on every request. If a cookie exists but contains a token from a previous server session (different token, same port), the cookie is expired immediately. This handles the case where a new server starts on a port that was previously used by another instance.
- Cookies are `HttpOnly` and use `SameSite=Lax`.

## Licensing Flows

### Existing License
Set `MWI_USE_EXISTING_LICENSE=true` or select "Existing License" in the web UI. No additional configuration needed — MATLAB uses whatever license is already activated on the machine.

### Network License Manager (NLM)
Set `MLM_LICENSE_FILE=27000@license-server` or enter the connection string in the web UI under "Network License". The connection string is passed to MATLAB via the `MLM_LICENSE_FILE` environment variable.

### MathWorks Online License (MHLM)
Select "Online License" in the web UI. The browser presents a MathWorks login iframe. After authentication, the server exchanges the identity token for entitlements via MathWorks APIs. If the user has multiple licenses, they select one from a dropdown. The selected entitlement is passed to MATLAB via `MLM_WEB_LICENSE`, `MLM_WEB_USER_CRED`, and `MLM_WEB_ID` environment variables.

## Running Behind a Reverse Proxy

Set `MWI_BASE_URL` to the path prefix used by your reverse proxy:

```bash
MWI_BASE_URL=/matlab MWI_APP_PORT=8080 ./matlab-proxy
```

Example nginx configuration:

```nginx
location /matlab/ {
    proxy_pass http://127.0.0.1:8080/matlab/;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_read_timeout 86400s;
}
```

The `proxy_read_timeout` should be set high because MATLAB WebSocket connections are long-lived.

## Signals

The server responds to:
- **SIGINT** (Ctrl+C) — Graceful shutdown: stops MATLAB, then stops the HTTP server
- **SIGTERM** — Same as SIGINT

## Concurrent Browser Sessions

MATLAB's Embedded Connector does not support being viewed from multiple browser tabs simultaneously. The server enforces single-active-client access:

- Each browser tab generates a unique client ID and sends it with every status poll.
- The first tab to poll claims ownership of the MATLAB session.
- If the active tab stops polling for more than 10 seconds (closed, crashed, or lost network), it is automatically expired and the next tab takes over.
- A second tab sees a "MATLAB is Open Elsewhere" dialog with an "Open MATLAB Here" button to forcibly transfer the session.
- If the active tab's session is transferred away, it sees a "Session Moved" dialog with a "Reclaim Session" button.
- On tab close, `navigator.sendBeacon` sends an immediate release request so the next tab can take over without waiting for the 10-second timeout.

## Data Directory

The server stores data in `~/.matlab/MWI/`:

```
~/.matlab/MWI/
├── proxy_app_config.json              Cached licensing configuration
└── hosts/<hostname>/ports/<port>/
    ├── mwi_server.info                Server discovery file
    ├── connector.securePort           MATLAB EC port (created by MATLAB)
    └── (MATLAB log files)
```

### Shutdown cleanup

On shutdown (Ctrl+C, SIGTERM, or "Shutdown" button), the server removes:
- `connector.securePort` — stale file would confuse the next launch on the same port
- `matlab_scripts/` — extracted startup scripts
- `mwi_server.info` — server discovery file

MATLAB log files are intentionally preserved for debugging.

---

Copyright 2026 The MathWorks, Inc.
