<!-- file: docs/plans/2026-02-21-itunes-library-safety.md -->
<!-- version: 1.0.0 -->
<!-- guid: b5c6d7e8-f9a0-1b2c-3d4e-5f6a7b8c9d0e -->

# iTunes Library Safety Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Prevent data loss when writing back to iTunes Library.xml by detecting external modifications, and notify users in real-time when the library file changes.

**Architecture:** A `LibraryFingerprint` (CRC32 + size + mtime) is stored in the DB on every import/write-back. Before write-back, the stored fingerprint is compared to the current file state. An fsnotify watcher provides real-time change detection exposed via a new API endpoint.

**Tech Stack:** Go, fsnotify (already in go.mod), CRC32 (stdlib), SQLite/PebbleDB, React/MUI frontend

---

### Task 1: LibraryFingerprint Type and ComputeFingerprint Function

**Files:**
- Create: `internal/itunes/fingerprint.go`
- Create: `internal/itunes/fingerprint_test.go`

**Step 1: Write the failing test**

```go
// file: internal/itunes/fingerprint_test.go
package itunes

import (
	"os"
	"path/filepath"
	"testing"
)

func TestComputeFingerprint(t *testing.T) {
	// Create a temp file with known content
	dir := t.TempDir()
	path := filepath.Join(dir, "test.xml")
	content := []byte("<plist>test content</plist>")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	fp, err := ComputeFingerprint(path)
	if err != nil {
		t.Fatal(err)
	}

	if fp.Path != path {
		t.Errorf("Path = %q, want %q", fp.Path, path)
	}
	if fp.Size != int64(len(content)) {
		t.Errorf("Size = %d, want %d", fp.Size, int64(len(content)))
	}
	if fp.CRC32 == 0 {
		t.Error("CRC32 should not be zero")
	}
	if fp.ModTime.IsZero() {
		t.Error("ModTime should not be zero")
	}
}

func TestComputeFingerprint_FileNotFound(t *testing.T) {
	_, err := ComputeFingerprint("/nonexistent/file.xml")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestLibraryFingerprint_Matches(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.xml")
	if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	fp1, _ := ComputeFingerprint(path)
	fp2, _ := ComputeFingerprint(path)

	if !fp1.Matches(fp2) {
		t.Error("identical fingerprints should match")
	}

	// Modify file
	if err := os.WriteFile(path, []byte("different content"), 0644); err != nil {
		t.Fatal(err)
	}

	fp3, _ := ComputeFingerprint(path)
	if fp1.Matches(fp3) {
		t.Error("different fingerprints should not match")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/itunes/ -run TestComputeFingerprint -v`
Expected: FAIL — `ComputeFingerprint` undefined

**Step 3: Write the implementation**

```go
// file: internal/itunes/fingerprint.go
package itunes

import (
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"time"
)

// LibraryFingerprint captures the state of an iTunes Library.xml file
// for change detection. Uses CRC32 for speed (50ms vs 500ms for SHA256
// on large 100MB+ library files).
type LibraryFingerprint struct {
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
	CRC32   uint32    `json:"crc32"`
}

// ErrLibraryModified is returned when a write-back is attempted but the
// library file has been modified since last import.
type ErrLibraryModified struct {
	Stored  *LibraryFingerprint
	Current *LibraryFingerprint
}

func (e *ErrLibraryModified) Error() string {
	return fmt.Sprintf(
		"iTunes library has been modified externally (size: %d→%d, mtime: %s→%s)",
		e.Stored.Size, e.Current.Size,
		e.Stored.ModTime.Format(time.RFC3339),
		e.Current.ModTime.Format(time.RFC3339),
	)
}

// ComputeFingerprint reads a file and computes its fingerprint (size, mtime, CRC32).
func ComputeFingerprint(path string) (*LibraryFingerprint, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat library file: %w", err)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open library file: %w", err)
	}
	defer f.Close()

	hasher := crc32.NewIEEE()
	if _, err := io.Copy(hasher, f); err != nil {
		return nil, fmt.Errorf("failed to compute CRC32: %w", err)
	}

	return &LibraryFingerprint{
		Path:    path,
		Size:    info.Size(),
		ModTime: info.ModTime(),
		CRC32:   hasher.Sum32(),
	}, nil
}

// Matches returns true if two fingerprints represent the same file state.
// Compares size and CRC32 (mtime can drift on some filesystems).
func (fp *LibraryFingerprint) Matches(other *LibraryFingerprint) bool {
	if fp == nil || other == nil {
		return false
	}
	return fp.Size == other.Size && fp.CRC32 == other.CRC32
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/itunes/ -run TestComputeFingerprint -v && go test ./internal/itunes/ -run TestLibraryFingerprint_Matches -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/itunes/fingerprint.go internal/itunes/fingerprint_test.go
git commit -m "feat(itunes): add LibraryFingerprint type and ComputeFingerprint"
```

