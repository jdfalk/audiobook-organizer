// file: internal/config/persistence.go
// version: 1.6.0
// guid: 9c8d7e6f-5a4b-3c2d-1e0f-9a8b7c6d5e4f

package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// ConfigFilePath returns the path to the YAML config file next to the database.
func ConfigFilePath() string {
	if AppConfig.DatabasePath != "" {
		return filepath.Join(filepath.Dir(AppConfig.DatabasePath), "config.yaml")
	}
	if AppConfig.RootDir != "" {
		return filepath.Join(AppConfig.RootDir, "config.yaml")
	}
	return ""
}

// LoadConfigFromFile loads settings from the YAML config file as a fallback.
// Called after LoadConfigFromDatabase so file values only fill in gaps.
func LoadConfigFromFile() error {
	path := ConfigFilePath()
	if path == "" {
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read config file: %w", err)
	}

	var fileConfig map[string]any
	if err := yaml.Unmarshal(data, &fileConfig); err != nil {
		log.Printf("Warning: Failed to parse config file %s: %v", path, err)
		return nil
	}

	applied := 0

	// Only fill in values that are currently empty/default.
	// This is the fallback path when DB decryption fails for secrets.
	stringFallbacks := map[string]*string{
		"openai_api_key":     &AppConfig.OpenAIAPIKey,
		"google_books_api_key": &AppConfig.GoogleBooksAPIKey,
		"hardcover_api_token": &AppConfig.HardcoverAPIToken,
		"root_dir":           &AppConfig.RootDir,
		"language":           &AppConfig.Language,
	}
	for key, ptr := range stringFallbacks {
		if *ptr == "" {
			if val, ok := fileConfig[key].(string); ok && val != "" {
				*ptr = val
				applied++
				log.Printf("[INFO] Loaded %s from config file", key)
			}
		}
	}

	if !AppConfig.EnableAIParsing {
		if val, ok := fileConfig["enable_ai_parsing"].(bool); ok && val {
			AppConfig.EnableAIParsing = true
			applied++
			log.Printf("[INFO] Loaded enable_ai_parsing from config file")
		}
	}

	if applied > 0 {
		log.Printf("Applied %d settings from config file %s", applied, path)
	}
	return nil
}

// SaveConfigToFile writes key settings to a YAML config file next to the database.
// Secrets are stored in plaintext here — file permissions restrict access.
func SaveConfigToFile() error {
	path := ConfigFilePath()
	if path == "" {
		return fmt.Errorf("cannot determine config file path")
	}

	fileConfig := map[string]any{
		"root_dir":              AppConfig.RootDir,
		"database_path":         AppConfig.DatabasePath,
		"playlist_dir":          AppConfig.PlaylistDir,
		"setup_complete":        AppConfig.SetupComplete,
		"organization_strategy": AppConfig.OrganizationStrategy,
		"scan_on_startup":       AppConfig.ScanOnStartup,
		"auto_organize":         AppConfig.AutoOrganize,
		"folder_naming_pattern": AppConfig.FolderNamingPattern,
		"file_naming_pattern":   AppConfig.FileNamingPattern,
		"auto_fetch_metadata":   AppConfig.AutoFetchMetadata,
		"language":              AppConfig.Language,
		"enable_ai_parsing":     AppConfig.EnableAIParsing,
		"concurrent_scans":      AppConfig.ConcurrentScans,
		"log_level":             AppConfig.LogLevel,
	}

	// Only write secrets if they're set (plaintext in file, file permissions protect them)
	if AppConfig.OpenAIAPIKey != "" {
		fileConfig["openai_api_key"] = AppConfig.OpenAIAPIKey
	}
	if AppConfig.GoogleBooksAPIKey != "" {
		fileConfig["google_books_api_key"] = AppConfig.GoogleBooksAPIKey
	}
	if AppConfig.HardcoverAPIToken != "" {
		fileConfig["hardcover_api_token"] = AppConfig.HardcoverAPIToken
	}

	data, err := yaml.Marshal(fileConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("failed to create config dir: %w", err)
	}

	// Write with restrictive permissions since it may contain secrets
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	log.Printf("Configuration saved to file: %s", path)
	return nil
}

