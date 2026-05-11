<!-- file: TODO.md -->
<!-- version: 8.17.0 -->
<!-- guid: 8e7d5d79-394f-4c91-9c7c-fc4a3a4e84d2 -->
<!-- last-edited: 2026-05-08 -->

# Project TODO

Canonical index into every piece of outstanding work across the project.
Details live in the linked files; this file exists so anyone (you, me, a
future agent) can scan the entire workspace in one page.

**Sources indexed here:**
- [`docs/backlog-2026-04-10.md`](docs/backlog-2026-04-10.md) — 1725-line working list, ranked by category
- [`docs/superpowers/plans/`](docs/superpowers/plans/) — implementation plans per feature
- [`docs/superpowers/specs/`](docs/superpowers/specs/) — design specs per feature
- [`docs/implementation-guide.md`](docs/implementation-guide.md) — integration guide for open items
- [`docs/codebase-evaluation.md`](docs/codebase-evaluation.md) — 2026-04-30 codebase audit (12 issue groups, 38 bot-tasks)
- Claude project memory at `~/.claude/projects/-Users-jdfalk-repos-github-com-jdfalk-audiobook-organizer/memory/` — items still to graduate here

---

## 🎯 Current Status — April 30, 2026

**Library:** 10,891 books / 2,970 authors / 8,507 series (cleaned)
**Production:** PebbleDB, Linux, HTTPS at `172.16.2.30:8484`, mTLS bridge active
**Latest shipped release:** v0.221.0 (2026-04-29) — PRs #507–#521; PRs #561–#563 merged 2026-04-30; PRs #570–#573 merged 2026-04-30
**In flight:** User Ratings UI, ASYNC spec revision, iTunes relink unresolved cases (6,719 files)

---

## AI Model Configuration

- [x] **AI-MODEL-1** Per-feature LLM model knob — adds `DedupReviewModel`, `MetadataReviewModel`, `FilenameParseModel`, `CoverArtModel` to `config.Config` (defaults `gpt-5-mini`). Replaces hardcoded literals in `openai_parser.go`, `openai_batch.go`, `metadata_llm_review.go`, and `dedup/engine.go` with config getters. PR feat/per-feature-llm-model.

---

## 🧹 Tech Debt Sweep — Deprecated Code & Warnings

- [ ] **TECHDEBT-1** Audit and remove deprecated code across the entire codebase
  - Backend: scan for `// Deprecated:` markers, dead code paths flagged in past evaluations, unused exported symbols, packages with replacement candidates already in use.
  - Frontend: resolve **React Router v6 future-flag warnings** (`v7_startTransition`, `v7_relativeSplatPath`) — opt in via `<BrowserRouter future={{...}}>` (and matching `MemoryRouter` usage in tests). Then upgrade-prep for v7 properly.
  - Frontend: audit `package.json` for deprecated transitive deps (`npm outdated`, `npm audit`), remove dead Material-UI v4-style imports if any remain, kill `console.log` left in src.
  - Go: `staticcheck`/`go vet` clean run; remove unused mocks; replace `ioutil.*` with `io`/`os`; collapse redundant context plumbing flagged in `docs/codebase-evaluation.md`.
  - SQL: drop schema columns/tables marked deprecated in migration history once readers/writers are gone.
  - Tests: replace `t.Skip` markers, remove `//nolint` that no longer apply, dedupe fixture builders.
  - Output: one PR per cluster (router warnings, backend deprecated APIs, frontend deps, dead code) so each can review/revert independently.

---

## 🔒 Security Alert Sweep — Audit 2026-05-03

**Complete inventory and remediation plan for all GitHub security alerts.**

**Audit Documents:**
- **Spec:** [`docs/security/audit-2026-05-03/spec.md`](docs/security/audit-2026-05-03/spec.md) — Alert inventory, severity breakdown, remediation recommendations
- **Implementation Plan:** [`docs/security/audit-2026-05-03/implementation-plan.md`](docs/security/audit-2026-05-03/implementation-plan.md) — Phased remediation plan (11 phases, 16 tasks, ~44 hours)
- **Raw Data:** [`docs/security/audit-2026-05-03/raw/`](docs/security/audit-2026-05-03/raw/) — JSON dumps from `gh api`

**Alert Totals (as of 2026-05-03):**
- **Code Scanning:** 602 total (235 open, 17 dismissed, 350 fixed)
- **Dependabot:** 20 total (1 open, 19 fixed)
- **Secret Scanning:** 0 alerts

**Open Alert Breakdown (236 total):**
- **231 Error/High:** 217 path injection, 6 clear-text logging, 4 SSRF, 2 allocation, 1 zipslip, 1 weak hashing
- **5 Warning/Medium:** 4 code scanning warnings, 1 Dependabot (follow-redirects)

### Phase -1: CodeQL Custom Sanitizer Pack (Noise Reduction)

