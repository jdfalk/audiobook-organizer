<!-- file: docs/superpowers/bot-tasks/2026-05-01-pkg-3-reconcile-split.md -->
<!-- version: 1.1.0 -->
<!-- guid: b2c3d4e5-f6a7-8901-bcde-f01234560003 -->
<!-- last-edited: 2026-05-01 -->

# BOT TASK: Split `reconcile.go` — Extract Pure Logic

**TODO ID:** PKG-3  
**Audience:** burndown bot  
**Design spec:** [`docs/superpowers/specs/2026-05-01-pkg-3-split-reconcile.md`](../specs/2026-05-01-pkg-3-split-reconcile.md)

## Prerequisites

- PKG-3 spec read and understood
- Branch `refactor/pkg-3-reconcile` created from latest main
- Work in a git worktree at `/Users/jdfalk/.worktrees/pkg-3-reconcile`

## Branch

```
refactor/pkg-3-reconcile
```

## Step 1 — Audit server field accesses

Run this and save the output before touching any code:

```bash
grep -n "s\." internal/server/reconcile.go | grep -v "//\|\"s\.\|s\.store" | head -60
```

For every `s.someField` access inside a **pure-logic function** (not a handler),
note the field name and type. These will need to become explicit parameters after
extraction. Common cases: `s.store`, `s.config`, `s.logger`.

## Step 2 — Create `internal/reconcile/` package

```bash
mkdir -p internal/reconcile
touch internal/reconcile/reconcile.go
```

Start `internal/reconcile/reconcile.go` with:

```go
// file: internal/reconcile/reconcile.go
// version: 1.0.0
// guid: <generate-new-uuid>
// last-edited: 2026-05-01

package reconcile

import (
    // (fill in imports as you move code)
)

// Store is the database dependency for reconcile operations.
type Store interface {
    database.BookStore
    database.BookFileStore
    database.ImportPathStore
    database.OperationStore
}
```

## Step 3 — Move all struct types

From `internal/server/reconcile.go`, move these type definitions (lines noted are
approximate — find them by name) into `internal/reconcile/reconcile.go`:

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

Delete them from `internal/server/reconcile.go`.

## Step 4 — Move pure-logic functions one at a time

For EACH function in this list (move in order, building after each):

1. `runReconcileScan`
2. `buildReconcilePreview`
3. `buildReconcilePreviewWithProgress`
4. `findUntrackedFiles`
5. `executeReconcile`
6. `normalizeFilename`
7. `countMatchType`
8. `cleanupDuplicateVersionGroups`
9. `findBrokenSegmentBooks`
10. `mergeNoVGDuplicates`
11. `mergeBookMetadata`
12. `assignOrphanVGs`

For each function:
- Cut from `internal/server/reconcile.go`
- Paste into `internal/reconcile/reconcile.go`
- If it had a `*Server` receiver (`func (s *Server) foo(...)`), change to a free function with `store Store` as the first parameter
- Replace any `s.store` accesses with the `store` parameter
- Replace any other `s.someField` accesses by adding that field as an explicit parameter
- Run `go build ./...` after each function move — fix errors before next function

## Step 5 — Update server-side handlers

In `internal/server/reconcile.go`, for each handler function:
1. Add import `"github.com/jdfalk/audiobook-organizer/internal/reconcile"`
2. Change any direct type references (e.g. `ReconcileMatch`) to `reconcile.ReconcileMatch`
3. Change function calls to the moved functions: `s.runReconcileScan(...)` → `reconcile.RunReconcileScan(s.store, ...)`
4. Also rename the `reconcileStore` interface — it should be removed (or kept for local use, since the handlers will just pass `s.store` directly)
5. **DO NOT touch `httputil.*` calls** — handlers already use `httputil.RespondWith*`
   and `httputil.InternalError`. Leave those as-is; they are already correct.

## Step 6 — Add required imports to `internal/reconcile/reconcile.go`

Based on what the moved functions use, add all needed imports:
```
"context"
"encoding/json"
"fmt"
"os"
"path/filepath"
"strings"
"time"
"github.com/jdfalk/audiobook-organizer/internal/config"
"github.com/jdfalk/audiobook-organizer/internal/database"
"github.com/jdfalk/audiobook-organizer/internal/logger"
"github.com/jdfalk/audiobook-organizer/internal/operations"
"github.com/jdfalk/audiobook-organizer/internal/scanner"
"github.com/oklog/ulid/v2"
```

## Step 7 — Final build and test

```bash
go build ./...
go vet ./...
go test ./internal/reconcile/... ./internal/server/...
```

All must pass.

## Commit Message

```
refactor(server): extract reconcile business logic to internal/reconcile/

Split reconcile.go (1317 lines) into:
- internal/reconcile/reconcile.go — all types + pure-logic functions
  (runReconcileScan, buildReconcilePreview, executeReconcile, etc.)
- internal/server/reconcile.go — HTTP handlers only (~300 lines)

reconcileStore renamed to reconcile.Store and exported. Pure-logic functions
converted from *Server receivers to free functions with explicit store param.

Refs: PKG-3
```
