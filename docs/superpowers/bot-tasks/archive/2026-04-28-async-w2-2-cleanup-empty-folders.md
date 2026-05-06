<!-- file: docs/superpowers/bot-tasks/2026-04-28-async-w2-2-cleanup-empty-folders.md -->
<!-- version: 1.0.0 -->
<!-- guid: c4d5e6f7-a8b9-0123-cdef-456789012def -->

# BOT TASK: Convert cleanup-empty-folders to MaintenanceJob

**TODO ID:** ASYNC-W2-2  
**Audience:** burndown bot

## Prerequisites

- `task:ASYNC-CORE-2` — dispatcher must be live before any job can register

```bash
count=$(gh pr list --label "task:ASYNC-CORE-2" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:ASYNC-CORE-2"; exit 0; }
```

## Branch

```
feat/async-w2-2-cleanup-empty-folders
```

## Label

```bash
gh label create "task:ASYNC-W2-2" --color "e4e669" --description "Bot task: cleanup-empty-folders as MaintenanceJob" 2>/dev/null || true
```

## What This Does

Converts the synchronous `POST /api/v1/maintenance/cleanup-empty-folders` handler
into a self-registering `MaintenanceJob`. Business logic moves from
`maintenance_fixups.go` into a new job file. The old route remains untouched until
ASYNC-CLEAN-1.

**Current handler:** `maintenance_fixups.go` ~line 817  
**Current params:** `dry_run` (query param, default true)  
**Current scale:** Full directory walk (bottom-up), unbounded count  
**Current return:** `{dry_run, removed, errors}`

## Files to Create / Edit

1. **Create** `internal/maintenance/jobs/cleanup_empty_folders.go`
2. **Create** `internal/maintenance/jobs/cleanup_empty_folders_test.go`
3. **Edit** `internal/server/server.go` — ensure `_ "github.com/jdfalk/audiobook-organizer/internal/maintenance/jobs"` blank import exists (check first; add once)

## Step 1 — Create the job file

```go
// file: internal/maintenance/jobs/cleanup_empty_folders.go
// version: 1.0.0
// guid: d5e6f7a8-b9c0-1234-defa-567890123ef0

package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/maintenance"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
)

func init() {
	maintenance.Register(&CleanupEmptyFoldersJob{})
}

// CleanupEmptyFoldersParams are the accepted parameters for this job.
type CleanupEmptyFoldersParams struct {
	DryRun bool `json:"dry_run"`
}

// CleanupEmptyFoldersJob removes empty directories from the library root (bottom-up walk).
type CleanupEmptyFoldersJob struct {
	store database.Store
}

func (j *CleanupEmptyFoldersJob) InjectStore(s database.Store) { j.store = s }

func (j *CleanupEmptyFoldersJob) ID() string         { return "cleanup-empty-folders" }
func (j *CleanupEmptyFoldersJob) Name() string        { return "Cleanup Empty Folders" }
func (j *CleanupEmptyFoldersJob) Description() string {
	return "Removes empty directories from the library root (bottom-up walk)."
}
func (j *CleanupEmptyFoldersJob) Category() string { return "files" }
func (j *CleanupEmptyFoldersJob) CanResume() bool  { return true }

func (j *CleanupEmptyFoldersJob) DefaultParams() any {
	return CleanupEmptyFoldersParams{DryRun: true}
}

func (j *CleanupEmptyFoldersJob) ValidateParams(raw json.RawMessage) error {
	var p CleanupEmptyFoldersParams
	return json.Unmarshal(raw, &p)
}

func (j *CleanupEmptyFoldersJob) Run(
	ctx context.Context,
	reporter operations.ProgressReporter,
	raw json.RawMessage,
	startFrom int,
) error {
	var params CleanupEmptyFoldersParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return fmt.Errorf("invalid params: %w", err)
	}

	// Resolve library root from store config
	// move config lookup from maintenance_fixups.go ~line 817
	libraryRoot, err := j.store.GetLibraryRoot()
	if err != nil {
		return fmt.Errorf("failed to get library root: %w", err)
	}

	// Collect all directories in a bottom-up order (deepest first)
	// move walk logic from maintenance_fixups.go ~line 830
	var dirs []string
	_ = filepath.Walk(libraryRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() || path == libraryRoot {
			return nil
		}
		dirs = append(dirs, path)
		return nil
	})
	// Sort deepest first so we remove children before parents
	sort.Slice(dirs, func(i, k int) bool { return len(dirs[i]) > len(dirs[k]) })

	_ = reporter.UpdateProgress(startFrom, len(dirs), fmt.Sprintf("Found %d directories to check", len(dirs)))

	for i := startFrom; i < len(dirs); i++ {
		if reporter.IsCanceled() {
			return nil
		}
		if i%100 == 0 {
			_ = reporter.UpdateProgress(i, len(dirs), fmt.Sprintf("Checking %d/%d directories", i, len(dirs)))
			operations.SaveCheckpoint(j.store, reporter.OperationID(), &operations.OperationState{
				PhaseIndex: i,
				PhaseTotal: len(dirs),
			})
		}

		dir := dirs[i]
		// Check if directory is empty
		// move emptiness check from maintenance_fixups.go ~line 850
		entries, err := os.ReadDir(dir)
		if err != nil {
			_ = reporter.Log("error", fmt.Sprintf("Failed to read dir %s: %v", dir, err), nil)
			continue
		}
		if len(entries) > 0 {
			continue
		}

		if params.DryRun {
			_ = reporter.Log("info", fmt.Sprintf("[dry] Would remove empty dir: %s", dir), nil)
			continue
		}
		// move removal logic from maintenance_fixups.go ~line 865
		if err := os.Remove(dir); err != nil {
			_ = reporter.Log("error", fmt.Sprintf("Failed to remove %s: %v", dir, err), nil)
		}
	}

	_ = reporter.UpdateProgress(len(dirs), len(dirs), fmt.Sprintf("Done: %d directories checked", len(dirs)))
	return nil
}
```

