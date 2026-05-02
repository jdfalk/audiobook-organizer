<!-- file: docs/superpowers/plans/2026-04-17-itl-transfer.md -->
<!-- version: 1.0.0 -->
<!-- guid: 8b9c0d1e-2f3a-4a70-b8c5-3d7e0f1b9a99 -->
<!-- last-edited: 2026-04-17 -->

# ITL Upload / Download / Partial Export — Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans to implement this plan task-by-task.

**Backlog item:** 6.4 — ITL upload / download / partial export
**Context:** The iTunes library file (`.itl`) lives on a remote Windows machine. Currently the server reads it via a mounted path (`itunes_library_read_path`) and writes back to another path (`itunes_library_write_path`). This plan adds HTTP endpoints so the frontend or external scripts can upload/download ITL files without needing direct filesystem access — useful for remote management, backups, and partial exports.

## Overview

Three capabilities:
1. **Download** — serve the current ITL file as a binary download
2. **Upload** — accept an ITL file upload, validate it, and optionally replace the active library
3. **Partial export** — generate a minimal ITL containing only selected tracks (for sharing subsets)

---

### Task 1: ITL download endpoint (1 PR)

**Goal:** Serve the current ITL file as a binary download.

**Files:**
- Create: `internal/server/itunes_transfer.go` — download handler
- Modify: `internal/server/server.go` — register route

**Route:** `GET /api/v1/itunes/library/download`

- [ ] Read `config.AppConfig.ITunesLibraryWritePath` (the canonical ITL)
- [ ] If not configured or file doesn't exist, return 404 with helpful message
- [ ] Serve as `application/octet-stream` with `Content-Disposition: attachment; filename="iTunes Library.itl"`
- [ ] Include file size and last-modified headers
- [ ] Gated on `integrations.manage` permission
- [ ] Test: mock file on disk, verify download returns correct bytes

---

### Task 2: ITL upload + validation endpoint (1 PR)

**Goal:** Accept an ITL file upload, validate its structure, and optionally install it.

**Files:**
- Modify: `internal/server/itunes_transfer.go` — upload handler
- Modify: `internal/server/server.go` — register route

**Route:** `POST /api/v1/itunes/library/upload`

Accepts multipart form upload with field `library`.

- [ ] Parse multipart upload (limit to 500 MB)
- [ ] Save to a temp file
- [ ] Validate: call `itunes.ParseITL(tempPath)` — if it fails, return 400 with parse error
- [ ] If `?install=true` query param:
  - Back up existing ITL to `{write_path}.bak-{timestamp}`
  - Copy uploaded file to `itunes_library_write_path`
  - Return `{installed: true, tracks: N, playlists: M}`
- [ ] If `?install=false` (default):
  - Return validation result only: `{valid: true, tracks: N, playlists: M, version: "..."}`
  - Delete temp file
- [ ] Gated on `integrations.manage` permission
- [ ] Test: upload a valid ITL fixture, verify validation response; upload garbage, verify 400

---

### Task 3: ITL backup list + restore (1 PR)

**Goal:** List ITL backups and restore from a previous version.

**Files:**
- Modify: `internal/server/itunes_transfer.go` — backup list + restore handlers
- Modify: `internal/server/server.go` — register routes

**Routes:**
- `GET /api/v1/itunes/library/backups` — list `.bak-*` files
- `POST /api/v1/itunes/library/restore` — restore from a named backup

- [ ] List: scan the directory containing `itunes_library_write_path` for `.bak-*` files, return sorted by timestamp
- [ ] Restore: validate the backup parses, back up the current file, copy backup into place
- [ ] Return restored file's track/playlist count
- [ ] Test: create fake backups, verify list returns them sorted

---

### Task 4: Partial export — selected tracks only (1 PR)

**Goal:** Generate a minimal ITL containing only specified tracks.

**Files:**
- Modify: `internal/server/itunes_transfer.go` — export handler
- May need: `internal/itunes/itl_filter.go` — filter tracks from a parsed library

**Route:** `POST /api/v1/itunes/library/export`

Accepts JSON body: `{book_ids: ["id1", "id2", ...]}` or `{playlist_id: "pl1"}`

- [ ] Load the current ITL
- [ ] Resolve book_ids → iTunes persistent IDs via `external_id_map`
- [ ] Filter the parsed library to include only matching tracks
- [ ] Write the filtered library to a temp file
- [ ] Serve as download with `Content-Disposition: attachment; filename="export.itl"`
- [ ] If `playlist_id` provided, resolve to book_ids first via `GetUserPlaylist → MaterializedBookIDs`
- [ ] Test: export with known PIDs, verify output parses and contains only those tracks

**Note:** This requires the ITL writer to support writing a full library (not just inserting/removing tracks). If the writer doesn't support this yet, this task depends on 7.9 (full rebuild). In that case, defer to after 7.9 and use the rebuild infrastructure.

---

### Task 5: Frontend — ITL management panel (1 PR)

**Goal:** Add UI for upload/download/backup in the iTunes settings tab.

**Files:**
- Modify: `web/src/components/settings/ITunesImport.tsx` — add transfer section
- Or create: `web/src/components/settings/ITunesTransfer.tsx` — standalone panel

UI elements:
- [ ] "Download Library" button → triggers GET download
- [ ] "Upload Library" file picker with drag-and-drop, shows validation result, "Install" button
- [ ] Backup list table with restore buttons
- [ ] "Export Selection" section (future — depends on task 4)

---

### Estimated effort

| Task | Size | Depends on |
|------|------|------------|
| 1. Download | S | — |
| 2. Upload + validate | M | — |
| 3. Backup list + restore | S | Task 2 (backups created by upload) |
| 4. Partial export | M-L | 7.9 (full ITL write support) |
| 5. Frontend | M | Tasks 1-3 |

### Critical path

Tasks 1-3 are independent of each other and can ship immediately. Task 4 may depend on 7.9. Task 5 is frontend-only after 1-3 are done.
