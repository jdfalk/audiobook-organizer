// file: web/src/services/api.ts
// version: 1.6.1
// guid: a0b1c2d3-e4f5-6789-abcd-ef0123456789

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
  format?: string;
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
  file_hash?: string;
  original_file_hash?: string;
  organized_file_hash?: string;
  library_state?: string;
  quantity?: number;
  marked_for_deletion?: boolean;
  marked_for_deletion_at?: string;
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

export interface TagSourceValues {
  file_value?: string | number | null;
  fetched_value?: string | number | null;
  stored_value?: string | number | null;
  override_value?: string | number | null;
  override_locked?: boolean;
}

export interface BookTags {
  media_info?: {
    codec?: string;
    bitrate?: number;
    sample_rate?: number;
    channels?: number;
    bit_depth?: number;
    quality?: string;
    duration?: number;
  };
  tags?: Record<string, TagSourceValues>;
}

export interface ImportPath {
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

// Active operations (subset when listing current queue state)
export interface ActiveOperationSummary {
  id: string;
  type: string;
  status: string;
  progress: number;
  total: number;
  message: string;
}

export interface SystemStatus {
  status: string;
  library_book_count?: number;
  import_book_count?: number;
  total_book_count?: number;
  library_size_bytes?: number;
  import_size_bytes?: number;
  total_size_bytes?: number;
  root_directory?: string;
  library: {
    book_count: number;
    folder_count: number;
    total_size: number;
    path?: string;
  };
  import_paths: {
    book_count: number;
    folder_count: number;
    total_size: number;
  };
  memory: {
    alloc_bytes: number;
    total_alloc_bytes: number;
    sys_bytes: number;
    num_gc: number;
    heap_alloc: number;
    heap_sys: number;
  };
  runtime: {
    go_version: string;
    num_goroutine: number;
    num_cpu: number;
    os: string;
    arch: string;
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

  // AI parsing
  enable_ai_parsing: boolean;
  openai_api_key: string;

  // Performance
  concurrent_scans: number;

  // Memory management
  memory_limit_type: string;
  cache_size: number;
  memory_limit_percent: number;
  memory_limit_mb: number;

  // Lifecycle / retention
  purge_soft_deleted_after_days?: number;
  purge_soft_deleted_delete_files?: boolean;

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
  return data.items || [];
}

export async function getBook(id: string): Promise<Book> {
  const response = await fetch(`${API_BASE}/audiobooks/${id}`);
  if (!response.ok) throw new Error('Failed to fetch book');
  return response.json();
}

export async function searchBooks(query: string, limit = 50): Promise<Book[]> {
  const response = await fetch(
    `${API_BASE}/audiobooks/search?q=${encodeURIComponent(query)}&limit=${limit}`
  );
  if (!response.ok) throw new Error('Failed to search books');
  const data = await response.json();
  return data.items || [];
}

export async function countBooks(): Promise<number> {
  const response = await fetch(`${API_BASE}/audiobooks/count`);
  if (!response.ok) throw new Error('Failed to count books');
  const data = await response.json();
  return data.count || 0;
}

export async function getSoftDeletedBooks(
  limit = 100,
  offset = 0,
  olderThanDays?: number
): Promise<{ items: Book[]; count: number }> {
  const params = new URLSearchParams({
    limit: String(limit),
    offset: String(offset),
  });
  if (olderThanDays && olderThanDays > 0) {
    params.set('older_than_days', String(olderThanDays));
  }
  const response = await fetch(`${API_BASE}/audiobooks/soft-deleted?${params.toString()}`);
  if (!response.ok) throw new Error('Failed to fetch soft-deleted books');
  const data = await response.json();
  return {
    items: data.items || [],
    count: data.total ?? data.count ?? (data.items ? data.items.length : 0),
  };
}

export async function purgeSoftDeletedBooks(
  deleteFiles = false,
  olderThanDays?: number
): Promise<{ attempted: number; purged: number; files_deleted: number; errors: string[] }> {
  const params = new URLSearchParams({
    delete_files: String(deleteFiles),
  });
  if (olderThanDays && olderThanDays > 0) {
    params.set('older_than_days', String(olderThanDays));
  }
  const response = await fetch(`${API_BASE}/audiobooks/purge-soft-deleted?${params.toString()}`, {
    method: 'DELETE',
  });
  if (!response.ok) throw new Error('Failed to purge soft-deleted books');
  return response.json();
}

export async function restoreSoftDeletedBook(bookId: string): Promise<void> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/restore`, {
    method: 'POST',
  });
  if (!response.ok) throw new Error('Failed to restore audiobook');
}

export async function deleteBook(
  bookId: string,
  options: { softDelete?: boolean; blockHash?: boolean } = {}
): Promise<void> {
  const params = new URLSearchParams();
  if (options.softDelete) params.set('soft_delete', 'true');
  if (options.blockHash) params.set('block_hash', 'true');
  const query = params.toString();
  const url =
    query.length > 0 ? `${API_BASE}/audiobooks/${bookId}?${query}` : `${API_BASE}/audiobooks/${bookId}`;
  const response = await fetch(url, {
    method: 'DELETE',
  });
  if (!response.ok) throw new Error('Failed to delete audiobook');
}

type OverridePayload = {
  value?: unknown;
  locked?: boolean;
  fetched_value?: unknown;
  clear?: boolean;
};

export async function updateBook(
  bookId: string,
  updates: Partial<Book> & { overrides?: Record<string, OverridePayload>; unlock_overrides?: string[] }
): Promise<Book> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(updates),
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data.error || 'Failed to update audiobook');
  }
  return response.json();
}

export async function getBookTags(bookId: string): Promise<BookTags> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/tags`);
  if (!response.ok) throw new Error('Failed to fetch book tags');
  return response.json();
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

// Import Paths
export async function getImportPaths(): Promise<ImportPath[]> {
  const response = await fetch(`${API_BASE}/import-paths`);
  if (!response.ok) throw new Error('Failed to fetch import paths');
  const data = await response.json();
  return data.importPaths || [];
}

export async function addImportPath(path: string, name: string): Promise<ImportPath> {
  const response = await fetch(`${API_BASE}/import-paths`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path, name }),
  });
  if (!response.ok) throw new Error('Failed to add import path');
  const data = await response.json();
  // Server returns { importPath, scan_operation_id?: string }
  // Gracefully handle both shapes
  return (data.importPath ? data.importPath : data) as ImportPath;
}

