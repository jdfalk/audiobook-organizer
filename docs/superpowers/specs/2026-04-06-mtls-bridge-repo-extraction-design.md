# mtls-bridge Standalone Repo Extraction

**Date:** 2026-04-06
**Status:** Draft

## Problem

The `mtls-bridge` binary and `internal/mtls/` package currently live inside the audiobook-organizer monorepo. The bridge is a general-purpose mTLS stdio proxy with no audiobook-specific code. Extracting it to its own repo enables independent versioning, CI, auto-update via GitHub Releases, and reuse by others.

## Solution

Create `github.com/jdfalk/mtls-bridge` as a standalone Go module. Add auto-update (self-update from GitHub Releases), automatic reconnect with exponential backoff, and CodeQL security scanning. Then remove the bridge code from audiobook-organizer.

## Repo Structure

```
github.com/jdfalk/mtls-bridge/
├── cmd/mtls-bridge/
│   └── main.go              # Cobra CLI (serve, connect, provision, update)
├── internal/mtls/
│   ├── certs.go              # CA + cert generation (ECDSA P-256)
│   ├── certs_test.go
│   ├── config.go             # .mtls/ directory, server.json, cert expiry
│   ├── config_test.go
│   ├── transport.go          # TLS 1.3 config factories
│   ├── transport_test.go
│   ├── bridge.go             # Bidirectional byte pipe with half-close
│   ├── bridge_test.go
│   ├── provisioning.go       # PSK exchange protocol
│   ├── provisioning_test.go
│   ├── updater.go            # Self-update from GitHub Releases
│   └── updater_test.go
├── .github/
│   ├── workflows/
│   │   ├── ci.yml            # Reusable CI from ghcommon
│   │   ├── release.yml       # GoReleaser on tag push
│   │   └── codeql.yml        # CodeQL security scanning
│   └── CODEOWNERS
├── .goreleaser.yml
├── go.mod                    # module github.com/jdfalk/mtls-bridge
├── go.sum
├── Makefile
├── CLAUDE.md
├── LICENSE                   # MIT
└── README.md
```

## Dependencies

Minimal — the bridge is pure stdlib plus:
- `github.com/spf13/cobra` — CLI framework
- `github.com/stretchr/testify` — test assertions

No CGO. Cross-compiles cleanly to darwin/linux/windows × amd64/arm64.

## Auto-Update

### Version Injection

GoReleaser injects version at build time via ldflags:
```
-X main.version={{.Version}} -X main.commit={{.Commit}} -X main.date={{.Date}}
```

### `mtls-bridge update` Subcommand

1. Calls GitHub Releases API: `GET /repos/jdfalk/mtls-bridge/releases/latest`
2. Compares `tag_name` against compiled-in `version`
3. If newer: downloads the correct platform asset (e.g., `mtls-bridge_Darwin_arm64.tar.gz`)
4. Verifies SHA256 against `checksums.txt` from the release
5. Replaces the running binary atomically (write to temp file, `os.Rename` over self)
6. Prints success message with old → new version

### `serve` Auto-Update on Startup

1. Check for update (5s timeout, non-blocking)
2. If newer version: download, replace, re-exec with same args (`syscall.Exec`)
3. If check fails (no network, rate limit): log warning, proceed with current version
4. Writes `{"last_check": "2026-04-06T...", "version": "v1.0.0"}` to `.mtls/update-check.json`
5. Skips check if last check was within 1 hour

### `connect` Notify Only

1. Check for update (5s timeout)
2. If newer: `[mtls-bridge] WARNING: update available (v1.0.0 → v1.1.0), run 'mtls-bridge update'`
3. Proceed with connection regardless — Claude Code controls the connect process lifecycle

## Reconnect

### `connect` Command — Exponential Backoff

On TCP connection drop:
1. Log `[mtls-bridge] connection lost, reconnecting...`
2. Re-read `server.json` (server may have restarted on a new port)
3. Retry with exponential backoff: 1s, 2s, 4s, 8s, 16s, 30s (cap)
4. On successful reconnect: reset backoff counter, log `[mtls-bridge] reconnected`
5. On stdin EOF (Claude Code closed its end): exit cleanly, no reconnect

The reconnect loop wraps the existing `BridgeStdio` call. On reconnect, stdin (os.Stdin from Claude Code) remains open — only the TCP connection to the server is re-established. MCP is stateless per-request, so the client just needs the TCP pipe alive for the next request. Any in-flight request at disconnect time will get a JSON-RPC error; Claude Code will retry.

### `serve` Command — No Changes Needed

Already loops on `listener.Accept()`. PowerShell respawn on crash is already implemented (new subprocess per connection).

## CI/CD

### ci.yml

Uses `jdfalk/ghcommon/.github/workflows/reusable-ci.yml@v1.10.4`:
```yaml
name: CI
on:
  push:
    branches: [main]
  pull_request:
jobs:
  ci:
    uses: jdfalk/ghcommon/.github/workflows/reusable-ci.yml@v1.10.4
    with:
      go-version: "1.26"
      coverage-threshold: 80
      frontend-enabled: false
      cgo-enabled: false
```

### codeql.yml

Standard GitHub CodeQL for Go:
```yaml
name: CodeQL
on:
  push:
    branches: [main]
  pull_request:
  schedule:
    - cron: "0 6 * * 1"  # Weekly Monday 6am UTC
jobs:
  analyze:
    runs-on: ubuntu-latest
    permissions:
      security-events: write
    steps:
      - uses: actions/checkout@v4
      - uses: github/codeql-action/init@v3
        with:
          languages: go
      - uses: github/codeql-action/autobuild@v3
      - uses: github/codeql-action/analyze@v3
```

### release.yml

GoReleaser triggered on version tags:
```yaml
name: Release
on:
  push:
    tags: ["v*"]
jobs:
  release:
    uses: jdfalk/ghcommon/.github/workflows/reusable-release.yml@v1.10.4
    with:
      go-enabled: true
      frontend-enabled: false
      docker-enabled: false
```

## GoReleaser Config

```yaml
version: 2
builds:
  - binary: mtls-bridge
    env: [CGO_ENABLED=0]
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
    ldflags:
      - -s -w
      - -X main.version={{.Version}}
      - -X main.commit={{.Commit}}
      - -X main.date={{.Date}}
    main: ./cmd/mtls-bridge
archives:
  - format: tar.gz
    format_overrides:
      - goos: windows
        format: zip
checksum:
  name_template: checksums.txt
  algorithm: sha256
changelog:
  sort: asc
  groups:
    - title: Features
      regexp: "^.*feat.*$"
    - title: Bug Fixes
      regexp: "^.*fix.*$"
```

## Cleanup in audiobook-organizer

After the new repo is created and published:

1. Delete `internal/mtls/` directory (8 files)
2. Delete `cmd/mtls-bridge/` directory (1 file)
3. Remove `build-mtls-bridge` and `build-mtls-bridge-windows` targets from Makefile
4. Remove `mtls-bridge` and `mtls-bridge.exe` from `.gitignore`
5. Update `.mcp.json` to reference `mtls-bridge` on PATH:
   ```json
   {
     "mcpServers": {
       "itunes": {
         "command": "mtls-bridge",
         "args": ["connect"]
       }
     }
   }
   ```
6. Add note to CLAUDE.md about external mtls-bridge dependency

## Non-Goals

- No Homebrew tap (GitHub Releases + self-update is sufficient)
- No Docker image (this is a CLI tool for local machines)
- No Windows service installer (manual start per design)
- No backward compatibility with the monorepo version (clean break)
