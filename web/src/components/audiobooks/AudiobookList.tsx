// file: web/src/components/audiobooks/AudiobookList.tsx
// version: 2.3.0
// guid: 0c1d2e3f-4a5b-6c7d-8e9f-0a1b2c3d4e5f
// last-edited: 2026-05-19

import React, { useCallback, useEffect, useRef, useState } from 'react';
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
  Switch,
  FormControlLabel,
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
  const [expandedRows, setExpandedRows] = useState<Set<string>>(new Set());
  const [filesCache, setFilesCache] = useState<Record<string, BookFile[]>>({});
  const [loadingFiles, setLoadingFiles] = useState<Set<string>>(new Set());

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

  const handleFileUpdate = (bookId: string, updatedFile: BookFile) => {
    setFilesCache((prev) => ({
      ...prev,
      [bookId]: (prev[bookId] || []).map((f) =>
        f.id === updatedFile.id ? updatedFile : f
      ),
    }));
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

  const toggleExpandRow = async (bookId: string) => {
    setExpandedRows((prev) => {
      const next = new Set(prev);
      if (next.has(bookId)) {
        next.delete(bookId);
      } else {
        next.add(bookId);
        // Fetch files if not cached
        if (!filesCache[bookId] && !loadingFiles.has(bookId)) {
          fetchFiles(bookId);
        }
      }
      return next;
    });
  };

  const fetchFiles = async (bookId: string) => {
    setLoadingFiles((prev) => new Set(prev).add(bookId));
    try {
      const result = await api.getBookFiles(bookId);
      setFilesCache((prev) => ({
        ...prev,
        [bookId]: result.files,
      }));
    } catch (error) {
      console.error(`Failed to fetch files for book ${bookId}:`, error);
      setFilesCache((prev) => ({
        ...prev,
        [bookId]: [],
      }));
    } finally {
      setLoadingFiles((prev) => {
        const next = new Set(prev);
        next.delete(bookId);
        return next;
      });
    }
  };

  const getFingerprinterStatusColor = (file: BookFile): 'success' | 'default' => {
    return file.acoustid_seg0 ? 'success' : 'default';
  };

  const getFingerprinterStatusIcon = (file: BookFile): React.ReactElement => {
    return file.acoustid_seg0 ? (
      <CheckCircleIcon sx={{ fontSize: 16 }} />
    ) : (
      <CancelIcon sx={{ fontSize: 16 }} />
    );
  };

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
            const isExpanded = expandedRows.has(audiobook.id);
            const files = filesCache[audiobook.id] ?? [];
            const isLoadingFiles = loadingFiles.has(audiobook.id);

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
                      ) : files.length === 0 ? (
                        <Typography variant="body2" color="text.secondary">
                          No files found
                        </Typography>
                      ) : (
                        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
                          {files.map((file) => (
                            <Box
                              key={file.id}
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
                                  label={file.acoustid_seg0 ? '✓ Fingerprinted' : '✗ Not Fingerprinted'}
                                  color={getFingerprinterStatusColor(file)}
                                  variant="outlined"
                                  size="small"
                                />
                                <FormControlLabel
                                  control={
                                    <Switch
                                      size="small"
                                      checked={!!file.skip_scan}
                                      onChange={async (e) => {
                                        const newValue = e.target.checked;
                                        try {
                                          const updated = await api.patchBookFile(audiobook.id, file.id, {
                                            skip_scan: newValue,
                                          });
                                          handleFileUpdate(audiobook.id, updated);
                                        } catch (err) {
                                          console.error('Failed to update skip_scan:', err);
                                        }
                                      }}
                                    />
                                  }
                                  label="Skip"
                                  sx={{ ml: 0.5 }}
                                />
                                <Typography variant="caption" color="text.secondary" sx={{ minWidth: 80, textAlign: 'right' }}>
                                  {formatFileSize(file.file_size)}
                                </Typography>
                              </Box>
                            </Box>
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
