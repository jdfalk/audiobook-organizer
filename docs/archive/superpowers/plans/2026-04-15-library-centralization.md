# Library Centralization + Versioning — Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans to implement this plan task-by-task.

**Spec:** `docs/superpowers/specs/2026-04-15-library-centralization-design.md`
**Depends on:** Nothing (can run in parallel with 4.4 DI, but large surface)
**Pairs with:** 6.1 Deluge move_storage integration (task 8)

---

### Task 1: Schema migration — `book_versions` table + `book_files.version_id` (1 PR)

**Files:**
- Modify: `internal/database/store.go` — add `BookVersion` struct + Store interface methods
- Modify: `internal/database/pebble_store.go` — implement with PebbleDB keys:
  ```
  book_version:{id} → BookVersion JSON
  book_version_by_book:{bookID}:{versionID} → ""
  book_version_active:{bookID} → versionID (unique active constraint)
  book_version_by_hash:{torrentHash} → versionID (fingerprint fast path)
  ```
- Add `version_id` to `BookFile` struct in store.go
- Modify: `internal/database/mock_store.go`
- Create: `internal/database/book_version_store_test.go`

- [ ] BookVersion struct: `ID`, `BookID`, `Status`, `Format`, `Source`, `SourceOriginalPath`, `TorrentHash`, `IngestDate`, `PurgedDate`, `MetadataJSON`
- [ ] Status values: `pending`, `active`, `alt`, `swapping_in`, `swapping_out`, `trash`, `inactive_purged`, `blocked_for_redownload`
- [ ] CRUD: CreateBookVersion, GetBookVersion, GetBookVersionsByBookID, UpdateBookVersion, DeleteBookVersion
- [ ] Active constraint: GetActiveVersionForBook, SetActiveVersion (enforces single-active via key)
- [ ] Fingerprint: GetVersionByTorrentHash, GetVersionsByFileHash (scan book_files by hash)
- [ ] Trash + purge: ListTrashedVersions, ListPurgedVersions
- [ ] `book_files.version_id` field added, `GetBookFilesByVersionID` query
- [ ] Test each method

---

### Task 2: `.versions/` filesystem operations (1 PR)

**Files:**
- Create: `internal/server/version_fs.go` — `MoveToVersionsDir(book, versionID)`, `MoveFromVersionsDir(book, versionID)`, `EnsureVersionsDir(bookDir)`
- Create: `internal/server/version_fs_test.go`

- [ ] `EnsureVersionsDir(bookDir)` — create `.versions/` under book's parent dir
- [ ] `MoveToVersionsDir(book, versionID, files)` — rename each file from natural path to `.versions/{versionID}/{filename}`; create `{versionID}/` subdir; return list of new paths
- [ ] `MoveFromVersionsDir(book, versionID, files)` — reverse: rename from `.versions/{versionID}/` to natural path
- [ ] On ZFS: each move is O(1) rename on the same dataset
- [ ] Error handling: if target exists, skip with warning (idempotent)
- [ ] Permission preservation: re-apply group + g+w ACL after move (per `feedback_linux_acls.md`)
- [ ] Test with temp dirs, verify file content intact, verify dirs created/cleaned

---

### Task 3: Primary swap tracked operation (1 PR)

**Files:**
- Create: `internal/server/version_swap.go` — `runVersionSwap(ctx, opID, bookID, fromVersionID, toVersionID, progress) error`
- Create: `internal/server/version_swap_test.go`
- Modify: `internal/server/server.go` — register `version_swap` resume case

- [ ] Step 1: DB → `from.status = swapping_out`, `to.status = swapping_in`
- [ ] Step 2: FS → `MoveToVersionsDir(book, fromVersionID, fromFiles)`
- [ ] Step 3: FS → `MoveFromVersionsDir(book, toVersionID, toFiles)`
- [ ] Step 4: DB → `from.status = alt`, `to.status = active`; update `book_files.file_path` for all affected files; update `books.file_path`
- [ ] Step 5: iTunes writeback → `GlobalWriteBackBatcher.Enqueue(bookID)`
- [ ] Step 6: Checkpoint clearing + operation completion
- [ ] Recovery: if `swapping_in`/`swapping_out` found on restart, resume from the DB state; each FS step is idempotent
- [ ] Test with mock filesystem + store

