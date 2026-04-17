<!-- file: docs/superpowers/plans/2026-04-17-itl-full-rebuild.md -->
<!-- version: 1.0.0 -->
<!-- guid: 963fcd5f-21fb-4f94-9ca3-573e89d49266 -->
<!-- last-edited: 2026-04-16 -->

# 7.9: Full iTunes Library Regenerate/Rebuild — Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans to implement this plan task-by-task.

**Backlog item:** 7.9 — Full iTunes library regenerate/rebuild (from scratch)
**Spec:** None — this plan is self-contained.
**Depends on:** Existing diff-and-batch rebuild mode (commit 286140d, `internal/server/itl_rebuild.go`)

## Overview

The existing ITL rebuild mode (`itl_rebuild.go`) diffs the current DB against an existing ITL file and applies minimal changes. This plan adds a "full rebuild from scratch" mode: generate a completely new ITL file from the database, for cases where the existing ITL is corrupted, lost, or needs a clean slate. The new mode uses the existing ITL binary format infrastructure (`internal/itunes/itl.go`, `itl_le.go`, `itl_le_mutate.go`, `itl_combined_mutate.go`) to construct a valid ITL file.

## Key files

- `internal/itunes/itl.go` — ITLLibrary, ITLTrack, ITLNewTrack structs; ParseITL; AES encryption/decryption
- `internal/itunes/itl_le.go` — little-endian ITL parsing
- `internal/itunes/itl_le_mutate.go` — track insertion, removal, metadata update on LE format
- `internal/itunes/itl_combined_mutate.go` — `ITLOperationSet`, `ApplyITLOperations`
- `internal/server/itl_rebuild.go` — existing diff-based rebuild handler
- `internal/server/itunes.go` — iTunes-related HTTP handlers
- `internal/database/store.go` — `external_id_map` for iTunes PID mappings

---

### Task 1: Blank ITL template generator (1 PR)

**Goal:** Create a function that generates a minimal valid ITL binary with no tracks — just the required headers and structure.

**Files:**
- Create: `internal/itunes/itl_blank.go` — `GenerateBlankITL(version string) (*ITLLibrary, []byte, error)`
- Create: `internal/itunes/itl_blank_test.go`

Implementation:
- [ ] Build the minimum valid ITL structure: hdfm header, empty hdsm (track list), empty hdsm (playlist list), footer
- [ ] Use the same AES encryption key and zlib compression as the real iTunes format
- [ ] Version string should match the latest supported iTunes version (from existing test fixtures)
- [ ] `GenerateBlankITL` returns both the parsed `ITLLibrary` (for in-memory manipulation) and the raw encrypted bytes (for disk write)
- [ ] Generate a random library persistent ID for the new file

**Acceptance criteria:**
- [ ] `ParseITL(blankITLPath)` successfully parses the generated file
- [ ] Parsed library has 0 tracks and 0 playlists
- [ ] Round-trip: generate blank → write to disk → parse back → fields match
- [ ] File is valid AES-encrypted and zlib-compressed (matches real ITL structure)

---

### Task 2: Bulk track insertion into blank ITL (1 PR)

**Goal:** Given a blank ITL and a list of books from the DB, insert all tracks with correct metadata and persistent IDs.

**Files:**
- Create: `internal/itunes/itl_bulk_insert.go` — `BulkInsertTracks(inputPath, outputPath string, tracks []ITLNewTrack, pidAssignments map[string][8]byte) (*ITLWriteBackResult, error)`
- Create: `internal/itunes/itl_bulk_insert_test.go`

Implementation:
- [ ] Accept a list of `ITLNewTrack` structs (location, name, album, artist, genre, etc.)
- [ ] For books that already have iTunes PIDs in `external_id_map`, reuse those PIDs (so the rebuilt ITL matches the old one)
- [ ] For books without existing PIDs, generate new random persistent IDs
- [ ] Use the existing `itl_le_mutate.go` insertion logic but batch all inserts into a single pass
- [ ] Respect the ITL's internal track ID counter (auto-increment)
- [ ] Set `DateAdded` to the book's `created_at` timestamp (preserve history)

**Acceptance criteria:**
- [ ] Insert 100 tracks into a blank ITL, parse it back, all 100 tracks present with correct metadata
- [ ] Reused PIDs match the input `pidAssignments` map exactly
- [ ] Track IDs are unique and sequential
- [ ] File remains a valid ITL after bulk insert

---

### Task 3: DB-to-ITLNewTrack conversion (1 PR)

**Goal:** Convert database Book + BookFile records into `ITLNewTrack` structs ready for insertion.

**Files:**
- Create: `internal/server/itl_rebuild_full.go` — conversion logic and the full rebuild orchestrator
- Create: `internal/server/itl_rebuild_full_test.go`

Conversion logic:
- [ ] `bookToITLNewTrack(book *database.Book, file *database.BookFile, author *database.Author) ITLNewTrack`
- [ ] Map fields: book.Title → Name, author.Name → Artist, book.Title → Album (audiobook convention), file.Format → Kind, file.Duration → TotalTime, book.AudiobookReleaseYear → Year
- [ ] Use `file.ITunesPath` (Windows path) for Location field — this is the path iTunes will use
- [ ] Skip books without ITunesPath (they are not in the iTunes library)
- [ ] Skip books without an iTunes PID mapping (unless `--include-unmapped` flag is set, which assigns new PIDs)
- [ ] For multi-file books: one ITLNewTrack per BookFile, with TrackNumber set from segment order

