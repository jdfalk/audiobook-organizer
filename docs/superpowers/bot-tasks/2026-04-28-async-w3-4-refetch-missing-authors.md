<!-- file: docs/superpowers/bot-tasks/2026-04-28-async-w3-4-refetch-missing-authors.md -->
<!-- version: 1.0.0 -->
<!-- guid: c2d3e4f5-a6b7-8901-cdef-234567890567 -->

# BOT TASK: Convert refetch-missing-authors to MaintenanceJob

**TODO ID:** ASYNC-W3-4  
**Audience:** burndown bot

## Prerequisites

- `task:ASYNC-CORE-2` — dispatcher must be live before any job can register

```bash
count=$(gh pr list --label "task:ASYNC-CORE-2" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:ASYNC-CORE-2"; exit 0; }
```

## Branch

```
feat/async-w3-4-refetch-missing-authors
```

## Label

```bash
gh label create "task:ASYNC-W3-4" --color "e4e669" --description "Bot task: refetch-missing-authors as MaintenanceJob" 2>/dev/null || true
```

## What This Does

Converts the synchronous `POST /api/v1/maintenance/refetch-missing-authors` handler
into a self-registering `MaintenanceJob`. Business logic moves from
`maintenance_fixups.go` into a new job file. The old route remains untouched until
ASYNC-CLEAN-1.

**Current handler:** `maintenance_fixups.go` ~line 2822  
**Current params:** `dry_run` (query param, default true)  
**Current scale:** Books without author field, batch=500, reads tags from disk  
**Current return:** `{dry_run, total_found, filled, errors}`

## Files to Create / Edit

1. **Create** `internal/maintenance/jobs/refetch_missing_authors.go`
2. **Create** `internal/maintenance/jobs/refetch_missing_authors_test.go`
3. **Edit** `internal/server/server.go` — ensure `_ "github.com/jdfalk/audiobook-organizer/internal/maintenance/jobs"` blank import exists (check first; add once)

## Step 1 — Create the job file

```go
// file: internal/maintenance/jobs/refetch_missing_authors.go
// version: 1.0.0
// guid: d3e4f5a6-b7c8-9012-defa-345678901678

package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/maintenance"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	"github.com/jdfalk/audiobook-organizer/internal/taglib"
)

func init() {
	maintenance.Register(&RefetchMissingAuthorsJob{})
}

// RefetchMissingAuthorsParams are the accepted parameters for this job.
type RefetchMissingAuthorsParams struct {
	DryRun bool `json:"dry_run"`
}

// RefetchMissingAuthorsJob re-reads author info from file tags for books where the author field is empty.
type RefetchMissingAuthorsJob struct {
	store database.Store
}

func (j *RefetchMissingAuthorsJob) InjectStore(s database.Store) { j.store = s }

func (j *RefetchMissingAuthorsJob) ID() string         { return "refetch-missing-authors" }
func (j *RefetchMissingAuthorsJob) Name() string        { return "Refetch Missing Authors" }
func (j *RefetchMissingAuthorsJob) Description() string {
	return "Re-reads author info from file tags for books where the author field is empty."
}
func (j *RefetchMissingAuthorsJob) Category() string { return "library" }
func (j *RefetchMissingAuthorsJob) CanResume() bool  { return true }

func (j *RefetchMissingAuthorsJob) DefaultParams() any {
	return RefetchMissingAuthorsParams{DryRun: true}
}

func (j *RefetchMissingAuthorsJob) ValidateParams(raw json.RawMessage) error {
	var p RefetchMissingAuthorsParams
	return json.Unmarshal(raw, &p)
}

func (j *RefetchMissingAuthorsJob) Run(
	ctx context.Context,
	reporter operations.ProgressReporter,
	raw json.RawMessage,
	startFrom int,
) error {
	var params RefetchMissingAuthorsParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return fmt.Errorf("invalid params: %w", err)
	}

	// Load all books without an author in one shot (check-then-apply)
	// move logic from maintenance_fixups.go ~line 2822
	books, err := j.store.GetBooksWithEmptyAuthor()
	if err != nil {
		return fmt.Errorf("failed to load books with empty author: %w", err)
	}

	_ = reporter.UpdateProgress(startFrom, len(books), fmt.Sprintf("Found %d books with missing author", len(books)))

	for i := startFrom; i < len(books); i++ {
		if reporter.IsCanceled() {
			return nil
		}
		if i%100 == 0 {
			_ = reporter.UpdateProgress(i, len(books), fmt.Sprintf("Refetching %d/%d", i, len(books)))
			operations.SaveCheckpoint(j.store, reporter.OperationID(), &operations.OperationState{
				PhaseIndex: i,
				PhaseTotal: len(books),
			})
		}

		b := books[i]

		// Find the first audio file for this book to read tags from
		// move file-selection logic from maintenance_fixups.go ~line 2850
		bookFiles, err := j.store.GetBookFilesByBookID(b.ID)
		if err != nil || len(bookFiles) == 0 {
			_ = reporter.Log("warn", fmt.Sprintf("No files for book %s (%s), skipping", b.ID, b.Title), nil)
			continue
		}

		// Read tags from disk using taglib
		// move tag-read logic from maintenance_fixups.go ~line 2870
		// Tag priority: album_artist > artist > composer (composer = narrator in audiobooks)
		meta, err := taglib.ExtractMetadata(bookFiles[0].Path)
		if err != nil {
			_ = reporter.Log("error", fmt.Sprintf("Failed to read tags for %s: %v", bookFiles[0].Path, err), nil)
			continue
		}

		author := meta.AlbumArtist
		if author == "" {
			author = meta.Artist
		}
		if author == "" {
			author = meta.Composer
		}
		if author == "" {
			_ = reporter.Log("warn", fmt.Sprintf("No author found in tags for book %s (%s)", b.ID, b.Title), nil)
			continue
		}

		if params.DryRun {
			_ = reporter.Log("info", fmt.Sprintf("[dry] Would set author: %q (book: %s)", author, b.Title), nil)
			continue
		}

		b.Author = author
		if err := j.store.UpdateBook(b); err != nil {
			_ = reporter.Log("error", fmt.Sprintf("Failed to update book %s: %v", b.ID, err), nil)
		}
	}

	_ = reporter.UpdateProgress(len(books), len(books), fmt.Sprintf("Done: %d books processed", len(books)))
	return nil
}
```

