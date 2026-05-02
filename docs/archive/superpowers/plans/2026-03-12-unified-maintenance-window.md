# Unified Maintenance Window Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace separate auto-update window + per-task individual schedules with a single unified maintenance window that runs tasks sequentially in a smart order.

**Architecture:** New config fields for window hours + per-task maintenance-window toggle. A new `RunMaintenanceWindow` method on `TaskScheduler` runs tasks in fixed order during the window. `lastMaintenanceRun` persisted to DB. Frontend gets per-task toggle and settings section.

**Tech Stack:** Go (Gin, PebbleDB), React/TypeScript (MUI), existing scheduler/config/operations infrastructure.

**Spec:** `docs/superpowers/specs/2026-03-12-unified-maintenance-window-design.md`

---

## Chunk 1: Backend Config + Store Optimize Methods

### Task 1: Add Optimize() to AIScanStore and OLStore

**Files:**
- Modify: `internal/database/ai_scan_store.go`
- Modify: `internal/openlibrary/store.go`
- Test: `internal/database/ai_scan_store_test.go`
- Test: `internal/openlibrary/store_test.go`

- [ ] **Step 1: Write failing test for AIScanStore.Optimize()**

In `internal/database/ai_scan_store_test.go`, add:

```go
func TestAIScanStore_Optimize(t *testing.T) {
	tmpdir := t.TempDir()
	store, err := NewAIScanStore(tmpdir + "/ai_scans.db")
	require.NoError(t, err)
	defer store.Close()

	// Optimize should succeed on an empty store
	err = store.Optimize()
	assert.NoError(t, err)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `make test 2>&1 | grep -A2 TestAIScanStore_Optimize`
Expected: FAIL — `Optimize` method does not exist

- [ ] **Step 3: Implement AIScanStore.Optimize()**

In `internal/database/ai_scan_store.go`, add after the `Close()` method:

```go
// Optimize compacts the AI scan PebbleDB store.
func (s *AIScanStore) Optimize() error {
	return s.db.Compact(context.Background(), nil, []byte{0xff}, false)
}
```

Add `"context"` to imports if not present.

- [ ] **Step 4: Run test to verify it passes**

Run: `make test 2>&1 | grep -A2 TestAIScanStore_Optimize`
Expected: PASS

- [ ] **Step 5: Write failing test for OLStore.Optimize()**

In `internal/openlibrary/store_test.go`, add:

```go
func TestOLStore_Optimize(t *testing.T) {
	tmpdir := t.TempDir()
	store, err := NewOLStore(tmpdir + "/ol.db")
	require.NoError(t, err)
	defer store.Close()

	err = store.Optimize()
	assert.NoError(t, err)
}
```

Ensure `"testing"`, `"github.com/stretchr/testify/assert"`, `"github.com/stretchr/testify/require"` are imported.

- [ ] **Step 6: Implement OLStore.Optimize()**

In `internal/openlibrary/store.go`, add after the `Close()` method:

```go
// Optimize compacts the Open Library PebbleDB cache.
func (s *OLStore) Optimize() error {
	return s.db.Compact(context.Background(), nil, []byte{0xff}, false)
}
```

Add `"context"` to imports.

- [ ] **Step 7: Run tests**

Run: `make test 2>&1 | grep -E "TestAIScanStore_Optimize|TestOLStore_Optimize"`
Expected: Both PASS

- [ ] **Step 8: Commit**

```bash
git add internal/database/ai_scan_store.go internal/database/ai_scan_store_test.go internal/openlibrary/store.go internal/openlibrary/store_test.go
git commit -m "feat(db): add Optimize() to AIScanStore and OLStore for maintenance window"
```

### Task 2: Add maintenance window config fields

**Files:**
- Modify: `internal/config/config.go` (lines 170-226 area)

- [ ] **Step 1: Add maintenance window fields to Config struct**

In `internal/config/config.go`, after the `AutoUpdateWindowEnd` field (line 175), add:

```go
	// Maintenance window (unified — replaces separate auto-update window)
	MaintenanceWindowEnabled bool `json:"maintenance_window_enabled"`
	MaintenanceWindowStart   int  `json:"maintenance_window_start"` // hour 0-23, default 1
	MaintenanceWindowEnd     int  `json:"maintenance_window_end"`   // hour 0-23, default 4
