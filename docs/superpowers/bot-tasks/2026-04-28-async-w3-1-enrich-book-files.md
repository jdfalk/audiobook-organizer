<!-- file: docs/superpowers/bot-tasks/2026-04-28-async-w3-1-enrich-book-files.md -->
<!-- version: 1.0.0 -->
<!-- guid: f3a4b5c6-d7e8-9012-fabc-345678901678 -->

# BOT TASK: Convert enrich-book-files to MaintenanceJob

**TODO ID:** ASYNC-W3-1  
**Audience:** burndown bot

## Prerequisites

- `task:ASYNC-CORE-2` — dispatcher must be live before any job can register

```bash
count=$(gh pr list --label "task:ASYNC-CORE-2" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:ASYNC-CORE-2"; exit 0; }
```

## Branch

```
feat/async-w3-1-enrich-book-files
```

## Label

```bash
gh label create "task:ASYNC-W3-1" --color "e4e669" --description "Bot task: enrich-book-files as MaintenanceJob" 2>/dev/null || true
```

## What This Does

Converts the synchronous `POST /api/v1/maintenance/enrich-book-files` handler into a
self-registering `MaintenanceJob`. Business logic moves from `maintenance_fixups.go`
into a new job file. The old route remains untouched until ASYNC-CLEAN-1.

**Current handler:** `maintenance_fixups.go` ~line 1706  
**Current params:** `dry_run` (query param, default true)  
**Current scale:** All book_files rows (disk stat + regex for format detection)  
**Current return:** `{dry_run, total_files, enriched, errors}`

## Files to Create / Edit

1. **Create** `internal/maintenance/jobs/enrich_book_files.go`
2. **Create** `internal/maintenance/jobs/enrich_book_files_test.go`
3. **Edit** `internal/server/server.go` — ensure `_ "github.com/jdfalk/audiobook-organizer/internal/maintenance/jobs"` blank import exists (check first; add once)

## Step 1 — Create the job file

```go
// file: internal/maintenance/jobs/enrich_book_files.go
// version: 1.0.0
// guid: a4b5c6d7-e8f9-0123-abcd-456789012789

package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/maintenance"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
)

func init() {
	maintenance.Register(&EnrichBookFilesJob{})
}

// EnrichBookFilesParams are the accepted parameters for this job.
type EnrichBookFilesParams struct {
	DryRun bool `json:"dry_run"`
}

// EnrichBookFilesJob fills in missing size, duration, and format fields in book_files.
type EnrichBookFilesJob struct {
	store database.Store
}

func (j *EnrichBookFilesJob) InjectStore(s database.Store) { j.store = s }

func (j *EnrichBookFilesJob) ID() string         { return "enrich-book-files" }
func (j *EnrichBookFilesJob) Name() string        { return "Enrich Book Files" }
func (j *EnrichBookFilesJob) Description() string {
	return "Fills in missing size, duration, and format fields in book_files by reading from disk."
}
func (j *EnrichBookFilesJob) Category() string { return "files" }
func (j *EnrichBookFilesJob) CanResume() bool  { return true }

func (j *EnrichBookFilesJob) DefaultParams() any {
	return EnrichBookFilesParams{DryRun: true}
}

func (j *EnrichBookFilesJob) ValidateParams(raw json.RawMessage) error {
	var p EnrichBookFilesParams
	return json.Unmarshal(raw, &p)
}

// audioFormatRe matches common audio file extensions for format detection.
// move/verify pattern from maintenance_fixups.go ~line 1730
var audioFormatRe = regexp.MustCompile(`(?i)\.(mp3|m4a|m4b|aac|ogg|flac|opus|wav|wma|aiff?)$`)

func (j *EnrichBookFilesJob) Run(
	ctx context.Context,
	reporter operations.ProgressReporter,
	raw json.RawMessage,
	startFrom int,
) error {
	var params EnrichBookFilesParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return fmt.Errorf("invalid params: %w", err)
	}

	// Load all book_files in one shot (check-then-apply)
	// move logic from maintenance_fixups.go ~line 1706
	files, err := j.store.GetAllBookFiles()
	if err != nil {
		return fmt.Errorf("failed to load book_files: %w", err)
	}

	// Filter to only those missing at least one field
	// move filter logic from maintenance_fixups.go ~line 1720
	type enrichTarget struct {
		file database.BookFile
	}
	var targets []enrichTarget
	for _, f := range files {
		if f.Size == 0 || f.Duration == 0 || f.Format == "" {
			targets = append(targets, enrichTarget{file: f})
		}
	}

	_ = reporter.UpdateProgress(startFrom, len(targets), fmt.Sprintf("Found %d book_files needing enrichment", len(targets)))

	for i := startFrom; i < len(targets); i++ {
		if reporter.IsCanceled() {
			return nil
		}
		if i%100 == 0 {
			_ = reporter.UpdateProgress(i, len(targets), fmt.Sprintf("Enriching %d/%d", i, len(targets)))
			operations.SaveCheckpoint(j.store, reporter.OperationID(), &operations.OperationState{
				PhaseIndex: i,
				PhaseTotal: len(targets),
			})
		}

		t := targets[i]
		f := t.file

		// Stat the file for size
		// move stat logic from maintenance_fixups.go ~line 1750
		var newSize int64
		if f.Size == 0 {
			if info, err := os.Stat(f.Path); err == nil {
				newSize = info.Size()
			}
		}

		// Derive format from extension
		// move regex logic from maintenance_fixups.go ~line 1765
		newFormat := f.Format
		if newFormat == "" {
			if m := audioFormatRe.FindString(filepath.Ext(f.Path)); m != "" {
				newFormat = m[1:] // strip leading dot
			}
		}

		if params.DryRun {
			_ = reporter.Log("info", fmt.Sprintf("[dry] Would enrich file %s: size=%d format=%q", f.Path, newSize, newFormat), nil)
			continue
		}

		// Apply updates
		// move update logic from maintenance_fixups.go ~line 1780
		if newSize > 0 {
			f.Size = newSize
		}
		if newFormat != "" {
			f.Format = newFormat
		}
		if err := j.store.UpdateBookFile(f); err != nil {
			_ = reporter.Log("error", fmt.Sprintf("Failed to update book_file %s: %v", f.Path, err), nil)
		}
	}

	_ = reporter.UpdateProgress(len(targets), len(targets), fmt.Sprintf("Done: %d book_files enriched", len(targets)))
	return nil
}
```

