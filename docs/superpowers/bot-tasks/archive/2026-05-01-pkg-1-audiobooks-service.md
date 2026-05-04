<!-- file: docs/superpowers/bot-tasks/2026-05-01-pkg-1-audiobooks-service.md -->
<!-- version: 1.0.0 -->
<!-- guid: b2c3d4e5-f6a7-8901-bcde-f01234560001 -->
<!-- last-edited: 2026-05-01 -->

# BOT TASK: Extract `internal/audiobooks/` Service Package

**TODO ID:** PKG-1  
**Audience:** burndown bot  
**Design spec:** [`docs/superpowers/specs/2026-05-01-pkg-1-extract-audiobooks-package.md`](../specs/2026-05-01-pkg-1-extract-audiobooks-package.md)

## Prerequisites

- PKG-1 spec read and understood
- Branch `refactor/pkg-1-audiobooks` created from latest main
- Work in a git worktree at `/Users/jdfalk/.worktrees/pkg-1-audiobooks`

## Branch

```
refactor/pkg-1-audiobooks
```

## Files to Create

None — all files are moved from `internal/server/`.

## Files to Move

| Source | Destination |
|--------|-------------|
| `internal/server/audiobook_service.go` | `internal/audiobooks/service.go` |
| `internal/server/audiobook_update_service.go` | `internal/audiobooks/update_service.go` |
| `internal/server/author_series_service.go` | `internal/audiobooks/author_series.go` |
| `internal/server/organize_service.go` | `internal/audiobooks/organize.go` |
| `internal/server/organize_preview_service.go` | `internal/audiobooks/organize_preview.go` |
| `internal/server/revert_service.go` | `internal/audiobooks/revert.go` |
| `internal/server/rename_service.go` | `internal/audiobooks/rename.go` |

## Step 1 — Create destination directory

```bash
mkdir -p internal/audiobooks
```

## Step 2 — Move files one at a time

For EACH file in the table above:
1. `cp internal/server/SOURCE.go internal/audiobooks/DEST.go`
2. Open the new file
3. Change `package server` to `package audiobooks`
4. Update `// file:` header to new path
5. Bump patch version in `// version:` header
6. Update `// last-edited:` to today
7. `go build ./internal/audiobooks/...` — fix any errors before moving to next file
8. `rm internal/server/SOURCE.go`

## Step 3 — Update `internal/server/server.go`

1. Add to imports: `"github.com/jdfalk/audiobook-organizer/internal/audiobooks"`
2. Find the field: `audiobooks *AudiobookService` → change to `audiobooks *audiobooks.AudiobookService`
3. Find `NewAudiobookService(` → change to `audiobooks.NewAudiobookService(`
4. Bump version header in server.go

## Step 4 — Update `internal/server/ai_handlers.go`

1. Search for `AudiobookService` in this file
2. Add import if needed: `"github.com/jdfalk/audiobook-organizer/internal/audiobooks"`
3. Change any `*AudiobookService` type references to `*audiobooks.AudiobookService`
4. Bump version header

## Step 5 — Fix test files

Run: `grep -rn "AudiobookService\|NewAudiobookService" internal/server/*_test.go`

For each match:
- Add import `"github.com/jdfalk/audiobook-organizer/internal/audiobooks"` to that test file
- Prefix type references with `audiobooks.`

## Step 6 — Final build and test

```bash
go build ./...
go vet ./...
go test ./internal/audiobooks/... ./internal/server/...
```

All must pass with zero errors before committing.

## Commit Message

```
refactor(server): extract audiobooks service to internal/audiobooks/

Move 7 gin-free service files (audiobook_service.go, author_series_service.go,
organize*.go, revert_service.go, rename_service.go) from internal/server/ to
the new internal/audiobooks/ package. The audiobookStore interface contract is
unchanged. Handlers in internal/server/ updated to use audiobooks.AudiobookService.

Refs: PKG-1
```
