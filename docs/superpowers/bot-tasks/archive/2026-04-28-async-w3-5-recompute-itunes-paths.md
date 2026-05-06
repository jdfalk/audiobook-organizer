<!-- file: docs/superpowers/bot-tasks/2026-04-28-async-w3-5-recompute-itunes-paths.md -->
<!-- version: 1.0.0 -->
<!-- guid: f5a6b7c8-d9e0-1234-fabc-567890123890 -->

# BOT TASK: Convert recompute-itunes-paths to MaintenanceJob

**TODO ID:** ASYNC-W3-5  
**Audience:** burndown bot

## Prerequisites

- `task:ASYNC-CORE-2` — dispatcher must be live before any job can register

```bash
count=$(gh pr list --label "task:ASYNC-CORE-2" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:ASYNC-CORE-2"; exit 0; }
```

## Branch

```
feat/async-w3-5-recompute-itunes-paths
```

## Label

```bash
gh label create "task:ASYNC-W3-5" --color "e4e669" --description "Bot task: recompute-itunes-paths as MaintenanceJob" 2>/dev/null || true
```

## What This Does

Converts the synchronous `POST /api/v1/maintenance/recompute-itunes-paths` handler
into a self-registering `MaintenanceJob`. Business logic moves from
`maintenance_fixups.go` into a new job file. The old route remains untouched until
ASYNC-CLEAN-1.

**Current handler:** `maintenance_fixups.go` ~line 3497  
**Current params:** `dry_run` (query param, default true)  
**Current scale:** Primary books → resolves itunes_path via book_files table + iTunes XML data  
**Current return:** `{dry_run, total_books, updated, not_found, errors}`

> **Important constraint:** The iTunes machine is a remote Windows machine. This job
> reads the iTunes XML that was already parsed and stored in the DB — it does NOT
> access the remote machine at job runtime. The XML data lives in the `itunes_*`
> tables populated by the iTunes scanner.

## Files to Create / Edit

1. **Create** `internal/maintenance/jobs/recompute_itunes_paths.go`
2. **Create** `internal/maintenance/jobs/recompute_itunes_paths_test.go`
3. **Edit** `internal/server/server.go` — ensure `_ "github.com/jdfalk/audiobook-organizer/internal/maintenance/jobs"` blank import exists (check first; add once)

## Step 1 — Create the job file

