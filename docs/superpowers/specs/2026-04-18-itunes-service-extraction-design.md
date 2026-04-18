<!-- file: docs/superpowers/specs/2026-04-18-itunes-service-extraction-design.md -->
<!-- version: 1.0.0 -->
<!-- guid: 6b6b5a4c-63f8-4b67-9554-49f150c942c8 -->

# iTunes Service Extraction — Design

**Status:** brainstormed, awaiting implementation plan.
**Relates to:** completes what the ISP sweep (4.8) couldn't — narrowing the `itunes.go` hub. Prerequisite for a future plugin system (4.9) and separate-binary extraction (4.10).

## 1. Problem

The iTunes integration is spread across `internal/server/` in nine files totaling ~6,000 lines: `itunes.go` (2,421 lines), `itunes_writeback_batcher.go`, `itunes_position_sync.go`, `itunes_path_reconcile.go`, `itunes_track_provisioner.go`, `itunes_transfer.go`, `playlist_itunes_sync.go`, plus tests. `server.go` has 67 iTunes references. The iTunes code reaches into the store, operation queue, scheduler, activity log, and realtime hub — every one of those couplings is implicit via the `Server` struct.

This shows up as three concrete problems:

1. **Cleanup.** `internal/server/` is a grab-bag, with iTunes being the biggest offender. Reading "how does iTunes import work" requires navigating nine files across a 70+-file directory.
2. **Testability.** Unit-testing iTunes logic requires spinning up a full `Server`. The ISP sweep narrowed most services — iTunes was deliberately left wide (hub shape) because narrowing it in-place cascades through 15+ helpers.
3. **Operational isolation.** iTunes is always-on when config is set. There's no clean "run without iTunes" mode; every subsystem that might touch iTunes has to check config flags inline.

## 2. Non-goals

- **Not changing behavior.** Import semantics, write-back batching, position sync, path reconcile — all unchanged. This is purely a boundary refactor.
- **Not reorganizing `internal/itunes/`.** The low-level layer (ITL parser, fingerprint, library watcher, path mapping, smart criteria translator) stays as one flat package. Reorganizing it into sub-packages is noted as a future improvement (scope II in the brainstorm).
- **Not designing the plugin system.** This refactor is a prerequisite for one, but plugin design is separate work (TODO 4.9).
- **Not extracting iTunes into a separate binary.** Also separate work (TODO 4.10 / the deferred "D" scope).

## 3. Design

### 3.1 Package layout (scope I)

New sub-package `internal/itunes/service/` (Go package name `itunesservice`). The low-level `internal/itunes/` package stays untouched.

```
internal/itunes/service/
  service.go            — Service struct + Deps + lifecycle (New, NewDisabled, Start, Shutdown)
  store.go              — narrow Store composite interface
  config.go             — Config struct (iTunes-specific slice of config.AppConfig)
  types.go              — request/response types (moved from internal/server/itunes.go)
  errors.go             — ErrITunesDisabled and other sentinels
  importer.go           — Importer sub-component (was executeITunesImport + helpers)
  writeback_batcher.go  — (moved from internal/server/)
  position_sync.go      — (moved from internal/server/)
  path_reconcile.go     — (moved from internal/server/)
  playlist_sync.go      — (moved from internal/server/playlist_itunes_sync.go)
  track_provisioner.go  — (moved from internal/server/)
  transfer.go           — (moved from internal/server/)
  validate.go           — Validate + TestMapping (package-level stateless fns)
  status.go             — importStatusTracker (moved from package-level state in itunes.go)
```

HTTP handlers stay in `internal/server/` but consolidate into one file:

```
internal/server/itunes_handlers.go  — thin wrappers calling s.itunesSvc.*
```

### 3.2 Service shape — top-level + sub-components (option 3)

Server gets a single field (`s.itunesSvc *itunesservice.Service`). The service internally composes seven sub-components plus lifecycle.

