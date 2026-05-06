<!-- file: docs/superpowers/bot-tasks/2026-04-29-user-ratings-api.md -->
<!-- version: 1.0.0 -->
<!-- guid: b8d41f3c-96e2-4a7b-c5d2-3f0e82b64d17 -->
<!-- last-edited: 2026-04-29 -->

# Bot Task: RATE-1 — PATCH /api/v1/audiobooks/:id/rating

## Task ID
RATE-1

## Summary
Add a `PATCH /api/v1/audiobooks/:id/rating` endpoint that lets callers set or clear user ratings (overall, story, performance) and notes on a book. The four DB columns already exist. The `Book` struct already has the fields. You are only adding a store method, a handler, a route, and tests.

## DO NOT DO THESE THINGS
- **DO NOT** modify the `Book` struct in `internal/database/store.go`. The fields are already there.
- **DO NOT** create any migration file. The columns already exist via `ensureExtendedBookColumns`.
- **DO NOT** rename or remove any existing functions.
- **DO NOT** touch any file not listed in this task.

---

## Step 1 — Add `UpdateBookRating` to the store interface

**File:** `internal/database/store.go`

Find the `Store` interface. It contains methods like `UpdateBook`, `GetBook`, `DeleteBook`, etc. Add the following method to the interface (in alphabetical order among the U methods, after `UpdateBook` and before any `Upsert` method):

```go
UpdateBookRating(ctx context.Context, id string, overall, story, performance *float64, notes *string) error
```

The full signature using the exact types:
- `ctx context.Context` — standard context
- `id string` — book UUID
- `overall *float64` — pointer; nil means "don't change this column"; non-nil sets the value (including to 0.0)
- `story *float64` — same semantics
- `performance *float64` — same semantics
- `notes *string` — pointer; nil means "don't change"; non-nil sets the value (empty string clears text but leaves column non-NULL; to set NULL pass a special sentinel — see handler section)

**Wait** — notes NULL vs empty string requires a different approach. Use `**float64` is not idiomatic in Go. Instead, use a wrapper struct for the partial-update logic in the handler, and pass explicit `sql.NullFloat64` / `sql.NullString` to the store. Revised signature:

```go
UpdateBookRating(ctx context.Context, id string, req UpdateBookRatingRequest) error
```

Where `UpdateBookRatingRequest` is a new struct you add to `internal/database/store.go`:

```go
// UpdateBookRatingRequest carries partial-update fields for user ratings.
// Each field uses a pointer so the caller can distinguish "omitted" (nil)
// from "set to zero/empty" (non-nil pointing to zero value).
// To clear a rating to NULL, set ClearOverall = true (etc.).
type UpdateBookRatingRequest struct {
    Overall        *float64
    ClearOverall   bool
    Story          *float64
    ClearStory     bool
    Performance    *float64
    ClearPerf      bool
    Notes          *string
    ClearNotes     bool
}
```

Add both the struct and the interface method to `internal/database/store.go`.

---

## Step 2 — Implement in SQLiteStore

**File:** `internal/database/sqlite_store.go`

Find the block where other `Update*` methods are implemented (search for `func (s *SQLiteStore) UpdateBook`). Add the following method immediately after it:

```go
func (s *SQLiteStore) UpdateBookRating(ctx context.Context, id string, req database.UpdateBookRatingRequest) error {
    setClauses := []string{}
    args := []interface{}{}

    if req.ClearOverall {
        setClauses = append(setClauses, "user_rating_overall = NULL")
    } else if req.Overall != nil {
        setClauses = append(setClauses, "user_rating_overall = ?")
        args = append(args, *req.Overall)
    }

    if req.ClearStory {
        setClauses = append(setClauses, "user_rating_story = NULL")
    } else if req.Story != nil {
        setClauses = append(setClauses, "user_rating_story = ?")
        args = append(args, *req.Story)
    }

    if req.ClearPerf {
        setClauses = append(setClauses, "user_rating_performance = NULL")
    } else if req.Performance != nil {
        setClauses = append(setClauses, "user_rating_performance = ?")
        args = append(args, *req.Performance)
    }

    if req.ClearNotes {
        setClauses = append(setClauses, "user_rating_notes = NULL")
    } else if req.Notes != nil {
        setClauses = append(setClauses, "user_rating_notes = ?")
        args = append(args, *req.Notes)
    }

    if len(setClauses) == 0 {
        return nil // nothing to do
    }

    query := "UPDATE books SET " + strings.Join(setClauses, ", ") + " WHERE id = ?"
    args = append(args, id)

    result, err := s.db.ExecContext(ctx, query, args...)
    if err != nil {
        return fmt.Errorf("UpdateBookRating: %w", err)
    }
    rows, err := result.RowsAffected()
    if err != nil {
        return fmt.Errorf("UpdateBookRating rows affected: %w", err)
    }
    if rows == 0 {
        return ErrNotFound
    }
    return nil
}
```