```go
// file: internal/maintenance/jobs/recompute_itunes_paths.go
// version: 1.0.0
// guid: a6b7c8d9-e0f1-2345-abcd-678901234901

package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/maintenance"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
)

func init() {
	maintenance.Register(&RecomputeItunesPathsJob{})
}

// RecomputeItunesPathsParams are the accepted parameters for this job.
type RecomputeItunesPathsParams struct {
	DryRun bool `json:"dry_run"`
}

// RecomputeItunesPathsJob recomputes itunes_path fields for all books using current iTunes XML data.
type RecomputeItunesPathsJob struct {
	store database.Store
}

func (j *RecomputeItunesPathsJob) InjectStore(s database.Store) { j.store = s }

func (j *RecomputeItunesPathsJob) ID() string         { return "recompute-itunes-paths" }
func (j *RecomputeItunesPathsJob) Name() string        { return "Recompute iTunes Paths" }
func (j *RecomputeItunesPathsJob) Description() string {
	return "Recomputes itunes_path fields for all books using current iTunes XML data."
}
func (j *RecomputeItunesPathsJob) Category() string { return "itunes" }
func (j *RecomputeItunesPathsJob) CanResume() bool  { return true }

func (j *RecomputeItunesPathsJob) DefaultParams() any {
	return RecomputeItunesPathsParams{DryRun: true}
}

func (j *RecomputeItunesPathsJob) ValidateParams(raw json.RawMessage) error {
	var p RecomputeItunesPathsParams
	return json.Unmarshal(raw, &p)
}

func (j *RecomputeItunesPathsJob) Run(
	ctx context.Context,
	reporter operations.ProgressReporter,
	raw json.RawMessage,
	startFrom int,
) error {
	var params RecomputeItunesPathsParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return fmt.Errorf("invalid params: %w", err)
	}

	// Load all primary books in one shot (check-then-apply)
	// move logic from maintenance_fixups.go ~line 3497
	books, err := j.store.GetAllPrimaryBooks()
	if err != nil {
		return fmt.Errorf("failed to load primary books: %w", err)
	}

	// Load iTunes track data from DB (already parsed from XML)
	// move iTunes-data loading from maintenance_fixups.go ~line 3520
	// NOTE: this reads from the itunes_* tables, NOT the remote Windows machine
	itunesTracks, err := j.store.GetAllItunesTracks()
	if err != nil {
		return fmt.Errorf("failed to load iTunes tracks: %w", err)
	}

	// Build a lookup: book_file path → iTunes track path
	// move lookup-build logic from maintenance_fixups.go ~line 3535
	type pathLookup struct {
		itunesPath string
		pid        string
	}
	fileToItunesPath := make(map[string]pathLookup, len(itunesTracks))
	for _, t := range itunesTracks {
		// move mapping logic from maintenance_fixups.go ~line 3545
		// (normalize paths for comparison, map local lib path → iTunes path)
		fileToItunesPath[t.LibraryPath] = pathLookup{itunesPath: t.Location, pid: t.PersistentID}
	}

	_ = reporter.UpdateProgress(startFrom, len(books), fmt.Sprintf("Found %d primary books, %d iTunes tracks", len(books), len(itunesTracks)))

	for i := startFrom; i < len(books); i++ {
		if reporter.IsCanceled() {
			return nil
		}
		if i%100 == 0 {
			_ = reporter.UpdateProgress(i, len(books), fmt.Sprintf("Recomputing %d/%d", i, len(books)))
			operations.SaveCheckpoint(j.store, reporter.OperationID(), &operations.OperationState{
				PhaseIndex: i,
				PhaseTotal: len(books),
			})
		}

		b := books[i]

		// Find the book's primary file in book_files
		// move file-selection logic from maintenance_fixups.go ~line 3565
		bookFiles, err := j.store.GetBookFilesByBookID(b.ID)
		if err != nil || len(bookFiles) == 0 {
			continue
		}

		// Look up iTunes path for the primary file
		// move lookup logic from maintenance_fixups.go ~line 3580
		var newItunesPath string
		for _, f := range bookFiles {
			if lookup, ok := fileToItunesPath[f.Path]; ok {
				newItunesPath = lookup.itunesPath
				break
			}
		}

		if newItunesPath == "" {
			// no iTunes match found — skip silently
			continue
		}

		if newItunesPath == b.ItunesPath {
			// already correct
			continue
		}

		if params.DryRun {
			_ = reporter.Log("info", fmt.Sprintf("[dry] Would update itunes_path: %q→%q (book: %s)", b.ItunesPath, newItunesPath, b.Title), nil)
			continue
		}

		b.ItunesPath = newItunesPath
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
// file: internal/maintenance/jobs/recompute_itunes_paths_test.go
// version: 1.0.0
// guid: b7c8d9e0-f1a2-3456-bcde-789012345012

package jobs_test

import (
	"encoding/json"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/maintenance/jobs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecomputeItunesPathsJob_ID(t *testing.T) {
	j := &jobs.RecomputeItunesPathsJob{}
	assert.Equal(t, "recompute-itunes-paths", j.ID())
}

func TestRecomputeItunesPathsJob_ValidateParams(t *testing.T) {
	j := &jobs.RecomputeItunesPathsJob{}
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
go test ./internal/maintenance/jobs/... -run TestRecomputeItunesPaths
go vet ./internal/maintenance/jobs/...
make build-api
```

After building, confirm:
```bash
curl -s http://localhost:8080/api/v1/maintenance/jobs | jq '.jobs[] | select(.id=="recompute-itunes-paths")'
```
Should return the job definition.

## Definition of Done

- `internal/maintenance/jobs/recompute_itunes_paths.go` exists and compiles
- Job registers itself via `init()`
- `GET /api/v1/maintenance/jobs` includes `recompute-itunes-paths`
- `POST /api/v1/maintenance/jobs/recompute-itunes-paths` returns 202
- DryRun mode logs without modifying DB
- Reads iTunes data from DB tables only (no remote Windows machine access at runtime)
- Skips books where itunes_path is already correct
- IsCanceled() checked in main loop
- Checkpoint saved every 100 books
- Tests pass

## PR Instructions

```bash
gh label create "task:ASYNC-W3-5" --color "e4e669" --description "Bot task: recompute-itunes-paths as MaintenanceJob" 2>/dev/null || true
gh pr create \
  --title "feat(maintenance): convert recompute-itunes-paths to MaintenanceJob" \
  --body "Moves business logic into a self-registering MaintenanceJob. Reads iTunes data from DB (not remote Windows machine), builds path lookup table, IsCanceled() checks, checkpoint every 100 books, resume support. Old route untouched. (ASYNC-W3-5)" \
  --label "task:ASYNC-W3-5"
```
