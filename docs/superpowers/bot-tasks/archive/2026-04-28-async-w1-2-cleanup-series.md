<!-- file: docs/superpowers/bot-tasks/2026-04-28-async-w1-2-cleanup-series.md -->
<!-- version: 1.0.0 -->
<!-- guid: b2c3d4e5-f6a7-8901-bcde-234567890123 -->

# BOT TASK: Convert cleanup-series to MaintenanceJob

**TODO ID:** ASYNC-W1-2  
**Audience:** burndown bot

## Prerequisites

- `task:ASYNC-CORE-2` — dispatcher must be live before any job can register

```bash
count=$(gh pr list --label "task:ASYNC-CORE-2" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:ASYNC-CORE-2"; exit 0; }
```

## Branch

```
feat/async-w1-2-cleanup-series
```

## Label

```bash
gh label create "task:ASYNC-W1-2" --color "e4e669" --description "Bot task: cleanup-series as MaintenanceJob" 2>/dev/null || true
```

## What This Does

Converts the synchronous `POST /api/v1/maintenance/cleanup-series` handler into a
self-registering `MaintenanceJob`. Business logic moves from `maintenance_fixups.go`
into a new job file. The old route remains untouched until ASYNC-CLEAN-1.

**Current handler:** `maintenance_fixups.go` ~line 350  
**Current params:** `dry_run` (query param, default true)  
**Current scale:** All series, 2 phases — single-book removal + duplicate merge  
**Current return:** `{dry_run, phase1_removed, phase2_merged, errors}`

## Files to Create / Edit

1. **Create** `internal/maintenance/jobs/cleanup_series.go`
2. **Create** `internal/maintenance/jobs/cleanup_series_test.go`
3. **Edit** `internal/server/server.go` — ensure `_ "github.com/jdfalk/audiobook-organizer/internal/maintenance/jobs"` blank import exists (check first; add once; subsequent wave tasks skip if already present)

## Step 1 — Create the job file

```go
// file: internal/maintenance/jobs/cleanup_series.go
// version: 1.0.0
// guid: c3d4e5f6-a7b8-9012-cdef-345678901234

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
	maintenance.Register(&CleanupSeriesJob{})
}

// CleanupSeriesParams are the accepted parameters for this job.
type CleanupSeriesParams struct {
	DryRun bool `json:"dry_run"`
}

// CleanupSeriesJob removes single-book series and merges duplicate series entries.
type CleanupSeriesJob struct {
	store database.Store
}

func (j *CleanupSeriesJob) InjectStore(s database.Store) { j.store = s }

func (j *CleanupSeriesJob) ID() string         { return "cleanup-series" }
func (j *CleanupSeriesJob) Name() string        { return "Cleanup Series" }
func (j *CleanupSeriesJob) Description() string {
	return "Removes single-book series and merges duplicate series entries."
}
func (j *CleanupSeriesJob) Category() string { return "library" }
func (j *CleanupSeriesJob) CanResume() bool  { return true }

func (j *CleanupSeriesJob) DefaultParams() any {
	return CleanupSeriesParams{DryRun: true}
}

func (j *CleanupSeriesJob) ValidateParams(raw json.RawMessage) error {
	var p CleanupSeriesParams
	return json.Unmarshal(raw, &p)
}

func (j *CleanupSeriesJob) Run(
	ctx context.Context,
	reporter operations.ProgressReporter,
	raw json.RawMessage,
	startFrom int,
) error {
	var params CleanupSeriesParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return fmt.Errorf("invalid params: %w", err)
	}

	// Phase 1: find series with exactly 1 book
	// move logic from maintenance_fixups.go ~line 350
	singleBookSeries, err := j.store.GetSeriesWithBookCount(1)
	if err != nil {
		return fmt.Errorf("failed to load single-book series: %w", err)
	}

	_ = reporter.UpdateProgress(0, len(singleBookSeries), fmt.Sprintf("Phase 1: found %d single-book series", len(singleBookSeries)))

	phase1Start := startFrom
	if phase1Start > len(singleBookSeries) {
		phase1Start = len(singleBookSeries)
	}

	for i := phase1Start; i < len(singleBookSeries); i++ {
		if reporter.IsCanceled() {
			return nil
		}
		if i%100 == 0 {
			_ = reporter.UpdateProgress(i, len(singleBookSeries), fmt.Sprintf("Phase 1: processing %d/%d", i, len(singleBookSeries)))
			operations.SaveCheckpoint(j.store, reporter.OperationID(), &operations.OperationState{
				PhaseIndex: i,
				PhaseTotal: len(singleBookSeries),
			})
		}

		s := singleBookSeries[i]
		if params.DryRun {
			_ = reporter.Log("info", fmt.Sprintf("[dry] Would remove single-book series: %q (id=%s)", s.Name, s.ID), nil)
			continue
		}
		// move removal logic from maintenance_fixups.go ~line 380
		if err := j.store.DeleteSeries(s.ID); err != nil {
			_ = reporter.Log("error", fmt.Sprintf("Failed to delete series %s: %v", s.ID, err), nil)
		}
	}

	// Checkpoint at phase boundary
	operations.SaveCheckpoint(j.store, reporter.OperationID(), &operations.OperationState{
		PhaseIndex: len(singleBookSeries),
		PhaseTotal: len(singleBookSeries),
	})

	// Phase 2: find series with same normalized name+author and merge duplicates
	// move logic from maintenance_fixups.go ~line 420
	duplicateGroups, err := j.store.GetDuplicateSeriesGroups()
	if err != nil {
		return fmt.Errorf("failed to load duplicate series groups: %w", err)
	}

	_ = reporter.UpdateProgress(0, len(duplicateGroups), fmt.Sprintf("Phase 2: found %d duplicate series groups", len(duplicateGroups)))

	phase2Start := 0
	if startFrom > len(singleBookSeries) {
		phase2Start = startFrom - len(singleBookSeries)
	}
	if phase2Start > len(duplicateGroups) {
		phase2Start = len(duplicateGroups)
	}

	for i := phase2Start; i < len(duplicateGroups); i++ {
		if reporter.IsCanceled() {
			return nil
		}
		if i%100 == 0 {
			_ = reporter.UpdateProgress(i, len(duplicateGroups), fmt.Sprintf("Phase 2: merging %d/%d", i, len(duplicateGroups)))
			operations.SaveCheckpoint(j.store, reporter.OperationID(), &operations.OperationState{
				PhaseIndex: len(singleBookSeries) + i,
				PhaseTotal: len(singleBookSeries) + len(duplicateGroups),
			})
		}

		grp := duplicateGroups[i]
		if params.DryRun {
			_ = reporter.Log("info", fmt.Sprintf("[dry] Would merge %d series into primary %q (id=%s)", len(grp.Duplicates), grp.Primary.Name, grp.Primary.ID), nil)
			continue
		}
		// move merge logic from maintenance_fixups.go ~line 460
		for _, dup := range grp.Duplicates {
			if err := j.store.MergeSeriesInto(dup.ID, grp.Primary.ID); err != nil {
				_ = reporter.Log("error", fmt.Sprintf("Failed to merge series %s into %s: %v", dup.ID, grp.Primary.ID, err), nil)
			}
		}
	}

	total := len(singleBookSeries) + len(duplicateGroups)
	_ = reporter.UpdateProgress(total, total, fmt.Sprintf("Done: removed %d single-book series, merged %d duplicate groups", len(singleBookSeries), len(duplicateGroups)))
	return nil
}
```

