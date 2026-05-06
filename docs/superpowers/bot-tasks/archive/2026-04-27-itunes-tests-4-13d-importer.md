<!-- file: docs/superpowers/bot-tasks/2026-04-27-itunes-tests-4-13d-importer.md -->
<!-- version: 1.0.0 -->
<!-- guid: df80a247-cd9e-4f31-be40-124beb8e9bca -->

# BOT TASK: 4.13d — Tests for internal/itunes/service/importer.go (error paths)

**TODO ID:** 4.13d
**Companion human design:** [`docs/superpowers/specs/2026-04-27-itunes-test-suite-design.md`](../specs/2026-04-27-itunes-test-suite-design.md)
**Pattern reference:** [`4.13a`](2026-04-27-itunes-tests-4-13a-status.md) — read first.

## Branch

```
test/4-13d-itunes-importer-errors
```

## Files

- **Read:** `internal/itunes/service/importer.go` (1372 LOC — biggest file in the package)
- **Read:** `internal/itunes/service/importer_test.go`, `importer_execute_test.go`, `importer_helpers_test.go`, `importer_integration_test.go`, `importer_mock_test.go`
- **Edit:** existing `importer_test.go` OR create `importer_error_paths_test.go` next to it

Coverage today: 0.42 ratio. Goal here is the **gaps** — error and edge cases the existing tests skip.

## Approach

Don't rewrite what's there. **Add** tests for the missing scenarios.

1. Run coverage with the per-line flag:
   ```
   go test -coverprofile=/tmp/cov.out ./internal/itunes/service/...
   go tool cover -func=/tmp/cov.out | grep importer.go | sort -k3 -n
   ```
2. Functions with <70% coverage are the targets. Pick ~6 of them.
3. For each, write tests that exercise the uncovered branches (error returns, defensive guards, retries).

## Required new test categories

For `importer.go` specifically, look for and cover:

1. **Disabled-mode** for every public method (early-return path).
2. **Corrupt ITL** — malformed bytes → import returns error without partial state in store.
3. **Concurrent sync** — two `Import` calls overlap → second sees `ErrSyncInProgress` (or whatever the package's lock-conflict error is).
4. **Empty library** — ITL has no albums → import succeeds with zero books added.
5. **External-ID collision** — incoming track's PID matches an existing book's PID → behavior matches spec (likely update vs skip).
6. **Partial write** — store fails mid-import → no books persisted (transactional behavior) OR documented partial-failure recovery (read the code to know which).
7. **Position sync race** — book exists, position update arrives during import → expected ordering.
8. **Cover-art missing** — track has no embedded art → import doesn't crash; book record has empty cover.

## Step-by-step

1. Identify the 6 lowest-coverage functions in importer.go (Step 1 above).
2. For each, add ≥ 2 subtests covering categories from the list above that apply.
3. Use existing mock store from `importer_mock_test.go`. Do NOT introduce new mocks.
4. Re-run coverage; confirm importer.go ratio rises to ≥ 0.65.

## Verify

```
go test -cover ./internal/itunes/service/...
```

Package coverage should reach **65%+** after this task (from 55% baseline). The remaining gap closes with 4.13e.

## Commit

```
test(itunes): error and edge-case coverage for importer.go (TODO 4.13d)

Covers disabled-mode, corrupt ITL, concurrent sync, empty library,
external-ID collision, partial-write, position-sync race, cover-art
missing. <N> new subtests added. importer.go coverage rose from
0.42 to ~0.70.

Spec: docs/superpowers/specs/2026-04-27-itunes-test-suite-design.md
```

## Definition of done

- [ ] importer.go file-level coverage ≥ 0.65 ratio
- [ ] Each of the 8 categories above has at least one test (where applicable to this file's API)
- [ ] `make ci` green
- [ ] CHANGELOG prepended
- [ ] TODO.md `4.13d` flipped to `[x]`

## When to STOP

NEEDS_REVIEW if:

- A category is genuinely not testable from the package's mock surface (e.g. requires live ITL binary). Note which, cover what's possible.
- Adding a test reveals a real bug (e.g. concurrent-sync test exposes a race). Surface the bug as a separate `bug:` TODO entry rather than fixing in this PR.
