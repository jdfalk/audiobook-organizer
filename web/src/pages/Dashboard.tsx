// file: web/src/pages/Dashboard.tsx
// version: 1.8.0
// guid: 2f3a4b5c-6d7e-8f9a-0b1c-2d3e4f5a6b7c

import { useState, useEffect, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Box,
  Typography,
  Grid,
  Paper,
  LinearProgress,
  CircularProgress,
  Button,
  Stack,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Alert,
  Card,
  CardContent,
  List,
  ListItem,
  ListItemText,
  Chip,
  CardActionArea,
  Skeleton,
} from '@mui/material';
import {
  LibraryBooks as LibraryBooksIcon,
  Folder as FolderIcon,
  CheckCircle as CheckCircleIcon,
  Error as ErrorIcon,
  Storage as StorageIcon,
  Person as PersonIcon,
  MenuBook as MenuBookIcon,
} from '@mui/icons-material';
import * as api from '../services/api';

interface SystemStats {
  library_books: number;
  import_books: number;
  total_books: number;
  total_authors: number;
  total_series: number;
  import_paths: number;
  library_size_gb: number;
  import_size_gb: number;
  total_size_gb: number;
  disk_used_gb: number;
  disk_total_gb: number;
  disk_usage_percent: number;
}

interface RecentOperation {
  id: string;
  type: string;
  status: 'success' | 'error' | 'running';
  message: string;
  timestamp: string;
}

