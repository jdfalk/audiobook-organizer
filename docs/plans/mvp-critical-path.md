<!-- file: docs/plans/mvp-critical-path.md -->
<!-- version: 2.0.0 -->
<!-- guid: b1c2d3e4-f5a6-7b8c-9d0e-1f2a3b4c5d6e -->
<!-- last-edited: 2026-01-31 -->

# MVP Critical Path

## Overview

Items blocking or directly gating the MVP release. iTunes integration is the
primary blocker; all other P0 items can proceed in parallel. See individual
feature plan docs for detailed designs.

**Current MVP completion: ~85%**

- Backend: ~95%
- Frontend: ~80%
- Testing: Go 100% pass (86.2% coverage), Frontend 92% coverage, all passing

---

## P0 — Must Complete Before Release

### Manual QA & Validation

Execute the manual validation checklist across all core workflows. The server
runs on port 8080 by default; all paths below assume `http://localhost:8080`.

#### 1. Library — Search, Sort, Import Path CRUD, Scan

| Step | URL / Action | What to verify |
|---|---|---|
| 1a | `GET /api/v1/audiobooks?limit=24&offset=0` | Response contains `{"items":[…], "total": N}`. `items` is an array of `Book` objects with at least `id`, `title`, `file_path`, `library_state`. |
| 1b | `GET /api/v1/audiobooks?search=sanderson&limit=10` | Only books whose title, author, or series contains "sanderson" (case-insensitive) are returned. `total` reflects filtered count. |
| 1c | `GET /api/v1/import-paths` | Returns `{"importPaths":[…]}`. Each entry has `id`, `path`, `name`, `enabled`, `book_count`. |
| 1d | `POST /api/v1/import-paths` with `{"path":"/tmp/test-import","name":"Test"}` | 201 Created. Returned object has an auto-assigned `id`. |
| 1e | `DELETE /api/v1/import-paths/<id>` | 200 OK. Re-list confirms entry is gone. |
| 1f | `POST /api/v1/operations/scan` with `{"folder_path":"/tmp/test-import"}` | 202 Accepted. Response is an `Operation` object with `status:"queued"` or `"running"`, and a non-empty `id`. Poll `GET /api/v1/operations/<id>/status` until `status` is `"completed"`. Verify `progress == total`. |

Expected `GET /api/v1/audiobooks` response shape:
```json
{
  "items": [
    {
      "id": "01HXZABC...",
      "title": "The Way of Kings",
      "file_path": "/library/...",
      "library_state": "organized",
      "file_hash": "sha256hex...",
      "created_at": "2026-01-01T00:00:00Z",
      "updated_at": "2026-01-01T00:00:00Z"
    }
  ],
  "total": 42
}
```

#### 2. Book Detail — Tabs, Metadata, Soft Delete

| Step | URL / Action | What to verify |
|---|---|---|
| 2a | `GET /api/v1/audiobooks/<id>` | Full book object including optional `author`, `series`, `metadata_provenance` map. `metadata_provenance` keys include `title`, `author_name`, `narrator`, each with `effective_value` and `effective_source` (one of `file`, `fetched`, `stored`, `override`). |
| 2b | `GET /api/v1/audiobooks/<id>/tags` | Returns `{"tags":[…], "media":[…], "file_count": N, "duration": N}`. |
| 2c | `POST /api/v1/audiobooks/<id>/fetch-metadata` with `{}` | 200 OK. Response includes `{"message":"…","book":{…},"source":"…"}`. Provenance fields on subsequent GET of the book should now show `fetched_value` populated. |
| 2d | `DELETE /api/v1/audiobooks/<id>` | 200 OK. Re-fetch the book; `marked_for_deletion` must be `true`. |
| 2e | `POST /api/v1/audiobooks/<id>/restore` | 200 OK. Re-fetch; `marked_for_deletion` must be `false`. |
| 2f | `POST /api/v1/blocked-hashes` with `{"hash":"<file_hash_from_2a>","reason":"test block"}` | 201 Created. `GET /api/v1/blocked-hashes` must include the hash. |
| 2g | `DELETE /api/v1/blocked-hashes/<hash>` | 200 OK. Confirm removal via list. |

#### 3. Settings — Blocked Hashes, Config, System Info

| Step | URL / Action | What to verify |
|---|---|---|
| 3a | `GET /api/v1/config` | Returns `{"config":{…}}`. All secret fields (`openai_api_key`, `api_keys.goodreads`) are masked as `***XXXX`. |
| 3b | `PUT /api/v1/config` with `{"log_level":"debug"}` | 200 OK. Subsequent GET returns `log_level:"debug"`. |
| 3c | `GET /api/v1/system/status` | Contains `library.book_count`, `library.total_size`, `import_paths.folder_count`, `memory.*`, `runtime.go_version`, `operations.recent`. All numeric fields must be non-negative. |