```

After the `ScheduledReconcileOnStartup` field (line 222), add per-task maintenance window fields:

```go
	// Per-task maintenance window toggles
	MaintenanceWindowDedupRefresh    bool `json:"maintenance_window_dedup_refresh"`
	MaintenanceWindowSeriesPrune     bool `json:"maintenance_window_series_prune"`
	MaintenanceWindowAuthorSplit     bool `json:"maintenance_window_author_split"`
	MaintenanceWindowTombstoneCleanup bool `json:"maintenance_window_tombstone_cleanup"`
	MaintenanceWindowReconcile       bool `json:"maintenance_window_reconcile"`
	MaintenanceWindowPurgeDeleted    bool `json:"maintenance_window_purge_deleted"`
	MaintenanceWindowPurgeOldLogs    bool `json:"maintenance_window_purge_old_logs"`
	MaintenanceWindowDbOptimize      bool `json:"maintenance_window_db_optimize"`
	MaintenanceWindowLibraryScan     bool `json:"maintenance_window_library_scan"`
	MaintenanceWindowLibraryOrganize bool `json:"maintenance_window_library_organize"`
	MaintenanceWindowMetadataRefresh bool `json:"maintenance_window_metadata_refresh"`
```

- [ ] **Step 2: Add viper defaults in InitConfig()**

In `InitConfig()`, add defaults:

```go
	// Maintenance window defaults
	viper.SetDefault("maintenance_window_enabled", true)
	viper.SetDefault("maintenance_window_start", 1)
	viper.SetDefault("maintenance_window_end", 4)
	// Per-task defaults — maintenance tasks default true
	viper.SetDefault("maintenance_window_dedup_refresh", true)
	viper.SetDefault("maintenance_window_series_prune", true)
	viper.SetDefault("maintenance_window_author_split", true)
	viper.SetDefault("maintenance_window_tombstone_cleanup", true)
	viper.SetDefault("maintenance_window_reconcile", true)
	viper.SetDefault("maintenance_window_purge_deleted", true)
	viper.SetDefault("maintenance_window_purge_old_logs", true)
	viper.SetDefault("maintenance_window_db_optimize", true)
	// Non-maintenance tasks default false
	viper.SetDefault("maintenance_window_library_scan", false)
	viper.SetDefault("maintenance_window_library_organize", false)
	viper.SetDefault("maintenance_window_metadata_refresh", false)
```

- [ ] **Step 3: Build to verify**

Run: `make build-api 2>&1 | tail -3`
Expected: `✅ Built ./audiobook-organizer`

- [ ] **Step 4: Commit**

```bash
git add internal/config/config.go
git commit -m "feat(config): add unified maintenance window config fields"
```

### Task 3: Add config migration from auto-update window fields

**Files:**
- Modify: `internal/config/persistence.go`

- [ ] **Step 1: Find where config is loaded from DB**

Read `internal/config/persistence.go` and find the `LoadConfigFromDatabase` function. Add migration logic that runs after loading:

```go
// MigrateMaintenanceWindow migrates auto-update window fields to maintenance window.
// Idempotent — safe to call multiple times.
func MigrateMaintenanceWindow(store interface{ GetSetting(string) (*database.Setting, error); SetSetting(string, string, string, bool) error }) {
	migrated, _ := store.GetSetting("maintenance_window_migrated")
	if migrated != nil && migrated.Value == "true" {
		return
	}

	// Migrate auto-update window start/end if maintenance window not yet configured
	if AppConfig.MaintenanceWindowStart == 0 && AppConfig.AutoUpdateWindowStart > 0 {
		AppConfig.MaintenanceWindowStart = AppConfig.AutoUpdateWindowStart
	}
	if AppConfig.MaintenanceWindowEnd == 0 && AppConfig.AutoUpdateWindowEnd > 0 {
		AppConfig.MaintenanceWindowEnd = AppConfig.AutoUpdateWindowEnd
	}
	// Ensure sensible defaults
	if AppConfig.MaintenanceWindowStart == 0 && AppConfig.MaintenanceWindowEnd == 0 {
		AppConfig.MaintenanceWindowStart = 1
		AppConfig.MaintenanceWindowEnd = 4
	}

	_ = store.SetSetting("maintenance_window_migrated", "true", "bool", false)
}
```

Import `database` package if needed. Call `MigrateMaintenanceWindow(store)` at the end of `LoadConfigFromDatabase`.

- [ ] **Step 2: Build to verify**

Run: `make build-api 2>&1 | tail -3`
Expected: Compiles

- [ ] **Step 3: Commit**

```bash
git add internal/config/persistence.go
git commit -m "feat(config): add maintenance window migration from auto-update window"
```

---

## Chunk 2: Scheduler Maintenance Window Runner

### Task 4: Add RunInMaintenanceWindow to TaskDefinition and TaskInfo

**Files:**
- Modify: `internal/server/scheduler.go`

- [ ] **Step 1: Add field to TaskDefinition**

In `TaskDefinition` struct (line 25-35), add:

```go
	RunInMaintenanceWindow func() bool // whether this task runs during the maintenance window
