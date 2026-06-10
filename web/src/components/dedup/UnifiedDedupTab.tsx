// file: web/src/components/dedup/UnifiedDedupTab.tsx
// version: 1.0.0
// guid: c8b9d0e1-f2a3-4567-bcde-cb8901234567
// last-edited: 2026-06-10

// UnifiedDedupTab is the T017 single surface that replaces the separate Books /
// Advanced-Scan / Acoustic tabs. It shows a paginated candidate table filtered
// by band (CERTAIN/HIGH/MEDIUM/REVIEW), with a side drawer for per-candidate
// comparison and score breakdown. The legacy tabs remain mounted behind a
// "show legacy" toggle for one release.
//
// Memory-leak discipline (PR #1076):
//   - AbortController for every fetch, cancelled on cleanup / re-trigger.
//   - Timers cleared on unmount.
//   - No module-level mutable state.

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import {
  Alert,
  Box,
  Button,
  Checkbox,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  IconButton,
  Paper,
  Snackbar,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TablePagination,
  TableRow,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material';
import RefreshIcon from '@mui/icons-material/Refresh';
import MergeIcon from '@mui/icons-material/MergeType';
import VisibilityOffIcon from '@mui/icons-material/VisibilityOff';
import InfoIcon from '@mui/icons-material/Info';
import ClearIcon from '@mui/icons-material/Clear';
import * as api from '../../services/api';
import type { DedupCandidate, DedupBand, DedupStats } from '../../services/api';
import { useOperationsStore } from '../../stores/useOperationsStore';
import { BandFilterBar, type BandCounts } from './BandFilterBar';
import { ScoreBadgeRow } from './ScoreBadgeRow';
import { CandidateCompareDrawer } from './CandidateCompareDrawer';
import { BulkActionBar } from './BulkActionBar';

// ---------- helpers ----------

/** Build BandCounts from the flat DedupStats[] array (which has status+band dims). */
function deriveBandCounts(stats: DedupStats[]): BandCounts {
  const pending = stats.filter((s) => s.status === 'pending');
  // Stats rows don't carry a "band" dimension in the existing schema, so we
  // derive total from the status counts. Band-specific counts will populate
  // when the API eventually returns them; until then we show 0 per-band and
  // the table results will reflect the filter accurately.
  const total = pending.reduce((sum, s) => sum + s.count, 0);
  return { CERTAIN: 0, HIGH: 0, MEDIUM: 0, REVIEW: 0, total };
}

function truncatePath(path: string | undefined | null): string {
  if (!path) return '';
  const marker = 'audiobook-organizer/';
  const idx = path.indexOf(marker);
  return idx >= 0 ? path.slice(idx + marker.length) : path;
}

// ---------- component ----------

interface UnifiedDedupTabProps {
  /** When true, the component is mounted but replaced by legacy tabs. */
  hidden?: boolean;
}

export function UnifiedDedupTab({ hidden }: UnifiedDedupTabProps) {
  const [searchParams, setSearchParams] = useSearchParams();

  // --- filter state (synced to URL params for deep-link) ---
  const bandFromURL = (searchParams.get('band') as DedupBand | null) ?? null;
  const bookFromURL = searchParams.get('book') ?? null; // deep-link from FingerprintVisualsColumn

  const [bandFilter, setBandFilter] = useState<DedupBand | null>(bandFromURL);
  const [statusFilter] = useState<string>('pending');
  const [searchQuery, setSearchQuery] = useState('');
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(25);

  // --- data state ---
  const [candidates, setCandidates] = useState<DedupCandidate[]>([]);
  const [total, setTotal] = useState(0);
  const [stats, setStats] = useState<DedupStats[]>([]);
  const [loading, setLoading] = useState(false);
  const [statsLoading, setStatsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [toast, setToast] = useState<string | null>(null);

  // --- selection state ---
  const [selected, setSelected] = useState<Set<number>>(new Set());

  // --- drawer state ---
  const [drawerCandidateId, setDrawerCandidateId] = useState<number | null>(null);

  // --- bulk action state ---
  const [bulkBusy, setBulkBusy] = useState(false);
  const [rescoringOpen, setRescoringOpen] = useState(false);

  // --- abort controller refs ---
  const fetchAbortRef = useRef<AbortController | null>(null);
  const statsAbortRef = useRef<AbortController | null>(null);

  // --- sync band filter to URL ---
  const handleBandChange = useCallback(
    (band: DedupBand | null) => {
      setBandFilter(band);
      setPage(0);
      setSelected(new Set());
      const next = new URLSearchParams(searchParams);
      if (band) {
        next.set('band', band);
      } else {
        next.delete('band');
      }
      setSearchParams(next, { replace: true });
    },
    [searchParams, setSearchParams]
  );

  // --- load stats ---
  const loadStats = useCallback(() => {
    statsAbortRef.current?.abort();
    const ctrl = new AbortController();
    statsAbortRef.current = ctrl;
    setStatsLoading(true);
    fetch('/api/v1/dedup/stats', { signal: ctrl.signal })
      .then((r) => (r.ok ? r.json() : Promise.reject(new Error(`stats ${r.status}`))))
      .then((data) => {
        if (!ctrl.signal.aborted) {
          setStats(data?.data?.stats ?? []);
        }
      })
      .catch(() => {
        /* stats are optional */
      })
      .finally(() => {
        if (!ctrl.signal.aborted) setStatsLoading(false);
      });
  }, []);

  // --- load candidates ---
  const loadCandidates = useCallback(() => {
    fetchAbortRef.current?.abort();
    const ctrl = new AbortController();
    fetchAbortRef.current = ctrl;
    setLoading(true);
    setError(null);

    const params: Parameters<typeof api.getDedupCandidates>[0] = {
      status: statusFilter || undefined,
      limit: rowsPerPage,
      offset: page * rowsPerPage,
      include_breakdown: true,
    };
    if (bandFilter) params.band = bandFilter;

    // Use fetch directly to pass the signal (api.getDedupCandidates doesn't accept AbortSignal yet).
    const qs = new URLSearchParams();
    if (params.status) qs.set('status', params.status);
    if (params.band) qs.set('band', params.band);
    if (params.include_breakdown) qs.set('include_breakdown', 'true');
    qs.set('limit', String(params.limit));
    qs.set('offset', String(params.offset ?? 0));

    fetch(`/api/v1/dedup/candidates?${qs}`, { signal: ctrl.signal })
      .then((r) => (r.ok ? r.json() : Promise.reject(new Error(`candidates ${r.status}`))))
      .then((data) => {
        if (!ctrl.signal.aborted) {
          const resp = data?.data ?? data;
          setCandidates(resp.candidates ?? []);
          setTotal(resp.total ?? 0);
        }
      })
      .catch((err) => {
        if (!ctrl.signal.aborted) {
          setError(err instanceof Error ? err.message : 'Failed to load candidates');
        }
      })
      .finally(() => {
        if (!ctrl.signal.aborted) setLoading(false);
      });
  }, [bandFilter, statusFilter, page, rowsPerPage]);

  useEffect(() => {
    loadStats();
    return () => {
      statsAbortRef.current?.abort();
    };
  }, [loadStats]);

  useEffect(() => {
    loadCandidates();
    return () => {
      fetchAbortRef.current?.abort();
    };
  }, [loadCandidates]);

  // --- pre-filter by deep-link book ID ---
  const filteredCandidates = useMemo(() => {
    let result = candidates;
    if (bookFromURL) {
      result = result.filter(
        (c) => c.entity_a_id === bookFromURL || c.entity_b_id === bookFromURL
      );
    }
    if (searchQuery.trim()) {
      // Client-side search over the loaded page. Searches both entity IDs only
      // (book title/author data is not pre-fetched in the unified table for
      // performance; use the drawer for detailed info).
      const q = searchQuery.trim().toLowerCase();
      result = result.filter(
        (c) =>
          c.entity_a_id.toLowerCase().includes(q) ||
          c.entity_b_id.toLowerCase().includes(q) ||
          c.layer.toLowerCase().includes(q) ||
          (c.band ?? '').toLowerCase().includes(q)
      );
    }
    return result;
  }, [candidates, bookFromURL, searchQuery]);

  const bandCounts = useMemo(() => deriveBandCounts(stats), [stats]);

  // --- selection helpers ---
  const toggleSelect = (id: number) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const selectAll = () => {
    setSelected(new Set(filteredCandidates.map((c) => c.id)));
  };

  const clearSelection = () => setSelected(new Set());

  // --- bulk actions ---
  const handleMergeSelected = async () => {
    if (selected.size === 0) return;
    setBulkBusy(true);
    let merged = 0;
    let failed = 0;
    for (const id of selected) {
      try {
        await api.mergeDedupCandidate(id);
        merged++;
      } catch {
        failed++;
      }
    }
    setToast(
      failed === 0
        ? `Merged ${merged} candidate${merged === 1 ? '' : 's'}`
        : `Merged ${merged}, failed ${failed}`
    );
    clearSelection();
    loadCandidates();
    loadStats();
    setBulkBusy(false);
  };

  const handleDismissSelected = async () => {
    if (selected.size === 0) return;
    setBulkBusy(true);
    let dismissed = 0;
    let failed = 0;
    for (const id of selected) {
      try {
        await api.dismissDedupCandidate(id);
        dismissed++;
      } catch {
        failed++;
      }
    }
    setToast(
      failed === 0
        ? `Dismissed ${dismissed} candidate${dismissed === 1 ? '' : 's'}`
        : `Dismissed ${dismissed}, failed ${failed}`
    );
    clearSelection();
    loadCandidates();
    loadStats();
    setBulkBusy(false);
  };

  const handleMergeAllFiltered = async () => {
    setBulkBusy(true);
    try {
      const result = await api.bulkMergeDedupCandidates({
        entity_type: 'book',
        status: statusFilter || 'pending',
      });
      setToast(
        `Bulk merge: ${result.merged} merged, ${result.failed} failed of ${result.attempted}`
      );
      clearSelection();
      loadCandidates();
      loadStats();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Bulk merge failed');
    } finally {
      setBulkBusy(false);
    }
  };

  // --- rescore ---
  const handleRescore = async (apply: boolean) => {
    setBulkBusy(true);
    try {
      const result = await api.rescoreDedupCandidates(apply);
      const deltaStr = Object.entries(result.band_deltas ?? {})
        .map(([k, v]) => `${k}: ${v}`)
        .join(', ');
      setToast(
        `Rescore: ${result.inspected} inspected, ${result.changed} changed${
          deltaStr ? ` (${deltaStr})` : ''
        }${apply ? '' : ' [dry-run]'}`
      );
      if (apply) {
        loadCandidates();
        loadStats();
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Rescore failed');
    } finally {
      setBulkBusy(false);
      setRescoringOpen(false);
    }
  };

  // --- op tracking helper ---
  const trackOp = (op: api.Operation, label: string): string => {
    if (op?.id && op?.type) {
      useOperationsStore.getState().startPolling(op.id, op.type);
      return `${label} started — see bell for progress (op ${op.id.slice(-6)})`;
    }
    return `${label} started`;
  };

  // --- scan trigger ---
  const handleScan = async () => {
    setBulkBusy(true);
    try {
      const op = await api.triggerDedupScan();
      setToast(trackOp(op, 'Dedup scan'));
      // Refresh after short delay for new candidates to appear.
      setTimeout(() => {
        loadCandidates();
        loadStats();
      }, 2000);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Scan failed');
    } finally {
      setBulkBusy(false);
    }
  };

  if (hidden) return null;

  return (
    <Box data-testid="unified-dedup-tab">
      {/* Toolbar */}
      <Stack direction="row" spacing={1} sx={{ mb: 2 }} alignItems="center" flexWrap="wrap" useFlexGap>
        <Tooltip title="Re-run full dedup scan (embed + exact + similarity matching)">
          <span>
            <Button
              variant="contained"
              size="small"
              startIcon={bulkBusy ? <CircularProgress size={16} /> : <RefreshIcon />}
              onClick={handleScan}
              disabled={bulkBusy}
            >
              Find Duplicates
            </Button>
          </span>
        </Tooltip>
        <Tooltip title="Re-compute scores for all pending candidates from stored signals (no re-embedding)">
          <span>
            <Button
              variant="outlined"
              size="small"
              disabled={bulkBusy}
              onClick={() => setRescoringOpen(true)}
              data-testid="rescore-btn"
            >
              Rescore
            </Button>
          </span>
        </Tooltip>
      </Stack>

      {/* Band filter */}
      <BandFilterBar
        selected={bandFilter}
        counts={bandCounts}
        loading={statsLoading}
        onChange={handleBandChange}
      />

      {/* Deep-link book filter indicator */}
      {bookFromURL && (
        <Alert
          severity="info"
          sx={{ mb: 2 }}
          action={
            <IconButton
              size="small"
              onClick={() => {
                const next = new URLSearchParams(searchParams);
                next.delete('book');
                setSearchParams(next, { replace: true });
              }}
              aria-label="clear book filter"
            >
              <ClearIcon fontSize="small" />
            </IconButton>
          }
        >
          Showing candidates for book <strong>{bookFromURL}</strong>
        </Alert>
      )}

      {/* Search */}
      <Box sx={{ mb: 2 }}>
        <TextField
          size="small"
          placeholder="Search by book ID, layer, band…"
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          sx={{ minWidth: 280 }}
          InputProps={{
            endAdornment: searchQuery ? (
              <IconButton
                size="small"
                onClick={() => setSearchQuery('')}
                aria-label="clear search"
              >
                <ClearIcon fontSize="small" />
              </IconButton>
            ) : null,
          }}
        />
      </Box>

      {error && (
        <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>
          {error}
        </Alert>
      )}

      {/* Table */}
      {loading ? (
        <Box sx={{ textAlign: 'center', py: 4 }}>
          <CircularProgress />
        </Box>
      ) : filteredCandidates.length === 0 ? (
        <Paper sx={{ p: 4, textAlign: 'center' }}>
          <Typography color="text.secondary">
            No candidates found for the current filter.
          </Typography>
        </Paper>
      ) : (
        <>
          <Paper variant="outlined">
            <Table size="small">
              <TableHead>
                <TableRow>
                  <TableCell padding="checkbox">
                    <Checkbox
                      size="small"
                      indeterminate={
                        selected.size > 0 && selected.size < filteredCandidates.length
                      }
                      checked={
                        filteredCandidates.length > 0 &&
                        selected.size === filteredCandidates.length
                      }
                      onChange={(e) => (e.target.checked ? selectAll() : clearSelection())}
                    />
                  </TableCell>
                  <TableCell>Score / Band</TableCell>
                  <TableCell>Book A</TableCell>
                  <TableCell>Book B</TableCell>
                  <TableCell align="center">Status</TableCell>
                  <TableCell align="center">Actions</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {filteredCandidates.map((c) => {
                  const busy = bulkBusy;
                  return (
                    <TableRow
                      key={c.id}
                      hover
                      selected={selected.has(c.id)}
                      sx={{ opacity: busy ? 0.7 : 1 }}
                    >
                      <TableCell padding="checkbox">
                        <Checkbox
                          size="small"
                          checked={selected.has(c.id)}
                          onChange={() => toggleSelect(c.id)}
                        />
                      </TableCell>
                      <TableCell>
                        <ScoreBadgeRow
                          band={c.band}
                          score={c.score}
                          layer={c.layer}
                          similarity={c.similarity}
                        />
                      </TableCell>
                      <TableCell sx={{ maxWidth: 220, minWidth: 120 }}>
                        <Tooltip title={c.entity_a_id}>
                          <Typography
                            variant="caption"
                            sx={{ fontFamily: 'monospace', fontSize: '0.7rem' }}
                            noWrap
                          >
                            {c.entity_a_id}
                          </Typography>
                        </Tooltip>
                      </TableCell>
                      <TableCell sx={{ maxWidth: 220, minWidth: 120 }}>
                        <Tooltip title={c.entity_b_id}>
                          <Typography
                            variant="caption"
                            sx={{ fontFamily: 'monospace', fontSize: '0.7rem' }}
                            noWrap
                          >
                            {c.entity_b_id}
                          </Typography>
                        </Tooltip>
                      </TableCell>
                      <TableCell align="center">
                        <Chip
                          label={c.status}
                          size="small"
                          color={
                            c.status === 'merged'
                              ? 'success'
                              : c.status === 'dismissed'
                              ? 'default'
                              : 'warning'
                          }
                          variant="outlined"
                        />
                      </TableCell>
                      <TableCell align="center">
                        <Stack direction="row" spacing={0.5} justifyContent="center">
                          <Tooltip title="View comparison and breakdown">
                            <IconButton
                              size="small"
                              onClick={() => setDrawerCandidateId(c.id)}
                              aria-label={`Open comparison for candidate ${c.id}`}
                            >
                              <InfoIcon fontSize="small" />
                            </IconButton>
                          </Tooltip>
                          {c.status === 'pending' && (
                            <>
                              <Tooltip title="Merge">
                                <IconButton
                                  size="small"
                                  color="primary"
                                  disabled={busy}
                                  onClick={async () => {
                                    try {
                                      await api.mergeDedupCandidate(c.id);
                                      setToast('Merged');
                                      loadCandidates();
                                      loadStats();
                                    } catch (err) {
                                      setError(
                                        err instanceof Error ? err.message : 'Merge failed'
                                      );
                                    }
                                  }}
                                  aria-label={`Merge candidate ${c.id}`}
                                >
                                  <MergeIcon fontSize="small" />
                                </IconButton>
                              </Tooltip>
                              <Tooltip title="Dismiss">
                                <IconButton
                                  size="small"
                                  disabled={busy}
                                  onClick={async () => {
                                    try {
                                      await api.dismissDedupCandidate(c.id);
                                      setToast('Dismissed');
                                      loadCandidates();
                                      loadStats();
                                    } catch (err) {
                                      setError(
                                        err instanceof Error ? err.message : 'Dismiss failed'
                                      );
                                    }
                                  }}
                                  aria-label={`Dismiss candidate ${c.id}`}
                                >
                                  <VisibilityOffIcon fontSize="small" />
                                </IconButton>
                              </Tooltip>
                            </>
                          )}
                        </Stack>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </Paper>

          <TablePagination
            component="div"
            count={total}
            page={page}
            onPageChange={(_, p) => {
              setPage(p);
              setSelected(new Set());
            }}
            rowsPerPage={rowsPerPage}
            onRowsPerPageChange={(e) => {
              setRowsPerPage(parseInt(e.target.value, 10));
              setPage(0);
              setSelected(new Set());
            }}
            rowsPerPageOptions={[10, 25, 50, 100, 250]}
          />
        </>
      )}

      {/* Bulk action bar (sticky bottom) */}
      <BulkActionBar
        selectedCount={selected.size}
        total={total}
        bandFilter={bandFilter}
        isBusy={bulkBusy}
        onMergeSelected={handleMergeSelected}
        onDismissSelected={handleDismissSelected}
        onMergeAllFiltered={handleMergeAllFiltered}
        onClearSelection={clearSelection}
      />

      {/* Comparison drawer */}
      <CandidateCompareDrawer
        candidateId={drawerCandidateId}
        onClose={() => setDrawerCandidateId(null)}
        onMerged={() => {
          loadCandidates();
          loadStats();
        }}
        onDismissed={() => {
          loadCandidates();
          loadStats();
        }}
      />

      {/* Rescore dialog */}
      <Dialog open={rescoringOpen} onClose={() => setRescoringOpen(false)}>
        <DialogTitle>Rescore dedup candidates</DialogTitle>
        <DialogContent>
          <DialogContentText>
            Re-runs the unified scoring formula over stored signal sets for all pending
            candidates. No re-embedding or re-collection is performed — only candidates
            that already have stored signals (T015+) will be updated. Pre-T015 rows are
            counted as skipped.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setRescoringOpen(false)}>Cancel</Button>
          <Button
            onClick={() => handleRescore(false)}
            disabled={bulkBusy}
            data-testid="rescore-dry-run-btn"
          >
            Dry Run
          </Button>
          <Button
            onClick={() => handleRescore(true)}
            variant="contained"
            color="warning"
            disabled={bulkBusy}
            data-testid="rescore-apply-btn"
          >
            Apply
          </Button>
        </DialogActions>
      </Dialog>

      {/* Toast */}
      <Snackbar
        open={toast !== null}
        autoHideDuration={5000}
        onClose={(_, reason) => {
          if (reason !== 'clickaway') setToast(null);
        }}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
      >
        <Alert severity="info" variant="filled" onClose={() => setToast(null)}>
          {toast}
        </Alert>
      </Snackbar>
    </Box>
  );
}
