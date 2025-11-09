# Audiobook Organizer

A comprehensive audiobook management application with a modern web interface and
powerful Go backend for organizing, managing, and enjoying your audiobook collection.

## 🎯 MVP Development in Progress

**Current Status**: CLI application complete (~35% of MVP), transitioning to full web application

See our detailed planning documents:
- **[MVP Specification](docs/mvp-specification.md)**: Complete feature requirements and architecture
- **[Implementation Plan](docs/mvp-implementation-plan.md)**: 14-week development roadmap
- **[Progress Analysis](docs/current-progress-analysis.md)**: Current state assessment
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

- **Safe File Operations**: Copy-first with SHA256 verification, automatic backups, rollback support
- **File System API**: Directory browsing, .jabexclude management, disk space checking
- **Real-time Updates**: Server-Sent Events (SSE) for operation progress, status, and logs
- **Async Operation Queue**: Priority-based background processing with configurable workers
- **Event Broadcasting**: Automatic client notification for operation updates

### 🚧 In Progress

- **Enhanced Metadata**: Batch updates, validation, history tracking
- **Database Backup/Restore**: Automated backup creation and restoration

### 📋 Planned

- **Web Interface (React + Material Design)**:
  - Library Browser with grid/list view and advanced filtering
  - Metadata Editor with inline editing and batch operations
  - Folder Management with server directory browsing
  - Settings Dashboard and status monitoring
- **Self-contained Binary**: Embedded React frontend, zero external dependencies

## 📋 Current CLI Features

The existing command-line interface provides a solid foundation with:

- Scanning directories for audiobook files (.m4b, .mp3, .m4a, .aac, .ogg, .flac, .wma)
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
```

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
[PebbleDB Keyspace Schema](docs/database-pebble-schema.md) for the database layout and persistence model.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file
for details.

## Repository Automation

This project uses standard workflows and scripts from
[ghcommon](https://github.com/jdfalk/ghcommon).
