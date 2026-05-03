# Plan: Fingerprint rescan endpoint + unified per-book fingerprint spec

## Goal

1. **Bot-task spec** for "unified per-book audio fingerprint synthesis +
   book-level matching" — fills the documented gap where today's matcher only
   compares per-file segments and has no canonical book-level signature.
2. **HTTP endpoint** (`POST /api/v1/dedup/fingerprint-rescan`) that
   force-(re)generates per-file AcoustID segments on demand.

## Files

- **NEW** `docs/superpowers/bot-tasks/2026-05-03-unified-book-fingerprint.md`
- **NEW** `internal/server/fingerprint_rescan.go` — handler + worker
- **EDIT** `internal/server/server_lifecycle.go` — register route under `/dedup`
- **EDIT** `internal/server/acoustid_backfill.go` — extract shared per-file helper
- **NEW** `internal/server/fingerprint_rescan_test.go`

## Endpoint

```
POST /api/v1/dedup/fingerprint-rescan
Auth: PermScanTrigger
Body: { "scope": "missing"|"all"|"books", "book_ids": [...], "force": bool }
202 Accepted: { "operation_id": "..." }
```

## Test

- Unit tests via mockery store
- `make build-api`
- `make test-short ./internal/server/...`

## Rollback

Additive route, no migrations. Revert the branch.
