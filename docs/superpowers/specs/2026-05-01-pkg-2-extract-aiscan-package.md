<!-- file: docs/superpowers/specs/2026-05-01-pkg-2-extract-aiscan-package.md -->
<!-- version: 1.0.0 -->
<!-- guid: a1b2c3d4-e5f6-7890-abcd-ef1234560002 -->
<!-- last-edited: 2026-05-01 -->

# PKG-2: Extract `internal/aiscan/` AI Scan Pipeline Package

**TODO ID:** PKG-2  
**Effort:** Medium  
**Impact:** High — 928 lines of pure orchestration logic decoupled from HTTP layer  
**Companion bot-task:** [`docs/superpowers/bot-tasks/2026-05-01-pkg-2-aiscan-pipeline.md`](../bot-tasks/2026-05-01-pkg-2-aiscan-pipeline.md)

---

## Problem

`internal/server/ai_scan_pipeline.go` (928 lines) implements the AI author dedup scan
pipeline: start/cancel scans, phase transitions, batch/realtime processing, enrichment,
cross-validation, and progress updates.

It has **zero gin/http imports** and its `aiScanPipelineStore` interface is already
defined as a narrow composite. However, it lives in `internal/server/` and its
`PipelineManager` struct stores a `*Server` field that is **never accessed** after
construction (confirmed by grep — `pm.server` has zero call sites in the file).

The stale `*Server` field is the only thing preventing a clean extraction.

---

## Evidence

```go
// ai_scan_pipeline.go lines 21–40
type aiScanPipelineStore interface {
    database.AuthorReader
    database.OperationStore
}

type PipelineManager struct {
    // ...
    server *Server   // ← stored at line 33, NEVER read after construction
}

func NewPipelineManager(store aiScanPipelineStore, ..., server *Server) *PipelineManager {
    return &PipelineManager{
        // ...
        server: server,   // ← assigned but never used
    }
}
```

`grep -n "pm\.server\|\.server\." internal/server/ai_scan_pipeline.go` returns no
results beyond the struct field declaration and constructor assignment.

---

## Solution

1. Remove the `server *Server` field from `PipelineManager` and the corresponding
   parameter from `NewPipelineManager`.
2. Move `ai_scan_pipeline.go` to `internal/aiscan/pipeline.go`.
3. Change package declaration from `server` to `aiscan`.
4. Update the one call site in `internal/server/` that constructs `NewPipelineManager`.

---

## No Import Cycles

`internal/ai`, `internal/database`, `internal/dedup` — none import `internal/server`.
No cycles introduced.

---

## Package Structure After

```
internal/aiscan/
    pipeline.go     ← ai_scan_pipeline.go (renamed, *Server removed)
```

---

## Interface Contract

```go
// aiscan/pipeline.go
type Store interface {
    database.AuthorReader
    database.OperationStore
}
```

The `aiScanPipelineStore` type is renamed to `Store` and exported (since it is now
the package's public dependency contract).

---

## Constructor Change

Before:
```go
// internal/server/ai_scan_pipeline.go
func NewPipelineManager(store aiScanPipelineStore, parser *ai.OpenAIParser,
    queue operations.Queue, server *Server) *PipelineManager
```

After:
```go
// internal/aiscan/pipeline.go
func NewPipelineManager(store Store, parser *ai.OpenAIParser,
    queue operations.Queue) *PipelineManager
```

The call site in `internal/server/` (wherever `NewPipelineManager` is called) drops
the `server` argument.

---

## Step-by-Step Implementation

### Step 1 — Remove dead field

In `internal/server/ai_scan_pipeline.go`, before moving:
1. Delete `server *Server` from the `PipelineManager` struct
2. Delete `server *Server` parameter from `NewPipelineManager`
3. Delete `server: server,` from the constructor body
4. Run `go build ./...` — must pass before moving

### Step 2 — Create package directory

```bash
mkdir -p internal/aiscan
```

### Step 3 — Move and rename

```
internal/server/ai_scan_pipeline.go → internal/aiscan/pipeline.go
```

Change `package server` → `package aiscan`.  
Rename `aiScanPipelineStore` → `Store` and export it.  
Update file header (path, version bump, last-edited).  
Delete original from `internal/server/`.

### Step 4 — Update call site in `internal/server/`

Find where `NewPipelineManager` is called (likely `server.go` or a setup file).
Remove the `server` argument. Add import
`"github.com/jdfalk/audiobook-organizer/internal/aiscan"` and change any type
references from `*PipelineManager` to `*aiscan.PipelineManager`.

### Step 5 — Build and verify

```bash
go build ./...
go vet ./...
go test ./internal/aiscan/... ./internal/server/...
```

---

## Rollback

Delete `internal/aiscan/`, restore `internal/server/ai_scan_pipeline.go` from git.

---

## Future

Once extracted, `PipelineManager` can be unit-tested with a mock `Store` without
any server setup. This enables tests for phase transitions, cancellation, and
progress tracking that are currently impossible to write cleanly.
