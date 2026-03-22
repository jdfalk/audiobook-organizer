// file: web/src/components/audiobooks/AudiobookList.tsx
// version: 2.0.0
// guid: 0c1d2e3f-4a5b-6c7d-8e9f-0a1b2c3d4e5f

import React, { useCallback, useEffect, useRef } from 'react';
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
} from '@mui/material';
import {
  MoreVert as MoreVertIcon,
  Edit as EditIcon,
  Delete as DeleteIcon,
  ArrowUpward as ArrowUpwardIcon,
  ArrowDownward as ArrowDownwardIcon,
} from '@mui/icons-material';
import type { Audiobook } from '../../types';
import { type ColumnDefinition, getDefaultVisibleColumns } from '../../config/columnDefinitions';

interface AudiobookListProps {
  audiobooks: Audiobook[];
  loading?: boolean;
  onEdit?: (audiobook: Audiobook) => void;
  onDelete?: (audiobook: Audiobook) => void;
  onClick?: (audiobook: Audiobook) => void;
  selectedIds?: Set<string>;
  onToggleSelect?: (audiobook: Audiobook) => void;
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
          {audiobooks.map((audiobook) => (
            <TableRow
              key={audiobook.id}
              hover
              onClick={() => handleRowClick(audiobook)}
              sx={{ cursor: onClick ? 'pointer' : 'default' }}
            >
              {/* Checkbox cell */}
              {hasSelection && (
                <TableCell padding="checkbox">
                  {onToggleSelect && (
                    <Checkbox
                      checked={selectedIds?.has(audiobook.id) || false}
                      onClick={(event) => event.stopPropagation()}
                      onChange={() => onToggleSelect(audiobook)}
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
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  );
};
