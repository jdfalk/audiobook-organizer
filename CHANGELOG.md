# Changelog

## [Unreleased]

### Added / Changed

- Extended Book metadata fields: work_id, narrator, edition, language, publisher, isbn10, isbn13 (with SQLite migration & CRUD support)
- API tests for extended metadata (round‑trip + update semantics)
- Hardened audiobook update handler error checking (nil-safe not found handling)
- Metadata extraction scaffolding for future multi-format support (tag reader integration prep)
- Work entity: basic model, SQLite schema, Pebble+SQLite store methods, and REST API endpoints (list/get/create/update/delete, list books by work)
- **Frontend**: Complete web interface with React + TypeScript + Material-UI
  - Dashboard with library statistics
  - Library page with import path management and manual import
  - Works page for audiobook organization
  - System page with tabs: Logs (real-time filtering), Storage breakdown, Quota management, System info
  - Settings page with comprehensive configuration (library paths, metadata sources, quotas, memory, logging)
- Media info and version management system:
  - Media quality fields: bitrate (kbps), codec (AAC/MP3/FLAC), sample rate, channels, bit depth
  - Human-readable quality strings (e.g., "320kbps AAC", "FLAC Lossless")
  - Version management: link multiple versions of same audiobook, mark primary version
  - Version notes for describing differences (e.g., "Remastered 2020", "Unabridged")
  - Organized in "Additional Versions" subfolder structure
  - Pattern fields support media info: `{bitrate}`, `{codec}`, `{quality}`
- Database migration (v5) adding media info and version management fields to SQLite books table
  - Automatically detects and handles duplicate columns
  - Creates indices for version_group_id and is_primary_version for query performance
- Media info extraction package for audio file metadata parsing
  - Supports MP3, M4A/M4B (AAC), FLAC, and OGG Vorbis formats
  - Extracts bitrate, codec, sample rate, channels, and bit depth
  - Generates human-readable quality strings (e.g., "320kbps MP3", "FLAC Lossless (16-bit/44.1kHz)")
  - Quality tier system for comparing audio versions (0-100 scale)
- **Smart Path Handling**: Empty fields (like {series}) automatically removed from folder paths (no duplicate slashes)
- **Naming Pattern Examples**: Live preview with both series and non-series books (Nancy Drew + To Kill a Mockingbird)

### Upcoming

- Audio tag reading for MP3 (ID3v2), M4B/M4A (iTunes atoms), FLAC/OGG (Vorbis comments), AAC
- Safe in-place metadata writing with backup/rollback
- Work entity (model + CRUD + association to Book via `work_id`)
- Manual endpoint regression run post ULID + metadata changes
- Git LFS sample audiobook fixtures for integration tests
  - POST `/api/filesystem/exclude` - Create .jabexclude files
