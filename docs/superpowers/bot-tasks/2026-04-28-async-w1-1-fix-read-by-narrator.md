<!-- file: docs/superpowers/bot-tasks/2026-04-28-async-w1-1-fix-read-by-narrator.md -->
<!-- version: 1.0.0 -->
<!-- guid: e1f2a3b4-c5d6-7890-efab-123456789012 -->

# BOT TASK: Convert fix-read-by-narrator to MaintenanceJob

**TODO ID:** ASYNC-W1-1  
**Audience:** burndown bot

## Prerequisites

- `task:ASYNC-CORE-2` — dispatcher must be live before any job can register

```bash
count=$(gh pr list --label "task:ASYNC-CORE-2" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:ASYNC-CORE-2"; exit 0; }
```

## Branch

```
feat/async-w1-1-fix-read-by-narrator
```

## Label

```bash
gh label create "task:ASYNC-W1-1" --color "e4e669" --description "Bot task: fix-read-by-narrator as MaintenanceJob" 2>/dev/null || true
```

## What This Does

Converts the synchronous `POST /api/v1/maintenance/fix-read-by-narrator` handler
into a self-registering `MaintenanceJob`. The business logic moves from
`maintenance_fixups.go` into a new job file. The old route remains untouched
until ASYNC-CLEAN-1.

**Current handler:** `maintenance_fixups.go` ~line 71  
**Current params:** `dry_run` (query param, default true)  
**Current scale:** ~11K books, per-book string matching  
**Current return:** `{dry_run, total_found, applied, errors, results[]}`

## Files to Create / Edit

