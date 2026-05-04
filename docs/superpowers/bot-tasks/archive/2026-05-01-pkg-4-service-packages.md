<!-- file: docs/superpowers/bot-tasks/2026-05-01-pkg-4-service-packages.md -->
<!-- version: 1.1.0 -->
<!-- guid: b2c3d4e5-f6a7-8901-bcde-f01234560004 -->
<!-- last-edited: 2026-05-01 -->

# BOT TASK: Extract Remaining Service Packages (PKG-4)

**TODO ID:** PKG-4  
**Audience:** burndown bot  
**Design spec:** [`docs/superpowers/specs/2026-05-01-pkg-4-extract-service-packages.md`](../specs/2026-05-01-pkg-4-extract-service-packages.md)

## Prerequisites

- PKG-4 spec read and understood
- Each sub-task (4a–4e) should be done on its own branch
- Work in separate git worktrees per sub-task

---

## PKG-4a: `scan_service.go` → `internal/scanner/`

**Branch:** `refactor/pkg-4a-scan-service`

### Step 1 — Audit

```bash
grep -n "s\.\|server\." internal/server/scan_service.go | grep -v "//"
grep -rn "type.*Service\|func New" internal/scanner/*.go | grep -v _test
```

Note any `*Server` field accesses and any naming conflicts.

### Step 2 — Move

```bash
cp internal/server/scan_service.go internal/scanner/service.go
```

In `internal/scanner/service.go`:
- `package server` → `package scanner`
- Resolve any naming conflicts (rename if needed)
- Replace `*Server` receiver / field accesses with explicit params
- Update file header
- Bump version, update last-edited

```bash
rm internal/server/scan_service.go
```

### Step 3 — Update server references

```bash
grep -rn "ScanService\|NewScanService" internal/server/*.go | grep -v _test
```

Add `"github.com/jdfalk/audiobook-organizer/internal/scanner"` import where needed.
Update type references and constructor calls.

### Step 4 — Build

```bash
go build ./... && go vet ./...
```

---

## PKG-4b: Import services → `internal/importer/`

**Branch:** `refactor/pkg-4b-importer`

### Step 1 — Audit

```bash
grep -n "s\.\|server\." internal/server/import_service.go internal/server/import_path_service.go | grep -v "//"
```

### Step 2 — Create and move

```bash
mkdir -p internal/importer
cp internal/server/import_service.go internal/importer/service.go
cp internal/server/import_path_service.go internal/importer/path_service.go
```

In both files:
- `package server` → `package importer`
- Replace `*Server` field accesses with explicit params
- Update file headers

```bash
rm internal/server/import_service.go internal/server/import_path_service.go
```

### Step 3 — Update server references

```bash
grep -rn "ImportService\|ImportPathService\|NewImport" internal/server/*.go | grep -v _test
```

Add import, update type references and constructor calls.

### Step 4 — Build

```bash
go build ./... && go vet ./...
```

---

## PKG-4c: `quarantine_service.go` → `internal/quarantine/`

**Branch:** `refactor/pkg-4c-quarantine`

### Step 1 — Add EventPublisher interface to plugin package

Open `internal/plugin/events.go`. Check if `EventPublisher` interface already exists.
If not, add after the `EventBus` struct:

```go
// EventPublisher is the narrow interface for publishing lifecycle events.
type EventPublisher interface {
    Publish(ctx context.Context, event Event)
}
```

Build: `go build ./internal/plugin/...` must pass.

### Step 2 — Create destination package

```bash
mkdir -p internal/quarantine
```

### Step 3 — Move and refactor

```bash
cp internal/server/quarantine_service.go internal/quarantine/service.go
```

In `internal/quarantine/service.go`:
1. `package server` → `package quarantine`
2. Add a `Store` interface (narrow slice of database methods the service needs):
   ```go
   type Store interface {
       // (copy the specific db method signatures called in the file)
   }
   ```
