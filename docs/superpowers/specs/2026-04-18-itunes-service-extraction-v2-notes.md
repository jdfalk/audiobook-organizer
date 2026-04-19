<!-- file: docs/superpowers/specs/2026-04-18-itunes-service-extraction-v2-notes.md -->
<!-- version: 1.0.0 -->
<!-- guid: 7c4f1e2a-8d5b-4e9c-a3f6-2b1d0e4c5f78 -->

# iTunes Extraction — v2 notes

**Status:** v1 foundation (#401) shipped. v1 Task 2 attempted and escalated BLOCKED. This document captures what v2 needs to address — the v1 spec and plan remain authoritative for everything else.

## Context

The v1 Task 2 implementer subagent hit real plan gaps during code inspection and escalated `BLOCKED` rather than commit broken intermediate state. Its analysis was correct; the v1 plan was written optimistically. Next session should incorporate these findings before retrying.

## What v2 changes

### 1. Shift from v1 design option A → option B (self-registering routes)

Investigation found 4 iTunes handlers outside `internal/server/itunes*.go`:

| Handler | Current file | Route |
|---|---|---|
| `startITunesPathReconcile` | `internal/server/itunes_path_reconcile.go` | `POST /operations/itunes-path-reconcile` |
| `handleRecomputeITunesPaths` | `internal/server/maintenance_fixups.go:3420` | `POST /maintenance/recompute-itunes-paths` |
| `handleGenerateITLTests` | `internal/server/maintenance_fixups.go:3502` | `POST /maintenance/generate-itl-tests` |
| `rebuildITLHandler` | `internal/server/itl_rebuild.go` | `POST /itunes/rebuild` |

Keeping handlers in `server.go` per v1 option A would require pulling those scattered handlers back into server.go first. Instead, move ALL iTunes handlers into `internal/itunes/service/handlers_*.go` and have the service self-register routes via three methods:

```go
func (s *Service) RegisterRoutes(rg *gin.RouterGroup, perm auth.MW)            // /itunes/*
func (s *Service) RegisterOperationsRoutes(rg *gin.RouterGroup, perm auth.MW)   // /operations/itunes-path-reconcile
func (s *Service) RegisterMaintenanceRoutes(rg *gin.RouterGroup, perm auth.MW)  // /maintenance/*itunes*
```

Disabled mode: `RegisterRoutes` installs 503-returning stubs so endpoints remain stable (not 404) when iTunes is off.

### 2. Pre-work PRs required before main extraction

Code inspection found these blockers to a clean v1 Task 2 execution:

| Blocker | Scope | Fix |
|---|---|---|
| `*WriteBackBatcher` referenced in 52+ sites across 18+ files | Signature churn | **P1:** Introduce `server.Enqueuer` interface; switch callers to use it |
| `GlobalWriteBackBatcher` package singleton + `InitWriteBackBatcher()` | Hidden dep in `file_io_pool.go:223-224` | **P2:** Inject the batcher into `file_io_pool.go`; delete the global |
| `config.AppConfig.*` reads inline inside batcher goroutines (lines 61, 75, 90, 165, 256 of `itunes_writeback_batcher.go`) | Package portability | **P3:** Add `WriteBackBatcherConfig` struct; inject at construction; add `UpdateConfig` for hot-reload |
| v1 plan uses wrong handler names (`handleITunesDownload` vs real `handleITLDownload`, plus 3 others) | Plan correctness | **P4:** Trivial search-and-replace in the v1 plan |

Each pre-work PR is small, independently valuable, and independently testable. Together they make the main extraction mechanical.

### 3. Revised main-extraction ordering

v1 plan's step 2b (TrackProvisioner) depended on `*WriteBackBatcher`, but step 2f moved the batcher — circular during intermediate states.

After pre-work (specifically P1 Enqueuer interface), the order becomes:

1. Transfer
2. TrackProvisioner (now takes `Enqueuer`, not `*WriteBackBatcher`)
3. **WriteBackBatcher** (promoted from v1's 6th to 3rd — safe because P3 made it config-portable)
4. PositionSync
5. PathReconciler
6. PlaylistSync
7. Importer (last, biggest; also flips `Server` from `NewDisabled()` to conditional `New(deps)`)

### 4. Deletions after extraction completes

Delete entirely:
- `internal/server/itunes.go`
- `internal/server/itunes_transfer.go`
- `internal/server/itunes_writeback_batcher.go` (moved during main extraction step 3)
- `internal/server/itunes_position_sync.go`
- `internal/server/itunes_path_reconcile.go`
- `internal/server/itunes_track_provisioner.go`
- `internal/server/playlist_itunes_sync.go`
- `internal/server/itl_rebuild.go`

Shrink:
- `internal/server/maintenance_fixups.go` — remove `handleRecomputeITunesPaths` + `handleGenerateITLTests` (now in iTunes service)

v1 spec success criteria (`server.go` iTunes refs ≤ 15, `internal/server/itunes_handlers.go` ≤ 800 lines) still apply — except `itunes_handlers.go` no longer exists in v2 (handlers live inside the service instead).

## Next-session execution plan

Phase 1 (pre-work PRs) — one session session of focused work:

1. **P1** — `Enqueuer` interface. ~25 call-site changes. Maybe 30 min.
2. **P2** — Remove `GlobalWriteBackBatcher`. ~3 file changes. Maybe 20 min.
3. **P3** — Inject `WriteBackBatcherConfig` into batcher. ~5 inline reads converted + 1 caller. Maybe 30 min.
4. **P4** — Plan handler-name corrections. 5 min.

Phase 2 (main extraction) — separate session(s):

5. **M1** — Move 7 sub-components (one commit each). 2-3 hours.
6. **M2** — Self-registering routes; handlers move into service; delete the 7 server-side files. 1-2 hours.
7. **M3** — Docs closure (CHANGELOG, TODO 4.9, TODO 4.10, disabled-mode smoke test). 30 min.

## References

- v1 spec: `docs/superpowers/specs/2026-04-18-itunes-service-extraction-design.md` (§1–§8 still authoritative)
- v1 plan: `docs/superpowers/plans/2026-04-18-itunes-service-extraction.md` (replace with v2 before Phase 2 starts)
- v1 foundation PR: #401 (merged)
- v1 Task 2 BLOCKED escalation: session notes from 2026-04-18