```

- [ ] **Step 2: Add field to TaskInfo**

In `TaskInfo` struct (line 38-46), add:

```go
	RunInMaintenanceWindow bool `json:"run_in_maintenance_window"`
```

- [ ] **Step 3: Update ListTasks to include the new field**

In `ListTasks()` (line 932), add after `RunOnStartup`:

```go
		if task.RunInMaintenanceWindow != nil {
			info.RunInMaintenanceWindow = task.RunInMaintenanceWindow()
		}
```

- [ ] **Step 4: Add RunInMaintenanceWindow closures to all task registrations**

For each `registerTask` call in `registerAllTasks()`, add the `RunInMaintenanceWindow` field. Reference the config field:

```go
// For library_scan:
RunInMaintenanceWindow: func() bool { return config.AppConfig.MaintenanceWindowLibraryScan },

// For library_organize:
RunInMaintenanceWindow: func() bool { return config.AppConfig.MaintenanceWindowLibraryOrganize },

// For transcode:
RunInMaintenanceWindow: func() bool { return false }, // always manual

// For itunes_sync:
RunInMaintenanceWindow: func() bool { return false }, // interval only

// For itunes_import:
RunInMaintenanceWindow: func() bool { return false }, // manual only

// For dedup_refresh:
RunInMaintenanceWindow: func() bool { return config.AppConfig.MaintenanceWindowDedupRefresh },

// For series_prune:
RunInMaintenanceWindow: func() bool { return config.AppConfig.MaintenanceWindowSeriesPrune },

// For author_split_scan:
RunInMaintenanceWindow: func() bool { return config.AppConfig.MaintenanceWindowAuthorSplit },

// For db_optimize:
RunInMaintenanceWindow: func() bool { return config.AppConfig.MaintenanceWindowDbOptimize },

// For purge_deleted:
RunInMaintenanceWindow: func() bool { return config.AppConfig.MaintenanceWindowPurgeDeleted },

// For tombstone_cleanup:
RunInMaintenanceWindow: func() bool { return config.AppConfig.MaintenanceWindowTombstoneCleanup },

// For resolve_production_authors:
RunInMaintenanceWindow: func() bool { return false }, // manual

// For metadata_refresh:
RunInMaintenanceWindow: func() bool { return config.AppConfig.MaintenanceWindowMetadataRefresh },

// For reconcile_scan:
RunInMaintenanceWindow: func() bool { return config.AppConfig.MaintenanceWindowReconcile },

// For ai_dedup_batch:
RunInMaintenanceWindow: func() bool { return false }, // long-running, not suitable

// For ai_pipeline_batch_poll:
RunInMaintenanceWindow: func() bool { return false }, // polling, not window

// For purge_old_logs:
RunInMaintenanceWindow: func() bool { return config.AppConfig.MaintenanceWindowPurgeOldLogs },
```

- [ ] **Step 5: Build to verify**

Run: `make build-api 2>&1 | tail -3`
Expected: Compiles

- [ ] **Step 6: Commit**

```bash
git add internal/server/scheduler.go
git commit -m "feat(scheduler): add RunInMaintenanceWindow to task definitions"
```

### Task 5: Implement RunMaintenanceWindow method

**Files:**
- Modify: `internal/server/scheduler.go`

- [ ] **Step 1: Add maintenance window state to TaskScheduler**

In the `TaskScheduler` struct (line 49-56), add:

```go
	maintenanceOrder   []string
	lastMaintenanceRun time.Time
```

- [ ] **Step 2: Define maintenance order in NewTaskScheduler**

After `ts.registerAllTasks()` in `NewTaskScheduler` (line 65), add:

```go
	ts.maintenanceOrder = []string{
		"reconcile_scan",
		"dedup_refresh",
		"author_split_scan",
		"series_prune",
		"tombstone_cleanup",
		"purge_deleted",
		"purge_old_logs",
		"db_optimize",
	}
```

- [ ] **Step 3: Add isInMaintenanceWindow helper (testable version)**

```go
// isInMaintenanceWindowAt checks if a given hour falls within the configured window.
// Supports midnight-spanning windows (e.g., start=23, end=2).
func isInMaintenanceWindowAt(hour int) bool {
	if !config.AppConfig.MaintenanceWindowEnabled {
		return false
	}
	start := config.AppConfig.MaintenanceWindowStart
	end := config.AppConfig.MaintenanceWindowEnd

	if start < end {
		return hour >= start && hour < end
	}
	// Midnight spanning: e.g., start=23, end=2 → 23,0,1 are in window
	return hour >= start || hour < end
}

