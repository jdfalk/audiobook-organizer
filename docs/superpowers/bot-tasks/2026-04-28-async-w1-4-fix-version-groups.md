<!-- file: docs/superpowers/bot-tasks/2026-04-28-async-w1-4-fix-version-groups.md -->
<!-- version: 1.0.0 -->
<!-- guid: b8c9d0e1-f2a3-4567-bcde-890123456789 -->

# BOT TASK: Convert fix-version-groups to MaintenanceJob

**TODO ID:** ASYNC-W1-4  
**Audience:** burndown bot

## Prerequisites

- `task:ASYNC-CORE-2` — dispatcher must be live before any job can register

```bash
count=$(gh pr list --label "task:ASYNC-CORE-2" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:ASYNC-CORE-2"; exit 0; }
```

## Branch

```
feat/async-w1-4-fix-version-groups
```

## Label

```bash
gh label create "task:ASYNC-W1-4" --color "e4e669" --description "Bot task: fix-version-groups as MaintenanceJob" 2>/dev/null || true
```

## What This Does

Converts the synchronous `POST /api/v1/maintenance/fix-version-groups` handler into a
self-registering `MaintenanceJob`. Business logic moves from `maintenance_fixups.go`
into a new job file. The old route remains untouched until ASYNC-CLEAN-1.

**Current handler:** `maintenance_fixups.go` ~line 1279  
**Current params:** `dry_run` (query param, default true)  
**Current scale:** All version groups, 2 phases — mismatched author dirs + missing primary versions  
**Current return:** `{dry_run, phase1_fixed, phase2_fixed, errors}`

## Files to Create / Edit

1. **Create** `internal/maintenance/jobs/fix_version_groups.go`
2. **Create** `internal/maintenance/jobs/fix_version_groups_test.go`
3. **Edit** `internal/server/server.go` — ensure `_ "github.com/jdfalk/audiobook-organizer/internal/maintenance/jobs"` blank import exists (check first; add once)

## Step 1 — Create the job file

```go
// file: internal/maintenance/jobs/fix_version_groups.go
// version: 1.0.0
// guid: c9d0e1f2-a3b4-5678-cdef-901234567890

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
	maintenance.Register(&FixVersionGroupsJob{})
}

// FixVersionGroupsParams are the accepted parameters for this job.
type FixVersionGroupsParams struct {
	DryRun bool `json:"dry_run"`
}

// FixVersionGroupsJob repairs version groups with mismatched author directories or missing primary versions.
type FixVersionGroupsJob struct {
	store database.Store
}

func (j *FixVersionGroupsJob) InjectStore(s database.Store) { j.store = s }

func (j *FixVersionGroupsJob) ID() string         { return "fix-version-groups" }
func (j *FixVersionGroupsJob) Name() string        { return "Fix Version Groups" }
func (j *FixVersionGroupsJob) Description() string {
	return "Repairs version groups with mismatched author directories or missing primary versions."
}
func (j *FixVersionGroupsJob) Category() string { return "library" }
func (j *FixVersionGroupsJob) CanResume() bool  { return true }

func (j *FixVersionGroupsJob) DefaultParams() any {
	return FixVersionGroupsParams{DryRun: true}
}

func (j *FixVersionGroupsJob) ValidateParams(raw json.RawMessage) error {
	var p FixVersionGroupsParams
	return json.Unmarshal(raw, &p)
}

func (j *FixVersionGroupsJob) Run(
	ctx context.Context,
	reporter operations.ProgressReporter,
	raw json.RawMessage,
	startFrom int,
) error {
	var params FixVersionGroupsParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return fmt.Errorf("invalid params: %w", err)
	}

	// Load all version groups in one shot (check-then-apply)
	// move logic from maintenance_fixups.go ~line 1279
	groups, err := j.store.GetAllVersionGroups()
	if err != nil {
		return fmt.Errorf("failed to load version groups: %w", err)
	}

	// Phase 1: identify groups with mismatched author directories
	// move detection logic from maintenance_fixups.go ~line 1300
	type mismatchedGroup struct {
		groupID   string
		bookID    string
		wrongDir  string
		correctDir string
	}
	var phase1Targets []mismatchedGroup
	for _, g := range groups {
		// move mismatch detection from maintenance_fixups.go ~line 1310
		_ = g // placeholder: populate phase1Targets from actual logic
	}

	_ = reporter.UpdateProgress(0, len(phase1Targets), fmt.Sprintf("Phase 1: found %d groups with mismatched author dirs", len(phase1Targets)))

	phase1Start := startFrom
	if phase1Start > len(phase1Targets) {
		phase1Start = len(phase1Targets)
	}

	for i := phase1Start; i < len(phase1Targets); i++ {
		if reporter.IsCanceled() {
			return nil
		}
		if i%100 == 0 {
			_ = reporter.UpdateProgress(i, len(phase1Targets), fmt.Sprintf("Phase 1: fixing %d/%d", i, len(phase1Targets)))
			operations.SaveCheckpoint(j.store, reporter.OperationID(), &operations.OperationState{
				PhaseIndex: i,
				PhaseTotal: len(phase1Targets),
			})
		}

		t := phase1Targets[i]
		if params.DryRun {
			_ = reporter.Log("info", fmt.Sprintf("[dry] Would fix author dir: group %s book %s %q→%q", t.groupID, t.bookID, t.wrongDir, t.correctDir), nil)
			continue
		}
		// move fix logic from maintenance_fixups.go ~line 1340
	}

	// Checkpoint at phase boundary
	operations.SaveCheckpoint(j.store, reporter.OperationID(), &operations.OperationState{
		PhaseIndex: len(phase1Targets),
		PhaseTotal: len(phase1Targets),
	})

	// Phase 2: identify groups with no primary version set
	// move logic from maintenance_fixups.go ~line 1370
	type missingPrimaryGroup struct {
		groupID        string
		candidateBookID string
	}
	var phase2Targets []missingPrimaryGroup
	for _, g := range groups {
		// move missing-primary detection from maintenance_fixups.go ~line 1380
		_ = g // placeholder: populate phase2Targets from actual logic
	}

	_ = reporter.UpdateProgress(0, len(phase2Targets), fmt.Sprintf("Phase 2: found %d groups missing primary version", len(phase2Targets)))

	phase2Start := 0
	if startFrom > len(phase1Targets) {
		phase2Start = startFrom - len(phase1Targets)
	}
	if phase2Start > len(phase2Targets) {
		phase2Start = len(phase2Targets)
	}

	for i := phase2Start; i < len(phase2Targets); i++ {
		if reporter.IsCanceled() {
			return nil
		}
		if i%100 == 0 {
			_ = reporter.UpdateProgress(i, len(phase2Targets), fmt.Sprintf("Phase 2: setting primary %d/%d", i, len(phase2Targets)))
			operations.SaveCheckpoint(j.store, reporter.OperationID(), &operations.OperationState{
				PhaseIndex: len(phase1Targets) + i,
				PhaseTotal: len(phase1Targets) + len(phase2Targets),
			})
		}

		t := phase2Targets[i]
		if params.DryRun {
			_ = reporter.Log("info", fmt.Sprintf("[dry] Would set primary version: group %s → book %s", t.groupID, t.candidateBookID), nil)
			continue
		}
		// move primary-set logic from maintenance_fixups.go ~line 1420
	}

	total := len(phase1Targets) + len(phase2Targets)
	_ = reporter.UpdateProgress(total, total, fmt.Sprintf("Done: fixed %d mismatched dirs, set %d missing primaries", len(phase1Targets), len(phase2Targets)))
	return nil
}
```

