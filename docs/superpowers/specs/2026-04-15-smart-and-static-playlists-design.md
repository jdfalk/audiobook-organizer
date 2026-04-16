<!-- file: docs/superpowers/specs/2026-04-15-smart-and-static-playlists-design.md -->
<!-- version: 1.1.0 -->
<!-- guid: 7c4a8e2f-3d1b-4f90-a5d7-6b9e1c0f8a43 -->

# Smart + Static Playlists with iTunes Sync — Design

**Status:** Design complete (Apr 15, 2026, revised same session). Ready for implementation plan.
**Scope item:** TODO.md §3.4.
**Depends on:** existing library search parser (`web/src/utils/searchParser.ts`) + ITL parser (PR 2026-03-27).
**Brainstorm:** Apr 15, 2026 session (v1.0 → v1.1 revision: simplified iTunes sync to push-only-static + one-time dynamic migration).

## Goal

Two playlist types — **static** (ordered book list, manually curated) and **smart** (live-evaluated filter expression) — with iTunes sync that is deliberately one-way-in-form: we always push as iTunes **static** playlists, never as iTunes dynamic (Smart Criteria). We own the evaluation; iTunes receives a materialized list.

## Locked decisions

### 1. Two types, unified storage

| Type | Membership | Edit actions | iTunes form |
|---|---|---|---|
| `static` | Explicit ordered list of book IDs | Add / remove / reorder | Static iTunes playlist |
| `smart` | Live-evaluated filter expression | Edit query, sort, limit; "Materialize as Static" | **Materialized** iTunes static playlist (refreshed each sync) |

Single `playlist:{id}` JSON blob in PebbleDB with a `type` discriminator.

### 2. Query DSL — extend existing library search

Existing syntax unchanged. Additions (backward-compatible):

```
AND:  whitespace  |  &&  |  AND
OR:               ||  |  OR
NOT:  -   |  NOT
Group: ( ... )
Within-field alternation: field:(a|b|c)
```

- Single pipe `|` is reserved for within-field alternation
- Double pipe `||` is top-level OR between clauses
- Visually distinct on purpose

Worked example (both parse identically):

```
(-title:twilight && -title:"New Dawn" && title:vampire) || title:(Fangtown|fangtown|"fang town")

(NOT title:twilight AND NOT title:"New Dawn" AND title:vampire) OR title:(Fangtown|fangtown|"fang town")
```

- Substring match is default. `title:vampire` matches "Interview with the Vampire."
- Quoted values stay substring; quotes only group whitespace.
- Exact-match operator (`field:=value`) reserved for future, not MVP.
- Field set: everything in existing `SEARCH_FIELDS`.

### 3. iTunes sync — push-only-as-static

**We never write iTunes Smart Criteria.** Every app-owned playlist pushes to iTunes as a **static** iTunes playlist.

- `static` playlists: trivially, the book list pushes as-is
- `smart` playlists: evaluate locally, **materialize** into a static book list, push that
- Re-materialize smart playlists when:
  - Query / sort / limit is edited
  - Library changes could affect membership (new book, metadata edit, tag add/remove) — marked stale via a `dirty` flag, actual re-eval at next iTunes sync pass or on "Sync now"

**Rationale for pushing static only:** iTunes's own Smart Criteria evaluator is weaker than ours, its field set is incomplete for our data (no `tag`, `language`, internal states), and we don't want to race iTunes's rule engine against ours. Our side computes; iTunes receives the result.

### 4. One-time iTunes dynamic playlist migration

New tracked operation `itunes_dynamic_playlist_migration`, idempotent. Runs once at feature rollout and on-demand from admin UI if needed.

1. Enumerate all iTunes dynamic playlists (Smart Info + Smart Criteria present in ITL)
2. For each:
   1. Read + translate rules to our DSL (best-effort)
   2. Store raw Smart Criteria blob alongside the translated query for audit
   3. Create an `app`-owned **smart** playlist in our DB with the translated query
   4. Evaluate locally, materialize the current matches
   5. Write a **static** iTunes playlist with those matches; grab the new PID
   6. Delete the original dynamic iTunes playlist
   7. Log: "Migrated iTunes smart playlist 'X' → app-owned smart + iTunes static"
