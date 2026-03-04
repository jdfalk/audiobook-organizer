// file: web/src/pages/BookDetail.tsx
// version: 1.28.0
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
  IconButton,
  Tooltip,
} from '@mui/material';
import ArrowBackIcon from '@mui/icons-material/ArrowBack.js';
import DeleteIcon from '@mui/icons-material/Delete.js';
import RestoreIcon from '@mui/icons-material/Restore.js';
import EditIcon from '@mui/icons-material/Edit.js';
import PsychologyIcon from '@mui/icons-material/Psychology.js';
import CloudDownloadIcon from '@mui/icons-material/CloudDownload.js';
import CompareIcon from '@mui/icons-material/Compare.js';
import HistoryIcon from '@mui/icons-material/History.js';
import InfoIcon from '@mui/icons-material/Info.js';
import AccessTimeIcon from '@mui/icons-material/AccessTime.js';
import StorageIcon from '@mui/icons-material/Storage.js';
import SaveIcon from '@mui/icons-material/Save.js';
import SearchIcon from '@mui/icons-material/Search.js';
import TransformIcon from '@mui/icons-material/Transform.js';
import StarIcon from '@mui/icons-material/Star.js';
import StarBorderIcon from '@mui/icons-material/StarBorder.js';
import LinkIcon from '@mui/icons-material/Link.js';
import LinkOffIcon from '@mui/icons-material/LinkOff.js';
import type { Book, BookSegment, SegmentTags, OverridePayload } from '../services/api';
import * as api from '../services/api';
import { VersionManagement } from '../components/audiobooks/VersionManagement';
import { MetadataEditDialog } from '../components/audiobooks/MetadataEditDialog';
import { MetadataHistory } from '../components/MetadataHistory';
import { MetadataSearchDialog } from '../components/audiobooks/MetadataSearchDialog';
import { FileSelector } from '../components/audiobooks/FileSelector';
import { useToast } from '../components/toast/ToastProvider';
import type { Audiobook } from '../types';

