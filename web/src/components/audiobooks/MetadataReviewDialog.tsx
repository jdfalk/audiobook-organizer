// file: web/src/components/audiobooks/MetadataReviewDialog.tsx
// version: 1.1.0
// guid: e7f8a9b0-c1d2-3e4f-5a6b-7c8d9e0f1a2b

import { useCallback, useEffect, useRef, useState } from 'react';
import {
  Avatar,
  Box,
  Button,
  Checkbox,
  Chip,
  CircularProgress,
  Dialog,
  FormControlLabel,
  IconButton,
  MenuItem,
  Pagination,
  DialogActions,
  DialogContent,
  DialogTitle,
  Slider,
  Stack,
  Switch,
  TextField,
  ToggleButton,
  ToggleButtonGroup,
  Tooltip,
  Typography,
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
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

// Matches ActivityLog's selector so users see the same options
// across pagination controls.
const PAGE_SIZE_OPTIONS = [25, 50, 100, 250];

// Language filter: when enabled (default), candidates whose
// language disagrees with the book's are hidden. Preference
// persists in localStorage so users don't re-enable it on
// every dialog open.
const LANGUAGE_FILTER_KEY = 'metadata-review-language-filter';

function loadLanguageFilter(): boolean {
  if (typeof window === 'undefined') return true;
  const raw = window.localStorage.getItem(LANGUAGE_FILTER_KEY);
  return raw === null ? true : raw === 'true';
}

// Normalizes a language string to an ISO 639-1 2-letter code
// for comparison. Mirrors the server-side metadataLanguageTag
// logic — the review dialog filter compares book.language
// against candidate.language, and both sides might come from
// different source APIs that return the language in different
// formats (full name, 2-letter code, 3-letter code).
//
// Returns the lowercased input for unknown languages so the
// filter still works via string equality.
function normalizeLanguage(lang: string | undefined | null): string {
  if (!lang) return '';
  const s = lang.trim().toLowerCase();
  if (!s) return '';
  const canonical: Record<string, string> = {
    english: 'en',
    eng: 'en',
    spanish: 'es',
    spa: 'es',
    french: 'fr',
    fre: 'fr',
    fra: 'fr',
    german: 'de',
    ger: 'de',
    deu: 'de',
    italian: 'it',
    ita: 'it',
    japanese: 'ja',
    jpn: 'ja',
    chinese: 'zh',
    chi: 'zh',
    zho: 'zh',
    mandarin: 'zh',
    portuguese: 'pt',
    por: 'pt',
    russian: 'ru',
    rus: 'ru',
    dutch: 'nl',
    nld: 'nl',
    korean: 'ko',
    kor: 'ko',
    arabic: 'ar',
    ara: 'ar',
  };
  if (canonical[s]) return canonical[s];
  if (s.length === 2) return s;
  return s;
}

// Persist the review page size in localStorage so users don't have
// to re-select it every time they open the dialog. The activity log
// uses session-only state, but the review dialog is opened ad-hoc
// many times per session — re-picking "250 per page" on every open
// is annoying.
const REVIEW_PAGE_SIZE_KEY = 'metadata-review-page-size';

function loadReviewPageSize(): number {
  if (typeof window === 'undefined') return 25;
  const raw = window.localStorage.getItem(REVIEW_PAGE_SIZE_KEY);
  const n = raw ? Number(raw) : 25;
  return PAGE_SIZE_OPTIONS.includes(n) ? n : 25;
}

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
  const [rowStates, setRowStates] = useState<
    Map<string, 'pending' | 'applied' | 'rejected' | 'skipped'>
  >(new Map());
  const [sourceFilter, setSourceFilter] = useState<string | null>(null);
  const [confidenceThreshold, setConfidenceThreshold] = useState(85);
  const [viewMode, setViewMode] = useState<'compact' | 'two-column'>('compact');
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [applying, setApplying] = useState(false);
  const [summary, setSummary] = useState({ matched: 0, no_match: 0, errors: 0, total: 0 });
  const [previewCover, setPreviewCover] = useState<string | null>(null);
  const [reviewPage, setReviewPage] = useState(1);
  const [reviewPageSize, setReviewPageSize] = useState<number>(loadReviewPageSize);
  const [hideApplied, setHideApplied] = useState(true);
  const [hideRejected, setHideRejected] = useState(true);
  const [hideNoMatch, setHideNoMatch] = useState(true);
  const [matchLanguage, setMatchLanguage] = useState<boolean>(loadLanguageFilter);

  useEffect(() => {
    if (!open || !operationId) return;
    setLoading(true);
    api
      .getOperationResults(operationId)
      .then(async (data) => {
        const results = data.results || [];

        // Detect already-applied and rejected books from stored results + current book state
        const initialStates = new Map<string, 'pending' | 'applied' | 'rejected' | 'skipped'>();
        for (const r of results) {
          if (r.status === 'rejected') {
            initialStates.set(r.book.id, 'rejected');
          }
        }

        // Check current book state for applied metadata (batch fetch current data)
        try {
          const bookIds = results.filter((r) => r.status === 'matched').map((r) => r.book.id);
          for (const id of bookIds) {
            if (initialStates.has(id)) continue;
            try {
              const book = await api.getBook(id);
              if (book.metadata_review_status === 'matched') {
                initialStates.set(id, 'applied');
              }
              // Update book info with current data (cover, title, etc.)
              const result = results.find((r) => r.book.id === id);
              if (result && book) {
                result.book.cover_url = book.cover_url || result.book.cover_url;
                result.book.title = book.title || result.book.title;
              }
            } catch {
              // Book may have been deleted
            }
          }
        } catch {
          // Ignore batch fetch errors
        }

        setRowStates(initialStates);
        setResults([...results]);
        setSummary({
          matched:
            data.matched ??
            results.filter((r: api.CandidateResult) => r.status === 'matched').length,
          no_match:
            data.no_match ??
            results.filter((r: api.CandidateResult) => r.status === 'no_match').length,
          errors:
            data.errors ?? results.filter((r: api.CandidateResult) => r.status === 'error').length,
          total: data.total ?? results.length,
        });
        setLoading(false);
      })
      .catch(() => setLoading(false));
  }, [open, operationId]);

  // Poll for new results while the operation is still running.
  // The fetch writes results to operation_results incrementally,
  // so partial results are available before the operation
  // finishes. Polling every 5s gives a responsive "results
  // streaming in" experience without hammering the backend.
  // Stops automatically when the data says the operation is done
  // (total count stabilizes) or when the dialog is closed.
  const [operationDone, setOperationDone] = useState(false);
  const prevTotalRef = useRef(0);

  useEffect(() => {
    if (!open || !operationId || loading || operationDone) return;
    const interval = setInterval(async () => {
      try {
        const data = await api.getOperationResults(operationId);
        const newResults = data.results || [];
        const newTotal = data.total ?? newResults.length;

        // If the count hasn't changed in two consecutive polls,
        // the operation is likely done. Stop polling to save
        // bandwidth. The user can always close and reopen the
        // dialog to get the final state.
        if (newTotal > 0 && newTotal === prevTotalRef.current) {
          setOperationDone(true);
          return;
        }
        prevTotalRef.current = newTotal;

        // Only update if we got more results than we currently have.
        if (newTotal > results.length) {
          setResults([...newResults]);
          setSummary({
            matched: data.matched ?? newResults.filter((r: api.CandidateResult) => r.status === 'matched').length,
            no_match: data.no_match ?? newResults.filter((r: api.CandidateResult) => r.status === 'no_match').length,
            errors: data.errors ?? newResults.filter((r: api.CandidateResult) => r.status === 'error').length,
            total: newTotal,
          });
        }
      } catch {
        // Silent — polling failure is not fatal
      }
    }, 5000);
    return () => clearInterval(interval);
  }, [open, operationId, loading, operationDone, results.length]);

  // Reset polling state when the operation changes.
  useEffect(() => {
    setOperationDone(false);
    prevTotalRef.current = 0;
  }, [operationId]);

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
        (r.status === 'matched' && r.candidate && r.candidate.score * 100 >= confidenceThreshold) ||
        r.status !== 'matched'
    )
    .filter((r) => !hideApplied || rowStates.get(r.book.id) !== 'applied')
    .filter((r) => !hideRejected || rowStates.get(r.book.id) !== 'rejected')
    .filter((r) => !hideNoMatch || (r.status !== 'no_match' && r.status !== 'error'))
    .filter((r) => {
      // Language filter: hide candidates whose language
      // doesn't match the book's. Only active when the toggle
      // is on AND both the book and candidate have a language
      // set — an unknown language on either side is a
      // no-op (show the row) so new books without metadata
      // still get candidates offered to them.
      if (!matchLanguage) return true;
      if (!r.candidate) return true;
      const bookLang = normalizeLanguage(r.book.language);
      const candLang = normalizeLanguage(r.candidate.language);
      if (!bookLang || !candLang) return true;
      return bookLang === candLang;
    });

  // Reset page when filters change
  useEffect(() => {
    setReviewPage(1);
  }, [sourceFilter, confidenceThreshold, hideApplied, hideRejected, hideNoMatch, matchLanguage]);

  // Coalesce rapid Apply clicks into one batched API call
  const applyQueueRef = useRef<string[]>([]);
  const applyTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const flushApplyQueue = useCallback(async () => {
    const ids = [...applyQueueRef.current];
    applyQueueRef.current = [];
    if (ids.length === 0) return;
    try {
      await api.batchApplyCandidates(operationId, ids);
      toast(`Applied metadata to ${ids.length} book${ids.length > 1 ? 's' : ''}`, 'success', {
        label: 'Undo',
        onClick: async () => {
          for (const id of ids) {
            try {
              await api.undoLastApply(id);
            } catch {
              /* ignore */
            }
          }
          setRowStates((prev) => {
            const next = new Map(prev);
            ids.forEach((id) => next.set(id, 'pending'));
            return next;
          });
          toast(`Undid ${ids.length} apply(s)`, 'info');
        },
      });
    } catch {
      // Revert optimistic updates
      setRowStates((prev) => {
        const next = new Map(prev);
        ids.forEach((id) => next.set(id, 'pending'));
        return next;
      });
      toast('Failed to apply', 'error');
    }
  }, [operationId, toast]);

  const handleApplyOne = (bookId: string) => {
    // Optimistic UI update
    setRowStates((prev) => new Map(prev).set(bookId, 'applied'));
    // Queue for batched API call
    applyQueueRef.current.push(bookId);
    if (applyTimerRef.current) clearTimeout(applyTimerRef.current);
    applyTimerRef.current = setTimeout(flushApplyQueue, 500);
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
    setRowStates((prev) => {
      const current = prev.get(bookId);
      // Toggle: skip ↔ pending
      return new Map(prev).set(bookId, current === 'skipped' ? 'pending' : 'skipped');
    });
  };

  const handleReject = async (bookId: string) => {
    try {
      await api.batchRejectCandidates(operationId, [bookId]);
      setRowStates((prev) => new Map(prev).set(bookId, 'rejected'));
      toast('Candidate rejected — will be excluded from future fetches', 'info', {
        label: 'Undo',
        onClick: async () => {
          try {
            await api.batchUnrejectCandidates(operationId, [bookId]);
            setRowStates((prev) => new Map(prev).set(bookId, 'pending'));
            toast('Rejection undone', 'success');
          } catch {
            toast('Failed to undo rejection', 'error');
          }
        },
      });
    } catch {
      toast('Failed to reject', 'error');
    }
  };

  const handleUnreject = async (bookId: string) => {
    try {
      await api.batchUnrejectCandidates(operationId, [bookId]);
      setRowStates((prev) => new Map(prev).set(bookId, 'pending'));
      toast('Rejection undone', 'success');
    } catch {
      toast('Failed to undo rejection', 'error');
    }
  };

  // Current page slice — buttons should only act on what's visible on screen
  const pageResults = filteredResults.slice(
    (reviewPage - 1) * reviewPageSize,
    reviewPage * reviewPageSize
  );

  const highConfidenceIds = pageResults
    .filter(
      (r) =>
        r.status === 'matched' &&
        r.candidate &&
        r.candidate.score * 100 >= confidenceThreshold &&
        r.candidate.narrator &&
        !['applied', 'skipped', 'rejected'].includes(rowStates.get(r.book.id) || '')
    )
    .map((r) => r.book.id);

  const allVisiblePendingIds = pageResults
    .filter(
      (r) =>
        r.status === 'matched' &&
        r.candidate &&
        !['applied', 'skipped', 'rejected'].includes(rowStates.get(r.book.id) || '')
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
      return {
        bgcolor: 'action.disabledBackground',
        opacity: 0.5,
        borderRadius: 1,
        transition: 'all 0.3s',
      };
    return { borderRadius: 1, transition: 'all 0.3s' };
  };

  const isRowActionable = (bookId: string) => {
    const state = rowStates.get(bookId);
    return state !== 'applied' && state !== 'rejected';
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
            sx={{ width: 40, height: 50, cursor: 'pointer' }}
            onClick={(e) => {
              e.stopPropagation();
              setPreviewCover(r.candidate?.cover_url || r.book.cover_url || '');
            }}
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
                color="error"
                onClick={(e) => {
                  e.stopPropagation();
                  handleReject(bookId);
                }}
              >
                Reject
              </Button>
              <Button
                size="small"
                variant="text"
                onClick={(e) => {
                  e.stopPropagation();
                  handleSkip(bookId);
                }}
              >
                Skip
              </Button>
            </>
          )}
          {rowStates.get(bookId) === 'skipped' && (
            <Chip
              label="Skipped"
              size="small"
              onClick={(e) => {
                e.stopPropagation();
                handleSkip(bookId);
              }}
              sx={{ cursor: 'pointer' }}
            />
          )}
          {rowStates.get(bookId) === 'applied' && (
            <Chip label="Applied" size="small" color="success" />
          )}
          {rowStates.get(bookId) === 'rejected' && (
            <Chip
              label="Rejected — click to undo"
              size="small"
              color="error"
              onClick={(e) => {
                e.stopPropagation();
                handleUnreject(bookId);
              }}
              sx={{ cursor: 'pointer' }}
            />
          )}
        </Stack>

        {/* Expanded two-column detail for this row */}
        {isExpanded && r.candidate && (
          <Stack
            direction="row"
            spacing={2}
            sx={{ p: 2, pl: 7, bgcolor: 'action.hover', borderRadius: 1 }}
          >
            <Box sx={{ flex: 1 }}>
              <Typography variant="subtitle2" gutterBottom>
                Current
              </Typography>
              <Stack direction="row" spacing={1} alignItems="flex-start">
                <Avatar
                  src={r.book.cover_url || ''}
                  variant="rounded"
                  sx={{ width: 60, height: 80, cursor: r.book.cover_url ? 'pointer' : 'default' }}
                  onClick={() => r.book.cover_url && setPreviewCover(r.book.cover_url)}
                />
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
                    <Typography
                      variant="caption"
                      color="info.main"
                      display="block"
                      sx={{ wordBreak: 'break-all' }}
                    >
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
                <Avatar
                  src={r.candidate.cover_url || ''}
                  variant="rounded"
                  sx={{
                    width: 60,
                    height: 80,
                    cursor: r.candidate?.cover_url ? 'pointer' : 'default',
                  }}
                  onClick={() => r.candidate?.cover_url && setPreviewCover(r.candidate.cover_url)}
                />
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
                      {r.candidate.series_position
                        ? ` \u00b7 Book ${r.candidate.series_position}`
                        : ''}
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
                    color={
                      r.candidate.score >= 0.85
                        ? 'success'
                        : r.candidate.score >= 0.6
                          ? 'warning'
                          : 'default'
                    }
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
              <Avatar
                src={r.book.cover_url || ''}
                variant="rounded"
                sx={{ width: 60, height: 80, cursor: r.book.cover_url ? 'pointer' : 'default' }}
                onClick={() => r.book.cover_url && setPreviewCover(r.book.cover_url)}
              />
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
                  <Typography
                    variant="caption"
                    color="info.main"
                    display="block"
                    sx={{ wordBreak: 'break-all' }}
                  >
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
                <Avatar
                  src={r.candidate.cover_url || ''}
                  variant="rounded"
                  sx={{
                    width: 60,
                    height: 80,
                    cursor: r.candidate?.cover_url ? 'pointer' : 'default',
                  }}
                  onClick={() => r.candidate?.cover_url && setPreviewCover(r.candidate.cover_url)}
                />
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
                      {r.candidate.series_position
                        ? ` \u00b7 Book ${r.candidate.series_position}`
                        : ''}
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
                      <Button
                        size="small"
                        variant="contained"
                        color="success"
                        onClick={() => handleApplyOne(bookId)}
                      >
                        Apply
                      </Button>
                      <Button
                        size="small"
                        variant="outlined"
                        color="error"
                        onClick={() => handleReject(bookId)}
                      >
                        Reject
                      </Button>
                      <Button size="small" variant="text" onClick={() => handleSkip(bookId)}>
                        Skip
                      </Button>
                    </Stack>
                  )}
                  {rowStates.get(bookId) === 'skipped' && (
                    <Chip
                      label="Skipped — click to undo"
                      size="small"
                      onClick={() => handleSkip(bookId)}
                      sx={{ cursor: 'pointer', mt: 1 }}
                    />
                  )}
                  {rowStates.get(bookId) === 'rejected' && (
                    <Chip
                      label="Rejected — click to undo"
                      size="small"
                      color="error"
                      onClick={() => handleUnreject(bookId)}
                      sx={{ cursor: 'pointer', mt: 1 }}
                    />
                  )}
                  {rowStates.get(bookId) === 'applied' && (
                    <Chip label="Applied" size="small" color="success" sx={{ mt: 1 }} />
                  )}
                </Box>
              </Stack>
            ) : (
              <Box sx={{ display: 'flex', alignItems: 'center', height: '100%' }}>
                <Chip
                  label={
                    r.status === 'no_match'
                      ? 'No match found'
                      : `Error: ${r.error_message || 'Unknown'}`
                  }
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
    <>
      <Dialog
        open={open}
        // Ignore backdrop clicks so the dialog stays open while
        // the user is reviewing. The only way to close is the
        // explicit × button or the Done action. This prevents
        // the "accidentally clicked outside, lost all my review
        // state, had to re-query and wait for it to load again"
        // problem. Escape key is also suppressed — reviewing
        // 1000 candidates is a long workflow where the user
        // reaches for the keyboard often and hitting Esc by
        // accident should not blow away their session.
        onClose={(_event, reason) => {
          if (reason === 'backdropClick' || reason === 'escapeKeyDown') return;
          onClose();
        }}
        disableEscapeKeyDown
        maxWidth="xl"
        fullWidth
      >
        <DialogTitle sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <span>Review Metadata Matches &mdash; {summary.total} books</span>
          <IconButton
            onClick={onClose}
            size="small"
            aria-label="close review dialog"
            sx={{ ml: 2 }}
          >
            <CloseIcon />
          </IconButton>
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
                  max={300}
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

              {/* View toggle + hide filters */}
              <Stack direction="row" spacing={2} alignItems="center" sx={{ mb: 2 }} flexWrap="wrap">
                <ToggleButtonGroup
                  size="small"
                  value={viewMode}
                  exclusive
                  onChange={(_, v) => v && setViewMode(v)}
                >
                  <ToggleButton value="compact">Compact</ToggleButton>
                  <ToggleButton value="two-column">Two-Column</ToggleButton>
                </ToggleButtonGroup>
                <FormControlLabel
                  control={
                    <Switch
                      size="small"
                      checked={hideApplied}
                      onChange={(e) => setHideApplied(e.target.checked)}
                    />
                  }
                  label={<Typography variant="body2">Hide Applied</Typography>}
                />
                <FormControlLabel
                  control={
                    <Switch
                      size="small"
                      checked={hideRejected}
                      onChange={(e) => setHideRejected(e.target.checked)}
                    />
                  }
                  label={<Typography variant="body2">Hide Rejected</Typography>}
                />
                <FormControlLabel
                  control={
                    <Switch
                      size="small"
                      checked={hideNoMatch}
                      onChange={(e) => setHideNoMatch(e.target.checked)}
                    />
                  }
                  label={<Typography variant="body2">Hide No Match</Typography>}
                />
                <Tooltip title="Hide candidates whose language doesn't match the book's current language. Books without a language set still show all candidates.">
                  <FormControlLabel
                    control={
                      <Switch
                        size="small"
                        checked={matchLanguage}
                        onChange={(e) => {
                          const next = e.target.checked;
                          setMatchLanguage(next);
                          if (typeof window !== 'undefined') {
                            window.localStorage.setItem(
                              LANGUAGE_FILTER_KEY,
                              String(next)
                            );
                          }
                        }}
                      />
                    }
                    label={<Typography variant="body2">Match Language</Typography>}
                  />
                </Tooltip>
              </Stack>

              {/* Smart action buttons */}
              <Stack direction="row" spacing={1} sx={{ mb: 2 }}>
                <Tooltip
                  title={`Apply ${highConfidenceIds.length} high-confidence matches with narrator`}
                >
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
                <Tooltip
                  title={`Apply all ${allVisiblePendingIds.length} pending matches on this page`}
                >
                  <span>
                    <Button
                      size="small"
                      variant="outlined"
                      disabled={applying || allVisiblePendingIds.length === 0}
                      onClick={() => handleBulkApply(allVisiblePendingIds)}
                    >
                      Apply Page ({allVisiblePendingIds.length})
                    </Button>
                  </span>
                </Tooltip>
                <Button
                  size="small"
                  variant="outlined"
                  color="warning"
                  onClick={handleSkipAllUnmatched}
                >
                  Skip All Unmatched
                </Button>
              </Stack>

              {/* Results list (paginated) */}
              {filteredResults.length > 0 && (
                <Stack
                  direction="row"
                  justifyContent="space-between"
                  alignItems="center"
                  sx={{ mb: 1 }}
                >
                  <Typography variant="caption" color="text.secondary">
                    Showing{' '}
                    {Math.min((reviewPage - 1) * reviewPageSize + 1, filteredResults.length)}-
                    {Math.min(reviewPage * reviewPageSize, filteredResults.length)} of{' '}
                    {filteredResults.length}
                  </Typography>
                  <Stack direction="row" spacing={1} alignItems="center">
                    <Pagination
                      count={Math.ceil(filteredResults.length / reviewPageSize)}
                      page={reviewPage}
                      onChange={(_, p) => setReviewPage(p)}
                      size="small"
                    />
                    <TextField
                      select
                      size="small"
                      value={reviewPageSize}
                      onChange={(e) => {
                        const next = Number(e.target.value);
                        setReviewPageSize(next);
                        setReviewPage(1);
                        if (typeof window !== 'undefined') {
                          window.localStorage.setItem(REVIEW_PAGE_SIZE_KEY, String(next));
                        }
                      }}
                      sx={{ minWidth: 100 }}
                    >
                      {PAGE_SIZE_OPTIONS.map((n) => (
                        <MenuItem key={n} value={n}>
                          {n} / page
                        </MenuItem>
                      ))}
                    </TextField>
                  </Stack>
                </Stack>
              )}
              <Box sx={{ maxHeight: '60vh', overflow: 'auto' }}>
                {filteredResults.length === 0 ? (
                  <Typography
                    variant="body2"
                    color="text.secondary"
                    sx={{ p: 2, textAlign: 'center' }}
                  >
                    No results match current filters
                  </Typography>
                ) : viewMode === 'compact' ? (
                  pageResults.map(renderCompactRow)
                ) : (
                  pageResults.map(renderTwoColumnCard)
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
            {applying ? <CircularProgress size={20} sx={{ mr: 1 }} /> : null}
            Apply Selected ({selectedIds.size})
          </Button>
        </DialogActions>
      </Dialog>

      {/* Cover preview lightbox */}
      {previewCover && (
        <Box
          onClick={() => setPreviewCover(null)}
          sx={{
            position: 'fixed',
            inset: 0,
            zIndex: 2000,
            bgcolor: 'rgba(0,0,0,0.85)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            cursor: 'pointer',
          }}
        >
          <Box
            component="img"
            src={previewCover}
            alt="Cover preview"
            sx={{ maxWidth: '80vw', maxHeight: '80vh', borderRadius: 2, boxShadow: 8 }}
          />
        </Box>
      )}
    </>
  );
}
