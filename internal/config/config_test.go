// file: internal/config/config_test.go
// version: 1.2.1
// guid: b2c3d4e5-f6a7-8b9c-0d1e-2f3a4b5c6d7e

package config

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/spf13/viper"
)

// Implement minimal store interface methods - only the ones needed for config tests
func (m *mockStore) Close() error { return nil }

// Settings methods (required for config tests)
func (m *mockStore) GetSetting(key string) (*database.Setting, error) {
	if m.err != nil {
		return nil, m.err
	}
	for _, s := range m.settings {
		if s.Key == key {
			return &s, nil
		}
	}
	return nil, fmt.Errorf("setting not found")
}

func (m *mockStore) SetSetting(key, value, typ string, isSecret bool) error {
	if m.err != nil {
		return m.err
	}
	if m.saved == nil {
		m.saved = make(map[string]database.Setting)
	}
	m.saved[key] = database.Setting{Key: key, Value: value, Type: typ}
	return nil
}

// Stub implementations for interface compliance (not used in config tests)
func (m *mockStore) GetMetadataFieldStates(bookID string) ([]database.MetadataFieldState, error) {
	return nil, nil
}
func (m *mockStore) UpsertMetadataFieldState(state *database.MetadataFieldState) error { return nil }
func (m *mockStore) DeleteMetadataFieldState(bookID, field string) error               { return nil }
func (m *mockStore) GetAllAuthors() ([]database.Author, error)                         { return nil, nil }
func (m *mockStore) GetAuthorByID(id int) (*database.Author, error)                    { return nil, nil }
func (m *mockStore) GetAuthorByName(name string) (*database.Author, error)             { return nil, nil }
func (m *mockStore) CreateAuthor(name string) (*database.Author, error)                { return nil, nil }
func (m *mockStore) GetAllSeries() ([]database.Series, error)                          { return nil, nil }
func (m *mockStore) GetSeriesByID(id int) (*database.Series, error)                    { return nil, nil }
func (m *mockStore) GetSeriesByName(name string, authorID *int) (*database.Series, error) {
	return nil, nil
}
func (m *mockStore) CreateSeries(name string, authorID *int) (*database.Series, error) {
	return nil, nil
}
func (m *mockStore) GetAllWorks() ([]database.Work, error)                  { return nil, nil }
func (m *mockStore) GetWorkByID(id string) (*database.Work, error)          { return nil, nil }
func (m *mockStore) CreateWork(work *database.Work) (*database.Work, error) { return nil, nil }
func (m *mockStore) UpdateWork(id string, work *database.Work) (*database.Work, error) {
	return nil, nil
}
func (m *mockStore) DeleteWork(id string) error                                 { return nil }
func (m *mockStore) GetBooksByWorkID(workID string) ([]database.Book, error)    { return nil, nil }
func (m *mockStore) GetAllBooks(limit, offset int) ([]database.Book, error)     { return nil, nil }
func (m *mockStore) GetBookByID(id string) (*database.Book, error)              { return nil, nil }
func (m *mockStore) GetBookByFilePath(path string) (*database.Book, error)      { return nil, nil }
func (m *mockStore) GetBookByFileHash(hash string) (*database.Book, error)      { return nil, nil }
func (m *mockStore) GetBookByOriginalHash(hash string) (*database.Book, error)  { return nil, nil }
func (m *mockStore) GetBookByOrganizedHash(hash string) (*database.Book, error) { return nil, nil }
func (m *mockStore) GetDuplicateBooks() ([][]database.Book, error)              { return nil, nil }
func (m *mockStore) GetBooksBySeriesID(seriesID int) ([]database.Book, error)   { return nil, nil }
func (m *mockStore) GetBooksByAuthorID(authorID int) ([]database.Book, error)   { return nil, nil }
func (m *mockStore) CreateBook(book *database.Book) (*database.Book, error)     { return nil, nil }
func (m *mockStore) UpdateBook(id string, book *database.Book) (*database.Book, error) {
	return nil, nil
}
func (m *mockStore) DeleteBook(id string) error { return nil }
func (m *mockStore) SearchBooks(query string, limit, offset int) ([]database.Book, error) {
	return nil, nil
}
func (m *mockStore) CountBooks() (int, error) { return 0, nil }
func (m *mockStore) ListSoftDeletedBooks(limit, offset int, olderThan *time.Time) ([]database.Book, error) {
	return nil, nil
}
func (m *mockStore) GetBooksByVersionGroup(groupID string) ([]database.Book, error) { return nil, nil }
func (m *mockStore) GetAllImportPaths() ([]database.ImportPath, error)              { return nil, nil }
func (m *mockStore) GetImportPathByID(id int) (*database.ImportPath, error)         { return nil, nil }
func (m *mockStore) GetImportPathByPath(path string) (*database.ImportPath, error)  { return nil, nil }
func (m *mockStore) CreateImportPath(path, name string) (*database.ImportPath, error) {
	return nil, nil
}
func (m *mockStore) UpdateImportPath(id int, importPath *database.ImportPath) error { return nil }
func (m *mockStore) DeleteImportPath(id int) error                                  { return nil }
func (m *mockStore) CreateOperation(id, opType string, folderPath *string) (*database.Operation, error) {
	return nil, nil
}
func (m *mockStore) GetOperationByID(id string) (*database.Operation, error)     { return nil, nil }
func (m *mockStore) GetRecentOperations(limit int) ([]database.Operation, error) { return nil, nil }
func (m *mockStore) UpdateOperationStatus(id, status string, progress, total int, message string) error {
	return nil
}
func (m *mockStore) UpdateOperationError(id, errorMessage string) error { return nil }
func (m *mockStore) AddOperationLog(operationID, level, message string, details *string) error {
	return nil
}
func (m *mockStore) GetOperationLogs(operationID string) ([]database.OperationLog, error) {
	return nil, nil
}
func (m *mockStore) GetUserPreference(key string) (*database.UserPreference, error) { return nil, nil }
func (m *mockStore) SetUserPreference(key, value string) error                      { return nil }
func (m *mockStore) GetAllUserPreferences() ([]database.UserPreference, error)      { return nil, nil }
func (m *mockStore) DeleteSetting(key string) error                                 { return nil }
func (m *mockStore) CreatePlaylist(name string, seriesID *int, filePath string) (*database.Playlist, error) {
	return nil, nil
}
func (m *mockStore) GetPlaylistByID(id int) (*database.Playlist, error)             { return nil, nil }
func (m *mockStore) GetPlaylistBySeriesID(seriesID int) (*database.Playlist, error) { return nil, nil }
func (m *mockStore) AddPlaylistItem(playlistID, bookID, position int) error         { return nil }
func (m *mockStore) GetPlaylistItems(playlistID int) ([]database.PlaylistItem, error) {
	return nil, nil
}
func (m *mockStore) CreateUser(username, email, passwordHashAlgo, passwordHash string, roles []string, status string) (*database.User, error) {
	return nil, nil
}
func (m *mockStore) GetUserByID(id string) (*database.User, error)             { return nil, nil }
func (m *mockStore) GetUserByUsername(username string) (*database.User, error) { return nil, nil }
func (m *mockStore) GetUserByEmail(email string) (*database.User, error)       { return nil, nil }
func (m *mockStore) UpdateUser(user *database.User) error                      { return nil }
func (m *mockStore) CreateSession(userID, ip, userAgent string, ttl time.Duration) (*database.Session, error) {
	return nil, nil
}
func (m *mockStore) GetSession(id string) (*database.Session, error)            { return nil, nil }
func (m *mockStore) RevokeSession(id string) error                              { return nil }
func (m *mockStore) ListUserSessions(userID string) ([]database.Session, error) { return nil, nil }
func (m *mockStore) SetUserPreferenceForUser(userID, key, value string) error   { return nil }
func (m *mockStore) GetUserPreferenceForUser(userID, key string) (*database.UserPreferenceKV, error) {
	return nil, nil
}
func (m *mockStore) GetAllPreferencesForUser(userID string) ([]database.UserPreferenceKV, error) {
	return nil, nil
}
func (m *mockStore) CreateBookSegment(bookNumericID int, segment *database.BookSegment) (*database.BookSegment, error) {
	return nil, nil
}
func (m *mockStore) ListBookSegments(bookNumericID int) ([]database.BookSegment, error) {
	return nil, nil
}
func (m *mockStore) MergeBookSegments(bookNumericID int, newSegment *database.BookSegment, supersedeIDs []string) error {
	return nil
}
func (m *mockStore) AddPlaybackEvent(event *database.PlaybackEvent) error { return nil }
func (m *mockStore) ListPlaybackEvents(userID string, bookNumericID int, limit int) ([]database.PlaybackEvent, error) {
	return nil, nil
}
func (m *mockStore) UpdatePlaybackProgress(progress *database.PlaybackProgress) error { return nil }
func (m *mockStore) GetPlaybackProgress(userID string, bookNumericID int) (*database.PlaybackProgress, error) {
	return nil, nil
}
func (m *mockStore) IncrementBookPlayStats(bookNumericID int, seconds int) error     { return nil }
func (m *mockStore) GetBookStats(bookNumericID int) (*database.BookStats, error)     { return nil, nil }
func (m *mockStore) IncrementUserListenStats(userID string, seconds int) error       { return nil }
func (m *mockStore) GetUserStats(userID string) (*database.UserStats, error)         { return nil, nil }
func (m *mockStore) IsHashBlocked(hash string) (bool, error)                         { return false, nil }
func (m *mockStore) AddBlockedHash(hash, reason string) error                        { return nil }
func (m *mockStore) RemoveBlockedHash(hash string) error                             { return nil }
func (m *mockStore) GetAllBlockedHashes() ([]database.DoNotImport, error)            { return nil, nil }
func (m *mockStore) GetBlockedHashByHash(hash string) (*database.DoNotImport, error) { return nil, nil }

