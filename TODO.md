<!-- file: TODO.md -->
<!-- version: 4.0.0 -->
<!-- guid: 8e7d5d79-394f-4c91-9c7c-fc4a3a4e84d2 -->
<!-- last-edited: 2026-03-19 -->

# Project TODO

> All detailed plans live in [`docs/superpowers/plans/`](docs/superpowers/plans/) and specs in [`docs/superpowers/specs/`](docs/superpowers/specs/). Implementation guides for open items are in [`docs/implementation-guide.md`](docs/implementation-guide.md).

---

## 🎯 Current Status — March 26, 2026

**Library:** 10,891 books / 2,970 authors / 8,507 series
**Production:** PebbleDB, Linux, HTTPS at 172.16.2.30:8484
**Activity Log:** Unified page captures all server logs, replaces Operations page

---

## 🐛 Bugs — All Resolved (Session 15, Mar 25-27)

| # | Item | Status |
|---|------|--------|
| B1 | Author merge variant display | ✅ Fixed — shows merge target + all variant names |
| B2 | Tag extraction conflicting metadata | ✅ Fixed — composer cleared on write |
| B3 | Author/narrator swap | ✅ Mitigated by B2; full fix needs metadata pipeline redesign (P2) |
| B4 | series_index not read back | ✅ Already fixed — reads SERIES_INDEX/MVIN |
| B5 | 35 iTunes sync path errors | ✅ Not a bug — files genuinely missing on disk |
| B6 | File version separator too faint | ✅ Fixed — thicker separator |
| B7 | Book detail refresh after metadata | ✅ Fixed — refresh button + auto-refresh after operations |
| B8 | Write-back fails on multi-file books | ✅ Fixed — globs audio files in directory |

---

## 🚨 P0 — All Resolved

| # | Item | Status |
|---|------|--------|
| 1 | ISBN enrichment wrong matches | ✅ Validated — 60% length ratio fix working, all ISBNs valid |

---

## 🔴 P1 — All Resolved or Assessed

| # | Item | Status |
|---|------|--------|
| 2 | Preview Organize (single book) | ✅ Built — step-by-step preview with Apply button |
| 3 | Playlist system | ⏳ Assessed — 248 lines of data model, no API/UI. Needs brainstorming. |
| 4 | Bulk "Save to Files" | ✅ Built — `POST /api/v1/audiobooks/bulk-write-back` with filters + "Save All" button |
| 5 | Series dedup cleanup | ✅ Built — `POST /api/v1/maintenance/cleanup-series` (1-book removal + duplicate merge) |
| 6 | "read by narrator" fix | ✅ Built — `POST /api/v1/maintenance/fix-read-by-narrator` (dry_run by default) |
| 7 | M4B conversion live test | ⏳ Local tests pass. Needs user supervision for production test. |

---

## 🟡 P2 — Current Priority

| # | Item | Details | Status |
|---|------|---------|--------|
| 8 | **Activity page mobile layout** | Collapsible filter drawer for mobile | ✅ Fixed |
| 9 | **Activity page adaptive refresh** | 5s when ops running, 30s when idle | ✅ Fixed |
| 10 | **Version vs Snapshot UI polish** | Edge case handling | Open |
| 11 | **Compare snapshot wiring** | Load snapshot data on Compare click | ✅ Partially fixed — falls back to activity log |
| 12 | **Background ISBN enrichment** | Scheduled maintenance task | ✅ Already implemented |
| 13 | **Copy-on-write TTL tuning** | Verify on production | ✅ Verified — 23 files, TTL working |
| 14 | **iTunes PID detail view** | Show file paths, track names | ✅ Fixed — shows file paths and track PIDs |
| 15 | **ITL write-back testing** | 176K tracks written to ITL copy | ✅ Working — needs user to test with iTunes |
| 16 | **TAG-DIAG instrumentation** | Clean up diagnostic logging | ✅ Already cleaned up |
| 17 | **Author/narrator swap (full fix)** | Cross-reference DB author during tag extraction | Open — needs design |
| 18 | **3-state library badge** | scanned → imported → organized states | Open — needs brainstorming |
| 19 | **Vite chunk splitting** | Code-split large JS bundle | ✅ Fixed — index 514KB → 107KB |
| 20 | **Stale interrupted operations** | Mark as failed on startup | ✅ Fixed |
| 21 | **Settings save/buttons not visible** | Sticky floating buttons | ✅ Already implemented |
| 22 | **iTunes sync dialog pre-fill from config** | Auto-fill XML path | ✅ Fixed |
| 23 | **iTunes sync should support ITL** | Read directly from ITL | Open — tied to XML deprecation |
| 24 | **Force Import always greyed out** | Disabled condition fixed | ✅ Fixed |
| 25 | **ITL write-back — multi-file books** | Per-file itunes_path via book_files | ✅ Fixed — 176K tracks updated |
| 26 | **Files & History layout — separate version boxes** | Path-based labels when same format | ✅ Fixed |
| 27 | **Files & History — show individual files** | book_files API + frontend | ✅ Fixed — 114K files tracked |
| 28 | **Track PIDs sorted by track number** | Sorted in iTunes panel | ✅ Fixed |
| 29 | **Deprecate XML functions** | Read from ITL directly | Open — long term |

