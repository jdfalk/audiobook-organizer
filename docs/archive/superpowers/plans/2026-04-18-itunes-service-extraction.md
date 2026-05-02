<!-- file: docs/superpowers/plans/2026-04-18-itunes-service-extraction.md -->
<!-- version: 1.0.0 -->
<!-- guid: 03acd2fc-1cd1-4a2c-bc5a-4d2939254e0f -->

# iTunes Service Extraction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract the iTunes integration from `internal/server/itunes*.go` (~6,060 lines across 9 files, 67 references in `server.go`) into a new `internal/itunes/service/` sub-package with explicit dependencies, lifecycle hooks, and first-class disabled mode — with zero behavior change.

**Architecture:** One top-level `itunesservice.Service` struct that composes seven sub-components (Importer, Batcher, Positions, Paths, Playlists, Provisioner, Transfer) and two stateless helpers (ValidateITL, TestMapping). Server holds a single `s.itunesSvc` field, constructs either `itunesservice.New(deps)` or `itunesservice.NewDisabled()` based on config, and wires `Start(ctx)`/`Shutdown(timeout)` into its existing lifecycle. HTTP handlers stay in `internal/server/` but consolidate into one `itunes_handlers.go` file of thin wrappers that call `s.itunesSvc.*`. Three PRs: foundation → per-component move (7 commits inside one PR) → consolidate-handlers-and-delete.

**Tech Stack:** Go 1.26, gin (HTTP), existing `internal/itunes` low-level package (ITL parser etc. — not touched), mockery for narrow Store mocks, `make test-short` for fast iteration.

**Spec:** `docs/superpowers/specs/2026-04-18-itunes-service-extraction-design.md`

---

## Execution model

Per the spec's section 4, this is **three PRs**, not three tasks. Each PR is merged before the next begins so the codebase compiles and tests pass between each checkpoint. Inside PR 2, each sub-component moves as one commit (7 commits total) so review + bisect stay sane.

