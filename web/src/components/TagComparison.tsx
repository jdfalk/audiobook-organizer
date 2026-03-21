// file: web/src/components/TagComparison.tsx
// version: 1.4.0
// guid: cfed2692-76f6-47b0-bc84-cc2a4075e554

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  Alert,
  Box,
  Chip,
  Collapse,
  FormControl,
  IconButton,
  InputLabel,
  LinearProgress,
  MenuItem,
  Select,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Tooltip,
  Typography,
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close.js';
import VisibilityOffIcon from '@mui/icons-material/VisibilityOff.js';
import RestoreIcon from '@mui/icons-material/Restore.js';
import type { Book, BookTags, TagSourceValues } from '../services/api';
import * as api from '../services/api';

interface TagComparisonProps {
  bookId: string;
  versions: Book[];
  refreshKey?: number;
  snapshotTimestamp?: string | null;
  onClearSnapshot?: () => void;
}

/** Key tags we always show badges for */
const KEY_TAGS = ['title', 'author_name', 'narrator', 'series_name', 'publisher', 'language', 'isbn13'] as const;

const TAG_LABELS: Record<string, string> = {
  title: 'title',
  author_name: 'author',
  narrator: 'narrator',
  series_name: 'series',
  publisher: 'publisher',
  language: 'language',
  isbn13: 'isbn',
  isbn10: 'isbn10',
  audiobook_release_year: 'year',
  description: 'description',
  asin: 'asin',
  edition: 'edition',
  print_year: 'print year',
  series_index: 'series #',
  album: 'album',
  genre: 'genre',
};

