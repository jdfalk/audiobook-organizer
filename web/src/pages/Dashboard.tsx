// file: web/src/pages/Dashboard.tsx
// version: 1.2.0
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

interface SystemStats {
  total_books: number;
  total_authors: number;
  total_series: number;
  library_folders: number;
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
    total_books: 0,
    total_authors: 0,
    total_series: 0,
    library_folders: 0,
    total_size_gb: 0,
    disk_usage_percent: 0,
  });
  const [operations, setOperations] = useState<RecentOperation[]>([]);

  useEffect(() => {
    loadDashboardData();
  }, []);

  const loadDashboardData = async () => {
    try {
      // TODO: Replace with actual API calls
      // const statsResponse = await fetch('/api/v1/stats');
      // const opsResponse = await fetch('/api/v1/operations/recent');
      // setStats(await statsResponse.json());
      // setOperations(await opsResponse.json());

      // Placeholder data
      setStats({
        total_books: 1247,
        total_authors: 342,
        total_series: 89,
        library_folders: 3,
        total_size_gb: 156.4,
        disk_usage_percent: 45,
      });

      setOperations([
        {
          id: '1',
          type: 'Scan',
          status: 'success',
          message: 'Scanned /audiobooks/fiction',
          timestamp: new Date().toISOString(),
        },
        {
          id: '2',
          type: 'Metadata Update',
          status: 'success',
          message: 'Updated 12 audiobooks',
          timestamp: new Date(Date.now() - 3600000).toISOString(),
        },
      ]);
    } catch (error) {
      console.error('Failed to load dashboard data:', error);
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
    <Box>
      <Typography variant="h4" gutterBottom>
        Dashboard
      </Typography>

      <Grid container spacing={3}>
        <Grid item xs={12} sm={6} md={3}>
          <StatCard
            title="Total Audiobooks"
            value={stats.total_books}
            icon={<LibraryBooksIcon sx={{ fontSize: 40 }} />}
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

        <Grid item xs={12} sm={6} md={3}>
          <StatCard
            title="Library Folders"
            value={stats.library_folders}
            icon={<FolderIcon sx={{ fontSize: 40 }} />}
            onClick={() => navigate('/file-manager')}
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
              <Typography variant="caption" color="text.secondary" sx={{ mt: 0.5, display: 'block' }}>
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
