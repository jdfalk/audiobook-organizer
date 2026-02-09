// file: web/src/components/audiobooks/FilterSidebar.tsx
// version: 1.2.0
// guid: 2e3f4a5b-6c7d-8e9f-0a1b-2c3d4e5f6a7b

import React from 'react';
import {
  Drawer,
  Box,
  Typography,
  Button,
  Divider,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Chip,
  Stack,
} from '@mui/material';
import { FilterList as FilterListIcon } from '@mui/icons-material';
import type { FilterOptions } from '../../types';

interface FilterSidebarProps {
  open: boolean;
  onClose: () => void;
  filters: FilterOptions;
  onFiltersChange: (filters: FilterOptions) => void;
  authors?: string[];
  series?: string[];
  genres?: string[];
  languages?: string[];
}

export const FilterSidebar: React.FC<FilterSidebarProps> = ({
  open,
  onClose,
  filters,
  onFiltersChange,
  authors = [],
  series = [],
  genres = [],
  languages = [],
}) => {
  const handleFilterChange = (key: keyof FilterOptions, value: string) => {
    onFiltersChange({ ...filters, [key]: value || undefined });
  };

  const handleClearFilters = () => {
    onFiltersChange({});
  };

  const getActiveFilterCount = () => {
    return Object.values(filters).filter((v) => v !== undefined && v !== '')
      .length;
  };

  return (
    <Drawer anchor="right" open={open} onClose={onClose}>
      <Box sx={{ width: 320, p: 3 }}>
        <Box
          display="flex"
          alignItems="center"
          justifyContent="space-between"
          mb={2}
        >
          <Box display="flex" alignItems="center" gap={1}>
            <FilterListIcon />
            <Typography variant="h6">Filters</Typography>
            {getActiveFilterCount() > 0 && (
              <Chip
                label={getActiveFilterCount()}
                size="small"
                color="primary"
              />
            )}
          </Box>
          <Button size="small" onClick={handleClearFilters}>
            Clear All
          </Button>
        </Box>

        <Divider sx={{ mb: 3 }} />

        <Stack spacing={3}>
          <FormControl fullWidth>
            <InputLabel id="filter-library-state-label">Library State</InputLabel>
            <Select
              labelId="filter-library-state-label"
              value={filters.libraryState || ''}
              onChange={(e) =>
                handleFilterChange('libraryState', e.target.value)
              }
              label="Library State"
            >
              <MenuItem value="">
                <em>All States</em>
              </MenuItem>
              <MenuItem value="organized">Organized</MenuItem>
              <MenuItem value="import">Import</MenuItem>
              <MenuItem value="deleted">Deleted</MenuItem>
            </Select>
          </FormControl>

          <FormControl fullWidth>
            <InputLabel id="filter-author-label">Author</InputLabel>
            <Select
              labelId="filter-author-label"
              value={filters.author || ''}
              onChange={(e) => handleFilterChange('author', e.target.value)}
              label="Author"
            >
              <MenuItem value="">
                <em>All Authors</em>
              </MenuItem>
              {authors.map((author) => (
                <MenuItem key={author} value={author}>
                  {author}
                </MenuItem>
              ))}
            </Select>
          </FormControl>

          <FormControl fullWidth>
            <InputLabel id="filter-series-label">Series</InputLabel>
            <Select
              labelId="filter-series-label"
              value={filters.series || ''}
              onChange={(e) => handleFilterChange('series', e.target.value)}
              label="Series"
            >
              <MenuItem value="">
                <em>All Series</em>
              </MenuItem>
              {series.map((s) => (
                <MenuItem key={s} value={s}>
                  {s}
                </MenuItem>
              ))}
            </Select>
          </FormControl>

          <FormControl fullWidth>
            <InputLabel id="filter-genre-label">Genre</InputLabel>
            <Select
              labelId="filter-genre-label"
              value={filters.genre || ''}
              onChange={(e) => handleFilterChange('genre', e.target.value)}
              label="Genre"
            >
              <MenuItem value="">
                <em>All Genres</em>
              </MenuItem>
              {genres.map((genre) => (
                <MenuItem key={genre} value={genre}>
                  {genre}
                </MenuItem>
              ))}
            </Select>
          </FormControl>

          <FormControl fullWidth>
            <InputLabel id="filter-language-label">Language</InputLabel>
            <Select
              labelId="filter-language-label"
              value={filters.language || ''}
              onChange={(e) => handleFilterChange('language', e.target.value)}
              label="Language"
            >
              <MenuItem value="">
                <em>All Languages</em>
              </MenuItem>
              {languages.map((lang) => (
                <MenuItem key={lang} value={lang}>
                  {lang}
                </MenuItem>
              ))}
            </Select>
          </FormControl>

        </Stack>
      </Box>
    </Drawer>
  );
};
