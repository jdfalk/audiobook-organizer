// file: internal/config/config_test.go
// version: 1.2.0
// guid: b2c3d4e5-f6a7-8b9c-0d1e-2f3a4b5c6d7e

package config

import (
	"testing"

	"github.com/spf13/viper"
)

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
