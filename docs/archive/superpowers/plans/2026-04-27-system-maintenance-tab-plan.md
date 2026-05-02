<!-- file: docs/superpowers/plans/2026-04-27-system-maintenance-tab-plan.md -->
<!-- version: 1.0.0 -->
<!-- guid: b2c3d4e5-f6a7-8901-bcde-234567890123 -->

# Implementation Plan: System Maintenance Tab

**Spec:** `docs/superpowers/specs/2026-04-27-system-maintenance-tab-design.md`  
**Model:** Haiku subagents  
**Convention:** rebase/FF only (`gh pr merge --rebase`), conventional commits mandatory,
version headers bumped on every file touched, `make ci` must pass before merging.

---

## Execution Order

```
Wave 1 (parallel):  Agent 1 (PR-backend)  +  Agent 2 (PR-frontend)
Wave 2:             Agent 2 PR rebased onto Agent 1 after Agent 1 merges
```

Agent 1 owns all backend changes. Agent 2 owns all frontend changes.
Agent 2 can start immediately; it just needs to rebase before its PR merges.

---

## Agent 1 — Backend

**Branch:** `feat/maintenance-tab-backend`  
**Worktree:** `git worktree add ../audiobook-organizer-maintenance-backend -b feat/maintenance-tab-backend`

### Step 1 — Add `IsRunning` to `TaskInfo` and populate it

**File:** `internal/server/scheduler.go`

1. Read the file to confirm current line numbers before editing.

2. Find the `TaskInfo` struct (~line 42). After the `LastRun` field, add:
   ```go
   IsRunning bool `json:"is_running"`
   ```

3. Find `ListTasks()` (~line 1257). Inside the loop, after the `if t, ok := ts.lastRun[name]` block
   and before `result = append(result, info)`, add:
   ```go
   info.IsRunning = ts.isTaskRunning(info.Name)
   ```

