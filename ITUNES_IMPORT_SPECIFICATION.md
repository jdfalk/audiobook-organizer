<!-- file: ITUNES_IMPORT_SPECIFICATION.md -->
<!-- version: 1.0.0 -->
<!-- guid: c1d2e3f4-a5b6-7890-cdef-1a2b3c4d5e6f -->
<!-- last-edited: 2026-01-25 -->

# iTunes Library Import - Complete Specification

**Date**: 2026-01-25 **Priority**: P0 - Critical for MVP (personal use case
blocker) **Status**: Design specification - ready for implementation

---

## Executive Summary

**Goal**: Enable seamless migration from iTunes library management to
audiobook-organizer by importing all iTunes library metadata, file locations,
playback statistics, and optionally writing back new file paths after
organization.

**User Story**: "As an iTunes user with 500+ audiobooks, I want to import my
entire iTunes library with all my play counts, ratings, and bookmarks preserved,
so I can switch to audiobook-organizer without losing years of listening
history."

**Critical Success Factors**:

1. ✅ Zero data loss - All iTunes metadata preserved
2. ✅ File discovery - Locate all audiobook files referenced in iTunes
3. ✅ Playback continuity - Preserve bookmarks and play counts
4. ✅ Optional write-back - Update iTunes with new file paths after organize

---

## Part 1: iTunes Library File Formats

### iTunes Library.xml

**Location** (macOS):

- Modern iTunes/Music.app: `~/Music/Music/Library.xml`
- Older iTunes: `~/Music/iTunes/iTunes Music Library.xml`

**Location** (Windows):

- `C:\Users\[Username]\Music\iTunes\iTunes Music Library.xml`

**Format**: XML plist (Property List)

**Key Structure**:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Major Version</key>
    <integer>1</integer>
    <key>Minor Version</key>
    <integer>1</integer>
    <key>Application Version</key>
    <string>12.12.9.1</string>
    <key>Music Folder</key>
    <string>file://localhost/Users/username/Music/iTunes/iTunes%20Media/</string>

    <key>Tracks</key>
    <dict>
        <key>12345</key>
        <dict>
            <key>Track ID</key>
            <integer>12345</integer>
            <key>Name</key>
            <string>The Hobbit</string>
            <key>Artist</key>
            <string>J.R.R. Tolkien</string>
            <key>Album</key>
            <string>The Hobbit (Unabridged)</string>
            <key>Genre</key>
            <string>Audiobooks</string>
            <key>Kind</key>
            <string>Audiobook</string>
            <key>Size</key>
            <integer>425984000</integer>
            <key>Total Time</key>
            <integer>43200000</integer>
            <key>Year</key>
            <integer>2012</integer>
            <key>Date Added</key>
            <date>2020-05-15T14:30:00Z</date>
            <key>Play Count</key>
            <integer>3</integer>
            <key>Play Date</key>
            <integer>3723456789</integer>
            <key>Rating</key>
            <integer>100</integer>
            <key>Bookmark</key>
            <integer>5432100</integer>
            <key>Bookmarkable</key>
            <true/>
            <key>Location</key>
            <string>file://localhost/Users/username/Music/iTunes/iTunes%20Media/Audiobooks/J.R.R.%20Tolkien/The%20Hobbit/The%20Hobbit.m4b</string>
            <key>Persistent ID</key>
            <string>1234567890ABCDEF</string>
        </dict>
    </dict>

    <key>Playlists</key>
    <array>
        <dict>
            <key>Name</key>
            <string>Fantasy</string>
            <key>Playlist ID</key>
            <integer>67890</integer>
            <key>Playlist Items</key>
            <array>
                <dict>
                    <key>Track ID</key>
                    <integer>12345</integer>
                </dict>
            </array>
        </dict>
    </array>
