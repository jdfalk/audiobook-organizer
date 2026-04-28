<!-- file: docs/superpowers/bot-tasks/2026-04-28-async-w3-3-fix-book-file-paths.md -->
<!-- version: 1.0.0 -->
<!-- guid: f9a0b1c2-d3e4-5678-fabc-901234567234 -->

# BOT TASK: Convert fix-book-file-paths to MaintenanceJob

**TODO ID:** ASYNC-W3-3  
**Audience:** burndown bot

## Prerequisites

- `task:ASYNC-CORE-2` — dispatcher must be live before any job can register

```bash
count=$(gh pr list --label "task:ASYNC-CORE-2" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:ASYNC-CORE-2"; exit 0; }
```

## Branch

```
feat/async-w3-3-fix-book-file-paths
```

## Label

```bash
gh label create "task:ASYNC-W3-3" --color "e4e669" --description "Bot task: fix-book-file-paths as MaintenanceJob" 2>/dev/null || true
```

## What This Does

Converts the synchronous `POST /api/v1/maintenance/fix-book-file-paths` handler into a
self-registering `MaintenanceJob`. Business logic moves from `maintenance_fixups.go`
into a new job file. The old route remains untouched until ASYNC-CLEAN-1.

**Current handler:** `maintenance_fixups.go` ~line 1920  
**Current params:** `dry_run` (query param, default true)  
**Current scale:** All book_files rows (disk walk + glob to find actual location)  
**Current return:** `{dry_run, total_files, fixed, not_found, errors}`

## Files to Create / Edit

1. **Create** `internal/maintenance/jobs/fix_book_file_paths.go`
2. **Create** `internal/maintenance/jobs/fix_book_file_paths_test.go`
3. **Edit** `internal/server/server.go` — ensure `_ "github.com/jdfalk/audiobook-organizer/internal/maintenance/jobs"` blank import exists (check first; add once)

## Step 1 — Create the job file

```go
// file: internal/maintenance/jobs/fix_book_file_paths.go
// version: 1.0.0
// guid: a0b1c2d3-e4f5-6789-abcd-012345678345

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
	maintenance.Register(&FixBookFilePathsJob{})
}

// FixBookFilePathsParams are the accepted parameters for this job.
type FixBookFilePathsParams struct {
	DryRun bool `json:"dry_run"`
}

// FixBookFilePathsJob repairs book_files rows where the stored path doesn't match the actual file on disk.
type FixBookFilePathsJob struct {
	store database.Store
}

func (j *FixBookFilePathsJob) InjectStore(s database.Store) { j.store = s }

func (j *FixBookFilePathsJob) ID() string         { return "fix-book-file-paths" }
func (j *FixBookFilePathsJob) Name() string        { return "Fix Book File Paths" }
func (j *FixBookFilePathsJob) Description() string {
	return "Repairs book_files rows where the stored path doesn't match the actual file on disk."
}
func (j *FixBookFilePathsJob) Category() string { return "files" }
func (j *FixBookFilePathsJob) CanResume() bool  { return true }

func (j *FixBookFilePathsJob) DefaultParams() any {
	return FixBookFilePathsParams{DryRun: true}
}

func (j *FixBookFilePathsJob) ValidateParams(raw json.RawMessage) error {
	var p FixBookFilePathsParams
	return json.Unmarshal(raw, &p)
}

func (j *FixBookFilePathsJob) Run(
	ctx context.Context,
	reporter operations.ProgressReporter,
	raw json.RawMessage,
	startFrom int,
) error {
	var params FixBookFilePathsParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return fmt.Errorf("invalid params: %w", err)
	}

	// Load all book_files in one shot (check-then-apply)
	// move logic from maintenance_fixups.go ~line 1920
	files, err := j.store.GetAllBookFiles()
	if err != nil {
		return fmt.Errorf("failed to load book_files: %w", err)
	}

	// Filter to only those whose stored path does not exist on disk
	// move filter logic from maintenance_fixups.go ~line 1940
	type brokenPath struct {
		file database.BookFile
	}
	var targets []brokenPath
	for _, f := range files {
		if _, err := os.Stat(f.Path); os.IsNotExist(err) {
			targets = append(targets, brokenPath{file: f})
		}
	}

	_ = reporter.UpdateProgress(startFrom, len(targets), fmt.Sprintf("Found %d book_files with broken paths", len(targets)))

	for i := startFrom; i < len(targets); i++ {
		if reporter.IsCanceled() {
			return nil
		}
		if i%100 == 0 {
			_ = reporter.UpdateProgress(i, len(targets), fmt.Sprintf("Fixing %d/%d", i, len(targets)))
			operations.SaveCheckpoint(j.store, reporter.OperationID(), &operations.OperationState{
				PhaseIndex: i,
				PhaseTotal: len(targets),
			})
		}

		t := targets[i]
		f := t.file

		// Try to locate the file using the parent book's directory + glob
		// move glob logic from maintenance_fixups.go ~line 1960
		book, err := j.store.GetBookByID(f.BookID)
		if err != nil {
			_ = reporter.Log("error", fmt.Sprintf("Could not load book %s for file %s: %v", f.BookID, f.Path, err), nil)
			continue
		}

		filename := filepath.Base(f.Path)
		// move glob search logic from maintenance_fixups.go ~line 1975
		candidates, err := filepath.Glob(filepath.Join(book.Path, "**", filename))
		if err != nil || len(candidates) == 0 {
			// Try flat glob
			candidates, _ = filepath.Glob(filepath.Join(book.Path, filename))
		}

		if len(candidates) == 0 {
			_ = reporter.Log("warn", fmt.Sprintf("Could not locate file %s for book %s", filename, book.Title), nil)
			continue
		}

		newPath := candidates[0]
		if params.DryRun {
			_ = reporter.Log("info", fmt.Sprintf("[dry] Would fix path: %q → %q (book: %s)", f.Path, newPath, book.Title), nil)
			continue
		}

		f.Path = newPath
		if err := j.store.UpdateBookFile(f); err != nil {
			_ = reporter.Log("error", fmt.Sprintf("Failed to update book_file %s: %v", f.ID, err), nil)
		}
	}

	_ = reporter.UpdateProgress(len(targets), len(targets), fmt.Sprintf("Done: %d broken paths processed", len(targets)))
	return nil
}
```