// LoadConfigFromDatabase loads settings from database and applies them to AppConfig
// This is called after database initialization to override defaults with persisted values
func LoadConfigFromDatabase(store database.Store) error {
	if store == nil {
		return fmt.Errorf("store is nil")
	}

	log.Println("Loading configuration from database...")

	// Get all settings
	settings, err := store.GetAllSettings()
	if err != nil {
		// If table doesn't exist yet or is empty, that's OK
		log.Printf("Note: Could not load settings from database: %v", err)
		return nil
	}

	// Apply each setting
	applied := 0
	for _, setting := range settings {
		value := setting.Value

		if setting.Key == "openai_api_key" || setting.Key == "enable_ai_parsing" {
			log.Printf("[DEBUG] LoadConfigFromDatabase: found setting %s (isSecret=%v, valueLen=%d)",
				setting.Key, setting.IsSecret, len(setting.Value))
		}

		// Decrypt if secret
		if setting.IsSecret {
			decrypted, err := database.DecryptValue(value)
			if err != nil {
				log.Printf("WARNING: Failed to decrypt setting %q — will try config file fallback. (error: %v)",
					setting.Key, err)
				continue
			}
			value = decrypted
		}

		// Apply to AppConfig based on key
		if err := applySetting(setting.Key, value, setting.Type); err != nil {
			log.Printf("Warning: Failed to apply setting %s: %v", setting.Key, err)
			continue
		}
		applied++
	}

	log.Printf("Applied %d settings from database", applied)

	// Fall back to config file for anything the DB didn't provide (e.g. corrupted secrets)
	if err := LoadConfigFromFile(); err != nil {
		log.Printf("Warning: Config file fallback failed: %v", err)
	}

	log.Printf("[DEBUG] After config load: EnableAIParsing=%v, OpenAIAPIKey length=%d",
		AppConfig.EnableAIParsing, len(AppConfig.OpenAIAPIKey))

	// Re-derive defaults that depend on RootDir
	if AppConfig.OpenLibraryDumpDir == "" && AppConfig.RootDir != "" {
		AppConfig.OpenLibraryDumpDir = filepath.Join(AppConfig.RootDir, "openlibrary-dumps")
	}

	return nil
}

