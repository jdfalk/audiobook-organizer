# Smart + Static Playlists — Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans to implement this plan task-by-task.

**Spec:** `docs/superpowers/specs/2026-04-15-smart-and-static-playlists-design.md` (v1.1)
**Depends on:** DES-1 Bleve (query engine), 3.6 Read/unread (read_status field in smart queries), 3.7 Multi-user (per-user playlist ownership)
**Partial start possible:** Tasks 1-3 can begin before DES-1 ships (use Go-side eval, swap to Bleve later)

---

### Task 1: Playlist schema + CRUD (1 PR)

**Files:**
- Modify: `internal/database/store.go` — add `Playlist`, `PlaylistFilter` structs + Store interface methods
- Modify: `internal/database/pebble_store.go` — implement with PebbleDB keys:
  ```
  playlist:{id} → Playlist JSON
  user_playlist:{userID}:{playlistID} → ""
  playlist_name:{lcase-name} → playlistID
  itunes_playlist_map:{persistentID} → playlistID
  itunes_declined_pid:{persistentID} → ""
  setting:itunes_dynamic_migration_done → "true"
  ```
- Modify: `internal/database/mock_store.go`
- Create: `internal/database/playlist_store_test.go`

- [ ] CRUD: CreatePlaylist, GetPlaylist, GetPlaylistByName, GetPlaylistByITunesPID, ListPlaylists, UpdatePlaylist, DeletePlaylist
- [ ] Static ops: AddBookToPlaylist, RemoveBookFromPlaylist, ReorderPlaylist (atomic batch)
- [ ] Dirty tracking: MarkPlaylistDirty, ListDirtyPlaylists
- [ ] iTunes migration helpers: DeclineITunesPlaylistMigration, IsITunesPlaylistDeclined, IsITunesDynamicMigrationDone, SetITunesDynamicMigrationDone
- [ ] Permission field: `playlists.create` in auth/permissions.go (from 3.7)
- [ ] Test each method

---

### Task 2: Smart playlist query evaluator (1 PR)

**Goal:** Evaluate a smart playlist's query against the library and return matching book IDs.

**Files:**
- Create: `internal/server/playlist_evaluator.go` — `EvaluateSmartPlaylist(store, query, sort, limit, userID) ([]string, error)`
- Create: `internal/server/playlist_evaluator_test.go`

- [ ] Parse query via DSL parser (from DES-1 task 2, or the Go parser)
- [ ] If DES-1 Bleve is available → translate AST → Bleve → post-filter per-user fields → return
- [ ] If Bleve not yet available → fallback: load all books, compile AST to `func(Book) bool` predicate, filter in Go
- [ ] Sort by specified field(s) + direction
- [ ] Limit
- [ ] Cache `materialized_book_ids` on the playlist blob for iTunes sync
- [ ] Test with synthetic books + various query shapes

---

### Task 3: Playlist HTTP endpoints (1 PR)

**Files:**
- Create: `internal/server/playlist_handlers.go`
- Modify: `internal/server/server.go` — register routes

Endpoints:
```
GET    /api/v1/playlists                     — list (filter by type, search by name)
POST   /api/v1/playlists                     — create (type, name, query or book_ids)
GET    /api/v1/playlists/:id                 — get (smart: includes live-evaluated book list)
PUT    /api/v1/playlists/:id                 — update
DELETE /api/v1/playlists/:id                 — delete
POST   /api/v1/playlists/:id/books           — add book(s) to static playlist
DELETE /api/v1/playlists/:id/books/:bookID   — remove book from static playlist
POST   /api/v1/playlists/:id/reorder         — reorder static playlist
POST   /api/v1/playlists/:id/materialize     — materialize smart → new static
```

- [ ] Create: validate type, name uniqueness, query syntax (for smart)
- [ ] Get (smart): evaluate query, return `{playlist, books: [evaluated results]}`
- [ ] Get (static): return `{playlist, books: [ordered book list]}`
- [ ] Materialize: evaluate smart → create new static with name `"{name} (snapshot {date})"`
- [ ] All routes gated on `playlists.create` for mutation, `library.view` for read
- [ ] Test via httptest

---

### Task 4: ITL Smart Criteria reader (1 PR)

**Goal:** Parse iTunes Smart Criteria binary blobs so we can import iTunes dynamic playlists.

**Files:**
- Create: `internal/itunes/smart_criteria_reader.go` — parse Smart Info + Smart Criteria binary → rule tree
- Create: `internal/itunes/smart_criteria_reader_test.go`
- Create: `internal/itunes/smart_criteria_translator.go` — translate iTunes rule tree → our DSL string
- Create: `internal/itunes/smart_criteria_translator_test.go`