Make sure `strings` and `fmt` are already imported in `sqlite_store.go` (they will be). `ErrNotFound` is whatever sentinel the file already uses for not-found (search for `ErrNotFound` in the file and use that exact identifier).

---

## Step 3 — Implement in PebbleStore

**File:** `internal/database/pebble_store.go`

Find where other `Update*` methods are implemented (search for `func (s *PebbleStore) UpdateBook`). Add after it:

```go
func (s *PebbleStore) UpdateBookRating(ctx context.Context, id string, req database.UpdateBookRatingRequest) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    book, err := s.getBook(id)
    if err != nil {
        return err
    }

    if req.ClearOverall {
        book.UserRatingOverall = nil
    } else if req.Overall != nil {
        book.UserRatingOverall = req.Overall
    }

    if req.ClearStory {
        book.UserRatingStory = nil
    } else if req.Story != nil {
        book.UserRatingStory = req.Story
    }

    if req.ClearPerf {
        book.UserRatingPerformance = nil
    } else if req.Performance != nil {
        book.UserRatingPerformance = req.Performance
    }

    if req.ClearNotes {
        book.UserRatingNotes = nil
    } else if req.Notes != nil {
        book.UserRatingNotes = req.Notes
    }

    return s.putBook(book)
}
```

`getBook` and `putBook` are the internal helpers already used by other methods in `pebble_store.go`. If they have different names, search for them with `grep -n "func (s \*PebbleStore) get" internal/database/pebble_store.go` and use whatever names you find.

---

## Step 4 — Add to MockStore

**File:** `internal/database/mock_store.go` (or wherever the mock/test store is defined — search with `grep -rn "MockStore\|mockStore\|FakeStore" internal/database/`)

Add a method with the same signature. The mock implementation just records the call and returns nil:

```go
func (m *MockStore) UpdateBookRating(ctx context.Context, id string, req database.UpdateBookRatingRequest) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    // record the call if the mock tracks calls
    return m.UpdateBookRatingError // field you add to the mock struct
}
```

Also add `UpdateBookRatingError error` to the `MockStore` struct so tests can inject errors.

If the project uses a generated mock (e.g., with `mockgen`), add `UpdateBookRating` to the interface and regenerate. The regenerate command is typically in a `//go:generate` comment at the top of `mock_store.go` or in `Makefile`. Run it.

---

## Step 5 — Write the HTTP Handler

**File:** `internal/server/metadata_handlers.go`

### 5a. Add the request body struct

Near the top of the file (after the package declaration and imports), add:

```go
// ratingPatchRequest is the JSON body for PATCH /api/v1/audiobooks/:id/rating.
// Each field is a json.RawMessage so the handler can distinguish null (clear)
// from absent (don't touch) from a numeric value.
type ratingPatchRequest struct {
    Overall     json.RawMessage `json:"overall"`
    Story       json.RawMessage `json:"story"`
    Performance json.RawMessage `json:"performance"`
    Notes       json.RawMessage `json:"notes"`
}
```

### 5b. Add the validation helper

```go
// parseOptionalRating decodes a json.RawMessage into a *float64 and a clear flag.
// Returns (nil, false, nil) if raw is empty (field omitted).
// Returns (nil, true, nil) if raw is JSON null (clear).
// Returns (&v, false, nil) if raw is a valid number in [0,5] step 0.5.
// Returns (nil, false, err) on invalid value.
func parseOptionalRating(raw json.RawMessage, fieldName string) (*float64, bool, error) {
    if len(raw) == 0 {
        return nil, false, nil
    }
    if string(raw) == "null" {
        return nil, true, nil
    }
    var v float64
    if err := json.Unmarshal(raw, &v); err != nil {
        return nil, false, fmt.Errorf("%s: must be a number", fieldName)
    }
    if v < 0 || v > 5 {
        return nil, false, fmt.Errorf("%s: must be between 0 and 5", fieldName)
    }
    // check 0.5 step: v*2 must be an integer
    if math.Round(v*2) != v*2 {
        return nil, false, fmt.Errorf("%s: must be a multiple of 0.5", fieldName)
    }
    return &v, false, nil
}
```

Add `"math"` to the import block if it is not already there.

### 5c. Add the handler function

Search for `func (s *Server) handleUpdateBook` — add the new handler right after it:

```go
// handleUpdateBookRating handles PATCH /api/v1/audiobooks/:id/rating.
func (s *Server) handleUpdateBookRating(c *gin.Context) {
    id := c.Param("id")
    if id == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "missing book id"})
        return
    }

    var body ratingPatchRequest
    // Allow empty body (no changes)
    if c.Request.ContentLength != 0 {
        if err := c.ShouldBindJSON(&body); err != nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
            return
        }
    }

    req := database.UpdateBookRatingRequest{}

    if overall, clear, err := parseOptionalRating(body.Overall, "overall"); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    } else {
        req.Overall = overall
        req.ClearOverall = clear
    }

    if story, clear, err := parseOptionalRating(body.Story, "story"); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    } else {
        req.Story = story
        req.ClearStory = clear
    }

    if perf, clear, err := parseOptionalRating(body.Performance, "performance"); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    } else {
        req.Performance = perf
        req.ClearPerf = clear
    }

    // Notes: null clears, string sets, absent leaves alone
    if len(body.Notes) > 0 {
        if string(body.Notes) == "null" {
            req.ClearNotes = true
        } else {
            var notes string
            if err := json.Unmarshal(body.Notes, &notes); err != nil {
                c.JSON(http.StatusBadRequest, gin.H{"error": "notes: must be a string or null"})
                return
            }
            req.Notes = &notes
        }
    }

    ctx := c.Request.Context()
    if err := s.store.UpdateBookRating(ctx, id, req); err != nil {
        if errors.Is(err, database.ErrNotFound) {
            c.JSON(http.StatusNotFound, gin.H{"error": "book not found"})
            return
        }
        s.log.Errorf("UpdateBookRating %s: %v", id, err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
        return
    }

    // Return the updated book
    book, err := s.store.GetBook(ctx, id)
    if err != nil {
        s.log.Errorf("GetBook after rating update %s: %v", id, err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
        return
    }
    c.JSON(http.StatusOK, book)
}
```

Make sure `errors`, `encoding/json`, `net/http` are imported. They will already be in the file — just verify.

---

## Step 6 — Register the Route

**File:** `internal/server/server.go`

Search for where `handleUpdateBook` is registered. It will look like:

```go
audiobooks.PATCH("/:id", s.handleUpdateBook)
```

or similar (the exact router group name may vary — it's the group that handles `/api/v1/audiobooks`).

Add the new route **immediately after** the existing PATCH route for the book:

```go
audiobooks.PATCH("/:id/rating", s.handleUpdateBookRating)
```

Make sure this line is inside the same router group as the other audiobook routes, before the group's closing brace.

---

## Step 7 — Write Tests

**File:** `internal/server/metadata_handlers_test.go`

Add two test functions at the end of the file:

### Test 1: Happy path — set ratings

```go
func TestHandleUpdateBookRating_SetRatings(t *testing.T) {
    s, mock := newTestServer(t)  // use whatever helper already exists in the file

    bookID := "test-book-123"
    // Ensure GetBook returns a book after the update
    mock.Books[bookID] = &database.Book{ID: bookID, Title: "Test Book"}

    body := `{"overall": 4.5, "story": 4.0, "performance": 5.0, "notes": "Great!"}`
    req := httptest.NewRequest(http.MethodPatch, "/api/v1/audiobooks/"+bookID+"/rating", strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()

    s.ServeHTTP(w, req)

    assert.Equal(t, http.StatusOK, w.Code)

    var result database.Book
    require.NoError(t, json.NewDecoder(w.Body).Decode(&result))
    assert.Equal(t, bookID, result.ID)
}
```

### Test 2: Validation error — out of range

```go
func TestHandleUpdateBookRating_InvalidValue(t *testing.T) {
    s, _ := newTestServer(t)

    body := `{"overall": 6.0}`
    req := httptest.NewRequest(http.MethodPatch, "/api/v1/audiobooks/anything/rating", strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()

    s.ServeHTTP(w, req)

    assert.Equal(t, http.StatusBadRequest, w.Code)
    assert.Contains(t, w.Body.String(), "overall")
}
```

Note: `newTestServer(t)` is whatever test helper already exists in the test file. Search for it with `grep -n "func newTestServer\|func setupTestServer" internal/server/`. Use the exact name you find.

---

## Step 8 — Verify

Run:

```bash
go build ./...
go test ./internal/database/... ./internal/server/...
```

Both must pass with zero failures.

Also run the full test suite:

```bash
make test
```

Must pass.

---

## Step 9 — PR Instructions

1. Branch name: `feat/user-ratings-api`
2. Commit message: `feat(ratings): add PATCH /api/v1/audiobooks/:id/rating endpoint`
3. PR title: `feat(ratings): user rating PATCH endpoint (RATE-1)`
4. PR body must mention: "closes RATE-1", list the four files changed, note that no migration is needed.
5. Do NOT include frontend changes in this PR.
6. Do NOT squash commits.

---

## Checklist

- [ ] `UpdateBookRatingRequest` struct added to `internal/database/store.go`
- [ ] `UpdateBookRating` added to `Store` interface in `internal/database/store.go`
- [ ] `SQLiteStore.UpdateBookRating` implemented in `internal/database/sqlite_store.go`
- [ ] `PebbleStore.UpdateBookRating` implemented in `internal/database/pebble_store.go`
- [ ] Mock updated in `internal/database/mock_store.go`
- [ ] `ratingPatchRequest` struct added to `internal/server/metadata_handlers.go`
- [ ] `parseOptionalRating` helper added to `internal/server/metadata_handlers.go`
- [ ] `handleUpdateBookRating` handler added to `internal/server/metadata_handlers.go`
- [ ] Route registered in `internal/server/server.go`
- [ ] 2 tests added to `internal/server/metadata_handlers_test.go`
- [ ] `go build ./...` passes
- [ ] `make test` passes
