// file: web/src/types/index.ts
// version: 1.7.0
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
  cover_url?: string;
  narrators_json?: string;
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
  file_hash?: string;
  original_file_hash?: string;
  organized_file_hash?: string;
  library_state?: string;
  quantity?: number;
  marked_for_deletion?: boolean;
  marked_for_deletion_at?: string;
  organize_error?: string;

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

// Sort field enum for type safety
export enum SortField {
  Title = 'title',
  Author = 'author',
  Year = 'year',
  CreatedAt = 'created_at',
}

// Sort order enum for type safety
export enum SortOrder {
  Ascending = 'asc',
  Descending = 'desc',
}

// Filter options for audiobook queries
export interface FilterOptions {
  search?: string;
  author?: string;
  series?: string;
  genre?: string;
  language?: string;
  libraryState?: string;
  sortBy?: SortField;
  sortOrder?: SortOrder;
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
