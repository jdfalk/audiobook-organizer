// file: web/src/components/layout/OperationsIndicator.tsx
// version: 3.0.0
// guid: 3b4c5d6e-7f8a-9b0c-1d2e-3f4a5b6c7d8e

import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Badge,
  Box,
  Button,
  CircularProgress,
  Divider,
  IconButton,
  LinearProgress,
  Popover,
  Tooltip,
  Typography,
} from '@mui/material';
import NotificationsIcon from '@mui/icons-material/Notifications.js';
import CheckCircleIcon from '@mui/icons-material/CheckCircle.js';
import ErrorIcon from '@mui/icons-material/Error.js';
import CancelIcon from '@mui/icons-material/Cancel.js';
import OpenInNewIcon from '@mui/icons-material/OpenInNew.js';
import {
  useOperationsStore,
  type ActiveOperation,
} from '../../stores/useOperationsStore';
import { cancelOperation, getActiveOperations } from '../../services/api';

function formatOperationType(type: string): string {
  switch (type) {
    case 'itunes_import':
      return 'iTunes Import';
    case 'itunes_sync':
      return 'iTunes Sync';
    case 'scan':
      return 'Library Scan';
    case 'organize':
      return 'Organize';
    case 'metadata_fetch':
      return 'Metadata Fetch';
    case 'ol_dump_import':
      return 'Open Library Import';
    default:
      return type.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
  }
}

function formatETA(op: ActiveOperation): string | null {
  if (!op.startedAt || op.progress <= 0 || op.total <= 0) return null;
  const elapsed = (Date.now() - op.startedAt) / 1000;
  if (elapsed < 5) return null;
  const rate = op.progress / elapsed;
  if (rate <= 0) return null;
  const remaining = (op.total - op.progress) / rate;
  if (remaining < 60) return `~${Math.ceil(remaining)}s left`;
  if (remaining < 3600) return `~${Math.ceil(remaining / 60)}m left`;
  const h = Math.floor(remaining / 3600);
  const m = Math.ceil((remaining % 3600) / 60);
  return `~${h}h ${m}m left`;
}

function formatElapsed(op: ActiveOperation): string | null {
  if (!op.startedAt) return null;
  const sec = Math.floor((Date.now() - op.startedAt) / 1000);
  if (sec < 60) return `${sec}s`;
  if (sec < 3600) return `${Math.floor(sec / 60)}m ${sec % 60}s`;
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  return `${h}h ${m}m`;
}

function parseMessageDetails(message: string) {
  const titleMatch = message.match(/\u2014\s*(.+)$/);
  const countsMatch = message.match(
    /\(imported (\d+), skipped (\d+), failed (\d+)\)/
  );
  // OL import: "Importing editions: 1234k records"
  const olMatch = message.match(/Importing (\w+): (\d+)k records/);
  return {
    currentTitle: titleMatch ? titleMatch[1] : null,
    imported: countsMatch ? parseInt(countsMatch[1]) : null,
    skipped: countsMatch ? parseInt(countsMatch[2]) : null,
    failed: countsMatch ? parseInt(countsMatch[3]) : null,
    olType: olMatch ? olMatch[1] : null,
    olRecords: olMatch ? `${olMatch[2]}k` : null,
  };
}

