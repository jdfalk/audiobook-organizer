<!-- file: docs/superpowers/bot-tasks/2026-04-27-fetch-path-4-17a-bulk-fetch.md -->
<!-- version: 1.0.0 -->
<!-- guid: 5d0e2a6f-4b17-4c89-95fb-8a7d9e061342 -->

# BOT TASK: 4.17a — Delegate bulkFetchMetadata to Service

**TODO ID:** 4.17a
**Companion human design:** [`docs/superpowers/specs/2026-04-27-fetch-path-audit-design.md`](../specs/2026-04-27-fetch-path-audit-design.md)

## Branch

```
refactor/4-17a-bulk-fetch-delegate
```

## Files to edit

1. `internal/server/metadata_handlers.go` (the handler — delete most of `bulkFetchMetadata` body, replace with service calls)
2. `internal/metafetch/service.go` (only if `FetchMetadataForBook` needs an `opts` arg — see Step 2)
3. `internal/metafetch/service_test.go` or `internal/metafetch/service_mock_test.go` (add coverage for new opts if added)
4. `internal/server/metadata_handlers_test.go` (if it exists; otherwise skip)

## Step 1 — Read the current handler

Read `internal/server/metadata_handlers.go:471` through end of `bulkFetchMetadata`. Note these blocks (will be deleted):

- Source-chain iteration (`for _, src := range sourceChain`)
- Per-source cache lookup with TTL (`metadataFetchCache.Get` + age check)
- Per-source cache write
- The fallback `metadata.NewAudibleClient()` block

The handler keeps:
- Request parsing (`bulkFetchMetadataRequest`, `c.ShouldBindJSON`)
- Per-book skip/error result construction
- Aggregating `results []bulkFetchMetadataResult` and the response

## Step 2 — Confirm Service supports the operation modes

Read `internal/metafetch/service.go` around line 268, function `FetchMetadataForBook`. Verify it supports:

- **Only-missing** behavior (`onlyMissing` param) — likely already does via `loadMetadataState`. If not, add an option struct:
  ```go
  type FetchOptions struct {
      OnlyMissing bool
      Fields      []string // optional filter
  }
  func (mfs *Service) FetchMetadataForBookWithOpts(id string, opts FetchOptions) (*FetchMetadataResponse, error)
  ```
  Keep the old `FetchMetadataForBook(id)` as a thin wrapper for backward compat.

- **Per-source TTL caching** — Service already does this via the metadata_fetch cache. No work needed.

- **Field filter** — if the handler today filters fields after fetching, expose that filter as `FetchOptions.Fields`.

If the existing Service signature already covers these (read carefully), skip the new method entirely and use what's there.

## Step 3 — Rewrite the handler body

Target shape (~80 LOC, down from ~280):

```go
func (s *Server) bulkFetchMetadata(c *gin.Context) {
    if s.Store() == nil {
        RespondWithInternalError(c, "database not initialized")
        return
    }
    var req bulkFetchMetadataRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        RespondWithBadRequest(c, err.Error())
        return
    }
    if len(req.BookIDs) == 0 {
        RespondWithBadRequest(c, "book_ids is required")
        return
    }

    onlyMissing := true
    if req.OnlyMissing != nil {
        onlyMissing = *req.OnlyMissing
    }

    opts := metafetch.FetchOptions{OnlyMissing: onlyMissing, Fields: req.Fields}
    results := make([]bulkFetchMetadataResult, 0, len(req.BookIDs))
    updatedCount := 0

    for _, bookID := range req.BookIDs {
        resp, err := s.metadataFetchService.FetchMetadataForBookWithOpts(bookID, opts)
        result := bulkFetchMetadataResult{BookID: bookID}
        switch {
        case errors.Is(err, metafetch.ErrBookNotFound):
            result.Status = "not_found"
            result.Message = "audiobook not found"
        case err != nil:
            result.Status = "error"
            result.Message = err.Error()
        case resp == nil || len(resp.Candidates) == 0:
            result.Status = "no_results"
        default:
            result.Status = "ok"
            result.Candidates = resp.Candidates
            updatedCount++
        }
        results = append(results, result)
    }

    RespondWithOK(c, gin.H{"results": results, "updated": updatedCount})
}
```

Adjust field names to match the actual response struct in `metafetch/service.go`. **Do not invent new exported types** — use what's there.

## Step 4 — Tests

Add or extend `internal/server/metadata_handlers_test.go`:

```go
func TestBulkFetchMetadata_DelegatesToService(t *testing.T) {
    // Build a fake metadataFetchService that records FetchMetadataForBookWithOpts calls.
    // POST to /api/v1/audiobooks/bulk-fetch-metadata with 3 book_ids.
    // Assert FetchMetadataForBookWithOpts was called exactly 3 times.
    // Assert the response shape contains 3 results.
}
```

If the service interface in the server package isn't easily mockable, use the `mockery`-generated mocks (run `make mocks` if needed). **Do not write a hand-rolled mock that duplicates an existing one.**

## Step 5 — Verify

```
go vet ./...
make test
make ci
```

Then verify the handler shrunk:
```
wc -l internal/server/metadata_handlers.go
```
Expect ~150 fewer lines than before (the deleted source-chain + cache logic).

## Step 6 — Commit

```
refactor(metadata): delegate bulkFetchMetadata to Service (TODO 4.17a)

- Removes ~150 LOC of duplicated source-chain iteration, per-source TTL cache
  lookup/write, and result scoring from the handler.
- Adds metafetch.FetchOptions{OnlyMissing, Fields} so the Service owns the
  behavior; the handler is now a thin orchestrator.
- All cache/retry/scoring behaviour now flows through the Service path.

Spec: docs/superpowers/specs/2026-04-27-fetch-path-audit-design.md
```

## Definition of done

- [ ] `make ci` green
- [ ] `bulkFetchMetadata` body has no `BuildSourceChain`, no `metadataFetchCache.Get`, no `metadataFetchCache.Put`, no inline TTL math (`grep` to confirm)
- [ ] Test asserts service is called per book ID
- [ ] CHANGELOG prepended
- [ ] TODO.md `4.17a` flipped to `[x]`

## When to STOP

NEEDS_REVIEW if:

- `metafetch.Service.FetchMetadataForBook` does NOT exist (sequence error: this task ran before the service existed).
- The existing Service signature would need more than `FetchOptions{OnlyMissing, Fields}` to cover the handler's behavior. Surface the gap; do not invent broader options.
- Removing the handler's logic would change observable response shape (e.g. error codes the frontend depends on). Diff the response carefully.
