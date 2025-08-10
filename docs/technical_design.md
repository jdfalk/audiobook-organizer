# Audiobook Organizer - Technical Design Document

## Overview

Audiobook Organizer is a command-line application designed to help users
organize their audiobook collections by identifying series, generating
playlists, and updating audio file metadata. The application scans audiobook
files, extracts metadata, uses pattern matching and fuzzy logic to identify
series relationships, and stores this information in a SQLite database without
modifying the original file structure.

## Architecture

The application follows a modular architecture with clear separation of
concerns:

```
audiobook-organizer/
├── cmd/               # Command-line interface definitions
├── internal/          # Private application code
│   ├── api/           # External API integrations
│   ├── config/        # Configuration management
│   ├── database/      # Database operations
│   ├── matcher/       # Series matching algorithms
│   ├── metadata/      # Audio metadata extraction
│   ├── playlist/      # Playlist generation
│   ├── scanner/       # File system scanning
│   └── tagger/        # Audio file tag updates
├── docs/              # Documentation
└── main.go            # Application entry point
```

## Key Components

### Command Interface (cmd)

Uses Cobra and Viper to provide a flexible and powerful CLI:

- **root**: Main command with global flags and configuration
- **scan**: Scans directories for audiobooks and processes metadata
- **playlist**: Generates playlists for identified series
- **tag**: Updates audio file metadata with series information
- **organize**: Convenience command that runs all the above in sequence

### Configuration (internal/config)

Manages application settings using Viper:

- Loads from config files, environment variables, and command-line flags
- Supports paths to audiobook directories, database, playlist output
- Configures supported file extensions and external API keys

### Database (internal/database)

SQLite3-based persistence layer:

- **authors**: Stores author information
- **series**: Stores series information with author relationships
- **books**: Stores book information with paths, formats, and series
  relationships
- **playlists**: Stores generated playlist information
- **playlist_items**: Stores the composition of playlists

### Scanner (internal/scanner)

Responsible for discovering and processing audiobook files:

- Walks directory structures to find supported audio files
- Extracts metadata and identifies series relationships
- Maps files to database entities

### Metadata (internal/metadata)

Extracts and processes metadata from audio files:

- Uses the `dhowden/tag` library to read standard tags
- Falls back to filename and path analysis when tags are missing
- Handles various audio formats including M4B, MP3, and others

### Matcher (internal/matcher)

Implements series identification algorithms:

- Pattern matching using regular expressions
- Directory structure analysis
- Fuzzy matching for similar titles using `lithammer/fuzzysearch`
- Keyword detection for series indicators

### Playlist (internal/playlist)

Generates playlists for audio applications:

- Creates M3U format playlists compatible with iTunes and other players
- Orders books by series sequence or title
- Sanitizes filenames and paths for cross-platform compatibility

### Tagger (internal/tagger)

Updates metadata tags in audio files:

- Adds or updates series information using format-specific tools
- Supports M4B/M4A/AAC (via AtomicParsley), MP3 (via eyeD3), and FLAC (via
  metaflac)
- Currently implemented as mock operations with actual commands commented

## Database Schema

```
+----------------+       +---------------+       +---------------+
| authors        |       | series        |       | books         |
+----------------+       +---------------+       +---------------+
| id             |<---+  | id            |<---+  | id            |
| name           |    |  | name          |    |  | title         |
+----------------+    |  | author_id     |----+  | author_id     |
                      |  +---------------+       | series_id     |
                      |                          | series_sequence|
                      |                          | file_path      |
                      |  +---------------+       | format         |
                      |  | playlists     |       | duration       |
                      |  +---------------+       +---------------+
                      |  | id            |             ^
                      |  | name          |             |
                      |  | series_id     |----+        |
                      |  | file_path     |    |        |
                      |  +---------------+    |        |
                                              |        |
                      +---------------------->|        |
                                              |        |
                                              |        |
                                     +----------------+|
                                     | playlist_items ||
                                     +----------------+|
                                     | id             ||
                                     | playlist_id    |+
                                     | book_id        |--+
                                     | position       |
                                     +----------------+
```

## Algorithmic Approach

### Series Identification

1. **Metadata Analysis**:
   - Extract standard metadata tags (artist, title, album)
   - Look for grouping/content group tags that might contain series info

2. **Pattern Matching**:
   - Regular expressions match common patterns like "Series Name - Book Title"
   - Identify book numbers in titles ("Book 1", "#1", "Vol. 1")

3. **Path Analysis**:
   - Extract information from directory names
   - Use author directory and subdirectories to infer series relationships

4. **Fuzzy Matching**:
   - Compare titles using fuzzy string matching to find similar patterns
   - Detect series keywords ("trilogy", "saga", "chronicles")

## Performance Considerations

- Progress bars for long-running operations
- Database transaction batching for bulk operations
- Parallel processing opportunities (currently sequential)
- Indexed queries for efficient database operations

## External Dependencies

- **spf13/cobra**: Command-line interface framework
- **spf13/viper**: Configuration management
- **mattn/go-sqlite3**: SQLite database driver
- **dhowden/tag**: Audio metadata extraction
- **lithammer/fuzzysearch**: Fuzzy string matching
- **schollz/progressbar**: Progress visualization

## Future Enhancements

1. **External API Integration**:
   - Integration with Goodreads or similar services for better series
     identification
   - Book database lookups to supplement metadata

2. **Improved Tag Writing**:
   - Direct tag writing implementation instead of external tool calls
   - Support for more audio formats

3. **Web Interface**:
   - Optional web UI for visualization and manual organization
   - Series relationship editing

4. **Advanced Matching**:
   - Machine learning-based title and series matching
   - User feedback loop to improve matching over time

5. **Additional Playlist Formats**:
   - Support for more playlist formats beyond M3U
   - Smart playlist generation based on listening habits

## Known Limitations

1. Tag writing is currently placeholder-only and requires external tools
2. No support for multi-author series
3. Limited handling of books that belong to multiple series
4. Fuzzy matching may produce false positives with similar titles
5. No handling of cover art or other media assets

6. **Library Organization**:
   - Optionally create a structured library using hard links, reflinks, or
     copies
   - Support multiple layout styles including iTunes-compatible organization
