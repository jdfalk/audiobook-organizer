<!-- file: TODO.md -->
<!-- version: 4.0.0 -->
<!-- guid: 8e7d5d79-394f-4c91-9c7c-fc4a3a4e84d2 -->
<!-- last-edited: 2026-03-19 -->

# Project TODO

> All detailed plans live in [`docs/superpowers/plans/`](docs/superpowers/plans/) and specs in [`docs/superpowers/specs/`](docs/superpowers/specs/). Implementation guides for open items are in [`docs/implementation-guide.md`](docs/implementation-guide.md).

---

## 🎯 Current Status — March 19, 2026

**Library:** 10,891 books / 2,970 authors / 8,507 series (cleaned from 68K/6K/19K)
**Production:** PebbleDB, Linux, HTTPS at 172.16.2.30:8484

---

## 🚨 P0 — Must Fix (Blocks Daily Use)

| # | Item | Details | Spec/Plan |
|---|------|---------|-----------|
| 1 | **Tag extraction reads conflicting metadata** | After writing correct tags, `ExtractMetadata` re-reads and gets wrong values from legacy `composer`, `album`, or other conflicting tags. Composer should be cleared when we write `artist`/`album_artist`. | [Implementation Guide §1](docs/implementation-guide.md#1-clear-conflicting-tags-during-write) |
| 2 | **Author/narrator swap in many files** | `composer` field contains narrator in Audible M4Bs. Our fix (album_artist > artist > composer) helps but doesn't solve files where `artist` IS the narrator. Need to cross-reference with DB author. | [Implementation Guide §2](docs/implementation-guide.md#2-smart-author-narrator-resolution) |
| 3 | **ISBN enrichment matched wrong book** | `isStrictTitleMatch` was too loose. Fixed to prefix match with 60% length ratio. Needs live testing to confirm fix works. | [Implementation Guide §3](docs/implementation-guide.md#3-isbn-enrichment-validation) |

---

## 🔴 P1 — High Priority

| # | Item | Details | Spec/Plan |
|---|------|---------|-----------|
| 4 | **M4B conversion live test** | Integration test passes but never tested on production server with real files. Need to verify chapters, version linking, primary/non-primary, deferred iTunes update. | [Transcode Integration Test](internal/transcode/transcode_integration_test.go) |
| 5 | **Bulk "Save to Files" for all books** | Currently per-book only. Need a batch operation to write tags + rename for all books (or filtered set). | [Implementation Guide §5](docs/implementation-guide.md#5-bulk-save-to-files) |
| 6 | **Series dedup cleanup** | 8,507 series for 10,891 books. Many 1-book series, some should be merged or removed. | [Implementation Guide §6](docs/implementation-guide.md#6-series-dedup) |
| 7 | **"read by narrator" books metadata fix** | ~100+ books still have title/author as "read by narrator". Need bulk AI-assisted metadata correction. | [Diagnostics Page](docs/superpowers/specs/2026-03-14-diagnostics-export-design.md) |

---

## 🟡 P2 — Medium Priority

| # | Item | Details | Spec/Plan |
|---|------|---------|-----------|
| 8 | **Version vs Snapshot UI polish** | Tab renamed to "Files & History" with format trays, changelog, comparison. Needs UX polish and edge case handling. | [Files & History Spec](docs/superpowers/specs/2026-03-18-files-history-redesign.md) |
| 9 | **Compare snapshot wiring** | ChangeLog "Compare →" link communicates timestamp to TagComparison but doesn't load snapshot data yet. | [Implementation Guide §9](docs/implementation-guide.md#9-snapshot-comparison) |
| 10 | **Background ISBN enrichment expansion** | Currently searches after metadata apply. Should also run as a scheduled maintenance task for all books missing ISBN. | [Implementation Guide §10](docs/implementation-guide.md#10-scheduled-isbn-enrichment) |
| 11 | **Copy-on-write TTL tuning** | Hardlink backups created before tag writes. Cleanup task registered. Need to verify TTL works and test disk usage. | [Implementation Guide §11](docs/implementation-guide.md#11-copy-on-write-verification) |
| 12 | **iTunes PID detail view** | Badge clickable, banner expandable with PID table. Could show more info (file paths, track names from XML). | [Implementation Guide §12](docs/implementation-guide.md#12-itunes-pid-detail-expansion) |

---

## 🟢 P3 — Nice to Have

| # | Item | Details | Spec/Plan |
|---|------|---------|-----------|
| 13 | **OpenAI batch polling via app** | Universal batch poller deployed but gpt-5.4 hit token limits. Need to handle rate limiting gracefully. | [Diagnostics Spec](docs/superpowers/specs/2026-03-14-diagnostics-export-design.md) |
| 14 | **Deferred iTunes updates** | Table/code deployed. Untested on production (requires M4B transcode + iTunes write-back disabled scenario). | [Deferred iTunes Spec](docs/superpowers/specs/2026-03-14-deferred-itunes-updates-design.md) |
| 15 | **File count display** | Dashboard/Authors/Series pages show "N books (M files)" when counts differ. Verified working. | Completed |
| 16 | **Path format customization UI** | Settings page should expose path_format and segment_title_format for user editing. | — |

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