---

### Task 4: Fingerprint check on new file (1 PR)

**Files:**
- Create: `internal/server/version_fingerprint.go` — `CheckFingerprint(torrentHash, fileHashes) *FingerprintMatch`
- Create: `internal/server/version_fingerprint_test.go`

- [ ] Fast path: lookup `book_version_by_hash:{torrentHash}` → if found, status is `inactive_purged` or `blocked_for_redownload` → return match with book info
- [ ] Slow path: for each fileHash in input, scan `book_files` where `file_hash = hash` AND `version.status IN (inactive_purged, blocked_for_redownload)` → collect matches
- [ ] Return: `{matched: bool, book_id, version_id, match_type: "torrent_hash" | "file_hash", purge_reason}`
- [ ] No action taken — caller decides what to do (pause deluge, surface dialog, etc.)
- [ ] Test with synthetic purged versions

---

### Task 5: Ingest flow — route new files through versioning (1 PR)

**Files:**
- Modify: `internal/scanner/scanner.go` — after detecting a new file, check fingerprint, create `book_version` row
- Modify: `internal/server/organize_service.go` — organize creates a version for the new organized copy
- Modify: relevant import handlers — file import creates a version

- [ ] New book (not yet in library) → `book_version` with `status=active`, `source=imported` or `source=deluge`
- [ ] Known book, new file → `book_version` with `status=alt`; never auto-promote unless book has zero active versions
- [ ] Fingerprint match → flag for user review (don't auto-add)
- [ ] Compute SHA-256 hash for each new file → store on `book_files.file_hash`
- [ ] Test: import a new file → verify version row created + hashed

---

### Task 6: Delete / trash / purge lifecycle (1 PR)

**Files:**
- Create: `internal/server/version_lifecycle.go` — delete, trash, restore, purge, hard-delete
- Modify: `internal/server/scheduler.go` — add `trash_cleanup` maintenance task (TTL 14d)
- Modify: `internal/server/server.go` — routes for version lifecycle actions

Endpoints:
```
DELETE /api/v1/books/:id/versions/:vid             — trash a version
POST   /api/v1/books/:id/versions/:vid/restore     — restore from trash
POST   /api/v1/books/:id/versions/:vid/purge-now   — skip TTL, purge immediately
DELETE /api/v1/purged-versions/:vid                 — hard-delete (remove fingerprint)
```

- [ ] Delete: `status → trash`, auto-promote if was primary
- [ ] Auto-promote: most-recent `alt` by `ingest_date` becomes `active` (triggers swap op)
- [ ] Restore: `trash → alt` (or → `active` with explicit flag)
- [ ] TTL cleanup: maintenance job scans `trash` versions where `created_at + 14d < now` → physically delete files, `status → inactive_purged`, keep `book_files` rows with `missing=true` for fingerprint
- [ ] Purge-now: skip TTL, same as TTL cleanup but immediate
- [ ] Hard-delete from purged view: delete `book_version` row + `book_files` rows entirely
- [ ] Test each lifecycle transition

---

### Task 7: Frontend — Versions panel + Trash view + Purged view (2 PRs)

**7a: BookDetail Versions panel**
- Modify: `web/src/pages/BookDetail.tsx` — replace/extend Version Group panel
- Create: `web/src/services/versionApi.ts`

- [ ] One row per `book_version`: status dot, format, bitrate, duration, source, ingest_date
- [ ] Green dot = active, gray = alt, orange = trash
- [ ] "Make Primary" on alts → triggers swap operation
- [ ] "Delete" → trash
- [ ] Multi-file expand → shows `book_files` per version

**7b: Trash + Purged pages**
- Create: `web/src/pages/Trash.tsx` — list trashed versions, Restore / Purge Now actions, TTL countdown
- Create: `web/src/pages/Purged.tsx` — list purged versions, Hard Delete action
- Modify: `web/src/components/layout/Sidebar.tsx` — Trash + Purged entries

---

### Task 8: Deluge `move_storage` integration (1 PR)

**Files:**
- Create: `internal/deluge/client.go` — wrap deluge JSON-RPC, `MoveStorage(torrentHash, newPath) error`
- Modify: `internal/server/version_swap.go` — after swap step 4, call deluge `MoveStorage` for affected versions
- Modify: `internal/server/version_lifecycle.go` — on purge, remove torrent from deluge

- [ ] Deluge RPC: `core.move_storage(torrent_id, dest)` — moves the torrent's download location
- [ ] Retry 3x with backoff; on persistent failure, flag `metadata_json.deluge_move_failed=true`
- [ ] On purge: `core.remove_torrent(torrent_id, remove_data=false)` — data already gone
- [ ] Config: `deluge_host`, `deluge_port`, `deluge_username`, `deluge_password` in AppConfig
- [ ] Test with mock RPC

---

### Task 9: `migrate-to-versions` one-time operation (1 PR)

**Files:**
- Create: `internal/server/version_migration.go` — tracked op `migrate_to_versions`
- Modify: `internal/server/server.go` — route + startup detection

- [ ] Dry-run mode: scan library, compute per-book plan, store as operation result
- [ ] User confirms from UI → execute mode
- [ ] Per book:
  1. Create one `book_version` row: `status=active`, `source=imported`, `ingest_date=book.created_at`
  2. Compute SHA-256 for each `book_file` (parallel via FileIOPool)
  3. Backfill `book_files.version_id` to the new version's ID
  4. File layout unchanged — files stay where they are; `.versions/` is empty
- [ ] Skip iTunes-sourced books (per spec)
- [ ] Skip books that already have a `book_version` (idempotent)
- [ ] Resumable: checkpoint every 100 books
- [ ] Runtime estimate: a few hours for 24K books (hashing is the bottleneck)

---

### Task 10: Fingerprint approval dialog (1 PR)

**Files:**
- Create: `web/src/components/FingerprintMatchDialog.tsx` — "This torrent was previously purged as version X of book Y. Add anyway?"
- Modify: scanner or deluge integration — surface matches via event system
- Create: endpoint `GET /api/v1/fingerprint-alerts` — pending fingerprint matches awaiting approval

- [ ] Pending matches stored as a short-lived queue (PebbleDB key or in-memory with persistence)
- [ ] Dialog shows: book title, version info, purge date, match type (torrent_hash vs file_hash)
- [ ] Approve → proceed with ingest (create new alt version)
- [ ] Reject → remove torrent from deluge, dismiss alert

---

### Estimated effort

| Task | Size | Depends on |
|---|---|---|
| 1 (schema) | L | — |
| 2 (filesystem ops) | M | — |
| 3 (swap op) | L | 1+2 |
| 4 (fingerprint) | M | 1 |
| 5 (ingest flow) | L | 1+2+4 |
| 6 (lifecycle) | L | 1+2 |
| 7a (versions panel) | M | 1+3 |
| 7b (trash/purged) | M | 6 |
| 8 (deluge) | M | 3 |
| 9 (migration) | L | 1 |
| 10 (fingerprint UI) | M | 4+5 |
| **Total** | ~11 PRs, XL overall | |

### Execution strategy

**Parallelizable tracks:**
- Track A (schema + migration): tasks 1 → 9
- Track B (filesystem + swap): tasks 2 → 3 → 8
- Track C (fingerprint): tasks 4 → 5 → 10
- Track D (lifecycle + UI): tasks 6 → 7a, 7b

Tasks 1 and 2 are the foundation — start those first. Task 9 (migration) is the last step and should NOT be merged until all other tasks are proven with new-book flows.

### Rollout

1. Ship tasks 1-8 behind a feature flag (`centralization_enabled = false`)
2. Migration (task 9) runs on demand after all code is in place
3. Flag flip enables the new ingest flow for new content
4. Monitor for 1 week with real library traffic before marking stable