// isInMaintenanceWindow checks if the current time falls within the configured window.
func isInMaintenanceWindow() bool {
	return isInMaintenanceWindowAt(time.Now().Hour())
}
```

- [ ] **Step 4: Add loadLastMaintenanceRun helper**

```go
// loadLastMaintenanceRun reads the persisted last-run date from the database.
func (ts *TaskScheduler) loadLastMaintenanceRun() {
	store := database.GlobalStore
	if store == nil {
		return
	}
	setting, err := store.GetSetting("maintenance_window_last_run")
	if err != nil || setting == nil {
		return
	}
	t, err := time.Parse("2006-01-02", setting.Value)
	if err != nil {
		return
	}
	ts.lastMaintenanceRun = t
}

// saveLastMaintenanceRun persists today's date as the last-run date.
func (ts *TaskScheduler) saveLastMaintenanceRun() {
	store := database.GlobalStore
	if store == nil {
		return
	}
	today := time.Now().Format("2006-01-02")
	_ = store.SetSetting("maintenance_window_last_run", today, "string", false)
	ts.lastMaintenanceRun = time.Now()
}

// hasRunToday checks if the maintenance window has already run today.
func (ts *TaskScheduler) hasRunToday() bool {
	today := time.Now().Format("2006-01-02")
	return ts.lastMaintenanceRun.Format("2006-01-02") == today
}
```

- [ ] **Step 5: Add context key type for ignore_window**

```go
// maintenanceCtxKey is a typed context key to avoid string-key collisions.
type maintenanceCtxKey string