4. After `saveLastMaintenanceRun()` (~line 1338), add these two new exported methods:

   ```go
   // GetLastMaintenanceRunDate returns the last-run date as "2006-01-02", or "" if never run.
   func (ts *TaskScheduler) GetLastMaintenanceRunDate() string {
       if ts.lastMaintenanceRun.IsZero() {
           return ""
       }
       return ts.lastMaintenanceRun.Format("2006-01-02")
   }

   // IsMaintenanceRunning returns true if a maintenance-window operation is active.
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

5. Bump the file's version header.

### Step 2 — Add two new handler functions

**File:** `internal/server/operations_handlers.go`

1. Read the file. Find the end of `runMaintenanceWindowNow` (~line 862).

2. Add the following immediately after that function. Make sure `"time"` is in the import block
   (add it if missing — check existing imports first).

   ```go
   // getMaintenanceWindowStatus returns current schedule config and live running status.
   func (s *Server) getMaintenanceWindowStatus(c *gin.Context) {
       if s.scheduler == nil {
           RespondWithInternalError(c, "scheduler not initialized")
           return
       }
       cfg := config.AppConfig
       c.JSON(http.StatusOK, gin.H{
           "enabled":           cfg.MaintenanceWindowEnabled,
           "window_start":      cfg.MaintenanceWindowStart,
           "window_end":        cfg.MaintenanceWindowEnd,
           "last_run_date":     s.scheduler.GetLastMaintenanceRunDate(),
           "next_run_estimate": calculateNextWindowRun(cfg.MaintenanceWindowStart),
           "currently_running": s.scheduler.IsMaintenanceRunning(),
       })
   }

   // calculateNextWindowRun returns the next RFC3339 timestamp when startHour occurs locally.
   func calculateNextWindowRun(startHour int) string {
       now := time.Now()
       next := time.Date(now.Year(), now.Month(), now.Day(), startHour, 0, 0, 0, now.Location())
       if !next.After(now) {
           next = next.Add(24 * time.Hour)
       }
       return next.Format(time.RFC3339)
   }

   type maintenanceWindowConfigReq struct {
       Enabled     bool `json:"enabled"`
       WindowStart int  `json:"window_start"`
       WindowEnd   int  `json:"window_end"`
   }

   // updateMaintenanceWindowConfig persists maintenance window schedule settings.
   func (s *Server) updateMaintenanceWindowConfig(c *gin.Context) {
       var req maintenanceWindowConfigReq
       if err := c.ShouldBindJSON(&req); err != nil {
           RespondWithBadRequest(c, err.Error())
           return
       }
       if req.WindowStart < 0 || req.WindowStart > 23 || req.WindowEnd < 0 || req.WindowEnd > 23 {
           RespondWithBadRequest(c, "window_start and window_end must be 0-23")
           return
       }
       config.AppConfig.MaintenanceWindowEnabled = req.Enabled
       config.AppConfig.MaintenanceWindowStart = req.WindowStart
       config.AppConfig.MaintenanceWindowEnd = req.WindowEnd
       if s.Store() != nil {
           if err := config.SaveConfigToDatabase(s.Store()); err != nil {
               internalError(c, "failed to save maintenance window config", err)
               return
           }
       }
       RespondWithOK(c, gin.H{"ok": true})
   }
   ```

3. Bump the file's version header.

### Step 3 — Register the new routes

**File:** `internal/server/server.go`

1. Read the file. Find the block containing `api.POST("/maintenance-window/run", ...)`.

2. Immediately after that line, add:
   ```go
   api.GET("/maintenance-window/status", auth.RequirePermission(auth.PermSettingsManage), s.getMaintenanceWindowStatus)
   api.PUT("/maintenance-window/config", auth.RequirePermission(auth.PermSettingsManage), s.updateMaintenanceWindowConfig)
   ```

3. Bump the file's version header.

### Step 4 — Write backend tests

**File:** Find the existing test file for `operations_handlers.go` — look for
`operations_handlers_test.go` or a nearby `*_test.go` that tests `listTasks` / `runTask`.
Add to whichever file is most appropriate (or create `maintenance_window_handlers_test.go`
if no suitable file exists).

```go
func TestListTasksHasIsRunning(t *testing.T) {
    // Use the existing test server setup pattern in this package.
    // GET /api/v1/tasks
    // Parse response body.
    // Assert len(tasks) > 0.
    // Assert each task has is_running field (bool).
}

func TestGetMaintenanceWindowStatus(t *testing.T) {
    // GET /api/v1/maintenance-window/status
    // Assert 200 OK.
    // Unmarshal body. Assert all fields present:
    //   enabled (bool), window_start (int), window_end (int),
    //   next_run_estimate (non-empty string), currently_running (bool).
}

func TestUpdateMaintenanceWindowConfig_Valid(t *testing.T) {
    // PUT /api/v1/maintenance-window/config  {"enabled":true,"window_start":3,"window_end":5}
    // Assert 200 OK.
    // GET /api/v1/maintenance-window/status.
    // Assert window_start==3, window_end==5, enabled==true.
}

