# mtls-bridge Repo Extraction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract mtls-bridge into a standalone repo at `github.com/jdfalk/mtls-bridge` with auto-update, reconnect, CI, and CodeQL.

**Architecture:** Create new GitHub repo, copy existing code with module path change, add updater and reconnect logic, set up CI/release/CodeQL workflows, then clean up the audiobook-organizer repo.

**Tech Stack:** Go 1.26, cobra, testify, GitHub Actions, GoReleaser, CodeQL

**Spec:** `docs/superpowers/specs/2026-04-06-mtls-bridge-repo-extraction-design.md`

---

## File Structure (new repo)

```
github.com/jdfalk/mtls-bridge/
  cmd/mtls-bridge/main.go         # CLI with serve, connect, provision, update, version
  internal/mtls/certs.go           # Copied from audiobook-organizer
  internal/mtls/certs_test.go
  internal/mtls/config.go          # Copied, add update-check.json support
  internal/mtls/config_test.go
  internal/mtls/transport.go       # Copied
  internal/mtls/transport_test.go
  internal/mtls/bridge.go          # Copied
  internal/mtls/bridge_test.go
  internal/mtls/provisioning.go    # Copied
  internal/mtls/provisioning_test.go
  internal/mtls/updater.go         # NEW: self-update from GitHub Releases
  internal/mtls/updater_test.go    # NEW
  .github/workflows/ci.yml
  .github/workflows/release.yml
  .github/workflows/codeql.yml
  .github/CODEOWNERS
  .goreleaser.yml
  .gitignore
  go.mod
  Makefile
  CLAUDE.md
  LICENSE
  README.md
```

---

### Task 1: Create GitHub Repo and Scaffold

**Files:**
- Create: GitHub repo `jdfalk/mtls-bridge`
- Create: `go.mod`, `go.sum`
- Create: `LICENSE`, `README.md`, `CLAUDE.md`, `.gitignore`, `Makefile`

- [ ] **Step 1: Create the GitHub repo**

```bash
gh repo create jdfalk/mtls-bridge --public --description "mTLS stdio bridge — wrap any stdin/stdout process with mutual TLS" --license MIT --clone
```

- [ ] **Step 2: Initialize go.mod**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/mtls-bridge
go mod init github.com/jdfalk/mtls-bridge
go mod edit -go=1.26
```

- [ ] **Step 3: Create `.gitignore`**

```gitignore
# Binaries
mtls-bridge
mtls-bridge.exe
*.test

# Coverage
*.out
*.cover
coverage.html

# IDE
.idea/
.vscode/
*.swp

# mTLS certificates and config
.mtls/

# OS
.DS_Store
Thumbs.db
```

- [ ] **Step 4: Create `LICENSE`**

```
MIT License

Copyright (c) 2025 Johnathan Falk

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

- [ ] **Step 5: Create `CLAUDE.md`**

```markdown
# CLAUDE.md

**mtls-bridge** — wrap any stdin/stdout process with mutual TLS.

## Build & Test

```bash
make build          # Build for current platform
make build-all      # Cross-compile for all platforms
make test           # Run tests
make coverage       # Run tests with coverage
make lint           # Run go vet
```

## Architecture

- `cmd/mtls-bridge/main.go` — Cobra CLI (serve, connect, provision, update, version)
- `internal/mtls/` — Core library (certs, config, transport, bridge, provisioning, updater)

## Subcommands

- `serve --powershell <path>` — mTLS server wrapping a subprocess
- `connect` — mTLS client bridging stdio
- `provision --generate-psk | --renew | --reset` — Certificate management
- `update` — Self-update from GitHub Releases
- `version` — Print version info

## Critical Rules

1. **Git:** Conventional commits mandatory.
2. **Pure Go:** No CGO. Must cross-compile cleanly.
3. **TLS 1.3 minimum:** All TLS configs enforce `MinVersion: tls.VersionTLS13`.
```

