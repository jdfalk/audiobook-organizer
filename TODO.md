- [x] ✅ **Backend**: Database migration system with version tracking
- [x] ✅ **Backend**: Complete audiobook CRUD API (create, read, update, delete, batch)
- [x] ✅ **Backend**: Authors and series API endpoints
- [x] ✅ **Backend**: Library folder management API
- [x] ✅ **Backend**: Operation tracking and logs API
- [x] ✅ **Backend**: HTTP server with configurable timeouts
- [x] ✅ **Backend**: Safe file operations wrapper (copy-first, checksums, rollback)
- [x] ✅ **Backend**: File system browsing API (.jabexclude, stats, permissions)
- [x] ✅ **Backend**: Async operation queue system with priority handling
- [x] ✅ **Backend**: WebSocket/SSE for real-time operation updates
- [x] ✅ **Backend**: Database backup and restore functionality
- [x] ✅ **Backend**: Enhanced metadata API (batch updates, history, validation)

- [x] ✅ **Frontend**: React app setup with TypeScript and Material-UI
- [x] ✅ **Frontend**: Dashboard with library statistics and navigation
- [x] ✅ **Frontend**: Library page with import path management
- [x] ✅ **Frontend**: System page with Logs, Storage, Quotas, System Info tabs
- [x] ✅ **Frontend**: Settings page with comprehensive configuration options
- [x] ✅ **Frontend**: Smart folder/file naming patterns with live examples

- [x] ✅ **Backend**: Media info extraction from audio files
  - ✅ Created mediainfo package with Extract() function
  - ✅ Supports MP3, M4A/M4B, FLAC, OGG formats
  - ✅ Extracts bitrate, codec, sample rate, channels, bit depth
  - ✅ Quality string generation and tier comparison
- [x] ✅ **Backend**: Version management API (link versions, set primary, manage version groups)
  - ✅ Added GetBooksByVersionGroup() to Store interface and both implementations
  - ✅ Implemented 4 API endpoints: list versions, link versions, set primary, get version group
  - ✅ Uses ULID-based version group IDs for grouping multiple versions
  - ✅ All handlers properly use database.GlobalStore
- [x] ✅ **Backend**: Import paths CRUD API (list, add, remove, scan)
  - ✅ GET /api/v1/library/folders - List all library folders/import paths
  - ✅ POST /api/v1/library/folders - Add new import path
  - ✅ DELETE /api/v1/library/folders/:id - Remove import path
  - ✅ POST /api/v1/operations/scan - Trigger scan (optionally for specific folder)
- [x] ✅ **Backend**: System info API (storage, quotas, system stats)
  - ✅ GET /api/v1/system/status - Comprehensive system status (library stats, memory, runtime, operations)
  - ✅ Includes book count, folder count, total storage size
  - ✅ Memory statistics (alloc, total_alloc, sys, num_gc)
  - ✅ Runtime information (Go version, goroutines, CPU count)
- [x] ✅ **Backend**: Logs API with filtering (level, source, search, pagination)
  - ✅ GET /api/v1/system/logs - System-wide logs with filtering
  - ✅ Supports filtering by level (info, warn, error)
  - ✅ Full-text search in messages and details
  - ✅ Pagination with limit/offset parameters
  - ✅ Aggregates logs from all recent operations
- [x] ✅ **Backend**: Settings API (save/load configuration)
  - ✅ GET /api/v1/config - Get current configuration
  - ✅ PUT /api/v1/config - Update configuration at runtime
  - ✅ Supports updating root_dir, database_path, playlist_dir, API keys
  - ✅ Safety restrictions on database_type and enable_sqlite (read-only at runtime)
- [x] **Backend - Database migration for media info and version fields**
  - ✅ Created migration005 adding all 9 fields to books table
  - ✅ Handles duplicate column detection gracefully
  - ✅ Creates indices for version_group_id and is_primary_version
- [x] ✅ **Backend**: Manual file import handling
  - ✅ POST /api/v1/import/file - Import single audio file with metadata extraction
  - ✅ File validation (existence, extension support)
  - ✅ Automatic metadata extraction (title, author, narrator, etc.)
  - ✅ Media info extraction (bitrate, codec, quality)
  - ✅ Author auto-creation if not exists
  - ✅ Optional organize flag to trigger file organization
- [x] ✅ **Backend**: Metadata source integration (Open Library)
  - ✅ Created OpenLibraryClient with SearchByTitle, SearchByTitleAndAuthor, GetBookByISBN methods
  - ✅ Returns title, author, description, publisher, publish_year, ISBN, cover_url, language
  - ✅ API endpoints: GET /api/v1/metadata/search, POST /api/v1/audiobooks/:id/fetch-metadata
  - ✅ 8 comprehensive test cases created (client init, search operations, error handling)

