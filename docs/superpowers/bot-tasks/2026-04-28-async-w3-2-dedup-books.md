<!-- file: docs/superpowers/bot-tasks/2026-04-28-async-w3-2-dedup-books.md -->
<!-- version: 1.0.0 -->
<!-- guid: c6d7e8f9-a0b1-2345-cdef-678901234901 -->

# BOT TASK: Convert dedup-books to MaintenanceJob

**TODO ID:** ASYNC-W3-2  
**Audience:** burndown bot

## Prerequisites

- `task:ASYNC-CORE-2` — dispatcher must be live before any job can register

```bash
count=$(gh pr list --label "task:ASYNC-CORE-2" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:ASYNC-CORE-2"; exit 0; }
```

## Branch

```
feat/async-w3-2-dedup-books
```

## Label

```bash
gh label create "task:ASYNC-W3-2" --color "e4e669" --description "Bot task: dedup-books as MaintenanceJob" 2>/dev/null || true
```

## What This Does

Converts the synchronous `POST /api/v1/maintenance/dedup-books` handler into a
self-registering `MaintenanceJob`. Business logic moves from `maintenance_fixups.go`
into a new job file. The old route remains untouched until ASYNC-CLEAN-1.

**Current handler:** `maintenance_fixups.go` ~line 2210  
**Current params:** `dry_run` (query param, default true)  
**Current scale:** ~11-12K books, 4 phases — checkpoint at each phase boundary  
**Current return:** `{dry_run, phase1_removed, phase2_removed, phase3_removed, phase4_cleaned, errors}`

Phases:
1. Remove junk files (known garbage book entries)
2. Remove path duplicates (same path, multiple book rows)
3. Remove title duplicates (same title+author, keep best)
4. Version group cleanup (orphaned group references)

## Files to Create / Edit

1. **Create** `internal/maintenance/jobs/dedup_books.go`
2. **Create** `internal/maintenance/jobs/dedup_books_test.go`
3. **Edit** `internal/server/server.go` — ensure `_ "github.com/jdfalk/audiobook-organizer/internal/maintenance/jobs"` blank import exists (check first; add once)

## Step 1 — Create the job file

