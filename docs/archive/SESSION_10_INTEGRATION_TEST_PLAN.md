# Session 10 Plan: Real Integration Tests for iTunes, Scanner, Organizer, Metadata

**Date:** Feb 14, 2026
**Goal:** Add real integration tests (not mock-based) for the four critical subsystems: iTunes import, file scanning, file organization, and metadata fetching.
**Context:** Coverage is at 80.1% but critical backend flows (iTunes handlers, auto-organize, metadata fetch) are untested with real data. E2E tests mock API responses, so the actual backend logic is unverified.

---

## Table of Contents

1. [Architecture Overview](#1-architecture-overview)
2. [Test Infrastructure to Build](#2-test-infrastructure-to-build)
3. [iTunes Integration Tests](#3-itunes-integration-tests)
4. [Scanner Integration Tests](#4-scanner-integration-tests)
5. [Organizer Integration Tests](#5-organizer-integration-tests)
6. [Metadata Fetch Integration Tests](#6-metadata-fetch-integration-tests)
7. [End-to-End Workflow Tests](#7-end-to-end-workflow-tests)
8. [File Listing & Modifications](#8-file-listing--modifications)
9. [Verification](#9-verification)

---

## 1. Architecture Overview

### Data Flow

```
iTunes XML ──→ ParseLibrary() ──→ []Track
                                    │
                                    ▼
                              ConvertTrack() ──→ *models.Audiobook
                                    │
                                    ▼
                         assignAuthorAndSeries() ──→ DB: CreateAuthor/CreateSeries
                                    │
                                    ▼
                            CreateBook() ──→ DB insert
                                    │
                                    ▼ (if organize mode)
                         organizeImportedBook() ──→ Organizer.OrganizeBook()
                                    │                    │
                                    ▼                    ▼
                            UpdateBook()          copyFile/hardlink/symlink
                                    │
                                    ▼ (optional)
                         itunes.WriteBack() ──→ Update iTunes XML locations

Scanner flow:
  ScanDirectoryParallel() ──→ []Book (file paths only)
            │
            ▼
  ProcessBooksParallel() ──→ ExtractMetadata() + saveBookToDatabase()
            │
            ▼ (if auto-organize)
  autoOrganizeScannedBooks() ──→ Organizer.OrganizeBook() + UpdateBook()

Metadata flow:
  FetchMetadataForBook(id) ──→ GetBookByID()
            │
            ▼
  OpenLibraryClient.SearchByTitle() ──→ HTTP GET openlibrary.org
            │
            ▼
  applyMetadataToBook() ──→ UpdateBook()
```

### Key Dependencies

| Component | Needs Real DB | Needs Real Files | Needs External API | Needs Config |
|-----------|:---:|:---:|:---:|:---:|
| iTunes Import | Yes | Yes (audio files) | No | Yes (RootDir) |
| Scanner | Yes | Yes (directory tree) | No (AI optional) | Yes (extensions, excludes) |
| Organizer | Yes | Yes (source files) | No | Yes (patterns, strategy) |
| Metadata Fetch | Yes | No | Yes (OpenLibrary) | No |

### Existing Test Fixtures

| Path | Description |
|------|-------------|
| `internal/itunes/testdata/test_library.xml` | 4 tracks: 2 audiobooks, 1 music, 1 spoken word |
| `testdata/itunes/iTunes Music Library.xml` | Real library with "Children of Time" (56 tracks) |
| `testdata/fixtures/test_sample.mp3` | Minimal MP3 (748 bytes, 0.1s) |
| `testdata/fixtures/test_sample.m4b` | Minimal M4B (1.4KB, 0.1s) |
| `testdata/fixtures/test_sample.flac` | Minimal FLAC (9.3KB, 0.1s) |
| `testdata/audio/librivox/` | ~40 MP3 + ~10 M4B/M4A real audiobook files |

### Existing Test Helpers

| Helper | File | What It Does |
|--------|------|-------------|
| `setupTestServer(t)` | `server_test.go` | Real SQLite + migrations + operation queue + event hub |
| `withTempBooks(t, names)` | `scanner_test.go` | Creates temp files with given names |
| `copyFixture(t, name)` | `metadata/metadata_test.go` | Copies Git LFS fixture to temp dir |

---

## 2. Test Infrastructure to Build

### 2.1 Shared Integration Test Helper

**File:** `internal/testutil/integration.go`

```go
package testutil

import (
    "os"
    "path/filepath"
    "testing"

    "github.com/jdfalk/audiobook-organizer/internal/config"
    "github.com/jdfalk/audiobook-organizer/internal/database"
    "github.com/jdfalk/audiobook-organizer/internal/operations"
    "github.com/jdfalk/audiobook-organizer/internal/realtime"
    "github.com/stretchr/testify/require"
)

// IntegrationEnv holds all resources for an integration test.
type IntegrationEnv struct {
    Store     database.Store
    RootDir   string   // organized output directory
    ImportDir string   // simulated import folder
    TempDir   string   // general temp space
    T         *testing.T
}

// SetupIntegration creates a real SQLite database, temp directories,
// and configures globals. Call cleanup with defer.
func SetupIntegration(t *testing.T) (*IntegrationEnv, func()) {
    t.Helper()

    tmpBase := t.TempDir()
    dbPath := filepath.Join(tmpBase, "test.db")
    rootDir := filepath.Join(tmpBase, "library")
    importDir := filepath.Join(tmpBase, "import")

    require.NoError(t, os.MkdirAll(rootDir, 0755))
    require.NoError(t, os.MkdirAll(importDir, 0755))

    store, err := database.NewSQLiteStore(dbPath)
    require.NoError(t, err)

    err = database.RunMigrations(store)
    require.NoError(t, err)

    database.GlobalStore = store

    queue := operations.NewOperationQueue(store, 2)
    operations.GlobalQueue = queue

    hub := realtime.NewEventHub()
    realtime.SetGlobalHub(hub)

    // Configure for tests
    config.AppConfig = config.Config{
        DatabaseType:        "sqlite",
        DatabasePath:        dbPath,
        RootDir:             rootDir,
        EnableSQLite:        true,
        OrganizationStrategy: "copy",
        FolderNamingPattern: "{author}/{title}",
        FileNamingPattern:   "{title}",
        SupportedExtensions: []string{".m4b", ".mp3", ".m4a", ".flac", ".ogg"},
        AutoOrganize:        false, // off by default, tests enable as needed
    }

    env := &IntegrationEnv{
        Store:     store,
        RootDir:   rootDir,
        ImportDir: importDir,
        TempDir:   tmpBase,
        T:         t,
    }

    cleanup := func() {
        store.Close()
        _ = queue.Shutdown(time.Second * 2)
    }

    return env, cleanup
}

// CreateFakeAudiobook creates a minimal audiobook file in the given directory.
// Returns the full file path.
func (env *IntegrationEnv) CreateFakeAudiobook(dir, filename string) string {
    env.T.Helper()
    path := filepath.Join(dir, filename)
    require.NoError(env.T, os.MkdirAll(filepath.Dir(path), 0755))
    // Write enough bytes to be a plausible file (use fixture if available)
    require.NoError(env.T, os.WriteFile(path, []byte("fake-audiobook-data-"+filename), 0644))
    return path
}

// CopyFixture copies a real audio fixture to the target directory.
// Fixtures are in testdata/fixtures/ (mp3, m4b, flac).
func (env *IntegrationEnv) CopyFixture(fixtureName, targetDir, targetName string) string {
    env.T.Helper()
    srcPath := filepath.Join(FindRepoRoot(env.T), "testdata", "fixtures", fixtureName)
    dstPath := filepath.Join(targetDir, targetName)
    require.NoError(env.T, os.MkdirAll(filepath.Dir(dstPath), 0755))
    data, err := os.ReadFile(srcPath)
    require.NoError(env.T, err)
    require.NoError(env.T, os.WriteFile(dstPath, data, 0644))
    return dstPath
}

// FindRepoRoot walks up from CWD to find go.mod.
func FindRepoRoot(t *testing.T) string {
    t.Helper()
    dir, err := os.Getwd()
    require.NoError(t, err)
    for {
        if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
            return dir
        }
        parent := filepath.Dir(dir)
        if parent == dir {
            t.Fatal("could not find repo root (go.mod)")
        }
        dir = parent
    }
}

// GenerateITunesXML creates a synthetic iTunes Library XML pointing to real files.
func GenerateITunesXML(t *testing.T, tracks []ITunesTestTrack, outputPath string) {
    t.Helper()
    // Build XML from template (see Section 3.2 for template)
    var sb strings.Builder
    sb.WriteString(itunesXMLHeader)
    for _, track := range tracks {
        sb.WriteString(fmt.Sprintf(itunesTrackTemplate,
            track.TrackID, track.TrackID, track.PersistentID,
            track.Name, track.Artist, track.AlbumArtist, track.Album,
            track.Genre, track.Kind, track.Year,
            itunes.EncodeLocation(track.FilePath),
            track.TotalTime, track.Comments,
        ))
    }
    sb.WriteString(itunesXMLFooter)
    require.NoError(t, os.WriteFile(outputPath, []byte(sb.String()), 0644))
}

type ITunesTestTrack struct {
    TrackID      int
    PersistentID string
    Name         string
    Artist       string
    AlbumArtist  string
    Album        string
    Genre        string
    Kind         string
    Year         int
    FilePath     string
    TotalTime    int // milliseconds
    Comments     string
}
```

### 2.2 iTunes XML Template Constants

```go
const itunesXMLHeader = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Major Version</key><integer>1</integer>
	<key>Minor Version</key><integer>1</integer>
	<key>Application Version</key><string>12.0</string>
	<key>Music Folder</key><string>file:///tmp/test-music/</string>
	<key>Tracks</key>
	<dict>
`

// Template: TrackID, TrackID, PersistentID, Name, Artist, AlbumArtist, Album, Genre, Kind, Year, Location, TotalTime, Comments
const itunesTrackTemplate = `		<key>%d</key>
		<dict>
			<key>Track ID</key><integer>%d</integer>
			<key>Persistent ID</key><string>%s</string>
			<key>Name</key><string>%s</string>
			<key>Artist</key><string>%s</string>
			<key>Album Artist</key><string>%s</string>
			<key>Album</key><string>%s</string>
			<key>Genre</key><string>%s</string>
			<key>Kind</key><string>%s</string>
			<key>Year</key><integer>%d</integer>
			<key>Location</key><string>%s</string>
			<key>Total Time</key><integer>%d</integer>
			<key>Comments</key><string>%s</string>
		</dict>
`

const itunesXMLFooter = `	</dict>
	<key>Playlists</key>
	<array>
	</array>
</dict>
</plist>
`
```

### 2.3 Mock HTTP Server for OpenLibrary

```go
// MockOpenLibraryServer creates an httptest.Server that mimics OpenLibrary API.
func MockOpenLibraryServer(t *testing.T, responses map[string]string) *httptest.Server {
    t.Helper()
    return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Match by path pattern
        for pattern, body := range responses {
            if strings.Contains(r.URL.String(), pattern) {
                w.Header().Set("Content-Type", "application/json")
                w.Write([]byte(body))
                return
            }
        }
        http.NotFound(w, r)
    }))
}

// Standard OpenLibrary search response for "The Hobbit"
const OpenLibraryHobbitResponse = `{
    "numFound": 1,
    "start": 0,
    "docs": [{
        "title": "The Hobbit",
        "author_name": ["J.R.R. Tolkien"],
        "first_publish_year": 1937,
        "publisher": ["Houghton Mifflin"],
        "language": ["eng"],
        "isbn": ["0618260307"]
    }]
}`
```

---

## 3. iTunes Integration Tests

**File:** `internal/server/itunes_integration_test.go`

### 3.1 Test: Full iTunes Import Workflow

Tests the complete flow: parse XML → validate → import → verify DB state.

```go
func TestITunesImport_FullWorkflow(t *testing.T) {
    env, cleanup := testutil.SetupIntegration(t)
    defer cleanup()

    // Create fake audiobook files matching test_library.xml track locations
    hobbitPath := env.CreateFakeAudiobook(env.ImportDir, "The Hobbit.m4b")
    dunePath := env.CreateFakeAudiobook(env.ImportDir, "Dune.mp3")
    artOfWarPath := env.CreateFakeAudiobook(env.ImportDir, "The Art of War.m4b")

    // Generate iTunes XML pointing to our fake files
    xmlPath := filepath.Join(env.TempDir, "iTunes Library.xml")
    testutil.GenerateITunesXML(t, []testutil.ITunesTestTrack{
        {TrackID: 100, PersistentID: "ABCD1234", Name: "The Hobbit",
         Artist: "J.R.R. Tolkien", AlbumArtist: "Rob Inglis",
         Album: "Middle-earth, Book 1", Genre: "Audiobook", Kind: "Audiobook",
         Year: 1997, FilePath: hobbitPath, TotalTime: 36000000, Comments: "Unabridged"},
        {TrackID: 200, PersistentID: "WXYZ9876", Name: "Dune",
         Artist: "Frank Herbert", Album: "Dune Chronicles",
         Genre: "Audiobooks", Kind: "MPEG audio file",
         Year: 1965, FilePath: dunePath, TotalTime: 72000000},
        {TrackID: 300, PersistentID: "ROCK1234", Name: "Bohemian Rhapsody",
         Artist: "Queen", Genre: "Rock", Kind: "MPEG audio file",
         Year: 1975, FilePath: "/nonexistent/queen.mp3", TotalTime: 355000},
        {TrackID: 400, PersistentID: "SPKN4567", Name: "The Art of War",
         Artist: "Sun Tzu", Genre: "Philosophy", Kind: "Spoken Word",
         Year: -500, FilePath: artOfWarPath, TotalTime: 7200000},
    }, xmlPath)

    // Step 1: Validate import
    validationResult, err := itunes.ValidateImport(itunes.ImportOptions{
        LibraryPath: xmlPath,
    })
    require.NoError(t, err)
    assert.Equal(t, 4, validationResult.TotalTracks)
    assert.Equal(t, 3, validationResult.AudiobookTracks) // Hobbit, Dune, Art of War
    assert.Equal(t, 3, validationResult.FilesFound)
    assert.Equal(t, 0, validationResult.FilesMissing) // Bohemian Rhapsody is not an audiobook

    // Step 2: Execute import via HTTP handler
    server := NewServer()
    body := `{"library_path":"` + xmlPath + `","import_mode":"import","skip_duplicates":true}`
    req := httptest.NewRequest(http.MethodPost, "/api/v1/itunes/import", strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()
    server.router.ServeHTTP(w, req)
    assert.Equal(t, http.StatusAccepted, w.Code)

    // Step 3: Wait for async import to complete
    // The operation is enqueued; we need to wait for the operation queue to process it.
    // Extract operation ID from response, poll status.
    var importResp map[string]string
    require.NoError(t, json.Unmarshal(w.Body.Bytes(), &importResp))
    opID := importResp["operation_id"]
    require.NotEmpty(t, opID)

    // Wait for operation to complete (poll with timeout)
    require.Eventually(t, func() bool {
        op, err := env.Store.GetOperationByID(opID)
        return err == nil && op != nil && (op.Status == "completed" || op.Status == "failed")
    }, 10*time.Second, 100*time.Millisecond)

    // Step 4: Verify books in database
    books, err := env.Store.GetAllBooks(100, 0)
    require.NoError(t, err)
    assert.Len(t, books, 3) // 3 audiobooks, not Bohemian Rhapsody

    // Verify individual book fields
    hobbitBook, err := env.Store.GetBookByFilePath(hobbitPath)
    require.NoError(t, err)
    require.NotNil(t, hobbitBook)
    assert.Equal(t, "The Hobbit", hobbitBook.Title)
    assert.Equal(t, 36000, hobbitBook.Duration) // TotalTime/1000
    assert.NotNil(t, hobbitBook.ITunesPersistentID)
    assert.Equal(t, "ABCD1234", *hobbitBook.ITunesPersistentID)
    assert.NotNil(t, hobbitBook.AuthorID) // Author was created

    // Verify author was created
    author, err := env.Store.GetAuthorByName("J.R.R. Tolkien")
    require.NoError(t, err)
    require.NotNil(t, author)
    assert.Equal(t, *hobbitBook.AuthorID, author.ID)

    // Verify series was created from Album field
    // "Middle-earth, Book 1" → series "Middle-earth"
    // (depends on extractSeriesName logic)
}
```

### 3.2 Test: iTunes Import with Organize Mode

```go
func TestITunesImport_OrganizeMode(t *testing.T) {
    env, cleanup := testutil.SetupIntegration(t)
    defer cleanup()

    bookPath := env.CopyFixture("test_sample.m4b", env.ImportDir, "The Hobbit.m4b")

    xmlPath := filepath.Join(env.TempDir, "iTunes Library.xml")
    testutil.GenerateITunesXML(t, []testutil.ITunesTestTrack{
        {TrackID: 100, PersistentID: "ABCD1234", Name: "The Hobbit",
         Artist: "J.R.R. Tolkien", Genre: "Audiobook", Kind: "Audiobook",
         FilePath: bookPath, TotalTime: 100000},
    }, xmlPath)

    // Import with organize mode (should copy file to RootDir)
    // Call executeITunesImport directly (it's unexported, so use HTTP)
    server := NewServer()
    body := fmt.Sprintf(`{"library_path":"%s","import_mode":"organize","skip_duplicates":false}`, xmlPath)
    req := httptest.NewRequest(http.MethodPost, "/api/v1/itunes/import", strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()
    server.router.ServeHTTP(w, req)
    assert.Equal(t, http.StatusAccepted, w.Code)

    // Wait for completion
    var resp map[string]string
    json.Unmarshal(w.Body.Bytes(), &resp)
    require.Eventually(t, func() bool {
        op, _ := env.Store.GetOperationByID(resp["operation_id"])
        return op != nil && op.Status == "completed"
    }, 10*time.Second, 100*time.Millisecond)

    // Verify file was organized
    book, err := env.Store.GetBookByFilePath(bookPath) // original path
    // Book path should have changed to RootDir
    if book == nil {
        // Search all books to find the organized one
        books, _ := env.Store.GetAllBooks(100, 0)
        require.Len(t, books, 1)
        book = &books[0]
    }
    assert.Contains(t, book.FilePath, env.RootDir) // file moved to library
    assert.Equal(t, "organized", book.LibraryState)

    // Verify file exists at new location
    _, err = os.Stat(book.FilePath)
    assert.NoError(t, err)

    // Verify original still exists (copy strategy)
    _, err = os.Stat(bookPath)
    assert.NoError(t, err)
}
```

### 3.3 Test: iTunes Import Duplicate Handling

```go
func TestITunesImport_SkipDuplicates(t *testing.T) {
    env, cleanup := testutil.SetupIntegration(t)
    defer cleanup()

    bookPath := env.CreateFakeAudiobook(env.ImportDir, "Dune.m4b")
    xmlPath := filepath.Join(env.TempDir, "iTunes Library.xml")
    testutil.GenerateITunesXML(t, []testutil.ITunesTestTrack{
        {TrackID: 200, PersistentID: "WXYZ9876", Name: "Dune",
         Artist: "Frank Herbert", Genre: "Audiobook", Kind: "Audiobook",
         FilePath: bookPath, TotalTime: 50000},
    }, xmlPath)

    // Import once
    server := NewServer()
    importOnce := func() int {
        body := fmt.Sprintf(`{"library_path":"%s","import_mode":"import","skip_duplicates":true}`, xmlPath)
        req := httptest.NewRequest(http.MethodPost, "/api/v1/itunes/import", strings.NewReader(body))
        req.Header.Set("Content-Type", "application/json")
        w := httptest.NewRecorder()
        server.router.ServeHTTP(w, req)
        var resp map[string]string
        json.Unmarshal(w.Body.Bytes(), &resp)
        require.Eventually(t, func() bool {
            op, _ := env.Store.GetOperationByID(resp["operation_id"])
            return op != nil && op.Status == "completed"
        }, 10*time.Second, 100*time.Millisecond)
        books, _ := env.Store.GetAllBooks(100, 0)
        return len(books)
    }

    count1 := importOnce()
    assert.Equal(t, 1, count1)

    count2 := importOnce()
    assert.Equal(t, 1, count2) // Should NOT have created a duplicate
}
```

### 3.4 Test: iTunes Write-Back

```go
func TestITunesWriteBack(t *testing.T) {
    env, cleanup := testutil.SetupIntegration(t)
    defer cleanup()

    // Create a book in DB that was imported from iTunes
    origPath := env.CreateFakeAudiobook(env.ImportDir, "The Hobbit.m4b")
    newPath := filepath.Join(env.RootDir, "Tolkien", "The Hobbit", "The Hobbit.m4b")
    require.NoError(t, os.MkdirAll(filepath.Dir(newPath), 0755))
    require.NoError(t, copyFileForTest(t, origPath, newPath))

    persistentID := "ABCD1234EFGH5678"
    book := &database.Book{
        Title:              "The Hobbit",
        FilePath:           newPath,
        Format:             ".m4b",
        ITunesPersistentID: &persistentID,
    }
    created, err := env.Store.CreateBook(book)
    require.NoError(t, err)

    // Generate iTunes XML with original path
    xmlPath := filepath.Join(env.TempDir, "iTunes Library.xml")
    testutil.GenerateITunesXML(t, []testutil.ITunesTestTrack{
        {TrackID: 100, PersistentID: persistentID, Name: "The Hobbit",
         Artist: "J.R.R. Tolkien", Genre: "Audiobook", Kind: "Audiobook",
         FilePath: origPath, TotalTime: 36000000},
    }, xmlPath)

    // Execute write-back via HTTP
    server := NewServer()
    body := fmt.Sprintf(`{"library_path":"%s","audiobook_ids":["%s"],"create_backup":true}`, xmlPath, created.ID)
    req := httptest.NewRequest(http.MethodPost, "/api/v1/itunes/writeback", strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()
    server.router.ServeHTTP(w, req)
    assert.Equal(t, http.StatusOK, w.Code)

    // Verify iTunes library was updated
    updatedLib, err := itunes.ParseLibrary(xmlPath)
    require.NoError(t, err)
    for _, track := range updatedLib.Tracks {
        if track.PersistentID == persistentID {
            decodedPath, _ := itunes.DecodeLocation(track.Location)
            assert.Equal(t, newPath, decodedPath) // Location updated to new path
        }
    }

    // Verify backup was created
    backupPattern := xmlPath + ".backup.*"
    matches, _ := filepath.Glob(filepath.Dir(xmlPath) + "/*.backup.*")
    assert.NotEmpty(t, matches)
}
```

### 3.5 Test: iTunes Validate Endpoint

```go
func TestITunesValidate_Endpoint(t *testing.T) {
    env, cleanup := testutil.SetupIntegration(t)
    defer cleanup()

    bookPath := env.CreateFakeAudiobook(env.ImportDir, "Test Book.m4b")
    xmlPath := filepath.Join(env.TempDir, "Library.xml")
    testutil.GenerateITunesXML(t, []testutil.ITunesTestTrack{
        {TrackID: 1, PersistentID: "TEST1234", Name: "Test Book",
         Artist: "Author", Genre: "Audiobook", Kind: "Audiobook",
         FilePath: bookPath, TotalTime: 10000},
        {TrackID: 2, PersistentID: "MISS5678", Name: "Missing Book",
         Artist: "Author", Genre: "Audiobook", Kind: "Audiobook",
         FilePath: "/nonexistent/missing.m4b", TotalTime: 20000},
    }, xmlPath)

    server := NewServer()
    body := fmt.Sprintf(`{"library_path":"%s"}`, xmlPath)
    req := httptest.NewRequest(http.MethodPost, "/api/v1/itunes/validate", strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()
    server.router.ServeHTTP(w, req)
    assert.Equal(t, http.StatusOK, w.Code)

    var resp map[string]interface{}
    json.Unmarshal(w.Body.Bytes(), &resp)
    assert.Equal(t, float64(2), resp["audiobook_tracks"])
    assert.Equal(t, float64(1), resp["files_found"])
    assert.Equal(t, float64(1), resp["files_missing"])
}
```

---

## 4. Scanner Integration Tests

**File:** `internal/server/scan_integration_test.go`

### 4.1 Test: Scan Directory with Real Files

```go
func TestScanService_ScanWithRealFiles(t *testing.T) {
    env, cleanup := testutil.SetupIntegration(t)
    defer cleanup()

    // Create directory structure with audiobook files
    // Author1/Book1/book1.m4b
    // Author1/Book2/book2.mp3
    // Author2/Book3/book3.m4b
    env.CopyFixture("test_sample.m4b", filepath.Join(env.ImportDir, "Tolkien", "The Hobbit"), "The Hobbit.m4b")
    env.CopyFixture("test_sample.mp3", filepath.Join(env.ImportDir, "Herbert", "Dune"), "Dune.mp3")
    env.CopyFixture("test_sample.m4b", filepath.Join(env.ImportDir, "Asimov", "Foundation"), "Foundation.m4b")

    // Add import path to DB
    _, err := env.Store.CreateImportPath(env.ImportDir, "Test Import")
    require.NoError(t, err)

    // Create scan service and execute
    svc := NewScanService(env.Store)
    scanReq := &ScanRequest{
        FolderPath:  env.ImportDir,
        ForceUpdate: false,
    }

    // Use a mock progress function
    err = svc.PerformScan(context.Background(), scanReq, func(p operations.Progress) {})
    require.NoError(t, err)

    // Verify books were created in database
    books, err := env.Store.GetAllBooks(100, 0)
    require.NoError(t, err)
    assert.Len(t, books, 3)

    // Verify metadata was extracted (at least titles from filenames)
    titles := make(map[string]bool)
    for _, b := range books {
        titles[b.Title] = true
        assert.NotEmpty(t, b.FilePath)
        assert.NotEmpty(t, b.Format)
    }
}
```

### 4.2 Test: Scan with Auto-Organize

```go
func TestScanService_AutoOrganize(t *testing.T) {
    env, cleanup := testutil.SetupIntegration(t)
    defer cleanup()

    // Enable auto-organize
    config.AppConfig.AutoOrganize = true

    env.CopyFixture("test_sample.m4b", env.ImportDir, "The Hobbit.m4b")

    svc := NewScanService(env.Store)
    err := svc.PerformScan(context.Background(), &ScanRequest{
        FolderPath: env.ImportDir,
    }, func(p operations.Progress) {})
    require.NoError(t, err)

    // Verify book was organized to RootDir
    books, err := env.Store.GetAllBooks(100, 0)
    require.NoError(t, err)
    require.Len(t, books, 1)

    book := books[0]
    assert.Contains(t, book.FilePath, env.RootDir, "book should be in library dir")
    assert.Equal(t, "organized", book.LibraryState)

    // Verify file exists at organized location
    _, err = os.Stat(book.FilePath)
    assert.NoError(t, err, "organized file should exist")
}
```

### 4.3 Test: Multi-Folder Scan via Import Paths

```go
func TestScanService_MultipleFolders(t *testing.T) {
    env, cleanup := testutil.SetupIntegration(t)
    defer cleanup()

    // Create two import directories
    dir1 := filepath.Join(env.TempDir, "import1")
    dir2 := filepath.Join(env.TempDir, "import2")
    os.MkdirAll(dir1, 0755)
    os.MkdirAll(dir2, 0755)

    env.CopyFixture("test_sample.m4b", dir1, "Book1.m4b")
    env.CopyFixture("test_sample.mp3", dir2, "Book2.mp3")

    // Register both as import paths
    _, err := env.Store.CreateImportPath(dir1, "Import 1")
    require.NoError(t, err)
    _, err = env.Store.CreateImportPath(dir2, "Import 2")
    require.NoError(t, err)

    svc := NewScanService(env.Store)
    err = svc.PerformScan(context.Background(), &ScanRequest{
        // No FolderPath → scans all import paths
        ForceUpdate: true,
    }, func(p operations.Progress) {})
    require.NoError(t, err)

    books, err := env.Store.GetAllBooks(100, 0)
    require.NoError(t, err)
    assert.Len(t, books, 2, "should find books from both import dirs")
}
```

### 4.4 Test: Scan Skips Excluded Paths

```go
func TestScanService_ExcludePatterns(t *testing.T) {
    env, cleanup := testutil.SetupIntegration(t)
    defer cleanup()

    config.AppConfig.ExcludePatterns = []string{"*.tmp", "cache"}

    env.CopyFixture("test_sample.m4b", env.ImportDir, "Good Book.m4b")
    env.CreateFakeAudiobook(env.ImportDir, "temp.tmp")
    os.MkdirAll(filepath.Join(env.ImportDir, "cache"), 0755)
    env.CopyFixture("test_sample.m4b", filepath.Join(env.ImportDir, "cache"), "cached.m4b")

    svc := NewScanService(env.Store)
    err := svc.PerformScan(context.Background(), &ScanRequest{
        FolderPath: env.ImportDir,
    }, func(p operations.Progress) {})
    require.NoError(t, err)

    books, err := env.Store.GetAllBooks(100, 0)
    require.NoError(t, err)
    assert.Len(t, books, 1, "should only find the non-excluded book")
    assert.Contains(t, books[0].Title, "Good Book")
}
```

---

## 5. Organizer Integration Tests

**File:** `internal/organizer/organizer_integration_test.go`

### 5.1 Test: Organize with Copy Strategy

```go
func TestOrganizer_CopyStrategy(t *testing.T) {
    rootDir := t.TempDir()
    importDir := t.TempDir()

    cfg := &config.Config{
        RootDir:              rootDir,
        OrganizationStrategy: "copy",
        FolderNamingPattern:  "{author}/{title}",
        FileNamingPattern:    "{title}",
    }
    org := NewOrganizer(cfg)

    // Create source file
    srcPath := filepath.Join(importDir, "test.m4b")
    os.WriteFile(srcPath, []byte("audiobook-content"), 0644)

    authorName := "J.R.R. Tolkien"
    book := &database.Book{
        Title:    "The Hobbit",
        FilePath: srcPath,
        Format:   ".m4b",
        Author:   &database.Author{Name: authorName},
    }

    newPath, err := org.OrganizeBook(book)
    require.NoError(t, err)

    // Verify target path structure
    assert.Contains(t, newPath, rootDir)
    assert.Contains(t, newPath, "J.R.R. Tolkien")
    assert.Contains(t, newPath, "The Hobbit")

    // Verify file exists at new location
    _, err = os.Stat(newPath)
    assert.NoError(t, err)

    // Verify source file still exists (copy, not move)
    _, err = os.Stat(srcPath)
    assert.NoError(t, err)

    // Verify content matches
    srcData, _ := os.ReadFile(srcPath)
    dstData, _ := os.ReadFile(newPath)
    assert.Equal(t, srcData, dstData)
}
```

### 5.2 Test: Organize with Hardlink Strategy

```go
func TestOrganizer_HardlinkStrategy(t *testing.T) {
    rootDir := t.TempDir()
    importDir := t.TempDir()

    cfg := &config.Config{
        RootDir:              rootDir,
        OrganizationStrategy: "hardlink",
        FolderNamingPattern:  "{author}",
        FileNamingPattern:    "{title}",
    }
    org := NewOrganizer(cfg)

    srcPath := filepath.Join(importDir, "test.m4b")
    os.WriteFile(srcPath, []byte("hardlink-test"), 0644)

    book := &database.Book{
        Title:    "Dune",
        FilePath: srcPath,
        Format:   ".m4b",
        Author:   &database.Author{Name: "Frank Herbert"},
    }

    newPath, err := org.OrganizeBook(book)
    require.NoError(t, err)

    // Verify hardlink: both files share same inode
    srcInfo, _ := os.Stat(srcPath)
    dstInfo, _ := os.Stat(newPath)
    assert.True(t, os.SameFile(srcInfo, dstInfo), "should be hardlinked")
}
```

### 5.3 Test: Organize with Complex Naming Patterns

```go
func TestOrganizer_ComplexPatterns(t *testing.T) {
    rootDir := t.TempDir()

    tests := []struct {
        name          string
        folderPattern string
        filePattern   string
        book          *database.Book
        wantContains  []string // substrings expected in path
    }{
        {
            name:          "all fields populated",
            folderPattern: "{author}/{series}",
            filePattern:   "{series_number} - {title}",
            book: &database.Book{
                Title:          "The Two Towers",
                FilePath:       filepath.Join(t.TempDir(), "src.m4b"),
                Format:         ".m4b",
                Author:         &database.Author{Name: "J.R.R. Tolkien"},
                Series:         &database.Series{Name: "Lord of the Rings"},
                SeriesSequence: intPtr(2),
            },
            wantContains: []string{"J.R.R. Tolkien", "Lord of the Rings", "2 - The Two Towers"},
        },
        {
            name:          "missing series falls back",
            folderPattern: "{author}/{series}",
            filePattern:   "{title}",
            book: &database.Book{
                Title:    "Standalone Novel",
                FilePath: filepath.Join(t.TempDir(), "src.m4b"),
                Format:   ".m4b",
                Author:   &database.Author{Name: "Some Author"},
                // No series → empty segment removed
            },
            wantContains: []string{"Some Author", "Standalone Novel"},
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Create source file
            os.WriteFile(tt.book.FilePath, []byte("test"), 0644)

            cfg := &config.Config{
                RootDir:              rootDir,
                OrganizationStrategy: "copy",
                FolderNamingPattern:  tt.folderPattern,
                FileNamingPattern:    tt.filePattern,
            }
            org := NewOrganizer(cfg)

            newPath, err := org.OrganizeBook(tt.book)
            require.NoError(t, err)
            for _, want := range tt.wantContains {
                assert.Contains(t, newPath, want)
            }
        })
    }
}
```

### 5.4 Test: Organize via Server Handler (Full HTTP)

```go
// In internal/server/organize_integration_test.go
func TestOrganizeService_ViaHTTP(t *testing.T) {
    env, cleanup := testutil.SetupIntegration(t)
    defer cleanup()

    // Create a book in the DB with a file outside RootDir
    srcPath := env.CopyFixture("test_sample.m4b", env.ImportDir, "Book.m4b")
    author, _ := env.Store.CreateAuthor("Test Author")
    book := &database.Book{
        Title:    "Test Book",
        FilePath: srcPath,
        Format:   ".m4b",
        AuthorID: &author.ID,
    }
    created, err := env.Store.CreateBook(book)
    require.NoError(t, err)

    // Trigger organize via HTTP
    server := NewServer()
    req := httptest.NewRequest(http.MethodPost, "/api/v1/organize", strings.NewReader("{}"))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()
    server.router.ServeHTTP(w, req)
    assert.Equal(t, http.StatusAccepted, w.Code)

    // Wait for operation
    var resp map[string]string
    json.Unmarshal(w.Body.Bytes(), &resp)
    require.Eventually(t, func() bool {
        op, _ := env.Store.GetOperationByID(resp["operation_id"])
        return op != nil && (op.Status == "completed" || op.Status == "failed")
    }, 10*time.Second, 100*time.Millisecond)

    // Verify book was organized
    updated, err := env.Store.GetBookByID(created.ID)
    require.NoError(t, err)
    assert.Contains(t, updated.FilePath, env.RootDir)
    assert.Equal(t, "organized", updated.LibraryState)
    _, err = os.Stat(updated.FilePath)
    assert.NoError(t, err)
}
```

---

## 6. Metadata Fetch Integration Tests

**File:** `internal/server/metadata_integration_test.go`

### 6.1 Test: Fetch Metadata with Mock OpenLibrary

```go
func TestMetadataFetch_WithMockAPI(t *testing.T) {
    env, cleanup := testutil.SetupIntegration(t)
    defer cleanup()

    // Create mock OpenLibrary server
    mockServer := testutil.MockOpenLibraryServer(t, map[string]string{
        "search.json": testutil.OpenLibraryHobbitResponse,
    })
    defer mockServer.Close()

    // Point OpenLibrary client to mock
    os.Setenv("OPENLIBRARY_BASE_URL", mockServer.URL)
    defer os.Unsetenv("OPENLIBRARY_BASE_URL")

    // Create a book in DB with minimal metadata
    author, _ := env.Store.CreateAuthor("J.R.R. Tolkien")
    book := &database.Book{
        Title:    "The Hobbit",
        FilePath: "/fake/hobbit.m4b",
        Format:   ".m4b",
        AuthorID: &author.ID,
    }
    created, err := env.Store.CreateBook(book)
    require.NoError(t, err)

    // Fetch metadata
    svc := NewMetadataFetchService(env.Store)
    resp, err := svc.FetchMetadataForBook(created.ID)
    require.NoError(t, err)
    assert.NotNil(t, resp)
    assert.Equal(t, "openlibrary", resp.Source)

    // Verify book was updated
    updated, err := env.Store.GetBookByID(created.ID)
    require.NoError(t, err)
    // Publisher should be populated from mock response
    assert.NotNil(t, updated.Publisher)
    assert.Equal(t, "Houghton Mifflin", *updated.Publisher)
}
```

### 6.2 Test: Metadata Fetch Fallback Chain

```go
func TestMetadataFetch_FallbackToAuthorSearch(t *testing.T) {
    env, cleanup := testutil.SetupIntegration(t)
    defer cleanup()

    // Mock: title-only search returns nothing, title+author works
    callCount := 0
    mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        callCount++
        query := r.URL.Query()
        w.Header().Set("Content-Type", "application/json")

        if query.Get("author") != "" {
            // Title + Author search succeeds
            w.Write([]byte(testutil.OpenLibraryHobbitResponse))
        } else {
            // Title-only search returns no results
            w.Write([]byte(`{"numFound":0,"start":0,"docs":[]}`))
        }
    }))
    defer mockServer.Close()

    os.Setenv("OPENLIBRARY_BASE_URL", mockServer.URL)
    defer os.Unsetenv("OPENLIBRARY_BASE_URL")

    author, _ := env.Store.CreateAuthor("J.R.R. Tolkien")
    book := &database.Book{
        Title:    "The Hobbit - Chapter 1",
        FilePath: "/fake/hobbit.m4b",
        Format:   ".m4b",
        AuthorID: &author.ID,
    }
    created, _ := env.Store.CreateBook(book)

    svc := NewMetadataFetchService(env.Store)
    resp, err := svc.FetchMetadataForBook(created.ID)
    require.NoError(t, err)
    assert.NotNil(t, resp)

    // Should have tried multiple searches before succeeding
    assert.GreaterOrEqual(t, callCount, 2, "should have tried title-only first, then title+author")
}
```

### 6.3 Test: Metadata Fetch Not Found

```go
func TestMetadataFetch_NotFound(t *testing.T) {
    env, cleanup := testutil.SetupIntegration(t)
    defer cleanup()

    // Mock returns no results for everything
    mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.Write([]byte(`{"numFound":0,"start":0,"docs":[]}`))
    }))
    defer mockServer.Close()

    os.Setenv("OPENLIBRARY_BASE_URL", mockServer.URL)
    defer os.Unsetenv("OPENLIBRARY_BASE_URL")

    book := &database.Book{
        Title:    "Completely Unknown Book XYZ123",
        FilePath: "/fake/unknown.m4b",
        Format:   ".m4b",
    }
    created, _ := env.Store.CreateBook(book)

    svc := NewMetadataFetchService(env.Store)
    _, err := svc.FetchMetadataForBook(created.ID)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "no metadata found")
}
```

---

## 7. End-to-End Workflow Tests

**File:** `internal/server/e2e_workflow_test.go`

### 7.1 Test: iTunes Import → Organize → Write-Back

```go
func TestE2E_ITunesImportOrganizeWriteBack(t *testing.T) {
    env, cleanup := testutil.SetupIntegration(t)
    defer cleanup()

    // Step 1: Create source audiobook files
    hobbitPath := env.CopyFixture("test_sample.m4b", env.ImportDir, "The Hobbit.m4b")
    dunePath := env.CopyFixture("test_sample.mp3", env.ImportDir, "Dune.mp3")

    // Step 2: Generate iTunes library XML
    xmlPath := filepath.Join(env.TempDir, "Library.xml")
    testutil.GenerateITunesXML(t, []testutil.ITunesTestTrack{
        {TrackID: 1, PersistentID: "HOBT1234", Name: "The Hobbit",
         Artist: "Tolkien", Genre: "Audiobook", Kind: "Audiobook",
         FilePath: hobbitPath, TotalTime: 36000000},
        {TrackID: 2, PersistentID: "DUNE5678", Name: "Dune",
         Artist: "Herbert", Genre: "Audiobook", Kind: "Audiobook",
         FilePath: dunePath, TotalTime: 72000000},
    }, xmlPath)

    server := NewServer()

    // Step 3: Import (non-organize mode)
    importBody := fmt.Sprintf(`{"library_path":"%s","import_mode":"import"}`, xmlPath)
    w := httptest.NewRecorder()
    server.router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/itunes/import",
        strings.NewReader(importBody)))
    require.Equal(t, http.StatusAccepted, w.Code)
    var importResp map[string]string
    json.Unmarshal(w.Body.Bytes(), &importResp)
    waitForOp(t, env.Store, importResp["operation_id"], 10*time.Second)

    // Step 4: Verify 2 books in DB
    books, _ := env.Store.GetAllBooks(100, 0)
    require.Len(t, books, 2)

    // Step 5: Organize
    w = httptest.NewRecorder()
    server.router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/organize",
        strings.NewReader("{}")))
    require.Equal(t, http.StatusAccepted, w.Code)
    var orgResp map[string]string
    json.Unmarshal(w.Body.Bytes(), &orgResp)
    waitForOp(t, env.Store, orgResp["operation_id"], 10*time.Second)

    // Step 6: Verify books moved to RootDir
    books, _ = env.Store.GetAllBooks(100, 0)
    for _, b := range books {
        assert.Contains(t, b.FilePath, env.RootDir)
        _, err := os.Stat(b.FilePath)
        assert.NoError(t, err, "organized file should exist: %s", b.FilePath)
    }

    // Step 7: Write back to iTunes
    bookIDs := make([]string, len(books))
    for i, b := range books {
        bookIDs[i] = `"` + b.ID + `"`
    }
    wbBody := fmt.Sprintf(`{"library_path":"%s","audiobook_ids":[%s],"create_backup":true}`,
        xmlPath, strings.Join(bookIDs, ","))
    w = httptest.NewRecorder()
    server.router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/itunes/writeback",
        strings.NewReader(wbBody)))
    assert.Equal(t, http.StatusOK, w.Code)

    // Step 8: Verify iTunes XML updated
    lib, _ := itunes.ParseLibrary(xmlPath)
    for _, track := range lib.Tracks {
        decoded, _ := itunes.DecodeLocation(track.Location)
        assert.Contains(t, decoded, env.RootDir, "iTunes location should point to organized path")
    }
}