#### 4. Dashboard — Stats, Navigation

| Step | URL / Action | What to verify |
|---|---|---|
| 4a | `GET /api/v1/dashboard` | Returns aggregate stats. `total_books` matches `GET /api/v1/audiobooks/count`. |

#### 5. State Transitions: import -> organized -> deleted -> purged

| Step | URL / Action | What to verify |
|---|---|---|
| 5a | Import a file via `POST /api/v1/import/file` with `{"file_path":"/path/to/book.m4b"}` | Book created with `library_state:"import"`. |
| 5b | `POST /api/v1/operations/organize` with `{"book_ids":["<id_from_5a>"]}` | After completion, book's `library_state` becomes `"organized"` and `organized_file_hash` is populated. |
| 5c | `DELETE /api/v1/audiobooks/<id>` | `marked_for_deletion` becomes `true`. |
| 5d | `GET /api/v1/audiobooks/soft-deleted` | Book appears in the list. |
| 5e | `DELETE /api/v1/audiobooks/purge-soft-deleted` | Book no longer appears anywhere. |

#### 6. Version Management

| Step | URL / Action | What to verify |
|---|---|---|
| 6a | `GET /api/v1/audiobooks/<id>/versions` | Returns `{"versions":[…]}`. If no version group exists, list is empty or contains only the book itself. |
| 6b | `POST /api/v1/audiobooks/<id>/versions` with `{"other_id":"<second_book_id>"}` | 200 OK. Both books now share the same `version_group_id`. |
| 6c | `PUT /api/v1/audiobooks/<id>/set-primary` | 200 OK. The targeted book has `is_primary_version:true`; all others in the group have `false`. |

#### 7. Duplicate Detection

| Step | URL / Action | What to verify |
|---|---|---|
| 7a | `GET /api/v1/audiobooks/duplicates` | Returns groups of books sharing the same `file_hash`. Each group is an array of at least 2 books. If no duplicates exist, the response is an empty array. |

### Release Pipeline Fixes

- Replace prerelease workflow token with one that has `contents:write` (or
  use PAT)
- Confirm GoReleaser publish works end-to-end
- Verify Docker frontend build succeeds (Vitest globals / node types fix)
- Replace local changelog stub with real generator once GHCOMMON sync is
  complete

### Test Coverage Expansion — Go 60% Minimum

Current Go coverage is ~25% (threshold temporarily lowered to 0 to unblock
merges). Raise to 60% minimum by adding tests for the items below. All tests
live in `package server` (same package as production code) so they have access
to unexported helpers like `setupTestServer`, `stringPtr`, `boolPtr`, etc.

**Verification commands:**
```bash
# Run all tests (including mock-tagged tests):
make test                        # go test ./... -v -race

# Generate coverage report:
make coverage                    # writes coverage.out + coverage.html

# Check 60% threshold (update Makefile threshold from 80 to 60 for now):
make coverage-check

# Run only the server package with coverage detail:
go test ./internal/server/... -coverprofile=server.out -covermode=atomic -v -race
go tool cover -func=server.out | sort -t: -k2 -n | tail -30

# Run mock-tagged tests explicitly (required for tests using dbmocks/queuemocks):
go test -tags mocks ./internal/server/... -v -race
```

#### A. Server Handlers — Organize, Scan, Metadata Operations

**Package:** `github.com/jdfalk/audiobook-organizer/internal/server`
**File to create/extend:** `internal/server/server_organize_test.go`

The project uses two test styles that may be combined:

1. **Integration style** (no build tag): Uses `setupTestServer(t)` which spins
   up a real SQLite store in a temp directory. Good for verifying full
   request-to-response round trips. See `internal/server/server_test.go` and
   `internal/server/server_coverage_test.go` for examples.

2. **Mock style** (build tag `//go:build mocks`): Replaces `database.GlobalStore`
   and `operations.GlobalQueue` with mockery-generated mocks from
   `internal/database/mocks` and `internal/operations/mocks`. Good for isolating
   a single handler and asserting exact Store method calls. See
   `internal/server/server_operations_test.go` for the canonical example.

**Functions/handlers that need tests:**

