---
name: project-patterns
description: Reference guide for common audiobook-organizer tasks and recurring patterns. Helps identify which tasks have existing skills or which new skills might be useful. Use when starting a new task to see if there's a pattern match, or to discover which skills exist for database, build, deployment, debugging, or operational tasks.
---

# Project Patterns Reference

A catalog of recurring tasks and patterns in audiobook-organizer, with pointers to skills and architectural notes.

## Quick Reference

**Skills Available:**
- `server-bootstrap` — Get API key from server
- `server-logs` — Fetch logs and login credentials
- `build-deploy` — Build and deploy using Makefile

## High-Frequency Tasks

### Build & Deployment

| Task | Use This | Time |
|------|----------|------|
| Build backend only | `make build-api` | 30s |
| Build frontend + backend | `make build` | 2-3m |
| Run full app locally | `make run` | — |
| Run API-only server | `make run-api` | — |
| Frontend dev server (Vite) | `make web-dev` | — |
| Run tests (fast) | `make test-short` | 1m |
| Run all tests | `make test-all` | 20m |
| Deploy to production | `make deploy` | — |

See `build-deploy` skill for details.

### Server Access & Logs

| Task | Use This | Notes |
|------|----------|-------|
| Get API key for server | `server-bootstrap` skill | Creates .api-token file |
| Fetch service logs | `server-logs status` | Last 50 lines |
| Stream logs live | `server-logs stream` | Ctrl+C to stop |
| Get login credentials | `server-logs login` | Shows web UI URL + API key |
| SSH to server | `ssh root@172.16.2.30` | Need SSH key/password |

See `server-logs` skill for details.

## Architectural Patterns

### PebbleDB Operations

**Common patterns:**
- Versioned keys: `external_id_backfill_v3_done` (not `v1`, `v2`)
- Cached aggregates with dirty flag: `stats:library` + `stats:dirty`
- Key structure: `namespace:resource:id` (e.g., `external_id_map:12345`)

**Files to check:**
- `internal/database/pebble_store.go` — primary DB interface
- `internal/database/store.go` — abstract Store interface
- `internal/database/models.go` — data structures

### Activity Log

**Pattern:** Dual-backend (legacy SQLite + NutsDB), with type derivation + tag enrichment.

**Files to check:**
- `internal/database/activity_log.go` — activity operations
- `internal/logging/operation_logger.go` — operation logging
- `internal/logging/enrichment.go` — tag enrichment (action/source/outcome/op/book/component/scope)

**Common operations:**
- `CompactByDay()` — compact digests by date
- `RecompactDigests()` — re-run tag enrichment on existing logs
- Click-to-filter by tag in UI (`web/src/features/activity/...`)

### Batch Operations

**Pattern:** Per-item async operations with resumption via OperationResult.

**Files to check:**
- `internal/operations/batch_operation.go`
- `internal/operations/batch_processor.go`
- `internal/database/operation_result.go` — graceful shutdown + restart recovery

**Common flows:**
- `POST /api/v1/audiobooks/batch-operations` with list of items
- Each item gets `operation_id:uuid:result_type` entry in OperationResult
- Restart service → resumes in-flight operations

### Metadata & Tagging

**Pattern:** Read/extract → compare → write → backfill.

**Files to check:**
- `internal/metadata/taglib_writer.go` — writes ALL DB fields to tags
- `internal/metadata/taglib_extractor.go` — reads tags back
- `internal/metadata/enrichment.go` — ISBN / Open Library lookup
- `internal/metadata/dedup.go` — hash-based deduplication

**Custom tags:**
- Prefix: `AUDIOBOOK_ORGANIZER_*`
- Fields: ISBN, external_id, fingerprint, etc.

## Common Problem Areas

### Memory Leaks (React Components)

**Pattern:** useEffect without cleanup, useRef without isMountedRef, polling without cancelation.

**Files to check:**
- `web/src/` — all React components
- `web/src/hooks/` — custom hooks (check cleanup)
- CI: Memory-leak scanner in GitHub Actions

