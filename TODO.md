<!-- file: TODO.md -->
<!-- version: 5.29.0 -->
<!-- guid: 8e7d5d79-394f-4c91-9c7c-fc4a3a4e84d2 -->
<!-- last-edited: 2026-04-27 -->

# Project TODO

Canonical index into every piece of outstanding work across the project.
Details live in the linked files; this file exists so anyone (you, me, a
future agent, the nightly burndown bot) can scan the entire workspace in
one page.

**Sources indexed here:**

- [`docs/backlog-2026-04-10.md`](docs/backlog-2026-04-10.md) — 1725-line working list, ranked by category (research source; uses `### headers` so the burndown bot does not pick from it directly)
- [`docs/superpowers/specs/`](docs/superpowers/specs/) — **human-readable design docs** (the `why`, tradeoffs, alternatives)
- [`docs/superpowers/bot-tasks/`](docs/superpowers/bot-tasks/) — **burndown-bot recipes** (mechanical contracts: file paths, exact diffs, definition-of-done, STOP conditions)
- [`docs/superpowers/plans/`](docs/superpowers/plans/) — implementation plans per feature
- [`docs/implementation-guide.md`](docs/implementation-guide.md) — integration guide for open items
- Claude project memory at `~/.claude/projects/-Users-jdfalk-repos-github-com-jdfalk-audiobook-organizer/memory/` — items still to graduate here

> **Two-spec convention** (April 27, 2026 onward): every burndown-queue task has TWO docs — one `-design.md` for the human reviewer (rationale, tradeoffs, scope) and one in `bot-tasks/` for the bot (literal recipe with stop conditions). The TODO line below each task labels which is which.
>
> **Checkbox legend** — the burndown bot's parser only picks up `- [ ]` lines, so we use three states:
> - `- [ ]` — open and actionable. **The burndown bot will queue this.** `[auto-ok]` after the checkbox flags it for auto-merge on green CI.
> - `- [x]` — done.
> - `- [~]` — partially complete (sub-tasks broken out elsewhere, usually in the burndown queue).
> - `- [-]` — intentionally not in the burndown queue. Reasons vary (research only, needs human-authored spec, deferred decision, operator action). **The bot does not see these** because `[-]` matches neither regex. Don't change `[-]` items to `[ ]` without first writing the spec/bot-task pair.

---

## 🎯 Current Status — April 27, 2026

**Library:** 10,891 books / 2,970 authors / 8,507 series (cleaned)
**Production:** PebbleDB, Linux, HTTPS at `172.16.2.30:8484`, mTLS bridge active
**Latest shipped release:** v0.213.0 (post-2026-04-16)
**In flight:** Bot-driven burndown queue prepared (see §🤖 below); 13 mechanical tasks ready for nightly dispatch.

---

## ✅ Recently Shipped — April 26–27, 2026

Consolidated rationale: [`docs/superpowers/specs/2026-04-27-recent-ships-backfill.md`](docs/superpowers/specs/2026-04-27-recent-ships-backfill.md).

