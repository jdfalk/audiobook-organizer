// API service layer for audiobook-organizer backend
// Provides typed functions for all backend endpoints

const API_BASE = '/api/v1';

// Response types
export interface Book {
  id: string;
  title: string;
  author_id?: number;
  author_name?: string;
  series_id?: number;
  series_name?: string;
  series_position?: number;
  file_path: string;
  duration?: number;
  narrator?: string;
  language?: string;
  publisher?: string;
  description?: string;
  cover_image?: string;
  isbn?: string;
  work_id?: string;
  edition?: string;
  print_year?: number;
  audiobook_release_year?: number;
  original_filename?: string;
  bitrate?: number;
  codec?: string;
  sample_rate?: number;
  channels?: number;
  bit_depth?: number;
  quality?: string;
  is_primary_version?: boolean;
  version_group_id?: string;
  version_notes?: string;
  created_at: string;
  updated_at: string;
}

export interface Author {
  id: number;
  name: string;
  created_at: string;
}

export interface Series {
  id: number;
  name: string;
  author_id?: number;
  created_at: string;
}

export interface Work {
  id: string;
  title: string;
  author_names?: string;
  description?: string;
  original_publish_year?: number;
  created_at: string;
  updated_at: string;
}

export interface LibraryFolder {
  id: number;
  path: string;
  name: string;
  enabled: boolean;
  created_at: string;
  last_scan?: string;
  book_count: number;
}

export interface Operation {
  id: string;
  type: string;
  status: string;
  progress: number;
  total: number;
  message: string;
  folder_path?: string;
  created_at: string;
  started_at?: string;
  completed_at?: string;
  error_message?: string;
}

export interface OperationLog {
  id: number;
  operation_id: string;
  level: string;
  message: string;
  details?: string;
  created_at: string;
}

export interface SystemStatus {
  status: string;
  library: {
    book_count: number;
    folder_count: number;
    total_size: number;
  };
  memory: {
    alloc_bytes: number;
    total_alloc_bytes: number;
    sys_bytes: number;
    num_gc: number;
  };
  runtime: {
    go_version: string;
    num_goroutine: number;
    num_cpu: number;
  };
  operations: {
    recent: Operation[];
  };
}

export interface SystemLogs {
  logs: Array<{
    operation_id: string;
    timestamp: string;
    level: string;
    message: string;
    details?: string;
  }>;
  total: number;
  limit: number;
  offset: number;
}

export interface MetadataSource {
  id: string;
  name: string;
  enabled: boolean;
  priority: number;
  requires_auth: boolean;
  credentials: {
    [key: string]: string;
  };
}

export interface Config {
  // Core paths
  root_dir: string;
  database_path: string;
  database_type: string;
  enable_sqlite: boolean;
  playlist_dir: string;

  // Library organization
  organization_strategy: string;
  scan_on_startup: boolean;
  auto_organize: boolean;
  folder_naming_pattern: string;
  file_naming_pattern: string;
  create_backups: boolean;

  // Storage quotas
  enable_disk_quota: boolean;
  disk_quota_percent: number;
  enable_user_quotas: boolean;
  default_user_quota_gb: number;

  // Metadata
  auto_fetch_metadata: boolean;
  metadata_sources: MetadataSource[];
  language: string;

  // Performance
  concurrent_scans: number;

  // Memory management
  memory_limit_type: string;
  cache_size: number;
  memory_limit_percent: number;
  memory_limit_mb: number;

  // Logging
  log_level: string;
  log_format: string;
  enable_json_logging: boolean;

  // Legacy fields
  api_keys: {
    goodreads: string;
  };
  supported_extensions: string[];
}

// API functions

// Books
export async function getBooks(limit = 100, offset = 0): Promise<Book[]> {
  const response = await fetch(`${API_BASE}/audiobooks?limit=${limit}&offset=${offset}`);
  if (!response.ok) throw new Error('Failed to fetch books');
  const data = await response.json();
  return data.audiobooks || [];
}

export async function getBook(id: string): Promise<Book> {
  const response = await fetch(`${API_BASE}/audiobooks/${id}`);
  if (!response.ok) throw new Error('Failed to fetch book');
  return response.json();
}

export async function searchBooks(query: string, limit = 50): Promise<Book[]> {
  const response = await fetch(`${API_BASE}/audiobooks/search?q=${encodeURIComponent(query)}&limit=${limit}`);
  if (!response.ok) throw new Error('Failed to search books');
  const data = await response.json();
  return data.audiobooks || [];
}

export async function countBooks(): Promise<number> {
  const response = await fetch(`${API_BASE}/audiobooks/count`);
  if (!response.ok) throw new Error('Failed to count books');
  const data = await response.json();
  return data.count || 0;
}

// Authors
export async function getAuthors(): Promise<Author[]> {
  const response = await fetch(`${API_BASE}/authors`);
  if (!response.ok) throw new Error('Failed to fetch authors');
  const data = await response.json();
  return data.authors || [];
}

// Series
export async function getSeries(): Promise<Series[]> {
  const response = await fetch(`${API_BASE}/series`);
  if (!response.ok) throw new Error('Failed to fetch series');
  const data = await response.json();
  return data.series || [];
}

