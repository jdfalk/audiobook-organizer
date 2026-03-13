// file: web/src/pages/Maintenance.tsx
// version: 1.1.0
// guid: b2c3d4e5-f6a7-8901-bcde-f23456789012

import { useEffect, useState } from 'react';
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  FormControlLabel,
  Stack,
  Switch,
  TextField,
  Typography,
} from '@mui/material';
import PlayArrowIcon from '@mui/icons-material/PlayArrow.js';
import * as api from '../services/api';

const categoryLabels: Record<string, string> = {
  library: 'Library',
  sync: 'Sync',
  maintenance: 'Maintenance',
};

const categoryOrder = ['library', 'sync', 'maintenance'];

export function Maintenance() {
  const [tasks, setTasks] = useState<api.TaskInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [runningTask, setRunningTask] = useState<string | null>(null);
  const [successMsg, setSuccessMsg] = useState<string | null>(null);

  const fetchTasks = async () => {
    try {
      const data = await api.getRegisteredTasks();
      setTasks(data);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load tasks');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchTasks();
    const interval = setInterval(fetchTasks, 10000);
    return () => clearInterval(interval);
  }, []);

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

  const handleRunMaintenanceWindow = async () => {
    setSuccessMsg(null);
    try {
      await api.runMaintenanceWindow();
      setSuccessMsg('Maintenance window triggered');
      setTimeout(fetchTasks, 1000);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to run maintenance window');
    }
  };

  const grouped = categoryOrder.map((cat) => ({
    category: cat,
    label: categoryLabels[cat] || cat,
    tasks: tasks.filter((t) => t.category === cat),
  })).filter((g) => g.tasks.length > 0);

  return (
    <Box sx={{ p: 3 }}>
      <Typography variant="h4" gutterBottom>
        Maintenance & Scheduled Tasks
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        Configure and manually trigger background tasks. Scheduled tasks run automatically at the configured interval.
      </Typography>

      <Box sx={{ mb: 3 }}>
        <Button
          variant="outlined"
          startIcon={<PlayArrowIcon />}
          onClick={handleRunMaintenanceWindow}
        >
          Run Maintenance Now
        </Button>
      </Box>

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
        <Typography>Loading tasks...</Typography>
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
                    <CardContent sx={{ display: 'flex', alignItems: 'center', gap: 2, py: 1.5, '&:last-child': { pb: 1.5 } }}>
                      <Box sx={{ flex: 1, minWidth: 0 }}>
                        <Typography variant="subtitle2">{task.description}</Typography>
                        <Typography variant="caption" color="text.secondary">
                          {task.name}
                          {task.last_run && ` · Last run: ${new Date(task.last_run).toLocaleString()}`}
                        </Typography>
                      </Box>

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
                            onChange={(e) => handleMaintenanceWindowToggle(task.name, e.target.checked)}
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
                        startIcon={<PlayArrowIcon />}
                        disabled={runningTask === task.name}
                        onClick={() => handleRun(task.name)}
                      >
                        {runningTask === task.name ? 'Starting...' : 'Run Now'}
                      </Button>
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