## Step 2 — Create the test file

```go
// file: internal/maintenance/jobs/refetch_missing_authors_test.go
// version: 1.0.0
// guid: e4f5a6b7-c8d9-0123-efab-456789012789

package jobs_test

import (
	"encoding/json"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/maintenance/jobs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefetchMissingAuthorsJob_ID(t *testing.T) {
	j := &jobs.RefetchMissingAuthorsJob{}
	assert.Equal(t, "refetch-missing-authors", j.ID())
}

func TestRefetchMissingAuthorsJob_ValidateParams(t *testing.T) {
	j := &jobs.RefetchMissingAuthorsJob{}
	require.NoError(t, j.ValidateParams(json.RawMessage(`{"dry_run":true}`)))
	require.NoError(t, j.ValidateParams(json.RawMessage(`{"dry_run":false}`)))
	require.Error(t, j.ValidateParams(json.RawMessage(`not json`)))
}
```

## Step 3 — Add blank import in server.go

Search `server.go` for any existing `_ "...maintenance/jobs"` blank import.
If not found, add to the import block:

```go
_ "github.com/jdfalk/audiobook-organizer/internal/maintenance/jobs" // registers all maintenance jobs
```

## Step 4 — Verify

```bash
go test ./internal/maintenance/jobs/... -run TestRefetchMissingAuthors
go vet ./internal/maintenance/jobs/...
make build-api
```

After building, confirm:
```bash
curl -s http://localhost:8080/api/v1/maintenance/jobs | jq '.jobs[] | select(.id=="refetch-missing-authors")'
```
Should return the job definition.

## Definition of Done

- `internal/maintenance/jobs/refetch_missing_authors.go` exists and compiles
- Job registers itself via `init()`
- `GET /api/v1/maintenance/jobs` includes `refetch-missing-authors`
- `POST /api/v1/maintenance/jobs/refetch-missing-authors` returns 202
- DryRun mode logs without modifying DB
- Tag priority: album_artist > artist > composer (matches codebase convention)
- Uses taglib.ExtractMetadata for tag reads
- IsCanceled() checked in main loop
- Checkpoint saved every 100 books
- Tests pass

## PR Instructions

```bash
gh label create "task:ASYNC-W3-4" --color "e4e669" --description "Bot task: refetch-missing-authors as MaintenanceJob" 2>/dev/null || true
gh pr create \
  --title "feat(maintenance): convert refetch-missing-authors to MaintenanceJob" \
  --body "Moves business logic into a self-registering MaintenanceJob. Re-reads album_artist/artist/composer from file tags via taglib, IsCanceled() checks, checkpoint every 100 books, resume support. Old route untouched. (ASYNC-W3-4)" \
  --label "task:ASYNC-W3-4"
```
