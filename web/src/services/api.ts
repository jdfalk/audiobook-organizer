// file: web/src/services/api.ts
// version: 1.26.0
// guid: a0b1c2d3-e4f5-6789-abcd-ef0123456789

// API service layer for audiobook-organizer backend
// Provides typed functions for all backend endpoints

const API_BASE = '/api/v1';

export class ApiError extends Error {
  readonly status: number;
  readonly data?: unknown;

  constructor(message: string, status: number, data?: unknown) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.data = data;
  }
}

export interface DeleteBookResponse {
  message: string;
  blocked?: boolean;
  soft_delete?: boolean;
}

const buildApiError = async (
  response: Response,
  fallbackMessage: string
) => {
  const data = await response.json().catch(() => ({}));
  const message =
    typeof (data as { error?: string }).error === 'string'
      ? (data as { error: string }).error
      : fallbackMessage;
  return new ApiError(message, response.status, data);
};

// Response types
export interface BookAuthorEntry {
  id: number;
  name: string;
  role: string;
  position: number;
}

export interface BookNarratorEntry {
  id: number;
  name: string;
  role: string;
  position: number;
}

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
  authors?: BookAuthorEntry[];
  narrators?: BookNarratorEntry[];
  language?: string;
  publisher?: string;
  description?: string;
  cover_image?: string;
  cover_url?: string;
  isbn?: string;
  isbn10?: string;
  isbn13?: string;
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
  itunes_persistent_id?: string;
  library_state?: string;
  quantity?: number;
  marked_for_deletion?: boolean;
  marked_for_deletion_at?: string;
  organize_error?: string;
  metadata_provenance?: Record<string, TagSourceValues>;
  metadata_provenance_at?: string;
  created_at: string;
  updated_at: string;
  metadata_updated_at?: string;
  last_written_at?: string;
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
  author_id?: number;
  series_id?: number;
  author_names?: string;
  alt_titles?: string[];
  description?: string;
  original_publish_year?: number;
  created_at?: string;
  updated_at?: string;
}

export interface TagSourceValues {
  file_value?: string | number | boolean | null;
  fetched_value?: string | number | boolean | null;
  stored_value?: string | number | boolean | null;
  override_value?: string | number | boolean | null;
  override_locked?: boolean;
  effective_value?: string | number | boolean | null;
  effective_source?: string;
  updated_at?: string;
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

export interface BookSegment {
  id: string;
  file_path: string;
  format: string;
  size_bytes: number;
  duration_seconds: number;
  track_number?: number;
  total_tracks?: number;
  active: boolean;
}

export interface SegmentTags {
  segment_id: string;
  file_path: string;
  format: string;
  size_bytes: number;
  duration_sec: number;
  track_number?: number;
  total_tracks?: number;
  tags: Record<string, string>;
  used_filename_fallback: boolean;
  tags_read_error?: string;
}

export interface PathMapping {
  from: string;
  to: string;
}

export interface ITunesValidateRequest {
  library_path: string;
  path_mappings?: PathMapping[];
}

export interface ITunesValidateResponse {
  total_tracks: number;
  audiobook_tracks: number;
  files_found: number;
  files_missing: number;
  missing_paths?: string[];
  path_prefixes?: string[];
  duplicate_count: number;
  estimated_import_time: string;
}

export interface ITunesImportRequest {
  library_path: string;
  import_mode: 'organized' | 'import' | 'organize';
  preserve_location: boolean;
  import_playlists: boolean;
  skip_duplicates: boolean;
  path_mappings?: PathMapping[];
}

export interface ITunesImportResponse {
  operation_id: string;
  status: string;
  message: string;
}

export interface ITunesWriteBackRequest {
  library_path: string;
  audiobook_ids: string[];
  create_backup: boolean;
  force_overwrite?: boolean;
}

export interface ITunesLibraryStatus {
  changed_since_import: boolean;
  fingerprint_stored: string;
  last_imported: string;
  last_external_change: string;
}

export interface ITunesWriteBackResponse {
  success: boolean;
  updated_count: number;
  backup_path?: string;
  message: string;
}

export interface ITunesImportStatus {
  operation_id: string;
  status: string;
  progress: number;
  message: string;
  total_books?: number;
  processed?: number;
  imported?: number;
  skipped?: number;
  failed?: number;
  errors?: string[];
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
  errors?: string[];
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
  version?: string;
  library_book_count?: number;
  import_book_count?: number;
  total_book_count?: number;
  author_count?: number;
  series_count?: number;
  library_size_bytes?: number;
  import_size_bytes?: number;
  total_size_bytes?: number;
  disk_total_bytes?: number;
  disk_used_bytes?: number;
  disk_free_bytes?: number;
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

export interface SystemStorage {
  path: string;
  total_bytes: number;
  used_bytes: number;
  free_bytes: number;
  percent_used: number;
  quota_enabled: boolean;
  quota_percent: number;
  user_quotas_enabled: boolean;
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
  setup_complete?: boolean;

