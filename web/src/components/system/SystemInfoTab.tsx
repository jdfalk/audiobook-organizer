// file: web/src/components/system/SystemInfoTab.tsx
// version: 1.0.0
// guid: 1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d

import { useState, useEffect } from 'react';
import {
  Box,
  Typography,
  Grid,
  Card,
  CardContent,
  Stack,
  Chip,
  Divider,
  Button,
} from '@mui/material';
import {
  Computer as ComputerIcon,
  Memory as MemoryIcon,
  Storage as StorageIcon,
  Code as CodeIcon,
  Refresh as RefreshIcon,
} from '@mui/icons-material';

interface SystemInfo {
  os: {
    platform: string;
    arch: string;
    version: string;
  };
  memory: {
    total: number;
    used: number;
    free: number;
    cacheSize: number;
  };
  cpu: {
    cores: number;
    model: string;
    usage: number;
  };
  runtime: {
    goVersion: string;
    numGoroutines: number;
    uptime: string;
  };
  database: {
    size: number;
    books: number;
    authors: number;
    series: number;
  };
}

export function SystemInfoTab() {
  const [info, setInfo] = useState<SystemInfo>({
    os: {
      platform: 'darwin',
      arch: 'arm64',
      version: 'macOS 14.2',
    },
    memory: {
      total: 16 * 1024 * 1024 * 1024, // 16GB
      used: 8 * 1024 * 1024 * 1024, // 8GB
      free: 8 * 1024 * 1024 * 1024, // 8GB
      cacheSize: 500 * 1024 * 1024, // 500MB
    },
    cpu: {
      cores: 8,
      model: 'Apple M1',
      usage: 35,
    },
    runtime: {
      goVersion: '1.23.0',
      numGoroutines: 24,
      uptime: '2 days, 14 hours',
    },
    database: {
      size: 125 * 1024 * 1024, // 125MB
      books: 1247,
      authors: 389,
      series: 156,
    },
  });

  useEffect(() => {
    fetchSystemInfo();
  }, []);

  const fetchSystemInfo = async () => {
    try {
      // TODO: Replace with actual API call
      // const response = await fetch('/api/v1/system/info');
      // const data = await response.json();
      // setInfo(data);
    } catch (error) {
      console.error('Failed to fetch system info:', error);
    }
  };

  const formatBytes = (bytes: number): string => {
    const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB'];
    if (bytes === 0) return '0 Bytes';
    const i = Math.floor(Math.log(bytes) / Math.log(1024));
    return `${(bytes / Math.pow(1024, i)).toFixed(2)} ${sizes[i]}`;
  };

  const getOSIcon = () => {
    switch (info.os.platform) {
      case 'darwin':
        return '🍎';
      case 'linux':
        return '🐧';
      case 'windows':
        return '🪟';
      default:
        return '💻';
    }
  };

  return (
    <Box>
      <Stack direction="row" justifyContent="space-between" alignItems="center" mb={2}>
        <Typography variant="h6">System Information</Typography>
        <Button variant="outlined" startIcon={<RefreshIcon />} onClick={fetchSystemInfo}>
          Refresh
        </Button>
      </Stack>

      <Grid container spacing={3}>
        {/* Operating System */}
        <Grid item xs={12} md={6}>
          <Card>
            <CardContent>
              <Stack direction="row" spacing={2} alignItems="center" mb={2}>
                <ComputerIcon color="primary" />
                <Typography variant="h6">Operating System</Typography>
              </Stack>
              <Stack spacing={1.5}>
                <Box>
                  <Typography variant="body2" color="text.secondary">
                    Platform
                  </Typography>
                  <Stack direction="row" alignItems="center" spacing={1}>
                    <Typography variant="body1">{getOSIcon()}</Typography>
                    <Typography variant="body1">{info.os.version}</Typography>
                  </Stack>
                </Box>
                <Divider />
                <Box>
                  <Typography variant="body2" color="text.secondary">
                    Architecture
                  </Typography>
                  <Typography variant="body1">{info.os.arch}</Typography>
                </Box>
              </Stack>
            </CardContent>
          </Card>
        </Grid>

        {/* Memory */}
        <Grid item xs={12} md={6}>
          <Card>
            <CardContent>
              <Stack direction="row" spacing={2} alignItems="center" mb={2}>
                <MemoryIcon color="primary" />
                <Typography variant="h6">Memory</Typography>
              </Stack>
              <Stack spacing={1.5}>
                <Box>
                  <Typography variant="body2" color="text.secondary">
                    Total / Used
                  </Typography>
                  <Typography variant="body1">
                    {formatBytes(info.memory.total)} / {formatBytes(info.memory.used)}
                  </Typography>
                </Box>
                <Divider />
                <Box>
                  <Typography variant="body2" color="text.secondary">
                    Free
                  </Typography>
                  <Typography variant="body1">{formatBytes(info.memory.free)}</Typography>
                </Box>
                <Divider />
                <Box>
                  <Typography variant="body2" color="text.secondary">
                    Cache Size (App)
                  </Typography>
                  <Typography variant="body1">{formatBytes(info.memory.cacheSize)}</Typography>
                </Box>
              </Stack>
            </CardContent>
          </Card>
        </Grid>

        {/* CPU */}
        <Grid item xs={12} md={6}>
          <Card>
            <CardContent>
              <Stack direction="row" spacing={2} alignItems="center" mb={2}>
                <ComputerIcon color="primary" />
                <Typography variant="h6">CPU</Typography>
              </Stack>
              <Stack spacing={1.5}>
                <Box>
                  <Typography variant="body2" color="text.secondary">
                    Model
                  </Typography>
                  <Typography variant="body1">{info.cpu.model}</Typography>
                </Box>
                <Divider />
                <Box>
                  <Typography variant="body2" color="text.secondary">
                    Cores
                  </Typography>
                  <Typography variant="body1">{info.cpu.cores}</Typography>
                </Box>
                <Divider />
                <Box>
                  <Stack direction="row" justifyContent="space-between" alignItems="center">
                    <Typography variant="body2" color="text.secondary">
                      Usage
                    </Typography>
                    <Chip
                      label={`${info.cpu.usage}%`}
                      size="small"
                      color={info.cpu.usage > 80 ? 'error' : info.cpu.usage > 60 ? 'warning' : 'success'}
                    />
                  </Stack>
                </Box>
              </Stack>
            </CardContent>
          </Card>
        </Grid>

        {/* Runtime */}
        <Grid item xs={12} md={6}>
          <Card>
            <CardContent>
              <Stack direction="row" spacing={2} alignItems="center" mb={2}>
                <CodeIcon color="primary" />
                <Typography variant="h6">Runtime</Typography>
              </Stack>
              <Stack spacing={1.5}>
                <Box>
                  <Typography variant="body2" color="text.secondary">
                    Go Version
                  </Typography>
                  <Typography variant="body1">{info.runtime.goVersion}</Typography>
                </Box>
                <Divider />
                <Box>
                  <Typography variant="body2" color="text.secondary">
                    Goroutines
                  </Typography>
                  <Typography variant="body1">{info.runtime.numGoroutines}</Typography>
                </Box>
                <Divider />
                <Box>
                  <Typography variant="body2" color="text.secondary">
                    Uptime
                  </Typography>
                  <Typography variant="body1">{info.runtime.uptime}</Typography>
                </Box>
              </Stack>
            </CardContent>
          </Card>
        </Grid>

        {/* Database */}
        <Grid item xs={12}>
          <Card>
            <CardContent>
              <Stack direction="row" spacing={2} alignItems="center" mb={2}>
                <StorageIcon color="primary" />
                <Typography variant="h6">Database</Typography>
              </Stack>
              <Grid container spacing={3}>
                <Grid item xs={12} sm={6} md={3}>
                  <Box>
                    <Typography variant="body2" color="text.secondary">
                      Database Size
                    </Typography>
                    <Typography variant="h6">{formatBytes(info.database.size)}</Typography>
                  </Box>
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  <Box>
                    <Typography variant="body2" color="text.secondary">
                      Books
                    </Typography>
                    <Typography variant="h6">{info.database.books.toLocaleString()}</Typography>
                  </Box>
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  <Box>
                    <Typography variant="body2" color="text.secondary">
                      Authors
                    </Typography>
                    <Typography variant="h6">{info.database.authors.toLocaleString()}</Typography>
                  </Box>
                </Grid>
                <Grid item xs={12} sm={6} md={3}>
                  <Box>
                    <Typography variant="body2" color="text.secondary">
                      Series
                    </Typography>
                    <Typography variant="h6">{info.database.series.toLocaleString()}</Typography>
                  </Box>
                </Grid>
              </Grid>
            </CardContent>
          </Card>
        </Grid>
      </Grid>
    </Box>
  );
}
