# Read/Unread Tracking — Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans to implement this plan task-by-task.

**Spec:** `docs/superpowers/specs/2026-04-15-read-unread-tracking-design.md`
**Depends on:** 3.7 Multi-user (per-user state attribution, `UserFromContext`)
**Unblocks:** 3.4 Playlists (smart playlist `read_status` field)

---

### Task 1: Schema — PebbleDB keys + Store methods (1 PR)

**Files:**
- Modify: `internal/database/store.go` — add `UserPosition`, `UserBookState` structs + Store interface methods
- Modify: `internal/database/pebble_store.go` — implement with keys:
  ```
  user_position:{userID}:{bookID}:{segmentID} → {position_seconds, updated_at}
  user_book_state:{userID}:{bookID} → {status, status_manual, last_activity_at, last_segment_id, total_listened_seconds, progress_pct}
  book_user:{bookID}:{userID} → ""
  ```
- Modify: `internal/database/mock_store.go`
- Create: `internal/database/user_state_test.go`

- [ ] `SetUserPosition(userID, bookID, segmentID, positionSeconds)` — upsert
- [ ] `GetUserPosition(userID, bookID)` — latest across segments (by `updated_at`)
- [ ] `ListUserPositionsForBook(userID, bookID)` — all segments
- [ ] `ClearUserPosition(userID, bookID)` — delete all position rows
- [ ] `SetUserBookStatus(userID, bookID, status, manual)` — write `user_book_state`
- [ ] `GetUserBookState(userID, bookID)` — read
- [ ] `ClearUserManualStatus(userID, bookID)` — flip `status_manual = false`, recompute
- [ ] `ListInProgressForUser(userID, limit, offset)` — prefix scan `user_book_state:{userID}:*`, filter status=in_progress, sort by last_activity_at DESC
- [ ] `ListFinishedForUser(userID, limit, offset)` — same, status=finished
- [ ] `ListAdminPositionsSince(t)` — for iTunes writeback
- [ ] Test each method with PebbleDB in-memory store

---

### Task 2: Position + status HTTP endpoints (1 PR)

**Files:**
- Create: `internal/server/reading_handlers.go` — all endpoints
- Modify: `internal/server/server.go` — register routes

Endpoints:
```
POST   /api/v1/books/:id/position      — { segment_id, position_seconds }
GET    /api/v1/books/:id/position
GET    /api/v1/books/:id/state
PATCH  /api/v1/books/:id/status         — { status }
DELETE /api/v1/books/:id/status
GET    /api/v1/me/in-progress?limit=N
GET    /api/v1/me/finished?limit=N
```

- [ ] `POST position`: read user from context → `SetUserPosition` → recompute `UserBookState` (auto-status logic: sum positions vs total duration, flip to `finished` at 95%)
- [ ] `GET position`: read user → `GetUserPosition`
- [ ] `GET state`: read user → `GetUserBookState`
- [ ] `PATCH status`: manual override → `SetUserBookStatus(manual=true)`
- [ ] `DELETE status`: clear manual → `ClearUserManualStatus` → recompute from positions
- [ ] `GET me/in-progress` and `me/finished`: list endpoints
- [ ] All routes gated on `library.view` (implicit self-scope)
- [ ] Test via httptest with mock store

---

### Task 3: Auto-status computation engine (1 PR)

**Files:**
- Create: `internal/server/reading_engine.go` — `recomputeBookState(store, userID, bookID) (*UserBookState, error)`
- Create: `internal/server/reading_engine_test.go`

- [ ] Load all position rows for (user, book)
- [ ] Load book_files with durations → compute total book duration
- [ ] Sum: for each segment with a position, use `min(position, segment_duration)` as listened
- [ ] If total_listened / total_duration >= 0.95 → status = `finished`
- [ ] Else if total_listened > 0 → `in_progress`
- [ ] Else → `unstarted`
- [ ] Skip computation if `status_manual = true`
- [ ] Cache result as `user_book_state` row
- [ ] Test: various scenarios (0%, 50%, 95%, 100%, manual override)

