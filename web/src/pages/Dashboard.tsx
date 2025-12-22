// file: web/src/pages/Dashboard.tsx
// version: 1.5.0
// guid: 2f3a4b5c-6d7e-8f9a-0b1c-2d3e4f5a6b7c

import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Box,
  Typography,
  Grid,
  Paper,
  LinearProgress,
  Card,
  CardContent,
  List,
  ListItem,
  ListItemText,
  Chip,
  CardActionArea,
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
  const [stats, setStats] = useState<SystemStats>({
    library_books: 0,
    import_books: 0,
    total_books: 0,
    total_authors: 0,
    total_series: 0,
    import_paths: 0,
    library_size_gb: 0,
    import_size_gb: 0,
    total_size_gb: 0,
    disk_usage_percent: 0,
  });
  const [operations, setOperations] = useState<RecentOperation[]>([]);

  useEffect(() => {
    loadDashboardData();
  }, []);

  const loadDashboardData = async () => {
    try {
      console.log('[Dashboard] Loading dashboard data...');
      // Fetch real data from API
      const [systemStatus, bookCount, authors, seriesList] = await Promise.all([
        api.getSystemStatus(),
        api.countBooks(),
        api.getAuthors(),
        api.getSeries(),
      ]);

      console.log('[Dashboard] System status:', systemStatus);
      console.log('[Dashboard] Import path_count:', systemStatus.library.folder_count);
      console.log('[Dashboard] Book count:', bookCount);

      const libraryBooks = systemStatus.library_book_count ?? systemStatus.library.book_count ?? 0;
      const importBooks =
        systemStatus.import_book_count ?? systemStatus.import_paths?.book_count ?? 0;
      const totalBooks = systemStatus.total_book_count ?? libraryBooks + importBooks;
      const librarySizeBytes =
        systemStatus.library_size_bytes ?? systemStatus.library.total_size ?? 0;
      const importSizeBytes =
        systemStatus.import_size_bytes ?? systemStatus.import_paths?.total_size ?? 0;
      const totalSizeBytes = systemStatus.total_size_bytes ?? librarySizeBytes + importSizeBytes;

      setStats({
        library_books: libraryBooks,
        import_books: importBooks,
        total_books: totalBooks,
        total_authors: authors.length,
        total_series: seriesList.length,
        import_paths: systemStatus.import_paths?.folder_count || 0,
        library_size_gb: librarySizeBytes / (1024 * 1024 * 1024),
        import_size_gb: importSizeBytes / (1024 * 1024 * 1024),
        total_size_gb: totalSizeBytes / (1024 * 1024 * 1024),
        disk_usage_percent: 0, // Calculate if needed
      });

      // Convert recent operations to dashboard format
      const recentOps = systemStatus.operations.recent.slice(0, 5).map((op) => ({
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
      console.error('Failed to load dashboard data:', error);
      // Keep default/empty state on error
    }
  };

  const StatCard = ({
    title,
    value,
    icon,
    suffix = '',
    onClick,
  }: {
    title: string;
    value: number;
    icon: React.ReactNode;
    suffix?: string;
    onClick?: () => void;
  }) => (
    <Card>
      <CardActionArea onClick={onClick} disabled={!onClick}>
        <CardContent>
          <Box display="flex" alignItems="center" justifyContent="space-between">
            <Box>
              <Typography color="text.secondary" gutterBottom>
                {title}
              </Typography>
              <Typography variant="h4">
                {value.toLocaleString()}
                {suffix}
              </Typography>
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

  return (
    <Box sx={{ height: '100%', overflow: 'auto' }}>
      <Typography variant="h4" gutterBottom>
        Dashboard
      </Typography>

      <Grid container spacing={3}>
        <Grid item xs={12} sm={6} md={3}>
          <StatCard
            title="Library Books"
            value={stats.library_books}
            icon={<LibraryBooksIcon sx={{ fontSize: 40 }} />}
            onClick={() => navigate('/library')}
          />
        </Grid>

        <Grid item xs={12} sm={6} md={3}>
          <StatCard
            title="Import Path Books"
            value={stats.import_books}
            icon={<FolderIcon sx={{ fontSize: 40 }} />}
            onClick={() => navigate('/library')}
          />
        </Grid>

        <Grid item xs={12} sm={6} md={3}>
          <StatCard
            title="Authors"
            value={stats.total_authors}
            icon={<PersonIcon sx={{ fontSize: 40 }} />}
            onClick={() => navigate('/library')}
          />
        </Grid>

        <Grid item xs={12} sm={6} md={3}>
          <StatCard
            title="Series"
            value={stats.total_series}
            icon={<MenuBookIcon sx={{ fontSize: 40 }} />}
            onClick={() => navigate('/library')}
          />
        </Grid>

        <Grid item xs={12} md={6}>
          <Paper sx={{ p: 3 }}>
            <Typography variant="h6" gutterBottom>
              Storage Usage
            </Typography>
            <Box sx={{ mb: 2 }}>
              <Box display="flex" justifyContent="space-between" mb={1}>
                <Typography variant="body2" color="text.secondary">
                  Total Size
                </Typography>
                <Typography variant="body2" fontWeight="medium">
                  {stats.total_size_gb.toFixed(1)} GB
                </Typography>
              </Box>
              <LinearProgress
                variant="determinate"
                value={stats.disk_usage_percent}
                sx={{ height: 8, borderRadius: 1 }}
              />
              <Typography
                variant="caption"
                color="text.secondary"
                sx={{ mt: 0.5, display: 'block' }}
              >
                {stats.disk_usage_percent}% of disk used
              </Typography>
            </Box>
            <Box display="flex" alignItems="center" gap={1}>
              <StorageIcon color="action" />
              <Typography variant="body2" color="text.secondary">
                System storage healthy
              </Typography>
            </Box>
          </Paper>
        </Grid>

        <Grid item xs={12} md={6}>
          <Paper sx={{ p: 3 }}>
            <Typography variant="h6" gutterBottom>
              Recent Operations
            </Typography>
            {operations.length === 0 ? (
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
                        color={getStatusColor(op.status) as 'success' | 'error' | 'default'}
                        variant="outlined"
                      />
                    </Box>
                  </ListItem>
                ))}
              </List>
            )}
          </Paper>
        </Grid>
      </Grid>
    </Box>
  );
}
