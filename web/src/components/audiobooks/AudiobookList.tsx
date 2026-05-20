// file: web/src/components/audiobooks/AudiobookList.tsx
// version: 2.3.0
// guid: 0c1d2e3f-4a5b-6c7d-8e9f-0a1b2c3d4e5f
// last-edited: 2026-05-19

import React, { useCallback, useEffect, useRef, useState, memo } from 'react';
import {
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  IconButton,
  Menu,
  MenuItem,
  Typography,
  Box,
  CircularProgress,
  Checkbox,
  Chip,
  Collapse,
} from '@mui/material';
import {
  MoreVert as MoreVertIcon,
  Edit as EditIcon,
  Delete as DeleteIcon,
  ArrowUpward as ArrowUpwardIcon,
  ArrowDownward as ArrowDownwardIcon,
  ExpandMore as ExpandMoreIcon,
  ExpandLess as ExpandLessIcon,
  CheckCircle as CheckCircleIcon,
  Cancel as CancelIcon,
} from '@mui/icons-material';
import type { Audiobook, BookFile } from '../../types';
import { type ColumnDefinition, getDefaultVisibleColumns } from '../../config/columnDefinitions';
import * as api from '../../services/api';

interface AudiobookListProps {
  audiobooks: Audiobook[];
  loading?: boolean;
  onEdit?: (audiobook: Audiobook) => void;
  onDelete?: (audiobook: Audiobook) => void;
  onClick?: (audiobook: Audiobook) => void;
  selectedIds?: Set<string>;
  onToggleSelect?: (audiobook: Audiobook, event?: React.MouseEvent) => void;
  onSelectAll?: () => void;
  columns?: ColumnDefinition[];
  columnWidths?: Record<string, number>;
  sortBy?: string;
  sortOrder?: 'asc' | 'desc';
  onSortChange?: (sortKey: string, order: 'asc' | 'desc') => void;
  onColumnResize?: (columnId: string, width: number) => void;
}

const fallbackColumns = getDefaultVisibleColumns();

