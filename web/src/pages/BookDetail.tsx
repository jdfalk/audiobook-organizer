// file: web/src/pages/BookDetail.tsx
// version: 1.1.0
// guid: 4d2f7c6a-1b3e-4c5d-8f7a-9b0c1d2e3f4a

import { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import {
  Avatar,
  Box,
  Paper,
  Stack,
  Chip,
  Divider,
  Button,
  CircularProgress,
  Alert,
  Snackbar,
  Typography,
  Grid,
  Tabs,
  Tab,
  Tooltip,
  FormControlLabel,
  Checkbox,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  IconButton,
  LinearProgress,
} from '@mui/material';
import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import DeleteIcon from '@mui/icons-material/Delete';
import RestoreIcon from '@mui/icons-material/Restore';
import PsychologyIcon from '@mui/icons-material/Psychology';
import CloudDownloadIcon from '@mui/icons-material/CloudDownload';
import CompareIcon from '@mui/icons-material/Compare';
import InfoIcon from '@mui/icons-material/Info';
import FileCopyIcon from '@mui/icons-material/FileCopy';
import AccessTimeIcon from '@mui/icons-material/AccessTime';
import StorageIcon from '@mui/icons-material/Storage';
import type { Book } from '../services/api';
import * as api from '../services/api';
import { VersionManagement } from '../components/audiobooks/VersionManagement';

export const BookDetail = () => {
  const { id } = useParams();
  const navigate = useNavigate();
  const [book, setBook] = useState<Book | null>(null);
  const [loading, setLoading] = useState(true);
  const [actionLoading, setActionLoading] = useState(false);
  const [actionLabel, setActionLabel] = useState<string | null>(null);
  const [alert, setAlert] = useState<{ severity: 'success' | 'error'; message: string } | null>(
    null
  );
  const [activeTab, setActiveTab] = useState<'info' | 'files' | 'versions'>('info');
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deleteOptions, setDeleteOptions] = useState({ softDelete: true, blockHash: true });
  const [purgeDialogOpen, setPurgeDialogOpen] = useState(false);
  const [versions, setVersions] = useState<Book[]>([]);
  const [versionsLoading, setVersionsLoading] = useState(false);
  const [versionsError, setVersionsError] = useState<string | null>(null);
  const [versionDialogOpen, setVersionDialogOpen] = useState(false);

  const loadBook = useCallback(async () => {
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
  }, [id]);

  const loadVersions = useCallback(async () => {
    if (!id) return;
    setVersionsLoading(true);
    setVersionsError(null);
    try {
      const data = await api.getBookVersions(id);
      setVersions(data);
    } catch (error) {
      console.error('Failed to load versions', error);
      setVersionsError('Unable to load linked versions right now.');
    } finally {
      setVersionsLoading(false);
    }
  }, [id]);

  useEffect(() => {
    loadBook();
    loadVersions();
  }, [id, loadBook, loadVersions]);

  const formatDateTime = (value?: string) => {
    if (!value) return '—';
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
  };

  const formatDuration = (seconds?: number) => {
    if (!seconds || seconds <= 0) return '—';
    const hours = Math.floor(seconds / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    const remainingSeconds = Math.floor(seconds % 60);
    const parts = [];
    if (hours > 0) parts.push(`${hours}h`);
    if (minutes > 0) parts.push(`${minutes}m`);
    if (remainingSeconds > 0 && hours === 0) parts.push(`${remainingSeconds}s`);
    return parts.join(' ');
  };

  const handleDelete = async () => {
    if (!book) return;
    setActionLoading(true);
    setActionLabel(deleteOptions.softDelete ? 'Soft deleting...' : 'Deleting...');
    try {
      await api.deleteBook(book.id, {
        softDelete: deleteOptions.softDelete,
        blockHash: deleteOptions.blockHash,
      });
      setAlert({
        severity: 'success',
        message: deleteOptions.softDelete
          ? 'Audiobook marked for deletion.'
          : 'Audiobook deleted permanently.',
      });
      setDeleteDialogOpen(false);
      await loadBook();
    } catch (error) {
      console.error('Failed to delete audiobook', error);
      setAlert({ severity: 'error', message: 'Failed to delete audiobook.' });
    } finally {
      setActionLabel(null);
      setActionLoading(false);
    }
  };

  const handleRestore = async () => {
    if (!book) return;
    setActionLoading(true);
    setActionLabel('Restoring...');
    try {
      await api.restoreSoftDeletedBook(book.id);
      setAlert({ severity: 'success', message: 'Audiobook restored.' });
      await loadBook();
    } catch (error) {
      console.error('Failed to restore audiobook', error);
      setAlert({ severity: 'error', message: 'Failed to restore audiobook.' });
    } finally {
      setActionLabel(null);
      setActionLoading(false);
    }
  };

  const handlePurge = async () => {
    if (!book) return;
    setActionLoading(true);
    setActionLabel('Purging...');
    try {
      await api.deleteBook(book.id, { softDelete: false, blockHash: false });
      setAlert({ severity: 'success', message: 'Audiobook permanently deleted.' });
      navigate('/library');
    } catch (error) {
      console.error('Failed to purge audiobook', error);
      setAlert({ severity: 'error', message: 'Failed to purge audiobook.' });
    } finally {
      setActionLabel(null);
      setActionLoading(false);
    }
  };

  const handleFetchMetadata = async () => {
    if (!book) return;
    setActionLoading(true);
    setActionLabel('Fetching metadata...');
    try {
      const result = await api.fetchBookMetadata(book.id);
      setBook(result.book);
      setAlert({
        severity: 'success',
        message: result.message || `Metadata refreshed from ${result.source || 'provider'}.`,
      });
    } catch (error) {
      console.error('Failed to fetch metadata', error);
      setAlert({ severity: 'error', message: 'Metadata fetch failed.' });
    } finally {
      setActionLabel(null);
      setActionLoading(false);
    }
  };

  const handleParseWithAI = async () => {
    if (!book) return;
    setActionLoading(true);
    setActionLabel('Asking AI to parse...');
    try {
      const result = await api.parseAudiobookWithAI(book.id);
      setBook(result.book);
      setAlert({
        severity: 'success',
        message: result.message || 'AI parsing completed.',
      });
    } catch (error) {
      console.error('Failed to parse with AI', error);
      setAlert({ severity: 'error', message: 'AI parsing failed.' });
    } finally {
      setActionLabel(null);
      setActionLoading(false);
    }
  };

  const versionSummary = useMemo(() => {
    if (!book) return null;
    const primary = versions.find((v) => v.is_primary_version);
    const linked = versions.filter((v) => v.id !== book.id);
    return { primary, linkedCount: linked.length };
  }, [book, versions]);

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
  const coverLetter = (book.title || 'A')[0]?.toUpperCase();

  return (
    <Box p={3} sx={{ height: '100%', overflowY: 'auto' }}>
      {actionLoading && (
        <LinearProgress
          sx={{ mb: 2 }}
          color="secondary"
          aria-label={actionLabel || 'Processing action'}
        />
      )}
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

      <Stack direction="row" alignItems="center" spacing={2} mb={3} flexWrap="wrap">
        <Button startIcon={<ArrowBackIcon />} variant="text" onClick={() => navigate('/library')}>
          Back to Library
        </Button>
        <Stack direction="row" spacing={2} alignItems="center">
          <Avatar
            sx={{
              bgcolor: 'primary.main',
              width: 56,
              height: 56,
              fontSize: 28,
            }}
          >
            {coverLetter}
          </Avatar>
          <Stack spacing={0.5}>
            <Box display="flex" alignItems="center" gap={1} flexWrap="wrap">
              <Typography variant="h4" component="h1">
                {book.title || 'Untitled'}
              </Typography>
              {isSoftDeleted && <Chip label="Soft Deleted" color="warning" size="small" />}
              {book.library_state && (
                <Chip label={`State: ${book.library_state}`} color="info" size="small" />
              )}
              {book.is_primary_version && <Chip label="Primary Version" color="primary" />}
            </Box>
            <Typography variant="subtitle1" color="text.secondary">
              {book.author_name || book.author_id ? `By ${book.author_name || book.author_id}` : ''}
            </Typography>
          </Stack>
        </Stack>
      </Stack>

      {isSoftDeleted && (
        <Alert
          severity="warning"
          action={
            <Stack direction="row" spacing={1}>
              <Button color="inherit" size="small" onClick={handleRestore} disabled={actionLoading}>
                Restore
              </Button>
              <Button
                color="inherit"
                size="small"
                onClick={() => setPurgeDialogOpen(true)}
                disabled={actionLoading}
              >
                Purge
              </Button>
            </Stack>
          }
          sx={{ mb: 2 }}
        >
          Marked for deletion on {formatDateTime(book.marked_for_deletion_at)}. Restore to keep the
          book or purge to remove it permanently.
        </Alert>
      )}

      <Paper sx={{ p: 3, mb: 3 }}>
        <Stack direction={{ xs: 'column', md: 'row' }} spacing={2} justifyContent="space-between">
          <Stack direction="row" spacing={1} flexWrap="wrap">
            <Chip
              icon={<AccessTimeIcon />}
              label={`Created ${formatDateTime(book.created_at)}`}
              variant="outlined"
            />
            <Chip
              icon={<InfoIcon />}
              label={`Updated ${formatDateTime(book.updated_at)}`}
              variant="outlined"
            />
            {book.version_group_id && (
              <Chip
                icon={<CompareIcon />}
                label="Version Group Linked"
                color="secondary"
                variant="outlined"
              />
            )}
          </Stack>
          <Stack direction="row" spacing={1} flexWrap="wrap" justifyContent="flex-end">
            <Button
              variant="outlined"
              startIcon={<CloudDownloadIcon />}
              onClick={handleFetchMetadata}
              disabled={actionLoading}
            >
              Fetch Metadata
            </Button>
            <Button
              variant="outlined"
              startIcon={<PsychologyIcon />}
              onClick={handleParseWithAI}
              disabled={actionLoading}
            >
              Parse with AI
            </Button>
            <Button
              variant="contained"
              startIcon={<CompareIcon />}
              color="secondary"
              onClick={() => setVersionDialogOpen(true)}
              disabled={versionsLoading}
            >
              Manage Versions
            </Button>
            {!isSoftDeleted ? (
              <Button
                variant="contained"
                color="warning"
                startIcon={<DeleteIcon />}
                onClick={() => {
                  setDeleteOptions({ softDelete: true, blockHash: true });
                  setDeleteDialogOpen(true);
                }}
                disabled={actionLoading}
              >
                Delete
              </Button>
            ) : (
              <Button
                variant="outlined"
                color="success"
                startIcon={<RestoreIcon />}
                onClick={handleRestore}
                disabled={actionLoading}
              >
                Restore
              </Button>
            )}
          </Stack>
        </Stack>
      </Paper>

      <Paper sx={{ p: 2, mb: 3 }}>
        <Tabs
          value={activeTab}
          onChange={(_, value) => setActiveTab(value)}
          textColor="primary"
          indicatorColor="primary"
          variant="scrollable"
        >
          <Tab label="Info" value="info" />
          <Tab label="Files" value="files" />
          <Tab
            label={`Versions${versionSummary?.linkedCount ? ` (${versionSummary.linkedCount})` : ''}`}
            value="versions"
          />
        </Tabs>
      </Paper>

      {activeTab === 'info' && (
        <Paper sx={{ p: 3, mb: 3 }}>
          <Grid container spacing={2}>
            {[
              { label: 'Title', value: book.title || 'Untitled' },
              { label: 'Author', value: book.author_name || book.author_id },
              { label: 'Series', value: book.series_name || book.series_id },
              { label: 'Narrator', value: book.narrator },
              { label: 'Publisher', value: book.publisher },
              { label: 'Language', value: book.language },
              { label: 'Edition', value: book.edition },
              { label: 'Release Year', value: book.audiobook_release_year || book.print_year },
              { label: 'Work ID', value: book.work_id },
              { label: 'Library State', value: book.library_state },
              { label: 'Quantity', value: book.quantity },
              { label: 'Quality', value: book.quality },
            ]
              .filter((item) => item.value !== undefined && item.value !== '')
              .map((item) => (
                <Grid item xs={12} sm={6} md={4} key={item.label}>
                  <Box
                    sx={{
                      p: 2,
                      borderRadius: 1,
                      bgcolor: 'background.default',
                      border: '1px solid',
                      borderColor: 'divider',
                      height: '100%',
                    }}
                  >
                    <Typography variant="caption" color="text.secondary" sx={{ textTransform: 'uppercase' }}>
                      {item.label}
                    </Typography>
                    <Typography variant="body1">{item.value as string}</Typography>
                  </Box>
                </Grid>
              ))}
          </Grid>
          {book.description && (
            <Box mt={3}>
              <Typography variant="h6" gutterBottom>
                Description
              </Typography>
              <Typography variant="body1" color="text.secondary">
                {book.description}
              </Typography>
            </Box>
          )}
        </Paper>
      )}

      {activeTab === 'files' && (
        <Paper sx={{ p: 3, mb: 3 }}>
          <Stack spacing={2}>
            <Stack direction="row" spacing={1} alignItems="center">
              <StorageIcon fontSize="small" />
              <Typography variant="h6">Files &amp; Media</Typography>
            </Stack>
            <Grid container spacing={2}>
              {[
                { label: 'File Path', value: book.file_path },
                { label: 'Original Filename', value: book.original_filename },
                { label: 'Format', value: book.format?.toUpperCase() },
                { label: 'Codec', value: book.codec },
                { label: 'Bitrate', value: book.bitrate ? `${book.bitrate} kbps` : undefined },
                {
                  label: 'Sample Rate',
                  value: book.sample_rate ? `${book.sample_rate} Hz` : undefined,
                },
                { label: 'Channels', value: book.channels },
                { label: 'Bit Depth', value: book.bit_depth },
                { label: 'Duration', value: formatDuration(book.duration) },
              ]
                .filter((item) => item.value !== undefined && item.value !== '')
                .map((item) => (
                  <Grid item xs={12} sm={6} md={4} key={item.label}>
                    <Box
                      sx={{
                        p: 2,
                        borderRadius: 1,
                        bgcolor: 'background.default',
                        border: '1px solid',
                        borderColor: 'divider',
                        height: '100%',
                      }}
                    >
                      <Typography
                        variant="caption"
                        color="text.secondary"
                        sx={{ textTransform: 'uppercase' }}
                      >
                        {item.label}
                      </Typography>
                      <Typography variant="body1" sx={{ wordBreak: 'break-all' }}>
                        {item.value as string}
                      </Typography>
                    </Box>
                  </Grid>
                ))}
            </Grid>
            <Divider />
            <Grid container spacing={2}>
              {[
                { label: 'File Hash', value: book.file_hash },
                { label: 'Original Hash', value: book.original_file_hash },
                { label: 'Organized Hash', value: book.organized_file_hash },
              ]
                .filter((item) => item.value)
                .map((item) => (
                  <Grid item xs={12} md={4} key={item.label}>
                    <Box
                      sx={{
                        p: 2,
                        borderRadius: 1,
                        bgcolor: 'grey.50',
                        border: '1px dashed',
                        borderColor: 'divider',
                        height: '100%',
                      }}
                    >
                      <Stack direction="row" justifyContent="space-between" alignItems="center">
                        <Typography variant="subtitle2">{item.label}</Typography>
                        <Tooltip title="Copy to clipboard">
                          <IconButton
                            size="small"
                            onClick={() => {
                              navigator.clipboard.writeText(item.value as string);
                              setAlert({
                                severity: 'success',
                                message: `${item.label} copied`,
                              });
                            }}
                          >
                            <FileCopyIcon fontSize="small" />
                          </IconButton>
                        </Tooltip>
                      </Stack>
                      <Typography variant="body2" sx={{ wordBreak: 'break-all' }}>
                        {item.value as string}
                      </Typography>
                    </Box>
                  </Grid>
                ))}
            </Grid>
          </Stack>
        </Paper>
      )}

      {activeTab === 'versions' && (
        <Paper sx={{ p: 3, mb: 3 }}>
          <Stack direction="row" alignItems="center" spacing={1} mb={2}>
            <CompareIcon />
            <Typography variant="h6">Versions</Typography>
          </Stack>
          {versionsError && (
            <Alert severity="error" sx={{ mb: 2 }}>
              {versionsError}
            </Alert>
          )}
          {versionsLoading ? (
            <Stack direction="row" spacing={1} alignItems="center">
              <CircularProgress size={20} />
              <Typography variant="body2">Loading versions...</Typography>
            </Stack>
          ) : versions.length === 0 ? (
            <Alert severity="info">No additional versions linked yet.</Alert>
          ) : (
            <Stack spacing={2}>
              {versions.map((version) => (
                <Box
                  key={version.id}
                  sx={{
                    p: 2,
                    borderRadius: 1,
                    border: '1px solid',
                    borderColor: version.id === book.id ? 'primary.main' : 'divider',
                    bgcolor: version.id === book.id ? 'primary.light' : 'background.paper',
                  }}
                >
                  <Stack direction={{ xs: 'column', md: 'row' }} spacing={1} alignItems="center">
                    <Box flex={1}>
                      <Typography variant="subtitle1">
                        {version.title} {version.id === book.id ? '(Current)' : ''}
                      </Typography>
                      <Typography variant="caption" color="text.secondary" display="block">
                        {version.file_path}
                      </Typography>
                    </Box>
                    <Stack direction="row" spacing={1} flexWrap="wrap">
                      {version.is_primary_version && <Chip label="Primary" color="primary" />}
                      {version.quality && <Chip label={version.quality} color="success" />}
                      {version.codec && <Chip label={version.codec} variant="outlined" />}
                      {version.format && (
                        <Chip label={version.format.toUpperCase()} variant="outlined" />
                      )}
                    </Stack>
                  </Stack>
                </Box>
              ))}
            </Stack>
          )}
          <Button
            variant="outlined"
            startIcon={<CompareIcon />}
            sx={{ mt: 2 }}
            onClick={() => setVersionDialogOpen(true)}
          >
            Open Version Manager
          </Button>
        </Paper>
      )}

      <Dialog open={deleteDialogOpen} onClose={() => setDeleteDialogOpen(false)}>
        <DialogTitle>Delete Audiobook</DialogTitle>
        <DialogContent dividers>
          <Typography variant="body1" gutterBottom>
            {deleteOptions.softDelete
              ? 'Soft delete hides the book from the library but keeps it available for purge or restore.'
              : 'Hard delete will remove this book immediately.'}
          </Typography>
          <FormControlLabel
            control={
              <Checkbox
                checked={deleteOptions.softDelete}
                onChange={(e) =>
                  setDeleteOptions((prev) => ({ ...prev, softDelete: e.target.checked }))
                }
              />
            }
            label="Soft delete (recommended)"
          />
          <FormControlLabel
            control={
              <Checkbox
                checked={deleteOptions.blockHash}
                onChange={(e) =>
                  setDeleteOptions((prev) => ({ ...prev, blockHash: e.target.checked }))
                }
              />
            }
            label="Prevent reimporting this file (block hash)"
          />
          <Alert severity="warning" sx={{ mt: 2 }}>
            Soft deleted books can be restored or purged later. Blocking the hash prevents
            reimports of the same file.
          </Alert>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDeleteDialogOpen(false)}>Cancel</Button>
          <Button
            onClick={handleDelete}
            color={deleteOptions.softDelete ? 'warning' : 'error'}
            variant="contained"
            disabled={actionLoading}
          >
            {deleteOptions.softDelete ? 'Soft Delete' : 'Delete Now'}
          </Button>
        </DialogActions>
      </Dialog>

      <Dialog open={purgeDialogOpen} onClose={() => setPurgeDialogOpen(false)}>
        <DialogTitle>Purge Audiobook</DialogTitle>
        <DialogContent dividers>
          <Alert severity="error" sx={{ mb: 2 }}>
            This permanently removes the record and associated files. This cannot be undone.
          </Alert>
          <Typography>
            Are you sure you want to purge{' '}
            <strong>{book.title || 'this audiobook'}</strong> from the library?
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setPurgeDialogOpen(false)}>Cancel</Button>
          <Button
            onClick={handlePurge}
            color="error"
            variant="contained"
            disabled={actionLoading}
          >
            Purge Permanently
          </Button>
        </DialogActions>
      </Dialog>

      <VersionManagement
        audiobookId={book.id}
        open={versionDialogOpen}
        onClose={() => setVersionDialogOpen(false)}
        onUpdate={() => {
          loadVersions();
          loadBook();
        }}
      />
    </Box>
  );
};