```go
package itunesservice

type Service struct {
    store      Store
    opQueue    operations.Queue
    activityFn func(database.ActivityEntry)
    realtime   *realtime.EventHub
    cfg        Config
    log        logger.Logger

    Importer    *Importer
    Batcher     *WriteBackBatcher
    Positions   *PositionSync
    Paths       *PathReconciler
    Playlists   *PlaylistSync
    Provisioner *TrackProvisioner
    Transfer    *TransferService
}

func New(deps Deps) (*Service, error)
func NewDisabled() *Service          // cfg.Enabled == false path
func (s *Service) Start(ctx context.Context) error   // launches batcher goroutine
func (s *Service) Shutdown(timeout time.Duration) error
```

Sub-components share the same pattern — narrow injected deps, single responsibility, isolated tests:

```go
type Importer struct {
    store      Store
    opQueue    operations.Queue
    activityFn func(database.ActivityEntry)
    cfg        Config
    log        logger.Logger
    status     *importStatusTracker
}

func (i *Importer) Execute(ctx context.Context, opID string, req ImportRequest, log logger.Logger) error
func (i *Importer) Status(bookID string) (ImportStatus, bool)
func (i *Importer) StatusBulk(bookIDs []string) map[string]ImportStatus
func (i *Importer) Resume(ctx context.Context, opID string) error
```

Stateless helpers (`Validate`, `TestMapping`) stay as package-level functions — no need to force them into a struct.

### 3.3 External surface — what Server calls

| What | Call site |
|---|---|
| Kick off an import op | `s.itunesSvc.Importer.Execute(ctx, opID, req, log)` |
| Check import status | `s.itunesSvc.Importer.Status(bookID)` / `StatusBulk(ids)` |
| Resume interrupted import | `s.itunesSvc.Importer.Resume(ctx, opID)` |
| Queue an ITL mutation | `s.itunesSvc.Batcher.Enqueue(update)` / `EnqueueRemove(pid)` |
| Sync iTunes bookmarks | `s.itunesSvc.Positions.Sync(userID)` |
| Reconcile broken paths | `s.itunesSvc.Paths.Reconcile(ctx, opID, progress)` |
| Resume path reconcile | `s.itunesSvc.Paths.Resume(ctx, opID)` |
| Push dirty smart playlists | `s.itunesSvc.Playlists.PushDirty()` |
| Import smart playlists from iTunes | `s.itunesSvc.Playlists.MigrateSmart(lib)` |
| Generate ITL track for new book | `s.itunesSvc.Provisioner.Provision(book, file)` |
| Provision all tracks for a book | `s.itunesSvc.Provisioner.ProvisionAll(book)` |
| Download current ITL | `s.itunesSvc.Transfer.Download(ctx) (io.ReadCloser, error)` |
| Upload/validate/install ITL | `s.itunesSvc.Transfer.Upload(ctx, r, install bool)` |
| List backups | `s.itunesSvc.Transfer.Backups()` |
| Restore backup | `s.itunesSvc.Transfer.Restore(ctx, filename)` |
| Validate ITL path | `itunesservice.ValidateITL(path)` (package-level) |
| Test path mapping | `itunesservice.TestMapping(req)` (package-level) |

### 3.4 Dependencies — `Deps`, `Store`, `Config`

```go
// Deps is the explicit dependency set — no globals, no Server, no config.AppConfig.
type Deps struct {
    Store      Store
    OpQueue    operations.Queue
    ActivityFn func(database.ActivityEntry)
    Realtime   *realtime.EventHub   // nil = no SSE push
    Config     Config
    Logger     logger.Logger
}

// Store is the narrow slice of database.Store the iTunes service uses. Wide
// because iTunes is a hub — books, authors, series, files, tags, external IDs,
// operations, preferences, playlists, fingerprints — but still smaller than
// full database.Store.
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

// Config is the iTunes-specific slice of config.AppConfig, passed by value
// at construction time so the service has no transitive dependency on the
// global config singleton.
type Config struct {
    Enabled            bool
    LibraryReadPath    string
    LibraryWritePath   string
    DefaultMappings    []PathMapping
    SyncInterval       time.Duration
    WriteBackInterval  time.Duration
    WriteBackMaxBatch  int
    BackupKeep         int
    ImportConcurrency  int
}

type PathMapping struct {
    From string
    To   string
}
```

