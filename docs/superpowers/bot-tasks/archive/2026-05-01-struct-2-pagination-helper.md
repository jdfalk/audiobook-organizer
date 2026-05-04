<!-- file: docs/superpowers/bot-tasks/2026-05-01-struct-2-pagination-helper.md -->
<!-- version: 1.0.0 -->
<!-- guid: c3d4e5f6-a7b8-9012-cdef-345678901234 -->
<!-- last-edited: 2026-05-01 -->

# BOT TASK: STRUCT-2 — Add shared pagination helper to `internal/server`

**TODO ID:** STRUCT-2
**Audience:** burndown bot
**Branch:** `refactor/struct-2-pagination-helper`
**PR title:** `refactor(server): add shared PaginationFromQuery helper`

---

## What This Task Does

`paginationFromQuery` **already exists** in `internal/server/playlist_handlers.go`
(package-level, accessible to all server files). Seven other handlers have
independent duplicate implementations with slightly different clamping rules.

This task consolidates: move `paginationFromQuery` to a dedicated
`internal/server/pagination.go` (with a `Pagination` return type), then update
the 7+ other duplicate parsers to call it.

**Evidence of duplicate parsers:**
- `internal/server/dedup_handlers.go:53–67`
- `internal/server/activity_handlers.go:41–50`
- `internal/server/metadata_handlers.go:412+`
- `internal/server/reading_handlers.go:171–176`
- `internal/server/itunes_handlers.go:535–540`
- `internal/server/metadata_batch_candidates.go:331–337`
- `internal/server/maintenance_fixups.go:6192–6195`

---

## What NOT to Do

- **Do NOT** change pagination defaults (limit=50, max=500 — match existing).
- **Do NOT** remove the existing `paginationFromQuery` from `playlist_handlers.go`
  until AFTER moving it — or the package will fail to compile.
- **Do NOT** rename the function in this PR (keep `paginationFromQuery`).

---

## Step-by-step

### Step 1 — Read 3 existing pagination patterns first

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer

grep -A6 'DefaultQuery.*limit\|Query.*"limit"' internal/server/audiobooks_handlers.go | head -30
grep -A6 'DefaultQuery.*limit\|Query.*"limit"' internal/server/audiobook_service.go | head -30
grep -A6 'DefaultQuery.*limit\|Query.*"limit"' internal/server/entities_handlers.go | head -20
```

Note the patterns: what default values are used for `limit`? What max is clamped to?
Use `50` as the default limit and `1000` as the max (most common in the codebase).

### Step 2 — Read the existing implementation

```bash
grep -n 'func paginationFromQuery' internal/server/playlist_handlers.go
sed -n '415,435p' internal/server/playlist_handlers.go
```

Note the exact implementation (limit=50 default, max=500).

### Step 3 — Create `internal/server/pagination.go`

Create a new file with just this function (cut-paste from playlist_handlers.go):

```go
// file: internal/server/pagination.go
// version: 1.0.0
// last-edited: 2026-05-01
// guid: d4e5f6a7-b8c9-0123-defa-456789012345

package server

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

// paginationFromQuery parses limit/offset query params with sane defaults + caps.
// Default limit = 50, max = 500, default offset = 0.
func paginationFromQuery(c *gin.Context) (int, int) {
	limit, offset := 50, 0
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	if o := c.Query("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			offset = n
		}
	}
	return limit, offset
}
```

### Step 4 — Remove the copy from playlist_handlers.go

Delete the function body from `internal/server/playlist_handlers.go` (lines ~418-431).
The package now has exactly one definition.

```bash
# Verify only one definition remains
grep -rn 'func paginationFromQuery' internal/server/
```

Expected: one line only, pointing to `pagination.go`.

### Step 5 — Update the 7 duplicate parsers

For each file below, delete the hand-rolled parser and replace the call with
`paginationFromQuery(c)`:

- `internal/server/dedup_handlers.go:53–67`
- `internal/server/activity_handlers.go:41–50`
- `internal/server/metadata_handlers.go:412+`
- `internal/server/reading_handlers.go:171–176`
- `internal/server/itunes_handlers.go:535–540`
- `internal/server/metadata_batch_candidates.go:331–337`
- `internal/server/maintenance_fixups.go:6192–6195`

Each caller becomes: `limit, offset := paginationFromQuery(c)`

Remove `"strconv"` import from each file if it was only used for pagination.

### Step 6 — Build and test

```bash
go build ./internal/server/...
go test ./internal/server/... -timeout 120s 2>&1 | grep -E 'FAIL|ok|---'
```

Both must pass.

### Step 7 — Bump version headers

Bump the patch version on every changed file.

### Step 8 — Commit and open PR

```bash
git checkout -b refactor/struct-2-pagination-helper
git add internal/server/
git commit -m "refactor(server): consolidate pagination parsing into shared helper

Moves paginationFromQuery from playlist_handlers.go into a dedicated
pagination.go and updates 7 duplicate hand-rolled parsers to use it.
Eliminates inconsistent limit/offset handling. Structure audit STRUCT-2.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin refactor/struct-2-pagination-helper
gh pr create \
  --title "refactor(server): consolidate pagination parsing into shared helper" \
  --body "Moves paginationFromQuery to pagination.go, removes 7 duplicate parsers. Structure audit STRUCT-2."
```

---

## Checklist

- [ ] `internal/server/pagination.go` created with `paginationFromQuery`
- [ ] Function removed from `playlist_handlers.go`
- [ ] 7 duplicate parsers replaced
- [ ] `go build ./internal/server/...` clean
- [ ] Tests pass
- [ ] PR opened on branch `refactor/struct-2-pagination-helper`
