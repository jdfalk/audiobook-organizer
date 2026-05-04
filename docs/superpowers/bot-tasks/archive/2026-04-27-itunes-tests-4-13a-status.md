<!-- file: docs/superpowers/bot-tasks/2026-04-27-itunes-tests-4-13a-status.md -->
<!-- version: 1.0.0 -->
<!-- guid: ac5d7f14-9a6b-4c0e-8b1d-df21eb5b6897 -->

# BOT TASK: 4.13a — Tests for internal/itunes/service/status.go

**TODO ID:** 4.13a
**Companion human design:** [`docs/superpowers/specs/2026-04-27-itunes-test-suite-design.md`](../specs/2026-04-27-itunes-test-suite-design.md)

## Branch

```
test/4-13a-itunes-status
```

## Files

- **Read:** `internal/itunes/service/status.go` (138 LOC, no test today)
- **Read for patterns:** `internal/itunes/service/path_repair_test.go` (strong coverage, copy its structure)
- **Read for mocks:** `internal/itunes/service/assert_test.go`, `*_mock_test.go`
- **Create:** `internal/itunes/service/status_test.go`

## Step 1 — Map the file

Run:
```
grep -n "^func " internal/itunes/service/status.go
```

For each exported function (capitalized first letter), note the signature and what it does. Typical content of a `status.go` in this package: query functions about iTunes connection state, last-sync time, pending operations.

## Step 2 — Test scaffold

Use the package's existing test pattern. Skeleton:

```go
package service

import (
    "context"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
)

func TestStatus_<FunctionName>_<Case>(t *testing.T) {
    s := newTestService(t) // helper from assert_test.go or service_test.go — verify name with grep
    // ...
}
```

If `newTestService` doesn't exist, search for a similar helper (`newServiceForTest`, `setupTestService`, etc.). **Do not invent a new constructor** — use what's there.

## Step 3 — Required test cases

For every exported function in `status.go`, write at minimum:

1. **Happy path** — function returns expected value on a normal service.
2. **Disabled mode** — `Deps.Enabled = false` (or the equivalent flag — read service.go for the flag name) → function returns the appropriate zero / error.
3. **Empty store** — store has no relevant data → function returns empty / not-found cleanly.
4. **Store error** — store returns an error → function propagates or wraps it.
5. **Context cancellation** (only if the function takes `ctx context.Context`) — pre-cancelled context → function returns `ctx.Err()`.

If the file has 4 exported functions, expect ~16–20 subtests. Use `t.Run` to organize.

## Step 4 — Verify

```
go test -v -cover ./internal/itunes/service/ -run TestStatus
go test -cover ./internal/itunes/service/...
```

The package coverage **should rise**. Compare to the baseline 55%; expect ~3–5 percentage points gain from this task alone.

## Step 5 — Commit

```
test(itunes): add status.go test coverage (TODO 4.13a)

Covers <N> functions previously at 0% coverage. Tests follow the
existing assert_test.go / path_repair_test.go pattern. Disabled-mode,
empty-store, and store-error paths covered for every exported function.

Spec: docs/superpowers/specs/2026-04-27-itunes-test-suite-design.md
```

## Definition of done

- [ ] `go test -cover ./internal/itunes/service/...` shows package coverage strictly higher than baseline
- [ ] Every exported function in `status.go` has at least one test
- [ ] Disabled-mode covered for every function that has the early-exit pattern
- [ ] `make ci` green
- [ ] CHANGELOG prepended
- [ ] TODO.md `4.13a` flipped to `[x]`

## When to STOP

NEEDS_REVIEW if:

- `status.go` has fewer than 2 exported functions (then this task is too small to be useful — fold into 4.13b).
- The package's test helpers (`newTestService` or equivalent) genuinely don't exist or are private to other test files in a way that's not portable. Surface the gap.
- An exported function takes a type that has no obvious mock (e.g. a remote-host SSH client). Note the gap; cover what's possible; surface the rest.
