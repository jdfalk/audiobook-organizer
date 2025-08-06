# Audiobook Organizer

A Go application to organize audiobook collections by identifying series and generating playlists.

## Overview

Audiobook Organizer helps you manage your audiobook collection by:

- Scanning directories for audiobook files (.m4b, .mp3, etc.)
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
- **Library organization**: Optionally create hard links, reflinks, or copies in a structured library compatible with iTunes and other layouts

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

Configuration can be provided via command-line flags, environment variables, or a config file:

```yaml
# $HOME/.audiobook-organizer.yaml
root_dir: "/path/to/audiobooks"
database_path: "audiobooks.db"
playlist_dir: "playlists"
api_keys:
  goodreads: "your-api-key-if-available"
```

## Documentation

For more detailed information, see the [Technical Design Document](docs/technical_design.md).

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.


## Repository Automation

This project uses standard workflows and scripts from [ghcommon](https://github.com/jdfalk/ghcommon).