**Acceptance criteria:**
- [ ] Conversion produces correct ITLNewTrack for a single-file book
- [ ] Conversion produces correct ITLNewTrack set for a multi-file book (correct track numbers)
- [ ] Books without ITunesPath are skipped
- [ ] Author name resolution works (falls back to empty string if no author)

---

### Task 4: Full rebuild HTTP handler + tracked operation (1 PR)

**Goal:** Add API endpoints for full ITL rebuild with dry-run preview and apply modes, tracked as a server operation.

**Files:**
- Modify: `internal/server/itl_rebuild.go` — add `fullRebuildITLHandler` alongside existing `rebuildITLHandler`
- Modify: `internal/server/server.go` — register new routes

Endpoints:
```
POST /api/v1/itunes/rebuild-full?dry_run=true   — preview: how many tracks will be inserted
POST /api/v1/itunes/rebuild-full?dry_run=false   — execute full rebuild
```

Implementation:
- [ ] Dry-run mode: scan all books, count eligible tracks, return `ITLFullRebuildPreview` (total_books, total_tracks, with_pid, without_pid, estimated_file_size)
- [ ] Apply mode: create tracked operation, generate blank ITL, bulk insert all tracks, write to output path
- [ ] Output path: write to a temporary file first, then validate by parsing it back, then atomically rename to the target path
- [ ] Back up the existing ITL before overwriting (same `safeWriteITL` pattern as diff-based rebuild)
- [ ] Progress reporting: update operation progress as tracks are inserted (every 100 tracks)
- [ ] Store new PID assignments in `external_id_map` for any newly generated PIDs
- [ ] Config: use `config.AppConfig.ITLPath` for the target path

**Acceptance criteria:**
- [ ] Dry-run returns correct counts without modifying any files
- [ ] Apply creates a valid ITL with all eligible tracks
- [ ] Existing ITL is backed up before overwrite
- [ ] New PID mappings are persisted in `external_id_map`
- [ ] Operation progress is trackable via the operations API

---

### Task 5: Round-trip verification + integration test (1 PR)

**Goal:** Verify the full rebuild by parsing the output ITL and comparing it against the DB state.

**Files:**
- Create: `internal/server/itl_rebuild_verify_test.go`
- Modify: `internal/server/itl_rebuild_full.go` — add `verifyRebuildResult` function

Verification logic:
- [ ] Parse the rebuilt ITL with `ParseITL`
- [ ] For each track in the rebuilt ITL, look up the corresponding book by PID in `external_id_map`
- [ ] Compare: title, artist, location, track number, year — flag mismatches
- [ ] Compare: total track count in ITL vs eligible books in DB — flag discrepancy
- [ ] Return a `VerificationResult` struct with match count, mismatch details, and overall pass/fail

Integration test:
- [ ] Create a test PebbleDB with 50 books (mix of single-file and multi-file, some with PIDs, some without)
- [ ] Run full rebuild
- [ ] Parse the output ITL
- [ ] Verify all books with PIDs are present in the ITL with correct metadata
- [ ] Verify books without ITunesPath are absent
- [ ] Verify the ITL can be parsed by `ParseITLAsLibrary` (the higher-level conversion)

**Acceptance criteria:**
- [ ] Verification passes for the integration test dataset
- [ ] Mismatches are reported with enough detail to debug (book ID, field name, expected vs actual)
- [ ] Round-trip: DB → full rebuild → parse → verify produces 100% match rate

---

### Task 6: Frontend rebuild UI (1 PR)

**Goal:** Add a "Full Rebuild" option to the iTunes settings panel in the UI.

**Files:**
- Modify: `web/src/components/settings/ITunesImport.tsx` (or create a new `ITunesRebuild.tsx` component)
- Modify: `web/src/services/api.ts` — add API calls for full rebuild

UI elements:
- [ ] "Full Rebuild" button in the iTunes settings section (separate from the existing diff-based rebuild)
- [ ] Click shows a confirmation dialog: "This will regenerate the entire ITL file from scratch. The existing ITL will be backed up."
- [ ] Dry-run preview shown before confirmation (total tracks, books with/without PIDs)
- [ ] Progress bar during rebuild (polls operation status)
- [ ] Success/error toast on completion
- [ ] Link to download/view the backup file path

**Acceptance criteria:**
- [ ] Button is visible in iTunes settings
- [ ] Dry-run preview displays before any destructive action
- [ ] Progress is shown during rebuild
- [ ] Error states are handled (API failure, no ITL path configured)

---

### Estimated effort

| Task | Size | Depends on |
|------|------|------------|
| 1 (blank ITL) | M | -- |
| 2 (bulk insert) | L | 1 |
| 3 (DB conversion) | M | -- |
| 4 (HTTP handler) | L | 1, 2, 3 |
| 5 (verification) | M | 4 |
| 6 (frontend UI) | M | 4 |
| **Total** | ~6 PRs, L overall | |

### Critical path

Tasks 1 and 3 can run in parallel. Task 2 depends on task 1. Task 4 depends on 1, 2, and 3. Tasks 5 and 6 depend on task 4 and can run in parallel with each other.

### Risk notes

- The ITL binary format is reverse-engineered and partially documented. The blank template generation (task 1) is the highest-risk task — if the minimum header structure is wrong, iTunes will reject the file. Mitigate by comparing generated headers against known-good ITL fixtures from the test suite.
- Bulk insertion of thousands of tracks may hit memory limits if the entire ITL is held in memory. The existing `ApplyITLOperations` works in-memory; monitor peak allocation for a 10K+ track library.
