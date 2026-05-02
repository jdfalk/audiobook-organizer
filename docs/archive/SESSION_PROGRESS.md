<!-- file: docs/SESSION_PROGRESS.md -->
<!-- version: 2.0.0 -->
<!-- last-edited: 2026-01-31 -->

# Session Progress — 2026-01-31

This file is the single source of truth for resuming work across sessions.
Read this first. Everything you need to pick up where we left off is here.

---

## What Was Done This Session

### Bug Fixes (all committed or staged)

- **Negative total_size bug** — `internal/itunes/plist_parser.go`: int64 overflow
  guard on `Size` field (negative values reset to 0). `internal/server/server.go`
  line 4242: added `&& *book.FileSize > 0` guard on dashboard accumulator.
- **Corrupted organize paths — migration 14** — `internal/database/migrations.go`:
  `migration014Up` fixes books with literal `{` `}` in FilePath. SQLite uses bulk
  UPDATE with LIKE; PebbleDB iterates. Affected books get `library_state =
  'needs_review'`.
- **Anthology typos** — Fixed `IsAnthlogy`/`IsAntholgy` → `IsAnthology` and
  `DetectAnthlogy` → `DetectAnthology` in the design doc.

### Release Pipeline (all staged)

- `.goreleaser.yml` — `version: 2`, `release.disable: false`, added description.
- `.github/repository-config.yml` — strategy manual, lock_files, goreleaser block,
  e2e block, coverage threshold 60.
- `.github/workflows/test-action-integration.yml` — `issues: write` permission,
  drift-alert step (creates GitHub issue on validation failure).
- `.github/workflows/scripts/release_workflow.py` — `generate_changelog()` wrapped
  in try/except; writes stub changelog on error.
- `.github/workflows/vulnerability-scan.yml` — CREATED but needs rewrite (see
  Remaining).

### ghcommon Updates

- `reusable-security.yml` CodeQL upgrade: added `github-actions` language,
  per-language build-mode, `start-proxy` step, conditional autobuild, npm ci
  working-directory fix, NPM dep audit step.

### Docs Cleanup

- Moved 42 stale .md files to `docs/archive/`.
- Moved guides (BUILD.md, BUILD_TAGS_GUIDE.md, MOCKERY_GUIDE.md,
  CODING_STANDARDS.md) from root → `docs/`.
- Root now: AGENTS.md, CHANGELOG.md, CLAUDE.md, labels.md, QUICKSTART.md,
  README.md, TESTING.md, TODO.md only.

### Design Doc

- `docs/plans/2026-01-31-anthology-handling-design.md` — Full anthology handling
  design: data model (AnthologyReview, AnthologyBookMapping, Book additions),
  detection logic (ISBN prefix, title pattern, duration threshold), review queue
  (3 views + resolution actions + API endpoints), confidence scoring, 60-day
  timeout job, migration 015, PebbleDB key schema, Store interface additions.
  **Design only — nothing implemented yet.**

---

## Remaining Work (pick up here)

### 🔴 High Priority — Next Up

1. **Rewrite vulnerability-scan.yml** — Replace the standalone govulncheck/npm
   audit version with a simple `workflow_call` to
   `ghcommon/.github/workflows/reusable-security.yml`. Inputs: `languages` +
   `secrets: inherit`. That's it.

2. **Coverage reporting in ghcommon reusable-ci.yml**:
   - Go: upload `cover.out` + HTML report as artifact after `go-test-coverage`.
   - JS/TS: add `npm run coverage` (or `vitest run --coverage`), upload LCOV.
   - PR comment: `github-script` step in CI summary job posts coverage summary
     reading from uploaded artifacts.

3. **E2E video recording**:
   - Set `video: 'retain-on-failure'` in Playwright config (`web/tests/e2e/`).
   - Videos auto-include in `playwright-report` artifact. Change retention to 7d.

4. **Parallel test DB conflicts** — CodeQL autobuild ran `go test` and some tests
   failed. Likely: parallel goroutines sharing a SQLite DB path. Check
   `internal/server/` test files for shared fixture paths. Fix: `t.TempDir()` for
   each test. Check `TestMain` / setup functions.

5. **Go test coverage increase** — Target: 25% → 60%. Run `go test ./... -cover`
   without mocks tag. Identify lowest-coverage packages. Server package (132KB, 17
   test files) is the biggest gap. Write targeted tests.

### 🟡 Medium Priority

1. **ghcommon goreleaser schema audit** — Check ghcommon's `.goreleaser.yml`
   against v2 schema. The audiobook-organizer one is fixed already.

2. **setup-node SHA pinning** — Pin `actions/setup-node@v5` to commit SHA in all
   workflows, matching what other actions already do.

### 🟢 Lower Priority (deferred)

- iTunes Integration Phases 2–4
- Metadata multi-author/narrator support
- Anthology design implementation (migration 015, detect.go, API endpoints,
  review queue UI) — see design doc for full spec
- Playwright E2E expansion
- All Post-MVP backlog (see TODO.md)

---

## Uncommitted Changes Summary

All changes are staged (not committed). 54 files changed: 145 insertions,
28184 deletions (most deletions are the archived docs). Key modified files:

- `.github/repository-config.yml`
- `.github/workflows/scripts/release_workflow.py`
- `.github/workflows/test-action-integration.yml`
- `.goreleaser.yml`
- `internal/database/migrations.go`
- `internal/itunes/plist_parser.go`
- `internal/server/server.go`
- `docs/plans/2026-01-31-anthology-handling-design.md`

New untracked files:

- `.github/workflows/vulnerability-scan.yml`
- `docs/BUILD.md`, `docs/BUILD_TAGS_GUIDE.md`, `docs/CODING_STANDARDS.md`,
  `docs/MOCKERY_GUIDE.md`, `docs/SESSION_PROGRESS.md`
- `docs/archive/` (42 files)

---

## Key File Locations

| What                 | Where                                                |
| -------------------- | ---------------------------------------------------- |
| Anthology design     | `docs/plans/2026-01-31-anthology-handling-design.md` |
| Database migrations  | `internal/database/migrations.go`                    |
| Server (4200+ lines) | `internal/server/server.go`                          |
| iTunes parser        | `internal/itunes/plist_parser.go`                    |
| CI workflows         | `.github/workflows/`                                 |
| ghcommon reusables   | Referenced via `workflow_call` (separate repo)       |
| TODO backlog         | `TODO.md`                                            |
| AI instructions      | `.github/instructions/` + `.github/prompts/`         |
