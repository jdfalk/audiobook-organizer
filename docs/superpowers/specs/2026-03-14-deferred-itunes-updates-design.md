# Deferred iTunes Updates After M4B Transcode

**Date:** 2026-03-14
**Status:** Approved

## Problem

When an MP3 audiobook sourced from iTunes is transcoded to M4B and iTunes write-back is disabled, iTunes still points at the MP3. The user's iTunes/Books.app continues playing the old format. Without intervention, enabling write-back later has no memory of which books were transcoded and need their iTunes location updated.

## Solution

Store deferred iTunes update records when a transcode completes with write-back disabled. Apply them automatically when write-back is later enabled (during the next iTunes sync). Show a warning banner on affected books.

## Components

### 1. Database: `deferred_itunes_updates` table

New migration adds:

```sql
CREATE TABLE IF NOT EXISTS deferred_itunes_updates (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id TEXT NOT NULL,
    persistent_id TEXT NOT NULL,
    old_path TEXT NOT NULL,
    new_path TEXT NOT NULL,
    update_type TEXT NOT NULL DEFAULT 'transcode',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    applied_at DATETIME
);
CREATE INDEX idx_deferred_itunes_pending ON deferred_itunes_updates(applied_at) WHERE applied_at IS NULL;
```

- `book_id`: the original MP3 book's ID (for linking back to the book detail)
- `persistent_id`: hex-encoded 8-byte iTunes persistent ID from the book record
- `old_path`: original MP3 file path
- `new_path`: new M4B file path
- `update_type`: "transcode" (extensible for organize/rename scenarios later)
- `applied_at`: NULL while pending, set to current time when applied

### 2. Store Interface

Add to `database.Store`:

```go
CreateDeferredITunesUpdate(bookID, persistentID, oldPath, newPath, updateType string) error
GetPendingDeferredITunesUpdates() ([]DeferredITunesUpdate, error)
MarkDeferredITunesUpdateApplied(id int) error
GetDeferredITunesUpdatesByBookID(bookID string) ([]DeferredITunesUpdate, error)
```

New struct:

```go
type DeferredITunesUpdate struct {
    ID           int
    BookID       string
    PersistentID string
    OldPath      string
    NewPath      string
    UpdateType   string
    CreatedAt    time.Time
    AppliedAt    *time.Time
}
```

Implement on SQLiteStore, PebbleStore, MockStore, mocks.MockStore, and stubStore.

### 3. Post-Transcode Hook

In `server.go` `startTranscode` handler's post-transcode callback (around line 5520), after creating the M4B book record:

```
if !config.AppConfig.ITLWriteBackEnabled && originalBook.ITunesPersistentID != "" {
    store.CreateDeferredITunesUpdate(
        originalBook.ID,
        originalBook.ITunesPersistentID,
        originalBook.FilePath,
        newM4BBook.FilePath,
        "transcode",
    )
}
```

The operation result message should include a note when a deferred update is created: "M4B created. iTunes library update deferred until write-back is enabled."

### 4. Auto-Apply on iTunes Sync

In `itunes.go` `executeITunesSync()`, near the top (after the fingerprint check but before processing tracks):

```
if config.AppConfig.ITLWriteBackEnabled {
    pending, _ := store.GetPendingDeferredITunesUpdates()
    if len(pending) > 0 {
        updates := make([]itunes.ITLLocationUpdate, len(pending))
        for i, p := range pending {
            updates[i] = itunes.ITLLocationUpdate{
                PersistentID: p.PersistentID,
                NewLocation:  p.NewPath,
            }
        }
        result, err := itunes.UpdateITLLocations(itlPath, itlPath+".tmp", updates)
        if err == nil {
            os.Rename(itlPath+".tmp", itlPath)  // atomic replace
            for _, p := range pending {
                store.MarkDeferredITunesUpdateApplied(p.ID)
            }
            log.Printf("[INFO] Applied %d deferred iTunes updates", result.UpdatedCount)
        }
    }
}
```

This means deferred updates apply automatically on the next sync after write-back is enabled. No user action required.

### 5. Frontend Warning Banner

**BookDetail page:** When viewing a book that belongs to a version group with a transcoded M4B:

- API endpoint: `GET /api/v1/audiobooks/:id/deferred-itunes-updates` returns pending updates for this book
- If pending updates exist, show an info banner:
  > "iTunes library hasn't been updated with the M4B version. Enable write-back in Settings to sync automatically."
- Banner links to Settings page
- Banner disappears once the deferred update is applied (or if write-back gets enabled and sync runs)

**API response:** Add `has_pending_itunes_update: bool` to the book detail response to avoid an extra API call. The frontend checks this flag and conditionally renders the banner.

### 6. System Status

Add `pending_itunes_updates: int` to the system status response so the dashboard can optionally show "N books waiting for iTunes sync" if the count is > 0.

## Data Flow

```
MP3 Book → Transcode → M4B created
                      ↓
              ITL write-back enabled?
              ├─ Yes → UpdateITLLocations() immediately
              └─ No  → Insert deferred_itunes_updates row
                        + operation result note
                        + BookDetail shows warning banner
                        ↓
              User enables write-back later
                        ↓
              Next iTunes sync runs
                        ↓
              GetPendingDeferredITunesUpdates()
                        ↓
              UpdateITLLocations() with pending rows
                        ↓
              MarkDeferredITunesUpdateApplied()
              Banner disappears
```

## Testing

- **Unit test:** CreateDeferredITunesUpdate + GetPending + MarkApplied round-trip on SQLite
- **Integration test:** Transcode with write-back disabled → verify deferred row created → enable write-back → run sync → verify row marked applied
- **E2E test:** Mock API returns `has_pending_itunes_update: true` → verify banner renders → mock it false → verify banner gone

## Edge Cases

- Book has no `ITunesPersistentID` (not from iTunes): skip deferred update entirely
- User deletes the M4B before sync: deferred update's `new_path` points to missing file — `UpdateITLLocations` will still update the ITL (iTunes will show a missing file indicator, which is correct)
- Multiple transcodes of same book: each creates its own deferred row; only the latest matters since `UpdateITLLocations` uses PersistentID as key (last write wins)
- ITL file is locked/inaccessible: `UpdateITLLocations` returns error, deferred updates stay pending, will retry on next sync
