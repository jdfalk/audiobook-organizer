<!-- file: docs/superpowers/bot-tasks/2026-04-29-deluge-3-import-to-library.md -->
<!-- version: 1.0.0 -->
<!-- guid: b2d9e7c3-5f1a-4082-9e6b-3c0a5d8f2e17 -->
<!-- last-edited: 2026-04-29 -->

# BOT TASK: DELUGE-3 — importToLibrary Function

**TODO ID:** DELUGE-3
**Audience:** burndown bot
**Branch:** `feat/deluge-3-import-to-library`
**PR title:** `feat(deluge): add importToLibrary to copy Deluge files into the library`

**Prerequisite:** DELUGE-1 must be merged first (the three new `BookFile` fields
`DelugeHash`, `DelugeOriginalPath`, `ImportedFromDelugeAt` must exist).

---

## What This Task Does

Creates `internal/server/deluge_import.go` — the `importToLibrary` function. When
a file currently living in a Deluge-managed directory needs to enter the organized
library, this function:

1. Copies the file into the library root (trying reflink first, falling back to
   `io.Copy`).
2. Updates the `BookFile` in the database (`DelugeOriginalPath`, `FilePath`,
   `ImportedFromDelugeAt`).
3. Optionally calls `delugeClient.MoveStorage` to move the torrent's storage to the
   new directory (best-effort — log errors but do not return them).

---

## What NOT to Do

- **Do NOT use `ffmpeg`** for the copy. Use either the OS reflink syscall or
  `io.Copy`. The old ffmpeg approach is gone from this codebase.
- **Do NOT call `os.Rename`** across filesystem boundaries. Reflink or copy only.
- **Do NOT call `store.UpdateBook`** — this function only updates `BookFile` rows
  via `store.UpdateBookFile`.
- **Do NOT return an error from `MoveStorage` failure** — log it with `log.Printf`
  and continue. The Deluge move is best-effort.
- **Do NOT import `internal/deluge` inside `internal/server`** as a package import
  without verifying the import path compiles. The Deluge client is passed in as a
  `*deluge.Client` parameter.
- **Do NOT skip creating the destination directory** — call `os.MkdirAll` before
  writing the file.

---

## Background: Key Types

Before writing any code, read these files:

- `internal/database/store.go` lines ~573-612 — the `BookFile` struct. After
  DELUGE-1 merges, it has `DelugeHash`, `DelugeOriginalPath`, `ImportedFromDelugeAt`.
- `internal/deluge/client.go` — `MoveStorage(torrentIDs []string, destPath string) error`.
  Note: `MoveStorage` takes a **slice** of torrent ID strings, not a single string.
- `internal/config/config.go` lines ~89, ~242-247 — `RootDir string`,
  `DelugeMoveEnabled bool`.

The relevant `UpdateBookFile` signature (from `internal/database/sqlite_store.go`):

```go
func (s *SQLiteStore) UpdateBookFile(id string, file *BookFile) error
```

The `Store` interface in `internal/database/iface_ops.go` has `UpdateBookFile`.
Use the interface type `database.Store` for the parameter.

---

## Step 1 — Create `internal/server/deluge_import.go`

Write the file with the **exact** contents below.

