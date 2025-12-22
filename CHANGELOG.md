# Changelog

## [Unreleased]

### Added / Changed

#### December 22, 2025 - MVP Implementation Sprint

- **All Tests Passing**: Fixed all failing Go tests across server and scanner packages
  - Fixed scanner panic with nil database check
  - Fixed test bug in TestIntegrationLargeScaleMixedFormats (string conversion)
  - 19 packages tested, all passing
  
- **Dashboard Analytics API**: New `/api/v1/dashboard` endpoint
  - Size distribution with 4 buckets (0-100MB, 100-500MB, 500MB-1GB, 1GB+)
  - Format distribution tracking (m4b, mp3, m4a, flac, etc.)
  - Total size calculation
  - Recent operations summary
  
- **Metadata Management API**: Comprehensive metadata field validation
  - `/api/v1/metadata/fields` - Lists all fields with validation rules
  - publishDate validation with YYYY-MM-DD format checking
  - Field types, required flags, patterns, and custom validators
  
- **Work Queue API**: Edition and work grouping
  - `/api/v1/work` - List all work items with associated books
  - `/api/v1/work/stats` - Statistics (total works, books, editions)
  
- **Blocked Hashes Management**: Hash blocklist for preventing reimports
  - `GET /api/v1/blocked-hashes` - List all blocked hashes with reasons
  - `POST /api/v1/blocked-hashes` - Add hash to blocklist
  - `DELETE /api/v1/blocked-hashes/:hash` - Remove from blocklist
  - SHA256 hash validation
  
- **State Machine Implementation**: Book lifecycle tracking (Migration 9)
  - `library_state` field - Track book status (imported/organized/deleted)
  - `quantity` field - Reference counting
  - `marked_for_deletion` field - Soft delete flag
  - `marked_for_deletion_at` timestamp
  - Indices for efficient state and deletion queries
  
- **Documentation**: Comprehensive session reports
  - MVP_IMPLEMENTATION_STATUS.md - Detailed task tracking
  - SESSION_SUMMARY.md - Session accomplishments
  - FINAL_REPORT.md - Complete progress report with metrics

#### Latest Changes (Metadata, UI Enhancements, Testing, Documentation, Release Workflow Integration)

- **Release Workflow Integration**: Full integration with pinned composite
  actions for cross-platform builds

  - Go builds: GoReleaser-managed releases and publishes
  - Python packages: Build-only mode with artifact staging
  - Rust crates: Optimized release builds with test suite
  - Frontend: Node.js optimization with production builds
  - Docker images: Multi-platform container builds to GitHub Container Registry
  - All artifacts coordinated through reusable-release orchestrator
  - GitHub Packages integration for artifact storage and distribution

- **Metadata Integration**: Open Library API integration for external metadata
  fetching
  - Created OpenLibraryClient with search and ISBN lookup capabilities
  - API endpoints: `GET /api/v1/metadata/search`,
    `POST /api/v1/audiobooks/:id/fetch-metadata`
  - Frontend: "Fetch Metadata" button in audiobook card menu with CloudDownload
    icon
  - Returns title, author, description, publisher, publish year, ISBN, cover
    URL, language
- **Library UI Enhancements**: Sorting functionality for audiobooks
  - Sorting dropdown with options: title, author, date added, date modified
  - Client-side sorting with localeCompare for strings, timestamp comparison for
    dates
  - Date sorting displays newest first (descending order)
- **Inline Editing**: Reusable InlineEditField component
  - Edit/display modes with TextField integration
  - Save/cancel buttons with keyboard shortcuts (Enter to save, Escape to
    cancel)
  - Support for single-line and multiline editing
- **Testing Framework**: Comprehensive test suite created
  - 8 metadata tests: client initialization, search operations, ISBN lookup,
    error handling
  - 11 database tests: CRUD operations, version management, author operations,
    pagination, counting
  - Uses setupTestDB pattern with temporary databases and cleanup
  - Network tests use t.Skip for rate limit protection
- **API Documentation**: Complete OpenAPI 3.0.3 specification
  (docs/openapi.yaml)
  - Documented 20+ endpoints across 9 categories
  - Full schema definitions for all models (Book with 25+ fields, Author,
    Series, etc.)
  - Request/response examples with proper types and error codes

#### Previous Changes

- Extended Book metadata fields: work_id, narrator, edition, language,
  publisher, isbn10, isbn13 (with SQLite migration & CRUD support)
- API tests for extended metadata (round‑trip + update semantics)
- Hardened audiobook update handler error checking (nil-safe not found handling)
- Metadata extraction scaffolding for future multi-format support (tag reader
  integration prep)
- Work entity: basic model, SQLite schema, Pebble+SQLite store methods, and REST
  API endpoints (list/get/create/update/delete, list books by work)
