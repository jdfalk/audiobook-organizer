# mTLS Bridge for iTunes MCP Server

**Date:** 2026-04-05
**Status:** Draft

## Problem

The iTunes MCP server (PowerShell on Windows) currently communicates with Claude Code (Mac) via SSH-piped stdio. This works but couples the MCP transport to SSH availability and configuration. The goal is to replace SSH with direct TCP using mTLS, bootstrapped from a pre-shared key (PSK).

## Solution

A single Go binary — `mtls-bridge` — that runs on both sides:

- **Windows:** `mtls-bridge serve` wraps the PowerShell MCP script, listens on mTLS
- **Mac:** `mtls-bridge connect` bridges Claude Code stdio to the mTLS connection
- **Either:** `mtls-bridge provision` handles PSK generation and cert exchange

The MCP protocol (Content-Length framed JSON-RPC 2.0) flows through unchanged. The PowerShell script requires no modifications.

## Architecture

```
Mac (Claude Code)                              Windows (unimatrixzero.local)
┌─────────────┐    mTLS TCP     ┌──────────────────────────────────────┐
│ Claude Code  │◄──stdin/out──►│ mtls-bridge connect │◄──mTLS──►│ mtls-bridge serve │◄──stdin/out──►│ PowerShell MCP │◄──COM──►│ iTunes │
└─────────────┘                └──────────────────────────────────────┘
```

Simplified:
```
Claude Code ←stdio→ mtls-bridge connect ←mTLS/TCP→ mtls-bridge serve ←stdio→ PowerShell ←COM→ iTunes
```

## Certificate & Config Directory

Both machines share a repo directory (shared filesystem at `W:\audiobook-organizer`). Certs live in `.mtls/` (gitignored):

```
.mtls/
  psk.txt           # PSK for provisioning (deleted after use)
  ca.crt            # CA certificate (both sides need this)
  ca.key            # CA private key (used by server for cert generation)
  server.crt        # server certificate
  server.key        # server private key
  client.crt        # client certificate
  client.key        # client private key
  server.json       # {"host": "unimatrixzero.local", "port": <random>}
```

Since `.mtls/` is on a shared filesystem, all files are visible to both machines. This is acceptable — both machines are owned by the same user, and the mTLS trust boundary is between this user's machines and the rest of the network, not between the two machines. The shared filesystem also means `server.json` and `psk.txt` require no manual copying.

## Provisioning Flow

### Phase 1: PSK Generation (one-time)

```
mtls-bridge provision --generate-psk
```

Generates a random 32-byte base64 PSK, writes to `.mtls/psk.txt`. Since `.mtls/` is on a shared filesystem, both machines see it immediately — no manual copy needed.

### Phase 2: Cert Exchange (first connection)

1. Server starts: `mtls-bridge serve --powershell "W:\...\itunes-mcp-server.ps1"`
2. Server sees `psk.txt` exists but no `ca.crt` → enters **provisioning mode**
3. Server listens on a random port with plain TLS (ephemeral self-signed cert — encryption only, so the PSK isn't sent in cleartext)
4. Server writes `server.json` with host + port
5. Client runs `mtls-bridge connect`, reads `psk.txt`, connects with TLS (skips server cert verification for this one connection)
6. Client sends PSK
7. Server validates PSK matches its own copy
8. Server generates:
   - CA keypair + self-signed CA cert (10 year expiry)
   - Server cert signed by CA (SAN: `unimatrixzero.local`, 1 year expiry)
   - Client cert signed by CA (1 year expiry)
9. Server sends `ca.crt`, `client.crt`, `client.key` to client
10. Server saves `ca.crt`, `ca.key`, `server.crt`, `server.key` locally
11. Both sides delete `psk.txt`
12. Server restarts listener in full mTLS mode

### Phase 3: Normal Operation (every subsequent start)

1. Server starts, finds `ca.crt` + `server.crt` + `server.key` → mTLS mode
2. Picks random port, writes `server.json`
3. Launches PowerShell subprocess
4. Listens for mTLS connections, verifies client cert is signed by CA
5. Bridges TCP ↔ PowerShell stdin/stdout

## Wire Protocol

The mTLS TCP connection carries the exact MCP protocol unchanged — Content-Length framed JSON-RPC 2.0. Both `mtls-bridge serve` and `mtls-bridge connect` are transparent bidirectional byte pipes.

## Server Behavior

- Accepts one mTLS connection at a time (single-client)
- Rejects additional connections while one is active
- PowerShell is spawned on first connection, kept warm across reconnects
- If PowerShell crashes, server respawns it on the next request (returns JSON-RPC error for the triggering request)
- If TCP connection drops, PowerShell stays alive (reconnect preserves iTunes COM state)
- Random port per start, written to `server.json`

## Client Behavior

- Reads `server.json` for host:port
- Opens mTLS connection with client cert
- Bidirectional copy: local stdin→TCP, TCP→local stdout
- On TCP drop, attempts reconnect with exponential backoff
- If `server.json` is missing or stale, logs "server unreachable, is mtls-bridge serve running?"

## MCP Configuration

`.mcp.json` changes from SSH to the local bridge binary:

```json
{
  "mcpServers": {
    "itunes": {
      "command": "./mtls-bridge",
      "args": ["connect"]
    }
  }
}
```

## Subcommands

### `mtls-bridge serve`

```
mtls-bridge serve --powershell <path-to-ps1> [--mtls-dir .mtls] [--host 0.0.0.0]
```

- Starts TLS/mTLS listener on random port
- Writes `server.json`
- Spawns PowerShell subprocess, bridges connections to it
- Logs to stderr

### `mtls-bridge connect`

```
mtls-bridge connect [--mtls-dir .mtls]
```

- Reads `server.json` and certs from `--mtls-dir`
- Connects to server with mTLS
- Bridges stdin/stdout to TCP
- If in provisioning mode (has `psk.txt`, no certs), performs cert exchange first

### `mtls-bridge provision`

```
mtls-bridge provision --generate-psk [--mtls-dir .mtls]
mtls-bridge provision --renew [--mtls-dir .mtls]
mtls-bridge provision --reset [--mtls-dir .mtls]
```

- `--generate-psk`: Create new PSK
- `--renew`: Regenerate server + client certs from existing CA (no PSK needed, must run on server side where `ca.key` lives — or either side since shared filesystem)
- `--reset`: Delete all certs, optionally generate new PSK for reprovisioning

## Error Handling

- **Cert expiry:** Log warning when certs are within 30 days of expiry on startup.
- **Stale server.json:** Client gets TCP timeout → "server unreachable" error.
- **PowerShell crash:** Server respawns on next request, returns JSON-RPC error for the triggering request.
- **Cert mismatch:** TLS handshake fails → clear error message suggesting `--reset`.
- **Multiple clients:** Reject with TLS-level error (single-client design).

## Code Location

```
cmd/mtls-bridge/
  main.go           # CLI entry point (cobra subcommands)
  serve.go          # serve subcommand
  connect.go        # connect subcommand
  provision.go      # provision subcommand

internal/mtls/
  certs.go          # CA and cert generation (crypto/x509)
  config.go         # .mtls/ directory management, server.json read/write
  transport.go      # mTLS listener and dialer setup
  provisioning.go   # PSK exchange protocol
```

## Non-Goals

- No HTTP/SSE transport — stays with raw TCP streaming
- No mDNS or other network discovery — shared filesystem is sufficient
- No Windows service integration — manual start only
- No changes to the PowerShell MCP script
