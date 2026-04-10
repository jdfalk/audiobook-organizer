// file: web/src/pages/ActivityLog.tsx
// version: 2.2.0
// guid: b2c3d4e5-f6a7-8901-bcde-f12345678901

import React, { useCallback, useEffect, useRef, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import {
  Box,
  Button,
  Chip,
  CircularProgress,
  Collapse,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  IconButton,
  LinearProgress,
  Menu,
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
  useMediaQuery,
  useTheme,
} from '@mui/material';
import RefreshIcon from '@mui/icons-material/Refresh.js';
import PushPinIcon from '@mui/icons-material/PushPin.js';
import TimelineIcon from '@mui/icons-material/Timeline.js';
import ClearIcon from '@mui/icons-material/Clear.js';
import UndoIcon from '@mui/icons-material/Undo.js';
import CancelIcon from '@mui/icons-material/Cancel.js';
import FilterListIcon from '@mui/icons-material/FilterList.js';
import { fetchActivity, fetchActivitySources, compactActivityLog } from '../services/activityApi';
import type { ActivityEntry, SourceCount } from '../services/activityApi';
import * as api from '../services/api';

const PAGE_SIZE_OPTIONS = [25, 50, 100, 250];

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
  digest: '#00897b',
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

const formatTimestampCompact = (ts: string): string => {
  const d = new Date(ts);
  if (isNaN(d.getTime())) return ts;
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
};

export default function ActivityLog() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down('sm'));

  // Filters
  const [search, setSearch] = useState('');
  const [tiers, setTiers] = useState<Set<string>>(new Set(['audit', 'change', 'digest']));
  const [typeFilter, setTypeFilter] = useState('');
  const [levelFilter, setLevelFilter] = useState('');
  const [operationId, setOperationId] = useState('');
  const [sinceFilter, setSinceFilter] = useState('');
  const [untilFilter, setUntilFilter] = useState('');
  const [excludedSources, setExcludedSources] = useState<Set<string>>(() => {
    const saved = localStorage.getItem('activity-source-prefs');
    return saved ? new Set(JSON.parse(saved)) : new Set();
  });

  // Mobile filter collapse
  const [filtersExpanded, setFiltersExpanded] = useState(false);

  // Active ops
  const [activeOps, setActiveOps] = useState<api.ActiveOperationSummary[]>([]);
  const [pinned, setPinned] = useState(() => localStorage.getItem('activity-ops-pinned') !== 'false');
  const [cancelling, setCancelling] = useState<Set<string>>(new Set());
  const [expandedOpId, setExpandedOpId] = useState<string | null>(searchParams.get('op'));
  const [opLogs, setOpLogs] = useState<string[]>([]);
  const opLogsRef = useRef<HTMLDivElement>(null);

  // Sources
  const [sources, setSources] = useState<SourceCount[]>([]);
  const [sourcesOpen, setSourcesOpen] = useState(false);

  // Feed
  const [entries, setEntries] = useState<ActivityEntry[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(50);
  const [loading, setLoading] = useState(false);

  // Auto-refresh
  const [autoRefresh, setAutoRefresh] = useState(true);

  // Compact
  const [compactAnchor, setCompactAnchor] = useState<null | HTMLElement>(null);
  const [compacting, setCompacting] = useState(false);
  const [customCompactDays, setCustomCompactDays] = useState('');
  const [expandedDigests, setExpandedDigests] = useState<Set<number>>(new Set());

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

  // Load logs for expanded operation
  useEffect(() => {
    if (!expandedOpId) {
      setOpLogs([]);
      return;
    }
    let cancelled = false;
    const fetchLogs = async () => {
      try {
        const logs = await api.getOperationLogs(expandedOpId);
        if (!cancelled) {
          setOpLogs(logs.map((l: { message?: string }) => l.message || String(l)));
          setTimeout(() => opLogsRef.current?.scrollTo({ top: opLogsRef.current.scrollHeight }), 50);
        }
      } catch {
        if (!cancelled) setOpLogs(['Failed to load logs']);
      }
    };
    fetchLogs();
    const interval = setInterval(fetchLogs, 3000);
    return () => { cancelled = true; clearInterval(interval); };
  }, [expandedOpId]);

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
      const allTiers = ['audit', 'change', 'debug', 'digest'];
      const inactiveTiers = allTiers.filter((t) => !tiers.has(t));
      const excludeTiersStr = inactiveTiers.length > 0 ? inactiveTiers.join(',') : undefined;

      const result = await fetchActivity({
        limit: pageSize,
        offset: (p - 1) * pageSize,
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

  // Load feed on page or pageSize change
  useEffect(() => {
    loadFeed(page);
  }, [page, pageSize, loadFeed]);

  // Auto-refresh feed — 5s when active ops exist, 30s when idle
  const refreshInterval = activeOps.length > 0 ? 5000 : 30000;
  useEffect(() => {
    if (feedIntervalRef.current) window.clearInterval(feedIntervalRef.current);
    if (autoRefresh) {
      feedIntervalRef.current = window.setInterval(() => {
        loadFeed(page);
        loadSources();
      }, refreshInterval);
    }
    return () => {
      if (feedIntervalRef.current) window.clearInterval(feedIntervalRef.current);
    };
  }, [autoRefresh, page, refreshInterval, loadFeed, loadSources]);

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

  const handleCompact = async (days: number) => {
    setCompactAnchor(null);
    setCompacting(true);
    try {
      const result = await compactActivityLog(days);
      alert(`Compacted ${result.days_compacted} days, removed ${result.entries_deleted.toLocaleString()} entries`);
      loadFeed(page);
    } catch (err) {
      alert(`Compaction failed: ${err}`);
    } finally {
      setCompacting(false);
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
    tiers.size !== 3 ||
    !tiers.has('audit') ||
    !tiers.has('change') ||
    !tiers.has('digest') ||
    typeFilter !== '' ||
    levelFilter !== '' ||
    operationId !== '' ||
    sinceFilter !== '' ||
    untilFilter !== '' ||
    excludedSources.size > 0;

  // Count active non-search filters (for mobile badge)
  const activeFilterCount = [
    tiers.size !== 3 || !tiers.has('audit') || !tiers.has('change') || !tiers.has('digest'),
    typeFilter !== '',
    levelFilter !== '',
    operationId !== '',
    sinceFilter !== '',
    untilFilter !== '',
    excludedSources.size > 0,
  ].filter(Boolean).length;

  const clearFilters = () => {
    setSearch('');
    setTiers(new Set(['audit', 'change', 'digest']));
    setTypeFilter('');
    setLevelFilter('');
    setOperationId('');
    setSinceFilter('');
    setUntilFilter('');
    setExcludedSources(new Set());
  };

  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const showOpsSection = pinned || activeOps.length > 0;

  // Shared filter controls (used in both mobile collapsed and desktop layouts)
  const tierChips = (
    <Stack direction="row" spacing={1} flexWrap="wrap">
      {['audit', 'change', 'debug', 'digest'].map((tier) => (
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
  );

  const sourcesButton = (fullWidth?: boolean) => (
    <Box sx={{ position: 'relative', width: fullWidth ? '100%' : undefined }} ref={sourcesDropdownRef}>
      <Button
        size="small"
        variant="outlined"
        onClick={() => setSourcesOpen(!sourcesOpen)}
        fullWidth={fullWidth}
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
            <Button size="small" onClick={() => setExcludedSources(new Set())}>All</Button>
            <Button size="small" onClick={() => setExcludedSources(new Set(sources.map((s) => s.source)))}>None</Button>
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
  );

  return (
    <Box sx={{ height: '100%', overflow: 'auto', p: 2 }}>
      {/* Header */}
      <Stack direction="row" alignItems="center" spacing={2} sx={{ mb: 2 }}>
        <TimelineIcon />
        <Typography variant="h4" sx={{ flexGrow: 1 }}>
          Activity
        </Typography>
        {!isMobile && (
          <Button
            size="small"
            variant={autoRefresh ? 'contained' : 'outlined'}
            onClick={() => setAutoRefresh(!autoRefresh)}
          >
            {autoRefresh ? 'Auto-refresh ON' : 'Auto-refresh OFF'}
          </Button>
        )}
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
                  <Paper
                    key={op.id}
                    variant="outlined"
                    sx={{
                      p: 1.5,
                      cursor: 'pointer',
                      border: expandedOpId === op.id ? 2 : 1,
                      borderColor: expandedOpId === op.id ? 'primary.main' : 'divider',
                    }}
                    onClick={() => setExpandedOpId(expandedOpId === op.id ? null : op.id)}
                  >
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
                        onClick={(e) => { e.stopPropagation(); handleCancelOp(op.id); }}
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
                    <Collapse in={expandedOpId === op.id}>
                      <Box
                        ref={expandedOpId === op.id ? opLogsRef : undefined}
                        sx={{
                          mt: 1,
                          maxHeight: 300,
                          overflowY: 'auto',
                          bgcolor: 'grey.900',
                          color: 'grey.300',
                          borderRadius: 1,
                          p: 1,
                          fontFamily: 'monospace',
                          fontSize: '0.75rem',
                          lineHeight: 1.4,
                        }}
                        onClick={(e) => e.stopPropagation()}
                      >
                        {opLogs.length === 0 ? (
                          <Typography variant="caption" color="grey.500">Loading logs...</Typography>
                        ) : (
                          opLogs.map((line, i) => (
                            <Box key={i} sx={{ whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>{line}</Box>
                          ))
                        )}
                      </Box>
                    </Collapse>
                  </Paper>
                );
              })}
            </Stack>
          )}
        </Paper>
      )}

      {/* Compound Filter Bar */}
      <Paper sx={{ p: 2, mb: 2 }}>
        {isMobile ? (
          /* ---- Mobile layout ---- */
          <Stack spacing={1.5}>
            {/* Search always visible */}
            <TextField
              size="small"
              placeholder="Search summaries..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              fullWidth
            />

            {/* Toggle row */}
            <Stack direction="row" alignItems="center" justifyContent="space-between">
              <Button
                size="small"
                variant="outlined"
                startIcon={<FilterListIcon />}
                onClick={() => setFiltersExpanded(!filtersExpanded)}
                endIcon={
                  activeFilterCount > 0 ? (
                    <Chip
                      size="small"
                      label={activeFilterCount}
                      color="primary"
                      sx={{ height: 18, fontSize: '0.65rem' }}
                    />
                  ) : undefined
                }
              >
                Filters
              </Button>
              <Stack direction="row" alignItems="center" spacing={1}>
                <Typography variant="caption" color="text.secondary">
                  {total} entries
                </Typography>
                {hasActiveFilters && (
                  <IconButton size="small" onClick={clearFilters} title="Clear filters">
                    <ClearIcon fontSize="small" />
                  </IconButton>
                )}
              </Stack>
            </Stack>

            {/* Collapsible filters */}
            <Collapse in={filtersExpanded}>
              <Stack spacing={1.5}>
                {/* Tier chips */}
                {tierChips}

                {/* Type dropdown */}
                <TextField
                  select
                  size="small"
                  label="Type"
                  value={typeFilter}
                  onChange={(e) => setTypeFilter(e.target.value)}
                  fullWidth
                >
                  <MenuItem value="">All Types</MenuItem>
                  {EVENT_TYPES.map((t) => (
                    <MenuItem key={t} value={t}>
                      {t.replace(/_/g, ' ')}
                    </MenuItem>
                  ))}
                </TextField>

                {/* Level dropdown */}
                <TextField
                  select
                  size="small"
                  label="Level"
                  value={levelFilter}
                  onChange={(e) => setLevelFilter(e.target.value)}
                  fullWidth
                >
                  <MenuItem value="">All Levels</MenuItem>
                  <MenuItem value="debug">debug</MenuItem>
                  <MenuItem value="info">info</MenuItem>
                  <MenuItem value="warn">warn</MenuItem>
                  <MenuItem value="error">error</MenuItem>
                </TextField>

                {/* Date range */}
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
                  fullWidth
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
                  fullWidth
                />

                {/* Sources */}
                {sourcesButton(true)}

                {/* Compact button */}
                <Button
                  size="small"
                  variant="outlined"
                  disabled={compacting}
                  onClick={(e) => setCompactAnchor(e.currentTarget)}
                  fullWidth
                >
                  {compacting ? 'Compacting…' : 'Compact'}
                </Button>
                <Menu
                  anchorEl={compactAnchor}
                  open={Boolean(compactAnchor)}
                  onClose={() => { setCompactAnchor(null); setCustomCompactDays(''); }}
                >
                  <MenuItem onClick={() => handleCompact(0)}>Everything (now)</MenuItem>
                  {[3, 7, 14, 30, 60].map((days) => (
                    <MenuItem key={days} onClick={() => handleCompact(days)}>
                      Older than {days} days
                    </MenuItem>
                  ))}
                  <MenuItem disableRipple sx={{ '&:hover': { bgcolor: 'transparent' } }}>
                    <TextField
                      size="small"
                      type="number"
                      placeholder="Custom days"
                      value={customCompactDays}
                      onChange={(e) => setCustomCompactDays(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') {
                          const n = parseInt(customCompactDays, 10);
                          if (n > 0) handleCompact(n);
                        }
                        e.stopPropagation();
                      }}
                      onClick={(e) => e.stopPropagation()}
                      sx={{ width: 120 }}
                      InputProps={{ inputProps: { min: 0 } }}
                    />
                  </MenuItem>
                </Menu>

                {/* Auto-refresh (moved here on mobile) */}
                <Button
                  size="small"
                  variant={autoRefresh ? 'contained' : 'outlined'}
                  onClick={() => setAutoRefresh(!autoRefresh)}
                  fullWidth
                >
                  {autoRefresh ? 'Auto-refresh ON' : 'Auto-refresh OFF'}
                </Button>
              </Stack>
            </Collapse>
          </Stack>
        ) : (
          /* ---- Desktop layout (unchanged) ---- */
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
              {tierChips}
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
              {sourcesButton()}

              {/* Compact button */}
              <Button
                size="small"
                variant="outlined"
                disabled={compacting}
                onClick={(e) => setCompactAnchor(e.currentTarget)}
              >
                {compacting ? 'Compacting…' : 'Compact'}
              </Button>
              <Menu
                anchorEl={compactAnchor}
                open={Boolean(compactAnchor)}
                onClose={() => { setCompactAnchor(null); setCustomCompactDays(''); }}
              >
                <MenuItem onClick={() => handleCompact(0)}>Everything (now)</MenuItem>
                {[3, 7, 14, 30, 60].map((days) => (
                  <MenuItem key={days} onClick={() => handleCompact(days)}>
                    Older than {days} days
                  </MenuItem>
                ))}
                <MenuItem disableRipple sx={{ '&:hover': { bgcolor: 'transparent' } }}>
                  <TextField
                    size="small"
                    type="number"
                    placeholder="Custom days"
                    value={customCompactDays}
                    onChange={(e) => setCustomCompactDays(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        const n = parseInt(customCompactDays, 10);
                        if (n > 0) handleCompact(n);
                      }
                      e.stopPropagation();
                    }}
                    onClick={(e) => e.stopPropagation()}
                    sx={{ width: 120 }}
                    InputProps={{ inputProps: { min: 0 } }}
                  />
                </MenuItem>
              </Menu>
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
        )}
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
                {!isMobile && <TableCell>Source</TableCell>}
                {!isMobile && <TableCell>Tags</TableCell>}
                <TableCell />
              </TableRow>
            </TableHead>
            <TableBody>
              {entries.map((entry) => {
                if (entry.tier === 'digest') {
                  const isExpanded = expandedDigests.has(Number(entry.id));
                  const details = entry.details as {
                    date?: string;
                    original_count?: number;
                    counts?: Record<string, number>;
                    items?: Array<{ type: string; book?: string; book_id?: string; summary: string; details?: string }>;
                    truncated?: boolean;
                    truncated_count?: number;
                  } | undefined;
                  const counts = details?.counts || {};
                  const items = details?.items || [];

                  return (
                    <React.Fragment key={entry.id}>
                      <TableRow
                        hover
                        sx={{ bgcolor: 'rgba(0, 137, 123, 0.06)', cursor: 'pointer' }}
                        onClick={() => {
                          setExpandedDigests((prev) => {
                            const next = new Set(prev);
                            if (next.has(Number(entry.id))) next.delete(Number(entry.id));
                            else next.add(Number(entry.id));
                            return next;
                          });
                        }}
                      >
                        <TableCell sx={{ whiteSpace: 'nowrap', color: 'text.secondary', fontSize: '0.75rem' }}>
                          {details?.date || entry.timestamp}
                        </TableCell>
                        <TableCell>
                          <Chip size="small" label="digest" sx={{ bgcolor: '#00897b', color: 'white' }} />
                        </TableCell>
                        <TableCell>
                          <Stack direction="row" spacing={0.5} flexWrap="wrap">
                            {Object.entries(counts).slice(0, 6).map(([type, count]) => (
                              <Chip key={type} size="small" variant="outlined" label={`${count} ${type.replace(/_/g, ' ')}`} />
                            ))}
                          </Stack>
                        </TableCell>
                        <TableCell colSpan={isMobile ? 1 : 2}>
                          <Typography variant="body2">
                            {entry.summary} {isExpanded ? '▾' : '▸'}
                          </Typography>
                        </TableCell>
                        {!isMobile && <TableCell />}
                        <TableCell />
                      </TableRow>
                      {isExpanded && (
                        <TableRow>
                          <TableCell colSpan={isMobile ? 5 : 7} sx={{ py: 0, px: 2 }}>
                            <Box sx={{ maxHeight: 400, overflow: 'auto', py: 1 }}>
                              {items.map((item, idx) => (
                                <Stack
                                  key={idx}
                                  direction="row"
                                  spacing={1}
                                  alignItems="center"
                                  sx={{
                                    py: 0.5,
                                    borderBottom: '1px solid',
                                    borderColor: 'divider',
                                    color: item.type === 'error' ? 'error.main' : 'text.primary',
                                  }}
                                >
                                  <Chip size="small" label={item.type.replace(/_/g, ' ')} sx={{ minWidth: 100 }} />
                                  {item.book_id ? (
                                    <Typography
                                      variant="body2"
                                      component="span"
                                      sx={{ cursor: 'pointer', color: 'primary.main', fontWeight: 500 }}
                                      onClick={(e: React.MouseEvent) => { e.stopPropagation(); navigate(`/library/${item.book_id}`); }}
                                    >
                                      {item.book || item.book_id}
                                    </Typography>
                                  ) : (
                                    <Typography variant="body2" component="span" sx={{ fontWeight: 500 }}>
                                      {item.book || '—'}
                                    </Typography>
                                  )}
                                  <Typography variant="body2" color="text.secondary" sx={{ flex: 1 }}>
                                    {item.summary}
                                  </Typography>
                                  {item.details && (
                                    <Typography variant="caption" color="error.main">
                                      {item.details}
                                    </Typography>
                                  )}
                                </Stack>
                              ))}
                              {details?.truncated && (
                                <Typography variant="caption" color="text.secondary" sx={{ pt: 1, display: 'block' }}>
                                  … and {details.truncated_count?.toLocaleString()} more entries not shown
                                </Typography>
                              )}
                            </Box>
                          </TableCell>
                        </TableRow>
                      )}
                    </React.Fragment>
                  );
                }

                // Regular entry
                return (
                  <TableRow
                    key={entry.id}
                    hover
                    sx={{
                      bgcolor: rowBgColor(entry),
                      opacity: entry.tier === 'debug' ? 0.6 : 1,
                    }}
                  >
                    <TableCell sx={{ whiteSpace: 'nowrap', color: 'text.secondary', fontSize: '0.75rem' }}>
                      {isMobile ? formatTimestampCompact(entry.timestamp) : formatTimestamp(entry.timestamp)}
                    </TableCell>
                    <TableCell>{levelChip(entry.level)}</TableCell>
                    <TableCell>
                      <Chip size="small" label={(entry.type || '').replace(/_/g, ' ')} />
                    </TableCell>
                    <TableCell sx={{ maxWidth: isMobile ? 180 : 400 }}>
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
                    {!isMobile && (
                      <TableCell>
                        <Typography variant="caption" color="text.secondary">
                          {entry.source}
                        </Typography>
                      </TableCell>
                    )}
                    {!isMobile && (
                      <TableCell>
                        {entry.tags && entry.tags.length > 0 ? (
                          <Stack direction="row" spacing={0.5} flexWrap="wrap">
                            {entry.tags.map((tag) => (
                              <Chip key={tag} size="small" label={tag} variant="outlined" />
                            ))}
                          </Stack>
                        ) : null}
                      </TableCell>
                    )}
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
                );
              })}
            </TableBody>
          </Table>
        )}

        <Stack direction="row" justifyContent="center" alignItems="center" spacing={2} sx={{ py: 2 }}>
          {totalPages > 1 && (
            <Pagination
              count={totalPages}
              page={page}
              onChange={(_, p) => setPage(p)}
              color="primary"
              size={isMobile ? 'small' : 'medium'}
            />
          )}
          <TextField
            select
            size="small"
            value={pageSize}
            onChange={(e) => { setPageSize(Number(e.target.value)); setPage(1); }}
            sx={{ minWidth: 90 }}
          >
            {PAGE_SIZE_OPTIONS.map((n) => (
              <MenuItem key={n} value={n}>{n} / page</MenuItem>
            ))}
          </TextField>
        </Stack>
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
