// file: web/src/components/audiobooks/MetadataSearchDialog.tsx
// version: 1.0.0
// guid: 8a9b0c1d-2e3f-4a5b-6c7d-8e9f0a1b2c3d

import { useCallback, useEffect, useState } from 'react';
import {
  Avatar,
  Box,
  Button,
  Checkbox,
  Chip,
  CircularProgress,
  Collapse,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  IconButton,
  InputAdornment,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import SearchIcon from '@mui/icons-material/Search.js';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore.js';
import ExpandLessIcon from '@mui/icons-material/ExpandLess.js';
import type { Book, MetadataCandidate } from '../../services/api';
import * as api from '../../services/api';

interface MetadataSearchDialogProps {
  open: boolean;
  book: Book;
  onClose: () => void;
  onApplied: (updatedBook: Book) => void;
  toast: (message: string, severity?: 'success' | 'error' | 'warning' | 'info') => void;
}

const FIELD_OPTIONS = [
  'title',
  'author',
  'narrator',
  'series',
  'series_position',
  'year',
  'publisher',
  'isbn',
  'cover_url',
  'description',
  'language',
] as const;

const SOURCE_COLORS: Record<string, 'primary' | 'secondary' | 'success' | 'warning' | 'info'> = {
  openlibrary: 'primary',
  google_books: 'secondary',
  audible: 'success',
  goodreads: 'warning',
  manual: 'info',
};

export function MetadataSearchDialog({
  open,
  book,
  onClose,
  onApplied,
  toast,
}: MetadataSearchDialogProps) {
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<MetadataCandidate[]>([]);
  const [loading, setLoading] = useState(false);
  const [expandedCard, setExpandedCard] = useState<number | null>(null);
  const [selectedFields, setSelectedFields] = useState<Set<string>>(new Set());
  const [applying, setApplying] = useState(false);

  // Auto-populate query and search on open
  useEffect(() => {
    if (open && book) {
      const q = [book.title, book.author_name].filter(Boolean).join(' ');
      setQuery(q);
      setResults([]);
      setExpandedCard(null);
      setSelectedFields(new Set());
      doSearch(q);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, book?.id]);

  const doSearch = useCallback(
    async (searchQuery: string) => {
      if (!book?.id) return;
      setLoading(true);
      try {
        const resp = await api.searchMetadataForBook(book.id, searchQuery);
        setResults(resp.results || []);
      } catch (err) {
        toast(
          err instanceof Error ? err.message : 'Search failed',
          'error'
        );
        setResults([]);
      } finally {
        setLoading(false);
      }
    },
    [book?.id, toast]
  );

  const handleSearch = () => {
    doSearch(query);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      handleSearch();
    }
  };

  const handleApplyAll = async (candidate: MetadataCandidate) => {
    setApplying(true);
    try {
      const resp = await api.applyMetadataCandidate(book.id, candidate);
      toast(`Metadata applied from ${resp.source}`, 'success');
      onApplied(resp.book);
      onClose();
    } catch (err) {
      toast(
        err instanceof Error ? err.message : 'Failed to apply metadata',
        'error'
      );
    } finally {
      setApplying(false);
    }
  };

  const handleApplySelected = async (candidate: MetadataCandidate) => {
    if (selectedFields.size === 0) {
      toast('Select at least one field to apply', 'warning');
      return;
    }
    setApplying(true);
    try {
      const resp = await api.applyMetadataCandidate(
        book.id,
        candidate,
        Array.from(selectedFields)
      );
      toast(`Selected fields applied from ${resp.source}`, 'success');
      onApplied(resp.book);
      onClose();
    } catch (err) {
      toast(
        err instanceof Error ? err.message : 'Failed to apply metadata',
        'error'
      );
    } finally {
      setApplying(false);
    }
  };

  const handleMarkNoMatch = async () => {
    try {
      await api.markNoMatch(book.id);
      toast('Marked as no match found', 'info');
      onClose();
    } catch (err) {
      toast(
        err instanceof Error ? err.message : 'Failed to mark as no match',
        'error'
      );
    }
  };

  const toggleField = (field: string) => {
    setSelectedFields((prev) => {
      const next = new Set(prev);
      if (next.has(field)) {
        next.delete(field);
      } else {
        next.add(field);
      }
      return next;
    });
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle>Search Metadata</DialogTitle>
      <DialogContent>
        <TextField
          fullWidth
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="Search by title, author, ISBN..."
          sx={{ mt: 1, mb: 2 }}
          InputProps={{
            startAdornment: (
              <InputAdornment position="start">
                <SearchIcon />
              </InputAdornment>
            ),
            endAdornment: (
              <InputAdornment position="end">
                <IconButton onClick={handleSearch} disabled={loading}>
                  <SearchIcon />
                </IconButton>
              </InputAdornment>
            ),
          }}
        />

        {loading && (
          <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
            <CircularProgress />
          </Box>
        )}

        {!loading && results.length === 0 && (
          <Typography color="text.secondary" sx={{ py: 4, textAlign: 'center' }}>
            No results found. Try a different search query.
          </Typography>
        )}

        <Stack spacing={2}>
          {results.map((candidate, idx) => (
            <Box
              key={idx}
              sx={{
                border: 1,
                borderColor: 'divider',
                borderRadius: 1,
                p: 2,
              }}
            >
              <Stack direction="row" spacing={2} alignItems="flex-start">
                <Avatar
                  src={candidate.cover_url}
                  variant="rounded"
                  sx={{ width: 60, height: 80 }}
                >
                  {candidate.title?.[0] || '?'}
                </Avatar>
                <Box sx={{ flex: 1, minWidth: 0 }}>
                  <Typography variant="subtitle1" fontWeight="bold" noWrap>
                    {candidate.title}
                  </Typography>
                  <Typography variant="body2" color="text.secondary">
                    {candidate.author}
                    {candidate.year ? ` (${candidate.year})` : ''}
                  </Typography>
                  {candidate.series && (
                    <Typography variant="body2" color="text.secondary">
                      Series: {candidate.series}
                      {candidate.series_position
                        ? ` #${candidate.series_position}`
                        : ''}
                    </Typography>
                  )}
                  {candidate.narrator && (
                    <Typography variant="body2" color="text.secondary">
                      Narrator: {candidate.narrator}
                    </Typography>
                  )}
                  <Stack direction="row" spacing={1} sx={{ mt: 1 }}>
                    <Chip
                      label={candidate.source}
                      size="small"
                      color={SOURCE_COLORS[candidate.source] || 'default'}
                    />
                    <Chip
                      label={`${Math.round(candidate.score * 100)}%`}
                      size="small"
                      variant="outlined"
                    />
                  </Stack>
                </Box>
                <Button
                  variant="contained"
                  size="small"
                  onClick={() => handleApplyAll(candidate)}
                  disabled={applying}
                >
                  Apply
                </Button>
              </Stack>

              {/* Expandable field selector */}
              <Box sx={{ mt: 1 }}>
                <Button
                  size="small"
                  onClick={() =>
                    setExpandedCard(expandedCard === idx ? null : idx)
                  }
                  endIcon={
                    expandedCard === idx ? (
                      <ExpandLessIcon />
                    ) : (
                      <ExpandMoreIcon />
                    )
                  }
                >
                  Select fields...
                </Button>
                <Collapse in={expandedCard === idx}>
                  <Box sx={{ mt: 1, pl: 1 }}>
                    {FIELD_OPTIONS.map((field) => {
                      const value =
                        candidate[field as keyof MetadataCandidate];
                      if (value === undefined || value === null || value === '')
                        return null;
                      return (
                        <FormControlLabel
                          key={field}
                          control={
                            <Checkbox
                              checked={selectedFields.has(field)}
                              onChange={() => toggleField(field)}
                              size="small"
                            />
                          }
                          label={`${field}: ${value}`}
                        />
                      );
                    })}
                    <Box sx={{ mt: 1 }}>
                      <Button
                        variant="outlined"
                        size="small"
                        onClick={() => handleApplySelected(candidate)}
                        disabled={applying || selectedFields.size === 0}
                      >
                        Apply Selected
                      </Button>
                    </Box>
                  </Box>
                </Collapse>
              </Box>
            </Box>
          ))}
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button
          color="warning"
          onClick={handleMarkNoMatch}
          disabled={applying}
        >
          No Match Found
        </Button>
        <Button onClick={onClose}>Cancel</Button>
      </DialogActions>
    </Dialog>
  );
}