// waitForOp polls until operation completes or times out.
func waitForOp(t *testing.T, store database.Store, opID string, timeout time.Duration) {
    t.Helper()
    require.Eventually(t, func() bool {
        op, err := store.GetOperationByID(opID)
        return err == nil && op != nil && (op.Status == "completed" || op.Status == "failed")
    }, timeout, 100*time.Millisecond)
}
```

### 7.2 Test: Scan → Metadata Fetch → Verify Enrichment

```go
func TestE2E_ScanAndFetchMetadata(t *testing.T) {
    env, cleanup := testutil.SetupIntegration(t)
    defer cleanup()

    // Mock OpenLibrary
    mockServer := testutil.MockOpenLibraryServer(t, map[string]string{
        "search.json": testutil.OpenLibraryHobbitResponse,
    })
    defer mockServer.Close()
    os.Setenv("OPENLIBRARY_BASE_URL", mockServer.URL)
    defer os.Unsetenv("OPENLIBRARY_BASE_URL")

    // Create audiobook file
    env.CopyFixture("test_sample.m4b", env.ImportDir, "The Hobbit.m4b")

    // Step 1: Scan
    svc := NewScanService(env.Store)
    err := svc.PerformScan(context.Background(), &ScanRequest{
        FolderPath: env.ImportDir,
    }, func(p operations.Progress) {})
    require.NoError(t, err)

    books, _ := env.Store.GetAllBooks(100, 0)
    require.Len(t, books, 1)
    bookID := books[0].ID

    // Step 2: Fetch metadata
    metaSvc := NewMetadataFetchService(env.Store)
    resp, err := metaSvc.FetchMetadataForBook(bookID)
    require.NoError(t, err)
    assert.NotNil(t, resp)

    // Step 3: Verify enrichment
    enriched, _ := env.Store.GetBookByID(bookID)
    assert.NotNil(t, enriched.Publisher, "publisher should be populated from OpenLibrary")
}
```

---

## 8. File Listing & Modifications

### New Files to Create

| File | Purpose |
|------|---------|
| `internal/testutil/integration.go` | Shared `SetupIntegration`, `CreateFakeAudiobook`, `CopyFixture`, `FindRepoRoot` |
| `internal/testutil/itunes_helpers.go` | `GenerateITunesXML`, XML template constants, `ITunesTestTrack` struct |
| `internal/testutil/mock_openlibrary.go` | `MockOpenLibraryServer`, mock response constants |
| `internal/server/itunes_integration_test.go` | Tests 3.1–3.5 |
| `internal/server/scan_integration_test.go` | Tests 4.1–4.4 |
| `internal/server/organize_integration_test.go` | Test 5.4 |
| `internal/server/metadata_integration_test.go` | Tests 6.1–6.3 |
| `internal/server/e2e_workflow_test.go` | Tests 7.1–7.2 |
| `internal/organizer/organizer_integration_test.go` | Tests 5.1–5.3 |

### Existing Files — No Modifications Needed

The integration tests are all in new files. No existing code needs to change.

### Build Tag Strategy

All integration tests run WITHOUT build tags (they use temp dirs, mock HTTP, and test fixtures — no external dependencies). They should run in normal `go test` and `make ci`.

If any tests are too slow (>30s), add `//go:build integration` and a `make test-integration` target.

