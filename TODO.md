<!-- file: TODO.md -->
<!-- version: 4.0.0 -->
<!-- guid: 8e7d5d79-394f-4c91-9c7c-fc4a3a4e84d2 -->
<!-- last-edited: 2026-03-19 -->

# Project TODO

> All detailed plans live in [`docs/superpowers/plans/`](docs/superpowers/plans/) and specs in [`docs/superpowers/specs/`](docs/superpowers/specs/). Implementation guides for open items are in [`docs/implementation-guide.md`](docs/implementation-guide.md).

---

## 🎯 Current Status — April 6, 2026

**Library:** 24,022 books / 3,341 authors / 10,924 series (migration 45)
**Production:** PebbleDB, Linux, HTTPS at 172.16.2.30:8484
**iTunes:** ITL write-back with LE track add/remove, metadata write-back to ITL
**New this session:** Bulk metadata review, ITL format documentation, ACL permission fixes

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
| 10 | **Version vs Snapshot UI polish** | Edge case handling | ✅ Fixed — empty trays, auto-expand single-file |
| 11 | **Compare snapshot wiring** | Load snapshot data on Compare click | ✅ Partially fixed — falls back to activity log |
| 12 | **Background ISBN enrichment** | Scheduled maintenance task | ✅ Already implemented |
| 13 | **Copy-on-write TTL tuning** | Verify on production | ✅ Verified — 23 files, TTL working |
| 14 | **iTunes PID detail view** | Show file paths, track names | ✅ Fixed — shows file paths and track PIDs |
| 15 | **ITL write-back testing** | 176K tracks written to ITL copy | ✅ Working — needs user to test with iTunes |
| 16 | **TAG-DIAG instrumentation** | Clean up diagnostic logging | ✅ Already cleaned up |
| 17 | **Author/narrator swap (full fix)** | Post-extraction guard + maintenance endpoint | ✅ Built — deploy pending |
| 18 | **Library state badges** | imported=amber, organized=green, filter, dashboard count | ✅ Fixed |
| 19 | **Vite chunk splitting** | Code-split large JS bundle | ✅ Fixed — index 514KB → 107KB |
| 20 | **Stale interrupted operations** | Mark as failed on startup | ✅ Fixed |
| 21 | **Settings save/buttons not visible** | Sticky floating buttons | ✅ Already implemented |
| 22 | **iTunes sync dialog pre-fill from config** | Auto-fill XML path | ✅ Fixed |
| 23 | **iTunes sync from ITL directly** | ParseLibrary auto-detects XML vs ITL | ✅ Fixed — point sync at ITL file and it just works |
| 24 | **Force Import always greyed out** | Disabled condition fixed | ✅ Fixed |
| 25 | **ITL write-back — multi-file books** | Per-file itunes_path via book_files | ✅ Fixed — 176K tracks updated |
| 26 | **Files & History layout — separate version boxes** | Path-based labels when same format | ✅ Fixed |
| 27 | **Files & History — show individual files** | book_files API + frontend | ✅ Fixed — 114K files tracked |
| 28 | **Track PIDs sorted by track number** | Sorted in iTunes panel | ✅ Fixed |
| 29 | **Deprecate XML functions** | ParseLibrary now auto-detects ITL vs XML. Can switch sync to ITL path. | ✅ Infrastructure done — XML still works as fallback |

---

## 🔴 P1 — Active Issues (April 6, 2026)

| # | Item | Details | Status |
|---|------|---------|--------|
| 30 | **Background file ops need graceful tracking** | Cover embed, tag write, rename are fire-and-forget goroutines. Lost on restart. Users told "applied" when file writes haven't happened. | ✅ Fixed — persistent tracking in PebbleDB, startup recovery |
| 31 | **Resume interrupted metadata fetch on startup** | If server restarts mid-fetch, already-fetched results survive but remaining books are lost. Need startup recovery. | ✅ Fixed — saves book_ids as params, resumes remaining on startup |
| 32 | **Aggressive search/book result caching** | Books rarely change — cache search results 30-60s, individual book lookups, metadata candidates. | ✅ Fixed — list 30s, metadata search 30s |
| 33 | **Batch apply still fires separate requests per click** | Frontend coalesces with 500ms debounce but rapid clicks still result in multiple API calls. Need true client-side queue. | ⏳ Partially fixed |

---

## 🟡 P2 — CI/CD & Lint Fixes (April 6, 2026)

| # | Item | Details | Status |
|---|------|---------|--------|
| 34 | **E2E test lint errors (10 errors)** | Unused `page` params, unused imports, unnecessary escapes in Playwright tests. Files: `file-browser.spec.ts`, `error-handling.spec.ts`, `dynamic-ui-interactions.spec.ts`, `demo-full-workflow.spec.ts`, `dashboard.spec.ts`, `core-functionality.spec.ts`, `book-detail.spec.ts`, `batch-operations.spec.ts` | ⏳ |
| 35 | **Frontend lint warnings (11 warnings)** | `Unexpected any` in Settings.tsx/BookDedup.tsx, missing useEffect deps in BookDedup.tsx, unnecessary useCallback dep in Library.tsx, Fast refresh warnings in main.tsx/AuthContext.tsx/ToastProvider.tsx/ConfigurableTable.tsx | ⏳ |
| 36 | **GitHub Actions Node.js 20 deprecation** | `actions/setup-node` running Node.js 20, deprecated June 2026. Update to Node.js 24 compatible version. | ⏳ |

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
