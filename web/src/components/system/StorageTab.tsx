// file: web/src/components/system/StorageTab.tsx
// version: 1.0.0
// guid: 9e0f1a2b-3c4d-5e6f-7a8b-9c0d1e2f3a4b

import { useState, useEffect } from 'react';
import {
  Box,
  Typography,
  Stack,
  LinearProgress,
  Grid,
  Card,
  CardContent,
  Divider,
  Button,
} from '@mui/material';
import {
  Storage as StorageIcon,
  Folder as FolderIcon,
  LibraryBooks as LibraryIcon,
  Refresh as RefreshIcon,
} from '@mui/icons-material';

interface StorageInfo {
  total: number;
  used: number;
  free: number;
  librarySize: number;
  importPathsSizes: { path: string; size: number }[];
}

export function StorageTab() {
  const [storage] = useState<StorageInfo>({
    total: 1000 * 1024 * 1024 * 1024, // 1TB
    used: 670 * 1024 * 1024 * 1024, // 670GB
    free: 330 * 1024 * 1024 * 1024, // 330GB
    librarySize: 450 * 1024 * 1024 * 1024, // 450GB
    importPathsSizes: [
      { path: '/audiobooks/import1', size: 120 * 1024 * 1024 * 1024 },
      { path: '/audiobooks/import2', size: 85 * 1024 * 1024 * 1024 },
      { path: '/media/downloads', size: 15 * 1024 * 1024 * 1024 },
    ],
  });

  useEffect(() => {
    fetchStorageInfo();
  }, []);

  const fetchStorageInfo = async () => {
    try {
      // TODO: Replace with actual API call
      // const response = await fetch('/api/v1/system/storage');
      // const data = await response.json();
      // setStorage(data);
    } catch (error) {
      console.error('Failed to fetch storage info:', error);
    }
  };

  const formatBytes = (bytes: number): string => {
    const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB'];
    if (bytes === 0) return '0 Bytes';
    const i = Math.floor(Math.log(bytes) / Math.log(1024));
    return `${(bytes / Math.pow(1024, i)).toFixed(2)} ${sizes[i]}`;
  };

  const getPercentage = (used: number, total: number): number => {
    return (used / total) * 100;
  };

  const usedPercentage = getPercentage(storage.used, storage.total);
  const libraryPercentage = getPercentage(storage.librarySize, storage.total);

  return (
    <Box>
      <Stack direction="row" justifyContent="space-between" alignItems="center" mb={2}>
        <Typography variant="h6">Storage Breakdown</Typography>
        <Button variant="outlined" startIcon={<RefreshIcon />} onClick={fetchStorageInfo}>
          Refresh
        </Button>
      </Stack>

      <Grid container spacing={3}>
        {/* Overall Storage */}
        <Grid item xs={12}>
          <Card>
            <CardContent>
              <Stack direction="row" spacing={2} alignItems="center" mb={2}>
                <StorageIcon color="primary" />
                <Typography variant="h6">Overall Disk Usage</Typography>
              </Stack>
              <Box sx={{ mb: 2 }}>
                <Stack direction="row" justifyContent="space-between" mb={1}>
                  <Typography variant="body2" color="text.secondary">
                    {formatBytes(storage.used)} used of {formatBytes(storage.total)}
                  </Typography>
                  <Typography variant="body2" color="text.secondary">
                    {usedPercentage.toFixed(1)}%
                  </Typography>
                </Stack>
                <LinearProgress
                  variant="determinate"
                  value={usedPercentage}
                  sx={{ height: 10, borderRadius: 1 }}
                  color={usedPercentage > 90 ? 'error' : usedPercentage > 75 ? 'warning' : 'primary'}
                />
              </Box>
              <Stack direction="row" spacing={4}>
                <Box>
                  <Typography variant="body2" color="text.secondary">
                    Used
                  </Typography>
                  <Typography variant="h6">{formatBytes(storage.used)}</Typography>
                </Box>
                <Box>
                  <Typography variant="body2" color="text.secondary">
                    Free
                  </Typography>
                  <Typography variant="h6">{formatBytes(storage.free)}</Typography>
                </Box>
                <Box>
                  <Typography variant="body2" color="text.secondary">
                    Total
                  </Typography>
                  <Typography variant="h6">{formatBytes(storage.total)}</Typography>
                </Box>
              </Stack>
            </CardContent>
          </Card>
        </Grid>

        {/* Library Storage */}
        <Grid item xs={12} md={6}>
          <Card>
            <CardContent>
              <Stack direction="row" spacing={2} alignItems="center" mb={2}>
                <LibraryIcon color="primary" />
                <Typography variant="h6">Library Storage</Typography>
              </Stack>
              <Box sx={{ mb: 2 }}>
                <Stack direction="row" justifyContent="space-between" mb={1}>
                  <Typography variant="body2" color="text.secondary">
                    {formatBytes(storage.librarySize)}
                  </Typography>
                  <Typography variant="body2" color="text.secondary">
                    {libraryPercentage.toFixed(1)}% of total disk
                  </Typography>
                </Stack>
                <LinearProgress
                  variant="determinate"
                  value={libraryPercentage}
                  sx={{ height: 10, borderRadius: 1 }}
                />
              </Box>
              <Typography variant="body2" color="text.secondary">
                Organized audiobook files in the main library
              </Typography>
            </CardContent>
          </Card>
        </Grid>

        {/* Import Paths Storage */}
        <Grid item xs={12} md={6}>
          <Card>
            <CardContent>
              <Stack direction="row" spacing={2} alignItems="center" mb={2}>
                <FolderIcon color="primary" />
                <Typography variant="h6">Import Paths</Typography>
              </Stack>
              {storage.importPathsSizes.map((item, index) => (
                <Box key={index} sx={{ mb: index < storage.importPathsSizes.length - 1 ? 2 : 0 }}>
                  <Stack direction="row" justifyContent="space-between" mb={0.5}>
                    <Typography variant="body2" noWrap sx={{ flex: 1, mr: 2 }}>
                      {item.path}
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                      {formatBytes(item.size)}
                    </Typography>
                  </Stack>
                  {index < storage.importPathsSizes.length - 1 && <Divider sx={{ mt: 1 }} />}
                </Box>
              ))}
              <Divider sx={{ my: 2 }} />
              <Stack direction="row" justifyContent="space-between">
                <Typography variant="body2" fontWeight="bold">
                  Total Import Paths
                </Typography>
                <Typography variant="body2" fontWeight="bold">
                  {formatBytes(storage.importPathsSizes.reduce((sum, item) => sum + item.size, 0))}
                </Typography>
              </Stack>
            </CardContent>
          </Card>
        </Grid>
      </Grid>
    </Box>
  );
}