## Step 2 — Create the test file

```go
// file: internal/maintenance/jobs/fix_version_groups_test.go
// version: 1.0.0
// guid: d0e1f2a3-b4c5-6789-defa-012345678901

package jobs_test

import (
	"encoding/json"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/maintenance/jobs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFixVersionGroupsJob_ID(t *testing.T) {
	j := &jobs.FixVersionGroupsJob{}
	assert.Equal(t, "fix-version-groups", j.ID())
}

func TestFixVersionGroupsJob_ValidateParams(t *testing.T) {
	j := &jobs.FixVersionGroupsJob{}
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
go test ./internal/maintenance/jobs/... -run TestFixVersionGroups
go vet ./internal/maintenance/jobs/...
make build-api
```

After building, confirm:
```bash
curl -s http://localhost:8080/api/v1/maintenance/jobs | jq '.jobs[] | select(.id=="fix-version-groups")'
```
Should return the job definition.

## Definition of Done

- `internal/maintenance/jobs/fix_version_groups.go` exists and compiles
- Job registers itself via `init()`
- `GET /api/v1/maintenance/jobs` includes `fix-version-groups`
- `POST /api/v1/maintenance/jobs/fix-version-groups` returns 202
- DryRun mode logs without modifying DB
- Two-phase execution with checkpoint at phase boundary
- IsCanceled() checked in both phase loops
- Checkpoint saved every 100 items per phase
- Tests pass

## PR Instructions

```bash
gh label create "task:ASYNC-W1-4" --color "e4e669" --description "Bot task: fix-version-groups as MaintenanceJob" 2>/dev/null || true
gh pr create \
  --title "feat(maintenance): convert fix-version-groups to MaintenanceJob" \
  --body "Moves business logic into a self-registering MaintenanceJob. Two-phase execution (mismatched author dirs + missing primary versions) with checkpoint at phase boundary, IsCanceled() checks every item, resume support. Old route untouched. (ASYNC-W1-4)" \
  --label "task:ASYNC-W1-4"
```
