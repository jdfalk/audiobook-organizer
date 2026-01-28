// file: web/src/pages/Operations.tsx
// version: 1.0.0
// guid: 3b2a1c4d-5e6f-7081-9a0b-1c2d3e4f5a6b

import { useEffect, useMemo, useRef, useState } from 'react';
import {
  Alert,
  Box,
  Button,
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
  const logContainerRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    loadHistory();
    loadActiveOperations();
    const interval = window.setInterval(loadActiveOperations, 3000);
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
    try {
      await api.cancelOperation(operationId);
      setNotice('Operation cancelled.');
      await loadActiveOperations();
      await loadHistory();
    } catch (error) {
      console.error('Failed to cancel operation', error);
      setNotice('Failed to cancel operation.');
    }
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
        <Alert severity="info" sx={{ mb: 2 }}>
          {notice}
        </Alert>
      )}

      <Paper sx={{ p: 2, mb: 3 }}>
        <Stack
          direction={{ xs: 'column', sm: 'row' }}
          spacing={2}
          alignItems={{ xs: 'flex-start', sm: 'center' }}
          justifyContent="space-between"
          mb={2}
        >
          <Typography variant="h6">Active Operations</Typography>
          <Button variant="outlined" onClick={loadActiveOperations}>
            Refresh
          </Button>
        </Stack>
        {activeOperations.length === 0 ? (
          <Typography variant="body2" color="text.secondary">
            No active operations.
          </Typography>
        ) : (
          <List>
            {activeOperations.map((op) => (
              <ListItem key={op.id} divider>
                <ListItemText
                  primary={`${op.type} • ${op.status}`}
                  secondary={`${op.message || ''}`.trim()}
                />
                <Box sx={{ minWidth: 240, mr: 2 }}>
                  <Typography variant="caption" color="text.secondary">
                    {op.progress}/{op.total}
                  </Typography>
                  <LinearProgress
                    variant="determinate"
                    value={op.total > 0 ? (op.progress / op.total) * 100 : 0}
                    sx={{ mt: 0.5 }}
                  />
                </Box>
                <Stack direction="row" spacing={1}>
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
                    View Logs
                  </Button>
                  <Button
                    size="small"
                    color="error"
                    variant="outlined"
                    onClick={() => handleCancelOperation(op.id)}
                  >
                    Cancel
                  </Button>
                </Stack>
              </ListItem>
            ))}
          </List>
        )}
      </Paper>

      <Paper sx={{ p: 2 }}>
        <Stack
          direction={{ xs: 'column', sm: 'row' }}
          spacing={2}
          alignItems={{ xs: 'flex-start', sm: 'center' }}
          justifyContent="space-between"
          mb={2}
        >
          <Typography variant="h6">Operation History</Typography>
          <Button variant="outlined" onClick={handleClearCompleted}>
            Clear Completed
          </Button>
        </Stack>
        {pagedHistory.length === 0 ? (
          <Typography variant="body2" color="text.secondary">
            No historical operations yet.
          </Typography>
        ) : (
          <List>
            {pagedHistory.map((op) => (
              <ListItem key={op.id} divider>
                <ListItemText
                  primary={`${op.type} • ${op.status}`}
                  secondary={new Date(op.created_at).toLocaleString()}
                />
                <Stack direction="row" spacing={1}>
                  <Button
                    size="small"
                    variant="outlined"
                    onClick={() => handleViewLogs(op)}
                  >
                    View Logs
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
                        Details
                      </Button>
                    </>
                  )}
                </Stack>
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

      <Dialog
        open={logDialogOpen}
        onClose={() => setLogDialogOpen(false)}
        fullWidth
        maxWidth="md"
      >
        <DialogTitle>
          {selectedOperation
            ? `Logs: ${selectedOperation.type}`
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
          </Stack>
          {logsLoading ? (
            <Typography variant="body2">Loading logs...</Typography>
          ) : filteredLogs.length === 0 ? (
            <Typography variant="body2" color="text.secondary">
              No logs available.
            </Typography>
          ) : (
            <Box
              ref={logContainerRef}
              sx={{ maxHeight: 320, overflow: 'auto' }}
            >
              <List dense>
                {filteredLogs.map((log) => (
                  <ListItem key={log.id} divider>
                    <ListItemText
                      primary={log.message}
                      secondary={`${log.level} • ${new Date(log.created_at).toLocaleString()}`}
                    />
                  </ListItem>
                ))}
              </List>
            </Box>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setLogDialogOpen(false)}>Close</Button>
        </DialogActions>
      </Dialog>

      <Dialog
        open={errorDialogOpen}
        onClose={() => setErrorDialogOpen(false)}
      >
        <DialogTitle>Operation Error Details</DialogTitle>
        <DialogContent>
          <Typography variant="body2">{errorDetails}</Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setErrorDialogOpen(false)}>Close</Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