const ignoreWindowKey maintenanceCtxKey = "ignore_window"
```

- [ ] **Step 6: Add isTaskRunning helper for duplicate prevention**

```go
// isTaskRunning checks if a task's operation is currently in progress.
func (ts *TaskScheduler) isTaskRunning(name string) bool {
	store := database.GlobalStore
	if store == nil {
		return false
	}
	// Check recent operations of this task's type for "running" or "pending" status
	ops, err := store.GetActiveOperations()
	if err != nil {
		return false
	}
	// Map task name to operation type
	opTypeMap := map[string]string{
		"library_scan": "scan", "library_organize": "organize",
		"dedup_refresh": "dedup-refresh", "series_prune": "series-prune",
		"author_split_scan": "author-split-scan", "db_optimize": "db-optimize",
		"purge_deleted": "purge-deleted", "tombstone_cleanup": "tombstone-cleanup",
		"reconcile_scan": "reconcile_scan", "purge_old_logs": "purge_old_logs",
		"metadata_refresh": "metadata-refresh",
	}
	opType, ok := opTypeMap[name]
	if !ok {
		return false
	}
	for _, op := range ops {
		if op.Type == opType && (op.Status == "running" || op.Status == "pending") {
			return true
		}
	}
	return false
}
```

- [ ] **Step 7: Implement RunMaintenanceWindow**

```go
// RunMaintenanceWindow runs all maintenance-window-eligible tasks in order.
// Step 1: auto-update (if enabled). Step 2+: maintenance tasks in fixed order.
// Creates a parent operation for tracking. Continues on error.
func (ts *TaskScheduler) RunMaintenanceWindow(ctx context.Context) error {
	store := database.GlobalStore
	if store == nil {
		return fmt.Errorf("database not initialized")
	}

	// Create parent operation
	opID := ulid.Make().String()
	op, err := store.CreateOperation(opID, "maintenance-window", nil)
	if err != nil {
		return fmt.Errorf("failed to create maintenance-window operation: %w", err)
	}

	if operations.GlobalQueue == nil {
		return fmt.Errorf("operation queue not initialized")
	}

	_ = operations.GlobalQueue.Enqueue(op.ID, "maintenance-window", operations.PriorityNormal,
		func(ctx context.Context, progress operations.ProgressReporter) error {
			ignoreWindow := ctx.Value(ignoreWindowKey) != nil

			// Step 1: Auto-update (if enabled and not already completed post-restart)
			if config.AppConfig.AutoUpdateEnabled {
				updateDone, _ := store.GetSetting("maintenance_window_update_completed")
				today := time.Now().Format("2006-01-02")
				if updateDone == nil || updateDone.Value != today {
					_ = progress.Log("info", "Running auto-update (step 1)", nil)
					_ = progress.UpdateProgress(0, 100, "Running auto-update...")
					// Mark update as completed for today BEFORE triggering
					// (if update restarts server, post-restart window skips this step)
					_ = store.SetSetting("maintenance_window_update_completed", today, "string", false)
					if ts.server.runAutoUpdate != nil {
						ts.server.runAutoUpdate()
					}
					_ = progress.Log("info", "Auto-update step complete", nil)
				} else {
					_ = progress.Log("info", "Auto-update already completed today, skipping", nil)
				}
			}

			// Step 2+: Maintenance tasks in order
			var eligible []string
			for _, name := range ts.maintenanceOrder {
				task, ok := ts.tasks[name]
				if !ok {
					continue
				}
				if task.IsEnabled() && task.RunInMaintenanceWindow != nil && task.RunInMaintenanceWindow() {
					eligible = append(eligible, name)
				}
			}

			_ = progress.Log("info", fmt.Sprintf("Maintenance window starting: %d tasks eligible", len(eligible)), nil)

			hadErrors := false
			for i, name := range eligible {
				// Check if window is still open (skip for manual "Run Now" triggers)
				if !ignoreWindow && !isInMaintenanceWindow() {
					_ = progress.Log("warning", fmt.Sprintf("Maintenance window closed after task %d/%d, skipping remaining", i, len(eligible)), nil)
					break
				}

				// Duplicate prevention: skip if task is already running (from interval ticker)
				if ts.isTaskRunning(name) {
					_ = progress.Log("info", fmt.Sprintf("Task %s already running (interval), skipping", name), nil)
					continue
				}

				_ = progress.UpdateProgress(i, len(eligible), fmt.Sprintf("Running task %d/%d: %s", i+1, len(eligible), name))
				_ = progress.Log("info", fmt.Sprintf("Starting maintenance task: %s", name), nil)

				taskOp, taskErr := ts.RunTask(name)
				if taskErr != nil {
					hadErrors = true
					_ = progress.Log("error", fmt.Sprintf("Task %s failed: %v", name, taskErr), nil)
				} else if taskOp != nil {
					// Wait for the task operation to complete before starting next
					ts.waitForOperation(ctx, taskOp.ID, progress)
					// Check if the waited task failed
					completedOp, _ := store.GetOperation(taskOp.ID)
					if completedOp != nil && completedOp.Status == "failed" {
						hadErrors = true
						_ = progress.Log("warning", fmt.Sprintf("Task %s operation failed", name), nil)
					} else {
						_ = progress.Log("info", fmt.Sprintf("Task %s completed (op: %s)", name, taskOp.ID), nil)
					}
				} else {
					_ = progress.Log("info", fmt.Sprintf("Task %s triggered (no operation)", name), nil)
				}
			}

			ts.saveLastMaintenanceRun()

			if hadErrors {
				_ = progress.UpdateProgress(len(eligible), len(eligible), "Maintenance window completed with errors")
				return fmt.Errorf("maintenance window completed with errors")
			}
			_ = progress.UpdateProgress(len(eligible), len(eligible), "Maintenance window completed successfully")
			return nil
		},
	)
	return nil
}

// waitForOperation polls until an operation completes or the context is canceled.
func (ts *TaskScheduler) waitForOperation(ctx context.Context, opID string, progress operations.ProgressReporter) {
	store := database.GlobalStore
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
			op, err := store.GetOperation(opID)
			if err != nil {
				return
			}
			if op.Status == "completed" || op.Status == "failed" || op.Status == "canceled" {
				return
			}
		}
	}
}
```

- [ ] **Step 6: Build to verify**

Run: `make build-api 2>&1 | tail -3`
Expected: Compiles

- [ ] **Step 7: Commit**

```bash
git add internal/server/scheduler.go
git commit -m "feat(scheduler): implement RunMaintenanceWindow with sequential task execution"
```

### Task 6: Add maintenance window goroutine to Start()

**Files:**
- Modify: `internal/server/scheduler.go`

- [ ] **Step 1: Load persisted last-run on startup**

In `Start()` method (line 864), add at the beginning:

```go
	ts.loadLastMaintenanceRun()
