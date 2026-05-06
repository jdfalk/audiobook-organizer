<!-- file: docs/superpowers/bot-tasks/2026-04-28-async-w2-3-cleanup-organize-mess.md -->
<!-- version: 1.0.0 -->
<!-- guid: f7a8b9c0-d1e2-3456-fabc-789012345012 -->

# BOT TASK: Convert cleanup-organize-mess to MaintenanceJob

**TODO ID:** ASYNC-W2-3  
**Audience:** burndown bot

## Prerequisites

- `task:ASYNC-CORE-2` — dispatcher must be live before any job can register

```bash
count=$(gh pr list --label "task:ASYNC-CORE-2" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:ASYNC-CORE-2"; exit 0; }
```

## Branch

```
feat/async-w2-3-cleanup-organize-mess
```

## Label

```bash
gh label create "task:ASYNC-W2-3" --color "e4e669" --description "Bot task: cleanup-organize-mess as MaintenanceJob" 2>/dev/null || true
```

## What This Does

Converts the synchronous `POST /api/v1/maintenance/cleanup-organize-mess` handler into
a self-registering `MaintenanceJob`. Business logic moves from `maintenance_fixups.go`
into a new job file. The old route remains untouched until ASYNC-CLEAN-1.

**Current handler:** `maintenance_fixups.go` ~line 993  
**Current params:** `dry_run` (query param, default true)  
**Current scale:** Full directory walk + pattern matching on filenames/dirs  
**Current return:** `{dry_run, removed_files, removed_dirs, errors}`

## Files to Create / Edit

1. **Create** `internal/maintenance/jobs/cleanup_organize_mess.go`
2. **Create** `internal/maintenance/jobs/cleanup_organize_mess_test.go`
3. **Edit** `internal/server/server.go` — ensure `_ "github.com/jdfalk/audiobook-organizer/internal/maintenance/jobs"` blank import exists (check first; add once)

## Step 1 — Create the job file

```go
// file: internal/maintenance/jobs/cleanup_organize_mess.go
// version: 1.0.0
// guid: a8b9c0d1-e2f3-4567-abcd-890123456123

package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/maintenance"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
)

func init() {
	maintenance.Register(&CleanupOrganizeMessJob{})
}

// CleanupOrganizeMessParams are the accepted parameters for this job.
type CleanupOrganizeMessParams struct {
	DryRun bool `json:"dry_run"`
}

// CleanupOrganizeMessJob removes empty directories and garbage organizer artifacts from the library.
type CleanupOrganizeMessJob struct {
	store database.Store
}

func (j *CleanupOrganizeMessJob) InjectStore(s database.Store) { j.store = s }

func (j *CleanupOrganizeMessJob) ID() string         { return "cleanup-organize-mess" }
func (j *CleanupOrganizeMessJob) Name() string        { return "Cleanup Organize Mess" }
func (j *CleanupOrganizeMessJob) Description() string {
	return "Removes empty directories and garbage organizer artifacts from the library."
}
func (j *CleanupOrganizeMessJob) Category() string { return "files" }
func (j *CleanupOrganizeMessJob) CanResume() bool  { return true }

func (j *CleanupOrganizeMessJob) DefaultParams() any {
	return CleanupOrganizeMessParams{DryRun: true}
}

func (j *CleanupOrganizeMessJob) ValidateParams(raw json.RawMessage) error {
	var p CleanupOrganizeMessParams
	return json.Unmarshal(raw, &p)
}

func (j *CleanupOrganizeMessJob) Run(
	ctx context.Context,
	reporter operations.ProgressReporter,
	raw json.RawMessage,
	startFrom int,
) error {
	var params CleanupOrganizeMessParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return fmt.Errorf("invalid params: %w", err)
	}

	libraryRoot, err := j.store.GetLibraryRoot()
	if err != nil {
		return fmt.Errorf("failed to get library root: %w", err)
	}

	// Collect all paths (files and dirs) that match garbage patterns
	// move pattern list from maintenance_fixups.go ~line 993
	// Patterns include: organizer temp dirs, .DS_Store, Thumbs.db, partial downloads, etc.
	var garbageFiles []string
	var garbageDirs []string
	_ = filepath.Walk(libraryRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		name := filepath.Base(path)
		// move garbage-file pattern matching from maintenance_fixups.go ~line 1010
		if isOrganizerGarbageFile(name) {
			garbageFiles = append(garbageFiles, path)
			return nil
		}
		if info.IsDir() && isOrganizerGarbageDir(name) {
			garbageDirs = append(garbageDirs, path)
			return filepath.SkipDir
		}
		return nil
	})

	// Sort dirs deepest first for safe bottom-up removal
	sort.Slice(garbageDirs, func(i, k int) bool { return len(garbageDirs[i]) > len(garbageDirs[k]) })

	allTargets := append(garbageFiles, garbageDirs...)
	_ = reporter.UpdateProgress(startFrom, len(allTargets), fmt.Sprintf("Found %d garbage files, %d garbage dirs", len(garbageFiles), len(garbageDirs)))

	for i := startFrom; i < len(allTargets); i++ {
		if reporter.IsCanceled() {
			return nil
		}
		if i%100 == 0 {
			_ = reporter.UpdateProgress(i, len(allTargets), fmt.Sprintf("Cleaning %d/%d", i, len(allTargets)))
			operations.SaveCheckpoint(j.store, reporter.OperationID(), &operations.OperationState{
				PhaseIndex: i,
				PhaseTotal: len(allTargets),
			})
		}

		target := allTargets[i]
		if params.DryRun {
			_ = reporter.Log("info", fmt.Sprintf("[dry] Would remove: %s", target), nil)
			continue
		}
		// move removal logic from maintenance_fixups.go ~line 1060
		if err := os.RemoveAll(target); err != nil {
			_ = reporter.Log("error", fmt.Sprintf("Failed to remove %s: %v", target, err), nil)
		}
	}

	_ = reporter.UpdateProgress(len(allTargets), len(allTargets), fmt.Sprintf("Done: %d items removed", len(allTargets)))
	return nil
}

// isOrganizerGarbageFile returns true if the filename matches known garbage patterns.
// move pattern list from maintenance_fixups.go ~line 1010
func isOrganizerGarbageFile(name string) bool {
	garbage := []string{".DS_Store", "Thumbs.db", "desktop.ini", ".Spotlight-V100"}
	for _, g := range garbage {
		if strings.EqualFold(name, g) {
			return true
		}
	}
	return false
}

// isOrganizerGarbageDir returns true if the directory name matches known garbage patterns.
// move pattern list from maintenance_fixups.go ~line 1025
func isOrganizerGarbageDir(name string) bool {
	garbage := []string{".Trashes", ".fseventsd", ".TemporaryItems"}
	for _, g := range garbage {
		if strings.EqualFold(name, g) {
			return true
		}
	}
	return false
}
```

