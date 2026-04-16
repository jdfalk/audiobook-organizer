<!-- file: docs/superpowers/specs/2026-04-15-read-unread-tracking-design.md -->
<!-- version: 1.0.0 -->
<!-- guid: 9a2c7e4f-5b3d-4f80-a6c1-8d0e2f1b9a65 -->

# Read/Unread Tracking Beyond iTunes — Design

**Status:** Design complete (Apr 15, 2026). Ready for implementation plan.
**Scope item:** TODO.md §3.6.
**Depends on:** 3.7 (multi-user — per-user attribution), existing iTunes ITL parser and bookmark field handling.

## Goal

First-class per-user read/progress tracking that works whether a book is in iTunes or not, and that supports multiple users each with their own progress against the shared library. iTunes sync is bidirectional for the attributed admin user using iTunes's `Bookmark` field (reliable for Audiobook-kind tracks).

## Locked decisions

### 1. Per-user state, shared library

All books / authors / series are shared (per 3.7). **Read progress is per-user.** Alice having finished *Oathbringer* doesn't mark it finished for Bob.

### 2. Two layers of state

| Layer | Kind | Granularity |
|---|---|---|
| **Play position** | Continuous | Per `(user, book_file)` — second-precision offset into a segment |
| **Read status** | Discrete | Per `(user, book)` — `unstarted` / `in_progress` / `finished` / `abandoned` |

Read status is auto-computed from play position but user can manually override (e.g., "finished it in paperback elsewhere").

### 3. iTunes interop — `Bookmark` is the real position

iTunes stores per-track position in the `Bookmark` field (seconds, float) for Audiobook-kind tracks, reliably. Our model:

- **On import/sync**: for each iTunes-sourced book_file with `Bookmark > 0`, seed the attributed admin user's position — one position row per book_file
- **On admin position writes**: write back to iTunes `Bookmark` on next iTunes sync pass (so iPod / Apple player picks up the updated position)
- **Other users' positions**: live in our DB only; iTunes has no multi-user model and we don't invent one on its side

Attributed user: admin by default (MVP). Configurable later to "my user" for self-hosted single-user installs via a setting.

### 4. Position tracking granularity

Position keyed on `book_file` (segment), carrying:

- `segment_id` — which chapter/file
- `position_seconds` — offset into that file
- `updated_at` — last event

"Current position in the book" is `(last_updated_segment, position_seconds)`. Percent-through and time-remaining are computed on the fly from segment durations in `book_files.duration`.

### 5. Event-based updates

A player (our UI, future mobile, Plex-style client) sends a position event whenever:

- User pauses
- User seeks
- User closes the book / navigates away
- Periodically during playback (configurable heartbeat; default 30 s)

Server upserts `(user, segment)` → position. No event queue, no write batching — rates are low (≤ 2/s per user). Writes also recompute and cache read status.

### 6. Read-status auto-compute