---

## 🟢 P3 — Nice to Have

| # | Item | Details | Spec/Plan |
|---|------|---------|-----------|
| 21 | **Playlist system (full)** | Tag-based vs stored vs iTunes sync. Smart playlists based on play counts. Bidirectional sync to iTunes. Needs brainstorming. | — |
| 29 | **Empty folder cleanup** | Audiobook organizer folder has empty directories from renames. Need maintenance task to clean up. | — |
| 30 | **External library sync abstraction** | Sonarr-style pluggable connectors for syncing with external libraries (iTunes, future apps). Needs brainstorming. | — |
| 22 | **OpenAI batch polling rate limiting** | gpt-5.4 hit token limits. | [Diagnostics Spec](docs/superpowers/specs/2026-03-14-diagnostics-export-design.md) |
| 23 | **Deferred iTunes updates live test** | Deployed but untested. | [Deferred iTunes Spec](docs/superpowers/specs/2026-03-14-deferred-itunes-updates-design.md) |
| 24 | **Path format customization UI** | Expose in Settings. | — |
| 25 | **Migrate old logging tables** | Convert to activity summaries. | [Activity Log Spec](docs/superpowers/specs/2026-03-25-unified-activity-page-design.md) |
| 26 | **Delete GitHub fork repos** | `gh repo delete jdfalk/go-taglib`. | — |
| 27 | **Dynamic/smart playlists** | Auto-updating based on queries. | — |
| 28 | **Database migration to PostgreSQL** | Research recommended. | — |
| 29 | **Add ffmpeg to production Dockerfile** | Runtime stage (`alpine:3.23`) is missing ffmpeg — transcoding, cover art embedding, and M4B tag writing silently fail in Docker. Add `apk add --no-cache ffmpeg`. | — |

---

## ✅ Recently Completed (Session 12-13, Mar 14-19)

<details>
<summary>49 commits — click to expand</summary>

### Data Cleanup
- Cleaned library from 68K → 10.9K books (84% reduction)
- Cleaned authors from 6K → 2.9K, series from 19K → 8.5K
- Deleted 15K same-path duplicates, 5K same-format duplicates, 2.9K unmatched organizer copies
- Merged 1.3K duplicate series, removed 7.3K empty series
- Removed 2.3K empty authors
- Stripped numeric title prefixes from 278 books
- Removed fake numeric series assignments from 332 books
- All ULID version groups converted to vg- style
- All version groups have a primary version set

### Features Built
- **Diagnostics page** — ZIP export, AI batch analysis, 4 categories, results review panel
- **External ID mapping** — migration 34, 97K PID mappings, merge/delete/tombstone support
- **Files & History tab** — format-grouped trays, TagComparison with dropdown, ChangeLog timeline
- **Background ISBN/ASIN enrichment** — after metadata apply
- **Bulk batch-operations API** — per-item update/delete/restore
- **Universal batch poller** — metadata tags on all batches, routes by type
- **Deferred iTunes updates** — migration 33, post-transcode hook, auto-apply on sync
- **File path history** — migration 35, records renames
- **Genre field** — migration 36, stored from metadata fetch
- **Copy-on-write backups** — hardlinks before tag writes, TTL cleanup task
- **Revert buttons** in ChangeLog entries (DB + file revert)

### Bug Fixes
- Metadata sync to library copies (stale data on tag write)
- `books.file_path` updated after segment rename
- iTunes files protected from `os.Rename` in apply pipeline
- Soft-deleted list uncapped (was 500, now 10K)
- Operation resume after server restart
- Reconcile scan visible in operations UI
- Operations list stable sort by created_at
- Save to Files now renames + cleans empty dirs
- Single-file books get virtual segment for rename
- Single-file naming: `{title}.{ext}` not `{title} - 1/1.{ext}`
- Tag extraction: album_artist > artist > composer priority
- Honest write-back counting (no false "written" messages)
- stripChapterFromTitle strips leading dashes
- Search by author/narrator without title
- Fetch metadata can't wipe title to Untitled
- ISBN enrichment strict title matching
- Read custom tags back (SERIES_INDEX, PUBLISHER, MVNM/MVIN)
- Write ALL metadata to file tags (series, publisher, language, ISBN, description)
- iTunes path removed from scanner import paths (prevented double import)
- Purge protects books with iTunes PIDs

</details>