  // Library organization
  organization_strategy: string;
  scan_on_startup: boolean;
  auto_organize: boolean;
  folder_naming_pattern: string;
  file_naming_pattern: string;
  create_backups: boolean;
  supported_extensions: string[];
  exclude_patterns?: string[];

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

  // Auto-update
  auto_update_enabled?: boolean;
  auto_update_channel?: string;
  auto_update_check_minutes?: number;
  auto_update_window_start?: number;
  auto_update_window_end?: number;

  // Legacy fields (Goodreads deprecated Dec 2020, removed)
  api_keys: Record<string, never>;
}

export interface AuthStatus {
  has_users: boolean;
  auth_enabled?: boolean;
  requires_auth: boolean;
  bootstrap_ready: boolean;
}

export interface AuthUser {
  id: string;
  username: string;
  email: string;
  roles: string[];
  status: string;
  created_at: string;
}

export interface AuthSession {
  id: string;
  user_id: string;
  expires_at: string;
  ip_address?: string;
  user_agent?: string;
  revoked?: boolean;
  created_at: string;
  current?: boolean;
}

// API functions

// Books
export async function getBooks(limit = 100, offset = 0): Promise<Book[]> {
  const response = await fetch(
    `${API_BASE}/audiobooks?limit=${limit}&offset=${offset}`
  );
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch books');
  }
  const data = await response.json();
  return data.items || [];
}

export async function getBook(id: string): Promise<Book> {
  const response = await fetch(`${API_BASE}/audiobooks/${id}`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch book');
  }
  return response.json();
}

export async function searchBooks(query: string, limit = 50): Promise<Book[]> {
  const response = await fetch(
    `${API_BASE}/audiobooks/search?q=${encodeURIComponent(query)}&limit=${limit}`
  );
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to search books');
  }
  const data = await response.json();
  return data.items || [];
}

export async function countBooks(): Promise<number> {
  const response = await fetch(`${API_BASE}/audiobooks/count`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to count books');
  }
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
  const response = await fetch(
    `${API_BASE}/audiobooks/soft-deleted?${params.toString()}`
  );
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch soft-deleted books');
  }
  const data = await response.json();
  return {
    items: data.items || [],
    count: data.total ?? data.count ?? (data.items ? data.items.length : 0),
  };
}

export async function purgeSoftDeletedBooks(
  deleteFiles = false,
  olderThanDays?: number
): Promise<{
  attempted: number;
  purged: number;
  files_deleted: number;
  errors: string[];
}> {
  const params = new URLSearchParams({
    delete_files: String(deleteFiles),
  });
  if (olderThanDays && olderThanDays > 0) {
    params.set('older_than_days', String(olderThanDays));
  }
  const response = await fetch(
    `${API_BASE}/audiobooks/purge-soft-deleted?${params.toString()}`,
    {
      method: 'DELETE',
    }
  );
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to purge soft-deleted books');
  }
  return response.json();
}

