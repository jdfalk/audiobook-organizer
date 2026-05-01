<!-- file: docs/superpowers/bot-tasks/2026-05-01-log-5-remaining-printf.md -->
<!-- version: 1.0.0 -->
<!-- guid: c8d9e0f1-a2b3-4c5d-6e7f-8a9b0c1d2e3f -->
<!-- last-edited: 2026-05-01 -->

# BOT TASK: LOG-5 — Replace remaining `fmt.Printf` / `log.Printf` in library packages

**TODO ID:** LOG-5  
**Audience:** burndown bot  
**Branch:** `fix/log-remaining-printf`  
**PR title:** `fix(logging): replace remaining fmt.Printf/log.Printf in library packages`

---

## What This Task Does

Replaces the remaining `fmt.Printf` and bare `log.Printf` calls in library
packages (`internal/database/`, `internal/playlist/`, `internal/organizer/`) with
structured logging. LOG-1..4 addressed scanner, tagger, fileops, and backup;
several packages were missed.

---

## What NOT to Do

- **Do NOT** change `fmt.Fprintf(os.Stderr, …)` in CLI entry points (`cmd/`).
- **Do NOT** change `log.Printf` in `internal/database/migrations.go` to a
  structured logger if no logger is available in scope — use `slog.Info/Warn/Error`
  which is always available in Go 1.21+.
- **Do NOT** change function signatures to add a logger parameter unless the
  function already accepts one via a context or struct.

---

## Target Call Sites

### `fmt.Printf` in library packages

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
grep -rn 'fmt\.Printf\|fmt\.Println' --include='*.go' \
  internal/database/ internal/playlist/ internal/organizer/ | grep -v test | head -20
```

| File | Line | Message |
|------|------|---------|
| `internal/database/sqlite_store.go` | 404 | `"Warning: series deduplication failed: …"` |
| `internal/database/sqlite_store.go` | 807 | `"Deduplicated %d series records\n"` |
| `internal/playlist/playlist.go` | 94 | `"Generated playlist: %s\n"` |
| `internal/organizer/organizer.go` | 68 | `"Warning: failed to clean temporary organizer files: …"` |

### `log.Printf` / `log.Println` in library packages

```bash
grep -rn 'log\.Printf\|log\.Println\|log\.Fatal' \
  --include='*.go' internal/database/ | grep -v test | head -20
```

| File | Lines | Context |
|------|-------|---------|
| `internal/database/pebble_store.go` | 93,119,134,754,783 | PebbleDB ops |
| `internal/database/migrations.go` | 395–433 | Migration progress |
| `internal/database/sqlite_store.go` | 2813, 3164 | Transaction rollback |
| `internal/database/metadata_fetch_cache.go` | 88 | Cache warning |

---

## Steps

### Step 1 — Replace `fmt.Printf` with `slog`

For packages without a structured logger available, use Go's standard `log/slog`:

```go
// Before:
fmt.Printf("Warning: series deduplication failed: %v\n", err)

// After:
slog.Warn("series deduplication failed", "error", err)
```

```go
// Before:
fmt.Printf("Deduplicated %d series records\n", totalMerged)

// After:
slog.Info("series deduplication complete", "merged", totalMerged)
```

### Step 2 — Replace bare `log.Printf` with `slog`

```go
// Before (pebble_store.go:93):
log.Printf("ERROR: pebble Delete stats:library: %v", err)

// After:
slog.Error("pebble Delete stats:library", "error", err)
```

```go
// Before (migrations.go:395):
log.Printf("Current database version: %d", currentVersion)

// After:
slog.Info("database version", "version", currentVersion)
```

### Step 3 — Check if `slog` import already exists

```bash
grep -n 'log/slog\|"log"' internal/database/sqlite_store.go internal/database/pebble_store.go \
  internal/database/migrations.go internal/playlist/playlist.go internal/organizer/organizer.go | head -20
```

Add `"log/slog"` import where missing; remove `"log"` if it is no longer used
after the replacement.

### Step 4 — Build and test

```bash
go build ./...
go test ./internal/database/... ./internal/playlist/... ./internal/organizer/... \
  -timeout 120s 2>&1 | grep -E 'FAIL|ok'
```

### Step 5 — Verify no `fmt.Printf`/`log.Printf` remain in targets

```bash
grep -rn 'fmt\.Printf\|fmt\.Println\|log\.Printf\|log\.Println' \
  --include='*.go' internal/database/ internal/playlist/ internal/organizer/ | grep -v test
```

Should be empty.

### Step 6 — Commit and open PR

```bash
git checkout -b fix/log-remaining-printf
git add internal/database/ internal/playlist/ internal/organizer/
git commit -m "fix(logging): replace remaining fmt.Printf/log.Printf in library packages

Replaces fmt.Printf in sqlite_store, playlist, organizer and bare
log.Printf in pebble_store, migrations, metadata_fetch_cache with
structured slog calls. Completes LOG-1..4 scope for missed packages.
Re-audit finding R-8 / LOG-5.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin fix/log-remaining-printf
gh pr create \
  --title "fix(logging): replace remaining fmt.Printf/log.Printf in library packages" \
  --body "Completes structured logging migration for database, playlist, and organizer packages. Re-audit finding R-8."
```

---

## Checklist

- [ ] `sqlite_store.go:404,807` — `fmt.Printf` replaced with `slog`
- [ ] `playlist.go:94` — `fmt.Printf` replaced with `slog`
- [ ] `organizer.go:68` — `fmt.Printf` replaced with `slog`
- [ ] `pebble_store.go:93,119,134,754,783` — `log.Printf` replaced with `slog`
- [ ] `migrations.go:395–433` — `log.Printf`/`log.Println` replaced with `slog`
- [ ] `sqlite_store.go:2813,3164` — `log.Printf` replaced with `slog`
- [ ] `metadata_fetch_cache.go:88` — `log.Printf` replaced with `slog`
- [ ] No `"log"` import left if all uses were replaced
- [ ] `go build ./...` clean
- [ ] Tests pass
- [ ] PR opened with correct branch and title