| Handler | Route | Key behaviors to test |
|---|---|---|
| `startOrganize` | `POST /api/v1/operations/organize` | (a) Missing queue returns 500. (b) Valid `book_ids` array enqueues operation, returns 202. (c) Empty body (organize all) still succeeds. |
| `startScan` | `POST /api/v1/operations/scan` | Already partially covered. Add: (a) `folder_path` pointing to non-existent dir returns 400. (b) Scan with valid path creates operation and enqueues. |
| `fetchAudiobookMetadata` | `POST /api/v1/audiobooks/:id/fetch-metadata` | (a) Non-existent book ID returns 404. (b) Successful fetch populates provenance. (c) Book with `fetch_metadata_error` flag returns 500. |
| `countAudiobooks` | `GET /api/v1/audiobooks/count` | Already covered. Verify count matches after insert/delete cycle. |
| `listDuplicateAudiobooks` | `GET /api/v1/audiobooks/duplicates` | (a) Empty DB returns empty array. (b) Two books with same `file_hash` appear as a group. |

**Code sample — mock-style test for `startOrganize` (file:
`internal/server/server_organize_test.go`):**

```go
//go:build mocks

package server

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/gin-gonic/gin"
    "github.com/jdfalk/audiobook-organizer/internal/database"
    dbmocks "github.com/jdfalk/audiobook-organizer/internal/database/mocks"
    "github.com/jdfalk/audiobook-organizer/internal/operations"
    queuemocks "github.com/jdfalk/audiobook-organizer/internal/operations/mocks"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
    "github.com/stretchr/testify/require"
)

func TestStartOrganize_QueueNil_Returns500(t *testing.T) {
    gin.SetMode(gin.TestMode)

    mockStore := dbmocks.NewMockStore(t)
    database.GlobalStore = mockStore
    operations.GlobalQueue = nil // simulate uninitialized queue

    srv := &Server{router: gin.New()}
    srv.setupRoutes()

    req := httptest.NewRequest(http.MethodPost, "/api/v1/operations/organize", bytes.NewBufferString("{}"))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()
    srv.router.ServeHTTP(w, req)

    assert.Equal(t, http.StatusInternalServerError, w.Code)
    assert.Contains(t, w.Body.String(), "operation queue not initialized")
}

func TestStartOrganize_WithBookIDs_ReturnsAccepted(t *testing.T) {
    gin.SetMode(gin.TestMode)

    mockStore := dbmocks.NewMockStore(t)
    database.GlobalStore = mockStore
    mockQueue := queuemocks.NewMockQueue(t)
    operations.GlobalQueue = mockQueue

    body := map[string]interface{}{
        "book_ids": []string{"01HXZABC123", "01HXZABC456"},
    }
    buf, _ := json.Marshal(body)

    returnedOp := &database.Operation{ID: "op-org-1", Type: "organize"}
    mockStore.EXPECT().CreateOperation(mock.Anything, "organize", (*string)(nil)).
        Return(returnedOp, nil).Once()
    mockQueue.EXPECT().Enqueue("op-org-1", "organize", operations.PriorityNormal, mock.Anything).
        Return(nil).Once()

    srv := &Server{router: gin.New()}
    srv.setupRoutes()

    req := httptest.NewRequest(http.MethodPost, "/api/v1/operations/organize", bytes.NewBuffer(buf))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()
    srv.router.ServeHTTP(w, req)

    assert.Equal(t, http.StatusAccepted, w.Code)
    var resp database.Operation
    require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
    assert.Equal(t, "op-org-1", resp.ID)
}

func TestStartOrganize_EnqueueError_Returns500(t *testing.T) {
    gin.SetMode(gin.TestMode)

    mockStore := dbmocks.NewMockStore(t)
    database.GlobalStore = mockStore
    mockQueue := queuemocks.NewMockQueue(t)
    operations.GlobalQueue = mockQueue

    returnedOp := &database.Operation{ID: "op-org-err", Type: "organize"}
    mockStore.EXPECT().CreateOperation(mock.Anything, "organize", (*string)(nil)).
        Return(returnedOp, nil).Once()
    mockQueue.EXPECT().Enqueue("op-org-err", "organize", operations.PriorityNormal, mock.Anything).
        Return(assert.AnError).Once()

    srv := &Server{router: gin.New()}
    srv.setupRoutes()

    req := httptest.NewRequest(http.MethodPost, "/api/v1/operations/organize", bytes.NewBufferString("{}"))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()
    srv.router.ServeHTTP(w, req)

    assert.Equal(t, http.StatusInternalServerError, w.Code)
}
```

**Code sample — integration-style test for duplicate detection
(`internal/server/server_duplicates_test.go`):**

