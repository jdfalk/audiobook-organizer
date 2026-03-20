// file: web/src/pages/BookDetail.tsx
// version: 1.37.0
// guid: 4d2f7c6a-1b3e-4c5d-8f7a-9b0c1d2e3f4a

import { useCallback, useEffect, useState } from 'react';
import { useNavigate, useParams, useSearchParams } from 'react-router-dom';
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
  TextField,
  Tooltip,
  Collapse,
  IconButton,
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
// StorageIcon removed — not used in Sonarr-style layout
import SaveIcon from '@mui/icons-material/Save.js';
import SearchIcon from '@mui/icons-material/Search.js';
import TransformIcon from '@mui/icons-material/Transform.js';
import LinkIcon from '@mui/icons-material/Link.js';
import LinkOffIcon from '@mui/icons-material/LinkOff.js';
import FolderOpenIcon from '@mui/icons-material/FolderOpen.js';
import ErrorOutlineIcon from '@mui/icons-material/ErrorOutline.js';
import DriveFileRenameOutlineIcon from '@mui/icons-material/DriveFileRenameOutline.js';
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown.js';
import KeyboardArrowUpIcon from '@mui/icons-material/KeyboardArrowUp.js';
import StarIcon from '@mui/icons-material/Star.js';
import StarBorderIcon from '@mui/icons-material/StarBorder.js';
import type { Book, BookSegment, BookTags, SegmentTags, OverridePayload, RenamePreview } from '../services/api';
import * as api from '../services/api';
import { MetadataEditDialog } from '../components/audiobooks/MetadataEditDialog';
import { MetadataHistory } from '../components/MetadataHistory';
import { MetadataSearchDialog } from '../components/audiobooks/MetadataSearchDialog';
import { RelocateFileDialog } from '../components/audiobooks/RelocateFileDialog';
import { TagComparison } from '../components/TagComparison';
import { ChangeLog } from '../components/ChangeLog';
import { useToast } from '../components/toast/ToastProvider';
import type { Audiobook } from '../types';

const SEGMENT_PREVIEW_COUNT = 5;
const OVERALL_METADATA_FIELDS = [
  { key: 'title', label: 'Title' },
  { key: 'author_name', label: 'Author' },
  { key: 'narrator', label: 'Narrator' },
  { key: 'series_name', label: 'Series' },
  { key: 'publisher', label: 'Publisher' },
  { key: 'language', label: 'Language' },
  { key: 'isbn13', label: 'ISBN' },
] as const;

