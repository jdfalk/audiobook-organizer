// file: internal/config/config.go
// version: 1.6.0
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

// DownloadClientConfig represents download client connection settings.
type DownloadClientConfig struct {
	Type        string            `json:"type"`
	Deluge      DelugeConfig      `json:"deluge"`
	QBittorrent QBittorrentConfig `json:"qbittorrent"`
	SABnzbd     SABnzbdConfig     `json:"sabnzbd"`
}

// DelugeConfig holds Deluge RPC configuration.
type DelugeConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// QBittorrentConfig holds qBittorrent Web API configuration.
type QBittorrentConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	UseHTTPS bool   `json:"use_https"`
}

// SABnzbdConfig holds SABnzbd API configuration.
type SABnzbdConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	APIKey   string `json:"api_key"`
	UseHTTPS bool   `json:"use_https"`
}

// Config holds application configuration
type Config struct {
	// Core paths
	RootDir       string `json:"root_dir"`
	DatabasePath  string `json:"database_path"`
	DatabaseType  string `json:"database_type"` // "pebble" (default) or "sqlite"
	EnableSQLite  bool   `json:"enable_sqlite"` // Must be true to use SQLite (safety flag)
	PlaylistDir   string `json:"playlist_dir"`
	SetupComplete bool   `json:"setup_complete"`

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

	// AI-powered parsing
	EnableAIParsing bool   `json:"enable_ai_parsing"`
	OpenAIAPIKey    string `json:"openai_api_key"`

	// Performance
	ConcurrentScans int `json:"concurrent_scans"`

	// Memory management
	MemoryLimitType    string `json:"memory_limit_type"`    // 'items', 'percent', 'absolute'
	CacheSize          int    `json:"cache_size"`           // number of items
	MemoryLimitPercent int    `json:"memory_limit_percent"` // % of system memory
	MemoryLimitMB      int    `json:"memory_limit_mb"`      // absolute MB

	// Lifecycle / retention
	PurgeSoftDeletedAfterDays   int  `json:"purge_soft_deleted_after_days"`
	PurgeSoftDeletedDeleteFiles bool `json:"purge_soft_deleted_delete_files"`

	// Logging
	LogLevel          string `json:"log_level"`  // 'debug', 'info', 'warn', 'error'
	LogFormat         string `json:"log_format"` // 'text' or 'json'
	EnableJsonLogging bool   `json:"enable_json_logging"`

	// Download client integration
	DownloadClient DownloadClientConfig `json:"download_client"`

	// API Keys (kept for backward compatibility)
	APIKeys struct {
		Goodreads string `json:"goodreads"`
	} `json:"api_keys"`

	SupportedExtensions []string `json:"supported_extensions"`
	ExcludePatterns     []string `json:"exclude_patterns"`
}

var AppConfig Config