```go
package server

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "os"
    "path/filepath"
    "testing"

    "github.com/jdfalk/audiobook-organizer/internal/database"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestListDuplicateAudiobooks_EmptyDB(t *testing.T) {
    server, cleanup := setupTestServer(t)
    defer cleanup()

    req := httptest.NewRequest(http.MethodGet, "/api/v1/audiobooks/duplicates", nil)
    w := httptest.NewRecorder()
    server.router.ServeHTTP(w, req)

    assert.Equal(t, http.StatusOK, w.Code)
    var resp map[string]interface{}
    require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
    // Duplicates is either null or an empty array
    dups, ok := resp["duplicates"].([]interface{})
    if ok {
        assert.Empty(t, dups)
    }
}

func TestListDuplicateAudiobooks_TwoBooksShareHash(t *testing.T) {
    server, cleanup := setupTestServer(t)
    defer cleanup()

    tempDir := t.TempDir()
    hash := "aaaa1111bbbb2222cccc3333dddd4444eeee5555ffff6666777788889999aaaa"

    for i, name := range []string{"dup_a.m4b", "dup_b.m4b"} {
        path := filepath.Join(tempDir, name)
        require.NoError(t, os.WriteFile(path, []byte("audio"), 0o644))
        _, err := database.GlobalStore.CreateBook(&database.Book{
            Title:    "Duplicate Book " + string(rune('A'+i)),
            FilePath: path,
            Format:   "m4b",
            FileHash: &hash,
        })
        require.NoError(t, err)
    }

    req := httptest.NewRequest(http.MethodGet, "/api/v1/audiobooks/duplicates", nil)
    w := httptest.NewRecorder()
    server.router.ServeHTTP(w, req)

    assert.Equal(t, http.StatusOK, w.Code)
    var resp map[string]interface{}
    require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
    dups := resp["duplicates"].([]interface{})
    assert.GreaterOrEqual(t, len(dups), 1, "expect at least one duplicate group")
    group := dups[0].([]interface{})
    assert.Equal(t, 2, len(group))
}
```

#### B. Scanner Package — Progress, Metadata Extraction, Duplicate Detection

**Package:** `github.com/jdfalk/audiobook-organizer/internal/scanner`
**File to extend:** `internal/scanner/scanner_test.go` or create
`internal/scanner/scanner_progress_test.go`

**Functions/behaviors that need tests:**

| Function / behavior | What to test |
|---|---|
| `ComputeFileHash` | (a) Returns consistent SHA-256 for a fixed file. (b) Returns empty string or error for missing file. |
| `SaveBookToDatabase` (scanner logic) | (a) Book with existing `file_path` is updated, not duplicated. (b) Book with existing `file_hash` triggers duplicate detection. |
| Progress reporting | Scan of a directory with N files reports progress from 0 to N. |

**Code sample — unit test for `ComputeFileHash`:**

```go
package scanner

import (
    "os"
    "path/filepath"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestComputeFileHash_ConsistentForSameContent(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "test.m4b")
    content := []byte("deterministic audiobook content for hashing")
    require.NoError(t, os.WriteFile(path, content, 0o644))

    hash1, err := ComputeFileHash(path)
    require.NoError(t, err)
    assert.NotEmpty(t, hash1)

    hash2, err := ComputeFileHash(path)
    require.NoError(t, err)
    assert.Equal(t, hash1, hash2, "same file must produce same hash")
}

func TestComputeFileHash_MissingFile_ReturnsError(t *testing.T) {
    _, err := ComputeFileHash("/nonexistent/path/book.m4b")
    assert.Error(t, err)
}
```

#### C. Database Queries — Soft Delete, State Transitions, Provenance

**Package:** `github.com/jdfalk/audiobook-organizer/internal/database`
**Files to extend:** `internal/database/audiobooks_test.go` or create
`internal/database/soft_delete_test.go`

**Functions/behaviors that need tests:**

| Store method | What to test |
|---|---|
| `ListSoftDeletedBooks` | (a) Returns only books where `marked_for_deletion == true`. (b) `olderThan` filter works correctly. (c) Empty result when no soft-deleted books exist. |
| `GetBookByFileHash` | Returns the correct book when hash matches; returns nil when no match. |
| `GetBookByOriginalHash` / `GetBookByOrganizedHash` | Same pattern as above, for the respective hash fields. |
| `UpsertMetadataFieldState` + `GetMetadataFieldStates` | (a) Insert new field state, read it back. (b) Update existing field state, verify overwrite. (c) Delete via `DeleteMetadataFieldState`, confirm gone. |