// applySetting applies a single setting to AppConfig
func applySetting(key, value, typ string) error {
	switch key {
	// Core paths
	case "root_dir":
		AppConfig.RootDir = value
	case "database_path":
		AppConfig.DatabasePath = value
	case "playlist_dir":
		AppConfig.PlaylistDir = value
	case "setup_complete":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.SetupComplete = b
		}

	// Organization
	case "organization_strategy":
		AppConfig.OrganizationStrategy = value
	case "scan_on_startup":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.ScanOnStartup = b
		}
	case "auto_organize":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.AutoOrganize = b
		}
	case "folder_naming_pattern":
		AppConfig.FolderNamingPattern = value
	case "file_naming_pattern":
		AppConfig.FileNamingPattern = value
	case "create_backups":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.CreateBackups = b
		}
	case "supported_extensions":
		var extensions []string
		if err := json.Unmarshal([]byte(value), &extensions); err == nil {
			if len(extensions) > 0 {
				AppConfig.SupportedExtensions = extensions
			}
		}
	case "exclude_patterns":
		var patterns []string
		if err := json.Unmarshal([]byte(value), &patterns); err == nil {
			AppConfig.ExcludePatterns = patterns
		}

	// Storage quotas
	case "enable_disk_quota":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.EnableDiskQuota = b
		}
	case "disk_quota_percent":
		if i, err := strconv.Atoi(value); err == nil {
			AppConfig.DiskQuotaPercent = i
		}
	case "enable_user_quotas":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.EnableUserQuotas = b
		}
	case "default_user_quota_gb":
		if i, err := strconv.Atoi(value); err == nil {
			AppConfig.DefaultUserQuotaGB = i
		}

	// Metadata
	case "auto_fetch_metadata":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.AutoFetchMetadata = b
		}
	case "language":
		AppConfig.Language = value

	// Open Library dumps
	case "openlibrary_dump_enabled":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.OpenLibraryDumpEnabled = b
		}
	case "openlibrary_dump_dir":
		AppConfig.OpenLibraryDumpDir = value

	// Hardcover.app
	case "hardcover_api_token":
		AppConfig.HardcoverAPIToken = value

	// AI parsing
	case "enable_ai_parsing":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.EnableAIParsing = b
		}
	case "openai_api_key":
		AppConfig.OpenAIAPIKey = value
	case "google_books_api_key":
		AppConfig.GoogleBooksAPIKey = value

	// Performance
	case "concurrent_scans":
		if i, err := strconv.Atoi(value); err == nil {
			AppConfig.ConcurrentScans = i
		}
	case "operation_timeout_minutes":
		if i, err := strconv.Atoi(value); err == nil {
			AppConfig.OperationTimeoutMinutes = i
		}
	case "api_rate_limit_per_minute":
		if i, err := strconv.Atoi(value); err == nil {
			AppConfig.APIRateLimitPerMinute = i
		}
	case "auth_rate_limit_per_minute":
		if i, err := strconv.Atoi(value); err == nil {
			AppConfig.AuthRateLimitPerMinute = i
		}
	case "json_body_limit_mb":
		if i, err := strconv.Atoi(value); err == nil {
			AppConfig.JSONBodyLimitMB = i
		}
	case "upload_body_limit_mb":
		if i, err := strconv.Atoi(value); err == nil {
			AppConfig.UploadBodyLimitMB = i
		}
	case "enable_auth":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.EnableAuth = b
		}
	case "write_back_metadata":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.WriteBackMetadata = b
		}
	case "embed_cover_art":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.EmbedCoverArt = b
		}
	case "auto_scan_enabled":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.AutoScanEnabled = b
		}
	case "auto_scan_debounce_seconds":
		if i, err := strconv.Atoi(value); err == nil {
			AppConfig.AutoScanDebounceSeconds = i
		}

	// Memory management
	case "memory_limit_type":
		AppConfig.MemoryLimitType = value
	case "cache_size":
		if i, err := strconv.Atoi(value); err == nil {
			AppConfig.CacheSize = i
		}
	case "memory_limit_percent":
		if i, err := strconv.Atoi(value); err == nil {
			AppConfig.MemoryLimitPercent = i
		}
	case "memory_limit_mb":
		if i, err := strconv.Atoi(value); err == nil {
			AppConfig.MemoryLimitMB = i
		}

	// Logging
	case "log_level":
		AppConfig.LogLevel = value
	case "log_format":
		AppConfig.LogFormat = value
	case "enable_json_logging":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.EnableJsonLogging = b
		}

	// Auto-update
	case "auto_update_enabled":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.AutoUpdateEnabled = b
		}
	case "auto_update_channel":
		AppConfig.AutoUpdateChannel = value
	case "auto_update_check_minutes":
		if i, err := strconv.Atoi(value); err == nil {
			AppConfig.AutoUpdateCheckMinutes = i
		}
	case "auto_update_window_start":
		if i, err := strconv.Atoi(value); err == nil {
			AppConfig.AutoUpdateWindowStart = i
		}
	case "auto_update_window_end":
		if i, err := strconv.Atoi(value); err == nil {
			AppConfig.AutoUpdateWindowEnd = i
		}

	// Lifecycle / retention
	case "purge_soft_deleted_after_days":
		if i, err := strconv.Atoi(value); err == nil {
			AppConfig.PurgeSoftDeletedAfterDays = i
		}
	case "purge_soft_deleted_delete_files":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.PurgeSoftDeletedDeleteFiles = b
		}

	// iTunes sync
	case "itunes_sync_enabled":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.ITunesSyncEnabled = b
		}
	case "itunes_sync_interval":
		if i, err := strconv.Atoi(value); err == nil {
			AppConfig.ITunesSyncInterval = i
		}
	case "itl_write_back_enabled":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.ITLWriteBackEnabled = b
		}
	case "itunes_library_itl_path":
		AppConfig.ITunesLibraryITLPath = value
	case "itunes_library_xml_path":
		AppConfig.ITunesLibraryXMLPath = value
	case "itunes_auto_write_back":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.ITunesAutoWriteBack = b
		}
	case "itunes_path_mappings":
		var mappings []ITunesPathMap
		if err := json.Unmarshal([]byte(value), &mappings); err == nil {
			AppConfig.ITunesPathMappings = mappings
		}

	// Basic auth
	case "basic_auth_enabled":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.BasicAuthEnabled = b
		}
	case "basic_auth_username":
		AppConfig.BasicAuthUsername = value
	case "basic_auth_password":
		AppConfig.BasicAuthPassword = value

	default:
		return fmt.Errorf("unknown setting key: %s", key)
	}

	return nil
}

