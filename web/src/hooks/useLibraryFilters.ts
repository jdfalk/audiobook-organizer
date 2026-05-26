// file: web/src/hooks/useLibraryFilters.ts
// version: 1.3.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890
// last-edited: 2026-05-26

import { useState, useEffect, useCallback } from 'react';
import type { FilterOptions } from '../types';
import * as api from '../services/api';

interface UseLibraryFiltersOptions {
  searchParams: URLSearchParams;
  /** Called when filters change so the caller can reset pagination to page 1. */
  onFiltersChange?: () => void;
}

export interface LibraryFiltersResult {
  filterOpen: boolean;
  setFilterOpen: (open: boolean) => void;
  filters: FilterOptions;
  setFilters: React.Dispatch<React.SetStateAction<FilterOptions>>;
  handleFiltersChange: (newFilters: FilterOptions) => void;
  selectedTags: string[];
  setSelectedTags: React.Dispatch<React.SetStateAction<string[]>>;
  handleTagFilterChange: (tags: string[]) => void;
  refreshTags: () => void;
  availableAuthors: string[];
  availableSeries: string[];
  availableGenres: string[];
  availableLanguages: string[];
  availableTags: Array<{ tag: string; count: number }>;
  getActiveFilterCount: () => number;
}

export function useLibraryFilters({
  searchParams,
  onFiltersChange,
}: UseLibraryFiltersOptions): LibraryFiltersResult {
  const [filterOpen, setFilterOpen] = useState(false);
  const [filters, setFilters] = useState<FilterOptions>(() => ({
    author: searchParams.get('author') || undefined,
    series: searchParams.get('series') || undefined,
    genre: searchParams.get('genre') || undefined,
    language: searchParams.get('language') || undefined,
    libraryState: searchParams.get('state') || undefined,
    hasFileErrors: (searchParams.get('has_file_errors') === 'true') || undefined,
    fingerprintStatus: (searchParams.get('fingerprint_status') as "complete" | "partial" | "none" | null) || undefined,
    coveragePercentMin: searchParams.get('coverage_percent_min') ? parseInt(searchParams.get('coverage_percent_min')!, 10) : undefined,
    coveragePercentMax: searchParams.get('coverage_percent_max') ? parseInt(searchParams.get('coverage_percent_max')!, 10) : undefined,
    // Quick-filter preset params
    missingCovers: (searchParams.get('missing_covers') === 'true') || undefined,
    inImportPath: (searchParams.get('in_import_path') === 'true') || undefined,
    noIsbn: (searchParams.get('no_isbn') === 'true') || undefined,
    duplicatesFlagged: (searchParams.get('duplicates_flagged') === 'true') || undefined,
  }));
  const [selectedTags, setSelectedTags] = useState<string[]>([]);
  const [availableAuthors, setAvailableAuthors] = useState<string[]>([]);
  const [availableSeries, setAvailableSeries] = useState<string[]>([]);
  const [availableGenres, setAvailableGenres] = useState<string[]>([]);
  const [availableLanguages, setAvailableLanguages] = useState<string[]>([]);
  const [availableTags, setAvailableTags] = useState<Array<{ tag: string; count: number }>>([]);

  useEffect(() => {
    api
      .listAllUserTags()
      .then((tags) => setAvailableTags(tags ?? []))
      .catch((_err) => {
        console.error('Failed to load tags:', _err);
      });
  }, []);

  useEffect(() => {
    api
      .getBookFacets()
      .then((facets) => {
        setAvailableGenres(facets.genres);
        setAvailableLanguages(facets.languages);
      })
      .catch((e) => {
        console.error('Failed to load facets:', e);
      });
  }, []);

  useEffect(() => {
    api
      .getAuthors()
      .then((authors) => {
        setAvailableAuthors(authors.map((a) => a.name).filter(Boolean).sort());
      })
      .catch((e) => {
        console.error('Failed to load authors:', e);
      });
  }, []);

  useEffect(() => {
    api
      .getSeries()
      .then((series) => {
        setAvailableSeries(series.map((s) => s.name).filter(Boolean).sort());
      })
      .catch((e) => {
        console.error('Failed to load series:', e);
      });
  }, []);

  const handleFiltersChange = useCallback(
    (newFilters: FilterOptions) => {
      setFilters(newFilters);
      onFiltersChange?.();
    },
    [onFiltersChange]
  );

  const handleTagFilterChange = useCallback((tags: string[]) => {
    setSelectedTags(tags);
    setFilters((prev) => ({ ...prev, tags: tags.length > 0 ? tags : undefined }));
  }, []);

  const refreshTags = useCallback(() => {
    api
      .listAllUserTags()
      .then((tags) => setAvailableTags(tags ?? []))
      .catch((_err) => {
        console.error('Failed to refresh tags:', _err);
      });
  }, []);

  // Sync filters whenever searchParams change (e.g., when navigating to /library with no params)
  useEffect(() => {
    setFilters({
      author: searchParams.get('author') || undefined,
      series: searchParams.get('series') || undefined,
      genre: searchParams.get('genre') || undefined,
      language: searchParams.get('language') || undefined,
      libraryState: searchParams.get('state') || undefined,
      hasFileErrors: (searchParams.get('has_file_errors') === 'true') || undefined,
      fingerprintStatus: (searchParams.get('fingerprint_status') as "complete" | "partial" | "none" | null) || undefined,
      coveragePercentMin: searchParams.get('coverage_percent_min') ? parseInt(searchParams.get('coverage_percent_min')!, 10) : undefined,
      coveragePercentMax: searchParams.get('coverage_percent_max') ? parseInt(searchParams.get('coverage_percent_max')!, 10) : undefined,
      missingCovers: (searchParams.get('missing_covers') === 'true') || undefined,
      inImportPath: (searchParams.get('in_import_path') === 'true') || undefined,
      noIsbn: (searchParams.get('no_isbn') === 'true') || undefined,
      duplicatesFlagged: (searchParams.get('duplicates_flagged') === 'true') || undefined,
    });
  }, [searchParams]);

  const getActiveFilterCount = useCallback(
    () => Object.values(filters).filter((v) => v !== undefined && v !== '').length,
    [filters]
  );

  return {
    filterOpen,
    setFilterOpen,
    filters,
    setFilters,
    handleFiltersChange,
    selectedTags,
    setSelectedTags,
    handleTagFilterChange,
    refreshTags,
    availableAuthors,
    availableSeries,
    availableGenres,
    availableLanguages,
    availableTags,
    getActiveFilterCount,
  };
}
