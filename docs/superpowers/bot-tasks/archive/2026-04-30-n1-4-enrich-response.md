<!-- file: docs/superpowers/bot-tasks/2026-04-30-n1-4-enrich-response.md -->
<!-- version: 1.0.0 -->
<!-- guid: b6c7d8e9-f0a1-2345-bcde-678901234fab -->
<!-- last-edited: 2026-04-30 -->

# BOT TASK: N1-4 — Wire Batch Fetch into enrichBookForResponse

**TODO ID:** N1-4
**Audience:** burndown bot
**Branch:** `perf/n1-enrich-response`
**PR title:** `perf(server): eliminate N+1 queries in enrichBookForResponse`

**Prerequisite:** N1-2 AND N1-3 must both be merged first.

---

## What This Task Does

Refactors `enrichBookForResponse` (`internal/server/server.go:334–406`) and
`EnrichAudiobooksWithNames` (`internal/server/audiobook_service.go:782`) to use
the new batch-fetch methods instead of per-book DB calls. The JSON response shape
must not change.

---

## What NOT to Do

- **Do NOT change** the JSON response structure or field names.
- **Do NOT remove** `enrichBookForResponse` — only change how it gets its data.
- **Do NOT change** the DB layer — N1-2 and N1-3 cover that.
- **Do NOT use** `context.Background()` — use the request context from the handler.

---

## Read First

1. `internal/server/server.go:334–406` — read `enrichBookForResponse` fully.
   Identify every DB call it makes per book. Focus on author/narrator fetches.
2. `internal/server/audiobook_service.go:782` — read `EnrichAudiobooksWithNames`.
   Identify the per-book loop with DB calls.
3. Find the call sites where `enrichBookForResponse` is called in a loop — this is
   where the pre-fetch must be inserted.
4. `internal/database/store.go` — confirm the signatures of `GetAuthorsByBookIDs`
   and `GetNarratorsByBookIDs` (from N1-1).

---

## Steps

### Step 1 — Locate the list-response loop

Search for all call sites of `enrichBookForResponse`:

```bash
grep -n 'enrichBookForResponse' internal/server/server.go | head -20
```

Find the outer loop that calls it for each book in a page result. It will look
roughly like:

```go
for _, book := range books {
    enrichedBook := enrichBookForResponse(ctx, s, book)
    results = append(results, enrichedBook)
}
```

### Step 2 — Pre-fetch before the loop

Before the loop, collect book IDs and batch-fetch authors and narrators:

```go
bookIDs := make([]string, len(books))
for i, b := range books { bookIDs[i] = b.ID }

authorsByBook, err := s.store.GetAuthorsByBookIDs(ctx, bookIDs)
if err != nil {
    log.Printf("GetAuthorsByBookIDs: %v", err)
    authorsByBook = map[string][]database.Author{}
}
narratorsByBook, err := s.store.GetNarratorsByBookIDs(ctx, bookIDs)
if err != nil {
    log.Printf("GetNarratorsByBookIDs: %v", err)
    narratorsByBook = map[string][]database.Narrator{}
}
```

### Step 3 — Pass maps into enrichBookForResponse

Change the signature of `enrichBookForResponse` to accept the pre-fetched maps:

```go
func enrichBookForResponse(
    ctx context.Context,
    s *Server,
    book database.Book,
    authorsByBook map[string][]database.Author,
    narratorsByBook map[string][]database.Narrator,
) ResponseBook {
```

Inside `enrichBookForResponse`, replace the per-book author DB call with:
```go
authors := authorsByBook[book.ID]  // O(1) map lookup, no DB call
```

And the narrator DB call:
```go
narrators := narratorsByBook[book.ID]
```

Update all call sites of `enrichBookForResponse` to pass the maps. For any call
site that is NOT inside a page-level loop (e.g., a single-book endpoint), pass
pre-fetched maps built from a single-element call.

### Step 4 — Apply the same fix to EnrichAudiobooksWithNames

Open `internal/server/audiobook_service.go:782`. Apply the same pattern:
pre-fetch before the loop, use map lookup inside.

### Step 5 — Verify

```bash
cd /Users/jdfalk/.worktrees/audiobook-eval
go test ./internal/server/... -v 2>&1 | tail -30
go vet ./...
```

Both must pass.

### Step 6 — Commit and open PR

```bash
git checkout -b perf/n1-enrich-response
git add internal/server/server.go internal/server/audiobook_service.go
git commit -m "perf(server): eliminate N+1 queries in enrichBookForResponse

Pre-fetches authors and narrators for the full page using
GetAuthorsByBookIDs / GetNarratorsByBookIDs before the per-book loop.
Reduces ~4,000 DB round-trips to 2 for a 1,000-book page.
Response JSON shape is unchanged.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin perf/n1-enrich-response
gh pr create \
  --title "perf(server): eliminate N+1 queries in enrichBookForResponse" \
  --body "Eliminates N+1 author/narrator DB queries in list endpoints. Pre-fetches via GetAuthorsByBookIDs and GetNarratorsByBookIDs (N1-2/N1-3). No API change. Depends on N1-2 and N1-3."
```

---

## Checklist

- [ ] `enrichBookForResponse` no longer calls per-book author/narrator DB methods
- [ ] `EnrichAudiobooksWithNames` no longer loops with per-book DB calls
- [ ] Pre-fetch happens before the page-level loop
- [ ] Response JSON shape is unchanged
- [ ] `go test ./internal/server/...` passes
- [ ] `go vet ./...` is clean
- [ ] PR opened with correct branch and title
