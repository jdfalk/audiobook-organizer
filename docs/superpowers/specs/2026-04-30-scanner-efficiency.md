<!-- file: docs/superpowers/specs/2026-04-30-scanner-efficiency.md -->
<!-- version: 1.0.0 -->
<!-- guid: 6d7e8f90-1a2b-3c4d-5e6f-7890abcdef01 -->
<!-- last-edited: 2026-04-30 -->

# Scanner Efficiency — Replace filepath.Walk with filepath.WalkDir

**Status:** Draft — awaiting implementation
**Scope:** `internal/scanner/scanner.go`, `internal/server/malformed_m4b_remux.go`
**Related specs:** [`2026-04-30-structured-logging.md`](./2026-04-30-structured-logging.md)

---

## Problem

**N-1 — `filepath.Walk` calls `os.Lstat` on every entry:**
`filepath.Walk` is implemented by calling `os.Lstat` on each file/directory to
obtain a `FileInfo`. For directories with thousands of files this creates thousands
of extra syscalls. The newer `filepath.WalkDir` (Go 1.16+) uses `fs.DirEntry`,
which provides name and type information from the directory read — no extra stat
required unless `Size()` or `ModTime()` are needed.

The scan path in `internal/scanner/scanner.go` and the remux helper in
`internal/server/malformed_m4b_remux.go` both use the older `filepath.Walk`.

---

## Core Rule / Goal

> **Replace `filepath.Walk` with `filepath.WalkDir` everywhere in the codebase.
> Only call `d.Info()` (which stats the file) when size or modtime is actually needed.**

---

## Approach

For each `filepath.Walk(root, func(path string, info os.FileInfo, err error) error {...})`
call:

1. Replace with `filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {...})`.
2. Replace `info.IsDir()` → `d.IsDir()`.
3. Replace `info.Name()` → `d.Name()`.
4. Replace `info.Size()` / `info.ModTime()` / `info.Mode()` → call `info, err := d.Info()`
   first and use the returned `FileInfo`. Add error handling for the `d.Info()` call.
5. Add `"io/fs"` to imports if not already present.
6. Remove `"os"` from imports if it was only used for `os.FileInfo` (check for other uses first).

---

## Files to Change

| File | Walk location |
|------|--------------|
| `internal/scanner/scanner.go` | Main file scan loop |
| `internal/server/malformed_m4b_remux.go:52` | Remux helper scan |

---

## Acceptance Criteria

- [ ] `grep -rn 'filepath\.Walk[^D]' internal/` returns 0 matches.
- [ ] `go build ./...` is clean.
- [ ] `go vet ./...` is clean.
- [ ] No `fs.DirEntry.Info()` calls appear in the hot path when only `IsDir()` or `Name()` are needed.

---

## Related Bot-Tasks

- [`2026-04-30-scan-1-walkdir.md`](../bot-tasks/2026-04-30-scan-1-walkdir.md) — SCAN-1
