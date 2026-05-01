<!-- file: docs/superpowers/specs/2026-04-30-structured-logging.md -->
<!-- version: 1.0.0 -->
<!-- guid: 9e8d7c6b-5a4f-3e2d-1c0b-9a8f7e6d5c4b -->
<!-- last-edited: 2026-04-30 -->

# Structured Logging — Replace fmt.Printf in Library Packages

**Status:** Draft — awaiting implementation
**Scope:** `internal/tagger/`, `internal/fileops/`, `internal/backup/`, `internal/scanner/`
**Related specs:** none

---

## Problem

**M-12 — `fmt.Printf` in library packages:**
Three library packages write directly to stdout using `fmt.Printf` / `fmt.Println`:

- `internal/tagger/tagger.go:48, 55, 93, 104, 115`
- `internal/fileops/safe_operations.go:137, 144, 207`
- `internal/backup/backup.go:119, 194, 367`

When these packages are used inside the server process, their output interleaves with
the server's structured log stream on stdout/stderr, making logs unparseable by
log aggregators (Loki, CloudWatch, etc.).

**N-2 — `progressbar` writes ANSI to stderr in server context:**
`internal/scanner/scanner.go:286` uses `github.com/schollz/progressbar/v3` to
render a terminal progress bar to stderr. In a server process with no TTY (systemd,
Docker), this produces garbled ANSI escape sequences in logs. The scanner already
reports progress via SSE (`internal/realtime`), so the progressbar is redundant.

---

## Core Rule / Goal

> **Library packages must never write to stdout/stderr directly.
> Use `log.Printf` (or the project's structured logger) so output flows through
> the centrally configured log sink. Remove TTY-only libraries from server code.**

---

## Approach

### LOG-1, LOG-2, LOG-3 — Replace fmt.Printf

For each affected file, replace:
- `fmt.Printf(...)` → `log.Printf(...)`
- `fmt.Println(...)` → `log.Println(...)`
- `fmt.Fprintf(os.Stderr, ...)` → `log.Printf(...)`

Import `"log"` (standard library) or the project's internal logger if one exists
at `internal/logger/`. Do not import `fmt` solely for printing.

### LOG-4 — Remove progressbar

Remove all uses of `progressbar` from `scanner.go`. Delete the initialization,
update, and finish calls. Run `go mod tidy` to remove the `github.com/schollz/progressbar/v3`
dependency from `go.mod` / `go.sum`.

---

## Acceptance Criteria

- [ ] `grep -n 'fmt\.Printf\|fmt\.Println' internal/tagger/tagger.go` returns 0.
- [ ] `grep -n 'fmt\.Printf\|fmt\.Println' internal/fileops/safe_operations.go` returns 0.
- [ ] `grep -n 'fmt\.Printf\|fmt\.Println' internal/backup/backup.go` returns 0.
- [ ] `grep -rn 'progressbar' internal/` returns 0 matches (excluding any test files that explicitly test it).
- [ ] `go build ./...` is clean.
- [ ] `go mod tidy` produces no changes after LOG-4.

---

## Related Bot-Tasks

- [`2026-04-30-log-1-tagger.md`](../bot-tasks/2026-04-30-log-1-tagger.md) — LOG-1
- [`2026-04-30-log-2-fileops.md`](../bot-tasks/2026-04-30-log-2-fileops.md) — LOG-2
- [`2026-04-30-log-3-backup.md`](../bot-tasks/2026-04-30-log-3-backup.md) — LOG-3
- [`2026-04-30-log-4-progressbar.md`](../bot-tasks/2026-04-30-log-4-progressbar.md) — LOG-4