## Step 2 — Create the test file

```go
// file: internal/maintenance/jobs/fix_book_file_paths_test.go
// version: 1.0.0
// guid: b1c2d3e4-f5a6-7890-bcde-123456789456

package jobs_test

import (
	"encoding/json"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/maintenance/jobs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFixBookFilePathsJob_ID(t *testing.T) {
	j := &jobs.FixBookFilePathsJob{}
	assert.Equal(t, "fix-book-file-paths", j.ID())
}

func TestFixBookFilePathsJob_ValidateParams(t *testing.T) {
	j := &jobs.FixBookFilePathsJob{}
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
go test ./internal/maintenance/jobs/... -run TestFixBookFilePaths
go vet ./internal/maintenance/jobs/...
make build-api
```

After building, confirm:
```bash
curl -s http://localhost:8080/api/v1/maintenance/jobs | jq '.jobs[] | select(.id=="fix-book-file-paths")'
```
Should return the job definition.

## Definition of Done

- `internal/maintenance/jobs/fix_book_file_paths.go` exists and compiles
- Job registers itself via `init()`
- `GET /api/v1/maintenance/jobs` includes `fix-book-file-paths`
- `POST /api/v1/maintenance/jobs/fix-book-file-paths` returns 202
- DryRun mode logs without modifying DB
- Only processes book_files whose stored path does not exist on disk
- Uses glob-based file location strategy from original handler
- IsCanceled() checked in main loop
- Checkpoint saved every 100 files
- Tests pass

## PR Instructions

```bash
gh label create "task:ASYNC-W3-3" --color "e4e669" --description "Bot task: fix-book-file-paths as MaintenanceJob" 2>/dev/null || true
gh pr create \
  --title "feat(maintenance): convert fix-book-file-paths to MaintenanceJob" \
  --body "Moves business logic into a self-registering MaintenanceJob. Uses disk-stat + glob to locate missing files, IsCanceled() checks, checkpoint every 100 files, resume support. Old route untouched. (ASYNC-W3-3)" \
  --label "task:ASYNC-W3-3"
```
