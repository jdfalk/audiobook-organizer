<!-- file: docs/mvp-specification.md -->
<!-- version: 1.0.0 -->
<!-- guid: 1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d -->
<!-- last-edited: 2026-01-19 -->

# Audiobook Organizer - MVP Specification

## Executive Summary

This document defines the Minimum Viable Product (MVP) for the Audiobook
Organizer, transforming the existing CLI tool into a full-featured web
application with an interactive React frontend and enhanced Go backend. The MVP
will provide a complete audiobook management solution with metadata editing,
file organization, and library management capabilities.

## Current State Analysis

### What We Have ✅

- **Go CLI Application**: Functional command-line tool with scan, playlist, tag,
  and organize commands
- **Database Layer**: SQLite database with schema for authors, series, books,
  playlists, and playlist items
- **Metadata Extraction**: Audio file metadata reading using dhowden/tag library
- **File Scanner**: Directory traversal with support for multiple audio formats
  (.m4b, .mp3, .m4a, .aac, .ogg, .flac, .wma)
- **Series Matching**: Basic pattern matching and fuzzy logic for series
  identification
- **Playlist Generation**: iTunes-compatible playlist creation
- **Tag Updates**: Audio file metadata modification capabilities
- **Configuration**: Viper-based config management with YAML support

### What We Need to Build 🔨

- **React Web Interface**: Complete frontend application with Material Design
- **REST API Backend**: HTTP endpoints for web interface communication
- **File Browser**: Server-side directory browsing and .jabexclude management
- **Real-time Updates**: WebSocket or SSE for operation progress
- **Safe File Operations**: Copy-first, backup-on-success file handling
- **Enhanced Format Support**: Broader audio format compatibility
- **File Operations API**: Move, rename, reorganize capabilities
- **Authentication**: Basic security for web interface (future consideration)

## MVP Feature Specification

### 1. Web Interface (React + Material Design)

#### 1.1 Application Layout

- **Left Sidebar Navigation**: Collapsible menu with the following sections:
  - 📚 **Library Browser**: Main audiobook management interface
  - 📁 **Folder Management**: Add/remove library folders
  - ⚙️ **Settings**: Application configuration
  - 📊 **Status**: Current operations and system status
  - 📋 **Logs**: Operation history and debug information

#### 1.2 Library Browser

- **Audiobook Grid/List View**:
  - Toggle between card grid and detailed list views
  - Sortable by: Title, Author, Series, Date Added, Duration, Format
  - Filterable by: Author, Series, Format, Status
  - Search functionality across all metadata fields
- **Audiobook Cards**: Display cover art, title, author, series info, duration
- **Metadata Editor**:
  - Inline editing for title, author, album, genre, series, position
  - Batch editing for multiple selections
  - Real-time validation and conflict detection
  - Undo/redo functionality

#### 1.3 Folder Management

- **Directory Browser**:
  - Server filesystem navigation with breadcrumb trail
  - Display folder contents with file counts
  - Create/delete .jabexclude files for folder exclusion
  - Visual indicators for excluded folders and existing audiobooks
- **Library Management**:
  - Add folders to audiobook library
  - Remove folders from scanning
  - Rescan specific folders
  - Bulk operations for multiple folders

#### 1.4 Settings Page

- **Library Settings**: Default scan locations, file naming patterns
- **Metadata Settings**: Preferred metadata sources, auto-tagging rules
- **File Operation Settings**: Backup preferences, safety mode toggles
- **Performance Settings**: Concurrent operations, cache settings
- **Export/Import**: Configuration backup and restore

#### 1.5 Status Dashboard

- **Current Operations**: Real-time progress of scans, metadata updates, file
  moves
- **System Information**: Library statistics, disk usage, performance metrics
- **Recent Activities**: Log of recent operations with timestamps
- **Queue Management**: View and manage pending operations

### 2. Go Backend (Go 1.25)

#### 2.1 Web Server Architecture

- **Self-contained Binary**: Embedded React build, no external dependencies
- **HTTP REST API**: JSON endpoints for all web interface operations
- **Static File Serving**: Serve React application and assets
- **WebSocket Support**: Real-time updates for long-running operations
- **Graceful Shutdown**: Proper cleanup of resources and in-progress operations

#### 2.2 API Endpoints

##### Library Management

```
GET    /api/audiobooks              # List all audiobooks with pagination/filtering
GET    /api/audiobooks/{id}         # Get specific audiobook details
PUT    /api/audiobooks/{id}         # Update audiobook metadata
DELETE /api/audiobooks/{id}         # Remove audiobook from library
POST   /api/audiobooks/batch        # Batch metadata updates
GET    /api/authors                 # List all authors
GET    /api/series                  # List all series
```

