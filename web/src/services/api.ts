// file: web/src/services/api.ts
// version: 1.69.0
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
  asin?: string;
  open_library_id?: string;
  hardcover_id?: string;
  google_books_id?: string;
  work_id?: string;
  edition?: string;
  genre?: string;
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
  file_size?: number;
  original_file_hash?: string;
  organized_file_hash?: string;
  itunes_persistent_id?: string;
  itunes_date_added?: string;
  itunes_play_count?: number;
  itunes_last_played?: string;
  itunes_rating?: number;
  itunes_bookmark?: number;
  itunes_import_source?: string;
  itunes_path?: string;
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
  metadata_review_status?: string;
  last_written_at?: string;
  file_exists?: boolean;
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

export interface AuthorWithCount {
  id: number;
  name: string;
  book_count: number;
  file_count: number;
  aliases: AuthorAlias[];
}

export interface SeriesWithCount {
  id: number;
  name: string;
  author_id?: number;
  created_at: string;
  book_count: number;
  file_count: number;
  author_name?: string;
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

/** @deprecated Use BookFile instead. Kept for backward compatibility with legacy endpoints. */
export interface BookSegment {
  id: string;
  file_path: string;
  format: string;
  size_bytes: number;
  duration_seconds: number;
  track_number?: number;
  total_tracks?: number;
  active: boolean;
  file_exists?: boolean;
}

export interface BookFile {
  id: string;
  book_id: string;
  file_path: string;
  original_filename?: string;
  itunes_path?: string;
  itunes_persistent_id?: string;
  track_number?: number;
  track_count?: number;
  disc_number?: number;
  disc_count?: number;
  title?: string;
  format?: string;
  codec?: string;
  duration?: number;
  file_size?: number;
  bitrate_kbps?: number;
  sample_rate_hz?: number;
  channels?: number;
  bit_depth?: number;
  file_hash?: string;
  missing: boolean;
  file_exists?: boolean;
  created_at: string;
  updated_at: string;
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
  audiobook_count: number;
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
  result_data?: string;
}

export interface SuggestionRole {
  name?: string;
  ids?: number[];
  variants?: string[];
  reason?: string;
}

export interface SuggestionRoles {
  author?: SuggestionRole;
  narrator?: SuggestionRole;
  publisher?: SuggestionRole;
}

export interface AIAuthorSuggestion {
  group_index: number;
  action: 'merge' | 'split' | 'rename' | 'skip' | 'alias' | 'reclassify';
  canonical_name: string;
  reason: string;
  confidence: 'high' | 'medium' | 'low';
  is_narrator?: number[];
  is_publisher?: number[];
  roles?: SuggestionRoles;
}

export interface ApplyAISuggestion {
  group_index: number;
  action: string;
  canonical_name: string;
  keep_id: number;
  merge_ids: number[];
  rename: boolean;
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
  total_file_count?: number;
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
  metadata_llm_scoring_enabled?: boolean;
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

  // Maintenance window
  maintenance_window_enabled?: boolean;
  maintenance_window_start?: number;
  maintenance_window_end?: number;

  // Smart apply pipeline
  path_format?: string;
  segment_title_format?: string;
  auto_rename_on_apply?: boolean;
  auto_write_tags_on_apply?: boolean;
  verify_after_write?: boolean;

  // iTunes sync
  itunes_library_read_path?: string;
  itunes_library_write_path?: string;
  itl_write_back_enabled?: boolean;
  itunes_auto_write_back?: boolean;
  itunes_sync_enabled?: boolean;

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
export async function getBooks(
  limit = 100,
  offset = 0,
  options?: {
    sortBy?: string;
    sortOrder?: string;
    tag?: string;
    libraryState?: string;
    filters?: string;
  }
): Promise<Book[]> {
  const params = new URLSearchParams();
  params.set('limit', String(limit));
  params.set('offset', String(offset));
  if (options?.sortBy) params.set('sort_by', options.sortBy);
  if (options?.sortOrder) params.set('sort_order', options.sortOrder);
  if (options?.tag) params.set('tag', options.tag);
  if (options?.libraryState)
    params.set('library_state', options.libraryState);
  if (options?.filters) params.set('filters', options.filters);
  params.set('is_primary_version', 'true');

  const response = await fetch(`${API_BASE}/audiobooks?${params}`);
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
    `${API_BASE}/audiobooks?search=${encodeURIComponent(query)}&limit=${limit}&is_primary_version=true`
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

export async function countBooksFiltered(options: {
  libraryState?: string;
}): Promise<number> {
  const params = new URLSearchParams({ limit: '1', offset: '0' });
  if (options.libraryState) params.set('library_state', options.libraryState);
  const response = await fetch(`${API_BASE}/audiobooks?${params}`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to count filtered books');
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

export async function getBookTags(
  bookId: string,
  compareId?: string,
  snapshotTimestamp?: string
): Promise<BookTags> {
  const params = new URLSearchParams();
  if (compareId) params.set('compare_id', compareId);
  if (snapshotTimestamp) params.set('snapshot_ts', snapshotTimestamp);
  const query = params.toString();
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/tags${query ? `?${query}` : ''}`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch book tags');
  }
  return response.json();
}

export interface ChangeLogEntry {
  timestamp: string;
  type: 'tag_write' | 'rename' | 'metadata_apply' | 'import' | 'transcode';
  summary: string;
  details?: Record<string, unknown>;
}

export async function getBookChangelog(bookId: string): Promise<{ entries: ChangeLogEntry[] }> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/changelog`);
  if (!response.ok) return { entries: [] };
  return response.json();
}

/** @deprecated Use getBookFiles instead. This calls the legacy segments endpoint. */
export async function getBookSegments(bookId: string): Promise<BookSegment[]> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/segments`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch book segments');
  }
  return response.json();
}

export async function getBookFiles(
  bookId: string
): Promise<{ files: BookFile[]; count: number }> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/files`);
  if (!response.ok)
    throw new Error(`Failed to fetch book files: ${response.status}`);
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
  return data.items || data.authors || [];
}

export async function countAuthors(): Promise<number> {
  const response = await fetch(`${API_BASE}/authors/count`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to count authors');
  }
  const data = await response.json();
  return data.count ?? 0;
}

export interface Announcement {
  id: string;
  severity: 'info' | 'warning' | 'error';
  message: string;
  link?: string;
}

export async function getAnnouncements(): Promise<Announcement[]> {
  const response = await fetch(`${API_BASE}/system/announcements`);
  if (!response.ok) return [];
  const data = await response.json();
  return data.announcements || [];
}

export interface AuthorDedupGroup {
  canonical: Author;
  variants: Author[];
  book_count: number;
  suggested_name?: string;
  split_names?: string[];
  is_production_company?: boolean;
}

export interface MergeAuthorsResult {
  merged: number;
  errors: string[];
}

export async function getAuthorDuplicates(): Promise<{ groups: AuthorDedupGroup[]; needs_refresh?: boolean }> {
  const response = await fetch(`${API_BASE}/authors/duplicates`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch author duplicates');
  }
  const data = await response.json();
  return { groups: data.groups || [], needs_refresh: data.needs_refresh };
}

export async function refreshAuthorDuplicates(): Promise<Operation> {
  const response = await fetch(`${API_BASE}/authors/duplicates/refresh`, { method: 'POST' });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to start author dedup scan');
  }
  return response.json();
}

export async function mergeAuthors(keepId: number, mergeIds: number[]): Promise<Operation> {
  const response = await fetch(`${API_BASE}/authors/merge`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ keep_id: keepId, merge_ids: mergeIds }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to merge authors');
  }
  return response.json();
}

