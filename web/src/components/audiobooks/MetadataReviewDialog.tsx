// file: web/src/components/audiobooks/MetadataReviewDialog.tsx
// version: 1.7.0
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
import RefreshIcon from '@mui/icons-material/Refresh';
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

const PAGE_SIZE_OPTIONS = [25, 50, 100, 250, 500];
const LANGUAGE_FILTER_KEY = 'metadata-review-language-filter';

function loadLanguageFilter(): boolean {
  if (typeof window === 'undefined') return true;
  const raw = window.localStorage.getItem(LANGUAGE_FILTER_KEY);
  return raw === null ? true : raw === 'true';
}

function normalizeLanguage(lang: string | undefined | null): string {
  if (!lang) return '';
  const s = lang.trim().toLowerCase();
  if (!s) return '';
  const canonical: Record<string, string> = {
    english: 'en', eng: 'en', spanish: 'es', spa: 'es', french: 'fr', fre: 'fr', fra: 'fr',
    german: 'de', ger: 'de', deu: 'de', italian: 'it', ita: 'it', japanese: 'ja', jpn: 'ja',
    chinese: 'zh', chi: 'zh', zho: 'zh', mandarin: 'zh', portuguese: 'pt', por: 'pt',
    russian: 'ru', rus: 'ru', dutch: 'nl', nld: 'nl', korean: 'ko', kor: 'ko', arabic: 'ar', ara: 'ar',
  };
  if (canonical[s]) return canonical[s];
  if (s.length === 2) return s;
  return s;
}

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

function ratingChip(label: string, rating?: number, count?: number) {
  if (rating === undefined && count === undefined) return null;
  const value = rating !== undefined ? rating.toFixed(1) : 'n/a';
  const suffix = count !== undefined ? ` (${count})` : '';
  return <Chip label={`${label}: ${value}${suffix}`} size="small" variant="outlined" sx={{ mt: 0.5, mr: 0.5 }} />;
}