---

## 9. Verification

### Run Integration Tests

```bash
# Run just the new integration tests
go test ./internal/server/ -run "TestITunesImport_|TestScanService_|TestOrganizeService_|TestMetadataFetch_|TestE2E_" -v -count=1

# Run organizer integration tests
go test ./internal/organizer/ -run "TestOrganizer_" -v -count=1

# Run all tests with coverage
go test ./internal/... -count=1 -coverprofile=/tmp/cover_integration.out
go tool cover -func=/tmp/cover_integration.out | grep ^total

# Run make ci to verify everything passes
make ci
```

### Expected Coverage Impact

These tests should significantly improve coverage for:

| Function | Current | Expected |
|----------|---------|----------|
| `executeITunesImport` | 0% | 70%+ |
| `handleITunesImport` | 0% | 80%+ |
| `handleITunesWriteBack` | 0% | 80%+ |
| `handleITunesValidate` | 55.6% | 90%+ |
| `assignAuthorAndSeries` | 0% | 100% |
| `ensureAuthorID` | 0% | 100% |
| `ensureSeriesID` | 0% | 100% |
| `organizeImportedBook` | 0% | 70%+ |
| `autoOrganizeScannedBooks` | 12.5% | 70%+ |
| `FetchMetadataForBook` | 53.6% | 90%+ |
| `organizeBooks` | 65.2% | 85%+ |
| `OrganizeBook` (organizer) | ~85% | 95%+ |