Server constructs `Deps` once, at startup, unpacking `config.AppConfig`. The service never imports `config` at runtime.

### 3.5 Disabled mode (operational isolation — goal C)

`NewDisabled()` returns a `*Service` with every sub-component nil. Methods called on nil sub-components return `ErrITunesDisabled` — but the normal path is that `server.go` checks `if cfg.ITunes.Enabled` at construction time and uses the disabled path. Handlers in `internal/server/itunes_handlers.go` do not need `if svc == nil` guards; the service type itself absorbs the nil-check.

Alternatively (cleaner if it turns out nicely): `NewDisabled()` returns a `*Service` whose sub-component fields point to no-op implementations. Each no-op returns `ErrITunesDisabled`. Decided at implementation time — both variants are small.

### 3.6 Wiring change at the `Server` boundary

Before:

```go
type Server struct {
    libraryWatcher   *itunes.LibraryWatcher
    writeBackBatcher *WriteBackBatcher
    itunesActivityFn func(database.ActivityEntry)
    // ... 10+ iTunes-related fields scattered
}
```

After:

```go
type Server struct {
    itunesSvc *itunesservice.Service   // all iTunes state lives inside this
    // ... no other iTunes fields
}
```

Construction:

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
if itunesCfg.Enabled {
    svc, err := itunesservice.New(itunesservice.Deps{
        Store:      s.store,
        OpQueue:    s.opQueue,
        ActivityFn: s.activityRecorder,
        Realtime:   s.realtime,
        Config:     itunesCfg,
        Logger:     s.log.With("subsystem", "itunes"),
    })
    if err != nil { return err }
    s.itunesSvc = svc
} else {
    s.itunesSvc = itunesservice.NewDisabled()
}
```

Lifecycle hooks piggyback on the existing `Server.Start` / `Server.Shutdown`:

```go
func (s *Server) Start(ctx context.Context) error {
    // ...
    if err := s.itunesSvc.Start(ctx); err != nil { return err }
    // ...
}