These tests use an in-memory SQLite store, exactly as the server integration
tests do:

```go
package database

import (
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func setupTestStore(t *testing.T) (Store, func()) {
    t.Helper()
    store, err := NewSQLiteStore(":memory:")
    require.NoError(t, err)
    return store, func() { store.Close() }
}

func TestListSoftDeletedBooks_OnlyReturnsDeleted(t *testing.T) {
    store, cleanup := setupTestStore(t)
    defer cleanup()

    marked := true
    unmarked := false
    _, err := store.CreateBook(&Book{Title: "Active", FilePath: "/a.m4b", MarkedForDeletion: &unmarked})
    require.NoError(t, err)
    _, err = store.CreateBook(&Book{Title: "Deleted", FilePath: "/b.m4b", MarkedForDeletion: &marked})
    require.NoError(t, err)

    results, err := store.ListSoftDeletedBooks(100, 0, nil)
    require.NoError(t, err)
    assert.Len(t, results, 1)
    assert.Equal(t, "Deleted", results[0].Title)
}

func TestMetadataFieldState_RoundTrip(t *testing.T) {
    store, cleanup := setupTestStore(t)
    defer cleanup()

    bookID := "01HXZTEST000000000000001"
    fetched := `"Open Library"`
    state := &MetadataFieldState{
        BookID:       bookID,
        Field:        "publisher",
        FetchedValue: &fetched,
        OverrideLocked: false,
        UpdatedAt:    time.Now(),
    }

    err := store.UpsertMetadataFieldState(state)
    require.NoError(t, err)

    states, err := store.GetMetadataFieldStates(bookID)
    require.NoError(t, err)
    require.Len(t, states, 1)
    assert.Equal(t, "publisher", states[0].Field)
    assert.NotNil(t, states[0].FetchedValue)
    assert.Equal(t, fetched, *states[0].FetchedValue)

    // Delete and confirm
    err = store.DeleteMetadataFieldState(bookID, "publisher")
    require.NoError(t, err)
    states, err = store.GetMetadataFieldStates(bookID)
    require.NoError(t, err)
    assert.Empty(t, states)
}
```

#### D. Migration 10 Validation

**Package:** `github.com/jdfalk/audiobook-organizer/internal/database`
**File to extend:** `internal/database/migrations_extra_test.go`

Verify that after `RunMigrations`, the provenance table exists and can
accept `UpsertMetadataFieldState` calls without error. The existing
`setupTestStore` pattern (SQLite `:memory:`) automatically runs migrations
on `NewSQLiteStore`, so simply calling `UpsertMetadataFieldState` after
setup is sufficient validation.

### E2E Backend Integration

Expand Playwright E2E from smoke tests to critical workflow coverage.
All E2E tests live under `web/tests/e2e/` and run against a Vite dev server
on `http://127.0.0.1:4173` (configured in `web/tests/e2e/playwright.config.ts`).
Every test file follows the same three-step setup:

```typescript
import { test, expect } from '@playwright/test';
import { mockEventSource, skipWelcomeWizard, setupMockApi } from './utils/test-helpers';

test.describe('Feature Name', () => {
  test.beforeEach(async ({ page }) => {
    await mockEventSource(page);    // prevents real SSE connections
    await skipWelcomeWizard(page);  // sets localStorage flag
  });

  test('scenario name', async ({ page }) => {
    // 1. Arrange — configure mock API responses via setupMockApi
    await setupMockApi(page, { books: [...], config: {...}, failures: {...} });

    // 2. Act — navigate and interact
    await page.goto('/library');
    await page.waitForLoadState('networkidle');

    // 3. Assert — verify UI state
    await expect(page.getByText('Expected Text')).toBeVisible();
  });
});
```

The `setupMockApi` helper intercepts `window.fetch` entirely within the page
context. It accepts an `options` object with these keys (all optional):
`books`, `config`, `importPaths`, `backups`, `blockedHashes`, `filesystem`,
`operations`, `itunes`, `failures`, `systemStatus`. See
`web/tests/e2e/utils/test-helpers.ts` for full type definitions.

**Run E2E tests:**
```bash
cd web
npm run test:e2e                          # runs all specs
npx playwright test --grep "Library"     # filter by describe block name
npx playwright test library-browser.spec.ts --headed  # single file, visible browser
```

#### Library: Search, Sort, Pagination, Metadata Fetch, AI Parse

**File:** `web/tests/e2e/library-browser.spec.ts` (extend existing)

Tests to add:

```typescript
test('searches books and updates displayed results', async ({ page }) => {
  const books = [
    { ...generateTestBooks(1)[0], id: 'b1', title: 'Dune', author_name: 'Frank Herbert' },
    { ...generateTestBooks(1)[0], id: 'b2', title: 'Foundation', author_name: 'Isaac Asimov' },
    { ...generateTestBooks(1)[0], id: 'b3', title: 'Neuromancer', author_name: 'William Gibson' },
  ];
  await setupLibraryWithBooks(page, books);
  await page.goto('/library');
  await page.waitForLoadState('networkidle');

  // Type into search box
  await page.getByPlaceholderText(/search/i).fill('Dune');
  await page.waitForTimeout(500); // debounce

  // Only Dune should be visible
  await expect(page.getByRole('heading', { name: 'Dune', exact: true })).toBeVisible();
  await expect(page.getByText('Foundation')).not.toBeVisible();
});

test('paginates through books across multiple pages', async ({ page }) => {
  const books = generateTestBooks(30); // default page size is 24
  await setupLibraryWithBooks(page, books);
  await page.goto('/library');
  await page.waitForLoadState('networkidle');

  // Page 1 shows first 24
  await expect(page.getByRole('heading', { name: 'Test Book 1', exact: true })).toBeVisible();

  // Navigate to page 2
  await page.getByRole('button', { name: /page 2/i }).click();
  await page.waitForLoadState('networkidle');

  // Page 2 shows books 25-30
  await expect(page.getByRole('heading', { name: 'Test Book 25', exact: true })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Test Book 1', exact: true })).not.toBeVisible();
});

test('triggers metadata fetch for a book', async ({ page }) => {
  const books = [generateTestBook({ id: 'fetch-test', title: 'Fetch Me' })];
  await setupLibraryWithBooks(page, books);

  // Navigate directly to the book detail page
  await page.goto('/library');
  await page.waitForLoadState('networkidle');
  await page.getByRole('heading', { name: 'Fetch Me' }).click();
  await page.waitForLoadState('networkidle');

  // Click Fetch Metadata button
  await page.getByRole('button', { name: /fetch metadata/i }).click();

  // Verify success indicator appears
  await expect(page.getByText(/metadata fetched/i)).toBeVisible();
});
```

#### Book Detail: Tab Navigation, Soft Delete + Block Hash, Restore, Version Linking

**File:** `web/tests/e2e/book-detail.spec.ts` (extend existing)

```typescript
test('soft-deletes a book and blocks its hash', async ({ page }) => {
  const book = generateTestBook({
    id: 'sd-test',
    title: 'Soft Delete Me',
    file_hash: 'deadbeef'.repeat(8),
  });
  await setupLibraryWithBooks(page, [book]);

  await page.goto('/library');
  await page.waitForLoadState('networkidle');
  await page.getByRole('heading', { name: 'Soft Delete Me' }).click();
  await page.waitForLoadState('networkidle');

  // Soft delete
  await page.getByRole('button', { name: /delete/i }).click();
  // Confirm dialog if present
  await page.getByRole('button', { name: /confirm/i }).click().catch(() => {});

  // Verify book is marked deleted (check for visual indicator)
  await expect(page.getByText(/deleted|marked for deletion/i)).toBeVisible();

  // Block the hash
  await page.getByRole('button', { name: /block hash/i }).click();
  await expect(page.getByText(/blocked/i)).toBeVisible();
});

test('links two books as versions', async ({ page }) => {
  const books = [
    generateTestBook({ id: 'ver-a', title: 'Book V1', file_hash: 'aaa'.repeat(21) + 'a' }),
    generateTestBook({ id: 'ver-b', title: 'Book V2', file_hash: 'bbb'.repeat(21) + 'b' }),
  ];
  await setupLibraryWithBooks(page, books);

  await page.goto('/library');
  await page.waitForLoadState('networkidle');
  await page.getByRole('heading', { name: 'Book V1' }).click();
  await page.waitForLoadState('networkidle');

  // Navigate to Versions tab
  await page.getByRole('tab', { name: /versions/i }).click();

  // Link Book V2
  await page.getByRole('button', { name: /link version/i }).click();
  // Select or type the other book
  await page.getByPlaceholderText(/search books/i).fill('Book V2');
  await page.getByText('Book V2').click();
  await page.getByRole('button', { name: /link/i }).click();

  // Verify both appear in the versions list
  await expect(page.getByText('Book V1')).toBeVisible();
  await expect(page.getByText('Book V2')).toBeVisible();
});
```

#### Settings: Import Paths End-to-End