---

### Task 4: iTunes Bookmark sync (1 PR)

**Files:**
- Modify: `internal/server/itunes.go` — during iTunes import, seed admin positions from Bookmark
- Modify: `internal/server/itunes_writeback_batcher.go` — on flush, write admin positions back to iTunes Bookmark
- Modify: `internal/server/scheduler.go` — add `itunes_position_sync` maintenance task

- [ ] **Pull** (iTunes → app): for each iTunes-sourced book_file with Bookmark > 0, create admin `user_position` row if not already set
- [ ] **Push** (app → iTunes): for admin user, if position `updated_at > last_sync_at`, write Bookmark field via ITL writer
- [ ] Also: if play_count > 0 in iTunes and admin has no state → seed `finished`
- [ ] If admin flips to `finished` → increment iTunes `Play Count` by 1, set `Played Date`
- [ ] Test with mock ITL data

---

### Task 5: Search integration — new fields (1 PR)

**Files:**
- Modify: `web/src/utils/searchParser.ts` — add `read_status`, `last_played`, `progress_pct` to SEARCH_FIELDS
- Modify: `internal/search/query_parser.go` (from DES-1) — recognize new fields
- Modify: `internal/search/post_filter.go` (from DES-1) — handle per-user filtering for these fields

- [ ] `read_status:in_progress` → filter by calling user's UserBookState
- [ ] `last_played:within_days:30` → filter by `last_activity_at > (now - 30d)`
- [ ] `progress_pct:>75` → filter by cached progress_pct
- [ ] All are Go-side post-filters (not indexed in Bleve per DES-1 spec)

---

### Task 6: Frontend — BookDetail + Library column (1 PR)

**Files:**
- Modify: `web/src/pages/BookDetail.tsx` — status chip, "X h Y m left" resume button, "Mark as..." menu
- Modify: `web/src/pages/Library.tsx` — optional `read_status` column (hidden by default, toggleable)
- Create: `web/src/services/readingApi.ts` — `getBookState`, `setPosition`, `setStatus`, `clearStatus`, `getInProgress`, `getFinished`

- [ ] BookDetail: prominent chip (green=finished, blue=in_progress, gray=unstarted, amber=abandoned)
- [ ] Resume button: "Resume — Ch 5, 1h23m left" → navigates to... (player TBD, or just shows position)
- [ ] "Mark as..." dropdown: Finished, Unstarted, Abandoned, Use Automatic
- [ ] Library column: small status icon, sortable

---

### Task 7: Sidebar entries — "Currently Reading" + "Finished" (1 PR)

**Files:**
- Modify: `web/src/components/layout/Sidebar.tsx` — two new entries under Library
- These route to `/library?filter=read_status:in_progress` and `/library?filter=read_status:finished`

- [ ] Sidebar entries with book icon + count badge (fetched from `GET /me/in-progress?limit=0` for count)
- [ ] Click navigates to Library with pre-set search filter

---

### Task 8: iTunes backfill migration (1 PR)

**Files:**
- Create: `internal/server/reading_backfill.go` — one-time tracked op
- Modify: `internal/server/server.go` — run at startup if setting `reading_backfill_done` is not set

- [ ] For each iTunes-sourced book with `play_count > 0` or Bookmark > 0:
  - Create admin's `user_book_state` row
  - Seed `finished` if play_count > 0, else `in_progress`
  - For each book_file with Bookmark, create `user_position` row
- [ ] Set `reading_backfill_done` setting
- [ ] Idempotent (skip books that already have state)

---

### Estimated effort

| Task | Size | Depends on |
|---|---|---|
| 1 (schema) | M | 3.7 task 1 |
| 2 (endpoints) | M | 1 |
| 3 (engine) | M | 1 |
| 4 (iTunes sync) | M | 2+3 |
| 5 (search fields) | S | DES-1 task 5 |
| 6 (frontend) | M | 2 |
| 7 (sidebar) | S | 6 |
| 8 (backfill) | S | 2+3+3.7 task 5 |
| **Total** | ~8 PRs, L overall | |