## Step 2 — Create the test file

```go
// file: internal/maintenance/jobs/cleanup_series_test.go
// version: 1.0.0
// guid: d4e5f6a7-b8c9-0123-defa-456789012345

package jobs_test

import (
	"encoding/json"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/maintenance/jobs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanupSeriesJob_ID(t *testing.T) {
	j := &jobs.CleanupSeriesJob{}
	assert.Equal(t, "cleanup-series", j.ID())
}

func TestCleanupSeriesJob_ValidateParams(t *testing.T) {
	j := &jobs.CleanupSeriesJob{}
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

This triggers all `init()` functions.

## Step 4 — Verify

```bash
go test ./internal/maintenance/jobs/... -run TestCleanupSeries
go vet ./internal/maintenance/jobs/...
make build-api
```

After building, confirm:
```bash
curl -s http://localhost:8080/api/v1/maintenance/jobs | jq '.jobs[] | select(.id=="cleanup-series")'
```
Should return the job definition.

## Definition of Done

- `internal/maintenance/jobs/cleanup_series.go` exists and compiles
- Job registers itself via `init()`
- `GET /api/v1/maintenance/jobs` includes `cleanup-series`
- `POST /api/v1/maintenance/jobs/cleanup-series` returns 202
- DryRun mode logs without modifying DB
- Two-phase execution with checkpoint at phase boundary
- IsCanceled() checked in both phase loops
- Checkpoint saved every 100 items per phase
- Tests pass

## PR Instructions

```bash
gh label create "task:ASYNC-W1-2" --color "e4e669" --description "Bot task: cleanup-series as MaintenanceJob" 2>/dev/null || true
gh pr create \
  --title "feat(maintenance): convert cleanup-series to MaintenanceJob" \
  --body "Moves business logic into a self-registering MaintenanceJob. Two-phase execution (single-book removal + duplicate merge) with checkpoint at phase boundary, IsCanceled() checks every item, resume support. Old route untouched. (ASYNC-W1-2)" \
  --label "task:ASYNC-W1-2"
```
