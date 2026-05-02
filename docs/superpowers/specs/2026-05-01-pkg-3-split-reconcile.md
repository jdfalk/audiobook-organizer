<!-- file: docs/superpowers/specs/2026-05-01-pkg-3-split-reconcile.md -->
<!-- version: 1.1.0 -->
<!-- guid: a1b2c3d4-e5f6-7890-abcd-ef1234560003 -->
<!-- last-edited: 2026-05-01 -->

# PKG-3: Split `reconcile.go` — Extract Pure Logic to `internal/reconcile/`

**TODO ID:** PKG-3  
**Effort:** Medium  
**Impact:** Medium — 1317-line file currently mixes HTTP handlers with pure reconcile logic  
**Companion bot-task:** [`docs/superpowers/bot-tasks/2026-05-01-pkg-3-reconcile-split.md`](../bot-tasks/2026-05-01-pkg-3-reconcile-split.md)

---

## Problem

`internal/server/reconcile.go` (1317 lines) has **two distinct concerns** interleaved:

**HTTP handlers** (gin-coupled, ~220 lines):
- `reconcilePreview` (line 84)
- `startReconcileScan` (line 100)
- `latestReconcileScan` (line 153)
- `startReconcile` (line 194)
- `cleanupDuplicateVersionGroupsHandler` (line 774)
- `markBrokenSegmentBooksHandler` (line 876)
- `mergeNoVGDuplicatesHandler` (line 1234)
- `assignOrphanVGsHandler` (line 1310)

**Pure business logic** (~1100 lines):
- `runReconcileScan` (line 133)
- `buildReconcilePreview` / `buildReconcilePreviewWithProgress` (lines 239, 245)
- `findUntrackedFiles` (line 487)
- `executeReconcile` (line 571)
- `normalizeFilename` / `countMatchType` (lines 643, 658)
- `cleanupDuplicateVersionGroups` (line 679)
- `findBrokenSegmentBooks` (line 807)
- `mergeNoVGDuplicates` (line 912)
- `mergeBookMetadata` (line 1108)
- `assignOrphanVGs` (line 1259)

All types (`ReconcileMatch`, `ReconcilePreviewResult`, `ReconcileBrokenRecord`, etc.)
are used by both layers. The pure-logic functions do DB work and filesystem work but have
no gin/http dependency.

---

## Solution

Create `internal/reconcile/` for all types and pure-logic functions. The HTTP handlers
remain in `internal/server/reconcile.go` (now much smaller, ~300 lines) and call into
the new package.

---

## No Import Cycles

`internal/config`, `internal/database`, `internal/logger`, `internal/operations`,
`internal/scanner` — none import `internal/server`. No cycles introduced.

---

## Package Structure After

```
internal/reconcile/
    reconcile.go    ← all types + pure-logic functions from reconcile.go

internal/server/
    reconcile.go    ← HTTP handlers only; calls internal/reconcile functions
```

---

## Interface Contract

```go
// internal/reconcile/reconcile.go
type Store interface {
    database.BookStore
    database.BookFileStore
    database.ImportPathStore
    database.OperationStore
}
```

(Previously `reconcileStore` in `internal/server/reconcile.go`, lines 31–36.)

---

## Types Moving to `internal/reconcile/`

All struct types currently defined in `reconcile.go`:
- `ReconcileMatch`
- `ReconcilePreviewResult`
- `ReconcileBrokenRecord`
- `ReconcileApplyRequest`
- `ReconcileApplyItem`
- `ReconcileApplyResult`
- `VersionGroupCleanupResult`
- `BrokenSegmentResult`
- `BrokenSegmentEntry`
- `MergeDuplicatesResult`
- `MergeDuplicateEntry`
- `AssignVGResult`

---

## Functions Moving to `internal/reconcile/`

All pure-logic functions (no `*gin.Context` parameter):
- `runReconcileScan`
- `buildReconcilePreview`
- `buildReconcilePreviewWithProgress`
- `findUntrackedFiles`
- `executeReconcile`
- `normalizeFilename`
- `countMatchType`
- `cleanupDuplicateVersionGroups`
- `findBrokenSegmentBooks`
- `mergeNoVGDuplicates`
- `mergeBookMetadata`
- `assignOrphanVGs`

These functions currently have `*Server` as a receiver or parameter. After extraction,
they will take a `reconcile.Store` interface instead.

**Critical:** Any of these functions that currently access `s.someField` (where `s` is
`*Server`) must have those accesses replaced with explicit parameters passed in.
The most likely case is `s.store` → pass `store reconcile.Store` explicitly.
Use `grep -n "s\." internal/server/reconcile.go` to find all server field accesses in
pure-logic functions before moving.

---

## HTTP Handlers Remaining in `internal/server/reconcile.go`

After extraction, the server-side handlers become thin wrappers:

```go
// Example after extraction — handlers now use httputil package (not server-local helpers)
func (s *Server) reconcilePreview(c *gin.Context) {
    result, err := reconcile.BuildReconcilePreview(c.Request.Context(), s.store, ...)
    if err != nil {
        httputil.RespondWithInternalError(c, err.Error())
        return
    }
    httputil.RespondWithOK(c, result)
}
```

> **Note:** As of 2026-05-01 the httputil migration is complete. Handlers in reconcile.go
> already call `httputil.RespondWith*` and `httputil.InternalError`. The pure-logic
> extraction does NOT touch handler code — keep those httputil calls as-is.

---

## Step-by-Step Implementation

### Step 1 — Audit server field accesses in pure-logic functions

Run:
```bash
grep -n "s\.\|p\.server\." internal/server/reconcile.go
```

For each access, note: which field is accessed, and what its type is. These become
explicit parameters in the extracted functions.

### Step 2 — Create `internal/reconcile/` package

```bash
mkdir -p internal/reconcile
```

Create `internal/reconcile/reconcile.go` with:
- File header
- `package reconcile`
- All imports needed by the pure-logic functions
- The `Store` interface (renamed from `reconcileStore`)
- All type definitions
- All pure-logic functions (as free functions or on a `Service` struct, not on `*Server`)

### Step 3 — Update function signatures

For every pure-logic function moved to `internal/reconcile/`, change:
- Remove `*Server` receiver; if the function needs a store, it takes `store Store` as first param
- Any other `*Server` fields accessed become explicit params

### Step 4 — Shrink `internal/server/reconcile.go`

Replace the moved functions with calls to `reconcile.XYZ(...)`.
Add import `"github.com/jdfalk/audiobook-organizer/internal/reconcile"`.
Keep only the 8 handler functions.

### Step 5 — Build and verify

```bash
go build ./...
go vet ./...
go test ./internal/reconcile/... ./internal/server/...
```

---

## Risk

**Medium** — The pure-logic functions may access multiple `*Server` fields beyond
just `s.store`. Each one needs to be converted to an explicit parameter. A thorough
`grep -n "s\."` audit in Step 1 is critical before writing any code.

---

## Rollback

`git checkout internal/server/reconcile.go` restores original. Delete
`internal/reconcile/`.