**File:** `web/tests/e2e/settings-configuration.spec.ts` (extend existing)

```typescript
test('adds and removes an import path', async ({ page }) => {
  await setupMockApi(page, { importPaths: [] });
  await page.goto('/settings');
  await page.waitForLoadState('networkidle');

  // Navigate to Import Paths section (tab or scroll as needed)
  await page.getByRole('tab', { name: /import paths/i }).click().catch(() => {});

  // Add new import path
  await page.getByPlaceholderText(/path/i).fill('/tmp/new-import');
  await page.getByRole('button', { name: /add/i }).click();

  // Verify it appears
  await expect(page.getByText('/tmp/new-import')).toBeVisible();

  // Remove it
  const removeBtn = page.getByText('/tmp/new-import').locator('..').getByRole('button', { name: /remove|delete/i });
  await removeBtn.click();

  // Verify it is gone
  await expect(page.getByText('/tmp/new-import')).not.toBeVisible();
});
```

#### Soft-Deleted List: Restore and Purge Actions

**File:** `web/tests/e2e/book-detail.spec.ts` or create
`web/tests/e2e/soft-deleted.spec.ts`

```typescript
test('restores a soft-deleted book from the deleted list', async ({ page }) => {
  const book = generateTestBook({
    id: 'restore-me',
    title: 'Restore This Book',
    marked_for_deletion: true,
  });
  await setupMockApi(page, { books: [book] });

  // Navigate to soft-deleted list (typically at /library/deleted or a tab)
  await page.goto('/library');
  await page.waitForLoadState('networkidle');

  // Find the deleted section/tab
  await page.getByRole('tab', { name: /deleted/i }).click().catch(() => {});

  // Verify the book appears
  await expect(page.getByText('Restore This Book')).toBeVisible();

  // Click restore
  await page.getByText('Restore This Book').locator('..').getByRole('button', { name: /restore/i }).click();

  // Book should disappear from deleted list (or show as restored)
  await expect(page.getByText('Restore This Book')).not.toBeVisible();
});
```

### iTunes Library Import — Phases 2–4

⚡ In progress (40% complete — Phase 1 done). This is the **primary MVP
blocker**. See [`itunes-integration.md`](itunes-integration.md) for full
details.

**Files involved:**

| Phase | File(s) | Role |
|---|---|---|
| Phase 1 (done) | `internal/itunes/parser.go`, `internal/itunes/plist_parser.go` | iTunes Library.xml plist parsing, track classification (`IsAudiobook`). |
| Phase 2 | `internal/server/itunes.go` | HTTP handler layer: validate, import, write-back, status. Routes registered at `api.Group("/itunes")` in `internal/server/server.go` lines 750-755. |
| Phase 2 | `internal/itunes/import.go` | `ValidateImport()` — walks the parsed library, checks file existence, counts duplicates. |
| Phase 2 | `internal/itunes/writeback.go` | `WriteBack()` — rewrites iTunes Library.xml with updated file paths. |
| Phase 3 | `web/src/` (Settings page, iTunes Import tab) | UI: path input, validate button, import button, progress bar, status display. |
| Phase 4 | `internal/server/server_coverage_test.go` or new `server_itunes_test.go` | Go unit tests for iTunes handlers. |
| Phase 4 | `web/tests/e2e/itunes-import.spec.ts` | Playwright E2E tests (already has skeleton; expand). |

**Phase 2 — API Endpoints (routes already registered):**

```
POST  /api/v1/itunes/validate          → handleITunesValidate
POST  /api/v1/itunes/import            → handleITunesImport
POST  /api/v1/itunes/write-back        → handleITunesWriteBack
GET   /api/v1/itunes/import-status/:id → handleITunesImportStatus
```

Validate request body: `{"library_path": "/path/to/iTunes Library.xml"}`
Import request body:
```json
{
  "library_path": "/path/to/iTunes Library.xml",
  "import_mode": "organized",   // one of: organized | import | organize
  "preserve_location": false,
  "import_playlists": true,
  "skip_duplicates": true
}
```

Import response (202 Accepted):
```json
{
  "operation_id": "01HXZULID...",
  "status": "queued",
  "message": "iTunes import operation queued"
}
```

Status response (poll `GET /api/v1/itunes/import-status/<operation_id>`):
```json
{
  "operation_id": "01HXZULID...",
  "status": "running",
  "progress": 45,
  "message": "Processed 9/20 (imported 8, skipped 0, failed 1)",
  "total_books": 20,
  "processed": 9,
  "imported": 8,
  "skipped": 0,
  "failed": 1,
  "errors": ["Failed to save 'Some Title': file does not exist: /old/path.m4b"]
}
```