export const BookDetail = () => {
  const { id } = useParams();
  const navigate = useNavigate();
  const [book, setBook] = useState<Book | null>(null);
  const [loading, setLoading] = useState(true);
  const [actionLoading, setActionLoading] = useState(false);
  const [actionLabel, setActionLabel] = useState<string | null>(null);
  const [fetchingMetadata, setFetchingMetadata] = useState(false);
  const [parsingWithAI, setParsingWithAI] = useState(false);
  const [writingToFiles, setWritingToFiles] = useState(false);
  const [transcoding, setTranscoding] = useState(false);
  const [writeBackDialogOpen, setWriteBackDialogOpen] = useState(false);
  // preAIBook removed — use History to revert AI changes
  const { toast } = useToast();
  const [activeTab, setActiveTab] = useState<
    'info' | 'files' | 'versions'
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
  const [purgeConfirmed, setPurgeConfirmed] = useState(false);
  const [versions, setVersions] = useState<Book[]>([]);
  const [versionsLoading, setVersionsLoading] = useState(false);
  const [versionsError, setVersionsError] = useState<string | null>(null);
  const [versionDialogOpen, setVersionDialogOpen] = useState(false);
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [historyDialogOpen, setHistoryDialogOpen] = useState(false);
  const [metadataSearchOpen, setMetadataSearchOpen] = useState(false);
  const [segments, setSegments] = useState<BookSegment[]>([]);
  const [segmentsLoaded, setSegmentsLoaded] = useState(false);
  const [selectedSegmentIds, setSelectedSegmentIds] = useState<Set<string>>(new Set());
  const [segmentTags, setSegmentTags] = useState<SegmentTags | null>(null);
  const [segmentTagsLoading, setSegmentTagsLoading] = useState(false);
  const [coverError, setCoverError] = useState(false);
  const [coverLightboxOpen, setCoverLightboxOpen] = useState(false);

  // Derived multi-select state
  const isSingleSelect = selectedSegmentIds.size === 1;
  const singleSelectedId = isSingleSelect ? Array.from(selectedSegmentIds)[0] : null;

  // Reset cover error when cover URL changes (e.g. after metadata fetch)
  useEffect(() => {
    setCoverError(false);
  }, [book?.cover_url]);

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

  // Silent refresh: re-fetches book without showing the full-page loading spinner
  const refreshBook = useCallback(async () => {
    if (!id) return;
    try {
      const data = await api.getBook(id);
      setBook(data);
    } catch {
      // Silent refresh - errors are non-critical
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

  // Load segments on mount (after book loads)
  useEffect(() => {
    if (!id || segmentsLoaded) return;
    const loadSegments = async () => {
      try {
        const data = await api.getBookSegments(id);
        setSegments(data);
      } catch {
        // Segments not available, that's fine
      } finally {
        setSegmentsLoaded(true);
      }
    };
    loadSegments();
  }, [id, segmentsLoaded]);

  // Load segment tags when selection changes
  const loadSegmentTags = useCallback(async (segmentId: string) => {
    if (!id) return;
    setSegmentTagsLoading(true);
    try {
      const data = await api.getSegmentTags(id, segmentId);
      setSegmentTags(data);
    } catch {
      setSegmentTags(null);
    } finally {
      setSegmentTagsLoading(false);
    }
  }, [id]);

  useEffect(() => {
    if (isSingleSelect && singleSelectedId) {
      loadSegmentTags(singleSelectedId);
    } else {
      setSegmentTags(null);
    }
  }, [selectedSegmentIds, isSingleSelect, singleSelectedId, loadSegmentTags]);

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
      // Re-fetch enriched book (with populated authors array) and tags
      await refreshBook();
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

  const handleTranscode = async () => {
    if (!book) return;
    setTranscoding(true);
    try {
      await api.startTranscode(book.id);
      toast('Transcode started. The book will be converted to M4B.', 'success');
    } catch (error: unknown) {
      console.error('Failed to start transcode', error);
      const msg =
        error instanceof Error ? error.message : 'Transcode failed to start.';
      toast(msg, 'error');
    } finally {
      setTranscoding(false);
    }
  };

  const handleWriteBackMetadata = async () => {
    if (!book) return;
    setWritingToFiles(true);
    setWriteBackDialogOpen(false);
    try {
      const result = await api.writeBackMetadata(book.id);
      toast(result.message || 'Metadata written to files.', 'success');
    } catch (error: unknown) {
      console.error('Failed to write metadata to files', error);
      const msg =
        error instanceof Error ? error.message : 'Write to files failed.';
      toast(msg, 'error');
    } finally {
      setWritingToFiles(false);
    }
  };

  const handleParseWithAI = async () => {
    if (!book) return;
    setParsingWithAI(true);
    try {
      const result = await api.parseAudiobookWithAI(book.id);
      setBook(result.book);
      toast(result.message || 'AI parsing completed. Use History to revert if needed.', 'success');
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

  const handleEditSave = async (updated: Audiobook, dirtyFields?: Set<string>) => {
    if (!book) return;
    setActionLoading(true);
    setActionLabel('Saving changes...');
    try {
      // Map form field names to API field names for overrides
      const FIELD_TO_API: Record<string, string> = {
        title: 'title',
        author: 'author_name',
        narrator: 'narrator',
        series: 'series_name',
        series_number: 'series_position',
        genre: 'genre',
        year: 'audiobook_release_year',
        language: 'language',
        publisher: 'publisher',
        isbn10: 'isbn10',
        isbn13: 'isbn13',
        description: 'description',
      };

      const payload: Partial<Book> & { overrides?: Record<string, OverridePayload> } = {
        title: updated.title,
        description: updated.description,
        publisher: updated.publisher,
        language: updated.language,
        narrator: updated.narrator,
        series_position: updated.series_number,
        audiobook_release_year:
          updated.audiobook_release_year ||
          updated.year ||
          book.audiobook_release_year,
        print_year: updated.year || book.print_year,
        isbn:
          updated.isbn13 ||
          updated.isbn10 ||
          book.isbn,
        author_name: updated.author,
        series_name: updated.series,
      };

      // Auto-lock edited fields (Plex-style: edits are automatically locked)
      if (dirtyFields && dirtyFields.size > 0) {
        const overrides: Record<string, OverridePayload> = {};
        for (const field of dirtyFields) {
          const apiField = FIELD_TO_API[field];
          if (!apiField) continue;
          const value = field === 'year'
            ? (updated.year ?? updated.audiobook_release_year ?? null)
            : field === 'series_number'
              ? (updated.series_number ?? null)
              : ((updated as unknown as Record<string, unknown>)[field] ?? null);
          overrides[apiField] = { value, locked: true };
        }
        if (Object.keys(overrides).length > 0) {
          payload.overrides = overrides;
        }
      }

      const saved = await api.updateBook(book.id, payload);
      setBook(saved);
      toast('Metadata saved. Edited fields are now locked.', 'success');
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
      toast('Metadata saved to database (overwrite).', 'success');
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
  const coverImageUrl = book.cover_url
    ? book.cover_url.startsWith('/')
      ? book.cover_url
      : `/api/v1/covers/proxy?url=${encodeURIComponent(book.cover_url)}`
    : `/api/v1/audiobooks/${book.id}/cover`;

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
          {coverError ? (
            <Avatar
              sx={{
                bgcolor: 'primary.main',
                width: 120,
                height: 120,
                fontSize: 48,
                borderRadius: 2,
              }}
              variant="rounded"
            >
              {coverLetter}
            </Avatar>
          ) : (
            <Box
              component="img"
              src={coverImageUrl}
              alt={`Cover art for ${book.title || 'Untitled'}`}
              onError={() => setCoverError(true)}
              onClick={() => setCoverLightboxOpen(true)}
              sx={{
                width: 120,
                height: 120,
                objectFit: 'cover',
                borderRadius: 2,
                boxShadow: 2,
                cursor: 'pointer',
                '&:hover': { opacity: 0.85 },
              }}
            />
          )}
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
              {book.authors && book.authors.length > 0
                ? `By ${book.authors.map((a) => a.name).join(' & ')}`
                : book.author_name
                  ? `By ${book.author_name}`
                  : 'Unknown Author'}
            </Typography>
            <Stack direction="row" spacing={1} flexWrap="wrap" sx={{ mt: 0.5 }}>
              <Chip
                icon={<AccessTimeIcon />}
                label={`Created ${formatDateTime(book.created_at)}`}
                variant="outlined"
                size="small"
              />
              <Chip
                icon={<InfoIcon />}
                label={`Updated ${formatDateTime(book.updated_at)}`}
                variant="outlined"
                size="small"
              />
              {book.version_group_id && (
                <Chip
                  icon={<CompareIcon />}
                  label="Version Group Linked"
                  color="secondary"
                  variant="outlined"
                  size="small"
                />
              )}
            </Stack>
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
          Last updated {formatDateTime(book.updated_at)}.
          Restore to keep the book or purge to remove it permanently.
        </Alert>
      )}

      <Paper sx={{ p: 2, mb: 3 }}>
        <Stack
          direction={{ xs: 'column', md: 'row' }}
          spacing={2}
          justifyContent="space-between"
        >
          <Stack direction="row" spacing={1} flexWrap="wrap">
            <Button
              variant="outlined"
              startIcon={<HistoryIcon />}
              onClick={() => setHistoryDialogOpen(true)}
              disabled={actionLoading}
            >
              History
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
              startIcon={<SearchIcon />}
              onClick={() => setMetadataSearchOpen(true)}
              disabled={actionLoading}
            >
              Search Metadata
            </Button>
            {book && book.format?.toLowerCase() !== 'm4b' && (
              <Button
                variant="outlined"
                startIcon={
                  transcoding ? (
                    <CircularProgress size={20} />
                  ) : (
                    <TransformIcon />
                  )
                }
                onClick={handleTranscode}
                disabled={transcoding || actionLoading}
              >
                {transcoding ? 'Converting...' : 'Convert to M4B'}
              </Button>
            )}
            <Button
              variant="outlined"
              startIcon={
                writingToFiles ? (
                  <CircularProgress size={20} />
                ) : (
                  <SaveIcon />
                )
              }
              onClick={() => setWriteBackDialogOpen(true)}
              disabled={writingToFiles || actionLoading}
            >
              {writingToFiles ? 'Writing...' : 'Save to Files'}
            </Button>
            {!isSoftDeleted ? (
              <Button
                variant="contained"
                color="error"
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

      {segments.length > 1 && (
        <Paper sx={{ px: 2, py: 1.5, mb: 1 }}>
          <FileSelector
            segments={segments}
            selectedIds={selectedSegmentIds}
            onToggle={(id) => setSelectedSegmentIds(prev => {
              const next = new Set(prev);
              if (next.has(id)) next.delete(id); else next.add(id);
              return next;
            })}
            onSelectAll={() => setSelectedSegmentIds(new Set(segments.map(s => s.id)))}
            onClearAll={() => setSelectedSegmentIds(new Set())}
          />
        </Paper>
      )}

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
          {singleSelectedId && segmentTags ? (
            <>
              {segmentTagsLoading && <LinearProgress sx={{ mb: 2 }} />}
              <Typography variant="subtitle2" color="text.secondary" gutterBottom>
                File-specific info for: {segmentTags.file_path.split('/').pop()}
              </Typography>
              <Grid container spacing={2}>
                {[
                  { label: 'Filename', value: segmentTags.file_path.split('/').pop() },
                  { label: 'Format', value: segmentTags.format?.toUpperCase() },
                  { label: 'Duration', value: formatDuration(segmentTags.duration_sec) },
                  {
                    label: 'Size',
                    value: segmentTags.size_bytes > 0
                      ? `${(segmentTags.size_bytes / 1048576).toFixed(1)} MB`
                      : undefined,
                  },
                  {
                    label: 'Track Number',
                    value: segmentTags.track_number != null
                      ? `${segmentTags.track_number}${segmentTags.total_tracks ? ` of ${segmentTags.total_tracks}` : ''}`
                      : undefined,
                  },
                  { label: 'Codec', value: segmentTags.tags?.codec },
                  { label: 'Bitrate', value: segmentTags.tags?.bitrate ? `${segmentTags.tags.bitrate} kbps` : undefined },
                  { label: 'Sample Rate', value: segmentTags.tags?.sample_rate ? `${segmentTags.tags.sample_rate} Hz` : undefined },
                ]
                  .filter((item) => item.value !== undefined && item.value !== '' && item.value !== null && item.value !== '\u2014')
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
              {segmentTags.tags_read_error && (
                <Alert severity="warning" sx={{ mt: 2 }}>
                  Tag read error: {segmentTags.tags_read_error}
                </Alert>
              )}
              {segmentTags.used_filename_fallback && (
                <Alert severity="info" sx={{ mt: 2 }}>
                  Some metadata was extracted from the filename because embedded tags were incomplete.
                </Alert>
              )}
            </>
          ) : (
            <>
              {singleSelectedId && segmentTagsLoading && <LinearProgress sx={{ mb: 2 }} />}
              <Grid container spacing={2}>
                {(() => {
                  const authorVal = book.authors && book.authors.length > 0
                    ? book.authors.map((a) => a.name).join(' & ')
                    : book.author_name || '';
                  const narratorVal = book.narrators && book.narrators.length > 0
                    ? book.narrators.map((n) => n.name).join(' & ')
                    : book.narrator || '';
                  // Core fields always shown
                  const coreFields = [
                    { label: 'Title', value: book.title || '' },
                    { label: 'Author', value: authorVal },
                    { label: 'Narrator', value: narratorVal },
                    { label: 'Language', value: book.language || '' },
                    { label: 'Series', value: book.series_name ? `${book.series_name}${book.series_position ? ` #${book.series_position}` : ''}` : '' },
                  ];
                  // Dynamic fields: only shown when set
                  const dynamicFields = [
                    { label: 'Publisher', value: book.publisher },
                    { label: 'Release Year', value: book.audiobook_release_year ? String(book.audiobook_release_year) : undefined },
                    { label: 'Print Year', value: book.print_year ? String(book.print_year) : undefined },
                    { label: 'ISBN 13', value: book.isbn13 },
                    { label: 'ISBN 10', value: book.isbn10 },
                    { label: 'Genre', value: book.quality },
                    { label: 'Format', value: book.format?.toUpperCase() },
                    { label: 'Codec', value: book.codec },
                    { label: 'Bitrate', value: book.bitrate ? `${book.bitrate} kbps` : undefined },
                    { label: 'Duration', value: book.duration ? formatDuration(book.duration) : undefined },
                    { label: 'Edition', value: book.edition },
                    { label: 'Work ID', value: book.work_id },
                  ].filter((item) => item.value !== undefined && item.value !== '' && item.value !== null);
                  return [...coreFields, ...dynamicFields].map((item) => (
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
                          sx={{ textTransform: 'uppercase', letterSpacing: 0.5 }}
                        >
                          {item.label}
                        </Typography>
                        <Typography variant="body1" sx={{ color: item.value ? 'text.primary' : 'text.disabled' }}>
                          {item.value || '\u2014'}
                        </Typography>
                      </Box>
                    </Grid>
                  ));
                })()}
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
            </>
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
                <Stack direction="row" alignItems="center" spacing={2} sx={{ mt: 2, mb: 1 }}>
                  <Typography variant="h6">Individual Files</Typography>
                  {segments.length > 1 && (
                    <Button
                      size="small"
                      variant="outlined"
                      onClick={async () => {
                        try {
                          const result = await api.extractTrackInfo(book.id);
                          toast(`Updated track numbers for ${result.updated} of ${result.total} files`, 'success');
                          if (id) {
                            const segs = await api.getBookSegments(id);
                            setSegments(segs);
                          }
                        } catch (err) {
                          toast('Failed to extract track info', 'error');
                        }
                      }}
                    >
                      Auto-fill Track Numbers
                    </Button>
                  )}
                </Stack>
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
                      <TableRow
                        key={seg.id}
                        hover
                        sx={{ cursor: 'pointer' }}
                        onClick={() => {
                          setSelectedSegmentIds(new Set([seg.id]));
                          setActiveTab('info');
                        }}
                      >
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
                  }}
                >
                  <Stack
                    direction={{ xs: 'column', md: 'row' }}
                    spacing={1}
                    alignItems="center"
                  >
                    <Tooltip title={version.is_primary_version ? 'Primary version' : 'Set as primary'}>
                      <IconButton
                        size="small"
                        color={version.is_primary_version ? 'primary' : 'default'}
                        onClick={async () => {
                          try {
                            await api.setPrimaryVersion(version.id);
                            loadVersions();
                            loadBook();
                          } catch {
                            toast('Failed to set primary version', 'error');
                          }
                        }}
                      >
                        {version.is_primary_version ? <StarIcon /> : <StarBorderIcon />}
                      </IconButton>
                    </Tooltip>
                    <Box
                      flex={1}
                      sx={{ cursor: version.id === book.id ? 'default' : 'pointer' }}
                      onClick={() => {
                        if (version.id !== book.id) {
                          navigate(`/library/${version.id}`);
                        }
                      }}
                    >
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
                    <Stack direction="row" spacing={1} flexWrap="wrap" alignItems="center">
                      {version.is_primary_version && (
                        <Chip label="Primary" color="primary" size="small" />
                      )}
                      {version.quality && (
                        <Chip label={version.quality} color="success" size="small" />
                      )}
                      {version.codec && (
                        <Chip label={version.codec} variant="outlined" size="small" />
                      )}
                      {version.format && (
                        <Chip
                          label={version.format.toUpperCase()}
                          variant="outlined"
                          size="small"
                        />
                      )}
                      {version.id !== book.id && (
                        <Tooltip title="Unlink this version">
                          <IconButton
                            size="small"
                            color="secondary"
                            onClick={async () => {
                              try {
                                await api.unlinkBookVersion(book.id, version.id);
                                loadVersions();
                                loadBook();
                                toast('Version unlinked', 'success');
                              } catch {
                                toast('Failed to unlink version', 'error');
                              }
                            }}
                          >
                            <LinkOffIcon fontSize="small" />
                          </IconButton>
                        </Tooltip>
                      )}
                    </Stack>
                  </Stack>
                </Box>
              ))}
            </Stack>
          )}
          <Button
            variant="outlined"
            startIcon={<LinkIcon />}
            sx={{ mt: 2 }}
            onClick={() => setVersionDialogOpen(true)}
          >
            Link Another Version
          </Button>
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
          <Typography variant="caption" color="text.secondary" sx={{ ml: 4, display: 'block', mt: -0.5, mb: 1 }}>
            Block hash prevents this exact file from being re-imported by remembering its unique fingerprint.
          </Typography>
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

      {/* Write-back confirmation dialog */}
      <Dialog
        open={writeBackDialogOpen}
        onClose={() => setWriteBackDialogOpen(false)}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>Save Metadata to Files</DialogTitle>
        <DialogContent>
          <Typography variant="body1" gutterBottom>
            This will write the following metadata from the database directly
            into the audio file tags on disk:
          </Typography>
          <Box component="ul" sx={{ mt: 1 }}>
            <li>
              <strong>Title</strong> — {book?.title}
            </li>
            <li>
              <strong>Album</strong> — {book?.title} (groups tracks in players)
            </li>
            <li>
              <strong>Artist</strong> —{' '}
              {book?.authors?.map((a) => a.name).join(' & ') ||
                book?.author_name ||
                '(none)'}
            </li>
            <li>
              <strong>Narrator</strong> —{' '}
              {book?.narrators?.map((n) => n.name).join(' & ') ||
                book?.narrator ||
                '(none)'}
            </li>
            <li>
              <strong>Year</strong> —{' '}
              {book?.audiobook_release_year || book?.print_year || '(none)'}
            </li>
            <li>
              <strong>Genre</strong> — Audiobook
            </li>
            <li>
              <strong>Track numbers</strong> — written for multi-file books
            </li>
          </Box>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 2 }}>
            A backup of each file is created before writing and removed on
            success. The original file is restored automatically if writing
            fails.
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setWriteBackDialogOpen(false)}>Cancel</Button>
          <Button
            variant="contained"
            startIcon={<SaveIcon />}
            onClick={handleWriteBackMetadata}
          >
            Write to Files
          </Button>
        </DialogActions>
      </Dialog>

      <Dialog open={purgeDialogOpen} onClose={() => { setPurgeDialogOpen(false); setPurgeConfirmed(false); }}>
        <DialogTitle>Purge Audiobook</DialogTitle>
        <DialogContent dividers>
          <Alert severity="error" sx={{ mb: 2 }}>
            This will permanently delete this audiobook. This cannot be undone.
          </Alert>
          <Typography gutterBottom>
            Are you sure you want to purge{' '}
            <strong>{book.title || 'this audiobook'}</strong> from the library?
            All associated files and metadata will be removed.
          </Typography>
          {book.marked_for_deletion_at && (
            <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
              Soft deleted on {formatDateTime(book.marked_for_deletion_at)}.
            </Typography>
          )}
          <FormControlLabel
            control={
              <Checkbox
                checked={purgeConfirmed}
                onChange={(e) => setPurgeConfirmed(e.target.checked)}
              />
            }
            label="I understand this action is permanent and cannot be undone"
            sx={{ mt: 2 }}
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => { setPurgeDialogOpen(false); setPurgeConfirmed(false); }}>Cancel</Button>
          <Button
            onClick={handlePurge}
            color="error"
            variant="contained"
            disabled={actionLoading || !purgeConfirmed}
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

      <MetadataHistory
        bookId={book.id}
        open={historyDialogOpen}
        onClose={() => setHistoryDialogOpen(false)}
        onUndoComplete={() => {
          loadBook();
        }}
      />
      <MetadataSearchDialog
        open={metadataSearchOpen}
        book={book}
        onClose={() => setMetadataSearchOpen(false)}
        onApplied={(updatedBook) => {
          setBook(updatedBook);
        }}
        toast={toast}
      />

      {/* Cover image lightbox */}
      <Dialog
        open={coverLightboxOpen}
        onClose={() => setCoverLightboxOpen(false)}
        maxWidth="sm"
      >
        <DialogContent sx={{ p: 1 }}>
          <Box
            component="img"
            src={coverImageUrl}
            alt={`Cover art for ${book.title || 'Untitled'}`}
            sx={{ width: '100%', maxWidth: 600, display: 'block' }}
          />
        </DialogContent>
      </Dialog>
    </Box>
  );
};