```go
// file: internal/server/deluge_import.go
// version: 1.0.0
// guid: f3e7a9c1-2b4d-5086-d9f2-4e1c7b0a3e58

package server

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/deluge"
)

// importToLibrary copies a file from a Deluge-managed path into the library root,
// updates the BookFile record in the database, and optionally tells Deluge to move
// the torrent storage to the new directory.
//
// Parameters:
//   - cfg: app config (used for RootDir and DelugeMoveEnabled)
//   - delugeClient: Deluge JSON-RPC client (may be nil; if nil, MoveStorage is skipped)
//   - store: database store (used to call UpdateBookFile)
//   - bookFile: the BookFile to import; its FilePath must point to the source file.
//     After a successful return, bookFile.FilePath is updated to the new path.
//
// Returns the new absolute file path and nil on success.
// Returns an error if the source file cannot be read or the destination cannot be written.
// A MoveStorage failure is NOT returned as an error — it is logged only.
func importToLibrary(
	cfg *config.Config,
	delugeClient *deluge.Client,
	store database.Store,
	bookFile *database.BookFile,
) (newPath string, err error) {
	if bookFile == nil {
		return "", fmt.Errorf("importToLibrary: bookFile is nil")
	}
	src := bookFile.FilePath
	if src == "" {
		return "", fmt.Errorf("importToLibrary: bookFile.FilePath is empty")
	}

	// Determine destination directory inside RootDir.
	// If the source is already under RootDir, preserve relative structure.
	// If it is outside, place it directly under RootDir using just the filename.
	var destDir string
	rel, relErr := filepath.Rel(cfg.RootDir, filepath.Dir(src))
	if relErr == nil && !filepath.IsAbs(rel) && !isParentTraversal(rel) {
		// Source is under RootDir (or a sub-path of it) — preserve structure.
		destDir = filepath.Join(cfg.RootDir, rel)
	} else {
		// Source is outside RootDir — place directly under RootDir.
		destDir = cfg.RootDir
	}

	dest := filepath.Join(destDir, filepath.Base(src))

	// Do not copy if source and destination are the same path.
	if src == dest {
		log.Printf("[INFO] importToLibrary: source and dest are the same (%s), skipping copy", src)
		return src, nil
	}

	// Create destination directory if it does not exist.
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("importToLibrary: create dest dir %s: %w", destDir, err)
	}

	// Attempt reflink copy (Linux: ioctl FICLONE; macOS: clonefile).
	// Falls back to io.Copy on any error.
	copyErr := reflinkCopy(src, dest)
	if copyErr != nil {
		log.Printf("[DEBUG] importToLibrary: reflink failed (%v), falling back to io.Copy", copyErr)
		if err := ioCopy(src, dest); err != nil {
			return "", fmt.Errorf("importToLibrary: copy %s -> %s: %w", src, dest, err)
		}
	}

	// Update the BookFile record.
	now := time.Now()
	bookFile.DelugeOriginalPath = src
	bookFile.FilePath = dest
	bookFile.ImportedFromDelugeAt = &now

	if err := store.UpdateBookFile(bookFile.ID, bookFile); err != nil {
		// The file has been copied but the DB update failed. Log it — the
		// caller is responsible for retry or rollback.
		return dest, fmt.Errorf("importToLibrary: UpdateBookFile %s: %w", bookFile.ID, err)
	}

	log.Printf("[INFO] importToLibrary: copied %s -> %s", src, dest)

	// Best-effort: tell Deluge to move the torrent storage.
	if cfg.DelugeMoveEnabled && bookFile.DelugeHash != "" && delugeClient != nil {
		moveErr := delugeClient.MoveStorage([]string{bookFile.DelugeHash}, filepath.Dir(dest))
		if moveErr != nil {
			log.Printf("[WARN] importToLibrary: MoveStorage for hash %s failed (non-fatal): %v",
				bookFile.DelugeHash, moveErr)
			// Do NOT return this error. MoveStorage is best-effort.
		} else {
			log.Printf("[INFO] importToLibrary: MoveStorage for hash %s -> %s succeeded",
				bookFile.DelugeHash, filepath.Dir(dest))
		}
	}

	return dest, nil
}

// isParentTraversal returns true if the rel path starts with ".." (escapes root).
func isParentTraversal(rel string) bool {
	return len(rel) >= 2 && rel[:2] == ".."
}

// reflinkCopy attempts a copy-on-write clone of src to dest using OS-specific
// syscalls. Returns an error if the reflink is not supported or fails.
func reflinkCopy(src, dest string) error {
	return reflinkCopyOS(src, dest)
}

// ioCopy copies src to dest using standard io.Copy (read all bytes, write all bytes).
func ioCopy(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open src: %w", err)
	}
	defer in.Close()

	out, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create dest: %w", err)
	}
	defer func() {
		cerr := out.Close()
		if err == nil {
			err = cerr
		}
	}()

	if _, err = io.Copy(out, in); err != nil {
		return fmt.Errorf("io.Copy: %w", err)
	}
	return nil
}
```