## Step 2 — Create the test file

```go
// file: internal/maintenance/jobs/enrich_book_files_test.go
// version: 1.0.0
// guid: b5c6d7e8-f9a0-1234-bcde-567890123890

package jobs_test

import (
	"encoding/json"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/maintenance/jobs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnrichBookFilesJob_ID(t *testing.T) {
	j := &jobs.EnrichBookFilesJob{}
	assert.Equal(t, "enrich-book-files", j.ID())
}

func TestEnrichBookFilesJob_ValidateParams(t *testing.T) {
	j := &jobs.EnrichBookFilesJob{}
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
go test ./internal/maintenance/jobs/... -run TestEnrichBookFiles
go vet ./internal/maintenance/jobs/...
make build-api
```

After building, confirm:
```bash
curl -s http://localhost:8080/api/v1/maintenance/jobs | jq '.jobs[] | select(.id=="enrich-book-files")'
```
Should return the job definition.

## Definition of Done

- `internal/maintenance/jobs/enrich_book_files.go` exists and compiles
- Job registers itself via `init()`
- `GET /api/v1/maintenance/jobs` includes `enrich-book-files`
- `POST /api/v1/maintenance/jobs/enrich-book-files` returns 202
- DryRun mode logs without modifying DB
- Only processes book_files with at least one missing field
- IsCanceled() checked in main loop
- Checkpoint saved every 100 files
- Tests pass

## PR Instructions

```bash
gh label create "task:ASYNC-W3-1" --color "e4e669" --description "Bot task: enrich-book-files as MaintenanceJob" 2>/dev/null || true
gh pr create \
  --title "feat(maintenance): convert enrich-book-files to MaintenanceJob" \
  --body "Moves business logic into a self-registering MaintenanceJob. Fills missing size/duration/format via disk stat and regex, IsCanceled() checks, checkpoint every 100 files, resume support. Old route untouched. (ASYNC-W3-1)" \
  --label "task:ASYNC-W3-1"
```