// TestInitConfig tests configuration initialization with defaults
func TestInitConfig(t *testing.T) {
	// Arrange
	viper.Reset()

	// Act
	InitConfig()

	// Assert - Verify database defaults
	dbType := viper.GetString("database_type")
	if dbType != "pebble" {
		t.Errorf("Expected database_type to be 'pebble', got '%s'", dbType)
	}

	if enableSQLite := viper.GetBool("enable_sqlite3_i_know_the_risks"); enableSQLite {
		t.Error("Expected enable_sqlite3_i_know_the_risks to be false by default")
	}

	// Verify organization strategy defaults
	orgStrategy := viper.GetString("organization_strategy")
	if orgStrategy != "auto" {
		t.Errorf("Expected organization_strategy to be 'auto', got '%s'", orgStrategy)
	}

	if scanOnStartup := viper.GetBool("scan_on_startup"); scanOnStartup {
		t.Error("Expected scan_on_startup to be false by default")
	}

	if autoOrganize := viper.GetBool("auto_organize"); !autoOrganize {
		t.Error("Expected auto_organize to be true by default")
	}

	// Verify naming patterns
	expectedFolderPattern := "{author}/{series}/{title} ({print_year})"
	folderPattern := viper.GetString("folder_naming_pattern")
	if folderPattern != expectedFolderPattern {
		t.Errorf("Expected folder_naming_pattern to be '%s', got '%s'", expectedFolderPattern, folderPattern)
	}

	expectedFilePattern := "{title} - {author} - read by {narrator}"
	filePattern := viper.GetString("file_naming_pattern")
	if filePattern != expectedFilePattern {
		t.Errorf("Expected file_naming_pattern to be '%s', got '%s'", expectedFilePattern, filePattern)
	}

	// Verify backup defaults
	if createBackups := viper.GetBool("create_backups"); !createBackups {
		t.Error("Expected create_backups to be true by default")
	}
}