export async function getBooksByAuthor(authorId: number): Promise<Book[]> {
  const response = await fetch(`${API_BASE}/audiobooks?author_id=${authorId}`);
  if (!response.ok) return [];
  const data = await response.json();
  return data.items || [];
}

export interface AuthorAlias {
  id: number;
  author_id: number;
  alias_name: string;
  alias_type: string;
  created_at: string;
}

export async function getAuthorAliases(authorId: number): Promise<AuthorAlias[]> {
  const response = await fetch(`${API_BASE}/authors/${authorId}/aliases`);
  if (!response.ok) return [];
  const data = await response.json();
  return data.aliases || [];
}

export async function createAuthorAlias(authorId: number, aliasName: string, aliasType: string = 'alias'): Promise<AuthorAlias> {
  const response = await fetch(`${API_BASE}/authors/${authorId}/aliases`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ alias_name: aliasName, alias_type: aliasType }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to create author alias');
  }
  return response.json();
}

export async function deleteAuthorAlias(authorId: number, aliasId: number): Promise<void> {
  const response = await fetch(`${API_BASE}/authors/${authorId}/aliases/${aliasId}`, {
    method: 'DELETE',
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to delete author alias');
  }
}

export async function resolveProductionAuthor(authorId: number): Promise<Operation> {
  const response = await fetch(`${API_BASE}/authors/${authorId}/resolve-production`, {
    method: 'POST',
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to resolve production author');
  }
  const data = await response.json();
  return data.operation;
}

// Book dedup — uses existing /audiobooks/duplicates endpoint which returns Book[][] groups
export interface DuplicatesResponse {
  groups: Book[][];
  group_count: number;
  duplicate_count: number;
}

export interface MergeBooksResult {
  merged: number;
  errors: string[];
}

export async function getBookDuplicates(): Promise<DuplicatesResponse> {
  const response = await fetch(`${API_BASE}/audiobooks/duplicates`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch book duplicates');
  }
  return response.json();
}

export async function mergeBooks(keepId: string, mergeIds: string[]): Promise<Operation> {
  const response = await fetch(`${API_BASE}/audiobooks/merge`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ keep_id: keepId, merge_ids: mergeIds }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to merge books');
  }
  return response.json();
}

// Book dedup scan — advanced duplicate detection with confidence levels
export interface BookDedupGroup {
  books: Book[];
  confidence: 'high' | 'medium' | 'low';
  reason: string;
  group_key: string;
}

export interface BookDedupScanResponse {
  groups: BookDedupGroup[];
  group_count: number;
  duplicate_count: number;
  needs_refresh?: boolean;
}

export async function getBookDedupScanResults(): Promise<BookDedupScanResponse> {
  const response = await fetch(`${API_BASE}/audiobooks/duplicates/scan-results`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch book dedup scan results');
  }
  return response.json();
}

export async function scanBookDuplicates(): Promise<Operation> {
  const response = await fetch(`${API_BASE}/audiobooks/duplicates/scan`, {
    method: 'POST',
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to start book dedup scan');
  }
  return response.json();
}

export async function mergeBookDuplicatesAsVersions(bookIds: string[]): Promise<{ message: string; version_group_id: string; primary_id: string }> {
  const response = await fetch(`${API_BASE}/audiobooks/duplicates/merge`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ book_ids: bookIds }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to merge book duplicates as versions');
  }
  return response.json();
}

export async function dismissBookDuplicateGroup(groupKey: string): Promise<{ message: string }> {
  const response = await fetch(`${API_BASE}/audiobooks/duplicates/dismiss`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ group_key: groupKey }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to dismiss duplicate group');
  }
  return response.json();
}

// Series
export async function getSeries(): Promise<SeriesWithCount[]> {
  const response = await fetch(`${API_BASE}/series`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch series');
  }
  const data = await response.json();
  return data.items || data.series || [];
}

export async function countSeries(): Promise<number> {
  const response = await fetch(`${API_BASE}/series/count`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to count series');
  }
  const data = await response.json();
  return data.count ?? 0;
}

export async function getSeriesBooks(seriesId: number): Promise<Book[]> {
  const response = await fetch(`${API_BASE}/series/${seriesId}/books`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch series books');
  }
  const data = await response.json();
  return data.items || data.books || [];
}

export async function renameSeries(seriesId: number, name: string): Promise<void> {
  const response = await fetch(`${API_BASE}/series/${seriesId}/name`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to rename series');
  }
}

export async function splitSeries(seriesId: number, bookIds: string[]): Promise<{ new_series_id: number; books_moved: number }> {
  const response = await fetch(`${API_BASE}/series/${seriesId}/split`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ book_ids: bookIds }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to split series');
  }
  return response.json();
}

