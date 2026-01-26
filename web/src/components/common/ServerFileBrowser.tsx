// file: web/src/components/common/ServerFileBrowser.tsx
// version: 1.4.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

import { useState, useEffect, useCallback, MouseEvent } from 'react';
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
  AlertColor,
  Chip,
  Stack,
  TextField,
  IconButton,
  Menu,
  MenuItem,
  Snackbar,
} from '@mui/material';
import {
  Folder as FolderIcon,
  InsertDriveFile as FileIcon,
  Home as HomeIcon,
  NavigateNext as NavigateNextIcon,
  Block as BlockIcon,
  Edit as EditIcon,
  Check as CheckIcon,
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
  const [diskInfo, setDiskInfo] =
    useState<api.FilesystemBrowseResult['disk_info']>();
  const [editingPath, setEditingPath] = useState(false);
  const [editPath, setEditPath] = useState(currentPath);
  const [extensionFilter, setExtensionFilter] = useState('');
  const [notice, setNotice] = useState<{
    message: string;
    severity: AlertColor;
  } | null>(null);
  const [contextMenu, setContextMenu] = useState<{
    mouseX: number;
    mouseY: number;
  } | null>(null);
  const [contextItem, setContextItem] = useState<api.FileSystemItem | null>(
    null
  );

  const fetchDirectory = useCallback(
    async (path: string) => {
      setLoading(true);
      setError(null);
      try {
        const result = await api.browseFilesystem(path);
        setItems(result.items);
        setCurrentPath(result.path);
        setDiskInfo(result.disk_info);

        // Automatically notify parent of current path after successful fetch (if directory selection is allowed)
        if (allowDirSelect && onSelect) {
          onSelect(result.path, true);
        }
      } catch (err) {
        console.error('Failed to browse filesystem:', err);
        setError(
          err instanceof Error ? err.message : 'Failed to browse filesystem'
        );
      } finally {
        setLoading(false);
      }
    },
    [allowDirSelect, onSelect]
  );

  useEffect(() => {
    fetchDirectory(currentPath);
    setEditPath(currentPath);
  }, [currentPath, fetchDirectory]);

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

  const handleEditPath = () => {
    setEditingPath(true);
    setEditPath(currentPath);
  };

  const handleSavePath = () => {
    setEditingPath(false);
    if (editPath !== currentPath) {
      setCurrentPath(editPath);
    }
  };

  const handleCancelEdit = () => {
    setEditingPath(false);
    setEditPath(currentPath);
  };

  const handleContextMenu = (
    event: MouseEvent,
    item: api.FileSystemItem
  ) => {
    if (!item.is_dir) return;
    event.preventDefault();
    setContextItem(item);
    setContextMenu({ mouseX: event.clientX + 2, mouseY: event.clientY - 2 });
  };

  const handleCloseContextMenu = () => {
    setContextMenu(null);
    setContextItem(null);
  };

  const handleToggleExclude = async () => {
    if (!contextItem) return;
    try {
      if (contextItem.excluded) {
        await api.includeFilesystemPath(contextItem.path);
        setNotice({
          message: 'Folder included in scan.',
          severity: 'success',
        });
      } else {
        await api.excludeFilesystemPath(contextItem.path);
        setNotice({
          message: 'Folder excluded from scan.',
          severity: 'success',
        });
      }
      await fetchDirectory(currentPath);
    } catch (err) {
      const message =
        err instanceof Error ? err.message : 'Failed to update exclusion';
      setError(message);
      setNotice({ message, severity: 'error' });
    } finally {
      handleCloseContextMenu();
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

  const normalizedFilter = extensionFilter.trim().toLowerCase();
  const extensionQuery =
    normalizedFilter.length > 0 && !normalizedFilter.startsWith('.')
      ? `.${normalizedFilter}`
      : normalizedFilter;

  const filteredItems = sortedItems.filter((item) => {
    if (item.is_dir) {
      return true;
    }
    if (!showFiles) {
      return false;
    }
    if (!extensionQuery) {
      return true;
    }
    return item.name.toLowerCase().endsWith(extensionQuery);
  });

  const pathParts = getPathParts(currentPath);
  const availableLabel =
    diskInfo?.total_bytes !== undefined && diskInfo?.free_bytes !== undefined
      ? [
          'Available',
          formatBytes(diskInfo.free_bytes),
          '/',
          formatBytes(diskInfo.total_bytes),
        ].join(' ')
      : null;
  const libraryLabel =
    diskInfo?.library_bytes !== undefined
      ? `Library ${formatBytes(diskInfo.library_bytes)}`
      : null;

  return (
    <Box>
      <Snackbar
        open={!!notice}
        autoHideDuration={4000}
        onClose={() => setNotice(null)}
        anchorOrigin={{ vertical: 'top', horizontal: 'center' }}
      >
        {notice ? (
          <Alert
            severity={notice.severity}
            onClose={() => setNotice(null)}
            sx={{ width: '100%' }}
          >
            {notice.message}
          </Alert>
        ) : null}
      </Snackbar>
      {/* Sticky Path Editor */}
      <Paper
        sx={{
          p: 2,
          mb: 2,
          position: 'sticky',
          top: 0,
          zIndex: 10,
          bgcolor: 'background.paper',
        }}
      >
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

        {editingPath ? (
          <Stack direction="row" spacing={1} alignItems="center">
            <TextField
              fullWidth
              size="small"
              value={editPath}
              onChange={(e) => setEditPath(e.target.value)}
              onKeyPress={(e) => {
                if (e.key === 'Enter') {
                  handleSavePath();
                }
              }}
              autoFocus
            />
            <IconButton size="small" color="primary" onClick={handleSavePath}>
              <CheckIcon />
            </IconButton>
            <IconButton size="small" onClick={handleCancelEdit}>
              <EditIcon />
            </IconButton>
          </Stack>
        ) : (
          <Stack direction="row" spacing={1} alignItems="center">
            <Breadcrumbs
              separator={<NavigateNextIcon fontSize="small" />}
              sx={{ flex: 1 }}
            >
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
            <IconButton size="small" onClick={handleEditPath}>
              <EditIcon />
            </IconButton>
          </Stack>
        )}
        <Stack
          direction={{ xs: 'column', md: 'row' }}
          spacing={2}
          alignItems={{ xs: 'stretch', md: 'center' }}
          sx={{ mt: 2 }}
        >
          {showFiles && (
            <TextField
              size="small"
              label="Filter extension"
              placeholder=".m4b"
              value={extensionFilter}
              onChange={(e) => setExtensionFilter(e.target.value)}
              sx={{ maxWidth: 220 }}
            />
          )}
          {availableLabel && (
            <Chip label={availableLabel} size="small" color="info" />
          )}
          {libraryLabel && (
            <Chip
              label={libraryLabel}
              size="small"
              color="primary"
              variant="outlined"
            />
          )}
        </Stack>
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
                  secondary={
                    showFiles
                      ? 'This directory is empty'
                      : 'No subdirectories found'
                  }
                />
              </ListItem>
            )}
            {filteredItems.map((item) => (
              <ListItem
                key={item.path}
                disablePadding
                onContextMenu={(event) => handleContextMenu(event, item)}
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
                      <FolderIcon
                        color={item.excluded ? 'disabled' : 'primary'}
                      />
                    ) : (
                      <FileIcon color="action" />
                    )}
                  </ListItemIcon>
                  <ListItemText
                    primary={item.name}
                    secondary={
                      !item.is_dir && item.size !== undefined
                        ? `${formatBytes(item.size)}${
                            item.mod_time
                              ? ` • ${formatDate(item.mod_time)}`
                              : ''
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

      <Menu
        open={contextMenu !== null}
        onClose={handleCloseContextMenu}
        anchorReference="anchorPosition"
        anchorPosition={
          contextMenu !== null
            ? { top: contextMenu.mouseY, left: contextMenu.mouseX }
            : undefined
        }
      >
        <MenuItem onClick={handleToggleExclude} disabled={!contextItem}>
          {contextItem?.excluded ? 'Include in scan' : 'Exclude from scan'}
        </MenuItem>
      </Menu>

      {/* Help Text */}
      <Box mt={2}>
        <Typography variant="caption" color="text.secondary">
          {allowDirSelect && allowFileSelect
            ? 'Click folders to navigate. Current folder is automatically selected. Double-click items to explicitly select.'
            : allowDirSelect
              ? 'Click folders to navigate. Current folder is automatically selected.'
              : allowFileSelect
                ? 'Click to navigate, double-click files to select'
                : 'Click to navigate through directories'}
        </Typography>
      </Box>
    </Box>
  );
}
