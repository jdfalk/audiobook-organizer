<!-- file: docs/superpowers/specs/2026-04-28-unified-maintenance-system.md -->
<!-- version: 1.0.0 -->
<!-- guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890 -->

# Design: Unified Maintenance System

**Status:** Spec written, bot-tasks written — ready for burndown bot  
**TODO IDs:** ASYNC-CORE-1..4, ASYNC-W1-1..4, ASYNC-W2-1..4, ASYNC-W3-1..5, ASYNC-CLEAN-1  
**Replaces:** `docs/superpowers/specs/2026-04-28-async-operations-design.md` (superseded)

---

## Problem

Every maintenance one-off fix requires:
1. A new handler method on `*Server` (~50–200 lines of boilerplate)
2. Manual route registration in `server.go`
3. Manual async wrapping in the queue
4. Manual resume wiring in `resumeInterruptedOperations()`
5. Manual progress reporting setup
6. A hardcoded button in `MaintenanceTab.tsx`

Result: 14 handlers × 6 concerns = 84 wiring points, all synchronous, no progress visibility, no cancellation, and repeated forever for every future fix.

---

## Solution: `internal/maintenance` Package

A self-registering job interface. Each new fix is **one file, one struct, zero wiring**.

### The Interface

```go
// internal/maintenance/job.go

package maintenance

import (
    "context"
    "encoding/json"

    "github.com/jdfalk/audiobook-organizer/internal/operations"
)

// MaintenanceJob is the contract every maintenance fix must satisfy.
// Implement this interface and call Register() in an init() function.
type MaintenanceJob interface {
    // Identity
    ID() string          // URL-safe slug; becomes the route param and operation type
    Name() string        // Human label, e.g. "Fix Read-by Narrator"
    Description() string // One sentence shown in the UI
    Category() string    // UI grouping: "library" | "files" | "itunes" | "dedup" | "cleanup"

    // Parameters
    // DefaultParams returns the zero-value params struct for JSON schema generation.
    DefaultParams() any
    // ValidateParams parses and validates raw JSON body; return error for 4xx.
    ValidateParams(raw json.RawMessage) error

    // Execution
    // Run executes the job. startFrom=0 for fresh run, >0 to resume from checkpoint.
    Run(ctx context.Context, reporter operations.ProgressReporter, params json.RawMessage, startFrom int) error

    // Resume
    CanResume() bool // true if Run supports startFrom > 0
}
```

### The Registry

```go
// internal/maintenance/registry.go

package maintenance

import (
    "sort"
    "sync"
)

var (
    mu       sync.RWMutex
    registry = map[string]MaintenanceJob{}
)

// Register adds a job to the global registry. Call from init().
func Register(job MaintenanceJob) {
    mu.Lock()
    defer mu.Unlock()
    registry[job.ID()] = job
}

// Get returns a job by ID.
func Get(id string) (MaintenanceJob, bool) {
    mu.RLock()
    defer mu.RUnlock()
    j, ok := registry[id]
    return j, ok
}

// All returns all registered jobs sorted by ID.
func All() []MaintenanceJob {
    mu.RLock()
    defer mu.RUnlock()
    jobs := make([]MaintenanceJob, 0, len(registry))
    for _, j := range registry {
        jobs = append(jobs, j)
    }
    sort.Slice(jobs, func(i, k int) bool { return jobs[i].ID() < jobs[k].ID() })
    return jobs
}
```

### A Concrete Job (example)

```go
// internal/maintenance/jobs/fix_read_by_narrator.go

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
    // store is set after construction; see maintenance.SetStore()
    maintenance.Register(&FixReadByNarratorJob{})
}

type FixReadByNarratorParams struct {
    DryRun bool `json:"dry_run"`
}

type FixReadByNarratorJob struct {
    store database.Store
}

func (j *FixReadByNarratorJob) ID()          string { return "fix-read-by-narrator" }
func (j *FixReadByNarratorJob) Name()        string { return "Fix Read-by Narrator" }
func (j *FixReadByNarratorJob) Description() string {
    return "Strips 'read by NARRATOR' suffixes from author names inserted by some importers."
}
func (j *FixReadByNarratorJob) Category() string { return "library" }
func (j *FixReadByNarratorJob) CanResume()  bool   { return true }

func (j *FixReadByNarratorJob) DefaultParams() any {
    return FixReadByNarratorParams{DryRun: true}
}

func (j *FixReadByNarratorJob) ValidateParams(raw json.RawMessage) error {
    var p FixReadByNarratorParams
    return json.Unmarshal(raw, &p)
}

func (j *FixReadByNarratorJob) Run(ctx context.Context, reporter operations.ProgressReporter, raw json.RawMessage, startFrom int) error {
    var params FixReadByNarratorParams
    if err := json.Unmarshal(raw, &params); err != nil {
        return err
    }

    books, err := j.store.GetAllBooks()
    if err != nil {
        return err
    }

    affected := filterReadByNarrator(books)
    reporter.UpdateProgress(startFrom, len(affected), "Scanning...")

    for i := startFrom; i < len(affected); i++ {
        if reporter.IsCanceled() {
            return nil
        }
        // checkpoint every 100
        if i%100 == 0 {
            reporter.UpdateProgress(i, len(affected), fmt.Sprintf("Fixing %d/%d", i, len(affected)))
            operations.SaveCheckpoint(j.store, reporter.OperationID(), &operations.OperationState{
                PhaseIndex: i,
                PhaseTotal: len(affected),
            })
        }
        if params.DryRun {
            reporter.Log("info", fmt.Sprintf("Would fix: %s", affected[i].Title), nil)
            continue
        }
        // ... apply fix
    }
    return nil
}
```

