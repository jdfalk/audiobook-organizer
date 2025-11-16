// file: web/src/pages/Logs.tsx
// version: 1.0.0
// guid: 6b7c8d9e-0f1a-2b3c-4d5e-6f7a8b9c0d1e

import { useState, useEffect } from 'react';
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
} from '@mui/material';
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

export function Logs() {
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(25);
  const [levelFilter, setLevelFilter] = useState<string>('all');
  const [sourceFilter, setSourceFilter] = useState<string>('all');
  const [searchQuery, setSearchQuery] = useState('');
  const [expandedRow, setExpandedRow] = useState<string | null>(null);
  const [autoRefresh, setAutoRefresh] = useState(false);

  useEffect(() => {
    fetchLogs();
  }, [levelFilter, sourceFilter, page, rowsPerPage]);

  useEffect(() => {
    if (!autoRefresh) return;

    const interval = setInterval(() => {
      fetchLogs();
    }, 5000);

    return () => clearInterval(interval);
  }, [autoRefresh, levelFilter, sourceFilter]);

  const fetchLogs = async () => {
    try {
      // TODO: Replace with actual API call
      // const response = await fetch(`/api/v1/logs?level=${levelFilter}&source=${sourceFilter}&page=${page}&limit=${rowsPerPage}`);
      // const data = await response.json();

      // Mock data for demonstration
      const mockLogs: LogEntry[] = Array.from({ length: 50 }, (_, i) => ({
        id: `log-${i}`,
        timestamp: new Date(Date.now() - i * 60000).toISOString(),
        level: ['debug', 'info', 'warn', 'error'][Math.floor(Math.random() * 4)] as LogEntry['level'],
        message: [
          'Scanning library folder: /audiobooks/import',
          'Successfully imported audiobook: To Kill a Mockingbird',
          'Failed to fetch metadata from Goodreads',
          'Database connection established',
          'File organization completed',
          'Memory usage: 45%',
          'Disk quota check: 67% used',
        ][Math.floor(Math.random() * 7)],
        source: ['scanner', 'importer', 'metadata', 'database', 'organizer'][Math.floor(Math.random() * 5)],
        metadata: {
          duration: Math.floor(Math.random() * 1000),
          files_processed: Math.floor(Math.random() * 100),
        },
      }));

      setLogs(mockLogs);
    } catch (error) {
      console.error('Failed to fetch logs:', error);
    } finally {
      setLoading(false);
    }
  };

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

  const getLevelColor = (level: string): 'error' | 'warning' | 'info' | 'default' => {
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
    if (searchQuery && !log.message.toLowerCase().includes(searchQuery.toLowerCase())) return false;
    return true;
  });

  const handleChangePage = (_: unknown, newPage: number) => {
    setPage(newPage);
  };

  const handleChangeRowsPerPage = (event: React.ChangeEvent<HTMLInputElement>) => {
    setRowsPerPage(parseInt(event.target.value, 10));
    setPage(0);
  };

  return (
    <Box>
      <Stack direction="row" justifyContent="space-between" alignItems="center" mb={3}>
        <Typography variant="h4">Logs & Events</Typography>
        <Stack direction="row" spacing={2}>
          <Button
            variant={autoRefresh ? 'contained' : 'outlined'}
            onClick={() => setAutoRefresh(!autoRefresh)}
          >
            Auto Refresh {autoRefresh && '(5s)'}
          </Button>
          <Button variant="outlined" startIcon={<RefreshIcon />} onClick={fetchLogs}>
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
                        onClick={() => setExpandedRow(expandedRow === log.id ? null : log.id)}
                      >
                        <ExpandMoreIcon
                          sx={{
                            transform: expandedRow === log.id ? 'rotate(180deg)' : 'rotate(0deg)',
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
                      <Chip label={log.source || 'system'} size="small" variant="outlined" />
                    </TableCell>
                    <TableCell>{log.message}</TableCell>
                  </TableRow>
                  {expandedRow === log.id && log.metadata && (
                    <TableRow>
                      <TableCell colSpan={5} sx={{ bgcolor: 'background.default' }}>
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