##### File Operations

```
POST   /api/operations/scan         # Trigger library scan
POST   /api/operations/move         # Move/rename files
POST   /api/operations/organize     # Auto-organize library
GET    /api/operations/{id}/status  # Check operation status
DELETE /api/operations/{id}         # Cancel operation
```

##### Folder Management

```
GET    /api/filesystem/browse       # Browse server filesystem
POST   /api/filesystem/exclude      # Create .jabexclude file
DELETE /api/filesystem/exclude      # Remove .jabexclude file
GET    /api/library/folders         # List managed library folders
POST   /api/library/folders         # Add folder to library
DELETE /api/library/folders/{id}    # Remove folder from library
```

##### System

```
GET    /api/system/status           # System health and statistics
GET    /api/system/logs             # Application logs
GET    /api/config                  # Current configuration
PUT    /api/config                  # Update configuration
```

#### 2.3 Enhanced File Operations

- **Safe Operation Mode**:
  - Copy original to `.bak` extension before modification
  - Atomic operations with rollback capability
  - Verification checksums for data integrity
  - Detailed operation logging

- **File Format Support**:
  - **Primary**: MP3, M4A, M4B (iTunes audiobook format)
  - **Secondary**: AAC, FLAC, OGG Vorbis, WMA
  - **Metadata Standards**: ID3v2.4, MP4 atoms, Vorbis comments

- **Reorganization Features**:
  - Create directory structures based on metadata
  - Rename files using configurable patterns
  - Move files to organized locations
  - Handle duplicate detection and resolution

#### 2.4 Database Enhancements

- **Extended Schema**: Add tables for library folders, operation history, user
  preferences
- **Migration System**: Automatic schema updates for future versions
- **Backup/Restore**: Database export and import functionality
- **Optimization**: Indexing for performance, cleanup routines

### 3. File Format & Metadata Standards

#### 3.1 Supported Audio Formats

| Format | Extension | Priority | Metadata Support | Notes                     |
| ------ | --------- | -------- | ---------------- | ------------------------- |
| M4B    | .m4b      | High     | Full MP4 atoms   | iTunes audiobook standard |
| MP3    | .mp3      | High     | ID3v2.4          | Most common format        |
| M4A    | .m4a      | High     | Full MP4 atoms   | iTunes audio standard     |
| AAC    | .aac      | Medium   | Limited          | Raw AAC streams           |
| FLAC   | .flac     | Medium   | Vorbis comments  | Lossless quality          |
| OGG    | .ogg      | Low      | Vorbis comments  | Open source format        |
| WMA    | .wma      | Low      | ASF metadata     | Windows media format      |

#### 3.2 Metadata Fields

- **Standard Fields**: Title, Artist/Author, Album, Genre, Year, Track Number
- **Audiobook Specific**: Series Name, Series Position, Duration, Narrator
- **Custom Fields**: Tags, Rating, Description, Publisher, ISBN
- **Technical**: Bitrate, Sample Rate, Codec, File Size

### 4. .jabexclude File Format

Simple JSON format for folder exclusion:

```json
{
  "version": "1.0",
  "excluded_by": "Audiobook Organizer",
  "excluded_date": "2025-01-15T10:30:00Z",
  "reason": "User excluded this folder from audiobook library",
  "note": "This folder is ignored by the audiobook organizer"
}
```

## Implementation Phases

### Phase 1: Backend Foundation (Weeks 1-2)

1. **Upgrade Go Version**: Update to Go 1.25, update go.mod
2. **Web Server Setup**: Add HTTP server with basic routing
3. **API Framework**: Implement core REST endpoints with proper error handling
4. **Database Migration**: Extend schema for web interface needs
5. **File Operations**: Implement safe copy-first file handling

### Phase 2: Core API Development (Weeks 3-4)

1. **Library API**: Complete audiobook CRUD operations
2. **Metadata API**: Implement metadata reading/writing with validation
3. **File System API**: Add directory browsing and .jabexclude management
4. **Operations API**: Async operations with status tracking
5. **WebSocket Setup**: Real-time updates for long operations

### Phase 3: React Frontend Foundation (Weeks 5-6)

1. **Project Setup**: Create React app with Material-UI and TypeScript
2. **Layout Components**: Implement sidebar navigation and main content areas
3. **API Integration**: Set up Axios client with proper error handling
4. **Routing**: Implement client-side routing for different sections
5. **State Management**: Set up Redux or Context for global state

### Phase 4: Library Browser (Weeks 7-8)