export const TagComparison = ({ bookId, versions, refreshKey, snapshotTimestamp, onClearSnapshot }: TagComparisonProps) => {
  const [tags, setTags] = useState<BookTags | null>(null);
  const [loading, setLoading] = useState(false);
  const [compareId, setCompareId] = useState<string>('');
  const [expanded, setExpanded] = useState(false);
  const [hiddenTags, setHiddenTags] = useState<Set<string>>(new Set());
  const [colWidths, setColWidths] = useState<Record<number, number>>({});
  const resizingCol = useRef<number | null>(null);
  const resizeStartX = useRef(0);
  const resizeStartWidth = useRef(0);
  const tableRef = useRef<HTMLTableElement | null>(null);

  const snapshotComparisonActive = Boolean(snapshotTimestamp) && compareId === '';

  const loadTags = useCallback(async () => {
    setLoading(true);
    try {
      const result = await api.getBookTags(
        bookId,
        compareId || undefined,
        snapshotComparisonActive ? snapshotTimestamp ?? undefined : undefined
      );
      setTags(result);
    } catch {
      setTags(null);
    } finally {
      setLoading(false);
    }
  }, [bookId, compareId, snapshotComparisonActive, snapshotTimestamp]);

  useEffect(() => {
    loadTags();
  }, [loadTags, refreshKey]);

  useEffect(() => {
    if (snapshotTimestamp) {
      setExpanded(true);
      setCompareId('');
      loadTags();
    }
  }, [snapshotTimestamp, loadTags]);

  const tagEntries = useMemo(() => {
    if (!tags?.tags) return [];
    return Object.entries(tags.tags);
  }, [tags]);

  const visibleTagEntries = useMemo(
    () => tagEntries.filter(([name]) => !hiddenTags.has(name)),
    [tagEntries, hiddenTags]
  );

  const keyTagStatus = useMemo(() => {
    const status: Record<string, boolean> = {};
    for (const key of KEY_TAGS) {
      const entry = tags?.tags?.[key];
      const val = entry?.file_value;
      status[key] = val != null && val !== '' && val !== false;
    }
    return status;
  }, [tags]);

  const otherVersions = useMemo(
    () => versions.filter((v) => v.id !== bookId),
    [versions, bookId]
  );

  const hasComparison = compareId !== '' || snapshotComparisonActive;

  // Column resize handlers
  const handleResizeStart = (colIndex: number, e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    resizingCol.current = colIndex;
    resizeStartX.current = e.clientX;

    if (tableRef.current) {
      const headerCells = tableRef.current.querySelectorAll('thead th');
      if (headerCells[colIndex]) {
        resizeStartWidth.current = headerCells[colIndex].getBoundingClientRect().width;
      }
    }

    const handleMouseMove = (ev: MouseEvent) => {
      if (resizingCol.current === null) return;
      const delta = ev.clientX - resizeStartX.current;
      const newWidth = Math.max(60, resizeStartWidth.current + delta);
      setColWidths((prev) => ({ ...prev, [resizingCol.current!]: newWidth }));
    };

    const handleMouseUp = () => {
      resizingCol.current = null;
      document.removeEventListener('mousemove', handleMouseMove);
      document.removeEventListener('mouseup', handleMouseUp);
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
    };

    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';
    document.addEventListener('mousemove', handleMouseMove);
    document.addEventListener('mouseup', handleMouseUp);
  };

  const resizeHandle = (colIndex: number) => (
    <Box
      sx={{
        position: 'absolute',
        right: -3,
        top: 0,
        bottom: 0,
        width: 6,
        cursor: 'col-resize',
        zIndex: 1,
        '&:hover': { bgcolor: 'primary.main', opacity: 0.4 },
      }}
      onMouseDown={(e) => handleResizeStart(colIndex, e)}
    />
  );

  // Helper to get cell color for a specific tag+source combination
  const getCellStyle = (tagValues: TagSourceValues, source: 'file' | 'db' | 'comparison') => {
    const fileVal = tagValues.file_value != null ? String(tagValues.file_value) : '';
    const storedVal = tagValues.stored_value != null ? String(tagValues.stored_value) : '';
    const compVal = (tagValues as TagSourceValues & { comparison_value?: string | number | boolean | null }).comparison_value;
    const compStr = compVal != null ? String(compVal) : '';

    if (source === 'db') {
      if (storedVal && !fileVal) return { color: '#4caf50' }; // green: in DB but not file
      if (fileVal && storedVal && fileVal !== storedVal) return { color: '#ff9800' }; // amber: differs
    }
    if (source === 'comparison' && hasComparison) {
      if (compStr && fileVal && compStr !== fileVal) return { color: '#ef5350' }; // red: differs
      if (fileVal && !compStr) return { color: '#4caf50' }; // green: present here, missing there
    }
    return {};
  };

  const clearComparison = () => {
    setCompareId('');
    onClearSnapshot?.();
  };

  return (
    <Box>
      {/* Key tag badges */}
      <Stack direction="row" spacing={0.5} flexWrap="wrap" useFlexGap sx={{ mb: 1 }}>
        {KEY_TAGS.map((key) => (
          <Chip
            key={key}
            label={`${keyTagStatus[key] ? '\u2713' : '\u2717'} ${TAG_LABELS[key] || key}`}
            size="small"
            color={keyTagStatus[key] ? 'success' : 'default'}
            variant={keyTagStatus[key] ? 'filled' : 'outlined'}
            sx={{ fontSize: '0.75rem' }}
          />
        ))}
      </Stack>

      {/* Toggle for full comparison */}
      <Typography
        variant="body2"
        color="primary"
        sx={{ cursor: 'pointer', mb: 1, '&:hover': { textDecoration: 'underline' } }}
        onClick={() => setExpanded(!expanded)}
        data-testid="tag-comparison-toggle"
      >
        {expanded ? 'Hide full tag comparison' : 'View full tag comparison \u2192'}
      </Typography>

      <Collapse in={expanded}>
        {/* Snapshot/comparison banner with dismiss */}
        {(snapshotComparisonActive || compareId) && (
          <Alert
            severity="info"
            sx={{ mb: 2 }}
            data-testid="snapshot-comparison-banner"
            action={
              <IconButton size="small" onClick={clearComparison}>
                <CloseIcon fontSize="small" />
              </IconButton>
            }
          >
            {snapshotComparisonActive
              ? `Comparing against snapshot from ${new Date(snapshotTimestamp ?? '').toLocaleString()}`
              : `Comparing against version`}
          </Alert>
        )}

        {/* Comparison dropdown */}
        <Stack direction="row" spacing={1} alignItems="center" sx={{ mb: 2 }}>
          {otherVersions.length > 0 && (
            <FormControl size="small" sx={{ minWidth: 280 }}>
              <InputLabel>Compare against</InputLabel>
              <Select
                value={compareId}
                label="Compare against"
                onChange={(e) => { setCompareId(e.target.value); if (e.target.value) onClearSnapshot?.(); }}
                data-testid="tag-comparison-select"
              >
                <MenuItem value="">None</MenuItem>
                {otherVersions.map((v) => (
                  <MenuItem key={v.id} value={v.id}>
                    {v.title || 'Untitled'}{v.format ? ` (${v.format.toUpperCase()})` : ''}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
          )}
        </Stack>

        {loading && <LinearProgress sx={{ mb: 1 }} />}

        {/* Hidden tags restore bar */}
        {hiddenTags.size > 0 && (
          <Stack direction="row" spacing={0.5} flexWrap="wrap" useFlexGap alignItems="center" sx={{ mb: 1 }}>
            <Typography variant="caption" color="text.secondary">
              {hiddenTags.size} hidden:
            </Typography>
            {Array.from(hiddenTags).map((tag) => (
              <Chip
                key={tag}
                label={TAG_LABELS[tag] || tag}
                size="small"
                variant="outlined"
                onDelete={() => setHiddenTags((prev) => { const next = new Set(prev); next.delete(tag); return next; })}
                sx={{ fontSize: '0.7rem' }}
              />
            ))}
            <Chip
              label="Show all"
              size="small"
              color="primary"
              variant="outlined"
              icon={<RestoreIcon sx={{ fontSize: 14 }} />}
              onClick={() => setHiddenTags(new Set())}
              sx={{ fontSize: '0.7rem' }}
            />
          </Stack>
        )}

        {/* Transposed table: tags as columns, sources as rows */}
        {visibleTagEntries.length > 0 && (
          <Box sx={{ overflowX: 'auto' }}>
            <Table
              size="small"
              ref={tableRef}
              sx={{ '& td, & th': { py: 0.75, px: 1.5, whiteSpace: 'nowrap' } }}
            >
              <TableHead>
                <TableRow>
                  {/* Row label column */}
                  <TableCell sx={{ fontWeight: 'bold', position: 'sticky', left: 0, bgcolor: 'background.paper', zIndex: 2, minWidth: 100 }}>
                    Source
                  </TableCell>
                  {/* One column per tag */}
                  {visibleTagEntries.map(([tagName], colIdx) => (
                    <TableCell
                      key={tagName}
                      sx={{
                        fontWeight: 'bold',
                        position: 'relative',
                        minWidth: 80,
                        ...(colWidths[colIdx] ? { width: colWidths[colIdx] } : {}),
                      }}
                    >
                      <Stack direction="row" alignItems="center" spacing={0.5}>
                        <span>{TAG_LABELS[tagName] || tagName}</span>
                        <Tooltip title={`Hide "${TAG_LABELS[tagName] || tagName}"`}>
                          <IconButton
                            size="small"
                            onClick={() => setHiddenTags((prev) => new Set(prev).add(tagName))}
                            sx={{ opacity: 0.3, '&:hover': { opacity: 1 }, p: 0 }}
                          >
                            <VisibilityOffIcon sx={{ fontSize: 12 }} />
                          </IconButton>
                        </Tooltip>
                      </Stack>
                      {resizeHandle(colIdx)}
                    </TableCell>
                  ))}
                </TableRow>
              </TableHead>
              <TableBody>
                {/* File Value row */}
                <TableRow>
                  <TableCell sx={{ fontWeight: 'bold', color: 'text.secondary', position: 'sticky', left: 0, bgcolor: 'background.paper', zIndex: 1 }}>
                    File
                  </TableCell>
                  {visibleTagEntries.map(([tagName, tagValues]) => {
                    const val = tagValues.file_value != null ? String(tagValues.file_value) : '\u2014';
                    return (
                      <TableCell key={tagName} sx={{ fontSize: '0.85rem' }}>
                        {val}
                      </TableCell>
                    );
                  })}
                </TableRow>

                {/* DB Value row */}
                <TableRow>
                  <TableCell sx={{ fontWeight: 'bold', color: 'text.secondary', position: 'sticky', left: 0, bgcolor: 'background.paper', zIndex: 1 }}>
                    DB
                  </TableCell>
                  {visibleTagEntries.map(([tagName, tagValues]) => {
                    const val = tagValues.stored_value != null ? String(tagValues.stored_value) : '\u2014';
                    const style = getCellStyle(tagValues, 'db');
                    return (
                      <TableCell key={tagName} sx={{ fontSize: '0.85rem', ...style }}>
                        {val}
                      </TableCell>
                    );
                  })}
                </TableRow>

                {/* Comparison row (only when active) */}
                {hasComparison && (
                  <TableRow>
                    <TableCell sx={{ fontWeight: 'bold', color: 'text.secondary', position: 'sticky', left: 0, bgcolor: 'background.paper', zIndex: 1 }}>
                      {snapshotComparisonActive ? 'Snapshot' : 'Compare'}
                    </TableCell>
                    {visibleTagEntries.map(([tagName, tagValues]) => {
                      const compVal = (tagValues as TagSourceValues & { comparison_value?: string | number | boolean | null }).comparison_value;
                      const val = compVal != null ? String(compVal) : '\u2014';
                      const style = getCellStyle(tagValues, 'comparison');
                      return (
                        <TableCell key={tagName} sx={{ fontSize: '0.85rem', ...style }}>
                          {val}
                        </TableCell>
                      );
                    })}
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </Box>
        )}

        {visibleTagEntries.length === 0 && !loading && tagEntries.length === 0 && (
          <Typography variant="body2" color="text.secondary" sx={{ py: 2 }}>
            No tag data available.
          </Typography>
        )}
      </Collapse>
    </Box>
  );
};
