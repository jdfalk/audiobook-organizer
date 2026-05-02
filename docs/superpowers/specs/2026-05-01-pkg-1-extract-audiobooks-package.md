<!-- file: docs/superpowers/specs/2026-05-01-pkg-1-extract-audiobooks-package.md -->
<!-- version: 1.0.0 -->
<!-- guid: a1b2c3d4-e5f6-7890-abcd-ef1234560001 -->
<!-- last-edited: 2026-05-01 -->

# PKG-1: Extract `internal/audiobooks/` Service Package

**TODO ID:** PKG-1  
**Effort:** Large  
**Impact:** High — removes ~2750 lines of business logic from the server package  
**Companion bot-task:** [`docs/superpowers/bot-tasks/2026-05-01-pkg-1-audiobooks-service.md`](../bot-tasks/2026-05-01-pkg-1-audiobooks-service.md)

---

## Problem

Seven audiobook-domain service files live in `internal/server/` despite having zero
HTTP/gin dependency. They contain the core business logic of the application — listing,
filtering, sorting, updating, organizing, and reverting audiobooks — but are compiled
into the same package as 100+ HTTP handler files. This makes the business logic:

- Untestable without standing up the full server
- Invisible as a boundary (callers can reach any unexported symbol)
- Impossible to reuse from CLI or batch tools without importing the HTTP layer

Evidence (all gin-free, confirmed by `grep -rL 'gin\|"net/http"'`):

| File | Lines | Purpose |
|------|-------|---------|
| `audiobook_service.go` | 1891 | Core CRUD, filtering, sorting, search, tags |
| `audiobook_update_service.go` | 174 | Partial-update logic |
| `author_series_service.go` | 185 | Author/series/narrator management |
| `organize_service.go` | 40 | File organize orchestration |
| `organize_preview_service.go` | 29 | Dry-run preview |
| `revert_service.go` | 204 | Revert organize operations |
| `rename_service.go` | 33 | Rename operations |

**Total: ~2560 lines to move.**

---

## Solution

Create `internal/audiobooks/` as the audiobook service package. Move all 7 files there,
change the package declaration from `server` to `audiobooks`, and update the 4 call
sites in `internal/server/` that reference `AudiobookService`.

No new abstractions needed — the `audiobookStore` composite interface already exists in
`audiobook_service.go` (lines 29–50) and captures exactly what the service needs from
the database layer.

---

## No Import Cycles

Verified: `internal/activity`, `internal/cache`, `internal/config`, `internal/database`,
`internal/dedup`, `internal/mediainfo`, `internal/metadata`, `internal/search` — none
import `internal/server`. Moving to `internal/audiobooks/` introduces no cycles.

---

## Package Structure After

```
internal/audiobooks/
    service.go              ← audiobook_service.go (renamed)
    update_service.go       ← audiobook_update_service.go (renamed)
    author_series.go        ← author_series_service.go (renamed)
    organize.go             ← organize_service.go (renamed)
    organize_preview.go     ← organize_preview_service.go (renamed)
    revert.go               ← revert_service.go (renamed)
    rename.go               ← rename_service.go (renamed)
```

All files keep the same public API. The package declaration changes from `package server`
to `package audiobooks`.

---

## Interface Contract

The `audiobookStore` interface (currently unexported in `audiobook_service.go`) remains
unexported inside the new package. It is the dependency contract between
`internal/audiobooks/` and the database layer:

```go
// audiobooks/service.go — unchanged from current internal/server/audiobook_service.go lines 29–50
type audiobookStore interface {
    database.BookStore
    database.AuthorStore
    database.SeriesStore
    database.NarratorStore
    database.BookFileStore
    database.HashBlocklistStore
    database.TagStore
    database.MetadataStore
    database.UserPreferenceStore
    database.UserPositionStore
}
```

---

## Changes in `internal/server/`

After moving files, `internal/server/` needs these updates:

### `internal/server/server.go`

1. Add import: `"github.com/jdfalk/audiobook-organizer/internal/audiobooks"`
2. Change field type: `audiobooks *AudiobookService` → `audiobooks *audiobooks.AudiobookService`
3. Change constructor call (lines ~141, ~282): `NewAudiobookService(...)` → `audiobooks.NewAudiobookService(...)`

### `internal/server/ai_handlers.go` (line 179)

Change any direct `AudiobookService` type reference to `audiobooks.AudiobookService`.
The method call syntax (`s.audiobooks.GetAudiobook(...)`) is unchanged.

### `internal/server/audiobook_update_service.go`

This file is moving to `internal/audiobooks/update_service.go` — no server-side changes
needed once it moves.

---

## Step-by-Step Implementation

### Step 1 — Create the new package directory

```bash
mkdir -p internal/audiobooks
```

### Step 2 — Move and rename each file

For each file, copy it to the new location, change the first line from
`package server` to `package audiobooks`, and bump the version header.

Files to move (source → destination):
```
internal/server/audiobook_service.go          → internal/audiobooks/service.go
internal/server/audiobook_update_service.go   → internal/audiobooks/update_service.go
internal/server/author_series_service.go      → internal/audiobooks/author_series.go
internal/server/organize_service.go           → internal/audiobooks/organize.go
internal/server/organize_preview_service.go   → internal/audiobooks/organize_preview.go
internal/server/revert_service.go             → internal/audiobooks/revert.go
internal/server/rename_service.go             → internal/audiobooks/rename.go
```

After moving, delete the originals from `internal/server/`.

### Step 3 — Update file headers

In each moved file:
- Change `package server` → `package audiobooks`
- Update `// file:` header to new path
- Bump version (patch increment)
- Update `// last-edited:` date

### Step 4 — Fix imports inside moved files

Inside the moved files, any import of `internal/server` sub-symbols is a red flag
(should be none — confirm with `grep`). All other imports are unchanged.

### Step 5 — Update `internal/server/server.go`

1. Add `"github.com/jdfalk/audiobook-organizer/internal/audiobooks"` to imports
2. Find `*AudiobookService` field → change to `*audiobooks.AudiobookService`
3. Find `NewAudiobookService(` → change to `audiobooks.NewAudiobookService(`
4. Bump server.go version header

### Step 6 — Update `internal/server/ai_handlers.go`

Find any `AudiobookService` type reference, prefix with `audiobooks.`.
Confirm call sites (s.audiobooks.XYZ) still work — they will since the field type changes
but the value is the same struct pointer.

### Step 7 — Build and verify

```bash
go build ./...
go vet ./...
go test ./internal/audiobooks/... ./internal/server/...
```

---

## Rollback

If the build fails at any step, `git checkout internal/server/` restores all original
files. The new `internal/audiobooks/` directory can be deleted.

---

## Test Strategy

Existing unit tests in `internal/server/*_test.go` that test `AudiobookService` should
continue to pass after adding the `audiobooks.` import prefix where needed. If tests are
in `package server` and reference `AudiobookService` directly, they may need to either:
- Move to `package audiobooks` (for white-box tests)
- Or add the `audiobooks.` import prefix (for black-box tests using the exported API)
