// file: web/tests/e2e/utils/test-helpers.ts
// version: 1.6.0
// guid: a1b2c3d4-e5f6-7890-abcd-e1f2a3b4c5d6

import { Page } from '@playwright/test';

// Re-export setup mode helpers for convenience
export {
  resetToFactoryDefaults,
  setupPhase1ApiDriven,
  setupPhase2Interactive,
} from './setup-modes';

export interface MockMetadataSource {
  id: string;
  name: string;
  enabled: boolean;
  priority: number;
  requires_auth: boolean;
  credentials: Record<string, string>;
}

export interface MockConfig {
  root_dir: string;
  database_path: string;
  database_type: string;
  enable_sqlite: boolean;
  playlist_dir: string;
  organization_strategy: string;
  scan_on_startup: boolean;
  auto_organize: boolean;
  folder_naming_pattern: string;
  file_naming_pattern: string;
  create_backups: boolean;
  enable_disk_quota: boolean;
  disk_quota_percent: number;
  enable_user_quotas: boolean;
  default_user_quota_gb: number;
  auto_fetch_metadata: boolean;
  metadata_sources: MockMetadataSource[];
  language: string;
  enable_ai_parsing: boolean;
  openai_api_key: string;
  concurrent_scans: number;
  memory_limit_type: string;
  cache_size: number;
  memory_limit_percent: number;
  memory_limit_mb: number;
  purge_soft_deleted_after_days: number;
  purge_soft_deleted_delete_files: boolean;
  log_level: string;
  log_format: string;
  enable_json_logging: boolean;
  api_keys: { goodreads: string };
  supported_extensions: string[];
  exclude_patterns?: string[];
}

interface MockImportPath {
  id: number;
  path: string;
  book_count: number;
}

interface MockBackup {
  filename: string;
  size: number;
  created_at: string;
  auto?: boolean;
  trigger?: string;
}

interface MockBlockedHash {
  hash: string;
  reason: string;
  created_at: string;
}

interface MockFilesystemItem {
  name: string;
  path: string;
  is_dir: boolean;
  size?: number;
  mod_time?: number;
  excluded: boolean;
}

interface MockFilesystemEntry {
  path: string;
  items: MockFilesystemItem[];
  disk_info?: {
    exists: boolean;
    readable: boolean;
    writable: boolean;
    total_bytes?: number;
    free_bytes?: number;
    library_bytes?: number;
  };
}

interface MockOperationLog {
  id: string;
  level: string;
  message: string;
  created_at: string;
  details?: string;
}

interface MockOperation {
  id: string;
  type: string;
  status: string;
  progress: number;
  total: number;
  message?: string;
  created_at: string;
  folder_path?: string;
  error_message?: string;
}

interface MockActiveOperation {
  id: string;
  type: string;
  status: string;
  progress: number;
  total: number;
  message: string;
  folder_path?: string;
}

interface MockITunesValidation {
  total_tracks: number;
  audiobook_tracks: number;
  files_found: number;
  files_missing: number;
  missing_paths?: string[];
  duplicate_count: number;
  estimated_import_time: string;
}

interface MockITunesImportStatus {
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

interface MockITunesState {
  validation?: MockITunesValidation;
  importStatus?: MockITunesImportStatus;
}

interface MockFailures {
  getBooks?: number | 'timeout';
  searchBooks?: number | 'timeout';
  getConfig?: number;
  updateConfig?: number;
  openaiTest?: number;
  createBackup?: number;
  listBackups?: number;
  restoreBackup?: number;
  deleteBackup?: number;
  blockedHashes?: number;
  filesystem?: number;
  importFile?: number | 'timeout';
  operationsActive?: number;
  operationLogs?: number;
}

export interface MockApiOptions {
  books?: TestBook[];
  config?: Partial<MockConfig>;
  importPaths?: MockImportPath[];
  backups?: MockBackup[];
  blockedHashes?: MockBlockedHash[];
  filesystem?: Record<string, MockFilesystemEntry>;
  homeDirectory?: string;
  operations?: {
    active?: MockActiveOperation[];
    history?: MockOperation[];
    logs?: Record<string, MockOperationLog[]>;
  };
  itunes?: MockITunesState;
  failures?: MockFailures;
  systemStatus?: Record<string, unknown>;
}

interface TestBook {
  id: string;
  title: string;
  author_name?: string;
  series_name?: string | null;
  series_position?: number | null;
  library_state: string;
  marked_for_deletion: boolean;
  language?: string;
  file_path: string;
  file_hash: string;
  original_file_hash: string;
  organized_file_hash?: string | null;
  created_at: string;
  updated_at: string;
  duration?: number;
  file_size?: number;
  publisher?: string;
  description?: string;
  audiobook_release_year?: number;
  print_year?: number;
  quality?: string;
  bitrate?: number;
  codec?: string;
  sample_rate?: number;
  format?: string;
  version_group_id?: string;
  version_notes?: string;
  is_primary_version?: boolean;
  fetch_metadata_error?: boolean;
  fetch_metadata_delay_ms?: number;
  organize_error?: string;
  force_update_required?: boolean;
}

/**
 * Mock EventSource to prevent SSE connections during tests
 */
export async function mockEventSource(page: Page) {
  await page.addInitScript(() => {
    class MockEventSource {
      static instances: MockEventSource[] = [];
      url: string;
      onopen: ((event: Event) => void) | null = null;
      onerror: ((event: Event) => void) | null = null;
      onmessage: ((event: MessageEvent) => void) | null = null;
      constructor(url: string) {
        this.url = url;
        MockEventSource.instances.push(this);
        window.setTimeout(() => {
          if (this.onopen) {
            this.onopen(new Event('open'));
          }
        }, 0);
      }
      addEventListener() {}
      removeEventListener() {}
      close() {}
      emitOpen() {
        if (this.onopen) {
          this.onopen(new Event('open'));
        }
      }
      emitError() {
        if (this.onerror) {
          this.onerror(new Event('error'));
        }
      }
      emitMessage(data: Record<string, unknown>) {
        if (this.onmessage) {
          const event = new MessageEvent('message', {
            data: JSON.stringify(data),
          });
          this.onmessage(event);
        }
      }
    }
    (window as unknown as { EventSource: typeof EventSource }).EventSource =
      MockEventSource as unknown as typeof EventSource;
    (window as unknown as { __mockEventSource: typeof MockEventSource })
      .__mockEventSource = MockEventSource;
  });
}

/**
 * Skip welcome wizard
 */
export async function skipWelcomeWizard(page: Page) {
  await page.addInitScript(() => {
    localStorage.setItem('welcome_wizard_completed', 'true');
  });
}

/**
 * Setup common routes for all tests
 */
export async function setupCommonRoutes(page: Page) {
  await page.route('**/api/v1/health', (route) => {
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ status: 'ok' }),
    });
  });

  await page.route('**/api/v1/system/status', (route) => {
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        status: 'ok',
        library: { book_count: 0, folder_count: 1, total_size: 0 },
        import_paths: { book_count: 0, folder_count: 0, total_size: 0 },
        memory: {},
        runtime: {},
        operations: { recent: [] },
      }),
    });
  });
}

