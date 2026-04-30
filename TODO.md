<!-- file: TODO.md -->
<!-- version: 6.5.0 -->
<!-- guid: 8e7d5d79-394f-4c91-9c7c-fc4a3a4e84d2 -->
<!-- last-edited: 2026-04-30 -->

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

## 🎯 Current Status — April 30, 2026

**Library:** 10,891 books / 2,970 authors / 8,507 series (cleaned)
**Production:** PebbleDB, Linux, HTTPS at `172.16.2.30:8484`, mTLS bridge active
**Latest shipped release:** v0.221.0 (2026-04-29) — PRs #507–#521; PRs #561–#563 merged 2026-04-30
**In flight:** User Ratings UI, ASYNC spec revision, iTunes relink unresolved cases (6,719 files)

---

## 🔧 CI / Release Infrastructure — Complete

- [x] Revert corrupted `release-go-action/action.yml`
- [x] `ghcommon/scripts/setup-ci-app.sh` — one-shot GitHub App creator + secret distributor
- [x] `ghcommon/reusable-release.yml` — stale draft + superseded-RC auto-cleanup on stable cuts
- [x] `ghcommon/reusable-release.yml` — keep-5 most-recent RCs policy (`RC_KEEP_COUNT`)
- [x] Create `jdfalk-ci-bot` GitHub App — done, secrets `CI_APP_ID` + `CI_APP_PRIVATE_KEY` present
- [x] Distribute secrets to repos — confirmed present on audiobook-organizer
- [x] Install App on target repos — working (releases use it for tag push)
- [x] `release-go-action/action.yml` — `github-token` input wired
- [x] `gha-release-go` — passes token through
- [x] `ghcommon/reusable-release.yml` — `create-github-app-token` wired
- [x] v0.207.0 through v0.213.0 all released successfully

---

## ⭐ User Ratings UI — DB + schema done, API + UI pending

PR #516 added full Audible rating dimensions (5 dims + count + reviews) and Google Books
(rating + count) to DB and metadata pipeline. PR #517 reserved `user_rating_overall`,
`user_rating_story`, `user_rating_performance`, `user_rating_notes` on `books` table.
PR #520 wires Audible `runtime_length_min` into candidate scoring. Still needed:

- [x] Audible ratings ingested (overall/story/performance/concept/delivery + count + reviews) — PR #516
- [x] Google Books ratings ingested (rating + count) — PR #516
- [x] User rating columns reserved on `books` table — PR #517
- [x] Duration scoring for candidates from Audible runtime — PR #520
- [x] **RATE-1** `PATCH /api/v1/audiobooks/:id/rating` accepts `{overall, story, performance, notes}` — PR #542
- [x] **RATE-2** Book detail UI: star rating widget (overall / story / performance + notes) — PR #552
- [x] **RATE-3** Audible/Google ratings shown on MetadataReviewDialog candidate cards — PR #553
- [x] **RATE-4** Library search/filter with numeric operators (>, <, >=, <=, ==, !=) for user_rating_* — PR #554
- [ ] **RATE-5** Bulk rating view / quick-rate from list

---

## ⏱️ Audible Runtime vs Book Duration Mismatch Detection

Audible returns `runtime_length_min` for every product. We now store `Duration`
on the `books` table (set during metadata apply). These two numbers should be
within ~10 minutes of each other — large gaps (> 10 min) suggest the wrong
Audible product was matched or the file is an abridged version.

- [x] WARN log + `duration_mismatch` flag on candidate result when delta > 600s — PR #549
- [x] `GET /api/v1/maintenance/scan-duration-mismatch` bulk scan endpoint — PR #549
- [x] **DUR-1** Surface in `MetadataReviewDialog`: show a yellow warning chip on the candidate row when `audible_runtime_min` and book `duration` differ by > 10 min, e.g. "⚠ runtime differs by 45 min" — chip implemented at `MetadataReviewDialog.tsx:604`
- [x] Book detail panel: show Audible runtime alongside local duration so mismatches are obvious — PR #561
- [x] Threshold configurable via query param `?max_delta_min=10` — PR #549

---

## 🔒 Deluge Protected Paths — Reflink Import Workflow

