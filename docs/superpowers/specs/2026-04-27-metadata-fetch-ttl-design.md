<!-- file: docs/superpowers/specs/2026-04-27-metadata-fetch-ttl-design.md -->
<!-- version: 1.0.0 -->
<!-- guid: 2a7b9d3c-1e84-4f56-92c8-5d4a6b738019 -->

# Metadata-fetch Cache TTL Enforcement

**TODO ID:** CACHE-FOLLOWUP-1
**Audience:** human reviewer
**Companion bot recipe:** [`docs/superpowers/bot-tasks/2026-04-27-metadata-fetch-ttl.md`](../bot-tasks/2026-04-27-metadata-fetch-ttl.md)
**Size:** S — one PR, ~80 LOC.

## Problem

`internal/database/metadata_fetch_cache.go` line 41 already has a self-flagged comment:

```go
// The CachedAt timestamp exists so an optional TTL policy can
```

It does. But nothing reads it. Every cache lookup returns hits regardless of age. The comment has been a TODO-in-code since the cache was introduced.

After the cache observability work (CHANGELOG 2026-04-25), `RecordCacheMiss(reason="expired")` is wired and ready — when an entry's age exceeds a configured max, we should emit it as a miss with that specific reason so the dashboard makes age-based eviction visible.

## Goal

A configurable max-age. Reads older than max-age return as misses (with metric reason) and trigger refetch. Default behavior (no max-age set) preserves existing infinite-TTL.

## Why this matters

The metadata-fetch cache is the long-lived one — it stores OpenLibrary / Audible / Hardcover responses. A book's Audible page can update (cover, narrator credit, series number). Without TTL we serve stale forever. The dedup engine and metadata-review UI both consume from this cache, so stale propagates downstream.

A 30-day default is conservative — if a book's external metadata changes, we accept up to 30 days of staleness in exchange for the cache hit rate. Users with strict freshness needs can set 1d.

## Design decisions

**Config field, not constant.** A user-tunable knob. Operators may want 7d for active curation, 90d for archival use.

**Zero = infinite (today's behavior).** Don't surprise existing installs on upgrade.

**Metric reason `"expired"`.** Already a recognized label on the cache miss counter. No metrics-side changes.

**Lazy expiry, not background sweep.** Match the in-memory cache's lazy-expire pattern from PR #461. No goroutine to manage.

## Out of scope

- Per-source TTLs (different max-age for Audible vs OpenLibrary). Add later if usage data justifies it.
- Eviction by total size. The cache is unbounded today; size-based eviction is a separate larger change.

## Bot recipe

[`docs/superpowers/bot-tasks/2026-04-27-metadata-fetch-ttl.md`](../bot-tasks/2026-04-27-metadata-fetch-ttl.md).
