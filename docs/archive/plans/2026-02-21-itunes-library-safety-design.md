<!-- file: docs/plans/2026-02-21-itunes-library-safety-design.md -->
<!-- version: 1.0.0 -->
<!-- guid: a4b5c6d7-e8f9-0a1b-2c3d-4e5f6a7b8c9d -->

# iTunes Library Safety — Design Document

**Goal:** Prevent silent data loss when writing back to iTunes Library.xml by detecting external modifications, and notify users in real-time when the library file changes externally.

**Two features:**
1. **Write-back safety** — Before modifying Library.xml, verify it hasn't changed since last read. Prevents overwriting iTunes changes.
2. **Real-time change detection** — Watch the library file with fsnotify and expose status via API. Frontend shows a banner when library has changed.

---

## 1. Library File Fingerprint

Core primitive both features use:

```go
// internal/itunes/fingerprint.go
type LibraryFingerprint struct {
    Path      string    // absolute path to Library.xml
    Size      int64     // file size in bytes
    ModTime   time.Time // filesystem mtime
    CRC32     uint32    // fast checksum (CRC32 not SHA256 — 50ms vs 500ms for large files)
}
```

- CRC32 chosen for speed — Library.xml can be 50-200MB
- Fingerprint captured at import time, stored in DB
- Also computed just before write-back for comparison

## 2. Write-Back Safety

### Flow

```
WriteBack(opts) {
    if !opts.ForceOverwrite {
        stored := db.GetLibraryFingerprint(opts.LibraryPath)
        current := ComputeFingerprint(opts.LibraryPath)
        if stored != nil && !stored.Matches(current) {
            return ErrLibraryModified{Stored: stored, Current: current}
        }
    }
    // ... existing backup + parse + update + write logic ...
    // After successful write, update stored fingerprint
    newFP := ComputeFingerprint(opts.LibraryPath)
    db.SaveLibraryFingerprint(newFP)
}
```

### API Behavior

- Normal write-back returns **409 Conflict** if library changed:
  ```json
  {
    "error": "library_modified",
    "message": "iTunes library has been modified since last import",
    "details": {
      "stored_size": 12345678,
      "current_size": 12345999,
      "stored_mtime": "2026-02-20T10:00:00Z",
      "current_mtime": "2026-02-21T14:30:00Z"
    }
  }
  ```
- Client can retry with `"force_overwrite": true` to skip the check
- Frontend shows confirmation dialog on 409: "Library changed since import. Overwrite anyway?"

### WriteBackOptions Change

```go
type WriteBackOptions struct {
    LibraryPath    string
    Updates        []*WriteBackUpdate
    CreateBackup   bool
    BackupPath     string
    ForceOverwrite bool               // NEW: skip fingerprint check
    Store          database.Store     // NEW: for fingerprint load/save
}
```

## 3. Real-Time Change Detection (fsnotify)

Since the library file is local to the server, use fsnotify for instant detection.

### Server Integration

- On startup (if iTunes library path is configured), start watching the file
- On fsnotify event (Write/Create/Rename), set `libraryChanged = true` flag with timestamp
- Flag is reset when a new import or write-back completes successfully

### API Endpoint

`GET /api/v1/itunes/library-status`

```json
{
  "path": "/Users/user/Music/iTunes/iTunes Music Library.xml",
  "configured": true,
  "changed_since_import": true,
  "last_imported": "2026-02-20T10:00:00Z",
  "last_external_change": "2026-02-21T14:30:00Z",
  "fingerprint_stored": true
}
```

### Frontend

- Dashboard or Settings page checks library status
- Shows banner: "iTunes library has been modified since last import — consider re-importing"
- Banner dismissable, reappears on next change

## 4. DB Schema

New table (added via migration):

```sql
CREATE TABLE IF NOT EXISTS itunes_library_state (
    path        TEXT PRIMARY KEY,
    size        INTEGER NOT NULL,
    mod_time    TEXT NOT NULL,
    crc32       INTEGER NOT NULL,
    updated_at  TEXT NOT NULL
);
```

Single row per library path. Updated on import completion and write-back completion.

## 5. Store Interface Additions

```go
type Store interface {
    // ... existing methods ...
    SaveLibraryFingerprint(path string, size int64, modTime time.Time, crc32 uint32) error
    GetLibraryFingerprint(path string) (*LibraryFingerprint, error)
}
```

## 6. Files Changed

| File | Change |
|------|--------|
| `internal/itunes/fingerprint.go` | New file: `LibraryFingerprint` struct, `ComputeFingerprint()`, `ErrLibraryModified` |
| `internal/itunes/writeback.go` | Add `ForceOverwrite`+`Store` to opts, pre-write fingerprint check, post-write save |
| `internal/database/migrations.go` | Add `itunes_library_state` table migration |
| `internal/database/store.go` | Add `SaveLibraryFingerprint`, `GetLibraryFingerprint` to interface |
| `internal/database/sqlite_store.go` | Implement fingerprint methods |
| `internal/database/mocks/mock_store.go` | Add mock methods for fingerprint |
| `internal/server/itunes.go` | Save fingerprint on import, 409 on write-back conflict, library-status endpoint |
| `internal/server/server.go` | fsnotify watcher goroutine for library file |
| `web/src/services/api.ts` | `getLibraryStatus()` API call |
| `web/src/pages/Settings.tsx` | "Library changed" banner component |
| Tests for all of the above |

## 7. Estimated Effort

~4-6 hours implementation + testing.

## 8. Edge Cases

- **No prior fingerprint**: First write-back before any import — skip check (no stored baseline)
- **File deleted**: fsnotify fires Remove event — mark as changed, write-back should fail with "file not found"
- **Symlinks**: `os.Stat` follows symlinks, which is correct behavior
- **Concurrent write-backs**: Backup + CRC check makes this safe; second write-back would see changed CRC from first
