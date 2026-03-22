// file: web/src/components/audiobooks/SearchBar.tsx
// version: 1.4.0
// guid: 1d2e3f4a-5b6c-7d8e-9f0a-1b2c3d4e5f6a

import React, { useMemo } from 'react';
import {
  TextField,
  InputAdornment,
  IconButton,
  Box,
  Chip,
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
import { SortField, SortOrder } from '../../types';
import { parseSearch, type ParsedSearch } from '../../utils/searchParser';

export type ViewMode = 'grid' | 'list';

interface SearchBarProps {
  value: string;
  onChange: (value: string) => void;
  onParsedSearchChange?: (parsed: ParsedSearch) => void;
  viewMode: ViewMode;
  onViewModeChange: (mode: ViewMode) => void;
  sortBy?: SortField;
  onSortChange?: (sort: SortField) => void;
  sortOrder?: SortOrder;
  onSortOrderChange?: (order: SortOrder) => void;
  placeholder?: string;
}

export const SearchBar: React.FC<SearchBarProps> = ({
  value,
  onChange,
  onParsedSearchChange,
  viewMode,
  onViewModeChange,
  sortBy = SortField.Title,
  onSortChange,
  sortOrder = SortOrder.Ascending,
  onSortOrderChange,
  placeholder = 'Search audiobooks... (try author:"Name" tag:scifi)',
}) => {
  const parsed = useMemo(() => parseSearch(value), [value]);

  const handleChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    const newValue = event.target.value;
    onChange(newValue);
    if (onParsedSearchChange) {
      onParsedSearchChange(parseSearch(newValue));
    }
  };

  const handleClear = () => {
    onChange('');
    if (onParsedSearchChange) {
      onParsedSearchChange({ freeText: '', fieldFilters: [] });
    }
  };

  const handleRemoveFilter = (index: number) => {
    const filter = parsed.fieldFilters[index];
    // Reconstruct the token string that was in the input
    const prefix = filter.negated ? (value.includes('NOT ') ? 'NOT ' : '-') : '';
    const valStr = filter.quoted ? `"${filter.value}"` : filter.value;
    const token = `${prefix}${filter.field}:${valStr}`;
    // Remove the token from the raw input
    const newValue = value.replace(token, '').replace(/\s{2,}/g, ' ').trim();
    onChange(newValue);
    if (onParsedSearchChange) {
      onParsedSearchChange(parseSearch(newValue));
    }
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
    <Box display="flex" flexDirection="column" gap={1}>
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
            onChange={(e) => onSortChange(e.target.value as SortField)}
          >
            <MenuItem value={SortField.Title}>Title</MenuItem>
            <MenuItem value={SortField.Author}>Author</MenuItem>
            <MenuItem value={SortField.Series}>Series</MenuItem>
            <MenuItem value={SortField.Year}>Year</MenuItem>
            <MenuItem value={SortField.CreatedAt}>Date Added</MenuItem>
          </Select>
        </FormControl>
      )}

      {onSortOrderChange && (
        <FormControl sx={{ minWidth: 140 }}>
          <InputLabel id="sort-order-label">Order</InputLabel>
          <Select
            labelId="sort-order-label"
            value={sortOrder}
            label="Order"
            onChange={(e) => onSortOrderChange(e.target.value as SortOrder)}
          >
            <MenuItem value={SortOrder.Ascending}>Ascending</MenuItem>
            <MenuItem value={SortOrder.Descending}>Descending</MenuItem>
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

      {parsed.fieldFilters.length > 0 && (
        <Box display="flex" gap={0.5} flexWrap="wrap">
          {parsed.fieldFilters.map((filter, index) => {
            const label = `${filter.negated ? 'NOT ' : ''}${filter.field}:${filter.quoted ? `"${filter.value}"` : filter.value}`;
            return (
              <Chip
                key={`${filter.field}-${filter.value}-${index}`}
                label={label}
                size="small"
                color={filter.negated ? 'error' : 'primary'}
                variant="outlined"
                onDelete={() => handleRemoveFilter(index)}
              />
            );
          })}
        </Box>
      )}
    </Box>
  );
};