// TestStorageQuotaDefaults tests storage quota default values
func TestStorageQuotaDefaults(t *testing.T) {
	// Arrange-Act-Assert: Test disk quota defaults
	viper.Reset()
	InitConfig()

	if enableDiskQuota := viper.GetBool("enable_disk_quota"); enableDiskQuota {
		t.Error("Expected enable_disk_quota to be false by default")
	}

	diskQuotaPercent := viper.GetInt("disk_quota_percent")
	if diskQuotaPercent != 80 {
		t.Errorf("Expected disk_quota_percent to be 80, got %d", diskQuotaPercent)
	}

	if enableUserQuotas := viper.GetBool("enable_user_quotas"); enableUserQuotas {
		t.Error("Expected enable_user_quotas to be false by default")
	}

	defaultUserQuota := viper.GetInt("default_user_quota_gb")
	if defaultUserQuota != 100 {
		t.Errorf("Expected default_user_quota_gb to be 100, got %d", defaultUserQuota)
	}
}

// TestMetadataDefaults tests metadata configuration defaults
func TestMetadataDefaults(t *testing.T) {
	// Arrange-Act-Assert: Test metadata defaults
	viper.Reset()
	InitConfig()

	if autoFetch := viper.GetBool("auto_fetch_metadata"); !autoFetch {
		t.Error("Expected auto_fetch_metadata to be true by default")
	}

	language := viper.GetString("language")
	if language != "en" {
		t.Errorf("Expected language to be 'en', got '%s'", language)
	}

	// Verify metadata sources are populated
	if len(AppConfig.MetadataSources) == 0 {
		t.Error("Expected metadata sources to be populated")
	}

	// Verify Audible is first and enabled
	if len(AppConfig.MetadataSources) > 0 {
		audible := AppConfig.MetadataSources[0]
		if audible.ID != "audible" {
			t.Errorf("Expected first metadata source to be 'audible', got '%s'", audible.ID)
		}
		if !audible.Enabled {
			t.Error("Expected Audible to be enabled by default")
		}
	}
}

