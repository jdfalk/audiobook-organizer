// file: web/src/pages/BookDetail.tsx
// version: 1.11.0
// guid: 4d2f7c6a-1b3e-4c5d-8f7a-9b0c1d2e3f4a

import { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import {
  Avatar,
  Box,
  Paper,
  Stack,
  Chip,
  Button,
  CircularProgress,
  Alert,
  Typography,
  Grid,
  Tabs,
  Tab,
  FormControlLabel,
  Checkbox,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  LinearProgress,
  Table,
  TableHead,
  TableRow,
  TableCell,
  TableBody,
} from '@mui/material';
import ArrowBackIcon from '@mui/icons-material/ArrowBack.js';
import DeleteIcon from '@mui/icons-material/Delete.js';
import RestoreIcon from '@mui/icons-material/Restore.js';
import EditIcon from '@mui/icons-material/Edit.js';
import PsychologyIcon from '@mui/icons-material/Psychology.js';
import CloudDownloadIcon from '@mui/icons-material/CloudDownload.js';
import CompareIcon from '@mui/icons-material/Compare.js';
import InfoIcon from '@mui/icons-material/Info.js';
import AccessTimeIcon from '@mui/icons-material/AccessTime.js';
import StorageIcon from '@mui/icons-material/Storage.js';
import type { Book, BookTags, OverridePayload } from '../services/api';
import * as api from '../services/api';
import { VersionManagement } from '../components/audiobooks/VersionManagement';
import { MetadataEditDialog } from '../components/audiobooks/MetadataEditDialog';
import { useToast } from '../components/toast/ToastProvider';
import type { Audiobook } from '../types';

export const BookDetail = () => {
  const { id } = useParams();
  const navigate = useNavigate();
  const [book, setBook] = useState<Book | null>(null);
  const [loading, setLoading] = useState(true);
  const [actionLoading, setActionLoading] = useState(false);
  const [actionLabel, setActionLabel] = useState<string | null>(null);
  const [loadingField, setLoadingField] = useState<string | null>(null);
  const [fetchingMetadata, setFetchingMetadata] = useState(false);
  const [parsingWithAI, setParsingWithAI] = useState(false);
  const { toast } = useToast();
  const [activeTab, setActiveTab] = useState<
    'info' | 'files' | 'versions' | 'tags' | 'compare'
  >('info');
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deleteOptions, setDeleteOptions] = useState({
    softDelete: true,
    blockHash: true,
  });
  const [conflictDialogOpen, setConflictDialogOpen] = useState(false);
  const [pendingUpdate, setPendingUpdate] =
    useState<Partial<Book> | null>(null);
  const [purgeDialogOpen, setPurgeDialogOpen] = useState(false);
  const [versions, setVersions] = useState<Book[]>([]);
  const [versionsLoading, setVersionsLoading] = useState(false);
  const [versionsError, setVersionsError] = useState<string | null>(null);
  const [versionDialogOpen, setVersionDialogOpen] = useState(false);
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [tags, setTags] = useState<BookTags | null>(null);
  const [tagsLoading, setTagsLoading] = useState(false);
  const [tagsError, setTagsError] = useState<string | null>(null);
  const [segments, setSegments] = useState<Array<{
    id: string;
    file_path: string;
    format: string;
    size_bytes: number;
    duration_seconds: number;
    track_number?: number;
    total_tracks?: number;
    active: boolean;
  }>>([]);
  const [segmentsLoaded, setSegmentsLoaded] = useState(false);

  const loadBook = useCallback(async () => {
    if (!id) return;
    setLoading(true);
    try {
      const data = await api.getBook(id);
      setBook(data);
    } catch (error) {
      if (error instanceof api.ApiError) {
        if (error.status === 404) {
          setBook(null);
          return;
        }
        if (error.status === 401) {
          toast('Session expired.', 'error');
          navigate('/login');
          return;
        }
      }
      console.error('Failed to load book', error);
      toast('Failed to load audiobook details.', 'error');
    } finally {
      setLoading(false);
    }
  }, [id, navigate, toast]);

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

  const loadTags = useCallback(async () => {
    if (!id) return;
    setTagsLoading(true);
    setTagsError(null);
    try {
      const data = await api.getBookTags(id);
      setTags(data);
    } catch (error) {
      console.error('Failed to load tags', error);
      setTagsError('Unable to load file tags at the moment.');
    } finally {
      setTagsLoading(false);
    }
  }, [id]);

  useEffect(() => {
    loadBook();
    loadVersions();
    loadTags();
  }, [id, loadBook, loadTags, loadVersions]);

  // Load segments when files tab is active
  useEffect(() => {
    if (activeTab !== 'files' || !id || segmentsLoaded) return;
    const loadSegments = async () => {
      try {
        const resp = await fetch(`/api/v1/audiobooks/${id}/segments`);
        if (resp.ok) {
          const data = await resp.json();
          setSegments(data);
        }
      } catch {
        // Segments not available, that's fine
      } finally {
        setSegmentsLoaded(true);
      }
    };
    loadSegments();
  }, [activeTab, id, segmentsLoaded]);

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
    setActionLabel(
      deleteOptions.softDelete ? 'Soft deleting...' : 'Deleting...'
    );
    try {
      const result = await api.deleteBook(book.id, {
        softDelete: deleteOptions.softDelete,
        blockHash: deleteOptions.blockHash,
      });
      const baseMessage = deleteOptions.softDelete
        ? 'Audiobook marked for deletion.'
        : 'Audiobook deleted permanently.';
      const blockNotice = deleteOptions.blockHash
        ? result.blocked
          ? ' Hash blocked.'
          : ' Hash could not be blocked.'
        : '';
      const severity =
        deleteOptions.blockHash && !result.blocked ? 'warning' : 'success';
      toast(`${baseMessage}${blockNotice}`, severity);
      setDeleteDialogOpen(false);
      await loadBook();
    } catch (error) {
      console.error('Failed to delete audiobook', error);
      toast('Failed to delete audiobook.', 'error');
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
      toast('Audiobook restored.', 'success');
      await loadBook();
    } catch (error) {
      console.error('Failed to restore audiobook', error);
      toast('Failed to restore audiobook.', 'error');
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
      toast('Audiobook permanently deleted.', 'success');
      navigate('/library');
    } catch (error) {
      console.error('Failed to purge audiobook', error);
      toast('Failed to purge audiobook.', 'error');
    } finally {
      setActionLabel(null);
      setActionLoading(false);
    }
  };

  const handleFetchMetadata = async () => {
    if (!book) return;
    setFetchingMetadata(true);
    try {
      const result = await api.fetchBookMetadata(book.id);
      setBook(result.book);
      // Reload tags to reflect updated stored values in Compare tab
      await loadTags();
      toast(
        result.message ||
          `Metadata refreshed from ${result.source || 'provider'}.`,
        'success'
      );
    } catch (error: unknown) {
      console.error('Failed to fetch metadata', error);
      const msg =
        error instanceof Error ? error.message : 'Metadata fetch failed.';
      toast(msg, 'error');
    } finally {
      setFetchingMetadata(false);
    }
  };

  const handleParseWithAI = async () => {
    if (!book) return;
    setParsingWithAI(true);
    try {
      const result = await api.parseAudiobookWithAI(book.id);
      setBook(result.book);
      // Reload tags to reflect updated stored values in Compare tab
      await loadTags();
      toast(result.message || 'AI parsing completed.', 'success');
    } catch (error: unknown) {
      console.error('Failed to parse with AI', error);
      const msg =
        error instanceof Error ? error.message : 'AI parsing failed.';
      toast(msg, 'error');
    } finally {
      setParsingWithAI(false);
    }
  };

  const versionSummary = useMemo(() => {
    if (!book) return null;
    const primary = versions.find((v) => v.is_primary_version);
    const linked = versions.filter((v) => v.id !== book.id);
    return { primary, linkedCount: linked.length };
  }, [book, versions]);

  const mapBookToAudiobook = (current: Book): Audiobook => ({
    id: current.id,
    title: current.title,
    author: current.author_name,
    narrator: current.narrator,
    series: current.series_name,
    series_number: current.series_position,
    language: current.language,
    publisher: current.publisher,
    description: current.description,
    duration_seconds: current.duration,
    file_path: current.file_path,
    original_filename: current.original_filename,
    cover_path: current.cover_image,
    format: current.format,
    bitrate_kbps: current.bitrate,
    codec: current.codec,
    sample_rate_hz: current.sample_rate,
    channels: current.channels,
    bit_depth: current.bit_depth,
    quality: current.quality,
    is_primary_version: current.is_primary_version,
    version_group_id: current.version_group_id,
    version_notes: current.version_notes,
    file_hash: current.file_hash,
    original_file_hash: current.original_file_hash,
    organized_file_hash: current.organized_file_hash,
    library_state: current.library_state,
    quantity: current.quantity,
    marked_for_deletion: current.marked_for_deletion,
    marked_for_deletion_at: current.marked_for_deletion_at,
    created_at: current.created_at,
    updated_at: current.updated_at,
    work_id: current.work_id,
  });

  const getFieldSources = (field: string) => {
    const entry = tags?.tags?.[field];
    if (!entry) return null;
    const effective =
      entry.effective_value ??
      entry.override_value ??
      entry.stored_value ??
      entry.fetched_value ??
      entry.file_value;
    const effectiveSource =
      entry.effective_source ||
      (entry.override_value !== undefined && entry.override_value !== null
        ? 'override'
        : entry.stored_value !== undefined && entry.stored_value !== null
          ? 'stored'
          : entry.fetched_value !== undefined && entry.fetched_value !== null
            ? 'fetched'
            : entry.file_value !== undefined && entry.file_value !== null
              ? 'file'
              : undefined);
    return {
      file: entry.file_value,
      fetched: entry.fetched_value,
      stored: entry.stored_value,
      override: entry.override_value,
      locked: entry.override_locked,
      effective,
      source: effectiveSource,
      updatedAt: entry.updated_at,
    };
  };

  const applySourceValue = async (
    field: string,
    source: 'file' | 'fetched' | 'override'
  ) => {
    if (!book) return;
    const entry = getFieldSources(field);
    if (!entry) return;
    const value =
      source === 'file'
        ? entry.file
        : source === 'fetched'
          ? entry.fetched
          : entry.override;
    if (value === undefined) return;
    setLoadingField(`${field}-${source}`);
    try {
      const override: OverridePayload = { value, locked: true };
      if (source === 'file') {
        override.fetched_value = entry.fetched;
      }
      const payload: Partial<Book> & {
        overrides: Record<string, OverridePayload>;
      } = {
        overrides: {
          [field]: override,
        },
      };
      const saved = await api.updateBook(book.id, payload);
      setBook(saved);
      // Update local tags state to reflect new stored/override value
      setTags((prev) => {
        if (!prev?.tags) return prev;
        const updated = { ...prev.tags[field] };
        updated.override_value = value;
        updated.override_locked = true;
        updated.fetched_value = entry.fetched ?? updated.fetched_value;
        updated.effective_value = value as never;
        updated.effective_source = 'override';
        return {
          ...prev,
          tags: {
            ...prev.tags,
            [field]: updated,
          },
        };
      });
    } catch (error) {
      console.error('Failed to apply field value', error);
      toast('Failed to apply field value.', 'error');
    } finally {
      setLoadingField(null);
    }
  };

  const clearOverride = async (field: string) => {
    if (!book) return;
    setLoadingField(`${field}-clear`);
    try {
      const payload: Partial<Book> & {
        overrides: Record<string, { clear: boolean }>;
      } = {
        overrides: {
          [field]: { clear: true },
        },
      } as never;
      const saved = await api.updateBook(book.id, payload);
      setBook(saved);
      setTags((prev) => {
        if (!prev?.tags) return prev;
        const updated = { ...prev.tags[field] };
        updated.override_value = null;
        updated.override_locked = false;
        const effectiveValue =
          updated.stored_value ??
          updated.fetched_value ??
          updated.file_value ??
          (updated as { effective_value?: unknown }).effective_value ??
          null;
        const effectiveSource =
          (updated.stored_value ??
            updated.fetched_value ??
            updated.file_value) !== undefined
            ? updated.stored_value !== undefined &&
              updated.stored_value !== null
              ? 'stored'
              : updated.fetched_value !== undefined &&
                  updated.fetched_value !== null
                ? 'fetched'
                : updated.file_value !== undefined &&
                    updated.file_value !== null
                  ? 'file'
                  : undefined
            : undefined;
        updated.effective_value = effectiveValue as never;
        updated.effective_source = effectiveSource;
        return {
          ...prev,
          tags: {
            ...prev.tags,
            [field]: updated,
          },
        };
      });
    } catch (error) {
      console.error('Failed to clear override', error);
      toast('Failed to clear override.', 'error');
    } finally {
      setLoadingField(null);
    }
  };

  const unlockOverride = async (field: string) => {
    if (!book) return;
    setLoadingField(`${field}-unlock`);
    try {
      const payload: Partial<Book> & {
        overrides: Record<string, { locked: boolean }>;
      } = {
        overrides: {
          [field]: { locked: false },
        },
      } as never;
      const saved = await api.updateBook(book.id, payload);
      setBook(saved);
      setTags((prev) => {
        if (!prev?.tags) return prev;
        const updated = { ...prev.tags[field] };
        updated.override_locked = false;
        return {
          ...prev,
          tags: {
            ...prev.tags,
            [field]: updated,
          },
        };
      });
    } catch (error) {
      console.error('Failed to unlock override', error);
      toast('Failed to unlock override.', 'error');
    } finally {
      setLoadingField(null);
    }
  };

  const handleEditSave = async (updated: Audiobook) => {
    if (!book) return;
    setActionLoading(true);
    setActionLabel('Saving changes...');
    try {
      const payload: Partial<Book> = {
        title: updated.title,
        description: updated.description,
        publisher: updated.publisher,
        language: updated.language,
        narrator: updated.narrator,
        series_position: updated.series_number,
        audiobook_release_year:
          (updated as unknown as { audiobook_release_year?: number })
            .audiobook_release_year ||
          updated.year ||
          book.audiobook_release_year,
        print_year: updated.year || book.print_year,
        isbn:
          (updated as unknown as { isbn13?: string }).isbn13 ||
          (updated as unknown as { isbn10?: string }).isbn10 ||
          book.isbn,
        author_name: updated.author,
        series_name: updated.series,
      };
      const saved = await api.updateBook(book.id, payload);
      setBook(saved);
      toast('Metadata updated.', 'success');
      setEditDialogOpen(false);
    } catch (error) {
      if (error instanceof api.ApiError) {
        if (error.status === 409) {
          setPendingUpdate(null);
          setConflictDialogOpen(true);
          toast('Book was updated by another user.', 'warning');
          return;
        }
        if (error.status === 401) {
          toast('Session expired.', 'error');
          navigate('/login');
          return;
        }
      }
      console.error('Failed to update metadata', error);
      toast('Failed to update metadata.', 'error');
    } finally {
      setActionLabel(null);
      setActionLoading(false);
    }
  };

  const handleConflictReload = async () => {
    setConflictDialogOpen(false);
    setPendingUpdate(null);
    await loadBook();
  };

  const handleConflictOverwrite = async () => {
    if (!book || !pendingUpdate) return;
    setActionLoading(true);
    setActionLabel('Overwriting...');
    try {
      const saved = await api.updateBook(book.id, {
        ...pendingUpdate,
        force_update: true,
      });
      setBook(saved);
      toast('Metadata updated.', 'success');
      setEditDialogOpen(false);
      setConflictDialogOpen(false);
      setPendingUpdate(null);
    } catch (error) {
      console.error('Failed to overwrite metadata', error);
      toast('Failed to overwrite metadata.', 'error');
    } finally {
      setActionLabel(null);
      setActionLoading(false);
    }
  };

  if (loading) {
    return (
      <Box
        display="flex"
        alignItems="center"
        justifyContent="center"
        height="100%"
      >
        <CircularProgress />
      </Box>
    );
  }

  if (!book) {
    return (
      <Box p={3}>
        <Alert severity="error">Audiobook not found.</Alert>
        <Button
          startIcon={<ArrowBackIcon />}
          sx={{ mt: 2 }}
          onClick={() => navigate('/library')}
        >
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

      <Stack
        direction="row"
        alignItems="center"
        spacing={2}
        mb={3}
        flexWrap="wrap"
      >
        <Button
          startIcon={<ArrowBackIcon />}
          variant="text"
          onClick={() => navigate('/library')}
        >
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
              {isSoftDeleted && (
                <Chip label="Soft Deleted" color="warning" size="small" />
              )}
              {book.library_state && (
                <Chip
                  label={`State: ${book.library_state}`}
                  color="info"
                  size="small"
                />
              )}
              {book.is_primary_version && (
                <Chip label="Primary Version" color="primary" />
              )}
            </Box>
            <Typography variant="subtitle1" color="text.secondary">
              {book.author_name || book.author_id
                ? `By ${book.author_name || book.author_id}`
                : ''}
            </Typography>
          </Stack>
        </Stack>
      </Stack>

      {isSoftDeleted && (
        <Alert
          severity="warning"
          action={
            <Stack direction="row" spacing={1}>
              <Button
                color="inherit"
                size="small"
                onClick={handleRestore}
                disabled={actionLoading}
              >
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
          Marked for deletion on {formatDateTime(book.marked_for_deletion_at)}.
          Restore to keep the book or purge to remove it permanently.
        </Alert>
      )}

      <Paper sx={{ p: 3, mb: 3 }}>
        <Stack
          direction={{ xs: 'column', md: 'row' }}
          spacing={2}
          justifyContent="space-between"
        >
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
          <Stack
            direction="row"
            spacing={1}
            flexWrap="wrap"
            justifyContent="flex-end"
          >
            <Button
              variant="outlined"
              startIcon={<EditIcon />}
              onClick={() => setEditDialogOpen(true)}
              disabled={actionLoading}
            >
              Edit Metadata
            </Button>
            <Button
              variant="outlined"
              startIcon={
                fetchingMetadata ? (
                  <CircularProgress size={20} />
                ) : (
                  <CloudDownloadIcon />
                )
              }
              onClick={handleFetchMetadata}
              disabled={fetchingMetadata || actionLoading}
            >
              {fetchingMetadata ? 'Fetching...' : 'Fetch Metadata'}
            </Button>
            <Button
              variant="outlined"
              startIcon={
                parsingWithAI ? (
                  <CircularProgress size={20} />
                ) : (
                  <PsychologyIcon />
                )
              }
              onClick={handleParseWithAI}
              disabled={parsingWithAI || actionLoading}
            >
              {parsingWithAI ? 'Parsing...' : 'Parse with AI'}
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
          <Tab label="Tags" value="tags" />
          <Tab label="Compare" value="compare" />
        </Tabs>
      </Paper>

      {activeTab === 'info' && (
        <Paper sx={{ p: 3, mb: 3 }}>
          <Grid container spacing={2}>
            {[
              { label: 'Title', value: book.title || 'Untitled' },
              { label: 'Author', value: book.author_name || 'Unknown' },
              { label: 'Series', value: book.series_name },
              { label: 'Narrator', value: book.narrator },
              { label: 'Language', value: book.language },
              { label: 'ISBN 13', value: book.isbn13 },
              { label: 'Work ID', value: book.work_id },
            ]
              .filter((item) => item.value !== undefined && item.value !== '' && item.value !== null)
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
                    <Typography variant="body1">
                      {item.value as string}
                    </Typography>
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
                {
                  label: 'Bitrate',
                  value: book.bitrate ? `${book.bitrate} kbps` : undefined,
                },
                {
                  label: 'Sample Rate',
                  value: book.sample_rate
                    ? `${book.sample_rate} Hz`
                    : undefined,
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
                      <Typography
                        variant="body1"
                        sx={{ wordBreak: 'break-all' }}
                      >
                        {item.value as string}
                      </Typography>
                    </Box>
                  </Grid>
                ))}
            </Grid>

            {segments.length > 0 && (
              <Box>
                <Typography variant="h6" sx={{ mt: 2, mb: 1 }}>Individual Files</Typography>
                <Table size="small">
                  <TableHead>
                    <TableRow>
                      <TableCell>Track #</TableCell>
                      <TableCell>File Name</TableCell>
                      <TableCell>Duration</TableCell>
                      <TableCell>Format</TableCell>
                      <TableCell align="right">Size</TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {segments.map((seg) => (
                      <TableRow key={seg.id}>
                        <TableCell>{seg.track_number ?? '—'}</TableCell>
                        <TableCell sx={{ wordBreak: 'break-all' }}>
                          {seg.file_path.split('/').pop()}
                        </TableCell>
                        <TableCell>{formatDuration(seg.duration_seconds)}</TableCell>
                        <TableCell>{seg.format?.toUpperCase()}</TableCell>
                        <TableCell align="right">
                          {seg.size_bytes > 0
                            ? `${(seg.size_bytes / 1048576).toFixed(1)} MB`
                            : '—'}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </Box>
            )}
          </Stack>
        </Paper>
      )}

      {activeTab === 'versions' && (
        <Paper sx={{ p: 3, mb: 3 }}>
          <Stack direction="row" alignItems="center" spacing={1} mb={2}>
            <CompareIcon />
            <Typography variant="h6">Versions</Typography>
          </Stack>
          {versionSummary?.linkedCount ? (
            <Alert severity="info" sx={{ mb: 2 }}>
              Part of version group with {versionSummary.linkedCount + 1} books.
            </Alert>
          ) : null}
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
                    borderColor:
                      version.id === book.id ? 'primary.main' : 'divider',
                    bgcolor:
                      version.id === book.id
                        ? 'primary.light'
                        : 'background.paper',
                    cursor: version.id === book.id ? 'default' : 'pointer',
                  }}
                  onClick={() => {
                    if (version.id !== book.id) {
                      navigate(`/library/${version.id}`);
                    }
                  }}
                >
                  <Stack
                    direction={{ xs: 'column', md: 'row' }}
                    spacing={1}
                    alignItems="center"
                  >
                    <Box flex={1}>
                      <Typography variant="subtitle1">
                        {version.title}{' '}
                        {version.id === book.id ? '(Current)' : ''}
                      </Typography>
                      <Typography
                        variant="caption"
                        color="text.secondary"
                        display="block"
                      >
                        {version.file_path}
                      </Typography>
                    </Box>
                    <Stack direction="row" spacing={1} flexWrap="wrap">
                      {version.is_primary_version && (
                        <Chip label="Primary" color="primary" />
                      )}
                      {version.quality && (
                        <Chip label={version.quality} color="success" />
                      )}
                      {version.codec && (
                        <Chip label={version.codec} variant="outlined" />
                      )}
                      {version.format && (
                        <Chip
                          label={version.format.toUpperCase()}
                          variant="outlined"
                        />
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

      {activeTab === 'tags' && (
        <Paper sx={{ p: 3, mb: 3 }}>
          <Stack direction="row" alignItems="center" spacing={1} mb={2}>
            <InfoIcon />
            <Typography variant="h6">Tags &amp; Media</Typography>
          </Stack>
          {tagsError && (
            <Alert severity="error" sx={{ mb: 2 }}>
              {tagsError}
            </Alert>
          )}
          {tagsLoading && (
            <Stack direction="row" spacing={1} alignItems="center" mb={2}>
              <CircularProgress size={18} />
              <Typography variant="body2">Loading tags...</Typography>
            </Stack>
          )}
          <Grid container spacing={2}>
            <Grid item xs={12} md={6}>
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
                <Typography variant="subtitle2" gutterBottom>
                  Embedded / Media Info
                </Typography>
                <Stack spacing={1}>
                  <Typography variant="body2">
                    Codec: {tags?.media_info?.codec || book.codec || '—'}
                  </Typography>
                  <Typography variant="body2">
                    Bitrate:{' '}
                    {tags?.media_info?.bitrate
                      ? `${tags.media_info.bitrate} kbps`
                      : book.bitrate
                        ? `${book.bitrate} kbps`
                        : '—'}
                  </Typography>
                  <Typography variant="body2">
                    Sample Rate:{' '}
                    {tags?.media_info?.sample_rate
                      ? `${tags.media_info.sample_rate} Hz`
                      : book.sample_rate
                        ? `${book.sample_rate} Hz`
                        : '—'}
                  </Typography>
                  <Typography variant="body2">
                    Channels:{' '}
                    {tags?.media_info?.channels ?? book.channels ?? '—'}
                  </Typography>
                  <Typography variant="body2">
                    Bit Depth:{' '}
                    {tags?.media_info?.bit_depth ?? book.bit_depth ?? '—'}
                  </Typography>
                  <Typography variant="body2">
                    Quality: {tags?.media_info?.quality || book.quality || '—'}
                  </Typography>
                  <Typography variant="body2">
                    Duration:{' '}
                    {formatDuration(
                      tags?.media_info?.duration || book.duration
                    ) || '—'}
                  </Typography>
                </Stack>
              </Box>
            </Grid>
            <Grid item xs={12} md={6}>
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
                <Typography variant="subtitle2" gutterBottom>
                  Metadata (Current)
                </Typography>
                <Stack spacing={1}>
                  <Typography variant="body2">
                    Title: {book.title || '—'}
                  </Typography>
                  <Typography variant="body2">
                    Author: {book.author_name || '—'}
                  </Typography>
                  <Typography variant="body2">
                    Narrator: {book.narrator || '—'}
                  </Typography>
                  <Typography variant="body2">
                    Series: {book.series_name || '—'}
                  </Typography>
                  <Typography variant="body2">
                    Series Position: {book.series_position ?? '—'}
                  </Typography>
                  <Typography variant="body2">
                    Publisher: {book.publisher || '—'}
                  </Typography>
                  <Typography variant="body2">
                    Language: {book.language || '—'}
                  </Typography>
                  <Typography variant="body2">
                    Year:{' '}
                    {book.audiobook_release_year || book.print_year || '—'}
                  </Typography>
                  <Typography variant="body2">
                    ISBN: {book.isbn || '—'}
                  </Typography>
                </Stack>
              </Box>
            </Grid>
          </Grid>
          {tags?.tags && (
            <Box mt={3}>
              <Typography variant="subtitle2" gutterBottom>
                File Tags
              </Typography>
              <Grid container spacing={1}>
                {Object.entries(tags.tags).map(([key, values]) => (
                  <Grid item xs={12} sm={6} md={4} key={key}>
                    <Box
                      sx={{
                        p: 1.5,
                        borderRadius: 1,
                        border: '1px dashed',
                        borderColor: 'divider',
                        bgcolor: 'background.default',
                      }}
                    >
                      <Typography
                        variant="caption"
                        color="text.secondary"
                        sx={{ textTransform: 'uppercase' }}
                      >
                        {key.replace(/_/g, ' ')}
                      </Typography>
                      <Stack
                        direction="row"
                        spacing={1}
                        alignItems="center"
                        flexWrap="wrap"
                      >
                        <Typography variant="body2">
                          {values.effective_value ??
                            values.override_value ??
                            values.stored_value ??
                            values.fetched_value ??
                            values.file_value ??
                            '—'}
                        </Typography>
                        {values.effective_source && (
                          <Chip
                            size="small"
                            label={values.effective_source}
                            variant="outlined"
                          />
                        )}
                        {values.override_locked && (
                          <Chip
                            size="small"
                            label="locked"
                            color="warning"
                            variant="outlined"
                          />
                        )}
                      </Stack>
                    </Box>
                  </Grid>
                ))}
              </Grid>
            </Box>
          )}
          <Alert severity="info" sx={{ mt: 2 }}>
            Showing current metadata and media info. File-tag provenance will
            populate here when available from the backend.
          </Alert>
        </Paper>
      )}

      {activeTab === 'compare' && (
        <Paper sx={{ p: 3, mb: 3 }}>
          <Stack direction="row" alignItems="center" spacing={1} mb={2}>
            <CompareIcon />
            <Typography variant="h6">Compare &amp; Resolve</Typography>
          </Stack>
          {tagsError && (
            <Alert severity="error" sx={{ mb: 2 }}>
              {tagsError}
            </Alert>
          )}
          {!tags?.tags && !tagsLoading ? (
            <Alert severity="info">No tag data available yet.</Alert>
          ) : (
            <Table size="small">
              <TableHead>
                <TableRow>
                  <TableCell>Field</TableCell>
                  <TableCell>File Tag</TableCell>
                  <TableCell>Fetched</TableCell>
                  <TableCell>Stored</TableCell>
                  <TableCell>Override</TableCell>
                  <TableCell>Actions</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {[
                  'title',
                  'author_name',
                  'narrator',
                  'series_name',
                  'publisher',
                  'language',
                  'audiobook_release_year',
                ].map((field) => {
                  const entry = getFieldSources(field);
                  return (
                    <TableRow key={field}>
                      <TableCell sx={{ textTransform: 'capitalize' }}>
                        {field.replace(/_/g, ' ')}
                      </TableCell>
                      <TableCell>{entry?.file ?? '—'}</TableCell>
                      <TableCell>{entry?.fetched ?? '—'}</TableCell>
                      <TableCell>{entry?.stored ?? '—'}</TableCell>
                      <TableCell>
                        <Stack
                          direction="row"
                          spacing={1}
                          alignItems="center"
                          flexWrap="wrap"
                        >
                          <span>{entry?.override ?? '—'}</span>
                          {entry?.locked && (
                            <Chip
                              label="locked"
                              size="small"
                              color="warning"
                              sx={{ ml: 0.5 }}
                            />
                          )}
                          {entry?.source && (
                            <Chip
                              label={entry.source}
                              size="small"
                              variant="outlined"
                            />
                          )}
                        </Stack>
                      </TableCell>
                      <TableCell>
                        <Stack direction="row" spacing={1}>
                          <Button
                            size="small"
                            variant="outlined"
                            onClick={() => applySourceValue(field, 'file')}
                            disabled={
                              (!entry?.file && entry?.file !== 0) ||
                              loadingField === `${field}-file`
                            }
                            startIcon={
                              loadingField === `${field}-file` ? (
                                <CircularProgress size={16} />
                              ) : undefined
                            }
                          >
                            {loadingField === `${field}-file`
                              ? 'Applying...'
                              : 'Use File'}
                          </Button>
                          <Button
                            size="small"
                            variant="outlined"
                            onClick={() => applySourceValue(field, 'fetched')}
                            disabled={
                              (!entry?.fetched && entry?.fetched !== 0) ||
                              loadingField === `${field}-fetched`
                            }
                            startIcon={
                              loadingField === `${field}-fetched` ? (
                                <CircularProgress size={16} />
                              ) : undefined
                            }
                          >
                            {loadingField === `${field}-fetched`
                              ? 'Applying...'
                              : 'Use Fetched'}
                          </Button>
                          {entry?.override && (
                            <Button
                              size="small"
                              variant="outlined"
                              color="secondary"
                              onClick={() => clearOverride(field)}
                              disabled={loadingField === `${field}-clear`}
                              startIcon={
                                loadingField === `${field}-clear` ? (
                                  <CircularProgress size={16} />
                                ) : undefined
                              }
                            >
                              {loadingField === `${field}-clear`
                                ? 'Clearing...'
                                : 'Clear'}
                            </Button>
                          )}
                          {entry?.locked && (
                            <Button
                              size="small"
                              variant="outlined"
                              onClick={() => unlockOverride(field)}
                              disabled={loadingField === `${field}-unlock`}
                              startIcon={
                                loadingField === `${field}-unlock` ? (
                                  <CircularProgress size={16} />
                                ) : undefined
                              }
                            >
                              {loadingField === `${field}-unlock`
                                ? 'Unlocking...'
                                : 'Unlock'}
                            </Button>
                          )}
                        </Stack>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          )}
          <Alert severity="info" sx={{ mt: 2 }}>
            Locked overrides prevent future fetch/tag updates from changing the
            field. Apply a source to set/lock a field; use Unlock to allow edits
            without clearing the override value.
          </Alert>
        </Paper>
      )}

      <Dialog
        open={deleteDialogOpen}
        onClose={() => setDeleteDialogOpen(false)}
      >
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
                  setDeleteOptions((prev) => ({
                    ...prev,
                    softDelete: e.target.checked,
                  }))
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
                  setDeleteOptions((prev) => ({
                    ...prev,
                    blockHash: e.target.checked,
                  }))
                }
              />
            }
            label="Prevent reimporting this file (block hash)"
          />
          <Alert severity="warning" sx={{ mt: 2 }}>
            Soft deleted books can be restored or purged later. Blocking the
            hash prevents reimports of the same file.
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
            This permanently removes the record and associated files. This
            cannot be undone.
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

      <MetadataEditDialog
        open={editDialogOpen}
        audiobook={book ? mapBookToAudiobook(book) : null}
        onClose={() => setEditDialogOpen(false)}
        onSave={handleEditSave}
      />

      <Dialog
        open={conflictDialogOpen}
        onClose={() => setConflictDialogOpen(false)}
      >
        <DialogTitle>Update Conflict</DialogTitle>
        <DialogContent>
          <Typography variant="body1" gutterBottom>
            This audiobook was updated by another user while you were editing.
          </Typography>
          <Typography variant="body2" color="text.secondary">
            Reload to fetch the latest data, or overwrite to save your changes
            anyway.
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleConflictReload}>Reload</Button>
          <Button variant="contained" onClick={handleConflictOverwrite}>
            Overwrite
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