func TestUpdateMaintenanceWindowConfig_InvalidHour(t *testing.T) {
    // PUT /api/v1/maintenance-window/config  {"enabled":true,"window_start":24,"window_end":5}
    // Assert 400 Bad Request.
}
```

### Step 5 — Verify and commit

```bash
make test          # must pass
make build-api     # must compile
```

Commit message:
```
feat(scheduler): add is_running to TaskInfo and maintenance window status/config endpoints
```

Open PR targeting `main`. Title: `feat(scheduler): maintenance window status/config API + is_running field`

---

## Agent 2 — Frontend

**Branch:** `feat/maintenance-tab-frontend`  
**Worktree:** `git worktree add ../audiobook-organizer-maintenance-frontend -b feat/maintenance-tab-frontend`

> Agent 2 can start immediately. Before opening the PR, rebase onto `origin/main` so Agent 1's
> backend changes are included.

### Step 1 — Fix and extend `web/src/services/api.ts`

1. Read the file. Locate the `TaskInfo` interface (~line 3216) and the `getRegisteredTasks`
   function (~line 3227).

2. **Add `is_running` to `TaskInfo`:**
   After `last_run?: string;`, add:
   ```typescript
   is_running: boolean;
   ```

3. **Fix `getRegisteredTasks()`:** Replace the existing function body:
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

4. **Add new interfaces and functions** immediately after `updateTaskConfig` (~line 3262):
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

   Note: `runMaintenanceWindow()` already exists at ~line 3243 — do NOT add a duplicate.

5. Bump the file's version header.

### Step 2 — Create `MaintenanceTab.tsx`

**File:** `web/src/components/system/MaintenanceTab.tsx` (new file)

Read `web/src/pages/Maintenance.tsx` in full before writing. Read
`web/src/components/system/SystemInfoTab.tsx` lines 1–30 for the file-header pattern.

Write the file with the following content (adapt file header to match project style):

```typescript
// file: web/src/components/system/MaintenanceTab.tsx
// version: 1.0.0
// guid: c3d4e5f6-a7b8-9012-cdef-345678901234

import { useEffect, useState, useCallback } from 'react';
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  CardHeader,
  Chip,
  CircularProgress,
  FormControlLabel,
  Stack,
  Switch,
  TextField,
  Typography,
  useMediaQuery,
  useTheme,
} from '@mui/material';
import PlayArrowIcon from '@mui/icons-material/PlayArrow.js';
import * as api from '../../services/api';

// ─── MaintenanceWindowCard ───────────────────────────────────────────────────

function MaintenanceWindowCard() {
  const [status, setStatus] = useState<api.MaintenanceWindowStatus | null>(null);
  const [config, setConfig] = useState<api.MaintenanceWindowConfig>({
    enabled: false,
    window_start: 1,
    window_end: 4,
  });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [successMsg, setSuccessMsg] = useState<string | null>(null);

  const loadStatus = useCallback(async () => {
    try {
      const s = await api.getMaintenanceWindowStatus();
      setStatus(s);
      setConfig({ enabled: s.enabled, window_start: s.window_start, window_end: s.window_end });
    } catch (e) {
      // non-fatal: if backend doesn't have the endpoint yet, degrade silently
    }
  }, []);

  useEffect(() => { loadStatus(); }, [loadStatus]);

  const handleSave = async () => {
    setSaving(true);
    setError(null);
    setSuccessMsg(null);
    try {
      await api.updateMaintenanceWindowConfig(config);
      setSuccessMsg('Maintenance window settings saved');
      await loadStatus();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to save');
    } finally {
      setSaving(false);
    }
  };

  const handleRunNow = async () => {
    setError(null);
    setSuccessMsg(null);
    try {
      await api.runMaintenanceWindow();
      setSuccessMsg('Maintenance window triggered');
      setTimeout(loadStatus, 1000);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to trigger');
    }
  };

  const nextRunLabel = status?.next_run_estimate
    ? new Date(status.next_run_estimate).toLocaleString()
    : '—';
  const lastRunLabel = status?.last_run_date || 'Never';

  return (
    <Card sx={{ mb: 3 }}>
      <CardHeader
        title="Maintenance Window"
        subheader={`Last run: ${lastRunLabel} · Next run: ${nextRunLabel}`}
        action={status?.currently_running ? <CircularProgress size={20} sx={{ mt: 1, mr: 1 }} /> : null}
      />
      <CardContent>
        {error && (
          <Alert severity="error" sx={{ mb: 1.5 }} onClose={() => setError(null)}>
            {error}
          </Alert>
        )}
        {successMsg && (
          <Alert severity="success" sx={{ mb: 1.5 }} onClose={() => setSuccessMsg(null)}>
            {successMsg}
          </Alert>
        )}
        <Box sx={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 2 }}>
          <FormControlLabel
            control={
              <Switch
                checked={config.enabled}
                onChange={(e) => setConfig((c) => ({ ...c, enabled: e.target.checked }))}
              />
            }
            label="Enabled"
          />
          <TextField
            label="Start hour (0–23)"
            type="number"
            size="small"
            inputProps={{ min: 0, max: 23 }}
            value={config.window_start}
            onChange={(e) =>
              setConfig((c) => ({ ...c, window_start: Math.min(23, Math.max(0, parseInt(e.target.value) || 0)) }))
            }
            sx={{ width: 155 }}
          />
          <TextField
            label="End hour (0–23)"
            type="number"
            size="small"
            inputProps={{ min: 0, max: 23 }}
            value={config.window_end}
            onChange={(e) =>
              setConfig((c) => ({ ...c, window_end: Math.min(23, Math.max(0, parseInt(e.target.value) || 0)) }))
            }
            sx={{ width: 155 }}
          />
          <Button variant="outlined" onClick={handleSave} disabled={saving}>
            {saving ? 'Saving…' : 'Save'}
          </Button>
          <Button
            variant="contained"
            startIcon={<PlayArrowIcon />}
            onClick={handleRunNow}
            disabled={status?.currently_running ?? false}
          >
            Run Maintenance Now
          </Button>
        </Box>
      </CardContent>
    </Card>
  );
}