export async function restoreSoftDeletedBook(bookId: string): Promise<void> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/restore`, {
    method: 'POST',
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to restore audiobook');
  }
}

export async function deleteBook(
  bookId: string,
  options: { softDelete?: boolean; blockHash?: boolean } = {}
): Promise<DeleteBookResponse> {
  const params = new URLSearchParams();
  if (options.softDelete) params.set('soft_delete', 'true');
  if (options.blockHash) params.set('block_hash', 'true');
  const query = params.toString();
  const url =
    query.length > 0
      ? `${API_BASE}/audiobooks/${bookId}?${query}`
      : `${API_BASE}/audiobooks/${bookId}`;
  const response = await fetch(url, {
    method: 'DELETE',
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to delete audiobook');
  }
  return response.json();
}

export type OverridePayload = {
  value?: unknown;
  locked?: boolean;
  fetched_value?: unknown;
  clear?: boolean;
};

export async function updateBook(
  bookId: string,
  updates: Partial<Book> & {
    overrides?: Record<string, OverridePayload>;
    unlock_overrides?: string[];
    force_update?: boolean;
  }
): Promise<Book> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(updates),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to update audiobook');
  }
  return response.json();
}

export async function getBookTags(bookId: string): Promise<BookTags> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/tags`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch book tags');
  }
  return response.json();
}

export async function getBookSegments(bookId: string): Promise<BookSegment[]> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/segments`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch book segments');
  }
  return response.json();
}

export async function getSegmentTags(
  bookId: string,
  segmentId: string
): Promise<SegmentTags> {
  const response = await fetch(
    `${API_BASE}/audiobooks/${bookId}/segments/${segmentId}/tags`
  );
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch segment tags');
  }
  return response.json();
}

// Authors
export async function getAuthors(): Promise<Author[]> {
  const response = await fetch(`${API_BASE}/authors`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch authors');
  }
  const data = await response.json();
  return data.authors || [];
}

export async function countAuthors(): Promise<number> {
  const response = await fetch(`${API_BASE}/authors/count`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to count authors');
  }
  const data = await response.json();
  return data.count ?? 0;
}

// Series
export async function getSeries(): Promise<Series[]> {
  const response = await fetch(`${API_BASE}/series`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch series');
  }
  const data = await response.json();
  return data.series || [];
}

export async function countSeries(): Promise<number> {
  const response = await fetch(`${API_BASE}/series/count`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to count series');
  }
  const data = await response.json();
  return data.count ?? 0;
}

// Works
export async function getWorks(): Promise<Work[]> {
  const response = await fetch(`${API_BASE}/works`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch works');
  }
  const data = await response.json();
  return data.items || data.works || [];
}

// Import Paths
export async function getImportPaths(): Promise<ImportPath[]> {
  const response = await fetch(`${API_BASE}/import-paths`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch import paths');
  }
  const data = await response.json();
  return data.importPaths || [];
}

export async function addImportPath(
  path: string,
  name: string
): Promise<ImportPath> {
  const response = await fetch(`${API_BASE}/import-paths`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path, name }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to add import path');
  }
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
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to add import path');
  }
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
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to remove import path');
  }
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
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to start scan');
  }
  return response.json();
}

export async function getOperationStatus(id: string): Promise<Operation> {
  const response = await fetch(`${API_BASE}/operations/${id}/status`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch operation status');
  }
  return response.json();
}

export async function getOperationLogs(id: string): Promise<OperationLog[]> {
  const response = await fetch(`${API_BASE}/operations/${id}/logs`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch operation logs');
  }
  const data = await response.json();
  return data.items || data.logs || [];
}

export async function getOperationLogsTail(
  id: string,
  tail: number
): Promise<OperationLog[]> {
  const response = await fetch(
    `${API_BASE}/operations/${id}/logs?tail=${tail}`
  );
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch operation logs tail');
  }
  const data = await response.json();
  return data.items || data.logs || [];
}

export async function cancelOperation(id: string): Promise<void> {
  const response = await fetch(`${API_BASE}/operations/${id}`, {
    method: 'DELETE',
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to cancel operation');
  }
}

export async function clearStaleOperations(): Promise<{ cleared: number }> {
  const response = await fetch(`${API_BASE}/operations/clear-stale`, {
    method: 'POST',
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to clear stale operations');
  }
  return response.json();
}

export async function deleteOperationHistory(
  status: string
): Promise<{ deleted: number }> {
  const response = await fetch(
    `${API_BASE}/operations/history?status=${encodeURIComponent(status)}`,
    { method: 'DELETE' }
  );
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to delete operation history');
  }
  return response.json();
}

export async function getActiveOperations(): Promise<ActiveOperationSummary[]> {
  const response = await fetch(`${API_BASE}/operations/active`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch active operations');
  }
  const data = await response.json();
  return data.operations || [];
}

// System
export async function getSystemStatus(): Promise<SystemStatus> {
  const response = await fetch(`${API_BASE}/system/status`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch system status');
  }
  return response.json();
}

export async function getSystemStorage(): Promise<SystemStorage> {
  const response = await fetch(`${API_BASE}/system/storage`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch system storage');
  }
  return response.json();
}

export async function factoryReset(confirm: string): Promise<{ message: string }> {
  const response = await fetch(`${API_BASE}/system/factory-reset`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ confirm }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Factory reset failed');
  }
  return response.json();
}

// Organize operation
export async function startOrganize(
  folderPath?: string,
  priority?: number,
  bookIds?: string[]
): Promise<Operation> {
  const response = await fetch(`${API_BASE}/operations/organize`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      folder_path: folderPath,
      priority,
      book_ids: bookIds,
    }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to start organize');
  }
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
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch system logs');
  }
  return response.json();
}

// Config
export async function getConfig(): Promise<Config> {
  const response = await fetch(`${API_BASE}/config`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch config');
  }
  const data = await response.json();
  return data.config;
}

export async function updateConfig(updates: Partial<Config>): Promise<Config> {
  const response = await fetch(`${API_BASE}/config`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(updates),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to update config');
  }
  const data = await response.json();
  return data.config;
}

// Auth
export async function getAuthStatus(): Promise<AuthStatus> {
  const response = await fetch(`${API_BASE}/auth/status`, {
    credentials: 'include',
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch auth status');
  }
  return response.json();
}

export async function setupAdmin(payload: {
  username: string;
  password: string;
  email?: string;
}): Promise<{ message: string; user: AuthUser }> {
  const response = await fetch(`${API_BASE}/auth/setup`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to create admin account');
  }
  return response.json();
}

export async function login(payload: {
  username: string;
  password: string;
}): Promise<{ user: AuthUser; session: AuthSession }> {
  const response = await fetch(`${API_BASE}/auth/login`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to login');
  }
  return response.json();
}

export async function getMe(): Promise<AuthUser> {
  const response = await fetch(`${API_BASE}/auth/me`, {
    credentials: 'include',
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch current user');
  }
  const data = await response.json();
  return data.user;
}

export async function logout(): Promise<void> {
  const response = await fetch(`${API_BASE}/auth/logout`, {
    method: 'POST',
    credentials: 'include',
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to logout');
  }
}

export async function listSessions(): Promise<AuthSession[]> {
  const response = await fetch(`${API_BASE}/auth/sessions`, {
    credentials: 'include',
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to list sessions');
  }
  const data = await response.json();
  return data.sessions || [];
}

export async function revokeSession(sessionId: string): Promise<void> {
  const response = await fetch(`${API_BASE}/auth/sessions/${sessionId}`, {
    method: 'DELETE',
    credentials: 'include',
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to revoke session');
  }
}

// Version Management
export async function getBookVersions(bookId: string): Promise<Book[]> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/versions`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch book versions');
  }
  const data = await response.json();
  return data.versions || [];
}