// InitConfig initializes the application configuration
func InitConfig() {
	// Set core defaults
	viper.SetDefault("database_type", "pebble")
	viper.SetDefault("enable_sqlite3_i_know_the_risks", false)
	viper.SetDefault("setup_complete", false)

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

	// Set AI parsing defaults
	viper.SetDefault("enable_ai_parsing", false)
	viper.SetDefault("openai_api_key", "")

	// Set performance defaults
	viper.SetDefault("concurrent_scans", 4)

	// Set memory management defaults
	viper.SetDefault("memory_limit_type", "items")
	viper.SetDefault("cache_size", 1000)
	viper.SetDefault("memory_limit_percent", 25)
	viper.SetDefault("memory_limit_mb", 512)

	// Lifecycle / retention defaults
	viper.SetDefault("purge_soft_deleted_after_days", 30)
	viper.SetDefault("purge_soft_deleted_delete_files", false)

	// Set logging defaults
	viper.SetDefault("log_level", "info")
	viper.SetDefault("log_format", "text")
	viper.SetDefault("enable_json_logging", false)

	// Download client defaults
	viper.SetDefault("download_client.type", "")
	viper.SetDefault("download_client.deluge.host", "")
	viper.SetDefault("download_client.deluge.port", 0)
	viper.SetDefault("download_client.deluge.username", "")
	viper.SetDefault("download_client.deluge.password", "")
	viper.SetDefault("download_client.qbittorrent.host", "")
	viper.SetDefault("download_client.qbittorrent.port", 0)
	viper.SetDefault("download_client.qbittorrent.username", "")
	viper.SetDefault("download_client.qbittorrent.password", "")
	viper.SetDefault("download_client.qbittorrent.use_https", false)
	viper.SetDefault("download_client.sabnzbd.host", "")
	viper.SetDefault("download_client.sabnzbd.port", 0)
	viper.SetDefault("download_client.sabnzbd.api_key", "")
	viper.SetDefault("download_client.sabnzbd.use_https", false)
	viper.SetDefault("supported_extensions", []string{
		".m4b", ".mp3", ".m4a", ".aac", ".ogg", ".flac", ".wma",
	})
	viper.SetDefault("exclude_patterns", []string{})

	supportedExtensions := []string{
		".m4b", ".mp3", ".m4a", ".aac", ".ogg", ".flac", ".wma",
	}
	if viper.IsSet("supported_extensions") {
		supportedExtensions = viper.GetStringSlice("supported_extensions")
	}
	excludePatterns := viper.GetStringSlice("exclude_patterns")

	AppConfig = Config{
		// Core paths
		RootDir:       viper.GetString("root_dir"),
		DatabasePath:  viper.GetString("database_path"),
		DatabaseType:  viper.GetString("database_type"),
		EnableSQLite:  viper.GetBool("enable_sqlite3_i_know_the_risks"),
		PlaylistDir:   viper.GetString("playlist_dir"),
		SetupComplete: viper.GetBool("setup_complete"),

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

		// AI parsing
		EnableAIParsing: viper.GetBool("enable_ai_parsing"),
		OpenAIAPIKey:    viper.GetString("openai_api_key"),

		// Performance
		ConcurrentScans: viper.GetInt("concurrent_scans"),

		// Memory management
		MemoryLimitType:    viper.GetString("memory_limit_type"),
		CacheSize:          viper.GetInt("cache_size"),
		MemoryLimitPercent: viper.GetInt("memory_limit_percent"),
		MemoryLimitMB:      viper.GetInt("memory_limit_mb"),

		// Lifecycle / retention
		PurgeSoftDeletedAfterDays:   viper.GetInt("purge_soft_deleted_after_days"),
		PurgeSoftDeletedDeleteFiles: viper.GetBool("purge_soft_deleted_delete_files"),

		// Logging
		LogLevel:          viper.GetString("log_level"),
		LogFormat:         viper.GetString("log_format"),
		EnableJsonLogging: viper.GetBool("enable_json_logging"),

		// Download client integration
		DownloadClient: DownloadClientConfig{
			Type: viper.GetString("download_client.type"),
			Deluge: DelugeConfig{
				Host:     viper.GetString("download_client.deluge.host"),
				Port:     viper.GetInt("download_client.deluge.port"),
				Username: viper.GetString("download_client.deluge.username"),
				Password: viper.GetString("download_client.deluge.password"),
			},
			QBittorrent: QBittorrentConfig{
				Host:     viper.GetString("download_client.qbittorrent.host"),
				Port:     viper.GetInt("download_client.qbittorrent.port"),
				Username: viper.GetString("download_client.qbittorrent.username"),
				Password: viper.GetString("download_client.qbittorrent.password"),
				UseHTTPS: viper.GetBool("download_client.qbittorrent.use_https"),
			},
			SABnzbd: SABnzbdConfig{
				Host:     viper.GetString("download_client.sabnzbd.host"),
				Port:     viper.GetInt("download_client.sabnzbd.port"),
				APIKey:   viper.GetString("download_client.sabnzbd.api_key"),
				UseHTTPS: viper.GetBool("download_client.sabnzbd.use_https"),
			},
		},

		SupportedExtensions: supportedExtensions,
		ExcludePatterns:     excludePatterns,
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