**Spec:** [`docs/superpowers/specs/2026-04-29-deluge-protected-paths-design.md`](docs/superpowers/specs/2026-04-29-deluge-protected-paths-design.md)

Core rule: never edit files outside `RootDir`. Deluge files are reflinked into the library before any tag write, then `core.move_storage` keeps Deluge seeding from the new location.

Implementation steps (in order):

- [x] **DELUGE-1** `deluge_hash`, `deluge_original_path`, `imported_from_deluge_at` columns on `book_files` — PR #540
- [x] **DELUGE-2** `protectedPathCache` with TTL refresh + IsProtected() — PR #556
- [ ] **DELUGE-3** `importToLibrary`: reflink `src → library_path`, update DB, call `core.move_storage` if enabled (best-effort) — bot-task: [`docs/superpowers/bot-tasks/2026-04-29-deluge-3-import-to-library.md`](docs/superpowers/bot-tasks/2026-04-29-deluge-3-import-to-library.md)
- [ ] **`WriteTagsSafe`**: pre-flight guard wrapping all tag-write call sites; falls back to `os.Copy` on non-reflink FS
- [ ] **Migrate all call sites** to `WriteTagsSafe` (bulk write-back, single-file write, cover embed)
- [x] **Discovery → Import UI**: "Import" button on discovered torrent calls the import flow — PR #562
- [x] **UI**: "Imported from Deluge" badge on book detail; original path shown in Files tab audit row — PR #561
- [x] **Config**: add `protected_paths []string` field; expose in Settings UI — PR #562

---

## 🔗 iTunes Relink — Unresolved Cases

PR #507 shipped the iTunes relink endpoint (3-tier path resolver: same-dir M4B → flat iTunes
search → disambiguation). It resolved **94.7%** of broken organizer-root books. Three groups
of cases remain:

**13 manually-identified unresolved books** — documented in [`docs/reports/unresolved-relinks-2026-04-28.md`](docs/reports/unresolved-relinks-2026-04-28.md). Root causes: co-author directory mismatch (organizer uses plain author, iTunes uses `Author, Co-Author`), title prefix collision after colon→underscore substitution, and zero-match disambiguation edge cases.

**~6,719 missing-file-repair unresolved** — books whose organizer-root paths cannot be found
anywhere (not in iTunes, not as flat M4B). Many are likely Deluge-only files not yet imported.

- [x] **RELINK-1** Apply 13 manual path fixes from the report — bot-task spec: [`docs/superpowers/bot-tasks/2026-04-29-relink-manual-fixes.md`](docs/superpowers/bot-tasks/2026-04-29-relink-manual-fixes.md) — 9 fixed via API, 4 absent from iTunes (results: `docs/reports/relink-manual-fixes-result-2026-04-29.md`)
- [x] **RELINK-2** Co-author dir matching: tries all dirs where author's surname appears — implemented at `maintenance_fixups.go:4154`
- [x] **RELINK-3** Title prefix colon→underscore normalization — implemented at `maintenance_fixups.go:4257`
- [x] **RELINK-4** `GET /api/v1/maintenance/relink-report` re-runs dry-run with why_unresolved annotations — PR #555
- [x] **RELINK-5** Bulk-import Deluge files into library for the ~6,719 that are Deluge-only — depends on Deluge Protected Paths (see below) — PR #563

---

## 📡 Activity Feed — Follow-up Gaps

PRs #509, #518, #521 wired batch logging + EmitInfo summaries + no-op tag filtering for
the four scheduler-driven maintenance ops. A few gaps remain:

- [x] **ACT-1** series-normalize EmitInfo (dedup-scan/author-dedup-scan already covered) — PR #547
- [x] **ACT-2** `info` tier in default-on tier filter — PR #539
- [x] **ACT-3** Batch noun for `isbn-enrich` — implemented at `batcher.go:211`

---

## 🏷️ Audible Category Ladders → Book Tags

Audible's `category_ladders` response group returns a full hierarchy per book,
e.g. `Audible Books > Science Fiction > Space Opera`. Each layer should be
applied as a user tag on the book so browsing by genre is hierarchical, not flat.