/**
 * Wait for toast notification
 */
export async function waitForToast(
  page: Page,
  text: string,
  timeout = 5000
) {
  await page.waitForSelector(`text=${text}`, { timeout });
}

const DEFAULT_METADATA_SOURCES: MockMetadataSource[] = [
  {
    id: 'audible',
    name: 'Audible',
    enabled: true,
    priority: 1,
    requires_auth: false,
    credentials: {},
  },
  {
    id: 'openlibrary',
    name: 'Open Library',
    enabled: true,
    priority: 2,
    requires_auth: true,
    credentials: { apiKey: '' },
  },
];

const DEFAULT_CONFIG: MockConfig = {
  root_dir: '/library',
  database_path: '/data/library.db',
  database_type: 'pebble',
  enable_sqlite: false,
  playlist_dir: '/library/playlists',
  organization_strategy: 'auto',
  scan_on_startup: false,
  auto_organize: true,
  folder_naming_pattern: '{author}/{series}/{title} ({print_year})',
  file_naming_pattern: '{title} - {author} - read by {narrator}',
  create_backups: true,
  enable_disk_quota: false,
  disk_quota_percent: 80,
  enable_user_quotas: false,
  default_user_quota_gb: 100,
  auto_fetch_metadata: true,
  metadata_sources: DEFAULT_METADATA_SOURCES,
  language: 'en',
  enable_ai_parsing: false,
  openai_api_key: '',
  concurrent_scans: 4,
  memory_limit_type: 'items',
  cache_size: 1000,
  memory_limit_percent: 25,
  memory_limit_mb: 512,
  purge_soft_deleted_after_days: 30,
  purge_soft_deleted_delete_files: false,
  log_level: 'info',
  log_format: 'text',
  enable_json_logging: false,
  api_keys: { goodreads: '' },
  supported_extensions: ['.m4b', '.mp3', '.m4a'],
  exclude_patterns: [],
};

const DEFAULT_SYSTEM_STATUS = {
  status: 'ok',
  library_book_count: 0,
  import_book_count: 0,
  total_book_count: 0,
  library_size_bytes: 0,
  import_size_bytes: 0,
  total_size_bytes: 0,
  disk_total_bytes: 500 * 1024 * 1024 * 1024,
  disk_used_bytes: 0,
  disk_free_bytes: 500 * 1024 * 1024 * 1024,
  root_directory: '/library',
  library: {
    book_count: 0,
    folder_count: 1,
    total_size: 0,
    path: '/library',
  },
  import_paths: {
    book_count: 0,
    folder_count: 0,
    total_size: 0,
  },
  memory: {
    alloc_bytes: 0,
    total_alloc_bytes: 0,
    sys_bytes: 0,
    num_gc: 0,
    heap_alloc: 0,
    heap_sys: 0,
  },
  runtime: {
    go_version: 'go1.25',
    num_goroutine: 1,
    num_cpu: 4,
    os: 'linux',
    arch: 'amd64',
  },
  operations: { recent: [] },
};

const buildConfig = (overrides?: Partial<MockConfig>): MockConfig => ({
  ...DEFAULT_CONFIG,
  ...overrides,
  metadata_sources:
    overrides?.metadata_sources ?? DEFAULT_CONFIG.metadata_sources,
  supported_extensions:
    overrides?.supported_extensions ?? DEFAULT_CONFIG.supported_extensions,
  exclude_patterns:
    overrides?.exclude_patterns ?? DEFAULT_CONFIG.exclude_patterns,
});

const buildSystemStatus = (
  overrides?: Record<string, unknown>
): Record<string, unknown> => ({
  ...DEFAULT_SYSTEM_STATUS,
  ...(overrides || {}),
});

/**
 * Generate test audiobooks
 */