</dict>
</plist>
```

### iTunes Database (Alternative)

**Location**: `~/Music/iTunes/iTunes Library.itl`

**Format**: Binary database (SQLite-based)

**Note**: XML is preferred as it's documented and officially exported by iTunes.
Binary database requires reverse engineering.

---

## Part 2: Metadata Mapping

### iTunes Fields → Audiobook Organizer Fields

| iTunes Field    | Type    | Audiobook Organizer Field             | Notes                                |
| --------------- | ------- | ------------------------------------- | ------------------------------------ |
| `Track ID`      | integer | Import tracking (temp)                | Used during import, not stored       |
| `Persistent ID` | string  | `itunes_persistent_id` (new field)    | Unique iTunes identifier             |
| `Name`          | string  | `title`                               | Direct mapping                       |
| `Artist`        | string  | `author_name`                         | Direct mapping                       |
| `Album Artist`  | string  | `narrator` (if different from Artist) | Narrator inference                   |
| `Album`         | string  | `series_name` (extract series)        | May contain series info              |
| `Genre`         | string  | `genre`                               | Direct mapping                       |
| `Year`          | integer | `audiobook_release_year`              | Publication year                     |
| `Comments`      | string  | `description`                         | User notes/description               |
| `Location`      | string  | `file_path` (after decoding)          | URL-decode file:// path              |
| `Size`          | integer | Metadata only                         | File size in bytes                   |
| `Total Time`    | integer | `duration`                            | Duration in milliseconds             |
| `Date Added`    | date    | `itunes_date_added` (new field)       | When added to iTunes                 |
| `Play Count`    | integer | `itunes_play_count` (new field)       | Number of plays                      |
| `Play Date`     | integer | `itunes_last_played` (new field)      | Unix timestamp or iTunes epoch       |
| `Rating`        | integer | `itunes_rating` (new field)           | 0-100 scale                          |
| `Bookmark`      | integer | `itunes_bookmark` (new field)         | Playback position in ms              |
| `Bookmarkable`  | boolean | Metadata only                         | Always true for audiobooks           |
| `Kind`          | string  | Filter criteria                       | Must be "Audiobook" or "Spoken Word" |

### New Database Fields Required

```sql
-- Add to audiobooks table (migration 11)
ALTER TABLE audiobooks ADD COLUMN itunes_persistent_id TEXT;
ALTER TABLE audiobooks ADD COLUMN itunes_date_added TIMESTAMP;
ALTER TABLE audiobooks ADD COLUMN itunes_play_count INTEGER DEFAULT 0;
ALTER TABLE audiobooks ADD COLUMN itunes_last_played TIMESTAMP;
ALTER TABLE audiobooks ADD COLUMN itunes_rating INTEGER; -- 0-100 scale
ALTER TABLE audiobooks ADD COLUMN itunes_bookmark INTEGER; -- milliseconds
ALTER TABLE audiobooks ADD COLUMN itunes_import_source TEXT; -- Path to iTunes Library.xml