- [x] **CAT-1** category_ladders parsed into CategoryTags + AddBookTagWithSource("audible_category") in apply pipeline — PR #548
- [x] Parse ladder entries into `BookMetadata.CategoryTags []string` (all layers, e.g. `["Science Fiction", "Space Opera"]`) — PR #548
- [x] In the apply pipeline, write each tag via `AddBookTagWithSource` (idempotent) with source `"audible_category"` — PR #548
- [x] UI: show Audible-sourced category tags separately from user tags in the book detail panel — PR #561
- [ ] Search/filter: "has tag Science Fiction" or browsable tag cloud on library page

---

## 🤖 OpenAI Responses API Migration

Chat Completions is in maintenance; new models (gpt-5.4, codex-mini, the
o-series at full effort) ship on `/v1/responses` first or only. Plus
`PreviousResponseID` keeps history server-side, which collapses the
prompt-token cost for our multi-turn flows. Six phases sequenced
lowest-risk first; each phase ships independently and soaks before the
next picks up. Full plan in
[`docs/superpowers/specs/2026-04-29-responses-api-migration-design.md`](docs/superpowers/specs/2026-04-29-responses-api-migration-design.md).

- [ ] **AI-RESP-A** [hold] Migrate `metadata_llm_review.go` (single call) — design spec linked above
- [ ] **AI-RESP-B** [hold] Migrate `openai_parser.go` single-shot calls (6 sites) — depends on A clean
- [ ] **AI-RESP-C** [hold] **DO NOT MIGRATE EMBEDDINGS** — `/v1/embeddings` stays as-is. This entry is here only to make the bot aware not to touch `embedding_client.go`.
- [ ] **AI-RESP-D** [hold] Migrate Batches API (`openai_batch.go`) once OpenAI supports `/v1/responses` URLs in batch lines — verify endpoint allowlist before pickup
- [ ] **AI-RESP-E** [hold] Migrate `aijobs/aijobs.go` multi-turn flows — adds `last_response_id` to job state; biggest token win
- [ ] **AI-RESP-F** [hold] Cleanup: delete remaining Chat Completions call sites in `internal/ai/`

---

---

## 🩺 Diagnostics & Visibility

- [ ] **DIAG-1** Fix `ApiError: store does not implement AIJobsStore` on Diagnostics page — `AIJobsStore` interface (`iface_misc.go:255-265`) has no methods implemented in `sqlite_store.go` or `pebble_store.go`; crash occurs when `batch_poller` asserts `s.Store().(database.AIJobsStore)`
- [ ] **DIAG-2** Expand Diagnostics to surface DB health — SQLite table row counts, PebbleDB key counts, embeddings DB stats, `ai_scans.db` stats, recently-rejected metadata with reasons, `metadata_fetch` cache hit/miss/age
- [ ] **DIAG-3** Surface `ai_scans.db` and `embeddings.db` stats in Diagnostics — both are opened in `server.go:934-1004` but never shown on the diagnostics or system-info pages
- [ ] **DIAG-4** Increase `MetadataFetchCacheTTLDays` default — metadata_fetch cache TTL (configured via `config.AppConfig.MetadataFetchCacheTTLDays`) is expiring too fast; audit and increase default to 30+ days

---

## 🖥️ System Page Cleanup

- [ ] **SYS-1** Remove duplicate log viewer from System page — System page uses `/system/logs` (a different endpoint and data model from Activity); replace with a navigation link to the Activity page
- [ ] **SYS-2** Fix Storage page showing 0 books for `/mnt/bigdata/books/newbooks` — storage handler uses `file_path LIKE rootDir || '%'` prefix matching; investigate whether `newbooks` mount is configured as `RootDir` or import path

---

## 🔍 Data Quality & Matching Improvements

- [ ] **MATCH-1** Deduplicate books by metadata URL/response hash — when importing or applying metadata, compute a hash of the source URL or response body; if two books share the same hash they should be merged into one book with multiple versions (`BookVersion`)
- [ ] **MATCH-2** Consolidate multi-file chapter books by duration — files with sequential naming (`01 - Book`, `02 - Book`, etc.) that are individually very short (< 10 min each) should be pre-consolidated into a single book entry using cumulative duration rather than treated as separate books
- [ ] **MATCH-3** Use duration as metadata scoring signal — boost metadata candidates whose Audible `runtime_length_min` roughly matches local file total duration; combine with existing title/author/series scoring for much higher confidence matches

