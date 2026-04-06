// file: web/src/components/audiobooks/MetadataReviewDialog.tsx
// version: 1.0.0
// guid: e7f8a9b0-c1d2-3e4f-5a6b-7c8d9e0f1a2b

import { useEffect, useState } from 'react';
import {
  Avatar,
  Box,
  Button,
  Checkbox,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Slider,
  Stack,
  ToggleButton,
  ToggleButtonGroup,
  Tooltip,
  Typography,
} from '@mui/material';
import type { CandidateResult } from '../../services/api';
import * as api from '../../services/api';

interface MetadataReviewDialogProps {
  open: boolean;
  operationId: string;
  onClose: () => void;
  onComplete: () => void;
  toast: (
    message: string,
    severity?: 'success' | 'error' | 'warning' | 'info',
    action?: { label: string; onClick: () => void }
  ) => void;
}

const SOURCE_COLORS: Record<string, 'primary' | 'secondary' | 'success' | 'warning' | 'info'> = {
  openlibrary: 'primary',
  google_books: 'secondary',
  audible: 'success',
  goodreads: 'warning',
  manual: 'info',
};

function formatDuration(seconds: number): string {
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  return h > 0 ? `${h}h ${m}m` : `${m}m`;
}

function formatFileSize(bytes: number): string {
  if (bytes >= 1073741824) return `${(bytes / 1073741824).toFixed(1)} GB`;
  if (bytes >= 1048576) return `${(bytes / 1048576).toFixed(0)} MB`;
  return `${(bytes / 1024).toFixed(0)} KB`;
}

