<!-- file: docs/superpowers/specs/2026-05-01-pkg-4-extract-service-packages.md -->
<!-- version: 1.1.0 -->
<!-- guid: a1b2c3d4-e5f6-7890-abcd-ef1234560004 -->
<!-- last-edited: 2026-05-01 -->

# PKG-4: Extract Remaining Gin-Free Service Packages

**TODO ID:** PKG-4  
**Effort:** Large (multiple independent sub-tasks)  
**Impact:** Medium — further reduces `internal/server/` bloat  
**Companion bot-task:** [`docs/superpowers/bot-tasks/2026-05-01-pkg-4-service-packages.md`](../bot-tasks/2026-05-01-pkg-4-service-packages.md)

---

## Problem

Beyond the files covered by PKG-1 through PKG-3, there are additional gin-free service
files in `internal/server/` that belong in focused packages. All were verified
gin-free by `grep -rL 'gin\|"net/http"'`.

| File | Lines | Proposed Destination |
|------|-------|---------------------|
| `scan_service.go` | 405 | `internal/scanner/` (package already exists) |
| `import_service.go` | 202 | `internal/importer/` (new) |
| `import_path_service.go` | 60 | `internal/importer/` (new) |
| `quarantine_service.go` | 272 | `internal/quarantine/` (new) |
| `writeback_enqueuer.go` | ? | `internal/writeback/` (new) |
| `writeback_outbox.go` | ? | `internal/writeback/` (new) |
| `filesystem_service.go` | 198 | `internal/fileops/` (package already exists) |
| `system_service.go` | 319 | `internal/sysinfo/` (package already exists) |

**Files that should STAY in `internal/server/`** (too coupled to HTTP/server lifecycle):
- `batch_service.go` — orchestrates HTTP batch request handling
- `work_service.go` — tied to server work queue
- `config_update_service.go` — modifies server-owned config state
- `dashboard_service.go` — presentation/aggregation for HTTP response
- `metadata_state_service.go` — tightly coupled to server's metadata pipeline state
- `indexed_store.go` — server-owned Bleve index wrapper

---

## Sub-Tasks

This spec is split into 4 independent sub-tasks (PKG-4a through PKG-4d) that can be
done in any order since they touch different destination packages.

---

## PKG-4a: Move `scan_service.go` → `internal/scanner/`

**Pre-check:** `grep -n "s\.\|server\." internal/server/scan_service.go` — identify
any `*Server` field accesses. If present, they must become explicit params first.

**Steps:**
1. Copy `internal/server/scan_service.go` → `internal/scanner/service.go`
2. Change `package server` → `package scanner`
3. Check for naming conflicts with existing `internal/scanner/` files
   (`grep -rn "type.*Service\|func New" internal/scanner/`)
4. Rename conflicting types if needed (e.g. `ScanService` → rename if already used)
5. Delete original from `internal/server/`
6. Update all references in `internal/server/` to use `scanner.XYZ`
7. `go build ./...` and `go vet ./...`

---

## PKG-4b: Create `internal/importer/` from import services

**Files:** `import_service.go` + `import_path_service.go`

**Steps:**
1. `mkdir -p internal/importer`
2. Copy `internal/server/import_service.go` → `internal/importer/service.go`
3. Copy `internal/server/import_path_service.go` → `internal/importer/path_service.go`
4. Change both to `package importer`
5. Check for `*Server` field accesses; replace with explicit params
6. Delete originals from `internal/server/`
7. Update references in `internal/server/` to `importer.XYZ`
8. `go build ./...` and `go vet ./...`

---

## PKG-4c: Create `internal/quarantine/` from quarantine service

**File:** `quarantine_service.go`

**⚠️ Coupling warning:** `quarantine_service.go` calls `s.publishEvent(ctx, plugin.Event)` and
`s.Store()`, and accesses `s.writeBackBatcher`. These are `*Server` fields. Before
extraction, the service must accept a `plugin.EventPublisher` interface:

```go
// Add to internal/plugin/events.go (or internal/quarantine/service.go):
type EventPublisher interface {
    Publish(ctx context.Context, event plugin.Event)
}
```

Then change `QuarantineService` constructor to accept `EventPublisher` instead of `*Server`.
The `*plugin.EventBus` concrete type already implements `Publish(ctx, event)`, so no
other changes are needed at call sites.

**Steps:**
1. Add `EventPublisher` interface to `internal/plugin/events.go` (if not already present)
2. `mkdir -p internal/quarantine`
3. Copy `internal/server/quarantine_service.go` → `internal/quarantine/service.go`
4. Change to `package quarantine`
5. Replace `s *Server` receiver with `qs *QuarantineService` and a `QuarantineService` struct:
   ```go
   type QuarantineService struct {
       store  Store
       cfg    *config.Config
       events plugin.EventPublisher
   }
   ```
6. Replace `s.Store()` with `qs.store`, `s.publishEvent(...)` with `qs.events.Publish(...)`
7. Delete original from `internal/server/`
8. Update references in `internal/server/` to `quarantine.XYZ`, passing `s.eventBus`
9. `go build ./...` and `go vet ./...`

---

## PKG-4d: Create `internal/writeback/` from writeback files

**Files:** `writeback_enqueuer.go` + `writeback_outbox.go`

**Pre-check:** These files likely interact with the server's outbox/queue machinery.
Run `grep -n "s\.\|server\." internal/server/writeback_*.go` before moving.

**Steps:**
1. `mkdir -p internal/writeback`
2. Copy `writeback_enqueuer.go` → `internal/writeback/enqueuer.go`
3. Copy `writeback_outbox.go` → `internal/writeback/outbox.go`
4. Change both to `package writeback`
5. Replace any `*Server` field accesses with explicit constructor params
6. Delete originals from `internal/server/`
7. Update references in `internal/server/` to `writeback.XYZ`
8. `go build ./...` and `go vet ./...`

---

## PKG-4e: Move to existing packages

**`filesystem_service.go` → `internal/fileops/`**
- Check for name conflicts: `grep -rn "type\|func New" internal/fileops/`
- Copy, change package, delete original, update server references

**`system_service.go` → `internal/sysinfo/`**
- Check for name conflicts: `grep -rn "type\|func New" internal/sysinfo/`
- Copy, change package, delete original, update server references

---

## Import Cycle Pre-checks

Before moving each file, verify the destination package doesn't already import
`internal/server` (which would create a cycle):

```bash
grep -r '"github.com/jdfalk/audiobook-organizer/internal/server"' \
    internal/scanner/ internal/fileops/ internal/sysinfo/ 2>/dev/null
```

If any results appear, that group must be skipped or the cycle resolved first.

---

## Order of Operations

PKG-4a through PKG-4e are **independent** and can be done in any order or in parallel
(separate branches). Suggested order based on confidence/risk:

1. PKG-4e (filesystem/sysinfo → existing packages) — lowest risk
2. PKG-4b (importer) — small files, clear scope
3. PKG-4c (quarantine) — single file
4. PKG-4a (scanner) — existing package may have conflicts
5. PKG-4d (writeback) — most complex internal machinery

---

## Rollback

Each sub-task is independent. `git checkout internal/server/<filename>` restores
individual files. Delete the new destination package directory.