3. Set `itunes_dynamic_migration_done = true`. Subsequent runs no-op unless new dynamic playlists appear (defensive catch, §5).

Readers needed: ITL Smart Criteria parser (prereq — verify 2026-03-27 parser covers it; if not, add in this PR).

Writers needed: none for Smart Criteria. Static playlist writer already exists.

### 5. Defensive catch — new iTunes dynamic playlists post-migration

If a user creates a new dynamic playlist directly in iTunes after migration, next iTunes sync pass detects it and surfaces a UI review dialog:

> "Found a new iTunes smart playlist 'X'. Import and convert to app-owned smart + delete iTunes dynamic?"

- **Yes** → runs the §4 migration path for that one playlist
- **No** → leave it alone, stop re-prompting (mark the PID as `declined`, user can change mind later in settings)

### 6. Ownership — simplified

All playlists in our DB are **app-owned**. No `itunes` or `forked` states (earlier design had these). The only transient state is "being migrated" — a flag cleared once the new app-owned version is created and the old iTunes dynamic is deleted.

### 7. Field mapping (iTunes → ours, one direction only)

Needed for reading iTunes Smart Criteria during migration. Reverse mapping not needed since we never write Smart Criteria.

| iTunes field | Our field | Notes |
|---|---|---|
| Name | `title` | |
| Album Artist | `author` | Primary mapping for audiobooks |
| Artist | `author` | Fallback if AA absent |
| Composer | `narrator` | Per audiobook tag convention |
| Grouping | `series` | |
| Genre | `genre` | |
| Year | `year` | |
| Kind | `format` | String translation |
| Time | `duration` | ms → seconds |
| Bit Rate | `bitrate` | |
| Has Artwork | `has_cover` | |
| Plays, Rating, Last Played, etc. | (flagged as untranslatable, stored in raw blob for reference) | |

Un-mappable iTunes fields get recorded as "approximation" markers in the translated query with a comment (`// approx: iTunes rule "Plays > 0" not directly representable`). User can refine after migration.

### 8. Operator mapping (iTunes → ours)

| iTunes operator | Our DSL |
|---|---|
| `contains` | `field:substring` |
| `does not contain` | `-field:value` |
| `is` | `field:=value` (future exact-match syntax — for MVP, approximate with substring) |
| `is not` | `-field:value` |
| `>`, `<` | `field:>N`, `field:<N` |
| `in range` | `field:>=A && field:<=B` (unfold, or future `field:[A TO B]`) |
| `All of the following` | AND (whitespace or `&&`) |
| `Any of the following` | `||` |
| Nested groups | `( ... )` |

### 9. Manual playlist UX (static)

- "Add to playlist" action on BookDetail, Library row, dedup view → dialog with existing static playlists + "+ New playlist"
- Playlist page: drag-reorder, remove per row, "+ Add books" library picker, rename, delete
- Bulk "Add to playlist" from Library toolbar depends on **5.3 Batch Select**; single-book works immediately

### 10. Smart playlist UX (dynamic)

- Single text box for query + live preview pane (first 20 matches, 300 ms debounced)
- Syntax-error highlighting: "expected `)`", "unknown field `autor` — did you mean `author`?"
- Field autocomplete dropdown from `SEARCH_FIELDS`
- "Materialize as Static" button → creates new `static` playlist with current matches, named `"{smart name} (snapshot 2026-04-15)"`

### 11. Permissions (ties to 3.7)

- `library.view` → see any playlist
- New `playlists.create` (default on `editor` + `admin`) → create / edit / delete own
- Admin can edit / delete any
- All users can duplicate-and-edit

## PebbleDB schema