export const AudiobookList: React.FC<AudiobookListProps> = ({
  audiobooks,
  loading = false,
  onEdit,
  onDelete,
  onClick,
  selectedIds,
  onToggleSelect,
  onSelectAll,
  columns,
  columnWidths,
  sortBy,
  sortOrder = 'asc',
  onSortChange,
  onColumnResize,
}) => {
  const activeColumns = columns ?? fallbackColumns;

  const [anchorEls, setAnchorEls] = React.useState<Record<string, HTMLElement | null>>({});
  const [expandedRows, setExpandedRows] = useState<Record<string, boolean>>({});
  const [filesCache, setFilesCache] = useState<Record<string, BookFile[]>>({});
  const [loadingFiles, setLoadingFiles] = useState<Record<string, boolean>>({});
  const [fetchErrors, setFetchErrors] = useState<Record<string, boolean>>({});
  const abortControllersRef = useRef<Record<string, AbortController>>({});

  // --- Resize state (non-React for perf during drag) ---
  const resizingRef = useRef<{
    columnId: string;
    startX: number;
    startWidth: number;
    minWidth: number;
  } | null>(null);
  const cleanupRef = useRef<(() => void) | null>(null);

  // Cleanup resize listeners on unmount
  useEffect(() => {
    return () => {
      cleanupRef.current?.();
    };
  }, []);

  const handleMenuClick = (event: React.MouseEvent<HTMLElement>, id: string) => {
    event.stopPropagation();
    setAnchorEls((prev) => ({ ...prev, [id]: event.currentTarget }));
  };

  const handleClose = (id: string) => {
    setAnchorEls((prev) => ({ ...prev, [id]: null }));
  };

  const handleEdit = (audiobook: Audiobook) => {
    handleClose(audiobook.id);
    onEdit?.(audiobook);
  };

  const handleDelete = (audiobook: Audiobook) => {
    handleClose(audiobook.id);
    onDelete?.(audiobook);
  };

  const handleRowClick = (audiobook: Audiobook) => {
    onClick?.(audiobook);
  };

  const handleSortClick = useCallback(
    (col: ColumnDefinition) => {
      if (!col.sortable || !onSortChange) return;
      const newOrder = sortBy === col.sortKey && sortOrder === 'asc' ? 'desc' : 'asc';
      onSortChange(col.sortKey, newOrder);
    },
    [sortBy, sortOrder, onSortChange]
  );

  const handleResizeStart = useCallback(
    (e: React.MouseEvent, col: ColumnDefinition) => {
      e.preventDefault();
      e.stopPropagation();
      const currentWidth = columnWidths?.[col.id] ?? col.defaultWidth;
      resizingRef.current = {
        columnId: col.id,
        startX: e.clientX,
        startWidth: currentWidth,
        minWidth: col.minWidth,
      };

      const handleMouseMove = (moveEvt: MouseEvent) => {
        if (!resizingRef.current) return;
        const delta = moveEvt.clientX - resizingRef.current.startX;
        const newWidth = Math.max(
          resizingRef.current.minWidth,
          resizingRef.current.startWidth + delta
        );
        // Apply width visually via the header cell's parent
        const th = (e.target as HTMLElement).closest('th');
        if (th) {
          th.style.width = `${newWidth}px`;
          th.style.minWidth = `${newWidth}px`;
        }
      };

      const handleMouseUp = (upEvt: MouseEvent) => {
        document.removeEventListener('mousemove', handleMouseMove);
        document.removeEventListener('mouseup', handleMouseUp);
        cleanupRef.current = null;
        if (!resizingRef.current) return;
        const delta = upEvt.clientX - resizingRef.current.startX;
        const newWidth = Math.max(
          resizingRef.current.minWidth,
          resizingRef.current.startWidth + delta
        );
        onColumnResize?.(resizingRef.current.columnId, newWidth);
        resizingRef.current = null;
      };

      document.addEventListener('mousemove', handleMouseMove);
      document.addEventListener('mouseup', handleMouseUp);
      cleanupRef.current = () => {
        document.removeEventListener('mousemove', handleMouseMove);
        document.removeEventListener('mouseup', handleMouseUp);
        resizingRef.current = null;
      };
    },
    [columnWidths, onColumnResize]
  );

  const allSelected =
    audiobooks.length > 0 && audiobooks.every((book) => selectedIds?.has(book.id));
  const someSelected = audiobooks.some((book) => selectedIds?.has(book.id));

  const hasSelection = Boolean(onToggleSelect);
  const hasActions = Boolean(onEdit || onDelete);

  const formatCellValue = (col: ColumnDefinition, book: Audiobook): string => {
    const raw = col.accessor(book);
    if (col.formatter) return col.formatter(raw);
    return raw != null ? String(raw) : '--';
  };

  // Check if ANY segment (0-6) exists for fingerprint status
  const hasFingerprint = useCallback((file: BookFile): boolean => {
    return !!(
      file.acoustid_seg0 ||
      file.acoustid_seg1 ||
      file.acoustid_seg2 ||
      file.acoustid_seg3 ||
      file.acoustid_seg4 ||
      file.acoustid_seg5 ||
      file.acoustid_seg6
    );
  }, []);

  const toggleExpandRow = useCallback(
    (bookId: string) => {
      setExpandedRows((prev) => {
        const isExpanded = prev[bookId];
        const next = { ...prev, [bookId]: !isExpanded };

        if (!isExpanded) {
          // Expanding: fetch files if not cached
          if (!filesCache[bookId] && !loadingFiles[bookId]) {
            fetchFiles(bookId);
          }
        } else {
          // Collapsing: cancel any in-flight fetch
          const controller = abortControllersRef.current[bookId];
          if (controller) {
            controller.abort();
            delete abortControllersRef.current[bookId];
          }
        }
        return next;
      });
    },
    [filesCache, loadingFiles]
  );

  const fetchFiles = useCallback(async (bookId: string) => {
    setLoadingFiles((prev) => ({ ...prev, [bookId]: true }));
    setFetchErrors((prev) => ({ ...prev, [bookId]: false }));

    const controller = new AbortController();
    abortControllersRef.current[bookId] = controller;

    try {
      const result = await api.getBookFiles(bookId, { signal: controller.signal });

      // Only update state if not aborted and still expanded
      if (!controller.signal.aborted) {
        setFilesCache((prev) => ({
          ...prev,
          [bookId]: result.files || [],
        }));
      }
    } catch (error) {
      if (error instanceof Error && error.name === 'AbortError') {
        // Request was cancelled, do nothing
        return;
      }
      console.error(`Failed to fetch files for book ${bookId}:`, error);
      setFetchErrors((prev) => ({ ...prev, [bookId]: true }));
      setFilesCache((prev) => ({
        ...prev,
        [bookId]: [],
      }));
    } finally {
      setLoadingFiles((prev) => ({ ...prev, [bookId]: false }));
      delete abortControllersRef.current[bookId];
    }
  }, []);

  const getFingerprinterStatusColor = useCallback(
    (file: BookFile): 'success' | 'default' => {
      return hasFingerprint(file) ? 'success' : 'default';
    },
    [hasFingerprint]
  );

  const getFingerprinterStatusIcon = useCallback(
    (file: BookFile): React.ReactElement => {
      return hasFingerprint(file) ? (
        <CheckCircleIcon sx={{ fontSize: 16 }} />
      ) : (
        <CancelIcon sx={{ fontSize: 16 }} />
      );
    },
    [hasFingerprint]
  );

  const formatFileSize = (bytes?: number): string => {
    if (!bytes) return '--';
    return `${(bytes / 1024 / 1024).toFixed(2)} MB`;
  };

  if (loading) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight="400px">
        <CircularProgress />
      </Box>
    );
  }

  if (audiobooks.length === 0) {
    return (
      <Box
        display="flex"
        justifyContent="center"
        alignItems="center"
        minHeight="400px"
        flexDirection="column"
        gap={2}
      >
        <Typography variant="h6" color="text.secondary">
          No audiobooks found
        </Typography>
        <Typography variant="body2" color="text.secondary">
          Try adjusting your filters or add audiobooks to your library
        </Typography>
      </Box>
    );
  }

  return (
    <TableContainer component={Paper} sx={{ overflowX: 'auto' }}>
      <Table sx={{ tableLayout: 'fixed' }}>
        <TableHead>
          <TableRow>
            {/* Expand column */}
            <TableCell width={50} padding="checkbox" />

            {/* Checkbox column */}
            {hasSelection && (
              <TableCell width={50} padding="checkbox">
                {onSelectAll && (
                  <Checkbox
                    checked={allSelected}
                    indeterminate={someSelected && !allSelected}
                    onChange={onSelectAll}
                    inputProps={{
                      'aria-label': 'Select all books on page',
                    }}
                  />
                )}
              </TableCell>
            )}

            {/* Dynamic columns */}
            {activeColumns.map((col) => {
              const width = columnWidths?.[col.id] ?? col.defaultWidth;
              const isActiveSortCol = sortBy === col.sortKey;
              return (
                <TableCell
                  key={col.id}
                  sx={{
                    width,
                    minWidth: col.minWidth,
                    position: 'relative',
                    userSelect: 'none',
                    cursor: col.sortable ? 'pointer' : 'default',
                    whiteSpace: 'nowrap',
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    pr: 2,
                  }}
                  onClick={() => handleSortClick(col)}
                >
                  <Box
                    sx={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: 0.5,
                    }}
                  >
                    <span>{col.label}</span>
                    {isActiveSortCol &&
                      (sortOrder === 'asc' ? (
                        <ArrowUpwardIcon sx={{ fontSize: 16 }} />
                      ) : (
                        <ArrowDownwardIcon sx={{ fontSize: 16 }} />
                      ))}
                  </Box>

                  {/* Resize handle */}
                  {onColumnResize && (
                    <Box
                      onMouseDown={(e) => handleResizeStart(e, col)}
                      sx={{
                        position: 'absolute',
                        right: 0,
                        top: 0,
                        bottom: 0,
                        width: 5,
                        cursor: 'col-resize',
                        '&:hover': {
                          backgroundColor: 'action.hover',
                        },
                      }}
                    />
                  )}
                </TableCell>
              );
            })}

            {/* Actions column */}
            {hasActions && <TableCell width={50} />}
          </TableRow>
        </TableHead>
        <TableBody>
          {audiobooks.map((audiobook) => {
            const isExpanded = expandedRows[audiobook.id] || false;
            const files = filesCache[audiobook.id] ?? [];
            const isLoadingFiles = loadingFiles[audiobook.id] || false;
            const hasFetchError = fetchErrors[audiobook.id] || false;

            return (
              <React.Fragment key={audiobook.id}>
                <TableRow
                  hover
                  onClick={() => handleRowClick(audiobook)}
                  sx={{ cursor: onClick ? 'pointer' : 'default', contentVisibility: 'auto', containIntrinsicSize: '1px 52px' }}
                >
                  {/* Expand cell */}
                  <TableCell padding="checkbox" sx={{ width: 50 }}>
                    <IconButton
                      size="small"
                      onClick={(e) => {
                        e.stopPropagation();
                        toggleExpandRow(audiobook.id);
                      }}
                      aria-label={`${isExpanded ? 'Collapse' : 'Expand'} files for ${audiobook.title || 'audiobook'}`}
                      aria-expanded={isExpanded}
                    >
                      {isExpanded ? <ExpandLessIcon /> : <ExpandMoreIcon />}
                    </IconButton>
                  </TableCell>

                  {/* Checkbox cell */}
                  {hasSelection && (
                    <TableCell padding="checkbox">
                      {onToggleSelect && (
                        <Checkbox
                          checked={selectedIds?.has(audiobook.id) || false}
                          onClick={(event) => {
                            event.stopPropagation();
                            onToggleSelect(audiobook, event as unknown as React.MouseEvent);
                          }}
                          onChange={() => {}}
                          inputProps={{
                            'aria-label': `Select ${audiobook.title || 'audiobook'}`,
                          }}
                        />
                      )}
                    </TableCell>
                  )}

              {/* Dynamic data cells */}
              {activeColumns.map((col) => (
                <TableCell
                  key={col.id}
                  sx={{
                    maxWidth: columnWidths?.[col.id] ?? col.defaultWidth,
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap',
                  }}
                >
                  <Typography variant="body2" color="text.secondary" noWrap>
                    {formatCellValue(col, audiobook)}
                  </Typography>
                </TableCell>
              ))}

              {/* Actions cell */}
              {hasActions && (
                <TableCell>
                  <IconButton size="small" onClick={(e) => handleMenuClick(e, audiobook.id)}>
                    <MoreVertIcon />
                  </IconButton>
                  <Menu
                    anchorEl={anchorEls[audiobook.id] || null}
                    open={Boolean(anchorEls[audiobook.id])}
                    onClose={() => handleClose(audiobook.id)}
                  >
                    {onEdit && (
                      <MenuItem onClick={() => handleEdit(audiobook)}>
                        <EditIcon sx={{ mr: 1 }} fontSize="small" />
                        Edit
                      </MenuItem>
                    )}
                    {onDelete && (
                      <MenuItem onClick={() => handleDelete(audiobook)}>
                        <DeleteIcon sx={{ mr: 1 }} fontSize="small" />
                        Delete
                      </MenuItem>
                    )}
                  </Menu>
                </TableCell>
              )}
            </TableRow>

            {/* Expanded files rows */}
            {isExpanded && (
              <TableRow sx={{ bgcolor: '#f9f9f9' }}>
                <TableCell colSpan={hasSelection ? activeColumns.length + 3 : activeColumns.length + 2} sx={{ p: 0 }}>
                  <Collapse in={isExpanded} timeout="auto" unmountOnExit>
                    <Box sx={{ p: 2 }}>
                      <Typography variant="subtitle2" sx={{ mb: 1.5, fontWeight: 600 }}>
                        Files ({audiobook.total_file_count || 0})
                      </Typography>

                      {isLoadingFiles ? (
                        <Box display="flex" justifyContent="center" p={2}>
                          <CircularProgress size={24} />
                        </Box>
                      ) : hasFetchError ? (
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, p: 1.5, bgcolor: '#ffebee', borderRadius: 1 }}>
                          <Typography variant="body2" color="error">
                            Failed to load files
                          </Typography>
                          <IconButton
                            size="small"
                            onClick={() => fetchFiles(audiobook.id)}
                            sx={{ ml: 'auto' }}
                          >
                            <Typography variant="caption" sx={{ textDecoration: 'underline', cursor: 'pointer' }}>
                              Retry
                            </Typography>
                          </IconButton>
                        </Box>
                      ) : files.length === 0 ? (
                        <Typography variant="body2" color="text.secondary">
                          No files found
                        </Typography>
                      ) : (
                        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
                          {files.map((file) => (
                            <FileListRow
                              key={file.id}
                              file={file}
                              hasFingerprint={hasFingerprint}
                              getFingerprinterStatusIcon={getFingerprinterStatusIcon}
                              getFingerprinterStatusColor={getFingerprinterStatusColor}
                              formatFileSize={formatFileSize}
                            />
                          ))}
                        </Box>
                      )}
                    </Box>
                  </Collapse>
                </TableCell>
              </TableRow>
            )}
          </React.Fragment>
          );
          })}
        </TableBody>
      </Table>
    </TableContainer>
  );
};