CREATE INDEX idx_audiobooks_itunes_persistent_id ON audiobooks(itunes_persistent_id);
```

### Playlist Handling

**Option 1**: Import as tags

- Each iTunes playlist becomes a tag on the audiobook
- Example: "Fantasy" playlist → `tags: ["fantasy"]`

**Option 2**: Create playlist mapping table

```sql
CREATE TABLE itunes_playlists (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    itunes_playlist_id INTEGER,
    imported_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE audiobook_playlists (
    audiobook_id TEXT NOT NULL,
    playlist_id TEXT NOT NULL,
    FOREIGN KEY (audiobook_id) REFERENCES audiobooks(id),
    FOREIGN KEY (playlist_id) REFERENCES itunes_playlists(id),
    PRIMARY KEY (audiobook_id, playlist_id)
);
```

**Recommendation**: Option 1 (tags) for MVP, Option 2 for vNext multi-user
support

---

## Part 3: Import Workflow

### Phase 1: Parse iTunes Library

**Step 1**: Locate iTunes Library file

```
Search locations:
1. User-provided path (UI file picker)
2. macOS default: ~/Music/Music/Library.xml
3. macOS legacy: ~/Music/iTunes/iTunes Music Library.xml
4. Windows: C:\Users\[Username]\Music\iTunes\iTunes Music Library.xml
```

**Step 2**: Parse XML

```go
type ITunesLibrary struct {
    MajorVersion       int
    MinorVersion       int
    ApplicationVersion string
    MusicFolder        string
    Tracks             map[string]ITunesTrack
    Playlists          []ITunesPlaylist
}

type ITunesTrack struct {
    TrackID       int
    PersistentID  string
    Name          string
    Artist        string
    AlbumArtist   string
    Album         string
    Genre         string
    Kind          string
    Year          int
    Comments      string
    Location      string
    Size          int64
    TotalTime     int64 // milliseconds
    DateAdded     time.Time
    PlayCount     int
    PlayDate      int64 // Unix timestamp
    Rating        int   // 0-100
    Bookmark      int64 // milliseconds
    Bookmarkable  bool
}

type ITunesPlaylist struct {
    PlaylistID   int
    Name         string
    TrackIDs     []int
}
```

**Step 3**: Filter audiobooks

```go
func isAudiobook(track ITunesTrack) bool {
    // Check Kind field
    if strings.Contains(strings.ToLower(track.Kind), "audiobook") {
        return true
    }
    if strings.Contains(strings.ToLower(track.Kind), "spoken word") {
        return true
    }

    // Check Genre
    if strings.Contains(strings.ToLower(track.Genre), "audiobook") {
        return true
    }

    // Check file location contains "Audiobooks"
    if strings.Contains(track.Location, "Audiobooks") {
        return true
    }

    return false
}
```

### Phase 2: File Discovery & Validation

**Step 1**: Decode file paths

```go
func decodeITunesLocation(location string) (string, error) {
    // Remove "file://localhost" prefix
    location = strings.TrimPrefix(location, "file://localhost")
    location = strings.TrimPrefix(location, "file://")

    // URL decode (handles %20, %2F, etc.)
    decoded, err := url.QueryUnescape(location)
    if err != nil {
        return "", err
    }

    // Handle Windows paths (C:/ vs /C:/)
    if runtime.GOOS == "windows" {
        decoded = strings.TrimPrefix(decoded, "/")
    }

    return decoded, nil
}
```

**Step 2**: Verify files exist

```go
type ImportValidationResult struct {
    TotalTracks      int
    AudiobookTracks  int
    FilesFound       int
    FilesMissing     int
    MissingPaths     []string
    DuplicateHashes  map[string][]string // hash -> list of titles
}

func validateImport(library *ITunesLibrary) (*ImportValidationResult, error) {
    result := &ImportValidationResult{
        TotalTracks:     len(library.Tracks),
        MissingPaths:    []string{},
        DuplicateHashes: make(map[string][]string),
    }

    for _, track := range library.Tracks {
        if !isAudiobook(track) {
            continue
        }
        result.AudiobookTracks++

        path, err := decodeITunesLocation(track.Location)
        if err != nil {
            result.MissingPaths = append(result.MissingPaths, track.Location)
            continue
        }

        if _, err := os.Stat(path); os.IsNotExist(err) {
            result.FilesMissing++
            result.MissingPaths = append(result.MissingPaths, path)
        } else {
            result.FilesFound++

            // Check for duplicates by hash
            hash := computeFileHash(path)
            if existing, ok := result.DuplicateHashes[hash]; ok {
                result.DuplicateHashes[hash] = append(existing, track.Name)
            } else {
                result.DuplicateHashes[hash] = []string{track.Name}
            }
        }
    }

    return result, nil
}
```

### Phase 3: Import Execution

**Import Modes**:

1. **Import as Organized** (files already in desired location)
   - Creates database entries with `library_state = "organized"`
   - Sets `file_path` to current location
   - No file operations performed

2. **Import for Organization** (files need to be organized)
   - Creates database entries with `library_state = "import"`
   - Sets `file_path` to current iTunes location
   - User can then run organize operation to move files

3. **Import and Organize** (immediate organization)
   - Creates database entries
   - Triggers organize operation for each audiobook
   - Moves/copies files to organized structure

**Import Process**:

```go
type ImportOptions struct {
    LibraryPath      string
    ImportMode       string // "organized", "import", "organize"
    PreserveLocation bool   // Keep files in iTunes location
    ImportPlaylists  bool   // Import playlists as tags
    SkipDuplicates   bool   // Skip books already in library (by hash)
}

func importFromITunes(opts ImportOptions) (*ImportResult, error) {
    // 1. Parse iTunes library
    library, err := parseITunesLibrary(opts.LibraryPath)
    if err != nil {
        return nil, err
    }

    // 2. Validate files
    validation, err := validateImport(library)
    if err != nil {
        return nil, err
    }

    // 3. Import each audiobook
    result := &ImportResult{
        TotalProcessed: 0,
        Imported:       0,
        Skipped:        0,
        Failed:         0,
        Errors:         []string{},
    }

    for _, track := range library.Tracks {
        if !isAudiobook(track) {
            continue
        }

        result.TotalProcessed++

        // Convert iTunes track to audiobook
        audiobook, err := convertITunesTrack(track, opts)
        if err != nil {
            result.Failed++
            result.Errors = append(result.Errors, err.Error())
            continue
        }

        // Check for duplicates
        if opts.SkipDuplicates {
            existing, _ := database.GlobalStore.GetBookByHash(audiobook.FileHash)
            if existing != nil {
                result.Skipped++
                continue
            }
        }

        // Save to database
        if err := database.GlobalStore.CreateBook(audiobook); err != nil {
            result.Failed++
            result.Errors = append(result.Errors, err.Error())
            continue
        }

        result.Imported++

        // Import playlists as tags
        if opts.ImportPlaylists {
            tags := extractPlaylistTags(track.TrackID, library.Playlists)
            // Apply tags to audiobook
        }
    }

    return result, nil
}
```

---

## Part 4: UI Implementation

### Settings Page - iTunes Import Section

**Location**: Settings → Import → iTunes Library

**UI Components**:

```typescript
interface ITunesImportSettings {
  libraryPath: string;
  importMode: 'organized' | 'import' | 'organize';
  preserveLocation: boolean;
  importPlaylists: boolean;
  skipDuplicates: boolean;
}

const ITunesImportSection: React.FC = () => {
  const [settings, setSettings] = useState<ITunesImportSettings>({
    libraryPath: '',
    importMode: 'import',
    preserveLocation: false,
    importPlaylists: true,
    skipDuplicates: true,
  });

  const [validationResult, setValidationResult] = useState<ValidationResult | null>(null);
  const [importing, setImporting] = useState(false);
  const [importResult, setImportResult] = useState<ImportResult | null>(null);

  return (
    <Card>
      <CardHeader title="iTunes Library Import" />
      <CardContent>
        <Typography variant="body2" gutterBottom>
          Import your entire iTunes library with all metadata, play counts, ratings, and bookmarks preserved.
        </Typography>

        {/* Step 1: Select iTunes Library file */}
        <TextField
          label="iTunes Library Path"
          value={settings.libraryPath}
          onChange={(e) => setSettings({ ...settings, libraryPath: e.target.value })}
          fullWidth
          helperText="Path to iTunes Library.xml or iTunes Music Library.xml"
          InputProps={{
            endAdornment: (
              <IconButton onClick={handleBrowseFile}>
                <FolderOpenIcon />
              </IconButton>
            ),
          }}
        />

        {/* Step 2: Configure import options */}
        <FormControl component="fieldset">
          <FormLabel>Import Mode</FormLabel>
          <RadioGroup
            value={settings.importMode}
            onChange={(e) => setSettings({ ...settings, importMode: e.target.value as any })}
          >
            <FormControlLabel
              value="organized"
              control={<Radio />}
              label="Import as Organized (files already in place)"
            />
            <FormControlLabel
              value="import"
              control={<Radio />}
              label="Import for Organization (will organize later)"
            />
            <FormControlLabel
              value="organize"
              control={<Radio />}
              label="Import and Organize Now (move files immediately)"
            />
          </RadioGroup>
        </FormControl>

        <FormControlLabel
          control={
            <Checkbox
              checked={settings.importPlaylists}
              onChange={(e) => setSettings({ ...settings, importPlaylists: e.target.checked })}
            />
          }
          label="Import playlists as tags"
        />

        <FormControlLabel
          control={
            <Checkbox
              checked={settings.skipDuplicates}
              onChange={(e) => setSettings({ ...settings, skipDuplicates: e.target.checked })}
            />
          }
          label="Skip books already in library (by file hash)"
        />

        {/* Step 3: Validate import */}
        <Button
          variant="outlined"
          onClick={handleValidate}
          disabled={!settings.libraryPath}
        >
          Validate Import
        </Button>

        {/* Validation results */}
        {validationResult && (
          <Alert severity={validationResult.filesMissing > 0 ? 'warning' : 'success'}>
            <AlertTitle>Validation Results</AlertTitle>
            <Typography variant="body2">
              Found {validationResult.filesFound} audiobooks, {validationResult.filesMissing} missing files
            </Typography>
            {validationResult.filesMissing > 0 && (
              <Button onClick={() => showMissingFiles(validationResult.missingPaths)}>
                View Missing Files
              </Button>
            )}
          </Alert>
        )}

        {/* Step 4: Execute import */}
        <Button
          variant="contained"
          onClick={handleImport}
          disabled={!validationResult || importing}
        >
          {importing ? 'Importing...' : 'Import Library'}
        </Button>

        {/* Import results */}
        {importResult && (
          <Alert severity="success">
            <AlertTitle>Import Complete</AlertTitle>
            <Typography variant="body2">
              Imported {importResult.imported} audiobooks
              {importResult.skipped > 0 && `, skipped ${importResult.skipped} duplicates`}
              {importResult.failed > 0 && `, ${importResult.failed} failed`}
            </Typography>
          </Alert>
        )}
      </CardContent>
    </Card>
  );
};
```

---

## Part 5: Write-Back Support (BONUS)

### Update iTunes Library After Organization

**Goal**: Update iTunes with new file paths after organizing audiobooks

**Approach**:

1. Read original iTunes Library.xml
2. Parse to in-memory structure
3. Update `<key>Location</key>` values for organized audiobooks
4. Write back to iTunes Library.xml

**Implementation**:

```go
func updateITunesLibrary(iTunesPath string, updates map[string]string) error {
    // updates maps iTunes Persistent ID -> new file path

    // 1. Read original XML
    data, err := os.ReadFile(iTunesPath)
    if err != nil {
        return err
    }

    // 2. Parse to library structure
    var library ITunesLibrary
    if err := plist.Unmarshal(data, &library); err != nil {
        return err
    }

    // 3. Update locations
    for persistentID, newPath := range updates {
        for trackID, track := range library.Tracks {
            if track.PersistentID == persistentID {
                // Encode new path as file:// URL
                encodedPath := "file://localhost" + url.PathEscape(newPath)
                library.Tracks[trackID].Location = encodedPath
            }
        }
    }

    // 4. Write back
    newData, err := plist.Marshal(&library, plist.XMLFormat)
    if err != nil {
        return err
    }

    // 5. Backup original
    backupPath := iTunesPath + ".backup." + time.Now().Format("20060102-150405")
    if err := os.Rename(iTunesPath, backupPath); err != nil {
        return err
    }

    // 6. Write new file
    if err := os.WriteFile(iTunesPath, newData, 0644); err != nil {
        // Restore backup on error
        os.Rename(backupPath, iTunesPath)
        return err
    }

    return nil
}
```

**Safety Measures**:

1. ✅ Always create backup before modifying
2. ✅ Validate XML before writing
3. ✅ Atomic write (write to temp, then rename)
4. ✅ Restore backup on any error
5. ✅ User confirmation before write-back

**UI Implementation**:

```typescript
const ITunesWriteBackDialog: React.FC = ({ audiobookUpdates }) => {
  return (
    <Dialog open={open}>
      <DialogTitle>Update iTunes Library?</DialogTitle>
      <DialogContent>
        <Alert severity="info">
          This will update your iTunes library with the new file locations for {audiobookUpdates.length} audiobooks.
        </Alert>
        <Typography variant="body2" gutterBottom>
          A backup will be created at: iTunes Library.xml.backup.{timestamp}
        </Typography>
        <FormControlLabel
          control={<Checkbox checked={createBackup} onChange={...} />}
          label="Create backup before updating (recommended)"
        />
      </DialogContent>
      <DialogActions>
        <Button onClick={onCancel}>Cancel</Button>
        <Button variant="contained" onClick={handleWriteBack}>
          Update iTunes Library
        </Button>
      </DialogActions>
    </Dialog>
  );
};
```

---

## Part 6: API Endpoints

### Import Endpoints

```
POST   /api/v1/itunes/validate
POST   /api/v1/itunes/import
POST   /api/v1/itunes/write-back
GET    /api/v1/itunes/import-status/:id
```

### Validate Import

**Request**:

```json
{
  "library_path": "/Users/username/Music/iTunes/iTunes Music Library.xml"
}
```

**Response**:

```json
{
  "total_tracks": 15000,
  "audiobook_tracks": 523,
  "files_found": 520,
  "files_missing": 3,
  "missing_paths": [
    "/Users/username/Music/iTunes/Audiobooks/Missing Book 1.m4b",
    "/Users/username/Music/iTunes/Audiobooks/Missing Book 2.m4b"
  ],
  "duplicate_count": 5,
  "estimated_import_time": "2-3 minutes"
}
```

### Import Library

**Request**:

```json
{
  "library_path": "/Users/username/Music/iTunes/iTunes Music Library.xml",
  "import_mode": "import",
  "preserve_location": false,
  "import_playlists": true,
  "skip_duplicates": true
}
```

**Response**:

```json
{
  "operation_id": "itunes-import-abc123",
  "status": "running",
  "message": "iTunes library import started"
}
```

### Write-Back

**Request**:

```json
{
  "library_path": "/Users/username/Music/iTunes/iTunes Music Library.xml",
  "audiobook_updates": [
    {
      "itunes_persistent_id": "1234567890ABCDEF",
      "old_path": "/Users/username/Music/iTunes/Audiobooks/The Hobbit.m4b",
      "new_path": "/Users/username/Audiobooks/J.R.R. Tolkien/The Hobbit (2012)/The Hobbit.m4b"
    }
  ],
  "create_backup": true
}
```

**Response**:

```json
{
  "success": true,
  "updated_count": 520,
  "backup_path": "/Users/username/Music/iTunes/iTunes Music Library.xml.backup.20260125-143000",
  "message": "iTunes library updated successfully"
}
```

---

## Part 7: Implementation Plan

### Phase 1: Core Import (6-8 hours)

**Tasks**:

1. [ ] Create iTunes library parser (`internal/itunes/parser.go`) - 2 hours
2. [ ] Add database migration for iTunes fields (migration 11) - 30 min
3. [ ] Implement file discovery and validation - 1 hour
4. [ ] Create import service (`internal/itunes/import.go`) - 2 hours
5. [ ] Add API endpoints (`/api/v1/itunes/*`) - 1 hour
6. [ ] Add comprehensive tests - 1.5 hours

### Phase 2: UI Implementation (3-4 hours)

**Tasks**:

1. [ ] Create iTunes import settings section - 1.5 hours
2. [ ] Build validation results display - 1 hour
3. [ ] Implement import progress monitoring - 1 hour
4. [ ] Add E2E tests for import workflow - 0.5 hours

### Phase 3: Write-Back Support (2-3 hours)

**Tasks**:

1. [ ] Implement iTunes XML write-back - 1.5 hours
2. [ ] Add safety measures (backup, validation) - 1 hour
3. [ ] Create write-back UI dialog - 0.5 hours

### Total Estimated Time: 11-15 hours

---

## Part 8: Testing Strategy

### Unit Tests

```go
func TestParseITunesLibrary(t *testing.T) {
    // Test parsing valid iTunes Library.xml
    // Test handling malformed XML
    // Test extracting audiobook tracks
}

func TestDecodeITunesLocation(t *testing.T) {
    // Test URL decoding
    // Test file:// prefix removal
    // Test Windows path handling
    // Test special characters
}

func TestValidateImport(t *testing.T) {
    // Test file discovery
    // Test missing file detection
    // Test duplicate detection by hash
}

func TestImportFromITunes(t *testing.T) {
    // Test import as organized
    // Test import for organization
    // Test skip duplicates
    // Test playlist import as tags
}

func TestWriteBackITunesLibrary(t *testing.T) {
    // Test updating locations
    // Test backup creation
    // Test atomic write
    // Test error rollback
}
```

### E2E Tests

```typescript
describe('iTunes Import', () => {
  test('validates iTunes library', async ({ page }) => {
    // GIVEN: iTunes Library.xml with 100 audiobooks
    // WHEN: User uploads library file and clicks Validate
    // THEN: Shows validation results (files found/missing)
  });

  test('imports iTunes library as organized', async ({ page }) => {
    // GIVEN: Valid iTunes library
    // WHEN: User selects "Import as Organized" and confirms
    // THEN: Audiobooks imported with organized state
    // AND: Play counts, ratings, bookmarks preserved
  });

  test('imports playlists as tags', async ({ page }) => {
    // GIVEN: iTunes library with playlists
    // WHEN: User enables "Import playlists" and imports
    // THEN: Audiobooks have tags matching iTunes playlists
  });

  test('writes back to iTunes after organize', async ({ page }) => {
    // GIVEN: Imported audiobooks, some organized to new paths
    // WHEN: User clicks "Update iTunes Library"
    // THEN: Confirmation dialog appears
    // WHEN: User confirms
    // THEN: iTunes Library.xml updated with new paths
    // AND: Backup created
  });
});
```

---

## Part 9: User Documentation

### iTunes Import Quick Start

**Step 1**: Locate your iTunes Library file

- macOS: Open Finder → Go → Home → Music → iTunes → `iTunes Music Library.xml`
- Windows: Open File Explorer →
  `C:\Users\[Your Name]\Music\iTunes\iTunes Music Library.xml`

**Step 2**: Import to audiobook-organizer

1. Open audiobook-organizer web interface
2. Navigate to Settings → Import → iTunes Library
3. Click "Browse" and select your iTunes Library.xml
4. Click "Validate Import" to check files
5. Choose import mode:
   - **Import as Organized**: Files are already where you want them
   - **Import for Organization**: You'll organize files later
   - **Import and Organize Now**: Move files immediately
6. Click "Import Library"

**Step 3** (Optional): Update iTunes with new file locations

1. After organizing audiobooks, go to Settings → Import → iTunes Library
2. Click "Write Back to iTunes"
3. Confirm backup creation
4. Click "Update iTunes Library"

**Step 4**: Verify in iTunes

1. Open iTunes/Music app
2. Check that audiobooks show correct file locations
3. Verify play counts and ratings are preserved

---

## Part 10: Success Metrics

### Import Accuracy

- [ ] 100% of iTunes audiobooks discovered (if files exist)
- [ ] 100% of metadata preserved (title, author, year, genre)
- [ ] 100% of playback data preserved (play count, rating, bookmark)
- [ ] 100% of playlists converted to tags

### Performance

- [ ] Import of 500 audiobooks completes in < 5 minutes
- [ ] Validation of 500 audiobooks completes in < 30 seconds

### Safety

- [ ] Zero data loss during import
- [ ] Automatic backup before iTunes write-back
- [ ] Rollback on any error during write-back

### User Experience

- [ ] Clear validation feedback (files found/missing)
- [ ] Progress indicator during import
- [ ] Confirmation dialogs for destructive operations

---

## Conclusion

This iTunes import feature is **critical for switching from iTunes to
audiobook-organizer** as it preserves years of listening history (play counts,
ratings, bookmarks) while enabling seamless file organization.

**Priority**: P0 - Must have for MVP (personal blocker)

**Estimated Implementation**: 11-15 hours total

- Core import: 6-8 hours
- UI: 3-4 hours
- Write-back: 2-3 hours

**Next Steps**:

1. Review and approve specification
2. Create database migration 11 (iTunes fields)
3. Implement iTunes library parser
4. Build import service and API
5. Create UI components
6. Add comprehensive tests
7. (Optional) Implement write-back support

---

_Specification created_: 2026-01-25 _Status_: Ready for implementation _Owner_:
To be assigned _Dependencies_: None - can start immediately
