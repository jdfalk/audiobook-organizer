# Audiobook Organizer

A comprehensive audiobook management application with a modern web interface and
powerful Go backend for organizing, managing, and enjoying your audiobook
collection.

## 📚 Glossary of Terms

To avoid confusion, this application uses specific terminology:

- **Library** / **Library Folder** / **Library Path**: The main root directory (`root_dir`) and its organized subdirectories where your audiobooks are permanently stored in a structured format. This is your primary collection.

- **Import Path** / **Import Folder** / **Monitored Folder**: External directories that are scanned for new audiobook files but are NOT part of your organized library. These are temporary staging locations (like Downloads folders) where the app looks for new content to import into the library.

- **Scan Operation**: The process of discovering audiobook files in both the library and import paths, extracting metadata, and updating the database.

- **Organize Operation**: Moving or copying audiobooks from import paths into the library with proper folder structure and naming.

**Important**: When you see "library folders" in the API or UI, it refers to *import paths* only (for historical reasons). The actual library path is configured separately as `root_dir`.

## 🎯 MVP Development in Progress

**Current Status**: Backend foundation complete (~45% of MVP), React frontend
scaffolded

See our detailed planning documents:

- **[MVP Specification](docs/mvp-specification.md)**: Complete feature
  requirements and architecture
- **[Implementation Plan](docs/mvp-implementation-plan.md)**: 14-week
  development roadmap
- **[Progress Analysis](docs/current-progress-analysis.md)**: Current state
  assessment
- **[Executive Summary](docs/mvp-summary.md)**: High-level project overview

## 🌟 MVP Features

### ✅ Completed Backend (Go 1.25)

- **REST API**: Complete HTTP endpoints including:
  - Audiobooks CRUD (list, get, update, delete, batch operations)
  - Authors and Series listing
  - Library folder management (add, list, remove)
  - Operation tracking (create, status, cancel, logs)
  - Health check with database metrics
- **Database Migration System**: Version tracking with sequential migrations
- **PebbleDB Store**: Extended keyspace with users, sessions, playback tracking
- **Configurable Server**: Timeout settings, CORS, graceful shutdown
- **Comprehensive Format Support**: MP3, M4A, M4B, AAC, FLAC, OGG, WMA

### ✅ Additional Backend Features

- **Safe File Operations**: Copy-first with SHA256 verification, automatic
  backups, rollback support
- **File System API**: Directory browsing, .jabexclude management, disk space
  checking
- **Real-time Updates**: Server-Sent Events (SSE) for operation progress,
  status, and logs
- **Async Operation Queue**: Priority-based background processing with
  configurable workers
- **Event Broadcasting**: Automatic client notification for operation updates
- **Database Backup/Restore**: Compressed backups with checksums, automatic
  cleanup, restore verification

### ✅ Additional Backend Features (cont'd)

- **Enhanced Metadata API**: Batch updates with validation, export/import,
  comprehensive validation rules

### 🚧 In Progress (Phase 3: React Frontend Foundation)

- **Project Structure**: ✅ Vite + TypeScript + React 18
- **Material-UI v5**: ✅ Theme configuration and component library
- **Layout & Navigation**: ✅ Responsive sidebar, top bar, routing
- **State Management**: ✅ Zustand for global app state
- **API Client**: ✅ Fetch wrapper with error handling
- **Error Handling**: ✅ ErrorBoundary and loading states
- **Testing**: ✅ Vitest + React Testing Library setup
- **CI/CD**: ✅ GitHub Actions for frontend build/test

### 📋 Planned (Phases 4-7)

- **Library Browser (Phase 4)**:
  - Audiobook grid/list views with virtual scrolling
  - Advanced search and filtering
  - Inline metadata editing with validation
  - Batch operations for multiple books
- **File Management (Phase 5)**:
  - Directory tree browser
  - Library folder management interface
  - Safe file operations UI with previews
- **Settings & Monitoring (Phase 6)**:
  - Configuration management interface
  - System status dashboard
  - Operation monitoring and logs
- **Integration & Polish (Phase 7)**:
  - End-to-end testing and optimization
  - Accessibility improvements
  - Mobile responsiveness
  - Self-contained binary with embedded frontend

## 📋 Current CLI Features

The existing command-line interface provides a solid foundation with:

- Scanning directories for audiobook files (.m4b, .mp3, .m4a, .aac, .ogg, .flac,
  .wma)
- Extracting metadata and analyzing filenames/paths
- Identifying series relationships using pattern matching and fuzzy logic
- Storing organization data in a SQLite database (no files are moved)
- Generating iTunes-compatible playlists for each series
- Updating audio file metadata tags with series information

## Features

- **Non-destructive organization**: No files are moved or renamed
- **Smart series detection**: Uses multiple techniques to identify book series
- **Database-backed**: All organization info stored in SQLite
- **Playlist generation**: Creates iTunes-compatible playlists by series
- **Metadata tagging**: Updates audio files with series information
- **Library organization**: Optionally create hard links, reflinks, or copies in
  a structured library compatible with iTunes and other layouts

## Installation

```bash
# Clone the repository
git clone https://github.com/jdfalk/audiobook-organizer.git
cd audiobook-organizer

# Build the application
go build -o audiobook-organizer
```

### Metadata Writing

The application uses **native TagLib** (pure Go with bundled Wasm) for metadata
writing by default. This provides:

- Fast, dependency-free tag writing for all formats (MP3, M4B, M4A, FLAC, etc.)
- No external tools required
- Automatic fallback to CLI tools if native write fails

**Optional CLI tool fallback:**

If needed, the app can fall back to external tools:

- **M4B/M4A**: AtomicParsley (`brew install atomicparsley`)
- **MP3**: eyeD3 (`pip install eyeD3`)
- **FLAC**: metaflac (`brew install flac`)

These are only used if the native TagLib write fails.

## Usage

```bash
# Scan a directory of audiobooks
./audiobook-organizer scan --dir /path/to/audiobooks

# Generate playlists
./audiobook-organizer playlist

# Update audio file tags
./audiobook-organizer tag

# Or do everything at once
./audiobook-organizer organize --dir /path/to/audiobooks
````

## Configuration

Configuration can be provided via command-line flags, environment variables, or
a config file:

```yaml
# $HOME/.audiobook-organizer.yaml
root_dir: '/path/to/audiobooks'
database_path: 'audiobooks.db'
playlist_dir: 'playlists'
api_keys:
  goodreads: 'your-api-key-if-available'
```

## Documentation

For more detailed information, see the
[Technical Design Document](docs/technical_design.md) and the
[PebbleDB Keyspace Schema](docs/database-pebble-schema.md) for the database
layout and persistence model.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file
for details.

## Repository Automation

This project uses standard workflows and scripts from
[ghcommon](https://github.com/jdfalk/ghcommon).
