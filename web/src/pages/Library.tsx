// file: web/src/pages/Library.tsx
// version: 1.30.2
// guid: 3f4a5b6c-7d8e-9f0a-1b2c-3d4e5f6a7b8c

import { useState, useEffect, useCallback, useRef } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import {
  Typography,
  Box,
  Pagination,
  Button,
  Stack,
  Chip,
  Paper,
  Alert,
  AlertTitle,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  TextField,
  IconButton,
  FormControlLabel,
  Checkbox,
  Snackbar,
  Collapse,
  MenuItem,
  LinearProgress,
  List,
  ListItem,
  ListItemText,
  ListItemSecondaryAction,
  Tooltip,
} from '@mui/material';
import {
  FilterList as FilterListIcon,
  Upload as UploadIcon,
  FolderOpen as FolderOpenIcon,
  Add as AddIcon,
  Delete as DeleteIcon,
  DeleteSweep as DeleteSweepIcon,
  ExpandMore as ExpandMoreIcon,
  CloudDownload as CloudDownloadIcon,
  Refresh as RefreshIcon,
} from '@mui/icons-material';
import { AudiobookGrid } from '../components/audiobooks/AudiobookGrid';
import { AudiobookList } from '../components/audiobooks/AudiobookList';
import { SearchBar, ViewMode } from '../components/audiobooks/SearchBar';
import { FilterSidebar } from '../components/audiobooks/FilterSidebar';
import { ServerFileBrowser } from '../components/common/ServerFileBrowser';
import { MetadataEditDialog } from '../components/audiobooks/MetadataEditDialog';
import { BatchEditDialog } from '../components/audiobooks/BatchEditDialog';
import { VersionManagement } from '../components/audiobooks/VersionManagement';
import type { Audiobook, FilterOptions } from '../types';
import { SortField, SortOrder } from '../types';
import * as api from '../services/api';
import {
  eventSourceManager,
  type EventSourceEvent,
  type EventSourceStatus,
} from '../services/eventSourceManager';
import { pollOperation } from '../utils/operationPolling';

interface ImportPath {
  id: number;
  path: string;
  status: 'idle' | 'scanning';
  book_count: number;
}

interface BulkActionResult {
  book_id: string;
  title: string;
  status: 'updated' | 'organized' | 'error' | 'skipped';
  message?: string;
}

interface BulkActionProgress {
  total: number;
  completed: number;
  results: BulkActionResult[];
}

type DuplicateAction = 'skip' | 'link' | 'replace';

type DuplicateDialogState = {
  duplicate: Audiobook;
  existing: Audiobook;
};

type OrganizeErrorState = {
  book: Audiobook;
  message: string;
};

const buildHashCandidates = (book: Audiobook): string[] => {
  const hashes: string[] = [];
  if (book.file_hash) hashes.push(book.file_hash);
  if (book.original_file_hash) hashes.push(book.original_file_hash);
  if (book.organized_file_hash) hashes.push(book.organized_file_hash);
  return hashes;
};

const getResultLabel = (result: BulkActionResult): string => {
  if (result.message) return result.message;
  if (result.status === 'organized') return 'Organized';
  if (result.status === 'updated') return 'Updated';
  if (result.status === 'skipped') return 'Skipped';
  return 'Failed';
};

