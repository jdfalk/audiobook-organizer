<!-- file: docs/superpowers/specs/2026-04-29-deluge-protected-paths-design.md -->
<!-- version: 1.0.0 -->
<!-- guid: a3b4c5d6-e7f8-4901-b234-c5d6e7f8a9b0 -->
<!-- last-edited: 2026-04-29 -->

# Deluge Protected Paths — File Ownership & Reflink Workflow

**Status:** Draft — awaiting implementation
**Scope:** All file operations touching Deluge-sourced audiobooks
**Related specs:**
- [`2026-04-27-deluge-move-storage-integration-design.md`](./2026-04-27-deluge-move-storage-integration-design.md) — MoveStorage wiring
- [`2026-04-15-library-centralization-design.md`](./2026-04-15-library-centralization-design.md) — centralization pipeline

---

## Problem

When Deluge downloads an audiobook, the torrent client owns the file. Editing tags
in-place (1) corrupts the torrent piece hashes, breaking seeding; (2) modifies a file
the user did not explicitly hand to us. The same rule already applies to iTunes files —
we never touch them except via the iTunes API.

Deluge files need the same treatment, but with one twist: unlike iTunes (read-only
remote library), we CAN physically move Deluge files — we just have to tell Deluge
about the move via `core.move_storage` so it keeps seeding from the new location.

---

## Core Rule

> **We never edit any file outside our audiobook-organizer library root.**
> We reflink it in, edit the copy, and optionally tell Deluge where it moved.

"Edit" means: writing tags, renaming, any in-place mutation.
"Our library root" means: the path in `config.RootDir` (e.g. `/mnt/bigdata/books/audiobook-organizer`).

---

## What Is a "Protected Path"?

A file path is **protected** if it lives under any Deluge torrent's `save_path`.
The set of protected path prefixes is built at startup (and refreshed on demand)
by calling `core.get_torrents_status` and collecting all `save_path` values.

Additionally, any path explicitly listed in `config.ProtectedPaths` (new config
field, slice of strings) is also protected — this covers iTunes and any other
external library the user wants to fence off.

```go
// config/config.go
type Config struct {
    // ...existing fields...
    ProtectedPaths []string `json:"protected_paths"` // explicit fences beyond Deluge
}
```

The full protected set at runtime is:
```
protectedPaths = config.ProtectedPaths ∪ {all Deluge save_paths}
```

A path `p` is protected if any prefix in the set is a prefix of `p`.

---

## Allowed Operations on Protected Files