- [x] ✅ **Frontend**: Connect all pages to backend APIs
  - ✅ Created comprehensive API service layer (src/services/api.ts) with 30+ typed endpoints
  - ✅ Dashboard: Real statistics from /api/v1/audiobooks, /api/v1/authors, /api/v1/series, /api/v1/system/status
  - ✅ Library page: Real audiobook listing, search, import path management, scan operations
  - ✅ System page: Real logs with filtering, system status, memory/CPU stats, SystemInfoTab displays real-time data
  - ✅ Settings page: Loads configuration on mount with api.getConfig(), saves with api.updateConfig()
  - ✅ All API endpoints integrated with proper error handling
  - ✅ Backend Config struct expanded to support all frontend settings (organization, quotas, metadata, performance, memory, logging)
- [x] ✅ **Frontend**: Version management UI components
  - ✅ VersionManagement dialog component with version comparison view
  - ✅ Quality indicators (codec, bitrate, sample rate display)
  - ✅ Primary version selection with star icon
  - ✅ Link version dialog for connecting multiple editions
  - ✅ Version indicator chips on audiobook cards
  - ✅ Integrated into Library page grid view
  - ✅ Uses all version management API endpoints (getBookVersions, linkBookVersion, setPrimaryVersion)
- [x] ✅ **Frontend**: Library browser with grid/list views and sorting
  - ✅ Grid view fully functional with AudiobookCard and AudiobookGrid components
  - ✅ Sorting dropdown with options: title, author, date added, date modified
  - ✅ Client-side sort implementation in Library.tsx with localeCompare for strings
  - ✅ Date sorting (descending - newest first) for created_at and updated_at fields
- [x] ✅ **Frontend**: Metadata editor with inline editing
  - ✅ MetadataEditDialog component with comprehensive edit form
  - ✅ InlineEditField component created for quick inline edits
  - ✅ "Fetch Metadata" button with CloudDownload icon in AudiobookCard menu
  - ✅ Full integration in Library.tsx with handleFetchMetadata function

- [x] ✅ **General**: Configure GitHub workflows
  - ✅ Comprehensive CI workflow v1.18.1 already exists
  - ✅ Backend tests: Go 1.24, test execution, race detection, coverage
  - ✅ Frontend tests: Node 22, npm ci, build, test
  - ✅ Security scanning: gosec, npm audit, Trivy
  - ✅ Python script validation: Python 3.13, pip, script checks
- [x] ✅ **Testing**: Unit and integration test framework
  - ✅ Created internal/metadata/openlibrary_test.go (8 test cases)
  - ✅ Created internal/database/sqlite_test.go (11 test cases)
  - ✅ Tests cover client initialization, search operations, CRUD, version management, author operations
  - ✅ Uses setupTestDB pattern with temporary database and cleanup
  - ✅ Network tests use t.Skip for rate limits
- [x] ✅ **Docs**: OpenAPI/Swagger documentation
  - ✅ Created docs/openapi.yaml with complete OpenAPI 3.0.3 specification
  - ✅ Documented 20+ endpoints across 9 tags (Audiobooks, Authors, Series, Library, Operations, Metadata, Versions, System, Backup)
  - ✅ Full schema definitions for Book (25+ fields), Author, Series, LibraryFolder, MetadataResult, SystemStatus, Config
  - ✅ Request/response examples with proper types, error codes, ULID format specifications

- [ ] 🟡 **General**: Implement library organization with hard links, reflinks,
      or copies (auto mode tries reflink → hardlink → copy)

## 🚨 CRITICAL FIXES - HIGH PRIORITY

### System Page Issues

- [x] **Fix memory display** - Changed label to "App Memory System" to clarify
      that displayed memory is Go runtime memory, not system RAM. ✅ COMPLETED
- [ ] **Fix logs not displaying** - Logs tab shows no data because no operations
      have been run yet. Logs are fetched from operation records. Will populate
      after running scan operations.

### Settings Page Issues

- [x] **Fix scrolling in Settings page** - Removed maxHeight constraint from
      Paper component, proper Box structure for scrollable tabs. ✅ COMPLETED
- [x] **Fix library path browser** - ServerFileBrowser properly initialized and
      working. ✅ COMPLETED

### ServerFileBrowser Component Issues

- [x] **Make current path sticky** - Added position: sticky, top: 0, zIndex: 10
      to path bar. ✅ COMPLETED