export function generateTestBooks(count: number) {
  const authors = [
    'Brandon Sanderson',
    'J.R.R. Tolkien',
    'Terry Pratchett',
    'Isaac Asimov',
    'Ursula K. Le Guin',
  ];
  const series = [
    'The Stormlight Archive',
    'The Lord of the Rings',
    'Discworld',
    'Foundation',
    'Earthsea',
  ];
  const languages = ['en', 'es', 'fr'];

  return Array.from({ length: count }, (_, i) => ({
    id: `book-${i + 1}`,
    title: `Test Book ${i + 1}`,
    author_name: authors[i % authors.length],
    series_name: i % 3 === 0 ? series[i % series.length] : null,
    series_position: i % 3 === 0 ? (i % 5) + 1 : null,
    library_state: i % 4 === 0 ? 'import' : 'organized',
    marked_for_deletion: false,
    language: languages[i % languages.length],
    file_path: `/library/book${i + 1}.m4b`,
    file_hash: `hash-${i + 1}`,
    original_file_hash: `hash-orig-${i + 1}`,
    organized_file_hash: i % 4 !== 0 ? `hash-org-${i + 1}` : null,
    created_at: new Date(2024, 0, i + 1).toISOString(),
    updated_at: new Date(2024, 11, i + 1).toISOString(),
    duration: 3600 + (i * 100),
    file_size: 100000000 + (i * 1000000),
  }));
}

/**
 * Generate test audiobook with full metadata
 */
export function generateTestBook(overrides: Record<string, unknown> = {}) {
  return {
    id: 'test-book-1',
    title: 'The Way of Kings',
    author_name: 'Brandon Sanderson',
    narrator: 'Michael Kramer, Kate Reading',
    series_name: 'The Stormlight Archive',
    series_position: 1,
    publisher: 'Tor Books',
    audiobook_release_year: 2010,
    language: 'en',
    isbn: '9780765326355',
    description: 'Epic fantasy novel',
    genre: 'Fantasy',
    library_state: 'organized',
    marked_for_deletion: false,
    file_path:
      '/library/Brandon Sanderson/The Stormlight Archive/' +
      'The Way of Kings.m4b',
    file_hash: 'hash-twok',
    original_file_hash: 'hash-orig-twok',
    organized_file_hash: 'hash-org-twok',
    created_at: '2024-01-01T12:00:00Z',
    updated_at: '2024-12-01T12:00:00Z',
    duration: 45600,
    file_size: 450000000,
    ...overrides,
  };
}

/**
 * Setup shared mock API responses and state for tests
 */
