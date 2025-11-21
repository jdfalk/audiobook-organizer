// file: web/src/pages/FileManager.tsx
// version: 1.2.0
// guid: 4a5b6c7d-8e9f-0a1b-2c3d-4e5f6a7b8c9d

import { useState, useCallback, useRef } from 'react';
import {
  Box,
  Typography,
  Button,
  Grid,
  Paper,
  Breadcrumbs,
  Link,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  TextField,
  Alert,
  Stack,
} from '@mui/material';
import {
  Add as AddIcon,
  Home as HomeIcon,
  NavigateNext as NavigateNextIcon,
  FolderOpen as FolderOpenIcon,
} from '@mui/icons-material';
import {
  DirectoryTree,
  DirectoryNode,
} from '../components/filemanager/DirectoryTree';
import {
  LibraryFolderCard,
  LibraryFolder,
} from '../components/filemanager/LibraryFolderCard';

export function FileManager() {
  const [libraryFolders, setLibraryFolders] = useState<LibraryFolder[]>([]);
  const [addFolderOpen, setAddFolderOpen] = useState(false);
  const [newFolderPath, setNewFolderPath] = useState('');
  const [selectedPath, setSelectedPath] = useState<string>('');
  const [directoryTree] = useState<DirectoryNode | null>(null);
  const folderInputRef = useRef<HTMLInputElement>(null);

  const handleAddFolder = async () => {
    if (!newFolderPath.trim()) return;

    try {
      // TODO: Replace with actual API call
      // await fetch('/api/v1/library-folders', {
      //   method: 'POST',
      //   headers: { 'Content-Type': 'application/json' },
      //   body: JSON.stringify({ path: newFolderPath })
      // });

      const newFolder: LibraryFolder = {
        id: Date.now().toString(),
        path: newFolderPath,
        status: 'idle',
        book_count: 0,
      };

      setLibraryFolders((prev) => [...prev, newFolder]);
      setNewFolderPath('');
      setAddFolderOpen(false);
    } catch (error) {
      console.error('Failed to add library folder:', error);
    }
  };

  const handleRemoveFolder = useCallback(async (folder: LibraryFolder) => {
    if (!confirm(`Remove library folder: ${folder.path}?`)) return;

    try {
      // TODO: Replace with actual API call
      // await fetch(`/api/v1/library-folders/${folder.id}`, {
      //   method: 'DELETE'
      // });

      setLibraryFolders((prev) => prev.filter((f) => f.id !== folder.id));
    } catch (error) {
      console.error('Failed to remove library folder:', error);
    }
  }, []);

  const handleScanFolder = useCallback(async (folder: LibraryFolder) => {
    try {
      // TODO: Replace with actual API call
      // await fetch(`/api/v1/library-folders/${folder.id}/scan`, {
      //   method: 'POST'
      // });

      setLibraryFolders((prev) =>
        prev.map((f) =>
          f.id === folder.id
            ? { ...f, status: 'scanning' as const, progress: 0 }
            : f
        )
      );

      // Simulate scan progress
      let progress = 0;
      const interval = setInterval(() => {
        progress += 10;
        setLibraryFolders((prev) =>
          prev.map((f) =>
            f.id === folder.id
              ? {
                  ...f,
                  progress,
                  status:
                    progress >= 100
                      ? ('complete' as const)
                      : ('scanning' as const),
                }
              : f
          )
        );
        if (progress >= 100) clearInterval(interval);
      }, 500);
    } catch (error) {
      console.error('Failed to scan library folder:', error);
      setLibraryFolders((prev) =>
        prev.map((f) =>
          f.id === folder.id
            ? { ...f, status: 'error' as const, error_message: 'Scan failed' }
            : f
        )
      );
    }
  }, []);

  const handleLoadChildren = async (
    _path: string
  ): Promise<DirectoryNode[]> => {
    // TODO: Replace with actual API call
    // const response = await fetch(`/api/v1/filesystem/browse?path=${encodeURIComponent(path)}`);
    // const data = await response.json();
    // return data.children;
    return [];
  };

  const handleSelectDirectory = (path: string) => {
    setSelectedPath(path);
  };

  const pathSegments = selectedPath.split('/').filter(Boolean);

  const handleBrowseFolder = () => {
    folderInputRef.current?.click();
  };

  const handleFolderSelect = async (
    event: React.ChangeEvent<HTMLInputElement>
  ) => {
    const files = event.target.files;
    if (!files || files.length === 0) return;

    // Get the directory path from the first file
    const firstFile = files[0];
    const webkitPath = (firstFile as any).webkitRelativePath;
    if (webkitPath) {
      const folderPath = webkitPath.split('/')[0];
      setNewFolderPath(folderPath);
      setAddFolderOpen(true);
    }
  };

  return (
    <Box>
      <Box
        display="flex"
        justifyContent="space-between"
        alignItems="center"
        mb={3}
      >
        <Typography variant="h4">File Manager</Typography>
        <Stack direction="row" spacing={2}>
          <Button
            variant="outlined"
            startIcon={<FolderOpenIcon />}
            onClick={handleBrowseFolder}
          >
            Browse for Folder
          </Button>
          <Button
            variant="contained"
            startIcon={<AddIcon />}
            onClick={() => setAddFolderOpen(true)}
          >
            Add Library Folder
          </Button>
        </Stack>
      </Box>

      {/* Hidden folder input for browsing */}
      <input
        ref={folderInputRef}
        type="file"
        // @ts-ignore - webkitdirectory is not in TypeScript types
        webkitdirectory=""
        directory=""
        multiple
        style={{ display: 'none' }}
        onChange={handleFolderSelect}
      />

      <Grid container spacing={3}>
        <Grid item xs={12}>
          <Typography variant="h6" gutterBottom>
            Library Folders
          </Typography>
          {libraryFolders.length === 0 ? (
            <Alert severity="info">
              No library folders added yet. Click "Add Library Folder" to get
              started.
            </Alert>
          ) : (
            <Grid container spacing={2}>
              {libraryFolders.map((folder) => (
                <Grid item xs={12} md={6} lg={4} key={folder.id}>
                  <LibraryFolderCard
                    folder={folder}
                    onRemove={handleRemoveFolder}
                    onScan={handleScanFolder}
                  />
                </Grid>
              ))}
            </Grid>
          )}
        </Grid>

        {directoryTree && (
          <Grid item xs={12}>
            <Paper sx={{ p: 2 }}>
              <Typography variant="h6" gutterBottom>
                Directory Browser
              </Typography>

              <Breadcrumbs
                separator={<NavigateNextIcon fontSize="small" />}
                sx={{ mb: 2 }}
              >
                <Link
                  underline="hover"
                  color="inherit"
                  href="#"
                  onClick={() => handleSelectDirectory('/')}
                  sx={{ display: 'flex', alignItems: 'center' }}
                >
                  <HomeIcon sx={{ mr: 0.5 }} fontSize="small" />
                  Home
                </Link>
                {pathSegments.map((segment, index) => {
                  const path = '/' + pathSegments.slice(0, index + 1).join('/');
                  return (
                    <Link
                      key={path}
                      underline="hover"
                      color="inherit"
                      href="#"
                      onClick={() => handleSelectDirectory(path)}
                    >
                      {segment}
                    </Link>
                  );
                })}
              </Breadcrumbs>

              <DirectoryTree
                root={directoryTree}
                onSelectDirectory={handleSelectDirectory}
                onLoadChildren={handleLoadChildren}
                selectedPath={selectedPath}
              />
            </Paper>
          </Grid>
        )}
      </Grid>

      <Dialog
        open={addFolderOpen}
        onClose={() => setAddFolderOpen(false)}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>Add Library Folder</DialogTitle>
        <DialogContent>
          <TextField
            autoFocus
            margin="dense"
            label="Folder Path"
            type="text"
            fullWidth
            value={newFolderPath}
            onChange={(e) => setNewFolderPath(e.target.value)}
            placeholder="/path/to/audiobooks"
            helperText="Enter the full path to your audiobook folder"
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setAddFolderOpen(false)}>Cancel</Button>
          <Button
            onClick={handleAddFolder}
            variant="contained"
            disabled={!newFolderPath.trim()}
          >
            Add Folder
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
