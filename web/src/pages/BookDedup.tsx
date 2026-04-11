// file: web/src/pages/BookDedup.tsx
// version: 3.9.0
// guid: c3d4e5f6-a7b8-9c0d-1e2f-book0dedup02

import { useState, useEffect, useCallback, useMemo } from 'react';
import { useSearchParams, useNavigate } from 'react-router-dom';
import {
  Box,
  Typography,
  Paper,
  Button,
  Alert,
  Chip,
  CircularProgress,
  Divider,
  IconButton,
  Tooltip,
  Card,
  CardContent,
  CardActions,
  Stack,
  Radio,
  RadioGroup,
  FormControlLabel,
  Tab,
  Tabs,
  Badge,
  LinearProgress,
  Checkbox,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogContentText,
  DialogActions,
  TextField,
  TablePagination,
  Popover,
  Drawer,
  Switch,
} from '@mui/material';
import MergeIcon from '@mui/icons-material/MergeType';
import RefreshIcon from '@mui/icons-material/Refresh';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import FolderIcon from '@mui/icons-material/Folder';
import MenuBookIcon from '@mui/icons-material/MenuBook';
import PersonIcon from '@mui/icons-material/Person';
import ListIcon from '@mui/icons-material/List';
import EditIcon from '@mui/icons-material/Edit';
import SaveIcon from '@mui/icons-material/Save';
import CloseIcon from '@mui/icons-material/Close';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import VisibilityOffIcon from '@mui/icons-material/VisibilityOff';
import * as api from '../services/api';
import type { Book, AuthorDedupGroup, SeriesDupGroup, ValidationResult, Operation, BookDedupGroup, DedupCandidate, DedupStats } from '../services/api';
import SearchIcon from '@mui/icons-material/Search';
import AutoAwesomeIcon from '@mui/icons-material/AutoAwesome';
import Collapse from '@mui/material/Collapse';
import MicIcon from '@mui/icons-material/Mic';
import BusinessIcon from '@mui/icons-material/Business';
import CleaningServicesIcon from '@mui/icons-material/CleaningServices';
import type { SuggestionRoles } from '../services/api';

/** Strip "(Unabridged)", "(Abridged)", and leading "[Series X]" from display titles */
function cleanDisplayTitle(title: string): string {
  return title
    .replace(/\s*\((un)?abridged\)/gi, '')
    .replace(/^\[.*?\]\s*/g, '')
    .trim();
}

/** Structured role display for AI suggestions with role decomposition */
function RoleDetails({ roles }: { roles: SuggestionRoles }) {
  return (
    <Box sx={{ ml: 5, mt: 0.5 }}>
      {roles.author && (
        <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: 0.5, mb: 0.5 }}>
          <PersonIcon sx={{ fontSize: 16, mt: 0.3, color: 'primary.main' }} />
          <Box>
            <Typography variant="body2" component="span" sx={{ fontWeight: 500 }}>
              Author: {roles.author.name}
            </Typography>
            {roles.author.variants && roles.author.variants.length > 0 && (
              <Typography variant="caption" display="block" color="text.secondary">
                Variants: {roles.author.variants.join(', ')}
              </Typography>
            )}
            {roles.author.reason && (
              <Typography variant="caption" display="block" sx={{ fontStyle: 'italic', color: 'text.secondary' }}>
                &ldquo;{roles.author.reason}&rdquo;
              </Typography>
            )}
          </Box>
        </Box>
      )}
      {roles.narrator && (
        <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: 0.5, mb: 0.5 }}>
          <MicIcon sx={{ fontSize: 16, mt: 0.3, color: 'secondary.main' }} />
          <Box>
            <Typography variant="body2" component="span" sx={{ fontWeight: 500 }}>
              Narrator: {roles.narrator.name}
            </Typography>
            {roles.narrator.reason && (
              <Typography variant="caption" display="block" sx={{ fontStyle: 'italic', color: 'text.secondary' }}>
                &ldquo;{roles.narrator.reason}&rdquo;
              </Typography>
            )}
          </Box>
        </Box>
      )}
      {roles.publisher && (
        <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: 0.5, mb: 0.5 }}>
          <BusinessIcon sx={{ fontSize: 16, mt: 0.3, color: 'warning.main' }} />
          <Box>
            <Typography variant="body2" component="span" sx={{ fontWeight: 500 }}>
              Publisher: {roles.publisher.name}
            </Typography>
            {roles.publisher.reason && (
              <Typography variant="caption" display="block" sx={{ fontStyle: 'italic', color: 'text.secondary' }}>
                &ldquo;{roles.publisher.reason}&rdquo;
              </Typography>
            )}
          </Box>
        </Box>
      )}
    </Box>
  );
}

/** Popover showing books for a set of author IDs */
function AuthorBooksPopover({
  anchorEl,
  onClose,
  authorIds,
}: {
  anchorEl: HTMLElement | null;
  onClose: () => void;
  authorIds: number[];
}) {
  const [books, setBooks] = useState<Book[]>([]);
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();

  useEffect(() => {
    if (!anchorEl || authorIds.length === 0) return;
    let cancelled = false;
    setLoading(true);
    Promise.all(authorIds.map((id) => api.getBooksByAuthor(id)))
      .then((results) => {
        if (cancelled) return;
        // Deduplicate by book id
        const seen = new Set<string>();
        const all: Book[] = [];
        for (const list of results) {
          for (const b of list) {
            if (!seen.has(b.id)) {
              seen.add(b.id);
              all.push(b);
            }
          }
        }
        setBooks(all);
      })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [anchorEl, authorIds]);

  return (
    <Popover
      open={Boolean(anchorEl)}
      anchorEl={anchorEl}
      onClose={onClose}
      anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
      transformOrigin={{ vertical: 'top', horizontal: 'left' }}
      slotProps={{ paper: { sx: { maxWidth: 480, maxHeight: 400, overflow: 'auto', p: 1 } } }}
    >
      {loading ? (
        <Box sx={{ p: 2, textAlign: 'center' }}><CircularProgress size={24} /></Box>
      ) : books.length === 0 ? (
        <Typography sx={{ p: 2 }} variant="body2" color="text.secondary">No books found</Typography>
      ) : (
        <Stack spacing={0.5}>
          {books.map((book) => (
            <Box
              key={book.id}
              sx={{ display: 'flex', alignItems: 'center', gap: 1, p: 0.5, cursor: 'pointer',
                borderRadius: 1, '&:hover': { bgcolor: 'action.hover' } }}
              onClick={() => { onClose(); navigate(`/books/${book.id}`); }}
            >
              {book.cover_url ? (
                <Box component="img" src={book.cover_url} alt="" sx={{ width: 40, height: 56, objectFit: 'cover', borderRadius: 0.5, flexShrink: 0 }} />
              ) : (
                <Box sx={{ width: 40, height: 56, display: 'flex', alignItems: 'center', justifyContent: 'center',
                  bgcolor: 'action.selected', borderRadius: 0.5, flexShrink: 0 }}>
                  <MenuBookIcon fontSize="small" color="disabled" />
                </Box>
              )}
              <Box sx={{ overflow: 'hidden' }}>
                <Typography variant="body2" noWrap fontWeight="medium">{cleanDisplayTitle(book.title)}</Typography>
                {book.author_name && <Typography variant="caption" color="text.secondary" noWrap>{book.author_name}</Typography>}
              </Box>
            </Box>
          ))}
        </Stack>
      )}
    </Popover>
  );
}

// Shared operation progress banner
function OperationProgress({ operation, label }: { operation: Operation | null; label?: string }) {
  if (!operation || operation.status === 'completed' || operation.status === 'failed' || operation.status === 'cancelled') return null;
  const pct = operation.total > 0 ? Math.round((operation.progress / operation.total) * 100) : 0;
  return (
    <Paper sx={{ p: 2, mb: 2 }}>
      <Stack spacing={1}>
        {label && <Typography variant="caption" color="text.secondary" fontWeight="bold">{label}</Typography>}
        <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          <Typography variant="body2">{operation.message || 'Processing...'}</Typography>
          <Typography variant="caption">{pct}%</Typography>
        </Box>
        <LinearProgress variant={operation.total > 0 ? 'determinate' : 'indeterminate'} value={pct} />
      </Stack>
    </Paper>
  );
}

// Helper: start an operation and poll until done
async function runOperationWithPolling(
  startFn: () => Promise<Operation>,
  setOp: (op: Operation | null) => void,
  onComplete: (op: Operation) => void,
  onError: (msg: string) => void,
) {
  try {
    const initial = await startFn();
    setOp(initial);
    const final = await api.pollOperation(initial.id, (update) => setOp(update));
    setOp(null);
    onComplete(final);
  } catch (err) {
    setOp(null);
    onError(err instanceof Error ? err.message : 'Operation failed');
  }
}

const PAGE_SIZE_OPTIONS = [10, 25, 50, 100];

function usePagination(totalItems: number) {
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(25);

  // Reset page when total changes
  useEffect(() => { setPage(0); }, [totalItems]);

  const startIdx = page * rowsPerPage;
  const endIdx = startIdx + rowsPerPage;

  return { page, setPage, rowsPerPage, setRowsPerPage, startIdx, endIdx };
}

function PaginationControls({ total, page, rowsPerPage, onPageChange, onRowsPerPageChange }: {
  total: number;
  page: number;
  rowsPerPage: number;
  onPageChange: (page: number) => void;
  onRowsPerPageChange: (rpp: number) => void;
}) {
  if (total <= PAGE_SIZE_OPTIONS[0]) return null;
  return (
    <TablePagination
      component="div"
      count={total}
      page={page}
      onPageChange={(_, p) => onPageChange(p)}
      rowsPerPage={rowsPerPage}
      onRowsPerPageChange={(e) => { onRowsPerPageChange(parseInt(e.target.value, 10)); onPageChange(0); }}
      rowsPerPageOptions={PAGE_SIZE_OPTIONS}
      labelRowsPerPage="Groups per page:"
      sx={{ mt: 2 }}
    />
  );
}

// ---- Book Dedup Tab ----
function BookDedupTab() {
  const [groups, setGroups] = useState<Book[][]>([]);
  const [totalDuplicates, setTotalDuplicates] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [activeOp, setActiveOp] = useState<Operation | null>(null);
  const [mergeSuccess, setMergeSuccess] = useState<string | null>(null);
  const [keepSelections, setKeepSelections] = useState<Record<string, string>>({});
  const [selectedGroups, setSelectedGroups] = useState<Set<string>>(new Set());
  const [confirmOpen, setConfirmOpen] = useState(false);
  const pagination = usePagination(groups.length);

  const fetchDuplicates = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await api.getBookDuplicates();
      setGroups(data.groups || []);
      setTotalDuplicates(data.duplicate_count || 0);
      const defaults: Record<string, string> = {};
      (data.groups || []).forEach((g, i) => {
        if (g.length > 0) defaults[`group-${i}`] = g[0].id;
      });
      setKeepSelections(defaults);
      setSelectedGroups(new Set());
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch duplicates');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { fetchDuplicates(); }, [fetchDuplicates]);

  const handleMerge = async (group: Book[], groupKey: string) => {
    const keepId = keepSelections[groupKey];
    if (!keepId) return;
    const mergeIds = group.filter((b) => b.id !== keepId).map((b) => b.id);
    setMergeSuccess(null);
    await runOperationWithPolling(
      () => api.mergeBooks(keepId, mergeIds),
      setActiveOp,
      (final) => {
        if (final.status === 'failed') {
          setError(final.error_message || 'Merge failed');
        } else {
          setMergeSuccess(`Merged duplicates of "${group[0]?.title}"`);
          setGroups((prev) => prev.filter((_, i) => `group-${i}` !== groupKey));
          setSelectedGroups((prev) => { const next = new Set(prev); next.delete(groupKey); return next; });
        }
      },
      setError,
    );
  };

  const handleMergeSelected = async () => {
    setMergeSuccess(null);
    for (let i = 0; i < groups.length; i++) {
      const groupKey = `group-${i}`;
      if (!selectedGroups.has(groupKey)) continue;
      const group = groups[i];
      const keepId = keepSelections[groupKey];
      if (!keepId) continue;
      const mergeIds = group.filter((b) => b.id !== keepId).map((b) => b.id);
      try {
        const initial = await api.mergeBooks(keepId, mergeIds);
        setActiveOp(initial);
        await api.pollOperation(initial.id, (update) => setActiveOp(update));
      } catch (err) {
        setError(err instanceof Error ? err.message : `Failed to merge "${group[0]?.title}"`);
      }
    }
    setActiveOp(null);
    setMergeSuccess(`Merged ${selectedGroups.size} selected group(s)`);
    fetchDuplicates();
  };

  const handleMergeAll = async () => {
    setConfirmOpen(false);
    setMergeSuccess(null);
    for (let i = 0; i < groups.length; i++) {
      const group = groups[i];
      const groupKey = `group-${i}`;
      const keepId = keepSelections[groupKey];
      if (!keepId) continue;
      const mergeIds = group.filter((b) => b.id !== keepId).map((b) => b.id);
      try {
        const initial = await api.mergeBooks(keepId, mergeIds);
        setActiveOp(initial);
        await api.pollOperation(initial.id, (update) => setActiveOp(update));
      } catch (err) {
        setError(err instanceof Error ? err.message : `Failed to merge "${group[0]?.title}"`);
      }
    }
    setActiveOp(null);
    setMergeSuccess('Merged all duplicate books');
    fetchDuplicates();
  };

  const toggleGroup = (key: string) => {
    setSelectedGroups((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key); else next.add(key);
      return next;
    });
  };

  const toggleAll = () => {
    if (selectedGroups.size === groups.length) {
      setSelectedGroups(new Set());
    } else {
      setSelectedGroups(new Set(groups.map((_, i) => `group-${i}`)));
    }
  };

  const busy = activeOp !== null;

  return (
    <Box>
      <Box sx={{ display: 'flex', alignItems: 'center', mb: 2 }}>
        <Typography variant="body2" color="text.secondary" sx={{ flexGrow: 1 }}>
          Detects books with identical titles and authors at different file paths.
        </Typography>
        <Stack direction="row" spacing={1}>
          {groups.length > 0 && (
            <>
              <Button size="small" onClick={toggleAll} disabled={busy}>
                {selectedGroups.size === groups.length ? 'Deselect All' : 'Select All'}
              </Button>
              {selectedGroups.size > 0 && (
                <Button variant="contained" color="primary" startIcon={<MergeIcon />}
                  onClick={handleMergeSelected} disabled={busy}>
                  Merge Selected ({selectedGroups.size})
                </Button>
              )}
              <Button variant="contained" color="warning" startIcon={<MergeIcon />}
                onClick={() => setConfirmOpen(true)} disabled={busy}>
                Merge All ({totalDuplicates})
              </Button>
            </>
          )}
          <Tooltip title="Refresh"><IconButton onClick={fetchDuplicates} disabled={loading || busy}><RefreshIcon /></IconButton></Tooltip>
        </Stack>
      </Box>

      <OperationProgress operation={activeOp} />
      {error && <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>{error}</Alert>}
      {mergeSuccess && <Alert severity="success" sx={{ mb: 2 }} icon={<CheckCircleIcon />} onClose={() => setMergeSuccess(null)}>{mergeSuccess}</Alert>}

      {loading ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}><CircularProgress /></Box>
      ) : groups.length === 0 ? (
        <Paper sx={{ p: 4, textAlign: 'center' }}>
          <CheckCircleIcon sx={{ fontSize: 48, color: 'success.main', mb: 1 }} />
          <Typography variant="h6">No duplicate books found</Typography>
        </Paper>
      ) : (
        <>
        <PaginationControls total={groups.length} page={pagination.page} rowsPerPage={pagination.rowsPerPage}
          onPageChange={pagination.setPage} onRowsPerPageChange={pagination.setRowsPerPage} />
        <Stack spacing={2}>
          {groups.slice(pagination.startIdx, pagination.endIdx).map((group, sliceIdx) => {
            const idx = pagination.startIdx + sliceIdx;
            const groupKey = `group-${idx}`;
            return (
              <Card key={groupKey} variant="outlined">
                <CardContent>
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
                    <Checkbox
                      checked={selectedGroups.has(groupKey)}
                      onChange={() => toggleGroup(groupKey)}
                      disabled={busy}
                      size="small"
                    />
                    <Typography variant="subtitle1" fontWeight="bold">{cleanDisplayTitle(group[0]?.title || 'Unknown')}</Typography>
                    <Chip label={`${group.length} copies`} size="small" color="warning" variant="outlined" />
                  </Box>
                  <Divider sx={{ my: 1 }} />
                  <RadioGroup value={keepSelections[groupKey] || ''}
                    onChange={(e) => setKeepSelections((prev) => ({ ...prev, [groupKey]: e.target.value }))}>
                    {group.map((book) => (
                      <FormControlLabel key={book.id} value={book.id} control={<Radio size="small" />}
                        label={
                          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                            <FolderIcon fontSize="small" color="action" />
                            <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.8rem' }}>{book.file_path}</Typography>
                            {book.itunes_persistent_id && <Chip label="iTunes" size="small" color="info" variant="outlined" />}
                            {book.format && <Chip label={book.format} size="small" variant="outlined" />}
                          </Box>
                        } />
                    ))}
                  </RadioGroup>
                </CardContent>
                <CardActions>
                  <Button startIcon={<MergeIcon />} variant="contained" size="small"
                    onClick={() => handleMerge(group, groupKey)} disabled={busy}>
                    Merge
                  </Button>
                </CardActions>
              </Card>
            );
          })}
        </Stack>
        <PaginationControls total={groups.length} page={pagination.page} rowsPerPage={pagination.rowsPerPage}
          onPageChange={pagination.setPage} onRowsPerPageChange={pagination.setRowsPerPage} />
        </>
      )}

      <Dialog open={confirmOpen} onClose={() => setConfirmOpen(false)}>
        <DialogTitle>Confirm Merge All</DialogTitle>
        <DialogContent>
          <DialogContentText>
            This will merge {groups.length} groups. This action cannot be undone. Are you sure?
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setConfirmOpen(false)}>Cancel</Button>
          <Button onClick={handleMergeAll} color="warning" variant="contained">Confirm</Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}

