// file: cmd/commands_test.go
// version: 1.0.2
// guid: 6f5b7d78-11d8-4c1a-a150-96d2c4a1a885

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/scanner"
	"github.com/jdfalk/audiobook-organizer/internal/server"
)

// stubStore is a minimal stub implementation of database.Store for command tests
// It provides no-op implementations for all methods to avoid nil pointer panics
type stubStore struct{}

// Implement all required Store interface methods as no-ops
func (s *stubStore) Close() error { return nil }
func (s *stubStore) GetMetadataFieldStates(bookID string) ([]database.MetadataFieldState, error) {
	return []database.MetadataFieldState{}, nil
}
func (s *stubStore) UpsertMetadataFieldState(state *database.MetadataFieldState) error { return nil }
func (s *stubStore) DeleteMetadataFieldState(bookID, field string) error               { return nil }
func (s *stubStore) GetAllAuthors() ([]database.Author, error)                         { return []database.Author{}, nil }
func (s *stubStore) GetAuthorByID(id int) (*database.Author, error)                    { return nil, nil }
func (s *stubStore) GetAuthorByName(name string) (*database.Author, error)             { return nil, nil }
func (s *stubStore) CreateAuthor(name string) (*database.Author, error) {
	return &database.Author{ID: 1, Name: name}, nil
}
func (s *stubStore) GetAllSeries() ([]database.Series, error)       { return []database.Series{}, nil }
func (s *stubStore) GetSeriesByID(id int) (*database.Series, error) { return nil, nil }
func (s *stubStore) GetSeriesByName(name string, authorID *int) (*database.Series, error) {
	return nil, nil
}
func (s *stubStore) CreateSeries(name string, authorID *int) (*database.Series, error) {
	return &database.Series{ID: 1, Name: name}, nil
}
func (s *stubStore) GetAllWorks() ([]database.Work, error)                  { return []database.Work{}, nil }
func (s *stubStore) GetWorkByID(id string) (*database.Work, error)          { return nil, nil }
func (s *stubStore) CreateWork(work *database.Work) (*database.Work, error) { return work, nil }
func (s *stubStore) UpdateWork(id string, work *database.Work) (*database.Work, error) {
	return work, nil
}
func (s *stubStore) DeleteWork(id string) error { return nil }
func (s *stubStore) GetBooksByWorkID(workID string) ([]database.Book, error) {
	return []database.Book{}, nil
}
func (s *stubStore) GetAllBooks(limit, offset int) ([]database.Book, error) {
	return []database.Book{}, nil
}
func (s *stubStore) GetBookByID(id string) (*database.Book, error)              { return nil, nil }
func (s *stubStore) GetBookByFilePath(path string) (*database.Book, error)      { return nil, nil }
func (s *stubStore) GetBookByFileHash(hash string) (*database.Book, error)      { return nil, nil }
func (s *stubStore) GetBookByOriginalHash(hash string) (*database.Book, error)  { return nil, nil }
func (s *stubStore) GetBookByOrganizedHash(hash string) (*database.Book, error) { return nil, nil }
func (s *stubStore) GetDuplicateBooks() ([][]database.Book, error)              { return [][]database.Book{}, nil }
func (s *stubStore) GetBooksBySeriesID(seriesID int) ([]database.Book, error) {
	return []database.Book{}, nil
}
func (s *stubStore) GetBooksByAuthorID(authorID int) ([]database.Book, error) {
	return []database.Book{}, nil
}
func (s *stubStore) CreateBook(book *database.Book) (*database.Book, error) { return book, nil }
func (s *stubStore) UpdateBook(id string, book *database.Book) (*database.Book, error) {
	return book, nil
}
func (s *stubStore) DeleteBook(id string) error { return nil }
func (s *stubStore) SearchBooks(query string, limit, offset int) ([]database.Book, error) {
	return []database.Book{}, nil
}
func (s *stubStore) CountBooks() (int, error)                                                      { return 0, nil }
func (s *stubStore) CountAuthors() (int, error)                                                    { return 0, nil }
func (s *stubStore) CountSeries() (int, error)                                                     { return 0, nil }
func (s *stubStore) GetBookCountsByLocation(rootDir string) (int, int, error)                      { return 0, 0, nil }
func (s *stubStore) GetBookSizesByLocation(rootDir string) (int64, int64, error)                   { return 0, 0, nil }
func (s *stubStore) GetDashboardStats() (*database.DashboardStats, error) {
	return &database.DashboardStats{StateDistribution: map[string]int{}, FormatDistribution: map[string]int{}}, nil
}
func (s *stubStore) ListSoftDeletedBooks(limit, offset int, olderThan *time.Time) ([]database.Book, error) {
	return []database.Book{}, nil
}
func (s *stubStore) GetBooksByVersionGroup(groupID string) ([]database.Book, error) {
	return []database.Book{}, nil
}
func (s *stubStore) GetAllImportPaths() ([]database.ImportPath, error) {
	return []database.ImportPath{}, nil
}
func (s *stubStore) GetImportPathByID(id int) (*database.ImportPath, error)        { return nil, nil }
func (s *stubStore) GetImportPathByPath(path string) (*database.ImportPath, error) { return nil, nil }
func (s *stubStore) CreateImportPath(path, name string) (*database.ImportPath, error) {
	return &database.ImportPath{}, nil
}
func (s *stubStore) UpdateImportPath(id int, importPath *database.ImportPath) error { return nil }
func (s *stubStore) DeleteImportPath(id int) error                                  { return nil }
func (s *stubStore) CreateOperation(id, opType string, folderPath *string) (*database.Operation, error) {
	return &database.Operation{}, nil
}
func (s *stubStore) GetOperationByID(id string) (*database.Operation, error) { return nil, nil }
func (s *stubStore) GetRecentOperations(limit int) ([]database.Operation, error) {
	return []database.Operation{}, nil
}
func (s *stubStore) UpdateOperationStatus(id, status string, progress, total int, message string) error {
	return nil
}
func (s *stubStore) UpdateOperationError(id, errorMessage string) error             { return nil }
func (s *stubStore) AddOperationLog(opID, level, msg string, details *string) error { return nil }
func (s *stubStore) GetOperationLogs(opID string) ([]database.OperationLog, error) {
	return []database.OperationLog{}, nil
}
func (s *stubStore) SaveOperationState(opID string, state []byte) error  { return nil }
func (s *stubStore) GetOperationState(opID string) ([]byte, error)      { return nil, nil }
func (s *stubStore) SaveOperationParams(opID string, params []byte) error { return nil }
func (s *stubStore) GetOperationParams(opID string) ([]byte, error)     { return nil, nil }
func (s *stubStore) DeleteOperationState(opID string) error             { return nil }
func (s *stubStore) GetInterruptedOperations() ([]database.Operation, error) {
	return nil, nil
}
func (s *stubStore) GetUserPreference(key string) (*database.UserPreference, error) { return nil, nil }
func (s *stubStore) SetUserPreference(key, value string) error                      { return nil }
func (s *stubStore) GetAllUserPreferences() ([]database.UserPreference, error) {
	return []database.UserPreference{}, nil
}
func (s *stubStore) GetSetting(key string) (*database.Setting, error)       { return nil, nil }
func (s *stubStore) SetSetting(key, value, typ string, isSecret bool) error { return nil }
func (s *stubStore) GetAllSettings() ([]database.Setting, error)            { return []database.Setting{}, nil }
func (s *stubStore) DeleteSetting(key string) error                         { return nil }
func (s *stubStore) CreatePlaylist(name string, seriesID *int, filePath string) (*database.Playlist, error) {
	return &database.Playlist{}, nil
}
func (s *stubStore) GetPlaylistByID(id int) (*database.Playlist, error)             { return nil, nil }
func (s *stubStore) GetPlaylistBySeriesID(seriesID int) (*database.Playlist, error) { return nil, nil }
func (s *stubStore) AddPlaylistItem(playlistID, bookID, position int) error         { return nil }
func (s *stubStore) GetPlaylistItems(playlistID int) ([]database.PlaylistItem, error) {
	return []database.PlaylistItem{}, nil
}
func (s *stubStore) CreateUser(username, email, passwordHashAlgo, passwordHash string, roles []string, status string) (*database.User, error) {
	return &database.User{}, nil
}
func (s *stubStore) GetUserByID(id string) (*database.User, error)             { return nil, nil }
func (s *stubStore) GetUserByUsername(username string) (*database.User, error) { return nil, nil }
func (s *stubStore) GetUserByEmail(email string) (*database.User, error)       { return nil, nil }
func (s *stubStore) UpdateUser(user *database.User) error                      { return nil }
func (s *stubStore) CreateSession(userID, ip, userAgent string, ttl time.Duration) (*database.Session, error) {
	return &database.Session{}, nil
}
func (s *stubStore) GetSession(id string) (*database.Session, error) { return nil, nil }
func (s *stubStore) RevokeSession(id string) error                   { return nil }
func (s *stubStore) ListUserSessions(userID string) ([]database.Session, error) {
	return []database.Session{}, nil
}
func (s *stubStore) CountUsers() (int, error) { return 0, nil }
func (s *stubStore) DeleteExpiredSessions(now time.Time) (int, error) {
	return 0, nil
}
func (s *stubStore) SetUserPreferenceForUser(userID, key, value string) error { return nil }
func (s *stubStore) GetUserPreferenceForUser(userID, key string) (*database.UserPreferenceKV, error) {
	return nil, nil
}
func (s *stubStore) GetAllPreferencesForUser(userID string) ([]database.UserPreferenceKV, error) {
	return []database.UserPreferenceKV{}, nil
}
func (s *stubStore) CreateBookSegment(bookNumericID int, segment *database.BookSegment) (*database.BookSegment, error) {
	return segment, nil
}
func (s *stubStore) ListBookSegments(bookNumericID int) ([]database.BookSegment, error) {
	return []database.BookSegment{}, nil
}
func (s *stubStore) MergeBookSegments(bookNumericID int, newSegment *database.BookSegment, supersedeIDs []string) error {
	return nil
}
func (s *stubStore) AddPlaybackEvent(event *database.PlaybackEvent) error { return nil }
func (s *stubStore) ListPlaybackEvents(userID string, bookNumericID int, limit int) ([]database.PlaybackEvent, error) {
	return []database.PlaybackEvent{}, nil
}
func (s *stubStore) UpdatePlaybackProgress(progress *database.PlaybackProgress) error { return nil }
func (s *stubStore) GetPlaybackProgress(userID string, bookNumericID int) (*database.PlaybackProgress, error) {
	return nil, nil
}
func (s *stubStore) IncrementBookPlayStats(bookNumericID int, seconds int) error { return nil }
func (s *stubStore) GetBookStats(bookNumericID int) (*database.BookStats, error) { return nil, nil }
func (s *stubStore) IncrementUserListenStats(userID string, seconds int) error   { return nil }
func (s *stubStore) GetUserStats(userID string) (*database.UserStats, error)     { return nil, nil }
func (s *stubStore) IsHashBlocked(hash string) (bool, error)                     { return false, nil }
func (s *stubStore) AddBlockedHash(hash, reason string) error                    { return nil }
func (s *stubStore) RemoveBlockedHash(hash string) error                         { return nil }
func (s *stubStore) GetAllBlockedHashes() ([]database.DoNotImport, error) {
	return []database.DoNotImport{}, nil
}
func (s *stubStore) GetBlockedHashByHash(hash string) (*database.DoNotImport, error) { return nil, nil }
func (s *stubStore) SaveLibraryFingerprint(path string, size int64, modTime time.Time, crc32 uint32) error {
	return nil
}
func (s *stubStore) GetLibraryFingerprint(path string) (*database.LibraryFingerprintRecord, error) {
	return nil, nil
}
func (s *stubStore) Reset() error                                                      { return nil }
func (s *stubStore) GetBookAuthors(bookID string) ([]database.BookAuthor, error)       { return nil, nil }
func (s *stubStore) SetBookAuthors(bookID string, authors []database.BookAuthor) error { return nil }
func (s *stubStore) GetBooksByAuthorIDWithRole(authorID int) ([]database.Book, error) {
	return nil, nil
}

