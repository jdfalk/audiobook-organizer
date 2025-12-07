// file: web/src/pages/Library.tsx
// version: 1.18.0
// guid: 3f4a5b6c-7d8e-9f0a-1b2c-3d4e5f6a7b8c

import { useState, useEffect, useCallback, useRef } from 'react';
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
  Collapse,
  List,
  ListItem,
  ListItemText,
  ListItemSecondaryAction,
} from '@mui/material';
import {
  FilterList as FilterListIcon,
  Upload as UploadIcon,
  FolderOpen as FolderOpenIcon,
  Add as AddIcon,
  Delete as DeleteIcon,
  ExpandMore as ExpandMoreIcon,
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
import * as api from '../services/api';
import { pollOperation } from '../utils/operationPolling';

interface ImportPath {
  id: number;
  path: string;
  status: 'idle' | 'scanning';
  book_count: number;
}

export const Library = () => {
  const [audiobooks, setAudiobooks] = useState<Audiobook[]>([]);
  const [loading, setLoading] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const [debouncedSearch, setDebouncedSearch] = useState('');
  const [viewMode, setViewMode] = useState<ViewMode>('grid');
  const [sortBy, setSortBy] =
    useState<import('../components/audiobooks/SearchBar').SortOption>('title');
  const [filterOpen, setFilterOpen] = useState(false);
  const [filters, setFilters] = useState<FilterOptions>({});
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
  const [editingAudiobook, setEditingAudiobook] = useState<Audiobook | null>(
    null
  );
  const [selectedAudiobooks, setSelectedAudiobooks] = useState<Audiobook[]>([]);
  const [batchEditOpen, setBatchEditOpen] = useState(false);
  const [versionManagementOpen, setVersionManagementOpen] = useState(false);
  const [versionManagingAudiobook, setVersionManagingAudiobook] =
    useState<Audiobook | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

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
  const eventSourceRef = useRef<EventSource | null>(null);
  const reconnectAttempts = useRef(0);

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

    const setupEventSource = () => {
      if (eventSourceRef.current) {
        eventSourceRef.current.close();
      }

      const es = new EventSource('/api/events');
      eventSourceRef.current = es;

      es.onmessage = (ev) => {
        try {
          const evt = JSON.parse(ev.data);
          if (!evt || !evt.type) return;
          if (evt.type === 'heartbeat') return; // Ignore heartbeat messages

          if (evt.type === 'operation.log') {
            const opId = evt.data.operation_id;
            setOperationLogs((prev) => {
              const existing = prev[opId] || [];
              const next = [
                ...existing,
                {
                  level: evt.data.level,
                  message: evt.data.message,
                  details: evt.data.details,
                  timestamp: Date.now(),
                },
              ];
              return { ...prev, [opId]: next.slice(-200) };
            });
          } else if (evt.type === 'operation.progress') {
            const opId = evt.data.operation_id;
            const update = (op: api.Operation | null): api.Operation | null => {
              if (!op || op.id !== opId) return op;
              return {
                ...op,
                progress: evt.data.current,
                total: evt.data.total,
                message: evt.data.message,
              };
            };
            setActiveScanOp((prev) => update(prev));
            setActiveOrganizeOp((prev) => update(prev));
          } else if (evt.type === 'operation.status') {
            const opId = evt.data.operation_id;
            const status = evt.data.status;
            const finalize = (
              op: api.Operation | null
            ): api.Operation | null => {
              if (!op || op.id !== opId) return op;
              return { ...op, status };
            };
            setActiveScanOp((prev) => finalize(prev));
            setActiveOrganizeOp((prev) => finalize(prev));
          }
        } catch {
          // ignore parse errors
        }
      };

      es.onerror = () => {
        es.close();
        reconnectAttempts.current++;
        // Exponential backoff: 3s, 6s, 12s, 24s, capped at 30s
        const delay = Math.min(3000 * Math.pow(2, reconnectAttempts.current - 1), 30000);
        console.warn(`EventSource connection lost (attempt ${reconnectAttempts.current}), reconnecting in ${delay/1000}s...`);
        setTimeout(setupEventSource, delay);
      };

      es.onopen = () => {
        console.log('EventSource connection established');
        reconnectAttempts.current = 0; // Reset on successful connection
      };
    };

    setupEventSource();
    return () => {
      if (eventSourceRef.current) {
        eventSourceRef.current.close();
        eventSourceRef.current = null;
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

  const loadAudiobooks = useCallback(async () => {
    setLoading(true);
    try {
      const limit = 24;
      const offset = (page - 1) * limit;

      // Fetch audiobooks and import paths
      const [books, folders, bookCount] = await Promise.all([
        debouncedSearch
          ? api.searchBooks(debouncedSearch, limit)
          : api.getBooks(limit, offset),
        api.getImportPaths(),
        debouncedSearch ? Promise.resolve(0) : api.countBooks(),
      ]);

      // Convert API books to Audiobook type
      const convertedBooks: Audiobook[] = books.map((book) => ({
        id: book.id,
        title: book.title,
        author: book.author_name || 'Unknown',
        narrator: book.narrator,
        series: book.series_name,
        seriesPosition: book.series_position,
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
      }));

      // Apply client-side sorting
      const sortedBooks = [...convertedBooks].sort((a, b) => {
        switch (sortBy) {
          case 'title':
            return a.title.localeCompare(b.title);
          case 'author':
            return (a.author || '').localeCompare(b.author || '');
          case 'date_added':
            return (
              new Date(b.created_at).getTime() -
              new Date(a.created_at).getTime()
            );
          case 'date_modified':
            return (
              new Date(b.updated_at).getTime() -
              new Date(a.updated_at).getTime()
            );
          default:
            return 0;
        }
      });

      setAudiobooks(sortedBooks);
      setTotalPages(
        Math.ceil((debouncedSearch ? books.length : bookCount) / limit)
      );

      // Load import paths
      const convertedPaths: ImportPath[] = folders.map((folder) => ({
        id: folder.id,
        path: folder.path,
        status: 'idle',
        book_count: folder.book_count,
      }));
      setImportPaths(convertedPaths);
    } catch (error) {
      console.error('Failed to load audiobooks:', error);
      setAudiobooks([]);
      setTotalPages(1);
    } finally {
      setLoading(false);
    }
  }, [debouncedSearch, page, sortBy]);

  const handleManualImport = () => {
    fileInputRef.current?.click();
  };

  const handleFileSelect = async (
    event: React.ChangeEvent<HTMLInputElement>
  ) => {
    const files = event.target.files;
    if (!files || files.length === 0) return;

    // TODO: Implement file upload logic
    console.log('Selected files:', files);
    // const formData = new FormData();
    // for (let i = 0; i < files.length; i++) {
    //   formData.append('files', files[i]);
    // }
    // await fetch('/api/v1/audiobooks/import', {
    //   method: 'POST',
    //   body: formData
    // });
    // loadAudiobooks();
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

  const handleEdit = useCallback((audiobook: Audiobook) => {
    setEditingAudiobook(audiobook);
  }, []);

  const handleDelete = useCallback((audiobook: Audiobook) => {
    console.log('Delete audiobook:', audiobook);
    // TODO: Implement delete confirmation
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
      setSelectedAudiobooks([]);
      setBatchEditOpen(false);
    } catch (error) {
      console.error('Failed to batch update audiobooks:', error);
      throw error;
    }
  };

  const handleClick = useCallback((audiobook: Audiobook) => {
    console.log('View audiobook:', audiobook);
    // TODO: Navigate to audiobook detail page
  }, []);

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

  const getActiveFilterCount = () => {
    return Object.values(filters).filter((v) => v !== undefined && v !== '')
      .length;
  };

  const libraryBookCount =
    systemStatus?.library_book_count ??
    systemStatus?.library.book_count ??
    0;
  const importBookCount =
    systemStatus?.import_book_count ??
    systemStatus?.import_paths?.book_count ??
    0;
  const librarySizeBytes =
    systemStatus?.library_size_bytes ??
    systemStatus?.library.total_size ??
    0;
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

      {/* Hidden file input for manual import */}
      <input
        ref={fileInputRef}
        type="file"
        multiple
        accept="audio/*,.m4b,.mp3,.m4a,.flac,.opus,.ogg"
        style={{ display: 'none' }}
        onChange={handleFileSelect}
      />

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
                  <ul style={{ marginTop: 8, marginBottom: 0, textAlign: 'left' }}>
                    <li>
                      Importing individual audiobook files using the "Import Files"
                      button below
                    </li>
                    <li>
                      Adding import paths using the "Add Import Path" button below
                      (watches folders for new files)
                    </li>
                  </ul>
                </>
              ) : (
                <>
                  No audiobooks found in your library. Try:
                  <ul style={{ marginTop: 8, marginBottom: 0, textAlign: 'left' }}>
                    <li>Scanning your import paths using the "Scan All" button below</li>
                    <li>Adding more import paths where audiobooks are located</li>
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
              onSortChange={setSortBy}
            />

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
              />
            ) : (
              <AudiobookList
                audiobooks={audiobooks}
                loading={loading}
                onEdit={handleEdit}
                onDelete={handleDelete}
                onClick={handleClick}
              />
            )}

            {!loading && totalPages > 1 && (
              <Box display="flex" justifyContent="center" mt={4}>
                <Pagination
                  count={totalPages}
                  page={page}
                  onChange={(_, value) => setPage(value)}
                  color="primary"
                />
              </Box>
            )}
          </Stack>
        )}

        <FilterSidebar
          open={filterOpen}
          onClose={() => setFilterOpen(false)}
          filters={filters}
          onFiltersChange={handleFiltersChange}
          authors={[]} // TODO: Load from API
          series={[]} // TODO: Load from API
          genres={[]} // TODO: Load from API
          languages={[]} // TODO: Load from API
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

        <VersionManagement
          audiobookId={versionManagingAudiobook?.id || ''}
          open={versionManagementOpen}
          onClose={handleVersionManagementClose}
          onUpdate={handleVersionUpdate}
        />

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