// ---- Advanced Book Dedup Scan Tab ----
function BookDedupScanTab() {
  const [groups, setGroups] = useState<BookDedupGroup[]>([]);
  const [totalDuplicates, setTotalDuplicates] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [activeOp, setActiveOp] = useState<Operation | null>(null);
  const [mergeSuccess, setMergeSuccess] = useState<string | null>(null);
  const [needsRefresh, setNeedsRefresh] = useState(false);
  const [confidenceFilter, setConfidenceFilter] = useState<'all' | 'high' | 'medium' | 'low'>('all');
  const pagination = usePagination(groups.length);

  const fetchResults = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await api.getBookDedupScanResults();
      setGroups(data.groups || []);
      setTotalDuplicates(data.duplicate_count || 0);
      setNeedsRefresh(data.needs_refresh || false);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch scan results');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { fetchResults(); }, [fetchResults]);

  const handleScan = async () => {
    setMergeSuccess(null);
    await runOperationWithPolling(
      () => api.scanBookDuplicates(),
      setActiveOp,
      (final) => {
        if (final.status === 'failed') {
          setError(final.error_message || 'Scan failed');
        } else {
          setMergeSuccess('Scan complete');
          fetchResults();
        }
      },
      (msg) => setError(msg),
    );
  };

  const handleMerge = async (group: BookDedupGroup) => {
    setMergeSuccess(null);
    setError(null);
    try {
      const bookIds = group.books.map(b => b.id);
      const result = await api.mergeBookDuplicatesAsVersions(bookIds);
      setMergeSuccess(result.message);
      setGroups(prev => prev.filter(g => g.group_key !== group.group_key));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Merge failed');
    }
  };

  const handleDismiss = async (group: BookDedupGroup) => {
    setError(null);
    try {
      await api.dismissBookDuplicateGroup(group.group_key);
      setGroups(prev => prev.filter(g => g.group_key !== group.group_key));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Dismiss failed');
    }
  };

  const filteredGroups = confidenceFilter === 'all'
    ? groups
    : groups.filter(g => g.confidence === confidenceFilter);

  const confidenceCounts = useMemo(() => {
    const counts = { high: 0, medium: 0, low: 0 };
    for (const g of groups) {
      if (g.confidence in counts) counts[g.confidence as keyof typeof counts]++;
    }
    return counts;
  }, [groups]);

  const confidenceColor = (c: string) => {
    switch (c) {
      case 'high': return 'error';
      case 'medium': return 'warning';
      case 'low': return 'info';
      default: return 'default';
    }
  };

  const formatFileSize = (bytes?: number) => {
    if (!bytes) return '--';
    if (bytes > 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`;
    if (bytes > 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
    return `${(bytes / 1024).toFixed(0)} KB`;
  };

  const formatDuration = (secs?: number) => {
    if (!secs) return '--';
    const h = Math.floor(secs / 3600);
    const m = Math.floor((secs % 3600) / 60);
    return h > 0 ? `${h}h ${m}m` : `${m}m`;
  };

  const busy = activeOp !== null;

  return (
    <Box>
      <Box sx={{ display: 'flex', alignItems: 'center', mb: 2 }}>
        <Typography variant="body2" color="text.secondary" sx={{ flexGrow: 1 }}>
          Advanced duplicate detection using file hashes, folder structure, and fuzzy title/author matching.
        </Typography>
        <Stack direction="row" spacing={1}>
          <Button variant="contained" startIcon={<SearchIcon />} onClick={handleScan} disabled={busy}>
            {needsRefresh ? 'Run Scan' : 'Re-Scan'}
          </Button>
          <Tooltip title="Refresh cached results">
            <IconButton onClick={fetchResults} disabled={loading || busy}><RefreshIcon /></IconButton>
          </Tooltip>
        </Stack>
      </Box>

      <OperationProgress operation={activeOp} label="Book Duplicate Scan" />
      {error && <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>{error}</Alert>}
      {mergeSuccess && <Alert severity="success" sx={{ mb: 2 }} icon={<CheckCircleIcon />} onClose={() => setMergeSuccess(null)}>{mergeSuccess}</Alert>}

      {loading ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}><CircularProgress /></Box>
      ) : needsRefresh && groups.length === 0 ? (
        <Paper sx={{ p: 4, textAlign: 'center' }}>
          <ContentCopyIcon sx={{ fontSize: 48, color: 'text.secondary', mb: 1 }} />
          <Typography variant="h6">No scan results yet</Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            Click "Run Scan" to detect duplicate books using hashes, folder structure, and metadata matching.
          </Typography>
        </Paper>
      ) : groups.length === 0 ? (
        <Paper sx={{ p: 4, textAlign: 'center' }}>
          <CheckCircleIcon sx={{ fontSize: 48, color: 'success.main', mb: 1 }} />
          <Typography variant="h6">No duplicate books found</Typography>
        </Paper>
      ) : (
        <>
          {/* Confidence filter tabs */}
          <Tabs value={confidenceFilter} onChange={(_, v) => setConfidenceFilter(v)} sx={{ mb: 2 }}>
            <Tab value="all" label={`All (${groups.length})`} />
            <Tab value="high" label={`High (${confidenceCounts.high})`} />
            <Tab value="medium" label={`Medium (${confidenceCounts.medium})`} />
            <Tab value="low" label={`Low (${confidenceCounts.low})`} />
          </Tabs>

          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            {totalDuplicates} total duplicates across {groups.length} groups
          </Typography>

          <PaginationControls total={filteredGroups.length} page={pagination.page} rowsPerPage={pagination.rowsPerPage}
            onPageChange={pagination.setPage} onRowsPerPageChange={pagination.setRowsPerPage} />

          <Stack spacing={2}>
            {filteredGroups.slice(pagination.startIdx, pagination.endIdx).map((group) => (
              <Card key={group.group_key} variant="outlined">
                <CardContent>
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
                    <Typography variant="subtitle1" fontWeight="bold">
                      {cleanDisplayTitle(group.books[0]?.title || 'Unknown')}
                    </Typography>
                    {group.books[0]?.author_name && (
                      <Typography variant="body2" color="text.secondary">
                        by {group.books[0].author_name}
                      </Typography>
                    )}
                    <Chip label={`${group.books.length} copies`} size="small" color="warning" variant="outlined" />
                    <Chip label={group.confidence} size="small" color={confidenceColor(group.confidence) as 'error' | 'warning' | 'info' | 'default'} />
                    <Typography variant="caption" color="text.secondary">{group.reason}</Typography>
                  </Box>
                  <Divider sx={{ my: 1 }} />
                  {/* Table of duplicate books */}
                  <Box component="table" sx={{ width: '100%', borderCollapse: 'collapse', '& td, & th': { py: 0.5, px: 1, fontSize: '0.85rem' } }}>
                    <thead>
                      <tr>
                        <th style={{ textAlign: 'left' }}>File Path</th>
                        <th>Format</th>
                        <th>Bitrate</th>
                        <th>Duration</th>
                        <th>Size</th>
                      </tr>
                    </thead>
                    <tbody>
                      {group.books.map((book) => (
                        <tr key={book.id}>
                          <td>
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                              <FolderIcon fontSize="small" color="action" />
                              <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.75rem' }} noWrap title={book.file_path}>
                                {book.file_path}
                              </Typography>
                              {book.itunes_persistent_id && <Chip label="iTunes" size="small" color="info" variant="outlined" sx={{ ml: 0.5 }} />}
                            </Box>
                          </td>
                          <td style={{ textAlign: 'center' }}>
                            {book.format ? <Chip label={book.format.toUpperCase()} size="small" variant="outlined" /> : '--'}
                          </td>
                          <td style={{ textAlign: 'center' }}>
                            {book.bitrate ? `${book.bitrate} kbps` : '--'}
                          </td>
                          <td style={{ textAlign: 'center' }}>{formatDuration(book.duration)}</td>
                          <td style={{ textAlign: 'center' }}>{formatFileSize(book.file_size)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </Box>
                </CardContent>
                <CardActions>
                  <Button startIcon={<MergeIcon />} variant="contained" size="small"
                    onClick={() => handleMerge(group)} disabled={busy}>
                    Merge as Versions
                  </Button>
                  <Button startIcon={<VisibilityOffIcon />} size="small" color="inherit"
                    onClick={() => handleDismiss(group)} disabled={busy}>
                    Dismiss
                  </Button>
                </CardActions>
              </Card>
            ))}
          </Stack>

          <PaginationControls total={filteredGroups.length} page={pagination.page} rowsPerPage={pagination.rowsPerPage}
            onPageChange={pagination.setPage} onRowsPerPageChange={pagination.setRowsPerPage} />
        </>
      )}
    </Box>
  );
}

// Ensure variants arrays are never null (Go serializes nil slices as null)
function normalizeGroups(groups: AuthorDedupGroup[]): AuthorDedupGroup[] {
  return (groups || []).map((g) => ({ ...g, variants: g.variants || [] }));
}

// ---- Author Dedup Tab ----
function AuthorDedupTab() {
  const [groups, setGroups] = useState<AuthorDedupGroup[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [activeOp, setActiveOp] = useState<Operation | null>(null);
  const [mergeSuccess, setMergeSuccess] = useState<string | null>(null);
  const [selectedGroups, setSelectedGroups] = useState<Set<string>>(new Set());
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [editingCanonicalId, setEditingCanonicalId] = useState<number | null>(null);
  const [editingCanonicalName, setEditingCanonicalName] = useState('');
  const [narratorFlags, setNarratorFlags] = useState<Set<string>>(new Set()); // "authorId" or "authorId:splitName" keys
  const [removedVariants, setRemovedVariants] = useState<Set<string>>(new Set()); // "canonicalId:variantId" keys
  const [validatingAuthor, setValidatingAuthor] = useState<string | null>(null); // authorId being validated
  const [authorValidation, setAuthorValidation] = useState<Record<string, { results: { source: string; title: string; author: string }[]; query: string }>>({});
  const [popoverAnchor, setPopoverAnchor] = useState<HTMLElement | null>(null);
  const [popoverAuthorIds, setPopoverAuthorIds] = useState<number[]>([]);
  const [resolvingAuthor, setResolvingAuthor] = useState<number | null>(null);
  const pagination = usePagination(groups.length);

  const fetchDuplicates = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await api.getAuthorDuplicates();
      if (result.needs_refresh) {
        // Cache is cold — trigger async scan with progress
        await runOperationWithPolling(
          () => api.refreshAuthorDuplicates(),
          setActiveOp,
          async () => {
            const fresh = await api.getAuthorDuplicates();
            setGroups(normalizeGroups(fresh.groups));
            setSelectedGroups(new Set());
            setLoading(false);
          },
          (msg) => { setError(msg); setLoading(false); },
        );
        return;
      }
      setGroups(normalizeGroups(result.groups));
      setSelectedGroups(new Set());
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch duplicates');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { fetchDuplicates(); }, [fetchDuplicates]);

  const handleSaveCanonicalName = async (group: AuthorDedupGroup) => {
    if (!editingCanonicalName.trim()) return;
    try {
      await api.renameAuthor(group.canonical.id, editingCanonicalName.trim());
      setGroups((prev) => prev.map((g) =>
        g.canonical.id === group.canonical.id
          ? { ...g, canonical: { ...g.canonical, name: editingCanonicalName.trim() } }
          : g
      ));
      setEditingCanonicalId(null);
      setEditingCanonicalName('');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to rename author');
    }
  };

  const handleSplitAuthor = async (group: AuthorDedupGroup) => {
    try {
      // Collect which split names are narrators
      const narratorNames = (group.split_names || []).filter((n) => narratorFlags.has(`${group.canonical.id}:${n}`));
      const result = await api.splitCompositeAuthor(group.canonical.id);
      // After split, reclassify any flagged narrators
      for (const na of narratorNames) {
        const match = result.authors.find((a) => a.name === na);
        if (match) {
          try { await api.reclassifyAuthorAsNarrator(match.id); } catch { /* best effort */ }
        }
      }
      setMergeSuccess(`Split "${group.canonical.name}" into ${result.authors.length} authors`);
      fetchDuplicates();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to split author');
    }
  };

  const handleValidateAuthor = async (authorName: string, authorId: string) => {
    setValidatingAuthor(authorId);
    try {
      const resp = await api.validateDedupEntry(authorName, 'author');
      setAuthorValidation((prev) => ({ ...prev, [authorId]: { results: resp.results, query: resp.query } }));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Validation failed');
    } finally {
      setValidatingAuthor(null);
    }
  };

  const handleMerge = async (group: AuthorDedupGroup) => {
    setMergeSuccess(null);
    // Filter out removed variants and reclassify narrator-flagged ones first
    const activeVariants = group.variants.filter((v) => !removedVariants.has(`${group.canonical.id}:${v.id}`));
    const narratorVariantIds = activeVariants.filter((v) => narratorFlags.has(String(v.id))).map((v) => v.id);
    const mergeVariantIds = activeVariants.filter((v) => !narratorFlags.has(String(v.id))).map((v) => v.id);

    // Reclassify narrator-flagged variants first
    for (const nId of narratorVariantIds) {
      try { await api.reclassifyAuthorAsNarrator(nId); } catch { /* best effort */ }
    }
    if (mergeVariantIds.length === 0) {
      setMergeSuccess(`Reclassified ${narratorVariantIds.length} variant(s) as narrator`);
      fetchDuplicates();
      return;
    }

    await runOperationWithPolling(
      () => api.mergeAuthors(group.canonical.id, mergeVariantIds),
      setActiveOp,
      (final) => {
        if (final.status === 'failed') {
          setError(final.error_message || 'Merge failed');
        } else {
          setMergeSuccess(`Merged author(s) into "${group.canonical.name}"`);
          setGroups((prev) => prev.filter((g) => g.canonical.id !== group.canonical.id));
          setSelectedGroups((prev) => {
            const next = new Set(prev);
            next.delete(String(group.canonical.id));
            return next;
          });
        }
      },
      setError,
    );
  };

  const handleMergeSelected = async () => {
    setMergeSuccess(null);
    for (const group of groups) {
      const key = String(group.canonical.id);
      if (!selectedGroups.has(key)) continue;
      try {
        const initial = await api.mergeAuthors(group.canonical.id, group.variants.map((v) => v.id));
        setActiveOp(initial);
        await api.pollOperation(initial.id, (update) => setActiveOp(update));
      } catch (err) {
        setError(err instanceof Error ? err.message : `Failed to merge "${group.canonical.name}"`);
      }
    }
    setActiveOp(null);
    setMergeSuccess(`Merged ${selectedGroups.size} selected group(s)`);
    fetchDuplicates();
  };

  const handleMergeAll = async () => {
    setConfirmOpen(false);
    setMergeSuccess(null);
    for (const group of groups) {
      try {
        const initial = await api.mergeAuthors(group.canonical.id, group.variants.map((v) => v.id));
        setActiveOp(initial);
        await api.pollOperation(initial.id, (update) => setActiveOp(update));
      } catch (err) {
        setError(err instanceof Error ? err.message : `Failed to merge "${group.canonical.name}"`);
      }
    }
    setActiveOp(null);
    setMergeSuccess('Merged all duplicate authors');
    fetchDuplicates();
  };

  const toggleGroup = (key: string) => {
    setSelectedGroups((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key); else next.add(key);
      return next;
    });
  };

  const toggleAll = () => {
    if (selectedGroups.size === groups.length) {
      setSelectedGroups(new Set());
    } else {
      setSelectedGroups(new Set(groups.map((g) => String(g.canonical.id))));
    }
  };

  const busy = activeOp !== null;

  return (
    <Box>
      <OperationProgress operation={activeOp} />
      {error && <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>{error}</Alert>}
      {mergeSuccess && <Alert severity="success" sx={{ mb: 2 }} icon={<CheckCircleIcon />} onClose={() => setMergeSuccess(null)}>{mergeSuccess}</Alert>}

      <Box sx={{ display: 'flex', alignItems: 'center', mb: 2 }}>
        <Typography variant="body2" color="text.secondary" sx={{ flexGrow: 1 }}>
          Uses structured name comparison to detect author name variants like &quot;James S. A. Corey&quot; vs &quot;James S.A. Corey&quot;.
        </Typography>
        <Stack direction="row" spacing={1}>
          {groups.length > 0 && (
            <>
              <Button size="small" onClick={toggleAll} disabled={busy}>
                {selectedGroups.size === groups.length ? 'Deselect All' : 'Select All'}
              </Button>
              {selectedGroups.size > 0 && (
                <Button variant="contained" color="primary" startIcon={<MergeIcon />}
                  onClick={handleMergeSelected} disabled={busy}>
                  Merge Selected ({selectedGroups.size})
                </Button>
              )}
              <Button variant="contained" color="warning" startIcon={<MergeIcon />}
                onClick={() => setConfirmOpen(true)} disabled={busy}>
                Merge All ({groups.length})
              </Button>
            </>
          )}
          <Tooltip title="Refresh"><IconButton onClick={fetchDuplicates} disabled={loading || busy}><RefreshIcon /></IconButton></Tooltip>
        </Stack>
      </Box>

      {loading ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}><CircularProgress /></Box>
      ) : groups.length === 0 ? (
        <Paper sx={{ p: 4, textAlign: 'center' }}>
          <CheckCircleIcon sx={{ fontSize: 48, color: 'success.main', mb: 1 }} />
          <Typography variant="h6">No duplicate authors found</Typography>
        </Paper>
      ) : (
        <>
        <PaginationControls total={groups.length} page={pagination.page} rowsPerPage={pagination.rowsPerPage}
          onPageChange={pagination.setPage} onRowsPerPageChange={pagination.setRowsPerPage} />
        <Stack spacing={2}>
          {groups.slice(pagination.startIdx, pagination.endIdx).map((group) => {
            const key = String(group.canonical.id);
            return (
              <Card key={key} variant="outlined">
                <CardContent>
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
                    <Checkbox
                      checked={selectedGroups.has(key)}
                      onChange={() => toggleGroup(key)}
                      disabled={busy}
                      size="small"
                    />
                    {editingCanonicalId === group.canonical.id ? (
                      <>
                        <TextField size="small" value={editingCanonicalName}
                          onChange={(e) => setEditingCanonicalName(e.target.value)}
                          onKeyDown={(e) => { if (e.key === 'Enter') handleSaveCanonicalName(group); if (e.key === 'Escape') { setEditingCanonicalId(null); setEditingCanonicalName(''); } }}
                          autoFocus sx={{ minWidth: 300 }} />
                        <IconButton size="small" color="primary" onClick={() => handleSaveCanonicalName(group)}><SaveIcon fontSize="small" /></IconButton>
                        <IconButton size="small" onClick={() => { setEditingCanonicalId(null); setEditingCanonicalName(''); }}><CloseIcon fontSize="small" /></IconButton>
                      </>
                    ) : (
                      <>
                        <Typography variant="subtitle1" fontWeight="bold">{cleanDisplayTitle(group.canonical.name)}</Typography>
                        <Tooltip title="Edit canonical name">
                          <IconButton size="small" onClick={() => { setEditingCanonicalId(group.canonical.id); setEditingCanonicalName(group.canonical.name); }}>
                            <EditIcon fontSize="small" />
                          </IconButton>
                        </Tooltip>
                        {group.suggested_name && group.suggested_name !== group.canonical.name && (
                          <Tooltip title={`Use suggested: "${group.suggested_name}"`}>
                            <Chip label={group.suggested_name} size="small" color="info" variant="outlined"
                              onClick={() => { setEditingCanonicalId(group.canonical.id); setEditingCanonicalName(group.suggested_name!); }}
                              sx={{ cursor: 'pointer' }} />
                          </Tooltip>
                        )}
                      </>
                    )}
                    <Chip icon={<MenuBookIcon />} label={`${group.book_count} book(s)`} size="small" variant="outlined"
                      onClick={(e) => {
                        const ids = [group.canonical.id, ...group.variants.map((v) => v.id)];
                        setPopoverAuthorIds(ids);
                        setPopoverAnchor(e.currentTarget);
                      }}
                      sx={{ cursor: 'pointer' }} />
                    {group.is_production_company && (
                      <Chip label="Production Company" size="small" color="warning" />
                    )}
                  </Box>
                  {group.split_names && group.split_names.length > 1 ? (
                    <>
                      <Divider sx={{ my: 1 }} />
                      <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                        Composite author — will split into:
                      </Typography>
                      <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 1 }}>
                        {group.split_names.map((name) => {
                          const flagKey = `${group.canonical.id}:${name}`;
                          const isNarrator = narratorFlags.has(flagKey);
                          return (
                            <Box key={name} sx={{ display: 'flex', gap: 0.5, alignItems: 'center' }}>
                              <Chip label={name} color="warning" variant="outlined" size="small" />
                              <Chip
                                label={isNarrator ? 'Narrator' : 'Author'}
                                size="small"
                                color={isNarrator ? 'secondary' : 'default'}
                                variant={isNarrator ? 'filled' : 'outlined'}
                                onClick={() => setNarratorFlags((prev) => {
                                  const next = new Set(prev);
                                  if (next.has(flagKey)) next.delete(flagKey); else next.add(flagKey);
                                  return next;
                                })}
                                sx={{ cursor: 'pointer' }}
                              />
                            </Box>
                          );
                        })}
                      </Box>
                    </>
                  ) : group.variants.length > 0 ? (
                    <>
                      <Divider sx={{ my: 1 }} />
                      <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 1, alignItems: 'center', mb: 1 }}>
                        <Typography variant="body2" color="text.secondary">Merge target:</Typography>
                        <Chip label={group.canonical.name} color="primary" size="small" variant="outlined" />
                        <Typography variant="body2" color="text.secondary" sx={{ mx: 0.5 }}>←</Typography>
                        <Typography variant="body2" color="text.secondary">
                          {group.variants.filter((v) => !removedVariants.has(`${group.canonical.id}:${v.id}`)).length} variant(s) will be merged into it:
                        </Typography>
                      </Box>
                      <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 1 }}>
                        {group.variants.map((v) => {
                          const removeKey = `${group.canonical.id}:${v.id}`;
                          if (removedVariants.has(removeKey)) return null;
                          const isNarrator = narratorFlags.has(String(v.id));
                          const isSameAsCanonical = v.name === group.canonical.name;
                          return (
                            <Box key={v.id} sx={{ display: 'flex', gap: 0.5, alignItems: 'center' }}>
                              <Tooltip title={isSameAsCanonical ? `"${v.name}" is the current canonical name (ID ${v.id} will be merged)` : `Click to use "${v.name}" as the merge target (canonical spelling)`}>
                                <Chip label={v.name} color={isSameAsCanonical ? 'default' : 'warning'} variant="outlined" size="small"
                                  onClick={isSameAsCanonical ? undefined : async () => {
                                    try {
                                      await api.renameAuthor(group.canonical.id, v.name);
                                      setGroups((prev) => prev.map((g) =>
                                        g.canonical.id === group.canonical.id
                                          ? { ...g, canonical: { ...g.canonical, name: v.name } }
                                          : g
                                      ));
                                    } catch (err) {
                                      setError(err instanceof Error ? err.message : 'Failed to rename author');
                                    }
                                  }}
                                  sx={{ cursor: isSameAsCanonical ? 'default' : 'pointer' }} />
                              </Tooltip>
                              <Chip
                                label={isNarrator ? 'Narrator' : 'Author'}
                                size="small"
                                color={isNarrator ? 'secondary' : 'default'}
                                variant={isNarrator ? 'filled' : 'outlined'}
                                onClick={() => setNarratorFlags((prev) => {
                                  const next = new Set(prev);
                                  const k = String(v.id);
                                  if (next.has(k)) next.delete(k); else next.add(k);
                                  return next;
                                })}
                                sx={{ cursor: 'pointer', minWidth: 60 }}
                              />
                              <Tooltip title={`Remove "${v.name}" from this merge`}>
                                <IconButton size="small" onClick={() => setRemovedVariants((prev) => new Set(prev).add(removeKey))}
                                  sx={{ p: 0.25 }}>
                                  <CloseIcon fontSize="small" />
                                </IconButton>
                              </Tooltip>
                            </Box>
                          );
                        })}
                      </Box>
                      {/* Validate button */}
                      <Box sx={{ mt: 1 }}>
                        <Button size="small" variant="text"
                          disabled={validatingAuthor === key}
                          onClick={() => handleValidateAuthor(group.canonical.name, key)}>
                          {validatingAuthor === key ? 'Searching...' : 'Search external sources'}
                        </Button>
                        {authorValidation[key] && (
                          <Box sx={{ mt: 1 }}>
                            {authorValidation[key].results.length === 0 ? (
                              <Typography variant="caption" color="text.secondary">No external matches found</Typography>
                            ) : authorValidation[key].results.map((r, i) => (
                              <Chip key={i} label={`${r.source}: ${r.author || r.title}`} size="small" variant="outlined" sx={{ mr: 0.5, mb: 0.5 }} />
                            ))}
                          </Box>
                        )}
                      </Box>
                    </>
                  ) : null}
                </CardContent>
                <CardActions>
                  {group.is_production_company ? (
                    <Button startIcon={<SearchIcon />} variant="contained" size="small" color="warning"
                      disabled={busy || resolvingAuthor === group.canonical.id}
                      onClick={async () => {
                        try {
                          setResolvingAuthor(group.canonical.id);
                          const op = await api.resolveProductionAuthor(group.canonical.id);
                          await runOperationWithPolling(
                            () => Promise.resolve(op),
                            setActiveOp,
                            () => { fetchDuplicates(); setResolvingAuthor(null); },
                            (msg) => { setError(msg); setResolvingAuthor(null); },
                          );
                        } catch (err) {
                          setError(err instanceof Error ? err.message : 'Failed to resolve');
                          setResolvingAuthor(null);
                        }
                      }}>
                      {resolvingAuthor === group.canonical.id ? 'Resolving...' : 'Find Real Author'}
                    </Button>
                  ) : group.split_names && group.split_names.length > 1 ? (
                    <Button startIcon={<MergeIcon />} variant="contained" size="small" color="warning"
                      onClick={() => handleSplitAuthor(group)} disabled={busy}>
                      Split into {group.split_names.length} authors
                    </Button>
                  ) : (
                    <Button startIcon={<MergeIcon />} variant="contained" size="small"
                      onClick={() => handleMerge(group)} disabled={busy}>
                      {`Merge into "${group.canonical.name}"`}
                    </Button>
                  )}
                </CardActions>
              </Card>
            );
          })}
        </Stack>
        <PaginationControls total={groups.length} page={pagination.page} rowsPerPage={pagination.rowsPerPage}
          onPageChange={pagination.setPage} onRowsPerPageChange={pagination.setRowsPerPage} />
        </>
      )}

      <AuthorBooksPopover
        anchorEl={popoverAnchor}
        onClose={() => { setPopoverAnchor(null); setPopoverAuthorIds([]); }}
        authorIds={popoverAuthorIds}
      />

      <Dialog open={confirmOpen} onClose={() => setConfirmOpen(false)}>
        <DialogTitle>Confirm Merge All</DialogTitle>
        <DialogContent>
          <DialogContentText>
            This will merge {groups.length} groups. This action cannot be undone. Are you sure?
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setConfirmOpen(false)}>Cancel</Button>
          <Button onClick={handleMergeAll} color="warning" variant="contained">Confirm</Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}

// ---- Series Dedup Tab ----
function SeriesDedupTab() {
  const [groups, setGroups] = useState<SeriesDupGroup[]>([]);
  const [totalSeries, setTotalSeries] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [activeOp, setActiveOp] = useState<Operation | null>(null);
  const [mergeSuccess, setMergeSuccess] = useState<string | null>(null);
  const [keepSelections, setKeepSelections] = useState<Record<string, number[]>>({});
  const [selectedGroups, setSelectedGroups] = useState<Set<string>>(new Set());
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [editingSeriesId, setEditingSeriesId] = useState<number | null>(null);
  const [editingName, setEditingName] = useState('');
  const [validationResults, setValidationResults] = useState<Record<string, ValidationResult[]>>({});
  const [validatingKey, setValidatingKey] = useState<string | null>(null);
  const [expandedValidation, setExpandedValidation] = useState<Set<string>>(new Set());
  const [floatingCovers, setFloatingCovers] = useState<{ src: string; title: string; author: string }[]>([]);
  const [bubbleSize, setBubbleSize] = useState(500);
  const [narratorFlags, setNarratorFlags] = useState<Record<string, Set<number>>>({});
  const [prunePreview, setPrunePreview] = useState<api.SeriesPrunePreview | null>(null);
  const [pruneLoading, setPruneLoading] = useState(false);
  const [pruneConfirmOpen, setPruneConfirmOpen] = useState(false);
  const pagination = usePagination(groups.length);

  const handleValidate = async (groupKey: string, query: string) => {
    setValidatingKey(groupKey);
    try {
      const resp = await api.validateDedupEntry(query, 'series');
      setValidationResults((prev) => ({ ...prev, [groupKey]: resp.results }));
      setExpandedValidation((prev) => new Set(prev).add(groupKey));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Validation failed');
    } finally {
      setValidatingKey(null);
    }
  };

  const handleSaveEdit = async () => {
    if (editingSeriesId == null || !editingName.trim()) return;
    try {
      await api.updateSeriesName(editingSeriesId, editingName.trim());
      setEditingSeriesId(null);
      setEditingName('');
      fetchDuplicates();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to rename series');
    }
  };

  const populateFromData = useCallback((data: { groups: SeriesDupGroup[]; total_series: number }) => {
    setGroups(data.groups || []);
    setTotalSeries(data.total_series || 0);
    const defaults: Record<string, number[]> = {};
    (data.groups || []).forEach((g, i) => {
      const sorted = [...g.series].sort((a, b) => (a.author_id != null ? -1 : 0) - (b.author_id != null ? -1 : 0));
      defaults[`group-${i}`] = sorted.map((s) => s.id);
    });
    setKeepSelections(defaults);
    setSelectedGroups(new Set());
  }, []);

  const fetchDuplicates = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await api.getSeriesDuplicates();
      if (data.needs_refresh) {
        await runOperationWithPolling(
          () => api.refreshSeriesDuplicates(),
          setActiveOp,
          async () => {
            const fresh = await api.getSeriesDuplicates();
            populateFromData(fresh);
            setLoading(false);
          },
          (msg) => { setError(msg); setLoading(false); },
        );
        return;
      }
      populateFromData(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch series duplicates');
    } finally {
      setLoading(false);
    }
  }, [populateFromData]);

  useEffect(() => { fetchDuplicates(); }, [fetchDuplicates]);

  const handleMerge = async (group: SeriesDupGroup, groupKey: string) => {
    const selected = keepSelections[groupKey] || [];
    if (selected.length === 0) return;
    const keepId = selected[0]; // first selected is the one to keep
    const mergeIds = group.series.filter((s) => s.id !== keepId && selected.includes(s.id)).map((s) => s.id);
    if (mergeIds.length === 0) return;
    setMergeSuccess(null);

    // Reclassify any authors flagged as narrators before merging
    const flagged = narratorFlags[groupKey];
    if (flagged && flagged.size > 0) {
      for (const authorId of flagged) {
        try {
          await api.reclassifyAuthorAsNarrator(authorId);
        } catch (err) {
          setError(err instanceof Error ? err.message : `Failed to reclassify author ${authorId} as narrator`);
          return;
        }
      }
      // Clear flags for this group
      setNarratorFlags((prev) => { const next = { ...prev }; delete next[groupKey]; return next; });
    }

    await runOperationWithPolling(
      () => api.mergeSeriesGroup(keepId, mergeIds),
      setActiveOp,
      (final) => {
        if (final.status === 'failed') {
          setError(final.error_message || 'Series merge failed');
        } else {
          setMergeSuccess(`Merged series "${group.name}"`);
          setGroups((prev) => prev.filter((_, i) => `group-${i}` !== groupKey));
          setSelectedGroups((prev) => { const next = new Set(prev); next.delete(groupKey); return next; });
        }
      },
      setError,
    );
  };

  const handleMergeSelected = async () => {
    setMergeSuccess(null);
    for (let i = 0; i < groups.length; i++) {
      const groupKey = `group-${i}`;
      if (!selectedGroups.has(groupKey)) continue;
      const group = groups[i];
      const selected = keepSelections[groupKey] || [];
      if (selected.length < 2) continue;
      const keepId = selected[0];
      const mergeIds = selected.slice(1);
      try {
        const initial = await api.mergeSeriesGroup(keepId, mergeIds);
        setActiveOp(initial);
        await api.pollOperation(initial.id, (update) => setActiveOp(update));
      } catch (err) {
        setError(err instanceof Error ? err.message : `Failed to merge series "${group.name}"`);
      }
    }
    setActiveOp(null);
    setMergeSuccess(`Merged ${selectedGroups.size} selected group(s)`);
    fetchDuplicates();
  };

  const handleMergeAll = async () => {
    setConfirmOpen(false);
    setMergeSuccess(null);
    setError(null);
    await runOperationWithPolling(
      () => api.deduplicateSeries(),
      setActiveOp,
      (final) => {
        if (final.status === 'failed') {
          setError(final.error_message || 'Series dedup failed');
        } else {
          setMergeSuccess(final.message || 'Series deduplication complete');
          fetchDuplicates();
        }
      },
      setError,
    );
  };

  const toggleGroup = (key: string) => {
    setSelectedGroups((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key); else next.add(key);
      return next;
    });
  };

  const toggleAll = () => {
    if (selectedGroups.size === groups.length) {
      setSelectedGroups(new Set());
    } else {
      setSelectedGroups(new Set(groups.map((_, i) => `group-${i}`)));
    }
  };

  const handlePrunePreview = async () => {
    setPruneLoading(true);
    setError(null);
    try {
      const preview = await api.seriesPrunePreview();
      setPrunePreview(preview);
      if (preview.total_count > 0) {
        setPruneConfirmOpen(true);
      } else {
        setMergeSuccess('No series to prune - library is clean!');
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to get prune preview');
    } finally {
      setPruneLoading(false);
    }
  };

  const handlePruneExecute = async () => {
    setPruneConfirmOpen(false);
    setPrunePreview(null);
    setMergeSuccess(null);
    setError(null);
    await runOperationWithPolling(
      () => api.seriesPrune(),
      setActiveOp,
      (final) => {
        if (final.status === 'failed') {
          setError(final.error_message || 'Series prune failed');
        } else {
          setMergeSuccess(final.message || 'Series prune complete');
          fetchDuplicates();
        }
      },
      setError,
    );
  };

  const busy = activeOp !== null;

  return (
    <Box>
      <Box sx={{ display: 'flex', alignItems: 'center', mb: 2 }}>
        <Typography variant="body2" color="text.secondary" sx={{ flexGrow: 1 }}>
          Detects series with identical names (ignoring case). Often caused by reimports creating series with/without author links.
          Total series: {totalSeries}.
        </Typography>
        <Stack direction="row" spacing={1}>
          <Button variant="outlined" color="secondary" startIcon={<CleaningServicesIcon />}
            onClick={handlePrunePreview} disabled={busy || pruneLoading}>
            {pruneLoading ? 'Checking...' : 'Prune Series'}
          </Button>
          {groups.length > 0 && (
            <>
              <Button size="small" onClick={toggleAll} disabled={busy}>
                {selectedGroups.size === groups.length ? 'Deselect All' : 'Select All'}
              </Button>
              {selectedGroups.size > 0 && (
                <Button variant="contained" color="primary" startIcon={<MergeIcon />}
                  onClick={handleMergeSelected} disabled={busy}>
                  Merge Selected ({selectedGroups.size})
                </Button>
              )}
              <Button variant="contained" color="warning" startIcon={<MergeIcon />}
                onClick={() => setConfirmOpen(true)} disabled={busy}>
                {busy ? 'Merging...' : `Merge All (${groups.length} groups)`}
              </Button>
            </>
          )}
          <Tooltip title="Rescan"><IconButton onClick={fetchDuplicates} disabled={loading || busy}><RefreshIcon /></IconButton></Tooltip>
        </Stack>
      </Box>

      <OperationProgress operation={activeOp} />
      {error && <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>{error}</Alert>}
      {mergeSuccess && <Alert severity="success" sx={{ mb: 2 }} icon={<CheckCircleIcon />} onClose={() => setMergeSuccess(null)}>{mergeSuccess}</Alert>}

      {loading ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}><CircularProgress /></Box>
      ) : groups.length === 0 ? (
        <Paper sx={{ p: 4, textAlign: 'center' }}>
          <CheckCircleIcon sx={{ fontSize: 48, color: 'success.main', mb: 1 }} />
          <Typography variant="h6">No duplicate series found</Typography>
          <Typography variant="body2" color="text.secondary">{totalSeries} unique series in library.</Typography>
        </Paper>
      ) : (
        <>
        <PaginationControls total={groups.length} page={pagination.page} rowsPerPage={pagination.rowsPerPage}
          onPageChange={pagination.setPage} onRowsPerPageChange={pagination.setRowsPerPage} />
        <Stack spacing={2}>
          {groups.slice(pagination.startIdx, pagination.endIdx).map((group, sliceIdx) => {
            const idx = pagination.startIdx + sliceIdx;
            const groupKey = `group-${idx}`;
            return (
              <Card key={groupKey} variant="outlined">
                <Box sx={{ display: 'flex' }}>
                <CardContent sx={{ flex: '0 1 auto', minWidth: 280, maxWidth: '50%' }}>
                      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1, flexWrap: 'wrap' }}>
                        <Checkbox
                          checked={selectedGroups.has(groupKey)}
                          onChange={() => toggleGroup(groupKey)}
                          disabled={busy}
                          size="small"
                        />
                        <Typography variant="subtitle1" fontWeight="bold">{cleanDisplayTitle(group.name)}</Typography>
                        <Chip label={`${group.count} entries`} size="small" color="warning" variant="outlined" />
                        {group.match_type === 'subseries' && (
                          <Chip label="sub-series" size="small" color="info" variant="outlined" />
                        )}
                        {group.suggested_name && (
                          <Chip
                            label={`Suggested: "${group.suggested_name}"`}
                            size="small"
                            color="primary"
                            variant="outlined"
                            onClick={() => {
                              const selected = keepSelections[groupKey] || [];
                              if (selected.length > 0) {
                                setEditingSeriesId(selected[0]);
                                setEditingName(group.suggested_name!);
                              }
                            }}
                            sx={{ cursor: 'pointer' }}
                          />
                        )}
                      </Box>
                      <Divider sx={{ my: 1 }} />
                      {group.series.map((s) => {
                        const selected = keepSelections[groupKey] || [];
                        const isChecked = selected.includes(s.id);
                        const authorLabel = s.author_name
                          ? `${s.author_name} (id: ${s.author_id})`
                          : s.author_id != null ? `author ${s.author_id}` : 'no author';
                        return (
                          <Box key={s.id} sx={{ display: 'flex', alignItems: 'center', gap: 1, py: 0.25 }}>
                            <Checkbox size="small" checked={isChecked}
                              onChange={() => setKeepSelections((prev) => {
                                const cur = prev[groupKey] || [];
                                return { ...prev, [groupKey]: isChecked ? cur.filter((id) => id !== s.id) : [...cur, s.id] };
                              })} />
                            {editingSeriesId === s.id ? (
                              <>
                                <TextField size="small" value={editingName}
                                  onChange={(e) => setEditingName(e.target.value)}
                                  onKeyDown={(e) => { if (e.key === 'Enter') handleSaveEdit(); if (e.key === 'Escape') { setEditingSeriesId(null); setEditingName(''); } }}
                                  autoFocus sx={{ minWidth: 300 }} />
                                <IconButton size="small" color="primary" onClick={handleSaveEdit}><SaveIcon fontSize="small" /></IconButton>
                                <IconButton size="small" onClick={() => { setEditingSeriesId(null); setEditingName(''); }}><CloseIcon fontSize="small" /></IconButton>
                              </>
                            ) : (
                              <>
                                <Typography variant="body2">
                                  ID {s.id}: &quot;{s.name}&quot;
                                </Typography>
                                <Tooltip title="Edit series name">
                                  <IconButton size="small" onClick={(e) => { e.stopPropagation(); setEditingSeriesId(s.id); setEditingName(s.name); }}>
                                    <EditIcon fontSize="small" />
                                  </IconButton>
                                </Tooltip>
                              </>
                            )}
                            <Chip label={authorLabel} size="small"
                              color={(narratorFlags[groupKey]?.has(s.author_id!) ? 'secondary' : 'success')}
                              variant="outlined" />
                            {s.author_id != null && (
                              <Chip
                                label={narratorFlags[groupKey]?.has(s.author_id) ? 'Narrator' : 'Author'}
                                size="small"
                                color={narratorFlags[groupKey]?.has(s.author_id) ? 'secondary' : 'default'}
                                variant={narratorFlags[groupKey]?.has(s.author_id) ? 'filled' : 'outlined'}
                                onClick={() => setNarratorFlags((prev) => {
                                  const cur = new Set(prev[groupKey] || []);
                                  if (cur.has(s.author_id!)) cur.delete(s.author_id!); else cur.add(s.author_id!);
                                  return { ...prev, [groupKey]: cur };
                                })}
                                sx={{ cursor: 'pointer' }}
                              />
                            )}
                          </Box>
                        );
                      })}
                    </CardContent>
                {/* Book covers: per series/author, books in a row, vertical divider between, dup badge if shared */}
                {(() => {
                  // Collect all book IDs across all series to detect duplicates
                  const bookIdCounts = new Map<string, number>();
                  group.series.forEach((s) => (s.books || []).forEach((b) => bookIdCounts.set(b.id, (bookIdCounts.get(b.id) || 0) + 1)));
                  return (
                    <Box sx={{ flex: 1, display: 'flex', borderLeft: '1px solid', borderColor: 'divider', overflowX: 'auto', alignItems: 'flex-start', py: 1 }}>
                      {group.series.map((s, sIdx) => {
                        const books = s.books || [];
                        if (books.length === 0) return null;
                        const authorLabel = s.author_name || (s.author_id != null ? `Author ${s.author_id}` : '');
                        return (
                          <Box key={`covers-${s.id}`} sx={{ display: 'flex' }}>
                            {sIdx > 0 && (
                              <Divider orientation="vertical" flexItem sx={{ mx: 1 }} />
                            )}
                            <Box sx={{ px: 1 }}>
                              <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 0.5, fontWeight: 'bold' }}>
                                {authorLabel}
                              </Typography>
                              <Box sx={{ display: 'flex', gap: 1.5, flexWrap: 'nowrap' }}>
                                {books.map((book) => {
                                  const src = book.cover_url
                                    ? (book.cover_url.startsWith('/') || book.cover_url.startsWith('http') ? book.cover_url : `/api/v1/covers/local/${book.cover_url}`)
                                    : '';
                                  const isDup = (bookIdCounts.get(book.id) || 0) > 1;
                                  return (
                                    <Box key={book.id} sx={{ flexShrink: 0, width: 130, textAlign: 'center' }}>
                                      <Box sx={{ width: 130, height: 195, borderRadius: 1, overflow: 'hidden', border: '1px solid', borderColor: isDup ? 'warning.main' : 'divider', bgcolor: 'action.hover', cursor: src ? 'pointer' : 'default', position: 'relative' }}
                                        onClick={() => { if (src) setFloatingCovers((prev) => prev.some((c) => c.src === src) ? prev.filter((c) => c.src !== src) : [...prev, { src, title: cleanDisplayTitle(book.title), author: authorLabel }]); }}>
                                        {isDup && (
                                          <Chip label="dup" size="small" color="warning" sx={{ position: 'absolute', top: 4, right: 4, zIndex: 1, height: 18, fontSize: '0.6rem' }} />
                                        )}
                                        {src ? (
                                          <img src={src} alt={book.title} style={{ width: '100%', height: '100%', objectFit: 'cover' }}
                                            onError={(e) => { (e.target as HTMLImageElement).style.display = 'none'; }} />
                                        ) : (
                                          <Box sx={{ width: '100%', height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                                            <MenuBookIcon color="disabled" />
                                          </Box>
                                        )}
                                      </Box>
                                      <Typography variant="caption" sx={{ display: 'block', mt: 0.5, fontSize: '0.65rem', lineHeight: 1.2, whiteSpace: 'normal', wordBreak: 'break-word' }}>
                                        {cleanDisplayTitle(book.title)}
                                      </Typography>
                                      <Typography variant="caption" color="text.secondary" sx={{ display: 'block', fontSize: '0.6rem', lineHeight: 1.1 }} noWrap>
                                        {authorLabel}
                                      </Typography>
                                    </Box>
                                  );
                                })}
                              </Box>
                            </Box>
                          </Box>
                        );
                      })}
                    </Box>
                  );
                })()}
                </Box>
                <CardActions>
                  <Button startIcon={<MergeIcon />} variant="contained" size="small"
                    onClick={() => handleMerge(group, groupKey)} disabled={busy}>
                    Merge
                  </Button>
                  <Button startIcon={validatingKey === groupKey ? <CircularProgress size={16} /> : <SearchIcon />}
                    variant="outlined" size="small"
                    onClick={() => handleValidate(groupKey, group.name)}
                    disabled={validatingKey === groupKey}>
                    Validate
                  </Button>
                </CardActions>
                <Collapse in={expandedValidation.has(groupKey)}>
                  <Box sx={{ px: 2, pb: 2 }}>
                    {validationResults[groupKey]?.length ? (
                      <>
                        <Typography variant="caption" color="text.secondary" gutterBottom>
                          Found {validationResults[groupKey].length} result(s) from metadata sources:
                        </Typography>
                        <Stack spacing={0.5} sx={{ mt: 0.5 }}>
                          {validationResults[groupKey].map((r, i) => (
                            <Box key={i} sx={{ display: 'flex', alignItems: 'center', gap: 1, p: 0.5, borderRadius: 1, bgcolor: 'action.hover' }}>
                              {r.cover_url && (
                                <img src={r.cover_url.startsWith('http') ? `/api/v1/covers/proxy?url=${encodeURIComponent(r.cover_url)}` : r.cover_url}
                                  alt="" style={{ width: 32, height: 44, objectFit: 'cover', borderRadius: 2 }}
                                  onError={(e) => { (e.target as HTMLImageElement).style.display = 'none'; }} />
                              )}
                              <Box sx={{ flex: 1, minWidth: 0 }}>
                                <Typography variant="body2" noWrap>{r.title}</Typography>
                                <Typography variant="caption" color="text.secondary" noWrap>
                                  {r.author}{r.series ? ` — Series: ${r.series}${r.series_position ? ` #${r.series_position}` : ''}` : ''}
                                </Typography>
                              </Box>
                              <Chip label={r.source} size="small" variant="outlined" />
                            </Box>
                          ))}
                        </Stack>
                      </>
                    ) : validationResults[groupKey] ? (
                      <Typography variant="caption" color="text.secondary">No results found from metadata sources.</Typography>
                    ) : null}
                  </Box>
                </Collapse>
              </Card>
            );
          })}
        </Stack>
        <PaginationControls total={groups.length} page={pagination.page} rowsPerPage={pagination.rowsPerPage}
          onPageChange={pagination.setPage} onRowsPerPageChange={pagination.setRowsPerPage} />
        </>
      )}

      <Dialog open={confirmOpen} onClose={() => setConfirmOpen(false)}>
        <DialogTitle>Confirm Merge All</DialogTitle>
        <DialogContent>
          <DialogContentText>
            This will merge {groups.length} groups. This action cannot be undone. Are you sure?
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setConfirmOpen(false)}>Cancel</Button>
          <Button onClick={handleMergeAll} color="warning" variant="contained">Confirm</Button>
        </DialogActions>
      </Dialog>

      <Dialog open={pruneConfirmOpen} onClose={() => setPruneConfirmOpen(false)} maxWidth="sm" fullWidth>
        <DialogTitle>Prune Series</DialogTitle>
        <DialogContent>
          <DialogContentText>
            {prunePreview && (
              <>
                This will clean up {prunePreview.total_count} series entries:
                <br />
                - {prunePreview.duplicate_count} duplicate series will be merged (books reassigned to canonical entry)
                <br />
                - {prunePreview.orphan_count} orphan series (0 books) will be deleted
                <br /><br />
                This action cannot be undone.
              </>
            )}
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setPruneConfirmOpen(false)}>Cancel</Button>
          <Button onClick={handlePruneExecute} color="secondary" variant="contained" startIcon={<CleaningServicesIcon />}>
            Prune {prunePreview?.total_count} Series
          </Button>
        </DialogActions>
      </Dialog>

      {/* Floating cover bubble — fixed to right side, resizable */}
      {floatingCovers.length > 0 && (
        <Paper elevation={8} sx={{ position: 'fixed', top: 80, right: 16, zIndex: 1300, p: 1.5, maxWidth: '60vw', maxHeight: '85vh', overflowY: 'auto', borderRadius: 2 }}>
          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1, gap: 2 }}>
            <Typography variant="caption" color="text.secondary">{floatingCovers.length} cover(s) — click to dismiss</Typography>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
              <Typography variant="caption" color="text.secondary">Size:</Typography>
              <input type="range" min={150} max={800} step={25} value={bubbleSize}
                onChange={(e) => setBubbleSize(Number(e.target.value))}
                style={{ width: 100, accentColor: '#90caf9' }} />
              <Typography variant="caption" color="text.secondary">{bubbleSize}px</Typography>
              <IconButton size="small" onClick={() => setFloatingCovers([])}><CloseIcon fontSize="small" /></IconButton>
            </Box>
          </Box>
          <Box sx={{ display: 'flex', gap: 2, flexWrap: 'wrap' }}>
            {floatingCovers.map((cover, ci) => (
              <Box key={ci} sx={{ textAlign: 'center', cursor: 'pointer', width: bubbleSize }}
                onClick={() => setFloatingCovers((prev) => prev.filter((_, j) => j !== ci))}>
                <img src={cover.src} alt={cover.title} style={{ width: bubbleSize, borderRadius: 4, display: 'block' }}
                  onError={(e) => { (e.target as HTMLImageElement).style.display = 'none'; }} />
                <Typography variant="caption" sx={{ display: 'block', mt: 0.5, fontSize: '0.75rem' }}>{cover.title}</Typography>
                <Typography variant="caption" color="text.secondary" sx={{ display: 'block', fontSize: '0.65rem' }}>{cover.author}</Typography>
              </Box>
            ))}
          </Box>
        </Paper>
      )}

    </Box>
  );
}

// ---- AI Author Sub-Page (self-contained per mode) ----
// ---- AI Author Pipeline Page (unified scan-based view) ----
function AIAuthorPipelinePage() {
  const [scan, setScan] = useState<api.AIScanDetail | null>(null);
  const [results, setResults] = useState<api.AIScanResult[]>([]);
  const [scans, setScans] = useState<api.AIScan[]>([]);
  const [batchMode, setBatchMode] = useState(false);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [agreementFilter, setAgreementFilter] = useState<string>('all');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Load scan list on mount
  useEffect(() => {
    api.listAIScans().then(setScans).catch(() => {});
  }, []);

  // Poll active scan status
  useEffect(() => {
    if (!scan || scan.status === 'complete' || scan.status === 'failed') return;
    const interval = setInterval(async () => {
      try {
        const updated = await api.getAIScan(scan.id);
        setScan(updated);
        if (updated.status === 'complete') {
          const res = await api.getAIScanResults(scan.id);
          setResults(res);
          clearInterval(interval);
        }
      } catch { /* ignore polling errors */ }
    }, 5000);
    return () => clearInterval(interval);
    // scan?.id and scan?.status are the meaningful change signals; including
    // the full `scan` object would restart the interval on every poll update.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [scan?.id, scan?.status]);

  const startScan = async () => {
    setLoading(true);
    setError(null);
    try {
      const newScan = await api.startAIScan(batchMode ? 'batch' : 'realtime');
      const detail = await api.getAIScan(newScan.id);
      setScan(detail);
      // Refresh scan list
      api.listAIScans().then(setScans).catch(() => {});
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to start scan');
    }
    setLoading(false);
  };

  const loadScan = async (scanId: number) => {
    setLoading(true);
    setError(null);
    try {
      const detail = await api.getAIScan(scanId);
      setScan(detail);
      if (detail.status === 'complete') {
        const res = await api.getAIScanResults(scanId);
        setResults(res);
      }
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to load scan');
    }
    setLoading(false);
  };

  const applySelected = async () => {
    if (!scan || selected.size === 0) return;
    try {
      await api.applyAIScanResults(scan.id, Array.from(selected));
      const res = await api.getAIScanResults(scan.id);
      setResults(res);
      setSelected(new Set());
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to apply results');
    }
  };

  const filteredResults = agreementFilter === 'all'
    ? results
    : results.filter(r => r.agreement === agreementFilter);

  const toggleSelect = (id: number) => {
    setSelected(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id); else next.add(id);
      return next;
    });
  };

  return (
    <Box>
      {/* Header Bar */}
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, p: 2 }}>
        <Button
          variant="contained"
          onClick={startScan}
          disabled={loading || (scan != null && scan.status !== 'complete' && scan.status !== 'failed')}
          startIcon={<AutoAwesomeIcon />}
        >
          Run Scan
        </Button>
        <FormControlLabel
          control={<Switch checked={batchMode} onChange={(_, v) => setBatchMode(v)} />}
          label={batchMode ? 'Batch (cheaper, hours)' : 'Realtime (faster, more expensive)'}
        />
        <Box sx={{ flex: 1 }} />
        <Button variant="outlined" onClick={() => setHistoryOpen(true)}>
          Scan History
        </Button>
      </Box>

      {error && (
        <Alert severity="error" sx={{ mx: 2 }} onClose={() => setError(null)}>
          {error}
        </Alert>
      )}

      {/* Active Scan Status */}
      {scan && scan.status !== 'complete' && scan.status !== 'failed' && scan.status !== 'canceled' && (
        <Paper
          elevation={3}
          sx={{
            position: 'sticky',
            top: 0,
            zIndex: 10,
            mx: 2,
            mb: 2,
            p: 2,
            borderRadius: 2,
          }}
        >
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
            <Typography variant="subtitle2">Scan #{scan.id} — {scan.status}</Typography>
            <Box sx={{ display: 'flex', gap: 1 }}>
              {(scan.phases || []).map(phase => (
                <Chip
                  key={phase.phase_type}
                  label={`${phase.phase_type.replace('_', ' ')}: ${phase.status}`}
                  color={phase.status === 'complete' ? 'success' : phase.status === 'failed' ? 'error' : 'default'}
                  size="small"
                />
              ))}
            </Box>
            <Box sx={{ flex: 1 }} />
            <Button
              variant="outlined"
              color="error"
              size="small"
              onClick={async () => {
                try {
                  await api.cancelAIScan(scan.id);
                  const updated = await api.getAIScan(scan.id);
                  setScan(updated);
                } catch (e: unknown) {
                  setError(e instanceof Error ? e.message : 'Failed to cancel scan');
                }
              }}
            >
              Cancel Scan
            </Button>
          </Box>
          <LinearProgress sx={{ mt: 1 }} />
        </Paper>
      )}

      {/* Canceled scan message */}
      {scan && scan.status === 'canceled' && (
        <Alert severity="warning" sx={{ mx: 2, mb: 2 }}>
          Scan #{scan.id} was canceled.
        </Alert>
      )}

      {/* No scan loaded */}
      {!scan && !loading && (
        <Paper sx={{ p: 4, mx: 2, textAlign: 'center' }}>
          <Typography variant="body1" color="text.secondary">
            Run a scan to discover author duplicates using multi-pass AI analysis, or load a previous scan from history.
          </Typography>
        </Paper>
      )}

      {loading && !scan && (
        <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}><CircularProgress /></Box>
      )}

      {/* Scan failed */}
      {scan?.status === 'failed' && (
        <Alert severity="error" sx={{ mx: 2 }}>
          Scan #{scan.id} failed. Try running a new scan.
        </Alert>
      )}

      {/* Results */}
      {scan?.status === 'complete' && results.length > 0 && (
        <Box sx={{ px: 2 }}>
          {/* Filter Tabs */}
          <Tabs value={agreementFilter} onChange={(_, v) => setAgreementFilter(v)} sx={{ mb: 2 }}>
            <Tab value="all" label={`All (${results.length})`} />
            <Tab value="agreed" label={`Agreed (${results.filter(r => r.agreement === 'agreed').length})`} />
            <Tab value="groups_only" label={`Groups Only (${results.filter(r => r.agreement === 'groups_only').length})`} />
            <Tab value="full_only" label={`Full Only (${results.filter(r => r.agreement === 'full_only').length})`} />
            <Tab value="disagreed" label={`Disagreed (${results.filter(r => r.agreement === 'disagreed').length})`} />
          </Tabs>

          {/* Floating Apply Bar */}
          {selected.size > 0 && (
            <Paper
              elevation={4}
              sx={{
                position: 'sticky',
                bottom: 16,
                zIndex: 10,
                p: 1.5,
                mx: -2,
                mb: 2,
                display: 'flex',
                alignItems: 'center',
                gap: 2,
                borderRadius: 2,
                bgcolor: 'background.paper',
              }}
            >
              <Button variant="contained" color="primary" onClick={applySelected}>
                Apply Selected ({selected.size})
              </Button>
              <Button variant="outlined" size="small" onClick={() => setSelected(new Set())}>
                Clear Selection
              </Button>
              <Typography variant="body2" color="text.secondary" sx={{ ml: 'auto' }}>
                {selected.size} of {filteredResults.filter(r => !r.applied).length} selected
              </Typography>
            </Paper>
          )}

          {/* Result Cards */}
          {filteredResults.map(result => (
            <Card key={result.id} sx={{ mb: 1, opacity: result.applied ? 0.5 : 1 }} variant="outlined">
              <CardContent sx={{ py: 1, '&:last-child': { pb: 1 } }}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                  <Checkbox
                    checked={selected.has(result.id)}
                    onChange={() => toggleSelect(result.id)}
                    disabled={result.applied}
                    size="small"
                  />
                  <Chip
                    label={result.agreement}
                    size="small"
                    color={result.agreement === 'agreed' ? 'success' : result.agreement === 'disagreed' ? 'error' : 'default'}
                  />
                  <Chip label={result.suggestion.action} size="small" variant="outlined"
                    color={result.suggestion.action === 'merge' ? 'primary' : result.suggestion.action === 'rename' ? 'warning' : result.suggestion.action === 'alias' ? 'info' : 'default'} />
                  <Chip label={result.suggestion.confidence} size="small" variant="outlined"
                    color={result.suggestion.confidence === 'high' ? 'success' : result.suggestion.confidence === 'medium' ? 'warning' : 'error'} />
                  <Box sx={{ flex: 1 }}>
                    <Typography variant="body2" fontWeight="bold">
                      {result.suggestion.canonical_name}
                    </Typography>
                    <Typography variant="caption" color="text.secondary">
                      {result.suggestion.reason}
                    </Typography>
                  </Box>
                  {result.applied && <Chip label="Applied" size="small" color="info" />}
                </Box>
                {result.suggestion.roles && (
                  <>
                    <Divider sx={{ my: 0.5, ml: 5 }} />
                    <RoleDetails roles={result.suggestion.roles} />
                  </>
                )}
              </CardContent>
            </Card>
          ))}

          {/* No results for this filter */}
          {filteredResults.length === 0 && (
            <Typography color="text.secondary" sx={{ p: 2, textAlign: 'center' }}>
              No results matching this filter.
            </Typography>
          )}
        </Box>
      )}

      {/* Scan complete but no results */}
      {scan?.status === 'complete' && results.length === 0 && (
        <Paper sx={{ p: 4, mx: 2, textAlign: 'center' }}>
          <Typography variant="body1" color="text.secondary">
            Scan complete — no duplicate authors found.
          </Typography>
        </Paper>
      )}

      {/* Scan History Drawer */}
      <Drawer anchor="right" open={historyOpen} onClose={() => setHistoryOpen(false)}>
        <Box sx={{ width: 400, p: 2 }}>
          <Typography variant="h6" gutterBottom>Scan History</Typography>
          {scans.map(s => (
            <Card
              key={s.id}
              sx={{ mb: 1, cursor: 'pointer', border: scan?.id === s.id ? 2 : undefined, borderColor: scan?.id === s.id ? 'primary.main' : undefined }}
              variant="outlined"
              onClick={() => { loadScan(s.id); setHistoryOpen(false); }}
            >
              <CardContent sx={{ py: 1, '&:last-child': { pb: 1 } }}>
                <Typography variant="body2" fontWeight="bold">
                  Scan #{s.id} — {s.status}
                </Typography>
                <Typography variant="caption" color="text.secondary">
                  {new Date(s.created_at).toLocaleString()} · {s.author_count} authors · {s.mode}
                </Typography>
              </CardContent>
            </Card>
          ))}
          {scans.length === 0 && (
            <Typography color="text.secondary">No scan history yet.</Typography>
          )}
        </Box>
      </Drawer>
    </Box>
  );
}

// ---- AI Review Top-Level Tab ----
function AIReviewTab() {
  const [searchParams, setSearchParams] = useSearchParams();
  const aiSub = searchParams.get('aisub') || 'authors';
  const setAiSub = (v: string) => {
    const next = new URLSearchParams(searchParams);
    next.set('aisub', v);
    setSearchParams(next, { replace: true });
  };

  return (
    <Box>
      <Tabs value={aiSub} onChange={(_, v) => setAiSub(v)} sx={{ mb: 2, borderBottom: 1, borderColor: 'divider' }}>
        <Tab value="authors" label="Authors" icon={<PersonIcon />} iconPosition="start" />
        <Tab value="books" label="Books" icon={<MenuBookIcon />} iconPosition="start" />
      </Tabs>

      {aiSub === 'authors' && <AIAuthorPipelinePage />}
      {aiSub === 'books' && (
        <Box sx={{ p: 4, textAlign: 'center' }}>
          <Typography color="text.secondary">Book deduplication coming soon.</Typography>
        </Box>
      )}
    </Box>
  );
}

// ---- Reconcile Tab ----
import BuildIcon from '@mui/icons-material/Build';
import FingerprintIcon from '@mui/icons-material/Fingerprint';
import type { ReconcileMatch, ReconcilePreview, ReconcileBrokenRecord } from '../services/api';

function ReconcileTab() {
  const [scanning, setScanning] = useState(false);
  const [scanProgress, setScanProgress] = useState<{ progress: number; total: number; message: string } | null>(null);
  const [preview, setPreview] = useState<ReconcilePreview | null>(null);
  const [lastScanTime, setLastScanTime] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [applying, setApplying] = useState(false);
  const [applyResult, setApplyResult] = useState<string | null>(null);

  const autoSelectHighConfidence = (data: ReconcilePreview) => {
    const autoSelect = new Set<string>();
    for (const m of data.matches) {
      if (m.confidence === 'high') autoSelect.add(m.book_id);
    }
    setSelected(autoSelect);
  };

  // On mount, load the latest scan result from DB
  useEffect(() => {
    const loadLatest = async () => {
      try {
        const { operation, preview: data } = await api.getLatestReconcileScan();
        if (operation && operation.status === 'running') {
          // A scan is already in progress — poll for it
          setScanning(true);
    
          pollForResult(operation.id);
        } else if (data) {
          setPreview(data);
          autoSelectHighConfidence(data);
          if (operation?.completed_at) setLastScanTime(operation.completed_at);
        }
      } catch {
        // No previous scan, that's fine
      }
    };
    loadLatest();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const pollForResult = async (opId: string) => {
    try {
      const result = await api.pollOperation(opId, (op) => {
        setScanProgress({ progress: op.progress, total: op.total, message: op.message });
      });
      if (result.status === 'completed') {
        const { preview: data } = await api.getLatestReconcileScan();
        if (data) {
          setPreview(data);
          autoSelectHighConfidence(data);
        }
        setLastScanTime(new Date().toISOString());
      } else {
        setError(`Scan ${result.status}: ${result.message || result.error_message || ''}`);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Scan failed');
    } finally {
      setScanning(false);
      setScanProgress(null);
    }
  };

  const startScan = async () => {
    setScanning(true);
    setError(null);
    setApplyResult(null);
    try {
      const op = await api.startReconcileScan();

      pollForResult(op.id);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to start scan');
      setScanning(false);
    }
  };

  const toggleMatch = (bookId: string) => {
    setSelected(prev => {
      const next = new Set(prev);
      if (next.has(bookId)) next.delete(bookId);
      else next.add(bookId);
      return next;
    });
  };

  const selectAll = () => {
    if (!preview) return;
    setSelected(new Set(preview.matches.map(m => m.book_id)));
  };

  const deselectAll = () => setSelected(new Set());

  const applyFixes = async () => {
    if (!preview || selected.size === 0) return;
    setApplying(true);
    setApplyResult(null);
    try {
      const matches = preview.matches
        .filter(m => selected.has(m.book_id))
        .map(m => ({ book_id: m.book_id, new_path: m.new_path }));
      const op = await api.startReconcile(matches);
      const result = await api.pollOperation(op.id);
      if (result.status === 'completed') {
        setApplyResult('Reconciliation completed successfully.');
        // Re-scan to refresh
        startScan();
      } else {
        setApplyResult(`Reconciliation ${result.status}: ${result.message || result.error_message || ''}`);
      }
    } catch (err) {
      setApplyResult(err instanceof Error ? err.message : 'Failed to apply fixes');
    } finally {
      setApplying(false);
    }
  };

  const confidenceColor = (confidence: string): 'success' | 'warning' | 'error' => {
    switch (confidence) {
      case 'high': return 'success';
      case 'medium': return 'warning';
      default: return 'error';
    }
  };

  const matchTypeLabel = (type: string): string => {
    switch (type) {
      case 'hash': return 'File Hash';
      case 'original_hash': return 'Original Hash';
      case 'filename': return 'Filename';
      default: return type;
    }
  };

  return (
    <Box>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        Scan the library to find books whose file paths no longer exist on disk,
        then match them against untracked audio files by hash or filename.
        Scans run in the background — you can refresh the page and results will persist.
      </Typography>

      <Stack direction="row" spacing={2} sx={{ mb: 2 }} alignItems="center">
        <Button
          variant="contained"
          startIcon={scanning ? <CircularProgress size={16} /> : <SearchIcon />}
          onClick={startScan}
          disabled={scanning}
        >
          {scanning ? 'Scanning...' : 'Scan for Broken Links'}
        </Button>
        {lastScanTime && (
          <Typography variant="caption" color="text.secondary">
            Last scan: {new Date(lastScanTime).toLocaleString()}
          </Typography>
        )}
      </Stack>

      {scanning && (
        <Paper sx={{ p: 2, mb: 2 }}>
          <Stack spacing={1}>
            <Typography variant="subtitle2">Scan in progress...</Typography>
            {scanProgress && (
              <>
                <LinearProgress
                  variant={scanProgress.total > 0 ? 'determinate' : 'indeterminate'}
                  value={scanProgress.total > 0 ? (scanProgress.progress / scanProgress.total) * 100 : 0}
                />
                <Typography variant="body2" color="text.secondary">
                  {scanProgress.message}
                </Typography>
              </>
            )}
            {!scanProgress && <LinearProgress />}
            <Typography variant="caption" color="text.secondary">
              You can navigate away and come back — results will be saved.
            </Typography>
          </Stack>
        </Paper>
      )}

      {error && <Alert severity="error" sx={{ mb: 2 }}>{error}</Alert>}
      {applyResult && <Alert severity="info" sx={{ mb: 2 }}>{applyResult}</Alert>}

      {preview && (
        <>
          {/* Summary */}
          <Stack direction="row" spacing={2} sx={{ mb: 2 }} flexWrap="wrap" useFlexGap>
            <Chip
              label={`${preview.broken_records.length} broken records`}
              color={preview.broken_records.length > 0 ? 'error' : 'success'}
            />
            <Chip label={`${preview.untracked_files.length} untracked files`} color="default" />
            <Chip label={`${preview.matches.length} matches found`} color="primary" />
            <Chip
              label={`${preview.unmatched_books.length} unmatched`}
              color={preview.unmatched_books.length > 0 ? 'warning' : 'success'}
            />
          </Stack>

          {/* Matches */}
          {preview.matches.length > 0 && (
            <Paper sx={{ p: 2, mb: 2 }}>
              <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ mb: 1 }}>
                <Typography variant="h6">Matches ({preview.matches.length})</Typography>
                <Stack direction="row" spacing={1}>
                  <Button size="small" onClick={selectAll}>Select All</Button>
                  <Button size="small" onClick={deselectAll}>Deselect All</Button>
                  <Button
                    variant="contained"
                    color="primary"
                    size="small"
                    onClick={applyFixes}
                    disabled={applying || selected.size === 0}
                    startIcon={applying ? <CircularProgress size={16} /> : <CheckCircleIcon />}
                  >
                    Apply {selected.size} Fix{selected.size !== 1 ? 'es' : ''}
                  </Button>
                </Stack>
              </Stack>

              {preview.matches.map((m: ReconcileMatch) => {
                // Find common path prefix to avoid repeating long identical paths
                const oldParts = m.old_path.split('/');
                const newParts = m.new_path.split('/');
                let commonIdx = 0;
                while (commonIdx < oldParts.length - 1 && commonIdx < newParts.length - 1 && oldParts[commonIdx] === newParts[commonIdx]) {
                  commonIdx++;
                }
                const commonPrefix = oldParts.slice(0, commonIdx).join('/');
                const oldSuffix = oldParts.slice(commonIdx).join('/');
                const newSuffix = newParts.slice(commonIdx).join('/');
                const hasCommon = commonIdx > 1;

                return (
                <Card key={m.book_id} variant="outlined" sx={{ mb: 1 }}>
                  <CardContent sx={{ pb: 1 }}>
                    <Stack direction="row" spacing={1} alignItems="flex-start">
                      <Checkbox
                        checked={selected.has(m.book_id)}
                        onChange={() => toggleMatch(m.book_id)}
                        sx={{ mt: -0.5 }}
                      />
                      <Box sx={{ flex: 1, minWidth: 0, overflow: 'hidden' }}>
                        <Stack direction="row" spacing={1} alignItems="center" sx={{ mb: 0.5 }}>
                          <Typography variant="subtitle2">{m.book_title}</Typography>
                          <Chip
                            label={matchTypeLabel(m.match_type)}
                            color={confidenceColor(m.confidence)}
                            size="small"
                          />
                          <Chip
                            label={m.confidence}
                            color={confidenceColor(m.confidence)}
                            size="small"
                            variant="outlined"
                          />
                        </Stack>
                        {hasCommon && (
                          <Typography variant="body2" color="text.secondary" sx={{ fontFamily: 'monospace', fontSize: '0.7rem', opacity: 0.6 }}>
                            {commonPrefix}/
                          </Typography>
                        )}
                        <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.75rem', color: 'error.main', overflowX: 'auto', whiteSpace: 'nowrap' }}>
                          - {hasCommon ? oldSuffix : m.old_path}
                        </Typography>
                        <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.75rem', color: 'success.main', overflowX: 'auto', whiteSpace: 'nowrap' }}>
                          + {hasCommon ? newSuffix : m.new_path}
                        </Typography>
                      </Box>
                    </Stack>
                  </CardContent>
                </Card>
                );
              })}
            </Paper>
          )}

          {/* Unmatched books */}
          {preview.unmatched_books.length > 0 && (
            <Paper sx={{ p: 2, mb: 2 }}>
              <Typography variant="h6" sx={{ mb: 1 }}>
                Unmatched Books ({preview.unmatched_books.length})
              </Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                These books have missing files and no matching file was found on disk.
              </Typography>
              {preview.unmatched_books.map((b: ReconcileBrokenRecord) => (
                <Card key={b.book_id} variant="outlined" sx={{ mb: 1 }}>
                  <CardContent sx={{ pb: 1 }}>
                    <Typography variant="subtitle2">{b.title}</Typography>
                    <Typography variant="body2" color="text.secondary" noWrap sx={{ fontFamily: 'monospace', fontSize: '0.75rem' }}>
                      {b.file_path}
                    </Typography>
                  </CardContent>
                </Card>
              ))}
            </Paper>
          )}

          {preview.broken_records.length === 0 && (
            <Alert severity="success">All book file paths are valid. No reconciliation needed.</Alert>
          )}
        </>
      )}
    </Box>
  );
}

// ---- Embedding Dedup Tab ----

/** Cached book details for candidate display */
const bookCache = new Map<string, Book>();
/** Cached book file lists. Files are fetched in parallel with book details so
 * hovering a file path can show every segment without waiting on a network
 * round trip. An empty array means "we tried and got none", `undefined`
 * means "not fetched yet". */
const bookFilesCache = new Map<string, string[]>();

async function fetchBookCached(id: string): Promise<Book | null> {
  if (bookCache.has(id)) return bookCache.get(id)!;
  try {
    const book = await api.getBook(id);
    bookCache.set(id, book);
    return book;
  } catch {
    return null;
  }
}

async function fetchBookFilesCached(id: string): Promise<string[]> {
  const cached = bookFilesCache.get(id);
  if (cached) return cached;
  try {
    const { files } = await api.getBookFiles(id);
    const paths = (files || []).map((f) => f.file_path).filter(Boolean);
    bookFilesCache.set(id, paths);
    return paths;
  } catch {
    bookFilesCache.set(id, []);
    return [];
  }
}

const LAYER_COLORS: Record<string, 'error' | 'primary' | 'secondary'> = {
  exact: 'error',
  embedding: 'primary',
  llm: 'secondary',
};

/**
 * A cluster groups candidate pairs that share books via connected components.
 * A 2-way cluster is a single pair; a 3+ way cluster is what happens when
 * (A,B), (B,C), (A,C) all hit — previously shown as three duplicate-looking
 * rows, now collapsed into one multi-book card.
 */
interface BookCluster {
  key: string;
  bookIds: string[];
  candidateIds: number[];
  layer: string;
  maxSimilarity: number | null;
  hasPending: boolean;
  overallStatus: string;
  llmInfo: string | null;
}

const LAYER_RANK: Record<string, number> = { exact: 3, llm: 2, embedding: 1 };

/**
 * Group candidates into clusters using union-find. Each cluster's layer is
 * the strongest layer seen across its pairs (exact > llm > embedding) so
 * the visual chip reflects the most trustworthy signal in the group.
 */
function buildClusters(candidates: DedupCandidate[]): BookCluster[] {
  const parent = new Map<string, string>();
  const find = (x: string): string => {
    let root = x;
    while (parent.get(root) !== root) root = parent.get(root)!;
    let cur = x;
    while (parent.get(cur) !== root) {
      const next = parent.get(cur)!;
      parent.set(cur, root);
      cur = next;
    }
    return root;
  };
  const union = (a: string, b: string) => {
    const ra = find(a);
    const rb = find(b);
    if (ra !== rb) parent.set(ra, rb);
  };
  for (const c of candidates) {
    if (!parent.has(c.entity_a_id)) parent.set(c.entity_a_id, c.entity_a_id);
    if (!parent.has(c.entity_b_id)) parent.set(c.entity_b_id, c.entity_b_id);
    union(c.entity_a_id, c.entity_b_id);
  }
  const groups = new Map<string, BookCluster>();
  for (const c of candidates) {
    const root = find(c.entity_a_id);
    let g = groups.get(root);
    if (!g) {
      g = {
        key: root,
        bookIds: [],
        candidateIds: [],
        layer: c.layer,
        maxSimilarity: c.similarity ?? null,
        hasPending: false,
        overallStatus: c.status,
        llmInfo: null,
      };
      groups.set(root, g);
    }
    if (!g.bookIds.includes(c.entity_a_id)) g.bookIds.push(c.entity_a_id);
    if (!g.bookIds.includes(c.entity_b_id)) g.bookIds.push(c.entity_b_id);
    g.candidateIds.push(c.id);
    if ((LAYER_RANK[c.layer] ?? 0) > (LAYER_RANK[g.layer] ?? 0)) g.layer = c.layer;
    if (c.similarity != null && (g.maxSimilarity == null || c.similarity > g.maxSimilarity)) {
      g.maxSimilarity = c.similarity;
    }
    if (c.status === 'pending') g.hasPending = true;
    if (g.overallStatus !== c.status) g.overallStatus = 'mixed';
    if (c.llm_reason && !g.llmInfo) g.llmInfo = `${c.llm_verdict ?? ''}: ${c.llm_reason}`;
  }
  // Order clusters by the lowest candidate id they contain so the page
  // order stays stable across refreshes.
  return Array.from(groups.values()).sort((a, b) => {
    const minA = Math.min(...a.candidateIds);
    const minB = Math.min(...b.candidateIds);
    return minA - minB;
  });
}

/**
 * Strip everything up to and including "audiobook-organizer/" so long
 * production paths don't blow out the card width. Falls back to the full
 * path if the marker isn't present (e.g. during tests or odd mounts).
 */
function truncateAudiobookPath(path: string | undefined | null): string {
  if (!path) return '';
  const marker = 'audiobook-organizer/';
  const idx = path.indexOf(marker);
  return idx >= 0 ? path.slice(idx + marker.length) : path;
}

function EmbeddingDedupTab() {
  const navigate = useNavigate();
  const [stats, setStats] = useState<DedupStats[]>([]);
  const [candidates, setCandidates] = useState<DedupCandidate[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [statusFilter, setStatusFilter] = useState<string>('pending');
  const [layerFilter, setLayerFilter] = useState<string>('');
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(25);
  const [bookDetails, setBookDetails] = useState<Map<string, Book>>(new Map());
  const [bookFiles, setBookFiles] = useState<Map<string, string[]>>(new Map());
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [scanning, setScanning] = useState(false);
  const [scanMsg, setScanMsg] = useState<string | null>(null);
  const [bulkMergeOpen, setBulkMergeOpen] = useState(false);
  const [bulkMerging, setBulkMerging] = useState(false);

  // Load stats
  const loadStats = useCallback(async () => {
    try {
      const { stats: s } = await api.getDedupStats();
      setStats(s);
    } catch {
      // stats are optional
    }
  }, []);

  // Load candidates
  const loadCandidates = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const params: Parameters<typeof api.getDedupCandidates>[0] = {
        status: statusFilter || undefined,
        layer: layerFilter || undefined,
        limit: rowsPerPage,
        offset: page * rowsPerPage,
      };
      const resp = await api.getDedupCandidates(params);
      setCandidates(resp.candidates || []);
      setTotal(resp.total || 0);

      // Fetch book details + file lists in parallel for every candidate
      // side. File lists are what makes the "hover for all files" tooltip
      // instant — without them a 4-way cluster would trigger a burst of
      // network requests on mouse-over.
      const ids = new Set<string>();
      for (const c of resp.candidates || []) {
        ids.add(c.entity_a_id);
        ids.add(c.entity_b_id);
      }
      const details = new Map<string, Book>();
      const filesMap = new Map<string, string[]>();
      await Promise.all(
        Array.from(ids).flatMap((id) => [
          fetchBookCached(id).then((book) => {
            if (book) details.set(id, book);
          }),
          fetchBookFilesCached(id).then((paths) => {
            filesMap.set(id, paths);
          }),
        ])
      );
      setBookDetails(details);
      setBookFiles(filesMap);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load candidates');
    } finally {
      setLoading(false);
    }
  }, [statusFilter, layerFilter, page, rowsPerPage]);

  useEffect(() => { loadStats(); }, [loadStats]);
  useEffect(() => { loadCandidates(); }, [loadCandidates]);

  const handleMergeCluster = async (cluster: BookCluster) => {
    setActionLoading(cluster.key);
    try {
      await api.mergeDedupCluster(cluster.bookIds);
      loadCandidates();
      loadStats();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Merge failed');
    } finally {
      setActionLoading(null);
    }
  };

  const handleDismissCluster = async (cluster: BookCluster) => {
    setActionLoading(cluster.key);
    try {
      await api.dismissDedupCluster(cluster.bookIds);
      loadCandidates();
      loadStats();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Dismiss failed');
    } finally {
      setActionLoading(null);
    }
  };

  const handleScan = async () => {
    setScanning(true);
    setScanMsg(null);
    try {
      const { status } = await api.triggerDedupScan();
      setScanMsg(status || 'Scan triggered');
      // Refresh after a short delay
      setTimeout(() => { loadCandidates(); loadStats(); }, 2000);
    } catch (err) {
      setScanMsg(err instanceof Error ? err.message : 'Scan failed');
    } finally {
      setScanning(false);
    }
  };

  const handleLLM = async () => {
    setScanning(true);
    setScanMsg(null);
    try {
      const { status } = await api.triggerDedupLLM();
      setScanMsg(status || 'AI review triggered');
      setTimeout(() => { loadCandidates(); loadStats(); }, 3000);
    } catch (err) {
      setScanMsg(err instanceof Error ? err.message : 'AI review failed');
    } finally {
      setScanning(false);
    }
  };

  const handleBulkMerge = async () => {
    setBulkMerging(true);
    setBulkMergeOpen(false);
    setScanMsg(null);
    try {
      const result = await api.bulkMergeDedupCandidates({
        entity_type: 'book',
        status: statusFilter || 'pending',
        layer: layerFilter || undefined,
      });
      setScanMsg(
        `Bulk merge complete: ${result.merged} merged, ${result.failed} failed (of ${result.attempted} matched)`
      );
      loadCandidates();
      loadStats();
    } catch (err) {
      setScanMsg(err instanceof Error ? err.message : 'Bulk merge failed');
    } finally {
      setBulkMerging(false);
    }
  };

  // Aggregate stats for display
  const pendingCount = stats.filter(s => s.status === 'pending').reduce((sum, s) => sum + s.count, 0);
  const exactCount = stats.filter(s => s.layer === 'exact').reduce((sum, s) => sum + s.count, 0);
  const embeddingCount = stats.filter(s => s.layer === 'embedding').reduce((sum, s) => sum + s.count, 0);
  const llmCount = stats.filter(s => s.layer === 'llm').reduce((sum, s) => sum + s.count, 0);

  const renderBookSide = (id: string) => {
    const book = bookDetails.get(id);
    if (!book) {
      return (
        <Typography variant="body2" color="text.secondary">
          Book #{id}
        </Typography>
      );
    }
    const allFiles = bookFiles.get(id) ?? [];
    // Prefer the full file list (book_files table) over the Book.file_path
    // column because multi-file audiobooks only track the first file on the
    // Book row. When the list is empty (iTunes-linked, unorganized, or
    // haven't-loaded-yet) we fall back to Book.file_path so something shows.
    const primaryPath = allFiles[0] ?? book.file_path ?? '';
    const shortPath = truncateAudiobookPath(primaryPath);
    const extraCount = Math.max(0, allFiles.length - 1);
    // Build a multi-line tooltip that lists every file with the repo-root
    // prefix stripped. This is what lets the user tell near-identical
    // cluster sides apart — "Turn Coat / Turn Coat - 1" vs
    // "Turn Coat / Turn Coat - 1" looks identical until you see the full
    // file lists diverge.
    const tooltipContent =
      allFiles.length > 0 ? (
        <Box sx={{ maxWidth: 600 }}>
          <Typography variant="caption" sx={{ fontWeight: 600, display: 'block', mb: 0.5 }}>
            {allFiles.length} file{allFiles.length === 1 ? '' : 's'}:
          </Typography>
          {allFiles.map((p, idx) => (
            <Typography
              key={idx}
              variant="caption"
              sx={{ display: 'block', fontFamily: 'monospace', fontSize: '0.7rem', whiteSpace: 'pre' }}
            >
              {truncateAudiobookPath(p)}
            </Typography>
          ))}
        </Box>
      ) : (
        <Typography variant="caption" sx={{ fontFamily: 'monospace' }}>
          {primaryPath || '(no file path)'}
        </Typography>
      );
    return (
      <Box
        sx={{ cursor: 'pointer', minWidth: 0, '&:hover .dedup-side-title': { textDecoration: 'underline' } }}
        onClick={() => navigate(`/books/${book.id}`)}
      >
        <Typography
          className="dedup-side-title"
          variant="body2"
          fontWeight="medium"
          noWrap
          title={book.title}
        >
          {cleanDisplayTitle(book.title)}
        </Typography>
        {book.author_name && (
          <Typography variant="caption" color="text.secondary" noWrap title={book.author_name}>
            {book.author_name}
          </Typography>
        )}
        {shortPath && (
          <Tooltip
            title={tooltipContent}
            enterDelay={300}
            placement="bottom-start"
            componentsProps={{ tooltip: { sx: { maxWidth: 'none' } } }}
          >
            <Typography
              variant="caption"
              color="text.disabled"
              noWrap
              sx={{ display: 'block', fontFamily: 'monospace', fontSize: '0.7rem' }}
              onClick={(e) => e.stopPropagation()}
            >
              {shortPath}
              {extraCount > 0 && (
                <Box component="span" sx={{ ml: 0.5, color: 'primary.main', fontWeight: 600 }}>
                  +{extraCount} more
                </Box>
              )}
            </Typography>
          </Tooltip>
        )}
      </Box>
    );
  };

  const clusters = useMemo(() => buildClusters(candidates), [candidates]);

  return (
    <Box>
      {/* Toolbar */}
      <Stack direction="row" spacing={1} sx={{ mb: 2 }} alignItems="center">
        <Button
          variant="outlined"
          startIcon={scanning ? <CircularProgress size={16} /> : <RefreshIcon />}
          onClick={handleScan}
          disabled={scanning || bulkMerging}
          size="small"
        >
          Re-scan
        </Button>
        <Button
          variant="outlined"
          startIcon={scanning ? <CircularProgress size={16} /> : <AutoAwesomeIcon />}
          onClick={handleLLM}
          disabled={scanning || bulkMerging}
          size="small"
        >
          AI Review
        </Button>
        <Button
          variant="outlined"
          color="warning"
          startIcon={bulkMerging ? <CircularProgress size={16} /> : <MergeIcon />}
          onClick={() => setBulkMergeOpen(true)}
          disabled={scanning || bulkMerging || total === 0 || statusFilter !== 'pending'}
          size="small"
          title={statusFilter !== 'pending' ? 'Switch to Pending filter to enable bulk merge' : ''}
        >
          Merge Filtered ({total})
        </Button>
        {scanMsg && (
          <Alert severity="info" sx={{ py: 0, flexGrow: 1 }} onClose={() => setScanMsg(null)}>
            {scanMsg}
          </Alert>
        )}
      </Stack>

      {/* Bulk merge confirmation dialog */}
      <Dialog open={bulkMergeOpen} onClose={() => setBulkMergeOpen(false)}>
        <DialogTitle>Merge all filtered candidates?</DialogTitle>
        <DialogContent>
          <DialogContentText>
            You are about to merge <strong>{total}</strong> candidate
            {total === 1 ? '' : 's'} matching the current filter
            {layerFilter ? ` (layer: ${layerFilter})` : ''}. Each candidate
            becomes a version group; this is irreversible.
          </DialogContentText>
          <DialogContentText sx={{ mt: 2 }}>
            <strong>Warning:</strong> Bulk merging trusts the scorer entirely.
            Review a sample first if you are not confident in the current
            filter's precision.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setBulkMergeOpen(false)}>Cancel</Button>
          <Button onClick={handleBulkMerge} color="warning" variant="contained">
            Merge {total}
          </Button>
        </DialogActions>
      </Dialog>

      {/* Stats chips */}
      <Stack direction="row" spacing={1} sx={{ mb: 2 }} flexWrap="wrap" useFlexGap>
        <Chip label={`${pendingCount} pending`} size="small" color="warning" variant="outlined" />
        <Chip label={`${exactCount} exact`} size="small" color="error" variant="outlined" />
        <Chip label={`${embeddingCount} embedding`} size="small" color="primary" variant="outlined" />
        <Chip label={`${llmCount} LLM`} size="small" color="secondary" variant="outlined" />
        <Chip label={`${total} showing`} size="small" variant="outlined" />
      </Stack>

      {/* Filters */}
      <Stack direction="row" spacing={2} sx={{ mb: 2 }} alignItems="center">
        <Tabs value={statusFilter} onChange={(_, v) => { setStatusFilter(v); setPage(0); }}>
          <Tab value="pending" label="Pending" />
          <Tab value="merged" label="Merged" />
          <Tab value="dismissed" label="Dismissed" />
          <Tab value="" label="All" />
        </Tabs>
        <Divider orientation="vertical" flexItem />
        <Stack direction="row" spacing={0.5}>
          {(['', 'exact', 'embedding', 'llm'] as const).map((layer) => (
            <Chip
              key={layer || 'all'}
              label={layer || 'All'}
              size="small"
              color={layerFilter === layer ? (LAYER_COLORS[layer] || 'default') : 'default'}
              variant={layerFilter === layer ? 'filled' : 'outlined'}
              onClick={() => { setLayerFilter(layer); setPage(0); }}
              sx={{ cursor: 'pointer' }}
            />
          ))}
        </Stack>
      </Stack>

      {error && <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>{error}</Alert>}

      {loading ? (
        <Box sx={{ textAlign: 'center', py: 4 }}><CircularProgress /></Box>
      ) : candidates.length === 0 ? (
        <Paper sx={{ p: 4, textAlign: 'center' }}>
          <Typography color="text.secondary">No candidates found matching the current filters.</Typography>
        </Paper>
      ) : (
        <>
          <Stack spacing={1}>
            {clusters.map((cluster) => {
              const busy = actionLoading === cluster.key;
              const isMultiWay = cluster.bookIds.length > 2;
              // Horizontal cramming stops being readable around 4 sides —
              // dividing the card width by 5+ produces columns too narrow
              // to fit a full title. Switch to a stacked vertical layout
              // (one book per row, full-width file paths) for large
              // clusters so every side stays legible.
              const isLargeCluster = cluster.bookIds.length >= 5;
              return (
                <Card key={cluster.key} variant="outlined">
                  <CardContent sx={{ pb: 1 }}>
                    {/* Top info row: layer, similarity, cluster size */}
                    <Stack
                      direction="row"
                      spacing={1}
                      alignItems="center"
                      sx={{ mb: 1 }}
                    >
                      <Chip
                        label={cluster.layer}
                        size="small"
                        color={LAYER_COLORS[cluster.layer] || 'default'}
                      />
                      {cluster.maxSimilarity != null && (
                        <Typography variant="caption" color="text.secondary">
                          {(cluster.maxSimilarity * 100).toFixed(1)}%
                        </Typography>
                      )}
                      {isMultiWay && (
                        <Chip
                          label={`${cluster.bookIds.length}-way cluster`}
                          size="small"
                          color="warning"
                          variant="outlined"
                        />
                      )}
                      <Box sx={{ flex: 1 }} />
                      <MergeIcon color="action" fontSize="small" />
                    </Stack>

                    {/* Book sides — horizontal for small clusters (2-4 sides
                        fit comfortably side-by-side), vertical for large ones
                        so a 19-way cluster is still mergeable. */}
                    <Stack
                      direction={isLargeCluster ? 'column' : 'row'}
                      spacing={isLargeCluster ? 1 : 2}
                      alignItems="stretch"
                      divider={
                        <Divider
                          orientation={isLargeCluster ? 'horizontal' : 'vertical'}
                          flexItem
                        />
                      }
                      sx={isLargeCluster ? undefined : { overflowX: 'auto' }}
                    >
                      {cluster.bookIds.map((bookId) => (
                        <Box
                          key={bookId}
                          sx={
                            isLargeCluster
                              ? { minWidth: 0 }
                              : { flex: 1, minWidth: 0, maxWidth: `${100 / cluster.bookIds.length}%` }
                          }
                        >
                          {renderBookSide(bookId)}
                        </Box>
                      ))}
                    </Stack>

                    {cluster.llmInfo && (
                      <Typography
                        variant="caption"
                        color="text.secondary"
                        sx={{ mt: 0.5, display: 'block', fontStyle: 'italic' }}
                      >
                        LLM: {cluster.llmInfo}
                      </Typography>
                    )}
                  </CardContent>
                  <CardActions sx={{ pt: 0 }}>
                    {cluster.hasPending ? (
                      <>
                        <Button
                          size="small"
                          color="primary"
                          startIcon={busy ? <CircularProgress size={14} /> : <MergeIcon />}
                          onClick={() => handleMergeCluster(cluster)}
                          disabled={actionLoading != null}
                        >
                          {isMultiWay ? `Merge ${cluster.bookIds.length} Books` : 'Merge'}
                        </Button>
                        <Button
                          size="small"
                          color="inherit"
                          startIcon={busy ? <CircularProgress size={14} /> : <VisibilityOffIcon />}
                          onClick={() => handleDismissCluster(cluster)}
                          disabled={actionLoading != null}
                        >
                          Dismiss
                        </Button>
                      </>
                    ) : (
                      <Chip
                        label={cluster.overallStatus}
                        size="small"
                        color={cluster.overallStatus === 'merged' ? 'success' : 'default'}
                        variant="outlined"
                      />
                    )}
                  </CardActions>
                </Card>
              );
            })}
          </Stack>

          <TablePagination
            component="div"
            count={total}
            page={page}
            onPageChange={(_, p) => setPage(p)}
            rowsPerPage={rowsPerPage}
            onRowsPerPageChange={(e) => { setRowsPerPage(parseInt(e.target.value, 10)); setPage(0); }}
            rowsPerPageOptions={[10, 25, 50, 100, 250, 500, 1000]}
          />
        </>
      )}
    </Box>
  );
}

// ---- Main Dedup Page ----
const TAB_NAMES = ['books', 'book-duplicates', 'authors', 'series', 'ai', 'reconcile', 'embedding'] as const;

export function BookDedup() {
  const [searchParams, setSearchParams] = useSearchParams();
  const tab = useMemo(() => {
    const t = searchParams.get('tab');
    const idx = TAB_NAMES.indexOf(t as typeof TAB_NAMES[number]);
    return idx >= 0 ? idx : 0;
  }, [searchParams]);

  const setTab = (v: number) => {
    setSearchParams({ tab: TAB_NAMES[v] || 'books' }, { replace: true });
  };

  return (
    <Box sx={{ p: 3 }}>
      <Typography variant="h5" sx={{ mb: 2 }}>Deduplication</Typography>

      <Tabs value={tab} onChange={(_, v) => setTab(v)} sx={{ mb: 3, borderBottom: 1, borderColor: 'divider' }}>
        <Tab icon={<Badge color="default"><MenuBookIcon /></Badge>} label="Version Groups" iconPosition="start" />
        <Tab icon={<Badge color="default"><ContentCopyIcon /></Badge>} label="Duplicate Scan" iconPosition="start" />
        <Tab icon={<Badge color="default"><PersonIcon /></Badge>} label="Authors" iconPosition="start" />
        <Tab icon={<Badge color="default"><ListIcon /></Badge>} label="Series" iconPosition="start" />
        <Tab icon={<Badge color="default"><AutoAwesomeIcon /></Badge>} label="AI Review" iconPosition="start" />
        <Tab icon={<Badge color="default"><BuildIcon /></Badge>} label="Reconcile" iconPosition="start" />
        <Tab icon={<Badge color="default"><FingerprintIcon /></Badge>} label="Embedding" iconPosition="start" />
      </Tabs>

      {tab === 0 && <BookDedupTab />}
      {tab === 1 && <BookDedupScanTab />}
      {tab === 2 && <AuthorDedupTab />}
      {tab === 3 && <SeriesDedupTab />}
      {tab === 4 && <AIReviewTab />}
      {tab === 5 && <ReconcileTab />}
      {tab === 6 && <EmbeddingDedupTab />}
    </Box>
  );
}
