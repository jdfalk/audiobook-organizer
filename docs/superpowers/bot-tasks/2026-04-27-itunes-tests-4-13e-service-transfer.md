<!-- file: docs/superpowers/bot-tasks/2026-04-27-itunes-tests-4-13e-service-transfer.md -->
<!-- version: 1.0.0 -->
<!-- guid: e0913358-de0f-4042-cf51-23501fc09acb -->

# BOT TASK: 4.13e — Tests for service.go and transfer.go

**TODO ID:** 4.13e
**Companion human design:** [`docs/superpowers/specs/2026-04-27-itunes-test-suite-design.md`](../specs/2026-04-27-itunes-test-suite-design.md)
**Pattern reference:** [`4.13a`](2026-04-27-itunes-tests-4-13a-status.md) — read first.

## Branch

```
test/4-13e-itunes-service-transfer
```

## Files

- **Read:** `internal/itunes/service/service.go` (161 LOC, 0.31 coverage), `internal/itunes/service/transfer.go` (333 LOC, 0.35 coverage)
- **Read:** `internal/itunes/service/service_test.go`, `transfer_test.go`, `transfer_handler_test.go`
- **Edit:** existing `service_test.go` and `transfer_test.go`. Do NOT create new files unless an existing one is genuinely the wrong place.

## Approach

Same as 4.13d — find the lowest-coverage functions and fill the gaps.

```
go test -coverprofile=/tmp/cov.out ./internal/itunes/service/...
go tool cover -func=/tmp/cov.out | grep -E "service\.go|transfer\.go" | sort -k3 -n
```

## service.go required coverage

Functions/methods to cover with ≥ 1 test each (read the file to confirm names):

- Constructor (`NewService` or equivalent) — happy path + missing-deps error.
- Lifecycle: `Start`, `Stop` — clean start, double-start, stop-without-start.
- `Status()` accessor — returns expected struct in all states (running / stopped / disabled).
- Disabled-mode propagation — service constructed with `Deps.Enabled = false` returns early from every public method.

## transfer.go required coverage

This is the file that ships .itl writes to the remote Windows machine.

- **Happy path** — full ITL transfer succeeds.
- **Network error** — remote unreachable → expected error wrapping.
- **Auth error** — credentials reject → distinct error type if one exists.
- **Partial transfer** — write fails mid-stream → no commit on remote; local state unchanged.
- **Backup creation** — `SafeWriteITL` creates timestamped backup before overwriting (if applicable in this layer).
- **Atomic rename failure** — temp file rename fails → original remote file preserved, error surfaced.
- **Concurrent transfer** — two transfers attempted simultaneously → second blocks or errors.
- **Disabled mode** — transfer attempted on disabled service → early return.

Transfer tests will need a fake remote target. Look for an existing `fakeRemote` or similar in `transfer_handler_test.go`. If absent, build a minimal one in the test file (a `bytes.Buffer`-backed `io.Writer` is usually enough for these layers).

## Verify

```
go test -cover ./internal/itunes/service/...
```

Package coverage should reach **80%+** after this task. This is the final test-suite sub-task.

If coverage is still < 80% after this PR, file `4.13f` (writeback_batcher deepening) and surface the result in the TODO.md update.

## Commit

```
test(itunes): service.go and transfer.go coverage (TODO 4.13e)

Lifecycle (start/stop/status), disabled-mode, network/auth/partial-
transfer, backup creation, atomic-rename, concurrent-transfer.
Package coverage rose from <X>% to <Y>%.

Spec: docs/superpowers/specs/2026-04-27-itunes-test-suite-design.md
```

## Definition of done

- [ ] service.go coverage ≥ 0.65 ratio
- [ ] transfer.go coverage ≥ 0.65 ratio
- [ ] **Package coverage ≥ 80%** (the spec target). If not, `4.13f` filed.
- [ ] `make ci` green
- [ ] CHANGELOG prepended
- [ ] TODO.md `4.13e` flipped to `[x]`; `4.13` parent task flipped to `[x]` if package ≥ 80%

## When to STOP

NEEDS_REVIEW if:

- The remote-transfer surface uses a real SSH / network library that resists faking. Document what's reachable, surface the rest.
- Coverage doesn't reach 80% even after this task. Don't open a follow-up TODO unilaterally — surface the gap for human triage.
