<!-- file: docs/superpowers/bot-tasks/2026-05-01-pkg-2-aiscan-pipeline.md -->
<!-- version: 1.0.0 -->
<!-- guid: b2c3d4e5-f6a7-8901-bcde-f01234560002 -->
<!-- last-edited: 2026-05-01 -->

# BOT TASK: Extract `internal/aiscan/` Pipeline Package

**TODO ID:** PKG-2  
**Audience:** burndown bot  
**Design spec:** [`docs/superpowers/specs/2026-05-01-pkg-2-extract-aiscan-package.md`](../specs/2026-05-01-pkg-2-extract-aiscan-package.md)

## Prerequisites

- PKG-2 spec read and understood
- Branch `refactor/pkg-2-aiscan` created from latest main
- Work in a git worktree at `/Users/jdfalk/.worktrees/pkg-2-aiscan`

## Branch

```
refactor/pkg-2-aiscan
```

## Step 1 — Remove dead `*Server` field from `ai_scan_pipeline.go`

In `internal/server/ai_scan_pipeline.go`:

1. Find the `PipelineManager` struct. Delete the line `server *Server`.
2. Find `NewPipelineManager(...)`. Delete the `server *Server` parameter from the signature.
3. Find `server: server,` in the constructor body. Delete that line.
4. Run `go build ./...` — must pass before continuing.

## Step 2 — Create destination package

```bash
mkdir -p internal/aiscan
```

## Step 3 — Move and rename

```bash
cp internal/server/ai_scan_pipeline.go internal/aiscan/pipeline.go
```

In `internal/aiscan/pipeline.go`:
1. Change `package server` → `package aiscan`
2. Rename `type aiScanPipelineStore interface` → `type Store interface`
3. Anywhere `aiScanPipelineStore` is used (field type, constructor param), change to `Store`
4. Update `// file:` header to `internal/aiscan/pipeline.go`
5. Bump patch version
6. Update `// last-edited:`

```bash
rm internal/server/ai_scan_pipeline.go
```

## Step 4 — Fix the call site in `internal/server/`

Run: `grep -rn "NewPipelineManager\|PipelineManager" internal/server/*.go | grep -v _test`

For each match:
1. Add import `"github.com/jdfalk/audiobook-organizer/internal/aiscan"`
2. Change `*PipelineManager` → `*aiscan.PipelineManager`
3. Change `NewPipelineManager(` → `aiscan.NewPipelineManager(`
4. Remove the `server` argument from the `NewPipelineManager(...)` call
5. Bump version header of any modified file

## Step 5 — Fix test files

Run: `grep -rn "PipelineManager\|NewPipelineManager" internal/server/*_test.go`

Apply same import/prefix changes.

## Step 6 — Build and test

```bash
go build ./...
go vet ./...
go test ./internal/aiscan/... ./internal/server/...
```

## Commit Message

```
refactor(server): extract AI scan pipeline to internal/aiscan/

Move ai_scan_pipeline.go to internal/aiscan/pipeline.go. Remove the unused
*Server field from PipelineManager (pm.server was stored but never read).
Rename aiScanPipelineStore to aiscan.Store (exported package interface).
Update call site in internal/server/ to drop the server argument.

Refs: PKG-2
```