```go
// file: internal/maintenance/jobs/dedup_books.go
// version: 1.0.0
// guid: d7e8f9a0-b1c2-3456-defa-789012345012

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
	maintenance.Register(&DedupBooksJob{})
}

// DedupBooksParams are the accepted parameters for this job.
type DedupBooksParams struct {
	DryRun bool `json:"dry_run"`
}

// DedupBooksJob performs four-phase duplicate removal: junk files, path duplicates, title duplicates, version group cleanup.
type DedupBooksJob struct {
	store database.Store
}

func (j *DedupBooksJob) InjectStore(s database.Store) { j.store = s }

func (j *DedupBooksJob) ID() string         { return "dedup-books" }
func (j *DedupBooksJob) Name() string        { return "Dedup Books" }
func (j *DedupBooksJob) Description() string {
	return "Four-phase duplicate removal: junk files, path duplicates, title duplicates, version group cleanup."
}
func (j *DedupBooksJob) Category() string { return "dedup" }
func (j *DedupBooksJob) CanResume() bool  { return true }

func (j *DedupBooksJob) DefaultParams() any {
	return DedupBooksParams{DryRun: true}
}

func (j *DedupBooksJob) ValidateParams(raw json.RawMessage) error {
	var p DedupBooksParams
	return json.Unmarshal(raw, &p)
}

func (j *DedupBooksJob) Run(
	ctx context.Context,
	reporter operations.ProgressReporter,
	raw json.RawMessage,
	startFrom int,
) error {
	var params DedupBooksParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return fmt.Errorf("invalid params: %w", err)
	}

	// Load all books in one shot (check-then-apply)
	// move logic from maintenance_fixups.go ~line 2210
	books, err := j.store.GetAllBooks()
	if err != nil {
		return fmt.Errorf("failed to load books: %w", err)
	}

	// ─── Phase 1: Junk file removal ───────────────────────────────────────────
	// move junk-detection logic from maintenance_fixups.go ~line 2240
	var junkTargets []database.Book
	for _, b := range books {
		if isJunkBook(b) {
			junkTargets = append(junkTargets, b)
		}
	}
	_ = reporter.UpdateProgress(0, len(junkTargets), fmt.Sprintf("Phase 1: found %d junk books", len(junkTargets)))

	phase1Start := startFrom
	if phase1Start > len(junkTargets) {
		phase1Start = len(junkTargets)
	}
	for i := phase1Start; i < len(junkTargets); i++ {
		if reporter.IsCanceled() {
			return nil
		}
		if i%100 == 0 {
			_ = reporter.UpdateProgress(i, len(junkTargets), fmt.Sprintf("Phase 1: removing junk %d/%d", i, len(junkTargets)))
			operations.SaveCheckpoint(j.store, reporter.OperationID(), &operations.OperationState{PhaseIndex: i, PhaseTotal: len(junkTargets)})
		}
		b := junkTargets[i]
		if params.DryRun {
			_ = reporter.Log("info", fmt.Sprintf("[dry] Phase1: Would remove junk book: %s (id=%s)", b.Title, b.ID), nil)
			continue
		}
		// move delete logic from maintenance_fixups.go ~line 2280
		if err := j.store.DeleteBook(b.ID); err != nil {
			_ = reporter.Log("error", fmt.Sprintf("Phase1: failed to delete %s: %v", b.ID, err), nil)
		}
	}
	// Checkpoint at phase 1 boundary
	operations.SaveCheckpoint(j.store, reporter.OperationID(), &operations.OperationState{PhaseIndex: len(junkTargets), PhaseTotal: len(junkTargets)})

	// ─── Phase 2: Path duplicates ─────────────────────────────────────────────
	// move path-dedup logic from maintenance_fixups.go ~line 2300
	pathDupGroups, err := j.store.GetPathDuplicateBookGroups()
	if err != nil {
		return fmt.Errorf("phase 2: failed to load path duplicates: %w", err)
	}
	_ = reporter.UpdateProgress(0, len(pathDupGroups), fmt.Sprintf("Phase 2: found %d path duplicate groups", len(pathDupGroups)))

	phase2Start := 0
	if startFrom > len(junkTargets) {
		phase2Start = startFrom - len(junkTargets)
	}
	if phase2Start > len(pathDupGroups) {
		phase2Start = len(pathDupGroups)
	}
	for i := phase2Start; i < len(pathDupGroups); i++ {
		if reporter.IsCanceled() {
			return nil
		}
		if i%100 == 0 {
			_ = reporter.UpdateProgress(i, len(pathDupGroups), fmt.Sprintf("Phase 2: deduping paths %d/%d", i, len(pathDupGroups)))
			operations.SaveCheckpoint(j.store, reporter.OperationID(), &operations.OperationState{
				PhaseIndex: len(junkTargets) + i,
				PhaseTotal: len(junkTargets) + len(pathDupGroups),
			})
		}
		grp := pathDupGroups[i]
		if params.DryRun {
			_ = reporter.Log("info", fmt.Sprintf("[dry] Phase2: Would dedup %d books at path %q", len(grp.Books), grp.Path), nil)
			continue
		}
		// move path-dedup logic from maintenance_fixups.go ~line 2340
	}
	operations.SaveCheckpoint(j.store, reporter.OperationID(), &operations.OperationState{
		PhaseIndex: len(junkTargets) + len(pathDupGroups),
		PhaseTotal: len(junkTargets) + len(pathDupGroups),
	})

	// ─── Phase 3: Title duplicates ────────────────────────────────────────────
	// move title-dedup logic from maintenance_fixups.go ~line 2380
	titleDupGroups, err := j.store.GetTitleDuplicateBookGroups()
	if err != nil {
		return fmt.Errorf("phase 3: failed to load title duplicates: %w", err)
	}
	_ = reporter.UpdateProgress(0, len(titleDupGroups), fmt.Sprintf("Phase 3: found %d title duplicate groups", len(titleDupGroups)))

	phase3Start := 0
	if startFrom > len(junkTargets)+len(pathDupGroups) {
		phase3Start = startFrom - len(junkTargets) - len(pathDupGroups)
	}
	if phase3Start > len(titleDupGroups) {
		phase3Start = len(titleDupGroups)
	}
	for i := phase3Start; i < len(titleDupGroups); i++ {
		if reporter.IsCanceled() {
			return nil
		}
		if i%100 == 0 {
			_ = reporter.UpdateProgress(i, len(titleDupGroups), fmt.Sprintf("Phase 3: deduping titles %d/%d", i, len(titleDupGroups)))
			operations.SaveCheckpoint(j.store, reporter.OperationID(), &operations.OperationState{
				PhaseIndex: len(junkTargets) + len(pathDupGroups) + i,
				PhaseTotal: len(junkTargets) + len(pathDupGroups) + len(titleDupGroups),
			})
		}
		grp := titleDupGroups[i]
		if params.DryRun {
			_ = reporter.Log("info", fmt.Sprintf("[dry] Phase3: Would merge %d books with title %q", len(grp.Books), grp.Title), nil)
			continue
		}
		// move title-dedup logic from maintenance_fixups.go ~line 2430
	}
	operations.SaveCheckpoint(j.store, reporter.OperationID(), &operations.OperationState{
		PhaseIndex: len(junkTargets) + len(pathDupGroups) + len(titleDupGroups),
		PhaseTotal: len(junkTargets) + len(pathDupGroups) + len(titleDupGroups),
	})

	// ─── Phase 4: Version group cleanup ───────────────────────────────────────
	// move version-group-cleanup logic from maintenance_fixups.go ~line 2480
	orphanedGroupRefs, err := j.store.GetOrphanedVersionGroupRefs()
	if err != nil {
		return fmt.Errorf("phase 4: failed to load orphaned group refs: %w", err)
	}
	_ = reporter.UpdateProgress(0, len(orphanedGroupRefs), fmt.Sprintf("Phase 4: found %d orphaned version group refs", len(orphanedGroupRefs)))

	phase4Start := 0
	offset4 := len(junkTargets) + len(pathDupGroups) + len(titleDupGroups)
	if startFrom > offset4 {
		phase4Start = startFrom - offset4
	}
	if phase4Start > len(orphanedGroupRefs) {
		phase4Start = len(orphanedGroupRefs)
	}
	for i := phase4Start; i < len(orphanedGroupRefs); i++ {
		if reporter.IsCanceled() {
			return nil
		}
		if i%100 == 0 {
			_ = reporter.UpdateProgress(i, len(orphanedGroupRefs), fmt.Sprintf("Phase 4: cleaning refs %d/%d", i, len(orphanedGroupRefs)))
			operations.SaveCheckpoint(j.store, reporter.OperationID(), &operations.OperationState{
				PhaseIndex: offset4 + i,
				PhaseTotal: offset4 + len(orphanedGroupRefs),
			})
		}
		ref := orphanedGroupRefs[i]
		if params.DryRun {
			_ = reporter.Log("info", fmt.Sprintf("[dry] Phase4: Would clean orphaned group ref: book %s group %s", ref.BookID, ref.GroupID), nil)
			continue
		}
		// move cleanup logic from maintenance_fixups.go ~line 2520
	}

	total := offset4 + len(orphanedGroupRefs)
	_ = reporter.UpdateProgress(total, total, "Done: all four dedup phases complete")
	return nil
}

// isJunkBook returns true if the book matches known junk/garbage patterns.
// move detection logic from maintenance_fixups.go ~line 2240
func isJunkBook(b database.Book) bool {
	// Implementation: read from maintenance_fixups.go ~line 2240
	// (e.g. title == "", path has garbage extension, etc.)
	return false // placeholder
}
```

