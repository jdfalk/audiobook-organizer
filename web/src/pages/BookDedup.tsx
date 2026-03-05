// file: web/src/pages/BookDedup.tsx
// version: 2.2.0
// guid: c3d4e5f6-a7b8-9c0d-1e2f-book0dedup02

import { useState, useEffect, useCallback } from 'react';
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
} from '@mui/material';
import MergeIcon from '@mui/icons-material/MergeType';
import RefreshIcon from '@mui/icons-material/Refresh';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import FolderIcon from '@mui/icons-material/Folder';
import MenuBookIcon from '@mui/icons-material/MenuBook';
import PersonIcon from '@mui/icons-material/Person';
import ListIcon from '@mui/icons-material/List';
import * as api from '../services/api';
import type { Book, AuthorDedupGroup, SeriesDupGroup, Operation } from '../services/api';

// Shared operation progress banner
function OperationProgress({ operation }: { operation: Operation | null }) {
  if (!operation || operation.status === 'completed' || operation.status === 'failed' || operation.status === 'cancelled') return null;
  const pct = operation.total > 0 ? Math.round((operation.progress / operation.total) * 100) : 0;
  return (
    <Paper sx={{ p: 2, mb: 2 }}>
      <Stack spacing={1}>
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
        <Stack spacing={2}>
          {groups.map((group, idx) => {
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
                    <Typography variant="subtitle1" fontWeight="bold">{group[0]?.title || 'Unknown'}</Typography>
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

// ---- Author Dedup Tab ----
function AuthorDedupTab() {
  const [groups, setGroups] = useState<AuthorDedupGroup[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [activeOp, setActiveOp] = useState<Operation | null>(null);
  const [mergeSuccess, setMergeSuccess] = useState<string | null>(null);
  const [selectedGroups, setSelectedGroups] = useState<Set<string>>(new Set());
  const [confirmOpen, setConfirmOpen] = useState(false);

  const fetchDuplicates = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setGroups(await api.getAuthorDuplicates());
      setSelectedGroups(new Set());
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch duplicates');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { fetchDuplicates(); }, [fetchDuplicates]);

  const handleMerge = async (group: AuthorDedupGroup) => {
    setMergeSuccess(null);
    await runOperationWithPolling(
      () => api.mergeAuthors(group.canonical.id, group.variants.map((v) => v.id)),
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

      <OperationProgress operation={activeOp} />
      {error && <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>{error}</Alert>}
      {mergeSuccess && <Alert severity="success" sx={{ mb: 2 }} icon={<CheckCircleIcon />} onClose={() => setMergeSuccess(null)}>{mergeSuccess}</Alert>}

      {loading ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}><CircularProgress /></Box>
      ) : groups.length === 0 ? (
        <Paper sx={{ p: 4, textAlign: 'center' }}>
          <CheckCircleIcon sx={{ fontSize: 48, color: 'success.main', mb: 1 }} />
          <Typography variant="h6">No duplicate authors found</Typography>
        </Paper>
      ) : (
        <Stack spacing={2}>
          {groups.map((group) => {
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
                    <Typography variant="subtitle1" fontWeight="bold">{group.canonical.name}</Typography>
                    <Chip icon={<MenuBookIcon />} label={`${group.book_count} book(s)`} size="small" variant="outlined" />
                  </Box>
                  <Divider sx={{ my: 1 }} />
                  <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>Variants to merge:</Typography>
                  <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 1 }}>
                    {group.variants.map((v) => (
                      <Chip key={v.id} label={v.name} color="warning" variant="outlined" size="small" />
                    ))}
                  </Box>
                </CardContent>
                <CardActions>
                  <Button startIcon={<MergeIcon />} variant="contained" size="small"
                    onClick={() => handleMerge(group)} disabled={busy}>
                    {`Merge into "${group.canonical.name}"`}
                  </Button>
                </CardActions>
              </Card>
            );
          })}
        </Stack>
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

// ---- Series Dedup Tab ----
function SeriesDedupTab() {
  const [groups, setGroups] = useState<SeriesDupGroup[]>([]);
  const [totalSeries, setTotalSeries] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [activeOp, setActiveOp] = useState<Operation | null>(null);
  const [mergeSuccess, setMergeSuccess] = useState<string | null>(null);
  const [keepSelections, setKeepSelections] = useState<Record<string, number>>({});
  const [selectedGroups, setSelectedGroups] = useState<Set<string>>(new Set());
  const [confirmOpen, setConfirmOpen] = useState(false);

  const fetchDuplicates = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await api.getSeriesDuplicates();
      setGroups(data.groups || []);
      setTotalSeries(data.total_series || 0);
      // Default keep selection: prefer series with author_id set
      const defaults: Record<string, number> = {};
      (data.groups || []).forEach((g, i) => {
        const withAuthor = g.series.find((s) => s.author_id != null);
        defaults[`group-${i}`] = withAuthor ? withAuthor.id : g.series[0].id;
      });
      setKeepSelections(defaults);
      setSelectedGroups(new Set());
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch series duplicates');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { fetchDuplicates(); }, [fetchDuplicates]);

  const handleMerge = async (group: SeriesDupGroup, groupKey: string) => {
    const keepId = keepSelections[groupKey];
    if (!keepId) return;
    const mergeIds = group.series.filter((s) => s.id !== keepId).map((s) => s.id);
    setMergeSuccess(null);
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
      const keepId = keepSelections[groupKey];
      if (!keepId) continue;
      const mergeIds = group.series.filter((s) => s.id !== keepId).map((s) => s.id);
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
        <Stack spacing={2}>
          {groups.map((group, idx) => {
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
                    <Typography variant="subtitle1" fontWeight="bold">{group.name}</Typography>
                    <Chip label={`${group.count} entries`} size="small" color="warning" variant="outlined" />
                  </Box>
                  <Divider sx={{ my: 1 }} />
                  <RadioGroup
                    value={String(keepSelections[groupKey] || '')}
                    onChange={(e) => setKeepSelections((prev) => ({ ...prev, [groupKey]: Number(e.target.value) }))}
                  >
                    {group.series.map((s) => (
                      <FormControlLabel key={s.id} value={String(s.id)} control={<Radio size="small" />}
                        label={
                          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                            <Typography variant="body2">
                              ID {s.id}: &quot;{s.name}&quot;
                            </Typography>
                            {s.author_id != null ? (
                              <Chip label={`author: ${s.author_id}`} size="small" color="success" variant="outlined" />
                            ) : (
                              <Chip label="no author" size="small" variant="outlined" />
                            )}
                          </Box>
                        }
                      />
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

// ---- Main Dedup Page ----
export function BookDedup() {
  const [tab, setTab] = useState(0);

  return (
    <Box sx={{ p: 3 }}>
      <Typography variant="h5" sx={{ mb: 2 }}>Deduplication</Typography>

      <Tabs value={tab} onChange={(_, v) => setTab(v)} sx={{ mb: 3, borderBottom: 1, borderColor: 'divider' }}>
        <Tab icon={<Badge color="default"><MenuBookIcon /></Badge>} label="Books" iconPosition="start" />
        <Tab icon={<Badge color="default"><PersonIcon /></Badge>} label="Authors" iconPosition="start" />
        <Tab icon={<Badge color="default"><ListIcon /></Badge>} label="Series" iconPosition="start" />
      </Tabs>

      {tab === 0 && <BookDedupTab />}
      {tab === 1 && <AuthorDedupTab />}
      {tab === 2 && <SeriesDedupTab />}
    </Box>
  );
}