func stubCommandDeps(t *testing.T) {
	t.Helper()

	origInit := initializeStore
	origClose := closeStore
	origScan := scanDirectory
	origProcess := processBooks
	origPlaylists := generatePlaylists
	origTags := updateSeriesTags
	origInitEncrypt := initEncryption
	origLoadConfig := loadConfigFromDB
	origSyncEnv := syncConfigFromEnv
	origInitQueue := initializeQueue
	origShutdownQueue := shutdownQueue
	origNewServer := newServer
	origDefaultCfg := getDefaultServerConfig
	origStart := startServer

	initializeStore = func(dbType, path string, enableSQLite bool) error {
		// For command tests, we just need GlobalStore to be non-nil
		// Using a minimal mock implementation
		database.GlobalStore = &stubStore{}
		return nil
	}
	closeStore = func() error {
		database.GlobalStore = nil
		return nil
	}
	scanDirectory = func(rootDir string) ([]scanner.Book, error) {
		return []scanner.Book{}, nil
	}
	processBooks = func(books []scanner.Book) error {
		return nil
	}
	generatePlaylists = func() error {
		return nil
	}
	updateSeriesTags = func() error {
		return nil
	}
	initEncryption = func(dir string) error {
		return nil
	}
	loadConfigFromDB = func(store database.Store) error {
		return nil
	}
	syncConfigFromEnv = func() {}
	initializeQueue = func(store database.Store, workers int) {}
	shutdownQueue = func(timeout time.Duration) error { return nil }
	newServer = func() *server.Server { return &server.Server{} }
	getDefaultServerConfig = func() server.ServerConfig {
		return server.ServerConfig{Host: "localhost", Port: "8080"}
	}
	startServer = func(srv *server.Server, cfg server.ServerConfig) error { return nil }

	t.Cleanup(func() {
		initializeStore = origInit
		closeStore = origClose
		scanDirectory = origScan
		processBooks = origProcess
		generatePlaylists = origPlaylists
		updateSeriesTags = origTags
		initEncryption = origInitEncrypt
		loadConfigFromDB = origLoadConfig
		syncConfigFromEnv = origSyncEnv
		initializeQueue = origInitQueue
		shutdownQueue = origShutdownQueue
		newServer = origNewServer
		getDefaultServerConfig = origDefaultCfg
		startServer = origStart
		database.GlobalStore = nil
	})
}