## Step 2 — Create the test file

```go
// file: internal/maintenance/jobs/dedup_books_test.go
// version: 1.0.0
// guid: e8f9a0b1-c2d3-4567-efab-890123456123

package jobs_test

import (
	"encoding/json"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/maintenance/jobs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDedupBooksJob_ID(t *testing.T) {
	j := &jobs.DedupBooksJob{}
	assert.Equal(t, "dedup-books", j.ID())
}

func TestDedupBooksJob_ValidateParams(t *testing.T) {
	j := &jobs.DedupBooksJob{}
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
go test ./internal/maintenance/jobs/... -run TestDedupBooks
go vet ./internal/maintenance/jobs/...
make build-api
```

After building, confirm:
```bash
curl -s http://localhost:8080/api/v1/maintenance/jobs | jq '.jobs[] | select(.id=="dedup-books")'
```
Should return the job definition.

## Definition of Done

- `internal/maintenance/jobs/dedup_books.go` exists and compiles
- Job registers itself via `init()`
- `GET /api/v1/maintenance/jobs` includes `dedup-books`
- `POST /api/v1/maintenance/jobs/dedup-books` returns 202
- DryRun mode logs without modifying DB
- Four phases each with checkpoint at phase boundary
- IsCanceled() checked in every phase loop
- Checkpoint saved every 100 items per phase
- Resume correctly skips completed phases
- Tests pass

## PR Instructions

```bash
gh label create "task:ASYNC-W3-2" --color "e4e669" --description "Bot task: dedup-books as MaintenanceJob" 2>/dev/null || true
gh pr create \
  --title "feat(maintenance): convert dedup-books to MaintenanceJob" \
  --body "Moves four-phase dedup logic into a self-registering MaintenanceJob. Checkpoint at each phase boundary, IsCanceled() in every loop, full resume support across all four phases. Old route untouched. (ASYNC-W3-2)" \
  --label "task:ASYNC-W3-2"
```