## Step 2 — Create the test file

```go
// file: internal/maintenance/jobs/cleanup_empty_folders_test.go
// version: 1.0.0
// guid: e6f7a8b9-c0d1-2345-efab-678901234f01

package jobs_test

import (
	"encoding/json"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/maintenance/jobs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanupEmptyFoldersJob_ID(t *testing.T) {
	j := &jobs.CleanupEmptyFoldersJob{}
	assert.Equal(t, "cleanup-empty-folders", j.ID())
}

func TestCleanupEmptyFoldersJob_ValidateParams(t *testing.T) {
	j := &jobs.CleanupEmptyFoldersJob{}
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
go test ./internal/maintenance/jobs/... -run TestCleanupEmptyFolders
go vet ./internal/maintenance/jobs/...
make build-api
```

After building, confirm:
```bash
curl -s http://localhost:8080/api/v1/maintenance/jobs | jq '.jobs[] | select(.id=="cleanup-empty-folders")'
```
Should return the job definition.

## Definition of Done

- `internal/maintenance/jobs/cleanup_empty_folders.go` exists and compiles
- Job registers itself via `init()`
- `GET /api/v1/maintenance/jobs` includes `cleanup-empty-folders`
- `POST /api/v1/maintenance/jobs/cleanup-empty-folders` returns 202
- DryRun mode logs without modifying filesystem
- Bottom-up directory ordering (deepest first)
- IsCanceled() checked in main loop
- Checkpoint saved every 100 directories
- Tests pass

## PR Instructions

```bash
gh label create "task:ASYNC-W2-2" --color "e4e669" --description "Bot task: cleanup-empty-folders as MaintenanceJob" 2>/dev/null || true
gh pr create \
  --title "feat(maintenance): convert cleanup-empty-folders to MaintenanceJob" \
  --body "Moves business logic into a self-registering MaintenanceJob. Bottom-up directory walk with IsCanceled() checks, checkpoint every 100 dirs, and resume support. Old route untouched. (ASYNC-W2-2)" \
  --label "task:ASYNC-W2-2"
```