export async function deleteSeries(seriesId: number): Promise<void> {
  const response = await fetch(`${API_BASE}/series/${seriesId}`, {
    method: 'DELETE',
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to delete series');
  }
}

export async function getAuthorsWithCounts(): Promise<AuthorWithCount[]> {
  const response = await fetch(`${API_BASE}/authors`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch authors');
  }
  const data = await response.json();
  return data.items || data.authors || [];
}

export async function getAuthorBooks(authorId: number): Promise<Book[]> {
  const response = await fetch(`${API_BASE}/authors/${authorId}/books`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch author books');
  }
  const data = await response.json();
  return data.items || data.books || [];
}

export async function deleteAuthor(authorId: number): Promise<void> {
  const response = await fetch(`${API_BASE}/authors/${authorId}`, {
    method: 'DELETE',
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to delete author');
  }
}

export async function bulkDeleteAuthors(ids: number[]): Promise<{ deleted: number; skipped: number; errors: string[]; total: number }> {
  const response = await fetch(`${API_BASE}/authors/bulk-delete`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ ids }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to bulk delete authors');
  }
  return response.json();
}

export async function bulkDeleteSeries(ids: number[]): Promise<{ deleted: number; skipped: number; errors: string[]; total: number }> {
  const response = await fetch(`${API_BASE}/series/bulk-delete`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ ids }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to bulk delete series');
  }
  return response.json();
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

export async function startTranscode(
  bookId: string,
  opts?: { output_format?: string; bitrate?: number; keep_original?: boolean }
): Promise<Operation> {
  const response = await fetch(`${API_BASE}/operations/transcode`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      book_id: bookId,
      output_format: opts?.output_format,
      bitrate: opts?.bitrate,
      keep_original: opts?.keep_original,
    }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to start transcode');
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

// Poll an operation until it completes or fails. Calls onProgress with each update.
export async function pollOperation(
  id: string,
  onProgress?: (op: Operation) => void,
  intervalMs = 1000
): Promise<Operation> {
  while (true) {
    const op = await getOperationStatus(id);
    onProgress?.(op);
    if (op.status === 'completed' || op.status === 'failed' || op.status === 'cancelled') {
      return op;
    }
    await new Promise((r) => setTimeout(r, intervalMs));
  }
}

export interface OptimizeDatabaseResult {
  books_processed: number;
  authors_split: number;
  narrators_split: number;
}

export async function optimizeDatabase(): Promise<OptimizeDatabaseResult> {
  const response = await fetch(`${API_BASE}/operations/optimize-database`, {
    method: 'POST',
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to optimize database');
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

export async function listOperations(limit = 50, offset = 0): Promise<{ items: Operation[]; total: number; limit: number; offset: number }> {
  const response = await fetch(`${API_BASE}/operations?limit=${limit}&offset=${offset}`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch operations');
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

export interface OperationChange {
  id: string;
  operation_id: string;
  book_id: string;
  change_type: string;
  field_name: string;
  old_value: string;
  new_value: string;
  reverted_at: string | null;
  created_at: string;
}

export async function getOperationChanges(operationId: string): Promise<OperationChange[]> {
  const response = await fetch(`${API_BASE}/operations/${operationId}/changes`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch operation changes');
  }
  const data = await response.json();
  return data.changes || [];
}

export async function revertOperation(operationId: string): Promise<void> {
  const response = await fetch(`${API_BASE}/operations/${operationId}/revert`, {
    method: 'POST',
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to revert operation');
  }
}

export async function getBookChanges(bookId: string): Promise<OperationChange[]> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/changes`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch book changes');
  }
  const data = await response.json();
  return data.changes || [];
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
  bookIds?: string[],
  options?: { fetchMetadataFirst?: boolean; syncITunesFirst?: boolean }
): Promise<Operation> {
  const response = await fetch(`${API_BASE}/operations/organize`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      folder_path: folderPath,
      priority,
      book_ids: bookIds,
      fetch_metadata_first: options?.fetchMetadataFirst,
      sync_itunes_first: options?.syncITunesFirst,
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

// Split selected segments into a new version (new book in same version group)
export async function splitVersion(bookId: string, segmentIds: string[]): Promise<Book> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/split-version`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ segment_ids: segmentIds }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to split version');
  }
  return response.json();
}

// Split selected segments into independent new books (one per segment).
// Unlike splitVersion, new books are NOT version-linked to the source.
export async function splitSegmentsToBooks(bookId: string, segmentIds: string[]): Promise<{ created_books: Book[]; count: number }> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/split-to-books`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ segment_ids: segmentIds }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to split segments to books');
  }
  return response.json();
}

// Move segments from one book to another (must be in same version group)
export async function moveSegments(bookId: string, segmentIds: string[], targetBookId: string): Promise<void> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/move-segments`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ segment_ids: segmentIds, target_book_id: targetBookId }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to move segments');
  }
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
    throw new ApiError(data.message || 'Library modified', 409, data);
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

export async function getITunesImportStatusBulk(
  operationIds: string[]
): Promise<Record<string, ITunesImportStatus>> {
  const response = await fetch(`${API_BASE}/itunes/import-status/bulk`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ ids: operationIds }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch bulk import status');
  }
  const data = await response.json();
  return data.statuses || {};
}

// Series dedup
export interface SeriesBookSummary {
  id: string;
  title: string;
  cover_url?: string;
}

export interface SeriesWithBooks extends Series {
  books?: SeriesBookSummary[];
  author_name?: string;
}

export interface SeriesDupGroup {
  name: string;
  count: number;
  series: SeriesWithBooks[];
  suggested_name?: string;
  match_type?: string;
}

export async function getSeriesDuplicates(): Promise<{ groups: SeriesDupGroup[]; count: number; total_series: number; needs_refresh?: boolean }> {
  const response = await fetch(`${API_BASE}/series/duplicates`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch series duplicates');
  }
  return response.json();
}

export async function refreshSeriesDuplicates(): Promise<Operation> {
  const response = await fetch(`${API_BASE}/series/duplicates/refresh`, { method: 'POST' });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to start series dedup scan');
  }
  return response.json();
}

// Dedup validation
export interface ValidationResult {
  source: string;
  title: string;
  author: string;
  series?: string;
  series_position?: string;
  cover_url?: string;
  isbn?: string;
}

export async function validateDedupEntry(query: string, type: 'series' | 'author' | 'book' = 'series'): Promise<{ results: ValidationResult[]; query: string; type: string }> {
  const response = await fetch(`${API_BASE}/dedup/validate`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ query, type }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to validate dedup entry');
  }
  return response.json();
}

