// file: internal/maintenance/registry.go
// version: 1.1.0
// guid: 22222222-2222-2222-2222-222222222222
// last-edited: 2026-05-04

package maintenance

import (
	"fmt"
	"sync"
)

var (
	mu       sync.RWMutex
	registry []MaintenanceJob
	byID     = map[string]MaintenanceJob{}
)

func Register(j MaintenanceJob) {
	mu.Lock()
	defer mu.Unlock()
	if _, dup := byID[j.ID()]; dup {
		panic(fmt.Sprintf("maintenance: duplicate job ID %q", j.ID()))
	}
	registry = append(registry, j)
	byID[j.ID()] = j
}

var enqueuer WriteBackEnqueuer

// InjectEnqueuer injects the write-back enqueuer into all registered jobs
// that implement EnqueuerInjectable.
func InjectEnqueuer(e WriteBackEnqueuer) {
	mu.Lock()
	defer mu.Unlock()
	enqueuer = e
	for _, j := range registry {
		if ei, ok := j.(EnqueuerInjectable); ok {
			ei.InjectEnqueuer(e)
		}
	}
}

func Get(id string) (MaintenanceJob, error) {
	mu.RLock()
	defer mu.RUnlock()
	j, ok := byID[id]
	if !ok {
		return nil, fmt.Errorf("maintenance: unknown job %q", id)
	}
	return j, nil
}

func All() []MaintenanceJob {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]MaintenanceJob, len(registry))
	copy(out, registry)
	return out
}