export async function setupMockApi(
  page: Page,
  options: MockApiOptions = {}
) {
  const configData = buildConfig(options.config);
  const systemStatusData = buildSystemStatus(options.systemStatus);
  const importPathsData = options.importPaths || [];
  const backupsData = options.backups || [];
  const blockedHashesData = options.blockedHashes || [];
  const filesystemData = options.filesystem || {};
  const homeDirectoryData = options.homeDirectory || '/';
  const operationsData = options.operations || {};
  const itunesData = options.itunes || {};
  const failuresData = options.failures || {};
  const bookData = options.books || [];

  await page.addInitScript(
    ({
      bookData,
      configData,
      importPathsData,
      backupsData,
      blockedHashesData,
      filesystemData,
      homeDirectoryData,
      operationsData,
      itunesData,
      failuresData,
      systemStatusData,
    }: {
      bookData: TestBook[];
      configData: MockConfig;
      importPathsData: MockImportPath[];
      backupsData: MockBackup[];
      blockedHashesData: MockBlockedHash[];
      filesystemData: Record<string, MockFilesystemEntry>;
      homeDirectoryData: string;
      operationsData: {
        active?: MockActiveOperation[];
        history?: MockOperation[];
        logs?: Record<string, MockOperationLog[]>;
      };
      itunesData: MockITunesState;
      failuresData: MockFailures;
      systemStatusData: Record<string, unknown>;
    }) => {
      let libraryBooks = [...bookData];
      let configState = { ...configData };
      let importPaths = [...importPathsData];
      let backups = [...backupsData];
      let blockedHashes = [...blockedHashesData];
      let filesystem = { ...filesystemData };
      let activeOperations = [...(operationsData.active || [])];
      let historyOperations = [...(operationsData.history || [])];
      let operationLogs = { ...(operationsData.logs || {}) };
      const defaultITunesValidation: MockITunesValidation = {
        total_tracks: 120,
        audiobook_tracks: 12,
        files_found: 12,
        files_missing: 0,
        missing_paths: [],
        duplicate_count: 1,
        estimated_import_time: '12 seconds',
      };
      const defaultITunesImportStatus: MockITunesImportStatus = {
        operation_id: 'op-itunes-default',
        status: 'completed',
        progress: 100,
        message: 'Import completed',
        total_books: 12,
        processed: 12,
        imported: 11,
        skipped: 1,
        failed: 0,
        errors: [],
      };
      const itunesValidation = itunesData.validation || defaultITunesValidation;
      let itunesImportStatus =
        itunesData.importStatus || defaultITunesImportStatus;
      let itunesOperationId = itunesImportStatus.operation_id;
      const failures = { ...failuresData };

      const jsonResponse = (body: unknown, status = 200) =>
        new Response(JSON.stringify(body), {
          status,
          headers: { 'Content-Type': 'application/json' },
        });

      const originalFetch = window.fetch.bind(window);
      const parseJsonBody = (init?: RequestInit) => {
        if (!init?.body) return null;
        if (typeof init.body === 'string') {
          try {
            return JSON.parse(init.body);
          } catch {
            return null;
          }
        }
        return null;
      };

      const apiState = {
        searchCalls: 0,
        activeOperations,
        historyOperations,
      };

      const updateApiState = () => {
        apiState.activeOperations = activeOperations;
        apiState.historyOperations = historyOperations;
      };

      const failWithStatus = (
        status: number | undefined,
        message: string
      ) => {
        if (!status) return null;
        return jsonResponse({ error: message }, status);
      };

      const maybeTimeout = (value: number | 'timeout' | undefined) => {
        if (value === 'timeout') {
          return Promise.reject(new Error('timeout'));
        }
        return null;
      };

      const maskSecretValue = (value: string) => {
        if (!value) return '';
        return `***${value.slice(-4)}`;
      };

      const maskedConfig = () => {
        const apiKeys = configState.api_keys || { goodreads: '' };
        return {
          ...configState,
          openai_api_key: maskSecretValue(configState.openai_api_key || ''),
          api_keys: {
            ...apiKeys,
            goodreads: maskSecretValue(apiKeys.goodreads || ''),
          },
        };
      };

      const updateExcluded = (path: string, excluded: boolean) => {
        Object.values(filesystem).forEach((entry) => {
          entry.items = entry.items.map((item) =>
            item.path === path ? { ...item, excluded } : item
          );
        });
      };

      const getVersionGroupId = (bookId: string): string | null => {
        const match = libraryBooks.find((book) => book.id === bookId);
        return match?.version_group_id || null;
      };

      const ensureVersionGroup = (bookId: string): string => {
        const existing = getVersionGroupId(bookId);
        if (existing) return existing;
        const newGroup = `group-${Date.now()}-${Math.random()}`;
        libraryBooks = libraryBooks.map((book) =>
          book.id === bookId
            ? { ...book, version_group_id: newGroup, is_primary_version: true }
            : book
        );
        return newGroup;
      };

      const getVersionsForBook = (bookId: string) => {
        const groupId = getVersionGroupId(bookId);
        if (!groupId) return [];
        return libraryBooks.filter((book) => book.version_group_id === groupId);
      };

      const normalizeRoot = (root: string) => {
        return root.endsWith('/') ? root.slice(0, -1) : root;
      };

      const getFileName = (path: string | undefined, id: string) => {
        if (!path) return `${id}.m4b`;
        const parts = path.split('/');
        const name = parts[parts.length - 1];
        return name || `${id}.m4b`;
      };

      const buildOrganizedBook = (book: TestBook): TestBook => {
        const rootDir = normalizeRoot(configState.root_dir || '/library');
        const fileName = getFileName(book.file_path, book.id);
        const organizedHash =
          book.organized_file_hash ||
          (book.file_hash
            ? `organized-${book.file_hash}`
            : `organized-${book.id}`);
        return {
          ...book,
          library_state: 'organized',
          marked_for_deletion: false,
          file_path: `${rootDir}/${fileName}`,
          organized_file_hash: organizedHash,
        };
      };

      (window as unknown as { __apiMock: unknown }).__apiMock = {
        state: apiState,
        setActiveOperations: (ops: MockActiveOperation[]) => {
          activeOperations = ops;
          updateApiState();
        },
        setHistoryOperations: (ops: MockOperation[]) => {
          historyOperations = ops;
          updateApiState();
        },
        setOperationLogs: (id: string, logs: MockOperationLog[]) => {
          operationLogs[id] = logs;
        },
        setConfig: (updates: Partial<MockConfig>) => {
          configState = { ...configState, ...updates };
        },
      };

      window.fetch = (input: RequestInfo | URL, init?: RequestInit) => {
        const url =
          typeof input === 'string'
            ? input
            : input instanceof URL
              ? input.toString()
              : input.url;
        const method = (init?.method || 'GET').toUpperCase();
        const urlObj = new URL(url, window.location.origin);
        const pathname = urlObj.pathname;

        const applySearchFilter = (
          items: typeof libraryBooks,
          query: string
        ) => {
          if (!query) return items;
          const searchLower = query.toLowerCase();
          return items.filter((b) => {
            const title = b.title?.toLowerCase() || '';
            const author = b.author_name?.toLowerCase() || '';
            const series = b.series_name?.toLowerCase() || '';
            return (
              title.includes(searchLower) ||
              author.includes(searchLower) ||
              series.includes(searchLower)
            );
          });
        };

        // Health/system
        if (pathname === '/api/v1/health') {
          return Promise.resolve(jsonResponse({ status: 'ok' }));
        }
        if (pathname === '/api/v1/system/status') {
          const librarySize = libraryBooks.reduce(
            (sum, book) => sum + (book.file_size || 0),
            0
          );
          const importBookCount = libraryBooks.filter(
            (book) =>
              book.library_state === 'import' && !book.marked_for_deletion
          ).length;
          const totalBooks = libraryBooks.length;
          const diskTotal =
            (systemStatusData as { disk_total_bytes?: number })
              .disk_total_bytes || 0;
          const status = {
            ...systemStatusData,
            library: {
              ...(systemStatusData as { library?: Record<string, unknown> })
                .library,
              book_count: totalBooks,
              total_size: librarySize,
            },
            import_paths: {
              ...(systemStatusData as {
                import_paths?: Record<string, unknown>;
              }).import_paths,
              folder_count: importPaths.length,
              book_count: importBookCount,
            },
            operations: { recent: historyOperations },
            library_book_count: totalBooks,
            import_book_count: importBookCount,
            total_book_count: totalBooks,
            library_size_bytes: librarySize,
            total_size_bytes: librarySize,
            disk_used_bytes: librarySize,
            disk_free_bytes: Math.max(0, diskTotal - librarySize),
          };
          return Promise.resolve(jsonResponse(status));
        }
        if (pathname === '/api/v1/config') {
          if (method === 'GET') {
            const failed = failWithStatus(
              failures.getConfig,
              'Failed to fetch config'
            );
            if (failed) {
              return Promise.resolve(failed);
            }
            return Promise.resolve(jsonResponse({ config: maskedConfig() }));
          }
          if (method === 'PUT') {
            const failed = failWithStatus(
              failures.updateConfig,
              'Failed to update config'
            );
            if (failed) {
              return Promise.resolve(failed);
            }
            const updates = parseJsonBody(init) || {};
            if (updates.api_keys) {
              const apiKeys = configState.api_keys || { goodreads: '' };
              configState.api_keys = { ...apiKeys, ...updates.api_keys };
            }
            configState = { ...configState, ...updates };
            return Promise.resolve(jsonResponse({ config: maskedConfig() }));
          }
        }
        if (pathname === '/api/v1/ai/test-connection' && method === 'POST') {
          const failed = failWithStatus(
            failures.openaiTest,
            'Connection failed'
          );
          if (failed) {
            return Promise.resolve(failed);
          }
          return Promise.resolve(
            jsonResponse({
              success: true,
              model: 'gpt-4o-mini',
              message: 'Connection successful',
            })
          );
        }

        if (pathname === '/api/v1/import-paths' && method === 'GET') {
          return Promise.resolve(jsonResponse({ importPaths }));
        }
        if (pathname === '/api/v1/import-paths' && method === 'POST') {
          const payload = parseJsonBody(init) || {};
          const nextId =
            importPaths.reduce((max, item) => Math.max(max, item.id), 0) + 1;
          const newPath = {
            id: nextId,
            path: payload.path || '/unknown',
            book_count: 0,
          };
          importPaths = [...importPaths, newPath];
          return Promise.resolve(jsonResponse(newPath, 201));
        }
        if (
          pathname.startsWith('/api/v1/import-paths/') &&
          method === 'DELETE'
        ) {
          const id = Number(pathname.split('/').pop() || 0);
          importPaths = importPaths.filter((item) => item.id !== id);
          return Promise.resolve(jsonResponse({ message: 'Removed' }));
        }
        if (pathname === '/api/v1/import/file' && method === 'POST') {
          const timeout = maybeTimeout(failures.importFile);
          if (timeout) {
            return timeout;
          }
          const failed = failWithStatus(
            typeof failures.importFile === 'number'
              ? failures.importFile
              : undefined,
            'Failed to import file'
          );
          if (failed) {
            return Promise.resolve(failed);
          }
          const payload = parseJsonBody(init) || {};
          const newBook = {
            id: `import-${Date.now()}`,
            title: payload.file_path?.split('/').pop() || 'Imported Book',
            author_name: 'Unknown',
            series_name: null,
            series_position: null,
            library_state: payload.organize ? 'organized' : 'import',
            marked_for_deletion: false,
            language: 'en',
            file_path: payload.file_path || '/imported/book.m4b',
            file_hash: `hash-${Date.now()}`,
            original_file_hash: `hash-orig-${Date.now()}`,
            organized_file_hash: payload.organize
              ? `hash-org-${Date.now()}`
              : null,
            created_at: new Date().toISOString(),
            updated_at: new Date().toISOString(),
          };
          libraryBooks = [newBook, ...libraryBooks];
          return Promise.resolve(
            jsonResponse({ message: 'Import started', book: newBook })
          );
        }

        if (pathname === '/api/v1/backup/list' && method === 'GET') {
          const failed = failWithStatus(
            failures.listBackups,
            'Failed to list backups'
          );
          if (failed) {
            return Promise.resolve(failed);
          }
          return Promise.resolve(jsonResponse({ backups }));
        }
        if (pathname === '/api/v1/backup/create' && method === 'POST') {
          const failed = failWithStatus(
            failures.createBackup,
            'Failed to create backup'
          );
          if (failed) {
            return Promise.resolve(failed);
          }
          const filename = `backup-${Date.now()}.db.gz`;
          const created = {
            filename,
            size: 25 * 1024 * 1024,
            created_at: new Date().toISOString(),
            auto: false,
          };
          backups = [created, ...backups];
          return Promise.resolve(jsonResponse({ backup: created }));
        }
        if (pathname === '/api/v1/backup/restore' && method === 'POST') {
          const failed = failWithStatus(
            failures.restoreBackup,
            'Backup file is corrupt'
          );
          if (failed) {
            return Promise.resolve(failed);
          }
          return Promise.resolve(jsonResponse({ message: 'Restored' }));
        }
        if (pathname.startsWith('/api/v1/backup/') && method === 'DELETE') {
          const failed = failWithStatus(
            failures.deleteBackup,
            'Failed to delete backup'
          );
          if (failed) {
            return Promise.resolve(failed);
          }
          const filename = pathname.split('/').pop() || '';
          backups = backups.filter((item) => item.filename !== filename);
          return Promise.resolve(jsonResponse({ message: 'Deleted' }));
        }

        if (pathname === '/api/v1/blocked-hashes' && method === 'GET') {
          const failed = failWithStatus(
            failures.blockedHashes,
            'Failed to load blocked hashes'
          );
          if (failed) {
            return Promise.resolve(failed);
          }
          return Promise.resolve(
            jsonResponse({
              items: blockedHashes,
              total: blockedHashes.length,
            })
          );
        }
        if (pathname === '/api/v1/blocked-hashes' && method === 'POST') {
          const payload = parseJsonBody(init) || {};
          const newItem = {
            hash: payload.hash || 'missing',
            reason: payload.reason || 'Manual block',
            created_at: new Date().toISOString(),
          };
          blockedHashes = [newItem, ...blockedHashes];
          return Promise.resolve(jsonResponse(newItem, 201));
        }
        if (
          pathname.startsWith('/api/v1/blocked-hashes/') &&
          method === 'DELETE'
        ) {
          const hash = pathname.split('/').pop() || '';
          blockedHashes = blockedHashes.filter((item) => item.hash !== hash);
          return Promise.resolve(jsonResponse({ message: 'Removed' }));
        }

        if (pathname === '/api/v1/filesystem/home' && method === 'GET') {
          return Promise.resolve(
            jsonResponse({ path: homeDirectoryData })
          );
        }

        if (pathname === '/api/v1/filesystem/browse' && method === 'GET') {
          const failed = failWithStatus(
            failures.filesystem,
            'Failed to browse filesystem'
          );
          if (failed) {
            return Promise.resolve(failed);
          }
          const pathParam = urlObj.searchParams.get('path') || '/';
          const entry = filesystem[pathParam];
          if (entry) {
            return Promise.resolve(
              jsonResponse({
                path: entry.path,
                items: entry.items,
                count: entry.items.length,
                disk_info: entry.disk_info,
              })
            );
          }
          return Promise.resolve(
            jsonResponse({
              path: pathParam,
              items: [],
              count: 0,
              disk_info: {
                exists: true,
                readable: true,
                writable: true,
                total_bytes: 100 * 1024 * 1024 * 1024,
                free_bytes: 50 * 1024 * 1024 * 1024,
                library_bytes: 10 * 1024 * 1024 * 1024,
              },
            })
          );
        }
        if (pathname === '/api/v1/filesystem/exclude' && method === 'POST') {
          const payload = parseJsonBody(init) || {};
          updateExcluded(payload.path, true);
          return Promise.resolve(
            jsonResponse({ excluded: true, path: payload.path })
          );
        }
        if (pathname === '/api/v1/filesystem/exclude' && method === 'DELETE') {
          const payload = parseJsonBody(init) || {};
          updateExcluded(payload.path, false);
          return Promise.resolve(
            jsonResponse({ excluded: false, path: payload.path })
          );
        }

        if (pathname === '/api/v1/itunes/validate' && method === 'POST') {
          return Promise.resolve(jsonResponse(itunesValidation));
        }
        if (pathname === '/api/v1/itunes/import' && method === 'POST') {
          itunesOperationId = `op-itunes-${Date.now()}`;
          itunesImportStatus = {
            ...itunesImportStatus,
            operation_id: itunesOperationId,
          };
          return Promise.resolve(
            jsonResponse({
              operation_id: itunesOperationId,
              status: 'queued',
              message: 'iTunes import queued',
            })
          );
        }
        if (
          pathname.startsWith('/api/v1/itunes/import-status/') &&
          method === 'GET'
        ) {
          const opId = pathname.split('/').pop() || itunesOperationId;
          return Promise.resolve(
            jsonResponse({ ...itunesImportStatus, operation_id: opId })
          );
        }
        if (pathname === '/api/v1/itunes/write-back' && method === 'POST') {
          return Promise.resolve(
            jsonResponse({
              success: true,
              updated_count: itunesImportStatus.imported || 0,
              message: 'Write-back completed',
            })
          );
        }

        if (pathname === '/api/v1/operations/active' && method === 'GET') {
          const failed = failWithStatus(
            failures.operationsActive,
            'Failed to fetch operations'
          );
          if (failed) {
            return Promise.resolve(failed);
          }
          return Promise.resolve(
            jsonResponse({ operations: activeOperations })
          );
        }
        if (pathname === '/api/v1/operations/scan' && method === 'POST') {
          const op = {
            id: `op-scan-${Date.now()}`,
            type: 'scan',
            status: 'running',
            progress: 0,
            total: libraryBooks.length,
            message: 'Scanning',
            folder_path: parseJsonBody(init)?.folder_path,
          };
          activeOperations = [op, ...activeOperations];
          updateApiState();
          return Promise.resolve(jsonResponse(op));
        }
        if (pathname === '/api/v1/operations/organize' && method === 'POST') {
          const payload = parseJsonBody(init) || {};
          const ids = Array.isArray(payload.book_ids) ? payload.book_ids : [];
          if (ids.length > 0) {
            libraryBooks = libraryBooks.map((book) =>
              ids.includes(book.id) ? buildOrganizedBook(book) : book
            );
          }
          const op = {
            id: `op-organize-${Date.now()}`,
            type: 'organize',
            status: 'running',
            progress: 0,
            total: ids.length || libraryBooks.length,
            message: 'Organizing',
            folder_path: payload.folder_path,
          };
          activeOperations = [op, ...activeOperations];
          updateApiState();
          return Promise.resolve(jsonResponse(op));
        }
        if (
          pathname.startsWith('/api/v1/operations/') &&
          pathname.endsWith('/logs') &&
          method === 'GET'
        ) {
          const failed = failWithStatus(
            failures.operationLogs,
            'Failed to fetch operation logs'
          );
          if (failed) {
            return Promise.resolve(failed);
          }
          const parts = pathname.split('/').filter(Boolean);
          const opId = parts[parts.length - 2];
          const logs = operationLogs[opId] || [];
          const tail = parseInt(urlObj.searchParams.get('tail') || '0', 10);
          const items = tail > 0 ? logs.slice(-tail) : logs;
          return Promise.resolve(jsonResponse({ logs: items, items }));
        }
        if (
          pathname.startsWith('/api/v1/operations/') &&
          method === 'DELETE'
        ) {
          const opId = pathname.split('/').pop() || '';
          activeOperations = activeOperations.filter((op) => op.id !== opId);
          historyOperations = [
            {
              id: opId,
              type: 'scan',
              status: 'cancelled',
              progress: 0,
              total: 0,
              message: 'Operation cancelled',
              created_at: new Date().toISOString(),
            },
            ...historyOperations,
          ];
          updateApiState();
          return Promise.resolve(jsonResponse({ message: 'Cancelled' }));
        }

        if (pathname === '/api/v1/authors' && method === 'GET') {
          const authors = Array.from(
            new Set(
              libraryBooks.map((book) => book.author_name).filter(Boolean)
            )
          ).map((name, index) => ({ id: `author-${index}`, name }));
          return Promise.resolve(jsonResponse({ authors }));
        }
        if (pathname === '/api/v1/series' && method === 'GET') {
          const series = Array.from(
            new Set(
              libraryBooks.map((book) => book.series_name).filter(Boolean)
            )
          ).map((name, index) => ({ id: `series-${index}`, name }));
          return Promise.resolve(jsonResponse({ series }));
        }

        if (pathname === '/api/v1/audiobooks/count' && method === 'GET') {
          return Promise.resolve(jsonResponse({ count: libraryBooks.length }));
        }

        if (
          pathname === '/api/v1/audiobooks/soft-deleted' &&
          method === 'GET'
        ) {
          const deleted = libraryBooks.filter(
            (book) => book.marked_for_deletion
          );
          return Promise.resolve(
            jsonResponse({
              items: deleted,
              total: deleted.length,
              count: deleted.length,
            })
          );
        }

        if (pathname === '/api/v1/audiobooks/search' && method === 'GET') {
          const timeout = maybeTimeout(failures.searchBooks);
          if (timeout) {
            return timeout;
          }
          const failed = failWithStatus(
            typeof failures.searchBooks === 'number'
              ? failures.searchBooks
              : undefined,
            'Failed to search books'
          );
          if (failed) {
            return Promise.resolve(failed);
          }
          const query =
            urlObj.searchParams.get('q') ||
            urlObj.searchParams.get('search') ||
            '';
          const limit = parseInt(urlObj.searchParams.get('limit') || '50');
          const filtered = applySearchFilter([...libraryBooks], query);
          const paginated = filtered.slice(0, limit);
          apiState.searchCalls += 1;
          return Promise.resolve(
            jsonResponse({
              items: paginated,
              audiobooks: paginated,
              total: filtered.length,
            })
          );
        }

        if (pathname === '/api/v1/audiobooks' && method === 'GET') {
          const timeout = maybeTimeout(failures.getBooks);
          if (timeout) {
            return timeout;
          }
          const failed = failWithStatus(
            typeof failures.getBooks === 'number'
              ? failures.getBooks
              : undefined,
            'Failed to fetch books'
          );
          if (failed) {
            return Promise.resolve(failed);
          }
          const limit = parseInt(urlObj.searchParams.get('limit') || '24');
          const offset = parseInt(urlObj.searchParams.get('offset') || '0');
          const page = parseInt(urlObj.searchParams.get('page') || '1');

          const effectiveOffset =
            offset > 0 ? offset : Math.max(0, (page - 1) * limit);

          const paginatedBooks = libraryBooks.slice(
            effectiveOffset,
            effectiveOffset + limit
          );

          return Promise.resolve(
            jsonResponse({
              items: paginatedBooks,
              audiobooks: paginatedBooks,
              total: libraryBooks.length,
              page,
              limit,
            })
          );
        }

        if (pathname.startsWith('/api/v1/audiobooks/') && method === 'GET') {
          const parts = pathname.split('/').filter(Boolean);
          const bookId = parts[parts.length - 1];
          if (bookId && bookId !== 'audiobooks') {
            const match = libraryBooks.find((book) => book.id === bookId);
            if (!match) {
              return Promise.resolve(
                jsonResponse({ error: 'Not found' }, 404)
              );
            }
            return Promise.resolve(jsonResponse(match));
          }
        }

        if (
          pathname.startsWith('/api/v1/audiobooks/') &&
          pathname.endsWith('/tags') &&
          method === 'GET'
        ) {
          return Promise.resolve(
            jsonResponse({
              tags: [],
              media: [],
              file_count: 0,
              duration: 0,
            })
          );
        }

        if (
          pathname.startsWith('/api/v1/audiobooks/') &&
          pathname.endsWith('/versions') &&
          method === 'GET'
        ) {
          const parts = pathname.split('/').filter(Boolean);
          const bookId = parts[parts.length - 2];
          return Promise.resolve(
            jsonResponse({ versions: getVersionsForBook(bookId) })
          );
        }
        if (
          pathname.startsWith('/api/v1/audiobooks/') &&
          pathname.endsWith('/versions') &&
          method === 'POST'
        ) {
          const parts = pathname.split('/').filter(Boolean);
          const bookId = parts[parts.length - 2];
          const payload = parseJsonBody(init) || {};
          const otherId = payload.other_id;
          const groupId = ensureVersionGroup(bookId);
          libraryBooks = libraryBooks.map((book) =>
            book.id === otherId
              ? { ...book, version_group_id: groupId }
              : book
          );
          return Promise.resolve(jsonResponse({ message: 'Linked' }));
        }
        if (
          pathname.startsWith('/api/v1/audiobooks/') &&
          pathname.endsWith('/versions') &&
          method === 'DELETE'
        ) {
          const parts = pathname.split('/').filter(Boolean);
          const bookId = parts[parts.length - 2];
          const payload = parseJsonBody(init) || {};
          const otherId = payload.other_id;
          const groupId = getVersionGroupId(bookId);
          libraryBooks = libraryBooks.map((book) =>
            book.id === otherId
              ? { ...book, version_group_id: '', is_primary_version: false }
              : book
          );
          if (groupId) {
            const remaining = libraryBooks.filter(
              (book) => book.version_group_id === groupId
            );
            if (remaining.length <= 1) {
              libraryBooks = libraryBooks.map((book) =>
                book.version_group_id === groupId
                  ? {
                      ...book,
                      version_group_id: '',
                      is_primary_version: false,
                    }
                  : book
              );
            }
          }
          return Promise.resolve(jsonResponse({ message: 'Unlinked' }));
        }
        if (
          pathname.startsWith('/api/v1/audiobooks/') &&
          pathname.endsWith('/set-primary') &&
          method === 'PUT'
        ) {
          const parts = pathname.split('/').filter(Boolean);
          const bookId = parts[parts.length - 2];
          const groupId = ensureVersionGroup(bookId);
          libraryBooks = libraryBooks.map((book) =>
            book.version_group_id === groupId
              ? { ...book, is_primary_version: book.id === bookId }
              : book
          );
          return Promise.resolve(jsonResponse({ message: 'Primary set' }));
        }

        if (
          pathname.startsWith('/api/v1/audiobooks/') &&
          method === 'DELETE'
        ) {
          const parts = pathname.split('/').filter(Boolean);
          const bookId = parts[parts.length - 1];
          libraryBooks = libraryBooks.map((book) =>
            book.id === bookId
              ? { ...book, marked_for_deletion: true }
              : book
          );
          return Promise.resolve(jsonResponse({ message: 'Deleted' }));
        }

        if (
          pathname.startsWith('/api/v1/audiobooks/') &&
          method === 'PUT'
        ) {
          const parts = pathname.split('/').filter(Boolean);
          const bookId = parts[parts.length - 1];
          const payload = parseJsonBody(init) || {};
          const target = libraryBooks.find((book) => book.id === bookId);
          if (target?.force_update_required && !payload.force_update) {
            return Promise.resolve(
              jsonResponse({ error: 'Conflict' }, 409)
            );
          }
          libraryBooks = libraryBooks.map((book) =>
            book.id === bookId ? { ...book, ...payload } : book
          );
          const updated = libraryBooks.find((book) => book.id === bookId);
          return Promise.resolve(jsonResponse(updated || payload));
        }

        if (
          pathname.startsWith('/api/v1/audiobooks/') &&
          pathname.endsWith('/restore') &&
          method === 'POST'
        ) {
          const parts = pathname.split('/').filter(Boolean);
          const bookId = parts[parts.length - 2];
          libraryBooks = libraryBooks.map((book) =>
            book.id === bookId
              ? { ...book, marked_for_deletion: false }
              : book
          );
          return Promise.resolve(jsonResponse({ message: 'Restored' }));
        }

        if (
          pathname.startsWith('/api/v1/audiobooks/') &&
          pathname.endsWith('/fetch-metadata') &&
          method === 'POST'
        ) {
          const parts = pathname.split('/').filter(Boolean);
          const bookId = parts[parts.length - 2];
          const target = libraryBooks.find((book) => book.id === bookId);
          const shouldFail =
            target &&
            (target as { fetch_metadata_error?: boolean }).fetch_metadata_error;
          const delayMs =
            (target as { fetch_metadata_delay_ms?: number })
              ?.fetch_metadata_delay_ms || 0;

          if (shouldFail) {
            const failure = () =>
              jsonResponse({ error: 'Metadata fetch failed' }, 500);
            if (delayMs > 0) {
              return new Promise((resolve) =>
                window.setTimeout(() => resolve(failure()), delayMs)
              );
            }
            return Promise.resolve(failure());
          }
          libraryBooks = libraryBooks.map((book) =>
            book.id === bookId
              ? {
                  ...book,
                  language: book.language || 'en',
                  publisher: book.publisher || 'Test Publisher',
                  description: book.description || 'Updated metadata',
                }
              : book
          );
          const updated = libraryBooks.find((book) => book.id === bookId);
          const success = () =>
            jsonResponse({
              message: 'Metadata fetched',
              book: updated,
              source: 'mock',
            });
          if (delayMs > 0) {
            return new Promise((resolve) =>
              window.setTimeout(() => resolve(success()), delayMs)
            );
          }
          return Promise.resolve(success());
        }

        // Fallback
        return originalFetch(input, init);
      };
    },
    {
      bookData,
      configData,
      importPathsData,
      backupsData,
      blockedHashesData,
      filesystemData,
      homeDirectoryData,
      operationsData,
      itunesData,
      failuresData,
      systemStatusData,
    }
  );
}

/**
 * Setup library page with mock books
 */
export async function setupLibraryWithBooks(
  page: Page,
  books: ReturnType<typeof generateTestBooks>,
  options: Omit<MockApiOptions, 'books'> = {}
) {
  await setupMockApi(page, { ...options, books });
}