export interface SeriesDedupResult {
  merged: number;
  remaining_series: number;
  errors: string[];
}

export async function deduplicateSeries(): Promise<Operation> {
  const response = await fetch(`${API_BASE}/series/deduplicate`, {
    method: 'POST',
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to deduplicate series');
  }
  return response.json();
}

export async function mergeSeriesGroup(keepId: number, mergeIds: number[], customName?: string): Promise<Operation> {
  const body: Record<string, unknown> = { keep_id: keepId, merge_ids: mergeIds };
  if (customName) body.custom_name = customName;
  const response = await fetch(`${API_BASE}/series/merge`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to merge series');
  }
  return response.json();
}

export interface SeriesPrunePreviewGroup {
  name: string;
  canonical_id: number;
  merge_ids: number[] | null;
  book_count: number;
  type: 'duplicate' | 'orphan';
}

export interface SeriesPrunePreview {
  groups: SeriesPrunePreviewGroup[];
  duplicate_count: number;
  orphan_count: number;
  total_count: number;
}

export async function seriesPrunePreview(): Promise<SeriesPrunePreview> {
  const response = await fetch(`${API_BASE}/series/prune/preview`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to get series prune preview');
  }
  return response.json();
}

export async function seriesPrune(): Promise<Operation> {
  const response = await fetch(`${API_BASE}/series/prune`, {
    method: 'POST',
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to prune series');
  }
  return response.json();
}

export async function updateSeriesName(id: number, name: string): Promise<Series> {
  const response = await fetch(`${API_BASE}/series/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to update series name');
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

export interface MetadataCandidate {
  title: string;
  author: string;
  narrator?: string;
  series?: string;
  series_position?: string;
  year?: number;
  publisher?: string;
  isbn?: string;
  asin?: string;
  cover_url?: string;
  description?: string;
  language?: string;
  source: string;
  score: number;
}

export interface SearchMetadataResponse {
  results: MetadataCandidate[];
  query: string;
  sources_tried?: string[];
  sources_failed?: Record<string, string>;
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

export async function searchMetadataForBook(
  bookId: string,
  query?: string,
  author?: string,
  narrator?: string,
  series?: string,
  useRerank?: boolean
): Promise<SearchMetadataResponse> {
  const body: {
    query: string;
    author?: string;
    narrator?: string;
    series?: string;
    use_rerank?: boolean;
  } = { query: query || '' };
  if (author) body.author = author;
  if (narrator) body.narrator = narrator;
  if (series) body.series = series;
  if (useRerank) body.use_rerank = true;
  const response = await fetch(
    `${API_BASE}/audiobooks/${bookId}/search-metadata`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    }
  );
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to search metadata');
  }
  return response.json();
}

export async function applyMetadataCandidate(
  bookId: string,
  candidate: MetadataCandidate,
  fields?: string[],
  writeBack?: boolean
): Promise<{ message: string; book: Book; source: string }> {
  const payload: { candidate: MetadataCandidate; fields: string[]; write_back?: boolean } = {
    candidate,
    fields: fields || [],
  };
  if (writeBack !== undefined) {
    payload.write_back = writeBack;
  }
  const response = await fetch(
    `${API_BASE}/audiobooks/${bookId}/apply-metadata`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    }
  );
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to apply metadata');
  }
  return response.json();
}

export async function markNoMatch(bookId: string): Promise<void> {
  const response = await fetch(
    `${API_BASE}/audiobooks/${bookId}/mark-no-match`,
    {
      method: 'POST',
    }
  );
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to mark as no match');
  }
}

export interface WriteBackMetadataResponse {
  message: string;
  written_count: number;
}

export interface BatchWriteBackError {
  book_id: string;
  error: string;
}

export interface BatchWriteBackResponse {
  written: number;
  written_files: number;
  renamed: number;
  organized: number;
  failed: number;
  errors: BatchWriteBackError[];
}

export async function writeBackMetadata(
  bookId: string,
  segmentIds?: string[]
): Promise<WriteBackMetadataResponse> {
  const options: RequestInit = { method: 'POST' };
  if (segmentIds && segmentIds.length > 0) {
    options.headers = { 'Content-Type': 'application/json' };
    options.body = JSON.stringify({ segment_ids: segmentIds });
  }
  const response = await fetch(
    `${API_BASE}/audiobooks/${bookId}/write-back`,
    options
  );
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to write metadata to files');
  }
  return response.json();
}

export async function batchWriteBackMetadata(
  bookIds: string[],
  organize = false,
  force = false
): Promise<BatchWriteBackResponse> {
  const response = await fetch(`${API_BASE}/audiobooks/batch-write-back`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ book_ids: bookIds, organize, force }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to write metadata to files');
  }
  return response.json();
}

// Bulk write-back (async operation for all/filtered books)
export interface BulkWriteBackFilter {
  library_state?: string;
  author_id?: string;
  series_id?: string;
}

export interface BulkWriteBackRequest {
  filter?: BulkWriteBackFilter;
  dry_run?: boolean;
  rename?: boolean;
}

export interface BulkWriteBackResponse {
  operation_id?: string;
  estimated_books: number;
  dry_run?: boolean;
  message?: string;
}

export async function bulkWriteBackMetadata(
  options: BulkWriteBackRequest = {}
): Promise<BulkWriteBackResponse> {
  const response = await fetch(`${API_BASE}/audiobooks/bulk-write-back`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(options),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to start bulk write-back');
  }
  return response.json();
}

// Extract track info from filenames
export interface ExtractTrackInfoResponse {
  updated: number;
  total: number;
  segments: BookSegment[];
}

export async function extractTrackInfo(
  bookId: string
): Promise<ExtractTrackInfoResponse> {
  const response = await fetch(
    `${API_BASE}/audiobooks/${bookId}/extract-track-info`,
    { method: 'POST' }
  );
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to extract track info');
  }
  return response.json();
}

// File Relocation
export interface RelocateRequest {
  segment_id?: string;
  new_path?: string;
  folder_path?: string;
}

export interface RelocateResult {
  updated: number;
  errors?: string[];
}

export async function relocateBookFiles(
  bookId: string,
  req: RelocateRequest
): Promise<RelocateResult> {
  const response = await fetch(
    `${API_BASE}/audiobooks/${bookId}/relocate`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    }
  );
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to relocate files');
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

export async function testMetadataSource(
  sourceId: string,
  apiKey: string
): Promise<{ success: boolean; message?: string; error?: string }> {
  const response = await fetch(`${API_BASE}/metadata-sources/test`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ source_id: sourceId, api_key: apiKey }),
  });
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

export async function undoLastApply(bookId: string): Promise<{ message: string; undone_fields: string[] }> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/undo-last-apply`, { method: 'POST' });
  if (!response.ok) throw await buildApiError(response, 'Failed to undo last apply');
  return response.json();
}

export async function undoMetadataChange(bookId: string, field: string): Promise<{ message: string }> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/metadata-history/${field}/undo`, { method: 'POST' });
  if (!response.ok) throw await buildApiError(response, 'Failed to undo change');
  return response.json();
}

// Batch metadata candidate types and functions

export interface CandidateBookInfo {
  id: string;
  title: string;
  author: string;
  file_path: string;
  itunes_path?: string;
  cover_url?: string;
  format?: string;
  duration_seconds?: number;
  file_size_bytes?: number;
}

export interface CandidateResult {
  book: CandidateBookInfo;
  candidate?: MetadataCandidate;
  status: 'matched' | 'no_match' | 'error' | 'rejected';
  error_message?: string;
}

export interface BatchFetchResponse {
  results: CandidateResult[];
  matched: number;
  no_match: number;
  errors: number;
  total: number;
}

export async function batchFetchCandidates(bookIds: string[]): Promise<{ operation_id: string }> {
  const response = await fetch(`${API_BASE}/metadata/batch-fetch-candidates`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ book_ids: bookIds }),
  });
  if (!response.ok) throw await buildApiError(response, 'Failed to start batch fetch');
  return response.json();
}

export async function getOperationResults(operationId: string): Promise<BatchFetchResponse> {
  const response = await fetch(`${API_BASE}/operations/${operationId}/results`);
  if (!response.ok) throw await buildApiError(response, 'Failed to get operation results');
  return response.json();
}

export async function batchApplyCandidates(operationId: string, bookIds: string[]): Promise<{ applied: number }> {
  const response = await fetch(`${API_BASE}/metadata/batch-apply-candidates`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ operation_id: operationId, book_ids: bookIds }),
  });
  if (!response.ok) throw await buildApiError(response, 'Failed to apply candidates');
  return response.json();
}

export async function batchRejectCandidates(operationId: string, bookIds: string[]): Promise<{ rejected: number }> {
  const response = await fetch(`${API_BASE}/metadata/batch-reject-candidates`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ operation_id: operationId, book_ids: bookIds }),
  });
  if (!response.ok) throw await buildApiError(response, 'Failed to reject candidates');
  return response.json();
}

export async function batchUnrejectCandidates(operationId: string, bookIds: string[]): Promise<{ unrejected: number }> {
  const response = await fetch(`${API_BASE}/metadata/batch-unreject-candidates`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ operation_id: operationId, book_ids: bookIds }),
  });
  if (!response.ok) throw await buildApiError(response, 'Failed to unreject candidates');
  return response.json();
}

export async function getRecentCompletedOperations(): Promise<Operation[]> {
  const response = await fetch(`${API_BASE}/operations/recent`);
  if (!response.ok) throw await buildApiError(response, 'Failed to get recent operations');
  const data = await response.json();
  return data.operations || [];
}

export async function revertToSnapshot(bookId: string, timestamp: string): Promise<{ message: string; book: Book }> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/revert-metadata`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ timestamp }),
  });
  if (!response.ok) throw await buildApiError(response, 'Failed to revert to version');
  return response.json();
}

