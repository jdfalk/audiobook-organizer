// file: web/src/pages/BookDedup.tsx
// version: 2.11.0
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
import * as api from '../services/api';
import type { Book, AuthorDedupGroup, SeriesDupGroup, ValidationResult, Operation } from '../services/api';
import SearchIcon from '@mui/icons-material/Search';
import AutoAwesomeIcon from '@mui/icons-material/AutoAwesome';
import Collapse from '@mui/material/Collapse';
import type { AIAuthorSuggestion, ApplyAISuggestion, AIReviewMode } from '../services/api';

/** Strip "(Unabridged)", "(Abridged)", and leading "[Series X]" from display titles */
function cleanDisplayTitle(title: string): string {
  return title
    .replace(/\s*\((un)?abridged\)/gi, '')
    .replace(/^\[.*?\]\s*/g, '')
    .trim();
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
                      <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>Variants to merge:</Typography>
                      <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 1 }}>
                        {group.variants.map((v) => {
                          const removeKey = `${group.canonical.id}:${v.id}`;
                          if (removedVariants.has(removeKey)) return null;
                          const isNarrator = narratorFlags.has(String(v.id));
                          return (
                            <Box key={v.id} sx={{ display: 'flex', gap: 0.5, alignItems: 'center' }}>
                              <Tooltip title={`Click to use "${v.name}" as canonical spelling`}>
                                <Chip label={v.name} color="warning" variant="outlined" size="small"
                                  onClick={async () => {
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
                                  sx={{ cursor: 'pointer' }} />
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

  const busy = activeOp !== null;

  return (
    <Box>
      <Box sx={{ display: 'flex', alignItems: 'center', mb: 2 }}>
        <Typography variant="body2" color="text.secondary" sx={{ flexGrow: 1 }}>
          Detects series with identical names (ignoring case). Often caused by reimports creating series with/without author links.
          Total series: {totalSeries}.
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
function AIAuthorSubPage({ mode }: { mode: AIReviewMode }) {
  const [suggestions, setSuggestions] = useState<AIAuthorSuggestion[] | null>(null);
  const [groups, setGroups] = useState<AuthorDedupGroup[]>([]);
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [activeOp, setActiveOp] = useState<Operation | null>(null);
  const [confidenceFilter, setConfidenceFilter] = useState<string>('all');
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [success, setSuccess] = useState<string | null>(null);

  const busy = activeOp !== null;

  const loadFromOp = async (opId: string) => {
    try {
      const result = await api.getOperationResult(opId);
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const data = result.result_data as any;
      if (!data?.suggestions) { setError('AI review returned no data'); return; }
      const sug = data.suggestions as AIAuthorSuggestion[];
      const grps = normalizeGroups((data.groups || []) as AuthorDedupGroup[]);
      setSuggestions(sug);
      setGroups(grps);
      setSelected(new Set(sug.map((_, i) => i)));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load AI suggestions');
    }
  };

  const loadLast = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const ops = await api.listOperations(50, 0);
      const validTypes = mode === 'full'
        ? ['ai-author-review-full']
        : ['ai-author-review-groups', 'ai-author-review'];
      const lastReview = ops.items.find(
        (op) => validTypes.includes(op.type) && op.status === 'completed'
      );
      if (lastReview) await loadFromOp(lastReview.id);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load');
    }
    setLoading(false);
  }, [mode]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => { if (!suggestions) loadLast(); }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const handleRun = async () => {
    setError(null);
    await runOperationWithPolling(
      () => api.requestAIAuthorReview(mode),
      setActiveOp,
      async (op) => { await loadFromOp(op.id); },
      (msg) => setError(msg),
    );
  };

  const handleApply = async () => {
    setConfirmOpen(false);
    if (!suggestions) return;
    const toApply: ApplyAISuggestion[] = [];
    for (const idx of selected) {
      const sug = suggestions[idx];
      if (!sug || sug.action === 'skip') continue;
      const group = groups[sug.group_index];
      if (!group) continue;
      toApply.push({
        group_index: sug.group_index, action: sug.action,
        canonical_name: sug.canonical_name, keep_id: group.canonical.id,
        merge_ids: group.variants.map((v) => v.id),
        rename: sug.canonical_name !== group.canonical.name,
      });
    }
    if (toApply.length === 0) { setError('No applicable suggestions'); return; }
    await runOperationWithPolling(
      () => api.applyAIAuthorReview(toApply),
      setActiveOp,
      () => { setSuccess(`Applied ${toApply.length} suggestion(s)`); setSuggestions(null); setSelected(new Set()); },
      (msg) => setError(msg),
    );
  };

  const filtered = useMemo(() => {
    if (!suggestions) return [];
    let items = suggestions.map((s, i) => ({ ...s, _idx: i }));
    if (confidenceFilter !== 'all') items = items.filter((s) => s.confidence === confidenceFilter);
    return items;
  }, [suggestions, confidenceFilter]);

  return (
    <Box>
      <OperationProgress operation={activeOp} />
      {error && <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>{error}</Alert>}
      {success && <Alert severity="success" sx={{ mb: 2 }} onClose={() => setSuccess(null)}>{success}</Alert>}

      {loading ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}><CircularProgress /></Box>
      ) : suggestions === null ? (
        <Paper sx={{ p: 4, textAlign: 'center' }}>
          <Typography variant="body1" color="text.secondary" sx={{ mb: 2 }}>
            {mode === 'full' ? 'AI discovers all duplicates from scratch.' : 'AI validates heuristic-detected groups.'}
          </Typography>
          <Stack direction="row" spacing={2} justifyContent="center">
            <Button variant="contained" color="secondary" startIcon={<AutoAwesomeIcon />}
              onClick={handleRun} disabled={busy}>Run AI Review ({mode})</Button>
            <Button variant="outlined" onClick={loadLast} disabled={busy}>Load Last</Button>
          </Stack>
        </Paper>
      ) : (
        <Paper sx={{ p: 2 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', mb: 2 }}>
            <Typography variant="h6" sx={{ flexGrow: 1 }}>{suggestions.length} suggestions</Typography>
            <Stack direction="row" spacing={1} alignItems="center">
              {['all', 'high', 'medium', 'low'].map((level) => (
                <Chip key={level} label={level === 'all' ? 'All' : level.charAt(0).toUpperCase() + level.slice(1)}
                  variant={confidenceFilter === level ? 'filled' : 'outlined'}
                  color={level === 'high' ? 'success' : level === 'medium' ? 'warning' : level === 'low' ? 'error' : 'default'}
                  onClick={() => setConfidenceFilter(level)} size="small" />
              ))}
              <Tooltip title="Re-run AI Review">
                <IconButton onClick={handleRun} disabled={busy} size="small"><RefreshIcon /></IconButton>
              </Tooltip>
            </Stack>
          </Box>
          <Box sx={{ display: 'flex', gap: 1, mb: 2 }}>
            <Button size="small" onClick={() => setSelected(new Set(filtered.map((s) => s._idx)))}>Select All</Button>
            <Button size="small" onClick={() => setSelected(new Set())}>Deselect All</Button>
            <Box sx={{ flexGrow: 1 }} />
            <Button variant="contained" color="primary" disabled={busy || selected.size === 0}
              onClick={() => setConfirmOpen(true)}>Apply {selected.size} Selected</Button>
          </Box>

          {filtered.map((sug) => {
            const group = groups[sug.group_index];
            const nameChanged = group && sug.canonical_name !== group.canonical.name;
            return (
              <Card key={sug._idx} sx={{ mb: 1 }} variant="outlined">
                <CardContent sx={{ py: 1, '&:last-child': { pb: 1 } }}>
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <Checkbox checked={selected.has(sug._idx)}
                      onChange={() => setSelected((prev) => {
                        const next = new Set(prev);
                        if (next.has(sug._idx)) next.delete(sug._idx); else next.add(sug._idx);
                        return next;
                      })} size="small" />
                    <Typography variant="body2" sx={{ fontWeight: 'bold', minWidth: 200 }}>
                      {group ? group.canonical.name : `Group ${sug.group_index}`}
                      {nameChanged && <> → <span style={{ color: '#1976d2' }}>{sug.canonical_name}</span></>}
                    </Typography>
                    <Chip size="small" label={sug.action}
                      color={sug.action === 'merge' ? 'primary' : sug.action === 'rename' ? 'warning' : sug.action === 'alias' ? 'info' : sug.action === 'split' ? 'secondary' : 'default'} />
                    <Chip size="small" label={sug.confidence}
                      color={sug.confidence === 'high' ? 'success' : sug.confidence === 'medium' ? 'warning' : 'error'} />
                    {group && <Typography variant="caption" color="text.secondary">{group.variants.length} variant(s), {group.book_count} books</Typography>}
                  </Box>
                  <Divider sx={{ my: 0.5, ml: 5 }} />
                  <Typography variant="body2" color="text.secondary" sx={{ ml: 5 }}>{sug.reason}</Typography>
                  {group && group.variants.length > 0 && (
                    <Typography variant="caption" sx={{ ml: 5, display: 'block' }}>Variants: {group.variants.map((v) => v.name).join(', ')}</Typography>
                  )}
                </CardContent>
              </Card>
            );
          })}

          <Dialog open={confirmOpen} onClose={() => setConfirmOpen(false)}>
            <DialogTitle>Apply AI Suggestions</DialogTitle>
            <DialogContent><DialogContentText>Apply {selected.size} correction(s)?</DialogContentText></DialogContent>
            <DialogActions>
              <Button onClick={() => setConfirmOpen(false)}>Cancel</Button>
              <Button onClick={handleApply} color="primary" variant="contained">Apply</Button>
            </DialogActions>
          </Dialog>
        </Paper>
      )}
    </Box>
  );
}

// ---- AI Combined Sub-Page ----
function AIAuthorCombinedSubPage() {
  const [suggestions, setSuggestions] = useState<AIAuthorSuggestion[] | null>(null);
  const [groups, setGroups] = useState<AuthorDedupGroup[]>([]);
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [error, setError] = useState<string | null>(null);
  const [activeOp, setActiveOp] = useState<Operation | null>(null);
  const [activeOp2, setActiveOp2] = useState<Operation | null>(null);
  const [confidenceFilter, setConfidenceFilter] = useState<string>('all');
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [success, setSuccess] = useState<string | null>(null);
  const [combinedSections, setCombinedSections] = useState<{
    agreed: { suggestion: AIAuthorSuggestion; group: AuthorDedupGroup }[];
    full_only: { suggestion: AIAuthorSuggestion; group: AuthorDedupGroup }[];
    groups_only: { suggestion: AIAuthorSuggestion; group: AuthorDedupGroup }[];
  } | null>(null);
  const [combinedFilter, setCombinedFilter] = useState<'all' | 'agreed' | 'full_only' | 'groups_only'>('all');

  const busy = activeOp !== null || activeOp2 !== null;

  const handleRun = async () => {
    setError(null);
    setCombinedSections(null);
    try {
      const fullInitial = await api.requestAIAuthorReview('full');
      setActiveOp(fullInitial);
      const groupsInitial = await api.requestAIAuthorReview('groups');
      setActiveOp2(groupsInitial);

      const [fullFinal, groupsFinal] = await Promise.all([
        api.pollOperation(fullInitial.id, (update) => setActiveOp(update)),
        api.pollOperation(groupsInitial.id, (update) => setActiveOp2(update)),
      ]);
      setActiveOp(null);
      setActiveOp2(null);

      const fullResult = await api.getOperationResult(fullFinal.id);
      const groupsResult = await api.getOperationResult(groupsFinal.id);
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const fullData = fullResult.result_data as any;
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const groupsData = groupsResult.result_data as any;

      const fullSugs = (fullData?.suggestions || []).map((s: AIAuthorSuggestion, i: number) => ({
        suggestion: s,
        group: normalizeGroups(fullData?.groups || [])[s.group_index ?? i] || { canonical: { id: 0, name: '' }, variants: [], book_count: 0 },
      }));
      const groupsSugs = (groupsData?.suggestions || []).map((s: AIAuthorSuggestion, i: number) => ({
        suggestion: s,
        group: normalizeGroups(groupsData?.groups || [])[s.group_index ?? i] || { canonical: { id: 0, name: '' }, variants: [], book_count: 0 },
      }));

      const getAuthorIds = (grp: AuthorDedupGroup): Set<number> => {
        const ids = new Set<number>();
        if (grp?.canonical?.id) ids.add(grp.canonical.id);
        for (const v of grp?.variants || []) ids.add(v.id);
        return ids;
      };
      const overlapRatio = (a: Set<number>, b: Set<number>): number => {
        if (a.size === 0 || b.size === 0) return 0;
        let count = 0;
        for (const id of a) { if (b.has(id)) count++; }
        return count / Math.min(a.size, b.size);
      };

      const agreed: typeof fullSugs = [];
      const fullOnly: typeof fullSugs = [];
      const groupsOnly: typeof fullSugs = [];
      const grMatched = new Set<number>();

      for (const fItem of fullSugs) {
        const fIds = getAuthorIds(fItem.group);
        let matched = false;
        for (let j = 0; j < groupsSugs.length; j++) {
          if (grMatched.has(j)) continue;
          const gIds = getAuthorIds(groupsSugs[j].group);
          if (overlapRatio(fIds, gIds) >= 0.5 && fItem.suggestion.action === groupsSugs[j].suggestion.action) {
            agreed.push(fItem);
            grMatched.add(j);
            matched = true;
            break;
          }
        }
        if (!matched) fullOnly.push(fItem);
      }
      for (let j = 0; j < groupsSugs.length; j++) {
        if (!grMatched.has(j)) groupsOnly.push(groupsSugs[j]);
      }

      const secs = { agreed, full_only: fullOnly, groups_only: groupsOnly };
      setCombinedSections(secs);
      const allItems = [...agreed, ...fullOnly, ...groupsOnly];
      const sug = allItems.map((item, i) => ({ ...item.suggestion, group_index: i }));
      const grps = normalizeGroups(allItems.map((item) => item.group));
      setSuggestions(sug);
      setGroups(grps);
      setSelected(new Set(agreed.map((_: unknown, i: number) => i)));
    } catch (err) {
      setActiveOp(null);
      setActiveOp2(null);
      setError(err instanceof Error ? err.message : 'Combined review failed');
    }
  };

  const handleApply = async () => {
    setConfirmOpen(false);
    if (!suggestions) return;
    const toApply: ApplyAISuggestion[] = [];
    for (const idx of selected) {
      const sug = suggestions[idx];
      if (!sug || sug.action === 'skip') continue;
      const group = groups[sug.group_index];
      if (!group) continue;
      toApply.push({
        group_index: sug.group_index, action: sug.action,
        canonical_name: sug.canonical_name, keep_id: group.canonical.id,
        merge_ids: group.variants.map((v) => v.id),
        rename: sug.canonical_name !== group.canonical.name,
      });
    }
    if (toApply.length === 0) { setError('No applicable suggestions'); return; }
    await runOperationWithPolling(
      () => api.applyAIAuthorReview(toApply),
      setActiveOp,
      () => { setSuccess(`Applied ${toApply.length} suggestion(s)`); setSuggestions(null); setSelected(new Set()); },
      (msg) => setError(msg),
    );
  };

  const filtered = useMemo(() => {
    if (!suggestions) return [];
    let items = suggestions.map((s, i) => ({ ...s, _idx: i }));
    if (combinedFilter !== 'all' && combinedSections) {
      const ac = combinedSections.agreed.length;
      const fc = combinedSections.full_only.length;
      if (combinedFilter === 'agreed') items = items.filter((s) => s._idx < ac);
      else if (combinedFilter === 'full_only') items = items.filter((s) => s._idx >= ac && s._idx < ac + fc);
      else if (combinedFilter === 'groups_only') items = items.filter((s) => s._idx >= ac + fc);
    }
    if (confidenceFilter !== 'all') items = items.filter((s) => s.confidence === confidenceFilter);
    return items;
  }, [suggestions, confidenceFilter, combinedFilter, combinedSections]);

  useEffect(() => { if (!suggestions && !busy) handleRun(); }, []); // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <Box>
      <OperationProgress operation={activeOp} label="Full Mode" />
      <OperationProgress operation={activeOp2} label="Groups Mode" />
      {error && <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>{error}</Alert>}
      {success && <Alert severity="success" sx={{ mb: 2 }} onClose={() => setSuccess(null)}>{success}</Alert>}

      {busy && !suggestions ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}><CircularProgress /></Box>
      ) : suggestions === null ? (
        <Paper sx={{ p: 4, textAlign: 'center' }}>
          <Typography variant="body1" color="text.secondary" sx={{ mb: 2 }}>
            Runs both Full and Groups modes, then shows where they agree vs. disagree.
          </Typography>
          <Button variant="contained" color="secondary" startIcon={<AutoAwesomeIcon />}
            onClick={handleRun} disabled={busy}>Run Combined Review</Button>
        </Paper>
      ) : (
        <Paper sx={{ p: 2 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', mb: 2 }}>
            <Typography variant="h6" sx={{ flexGrow: 1 }}>{suggestions.length} suggestions</Typography>
            <Stack direction="row" spacing={1} alignItems="center">
              {['all', 'high', 'medium', 'low'].map((level) => (
                <Chip key={level} label={level === 'all' ? 'All' : level.charAt(0).toUpperCase() + level.slice(1)}
                  variant={confidenceFilter === level ? 'filled' : 'outlined'}
                  color={level === 'high' ? 'success' : level === 'medium' ? 'warning' : level === 'low' ? 'error' : 'default'}
                  onClick={() => setConfidenceFilter(level)} size="small" />
              ))}
              <Tooltip title="Re-run"><IconButton onClick={handleRun} disabled={busy} size="small"><RefreshIcon /></IconButton></Tooltip>
            </Stack>
          </Box>

          {combinedSections && (
            <Box sx={{ mb: 2 }}>
              <Stack direction="row" spacing={2}>
                <Chip label={`Agreed: ${combinedSections.agreed.length}`} color="success" size="small"
                  variant={combinedFilter === 'agreed' ? 'filled' : 'outlined'}
                  onClick={() => setCombinedFilter(combinedFilter === 'agreed' ? 'all' : 'agreed')} sx={{ cursor: 'pointer' }} />
                <Chip label={`AI Discovered: ${combinedSections.full_only.length}`} color="info" size="small"
                  variant={combinedFilter === 'full_only' ? 'filled' : 'outlined'}
                  onClick={() => setCombinedFilter(combinedFilter === 'full_only' ? 'all' : 'full_only')} sx={{ cursor: 'pointer' }} />
                <Chip label={`Heuristic Only: ${combinedSections.groups_only.length}`} color="default" size="small"
                  variant={combinedFilter === 'groups_only' ? 'filled' : 'outlined'}
                  onClick={() => setCombinedFilter(combinedFilter === 'groups_only' ? 'all' : 'groups_only')} sx={{ cursor: 'pointer' }} />
              </Stack>
            </Box>
          )}

          <Box sx={{ display: 'flex', gap: 1, mb: 2 }}>
            <Button size="small" onClick={() => setSelected(new Set(filtered.map((s) => s._idx)))}>Select All</Button>
            <Button size="small" onClick={() => setSelected(new Set())}>Deselect All</Button>
            <Box sx={{ flexGrow: 1 }} />
            <Button variant="contained" color="primary" disabled={busy || selected.size === 0}
              onClick={() => setConfirmOpen(true)}>Apply {selected.size} Selected</Button>
          </Box>

          {filtered.map((sug) => {
            const group = groups[sug.group_index];
            const nameChanged = group && sug.canonical_name !== group.canonical.name;
            let sectionHeader: React.ReactNode = null;
            if (combinedSections) {
              const ac = combinedSections.agreed.length;
              const fc = combinedSections.full_only.length;
              if (sug._idx === 0 && ac > 0) sectionHeader = <Typography variant="subtitle2" sx={{ mt: 1, mb: 0.5, color: 'success.main', fontWeight: 'bold' }}>Agreed ({ac})</Typography>;
              else if (sug._idx === ac && fc > 0) sectionHeader = <Typography variant="subtitle2" sx={{ mt: 2, mb: 0.5, color: 'info.main', fontWeight: 'bold' }}>AI Discovered ({fc})</Typography>;
              else if (sug._idx === ac + fc && combinedSections.groups_only.length > 0) sectionHeader = <Typography variant="subtitle2" sx={{ mt: 2, mb: 0.5, color: 'text.secondary', fontWeight: 'bold' }}>Heuristic Only ({combinedSections.groups_only.length})</Typography>;
            }
            return (
              <Box key={sug._idx}>
                {sectionHeader}
                <Card sx={{ mb: 1 }} variant="outlined">
                  <CardContent sx={{ py: 1, '&:last-child': { pb: 1 } }}>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                      <Checkbox checked={selected.has(sug._idx)}
                        onChange={() => setSelected((prev) => {
                          const next = new Set(prev);
                          if (next.has(sug._idx)) next.delete(sug._idx); else next.add(sug._idx);
                          return next;
                        })} size="small" />
                      <Typography variant="body2" sx={{ fontWeight: 'bold', minWidth: 200 }}>
                        {group ? group.canonical.name : `Group ${sug.group_index}`}
                        {nameChanged && <> → <span style={{ color: '#1976d2' }}>{sug.canonical_name}</span></>}
                      </Typography>
                      <Chip size="small" label={sug.action}
                        color={sug.action === 'merge' ? 'primary' : sug.action === 'rename' ? 'warning' : sug.action === 'alias' ? 'info' : sug.action === 'split' ? 'secondary' : 'default'} />
                      <Chip size="small" label={sug.confidence}
                        color={sug.confidence === 'high' ? 'success' : sug.confidence === 'medium' ? 'warning' : 'error'} />
                      {group && <Typography variant="caption" color="text.secondary">{group.variants.length} variant(s), {group.book_count} books</Typography>}
                    </Box>
                    <Divider sx={{ my: 0.5, ml: 5 }} />
                    <Typography variant="body2" color="text.secondary" sx={{ ml: 5 }}>{sug.reason}</Typography>
                    {group && group.variants.length > 0 && (
                      <Typography variant="caption" sx={{ ml: 5, display: 'block' }}>Variants: {group.variants.map((v) => v.name).join(', ')}</Typography>
                    )}
                  </CardContent>
                </Card>
              </Box>
            );
          })}

          <Dialog open={confirmOpen} onClose={() => setConfirmOpen(false)}>
            <DialogTitle>Apply AI Suggestions</DialogTitle>
            <DialogContent><DialogContentText>Apply {selected.size} correction(s)?</DialogContentText></DialogContent>
            <DialogActions>
              <Button onClick={() => setConfirmOpen(false)}>Cancel</Button>
              <Button onClick={handleApply} color="primary" variant="contained">Apply</Button>
            </DialogActions>
          </Dialog>
        </Paper>
      )}
    </Box>
  );
}

// ---- AI Review Top-Level Tab ----
function AIReviewTab() {
  const [searchParams, setSearchParams] = useSearchParams();
  const aiSub = searchParams.get('aisub') || 'author-full';
  const setAiSub = (v: string) => {
    const next = new URLSearchParams(searchParams);
    next.set('aisub', v);
    setSearchParams(next, { replace: true });
  };

  return (
    <Box>
      <Tabs value={aiSub} onChange={(_, v) => setAiSub(v)} sx={{ mb: 2, borderBottom: 1, borderColor: 'divider' }}>
        <Tab value="author-full" label="Author Full" icon={<AutoAwesomeIcon />} iconPosition="start" />
        <Tab value="author-groups" label="Author Groups" icon={<AutoAwesomeIcon />} iconPosition="start" />
        <Tab value="author-combined" label="Author Combined" icon={<AutoAwesomeIcon />} iconPosition="start" />
      </Tabs>

      {aiSub === 'author-full' && <AIAuthorSubPage mode="full" />}
      {aiSub === 'author-groups' && <AIAuthorSubPage mode="groups" />}
      {aiSub === 'author-combined' && <AIAuthorCombinedSubPage />}
    </Box>
  );
}

// ---- Main Dedup Page ----
const TAB_NAMES = ['books', 'authors', 'series', 'ai'] as const;

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
        <Tab icon={<Badge color="default"><MenuBookIcon /></Badge>} label="Books" iconPosition="start" />
        <Tab icon={<Badge color="default"><PersonIcon /></Badge>} label="Authors" iconPosition="start" />
        <Tab icon={<Badge color="default"><ListIcon /></Badge>} label="Series" iconPosition="start" />
        <Tab icon={<Badge color="default"><AutoAwesomeIcon /></Badge>} label="AI Review" iconPosition="start" />
      </Tabs>

      {tab === 0 && <BookDedupTab />}
      {tab === 1 && <AuthorDedupTab />}
      {tab === 2 && <SeriesDedupTab />}
      {tab === 3 && <AIReviewTab />}
    </Box>
  );
}