export function Dashboard() {
  const navigate = useNavigate();
  const [stats, setStats] = useState<SystemStats | null>(null);
  const [authorCount, setAuthorCount] = useState<number | null>(null);
  const [seriesCount, setSeriesCount] = useState<number | null>(null);
  const [operations, setOperations] = useState<RecentOperation[] | null>(null);
  const [storageLoaded, setStorageLoaded] = useState(false);
  const [actionNotice, setActionNotice] = useState<string | null>(null);
  const [organizeDialogOpen, setOrganizeDialogOpen] = useState(false);
  const [organizeInProgress, setOrganizeInProgress] = useState(false);
  const [scanInProgress, setScanInProgress] = useState(false);

  const loadStats = useCallback(async () => {
    try {
      const systemStatus = await api.getSystemStatus();

      const libraryBooks =
        systemStatus.library_book_count ?? systemStatus.library.book_count ?? 0;
      const importBooks =
        systemStatus.import_book_count ??
        systemStatus.import_paths?.book_count ??
        0;
      const totalBooks =
        systemStatus.total_book_count ?? libraryBooks + importBooks;
      const librarySizeBytes =
        systemStatus.library_size_bytes ?? systemStatus.library.total_size ?? 0;
      const importSizeBytes =
        systemStatus.import_size_bytes ??
        systemStatus.import_paths?.total_size ??
        0;
      const totalSizeBytes =
        systemStatus.total_size_bytes ?? librarySizeBytes + importSizeBytes;
      const diskTotalBytes =
        systemStatus.disk_total_bytes ?? totalSizeBytes;
      const diskUsedBytes =
        systemStatus.disk_used_bytes ?? librarySizeBytes + importSizeBytes;
      const diskUsagePercent =
        diskTotalBytes > 0 ? (diskUsedBytes / diskTotalBytes) * 100 : 0;

      setStats({
        library_books: libraryBooks,
        import_books: importBooks,
        total_books: totalBooks,
        total_authors: systemStatus.author_count ?? 0,
        total_series: systemStatus.series_count ?? 0,
        import_paths: systemStatus.import_paths?.folder_count || 0,
        library_size_gb: librarySizeBytes / (1024 * 1024 * 1024),
        import_size_gb: importSizeBytes / (1024 * 1024 * 1024),
        total_size_gb: totalSizeBytes / (1024 * 1024 * 1024),
        disk_used_gb: diskUsedBytes / (1024 * 1024 * 1024),
        disk_total_gb: diskTotalBytes / (1024 * 1024 * 1024),
        disk_usage_percent: diskUsagePercent,
      });
      setStorageLoaded(true);

      // Convert recent operations
      const recentOps = (systemStatus.operations?.recent || [])
        .slice(0, 5)
        .map((op) => ({
          id: op.id,
          type: op.type,
          status: (op.status === 'completed'
            ? 'success'
            : op.status === 'failed'
              ? 'error'
              : 'running') as 'success' | 'error' | 'running',
          message: op.message || `${op.type} operation`,
          timestamp: op.created_at,
        }));
      setOperations(recentOps);
    } catch (error) {
      console.error('Failed to load system status:', error);
      // Set empty defaults so spinners stop
      setStats({
        library_books: 0, import_books: 0, total_books: 0,
        total_authors: 0, total_series: 0, import_paths: 0,
        library_size_gb: 0, import_size_gb: 0, total_size_gb: 0,
        disk_used_gb: 0, disk_total_gb: 0, disk_usage_percent: 0,
      });
      setStorageLoaded(true);
      setOperations([]);
    }
  }, []);

  const loadAuthors = useCallback(async () => {
    try {
      const count = await api.countAuthors();
      setAuthorCount(count);
    } catch {
      setAuthorCount(0);
    }
  }, []);

  const loadSeries = useCallback(async () => {
    try {
      const count = await api.countSeries();
      setSeriesCount(count);
    } catch {
      setSeriesCount(0);
    }
  }, []);

  // Fire all requests in parallel — each section updates independently
  useEffect(() => {
    loadStats();
    loadAuthors();
    loadSeries();
  }, [loadStats, loadAuthors, loadSeries]);

  // Auto-refresh every 15s while a scan is active
  useEffect(() => {
    const hasActiveScan = operations?.some((op) => op.status === 'running');
    if (!hasActiveScan) return;
    const interval = setInterval(() => {
      loadStats();
      loadAuthors();
      loadSeries();
    }, 15000);
    return () => clearInterval(interval);
  }, [operations, loadStats, loadAuthors, loadSeries]);

  const handleScanAll = async () => {
    setScanInProgress(true);
    setActionNotice(null);
    try {
      await api.startScan();
      setActionNotice('Scan started for all import paths.');
      navigate('/operations');
    } catch (error) {
      console.error('Failed to start scan', error);
      setActionNotice('Failed to start scan.');
    } finally {
      setScanInProgress(false);
    }
  };

  const handleOrganizeAll = () => {
    setOrganizeDialogOpen(true);
  };

  const handleConfirmOrganizeAll = async () => {
    setOrganizeInProgress(true);
    setActionNotice(null);
    try {
      await api.startOrganize();
      setActionNotice('Organize operation started.');
      setOrganizeDialogOpen(false);
      navigate('/operations');
    } catch (error) {
      console.error('Failed to start organize', error);
      setActionNotice('Failed to start organize.');
    } finally {
      setOrganizeInProgress(false);
    }
  };

  const StatCard = ({
    title,
    value,
    loading,
    icon,
    suffix = '',
    onClick,
  }: {
    title: string;
    value: number;
    loading: boolean;
    icon: React.ReactNode;
    suffix?: string;
    onClick?: () => void;
  }) => (
    <Card>
      <CardActionArea onClick={onClick} disabled={!onClick}>
        <CardContent>
          <Box
            display="flex"
            alignItems="center"
            justifyContent="space-between"
          >
            <Box>
              <Typography color="text.secondary" gutterBottom>
                {title}
              </Typography>
              {loading ? (
                <Skeleton variant="text" width={80} height={42} />
              ) : (
                <Typography variant="h4">
                  {value.toLocaleString()}
                  {suffix}
                </Typography>
              )}
            </Box>
            <Box sx={{ color: 'primary.main' }}>{icon}</Box>
          </Box>
        </CardContent>
      </CardActionArea>
    </Card>
  );

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'success':
        return <CheckCircleIcon color="success" />;
      case 'error':
        return <ErrorIcon color="error" />;
      default:
        return <CheckCircleIcon color="action" />;
    }
  };

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'success':
        return 'success';
      case 'error':
        return 'error';
      default:
        return 'default';
    }
  };

  // Derive loading states per component
  const bookStatsLoading = stats === null;
  const authorsLoading = authorCount === null && (stats === null || !stats.total_authors);
  const seriesLoading = seriesCount === null && (stats === null || !stats.total_series);

  return (
    <Box sx={{ height: '100%', overflow: 'auto' }}>
      <Typography variant="h4" gutterBottom>
        Dashboard
      </Typography>

      {actionNotice && (
        <Alert severity="info" sx={{ mb: 2 }}>
          {actionNotice}
        </Alert>
      )}

      <Grid container spacing={3}>
        <Grid item xs={12} sm={6} md={3}>
          <StatCard
            title="Library Books"
            value={stats?.library_books ?? 0}
            loading={bookStatsLoading}
            icon={<LibraryBooksIcon sx={{ fontSize: 40 }} />}
            onClick={() => navigate('/library')}
          />
        </Grid>

        <Grid item xs={12} sm={6} md={3}>
          <StatCard
            title="Import Path Books"
            value={stats?.import_books ?? 0}
            loading={bookStatsLoading}
            icon={<FolderIcon sx={{ fontSize: 40 }} />}
            onClick={() => navigate('/library')}
          />
        </Grid>

        <Grid item xs={12} sm={6} md={3}>
          <StatCard
            title="Authors"
            value={authorCount ?? stats?.total_authors ?? 0}
            loading={authorsLoading}
            icon={<PersonIcon sx={{ fontSize: 40 }} />}
            onClick={() => navigate('/library')}
          />
        </Grid>

        <Grid item xs={12} sm={6} md={3}>
          <StatCard
            title="Series"
            value={seriesCount ?? stats?.total_series ?? 0}
            loading={seriesLoading}
            icon={<MenuBookIcon sx={{ fontSize: 40 }} />}
            onClick={() => navigate('/library')}
          />
        </Grid>

        <Grid item xs={12} md={6}>
          <Paper sx={{ p: 3 }}>
            <Typography variant="h6" gutterBottom>
              Storage Usage
            </Typography>
            {!storageLoaded ? (
              <Box sx={{ py: 2 }}>
                <Skeleton variant="text" width="60%" height={24} />
                <Skeleton variant="rectangular" height={8} sx={{ my: 1, borderRadius: 1 }} />
                <Skeleton variant="text" width="40%" height={20} />
              </Box>
            ) : (
              <>
                <Box sx={{ mb: 2 }}>
                  <Box display="flex" justifyContent="space-between" mb={1}>
                    <Typography variant="body2" color="text.secondary">
                      Total Size
                    </Typography>
                    <Typography variant="body2" fontWeight="medium">
                      {(stats?.disk_used_gb ?? 0).toFixed(1)} GB /{' '}
                      {(stats?.disk_total_gb ?? 0).toFixed(1)} GB
                    </Typography>
                  </Box>
                  <LinearProgress
                    variant="determinate"
                    value={stats?.disk_usage_percent ?? 0}
                    sx={{ height: 8, borderRadius: 1 }}
                  />
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{ mt: 0.5, display: 'block' }}
                  >
                    {(stats?.disk_usage_percent ?? 0).toFixed(0)}% of disk used
                  </Typography>
                </Box>
                <Box display="flex" alignItems="center" gap={1}>
                  <StorageIcon color="action" />
                  <Typography variant="body2" color="text.secondary">
                    System storage healthy
                  </Typography>
                </Box>
              </>
            )}
          </Paper>
        </Grid>

        <Grid item xs={12} md={6}>
          <Paper sx={{ p: 3 }}>
            <Typography variant="h6" gutterBottom>
              Recent Operations
            </Typography>
            {operations === null ? (
              <Box sx={{ py: 1 }}>
                <Skeleton variant="text" width="80%" />
                <Skeleton variant="text" width="60%" />
                <Skeleton variant="text" width="70%" />
              </Box>
            ) : operations.length === 0 ? (
              <Typography variant="body2" color="text.secondary">
                No recent operations
              </Typography>
            ) : (
              <List dense>
                {operations.map((op) => (
                  <ListItem key={op.id} sx={{ px: 0 }}>
                    <Box display="flex" alignItems="center" width="100%">
                      {getStatusIcon(op.status)}
                      <ListItemText
                        primary={op.message}
                        secondary={new Date(op.timestamp).toLocaleTimeString()}
                        sx={{ ml: 1 }}
                      />
                      <Chip
                        label={op.type}
                        size="small"
                        color={
                          getStatusColor(op.status) as
                            | 'success'
                            | 'error'
                            | 'default'
                        }
                        variant="outlined"
                      />
                    </Box>
                  </ListItem>
                ))}
              </List>
            )}
          </Paper>
        </Grid>

        <Grid item xs={12}>
          <Paper sx={{ p: 3 }}>
            <Typography variant="h6" gutterBottom>
              Quick Actions
            </Typography>
            <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2}>
              <Button
                variant="contained"
                onClick={handleScanAll}
                disabled={scanInProgress}
                startIcon={
                  scanInProgress ? <CircularProgress size={20} /> : undefined
                }
              >
                {scanInProgress ? 'Starting Scan...' : 'Scan All Import Paths'}
              </Button>
              <Button variant="outlined" onClick={handleOrganizeAll}>
                Organize All
              </Button>
            </Stack>
          </Paper>
        </Grid>
      </Grid>

      <Dialog
        open={organizeDialogOpen}
        onClose={() => setOrganizeDialogOpen(false)}
      >
        <DialogTitle>Organize All Scanned Books</DialogTitle>
        <DialogContent>
          <Typography variant="body2" color="text.secondary">
            This will organize all books currently scanned but not yet imported to the library.
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button
            onClick={() => setOrganizeDialogOpen(false)}
            disabled={organizeInProgress}
          >
            Cancel
          </Button>
          <Button
            variant="contained"
            onClick={handleConfirmOrganizeAll}
            disabled={organizeInProgress}
            startIcon={
              organizeInProgress ? <CircularProgress size={20} /> : undefined
            }
          >
            {organizeInProgress ? 'Organizing...' : 'Organize'}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