export interface BookVersionEntry {
  book_id: string;
  timestamp: string;
  data: string;
}

export async function getBookCOWVersions(bookId: string, limit?: number): Promise<BookVersionEntry[]> {
  const params = limit ? `?limit=${limit}` : '';
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/cow-versions${params}`);
  if (!response.ok) throw await buildApiError(response, 'Failed to fetch book versions');
  const data = await response.json();
  return data.versions || [];
}

export async function pruneBookVersions(bookId: string, keepCount: number): Promise<{ pruned: number }> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/cow-versions/prune`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ keep_count: keepCount }),
  });
  if (!response.ok) throw await buildApiError(response, 'Failed to prune versions');
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

export async function splitCompositeAuthor(authorId: number, names?: string[]): Promise<{ authors: { id: number; name: string }[]; books_updated: number }> {
  const response = await fetch(`${API_BASE}/authors/${authorId}/split`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(names ? { names } : {}),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to split author');
  }
  return response.json();
}

export async function renameAuthor(authorId: number, name: string): Promise<{ id: number; name: string }> {
  const response = await fetch(`${API_BASE}/authors/${authorId}/name`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to rename author');
  }
  return response.json();
}

export async function reclassifyAuthorAsNarrator(authorId: number): Promise<{ narrator_id: number; books_updated: number }> {
  const response = await fetch(`${API_BASE}/authors/${authorId}/reclassify-as-narrator`, { method: 'POST' });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to reclassify author as narrator');
  }
  return response.json();
}

// --- Unified Task/Scheduler API ---

export interface TaskInfo {
  name: string;
  description: string;
  category: string;
  enabled: boolean;
  interval_minutes: number;
  run_on_startup: boolean;
  run_in_maintenance_window: boolean;
  last_run?: string;
}

export async function getRegisteredTasks(): Promise<TaskInfo[]> {
  const response = await fetch(`${API_BASE}/tasks`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch tasks');
  }
  return response.json();
}