// TestAIParsingDefaults tests AI parsing configuration defaults
func TestAIParsingDefaults(t *testing.T) {
	// Arrange-Act-Assert: Test AI defaults
	viper.Reset()
	InitConfig()

	if enableAI := viper.GetBool("enable_ai_parsing"); enableAI {
		t.Error("Expected enable_ai_parsing to be false by default")
	}

	apiKey := viper.GetString("openai_api_key")
	if apiKey != "" {
		t.Errorf("Expected openai_api_key to be empty by default, got '%s'", apiKey)
	}
}

// TestPerformanceDefaults tests performance configuration defaults
func TestPerformanceDefaults(t *testing.T) {
	// Arrange-Act-Assert: Test performance defaults
	viper.Reset()
	InitConfig()

	concurrentScans := viper.GetInt("concurrent_scans")
	if concurrentScans != 4 {
		t.Errorf("Expected concurrent_scans to be 4, got %d", concurrentScans)
	}
}

// TestMemoryManagementDefaults tests memory management defaults
func TestMemoryManagementDefaults(t *testing.T) {
	// Arrange-Act-Assert: Test memory defaults
	viper.Reset()
	InitConfig()

	memoryLimitType := viper.GetString("memory_limit_type")
	if memoryLimitType != "items" {
		t.Errorf("Expected memory_limit_type to be 'items', got '%s'", memoryLimitType)
	}

	cacheSize := viper.GetInt("cache_size")
	if cacheSize != 1000 {
		t.Errorf("Expected cache_size to be 1000, got %d", cacheSize)
	}

	memoryLimitPercent := viper.GetInt("memory_limit_percent")
	if memoryLimitPercent != 25 {
		t.Errorf("Expected memory_limit_percent to be 25, got %d", memoryLimitPercent)
	}

	memoryLimitMB := viper.GetInt("memory_limit_mb")
	if memoryLimitMB != 512 {
		t.Errorf("Expected memory_limit_mb to be 512, got %d", memoryLimitMB)
	}
}