func TestCommandsRunWithStubs(t *testing.T) {
	stubCommandDeps(t)

	tempDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(origDir)
	})

	origConfig := config.AppConfig
	t.Cleanup(func() {
		config.AppConfig = origConfig
	})

	config.AppConfig.DatabaseType = "sqlite"
	config.AppConfig.DatabasePath = filepath.Join(tempDir, "db.sqlite")
	config.AppConfig.RootDir = tempDir
	config.AppConfig.EnableSQLite = true
	config.AppConfig.PlaylistDir = filepath.Join(tempDir, "playlists")

	if err := scanCmd.RunE(scanCmd, nil); err != nil {
		t.Fatalf("scanCmd failed: %v", err)
	}
	if err := playlistCmd.RunE(playlistCmd, nil); err != nil {
		t.Fatalf("playlistCmd failed: %v", err)
	}
	if err := tagCmd.RunE(tagCmd, nil); err != nil {
		t.Fatalf("tagCmd failed: %v", err)
	}
	if err := organizeCmd.RunE(organizeCmd, nil); err != nil {
		t.Fatalf("organizeCmd failed: %v", err)
	}
	if err := serveCmd.RunE(serveCmd, nil); err != nil {
		t.Fatalf("serveCmd failed: %v", err)
	}
}

func TestScanCommandErrorPaths(t *testing.T) {
	stubCommandDeps(t)

	tempDir := t.TempDir()
	origConfig := config.AppConfig
	t.Cleanup(func() {
		config.AppConfig = origConfig
	})

	config.AppConfig.DatabaseType = "sqlite"
	config.AppConfig.DatabasePath = filepath.Join(tempDir, "db.sqlite")
	config.AppConfig.RootDir = tempDir
	config.AppConfig.EnableSQLite = true

	scanDirectory = func(rootDir string) ([]scanner.Book, error) {
		return nil, fmt.Errorf("scan failed")
	}
	if err := scanCmd.RunE(scanCmd, nil); err == nil {
		t.Fatal("expected scan command error")
	}

	scanDirectory = func(rootDir string) ([]scanner.Book, error) {
		return []scanner.Book{{FilePath: filepath.Join(tempDir, "book.m4b")}}, nil
	}
	processBooks = func(books []scanner.Book) error {
		return fmt.Errorf("process failed")
	}
	if err := scanCmd.RunE(scanCmd, nil); err == nil {
		t.Fatal("expected processBooks error")
	}
}