export async function runTask(name: string): Promise<Operation | { message: string }> {
  const response = await fetch(`${API_BASE}/tasks/${name}/run`, { method: 'POST' });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to run task');
  }
  return response.json();
}

export async function runMaintenanceWindow(): Promise<void> {
  const response = await fetch(`${API_BASE}/maintenance-window/run`, { method: 'POST' });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to run maintenance window');
  }
}

export async function updateTaskConfig(
  name: string,
  updates: { enabled?: boolean; interval_minutes?: number; run_on_startup?: boolean; run_in_maintenance_window?: boolean }
): Promise<void> {
  const response = await fetch(`${API_BASE}/tasks/${name}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(updates),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to update task config');
  }
}

// AI Author Review
export type AIReviewMode = 'full' | 'groups';

export async function requestAIAuthorReview(mode: AIReviewMode = 'groups'): Promise<Operation> {
  const response = await fetch(`${API_BASE}/authors/duplicates/ai-review`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ mode }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to start AI author review');
  }
  return response.json();
}

export async function getOperationResult(id: string): Promise<{ result_data: unknown }> {
  const response = await fetch(`${API_BASE}/operations/${id}/result`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to get operation result');
  }
  return response.json();
}

export async function applyAIAuthorReview(
  suggestions: ApplyAISuggestion[]
): Promise<Operation> {
  const response = await fetch(`${API_BASE}/authors/duplicates/ai-review/apply`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ suggestions }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to apply AI author review');
  }
  return response.json();
}

// --- AI Scan Pipeline Types ---

export interface AIScan {
  id: number;
  status: 'pending' | 'scanning' | 'enriching' | 'cross_validating' | 'complete' | 'failed' | 'canceled';
  mode: 'batch' | 'realtime';
  models: { groups: string; full: string };
  author_count: number;
  created_at: string;
  completed_at?: string;
}

export interface AIScanPhase {
  scan_id: number;
  phase_type: string;
  status: string;
  batch_id?: string;
  model: string;
  started_at?: string;
  completed_at?: string;
}

export interface AIScanResult {
  id: number;
  scan_id: number;
  agreement: 'agreed' | 'groups_only' | 'full_only' | 'disagreed';
  suggestion: {
    action: string;
    canonical_name: string;
    reason: string;
    confidence: string;
    author_ids?: number[];
    roles?: SuggestionRoles;
    source: string;
  };
  applied: boolean;
  applied_at?: string;
}

export interface AIScanDetail extends AIScan {
  phases: AIScanPhase[];
}

export interface AIScanComparison {
  new_in_b: AIScanResult[];
  resolved_from_a: AIScanResult[];
  unchanged: AIScanResult[];
}

// --- AI Scan Pipeline API Functions ---

export async function startAIScan(mode: 'batch' | 'realtime'): Promise<AIScan> {
  const response = await fetch(`${API_BASE}/ai/scans`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ mode }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to start AI scan');
  }
  return response.json();
}

export async function listAIScans(): Promise<AIScan[]> {
  const response = await fetch(`${API_BASE}/ai/scans`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to list AI scans');
  }
  const data = await response.json();
  return data.scans || [];
}

export async function getAIScan(id: number): Promise<AIScanDetail> {
  const response = await fetch(`${API_BASE}/ai/scans/${id}`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to get AI scan');
  }
  const data = await response.json();
  return { ...data.scan, phases: data.phases || [] };
}

export async function getAIScanResults(id: number, agreement?: string): Promise<AIScanResult[]> {
  const params = agreement ? `?agreement=${agreement}` : '';
  const response = await fetch(`${API_BASE}/ai/scans/${id}/results${params}`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to get scan results');
  }
  const data = await response.json();
  return data.results || [];
}

export async function applyAIScanResults(scanID: number, resultIDs: number[]): Promise<{ applied: number; errors: string[] }> {
  const response = await fetch(`${API_BASE}/ai/scans/${scanID}/apply`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ result_ids: resultIDs }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to apply scan results');
  }
  return response.json();
}

export async function cancelAIScan(id: number): Promise<void> {
  const response = await fetch(`${API_BASE}/ai/scans/${id}/cancel`, { method: 'POST' });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to cancel scan');
  }
}

export async function deleteAIScan(id: number): Promise<void> {
  const response = await fetch(`${API_BASE}/ai/scans/${id}`, { method: 'DELETE' });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to delete scan');
  }
}

export async function compareAIScans(a: number, b: number): Promise<AIScanComparison> {
  const response = await fetch(`${API_BASE}/ai/scans/compare?a=${a}&b=${b}`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to compare scans');
  }
  return response.json();
}

// --- Rename Preview & Apply ---

export interface TagChange {
  field: string;
  current: string;
  proposed: string;
}

export interface RenamePreview {
  book_id: string;
  current_path: string;
  proposed_path: string;
  tag_changes: TagChange[];
}

export interface RenameApplyResult {
  book_id: string;
  old_path: string;
  new_path: string;
  tags_written: number;
  message: string;
}

export async function previewRename(bookId: string): Promise<RenamePreview> {
  const response = await fetch(
    `${API_BASE}/audiobooks/${bookId}/rename/preview`,
    { method: 'POST' }
  );
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to preview rename');
  }
  return response.json();
}

export async function applyRename(bookId: string): Promise<RenameApplyResult> {
  const response = await fetch(
    `${API_BASE}/audiobooks/${bookId}/rename/apply`,
    { method: 'POST' }
  );
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to apply rename');
  }
  return response.json();
}

// ---- Organize Preview & Execute ----

export interface OrganizePreviewStep {
  action: string;
  description: string;
  from?: string;
  to?: string;
  files?: string[];
  tags?: Record<string, unknown>;
  cover_url?: string;
  warning?: string;
}

export interface OrganizePreviewResponse {
  steps: OrganizePreviewStep[];
  needs_copy: boolean;
  needs_rename: boolean;
  current_path: string;
  target_path: string;
  is_protected: boolean;
  has_book_files: boolean;
  book_file_count: number;
}

export interface OrganizeResult {
  message: string;
  book_id: string;
  old_path: string;
  new_path: string;
  tags_written: number;
  operation_id: string;
}

export async function previewOrganize(bookId: string): Promise<OrganizePreviewResponse> {
  const response = await fetch(
    `${API_BASE}/audiobooks/${bookId}/preview-organize`
  );
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to preview organize');
  }
  return response.json();
}

export async function organizeBook(bookId: string): Promise<OrganizeResult> {
  const response = await fetch(
    `${API_BASE}/audiobooks/${bookId}/organize`,
    { method: 'POST' }
  );
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to organize book');
  }
  return response.json();
}

// ---- Reconciliation ----

export interface ReconcileMatch {
  book_id: string;
  book_title: string;
  old_path: string;
  new_path: string;
  match_type: 'hash' | 'original_hash' | 'filename';
  confidence: 'high' | 'medium' | 'low';
  score: number;
}

export interface ReconcileBrokenRecord {
  book_id: string;
  title: string;
  file_path: string;
  file_hash?: string;
}

export interface ReconcilePreview {
  broken_records: ReconcileBrokenRecord[];
  untracked_files: string[];
  matches: ReconcileMatch[];
  unmatched_books: ReconcileBrokenRecord[];
}

export async function getReconcilePreview(): Promise<ReconcilePreview> {
  const response = await fetch(`${API_BASE}/operations/reconcile/preview`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to get reconcile preview');
  }
  return response.json();
}

export async function startReconcile(
  matches: Array<{ book_id: string; new_path: string }>
): Promise<Operation> {
  const response = await fetch(`${API_BASE}/operations/reconcile`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ matches }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to start reconciliation');
  }
  return response.json();
}

export async function startReconcileScan(): Promise<Operation> {
  const response = await fetch(`${API_BASE}/operations/reconcile/scan`, {
    method: 'POST',
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to start reconcile scan');
  }
  return response.json();
}

export interface LatestReconcileScan {
  operation: Operation | null;
  preview: ReconcilePreview | null;
}

export async function getLatestReconcileScan(): Promise<LatestReconcileScan> {
  const response = await fetch(`${API_BASE}/operations/reconcile/scan/latest`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to get latest reconcile scan');
  }
  return response.json();
}

// Diagnostics
export interface DiagnosticsSuggestion {
  id: string;
  action: 'merge_versions' | 'delete_orphan' | 'fix_metadata' | 'reassign_series';
  book_ids: string[];
  primary_id?: string;
  reason: string;
  fix?: Record<string, unknown>;
  applied: boolean;
}

export interface DiagnosticsAIResults {
  status: string;
  schema_version?: number;
  suggestions: DiagnosticsSuggestion[];
  raw_responses: unknown[];
}

export async function startDiagnosticsExport(
  category: string,
  description: string
): Promise<{ operation_id: string; status: string }> {
  const response = await fetch(`${API_BASE}/diagnostics/export`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ category, description }),
  });
  if (!response.ok) throw await buildApiError(response, 'Failed to start export');
  return response.json();
}

export async function downloadDiagnosticsExport(operationId: string): Promise<Blob> {
  const response = await fetch(`${API_BASE}/diagnostics/export/${operationId}/download`);
  if (!response.ok) throw await buildApiError(response, 'Failed to download export');
  return response.blob();
}

export async function submitDiagnosticsAI(
  category: string,
  description: string
): Promise<{ operation_id: string; batch_id: string; status: string; request_count: number }> {
  const response = await fetch(`${API_BASE}/diagnostics/submit-ai`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ category, description }),
  });
  if (!response.ok) throw await buildApiError(response, 'Failed to submit AI analysis');
  return response.json();
}

export async function getDiagnosticsAIResults(operationId: string): Promise<DiagnosticsAIResults> {
  const response = await fetch(`${API_BASE}/diagnostics/ai-results/${operationId}`);
  if (!response.ok) throw await buildApiError(response, 'Failed to get AI results');
  return response.json();
}

export async function applyDiagnosticsSuggestions(
  operationId: string,
  approvedIds: string[]
): Promise<{ applied: number; failed: number; errors: string[] }> {
  const response = await fetch(`${API_BASE}/diagnostics/apply-suggestions`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ operation_id: operationId, approved_suggestion_ids: approvedIds }),
  });
  if (!response.ok) throw await buildApiError(response, 'Failed to apply suggestions');
  return response.json();
}

// External ID mappings
export interface ExternalIDMapping {
  id: number;
  source: string;
  external_id: string;
  book_id: string;
  track_number?: number;
  file_path?: string;
  tombstoned: boolean;
  created_at: string;
  updated_at: string;
}

export async function getBookExternalIDs(bookId: string): Promise<{
  external_ids: ExternalIDMapping[];
  itunes_linked: boolean;
  total: number;
}> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/external-ids`);
  if (!response.ok) return { external_ids: [], itunes_linked: false, total: 0 };
  return response.json();
}