- [ ] First: check if existing ITL parser (`internal/itunes/itl_*.go`) already emits Smart Criteria. If yes, build on top. If no, add the binary reader.
- [ ] Parse Smart Criteria header (rule count, combinator All/Any, nested groups)
- [ ] Parse each rule (field ID → our field name via mapping table, operator, value)
- [ ] Handle nested groups recursively
- [ ] Translator: walk parsed tree → emit our DSL string (with `&&`/`||`/`()`)
- [ ] Lossy handling: iTunes fields we can't map → store as comment + flag `partial_translation`
- [ ] Test with known iTunes smart playlist binary fixtures (extract from a real ITL)

---

### Task 5: One-time iTunes dynamic playlist migration (1 PR)

**Files:**
- Create: `internal/server/itunes_playlist_migration.go` — tracked op `itunes_dynamic_playlist_migration`
- Modify: `internal/server/server.go` — route + resume case

- [ ] Enumerate iTunes playlists with Smart Info + Smart Criteria (from ITL parse output)
- [ ] For each:
  1. Read + translate Smart Criteria → our DSL (via task 4 translator)
  2. Store raw criteria blob on playlist for audit
  3. Create `app`-owned smart playlist
  4. Evaluate → materialize → write static iTunes playlist (new PID)
  5. Delete original dynamic iTunes playlist from ITL
  6. Log the migration
- [ ] Set `itunes_dynamic_migration_done` setting
- [ ] Idempotent: skip playlists that already have a matching app-owned counterpart
- [ ] Resumable via operation framework

---

### Task 6: iTunes sync pass — push playlists (1 PR)

**Files:**
- Modify: `internal/server/itunes.go` or create `internal/server/itunes_playlist_sync.go` — playlist sync during scheduled iTunes sync pass
- Modify: `internal/server/scheduler.go` — wire into existing iTunes sync schedule

- [ ] For each `app`-owned playlist with `dirty = true`:
  - Static → write as iTunes static playlist, store PID
  - Smart → evaluate → materialize → write as iTunes static playlist, store PID
- [ ] Clear `dirty`, update `itunes_persistent_id`
- [ ] Defensive catch: if new iTunes dynamic playlists detected post-migration, surface approval dialog data (store as operation result for UI to display)

---

### Task 7: Frontend — playlist list page + static editor (1 PR)

**Files:**
- Create: `web/src/pages/Playlists.tsx` — list page with both types
- Create: `web/src/components/playlists/StaticPlaylistEditor.tsx` — drag-reorder, add/remove, rename
- Create: `web/src/services/playlistApi.ts`
- Modify: `web/src/components/layout/Sidebar.tsx` — add Playlists entry
- Modify: `web/src/App.tsx` — route `/playlists`

- [ ] List page: all playlists, type chip (Static/Smart), book count, created date, owner
- [ ] "+ New Playlist" → choose type → navigate to editor
- [ ] Static editor: drag-reorder rows, remove button per row, "+ Add books" (library picker dialog)
- [ ] Delete playlist with confirmation

---

### Task 8: Frontend — smart playlist editor (1 PR)

**Files:**
- Create: `web/src/components/playlists/SmartPlaylistEditor.tsx` — query text box + live preview
- Modify: `web/src/utils/searchParser.ts` — expose syntax validation for inline error display

- [ ] Single text box with monospace font for the query
- [ ] Live preview pane: first 20 matching books, debounced 300ms
- [ ] Syntax error inline: "expected `)` at position 23" / "unknown field `autor` — did you mean `author`?"
- [ ] Field autocomplete dropdown from SEARCH_FIELDS
- [ ] "Materialize as Static" button

---

### Task 9: "Add to Playlist" dialog on BookDetail + Library (1 PR)

**Files:**
- Create: `web/src/components/playlists/AddToPlaylistDialog.tsx`
- Modify: `web/src/pages/BookDetail.tsx` — action button
- Modify: `web/src/pages/Library.tsx` — action button per row (and bulk when 5.3 lands)

- [ ] Dialog: search/pick existing static playlist, or "+ New playlist" (name prompt)
- [ ] Confirm → POST `/playlists/:id/books` → success toast
- [ ] Single-book path works immediately; bulk path when 5.3 Batch Select lands

---

### Estimated effort

| Task | Size | Depends on |
|---|---|---|
| 1 (schema) | M | 3.7 task 1 |
| 2 (evaluator) | M | DES-1 task 3 (or fallback to Go-side) |
| 3 (endpoints) | M | 1+2 |
| 4 (Smart Criteria reader) | L | — (can start immediately) |
| 5 (migration op) | M | 4+3 |
| 6 (iTunes sync push) | M | 3+5 |
| 7 (frontend list + static) | L | 3 |
| 8 (frontend smart) | M | 2+7 |
| 9 (add-to-playlist) | S | 7 |
| **Total** | ~9 PRs, XL overall | |

### Critical path

Task 4 (Smart Criteria reader) is the riskiest and can start immediately in parallel with everything else. If the binary format is harder to parse than expected, it's the schedule blocker. Tasks 1-3 are safe to start before DES-1 ships using Go-side fallback eval.