func TestServeCommandErrorPaths(t *testing.T) {
	stubCommandDeps(t)

	tempDir := t.TempDir()
	origConfig := config.AppConfig
	t.Cleanup(func() {
		config.AppConfig = origConfig
	})

	config.AppConfig.DatabaseType = "sqlite"
	config.AppConfig.DatabasePath = filepath.Join(tempDir, "db.sqlite")
	config.AppConfig.EnableSQLite = true

	initEncryption = func(dir string) error { return fmt.Errorf("encrypt fail") }
	if err := serveCmd.RunE(serveCmd, nil); err == nil {
		t.Fatal("expected serve command to fail on encryption error")
	}

	initEncryption = func(dir string) error { return nil }
	startServer = func(srv *server.Server, cfg server.ServerConfig) error {
		return fmt.Errorf("start failed")
	}
	if err := serveCmd.RunE(serveCmd, nil); err == nil {
		t.Fatal("expected serve command to fail on start error")
	}
}

func TestPlaylistCommandError(t *testing.T) {
	stubCommandDeps(t)

	tempDir := t.TempDir()
	origConfig := config.AppConfig
	t.Cleanup(func() {
		config.AppConfig = origConfig
	})

	config.AppConfig.DatabaseType = "sqlite"
	config.AppConfig.DatabasePath = filepath.Join(tempDir, "db.sqlite")
	config.AppConfig.EnableSQLite = true
	config.AppConfig.PlaylistDir = filepath.Join(tempDir, "playlists")

	generatePlaylists = func() error {
		return fmt.Errorf("playlist generation failed")
	}
	if err := playlistCmd.RunE(playlistCmd, nil); err == nil {
		t.Fatal("expected playlist command error")
	}
}

