<!-- file: TODO.md -->
<!-- version: 5.2.0 -->
<!-- guid: 8e7d5d79-394f-4c91-9c7c-fc4a3a4e84d2 -->
<!-- last-edited: 2026-04-16 -->

# Project TODO

Canonical index into every piece of outstanding work across the project.
Details live in the linked files; this file exists so anyone (you, me, a
future agent) can scan the entire workspace in one page.

**Sources indexed here:**
- [`docs/backlog-2026-04-10.md`](docs/backlog-2026-04-10.md) — 1725-line working list, ranked by category
- [`docs/superpowers/plans/`](docs/superpowers/plans/) — implementation plans per feature
- [`docs/superpowers/specs/`](docs/superpowers/specs/) — design specs per feature
- [`docs/implementation-guide.md`](docs/implementation-guide.md) — integration guide for open items
- Claude project memory at `~/.claude/projects/-Users-jdfalk-repos-github-com-jdfalk-audiobook-organizer/memory/` — items still to graduate here

---

## 🎯 Current Status — April 16, 2026

**Library:** 10,891 books / 2,970 authors / 8,507 series (cleaned)
**Production:** PebbleDB, Linux, HTTPS at `172.16.2.30:8484`, mTLS bridge active
**Latest shipped release:** v0.210.0 (2026-04-16)
**In flight:** Backend foundations for 6 major features (see below)

---

## 🔧 CI / Release Infrastructure — In Progress (April 14, 2026)

The default `GITHUB_TOKEN` can't push refs whose reachable commits
modify `.github/workflows/` files. This blocked v0.207.0. Fix is a
GitHub App that mints short-lived tokens with `workflows: write`.

**Cross-repo work** (ghcommon, audiobook-organizer, release-go-action,
gha-release-go):

- [x] Revert corrupted `release-go-action/action.yml` (2 MCP push bugs from earlier session)
- [x] `ghcommon/scripts/setup-ci-app.sh` — one-shot GitHub App creator + secret distributor
- [x] `ghcommon/reusable-release.yml` — stale draft + superseded-RC auto-cleanup on stable cuts
- [x] `ghcommon/reusable-release.yml` — keep-5 most-recent RCs policy (`RC_KEEP_COUNT`)
- [ ] Create `jdfalk-ci-bot` GitHub App via manifest flow (one browser click)
- [ ] Distribute `CI_APP_ID` + `CI_APP_PRIVATE_KEY` to: audiobook-organizer, ghcommon, release-go-action, gha-release-go, release-frontend-action, gha-release-frontend, release-docker-action, gha-release-docker
- [ ] Install App on all target repos
- [ ] Patch `release-go-action/action.yml` — token-in-URL push using `github-token` input
- [ ] Wire `github-token` input through `gha-release-go`
- [ ] Wire `actions/create-github-app-token@v1` into `ghcommon/reusable-release.yml`
- [ ] Cut production release `v0.207.0`
- [ ] Cut production release `v0.208.0`

**Session memory:** `project_session_state.md` has full context of what broke and why.

---

## 📋 Backlog — [full file](docs/backlog-2026-04-10.md)

Statuses below reflect the current state including v0.206.0's shipped
work (many items marked "open" in the backlog file were quietly shipped
since it was last edited on 2026-04-11).

### 1. Dedup & Library Integrity — [section](docs/backlog-2026-04-10.md#1-dedup--library-integrity)

