// file: web/src/components/common/ServerFileBrowser.tsx
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

import { useState, useEffect } from 'react';
import {
  Box,
  Typography,
  List,
  ListItem,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  Breadcrumbs,
  Link,
  Paper,
  CircularProgress,
  Alert,
  Chip,
  Stack,
} from '@mui/material';
import {
  Folder as FolderIcon,
  InsertDriveFile as FileIcon,
  Home as HomeIcon,
  NavigateNext as NavigateNextIcon,
  Block as BlockIcon,
} from '@mui/icons-material';
import * as api from '../../services/api';

interface ServerFileBrowserProps {
  /**
   * Initial path to start browsing from
   */
  initialPath?: string;

  /**
   * Callback when a file or folder is selected
   */
  onSelect?: (path: string, isDir: boolean) => void;

  /**
   * Whether to show files or only directories
   */
  showFiles?: boolean;

  /**
   * Whether to allow selecting directories
   */
  allowDirSelect?: boolean;

  /**
   * Whether to allow selecting files
   */
  allowFileSelect?: boolean;
}

export function ServerFileBrowser({
  initialPath = '/',
  onSelect,
  showFiles = true,
  allowDirSelect = true,
  allowFileSelect = false,
}: ServerFileBrowserProps) {
  const [currentPath, setCurrentPath] = useState(initialPath);
  const [items, setItems] = useState<api.FileSystemItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [diskInfo, setDiskInfo] = useState<api.FilesystemBrowseResult['disk_info']>();

  useEffect(() => {
    fetchDirectory(currentPath);
  }, [currentPath]);

  const fetchDirectory = async (path: string) => {
    setLoading(true);
    setError(null);
    try {
      const result = await api.browseFilesystem(path);
      setItems(result.items);
      setCurrentPath(result.path);
      setDiskInfo(result.disk_info);
    } catch (err) {
      console.error('Failed to browse filesystem:', err);
      setError(err instanceof Error ? err.message : 'Failed to browse filesystem');
    } finally {
      setLoading(false);
    }
  };

  const handleItemClick = (item: api.FileSystemItem) => {
    if (item.is_dir) {
      // Navigate into directory
      setCurrentPath(item.path);
    } else {
      // Select file if allowed
      if (allowFileSelect && onSelect) {
        onSelect(item.path, false);
      }
    }
  };

  const handleItemSelect = (item: api.FileSystemItem) => {
    if (item.is_dir && allowDirSelect && onSelect) {
      onSelect(item.path, true);
    } else if (!item.is_dir && allowFileSelect && onSelect) {
      onSelect(item.path, false);
    }
  };

  const getPathParts = (path: string): string[] => {
    const parts = path.split('/').filter(Boolean);
    return ['/', ...parts];
  };

  const navigateToPath = (index: number) => {
    const parts = getPathParts(currentPath);
    if (index === 0) {
      setCurrentPath('/');
    } else {
      const newPath = '/' + parts.slice(1, index + 1).join('/');
      setCurrentPath(newPath);
    }
  };

  const formatBytes = (bytes: number): string => {
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    if (bytes === 0) return '0 B';
    const i = Math.floor(Math.log(bytes) / Math.log(1024));
    return `${(bytes / Math.pow(1024, i)).toFixed(2)} ${sizes[i]}`;
  };

  const formatDate = (timestamp: number): string => {
    return new Date(timestamp * 1000).toLocaleDateString();
  };

  // Sort items: directories first, then files, both alphabetically
  const sortedItems = [...items].sort((a, b) => {
    if (a.is_dir && !b.is_dir) return -1;
    if (!a.is_dir && b.is_dir) return 1;
    return a.name.localeCompare(b.name);
  });

  // Filter items based on showFiles prop
  const filteredItems = showFiles
    ? sortedItems
    : sortedItems.filter((item) => item.is_dir);

  const pathParts = getPathParts(currentPath);

  return (
    <Box>
      {/* Breadcrumb Navigation */}
      <Paper sx={{ p: 2, mb: 2 }}>
        <Stack direction="row" alignItems="center" spacing={2} mb={1}>
          <Typography variant="subtitle2" color="text.secondary">
            Current Path:
          </Typography>
          {diskInfo && (
            <Stack direction="row" spacing={1}>
              {diskInfo.readable && (
                <Chip label="Readable" size="small" color="success" />
              )}
              {diskInfo.writable && (
                <Chip label="Writable" size="small" color="success" />
              )}
            </Stack>
          )}
        </Stack>
        <Breadcrumbs separator={<NavigateNextIcon fontSize="small" />}>
          {pathParts.map((part, index) => (
            <Link
              key={index}
              component="button"
              variant="body1"
              onClick={() => navigateToPath(index)}
              sx={{ cursor: 'pointer' }}
            >
              {index === 0 ? <HomeIcon fontSize="small" /> : part}
            </Link>
          ))}
        </Breadcrumbs>
      </Paper>

      {/* Loading State */}
      {loading && (
        <Box display="flex" justifyContent="center" py={4}>
          <CircularProgress />
        </Box>
      )}

      {/* Error State */}
      {error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {error}
        </Alert>
      )}

      {/* File/Directory List */}
      {!loading && !error && (
        <Paper>
          <List>
            {filteredItems.length === 0 && (
              <ListItem>
                <ListItemText
                  primary="No items found"
                  secondary={showFiles ? 'This directory is empty' : 'No subdirectories found'}
                />
              </ListItem>
            )}
            {filteredItems.map((item) => (
              <ListItem
                key={item.path}
                disablePadding
                secondaryAction={
                  item.is_dir && item.excluded ? (
                    <Chip
                      icon={<BlockIcon />}
                      label="Excluded"
                      size="small"
                      color="warning"
                    />
                  ) : null
                }
              >
                <ListItemButton
                  onClick={() => handleItemClick(item)}
                  onDoubleClick={() => handleItemSelect(item)}
                  disabled={item.excluded}
                >
                  <ListItemIcon>
                    {item.is_dir ? (
                      <FolderIcon color={item.excluded ? 'disabled' : 'primary'} />
                    ) : (
                      <FileIcon color="action" />
                    )}
                  </ListItemIcon>
                  <ListItemText
                    primary={item.name}
                    secondary={
                      !item.is_dir && item.size !== undefined
                        ? `${formatBytes(item.size)}${
                            item.mod_time ? ` • ${formatDate(item.mod_time)}` : ''
                          }`
                        : null
                    }
                    sx={{
                      opacity: item.excluded ? 0.5 : 1,
                    }}
                  />
                </ListItemButton>
              </ListItem>
            ))}
          </List>
        </Paper>
      )}

      {/* Help Text */}
      <Box mt={2}>
        <Typography variant="caption" color="text.secondary">
          {allowDirSelect && allowFileSelect
            ? 'Click to navigate, double-click to select a file or folder'
            : allowDirSelect
            ? 'Click to navigate, double-click to select a folder'
            : allowFileSelect
            ? 'Click to navigate, double-click to select a file'
            : 'Click to navigate through directories'}
        </Typography>
      </Box>
    </Box>
  );
}
