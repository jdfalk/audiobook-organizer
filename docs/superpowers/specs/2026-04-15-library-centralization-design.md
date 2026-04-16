<!-- file: docs/superpowers/specs/2026-04-15-library-centralization-design.md -->
<!-- version: 1.0.0 -->
<!-- guid: 3f1a8c6d-2b4e-4f80-a9c1-5d2e7b8f0c34 -->

# Library Centralization + Versioning — Design

**Status:** Design complete (Apr 15, 2026). Ready for implementation plan.
**Source:** Brainstorm session Apr 15, 2026 + locked decisions from memory `project_centralization_backlog.md` (Apr 10, 2026).
**Scope item:** TODO.md §3.1 (pairs with §6.1 Deluge integration, partial overlap with §7.9 iTunes regen, §7.10 archive sweep).

## Goal

Centralize all non-iTunes book files into the managed library tree with a structured multi-version model, so every format / quality / source of the same book lives under one book and the user can swap primary, trash, and restore at will. iTunes-sourced files stay where they are (read-only references).

Additionally: when a user purges a version, remember its content fingerprint forever so a re-download (via deluge) can be paused and surfaced for approval, preventing silent re-acquisition of deleted content.

## Locked decisions

### 1. Filesystem layout

```
Author/
  Book Title/
    Book Title.m4b              ← primary file — whichever version is "active"
    .versions/
      {version-ulid-A}/          ← alt version 1
        Book Title.m4b
      {version-ulid-B}/          ← alt version 2 (different format)
        Book 01.mp3
        Book 02.mp3
        …
```

- Primary at the natural path (browsing the library filesystem directly looks normal)
- Alts under `.versions/{ulid}/` (dot prefix hides from default directory listings)
- No per-folder JSON manifest; all version metadata lives in the DB

### 2. Version ID