3. Add a `QuarantineService` struct:
   ```go
   type QuarantineService struct {
       store  Store
       cfg    *config.Config
       events plugin.EventPublisher
   }
   func NewQuarantineService(store Store, cfg *config.Config, events plugin.EventPublisher) *QuarantineService {
       return &QuarantineService{store: store, cfg: cfg, events: events}
   }
   ```
4. Change all `func (s *Server) QuarantineBook(...)` → `func (qs *QuarantineService) QuarantineBook(...)`
5. Replace `s.Store()` → `qs.store`
6. Replace `s.publishEvent(ctx, ...)` → `qs.events.Publish(ctx, ...)`
7. Replace `s.Config` or `s.cfg` → `qs.cfg`
8. Update file header (path, bump patch version, update last-edited)

```bash
rm internal/server/quarantine_service.go
```

### Step 4 — Update server references

```bash
grep -rn "QuarantineBook\|UnquarantineBook\|autoQuarantine\|processITunes\|QuarantineService" internal/server/*.go | grep -v _test
```

For each match:
1. Add import `"github.com/jdfalk/audiobook-organizer/internal/quarantine"`
2. Add `quarantine *quarantine.QuarantineService` field to the `Server` struct (in server.go or similar)
3. Initialize in the server constructor: `quarantine.NewQuarantineService(store, cfg, s.eventBus)`
4. Update method calls: `s.QuarantineBook(...)` → `s.quarantine.QuarantineBook(...)`

### Step 5 — Build

```bash
go build ./... && go vet ./...
```

---

## PKG-4d: Writeback files → `internal/writeback/`

**Branch:** `refactor/pkg-4d-writeback`

### Step 1 — Audit

```bash
grep -n "s\.\|server\." internal/server/writeback_enqueuer.go internal/server/writeback_outbox.go | grep -v "//"
```

### Step 2 — Create and move

```bash
mkdir -p internal/writeback
cp internal/server/writeback_enqueuer.go internal/writeback/enqueuer.go
cp internal/server/writeback_outbox.go internal/writeback/outbox.go
```

In both files:
- `package server` → `package writeback`
- Replace `*Server` field accesses with explicit params
- Update file headers

```bash
rm internal/server/writeback_enqueuer.go internal/server/writeback_outbox.go
```

### Step 3 — Update server references

```bash
grep -rn "Enqueuer\|Outbox\|writeback\." internal/server/*.go | grep -v _test
```

### Step 4 — Build

```bash
go build ./... && go vet ./...
```

---

## PKG-4e: Move to existing packages

**Branch:** `refactor/pkg-4e-existing-pkgs`

### filesystem_service.go → internal/fileops/

```bash
# Check for conflicts
grep -rn "type\|func New" internal/fileops/*.go | grep -v _test

cp internal/server/filesystem_service.go internal/fileops/service.go
# Change package, fix server refs, update header
rm internal/server/filesystem_service.go
grep -rn "FilesystemService\|NewFilesystem" internal/server/*.go | grep -v _test
# Update refs, add import
go build ./... && go vet ./...
```

### system_service.go → internal/sysinfo/

```bash
# Check for conflicts
grep -rn "type\|func New" internal/sysinfo/*.go | grep -v _test

cp internal/server/system_service.go internal/sysinfo/service.go
# Change package, fix server refs, update header
rm internal/server/system_service.go
grep -rn "SystemService\|NewSystem" internal/server/*.go | grep -v _test
# Update refs, add import
go build ./... && go vet ./...
```

---

## Commit Message Template

Use one commit per sub-task (4a–4e):

```
refactor(server): extract <name> service to internal/<pkg>/

Move <file(s)> from internal/server/ to internal/<pkg>/. Replace *Server
receiver/field accesses with explicit constructor parameters. Update all
call sites in internal/server/ to use <pkg>.<Type>.

Refs: PKG-4<letter>
```