- [x] **Fix Add Folder button always disabled** - Added "Select This Folder"
      button for immediate selection without double-click. Single click navigates,
      button selects. ✅ COMPLETED
- [x] **Add manual path editing** - Added edit icon that enables TextField for
      direct path editing with save/cancel functionality. ✅ COMPLETED

### Import Path Functionality Issues

- [x] **Fix folder scanning doesn't traverse subdirectories** - Implemented real
      scanner.ScanDirectory() call in startScan handler. Uses filepath.Walk for
      recursive traversal. Updates book_count in LibraryFolder records. ✅ COMPLETED
- [x] **Auto-scan on import path add** - Modified addLibraryFolder handler to
      automatically trigger async scan operation. Returns scan_operation_id for
      progress tracking. ✅ COMPLETED
- [x] **Fix import path terminology** - Updated all UI: Dashboard, Settings,
      Library pages now consistently use "Import Folders (Watch Locations)" vs
      "Library Path". ✅ COMPLETED

### Dashboard Navigation Issues

- [x] **Fix Library Folders link** - Fixed navigation from /file-manager to
      /library. Card title updated to "Import Folders". ✅ COMPLETED

## Current Sprint Tasks

### Frontend UI Improvements

- [ ] **Fix System Page Fake Data**
  - [ ] Wire StorageTab.tsx to real API data from /api/v1/system/status and /api/v1/library/folders
  - [ ] Wire QuotaTab.tsx to real API data or remove if quotas not implemented
  - [ ] Wire LogsTab.tsx to /api/v1/system/logs endpoint to show actual application logs
  - [ ] Wire SystemInfoTab.tsx to /api/v1/system/status to show real OS (linux), memory stats, CPU count, Go version

### Library & Import Management

- [ ] **Add Library Path Configuration**
  - [ ] Add central library path setting in Settings page (where organized audiobooks go)
  - [ ] Add UI in Settings to manage download/import folders with server filesystem browser
  - [ ] Add UI in Library tab to add/remove download folders with server filesystem browser

- [ ] **Server Filesystem Browser**
  - [ ] Create reusable ServerFileBrowser component using /api/v1/filesystem/browse
  - [ ] Update Library page import workflow - replace local file upload with server browser
  - [ ] Allow selecting files and folders from remote server filesystem

### First-Run Experience

- [ ] **Welcome Wizard**
  - [ ] Create WelcomeWizard component that runs on first launch
  - [ ] Step 1: Set library folder path (where organized books go)
  - [ ] Step 2: Optional OpenAI API key setup with connection test
  - [ ] Step 3: Add import/download folder paths using server browser
  - [ ] Store completion flag in config/database to skip wizard on subsequent launches

### Testing

- [ ] Create database_test.go - test initialization, configuration, database type selection
- [ ] Create migrations_test.go - test schema versioning, migration execution, rollback
- [ ] Create store_test.go - test interface methods and common store functionality
- [ ] Create web_test.go - test HTTP handlers and API endpoints

## Future Improvements

### Multi-User & Security

- [ ] **Multi-User Interface**
  - [ ] User profiles and authentication system
  - [ ] Per-user playback tracking and statistics
  - [ ] User-specific library views and permissions
  - [ ] Role-based access control (admin, user, read-only)

- [ ] **SSL/TLS Support**
  - [ ] HTTPS support with certificate management
  - [ ] Let's Encrypt integration for automatic certificates
  - [ ] Self-signed certificate generation for local deployments
  - [ ] Configurable cipher suites and TLS versions

### BitTorrent Client Integration

- [ ] **Torrent Client Interoperability**
  - [ ] uTorrent/µTorrent API integration
  - [ ] Deluge RPC integration
  - [ ] qBittorrent Web API integration
  - [ ] Automatic torrent removal after successful library organization
  - [ ] Support for preserving seeding after organization:
    - [ ] Create shadow/mirror directory structure outside main library (e.g., `/audiobooks-seeding/`)
    - [ ] Maintain hard links in shadow directory matching original torrent structure
    - [ ] Update torrent client to serve from shadow directory after files are organized
    - [ ] Handle cross-filesystem scenarios (copy to shadow dir when hard links impossible)
    - [ ] Detect and handle metadata updates that modify organized files (break hard links)
    - [ ] Optional: Re-link shadow files if organized files haven't been modified
  - [ ] Configurable removal policies (remove after move, keep seeding, etc.)

### iTunes Library Integration

- [ ] **iTunes Interoperability**
  - [ ] Read iTunes library XML/database for playback statistics
  - [ ] Import play count, last played date, and user ratings from iTunes
  - [ ] Sync playback progress and bookmarks between systems
  - [ ] Write metadata updates back to iTunes library
  - [ ] Bidirectional sync for play counts and ratings
  - [ ] Support for multiple iTunes libraries (multi-user scenarios)

