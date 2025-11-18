// file: web/src/components/audiobooks/SearchBar.tsx
// version: 1.0.0
// guid: 1d2e3f4a-5b6c-7d8e-9f0a-1b2c3d4e5f6a

import React from 'react';
import {
  TextField,
  InputAdornment,
  IconButton,
  Box,
  ToggleButton,
  ToggleButtonGroup,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
} from '@mui/material';
import {
  Search as SearchIcon,
  Clear as ClearIcon,
  GridView as GridViewIcon,
  ViewList as ViewListIcon,
} from '@mui/icons-material';

export type ViewMode = 'grid' | 'list';
export type SortOption = 'title' | 'author' | 'date_added' | 'date_modified';

interface SearchBarProps {
  value: string;
  onChange: (value: string) => void;
  viewMode: ViewMode;
  onViewModeChange: (mode: ViewMode) => void;
  sortBy?: SortOption;
  onSortChange?: (sort: SortOption) => void;
  placeholder?: string;
}

export const SearchBar: React.FC<SearchBarProps> = ({
  value,
  onChange,
  viewMode,
  onViewModeChange,
  sortBy = 'title',
  onSortChange,
  placeholder = 'Search audiobooks...',
}) => {
  const handleChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    onChange(event.target.value);
  };

  const handleClear = () => {
    onChange('');
  };

  const handleViewModeChange = (
    _event: React.MouseEvent<HTMLElement>,
    newMode: ViewMode | null
  ) => {
    if (newMode !== null) {
      onViewModeChange(newMode);
    }
  };

  return (
    <Box display="flex" gap={2} alignItems="center">
      <TextField
        fullWidth
        value={value}
        onChange={handleChange}
        placeholder={placeholder}
        InputProps={{
          startAdornment: (
            <InputAdornment position="start">
              <SearchIcon />
            </InputAdornment>
          ),
          endAdornment: value && (
            <InputAdornment position="end">
              <IconButton size="small" onClick={handleClear}>
                <ClearIcon />
              </IconButton>
            </InputAdornment>
          ),
        }}
      />
      
      {onSortChange && (
        <FormControl sx={{ minWidth: 150 }}>
          <InputLabel id="sort-select-label">Sort by</InputLabel>
          <Select
            labelId="sort-select-label"
            value={sortBy}
            label="Sort by"
            onChange={(e) => onSortChange(e.target.value as SortOption)}
          >
            <MenuItem value="title">Title</MenuItem>
            <MenuItem value="author">Author</MenuItem>
            <MenuItem value="date_added">Date Added</MenuItem>
            <MenuItem value="date_modified">Date Modified</MenuItem>
          </Select>
        </FormControl>
      )}

      <ToggleButtonGroup
        value={viewMode}
        exclusive
        onChange={handleViewModeChange}
        aria-label="view mode"
      >
        <ToggleButton value="grid" aria-label="grid view">
          <GridViewIcon />
        </ToggleButton>
        <ToggleButton value="list" aria-label="list view">
          <ViewListIcon />
        </ToggleButton>
      </ToggleButtonGroup>
    </Box>
  );
};
