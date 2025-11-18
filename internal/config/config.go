// file: internal/config/config.go
// version: 1.3.0
// guid: 7b8c9d0e-1f2a-3b4c-5d6e-7f8a9b0c1d2e

package config

import (
	"github.com/spf13/viper"
)

// MetadataSource represents a metadata provider configuration
type MetadataSource struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Enabled      bool              `json:"enabled"`
	Priority     int               `json:"priority"`
	RequiresAuth bool              `json:"requires_auth"`
	Credentials  map[string]string `json:"credentials"`
}

// Config holds application configuration
type Config struct {
	// Core paths
	RootDir      string `json:"root_dir"`
	DatabasePath string `json:"database_path"`
	DatabaseType string `json:"database_type"` // "pebble" (default) or "sqlite"
	EnableSQLite bool   `json:"enable_sqlite"` // Must be true to use SQLite (safety flag)
	PlaylistDir  string `json:"playlist_dir"`

	// Library organization
	OrganizationStrategy string `json:"organization_strategy"` // 'auto', 'copy', 'hardlink', 'reflink', 'symlink'
	ScanOnStartup        bool   `json:"scan_on_startup"`
	AutoOrganize         bool   `json:"auto_organize"`
	FolderNamingPattern  string `json:"folder_naming_pattern"`
	FileNamingPattern    string `json:"file_naming_pattern"`
	CreateBackups        bool   `json:"create_backups"`

	// Storage quotas
	EnableDiskQuota    bool `json:"enable_disk_quota"`
	DiskQuotaPercent   int  `json:"disk_quota_percent"`
	EnableUserQuotas   bool `json:"enable_user_quotas"`
	DefaultUserQuotaGB int  `json:"default_user_quota_gb"`

	// Metadata
	AutoFetchMetadata bool             `json:"auto_fetch_metadata"`
	MetadataSources   []MetadataSource `json:"metadata_sources"`
	Language          string           `json:"language"`

	// Performance
	ConcurrentScans int `json:"concurrent_scans"`

	// Memory management
	MemoryLimitType    string `json:"memory_limit_type"`    // 'items', 'percent', 'absolute'
	CacheSize          int    `json:"cache_size"`           // number of items
	MemoryLimitPercent int    `json:"memory_limit_percent"` // % of system memory
	MemoryLimitMB      int    `json:"memory_limit_mb"`      // absolute MB

	// Logging
	LogLevel          string `json:"log_level"`  // 'debug', 'info', 'warn', 'error'
	LogFormat         string `json:"log_format"` // 'text' or 'json'
	EnableJsonLogging bool   `json:"enable_json_logging"`

	// API Keys (kept for backward compatibility)
	APIKeys struct {
		Goodreads string `json:"goodreads"`
	} `json:"api_keys"`

	SupportedExtensions []string `json:"supported_extensions"`
}

var AppConfig Config

