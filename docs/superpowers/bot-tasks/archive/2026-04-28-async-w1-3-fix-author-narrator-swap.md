<!-- file: docs/superpowers/bot-tasks/2026-04-28-async-w1-3-fix-author-narrator-swap.md -->
<!-- version: 1.0.0 -->
<!-- guid: e5f6a7b8-c9d0-1234-efab-567890123456 -->

# BOT TASK: Convert fix-author-narrator-swap to MaintenanceJob

**TODO ID:** ASYNC-W1-3  
**Audience:** burndown bot

## Prerequisites

- `task:ASYNC-CORE-2` — dispatcher must be live before any job can register

```bash
count=$(gh pr list --label "task:ASYNC-CORE-2" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:ASYNC-CORE-2"; exit 0; }
```

## Branch

```
feat/async-w1-3-fix-author-narrator-swap
```

## Label

```bash
gh label create "task:ASYNC-W1-3" --color "e4e669" --description "Bot task: fix-author-narrator-swap as MaintenanceJob" 2>/dev/null || true
```

## What This Does

Converts the synchronous `POST /api/v1/maintenance/fix-author-narrator-swap` handler
into a self-registering `MaintenanceJob`. Business logic moves from
`maintenance_fixups.go` into a new job file. The old route remains untouched until
ASYNC-CLEAN-1.

**Current handler:** `maintenance_fixups.go` ~line 1114  
**Current params:** `dry_run` (query param, default true)  
**Current scale:** ~11K books, batch=500  
**Current return:** `{dry_run, total_found, applied, errors, results[]}`

## Files to Create / Edit

1. **Create** `internal/maintenance/jobs/fix_author_narrator_swap.go`
2. **Create** `internal/maintenance/jobs/fix_author_narrator_swap_test.go`
3. **Edit** `internal/server/server.go` — ensure `_ "github.com/jdfalk/audiobook-organizer/internal/maintenance/jobs"` blank import exists (check first; add once)

## Step 1 — Create the job file

```go
// file: internal/maintenance/jobs/fix_author_narrator_swap.go
// version: 1.0.0
// guid: f6a7b8c9-d0e1-2345-fabc-678901234567

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
	maintenance.Register(&FixAuthorNarratorSwapJob{})
}

// FixAuthorNarratorSwapParams are the accepted parameters for this job.
type FixAuthorNarratorSwapParams struct {
	DryRun bool `json:"dry_run"`
}

// FixAuthorNarratorSwapJob corrects books where the author and narrator fields were accidentally swapped.
type FixAuthorNarratorSwapJob struct {
	store database.Store
}

func (j *FixAuthorNarratorSwapJob) InjectStore(s database.Store) { j.store = s }

func (j *FixAuthorNarratorSwapJob) ID() string         { return "fix-author-narrator-swap" }
func (j *FixAuthorNarratorSwapJob) Name() string        { return "Fix Author/Narrator Swap" }
func (j *FixAuthorNarratorSwapJob) Description() string {
	return "Corrects books where the author and narrator fields were accidentally swapped."
}
func (j *FixAuthorNarratorSwapJob) Category() string { return "library" }
func (j *FixAuthorNarratorSwapJob) CanResume() bool  { return true }

func (j *FixAuthorNarratorSwapJob) DefaultParams() any {
	return FixAuthorNarratorSwapParams{DryRun: true}
}

func (j *FixAuthorNarratorSwapJob) ValidateParams(raw json.RawMessage) error {
	var p FixAuthorNarratorSwapParams
	return json.Unmarshal(raw, &p)
}

func (j *FixAuthorNarratorSwapJob) Run(
	ctx context.Context,
	reporter operations.ProgressReporter,
	raw json.RawMessage,
	startFrom int,
) error {
	var params FixAuthorNarratorSwapParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return fmt.Errorf("invalid params: %w", err)
	}

	// Load all books in one shot (check-then-apply pattern)
	// move logic from maintenance_fixups.go ~line 1114
	books, err := j.store.GetAllBooks()
	if err != nil {
		return fmt.Errorf("failed to load books: %w", err)
	}

	// Identify affected books: author looks like a narrator name and narrator looks like an author name
	// move detection heuristic from maintenance_fixups.go ~line 1140
	type swapCandidate struct {
		book          database.Book
		correctAuthor string
		correctNarrator string
	}
	var targets []swapCandidate
	for _, b := range books {
		if ca, cn, swapped := detectAuthorNarratorSwap(b); swapped {
			targets = append(targets, swapCandidate{book: b, correctAuthor: ca, correctNarrator: cn})
		}
	}

	_ = reporter.UpdateProgress(startFrom, len(targets), fmt.Sprintf("Found %d books with swapped author/narrator", len(targets)))

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
			_ = reporter.Log("info", fmt.Sprintf("[dry] Would swap: author %q→%q, narrator %q→%q (book: %s)",
				t.book.Author, t.correctAuthor, t.book.Narrator, t.correctNarrator, t.book.Title), nil)
			continue
		}

		t.book.Author = t.correctAuthor
		t.book.Narrator = t.correctNarrator
		if err := j.store.UpdateBook(t.book); err != nil {
			_ = reporter.Log("error", fmt.Sprintf("Failed to update %s: %v", t.book.Title, err), nil)
		}
	}

	_ = reporter.UpdateProgress(len(targets), len(targets), fmt.Sprintf("Done: %d books processed", len(targets)))
	return nil
}

// detectAuthorNarratorSwap returns corrected author/narrator and true if a swap is detected.
// move detection heuristic from maintenance_fixups.go ~line 1155
func detectAuthorNarratorSwap(b database.Book) (correctAuthor, correctNarrator string, swapped bool) {
	// Implementation: read heuristic from maintenance_fixups.go ~line 1155
	// (e.g. author contains "narrated by", narrator matches known-author pattern, etc.)
	return b.Narrator, b.Author, false // placeholder; replace with real logic
}
```

