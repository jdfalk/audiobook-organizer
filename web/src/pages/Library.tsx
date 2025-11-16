// file: web/src/pages/Library.tsx
// version: 1.3.0
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
} from '@mui/material';
import { 
  FilterList as FilterListIcon,
  Upload as UploadIcon,
  FolderOpen as FolderOpenIcon,
} from '@mui/icons-material';
import { AudiobookGrid } from '../components/audiobooks/AudiobookGrid';
import { AudiobookList } from '../components/audiobooks/AudiobookList';
import { SearchBar, ViewMode } from '../components/audiobooks/SearchBar';
import { FilterSidebar } from '../components/audiobooks/FilterSidebar';
import { MetadataEditDialog } from '../components/audiobooks/MetadataEditDialog';
import { BatchEditDialog } from '../components/audiobooks/BatchEditDialog';
import type { Audiobook, FilterOptions } from '../types';

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
  const fileInputRef = useRef<HTMLInputElement>(null);

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
      // TODO: Replace with actual API call
      // const response = await fetch('/api/v1/audiobooks?' + new URLSearchParams({
      //   page: page.toString(),
      //   limit: '24',
      //   search: debouncedSearch,
      //   ...filters
      // }));
      // const data = await response.json();
      // setAudiobooks(data.data);
      // setTotalPages(Math.ceil(data.total / 24));

      // Check if library folders exist
      // const foldersResponse = await fetch('/api/v1/library-folders');
      // const foldersData = await foldersResponse.json();
      // setHasLibraryFolders(foldersData.folders && foldersData.folders.length > 0);

      // Placeholder data for now
      setAudiobooks([]);
      setTotalPages(1);
      setHasLibraryFolders(false); // Set to false to show empty state
    } catch (error) {
      console.error('Failed to load audiobooks:', error);
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

  const handleFiltersChange = (newFilters: FilterOptions) => {
    setFilters(newFilters);
    setPage(1); // Reset to first page on filter change
  };

  const getActiveFilterCount = () => {
    return Object.values(filters).filter((v) => v !== undefined && v !== '').length;
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
          <Alert severity="info" sx={{ maxWidth: 600, mx: 'auto' }}>
            <AlertTitle>No Library Paths Imported</AlertTitle>
            You haven't added any library folders yet. Get started by either:
            <ul style={{ marginTop: 8, marginBottom: 0, textAlign: 'left' }}>
              <li>Importing individual audiobook files using the "Import Files" button above</li>
              <li>Adding library folders in the <a href="/file-manager" style={{ color: 'inherit', textDecoration: 'underline' }}>File Manager</a></li>
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
              startIcon={<FolderOpenIcon />}
              href="/file-manager"
            >
              Add Library Folders
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
    </Box>
  );
};