// ─── MaintenanceTab ──────────────────────────────────────────────────────────

const categoryLabels: Record<string, string> = {
  library: 'Library',
  sync: 'Sync',
  maintenance: 'Maintenance',
};
const categoryOrder = ['library', 'sync', 'maintenance'];

export function MaintenanceTab() {
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down('sm'));
  const [tasks, setTasks] = useState<api.TaskInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [runningTask, setRunningTask] = useState<string | null>(null);
  const [successMsg, setSuccessMsg] = useState<string | null>(null);

  const fetchTasks = useCallback(async () => {
    try {
      const data = await api.getRegisteredTasks();
      setTasks(data);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load tasks');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchTasks();
    const interval = setInterval(fetchTasks, 10000);
    return () => clearInterval(interval);
  }, [fetchTasks]);

  const handleRun = async (name: string) => {
    setRunningTask(name);
    setSuccessMsg(null);
    try {
      await api.runTask(name);
      setSuccessMsg(`Task "${name}" started`);
      setTimeout(fetchTasks, 1000);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to run task');
    } finally {
      setRunningTask(null);
    }
  };

  const handleToggle = async (name: string, enabled: boolean) => {
    try {
      await api.updateTaskConfig(name, { enabled });
      fetchTasks();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update task');
    }
  };

  const handleIntervalChange = async (name: string, minutes: number) => {
    try {
      await api.updateTaskConfig(name, { interval_minutes: minutes });
      fetchTasks();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update interval');
    }
  };

  const handleStartupToggle = async (name: string, runOnStartup: boolean) => {
    try {
      await api.updateTaskConfig(name, { run_on_startup: runOnStartup });
      fetchTasks();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update startup setting');
    }
  };

  const handleMaintenanceWindowToggle = async (name: string, runInMaintenanceWindow: boolean) => {
    try {
      await api.updateTaskConfig(name, { run_in_maintenance_window: runInMaintenanceWindow });
      fetchTasks();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update maintenance window setting');
    }
  };

  const grouped = categoryOrder
    .map((cat) => ({
      category: cat,
      label: categoryLabels[cat] || cat,
      tasks: tasks.filter((t) => t.category === cat),
    }))
    .filter((g) => g.tasks.length > 0);

  return (
    <Box>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        Configure and manually trigger background tasks. Tasks marked "Maint. Window" run
        automatically during the scheduled maintenance window.
      </Typography>

      <MaintenanceWindowCard />

      {error && (
        <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>
          {error}
        </Alert>
      )}
      {successMsg && (
        <Alert severity="success" sx={{ mb: 2 }} onClose={() => setSuccessMsg(null)}>
          {successMsg}
        </Alert>
      )}

      {loading ? (
        <Typography>Loading tasks…</Typography>
      ) : (
        <Stack spacing={4}>
          {grouped.map((group) => (
            <Box key={group.category}>
              <Typography variant="h6" sx={{ mb: 1 }}>
                {group.label}
              </Typography>
              <Stack spacing={1}>
                {group.tasks.map((task) => (
                  <Card key={task.name} variant="outlined">
                    <CardContent sx={{ py: 1.5, '&:last-child': { pb: 1.5 } }}>
                      <Box sx={{ mb: 0.5, display: 'flex', alignItems: 'center', gap: 1 }}>
                        {task.is_running && <CircularProgress size={14} />}
                        <Typography variant="subtitle2" sx={{ flexGrow: 1 }}>
                          {task.description}
                        </Typography>
                      </Box>
                      <Typography variant="caption" color="text.secondary">
                        {task.name}
                        {task.last_run && ` · Last run: ${new Date(task.last_run).toLocaleString()}`}
                      </Typography>

                      <Box
                        sx={{
                          display: 'flex',
                          flexWrap: 'wrap',
                          alignItems: 'center',
                          gap: 1,
                          mt: isMobile ? 1 : 0.5,
                        }}
                      >
                        <FormControlLabel
                          control={
                            <Switch
                              size="small"
                              checked={task.enabled}
                              onChange={(e) => handleToggle(task.name, e.target.checked)}
                            />
                          }
                          label="Enabled"
                          sx={{ mx: 0 }}
                        />

                        {task.interval_minutes > 0 && (
                          <TextField
                            label="Interval (min)"
                            type="number"
                            size="small"
                            value={task.interval_minutes}
                            onChange={(e) => {
                              const val = parseInt(e.target.value, 10);
                              if (val > 0) handleIntervalChange(task.name, val);
                            }}
                            sx={{ width: 120 }}
                            inputProps={{ min: 1 }}
                          />
                        )}

                        <FormControlLabel
                          control={
                            <Switch
                              size="small"
                              checked={task.run_on_startup}
                              onChange={(e) => handleStartupToggle(task.name, e.target.checked)}
                            />
                          }
                          label="On Start"
                          sx={{ mx: 0 }}
                        />

                        <FormControlLabel
                          control={
                            <Switch
                              size="small"
                              checked={task.run_in_maintenance_window}
                              onChange={(e) =>
                                handleMaintenanceWindowToggle(task.name, e.target.checked)
                              }
                            />
                          }
                          label="Maint. Window"
                          sx={{ mx: 0 }}
                        />

                        <Chip
                          size="small"
                          label={task.enabled ? 'Active' : 'Disabled'}
                          color={task.enabled ? 'success' : 'default'}
                          variant="outlined"
                        />

                        <Button
                          variant="contained"
                          size="small"
                          fullWidth={isMobile}
                          startIcon={
                            task.is_running ? <CircularProgress size={14} color="inherit" /> : <PlayArrowIcon />
                          }
                          disabled={runningTask === task.name || task.is_running}
                          onClick={() => handleRun(task.name)}
                        >
                          {runningTask === task.name ? 'Starting…' : 'Run Now'}
                        </Button>
                      </Box>
                    </CardContent>
                  </Card>
                ))}
              </Stack>
            </Box>
          ))}
        </Stack>
      )}
    </Box>
  );
}
```

### Step 3 — Add the Maintenance tab to `System.tsx`

**File:** `web/src/pages/System.tsx`

1. Read the file first to confirm current content.

2. Add import after the `QuotaTab` import line:
   ```typescript
   import { MaintenanceTab } from '../components/system/MaintenanceTab';
   ```

3. In the `<Tabs>` block, add a new `<Tab>` between Quota and Logs:
   ```tsx
   <Tab label="Maintenance" />
   ```
   After this change the indices are: System Info=0, Storage=1, Quota=2, **Maintenance=3**, Logs=4.

4. Add a new `<TabPanel>` after the Quota panel (index 2) and before the existing Logs panel:
   ```tsx
   <TabPanel value={tabValue} index={3}>
     <MaintenanceTab />
   </TabPanel>
   ```

5. Update the Logs `<TabPanel>` index from `3` to `4`:
   ```tsx
   <TabPanel value={tabValue} index={4}>
     <LogsTab />
   </TabPanel>
   ```

6. Bump the file's version header from `1.2.0` to `1.3.0`.

### Step 4 — Remove `/maintenance` route from `App.tsx`

**File:** `web/src/App.tsx`

1. Read the file. Find and delete the lazy import line:
   ```typescript
   const Maintenance = React.lazy(() => import('./pages/Maintenance'));
   ```

2. Find and delete the Route element for `/maintenance` (it wraps `<Maintenance />`).
   It looks like: `<Route path="/maintenance" element={<Maintenance />} />`
   or it may be wrapped in a Suspense boundary — delete just the `<Route>` element, not
   the surrounding Suspense if other routes share it.

3. Bump the file's version header.

### Step 5 — Remove Maintenance from `Sidebar.tsx`

**File:** `web/src/components/Sidebar.tsx`

1. Read the file. Find the list item for Maintenance — it links to `/maintenance` and uses
   `BuildIcon` (or similar). Remove the entire ListItem / NavItem element for it.

2. If `BuildIcon` is no longer used anywhere in the file after removal, remove that import too.

3. Bump the file's version header.

### Step 6 — Delete `Maintenance.tsx`

Only after Steps 3–5 are complete and `npm run build` passes:

```bash
rm web/src/pages/Maintenance.tsx
```

Verify `npm run build` still passes after deletion.

### Step 7 — Verify and commit

```bash
make build   # full build: npm + go
make ci      # all tests + 80% coverage check
```

Commit message:
```
feat(system): add Maintenance tab with window scheduling, fix task-list rendering bug
```

Open PR targeting `main`.  
Title: `feat(system): merge Maintenance page into System tab + fix task list bug`

Before merging, rebase onto `origin/main` to pick up Agent 1's backend changes:
```bash
git fetch origin main
git rebase origin/main
```

---

## Post-Merge Smoke Test

After both PRs are merged and deployed:

```bash
# On prod server or locally
curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/tasks | jq '.[0].is_running'
# → false

curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/maintenance-window/status | jq .
# → {"enabled":..., "window_start":..., "window_end":..., "last_run_date":..., ...}

curl -s -X PUT -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"enabled":true,"window_start":2,"window_end":6}' \
  http://localhost:8080/api/v1/maintenance-window/config
# → {"ok":true}
```

Browser: navigate to `/system` → click **Maintenance** tab → tasks load, window card shows,
"Run Now" buttons work, `/maintenance` no longer appears in sidebar.

---

## Files Changed Summary

| File | Change |
|------|--------|
| `internal/server/scheduler.go` | Add `IsRunning` to `TaskInfo`; populate in `ListTasks`; add `GetLastMaintenanceRunDate()` + `IsMaintenanceRunning()` |
| `internal/server/operations_handlers.go` | Add `getMaintenanceWindowStatus`, `updateMaintenanceWindowConfig`, `calculateNextWindowRun` |
| `internal/server/server.go` | Register 2 new routes |
| `web/src/services/api.ts` | Fix `getRegisteredTasks()`, add `is_running` to `TaskInfo`, add `MaintenanceWindowStatus`/`Config` interfaces + 2 new functions |
| `web/src/components/system/MaintenanceTab.tsx` | **New file** — full maintenance tab component |
| `web/src/pages/System.tsx` | Add Maintenance tab (index 3), shift Logs to index 4 |
| `web/src/App.tsx` | Remove lazy import + Route for `/maintenance` |
| `web/src/components/Sidebar.tsx` | Remove Maintenance nav entry |
| `web/src/pages/Maintenance.tsx` | **Deleted** |