| PR | Rough size | Depends on | Risk |
|---|---|---|---|
| 1: Foundation | ~500 lines new | — | low (adds, doesn't remove) |
| 2: Move sub-components | ~5000 lines moved | PR 1 | medium (behavior preserved by existing tests) |
| 3: Handlers + deletions | ~500 net delta | PR 2 | low (mechanical) |

For every PR: use the Quick Fix Workflow from `CLAUDE.md` (branch from main → commit → push → PR → `gh pr merge <n> --rebase --admin`). Use `make test-short` for verification — full `go test ./...` takes ~15 min and isn't needed for type-level changes.

**Critical pre-flight check for every commit:** `go vet ./...` (project-wide, not scoped). The PR #394 regression was caused by running `go vet ./internal/server/` instead of the whole tree — that variant misses test-file compile breakage in other packages. Don't repeat it.

---

## Task 1 — PR 1: Foundation

Creates `internal/itunes/service/` with the shell types and lifecycle. Server gets a `s.itunesSvc` field wired to `NewDisabled()`. No behavior moved yet — this PR just puts the scaffolding in place.

**Files created:**
- `internal/itunes/service/service.go` — `Service`, `Deps`, `New`, `NewDisabled`, `Start`, `Shutdown`
- `internal/itunes/service/store.go` — narrow `Store` interface
- `internal/itunes/service/config.go` — `Config`, `PathMapping`
- `internal/itunes/service/errors.go` — `ErrITunesDisabled` + other sentinels
- `internal/itunes/service/types.go` — placeholder (populated by Task 2g)
- `internal/itunes/service/service_test.go` — smoke tests for New / NewDisabled / Start / Shutdown

**Files modified:**
- `internal/server/server.go` — add `itunesSvc *itunesservice.Service` field, construct in `NewServer`, call `Start`/`Shutdown`
- No existing iTunes files touched yet

- [ ] **Step 1.1: Create the worktree**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
git fetch origin main
git worktree add .worktrees/itunes-svc-foundation -b feat/itunes-svc-foundation origin/main
cd .worktrees/itunes-svc-foundation
```

- [ ] **Step 1.2: Create `internal/itunes/service/store.go`**

Generate a fresh GUID: `uuidgen | tr '[:upper:]' '[:lower:]'`.

```go
// file: internal/itunes/service/store.go
// version: 1.0.0
// guid: <fresh-uuid>

// Package itunesservice contains the iTunes integration: import pipeline,
// ITL write-back batcher, position sync, path reconcile, playlist sync,
// track provisioner, and ITL transfer. The low-level ITL parser, fingerprint,
// path mapping, and smart-criteria translator live in the parent package
// internal/itunes and are untouched by this extraction.
//
// See docs/superpowers/specs/2026-04-18-itunes-service-extraction-design.md.
package itunesservice

import "github.com/jdfalk/audiobook-organizer/internal/database"

// Store is the narrow slice of database.Store that the iTunes service
// uses. Wide because iTunes is a hub — books, authors, series, files,
// tags, external IDs, operations, preferences, playlists, fingerprints
// — but still smaller than full database.Store.
type Store interface {
	database.BookStore
	database.AuthorStore
	database.SeriesStore
	database.NarratorStore
	database.BookFileStore
	database.HashBlocklistStore
	database.ITunesStateStore
	database.ExternalIDStore
	database.UserPositionStore
	database.UserPlaylistStore
	database.UserPreferenceStore
	database.OperationStore
	database.SettingsStore
	database.MetadataStore
	database.TagStore
	database.RawKVStore
}
```

- [ ] **Step 1.3: Create `internal/itunes/service/config.go`**

```go
// file: internal/itunes/service/config.go
// version: 1.0.0
// guid: <fresh-uuid>

package itunesservice

import "time"

// Config is the iTunes-specific slice of config.AppConfig, passed by
// value at construction so the service has no transitive dependency on
// the global config singleton.
type Config struct {
	Enabled           bool
	LibraryReadPath   string
	LibraryWritePath  string
	DefaultMappings   []PathMapping
	SyncInterval      time.Duration
	WriteBackInterval time.Duration
	WriteBackMaxBatch int
	BackupKeep        int
	ImportConcurrency int
}

// PathMapping is a single ITunesPath → OrganizedPath transform applied
// during import when iTunes PIDs resolve to a different filesystem
// location than the library's canonical layout.
type PathMapping struct {
	From string
	To   string
}
```

- [ ] **Step 1.4: Create `internal/itunes/service/errors.go`**

```go
// file: internal/itunes/service/errors.go
// version: 1.0.0
// guid: <fresh-uuid>

package itunesservice

import "errors"

// ErrITunesDisabled is returned by methods called on a Service
// constructed with NewDisabled. Callers should surface this as a 503
// Service Unavailable at the HTTP layer.
var ErrITunesDisabled = errors.New("iTunes integration is disabled")

// ErrNotImplemented is a placeholder returned from sub-component method
// stubs until they're filled in during PR 2. Should never appear on
// main after PR 2 merges.
var ErrNotImplemented = errors.New("iTunes service method not yet implemented")
```

- [ ] **Step 1.5: Create `internal/itunes/service/types.go` (placeholder)**

```go
// file: internal/itunes/service/types.go
// version: 1.0.0
// guid: <fresh-uuid>

// This file holds the request/response types used by the iTunes service's
// HTTP surface. Populated by PR 2 (Importer task) — until then it only
// carries the file header.

package itunesservice
```

- [ ] **Step 1.6: Create `internal/itunes/service/service.go`**

```go
// file: internal/itunes/service/service.go
// version: 1.0.0
// guid: <fresh-uuid>

package itunesservice

import (
	"context"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/logger"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	"github.com/jdfalk/audiobook-organizer/internal/realtime"
)

// Deps is the explicit dependency set for Service. No globals, no Server,
// no config.AppConfig — everything the service needs is passed in.
type Deps struct {
	Store      Store
	OpQueue    operations.Queue
	ActivityFn func(database.ActivityEntry)
	Realtime   *realtime.EventHub // may be nil; means no SSE push
	Config     Config
	Logger     logger.Logger
}

// Service owns the iTunes integration. Prefer a single *Service on the
// Server struct — it composes the seven sub-components below with shared
// lifecycle (Start / Shutdown).
type Service struct {
	deps Deps

	// Sub-components. Nil when the service is disabled; populated by New.
	Importer    *Importer
	Batcher     *WriteBackBatcher
	Positions   *PositionSync
	Paths       *PathReconciler
	Playlists   *PlaylistSync
	Provisioner *TrackProvisioner
	Transfer    *TransferService
}

// New constructs a fully-wired iTunes service. Returns ErrITunesDisabled
// equivalent (cfg.Enabled == false) routes through NewDisabled instead —
// callers should branch on cfg.Enabled at the construction site.
func New(deps Deps) (*Service, error) {
	if !deps.Config.Enabled {
		return NewDisabled(), nil
	}
	if deps.Logger == nil {
		deps.Logger = logger.New("itunes")
	}
	return &Service{
		deps: deps,
		// Sub-components populated in PR 2. Until then they stay nil;
		// method calls on a nil sub-component return ErrNotImplemented.
	}, nil
}

// NewDisabled constructs a Service whose methods all return
// ErrITunesDisabled. Use when cfg.Enabled == false so the rest of the
// server can still wire a non-nil *Service and avoid nil guards at every
// call site.
func NewDisabled() *Service {
	return &Service{}
}

// Enabled reports whether the service has active sub-components wired.
// A disabled service returns false; a real service returns true once
// Start has run (or immediately — PR 2 decides per component).
func (s *Service) Enabled() bool {
	// PR 2 will refine: "enabled and started" vs "enabled but not yet
	// started". For now, Enabled == cfg.Enabled.
	return s.deps.Config.Enabled
}

// Start launches any long-lived sub-component goroutines (currently just
// the WriteBackBatcher, wired in PR 2's step 2f). No-op when disabled.
func (s *Service) Start(ctx context.Context) error {
	if !s.Enabled() {
		return nil
	}
	// Sub-component Start calls added in PR 2. This skeleton is a no-op
	// so PR 1 can ship without behavior change.
	return nil
}

// Shutdown flushes any long-lived sub-components and waits up to timeout
// for graceful completion. No-op when disabled.
func (s *Service) Shutdown(timeout time.Duration) error {
	if !s.Enabled() {
		return nil
	}
	// Sub-component Shutdown calls added in PR 2.
	return nil
}
```

- [ ] **Step 1.7: Create `internal/itunes/service/service_test.go`**

```go
// file: internal/itunes/service/service_test.go
// version: 1.0.0
// guid: <fresh-uuid>

package itunesservice

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNewDisabled_ReturnsService(t *testing.T) {
	svc := NewDisabled()
	if svc == nil {
		t.Fatal("NewDisabled returned nil")
	}
	if svc.Enabled() {
		t.Error("disabled service should report Enabled() == false")
	}
}

func TestNew_WithDisabledConfig_ReturnsDisabledService(t *testing.T) {
	svc, err := New(Deps{Config: Config{Enabled: false}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if svc.Enabled() {
		t.Error("service constructed with Enabled=false should report Enabled() == false")
	}
}

func TestService_StartShutdown_Disabled_NoOp(t *testing.T) {
	svc := NewDisabled()
	if err := svc.Start(context.Background()); err != nil {
		t.Errorf("Start on disabled: %v", err)
	}
	if err := svc.Shutdown(100 * time.Millisecond); err != nil {
		t.Errorf("Shutdown on disabled: %v", err)
	}
}

func TestErrITunesDisabled_Exported(t *testing.T) {
	// Sanity check that ErrITunesDisabled exists and is an error. Prevents
	// an accidental rename from breaking call sites that sentinel-check.
	if !errors.Is(ErrITunesDisabled, ErrITunesDisabled) {
		t.Fatal("ErrITunesDisabled failed errors.Is identity check")
	}
}
```

- [ ] **Step 1.8: Run the new package's build + tests**

```bash
go build ./internal/itunes/service/
go test ./internal/itunes/service/ -count=1 -v
```

Expected: all tests pass; no warnings.

- [ ] **Step 1.9: Verify `*database.PebbleStore` still satisfies the new `itunesservice.Store`**

Add a compile-time assertion. Create a new file `internal/itunes/service/assert_test.go`:

```go
// file: internal/itunes/service/assert_test.go
// version: 1.0.0
// guid: <fresh-uuid>

package itunesservice_test

import (
	"github.com/jdfalk/audiobook-organizer/internal/database"
	itunesservice "github.com/jdfalk/audiobook-organizer/internal/itunes/service"
)

// Compile-time proof that *database.PebbleStore satisfies
// itunesservice.Store. If a method is renamed or removed from PebbleStore,
// the assertion below fails to build — we find out here rather than at
// the Server wiring step.
var _ itunesservice.Store = (*database.PebbleStore)(nil)
```

This file lives in the `itunesservice_test` external test package to avoid any import cycle risk.

Run `go build ./...` — must succeed. If it fails, the `Store` interface in step 1.2 doesn't match what `PebbleStore` exposes; fix the interface.

- [ ] **Step 1.10: Wire `s.itunesSvc` into Server**

Edit `internal/server/server.go`. Find the `Server` struct definition and add the field:

```go
// Find this cluster of iTunes-related fields:
libraryWatcher         *itunes.LibraryWatcher
// ... other iTunes fields ...

// Add this line alongside them. Do NOT remove the other fields yet —
// they're still in active use. They go away in PR 2/3.
itunesSvc              *itunesservice.Service
```

Add the import:

```go
itunesservice "github.com/jdfalk/audiobook-organizer/internal/itunes/service"
```

In `NewServer` (grep for `func NewServer` in `internal/server/server.go`), after the existing Store resolution but before the scheduler/watcher setup, add:

```go
// Construct the iTunes service. PR 1 always uses NewDisabled — PR 2
// flips to conditional New based on config once sub-components are
// moved. Server still has the old *WriteBackBatcher, *LibraryWatcher,
// etc. fields populated via the existing code paths during PR 1+2.
server.itunesSvc = itunesservice.NewDisabled()
```

In the `Start(ctx)` method (grep `func (s *Server) Start`), add at the top of the function body:

```go
if err := s.itunesSvc.Start(ctx); err != nil {
    return fmt.Errorf("itunes service start: %w", err)
}
```

In the `Shutdown(timeout)` method, add before the final return:

```go
if err := s.itunesSvc.Shutdown(timeout); err != nil {
    log.Printf("[WARN] itunes service shutdown: %v", err)
}
```

- [ ] **Step 1.11: Full build + vet + short-mode tests**

```bash
go build ./...
go vet ./...
make test-short
```

All three must be clean. This PR doesn't touch any existing iTunes code paths, so all existing iTunes tests should still pass unchanged.

- [ ] **Step 1.12: Bump version headers**

```bash
v=$(grep "^// version:" internal/server/server.go | head -1 | awk '{print $3}')
major=${v%%.*}; rest=${v#*.}; minor=${rest%%.*}
sed -i '' "s|^// version: $v|// version: $major.$((minor+1)).0|" internal/server/server.go
```

(The new files already have `// version: 1.0.0`.)

- [ ] **Step 1.13: Commit**

```bash
git add internal/itunes/service/ internal/server/server.go
git commit -m "$(cat <<'EOF'
feat(itunes): foundation for service extraction (PR 1/3)

Creates internal/itunes/service/ sub-package with Service shell, narrow
Store interface, Config value type, and Deps. Adds s.itunesSvc *Service
field to Server, constructed via NewDisabled() — no behavior change.

PR 2 moves sub-components (Transfer, Provisioner, PositionSync,
PathReconciler, PlaylistSync, WriteBackBatcher, Importer). PR 3
consolidates handlers and deletes the old internal/server/itunes*.go
files.

Spec: docs/superpowers/specs/2026-04-18-itunes-service-extraction-design.md
Plan: docs/superpowers/plans/2026-04-18-itunes-service-extraction.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 1.14: Push, PR, merge**

```bash
git push -u origin feat/itunes-svc-foundation
gh pr create --title "feat(itunes): foundation for service extraction (PR 1/3)" --body "First of three PRs extracting iTunes from internal/server/ per spec 2026-04-18-itunes-service-extraction-design.md.

## Scope
- New package \`internal/itunes/service/\` with \`Service\`, \`Deps\`, \`Config\`, narrow \`Store\` interface
- \`Server\` gets a \`s.itunesSvc\` field wired to \`NewDisabled()\` — no behavior change
- Sub-components moved in PR 2; handlers consolidated + old files deleted in PR 3

## Test plan
- [x] \`go build ./...\` clean
- [x] \`go vet ./...\` clean
- [x] \`go test ./internal/itunes/service/\` green (service + assert tests)
- [x] \`make test-short\` green (no existing behavior changed)"
gh pr merge $(gh pr view --json number -q .number) --rebase --admin
```

---

## Task 2 — PR 2: Move sub-components

Seven commits inside one branch, in dependency order (simplest first, Importer last). Each commit is a self-contained move + rewire + verify cycle.

**Files created (new location):**
- `internal/itunes/service/transfer.go` — from `internal/server/itunes_transfer.go`
- `internal/itunes/service/transfer_test.go` — from `internal/server/itunes_transfer_test.go`
- `internal/itunes/service/track_provisioner.go` — from `internal/server/itunes_track_provisioner.go`
- `internal/itunes/service/position_sync.go` — from `internal/server/itunes_position_sync.go`
- `internal/itunes/service/position_sync_test.go` — from `internal/server/itunes_position_sync_test.go`
- `internal/itunes/service/path_reconcile.go` — from `internal/server/itunes_path_reconcile.go`
- `internal/itunes/service/playlist_sync.go` — from `internal/server/playlist_itunes_sync.go`
- `internal/itunes/service/playlist_sync_test.go` — from `internal/server/playlist_itunes_sync_test.go`
- `internal/itunes/service/writeback_batcher.go` — from `internal/server/itunes_writeback_batcher.go`
- `internal/itunes/service/writeback_batcher_test.go` — from `internal/server/itunes_writeback_batcher_test.go`
- `internal/itunes/service/importer.go` — from bulk of `internal/server/itunes.go`
- `internal/itunes/service/status.go` — from status tracker portion of `internal/server/itunes.go`
- `internal/itunes/service/importer_test.go` — from `internal/server/itunes_import_integration_test.go` and `itunes_integration_test.go` (merge or split by concern)

**Files modified:**
- `internal/server/server.go` — constructor flips `NewDisabled()` → conditional `New(deps)` / `NewDisabled()`; removes `writeBackBatcher`, `libraryWatcher`, `itunesActivityFn` fields after they're relocated
- `internal/server/audiobook_service.go`, `internal/server/ai_handlers.go`, etc. — any file that references a moved function (e.g., `ProvisionITLTracksForBook`, `NotifyDelugeAfterVersionSwap` already done in the sweep) updates to `s.itunesSvc.Provisioner.Provision(...)` style
- `internal/server/import_service.go` — `ProvisionITLTracksForBook` call → `is.itunesSvc.Provisioner.ProvisionAll(book)` (requires a field add — explicit in the step)

### Task 2 setup

- [ ] **Step 2.0: Create the worktree**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
git fetch origin main
git worktree add .worktrees/itunes-svc-move -b feat/itunes-svc-move origin/main
cd .worktrees/itunes-svc-move
```

---

### Step 2a — Move TransferService (smallest, isolated)

Transfer handles ITL file download/upload/backup/restore. Lives in `itunes_transfer.go` today. Minimal coupling — good starting point.

- [ ] **Step 2a.1: Move the file**

```bash
git mv internal/server/itunes_transfer.go internal/itunes/service/transfer.go
git mv internal/server/itunes_transfer_test.go internal/itunes/service/transfer_test.go
```

- [ ] **Step 2a.2: Rewrite the header + package line**

Edit `internal/itunes/service/transfer.go`:

- Change `package server` → `package itunesservice`
- Update file header: `// file: internal/itunes/service/transfer.go` and bump version one minor
- Remove any unneeded imports (e.g., `internal/server/...` self-references)

Same treatment for `transfer_test.go`.

- [ ] **Step 2a.3: Extract the Transfer type**

The current file has functions hanging off `Server` (e.g., `func (s *Server) handleITunesDownload(c *gin.Context)`). Separate the logic from the HTTP handler:

- Identify the business logic functions (anything that's not a `gin.HandlerFunc`). These become methods on a new `TransferService` type.
- HTTP handlers stay in `internal/server/itunes_transfer.go`, which we temporarily leave in place — PR 3 will consolidate those into `itunes_handlers.go` and delete.

Top of `transfer.go`:

```go
// TransferService owns ITL file transfer operations: download the live
// library, upload + validate a candidate, list .bak-* backups, restore
// from backup. Thin wrapper over the internal/itunes package plus
// filesystem atomic-rename primitives.
type TransferService struct {
	cfg Config
	log logger.Logger
}

func newTransferService(cfg Config, log logger.Logger) *TransferService {
	return &TransferService{cfg: cfg, log: log}
}

// Download returns a reader for the current live ITL file. Caller
// closes.
func (t *TransferService) Download(ctx context.Context) (io.ReadCloser, error) {
	// Body: port from existing handleITunesDownload, but return an
	// io.ReadCloser instead of writing to gin.Context.
}

// Upload validates the candidate ITL reader. If install is true, it
// backs up the current ITL and installs the candidate atomically.
// Returns the validation result.
func (t *TransferService) Upload(ctx context.Context, r io.Reader, install bool) (ValidateResult, error) {
	// Port from existing handleITunesUpload minus gin specifics.
}

// Backups lists .bak-* files, newest first.
func (t *TransferService) Backups() ([]BackupEntry, error) {
	// Port from existing handleITunesListBackups.
}

// Restore validates the named backup, backs up current, and installs.
func (t *TransferService) Restore(ctx context.Context, filename string) error {
	// Port from existing handleITunesRestore.
}
```

For each handler in the old file (`handleITunesDownload`, `handleITunesUpload`, `handleITunesListBackups`, `handleITunesRestore`): extract the business logic into the method above, leave the handler as a thin shim that parses the request, calls the method, and writes the response.

- [ ] **Step 2a.4: Add `Transfer` field to `Service.New`**

Edit `internal/itunes/service/service.go`. In `New`:

```go
return &Service{
	deps:     deps,
	Transfer: newTransferService(deps.Config, deps.Logger),
	// other sub-components added in later steps
}, nil
```

- [ ] **Step 2a.5: Add types**

Move `ValidateResult`, `BackupEntry`, and any related request/response types used by transfer from the top of `internal/server/itunes.go` into `internal/itunes/service/types.go` (or alongside in `transfer.go` if they're only used by Transfer).

- [ ] **Step 2a.6: Put a temporary bridge in the old handlers**

Edit `internal/server/itunes_transfer.go` (the old file — we're going to leave a shim here for PR 2, then delete it in PR 3). Each handler becomes:

```go
func (s *Server) handleITunesDownload(c *gin.Context) {
	if !s.itunesSvc.Enabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": itunesservice.ErrITunesDisabled.Error()})
		return
	}
	rc, err := s.itunesSvc.Transfer.Download(c.Request.Context())
	if err != nil {
		internalError(c, "itunes download", err)
		return
	}
	defer rc.Close()
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", `attachment; filename="iTunes Library.xml"`)
	_, _ = io.Copy(c.Writer, rc)
}
```

Same pattern for the three other transfer handlers.

- [ ] **Step 2a.7: Build + vet + test**

```bash
go build ./...
go vet ./...
go test ./internal/itunes/service/ -count=1
go test ./internal/server/ -run Transfer -count=1 -short
```

All must pass. The transfer tests follow their code into the new package; the server-side handler tests (if any) still exercise the shim handlers.

- [ ] **Step 2a.8: Commit**

```bash
git add -A
git commit -m "refactor(itunes): move Transfer into itunesservice (PR 2/3, 1/7)

Moves ITL transfer logic into internal/itunes/service/TransferService.
HTTP handlers stay in internal/server/itunes_transfer.go as shims that
call svc.Transfer.* — those shims consolidate in PR 3.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Step 2b — Move TrackProvisioner

`TrackProvisioner` generates ITL tracks for new books and is called from `ImportService` + `itunes.go` import pipeline. Leaf — no sub-dependencies beyond Store + Logger.

- [ ] **Step 2b.1: Move the file**

```bash
git mv internal/server/itunes_track_provisioner.go internal/itunes/service/track_provisioner.go
```

(The associated test — `compute_itunes_path_test.go` — stays because it tests pure path-computation helpers; keep them in server package unless they naturally belong with Provisioner. Check with `grep -l "TestProvisionITLTrack\|TestProvisionITLTracksForBook" internal/server/*_test.go`. Move any test that references the moved functions.)

- [ ] **Step 2b.2: Rewrite header + package**

Same pattern as 2a.2. Change `package server` → `package itunesservice`, update file header, fix imports.

- [ ] **Step 2b.3: Extract the TrackProvisioner type**

The current file has free functions `ProvisionITLTrack(store database.Store, book *database.Book, ...)` and `ProvisionITLTracksForBook(...)`. Convert to methods on a struct:

```go
type TrackProvisioner struct {
	store Store // narrow — reuses the package-level Store interface
	cfg   Config
	log   logger.Logger
}

func newTrackProvisioner(store Store, cfg Config, log logger.Logger) *TrackProvisioner {
	return &TrackProvisioner{store: store, cfg: cfg, log: log}
}

// Provision generates an ITL track for a single book file. Writes the
// mapping into ExternalIDStore and enqueues an ITL add via the batcher.
// batcher may be nil in tests — non-nil in production (wired by Service).
func (p *TrackProvisioner) Provision(book *database.Book, bookFile *database.BookFile, batcher *WriteBackBatcher) error {
	// Port from existing ProvisionITLTrack. Replace free-function calls
	// on store with p.store.
}

// ProvisionAll iterates all book files for a book and provisions each.
func (p *TrackProvisioner) ProvisionAll(book *database.Book, batcher *WriteBackBatcher) error {
	// Port from existing ProvisionITLTracksForBook.
}
```

The `batcher` parameter threads because Provisioner needs to enqueue ITL adds. Rather than holding a batcher reference on the Provisioner (which creates a lifecycle ordering problem during New), we pass it per-call. The Service's public wrapper methods will pass `s.Batcher` automatically:

```go
// On *Service — in service.go
func (s *Service) ProvisionTrack(book *database.Book, bookFile *database.BookFile) error {
	if !s.Enabled() { return ErrITunesDisabled }
	return s.Provisioner.Provision(book, bookFile, s.Batcher)
}
```

- [ ] **Step 2b.4: Wire into `Service.New`**

```go
// In service.go New:
svc := &Service{
	deps:     deps,
	Transfer: newTransferService(deps.Config, deps.Logger),
}
svc.Provisioner = newTrackProvisioner(deps.Store, deps.Config, deps.Logger)
return svc, nil
```

(Order matters in later steps: Batcher must be constructed before Provisioner if Provisioner holds a batcher reference. We're keeping batcher out of the Provisioner struct, so order is flexible here.)

- [ ] **Step 2b.5: Update callers**

Grep for the old function names:

```bash
grep -rn "ProvisionITLTrack\|ProvisionITLTracksForBook" internal/server/ cmd/
```

For each caller:
- If it's an existing `*Server` method: change `ProvisionITLTracksForBook(s.Store(), book, s.writeBackBatcher)` → `s.itunesSvc.ProvisionTrackAll(book)` (using the public wrapper from step 2b.3).
- If it's in a service that doesn't currently have `itunesSvc`: add a `provisioner *itunesservice.Service` field to that service's struct and wire it at construction. The primary case is `internal/server/import_service.go` — add an `itunesSvc *itunesservice.Service` field and populate it in `NewImportService`.

Concretely for `internal/server/import_service.go`:

```go
// Add to ImportService struct:
itunesSvc *itunesservice.Service

// Update NewImportService signature:
func NewImportService(db importServiceStore, itunesSvc *itunesservice.Service) *ImportService {
	return &ImportService{db: db, itunesSvc: itunesSvc}
}

// In ImportFile (or wherever ProvisionITLTracksForBook is called):
if err := is.itunesSvc.ProvisionTrackAll(created); err != nil {
	log.Printf("[WARN] ITL track provisioning failed for %s: %v", created.ID, err)
}
```

And at the Server construction site (grep `NewImportService(` in server.go):

```go
server.importService = NewImportService(resolvedStore, server.itunesSvc)
```

- [ ] **Step 2b.6: Build + vet + test**

```bash
go build ./...
go vet ./...
go test ./internal/itunes/service/ -count=1
go test ./internal/server/ -run "Import|Provision" -count=1 -short
```

- [ ] **Step 2b.7: Commit**

```bash
git add -A
git commit -m "refactor(itunes): move TrackProvisioner into itunesservice (PR 2/3, 2/7)

Moves ITL track provisioning into itunesservice.TrackProvisioner. Adds
itunesSvc field to ImportService so it can call
svc.ProvisionTrackAll(book) instead of the old free function.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Step 2c — Move PositionSync

Handles bidirectional bookmark sync between iTunes and the app's UserPosition store. Runs on a schedule.

- [ ] **Step 2c.1: Move the files**

```bash
git mv internal/server/itunes_position_sync.go internal/itunes/service/position_sync.go
git mv internal/server/itunes_position_sync_test.go internal/itunes/service/position_sync_test.go
```

- [ ] **Step 2c.2: Rewrite header + package**

Same pattern as 2a.2.

- [ ] **Step 2c.3: Extract `PositionSync` type**

Current file has free function `SyncITunesPositions(store, batcher)`. Convert:

```go
type PositionSync struct {
	store Store
	cfg   Config
	log   logger.Logger
}

func newPositionSync(store Store, cfg Config, log logger.Logger) *PositionSync {
	return &PositionSync{store: store, cfg: cfg, log: log}
}

// Sync runs one pass of bidirectional position sync. pulls iTunes
// bookmarks from the ITL file (user ← iTunes), pushes user positions as
// pending ITL bookmark writes via the batcher (user → iTunes).
// batcher may be nil in tests.
func (p *PositionSync) Sync(batcher *WriteBackBatcher) (pulled, pushed int) {
	// Port from existing SyncITunesPositions.
}
```

Add a public wrapper on Service:

```go
// In service.go:
func (s *Service) SyncPositions() (pulled, pushed int, err error) {
	if !s.Enabled() { return 0, 0, ErrITunesDisabled }
	pulled, pushed = s.Positions.Sync(s.Batcher)
	return pulled, pushed, nil
}
```

- [ ] **Step 2c.4: Wire into `Service.New`**

```go
svc.Positions = newPositionSync(deps.Store, deps.Config, deps.Logger)
```

- [ ] **Step 2c.5: Update callers**

```bash
grep -rn "SyncITunesPositions" internal/server/ cmd/
```

Likely callers: the unified task scheduler closure in `server.go`. Replace `SyncITunesPositions(store, batcher)` with `s.itunesSvc.SyncPositions()`.

- [ ] **Step 2c.6: Build + vet + test**

```bash
go build ./...
go vet ./...
go test ./internal/itunes/service/ -count=1 -run PositionSync
```

- [ ] **Step 2c.7: Commit**

```bash
git add -A
git commit -m "refactor(itunes): move PositionSync into itunesservice (PR 2/3, 3/7)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Step 2d — Move PathReconciler

Runs as an async operation that fixes iTunes path references after library moves.

- [ ] **Step 2d.1: Move the file**

```bash
git mv internal/server/itunes_path_reconcile.go internal/itunes/service/path_reconcile.go
```

(If a test file exists specifically for path reconcile — check with `grep -l "TestITunesPathReconcile\|runITunesPathReconcile" internal/server/*_test.go` — move it too.)

- [ ] **Step 2d.2: Rewrite header + package**

Same pattern.

- [ ] **Step 2d.3: Extract `PathReconciler` type**

```go
type PathReconciler struct {
	store   Store
	opQueue operations.Queue
	cfg     Config
	log     logger.Logger
}

func newPathReconciler(store Store, opQueue operations.Queue, cfg Config, log logger.Logger) *PathReconciler {
	return &PathReconciler{store: store, opQueue: opQueue, cfg: cfg, log: log}
}

// Reconcile runs one full path-reconcile pass as a tracked operation.
// Called from the HTTP handler and from operation-resume at startup.
func (p *PathReconciler) Reconcile(ctx context.Context, opID string, progress operations.ProgressReporter) error {
	// Port from existing runITunesPathReconcile.
}

// Resume re-enters an interrupted reconcile operation at startup.
func (p *PathReconciler) Resume(ctx context.Context, opID string) error {
	// Port from existing resume path in resumeInterruptedOperations.
}
```

Public wrapper on Service:

```go
func (s *Service) ReconcilePaths(ctx context.Context, opID string, progress operations.ProgressReporter) error {
	if !s.Enabled() { return ErrITunesDisabled }
	return s.Paths.Reconcile(ctx, opID, progress)
}
```

- [ ] **Step 2d.4: Wire**

```go
svc.Paths = newPathReconciler(deps.Store, deps.OpQueue, deps.Config, deps.Logger)
```

- [ ] **Step 2d.5: Update callers**

```bash
grep -rn "runITunesPathReconcile\|startITunesPathReconcile" internal/server/ cmd/
```

The `startITunesPathReconcile` handler stays in `internal/server/`; its body changes to call `s.itunesSvc.ReconcilePaths(...)`. The `resumeInterruptedOperations` switch case for `"itunes_path_reconcile"` dispatches to `s.itunesSvc.Paths.Resume(ctx, opID)`.

- [ ] **Step 2d.6: Build + vet + test**

```bash
go build ./...
go vet ./...
make test-short
```

- [ ] **Step 2d.7: Commit**

```bash
git add -A
git commit -m "refactor(itunes): move PathReconciler into itunesservice (PR 2/3, 4/7)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Step 2e — Move PlaylistSync

Handles iTunes smart playlist import + push. Called from the smart-playlist evaluator.

- [ ] **Step 2e.1: Move the files**

```bash
git mv internal/server/playlist_itunes_sync.go internal/itunes/service/playlist_sync.go
git mv internal/server/playlist_itunes_sync_test.go internal/itunes/service/playlist_sync_test.go
```

- [ ] **Step 2e.2: Rewrite header + package**

Same pattern.

- [ ] **Step 2e.3: Extract `PlaylistSync` type**

```go
type PlaylistSync struct {
	store Store
	cfg   Config
	log   logger.Logger
}

func newPlaylistSync(store Store, cfg Config, log logger.Logger) *PlaylistSync {
	return &PlaylistSync{store: store, cfg: cfg, log: log}
}

// MigrateSmart imports iTunes smart playlists into the app's user-playlist
// store on one-time migration.
func (p *PlaylistSync) MigrateSmart(lib *itunes.ITLLibrary) (imported, skipped int) {
	// Port from existing MigrateITunesSmartPlaylists.
}

// PushDirty walks all dirty UserPlaylists and enqueues pending ITL writes
// via the batcher. Returns count pushed.
func (p *PlaylistSync) PushDirty(batcher *WriteBackBatcher) int {
	// Port from existing PushDirtyPlaylistsToITunes.
}
```

Public wrapper:

```go
func (s *Service) PushDirtyPlaylists() (int, error) {
	if !s.Enabled() { return 0, ErrITunesDisabled }
	return s.Playlists.PushDirty(s.Batcher), nil
}
```

- [ ] **Step 2e.4: Wire**

```go
svc.Playlists = newPlaylistSync(deps.Store, deps.Config, deps.Logger)
```

- [ ] **Step 2e.5: Update callers**

```bash
grep -rn "MigrateITunesSmartPlaylists\|PushDirtyPlaylistsToITunes" internal/server/ cmd/
```

Replace with `s.itunesSvc.Playlists.MigrateSmart(lib)` and `s.itunesSvc.PushDirtyPlaylists()`.

- [ ] **Step 2e.6: Build + vet + test**

```bash
go build ./...
go vet ./...
go test ./internal/itunes/service/ -count=1 -run Playlist
```

- [ ] **Step 2e.7: Commit**

```bash
git add -A
git commit -m "refactor(itunes): move PlaylistSync into itunesservice (PR 2/3, 5/7)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Step 2f — Move WriteBackBatcher (with lifecycle wiring)

The batcher has a long-lived goroutine. This commit also wires `Start`/`Shutdown` to launch/flush it.

- [ ] **Step 2f.1: Move the files**

```bash
git mv internal/server/itunes_writeback_batcher.go internal/itunes/service/writeback_batcher.go
git mv internal/server/itunes_writeback_batcher_test.go internal/itunes/service/writeback_batcher_test.go
```

- [ ] **Step 2f.2: Rewrite header + package**

Same pattern. Also rename the exported type if it was something like `WriteBackBatcher` — keep the name, just move packages.

- [ ] **Step 2f.3: Add Start/Shutdown methods to Batcher**

The current batcher likely has an explicit `Start`/`Flush`/`Close` contract. Adapt its lifecycle methods to match `context.Context` + `timeout` signatures:

```go
// On *WriteBackBatcher:
func (b *WriteBackBatcher) Start(ctx context.Context) error {
	// Launch the existing goroutine with ctx for cancellation.
}

func (b *WriteBackBatcher) Shutdown(timeout time.Duration) error {
	// Flush the pending queue with a deadline.
}
```

Keep existing public methods (`Enqueue`, `EnqueueRemove`, etc.) unchanged.

- [ ] **Step 2f.4: Wire into `Service.New` + Service lifecycle**

```go
// In service.go New:
svc.Batcher = newWriteBackBatcher(deps.Store, deps.Config, deps.Logger)

// Update Service.Start:
func (s *Service) Start(ctx context.Context) error {
	if !s.Enabled() { return nil }
	if err := s.Batcher.Start(ctx); err != nil {
		return fmt.Errorf("batcher start: %w", err)
	}
	return nil
}

// Update Service.Shutdown:
func (s *Service) Shutdown(timeout time.Duration) error {
	if !s.Enabled() { return nil }
	if err := s.Batcher.Shutdown(timeout); err != nil {
		return fmt.Errorf("batcher shutdown: %w", err)
	}
	return nil
}
```

- [ ] **Step 2f.5: Remove the old `s.writeBackBatcher` field**

Edit `internal/server/server.go`:

- Remove the `writeBackBatcher *WriteBackBatcher` field from the `Server` struct
- Remove the existing construction and lifecycle code for it (grep `s.writeBackBatcher` or `writeBackBatcher =` to find all sites)
- Any caller that was `s.writeBackBatcher.Enqueue(...)` now uses `s.itunesSvc.Batcher.Enqueue(...)`

- [ ] **Step 2f.6: Update other callers**

```bash
grep -rn "WriteBackBatcher\|writeBackBatcher" internal/server/ cmd/
```

Replace every reference with `s.itunesSvc.Batcher` (or pass via the service for services that already have `itunesSvc`).

The services that currently take a `*WriteBackBatcher` parameter (e.g., `ImportService.SetWriteBackBatcher`, `MergeService.writeBackBatcher`) should either:
- Take `*itunesservice.Service` instead (preferred — they already hold it or can), then use `svc.Batcher`
- Or take `*itunesservice.WriteBackBatcher` directly if they only need the batcher (keeps their dep surface narrow)

Decide per-caller based on what else they need. Don't create a third path.

- [ ] **Step 2f.7: Build + vet + test**

```bash
go build ./...
go vet ./...
go test ./internal/itunes/service/ -count=1 -run WriteBack
make test-short
```

- [ ] **Step 2f.8: Commit**

```bash
git add -A
git commit -m "refactor(itunes): move WriteBackBatcher into itunesservice (PR 2/3, 6/7)

Also wires Service.Start/Shutdown to launch/flush the batcher goroutine,
matching the pattern Server already uses for the scanner + index worker.
Removes the writeBackBatcher field from Server — all callers now use
s.itunesSvc.Batcher.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Step 2g — Move Importer (big one, last)

The import pipeline is the bulk of `internal/server/itunes.go` (~1500+ lines of relevant code). Also hosts the shared `itunesImportStatus` tracker, which moves along with it.

- [ ] **Step 2g.1: Identify the import-related content in `itunes.go`**

Grep for the boundary:

```bash
grep -n "^func " internal/server/itunes.go | head -40
```

Everything NOT already moved in steps 2a–2f and NOT an HTTP handler belongs to the importer. Specifically:

- `executeITunesImport(ctx, store, log, opID, req)` and its helpers (`groupTracksByAlbum`, `enrichITunesImportedBooks`, `organizeImportedBooks`, `linkITunesMetadata`, `linkAsVersion`, `buildBookFromAlbumGroup`, `commonParentDir`, `assignAuthorAndSeries`, `ensureAuthorIDs`, `ensureSeriesID`, `extractSeriesName`)
- `itunesImportStatus` map + `recordITLReadTime` + `checkITLConflict` globals
- The status-query functions (`handleITunesImportStatus`, `handleITunesImportStatusBulk`) — handler bodies stay; logic moves

- [ ] **Step 2g.2: Create `internal/itunes/service/status.go`**

Move the status tracker into its own file:

```go
// file: internal/itunes/service/status.go
// ...

package itunesservice

import "sync"

// importStatusTracker is the concurrent map of per-book import status
// owned by Importer. Was a package-level var in internal/server/itunes.go
// named itunesImportStatus — moved here so there's one instance per
// Importer instead of one per process, which makes tests isolable.
type importStatusTracker struct {
	mu     sync.RWMutex
	byBook map[string]ImportStatus
}

func newImportStatusTracker() *importStatusTracker {
	return &importStatusTracker{byBook: make(map[string]ImportStatus)}
}

func (t *importStatusTracker) Set(bookID string, s ImportStatus) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.byBook[bookID] = s
}

func (t *importStatusTracker) Get(bookID string) (ImportStatus, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	s, ok := t.byBook[bookID]
	return s, ok
}

func (t *importStatusTracker) GetBulk(bookIDs []string) map[string]ImportStatus {
	out := make(map[string]ImportStatus, len(bookIDs))
	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, id := range bookIDs {
		if s, ok := t.byBook[id]; ok {
			out[id] = s
		}
	}
	return out
}
```

Copy the `ImportStatus` type from the old file (whatever shape it has today) into `internal/itunes/service/types.go`.

- [ ] **Step 2g.3: Create `internal/itunes/service/importer.go`**

Move the import functions. Convert from free functions on Store to methods on Importer:

```go
type Importer struct {
	store      Store
	opQueue    operations.Queue
	activityFn func(database.ActivityEntry)
	cfg        Config
	log        logger.Logger
	status     *importStatusTracker
}

func newImporter(deps Deps) *Importer {
	return &Importer{
		store:      deps.Store,
		opQueue:    deps.OpQueue,
		activityFn: deps.ActivityFn,
		cfg:        deps.Config,
		log:        deps.Logger,
		status:     newImportStatusTracker(),
	}
}

// Execute runs one full iTunes import operation. opID is the tracked
// operation ID; req is the user-supplied import parameters.
func (i *Importer) Execute(ctx context.Context, opID string, req ImportRequest, log logger.Logger) error {
	// Port from existing executeITunesImport. Each call to the old
	// free function `foo(store, ...)` becomes `i.foo(...)`.
}

func (i *Importer) Status(bookID string) (ImportStatus, bool) { return i.status.Get(bookID) }
func (i *Importer) StatusBulk(ids []string) map[string]ImportStatus { return i.status.GetBulk(ids) }

// Resume re-enters an interrupted import at startup.
func (i *Importer) Resume(ctx context.Context, opID string) error {
	// Port the resume branch from resumeInterruptedOperations.
}
```

All the private helpers (`groupTracksByAlbum`, `enrichITunesImportedBooks`, etc.) become methods on `*Importer` (or stay as package-level functions if they don't touch state — the former reduces argument-passing).

- [ ] **Step 2g.4: Move the types**

Copy `ITunesImportRequest`, `ITunesImportResponse`, `ITunesValidateRequest`, `ITunesValidateResponse`, `ITunesWriteBackRequest/Response`, `ITunesBookMapping`, `ITunesWriteBackPreviewRequest/Response`, `ITunesImportStatusResponse`, `ITunesTestMappingRequest/Response`, `ITunesTestExample`, `albumGroup`, and any other top-level type defined in `internal/server/itunes.go` into `internal/itunes/service/types.go`.

Rename exported types to remove the `ITunes` prefix where sensible — inside the package they're redundantly namespaced. E.g., `ITunesImportRequest` → `ImportRequest`, `ITunesValidateResponse` → `ValidateResponse`. At the call sites (HTTP handlers), keep the JSON wire format identical by using the same field names + json tags.

- [ ] **Step 2g.5: Move the validate + test-mapping helpers**

Create `internal/itunes/service/validate.go`:

```go
// file: internal/itunes/service/validate.go
// ...

package itunesservice

// ValidateITL opens the ITL file at path, runs ParseITL, and returns
// the validation result. Stateless — no Service needed.
func ValidateITL(path string) (ValidateResult, error) {
	// Port from existing handleITunesValidate body.
}

// TestMapping applies the given path mappings to a sample of iTunes
// track paths and returns the results. Used by the UI "test mapping"
// button. Stateless.
func TestMapping(req TestMappingRequest) (TestMappingResponse, error) {
	// Port from existing handleITunesTestMapping body.
}
```

- [ ] **Step 2g.6: Wire into `Service.New`**

```go
svc.Importer = newImporter(deps)
```

- [ ] **Step 2g.7: Flip Server constructor to conditional New**

Edit `internal/server/server.go`. Find the `server.itunesSvc = itunesservice.NewDisabled()` line from PR 1 and replace with the conditional construction:

```go
itunesCfg := itunesservice.Config{
	Enabled:           config.AppConfig.ITunesEnabled,
	LibraryReadPath:   config.AppConfig.ITunesLibraryReadPath,
	LibraryWritePath:  config.AppConfig.ITunesLibraryWritePath,
	DefaultMappings:   convertPathMappings(config.AppConfig.ITunesPathMappings),
	SyncInterval:      config.AppConfig.ITunesSyncInterval,
	WriteBackInterval: config.AppConfig.ITunesWriteBackInterval,
	WriteBackMaxBatch: config.AppConfig.ITunesWriteBackMaxBatch,
	BackupKeep:        config.AppConfig.ITunesBackupKeep,
	ImportConcurrency: config.AppConfig.ITunesImportConcurrency,
}
itunesDeps := itunesservice.Deps{
	Store:      resolvedStore,
	OpQueue:    operations.GlobalQueue,
	ActivityFn: func(entry database.ActivityEntry) {
		if server.activityService != nil {
			server.activityService.Record(entry)
		}
	},
	Realtime: hub,
	Config:   itunesCfg,
	Logger:   logger.New("itunes"),
}
svc, err := itunesservice.New(itunesDeps)
if err != nil {
	return nil, fmt.Errorf("itunes service: %w", err)
}
server.itunesSvc = svc
```

Add the `convertPathMappings` helper locally (or inline the conversion):

```go
func convertPathMappings(src []config.ITunesPathMapping) []itunesservice.PathMapping {
	out := make([]itunesservice.PathMapping, len(src))
	for i, m := range src {
		out[i] = itunesservice.PathMapping{From: m.From, To: m.To}
	}
	return out
}
```

(Adjust to match the actual `config.ITunesPathMapping` field names — check with `grep -n "ITunesPathMapping" internal/config/*.go`.)

- [ ] **Step 2g.8: Update callers to new signatures**

```bash
grep -rn "executeITunesImport\|handleITunesImport\|itunesImportStatus" internal/server/ cmd/
```

For each caller:
- Import handler: `s.itunesSvc.Importer.Execute(ctx, opID, req, log)` (convert the ImportRequest from the parsed HTTP body)
- Status handler: `s.itunesSvc.Importer.Status(bookID)`
- Bulk status: `s.itunesSvc.Importer.StatusBulk(ids)`
- Resume: `s.itunesSvc.Importer.Resume(ctx, opID)` in the `resumeInterruptedOperations` switch

- [ ] **Step 2g.9: Move + adapt import tests**

```bash
git mv internal/server/itunes_import_integration_test.go internal/itunes/service/import_integration_test.go
git mv internal/server/itunes_integration_test.go internal/itunes/service/integration_test.go
```

Update the `package` line and adjust any `newTestServer()` calls so the tests construct an `itunesservice.Service` directly instead of going through Server. Most integration tests will still need to construct a Server (because they exercise the HTTP handlers); those move only if the tests are testing business logic, not HTTP. Decide test-by-test:

- Tests that do `httptest.NewRequest(...)` → stay in `internal/server/` (they're HTTP tests)
- Tests that call business functions directly → move to `internal/itunes/service/`

If a test file mixes both, split it.

- [ ] **Step 2g.10: Build + vet + short tests**

```bash
go build ./...
go vet ./...
go test ./internal/itunes/service/ -count=1 -v
make test-short
```

- [ ] **Step 2g.11: Commit**

```bash
git add -A
git commit -m "refactor(itunes): move Importer into itunesservice (PR 2/3, 7/7)

Final sub-component move. Also flips Server construction from
NewDisabled() to conditional New(deps)/NewDisabled() based on
ITunesEnabled config, and relocates the itunesImportStatus global
into an Importer-scoped status tracker.

After this commit, internal/server/itunes.go is reduced to HTTP
handlers only — PR 3 consolidates those into itunes_handlers.go and
deletes the source files.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 2 — Push + PR + merge

- [ ] **Step 2.final.1: Full verification**

```bash
go build ./...
go vet ./...
make test-short
```

All three clean.

- [ ] **Step 2.final.2: Push + PR + merge**

```bash
git push -u origin feat/itunes-svc-move
gh pr create --title "refactor(itunes): move sub-components into itunesservice (PR 2/3)" --body "Second of three PRs extracting iTunes. Seven commits, one per sub-component, in dependency order:

1. TransferService (ITL download/upload/backup/restore)
2. TrackProvisioner (generates ITL tracks for new books)
3. PositionSync (bidirectional bookmark sync)
4. PathReconciler (operation — fixes broken paths)
5. PlaylistSync (iTunes smart-playlist import + push)
6. WriteBackBatcher (long-lived goroutine; also wires Service.Start/Shutdown)
7. Importer (the big one; also flips Server to conditional New)

Behavior unchanged — every existing iTunes test still passes. HTTP handlers remain in internal/server/ as shims calling s.itunesSvc.* — PR 3 consolidates.

## Test plan
- [x] go build ./... clean
- [x] go vet ./... clean (full-tree — not scoped)
- [x] make test-short green
- [x] go test ./internal/itunes/service/ -count=1 -v green"
gh pr merge $(gh pr view --json number -q .number) --rebase --admin
```

---

## Task 3 — PR 3: Consolidate handlers + delete old files

Handler bodies in `internal/server/itunes*.go` (left as shims in PR 2) are already thin — this PR moves all of them into a single `itunes_handlers.go` file, adds the disabled-mode smoke test, and deletes the old files.

**Files created:**
- `internal/server/itunes_handlers.go` — all `handleITunes*` methods in one file
- `internal/server/itunes_handlers_test.go` — disabled-mode smoke test

**Files deleted:**
- `internal/server/itunes.go` (handler shell only by end of PR 2; validated empty or near-empty)
- `internal/server/itunes_transfer.go`
- `internal/server/itunes_writeback_batcher.go` (already moved, file should be empty — delete if so)
- `internal/server/itunes_position_sync.go`
- `internal/server/itunes_path_reconcile.go`
- `internal/server/itunes_track_provisioner.go`
- `internal/server/playlist_itunes_sync.go`

- [ ] **Step 3.1: Create the worktree**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
git fetch origin main
git worktree add .worktrees/itunes-svc-handlers -b feat/itunes-svc-handlers origin/main
cd .worktrees/itunes-svc-handlers
```

- [ ] **Step 3.2: Create `internal/server/itunes_handlers.go`**

```go
// file: internal/server/itunes_handlers.go
// version: 1.0.0
// guid: <fresh-uuid>

// itunes_handlers.go holds the HTTP surface for the iTunes service. Each
// handler is a thin wrapper that validates the request, checks whether
// iTunes is enabled, calls svc.itunesSvc.*, and writes the response.
// All business logic lives in internal/itunes/service/.
package server

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/auth"
	itunesservice "github.com/jdfalk/audiobook-organizer/internal/itunes/service"
)

// itunesDisabledResponse is the shared 503 response for iTunes endpoints
// when the service is disabled via config.
func itunesDisabledResponse(c *gin.Context) {
	c.JSON(http.StatusServiceUnavailable, gin.H{
		"error": itunesservice.ErrITunesDisabled.Error(),
	})
}

// Populate each handler by moving the shim bodies from the old files.
// Example shape:

func (s *Server) handleITunesValidate(c *gin.Context) {
	if !s.itunesSvc.Enabled() {
		itunesDisabledResponse(c)
		return
	}
	// ... parse request, call itunesservice.ValidateITL(path), respond ...
}

// (Continue for every handler: handleITunesTestMapping, handleITunesImport,
// handleITunesImportStatus, handleITunesImportStatusBulk,
// handleITunesWriteBack, handleITunesWriteBackAll,
// handleITunesWriteBackPreview, handleListITunesBooks,
// handleITunesDownload, handleITunesUpload, handleITunesListBackups,
// handleITunesRestore, startITunesPathReconcile.)
```

To know which handlers exist, grep the old files before deleting:

```bash
grep -h "^func (s \*Server) handleITunes\|^func (s \*Server) startITunes\|^func (s \*Server) handleList*ITunes*" internal/server/itunes*.go internal/server/playlist_itunes_sync.go
```

Move each handler body into `itunes_handlers.go`. Use the same gin routing patterns already in place; don't change the HTTP surface.

- [ ] **Step 3.3: Delete the old files**

```bash
git rm internal/server/itunes.go
git rm internal/server/itunes_transfer.go
git rm internal/server/itunes_writeback_batcher.go
git rm internal/server/itunes_position_sync.go
git rm internal/server/itunes_path_reconcile.go
git rm internal/server/itunes_track_provisioner.go
git rm internal/server/playlist_itunes_sync.go
```

Any test files for these that aren't already moved — check:

```bash
ls internal/server/itunes*_test.go internal/server/playlist_itunes_sync_test.go 2>/dev/null
```

Move (test belongs with new production code) or delete (test of HTTP handler — keep or rewrite).

- [ ] **Step 3.4: Create `internal/server/itunes_handlers_test.go`**

```go
// file: internal/server/itunes_handlers_test.go
// version: 1.0.0
// guid: <fresh-uuid>

package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestITunesDisabled_ReturnsServiceUnavailable proves the operational-
// isolation (C) goal: with iTunes disabled, every iTunes endpoint
// returns 503 with a clear error message and never calls into the
// service internals.
func TestITunesDisabled_ReturnsServiceUnavailable(t *testing.T) {
	srv := newTestServerWithITunesDisabled(t)

	cases := []struct {
		method, path string
	}{
		{http.MethodPost, "/api/v1/itunes/validate"},
		{http.MethodPost, "/api/v1/itunes/test-mapping"},
		{http.MethodPost, "/api/v1/itunes/import"},
		{http.MethodPost, "/api/v1/itunes/write-back"},
		{http.MethodGet, "/api/v1/itunes/library/download"},
		{http.MethodGet, "/api/v1/itunes/library/backups"},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
			req.Header.Set("Content-Type", "application/json")
			srv.Router().ServeHTTP(w, req)

			require.Equal(t, http.StatusServiceUnavailable, w.Code,
				"disabled iTunes should return 503")
			require.Contains(t, w.Body.String(), "disabled",
				"response should mention that iTunes is disabled")
		})
	}
}
```

Add the helper if it doesn't exist — look for `newTestServer` in `internal/server/server_test.go` and mirror that pattern, but construct with `ITunesEnabled: false` in config:

```go
func newTestServerWithITunesDisabled(t *testing.T) *Server {
	t.Helper()
	cfg := config.Config{
		// ... minimal valid config ...
		ITunesEnabled: false,
	}
	// ... standard test server setup ...
}
```

- [ ] **Step 3.5: Run full verification**

```bash
go build ./...
go vet ./...
make test-short
go test ./internal/server/ -run TestITunesDisabled -count=1 -v
```

All must pass.

- [ ] **Step 3.6: Success-criteria grep checks**

```bash
# Server.go iTunes refs should be <= 15:
grep -cE "itunes|iTunes|ITL" internal/server/server.go

# Old server-side iTunes files should be gone — only itunes_handlers.go remains:
ls internal/server/itunes*.go internal/server/playlist_itunes_sync.go 2>/dev/null

# Line count for remaining server-side iTunes files should be <= 800:
wc -l internal/server/itunes_handlers.go

# No config.AppConfig reads inside the service package:
grep -n "config.AppConfig" internal/itunes/service/ 2>/dev/null | wc -l
# (expect 0)
```

If any of these thresholds fails, investigate before continuing.

- [ ] **Step 3.7: Bump server.go version header**

```bash
v=$(grep "^// version:" internal/server/server.go | head -1 | awk '{print $3}')
major=${v%%.*}; rest=${v#*.}; minor=${rest%%.*}
sed -i '' "s|^// version: $v|// version: $major.$((minor+1)).0|" internal/server/server.go
```

- [ ] **Step 3.8: Commit**

```bash
git add -A
git commit -m "$(cat <<'EOF'
refactor(itunes): consolidate handlers + delete old files (PR 3/3)

All HTTP handlers moved into internal/server/itunes_handlers.go as
thin wrappers over s.itunesSvc.*. Deletes the seven now-empty source
files from internal/server/. Adds a disabled-mode smoke test that
proves every iTunes endpoint returns 503 with a clear error when
ITunesEnabled=false.

Completes the iTunes service extraction. Success criteria:
- server.go iTunes refs: 67 → <= 15
- internal/server/itunes*.go total lines: 6060 → <= 800 (just handlers)
- internal/itunes/service/ total lines: ~5000-5500
- config.AppConfig reads inside service package: 0
- full test suite green

Spec: docs/superpowers/specs/2026-04-18-itunes-service-extraction-design.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 3.9: Push, PR, merge**

```bash
git push -u origin feat/itunes-svc-handlers
gh pr create --title "refactor(itunes): consolidate handlers + delete old files (PR 3/3)" --body "Final PR of the iTunes extraction. Moves every HTTP handler into one \`internal/server/itunes_handlers.go\`, deletes the 7 now-empty files, and adds a disabled-mode smoke test.

## Success criteria
- [x] \`server.go\` iTunes refs ≤ 15 (was 67)
- [x] \`internal/server/itunes*.go\` ≤ 800 lines (was 6060)
- [x] \`internal/itunes/service/\` ≈ 5000–5500 lines
- [x] Zero \`config.AppConfig\` reads in the service package
- [x] Disabled-mode smoke test passes
- [x] \`make test-short\` green

Spec: \`docs/superpowers/specs/2026-04-18-itunes-service-extraction-design.md\`"
gh pr merge $(gh pr view --json number -q .number) --rebase --admin
```

---

## Task 4 — Docs closure

Update `CHANGELOG.md` + `TODO.md` + add future-work TODO entries.

- [ ] **Step 4.1: Create the worktree**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
git fetch origin main
git worktree add .worktrees/itunes-svc-docs -b docs/itunes-svc-close origin/main
cd .worktrees/itunes-svc-docs
```

- [ ] **Step 4.2: Update CHANGELOG.md**

Add a new `#### April 18, 2026 — iTunes service extraction` section under `## [Unreleased]` → `### Added / Changed`. Summarize the three PRs, the three goals (A cleanup, B testability, C operational isolation), and the future-work entries.

Bump header version (`CHANGELOG.md` → minor+1).

- [ ] **Step 4.3: Update TODO.md**

Add two new entries under section 4 (Architecture / Future-Proofing):

```markdown
- [ ] **4.9** Plugin system — pluggable sync targets (iTunes / Plex / …) + download clients (Deluge / Transmission / qBittorrent / rtorrent / NNTP). **L**, investigation. Depends on 4.8 + the iTunes extraction landing (shipped today). The extracted `itunesservice.Service` is the candidate first implementation of a `SyncTarget` interface; existing `internal/deluge` is the template for `DownloadClient`.
- [ ] **4.10** Separate-binary iTunes worker — extract iTunes into its own process (scope D from the extraction brainstorm). **XL**, investigation. Would run on a Mac next to the main server; communicates via gRPC/HTTP using the same `Deps` surface used in-process today.
```

Bump header version.

- [ ] **Step 4.4: Commit + PR + merge**

```bash
git add CHANGELOG.md TODO.md
git commit -m "docs: CHANGELOG + TODO entries for iTunes service extraction

- CHANGELOG: April 18 section summarizing the 3-PR extraction
- TODO: add 4.9 (plugin system investigation) and 4.10 (separate-binary extraction)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
git push -u origin docs/itunes-svc-close
gh pr create --title "docs: CHANGELOG + TODO entries for iTunes service extraction" --body "Closes out the iTunes extraction (PRs 1/3, 2/3, 3/3) with docs updates and the two new backlog items (4.9 plugin system, 4.10 separate-binary)."
gh pr merge $(gh pr view --json number -q .number) --rebase --admin
```

---

## Self-review notes

**Spec coverage check:**

| Spec section | Implemented by |
|---|---|
| §3.1 package layout | Task 1 steps 1.2–1.6, Task 2 step-by-step moves |
| §3.2 Service shape + sub-components | Task 1 step 1.6 (Service skeleton) + Task 2 steps 2a–2g (each sub-component) |
| §3.3 external surface | Task 2 per-component step "Public wrapper on Service" subsections |
| §3.4 Deps / Store / Config | Task 1 steps 1.2, 1.3, 1.6 |
| §3.5 disabled mode | Task 1 step 1.6 (NewDisabled) + Task 3 step 3.4 (smoke test) |
| §3.6 wiring at Server boundary | Task 1 step 1.10 (initial) + Task 2 step 2g.7 (flip to conditional) |
| §4 migration strategy | Task 1–3 overall structure |
| §5 testing | Tests move with their code in steps 2a.1 / 2c.1 / 2e.1 / 2f.1 / 2g.9; Task 3 step 3.4 adds disabled smoke test |
| §6 risks + mitigations | Each task step's verification gates |
| §7 success criteria | Task 3 step 3.6 explicit grep checks |
| §8 future evolution | Task 4 step 4.3 TODO entries |

No gaps.

**Placeholder scan:** Steps that reference "port from existing X" — this is acceptable because the moved code is a mechanical translation of existing production code. The alternative would be copying 6000+ lines into the plan verbatim, which is worse for readability than trusting the implementer to preserve behavior.

**Type consistency:** `Service`, `Importer`, `WriteBackBatcher`, `PositionSync`, `PathReconciler`, `PlaylistSync`, `TrackProvisioner`, `TransferService`, `Store`, `Config`, `Deps`, `ErrITunesDisabled` — names are stable across all tasks. Public wrapper method names (`SyncPositions`, `ReconcilePaths`, `PushDirtyPlaylists`, `ProvisionTrack`, `ProvisionTrackAll`) consistent.

## Success criteria (overall, after PR 3 merges)

- `grep -cE "itunes|iTunes|ITL" internal/server/server.go` drops from 67 to ≤ 15
- `wc -l internal/server/itunes_handlers.go` ≤ 800
- `internal/itunes/service/` ≈ 5000–5500 lines across the 13+ files
- `grep -rn "config.AppConfig" internal/itunes/service/` returns zero
- Full test suite (`go test ./...`) green with only path-rename changes to test files
- Disabled-mode smoke test passes
- Running with `ITunesEnabled=false`: server starts cleanly, iTunes endpoints return 503 with a clear error, no iTunes goroutines start, no iTunes log spam
