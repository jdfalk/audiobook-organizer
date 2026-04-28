<!-- file: docs/superpowers/bot-tasks/2026-04-28-async-core-2-dispatcher.md -->
<!-- version: 1.0.0 -->
<!-- guid: a7b8c9d0-e1f2-3456-abcd-789012345678 -->

# BOT TASK: Unified Maintenance — Dispatcher Handler + Resume

**TODO ID:** ASYNC-CORE-2  
**Audience:** burndown bot  
**Companion design:** [`docs/superpowers/specs/2026-04-28-unified-maintenance-system.md`](../specs/2026-04-28-unified-maintenance-system.md)

## Prerequisites

- `task:ASYNC-CORE-1` — maintenance interface + registry

```bash
count=$(gh pr list --label "task:ASYNC-CORE-1" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:ASYNC-CORE-1"; exit 0; }
```

## Branch

```
feat/async-core-2-maintenance-dispatcher
```

## Label

```bash
gh label create "task:ASYNC-CORE-2" --color "0075ca" --description "Bot task: maintenance dispatcher handler" 2>/dev/null || true
```

## Files to Create / Edit

1. **Create** `internal/server/maintenance_dispatcher.go` — new dispatcher + discovery handlers
2. **Edit** `internal/server/server.go` — add routes + call `maintenance.InjectStore()`
3. **Edit** `internal/server/server.go` — add generic catch-all in `resumeInterruptedOperations()`
4. **Edit** `internal/operations/state.go` — add `LoadRawParams` helper

## Step 1 — Add `LoadRawParams` to `internal/operations/state.go`

Find the file and add after the existing `LoadParams` function:

```go
// LoadRawParams loads the raw JSON params blob for an operation.
// Used by the generic resume path for MaintenanceJob implementations.
func LoadRawParams(store OperationParamStore, operationID string) json.RawMessage {
	raw, err := store.GetOperationParam(operationID, "params_raw")
	if err != nil || raw == "" {
		return json.RawMessage("{}")
	}
	return json.RawMessage(raw)
}
```

Also update `SaveParams` (or add a parallel `SaveRawParams`) to persist the raw JSON when a maintenance job is enqueued:

```go
// SaveRawParams persists the raw request body so it can be reloaded on resume.
func SaveRawParams(store OperationParamStore, operationID string, raw json.RawMessage) {
	_ = store.SetOperationParam(operationID, "params_raw", string(raw))
}
```

## Step 2 — Create `internal/server/maintenance_dispatcher.go`

```go
// file: internal/server/maintenance_dispatcher.go
// version: 1.0.0
// guid: b8c9d0e1-f2a3-4567-bcde-890123456789

package server

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/oklog/ulid/v2"

	"github.com/jdfalk/audiobook-organizer/internal/maintenance"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
)

// runMaintenanceJob dispatches POST /api/v1/maintenance/jobs/:job_id to the
// registered MaintenanceJob with that ID.
func (s *Server) runMaintenanceJob(c *gin.Context) {
	jobID := c.Param("job_id")
	job, ok := maintenance.Get(jobID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "unknown maintenance job: " + jobID})
		return
	}

	raw := json.RawMessage("{}")
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&raw); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
			return
		}
	}

	if err := job.ValidateParams(raw); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	opID := ulid.Make().String()
	rawCopy := raw // capture for closure

	err := s.queue.Enqueue(opID, jobID, operations.PriorityNormal,
		func(ctx context.Context, reporter operations.ProgressReporter) error {
			return job.Run(ctx, reporter, rawCopy, 0)
		})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to enqueue operation"})
		return
	}

	operations.SaveRawParams(s.Store(), opID, raw)
	c.JSON(http.StatusAccepted, gin.H{"operation_id": opID})
}

// listMaintenanceJobs handles GET /api/v1/maintenance/jobs
func (s *Server) listMaintenanceJobs(c *gin.Context) {
	jobs := maintenance.All()
	result := make([]gin.H, 0, len(jobs))
	for _, j := range jobs {
		result = append(result, gin.H{
			"id":             j.ID(),
			"name":           j.Name(),
			"description":    j.Description(),
			"category":       j.Category(),
			"default_params": j.DefaultParams(),
			"can_resume":     j.CanResume(),
		})
	}
	c.JSON(http.StatusOK, gin.H{"jobs": result})
}
```

## Step 3 — Register routes in `server.go`

Find the `protected` route group block (around line 2365) and add:

```go
// Unified maintenance job dispatcher
protected.GET("/maintenance/jobs", s.perm(auth.PermLibraryAdmin), s.listMaintenanceJobs)
protected.POST("/maintenance/jobs/:job_id", s.perm(auth.PermLibraryAdmin), s.runMaintenanceJob)
```

## Step 4 — Call `maintenance.InjectStore` during server startup

In the `Start()` or `NewServer()` function, after the store is initialized, add:

```go
maintenance.InjectStore(s.store)
```

## Step 5 — Add generic resume catch-all in `resumeInterruptedOperations()`

Find `resumeInterruptedOperations()` in `server.go`. At the END of the type-switch
(after all the existing `case` statements), add before the closing `}`:

```go
default:
	// Generic catch-all: delegate to registered MaintenanceJob if available
	if job, ok := maintenance.Get(opType); ok && job.CanResume() {
		rawParams := operations.LoadRawParams(store, opID)
		checkpoint := operations.LoadCheckpoint(store, opID)
		startFrom := 0
		if checkpoint != nil {
			startFrom = checkpoint.PhaseIndex
		}
		resumeFn := func(ctx context.Context, reporter operations.ProgressReporter) error {
			return job.Run(ctx, reporter, rawParams, startFrom)
		}
		oq.EnqueueResume(opID, opType, operations.PriorityLow, resumeFn)
		log.Printf("resumeInterruptedOperations: resumed maintenance job %s (op %s) from index %d", opType, opID, startFrom)
	} else {
		log.Printf("resumeInterruptedOperations: no resume handler for op type %q (op %s), marking failed", opType, opID)
		_ = store.UpdateOperationStatus(opID, "failed", "no resume handler")
	}
```

## Step 6 — Verify

```bash
make build-api
go test ./internal/server/... -run TestMaintenance
go vet ./...
```

## Definition of Done

- `GET /api/v1/maintenance/jobs` returns 200 with `{"jobs": [...]}`
- `POST /api/v1/maintenance/jobs/unknown-id` returns 404
- `POST /api/v1/maintenance/jobs/:id` with invalid JSON returns 400
- `POST /api/v1/maintenance/jobs/:id` with valid params returns 202 with `operation_id`
- `resumeInterruptedOperations` generic catch-all works for any registered job
- No existing routes changed or removed

## PR Instructions

```bash
gh label create "task:ASYNC-CORE-2" --color "0075ca" --description "Bot task: maintenance dispatcher handler" 2>/dev/null || true
gh pr create \
  --title "feat(maintenance): add unified job dispatcher and resume catch-all" \
  --body "Adds POST /api/v1/maintenance/jobs/:job_id dispatcher and GET /api/v1/maintenance/jobs discovery. Adds generic resume catch-all so any registered MaintenanceJob is automatically resumed on restart. No existing routes changed. (ASYNC-CORE-2)" \
  --label "task:ASYNC-CORE-2"
```