```
playlist:{playlistID} → Playlist JSON {
    id, name, description,
    type,                       // "static" | "smart"
    book_ids,                   // static-only: ordered
    query, sort_json, limit,    // smart-only
    materialized_book_ids,      // smart-only: cached last materialization (for iTunes sync speed)
    itunes_persistent_id,       // null until first sync
    itunes_raw_criteria_b64,    // migration audit trail (only for playlists that came from iTunes migration)
    created_at, updated_at,
    created_by_user_id,
    dirty                       // pending iTunes push
}

user_playlist:{userID}:{playlistID}  → ""      (listing "my playlists")
playlist_name:{lcase-name}           → playlistID   (uniqueness + lookup)
itunes_playlist_map:{persistentID}   → playlistID   (reverse lookup during sync)
itunes_declined_pid:{persistentID}   → ""      (user said "don't migrate this one")
```

Settings keys:

```
setting:itunes_dynamic_migration_done → "true" | missing
```

## Query engine

Same as v1.0 design: parse → AST → predicate compiler → index-aware evaluation → sort + limit. No materialization cache in MVP; the `materialized_book_ids` field is only for iTunes-sync round-trips, not for in-app viewing.

## `Store` interface additions

```go
// CRUD
CreatePlaylist(*Playlist) (*Playlist, error)
GetPlaylist(id string) (*Playlist, error)
GetPlaylistByName(name string) (*Playlist, error)
GetPlaylistByITunesPID(pid string) (*Playlist, error)
ListPlaylists(filter PlaylistFilter, limit, offset int) ([]Playlist, int, error)
UpdatePlaylist(id string, *Playlist) (*Playlist, error)
DeletePlaylist(id string) error

// Static ops
AddBookToPlaylist(playlistID, bookID string, position int) error
RemoveBookFromPlaylist(playlistID, bookID string) error
ReorderPlaylist(playlistID string, bookIDs []string) error

// Dirty tracking for iTunes sync
MarkPlaylistDirty(id string) error
ListDirtyPlaylists() ([]Playlist, error)

// iTunes migration
DeclineITunesPlaylistMigration(pid string) error
IsITunesPlaylistDeclined(pid string) (bool, error)
IsITunesDynamicMigrationDone() (bool, error)
SetITunesDynamicMigrationDone() error
```

## MVP scope

1. Playlist schema + struct + CRUD
2. Query-parser extension (new operators, value alternation)
3. Predicate compiler + index-aware evaluator
4. ITL Smart Criteria **reader** (verify existence first; add if needed)
5. iTunes Smart Criteria → our DSL translator
6. One-time `itunes_dynamic_playlist_migration` tracked op
7. Defensive catch UI for post-migration iTunes dynamic playlists
8. iTunes sync pass: push `app`-owned playlists as static (materialize smart first)
9. Static editor UI: list page, drag-reorder, add/remove, "+ Add to Playlist" from BookDetail and Library row
10. Smart editor UI: text query + live preview + field autocomplete
11. Sidebar entry + playlist list page
12. "Materialize as Static" on smart playlists

## Deferred

- Bulk "Add to playlist" from Library multi-select (needs 5.3)
- Exact-match operator `field:=value`
- Tracking iTunes fields we don't have (Plays, Rating, Last Played) for better migration translation
- User-editable field mapping
- Playlist folders / nesting
- M3U / PLS export for user playlists
- Result caching keyed on `(query, library_version)`

## Risks

- **ITL parser may not read Smart Criteria.** Verify first. Reader is required for migration; writer is explicitly not needed (that's the simplification).
- **Lossy translation during migration.** Complex iTunes rules (Plays > N, Last Played in last N days) don't map. User sees the translated query, can refine. Raw blob stored for fidelity.
- **Post-migration iTunes dynamic discovery** could annoy users if they intentionally create dynamic playlists in iTunes. The "decline" option lets them opt out per-playlist.

## Non-goals

- Writing iTunes Smart Criteria (explicitly dropped — major simplification)
- Per-user playlist libraries (3.7 shared-library)
- Plex/Emby/Jellyfin compatibility (3.8's territory)
- Playlist import from M3U files

## Open implementation questions

- Does ITL parser already emit Smart Criteria? First read task.
- How expensive is re-materialization on library changes? If it becomes a hot path, debounce or move behind a library-change counter.
- Migration behavior if a user already has an app-owned playlist with the same name as an incoming iTunes dynamic → append suffix `(imported)` and surface in the review.
