# Diagnostics Export & AI Analysis Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Diagnostics page that exports categorized library data as a ZIP and optionally submits it to OpenAI's batch API for automated dedup/error analysis, with an actionable review list for applying suggestions.

**Architecture:** New `DiagnosticsService` in the server package handles ZIP generation and AI batch management. A generic `DownloadBatchRaw` function in the AI package returns raw responses. The frontend adds a new Diagnostics page with category selection, export/submit buttons, and a results review panel.

**Tech Stack:** Go `archive/zip`, OpenAI batch API, React/TypeScript/MUI

**Spec:** `docs/superpowers/specs/2026-03-14-diagnostics-export-design.md`

---

## Chunk 1: Backend — DiagnosticsService & ZIP Export

### Task 1: Add generic batch download to AI package

**Files:**
- Modify: `internal/ai/openai_batch.go` (after line 283)
- Test: `internal/ai/openai_batch_test.go` (new test)

- [ ] **Step 1: Write the test for DownloadBatchRaw**

```go
func TestDownloadBatchRaw(t *testing.T) {
    // This tests the parsing logic, not the API call
    // We'll test with a mock response in integration tests
    raw := `{"id":"batch_1","custom_id":"chunk-000","response":{"body":{"choices":[{"message":{"content":"[{\"action\":\"merge_versions\"}]"}}]}}}`
    results, err := ParseBatchRawResults([]byte(raw))
    require.NoError(t, err)
    require.Len(t, results, 1)
    assert.Equal(t, "chunk-000", results[0].CustomID)
    assert.Contains(t, results[0].Content, "merge_versions")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ai/... -run TestDownloadBatchRaw -v`
Expected: FAIL — `ParseBatchRawResults` undefined

- [ ] **Step 3: Implement DownloadBatchRaw and ParseBatchRawResults**

In `internal/ai/openai_batch.go`, add after line 283:

```go
// BatchRawResult holds one response from a batch API result file.
type BatchRawResult struct {
    CustomID string `json:"custom_id"`
    Content  string `json:"content"`
    Error    string `json:"error,omitempty"`
}

// DownloadBatchRaw downloads batch results and returns raw response content.
func (p *Pipeline) DownloadBatchRaw(ctx context.Context, outputFileID string) ([]BatchRawResult, error) {
    content, err := p.client.Files.Content(ctx, outputFileID)
    if err != nil {
        return nil, fmt.Errorf("download batch output: %w", err)
    }
    defer content.Body.Close()
    data, err := io.ReadAll(content.Body)
    if err != nil {
        return nil, fmt.Errorf("read batch output: %w", err)
    }
    return ParseBatchRawResults(data)
}

// ParseBatchRawResults parses JSONL batch output into raw results.
func ParseBatchRawResults(data []byte) ([]BatchRawResult, error) {
    var results []BatchRawResult
    scanner := bufio.NewScanner(bytes.NewReader(data))
    scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
    for scanner.Scan() {
        line := scanner.Bytes()
        if len(line) == 0 {
            continue
        }
        var entry struct {
            CustomID string `json:"custom_id"`
            Response struct {
                Body struct {
                    Choices []struct {
                        Message struct {
                            Content string `json:"content"`
                        } `json:"message"`
                    } `json:"choices"`
                } `json:"body"`
            } `json:"response"`
            Error *struct {
                Message string `json:"message"`
            } `json:"error"`
        }
        if err := json.Unmarshal(line, &entry); err != nil {
            continue
        }
        result := BatchRawResult{CustomID: entry.CustomID}
        if entry.Error != nil {
            result.Error = entry.Error.Message
        } else if len(entry.Response.Body.Choices) > 0 {
            result.Content = entry.Response.Body.Choices[0].Message.Content
        }
        results = append(results, result)
    }
    return results, scanner.Err()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ai/... -run TestDownloadBatchRaw -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ai/openai_batch.go internal/ai/openai_batch_test.go
git commit -m "feat(ai): add generic DownloadBatchRaw for diagnostics batch results"
```

---

### Task 2: Create DiagnosticsService with ZIP export

**Files:**
- Create: `internal/server/diagnostics_service.go`
- Test: `internal/server/diagnostics_service_test.go`

- [ ] **Step 1: Write the test for ZIP generation**