export function OperationsIndicator() {
  const activeOperations = useOperationsStore(
    (state) => state.activeOperations
  );
  const startPolling = useOperationsStore((state) => state.startPolling);
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null);
  const [cancelling, setCancelling] = useState<Set<string>>(new Set());
  const navigate = useNavigate();

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
        // Ignore
      }
    };

    void checkActiveOps();
    const interval = setInterval(checkActiveOps, 15000);
    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, [startPolling]);

  const handleCancel = async (opId: string) => {
    setCancelling((prev) => new Set(prev).add(opId));
    try {
      await cancelOperation(opId);
    } catch {
      // Will show as failed in next poll
    }
    setCancelling((prev) => {
      const next = new Set(prev);
      next.delete(opId);
      return next;
    });
  };

  const inProgress = activeOperations.filter(
    (op) => !['completed', 'failed', 'canceled'].includes(op.status)
  );
  const terminal = activeOperations.filter((op) =>
    ['completed', 'failed', 'canceled'].includes(op.status)
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
        <Box sx={{ minWidth: 400, maxWidth: 480 }}>
          {/* Header */}
          <Box
            sx={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              px: 2,
              pt: 1.5,
              pb: 1,
            }}
          >
            <Typography variant="subtitle2">Operations</Typography>
            <Button
              size="small"
              endIcon={<OpenInNewIcon sx={{ fontSize: 14 }} />}
              onClick={() => {
                setAnchorEl(null);
                navigate('/operations');
              }}
              sx={{ textTransform: 'none', fontSize: '0.75rem' }}
            >
              View All
            </Button>
          </Box>

          <Divider />

          {activeOperations.length === 0 && (
            <Typography
              variant="body2"
              color="text.secondary"
              sx={{ px: 2, py: 3, textAlign: 'center' }}
            >
              No active operations
            </Typography>
          )}

          {/* Active operations */}
          {inProgress.map((op: ActiveOperation) => {
            const progressPct =
              op.total > 0 ? Math.round((op.progress / op.total) * 100) : 0;
            const eta = formatETA(op);
            const elapsed = formatElapsed(op);
            const details = parseMessageDetails(op.message);

            return (
              <Box key={op.id} sx={{ px: 2, py: 1.5, '&:not(:last-child)': { borderBottom: '1px solid', borderColor: 'divider' } }}>
                {/* Row 1: Type + Cancel */}
                <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 0.5 }}>
                  <Typography variant="body2" fontWeight="bold">
                    {formatOperationType(op.type)}
                  </Typography>
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                    {elapsed && (
                      <Typography variant="caption" color="text.secondary">
                        {elapsed}
                      </Typography>
                    )}
                    <Tooltip title="Cancel">
                      <IconButton
                        size="small"
                        color="error"
                        onClick={() => handleCancel(op.id)}
                        disabled={cancelling.has(op.id)}
                        sx={{ p: 0.25 }}
                      >
                        {cancelling.has(op.id) ? (
                          <CircularProgress size={14} />
                        ) : (
                          <CancelIcon sx={{ fontSize: 18 }} />
                        )}
                      </IconButton>
                    </Tooltip>
                  </Box>
                </Box>

                {/* Row 2: Progress bar */}
                {op.total > 0 ? (
                  <LinearProgress
                    variant="determinate"
                    value={progressPct}
                    sx={{ height: 6, borderRadius: 1, mb: 0.5 }}
                  />
                ) : (
                  <LinearProgress sx={{ height: 6, borderRadius: 1, mb: 0.5 }} />
                )}

                {/* Row 3: Numbers */}
                <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 0.25 }}>
                  <Typography variant="caption" color="text.secondary" sx={{ fontFamily: 'monospace' }}>
                    {op.total > 0 ? (
                      <>
                        {op.progress.toLocaleString()} / {op.total.toLocaleString()}
                        {' '}({progressPct}%)
                      </>
                    ) : (
                      'Starting...'
                    )}
                  </Typography>
                  {eta && (
                    <Typography variant="caption" color="text.secondary" fontStyle="italic">
                      {eta}
                    </Typography>
                  )}
                </Box>

                {/* Row 4: Import stats (if available) */}
                {details.imported !== null && (
                  <Typography variant="caption" sx={{ display: 'block', mb: 0.25 }}>
                    <Box component="span" sx={{ color: 'success.main' }}>{details.imported} imported</Box>
                    {details.skipped! > 0 && (
                      <Box component="span" sx={{ color: 'text.secondary', ml: 1 }}>{details.skipped} skipped</Box>
                    )}
                    {details.failed! > 0 && (
                      <Box component="span" sx={{ color: 'error.main', ml: 1 }}>{details.failed} failed</Box>
                    )}
                  </Typography>
                )}

                {/* Row 4 alt: OL import stats */}
                {details.olType && (
                  <Typography variant="caption" color="info.main" sx={{ display: 'block', mb: 0.25 }}>
                    {details.olType}: {details.olRecords} records
                  </Typography>
                )}

                {/* Row 5: Current item */}
                {details.currentTitle && (
                  <Typography
                    variant="caption"
                    color="primary.main"
                    display="block"
                    noWrap
                    title={details.currentTitle}
                  >
                    {details.currentTitle}
                  </Typography>
                )}
              </Box>
            );
          })}

          {/* Completed/failed */}
          {terminal.length > 0 && inProgress.length > 0 && <Divider />}
          {terminal.map((op: ActiveOperation) => {
            const details = parseMessageDetails(op.message);
            return (
              <Box
                key={op.id}
                sx={{ px: 2, py: 1, opacity: 0.7, '&:not(:last-child)': { borderBottom: '1px solid', borderColor: 'divider' } }}
              >
                <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <Typography variant="caption" fontWeight="bold">
                    {formatOperationType(op.type)}
                  </Typography>
                  {op.status === 'completed' && <CheckCircleIcon color="success" sx={{ fontSize: 16 }} />}
                  {op.status === 'failed' && <ErrorIcon color="error" sx={{ fontSize: 16 }} />}
                  {op.status === 'canceled' && <CancelIcon color="disabled" sx={{ fontSize: 16 }} />}
                </Box>
                {details.imported !== null && (
                  <Typography variant="caption" color="text.secondary" display="block">
                    {details.imported} imported
                    {details.skipped! > 0 ? `, ${details.skipped} skipped` : ''}
                    {details.failed! > 0 ? `, ${details.failed} failed` : ''}
                  </Typography>
                )}
                {!details.imported && op.message && (
                  <Typography variant="caption" color="text.secondary" display="block" noWrap title={op.message}>
                    {op.message}
                  </Typography>
                )}
              </Box>
            );
          })}
        </Box>
      </Popover>
    </>
  );
}