export function MetadataReviewDialog({
  open,
  operationId,
  onClose,
  onComplete,
  toast,
}: MetadataReviewDialogProps) {
  const [results, setResults] = useState<CandidateResult[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [rowStates, setRowStates] = useState<Map<string, 'pending' | 'applied' | 'skipped'>>(
    new Map()
  );
  const [sourceFilter, setSourceFilter] = useState<string | null>(null);
  const [confidenceThreshold, setConfidenceThreshold] = useState(85);
  const [viewMode, setViewMode] = useState<'compact' | 'two-column'>('compact');
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [applying, setApplying] = useState(false);
  const [summary, setSummary] = useState({ matched: 0, no_match: 0, errors: 0, total: 0 });

  useEffect(() => {
    if (!open || !operationId) return;
    setLoading(true);
    api
      .getOperationResults(operationId)
      .then((data) => {
        setResults(data.results || []);
        setSummary(data.summary);
        setLoading(false);
      })
      .catch(() => setLoading(false));
  }, [open, operationId]);

  // Compute unique sources with counts
  const sourceCounts = results.reduce<Record<string, number>>((acc, r) => {
    if (r.candidate?.source) {
      acc[r.candidate.source] = (acc[r.candidate.source] || 0) + 1;
    }
    return acc;
  }, {});

  const filteredResults = results
    .filter((r) => !sourceFilter || r.candidate?.source === sourceFilter)
    .filter(
      (r) =>
        (r.status === 'matched' &&
          r.candidate &&
          r.candidate.score * 100 >= confidenceThreshold) ||
        r.status !== 'matched'
    );

  const handleApplyOne = async (bookId: string) => {
    try {
      await api.batchApplyCandidates(operationId, [bookId]);
      setRowStates((prev) => new Map(prev).set(bookId, 'applied'));
      toast('Applied metadata', 'success', {
        label: 'Undo',
        onClick: async () => {
          try {
            await api.undoLastApply(bookId);
            setRowStates((prev) => new Map(prev).set(bookId, 'pending'));
            toast('Undone', 'info');
          } catch {
            /* ignore */
          }
        },
      });
    } catch {
      toast('Failed to apply', 'error');
    }
  };

  const handleBulkApply = async (bookIds: string[]) => {
    if (bookIds.length === 0) return;
    setApplying(true);
    try {
      const { applied } = await api.batchApplyCandidates(operationId, bookIds);
      const newStates = new Map(rowStates);
      bookIds.forEach((id) => newStates.set(id, 'applied'));
      setRowStates(newStates);
      setSelectedIds(new Set());
      toast(`Applied metadata to ${applied} books`, 'success', {
        label: 'Undo All',
        onClick: async () => {
          for (const id of bookIds) {
            try {
              await api.undoLastApply(id);
            } catch {
              /* ignore */
            }
          }
          const revertStates = new Map(rowStates);
          bookIds.forEach((id) => revertStates.set(id, 'pending'));
          setRowStates(revertStates);
          toast(`Undid ${bookIds.length} applies`, 'info');
        },
      });
      onComplete();
    } catch {
      toast('Failed to apply', 'error');
    } finally {
      setApplying(false);
    }
  };

  const handleSkip = (bookId: string) => {
    setRowStates((prev) => new Map(prev).set(bookId, 'skipped'));
  };

  const highConfidenceIds = filteredResults
    .filter(
      (r) =>
        r.status === 'matched' &&
        r.candidate &&
        r.candidate.score * 100 >= confidenceThreshold &&
        r.candidate.narrator &&
        rowStates.get(r.book.id) !== 'applied' &&
        rowStates.get(r.book.id) !== 'skipped'
    )
    .map((r) => r.book.id);

  const allVisiblePendingIds = filteredResults
    .filter(
      (r) =>
        r.status === 'matched' &&
        r.candidate &&
        rowStates.get(r.book.id) !== 'applied' &&
        rowStates.get(r.book.id) !== 'skipped'
    )
    .map((r) => r.book.id);

  const handleSkipAllUnmatched = () => {
    const newStates = new Map(rowStates);
    results
      .filter((r) => r.status === 'no_match' || r.status === 'error')
      .forEach((r) => newStates.set(r.book.id, 'skipped'));
    setRowStates(newStates);
  };

  const toggleSelected = (bookId: string) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(bookId)) next.delete(bookId);
      else next.add(bookId);
      return next;
    });
  };

  const getRowSx = (bookId: string) => {
    const state = rowStates.get(bookId);
    if (state === 'applied')
      return { bgcolor: 'success.main', opacity: 0.6, borderRadius: 1, transition: 'all 0.3s' };
    if (state === 'skipped')
      return { bgcolor: 'action.disabledBackground', opacity: 0.5, borderRadius: 1, transition: 'all 0.3s' };
    return { borderRadius: 1, transition: 'all 0.3s' };
  };

  const isRowActionable = (bookId: string) => {
    const state = rowStates.get(bookId);
    return state !== 'applied' && state !== 'skipped';
  };

  const renderCompactRow = (r: CandidateResult) => {
    const bookId = r.book.id;
    const isExpanded = expandedId === bookId;

    return (
      <Box key={bookId}>
        <Stack
          direction="row"
          alignItems="center"
          spacing={1}
          sx={{
            p: 1,
            cursor: 'pointer',
            '&:hover': { bgcolor: 'action.hover' },
            ...getRowSx(bookId),
          }}
          onClick={() => setExpandedId(isExpanded ? null : bookId)}
        >
          <Checkbox
            size="small"
            checked={selectedIds.has(bookId)}
            onClick={(e) => e.stopPropagation()}
            onChange={() => toggleSelected(bookId)}
            disabled={!isRowActionable(bookId)}
          />
          <Avatar
            src={r.candidate?.cover_url || r.book.cover_url || ''}
            variant="rounded"
            sx={{ width: 40, height: 50 }}
          />
          <Box sx={{ flex: 1, minWidth: 0 }}>
            <Typography variant="body2" noWrap>
              {r.book.title}
              {r.candidate ? (
                <>
                  {' \u2192 '}
                  <strong>{r.candidate.title}</strong>
                </>
              ) : r.status === 'no_match' ? (
                <Chip label="No match" size="small" sx={{ ml: 1 }} />
              ) : r.status === 'error' ? (
                <Chip label="Error" size="small" color="error" sx={{ ml: 1 }} />
              ) : null}
            </Typography>
          </Box>
          {r.candidate && (
            <>
              <Chip
                label={`${Math.round(r.candidate.score * 100)}%`}
                size="small"
                color={r.candidate.score >= 0.85 ? 'success' : r.candidate.score >= 0.6 ? 'warning' : 'default'}
              />
              <Chip
                label={r.candidate.source}
                size="small"
                color={SOURCE_COLORS[r.candidate.source] || 'default'}
                variant="outlined"
              />
            </>
          )}
          {isRowActionable(bookId) && r.candidate && (
            <>
              <Button
                size="small"
                variant="contained"
                color="success"
                onClick={(e) => {
                  e.stopPropagation();
                  handleApplyOne(bookId);
                }}
              >
                Apply
              </Button>
              <Button
                size="small"
                variant="outlined"
                onClick={(e) => {
                  e.stopPropagation();
                  handleSkip(bookId);
                }}
              >
                Skip
              </Button>
            </>
          )}
          {!isRowActionable(bookId) && (
            <Chip
              label={rowStates.get(bookId) === 'applied' ? 'Applied' : 'Skipped'}
              size="small"
              color={rowStates.get(bookId) === 'applied' ? 'success' : 'default'}
            />
          )}
        </Stack>

        {/* Expanded two-column detail for this row */}
        {isExpanded && r.candidate && (
          <Stack direction="row" spacing={2} sx={{ p: 2, pl: 7, bgcolor: 'action.hover', borderRadius: 1 }}>
            <Box sx={{ flex: 1 }}>
              <Typography variant="subtitle2" gutterBottom>
                Current
              </Typography>
              <Stack direction="row" spacing={1} alignItems="flex-start">
                <Avatar src={r.book.cover_url || ''} variant="rounded" sx={{ width: 60, height: 80 }} />
                <Box>
                  <Typography variant="body2" fontWeight="bold">
                    {r.book.title}
                  </Typography>
                  <Typography variant="body2">{r.book.author}</Typography>
                  {r.book.format && <Chip label={r.book.format} size="small" sx={{ mt: 0.5 }} />}
                  {r.book.duration_seconds && (
                    <Typography variant="caption" display="block">
                      {formatDuration(r.book.duration_seconds)}
                    </Typography>
                  )}
                  {r.book.file_size_bytes && (
                    <Typography variant="caption" display="block">
                      {formatFileSize(r.book.file_size_bytes)}
                    </Typography>
                  )}
                  <Typography variant="caption" sx={{ wordBreak: 'break-all' }}>
                    {r.book.file_path}
                  </Typography>
                  {r.book.itunes_path && (
                    <Typography variant="caption" color="info.main" display="block" sx={{ wordBreak: 'break-all' }}>
                      iTunes: {r.book.itunes_path}
                    </Typography>
                  )}
                </Box>
              </Stack>
            </Box>
            <Box sx={{ flex: 1 }}>
              <Typography variant="subtitle2" gutterBottom>
                Proposed
              </Typography>
              <Stack direction="row" spacing={1} alignItems="flex-start">
                <Avatar src={r.candidate.cover_url || ''} variant="rounded" sx={{ width: 60, height: 80 }} />
                <Box>
                  <Typography variant="body2" fontWeight="bold">
                    {r.candidate.title}
                  </Typography>
                  <Typography variant="body2">{r.candidate.author}</Typography>
                  {r.candidate.narrator && (
                    <Typography variant="body2" color="text.secondary">
                      Narrated by {r.candidate.narrator}
                    </Typography>
                  )}
                  {r.candidate.series && (
                    <Typography variant="body2">
                      Series: {r.candidate.series}
                      {r.candidate.series_position ? ` \u00b7 Book ${r.candidate.series_position}` : ''}
                    </Typography>
                  )}
                  {r.candidate.year && (
                    <Typography variant="caption" display="block">
                      {r.candidate.year}
                    </Typography>
                  )}
                  {r.candidate.publisher && (
                    <Typography variant="caption" display="block">
                      {r.candidate.publisher}
                    </Typography>
                  )}
                  <Chip
                    label={`${Math.round(r.candidate.score * 100)}%`}
                    size="small"
                    color={r.candidate.score >= 0.85 ? 'success' : r.candidate.score >= 0.6 ? 'warning' : 'default'}
                    sx={{ mt: 0.5, mr: 0.5 }}
                  />
                  <Chip
                    label={r.candidate.source}
                    size="small"
                    color={SOURCE_COLORS[r.candidate.source] || 'default'}
                    variant="outlined"
                    sx={{ mt: 0.5 }}
                  />
                </Box>
              </Stack>
            </Box>
          </Stack>
        )}
      </Box>
    );
  };

  const renderTwoColumnCard = (r: CandidateResult) => {
    const bookId = r.book.id;

    return (
      <Box
        key={bookId}
        sx={{
          p: 2,
          mb: 1,
          border: 1,
          borderColor: 'divider',
          ...getRowSx(bookId),
        }}
      >
        <Stack direction="row" spacing={2}>
          {/* Left: current book info */}
          <Box sx={{ flex: 1 }}>
            <Stack direction="row" spacing={1} alignItems="flex-start">
              <Checkbox
                size="small"
                checked={selectedIds.has(bookId)}
                onChange={() => toggleSelected(bookId)}
                disabled={!isRowActionable(bookId)}
              />
              <Avatar src={r.book.cover_url || ''} variant="rounded" sx={{ width: 60, height: 80 }} />
              <Box sx={{ minWidth: 0 }}>
                <Typography variant="body2" fontWeight="bold">
                  {r.book.title}
                </Typography>
                <Typography variant="body2">{r.book.author}</Typography>
                {r.book.format && <Chip label={r.book.format} size="small" sx={{ mt: 0.5 }} />}
                {r.book.duration_seconds && (
                  <Typography variant="caption" display="block">
                    {formatDuration(r.book.duration_seconds)}
                  </Typography>
                )}
                {r.book.file_size_bytes && (
                  <Typography variant="caption" display="block">
                    {formatFileSize(r.book.file_size_bytes)}
                  </Typography>
                )}
                <Typography variant="caption" sx={{ wordBreak: 'break-all' }}>
                  {r.book.file_path}
                </Typography>
                {r.book.itunes_path && (
                  <Typography variant="caption" color="info.main" display="block" sx={{ wordBreak: 'break-all' }}>
                    iTunes: {r.book.itunes_path}
                  </Typography>
                )}
              </Box>
            </Stack>
          </Box>

          {/* Right: proposed match */}
          <Box sx={{ flex: 1 }}>
            {r.candidate ? (
              <Stack direction="row" spacing={1} alignItems="flex-start">
                <Avatar src={r.candidate.cover_url || ''} variant="rounded" sx={{ width: 60, height: 80 }} />
                <Box sx={{ minWidth: 0, flex: 1 }}>
                  <Typography variant="body2" fontWeight="bold">
                    {r.candidate.title}
                  </Typography>
                  <Typography variant="body2">{r.candidate.author}</Typography>
                  {r.candidate.narrator && (
                    <Typography variant="body2" color="text.secondary">
                      Narrated by {r.candidate.narrator}
                    </Typography>
                  )}
                  {r.candidate.series && (
                    <Typography variant="body2">
                      Series: {r.candidate.series}
                      {r.candidate.series_position ? ` \u00b7 Book ${r.candidate.series_position}` : ''}
                    </Typography>
                  )}
                  {r.candidate.year && (
                    <Typography variant="caption" display="block">
                      {r.candidate.year}
                    </Typography>
                  )}
                  {r.candidate.publisher && (
                    <Typography variant="caption" display="block">
                      {r.candidate.publisher}
                    </Typography>
                  )}
                  <Stack direction="row" spacing={0.5} sx={{ mt: 0.5 }}>
                    <Chip
                      label={`${Math.round(r.candidate.score * 100)}%`}
                      size="small"
                      color={
                        r.candidate.score >= 0.85
                          ? 'success'
                          : r.candidate.score >= 0.6
                            ? 'warning'
                            : 'default'
                      }
                    />
                    <Chip
                      label={r.candidate.source}
                      size="small"
                      color={SOURCE_COLORS[r.candidate.source] || 'default'}
                      variant="outlined"
                    />
                  </Stack>
                  {isRowActionable(bookId) && (
                    <Stack direction="row" spacing={1} sx={{ mt: 1 }}>
                      <Button size="small" variant="contained" color="success" onClick={() => handleApplyOne(bookId)}>
                        Apply
                      </Button>
                      <Button size="small" variant="outlined" onClick={() => handleSkip(bookId)}>
                        Skip
                      </Button>
                    </Stack>
                  )}
                  {!isRowActionable(bookId) && (
                    <Chip
                      label={rowStates.get(bookId) === 'applied' ? 'Applied' : 'Skipped'}
                      size="small"
                      color={rowStates.get(bookId) === 'applied' ? 'success' : 'default'}
                      sx={{ mt: 1 }}
                    />
                  )}
                </Box>
              </Stack>
            ) : (
              <Box sx={{ display: 'flex', alignItems: 'center', height: '100%' }}>
                <Chip
                  label={r.status === 'no_match' ? 'No match found' : `Error: ${r.error_message || 'Unknown'}`}
                  color={r.status === 'error' ? 'error' : 'default'}
                />
              </Box>
            )}
          </Box>
        </Stack>
      </Box>
    );
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="xl" fullWidth>
      <DialogTitle>
        Review Metadata Matches &mdash; {summary.total} books
      </DialogTitle>
      <DialogContent>
        {loading ? (
          <Box sx={{ display: 'flex', justifyContent: 'center', p: 4 }}>
            <CircularProgress />
          </Box>
        ) : (
          <>
            {/* Stats chips */}
            <Stack direction="row" spacing={1} sx={{ mb: 2 }}>
              <Chip label={`${summary.matched} matched`} color="success" size="small" />
              <Chip label={`${summary.no_match} no match`} size="small" />
              <Chip label={`${summary.errors} errors`} color="error" size="small" />
            </Stack>

            {/* Confidence slider */}
            <Stack direction="row" spacing={2} alignItems="center" sx={{ mb: 2 }}>
              <Typography variant="body2" sx={{ whiteSpace: 'nowrap' }}>
                Min confidence: {confidenceThreshold}%
              </Typography>
              <Slider
                value={confidenceThreshold}
                onChange={(_, v) => setConfidenceThreshold(v as number)}
                min={0}
                max={100}
                sx={{ maxWidth: 300 }}
              />
            </Stack>

            {/* Source filter chips */}
            <Stack direction="row" spacing={1} sx={{ mb: 2, flexWrap: 'wrap' }}>
              <Chip
                label={`All (${results.length})`}
                size="small"
                variant={sourceFilter === null ? 'filled' : 'outlined'}
                onClick={() => setSourceFilter(null)}
              />
              {Object.entries(sourceCounts).map(([source, count]) => (
                <Chip
                  key={source}
                  label={`${source} (${count})`}
                  size="small"
                  color={SOURCE_COLORS[source] || 'default'}
                  variant={sourceFilter === source ? 'filled' : 'outlined'}
                  onClick={() => setSourceFilter(sourceFilter === source ? null : source)}
                />
              ))}
            </Stack>

            {/* View toggle */}
            <Stack direction="row" spacing={2} alignItems="center" sx={{ mb: 2 }}>
              <ToggleButtonGroup
                size="small"
                value={viewMode}
                exclusive
                onChange={(_, v) => v && setViewMode(v)}
              >
                <ToggleButton value="compact">Compact</ToggleButton>
                <ToggleButton value="two-column">Two-Column</ToggleButton>
              </ToggleButtonGroup>
            </Stack>

            {/* Smart action buttons */}
            <Stack direction="row" spacing={1} sx={{ mb: 2 }}>
              <Tooltip title={`Apply ${highConfidenceIds.length} high-confidence matches with narrator`}>
                <span>
                  <Button
                    size="small"
                    variant="outlined"
                    color="success"
                    disabled={applying || highConfidenceIds.length === 0}
                    onClick={() => handleBulkApply(highConfidenceIds)}
                  >
                    Apply High Confidence ({highConfidenceIds.length})
                  </Button>
                </span>
              </Tooltip>
              <Tooltip title={`Apply all ${allVisiblePendingIds.length} visible pending matches`}>
                <span>
                  <Button
                    size="small"
                    variant="outlined"
                    disabled={applying || allVisiblePendingIds.length === 0}
                    onClick={() => handleBulkApply(allVisiblePendingIds)}
                  >
                    Apply All Visible ({allVisiblePendingIds.length})
                  </Button>
                </span>
              </Tooltip>
              <Button size="small" variant="outlined" color="warning" onClick={handleSkipAllUnmatched}>
                Skip All Unmatched
              </Button>
            </Stack>

            {/* Results list */}
            <Box sx={{ maxHeight: '60vh', overflow: 'auto' }}>
              {filteredResults.length === 0 ? (
                <Typography variant="body2" color="text.secondary" sx={{ p: 2, textAlign: 'center' }}>
                  No results match current filters
                </Typography>
              ) : viewMode === 'compact' ? (
                filteredResults.map(renderCompactRow)
              ) : (
                filteredResults.map(renderTwoColumnCard)
              )}
            </Box>
          </>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Close</Button>
        <Button
          variant="contained"
          disabled={selectedIds.size === 0 || applying}
          onClick={() => handleBulkApply(Array.from(selectedIds))}
        >
          {applying ? (
            <CircularProgress size={20} sx={{ mr: 1 }} />
          ) : null}
          Apply Selected ({selectedIds.size})
        </Button>
      </DialogActions>
    </Dialog>
  );
}
