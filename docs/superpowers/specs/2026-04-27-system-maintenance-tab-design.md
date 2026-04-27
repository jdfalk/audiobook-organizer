<!-- file: docs/superpowers/specs/2026-04-27-system-maintenance-tab-design.md -->
<!-- version: 1.0.0 -->
<!-- guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890 -->

# System Page Maintenance Tab — Design Spec

**Date:** 2026-04-27  
**Status:** Approved  
**Author:** Claude (on behalf of jdfalk)

---

## Problem Statement

The standalone `/maintenance` page renders an empty task list because `getRegisteredTasks()`
returns the raw `{"data": [...]}` envelope from `RespondWithOK`, and `Maintenance.tsx`'s
`Array.isArray(data)` check returns false on the envelope object — falling back to `[]`.

Additionally, the maintenance window schedule (start/end hour, enabled) has no UI; it can
only be changed via database edits. Tasks have no live running-status indicator. Maintenance
and System are separate nav entries despite being related system-management concerns.

---

## Goals

1. Fix the task-list rendering bug (one-line frontend fix).
2. Move all maintenance functionality into a new **Maintenance** tab on the `/system` page.
3. Add a **Maintenance Window** configuration card at the top of the tab (enable toggle,
   start/end hour, last-run / next-run display, Run Now button).
4. Surface live `is_running` status per task (spinner while a task executes).
5. Remove the redundant `/maintenance` route and sidebar entry.

## Non-Goals

- No changes to scheduler logic or task definitions.
- No database migrations.
- No changes to other System page tabs.
- No changes to existing API response format for tasks (fix is frontend-only).

---

## Architecture

### Backend — 3 Changes

#### 1. Add `IsRunning` to `TaskInfo`

**File:** `internal/server/scheduler.go`

Add `IsRunning bool \`json:"is_running"\`` to the `TaskInfo` struct (line ~50).

In `ListTasks()` (line ~1257), after building each `info` object, call:

```go
info.IsRunning = ts.isTaskRunning(info.Name)
```

`isTaskRunning` is already implemented (line ~1355) and only touches `ts.server.Store()` —
no mutex conflict with the `RLock` held by `ListTasks`.

#### 2. Add Two Public Scheduler Helper Methods

**File:** `internal/server/scheduler.go`

```go
// GetLastMaintenanceRunDate returns the persisted last-run date ("2006-01-02") or "".
func (ts *TaskScheduler) GetLastMaintenanceRunDate() string {
    if ts.lastMaintenanceRun.IsZero() {
        return ""
    }
    return ts.lastMaintenanceRun.Format("2006-01-02")
}

// IsMaintenanceRunning returns true if the maintenance-window operation is active.
func (ts *TaskScheduler) IsMaintenanceRunning() bool {
    store := ts.server.Store()
    if store == nil {
        return false
    }
    ops, _, err := store.ListOperations(20, 0)
    if err != nil {
        return false
    }
    for _, op := range ops {
        if op.Type == "maintenance-window" && (op.Status == "running" || op.Status == "pending") {
            return true
        }
    }
    return false
}
```

#### 3. Two New Maintenance-Window Endpoints

**File:** `internal/server/operations_handlers.go` — add after `runMaintenanceWindowNow` (~line 862).

**`GET /api/v1/maintenance-window/status`**

```go
func (s *Server) getMaintenanceWindowStatus(c *gin.Context) {
    if s.scheduler == nil {
        RespondWithInternalError(c, "scheduler not initialized")
        return
    }
    cfg := config.AppConfig
    nextRun := calculateNextWindowRun(cfg.MaintenanceWindowStart)

    c.JSON(http.StatusOK, gin.H{
        "enabled":           cfg.MaintenanceWindowEnabled,
        "window_start":      cfg.MaintenanceWindowStart,
        "window_end":        cfg.MaintenanceWindowEnd,
        "last_run_date":     s.scheduler.GetLastMaintenanceRunDate(),
        "next_run_estimate": nextRun,
        "currently_running": s.scheduler.IsMaintenanceRunning(),
    })
}

// calculateNextWindowRun returns an RFC3339 timestamp for the next occurrence
// of startHour (local time). If the hour hasn't passed today, returns today;
// otherwise returns tomorrow.
func calculateNextWindowRun(startHour int) string {
    now := time.Now()
    next := time.Date(now.Year(), now.Month(), now.Day(), startHour, 0, 0, 0, now.Location())
    if !next.After(now) {
        next = next.Add(24 * time.Hour)
    }
    return next.Format(time.RFC3339)
}
```

