// file: web/src/pages/Logs.tsx
// version: 2.0.0
// guid: 6b7c8d9e-0f1a-2b3c-4d5e-6f7a8b9c0d1e

import { useState, useEffect, useCallback } from 'react';
import {
  Box,
  Typography,
  Paper,
  TextField,
  MenuItem,
  Button,
  Stack,
  Chip,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TablePagination,
  IconButton,
  Collapse,
  Alert,
} from '@mui/material';
import {
  Refresh as RefreshIcon,
  ExpandMore as ExpandMoreIcon,
  Error as ErrorIcon,
  Warning as WarningIcon,
  Info as InfoIcon,
  BugReport as DebugIcon,
} from '@mui/icons-material';
import * as api from '../services/api';

interface LogRow {
  id: string;
  timestamp: string;
  level: string;
  message: string;
  source: string;
  details?: string;
}

export function Logs() {
  const [logs, setLogs] = useState<LogRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(25);
  const [levelFilter, setLevelFilter] = useState<string>('all');
  const [searchQuery, setSearchQuery] = useState('');
  const [expandedRow, setExpandedRow] = useState<string | null>(null);
  const [autoRefresh, setAutoRefresh] = useState(false);

  const fetchLogs = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const operations = await api.getActiveOperations();
      // Get logs from each operation
      const logPromises = operations.map(async (op) => {
        try {
          const opLogs = await api.getOperationLogs(op.id);
          return opLogs.map((log) => ({
            id: `${op.id}-${log.id}`,
            timestamp: log.created_at,
            level: log.level,
            message: log.message,
            source: `${op.type} (${op.id.slice(0, 8)})`,
            details: log.details,
          }));
        } catch {
          return [];
        }
      });
      const allLogs = (await Promise.all(logPromises)).flat();
      // Sort by timestamp descending
      allLogs.sort((a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime());
      setLogs(allLogs);
      if (allLogs.length === 0 && operations.length === 0) {
        setError('No active operations. Logs appear here during scans, imports, and other operations.');
      }
    } catch (err) {
      console.error('Failed to fetch logs:', err);
      setError('Failed to fetch operation logs.');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchLogs();
  }, [fetchLogs]);

  useEffect(() => {
    if (!autoRefresh) return;
    const interval = setInterval(fetchLogs, 5000);
    return () => clearInterval(interval);
  }, [autoRefresh, fetchLogs]);

  const getLevelIcon = (level: string) => {
    switch (level) {
      case 'error':
        return <ErrorIcon color="error" />;
      case 'warn':
      case 'warning':
        return <WarningIcon color="warning" />;
      case 'info':
        return <InfoIcon color="info" />;
      case 'debug':
      default:
        return <DebugIcon color="action" />;
    }
  };

  const getLevelColor = (
    level: string
  ): 'error' | 'warning' | 'info' | 'default' => {
    switch (level) {
      case 'error':
        return 'error';
      case 'warn':
      case 'warning':
        return 'warning';
      case 'info':
        return 'info';
      case 'debug':
      default:
        return 'default';
    }
  };

  const filteredLogs = logs.filter((log) => {
    if (levelFilter !== 'all' && log.level !== levelFilter) return false;
    if (
      searchQuery &&
      !log.message.toLowerCase().includes(searchQuery.toLowerCase())
    )
      return false;
    return true;
  });

  return (
    <Box>
      <Stack
        direction="row"
        justifyContent="space-between"
        alignItems="center"
        mb={3}
      >
        <Typography variant="h4">Operation Logs</Typography>
        <Stack direction="row" spacing={2}>
          <Button
            variant={autoRefresh ? 'contained' : 'outlined'}
            onClick={() => setAutoRefresh(!autoRefresh)}
          >
            Auto Refresh {autoRefresh && '(5s)'}
          </Button>
          <Button
            variant="outlined"
            startIcon={<RefreshIcon />}
            onClick={fetchLogs}
            disabled={loading}
          >
            Refresh
          </Button>
        </Stack>
      </Stack>

      <Paper sx={{ p: 2, mb: 2 }}>
        <Stack direction="row" spacing={2} alignItems="center">
          <TextField
            size="small"
            placeholder="Search logs..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            sx={{ flex: 1 }}
          />
          <TextField
            select
            size="small"
            label="Level"
            value={levelFilter}
            onChange={(e) => setLevelFilter(e.target.value)}
            sx={{ minWidth: 120 }}
          >
            <MenuItem value="all">All Levels</MenuItem>
            <MenuItem value="debug">Debug</MenuItem>
            <MenuItem value="info">Info</MenuItem>
            <MenuItem value="warn">Warning</MenuItem>
            <MenuItem value="error">Error</MenuItem>
          </TextField>
        </Stack>
      </Paper>

      {error && (
        <Alert severity="info" sx={{ mb: 2 }}>
          {error}
        </Alert>
      )}

      <TableContainer component={Paper}>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell width={50}></TableCell>
              <TableCell width={80}>Level</TableCell>
              <TableCell width={180}>Timestamp</TableCell>
              <TableCell width={180}>Operation</TableCell>
              <TableCell>Message</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {filteredLogs
              .slice(page * rowsPerPage, page * rowsPerPage + rowsPerPage)
              .map((log) => (
                <Box component="tbody" key={log.id}>
                  <TableRow hover>
                    <TableCell>
                      {log.details && (
                        <IconButton
                          size="small"
                          onClick={() =>
                            setExpandedRow(expandedRow === log.id ? null : log.id)
                          }
                        >
                          <ExpandMoreIcon
                            sx={{
                              transform:
                                expandedRow === log.id
                                  ? 'rotate(180deg)'
                                  : 'rotate(0deg)',
                              transition: 'transform 0.3s',
                            }}
                          />
                        </IconButton>
                      )}
                    </TableCell>
                    <TableCell>
                      <Chip
                        icon={getLevelIcon(log.level)}
                        label={log.level.toUpperCase()}
                        size="small"
                        color={getLevelColor(log.level)}
                      />
                    </TableCell>
                    <TableCell>
                      <Typography variant="body2" color="text.secondary">
                        {new Date(log.timestamp).toLocaleString()}
                      </Typography>
                    </TableCell>
                    <TableCell>
                      <Chip
                        label={log.source}
                        size="small"
                        variant="outlined"
                      />
                    </TableCell>
                    <TableCell>{log.message}</TableCell>
                  </TableRow>
                  {expandedRow === log.id && log.details && (
                    <TableRow>
                      <TableCell
                        colSpan={5}
                        sx={{ bgcolor: 'background.default' }}
                      >
                        <Collapse in={expandedRow === log.id}>
                          <Box sx={{ p: 2 }}>
                            <Typography variant="subtitle2" gutterBottom>
                              Details:
                            </Typography>
                            <pre style={{ margin: 0, fontSize: '0.875rem', whiteSpace: 'pre-wrap' }}>
                              {log.details}
                            </pre>
                          </Box>
                        </Collapse>
                      </TableCell>
                    </TableRow>
                  )}
                </Box>
              ))}
            {filteredLogs.length === 0 && !loading && (
              <TableRow>
                <TableCell colSpan={5} align="center">
                  <Typography color="text.secondary" sx={{ py: 4 }}>
                    No logs to display.
                  </Typography>
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
        <TablePagination
          rowsPerPageOptions={[10, 25, 50, 100]}
          component="div"
          count={filteredLogs.length}
          rowsPerPage={rowsPerPage}
          page={page}
          onPageChange={(_, newPage) => setPage(newPage)}
          onRowsPerPageChange={(e) => {
            setRowsPerPage(parseInt(e.target.value, 10));
            setPage(0);
          }}
        />
      </TableContainer>
    </Box>
  );
}