// Works
export async function getWorks(): Promise<Work[]> {
  const response = await fetch(`${API_BASE}/works`);
  if (!response.ok) throw new Error('Failed to fetch works');
  const data = await response.json();
  return data.works || [];
}

// Library Folders (Import Paths)
export async function getLibraryFolders(): Promise<LibraryFolder[]> {
  const response = await fetch(`${API_BASE}/library/folders`);
  if (!response.ok) throw new Error('Failed to fetch library folders');
  const data = await response.json();
  return data.folders || [];
}

export async function addLibraryFolder(path: string, name: string): Promise<LibraryFolder> {
  const response = await fetch(`${API_BASE}/library/folders`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path, name }),
  });
  if (!response.ok) throw new Error('Failed to add library folder');
  return response.json();
}

export async function removeLibraryFolder(id: number): Promise<void> {
  const response = await fetch(`${API_BASE}/library/folders/${id}`, {
    method: 'DELETE',
  });
  if (!response.ok) throw new Error('Failed to remove library folder');
}

// Operations
export async function startScan(folderPath?: string, priority?: number): Promise<Operation> {
  const response = await fetch(`${API_BASE}/operations/scan`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ folder_path: folderPath, priority }),
  });
  if (!response.ok) throw new Error('Failed to start scan');
  return response.json();
}

export async function getOperationStatus(id: string): Promise<Operation> {
  const response = await fetch(`${API_BASE}/operations/${id}/status`);
  if (!response.ok) throw new Error('Failed to fetch operation status');
  return response.json();
}

export async function getOperationLogs(id: string): Promise<OperationLog[]> {
  const response = await fetch(`${API_BASE}/operations/${id}/logs`);
  if (!response.ok) throw new Error('Failed to fetch operation logs');
  const data = await response.json();
  return data.logs || [];
}

export async function cancelOperation(id: string): Promise<void> {
  const response = await fetch(`${API_BASE}/operations/${id}`, {
    method: 'DELETE',
  });
  if (!response.ok) throw new Error('Failed to cancel operation');
}

// System
export async function getSystemStatus(): Promise<SystemStatus> {
  const response = await fetch(`${API_BASE}/system/status`);
  if (!response.ok) throw new Error('Failed to fetch system status');
  return response.json();
}

export async function getSystemLogs(params?: {
  level?: string;
  search?: string;
  limit?: number;
  offset?: number;
}): Promise<SystemLogs> {
  const query = new URLSearchParams();
  if (params?.level) query.append('level', params.level);
  if (params?.search) query.append('search', params.search);
  if (params?.limit) query.append('limit', params.limit.toString());
  if (params?.offset) query.append('offset', params.offset.toString());

  const response = await fetch(`${API_BASE}/system/logs?${query}`);
  if (!response.ok) throw new Error('Failed to fetch system logs');
  return response.json();
}

// Config
export async function getConfig(): Promise<Config> {
  const response = await fetch(`${API_BASE}/config`);
  if (!response.ok) throw new Error('Failed to fetch config');
  const data = await response.json();
  return data.config;
}

export async function updateConfig(updates: Partial<Config>): Promise<Config> {
  const response = await fetch(`${API_BASE}/config`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(updates),
  });
  if (!response.ok) throw new Error('Failed to update config');
  const data = await response.json();
  return data.config;
}

// Version Management
export async function getBookVersions(bookId: string): Promise<Book[]> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/versions`);
  if (!response.ok) throw new Error('Failed to fetch book versions');
  const data = await response.json();
  return data.versions || [];
}

export async function linkBookVersion(bookId: string, otherBookId: string): Promise<void> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/versions`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ other_id: otherBookId }),
  });
  if (!response.ok) throw new Error('Failed to link book version');
}

export async function setPrimaryVersion(bookId: string): Promise<void> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/set-primary`, {
    method: 'PUT',
  });
  if (!response.ok) throw new Error('Failed to set primary version');
}

export async function getVersionGroup(groupId: string): Promise<Book[]> {
  const response = await fetch(`${API_BASE}/version-groups/${groupId}`);
  if (!response.ok) throw new Error('Failed to fetch version group');
  const data = await response.json();
  return data.audiobooks || [];
}

// File Import
export async function importFile(filePath: string, organize = false): Promise<{ message: string; book: Book; operation_id?: string }> {
  const response = await fetch(`${API_BASE}/import/file`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ file_path: filePath, organize }),
  });
  if (!response.ok) throw new Error('Failed to import file');
  return response.json();
}

// Metadata Fetching
export interface MetadataResult {
  title: string;
  author: string;
  description?: string;
  publisher?: string;
  publish_year?: number;
  isbn?: string;
  cover_url?: string;
  language?: string;
}

export async function searchMetadata(title: string, author?: string): Promise<{ results: MetadataResult[]; source: string }> {
  const params = new URLSearchParams({ title });
  if (author) params.append('author', author);

  const response = await fetch(`${API_BASE}/metadata/search?${params.toString()}`);
  if (!response.ok) throw new Error('Failed to search metadata');
  return response.json();
}

export async function fetchBookMetadata(bookId: string): Promise<{ message: string; book: Book; source: string }> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/fetch-metadata`, {
    method: 'POST',
  });
  if (!response.ok) throw new Error('Failed to fetch book metadata');
  return response.json();
}