### Web Download & Export

- [ ] **Direct Book Download from Web Interface**
  - [ ] Download individual audiobook files via web UI
  - [ ] Automatic ZIP creation for multi-file books
  - [ ] Progress indicators for ZIP creation and download
  - [ ] Configurable download formats (original files, ZIP, M4B)
  - [ ] Batch download support for multiple books
  - [ ] Resume support for interrupted downloads

### Audio Transcoding & Optimization

- [ ] **Automated Audio Transcoding**
  - [ ] MP3 to M4B conversion for multi-file books
  - [ ] Chapter metadata preservation during transcoding
  - [ ] Automatic chapter detection from file names/structure
  - [ ] Cover art embedding in M4B files
  - [ ] Configurable quality settings (bitrate, codec, sample rate)
  - [ ] Batch transcoding operations with priority queue
  - [ ] Original file preservation options (keep, replace, archive)
  - [ ] Integration with book download (serve M4B instead of ZIP for transcoded books)

## Recently Added Observability Tasks

- [ ] Persist operation logs (retain historical tail per operation; add `/api/v1/operations/:id/logs?tail=` and system-wide retention)
- [ ] Improve log view UX (auto-scroll when following tail, level-based coloring, collapsible verbose details, memory usage guard)
- [ ] SSE system status heartbeats (push `system.status` diff events every 5s for live memory / library metrics without polling)

## Extended Improvement Backlog

### Observability & Monitoring

- [ ] Structured application metrics endpoint (Prometheus `/metrics`, operation duration histograms, scan/organize counters)
- [ ] Per-operation timing summary stored after completion (wall time, file count, throughput)
- [ ] Slow operation detector (warn if scan > configurable threshold)
- [ ] Library growth trend stats (daily book count snapshot table)
- [ ] File integrity checker (periodic checksum verification with mismatch surfacing)
- [ ] Background health check SSE pings (report DB latency classification)
- [ ] Error aggregation dashboard (top recurring errors with counts)

### Performance

- [ ] Parallel scanning (goroutine pool respecting `concurrent_scans` setting)
- [ ] Debounced library size recomputation using inotify / fsnotify events instead of periodic full walk
- [ ] Caching layer for frequent book queries (LRU keyed by filter + page)
- [ ] Batch metadata fetch pipeline (queue & coalesce external API calls)
- [ ] Adaptive operation worker scaling (increase workers under backlog, shrink when idle)
- [ ] Memory pressure monitor triggering GC hints / cache trimming

### Reliability & Resilience

- [ ] Graceful resume of interrupted scan (persist walker state checkpoints)
- [ ] Operation retry policy for transient failures (network metadata retrieval)
- [ ] Circuit breaker for external metadata sources (avoid cascading failures)
- [ ] Transactional organize rollback journal (record actions, allow revert)
- [ ] Startup self-diagnostic (verify paths writable, database schema current, config sanity)

### UX / Frontend

- [ ] Global notification/toast system for successes & errors
- [ ] Dark mode / theme customization with persisted preference
- [ ] Keyboard shortcuts (e.g. '/' focus search, 'o' organize, 's' scan all)
- [ ] Advanced filters (bitrate range, codec, quality tier, duration bucket)
- [ ] Progressive loading skeletons for long lists
- [ ] Inline author/series quick create dialog from edit form
- [ ] Book detail modal with expanded metadata & version timeline
- [ ] Accessible tab navigation (ARIA roles, focus management)
- [ ] Mobile responsive layout improvements (grid collapse, drawer nav)
- [ ] Virtualized audiobook list for large collections

### API Enhancements

- [ ] PATCH support for partial audiobook updates
- [ ] Bulk import endpoint for multiple file paths in one request
- [ ] Webhook system for external integrations (scan complete, organize complete)
- [ ] Rate limiting (token bucket) for expensive endpoints
- [ ] ETag / caching headers for read-only endpoints
- [ ] API key auth layer (for third-party consumers)

### Security

- [ ] Audit log (who changed config, when, old vs new values)
- [ ] Optional JWT auth for multi-user future
- [ ] Secret scanning in config updates (reject accidental API key leakage)
- [ ] Harden path traversal defenses in filesystem browse
- [ ] TLS termination guide / built-in ACME client

### Database & Data Quality