ULID. Sortable by creation time, stable across content changes (tag/cover rewrites don't rename the dir), 26 chars, already used repo-wide.

### 3. Schema

```sql
CREATE TABLE book_versions (
    id TEXT PRIMARY KEY,            -- ULID, matches .versions/{id}/
    book_id TEXT NOT NULL,
    status TEXT NOT NULL,           -- pending | active | alt | swapping_in | swapping_out
                                    -- trash | inactive_purged | blocked_for_redownload
    format TEXT NOT NULL,           -- m4b | mp3 | flac | …
    source TEXT NOT NULL,           -- deluge | manual | transcoded | imported
    source_original_path TEXT,      -- path at ingest (deluge's original)
    torrent_hash TEXT,              -- infohash, fast-path fingerprint match
    ingest_date TIMESTAMP NOT NULL,
    purged_date TIMESTAMP,
    metadata_json TEXT,             -- catch-all for source-specific extras
    FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX idx_book_versions_single_active
    ON book_versions(book_id) WHERE status = 'active';
CREATE INDEX idx_book_versions_book_id      ON book_versions(book_id);
CREATE INDEX idx_book_versions_torrent_hash ON book_versions(torrent_hash);
CREATE INDEX idx_book_versions_status       ON book_versions(status);
```

`book_files` gets a new column (existing per-file table stays authoritative for file-level data):

```sql
ALTER TABLE book_files
    ADD COLUMN version_id TEXT NULL REFERENCES book_versions(id) ON DELETE CASCADE;
CREATE INDEX idx_book_files_version_id ON book_files(version_id);
CREATE INDEX idx_book_files_file_hash  ON book_files(file_hash);
```

### 4. Primary swap mechanics

Tracked operation `version_swap`, fields `{book_id, from_version_id, to_version_id}`:

1. DB: `from.status = 'swapping_out'`, `to.status = 'swapping_in'`
2. FS: rename every file at natural path into `.versions/{from_id}/` (O(1) on ZFS)
3. FS: rename every file from `.versions/{to_id}/` up to natural path
4. DB: `from = 'alt'`, `to = 'active'`, update affected `book_files.file_path`, update `books.file_path`
5. Deluge: `move_storage` RPC for any version with a non-null `torrent_hash`
6. Close operation (`ClearState`)

Recovery on crash: standard `resumeInterruptedOperations` case. Each FS step checks destination before acting (idempotent). No "stuck in the middle" state.

### 5. Fingerprint identity

Two hashes, both stored:

- `book_versions.torrent_hash` — deluge infohash, free from deluge RPC
- `book_files.file_hash` — our SHA-256 of file contents, computed once at ingest

Piece hashes and BTv2 Merkle roots are explicitly **not** stored — they don't add match power beyond SHA-256 and would force format-specific code paths.

Fingerprint match on re-download:

1. Fast path: infohash against `book_versions` where `status IN ('inactive_purged', 'blocked_for_redownload')`
2. Fallback: hash-set match against `book_files.file_hash` scoped to the same status set

Either match → **pause deluge + surface approval dialog** ("This torrent was previously purged as version X of book Y — add anyway?").

### 6. Delete / restore / purge

Three-state deletion:

- `trash` — user clicked delete. Files stay, hidden from normal UI. 14-day TTL (configurable).
- `inactive_purged` — TTL expired. Background job deleted files. `book_files` rows **kept forever** with `missing=true`, `file_path=null`, `file_hash` preserved — this is the fingerprint record.
- Hard delete — only from the dedicated **Purged view**. Removes `book_versions` + `book_files` rows entirely. The only way to forget a fingerprint.

Deleting the primary auto-promotes the most-recent `alt` (by `ingest_date`) to `active`. If no alt exists, book ends with zero active versions and UI shows "no playable files."

Deleting a whole book: all versions → `trash`, book itself flips `MarkedForDeletion=true` after TTL. Uses the existing soft-delete field on `books`.

### 7. Ingest flow

**A. Deluge completes a torrent**

1. Hook on torrent-complete (RPC event or poll)
2. Fingerprint check (infohash first, then hash-set) against purged/blocked versions
3. If match → pause deluge, surface approval dialog, short-circuit the rest
4. If no match → identify the book (metadata match against existing library)
5. If book exists → new version lands in `.versions/{new-id}/` as `alt` (never auto-promoted unless book has zero active versions)
6. If new book → primary at natural path, `status='active'`, `source='deluge'`, `torrent_hash` set
7. Deluge `move_storage` immediately so seeding continues from the centralized path

**B. Manual file import**

Same pipeline minus the infohash check. SHA-256 fingerprint check still runs.

**C. iTunes import**

Unchanged. Excluded from centralization. Existing `external_id_map` maintains the iTunes ↔ book link.

### 8. UI surface

**BookDetail "Versions" panel** replaces / extends the current Version Group panel:

- One row per `book_versions` row (excluding trash/purged)
- Columns: status dot · format · bitrate · duration · source · ingest_date
- Actions per alt row: **Make primary** (triggers swap op), **Delete**
- Multi-file books: row expands to show `book_files` segments + their `missing` state
- `torrent_hash` present → small icon with deluge state tooltip

**New dedicated pages:**

- `/library/trash` — lists `trash` rows, actions: Restore, Purge Now, TTL countdown per row
- `/library/purged` — lists `inactive_purged` + `blocked_for_redownload` rows, actions: Hard Delete (removes fingerprint permanently)

### 9. Operational defaults

- **Permissions on moves**: preserve `chown`/`chmod`, re-apply import path's group + `g+w` ACL (per `feedback_linux_acls.md`)
- **Concurrent access during swap**: no in-flight protection. ZFS rename + kernel inode survival keeps streaming clients reading the old bytes
- **Scanner skip window**: scanner ignores books where any `book_versions.status IN ('swapping_in', 'swapping_out')`
- **Hash computation**: ingest only. Re-hash via explicit "verify integrity" user action
- **Deluge `move_storage` failure**: retry 3x with backoff. Persistent failure → `metadata_json.deluge_move_failed=true`, UI warning, manual retry action

### 10. Migration plan

One-time `migrate-to-versions` operation, runs at startup after the feature ships:

1. **Dry-run**: scan library, compute per-book plan, store as operation result
2. **User confirms** from a UI prompt → operation executes
3. **Per book**: one `book_versions` row, `status='active'`, `source='imported'`, `ingest_date = books.created_at or now()`. Compute SHA-256 for each file (parallel via `FileIOPool`). Backfill `book_files.version_id`.
4. **iTunes-sourced books**: skipped — no `book_versions` row
5. **Existing cross-book duplicates**: not merged by this migration; existing dedup handles them later
6. **Resumable**: standard tracked operation, PR #270 framework covers crash recovery

Expected runtime for the current 24K-book library (most <500 MB): a few hours of one-time hashing.

## Implementation sequencing

Rough order (each a separate PR):

1. Schema migration — add `book_versions`, `book_files.version_id`, indexes
2. Ingest refactor — route new files through version creation; keep old behavior when feature-flagged off
3. Primary swap operation + recovery case
4. Fingerprint check + deluge pause/approval dialog
5. Delete / trash / purge states + background TTL job
6. UI — Versions panel, Trash view, Purged view
7. Deluge `move_storage` integration
8. `migrate-to-versions` one-time operation + dry-run UI
9. Flip feature flag to on
10. Docs, retention-policy settings, cleanup

## Open implementation questions (deferred, not blockers)

- What does the "verify integrity" action check — rehash a version and compare? Per-file or roll-up?
- UI behavior when a `book_versions` row exists but all `book_files` are marked `missing` (partial deluge failure, etc.)
- Multi-user interaction (3.7): who owns a version? Per-user alts? Deferred until 3.7 design.
- Admin-only override to hard-delete without going through Trash? (Probably yes — in the Purged view UI.)
