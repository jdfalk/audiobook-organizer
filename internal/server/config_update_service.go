// file: internal/server/config_update_service.go
// version: 1.2.1
// guid: f6g7h8i9-j0k1-l2m3-n4o5-p6q7r8s9t0u1

package server

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

type ConfigUpdateService struct {
	db database.Store
}

func NewConfigUpdateService(db database.Store) *ConfigUpdateService {
	return &ConfigUpdateService{db: db}
}

// ValidateUpdate checks if the update payload has required fields
func (cus *ConfigUpdateService) ValidateUpdate(payload map[string]any) error {
	if len(payload) == 0 {
		return fmt.Errorf("no configuration updates provided")
	}
	return nil
}

// ExtractStringField extracts a string value from payload
func (cus *ConfigUpdateService) ExtractStringField(payload map[string]any, key string) (string, bool) {
	val, ok := payload[key]
	if !ok {
		return "", false
	}
	str, ok := val.(string)
	return str, ok
}

// ExtractBoolField extracts a bool value from payload
func (cus *ConfigUpdateService) ExtractBoolField(payload map[string]any, key string) (bool, bool) {
	val, ok := payload[key]
	if !ok {
		return false, false
	}
	b, ok := val.(bool)
	return b, ok
}

// ExtractIntField extracts an int value from payload (handling JSON float64)
func (cus *ConfigUpdateService) ExtractIntField(payload map[string]any, key string) (int, bool) {
	val, ok := payload[key]
	if !ok {
		return 0, false
	}
	f, ok := val.(float64)
	return int(f), ok
}

// ApplyUpdates applies all updates from payload to AppConfig
func (cus *ConfigUpdateService) ApplyUpdates(payload map[string]any) error {
	if payload == nil {
		return fmt.Errorf("configuration payload is required")
	}
	if _, ok := payload["database_type"]; ok {
		return fmt.Errorf("database_type cannot be changed at runtime")
	}
	if _, ok := payload["enable_sqlite"]; ok {
		return fmt.Errorf("enable_sqlite cannot be changed at runtime")
	}
	if rootDir, ok := cus.ExtractStringField(payload, "root_dir"); ok {
		config.AppConfig.RootDir = rootDir
	}

	if autoOrganize, ok := cus.ExtractBoolField(payload, "auto_organize"); ok {
		config.AppConfig.AutoOrganize = autoOrganize
	}

	if concurrentScans, ok := cus.ExtractIntField(payload, "concurrent_scans"); ok {
		config.AppConfig.ConcurrentScans = concurrentScans
	}
	if autoScanEnabled, ok := cus.ExtractBoolField(payload, "auto_scan_enabled"); ok {
		config.AppConfig.AutoScanEnabled = autoScanEnabled
	}
	if autoScanDebounceSeconds, ok := cus.ExtractIntField(payload, "auto_scan_debounce_seconds"); ok {
		config.AppConfig.AutoScanDebounceSeconds = autoScanDebounceSeconds
	}
	if operationTimeoutMinutes, ok := cus.ExtractIntField(payload, "operation_timeout_minutes"); ok {
		config.AppConfig.OperationTimeoutMinutes = operationTimeoutMinutes
	}
	if apiRate, ok := cus.ExtractIntField(payload, "api_rate_limit_per_minute"); ok {
		config.AppConfig.APIRateLimitPerMinute = apiRate
	}
	if authRate, ok := cus.ExtractIntField(payload, "auth_rate_limit_per_minute"); ok {
		config.AppConfig.AuthRateLimitPerMinute = authRate
	}
	if jsonBodyLimit, ok := cus.ExtractIntField(payload, "json_body_limit_mb"); ok {
		config.AppConfig.JSONBodyLimitMB = jsonBodyLimit
	}
	if uploadBodyLimit, ok := cus.ExtractIntField(payload, "upload_body_limit_mb"); ok {
		config.AppConfig.UploadBodyLimitMB = uploadBodyLimit
	}
	if enableDiskQuota, ok := cus.ExtractBoolField(payload, "enable_disk_quota"); ok {
		config.AppConfig.EnableDiskQuota = enableDiskQuota
	}
	if diskQuotaPercent, ok := cus.ExtractIntField(payload, "disk_quota_percent"); ok {
		config.AppConfig.DiskQuotaPercent = diskQuotaPercent
	}
	if enableUserQuotas, ok := cus.ExtractBoolField(payload, "enable_user_quotas"); ok {
		config.AppConfig.EnableUserQuotas = enableUserQuotas
	}
	if defaultUserQuotaGB, ok := cus.ExtractIntField(payload, "default_user_quota_gb"); ok {
		config.AppConfig.DefaultUserQuotaGB = defaultUserQuotaGB
	}

	if excludePatterns, ok := payload["exclude_patterns"].([]any); ok {
		patterns := make([]string, len(excludePatterns))
		for i, p := range excludePatterns {
			if s, ok := p.(string); ok {
				patterns[i] = s
			}
		}
		config.AppConfig.ExcludePatterns = patterns
	}

	if openaiKey, ok := cus.ExtractStringField(payload, "openai_api_key"); ok {
		config.AppConfig.OpenAIAPIKey = openaiKey
	}
	if enableAI, ok := cus.ExtractBoolField(payload, "enable_ai_parsing"); ok {
		config.AppConfig.EnableAIParsing = enableAI
	}
	if logLevel, ok := cus.ExtractStringField(payload, "log_level"); ok {
		config.AppConfig.LogLevel = logLevel
	}

	if supportedExtensions, ok := payload["supported_extensions"].([]any); ok {
		extensions := make([]string, len(supportedExtensions))
		for i, e := range supportedExtensions {
			if s, ok := e.(string); ok {
				extensions[i] = s
			}
		}
		config.AppConfig.SupportedExtensions = extensions
	}

	return nil
}