// InitConfig initializes the application configuration
func InitConfig() {
	// Set core defaults
	viper.SetDefault("database_type", "pebble")
	viper.SetDefault("enable_sqlite3_i_know_the_risks", false)

	// Set library organization defaults
	viper.SetDefault("organization_strategy", "auto")
	viper.SetDefault("scan_on_startup", false)
	viper.SetDefault("auto_organize", true)
	viper.SetDefault("folder_naming_pattern", "{author}/{series}/{title} ({print_year})")
	viper.SetDefault("file_naming_pattern", "{title} - {author} - read by {narrator}")
	viper.SetDefault("create_backups", true)

	// Set storage quota defaults
	viper.SetDefault("enable_disk_quota", false)
	viper.SetDefault("disk_quota_percent", 80)
	viper.SetDefault("enable_user_quotas", false)
	viper.SetDefault("default_user_quota_gb", 100)

	// Set metadata defaults
	viper.SetDefault("auto_fetch_metadata", true)
	viper.SetDefault("language", "en")

	// Set performance defaults
	viper.SetDefault("concurrent_scans", 4)

	// Set memory management defaults
	viper.SetDefault("memory_limit_type", "items")
	viper.SetDefault("cache_size", 1000)
	viper.SetDefault("memory_limit_percent", 25)
	viper.SetDefault("memory_limit_mb", 512)

	// Set logging defaults
	viper.SetDefault("log_level", "info")
	viper.SetDefault("log_format", "text")
	viper.SetDefault("enable_json_logging", false)

	AppConfig = Config{
		// Core paths
		RootDir:      viper.GetString("root_dir"),
		DatabasePath: viper.GetString("database_path"),
		DatabaseType: viper.GetString("database_type"),
		EnableSQLite: viper.GetBool("enable_sqlite3_i_know_the_risks"),
		PlaylistDir:  viper.GetString("playlist_dir"),

		// Library organization
		OrganizationStrategy: viper.GetString("organization_strategy"),
		ScanOnStartup:        viper.GetBool("scan_on_startup"),
		AutoOrganize:         viper.GetBool("auto_organize"),
		FolderNamingPattern:  viper.GetString("folder_naming_pattern"),
		FileNamingPattern:    viper.GetString("file_naming_pattern"),
		CreateBackups:        viper.GetBool("create_backups"),

		// Storage quotas
		EnableDiskQuota:    viper.GetBool("enable_disk_quota"),
		DiskQuotaPercent:   viper.GetInt("disk_quota_percent"),
		EnableUserQuotas:   viper.GetBool("enable_user_quotas"),
		DefaultUserQuotaGB: viper.GetInt("default_user_quota_gb"),

		// Metadata
		AutoFetchMetadata: viper.GetBool("auto_fetch_metadata"),
		Language:          viper.GetString("language"),

		// Performance
		ConcurrentScans: viper.GetInt("concurrent_scans"),

		// Memory management
		MemoryLimitType:    viper.GetString("memory_limit_type"),
		CacheSize:          viper.GetInt("cache_size"),
		MemoryLimitPercent: viper.GetInt("memory_limit_percent"),
		MemoryLimitMB:      viper.GetInt("memory_limit_mb"),

		// Logging
		LogLevel:          viper.GetString("log_level"),
		LogFormat:         viper.GetString("log_format"),
		EnableJsonLogging: viper.GetBool("enable_json_logging"),

		SupportedExtensions: []string{
			".m4b", ".mp3", ".m4a", ".aac", ".ogg", ".flac", ".wma",
		},
	}

	// API Keys
	AppConfig.APIKeys.Goodreads = viper.GetString("api_keys.goodreads")

	// Load metadata sources from config or use defaults
	if viper.IsSet("metadata_sources") {
		viper.UnmarshalKey("metadata_sources", &AppConfig.MetadataSources)
	} else {
		// Set default metadata sources
		AppConfig.MetadataSources = []MetadataSource{
			{
				ID:           "audible",
				Name:         "Audible",
				Enabled:      true,
				Priority:     1,
				RequiresAuth: false,
				Credentials:  make(map[string]string),
			},
			{
				ID:           "goodreads",
				Name:         "Goodreads",
				Enabled:      true,
				Priority:     2,
				RequiresAuth: true,
				Credentials: map[string]string{
					"apiKey":    "",
					"apiSecret": "",
				},
			},
			{
				ID:           "openlibrary",
				Name:         "Open Library",
				Enabled:      false,
				Priority:     3,
				RequiresAuth: true,
				Credentials: map[string]string{
					"apiKey": "",
				},
			},
			{
				ID:           "google-books",
				Name:         "Google Books",
				Enabled:      false,
				Priority:     4,
				RequiresAuth: true,
				Credentials: map[string]string{
					"apiKey": "",
				},
			},
		}
	}

	// Normalize database type
	if AppConfig.DatabaseType == "sqlite3" {
		AppConfig.DatabaseType = "sqlite"
	}
	if AppConfig.DatabaseType == "" {
		AppConfig.DatabaseType = "pebble"
	}
}
