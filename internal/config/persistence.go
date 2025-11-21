// file: internal/config/persistence.go
// version: 1.1.0
// guid: 9c8d7e6f-5a4b-3c2d-1e0f-9a8b7c6d5e4f

package config

import (
	"fmt"
	"log"
	"strconv"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/spf13/viper"
)

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

		// Decrypt if secret
		if setting.IsSecret {
			decrypted, err := database.DecryptValue(value)
			if err != nil {
				log.Printf("Warning: Failed to decrypt setting %s: %v", setting.Key, err)
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

	// AI parsing
	case "enable_ai_parsing":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.EnableAIParsing = b
		}
	case "openai_api_key":
		AppConfig.OpenAIAPIKey = value

	// Performance
	case "concurrent_scans":
		if i, err := strconv.Atoi(value); err == nil {
			AppConfig.ConcurrentScans = i
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

	// API Keys
	case "goodreads_api_key":
		AppConfig.APIKeys.Goodreads = value

	default:
		return fmt.Errorf("unknown setting key: %s", key)
	}

	return nil
}

// SaveConfigToDatabase persists current AppConfig to database
// This should be called whenever config is modified via API
func SaveConfigToDatabase(store database.Store) error {
	if store == nil {
		return fmt.Errorf("store is nil")
	}

	log.Println("Saving configuration to database...")

	settings := map[string]struct {
		value    string
		typ      string
		isSecret bool
	}{
		// Core paths
		"root_dir":      {AppConfig.RootDir, "string", false},
		"database_path": {AppConfig.DatabasePath, "string", false},
		"playlist_dir":  {AppConfig.PlaylistDir, "string", false},

		// Organization
		"organization_strategy": {AppConfig.OrganizationStrategy, "string", false},
		"scan_on_startup":       {strconv.FormatBool(AppConfig.ScanOnStartup), "bool", false},
		"auto_organize":         {strconv.FormatBool(AppConfig.AutoOrganize), "bool", false},
		"folder_naming_pattern": {AppConfig.FolderNamingPattern, "string", false},
		"file_naming_pattern":   {AppConfig.FileNamingPattern, "string", false},
		"create_backups":        {strconv.FormatBool(AppConfig.CreateBackups), "bool", false},

		// Storage quotas
		"enable_disk_quota":     {strconv.FormatBool(AppConfig.EnableDiskQuota), "bool", false},
		"disk_quota_percent":    {strconv.Itoa(AppConfig.DiskQuotaPercent), "int", false},
		"enable_user_quotas":    {strconv.FormatBool(AppConfig.EnableUserQuotas), "bool", false},
		"default_user_quota_gb": {strconv.Itoa(AppConfig.DefaultUserQuotaGB), "int", false},

		// Metadata
		"auto_fetch_metadata": {strconv.FormatBool(AppConfig.AutoFetchMetadata), "bool", false},
		"language":            {AppConfig.Language, "string", false},

		// AI parsing (API key is secret)
		"enable_ai_parsing": {strconv.FormatBool(AppConfig.EnableAIParsing), "bool", false},
		"openai_api_key":    {AppConfig.OpenAIAPIKey, "string", true},

		// Performance
		"concurrent_scans": {strconv.Itoa(AppConfig.ConcurrentScans), "int", false},

		// Memory management
		"memory_limit_type":    {AppConfig.MemoryLimitType, "string", false},
		"cache_size":           {strconv.Itoa(AppConfig.CacheSize), "int", false},
		"memory_limit_percent": {strconv.Itoa(AppConfig.MemoryLimitPercent), "int", false},
		"memory_limit_mb":      {strconv.Itoa(AppConfig.MemoryLimitMB), "int", false},

		// Logging
		"log_level":           {AppConfig.LogLevel, "string", false},
		"log_format":          {AppConfig.LogFormat, "string", false},
		"enable_json_logging": {strconv.FormatBool(AppConfig.EnableJsonLogging), "bool", false},

		// API Keys
		"goodreads_api_key": {AppConfig.APIKeys.Goodreads, "string", true},
	}

	log.Printf("[DEBUG] SaveConfigToDatabase: OpenAI key length: %d, EnableAIParsing: %v", len(AppConfig.OpenAIAPIKey), AppConfig.EnableAIParsing)

	saved := 0
	for key, s := range settings {
		// Skip empty secrets
		if s.isSecret && s.value == "" {
			log.Printf("[DEBUG] SaveConfigToDatabase: Skipping empty secret: %s", key)
			continue
		}

		if key == "openai_api_key" {
			log.Printf("[DEBUG] SaveConfigToDatabase: Saving OpenAI key (length: %d, secret: %v)", len(s.value), s.isSecret)
		}

		if err := store.SetSetting(key, s.value, s.typ, s.isSecret); err != nil {
			log.Printf("Warning: Failed to save setting %s: %v", key, err)
			continue
		}
		saved++
	}

	log.Printf("Saved %d settings to database", saved)
	return nil
}

// SyncConfigFromEnv loads env vars from viper and overrides AppConfig (without saving to DB)
// This is useful for command-line overrides or environment-specific settings
func SyncConfigFromEnv() {
	if viper.IsSet("root_dir") {
		AppConfig.RootDir = viper.GetString("root_dir")
	}
	if viper.IsSet("openai_api_key") {
		AppConfig.OpenAIAPIKey = viper.GetString("openai_api_key")
	}
	if viper.IsSet("enable_ai_parsing") {
		AppConfig.EnableAIParsing = viper.GetBool("enable_ai_parsing")
	}
	// Add more env overrides as needed
}