func (s *Server) Shutdown(timeout time.Duration) error {
    // ...
    if err := s.itunesSvc.Shutdown(timeout); err != nil { /* log + continue */ }
    // ...
}
```

## 4. Migration strategy

Three PRs, each independently mergeable with tests green.

### 4.1 PR 1 — Foundation (~500 lines, low risk)

- Create `internal/itunes/service/` with: `service.go` (Service + Deps + Config + New/NewDisabled/Start/Shutdown skeleton), `store.go` (narrow `Store` interface), `types.go` (all request/response types moved from `internal/server/itunes.go`), `errors.go` (`ErrITunesDisabled` + others).
- Sub-component fields on `Service` are declared but nil — no behavior moved yet.
- Add `s.itunesSvc *itunesservice.Service` field to `Server`, construct it (always via `NewDisabled()` in this PR), call Start/Shutdown. Server behavior unchanged because the service is a no-op.
- Delete duplicated types from `internal/server/itunes.go`, reference new ones.
- Verification: `go build ./...`, `go vet ./...`, `go test ./internal/server/` all green.

### 4.2 PR 2 — Move sub-components (~5000 lines moved)

One component per commit inside the PR, in this order:

1. `TransferService` (moved from `internal/server/itunes_transfer.go`) — smallest, least coupled
2. `TrackProvisioner` (moved from `internal/server/itunes_track_provisioner.go`) — leaf, called by `ImportService`
3. `PositionSync` (moved from `internal/server/itunes_position_sync.go`)
4. `PathReconciler` (moved from `internal/server/itunes_path_reconcile.go`)
5. `PlaylistSync` (moved from `internal/server/playlist_itunes_sync.go`)
6. `WriteBackBatcher` (moved from `internal/server/itunes_writeback_batcher.go`) — this commit also wires `Start`/`Shutdown` for the goroutine
7. `Importer` (moved from `internal/server/itunes.go` — the big one, last)

Each commit:
- Moves the file + renames receiver to the sub-component type
- Updates imports
- Updates all call sites (handlers in `internal/server`, cross-cutting callers like `internal/server/import_service.go` → `s.itunesSvc.Provisioner.Provision(...)`)
- After: `go build ./...`, `go vet ./...`, targeted tests green before the next commit

PR 2 also flips the `Server` constructor from `NewDisabled()` to conditional `New(...)` / `NewDisabled()` based on config.

### 4.3 PR 3 — Consolidate handlers + delete old files (~500 lines)

- Create `internal/server/itunes_handlers.go` containing all `handleITunes*` functions as thin wrappers calling `s.itunesSvc.*`.
- Delete: `internal/server/itunes.go`, `itunes_writeback_batcher.go`, `itunes_position_sync.go`, `itunes_path_reconcile.go`, `itunes_track_provisioner.go`, `itunes_transfer.go`, `playlist_itunes_sync.go`, `compute_itunes_path*.go` (any remaining helpers).
- `server.go` iTunes reference count drops from 67 to ≤ 15 (just wiring + routes block).
- Final verification: `go test ./...` full suite green.

## 5. Testing

**Existing tests are the primary safety net.** The following test files move alongside their production code (same relative path, new package) and should keep passing unchanged:

- `internal/server/itunes_import_integration_test.go` → `internal/itunes/service/import_integration_test.go`
- `internal/server/itunes_integration_test.go` → `internal/itunes/service/integration_test.go`
- `internal/server/itunes_writeback_batcher_test.go` → `internal/itunes/service/writeback_batcher_test.go`
- `internal/server/itunes_position_sync_test.go` → `internal/itunes/service/position_sync_test.go`
- `internal/server/itunes_transfer_test.go` → `internal/itunes/service/transfer_test.go`
- `internal/server/playlist_itunes_sync_test.go` → `internal/itunes/service/playlist_sync_test.go`
- `internal/server/itunes_error_test.go` stays in `internal/server/` (tests handler error shapes, stays at the HTTP layer)

Any test failure during PR 2 or 3 means the refactor changed behavior — investigate before proceeding.

**New per-component unit tests** grow organically. Once `TrackProvisioner` is its own struct with narrow deps, writing a unit test with mocked `AuthorReader` + `BookFileStore` + `ExternalIDStore` is trivial — no full `Server` construction, no integration-test setup. We don't need to write them all up front; the point is they're now *possible*.

**One new test is required**: the disabled-mode smoke test. In `internal/server/itunes_handlers_test.go`:

```go
func TestITunesDisabled_ReturnsServiceUnavailable(t *testing.T) {
    srv := newTestServerWithITunesDisabled(t)
    w := httptest.NewRecorder()
    req := httptest.NewRequest("POST", "/api/v1/itunes/import", ...)
    srv.Router().ServeHTTP(w, req)
    require.Equal(t, http.StatusServiceUnavailable, w.Code)
    require.Contains(t, w.Body.String(), "iTunes integration is disabled")
}
```

Proves the C goal end-to-end.

## 6. Risks and mitigations

| Risk | Mitigation |
|---|---|
| Cyclic imports — `service` depends on `database`, `operations`, `realtime`; `server` imports `service`. If any of those depend back on `server` it breaks. | Import graph already verified during design — all OK. If one appears during implementation, move the offending type to a lower-level package or to `internal/common/`. |
| Hidden global state — `itunesImportStatus` is currently a package-level map. | Move to `importStatusTracker` field on `Importer`. Matches the "one instance per test" pattern the audiobook_service_prop_test established. |
| `config.AppConfig` reads sprinkled through iTunes code. | All replaced with `s.cfg` (the `Config` value). Grep verifies: `grep "config.AppConfig" internal/itunes/service/` → zero hits after PR 2. |
| Operation resume (`resumeInterruptedOperations` switch on op type) — depends on knowing iTunes op types. | Stays in `server.go`; dispatches to `s.itunesSvc.Importer.Resume(...)` and `s.itunesSvc.Paths.Resume(...)`. No behavior change, just one level of indirection. |
| Scheduled task registration (unified task scheduler) — closes over iTunes-specific logic. | Stays in `server.go`; scheduled functions close over `s.itunesSvc` rather than iTunes internals. |
| Long-lived batcher goroutine — needs clean startup/shutdown. | `Service.Start(ctx)` launches it via `Batcher.Start(ctx)`. `Service.Shutdown(timeout)` calls `Batcher.Shutdown(timeout)` which flushes the queue. Matches the pattern scanner + index worker already use. |
| Test relocation could silently drop tests. | `find internal/itunes/service/ -name "*_test.go"` count before/after PR 2 to confirm parity. |
| Hidden cross-handler state (e.g., import status map read by one handler, written by another) | The status tracker is now owned by `Importer`; handlers that read it call `s.itunesSvc.Importer.Status(...)` — single ownership, no race via handler-ordering. |

## 7. Success criteria

- `grep -nE "itunes\|iTunes\|ITL" internal/server/server.go \| wc -l` drops from 67 → ≤ 15
- `wc -l internal/server/itunes*.go internal/server/playlist_itunes_sync.go` drops from ~6060 → ≤ 800
- New package `internal/itunes/service/` ≈ 5000–5500 lines
- `go test ./...` full suite green with zero test-code changes beyond path renames
- Disabled-mode smoke test passes
- `config.AppConfig` reads zero inside the service package
- Running with `ITunesEnabled=false` in config: server starts cleanly; iTunes endpoints return 503 with a clear error; no iTunes goroutines started; no iTunes log spam

## 8. Future evolution

This refactor is deliberately a prerequisite for two follow-on items:

### 8.1 Plugin system (TODO 4.9) — sync targets + download clients

The `Service` struct designed here is the candidate shape for a `SyncTarget` plugin interface. When we do plugin work:

- Extract public methods on `Service` into a `plugin.SyncTarget` interface
- `itunesservice.Service` becomes `var _ plugin.SyncTarget = (*Service)(nil)` — the first concrete implementation
- A future Plex integration implements the same interface as a second implementation
- Server wires `[]plugin.SyncTarget` instead of specific fields

The same pattern applies to download clients (Deluge is current; Transmission / qBittorrent / rtorrent / NNTP are plausible additions). Each download client would implement a `plugin.DownloadClient` interface extracted from today's `internal/deluge` package shape.

This refactor doesn't design the plugin system — it puts iTunes and Deluge into the per-subsystem directory shape that makes plugin extraction cheap later.

### 8.2 Separate binary / microservice (TODO 4.10) — scope D from brainstorm

Once iTunes is a clean sub-package with an explicit `Deps` surface, extracting it into a separate binary is a `go build ./cmd/itunes-worker/` problem rather than a refactoring problem. The service communicates with the main server via the same interface it uses today — just over gRPC or HTTP instead of in-process function calls. The injected `Store` becomes a remote client of the main database; the `ActivityFn` / `Realtime` / `OpQueue` surfaces become RPC stubs.

Not designed here. Noted so future readers understand the intent.

### 8.3 Reorganize `internal/itunes/` low-level package (scope II from brainstorm)

The low-level ITL parser / writer / fingerprint / watcher / path / smart-criteria code is currently ~11,600 lines in one flat directory. Sub-packaging it (`itunes/itl/`, `itunes/watcher/`, `itunes/path/`, `itunes/smart/`, `itunes/xml/`) would make the structure match the concerns. Not blocking today's work but a reasonable follow-on when a motivated reason arises (adding a new ITL format version, new watcher backend, etc.).