**Phase 4 — Go handler tests (file: `internal/server/server_itunes_test.go`):**

```go
//go:build mocks

package server

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/gin-gonic/gin"
    "github.com/jdfalk/audiobook-organizer/internal/database"
    dbmocks "github.com/jdfalk/audiobook-organizer/internal/database/mocks"
    "github.com/stretchr/testify/assert"
)

func TestITunesValidate_MissingLibraryPath_Returns400(t *testing.T) {
    gin.SetMode(gin.TestMode)

    mockStore := dbmocks.NewMockStore(t)
    database.GlobalStore = mockStore

    srv := &Server{router: gin.New()}
    srv.setupRoutes()

    // Empty body — binding:"required" on library_path triggers 400
    req := httptest.NewRequest(http.MethodPost, "/api/v1/itunes/validate", bytes.NewBufferString("{}"))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()
    srv.router.ServeHTTP(w, req)

    assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestITunesValidate_NonexistentFile_Returns400(t *testing.T) {
    gin.SetMode(gin.TestMode)

    mockStore := dbmocks.NewMockStore(t)
    database.GlobalStore = mockStore

    srv := &Server{router: gin.New()}
    srv.setupRoutes()

    body, _ := json.Marshal(map[string]string{"library_path": "/nonexistent/iTunes Library.xml"})
    req := httptest.NewRequest(http.MethodPost, "/api/v1/itunes/validate", bytes.NewBuffer(body))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()
    srv.router.ServeHTTP(w, req)

    assert.Equal(t, http.StatusBadRequest, w.Code)
    assert.Contains(t, w.Body.String(), "not found")
}

func TestITunesImport_NilStore_Returns500(t *testing.T) {
    gin.SetMode(gin.TestMode)
    database.GlobalStore = nil

    srv := &Server{router: gin.New()}
    srv.setupRoutes()

    body, _ := json.Marshal(map[string]string{
        "library_path": "/tmp/test.xml",
        "import_mode":  "import",
    })
    req := httptest.NewRequest(http.MethodPost, "/api/v1/itunes/import", bytes.NewBuffer(body))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()
    srv.router.ServeHTTP(w, req)

    assert.Equal(t, http.StatusInternalServerError, w.Code)
    assert.Contains(t, w.Body.String(), "database not initialized")
}
```

**Phase 4 — E2E expansion (extend `web/tests/e2e/itunes-import.spec.ts`):**

The existing skeleton already calls `setupMockApi(page)` which sets up default
iTunes mock responses (12 audiobook tracks, 11 imported, 1 skipped). To test
failure scenarios, pass custom `itunes` and `failures` options:

```typescript
test('shows validation errors when import has missing files', async ({ page }) => {
  await setupMockApi(page, {
    itunes: {
      validation: {
        total_tracks: 50,
        audiobook_tracks: 10,
        files_found: 7,
        files_missing: 3,
        missing_paths: ['/old/path1.m4b', '/old/path2.m4b', '/old/path3.m4b'],
        duplicate_count: 2,
        estimated_import_time: '10 seconds',
      },
    },
  });

  await page.goto('/settings');
  await page.waitForLoadState('networkidle');
  await page.getByRole('tab', { name: 'iTunes Import' }).click();
  await page.getByLabel('iTunes Library Path').fill('/path/to/library.xml');
  await page.getByRole('button', { name: 'Validate Import' }).click();

  // Verify missing files warning is displayed
  await expect(page.getByText('3 files missing')).toBeVisible();
  await expect(page.getByText('/old/path1.m4b')).toBeVisible();
});
```

---

## P1 — High Priority, Non-Blocking

### Documentation Capture

Record test results from P0 manual validation. Document findings in project
docs for future regression testing. Covers Settings → Blocked Hashes
verification and state transition + soft delete flows.

### Developer Guide

Architecture overview, data flow diagrams, and deployment instructions.
Currently no developer onboarding documentation exists beyond README.

---

## Dependencies

- iTunes integration phases 2–4 block MVP release
- Release pipeline fixes are needed before any release can ship
- Test coverage expansion can run in parallel with other work
- Manual QA should run after iTunes integration is functional

## References

- iTunes integration: [`itunes-integration.md`](itunes-integration.md)
- Release/packaging:
  [`release-packaging-and-devops.md`](release-packaging-and-devops.md)
- Manual QA checklist: `docs/MANUAL_TEST_CHECKLIST_P0.md`
- E2E test infrastructure: `docs/e2e-tests/`
