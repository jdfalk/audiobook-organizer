// file: internal/config/config.go
// version: 1.10.1
// guid: 7b8c9d0e-1f2a-3b4c-5d6e-7f8a9b0c1d2e

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

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
	Torrent TorrentClientConfig `json:"torrent"`
	Usenet  UsenetClientConfig  `json:"usenet"`
}

// TorrentClientConfig holds torrent client configuration.
type TorrentClientConfig struct {
	Type        string            `json:"type"`
	Deluge      DelugeConfig      `json:"deluge"`
	QBittorrent QBittorrentConfig `json:"qbittorrent"`
}

// UsenetClientConfig holds Usenet client configuration.
type UsenetClientConfig struct {
	Type    string        `json:"type"`
	SABnzbd SABnzbdConfig `json:"sabnzbd"`
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
	OrganizationStrategy    string `json:"organization_strategy"` // 'auto', 'copy', 'hardlink', 'reflink', 'symlink'
	ScanOnStartup           bool   `json:"scan_on_startup"`
	AutoOrganize            bool   `json:"auto_organize"`
	AutoScanEnabled         bool   `json:"auto_scan_enabled"`
	AutoScanDebounceSeconds int    `json:"auto_scan_debounce_seconds"`
	FolderNamingPattern     string `json:"folder_naming_pattern"`
	FileNamingPattern       string `json:"file_naming_pattern"`
	CreateBackups           bool   `json:"create_backups"`

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
	// Background operation timeout in minutes (0 disables timeout)
	OperationTimeoutMinutes int `json:"operation_timeout_minutes"`

	// API limits
	APIRateLimitPerMinute  int  `json:"api_rate_limit_per_minute"`
	AuthRateLimitPerMinute int  `json:"auth_rate_limit_per_minute"`
	JSONBodyLimitMB        int  `json:"json_body_limit_mb"`
	UploadBodyLimitMB      int  `json:"upload_body_limit_mb"`
	EnableAuth             bool `json:"enable_auth"`

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
	viper.SetDefault("auto_scan_enabled", false)
	viper.SetDefault("auto_scan_debounce_seconds", 30)
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
	viper.SetDefault("operation_timeout_minutes", 30)

	// API security/runtime limits
	viper.SetDefault("api_rate_limit_per_minute", 100)
	viper.SetDefault("auth_rate_limit_per_minute", 10)
	viper.SetDefault("json_body_limit_mb", 1)
	viper.SetDefault("upload_body_limit_mb", 10)
	viper.SetDefault("enable_auth", true)

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
	viper.SetDefault("download_client.torrent.type", "")
	viper.SetDefault("download_client.torrent.deluge.host", "")
	viper.SetDefault("download_client.torrent.deluge.port", 0)
	viper.SetDefault("download_client.torrent.deluge.username", "")
	viper.SetDefault("download_client.torrent.deluge.password", "")
	viper.SetDefault("download_client.torrent.qbittorrent.host", "")
	viper.SetDefault("download_client.torrent.qbittorrent.port", 0)
	viper.SetDefault("download_client.torrent.qbittorrent.username", "")
	viper.SetDefault("download_client.torrent.qbittorrent.password", "")
	viper.SetDefault("download_client.torrent.qbittorrent.use_https", false)
	viper.SetDefault("download_client.usenet.type", "")
	viper.SetDefault("download_client.usenet.sabnzbd.host", "")
	viper.SetDefault("download_client.usenet.sabnzbd.port", 0)
	viper.SetDefault("download_client.usenet.sabnzbd.api_key", "")
	viper.SetDefault("download_client.usenet.sabnzbd.use_https", false)
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
		OrganizationStrategy:    viper.GetString("organization_strategy"),
		ScanOnStartup:           viper.GetBool("scan_on_startup"),
		AutoOrganize:            viper.GetBool("auto_organize"),
		AutoScanEnabled:         viper.GetBool("auto_scan_enabled"),
		AutoScanDebounceSeconds: viper.GetInt("auto_scan_debounce_seconds"),
		FolderNamingPattern:     viper.GetString("folder_naming_pattern"),
		FileNamingPattern:       viper.GetString("file_naming_pattern"),
		CreateBackups:           viper.GetBool("create_backups"),

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
		ConcurrentScans:         viper.GetInt("concurrent_scans"),
		OperationTimeoutMinutes: viper.GetInt("operation_timeout_minutes"),
		APIRateLimitPerMinute:   viper.GetInt("api_rate_limit_per_minute"),
		AuthRateLimitPerMinute:  viper.GetInt("auth_rate_limit_per_minute"),
		JSONBodyLimitMB:         viper.GetInt("json_body_limit_mb"),
		UploadBodyLimitMB:       viper.GetInt("upload_body_limit_mb"),
		EnableAuth:              viper.GetBool("enable_auth"),

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
			Torrent: TorrentClientConfig{
				Type: viper.GetString("download_client.torrent.type"),
				Deluge: DelugeConfig{
					Host:     viper.GetString("download_client.torrent.deluge.host"),
					Port:     viper.GetInt("download_client.torrent.deluge.port"),
					Username: viper.GetString("download_client.torrent.deluge.username"),
					Password: viper.GetString("download_client.torrent.deluge.password"),
				},
				QBittorrent: QBittorrentConfig{
					Host:     viper.GetString("download_client.torrent.qbittorrent.host"),
					Port:     viper.GetInt("download_client.torrent.qbittorrent.port"),
					Username: viper.GetString("download_client.torrent.qbittorrent.username"),
					Password: viper.GetString("download_client.torrent.qbittorrent.password"),
					UseHTTPS: viper.GetBool("download_client.torrent.qbittorrent.use_https"),
				},
			},
			Usenet: UsenetClientConfig{
				Type: viper.GetString("download_client.usenet.type"),
				SABnzbd: SABnzbdConfig{
					Host:     viper.GetString("download_client.usenet.sabnzbd.host"),
					Port:     viper.GetInt("download_client.usenet.sabnzbd.port"),
					APIKey:   viper.GetString("download_client.usenet.sabnzbd.api_key"),
					UseHTTPS: viper.GetBool("download_client.usenet.sabnzbd.use_https"),
				},
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

var validPatternPlaceholder = regexp.MustCompile(`\{[A-Za-z0-9_]+\}`)

func hasBalancedBraces(value string) bool {
	return strings.Count(value, "{") == strings.Count(value, "}")
}

func validateNamingPattern(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("pattern cannot be empty")
	}
	if !hasBalancedBraces(trimmed) {
		return fmt.Errorf("unbalanced braces in pattern")
	}
	withoutPlaceholders := validPatternPlaceholder.ReplaceAllString(trimmed, "")
	if strings.Contains(withoutPlaceholders, "{") || strings.Contains(withoutPlaceholders, "}") {
		return fmt.Errorf("invalid placeholder format in pattern")
	}
	return nil
}

func validateParentDirExists(path string, field string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	parent := filepath.Dir(path)
	info, err := os.Stat(parent)
	if err != nil {
		return fmt.Errorf("%s parent directory %q does not exist", field, parent)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s parent path %q is not a directory", field, parent)
	}
	return nil
}

func validateParentDirWritable(path string, field string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	parent := filepath.Dir(path)
	testFile, err := os.CreateTemp(parent, ".ao-write-test-*")
	if err != nil {
		return fmt.Errorf("%s parent directory %q is not writable", field, parent)
	}
	testFile.Close()
	_ = os.Remove(testFile.Name())
	return nil
}

// Validate performs structural checks on runtime configuration values.
func (c *Config) Validate() error {
	if c == nil {
		return fmt.Errorf("config is nil")
	}

	var errs []string

	switch c.DatabaseType {
	case "pebble", "sqlite":
	default:
		errs = append(errs, "database_type must be 'pebble' or 'sqlite'")
	}

	if err := validateParentDirExists(c.DatabasePath, "database_path"); err != nil {
		errs = append(errs, err.Error())
	} else if err := validateParentDirWritable(c.DatabasePath, "database_path"); err != nil {
		errs = append(errs, err.Error())
	}

	if err := validateParentDirExists(c.PlaylistDir, "playlist_dir"); err != nil {
		errs = append(errs, err.Error())
	}

	if c.ConcurrentScans < 0 {
		errs = append(errs, "concurrent_scans must be >= 0")
	}
	if c.AutoScanDebounceSeconds < 0 {
		errs = append(errs, "auto_scan_debounce_seconds must be >= 0")
	}
	if c.OperationTimeoutMinutes < 0 {
		errs = append(errs, "operation_timeout_minutes must be >= 0")
	}
	if c.APIRateLimitPerMinute < 0 {
		errs = append(errs, "api_rate_limit_per_minute must be >= 0")
	}
	if c.AuthRateLimitPerMinute < 0 {
		errs = append(errs, "auth_rate_limit_per_minute must be >= 0")
	}
	if c.JSONBodyLimitMB < 0 {
		errs = append(errs, "json_body_limit_mb must be >= 0")
	}
	if c.UploadBodyLimitMB < 0 {
		errs = append(errs, "upload_body_limit_mb must be >= 0")
	}
	if c.EnableDiskQuota && (c.DiskQuotaPercent < 1 || c.DiskQuotaPercent > 100) {
		errs = append(errs, "disk_quota_percent must be between 1 and 100")
	}

	validStrategies := map[string]struct{}{
		"auto": {}, "copy": {}, "hardlink": {}, "reflink": {}, "symlink": {},
	}
	if c.OrganizationStrategy != "" {
		if _, ok := validStrategies[c.OrganizationStrategy]; !ok {
			errs = append(errs, "organization_strategy must be one of: auto, copy, hardlink, reflink, symlink")
		}
	}

	if strings.TrimSpace(c.FolderNamingPattern) != "" {
		if err := validateNamingPattern(c.FolderNamingPattern); err != nil {
			errs = append(errs, "folder_naming_pattern "+err.Error())
		}
	}
	if strings.TrimSpace(c.FileNamingPattern) != "" {
		if err := validateNamingPattern(c.FileNamingPattern); err != nil {
			errs = append(errs, "file_naming_pattern "+err.Error())
		}
	}

	for _, ext := range c.SupportedExtensions {
		if ext == "" {
			continue
		}
		if !strings.HasPrefix(ext, ".") {
			errs = append(errs, fmt.Sprintf("supported extension %q must start with '.'", ext))
			break
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("invalid configuration: %s", strings.Join(errs, "; "))
	}
	return nil
}

// ResetToDefaults resets the AppConfig to factory defaults
func ResetToDefaults() {
	AppConfig = Config{
		// Core paths
		RootDir:       AppConfig.RootDir,      // Keep existing paths
		DatabasePath:  AppConfig.DatabasePath, // Keep existing paths
		DatabaseType:  "pebble",
		EnableSQLite:  false,
		PlaylistDir:   AppConfig.PlaylistDir, // Keep existing paths
		SetupComplete: false,

		// Library organization
		OrganizationStrategy:    "auto",
		ScanOnStartup:           false,
		AutoOrganize:            true,
		AutoScanEnabled:         false,
		AutoScanDebounceSeconds: 30,
		FolderNamingPattern:     "{author}/{series}/{title} ({print_year})",
		FileNamingPattern:       "{title} - {author} - read by {narrator}",
		CreateBackups:           true,

		// Storage quotas
		EnableDiskQuota:    false,
		DiskQuotaPercent:   80,
		EnableUserQuotas:   false,
		DefaultUserQuotaGB: 100,

		// Metadata
		AutoFetchMetadata: true,
		Language:          "en",

		// AI parsing
		EnableAIParsing: false,
		OpenAIAPIKey:    "",

		// Performance
		ConcurrentScans:         4,
		OperationTimeoutMinutes: 30,
		APIRateLimitPerMinute:   100,
		AuthRateLimitPerMinute:  10,
		JSONBodyLimitMB:         1,
		UploadBodyLimitMB:       10,
		EnableAuth:              true,

		// Memory management
		MemoryLimitType:    "items",
		CacheSize:          1000,
		MemoryLimitPercent: 25,
		MemoryLimitMB:      512,

		// Lifecycle / retention
		PurgeSoftDeletedAfterDays:   30,
		PurgeSoftDeletedDeleteFiles: false,

		// Logging
		LogLevel:          "info",
		LogFormat:         "text",
		EnableJsonLogging: false,

		// Download client integration
		DownloadClient: DownloadClientConfig{
			Torrent: TorrentClientConfig{
				Type: "",
				Deluge: DelugeConfig{
					Host:     "",
					Port:     0,
					Username: "",
					Password: "",
				},
				QBittorrent: QBittorrentConfig{
					Host:     "",
					Port:     0,
					Username: "",
					Password: "",
					UseHTTPS: false,
				},
			},
			Usenet: UsenetClientConfig{
				Type: "",
				SABnzbd: SABnzbdConfig{
					Host:     "",
					Port:     0,
					APIKey:   "",
					UseHTTPS: false,
				},
			},
		},

		SupportedExtensions: []string{
			".m4b", ".mp3", ".m4a", ".aac", ".ogg", ".flac", ".wma",
		},
		ExcludePatterns: []string{},

		// Default metadata sources
		MetadataSources: []MetadataSource{
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
		},
	}
}