## Step 2 — Create the test file

```go
// file: internal/maintenance/jobs/fix_author_narrator_swap_test.go
// version: 1.0.0
// guid: a7b8c9d0-e1f2-3456-abcd-789012345678

package jobs_test

import (
	"encoding/json"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/maintenance/jobs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFixAuthorNarratorSwapJob_ID(t *testing.T) {
	j := &jobs.FixAuthorNarratorSwapJob{}
	assert.Equal(t, "fix-author-narrator-swap", j.ID())
}

func TestFixAuthorNarratorSwapJob_ValidateParams(t *testing.T) {
	j := &jobs.FixAuthorNarratorSwapJob{}
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
go test ./internal/maintenance/jobs/... -run TestFixAuthorNarratorSwap
go vet ./internal/maintenance/jobs/...
make build-api
```

After building, confirm:
```bash
curl -s http://localhost:8080/api/v1/maintenance/jobs | jq '.jobs[] | select(.id=="fix-author-narrator-swap")'
```
Should return the job definition.

## Definition of Done

- `internal/maintenance/jobs/fix_author_narrator_swap.go` exists and compiles
- Job registers itself via `init()`
- `GET /api/v1/maintenance/jobs` includes `fix-author-narrator-swap`
- `POST /api/v1/maintenance/jobs/fix-author-narrator-swap` returns 202
- DryRun mode logs without modifying DB
- IsCanceled() checked in main loop
- Checkpoint saved every 100 books
- Tests pass

## PR Instructions

```bash
gh label create "task:ASYNC-W1-3" --color "e4e669" --description "Bot task: fix-author-narrator-swap as MaintenanceJob" 2>/dev/null || true
gh pr create \
  --title "feat(maintenance): convert fix-author-narrator-swap to MaintenanceJob" \
  --body "Moves business logic into a self-registering MaintenanceJob. Adds IsCanceled() checks, checkpoint every 100 books, and resume support. Old route untouched. (ASYNC-W1-3)" \
  --label "task:ASYNC-W1-3"
```