```

- [ ] **Step 2: Add maintenance window goroutine**

After the task ticker loop in `Start()` (before the closing `}`), add:

```go
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
```

- [ ] **Step 3: Build to verify**

Run: `make build-api 2>&1 | tail -3`
Expected: Compiles

- [ ] **Step 4: Commit**

```bash
git add internal/server/scheduler.go
git commit -m "feat(scheduler): add maintenance window checker goroutine"
```

### Task 7: Expand db_optimize to compact all PebbleDB stores

**Files:**
- Modify: `internal/server/scheduler.go` (db_optimize task handler)

- [ ] **Step 1: Update db_optimize TriggerFn**

Find the `db_optimize` task registration (line ~389). Replace the TriggerFn body:

```go
		TriggerFn: func() (*database.Operation, error) {
			return ts.triggerOperation("db-optimize", func(ctx context.Context, progress operations.ProgressReporter) error {
				store := database.GlobalStore
				if store == nil {
					return fmt.Errorf("database not initialized")
				}

				storesOptimized := 0
				storesTotal := 3

				// 1. Main store
				_ = progress.Log("info", "Optimizing main database", nil)
				_ = progress.UpdateProgress(0, storesTotal, "Optimizing main database...")
				if err := store.Optimize(); err != nil {
					_ = progress.Log("error", fmt.Sprintf("Main DB optimization failed: %v", err), nil)
				} else {
					storesOptimized++
					_ = progress.Log("info", "Main database optimized", nil)
				}

				// 2. AI scan store
				_ = progress.UpdateProgress(1, storesTotal, "Optimizing AI scan database...")
				if s.aiScanStore != nil {
					if err := s.aiScanStore.Optimize(); err != nil {
						_ = progress.Log("error", fmt.Sprintf("AI scan DB optimization failed: %v", err), nil)
					} else {
						storesOptimized++
						_ = progress.Log("info", "AI scan database optimized", nil)
					}
				} else {
					_ = progress.Log("info", "AI scan store not initialized, skipping", nil)
				}

				// 3. OpenLibrary store (accessed via olService)
				_ = progress.UpdateProgress(2, storesTotal, "Optimizing OpenLibrary cache...")
				if s.olService != nil && s.olService.Store() != nil {
					if err := s.olService.Store().Optimize(); err != nil {
						_ = progress.Log("error", fmt.Sprintf("OL cache optimization failed: %v", err), nil)
					} else {
						storesOptimized++
						_ = progress.Log("info", "OpenLibrary cache optimized", nil)
					}
				} else {
					_ = progress.Log("info", "OpenLibrary store not initialized, skipping", nil)
				}

				_ = progress.UpdateProgress(storesTotal, storesTotal, fmt.Sprintf("Database optimization complete: %d/%d stores", storesOptimized, storesTotal))
				return nil
			})
		},
```

Note: `s.aiScanStore` exists on the Server struct (line 565). The OL store is accessed via `s.olService.Store()` (line 632). The `OLStore.Optimize()` method was added in Task 1.

- [ ] **Step 2: Build to verify**

Run: `make build-api 2>&1 | tail -3`
Expected: Compiles (may need to add `olStore` field to Server)

- [ ] **Step 3: Commit**

```bash
git add internal/server/scheduler.go internal/server/server.go
git commit -m "feat(scheduler): expand db_optimize to compact all PebbleDB stores"
```

---

## Chunk 3: API Handler Updates

### Task 8: Update updateTaskConfig to handle maintenance window toggle

**Files:**
- Modify: `internal/server/server.go` (lines 7968-8057)

- [ ] **Step 1: Add RunInMaintenanceWindow to request struct**

In `updateTaskConfig` (line 7971), add to the req struct:

```go
	var req struct {
		Enabled                *bool `json:"enabled"`
		IntervalMinutes        *int  `json:"interval_minutes"`
		RunOnStartup           *bool `json:"run_on_startup"`
		RunInMaintenanceWindow *bool `json:"run_in_maintenance_window"`
	}
```

- [ ] **Step 2: Add maintenance window config mapping to each case**

For each case in the switch statement, add the maintenance window field update. Example for `dedup_refresh`:

```go
	case "dedup_refresh":
		// ... existing enabled/interval/startup handling ...
		if req.RunInMaintenanceWindow != nil {
			config.AppConfig.MaintenanceWindowDedupRefresh = *req.RunInMaintenanceWindow
		}