---

## Step 2 — Create Platform-Specific Reflink Files

The `reflinkCopyOS` function referenced above must exist in platform-specific files.
Create both files below. Do NOT modify any other files.

### `internal/server/deluge_import_linux.go`

```go
// file: internal/server/deluge_import_linux.go
// version: 1.0.0
// guid: a9c2e5f7-1d3b-4087-b8a6-5f2c0e9d1b74

package server

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// reflinkCopyOS attempts a reflink copy using Linux's FICLONE ioctl.
// Returns an error if the filesystem does not support reflinks (e.g. ext4)
// or if the operation fails for any other reason.
func reflinkCopyOS(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open src: %w", err)
	}
	defer in.Close()

	out, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create dest: %w", err)
	}
	defer out.Close()

	err = unix.IoctlFileClone(int(out.Fd()), int(in.Fd()))
	if err != nil {
		// Remove the empty dest file created above, so the caller's ioCopy
		// will create a fresh one.
		_ = os.Remove(dest)
		return fmt.Errorf("FICLONE ioctl: %w", err)
	}
	return nil
}
```

### `internal/server/deluge_import_darwin.go`

```go
// file: internal/server/deluge_import_darwin.go
// version: 1.0.0
// guid: c7f4b1e8-2a6d-4093-9c8f-3d0b5a7e2c19

package server

import (
	"fmt"
	"syscall"
	"unsafe"
)

// reflinkCopyOS attempts a reflink copy using macOS's clonefile(2) syscall.
// clonefile is available on macOS 10.12+ on APFS volumes.
func reflinkCopyOS(src, dest string) error {
	srcBytes, err := syscall.BytePtrFromString(src)
	if err != nil {
		return fmt.Errorf("src path: %w", err)
	}
	destBytes, err := syscall.BytePtrFromString(dest)
	if err != nil {
		return fmt.Errorf("dest path: %w", err)
	}
	// clonefile(2): syscall number 462 on arm64/amd64 macOS.
	// Flags = 0 (no CLONE_NOFOLLOW, no CLONE_NOOWNERCOPY).
	_, _, errno := syscall.Syscall(462,
		uintptr(unsafe.Pointer(srcBytes)),
		uintptr(unsafe.Pointer(destBytes)),
		0,
	)
	if errno != 0 {
		return fmt.Errorf("clonefile: %w", errno)
	}
	return nil
}
```

**IMPORTANT:** The Linux file uses `golang.org/x/sys/unix`. Before writing that
file, verify this dependency exists in `go.mod`:

```bash
grep "golang.org/x/sys" /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/go.mod
```

If it exists, proceed. If it does NOT exist, replace the Linux implementation with
the following stub that always returns an error (which causes the fallback to
`io.Copy`):

```go
// reflinkCopyOS always returns an error on this platform (no reflink support).
func reflinkCopyOS(src, dest string) error {
	return fmt.Errorf("reflink not supported on this build")
}
```

And change the imports in `deluge_import_linux.go` to just `"fmt"`.

---

## Step 3 — Write 2 Tests

Create `internal/server/deluge_import_test.go`:

```go
// file: internal/server/deluge_import_test.go
// version: 1.0.0
// guid: e1b5d8f2-3c7a-4091-a2e9-6f4d0c8b3a15

package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// fakeStore is a minimal database.Store implementation for testing.
// Only UpdateBookFile is implemented; all others panic or return nil.
type fakeStore struct {
	database.Store // embed the interface so we don't need to implement all methods
	updated *database.BookFile
}

func (f *fakeStore) UpdateBookFile(id string, file *database.BookFile) error {
	f.updated = file
	return nil
}

// Test 1: When reflink fails, ioCopy succeeds and the database is updated.
func TestImportToLibrary_FallbackToCopy(t *testing.T) {
	// Create a temp source file.
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "book.m4b")
	if err := os.WriteFile(srcFile, []byte("audio data"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Destination is a different temp dir (the "library root").
	rootDir := t.TempDir()

	cfg := &config.Config{
		RootDir:          rootDir,
		DelugeMoveEnabled: false, // no Deluge move
	}
	store := &fakeStore{}
	bf := &database.BookFile{
		ID:       "test-id-001",
		FilePath: srcFile,
		// DelugeHash intentionally empty so MoveStorage is skipped.
	}

	newPath, err := importToLibrary(cfg, nil, store, bf)
	if err != nil {
		t.Fatalf("importToLibrary returned error: %v", err)
	}

	// Verify destination file exists.
	if _, statErr := os.Stat(newPath); statErr != nil {
		t.Errorf("destination file does not exist: %v", statErr)
	}

	// Verify DB was updated.
	if store.updated == nil {
		t.Fatal("UpdateBookFile was not called")
	}
	if store.updated.FilePath != newPath {
		t.Errorf("BookFile.FilePath = %q, want %q", store.updated.FilePath, newPath)
	}
	if store.updated.DelugeOriginalPath != srcFile {
		t.Errorf("BookFile.DelugeOriginalPath = %q, want %q", store.updated.DelugeOriginalPath, srcFile)
	}
	if store.updated.ImportedFromDelugeAt == nil {
		t.Error("BookFile.ImportedFromDelugeAt is nil, want non-nil")
	}
}

// Test 2: When source and destination are the same path, no copy is done.
func TestImportToLibrary_SameSourceAndDest(t *testing.T) {
	rootDir := t.TempDir()

	// Create a file inside the library root.
	srcFile := filepath.Join(rootDir, "book.m4b")
	if err := os.WriteFile(srcFile, []byte("audio data"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		RootDir:          rootDir,
		DelugeMoveEnabled: false,
	}
	store := &fakeStore{}
	bf := &database.BookFile{
		ID:       "test-id-002",
		FilePath: srcFile,
	}

	newPath, err := importToLibrary(cfg, nil, store, bf)
	if err != nil {
		t.Fatalf("importToLibrary returned error: %v", err)
	}
	if newPath != srcFile {
		t.Errorf("expected newPath = %q (same as src), got %q", srcFile, newPath)
	}
	// When source == dest, UpdateBookFile should NOT be called.
	if store.updated != nil {
		t.Error("UpdateBookFile was called even though source == dest; expected no-op")
	}
	_ = time.Now() // keep time import used
}
```

---

## Step 4 — Verify Build

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
go build ./...
go test ./internal/server/... -run TestImportToLibrary
```

Both must pass with no errors. If `go build` fails because `database.Store` does
not have `UpdateBookFile` in the interface, open `internal/database/iface_ops.go`
and check if it is listed. If it is not, do NOT add it — that is a sign that DELUGE-1
was not merged yet. Stop and report this dependency.

---

## Step 5 — Commit and Open PR

```bash
git checkout -b feat/deluge-3-import-to-library
git add \
  internal/server/deluge_import.go \
  internal/server/deluge_import_linux.go \
  internal/server/deluge_import_darwin.go \
  internal/server/deluge_import_test.go
git commit -m "feat(deluge): add importToLibrary to copy Deluge files into the library"
git push -u origin feat/deluge-3-import-to-library
gh pr create \
  --title "feat(deluge): add importToLibrary to copy Deluge files into the library" \
  --body "Adds importToLibrary: tries OS reflink (FICLONE on Linux, clonefile on macOS), falls back to io.Copy. Updates BookFile.DelugeOriginalPath/FilePath/ImportedFromDelugeAt in DB. Best-effort MoveStorage call to Deluge. 2 unit tests. Requires DELUGE-1."
```

---

## Checklist

- [ ] `internal/server/deluge_import.go` created
- [ ] `internal/server/deluge_import_linux.go` created
- [ ] `internal/server/deluge_import_darwin.go` created
- [ ] `internal/server/deluge_import_test.go` created with 2 tests
- [ ] `go build ./...` passes
- [ ] `go test ./internal/server/... -run TestImportToLibrary` passes both tests
- [ ] PR opened with correct branch name and title
