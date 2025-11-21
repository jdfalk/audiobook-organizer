// file: web/src/components/audiobooks/AudiobookList.tsx
// version: 1.0.0
// guid: 0c1d2e3f-4a5b-6c7d-8e9f-0a1b2c3d4e5f

import React from 'react';
import {
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  Avatar,
  IconButton,
  Menu,
  MenuItem,
  Typography,
  Chip,
  Box,
  CircularProgress,
} from '@mui/material';
import {
  MoreVert as MoreVertIcon,
  Edit as EditIcon,
  Delete as DeleteIcon,
} from '@mui/icons-material';
import type { Audiobook } from '../../types';

interface AudiobookListProps {
  audiobooks: Audiobook[];
  loading?: boolean;
  onEdit?: (audiobook: Audiobook) => void;
  onDelete?: (audiobook: Audiobook) => void;
  onClick?: (audiobook: Audiobook) => void;
}

export const AudiobookList: React.FC<AudiobookListProps> = ({
  audiobooks,
  loading = false,
  onEdit,
  onDelete,
  onClick,
}) => {
  const [anchorEls, setAnchorEls] = React.useState<
    Record<string, HTMLElement | null>
  >({});

  const handleMenuClick = (
    event: React.MouseEvent<HTMLElement>,
    id: string
  ) => {
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

  const formatDuration = (seconds?: number): string => {
    if (!seconds) return '--';
    const hours = Math.floor(seconds / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    return `${hours}h ${minutes}m`;
  };

  const formatFileSize = (bytes?: number): string => {
    if (!bytes) return '--';
    const mb = bytes / (1024 * 1024);
    if (mb >= 1024) {
      return `${(mb / 1024).toFixed(2)} GB`;
    }
    return `${mb.toFixed(2)} MB`;
  };

  if (loading) {
    return (
      <Box
        display="flex"
        justifyContent="center"
        alignItems="center"
        minHeight="400px"
      >
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
    <TableContainer component={Paper}>
      <Table>
        <TableHead>
          <TableRow>
            <TableCell width={50}></TableCell>
            <TableCell>Title</TableCell>
            <TableCell>Author</TableCell>
            <TableCell>Narrator</TableCell>
            <TableCell>Series</TableCell>
            <TableCell>Genre</TableCell>
            <TableCell>Duration</TableCell>
            <TableCell>Size</TableCell>
            <TableCell width={50}></TableCell>
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
              <TableCell>
                {audiobook.cover_path ? (
                  <Avatar
                    src={audiobook.cover_path}
                    alt={audiobook.title}
                    variant="rounded"
                    sx={{ width: 40, height: 40 }}
                  />
                ) : (
                  <Avatar variant="rounded" sx={{ width: 40, height: 40 }}>
                    {audiobook.title?.charAt(0).toUpperCase() || '?'}
                  </Avatar>
                )}
              </TableCell>
              <TableCell>
                <Typography variant="body2" fontWeight="medium">
                  {audiobook.title || 'Untitled'}
                </Typography>
              </TableCell>
              <TableCell>
                <Typography variant="body2" color="text.secondary">
                  {audiobook.author || '--'}
                </Typography>
              </TableCell>
              <TableCell>
                <Typography variant="body2" color="text.secondary">
                  {audiobook.narrator || '--'}
                </Typography>
              </TableCell>
              <TableCell>
                {audiobook.series && (
                  <Typography variant="body2" color="text.secondary">
                    {audiobook.series}
                    {audiobook.series_number && ` #${audiobook.series_number}`}
                  </Typography>
                )}
              </TableCell>
              <TableCell>
                {audiobook.genre && (
                  <Chip
                    label={audiobook.genre}
                    size="small"
                    variant="outlined"
                  />
                )}
              </TableCell>
              <TableCell>
                <Typography variant="body2" color="text.secondary">
                  {formatDuration(audiobook.duration_seconds)}
                </Typography>
              </TableCell>
              <TableCell>
                <Typography variant="body2" color="text.secondary">
                  {formatFileSize(audiobook.file_size_bytes)}
                </Typography>
              </TableCell>
              <TableCell>
                <IconButton
                  size="small"
                  onClick={(e) => handleMenuClick(e, audiobook.id)}
                >
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
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  );
};