// TestLoggingDefaults tests logging configuration defaults
func TestLoggingDefaults(t *testing.T) {
	// Arrange-Act-Assert: Test logging defaults
	viper.Reset()
	InitConfig()

	logLevel := viper.GetString("log_level")
	if logLevel != "info" {
		t.Errorf("Expected log_level to be 'info', got '%s'", logLevel)
	}

	logFormat := viper.GetString("log_format")
	if logFormat != "text" {
		t.Errorf("Expected log_format to be 'text', got '%s'", logFormat)
	}

	if enableJSON := viper.GetBool("enable_json_logging"); enableJSON {
		t.Error("Expected enable_json_logging to be false by default")
	}
}

// TestConfigStructure tests the Config struct
func TestConfigStructure(t *testing.T) {
	// Arrange
	config := Config{
		RootDir:              "/media/audiobooks",
		DatabasePath:         "/data/audiobooks.db",
		DatabaseType:         "pebble",
		EnableSQLite:         false,
		OrganizationStrategy: "auto",
		AutoFetchMetadata:    true,
		Language:             "en",
	}

	// Act & Assert
	if config.RootDir != "/media/audiobooks" {
		t.Errorf("Expected RootDir to be '/media/audiobooks', got '%s'", config.RootDir)
	}

	if config.DatabaseType != "pebble" {
		t.Errorf("Expected DatabaseType to be 'pebble', got '%s'", config.DatabaseType)
	}

	if config.EnableSQLite {
		t.Error("Expected EnableSQLite to be false")
	}

	if config.OrganizationStrategy != "auto" {
		t.Errorf("Expected OrganizationStrategy to be 'auto', got '%s'", config.OrganizationStrategy)
	}
}

// TestMetadataSourceStructure tests the MetadataSource struct
func TestMetadataSourceStructure(t *testing.T) {
	// Arrange
	credentials := map[string]string{
		"api_key": "test_key",
	}

	source := MetadataSource{
		ID:           "audible",
		Name:         "Audible",
		Enabled:      true,
		Priority:     1,
		RequiresAuth: true,
		Credentials:  credentials,
	}

	// Act & Assert
	if source.ID != "audible" {
		t.Errorf("Expected ID to be 'audible', got '%s'", source.ID)
	}

	if !source.Enabled {
		t.Error("Expected Enabled to be true")
	}

	if source.Priority != 1 {
		t.Errorf("Expected Priority to be 1, got %d", source.Priority)
	}

	if !source.RequiresAuth {
		t.Error("Expected RequiresAuth to be true")
	}

	if len(source.Credentials) != 1 {
		t.Errorf("Expected 1 credential, got %d", len(source.Credentials))
	}
}

// TestSupportedExtensionsDefaults tests supported extensions defaults
func TestSupportedExtensionsDefaults(t *testing.T) {
	// Arrange-Act
	viper.Reset()
	InitConfig()

	// Assert
	extensions := AppConfig.SupportedExtensions
	expectedExtensions := []string{".m4b", ".mp3", ".m4a", ".aac", ".ogg", ".flac", ".wma"}

	if len(extensions) != len(expectedExtensions) {
		t.Errorf("Expected %d extensions, got %d", len(expectedExtensions), len(extensions))
	}

	// Verify specific extensions
	extensionMap := make(map[string]bool)
	for _, ext := range extensions {
		extensionMap[ext] = true
	}

	for _, expected := range expectedExtensions {
		if !extensionMap[expected] {
			t.Errorf("Expected extension '%s' not found in defaults", expected)
		}
	}
}

// TestDatabaseTypeNormalization tests SQLite3 to SQLite normalization
func TestDatabaseTypeNormalization(t *testing.T) {
	// Arrange
	viper.Reset()
	viper.Set("database_type", "sqlite3")

	// Act
	InitConfig()

	// Assert
	if AppConfig.DatabaseType != "sqlite" {
		t.Errorf("Expected database_type to be normalized to 'sqlite', got '%s'", AppConfig.DatabaseType)
	}
}