// Detailed add returning scan operation id when auto-scan kicks off
export interface AddImportPathDetailedResponse {
  importPath: ImportPath;
  scan_operation_id?: string;
}

export async function addImportPathDetailed(
  path: string,
  name: string
): Promise<AddImportPathDetailedResponse> {
  const response = await fetch(`${API_BASE}/import-paths`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path, name }),
  });
  if (!response.ok) throw new Error('Failed to add import path');
  const data = await response.json();
  if (data.importPath) {
    return {
      importPath: data.importPath,
      scan_operation_id: data.scan_operation_id,
    };
  }
  // Legacy shape fallback
  return { importPath: data };
}

// Operation status polling

export async function removeImportPath(id: number): Promise<void> {
  const response = await fetch(`${API_BASE}/import-paths/${id}`, {
    method: 'DELETE',
  });
  if (!response.ok) throw new Error('Failed to remove import path');
}

// Operations
export async function startScan(
  folderPath?: string,
  priority?: number,
  forceUpdate?: boolean
): Promise<Operation> {
  const response = await fetch(`${API_BASE}/operations/scan`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      folder_path: folderPath,
      priority,
      force_update: forceUpdate,
    }),
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

export async function getOperationLogsTail(id: string, tail: number): Promise<OperationLog[]> {
  const response = await fetch(`${API_BASE}/operations/${id}/logs?tail=${tail}`);
  if (!response.ok) throw new Error('Failed to fetch operation logs tail');
  const data = await response.json();
  return data.items || data.logs || [];
}

export async function cancelOperation(id: string): Promise<void> {
  const response = await fetch(`${API_BASE}/operations/${id}`, {
    method: 'DELETE',
  });
  if (!response.ok) throw new Error('Failed to cancel operation');
}

export async function getActiveOperations(): Promise<ActiveOperationSummary[]> {
  const response = await fetch(`${API_BASE}/operations/active`);
  if (!response.ok) throw new Error('Failed to fetch active operations');
  const data = await response.json();
  return data.operations || [];
}