**Recent fix (May 20):** 41 memory leaks fixed across 25 components (PR #1076).

### Flaky Tests

**Pattern:** Property tests, concurrent database operations, timing-dependent assertions.

**Rule:** Don't rerun-and-ignore; diagnose and fix root cause.

**Files to check:**
- `internal/tests/` — test fixtures + helpers
- `internal/database/*_test.go` — database tests
- `web/tests/` — frontend test configs

### N+1 Query Problems

**Pattern:** Fetching one item calls query N times in loop. Solution: Bulk-fetch.

**Recent fix (May 12):** N+1 fixes reduced 20K queries to 3.

**How to detect:** `sqlc.Conn.Query()` inside `for _ := range items {}` loop.

## Testing Strategy

### Frontend Testing

- **Unit tests:** Vitest (web/tests/unit/)
- **E2E tests:** Playwright (web/tests/e2e/)
- **Memory leaks:** Custom CI scanner (detects useEffect, useRef patterns)

Command: `make test-frontend`

### Backend Testing

- **Unit tests:** Go `testing` package
- **Property tests:** Gopter (slow, skipped in `-short` mode)
- **Coverage:** 80% minimum (checked in CI)

Commands:
- Quick: `make test-short` (1m)
- Full: `make test` (15m)
- Coverage: `make coverage-check`

## Debugging Checklist

### "Tests Failing"

1. Check if it's a flaky test (rerun a few times)
2. If consistent: read error message carefully
3. If database-related: check transaction cleanup, concurrency
4. If frontend-related: check useEffect cleanup, state updates
5. If random: check goroutine leaks, timeout issues

### "Build Fails"

1. Try `make clean` first
2. Check npm cache: `npm ci` (instead of install)
3. For Go: ensure `go.mod` versions are correct
4. Check if frontend build failed (look for web/dist/)

### "API Returns 401"

1. Check if .api-token exists and is valid
2. Run `server-bootstrap` skill to get fresh token
3. Verify token format: `abbs_*`

### "Service Crashes"

1. Check logs: `server-logs status`
2. Look for panic, fatal errors
3. Check disk space on server
4. Check database consistency (Pebble compaction issues?)

## When to Create New Skills

These patterns might warrant new skills (if you find yourself repeating them):

- **Database migrations:** Create migration, apply, test, rollback
- **Performance profiling:** Profile production, analyze bottlenecks
- **PebbleDB operations:** Inspect keys, backfill logic, version checks
- **Batch job management:** Resume, check status, cleanup
- **File reconciliation:** Verify tags, update paths, handle missing files
- **Docker operations:** Build image, push registry, deploy container
- **CI debugging:** Inspect workflow failures, rerun jobs, view logs
- **Activity log analysis:** Query digests, generate reports, export data

Suggest these in future `project-patterns` updates as they become patterns.

## File Structure Quick Guide

```
.
├── internal/
│   ├── database/        # PebbleDB, stores, migrations
│   ├── metadata/        # Tagging, enrichment, dedup
│   ├── operations/      # Batch ops, scanning, maintenance
│   ├── logging/         # Activity log, operation logging
│   ├── server/          # HTTP handlers, auth, bootstrap
│   └── tests/           # Test helpers, fixtures
├── web/
│   ├── src/
│   │   ├── components/  # React components
│   │   ├── features/    # Feature modules (activity, library, etc.)
│   │   ├── hooks/       # Custom hooks
│   │   └── services/    # API client, utilities
│   ├── tests/
│   │   ├── unit/        # Vitest unit tests
│   │   └── e2e/         # Playwright tests
│   └── vite.config.ts
├── .github/
│   ├── copilot-instructions.md  # Main architecture guide
│   ├── instructions/            # Specific workflows
│   └── prompts/                 # Prompt templates
├── tools/
│   └── cmd/             # CLI utilities (reconcile-paths, etc.)
├── Makefile             # Build targets
├── Makefile.local       # Local overrides (not committed)
├── go.mod              # Go dependencies
└── package.json        # Node dependencies
```

## See Also

- `build-deploy` — Building and running the app
- `server-bootstrap` — Getting API credentials
- `server-logs` — Fetching logs and debugging
- `.github/copilot-instructions.md` — Full architecture guide
- `CLAUDE.md` — Workflow discipline + constraints
