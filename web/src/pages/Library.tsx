// file: web/src/pages/Library.tsx
// version: 1.6.0
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
import { MetadataEditDialog } from '../components/audiobooks/MetadataEditDialog';
import { BatchEditDialog } from '../components/audiobooks/BatchEditDialog';
import { VersionManagement } from '../components/audiobooks/VersionManagement';
import type { Audiobook, FilterOptions } from '../types';
import * as api from '../services/api';

interface ImportPath {
  id: string;
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
  const [filterOpen, setFilterOpen] = useState(false);
  const [filters, setFilters] = useState<FilterOptions>({});
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
  const [editingAudiobook, setEditingAudiobook] = useState<Audiobook | null>(null);
  const [selectedAudiobooks, setSelectedAudiobooks] = useState<Audiobook[]>([]);
  const [batchEditOpen, setBatchEditOpen] = useState(false);
  const [hasLibraryFolders, setHasLibraryFolders] = useState(true);
  const [versionManagementOpen, setVersionManagementOpen] = useState(false);
  const [versionManagingAudiobook, setVersionManagingAudiobook] = useState<Audiobook | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const folderInputRef = useRef<HTMLInputElement>(null);

  // Import path management
  const [importPaths, setImportPaths] = useState<ImportPath[]>([]);
  const [importPathsExpanded, setImportPathsExpanded] = useState(false);
  const [addPathDialogOpen, setAddPathDialogOpen] = useState(false);
  const [newImportPath, setNewImportPath] = useState('');

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

      // Fetch audiobooks and library folders
      const [books, folders, bookCount] = await Promise.all([
        debouncedSearch ? api.searchBooks(debouncedSearch, limit) : api.getBooks(limit, offset),
        api.getLibraryFolders(),
        debouncedSearch ? Promise.resolve(0) : api.countBooks(),
      ]);

      // Convert API books to Audiobook type
      const convertedBooks: Audiobook[] = books.map(book => ({
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

      setAudiobooks(convertedBooks);
      setTotalPages(Math.ceil((debouncedSearch ? books.length : bookCount) / limit));
      setHasLibraryFolders(folders.length > 0);

      // Load import paths
      const convertedPaths: ImportPath[] = folders.map(folder => ({
        id: folder.id.toString(),
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
  }, [debouncedSearch, filters, page]);

  const handleManualImport = () => {
    fileInputRef.current?.click();
  };

  const handleFileSelect = async (event: React.ChangeEvent<HTMLInputElement>) => {
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
      console.log('Batch updated:', selectedAudiobooks.length, 'audiobooks with', updates);

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

  const handleFiltersChange = (newFilters: FilterOptions) => {
    setFilters(newFilters);
    setPage(1); // Reset to first page on filter change
  };

  const getActiveFilterCount = () => {
    return Object.values(filters).filter((v) => v !== undefined && v !== '').length;
  };

  // Import path management handlers
  const handleAddImportPath = async () => {
    if (!newImportPath.trim()) return;

    try {
      const folder = await api.addLibraryFolder(newImportPath, newImportPath.split('/').pop() || 'Library');
      const newPath: ImportPath = {
        id: folder.id.toString(),
        path: folder.path,
        status: 'idle',
        book_count: folder.book_count,
      };
      setImportPaths((prev) => [...prev, newPath]);
      setNewImportPath('');
      setAddPathDialogOpen(false);
    } catch (error) {
      console.error('Failed to add import path:', error);
    }
  };

  const handleRemoveImportPath = async (id: string) => {
    try {
      await api.removeLibraryFolder(parseInt(id));
      setImportPaths((prev) => prev.filter((p) => p.id !== id));
    } catch (error) {
      console.error('Failed to remove import path:', error);
    }
  };

  const handleScanImportPath = async (id: string) => {
    try {
      const path = importPaths.find(p => p.id === id)?.path;
      setImportPaths((prev) =>
        prev.map((p) => (p.id === id ? { ...p, status: 'scanning' as const } : p))
      );
      await api.startScan(path);
      // Reload audiobooks after scan
      setTimeout(() => loadAudiobooks(), 1000);
    } catch (error) {
      console.error('Failed to scan import path:', error);
      setImportPaths((prev) =>
        prev.map((p) => (p.id === id ? { ...p, status: 'idle' as const } : p))
      );
    }
  };

  const handleBrowseFolder = () => {
    folderInputRef.current?.click();
  };

  const handleFolderSelect = (event: React.ChangeEvent<HTMLInputElement>) => {
    const files = event.target.files;
    if (files && files.length > 0) {
      const path = files[0].webkitRelativePath.split('/')[0];
      setNewImportPath(`/${path}`);
      setAddPathDialogOpen(true);
    }
  };

  return (
    <Box>
      <Box display="flex" justifyContent="space-between" alignItems="center" mb={3}>
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

      {/* Hidden file input for manual import */}
      <input
        ref={fileInputRef}
        type="file"
        multiple
        accept="audio/*,.m4b,.mp3,.m4a,.flac,.opus,.ogg"
        style={{ display: 'none' }}
        onChange={handleFileSelect}
      />

      {!hasLibraryFolders ? (
        <Paper sx={{ p: 4, textAlign: 'center', bgcolor: 'background.default' }}>
          <FolderOpenIcon sx={{ fontSize: 80, color: 'text.secondary', mb: 2 }} />
          <Alert severity="info" sx={{ textAlign: 'center' }}>
            <AlertTitle>No Import Paths Configured</AlertTitle>
            You haven't added any import paths yet. Get started by:
            <ul style={{ marginTop: 8, marginBottom: 0, textAlign: 'left' }}>
              <li>Importing individual audiobook files using the "Import Files" button below</li>
              <li>Adding import paths using the "Add Import Path" button below (watches folders for new files)</li>
            </ul>
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
            >
              Add Import Path
            </Button>
          </Box>
        </Paper>
      ) : (
        <Stack spacing={3}>
          <SearchBar
            value={searchQuery}
            onChange={setSearchQuery}
            viewMode={viewMode}
            onViewModeChange={setViewMode}
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
      <Dialog open={addPathDialogOpen} onClose={() => setAddPathDialogOpen(false)} maxWidth="sm" fullWidth>
        <DialogTitle>Add Import Path</DialogTitle>
        <DialogContent>
          <Alert severity="info" sx={{ mb: 2 }}>
            Import paths are watched for new audiobook files. Files found here will be imported into the library.
          </Alert>
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
            onClick={handleBrowseFolder}
            sx={{ mt: 2 }}
          >
            Browse
          </Button>
          <input
            ref={folderInputRef}
            type="file"
            {...({ webkitdirectory: '', directory: '' } as any)}
            multiple
            style={{ display: 'none' }}
            onChange={handleFolderSelect}
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setAddPathDialogOpen(false)}>Cancel</Button>
          <Button onClick={handleAddImportPath} variant="contained">
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
            <IconButton size="small">
              <ExpandMoreIcon
                sx={{
                  transform: importPathsExpanded ? 'rotate(180deg)' : 'rotate(0deg)',
                  transition: 'transform 0.3s',
                }}
              />
            </IconButton>
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
  );
};
