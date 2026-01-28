<!-- file: testdata/itunes/README.md -->
<!-- version: 1.0.0 -->
<!-- guid: a1b2c3d4-e5f6-7890-test-data123456 -->
<!-- last-edited: 2026-01-27 -->

# iTunes Test Data

**Purpose**: iTunes Library.xml files for testing iTunes import functionality
**Owner**: User's real iTunes library metadata (10TB+ audiobook collection)

---

## Files in This Directory

### Real iTunes Library Files

These are **copies** of the user's actual iTunes library files. They contain metadata for a massive audiobook collection (10TB+).

**Files**:
- `iTunes Music Library.xml` - Main iTunes library file (if present)
- `Library.xml` - Alternative/modern Music.app format (if present)
- Any other iTunes-related files copied here

**Usage**:
- Integration tests with real data
- Manual verification of import workflow
- Creating test subsets (see below)

**DO NOT COMMIT**: These files contain personal library metadata and should NOT be committed to git. Already in .gitignore.

---

## Test Data Strategy

### 1. Full Library (Real Data)

**Purpose**: Verification and manual testing
**File**: `iTunes Music Library.xml` or `Library.xml`
**Usage**:
```bash
# Manual verification script
./scripts/verify_itunes_import.sh

# Integration test
go test -tags=integration ./internal/itunes/ -run TestParseRealLibrary
```

**Why Keep**:
- Validates against real-world data
- Tests edge cases in actual library
- Ensures 10TB library can be imported

### 2. Test Subset (Generated)

**Purpose**: Fast automated testing
**File**: `test_library_subset.xml` (generated, not committed)
**Size**: 10-20 audiobooks
**Usage**:
```bash
# Generate subset
go run testdata/itunes/create_test_subset.go

# Use in tests
go test ./internal/itunes/ -run TestParseSubset
```

**Benefits**:
- Fast test execution (< 1 second)
- Reproducible results
- Small enough for CI/CD

### 3. Edge Case Libraries (To Be Created)

**Purpose**: Test specific scenarios

**Files to create**:
- `test_library_empty.xml` - Empty library (no tracks)
- `test_library_missing_files.xml` - Library with missing files
- `test_library_duplicates.xml` - Library with duplicate hashes
- `test_library_playlists.xml` - Library with many playlists

**How to create**: Manually edit XML or use subset generator with filters

---

## Creating Test Subset

### Automated Script

**File**: `testdata/itunes/create_test_subset.go`

```go
//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jdfalk/audiobook-organizer/internal/itunes"
)

func main() {
	// Find real library file
	realLibrary := ""
	candidates := []string{
		"iTunes Music Library.xml",
		"Library.xml",
		"iTunes Library.xml",
	}

	for _, candidate := range candidates {
		path := filepath.Join("testdata/itunes", candidate)
		if _, err := os.Stat(path); err == nil {
			realLibrary = path
			break
		}
	}

	if realLibrary == "" {
		fmt.Fprintf(os.Stderr, "ERROR: No iTunes library file found in testdata/itunes/\n")
		fmt.Fprintf(os.Stderr, "Expected one of: %v\n", candidates)
		os.Exit(1)
	}

	fmt.Printf("Parsing real library: %s\n", realLibrary)

	// Parse full library
	library, err := itunes.ParseLibrary(realLibrary)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d total tracks\n", len(library.Tracks))

	// Create subset library
	subset := &itunes.Library{
		MajorVersion:       library.MajorVersion,
		MinorVersion:       library.MinorVersion,
		ApplicationVersion: library.ApplicationVersion,
		MusicFolder:        library.MusicFolder,
		Tracks:             make(map[string]*itunes.Track),
		Playlists:          make([]*itunes.Playlist, 0),
	}

	// Copy first 10 audiobooks
	count := 0
	maxBooks := 10

	for id, track := range library.Tracks {
		if itunes.IsAudiobook(track) {
			subset.Tracks[id] = track
			count++
			fmt.Printf("  Added: %s by %s\n", track.Name, track.Artist)

			if count >= maxBooks {
				break
			}
		}
	}

	fmt.Printf("\nCreated subset with %d audiobooks\n", count)

	// Copy relevant playlists (only those containing selected tracks)
	trackIDs := make(map[int]bool)
	for _, track := range subset.Tracks {
		trackIDs[track.TrackID] = true
	}

	for _, playlist := range library.Playlists {
		// Skip built-in playlists
		if isBuiltInPlaylist(playlist.Name) {
			continue
		}

		// Check if playlist contains any of our tracks
		hasTrack := false
		for _, trackID := range playlist.TrackIDs {
			if trackIDs[trackID] {
				hasTrack = true
				break
			}
		}

		if hasTrack {
			subset.Playlists = append(subset.Playlists, playlist)
			fmt.Printf("  Added playlist: %s\n", playlist.Name)
		}
	}

	// Write subset
	outputPath := filepath.Join("testdata/itunes", "test_library_subset.xml")

	// Note: writePlist is not exported, need to use internal method
	// For now, print instructions
	fmt.Printf("\nTo complete subset creation:\n")
	fmt.Printf("  1. Add export to writePlist in internal/itunes/plist_parser.go\n")
	fmt.Printf("  2. Or manually call it via test helper\n")
	fmt.Printf("  3. Output should go to: %s\n", outputPath)
}

func isBuiltInPlaylist(name string) bool {
	builtIn := []string{
		"Music", "Movies", "TV Shows", "Podcasts", "Audiobooks",
		"iTunes U", "Books", "Genius", "Recently Added",
		"Recently Played", "Top 25 Most Played",
	}

	for _, b := range builtIn {
		if name == b {
			return true
		}
	}
	return false
}
```

