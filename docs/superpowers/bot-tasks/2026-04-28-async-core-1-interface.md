<!-- file: docs/superpowers/bot-tasks/2026-04-28-async-core-1-interface.md -->
<!-- version: 1.0.0 -->
<!-- guid: c3d4e5f6-a7b8-9012-cdef-345678901234 -->

# BOT TASK: Unified Maintenance — Interface + Registry

**TODO ID:** ASYNC-CORE-1  
**Audience:** burndown bot  
**Companion design:** [`docs/superpowers/specs/2026-04-28-unified-maintenance-system.md`](../specs/2026-04-28-unified-maintenance-system.md)

## Prerequisites

None — this is the foundation task.

## Branch

```
feat/async-core-1-maintenance-interface
```

## Label

```bash
gh label create "task:ASYNC-CORE-1" --color "0075ca" --description "Bot task: maintenance interface + registry" 2>/dev/null || true
```

## Files to Create

1. `internal/maintenance/job.go` — the interface
2. `internal/maintenance/registry.go` — the registry + store injection
3. `internal/maintenance/job_test.go` — registry unit tests

## Step 1 — Create `internal/maintenance/job.go`

```go
// file: internal/maintenance/job.go
// version: 1.0.0
// guid: d4e5f6a7-b8c9-0123-defa-456789012345

package maintenance

import (
	"context"
	"encoding/json"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
)

// MaintenanceJob is implemented by every self-registering maintenance fix.
// Add a new fix by implementing this interface, embedding StoreHolder,
// and calling maintenance.Register(&YourJob{}) in an init() function.
type MaintenanceJob interface {
	// Identity
	ID() string          // URL-safe slug; used as route param and operation type
	Name() string        // Human label shown in UI
	Description() string // One sentence describing what the job does
	Category() string    // UI grouping: "library" | "files" | "itunes" | "dedup" | "cleanup"

	// Parameters
	DefaultParams() any                         // Zero-value params struct for UI/schema generation
	ValidateParams(raw json.RawMessage) error   // Fast pre-flight; return non-nil for 400 response

	// Execution
	// Run executes the job. startFrom=0 for fresh run; startFrom>0 resumes from that index.
	Run(ctx context.Context, reporter operations.ProgressReporter, params json.RawMessage, startFrom int) error

	// Resume
	CanResume() bool // true when Run correctly handles startFrom > 0
}

// StoreInjectable is satisfied by jobs that need a database.Store.
// The dispatcher calls InjectStore on all registered jobs during server startup.
type StoreInjectable interface {
	InjectStore(store database.Store)
}
```

## Step 2 — Create `internal/maintenance/registry.go`

```go
// file: internal/maintenance/registry.go
// version: 1.0.0
// guid: e5f6a7b8-c9d0-1234-efab-567890123456

package maintenance

import (
	"sort"
	"sync"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

var (
	mu       sync.RWMutex
	registry = map[string]MaintenanceJob{}
)

// Register adds a job to the global registry. Must be called from an init() function.
func Register(job MaintenanceJob) {
	mu.Lock()
	defer mu.Unlock()
	registry[job.ID()] = job
}

// Get returns the job for the given ID, or false if not found.
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

// InjectStore calls InjectStore on every registered job that implements StoreInjectable.
// Call this from the server after the store is initialized.
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

## Step 3 — Create `internal/maintenance/job_test.go`

```go
// file: internal/maintenance/job_test.go
// version: 1.0.0
// guid: f6a7b8c9-d0e1-2345-fabc-678901234567

package maintenance_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/maintenance"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubJob struct {
	id       string
	runCalls int
}

func (s *stubJob) ID() string                   { return s.id }
func (s *stubJob) Name() string                 { return "Stub" }
func (s *stubJob) Description() string          { return "stub" }
func (s *stubJob) Category() string             { return "library" }
func (s *stubJob) DefaultParams() any           { return struct{}{} }
func (s *stubJob) ValidateParams(json.RawMessage) error { return nil }
func (s *stubJob) CanResume() bool              { return false }
func (s *stubJob) Run(_ context.Context, _ operations.ProgressReporter, _ json.RawMessage, _ int) error {
	s.runCalls++
	return nil
}

func TestRegisterAndGet(t *testing.T) {
	job := &stubJob{id: "test-job-" + t.Name()}
	maintenance.Register(job)

	got, ok := maintenance.Get(job.ID())
	require.True(t, ok)
	assert.Equal(t, job.ID(), got.ID())
}

func TestAllReturnsSorted(t *testing.T) {
	maintenance.Register(&stubJob{id: "zzz-job"})
	maintenance.Register(&stubJob{id: "aaa-job"})

	all := maintenance.All()
	require.GreaterOrEqual(t, len(all), 2)
	for i := 1; i < len(all); i++ {
		assert.LessOrEqual(t, all[i-1].ID(), all[i].ID())
	}
}

func TestGetMissing(t *testing.T) {
	_, ok := maintenance.Get("does-not-exist-" + t.Name())
	assert.False(t, ok)
}
```

## Step 4 — Verify

```bash
go test ./internal/maintenance/...
go vet ./internal/maintenance/...
```

Both must pass with zero errors.

## Definition of Done

- `internal/maintenance/job.go` exists with `MaintenanceJob` and `StoreInjectable` interfaces
- `internal/maintenance/registry.go` exists with `Register`, `Get`, `All`, `InjectStore`
- `internal/maintenance/job_test.go` exists with ≥3 passing tests
- `go test ./internal/maintenance/...` green
- No changes to any existing handler or route

## PR Instructions

```bash
gh label create "task:ASYNC-CORE-1" --color "0075ca" --description "Bot task: maintenance interface + registry" 2>/dev/null || true
gh pr create \
  --title "feat(maintenance): add MaintenanceJob interface and registry" \
  --body "Introduces the self-registering maintenance job interface and global registry. No existing behavior changed. Part of unified maintenance system (ASYNC-CORE-1)." \
  --label "task:ASYNC-CORE-1"
```