// --- User tag API functions ---

export async function getBookUserTags(bookId: string): Promise<string[]> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/user-tags`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to get book tags');
  }
  const data = await response.json();
  return data.tags;
}

export async function setBookUserTags(
  bookId: string,
  tags: string[]
): Promise<string[]> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/user-tags`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ tags }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to set book tags');
  }
  const data = await response.json();
  return data.tags;
}

export async function addBookUserTag(
  bookId: string,
  tag: string
): Promise<string[]> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/user-tags`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ tag }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to add book tag');
  }
  const data = await response.json();
  return data.tags;
}

export async function removeBookUserTag(
  bookId: string,
  tag: string
): Promise<string[]> {
  const response = await fetch(
    `${API_BASE}/audiobooks/${bookId}/user-tags/${encodeURIComponent(tag)}`,
    { method: 'DELETE' }
  );
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to remove book tag');
  }
  const data = await response.json();
  return data.tags;
}

export async function bulkUpdateTags(
  bookIds: string[],
  addTags: string[],
  removeTags: string[]
): Promise<number> {
  const response = await fetch(`${API_BASE}/audiobooks/batch-tags`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      book_ids: bookIds,
      add_tags: addTags,
      remove_tags: removeTags,
    }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to bulk update tags');
  }
  const data = await response.json();
  return data.updated;
}

export async function listAllUserTags(): Promise<
  Array<{ tag: string; count: number }>
> {
  const response = await fetch(`${API_BASE}/tags`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to list tags');
  }
  const data = await response.json();
  return data.tags;
}

// --- Column config preferences ---

export interface ColumnConfig {
  visibleColumns: string[]; // column IDs
  columnOrder: string[]; // column IDs in display order
  columnWidths: Record<string, number>; // column ID -> width in px
}

const COLUMN_CONFIG_KEY = 'library_column_config';

export async function getUserColumnConfig(): Promise<ColumnConfig | null> {
  const response = await fetch(
    `${API_BASE}/preferences/${COLUMN_CONFIG_KEY}`
  );
  if (!response.ok) return null;
  const data = await response.json();
  if (!data.value) return null;
  try {
    return JSON.parse(data.value) as ColumnConfig;
  } catch {
    return null;
  }
}

export async function saveUserColumnConfig(
  config: ColumnConfig
): Promise<void> {
  const response = await fetch(
    `${API_BASE}/preferences/${COLUMN_CONFIG_KEY}`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ value: JSON.stringify(config) }),
    }
  );
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to save column config');
  }
}

export async function deleteUserColumnConfig(): Promise<void> {
  const response = await fetch(
    `${API_BASE}/preferences/${COLUMN_CONFIG_KEY}`,
    { method: 'DELETE' }
  );
  // Ignore 404 — config may not exist
  if (!response.ok && response.status !== 404) {
    throw await buildApiError(response, 'Failed to delete column config');
  }
}

// --- Embedding-based deduplication ---

export interface DedupCandidate {
  id: number;
  entity_type: 'book' | 'author';
  entity_a_id: string;
  entity_b_id: string;
  layer: 'exact' | 'embedding' | 'llm';
  similarity?: number;
  llm_verdict?: string;
  llm_reason?: string;
  status: 'pending' | 'merged' | 'dismissed';
  created_at: string;
  updated_at: string;
}

export interface DedupCandidatesResponse {
  candidates: DedupCandidate[];
  total: number;
}

export interface DedupStats {
  entity_type: string;
  layer: string;
  status: string;
  count: number;
}

export async function getDedupCandidates(params?: {
  entity_type?: string;
  status?: string;
  layer?: string;
  min_similarity?: number;
  limit?: number;
  offset?: number;
}): Promise<DedupCandidatesResponse> {
  const qs = new URLSearchParams();
  if (params?.entity_type) qs.set('entity_type', params.entity_type);
  if (params?.status) qs.set('status', params.status);
  if (params?.layer) qs.set('layer', params.layer);
  if (params?.min_similarity != null)
    qs.set('min_similarity', String(params.min_similarity));
  if (params?.limit != null) qs.set('limit', String(params.limit));
  if (params?.offset != null) qs.set('offset', String(params.offset));
  const url = qs.toString()
    ? `${API_BASE}/dedup/candidates?${qs}`
    : `${API_BASE}/dedup/candidates`;
  const response = await fetch(url);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch dedup candidates');
  }
  return response.json();
}

export async function getDedupStats(): Promise<{ stats: DedupStats[] }> {
  const response = await fetch(`${API_BASE}/dedup/stats`);
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to fetch dedup stats');
  }
  return response.json();
}

export async function mergeDedupCandidate(id: number): Promise<void> {
  const response = await fetch(`${API_BASE}/dedup/candidates/${id}/merge`, {
    method: 'POST',
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to merge dedup candidate');
  }
}

export async function dismissDedupCandidate(id: number): Promise<void> {
  const response = await fetch(`${API_BASE}/dedup/candidates/${id}/dismiss`, {
    method: 'POST',
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to dismiss dedup candidate');
  }
}

export interface BulkMergeDedupResult {
  attempted: number;
  merged: number;
  failed: number;
  failures?: Array<{ candidate_id: number; reason: string }>;
}

export async function bulkMergeDedupCandidates(filter: {
  entity_type?: string;
  status?: string;
  layer?: string;
  min_similarity?: number;
  max_similarity?: number;
}): Promise<BulkMergeDedupResult> {
  const response = await fetch(`${API_BASE}/dedup/candidates/bulk-merge`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(filter),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to bulk-merge dedup candidates');
  }
  return response.json();
}

export interface ClusterMergeResult {
  status: string;
  merged_books: number;
  candidates_updated: number;
  result?: unknown;
}

export async function mergeDedupCluster(bookIds: string[]): Promise<ClusterMergeResult> {
  const response = await fetch(`${API_BASE}/dedup/candidates/merge-cluster`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ book_ids: bookIds }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to merge dedup cluster');
  }
  return response.json();
}

export async function dismissDedupCluster(bookIds: string[]): Promise<{ status: string; dismissed: number }> {
  const response = await fetch(`${API_BASE}/dedup/candidates/dismiss-cluster`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ book_ids: bookIds }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to dismiss dedup cluster');
  }
  return response.json();
}

// Remove a single book from a cluster by dismissing only the edges between
// it and the other cluster members. Pairs involving the book with books
// OUTSIDE the cluster are left alone. Used by the per-side "not a
// duplicate" button when one book in a 3+ way cluster is wrong but the
// rest are real duplicates.
export async function removeFromDedupCluster(
  clusterBookIds: string[],
  removeBookId: string
): Promise<{ status: string; dismissed: number }> {
  const response = await fetch(`${API_BASE}/dedup/candidates/remove-from-cluster`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      cluster_book_ids: clusterBookIds,
      remove_book_id: removeBookId,
    }),
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to remove book from dedup cluster');
  }
  return response.json();
}

export async function triggerDedupScan(): Promise<{ status: string }> {
  const response = await fetch(`${API_BASE}/dedup/scan`, { method: 'POST' });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to trigger dedup scan');
  }
  return response.json();
}

export async function triggerDedupLLM(): Promise<{ status: string }> {
  const response = await fetch(`${API_BASE}/dedup/scan-llm`, {
    method: 'POST',
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to trigger dedup LLM scan');
  }
  return response.json();
}

export async function triggerDedupRefresh(): Promise<{ status: string }> {
  const response = await fetch(`${API_BASE}/dedup/refresh`, {
    method: 'POST',
  });
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to trigger dedup refresh');
  }
  return response.json();
}
