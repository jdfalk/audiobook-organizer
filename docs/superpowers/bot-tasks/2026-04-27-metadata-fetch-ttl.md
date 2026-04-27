<!-- file: docs/superpowers/bot-tasks/2026-04-27-metadata-fetch-ttl.md -->
<!-- version: 1.0.0 -->
<!-- guid: 3b8c0e4d-2f95-4a67-83d9-6e5b7c849120 -->

# BOT TASK: Metadata-fetch Cache TTL Enforcement

**TODO ID:** CACHE-FOLLOWUP-1
**Companion human design:** [`docs/superpowers/specs/2026-04-27-metadata-fetch-ttl-design.md`](../specs/2026-04-27-metadata-fetch-ttl-design.md)

## Branch

```
feat/metadata-fetch-ttl
```

## Files to edit

1. `internal/config/config.go`
2. `internal/database/metadata_fetch_cache.go`
3. `internal/database/metadata_fetch_cache_test.go`

## Step 1 — Add config field

In `internal/config/config.go`, in the `Config` struct, add near the existing cache-related fields (search for `CacheTTL` or `MetadataFetchCache` to find the right region):

```go
// MetadataFetchCacheMaxAgeDays — entries older than this are treated as
// cache misses with reason="expired". 0 = no expiry (default; preserves
// existing behavior). See spec docs/superpowers/specs/2026-04-27-metadata-fetch-ttl-design.md.
MetadataFetchCacheMaxAgeDays int `json:"metadata_fetch_cache_max_age_days" mapstructure:"metadata_fetch_cache_max_age_days"`
```

Default in `NewDefaultConfig()` (or equivalent constructor): `0` (matches existing behavior — infinite TTL).

Bump file's version header.

## Step 2 — Wire TTL check in cache lookup

In `internal/database/metadata_fetch_cache.go`:

1. Find the `Get` (or equivalent lookup) function. Today it returns a hit if the row exists.

2. Add a max-age parameter to the lookup. The cleanest approach: add a method on the cache struct that takes max-age:

   ```go
   // GetWithMaxAge returns a hit only if the entry exists AND its CachedAt
   // is within maxAge of now. maxAge=0 disables the TTL check.
   func (c *MetadataFetchCache) GetWithMaxAge(bookID, source string, maxAge time.Duration) (*MetadataFetchEntry, bool, error)
   ```

3. Inside that method:
   - Call the existing `Get` logic.
   - If hit and `maxAge > 0`:
     - Compute `age := time.Since(entry.CachedAt)`.
     - If `age > maxAge`:
       - Call `metrics.RecordCacheMiss("metadata_fetch", "expired")`.
       - Return `nil, false, nil` (treat as miss).
   - Otherwise return the hit.

4. Find every existing caller of `Get` (`grep -rn "metadataFetchCache.Get\|\.Get(.*source" internal/`). Update them to:
   - Compute `maxAge := time.Duration(cfg.MetadataFetchCacheMaxAgeDays) * 24 * time.Hour` once.
   - Call `GetWithMaxAge(bookID, source, maxAge)` instead of `Get(bookID, source)`.

5. Keep `Get` as a thin wrapper for backward compat:
   ```go
   func (c *MetadataFetchCache) Get(bookID, source string) (*MetadataFetchEntry, bool, error) {
       return c.GetWithMaxAge(bookID, source, 0)
   }
   ```

Bump file's version header.

## Step 3 — Tests

Add to `internal/database/metadata_fetch_cache_test.go`:

```go
func TestMetadataFetchCache_TTL_ZeroMeansInfinite(t *testing.T) {
    // Insert entry with CachedAt = 1 year ago.
    // GetWithMaxAge(..., maxAge=0) → must return hit.
}

func TestMetadataFetchCache_TTL_ExpiredReturnsMiss(t *testing.T) {
    // Insert entry with CachedAt = 8 days ago.
    // GetWithMaxAge(..., maxAge=7*24h) → must return miss.
    // Verify metrics.RecordCacheMiss was called with reason="expired" (use the testing hook
    // already wired in metrics_test.go — search for `setMetricsRecorder` or similar).
}

func TestMetadataFetchCache_TTL_FreshReturnsHit(t *testing.T) {
    // Insert entry with CachedAt = 1 day ago.
    // GetWithMaxAge(..., maxAge=7*24h) → must return hit.
}
```

If the metrics-recording test hook doesn't exist (check `internal/metrics/`), the third test simply asserts the miss return value without verifying the metric. Don't invent a new hook.

## Step 4 — Verify

```
go vet ./...
make test
make ci
```

## Step 5 — Commit

```
feat(cache): metadata_fetch TTL enforcement (CACHE-FOLLOWUP-1)

- New config knob MetadataFetchCacheMaxAgeDays (default 0 = infinite).
- Entries older than max-age return as miss with reason="expired"
  via the existing observability counters.
- Get() preserved for backward compat as a maxAge=0 wrapper.

Spec: docs/superpowers/specs/2026-04-27-metadata-fetch-ttl-design.md
```

## Definition of done

- [ ] `make ci` green
- [ ] CHANGELOG prepended under `## [Unreleased]`
- [ ] TODO.md `CACHE-FOLLOWUP-1` flipped to `[x]`
- [ ] No callers of the old `Get` exist in non-test code (`grep -rn "\.Get(" internal/database/metadata_fetch_cache* | grep -v _test.go` shows only the wrapper itself).

## When to STOP

Surface NEEDS_REVIEW if:

- The cache `Get` signature in this file is materially different from `(bookID, source string)` — adapt the new method's signature but flag the structural change.
- Callers of `Get` exist in more than 4 files. The spec assumed 1-2.
- `metrics.RecordCacheMiss` is not present in the codebase (would mean this task ran before the cache observability work shipped).
