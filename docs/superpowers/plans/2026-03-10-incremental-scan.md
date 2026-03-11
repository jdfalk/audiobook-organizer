<!-- file: docs/superpowers/plans/2026-03-10-incremental-scan.md -->
<!-- version: 1.0.0 -->
<!-- guid: c4d5e6f7-a8b9-0c1d-2e3f-4a5b6c7d8e9f -->

# Incremental Scan & Single-Pass I/O Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make library scans skip unchanged files by default, and combine redundant file I/O into a single pass — dropping a 47k-file scan from ~50 min to ~30 sec (incremental) or ~15-20 min (full).

**Architecture:** Add mtime/size scan cache columns to the books table. At scan start, pre-load a path→cache map. Skip files whose mtime+size haven't changed and aren't marked dirty. For files that do need processing, open the file once instead of three times (tags + mediainfo + hash). Other services mark books `needs_rescan` when they modify files, triggering a folder-level rescan.

**Tech Stack:** Go, SQLite, PebbleDB, dhowden/tag, crypto/sha256

**Spec:** `docs/superpowers/specs/2026-03-10-incremental-scan-design.md`

---

## File Structure

| File | Responsibility |
|------|---------------|
| `internal/database/migrations.go` | Migration 32: scan cache columns + index |
| `internal/database/store.go` | ScanCacheEntry struct, 4 new interface methods |
| `internal/database/sqlite_store.go` | SQLite implementations |
| `internal/database/pebble_store.go` | PebbleDB implementations |
| `internal/database/mock_store.go` | Manual mock stubs |
| `internal/database/mocks/mock_store.go` | Testify mock implementations |
| `internal/scanner/process_file.go` | New: single-pass ProcessFile function |
| `internal/scanner/scanner.go` | Integrate ProcessFile, add skip logic |
| `internal/server/scan_service.go` | Incremental flow: cache pre-load, dirty folders, skip logic |
| `internal/server/metadata_fetch_service.go` | Call MarkNeedsRescan after writeback |
| `internal/server/organize_service.go` | Call MarkNeedsRescan after file move |

---

## Chunk 1: Database Layer

### Task 1: Migration 32 — Scan Cache Columns

**Files:**
- Modify: `internal/database/migrations.go`

- [ ] **Step 1: Add migration032Up function**

Add after the last migration function in `migrations.go`. The migration adds three columns to `books` and a composite index for fast cache lookups.

```go
// migration032Up adds scan cache columns for incremental scanning
func migration032Up(store Store) error {
	sqlStatements := []string{
		`ALTER TABLE books ADD COLUMN last_scan_mtime INTEGER DEFAULT NULL`,
		`ALTER TABLE books ADD COLUMN last_scan_size INTEGER DEFAULT NULL`,
		`ALTER TABLE books ADD COLUMN needs_rescan BOOLEAN DEFAULT 0`,
		`CREATE INDEX IF NOT EXISTS idx_books_scan_cache ON books(file_path, last_scan_mtime, last_scan_size)`,
		`CREATE INDEX IF NOT EXISTS idx_books_needs_rescan ON books(needs_rescan) WHERE needs_rescan = 1`,
	}
	return store.ExecMigrationSQL(sqlStatements)
}
```

- [ ] **Step 2: Register migration 32 in the migrations slice**

In the `getMigrations()` function, add after the migration 31 entry:

```go
{
    Version:     32,
    Description: "Add scan cache columns for incremental scanning",
    Up:          migration032Up,
},
```

- [ ] **Step 3: Build and verify**

Run: `GOEXPERIMENT=jsonv2 go build ./...`
Expected: Clean build

- [ ] **Step 4: Commit**

```bash
git add internal/database/migrations.go
git commit -m "feat(db): migration 32 — scan cache columns for incremental scanning"
```

---

### Task 2: Store Interface — Scan Cache Methods

**Files:**
- Modify: `internal/database/store.go`

- [ ] **Step 1: Add ScanCacheEntry struct**

Add near the other model structs (after `SystemActivityLog` or similar):

```go
// ScanCacheEntry holds mtime/size for incremental scan skip checks.
type ScanCacheEntry struct {
	Mtime       int64
	Size        int64
	NeedsRescan bool
}
```

- [ ] **Step 2: Add 4 new methods to the Store interface**

Add to the Store interface, in a new comment section:

```go
// Scan cache for incremental scanning
GetScanCacheMap() (map[string]ScanCacheEntry, error)
UpdateScanCache(bookID string, mtime int64, size int64) error
MarkNeedsRescan(bookID string) error
GetDirtyBookFolders() ([]string, error)
```

- [ ] **Step 3: Build and verify**

Run: `GOEXPERIMENT=jsonv2 go build ./...`
Expected: Build fails — SQLiteStore, PebbleStore, MockStore don't implement new methods yet. That's correct.

- [ ] **Step 4: Commit**

```bash
git add internal/database/store.go
git commit -m "feat(db): add scan cache interface methods to Store"
```

---

### Task 3: SQLite Implementation

**Files:**
- Modify: `internal/database/sqlite_store.go`

- [ ] **Step 1: Implement GetScanCacheMap**

```go
func (s *SQLiteStore) GetScanCacheMap() (map[string]ScanCacheEntry, error) {
	rows, err := s.db.Query(`SELECT file_path, last_scan_mtime, last_scan_size, needs_rescan FROM books WHERE file_path != '' AND last_scan_mtime IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cache := make(map[string]ScanCacheEntry)
	for rows.Next() {
		var path string
		var entry ScanCacheEntry
		if err := rows.Scan(&path, &entry.Mtime, &entry.Size, &entry.NeedsRescan); err != nil {
			return nil, err
		}
		cache[path] = entry
	}
	return cache, rows.Err()
}
```

- [ ] **Step 2: Implement UpdateScanCache**

```go
func (s *SQLiteStore) UpdateScanCache(bookID string, mtime int64, size int64) error {
	_, err := s.db.Exec(`UPDATE books SET last_scan_mtime = ?, last_scan_size = ?, needs_rescan = 0 WHERE id = ?`, mtime, size, bookID)
	return err
}
```

- [ ] **Step 3: Implement MarkNeedsRescan**

```go
func (s *SQLiteStore) MarkNeedsRescan(bookID string) error {
	_, err := s.db.Exec(`UPDATE books SET needs_rescan = 1 WHERE id = ?`, bookID)
	return err
}
```

- [ ] **Step 4: Implement GetDirtyBookFolders**

Returns distinct parent directories of books marked `needs_rescan`.

```go
func (s *SQLiteStore) GetDirtyBookFolders() ([]string, error) {
	// SQLite doesn't have a dirname function, so we fetch paths and extract dirs in Go
	rows, err := s.db.Query(`SELECT DISTINCT file_path FROM books WHERE needs_rescan = 1 AND file_path != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	dirSet := make(map[string]bool)
	for rows.Next() {
		var fp string
		if err := rows.Scan(&fp); err != nil {
			return nil, err
		}
		dirSet[filepath.Dir(fp)] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	dirs := make([]string, 0, len(dirSet))
	for d := range dirSet {
		dirs = append(dirs, d)
	}
	return dirs, nil
}
```

Note: Add `"path/filepath"` to imports if not already present.

- [ ] **Step 5: Build and verify**

Run: `GOEXPERIMENT=jsonv2 go build ./internal/database/...`
Expected: Build fails only on PebbleStore + MockStore (not SQLiteStore).

- [ ] **Step 6: Commit**

```bash
git add internal/database/sqlite_store.go
git commit -m "feat(db): SQLite scan cache method implementations"
```

---

### Task 4: PebbleDB Implementation

**Files:**
- Modify: `internal/database/pebble_store.go`

- [ ] **Step 1: Implement GetScanCacheMap**

PebbleDB stores books as JSON under `book:` prefix keys. The scan cache fields are embedded in the book JSON. Iterate all books and extract the fields.

```go
func (p *PebbleStore) GetScanCacheMap() (map[string]ScanCacheEntry, error) {
	cache := make(map[string]ScanCacheEntry)
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("book:"),
		UpperBound: []byte("book;"),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		var book Book
		if err := json.Unmarshal(iter.Value(), &book); err != nil {
			continue
		}
		if book.FilePath == "" || book.LastScanMtime == nil {
			continue
		}
		cache[book.FilePath] = ScanCacheEntry{
			Mtime:       *book.LastScanMtime,
			Size:        derefInt64(book.LastScanSize),
			NeedsRescan: derefBool(book.NeedsRescan),
		}
	}
	return cache, nil
}
```

Note: The `Book` struct in `store.go` will need `LastScanMtime *int64`, `LastScanSize *int64`, and `NeedsRescan *bool` fields. Add them in Task 2 alongside the interface changes, or add them here — check if they already exist from the migration. If the PebbleStore `Book` struct doesn't have these fields yet, add JSON-tagged fields:

```go
LastScanMtime *int64 `json:"last_scan_mtime,omitempty"`
LastScanSize  *int64 `json:"last_scan_size,omitempty"`
NeedsRescan   *bool  `json:"needs_rescan,omitempty"`
```

- [ ] **Step 2: Implement UpdateScanCache**

```go
func (p *PebbleStore) UpdateScanCache(bookID string, mtime int64, size int64) error {
	book, err := p.GetBook(bookID)
	if err != nil {
		return err
	}
	book.LastScanMtime = &mtime
	book.LastScanSize = &size
	f := false
	book.NeedsRescan = &f
	return p.putBook(book)
}
```

- [ ] **Step 3: Implement MarkNeedsRescan**

```go
func (p *PebbleStore) MarkNeedsRescan(bookID string) error {
	book, err := p.GetBook(bookID)
	if err != nil {
		return err
	}
	t := true
	book.NeedsRescan = &t
	return p.putBook(book)
}
```

- [ ] **Step 4: Implement GetDirtyBookFolders**

```go
func (p *PebbleStore) GetDirtyBookFolders() ([]string, error) {
	dirSet := make(map[string]bool)
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("book:"),
		UpperBound: []byte("book;"),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		var book Book
		if err := json.Unmarshal(iter.Value(), &book); err != nil {
			continue
		}
		if book.NeedsRescan != nil && *book.NeedsRescan && book.FilePath != "" {
			dirSet[filepath.Dir(book.FilePath)] = true
		}
	}

	dirs := make([]string, 0, len(dirSet))
	for d := range dirSet {
		dirs = append(dirs, d)
	}
	return dirs, nil
}
```

- [ ] **Step 5: Add helper functions if needed**

```go
func derefInt64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

func derefBool(p *bool) bool {
	if p == nil {
		return false
	}
	return *p
}
```

Check if these already exist in the file before adding.

- [ ] **Step 6: Build and verify**

Run: `GOEXPERIMENT=jsonv2 go build ./internal/database/...`
Expected: Build fails only on MockStore.

- [ ] **Step 7: Commit**

```bash
git add internal/database/pebble_store.go
git commit -m "feat(db): PebbleDB scan cache method implementations"
```

---

### Task 5: Mock Implementations

**Files:**
- Modify: `internal/database/mock_store.go`
- Modify: `internal/database/mocks/mock_store.go`

- [ ] **Step 1: Add to manual MockStore (mock_store.go)**

Add function fields and method implementations following the existing pattern:

```go
// Add to MockStore struct fields:
GetScanCacheMapFunc    func() (map[string]ScanCacheEntry, error)
UpdateScanCacheFunc    func(bookID string, mtime int64, size int64) error
MarkNeedsRescanFunc    func(bookID string) error
GetDirtyBookFoldersFunc func() ([]string, error)

// Add method implementations:
func (m *MockStore) GetScanCacheMap() (map[string]ScanCacheEntry, error) {
	if m.GetScanCacheMapFunc != nil {
		return m.GetScanCacheMapFunc()
	}
	return nil, nil
}

func (m *MockStore) UpdateScanCache(bookID string, mtime int64, size int64) error {
	if m.UpdateScanCacheFunc != nil {
		return m.UpdateScanCacheFunc(bookID, mtime, size)
	}
	return nil
}

func (m *MockStore) MarkNeedsRescan(bookID string) error {
	if m.MarkNeedsRescanFunc != nil {
		return m.MarkNeedsRescanFunc(bookID)
	}
	return nil
}

func (m *MockStore) GetDirtyBookFolders() ([]string, error) {
	if m.GetDirtyBookFoldersFunc != nil {
		return m.GetDirtyBookFoldersFunc()
	}
	return nil, nil
}
```

- [ ] **Step 2: Add to testify MockStore (mocks/mock_store.go)**

Follow the existing pattern in that file (uses `mock.Called()`):

```go
func (_mock *MockStore) GetScanCacheMap() (map[string]database.ScanCacheEntry, error) {
	ret := _mock.Called()
	var r0 map[string]database.ScanCacheEntry
	if ret.Get(0) != nil {
		r0 = ret.Get(0).(map[string]database.ScanCacheEntry)
	}
	return r0, ret.Error(1)
}

func (_mock *MockStore) UpdateScanCache(bookID string, mtime int64, size int64) error {
	ret := _mock.Called(bookID, mtime, size)
	return ret.Error(0)
}

func (_mock *MockStore) MarkNeedsRescan(bookID string) error {
	ret := _mock.Called(bookID)
	return ret.Error(0)
}

func (_mock *MockStore) GetDirtyBookFolders() ([]string, error) {
	ret := _mock.Called()
	var r0 []string
	if ret.Get(0) != nil {
		r0 = ret.Get(0).([]string)
	}
	return r0, ret.Error(1)
}
```

Also add the Expecter methods following the existing pattern in the file (each method gets a `_Call` type with `Run`, `Return`, `RunAndReturn`).

- [ ] **Step 3: Build and verify**

Run: `GOEXPERIMENT=jsonv2 go build ./...`
Expected: Clean build — all Store implementations now satisfy the interface.

- [ ] **Step 4: Run tests**

Run: `GOEXPERIMENT=jsonv2 go test ./internal/database/... -count=1 -timeout 60s`
Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/database/mock_store.go internal/database/mocks/mock_store.go
git commit -m "feat(db): mock implementations for scan cache methods"
```

---

### Task 6: Book Struct Fields

**Files:**
- Modify: `internal/database/store.go`

- [ ] **Step 1: Add scan cache fields to the Book struct**

Find the `Book` struct in `store.go` and add:

```go
LastScanMtime *int64 `json:"last_scan_mtime,omitempty"`
LastScanSize  *int64 `json:"last_scan_size,omitempty"`
NeedsRescan   *bool  `json:"needs_rescan,omitempty"`
```

- [ ] **Step 2: Build and run tests**

Run: `GOEXPERIMENT=jsonv2 go build ./... && GOEXPERIMENT=jsonv2 go test ./internal/database/... -count=1 -timeout 60s`
Expected: Clean build and all tests pass.

- [ ] **Step 3: Commit**

```bash
git add internal/database/store.go
git commit -m "feat(db): add scan cache fields to Book struct"
```

---

## Chunk 2: Single-Pass File I/O

### Task 7: ProcessFile — Single-Pass Function

**Files:**
- Create: `internal/scanner/process_file.go`
- Create: `internal/scanner/process_file_test.go`

- [ ] **Step 1: Write tests for ProcessFile**

Create `internal/scanner/process_file_test.go`:

```go
// file: internal/scanner/process_file_test.go
// version: 1.0.0
// guid: <generate>

package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProcessFile_NonexistentFile(t *testing.T) {
	_, _, _, err := ProcessFile("/nonexistent/file.mp3")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestProcessFile_EmptyPath(t *testing.T) {
	_, _, _, err := ProcessFile("")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestProcessFile_ValidFile(t *testing.T) {
	// Find a test fixture if available, otherwise skip
	testFiles, _ := filepath.Glob("../../testdata/*.mp3")
	if len(testFiles) == 0 {
		testFiles, _ = filepath.Glob("../../testdata/*.m4b")
	}
	if len(testFiles) == 0 {
		t.Skip("no test audio files found in testdata/")
	}

	meta, info, hash, err := ProcessFile(testFiles[0])
	if err != nil {
		t.Fatalf("ProcessFile failed: %v", err)
	}
	if meta == nil {
		t.Fatal("expected non-nil metadata")
	}
	if info == nil {
		t.Fatal("expected non-nil mediainfo")
	}
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}
}

func TestProcessFile_Directory(t *testing.T) {
	// ProcessFile should handle directories gracefully (return metadata from dir parsing)
	dir := t.TempDir()
	_, _, _, err := ProcessFile(dir)
	// Should not panic; may return error or empty results
	_ = err
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOEXPERIMENT=jsonv2 go test ./internal/scanner/ -run TestProcessFile -count=1 -timeout 30s`
Expected: FAIL — `ProcessFile` undefined.

- [ ] **Step 3: Implement ProcessFile**

Create `internal/scanner/process_file.go`:

```go
// file: internal/scanner/process_file.go
// version: 1.0.0
// guid: <generate>

package scanner

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"github.com/dhowden/tag"
	"github.com/jdfalk/audiobook-organizer/internal/mediainfo"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
)

// ProcessFile opens a file once and extracts metadata, media info, and file hash
// in a single pass. This eliminates the redundant file opens that occur when
// calling ExtractMetadata, mediainfo.Extract, and ComputeFileHash separately.
//
// Returns: (metadata, mediainfo, sha256hex, error)
func ProcessFile(filePath string) (*metadata.Metadata, *mediainfo.MediaInfo, string, error) {
	if filePath == "" {
		return nil, nil, "", fmt.Errorf("empty file path")
	}

	info, err := os.Stat(filePath)
	if err != nil {
		return nil, nil, "", fmt.Errorf("cannot stat %s: %w", filePath, err)
	}

	// For directories, fall back to metadata-only extraction (no hash, no mediainfo)
	if info.IsDir() {
		meta, metaErr := metadata.ExtractMetadata(filePath)
		if metaErr != nil {
			return nil, nil, "", metaErr
		}
		return &meta, nil, "", nil
	}

	f, err := os.Open(filePath)
	if err != nil {
		return nil, nil, "", fmt.Errorf("cannot open %s: %w", filePath, err)
	}
	defer f.Close()

	// Phase 1: Read tags (one call extracts both metadata and mediainfo fields)
	tagMeta, tagErr := tag.ReadFrom(f)

	// Extract metadata from tags
	var meta metadata.Metadata
	if tagErr == nil && tagMeta != nil {
		meta = metadata.BuildMetadataFromTag(tagMeta, filePath)
	} else {
		// Fall back to filename-based extraction
		meta, _ = metadata.ExtractMetadataFromPath(filePath)
		meta.UsedFilenameFallback = true
	}

	// Extract media info from the same tag read
	var mi *mediainfo.MediaInfo
	if tagErr == nil && tagMeta != nil {
		mi = mediainfo.BuildFromTag(tagMeta, filePath, info.Size())
	}

	// Phase 2: Compute file hash
	// Seek back to start for hashing
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return &meta, mi, "", fmt.Errorf("seek failed: %w", err)
	}

	fileSize := info.Size()
	var hash string

	if fileSize > 100*1024*1024 {
		// Large file: hash first 10MB + last 10MB + file size
		hash, err = hashLargeFile(f, fileSize)
	} else {
		// Small file: full SHA256
		h := sha256.New()
		if _, err = io.Copy(h, f); err == nil {
			hash = hex.EncodeToString(h.Sum(nil))
		}
	}
	if err != nil {
		// Non-fatal: return metadata without hash
		return &meta, mi, "", nil
	}

	return &meta, mi, hash, nil
}

// hashLargeFile hashes first 10MB + last 10MB + file size for large files.
// This matches the behavior of ComputeFileHash for files > 100MB.
func hashLargeFile(f *os.File, fileSize int64) (string, error) {
	const chunkSize = 10 * 1024 * 1024
	h := sha256.New()

	// Hash first 10MB
	buf := make([]byte, chunkSize)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.ErrUnexpectedEOF {
		return "", err
	}
	h.Write(buf[:n])

	// Hash last 10MB
	if fileSize > chunkSize {
		offset := fileSize - chunkSize
		if offset < chunkSize {
			offset = chunkSize // don't overlap with first chunk
		}
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return "", err
		}
		n, err = io.ReadFull(f, buf)
		if err != nil && err != io.ErrUnexpectedEOF {
			return "", err
		}
		h.Write(buf[:n])
	}

	// Include file size in hash to differentiate truncated files
	h.Write([]byte(fmt.Sprintf("%d", fileSize)))

	return hex.EncodeToString(h.Sum(nil)), nil
}
```

- [ ] **Step 4: Add helper functions to metadata and mediainfo packages**

The `ProcessFile` function needs two new helpers that extract data from an already-parsed `tag.Metadata` instead of re-opening the file.

In `internal/metadata/metadata.go`, add:

```go
// BuildMetadataFromTag constructs a Metadata struct from a pre-parsed tag.Metadata.
// This avoids re-opening the file when tags have already been read.
func BuildMetadataFromTag(t tag.Metadata, filePath string) Metadata {
	// Extract the same fields as ExtractMetadata, but from the already-parsed tag
	// ... (reuse the tag-reading logic from ExtractMetadata lines ~146-400)
}

// ExtractMetadataFromPath extracts metadata purely from the file path/name.
func ExtractMetadataFromPath(filePath string) (Metadata, error) {
	// ... (reuse the filename fallback logic)
}
```

In `internal/mediainfo/mediainfo.go`, add:

```go
// BuildFromTag constructs MediaInfo from a pre-parsed tag.Metadata.
// This avoids re-opening the file when tags have already been read.
func BuildFromTag(t tag.Metadata, filePath string, fileSize int64) *MediaInfo {
	// Extract the same fields as Extract, but from the already-parsed tag
	// ... (reuse the tag-based extraction logic from Extract)
}
```

**Important:** These functions should factor out the existing tag-reading logic from `ExtractMetadata` and `Extract` respectively. The original functions should then call the new helpers internally to avoid code duplication. This is the key refactoring step.

- [ ] **Step 5: Run tests**

Run: `GOEXPERIMENT=jsonv2 go test ./internal/scanner/ -run TestProcessFile -count=1 -timeout 30s`
Expected: All ProcessFile tests pass.

- [ ] **Step 6: Run full test suite**

Run: `GOEXPERIMENT=jsonv2 go test ./internal/scanner/... ./internal/metadata/... ./internal/mediainfo/... -count=1 -timeout 60s`
Expected: All tests pass — existing behavior unchanged.

- [ ] **Step 7: Commit**

```bash
git add internal/scanner/process_file.go internal/scanner/process_file_test.go internal/metadata/metadata.go internal/mediainfo/mediainfo.go
git commit -m "feat(scanner): single-pass ProcessFile combining tags, mediainfo, and hash"
```

---

## Chunk 3: Incremental Scan Integration

### Task 8: Integrate ProcessFile into ProcessBooksParallel

**Files:**
- Modify: `internal/scanner/scanner.go`

- [ ] **Step 1: Replace three separate calls with ProcessFile**

In `ProcessBooksParallel` (line 216), inside the worker goroutine (lines 281-440), replace the separate calls to `metadata.ExtractMetadata` (line 336), `mediainfo.Extract` (line 382), and the hash computation in `saveBookToDatabase` with a single `ProcessFile` call.

The worker body should become:

```go
// For non-generic-part files, use single-pass ProcessFile
meta, mi, fileHash, pfErr := ProcessFile(filePath)
if pfErr != nil {
    fmt.Printf("Warning: Could not process %s: %v\n", filePath, pfErr)
    fallbackUsed = true
} else {
    // Apply metadata fields (same logic as before)
    if meta != nil {
        fallbackUsed = meta.UsedFilenameFallback
        // ... apply title, author, narrator, etc. from meta
    }
    // Apply mediainfo (same logic as before)
    if mi != nil {
        // ... apply format, duration from mi
    }
    // Store hash for saveBookToDatabase to use
    books[idx].FileHash = fileHash
}
```

Note: The `Book` struct needs a `FileHash string` field added so the hash can flow through to `saveBookToDatabase` without re-computing it. Check if it already has one.

Keep the `IsGenericPartFilename` branch as-is — that path uses `AssembleBookMetadata` which has different logic.

- [ ] **Step 2: Update saveBookToDatabase to use pre-computed hash**

In `saveBookToDatabase` (line 806), check if `book.FileHash` is already set. If so, skip calling `ComputeFileHash`:

```go
fileHash := book.FileHash
if fileHash == "" {
    var err error
    fileHash, err = ComputeFileHash(book.FilePath)
    if err != nil {
        // ... existing error handling
    }
}
```

- [ ] **Step 3: Build and test**

Run: `GOEXPERIMENT=jsonv2 go build ./... && GOEXPERIMENT=jsonv2 go test ./internal/scanner/... -count=1 -timeout 60s`
Expected: Clean build, all tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/scanner/scanner.go
git commit -m "feat(scanner): use single-pass ProcessFile in worker loop"
```

---

### Task 9: Scan Cache Pre-Load and Skip Logic

**Files:**
- Modify: `internal/server/scan_service.go`
- Modify: `internal/scanner/scanner.go`

- [ ] **Step 1: Add scan cache to ScanService**

Add a `scanCache` field and pre-load method:

```go
type ScanService struct {
	db        database.Store
	scanCache map[string]database.ScanCacheEntry // pre-loaded at scan start
}
```

In `PerformScan`, before scanning folders, pre-load the cache:

```go
// Pre-load scan cache for incremental skip checks
if !forceUpdate {
    cache, err := ss.db.GetScanCacheMap()
    if err != nil {
        _ = progress.Log("warn", fmt.Sprintf("Failed to load scan cache, running full scan: %v", err), nil)
    } else {
        ss.scanCache = cache
        _ = progress.Log("info", fmt.Sprintf("Loaded scan cache with %d entries", len(cache)), nil)
    }
}
```

- [ ] **Step 2: Add dirty folder collection**

After pre-loading the cache, also collect dirty folders:

```go
// Collect dirty book folders for targeted rescan
if !forceUpdate {
    dirtyFolders, err := ss.db.GetDirtyBookFolders()
    if err == nil && len(dirtyFolders) > 0 {
        _ = progress.Log("info", fmt.Sprintf("Found %d folders with dirty books", len(dirtyFolders)), nil)
        // Add dirty folders to the scan list if not already present
        folderSet := make(map[string]bool)
        for _, f := range foldersToScan {
            folderSet[f] = true
        }
        for _, df := range dirtyFolders {
            if !folderSet[df] {
                foldersToScan = append(foldersToScan, df)
                folderSet[df] = true
            }
        }
    }
}
```

- [ ] **Step 3: Pass scan cache to ProcessBooksParallel**

Add a `scanCache` parameter to `ProcessBooksParallel` (or pass it via a new options struct). Inside the worker, before processing a file, check the cache:

```go
// Incremental skip check
if scanCache != nil {
    if entry, found := scanCache[filePath]; found {
        fi, statErr := os.Stat(filePath)
        if statErr == nil && fi.ModTime().Unix() == entry.Mtime && fi.Size() == entry.Size && !entry.NeedsRescan {
            // File unchanged — skip processing
            progressCh <- filePath
            return
        }
    }
}
```

- [ ] **Step 4: Update scan cache after processing**

After `saveBook` succeeds, update the scan cache in the DB:

```go
if err := saveBook(&books[idx]); err != nil {
    errChan <- fmt.Errorf("failed to save book %s: %w", books[idx].FilePath, err)
} else {
    // Update scan cache with current mtime/size
    if database.GlobalStore != nil {
        if fi, statErr := os.Stat(books[idx].FilePath); statErr == nil {
            if dbBook, dbErr := database.GlobalStore.GetBookByFilePath(books[idx].FilePath); dbErr == nil && dbBook != nil {
                _ = database.GlobalStore.UpdateScanCache(dbBook.ID, fi.ModTime().Unix(), fi.Size())
            }
        }
    }
}
```

- [ ] **Step 5: Skip the pre-scan file count for incremental scans**

In `PerformScan`, skip `countFilesAcrossFolders` when doing an incremental scan (it walks the entire tree just for a progress bar estimate, which is wasteful when we're going to skip most files):

```go
var totalFilesAcrossFolders int
if forceUpdate || ss.scanCache == nil {
    totalFilesAcrossFolders = ss.countFilesAcrossFolders(foldersToScan, progress)
} else {
    // For incremental scans, use cached book count as estimate
    totalFilesAcrossFolders = len(ss.scanCache)
    _ = progress.Log("info", fmt.Sprintf("Incremental scan: ~%d known files, checking for changes", totalFilesAcrossFolders), nil)
}
```

- [ ] **Step 6: Build and test**

Run: `GOEXPERIMENT=jsonv2 go build ./... && GOEXPERIMENT=jsonv2 go test ./internal/server/... ./internal/scanner/... -count=1 -timeout 120s`
Expected: Clean build, all tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/server/scan_service.go internal/scanner/scanner.go
git commit -m "feat(scanner): incremental scan with mtime/size cache and dirty folder support"
```

---

### Task 10: Mark needs_rescan From Other Services

**Files:**
- Modify: `internal/server/metadata_fetch_service.go`
- Modify: `internal/server/organize_service.go`

- [ ] **Step 1: Mark needs_rescan after metadata writeback**

In `metadata_fetch_service.go`, in the `writeBackMetadata` method, after successfully writing tags to a file, mark the book for rescan:

```go
// After successful write-back, mark for rescan so next scan picks up new mtime
if err := mfs.db.MarkNeedsRescan(book.ID); err != nil {
    log.Printf("[WARN] failed to mark book %s for rescan: %v", book.ID, err)
}
```

- [ ] **Step 2: Mark needs_rescan after organize/move**

In `organize_service.go`, after successfully moving a book's files, mark the book for rescan:

```go
// After successful file move, mark for rescan
if err := store.MarkNeedsRescan(book.ID); err != nil {
    log.Printf("[WARN] failed to mark book %s for rescan after organize: %v", book.ID, err)
}
```

Find the exact location where file moves complete successfully and add the call there.

- [ ] **Step 3: Build and test**

Run: `GOEXPERIMENT=jsonv2 go build ./... && GOEXPERIMENT=jsonv2 go test ./internal/server/... -count=1 -timeout 120s`
Expected: Clean build, all tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/server/metadata_fetch_service.go internal/server/organize_service.go
git commit -m "feat: mark books needs_rescan after metadata writeback and organize"
```

---

### Task 11: Update Scan Scheduler Task

**Files:**
- Modify: `internal/server/scheduler.go`

- [ ] **Step 1: Update library_scan task description**

Update the `library_scan` task definition to clarify incremental vs full behavior:

```go
Name:        "library_scan",
Description: "Scan library for new/changed audiobooks (incremental by default, use force_update for full rescan)",
```

- [ ] **Step 2: Build and verify**

Run: `GOEXPERIMENT=jsonv2 go build ./...`
Expected: Clean build.

- [ ] **Step 3: Commit**

```bash
git add internal/server/scheduler.go
git commit -m "docs(scheduler): clarify incremental vs full scan behavior"
```

---

### Task 12: Integration Test

**Files:**
- Create: `internal/scanner/incremental_test.go`

- [ ] **Step 1: Write integration test for incremental skip**

```go
// file: internal/scanner/incremental_test.go
// version: 1.0.0
// guid: <generate>

package scanner

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

func TestIncrementalSkip(t *testing.T) {
	// Verify that a file with matching mtime+size in the cache is skipped
	cache := map[string]database.ScanCacheEntry{
		"/fake/path/book.mp3": {
			Mtime:       1234567890,
			Size:        1048576,
			NeedsRescan: false,
		},
	}

	// File matches cache — should skip
	skipped := shouldSkipFile("/fake/path/book.mp3", 1234567890, 1048576, cache)
	if !skipped {
		t.Error("expected file to be skipped when mtime+size match cache")
	}

	// File mtime changed — should NOT skip
	skipped = shouldSkipFile("/fake/path/book.mp3", 1234567891, 1048576, cache)
	if skipped {
		t.Error("expected file to be processed when mtime changed")
	}

	// File size changed — should NOT skip
	skipped = shouldSkipFile("/fake/path/book.mp3", 1234567890, 2097152, cache)
	if skipped {
		t.Error("expected file to be processed when size changed")
	}

	// File not in cache — should NOT skip
	skipped = shouldSkipFile("/fake/path/other.mp3", 1234567890, 1048576, cache)
	if skipped {
		t.Error("expected file to be processed when not in cache")
	}

	// File in cache but needs_rescan — should NOT skip
	cache["/fake/path/dirty.mp3"] = database.ScanCacheEntry{
		Mtime:       1234567890,
		Size:        1048576,
		NeedsRescan: true,
	}
	skipped = shouldSkipFile("/fake/path/dirty.mp3", 1234567890, 1048576, cache)
	if skipped {
		t.Error("expected file to be processed when needs_rescan is true")
	}
}
```

- [ ] **Step 2: Extract shouldSkipFile helper**

In `scanner.go`, extract the skip logic into a testable function:

```go
// shouldSkipFile checks if a file can be skipped based on the scan cache.
func shouldSkipFile(filePath string, mtime int64, size int64, cache map[string]database.ScanCacheEntry) bool {
	if cache == nil {
		return false
	}
	entry, found := cache[filePath]
	if !found {
		return false
	}
	return entry.Mtime == mtime && entry.Size == size && !entry.NeedsRescan
}
```

- [ ] **Step 3: Run tests**

Run: `GOEXPERIMENT=jsonv2 go test ./internal/scanner/ -run TestIncremental -count=1 -timeout 30s`
Expected: All tests pass.

- [ ] **Step 4: Run full test suite**

Run: `GOEXPERIMENT=jsonv2 go test ./... -count=1 -timeout 300s`
Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/scanner/incremental_test.go internal/scanner/scanner.go
git commit -m "test(scanner): integration tests for incremental scan skip logic"
```