export const Library = () => {
  const [searchParams, setSearchParams] = useSearchParams();
  const initialSearch = searchParams.get('search') ?? '';
  const initialViewMode =
    (searchParams.get('view') as ViewMode) || ('grid' as ViewMode);
  const initialSortBy = ((): SortField => {
    const value = searchParams.get('sort');
    if (value && Object.values(SortField).includes(value as SortField)) {
      return value as SortField;
    }
    return SortField.Title;
  })();
  const initialSortOrder =
    searchParams.get('order') === SortOrder.Descending
      ? SortOrder.Descending
      : SortOrder.Ascending;
  const initialPage = Math.max(
    1,
    parseInt(searchParams.get('page') || '1', 10)
  );
  const initialItemsPerPage = Math.max(
    10,
    parseInt(searchParams.get('limit') || '20', 10)
  );
  const initialFilters: FilterOptions = {
    author: searchParams.get('author') || undefined,
    series: searchParams.get('series') || undefined,
    genre: searchParams.get('genre') || undefined,
    language: searchParams.get('language') || undefined,
    libraryState: searchParams.get('state') || undefined,
  };

  const [audiobooks, setAudiobooks] = useState<Audiobook[]>([]);
  const [loading, setLoading] = useState(false);
  const [searchQuery, setSearchQuery] = useState(initialSearch);
  const [debouncedSearch, setDebouncedSearch] = useState('');
  const [viewMode, setViewMode] = useState<ViewMode>(initialViewMode);
  const [sortBy, setSortBy] = useState<SortField>(initialSortBy);
  const [sortOrder, setSortOrder] = useState<SortOrder>(initialSortOrder);
  const [filterOpen, setFilterOpen] = useState(false);
  const [filters, setFilters] = useState<FilterOptions>(initialFilters);
  const [page, setPage] = useState(initialPage);
  const [itemsPerPage, setItemsPerPage] = useState(initialItemsPerPage);
  const [totalPages, setTotalPages] = useState(1);
  const [editingAudiobook, setEditingAudiobook] = useState<Audiobook | null>(
    null
  );
  const [selectedAudiobooks, setSelectedAudiobooks] = useState<Audiobook[]>([]);
  const [batchEditOpen, setBatchEditOpen] = useState(false);
  const [versionManagementOpen, setVersionManagementOpen] = useState(false);
  const [versionManagingAudiobook, setVersionManagingAudiobook] =
    useState<Audiobook | null>(null);
  const [availableAuthors, setAvailableAuthors] = useState<string[]>([]);
  const [availableSeries, setAvailableSeries] = useState<string[]>([]);
  const [availableGenres, setAvailableGenres] = useState<string[]>([]);
  const [availableLanguages, setAvailableLanguages] = useState<string[]>([]);

  // Import path management
  const [importPaths, setImportPaths] = useState<ImportPath[]>([]);
  const [importPathsExpanded, setImportPathsExpanded] = useState(false);
  const [addPathDialogOpen, setAddPathDialogOpen] = useState(false);
  const [newImportPath, setNewImportPath] = useState('');
  const [showServerBrowser, setShowServerBrowser] = useState(false);
  const [systemStatus, setSystemStatus] = useState<api.SystemStatus | null>(
    null
  );
  const [organizeRunning, setOrganizeRunning] = useState(false);
  const [activeScanOp, setActiveScanOp] = useState<api.Operation | null>(null);
  const [activeOrganizeOp, setActiveOrganizeOp] =
    useState<api.Operation | null>(null);
  const [operationLogs, setOperationLogs] = useState<
    Record<
      string,
      {
        level: string;
        message: string;
        details?: string;
        timestamp: number;
        expanded?: boolean;
      }[]
    >
  >({});
  const logContainerRefs = useRef<Record<string, HTMLDivElement | null>>({});
  const bulkFetchCancelRef = useRef(false);
  const bulkOrganizeCancelRef = useRef(false);
  const [softDeletedCount, setSoftDeletedCount] = useState(0);
  const [softDeletedBooks, setSoftDeletedBooks] = useState<Audiobook[]>([]);
  const [softDeletedLoading, setSoftDeletedLoading] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [bookPendingDelete, setBookPendingDelete] = useState<Audiobook | null>(
    null
  );
  const [deleteOptions, setDeleteOptions] = useState({
    softDelete: true,
    blockHash: true,
  });
  const [deleteInProgress, setDeleteInProgress] = useState(false);
  const [purgeDialogOpen, setPurgeDialogOpen] = useState(false);
  const [purgeDeleteFiles, setPurgeDeleteFiles] = useState(false);
  const [purgeInProgress, setPurgeInProgress] = useState(false);
  const [purgingBookId, setPurgingBookId] = useState<string | null>(null);
  const [restoringBookId, setRestoringBookId] = useState<string | null>(null);
  const [batchDeleteDialogOpen, setBatchDeleteDialogOpen] = useState(false);
  const [batchDeleteInProgress, setBatchDeleteInProgress] = useState(false);
  const [batchRestoreInProgress, setBatchRestoreInProgress] = useState(false);
  const [alert, setAlert] = useState<{
    severity: 'success' | 'error' | 'info' | 'warning';
    message: string;
    actionLabel?: string;
    onAction?: () => void;
  } | null>(null);
  const [sseNotice, setSseNotice] = useState<{
    severity: 'warning' | 'success';
    message: string;
  } | null>(null);
  const sseStatusRef = useRef<EventSourceStatus['state'] | null>(null);
  const sseNoticeTimerRef = useRef<number | null>(null);

  const [importFileDialogOpen, setImportFileDialogOpen] = useState(false);
  const [importFilePath, setImportFilePath] = useState('');
  const [importFileOrganize, setImportFileOrganize] = useState(true);
  const [importFileInProgress, setImportFileInProgress] = useState(false);

  const [bulkFetchDialogOpen, setBulkFetchDialogOpen] = useState(false);
  const [bulkFetchInProgress, setBulkFetchInProgress] = useState(false);
  const [bulkFetchProgress, setBulkFetchProgress] =
    useState<BulkActionProgress | null>(null);
  const [bulkOrganizeDialogOpen, setBulkOrganizeDialogOpen] = useState(false);
  const [bulkOrganizeInProgress, setBulkOrganizeInProgress] = useState(false);
  const [bulkOrganizeProgress, setBulkOrganizeProgress] =
    useState<BulkActionProgress | null>(null);
  const [duplicateDialog, setDuplicateDialog] =
    useState<DuplicateDialogState | null>(null);
  const duplicateResolverRef =
    useRef<((action: DuplicateAction) => void) | null>(null);
  const [bulkOrganizeError, setBulkOrganizeError] =
    useState<OrganizeErrorState | null>(null);
  const bulkOrganizeSnapshotRef = useRef<Map<string, Audiobook>>(
    new Map()
  );

  // SSE subscription for live operation progress & logs + historical hydration
  useEffect(() => {
    // Fetch active operations to hydrate UI on reload
    (async () => {
      try {
        const active = await api.getActiveOperations();
        for (const op of active) {
          const partial: api.Operation = {
            id: op.id,
            type: op.type,
            status: op.status,
            progress: op.progress,
            total: op.total,
            message: op.message,
            created_at: new Date().toISOString(),
          } as api.Operation;
          if (op.type === 'scan') setActiveScanOp(partial);
          if (op.type === 'organize') setActiveOrganizeOp(partial);
          // Hydrate historical tail logs (last 100)
          try {
            const hist = await api.getOperationLogsTail(op.id, 100);
            if (hist && hist.length) {
              setOperationLogs((prev) => ({
                ...prev,
                [op.id]: hist.map((h: api.OperationLog) => ({
                  level: h.level,
                  message: h.message,
                  details: h.details,
                  timestamp: Date.parse(h.created_at) || Date.now(),
                })),
              }));
            }
          } catch (e) {
            // ignore hydration errors
          }
        }
      } catch {
        // ignore
      }
    })();

    const unsubscribe = eventSourceManager.subscribe(
      (evt: EventSourceEvent) => {
        if (!evt || !evt.type) return;
        if (evt.type === 'heartbeat') return; // Ignore heartbeat messages

        if (evt.type === 'operation.log' && evt.data?.operation_id) {
          const opId = String(evt.data.operation_id);
          setOperationLogs((prev) => {
            const existing = prev[opId] || [];
            const next = [
              ...existing,
              {
                level: String(evt.data?.level ?? 'info'),
                message: String(evt.data?.message ?? ''),
                details: evt.data?.details as string | undefined,
                timestamp: Date.now(),
              },
            ];
            return { ...prev, [opId]: next.slice(-200) };
          });
        } else if (
          evt.type === 'operation.progress' &&
          evt.data?.operation_id
        ) {
          const opId = String(evt.data.operation_id);
          const update = (op: api.Operation | null): api.Operation | null => {
            if (!op || op.id !== opId) return op;
            return {
              ...op,
              progress: Number(evt.data?.current ?? 0),
              total: Number(evt.data?.total ?? 0),
              message: String(evt.data?.message ?? ''),
            };
          };
          setActiveScanOp((prev) => update(prev));
          setActiveOrganizeOp((prev) => update(prev));
        } else if (evt.type === 'operation.status' && evt.data?.operation_id) {
          const opId = String(evt.data.operation_id);
          const status = String(evt.data?.status ?? '');
          const finalize = (op: api.Operation | null): api.Operation | null => {
            if (!op || op.id !== opId) return op;
            return { ...op, status };
          };
          setActiveScanOp((prev) => finalize(prev));
          setActiveOrganizeOp((prev) => finalize(prev));
        }
      },
      (status: EventSourceStatus) => {
        const previousState = sseStatusRef.current;
        sseStatusRef.current = status.state;

        if (sseNoticeTimerRef.current) {
          window.clearTimeout(sseNoticeTimerRef.current);
          sseNoticeTimerRef.current = null;
        }

        if (status.state === 'reconnecting' || status.state === 'error') {
          setSseNotice({
            severity: 'warning',
            message: 'Connection lost. Reconnecting...',
          });
        } else if (status.state === 'open') {
          if (previousState && previousState !== 'open') {
            setSseNotice({
              severity: 'success',
              message: 'Connection restored.',
            });
            sseNoticeTimerRef.current = window.setTimeout(() => {
              setSseNotice(null);
              sseNoticeTimerRef.current = null;
            }, 3000);
          }
          console.log('EventSource connection established');
        }

        if (status.state === 'reconnecting' && status.delayMs) {
          console.warn(
            `EventSource connection lost (attempt ${status.attempt}), reconnecting in ${Math.round(status.delayMs / 1000)}s...`
          );
        }
      }
    );

    return () => {
      unsubscribe();
      if (sseNoticeTimerRef.current) {
        window.clearTimeout(sseNoticeTimerRef.current);
        sseNoticeTimerRef.current = null;
      }
    };
  }, []);

  // Auto-scroll effect when logs update (placed at component top-level, not inside JSX)
  useEffect(() => {
    Object.entries(logContainerRefs.current).forEach(([, el]) => {
      if (!el) return;
      const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 20;
      if (atBottom) {
        el.scrollTop = el.scrollHeight;
      }
    });
  }, [operationLogs]);

  // Debounce search query
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearch(searchQuery);
    }, 300);

    return () => clearTimeout(timer);
  }, [searchQuery]);

  useEffect(() => {
    setPage(1);
  }, [searchQuery, filters, sortBy, sortOrder, itemsPerPage]);

  useEffect(() => {
    const params = new URLSearchParams();

    if (searchQuery) params.set('search', searchQuery);
    if (filters.author) params.set('author', filters.author);
    if (filters.series) params.set('series', filters.series);
    if (filters.genre) params.set('genre', filters.genre);
    if (filters.language) params.set('language', filters.language);
    if (filters.libraryState) params.set('state', filters.libraryState);
    if (sortBy !== SortField.Title) params.set('sort', sortBy);
    if (sortOrder !== SortOrder.Ascending) params.set('order', sortOrder);
    if (viewMode !== 'grid') params.set('view', viewMode);
    if (page > 1) params.set('page', page.toString());
    if (itemsPerPage !== 20) params.set('limit', itemsPerPage.toString());

    setSearchParams(params, { replace: true });
  }, [
    filters,
    itemsPerPage,
    page,
    searchQuery,
    setSearchParams,
    sortBy,
    sortOrder,
    viewMode,
  ]);

  const loadSoftDeleted = useCallback(async () => {
    setSoftDeletedLoading(true);
    try {
      const { items, count } = await api.getSoftDeletedBooks(500, 0);
      setSoftDeletedBooks(items);
      setSoftDeletedCount(count);
    } catch (e) {
      console.error('Failed to load soft-deleted books', e);
      setSoftDeletedBooks([]);
      setSoftDeletedCount(0);
    } finally {
      setSoftDeletedLoading(false);
    }
  }, []);

  const selectedIds = new Set(selectedAudiobooks.map((book) => book.id));
  const hasSelection = selectedAudiobooks.length > 0;
  const allOnPageSelected =
    audiobooks.length > 0 &&
    audiobooks.every((book) => selectedIds.has(book.id));
  const someOnPageSelected = audiobooks.some((book) =>
    selectedIds.has(book.id)
  );
  const selectedHasDeleted = selectedAudiobooks.some(
    (book) => book.marked_for_deletion
  );
  const selectedHasActive = selectedAudiobooks.some(
    (book) => !book.marked_for_deletion
  );
  const selectedHasImport = selectedAudiobooks.some(
    (book) => book.library_state === 'import'
  );

  const handleToggleSelect = (audiobook: Audiobook) => {
    setSelectedAudiobooks((prev) => {
      if (prev.some((selected) => selected.id === audiobook.id)) {
        return prev.filter((selected) => selected.id !== audiobook.id);
      }
      return [...prev, audiobook];
    });
  };

  const handleSelectAllOnPage = () => {
    setSelectedAudiobooks((prev) => {
      const byId = new Map(prev.map((book) => [book.id, book]));
      audiobooks.forEach((book) => {
        if (!byId.has(book.id)) {
          byId.set(book.id, book);
        }
      });
      return Array.from(byId.values());
    });
  };

  const handleToggleSelectAllOnPage = () => {
    if (allOnPageSelected) {
      setSelectedAudiobooks((prev) =>
        prev.filter(
          (book) =>
            !audiobooks.some((pageBook) => pageBook.id === book.id)
        )
      );
      return;
    }
    handleSelectAllOnPage();
  };

  const handleClearSelection = () => {
    setSelectedAudiobooks([]);
  };

  const loadAudiobooks = useCallback(async () => {
    setLoading(true);
    try {
      const offset = (page - 1) * itemsPerPage;

      const [bookCount, folders] = await Promise.all([
        api.countBooks(),
        api.getImportPaths(),
      ]);

      const fetchLimit = Math.max(bookCount, itemsPerPage);
      const books = debouncedSearch
        ? await api.searchBooks(debouncedSearch, fetchLimit)
        : await api.getBooks(fetchLimit, 0);

      // Convert API books to Audiobook type
      const convertedBooks: Audiobook[] = books.map((book) => ({
        id: book.id,
        title: book.title,
        author: book.author_name || 'Unknown',
        narrator: book.narrator,
        series: book.series_name,
        series_number: book.series_position,
        genre: book.genre,
        language: book.language,
        audiobook_release_year: book.audiobook_release_year,
        year: book.audiobook_release_year || book.print_year,
        print_year: book.print_year,
        duration: book.duration,
        coverImage: book.cover_image,
        filePath: book.file_path,
        file_path: book.file_path,
        fileSize: 0, // Not provided by API
        format: book.file_path.split('.').pop()?.toUpperCase() || 'Unknown',
        quality: book.quality,
        bitrate: book.bitrate,
        addedDate: book.created_at,
        created_at: book.created_at,
        updated_at: book.updated_at,
        lastPlayed: undefined,
        library_state: book.library_state,
        marked_for_deletion: book.marked_for_deletion,
        marked_for_deletion_at: book.marked_for_deletion_at,
        original_file_hash: book.original_file_hash,
        organized_file_hash: book.organized_file_hash,
      }));

      const uniqueAuthors = Array.from(
        new Set(
          convertedBooks
            .map((book) => book.author)
            .filter((author): author is string => Boolean(author))
        )
      ).sort();
      const uniqueSeries = Array.from(
        new Set(
          convertedBooks
            .map((book) => book.series)
            .filter((series): series is string => Boolean(series))
        )
      ).sort();
      const uniqueGenres = Array.from(
        new Set(
          convertedBooks
            .map((book) => book.genre)
            .filter((genre): genre is string => Boolean(genre))
        )
      ).sort();
      const uniqueLanguages = Array.from(
        new Set(
          convertedBooks
            .map((book) => book.language)
            .filter((language): language is string => Boolean(language))
        )
      ).sort();

      setAvailableAuthors(uniqueAuthors);
      setAvailableSeries(uniqueSeries);
      setAvailableGenres(uniqueGenres);
      setAvailableLanguages(uniqueLanguages);

      let filteredBooks = [...convertedBooks];
      if (filters.author) {
        const authorFilter = filters.author.toLowerCase();
        filteredBooks = filteredBooks.filter((book) =>
          (book.author || '').toLowerCase().includes(authorFilter)
        );
      }
      if (filters.series) {
        const seriesFilter = filters.series.toLowerCase();
        filteredBooks = filteredBooks.filter((book) =>
          (book.series || '').toLowerCase().includes(seriesFilter)
        );
      }
      if (filters.genre) {
        const genreFilter = filters.genre.toLowerCase();
        filteredBooks = filteredBooks.filter((book) =>
          (book.genre || '').toLowerCase().includes(genreFilter)
        );
      }
      if (filters.language) {
        const languageFilter = filters.language.toLowerCase();
        filteredBooks = filteredBooks.filter((book) =>
          (book.language || '').toLowerCase().includes(languageFilter)
        );
      }
      if (filters.libraryState) {
        if (filters.libraryState === 'deleted') {
          filteredBooks = filteredBooks.filter(
            (book) => book.marked_for_deletion
          );
        } else {
          filteredBooks = filteredBooks.filter(
            (book) => book.library_state === filters.libraryState
          );
        }
      }

      const sortedBooks = filteredBooks.sort((a, b) => {
        let comparison = 0;
        switch (sortBy) {
          case SortField.Title:
            comparison = a.title.localeCompare(b.title);
            break;
          case SortField.Author:
            comparison = (a.author || '').localeCompare(b.author || '');
            break;
          case SortField.Year: {
            const aYear = a.audiobook_release_year || a.print_year || 0;
            const bYear = b.audiobook_release_year || b.print_year || 0;
            comparison = aYear - bYear;
            break;
          }
          case SortField.CreatedAt:
            comparison =
              new Date(a.created_at).getTime() -
              new Date(b.created_at).getTime();
            break;
          default:
            comparison = 0;
        }
        return sortOrder === SortOrder.Descending ? comparison * -1 : comparison;
      });

      const total = sortedBooks.length;
      const paginatedBooks = sortedBooks.slice(
        offset,
        offset + itemsPerPage
      );

      setAudiobooks(paginatedBooks);
      setTotalPages(Math.max(1, Math.ceil(total / itemsPerPage)));

      // Load import paths
      const convertedPaths: ImportPath[] = folders.map((folder) => ({
        id: folder.id,
        path: folder.path,
        status: 'idle',
        book_count: folder.book_count,
      }));
      setImportPaths(convertedPaths);
    } catch (error) {
      if (error instanceof api.ApiError && error.status === 401) {
        navigate('/login');
        return;
      }
      if (error instanceof api.ApiError && error.status >= 500) {
        setAlert({
          severity: 'error',
          message: 'Server error occurred.',
        });
      }
      const message =
        error instanceof Error ? error.message : 'Failed to load audiobooks.';
      if (message.toLowerCase().includes('timeout')) {
        setAlert({
          severity: 'error',
          message: 'Request timed out.',
          actionLabel: 'Retry',
          onAction: () => loadAudiobooks(),
        });
      }
      console.error('Failed to load audiobooks:', error);
      setAudiobooks([]);
      setTotalPages(1);
    } finally {
      setLoading(false);
    }
  }, [
    debouncedSearch,
    filters,
    itemsPerPage,
    page,
    sortBy,
    sortOrder,
    navigate,
  ]);

  const handleManualImport = () => {
    setImportFilePath('');
    setImportFileOrganize(true);
    setImportFileDialogOpen(true);
  };

  const handleImportFile = async () => {
    const target = importFilePath.trim();
    if (!target) {
      setAlert({
        severity: 'info',
        message: 'Select a file to import from the server.',
      });
      return;
    }

    setImportFileInProgress(true);
    try {
      await api.importFile(target, importFileOrganize);
      setAlert({
        severity: 'success',
        message: 'Import started successfully.',
      });
      setImportFileDialogOpen(false);
      setImportFilePath('');
      await loadAudiobooks();
    } catch (error) {
      console.error('Failed to import file:', error);
      const message =
        error instanceof Error ? error.message : 'Failed to import file.';
      setAlert({
        severity: 'error',
        message,
      });
    } finally {
      setImportFileInProgress(false);
    }
  };

  // Load audiobooks when filters change
  useEffect(() => {
    loadAudiobooks();
    // Load system status for library storage section
    (async () => {
      try {
        const status = await api.getSystemStatus();
        setSystemStatus(status);
      } catch (e) {
        console.error('Failed to load system status', e);
      }
    })();
  }, [loadAudiobooks]);

  useEffect(() => {
    loadSoftDeleted();
  }, [loadSoftDeleted]);

  const handleEdit = useCallback((audiobook: Audiobook) => {
    setEditingAudiobook(audiobook);
  }, []);

  const handleDelete = useCallback((audiobook: Audiobook) => {
    setBookPendingDelete(audiobook);
    setDeleteOptions({ softDelete: true, blockHash: true });
    setDeleteDialogOpen(true);
  }, []);

  const handleSaveMetadata = async (audiobook: Audiobook) => {
    try {
      // TODO: Replace with actual API call
      // await fetch(`/api/v1/audiobooks/${audiobook.id}`, {
      //   method: 'PUT',
      //   headers: { 'Content-Type': 'application/json' },
      //   body: JSON.stringify(audiobook)
      // });
      console.log('Saved audiobook:', audiobook);

      // Update local state
      setAudiobooks((prev) =>
        prev.map((ab) => (ab.id === audiobook.id ? audiobook : ab))
      );
      setEditingAudiobook(null);
    } catch (error) {
      console.error('Failed to save audiobook:', error);
      throw error;
    }
  };

  const handleConfirmDelete = async () => {
    if (!bookPendingDelete) return;
    setDeleteInProgress(true);
    try {
      await api.deleteBook(bookPendingDelete.id, {
        softDelete: deleteOptions.softDelete,
        blockHash: deleteOptions.blockHash,
      });
      setAlert({
        severity: 'success',
        message: deleteOptions.softDelete
          ? 'Audiobook was soft deleted and hidden from the library.'
          : 'Audiobook was deleted.',
      });
      setDeleteDialogOpen(false);
      setBookPendingDelete(null);
      await loadAudiobooks();
      await loadSoftDeleted();
    } catch (error) {
      console.error('Failed to delete audiobook:', error);
      setAlert({
        severity: 'error',
        message: 'Failed to delete audiobook. Please try again.',
      });
    } finally {
      setDeleteInProgress(false);
    }
  };

  const handleCloseDeleteDialog = () => {
    setDeleteDialogOpen(false);
    setBookPendingDelete(null);
  };

  const handleBatchDelete = async () => {
    if (!hasSelection) return;
    setBatchDeleteInProgress(true);
    try {
      const activeBooks = selectedAudiobooks.filter(
        (book) => !book.marked_for_deletion
      );
      await Promise.all(
        activeBooks.map((book) =>
          api.deleteBook(book.id, { softDelete: true, blockHash: true })
        )
      );
      setAlert({
        severity: 'success',
        message: `Soft deleted ${activeBooks.length} selected audiobooks.`,
      });
      setSelectedAudiobooks([]);
      await loadAudiobooks();
      await loadSoftDeleted();
    } catch (error) {
      console.error('Failed to batch delete audiobooks:', error);
      setAlert({
        severity: 'error',
        message: 'Failed to delete selected audiobooks.',
      });
    } finally {
      setBatchDeleteInProgress(false);
      setBatchDeleteDialogOpen(false);
    }
  };

  const handleBatchRestore = async () => {
    if (!hasSelection) return;
    setBatchRestoreInProgress(true);
    try {
      const deletedBooks = selectedAudiobooks.filter(
        (book) => book.marked_for_deletion
      );
      await Promise.all(
        deletedBooks.map((book) => api.restoreSoftDeletedBook(book.id))
      );
      setAlert({
        severity: 'success',
        message: `Restored ${deletedBooks.length} selected audiobooks.`,
      });
      setSelectedAudiobooks([]);
      await loadAudiobooks();
      await loadSoftDeleted();
    } catch (error) {
      console.error('Failed to restore selected audiobooks:', error);
      setAlert({
        severity: 'error',
        message: 'Failed to restore selected audiobooks.',
      });
    } finally {
      setBatchRestoreInProgress(false);
    }
  };

  const handlePurgeOne = async (book: Audiobook) => {
    setPurgingBookId(book.id);
    try {
      await api.deleteBook(book.id, { softDelete: false, blockHash: false });
      setAlert({
        severity: 'success',
        message: `"${book.title}" was purged from the library.`,
      });
      await loadAudiobooks();
      await loadSoftDeleted();
    } catch (error) {
      console.error('Failed to purge audiobook', error);
      setAlert({ severity: 'error', message: 'Failed to purge audiobook.' });
    } finally {
      setPurgingBookId(null);
    }
  };

  const handleRestoreOne = async (book: Audiobook) => {
    setRestoringBookId(book.id);
    try {
      await api.restoreSoftDeletedBook(book.id);
      setAlert({
        severity: 'success',
        message: `"${book.title}" was restored to the library.`,
      });
      await loadAudiobooks();
      await loadSoftDeleted();
    } catch (error) {
      console.error('Failed to restore audiobook', error);
      setAlert({ severity: 'error', message: 'Failed to restore audiobook.' });
    } finally {
      setRestoringBookId(null);
    }
  };

  const handleConfirmPurge = async () => {
    setPurgeInProgress(true);
    try {
      const result = await api.purgeSoftDeletedBooks(purgeDeleteFiles);
      setAlert({
        severity: 'success',
        message: `Purged ${result.purged} soft-deleted ${result.purged === 1 ? 'book' : 'books'}.`,
      });
      setPurgeDialogOpen(false);
      setPurgeDeleteFiles(false);
      await loadAudiobooks();
      await loadSoftDeleted();
    } catch (error) {
      console.error('Failed to purge soft-deleted books', error);
      setAlert({
        severity: 'error',
        message: 'Failed to purge soft-deleted books.',
      });
    } finally {
      setPurgeInProgress(false);
    }
  };

  const handleBatchSave = async (updates: Partial<Audiobook>) => {
    try {
      // TODO: Replace with actual API call
      // await fetch('/api/v1/audiobooks/batch', {
      //   method: 'PATCH',
      //   headers: { 'Content-Type': 'application/json' },
      //   body: JSON.stringify({
      //     ids: selectedAudiobooks.map(ab => ab.id),
      //     updates
      //   })
      // });
      console.log(
        'Batch updated:',
        selectedAudiobooks.length,
        'audiobooks with',
        updates
      );

      // Update local state
      setAudiobooks((prev) =>
        prev.map((ab) =>
          selectedAudiobooks.some((selected) => selected.id === ab.id)
            ? { ...ab, ...updates }
            : ab
        )
      );
      setAlert({
        severity: 'success',
        message: `Updated metadata for ${selectedAudiobooks.length} audiobooks.`,
      });
      setSelectedAudiobooks([]);
      setBatchEditOpen(false);
    } catch (error) {
      console.error('Failed to batch update audiobooks:', error);
      throw error;
    }
  };

  const navigate = useNavigate();

  const handleClick = useCallback(
    (audiobook: Audiobook) => {
      navigate(`/library/${audiobook.id}`);
    },
    [navigate]
  );

  const handleVersionManage = (audiobook: Audiobook) => {
    setVersionManagingAudiobook(audiobook);
    setVersionManagementOpen(true);
  };

  const handleVersionManagementClose = () => {
    setVersionManagementOpen(false);
    setVersionManagingAudiobook(null);
  };

  const handleVersionUpdate = () => {
    loadAudiobooks();
  };

  const handleFetchMetadata = async (audiobook: Audiobook) => {
    try {
      const result = await api.fetchBookMetadata(audiobook.id);
      console.log(`Metadata fetched from ${result.source}:`, result.book);
      // Reload audiobooks to show updated data
      loadAudiobooks();
    } catch (error) {
      console.error('Failed to fetch metadata:', error);
      // TODO: Show error notification to user
    }
  };

  const handleBulkFetchMetadata = async () => {
    if (!hasSelection) {
      setAlert({
        severity: 'info',
        message: 'Select audiobooks to fetch metadata for.',
      });
      return;
    }

    setBulkFetchInProgress(true);
    bulkFetchCancelRef.current = false;

    const total = selectedAudiobooks.length;
    const results: BulkActionResult[] = [];
    let completed = 0;
    setBulkFetchProgress({ total, completed, results: [] });

    try {
      for (const book of selectedAudiobooks) {
        if (bulkFetchCancelRef.current) {
          break;
        }

        try {
          await api.fetchBookMetadata(book.id);
          results.push({
            book_id: book.id,
            title: book.title,
            status: 'updated',
          });
        } catch (error) {
          const message =
            error instanceof Error ? error.message : 'Failed to fetch metadata';
          results.push({
            book_id: book.id,
            title: book.title,
            status: 'error',
            message,
          });
        }

        completed += 1;
        setBulkFetchProgress({ total, completed, results: [...results] });
      }

      if (bulkFetchCancelRef.current) {
        setAlert({
          severity: 'info',
          message: 'Bulk fetch cancelled.',
        });
      } else {
        const successCount = results.filter((result) => result.status !== 'error')
          .length;
        const failedCount = results.length - successCount;

        setAlert({
          severity: failedCount > 0 ? 'warning' : 'success',
          message:
            failedCount > 0
              ? `${successCount} succeeded, ${failedCount} failed.`
              : `Metadata fetched for ${successCount} books.`,
        });
        setSelectedAudiobooks([]);
      }

      await loadAudiobooks();
    } catch (error) {
      console.error('Failed to bulk fetch metadata:', error);
      setAlert({
        severity: 'error',
        message: 'Failed to bulk fetch metadata.',
      });
    } finally {
      setBulkFetchInProgress(false);
      bulkFetchCancelRef.current = false;
    }
  };

  const handleCancelBulkFetch = () => {
    if (!bulkFetchInProgress) {
      setBulkFetchDialogOpen(false);
      setBulkFetchProgress(null);
      return;
    }
    bulkFetchCancelRef.current = true;
  };

  const requestDuplicateAction = (
    duplicate: Audiobook,
    existing: Audiobook
  ): Promise<DuplicateAction> =>
    new Promise((resolve) => {
      duplicateResolverRef.current = resolve;
      setDuplicateDialog({ duplicate, existing });
    });

  const handleDuplicateAction = (action: DuplicateAction) => {
    if (duplicateResolverRef.current) {
      duplicateResolverRef.current(action);
      duplicateResolverRef.current = null;
    }
    setDuplicateDialog(null);
  };

  const handleBulkOrganize = async () => {
    if (!hasSelection) {
      setAlert({
        severity: 'info',
        message: 'Select audiobooks to organize.',
      });
      return;
    }

    const importBooks = selectedAudiobooks.filter(
      (book) => book.library_state === 'import'
    );
    if (importBooks.length === 0) {
      setAlert({
        severity: 'info',
        message: 'Select import-state audiobooks to organize.',
      });
      return;
    }
    const importBookIds = importBooks.map((book) => book.id);

    setBulkOrganizeInProgress(true);
    setBulkOrganizeError(null);
    bulkOrganizeCancelRef.current = false;
    const snapshot = new Map<string, Audiobook>();
    importBooks.forEach((book) => {
      snapshot.set(book.id, { ...book });
    });
    bulkOrganizeSnapshotRef.current = snapshot;

    const total = importBooks.length;
    const results: BulkActionResult[] = [];
    let completed = 0;
    let encounteredError = false;
    setBulkOrganizeProgress({ total, completed, results: [] });

    const organizedByHash = new Map<string, Audiobook>();
    audiobooks
      .filter((item) => item.library_state === 'organized')
      .forEach((item) => {
        buildHashCandidates(item).forEach((hash) => {
          organizedByHash.set(hash, item);
        });
      });

    const findDuplicate = (target: Audiobook): Audiobook | null => {
      for (const hash of buildHashCandidates(target)) {
        const match = organizedByHash.get(hash);
        if (match && match.id !== target.id) {
          return match;
        }
      }
      return null;
    };

    try {
      await api.startOrganize(undefined, undefined, importBookIds);

      for (const book of importBooks) {
        if (bulkOrganizeCancelRef.current) {
          break;
        }
        const organizeError = book.organize_error;
        if (organizeError) {
          const errorMessage = `Failed to organize ${
            book.title || 'audiobook'
          }.`;
          results.push({
            book_id: book.id,
            title: book.title,
            status: 'error',
            message: organizeError,
          });
          completed += 1;
          setBulkOrganizeProgress({
            total,
            completed,
            results: [...results],
          });
          setBulkOrganizeError({
            book,
            message: errorMessage,
          });
          encounteredError = true;
          break;
        }

        const duplicate = findDuplicate(book);
        if (duplicate) {
          const action = await requestDuplicateAction(book, duplicate);
          if (action === 'skip') {
            results.push({
              book_id: book.id,
              title: book.title,
              status: 'skipped',
              message: 'Skipped duplicate file.',
            });
            completed += 1;
            setBulkOrganizeProgress({
              total,
              completed,
              results: [...results],
            });
            continue;
          }
          if (action === 'link') {
            const groupId =
              duplicate.version_group_id || `group-${duplicate.id}`;
            await api.linkBookVersion(duplicate.id, book.id);
            setAudiobooks((prev) =>
              prev.map((item) => {
                if (item.id === duplicate.id) {
                  return {
                    ...item,
                    version_group_id: groupId,
                    is_primary_version: true,
                  };
                }
                if (item.id === book.id) {
                  return { ...item, version_group_id: groupId };
                }
                return item;
              })
            );
          }
          if (action === 'replace') {
            setAudiobooks((prev) =>
              prev.map((item) =>
                item.id === duplicate.id
                  ? { ...item, marked_for_deletion: true }
                  : item
              )
            );
          }
        }

        results.push({
          book_id: book.id,
          title: book.title,
          status: 'organized',
        });
        setAudiobooks((prev) =>
          prev.map((ab) =>
            ab.id === book.id
              ? { ...ab, library_state: 'organized' }
              : ab
          )
        );
        buildHashCandidates(book).forEach((hash) => {
          organizedByHash.set(hash, book);
        });
        completed += 1;
        setBulkOrganizeProgress({ total, completed, results: [...results] });
      }

      if (bulkOrganizeCancelRef.current) {
        setAlert({
          severity: 'info',
          message: 'Organize cancelled.',
        });
      } else if (!encounteredError) {
        setAlert({
          severity: 'success',
          message: `Successfully organized ${completed} audiobooks.`,
        });
        setSelectedAudiobooks([]);
      }

      if (!bulkOrganizeCancelRef.current && !encounteredError) {
        await loadAudiobooks();
      }
    } catch (error) {
      console.error('Failed to organize selected audiobooks:', error);
      setAlert({
        severity: 'error',
        message: 'Failed to organize selected audiobooks.',
      });
    } finally {
      setBulkOrganizeInProgress(false);
      bulkOrganizeCancelRef.current = false;
    }
  };

  const handleCancelBulkOrganize = () => {
    if (!bulkOrganizeInProgress) {
      setBulkOrganizeDialogOpen(false);
      setBulkOrganizeProgress(null);
      setBulkOrganizeError(null);
      return;
    }
    bulkOrganizeCancelRef.current = true;
  };

  const handleCloseOrganizeError = () => {
    setBulkOrganizeError(null);
  };

  const handleOrganizeRollback = async () => {
    const snapshot = bulkOrganizeSnapshotRef.current;
    if (!snapshot.size) {
      setBulkOrganizeError(null);
      return;
    }

    try {
      for (const book of snapshot.values()) {
        await api.updateBook(book.id, {
          library_state: book.library_state,
          file_path: book.file_path,
          organized_file_hash: book.organized_file_hash,
        });
      }
      setAlert({
        severity: 'success',
        message: 'Rollback complete.',
      });
      setBulkOrganizeError(null);
      await loadAudiobooks();
    } catch (error) {
      console.error('Failed to rollback organize:', error);
      setAlert({
        severity: 'error',
        message: 'Rollback failed.',
      });
    }
  };

  const handleParseWithAI = async (audiobook: Audiobook) => {
    try {
      const result = await api.parseAudiobookWithAI(audiobook.id);
      console.log(
        `AI parsing completed with ${result.confidence} confidence:`,
        result.book
      );
      // Reload audiobooks to show updated data
      loadAudiobooks();
    } catch (error) {
      console.error('Failed to parse with AI:', error);
      // TODO: Show error notification to user
    }
  };

  const handleFiltersChange = (newFilters: FilterOptions) => {
    setFilters(newFilters);
    setPage(1); // Reset to first page on filter change
  };

  const handleSortChange = (newSort: SortField) => {
    setSortBy(newSort);
    if (newSort === SortField.CreatedAt) {
      setSortOrder(SortOrder.Descending);
    }
  };

  const getActiveFilterCount = () => {
    return Object.values(filters).filter((v) => v !== undefined && v !== '')
      .length;
  };

  const libraryBookCount =
    systemStatus?.library_book_count ?? systemStatus?.library.book_count ?? 0;
  const importBookCount =
    systemStatus?.import_book_count ??
    systemStatus?.import_paths?.book_count ??
    0;
  const librarySizeBytes =
    systemStatus?.library_size_bytes ?? systemStatus?.library.total_size ?? 0;
  const importSizeBytes =
    systemStatus?.import_size_bytes ??
    systemStatus?.import_paths?.total_size ??
    0;

  // Import path management handlers
  const handleAddImportPath = async () => {
    if (!newImportPath.trim()) return;

    try {
      const detailed = await api.addImportPathDetailed(
        newImportPath,
        newImportPath.split('/').pop() || 'Library'
      );
      const importPath = detailed.importPath;
      const newPath: ImportPath = {
        id: importPath.id,
        path: importPath.path,
        status: detailed.scan_operation_id ? 'scanning' : 'idle',
        book_count: importPath.book_count,
      };
      setImportPaths((prev) => [...prev, newPath]);
      setNewImportPath('');
      setShowServerBrowser(false);
      setAddPathDialogOpen(false);

      // If scan started, poll status until complete then refresh folders
      if (detailed.scan_operation_id) {
        const opId = detailed.scan_operation_id;
        const pollInterval = 2000;
        let attempts = 0;
        const maxAttempts = 150; // ~5 minutes
        const poll = async () => {
          try {
            const op = await api.getOperationStatus(opId);
            if (
              op.status === 'completed' ||
              op.status === 'failed' ||
              op.status === 'canceled'
            ) {
              // Refresh folder list to get updated book counts
              const folders = await api.getImportPaths();
              setImportPaths(
                folders.map((f) => ({
                  id: f.id,
                  path: f.path,
                  status: 'idle',
                  book_count: f.book_count,
                }))
              );
              return; // stop polling
            }
            attempts++;
            if (attempts < maxAttempts) {
              setTimeout(poll, pollInterval);
            }
          } catch (e) {
            attempts++;
            if (attempts < maxAttempts) {
              setTimeout(poll, pollInterval);
            }
          }
        };
        setTimeout(poll, pollInterval);
      }
    } catch (error) {
      console.error('Failed to add import path:', error);
    }
  };

  const handleServerBrowserSelect = (path: string, isDir: boolean) => {
    if (isDir) {
      setNewImportPath(path);
    }
  };

  const handleRemoveImportPath = async (id: number) => {
    try {
      await api.removeImportPath(id);
      setImportPaths((prev) => prev.filter((p) => p.id !== id));
    } catch (error) {
      console.error('Failed to remove import path:', error);
    }
  };

  const startPolling = (opId: string, type: 'scan' | 'organize') => {
    pollOperation(
      opId,
      { intervalMs: 2000 },
      (op) => {
        if (type === 'scan') setActiveScanOp(op);
        else setActiveOrganizeOp(op);
      },
      async (op) => {
        if (type === 'scan') {
          const folders = await api.getImportPaths();
          setImportPaths(
            folders.map((f) => ({
              id: f.id,
              path: f.path,
              status: 'idle',
              book_count: f.book_count,
            }))
          );
          setActiveScanOp(op);
        } else {
          setOrganizeRunning(false);
          setActiveOrganizeOp(op);
        }
        loadAudiobooks();
      },
      (err) => {
        console.warn('Polling error', err);
        if (type === 'organize') setOrganizeRunning(false);
      }
    );
  };

  const handleScanImportPath = async (id: number) => {
    try {
      const pathEntry = importPaths.find((p) => p.id === id);
      const path = pathEntry?.path;
      if (!path) return;
      setImportPaths((prev) =>
        prev.map((p) => (p.id === id ? { ...p, status: 'scanning' } : p))
      );
      const op = await api.startScan(path);
      startPolling(op.id, 'scan');
    } catch (error) {
      console.error('Failed to scan import path:', error);
      setImportPaths((prev) =>
        prev.map((p) => (p.id === id ? { ...p, status: 'idle' } : p))
      );
    }
  };

  const handleScanAll = async () => {
    try {
      // Mark all paths scanning
      setImportPaths((prev) => prev.map((p) => ({ ...p, status: 'scanning' })));
      const op = await api.startScan(); // no folder path -> scan all
      startPolling(op.id, 'scan');
    } catch (error) {
      console.error('Failed to start full scan:', error);
      setImportPaths((prev) => prev.map((p) => ({ ...p, status: 'idle' })));
    }
  };

  const handleFullRescan = async () => {
    try {
      // Mark all paths scanning
      setImportPaths((prev) => prev.map((p) => ({ ...p, status: 'scanning' })));
      // Force full rescan with database path updates
      const op = await api.startScan(undefined, undefined, true);
      startPolling(op.id, 'scan');
    } catch (error) {
      console.error('Failed to start full rescan:', error);
      setImportPaths((prev) => prev.map((p) => ({ ...p, status: 'idle' })));
    }
  };

  const handleOrganizeLibrary = async () => {
    try {
      setOrganizeRunning(true);
      const op = await api.startOrganize();
      startPolling(op.id, 'organize');
    } catch (e) {
      console.error('Failed to start organize', e);
      setOrganizeRunning(false);
    }
  };

  return (
    <Box
      sx={{
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
      }}
    >
      <Snackbar
        open={!!alert}
        autoHideDuration={5000}
        onClose={() => setAlert(null)}
        anchorOrigin={{ vertical: 'top', horizontal: 'center' }}
      >
        {alert ? (
          <Alert
            severity={alert.severity}
            onClose={() => setAlert(null)}
            sx={{ width: '100%' }}
            action={
              alert.actionLabel && alert.onAction ? (
                <Button
                  color="inherit"
                  size="small"
                  onClick={alert.onAction}
                >
                  {alert.actionLabel}
                </Button>
              ) : undefined
            }
          >
            {alert.message}
          </Alert>
        ) : undefined}
      </Snackbar>
      <Snackbar
        open={!!sseNotice}
        autoHideDuration={sseNotice?.severity === 'success' ? 3000 : null}
        onClose={() => setSseNotice(null)}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
      >
        {sseNotice ? (
          <Alert
            severity={sseNotice.severity}
            onClose={() => setSseNotice(null)}
            sx={{ width: '100%' }}
          >
            {sseNotice.message}
          </Alert>
        ) : undefined}
      </Snackbar>
      <Box
        display="flex"
        justifyContent="space-between"
        alignItems="center"
        mb={3}
      >
        <Typography variant="h4">Library</Typography>
        <Stack direction="row" spacing={2}>
          <Button
            startIcon={<UploadIcon />}
            onClick={handleManualImport}
            variant="contained"
          >
            Import Files
          </Button>
          <Button
            startIcon={<FilterListIcon />}
            onClick={() => setFilterOpen(true)}
            variant="outlined"
          >
            Filters
            {getActiveFilterCount() > 0 && (
              <Chip
                label={getActiveFilterCount()}
                size="small"
                color="primary"
                sx={{ ml: 1 }}
              />
            )}
          </Button>
          <Button
            startIcon={<CloudDownloadIcon />}
            onClick={() => setBulkFetchDialogOpen(true)}
            variant="outlined"
            disabled={!hasSelection}
          >
            Bulk Fetch Metadata
          </Button>
          <Button
            startIcon={<DeleteSweepIcon />}
            onClick={() => setPurgeDialogOpen(true)}
            variant="outlined"
            color="secondary"
            disabled={softDeletedCount === 0}
          >
            Purge Deleted {softDeletedCount > 0 ? `(${softDeletedCount})` : ''}
          </Button>
        </Stack>
      </Box>

      {systemStatus && (
        <Paper sx={{ p: 2, mb: 3 }}>
          <Stack
            direction="row"
            justifyContent="space-between"
            alignItems="center"
            flexWrap="wrap"
            gap={2}
          >
            <Box>
              <Typography variant="h6" gutterBottom>
                Main Library Storage
              </Typography>
              <Typography
                variant="body2"
                color={
                  systemStatus.library.path ? 'text.secondary' : 'warning.main'
                }
              >
                Path:{' '}
                {systemStatus.library.path ||
                  'Not configured - Please set library path in Settings'}
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Library Books: {libraryBookCount} | Import Books:{' '}
                {importBookCount} | Import Paths:{' '}
                {systemStatus.import_paths?.folder_count || 0}
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Library Size: {(librarySizeBytes / (1024 * 1024)).toFixed(2)} MB
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Import Size: {(importSizeBytes / (1024 * 1024)).toFixed(2)} MB
              </Typography>
              {activeOrganizeOp && activeOrganizeOp.status !== 'completed' && (
                <Box mt={1}>
                  <Stack direction="row" spacing={1} alignItems="center">
                    <Typography variant="caption" color="text.secondary">
                      Organizing: {activeOrganizeOp.progress}/
                      {activeOrganizeOp.total} {activeOrganizeOp.message}
                    </Typography>
                    <Button
                      size="small"
                      variant="text"
                      onClick={() => api.cancelOperation(activeOrganizeOp.id)}
                    >
                      Cancel
                    </Button>
                  </Stack>
                  {operationLogs[activeOrganizeOp.id] && (
                    <Box
                      mt={0.5}
                      ref={(el: HTMLDivElement | null) => {
                        logContainerRefs.current[activeOrganizeOp.id] = el;
                      }}
                      sx={{
                        maxHeight: 140,
                        overflowY: 'auto',
                        borderLeft: '2px solid',
                        borderColor: 'divider',
                        pl: 1,
                      }}
                    >
                      {operationLogs[activeOrganizeOp.id].map((l, idx) => (
                        <Box key={idx} sx={{ mb: 0.3 }}>
                          <Typography
                            variant="caption"
                            display="block"
                            sx={{
                              color:
                                l.level === 'error'
                                  ? 'error.main'
                                  : l.level === 'warn'
                                    ? 'warning.main'
                                    : 'text.secondary',
                              fontWeight: l.level === 'error' ? 600 : 400,
                              cursor: l.details ? 'pointer' : 'default',
                            }}
                            onClick={() => {
                              if (!l.details) return;
                              setOperationLogs((prev) => {
                                const arr = prev[activeOrganizeOp.id] || [];
                                const updated = arr.map((item, i) =>
                                  i === idx
                                    ? { ...item, expanded: !item.expanded }
                                    : item
                                );
                                return {
                                  ...prev,
                                  [activeOrganizeOp.id]: updated,
                                };
                              });
                            }}
                          >
                            {l.message}
                          </Typography>
                          {l.details && l.expanded && (
                            <Typography
                              variant="caption"
                              sx={{ ml: 1.5, color: 'text.secondary' }}
                            >
                              {l.details}
                            </Typography>
                          )}
                        </Box>
                      ))}
                    </Box>
                  )}
                </Box>
              )}
              {activeScanOp && activeScanOp.status !== 'completed' && (
                <Box mt={1}>
                  <Stack direction="row" spacing={1} alignItems="center">
                    <Typography variant="caption" color="text.secondary">
                      Scanning: {activeScanOp.progress}/{activeScanOp.total}{' '}
                      {activeScanOp.message}
                    </Typography>
                    <Button
                      size="small"
                      variant="text"
                      onClick={() => api.cancelOperation(activeScanOp.id)}
                    >
                      Cancel
                    </Button>
                  </Stack>
                  {operationLogs[activeScanOp.id] && (
                    <Box
                      mt={0.5}
                      ref={(el: HTMLDivElement | null) => {
                        logContainerRefs.current[activeScanOp.id] = el;
                      }}
                      sx={{
                        maxHeight: 140,
                        overflowY: 'auto',
                        borderLeft: '2px solid',
                        borderColor: 'divider',
                        pl: 1,
                      }}
                    >
                      {operationLogs[activeScanOp.id].map((l, idx) => (
                        <Box key={idx} sx={{ mb: 0.3 }}>
                          <Typography
                            variant="caption"
                            display="block"
                            sx={{
                              color:
                                l.level === 'error'
                                  ? 'error.main'
                                  : l.level === 'warn'
                                    ? 'warning.main'
                                    : 'text.secondary',
                              fontWeight: l.level === 'error' ? 600 : 400,
                              cursor: l.details ? 'pointer' : 'default',
                            }}
                            onClick={() => {
                              if (!l.details) return;
                              setOperationLogs((prev) => {
                                const arr = prev[activeScanOp.id] || [];
                                const updated = arr.map((item, i) =>
                                  i === idx
                                    ? { ...item, expanded: !item.expanded }
                                    : item
                                );
                                return { ...prev, [activeScanOp.id]: updated };
                              });
                            }}
                          >
                            {l.message}
                          </Typography>
                          {l.details && l.expanded && (
                            <Typography
                              variant="caption"
                              sx={{ ml: 1.5, color: 'text.secondary' }}
                            >
                              {l.details}
                            </Typography>
                          )}
                        </Box>
                      ))}
                    </Box>
                  )}
                </Box>
              )}
              {/* Auto-scroll handled by top-level hook */}
            </Box>
            <Stack direction="row" spacing={2}>
              <Button
                variant="outlined"
                disabled={organizeRunning}
                onClick={handleOrganizeLibrary}
              >
                {organizeRunning ? 'Organizing…' : 'Organize Library'}
              </Button>
              <Button
                variant="outlined"
                startIcon={<RefreshIcon />}
                disabled={activeScanOp !== null}
                onClick={handleFullRescan}
              >
                {activeScanOp !== null ? 'Scanning…' : 'Full Rescan'}
              </Button>
              <Button
                variant="outlined"
                onClick={async () => {
                  try {
                    const status = await api.getSystemStatus();
                    setSystemStatus(status);
                  } catch (e) {
                    /* ignore refresh error */
                  }
                }}
              >
                Refresh Stats
              </Button>
            </Stack>
          </Stack>
        </Paper>
      )}

      <Box sx={{ flex: 1, overflowY: 'auto', minHeight: 0 }}>
        {audiobooks.length === 0 && !loading ? (
          <Paper
            sx={{ p: 4, textAlign: 'center', bgcolor: 'background.default' }}
          >
            <FolderOpenIcon
              sx={{ fontSize: 80, color: 'text.secondary', mb: 2 }}
            />
            <Alert severity="info" sx={{ textAlign: 'center' }}>
              <AlertTitle>No Audiobooks Found</AlertTitle>
              {importPaths.length === 0 ? (
                <>
                  You haven't added any import paths yet. Get started by:
                  <ul
                    style={{ marginTop: 8, marginBottom: 0, textAlign: 'left' }}
                  >
                    <li>
                      Importing individual audiobook files using the "Import
                      Files" button below
                    </li>
                    <li>
                      Adding import paths using the "Add Import Path" button
                      below (watches folders for new files)
                    </li>
                  </ul>
                </>
              ) : (
                <>
                  No audiobooks found in your library. Try:
                  <ul
                    style={{ marginTop: 8, marginBottom: 0, textAlign: 'left' }}
                  >
                    <li>
                      Scanning your import paths using the "Scan All" button
                      below
                    </li>
                    <li>
                      Adding more import paths where audiobooks are located
                    </li>
                  </ul>
                </>
              )}
            </Alert>
            <Box sx={{ mt: 3 }}>
              <Button
                variant="contained"
                size="large"
                startIcon={<UploadIcon />}
                onClick={handleManualImport}
                sx={{ mr: 2 }}
              >
                Import Files
              </Button>
              <Button
                variant="outlined"
                size="large"
                startIcon={<AddIcon />}
                onClick={() => setAddPathDialogOpen(true)}
                sx={{ mr: 2 }}
              >
                Add Import Path
              </Button>
              {importPaths.length > 0 && (
                <Button
                  variant="outlined"
                  size="large"
                  startIcon={<RefreshIcon />}
                  onClick={handleScanAll}
                >
                  Scan All
                </Button>
              )}
            </Box>
          </Paper>
        ) : (
          <Stack spacing={3}>
            <SearchBar
              value={searchQuery}
              onChange={setSearchQuery}
              viewMode={viewMode}
              onViewModeChange={setViewMode}
              sortBy={sortBy}
              onSortChange={handleSortChange}
              sortOrder={sortOrder}
              onSortOrderChange={setSortOrder}
            />

            <Paper sx={{ p: 2 }}>
              <Stack
                direction={{ xs: 'column', md: 'row' }}
                spacing={2}
                alignItems={{ xs: 'flex-start', md: 'center' }}
                justifyContent="space-between"
              >
                <Stack direction="row" spacing={2} alignItems="center">
                  <FormControlLabel
                    control={
                      <Checkbox
                        checked={allOnPageSelected}
                        indeterminate={someOnPageSelected && !allOnPageSelected}
                        onChange={handleToggleSelectAllOnPage}
                      />
                    }
                    label="Select All"
                  />
                  <Chip label={`${selectedAudiobooks.length} selected`} />
                  <Button
                    size="small"
                    variant="text"
                    onClick={handleClearSelection}
                    disabled={!hasSelection}
                  >
                    Deselect All
                  </Button>
                </Stack>

                <Stack
                  direction={{ xs: 'column', sm: 'row' }}
                  spacing={1}
                  alignItems={{ xs: 'flex-start', sm: 'center' }}
                >
                  <Tooltip
                    title={!hasSelection ? 'Select books first' : ''}
                    disableHoverListener={hasSelection}
                  >
                    <span>
                      <Button
                        size="small"
                        variant="outlined"
                        onClick={() => setBatchEditOpen(true)}
                        disabled={!hasSelection}
                      >
                        Batch Edit
                      </Button>
                    </span>
                  </Tooltip>
                  <Tooltip
                    title={!hasSelection ? 'Select books first' : ''}
                    disableHoverListener={hasSelection}
                  >
                    <span>
                      <Button
                        size="small"
                        variant="outlined"
                        onClick={() => setBulkFetchDialogOpen(true)}
                        disabled={!hasSelection}
                      >
                        Fetch Metadata
                      </Button>
                    </span>
                  </Tooltip>
                  <Tooltip
                    title={
                      !selectedHasImport
                        ? hasSelection
                          ? 'Select import books first'
                          : 'Select books first'
                        : ''
                    }
                    disableHoverListener={selectedHasImport}
                  >
                    <span>
                      <Button
                        size="small"
                        variant="outlined"
                        color="primary"
                        onClick={() => setBulkOrganizeDialogOpen(true)}
                        disabled={!selectedHasImport}
                      >
                        Organize Selected
                      </Button>
                    </span>
                  </Tooltip>
                  <Tooltip
                    title={
                      !selectedHasActive
                        ? hasSelection
                          ? 'Select active books first'
                          : 'Select books first'
                        : ''
                    }
                    disableHoverListener={selectedHasActive}
                  >
                    <span>
                      <Button
                        size="small"
                        variant="outlined"
                        color="secondary"
                        onClick={() => setBatchDeleteDialogOpen(true)}
                        disabled={!selectedHasActive}
                      >
                        Delete Selected
                      </Button>
                    </span>
                  </Tooltip>
                  <Tooltip
                    title={
                      !selectedHasDeleted
                        ? hasSelection
                          ? 'Select deleted books first'
                          : 'Select books first'
                        : ''
                    }
                    disableHoverListener={selectedHasDeleted}
                  >
                    <span>
                      <Button
                        size="small"
                        variant="outlined"
                        color="success"
                        onClick={handleBatchRestore}
                        disabled={!selectedHasDeleted || batchRestoreInProgress}
                      >
                        {batchRestoreInProgress
                          ? 'Restoring...'
                          : 'Restore Selected'}
                      </Button>
                    </span>
                  </Tooltip>
                </Stack>
              </Stack>
            </Paper>

            {viewMode === 'grid' ? (
              <AudiobookGrid
                audiobooks={audiobooks}
                loading={loading}
                onEdit={handleEdit}
                onDelete={handleDelete}
                onClick={handleClick}
                onVersionManage={handleVersionManage}
                onFetchMetadata={handleFetchMetadata}
                onParseWithAI={handleParseWithAI}
                selectedIds={selectedIds}
                onToggleSelect={handleToggleSelect}
              />
            ) : (
              <AudiobookList
                audiobooks={audiobooks}
                loading={loading}
                onEdit={handleEdit}
                onDelete={handleDelete}
                onClick={handleClick}
                selectedIds={selectedIds}
                onToggleSelect={handleToggleSelect}
                onSelectAll={handleToggleSelectAllOnPage}
              />
            )}

            {!loading && (
              <Stack
                direction={{ xs: 'column', sm: 'row' }}
                spacing={2}
                alignItems="center"
                justifyContent="center"
                mt={4}
              >
                <TextField
                  select
                  size="small"
                  label="Items per page"
                  value={itemsPerPage}
                  onChange={(e) => setItemsPerPage(Number(e.target.value))}
                  sx={{ minWidth: 150 }}
                >
                  <MenuItem value={20}>20</MenuItem>
                  <MenuItem value={50}>50</MenuItem>
                  <MenuItem value={100}>100</MenuItem>
                </TextField>
                {totalPages > 1 && (
                  <Pagination
                    count={totalPages}
                    page={page}
                    onChange={(_, value) => setPage(value)}
                    color="primary"
                  />
                )}
              </Stack>
            )}
          </Stack>
        )}

        <Paper sx={{ p: 2, mt: 3 }}>
          <Stack
            direction="row"
            alignItems="center"
            justifyContent="space-between"
            spacing={2}
            flexWrap="wrap"
            rowGap={1}
          >
            <Box>
              <Typography variant="h6">Soft-Deleted Books</Typography>
              <Typography variant="body2" color="text.secondary">
                Review recently deleted items before purging them permanently.
              </Typography>
            </Box>
            <Stack direction="row" spacing={1} alignItems="center">
              <Chip
                label={`${softDeletedCount} ${softDeletedCount === 1 ? 'item' : 'items'}`}
                color={softDeletedCount > 0 ? 'warning' : 'default'}
              />
              <Button
                size="small"
                variant="outlined"
                startIcon={<RefreshIcon />}
                onClick={loadSoftDeleted}
                disabled={softDeletedLoading}
              >
                {softDeletedLoading ? 'Refreshing...' : 'Refresh'}
              </Button>
            </Stack>
          </Stack>
          {softDeletedLoading ? (
            <Typography variant="body2" sx={{ mt: 2 }}>
              Loading soft-deleted books...
            </Typography>
          ) : softDeletedBooks.length === 0 ? (
            <Alert severity="info" sx={{ mt: 2 }}>
              No soft-deleted books at the moment.
            </Alert>
          ) : (
            <List dense sx={{ mt: 1 }}>
              {softDeletedBooks.map((book) => {
                const deletedAt =
                  book.marked_for_deletion_at &&
                  new Date(book.marked_for_deletion_at);
                return (
                  <ListItem key={book.id} alignItems="flex-start">
                    <ListItemText
                      primary={book.title || 'Untitled'}
                      secondary={
                        <Stack spacing={0.5}>
                          <Typography variant="body2" color="text.secondary">
                            {book.author || 'Unknown Author'}
                          </Typography>
                          {deletedAt && (
                            <Typography
                              variant="caption"
                              color="text.secondary"
                            >
                              Soft deleted at {deletedAt.toLocaleString()}
                            </Typography>
                          )}
                          {book.file_path && (
                            <Typography
                              variant="caption"
                              color="text.secondary"
                            >
                              {book.file_path}
                            </Typography>
                          )}
                        </Stack>
                      }
                    />
                    <ListItemSecondaryAction>
                      <Button
                        size="small"
                        variant="outlined"
                        sx={{ mr: 1 }}
                        onClick={() => handleRestoreOne(book)}
                        disabled={
                          restoringBookId === book.id ||
                          purgeInProgress ||
                          purgingBookId === book.id
                        }
                      >
                        {restoringBookId === book.id
                          ? 'Restoring...'
                          : 'Restore'}
                      </Button>
                      <Button
                        size="small"
                        color="error"
                        variant="outlined"
                        onClick={() => handlePurgeOne(book)}
                        disabled={purgingBookId === book.id || purgeInProgress}
                      >
                        {purgingBookId === book.id ? 'Purging...' : 'Purge now'}
                      </Button>
                    </ListItemSecondaryAction>
                  </ListItem>
                );
              })}
            </List>
          )}
        </Paper>

        <FilterSidebar
          open={filterOpen}
          onClose={() => setFilterOpen(false)}
          filters={filters}
          onFiltersChange={handleFiltersChange}
          authors={availableAuthors}
          series={availableSeries}
          genres={availableGenres}
          languages={availableLanguages}
        />

        <MetadataEditDialog
          open={!!editingAudiobook}
          audiobook={editingAudiobook}
          onClose={() => setEditingAudiobook(null)}
          onSave={handleSaveMetadata}
        />

        <BatchEditDialog
          open={batchEditOpen}
          audiobooks={selectedAudiobooks}
          onClose={() => setBatchEditOpen(false)}
          onSave={handleBatchSave}
        />

        <Dialog
          open={batchDeleteDialogOpen}
          onClose={() => setBatchDeleteDialogOpen(false)}
        >
          <DialogTitle>Delete Selected Audiobooks</DialogTitle>
          <DialogContent>
            <Typography variant="body1" gutterBottom>
              Are you sure you want to soft delete{' '}
              {selectedAudiobooks.length} selected audiobooks?
            </Typography>
            <Alert severity="warning">
              Selected books will be hidden from the library and can be restored
              from the soft-deleted list.
            </Alert>
          </DialogContent>
          <DialogActions>
            <Button
              onClick={() => setBatchDeleteDialogOpen(false)}
              disabled={batchDeleteInProgress}
            >
              Cancel
            </Button>
            <Button
              variant="contained"
              color="secondary"
              onClick={handleBatchDelete}
              disabled={batchDeleteInProgress}
            >
              {batchDeleteInProgress ? 'Deleting...' : 'Delete Selected'}
            </Button>
          </DialogActions>
        </Dialog>

        <Dialog
          open={bulkOrganizeDialogOpen}
          onClose={handleCancelBulkOrganize}
        >
          <DialogTitle>Organize Selected Audiobooks</DialogTitle>
          <DialogContent>
            <Typography variant="body1" gutterBottom>
              Organize {selectedAudiobooks.length} selected books.
            </Typography>
            {bulkOrganizeProgress && (
              <Box sx={{ mt: 2 }}>
                <Typography variant="body2" gutterBottom>
                  Organized {bulkOrganizeProgress.completed} of{' '}
                  {bulkOrganizeProgress.total}
                </Typography>
                <LinearProgress
                  variant="determinate"
                  value={
                    bulkOrganizeProgress.total > 0
                      ? (bulkOrganizeProgress.completed /
                          bulkOrganizeProgress.total) *
                        100
                      : 0
                  }
                />
                {bulkOrganizeProgress.results.length > 0 && (
                  <List dense sx={{ mt: 2 }}>
                    {bulkOrganizeProgress.results.map((result) => (
                      <ListItem key={result.book_id}>
                        <ListItemText
                          primary={result.title || result.book_id}
                          secondary={getResultLabel(result)}
                        />
                      </ListItem>
                    ))}
                  </List>
                )}
              </Box>
            )}
          </DialogContent>
          <DialogActions>
            <Button onClick={handleCancelBulkOrganize}>
              {bulkOrganizeInProgress ? 'Cancel' : 'Close'}
            </Button>
            <Button
              variant="contained"
              onClick={handleBulkOrganize}
              disabled={bulkOrganizeInProgress || !selectedHasImport}
            >
              {bulkOrganizeInProgress ? 'Organizing…' : 'Organize Selected'}
            </Button>
          </DialogActions>
        </Dialog>

        <Dialog
          open={Boolean(duplicateDialog)}
          onClose={() => handleDuplicateAction('skip')}
        >
          <DialogTitle>Duplicate File Detected</DialogTitle>
          <DialogContent>
            <Typography variant="body2" gutterBottom>
              The file for{' '}
              <strong>
                {duplicateDialog?.duplicate.title || 'this audiobook'}
              </strong>{' '}
              matches an existing audiobook.
            </Typography>
            <Typography variant="body2" color="text.secondary">
              Existing:{' '}
              {duplicateDialog?.existing.title || 'Unknown audiobook'}
            </Typography>
          </DialogContent>
          <DialogActions>
            <Button onClick={() => handleDuplicateAction('skip')}>
              Skip
            </Button>
            <Button onClick={() => handleDuplicateAction('link')}>
              Link as Version
            </Button>
            <Button
              color="warning"
              variant="contained"
              onClick={() => handleDuplicateAction('replace')}
            >
              Replace
            </Button>
          </DialogActions>
        </Dialog>

        <Dialog
          open={Boolean(bulkOrganizeError)}
          onClose={handleCloseOrganizeError}
        >
          <DialogTitle>Organize Error</DialogTitle>
          <DialogContent>
            <Typography variant="body2" gutterBottom>
              {bulkOrganizeError?.message ||
                'Organize operation failed.'}
            </Typography>
          </DialogContent>
          <DialogActions>
            <Button onClick={handleCloseOrganizeError}>Close</Button>
            <Button
              color="warning"
              variant="contained"
              onClick={handleOrganizeRollback}
            >
              Rollback
            </Button>
          </DialogActions>
        </Dialog>

        <Dialog
          open={importFileDialogOpen}
          onClose={() => setImportFileDialogOpen(false)}
          maxWidth="md"
          fullWidth
        >
          <DialogTitle>Import Audiobook File</DialogTitle>
          <DialogContent>
            <Alert severity="info" sx={{ mb: 2 }}>
              Select a file on the server to import into the library. Use the
              organize toggle to immediately move it into the library layout.
            </Alert>
            <TextField
              fullWidth
              label="Import file path"
              value={importFilePath}
              onChange={(e) => setImportFilePath(e.target.value)}
              placeholder="/path/to/audiobook.m4b"
              sx={{ mb: 2 }}
            />
            <ServerFileBrowser
              initialPath="/"
              showFiles
              allowDirSelect={false}
              allowFileSelect
              onSelect={(path, isDir) => {
                if (!isDir) {
                  setImportFilePath(path);
                }
              }}
            />
            <FormControlLabel
              control={
                <Checkbox
                  checked={importFileOrganize}
                  onChange={(e) => setImportFileOrganize(e.target.checked)}
                />
              }
              label="Organize into library after import"
            />
          </DialogContent>
          <DialogActions>
            <Button
              onClick={() => setImportFileDialogOpen(false)}
              disabled={importFileInProgress}
            >
              Cancel
            </Button>
            <Button
              variant="contained"
              onClick={handleImportFile}
              disabled={importFileInProgress}
            >
              {importFileInProgress ? 'Importing…' : 'Import'}
            </Button>
          </DialogActions>
        </Dialog>

        <Dialog
          open={bulkFetchDialogOpen}
          onClose={handleCancelBulkFetch}
        >
          <DialogTitle>Bulk Fetch Metadata</DialogTitle>
          <DialogContent>
            <Typography variant="body1" gutterBottom>
              Fetch metadata for {selectedAudiobooks.length} selected books.
            </Typography>
            {bulkFetchProgress && (
              <Box sx={{ mt: 2 }}>
                <Typography variant="body2" gutterBottom>
                  {bulkFetchProgress.completed} / {bulkFetchProgress.total}{' '}
                  completed
                </Typography>
                <LinearProgress
                  variant="determinate"
                  value={
                    bulkFetchProgress.total > 0
                      ? (bulkFetchProgress.completed /
                          bulkFetchProgress.total) *
                        100
                      : 0
                  }
                />
                {bulkFetchProgress.results.length > 0 && (
                  <List dense sx={{ mt: 2 }}>
                    {bulkFetchProgress.results.map((result) => (
                      <ListItem key={result.book_id}>
                        <ListItemText
                          primary={result.title || result.book_id}
                          secondary={getResultLabel(result)}
                        />
                      </ListItem>
                    ))}
                  </List>
                )}
              </Box>
            )}
          </DialogContent>
          <DialogActions>
            <Button
              onClick={handleCancelBulkFetch}
            >
              {bulkFetchInProgress ? 'Cancel' : 'Close'}
            </Button>
            <Button
              variant="contained"
              onClick={handleBulkFetchMetadata}
              disabled={bulkFetchInProgress || !hasSelection}
            >
              {bulkFetchInProgress ? 'Fetching…' : 'Fetch Metadata'}
            </Button>
          </DialogActions>
        </Dialog>

        <VersionManagement
          audiobookId={versionManagingAudiobook?.id || ''}
          open={versionManagementOpen}
          onClose={handleVersionManagementClose}
          onUpdate={handleVersionUpdate}
        />

        <Dialog open={deleteDialogOpen} onClose={handleCloseDeleteDialog}>
          <DialogTitle>Delete Audiobook</DialogTitle>
          <DialogContent>
            <Typography variant="body1" gutterBottom>
              {bookPendingDelete
                ? `Are you sure you want to delete "${bookPendingDelete.title}"?`
                : 'Are you sure you want to delete this audiobook?'}
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
              label="Soft delete (hide from library, keep for purge)"
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
              Soft deleting keeps the record for auditing and purging. Use purge
              to permanently remove it later.
            </Alert>
          </DialogContent>
          <DialogActions>
            <Button onClick={handleCloseDeleteDialog}>Cancel</Button>
            <Button
              onClick={handleConfirmDelete}
              color="error"
              variant="contained"
              disabled={deleteInProgress}
            >
              {deleteInProgress
                ? 'Deleting...'
                : deleteOptions.softDelete
                  ? 'Soft Delete'
                  : 'Delete'}
            </Button>
          </DialogActions>
        </Dialog>

        <Dialog
          open={purgeDialogOpen}
          onClose={() => {
            setPurgeDialogOpen(false);
            setPurgeDeleteFiles(false);
          }}
        >
          <DialogTitle>Purge Soft-Deleted Books</DialogTitle>
          <DialogContent>
            <Typography variant="body1" gutterBottom>
              {softDeletedCount === 0
                ? 'There are no soft-deleted books to purge.'
                : `This will permanently remove ${softDeletedCount} soft-deleted ${
                    softDeletedCount === 1 ? 'book' : 'books'
                  } from the library.`}
            </Typography>
            <FormControlLabel
              control={
                <Checkbox
                  checked={purgeDeleteFiles}
                  onChange={(e) => setPurgeDeleteFiles(e.target.checked)}
                />
              }
              label="Also delete files from disk (if they still exist)"
            />
            <Alert severity="warning" sx={{ mt: 2 }}>
              This cannot be undone. Purge removes the records entirely and
              deletes files when selected.
            </Alert>
          </DialogContent>
          <DialogActions>
            <Button
              onClick={() => {
                setPurgeDialogOpen(false);
                setPurgeDeleteFiles(false);
              }}
            >
              Cancel
            </Button>
            <Button
              onClick={handleConfirmPurge}
              color="error"
              variant="contained"
              disabled={purgeInProgress || softDeletedCount === 0}
            >
              {purgeInProgress ? 'Purging...' : 'Purge Now'}
            </Button>
          </DialogActions>
        </Dialog>

        {/* Import Path Management Dialog */}
        <Dialog
          open={addPathDialogOpen}
          onClose={() => setAddPathDialogOpen(false)}
          maxWidth="md"
          fullWidth
        >
          <DialogTitle>Add Import Folder (Watch Location)</DialogTitle>
          <DialogContent>
            <Alert severity="info" sx={{ mb: 2 }}>
              <strong>Import folders</strong> are watch locations where the
              scanner looks for new audiobooks. Files discovered here will be
              copied and organized into your main library path (configured in
              Settings).
            </Alert>

            {!showServerBrowser ? (
              <Box>
                <TextField
                  autoFocus
                  fullWidth
                  label="Import Path"
                  value={newImportPath}
                  onChange={(e) => setNewImportPath(e.target.value)}
                  placeholder="/path/to/downloads"
                  sx={{ mt: 1 }}
                />
                <Button
                  startIcon={<FolderOpenIcon />}
                  onClick={() => setShowServerBrowser(true)}
                  sx={{ mt: 2 }}
                >
                  Browse Server Filesystem
                </Button>
              </Box>
            ) : (
              <Box>
                <Button
                  onClick={() => setShowServerBrowser(false)}
                  sx={{ mb: 2 }}
                >
                  ← Back to Manual Entry
                </Button>
                <ServerFileBrowser
                  initialPath={newImportPath || '/'}
                  onSelect={handleServerBrowserSelect}
                  showFiles={false}
                  allowDirSelect={true}
                  allowFileSelect={false}
                />
                {newImportPath && (
                  <Alert severity="success" sx={{ mt: 2 }}>
                    <Typography variant="body2">
                      <strong>Selected:</strong> {newImportPath}
                    </Typography>
                  </Alert>
                )}
              </Box>
            )}
          </DialogContent>
          <DialogActions>
            <Button
              onClick={() => {
                setAddPathDialogOpen(false);
                setShowServerBrowser(false);
              }}
            >
              Cancel
            </Button>
            <Button
              onClick={handleAddImportPath}
              variant="contained"
              disabled={!newImportPath.trim()}
            >
              Add Path
            </Button>
          </DialogActions>
        </Dialog>

        {/* Import Paths List */}
        {importPaths.length > 0 && (
          <Paper sx={{ mt: 2 }}>
            <Box
              sx={{
                p: 2,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                cursor: 'pointer',
              }}
              onClick={() => setImportPathsExpanded(!importPathsExpanded)}
            >
              <Typography variant="h6">
                Import Paths ({importPaths.length})
              </Typography>
              <Stack direction="row" spacing={1} alignItems="center">
                <Button
                  size="small"
                  variant="outlined"
                  onClick={(e) => {
                    e.stopPropagation();
                    handleScanAll();
                  }}
                  disabled={
                    importPaths.length === 0 ||
                    importPaths.some((p) => p.status === 'scanning')
                  }
                >
                  Scan All
                </Button>
                <IconButton
                  size="small"
                  onClick={(e) => {
                    e.stopPropagation();
                    setImportPathsExpanded(!importPathsExpanded);
                  }}
                >
                  <ExpandMoreIcon
                    sx={{
                      transform: importPathsExpanded
                        ? 'rotate(180deg)'
                        : 'rotate(0deg)',
                      transition: 'transform 0.3s',
                    }}
                  />
                </IconButton>
              </Stack>
            </Box>
            <Collapse in={importPathsExpanded}>
              <List>
                {importPaths.map((path) => (
                  <ListItem key={path.id}>
                    <ListItemText
                      primary={path.path}
                      secondary={
                        path.status === 'scanning'
                          ? 'Scanning...'
                          : `${path.book_count} books found`
                      }
                    />
                    <ListItemSecondaryAction>
                      <IconButton
                        edge="end"
                        onClick={() => handleScanImportPath(path.id)}
                        disabled={path.status === 'scanning'}
                        sx={{ mr: 1 }}
                      >
                        <RefreshIcon />
                      </IconButton>
                      <IconButton
                        edge="end"
                        onClick={() => handleRemoveImportPath(path.id)}
                      >
                        <DeleteIcon />
                      </IconButton>
                    </ListItemSecondaryAction>
                  </ListItem>
                ))}
              </List>
            </Collapse>
          </Paper>
        )}
      </Box>
    </Box>
  );
};