Rule: **`finished` when user has listened past 95% of total book duration.** Sum of (fully-played segments' duration) + (current segment position) ≥ 0.95 × total book duration.

- `unstarted` — no position events ever
- `in_progress` — some position events, < 95% consumed
- `finished` — auto-flipped when 95% threshold crossed
- `abandoned` — user-set only; no auto trigger

### 7. Manual override

UI offers "Mark as finished / unstarted / abandoned." Setting a manual value flips `status_manual = true` — the status no longer auto-updates even if position changes later. Clearing the manual override ("Use automatic status") resumes auto-computation from current position.

## PebbleDB schema

```
user_position:{userID}:{bookID}:{segmentID}   → UserPosition JSON {
                                                    position_seconds,
                                                    updated_at
                                                 }
user_book_state:{userID}:{bookID}             → UserBookState JSON {
                                                    status,
                                                    status_manual,
                                                    last_activity_at,
                                                    last_segment_id,
                                                    total_listened_seconds,  // cached for fast % calc
                                                    progress_pct             // cached 0-100
                                                 }
book_user:{bookID}:{userID}                   → ""    (reverse lookup — "who has progress on this book")
```

No position history — latest wins. If future work wants "Continue Listening" history, that's a separate spec.

## API

```
POST   /api/v1/books/:id/position
  body: { segment_id, position_seconds }
  → upserts user_position, recomputes user_book_state, schedules iTunes write-back if caller is the attributed admin user and book has an iTunes PID

GET    /api/v1/books/:id/position
  → latest position for the calling user (or 404 if none)

GET    /api/v1/books/:id/state
  → UserBookState for the calling user

PATCH  /api/v1/books/:id/status
  body: { status: "finished" | "unstarted" | "abandoned" }
  → set status_manual=true, write status

DELETE /api/v1/books/:id/status
  → clear manual override, recompute from position

GET    /api/v1/me/in-progress?limit=N
GET    /api/v1/me/finished?limit=N
  → lists sorted by last_activity_at DESC
```

Permissions:

- Calling user reads/writes only their own state (implicit self-scope, no extra permission)
- `library.view` required as baseline
- Reading others' state: reserved for future `users.view_state` permission — not MVP

## Search / filter / smart playlist integration

Adds to `SEARCH_FIELDS` (evaluated against the calling user's state):

- `read_status` — `unstarted` / `in_progress` / `finished` / `abandoned`
- `last_played` — timestamp; supports `within_days:N`, `before:YYYY-MM-DD`, `after:YYYY-MM-DD`
- `progress_pct` — integer 0-100; supports `:>`, `:<`, `:>=`, `:<=`, range

Smart playlists like "Currently Reading" (`read_status:in_progress`) and "Abandoned" (`read_status:abandoned`) fall out naturally. See §3.4 design for playlist spec.

## UI

- **BookDetail**: prominent read-status chip, "X h Y m left" resume button (jumps to `last_segment_id` + `position_seconds`), "Mark as …" menu for override, "Use automatic" when manually overridden
- **Library**: optional column for read status (hidden by default, toggleable via existing column config); optional column for progress bar
- **Sidebar**: "Currently Reading" and "Finished" entries (quick filters, not saved smart playlists)

## iTunes sync interaction

Bidirectional for the attributed admin user only:

- **Pull** (iTunes → app): during iTunes sync, for each iTunes-sourced track with `Bookmark > 0`, upsert admin user's position
- **Push** (app → iTunes): during iTunes sync, for each admin position update since last sync (tracked via `updated_at > last_itunes_sync_at`), write back the `Bookmark` field for the corresponding iTunes track
- Status isn't directly represented in iTunes (no "finished" flag per track). Play count + play date in iTunes map to our signal loosely:
  - If `play_count > 0` in iTunes and we have no admin state → seed `finished` (assume listened)
  - If we flip admin to `finished` → increment iTunes `play_count` by 1 and set `Played Date` to now
  - Non-admin users' status never touches iTunes

## Migration / backfill

One-time at feature rollout:

1. For each iTunes-sourced book with `play_count > 0` or `Bookmark > 0`:
   - Create admin's `user_book_state` row
   - Seed `status = finished` if `play_count > 0` else `in_progress` if `Bookmark > 0`
   - `last_activity_at = max(last_played, bookmark_updated)` from ITL
   - For each `book_file` with a Bookmark, create `user_position` row for admin
2. No backfill for non-admin users — they start fresh

## `Store` interface additions

```go
// Position
SetUserPosition(userID, bookID, segmentID string, positionSeconds float64) error
GetUserPosition(userID, bookID string) (*UserPosition, error)   // latest across segments
ListUserPositionsForBook(userID, bookID string) ([]UserPosition, error)  // per-segment list
ClearUserPosition(userID, bookID string) error

// Status
SetUserBookStatus(userID, bookID, status string, manual bool) error
GetUserBookState(userID, bookID string) (*UserBookState, error)
ClearUserManualStatus(userID, bookID string) error

// Lists
ListInProgressForUser(userID string, limit, offset int) ([]UserBookState, error)
ListFinishedForUser(userID string, limit, offset int) ([]UserBookState, error)

// Sync helpers
ListAdminPositionsSince(t time.Time) ([]UserPosition, error)  // for iTunes write-back pass
```

## MVP scope

1. PebbleDB keys + struct types
2. `Store` method impls (Pebble + mock)
3. Position + status HTTP endpoints
4. BookDetail UI: status chip, resume button, override menu
5. Library column (hidden by default)
6. Search integration: `read_status`, `last_played`, `progress_pct`
7. iTunes backfill for existing admin data (one-time)
8. iTunes sync bidirectional for admin's `Bookmark`
9. "Currently Reading" + "Finished" sidebar entries

## Deferred

- Position-event history (would enable "Resume your session" UX)
- Cross-user analytics / admin dashboards (`users.view_state` permission)
- Non-admin user iTunes attribution (would need per-user iTunes library, out of scope)
- Listening-based recommendations (explicitly out of scope per TODO.md §8)
- Per-segment re-listen flags / bookmarks beyond position

## Risks

- **Position write volume.** Heartbeat at 30 s = 120 writes/hour per active listener. Multiply by N users. PebbleDB handles this trivially, but the recomputation of `user_book_state` on every position write could become hot. Mitigation: recompute only on segment boundary crosses, not every heartbeat. Position upsert is always cheap; status recompute is periodic.
- **iTunes Bookmark drift.** If user listens in our app, then in iTunes via iPod, Bookmark in iTunes advances independently. Next sync would see a forward-jumped Bookmark, write it to our position — correct. If both edit between syncs, last-sync-wins; acceptable for a "roughly keep in sync" model.
- **Multi-file position sanity.** `last_segment_id` becomes the resume point; if user seeks to segment 5 then segment 2, the "current" segment is whichever has the most recent `updated_at`, not the highest segment number. Keep the simple "most recently updated" rule.

## Non-goals

- Shared/global "has this book been read by anyone" signal — explicitly per-user
- Collaborative listening / watch-together
- Auto-marking-finished across users (no global sync of status)
- Writing status tags (beyond iTunes play count) back to the files themselves

## Open implementation questions

- Exact granularity of iTunes `Bookmark` — seconds float, ms, sample count? Confirm in ITL parser.
- Does the existing player UI have hooks for position events, or do we need to add them? Probably add — no audio player was top-of-mind before this.
- Browser audio player or rely on external players? In-app player would need streaming endpoint (ties into 3.8 Plex-style API).
