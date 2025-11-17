// file: web/src/types/index.ts
// version: 1.3.0
// guid: 0d1e2f3a-4b5c-6d7e-8f9a-0b1c2d3e4f5a

// Audiobook (Book) type
export interface Audiobook {
  id: string;
  title: string;
  author?: string;
  narrator?: string;
  series?: string;
  series_number?: number;
  genre?: string;
  year?: number;
  print_year?: number;
  audiobook_release_year?: number;
  language?: string;
  publisher?: string;
  edition?: string;
  isbn10?: string;
  isbn13?: string;
  description?: string;
  duration_seconds?: number;
  file_path: string;
  original_filename?: string;
  cover_path?: string;
  file_size_bytes?: number;
  format?: string;
  bitrate_kbps?: number;

  // Media info fields (parsed from file)
  codec?: string;
  sample_rate_hz?: number;
  channels?: number;
  bit_depth?: number;
  quality?: string; // e.g., '320kbps AAC', '128kbps MP3', 'FLAC Lossless'

  // Version management
  is_primary_version?: boolean;
  version_group_id?: string; // Links multiple versions of same book
  version_notes?: string; // e.g., 'Remastered 2020', 'Original Recording'

  created_at: string;
  updated_at: string;
  work_id?: string;
}

// Alias for backend compatibility
export type Book = Audiobook;

// Work type
export interface Work {
  id: string;
  title: string;
  author?: string;
  series?: string;
  series_position?: number;
  description?: string;
  original_language?: string;
  first_published_year?: number;
  created_at: string;
  updated_at: string;
}

// Author type
export interface Author {
  id: string;
  name: string;
  biography?: string;
  birth_year?: number;
  death_year?: number;
  created_at: string;
  updated_at: string;
}

// Series type
export interface Series {
  id: string;
  title: string;
  description?: string;
  total_books?: number;
  created_at: string;
  updated_at: string;
}

// User type
export interface User {
  id: string;
  name: string;
  email: string;
}

// API Error type
export interface ApiError {
  message: string;
  code?: string;
  details?: Record<string, unknown>;
}

// Filter options for audiobook queries
export interface FilterOptions {
  search?: string;
  author?: string;
  series?: string;
  genre?: string;
  language?: string;
  sortBy?: 'title' | 'author' | 'year' | 'created_at';
  sortOrder?: 'asc' | 'desc';
}

// Pagination parameters
export interface PaginationParams {
  page: number;
  limit: number;
}

// Paginated response wrapper
export interface PaginatedResponse<T> {
  data: T[];
  total: number;
  page: number;
  limit: number;
  has_more: boolean;
}