// MaskSecrets removes sensitive fields from config for response
func (cus *ConfigUpdateService) MaskSecrets(cfg config.Config) config.Config {
	masked := cfg
	if masked.OpenAIAPIKey != "" {
		masked.OpenAIAPIKey = database.MaskSecret(masked.OpenAIAPIKey)
	}
	return masked
}

// UpdateConfig applies updates, persists config, and returns the response payload.
func (cus *ConfigUpdateService) UpdateConfig(payload map[string]any) (int, map[string]any) {
	if cus.db == nil {
		return http.StatusInternalServerError, map[string]any{"error": "database not initialized"}
	}

	updated := []string{}

	if val, ok := payload["root_dir"].(string); ok {
		trimmed := strings.TrimSpace(val)
		config.AppConfig.RootDir = trimmed
		updated = append(updated, "root_dir")
		if trimmed == "" {
			config.AppConfig.SetupComplete = false
			updated = append(updated, "setup_complete")
		} else {
			config.AppConfig.SetupComplete = true
			updated = append(updated, "setup_complete")
		}
	}

	if val, ok := payload["database_path"].(string); ok {
		config.AppConfig.DatabasePath = val
		updated = append(updated, "database_path")
	}

	if val, ok := payload["playlist_dir"].(string); ok {
		config.AppConfig.PlaylistDir = val
		updated = append(updated, "playlist_dir")
	}

	if val, ok := payload["setup_complete"].(bool); ok {
		config.AppConfig.SetupComplete = val
		updated = append(updated, "setup_complete")
	}

	if val, ok := payload["organization_strategy"].(string); ok {
		config.AppConfig.OrganizationStrategy = val
		updated = append(updated, "organization_strategy")
	}
	if val, ok := payload["scan_on_startup"].(bool); ok {
		config.AppConfig.ScanOnStartup = val
		updated = append(updated, "scan_on_startup")
	}
	if val, ok := payload["auto_organize"].(bool); ok {
		config.AppConfig.AutoOrganize = val
		updated = append(updated, "auto_organize")
	}
	if val, ok := payload["folder_naming_pattern"].(string); ok {
		config.AppConfig.FolderNamingPattern = val
		updated = append(updated, "folder_naming_pattern")
	}
	if val, ok := payload["file_naming_pattern"].(string); ok {
		config.AppConfig.FileNamingPattern = val
		updated = append(updated, "file_naming_pattern")
	}
	if val, ok := payload["create_backups"].(bool); ok {
		config.AppConfig.CreateBackups = val
		updated = append(updated, "create_backups")
	}
	if val, ok := payload["supported_extensions"].([]any); ok {
		extensions := make([]string, 0, len(val))
		for _, item := range val {
			if ext, ok := item.(string); ok {
				extensions = append(extensions, ext)
			}
		}
		config.AppConfig.SupportedExtensions = extensions
		updated = append(updated, "supported_extensions")
	}
	if val, ok := payload["exclude_patterns"].([]any); ok {
		patterns := make([]string, 0, len(val))
		for _, item := range val {
			if pattern, ok := item.(string); ok {
				patterns = append(patterns, pattern)
			}
		}
		config.AppConfig.ExcludePatterns = patterns
		updated = append(updated, "exclude_patterns")
	}

	if _, ok := payload["database_type"]; ok {
		return http.StatusBadRequest, map[string]any{"error": "database_type cannot be changed at runtime"}
	}

	if _, ok := payload["enable_sqlite"]; ok {
		return http.StatusBadRequest, map[string]any{"error": "enable_sqlite cannot be changed at runtime"}
	}

	if val, ok := payload["openai_api_key"].(string); ok {
		log.Printf("[DEBUG] updateConfig: Updating OpenAI API key (length: %d, last 4: ***%s)", len(val), func() string {
			if len(val) > 4 {
				return val[len(val)-4:]
			}
			return val
		}())
		config.AppConfig.OpenAIAPIKey = val
		updated = append(updated, "openai_api_key")
	} else {
		log.Printf("[DEBUG] updateConfig: No openai_api_key in updates (present: %v, type: %T)", ok, payload["openai_api_key"])
	}
	if val, ok := payload["enable_ai_parsing"].(bool); ok {
		log.Printf("[DEBUG] updateConfig: Updating enable_ai_parsing to: %v", val)
		config.AppConfig.EnableAIParsing = val
		updated = append(updated, "enable_ai_parsing")
	}

	if val, ok := payload["concurrent_scans"].(float64); ok {
		config.AppConfig.ConcurrentScans = int(val)
		updated = append(updated, "concurrent_scans")
	} else if val, ok := payload["concurrent_scans"].(int); ok {
		config.AppConfig.ConcurrentScans = val
		updated = append(updated, "concurrent_scans")
	}
	if val, ok := payload["language"].(string); ok {
		config.AppConfig.Language = val
		updated = append(updated, "language")
	}
	if val, ok := payload["log_level"].(string); ok {
		config.AppConfig.LogLevel = val
		updated = append(updated, "log_level")
	}

	if err := config.SaveConfigToDatabase(cus.db); err != nil {
		log.Printf("ERROR: Failed to persist config to database: %v", err)
		return http.StatusInternalServerError, map[string]any{
			"error":   "failed to save configuration",
			"details": err.Error(),
		}
	}

	log.Printf("Configuration saved successfully. Updated fields: %v", updated)

	maskedConfig := cus.MaskSecrets(config.AppConfig)
	return http.StatusOK, map[string]any{
		"message": "configuration updated and saved to database",
		"updated": updated,
		"config":  maskedConfig,
	}
}
