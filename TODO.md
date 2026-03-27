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

## 🐛 Bugs — Confirmed Broken

| # | Item | Details |
|---|------|---------|
| B1 | **Author merge doesn't show all variants being merged** | Dedup/authors page: after selecting the canonical name, the variant list only shows one entry making it look like you're merging a name into itself. The other variant spellings (e.g., "J.T. Wright" vs "J. T. Wright") should all be listed so you can see what's actually being merged. |
| B2 | **Tag extraction reads conflicting metadata** | After writing correct tags, `ExtractMetadata` re-reads wrong values from legacy `composer`/`album` tags. Composer should be cleared when we write `artist`/`album_artist`. |
| B3 | **Author/narrator swap in many files** | `composer` = narrator in Audible M4Bs. Priority fix (album_artist > artist > composer) helps but doesn't solve files where `artist` IS the narrator. |
| B4 | **series_index not read back** | Custom SERIES_INDEX tag is written to files but `ExtractMetadata` doesn't read it back from the custom tag. |
| B5 | **35 remaining iTunes sync path errors** | Files genuinely missing on disk. May need cleanup or manual resolution. |
| B6 | **File version separator too faint** | In Files & History tag comparison, the horizontal line between two file versions (e.g., library copy vs iTunes copy) needs to be darker/bolder for clearer visual separation. |
| B7 | **Book detail page doesn't refresh after metadata apply** | After applying metadata, the page shows stale data. Need a refresh button on the book detail page. |

---

## 🚨 P0 — Must Fix (Blocks Daily Use)

| # | Item | Details | Spec/Plan |
|---|------|---------|-----------|
| 1 | **ISBN enrichment matched wrong book** | `isStrictTitleMatch` was too loose. Fixed to prefix match with 60% length ratio. Needs live testing. | [Implementation Guide §3](docs/implementation-guide.md#3-isbn-enrichment-validation) |

---

## 🔴 P1 — High Priority (New Features / Major Improvements)

| # | Item | Details | Spec/Plan |
|---|------|---------|-----------|
| 2 | **Preview Organize (single book)** | Merge "Preview Rename" and organize into one flow. Shows step-by-step preview (copy to library, rename, write tags, embed cover), then "Apply" executes. Currently can only organize in bulk. | — |
| 3 | **Playlist system** | Completely dead code. No API, no UI, no tagging. Need to decide: tag-based playlists vs stored playlists vs iTunes playlist sync. | — |
| 4 | **Bulk "Save to Files" for all books** | Currently per-book only. Need batch operation for tags + rename across all/filtered books. | [Implementation Guide §5](docs/implementation-guide.md#5-bulk-save-to-files) |
| 5 | **Series dedup cleanup** | 8,507 series for 10,891 books. Many 1-book series, some should be merged or removed. | [Implementation Guide §6](docs/implementation-guide.md#6-series-dedup) |
| 6 | **"read by narrator" books metadata fix** | ~100+ books still have title/author as "read by narrator". Need bulk AI-assisted correction. | [Diagnostics Page](docs/superpowers/specs/2026-03-14-diagnostics-export-design.md) |
| 7 | **M4B conversion live test** | Integration test passes but never tested on production with real files. | [Transcode Test](internal/transcode/transcode_integration_test.go) |

---

## 🟡 P2 — Medium Priority

| # | Item | Details | Spec/Plan |
|---|------|---------|-----------|
| 8 | **Activity page mobile layout** | Filter bar is cramped on mobile. Need collapsible filter drawer or responsive layout. | — |
| 9 | **Activity page auto-refresh speed** | 30s polling feels slow during active operations. Consider faster poll when ops are running, slower when idle. | — |
| 10 | **Version vs Snapshot UI polish** | Files & History tab needs UX polish and edge case handling. | [Files & History Spec](docs/superpowers/specs/2026-03-18-files-history-redesign.md) |
| 11 | **Compare snapshot wiring** | ChangeLog "Compare →" link communicates timestamp but doesn't load snapshot data. | [Implementation Guide §9](docs/implementation-guide.md#9-snapshot-comparison) |
| 12 | **Background ISBN enrichment expansion** | Should also run as scheduled maintenance for all books missing ISBN. | [Implementation Guide §10](docs/implementation-guide.md#10-scheduled-isbn-enrichment) |
| 13 | **Copy-on-write TTL tuning** | Verify TTL works and test disk usage on production. | [Implementation Guide §11](docs/implementation-guide.md#11-copy-on-write-verification) |
| 14 | **iTunes PID detail view expansion** | Show file paths, track names from XML. | [Implementation Guide §12](docs/implementation-guide.md#12-itunes-pid-detail-expansion) |
| 15 | **ITL write-back testing** | Experimental ITL copied and config set, but write-back still disabled. | — |
| 16 | **Remove TAG-DIAG instrumentation** | Diagnostic logging may still be in taglib_support.go. | — |

---

## 🟢 P3 — Nice to Have

| # | Item | Details | Spec/Plan |
|---|------|---------|-----------|
| 17 | **OpenAI batch polling rate limiting** | gpt-5.4 hit token limits. Need graceful handling. | [Diagnostics Spec](docs/superpowers/specs/2026-03-14-diagnostics-export-design.md) |
| 18 | **Deferred iTunes updates live test** | Deployed but untested on production. | [Deferred iTunes Spec](docs/superpowers/specs/2026-03-14-deferred-itunes-updates-design.md) |
| 19 | **Path format customization UI** | Settings page should expose path_format and segment_title_format. | — |
| 20 | **Migrate old logging tables to activity.db** | Convert operations/operation_logs/operation_changes/metadata_changes_history into activity summary entries. | [Activity Log Spec](docs/superpowers/specs/2026-03-25-unified-activity-page-design.md) |
| 21 | **Delete GitHub fork repos** | User needs to run `gh auth refresh` then delete jdfalk/go-taglib and jdfalk/go-taglib-1. | — |
| 22 | **Dynamic/smart playlist support** | Auto-updating playlists based on metadata queries. Depends on playlist system (#3). | — |
| 23 | **Database migration to PostgreSQL** | Research recommended PostgreSQL as next step (gives CockroachDB/YugabyteDB for free). | — |

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