export function MetadataReviewDialog({ open, operationId, onClose, onComplete, toast }: MetadataReviewDialogProps) {
  const [results, setResults] = useState<CandidateResult[]>([]);
  const [loading, setLoading] = useState(true);
  const [totalCount, setTotalCount] = useState(0);
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [rowStates, setRowStates] = useState<Map<string, 'pending' | 'applied' | 'rejected' | 'skipped'>>(new Map());
  const [sourceFilter, setSourceFilter] = useState<string | null>(null);
  const [confidenceThreshold, setConfidenceThreshold] = useState(85);
  const [viewMode, setViewMode] = useState<'compact' | 'two-column'>('compact');
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [applying, setApplying] = useState(false);
  const [summary, setSummary] = useState({ matched: 0, no_match: 0, errors: 0, total: 0 });
  const [totalSummary, setTotalSummary] = useState<{ matched: number; no_match: number; errors: number } | null>(null);
  const [previewCover, setPreviewCover] = useState<string | null>(null);
  const [serverPage, setServerPage] = useState(1);
  const [reviewPageSize, setReviewPageSize] = useState<number>(loadReviewPageSize);
  const [displayPage, setDisplayPage] = useState(1);
  const [hideApplied, setHideApplied] = useState(true);
  const [hideRejected, setHideRejected] = useState(true);
  const [hideNoMatch, setHideNoMatch] = useState(true);
  const [hideSkipped, setHideSkipped] = useState(false);
  const [matchLanguage, setMatchLanguage] = useState<boolean>(loadLanguageFilter);
  const [titleFilter, setTitleFilter] = useState('');
  const [refreshKey, setRefreshKey] = useState(0);
  const fetchIdRef = useRef(0);
  const hasChangesRef = useRef(false);
  const handleClose = useCallback(() => { if (hasChangesRef.current) { onComplete(); hasChangesRef.current = false; } onClose(); }, [onComplete, onClose]);
  const serverBatchSize = Math.min(Math.max(reviewPageSize * 3, 300), 2000);

  useEffect(() => {
    if (!open || !operationId) return;
    setLoading(true);
    const fetchId = ++fetchIdRef.current;
    const offset = (serverPage - 1) * serverBatchSize;
    api.getOperationResults(operationId, serverBatchSize, offset).then((data) => {
      if (fetchId !== fetchIdRef.current) return;
      const pageResults = data.results || [];
      const pageStates = new Map<string, 'pending' | 'applied' | 'rejected' | 'skipped'>();
      for (const r of pageResults) if (r.status === 'rejected') pageStates.set(r.book.id, 'rejected');
      setRowStates((prev) => { const merged = new Map(prev); pageStates.forEach((v, k) => { if (!merged.has(k)) merged.set(k, v); }); return merged; });
      setResults(pageResults);
      const tc = data.total_count ?? data.total ?? pageResults.length;
      setTotalCount(tc);
      setSummary({ matched: data.matched ?? pageResults.filter((r) => r.status === 'matched').length, no_match: data.no_match ?? pageResults.filter((r) => r.status === 'no_match').length, errors: data.errors ?? pageResults.filter((r) => r.status === 'error').length, total: tc });
      if (data.total_matched !== undefined) setTotalSummary({ matched: data.total_matched, no_match: data.total_no_match ?? 0, errors: data.total_errors ?? 0 });
      setLoading(false);
    }).catch(() => { if (fetchId !== fetchIdRef.current) return; setLoading(false); });
  }, [open, operationId, serverPage, serverBatchSize, refreshKey]);

  const titleRegex = (() => { if (!titleFilter) return null; try { return new RegExp(titleFilter, 'i'); } catch { return null; } })();
  const filteredResults = results.filter((r) => !titleRegex || titleRegex.test(r.book.title || '')).filter((r) => !sourceFilter || r.candidate?.source === sourceFilter).filter((r) => (r.status === 'matched' && r.candidate && r.candidate.score * 100 >= confidenceThreshold) || r.status !== 'matched').filter((r) => !hideApplied || rowStates.get(r.book.id) !== 'applied').filter((r) => !hideRejected || rowStates.get(r.book.id) !== 'rejected').filter((r) => !hideSkipped || rowStates.get(r.book.id) !== 'skipped').filter((r) => !hideNoMatch || (r.status !== 'no_match' && r.status !== 'error')).filter((r) => { if (!matchLanguage || !r.candidate) return true; const bookLang = normalizeLanguage(r.book.language); const candLang = normalizeLanguage(r.candidate.language); if (!bookLang || !candLang) return true; return bookLang === candLang; });

  const displayPageCount = Math.max(1, Math.ceil(filteredResults.length / reviewPageSize));
  const clampedDisplayPage = Math.min(displayPage, displayPageCount);
  const pageResults = filteredResults.slice((clampedDisplayPage - 1) * reviewPageSize, clampedDisplayPage * reviewPageSize);

  const renderCandidateRatings = (r: CandidateResult) => {
    const c = r.candidate as unknown as {
      audible_rating?: number; audible_rating_count?: number; google_books_rating?: number; google_books_rating_count?: number;
    };
    return (
      <>
        {ratingChip('Audible', c?.audible_rating, c?.audible_rating_count)}
        {ratingChip('Google', c?.google_books_rating, c?.google_books_rating_count)}
      </>
    );
  };

  return <Dialog open={open} onClose={(_e, reason) => { if (reason === 'backdropClick' || reason === 'escapeKeyDown') return; handleClose(); }} disableEscapeKeyDown maxWidth="xl" fullWidth>
    <DialogTitle>Review Metadata Matches</DialogTitle>
    <DialogContent>{loading ? <CircularProgress /> : <Box>{pageResults.map((r) => <Box key={r.book.id}>{r.candidate && renderCandidateRatings(r)}</Box>)}</Box>}</DialogContent>
    <DialogActions><Button onClick={handleClose}>Close</Button></DialogActions>
  </Dialog>;
}