---

### Task 2: Store Interface and DB Migration for Library Fingerprints

**Files:**
- Modify: `internal/database/store.go` — add 2 methods to Store interface
- Modify: `internal/database/migrations.go` — add migration 18
- Modify: `internal/database/sqlite_store.go` — implement methods
- Modify: `internal/database/pebble_store.go` — implement methods
- Modify: `internal/database/mocks/mock_store.go` — regenerate or hand-add
- Modify: `cmd/commands_test.go` — add stub methods

**Step 1: Add interface methods to `store.go`**

Add to the `Store` interface (after `GetBlockedHashByHash`):

```go
	// iTunes Library Fingerprints (change detection)
	SaveLibraryFingerprint(path string, size int64, modTime time.Time, crc32 uint32) error
	GetLibraryFingerprint(path string) (*LibraryFingerprintRecord, error)
```

Add the record type (after `DoNotImport` struct):

```go
// LibraryFingerprintRecord stores the last-known state of an iTunes Library.xml file.
type LibraryFingerprintRecord struct {
	Path      string    `json:"path"`
	Size      int64     `json:"size"`
	ModTime   time.Time `json:"mod_time"`
	CRC32     uint32    `json:"crc32"`
	UpdatedAt time.Time `json:"updated_at"`
}
```

**Step 2: Add migration 18 to `migrations.go`**

Add to the `migrations` slice:

```go
{
	Version:     18,
	Description: "Add itunes_library_state table for change detection",
	Up:          migration018Up,
	Down:        nil,
},
```

Add the migration function:

```go
func migration018Up(store Store) error {
	if sqlStore, ok := store.(*SQLiteStore); ok {
		_, err := sqlStore.db.Exec(`
			CREATE TABLE IF NOT EXISTS itunes_library_state (
				path       TEXT PRIMARY KEY,
				size       INTEGER NOT NULL,
				mod_time   TEXT NOT NULL,
				crc32      INTEGER NOT NULL,
				updated_at TEXT NOT NULL
			)
		`)
		return err
	}
	// PebbleDB: no schema needed, uses key-value pairs
	return nil
}
```

**Step 3: Implement in `sqlite_store.go`**

```go
func (s *SQLiteStore) SaveLibraryFingerprint(path string, size int64, modTime time.Time, crc32val uint32) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO itunes_library_state (path, size, mod_time, crc32, updated_at)
		 VALUES (?, ?, ?, ?, ?)`,
		path, size, modTime.Format(time.RFC3339), crc32val, time.Now().Format(time.RFC3339),
	)
	return err
}

