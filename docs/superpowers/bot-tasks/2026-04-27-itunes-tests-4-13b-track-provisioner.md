<!-- file: docs/superpowers/bot-tasks/2026-04-27-itunes-tests-4-13b-track-provisioner.md -->
<!-- version: 1.0.0 -->
<!-- guid: bd6e8025-ab7c-4d1f-9c2e-e032fc6c79a8 -->

# BOT TASK: 4.13b — Tests for internal/itunes/service/track_provisioner.go

**TODO ID:** 4.13b
**Companion human design:** [`docs/superpowers/specs/2026-04-27-itunes-test-suite-design.md`](../specs/2026-04-27-itunes-test-suite-design.md)
**Pattern reference:** [`4.13a`](2026-04-27-itunes-tests-4-13a-status.md) — read it first for the package test conventions.

## Branch

```
test/4-13b-itunes-track-provisioner
```

## Files

- **Read:** `internal/itunes/service/track_provisioner.go` (196 LOC, no test today)
- **Read:** `internal/itunes/service/track_provisioner_mock_test.go` (mocks exist; use them)
- **Create:** `internal/itunes/service/track_provisioner_test.go`

## What this code does

Track provisioning = creating new iTunes track entries when audiobook files are imported into the library. The provisioner takes a book + segment, builds the .itl track payload, and either inserts via the writeback batcher or returns the payload for caller-side staging.

## Required test cases

For each exported function in `track_provisioner.go`:

1. **Happy path** — single segment, valid book → track payload built correctly.
2. **Multi-segment book** — book with 3+ segments → one track per segment, ordered.
3. **Missing fields** — book with empty title/author → expected behavior (likely error or skip).
4. **Disabled mode** — service disabled → early return.
5. **Already-provisioned** — track already exists in `external_id_map` → no-op or update path.
6. **Idempotency** — call twice with same book → second call doesn't double-create.
7. **Store error on lookup** — `external_id_map` query fails → propagate.
8. **Writeback batcher full / closed** — Enqueue returns error → handle gracefully.
9. **Path is iTunes-protected** — book's path is on iTunes-managed location → either skip or compute correct relative path (read the code to determine which).

## Track-specific assertions

When a track payload is built, assert at minimum:
- `Title`, `Artist`, `Album` are populated from book fields
- `PersistentID` is unique per call (or stable for idempotent re-calls — read the code)
- `Location` URL is well-formed (`file://...` for local, server-relative path for managed)
- Any duration fields match book.Duration

## Step-by-step

Identical to 4.13a's Step 2–5. Use `track_provisioner_mock_test.go`'s mock store.

## Definition of done

- [ ] `track_provisioner.go` exported functions all have ≥ 1 test
- [ ] At least one multi-segment + one idempotency test
- [ ] Disabled-mode covered
- [ ] Package coverage rises by ~5+ percentage points (this is a bigger file)
- [ ] `make ci` green
- [ ] CHANGELOG prepended
- [ ] TODO.md `4.13b` flipped to `[x]`

## When to STOP

NEEDS_REVIEW if:

- Track payload construction has external dependencies that aren't mocked (e.g. live ITL parser). Note what's testable, surface the rest.
- The provisioner has a goroutine-based flush path that's hard to deterministically trigger from tests. The package has examples of `_, done := setupAsyncTest()` patterns elsewhere — try those first; surface if blocked.