**`PUT /api/v1/maintenance-window/config`**

```go
type maintenanceWindowConfigReq struct {
    Enabled     bool `json:"enabled"`
    WindowStart int  `json:"window_start"`
    WindowEnd   int  `json:"window_end"`
}

func (s *Server) updateMaintenanceWindowConfig(c *gin.Context) {
    var req maintenanceWindowConfigReq
    if err := c.ShouldBindJSON(&req); err != nil {
        RespondWithBadRequest(c, err.Error())
        return
    }
    if req.WindowStart < 0 || req.WindowStart > 23 || req.WindowEnd < 0 || req.WindowEnd > 23 {
        RespondWithBadRequest(c, "window_start and window_end must be 0–23")
        return
    }
    config.AppConfig.MaintenanceWindowEnabled = req.Enabled
    config.AppConfig.MaintenanceWindowStart = req.WindowStart
    config.AppConfig.MaintenanceWindowEnd = req.WindowEnd
    if s.Store() != nil {
        if err := config.SaveConfigToDatabase(s.Store()); err != nil {
            internalError(c, "failed to save config", err)
            return
        }
    }
    RespondWithOK(c, gin.H{"ok": true})
}
```

**Route registration** — `internal/server/server.go`, near `POST /maintenance-window/run`:

```go
api.GET("/maintenance-window/status", auth.RequirePermission(auth.PermSettingsManage), s.getMaintenanceWindowStatus)
api.PUT("/maintenance-window/config", auth.RequirePermission(auth.PermSettingsManage), s.updateMaintenanceWindowConfig)
```

---

### Frontend — 4 Changes

#### 1. Fix `getRegisteredTasks()` and Extend Types

**File:** `web/src/services/api.ts`

Add `is_running: boolean` to `TaskInfo` (line ~3224).

Fix the fetch to unwrap the envelope (line ~3227):

```typescript
export async function getRegisteredTasks(): Promise<TaskInfo[]> {
  const response = await fetch(`${API_BASE}/tasks`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch tasks');
  }
  const body = await response.json();
  return Array.isArray(body) ? body : (body?.data ?? []);
}
```

Add new interfaces and functions after `updateTaskConfig`:

```typescript
export interface MaintenanceWindowStatus {
  enabled: boolean;
  window_start: number;
  window_end: number;
  last_run_date: string;
  next_run_estimate: string;
  currently_running: boolean;
}

export interface MaintenanceWindowConfig {
  enabled: boolean;
  window_start: number;
  window_end: number;
}

export async function getMaintenanceWindowStatus(): Promise<MaintenanceWindowStatus> {
  const response = await fetch(`${API_BASE}/maintenance-window/status`);
  if (!response.ok) throw await buildApiError(response, 'Failed to fetch maintenance window status');
  return response.json();
}

export async function updateMaintenanceWindowConfig(cfg: MaintenanceWindowConfig): Promise<void> {
  const response = await fetch(`${API_BASE}/maintenance-window/config`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(cfg),
  });
  if (!response.ok) throw await buildApiError(response, 'Failed to update maintenance window config');
}
```

Note: `runMaintenanceWindow()` already exists — reuse it.

#### 2. Create `MaintenanceTab` Component

**File:** `web/src/components/system/MaintenanceTab.tsx` (new)

Exports: `export function MaintenanceTab() { ... }`

Sub-components (defined in the same file, not exported):
- `MaintenanceWindowCard` — schedule config at top of tab
- `TaskCard` — one card per task, inline controls