### Usage

```bash
# Generate test subset
cd /path/to/audiobook-organizer
go run testdata/itunes/create_test_subset.go

# Output will be: testdata/itunes/test_library_subset.xml
```

---

## Verification Workflow

### 1. Manual Verification

**Script**: `scripts/verify_itunes_import.sh`

```bash
#!/bin/bash
set -e

echo "=== iTunes Import Verification ==="
echo ""

# Check for real library
LIBRARY_FILE=""
for file in "testdata/itunes/iTunes Music Library.xml" "testdata/itunes/Library.xml"; do
    if [ -f "$file" ]; then
        LIBRARY_FILE="$file"
        break
    fi
done

if [ -z "$LIBRARY_FILE" ]; then
    echo "ERROR: No iTunes library found in testdata/itunes/"
    exit 1
fi

echo "✓ Found iTunes library: $LIBRARY_FILE"
echo ""

# Parse test
echo "1. Testing parser..."
go test -v ./internal/itunes/ -run TestParseLibrary
echo ""

# Integration test with real library
echo "2. Testing with real library..."
go test -tags=integration -v ./internal/itunes/ -run TestParseRealLibrary
echo ""

echo "✅ All verification tests passed!"
```

### 2. Automated Testing

**Unit Tests** (use subset):
```bash
go test ./internal/itunes/...
```

**Integration Tests** (use real library):
```bash
go test -tags=integration ./internal/itunes/...
```

**E2E Tests** (use subset):
```bash
cd web
npm run test:e2e -- itunes-import
```

---

## File Organization

```
testdata/itunes/
├── README.md                          # This file
├── iTunes Music Library.xml           # Real library (NOT committed)
├── Library.xml                        # Alternative format (NOT committed)
├── test_library_subset.xml           # Generated 10-book subset (NOT committed)
├── create_test_subset.go             # Script to generate subset
├── test_library_empty.xml            # Edge case: empty library (create manually)
├── test_library_missing_files.xml    # Edge case: missing files (create manually)
└── test_library_duplicates.xml       # Edge case: duplicates (create manually)
```

---

## .gitignore Entries

Make sure these are in `.gitignore`:

```gitignore
# iTunes test data (contains personal library metadata)
testdata/itunes/*.xml
testdata/itunes/Library.xml
testdata/itunes/iTunes*.xml

# Keep only the README and generator script
!testdata/itunes/README.md
!testdata/itunes/create_test_subset.go
```

---

## Test Data Maintenance

### When to Regenerate Subset

- After major iTunes library changes
- When adding new audiobooks to test specific scenarios
- If subset becomes stale or corrupted

### How to Update

```bash
# 1. Copy latest iTunes library
cp ~/Music/iTunes/iTunes\ Music\ Library.xml testdata/itunes/

# 2. Regenerate subset
go run testdata/itunes/create_test_subset.go

# 3. Run verification
./scripts/verify_itunes_import.sh
```

---

## Security Notes

⚠️ **IMPORTANT**: iTunes library files contain:
- Personal file paths
- Collection metadata
- Potentially sensitive information

**Never commit** these files to public repositories.

**Safe to commit**:
- ✅ This README
- ✅ Generator scripts
- ✅ Minimal test fixtures (manually created, no personal data)

---

## Troubleshooting

### "No iTunes library found"

**Solution**: Copy your iTunes library file to this directory:
```bash
# macOS
cp ~/Music/iTunes/iTunes\ Music\ Library.xml testdata/itunes/

# Windows
copy "C:\Users\YourName\Music\iTunes\iTunes Music Library.xml" testdata\itunes\
```

### "Parser fails on real library"

**Possible causes**:
1. Corrupted XML (check with XML validator)
2. Unsupported iTunes version (check version in XML)
3. Custom fields not handled (add support in parser)

**Debug**:
```bash
# Run parser with verbose output
go test -v ./internal/itunes/ -run TestParseRealLibrary
```

### "Subset generation fails"

**Possible causes**:
1. No audiobooks in library (all music)
2. writePlist not accessible (not exported)

**Solution**: Check audiobook detection logic in `IsAudiobook()`

---

## Resources

- [iTunes Library XML Format](https://developer.apple.com/documentation/ituneslibrary)
- [howett/plist Documentation](https://github.com/DHowett/go-plist)
- [ITUNES_IMPORT_SPECIFICATION.md](../../ITUNES_IMPORT_SPECIFICATION.md) - Complete spec
- [ITUNES_INTEGRATION_AI_GUIDE.md](../../ITUNES_INTEGRATION_AI_GUIDE.md) - Implementation guide

---

**Last Updated**: 2026-01-27
**Maintainer**: Project owner
**Status**: Ready for testing
