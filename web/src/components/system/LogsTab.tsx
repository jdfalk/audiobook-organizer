// file: web/src/components/system/LogsTab.tsx
// version: 1.0.1
// guid: 8d9e0f1a-2b3c-4d5e-6f7a-8b9c0d1e2f3a

import { useState, useEffect, useCallback } from 'react';
import {
  Box,
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
  Paper,
  Typography,
} from '@mui/material';
import * as api from '../../services/api';
import {
  Refresh as RefreshIcon,
  ExpandMore as ExpandMoreIcon,
  Error as ErrorIcon,
  Warning as WarningIcon,
  Info as InfoIcon,
  BugReport as DebugIcon,
} from '@mui/icons-material';

interface LogEntry {
  id: string;
  timestamp: string;
  level: 'debug' | 'info' | 'warn' | 'error';
  message: string;
  source?: string;
  metadata?: Record<string, string | number | boolean>;
}

export function LogsTab() {
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(25);
  const [levelFilter, setLevelFilter] = useState<string>('all');
  const [sourceFilter, setSourceFilter] = useState<string>('all');
  const [searchQuery, setSearchQuery] = useState('');
  const [expandedRow, setExpandedRow] = useState<string | null>(null);
  const [autoRefresh, setAutoRefresh] = useState(false);

  const fetchLogs = useCallback(async () => {
    setLoading(true);
    try {
      const response = await api.getSystemLogs({
        level: levelFilter !== 'all' ? levelFilter : undefined,
        search: searchQuery || undefined,
        limit: rowsPerPage,
        offset: page * rowsPerPage,
      });

      const convertedLogs: LogEntry[] = response.logs.map((log, i) => ({
        id: `${log.operation_id}-${i}`,
        timestamp: log.timestamp,
        level: log.level as LogEntry['level'],
        message: log.message,
        source: log.operation_id,
        metadata: log.details ? { details: log.details } : undefined,
      }));

      setLogs(convertedLogs);
    } catch (error) {
      console.error('Failed to fetch logs:', error);
      setLogs([]);
    } finally {
      setLoading(false);
    }
  }, [levelFilter, page, rowsPerPage, searchQuery, sourceFilter]);

  useEffect(() => {
    fetchLogs();
  }, [fetchLogs]);

  useEffect(() => {
    if (!autoRefresh) return;

    const interval = setInterval(() => {
      fetchLogs();
    }, 5000);

    return () => clearInterval(interval);
  }, [autoRefresh, fetchLogs]);

  const getLevelIcon = (level: string) => {
    switch (level) {
      case 'error':
        return <ErrorIcon color="error" />;
      case 'warn':
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
    if (sourceFilter !== 'all' && log.source !== sourceFilter) return false;
    if (
      searchQuery &&
      !log.message.toLowerCase().includes(searchQuery.toLowerCase())
    )
      return false;
    return true;
  });

  const handleChangePage = (_: unknown, newPage: number) => {
    setPage(newPage);
  };

  const handleChangeRowsPerPage = (
    event: React.ChangeEvent<HTMLInputElement>
  ) => {
    setRowsPerPage(parseInt(event.target.value, 10));
    setPage(0);
  };

  return (
    <Box>
      <Stack
        direction="row"
        justifyContent="space-between"
        alignItems="center"
        mb={2}
      >
        <Typography variant="h6">Logs & Events</Typography>
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
          <TextField
            select
            size="small"
            label="Source"
            value={sourceFilter}
            onChange={(e) => setSourceFilter(e.target.value)}
            sx={{ minWidth: 150 }}
          >
            <MenuItem value="all">All Sources</MenuItem>
            <MenuItem value="scanner">Scanner</MenuItem>
            <MenuItem value="importer">Importer</MenuItem>
            <MenuItem value="metadata">Metadata</MenuItem>
            <MenuItem value="database">Database</MenuItem>
            <MenuItem value="organizer">Organizer</MenuItem>
          </TextField>
        </Stack>
      </Paper>

      <TableContainer component={Paper}>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell width={50}></TableCell>
              <TableCell width={80}>Level</TableCell>
              <TableCell width={180}>Timestamp</TableCell>
              <TableCell width={120}>Source</TableCell>
              <TableCell>Message</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {filteredLogs
              .slice(page * rowsPerPage, page * rowsPerPage + rowsPerPage)
              .map((log) => (
                <>
                  <TableRow key={log.id} hover>
                    <TableCell>
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
                        label={log.source || 'system'}
                        size="small"
                        variant="outlined"
                      />
                    </TableCell>
                    <TableCell>{log.message}</TableCell>
                  </TableRow>
                  {expandedRow === log.id && log.metadata && (
                    <TableRow>
                      <TableCell
                        colSpan={5}
                        sx={{ bgcolor: 'background.default' }}
                      >
                        <Collapse in={expandedRow === log.id}>
                          <Box sx={{ p: 2 }}>
                            <Typography variant="subtitle2" gutterBottom>
                              Metadata:
                            </Typography>
                            <pre style={{ margin: 0, fontSize: '0.875rem' }}>
                              {JSON.stringify(log.metadata, null, 2)}
                            </pre>
                          </Box>
                        </Collapse>
                      </TableCell>
                    </TableRow>
                  )}
                </>
              ))}
          </TableBody>
        </Table>
        <TablePagination
          rowsPerPageOptions={[10, 25, 50, 100]}
          component="div"
          count={filteredLogs.length}
          rowsPerPage={rowsPerPage}
          page={page}
          onPageChange={handleChangePage}
          onRowsPerPageChange={handleChangeRowsPerPage}
        />
      </TableContainer>
    </Box>
  );
}