The full component is derived from the existing `Maintenance.tsx`, adapted to:
- Remove outer `<Box sx={{ p: 3 }}>` container and page `<Typography variant="h4">` title
  (System.tsx tab panel provides spacing)
- Add `MaintenanceWindowCard` at the top before the task list
- Show `<CircularProgress size={16} />` in the task name row when `task.is_running === true`
- Import from `../../services/api` (not `../services/api`)

See implementation plan for full component code.

#### 3. Add Maintenance Tab to `System.tsx`

**File:** `web/src/pages/System.tsx`

- Bump version header to `1.3.0`
- Add import: `import { MaintenanceTab } from '../components/system/MaintenanceTab';`
- Add `<Tab label="Maintenance" />` between Quota (index 2) and Logs (index 3)
  → Logs shifts to index 4
- Add `<TabPanel value={tabValue} index={3}><MaintenanceTab /></TabPanel>`
- Update Logs TabPanel from `index={3}` to `index={4}`

Tab order: **System Info (0) | Storage (1) | Quota (2) | Maintenance (3) | Logs (4)**

#### 4. Routing + Nav Cleanup

**`web/src/App.tsx`:**
- Remove `const Maintenance = React.lazy(() => import('./pages/Maintenance'));`
- Remove `<Route path="/maintenance" element={...} />`

**`web/src/components/Sidebar.tsx`:**
- Remove the ListItem/NavItem linking to `/maintenance`

**Delete:** `web/src/pages/Maintenance.tsx`

---

## Component Design: MaintenanceWindowCard

```
┌─ Maintenance Window ─────────────────────────────────────────────────────┐
│  [●] Enabled                                           [● running]        │
│  Window: [02] : 00  to  [06] : 00  (hours, 0–23)                        │
│  Last run: 2026-04-26 · Next: 2026-04-27 at 02:00                       │
│                                             [Save]  [Run Maintenance Now] │
└──────────────────────────────────────────────────────────────────────────┘
```

State: `status`, `config` (local draft for save), `saving`, `error`  
On mount: `getMaintenanceWindowStatus()` → populate.  
Save: `updateMaintenanceWindowConfig(config)`.  
Run Now: `runMaintenanceWindow()`.  
Shows `CircularProgress` in card action area when `status.currently_running`.

---

## Component Design: TaskCard

```
┌─ [●] dedup_refresh ─────────────────────────────────────  Active  ──────┐
│  Refresh author & series dedup cache                                      │
│  Last run: 2026-04-27 03:15:00                                            │
│  [●] Enabled  [60] min  [●] On Start  [●] Maint. Window    [Run Now]    │
└──────────────────────────────────────────────────────────────────────────┘
```

`is_running === true` → spinner replaces bullet prefix in name row; "Run Now" disabled.

---

## Data Flow

```
/system → Maintenance tab selected
  └─ MaintenanceTab (useEffect, 10s poll)
       ├─ getMaintenanceWindowStatus() → MaintenanceWindowCard
       └─ getRegisteredTasks() → TaskCard[]
              updateTaskConfig(name, patch)   → PUT /tasks/:name
              runTask(name)                   → POST /tasks/:name/run
              updateMaintenanceWindowConfig() → PUT /maintenance-window/config
              runMaintenanceWindow()          → POST /maintenance-window/run
```

---

## Test Plan

**Backend (in existing test files):**
- `TestListTasksHasIsRunning` — `GET /api/v1/tasks` response has `is_running` bool per task
- `TestGetMaintenanceWindowStatus` — all fields present, valid types
- `TestUpdateMaintenanceWindowConfig_Valid` — 200, config persisted
- `TestUpdateMaintenanceWindowConfig_InvalidHour` — 400 on hour > 23

**Frontend:** No new tests. Delete `Maintenance.test.tsx` if it exists.

---

## Rollout

1. Backend PR merged first (adds `is_running`, new endpoints)
2. Frontend PR rebased on backend PR (fixes bug, creates tab, cleans up route)
3. Smoke test: navigate `/system` → Maintenance tab → tasks load, window config saves