export async function linkBookVersion(
  bookId: string,
  otherBookId: string
): Promise<void> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/versions`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ other_id: otherBookId }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to link book version');
  }
}

export async function unlinkBookVersion(
  bookId: string,
  otherBookId: string
): Promise<void> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/versions`, {
    method: 'DELETE',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ other_id: otherBookId }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to unlink book version');
  }
}

export async function setPrimaryVersion(bookId: string): Promise<void> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/set-primary`, {
    method: 'PUT',
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to set primary version');
  }
}

export async function getVersionGroup(groupId: string): Promise<Book[]> {
  const response = await fetch(`${API_BASE}/version-groups/${groupId}`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch version group');
  }
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
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to import file');
  }
  return response.json();
}

export async function validateITunesLibrary(
  payload: ITunesValidateRequest
): Promise<ITunesValidateResponse> {
  const response = await fetch(`${API_BASE}/itunes/validate`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to validate iTunes library');
  }
  return response.json();
}

export interface ITunesTestMappingResponse {
  tested: number;
  found: number;
  examples: { title: string; path: string }[];
}

export async function testITunesPathMapping(
  libraryPath: string,
  from: string,
  to: string
): Promise<ITunesTestMappingResponse> {
  const response = await fetch(`${API_BASE}/itunes/test-mapping`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ library_path: libraryPath, from, to }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to test path mapping');
  }
  return response.json();
}

export async function importITunesLibrary(
  payload: ITunesImportRequest
): Promise<ITunesImportResponse> {
  const response = await fetch(`${API_BASE}/itunes/import`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to import iTunes library');
  }
  return response.json();
}

export async function writeBackITunesLibrary(
  payload: ITunesWriteBackRequest
): Promise<ITunesWriteBackResponse> {
  const response = await fetch(`${API_BASE}/itunes/write-back`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  if (response.status === 409) {
    const data = await response.json();
    const err = new Error(data.message || 'Library modified');
    (err as unknown as Record<string, unknown>).type = 'library_modified';
    (err as unknown as Record<string, unknown>).details = data;
    throw err;
  }
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to write back iTunes library');
  }
  return response.json();
}

export interface ITunesBookMapping {
  book_id: string;
  title: string;
  author: string;
  itunes_persistent_id: string;
  local_path: string;
  itunes_path: string;
  path_differs: boolean;
}

export async function getITunesBooks(
  search?: string,
  limit?: number,
  offset?: number
): Promise<{ items: ITunesBookMapping[]; count: number }> {
  const params = new URLSearchParams();
  if (search) params.set('search', search);
  if (limit != null) params.set('limit', String(limit));
  if (offset != null) params.set('offset', String(offset));
  const response = await fetch(`${API_BASE}/itunes/books?${params.toString()}`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch iTunes books');
  }
  return response.json();
}

export async function previewITunesWriteBack(
  libraryPath: string,
  bookIds?: string[]
): Promise<{ items: ITunesBookMapping[]; total: number }> {
  const response = await fetch(`${API_BASE}/itunes/write-back/preview`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ library_path: libraryPath, book_ids: bookIds }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to preview write-back');
  }
  return response.json();
}

export async function startITunesSync(
  libraryPath?: string,
  force?: boolean
): Promise<{ operation_id: string; message: string }> {
  const response = await fetch(`${API_BASE}/itunes/sync`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ library_path: libraryPath, force: force ?? true }),
  });
  if (!response.ok) throw await buildApiError(response, 'Sync failed');
  return response.json();
}

export async function getITunesLibraryStatus(
  path: string
): Promise<ITunesLibraryStatus> {
  const response = await fetch(
    `${API_BASE}/itunes/library-status?path=${encodeURIComponent(path)}`
  );
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch library status');
  }
  return response.json();
}

export async function getITunesImportStatus(
  operationId: string
): Promise<ITunesImportStatus> {
  const response = await fetch(
    `${API_BASE}/itunes/import-status/${operationId}`
  );
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch iTunes import status');
  }
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

  const response = await fetch(
    `${API_BASE}/metadata/search?${params.toString()}`
  );
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to search metadata');
  }
  return response.json();
}

export async function fetchBookMetadata(
  bookId: string
): Promise<{ message: string; book: Book; source: string }> {
  const response = await fetch(
    `${API_BASE}/audiobooks/${bookId}/fetch-metadata`,
    {
      method: 'POST',
    }
  );
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch metadata');
  }
  return response.json();
}

export interface WriteBackMetadataResponse {
  message: string;
  written_count: number;
}

export async function writeBackMetadata(
  bookId: string
): Promise<WriteBackMetadataResponse> {
  const response = await fetch(
    `${API_BASE}/audiobooks/${bookId}/write-back`,
    {
      method: 'POST',
    }
  );
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to write metadata to files');
  }
  return response.json();
}

// Bulk Metadata Fetching
export interface BulkFetchMetadataResult {
  book_id: string;
  status: string;
  message?: string;
  applied_fields?: string[];
  fetched_fields?: string[];
}

export interface BulkFetchMetadataResponse {
  updated_count: number;
  total_count: number;
  results: BulkFetchMetadataResult[];
  source: string;
}

export async function bulkFetchMetadata(
  bookIds: string[],
  onlyMissing = true
): Promise<BulkFetchMetadataResponse> {
  const response = await fetch(`${API_BASE}/metadata/bulk-fetch`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ book_ids: bookIds, only_missing: onlyMissing }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to bulk fetch metadata');
  }
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

export async function parseFilenameWithAI(
  filename: string
): Promise<{ metadata: AIParseResult }> {
  const response = await fetch(`${API_BASE}/ai/parse-filename`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ filename }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to parse filename with AI');
  }
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
  const response = await fetch(
    `${API_BASE}/audiobooks/${bookId}/parse-with-ai`,
    {
      method: 'POST',
    }
  );
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to parse audiobook with AI');
  }
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
    total_bytes?: number;
    free_bytes?: number;
    library_bytes?: number;
  };
}

export async function browseFilesystem(
  path: string
): Promise<FilesystemBrowseResult> {
  const response = await fetch(
    `${API_BASE}/filesystem/browse?path=${encodeURIComponent(path)}`
  );
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to browse filesystem');
  }
  return response.json();
}

/** Fetches the server user's home directory path. */
export async function getHomeDirectory(): Promise<string> {
  const response = await fetch(`${API_BASE}/filesystem/home`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch home directory');
  }
  const data = await response.json();
  return data.path as string;
}

export async function excludeFilesystemPath(
  path: string,
  reason?: string
): Promise<{ excluded: boolean; path: string; reason?: string }> {
  const response = await fetch(`${API_BASE}/filesystem/exclude`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path, reason }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to exclude path');
  }
  return response.json();
}

export async function includeFilesystemPath(path: string): Promise<void> {
  const response = await fetch(`${API_BASE}/filesystem/exclude`, {
    method: 'DELETE',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to remove exclusion');
  }
}

export interface BackupInfo {
  filename: string;
  path?: string;
  size: number;
  checksum?: string;
  database_type?: string;
  created_at: string;
  auto?: boolean;
  trigger?: string;
  status?: string;
}

export interface BackupListResponse {
  backups: BackupInfo[];
  count: number;
}

export async function createBackup(maxBackups?: number): Promise<BackupInfo> {
  const response = await fetch(`${API_BASE}/backup/create`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(
      typeof maxBackups === 'number' ? { max_backups: maxBackups } : {}
    ),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to create backup');
  }
  return response.json();
}

export async function listBackups(): Promise<BackupListResponse> {
  const response = await fetch(`${API_BASE}/backup/list`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to list backups');
  }
  return response.json();
}

export async function restoreBackup(
  filename: string,
  verify = true
): Promise<{ message: string }> {
  const response = await fetch(`${API_BASE}/backup/restore`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ backup_filename: filename, verify }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to restore backup');
  }
  return response.json();
}

export async function deleteBackup(filename: string): Promise<void> {
  const response = await fetch(`${API_BASE}/backup/${filename}`, {
    method: 'DELETE',
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to delete backup');
  }
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
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch blocked hashes');
  }
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
    throw await buildApiError(response, 'Failed to add blocked hash');
  }
  return response.json();
}

export async function removeBlockedHash(
  hash: string
): Promise<{ message: string; hash: string }> {
  const response = await fetch(`${API_BASE}/blocked-hashes/${hash}`, {
    method: 'DELETE',
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to remove blocked hash');
  }
  return response.json();
}

// Metadata History
export interface MetadataChangeRecord {
  id: number;
  book_id: string;
  field: string;
  previous_value?: string;
  new_value?: string;
  change_type: string;
  source?: string;
  changed_at: string;
}

export async function getBookMetadataHistory(bookId: string): Promise<MetadataChangeRecord[]> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/metadata-history`);
  if (!response.ok) throw await buildApiError(response, 'Failed to fetch metadata history');
  const data = await response.json();
  return data.history || [];
}

