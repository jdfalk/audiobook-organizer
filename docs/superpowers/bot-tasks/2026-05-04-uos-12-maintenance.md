<!-- file: docs/superpowers/bot-tasks/2026-05-04-uos-12-maintenance.md -->
<!-- version: 1.0.0 -->
<!-- guid: a2a3b4c5-d6e7-8f9a-0b1c-2d3e4f5a6b7c -->
<!-- last-edited: 2026-05-04 -->

# UOS-12 — Migrate maintenance plugin

**Companion human spec:** §11 phase B.

## Branch

```
feat/uos-12-maintenance
```

## Goal

Migrate the remaining ops that don't belong to a feature plugin —
maintenance / janitor / metadata / library work. From grep, the
candidates are:

- `reconcile_scan` — **MUST be ResumeDrop** per spec
- `series-normalize` — already in queue today
- `author-dedup-scan`, `author-split-scan` — author maintenance
- `series-prune`
- `purge-deleted`, `tombstone-cleanup`, `temp-file-cleanup`,
  `cleanup_activity_log`, `purge_old_logs`
- `db-optimize`, `cleanup-old-backups`
- `metadata-refresh`, `metadata_fetch`,
  `metadata_candidate_fetch`
- `composer_tag_scan`
- `isbn_enrichment`
- `bulk_write_back`
- `external_id_backfill`, `malformed_m4b_remux`,
  `movement_atom_cleanup`, `malformed_m4b_transcode`
- `batch_poller`
- `transcode`
- `diagnostics_export`, `diagnostics_ai`

Some of these are user-triggered, some scheduled, some both. Use
the existing scheduler/handler code as the authoritative behavior;
do not change semantics.

## Files to add

1. `internal/plugins/maintenance/plugin.go` — plugin shell. `Name()`
   returns `"maintenance"`.
2. One file per OperationDef under
   `internal/plugins/maintenance/`. Naming: `<id>.go`.
3. Each Def picks `ResumePolicy` per the table below. **No defaults
   permitted.**

| OperationDef ID | ResumePolicy | Schedule | Notes |
|---|---|---|---|
| `maintenance.reconcile-scan` | **ResumeDrop** | nightly | THE structural fix |
| `maintenance.series-normalize` | ResumeRestart with phases | weekly | already checkpoint-aware |
| `maintenance.author-dedup-scan` | ResumeRequeue | weekly | idempotent |
| `maintenance.author-split-scan` | ResumeRequeue | weekly | idempotent |
| `maintenance.series-prune` | ResumeRequeue | weekly | idempotent |
| `maintenance.purge-deleted` | ResumeRequeue | hourly | idempotent |
| `maintenance.tombstone-cleanup` | ResumeRequeue | daily | idempotent |
| `maintenance.temp-file-cleanup` | ResumeRequeue | hourly | idempotent |
| `maintenance.cleanup-activity-log` | ResumeRequeue | daily | idempotent |
| `maintenance.purge-old-logs` | ResumeRequeue | daily | idempotent |
| `maintenance.db-optimize` | ResumeDrop | weekly | should not auto-resume mid-vacuum |
| `maintenance.cleanup-old-backups` | ResumeRequeue | daily | idempotent |
| `maintenance.metadata-refresh` | ResumeRestart with checkpoint | manual | long-running |
| `maintenance.metadata-fetch` | ResumeDrop | manual | per-book; cheap to retrigger |
| `maintenance.metadata-candidate-fetch` | ResumeRestart | manual | bulk; valuable to resume |
| `maintenance.composer-tag-scan` | ResumeRestart | manual | resumable |
| `maintenance.isbn-enrichment` | ResumeRestart | continuous | the existing 4 stuck ones in prod were a symptom of NO drop policy; this fixes it |
| `maintenance.bulk-write-back` | ResumeAsk | manual | destructive; user picks |
| `maintenance.external-id-backfill` | ResumeRestart | once-on-startup-if-incomplete | matches existing version-flag behavior |
| `maintenance.malformed-m4b-remux` | ResumeRestart | once-on-startup | matches existing |
| `maintenance.movement-atom-cleanup` | ResumeRestart | once-on-startup | matches existing |
| `maintenance.malformed-m4b-transcode` | ResumeRestart | once-on-startup | matches existing |
| `maintenance.batch-poller` | ResumeRequeue | every 1min | always-on |
| `maintenance.transcode` | ResumeAsk | manual | destructive |
| `maintenance.diagnostics-export` | ResumeRequeue | manual | idempotent |
| `maintenance.diagnostics-ai` | ResumeDrop | manual | costs OpenAI tokens |

4. Tests covering at least the resume policy of each (a bot can
   verify by registering the def, running, killing the process
   simulation, restarting, asserting the expected post-restart
   status).

## Files to edit

1. `internal/server/scheduler_tasks.go`:
   - Remove every scheduler entry that now lives on a maintenance
     OperationDef (their schedules come from `Schedule` field).
2. `internal/server/server_lifecycle.go`:
   - `resumeInterruptedOperations` is now legacy code; UOS-08's
     `resumeAfterStartup` does the work for v2 ops. The legacy
     function only needs to handle pre-migration v1 rows. Delete
     the cases that have moved to v2; add a comment noting the
     remaining cases are the v1 stragglers, all of which will be
     deleted in UOS-14.
3. `internal/plugins/plugins.go` — import maintenance.

## Hard rules

- **Every Def in the table above MUST be registered with the
  `ResumePolicy` value listed.** This is non-negotiable; codex MUST
  NOT pick a "more conservative" policy on its own.
- `maintenance.reconcile-scan` MUST be ResumeDrop. This is the
  whole reason this spec exists.
- `maintenance.isbn-enrichment` (which had 4 stuck rows in production
  on the day of this spec) MUST be ResumeRestart with a checkpoint
  cadence of once per 100 books processed.
- No Def gets a `Capabilities: []` empty list — at minimum,
  declare `CapLibraryRead`. Use grep to verify each Def's actual
  capabilities.

## Acceptance criteria

- [ ] All Defs registered with the correct ResumePolicy per the
      table.
- [ ] `make ci` passes.
- [ ] Manual restart-test on a staging instance: kick off
      `maintenance.reconcile-scan`, kill the server mid-run, restart,
      confirm op ends as `interrupted_dropped`.

## PR title

```
feat(uos): migrate maintenance ops to UOS (closes the reconcile_scan auto-resume bug)
```
