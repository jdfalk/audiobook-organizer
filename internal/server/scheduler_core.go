// file: internal/server/scheduler_core.go
// version: 1.0.0
// guid: abbadd7b-b5ab-44ea-8f01-b519e3c1c947
// last-edited: 2026-05-02

package server

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
)

// TaskDefinition defines a registered task in the unified task system.
type TaskDefinition struct {
	Name        string // unique key: "library_scan", "itunes_sync", etc.
	Description string // human-readable
	Category    string // "maintenance", "library", "sync"
	// TriggerFn creates and enqueues an operation, returning it.
	TriggerFn func(source string) (*database.Operation, error)
	// Config accessors (read from AppConfig at runtime)
	IsEnabled              func() bool
	GetInterval            func() time.Duration // 0 = manual only
	RunOnStart             func() bool
	RunInMaintenanceWindow func() bool // whether this task runs during the maintenance window
}

// TaskInfo is the API-facing view of a registered task.
type TaskInfo struct {
	Name                   string  `json:"name"`
	Description            string  `json:"description"`
	Category               string  `json:"category"`
	Enabled                bool    `json:"enabled"`
	IntervalMinutes        int     `json:"interval_minutes"`
	RunOnStartup           bool    `json:"run_on_startup"`
	RunInMaintenanceWindow bool    `json:"run_in_maintenance_window"`
	LastRun                *string `json:"last_run,omitempty"`
	IsRunning              bool    `json:"is_running"`
}

// TaskScheduler manages all registered tasks, their schedules, and manual triggers.
type TaskScheduler struct {
	server             *Server
	tasks              map[string]*TaskDefinition
	order              []string // insertion order for listing
	lastRun            map[string]time.Time
	mu                 sync.RWMutex
	shutdown           chan struct{}
	maintenanceOrder   []string
	lastMaintenanceRun time.Time
}

// NewTaskScheduler creates a scheduler and registers all known tasks.
func NewTaskScheduler(s *Server) *TaskScheduler {
	ts := &TaskScheduler{
		server:  s,
		tasks:   make(map[string]*TaskDefinition),
		lastRun: make(map[string]time.Time),
	}
	ts.registerAllTasks()
	ts.maintenanceOrder = []string{
		"reconcile_scan",
		"dedup_refresh",
		"dedup_llm_review",
		"author_split_scan",
		"series_prune",
		"isbn_enrichment",
		"metadata_upgrade",
		"tombstone_cleanup",
		"purge_deleted",
		"purge_old_logs",
		"cleanup_activity_log",
		"cleanup_old_backups",
		"db_optimize",
	}
	return ts
}

func (ts *TaskScheduler) registerTask(def TaskDefinition) {
	ts.tasks[def.Name] = &def
	ts.order = append(ts.order, def.Name)
}

// Start launches background goroutines for all scheduled and startup tasks.
func (ts *TaskScheduler) Start(shutdown chan struct{}, wg *sync.WaitGroup) {
	ts.shutdown = shutdown
	ts.loadLastMaintenanceRun()

	for _, name := range ts.order {
		task := ts.tasks[name]

		// Run on startup if configured
		if task.RunOnStart != nil && task.RunOnStart() && task.IsEnabled() {
			taskName := name
			go func() {
				log.Printf("[INFO] Running startup task: %s", taskName)
				if op, err := ts.RunTask(taskName); err != nil {
					log.Printf("[WARN] Startup task %s failed: %v", taskName, err)
				} else if op != nil {
					log.Printf("[INFO] Startup task %s started: operation %s", taskName, op.ID)
				}
			}()
		}

		// Start scheduled ticker if interval > 0 and enabled
		if task.IsEnabled() && task.GetInterval() > 0 {
			interval := task.GetInterval()
			taskName := name
			wg.Add(1)
			go func() {
				defer wg.Done()
				ticker := time.NewTicker(interval)
				defer ticker.Stop()
				for {
					select {
					case <-ticker.C:
						if op, err := ts.RunTask(taskName); err != nil {
							log.Printf("[WARN] Scheduled task %s failed: %v", taskName, err)
						} else if op != nil {
							log.Printf("[INFO] Scheduled task %s started: operation %s", taskName, op.ID)
						}
					case <-shutdown:
						return
					}
				}
			}()
			log.Printf("[INFO] Scheduled task %s: interval=%v", taskName, interval)
		}
	}

	// Maintenance window checker — runs every 60 seconds
	if config.AppConfig.MaintenanceWindowEnabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(60 * time.Second)
			defer ticker.Stop()
			log.Printf("[INFO] Maintenance window enabled: %d:00 - %d:00",
				config.AppConfig.MaintenanceWindowStart, config.AppConfig.MaintenanceWindowEnd)
			for {
				select {
				case <-ticker.C:
					if isInMaintenanceWindow() && !ts.hasRunToday() {
						log.Printf("[INFO] Maintenance window open — starting maintenance run")
						if err := ts.RunMaintenanceWindow(context.Background()); err != nil {
							log.Printf("[WARN] Maintenance window failed: %v", err)
						}
					}
				case <-shutdown:
					return
				}
			}
		}()
	}
}

// RunTask triggers a scheduled task by name (source = TriggerScheduled).
func (ts *TaskScheduler) RunTask(name string) (*database.Operation, error) {
	return ts.runTask(name, operations.TriggerScheduled)
}

// RunTaskManual triggers a task as a user-initiated action (source = TriggerManual).
// Task functions can gate AlwaysShow activity-feed entries on operations.IsManual(ctx).
func (ts *TaskScheduler) RunTaskManual(name string) (*database.Operation, error) {
	return ts.runTask(name, operations.TriggerManual)
}

func (ts *TaskScheduler) runTask(name, source string) (*database.Operation, error) {
	ts.mu.RLock()
	task, ok := ts.tasks[name]
	ts.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown task: %s", name)
	}

	op, err := task.TriggerFn(source)
	if err != nil {
		return nil, err
	}

	ts.mu.Lock()
	ts.lastRun[name] = time.Now()
	ts.mu.Unlock()

	return op, nil
}

// ListTasks returns info about all registered tasks.
func (ts *TaskScheduler) ListTasks() []TaskInfo {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	result := make([]TaskInfo, 0, len(ts.order))
	for _, name := range ts.order {
		task := ts.tasks[name]
		info := TaskInfo{
			Name:            task.Name,
			Description:     task.Description,
			Category:        task.Category,
			Enabled:         task.IsEnabled(),
			IntervalMinutes: int(task.GetInterval() / time.Minute),
			RunOnStartup:    task.RunOnStart(),
		}
		if task.RunInMaintenanceWindow != nil {
			info.RunInMaintenanceWindow = task.RunInMaintenanceWindow()
		}
		if t, ok := ts.lastRun[name]; ok {
			s := t.Format(time.RFC3339)
			info.LastRun = &s
		}
		info.IsRunning = ts.isTaskRunning(info.Name)
		result = append(result, info)
	}
	return result
}

// GetTask returns the definition for a named task.
func (ts *TaskScheduler) GetTask(name string) (*TaskDefinition, bool) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	task, ok := ts.tasks[name]
	return task, ok
}

// waitForOperation polls until an operation completes or the context is canceled.
func (ts *TaskScheduler) waitForOperation(ctx context.Context, opID string) {
	store := ts.server.Store()
	if store == nil {
		return
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			op, err := store.GetOperationByID(opID)
			if err != nil {
				return
			}
			if op.Status == "completed" || op.Status == "failed" || op.Status == "canceled" {
				return
			}
		}
	}
}

