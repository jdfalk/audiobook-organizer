// file: web/src/components/FilterPanel.tsx
// version: 1.1.0
// guid: 5c7d8e9f-0a1b-2c3d-4e5f-6a7b8c9d0e1f
// last-edited: 2026-05-20

import React from 'react';
import {
  Box,
  IconButton,
  Tooltip,
} from '@mui/material';
import {
  Info as InfoIcon,
} from '@mui/icons-material';
import { SearchBar, ViewMode } from './audiobooks/SearchBar';
import type { ParsedSearch } from '../utils/searchParser';

interface FilterPanelProps {
  searchQuery: string;
  onSearchChange: (value: string) => void;
  onParsedSearchChange?: (parsed: ParsedSearch) => void;
  viewMode: ViewMode;
  onViewModeChange: (mode: ViewMode) => void;
  onLibraryInfoClick: () => void;
}

export const FilterPanel: React.FC<FilterPanelProps> = ({
  searchQuery,
  onSearchChange,
  onParsedSearchChange,
  viewMode,
  onViewModeChange,
  onLibraryInfoClick,
}) => {
  return (
    <Box display="flex" gap={1} alignItems="center">
      <Box flex={1}>
        <SearchBar
          value={searchQuery}
          onChange={onSearchChange}
          onParsedSearchChange={onParsedSearchChange}
          viewMode={viewMode}
          onViewModeChange={onViewModeChange}
        />
      </Box>
      <Tooltip title="Library info">
        <IconButton onClick={onLibraryInfoClick}>
          <InfoIcon />
        </IconButton>
      </Tooltip>
    </Box>
  );
};
