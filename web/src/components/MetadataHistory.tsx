// file: web/src/components/MetadataHistory.tsx
// version: 1.0.0
// guid: 8e3a7b2c-5d1f-4a9e-b6c0-2f8d4e7a1b3c

import { useCallback, useEffect, useState } from 'react';
import {
  Alert,
  Box,
  Button,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  IconButton,
  Snackbar,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Tooltip,
  Typography,
} from '@mui/material';
import UndoIcon from '@mui/icons-material/Undo.js';
import ArrowRightAltIcon from '@mui/icons-material/ArrowRightAlt.js';
import type { MetadataChangeRecord } from '../services/api';
import * as api from '../services/api';

const FIELD_LABELS: Record<string, string> = {
  title: 'Title',
  publisher: 'Publisher',
  language: 'Language',
  audiobook_release_year: 'Release Year',
  cover_url: 'Cover URL',
  author_name: 'Author',
  isbn10: 'ISBN-10',
  isbn13: 'ISBN-13',
  narrator: 'Narrator',
  series_name: 'Series',
  series_position: 'Series Position',
  description: 'Description',
  print_year: 'Print Year',
  isbn: 'ISBN',
  edition: 'Edition',
  work_id: 'Work ID',
};

const CHANGE_TYPE_COLORS: Record<
  string,
  'info' | 'warning' | 'default' | 'success' | 'secondary'
> = {
  fetched: 'info',
  override: 'warning',
  clear: 'default',
  undo: 'success',
  bulk_update: 'secondary',
};

function parseJsonValue(raw?: string): string {
  if (raw === undefined || raw === null || raw === '') return '(empty)';
  try {
    const parsed = JSON.parse(raw);
    if (parsed === null || parsed === undefined) return '(empty)';
    return String(parsed);
  } catch {
    return raw;
  }
}

function fieldLabel(field: string): string {
  return FIELD_LABELS[field] || field.replace(/_/g, ' ');
}

interface MetadataHistoryProps {
  bookId: string;
  open: boolean;
  onClose: () => void;
  onUndoComplete?: () => void;
}

export const MetadataHistory = ({
  bookId,
  open,
  onClose,
  onUndoComplete,
}: MetadataHistoryProps) => {
  const [history, setHistory] = useState<MetadataChangeRecord[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [undoingField, setUndoingField] = useState<string | null>(null);
  const [snackbar, setSnackbar] = useState<string | null>(null);

  const loadHistory = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await api.getBookMetadataHistory(bookId);
      setHistory(data);
    } catch (err) {
      console.error('Failed to load metadata history', err);
      setError('Failed to load metadata history.');
    } finally {
      setLoading(false);
    }
  }, [bookId]);

  useEffect(() => {
    if (open) {
      loadHistory();
    }
  }, [open, loadHistory]);

  // Determine the most recent change per field for undo eligibility
  const latestByField = new Set<string>();
  const latestIds = new Set<number>();
  for (const record of history) {
    if (!latestByField.has(record.field)) {
      latestByField.add(record.field);
      latestIds.add(record.id);
    }
  }

  const handleUndo = async (field: string) => {
    setUndoingField(field);
    try {
      const result = await api.undoMetadataChange(bookId, field);
      setSnackbar(result.message || `Undid change to ${fieldLabel(field)}.`);
      await loadHistory();
      onUndoComplete?.();
    } catch (err) {
      console.error('Failed to undo change', err);
      setSnackbar('Failed to undo change.');
    } finally {
      setUndoingField(null);
    }
  };

  return (
    <>
      <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
        <DialogTitle>Metadata Change History</DialogTitle>
        <DialogContent dividers>
          {loading && (
            <Box display="flex" justifyContent="center" py={3}>
              <CircularProgress />
            </Box>
          )}
          {error && (
            <Alert severity="error" sx={{ mb: 2 }}>
              {error}
            </Alert>
          )}
          {!loading && !error && history.length === 0 && (
            <Alert severity="info">No metadata changes recorded yet.</Alert>
          )}
          {!loading && history.length > 0 && (
            <Table size="small">
              <TableHead>
                <TableRow>
                  <TableCell>Field</TableCell>
                  <TableCell>Change</TableCell>
                  <TableCell>Type</TableCell>
                  <TableCell>Source</TableCell>
                  <TableCell>When</TableCell>
                  <TableCell align="right">Undo</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {history.map((record) => (
                  <TableRow key={record.id}>
                    <TableCell>
                      <Typography variant="body2" fontWeight={500}>
                        {fieldLabel(record.field)}
                      </Typography>
                    </TableCell>
                    <TableCell>
                      <Stack
                        direction="row"
                        spacing={0.5}
                        alignItems="center"
                        flexWrap="wrap"
                      >
                        <Typography
                          variant="body2"
                          color="text.secondary"
                          sx={{
                            maxWidth: 120,
                            overflow: 'hidden',
                            textOverflow: 'ellipsis',
                            whiteSpace: 'nowrap',
                          }}
                        >
                          {parseJsonValue(record.previous_value)}
                        </Typography>
                        <ArrowRightAltIcon
                          fontSize="small"
                          color="action"
                        />
                        <Typography
                          variant="body2"
                          sx={{
                            maxWidth: 120,
                            overflow: 'hidden',
                            textOverflow: 'ellipsis',
                            whiteSpace: 'nowrap',
                          }}
                        >
                          {parseJsonValue(record.new_value)}
                        </Typography>
                      </Stack>
                    </TableCell>
                    <TableCell>
                      <Chip
                        label={record.change_type}
                        size="small"
                        color={
                          CHANGE_TYPE_COLORS[record.change_type] || 'default'
                        }
                      />
                    </TableCell>
                    <TableCell>
                      <Typography variant="body2" color="text.secondary">
                        {record.source || '—'}
                      </Typography>
                    </TableCell>
                    <TableCell>
                      <Typography variant="body2" color="text.secondary">
                        {new Date(record.changed_at).toLocaleString()}
                      </Typography>
                    </TableCell>
                    <TableCell align="right">
                      {latestIds.has(record.id) &&
                      record.change_type !== 'undo' ? (
                        <Tooltip title={`Undo this ${fieldLabel(record.field)} change`}>
                          <span>
                            <IconButton
                              size="small"
                              onClick={() => handleUndo(record.field)}
                              disabled={undoingField === record.field}
                            >
                              {undoingField === record.field ? (
                                <CircularProgress size={18} />
                              ) : (
                                <UndoIcon fontSize="small" />
                              )}
                            </IconButton>
                          </span>
                        </Tooltip>
                      ) : null}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={onClose}>Close</Button>
        </DialogActions>
      </Dialog>
      <Snackbar
        open={!!snackbar}
        autoHideDuration={4000}
        onClose={() => setSnackbar(null)}
        message={snackbar}
      />
    </>
  );
};