## Step 2 — Create the test file

```go
// file: internal/maintenance/jobs/cleanup_organize_mess_test.go
// version: 1.0.0
// guid: b9c0d1e2-f3a4-5678-bcde-901234567234

package jobs_test

import (
	"encoding/json"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/maintenance/jobs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanupOrganizeMessJob_ID(t *testing.T) {
	j := &jobs.CleanupOrganizeMessJob{}
	assert.Equal(t, "cleanup-organize-mess", j.ID())
}

func TestCleanupOrganizeMessJob_ValidateParams(t *testing.T) {
	j := &jobs.CleanupOrganizeMessJob{}
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
go test ./internal/maintenance/jobs/... -run TestCleanupOrganizeMess
go vet ./internal/maintenance/jobs/...
make build-api
```

After building, confirm:
```bash
curl -s http://localhost:8080/api/v1/maintenance/jobs | jq '.jobs[] | select(.id=="cleanup-organize-mess")'
```
Should return the job definition.

## Definition of Done

- `internal/maintenance/jobs/cleanup_organize_mess.go` exists and compiles
- Job registers itself via `init()`
- `GET /api/v1/maintenance/jobs` includes `cleanup-organize-mess`
- `POST /api/v1/maintenance/jobs/cleanup-organize-mess` returns 202
- DryRun mode logs without modifying filesystem
- Garbage patterns extracted from original handler (read ~line 993-1080)
- IsCanceled() checked in main loop
- Checkpoint saved every 100 items
- Tests pass

## PR Instructions

```bash
gh label create "task:ASYNC-W2-3" --color "e4e669" --description "Bot task: cleanup-organize-mess as MaintenanceJob" 2>/dev/null || true
gh pr create \
  --title "feat(maintenance): convert cleanup-organize-mess to MaintenanceJob" \
  --body "Moves business logic into a self-registering MaintenanceJob. Full directory walk with pattern matching, IsCanceled() checks, checkpoint every 100 items, and resume support. Old route untouched. (ASYNC-W2-3)" \
  --label "task:ASYNC-W2-3"
```
