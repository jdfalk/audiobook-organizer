// file: web/src/components/system/QuotaTab.tsx
// version: 1.0.0
// guid: 0f1a2b3c-4d5e-6f7a-8b9c-0d1e2f3a4b5c

import { useState, useEffect } from 'react';
import {
  Box,
  Typography,
  Paper,
  Stack,
  LinearProgress,
  Grid,
  Card,
  CardContent,
  Alert,
  Button,
  Chip,
} from '@mui/material';
import {
  Warning as WarningIcon,
  CheckCircle as CheckIcon,
  Error as ErrorIcon,
  Refresh as RefreshIcon,
} from '@mui/icons-material';

interface QuotaInfo {
  systemQuotaEnabled: boolean;
  systemQuotaPercent: number;
  systemQuotaUsed: number;
  systemQuotaLimit: number;
  userQuotasEnabled: boolean;
  userQuotas: {
    username: string;
    used: number;
    limit: number;
    status: 'ok' | 'warning' | 'exceeded';
  }[];
}

export function QuotaTab() {
  const [quota] = useState<QuotaInfo>({
    systemQuotaEnabled: true,
    systemQuotaPercent: 80,
    systemQuotaUsed: 670 * 1024 * 1024 * 1024, // 670GB
    systemQuotaLimit: 800 * 1024 * 1024 * 1024, // 800GB (80% of 1TB)
    userQuotasEnabled: false,
    userQuotas: [
      {
        username: 'admin',
        used: 350 * 1024 * 1024 * 1024,
        limit: 500 * 1024 * 1024 * 1024,
        status: 'ok',
      },
      {
        username: 'user1',
        used: 240 * 1024 * 1024 * 1024,
        limit: 250 * 1024 * 1024 * 1024,
        status: 'warning',
      },
      {
        username: 'user2',
        used: 80 * 1024 * 1024 * 1024,
        limit: 100 * 1024 * 1024 * 1024,
        status: 'ok',
      },
    ],
  });

  useEffect(() => {
    fetchQuotaInfo();
  }, []);

  const fetchQuotaInfo = async () => {
    try {
      // TODO: Replace with actual API call
      // const response = await fetch('/api/v1/system/quotas');
      // const data = await response.json();
      // setQuota(data);
    } catch (error) {
      console.error('Failed to fetch quota info:', error);
    }
  };

  const formatBytes = (bytes: number): string => {
    const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB'];
    if (bytes === 0) return '0 Bytes';
    const i = Math.floor(Math.log(bytes) / Math.log(1024));
    return `${(bytes / Math.pow(1024, i)).toFixed(2)} ${sizes[i]}`;
  };

  const getPercentage = (used: number, limit: number): number => {
    return (used / limit) * 100;
  };

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'exceeded':
        return <ErrorIcon color="error" />;
      case 'warning':
        return <WarningIcon color="warning" />;
      default:
        return <CheckIcon color="success" />;
    }
  };

  const getStatusColor = (status: string): 'error' | 'warning' | 'success' => {
    switch (status) {
      case 'exceeded':
        return 'error';
      case 'warning':
        return 'warning';
      default:
        return 'success';
    }
  };

  const systemPercentage = getPercentage(quota.systemQuotaUsed, quota.systemQuotaLimit);

  return (
    <Box>
      <Stack direction="row" justifyContent="space-between" alignItems="center" mb={2}>
        <Typography variant="h6">Quota Information</Typography>
        <Button variant="outlined" startIcon={<RefreshIcon />} onClick={fetchQuotaInfo}>
          Refresh
        </Button>
      </Stack>

      <Grid container spacing={3}>
        {/* System-wide Quota */}
        <Grid item xs={12}>
          <Card>
            <CardContent>
              <Stack direction="row" justifyContent="space-between" alignItems="center" mb={2}>
                <Typography variant="h6">System-wide Quota</Typography>
                <Chip
                  label={quota.systemQuotaEnabled ? 'Enabled' : 'Disabled'}
                  color={quota.systemQuotaEnabled ? 'primary' : 'default'}
                  size="small"
                />
              </Stack>

              {quota.systemQuotaEnabled && (
                <>
                  {systemPercentage > 90 && (
                    <Alert severity="error" sx={{ mb: 2 }}>
                      System quota exceeded! Storage usage is above the configured limit of{' '}
                      {quota.systemQuotaPercent}% ({formatBytes(quota.systemQuotaLimit)}).
                    </Alert>
                  )}
                  {systemPercentage > 75 && systemPercentage <= 90 && (
                    <Alert severity="warning" sx={{ mb: 2 }}>
                      Approaching system quota limit. Currently at {systemPercentage.toFixed(1)}% of{' '}
                      {quota.systemQuotaPercent}% limit.
                    </Alert>
                  )}

                  <Box sx={{ mb: 2 }}>
                    <Stack direction="row" justifyContent="space-between" mb={1}>
                      <Typography variant="body2" color="text.secondary">
                        {formatBytes(quota.systemQuotaUsed)} used of{' '}
                        {formatBytes(quota.systemQuotaLimit)} limit
                      </Typography>
                      <Typography variant="body2" color="text.secondary">
                        {systemPercentage.toFixed(1)}%
                      </Typography>
                    </Stack>
                    <LinearProgress
                      variant="determinate"
                      value={Math.min(systemPercentage, 100)}
                      sx={{ height: 10, borderRadius: 1 }}
                      color={
                        systemPercentage > 90
                          ? 'error'
                          : systemPercentage > 75
                            ? 'warning'
                            : 'primary'
                      }
                    />
                  </Box>

                  <Typography variant="body2" color="text.secondary">
                    Maximum disk usage is limited to {quota.systemQuotaPercent}% of total available
                    space
                  </Typography>
                </>
              )}

              {!quota.systemQuotaEnabled && (
                <Typography variant="body2" color="text.secondary">
                  No system-wide quota is currently configured. The application can use all
                  available disk space.
                </Typography>
              )}
            </CardContent>
          </Card>
        </Grid>

        {/* Per-User Quotas */}
        {quota.userQuotasEnabled && (
          <Grid item xs={12}>
            <Card>
              <CardContent>
                <Stack direction="row" justifyContent="space-between" alignItems="center" mb={2}>
                  <Typography variant="h6">Per-User Quotas</Typography>
                  <Chip label="Multi-User Mode" color="primary" size="small" />
                </Stack>

                <Stack spacing={3}>
                  {quota.userQuotas.map((user) => {
                    const percentage = getPercentage(user.used, user.limit);
                    return (
                      <Paper key={user.username} sx={{ p: 2 }}>
                        <Stack
                          direction="row"
                          justifyContent="space-between"
                          alignItems="center"
                          mb={1}
                        >
                          <Stack direction="row" spacing={1} alignItems="center">
                            {getStatusIcon(user.status)}
                            <Typography variant="subtitle1">{user.username}</Typography>
                          </Stack>
                          <Chip
                            label={user.status.toUpperCase()}
                            size="small"
                            color={getStatusColor(user.status)}
                          />
                        </Stack>

                        <Box sx={{ mb: 1 }}>
                          <Stack direction="row" justifyContent="space-between" mb={0.5}>
                            <Typography variant="body2" color="text.secondary">
                              {formatBytes(user.used)} used of {formatBytes(user.limit)} limit
                            </Typography>
                            <Typography variant="body2" color="text.secondary">
                              {percentage.toFixed(1)}%
                            </Typography>
                          </Stack>
                          <LinearProgress
                            variant="determinate"
                            value={Math.min(percentage, 100)}
                            sx={{ height: 8, borderRadius: 1 }}
                            color={
                              percentage > 100 ? 'error' : percentage > 90 ? 'warning' : 'primary'
                            }
                          />
                        </Box>
                      </Paper>
                    );
                  })}
                </Stack>
              </CardContent>
            </Card>
          </Grid>
        )}

        {!quota.userQuotasEnabled && (
          <Grid item xs={12}>
            <Alert severity="info">
              Per-user quotas are not enabled. Enable multi-user mode in Settings to configure
              individual user storage limits.
            </Alert>
          </Grid>
        )}
      </Grid>
    </Box>
  );
}