```go
func TestDiagnosticsService_GenerateExport(t *testing.T) {
    store := dbmocks.NewMockStore(t)
    store.EXPECT().GetAllBooks(10000, 0).Return([]database.Book{
        {ID: "book1", Title: "Test Book"},
    }, nil).Maybe()
    store.EXPECT().GetAllBooks(10000, 10000).Return([]database.Book{}, nil).Maybe()
    store.EXPECT().GetAllAuthors().Return([]database.Author{}, nil).Maybe()
    store.EXPECT().GetAllSeries().Return([]database.Series{}, nil).Maybe()
    store.EXPECT().GetAllAuthorBookCounts().Return(map[int]int{}, nil).Maybe()
    store.EXPECT().GetAllSeriesBookCounts().Return(map[int]int{}, nil).Maybe()
    store.EXPECT().GetAllAuthorFileCounts().Return(map[int]int{}, nil).Maybe()
    store.EXPECT().GetAllSeriesFileCounts().Return(map[int]int{}, nil).Maybe()
    store.EXPECT().GetSystemActivityLogs("", 10000).Return(nil, nil).Maybe()
    store.EXPECT().GetRecentOperations(100).Return(nil, nil).Maybe()

    svc := NewDiagnosticsService(store, nil, "")
    zipPath, err := svc.GenerateExport("deduplication", "test export")
    require.NoError(t, err)
    defer os.Remove(zipPath)

    // Verify ZIP contents
    r, err := zip.OpenReader(zipPath)
    require.NoError(t, err)
    defer r.Close()

    fileNames := make(map[string]bool)
    for _, f := range r.File {
        fileNames[f.Name] = true
    }
    assert.True(t, fileNames["system_info.json"])
    assert.True(t, fileNames["books.json"])
    assert.True(t, fileNames["authors.json"])
    assert.True(t, fileNames["series.json"])
    assert.True(t, fileNames["batch.jsonl"])
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/... -run TestDiagnosticsService_GenerateExport -v`
Expected: FAIL — `NewDiagnosticsService` undefined

- [ ] **Step 3: Implement DiagnosticsService**

Create `internal/server/diagnostics_service.go` with:
- `DiagnosticsService` struct (fields: `db database.Store`, `aiPipeline *ai.Pipeline`, `itunesXMLPath string`)
- `NewDiagnosticsService(db, pipeline, itunesPath)` constructor
- `GenerateExport(category, description string) (zipPath string, err error)` — creates a temp ZIP file with:
  - `system_info.json` — calls `collectSystemInfo()`
  - `books.json` — paginates `GetAllBooks(10000, offset)` to get all books, slims fields
  - `authors.json` — calls `GetAllAuthors()` + `GetAllAuthorBookCounts()` + `GetAllAuthorFileCounts()`
  - `series.json` — calls `GetAllSeries()` + `GetAllSeriesBookCounts()` + `GetAllSeriesFileCounts()`
  - Category-specific files (logs, operations, itunes_albums, version_groups, missing_fields)
  - `batch.jsonl` — calls `buildBatchJSONL(category, description, books, ...)`