func TestTagCommandError(t *testing.T) {
	stubCommandDeps(t)

	tempDir := t.TempDir()
	origConfig := config.AppConfig
	t.Cleanup(func() {
		config.AppConfig = origConfig
	})

	config.AppConfig.DatabaseType = "sqlite"
	config.AppConfig.DatabasePath = filepath.Join(tempDir, "db.sqlite")
	config.AppConfig.EnableSQLite = true

	updateSeriesTags = func() error {
		return fmt.Errorf("tag update failed")
	}
	if err := tagCmd.RunE(tagCmd, nil); err == nil {
		t.Fatal("expected tag command error")
	}
}

func TestOrganizeCommandError(t *testing.T) {
	stubCommandDeps(t)

	tempDir := t.TempDir()
	origConfig := config.AppConfig
	t.Cleanup(func() {
		config.AppConfig = origConfig
	})

	config.AppConfig.DatabaseType = "sqlite"
	config.AppConfig.DatabasePath = filepath.Join(tempDir, "db.sqlite")
	config.AppConfig.EnableSQLite = true

	scanDirectory = func(rootDir string) ([]scanner.Book, error) {
		return nil, fmt.Errorf("scan failed in organize")
	}
	if err := organizeCmd.RunE(organizeCmd, nil); err == nil {
		t.Fatal("expected organize command error")
	}
}

func TestStoreInitializationError(t *testing.T) {
	stubCommandDeps(t)

	tempDir := t.TempDir()
	origConfig := config.AppConfig
	t.Cleanup(func() {
		config.AppConfig = origConfig
	})

	config.AppConfig.DatabaseType = "sqlite"
	config.AppConfig.DatabasePath = filepath.Join(tempDir, "db.sqlite")
	config.AppConfig.EnableSQLite = true

	initializeStore = func(dbType, path string, enableSQLite bool) error {
		return fmt.Errorf("store init failed")
	}

	if err := scanCmd.RunE(scanCmd, nil); err == nil {
		t.Fatal("expected scan command to fail on store init error")
	}
}
