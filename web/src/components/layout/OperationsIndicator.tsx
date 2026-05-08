// file: web/src/components/layout/OperationsIndicator.tsx
// version: 3.7.0
// guid: 3b4c5d6e-7f8a-9b0c-1d2e-3f4a5b6c7d8e

import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Badge,
  Box,
  Button,
  Chip,
  CircularProgress,
  Divider,
  IconButton,
  LinearProgress,
  Popover,
  Tooltip,
  Typography,
} from '@mui/material';
import NotificationsIcon from '@mui/icons-material/Notifications.js';
import CancelIcon from '@mui/icons-material/Cancel.js';
import OpenInNewIcon from '@mui/icons-material/OpenInNew.js';
import HourglassEmptyIcon from '@mui/icons-material/HourglassEmpty.js';
import {
  useOperationsStore,
  type ActiveOperation,
} from '../../stores/useOperationsStore';
import {
  cancelOperation,
} from '../../services/api';
import { getUndoPreflight, revertOperation as revertOp } from '../../services/versionApi';

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
    case 'metadata_candidate_fetch':
      return 'Metadata Fetch (Batch)';
    case 'ol_dump_import':
      return 'Open Library Import';
    case 'dedup-scan':
      return 'Dedup Scan';
    case 'dedup-llm-review':
      return 'Dedup AI Review';
    case 'dedup-acoustid-scan':
      return 'AcoustID Scan';
    case 'dedup-book-signature-scan':
      return 'Book Signature Scan';
    case 'embed-scan':
      return 'Embedding Rescan';
    case 'fingerprint-rescan':
      return 'Fingerprint Rescan';
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
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null);
  const [cancelling, setCancelling] = useState<Set<string>>(new Set());
  const navigate = useNavigate();

  // Active ops are now discovered exclusively via SSE (op.created) and
  // loadFromServer (v2 timeline). No v1 polling loop needed here.

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
  const queued = inProgress.filter((op) => op.status === 'queued');
  const running = inProgress.filter((op) => op.status !== 'queued');
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

          {/* Queued (pending) operations */}
          {queued.length > 0 && (
            <>
              <Box sx={{ px: 2, pt: 1, pb: 0.25 }}>
                <Typography variant="caption" color="text.secondary" sx={{ textTransform: 'uppercase', fontSize: '0.65rem', letterSpacing: '0.08em' }}>
                  Pending ({queued.length})
                </Typography>
              </Box>
              {queued.map((op: ActiveOperation) => (
                <Box key={op.id} sx={{ px: 2, py: 1, display: 'flex', alignItems: 'center', justifyContent: 'space-between', '&:not(:last-child)': { borderBottom: '1px solid', borderColor: 'divider' } }}>
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <HourglassEmptyIcon sx={{ fontSize: 16, color: 'text.secondary' }} />
                    <Box>
                      <Typography variant="body2" fontWeight="bold">{formatOperationType(op.type)}</Typography>
                      <Typography variant="caption" color="text.secondary">Waiting to start…</Typography>
                    </Box>
                  </Box>
                  <Tooltip title="Cancel">
                    <IconButton
                      size="small"
                      color="error"
                      onClick={() => handleCancel(op.id)}
                      disabled={cancelling.has(op.id)}
                      sx={{ p: 0.25 }}
                    >
                      {cancelling.has(op.id) ? <CircularProgress size={14} /> : <CancelIcon sx={{ fontSize: 18 }} />}
                    </IconButton>
                  </Tooltip>
                </Box>
              ))}
              {running.length > 0 && (
                <Box sx={{ px: 2, pt: 1, pb: 0.25 }}>
                  <Typography variant="caption" color="text.secondary" sx={{ textTransform: 'uppercase', fontSize: '0.65rem', letterSpacing: '0.08em' }}>
                    Running ({running.length})
                  </Typography>
                </Box>
              )}
            </>
          )}

          {/* Active (running) operations */}
          {running.map((op: ActiveOperation) => {
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

          {/* Recent completed operations — read from v2 store (terminal ops).
              V1 getRecentCompletedOperations() is no longer called (UOS-13). */}
          {terminal.length === 0 && inProgress.length === 0 && (
            <>
              <Divider />
              <Typography
                variant="body2"
                color="text.secondary"
                sx={{ px: 2, py: 1.5, textAlign: 'center', fontSize: '0.8rem' }}
              >
                No recent operations
              </Typography>
            </>
          )}

          {terminal.map((op: ActiveOperation) => (
            <Box
              key={`recent-${op.id}`}
              onClick={() => {
                setAnchorEl(null);
                navigate(`/activity?op=${op.id}`);
              }}
              sx={{
                px: 2,
                py: 1,
                cursor: 'pointer',
                '&:hover': { bgcolor: 'action.hover' },
                '&:not(:last-child)': { borderBottom: '1px solid', borderColor: 'divider' },
              }}
            >
              <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 1 }}>
                <Typography variant="caption" fontWeight="bold" noWrap sx={{ flex: 1 }}>
                  {formatOperationType(op.type)}
                </Typography>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, flexShrink: 0 }}>
                  <Chip
                    label={op.status}
                    size="small"
                    color={op.status === 'completed' ? 'success' : op.status === 'failed' ? 'error' : 'default'}
                    sx={{ height: 18, fontSize: '0.65rem', '& .MuiChip-label': { px: 0.75 } }}
                  />
                </Box>
              </Box>
              {op.message && (
                <Typography variant="caption" color="text.secondary" display="block" noWrap title={op.message}>
                  {op.message}
                </Typography>
              )}
              {op.type === 'metadata_candidate_fetch' && op.status === 'completed' && (
                <Button
                  size="small"
                  variant="outlined"
                  sx={{ mt: 0.5, textTransform: 'none', fontSize: '0.7rem', py: 0, minHeight: 22 }}
                  onClick={(e) => {
                    e.stopPropagation();
                    e.preventDefault();
                    setAnchorEl(null);
                    window.location.href = `/library?reviewOp=${op.id}`;
                  }}
                >
                  Review Results
                </Button>
              )}
              {(op.type === 'organize' || op.type === 'scan_and_organize') && op.status === 'completed' && (
                <Button
                  size="small"
                  variant="outlined"
                  color="warning"
                  sx={{ mt: 0.5, textTransform: 'none', fontSize: '0.7rem', py: 0, minHeight: 22 }}
                  onClick={async (e) => {
                    e.stopPropagation();
                    e.preventDefault();
                    try {
                      const preflight = await getUndoPreflight(op.id);
                      const conflicts = (preflight.content_changed?.length || 0) +
                        (preflight.book_deleted?.length || 0) +
                        (preflight.re_organized?.length || 0);
                      const msg = conflicts > 0
                        ? `${preflight.safe} changes can be undone. ${conflicts} conflict(s) detected. Proceed?`
                        : `Undo ${preflight.safe} change(s) from this operation?`;
                      if (confirm(msg)) {
                        await revertOp(op.id);
                        alert('Operation reverted successfully');
                      }
                    } catch (err: unknown) {
                      const msg = (err as { message?: string })?.message || 'Undo failed';
                      alert(msg);
                    }
                  }}
                >
                  Undo
                </Button>
              )}
            </Box>
          ))}
        </Box>
      </Popover>
    </>
  );
}