- [x] **1.1** `book_alternative_titles` schema + engine integration (#234)
- [x] **1.2** Duration-based similarity signal (shipped v0.206.0, commit `4c6139e`)
- [x] **1.3** Dedup scan as a real Operation (#227)
- [x] **1.4** LLM verdict auto-apply above confidence threshold (shipped v0.206.0, commit `28257a9`)
- [ ] **1.5** Side-by-side metadata diff in cluster card (**M**)
- [x] **1.6** Import-time collision preview (**M**) — #343
- [x] **1.7** Per-side "merge into this" quick action (#230)
- [x] **1.8** Smarter "split cluster" with edge preview (#233)
- [x] **1.9** Series-aware bulk merge (#232)
- [x] **1.10** Export dedup state as CSV/JSON (#231)

### 2. Known Bugs — all closed in #227

- [x] **2.1** Activity log compact "Everything (now)" returns 0
- [x] **2.2** Dedup scan isn't tracked in Operations (see 1.3)
- [x] **2.3** Dedup scan has no completion messages
- [x] **2.4** Directory organize has no cleanup on partial failure
- [x] **2.5** Scanner may double-count iTunes + organized paths as separate books
- [x] **2.6** `GetAllBooks` is O(n²) when called in a loop
- [x] **2.7** Auto-scan file watcher only watches one import path

### 3. Features — [section](docs/backlog-2026-04-10.md#3-features)

- [x] **3.1** Library centralization / `.versions/` layout (**L**) — 9/10 tasks (#296, #306, #315-#316, #324-#325, #337)
- [x] **3.2** Bulk organize undo via `operation_changes` (**M**) — 6/7 tasks (#318-#319, #326, #332)
- [x] **3.3** Bulk edit metadata across selected books (shipped v0.206.0)
- [x] **3.4** Smart playlists (**M**) — complete 9/9 (#307-#309, #338-#340)
- [ ] **3.5** Cover art browse/restore UI (**S**)
- [x] **3.6** Read/unread tracking (**M**) — complete 8/8 (#300, #303, #317, #331, #336)
- [x] **3.7** Multi-user support (**L**) — complete 8/8 (#292-#295, #313-#314, #322, #334)
- [ ] **3.8** Plex-style HTTP media server API (**L**)
- [ ] **3.9** LLM-based series detection and ordering (**M**)
- [ ] **3.10** AI-generated cover art when none exists (**S**)

### 4. Architecture / Future-Proofing — [section](docs/backlog-2026-04-10.md#4-architecture--future-proofing)

- [ ] **4.1** PostgreSQL research track (**XL**)
- [x] **4.2** Split the monolithic `server.go` (commit `c858ceb`)
- [x] **4.3** Move write-back queue to a durable outbox (**M**) — #344
- [x] **4.4** Replace `database.GlobalStore` package var with DI (**L**) — complete (#280-#291)
- [ ] **4.5** Property-based tests for dedup engine (**M**)
- [ ] **4.6** Chaos tests for the embedding store under shutdown (**M**)
- [ ] **4.7** Per-workload store evaluation: Pebble vs SQLite vs PostgreSQL vs Go-native NoSQL (**L** research)

### 5. UX / DX Polish — [section](docs/backlog-2026-04-10.md#5-ux--dx-polish)

- [x] **5.1** Search inside the dedup tab (shipped v0.206.0, commit `191faa3`)
- [x] **5.2** "Similar books" lookup on BookDetail page (**S**) — #342
- [ ] **5.3** Batch select in library view (**S**)
- [x] **5.4** Better error messages on organize failures (#273)
- [x] **5.5** Dev mode "seed library" command (#274)
- [ ] **5.6** Frontend test coverage baseline (**M**)
- [ ] **5.7** API documentation (**M**)
- [ ] **5.8** Regenerate ITL test fixtures after format work (**S**) — prereq for 7.9
- [x] **5.9** Enforce mockery-generated mocks via CI gate (commit `45492c3`)

### 6. Integration / Ecosystem — [section](docs/backlog-2026-04-10.md#6-integration--ecosystem)

- [ ] **6.1** Deluge `move_storage` integration (**M**) — pairs with 3.1
- [x] **6.2** Audnexus + Hardcover full integration (#7daef15)
- [x] **6.3** Tag writeback to iTunes via ITL updates (shipped previously)
- [ ] **6.4** ITL upload / download / partial export (**M**)

### 7. Tagging as Infrastructure — [section](docs/backlog-2026-04-10.md#7-tagging-as-infrastructure)

Underlying tag plumbing shipped in #244. Most items below are follow-ons
that layer on that foundation.

- [ ] **7.1** Tag-based policies / preference inheritance (**L**) — depends on 7.2
- [x] **7.2** Language filter in metadata review (shipped v0.206.0, commit `df6c9bd`)
- [x] **7.3** Metadata-apply tagging — source + language (shipped v0.206.0, commit `441fd43`)
- [x] **7.4** Google Books → Audible auto-upgrade maintenance job (shipped v0.206.0, commit `24201d4`)
- [x] **7.5** Metadata fetch caching (shipped v0.206.0, commit `2080c87`)
- [x] **7.6** Persistent review dialog + concurrent review during fetch (shipped v0.206.0, commit `1d2bf53`)
- [ ] **7.7** Author and series tag HTTP endpoints + frontend (**M**) — store parity shipped in #244; HTTP/UI remain
- [x] **7.8** System tag UX — visual distinction user vs system (shipped v0.206.0, commit `4dda739`)
- [ ] **7.9** Full iTunes library regenerate / rebuild (**L**) — diff-and-batch mode shipped (commit `286140d`); full rebuild-from-scratch mode remains
- [x] **7.10** Archive sweep for soft-deleted books (**M**) — #342
- [x] **7.11** Author/series merge — sync denormalized `book.AuthorID` (shipped v0.206.0, commit `f244824`)
- [x] **7.12** Organize rewrites file tags on every run even when unchanged (shipped v0.206.0, commit `2d4ad01`)

### 8. Out of Scope / Decide Later — [section](docs/backlog-2026-04-10.md#8-out-of-scope--decide-later)

Intentionally deferred. Captured here so they don't resurface as new ideas.

- iOS / Android companion app (scope explosion)
- WebDAV browse of the library (niche)
- RSS / Atom feed of new additions (niche)
- Notification system (Slack / Discord when scan completes) (rabbit hole)
- Cross-library federation (architecturally premature)
- Voice control / Alexa skill (out of focus)
- Audio preview in dedup tab — play first 30 seconds (requires streaming infra)
- "Recommended for you" based on listening history (no listening history store)
- Book recommendation engine (same)

---

## 🧠 From Memory — items not yet in the backlog file

These surfaced in later sessions and live only in Claude project memory.
Promote to `docs/backlog-2026-04-10.md` (or a successor) when touched.

### Graceful File Ops — 1 remaining gap

Full details: [`memory/project_graceful_file_ops.md`](../../.claude/projects/-Users-jdfalk-repos-github-com-jdfalk-audiobook-organizer/memory/project_graceful_file_ops.md)

- [x] **GFO-1** UI indicator for in-flight file ops + `GET /api/v1/file-ops/pending` (#270)
- [x] **GFO-2** Per-book tracking key collision — moved to `pending_file_op:{bookID}:{opType}` (#270)
- [x] **GFO-3** Resumable ops — `bulk_write_back`, `isbn-enrichment`, `metadata-refresh` (#270), `reconcile_scan` (#272). ~13 cleanup/maintenance types still silently fail on restart but are low-impact.
- [ ] **GFO-4** No sub-operation phase tracking — if ffmpeg (cover) succeeds but taglib (tags) fails, recovery re-runs ffmpeg pointlessly. Add phase checkpoints inside the apply pipeline
- [x] **GFO-5** `GET /operations/recent` ~900ms — fixed by replacing O(N²) bubble sort with `sort.Slice` (#270). Side-index deferred until benchmarks show it's needed.

### Bulk Metadata Review — Audible series format bug

Full details: [`memory/project_bulk_metadata_review.md`](../../.claude/projects/-Users-jdfalk-repos-github-com-jdfalk-audiobook-organizer/memory/project_bulk_metadata_review.md)

- [x] **BMR-1** Audible "Series, Book N" baked into series field — `normalizeMetaSeries` now runs in `ApplyMetadataCandidate` too, not just the auto-fetch paths (#271)

### Design Spec Already Written (but not yet planned)

- [x] **DES-1** Bleve library search — complete 6/7 (#298, #301-#302, #311-#312, #321)
- [ ] **DES-2** chromem-go embedding store — spec at [`docs/superpowers/specs/2026-04-11-chromem-go-embedding-store.md`](docs/superpowers/specs/2026-04-11-chromem-go-embedding-store.md)

---

## 📚 Implementation Plans — [`docs/superpowers/plans/`](docs/superpowers/plans/)

Every plan in chronological order. ✅ = implemented, ⏳ = design done, plan written, not yet executed.

- [x] [2026-03-10 Central logger](docs/superpowers/plans/2026-03-10-central-logger.md)
- [x] [2026-03-10 Incremental scan](docs/superpowers/plans/2026-03-10-incremental-scan.md)
- [x] [2026-03-12 Unified maintenance window](docs/superpowers/plans/2026-03-12-unified-maintenance-window.md)
- [x] [2026-03-14 Diagnostics export](docs/superpowers/plans/2026-03-14-diagnostics-export.md)
- [x] [2026-03-18 Files & History redesign](docs/superpowers/plans/2026-03-18-files-history-redesign.md)
- [x] [2026-03-25 Unified activity log](docs/superpowers/plans/2026-03-25-unified-activity-log.md)
- [x] [2026-03-25 Unified activity page](docs/superpowers/plans/2026-03-25-unified-activity-page.md)
- [x] [2026-03-27 ITL parser rewrite](docs/superpowers/plans/2026-03-27-itl-parser-rewrite.md)
- [x] [2026-03-28 Book-files table](docs/superpowers/plans/2026-03-28-book-files-table.md)
- [x] [2026-04-05 mTLS bridge](docs/superpowers/plans/2026-04-05-mtls-bridge.md)
- [x] [2026-04-06 Bulk metadata review](docs/superpowers/plans/2026-04-06-bulk-metadata-review.md)
- [x] [2026-04-06 mTLS bridge repo extraction](docs/superpowers/plans/2026-04-06-mtls-bridge-repo-extraction.md)
- [x] [2026-04-09 Activity log compaction](docs/superpowers/plans/2026-04-09-activity-log-compaction.md)
- [x] [2026-04-09 Embedding dedup](docs/superpowers/plans/2026-04-09-embedding-dedup.md)
- [x] [2026-04-10 Metadata candidate scoring PR1](docs/superpowers/plans/2026-04-10-metadata-candidate-scoring-pr1.md)
- [x] [2026-04-10 Metadata candidate scoring PR2](docs/superpowers/plans/2026-04-10-metadata-candidate-scoring-pr2.md)
- ⏳ [2026-04-15 Library centralization](docs/superpowers/plans/2026-04-15-library-centralization.md) — tasks 1-9 done (deluge integration deferred)
- ⏳ [2026-04-15 Bulk organize undo](docs/superpowers/plans/2026-04-15-bulk-organize-undo.md) — tasks 1-6 done (torrent move_storage deferred)
- [x] [2026-04-15 Smart + static playlists](docs/superpowers/plans/2026-04-15-smart-and-static-playlists.md) — complete (9/9 tasks)
- [x] [2026-04-15 Read/unread tracking](docs/superpowers/plans/2026-04-15-read-unread-tracking.md) — complete (8/8 tasks)
- [x] [2026-04-15 Multi-user support](docs/superpowers/plans/2026-04-15-multi-user-support.md) — complete (8/8, OAuth deferred)
- ⏳ [2026-04-15 Bleve library search (DES-1)](docs/superpowers/plans/2026-04-15-bleve-library-search.md) — tasks 1-6 done (skeleton through frontend)
- [x] [2026-04-15 DI migration (4.4)](docs/superpowers/plans/2026-04-15-di-migration.md) — complete

---

## 📐 Design Specs — [`docs/superpowers/specs/`](docs/superpowers/specs/)

- [2026-03-10 Central logger](docs/superpowers/specs/2026-03-10-central-logger-design.md)
- [2026-03-10 Incremental scan](docs/superpowers/specs/2026-03-10-incremental-scan-design.md)
- [2026-03-12 Unified maintenance window](docs/superpowers/specs/2026-03-12-unified-maintenance-window-design.md)
- [2026-03-14 Deferred iTunes updates](docs/superpowers/specs/2026-03-14-deferred-itunes-updates-design.md)
- [2026-03-14 Diagnostics export](docs/superpowers/specs/2026-03-14-diagnostics-export-design.md)
- [2026-03-15 External ID mapping](docs/superpowers/specs/2026-03-15-external-id-mapping-design.md)
- [2026-03-18 Files & History redesign](docs/superpowers/specs/2026-03-18-files-history-redesign.md)
- [2026-03-25 Unified activity log](docs/superpowers/specs/2026-03-25-unified-activity-log-design.md)
- [2026-03-25 Unified activity page](docs/superpowers/specs/2026-03-25-unified-activity-page-design.md)
- [2026-03-25 Unified change tracking](docs/superpowers/specs/2026-03-25-unified-change-tracking-design.md)
- [2026-03-27 ITL parser rewrite](docs/superpowers/specs/2026-03-27-itl-parser-rewrite-design.md)
- [2026-03-28 Book-files table](docs/superpowers/specs/2026-03-28-book-files-table-design.md)
- [2026-04-05 mTLS bridge](docs/superpowers/specs/2026-04-05-mtls-bridge-design.md)
- [2026-04-06 Bulk metadata review](docs/superpowers/specs/2026-04-06-bulk-metadata-review-design.md)
- [2026-04-06 mTLS bridge repo extraction](docs/superpowers/specs/2026-04-06-mtls-bridge-repo-extraction-design.md)
- [2026-04-09 Activity log compaction](docs/superpowers/specs/2026-04-09-activity-log-compaction-design.md)
- [2026-04-09 Embedding dedup](docs/superpowers/specs/2026-04-09-embedding-dedup-design.md)
- [2026-04-10 Metadata candidate scoring](docs/superpowers/specs/2026-04-10-metadata-candidate-scoring-design.md)
- [2026-04-11 Bleve library search](docs/superpowers/specs/2026-04-11-bleve-library-search.md) — design only, no plan yet
- [2026-04-11 chromem-go embedding store](docs/superpowers/specs/2026-04-11-chromem-go-embedding-store.md) — design only, no plan yet

---

## ✅ Recently Completed

### Sessions 21-22 (2026-04-16) — feature foundations + v0.209.0/v0.210.0

**60 PRs merged (#280-#340)** across two sessions + 3 releases (v0.209.0, v0.210.0, v0.211.0):

- **4.4 DI migration** — complete (#280-#291): replaced `database.GlobalStore` with constructor injection
- **3.7 Multi-user auth** — tasks 1-4, 6 (#292-#295, #299, #313-#314): schema, permissions, middleware, lockout, 247-route permission wiring
- **3.1 Library centralization** — tasks 1-4 (#296-#297, #306, #315-#316): BookVersion schema, `.versions/` fs ops, primary swap, fingerprint check
- **3.6 Read/unread tracking** — tasks 1-4 (#300, #303, #317): position/state schema, recomputation engine, HTTP endpoints, iTunes Bookmark sync
- **DES-1 Bleve search** — tasks 1-5 (#298, #301-#302, #311-#312): index, parser, translator, indexedStore decorator, endpoint routing
- **3.4 Playlists** — tasks 1-3 (#307-#309): UserPlaylist schema, smart evaluator, 9 HTTP endpoints
- **3.2 Undo** — tasks 3, 5 (#318-#319): undo engine, pre-flight conflict detection
- **Bug fixes**: Pebble prefix iteration slice aliasing (#318), go.mod tidy for release (#310)
- **Releases**: v0.209.0, v0.210.0 published

### Session 20 (2026-04-14) — operations infrastructure + UX cleanup

- **#270** Per-op file I/O tracking + resumable bulk ops (GFO-1, GFO-2, GFO-3 partial, GFO-5)
- **#271** Normalize "Series, Book N" out of Audible candidates (BMR-1)
- **#272** Make `reconcile_scan` resumable (GFO-3 final)
- **#273** Richer organize error messages with paths and remediation hints (5.4)
- **#274** `seed` subcommand for local dev libraries (5.5)

### v0.206.0 release (2026-04-13)

See [v0.206.0 release notes](https://github.com/jdfalk/audiobook-organizer/releases/tag/v0.206.0) for the full commit list. Highlights folded into §1, §3, §5, §7 above.

<details>
<summary>Session 12-19 archive — click to expand</summary>

### Bugs — Session 15 (March 25-27, 2026) — all fixed
- **B1** Author merge variant display — shows merge target + all variant names
- **B2** Tag extraction conflicting metadata — composer cleared on write
- **B3** Author/narrator swap — mitigated by B2; full fix needs metadata pipeline redesign (7.11 covered the worst of it)
- **B4** `series_index` not read back — already fixed (reads `SERIES_INDEX` / `MVIN`)
- **B5** 35 iTunes sync path errors — not a bug, files genuinely missing on disk
- **B6** File version separator too faint — thicker separator
- **B7** Book detail refresh after metadata — refresh button + auto-refresh after operations
- **B8** Write-back fails on multi-file books — globs audio files in directory

### P0 / P1 — all resolved
- **1** ISBN enrichment wrong matches — 60% length ratio fix validated
- **2** Preview Organize (single book) — built with step-by-step preview + Apply
- **3** Playlist system — assessed, needs brainstorming (tracked as 3.4 above)
- **4** Bulk "Save to Files" — `POST /api/v1/audiobooks/bulk-write-back`
- **5** Series dedup cleanup — `POST /api/v1/maintenance/cleanup-series`
- **6** "read by narrator" fix — `POST /api/v1/maintenance/fix-read-by-narrator` (dry-run default)
- **7** M4B conversion live test — local tests pass; production test user-gated

### P2 items 8-29 (April 6, 2026 session) — all fixed
Activity page mobile layout, adaptive refresh, version vs snapshot UI polish, compare snapshot wiring, background ISBN enrichment, copy-on-write TTL tuning, iTunes PID detail view, ITL write-back testing, TAG-DIAG cleanup, author/narrator swap full fix, library state badges, Vite chunk splitting, stale interrupted operations, sticky settings buttons, iTunes sync dialog pre-fill, iTunes sync from ITL directly, Force Import greyed out, ITL multi-file books, Files & History separate version boxes, show individual files, track PIDs sorted, XML function deprecation.

### Active P1 items 30-33 (April 6, 2026) — resolved or partial
- **30** Background file ops graceful tracking — persistent PebbleDB tracking + startup recovery. Five follow-up gaps captured under **GFO-1..5** above.
- **31** Resume interrupted metadata fetch on startup — saves book_ids as params, resumes remaining
- **32** Aggressive search/book result caching — list 30s, metadata search 30s
- **33** Batch apply separate requests per click — partially fixed (500ms debounce); true client-side queue still open

### CI/CD & Lint Fixes (April 6, 2026)
- **34** E2E test lint errors — 15 fixes across 12 files
- **35** Frontend lint warnings — proper types, targeted eslint-disable
- **36** GitHub Actions Node.js 20 deprecation — `setup-node` already at v6.3.0; transitive updates ongoing

### Data Cleanup (Session 12-13)
- Library: 68K → 10.9K books (84% reduction)
- Authors: 6K → 2.9K; series: 19K → 8.5K
- 15K same-path duplicates, 5K same-format duplicates, 2.9K unmatched organizer copies deleted
- 1.3K duplicate series merged, 7.3K empty series removed
- 2.3K empty authors removed
- 278 numeric title prefixes stripped
- 332 fake numeric series assignments removed
- All ULID version groups converted to `vg-` style
- All version groups have a primary version set

### Features — Session 12-13
- Diagnostics page (ZIP export, AI batch analysis, 4 categories, results review)
- External ID mapping (migration 34, 97K PID mappings, merge/delete/tombstone)
- Files & History tab (format-grouped trays, TagComparison, ChangeLog timeline)
- Background ISBN/ASIN enrichment after metadata apply
- Bulk batch-operations API (per-item update/delete/restore)
- Universal batch poller (routes by metadata tag)
- Deferred iTunes updates (migration 33, post-transcode hook)
- File path history (migration 35)
- Genre field (migration 36)
- Copy-on-write backups with TTL cleanup
- Revert buttons in ChangeLog (DB + file revert)

</details>