```

Add similar for all tasks:
- `series_prune` → `MaintenanceWindowSeriesPrune`
- `author_split_scan` → `MaintenanceWindowAuthorSplit`
- `db_optimize` → `MaintenanceWindowDbOptimize`
- `purge_deleted` → `MaintenanceWindowPurgeDeleted`
- `tombstone_cleanup` → `MaintenanceWindowTombstoneCleanup`
- `reconcile_scan` → `MaintenanceWindowReconcile`
- `purge_old_logs` → `MaintenanceWindowPurgeOldLogs`
- `library_scan` → `MaintenanceWindowLibraryScan`
- `library_organize` → `MaintenanceWindowLibraryOrganize`
- `metadata_refresh` → `MaintenanceWindowMetadataRefresh`

- [ ] **Step 3: Build to verify**

Run: `make build-api 2>&1 | tail -3`

- [ ] **Step 4: Commit**

```bash
git add internal/server/server.go
git commit -m "feat(api): add run_in_maintenance_window to task config updates"
```

### Task 9: Add "Run Maintenance Now" endpoint

**Files:**
- Modify: `internal/server/server.go`

- [ ] **Step 1: Add route**

Find the task routes (around line 1290-1293). Add:

```go
	tasks.POST("/maintenance-window/run", s.runMaintenanceWindow)
```

- [ ] **Step 2: Add handler**

```go
// runMaintenanceWindow triggers the full maintenance window sequence immediately.
func (s *Server) runMaintenanceWindow(c *gin.Context) {
	if s.scheduler == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "scheduler not initialized"})
		return
	}
	ctx := context.WithValue(c.Request.Context(), ignoreWindowKey, true)
	if err := s.scheduler.RunMaintenanceWindow(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"message": "maintenance window triggered"})
}
```

- [ ] **Step 3: Build to verify**

Run: `make build-api 2>&1 | tail -3`

- [ ] **Step 4: Commit**

```bash
git add internal/server/server.go
git commit -m "feat(api): add POST /tasks/maintenance-window/run endpoint"
```

---

## Chunk 4: Frontend Changes

### Task 10: Update API types and add maintenance window API calls

**Files:**
- Modify: `web/src/services/api.ts`

- [ ] **Step 1: Update TaskInfo type**

Find the `TaskInfo` interface in api.ts. Add:

```typescript
  run_in_maintenance_window: boolean;
