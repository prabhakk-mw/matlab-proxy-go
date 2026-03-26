# Web Terminal

matlab-proxy-go includes a built-in web terminal that provides shell access directly from the browser. This allows you to run system commands, inspect logs, manage files, and interact with the operating system without leaving the MATLAB proxy UI.

## Opening the Terminal

There are three ways to open the terminal:

1. **Keyboard shortcut** — Press ``Ctrl+` `` (Ctrl + backtick)
2. **Toggle button** — Click the `>_` button near the bottom-right of the screen (next to the MATLAB status trigger)
3. **Programmatically** — The terminal connects to the WebSocket endpoint at `/terminal/ws`

## Terminal States

The terminal has three states:

| State | Description |
|---|---|
| **Closed** | Terminal is not visible. No shell process is running. |
| **Open** | Split-screen view — MATLAB iframe on top, terminal on the bottom. The divider between them is draggable. |
| **Minimized** | A thin bar at the bottom of the screen. The shell process continues running in the background — output accumulates and is visible when you reopen. |

- Click the **minimize button** (`—`) or press ``Ctrl+` `` to minimize an open terminal.
- Click the **close button** (`×`) to terminate the shell session and hide the terminal.
- Press ``Ctrl+` `` or click the `>_` toggle to restore a minimized terminal.

## Resizing

Drag the divider handle between the MATLAB iframe and the terminal to adjust the split. The terminal height is saved to `localStorage` and restored across page reloads.

The terminal automatically adjusts its column and row count when resized (via xterm.js `fit` addon).

## Shell Selection

The terminal starts the user's default shell, determined in this order:

1. The `$SHELL` environment variable
2. `bash` (if found on `$PATH`)
3. `sh` (fallback)

The shell runs as the same OS user that started the matlab-proxy server.

## Authentication

The terminal WebSocket endpoint (`/terminal/ws`) is protected by the same token authentication as other API endpoints. When auth is enabled, the terminal passes the auth token as a query parameter on the WebSocket connection.

## Platform Support

| Platform | Status |
|---|---|
| Linux | Fully supported |
| macOS | Fully supported |
| Windows | Not supported (PTY allocation is not implemented) |

## Security Considerations

The web terminal provides full shell access as the server's OS user. Keep these points in mind:

- **Always enable token authentication** (`MWI_ENABLE_TOKEN_AUTH=true`, the default) when running on shared machines.
- The terminal is only accessible to authenticated users — the same authentication that protects MATLAB access also protects the terminal.
- The shell session is tied to the WebSocket connection. Closing the browser tab or clicking the close button terminates the shell process.

## Technical Details

- **Frontend:** [xterm.js](https://github.com/xtermjs/xterm.js) v5.5.0 with the fit addon, embedded in the binary.
- **Backend:** A WebSocket handler spawns a shell process attached to a PTY (pseudo-terminal). Terminal I/O is bridged over the WebSocket as text messages. Resize commands are sent as binary messages containing JSON (`{"cols": N, "rows": N}`).
- **PTY allocation:** Uses `/dev/ptmx` with platform-specific ioctls (Linux: `TIOCSPTLCK`/`TIOCGPTN`, macOS: `TIOCPTYGRANT`/`TIOCPTYUNLK`/`TIOCPTYGNAME`).

---

Copyright 2026 The MathWorks, Inc.
