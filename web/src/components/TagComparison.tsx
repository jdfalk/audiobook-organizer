// file: web/src/components/TagComparison.tsx
// version: 1.3.0
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
import VisibilityOffIcon from '@mui/icons-material/VisibilityOff.js';
import RestoreIcon from '@mui/icons-material/Restore.js';
import type { Book, BookTags, TagSourceValues } from '../services/api';
import * as api from '../services/api';

interface TagComparisonProps {
  bookId: string;
  versions: Book[];
  refreshKey?: number; // increment to force reload after mutations
  snapshotTimestamp?: string | null; // when set, auto-expand and show snapshot comparison banner
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
  audiobook_release_year: 'year',
  description: 'description',
};

export const TagComparison = ({ bookId, versions, refreshKey, snapshotTimestamp }: TagComparisonProps) => {
  const [tags, setTags] = useState<BookTags | null>(null);
  const [loading, setLoading] = useState(false);
  const [compareId, setCompareId] = useState<string>('');
  const [expanded, setExpanded] = useState(false);
  const [hiddenTags, setHiddenTags] = useState<Set<string>>(new Set());
  const [colWidths, setColWidths] = useState<number[]>([180, 0, 0, 0]); // tag, file, db, comparison; 0 = flex
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

  // Auto-expand and reload when a snapshot timestamp is selected from ChangeLog
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

  /** Check which key tags are present (have a non-empty file_value) */
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

  const getRowStyle = (entry: TagSourceValues, _tagName: string) => {
    const fileVal = entry.file_value != null ? String(entry.file_value) : '';
    const storedVal = entry.stored_value != null ? String(entry.stored_value) : '';

    if (hasComparison) {
      const compVal = (entry as TagSourceValues & { comparison_value?: string | number | boolean | null }).comparison_value;
      const compStr = compVal != null ? String(compVal) : '';

      if (compStr && fileVal && compStr !== fileVal) {
        return { bgcolor: '#1a1500' };
      }
      if (fileVal && !compStr) {
        return { bgcolor: '#001a00' };
      }
    }

    if (fileVal && storedVal && fileVal !== storedVal) {
      return { bgcolor: 'warning.900' };
    }
    return {};
  };

  const getComparisonTextColor = (entry: TagSourceValues) => {
    const fileVal = entry.file_value != null ? String(entry.file_value) : '';
    const compVal = (entry as TagSourceValues & { comparison_value?: string | number | boolean | null }).comparison_value;
    const compStr = compVal != null ? String(compVal) : '';
    if (compStr && fileVal && compStr !== fileVal) {
      return '#ef5350';
    }
    return undefined;
  };

  const getStoredTextColor = (entry: TagSourceValues) => {
    const fileVal = entry.file_value != null ? String(entry.file_value) : '';
    const storedVal = entry.stored_value != null ? String(entry.stored_value) : '';
    if (storedVal && !fileVal) {
      return '#4caf50';
    }
    return undefined;
  };

  // Column resize handlers
  const handleResizeStart = (colIndex: number, e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    resizingCol.current = colIndex;
    resizeStartX.current = e.clientX;

    // Measure actual column width from DOM
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
      setColWidths((prev) => {
        const next = [...prev];
        next[resizingCol.current!] = newWidth;
        return next;
      });
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

  const colStyle = (idx: number): Record<string, unknown> => {
    if (colWidths[idx]) return { width: colWidths[idx], minWidth: 60, position: 'relative' };
    return { position: 'relative' };
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
        {/* Snapshot comparison banner */}
        {snapshotComparisonActive && (
          <Alert severity="info" sx={{ mb: 2 }} data-testid="snapshot-comparison-banner">
            Comparing against snapshot from {new Date(snapshotTimestamp ?? '').toLocaleString()}
          </Alert>
        )}

        {/* Comparison dropdown */}
        {otherVersions.length > 0 && (
          <FormControl size="small" sx={{ mb: 2, minWidth: 280 }}>
            <InputLabel>Compare against</InputLabel>
            <Select
              value={compareId}
              label="Compare against"
              onChange={(e) => setCompareId(e.target.value)}
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

        {visibleTagEntries.length > 0 && (
          <Table
            size="small"
            ref={tableRef}
            sx={{ tableLayout: 'fixed', '& td, & th': { py: 0.5, overflow: 'hidden', textOverflow: 'ellipsis' } }}
          >
            <TableHead>
              <TableRow>
                <TableCell sx={{ fontWeight: 'bold', ...colStyle(0) }}>
                  Tag
                  {resizeHandle(0)}
                </TableCell>
                <TableCell sx={{ fontWeight: 'bold', ...colStyle(1) }}>
                  File Value
                  {resizeHandle(1)}
                </TableCell>
                <TableCell sx={{ fontWeight: 'bold', ...colStyle(2) }}>
                  DB Value
                  {resizeHandle(2)}
                </TableCell>
                {hasComparison && (
                  <TableCell sx={{ fontWeight: 'bold', ...colStyle(3) }}>
                    Comparison Value
                    {resizeHandle(3)}
                  </TableCell>
                )}
                <TableCell sx={{ width: 36, p: 0 }} />
              </TableRow>
            </TableHead>
            <TableBody>
              {visibleTagEntries.map(([tagName, tagValues]) => {
                const fileVal = tagValues.file_value != null ? String(tagValues.file_value) : '\u2014';
                const storedVal = tagValues.stored_value != null ? String(tagValues.stored_value) : '\u2014';
                const compVal = (tagValues as TagSourceValues & { comparison_value?: string | number | boolean | null }).comparison_value;
                const compStr = compVal != null ? String(compVal) : '\u2014';
                const storedColor = getStoredTextColor(tagValues);
                const compColor = getComparisonTextColor(tagValues);

                return (
                  <TableRow key={tagName} sx={getRowStyle(tagValues, tagName)}>
                    <TableCell sx={{ fontWeight: 'bold', color: 'text.secondary', fontSize: '0.85rem' }}>
                      {TAG_LABELS[tagName] || tagName}
                    </TableCell>
                    <TableCell sx={{ fontSize: '0.85rem', wordBreak: 'break-all' }}>
                      {fileVal}
                    </TableCell>
                    <TableCell sx={{ fontSize: '0.85rem', wordBreak: 'break-all', ...(storedColor && { color: storedColor }) }}>
                      {storedVal}
                    </TableCell>
                    {hasComparison && (
                      <TableCell sx={{ fontSize: '0.85rem', wordBreak: 'break-all', ...(compColor && { color: compColor }) }}>
                        {compStr}
                      </TableCell>
                    )}
                    <TableCell sx={{ p: 0, textAlign: 'center' }}>
                      <Tooltip title={`Hide "${TAG_LABELS[tagName] || tagName}"`}>
                        <IconButton
                          size="small"
                          onClick={() => setHiddenTags((prev) => new Set(prev).add(tagName))}
                          sx={{ opacity: 0.3, '&:hover': { opacity: 1 } }}
                        >
                          <VisibilityOffIcon sx={{ fontSize: 14 }} />
                        </IconButton>
                      </Tooltip>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
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