// System
export async function getSystemStatus(): Promise<SystemStatus> {
  const response = await fetch(`${API_BASE}/system/status`);
  if (!response.ok) throw new Error('Failed to fetch system status');
  return response.json();
}

// Organize operation
export async function startOrganize(folderPath?: string, priority?: number): Promise<Operation> {
  const response = await fetch(`${API_BASE}/operations/organize`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ folder_path: folderPath, priority }),
  });
  if (!response.ok) throw new Error('Failed to start organize');
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
export async function importFile(
  filePath: string,
  organize = false
): Promise<{ message: string; book: Book; operation_id?: string }> {
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

export async function searchMetadata(
  title: string,
  author?: string
): Promise<{ results: MetadataResult[]; source: string }> {
  const params = new URLSearchParams({ title });
  if (author) params.append('author', author);

  const response = await fetch(`${API_BASE}/metadata/search?${params.toString()}`);
  if (!response.ok) throw new Error('Failed to search metadata');
  return response.json();
}

export async function fetchBookMetadata(
  bookId: string
): Promise<{ message: string; book: Book; source: string }> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/fetch-metadata`, {
    method: 'POST',
  });
  if (!response.ok) throw new Error('Failed to fetch metadata');
  return response.json();
}

// AI Parsing
export interface AIParseResult {
  title: string;
  author: string;
  series?: string;
  series_number?: number;
  narrator?: string;
  publisher?: string;
  year?: number;
  confidence: 'high' | 'medium' | 'low';
}

export async function parseFilenameWithAI(filename: string): Promise<{ metadata: AIParseResult }> {
  const response = await fetch(`${API_BASE}/ai/parse-filename`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ filename }),
  });
  if (!response.ok) throw new Error('Failed to parse filename with AI');
  return response.json();
}

export async function testAIConnection(
  apiKey?: string
): Promise<{ success: boolean; message?: string; error?: string }> {
  const response = await fetch(`${API_BASE}/ai/test-connection`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: apiKey ? JSON.stringify({ api_key: apiKey }) : undefined,
  });
  if (!response.ok) {
    const data = await response.json();
    throw new Error(data.error || 'Connection test failed');
  }
  return response.json();
}

export async function parseAudiobookWithAI(
  bookId: string
): Promise<{ message: string; book: Book; confidence: string }> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/parse-with-ai`, {
    method: 'POST',
  });
  if (!response.ok) throw new Error('Failed to parse audiobook with AI');
  return response.json();
}

// Filesystem Browsing
export interface FileSystemItem {
  name: string;
  path: string;
  is_dir: boolean;
  size?: number;
  mod_time?: number;
  excluded: boolean;
}

export interface FilesystemBrowseResult {
  path: string;
  items: FileSystemItem[];
  count: number;
  disk_info?: {
    exists: boolean;
    readable: boolean;
    writable: boolean;
  };
}

export async function browseFilesystem(path: string): Promise<FilesystemBrowseResult> {
  const response = await fetch(`${API_BASE}/filesystem/browse?path=${encodeURIComponent(path)}`);
  if (!response.ok) throw new Error('Failed to browse filesystem');
  return response.json();
}

// Blocked Hashes Management
export interface BlockedHash {
  hash: string;
  reason: string;
  created_at: string;
}

export interface BlockedHashesResponse {
  items: BlockedHash[];
  total: number;
}

export async function getBlockedHashes(): Promise<BlockedHashesResponse> {
  const response = await fetch(`${API_BASE}/blocked-hashes`);
  if (!response.ok) throw new Error('Failed to fetch blocked hashes');
  return response.json();
}

export async function addBlockedHash(
  hash: string,
  reason: string
): Promise<{ message: string; hash: string; reason: string }> {
  const response = await fetch(`${API_BASE}/blocked-hashes`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ hash, reason }),
  });
  if (!response.ok) {
    const data = await response.json();
    throw new Error(data.error || 'Failed to add blocked hash');
  }
  return response.json();
}

export async function removeBlockedHash(hash: string): Promise<{ message: string; hash: string }> {
  const response = await fetch(`${API_BASE}/blocked-hashes/${hash}`, {
    method: 'DELETE',
  });
  if (!response.ok) {
    const data = await response.json();
    throw new Error(data.error || 'Failed to remove blocked hash');
  }
  return response.json();
}
