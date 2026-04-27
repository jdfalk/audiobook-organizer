<!-- file: docs/superpowers/specs/2026-04-27-fetch-path-audit-design.md -->
<!-- version: 1.0.0 -->
<!-- guid: 4c9d1f5e-3a06-4b78-94ea-7f6c8d950231 -->

# Divergent Fetch-Path Audit (TODO 4.17)

**TODO ID:** 4.17
**Audience:** human reviewer
**Companion bot recipes:** see [Bot tasks](#bot-tasks) section.
**Size:** M — split into 4 sub-tasks, ~150 LOC each.

## Problem

`bulkFetchMetadata` in `internal/server/metadata_handlers.go:471` re-implements logic already owned by `metafetch.Service.FetchMetadataForBook`:

| Concern | Service has it | bulkFetchMetadata has it | Divergent? |
|---|---|---|---|
| Source chain iteration | yes | yes | yes — bulk only walks `BuildSourceChain()` results |
| Per-source caching (read) | yes (TTL-aware) | yes (manual TTL check) | yes — duplicate TTL math |
| Per-source caching (write) | yes | yes | yes — different code paths |
| Result scoring | yes | partial | yes — bulk has simpler scoring |
| `ContextualSearch` fallback | yes | no | yes |
| Retry on transient errors | yes | no | yes |
| ApplyMetadataCandidate hook | indirect | direct DB write | yes |

The divergence has bitten us before — when a fix to `Service.FetchMetadataForBook` doesn't propagate to the bulk handler, users see "the same book fetched two ways gives different results."

A grep for `BuildSourceChain` shows two callsites: the service itself, and the handler. Every other handler delegates correctly. **This handler is the only known violation today** — but the directive says "audit all handler/service pairs for similar TTL-check or source-chain duplication."

## Goal

Two outcomes:

1. **`bulkFetchMetadata` becomes a thin orchestrator** — iterates book IDs, calls `metafetch.Service.FetchMetadataForBook(ctx, bookID, opts)` per book, aggregates results. No source-chain logic in the handler. No cache logic in the handler. No TTL math in the handler.

2. **A repo-wide audit confirms no other handler/service pair has the same problem** — produce a report listing every handler that touches metadata, dedup, or audiobook services and confirms it delegates rather than reimplementing.

## Why this matters

The divergence is silent. A user sees an inconsistent result and can't tell which path produced it. As the service grows (it's the layer where retries, contextual search, and apply-pipeline live), every divergent caller becomes a potential bug surface. Centralizing now is a one-time cost; leaving it is a tax on every future service-layer change.

## Design decisions

**One service method, parameterized.** The handler's "only-missing", "force-refresh", and "fields filter" knobs all become options on `FetchMetadataForBook`. The handler stops owning behavior; it owns presentation (what to put in the response).

**Audit produced as a markdown file**, not just a code change. Future contributors need the inventory to spot regressions. Output: `docs/audits/2026-04-27-handler-service-delegation.md`.

**Per-handler bot tasks**, not one big sweep. Each handler is independently auditable; mixing them defeats the resumability of the burndown bot.

## Bot tasks {#bot-tasks}

Four sub-tasks, each independently mergeable:

1. **4.17a** — Refactor `bulkFetchMetadata` to delegate. [`bot-tasks/2026-04-27-fetch-path-4-17a-bulk-fetch.md`](../bot-tasks/2026-04-27-fetch-path-4-17a-bulk-fetch.md)
2. **4.17b** — Audit metadata handlers (`metadata_handlers.go`). [`bot-tasks/2026-04-27-fetch-path-4-17b-metadata-handlers.md`](../bot-tasks/2026-04-27-fetch-path-4-17b-metadata-handlers.md)
3. **4.17c** — Audit dedup handlers (`dedup_handlers.go`, `dedup_*.go`). [`bot-tasks/2026-04-27-fetch-path-4-17c-dedup-handlers.md`](../bot-tasks/2026-04-27-fetch-path-4-17c-dedup-handlers.md)
4. **4.17d** — Audit audiobook handlers (`audiobook*.go`). [`bot-tasks/2026-04-27-fetch-path-4-17d-audiobook-handlers.md`](../bot-tasks/2026-04-27-fetch-path-4-17d-audiobook-handlers.md)

4.17a is the actual refactor. 4.17b/c/d are read-only audits that produce a markdown report and open a docs-only PR. They're auto-ok eligible because they don't touch code; they document the inventory.

If an audit (b/c/d) discovers a violation, it should NOT auto-fix it — instead, append a follow-up task to TODO.md (`4.17e`, `4.17f`, …) and let a separate bot run handle the refactor with a fresh spec.

## Out of scope

- Cross-cutting refactor of the service itself. The service stays as-is; the handlers move toward it.
- Performance work. Delegation may add a function-call layer; if the bulk path becomes measurably slower, file a separate task.