1. **Create** `internal/maintenance/jobs/fix_read_by_narrator.go`
2. **Create** `internal/maintenance/jobs/fix_read_by_narrator_test.go`
3. **Edit** `internal/server/server.go` — ensure `_ "github.com/jdfalk/audiobook-organizer/internal/maintenance/jobs"` blank import exists (or add it once; subsequent wave tasks check it's already there)

## Step 1 — Create the job file

```go
// file: internal/maintenance/jobs/fix_read_by_narrator.go
// version: 1.0.0
// guid: f2a3b4c5-d6e7-8901-fabc-234567890123

package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/maintenance"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
)

func init() {
	maintenance.Register(&FixReadByNarratorJob{})
}

// FixReadByNarratorParams are the accepted parameters for this job.
type FixReadByNarratorParams struct {
	DryRun bool `json:"dry_run"`
}

// FixReadByNarratorJob removes "read by NARRATOR" suffixes from author names.
type FixReadByNarratorJob struct {
	store database.Store
}

func (j *FixReadByNarratorJob) InjectStore(s database.Store) { j.store = s }

func (j *FixReadByNarratorJob) ID() string          { return "fix-read-by-narrator" }
func (j *FixReadByNarratorJob) Name() string         { return "Fix Read-by Narrator" }
func (j *FixReadByNarratorJob) Description() string  {
	return "Strips 'read by NARRATOR' suffixes from author names inserted by some importers."
}
func (j *FixReadByNarratorJob) Category() string { return "library" }
func (j *FixReadByNarratorJob) CanResume() bool  { return true }

func (j *FixReadByNarratorJob) DefaultParams() any {
	return FixReadByNarratorParams{DryRun: true}
}

func (j *FixReadByNarratorJob) ValidateParams(raw json.RawMessage) error {
	var p FixReadByNarratorParams
	return json.Unmarshal(raw, &p)
}

func (j *FixReadByNarratorJob) Run(
	ctx context.Context,
	reporter operations.ProgressReporter,
	raw json.RawMessage,
	startFrom int,
) error {
	var params FixReadByNarratorParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return fmt.Errorf("invalid params: %w", err)
	}

	books, err := j.store.GetAllBooks()
	if err != nil {
		return fmt.Errorf("failed to load books: %w", err)
	}

	// Phase 1: identify affected books (check-then-apply)
	type affected struct {
		book    database.Book
		fixedBy string
	}
	var targets []affected
	for _, b := range books {
		if fixed, ok := stripReadByNarrator(b.Author); ok {
			targets = append(targets, affected{book: b, fixedBy: fixed})
		}
	}

	_ = reporter.UpdateProgress(startFrom, len(targets), fmt.Sprintf("Found %d books to fix", len(targets)))

	for i := startFrom; i < len(targets); i++ {
		if reporter.IsCanceled() {
			return nil
		}

		t := targets[i]
		if i%100 == 0 {
			_ = reporter.UpdateProgress(i, len(targets), fmt.Sprintf("Fixing %d/%d", i, len(targets)))
			operations.SaveCheckpoint(j.store, reporter.OperationID(), &operations.OperationState{
				PhaseIndex: i,
				PhaseTotal: len(targets),
			})
		}

		if params.DryRun {
			_ = reporter.Log("info", fmt.Sprintf("[dry] Would fix: %q → %q", t.book.Author, t.fixedBy), nil)
			continue
		}

		t.book.Author = t.fixedBy
		if err := j.store.UpdateBook(t.book); err != nil {
			_ = reporter.Log("error", fmt.Sprintf("Failed to update %s: %v", t.book.Title, err), nil)
		}
	}

	_ = reporter.UpdateProgress(len(targets), len(targets), fmt.Sprintf("Done: %d books processed", len(targets)))
	return nil
}

// stripReadByNarrator removes "read by ..." and "narrated by ..." suffixes.
// Returns (fixedName, true) if a suffix was found.
func stripReadByNarrator(author string) (string, bool) {
	lower := strings.ToLower(author)
	for _, prefix := range []string{" read by ", " narrated by ", ", read by ", ", narrated by "} {
		if idx := strings.Index(lower, prefix); idx > 0 {
			return strings.TrimSpace(author[:idx]), true
		}
	}
	return author, false
}
```

## Step 2 — Create the test file

```go
// file: internal/maintenance/jobs/fix_read_by_narrator_test.go
// version: 1.0.0
// guid: a3b4c5d6-e7f8-9012-abcd-345678901234

package jobs_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/maintenance/jobs"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFixReadByNarratorJob_ID(t *testing.T) {
	j := &jobs.FixReadByNarratorJob{}
	assert.Equal(t, "fix-read-by-narrator", j.ID())
}

func TestFixReadByNarratorJob_ValidateParams(t *testing.T) {
	j := &jobs.FixReadByNarratorJob{}
	require.NoError(t, j.ValidateParams(json.RawMessage(`{"dry_run":true}`)))
	require.Error(t, j.ValidateParams(json.RawMessage(`not json`)))
}

func TestStripReadByNarrator(t *testing.T) {
	// Export stripReadByNarrator or test via Run with mock store
	cases := []struct {
		input string
		want  string
		found bool
	}{
		{"John Smith read by Jane Doe", "John Smith", true},
		{"John Smith, narrated by Jane Doe", "John Smith", true},
		{"John Smith", "John Smith", false},
	}
	// Test via the job's effect on mock data
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			// Use reflection or make stripReadByNarrator exported
			_ = tc
		})
	}
}
```

> Note: export `StripReadByNarrator` (capitalize) so it can be tested directly.

## Step 3 — Add blank import in server.go

Search `server.go` for any existing `_ "...maintenance/jobs"` blank import.
If not found, add to the import block:

```go
_ "github.com/jdfalk/audiobook-organizer/internal/maintenance/jobs" // registers all maintenance jobs
```

This triggers all `init()` functions.

## Step 4 — Verify

```bash
go test ./internal/maintenance/jobs/... -run TestFixReadByNarrator
go vet ./internal/maintenance/jobs/...
make build-api
```

After building, confirm:
```bash
curl -s http://localhost:8080/api/v1/maintenance/jobs | jq '.jobs[] | select(.id=="fix-read-by-narrator")'
```
Should return the job definition.

## Definition of Done

- `internal/maintenance/jobs/fix_read_by_narrator.go` exists and compiles
- Job registers itself via `init()`
- `GET /api/v1/maintenance/jobs` includes `fix-read-by-narrator`
- `POST /api/v1/maintenance/jobs/fix-read-by-narrator` returns 202
- DryRun mode logs without modifying DB
- IsCanceled() checked in main loop
- Checkpoint saved every 100 books
- Tests pass

## PR Instructions

```bash
gh label create "task:ASYNC-W1-1" --color "e4e669" --description "Bot task: fix-read-by-narrator as MaintenanceJob" 2>/dev/null || true
gh pr create \
  --title "feat(maintenance): convert fix-read-by-narrator to MaintenanceJob" \
  --body "Moves business logic into a self-registering MaintenanceJob. Adds IsCanceled() checks, checkpoint every 100 books, and resume support. Old route untouched. (ASYNC-W1-1)" \
  --label "task:ASYNC-W1-1"
```
