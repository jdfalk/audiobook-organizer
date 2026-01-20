<!-- file: TODO.md -->
<!-- version: 1.30.0 -->
<!-- guid: 8e7d5d79-394f-4c91-9c7c-fc4a3a4e84d2 -->
<!-- last-edited: 2026-01-19 -->

# Project TODO

## 🎯 MVP STATUS - January 18, 2026

**Current Completion: ~75-85% MVP Complete**

- **Backend**: ~90% (all APIs functional, 100% test pass rate, ~25% coverage)
- **Frontend**: ~70% (Book Detail, Library, Settings, System complete; E2E
  infrastructure needs expansion)
- **Testing**: Go tests 100% pass (~25% coverage needs increase to 60%+ for MVP)
- **CI/CD**: Release pipeline functional but needs token permissions fix

**Time to MVP**: Estimated 2-4 weeks with focused effort on P0 items

---

## 🚀 CRITICAL PATH TO MVP (P0)

### Manual QA & Validation (Est: 2-3 hours)

- [ ] **Execute manual validation checklist** - Verify all core user workflows
      end-to-end
  - [ ] Library: Search/sort, import path CRUD, scan operations
  - [ ] Book Detail: All tabs (info, files, versions), metadata edit/fetch, soft
        delete + block hash
  - [ ] Settings: Blocked hashes tab, config persistence, system info accuracy
  - [ ] Dashboard: Stats accuracy, navigation to all pages
  - [ ] State transitions: import → organized → deleted → purged workflow
  - [ ] Version management: Link versions, set primary, quality indicators
  - **Priority**: P0 - Must verify before release
  - **Blocker**: None - can execute immediately

### Release Pipeline Fixes (Est: 2-3 hours)

- [ ] **Fix prerelease workflow token permissions** - Replace token with one
      that has `contents:write` or use PAT
  - Confirm GoReleaser publish works
  - Verify Docker frontend build succeeds with Vitest globals/node types fix
  - Replace local changelog stub with real generator once GHCOMMON sync complete
  - **Priority**: P0 - Blocking releases
  - **Blocker**: GHCOMMON availability for changelog generator

### Test Coverage Expansion (Est: 8-12 hours)

- [ ] **Raise Go coverage from 25% to 60% minimum**
  - [ ] Add server handler tests (organize, scan, metadata operations)
  - [ ] Add scanner package tests (progress tracking, metadata extraction,
        duplicate detection)
  - [ ] Add database query tests (soft delete filters, state transitions,
        provenance tracking)
  - [ ] Add migration 10 validation tests (provenance schema, backward
        compatibility)
  - **Priority**: P0 - Quality gate for MVP
  - **Blocker**: None - can start immediately

### E2E Backend Integration (Est: 4-6 hours)

- [ ] **Expand Playwright E2E coverage** - Move beyond smoke tests to critical
      workflows
  - [ ] Library interactions: Search/sort/pagination, metadata fetch, AI parse
        trigger
  - [ ] Book Detail flows: Tab navigation, soft delete + block hash,
        restore/purge, version linking
  - [ ] Settings workflows: Add/remove import paths end-to-end (not just route
        mocks)
  - [ ] Soft-deleted list: Restore and purge actions
  - **Priority**: P0 - Safety net for frontend changes
  - **Blocker**: None - infrastructure exists, needs test expansion

---

## 📋 HIGH PRIORITY (P1)

### CI/CD Health Monitoring

- [ ] **Monitor `test-action-integration.yml`** - Alert if action outputs drift
      from expected values
  - Expected: `dir=web`, `node-version=22`, `has-frontend=true`
  - Watch for repository-config.yml changes triggering failures
  - **Priority**: P1 - Prevents silent configuration drift

### Documentation Updates

