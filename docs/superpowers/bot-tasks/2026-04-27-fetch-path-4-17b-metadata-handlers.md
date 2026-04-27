<!-- file: docs/superpowers/bot-tasks/2026-04-27-fetch-path-4-17b-metadata-handlers.md -->
<!-- version: 1.0.0 -->
<!-- guid: 6e1f3b70-5c28-4d9a-86fc-9b8e0a172453 -->

# BOT TASK: 4.17b — Audit metadata_handlers.go for service-delegation gaps

**TODO ID:** 4.17b
**Companion human design:** [`docs/superpowers/specs/2026-04-27-fetch-path-audit-design.md`](../specs/2026-04-27-fetch-path-audit-design.md)

This is an **audit-only, docs-only PR**. No code changes. Output is a markdown report.

## Branch

```
docs/4-17b-metadata-handlers-audit
```

## Files to create

1. `docs/audits/2026-04-27-handler-service-delegation.md` (new — see Step 3 for shape; you'll be the first to create it; subsequent audits append)

## Files to read

Every handler in `internal/server/metadata_handlers.go`. List with:

```
grep -n "^func (s \*Server)" internal/server/metadata_handlers.go
```

## Step 1 — Build the inventory

For each handler in that file, fill out a row in the report with:

- **Handler name** (e.g. `bulkFetchMetadata`)
- **Lines of code** (`awk '/^func \(s \*Server\) NAME\(/,/^}/' file | wc -l`)
- **Touches metafetch.Service?** (grep handler body for `metadataFetchService\.`)
- **Reimplements source-chain?** (grep handler body for `BuildSourceChain` or `metadata\.MetadataSource`)
- **Reimplements cache?** (grep handler body for `metadataFetchCache\.`)
- **Verdict:** `delegates`, `partial-divergence`, `full-divergence`, or `no-fetch`

## Step 2 — Skip 4.17a's target

`bulkFetchMetadata` is being refactored under 4.17a. Mark its row as `tracked-by-4.17a` and exclude it from the divergence count.

## Step 3 — Report shape

```markdown
<!-- file: docs/audits/2026-04-27-handler-service-delegation.md -->
<!-- version: 1.0.0 -->

# Handler / Service Delegation Audit

Tracks whether each handler delegates fetch/cache/retry to the appropriate
service, or reimplements logic that lives in the service layer.

Generated for TODO 4.17 (fetch-path audit).

## metadata_handlers.go (4.17b)

| Handler | LOC | Touches Service | Source-chain | Cache | Verdict |
|---|---|---|---|---|---|
| `batchUpdateMetadata` | 36 | yes | no | no | delegates |
| `bulkFetchMetadata` | 280 | yes | yes | yes | tracked-by-4.17a |
| ... | | | | | |

## dedup_handlers.go (4.17c)

(populated by 4.17c)

## audiobook handlers (4.17d)

(populated by 4.17d)

## Follow-up tasks

For every row marked `partial-divergence` or `full-divergence`, append a
TODO.md task `4.17e`, `4.17f`, ... pointing at this report row.
```

## Step 4 — Append follow-ups to TODO.md

For each `partial-divergence` or `full-divergence` row found:

Add a line to TODO.md under the existing 4.17 area, with letter increment after `4.17a`–`4.17d`:

```
- [ ] **4.17e** — Delegate `<handler>` in `<file>` to `metafetch.Service` (gap: <which divergence>). See [audit row](docs/audits/2026-04-27-handler-service-delegation.md#metadata_handlersgo-417b).
```

If zero divergences are found in this file, write "All handlers in this file delegate correctly. No follow-up needed." in the report and **do not** add any TODO entries.

## Step 5 — Verify

```
make ci  # should pass — only docs changed
```

`grep` for the audit file to confirm it's tracked:
```
git status docs/audits/
```

## Step 6 — Commit

```
docs(audit): metadata_handlers.go service-delegation audit (TODO 4.17b)

Inventory of every metadata handler with verdict on whether it
delegates to metafetch.Service. <N> divergences found, follow-ups
filed as TODO 4.17e..4.17?.

Spec: docs/superpowers/specs/2026-04-27-fetch-path-audit-design.md
```

## Definition of done

- [ ] `docs/audits/2026-04-27-handler-service-delegation.md` exists with the metadata_handlers.go section populated
- [ ] Every handler in the file has a row
- [ ] Any divergences turned into `4.17e+` TODO entries
- [ ] `make ci` green
- [ ] PR opened with title `docs(audit): metadata_handlers.go delegation audit (4.17b)`

## When to STOP

NEEDS_REVIEW if:

- A handler's verdict is genuinely ambiguous — e.g. it calls the service for one path and reimplements for another. Mark as `partial-divergence` and surface a comment for the human reviewer to decide refactor scope.
- A handler reimplements logic from a service that does NOT yet exist (rare, but possible during half-finished extractions).