| Operation | Allowed? | Mechanism |
|-----------|----------|-----------|
| Read (tags, duration, cover) | ✅ Yes | Direct file read — harmless |
| Move to library root | ✅ Yes | Reflink → update DB → `core.move_storage` (if enabled) |
| Move within Deluge save_path | ✅ Yes | `core.move_storage` only (no reflink needed; still in Deluge's domain) |
| Write tags in-place | ❌ Never | Must bring into library first |
| Rename in-place | ❌ Never | Must bring into library first |
| Delete | ❌ Never | User manages their torrent client |
| Cover embed in-place | ❌ Never | Must bring into library first |

---

## Bringing a Protected File Into the Library ("Import")

When we need to edit a Deluge-sourced file (tag write-back, cover embed, rename):

```
1. Determine canonical library path: <RootDir>/<author>/<series>/<title>/<filename>
2. Reflink: ioctl_ficlone(src=deluge_path, dst=library_path)
   - Falls back to os.Copy if FS doesn't support reflinks (APFS/Btrfs/XFS/ZFS required)
   - The copy is instantaneous and uses no extra disk space until either copy is modified
3. Update book_files.file_path in DB → library_path
4. If DelugeMoveEnabled AND torrent hash is known:
   a. Call Deluge core.move_storage(hash, new_dir=dirname(library_path))
   b. Deluge atomically updates its internal path and continues seeding from library_path
   c. The reflink and the original are now the same data; Deluge re-checks and carries on
5. Proceed with tag writes / cover embed / rename against library_path
```

The result: the user's torrent client continues seeding normally. The file is now
inside our library and we can do whatever we want with it.

---

## Move-Only Workflow (No Tag Edit Needed)

When centralization just needs to relocate a file (no tag changes):

```
1. Reflink src → dst (library path) — or os.Rename if src is already in library root
2. Update DB: book_files.file_path = dst
3. Call core.move_storage(hash, dirname(dst)) — best-effort, log on failure
4. Remove src only if src ≠ dst AND src was already in library root
   (never remove the original Deluge path — Deluge will handle it after move_storage)
```

The distinction from "import" is that here we may or may not need to reflink — if the
file is already in the library root, an `os.Rename` suffices (no Deluge call needed because
we didn't cross a filesystem boundary and the torrent hash is still valid at the new path
inside the library).

---

## Tag Write-Back Guard

`metadata.WriteSingleTag` and `taglib.WriteAudiobookTags` already accept a file path.
Wrap them with a pre-flight check:

```go
// internal/metadata/write.go

func WriteTagsSafe(cfg *config.Config, delugeClient DelugeClient, store database.Store,
    filePath string, tags map[string]string) error {

    if isProtected(cfg, delugeClient, filePath) {
        dst, err := importToLibrary(cfg, delugeClient, store, filePath)
        if err != nil {
            return fmt.Errorf("WriteTagsSafe: import to library failed: %w", err)
        }
        filePath = dst
    }
    return taglib.WriteAudiobookTags(filePath, tags)
}
```

`isProtected` checks the prefix list built from config + Deluge save_paths (cached,
refreshed every 5 minutes or on explicit invalidate).

All call sites that currently call `WriteSingleTag` or `WriteAudiobookTags` must be
migrated to `WriteTagsSafe`. The bulk write-back pipeline
(`internal/server/bulk_write_back.go`) is the primary consumer.

---

## Protected Path Cache

Fetching all Deluge save_paths on every tag write is too slow. Use a short-lived cache:

```go
type protectedPathCache struct {
    mu      sync.RWMutex
    paths   []string
    builtAt time.Time
    ttl     time.Duration // default 5 min
}

func (c *protectedPathCache) IsProtected(p string) bool {
    c.mu.RLock()
    if time.Since(c.builtAt) < c.ttl {
        defer c.mu.RUnlock()
        return hasPrefix(c.paths, p)
    }
    c.mu.RUnlock()
    c.refresh() // acquires write lock, refetches from Deluge
    c.mu.RLock()
    defer c.mu.RUnlock()
    return hasPrefix(c.paths, p)
}
```

If Deluge is unreachable during refresh, keep the stale cache and log a warning —
better to use slightly stale path data than to block every tag write.

---

## DB Tracking

Add to `book_files` table (new migration or extend existing):

```sql
-- Already have: file_path, book_id, missing, etc.
ALTER TABLE book_files ADD COLUMN deluge_hash TEXT;        -- torrent info-hash if sourced from Deluge
ALTER TABLE book_files ADD COLUMN deluge_original_path TEXT; -- original Deluge save path before import
ALTER TABLE book_files ADD COLUMN imported_from_deluge_at TIMESTAMP; -- when it was brought into library
```

PebbleDB: add these fields to `BookFile` struct (`json:"..."` tags); existing records get nil automatically.

`deluge_original_path` is write-once — it's the audit trail proving where the file came from.
`deluge_hash` is the torrent info-hash used to call `move_storage`.

---

## Config

```json
{
  "deluge_url": "http://172.16.2.30:8112",
  "deluge_password": "deluge",
  "deluge_discovery_enabled": true,
  "deluge_move_enabled": false,
  "deluge_discovery_label": "audiobooks",
  "protected_paths": [
    "/mnt/bigdata/books/itunes"
  ]
}
```

`deluge_move_enabled` gates `core.move_storage` calls — off by default. Turning it on
means "I trust the organizer to notify Deluge when it moves files." The reflink import
itself is always safe; only the Deluge notification is gated.

---

## What Does NOT Change

- **Discovery** (listing unimported torrents) — unchanged; we only read Deluge metadata
- **iTunes path protection** — unchanged; iTunes paths are already in `ProtectedPaths`
- **Organize / rename within library** — unchanged for files already inside `RootDir`
- **The torrent itself** — we never call `core.remove_torrent`, never delete `.torrent` files

---

## Implementation Order

1. **DB migration**: add `deluge_hash`, `deluge_original_path`, `imported_from_deluge_at` to `book_files`
2. **`protectedPathCache`**: build the cache, wire into server startup
3. **`importToLibrary`**: reflink + DB update + optional `core.move_storage`
4. **`WriteTagsSafe`**: wrap existing tag writers with the guard
5. **Migrate all call sites** to `WriteTagsSafe` (bulk write-back, single-file write, cover embed)
6. **Wire discovery → import**: when user clicks "Import" on a discovered torrent, run the import flow
7. **UI**: "Imported from Deluge" badge on book detail, original path in Files tab audit row

---

## Open Questions

- **What if the user has already manually moved the Deluge file before we're involved?**
  Deluge would be desynced. We detect this via `book_files.missing = true` on the old path
  and surface it as a "missing file" repair candidate. Out of scope for this spec.

- **What if the FS doesn't support reflinks (e.g., ext4)?**
  Fall back to `os.Copy` — same data, more disk space. Log a one-time warning at startup.
  The correctness guarantee (never edit Deluge originals) still holds.

- **Should `deluge_move_enabled` default to true once the feature is stable?**
  Suggested: yes, after two releases of field testing. Gate the flip behind a CHANGELOG note.
