// file: web/src/pages/ActivityLog.tsx
// version: 2.0.0
// guid: b2c3d4e5-f6a7-8901-bcde-f12345678901

import { useCallback, useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Box,
  Button,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  IconButton,
  LinearProgress,
  MenuItem,
  Pagination,
  Paper,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material';
import RefreshIcon from '@mui/icons-material/Refresh.js';
import PushPinIcon from '@mui/icons-material/PushPin.js';
import TimelineIcon from '@mui/icons-material/Timeline.js';
import ClearIcon from '@mui/icons-material/Clear.js';
import UndoIcon from '@mui/icons-material/Undo.js';
import CancelIcon from '@mui/icons-material/Cancel.js';
import { fetchActivity, fetchActivitySources } from '../services/activityApi';
import type { ActivityEntry, SourceCount } from '../services/activityApi';
import * as api from '../services/api';

const PAGE_SIZE = 50;

const EVENT_TYPES = [
  'book_added',
  'book_updated',
  'book_deleted',
  'book_restored',
  'tag_written',
  'metadata_applied',
  'scan_started',
  'scan_completed',
  'organize_started',
  'organize_completed',
  'import_started',
  'import_completed',
  'maintenance_run',
  'config_changed',
  'user_action',
];

const TIER_COLORS: Record<string, string> = {
  audit: '#1976d2',
  change: '#9c27b0',
  debug: '#757575',
};

function levelChip(level: string) {
  const colorMap: Record<string, 'error' | 'warning' | 'info' | 'success' | 'default'> = {
    error: 'error',
    warn: 'warning',
    warning: 'warning',
    info: 'info',
    debug: 'default',
  };
  return (
    <Chip
      size="small"
      label={level}
      color={colorMap[level] ?? 'default'}
      variant="outlined"
    />
  );
}

function rowBgColor(entry: ActivityEntry): string | undefined {
  if (entry.level === 'error') return 'rgba(211, 47, 47, 0.08)';
  if (entry.level === 'warn' || entry.level === 'warning') return 'rgba(237, 108, 2, 0.08)';
  if (entry.summary.startsWith('\u2713')) return 'rgba(46, 125, 50, 0.08)';
  return undefined;
}

const formatTimestamp = (ts: string): string => {
  const d = new Date(ts);
  if (isNaN(d.getTime())) return ts;
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
};

export default function ActivityLog() {
  const navigate = useNavigate();

  // Filters
  const [search, setSearch] = useState('');
  const [tiers, setTiers] = useState<Set<string>>(new Set(['audit', 'change']));
  const [typeFilter, setTypeFilter] = useState('');
  const [levelFilter, setLevelFilter] = useState('');
  const [operationId, setOperationId] = useState('');
  const [sinceFilter, setSinceFilter] = useState('');
  const [untilFilter, setUntilFilter] = useState('');
  const [excludedSources, setExcludedSources] = useState<Set<string>>(() => {
    const saved = localStorage.getItem('activity-source-prefs');
    return saved ? new Set(JSON.parse(saved)) : new Set();
  });

  // Active ops
  const [activeOps, setActiveOps] = useState<api.ActiveOperationSummary[]>([]);
  const [pinned, setPinned] = useState(() => localStorage.getItem('activity-ops-pinned') !== 'false');
  const [cancelling, setCancelling] = useState<Set<string>>(new Set());

  // Sources
  const [sources, setSources] = useState<SourceCount[]>([]);
  const [sourcesOpen, setSourcesOpen] = useState(false);

  // Feed
  const [entries, setEntries] = useState<ActivityEntry[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(false);

  // Auto-refresh
  const [autoRefresh, setAutoRefresh] = useState(true);

  // Revert dialog
  const [revertEntry, setRevertEntry] = useState<ActivityEntry | null>(null);
  const [reverting, setReverting] = useState(false);

  // Refs for intervals
  const opsIntervalRef = useRef<number | null>(null);
  const feedIntervalRef = useRef<number | null>(null);
  const sourcesDropdownRef = useRef<HTMLDivElement | null>(null);

  // Persist excluded sources
  useEffect(() => {
    localStorage.setItem('activity-source-prefs', JSON.stringify([...excludedSources]));
  }, [excludedSources]);

  // Persist pin state
  useEffect(() => {
    localStorage.setItem('activity-ops-pinned', String(pinned));
  }, [pinned]);

  // Load active operations
  const loadActiveOps = useCallback(async () => {
    try {
      const ops = await api.getActiveOperations();
      setActiveOps(ops);
    } catch (err) {
      console.error('Failed to load active operations', err);
    }
  }, []);

  // Load sources
  const loadSources = useCallback(async () => {
    try {
      const data = await fetchActivitySources({
        since: sinceFilter || undefined,
        until: untilFilter || undefined,
      });
      setSources(data.sources || []);
    } catch (err) {
      console.error('Failed to load sources', err);
    }
  }, [sinceFilter, untilFilter]);

  // Load activity feed
  const loadFeed = useCallback(async (p: number) => {
    setLoading(true);
    try {
      const excludeStr = excludedSources.size > 0 ? [...excludedSources].join(',') : undefined;

      // Server-side tier filtering via exclude_tiers
      const allTiers = ['audit', 'change', 'debug'];
      const inactiveTiers = allTiers.filter((t) => !tiers.has(t));
      const excludeTiersStr = inactiveTiers.length > 0 ? inactiveTiers.join(',') : undefined;

      const result = await fetchActivity({
        limit: PAGE_SIZE,
        offset: (p - 1) * PAGE_SIZE,
        type: typeFilter || undefined,
        level: levelFilter || undefined,
        operation_id: operationId.trim() || undefined,
        since: sinceFilter || undefined,
        until: untilFilter || undefined,
        search: search.trim() || undefined,
        exclude_sources: excludeStr,
        exclude_tiers: excludeTiersStr,
      });

      setEntries(result.entries || []);
      setTotal(result.total || 0);
    } catch (err) {
      console.error('Failed to load activity', err);
      setEntries([]);
      setTotal(0);
    } finally {
      setLoading(false);
    }
  }, [typeFilter, levelFilter, operationId, sinceFilter, untilFilter, search, excludedSources, tiers]);

  // Initial load + polling for active ops (3s)
  useEffect(() => {
    loadActiveOps();
    opsIntervalRef.current = window.setInterval(loadActiveOps, 3000);
    return () => {
      if (opsIntervalRef.current) window.clearInterval(opsIntervalRef.current);
    };
  }, [loadActiveOps]);

  // Load feed when filters change
  useEffect(() => {
    setPage(1);
    loadFeed(1);
    loadSources();
  }, [typeFilter, levelFilter, operationId, sinceFilter, untilFilter, search, excludedSources, tiers, loadFeed, loadSources]);

  // Load feed on page change
  useEffect(() => {
    loadFeed(page);
  }, [page, loadFeed]);

  // Auto-refresh feed (30s)
  useEffect(() => {
    if (feedIntervalRef.current) window.clearInterval(feedIntervalRef.current);
    if (autoRefresh) {
      feedIntervalRef.current = window.setInterval(() => {
        loadFeed(page);
        loadSources();
      }, 30000);
    }
    return () => {
      if (feedIntervalRef.current) window.clearInterval(feedIntervalRef.current);
    };
  }, [autoRefresh, page, loadFeed, loadSources]);

  // Close sources dropdown on outside click
  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (sourcesDropdownRef.current && !sourcesDropdownRef.current.contains(e.target as Node)) {
        setSourcesOpen(false);
      }
    };
    if (sourcesOpen) {
      document.addEventListener('mousedown', handler);
    }
    return () => document.removeEventListener('mousedown', handler);
  }, [sourcesOpen]);

  const handleRefresh = () => {
    loadFeed(page);
    loadActiveOps();
    loadSources();
  };

  const handleCancelOp = async (opId: string) => {
    setCancelling((prev) => new Set(prev).add(opId));
    try {
      await api.cancelOperation(opId);
      await loadActiveOps();
    } catch (err) {
      console.error('Failed to cancel operation', err);
    }
    setCancelling((prev) => {
      const next = new Set(prev);
      next.delete(opId);
      return next;
    });
  };

  const handleClearStale = async () => {
    try {
      await api.clearStaleOperations();
      await loadActiveOps();
    } catch (err) {
      console.error('Failed to clear stale operations', err);
    }
  };

  const handleRevert = async () => {
    if (!revertEntry?.operation_id) return;
    setReverting(true);
    try {
      await api.revertOperation(revertEntry.operation_id);
      loadFeed(page);
    } catch (err) {
      console.error('Failed to revert operation', err);
    } finally {
      setReverting(false);
      setRevertEntry(null);
    }
  };

  const toggleTier = (tier: string) => {
    setTiers((prev) => {
      const next = new Set(prev);
      if (next.has(tier)) {
        next.delete(tier);
      } else {
        next.add(tier);
      }
      return next;
    });
  };

  const hasActiveFilters =
    search !== '' ||
    tiers.size !== 2 ||
    !tiers.has('audit') ||
    !tiers.has('change') ||
    typeFilter !== '' ||
    levelFilter !== '' ||
    operationId !== '' ||
    sinceFilter !== '' ||
    untilFilter !== '' ||
    excludedSources.size > 0;

  const clearFilters = () => {
    setSearch('');
    setTiers(new Set(['audit', 'change']));
    setTypeFilter('');
    setLevelFilter('');
    setOperationId('');
    setSinceFilter('');
    setUntilFilter('');
    setExcludedSources(new Set());
  };

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));
  const showOpsSection = pinned || activeOps.length > 0;

  return (
    <Box sx={{ height: '100%', overflow: 'auto', p: 2 }}>
      {/* Header */}
      <Stack direction="row" alignItems="center" spacing={2} sx={{ mb: 2 }}>
        <TimelineIcon />
        <Typography variant="h4" sx={{ flexGrow: 1 }}>
          Activity
        </Typography>
        <Button
          size="small"
          variant={autoRefresh ? 'contained' : 'outlined'}
          onClick={() => setAutoRefresh(!autoRefresh)}
        >
          {autoRefresh ? 'Auto-refresh ON' : 'Auto-refresh OFF'}
        </Button>
        <IconButton onClick={handleRefresh} title="Refresh">
          <RefreshIcon />
        </IconButton>
      </Stack>

      {/* Pinned Operations Section */}
      {showOpsSection && (
        <Paper sx={{ p: 2, mb: 2 }}>
          <Stack direction="row" alignItems="center" justifyContent="space-between" sx={{ mb: 1 }}>
            <Stack direction="row" alignItems="center" spacing={1}>
              <Typography variant="h6">
                Active Operations ({activeOps.length})
              </Typography>
              <Tooltip title={pinned ? 'Unpin section' : 'Pin section'}>
                <IconButton
                  size="small"
                  onClick={() => setPinned(!pinned)}
                  color={pinned ? 'primary' : 'default'}
                >
                  <PushPinIcon fontSize="small" />
                </IconButton>
              </Tooltip>
            </Stack>
            <Button size="small" variant="outlined" onClick={handleClearStale}>
              Clear Stale
            </Button>
          </Stack>

          {activeOps.length === 0 ? (
            <Typography variant="body2" color="text.secondary">
              No active operations.
            </Typography>
          ) : (
            <Stack spacing={1.5}>
              {activeOps.map((op) => {
                const pct = op.total > 0 ? Math.round((op.progress / op.total) * 100) : 0;
                return (
                  <Paper key={op.id} variant="outlined" sx={{ p: 1.5 }}>
                    <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ mb: 0.5 }}>
                      <Stack direction="row" spacing={1} alignItems="center">
                        <Typography variant="subtitle2" fontWeight="bold">
                          {op.type.replace(/_/g, ' ')}
                        </Typography>
                        <Chip size="small" label={op.status} color="info" />
                      </Stack>
                      <Button
                        size="small"
                        color="error"
                        variant="outlined"
                        startIcon={<CancelIcon />}
                        onClick={() => handleCancelOp(op.id)}
                        disabled={cancelling.has(op.id)}
                      >
                        {cancelling.has(op.id) ? 'Cancelling...' : 'Cancel'}
                      </Button>
                    </Stack>
                    {op.total > 0 ? (
                      <Box>
                        <LinearProgress variant="determinate" value={pct} sx={{ height: 6, borderRadius: 1, mb: 0.5 }} />
                        <Typography variant="caption" color="text.secondary">
                          {op.progress.toLocaleString()} / {op.total.toLocaleString()} ({pct}%)
                        </Typography>
                      </Box>
                    ) : (
                      <LinearProgress sx={{ height: 6, borderRadius: 1, mb: 0.5 }} />
                    )}
                    <Typography variant="caption" color="text.secondary" display="block" noWrap title={op.message}>
                      {op.message}
                    </Typography>
                  </Paper>
                );
              })}
            </Stack>
          )}
        </Paper>
      )}

      {/* Compound Filter Bar */}
      <Paper sx={{ p: 2, mb: 2 }}>
        <Stack spacing={1.5}>
          {/* Row 1: Search + tier chips */}
          <Stack direction="row" spacing={2} alignItems="center" flexWrap="wrap">
            <TextField
              size="small"
              placeholder="Search summaries..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              sx={{ minWidth: 220 }}
            />
            {['audit', 'change', 'debug'].map((tier) => (
              <Chip
                key={tier}
                label={tiers.has(tier) ? `\u2713 ${tier}` : tier}
                onClick={() => toggleTier(tier)}
                variant={tiers.has(tier) ? 'filled' : 'outlined'}
                sx={{
                  borderColor: tiers.has(tier) ? TIER_COLORS[tier] : undefined,
                  borderWidth: tiers.has(tier) ? 2 : 1,
                  color: tiers.has(tier) ? TIER_COLORS[tier] : undefined,
                  fontWeight: tiers.has(tier) ? 'bold' : 'normal',
                  cursor: 'pointer',
                }}
              />
            ))}
          </Stack>

          {/* Row 2: Type, Level, dates, sources */}
          <Stack direction="row" spacing={2} alignItems="center" flexWrap="wrap">
            <TextField
              select
              size="small"
              label="Type"
              value={typeFilter}
              onChange={(e) => setTypeFilter(e.target.value)}
              sx={{ minWidth: 180 }}
            >
              <MenuItem value="">All Types</MenuItem>
              {EVENT_TYPES.map((t) => (
                <MenuItem key={t} value={t}>
                  {t.replace(/_/g, ' ')}
                </MenuItem>
              ))}
            </TextField>

            <TextField
              select
              size="small"
              label="Level"
              value={levelFilter}
              onChange={(e) => setLevelFilter(e.target.value)}
              sx={{ minWidth: 140 }}
            >
              <MenuItem value="">All Levels</MenuItem>
              <MenuItem value="debug">debug</MenuItem>
              <MenuItem value="info">info</MenuItem>
              <MenuItem value="warn">warn</MenuItem>
              <MenuItem value="error">error</MenuItem>
            </TextField>

            <TextField
              size="small"
              label="Since"
              type={sinceFilter ? 'datetime-local' : 'text'}
              placeholder="All time"
              value={sinceFilter}
              onFocus={(e) => { if (!sinceFilter) (e.target as HTMLInputElement).type = 'datetime-local'; }}
              onChange={(e) => setSinceFilter(e.target.value)}
              InputLabelProps={{ shrink: true }}
              InputProps={sinceFilter ? {
                endAdornment: <IconButton size="small" onClick={() => setSinceFilter('')}><ClearIcon fontSize="small" /></IconButton>,
              } : undefined}
              sx={{ minWidth: 180 }}
            />

            <TextField
              size="small"
              label="Until"
              type={untilFilter ? 'datetime-local' : 'text'}
              placeholder="Now"
              value={untilFilter}
              onFocus={(e) => { if (!untilFilter) (e.target as HTMLInputElement).type = 'datetime-local'; }}
              onChange={(e) => setUntilFilter(e.target.value)}
              InputLabelProps={{ shrink: true }}
              InputProps={untilFilter ? {
                endAdornment: <IconButton size="small" onClick={() => setUntilFilter('')}><ClearIcon fontSize="small" /></IconButton>,
              } : undefined}
              sx={{ minWidth: 180 }}
            />

            {/* Sources dropdown */}
            <Box sx={{ position: 'relative' }} ref={sourcesDropdownRef}>
              <Button
                size="small"
                variant="outlined"
                onClick={() => setSourcesOpen(!sourcesOpen)}
              >
                Sources
                {excludedSources.size > 0 && (
                  <Chip
                    size="small"
                    label={`-${excludedSources.size}`}
                    color="warning"
                    sx={{ ml: 0.5, height: 20, fontSize: '0.7rem' }}
                  />
                )}
              </Button>
              {sourcesOpen && (
                <Paper
                  elevation={8}
                  sx={{
                    position: 'absolute',
                    top: '100%',
                    left: 0,
                    zIndex: 10,
                    minWidth: 280,
                    maxHeight: 400,
                    overflow: 'auto',
                    mt: 0.5,
                    p: 1,
                  }}
                >
                  {sources.length === 0 ? (
                    <Typography variant="body2" color="text.secondary" sx={{ p: 1 }}>
                      No sources found.
                    </Typography>
                  ) : (
                    sources.map((s) => {
                      const isExcluded = excludedSources.has(s.source);
                      return (
                        <Stack
                          key={s.source}
                          direction="row"
                          alignItems="center"
                          spacing={1}
                          sx={{
                            px: 1,
                            py: 0.5,
                            cursor: 'pointer',
                            '&:hover': { bgcolor: 'action.hover' },
                          }}
                          onClick={() => {
                            setExcludedSources((prev) => {
                              const next = new Set(prev);
                              if (isExcluded) {
                                next.delete(s.source);
                              } else {
                                next.add(s.source);
                              }
                              return next;
                            });
                          }}
                        >
                          <input type="checkbox" checked={!isExcluded} readOnly style={{ pointerEvents: 'none' }} />
                          <Typography
                            variant="body2"
                            sx={{
                              textDecoration: isExcluded ? 'line-through' : 'none',
                              opacity: isExcluded ? 0.5 : 1,
                              flexGrow: 1,
                            }}
                          >
                            {s.source}
                          </Typography>
                          <Typography variant="caption" color="text.secondary">
                            {s.count}
                          </Typography>
                        </Stack>
                      );
                    })
                  )}
                  <Stack direction="row" spacing={1} sx={{ mt: 1, pt: 1, borderTop: '1px solid', borderColor: 'divider' }}>
                    <Button
                      size="small"
                      onClick={() => setExcludedSources(new Set())}
                    >
                      All
                    </Button>
                    <Button
                      size="small"
                      onClick={() => setExcludedSources(new Set(sources.map((s) => s.source)))}
                    >
                      None
                    </Button>
                    <Button
                      size="small"
                      onClick={() => {
                        setExcludedSources(new Set());
                        localStorage.removeItem('activity-source-prefs');
                      }}
                    >
                      Reset
                    </Button>
                  </Stack>
                </Paper>
              )}
            </Box>
          </Stack>

          {/* Row 3: Active filter summary */}
          <Stack direction="row" spacing={1} alignItems="center">
            <Typography variant="caption" color="text.secondary">
              {total} entries
            </Typography>
            {hasActiveFilters && (
              <Button size="small" startIcon={<ClearIcon />} onClick={clearFilters}>
                Clear filters
              </Button>
            )}
          </Stack>
        </Stack>
      </Paper>

      {/* Activity Feed */}
      <Paper>
        {loading ? (
          <Box sx={{ display: 'flex', justifyContent: 'center', py: 6 }}>
            <CircularProgress />
          </Box>
        ) : entries.length === 0 ? (
          <Typography
            variant="body2"
            color="text.secondary"
            sx={{ py: 4, textAlign: 'center' }}
          >
            {operationId
              ? 'No activity entries for this operation (pre-migration).'
              : 'No activity entries found.'}
          </Typography>
        ) : (
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>Time</TableCell>
                <TableCell>Level</TableCell>
                <TableCell>Type</TableCell>
                <TableCell sx={{ width: '40%' }}>Summary</TableCell>
                <TableCell>Source</TableCell>
                <TableCell>Tags</TableCell>
                <TableCell />
              </TableRow>
            </TableHead>
            <TableBody>
              {entries.map((entry) => (
                <TableRow
                  key={entry.id}
                  hover
                  sx={{
                    bgcolor: rowBgColor(entry),
                    opacity: entry.tier === 'debug' ? 0.6 : 1,
                  }}
                >
                  <TableCell sx={{ whiteSpace: 'nowrap', color: 'text.secondary', fontSize: '0.75rem' }}>
                    {formatTimestamp(entry.timestamp)}
                  </TableCell>
                  <TableCell>{levelChip(entry.level)}</TableCell>
                  <TableCell>
                    <Chip size="small" label={(entry.type || '').replace(/_/g, ' ')} />
                  </TableCell>
                  <TableCell sx={{ maxWidth: 400 }}>
                    <Typography variant="body2" noWrap title={entry.summary}>
                      {entry.summary}
                    </Typography>
                    {entry.operation_id && !operationId && (
                      <Typography
                        variant="caption"
                        sx={{ cursor: 'pointer', color: 'primary.main' }}
                        onClick={() => setOperationId(entry.operation_id!)}
                      >
                        view operation &rarr;
                      </Typography>
                    )}
                    {entry.book_id && (
                      <Typography
                        variant="caption"
                        sx={{ cursor: 'pointer', color: 'primary.main', ml: 1 }}
                        onClick={() => navigate(`/library/${entry.book_id}`)}
                      >
                        book &rarr;
                      </Typography>
                    )}
                  </TableCell>
                  <TableCell>
                    <Typography variant="caption" color="text.secondary">
                      {entry.source}
                    </Typography>
                  </TableCell>
                  <TableCell>
                    {entry.tags && entry.tags.length > 0 ? (
                      <Stack direction="row" spacing={0.5} flexWrap="wrap">
                        {entry.tags.map((tag) => (
                          <Chip key={tag} size="small" label={tag} variant="outlined" />
                        ))}
                      </Stack>
                    ) : null}
                  </TableCell>
                  <TableCell>
                    {entry.operation_id &&
                      (entry.type === 'organize_completed' || entry.type === 'metadata_applied') && (
                        <Tooltip title="Revert operation">
                          <IconButton
                            size="small"
                            onClick={() => setRevertEntry(entry)}
                          >
                            <UndoIcon fontSize="small" />
                          </IconButton>
                        </Tooltip>
                      )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}

        {totalPages > 1 && (
          <Box sx={{ display: 'flex', justifyContent: 'center', py: 2 }}>
            <Pagination
              count={totalPages}
              page={page}
              onChange={(_, p) => setPage(p)}
              color="primary"
            />
          </Box>
        )}
      </Paper>

      {/* Revert Confirmation Dialog */}
      <Dialog open={!!revertEntry} onClose={() => setRevertEntry(null)}>
        <DialogTitle>Revert Operation?</DialogTitle>
        <DialogContent>
          <Typography variant="body2">
            This will undo all tracked changes from operation{' '}
            <strong>{revertEntry?.operation_id?.slice(0, 12)}...</strong>.
            This cannot be undone.
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setRevertEntry(null)}>Cancel</Button>
          <Button
            color="warning"
            variant="contained"
            onClick={handleRevert}
            disabled={reverting}
          >
            {reverting ? 'Reverting...' : 'Revert'}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
