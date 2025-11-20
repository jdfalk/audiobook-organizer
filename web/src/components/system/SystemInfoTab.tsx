// file: web/src/components/system/SystemInfoTab.tsx
// version: 1.3.0
// guid: 1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d

import { useState, useEffect } from 'react';
import {
  Box,
  Typography,
  Grid,
  Card,
  CardContent,
  Stack,
  Divider,
  Button,
  CircularProgress,
} from '@mui/material';
import {
  Computer as ComputerIcon,
  Memory as MemoryIcon,
  Storage as StorageIcon,
  Code as CodeIcon,
  Refresh as RefreshIcon,
} from '@mui/icons-material';
import * as api from '../../services/api';

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
    heapAlloc: number;
    heapSys: number;
  };
  cpu: {
    cores: number;
  };
  runtime: {
    goVersion: string;
    numGoroutines: number;
  };
  database: {
    size: number;
    books: number;
    folderCount: number;
  };
}

export function SystemInfoTab() {
  const [info, setInfo] = useState<SystemInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    fetchSystemInfo();
  }, []);

  const fetchSystemInfo = async () => {
    setLoading(true);
    setError(null);
    try {
      const status = await api.getSystemStatus();

      // Map API SystemStatus to SystemInfo format
      setInfo({
        os: {
          platform: status.runtime.os,
          arch: status.runtime.arch,
          version: `${status.runtime.os} ${status.runtime.arch}`,
        },
        memory: {
          total: status.memory.sys_bytes,
          used: status.memory.alloc_bytes,
          free: status.memory.sys_bytes - status.memory.alloc_bytes,
          heapAlloc: status.memory.heap_alloc,
          heapSys: status.memory.heap_sys,
        },
        cpu: {
          cores: status.runtime.num_cpu,
        },
        runtime: {
          goVersion: status.runtime.go_version,
          numGoroutines: status.runtime.num_goroutine,
        },
        database: {
          size: status.library.total_size,
          books: status.library.book_count,
          folderCount: status.library.folder_count,
        },
      });
    } catch (err) {
      console.error('Failed to fetch system info:', err);
      setError(err instanceof Error ? err.message : 'Failed to fetch system info');
    } finally {
      setLoading(false);
    }
  };

  const formatBytes = (bytes: number): string => {
    const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB'];
    if (bytes === 0) return '0 Bytes';
    const i = Math.floor(Math.log(bytes) / Math.log(1024));
    return `${(bytes / Math.pow(1024, i)).toFixed(2)} ${sizes[i]}`;
  };

  const getOSIcon = () => {
    if (!info) return '💻';
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

  if (loading) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight="400px">
        <CircularProgress />
      </Box>
    );
  }

  if (error) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight="400px">
        <Typography color="error">{error}</Typography>
      </Box>
    );
  }

  if (!info) {
    return null;
  }

  return (
    <Box>
      <Stack direction="row" justifyContent="space-between" alignItems="center" mb={2}>
        <Typography variant="h6">System Information</Typography>
        <Button
          variant="outlined"
          startIcon={<RefreshIcon />}
          onClick={fetchSystemInfo}
          disabled={loading}
        >
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
                    App Memory System
                  </Typography>
                  <Typography variant="body1">
                    {formatBytes(info.memory.total)}
                  </Typography>
                </Box>
                <Divider />
                <Box>
                  <Typography variant="body2" color="text.secondary">
                    Used by App
                  </Typography>
                  <Typography variant="body1">{formatBytes(info.memory.used)}</Typography>
                </Box>
                <Divider />
                <Box>
                  <Typography variant="body2" color="text.secondary">
                    Heap Allocated
                  </Typography>
                  <Typography variant="body1">{formatBytes(info.memory.heapAlloc)}</Typography>
                </Box>
                <Divider />
                <Box>
                  <Typography variant="body2" color="text.secondary">
                    Heap System
                  </Typography>
                  <Typography variant="body1">{formatBytes(info.memory.heapSys)}</Typography>
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
                    Available Cores
                  </Typography>
                  <Typography variant="body1">{info.cpu.cores}</Typography>
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
              </Stack>
            </CardContent>
          </Card>
        </Grid>

        {/* Database & Library */}
        <Grid item xs={12}>
          <Card>
            <CardContent>
              <Stack direction="row" spacing={2} alignItems="center" mb={2}>
                <StorageIcon color="primary" />
                <Typography variant="h6">Library & Storage</Typography>
              </Stack>
              <Grid container spacing={3}>
                <Grid item xs={12} sm={6} md={4}>
                  <Box>
                    <Typography variant="body2" color="text.secondary">
                      Library Size
                    </Typography>
                    <Typography variant="h6">{formatBytes(info.database.size)}</Typography>
                  </Box>
                </Grid>
                <Grid item xs={12} sm={6} md={4}>
                  <Box>
                    <Typography variant="body2" color="text.secondary">
                      Books
                    </Typography>
                    <Typography variant="h6">{info.database.books.toLocaleString()}</Typography>
                  </Box>
                </Grid>
                <Grid item xs={12} sm={6} md={4}>
                  <Box>
                    <Typography variant="body2" color="text.secondary">
                      Library Folders
                    </Typography>
                    <Typography variant="h6">{info.database.folderCount}</Typography>
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