- **Frontend**: Complete web interface with React + TypeScript + Material-UI
  - Dashboard with library statistics
  - Library page with import path management and manual import
  - Works page for audiobook organization
  - System page with tabs: Logs (real-time filtering), Storage breakdown, Quota
    management, System info
  - Settings page with comprehensive configuration (library paths, metadata
    sources, quotas, memory, logging)
- Media info and version management system:
  - Media quality fields: bitrate (kbps), codec (AAC/MP3/FLAC), sample rate,
    channels, bit depth
  - Human-readable quality strings (e.g., "320kbps AAC", "FLAC Lossless")
  - Version management: link multiple versions of same audiobook, mark primary
    version
  - Version notes for describing differences (e.g., "Remastered 2020",
    "Unabridged")
  - Organized in "Additional Versions" subfolder structure
  - Pattern fields support media info: `{bitrate}`, `{codec}`, `{quality}`
- Database migration (v5) adding media info and version management fields to
  SQLite books table
  - Automatically detects and handles duplicate columns
  - Creates indices for version_group_id and is_primary_version for query
    performance
- Media info extraction package for audio file metadata parsing
  - Supports MP3, M4A/M4B (AAC), FLAC, and OGG Vorbis formats
  - Extracts bitrate, codec, sample rate, channels, and bit depth
  - Generates human-readable quality strings (e.g., "320kbps MP3", "FLAC
    Lossless (16-bit/44.1kHz)")
  - Quality tier system for comparing audio versions (0-100 scale)
- Version management API endpoints implemented
  - `GET /api/v1/audiobooks/:id/versions` - List all versions of an audiobook
  - `POST /api/v1/audiobooks/:id/versions` - Link two audiobooks as versions
    (creates/uses version_group_id)
  - `PUT /api/v1/audiobooks/:id/set-primary` - Set an audiobook as the primary
    version in its group
  - `GET /api/v1/version-groups/:id` - Get all audiobooks in a version group
  - GetBooksByVersionGroup() method added to Store interface with SQLite and
    PebbleDB implementations
- System information and monitoring APIs
  - `GET /api/v1/system/status` - Comprehensive system status with library
    stats, memory usage, runtime info, recent operations
  - `GET /api/v1/system/logs` - System-wide logs with filtering by level,
    search, and pagination
  - `GET /api/v1/config` - Get current configuration
  - `PUT /api/v1/config` - Update configuration at runtime (with safety
    restrictions on critical settings)
- Manual file import endpoint
  - `POST /api/v1/import/file` - Import single audio file with automatic
    metadata and media info extraction
  - File validation, author auto-creation, optional file organization
- **Frontend API Integration**: Complete connection to backend services
  - Created comprehensive API service layer (src/services/api.ts) with typed
    functions for 30+ endpoints
  - Dashboard: Real-time statistics from multiple endpoints (books, authors,
    series, system status)
  - Library page: Live audiobook data with search, import path CRUD, scan
    operations
  - System page: Complete integration with real logs (filtering), system metrics
    (memory/CPU/runtime), operation monitoring
  - Settings page: Full configuration management with backend persistence
  - All pages now use real backend APIs with comprehensive error handling and
    type safety
- **Expanded Backend Configuration**: Config struct now supports complete
  frontend settings
  - Library organization: strategy (auto/copy/hardlink/reflink), folder/file
    naming patterns, backups
  - Storage quotas: disk quota limits, per-user quotas
  - Metadata sources: configurable providers (Audible, Goodreads, Open Library,
    Google Books) with credentials
  - Performance: concurrent scan control
  - Memory management: cache size, memory limits (items/percent/absolute)
  - Logging: level, format (text/json), structured logging options
  - All settings persist to configuration file and sync between frontend/backend
- **Version Management UI**: Complete interface for managing multiple audiobook
  versions
  - VersionManagement dialog component displaying all linked versions with
    quality comparison
  - Quality indicators showing codec (MP3/AAC/FLAC), bitrate, sample rate for
    each version
  - Primary version selection with visual star indicator
  - Link version dialog for connecting different editions/qualities of same
    audiobook
  - Version indicator chips on audiobook cards ("Multiple Versions" badge)
  - Integrated into Library page with menu item and handlers
  - Full CRUD support using version management API endpoints
- **Smart Path Handling**: Empty fields (like {series}) automatically removed
  from folder paths (no duplicate slashes)
- **Naming Pattern Examples**: Live preview with both series and non-series
  books (Nancy Drew + To Kill a Mockingbird)

### Upcoming

- Audio tag reading for MP3 (ID3v2), M4B/M4A (iTunes atoms), FLAC/OGG (Vorbis
  comments), AAC
- Safe in-place metadata writing with backup/rollback
- Work entity (model + CRUD + association to Book via `work_id`)
- Manual endpoint regression run post ULID + metadata changes
- Git LFS sample audiobook fixtures for integration tests
  - POST `/api/filesystem/exclude` - Create .jabexclude files