- [ ] **Step 6: Create `Makefile`**

```makefile
BINARY := mtls-bridge
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: build build-all test coverage lint clean help

help:
	@echo "Targets:"
	@echo "  make build       - Build for current platform"
	@echo "  make build-all   - Cross-compile for all platforms"
	@echo "  make test        - Run tests"
	@echo "  make coverage    - Run tests with coverage"
	@echo "  make lint        - Run go vet"
	@echo "  make clean       - Remove build artifacts"

build:
	@go build -ldflags="$(LDFLAGS)" -o $(BINARY) ./cmd/mtls-bridge

build-all:
	@GOOS=darwin GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BINARY)-darwin-amd64 ./cmd/mtls-bridge
	@GOOS=darwin GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(BINARY)-darwin-arm64 ./cmd/mtls-bridge
	@GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BINARY)-linux-amd64 ./cmd/mtls-bridge
	@GOOS=linux GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(BINARY)-linux-arm64 ./cmd/mtls-bridge
	@GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BINARY)-windows-amd64.exe ./cmd/mtls-bridge

test:
	@go test ./... -v -count=1

coverage:
	@go test ./... -coverprofile=coverage.out -covermode=atomic
	@go tool cover -func=coverage.out

lint:
	@go vet ./...

clean:
	@rm -f $(BINARY) $(BINARY)-* coverage.out
```

- [ ] **Step 7: Create `README.md`**

