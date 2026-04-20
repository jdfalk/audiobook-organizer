<!-- file: docs/superpowers/specs/2026-04-18-itunes-service-extraction-v2-notes.md -->
<!-- version: 1.1.0 -->
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

Phase 1 (pre-work PRs) — **partially shipped 2026-04-18:**

1. ✅ **P1** — `Enqueuer` interface (#403 merged). 7 server-package consumers narrowed from `*WriteBackBatcher` to `server.Enqueuer`.
2. ✅ **P2** — Removed `GlobalWriteBackBatcher` singleton (#404 merged). Bonus: fixed latent bug where the `apply_metadata` recovery handler never actually enqueued write-backs (the nil-guarded path was always nil).
3. ✅ **P3** — Injected `WriteBackBatcherConfig` into the batcher (#405 merged). Five inline `config.AppConfig` reads replaced with struct fields behind a dedicated `cfgMu` RWMutex (separate from `mu` so hot-reload doesn't contend with the pending-ops critical section). `UpdateConfig` method ready for hot-reload wiring.
4. ⏭ **P4** — Plan handler-name corrections. Skipped — the v1 plan is being replaced by v2 entirely.

**A fresh Phase 2 M1 attempt after P1–P3 landed escalated BLOCKED with different cascades** than the v1 Task 2 attempt. Four additional pre-work PRs are needed before M1 can run mechanically. These were not anticipated by the original v1 spec or the v2 shift to self-registering routes.

## 5. Additional pre-work required (discovered during v2 M1 attempt)

### 5.1 Pre-PR P5 — Extract `read_status_engine.go` from `internal/server/`

**Blocker:** `internal/server/itunes_position_sync.go` calls `RecomputeUserBookState` and `SetManualStatus` which live in `internal/server/read_status_engine.go`. Moving `itunes_position_sync.go` into `internal/itunes/service/` would force the service to import `internal/server`, but `internal/server` already imports `internal/itunes/service` — that's a circular dep.

**Fix:** extract `read_status_engine.go` (+ its test) to a new `internal/readstatus/` package. Both `internal/server/` (for its existing handlers) and `internal/itunes/service/` (post-move) import it. Size: ~156 LOC plus tests. Narrow deps — probably just `database.BookFileStore + database.UserPositionStore` (already narrowed during the ISP sweep 4.8).

### 5.2 Pre-PR P6 — Inject store into `WriteBackBatcher`

**Blocker:** `internal/server/itunes_writeback_batcher.go` lines 147, 203 call `database.GetGlobalStore()` — a second hidden global that P2 didn't touch. Moving the batcher into `internal/itunes/service/` would leave a transitive dependency on the database-package global.

**Fix:** mirror P3's pattern. Add a `store` field on `WriteBackBatcher` populated at construction. The line 147 case is a best-effort goroutine inside `EnqueueRemove`; the line 203 case is the flush-time store fetch. Both become field reads. Constructor signature grows one more arg.

### 5.3 Pre-PR P7 — Organizer dependency shape

**Blocker:** `internal/server/itunes.go:1742` calls `organizer.NewOrganizer(&config.AppConfig)` — the organizer takes a full `*config.Config`, not a narrow subset. The v2 `itunesservice.Config` (9 fields) doesn't expose a path for this. Can't just pass through `config.AppConfig` without reintroducing the global dependency the whole extraction is trying to remove.

**Fix options** (each has trade-offs):
- **(a) Inject the organizer as a `Deps.Organizer` field.** Cleanest — iTunes doesn't know about organizer config, just calls into a pre-built instance. Server owns organizer lifecycle. Needs the organizer to expose the exact surface iTunes uses (just `Organize(book)`? — check before deciding).
- **(b) Widen `itunesservice.Config` to hold a `*config.Config` pointer.** Escape hatch. Explicitly violates the "no `config.AppConfig` transitive dep" invariant we just bought in P3. Marks the extraction as incomplete.
- **(c) Narrow the organizer's signature first.** `organizer.NewOrganizer` taking a full config is itself a smell. Pre-PR would change it to take only the fields it needs. Then iTunes can pass those explicitly.

Recommended: (a) for Phase 2; consider (c) as a later cleanup if the organizer surface is small enough.

### 5.4 Pre-PR P8 — `executeITunesSync` scheduler integration

**Blocker:** `internal/server/itunes.go:2065` defines `executeITunesSync` as a plain function. Two callers:
- `internal/server/server.go:~2540` — invoked directly from the `triggerITunesSync` scheduler callback
- The interrupted-operation resume path around `server.go:1413`

When `executeITunesSync` moves to `internal/itunes/service/` as a `*Service` method (via `*Importer` or similar), both call sites need rewiring. The scheduler callback currently closes over package-level scope; after the move it needs to close over `s.itunesSvc`.

**Fix:** expose `executeITunesSync` as a public method (probably `(s *Service) RunSync(...)` or on `*Importer.Sync`). Server's scheduler callback becomes a closure over `s` that calls `s.itunesSvc.Importer.Sync(...)`. Same pattern for the resume dispatch switch.

Small change but spans two call sites + scheduler shape.

### 5.5 Impact on M1

With P5–P8 landed, M1's sub-component moves become truly mechanical:

- **Transfer / TrackProvisioner / PlaylistSync / PathReconciler / Importer** — no additional surprises expected
- **PositionSync** — unblocked by P5 (no longer reaches into server package)
- **WriteBackBatcher** — unblocked by P6 (no more `GetGlobalStore` calls)
- **Server wiring** — unblocked by P7 (organizer via Deps) + P8 (scheduler callback through service)

Revised M1 estimate: still 2–3 hours after P5–P8 land, but with high confidence of success.

## 6. Revised next-session execution order

**Phase 1a** (this session's state): ✅ P1 #403, ✅ P2 #404, ✅ P3 #405, ⏭ P4 skipped.

**Phase 1b** (next session — pre-work continued):
5. **P5** — Extract `read_status_engine.go` to `internal/readstatus/`. ~30 min.
6. **P6** — Inject store into `WriteBackBatcher`. ~20 min.
7. **P7** — Inject organizer into `Deps` (option (a)). ~30 min unless organizer surface is wider than expected.
8. **P8** — Expose `executeITunesSync` as a method + rewire scheduler callback. ~20 min.

**Phase 2** (subsequent session — unchanged shape):
9. **M1** — Move 7 sub-components. 2–3 hours.
10. **M2** — Self-registering routes. 1–2 hours.
11. **M3** — Docs closure. 30 min.

## References

- v1 spec: `docs/superpowers/specs/2026-04-18-itunes-service-extraction-design.md` (§1–§8 still authoritative for the Service shape)
- v1 plan: `docs/superpowers/plans/2026-04-18-itunes-service-extraction.md` (replace with v2 before Phase 2 starts)
- v1 foundation PR: #401 (merged)
- v2 pre-work PRs: #403 (P1 Enqueuer), #404 (P2 no-global), #405 (P3 config-injected)
- v1 Task 2 BLOCKED escalation: 2026-04-18 session notes (captured inline above under §10 in the main spec)
- v2 M1 BLOCKED escalation: 2026-04-19 session — identified P5–P8 cascades above
