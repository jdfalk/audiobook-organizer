# iTunes MCP Server

An MCP (Model Context Protocol) server that exposes the iTunes COM API on the
Windows machine (`unimatrixzero.local`) for remote control from Claude Code.

## Overview

The server runs as a PowerShell script, wrapped by `mtls-bridge serve` which
provides mTLS encryption over TCP. Claude Code connects via `mtls-bridge connect`
which bridges stdio to the mTLS connection.

## Architecture

```
Claude Code <-stdio-> mtls-bridge connect <-mTLS/TCP-> mtls-bridge serve <-stdio-> PowerShell <-COM-> iTunes
```

## Setup

### Prerequisites

- **Windows machine** with iTunes for Windows installed
- **PowerShell 5.1+** (built into Windows)
- **Shared filesystem** between Mac and Windows (the repo directory)

### 1. Build the bridge binary

On Mac:
```bash
make build-mtls-bridge              # macOS binary
make build-mtls-bridge-windows      # Windows binary (cross-compiled)
```

Copy `mtls-bridge.exe` to the Windows machine (or it's already there via shared filesystem).

### 2. Generate PSK and provision certificates

On either machine (shared filesystem means both see it):
```bash
./mtls-bridge provision --generate-psk
```

Start the server on Windows:
```powershell
.\mtls-bridge.exe serve --powershell "W:\audiobook-organizer\scripts\itunes-mcp-server.ps1"
```

On Mac, run connect (or let Claude Code trigger it via `.mcp.json`):
```bash
./mtls-bridge connect
```

The first connection exchanges the PSK for mTLS certificates. Subsequent connections use mTLS directly.

### 3. Normal usage

Start server on Windows:
```powershell
.\mtls-bridge.exe serve --powershell "W:\audiobook-organizer\scripts\itunes-mcp-server.ps1"
```

Claude Code automatically connects via `.mcp.json` configuration.

### Certificate Management

```bash
# Renew certs (when expiry warning appears)
./mtls-bridge provision --renew

# Full reset (re-provision from scratch)
./mtls-bridge provision --reset --generate-psk
```

## Available Tools

| Tool | Description |
|------|-------------|
| `itunes_open_library(path)` | Set registry and launch iTunes with specified library folder |
| `itunes_close()` | Quit iTunes via COM and release resources |
| `itunes_get_track_count()` | Return total track count |
| `itunes_get_tracks(offset, limit)` | Paginated track list |
| `itunes_verify_files(limit)` | Check if track file locations exist on disk |
| `itunes_get_library_info()` | Library path, iTunes version, track/playlist counts |
| `itunes_search(query, limit)` | Search tracks by name |
| `itunes_run_test(test_folder)` | Run a single test case, return results |

## Troubleshooting

**"no PSK or certs found"**: Run `mtls-bridge provision --generate-psk` first.

**"server unreachable"**: Ensure `mtls-bridge serve` is running on Windows.
Check that `.mtls/server.json` exists and has the correct host/port.

**"TLS handshake failure"**: Certs may be mismatched. Run `mtls-bridge provision --reset --generate-psk` and re-provision.

**"cert expires in N days"**: Run `mtls-bridge provision --renew` on either machine, then restart the server.

**"iTunes COM object failed"**: iTunes may not be installed, or another instance
is running. The server will try to kill existing instances before launching.