// TestDefaultMetadataSources tests the default metadata sources configuration
func TestDefaultMetadataSources(t *testing.T) {
	// Arrange
	viper.Reset()

	// Act
	InitConfig()

	// Assert
	if len(AppConfig.MetadataSources) < 4 {
		t.Errorf("Expected at least 4 default metadata sources, got %d", len(AppConfig.MetadataSources))
	}

	// Verify Audible
	audible := AppConfig.MetadataSources[0]
	if audible.ID != "audible" || !audible.Enabled || audible.Priority != 1 {
		t.Error("Audible metadata source not configured correctly")
	}

	// Verify Goodreads
	goodreads := AppConfig.MetadataSources[1]
	if goodreads.ID != "goodreads" || !goodreads.Enabled || goodreads.Priority != 2 {
		t.Error("Goodreads metadata source not configured correctly")
	}
}

// TestConfigurationValidation tests basic configuration validation
func TestConfigurationValidation(t *testing.T) {
	// Arrange-Act-Assert: Verify that invalid database types are handled
	viper.Reset()
	InitConfig()

	// Verify valid database types
	validTypes := []string{"pebble", "sqlite"}
	dbType := AppConfig.DatabaseType

	isValid := false
	for _, valid := range validTypes {
		if dbType == valid {
			isValid = true
			break
		}
	}

	if !isValid {
		t.Errorf("Database type '%s' is not a valid type. Expected one of: %v", dbType, validTypes)
	}
}

func TestLoadConfigFromDatabase_NilStore(t *testing.T) {
	err := LoadConfigFromDatabase(nil)
	if err == nil {
		t.Error("Expected error for nil store")
	}
	if !strings.Contains(err.Error(), "nil") {
		t.Errorf("Expected 'nil' in error, got: %v", err)
	}
}

func TestLoadConfigFromDatabase_EmptySettings(t *testing.T) {
	store := &mockStore{settings: []database.Setting{}}
	err := LoadConfigFromDatabase(store)
	if err != nil {
		t.Errorf("Expected no error for empty settings, got: %v", err)
	}
}

func TestLoadConfigFromDatabase_StringSettings(t *testing.T) {
	store := &mockStore{
		settings: []database.Setting{
			{Key: "root_dir", Value: "/test/root", Type: "string"},
			{Key: "organization_strategy", Value: "copy", Type: "string"},
			{Key: "language", Value: "en", Type: "string"},
		},
	}

	// Initialize with defaults first
	InitConfig()

	err := LoadConfigFromDatabase(store)
	if err != nil {
		t.Fatalf("LoadConfigFromDatabase failed: %v", err)
	}

	if AppConfig.RootDir != "/test/root" {
		t.Errorf("Expected root_dir /test/root, got %s", AppConfig.RootDir)
	}
	if AppConfig.OrganizationStrategy != "copy" {
		t.Errorf("Expected strategy copy, got %s", AppConfig.OrganizationStrategy)
	}
	if AppConfig.Language != "en" {
		t.Errorf("Expected language en, got %s", AppConfig.Language)
	}
}

func TestLoadConfigFromDatabase_BoolSettings(t *testing.T) {
	store := &mockStore{
		settings: []database.Setting{
			{Key: "scan_on_startup", Value: "true", Type: "bool"},
			{Key: "auto_organize", Value: "false", Type: "bool"},
			{Key: "create_backups", Value: "true", Type: "bool"},
			{Key: "enable_disk_quota", Value: "true", Type: "bool"},
		},
	}

	InitConfig()
	err := LoadConfigFromDatabase(store)
	if err != nil {
		t.Fatalf("LoadConfigFromDatabase failed: %v", err)
	}

	if !AppConfig.ScanOnStartup {
		t.Error("Expected scan_on_startup to be true")
	}
	if AppConfig.AutoOrganize {
		t.Error("Expected auto_organize to be false")
	}
	if !AppConfig.CreateBackups {
		t.Error("Expected create_backups to be true")
	}
	if !AppConfig.EnableDiskQuota {
		t.Error("Expected enable_disk_quota to be true")
	}
}

