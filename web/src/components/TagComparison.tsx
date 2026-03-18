// file: web/src/components/TagComparison.tsx
// version: 1.0.0
// guid: cfed2692-76f6-47b0-bc84-cc2a4075e554

import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Box,
  Chip,
  Collapse,
  FormControl,
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
  Typography,
} from '@mui/material';
import type { Book, BookTags, TagSourceValues } from '../services/api';
import * as api from '../services/api';

interface TagComparisonProps {
  bookId: string;
  versions: Book[];
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

export const TagComparison = ({ bookId, versions }: TagComparisonProps) => {
  const [tags, setTags] = useState<BookTags | null>(null);
  const [loading, setLoading] = useState(false);
  const [compareId, setCompareId] = useState<string>('');
  const [expanded, setExpanded] = useState(false);

  const loadTags = useCallback(async () => {
    setLoading(true);
    try {
      const result = await api.getBookTags(bookId, compareId || undefined);
      setTags(result);
    } catch {
      setTags(null);
    } finally {
      setLoading(false);
    }
  }, [bookId, compareId]);

  useEffect(() => {
    loadTags();
  }, [loadTags]);

  const tagEntries = useMemo(() => {
    if (!tags?.tags) return [];
    return Object.entries(tags.tags);
  }, [tags]);

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

  const hasComparison = compareId !== '';

  const getRowStyle = (entry: TagSourceValues, _tagName: string) => {
    const fileVal = entry.file_value != null ? String(entry.file_value) : '';
    const storedVal = entry.stored_value != null ? String(entry.stored_value) : '';

    if (hasComparison) {
      // comparison_value comes from the API when compare_id is set
      const compVal = (entry as TagSourceValues & { comparison_value?: string | number | boolean | null }).comparison_value;
      const compStr = compVal != null ? String(compVal) : '';

      if (compStr && fileVal && compStr !== fileVal) {
        // Differs from file value - amber background
        return { bgcolor: '#1a1500' };
      }
      if (fileVal && !compStr) {
        // Present here but missing in comparison - green background
        return { bgcolor: '#001a00' };
      }
    }

    // No comparison mode: highlight file vs stored diffs
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
      return '#ef5350'; // Red for differing comparison values
    }
    return undefined;
  };

  const getStoredTextColor = (entry: TagSourceValues) => {
    const fileVal = entry.file_value != null ? String(entry.file_value) : '';
    const storedVal = entry.stored_value != null ? String(entry.stored_value) : '';
    if (storedVal && !fileVal) {
      return '#4caf50'; // Green for DB values not in file
    }
    return undefined;
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

        {tagEntries.length > 0 && (
          <Table size="small" sx={{ '& td, & th': { py: 0.5 } }}>
            <TableHead>
              <TableRow>
                <TableCell sx={{ fontWeight: 'bold', width: 180 }}>Tag</TableCell>
                <TableCell sx={{ fontWeight: 'bold' }}>File Value</TableCell>
                <TableCell sx={{ fontWeight: 'bold' }}>DB Value</TableCell>
                {hasComparison && (
                  <TableCell sx={{ fontWeight: 'bold' }}>Comparison Value</TableCell>
                )}
              </TableRow>
            </TableHead>
            <TableBody>
              {tagEntries.map(([tagName, tagValues]) => {
                const fileVal = tagValues.file_value != null ? String(tagValues.file_value) : '\u2014';
                const storedVal = tagValues.stored_value != null ? String(tagValues.stored_value) : '\u2014';
                const compVal = (tagValues as TagSourceValues & { comparison_value?: string | number | boolean | null }).comparison_value;
                const compStr = compVal != null ? String(compVal) : '\u2014';
                const storedColor = getStoredTextColor(tagValues);
                const compColor = getComparisonTextColor(tagValues);

                return (
                  <TableRow key={tagName} sx={getRowStyle(tagValues, tagName)}>
                    <TableCell sx={{ fontWeight: 'bold', color: 'text.secondary', fontSize: '0.8rem' }}>
                      {TAG_LABELS[tagName] || tagName}
                    </TableCell>
                    <TableCell sx={{ fontSize: '0.8rem', wordBreak: 'break-all' }}>
                      {fileVal}
                    </TableCell>
                    <TableCell sx={{ fontSize: '0.8rem', wordBreak: 'break-all', ...(storedColor && { color: storedColor }) }}>
                      {storedVal}
                    </TableCell>
                    {hasComparison && (
                      <TableCell sx={{ fontSize: '0.8rem', wordBreak: 'break-all', ...(compColor && { color: compColor }) }}>
                        {compStr}
                      </TableCell>
                    )}
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        )}

        {tagEntries.length === 0 && !loading && (
          <Typography variant="body2" color="text.secondary" sx={{ py: 2 }}>
            No tag data available.
          </Typography>
        )}
      </Collapse>
    </Box>
  );
};