- [ ] **SEC-AUDIT--1** Deploy CodeQL Models-as-Data pack for existing sanitizers
  - **Priority:** P0 (Unblocks Phase 1-6 by reducing false positives)
  - **Effort:** 2 hours
  - **Alerts:** Expected to reduce path injection from 217 → ~120-140 (~77 FP reduction)
  - **Files:** `.github/codeql/` (new pack), `.github/workflows/codeql.yml`, `docs/security/audit-2026-05-03/spec.md`
  - **Action:** Create MaD pack declaring `internal/util.SafeJoin` and `internal/util.WithinRoot` as path-injection sanitizers
  - **Dependencies:** None
  - **Status:** ✅ **IN PROGRESS** (PR pending)
  - **Details:** 
    - Pack declares `SafeJoin` return value as barrier for path-injection
    - Pack declares `WithinRoot` as barrier guard (conditional sanitizer)
    - Based on sast-sca-auditor spot-check: 35-45% of alerts are FPs from CodeQL not recognizing existing sanitizers
  - **Spec:** [`spec.md#remediation-strategy-phase-0`](docs/security/audit-2026-05-03/spec.md#remediation-strategy)

### Phase 0: Unblock Govulncheck

- [ ] **SEC-AUDIT-0** Enable govulncheck for `GOEXPERIMENT=jsonv2` builds
  - **Priority:** P0 (Blocker)
  - **Effort:** 1 hour
  - **Alerts:** N/A (unblocks Go vuln detection)
  - **Files:** `.github/workflows/vulnerability-scan.yml`
  - **Action:** Switch to binary-mode scanning (`govulncheck -mode=binary`)
  - **Dependencies:** None
  - **Spec:** [`spec.md#govulncheck-blocker`](docs/security/audit-2026-05-03/spec.md#govulncheck-blocker--goexperimentjsonv2)
  - **Plan:** [`implementation-plan.md#phase-0`](docs/security/audit-2026-05-03/implementation-plan.md#phase-0-enable-govulncheck-unblock-vulnerability-scanning)

### Phase 1-6: Path Injection (217 alerts)

- [ ] **SEC-AUDIT-1** Create `internal/security/pathvalidation` package (foundation)
  - **Priority:** P0
  - **Effort:** 4 hours
  - **Alerts:** Foundation for 217 path injection alerts
  - **Files:** `internal/security/pathvalidation/` (new)
  - **Action:** Build centralized path validation utilities (`ValidateRelativePath`, `SanitizeFilename`, `SecureJoin`)
  - **Dependencies:** Phase 0
  - **Plan:** [`implementation-plan.md#phase-1`](docs/security/audit-2026-05-03/implementation-plan.md#phase-1-path-injection--foundation-build-validation-utilities)

- [ ] **SEC-AUDIT-2** Fix path injection in fileops layer (9 alerts: #625-#620, #543, #542, #539, #538-#536)
  - **Priority:** P0
  - **Effort:** 6 hours
  - **Files:** `internal/fileops/` (service.go, hash.go, write_tags_safe.go, safe_operations.go)
  - **Dependencies:** Phase 1
  - **Plan:** [`implementation-plan.md#phase-2`](docs/security/audit-2026-05-03/implementation-plan.md#phase-2-path-injection--apply-validation-file-operations-core)

- [ ] **SEC-AUDIT-3** Fix path injection in cover handlers (9 alerts: #602-#594)
  - **Priority:** P0
  - **Effort:** 3 hours
  - **Files:** `internal/server/covers.go`, `internal/server/cover_history.go`
  - **Dependencies:** Phase 2
  - **Plan:** [`implementation-plan.md#phase-3`](docs/security/audit-2026-05-03/implementation-plan.md#phase-3-path-injection--server-handlers-covers)

- [ ] **SEC-AUDIT-4** Fix path injection in iTunes/transfer/audiobook handlers (20+ alerts: #627-#603, #619-#588)
  - **Priority:** P0
  - **Effort:** 6 hours
  - **Files:** `internal/server/itunes_handlers.go`, `internal/itunes/service/transfer.go`, `internal/server/audiobooks_handlers.go`, `internal/audiobooks/service.go`, `internal/server/server.go`
  - **Dependencies:** Phase 2
  - **Plan:** [`implementation-plan.md#phase-4`](docs/security/audit-2026-05-03/implementation-plan.md#phase-4-path-injection--itunestransferserver-core)

- [ ] **SEC-AUDIT-5** Fix path injection in scanner/reconcile/OpenLibrary (15+ alerts: #618-#608)
  - **Priority:** P0
  - **Effort:** 5 hours
  - **Files:** `internal/scanner/service.go`, `internal/reconcile/reconcile.go`, `internal/server/openlibrary_service.go`, `internal/importer/service.go`
  - **Dependencies:** Phase 2
  - **Plan:** [`implementation-plan.md#phase-5`](docs/security/audit-2026-05-03/implementation-plan.md#phase-5-path-injection--scannerreconcileopenlibrary)

- [ ] **SEC-AUDIT-6** Fix path injection in backup/Deluge/remaining (10+ alerts: #541, #535-#534, others)
  - **Priority:** P0
  - **Effort:** 3 hours
  - **Files:** `internal/backup/backup.go`, `internal/server/deluge_import_unix.go`
  - **Dependencies:** Phase 2
  - **Plan:** [`implementation-plan.md#phase-6`](docs/security/audit-2026-05-03/implementation-plan.md#phase-6-path-injection--backupdelugeremaining)

### Phase 7: Non-Path-Injection Errors (14 alerts)

- [ ] **SEC-AUDIT-7a** Fix clear-text logging (6 alerts: #530-#526, #47)
  - **Priority:** P1
  - **Effort:** 2 hours
  - **Files:** `internal/server/maintenance_fixups.go`, `cmd/root.go`
  - **Action:** Redact sensitive fields before logging
  - **Plan:** [`implementation-plan.md#task-71`](docs/security/audit-2026-05-03/implementation-plan.md#task-71-fix-clear-text-logging-6-alerts)

- [ ] **SEC-AUDIT-7b** Fix SSRF via URL validation (4 alerts: #587, #467, #458, #232)
  - **Priority:** P1
  - **Effort:** 4 hours
  - **Files:** `internal/server/covers.go`, `internal/deluge/client.go`, `internal/plugins/webhook/plugin.go`, `internal/metadata/cover.go`
  - **Action:** Whitelist allowed domains, block private IPs
  - **Plan:** [`implementation-plan.md#task-72`](docs/security/audit-2026-05-03/implementation-plan.md#task-72-fix-request-forgery-4-alerts)

- [ ] **SEC-AUDIT-7c** Fix uncontrolled allocation (2 alerts: #129, #44)
  - **Priority:** P2
  - **Effort:** 1 hour
  - **Files:** `internal/scanner/scanner.go`
  - **Action:** Cap allocation sizes
  - **Plan:** [`implementation-plan.md#task-73`](docs/security/audit-2026-05-03/implementation-plan.md#task-73-fix-uncontrolled-allocation-2-alerts)

- [ ] **SEC-AUDIT-7d** Fix zipslip in backup extraction (1 alert: #13)
  - **Priority:** P1
  - **Effort:** 1 hour
  - **Files:** `internal/backup/backup.go`
  - **Action:** Validate archive entry paths
  - **Plan:** [`implementation-plan.md#task-74`](docs/security/audit-2026-05-03/implementation-plan.md#task-74-fix-zipslip-1-alert)

- [ ] **SEC-AUDIT-7e** Fix weak sensitive data hashing (1 alert: #132)
  - **Priority:** P1
  - **Effort:** 2 hours
  - **Files:** `internal/database/settings.go`
  - **Action:** Upgrade to bcrypt/argon2 (passwords) or SHA-256 (non-password)
  - **Plan:** [`implementation-plan.md#task-75`](docs/security/audit-2026-05-03/implementation-plan.md#task-75-fix-weak-hashing-1-alert)

### Phase 8: Warnings (4 alerts)

- [ ] **SEC-AUDIT-8** Fix warning-level alerts (4 alerts: #379, #468, #160, #50)
  - **Priority:** P2-P3
  - **Effort:** 3.5 hours
  - **Alerts:** Disabled cert check (#379), allocation overflow (#468), JS cert bypass (#160), incomplete sanitization (#50)
  - **Files:** `internal/mtls/provisioning.go`, `internal/itunes/itl.go`, `scripts/record_demo.js`, `web/src/pages/Settings.tsx`
  - **Plan:** [`implementation-plan.md#phase-8`](docs/security/audit-2026-05-03/implementation-plan.md#phase-8-warnings-4-alerts)

### Phase 9: Dependabot

- [ ] **SEC-AUDIT-9** Bump follow-redirects to 1.16.0+ (1 alert: #27, GHSA-r4q5-vmmm-2653)
  - **Priority:** P2
  - **Effort:** 0.5 hours
  - **Files:** `web/package-lock.json`
  - **Action:** `npm update follow-redirects && npm audit fix`
  - **Plan:** [`implementation-plan.md#phase-9`](docs/security/audit-2026-05-03/implementation-plan.md#phase-9-dependabot-1-alert)

### Phase 10: Documentation

- [ ] **SEC-AUDIT-10** Document path validation policy & add dismissal comments
  - **Priority:** P3
  - **Effort:** 1.5 hours
  - **Action:** Create `docs/security/path-validation-policy.md`, add comments to 13 dismissed alerts (#560-#547)
  - **Plan:** [`implementation-plan.md#phase-10`](docs/security/audit-2026-05-03/implementation-plan.md#phase-10-documentation--dismissed-alerts)

### Phase 11: Verification

- [ ] **SEC-AUDIT-11** Final verification (re-pull alerts, confirm 0 open)
  - **Priority:** P0 (gate for completion)
  - **Effort:** 1 hour
  - **Action:** `gh api repos/.../code-scanning/alerts --paginate | jq '[.[] | select(.state == "open")] | length'`
  - **Plan:** [`implementation-plan.md#phase-11`](docs/security/audit-2026-05-03/implementation-plan.md#phase-11-final-verification)

**Estimated Total Effort:** 44 hours (~6-8 weeks part-time, 2-3 weeks full-time)

**Acceptance Criteria:**
- ✅ All 236 open alerts addressed (fixed or consciously dismissed with rationale)
- ✅ Govulncheck runs successfully on jsonv2 builds
- ✅ All PRs merged, `make ci` passes on main
- ✅ Post-remediation audit confirms 0 open alerts (or only accepted-risk)

---

## 📊 Codebase Evaluation — 2026-04-30

Full evaluation of the audiobook-organizer backend and frontend. 12 issue groups,
38 atomic bot-task PRs. Specs: `docs/superpowers/specs/2026-04-30-*.md`.
Bot-tasks: `docs/superpowers/bot-tasks/2026-04-30-*.md`.

### MOCK — Mock/CI Gate (2 tasks)

- [ ] **MOCK-1** `fix/regenerate-mocks` — Regenerate stale mockery mocks
  → [`2026-04-30-mock-1-regenerate.md`](docs/superpowers/bot-tasks/2026-04-30-mock-1-regenerate.md)
- [ ] **MOCK-2** `fix/mock-ci-gate` — Add `go generate` check to CI
  → [`2026-04-30-mock-2-ci-gate.md`](docs/superpowers/bot-tasks/2026-04-30-mock-2-ci-gate.md)

### N1 — N+1 Query Elimination (4 tasks)

- [ ] **N1-1** `perf/batch-fetch-interface` — Add batch-fetch methods to Store interface
  → [`2026-04-30-n1-1-batch-fetch-interface.md`](docs/superpowers/bot-tasks/2026-04-30-n1-1-batch-fetch-interface.md)
- [ ] **N1-2** `perf/n1-sqlite-impl` — Implement batch-fetch in SQLiteStore
  → [`2026-04-30-n1-2-sqlite-impl.md`](docs/superpowers/bot-tasks/2026-04-30-n1-2-sqlite-impl.md)
- [ ] **N1-3** `perf/n1-pebble-impl` — Implement batch-fetch in PebbleStore
  → [`2026-04-30-n1-3-pebble-impl.md`](docs/superpowers/bot-tasks/2026-04-30-n1-3-pebble-impl.md)
- [ ] **N1-4** `perf/n1-enrich-response` — Wire batch fetch into enrichBookForResponse
  → [`2026-04-30-n1-4-enrich-response.md`](docs/superpowers/bot-tasks/2026-04-30-n1-4-enrich-response.md)

### SEC — Filesystem / Security (4 tasks)

- [ ] **SEC-1** `fix/browse-dir-allowlist` — Restrict BrowseDirectory to configured import paths
  → [`2026-04-30-sec-1-browse-allowlist.md`](docs/superpowers/bot-tasks/2026-04-30-sec-1-browse-allowlist.md)
- [ ] **SEC-2** `fix/auth-enabled-default` — Startup warning when auth is disabled
  → [`2026-04-30-sec-2-auth-default.md`](docs/superpowers/bot-tasks/2026-04-30-sec-2-auth-default.md)
- [ ] **SEC-3** `fix/rate-limit-default` — Startup warning when rate limiting is disabled
  → [`2026-04-30-sec-3-rate-limit-default.md`](docs/superpowers/bot-tasks/2026-04-30-sec-3-rate-limit-default.md)
- [ ] **SEC-4** `fix/ratelimit-o1-cleanup` — Remove duplicate o1 rate-limit middleware
  → [`2026-04-30-sec-4-ratelimit-cleanup.md`](docs/superpowers/bot-tasks/2026-04-30-sec-4-ratelimit-cleanup.md)

### DB — Database Hygiene (6 tasks)

- [ ] **DB-1** `fix/db-file-hash-index` — Add unique index on (file_hash, library_id)
  → [`2026-04-30-db-1-file-hash-index.md`](docs/superpowers/bot-tasks/2026-04-30-db-1-file-hash-index.md)
- [ ] **DB-2** `fix/db-begin-tx-sqlite` — Wrap SaveBook pipeline in SQLite transaction
  → [`2026-04-30-db-2-begin-tx-sqlite.md`](docs/superpowers/bot-tasks/2026-04-30-db-2-begin-tx-sqlite.md)
- [ ] **DB-3** `fix/db-begin-tx-activity` — Wrap activity store writes in transactions
  → [`2026-04-30-db-3-begin-tx-activity.md`](docs/superpowers/bot-tasks/2026-04-30-db-3-begin-tx-activity.md)
- [ ] **DB-4** `fix/pipeline-save-errors` — Return errors from pipeline save steps
  → [`2026-04-30-db-4-pipeline-errors.md`](docs/superpowers/bot-tasks/2026-04-30-db-4-pipeline-errors.md)
- [ ] **DB-5** `fix/db-time-parse-errors` — Handle time.Parse errors in row scanners
  → [`2026-04-30-db-5-time-parse-errors.md`](docs/superpowers/bot-tasks/2026-04-30-db-5-time-parse-errors.md)
- [ ] **DB-6** `fix/pebble-silent-errors` — Surface silent errors in PebbleDB writes
  → [`2026-04-30-db-6-pebble-silent-errors.md`](docs/superpowers/bot-tasks/2026-04-30-db-6-pebble-silent-errors.md)

### CTX — Context Propagation (3 tasks)

- [ ] **CTX-1** `fix/ctx-audiobook-update-service` — Thread context through AudiobookUpdateService
  → [`2026-04-30-ctx-1-audiobook-update.md`](docs/superpowers/bot-tasks/2026-04-30-ctx-1-audiobook-update.md)
- [ ] **CTX-2** `fix/ctx-openlibrary-service` — Thread context through OpenLibrary client
  → [`2026-04-30-ctx-2-openlibrary.md`](docs/superpowers/bot-tasks/2026-04-30-ctx-2-openlibrary.md)
- [ ] **CTX-3** `fix/ctx-filesystem-handlers` — Thread context through filesystem handlers
  → [`2026-04-30-ctx-3-filesystem-handlers.md`](docs/superpowers/bot-tasks/2026-04-30-ctx-3-filesystem-handlers.md)

### LOG — Structured Logging (4 tasks)

- [ ] **LOG-1** `fix/log-tagger-structured` — Replace fmt.Printf with structured logging in tagger
  → [`2026-04-30-log-1-tagger.md`](docs/superpowers/bot-tasks/2026-04-30-log-1-tagger.md)
- [ ] **LOG-2** `fix/log-fileops-structured` — Replace fmt.Printf in fileops
  → [`2026-04-30-log-2-fileops.md`](docs/superpowers/bot-tasks/2026-04-30-log-2-fileops.md)
- [ ] **LOG-3** `fix/log-backup-structured` — Replace fmt.Printf in backup
  → [`2026-04-30-log-3-backup.md`](docs/superpowers/bot-tasks/2026-04-30-log-3-backup.md)
- [ ] **LOG-4** `fix/scanner-remove-progressbar` — Remove terminal progress bar from scanner
  → [`2026-04-30-log-4-progressbar.md`](docs/superpowers/bot-tasks/2026-04-30-log-4-progressbar.md)

### PROJ — Query Projection (2 tasks)

- [ ] **PROJ-1** `perf/book-summary-columns` — Define BookSummary DB struct
  → [`2026-04-30-proj-1-summary-columns.md`](docs/superpowers/bot-tasks/2026-04-30-proj-1-summary-columns.md)
- [ ] **PROJ-2** `perf/book-list-summary-query` — Implement GetBookSummaries projected query
  → [`2026-04-30-proj-2-list-query.md`](docs/superpowers/bot-tasks/2026-04-30-proj-2-list-query.md)

### SCAN — Scanner Efficiency (1 task)

- [x] **SCAN-1** `perf/scanner-walkdir` — Replace filepath.Walk with filepath.WalkDir
  → [`2026-04-30-scan-1-walkdir.md`](docs/superpowers/bot-tasks/2026-04-30-scan-1-walkdir.md)

### SRV — Server Response Optimization (2 tasks)

- [ ] **SRV-1** `feat/server-gzip-compression` — Add gzip compression middleware
  → [`2026-04-30-srv-1-gzip.md`](docs/superpowers/bot-tasks/2026-04-30-srv-1-gzip.md)
- [ ] **SRV-2** `fix/sse-heartbeat` — Add SSE heartbeat to prevent proxy timeouts
  → [`2026-04-30-srv-2-sse-heartbeat.md`](docs/superpowers/bot-tasks/2026-04-30-srv-2-sse-heartbeat.md)

### FE — Frontend Cleanup (10 tasks)

- [ ] **FE-1** `refactor/library-filter-panel` — Extract FilterPanel from Library.tsx
  → [`2026-04-30-fe-1-filter-panel.md`](docs/superpowers/bot-tasks/2026-04-30-fe-1-filter-panel.md)
- [ ] **FE-2** `refactor/library-book-grid` — Extract BookGrid from Library.tsx
  → [`2026-04-30-fe-2-book-grid.md`](docs/superpowers/bot-tasks/2026-04-30-fe-2-book-grid.md)
- [ ] **FE-3** `refactor/library-batch-toolbar` — Extract BatchToolbar from Library.tsx
  → [`2026-04-30-fe-3-batch-toolbar.md`](docs/superpowers/bot-tasks/2026-04-30-fe-3-batch-toolbar.md)
- [ ] **FE-4** `refactor/settings-general-tab` — Extract GeneralSettingsTab from Settings.tsx
  → [`2026-04-30-fe-4-settings-general.md`](docs/superpowers/bot-tasks/2026-04-30-fe-4-settings-general.md)
- [ ] **FE-5** `refactor/settings-paths-tab` — Extract PathsSettingsTab from Settings.tsx
  → [`2026-04-30-fe-5-settings-paths.md`](docs/superpowers/bot-tasks/2026-04-30-fe-5-settings-paths.md)
- [ ] **FE-6** `refactor/settings-metadata-tab` — Extract MetadataSettingsTab from Settings.tsx
  → [`2026-04-30-fe-6-settings-metadata.md`](docs/superpowers/bot-tasks/2026-04-30-fe-6-settings-metadata.md)
- [ ] **FE-7** `fix/frontend-remove-console-logs` — Remove console.log from production code
  → [`2026-04-30-fe-7-console-log.md`](docs/superpowers/bot-tasks/2026-04-30-fe-7-console-log.md)
- [ ] **FE-8** `fix/frontend-error-boundaries` — Add error boundaries to page components
  → [`2026-04-30-fe-8-error-boundaries.md`](docs/superpowers/bot-tasks/2026-04-30-fe-8-error-boundaries.md)
- [ ] **FE-9** `fix/frontend-localstorage-keys` — Centralise localStorage keys as constants
  → [`2026-04-30-fe-9-localstorage-keys.md`](docs/superpowers/bot-tasks/2026-04-30-fe-9-localstorage-keys.md)
- [ ] **FE-10** `chore/frontend-coverage-thresholds` — Add Vitest coverage thresholds
  → [`2026-04-30-fe-10-coverage.md`](docs/superpowers/bot-tasks/2026-04-30-fe-10-coverage.md)

### STRUCT — Structural Refactors — 2026-05-01

Full audit at [`docs/audits/2026-05-01-structure-audit.md`](docs/audits/2026-05-01-structure-audit.md).
Bot-tasks at [`docs/superpowers/bot-tasks/2026-05-01-struct-*.md`](docs/superpowers/bot-tasks/).

- [x] **STRUCT-1** — Migrate all direct `c.JSON` calls to `httputil.RespondWith*` helpers
  → [`2026-05-01-struct-1-server-response-helpers.md`](docs/superpowers/bot-tasks/2026-05-01-struct-1-server-response-helpers.md)
  ✅ `internal/httputil/` created; 0 raw `c.JSON` calls remain outside test files
- [x] **STRUCT-2** — Consolidate duplicate pagination parsers into `httputil.ParsePaginationParams`
  → [`2026-05-01-struct-2-pagination-helper.md`](docs/superpowers/bot-tasks/2026-05-01-struct-2-pagination-helper.md)
  ✅ `internal/httputil/parse.go` exports `ParsePaginationParams`; `server/pagination.go` deleted
- [x] **STRUCT-3** — Reduce 6400-line `maintenance_fixups.go`
  → [`2026-05-01-struct-3-maintenance-fixups-split.md`](docs/superpowers/bot-tasks/2026-05-01-struct-3-maintenance-fixups-split.md)
  ✅ ASYNC-CLEAN-1 removed old sync maintenance handlers; file reduced 6400→581 lines; 8-domain split no longer necessary
- [x] **STRUCT-4** — Split 3932-line `metafetch/service.go` into domain files
  → [`2026-05-01-struct-4-metafetch-service-split.md`](docs/superpowers/bot-tasks/2026-05-01-struct-4-metafetch-service-split.md)
  ✅ Split into 11 files: `service_writeback.go`, `service_apply.go`, `service_scoring.go`, `service_search.go`, `service_fetch.go`, `service_normalize.go`, `service_files.go`, `helpers.go`, `isbn.go`, `file_pipeline.go`, `path_format.go`
- [x] **STRUCT-5** — Extract shared `withRetry` helper from 4 identical AI retry loops
  → [`2026-05-01-struct-5-ai-retry-helper.md`](docs/superpowers/bot-tasks/2026-05-01-struct-5-ai-retry-helper.md)
  ✅ `internal/ai/retry.go` created; wired into 5 AI callers
- [x] **STRUCT-6** — Split 6976-line `sqlite_store.go` into 7 domain files
  → [`2026-05-01-struct-6-sqlite-store-split.md`](docs/superpowers/bot-tasks/2026-05-01-struct-6-sqlite-store-split.md)
  ✅ `sqlite_store.go` deleted; 7 domain files created under `internal/database/`
- [x] **STRUCT-7** — Split 3401-line `server.go` into 6 responsibility files
  → [`2026-05-01-struct-7-server-go-split.md`](docs/superpowers/bot-tasks/2026-05-01-struct-7-server-go-split.md)
  ✅ `server.go` reduced to 853 lines; 6 split files created
- [x] **STRUCT-8** — Extract `useAsyncAction` hook from 148 `setLoading` patterns
  → [`2026-05-01-struct-8-use-async-action-hook.md`](docs/superpowers/bot-tasks/2026-05-01-struct-8-use-async-action-hook.md)
  ✅ `web/src/hooks/useAsyncAction.ts` created and wired
- [x] **STRUCT-9** — Split oversized frontend page components into sub-components *(completed)*
  → [`2026-05-01-struct-9-frontend-component-splits.md`](docs/superpowers/bot-tasks/2026-05-01-struct-9-frontend-component-splits.md)
  ✅ `Library.tsx` reduced 3243 → 1916 lines (LibraryToolbar, LibraryBookGrid, LibraryDialogs extracted)
  ✅ `BookDedup.tsx` reduced 3424 → 1656 lines (DedupAdvancedScanTab, DedupAuthorTab, DedupSeriesTab, DedupReconcileTab extracted)
  ✅ `BookDetail.tsx` reduced 2773 → 1073 lines (BookDetailHeader, BookDetailActions, BookDetailInfoTab, BookDetailFilesTab, BookDetailDialogs, BookDetailVersionGroup, BookDetailStatusAlerts extracted)
- [x] **STRUCT-10** — Narrow `*Server` receivers with small local interfaces in handler groups *(completed)*
  → [`2026-05-01-struct-10-narrow-server-interfaces.md`](docs/superpowers/bot-tasks/2026-05-01-struct-10-narrow-server-interfaces.md)
  ✅ `internal/server/interfaces.go` with 4 narrow store interfaces + compile-time assertions
  ✅ Handler receivers narrowed in organize_handlers.go, ai_jobs_handlers.go, filesystem_handlers.go, reading_handlers.go, activity_handlers.go

#### STRUCT — Open gaps from audit (no task yet)

- [x] **STRUCT-11** — Split 1686-line `scheduler.go` into domain files *(completed)*
  ✅ scheduler_core.go (254 lines), scheduler_tasks.go (1101 lines), scheduler_triggers.go (69 lines), scheduler_maintenance.go (344 lines)
- [x] **STRUCT-12** — Create `internal/util/normalize.go` path/string normalization helper *(completed)*
  ✅ NormalizePath, NormalizeTitle, NormalizeAuthor, NormalizeString, CollapseSpaces; 45 call-chain replacements across 5 files
- [x] **STRUCT-13** — Finish splitting `BookDetail.tsx` (2773 lines) into sub-components *(completed)*
  ✅ See STRUCT-9 above — BookDetail.tsx reduced to 1073 lines

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

- [x] **DIAG-1** Fix `ApiError: store does not implement AIJobsStore` on Diagnostics page — `AIJobsStore` interface (`iface_misc.go:255-265`) has no methods implemented in `sqlite_store.go` or `pebble_store.go`; crash occurs when `batch_poller` asserts `s.Store().(database.AIJobsStore)` — PR #570
- [x] **DIAG-2** Expand Diagnostics to surface DB health — SQLite table row counts, PebbleDB key counts, embeddings DB stats, `ai_scans.db` stats, recently-rejected metadata with reasons, `metadata_fetch` cache hit/miss/age — PR #570
- [x] **DIAG-3** Surface `ai_scans.db` and `embeddings.db` stats in Diagnostics — both are opened in `server.go:934-1004` but never shown on the diagnostics or system-info pages — PR #570
- [x] **DIAG-4** Increase `MetadataFetchCacheTTLDays` default — metadata_fetch cache TTL (configured via `config.AppConfig.MetadataFetchCacheTTLDays`) is expiring too fast; increased default to 30 days — PR #570
- [x] **DIAG-5** Add path-prefix diagnostic to Storage page UI — `GET /api/v1/diagnostics/db-health` now returns `book_path_prefixes`; surface this in StorageTab so mismatches between configured import paths and actual stored paths are visible without a separate API call
- [x] **CACHE-FOLLOWUP-1** Metadata-fetch cache TTL enforcement — `GetCachedMetadataFetchWithMaxAge` centralizes the TTL check and emits `metrics.RecordCacheMiss("metadata_fetch","expired")`; `GetCachedMetadataFetch` is a backward-compat maxAge=0 wrapper; all 7 non-test callers updated; 3 new TTL unit tests — PR feat/metadata-fetch-ttl

---

## 🖥️ System Page Cleanup

- [x] **SYS-1** Remove duplicate log viewer from System page — System page uses `/system/logs` (a different endpoint and data model from Activity); replace with a navigation link to the Activity page
- [x] **SYS-2** Fix Storage page showing 0 books for `/mnt/bigdata/books/newbooks` — removed `is_primary_version` filter from `GetAllImportPaths` live subquery; added `GetBookPathPrefixes` diagnostic — PR #572

---

## 🔍 Data Quality & Matching Improvements

- [x] **MATCH-1** Deduplicate books by metadata URL/response hash — `metadata_source_hash` column added to `books` (migration 055); `sha256("{source}:{canonical_id}")` populated on metadata apply; duplicate count surfaced in BookDetail — PR #573
- [x] **MATCH-2** Consolidate multi-file chapter books by duration — files with sequential naming (`01 - Book`, `02 - Book`, etc.) that are individually very short (< 10 min each) should be pre-consolidated into a single book entry using cumulative duration rather than treated as separate books
- [x] **MATCH-3** Use duration as metadata scoring signal — boost metadata candidates whose Audible `runtime_length_min` roughly matches local file total duration; combine with existing title/author/series scoring for much higher confidence matches
- [ ] **MATCH-4** Deduplicate on same-metadata-hash at import time — when a new book is scanned and its computed `metadata_source_hash` matches an existing book, automatically flag/merge instead of creating a new record

---

## 🔐 File Identity & SHA Tracking

- [x] **FILE-SHA-1** Pre-metadata-write SHA capture — `original_file_hash` recorded before any tag write; `post_metadata_hash` column added to `book_files` (migration 053); `UpdateBookFileHashes()` wired around all write-back paths — PR #571
- [x] **FILE-SHA-2** Cross-folder duplicate detection via SHA — use `original_file_hash` to identify identical files across different library paths (e.g. same file in iTunes + Deluge + organized); surface as consolidation candidates in the dedup UI

---

## 🗃️ Rejected Metadata Store

- [x] **META-REJ-1** Rejected metadata tracking — `metadata_rejections` table (migration 054); `RejectedMetadataStore` interface; SQLiteStore + PebbleStore implementations; `GET /api/v1/audiobooks/:id/metadata-rejections` endpoint; rejection history collapsible section in BookDetail UI — PR #571

---

## 🖼️ UX Polish — Spacing & Layout

- [x] **UX-FOOTER** Footer spacer on every page — `MainLayout.tsx` now renders a 56px `aria-hidden` spacer after `{children}` so content never bumps the bottom edge of the viewport

---

## 🔄 Async Backfill Operations — Queue, Bell, Resume

All backfill handlers currently run **synchronously inside the HTTP request**. If the server
restarts mid-run they silently stop and will not auto-resume. They also don't appear in Active
Operations or the notification bell. These need the same treatment as `composer_tag_scan` and
`missing-file-repair`: `s.queue.Enqueue` → `operations.SaveParams` → `SaveCheckpoint` loop →
`activity.EmitInfo` summary on finish.

- [ ] **BACKFILL-ASYNC-1** `handleBackfillFileHashes` — convert to async queued operation:
  - `operations.BackfillFileHashesParams{DryRun bool}` struct in `state.go`
  - Enqueue as `"backfill-file-hashes"`, return `opID` immediately
  - Worker loop: for each `book_file` missing hash, `SaveCheckpoint` every N files
  - On restart: `LoadCheckpoint` → skip already-processed file IDs (by index or file_id cursor)
  - `activity.EmitInfo` summary on completion; `activity.LogBatch` for errors
  - Poll via `GET /api/v1/operations/{id}`; UI "Backfill Missing Hashes" button uses opID

- [ ] **BACKFILL-ASYNC-2** `handleBackfillMetadataSourceHash` — same async treatment:
  - `operations.BackfillMetadataHashParams{DryRun bool, Force bool}` struct
  - Enqueue as `"backfill-metadata-source-hash"`, return `opID`
  - Worker: iterate all books, checkpoint every N; skip-on-resume by `PhaseIndex`
  - `activity.EmitInfo` + `activity.LogBatch` on finish

- [ ] **BACKFILL-ASYNC-3** `MetadataHashDuplicateCard` UI — add coverage stats panel + backfill button matching the SHA Duplicate Detection card style:
  - `GET /maintenance/metadata-hash-stats` endpoint: total books, with/without `metadata_source_hash`, by-library breakdown
  - `BookMetadataHashStats` struct in `store.go`; `GetBookMetadataHashStats` in interface + SQLite + PebbleDB + mock
  - Auto-load stats on mount; status chip ("N missing hashes" / "✓ All hashed"); "Backfill Missing Hashes" button
  - Make sure `metadata_source_hash` is set in every metadata-cache path (already set in `ApplyMetadataCandidate`; verify fetch-cache replay path sets it too)

---

## 🔐 File Provenance / Hash Chain

Track the full lifecycle of a file's hash so we can answer "has this file changed since download?".
Proposed chain: **DownloadHash** (as-downloaded) → **OriginalFileHash** (after iTunes/external tagger) → **FileHash** (current, after AO).

- [ ] **HASH-CHAIN-1** Add `download_hash` column to `book_files` (SQLite migration + PebbleDB field). Populate it from Deluge import data (already have `deluge_hash`) and allow manual set via API.
- [ ] **HASH-CHAIN-2** UI: show hash chain in book file detail view so users can see when/where a file changed.
- [ ] **HASH-CHAIN-3** Integrity alert: flag files where `file_hash != original_file_hash` and no AO tag-write is on record (possible external modification / bit-rot).

*Low priority — AcoustID fingerprinting covers the identity-across-re-encode case better. Useful mainly for strict download-integrity auditing.*

---

## 🎵 AcoustID / Audio Fingerprinting — Stats & Trigger UI

AcoustID segment fingerprints already exist in the schema (`acoustid_seg0`–`seg6`). Needs the same coverage-stats + backfill-trigger treatment as file hashes.

- [ ] **ACOUSTID-STATS-1** `GetAcoustIDStats()` — count books/files with ≥1 fingerprint segment populated, by-library breakdown. Add to interface + SQLite + PebbleDB + mock.
- [ ] **ACOUSTID-STATS-2** `GET /maintenance/acoustid-stats` handler + route.
- [ ] **ACOUSTID-STATS-3** UI card on Maintenance tab (same tile style as SHA Duplicate Detection): shows coverage %, "Fingerprint Library" trigger button, status chip.
- [ ] **ACOUSTID-DEDUP-1** Use fingerprint similarity to detect duplicates even when hashes differ (re-encodes, format conversions). Show in Maintenance as "Acoustic Duplicates" card.
- [ ] **ACOUSTID-COMPARE-1** Manual comparison tool — given two book IDs or file IDs, compute/fetch their fingerprint segments and return a similarity score + per-segment breakdown. `POST /api/v1/books/{id}/compare-acoustid?other={id2}` (or file-level). UI: picker in book detail or Maintenance tab that lets you select any two books/files and shows:
  - Both books/files displayed side-by-side (title, author, cover, duration, format)
  - Overall similarity score (0–100%)
  - Per-segment diff: seg0 (intro), seg1–5 (body), seg6 (outro) — each segment shown as a colored match/mismatch bar with its individual score
  - Clear visual indication of which segments match, which differ, and by how much

---

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
- [ ] **1.11** **Async embed via OpenAI Batch API for nightly re-scans** — submit FullScan as a single Batch job (`endpoint=/v1/embeddings`), 50% discount + 24h SLA, results routed via the existing universal batch poller. Sync path stays for interactive callers. Spec: [`docs/superpowers/bot-tasks/2026-05-04-async-embed-batch-api.md`](docs/superpowers/bot-tasks/2026-05-04-async-embed-batch-api.md)
- [ ] **1.12** **Tag operation log lines with the originating operation ID** — pipe `op.ID` into a context-bound logger, replace bare `log.Printf` inside operation funcs with op-scoped calls, and write each line into `operation_logs` so the Activity-page log view shows everything (ffmpeg warnings, fingerprint failures, etc.) instead of only the explicit `progress.Log()` calls. Spec: [`docs/superpowers/bot-tasks/2026-05-04-tag-operation-logs.md`](docs/superpowers/bot-tasks/2026-05-04-tag-operation-logs.md)
- [ ] **1.13** **Broken-files dashboard card + repair pipeline** — persist per-file ffmpeg / fingerprint errors to a new `book_file_errors` table associated with the book, surface a dashboard card ("N books with broken files"), add a `has_file_errors` library facet, and wire a repair pipeline (remux / restore-from-version / mark-ignored / delete-and-rescan). Pairs with 1.12. Spec: [`docs/superpowers/bot-tasks/2026-05-04-broken-files-card-and-repair.md`](docs/superpowers/bot-tasks/2026-05-04-broken-files-card-and-repair.md)
- [x] **1.14** **Unified Operations System (UOS)** — COMPLETE 2026-05-11 (infra 2026-05-08, full migration 2026-05-11). All 16 UOS tasks shipped across PRs #740–#759; v1→v2 `queue.Enqueue` migration completed across PRs #783–#797. Every `s.queue.Enqueue` call in `internal/server/` now routes through `s.opRegistry.EnqueueOp`. Single `Registry` owns every OperationDef; plugins register through `pkg/plugin/sdk`; subprocess isolation; explicit `ResumePolicy`; single SSE-fed frontend store. Human spec: [`docs/superpowers/specs/2026-05-04-unified-operations-system.md`](docs/superpowers/specs/2026-05-04-unified-operations-system.md). **Remaining legacy infra:** `triggerOperation`/`triggerOperationWithID` helpers in `scheduler_triggers.go` (dead code, safe to delete); `s.queue.EnqueueResume` in `server_lifecycle.go` (fallback for scan/organize interrupted ops — remove once those ops have v2 resume support).
  - [x] **UOS-01** Schema migrations for v2 tables (merged 2026-05-06)
  - [x] **UOS-02** Registry shell + dispatcher + in-process worker pool (PR #741, merged 2026-05-06)
  - [x] **UOS-03** Reporter DB writes + subprocess runner (PR #745, merged 2026-05-06)
  - [x] **UOS-04** Public plugin SDK at `pkg/plugin/sdk` + import lint tool (PR #746, merged 2026-05-06)
  - [x] **UOS-05** Frontend dual-source operations store (PR #740, merged 2026-05-06)
  - [x] **UOS-06** SSE EventHub + /operations/timeline + introspection endpoints (PR #748, merged 2026-05-06)
  - [x] **UOS-07** Canary — migrate `dedup.embed-scan` as the first live plugin op (PR #747, merged 2026-05-06)
  - [x] **UOS-08** Watchdog + op_strikes_v2 + startup resume orchestration (PR #744, merged 2026-05-06)
  - [x] **UOS-09** Migrate AcoustID + remaining dedup ops to UOS (PR #750, merged 2026-05-08)
  - [x] **UOS-10** Migrate iTunes plugin (5 ops) to UOS (PR #753, merged 2026-05-08)
  - [x] **UOS-11** Migrate Deluge plugin (3 ops) to UOS (PR #752, merged 2026-05-08)
  - [x] **UOS-12** Migrate 26 maintenance ops to UOS plugin (PR #751, merged 2026-05-08)
  - [x] **UOS-13** Frontend single-source — drop dual-source (PR #754, merged 2026-05-08)
  - [x] **UOS-14** Delete v1 OperationQueue + legacy endpoints (PR #756, merged 2026-05-08)
  - [x] **UOS-15** Promote pkg/plugin/sdk to stable public API + sdkguard CI (PR #755, merged 2026-05-08)
- [ ] **1.15** **UOS amendment — `Reporter.SetCurrentItem(label)` for live "currently working on" ticker** — Sonarr/Radarr-style high-frequency current-item display under the progress bar. New SDK Reporter method that's purely ephemeral (in-memory on the registry's run handle, no DB write); SSE event `op.current_item` patches the frontend store; timeline endpoint returns the cached value so refresh / new tab / re-login re-hydrates. Survives refresh; survives a brief gap on server restart (next per-item iteration repopulates). If we ever want it to survive restart, retrofit is a single column add to `operations_v2` flushed at 30s cadence — explicit out of v1. Implementation footprint: amend §1 (Reporter) + §9 (timeline payload) + UOS-03/UOS-06 bot-tasks. Spec: [`docs/superpowers/bot-tasks/2026-05-05-uos-amendment-current-item.md`](docs/superpowers/bot-tasks/2026-05-05-uos-amendment-current-item.md).
- [ ] **1.16** **Resizable + dynamically-sortable columns everywhere** — every page that renders a table (library, dedup, activity, iTunes write-back preview, metadata review, etc.) should have draggable column dividers and click-to-sort headers, persisted per-user. Today some pages have it, most don't, and the inconsistency is jarring. Build a single `<ResizableSortableTable>` component (or extend the existing `ConfigurableTable`); roll across pages in follow-ups. Acceptance: every column on every page resizes; every column on every page sorts; user-resized widths and sort states persist via `localStorage` keyed on page+column.
- [ ] **1.17** **Replace "AO" / "audiobook-organizer" branding with a real product name + logo** — the placeholder "AO" leaks into UI labels (e.g. proposed "AO Path" column on the iTunes write-back dialog), service names, status panels, etc. Pick a product name + minimal logo, apply consistently. Out of scope until name is decided; this entry is the placeholder for the rename sweep.

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
- [x] **4.11** Split `internal/server` into sub-packages (**XL**) — all 8 PKG tasks completed
  - ✅ **PKG-1** `internal/audiobooks/` — audiobook service extracted (#663)
  - ✅ **PKG-2** `internal/aiscan/` — AI scan pipeline extracted (#656)
  - ✅ **PKG-3** `internal/reconcile/` — reconcile logic extracted (#657)
  - ✅ **PKG-4a** `internal/scanner/` — scan service extracted (#658)
  - ✅ **PKG-4b** `internal/importer/` — import services extracted (#660)
  - ✅ **PKG-4c** `internal/quarantine/` — quarantine service extracted (#662)
  - ✅ **PKG-4d** `internal/writeback/` — writeback enqueuer/outbox extracted (#661)
  - ✅ **PKG-4e** `internal/fileops/` + `internal/sysinfo/` — filesystem/system services extracted (#664)
- [ ] **4.12** Narrow extracted service dependencies to ISP sub-interfaces (**M**) — after 4.8 + 4.11, update extracted packages to accept narrow store interfaces (BookReader, etc.) instead of full database.Store
- [ ] **4.13** Extract iTunes integration into `internal/itunes` (**L**) — decouple iTunes import/sync/writeback from Server lifecycle; currently ~3,900 LOC deeply coupled to Server, needs interface extraction and dependency injection redesign
  - [x] **4.13b** Unit tests for `internal/itunes/service/track_provisioner.go` — 11 tests: multi-segment, missing metadata, idempotency, UpsertBookFile error, managed/unmanaged paths, PID uniqueness, duration conversion, partial-failure best-effort. (`track_provisioner_test.go`, bot-task 4-13b)
  - [x] **4.13d** Error and edge-case tests for `internal/itunes/service/importer.go` (21 new tests; disabled-mode, corrupt ITL, concurrent sync, tombstoned PID, existing-PID link, SkipDuplicates, partial write, Sync GetAllBooks failure, cover-art missing, linkITunesMetadata, linkAsVersion, organizeOneBook nil/no-factory)

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
- [x] [2026-04-15 Bulk organize undo](docs/superpowers/plans/2026-04-15-bulk-organize-undo.md) — complete (tasks 1-6 + torrent move_storage PR)
- [x] [2026-04-15 Library centralization](docs/superpowers/plans/2026-04-15-library-centralization.md) — all tasks done including deluge integration (PR feat/deluge-centralization)
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

---

## 2026-05-01 Re-Audit Bot Tasks

Findings from the 2026-05-01 re-audit. See `docs/codebase-evaluation.md` §Re-Audit for evidence.

### High Priority

- **TEST-1** Fix 11+ failing unit tests in `internal/server` after PROJ-1/PROJ-2 changed `GetAllBooks` → `GetAllBookSummaries`  
  Spec: `docs/superpowers/bot-tasks/2026-05-01-test-1-fix-audiobook-service-tests.md`

- **TEST-2** Fix `TestStoreAdditionalCoverageSQLite` failure in `internal/database` package  
  Spec: `docs/superpowers/bot-tasks/2026-05-01-test-2-fix-database-test-coverage.md`

### Medium Priority

- **DEP-1** Overview: migrate ~34 deprecated `Book.ITunesPath` usages across 4 packages to `BookFile.ITunesPath` (SA1019). See sub-tasks below.  
  Overview: `docs/superpowers/bot-tasks/2026-05-01-dep-1-migrate-itunes-path-field.md`

  - **DEP-1a** metafetch package — `batch.go` + `service.go` (~9 usages)  
    Spec: `docs/superpowers/bot-tasks/2026-05-01-dep-1a-metafetch-itunes-path.md`

  - **DEP-1b** organizer package — `service.go` (1 usage)  
    Spec: `docs/superpowers/bot-tasks/2026-05-01-dep-1b-organizer-itunes-path.md`

  - **DEP-1c** server handlers — `itl_rebuild.go` + `metadata_batch_candidates.go` (6 usages)  
    Spec: `docs/superpowers/bot-tasks/2026-05-01-dep-1c-server-itunes-path.md`

  - **DEP-1d** itunes/service package — `importer.go`, `path_reconcile.go`, `path_repair.go`, `writeback_batcher.go` (~14 usages)  
    Spec: `docs/superpowers/bot-tasks/2026-05-01-dep-1d-itunes-service-path.md`

  - **DEP-1e** (blocked on 1a–1d) DB migration to drop `books.itunes_path` column and remove sqlite_store.go usages

- **DEAD-1** Remove dead code: `legacySaveConfigToDatabase_REMOVED`, `bookTagKeyspace`, `bookSummarySelectColumnsQualified`, `linkAsVersion`, SA4006 unused values  
  Spec: `docs/superpowers/bot-tasks/2026-05-01-dead-1-remove-unused-code.md`

- **CTX-4** Thread `context.Context` through `ActivityStore.Summarize` and `CompactByDay` transactions  
  Spec: `docs/superpowers/bot-tasks/2026-05-01-ctx-4-activity-store.md`

- **PERF-1** Paginate 20+ unbounded `GetAllBooks(0,0)` calls in background jobs (OOM risk)  
  Spec: `docs/superpowers/bot-tasks/2026-05-01-perf-1-paginate-getallbooks.md`

### Low Priority

- **LOG-5** Replace remaining `fmt.Printf`/`log.Printf` in `sqlite_store`, `pebble_store`, `migrations`, `playlist`, `organizer` with structured `slog` calls  
  Spec: `docs/superpowers/bot-tasks/2026-05-01-log-5-remaining-printf.md`

- **R-9** Remove stale `// TODO: Implement in N1-2` comments from `sqlite_store.go:6913,6946` (already implemented)  
  Spec: `docs/superpowers/bot-tasks/2026-05-01-r9-remove-stale-todo-comments.md`

- **R-10** Fix 12 capitalized error strings in metadata packages (staticcheck ST1005):  
  `internal/metadata/audible.go`, `audnexus.go`, `googlebooks.go`, `hardcover.go`, `openlibrary.go`, `wikipedia.go`  
  Spec: `docs/superpowers/bot-tasks/2026-05-01-r10-fix-capitalized-error-strings.md`
