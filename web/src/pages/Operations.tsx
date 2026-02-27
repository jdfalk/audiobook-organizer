// file: web/src/pages/Operations.tsx
// version: 2.0.0
// guid: 3b2a1c4d-5e6f-7081-9a0b-1c2d3e4f5a6b

import { useEffect, useMemo, useRef, useState } from 'react';
import {
  Alert,
  Box,
  Button,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  LinearProgress,
  List,
  ListItem,
  ListItemText,
  MenuItem,
  Pagination,
  Paper,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import * as api from '../services/api';

const HISTORY_PAGE_SIZE = 20;

function formatOperationType(type: string): string {
  switch (type) {
    case 'itunes_import':
      return 'iTunes Import';
    case 'scan':
      return 'Library Scan';
    case 'organize':
      return 'Organize';
    case 'metadata_fetch':
      return 'Metadata Fetch';
    case 'ol_dump_import':
      return 'Open Library Import';
    default:
      return type.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
  }
}

function statusChip(status: string) {
  const map: Record<string, { color: 'success' | 'error' | 'warning' | 'info' | 'default'; label: string }> = {
    completed: { color: 'success', label: 'Completed' },
    failed: { color: 'error', label: 'Failed' },
    canceled: { color: 'default', label: 'Cancelled' },
    running: { color: 'info', label: 'Running' },
    queued: { color: 'warning', label: 'Queued' },
    interrupted: { color: 'warning', label: 'Interrupted' },
  };
  const info = map[status] || { color: 'default' as const, label: status };
  return <Chip size="small" color={info.color} label={info.label} />;
}

function formatDuration(start: string, end?: string | null): string {
  const s = new Date(start).getTime();
  const e = end ? new Date(end).getTime() : Date.now();
  const sec = Math.floor((e - s) / 1000);
  if (sec < 60) return `${sec}s`;
  if (sec < 3600) return `${Math.floor(sec / 60)}m ${sec % 60}s`;
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  return `${h}h ${m}m`;
}

export function Operations() {
  const [activeOperations, setActiveOperations] = useState<
    api.ActiveOperationSummary[]
  >([]);
  const [history, setHistory] = useState<api.Operation[]>([]);
  const [historyPage, setHistoryPage] = useState(1);
  const [logDialogOpen, setLogDialogOpen] = useState(false);
  const [selectedOperation, setSelectedOperation] =
    useState<api.Operation | null>(null);
  const [logs, setLogs] = useState<api.OperationLog[]>([]);
  const [logsLoading, setLogsLoading] = useState(false);
  const [logFilter, setLogFilter] = useState('all');
  const [errorDialogOpen, setErrorDialogOpen] = useState(false);
  const [errorDetails, setErrorDetails] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [cancelling, setCancelling] = useState<Set<string>>(new Set());
  const logContainerRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    loadHistory();
    loadActiveOperations();
    const interval = window.setInterval(loadActiveOperations, 2000);
    return () => window.clearInterval(interval);
  }, []);

  const loadHistory = async () => {
    try {
      const status = await api.getSystemStatus();
      setHistory(status.operations?.recent || []);
    } catch (error) {
      console.error('Failed to load operations history', error);
      setNotice('Failed to load operations history.');
    }
  };

  const loadActiveOperations = async () => {
    try {
      const active = await api.getActiveOperations();
      setActiveOperations(active);
    } catch (error) {
      console.error('Failed to load active operations', error);
    }
  };

  const handleViewLogs = async (operation: api.Operation) => {
    setSelectedOperation(operation);
    setLogDialogOpen(true);
    setLogsLoading(true);
    try {
      const data = await api.getOperationLogs(operation.id);
      setLogs(data);
    } catch (error) {
      console.error('Failed to load operation logs', error);
      setLogs([]);
    } finally {
      setLogsLoading(false);
    }
  };

  const handleCancelOperation = async (operationId: string) => {
    setCancelling((prev) => new Set(prev).add(operationId));
    try {
      await api.cancelOperation(operationId);
      setNotice('Operation cancelled.');
      await loadActiveOperations();
      await loadHistory();
    } catch (error) {
      console.error('Failed to cancel operation', error);
      setNotice('Failed to cancel operation.');
    }
    setCancelling((prev) => {
      const next = new Set(prev);
      next.delete(operationId);
      return next;
    });
  };

  const handleRetryOperation = async (operation: api.Operation) => {
    try {
      if (operation.type === 'scan') {
        await api.startScan(operation.folder_path);
      } else if (operation.type === 'organize') {
        await api.startOrganize(operation.folder_path);
      }
      setNotice('Operation retried.');
      await loadActiveOperations();
    } catch (error) {
      console.error('Failed to retry operation', error);
      setNotice('Failed to retry operation.');
    }
  };

  const handleClearCompleted = () => {
    setHistory((prev) =>
      prev.filter((op) => !['completed', 'failed'].includes(op.status))
    );
  };

  const handleClearStuck = async () => {
    try {
      await api.clearStaleOperations();
      loadHistory();
      loadActiveOperations();
    } catch (err) {
      console.error('Failed to clear stuck operations:', err);
    }
  };

  const handleShowError = (operation: api.Operation) => {
    const details =
      operation.error_message ||
      operation.message ||
      'No additional error details provided.';
    setErrorDetails(details);
    setErrorDialogOpen(true);
  };

  const filteredLogs = useMemo(() => {
    if (logFilter === 'all') return logs;
    return logs.filter((log) => log.level === logFilter);
  }, [logs, logFilter]);

  useEffect(() => {
    if (!logDialogOpen) return;
    if (!logContainerRef.current) return;
    logContainerRef.current.scrollTop = logContainerRef.current.scrollHeight;
  }, [filteredLogs, logDialogOpen]);

  const totalPages = Math.max(
    1,
    Math.ceil(history.length / HISTORY_PAGE_SIZE)
  );
  const pagedHistory = history.slice(
    (historyPage - 1) * HISTORY_PAGE_SIZE,
    historyPage * HISTORY_PAGE_SIZE
  );

  return (
    <Box sx={{ height: '100%', overflow: 'auto' }}>
      <Typography variant="h4" gutterBottom>
        Operations
      </Typography>

      {notice && (
        <Alert
          severity="info"
          sx={{ mb: 2 }}
          onClose={() => setNotice(null)}
        >
          {notice}
        </Alert>
      )}

      {/* Active Operations */}
      <Paper sx={{ p: 2, mb: 3 }}>
        <Stack
          direction="row"
          spacing={2}
          alignItems="center"
          justifyContent="space-between"
          mb={2}
        >
          <Typography variant="h6">
            Active Operations
            {activeOperations.length > 0 && (
              <Chip
                size="small"
                label={activeOperations.length}
                color="info"
                sx={{ ml: 1, verticalAlign: 'middle' }}
              />
            )}
          </Typography>
          <Button variant="outlined" size="small" onClick={loadActiveOperations}>
            Refresh
          </Button>
        </Stack>

        {activeOperations.length === 0 ? (
          <Typography variant="body2" color="text.secondary" sx={{ py: 2 }}>
            No active operations.
          </Typography>
        ) : (
          <Stack spacing={2}>
            {activeOperations.map((op) => {
              const progressPct =
                op.total > 0 ? Math.round((op.progress / op.total) * 100) : 0;
              // Extract current book title from message
              const titleMatch = op.message.match(/\u2014\s*(.+)$/);
              const currentTitle = titleMatch ? titleMatch[1] : null;
              // Extract counts from message
              const countsMatch = op.message.match(
                /\(imported (\d+), skipped (\d+), failed (\d+)\)/
              );

              return (
                <Paper
                  key={op.id}
                  variant="outlined"
                  sx={{ p: 2 }}
                >
                  <Stack
                    direction="row"
                    justifyContent="space-between"
                    alignItems="center"
                    mb={1}
                  >
                    <Box>
                      <Typography variant="subtitle1" fontWeight="bold">
                        {formatOperationType(op.type)}
                      </Typography>
                      <Typography variant="caption" color="text.secondary">
                        ID: {op.id.slice(0, 12)}...
                      </Typography>
                    </Box>
                    <Stack direction="row" spacing={1} alignItems="center">
                      {statusChip(op.status)}
                      <Button
                        size="small"
                        variant="outlined"
                        onClick={() =>
                          handleViewLogs({
                            id: op.id,
                            type: op.type,
                            status: op.status,
                            progress: op.progress,
                            total: op.total,
                            message: op.message,
                            created_at: new Date().toISOString(),
                            folder_path: '',
                          })
                        }
                      >
                        Logs
                      </Button>
                      <Button
                        size="small"
                        color="error"
                        variant="contained"
                        onClick={() => handleCancelOperation(op.id)}
                        disabled={cancelling.has(op.id)}
                      >
                        {cancelling.has(op.id) ? 'Cancelling...' : 'Cancel'}
                      </Button>
                    </Stack>
                  </Stack>

                  {/* Progress bar */}
                  {op.total > 0 ? (
                    <Box sx={{ mb: 1 }}>
                      <LinearProgress
                        variant="determinate"
                        value={progressPct}
                        sx={{ height: 8, borderRadius: 1, mb: 0.5 }}
                      />
                      <Stack direction="row" justifyContent="space-between">
                        <Typography variant="body2" color="text.secondary">
                          {op.progress.toLocaleString()} of{' '}
                          {op.total.toLocaleString()} ({progressPct}%)
                        </Typography>
                      </Stack>
                    </Box>
                  ) : (
                    <LinearProgress sx={{ height: 8, borderRadius: 1, mb: 1 }} />
                  )}

                  {/* Current item being processed */}
                  {currentTitle && (
                    <Typography
                      variant="body2"
                      sx={{ mb: 0.5 }}
                      noWrap
                      title={currentTitle}
                    >
                      Currently processing: <strong>{currentTitle}</strong>
                    </Typography>
                  )}

                  {/* Import stats */}
                  {countsMatch && (
                    <Stack direction="row" spacing={2} sx={{ mb: 0.5 }}>
                      <Chip
                        size="small"
                        color="success"
                        variant="outlined"
                        label={`${countsMatch[1]} imported`}
                      />
                      <Chip
                        size="small"
                        color="default"
                        variant="outlined"
                        label={`${countsMatch[2]} skipped`}
                      />
                      {parseInt(countsMatch[3]) > 0 && (
                        <Chip
                          size="small"
                          color="error"
                          variant="outlined"
                          label={`${countsMatch[3]} failed`}
                        />
                      )}
                    </Stack>
                  )}

                  {/* Full message */}
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    display="block"
                  >
                    {op.message}
                  </Typography>
                </Paper>
              );
            })}
          </Stack>
        )}
      </Paper>

      {/* Operation History */}
      <Paper sx={{ p: 2 }}>
        <Stack
          direction="row"
          spacing={2}
          alignItems="center"
          justifyContent="space-between"
          mb={2}
        >
          <Typography variant="h6">Operation History</Typography>
          <Box sx={{ display: 'flex', gap: 1 }}>
            <Button variant="outlined" size="small" color="warning" onClick={handleClearStuck}>
              Clear Stuck
            </Button>
            <Button variant="outlined" size="small" onClick={handleClearCompleted}>
              Clear Completed
            </Button>
          </Box>
        </Stack>
        {pagedHistory.length === 0 ? (
          <Typography variant="body2" color="text.secondary" sx={{ py: 2 }}>
            No historical operations yet.
          </Typography>
        ) : (
          <List disablePadding>
            {pagedHistory.map((op) => (
              <ListItem
                key={op.id}
                divider
                sx={{ px: 0 }}
                secondaryAction={
                  <Stack direction="row" spacing={1}>
                    <Button
                      size="small"
                      variant="outlined"
                      onClick={() => handleViewLogs(op)}
                    >
                      Logs
                    </Button>
                    {op.status === 'failed' && (
                      <>
                        <Button
                          size="small"
                          variant="outlined"
                          onClick={() => handleRetryOperation(op)}
                        >
                          Retry
                        </Button>
                        <Button
                          size="small"
                          color="error"
                          variant="outlined"
                          onClick={() => handleShowError(op)}
                        >
                          Error
                        </Button>
                      </>
                    )}
                  </Stack>
                }
              >
                <ListItemText
                  primary={
                    <Stack direction="row" spacing={1} alignItems="center">
                      <Typography variant="body2" fontWeight="bold">
                        {formatOperationType(op.type)}
                      </Typography>
                      {statusChip(op.status)}
                      {op.progress > 0 && op.total > 0 && (
                        <Typography variant="caption" color="text.secondary">
                          {op.progress}/{op.total}
                        </Typography>
                      )}
                    </Stack>
                  }
                  secondary={
                    <Stack component="span" direction="row" spacing={2}>
                      <Typography variant="caption" component="span" color="text.secondary">
                        {new Date(op.created_at).toLocaleString()}
                      </Typography>
                      {op.completed_at && (
                        <Typography variant="caption" component="span" color="text.secondary">
                          Duration: {formatDuration(op.created_at, op.completed_at)}
                        </Typography>
                      )}
                      {op.message && (
                        <Typography
                          variant="caption"
                          component="span"
                          color="text.secondary"
                          noWrap
                          sx={{ maxWidth: 300 }}
                          title={op.message}
                        >
                          {op.message}
                        </Typography>
                      )}
                    </Stack>
                  }
                />
              </ListItem>
            ))}
          </List>
        )}
        {totalPages > 1 && (
          <Box mt={2} display="flex" justifyContent="center">
            <Pagination
              count={totalPages}
              page={historyPage}
              onChange={(_, page) => setHistoryPage(page)}
              color="primary"
            />
          </Box>
        )}
      </Paper>

      {/* Log Dialog */}
      <Dialog
        open={logDialogOpen}
        onClose={() => setLogDialogOpen(false)}
        fullWidth
        maxWidth="md"
      >
        <DialogTitle>
          {selectedOperation
            ? `Logs: ${formatOperationType(selectedOperation.type)}`
            : 'Operation Logs'}
        </DialogTitle>
        <DialogContent dividers>
          <Stack direction="row" spacing={2} alignItems="center" mb={2}>
            <TextField
              select
              size="small"
              label="Filter"
              value={logFilter}
              onChange={(event) => setLogFilter(event.target.value)}
              sx={{ minWidth: 160 }}
            >
              <MenuItem value="all">All levels</MenuItem>
              <MenuItem value="info">Info</MenuItem>
              <MenuItem value="warning">Warning</MenuItem>
              <MenuItem value="error">Error</MenuItem>
            </TextField>
            <Typography variant="caption" color="text.secondary">
              {filteredLogs.length} entries
            </Typography>
          </Stack>
          {logsLoading ? (
            <LinearProgress />
          ) : filteredLogs.length === 0 ? (
            <Typography variant="body2" color="text.secondary">
              No logs available.
            </Typography>
          ) : (
            <Box
              ref={logContainerRef}
              sx={{
                maxHeight: 400,
                overflow: 'auto',
                fontFamily: 'monospace',
                fontSize: '0.8rem',
              }}
            >
              {filteredLogs.map((log) => (
                <Box
                  key={log.id}
                  sx={{
                    py: 0.5,
                    px: 1,
                    borderBottom: '1px solid',
                    borderColor: 'divider',
                    bgcolor:
                      log.level === 'error'
                        ? 'error.dark'
                        : log.level === 'warning'
                          ? 'warning.dark'
                          : 'transparent',
                    opacity: log.level === 'error' || log.level === 'warning' ? 0.9 : 1,
                  }}
                >
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    component="span"
                    sx={{ mr: 1 }}
                  >
                    {new Date(log.created_at).toLocaleTimeString()}
                  </Typography>
                  <Chip
                    size="small"
                    label={log.level}
                    color={
                      log.level === 'error'
                        ? 'error'
                        : log.level === 'warning'
                          ? 'warning'
                          : 'default'
                    }
                    sx={{ mr: 1, height: 18, fontSize: '0.7rem' }}
                  />
                  <Typography variant="caption" component="span">
                    {log.message}
                  </Typography>
                </Box>
              ))}
            </Box>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setLogDialogOpen(false)}>Close</Button>
        </DialogActions>
      </Dialog>

      {/* Error Dialog */}
      <Dialog
        open={errorDialogOpen}
        onClose={() => setErrorDialogOpen(false)}
      >
        <DialogTitle>Operation Error Details</DialogTitle>
        <DialogContent>
          <Typography
            variant="body2"
            sx={{ fontFamily: 'monospace', whiteSpace: 'pre-wrap' }}
          >
            {errorDetails}
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setErrorDialogOpen(false)}>Close</Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