---

## 🔐 File Identity & SHA Tracking

- [ ] **FILE-SHA-1** Pre-metadata-write SHA capture — ensure `original_sha` / `OriginalFileHash` on `book_files` is recorded **before** any metadata tag write (scanner already computes it but confirm all write-back paths check); add `post_metadata_sha` field for after-write hash to detect transform drift
- [ ] **FILE-SHA-2** Cross-folder duplicate detection via SHA — use `original_sha` to identify identical files across different library paths (e.g. same file in iTunes + Deluge + organized); surface as consolidation candidates in the dedup UI

---

## 🗃️ Rejected Metadata Store

- [ ] **META-REJ-1** Rejected metadata tracking — add a store/table for metadata candidates that were explicitly skipped or rejected, recording: `book_file_id`, source (Audible/OpenLibrary), candidate title/author/ASIN, rejection reason, timestamp; surface on book detail page and diagnostics

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
- [x] **1.5** Side-by-side metadata diff in cluster card (**M**) — MetadataDiffTable component #348
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
- [x] **3.5** Cover art browse/restore UI (**S**) — #346
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
- [x] **4.5** Property-based tests for dedup engine (expanded to full codebase) (**M**) — complete (#357, #359, #361, #362, #363, #365, #366, #367, #368 — ~57 property tests across database / search / server / auth)
- [x] **4.6** Chaos tests for the embedding store under shutdown (**M**) — 7 tests: double-close, ops-after-close, concurrent write/read during close, mixed access, durability, WAL checkpoint
- [ ] **4.7** Per-workload store evaluation: Pebble vs SQLite vs PostgreSQL vs Go-native NoSQL (**L** research)
- [~] **4.8** Split the `database.Store` interface (ISP refactor) (**L**) — foundation + 3 proof-points shipped (#372, #376, #379, #380, #381, #382); ~38-file sweep + 18-file noop cleanup remain per [`docs/superpowers/plans/2026-04-17-store-iface-sweep.md`](docs/superpowers/plans/2026-04-17-store-iface-sweep.md)
- [x] **4.9** Eliminate remaining package globals (DI Phase 2) (**M**) — 10 globals replaced with interface injection + Server fields (#386)
- [ ] **4.10** Service-layer unit tests with mock stores (**L**) — leverage DI + ISP to unit-test AudiobookService, OrganizeService, MetadataFetchService, MergeService with MockStore; test error paths, edge cases, and business logic in isolation without HTTP or real DB
- [ ] **4.11** Split `internal/server` into sub-packages (**XL**) — extract standalone services (DedupEngine, MetadataFetchService, OrganizeService, etc.) into own packages; handlers stay in server; Server struct remains as wiring hub
- [ ] **4.12** Narrow extracted service dependencies to ISP sub-interfaces (**M**) — after 4.8 + 4.11, update extracted packages to accept narrow store interfaces (BookReader, etc.) instead of full database.Store
- [ ] **4.13** Extract iTunes integration into `internal/itunes` (**L**) — decouple iTunes import/sync/writeback from Server lifecycle; currently ~3,900 LOC deeply coupled to Server, needs interface extraction and dependency injection redesign

### 5. UX / DX Polish — [section](docs/backlog-2026-04-10.md#5-ux--dx-polish)

- [x] **5.1** Search inside the dedup tab (shipped v0.206.0, commit `191faa3`)
- [x] **5.2** "Similar books" lookup on BookDetail page (**S**) — #342
- [x] **5.3** Batch select in library view (**S**) — "Add to Playlist" batch action #345
- [x] **5.4** Better error messages on organize failures (#273)
- [x] **5.5** Dev mode "seed library" command (#274)
- [x] **5.6** Frontend test coverage baseline (**M**) — 22 test files / 160 tests: shared renderWithProviders + factories; component tests (SearchBar, ReadStatusChip, AddToPlaylistDialog, FilterSidebar); page tests (Playlists, Dashboard); CI: `make test-frontend`, `--run` flag, coverage thresholds
- [x] **5.7** API documentation (**M**) — OpenAPI 3.0.3 spec, 266 paths / 291 ops
- [x] **5.8** Regenerate ITL test fixtures after format work (**S**) — #348
- [x] **5.9** Enforce mockery-generated mocks via CI gate (commit `45492c3`)
- [x] **5.10** Fast-iteration backend test mode — `make test-short` + `testing.Short()` gates on 33 slow property tests (#384); `internal/server` drops from 760s → 63s

### 6. Integration / Ecosystem — [section](docs/backlog-2026-04-10.md#6-integration--ecosystem)

- [x] **6.1** Deluge `move_storage` integration (**M**) — #349
- [x] **6.2** Audnexus + Hardcover full integration (#7daef15)
- [x] **6.3** Tag writeback to iTunes via ITL updates (shipped previously)
- [ ] **6.4** ITL upload / download / partial export (**M**) — tasks 1-3 + 5 done (download, upload+validate, backup list+restore, frontend panel); task 4 (partial export) depends on 7.9

### 7. Tagging as Infrastructure — [section](docs/backlog-2026-04-10.md#7-tagging-as-infrastructure)

Underlying tag plumbing shipped in #244. Most items below are follow-ons
that layer on that foundation.

- [ ] **7.1** Tag-based policies / preference inheritance (**L**) — depends on 7.2
- [x] **7.2** Language filter in metadata review (shipped v0.206.0, commit `df6c9bd`)
- [x] **7.3** Metadata-apply tagging — source + language (shipped v0.206.0, commit `441fd43`)
- [x] **7.4** Google Books → Audible auto-upgrade maintenance job (shipped v0.206.0, commit `24201d4`)
- [x] **7.5** Metadata fetch caching (shipped v0.206.0, commit `2080c87`)
- [x] **7.6** Persistent review dialog + concurrent review during fetch (shipped v0.206.0, commit `1d2bf53`)
- [x] **7.7** Author and series tag HTTP endpoints (**M**) — #347; frontend UI remains
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
- [x] **GFO-4** Phase checkpoints in apply pipeline — rename/tags/itunes phases skip on recovery
- [x] **GFO-5** `GET /operations/recent` ~900ms — fixed by replacing O(N²) bubble sort with `sort.Slice` (#270). Side-index deferred until benchmarks show it's needed.

### Series Name Normalization — shipped

- [x] **SNR-1** `StripSeriesContamination` pure function — strips dash-embedded title/position and trailing ordinal words from series names (`internal/metadata/series_normalize.go`)
- [x] **SNR-2** Ingest gates — `NormalizeMetaSeries`, `resolveSeriesID`, `ensureSeriesID` all call `StripSeriesContamination` before any store write
- [x] **SNR-3** `GET /api/v1/series/normalize/preview` — dry-run preview of rename/merge actions
- [x] **SNR-4** `POST /api/v1/series/normalize` — async remediation: rename → merge → write-back → organize
- [x] **SNR-5** `series_normalize` maintenance task registered in scheduler (manual-only)

### Bulk Metadata Review — Audible series format bug

Full details: [`memory/project_bulk_metadata_review.md`](../../.claude/projects/-Users-jdfalk-repos-github-com-jdfalk-audiobook-organizer/memory/project_bulk_metadata_review.md)

- [x] **BMR-1** Audible "Series, Book N" baked into series field — `normalizeMetaSeries` now runs in `ApplyMetadataCandidate` too, not just the auto-fetch paths (#271)

### Async Operations — Unified Maintenance System

> 🛑 **BLOCKED ON SPEC REVISION — DO NOT BURNDOWN.** Opus review (2026-04-28) found
> BLOCKERs in CORE-2 (unverified `s.Store()` / `s.queue.Enqueue` / `EnqueueResume`
> signatures, body-bind into `json.RawMessage`, `default:` insertion assuming a
> switch), placeholder business logic in W1-3 / W1-4 / W2-4 / W3-2 (will land
> no-op PRs with green CI on destructive paths), `**` glob bug in W3-3, missing
> `itunes_path_trim_enabled` handling in W3-5, and CLEAN-1 gating that only
> checks PR labels (not registry presence). All bot-task entries below are
> intentionally left unchecked but **must not be picked up by the burndown bot
> until the spec is revised.** Tracked as ASYNC-REVISE.
>
> Design: [`docs/superpowers/specs/2026-04-28-unified-maintenance-system.md`](docs/superpowers/specs/2026-04-28-unified-maintenance-system.md)
> Dependency system: [`docs/superpowers/specs/2026-04-28-pr-label-dependencies.md`](docs/superpowers/specs/2026-04-28-pr-label-dependencies.md)
> Opus brief: [`docs/superpowers/specs/2026-04-28-opus-review-brief.md`](docs/superpowers/specs/2026-04-28-opus-review-brief.md)

- [x] **ASYNC-0** Frontend: toast notifications for operation lifecycle — PR #499
- [ ] [hold] **ASYNC-CORE-1** `MaintenanceJob` interface + registry package — [`bot-task`](docs/superpowers/bot-tasks/2026-04-28-async-core-1-interface.md)
- [ ] [hold] **ASYNC-CORE-2** Dispatcher `POST /maintenance/jobs/:id` + resume catch-all — [`bot-task`](docs/superpowers/bot-tasks/2026-04-28-async-core-2-dispatcher.md)
- [ ] [hold] **ASYNC-CORE-3** Frontend API client (`listMaintenanceJobs`, `runMaintenanceJob`) — [`bot-task`](docs/superpowers/bot-tasks/2026-04-28-async-core-3-discovery.md)
- [ ] [hold] **ASYNC-CORE-4** Dynamic "Manual Fixes" section in MaintenanceTab — [`bot-task`](docs/superpowers/bot-tasks/2026-04-28-async-core-4-frontend.md)
- [ ] [hold] **ASYNC-W1-1** Convert `fix-read-by-narrator` — [`bot-task`](docs/superpowers/bot-tasks/2026-04-28-async-w1-1-fix-read-by-narrator.md)
- [ ] [hold] **ASYNC-W1-2** Convert `cleanup-series` — [`bot-task`](docs/superpowers/bot-tasks/2026-04-28-async-w1-2-cleanup-series.md)
- [ ] [hold] **ASYNC-W1-3** Convert `fix-author-narrator-swap` — [`bot-task`](docs/superpowers/bot-tasks/2026-04-28-async-w1-3-fix-author-narrator-swap.md)
- [ ] [hold] **ASYNC-W1-4** Convert `fix-version-groups` — [`bot-task`](docs/superpowers/bot-tasks/2026-04-28-async-w1-4-fix-version-groups.md)
- [ ] [hold] **ASYNC-W2-1** Convert `backfill-book-files` — [`bot-task`](docs/superpowers/bot-tasks/2026-04-28-async-w2-1-backfill-book-files.md)
- [ ] [hold] **ASYNC-W2-2** Convert `cleanup-empty-folders` — [`bot-task`](docs/superpowers/bot-tasks/2026-04-28-async-w2-2-cleanup-empty-folders.md)
- [ ] [hold] **ASYNC-W2-3** Convert `cleanup-organize-mess` — [`bot-task`](docs/superpowers/bot-tasks/2026-04-28-async-w2-3-cleanup-organize-mess.md)
- [ ] [hold] **ASYNC-W2-4** Convert `fix-library-states` — [`bot-task`](docs/superpowers/bot-tasks/2026-04-28-async-w2-4-fix-library-states.md)
- [ ] [hold] **ASYNC-W3-1** Convert `enrich-book-files` — [`bot-task`](docs/superpowers/bot-tasks/2026-04-28-async-w3-1-enrich-book-files.md)
- [ ] [hold] **ASYNC-W3-2** Convert `dedup-books` — [`bot-task`](docs/superpowers/bot-tasks/2026-04-28-async-w3-2-dedup-books.md)
- [ ] [hold] **ASYNC-W3-3** Convert `fix-book-file-paths` — [`bot-task`](docs/superpowers/bot-tasks/2026-04-28-async-w3-3-fix-book-file-paths.md)
- [ ] [hold] **ASYNC-W3-4** Convert `refetch-missing-authors` — [`bot-task`](docs/superpowers/bot-tasks/2026-04-28-async-w3-4-refetch-missing-authors.md)
- [ ] [hold] **ASYNC-W3-5** Convert `recompute-itunes-paths` — [`bot-task`](docs/superpowers/bot-tasks/2026-04-28-async-w3-5-recompute-itunes-paths.md)
- [ ] [hold] **ASYNC-CLEAN-1** Remove 13 old synchronous routes (last, after all waves) — [`bot-task`](docs/superpowers/bot-tasks/2026-04-28-async-clean-1-remove-old-routes.md)

### Design Spec Already Written (but not yet planned)

- [x] **DES-1** Bleve library search — complete 6/7 (#298, #301-#302, #311-#312, #321)
- [x] **DES-2** chromem-go embedding store — #351 (store impl + tests; dedup engine wiring follows)

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
- [2026-04-28 Unified maintenance system](docs/superpowers/specs/2026-04-28-unified-maintenance-system.md) — MaintenanceJob interface + registry + dispatcher (ASYNC-CORE + W1-W3 + CLEAN-1; awaiting Opus review)
- [2026-04-28 PR label dependency system](docs/superpowers/specs/2026-04-28-pr-label-dependencies.md) — GitHub label-based prerequisite tracking for multi-wave burndown bot work
- [2026-04-29 iTunes relink manual fixes](docs/superpowers/bot-tasks/2026-04-29-relink-manual-fixes.md) — bot-task spec for applying 13 known manual path corrections (RELINK-1)

---

## ✅ Recently Completed

### Session 23 (2026-04-29) — metadata pipeline + activity feed + ratings (#507–#521)

**15 PRs merged** across one session:

- **#507** `feat(relink)`: iTunes relink endpoint — 3-tier path resolver (same-dir M4B → flat iTunes search → disambiguation), dir-grouping, 94.7% success on ~8K broken paths. 13 unresolved cases documented in `docs/reports/unresolved-relinks-2026-04-28.md`.
- **#508** `feat(metadata)`: async resumable bulk-fetch-metadata for full library
- **#509** `fix(activity)`: wire `LogBatch` into purge-deleted, isbn-enrichment, temp-file-cleanup, missing-file-repair; rename `missing_file_repair` → `missing-file-repair` (dash consistency)
- **#510** `fix(mocks)`: add missing `GetAllBookFiles` typed expecter to `MockStore` (unblocked `TestMockStore_Coverage`)
- **#511** `fix(maintenance)`: `revert-metadata-fetch` endpoint
- **#512** `fix(metadata)`: bulk-fetch-metadata no longer auto-applies
- **#513** `feat(metadata)`: `prefer_audible` and `skip_cached` options for bulk-fetch
- **#514** `fix(audible)`: json/v2 compat — `DiscardUnknownMembers` + nullable `RuntimeLengthMin`
- **#515** `feat(audible)`: map `runtime_length_min` → `DurationSec` → `Book.Duration`
- **#516** `feat(ratings)`: full Audible (5 dims + count + reviews) + Google Books (rating + count) rating dimensions ingested and stored
- **#517** `feat(db)`: reserve user rating columns (`user_rating_overall/story/performance/notes`) on `books` table
- **#518** `fix(activity)`: emit EmitInfo summary entries so maintenance ops show content in activity log (not just start/complete)
- **#519** `fix(ui)`: MetadataReviewDialog refresh, regex filter, correct pagination + Deluge timeout fix
- **#520** `feat(scoring)`: duration-based candidate ranking from Audible runtime
- **#521** `feat(activity)`: no-op tag filtering — `EmitInfo` variadic tags, `NoOpTag`/`TagsIf` helpers, `ExcludeTags` SQL + HTTP param, frontend "hide no-op" chip (default on)

Missing-file-repair scan results: **9,034 fixed**, 36 ambiguous, **6,719 unresolved** (see RELINK-5).
CI: disabled Docker in prerelease workflow (was exhausting 14GB GitHub runner disk).

---

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