- [x] **iTunes path repair operation** (PRs #467–#471) — three-tier resolver (PID→DB / tag-scan / fuzzy-rank), dry-run by default, `?apply=true` updates `BookFile.FilePath`/`ITunesPath` and queues writeback. Production dry-run resolved 7,637/8,066 stale paths (94.7%); 429 in tier-C review. Counted as TODO **7.9 Phase 1**.
- [x] **Metadata review pagination + apply-refresh fix** (PRs #466, #473, #481) — server-side pagination removes the 5,000-`getBook()` waterfall; apply no longer refetches the library mid-review.
- [x] **Config persistence JSON round-trip** (PR #472) — every `json`-tagged `Config` field now persists with zero registration glue.
- [x] **Activity batcher (15s / 200-item)** (PRs #477–#481) — `LogBatch` / `FlushOperation` structured API; cuts ~5,000 logs/scan to folder summaries; expandable UI.
- [x] **Cache tuning sweep** (PRs #461–#465) — TTLs 24h, list-cache invalidation opt-in, metadata-fetch cache preserved across apply, indexedStore unwrap fix.
- [x] **System Maintenance tab** (PRs #474, #475, #476) — merged into existing System tab; window scheduling UI; `is_running` on `TaskInfo`.
- [x] **Browser RAM optimization** (commit `54146c96`) — lazy image loading + `content-visibility` CSS; steady-state for 10K books drops from 1.8 GB → 250 MB.

---

## 🛠️ Slash commands

- **`/parallel-sweep`** (project-scoped, [`.claude/commands/parallel-sweep.md`](.claude/commands/parallel-sweep.md)) — coordinated multi-task refactor sweep (≥3 mechanical tasks). Spec in [`docs/superpowers/specs/parallel-sweep.md`](docs/superpowers/specs/parallel-sweep.md).
- **`/ship`** (global, `~/.claude/commands/ship.md`) — push current branch, open PR, admin-merge with rebase/FF, pull the default branch, run `make deploy` if present. Standard ship-it pipeline; works in any repo.

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

## 📋 Backlog — [full file](docs/backlog-2026-04-10.md)

### 1. Dedup & Library Integrity — all closed

- [x] **1.1**–**1.10** — see backlog file.

### 2. Known Bugs — all closed in #227

- [x] **2.1**–**2.7** — see backlog file.

### 3. Features

- [~] **3.1** Library centralization / `.versions/` layout (**L**) — 9/10 tasks done (#296, #306, #315–#316, #324–#325, #337). Remaining: deluge `move_storage` integration → **3.1-deluge** (in burndown queue).
- [~] **3.2** Bulk organize undo via `operation_changes` (**M**) — 6/7 tasks done (#318–#319, #326, #332). Remaining: torrent `move_storage` rollback → **3.2-deluge** (in burndown queue).
- [x] **3.3** Bulk edit metadata across selected books (shipped v0.206.0)
- [x] **3.4** Smart playlists (**M**) — 9/9 (#307–#309, #338–#340)
- [x] **3.5** Cover art browse/restore UI (**S**) — #346
- [x] **3.6** Read/unread tracking (**M**) — 8/8 (#300, #303, #317, #331, #336)
- [x] **3.7** Multi-user support (**L**) — 8/8 (#292–#295, #313–#314, #322, #334)
- [-] **3.8** Plex-style HTTP media server API (**L**) — *needs human-authored spec; not in burndown queue.*
- [-] **3.9** LLM-based series detection and ordering (**M**) — *needs human-authored spec; not in burndown queue.*
- [-] **3.10** AI-generated cover art when none exists (**S**) — promoted from backlog file. *Needs human-authored spec; not in burndown queue.*

### 4. Architecture / Future-Proofing

- [-] **4.1** PostgreSQL research track (**XL**) — *research only, no actionable scope; not in burndown queue.*
- [x] **4.2** Split the monolithic `server.go` (commit `c858ceb`)
- [x] **4.3** Move write-back queue to a durable outbox (**M**) — #344
- [x] **4.4** Replace `database.GlobalStore` package var with DI (**L**) — complete (#280–#291)
- [x] **4.5** Property-based tests for dedup engine (expanded to full codebase) — complete (#357, #359, #361–#363, #365–#368)
- [x] **4.6** Chaos tests for the embedding store under shutdown (**M**) — 7 tests
- [-] **4.7** Per-workload store evaluation (**L** research) — *research only; not in burndown queue.*
- [x] **4.8** Split the `database.Store` interface (ISP refactor) (**L**) — complete (#372, #376, #379–#382, #387–#395)
- [x] **4.9** Eliminate remaining package globals (DI Phase 2) (**M**) — 10 globals replaced (#386)
- [x] **4.10** Service-layer unit tests with mock stores (**L**) — ~300 new tests
- [x] **4.11** Split `internal/server` into sub-packages (**XL**) — 7 extractions (#398)
- [x] **4.12** Extract iTunes integration into `internal/itunes` (**L**) — 3 PRs, Apr 18-20
- [~] **4.13** Comprehensive iTunes test suite (**L**) — current package coverage 55%, target 80%. Split into 5 burndown sub-tasks: **4.13a–e** (see queue below).
  - human spec: [`docs/superpowers/specs/2026-04-27-itunes-test-suite-design.md`](docs/superpowers/specs/2026-04-27-itunes-test-suite-design.md)
- [~] **4.14** Plugin system framework (**XL**) — V1 done. V2 (RPC subprocess plugins + webhook event delivery) deferred indefinitely; not in burndown queue.
- [x] **4.15** HTTP response envelope migration (**L**) — complete (PRs #425–#438)
- [x] **4.16** `/parallel-sweep` slash command (**L**) — complete, 9 steps
- [~] **4.17** Audit divergent fetch-path code (**M**) — split into 1 refactor + 3 audits: **4.17a–d** (see queue below).
  - human spec: [`docs/superpowers/specs/2026-04-27-fetch-path-audit-design.md`](docs/superpowers/specs/2026-04-27-fetch-path-audit-design.md)

### 5. UX / DX Polish — all closed

- [x] **5.1**–**5.10** — see backlog file.

### 6. Integration / Ecosystem

- [x] **6.1** Deluge `move_storage` integration (**M**) — #349 (transport layer); upper-layer wiring tracked as **3.1-deluge** / **3.2-deluge**
- [x] **6.2** Audnexus + Hardcover full integration (#7daef15)
- [x] **6.3** Tag writeback to iTunes via ITL updates (shipped previously)
- [~] **6.4** ITL upload / download / partial export (**M**) — tasks 1–3 + 5 done. Task 4 (partial export) blocked on **7.9** Phase 2.

### 7. Tagging as Infrastructure

- [-] **7.1** Tag-based policies / preference inheritance (**L**) — *needs human-authored spec; not in burndown queue.*
- [x] **7.2** Language filter in metadata review (shipped v0.206.0)
- [x] **7.3** Metadata-apply tagging — source + language (shipped v0.206.0)
- [x] **7.4** Google Books → Audible auto-upgrade maintenance job (shipped v0.206.0)
- [x] **7.5** Metadata fetch caching (shipped v0.206.0)
- [x] **7.6** Persistent review dialog + concurrent review during fetch (shipped v0.206.0)
- [x] **7.7** Author and series tag HTTP endpoints (**M**) — #347
- [x] **7.8** System tag UX — visual distinction user vs system (shipped v0.206.0)
- [~] **7.9** Full iTunes library regenerate / rebuild (**L**) — **Phase 1 done** (path repair, PRs #467–#471, see backfill spec). **Phase 2** (full rebuild from scratch when ITL is corrupt) remains open. *Phase 2 needs human-authored spec; not in burndown queue.*
- [x] **7.10** Archive sweep for soft-deleted books (**M**) — #342
- [x] **7.11** Author/series merge — sync denormalized `book.AuthorID` (shipped v0.206.0)
- [x] **7.12** Organize rewrites file tags on every run even when unchanged (shipped v0.206.0)

### 8. Out of Scope / Decide Later — [section](docs/backlog-2026-04-10.md#8-out-of-scope--decide-later)

iOS/Android app, WebDAV, RSS, notification system, federation, voice control, audio preview, recommendation engine.

---

## 🧠 From Memory — items not yet in the backlog file

### Graceful File Ops — all closed

- [x] **GFO-1** through **GFO-5** — see prior history in this file (Session 20 / 24).

### Series Name Normalization — shipped

- [x] **SNR-1** `StripSeriesContamination` pure function — strips dash-embedded title/position and trailing ordinal words from series names (`internal/metadata/series_normalize.go`)
- [x] **SNR-2** Ingest gates — `NormalizeMetaSeries`, `resolveSeriesID`, `ensureSeriesID` all call `StripSeriesContamination` before any store write
- [x] **SNR-3** `GET /api/v1/series/normalize/preview` — dry-run preview of rename/merge actions
- [x] **SNR-4** `POST /api/v1/series/normalize` — async remediation: rename → merge → write-back → organize
- [x] **SNR-5** `series_normalize` maintenance task registered in scheduler (manual-only)

### Bulk Metadata Review — Audible series format bug

- [x] **BMR-1** — `normalizeMetaSeries` now runs in `ApplyMetadataCandidate` (#271)

### Design Spec Already Written

- [~] **DES-1** Bleve library search — 6/7 tasks done. Remaining: **DES-1-T7** cleanup of legacy FTS5 + LIKE path (in burndown queue).
  - human spec: [`docs/superpowers/specs/2026-04-27-bleve-task7-cleanup-design.md`](docs/superpowers/specs/2026-04-27-bleve-task7-cleanup-design.md)
- [x] **DES-2** chromem-go embedding store — #351

### Per-feature LLM model knob (follow-up to aijobs batch migration)

- [-] **AI-MODEL-1** — Per-feature OpenAI model selection. *Tracked in burndown queue below — see §🤖 for the actionable entry.*
- [-] **AI-BATCH-1** — Migrate `OpenAIParser.ParseBatch` (openai_parser.go:271) to aijobs. *Deferred pending architecture decision; not in burndown queue.*
- [-] **AI-BATCH-2** — Migrate `OpenAIParser.ParseCoverArt` (openai_parser.go:364) to aijobs. *Deferred pending architecture decision; not in burndown queue.*

### Cache observability follow-ups (CHANGELOG 2026-04-25)

- [-] **CACHE-FOLLOWUP-1** — metadata-fetch TTL enforcement. *Tracked in burndown queue below.*
- [-] **CACHE-FOLLOWUP-2** — Tune per-cache `maxEntries` defaults from 30 days of stats history. *Needs telemetry data first; not in burndown queue until April 27 + 30 days.*
- [-] **CACHE-FOLLOWUP-3** — OTel migration (separate larger PR; tracing). *Needs human-authored spec; not in burndown queue.*

### Activity batcher follow-ups (April 27, 2026)

- [-] **ACT-BATCH-FU-1** — LogBatch context-cancel flush test. *Tracked in burndown queue below.*
- [-] **ACT-BATCH-FU-2** — Convert scanner per-file logs to LogBatch. *Tracked in burndown queue below.*
  - shared human spec: [`docs/superpowers/specs/2026-04-27-activity-batcher-followups-design.md`](docs/superpowers/specs/2026-04-27-activity-batcher-followups-design.md)

### iTunes path repair follow-ups

- [-] **ITUNES-PR-FU-1** — Production apply run is incomplete; run `01KQ6H1K96AMGDT12NEG7P49C3` (April 27) was initiated but not finished. **Operator action**: re-trigger with `?apply=true` once dry-run report is reviewed. Not a code change; tracked here so it isn't lost.
- [-] **ITUNES-PR-FU-2** — 429 candidates remain in tier-C `needs_review_items` from production dry-run `01KQ64DYPNC6BABHGV2575VKXQ`. **Operator action**: review and resolve. Not a code change.

### Frontend handler test coverage gap

- [-] **HANDLER-COV-1** — `internal/server` handler tests at 35.2%. *Needs split into per-handler test bot tasks once 4.13 lands and proves the test-by-file pattern works. Not in burndown queue yet.*

---

## 📚 Implementation Plans — [`docs/superpowers/plans/`](docs/superpowers/plans/)

✅ = implemented, ⏳ = design done, plan written, not yet executed.

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
- ⏳ [2026-04-15 Library centralization](docs/superpowers/plans/2026-04-15-library-centralization.md) — tasks 1–9 done; **3.1-deluge** in burndown queue
- ⏳ [2026-04-15 Bulk organize undo](docs/superpowers/plans/2026-04-15-bulk-organize-undo.md) — tasks 1–6 done; **3.2-deluge** in burndown queue
- [x] [2026-04-15 Smart + static playlists](docs/superpowers/plans/2026-04-15-smart-and-static-playlists.md)
- [x] [2026-04-15 Read/unread tracking](docs/superpowers/plans/2026-04-15-read-unread-tracking.md)
- [x] [2026-04-15 Multi-user support](docs/superpowers/plans/2026-04-15-multi-user-support.md)
- ⏳ [2026-04-15 Bleve library search (DES-1)](docs/superpowers/plans/2026-04-15-bleve-library-search.md) — tasks 1–6 done; **DES-1-T7** in burndown queue
- [x] [2026-04-15 DI migration (4.4)](docs/superpowers/plans/2026-04-15-di-migration.md)
- [x] [2026-04-23 Envelope migration (parallel) (4.15)](docs/superpowers/plans/2026-04-23-envelope-migration-parallel.md)
- [x] [2026-04-24 Migrate-loop](docs/superpowers/plans/2026-04-24-migrate-loop.md)
- [x] [2026-04-24 Parallel-sweep slash command](docs/superpowers/plans/2026-04-24-parallel-sweep-slash-command.md)
- [x] [2026-04-27 System Maintenance tab](docs/superpowers/plans/2026-04-27-system-maintenance-tab-plan.md)
- [Activity batcher (under docs/plans/)](docs/plans/activity-batcher-plan.md) — shipped April 27.

---

## 📐 Design Specs — [`docs/superpowers/specs/`](docs/superpowers/specs/)

(Older specs still present in the directory; newest April 27 additions called out.)

**April 27, 2026 additions** (this batch — burndown-queue prerequisites):

- [`2026-04-27-recent-ships-backfill.md`](docs/superpowers/specs/2026-04-27-recent-ships-backfill.md) — consolidated backfill for shipped-but-undocumented work
- [`2026-04-27-per-feature-llm-model-knob-design.md`](docs/superpowers/specs/2026-04-27-per-feature-llm-model-knob-design.md)
- [`2026-04-27-metadata-fetch-ttl-design.md`](docs/superpowers/specs/2026-04-27-metadata-fetch-ttl-design.md)
- [`2026-04-27-fetch-path-audit-design.md`](docs/superpowers/specs/2026-04-27-fetch-path-audit-design.md)
- [`2026-04-27-itunes-test-suite-design.md`](docs/superpowers/specs/2026-04-27-itunes-test-suite-design.md)
- [`2026-04-27-bleve-task7-cleanup-design.md`](docs/superpowers/specs/2026-04-27-bleve-task7-cleanup-design.md)
- [`2026-04-27-activity-batcher-followups-design.md`](docs/superpowers/specs/2026-04-27-activity-batcher-followups-design.md)
- [`2026-04-27-deluge-move-storage-integration-design.md`](docs/superpowers/specs/2026-04-27-deluge-move-storage-integration-design.md)
- [`2026-04-27-system-maintenance-tab-design.md`](docs/superpowers/specs/2026-04-27-system-maintenance-tab-design.md) — shipped, kept for reference

(Earlier specs unchanged — see directory listing.)

---

## 🤖 Burndown Queue

> The nightly burndown bot reads this section. **Every task here has a paired
> `human spec:` (design rationale) and `bot task:` (mechanical recipe).** The
> bot follows the bot-task doc; humans review against the human-spec doc.
>
> `[auto-ok]` = candidate for auto-merge on green CI. Without `[auto-ok]`, the
> bot opens a draft PR for human review.
>
> Tasks here are sized for the cheapest, slowest, dumbest model. Each one has
> a "When to STOP" section in its bot recipe — bots flag NEEDS_REVIEW rather
> than guessing.

### Auto-merge candidates

- [ ] [auto-ok] **AI-MODEL-1** — Per-feature LLM model knob in `internal/openai/openai_parser.go` and `internal/dedup/engine.go`
  bot task: docs/superpowers/bot-tasks/2026-04-27-per-feature-llm-model-knob.md
  human spec: docs/superpowers/specs/2026-04-27-per-feature-llm-model-knob-design.md

- [ ] [auto-ok] **CACHE-FOLLOWUP-1** — Metadata-fetch cache TTL enforcement on read (max-age config + `RecordCacheMiss(reason="expired")`)
  bot task: docs/superpowers/bot-tasks/2026-04-27-metadata-fetch-ttl.md
  human spec: docs/superpowers/specs/2026-04-27-metadata-fetch-ttl-design.md

- [ ] [auto-ok] **ACT-BATCH-FU-1** — Test that activity batcher flushes pending entries on context cancel (`-race` required)
  bot task: docs/superpowers/bot-tasks/2026-04-27-activity-batcher-flush-test.md
  human spec: docs/superpowers/specs/2026-04-27-activity-batcher-followups-design.md

- [ ] [auto-ok] **ACT-BATCH-FU-2** — Convert scanner per-file logs to `LogBatch` (first real consumer of the structured-batch API)
  bot task: docs/superpowers/bot-tasks/2026-04-27-activity-batcher-scanner-convert.md
  human spec: docs/superpowers/specs/2026-04-27-activity-batcher-followups-design.md

- [ ] [auto-ok] **DES-1-T7** — Remove legacy FTS5 + LIKE search path; migration drops `books_fts` virtual table
  bot task: docs/superpowers/bot-tasks/2026-04-27-bleve-task7-cleanup.md
  human spec: docs/superpowers/specs/2026-04-27-bleve-task7-cleanup-design.md

- [ ] [auto-ok] **4.13a** — Tests for `internal/itunes/service/status.go` (currently 0% coverage)
  bot task: docs/superpowers/bot-tasks/2026-04-27-itunes-tests-4-13a-status.md
  human spec: docs/superpowers/specs/2026-04-27-itunes-test-suite-design.md

- [ ] [auto-ok] **4.13b** — Tests for `internal/itunes/service/track_provisioner.go` (currently 0% coverage)
  bot task: docs/superpowers/bot-tasks/2026-04-27-itunes-tests-4-13b-track-provisioner.md
  human spec: docs/superpowers/specs/2026-04-27-itunes-test-suite-design.md

- [ ] [auto-ok] **4.13c** — Tests for `internal/itunes/service/validate.go` (currently 0% coverage; pure-function, table-driven)
  bot task: docs/superpowers/bot-tasks/2026-04-27-itunes-tests-4-13c-validate.md
  human spec: docs/superpowers/specs/2026-04-27-itunes-test-suite-design.md

- [ ] [auto-ok] **4.13d** — Error-path tests for `internal/itunes/service/importer.go` (raise file coverage 0.42 → 0.65)
  bot task: docs/superpowers/bot-tasks/2026-04-27-itunes-tests-4-13d-importer.md
  human spec: docs/superpowers/specs/2026-04-27-itunes-test-suite-design.md

- [ ] [auto-ok] **4.13e** — Tests for `service.go` + `transfer.go` lifecycle/disabled-mode/network-error (target package coverage ≥ 80%)
  bot task: docs/superpowers/bot-tasks/2026-04-27-itunes-tests-4-13e-service-transfer.md
  human spec: docs/superpowers/specs/2026-04-27-itunes-test-suite-design.md

- [ ] [auto-ok] **4.17a** — Refactor `bulkFetchMetadata` in `metadata_handlers.go` to delegate to `metafetch.Service.FetchMetadataForBookWithOpts`
  bot task: docs/superpowers/bot-tasks/2026-04-27-fetch-path-4-17a-bulk-fetch.md
  human spec: docs/superpowers/specs/2026-04-27-fetch-path-audit-design.md

- [ ] [auto-ok] **4.17b** — Audit-only docs PR: inventory `metadata_handlers.go` for service-delegation gaps (no code changes)
  bot task: docs/superpowers/bot-tasks/2026-04-27-fetch-path-4-17b-metadata-handlers.md
  human spec: docs/superpowers/specs/2026-04-27-fetch-path-audit-design.md

- [ ] [auto-ok] **4.17c** — Audit-only docs PR: inventory `dedup_handlers.go` for service-delegation gaps
  bot task: docs/superpowers/bot-tasks/2026-04-27-fetch-path-4-17c-dedup-handlers.md
  human spec: docs/superpowers/specs/2026-04-27-fetch-path-audit-design.md

- [ ] [auto-ok] **4.17d** — Audit-only docs PR: inventory audiobook handlers (`audiobooks_handlers.go`, `entities_handlers.go`) for service-delegation gaps
  bot task: docs/superpowers/bot-tasks/2026-04-27-fetch-path-4-17d-audiobook-handlers.md
  human spec: docs/superpowers/specs/2026-04-27-fetch-path-audit-design.md

- [ ] [auto-ok] **3.1-deluge** — Wire deluge `MoveStorage` into library centralization path (gated by `DelugeMoveEnabled`)
  bot task: docs/superpowers/bot-tasks/2026-04-27-deluge-centralization.md
  human spec: docs/superpowers/specs/2026-04-27-deluge-move-storage-integration-design.md

- [ ] [auto-ok] **3.2-deluge** — Wire deluge `MoveStorage` into bulk-organize undo path (depends on 3.1-deluge merging first)
  bot task: docs/superpowers/bot-tasks/2026-04-27-deluge-undo.md
  human spec: docs/superpowers/specs/2026-04-27-deluge-move-storage-integration-design.md

### Spec-first (bot opens a draft PR adding only the spec doc)

*(Empty — directive: all specs must be Claude-authored. The bot does not write specs.)*

### Deferred / Not in queue

These have a TODO entry above but no spec/recipe pair, so the burndown bot
will not touch them. Each requires either a Claude-authored spec or an
explicit unblock decision before it can enter the queue.

| ID | Reason |
|---|---|
| **3.8** Plex API | Needs human-authored spec; large surface |
| **3.9** LLM series detection | Needs human-authored spec |
| **3.10** AI cover art | Needs human-authored spec |
| **4.1** PostgreSQL | Research only |
| **4.7** Store evaluation | Research only |
| **4.14** Plugin V2 RPC | XL refactor; deferred |
| **6.4** Task 4 partial export | Blocked on 7.9 Phase 2 |
| **7.1** Tag-based policies | Needs human-authored spec |
| **7.9 Phase 2** Full iTunes regenerate from scratch | Needs human-authored spec (Phase 1 done) |
| **AI-BATCH-1** Migrate ParseBatch | Deferred pending architecture decision |
| **AI-BATCH-2** Migrate ParseCoverArt | Deferred pending architecture decision |
| **CACHE-FOLLOWUP-2** maxEntries tuning | Needs telemetry data (≥ 30 days) |
| **CACHE-FOLLOWUP-3** OTel migration | Needs human-authored spec |
| **HANDLER-COV-1** server handler test coverage | Needs split into per-handler tasks; revisit after 4.13 lands |
| **ITUNES-PR-FU-1/2** | Operator actions, not code changes |

---

## ✅ Recently Completed

### Session 25 (2026-04-26 → 2026-04-27) — backfill + burndown prep + April 26-27 ships

- **Backfill spec** for shipped-but-undocumented April 26-27 work
- **Burndown queue** prepared with 16 mechanical tasks, each with paired human-design + bot-recipe docs
- **Ships:** see "Recently Shipped" section above (iTunes path repair, metadata review pagination, config persistence, activity batcher, cache tuning, maintenance tab, browser RAM)

### Session 24 (2026-04-26) — iTunes path repair

- New operation `POST /operations/itunes-path-repair`. Three-tier resolver, 18 tests, dry-run by default. Folded into TODO **7.9** as Phase 1.

### Session 23 (2026-04-25) — cache observability

- Cache metrics on `/metrics`; new endpoints; LRU + lazy expired-on-Get; metrics sidecar DB. Follow-ups tracked as **CACHE-FOLLOWUP-1/2/3**.

### Sessions 21–22 (2026-04-16) — feature foundations + v0.209.0/v0.210.0

60 PRs (#280–#340) over two sessions + 3 releases. DI migration, multi-user, library centralization, read/unread, Bleve, playlists, undo. Bug fixes (Pebble prefix iteration, go.mod tidy). v0.209.0, v0.210.0, v0.211.0 published.

### Session 20 (2026-04-14) — operations infrastructure + UX cleanup

#270 (GFO-1/2/3-partial/5), #271 (BMR-1), #272 (GFO-3 final), #273 (5.4), #274 (5.5).

### v0.206.0 release (2026-04-13)

See [v0.206.0 release notes](https://github.com/jdfalk/audiobook-organizer/releases/tag/v0.206.0). Highlights folded into §1, §3, §5, §7 above.

<details>
<summary>Session 12-19 archive — click to expand</summary>

### Bugs — Session 15 (March 25-27, 2026) — all fixed
- **B1** Author merge variant display
- **B2** Tag extraction conflicting metadata
- **B3** Author/narrator swap
- **B4** `series_index` not read back
- **B5** 35 iTunes sync path errors (not a bug)
- **B6** File version separator too faint
- **B7** Book detail refresh after metadata
- **B8** Write-back fails on multi-file books

### P0 / P1 — all resolved
- ISBN enrichment, Preview Organize, Playlist system, Bulk Save to Files, Series dedup, narrator fix, M4B conversion

### P2 items 8-29 (April 6, 2026) — all fixed
Activity page mobile layout, adaptive refresh, version vs snapshot UI, snapshot compare wiring, background ISBN enrichment, COW TTL, iTunes PID detail, ITL write-back testing, TAG-DIAG cleanup, author/narrator swap, library state badges, Vite chunk splitting, stale ops, sticky settings, sync dialog, Force Import, ITL multi-file, Files & History boxes, individual files, track PID sort, XML deprecation.

### P1 items 30-33 (April 6, 2026) — resolved
- **30** Background file-ops graceful tracking → GFO-1..5
- **31** Resume metadata fetch on startup
- **32** Aggressive cache (list 30s, metadata 30s)
- **33** Batch apply debounce (500ms)

### CI/CD & Lint Fixes (April 6, 2026)
- E2E test lint, frontend lint warnings, GH Actions Node.js 20

### Data Cleanup (Sessions 12-13)
- 68K → 10.9K books, 6K → 2.9K authors, 19K → 8.5K series
- Same-path/format/format-orphan duplicates removed
- Series merges/empty cleanup, author cleanup
- Title prefix and fake series cleanup
- Version groups normalized

### Features — Sessions 12-13
- Diagnostics, External ID mapping, Files & History tab, ISBN enrichment, batch operations, batch poller, deferred iTunes updates, path history, genre field, COW backups, ChangeLog reverts.

</details>