// SaveConfigToDatabase persists current AppConfig to database AND config file.
// This should be called whenever config is modified via API.
func SaveConfigToDatabase(store database.Store) error {
	if store == nil {
		return fmt.Errorf("store is nil")
	}

	log.Println("Saving configuration to database...")

	pathMappingsJSON, err := json.Marshal(AppConfig.ITunesPathMappings)
	if err != nil {
		return fmt.Errorf("failed to marshal itunes_path_mappings: %w", err)
	}

	extensionsJSON, err := json.Marshal(AppConfig.SupportedExtensions)
	if err != nil {
		return fmt.Errorf("failed to marshal supported_extensions: %w", err)
	}
	excludeJSON, err := json.Marshal(AppConfig.ExcludePatterns)
	if err != nil {
		return fmt.Errorf("failed to marshal exclude_patterns: %w", err)
	}

	settings := map[string]struct {
		value    string
		typ      string
		isSecret bool
	}{
		// Core paths
		"root_dir":      {AppConfig.RootDir, "string", false},
		"database_path": {AppConfig.DatabasePath, "string", false},
		"playlist_dir":  {AppConfig.PlaylistDir, "string", false},
		"setup_complete": {strconv.FormatBool(AppConfig.SetupComplete), "bool", false},

		// Organization
		"organization_strategy": {AppConfig.OrganizationStrategy, "string", false},
		"scan_on_startup":       {strconv.FormatBool(AppConfig.ScanOnStartup), "bool", false},
		"auto_organize":         {strconv.FormatBool(AppConfig.AutoOrganize), "bool", false},
		"folder_naming_pattern": {AppConfig.FolderNamingPattern, "string", false},
		"file_naming_pattern":   {AppConfig.FileNamingPattern, "string", false},
		"create_backups":        {strconv.FormatBool(AppConfig.CreateBackups), "bool", false},
		"supported_extensions":  {string(extensionsJSON), "json", false},
		"exclude_patterns":      {string(excludeJSON), "json", false},

		// Storage quotas
		"enable_disk_quota":     {strconv.FormatBool(AppConfig.EnableDiskQuota), "bool", false},
		"disk_quota_percent":    {strconv.Itoa(AppConfig.DiskQuotaPercent), "int", false},
		"enable_user_quotas":    {strconv.FormatBool(AppConfig.EnableUserQuotas), "bool", false},
		"default_user_quota_gb": {strconv.Itoa(AppConfig.DefaultUserQuotaGB), "int", false},

		// Metadata
		"auto_fetch_metadata": {strconv.FormatBool(AppConfig.AutoFetchMetadata), "bool", false},
		"language":            {AppConfig.Language, "string", false},

		// Open Library dumps
		"openlibrary_dump_enabled": {strconv.FormatBool(AppConfig.OpenLibraryDumpEnabled), "bool", false},
		"openlibrary_dump_dir":     {AppConfig.OpenLibraryDumpDir, "string", false},

		// Hardcover.app
		"hardcover_api_token": {AppConfig.HardcoverAPIToken, "string", true},

		// AI parsing (API key is secret in DB, plaintext in file)
		"enable_ai_parsing": {strconv.FormatBool(AppConfig.EnableAIParsing), "bool", false},
		"openai_api_key":        {AppConfig.OpenAIAPIKey, "string", true},
		"google_books_api_key":  {AppConfig.GoogleBooksAPIKey, "string", true},

		// Performance
		"concurrent_scans":           {strconv.Itoa(AppConfig.ConcurrentScans), "int", false},
		"operation_timeout_minutes":  {strconv.Itoa(AppConfig.OperationTimeoutMinutes), "int", false},
		"api_rate_limit_per_minute":  {strconv.Itoa(AppConfig.APIRateLimitPerMinute), "int", false},
		"auth_rate_limit_per_minute": {strconv.Itoa(AppConfig.AuthRateLimitPerMinute), "int", false},
		"json_body_limit_mb":         {strconv.Itoa(AppConfig.JSONBodyLimitMB), "int", false},
		"upload_body_limit_mb":       {strconv.Itoa(AppConfig.UploadBodyLimitMB), "int", false},
		"enable_auth":                {strconv.FormatBool(AppConfig.EnableAuth), "bool", false},
		"write_back_metadata":        {strconv.FormatBool(AppConfig.WriteBackMetadata), "bool", false},
		"embed_cover_art":            {strconv.FormatBool(AppConfig.EmbedCoverArt), "bool", false},
		"auto_scan_enabled":          {strconv.FormatBool(AppConfig.AutoScanEnabled), "bool", false},
		"auto_scan_debounce_seconds": {strconv.Itoa(AppConfig.AutoScanDebounceSeconds), "int", false},

		// Memory management
		"memory_limit_type":    {AppConfig.MemoryLimitType, "string", false},
		"cache_size":           {strconv.Itoa(AppConfig.CacheSize), "int", false},
		"memory_limit_percent": {strconv.Itoa(AppConfig.MemoryLimitPercent), "int", false},
		"memory_limit_mb":      {strconv.Itoa(AppConfig.MemoryLimitMB), "int", false},

		// Lifecycle / retention
		"purge_soft_deleted_after_days":   {strconv.Itoa(AppConfig.PurgeSoftDeletedAfterDays), "int", false},
		"purge_soft_deleted_delete_files": {strconv.FormatBool(AppConfig.PurgeSoftDeletedDeleteFiles), "bool", false},

		// Logging
		"log_level":           {AppConfig.LogLevel, "string", false},
		"log_format":          {AppConfig.LogFormat, "string", false},
		"enable_json_logging": {strconv.FormatBool(AppConfig.EnableJsonLogging), "bool", false},

		// Auto-update
		"auto_update_enabled":       {strconv.FormatBool(AppConfig.AutoUpdateEnabled), "bool", false},
		"auto_update_channel":       {AppConfig.AutoUpdateChannel, "string", false},
		"auto_update_check_minutes": {strconv.Itoa(AppConfig.AutoUpdateCheckMinutes), "int", false},
		"auto_update_window_start":  {strconv.Itoa(AppConfig.AutoUpdateWindowStart), "int", false},
		"auto_update_window_end":    {strconv.Itoa(AppConfig.AutoUpdateWindowEnd), "int", false},

		// Basic auth
		// iTunes sync
		"itunes_sync_enabled":    {strconv.FormatBool(AppConfig.ITunesSyncEnabled), "bool", false},
		"itunes_sync_interval":   {strconv.Itoa(AppConfig.ITunesSyncInterval), "int", false},
		"itl_write_back_enabled": {strconv.FormatBool(AppConfig.ITLWriteBackEnabled), "bool", false},
		"itunes_library_itl_path": {AppConfig.ITunesLibraryITLPath, "string", false},
		"itunes_library_xml_path": {AppConfig.ITunesLibraryXMLPath, "string", false},
		"itunes_auto_write_back":  {strconv.FormatBool(AppConfig.ITunesAutoWriteBack), "bool", false},
		"itunes_path_mappings":    {string(pathMappingsJSON), "json", false},

		"basic_auth_enabled":  {strconv.FormatBool(AppConfig.BasicAuthEnabled), "bool", false},
		"basic_auth_username": {AppConfig.BasicAuthUsername, "string", false},
		"basic_auth_password": {AppConfig.BasicAuthPassword, "string", true},
	}

	saved := 0
	for key, s := range settings {
		// For secrets: if the current value is empty, check if there's already
		// a value in the DB and preserve it. This prevents accidental deletion
		// when encryption fails to decrypt on load.
		if s.isSecret && s.value == "" {
			existing, err := store.GetSetting(key)
			if err == nil && existing != nil && existing.Value != "" {
				log.Printf("[DEBUG] Preserving existing secret %s in database (current AppConfig value is empty)", key)
				continue
			}
		}

		if err := store.SetSetting(key, s.value, s.typ, s.isSecret); err != nil {
			log.Printf("Warning: Failed to save setting %s: %v", key, err)
			continue
		}
		saved++
	}

	log.Printf("Saved %d settings to database", saved)

	// Also save to config file as a reliable fallback
	if err := SaveConfigToFile(); err != nil {
		log.Printf("Warning: Failed to save config file: %v", err)
	}

	return nil
}

// SyncConfigFromEnv loads env vars from viper and overrides AppConfig (without saving to DB).
// Only non-empty env values override DB-loaded values. This prevents empty env vars or
// viper defaults from wiping out API keys that were loaded from the database.
func SyncConfigFromEnv() {
	if viper.IsSet("root_dir") {
		if val := viper.GetString("root_dir"); val != "" {
			AppConfig.RootDir = val
		}
	}
	if viper.IsSet("openai_api_key") {
		if val := viper.GetString("openai_api_key"); val != "" {
			AppConfig.OpenAIAPIKey = val
			log.Printf("[DEBUG] SyncConfigFromEnv: overriding OpenAI API key from env/config (length: %d)", len(val))
		}
	}
	if viper.IsSet("google_books_api_key") {
		if val := viper.GetString("google_books_api_key"); val != "" {
			AppConfig.GoogleBooksAPIKey = val
		}
	}
	if viper.IsSet("enable_ai_parsing") {
		AppConfig.EnableAIParsing = viper.GetBool("enable_ai_parsing")
	}
	// Add more env overrides as needed
}