- Helper: `collectSystemInfo()` — runtime.GOOS, app version, DB counts
- Helper: `buildBatchJSONL(category, description, books, itunesAlbums)` — chunks data, writes JSONL with category-specific system prompts
- Helper: `buildVersionGroups(books)` — groups books by VersionGroupID in memory
- Uses `archive/zip` to write all files into the ZIP

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/server/... -run TestDiagnosticsService_GenerateExport -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/server/diagnostics_service.go internal/server/diagnostics_service_test.go
git commit -m "feat: add DiagnosticsService with ZIP export generation"
```

---

### Task 3: Extract merge logic into service function

**Files:**
- Modify: `internal/server/server.go` (line ~1686, extract from `mergeBookDuplicatesAsVersions`)
- Create: `internal/server/merge_service.go`
- Test: `internal/server/merge_service_test.go`

- [ ] **Step 1: Write test for service-layer merge**

```go
func TestMergeService_MergeBooks(t *testing.T) {
    server, cleanup := setupTestServer(t)
    defer cleanup()

    // Create two books
    book1 := &database.Book{ID: "book-1", Title: "Test", FilePath: "/test/book.m4b"}
    book2 := &database.Book{ID: "book-2", Title: "Test", FilePath: "/test/book.mp3"}
    database.GlobalStore.CreateBook(book1)
    database.GlobalStore.CreateBook(book2)

    ms := NewMergeService(database.GlobalStore)
    result, err := ms.MergeBooks([]string{"book-1", "book-2"}, "book-1")
    require.NoError(t, err)
    assert.Equal(t, "book-1", result.PrimaryID)
    assert.NotEmpty(t, result.VersionGroupID)

    // Verify books are now linked
    b1, _ := database.GlobalStore.GetBookByID("book-1")
    b2, _ := database.GlobalStore.GetBookByID("book-2")
    assert.True(t, *b1.IsPrimaryVersion)
    assert.False(t, *b2.IsPrimaryVersion)
    assert.Equal(t, *b1.VersionGroupID, *b2.VersionGroupID)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/... -run TestMergeService_MergeBooks -v`
Expected: FAIL — `NewMergeService` undefined

- [ ] **Step 3: Extract merge logic into MergeService**

Create `internal/server/merge_service.go`:
- `MergeService` struct with `db database.Store`
- `MergeResult` struct: `PrimaryID string`, `VersionGroupID string`, `MergedCount int`
- `MergeBooks(bookIDs []string, primaryID string) (*MergeResult, error)` — extracted from `mergeBookDuplicatesAsVersions` handler logic
- Refactor `mergeBookDuplicatesAsVersions` in server.go to call `MergeService.MergeBooks`

- [ ] **Step 4: Run test and full server tests**

Run: `go test ./internal/server/... -run TestMergeService -v && go test ./internal/server/... -count=1`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/server/merge_service.go internal/server/merge_service_test.go internal/server/server.go
git commit -m "refactor: extract merge logic into MergeService for reuse by diagnostics"
```

---

### Task 4: Add diagnostics API endpoints

**Files:**
- Modify: `internal/server/server.go` (route registration at line ~1170, new handler functions)
- Test: `internal/server/diagnostics_service_test.go` (add HTTP handler tests)

- [ ] **Step 1: Write test for export endpoint**

```go
func TestDiagnosticsExportEndpoint(t *testing.T) {
    server, cleanup := setupTestServer(t)
    defer cleanup()

    body := `{"category":"deduplication","description":"test"}`
    req := httptest.NewRequest(http.MethodPost, "/api/v1/diagnostics/export", strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()
    server.router.ServeHTTP(w, req)

    require.Equal(t, http.StatusAccepted, w.Code)
    var resp map[string]interface{}
    json.Unmarshal(w.Body.Bytes(), &resp)
    assert.NotEmpty(t, resp["operation_id"])
    assert.Equal(t, "generating", resp["status"])
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/... -run TestDiagnosticsExportEndpoint -v`

- [ ] **Step 3: Register routes and implement handlers**

In `server.go` route registration block (~line 1170), add:
```go
protected.POST("/diagnostics/export", s.startDiagnosticsExport)
protected.GET("/diagnostics/export/:operationId/download", s.downloadDiagnosticsExport)
protected.POST("/diagnostics/submit-ai", s.submitDiagnosticsAI)
protected.GET("/diagnostics/ai-results/:operationId", s.getDiagnosticsAIResults)
protected.POST("/diagnostics/apply-suggestions", s.applyDiagnosticsSuggestions)
```

Implement handlers:
- `startDiagnosticsExport` — parses body, creates Operation, enqueues `GenerateExport` via GlobalQueue, returns operation ID
- `downloadDiagnosticsExport` — reads operation result_data for ZIP path, streams the file
- `submitDiagnosticsAI` — generates export, extracts batch.jsonl from ZIP, uploads to OpenAI, creates polling operation
- `getDiagnosticsAIResults` — reads operation result_data, parses suggestions JSON
- `applyDiagnosticsSuggestions` — loops approved suggestions, calls MergeService/UpdateBook/soft-delete as appropriate

- [ ] **Step 4: Run test and full server tests**

Run: `go test ./internal/server/... -run TestDiagnostics -v && go test ./internal/server/... -count=1`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/server/server.go internal/server/diagnostics_service.go internal/server/diagnostics_service_test.go
git commit -m "feat(api): add diagnostics export, AI submit, and apply-suggestions endpoints"
```

---

### Task 5: Add batch JSONL builder with category-specific prompts

**Files:**
- Modify: `internal/server/diagnostics_service.go` (add `buildBatchJSONL`)
- Test: `internal/server/diagnostics_service_test.go`

- [ ] **Step 1: Write test for JSONL generation**

```go
func TestBuildBatchJSONL_Dedup(t *testing.T) {
    books := []slimBook{
        {ID: "b1", Title: "Book One", Author: "Author A"},
        {ID: "b2", Title: "Book One", Author: "Author A"},
    }
    data, err := buildBatchJSONL("deduplication", "find dupes", books, nil)
    require.NoError(t, err)

    // Parse JSONL lines
    lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
    assert.GreaterOrEqual(t, len(lines), 1)

    var req map[string]interface{}
    json.Unmarshal(lines[0], &req)
    assert.Equal(t, "POST", req["method"])
    assert.Equal(t, "/v1/chat/completions", req["url"])
    assert.Contains(t, req["custom_id"], "chunk-")
}
```

- [ ] **Step 2: Run test, verify fail, implement, verify pass**

- [ ] **Step 3: Implement `buildBatchJSONL`**

Handles all four categories:
- **deduplication:** system prompt focused on finding duplicates, orphan tracks, missing merges. Chunks books by 500.
- **error_analysis:** system prompt focused on log error patterns, root causes. Chunks logs by 200.
- **metadata_quality:** system prompt focused on missing fields, narrator-as-author, garbled titles. Chunks books by 500.
- **general:** combines all prompts.

Each chunk becomes one JSONL line with `custom_id`, `method`, `url`, `body` (model, messages, max_tokens, temperature).

- [ ] **Step 4: Commit**

```bash
git add internal/server/diagnostics_service.go internal/server/diagnostics_service_test.go
git commit -m "feat: add category-specific JSONL batch builder for diagnostics"
```

---

## Chunk 2: Frontend — Diagnostics Page

### Task 6: Add Diagnostics page with category selection

**Files:**
- Create: `web/src/pages/Diagnostics.tsx`
- Modify: `web/src/App.tsx` (add route, ~line 175)
- Modify: `web/src/services/api.ts` (add API functions)

- [ ] **Step 1: Add API functions**

In `web/src/services/api.ts`, add:
```typescript
export async function startDiagnosticsExport(
  category: string, description: string
): Promise<{ operation_id: string; status: string }> { ... }

export async function downloadDiagnosticsExport(operationId: string): Promise<Blob> { ... }

export async function submitDiagnosticsAI(
  category: string, description: string
): Promise<{ operation_id: string; batch_id: string; status: string; request_count: number }> { ... }

export async function getDiagnosticsAIResults(operationId: string): Promise<DiagnosticsAIResults> { ... }

export async function applyDiagnosticsSuggestions(
  operationId: string, approvedIds: string[]
): Promise<{ applied: number; failed: number; errors: string[] }> { ... }
```

- [ ] **Step 2: Create Diagnostics.tsx**

Build the page with:
- Category selector (4 radio cards: Error Analysis, Deduplication, Metadata Quality, General)
- Description textarea
- Two action buttons: "Download ZIP" and "Submit to AI"
- Loading state while generating
- Poll operation status until complete

- [ ] **Step 3: Add route in App.tsx**

```tsx
<Route path="/diagnostics" element={<Diagnostics />} />
```

Add nav link in the sidebar/nav component.

- [ ] **Step 4: Test manually in dev mode**

Run: `make web-dev` and navigate to `/diagnostics`
Expected: Page renders with category cards and buttons

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/Diagnostics.tsx web/src/App.tsx web/src/services/api.ts
git commit -m "feat(ui): add Diagnostics page with category selection and export"
```

---

### Task 7: Add AI results review panel

**Files:**
- Modify: `web/src/pages/Diagnostics.tsx`

- [ ] **Step 1: Add results panel component**

After AI batch completes, show:
- Grouped suggestion list (Merge Versions / Delete Orphans / Fix Metadata / Reassign Series)
- Each suggestion: checkbox, affected book titles, reason, proposed fix
- "Select All" / "Deselect All" per group
- "Apply Selected" button with confirmation dialog
- "View Raw" toggle showing raw JSON in a code block

- [ ] **Step 2: Add polling logic**

When "Submit to AI" is clicked:
1. Call `submitDiagnosticsAI`
2. Show progress with operation status polling (every 10s)
3. When completed, call `getDiagnosticsAIResults` and render the review panel

- [ ] **Step 3: Add apply logic**

"Apply Selected" button:
1. Collect checked suggestion IDs
2. Call `applyDiagnosticsSuggestions`
3. Show success/error toast
4. Refresh results to show applied status

- [ ] **Step 4: Commit**

```bash
git add web/src/pages/Diagnostics.tsx
git commit -m "feat(ui): add AI results review panel with approve/reject and apply"
```

---

## Chunk 3: E2E Tests

### Task 8: Write E2E tests for Diagnostics page

**Files:**
- Create: `web/tests/e2e/diagnostics.spec.ts`

- [ ] **Step 1: Write E2E tests**

Tests using Phase 2 (mocked routes):
1. **Category selection renders all 4 options**
2. **Download ZIP**: select category → click Download → verify blob download triggered
3. **Submit to AI**: select category → click Submit → verify progress polling → mock completion → verify results panel renders
4. **Results review**: mock AI results → verify suggestion grouping → check/uncheck suggestions → click Apply → verify API call
5. **View Raw toggle**: click toggle → verify raw JSON displayed
6. **Error handling**: mock API error → verify error toast

- [ ] **Step 2: Run E2E tests**

Run: `make test-e2e -- --grep Diagnostics`
Expected: All PASS

- [ ] **Step 3: Commit**

```bash
git add web/tests/e2e/diagnostics.spec.ts
git commit -m "test(e2e): add Diagnostics page tests for export and AI analysis"
```

---

### Task 9: Final integration test and deploy

- [ ] **Step 1: Run full test suite**

```bash
go test ./... && make test-all
```

- [ ] **Step 2: Build and verify**

```bash
make build
```

- [ ] **Step 3: Deploy**

```bash
make deploy
```

- [ ] **Step 4: Final commit with any fixes**

```bash
git push origin main
```