```markdown
# mtls-bridge

Wrap any stdin/stdout process with mutual TLS. Designed for bridging [MCP](https://modelcontextprotocol.io/) servers across machines, but works with any stdio-based protocol.

## Install

Download the latest release from [GitHub Releases](https://github.com/jdfalk/mtls-bridge/releases), or build from source:

```bash
go install github.com/jdfalk/mtls-bridge/cmd/mtls-bridge@latest
```

## Quick Start

### 1. Generate a pre-shared key

```bash
mtls-bridge provision --generate-psk
```

Copy `.mtls/psk.txt` to the other machine (or use a shared filesystem).

### 2. Start the server

```bash
mtls-bridge serve --powershell "/path/to/your-script.ps1"
```

### 3. Connect from the client

```bash
mtls-bridge connect
```

The first connection exchanges the PSK for mTLS certificates. All subsequent connections use mutual TLS.

## How It Works

```
Client (stdio) ←→ mtls-bridge connect ←mTLS/TCP→ mtls-bridge serve ←→ Subprocess (stdio)
```

- **Provisioning:** A pre-shared key (PSK) bootstraps certificate generation. The server generates a CA + server cert + client cert, sends the client its credentials over a TLS-encrypted channel.
- **Normal operation:** Both sides present certificates signed by the shared CA. TLS 1.3 minimum.
- **Auto-update:** The `serve` command auto-updates on startup. The `connect` command notifies when updates are available.

## Commands

| Command | Description |
|---------|-------------|
| `serve --powershell <path>` | Start mTLS server wrapping a subprocess |
| `connect` | Connect to server, bridge to local stdio |
| `provision --generate-psk` | Generate a new pre-shared key |
| `provision --renew` | Regenerate certs from existing CA |
| `provision --reset` | Delete all certs and config |
| `update` | Self-update to latest release |
| `version` | Print version, commit, and build date |

## License

MIT
```

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "feat: initial repo scaffold (go.mod, Makefile, docs)"
git push -u origin main
```

---

### Task 2: Copy Core Library

**Files:**
- Create: `internal/mtls/certs.go`, `certs_test.go`
- Create: `internal/mtls/config.go`, `config_test.go`
- Create: `internal/mtls/transport.go`, `transport_test.go`
- Create: `internal/mtls/bridge.go`, `bridge_test.go`
- Create: `internal/mtls/provisioning.go`, `provisioning_test.go`

- [ ] **Step 1: Copy all internal/mtls files from audiobook-organizer**

```bash
mkdir -p internal/mtls
cp /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/mtls/*.go internal/mtls/
```

- [ ] **Step 2: No import path changes needed**

The `internal/mtls` package uses only stdlib imports — no references to `github.com/jdfalk/audiobook-organizer`. Verify:

```bash
grep -r "audiobook-organizer" internal/mtls/
```

Expected: No output (no references).

- [ ] **Step 3: Add dependencies**

```bash
go get github.com/spf13/cobra@latest
go get github.com/stretchr/testify@latest
go mod tidy
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/mtls/ -v -count=1
```

Expected: All 18 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat: copy internal/mtls package from audiobook-organizer"
```

---

### Task 3: Copy and Update CLI

**Files:**
- Create: `cmd/mtls-bridge/main.go`

- [ ] **Step 1: Copy main.go from audiobook-organizer**

```bash
mkdir -p cmd/mtls-bridge
cp /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/cmd/mtls-bridge/main.go cmd/mtls-bridge/main.go
```

- [ ] **Step 2: Fix the import path**

In `cmd/mtls-bridge/main.go`, change:

```go
mtls "github.com/jdfalk/audiobook-organizer/internal/mtls"
```

to:

```go
mtls "github.com/jdfalk/mtls-bridge/internal/mtls"
```

- [ ] **Step 3: Add version variables and version command**

At the top of `main.go`, after the package declaration, add:

```go
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)
```

Add a version command in `init()`:

```go
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("mtls-bridge %s (commit: %s, built: %s)\n", version, commit, date)
	},
}
```

And in `init()`, add: `rootCmd.AddCommand(serveCmd, connectCmd, provisionCmd, versionCmd)`

- [ ] **Step 4: Verify build**

```bash
go build -ldflags="-X main.version=test" -o mtls-bridge ./cmd/mtls-bridge
./mtls-bridge version
```

Expected: `mtls-bridge test (commit: unknown, built: unknown)`

- [ ] **Step 5: Run full test suite**

```bash
go test ./... -v -count=1
```

Expected: All tests PASS.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat: add CLI with serve, connect, provision, version commands"
```

---

### Task 4: Self-Update (`internal/mtls/updater.go`)

**Files:**
- Create: `internal/mtls/updater.go`
- Create: `internal/mtls/updater_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/mtls/updater_test.go`:

```go
// file: internal/mtls/updater_test.go
// version: 1.0.0

package mtls

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseGitHubRelease(t *testing.T) {
	body := `{
		"tag_name": "v1.2.3",
		"assets": [
			{"name": "mtls-bridge_1.2.3_Darwin_arm64.tar.gz", "browser_download_url": "https://example.com/darwin-arm64.tar.gz"},
			{"name": "checksums.txt", "browser_download_url": "https://example.com/checksums.txt"}
		]
	}`

	release, err := parseGitHubRelease([]byte(body))
	require.NoError(t, err)
	assert.Equal(t, "v1.2.3", release.TagName)
	assert.Len(t, release.Assets, 2)
}

func TestNeedsUpdate(t *testing.T) {
	assert.True(t, needsUpdate("v1.0.0", "v1.1.0"))
	assert.True(t, needsUpdate("v1.0.0", "v2.0.0"))
	assert.False(t, needsUpdate("v1.1.0", "v1.1.0"))
	assert.False(t, needsUpdate("v1.2.0", "v1.1.0"))
	assert.False(t, needsUpdate("dev", "v1.0.0"))  // dev builds never auto-update
}

func TestCheckForUpdate_NewVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"tag_name": "v2.0.0",
			"assets":   []interface{}{},
		})
	}))
	defer server.Close()

	result, err := CheckForUpdate("v1.0.0", server.URL)
	require.NoError(t, err)
	assert.True(t, result.Available)
	assert.Equal(t, "v2.0.0", result.LatestVersion)
}

func TestCheckForUpdate_AlreadyCurrent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"tag_name": "v1.0.0",
			"assets":   []interface{}{},
		})
	}))
	defer server.Close()

	result, err := CheckForUpdate("v1.0.0", server.URL)
	require.NoError(t, err)
	assert.False(t, result.Available)
}

func TestUpdateCheck_Throttle(t *testing.T) {
	dir := t.TempDir()
	d := NewDir(dir)

	// Write a recent check
	check := UpdateCheckInfo{
		LastCheck: time.Now(),
		Version:   "v1.0.0",
	}
	data, _ := json.Marshal(check)
	os.WriteFile(filepath.Join(dir, "update-check.json"), data, 0644)

	assert.True(t, d.ShouldSkipUpdateCheck(1*time.Hour))

	// Write an old check
	check.LastCheck = time.Now().Add(-2 * time.Hour)
	data, _ = json.Marshal(check)
	os.WriteFile(filepath.Join(dir, "update-check.json"), data, 0644)

	assert.False(t, d.ShouldSkipUpdateCheck(1*time.Hour))
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/mtls/ -v -run "TestParse|TestNeeds|TestCheckFor|TestUpdateCheck"
```

Expected: FAIL — types not defined.

- [ ] **Step 3: Implement updater**

Create `internal/mtls/updater.go`:

```go
// file: internal/mtls/updater.go
// version: 1.0.0

package mtls

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

const defaultReleaseURL = "https://api.github.com/repos/jdfalk/mtls-bridge/releases/latest"

// UpdateCheckInfo is persisted to update-check.json.
type UpdateCheckInfo struct {
	LastCheck time.Time `json:"last_check"`
	Version   string    `json:"version"`
}

// UpdateResult is returned by CheckForUpdate.
type UpdateResult struct {
	Available     bool
	LatestVersion string
	AssetURL      string
	ChecksumURL   string
}

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func parseGitHubRelease(data []byte) (*githubRelease, error) {
	var release githubRelease
	if err := json.Unmarshal(data, &release); err != nil {
		return nil, fmt.Errorf("parse release: %w", err)
	}
	return &release, nil
}

// needsUpdate compares current version against latest.
// Returns false for dev builds (never auto-update non-release builds).
func needsUpdate(current, latest string) bool {
	if current == "dev" || current == "" {
		return false
	}
	current = strings.TrimPrefix(current, "v")
	latest = strings.TrimPrefix(latest, "v")
	if current == latest {
		return false
	}
	// Simple semver comparison: split on "." and compare numerically
	cp := strings.Split(current, ".")
	lp := strings.Split(latest, ".")
	for i := 0; i < 3; i++ {
		var c, l int
		if i < len(cp) {
			fmt.Sscanf(cp[i], "%d", &c)
		}
		if i < len(lp) {
			fmt.Sscanf(lp[i], "%d", &l)
		}
		if l > c {
			return true
		}
		if c > l {
			return false
		}
	}
	return false
}

// CheckForUpdate queries the GitHub Releases API for a newer version.
// Pass "" for releaseURL to use the default.
func CheckForUpdate(currentVersion, releaseURL string) (*UpdateResult, error) {
	if releaseURL == "" {
		releaseURL = defaultReleaseURL
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(releaseURL)
	if err != nil {
		return nil, fmt.Errorf("fetch release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	release, err := parseGitHubRelease(body)
	if err != nil {
		return nil, err
	}

	result := &UpdateResult{
		Available:     needsUpdate(currentVersion, release.TagName),
		LatestVersion: release.TagName,
	}

	if result.Available {
		assetName := assetNameForPlatform()
		for _, asset := range release.Assets {
			if asset.Name == assetName {
				result.AssetURL = asset.BrowserDownloadURL
			}
			if asset.Name == "checksums.txt" {
				result.ChecksumURL = asset.BrowserDownloadURL
			}
		}
	}

	return result, nil
}

// assetNameForPlatform returns the expected asset filename for the current OS/arch.
func assetNameForPlatform() string {
	osName := strings.Title(runtime.GOOS)
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x86_64"
	}
	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("mtls-bridge_%s_%s.%s", osName, arch, ext)
}

// SelfUpdate downloads and replaces the current binary.
func SelfUpdate(assetURL string) error {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(assetURL)
	if err != nil {
		return fmt.Errorf("download asset: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download returned %d", resp.StatusCode)
	}

	// Write to temp file next to current binary
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	tmpFile := execPath + ".update"
	f, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmpFile)
		return fmt.Errorf("write update: %w", err)
	}
	f.Close()

	// Atomic rename
	if err := os.Rename(tmpFile, execPath); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("replace binary: %w", err)
	}

	return nil
}

// ShouldSkipUpdateCheck returns true if the last check was within the throttle duration.
func (d *Dir) ShouldSkipUpdateCheck(throttle time.Duration) bool {
	data, err := os.ReadFile(d.Path("update-check.json"))
	if err != nil {
		return false
	}
	var info UpdateCheckInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return false
	}
	return time.Since(info.LastCheck) < throttle
}

// WriteUpdateCheck records that an update check was performed.
func (d *Dir) WriteUpdateCheck(version string) error {
	info := UpdateCheckInfo{
		LastCheck: time.Now(),
		Version:   version,
	}
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	return os.WriteFile(d.Path("update-check.json"), data, 0644)
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/mtls/ -v -run "TestParse|TestNeeds|TestCheckFor|TestUpdateCheck"
```

Expected: All 5 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/mtls/updater.go internal/mtls/updater_test.go
git commit -m "feat: add self-update from GitHub Releases"
```

---

### Task 5: Wire Update into CLI

**Files:**
- Modify: `cmd/mtls-bridge/main.go`

- [ ] **Step 1: Add `update` subcommand**

In `init()`, add:

```go
var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update mtls-bridge to the latest release",
	RunE:  runUpdate,
}
```

And register it: `rootCmd.AddCommand(serveCmd, connectCmd, provisionCmd, versionCmd, updateCmd)`

Add the implementation:

```go
func runUpdate(cmd *cobra.Command, args []string) error {
	fmt.Fprintf(os.Stderr, "[mtls-bridge] Checking for updates (current: %s)...\n", version)

	result, err := mtls.CheckForUpdate(version, "")
	if err != nil {
		return fmt.Errorf("check for update: %w", err)
	}

	if !result.Available {
		fmt.Fprintf(os.Stderr, "[mtls-bridge] Already up to date (%s)\n", version)
		return nil
	}

	fmt.Fprintf(os.Stderr, "[mtls-bridge] Update available: %s → %s\n", version, result.LatestVersion)

	if result.AssetURL == "" {
		return fmt.Errorf("no binary available for this platform")
	}

	fmt.Fprintf(os.Stderr, "[mtls-bridge] Downloading %s...\n", result.LatestVersion)
	if err := mtls.SelfUpdate(result.AssetURL); err != nil {
		return fmt.Errorf("update failed: %w", err)
	}

	fmt.Fprintf(os.Stderr, "[mtls-bridge] Updated to %s. Restart to use the new version.\n", result.LatestVersion)
	return nil
}
```

- [ ] **Step 2: Add auto-update check to `runServe`**

At the beginning of `runServe`, before the state check, add:

```go
	dir := mtls.NewDir(mtlsDir)

	// Auto-update on serve startup
	if !dir.ShouldSkipUpdateCheck(1 * time.Hour) {
		result, err := mtls.CheckForUpdate(version, "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "[mtls-bridge] Update check failed: %v\n", err)
		} else if result.Available && result.AssetURL != "" {
			fmt.Fprintf(os.Stderr, "[mtls-bridge] Updating %s → %s...\n", version, result.LatestVersion)
			if err := mtls.SelfUpdate(result.AssetURL); err != nil {
				fmt.Fprintf(os.Stderr, "[mtls-bridge] Auto-update failed: %v (continuing with current version)\n", err)
			} else {
				dir.WriteUpdateCheck(result.LatestVersion)
				fmt.Fprintf(os.Stderr, "[mtls-bridge] Updated. Re-executing...\n")
				execPath, _ := os.Executable()
				syscall.Exec(execPath, os.Args, os.Environ())
			}
		}
		dir.WriteUpdateCheck(version)
	}
```

Add `"syscall"` to the imports.

- [ ] **Step 3: Add update notification to `runConnect`**

At the beginning of `runConnect`, after creating the Dir, add:

```go
	// Notify about updates (don't auto-update the client)
	if !dir.ShouldSkipUpdateCheck(1 * time.Hour) {
		result, err := mtls.CheckForUpdate(version, "")
		if err == nil && result.Available {
			fmt.Fprintf(os.Stderr, "[mtls-bridge] WARNING: update available (%s → %s), run 'mtls-bridge update'\n", version, result.LatestVersion)
		}
		dir.WriteUpdateCheck(version)
	}
```

- [ ] **Step 4: Verify build**

```bash
go build ./cmd/mtls-bridge
./mtls-bridge update
```

Expected: Either "Already up to date" or "no binary available" (no releases exist yet).

- [ ] **Step 5: Commit**

```bash
git add cmd/mtls-bridge/main.go
git commit -m "feat: wire update command and auto-update into serve/connect"
```

---

### Task 6: Reconnect with Exponential Backoff

**Files:**
- Modify: `cmd/mtls-bridge/main.go`

- [ ] **Step 1: Refactor `runConnect` to add reconnect loop**

Replace the connection and bridge section of `runConnect` (after cert loading) with a reconnect loop:

```go
	maxBackoff := 30 * time.Second
	backoff := 1 * time.Second

	for {
		info, err := dir.ReadServerInfo()
		if err != nil {
			fmt.Fprintf(os.Stderr, "[mtls-bridge] read server.json: %v\n", err)
			time.Sleep(backoff)
			backoff = nextBackoff(backoff, maxBackoff)
			continue
		}

		tlsCfg, err := mtls.ClientTLSConfig(caCert, clientCert, clientKey, info.Host)
		if err != nil {
			return fmt.Errorf("create TLS config: %w", err)
		}

		addr := fmt.Sprintf("%s:%d", info.Host, info.Port)
		fmt.Fprintf(os.Stderr, "[mtls-bridge] connecting to %s...\n", addr)

		conn, err := tls.DialWithDialer(
			&net.Dialer{Timeout: 10 * time.Second},
			"tcp",
			addr,
			tlsCfg,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[mtls-bridge] connect failed: %v, retrying in %v...\n", err, backoff)
			time.Sleep(backoff)
			backoff = nextBackoff(backoff, maxBackoff)
			continue
		}

		backoff = 1 * time.Second // Reset on successful connect
		fmt.Fprintf(os.Stderr, "[mtls-bridge] connected, bridging stdio\n")

		err = mtls.BridgeStdio(conn, os.Stdin, os.Stdout)
		if err == nil {
			// Clean exit (stdin closed by Claude Code)
			return nil
		}

		fmt.Fprintf(os.Stderr, "[mtls-bridge] connection lost: %v, reconnecting...\n", err)
	}
```

Add the `nextBackoff` helper function:

```go
func nextBackoff(current, max time.Duration) time.Duration {
	next := current * 2
	if next > max {
		return max
	}
	return next
}
```

Also add `"net"` to imports if not already present, and use `tls.DialWithDialer` instead of `tls.Dial`.

- [ ] **Step 2: Verify build**

```bash
go build ./cmd/mtls-bridge
```

Expected: Clean build.

- [ ] **Step 3: Commit**

```bash
git add cmd/mtls-bridge/main.go
git commit -m "feat: add reconnect with exponential backoff to connect command"
```

---

### Task 7: CI Workflow

**Files:**
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Create CI workflow**

```yaml
# file: .github/workflows/ci.yml
# version: 1.0.0

name: CI

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]
  workflow_dispatch:

concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true

permissions:
  contents: write
  actions: write
  checks: write
  id-token: write
  attestations: write

jobs:
  ci:
    name: Run CI
    uses: jdfalk/ghcommon/.github/workflows/reusable-ci.yml@378e23a96c00d719075732dd2af4de45f7523cbb # v1.10.4
    with:
      go-version: '1.26'
      coverage-threshold: '80'
      frontend-enabled: false
      cgo-enabled: false
    secrets: inherit
```

- [ ] **Step 2: Commit**

```bash
mkdir -p .github/workflows
git add .github/workflows/ci.yml
git commit -m "ci: add CI workflow using ghcommon reusable workflow"
```

---

### Task 8: CodeQL Workflow

**Files:**
- Create: `.github/workflows/codeql.yml`

- [ ] **Step 1: Create CodeQL workflow**

```yaml
# file: .github/workflows/codeql.yml
# version: 1.0.0

name: CodeQL

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]
  schedule:
    - cron: "0 6 * * 1"

concurrency:
  group: codeql-${{ github.ref }}
  cancel-in-progress: true

permissions:
  contents: read
  security-events: write

jobs:
  analyze:
    name: Analyze Go
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: "1.26"

      - uses: github/codeql-action/init@v3
        with:
          languages: go

      - uses: github/codeql-action/autobuild@v3

      - uses: github/codeql-action/analyze@v3
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/codeql.yml
git commit -m "ci: add CodeQL security scanning"
```

---

### Task 9: Release Workflow and GoReleaser

**Files:**
- Create: `.github/workflows/release.yml`
- Create: `.goreleaser.yml`

- [ ] **Step 1: Create release workflow**

```yaml
# file: .github/workflows/release.yml
# version: 1.0.0

name: Release

on:
  workflow_dispatch:
    inputs:
      release-type:
        description: 'Release type'
        required: true
        type: choice
        options:
          - auto
          - major
          - minor
          - patch
        default: 'auto'

concurrency:
  group: release-${{ github.ref }}
  cancel-in-progress: false

permissions:
  contents: write
  packages: write
  id-token: write
  attestations: write

jobs:
  release:
    name: Create Release
    uses: jdfalk/ghcommon/.github/workflows/reusable-release.yml@378e23a96c00d719075732dd2af4de45f7523cbb # v1.10.4
    with:
      release-type: ${{ github.event.inputs.release-type }}
      prerelease: false
      go-enabled: true
      frontend-enabled: false
      docker-enabled: false
      cgo-enabled: false
    secrets: inherit
```

- [ ] **Step 2: Create `.goreleaser.yml`**

```yaml
# file: .goreleaser.yml
# version: 1.0.0

version: 2

before:
  hooks:
    - go mod tidy

builds:
  - id: mtls-bridge
    main: ./cmd/mtls-bridge
    binary: mtls-bridge
    env:
      - CGO_ENABLED=0
    goos:
      - linux
      - darwin
      - windows
    goarch:
      - amd64
      - arm64
    ldflags:
      - -s -w
      - -X main.version={{.Version}}
      - -X main.commit={{.Commit}}
      - -X main.date={{.Date}}

archives:
  - id: mtls-bridge-archive
    formats:
      - tar.gz
    name_template: >-
      {{ .ProjectName }}_
      {{- .Version }}_
      {{- title .Os }}_
      {{- if eq .Arch "amd64" }}x86_64
      {{- else }}{{ .Arch }}{{ end }}
    format_overrides:
      - goos: windows
        formats:
          - zip
    files:
      - LICENSE
      - README.md

checksum:
  name_template: "checksums.txt"
  algorithm: sha256

snapshot:
  version_template: "{{ incpatch .Version }}-next"

changelog:
  sort: asc
  use: github
  groups:
    - title: Features
      regexp: '^.*?feat(\([[:word:]]+\))??!?:.+$'
      order: 0
    - title: Bug Fixes
      regexp: '^.*?fix(\([[:word:]]+\))??!?:.+$'
      order: 1
    - title: Other
      order: 999
  filters:
    exclude:
      - "^docs:"
      - "^test:"
      - "^chore:"

release:
  disable: false
  prerelease: auto
  name_template: "mtls-bridge v{{.Version}}"
```

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/release.yml .goreleaser.yml
git commit -m "ci: add release workflow and GoReleaser config"
```

---

### Task 10: CODEOWNERS and Final Push

**Files:**
- Create: `.github/CODEOWNERS`

- [ ] **Step 1: Create CODEOWNERS**

```
# file: .github/CODEOWNERS
* @jdfalk
```

- [ ] **Step 2: Commit and push**

```bash
git add .github/CODEOWNERS
git commit -m "chore: add CODEOWNERS"
git push origin main
```

- [ ] **Step 3: Verify CI runs**

```bash
gh run list --repo jdfalk/mtls-bridge --limit 3
```

Expected: CI workflow triggered and running.

---

### Task 11: Cleanup audiobook-organizer

**Files (in audiobook-organizer repo):**
- Delete: `internal/mtls/` (10 files)
- Delete: `cmd/mtls-bridge/` (1 file)
- Modify: `Makefile` — remove `build-mtls-bridge` targets
- Modify: `.gitignore` — remove `mtls-bridge` entries
- Modify: `.mcp.json` — change `./mtls-bridge` to `mtls-bridge`

- [ ] **Step 1: Switch to audiobook-organizer repo**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
```

- [ ] **Step 2: Delete extracted code**

```bash
rm -rf internal/mtls/
rm -rf cmd/mtls-bridge/
```

- [ ] **Step 3: Remove Makefile targets**

Remove these lines from `Makefile`:
- The `build-mtls-bridge` target and its recipe
- The `build-mtls-bridge-windows` target and its recipe
- References to `build-mtls-bridge` in `.PHONY`

- [ ] **Step 4: Remove from `.gitignore`**

Remove these lines:
```
# mTLS bridge certificates and config
.mtls/

# mTLS bridge binaries
mtls-bridge
mtls-bridge.exe
```

- [ ] **Step 5: Update `.mcp.json`**

Change `"./mtls-bridge"` to `"mtls-bridge"` (rely on PATH):

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

- [ ] **Step 6: Run go mod tidy**

```bash
go mod tidy
```

- [ ] **Step 7: Verify build**

```bash
go build ./...
```

Expected: Clean build (no references to internal/mtls remain).

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "refactor: extract mtls-bridge to github.com/jdfalk/mtls-bridge"
```

---

### Task 12: Create Initial Release

**Files:** None (GitHub Actions)

- [ ] **Step 1: Tag and release v1.0.0**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/mtls-bridge
git tag v1.0.0
git push origin v1.0.0
```

Or trigger via workflow dispatch:

```bash
gh workflow run release.yml --repo jdfalk/mtls-bridge -f release-type=minor
```

- [ ] **Step 2: Verify release**

```bash
gh release view v1.0.0 --repo jdfalk/mtls-bridge
```

Expected: Release with binaries for all platforms + checksums.txt.

- [ ] **Step 3: Test auto-update**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/mtls-bridge
go build -ldflags="-X main.version=v0.0.1" -o mtls-bridge ./cmd/mtls-bridge
./mtls-bridge update
```

Expected: Downloads and replaces binary with v1.0.0.
