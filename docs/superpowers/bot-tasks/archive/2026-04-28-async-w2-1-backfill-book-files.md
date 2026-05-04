<!-- file: docs/superpowers/bot-tasks/2026-04-28-async-w2-1-backfill-book-files.md -->
<!-- version: 1.0.0 -->
<!-- guid: e1f2a3b4-c5d6-7890-efab-123456789abc -->

# BOT TASK: Convert backfill-book-files to MaintenanceJob

**TODO ID:** ASYNC-W2-1  
**Audience:** burndown bot

## Prerequisites

- `task:ASYNC-CORE-2` — dispatcher must be live before any job can register

```bash
count=$(gh pr list --label "task:ASYNC-CORE-2" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:ASYNC-CORE-2"; exit 0; }
```

## Branch

```
feat/async-w2-1-backfill-book-files
```

## Label

```bash
gh label create "task:ASYNC-W2-1" --color "e4e669" --description "Bot task: backfill-book-files as MaintenanceJob" 2>/dev/null || true
```

## What This Does

Converts the synchronous `POST /api/v1/maintenance/backfill-book-files` handler into a
self-registering `MaintenanceJob`. Business logic moves from `maintenance_fixups.go`
into a new job file. The old route remains untouched until ASYNC-CLEAN-1.

**Current handler:** `maintenance_fixups.go` ~line 655  
**Current params:** `dry_run` (query param, default true)  
**Current scale:** ~11K books + disk I/O (directory scan per book)  
**Current return:** `{dry_run, total_books, files_added, errors}`

## Files to Create / Edit

1. **Create** `internal/maintenance/jobs/backfill_book_files.go`
2. **Create** `internal/maintenance/jobs/backfill_book_files_test.go`
3. **Edit** `internal/server/server.go` — ensure `_ "github.com/jdfalk/audiobook-organizer/internal/maintenance/jobs"` blank import exists (check first; add once)

## Step 1 — Create the job file

```go
// file: internal/maintenance/jobs/backfill_book_files.go
// version: 1.0.0
// guid: f2a3b4c5-d6e7-8901-fabc-234567890bcd

package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/maintenance"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
)

func init() {
	maintenance.Register(&BackfillBookFilesJob{})
}

// BackfillBookFilesParams are the accepted parameters for this job.
type BackfillBookFilesParams struct {
	DryRun bool `json:"dry_run"`
}

// BackfillBookFilesJob scans library directories and populates missing book_files table entries.
type BackfillBookFilesJob struct {
	store database.Store
}

func (j *BackfillBookFilesJob) InjectStore(s database.Store) { j.store = s }

func (j *BackfillBookFilesJob) ID() string         { return "backfill-book-files" }
func (j *BackfillBookFilesJob) Name() string        { return "Backfill Book Files" }
func (j *BackfillBookFilesJob) Description() string {
	return "Scans library directories and populates missing book_files table entries."
}
func (j *BackfillBookFilesJob) Category() string { return "files" }
func (j *BackfillBookFilesJob) CanResume() bool  { return true }

func (j *BackfillBookFilesJob) DefaultParams() any {
	return BackfillBookFilesParams{DryRun: true}
}

func (j *BackfillBookFilesJob) ValidateParams(raw json.RawMessage) error {
	var p BackfillBookFilesParams
	return json.Unmarshal(raw, &p)
}

func (j *BackfillBookFilesJob) Run(
	ctx context.Context,
	reporter operations.ProgressReporter,
	raw json.RawMessage,
	startFrom int,
) error {
	var params BackfillBookFilesParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return fmt.Errorf("invalid params: %w", err)
	}

	// Load all books in one shot
	// move logic from maintenance_fixups.go ~line 655
	books, err := j.store.GetAllBooks()
	if err != nil {
		return fmt.Errorf("failed to load books: %w", err)
	}

	_ = reporter.UpdateProgress(startFrom, len(books), fmt.Sprintf("Found %d books to check", len(books)))

	for i := startFrom; i < len(books); i++ {
		if reporter.IsCanceled() {
			return nil
		}
		if i%100 == 0 {
			_ = reporter.UpdateProgress(i, len(books), fmt.Sprintf("Scanning %d/%d", i, len(books)))
			operations.SaveCheckpoint(j.store, reporter.OperationID(), &operations.OperationState{
				PhaseIndex: i,
				PhaseTotal: len(books),
			})
		}

		b := books[i]
		if b.Path == "" {
			continue
		}

		// Scan the book directory for audio files not yet in book_files
		// move directory-scan logic from maintenance_fixups.go ~line 690
		existingFiles, err := j.store.GetBookFilesByBookID(b.ID)
		if err != nil {
			_ = reporter.Log("error", fmt.Sprintf("Failed to get existing files for book %s: %v", b.ID, err), nil)
			continue
		}
		existingPaths := make(map[string]bool, len(existingFiles))
		for _, f := range existingFiles {
			existingPaths[f.Path] = true
		}

		// Walk the book directory to find audio files on disk
		// move walk logic from maintenance_fixups.go ~line 710
		_ = filepath.Walk(b.Path, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			// move audio-extension check from maintenance_fixups.go ~line 725
			if existingPaths[path] {
				return nil
			}
			if params.DryRun {
				_ = reporter.Log("info", fmt.Sprintf("[dry] Would add file: %s (book: %s)", path, b.Title), nil)
				return nil
			}
			// move book_files insert logic from maintenance_fixups.go ~line 740
			return nil
		})
	}

	_ = reporter.UpdateProgress(len(books), len(books), fmt.Sprintf("Done: %d books scanned", len(books)))
	return nil
}
```

## Step 2 — Create the test file

```go
// file: internal/maintenance/jobs/backfill_book_files_test.go
// version: 1.0.0
// guid: a3b4c5d6-e7f8-9012-abcd-345678901cde

package jobs_test

import (
	"encoding/json"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/maintenance/jobs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackfillBookFilesJob_ID(t *testing.T) {
	j := &jobs.BackfillBookFilesJob{}
	assert.Equal(t, "backfill-book-files", j.ID())
}

func TestBackfillBookFilesJob_ValidateParams(t *testing.T) {
	j := &jobs.BackfillBookFilesJob{}
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
go test ./internal/maintenance/jobs/... -run TestBackfillBookFiles
go vet ./internal/maintenance/jobs/...
make build-api
```

After building, confirm:
```bash
curl -s http://localhost:8080/api/v1/maintenance/jobs | jq '.jobs[] | select(.id=="backfill-book-files")'
```
Should return the job definition.

## Definition of Done

- `internal/maintenance/jobs/backfill_book_files.go` exists and compiles
- Job registers itself via `init()`
- `GET /api/v1/maintenance/jobs` includes `backfill-book-files`
- `POST /api/v1/maintenance/jobs/backfill-book-files` returns 202
- DryRun mode logs without modifying DB
- IsCanceled() checked in main loop
- Checkpoint saved every 100 books
- Tests pass

## PR Instructions

```bash
gh label create "task:ASYNC-W2-1" --color "e4e669" --description "Bot task: backfill-book-files as MaintenanceJob" 2>/dev/null || true
gh pr create \
  --title "feat(maintenance): convert backfill-book-files to MaintenanceJob" \
  --body "Moves business logic into a self-registering MaintenanceJob. Scans library directories with IsCanceled() checks, checkpoint every 100 books, and resume support. Old route untouched. (ASYNC-W2-1)" \
  --label "task:ASYNC-W2-1"
```