- [ ] Deduplication job (identify same book with different filenames via fuzzy match)
- [ ] Orphan file detector (files on disk not represented in DB)
- [ ] Full-text search index (author/title/narrator) for advanced queries
- [ ] Incremental migration harness with dry-run mode
- [ ] Archival strategy (move old logs & completed operations to cold storage)

### Operation Queue Improvements

- [ ] Priority aging (long-waiting normal ops get temporary priority boost)
- [ ] Operation dependency graph (organize waits for scan completion for same folder)
- [ ] Pause / resume queue functionality
- [ ] Real-time worker utilization stats
- [ ] Rate-controlled progress events (coalesce rapid updates)

### Real-Time & Streaming

- [ ] Upgrade SSE hub to optional WebSocket mode for bidirectional cancel/resubscribe
- [ ] Client subscription refinement (subscribe to multiple ops, filter types)
- [ ] Replay last N events on connect for quick hydration

### Frontend Components (New)

- [ ] Timeline visualization for operations
- [ ] Quality comparison chart between versions
- [ ] Folder tree viewer for import paths with status badges
- [ ] Log tail component standalone (filter by level, search live)

### Testing & QA

- [ ] Load test scenarios (large folder scan, 10k files)
- [ ] Fuzz tests for filename parser / AI parse fallback
- [ ] Frontend component snapshot tests
- [ ] End-to-end test harness (Playwright or Cypress) for critical flows
- [ ] Chaos test for operation cancellation mid-scan

### DevOps / CI/CD

- [ ] Automated release notes generation from conventional commits
- [ ] Build artifact publishing (binary + Docker image)
- [ ] Nightly vulnerability scan & report
- [ ] Performance regression benchmarks (scan speed comparison per commit)

### Documentation

- [ ] Developer guide (architecture overview, data flow diagrams)
- [ ] Operations handbook (recover from failed organize, manual rollback)
- [ ] REST API quickstart examples (curl / client code snippets)
- [ ] Advanced configuration examples (quota strategies, memory tuning)

### Integration / Ecosystem

- [ ] Calibre metadata export integration
- [ ] OPDS feed generation for external audiobook apps
- [ ] Plex / Jellyfin library sync stub
- [ ] External cover art provider fallback chain

### AI & Metadata Enhancements

- [ ] Confidence explanation tooltips for AI parsing results
- [ ] Batch AI parse queue for newly imported unparsed files
- [ ] Metadata merge policy editor (prefer source A unless missing field)
- [ ] Automatic language detection from text samples

### Internationalization (i18n)

- [ ] Extract UI strings into translation files
- [ ] Language switcher in settings
- [ ] Date/time localization and number formatting

### Accessibility (a11y)

- [ ] Screen reader labels for interactive elements
- [ ] High contrast theme option
- [ ] Focus outline consistency and skip-to-content link

### Mobile / PWA

- [ ] PWA manifest & offline shell
- [ ] Add to Home Screen guidance
- [ ] Basic offline read-only browsing of cached metadata

### Packaging & Deployment

- [ ] Docker multi-arch build pipeline (linux/amd64 + arm64)
- [ ] Helm chart for Kubernetes deployment
- [ ] Binary distribution script with checksums & SBOM

### Backup & Restore Enhancements

- [ ] Incremental backups (changes since last snapshot)
- [ ] Backup integrity verification (hash manifest)
- [ ] Scheduled backup task with retention policy

### File Handling Improvements

- [ ] Concurrent organize operations with folder-level locking
- [ ] Metadata tag writing improvements (add narrator, series sequence tags)
- [ ] Chapter file merging strategy (combine small segments automatically)

### User Features (Future Multi-User)

- [ ] Per-user favorites / starred books
- [ ] Listening progress tracking (position syncing)
- [ ] Personal notes / annotations per book

### Data Analysis / Insights

- [ ] Quality upgrade suggestions (identify low bitrate books with higher quality versions available)
- [ ] Duplicate version ranking (present best candidate to keep)
- [ ] Usage analytics (most scanned folders, peak operation times)

### Housekeeping / Maintenance

- [ ] Stale operation cleanup job (remove abandoned queued ops after timeout)
- [ ] Automatic log rotation & compression
- [ ] Config schema validation on update (reject invalid enum values)

### Security Hardening

- [ ] Content Security Policy headers for frontend
- [ ] Rate limit brute-force attempts (future auth system)
- [ ] Dependency vulnerability auto-PR updates

### Miscellaneous Ideas

- [ ] Embedded help panel with contextual docs
- [ ] CLI progress mirroring (serve mode exposes op summary to CLI)
- [ ] Export organized library manifest (JSON + checksums)
- [ ] Plugin system scaffold (register metadata providers / transcoding strategies)