### Store Injection

Jobs registered via `init()` don't have access to the store yet. The server calls
`maintenance.InjectStore(store)` during startup, which iterates the registry and
injects the store into each job that embeds `StoreHolder`:

```go
// internal/maintenance/registry.go (addition)

type StoreInjectable interface {
    InjectStore(store database.Store)
}

func InjectStore(store database.Store) {
    mu.Lock()
    defer mu.Unlock()
    for _, j := range registry {
        if si, ok := j.(StoreInjectable); ok {
            si.InjectStore(store)
        }
    }
}
```

Each job embeds:
```go
type FixReadByNarratorJob struct {
    store database.Store
}
func (j *FixReadByNarratorJob) InjectStore(s database.Store) { j.store = s }
```

---

## HTTP Dispatcher

### Single handler replaces 14 routes

```go
// internal/server/maintenance_dispatcher.go

// POST /api/v1/maintenance/jobs/:job_id
func (s *Server) runMaintenanceJob(c *gin.Context) {
    jobID := c.Param("job_id")
    job, ok := maintenance.Get(jobID)
    if !ok {
        c.JSON(404, gin.H{"error": "unknown job: " + jobID})
        return
    }

    raw := json.RawMessage("{}")
    if c.Request.ContentLength != 0 {
        if err := c.ShouldBindJSON(&raw); err != nil {
            c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
            return
        }
    }

    if err := job.ValidateParams(raw); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }

    opID := ulid.Make().String()
    err := s.queue.Enqueue(opID, jobID, operations.PriorityNormal,
        func(ctx context.Context, reporter operations.ProgressReporter) error {
            return job.Run(ctx, reporter, raw, 0)
        })
    if err != nil {
        c.JSON(500, gin.H{"error": "failed to enqueue"})
        return
    }

    c.JSON(202, gin.H{"operation_id": opID})
}

// GET /api/v1/maintenance/jobs
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
    c.JSON(200, gin.H{"jobs": result})
}
```

### Resume Integration

In `resumeInterruptedOperations()`, add a generic catch-all AFTER the type-switch:

```go
// After the existing type switch:
if _, handled := handledTypes[opType]; !handled {
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
    }
}
```

---

## Frontend: Dynamic Maintenance Tab

`GET /api/v1/maintenance/jobs` returns the full job list. The frontend renders:
- Category tabs: Library / Files / iTunes / Dedup / Cleanup
- Per-job card: name, description, a `dry_run` toggle (if `default_params.dry_run` exists), "Run" button
- "Run" POSTs to `/api/v1/maintenance/jobs/:id` with `{"dry_run": true/false}`
- Operation ID returned → `useOperationsStore.startPolling()` shows toast + badge

The old hardcoded maintenance buttons remain until ASYNC-CLEAN-1 removes the old routes.

---

## Execution Plan

See bot-task files in `docs/superpowers/bot-tasks/2026-04-28-async-*.md`.

Dependency graph:
```
ASYNC-CORE-1 (interface + registry)
    ↓
ASYNC-CORE-2 (dispatcher handler + resume wiring)
    ↓
ASYNC-CORE-3 (discovery endpoint)  ←→  ASYNC-W1..W3 (all can run in parallel after CORE-2)
    ↓
ASYNC-CORE-4 (frontend dynamic tab)
    ↓
ASYNC-CLEAN-1 (remove old routes + old handlers — last, depends on ALL waves done)
```

Waves W1–W3 (13 handler conversions) are independent of each other and can all
run in parallel after ASYNC-CORE-2 merges. Dispatch as one `/parallel-sweep`.
