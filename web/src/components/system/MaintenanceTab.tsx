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
    } catch {
      // degrade silently if backend endpoint not yet deployed
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
            label="Start hour (0-23)"
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
            label="End hour (0-23)"
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
                            task.is_running
                              ? <CircularProgress size={14} color="inherit" />
                              : <PlayArrowIcon />
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