### Key Assertions Checklist

For each test, verify:

- [ ] Database state: books created/updated with correct fields
- [ ] File system state: files exist at expected paths
- [ ] Author/Series resolution: entities created or reused correctly
- [ ] iTunes metadata: PersistentID, DateAdded, Rating, Bookmark preserved
- [ ] Operation tracking: operations created and completed
- [ ] Error paths: invalid inputs return appropriate errors
- [ ] Duplicate handling: no duplicate books created on re-import
- [ ] Write-back: iTunes XML locations updated correctly
- [ ] Organize: files at target path, source preserved (copy strategy)

---

## Implementation Order

1. **`internal/testutil/`** — Build shared infrastructure first (3 files)
2. **`internal/organizer/organizer_integration_test.go`** — Pure unit-level, no server needed (tests 5.1–5.3)
3. **`internal/server/itunes_integration_test.go`** — iTunes import/validate/writeback (tests 3.1–3.5)
4. **`internal/server/scan_integration_test.go`** — Scanner service (tests 4.1–4.4)
5. **`internal/server/metadata_integration_test.go`** — Metadata fetch with mock HTTP (tests 6.1–6.3)
6. **`internal/server/organize_integration_test.go`** — Organize via HTTP (test 5.4)
7. **`internal/server/e2e_workflow_test.go`** — Full workflow tests (tests 7.1–7.2)

Estimate: ~800-1000 lines of test code across 9 new files.
