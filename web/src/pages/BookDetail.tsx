// file: web/src/pages/BookDetail.tsx
// version: 1.0.0
// guid: 4d2f7c6a-1b3e-4c5d-8f7a-9b0c1d2e3f4a

import { useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import {
  Box,
  Typography,
  Paper,
  Stack,
  Chip,
  Divider,
  Button,
  CircularProgress,
  Alert,
  Snackbar,
} from '@mui/material';
import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import DeleteIcon from '@mui/icons-material/Delete';
import RestoreIcon from '@mui/icons-material/Restore';
import ShieldIcon from '@mui/icons-material/Shield';
import type { Book } from '../services/api';
import * as api from '../services/api';

export const BookDetail = () => {
  const { id } = useParams();
  const navigate = useNavigate();
  const [book, setBook] = useState<Book | null>(null);
  const [loading, setLoading] = useState(true);
  const [actionLoading, setActionLoading] = useState(false);
  const [alert, setAlert] = useState<{ severity: 'success' | 'error'; message: string } | null>(
    null
  );

  const loadBook = async () => {
    if (!id) return;
    setLoading(true);
    try {
      const data = await api.getBook(id);
      setBook(data);
    } catch (error) {
      console.error('Failed to load book', error);
      setAlert({ severity: 'error', message: 'Failed to load audiobook details.' });
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadBook();
  }, [id]);

  const handleSoftDelete = async (blockHash: boolean) => {
    if (!book) return;
    setActionLoading(true);
    try {
      await api.deleteBook(book.id, { softDelete: true, blockHash });
      setAlert({ severity: 'success', message: 'Audiobook soft deleted.' });
      await loadBook();
    } catch (error) {
      console.error('Failed to soft delete audiobook', error);
      setAlert({ severity: 'error', message: 'Failed to delete audiobook.' });
    } finally {
      setActionLoading(false);
    }
  };

  const handleRestore = async () => {
    if (!book) return;
    setActionLoading(true);
    try {
      await api.restoreSoftDeletedBook(book.id);
      setAlert({ severity: 'success', message: 'Audiobook restored.' });
      await loadBook();
    } catch (error) {
      console.error('Failed to restore audiobook', error);
      setAlert({ severity: 'error', message: 'Failed to restore audiobook.' });
    } finally {
      setActionLoading(false);
    }
  };

  const handlePurge = async () => {
    if (!book) return;
    setActionLoading(true);
    try {
      await api.deleteBook(book.id, { softDelete: false, blockHash: false });
      setAlert({ severity: 'success', message: 'Audiobook permanently deleted.' });
      navigate('/library');
    } catch (error) {
      console.error('Failed to purge audiobook', error);
      setAlert({ severity: 'error', message: 'Failed to purge audiobook.' });
    } finally {
      setActionLoading(false);
    }
  };

  if (loading) {
    return (
      <Box display="flex" alignItems="center" justifyContent="center" height="100%">
        <CircularProgress />
      </Box>
    );
  }

  if (!book) {
    return (
      <Box p={3}>
        <Alert severity="error">Audiobook not found.</Alert>
        <Button startIcon={<ArrowBackIcon />} sx={{ mt: 2 }} onClick={() => navigate('/library')}>
          Back to Library
        </Button>
      </Box>
    );
  }

  const isSoftDeleted = book.marked_for_deletion;

  return (
    <Box p={3} sx={{ height: '100%', overflowY: 'auto' }}>
      <Snackbar
        open={!!alert}
        autoHideDuration={4000}
        onClose={() => setAlert(null)}
        anchorOrigin={{ vertical: 'top', horizontal: 'center' }}
      >
        {alert ? (
          <Alert severity={alert.severity} onClose={() => setAlert(null)} sx={{ width: '100%' }}>
            {alert.message}
          </Alert>
        ) : undefined}
      </Snackbar>

      <Stack direction="row" alignItems="center" spacing={2} mb={3}>
        <Button startIcon={<ArrowBackIcon />} variant="text" onClick={() => navigate('/library')}>
          Back to Library
        </Button>
        <Typography variant="h4" component="h1">
          {book.title || 'Untitled'}
        </Typography>
        {isSoftDeleted && <Chip label="Soft Deleted" color="warning" />}
        {book.library_state && <Chip label={`State: ${book.library_state}`} />}
      </Stack>

      <Paper sx={{ p: 3 }}>
        <Stack spacing={2}>
          <Stack direction="row" spacing={2} alignItems="center" flexWrap="wrap">
            <Typography variant="h6" component="div">
              Details
            </Typography>
            {book.author_id && <Chip label={`Author ID: ${book.author_id}`} size="small" />}
            {book.series_id && <Chip label={`Series ID: ${book.series_id}`} size="small" />}
            {book.format && <Chip label={book.format.toUpperCase()} size="small" />}
            {book.quality && <Chip label={book.quality} size="small" />}
          </Stack>

          <Stack direction={{ xs: 'column', sm: 'row' }} spacing={3}>
            <Box flex={1}>
              <Typography variant="subtitle2" gutterBottom>
                File
              </Typography>
              <Typography variant="body2" color="text.secondary">
                {book.file_path}
              </Typography>
            </Box>
            <Box flex={1}>
              <Typography variant="subtitle2" gutterBottom>
                Hashes
              </Typography>
              <Typography variant="body2" color="text.secondary">
                File Hash: {book.file_hash || '—'}
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Original Hash: {book.original_file_hash || '—'}
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Organized Hash: {book.organized_file_hash || '—'}
              </Typography>
            </Box>
          </Stack>

          <Divider />

          <Stack direction="row" spacing={2} flexWrap="wrap">
            {!isSoftDeleted && (
              <Button
                variant="outlined"
                color="warning"
                startIcon={<DeleteIcon />}
                onClick={() => handleSoftDelete(true)}
                disabled={actionLoading}
              >
                Soft Delete &amp; Block Hash
              </Button>
            )}
            {!isSoftDeleted && (
              <Button
                variant="outlined"
                startIcon={<ShieldIcon />}
                onClick={() => handleSoftDelete(false)}
                disabled={actionLoading}
              >
                Soft Delete (keep hash unblocked)
              </Button>
            )}
            {isSoftDeleted && (
              <Button
                variant="contained"
                color="success"
                startIcon={<RestoreIcon />}
                onClick={handleRestore}
                disabled={actionLoading}
              >
                Restore
              </Button>
            )}
            {isSoftDeleted && (
              <Button
                variant="outlined"
                color="error"
                startIcon={<DeleteIcon />}
                onClick={handlePurge}
                disabled={actionLoading}
              >
                Purge Permanently
              </Button>
            )}
          </Stack>
        </Stack>
      </Paper>
    </Box>
  );
};