```

- [ ] **Step 2: Add runMaintenanceWindow function**

```typescript
export async function runMaintenanceWindow(): Promise<void> {
  await fetchApi('/api/v1/tasks/maintenance-window/run', { method: 'POST' });
}
```

- [ ] **Step 3: Commit**

```bash
git add web/src/services/api.ts
git commit -m "feat(frontend): add maintenance window types and API call"
```

### Task 11: Update Maintenance page with maintenance window toggle

**Files:**
- Modify: `web/src/pages/Maintenance.tsx`

- [ ] **Step 1: Add "Run Maintenance Now" button at top of page**

Above the category sections, add a button:

```tsx
<Box sx={{ mb: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
  <Typography variant="h5">Scheduled Tasks</Typography>
  <Button
    variant="contained"
    onClick={async () => {
      try {
        await api.runMaintenanceWindow();
        setSuccessMsg('Maintenance window triggered');
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to start maintenance window');
      }
    }}
  >
    Run Maintenance Now
  </Button>
</Box>
```

- [ ] **Step 2: Add "Maint. Window" toggle to each task card**

In the task card rendering, after the "On Start" toggle, add:

```tsx
<FormControlLabel
  control={
    <Switch
      size="small"
      checked={task.run_in_maintenance_window}
      onChange={async (e) => {
        try {
          await api.updateTaskConfig(task.name, {
            run_in_maintenance_window: e.target.checked,
          });
          fetchTasks();
        } catch (err) {
          setError(err instanceof Error ? err.message : 'Failed to update');
        }
      }}
    />
  }
  label="Maint. Window"
/>
```

- [ ] **Step 3: Build frontend**

Run: `cd web && npm run build 2>&1 | tail -5`
Expected: Build succeeds

- [ ] **Step 4: Commit**

```bash
git add web/src/pages/Maintenance.tsx
git commit -m "feat(frontend): add maintenance window toggle and Run Maintenance Now button"
```

### Task 12: Add Maintenance Window section to Settings page

**Files:**
- Modify: `web/src/pages/Settings.tsx`

- [ ] **Step 1: Add Maintenance Window section to System tab**

Find the system tab content. Add a new section (after or replacing the auto-update window fields):

```tsx
<Typography variant="h6" gutterBottom>Maintenance Window</Typography>
<Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
  Configure when maintenance tasks run. Auto-update runs first, then tasks in order.
</Typography>
<FormControlLabel
  control={
    <Switch
      checked={localConfig.maintenance_window_enabled ?? true}
      onChange={(e) => handleConfigChange('maintenance_window_enabled', e.target.checked)}
    />
  }
  label="Enable maintenance window"
/>
<Stack direction="row" spacing={2} sx={{ mt: 1 }}>
  <TextField
    label="Start hour (0-23)"
    type="number"
    size="small"
    inputProps={{ min: 0, max: 23 }}
    value={localConfig.maintenance_window_start ?? 1}
    onChange={(e) => handleConfigChange('maintenance_window_start', parseInt(e.target.value, 10))}
  />
  <TextField
    label="End hour (0-23)"
    type="number"
    size="small"
    inputProps={{ min: 0, max: 23 }}
    value={localConfig.maintenance_window_end ?? 4}
    onChange={(e) => handleConfigChange('maintenance_window_end', parseInt(e.target.value, 10))}
  />
</Stack>
```

- [ ] **Step 2: Remove auto-update window start/end from the auto-update section**

Find the `AutoUpdateWindowStart` / `AutoUpdateWindowEnd` fields in the auto-update section and remove them (they're now in the maintenance window section).

- [ ] **Step 3: Build frontend**

Run: `cd web && npm run build 2>&1 | tail -5`

- [ ] **Step 4: Commit**

```bash
git add web/src/pages/Settings.tsx
git commit -m "feat(frontend): add maintenance window settings section"
```

---

## Chunk 5: Tests and Final Verification

### Task 13: Write scheduler maintenance window tests

**Files:**
- Create or modify: `internal/server/scheduler_test.go`

- [ ] **Step 1: Write tests for isInMaintenanceWindowAt and hasRunToday**

```go
func TestIsInMaintenanceWindowAt(t *testing.T) {
	tests := []struct {
		name    string
		start   int
		end     int
		hour    int
		enabled bool
		want    bool
	}{
		{"disabled", 1, 4, 2, false, false},
		{"in window", 1, 4, 2, true, true},
		{"at start (inclusive)", 1, 4, 1, true, true},
		{"before window", 1, 4, 0, true, false},
		{"at end (exclusive)", 1, 4, 4, true, false},
		{"midnight span in midnight", 23, 2, 0, true, true},
		{"midnight span in 1am", 23, 2, 1, true, true},
		{"midnight span at start", 23, 2, 23, true, true},
		{"midnight span out 2am", 23, 2, 2, true, false},
		{"midnight span out 3am", 23, 2, 3, true, false},
		{"midnight span out noon", 23, 2, 12, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.AppConfig.MaintenanceWindowEnabled = tt.enabled
			config.AppConfig.MaintenanceWindowStart = tt.start
			config.AppConfig.MaintenanceWindowEnd = tt.end
			got := isInMaintenanceWindowAt(tt.hour)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHasRunToday(t *testing.T) {
	ts := &TaskScheduler{lastMaintenanceRun: time.Time{}}
	assert.False(t, ts.hasRunToday(), "zero time should not count as today")

	ts.lastMaintenanceRun = time.Now()
	assert.True(t, ts.hasRunToday(), "current time should count as today")

	ts.lastMaintenanceRun = time.Now().AddDate(0, 0, -1)
	assert.False(t, ts.hasRunToday(), "yesterday should not count as today")
}
```

- [ ] **Step 2: Run tests**

Run: `make test 2>&1 | grep TestIsInMaintenanceWindow`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/server/scheduler.go internal/server/scheduler_test.go
git commit -m "test(scheduler): add isInMaintenanceWindow tests with midnight-spanning support"
```

### Task 14: Full build and test verification

- [ ] **Step 1: Run full build**

Run: `make build 2>&1 | tail -5`
Expected: `✅ Built ./audiobook-organizer`

- [ ] **Step 2: Run all tests**

Run: `make test 2>&1 | tail -10`
Expected: All pass

- [ ] **Step 3: Run frontend tests**

Run: `make test-all 2>&1 | tail -10`
Expected: All pass

- [ ] **Step 4: Final commit with version bumps**

Bump version headers in all modified files, then:

```bash
git add -A
git commit -m "feat(maintenance): unified maintenance window with sequential task execution

Replaces separate auto-update window and per-task schedules with a single
configurable maintenance window. Tasks run sequentially: auto-update first,
then reconcile → dedup → split → prune → tombstone → purge → optimize.

- Config: MaintenanceWindowEnabled/Start/End + per-task toggles
- Scheduler: RunMaintenanceWindow with continue-on-error, window time checks
- db_optimize: now compacts all PebbleDB stores (main, AI scan, OL cache)
- Frontend: Maint. Window toggle per task, Run Maintenance Now button
- Settings: Maintenance window config section replaces auto-update window times
- Migration: auto-update window fields migrate to maintenance window on first load"
```