export async function getFieldMetadataHistory(bookId: string, field: string): Promise<MetadataChangeRecord[]> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/metadata-history/${field}`);
  if (!response.ok) throw await buildApiError(response, 'Failed to fetch field history');
  const data = await response.json();
  return data.history || [];
}

export async function undoMetadataChange(bookId: string, field: string): Promise<{ message: string }> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/metadata-history/${field}/undo`, { method: 'POST' });
  if (!response.ok) throw await buildApiError(response, 'Failed to undo change');
  return response.json();
}

// Metadata Field States (provenance)
export interface MetadataFieldStateEntry {
  fetched_value?: unknown;
  override_value?: unknown;
  override_locked: boolean;
  updated_at: string;
}

export type MetadataFieldStates = Record<string, MetadataFieldStateEntry>;

export async function getAudiobookFieldStates(bookId: string): Promise<MetadataFieldStates> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/field-states`);
  if (!response.ok) throw await buildApiError(response, 'Failed to fetch field states');
  const data = await response.json();
  return data.field_states || {};
}

// Version
export async function getAppVersion(): Promise<string> {
  try {
    const response = await fetch(`${API_BASE}/health`);
    if (response.ok) {
      const data = await response.json();
      return data.version || 'unknown';
    }
  } catch {
    // ignore
  }
  return 'unknown';
}

// Open Library Data Dumps
export interface OLDumpTypeStatus {
  filename?: string;
  date?: string;
  download_progress: number;
  import_progress: number;
  record_count: number;
  last_updated?: string;
}

export interface OLDownloadProgress {
  dump_type: string;
  status: 'idle' | 'downloading' | 'complete' | 'error';
  downloaded: number;
  total_size: number;
  error?: string;
  source?: string;
}

export interface OLUploadedFile {
  filename: string;
  size: number;
  mod_time: string;
}

export interface OLDumpStatus {
  enabled: boolean;
  status?: {
    editions: OLDumpTypeStatus;
    authors: OLDumpTypeStatus;
    works: OLDumpTypeStatus;
  };
  downloads?: Record<string, OLDownloadProgress>;
  uploaded_files?: Record<string, OLUploadedFile>;
}

export async function getOLDumpStatus(): Promise<OLDumpStatus> {
  const response = await fetch(`${API_BASE}/openlibrary/status`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to get OL dump status');
  }
  return response.json();
}

export async function startOLDumpDownload(
  types?: string[]
): Promise<{ message: string; types: string[] }> {
  const response = await fetch(`${API_BASE}/openlibrary/download`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ types: types || ['editions', 'authors', 'works'] }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to start OL dump download');
  }
  return response.json();
}

export async function startOLDumpImport(
  types?: string[]
): Promise<{ message: string; types: string[]; operation_id?: string }> {
  const response = await fetch(`${API_BASE}/openlibrary/import`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ types: types || ['editions', 'authors', 'works'] }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to start OL dump import');
  }
  return response.json();
}

export async function uploadOLDump(
  dumpType: string,
  file: File,
  onProgress?: (percent: number) => void
): Promise<{ message: string; type: string; size: number }> {
  const formData = new FormData();
  formData.append('type', dumpType);
  formData.append('file', file);

  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open('POST', `${API_BASE}/openlibrary/upload`);

    if (onProgress) {
      xhr.upload.addEventListener('progress', (e) => {
        if (e.lengthComputable) {
          onProgress(Math.round((e.loaded / e.total) * 100));
        }
      });
    }

    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        resolve(JSON.parse(xhr.responseText));
      } else {
        let msg = 'Failed to upload dump file';
        try {
          const body = JSON.parse(xhr.responseText);
          if (body.error) msg = body.error;
        } catch { /* ignore */ }
        reject(new Error(msg));
      }
    };

    xhr.onerror = () => reject(new Error('Upload network error'));
    xhr.send(formData);
  });
}

export async function deleteOLDumpData(): Promise<{ message: string }> {
  const response = await fetch(`${API_BASE}/openlibrary/data`, {
    method: 'DELETE',
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to delete OL dump data');
  }
  return response.json();
}

// Update management

export interface UpdateInfo {
  current_version: string;
  latest_version: string;
  channel: string;
  update_available: boolean;
  release_url?: string;
  release_notes?: string;
  published_at?: string;
  last_checked: string;
}

export async function getUpdateStatus(): Promise<UpdateInfo> {
  const response = await fetch(`${API_BASE}/update/status`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to get update status');
  }
  return response.json();
}

export async function checkForUpdate(): Promise<UpdateInfo> {
  const response = await fetch(`${API_BASE}/update/check`, { method: 'POST' });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to check for updates');
  }
  return response.json();
}

export async function applyUpdate(): Promise<void> {
  const response = await fetch(`${API_BASE}/update/apply`, { method: 'POST' });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to apply update');
  }
}