export const BookDetail = () => {
  const { id } = useParams();
  const navigate = useNavigate();
  const [book, setBook] = useState<Book | null>(null);
  const [loading, setLoading] = useState(true);
  const [actionLoading, setActionLoading] = useState(false);
  const [actionLabel, setActionLabel] = useState<string | null>(null);
  const [fetchingMetadata, setFetchingMetadata] = useState(false);
  const [parsingWithAI, setParsingWithAI] = useState(false);
  const [rescanningFolder, setRescanningFolder] = useState(false);
  const [writingToFiles, setWritingToFiles] = useState(false);
  const [transcoding, setTranscoding] = useState(false);
  const [writeBackDialogOpen, setWriteBackDialogOpen] = useState(false);
  // preAIBook removed — use History to revert AI changes
  const { toast } = useToast();
  const [searchParams, setSearchParams] = useSearchParams();
  const activeTab = (['info', 'files'].includes(searchParams.get('tab') || '')
    ? searchParams.get('tab')
    : 'info') as 'info' | 'files';
  const setActiveTab = (v: 'info' | 'files') => {
    setSearchParams((prev) => { const next = new URLSearchParams(prev); next.set('tab', v); return next; }, { replace: true });
  };
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
  const [itunesLinked, setItunesLinked] = useState(false);
  const [filesRefreshKey, setFilesRefreshKey] = useState(0);
  const refreshFilesTab = () => setFilesRefreshKey((k) => k + 1);
  const [compareSnapshotTs, setCompareSnapshotTs] = useState<string | null>(null);
  const [itunesPidCount, setItunesPidCount] = useState(0);
  const [linkSearchOpen, setLinkSearchOpen] = useState(false);
  const [linkSearchQuery, setLinkSearchQuery] = useState('');
  const [linkSearchResults, setLinkSearchResults] = useState<Book[]>([]);
  const [linkSearchLoading, setLinkSearchLoading] = useState(false);
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [historyDialogOpen, setHistoryDialogOpen] = useState(false);
  const [metadataSearchOpen, setMetadataSearchOpen] = useState(false);
  const [segments, setSegments] = useState<BookSegment[]>([]);
  const [segmentsLoaded, setSegmentsLoaded] = useState(false);
  const [relocateSegment, setRelocateSegment] = useState<BookSegment | null>(null);
  const [selectedSegmentIds, setSelectedSegmentIds] = useState<Set<string>>(new Set());
  const [segmentTags, setSegmentTags] = useState<SegmentTags | null>(null);
  const [segmentTagsLoading, setSegmentTagsLoading] = useState(false);
  const [coverError, setCoverError] = useState(false);
  const [coverLightboxOpen, setCoverLightboxOpen] = useState(false);
  const [splittingVersion, setSplittingVersion] = useState(false);
  const [splittingToBooks, setSplittingToBooks] = useState(false);
  // moveToVersionDialogOpen removed — "Move to Existing Version" is now inline
  const [renamePreviewDialogOpen, setRenamePreviewDialogOpen] = useState(false);
  const [renamePreview, setRenamePreview] = useState<RenamePreview | null>(null);
  const [renamePreviewLoading, setRenamePreviewLoading] = useState(false);
  const [applyingRename, setApplyingRename] = useState(false);
  // Sonarr-style version expansion
  const [expandedVersionIds, setExpandedVersionIds] = useState<Set<string>>(new Set());
  const [expandedSegmentVersionIds, setExpandedSegmentVersionIds] = useState<Set<string>>(new Set());
  const [versionSegments, setVersionSegments] = useState<Record<string, BookSegment[]>>({});
  const [versionFileTags, setVersionFileTags] = useState<Record<string, BookTags | null>>({});
  // versionFileTagsLoading removed — TagComparison handles its own loading
  const [, setVersionFileTagsLoading] = useState<Set<string>>(new Set());

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
    try {
      const data = await api.getBookVersions(id);
      setVersions(data);
    } catch (error) {
      console.error('Failed to load versions', error);
    }
  }, [id]);


  useEffect(() => {
    loadBook();
    loadVersions();
    // Load external ID info (iTunes linkage)
    api.getBookExternalIDs(id!).then((data) => {
      setItunesLinked(data.itunes_linked);
      setItunesPidCount(data.total);
    }).catch(() => {});
  }, [id, loadBook, loadVersions]);

  // Inline version link search
  useEffect(() => {
    if (!linkSearchOpen) {
      setLinkSearchQuery('');
      setLinkSearchResults([]);
      return;
    }
    const q = linkSearchQuery.trim();
    if (!q) { setLinkSearchResults([]); return; }
    let cancelled = false;
    setLinkSearchLoading(true);
    const timer = window.setTimeout(async () => {
      try {
        const results = await api.searchBooks(q, 10);
        if (!cancelled) setLinkSearchResults(results.filter(r => r.id !== book?.id && !versions.some(v => v.id === r.id)));
      } catch { /* ignore */ }
      finally { if (!cancelled) setLinkSearchLoading(false); }
    }, 300);
    return () => { cancelled = true; window.clearTimeout(timer); };
  }, [linkSearchOpen, linkSearchQuery, book?.id, versions]);

  const handleInlineLinkVersion = async (targetId: string) => {
    if (!book) return;
    try {
      await api.linkBookVersion(book.id, targetId);
      setLinkSearchOpen(false);
      loadVersions();
      loadBook();
      toast('Version linked', 'success');
    } catch {
      toast('Failed to link version', 'error');
    }
  };

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
    const parts: string[] = [];
    if (hours > 0) parts.push(`${hours}h`);
    if (minutes > 0 || hours > 0) parts.push(`${minutes}m`);
    if (remainingSeconds > 0 && hours === 0) parts.push(`${remainingSeconds}s`);
    return parts.join(' ');
  };

  const formatBytes = (bytes?: number) => {
    if (!bytes || bytes <= 0) return '—';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    let value = bytes;
    let unitIndex = 0;
    while (value >= 1024 && unitIndex < units.length - 1) {
      value /= 1024;
      unitIndex += 1;
    }
    const decimals = value >= 10 || unitIndex === 0 ? 0 : 1;
    return `${value.toFixed(decimals)} ${units[unitIndex]}`;
  };

  const formatTagValue = (value?: string | number | boolean | null) => {
    if (value == null || value === '') return '—';
    return String(value);
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
      refreshFilesTab(); // reload tags and changelog
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
      refreshFilesTab(); // reload tags and changelog after write
    } catch (error: unknown) {
      console.error('Failed to write metadata to files', error);
      const msg =
        error instanceof Error ? error.message : 'Write to files failed.';
      toast(msg, 'error');
    } finally {
      setWritingToFiles(false);
    }
  };

  const handlePreviewRename = async () => {
    if (!book) return;
    setRenamePreviewLoading(true);
    try {
      const preview = await api.previewRename(book.id);
      setRenamePreview(preview);
      setRenamePreviewDialogOpen(true);
    } catch (error: unknown) {
      console.error('Failed to preview rename', error);
      const msg =
        error instanceof Error ? error.message : 'Preview rename failed.';
      toast(msg, 'error');
    } finally {
      setRenamePreviewLoading(false);
    }
  };

  const handleApplyRename = async () => {
    if (!book) return;
    setApplyingRename(true);
    setRenamePreviewDialogOpen(false);
    try {
      const result = await api.applyRename(book.id);
      toast(result.message || 'Rename applied successfully.', 'success');
      await refreshBook();
    } catch (error: unknown) {
      console.error('Failed to apply rename', error);
      const msg =
        error instanceof Error ? error.message : 'Apply rename failed.';
      toast(msg, 'error');
    } finally {
      setApplyingRename(false);
    }
  };

  const handleSplitVersion = async () => {
    if (!book || selectedSegmentIds.size === 0) return;
    setSplittingVersion(true);
    try {
      const newBook = await api.splitVersion(book.id, Array.from(selectedSegmentIds));
      toast(`Created new version: ${newBook.title}`, 'success');
      setSelectedSegmentIds(new Set());
      loadBook();
      loadVersions();
      // Reload segments since some moved
      const segs = await api.getBookSegments(book.id);
      setSegments(segs);
    } catch (error: unknown) {
      const msg = error instanceof Error ? error.message : 'Failed to split version';
      toast(msg, 'error');
    } finally {
      setSplittingVersion(false);
    }
  };

  const handleSplitToBooks = async () => {
    if (!book || selectedSegmentIds.size === 0) return;
    setSplittingToBooks(true);
    try {
      const result = await api.splitSegmentsToBooks(book.id, Array.from(selectedSegmentIds));
      toast(`Created ${result.count} new book${result.count !== 1 ? 's' : ''}`, 'success');
      setSelectedSegmentIds(new Set());
      loadBook();
      const segs = await api.getBookSegments(book.id);
      setSegments(segs);
    } catch (error: unknown) {
      const msg = error instanceof Error ? error.message : 'Failed to split to books';
      toast(msg, 'error');
    } finally {
      setSplittingToBooks(false);
    }
  };

  const handleMoveToVersion = async (targetBookId: string) => {
    if (!book || selectedSegmentIds.size === 0) return;
    try {
      await api.moveSegments(book.id, Array.from(selectedSegmentIds), targetBookId);
      toast('Segments moved to version', 'success');
      setSelectedSegmentIds(new Set());
      // Dialog removed — move-to-version is now inline
      loadBook();
      loadVersions();
      const segs = await api.getBookSegments(book.id);
      setSegments(segs);
    } catch (error: unknown) {
      const msg = error instanceof Error ? error.message : 'Failed to move segments';
      toast(msg, 'error');
    }
  };

  // Auto-expand current book's version when versions load
  useEffect(() => {
    if (id && versions.length > 0) {
      setExpandedVersionIds(new Set([id]));
    }
  }, [id, versions.length]);

  const toggleVersionExpanded = useCallback(async (versionId: string) => {
    setExpandedVersionIds(prev => {
      const next = new Set(prev);
      if (next.has(versionId)) {
        next.delete(versionId);
      } else {
        next.add(versionId);
      }
      return next;
    });
    // Load segments for this version if not already loaded
    if (!versionSegments[versionId]) {
      try {
        const segs = await api.getBookSegments(versionId);
        setVersionSegments(prev => ({ ...prev, [versionId]: segs }));
      } catch {
        setVersionSegments(prev => ({ ...prev, [versionId]: [] }));
      }
    }
    // Load file tags if not already loaded
    if (versionFileTags[versionId] === undefined) {
      setVersionFileTagsLoading(prev => new Set(prev).add(versionId));
      try {
        const tags = await api.getBookTags(versionId);
        setVersionFileTags(prev => ({ ...prev, [versionId]: tags }));
      } catch {
        setVersionFileTags(prev => ({ ...prev, [versionId]: null }));
      } finally {
        setVersionFileTagsLoading(prev => {
          const next = new Set(prev);
          next.delete(versionId);
          return next;
        });
      }
    }
  }, [versionSegments, versionFileTags]);

  // Keep current book's segments synced
  useEffect(() => {
    if (id && segments.length > 0) {
      setVersionSegments(prev => ({ ...prev, [id]: segments }));
    }
  }, [id, segments]);

  // Preload current-book tag data so multi-file summaries can render immediately.
  useEffect(() => {
    if (!id || versionFileTags[id] !== undefined) return;
    let cancelled = false;
    setVersionFileTagsLoading(prev => new Set(prev).add(id));
    api.getBookTags(id)
      .then((tags) => {
        if (!cancelled) {
          setVersionFileTags(prev => ({ ...prev, [id]: tags }));
        }
      })
      .catch(() => {
        if (!cancelled) {
          setVersionFileTags(prev => ({ ...prev, [id]: null }));
        }
      })
      .finally(() => {
        if (!cancelled) {
          setVersionFileTagsLoading(prev => {
            const next = new Set(prev);
            next.delete(id);
            return next;
          });
        }
      });
    return () => {
      cancelled = true;
    };
  }, [id, versionFileTags]);

  const handleSetPrimary = async (versionId: string) => {
    try {
      await api.setPrimaryVersion(versionId);
      toast('Primary version updated', 'success');
      loadVersions();
      loadBook();
    } catch {
      toast('Failed to set primary version', 'error');
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

  const handleRescanFolder = async () => {
    if (!book) return;
    setRescanningFolder(true);
    try {
      // Get parent directory from file_path
      const lastSlash = book.file_path.lastIndexOf('/');
      const folderPath = lastSlash > 0 ? book.file_path.substring(0, lastSlash) : book.file_path;
      await api.startScan(folderPath, undefined, true);
      toast(`Scanning folder: ${folderPath}`, 'success');
      // Refresh book and segments after a short delay to let scan complete
      setTimeout(async () => {
        await refreshBook();
      }, 3000);
    } catch (error: unknown) {
      console.error('Failed to rescan folder', error);
      const msg =
        error instanceof Error ? error.message : 'Folder rescan failed.';
      toast(msg, 'error');
    } finally {
      setRescanningFolder(false);
    }
  };

  // versionSummary removed — tab label is now static "Files & History"

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

      // Use ?? '' to ensure all fields are present in JSON (undefined is dropped by JSON.stringify)
      const payload: Partial<Book> & { overrides?: Record<string, OverridePayload> } = {
        title: updated.title ?? '',
        description: updated.description ?? '',
        publisher: updated.publisher ?? '',
        language: updated.language ?? '',
        narrator: updated.narrator ?? '',
        series_position: updated.series_number ?? undefined,
        audiobook_release_year:
          updated.audiobook_release_year ||
          updated.year ||
          book.audiobook_release_year ||
          undefined,
        print_year: updated.year || book.print_year || undefined,
        isbn:
          updated.isbn13 ||
          updated.isbn10 ||
          book.isbn ||
          '',
        author_name: updated.author ?? '',
        series_name: updated.series ?? '',
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
          onClick={() => {
            const returnUrl = sessionStorage.getItem('library_return_url');
            if (returnUrl) navigate(returnUrl);
            else navigate('/library');
          }}
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
          onClick={() => {
            const returnUrl = sessionStorage.getItem('library_return_url');
            if (returnUrl) navigate(returnUrl);
            else navigate('/library');
          }}
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
              {itunesLinked && (
                <Chip
                  label={`iTunes Linked (${itunesPidCount} PID${itunesPidCount !== 1 ? 's' : ''})`}
                  color="info"
                  variant="outlined"
                  size="small"
                  clickable
                  onClick={() => setActiveTab('files')}
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

      {book.file_exists === false && (
        <Alert severity="error" sx={{ mb: 2 }}>
          File missing: The audio file at <strong>{book.file_path}</strong> could not be found on disk.
          The file may have been moved, renamed, or deleted.
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
            <Button
              variant="outlined"
              startIcon={
                rescanningFolder ? (
                  <CircularProgress size={20} />
                ) : (
                  <FolderOpenIcon />
                )
              }
              onClick={handleRescanFolder}
              disabled={rescanningFolder || actionLoading}
            >
              {rescanningFolder ? 'Scanning...' : 'Rescan Folder'}
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
                renamePreviewLoading || applyingRename ? (
                  <CircularProgress size={20} />
                ) : (
                  <DriveFileRenameOutlineIcon />
                )
              }
              onClick={handlePreviewRename}
              disabled={renamePreviewLoading || applyingRename || actionLoading}
            >
              {applyingRename ? 'Renaming...' : renamePreviewLoading ? 'Loading...' : 'Preview Rename'}
            </Button>
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

      {/* File selector chip bar removed — checkboxes are now in the files table */}

      <Paper sx={{ p: 2, mb: 3 }}>
        <Tabs
          value={activeTab}
          onChange={(_, value) => setActiveTab(value)}
          textColor="primary"
          indicatorColor="primary"
          variant="scrollable"
        >
          <Tab label="Info" value="info" />
          <Tab
            label="Files &amp; History"
            value="files"
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
                    value: formatBytes(segmentTags.size_bytes),
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
                    { label: 'Edition', value: book.edition && book.edition !== '0' && book.edition.length <= 50 ? book.edition : undefined },
                    { label: 'Description', value: book.description || (book.edition && book.edition.length > 50 ? book.edition : undefined) },
                    { label: 'Work ID', value: book.work_id },
                  ].filter((item) => item.value !== undefined && item.value !== '' && item.value !== null);
                  return [...coreFields, ...dynamicFields].map((item) => (
                    <Grid item xs={12} sm={item.label === 'Description' ? 12 : 6} md={item.label === 'Description' ? 12 : 4} key={item.label}>
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
            </>
          )}
        </Paper>
      )}

      {activeTab === 'files' && (() => {
        // Group versions by format
        const allVersions = versions.length > 0 ? versions : [book];
        const formatGroups = new Map<string, Book[]>();
        for (const v of allVersions) {
          const key = v.format?.toUpperCase() || 'UNKNOWN';
          if (!formatGroups.has(key)) formatGroups.set(key, []);
          formatGroups.get(key)!.push(v);
        }

        return (
        <Stack spacing={0}>
          {/* Link another version — above format trays */}
          <Box sx={{ mb: 1 }}>
            {!linkSearchOpen ? (
              <Button size="small" variant="text" startIcon={<LinkIcon />}
                onClick={() => setLinkSearchOpen(true)}>
                Link Another Version
              </Button>
            ) : (
              <Stack spacing={1} sx={{ maxWidth: 400 }}>
                <Stack direction="row" alignItems="center" justifyContent="space-between">
                  <Typography variant="caption">Search for a book to link as a version</Typography>
                  <Button size="small" onClick={() => setLinkSearchOpen(false)}>Cancel</Button>
                </Stack>
                <TextField size="small" autoFocus placeholder="Search by title or author..."
                  value={linkSearchQuery} onChange={(e) => setLinkSearchQuery(e.target.value)} fullWidth />
                {linkSearchLoading && <CircularProgress size={16} />}
                {linkSearchResults.map((result) => (
                  <Button key={result.id} variant="outlined" size="small"
                    sx={{ justifyContent: 'flex-start', textTransform: 'none' }}
                    onClick={() => handleInlineLinkVersion(result.id)}>
                    {result.title} — {result.author_name}
                  </Button>
                ))}
              </Stack>
            )}
          </Box>

          {/* Format group sections */}
          {Array.from(formatGroups.entries()).map(([formatKey, groupVersions]) => {
            // Use first version in group as the representative
            const representative = groupVersions[0];
            const isExpanded = groupVersions.some((v) => expandedVersionIds.has(v.id));
            const groupId = groupVersions.map((v) => v.id).join('-');
            const hasPrimary = groupVersions.some((v) => v.is_primary_version);
            const hasItunes = groupVersions.some((v) => v.itunes_persistent_id);
            const totalFiles = groupVersions.reduce((sum, v) => {
              const segs = v.id === book.id ? segments : (versionSegments[v.id] || []);
              return sum + (segs.length || 1);
            }, 0);
            const totalSize = groupVersions.reduce((sum, v) => sum + (v.file_size || 0), 0);
            const totalDuration = groupVersions.reduce((sum, v) => sum + (v.duration || 0), 0);

            return (
              <Paper key={groupId} sx={{ mb: 1, overflow: 'hidden' }} data-testid={`format-tray-${formatKey.toLowerCase()}`}>
                {/* Format tray header */}
                <Box
                  sx={{
                    display: 'flex',
                    alignItems: 'center',
                    px: 2,
                    py: 1,
                    bgcolor: hasPrimary ? 'primary.dark' : 'background.paper',
                    cursor: 'pointer',
                    '&:hover': { bgcolor: hasPrimary ? 'primary.dark' : 'action.hover' },
                    borderBottom: isExpanded ? '1px solid' : 'none',
                    borderColor: 'divider',
                  }}
                  onClick={() => {
                    // Toggle all versions in this format group
                    for (const v of groupVersions) {
                      toggleVersionExpanded(v.id);
                    }
                  }}
                >
                  <IconButton size="small" sx={{ mr: 1, color: 'inherit' }}>
                    {isExpanded ? <KeyboardArrowUpIcon /> : <KeyboardArrowDownIcon />}
                  </IconButton>
                  {hasPrimary ? (
                    <StarIcon fontSize="small" sx={{ mr: 1, color: 'warning.main' }} />
                  ) : (
                    <StarBorderIcon fontSize="small" sx={{ mr: 1, opacity: 0.4 }} />
                  )}
                  <Typography variant="subtitle1" fontWeight="bold" sx={{ flex: 1, minWidth: 0 }} noWrap>
                    {formatKey}{representative.codec ? ` (${representative.codec})` : ''}
                  </Typography>
                  <Stack direction="row" spacing={1} alignItems="center" sx={{ ml: 2, flexShrink: 0 }}>
                    <Chip
                      label={`${totalFiles} file${totalFiles !== 1 ? 's' : ''}`}
                      size="small"
                      variant="outlined"
                    />
                    {totalSize > 0 && (
                      <Typography variant="body2" color="text.secondary">
                        {formatBytes(totalSize)}
                      </Typography>
                    )}
                    {totalDuration > 0 && (
                      <Typography variant="body2" color="text.secondary" sx={{ minWidth: 50 }}>
                        {formatDuration(totalDuration)}
                      </Typography>
                    )}
                    {hasPrimary && (
                      <Chip label="Primary" size="small" color="warning" />
                    )}
                    {hasItunes && (
                      <Chip label="iTunes" size="small" color="info" variant="outlined" />
                    )}
                  </Stack>
                </Box>

                {/* Expanded content for each version in this format group */}
                <Collapse in={isExpanded}>
                  {groupVersions.map((version) => {
                    const isCurrent = version.id === book.id;
                    const isPrimary = version.is_primary_version;
                    const vSegs = isCurrent ? segments : (versionSegments[version.id] || []);
                    const showAllSegments = expandedSegmentVersionIds.has(version.id);
                    const visibleSegments = showAllSegments
                      ? vSegs
                      : vSegs.slice(0, SEGMENT_PREVIEW_COUNT);
                    const hiddenSegmentCount = Math.max(vSegs.length - visibleSegments.length, 0);
                    const metadataEntries = OVERALL_METADATA_FIELDS
                      .map(({ key, label }) => {
                        const tag = versionFileTags[version.id]?.tags?.[key];
                        if (!tag) return null;
                        const fileValue = formatTagValue(tag.file_value);
                        const storedValue = formatTagValue(tag.stored_value);
                        if (fileValue === '—' && storedValue === '—') return null;
                        return {
                          key,
                          label,
                          fileValue,
                          storedValue,
                          differsFromDb:
                            fileValue !== '—' &&
                            storedValue !== '—' &&
                            fileValue !== storedValue,
                        };
                      })
                      .filter((entry): entry is NonNullable<typeof entry> => entry !== null);

                    return (
                      <Box key={version.id} sx={{ p: 2, borderBottom: groupVersions.length > 1 ? '1px solid' : 'none', borderColor: 'divider' }}>
                        {/* Version action buttons */}
                        <Stack direction="row" spacing={1} sx={{ mb: 2 }} flexWrap="wrap" useFlexGap>
                          {!isPrimary && versions.length > 1 && (
                            <Button
                              size="small"
                              variant="outlined"
                              startIcon={<StarIcon />}
                              onClick={(e) => { e.stopPropagation(); handleSetPrimary(version.id); }}
                            >
                              Set as Primary
                            </Button>
                          )}
                          {!isCurrent && (
                            <Button
                              size="small"
                              variant="outlined"
                              onClick={(e) => { e.stopPropagation(); navigate(`/library/${version.id}`); }}
                            >
                              View Details
                            </Button>
                          )}
                          {!isCurrent && (
                            <Button
                              size="small"
                              variant="outlined"
                              color="error"
                              startIcon={<LinkOffIcon />}
                              onClick={async (e) => {
                                e.stopPropagation();
                                try {
                                  await api.unlinkBookVersion(book.id, version.id);
                                  loadVersions(); loadBook();
                                  toast('Version unlinked', 'success');
                                } catch { toast('Failed to unlink version', 'error'); }
                              }}
                            >
                              Unlink
                            </Button>
                          )}
                        </Stack>

                        {/* Path and codec info */}
                        <Table size="small" sx={{ mb: 2 }}>
                          <TableBody>
                            <TableRow>
                              <TableCell sx={{ fontWeight: 'bold', width: 140, color: 'text.secondary' }}>Path</TableCell>
                              <TableCell sx={{ wordBreak: 'break-all', fontSize: '0.85rem' }}>{version.file_path}</TableCell>
                            </TableRow>
                            {version.bitrate && (
                              <TableRow>
                                <TableCell sx={{ fontWeight: 'bold', color: 'text.secondary' }}>Bitrate</TableCell>
                                <TableCell>{version.bitrate} kbps</TableCell>
                              </TableRow>
                            )}
                            {version.sample_rate && (
                              <TableRow>
                                <TableCell sx={{ fontWeight: 'bold', color: 'text.secondary' }}>Sample Rate</TableCell>
                                <TableCell>{version.sample_rate} Hz</TableCell>
                              </TableRow>
                            )}
                          </TableBody>
                        </Table>

                        {vSegs.length > 1 && metadataEntries.length > 0 && (
                          <Box
                            sx={{
                              mb: 2,
                              p: 1.5,
                              border: '1px solid',
                              borderColor: 'divider',
                              borderRadius: 1,
                              bgcolor: 'background.default',
                            }}
                          >
                            <Stack
                              direction="row"
                              spacing={1}
                              alignItems="center"
                              flexWrap="wrap"
                              useFlexGap
                              sx={{ mb: 1 }}
                            >
                              <Typography variant="subtitle2">
                                Overall Metadata
                              </Typography>
                              <Chip
                                label={`${metadataEntries.length} field${metadataEntries.length !== 1 ? 's' : ''}`}
                                size="small"
                                variant="outlined"
                              />
                            </Stack>
                            <Grid container spacing={1.5}>
                              {metadataEntries.map((entry) => (
                                <Grid item xs={12} md={6} key={`${version.id}-${entry.key}`}>
                                  <Box
                                    sx={{
                                      p: 1.25,
                                      borderRadius: 1,
                                      border: '1px solid',
                                      borderColor: entry.differsFromDb ? 'warning.main' : 'divider',
                                      bgcolor: entry.differsFromDb ? 'warning.50' : 'background.paper',
                                      height: '100%',
                                    }}
                                  >
                                    <Stack
                                      direction="row"
                                      alignItems="center"
                                      justifyContent="space-between"
                                      spacing={1}
                                      sx={{ mb: 0.5 }}
                                    >
                                      <Typography variant="caption" color="text.secondary">
                                        {entry.label}
                                      </Typography>
                                      {entry.differsFromDb && (
                                        <Chip label="≠ DB" size="small" color="warning" />
                                      )}
                                    </Stack>
                                    <Typography variant="body2" sx={{ wordBreak: 'break-word' }}>
                                      {entry.fileValue}
                                    </Typography>
                                    {entry.differsFromDb && (
                                      <Typography variant="caption" color="text.secondary">
                                        DB: {entry.storedValue}
                                      </Typography>
                                    )}
                                  </Box>
                                </Grid>
                              ))}
                            </Grid>
                          </Box>
                        )}

                        {/* Tag comparison component (replaces inline tags table) */}
                        <TagComparison bookId={version.id} versions={allVersions} refreshKey={filesRefreshKey} snapshotTimestamp={compareSnapshotTs} />

                        {/* Segments/files table for multi-file books */}
                        {vSegs.length > 0 && (() => {
                          const missingCount = vSegs.filter((s) => s.file_exists === false).length;
                          const isCurrentBook = isCurrent;
                          const allSelected = isCurrentBook && vSegs.length > 0 && selectedSegmentIds.size === vSegs.length;
                          const someSelected = isCurrentBook && selectedSegmentIds.size > 0 && !allSelected;
                          return (
                            <Box sx={{ mt: 2 }}>
                              <Stack direction="row" alignItems="center" spacing={2} sx={{ mb: 1 }}>
                                <Typography variant="subtitle2" color="text.secondary">
                                  Files ({vSegs.length})
                                </Typography>
                                {isCurrentBook && vSegs.length > 1 && (
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
                                      } catch {
                                        toast('Failed to extract track info', 'error');
                                      }
                                    }}
                                  >
                                    Auto-fill Track Numbers
                                  </Button>
                                )}
                              </Stack>
                              {missingCount > 0 && (
                                <Alert severity="warning" sx={{ mb: 1 }}>
                                  {missingCount} of {vSegs.length} file{vSegs.length !== 1 ? 's' : ''} missing on disk.
                                </Alert>
                              )}
                              {/* Segment action bar for current version */}
                              {isCurrentBook && selectedSegmentIds.size > 0 && vSegs.length > 1 && (
                                <Stack direction="row" spacing={1} alignItems="center" sx={{ mb: 1, p: 1, bgcolor: 'action.selected', borderRadius: 1 }}>
                                  <Typography variant="body2">{selectedSegmentIds.size} selected</Typography>
                                  <Button size="small" variant="contained" startIcon={<TransformIcon />}
                                    disabled={splittingVersion} onClick={handleSplitVersion}>
                                    {splittingVersion ? 'Splitting...' : 'Split to New Version'}
                                  </Button>
                                  <Button size="small" variant="contained" color="secondary" startIcon={<TransformIcon />}
                                    disabled={splittingToBooks} onClick={handleSplitToBooks}>
                                    {splittingToBooks ? 'Splitting...' : 'Split to New Books'}
                                  </Button>
                                  {versions.length > 1 && versions
                                    .filter((v) => v.id !== book.id)
                                    .map((v) => (
                                      <Button key={v.id} size="small" variant="outlined"
                                        onClick={() => handleMoveToVersion(v.id)}>
                                        Move to: {v.title}{v.format ? ` (${v.format.toUpperCase()})` : ''}
                                      </Button>
                                    ))}
                                </Stack>
                              )}
                              <Table size="small" data-testid="segment-table">
                                <TableHead>
                                  <TableRow>
                                    {isCurrentBook && vSegs.length > 1 && (
                                      <TableCell padding="checkbox">
                                        <Checkbox size="small" checked={allSelected} indeterminate={someSelected}
                                          onChange={(e) => {
                                            if (e.target.checked) {
                                              setSelectedSegmentIds(new Set(vSegs.map((s) => s.id)));
                                            } else {
                                              setSelectedSegmentIds(new Set());
                                            }
                                          }} />
                                      </TableCell>
                                    )}
                                    <TableCell>#</TableCell>
                                    <TableCell>File</TableCell>
                                    <TableCell>Duration</TableCell>
                                    <TableCell align="right">Size</TableCell>
                                  </TableRow>
                                </TableHead>
                                <TableBody>
                                  {visibleSegments.map((seg) => {
                                    const isMissing = seg.file_exists === false;
                                    const isSelected = isCurrentBook && selectedSegmentIds.has(seg.id);
                                    return (
                                      <TableRow key={seg.id} hover selected={isSelected}
                                        sx={{ cursor: isCurrentBook ? 'pointer' : 'default',
                                          ...(isMissing && { bgcolor: 'error.50', '&:hover': { bgcolor: 'error.100' } }) }}
                                        onClick={() => {
                                          if (!isCurrentBook) return;
                                          if (isMissing) { setRelocateSegment(seg); }
                                          else { setSelectedSegmentIds(new Set([seg.id])); setActiveTab('info'); }
                                        }}>
                                        {isCurrentBook && vSegs.length > 1 && (
                                          <TableCell padding="checkbox" onClick={(e) => e.stopPropagation()}>
                                            <Checkbox size="small" checked={isSelected}
                                              onChange={(e) => {
                                                const next = new Set(selectedSegmentIds);
                                                if (e.target.checked) next.add(seg.id); else next.delete(seg.id);
                                                setSelectedSegmentIds(next);
                                              }} />
                                          </TableCell>
                                        )}
                                        <TableCell>
                                          <Stack direction="row" alignItems="center" spacing={0.5}>
                                            {isMissing && (
                                              <Tooltip title={`Missing: ${seg.file_path}`}>
                                                <ErrorOutlineIcon color="error" fontSize="small" />
                                              </Tooltip>
                                            )}
                                            <span>{seg.track_number ?? '\u2014'}</span>
                                          </Stack>
                                        </TableCell>
                                        <TableCell sx={{ wordBreak: 'break-all', fontSize: '0.8rem', ...(isMissing && { color: 'error.main' }) }}>
                                          <Tooltip title={seg.file_path}><span>{seg.file_path}</span></Tooltip>
                                        </TableCell>
                                        <TableCell>{formatDuration(seg.duration_seconds)}</TableCell>
                                        <TableCell align="right">
                                          {formatBytes(seg.size_bytes)}
                                        </TableCell>
                                      </TableRow>
                                    );
                                  })}
                                </TableBody>
                              </Table>
                              {vSegs.length > SEGMENT_PREVIEW_COUNT && (
                                <Box sx={{ mt: 1 }}>
                                  <Button
                                    size="small"
                                    onClick={() => {
                                      setExpandedSegmentVersionIds((prev) => {
                                        const next = new Set(prev);
                                        if (next.has(version.id)) {
                                          next.delete(version.id);
                                        } else {
                                          next.add(version.id);
                                        }
                                        return next;
                                      });
                                    }}
                                  >
                                    {showAllSegments
                                      ? 'Show fewer files'
                                      : `Show all ${vSegs.length} files${hiddenSegmentCount > 0 ? ` (${hiddenSegmentCount} more)` : ''}`}
                                  </Button>
                                </Box>
                              )}
                            </Box>
                          );
                        })()}
                      </Box>
                    );
                  })}
                </Collapse>
              </Paper>
            );
          })}

          {/* iTunes link info panel */}
          {itunesLinked && (
            <Alert
              severity="info"
              variant="outlined"
              icon={false}
              sx={{ mt: 1 }}
            >
              <Stack direction="row" spacing={3} flexWrap="wrap" useFlexGap alignItems="center">
                <Typography variant="subtitle2" sx={{ fontWeight: 'bold' }}>
                  iTunes Linked
                </Typography>
                {book.itunes_persistent_id && (
                  <Box>
                    <Typography variant="caption" color="text.secondary">PID</Typography>
                    <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>{book.itunes_persistent_id}</Typography>
                  </Box>
                )}
                <Box>
                  <Typography variant="caption" color="text.secondary">Tracks Mapped</Typography>
                  <Typography variant="body2">{itunesPidCount}</Typography>
                </Box>
                {book.itunes_date_added && (
                  <Box>
                    <Typography variant="caption" color="text.secondary">Date Added</Typography>
                    <Typography variant="body2">{new Date(book.itunes_date_added).toLocaleDateString()}</Typography>
                  </Box>
                )}
                {book.itunes_play_count != null && book.itunes_play_count > 0 && (
                  <Box>
                    <Typography variant="caption" color="text.secondary">Play Count</Typography>
                    <Typography variant="body2">{book.itunes_play_count}</Typography>
                  </Box>
                )}
                {book.itunes_last_played && (
                  <Box>
                    <Typography variant="caption" color="text.secondary">Last Played</Typography>
                    <Typography variant="body2">{new Date(book.itunes_last_played).toLocaleDateString()}</Typography>
                  </Box>
                )}
                {book.itunes_rating != null && book.itunes_rating > 0 && (
                  <Box>
                    <Typography variant="caption" color="text.secondary">Rating</Typography>
                    <Typography variant="body2">{'★'.repeat(Math.round(book.itunes_rating / 20))}{'☆'.repeat(5 - Math.round(book.itunes_rating / 20))}</Typography>
                  </Box>
                )}
                {book.itunes_bookmark != null && book.itunes_bookmark > 0 && (
                  <Box>
                    <Typography variant="caption" color="text.secondary">Bookmark</Typography>
                    <Typography variant="body2">{formatDuration(book.itunes_bookmark / 1000)}</Typography>
                  </Box>
                )}
                {book.itunes_import_source && (
                  <Box>
                    <Typography variant="caption" color="text.secondary">Import Source</Typography>
                    <Typography variant="body2">{book.itunes_import_source}</Typography>
                  </Box>
                )}
                {book.file_path && (
                  <Box sx={{ flex: '1 1 100%' }}>
                    <Typography variant="caption" color="text.secondary">File Path</Typography>
                    <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.8rem', wordBreak: 'break-all' }}>{book.file_path}</Typography>
                  </Box>
                )}
              </Stack>
            </Alert>
          )}

          {/* Change Log */}
          <Paper sx={{ p: 2, mt: 2 }} data-testid="changelog-section">
            <Typography variant="subtitle1" fontWeight="bold" sx={{ mb: 1 }}>
              Change Log
            </Typography>
            <ChangeLog bookId={book.id} refreshKey={filesRefreshKey} onRevert={() => { refreshFilesTab(); loadBook(); }} onCompareSnapshot={setCompareSnapshotTs} />
          </Paper>

        </Stack>
        );
      })()}


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

      {/* Rename preview dialog */}
      <Dialog
        open={renamePreviewDialogOpen}
        onClose={() => setRenamePreviewDialogOpen(false)}
        maxWidth="md"
        fullWidth
      >
        <DialogTitle>Preview Rename</DialogTitle>
        <DialogContent>
          {renamePreview && (
            <>
              <Typography variant="subtitle2" gutterBottom>
                File Path
              </Typography>
              <Table size="small" sx={{ mb: 2 }}>
                <TableBody>
                  <TableRow>
                    <TableCell sx={{ fontWeight: 'bold', width: 100 }}>Current</TableCell>
                    <TableCell sx={{ fontFamily: 'monospace', fontSize: '0.85rem', wordBreak: 'break-all' }}>
                      {renamePreview.current_path}
                    </TableCell>
                  </TableRow>
                  <TableRow>
                    <TableCell sx={{ fontWeight: 'bold' }}>Proposed</TableCell>
                    <TableCell sx={{ fontFamily: 'monospace', fontSize: '0.85rem', wordBreak: 'break-all' }}>
                      {renamePreview.proposed_path}
                    </TableCell>
                  </TableRow>
                </TableBody>
              </Table>

              {renamePreview.current_path === renamePreview.proposed_path && (
                <Alert severity="info" sx={{ mb: 2 }}>
                  File path will not change.
                </Alert>
              )}

              {renamePreview.tag_changes.length > 0 && (
                <>
                  <Typography variant="subtitle2" gutterBottom>
                    Tag Changes
                  </Typography>
                  <Table size="small">
                    <TableHead>
                      <TableRow>
                        <TableCell>Field</TableCell>
                        <TableCell>Current</TableCell>
                        <TableCell>Proposed</TableCell>
                      </TableRow>
                    </TableHead>
                    <TableBody>
                      {renamePreview.tag_changes.map((change) => (
                        <TableRow key={change.field}>
                          <TableCell sx={{ fontWeight: 'bold' }}>{change.field}</TableCell>
                          <TableCell sx={{ color: 'text.secondary' }}>
                            {change.current || '(empty)'}
                          </TableCell>
                          <TableCell>{change.proposed || '(empty)'}</TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </>
              )}

              {renamePreview.tag_changes.length === 0 && (
                <Alert severity="info" sx={{ mt: 1 }}>
                  No tag changes to apply.
                </Alert>
              )}
            </>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setRenamePreviewDialogOpen(false)}>Cancel</Button>
          <Button
            variant="contained"
            startIcon={<DriveFileRenameOutlineIcon />}
            onClick={handleApplyRename}
            disabled={applyingRename}
          >
            Apply Rename
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
          <Box component="ul" sx={{ mt: 1, '& li': { mb: 0.5 } }}>
            {[
              { label: 'Title', value: book?.title },
              { label: 'Album', value: book?.title ? `${book.title} (groups tracks in players)` : undefined },
              { label: 'Artist', value: book?.authors?.map((a) => a.name).join(' & ') || book?.author_name },
              { label: 'Narrator', value: book?.narrators?.map((n) => n.name).join(' & ') || book?.narrator },
              { label: 'Year', value: book?.audiobook_release_year || book?.print_year },
              { label: 'Genre', value: 'Audiobook' },
              { label: 'Language', value: book?.language },
              { label: 'Publisher', value: book?.publisher },
              { label: 'Series', value: book?.series_name },
              { label: 'Series Index', value: book?.series_position },
              { label: 'Description', value: book?.description ? `${book.description.slice(0, 60)}…` : undefined },
              { label: 'ISBN-13', value: book?.isbn13 },
              { label: 'ISBN-10', value: book?.isbn10 },
              { label: 'ASIN', value: book?.asin },
              { label: 'Edition', value: book?.edition },
              { label: 'Print Year', value: book?.print_year },
              { label: 'Book ID', value: book?.id },
              { label: 'Open Library', value: book?.open_library_id },
              { label: 'Google Books', value: book?.google_books_id },
              { label: 'Hardcover', value: book?.hardcover_id },
              { label: 'Cover URL', value: book?.cover_url },
              { label: 'Track numbers', value: segments.length > 1 ? 'written for multi-file books' : undefined },
            ]
              .filter((item) => item.value != null && item.value !== '' && item.value !== 0)
              .map((item) => (
                <li key={item.label}>
                  <strong>{item.label}</strong> — {String(item.value)}
                </li>
              ))}
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
      {relocateSegment && (
        <RelocateFileDialog
          open={!!relocateSegment}
          onClose={() => setRelocateSegment(null)}
          segment={relocateSegment}
          bookId={book.id}
          onRelocated={async () => {
            if (id) {
              const segs = await api.getBookSegments(id);
              setSegments(segs);
            }
          }}
        />
      )}
    </Box>
  );
};
