<!-- file: docs/superpowers/bot-tasks/2026-04-30-ctx-2-openlibrary.md -->
<!-- version: 1.0.0 -->
<!-- guid: b8c9d0e1-f2a3-4567-bcde-890123456fa7 -->
<!-- last-edited: 2026-04-30 -->

# BOT TASK: CTX-2 — Thread Context Through OpenLibrary Client

**TODO ID:** CTX-2
**Audience:** burndown bot
**Branch:** `fix/ctx-openlibrary-service`
**PR title:** `fix(metadata): thread context through OpenLibrary client`

---

## What This Task Does

Replaces `context.Background()` calls in the Open Library API client
(`internal/metadata/openlibrary.go` or equivalent) with the caller-supplied
context, so HTTP requests to Open Library are cancellable when the originating
request is cancelled.

---

## What NOT to Do

- **Do NOT change** the Open Library API URL or response parsing logic.
- **Do NOT use** `context.TODO()` — use the actual caller context.
- **Do NOT add** a hardcoded timeout inside the function — the caller should set
  the timeout via context if needed.
- **Do NOT change** the mock in the test file yet — adjust mocks as needed.

---

## Read First

1. Find the Open Library client:

```bash
find /Users/jdfalk/.worktrees/audiobook-eval/internal -name '*.go' \
  | xargs grep -l 'open.?library\|openlibrary\|OpenLibrary' 2>/dev/null
```

2. Read the file. Find all HTTP calls (`http.Get`, `http.NewRequest`, `client.Do`).
3. Check if the functions already accept `ctx context.Context` — if not, they need
   to be updated.

---

## Steps

### Step 1 — Find HTTP calls without context

```bash
grep -n 'http\.Get\|http\.NewRequest\|context\.Background' \
  internal/metadata/ -r | head -20
```

`http.Get` does not accept a context. It must be replaced with:
```go
req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
if err != nil {
    return nil, fmt.Errorf("new request: %w", err)
}
resp, err := client.Do(req)
```

### Step 2 — Update function signatures

For each function that makes an HTTP call:
```go
// Before:
func (c *OpenLibraryClient) SearchBooks(query string) ([]Book, error) {

// After:
func (c *OpenLibraryClient) SearchBooks(ctx context.Context, query string) ([]Book, error) {
```

Replace `http.Get(url)` with `http.NewRequestWithContext(ctx, ...)` + `client.Do`.

### Step 3 — Update callers

```bash
grep -rn 'SearchBooks\|FetchMetadata\|OpenLibrary' internal/server/ cmd/ | head -20
```

Update each caller to pass the request context.

### Step 4 — Verify

```bash
cd /Users/jdfalk/.worktrees/audiobook-eval
go build ./...
go vet ./...
go test ./internal/metadata/... -v 2>&1 | tail -20
```

### Step 5 — Commit and open PR

```bash
git checkout -b fix/ctx-openlibrary-service
git add internal/metadata/
git commit -m "fix(metadata): thread context through OpenLibrary client

Replaces context.Background()/http.Get with http.NewRequestWithContext
in the OpenLibrary client. HTTP calls to the external API now respect
request cancellation and caller-set deadlines.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin fix/ctx-openlibrary-service
gh pr create \
  --title "fix(metadata): thread context through OpenLibrary client" \
  --body "Enables cancellation of in-flight Open Library HTTP requests. Replaces http.Get with http.NewRequestWithContext. Context fix CTX-2."
```

---

## Checklist

- [ ] No `http.Get(url)` calls remain in the Open Library client (replaced with `NewRequestWithContext`)
- [ ] All public methods accept `ctx context.Context` as first parameter
- [ ] No `context.Background()` in request-path Open Library methods
- [ ] Callers updated to pass request context
- [ ] `go build ./...` passes
- [ ] `go test ./internal/metadata/...` passes
- [ ] PR opened with correct branch and title