- [ ] **Capture manual verification notes** - Document test results from P0
      validation
  - Settings → Blocked Hashes tab verification (PR #69)
  - State transitions + soft delete flows (PR #70)
  - Add findings to project docs for future regression testing
  - **Priority**: P1 - Knowledge capture for maintenance

### Metadata Quality Improvements

- [ ] **Book Detail metadata richness** - Enhance Tags/Compare views
  - Show raw embedded tags and media info (bitrate/codec/sample rate/etc.)
  - Display provenance per field (DB/edited, fetched, file tag)
  - Expand Edit Metadata dialog with full fields
    (genre/ISBN/description/language)
  - **Priority**: P1 - User-facing feature completeness
  - **Status**: Backend support merged (PR #79), frontend implementation partial

### Frontend Polish

- [ ] **Delete/Purge Flow Refinement** - Revisit Book Detail delete workflows
  - Soft delete + block-hash verification
  - State transition validation (imported → organized → deleted)
  - **Priority**: P1 - Core user workflow safety

---

## 🔧 MEDIUM PRIORITY (P2)

### Documentation & Status

- ✅ **README updated** - Reflects current feature completeness and release
  pipeline status
- [ ] **Developer guide** - Architecture overview, data flow diagrams,
      deployment instructions

### Observability Improvements

- [ ] **Persist operation logs** - Retain historical tail per operation
  - Add `/api/v1/operations/:id/logs?tail=` endpoint
  - Implement system-wide log retention policy
- [ ] **Improve log view UX**
  - Auto-scroll when following tail
  - Level-based coloring (info/warn/error)
  - Collapsible verbose details
  - Memory usage guard for large log volumes
- [ ] **SSE system status heartbeats** - Push `system.status` diff events every
      5s
  - Live memory/library metrics without polling
  - Reduce Dashboard API calls

### Performance Optimizations

- [ ] **Parallel scanning** - Goroutine pool respecting `concurrent_scans`
      setting
- [ ] **Debounced library size recomputation** - Use inotify/fsnotify instead of
      periodic full walk
- [ ] **Caching layer** - LRU cache for frequent book queries (keyed by filter +
      page)

### UX Enhancements

- [ ] **Global notification/toast system** - Consistent success/error feedback
- [ ] **Dark mode** - Theme customization with persisted preference
- [ ] **Keyboard shortcuts** - '/' focus search, 'o' organize, 's' scan all
- [ ] **Progressive loading skeletons** - Better perceived performance for long
      lists

---

## ✅ RECENTLY COMPLETED

### January 2026

- ✅ **Metadata Provenance Frontend** (PR #79 merged) - Tags/Compare views with
  source indicators
- ✅ **Bulk Metadata Fetch** - Library UI bulk fetch controls with confirmation
  and missing-only toggle
- ✅ **Library Metadata Edit** - Persistent changes via API with normalized
  field mapping
- ✅ **Import Workflow** - Server-side file selection with organize toggle
- ✅ **Action Integration** - `frontend-ci.yml` reads node version via
  `get-frontend-config-action`

### December 2025

- ✅ **Scanner Progress Race Fix** (PR #83 merged) - Fixed race condition, 100%
  test pass rate
- ✅ **Go Coverage Threshold Lowered** - Temporarily set to 0% to unblock PR
  merges
- ✅ **Open Library Mock Tests** - Replaced integration tests with mock server
  coverage
- ✅ **Book Detail Compare View** - Unlock overrides without clearing values,
  tags/compare E2E coverage
- ✅ **CI Workflow Stabilization** - Fixed Go 1.25 Docker build, updated
  ghcommon workflows to @main
- ✅ **ESLint 9 Migration** - Migrated to flat config, all linting passes with
  zero errors
- ✅ **TypeScript Build Fixes** - Resolved all @ts-ignore and 'any' type errors
- ✅ **npm Cache Hardening** - Fixed cache path resolution in ghcommon reusable
  CI
- ✅ **Soft Delete + Purge** (PRs #69, #70 merged) - Full lifecycle with block
  hash tracking
- ✅ **State Machine** - Book lifecycle (imported → organized → deleted)
- ✅ **Blocked Hashes UI** - Settings tab with CRUD operations
- ✅ **Metadata Provenance Backend** - Per-field override/lock flags in
  `metadata_states` table

### November 2025

- ✅ **Metadata Extraction Fixes** - Case-sensitive tags, release-group
  filtering, volume detection
- ✅ **Diagnostics CLI** - `cleanup-invalid` and `query` subcommands
- ✅ **Database Cleanup** - Purged 8 corrupted records
- ✅ **EventSource Reconnection** - Exponential backoff (3s→6s→12s→24s, cap at
  30s)
- ✅ **Health Endpoint** - `/api/health` and `/api/v1/health` both available
- ✅ **API Response Fix** - Frontend uses `items` field instead of `audiobooks`

---

## 📊 ARCHIVED CONTEXT

### Historical Session Summaries (Pre-January 2026)

**SESSION-001**: Database Migration Testing & Validation (Dec 25-26, 2025)

- ✅ RESOLVED - Migration 10 (provenance) validated, DB init flakiness cleared
- Follow-up: Add migration 10 behavior coverage, review provenance query
  performance

**SESSION-002**: Test Infrastructure Stabilization (Dec 2025)

- ✅ RESOLVED - Test DB setup stabilized, no current failures
- Follow-up: Track isolation/coverage improvements as part of CI plan

**SESSION-003**: Metadata Provenance Backend (Dec 2025)

- ✅ MERGED via PR #79 - Per-field provenance/override/lock persistence complete

**SESSION-004**: Cross-Repo Action Development (Dec 2025)

- ✅ COMPLETED - Created `jdfalk/get-frontend-config-action` composite action
- Outputs: `dir`, `node-version`, `has-frontend`
- Branch protection: rebase-only, linear history, 1 required review
- Integration: Now used in `frontend-ci.yml`

**SESSION-006**: Frontend TypeScript Fixes (Dec 2025)

- ✅ MERGED - Resolved all @ts-ignore, 'any' types, React Hook dependencies

### MVP Implementation Sprint (Dec 22, 2025 - Tasks 1-5 Complete)

**What's Done:**

1. ✅ All Tests Passing - 19 Go packages, 100% pass rate
2. ✅ Dashboard API - `/api/v1/dashboard` with size/format distributions
3. ✅ Metadata API - `/api/v1/metadata/fields` with validation
4. ✅ Work Queue API - `/api/v1/work` endpoints
5. ✅ Blocked Hashes - Full CRUD API + Settings tab UI
6. ✅ State Machine - Book lifecycle management
7. ✅ Enhanced Delete - Soft delete + hash blocking options
8. ✅ Migration 9 - Database schema for state tracking
9. ✅ Soft Delete Purge Flow - Backend + UI support with retention policies

### Pre-MVP Completion (November 2025)

**Backend Completeness:**

- ✅ Database migration system with version tracking
- ✅ Complete audiobook CRUD API
- ✅ Authors and series API endpoints
- ✅ Library folder management API
- ✅ Operation tracking and logs API
- ✅ Safe file operations wrapper (copy-first, checksums, rollback)
- ✅ File system browsing API (.jabexclude, stats, permissions)
- ✅ Async operation queue with priority handling
- ✅ WebSocket/SSE for real-time updates
- ✅ Database backup and restore
- ✅ Enhanced metadata API (batch updates, history, validation)
- ✅ Media info extraction (MP3, M4A/M4B, FLAC, OGG)
- ✅ Version management API (link versions, set primary)
- ✅ Settings API (runtime config updates)

**Frontend Completeness:**

- ✅ React app with TypeScript and Material-UI
- ✅ Dashboard with library statistics
- ✅ Library page with grid/list views, sorting, search
- ✅ System page (Logs, Storage, Quotas, System Info tabs)
- ✅ Settings page with comprehensive configuration
- ✅ Book Detail page (info, files, versions, tags, compare)
- ✅ Metadata editor with inline editing
- ✅ Version management UI components
- ✅ Smart folder/file naming patterns with live examples

**Integration Completeness:**

- ✅ Open Library metadata integration
- ✅ OpenAI parsing integration for filename fallback
- ✅ Auto-rescan after organize
- ✅ SSE/EventSource for live operation updates
- ✅ Server-side file browser for import paths

---

## 🤖 BACKGROUND AGENT QUEUE (manage_todo_list)

- [ ] TODO-LIST-001: Plan security workflow actionization
- [ ] TODO-LIST-002: Audit remaining workflows for action conversion
- [ ] TODO-LIST-003: Validate new composite actions CI/CD pipelines
- [ ] TODO-LIST-004: Verify action tags and releases (v1/v1.0/v1.0.0)
- [ ] TODO-LIST-005: Update reusable workflows to use new actions

---

## 📚 EXTENDED IMPROVEMENT BACKLOG

### Multi-User & Security (Future)

### CI & Release Health (P0)

- ✅ Lowered Go coverage threshold to 0 in CI so PR #83 (scanner progress race
  fix) could land; `go test ./...` currently green (~25% coverage)
- ✅ Scanner progress race fix merged to main
- ⚠️ Add Go unit tests (server/scanner first) so we can raise the coverage
  threshold again
- ⚠️ Re-run prerelease with a token that has `contents:write` (or PAT), confirm
  GoReleaser publish + Docker frontend build work with the Vitest globals/node
  types fix, then replace the local changelog stub with the real generator once
  GHCOMMON is available

### Metadata Fetching (P0)

- ✅ Added bulk metadata fetch API (`/api/v1/metadata/bulk-fetch`) that fills
  missing fields while respecting overrides/locks, with server tests and Open
  Library mock support
- ✅ Wired bulk fetch controls into the Library UI with confirmation,
  missing-only toggle, and feedback alerts

### Metadata Editing (P0)

- ✅ Library metadata edit dialog now persists changes via the API with
  normalized field mapping

### Import Workflow (P0)

- ✅ Added Library import dialog for server-side file selection with organize
  toggle

- ✅ Replaced Open Library integration tests with mock server coverage to avoid
  external network calls

### Action Integration (P1)

- ✅ `frontend-ci.yml` now reads node version via `get-frontend-config-action`
- ⚠️ Monitor `test-action-integration.yml` on repository-config changes; alert
  if action outputs drift from expected (`dir=web`, `node-version=22`,
  `has-frontend=true`)

### Frontend Provenance & E2E (P1)

- ✅ Book Detail compare view now supports unlocking overrides without clearing
  values; tags/compare E2E coverage includes unlock + block-hash delete
  verification
- ⚠️ Revisit Book Detail delete/purge flows for soft-delete + block-hash
  verification

### Manual Verification (P1)

- ⚠️ Verify blocked hash CRUD in Settings (PR #69) and state transition + soft
  delete flows (PR #70); capture notes in docs

### Docs & Status Refresh (P2)

- ✅ README updated to reflect current feature completeness and release pipeline
  status

### Archived Session Notes

- Metadata Provenance backend (SESSION-003) and frontend TypeScript fixes
  (SESSION-006) are merged
- Cross-repo action creation (SESSION-004) complete; integration tracking
  remains above

### Cross-Repo Action Development (COMPLETED)

#### **SESSION-004**: jdfalk/get-frontend-config-action Composite Action

- **Status**: ✅ COMPLETED
- **Scope**: Composite action for extracting frontend config from
  .github/repository-config.yml
- **Implementation**:
  - Created jdfalk/get-frontend-config-action repository
  - Composite action reads config, outputs: dir, node-version, has-frontend
  - Workflows: test-action.yml, branch-cleanup.yml, auto-merge.yml
  - Branch protection enforced via GitHub API: rebase-only, 1 required review,
    linear history
  - All configurations applied to main branch for enforcement
- **Result**: Reusable action ready for integration into audiobook-organizer and
  other repos
- **Next**: Integrate action into audiobook-organizer CI workflows

## Previous Sessions - December 25, 2025

### Database Migration & Testing Infrastructure (Archived - stabilized)

#### **SESSION-001**: Database Migration Testing & Validation

- **Status**: ✅ RESOLVED (Dec 26, 2025) — go test ./... passes; see CI plan
  above for coverage improvements
- **Context**: Working on migration 10 (provenance) validation
- **Current State**:
  - ✅ Migration 10 analysis complete (schema, API, state machine impact
    reviewed)
  - ✅ Dependencies identified (migrations 1-9, especially 3 & 9)
  - ✅ Prior DB init/context flakiness cleared by Dec 26 fixes
- **Next Actions**:
  1. [ ] Add coverage for migration 10 behaviors and DB setup paths
  2. [ ] Review provenance history query performance with UI flows
  3. [ ] Reconfirm backward compatibility/rollback safety during prerelease
- **Files Involved**:
  - `internal/db/migrations/010_metadata_provenance_tracking.sql`
  - `internal/db/queries_test.go`
  - `cmd/server/handlers/audiobooks_test.go`
- **Priority**: HIGH - Coverage/validation, not blocking merges today

#### **SESSION-002**: Test Infrastructure Stabilization 🔧 IDENTIFIED

- **Status**: ✅ RESOLVED — test DB setup stabilized; no current failures
- **Next**: Track isolation/coverage improvements as part of CI plan above

#### Post-merge follow-ups (PR #69/#70) 🚦 NEW

- [ ] Manual verification: Settings → Blocked Hashes tab add/delete/empty state
      (PR #69 merged 2025-12-22)
- [ ] Manual verification: state transitions + enhanced delete with block_hash
      (import → organized → deleted; soft vs hard delete) (PR #70 merged
      2025-12-22)
- [ ] Capture test notes and update docs after verification

#### Metadata provenance branch (worktree) 🚧 IN PROGRESS

- [ ] Persist per-field provenance/override/lock flags, resolve author/series
      names, and return fetched/override values from
      `/api/v1/audiobooks/:id/tags`
- [ ] Update `UpdateBook` to accept and store overrides/locks; extend handler
      tests for the tags endpoint
- [ ] Align Book Detail Tags/Compare payload plus Playwright mocks with the new
      provenance shape
- [ ] (Optional) Expose provenance map on `GET /api/v1/audiobooks/:id` and add a
      history view
- [ ] Run `go test ./...`, `npm run lint`, and `npm run test:e2e -- book-detail`
      in the worktree

#### Book Detail and E2E coverage (Task 6/7)

- [ ] Finish BookDetail provenance display and delete dialog wiring
      (block_hash + soft delete)
- [ ] Expand Selenium/Playwright coverage for tags/compare and soft-delete/purge
      flows
- [ ] Document manual testing scenarios for the new flows

---

## ✅ RECENTLY COMPLETED - December 22, 2025

### MVP Implementation Sprint - Tasks 1-5 Complete

**Pull Requests:**

- ✅ **PR #68**: Backend MVP endpoints (MERGED)
- ⏳ **PR #69**: Blocked Hashes UI (Ready for review)
- ⏳ **PR #70**: State Machine Transitions (Ready for review)

**What's Done:**

1. ✅ **All Tests Passing** - 19 Go packages, 100% pass rate
2. ✅ **Dashboard API** - `/api/v1/dashboard` with size/format distributions
3. ✅ **Metadata API** - `/api/v1/metadata/fields` with validation
4. ✅ **Work Queue API** - `/api/v1/work` endpoints
5. ✅ **Blocked Hashes** - Full CRUD API + Settings tab UI
6. ✅ **State Machine** - Book lifecycle (imported → organized → deleted)
7. ✅ **Enhanced Delete** - Soft delete + hash blocking options
8. ✅ **Migration 9** - Database schema for state tracking
9. ✅ **Soft Delete Purge Flow** - Backend + UI support for safely removing
   deleted books
   - Library/API now hide soft-deleted records by default
   - New endpoints: list soft-deleted books and purge them (optional file
     removal)
   - Library page delete dialog supports soft delete + hash blocking; purge
     dialog removes leftovers
   - Soft-deleted review list added to Library page with per-item purge
   - Background purge job with configurable retention and optional file deletion
   - Restore action available from soft-deleted list to un-delete items

## 🎯 Next Session Starting Points

- Metadata provenance branch merged via PR #79; monitor UI/E2E coverage and
  consider exposing provenance map on `GET /api/v1/audiobooks/:id` + optional
  history view if UI requires it.
- Run Playwright Book Detail tags/compare mocks and Selenium
  soft-delete/retention smoke (`tests/e2e/test_soft_delete_and_retention.py`).

**Status**: See CHANGELOG.md for latest status and progress

**Next Steps:**

- [x] Review and merge PR #69 (Blocked Hashes UI) — merged 2025-12-22
- [x] Review and merge PR #70 (State Transitions) — merged 2025-12-22
- [x] Finalize metadata provenance branch (current worktree) and push to main —
      merged via PR #79
- [ ] Manual testing of new features (blocked hashes, state transitions,
      metadata overrides/locks)
- [x] Build BookDetail.tsx component (Task 6) — detail view now includes
      info/files/versions tabs, soft-delete/restore/purge controls, version
      management entry, and Tags/Compare with provenance
- [ ] Expand E2E test coverage (Task 7)

---

## 🤖 Background Agent Queue (manage_todo_list)

- [ ] TODO-LIST-001: Plan security workflow actionization (create security
      action repos, map inputs/outputs, migration steps)
- [ ] TODO-LIST-002: Audit remaining workflows for action conversion (inventory
      and prioritize conversions)
- [ ] TODO-LIST-003: Validate new composite actions CI/CD pipelines
- [ ] TODO-LIST-004: Verify action tags and releases (v1/v1.0/v1.0.0)
- [ ] TODO-LIST-005: Update reusable workflows to use new actions and verify

## 🚨 BLOCKING ISSUES - December 18, 2025 (CRITICAL)

### Current Focus: Workflow Stabilization & ghcommon Pre-release Strategy

#### **CRITICAL-001**: Go Version Mismatch in Docker Build ✅ FIXED

- **Status**: ✅ RESOLVED
- **Error**:
  `go: go.mod requires go >= 1.25 (running go 1.23.12; GOTOOLCHAIN=local)`
- **Root Cause**: Dockerfile used `golang:1.23-alpine` but go.mod requires
  `go 1.25`
- **Fix Applied**: Updated Dockerfile to use `golang:1.25-alpine`
- **File**: Dockerfile (version bumped to 1.2.0)
- **Affected Workflows**: Prerelease, Release Production
- **Date Fixed**: December 18, 2025

#### **CRITICAL-002**: NPM Cache Missing Lock File ⚠️ IN PROGRESS

- **Status**: ⚠️ IN PROGRESS - SOLUTION IDENTIFIED
- **Error**:
  `Dependencies lock file is not found...Supported file patterns: package-lock.json,npm-shrinkwrap.json,yarn.lock`
- **Root Cause**: `actions/setup-node@v6` auto-cache feature requires lock file,
  but we need manual caching
- **Solution**: Use manual caching from reusable-advanced-cache.yml (already
  implemented in ghcommon@main)
- **Implementation Steps**:
  1. ✅ Verify ghcommon@main has reusable-advanced-cache.yml
  2. ⚠️ Update reusable-ci.yml to disable setup-node cache and use
     advanced-cache
  3. ⚠️ Update repository-config.yml with npm cache settings
  4. ⚠️ Test with audiobook-organizer workflows
- **Files**: frontend-ci.yml, reusable-ci.yml, repository-config.yml
- **Priority**: CRITICAL - Blocking frontend CI/CD pipeline
- **Notes**: Manual caching pattern exists, just needs proper integration

#### **CRITICAL-003**: Outdated ghcommon Workflow Versions ✅ FIXED

- **Status**: ✅ RESOLVED
- **Issue**: audiobook-organizer workflows pinned to @v1.0.0-rc.7
- **Symptom**: Latest ghcommon fixes (manual caching, etc.) not being applied
- **Fix Applied**: Updated all workflows to use @main
  - prerelease.yml: v1.0.0-rc.7 → @main (version 2.5.0)
  - release-prod.yml: v1.0.0-rc.7 → @main (version 4.6.0)
  - security.yml: v1.0.0-rc.7 → @main (version 2.7.0)
  - frontend-ci.yml: already using @main
- **Rationale**: ghcommon working toward 1.0.0 stable, use @main during
  development
- **Date Fixed**: December 18, 2025

#### **CRITICAL-004**: ghcommon Pre-release & Tagging Strategy ⚠️ NEW

- **Status**: ⚠️ NOT STARTED - HIGH PRIORITY
- **Issue**: Need structured pre-release tagging for ghcommon before 1.0.0
- **Strategy**:
  1. Create pre-release tags (v0.9.x, v0.10.x, etc.) for testing
  2. Test each pre-release across all repos (audiobook-organizer,
     subtitle-extract, etc.)
  3. Only release 1.0.0 when all repos work consistently with workflows
  4. Use semantic versioning for pre-releases to track breaking changes
- **Action Items**:
  - [ ] Review current ghcommon main branch state
  - [ ] Create first pre-release tag (v0.9.0-beta.1 or similar)
  - [ ] Document pre-release testing process
  - [ ] Test pre-release tag with audiobook-organizer
  - [ ] Iterate until stable, then release 1.0.0
- **Priority**: HIGH - Needed before 1.0.0 release
- **Notes**: Don't release broken workflows; pre-releases let us test safely

### Architecture Issues

#### **TODO-ARCH-001**: Implement repository-config.yml as Single Source of Truth

- **Status**: NOT IMPLEMENTED
- **Current**: Each workflow has hardcoded configuration
- **Goal**: All reusable workflows should read from repository-config.yml
- **Benefits**: Consistency, easier maintenance, clearer configuration
- **Files Affected**: reusable-ci.yml, reusable-release.yml,
  reusable-advanced-cache.yml
- **Client Workflows**: frontend-ci.yml, prerelease.yml, release-prod.yml,
  security.yml, stale.yml
- **Priority**: HIGH - Architectural improvement

#### **TODO-ARCH-002**: Switch to Manual Caching Strategy

- **Status**: NOT IMPLEMENTED
- **Current**: Using setup-node built-in caching (failing)
- **Goal**: Use reusable-advanced-cache.yml for all dependency caching
- **Benefits**: More control, better debugging, consistent with other repos
- **Priority**: HIGH - Part of overall caching strategy

#### **TODO-ARCH-003**: Simplify Client Workflows

- **Status**: NOT STARTED
- **Current**: Client workflows duplicate configuration
- **Goal**: Client workflows should only pass minimal options to reusable
  workflows
- **Example**: Just specify project type, paths, and special requirements
- **Priority**: MEDIUM - Maintainability improvement

## 🔥 CRITICAL CI/CD FIXES - December 18, 2025

### Frontend Build Errors and Warnings (11 Issues)

#### TypeScript/ESLint Errors (6 items)

- [x] **TODO-001**: Fix @ts-ignore in web/src/test/setup.ts:77
  - **COMPLETED**: Replaced with @ts-expect-error with explanation
- [x] **TODO-002**: Fix @ts-ignore in web/src/pages/FileManager.tsx:202
  - **COMPLETED**: Replaced with @ts-expect-error with explanation

- [x] **TODO-003**: Remove unused variable in web/src/pages/FileManager.tsx:134
  - **COMPLETED**: Removed unused \_path parameter

- [x] **TODO-004**: Remove unused variable in
      web/src/components/system/LogsTab.tsx:46
  - **COMPLETED**: Removed unused setLoading state setter

- [x] **TODO-005**: Remove unused import in
      web/src/components/common/ServerFileBrowser.tsx:5
  - **COMPLETED**: useCallback is now used (wrapped fetchDirectory)

- [x] **TODO-006**: Fix 'any' type in web/src/test/setup.ts:40
  - **COMPLETED**: Replaced with `as unknown as IntersectionObserver`

#### TypeScript 'any' Type Errors (2 items)

- [x] **TODO-007**: Fix 'any' type in web/src/pages/Settings.tsx:460
  - **COMPLETED**: Removed both 'as any' casts from credentials mapping

- [x] **TODO-008**: Fix 'any' type in web/src/pages/Settings.tsx:457
  - **COMPLETED**: Removed both 'as any' casts from credentials mapping

#### React Hooks Warnings (3 items)

- [x] **TODO-009**: Fix React Hook dependency in
      web/src/components/system/LogsTab.tsx:81
  - **COMPLETED**: Removed sourceFilter from dependency array (not used in
    fetchLogs)

- [x] **TODO-010**: Fix React Hook dependency in
      web/src/components/common/ServerFileBrowser.tsx:81
  - **COMPLETED**: Wrapped fetchDirectory in useCallback and added to
    dependencies

- [x] **TODO-011**: Verify all ESLint rules are passing
  - **File**: `web/`
  - **Issue**: Run `npm run lint` to verify all issues are resolved
  - **Fix**: Execute lint command and ensure clean output
  - **Priority**: High
  - **Category**: Validation
  - **Status**: COMPLETED - All ESLint rules passing with zero errors
  - **COMPLETED**: Migrated to ESLint 9 flat config, all linting passes

### CI Workflow Configuration (1 item)

- [x] **TODO-017**: Update CI workflow to use Go 1.25
  - **COMPLETED**: Updated ci.yml go-version from "1.23" to "1.25"

### Docker Build Issues (2 items)

- [x] **TODO-014**: Create missing Dockerfile
  - **COMPLETED**: Created multi-stage Dockerfile with Go 1.25-alpine

- [x] **TODO-015**: Update Dockerfile Go version to 1.25
  - **COMPLETED**: Created with golang:1.25-alpine base image

### Node.js/NPM Build Issues (2 items)

- [x] **TODO-012**: Fix npm cache path resolution error
  - **File**: `.github/workflows/ci.yml` or reusable workflow
  - **Issue**: "Some specified paths were not resolved, unable to cache
    dependencies"
  - **Root Cause**: cache-dependency-path may need to be `web/package-lock.json`
  - **Fix**: This is in the reusable workflow - need to verify
    cache-dependency-path configuration
  - **Priority**: High
  - **Category**: CI/CD
  - **Status**: COMPLETED — Hardened npm cache in ghcommon reusable CI (ensured
    cache dirs exist; cache `~/.npm` and `~/.cache/npm`; added Node version to
    keys). Broadened local repo-config paths accordingly.

- [ ] **TODO-013**: Fix punycode deprecation warning
  - **File**: `web/package.json` or dependencies
  - **Issue**: "(node:2311) [DEP0040] DeprecationWarning: The `punycode` module
    is deprecated"
  - **Fix**: Update dependencies that use deprecated punycode module
  - **Priority**: Low
  - **Category**: Dependencies
  - **Status**: Non-critical, can be addressed later

### CI Pipeline Tracking (Completed)

- [ ] **TODO-018**: CI Pipeline failure tracking - Frontend blocking entire
      pipeline
  - **File**: CI/CD pipeline (dependent on TODO-001 through TODO-011)
  - **Issue**: "❌ CI Pipeline failed: Frontend CI - Error: Process completed
    with exit code 1"
  - **Root Cause**: Frontend TypeScript/ESLint errors (TODO-001 through
    TODO-011) causing build failure
  - **Context**: Pipeline shows:

    ```text
    JOB_WORKFLOW_LINT: skipped
    JOB_WORKFLOW_SCRIPTS: skipped
    JOB_GO: skipped
    JOB_PYTHON: skipped
    JOB_RUST: skipped
    JOB_FRONTEND: failure
    JOB_DOCKER: skipped
    JOB_DOCS: skipped
    ```

  - **Fix**: All frontend linting errors (TODO-001 through TODO-011) are marked
    as completed - need to verify build passes
  - **Action Items**:
    1. Run `cd web && npm run lint` locally to verify all ESLint errors are
       fixed
    2. Run `cd web && npm run build` to verify frontend builds successfully
    3. Push changes and verify CI pipeline passes
    4. Once frontend passes, other jobs should run (Go, Docker, etc.)
  - **Priority**: Critical (Blocking entire CI/CD pipeline)
  - **Category**: CI/CD
  - **Status**: COMPLETED - All frontend issues fixed, build and lint passing

- [ ] **TODO-019**: ESLint configuration migration to v9.x
  - **File**: `web/.eslintrc.json` (needs to be migrated to `eslint.config.js`)
  - **Issue**: "ESLint couldn't find an eslint.config.(js|mjs|cjs) file"
  - **Root Cause**: ESLint v9.0.0+ requires new config file format
  - **Fix**: Migrate from `.eslintrc.json` to `eslint.config.js` using migration
    guide
  - **Migration Guide**:
    [ESLint migration guide](https://eslint.org/docs/latest/use/configure/migration-guide)
  - **Priority**: High (Blocking lint command)
  - **Category**: Build/Development
  - **Status**: COMPLETED - Migrated to eslint.config.mjs with ESLint 9
  - **COMPLETED**: Created eslint.config.mjs, removed .eslintrc.json, all
    linting passing

### Cross-Repo Actionization

- [x] **TODO-ACT-001**: Create `get-frontend-config-action` repo and implement
      composite action to read `.github/repository-config.yml`
  - **Outputs**: `dir`, `node-version`, `has-frontend`
  - **Workflows**: Added `test-action.yml`, `branch-cleanup.yml` (delete head
    branch after merge), and `auto-merge.yml` (label-driven REBASE auto-merge)
  - **AI**: Added standard `.github/copilot-instructions.md`
  - **Protections**: Applied via GitHub API
    - Rebase-only merges (squash/merge disabled)
    - Auto-merge enabled; delete branches on merge
    - `main` branch: require `run-action` status check, 1 approving review,
      dismiss stale, linear history, block force pushes and deletions, enforce
      on admins
  - **Status**: COMPLETED and fully configured

- [x] **TODO-020**: TypeScript compilation errors - Missing dependencies/types
  - **File**: `web/` directory - multiple TypeScript files
  - **Issue**: TypeScript build shows "Cannot find module" errors for react,
    @mui/material, etc.
  - **Sample Errors**:
    - "Cannot find module '@testing-library/react' or its corresponding type
      declarations"
    - "Cannot find module 'react' or its corresponding type declarations"
    - "Cannot find module '@mui/material' or its corresponding type
      declarations"
  - **Root Cause**: Either:
    1. Dependencies not installed (`npm install` needed)
    2. Type definitions missing
    3. tsconfig.json misconfiguration
  - **Fix**:
    1. Run `npm install` in web/ directory to ensure all dependencies are
       installed
    2. Verify package.json has all required @types/\* packages
    3. Check tsconfig.json paths and module resolution
  - **Priority**: Critical (Blocking build)
  - **Category**: Build/Dependencies
  - **Status**: COMPLETED - Fixed type narrowing and IntersectionObserver mock,
    build passing

### Python Script Issues (1 item)

- [ ] **TODO-016**: Fix generate_release_summary.py error handling
  - **File**: `.github/workflows/scripts/generate_release_summary.py`
  - **Issue**: Script exits with code 1 without creating summary report
  - **Fix**: Add error handling and ensure report is generated even on partial
    failures
  - **Output**: Should create summary in GitHub Actions summary or PR comment
  - **Priority**: High
  - **Category**: CI/CD
  - **Status**: Requires script analysis

---

## 🚨 CURRENT SPRINT - December 6, 2025

### ✅ Completed Dec 6, 2025

- [x] **Web UI book display FIXED** - Issue was hasImportPaths check blocking
      all books + API returning `items` not `audiobooks`
- [x] **EventSource reconnection FIXED** - Added exponential backoff
      (3s→6s→12s→24s, cap at 30s) and onopen handler to reset attempts
- [x] **Health endpoint FIXED** - Both `/api/health` and `/api/v1/health` now
      available (was already fixed)
- [x] **API response field fix** - Fixed frontend API client to use `items`
      instead of `audiobooks` field

### ✅ Completed Nov 21-22

- [x] **Metadata extraction fixes** - Fixed case-sensitive tags, release-group
      filtering, volume detection, series extraction, narrator/publisher fields
- [x] **Diagnostics CLI** - Created `diagnostics` command with `cleanup-invalid`
      and `query` subcommands
- [x] **Database cleanup** - Purged 8 corrupted records with placeholder values
- [x] **Full rescan verification** - Confirmed 4 books with correct metadata
      after cleanup
- [x] **Scan progress implementation** - Added pre-scan file count and separate
      library/import statistics (needs testing)

### 🔥 High Priority (Next Session)

- [ ] **Test scan progress reporting** - Trigger scan, verify progress shows
      actual file counts

### 📊 Medium Priority (Data Quality)

- [ ] **Separate dashboard counts** - Display library vs import book counts
      separately
- [ ] **Fix import path negative sizes** - Debug `total_size` calculation
      returning negative values

### 🧪 Low Priority (Optional)

- [ ] **Verify duplicate detection** - Test hash-based duplicate detection
      implemented in v1.9.0
- [ ] **Test AI parsing** - Verify OpenAI integration if needed (may not be
      necessary after metadata fixes)

---

## Legacy TODO Items

- [x] ✅ **Backend**: Database migration system with version tracking
- [x] ✅ **Backend**: Complete audiobook CRUD API (create, read, update, delete,
      batch)
- [x] ✅ **Backend**: Authors and series API endpoints
- [x] ✅ **Backend**: Library folder management API
- [x] ✅ **Backend**: Operation tracking and logs API
- [x] ✅ **Backend**: HTTP server with configurable timeouts
- [x] ✅ **Backend**: Safe file operations wrapper (copy-first, checksums,
      rollback)
- [x] ✅ **Backend**: File system browsing API (.jabexclude, stats, permissions)
- [x] ✅ **Backend**: Async operation queue system with priority handling
- [x] ✅ **Backend**: WebSocket/SSE for real-time operation updates
- [x] ✅ **Backend**: Database backup and restore functionality
- [x] ✅ **Backend**: Enhanced metadata API (batch updates, history, validation)

- [x] ✅ **Frontend**: React app setup with TypeScript and Material-UI
- [x] ✅ **Frontend**: Dashboard with library statistics and navigation
- [x] ✅ **Frontend**: Library page with import path management
- [x] ✅ **Frontend**: System page with Logs, Storage, Quotas, System Info tabs
- [x] ✅ **Frontend**: Settings page with comprehensive configuration options
- [x] ✅ **Frontend**: Smart folder/file naming patterns with live examples

- [x] ✅ **Backend**: Media info extraction from audio files
  - ✅ Created mediainfo package with Extract() function
  - ✅ Supports MP3, M4A/M4B, FLAC, OGG formats
  - ✅ Extracts bitrate, codec, sample rate, channels, bit depth
  - ✅ Quality string generation and tier comparison
- [x] ✅ **Backend**: Version management API (link versions, set primary, manage
      version groups)
  - ✅ Added GetBooksByVersionGroup() to Store interface and both
    implementations
  - ✅ Implemented 4 API endpoints: list versions, link versions, set primary,
    get version group
  - ✅ Uses ULID-based version group IDs for grouping multiple versions
  - ✅ All handlers properly use database.GlobalStore
- [x] ✅ **Backend**: Import paths CRUD API (list, add, remove, scan)
  - ✅ GET /api/v1/library/folders - List all library folders/import paths
  - ✅ POST /api/v1/library/folders - Add new import path
  - ✅ DELETE /api/v1/library/folders/:id - Remove import path
  - ✅ POST /api/v1/operations/scan - Trigger scan (optionally for specific
    folder)
- [x] ✅ **Backend**: System info API (storage, quotas, system stats)
  - ✅ GET /api/v1/system/status - Comprehensive system status (library stats,
    memory, runtime, operations)
  - ✅ Includes book count, folder count, total storage size
  - ✅ Memory statistics (alloc, total_alloc, sys, num_gc)
  - ✅ Runtime information (Go version, goroutines, CPU count)
- [x] ✅ **Backend**: Logs API with filtering (level, source, search,
      pagination)
  - ✅ GET /api/v1/system/logs - System-wide logs with filtering
  - ✅ Supports filtering by level (info, warn, error)
  - ✅ Full-text search in messages and details
  - ✅ Pagination with limit/offset parameters
  - ✅ Aggregates logs from all recent operations
- [x] ✅ **Backend**: Settings API (save/load configuration)
  - ✅ GET /api/v1/config - Get current configuration
  - ✅ PUT /api/v1/config - Update configuration at runtime
  - ✅ Supports updating root_dir, database_path, playlist_dir, API keys

## New Requirements - November 21, 2025

- [ ] **Scanner & Hash Tracking**: Persist both the original import hash and the
      post-organization hash for every book so that when a library copy is
      removed we can detect the import copy (matching original hash), recopy it,
      and compare the new hash to detect drift.
- [ ] **Book Detail Page & Delete Flow**: Confirm each book has a dedicated
      detail view showing files, metadata, and all versions; enhance the delete
      dialog with a "Prevent Reimporting of this file" checkbox that records the
      hash in a do-not-import list.
- [ ] **Quantity / State Lifecycle**: Add a per-book quantity/reference counter
      or state machine (wanted/imported/organized) plus soft-delete flags, a
      background purge job, and a do-not-import hash list that survives deletes
      so the UI can hide removed entries while preventing future reimports.
  - ✅ Manual + automatic purge flows implemented (API endpoint, UI review list,
    background job with retention and optional file removal)
- [ ] **Settings Tab for Banned Hashes**: Add a new tab on the Settings page to
      view/remove entries in the do-not-import hash list so users can unblock
      imports later.
- [ ] **Containerized E2E Suite**: Ensure the Docker test image can execute the
      Selenium/pytest E2E suite end-to-end, expand the failing tests, and add a
      VS Code task to run them inside the container for consistent automation.

## 🚨 CRITICAL FIXES COMPLETED - 2024-11-20

### ✅ Bug Fixes and UX Improvements

1. **Library Page Path Display** (v1.13.0)
   - Enhanced "Path: Not configured" message with helpful text and warning color
   - Added "Please set library path in Settings" guidance

2. **Folder Browser UX** (v1.2.0)
   - Removed two-step selection process (Select This Folder + bottom button)
   - Current browsed path now automatically selected for parent component
   - Simplified user experience for folder selection

3. **Organize Operation** (v1.18.0)
   - Fixed "organizing 0/0" issue by filtering books before operation
   - Now only organizes books NOT already in root directory
   - Skips books whose files don't exist
   - Added log message showing count of books needing organization

4. **Auto-Rescan After Organize** (v1.19.0)
   - Automatically triggers library rescan after successful organize
   - Rescan runs with low priority to avoid blocking other operations
   - Picks up newly organized books and extracts metadata

5. **AI Metadata Parsing Integration** (v1.8.0)
   - Integrated OpenAI parser into scanner workflow
   - When tag extraction fails or is incomplete, AI parser attempts to extract:
     - Title, Author, Series, Narrator, Publisher from filename
   - Requires EnableAIParsing=true and OpenAIAPIKey configured
   - Falls back to filepath extraction if AI parsing fails

6. **Dashboard Import Folders Count** (v1.3.0)
   - Fixed "Import Folders: 0" display issue
   - Changed from folders.length to systemStatus.library.folder_count
   - Now uses consistent data source with backend metrics

7. **Re-fetch Metadata UI** (v1.3.0 AudiobookCard, v1.3.0 AudiobookGrid, v1.14.0
   Library)
   - Added "Parse with AI" menu item to audiobook cards
   - Wired up handleParseWithAI handler
   - Uses existing backend endpoint: POST /api/v1/audiobooks/:id/parse-with-ai
   - Allows re-parsing books after enabling OpenAI integration

8. **Security Fix**
   - Added .encryption_key to .gitignore
   - Prevents accidental commit of encryption secrets

- [x] ✅ **Backend**: Settings API safety restrictions
  - ✅ Safety restrictions on database_type and enable_sqlite (read-only at
    runtime)

- [x] **Backend - Database migration for media info and version fields**
  - ✅ Created migration005 adding all 9 fields to books table
  - ✅ Handles duplicate column detection gracefully
  - ✅ Creates indices for version_group_id and is_primary_version
- [x] ✅ **Backend**: Manual file import handling
  - ✅ POST /api/v1/import/file - Import single audio file with metadata
    extraction
  - ✅ File validation (existence, extension support)
  - ✅ Automatic metadata extraction (title, author, narrator, etc.)
  - ✅ Media info extraction (bitrate, codec, quality)
  - ✅ Author auto-creation if not exists
  - ✅ Optional organize flag to trigger file organization
- [x] ✅ **Backend**: Metadata source integration (Open Library)
  - ✅ Created OpenLibraryClient with SearchByTitle, SearchByTitleAndAuthor,
    GetBookByISBN methods
  - ✅ Returns title, author, description, publisher, publish_year, ISBN,
    cover_url, language
  - ✅ API endpoints: GET /api/v1/metadata/search, POST
    /api/v1/audiobooks/:id/fetch-metadata
  - ✅ 8 comprehensive test cases created (client init, search operations, error
    handling)

- [x] ✅ **Frontend**: Connect all pages to backend APIs
  - ✅ Created comprehensive API service layer (src/services/api.ts) with 30+
    typed endpoints
  - ✅ Dashboard: Real statistics from /api/v1/audiobooks, /api/v1/authors,
    /api/v1/series, /api/v1/system/status
  - ✅ Library page: Real audiobook listing, search, import path management,
    scan operations
  - ✅ System page: Real logs with filtering, system status, memory/CPU stats,
    SystemInfoTab displays real-time data
  - ✅ Settings page: Loads configuration on mount with api.getConfig(), saves
    with api.updateConfig()
  - ✅ All API endpoints integrated with proper error handling
  - ✅ Backend Config struct expanded to support all frontend settings
    (organization, quotas, metadata, performance, memory, logging)
- [x] ✅ **Frontend**: Version management UI components
  - ✅ VersionManagement dialog component with version comparison view
  - ✅ Quality indicators (codec, bitrate, sample rate display)
  - ✅ Primary version selection with star icon
  - ✅ Link version dialog for connecting multiple editions
  - ✅ Version indicator chips on audiobook cards
  - ✅ Integrated into Library page grid view
  - ✅ Uses all version management API endpoints (getBookVersions,
    linkBookVersion, setPrimaryVersion)
- [x] ✅ **Frontend**: Library browser with grid/list views and sorting
  - ✅ Grid view fully functional with AudiobookCard and AudiobookGrid
    components
  - ✅ Sorting dropdown with options: title, author, date added, date modified
  - ✅ Client-side sort implementation in Library.tsx with localeCompare for
    strings
  - ✅ Date sorting (descending - newest first) for created_at and updated_at
    fields
- [x] ✅ **Frontend**: Metadata editor with inline editing
  - ✅ MetadataEditDialog component with comprehensive edit form
  - ✅ InlineEditField component created for quick inline edits
  - ✅ "Fetch Metadata" button with CloudDownload icon in AudiobookCard menu
  - ✅ Full integration in Library.tsx with handleFetchMetadata function

- [x] ✅ **General**: Configure GitHub workflows
  - ✅ Comprehensive CI workflow v1.18.1 already exists
  - ✅ Backend tests: Go 1.24, test execution, race detection, coverage
  - ✅ Frontend tests: Node 22, npm ci, build, test
  - ✅ Security scanning: gosec, npm audit, Trivy
  - ✅ Python script validation: Python 3.13, pip, script checks
- [x] ✅ **Testing**: Unit and integration test framework
  - ✅ Created internal/metadata/openlibrary_test.go (8 test cases)
  - ✅ Created internal/database/sqlite_test.go (11 test cases)
  - ✅ Tests cover client initialization, search operations, CRUD, version
    management, author operations
  - ✅ Uses setupTestDB pattern with temporary database and cleanup
  - ✅ Network tests use t.Skip for rate limits
- [x] ✅ **Docs**: OpenAPI/Swagger documentation
  - ✅ Created docs/openapi.yaml with complete OpenAPI 3.0.3 specification
  - ✅ Documented 20+ endpoints across 9 tags (Audiobooks, Authors, Series,
    Library, Operations, Metadata, Versions, System, Backup)
  - ✅ Full schema definitions for Book (25+ fields), Author, Series,
    LibraryFolder, MetadataResult, SystemStatus, Config
  - ✅ Request/response examples with proper types, error codes, ULID format
    specifications

- [ ] 🟡 **General**: Implement library organization with hard links, reflinks,
      or copies (auto mode tries reflink → hardlink → copy)

## 🚨 CRITICAL FIXES - HIGH PRIORITY

### System Page Issues

- [x] **Fix memory display** - Changed label to "App Memory System" to clarify
      that displayed memory is Go runtime memory, not system RAM. ✅ COMPLETED
- [ ] **Fix logs not displaying** - Logs tab shows no data because no operations
      have been run yet. Logs are fetched from operation records. Will populate
      after running scan operations.

### Settings Page Issues

- [x] **Fix scrolling in Settings page** - Removed maxHeight constraint from
      Paper component, proper Box structure for scrollable tabs. ✅ COMPLETED
- [x] **Fix library path browser** - ServerFileBrowser properly initialized and
      working. ✅ COMPLETED

### ServerFileBrowser Component Issues

- [x] **Make current path sticky** - Added position: sticky, top: 0, zIndex: 10
      to path bar. ✅ COMPLETED
- [x] **Fix Add Folder button always disabled** - Added "Select This Folder"
      button for immediate selection without double-click. Single click
      navigates, button selects. ✅ COMPLETED
- [x] **Add manual path editing** - Added edit icon that enables TextField for
      direct path editing with save/cancel functionality. ✅ COMPLETED

### Import Path Functionality Issues

- [x] **Fix folder scanning doesn't traverse subdirectories** - Implemented real
      scanner.ScanDirectory() call in startScan handler. Uses filepath.Walk for
      recursive traversal. Updates book_count in LibraryFolder records. ✅
      COMPLETED
- [x] **Auto-scan on import path add** - Modified addLibraryFolder handler to
      automatically trigger async scan operation. Returns scan_operation_id for
      progress tracking. ✅ COMPLETED
- [x] **Fix import path terminology** - Updated all UI: Dashboard, Settings,
      Library pages now consistently use "Import Folders (Watch Locations)" vs
      "Library Path". ✅ COMPLETED

### Dashboard Navigation Issues

- [x] **Fix Library Folders link** - Fixed navigation from /file-manager to
      /library. Card title updated to "Import Folders". ✅ COMPLETED

## Current Sprint Tasks

### Frontend UI Improvements

- [ ] **Fix System Page Fake Data**
  - [ ] Wire StorageTab.tsx to real API data from /api/v1/system/status and
        /api/v1/library/folders
  - [ ] Wire QuotaTab.tsx to real API data or remove if quotas not implemented
  - [ ] Wire LogsTab.tsx to /api/v1/system/logs endpoint to show actual
        application logs
  - [ ] Wire SystemInfoTab.tsx to /api/v1/system/status to show real OS (linux),
        memory stats, CPU count, Go version

### Library & Import Management

- [ ] **Add Library Path Configuration**
  - [ ] Add central library path setting in Settings page (where organized
        audiobooks go)
  - [ ] Add UI in Settings to manage download/import folders with server
        filesystem browser
  - [ ] Add UI in Library tab to add/remove download folders with server
        filesystem browser

- [ ] **Server Filesystem Browser**
  - [ ] Create reusable ServerFileBrowser component using
        /api/v1/filesystem/browse
  - [ ] Update Library page import workflow - replace local file upload with
        server browser
  - [ ] Allow selecting files and folders from remote server filesystem

### First-Run Experience

- [ ] **Welcome Wizard**
  - [ ] Create WelcomeWizard component that runs on first launch
  - [ ] Step 1: Set library folder path (where organized books go)
  - [ ] Step 2: Optional OpenAI API key setup with connection test
  - [ ] Step 3: Add import/download folder paths using server browser
  - [ ] Store completion flag in config/database to skip wizard on subsequent
        launches

### Testing

- [ ] Create database_test.go - test initialization, configuration, database
      type selection
- [ ] Create migrations_test.go - test schema versioning, migration execution,
      rollback
- [ ] Create store_test.go - test interface methods and common store
      functionality
- [ ] Create web_test.go - test HTTP handlers and API endpoints

## Future Improvements

### Multi-User & Security

- [ ] **Multi-User Interface**
  - [ ] User profiles and authentication system
  - [ ] Per-user playback tracking and statistics
  - [ ] User-specific library views and permissions
  - [ ] Role-based access control (admin, user, read-only)

- [ ] **SSL/TLS Support**
  - [ ] HTTPS support with certificate management
  - [ ] Let's Encrypt integration for automatic certificates
  - [ ] Self-signed certificate generation for local deployments
  - [ ] Configurable cipher suites and TLS versions

### BitTorrent Client Integration

- [ ] **Torrent Client Interoperability**
  - [ ] uTorrent/µTorrent API integration
  - [ ] Deluge RPC integration
  - [ ] qBittorrent Web API integration
  - [ ] Automatic torrent removal after successful library organization
  - [ ] Support for preserving seeding after organization:
    - [ ] Create shadow/mirror directory structure outside main library (e.g.,
          `/audiobooks-seeding/`)
    - [ ] Maintain hard links in shadow directory matching original torrent
          structure
    - [ ] Update torrent client to serve from shadow directory after files are
          organized
    - [ ] Handle cross-filesystem scenarios (copy to shadow dir when hard links
          impossible)
    - [ ] Detect and handle metadata updates that modify organized files (break
          hard links)
    - [ ] Optional: Re-link shadow files if organized files haven't been
          modified
  - [ ] Configurable removal policies (remove after move, keep seeding, etc.)

### iTunes Library Integration

- [ ] **iTunes Interoperability**
  - [ ] Read iTunes library XML/database for playback statistics
  - [ ] Import play count, last played date, and user ratings from iTunes
  - [ ] Sync playback progress and bookmarks between systems
  - [ ] Write metadata updates back to iTunes library
  - [ ] Bidirectional sync for play counts and ratings
  - [ ] Support for multiple iTunes libraries (multi-user scenarios)

### Web Download & Export

- [ ] **Direct Book Download from Web Interface**
  - [ ] Download individual audiobook files via web UI
  - [ ] Automatic ZIP creation for multi-file books
  - [ ] Progress indicators for ZIP creation and download
  - [ ] Configurable download formats (original files, ZIP, M4B)
  - [ ] Batch download support for multiple books
  - [ ] Resume support for interrupted downloads

### Audio Transcoding & Optimization

- [ ] **Automated Audio Transcoding**
  - [ ] MP3 to M4B conversion for multi-file books
  - [ ] Chapter metadata preservation during transcoding
  - [ ] Automatic chapter detection from file names/structure
  - [ ] Cover art embedding in M4B files
  - [ ] Configurable quality settings (bitrate, codec, sample rate)
  - [ ] Batch transcoding operations with priority queue
  - [ ] Original file preservation options (keep, replace, archive)
  - [ ] Integration with book download (serve M4B instead of ZIP for transcoded
        books)

## Recently Added Observability Tasks

- [ ] Persist operation logs (retain historical tail per operation; add
      `/api/v1/operations/:id/logs?tail=` and system-wide retention)
- [ ] Improve log view UX (auto-scroll when following tail, level-based
      coloring, collapsible verbose details, memory usage guard)
- [ ] SSE system status heartbeats (push `system.status` diff events every 5s
      for live memory / library metrics without polling)

## 🔥 CRITICAL - IN PROGRESS (Nov 21, 2025)

### Metadata Extraction Completely Broken

- [x] **URGENT**: Fix metadata extraction empty/Unknown values
  - [x] Honor AlbumArtist/Composer/Artist precedence so author and narrator map
        correctly
  - [x] Add fixture-based tests for author/narrator precedence and performer
        tags
  - [ ] Validate against real-world M4B sample:
        `/Users/jdfalk/Downloads/test_books/[PZG] My Quiet Blacksmith Life.../... [PZG].m4b`

### AI Parsing Not Working

- [x] **URGENT**: Fix OpenAI integration in scanner workflow
  - [x] Track when filename fallback was used and allow AI to re-parse fallback
        metadata
  - [x] Add AI fallback logs that explain why parsing was invoked
  - [x] Add tests for fallback flag and TXXX narrator/performer extraction
  - [ ] Validate with a real OpenAI key in Settings + scanner run

### Volume Number Extraction Missing

- [x] **HIGH**: Add volume/book number detection
  - [x] Added regex patterns for Vol/Volume/Book/Bk/Part/# detection
  - [x] Added tests for Arabic numeral volume patterns
  - [x] Apply to filename parsing and album tag parsing (already in metadata)

### Event Transport Regression (Nov 21, 2025)

- [x] Fix SSE lifetime in `internal/server.handleEvents` so `/api/events`
      streams remain open (remove premature context timeouts, keep heartbeats
      flowing)
- [x] Add client-side EventSource manager with exponential backoff (3s → 6s →
      12s, cap at 60s) and shared connection for Dashboard + Library
- [ ] Replace `/api/v1/health` polling with existing `/api/health` endpoint or
      add a v1 alias so reconnect overlay stops 404 spam
- [ ] When health probe succeeds after outage, auto-refresh UI to clear stuck
      "Attempt N" overlay and rehydrate state
- [ ] Log reconnection attempts + last error reason in UI for easier diagnosis

### Template Variables in Organized Paths

- [x] **HIGH**: Fix organizer writing literal `{series}` and `{narrator}`
  - [x] Normalize placeholder casing before expansion so `{Series}` resolves
  - [x] Apply defaults for missing narrator values to avoid empty placeholders
  - [x] Validate expanded patterns to prevent unresolved placeholders
  - [ ] Fix existing corrupted paths: `library/Unknown Author/{series}/...`

### Duplicate Detection Needs Testing

- [ ] **MEDIUM**: Test hash-based duplicate detection (added v1.9.0 but
      untested)
  - [ ] Delete existing duplicate records with `cleanup_invalid_books.go`
  - [ ] Run Full Rescan with duplicate detection enabled
  - [ ] Verify 4 unique books created (not 8) when same files in library +
        import path
  - [ ] Check logs for "Found duplicate book by hash" and "Skipping duplicate"
        messages

## Extended Improvement Backlog

### Observability & Monitoring

- [ ] Structured application metrics endpoint (Prometheus `/metrics`, operation
      duration histograms, scan/organize counters)
- [ ] Per-operation timing summary stored after completion (wall time, file
      count, throughput)
- [ ] Slow operation detector (warn if scan > configurable threshold)
- [ ] Library growth trend stats (daily book count snapshot table)
- [ ] File integrity checker (periodic checksum verification with mismatch
      surfacing)
- [ ] Background health check SSE pings (report DB latency classification)
- [ ] Error aggregation dashboard (top recurring errors with counts)

### Performance

- [ ] Parallel scanning (goroutine pool respecting `concurrent_scans` setting)
- [ ] Debounced library size recomputation using inotify / fsnotify events
      instead of periodic full walk
- [ ] Caching layer for frequent book queries (LRU keyed by filter + page)
- [ ] Batch metadata fetch pipeline (queue & coalesce external API calls)
- [ ] Adaptive operation worker scaling (increase workers under backlog, shrink
      when idle)
- [ ] Memory pressure monitor triggering GC hints / cache trimming

### Reliability & Resilience

- [ ] Graceful resume of interrupted scan (persist walker state checkpoints)
- [ ] Operation retry policy for transient failures (network metadata retrieval)
- [ ] Circuit breaker for external metadata sources (avoid cascading failures)
- [ ] Transactional organize rollback journal (record actions, allow revert)
- [ ] Startup self-diagnostic (verify paths writable, database schema current,
      config sanity)

### UX / Frontend

- [ ] Global notification/toast system for successes & errors
- [ ] Dark mode / theme customization with persisted preference
- [ ] Keyboard shortcuts (e.g. '/' focus search, 'o' organize, 's' scan all)
- [ ] Advanced filters (bitrate range, codec, quality tier, duration bucket)
- [ ] Progressive loading skeletons for long lists
- [ ] Inline author/series quick create dialog from edit form
- [ ] Book detail modal with expanded metadata & version timeline
- [ ] Accessible tab navigation (ARIA roles, focus management)
- [ ] Mobile responsive layout improvements (grid collapse, drawer nav)
- [ ] Virtualized audiobook list for large collections

### API Enhancements

- [ ] PATCH support for partial audiobook updates
- [ ] Bulk import endpoint for multiple file paths in one request
- [ ] Webhook system for external integrations (scan complete, organize
      complete)
- [ ] Rate limiting (token bucket) for expensive endpoints
- [ ] ETag / caching headers for read-only endpoints
- [ ] API key auth layer (for third-party consumers)

### Security

- [ ] Audit log (who changed config, when, old vs new values)
- [ ] Optional JWT auth for multi-user future
- [ ] Secret scanning in config updates (reject accidental API key leakage)
- [ ] Harden path traversal defenses in filesystem browse
- [ ] TLS termination guide / built-in ACME client

### Database & Data Quality

- [ ] Deduplication job (identify same book with different filenames via fuzzy
      match)
- [ ] Orphan file detector (files on disk not represented in DB)
- [ ] Full-text search index (author/title/narrator) for advanced queries
- [ ] Incremental migration harness with dry-run mode
- [ ] Archival strategy (move old logs & completed operations to cold storage)

### Operation Queue Improvements

- [ ] Priority aging (long-waiting normal ops get temporary priority boost)
- [ ] Operation dependency graph (organize waits for scan completion for same
      folder)
- [ ] Pause / resume queue functionality
- [ ] Real-time worker utilization stats
- [ ] Rate-controlled progress events (coalesce rapid updates)

### Real-Time & Streaming

- [ ] Upgrade SSE hub to optional WebSocket mode for bidirectional
      cancel/resubscribe
- [ ] Client subscription refinement (subscribe to multiple ops, filter types)
- [ ] Replay last N events on connect for quick hydration

### Frontend Components (New)

- [ ] Timeline visualization for operations
- [ ] Quality comparison chart between versions
- [ ] Folder tree viewer for import paths with status badges
- [ ] Log tail component standalone (filter by level, search live)

### Testing & QA

- [ ] Load test scenarios (large folder scan, 10k files)
- [ ] Fuzz tests for filename parser / AI parse fallback
- [ ] Frontend component snapshot tests
- [ ] End-to-end test harness (Playwright or Cypress) for critical flows
- [ ] Playwright UI coverage (current: minimal smoke + import-path mock)
  - Current specs: `web/tests/e2e/app.spec.ts` (smoke nav with mocked API/SSE),
    `web/tests/e2e/import-paths.spec.ts` (Settings add/remove import path via
    route mocks). Config: `web/tests/e2e/playwright.config.ts`, run with
    `cd web && npm run test:e2e`.
  - Needed coverage: Library list interactions (search/sort/view toggle,
    pagination), navigation into Book Detail, Book Detail tabs/actions (soft
    delete/block, restore, purge, metadata fetch, AI parse, version manager
    button, hash copy toast), soft-deleted list restore/purge, version linking
    dialog happy-path, Settings retention toggles (purge settings), dashboard
    tiles render, import paths end-to-end (add/remove/update via UI, not just
    mocked route), file manager browse dialogs, operation status banners.
  - Add stable API fixtures or route mocks per page; ensure wizard is bypassed,
    SSE mocked; use headless dev server via existing Playwright config; keep
    tests idempotent and non-networked.
- [ ] Book Detail metadata richness
  - Add Tags tab showing raw embedded/file tags and media info
    (bitrate/codec/sample
    rate/channels/publisher/narrator/year/album/series/title) from backend;
    read-only.
  - Show provenance on fields (DB/edited, fetched, file tag) when API can supply
    per-field source flags; fall back gracefully if not available.
  - Add Compare view (File tags vs Stored/Fetched vs Current overrides) when API
    returns multiple metadata sources.
  - Expand Edit Metadata dialog to include full fields
    (author/series/year/genre/ISBN/description/publisher/language/etc.) and save
    via API.
  - Ensure hashes and media details remain visible in Files tab; consider
    duration/size display if API provides.
- [ ] Backend support for Book Detail tags/provenance
  - [x] Add API endpoint to return raw embedded tags + media info +
        source/provenance per field (e.g., `/api/v1/audiobooks/:id/tags`), with
        payload including file tags, stored values, fetched metadata, and
        “locked/override” flags.
  - [x] Extend `GET /api/v1/audiobooks/:id` response to optionally include
        provenance map (field -> {source: file/db/fetched/override, value,
        last_updated}).
  - [x] Add override/lock semantics: when a user edits a field, mark it as
        “locked/override” so later fetches/AI/tag refresh won’t overwrite unless
        explicitly cleared; include a way to clear lock.
  - [ ] Provide metadata history or last-applied source so UI can show conflicts
        (e.g., file vs fetched vs override).
  - **Progress**: Metadata provenance now persists in the `metadata_states`
    table; tags endpoint returns effective value/source plus
    fetched/override/locked fields; update handler persists overrides/fetched
    metadata; Go handler tests added.
- [x] Backend gaps discovered (Book Detail tags/compare)
  - Prior behavior: `GET /audiobooks/:id/tags` returned only file/stored values;
    no fetched/override provenance, no lock persistence, and author/series names
    not resolved (only IDs exist in DB). Status: metadata state moved off
    user-preferences into durable table; tags handler now returns
    effective_source/value with resolved names.
  - Update `UpdateBook`/DB to accept override payloads and persist
    override_locked flags.
  - Add handler/unit tests for the tags endpoint covering file vs fetched vs
    override vs locked cases.
- [ ] Frontend Book Detail (Tags/Compare/Overrides)
  - Tags tab: show raw tags from new endpoint; include media info and tag values
    (title/author/narrator/series/position/publisher/language/year/genre/ISBN/comments).
    Progress: Tags/Compare now display effective source/value chips and lock
    badges.
  - Compare view: side-by-side (File tags vs Stored vs Fetched vs Override) with
    clear indication of locked fields; allow “use file value” / “use fetched
    value” actions per field when backend supports.
  - Edit dialog: include all key fields and allow setting/clearing overrides;
    send override flag with updates.
- [ ] Playwright coverage for Book Detail (with mocks)
  - Add fixture/mocks for tags/provenance endpoint (normal case, conflict case
    with override vs file/fetched). Progress: Book-detail mocks now include
    effective source/value and recompute on overrides.
  - Tests: render Tags tab with raw tags; render Compare view showing differing
    sources; override workflow (edit field, mark locked, verify UI shows
    override badge and Compare tab shows resolution); clear override resets to
    tag/fetched value; hash copy toast; delete dialog options (soft/hard, block
    hash); Manage Versions dialog opens.
  - Keep tests fully mocked (no backend dependency), bypass wizard, mock SSE,
    run under Chromium + WebKit.
- [ ] Chaos test for operation cancellation mid-scan

### DevOps / CI/CD

- [ ] Automated release notes generation from conventional commits
- [ ] Build artifact publishing (binary + Docker image)
- [ ] Nightly vulnerability scan & report
- [ ] Performance regression benchmarks (scan speed comparison per commit)

### Documentation

- [ ] Developer guide (architecture overview, data flow diagrams)
- [ ] Operations handbook (recover from failed organize, manual rollback)
- [ ] REST API quickstart examples (curl / client code snippets)
- [ ] Advanced configuration examples (quota strategies, memory tuning)

### Integration / Ecosystem

- [ ] Calibre metadata export integration
- [ ] OPDS feed generation for external audiobook apps
- [ ] Plex / Jellyfin library sync stub
- [ ] External cover art provider fallback chain

### AI & Metadata Enhancements

- [ ] Confidence explanation tooltips for AI parsing results
- [ ] Batch AI parse queue for newly imported unparsed files
- [ ] Metadata merge policy editor (prefer source A unless missing field)
- [ ] Automatic language detection from text samples

### Internationalization (i18n)

- [ ] Extract UI strings into translation files
- [ ] Language switcher in settings
- [ ] Date/time localization and number formatting

### Accessibility (a11y)

- [ ] Screen reader labels for interactive elements
- [ ] High contrast theme option
- [ ] Focus outline consistency and skip-to-content link

### Mobile / PWA

- [ ] PWA manifest & offline shell
- [ ] Add to Home Screen guidance
- [ ] Basic offline read-only browsing of cached metadata

### Packaging & Deployment

- [ ] Docker multi-arch build pipeline (linux/amd64 + arm64)
- [ ] Helm chart for Kubernetes deployment
- [ ] Binary distribution script with checksums & SBOM

### Backup & Restore Enhancements

- [ ] Incremental backups (changes since last snapshot)
- [ ] Backup integrity verification (hash manifest)
- [ ] Scheduled backup task with retention policy

### File Handling Improvements

- [ ] Concurrent organize operations with folder-level locking
- [ ] Metadata tag writing improvements (add narrator, series sequence tags)
- [ ] Chapter file merging strategy (combine small segments automatically)

### User Features (Future Multi-User)

- [ ] Per-user favorites / starred books
- [ ] Listening progress tracking (position syncing)
- [ ] Personal notes / annotations per book

### Data Analysis / Insights

- [ ] Quality upgrade suggestions (identify low bitrate books with higher
      quality versions available)
- [ ] Duplicate version ranking (present best candidate to keep)
- [ ] Usage analytics (most scanned folders, peak operation times)

### Housekeeping / Maintenance

- [ ] Stale operation cleanup job (remove abandoned queued ops after timeout)
- [ ] Automatic log rotation & compression
- [ ] Config schema validation on update (reject invalid enum values)

### Security Hardening

- [ ] Content Security Policy headers for frontend
- [ ] Rate limit brute-force attempts (future auth system)
- [ ] Dependency vulnerability auto-PR updates

### Miscellaneous Ideas

- [ ] Embedded help panel with contextual docs
- [ ] CLI progress mirroring (serve mode exposes op summary to CLI)
- [ ] Export organized library manifest (JSON + checksums)
- [ ] Plugin system scaffold (register metadata providers / transcoding
      strategies)
