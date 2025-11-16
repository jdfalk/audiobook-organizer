// file: web/src/components/filemanager/LibraryFolderCard.tsx
// version: 1.0.0
// guid: 7d8e9f0a-1b2c-3d4e-5f6a-7b8c9d0e1f2a

import React, { useState } from 'react';
import {
  Card,
  CardContent,
  Typography,
  IconButton,
  Box,
  Chip,
  LinearProgress,
  Menu,
  MenuItem,
} from '@mui/material';
import {
  Folder as FolderIcon,
  MoreVert as MoreVertIcon,
  Delete as DeleteIcon,
  Sync as SyncIcon,
  CheckCircle as CheckCircleIcon,
  Error as ErrorIcon,
} from '@mui/icons-material';

export interface LibraryFolder {
  id: string;
  path: string;
  status: 'idle' | 'scanning' | 'error' | 'complete';
  progress?: number;
  book_count?: number;
  last_scan?: string;
  error_message?: string;
}

interface LibraryFolderCardProps {
  folder: LibraryFolder;
  onRemove?: (folder: LibraryFolder) => void;
  onScan?: (folder: LibraryFolder) => void;
}

export const LibraryFolderCard: React.FC<LibraryFolderCardProps> = ({
  folder,
  onRemove,
  onScan,
}) => {
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);

  const handleMenuClick = (event: React.MouseEvent<HTMLElement>) => {
    setAnchorEl(event.currentTarget);
  };

  const handleClose = () => {
    setAnchorEl(null);
  };

  const handleRemove = () => {
    handleClose();
    onRemove?.(folder);
  };

  const handleScan = () => {
    handleClose();
    onScan?.(folder);
  };

  const getStatusIcon = () => {
    switch (folder.status) {
      case 'scanning':
        return <SyncIcon sx={{ animation: 'spin 1s linear infinite' }} />;
      case 'complete':
        return <CheckCircleIcon color="success" />;
      case 'error':
        return <ErrorIcon color="error" />;
      default:
        return <FolderIcon />;
    }
  };

  const getStatusColor = () => {
    switch (folder.status) {
      case 'scanning':
        return 'info';
      case 'complete':
        return 'success';
      case 'error':
        return 'error';
      default:
        return 'default';
    }
  };

  return (
    <Card>
      <CardContent>
        <Box display="flex" justifyContent="space-between" alignItems="flex-start">
          <Box display="flex" gap={2} flex={1} alignItems="flex-start">
            <Box mt={0.5}>{getStatusIcon()}</Box>
            <Box flex={1}>
              <Typography variant="h6" gutterBottom noWrap title={folder.path}>
                {folder.path.split('/').pop() || folder.path}
              </Typography>
              <Typography
                variant="caption"
                color="text.secondary"
                sx={{ display: 'block', mb: 1 }}
                noWrap
                title={folder.path}
              >
                {folder.path}
              </Typography>

              <Box display="flex" gap={1} flexWrap="wrap">
                <Chip
                  label={folder.status}
                  size="small"
                  color={getStatusColor() as 'default' | 'success' | 'error' | 'info'}
                />
                {folder.book_count !== undefined && (
                  <Chip label={`${folder.book_count} books`} size="small" variant="outlined" />
                )}
                {folder.last_scan && (
                  <Chip
                    label={`Scanned: ${new Date(folder.last_scan).toLocaleDateString()}`}
                    size="small"
                    variant="outlined"
                  />
                )}
              </Box>

              {folder.error_message && (
                <Typography
                  variant="caption"
                  color="error"
                  sx={{ display: 'block', mt: 1 }}
                >
                  {folder.error_message}
                </Typography>
              )}
            </Box>
          </Box>

          <IconButton size="small" onClick={handleMenuClick}>
            <MoreVertIcon />
          </IconButton>
        </Box>

        {folder.status === 'scanning' && folder.progress !== undefined && (
          <Box mt={2}>
            <LinearProgress variant="determinate" value={folder.progress} />
            <Typography variant="caption" color="text.secondary" sx={{ mt: 0.5 }}>
              {Math.round(folder.progress)}% complete
            </Typography>
          </Box>
        )}
      </CardContent>

      <Menu anchorEl={anchorEl} open={Boolean(anchorEl)} onClose={handleClose}>
        <MenuItem onClick={handleScan} disabled={folder.status === 'scanning'}>
          <SyncIcon sx={{ mr: 1 }} fontSize="small" />
          Scan Now
        </MenuItem>
        <MenuItem onClick={handleRemove}>
          <DeleteIcon sx={{ mr: 1 }} fontSize="small" />
          Remove
        </MenuItem>
      </Menu>
    </Card>
  );
};