// Memoized file list row component to prevent unnecessary rerenders
interface FileListRowProps {
  file: BookFile;
  hasFingerprint: (file: BookFile) => boolean;
  getFingerprinterStatusIcon: (file: BookFile) => React.ReactElement;
  getFingerprinterStatusColor: (file: BookFile) => 'success' | 'default';
  formatFileSize: (bytes?: number) => string;
}

const FileListRow = memo<FileListRowProps>(
  ({
    file,
    hasFingerprint,
    getFingerprinterStatusIcon,
    getFingerprinterStatusColor,
    formatFileSize,
  }) => (
    <Box
      sx={{
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        p: 1.5,
        bgcolor: '#fff',
        borderRadius: 1,
        border: '1px solid #e0e0e0',
      }}
    >
      <Box sx={{ flex: 1, minWidth: 0 }}>
        <Typography variant="body2" noWrap sx={{ fontWeight: 500 }}>
          {file.original_filename || file.file_path.split('/').pop() || 'Unknown'}
        </Typography>
        <Typography variant="caption" color="text.secondary" noWrap>
          {file.file_path}
        </Typography>
      </Box>

      <Box sx={{ display: 'flex', gap: 1, alignItems: 'center', ml: 1 }}>
        <Chip
          icon={getFingerprinterStatusIcon(file)}
          label={hasFingerprint(file) ? '✓ Fingerprinted' : '✗ Not Fingerprinted'}
          color={getFingerprinterStatusColor(file)}
          variant="outlined"
          size="small"
        />
        <Typography variant="caption" color="text.secondary" sx={{ minWidth: 80, textAlign: 'right' }}>
          {formatFileSize(file.file_size)}
        </Typography>
      </Box>
    </Box>
  )
);
