// file: web/src/components/layout/OperationsIndicator.tsx
// version: 1.1.0
// guid: 3b4c5d6e-7f8a-9b0c-1d2e-3f4a5b6c7d8e

import { useEffect, useState } from 'react';
import {
  Badge,
  Box,
  CircularProgress,
  IconButton,
  LinearProgress,
  Popover,
  Tooltip,
  Typography,
} from '@mui/material';
import NotificationsIcon from '@mui/icons-material/Notifications.js';
import CheckCircleIcon from '@mui/icons-material/CheckCircle.js';
import ErrorIcon from '@mui/icons-material/Error.js';
import {
  useOperationsStore,
  type ActiveOperation,
} from '../../stores/useOperationsStore';
import { getActiveOperations } from '../../services/api';

function formatOperationType(type: string): string {
  switch (type) {
    case 'itunes_import':
      return 'iTunes Import';
    case 'scan':
      return 'Library Scan';
    case 'organize':
      return 'Organize';
    default:
      return type;
  }
}

export function OperationsIndicator() {
  const activeOperations = useOperationsStore(
    (state) => state.activeOperations
  );
  const startPolling = useOperationsStore((state) => state.startPolling);
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null);

  // On mount, check for already-running operations from the backend
  useEffect(() => {
    let cancelled = false;

    const checkActiveOps = async () => {
      try {
        const ops = await getActiveOperations();
        if (cancelled) return;
        const store = useOperationsStore.getState();
        for (const op of ops) {
          const alreadyTracked = store.activeOperations.some(
            (a) => a.id === op.id
          );
          if (
            !alreadyTracked &&
            !['completed', 'failed', 'canceled'].includes(op.status)
          ) {
            startPolling(op.id, op.type);
          }
        }
      } catch {
        // Ignore — API may not be ready yet
      }
    };

    void checkActiveOps();
    // Re-check every 30s in case operations are started from another tab/API
    const interval = setInterval(checkActiveOps, 30000);

    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, [startPolling]);

  const inProgress = activeOperations.filter(
    (op) => !['completed', 'failed', 'canceled'].includes(op.status)
  );
  const badgeCount = inProgress.length;

  return (
    <>
      <Tooltip
        title={
          badgeCount > 0
            ? `${badgeCount} active operation${badgeCount !== 1 ? 's' : ''}`
            : 'No active operations'
        }
      >
        <IconButton
          color="inherit"
          onClick={(e) => setAnchorEl(e.currentTarget)}
          sx={{ mr: 1 }}
        >
          <Badge
            badgeContent={badgeCount > 0 ? badgeCount : undefined}
            color="warning"
          >
            {badgeCount > 0 ? (
              <CircularProgress size={24} color="inherit" thickness={4} />
            ) : (
              <NotificationsIcon />
            )}
          </Badge>
        </IconButton>
      </Tooltip>
      <Popover
        open={Boolean(anchorEl)}
        anchorEl={anchorEl}
        onClose={() => setAnchorEl(null)}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
        transformOrigin={{ vertical: 'top', horizontal: 'right' }}
      >
        <Box sx={{ p: 2, minWidth: 320, maxWidth: 400 }}>
          <Typography variant="subtitle2" sx={{ mb: 1 }}>
            Operations
          </Typography>
          {activeOperations.length === 0 && (
            <Typography variant="body2" color="text.secondary">
              No active operations
            </Typography>
          )}
          {activeOperations.map((op: ActiveOperation) => {
            const isTerminal = ['completed', 'failed', 'canceled'].includes(
              op.status
            );
            const progressPct =
              op.total > 0 ? Math.round((op.progress / op.total) * 100) : 0;

            return (
              <Box
                key={op.id}
                sx={{
                  mb: 1.5,
                  p: 1.5,
                  borderRadius: 1,
                  bgcolor: 'action.hover',
                }}
              >
                <Box
                  sx={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    mb: 0.5,
                  }}
                >
                  <Typography variant="body2" fontWeight="bold">
                    {formatOperationType(op.type)}
                  </Typography>
                  {op.status === 'completed' && (
                    <CheckCircleIcon color="success" fontSize="small" />
                  )}
                  {op.status === 'failed' && (
                    <ErrorIcon color="error" fontSize="small" />
                  )}
                </Box>
                {!isTerminal && op.total > 0 && (
                  <>
                    <LinearProgress
                      variant="determinate"
                      value={progressPct}
                      sx={{ mb: 0.5, borderRadius: 1 }}
                    />
                    <Typography variant="caption" color="text.secondary">
                      {op.progress.toLocaleString()} /{' '}
                      {op.total.toLocaleString()} ({progressPct}%)
                    </Typography>
                  </>
                )}
                {!isTerminal && op.total === 0 && (
                  <LinearProgress sx={{ mb: 0.5, borderRadius: 1 }} />
                )}
                <Typography
                  variant="caption"
                  color="text.secondary"
                  display="block"
                  noWrap
                  title={op.message}
                >
                  {op.message}
                </Typography>
              </Box>
            );
          })}
        </Box>
      </Popover>
    </>
  );
}