func TestLoadConfigFromDatabase_IntSettings(t *testing.T) {
	store := &mockStore{
		settings: []database.Setting{
			{Key: "concurrent_scans", Value: "8", Type: "int"},
			{Key: "disk_quota_percent", Value: "90", Type: "int"},
			{Key: "cache_size", Value: "5000", Type: "int"},
			{Key: "memory_limit_mb", Value: "2048", Type: "int"},
		},
	}

	InitConfig()
	err := LoadConfigFromDatabase(store)
	if err != nil {
		t.Fatalf("LoadConfigFromDatabase failed: %v", err)
	}

	if AppConfig.ConcurrentScans != 8 {
		t.Errorf("Expected concurrent_scans 8, got %d", AppConfig.ConcurrentScans)
	}
	if AppConfig.DiskQuotaPercent != 90 {
		t.Errorf("Expected disk_quota_percent 90, got %d", AppConfig.DiskQuotaPercent)
	}
	if AppConfig.CacheSize != 5000 {
		t.Errorf("Expected cache_size 5000, got %d", AppConfig.CacheSize)
	}
	if AppConfig.MemoryLimitMB != 2048 {
		t.Errorf("Expected memory_limit_mb 2048, got %d", AppConfig.MemoryLimitMB)
	}
}

func TestSaveConfigToDatabase_BasicSettings(t *testing.T) {
	store := &mockStore{saved: make(map[string]database.Setting)}

	InitConfig()
	AppConfig.RootDir = "/custom/root"
	AppConfig.ConcurrentScans = 10
	AppConfig.AutoOrganize = true

	err := SaveConfigToDatabase(store)
	if err != nil {
		t.Fatalf("SaveConfigToDatabase failed: %v", err)
	}

	// Check that settings were saved
	if len(store.saved) == 0 {
		t.Error("Expected settings to be saved")
	}

	// Verify some key settings
	if setting, ok := store.saved["root_dir"]; ok {
		if setting.Value != "/custom/root" {
			t.Errorf("Expected root_dir /custom/root, got %s", setting.Value)
		}
	}
}

func TestSaveConfigToDatabase_NilStore(t *testing.T) {
	err := SaveConfigToDatabase(nil)
	if err == nil {
		t.Error("Expected error for nil store")
	}
}

func TestSyncConfigFromEnv(t *testing.T) {
	// Set environment variables
	os.Setenv("AUDIOBOOK_ROOT_DIR", "/env/root")
	os.Setenv("AUDIOBOOK_CONCURRENT_SCANS", "16")
	os.Setenv("AUDIOBOOK_AUTO_ORGANIZE", "false")
	defer func() {
		os.Unsetenv("AUDIOBOOK_ROOT_DIR")
		os.Unsetenv("AUDIOBOOK_CONCURRENT_SCANS")
		os.Unsetenv("AUDIOBOOK_AUTO_ORGANIZE")
	}()

	InitConfig()
	SyncConfigFromEnv()

	if AppConfig.RootDir != "/env/root" {
		t.Errorf("Expected root_dir from env /env/root, got %s", AppConfig.RootDir)
	}
	if AppConfig.ConcurrentScans != 16 {
		t.Errorf("Expected concurrent_scans from env 16, got %d", AppConfig.ConcurrentScans)
	}
	if AppConfig.AutoOrganize {
		t.Error("Expected auto_organize from env to be false")
	}
}

// Mock store for testing
type mockStore struct {
	settings []database.Setting
	saved    map[string]database.Setting
	err      error
}

func (m *mockStore) GetAllSettings() ([]database.Setting, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.settings, nil
}

func (m *mockStore) SaveSetting(setting database.Setting) error {
	if m.err != nil {
		return m.err
	}
	if m.saved == nil {
		m.saved = make(map[string]database.Setting)
	}
	m.saved[setting.Key] = setting
	return nil
}