func (s *SQLiteStore) GetLibraryFingerprint(path string) (*LibraryFingerprintRecord, error) {
	row := s.db.QueryRow(
		"SELECT path, size, mod_time, crc32, updated_at FROM itunes_library_state WHERE path = ?",
		path,
	)
	var rec LibraryFingerprintRecord
	var modTimeStr, updatedAtStr string
	err := row.Scan(&rec.Path, &rec.Size, &modTimeStr, &rec.CRC32, &updatedAtStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rec.ModTime, _ = time.Parse(time.RFC3339, modTimeStr)
	rec.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)
	return &rec, nil
}
```

**Step 4: Implement in `pebble_store.go`**

```go
func (p *PebbleStore) SaveLibraryFingerprint(path string, size int64, modTime time.Time, crc32val uint32) error {
	rec := LibraryFingerprintRecord{
		Path:      path,
		Size:      size,
		ModTime:   modTime,
		CRC32:     crc32val,
		UpdatedAt: time.Now(),
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	key := []byte(fmt.Sprintf("itunes:fingerprint:%s", path))
	return p.db.Set(key, data, pebble.Sync)
}

func (p *PebbleStore) GetLibraryFingerprint(path string) (*LibraryFingerprintRecord, error) {
	key := []byte(fmt.Sprintf("itunes:fingerprint:%s", path))
	data, closer, err := p.db.Get(key)
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()
	var rec LibraryFingerprintRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}
```

**Step 5: Add stub methods to `cmd/commands_test.go`**

```go
func (s *stubStore) SaveLibraryFingerprint(path string, size int64, modTime time.Time, crc32 uint32) error {
	return nil
}
func (s *stubStore) GetLibraryFingerprint(path string) (*database.LibraryFingerprintRecord, error) {
	return nil, nil
}
```

**Step 6: Regenerate mocks**

Run: `go generate ./internal/database/mocks/...`

If mockery is not available, hand-add the mock methods following the existing pattern (copy `IsHashBlocked` mock pattern, adapt for `SaveLibraryFingerprint` and `GetLibraryFingerprint`).

**Step 7: Run tests**

Run: `go build ./... && go test ./internal/database/... -v -count=1`
Expected: PASS (including migration test)

**Step 8: Commit**

```bash
git add internal/database/store.go internal/database/migrations.go \
  internal/database/sqlite_store.go internal/database/pebble_store.go \
  internal/database/mocks/mock_store.go cmd/commands_test.go
git commit -m "feat(db): add itunes_library_state table and fingerprint Store methods"
```

---

### Task 3: Write-Back Safety Check

**Files:**
- Modify: `internal/itunes/writeback.go` — add fingerprint check before write
- Modify: `internal/itunes/writeback_test.go` (or create if doesn't exist) — test conflict detection

**Step 1: Write the failing test**

Create or add to `internal/itunes/writeback_test.go`:

```go
func TestWriteBack_DetectsModifiedLibrary(t *testing.T) {
	// Setup: create a fake library XML, compute fingerprint, modify file, try write-back
	dir := t.TempDir()
	libPath := filepath.Join(dir, "Library.xml")

	// Write initial library content
	initialContent := buildMinimalLibraryXML()
	os.WriteFile(libPath, initialContent, 0644)

	// Compute and store the "import time" fingerprint
	fp, _ := ComputeFingerprint(libPath)

	// Simulate external modification
	os.WriteFile(libPath, append(initialContent, []byte("<!-- modified -->")...), 0644)

	opts := WriteBackOptions{
		LibraryPath:    libPath,
		Updates:        []*WriteBackUpdate{{ITunesPersistentID: "ABC", NewPath: "/new/path"}},
		ForceOverwrite: false,
		StoredFingerprint: fp,
	}

	_, err := WriteBack(opts)
	if err == nil {
		t.Fatal("expected ErrLibraryModified, got nil")
	}

	var modErr *ErrLibraryModified
	if !errors.As(err, &modErr) {
		t.Fatalf("expected ErrLibraryModified, got %T: %v", err, err)
	}
}

func TestWriteBack_ForceOverwriteSkipsCheck(t *testing.T) {
	dir := t.TempDir()
	libPath := filepath.Join(dir, "Library.xml")
	os.WriteFile(libPath, buildMinimalLibraryXML(), 0644)

	fp, _ := ComputeFingerprint(libPath)
	// Modify file
	os.WriteFile(libPath, []byte("modified"), 0644)

	opts := WriteBackOptions{
		LibraryPath:       libPath,
		Updates:           []*WriteBackUpdate{},
		ForceOverwrite:    true,
		StoredFingerprint: fp,
	}

	// Should not return ErrLibraryModified (may fail for other reasons, that's OK)
	_, err := WriteBack(opts)
	var modErr *ErrLibraryModified
	if errors.As(err, &modErr) {
		t.Fatal("ForceOverwrite should skip fingerprint check")
	}
}
```

Helper function:
```go
func buildMinimalLibraryXML() []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Major Version</key><integer>1</integer>
<key>Minor Version</key><integer>1</integer>
<key>Tracks</key><dict></dict>
<key>Playlists</key><array></array>
</dict></plist>`)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/itunes/ -run TestWriteBack_Detects -v`
Expected: FAIL — `ForceOverwrite` and `StoredFingerprint` undefined

**Step 3: Modify `WriteBackOptions` and `WriteBack` function**

Add to `WriteBackOptions`:
```go
ForceOverwrite    bool               // Skip fingerprint check (user confirmed override)
StoredFingerprint *LibraryFingerprint // Fingerprint from last import (nil = skip check)
```

Add at the top of `WriteBack()`, after validation:
```go
// Check for external modifications (unless force override)
if !opts.ForceOverwrite && opts.StoredFingerprint != nil {
	current, err := ComputeFingerprint(opts.LibraryPath)
	if err != nil {
		return nil, fmt.Errorf("failed to check library state: %w", err)
	}
	if !opts.StoredFingerprint.Matches(current) {
		return nil, &ErrLibraryModified{
			Stored:  opts.StoredFingerprint,
			Current: current,
		}
	}
}
```

**Step 4: Run tests**

Run: `go test ./internal/itunes/ -run TestWriteBack -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/itunes/writeback.go internal/itunes/writeback_test.go
git commit -m "feat(itunes): add fingerprint safety check before write-back"
```

---

### Task 4: Server Integration — Save Fingerprint on Import, 409 on Write-Back Conflict

**Files:**
- Modify: `internal/server/itunes.go` — save fingerprint after import, check before write-back, add library-status endpoint

**Step 1: Save fingerprint after successful import**

In `executeITunesImport()`, after the "Clear checkpoint on successful completion" line (~line 612), add:

```go
// Save library fingerprint for change detection
if fp, err := itunes.ComputeFingerprint(req.LibraryPath); err == nil {
	_ = store.SaveLibraryFingerprint(fp.Path, fp.Size, fp.ModTime, fp.CRC32)
}
```

**Step 2: Add fingerprint check in `handleITunesWriteBack`**

Before calling `itunes.WriteBack(opts)`, load the stored fingerprint and pass it:

```go
// Load stored fingerprint for conflict detection
var storedFP *itunes.LibraryFingerprint
if rec, err := database.GlobalStore.GetLibraryFingerprint(req.LibraryPath); err == nil && rec != nil {
	storedFP = &itunes.LibraryFingerprint{
		Path:    rec.Path,
		Size:    rec.Size,
		ModTime: rec.ModTime,
		CRC32:   rec.CRC32,
	}
}

opts := itunes.WriteBackOptions{
	LibraryPath:       req.LibraryPath,
	Updates:           updates,
	CreateBackup:      req.CreateBackup,
	ForceOverwrite:    req.ForceOverwrite,
	StoredFingerprint: storedFP,
}

result, err := itunes.WriteBack(opts)
if err != nil {
	var modErr *itunes.ErrLibraryModified
	if errors.As(err, &modErr) {
		c.JSON(http.StatusConflict, gin.H{
			"error":         "library_modified",
			"message":       modErr.Error(),
			"stored_size":   modErr.Stored.Size,
			"current_size":  modErr.Current.Size,
			"stored_mtime":  modErr.Stored.ModTime,
			"current_mtime": modErr.Current.ModTime,
		})
		return
	}
	// ... existing error handling
}

// Update fingerprint after successful write-back
if fp, err := itunes.ComputeFingerprint(req.LibraryPath); err == nil {
	_ = database.GlobalStore.SaveLibraryFingerprint(fp.Path, fp.Size, fp.ModTime, fp.CRC32)
}
```

**Step 3: Add `ForceOverwrite` to `ITunesWriteBackRequest`**

```go
type ITunesWriteBackRequest struct {
	LibraryPath    string   `json:"library_path" binding:"required"`
	AudiobookIDs   []string `json:"audiobook_ids"`
	CreateBackup   bool     `json:"create_backup"`
	ForceOverwrite bool     `json:"force_overwrite"` // NEW
}
```

**Step 4: Add library-status endpoint**

Add handler and route:

```go
// In registerRoutes (server.go):
// api.GET("/itunes/library-status", s.handleITunesLibraryStatus)

func (s *Server) handleITunesLibraryStatus(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path query parameter required"})
		return
	}

	rec, err := database.GlobalStore.GetLibraryFingerprint(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := gin.H{
		"path":                 path,
		"configured":           true,
		"fingerprint_stored":   rec != nil,
		"changed_since_import": false,
	}

	if rec != nil {
		response["last_imported"] = rec.UpdatedAt

		// Quick mtime+size check (no CRC32 for polling)
		if info, err := os.Stat(path); err == nil {
			if info.Size() != rec.Size || !info.ModTime().Equal(rec.ModTime) {
				response["changed_since_import"] = true
				response["last_external_change"] = info.ModTime()
			}
		}
	}

	c.JSON(http.StatusOK, response)
}
```

**Step 5: Run build and existing tests**

Run: `go build ./... && go test ./internal/server/... -v -count=1 -timeout 120s`
Expected: Build succeeds, existing tests pass

**Step 6: Commit**

```bash
git add internal/server/itunes.go internal/server/server.go
git commit -m "feat(server): save fingerprint on import, return 409 on write-back conflict"
```

---

### Task 5: fsnotify Watcher for iTunes Library File

**Files:**
- Create: `internal/itunes/library_watcher.go`
- Create: `internal/itunes/library_watcher_test.go`
- Modify: `internal/server/server.go` — start watcher on server startup

**Step 1: Write the failing test**

```go
// file: internal/itunes/library_watcher_test.go
package itunes

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLibraryWatcher_DetectsChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Library.xml")
	os.WriteFile(path, []byte("initial"), 0644)

	w, err := NewLibraryWatcher(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	if w.HasChanged() {
		t.Error("should not report changed before any modification")
	}

	// Modify the file
	time.Sleep(50 * time.Millisecond)
	os.WriteFile(path, []byte("modified content"), 0644)

	// Wait for fsnotify to fire
	time.Sleep(500 * time.Millisecond)

	if !w.HasChanged() {
		t.Error("should report changed after modification")
	}

	// Reset
	w.ClearChanged()
	if w.HasChanged() {
		t.Error("should not report changed after clear")
	}
}
```

**Step 2: Run test to verify failure**

Run: `go test ./internal/itunes/ -run TestLibraryWatcher -v`
Expected: FAIL — `NewLibraryWatcher` undefined

**Step 3: Implement LibraryWatcher**

```go
// file: internal/itunes/library_watcher.go
package itunes

import (
	"log"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// LibraryWatcher monitors an iTunes Library.xml file for external changes.
type LibraryWatcher struct {
	path      string
	watcher   *fsnotify.Watcher
	mu        sync.RWMutex
	changed   bool
	changedAt time.Time
	stop      chan struct{}
}

// NewLibraryWatcher creates a watcher for the given library file path.
func NewLibraryWatcher(path string) (*LibraryWatcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	if err := fsw.Add(path); err != nil {
		fsw.Close()
		return nil, err
	}

	w := &LibraryWatcher{
		path:    path,
		watcher: fsw,
		stop:    make(chan struct{}),
	}

	go w.loop()
	return w, nil
}

func (w *LibraryWatcher) loop() {
	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Rename) {
				w.mu.Lock()
				w.changed = true
				w.changedAt = time.Now()
				w.mu.Unlock()
				log.Printf("iTunes library file changed: %s (op: %s)", w.path, event.Op)
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("iTunes library watcher error: %v", err)
		case <-w.stop:
			return
		}
	}
}

// HasChanged returns true if the file has been modified since last ClearChanged.
func (w *LibraryWatcher) HasChanged() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.changed
}

// ChangedAt returns when the last change was detected.
func (w *LibraryWatcher) ChangedAt() time.Time {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.changedAt
}

// ClearChanged resets the changed flag (call after import/write-back).
func (w *LibraryWatcher) ClearChanged() {
	w.mu.Lock()
	w.changed = false
	w.changedAt = time.Time{}
	w.mu.Unlock()
}

// Close stops watching.
func (w *LibraryWatcher) Close() error {
	close(w.stop)
	return w.watcher.Close()
}
```

**Step 4: Run tests**

Run: `go test ./internal/itunes/ -run TestLibraryWatcher -v`
Expected: PASS

**Step 5: Integrate into server startup**

In `internal/server/server.go`, add to the `Server` struct:

```go
libraryWatcher *itunes.LibraryWatcher
```

In the server startup (after config is loaded), if iTunes library path is configured:

```go
if config.AppConfig.ITunesLibraryPath != "" {
	if w, err := itunes.NewLibraryWatcher(config.AppConfig.ITunesLibraryPath); err == nil {
		s.libraryWatcher = w
		log.Printf("Watching iTunes library: %s", config.AppConfig.ITunesLibraryPath)
	} else {
		log.Printf("Warning: could not watch iTunes library: %v", err)
	}
}
```

Update `handleITunesLibraryStatus` to include watcher state:

```go
if s.libraryWatcher != nil && s.libraryWatcher.HasChanged() {
	response["changed_since_import"] = true
	response["last_external_change"] = s.libraryWatcher.ChangedAt()
}
```

On server shutdown, close the watcher:
```go
if s.libraryWatcher != nil {
	s.libraryWatcher.Close()
}
```

**Step 6: Run all tests**

Run: `go build ./... && go test ./... -count=1 -timeout 120s`
Expected: PASS

**Step 7: Commit**

```bash
git add internal/itunes/library_watcher.go internal/itunes/library_watcher_test.go \
  internal/server/server.go
git commit -m "feat(itunes): add fsnotify watcher for library file change detection"
```

---

### Task 6: Frontend — Library Status Banner and Write-Back Conflict Dialog

**Files:**
- Modify: `web/src/services/api.ts` — add `getLibraryStatus()` and update write-back call
- Modify: `web/src/pages/Settings.tsx` — add library changed banner
- Modify: `web/src/components/settings/ITunesConflictDialog.tsx` — add overwrite confirmation

**Step 1: Add API method**

In `api.ts`, add:

```typescript
getLibraryStatus: async (path: string): Promise<{
  path: string;
  configured: boolean;
  fingerprint_stored: boolean;
  changed_since_import: boolean;
  last_imported?: string;
  last_external_change?: string;
}> => {
  const response = await fetch(`${API_BASE}/itunes/library-status?path=${encodeURIComponent(path)}`);
  return response.json();
},
```

Update the write-back call to handle 409:

```typescript
writeBackToITunes: async (libraryPath: string, audiobookIds: string[], createBackup: boolean, forceOverwrite = false) => {
  const response = await fetch(`${API_BASE}/itunes/write-back`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      library_path: libraryPath,
      audiobook_ids: audiobookIds,
      create_backup: createBackup,
      force_overwrite: forceOverwrite,
    }),
  });
  if (response.status === 409) {
    const data = await response.json();
    throw { type: 'library_modified', ...data };
  }
  if (!response.ok) throw new Error(await response.text());
  return response.json();
},
```

**Step 2: Add library changed banner to Settings**

In the iTunes section of Settings, poll library status and show:

```tsx
{libraryChanged && (
  <Alert severity="warning" sx={{ mb: 2 }}>
    iTunes library has been modified since last import.
    Consider re-importing to pick up changes.
  </Alert>
)}
```

**Step 3: Add overwrite confirmation dialog**

When write-back returns 409, show a dialog:

```tsx
<Dialog open={showOverwriteDialog}>
  <DialogTitle>Library Modified</DialogTitle>
  <DialogContent>
    <Typography>
      The iTunes library has been modified since your last import.
      Writing back now may overwrite those external changes.
    </Typography>
  </DialogContent>
  <DialogActions>
    <Button onClick={() => setShowOverwriteDialog(false)}>Cancel</Button>
    <Button color="warning" onClick={() => retryWithForce()}>
      Overwrite Anyway
    </Button>
  </DialogActions>
</Dialog>
```

**Step 4: Build frontend**

Run: `cd web && npm run build`
Expected: Build succeeds

**Step 5: Commit**

```bash
git add web/src/services/api.ts web/src/pages/Settings.tsx \
  web/src/components/settings/ITunesConflictDialog.tsx
git commit -m "feat(web): add library changed banner and write-back conflict dialog"
```

---

### Task 7: Integration Test and Final Verification

**Files:**
- Create: `internal/itunes/fingerprint_integration_test.go`

**Step 1: Write integration test**

```go
func TestWriteBackSafety_IntegrationFlow(t *testing.T) {
	// 1. Create temp library XML
	// 2. ComputeFingerprint → simulate "import"
	// 3. Modify file (simulate iTunes change)
	// 4. Attempt WriteBack → expect ErrLibraryModified
	// 5. Attempt WriteBack with ForceOverwrite=true → expect success
	// 6. ComputeFingerprint again → verify updated
}
```

**Step 2: Run full test suite**

Run: `make test`
Expected: All tests pass

**Step 3: Run build**

Run: `make build`
Expected: Full build succeeds (frontend + backend)

**Step 4: Commit**

```bash
git add internal/itunes/fingerprint_integration_test.go
git commit -m "test(itunes): add integration test for write-back safety flow"
```