1. **Audiobook Grid**: Display audiobooks with sorting and filtering
2. **Metadata Editor**: Inline editing with validation and error handling
3. **Search Functionality**: Full-text search across metadata
4. **Batch Operations**: Multi-select and batch editing capabilities
5. **Progress Indicators**: Loading states and operation feedback

### Phase 5: File Management (Weeks 9-10)

1. **Directory Browser**: Server filesystem navigation component
2. **Folder Management**: Add/remove library folders interface
3. **Exclusion Management**: Create/delete .jabexclude files
4. **Visual Indicators**: Show excluded folders and scan status
5. **Rescan Operations**: Trigger and monitor folder rescans

### Phase 6: Settings & Status (Weeks 11-12)

1. **Settings Interface**: Configuration management UI
2. **Status Dashboard**: Operation monitoring and system stats
3. **Log Viewer**: Application logs with filtering and search
4. **Error Handling**: Comprehensive error display and recovery
5. **Performance Optimization**: Code splitting and lazy loading

### Phase 7: Integration & Testing (Weeks 13-14)

1. **End-to-End Testing**: Complete user workflow validation
2. **Performance Testing**: Large library handling and optimization
3. **Error Recovery**: Edge case handling and graceful degradation
4. **Documentation**: User guide and API documentation
5. **Deployment**: Build and packaging for distribution

## Current Progress Assessment

### Completed (≈40% of MVP)

- ✅ **Database Schema**: Core tables and relationships established
- ✅ **Metadata Extraction**: Audio file reading with dhowden/tag
- ✅ **File Scanning**: Directory traversal and file discovery
- ✅ **Configuration System**: Viper-based config management
- ✅ **Series Detection**: Basic pattern matching algorithms
- ✅ **CLI Interface**: Complete command-line functionality

### In Progress (≈10% of MVP)

- 🔄 **File Operations**: Basic metadata writing exists, needs safety features
- 🔄 **Format Support**: Limited to common formats, needs expansion

### Not Started (≈50% of MVP)

- ❌ **Web Interface**: Complete React application needed
- ❌ **REST API**: HTTP server and endpoints needed
- ❌ **File Browser**: Server-side directory browsing needed
- ❌ **Real-time Updates**: WebSocket implementation needed
- ❌ **Safe Operations**: Copy-first file handling needed
- ❌ **Library Management**: Folder add/remove functionality needed

## Technical Risks & Mitigations

### High Risk

1. **File Corruption**: Mitigate with atomic operations and checksums
2. **Large Library Performance**: Implement pagination and lazy loading
3. **Concurrent Operations**: Use proper locking and queue management

### Medium Risk

1. **Cross-platform Compatibility**: Test on Windows/macOS/Linux
2. **Memory Usage**: Profile and optimize for large collections
3. **Network Security**: Implement proper input validation and sanitization

### Low Risk

1. **Browser Compatibility**: Use modern React with polyfills
2. **Audio Format Edge Cases**: Graceful degradation for unsupported formats

## Success Metrics

### MVP Completion Criteria

- [ ] **Web Interface**: All 5 main sections functional and user-friendly
- [ ] **Metadata Editing**: Safe, reliable editing with undo/redo
- [ ] **File Operations**: Zero data loss with comprehensive backup system
- [ ] **Library Management**: Easy folder addition/removal with .jabexclude
      support
- [ ] **Performance**: Handle 10,000+ audiobook libraries smoothly
- [ ] **Documentation**: Complete user guide and installation instructions

### Quality Gates

- [ ] **Test Coverage**: >80% backend code coverage, comprehensive frontend
      testing
- [ ] **Performance**: <3 second load times, <500ms API response times
- [ ] **Reliability**: 99.9% operation success rate with proper error recovery
- [ ] **Usability**: Intuitive interface requiring minimal documentation

## Resource Requirements

### Development Environment

- **Go 1.25+**: Latest Go toolchain
- **Node.js 18+**: For React development
- **SQLite**: Database for development and testing
- **Git**: Version control and collaboration

### Dependencies

- **Backend**: Gin/Echo (HTTP), gorilla/websocket, testify (testing)
- **Frontend**: React 18, Material-UI v5, TypeScript, Axios, React Router
- **Build**: Embed for asset bundling, Docker for containerization

### Infrastructure

- **Development**: Local development servers
- **Testing**: Automated CI/CD pipeline
- **Distribution**: Binary releases for major platforms

This MVP specification provides a comprehensive roadmap for transforming the
existing CLI tool into a full-featured web application while maintaining the
reliability and functionality that users expect from professional audiobook
management software.
