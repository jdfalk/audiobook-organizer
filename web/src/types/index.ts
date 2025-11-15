// file: web/src/types/index.ts
// version: 1.0.0
// guid: 0d1e2f3a-4b5c-6d7e-8f9a-0b1c2d3e4f5a

export interface Book {
  id: string;
  title: string;
  author: string;
  narrator?: string;
  series?: string;
  series_position?: number;
  work_id?: string;
  language?: string;
  publisher?: string;
  isbn10?: string;
  isbn13?: string;
  duration_seconds?: number;
  file_path: string;
  cover_art_path?: string;
  file_size_bytes: number;
  format: string;
  bitrate_kbps?: number;
  created_at: string;
  updated_at: string;
}

export interface Work {
  id: string;
  title: string;
  author: string;
  series?: string;
  series_position?: number;
  description?: string;
  original_language?: string;
  first_published_year?: number;
  created_at: string;
  updated_at: string;
}

export interface Author {
  id: string;
  name: string;
  biography?: string;
  birth_year?: number;
  death_year?: number;
  created_at: string;
  updated_at: string;
}

export interface Series {
  id: string;
  title: string;
  description?: string;
  total_books?: number;
  created_at: string;
  updated_at: string;
}

export interface PaginatedResponse<T> {
  items: T[];
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
}

export interface ApiError {
  error: string;
  message?: string;
  details?: unknown;
}
