<!-- file: docs/superpowers/bot-tasks/2026-04-28-async-w2-4-fix-library-states.md -->
<!-- version: 1.0.0 -->
<!-- guid: c0d1e2f3-a4b5-6789-cdef-012345678345 -->

# BOT TASK: Convert fix-library-states to MaintenanceJob

**TODO ID:** ASYNC-W2-4  
**Audience:** burndown bot

## Prerequisites

- `task:ASYNC-CORE-2` — dispatcher must be live before any job can register

```bash
count=$(gh pr list --label "task:ASYNC-CORE-2" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:ASYNC-CORE-2"; exit 0; }
```

## Branch

```
feat/async-w2-4-fix-library-states
```

## Label

```bash
gh label create "task:ASYNC-W2-4" --color "e4e669" --description "Bot task: fix-library-states as MaintenanceJob" 2>/dev/null || true
```

## What This Does

Converts the synchronous `POST /api/v1/maintenance/fix-library-states` handler into a
self-registering `MaintenanceJob`. Business logic moves from `maintenance_fixups.go`
into a new job file. The old route remains untouched until ASYNC-CLEAN-1.

**Current handler:** `maintenance_fixups.go` ~line 3397  
**Current params:** `dry_run` (query param, default true)  
**Current scale:** ~11K books filtered to those with disagreeing library_state vs. actual file presence  
**Current return:** `{dry_run, total_checked, fixed, errors}`

## Files to Create / Edit

1. **Create** `internal/maintenance/jobs/fix_library_states.go`
2. **Create** `internal/maintenance/jobs/fix_library_states_test.go`
3. **Edit** `internal/server/server.go` — ensure `_ "github.com/jdfalk/audiobook-organizer/internal/maintenance/jobs"` blank import exists (check first; add once)

## Step 1 — Create the job file

```go
// file: internal/maintenance/jobs/fix_library_states.go
// version: 1.0.0
// guid: d1e2f3a4-b5c6-7890-defa-123456789456

package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/maintenance"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
)

func init() {
	maintenance.Register(&FixLibraryStatesJob{})
}

// FixLibraryStatesParams are the accepted parameters for this job.
type FixLibraryStatesParams struct {
	DryRun bool `json:"dry_run"`
}

// FixLibraryStatesJob recomputes library_state for books where it disagrees with actual file presence.
type FixLibraryStatesJob struct {
	store database.Store
}

func (j *FixLibraryStatesJob) InjectStore(s database.Store) { j.store = s }

func (j *FixLibraryStatesJob) ID() string         { return "fix-library-states" }
func (j *FixLibraryStatesJob) Name() string        { return "Fix Library States" }
func (j *FixLibraryStatesJob) Description() string {
	return "Recomputes library_state field for books where it disagrees with actual file presence."
}
func (j *FixLibraryStatesJob) Category() string { return "library" }
func (j *FixLibraryStatesJob) CanResume() bool  { return true }

func (j *FixLibraryStatesJob) DefaultParams() any {
	return FixLibraryStatesParams{DryRun: true}
}

func (j *FixLibraryStatesJob) ValidateParams(raw json.RawMessage) error {
	var p FixLibraryStatesParams
	return json.Unmarshal(raw, &p)
}

func (j *FixLibraryStatesJob) Run(
	ctx context.Context,
	reporter operations.ProgressReporter,
	raw json.RawMessage,
	startFrom int,
) error {
	var params FixLibraryStatesParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return fmt.Errorf("invalid params: %w", err)
	}

	// Load all books in one shot (check-then-apply)
	// move logic from maintenance_fixups.go ~line 3397
	books, err := j.store.GetAllBooks()
	if err != nil {
		return fmt.Errorf("failed to load books: %w", err)
	}

	// Identify books where library_state disagrees with file presence on disk
	// move detection logic from maintenance_fixups.go ~line 3420
	type stateCandidate struct {
		book          database.Book
		correctState  string
		actualOnDisk  bool
	}
	var targets []stateCandidate
	for _, b := range books {
		actualOnDisk := b.Path != "" && pathExists(b.Path)
		// move state computation from maintenance_fixups.go ~line 3440
		correctState := computeCorrectLibraryState(b, actualOnDisk)
		if correctState != b.LibraryState {
			targets = append(targets, stateCandidate{book: b, correctState: correctState, actualOnDisk: actualOnDisk})
		}
	}

	_ = reporter.UpdateProgress(startFrom, len(targets), fmt.Sprintf("Found %d books with incorrect library_state", len(targets)))

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
		if params.DryRun {
			_ = reporter.Log("info", fmt.Sprintf("[dry] Would fix library_state: %q→%q (book: %s, on_disk: %v)",
				t.book.LibraryState, t.correctState, t.book.Title, t.actualOnDisk), nil)
			continue
		}

		t.book.LibraryState = t.correctState
		if err := j.store.UpdateBook(t.book); err != nil {
			_ = reporter.Log("error", fmt.Sprintf("Failed to update %s: %v", t.book.Title, err), nil)
		}
	}

	_ = reporter.UpdateProgress(len(targets), len(targets), fmt.Sprintf("Done: %d books fixed", len(targets)))
	return nil
}

// pathExists returns true if the path exists on disk.
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// computeCorrectLibraryState derives the correct library_state from book data and disk presence.
// move logic from maintenance_fixups.go ~line 3440
func computeCorrectLibraryState(b database.Book, onDisk bool) string {
	// Implementation: read from maintenance_fixups.go ~line 3440
	// (e.g. "present", "missing", "deleted", etc.)
	if onDisk {
		return "present"
	}
	return "missing"
}
```

## Step 2 — Create the test file

```go
// file: internal/maintenance/jobs/fix_library_states_test.go
// version: 1.0.0
// guid: e2f3a4b5-c6d7-8901-efab-234567890567

package jobs_test

import (
	"encoding/json"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/maintenance/jobs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFixLibraryStatesJob_ID(t *testing.T) {
	j := &jobs.FixLibraryStatesJob{}
	assert.Equal(t, "fix-library-states", j.ID())
}

func TestFixLibraryStatesJob_ValidateParams(t *testing.T) {
	j := &jobs.FixLibraryStatesJob{}
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
go test ./internal/maintenance/jobs/... -run TestFixLibraryStates
go vet ./internal/maintenance/jobs/...
make build-api
```

After building, confirm:
```bash
curl -s http://localhost:8080/api/v1/maintenance/jobs | jq '.jobs[] | select(.id=="fix-library-states")'
```
Should return the job definition.

## Definition of Done

- `internal/maintenance/jobs/fix_library_states.go` exists and compiles
- Job registers itself via `init()`
- `GET /api/v1/maintenance/jobs` includes `fix-library-states`
- `POST /api/v1/maintenance/jobs/fix-library-states` returns 202
- DryRun mode logs without modifying DB
- IsCanceled() checked in main loop
- Checkpoint saved every 100 books
- Tests pass

## PR Instructions

```bash
gh label create "task:ASYNC-W2-4" --color "e4e669" --description "Bot task: fix-library-states as MaintenanceJob" 2>/dev/null || true
gh pr create \
  --title "feat(maintenance): convert fix-library-states to MaintenanceJob" \
  --body "Moves business logic into a self-registering MaintenanceJob. Checks disk presence vs stored library_state with IsCanceled() checks, checkpoint every 100 books, and resume support. Old route untouched. (ASYNC-W2-4)" \
  --label "task:ASYNC-W2-4"
```
