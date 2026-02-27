// file: internal/server/config_update_service.go
// version: 2.2.0
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
	switch v := val.(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	}
	return 0, false
}

// extractStringSlice extracts a []string from a payload field containing []any.
func extractStringSlice(payload map[string]any, key string) ([]string, bool) {
	raw, ok := payload[key].([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out, true
}

// MaskSecrets removes sensitive fields from config for response
func (cus *ConfigUpdateService) MaskSecrets(cfg config.Config) config.Config {
	masked := cfg
	if masked.OpenAIAPIKey != "" {
		masked.OpenAIAPIKey = database.MaskSecret(masked.OpenAIAPIKey)
	}
	if masked.BasicAuthPassword != "" {
		masked.BasicAuthPassword = database.MaskSecret(masked.BasicAuthPassword)
	}
	return masked
}

// UpdateConfig is the single unified method for applying config updates from any
// caller (wizard, settings page, etc.). It validates the payload, applies all
// fields to AppConfig, derives setup_complete from root_dir, persists to the
// database, and returns an HTTP-style status code plus response map.
func (cus *ConfigUpdateService) UpdateConfig(payload map[string]any) (int, map[string]any) {
	if cus.db == nil {
		return http.StatusInternalServerError, map[string]any{"error": "database not initialized"}
	}
	if payload == nil {
		return http.StatusBadRequest, map[string]any{"error": "configuration payload is required"}
	}

	// Reject immutable fields early
	if _, ok := payload["database_type"]; ok {
		return http.StatusBadRequest, map[string]any{"error": "database_type cannot be changed at runtime"}
	}
	if _, ok := payload["enable_sqlite"]; ok {
		return http.StatusBadRequest, map[string]any{"error": "enable_sqlite cannot be changed at runtime"}
	}

	updated := []string{}

	// --- String fields ---
	stringFields := map[string]*string{
		"database_path":         &config.AppConfig.DatabasePath,
		"playlist_dir":          &config.AppConfig.PlaylistDir,
		"organization_strategy": &config.AppConfig.OrganizationStrategy,
		"folder_naming_pattern": &config.AppConfig.FolderNamingPattern,
		"file_naming_pattern":   &config.AppConfig.FileNamingPattern,
		"language":              &config.AppConfig.Language,
		"log_level":             &config.AppConfig.LogLevel,
		"openlibrary_dump_dir":  &config.AppConfig.OpenLibraryDumpDir,
		"hardcover_api_token":   &config.AppConfig.HardcoverAPIToken,
		"google_books_api_key":  &config.AppConfig.GoogleBooksAPIKey,
		"memory_limit_type":     &config.AppConfig.MemoryLimitType,
		"basic_auth_username":   &config.AppConfig.BasicAuthUsername,
		"basic_auth_password":   &config.AppConfig.BasicAuthPassword,
		"auto_update_channel":   &config.AppConfig.AutoUpdateChannel,
	}
	for key, ptr := range stringFields {
		if val, ok := cus.ExtractStringField(payload, key); ok {
			*ptr = val
			updated = append(updated, key)
		}
	}

	// root_dir has special handling: derives setup_complete
	if val, ok := cus.ExtractStringField(payload, "root_dir"); ok {
		trimmed := strings.TrimSpace(val)
		config.AppConfig.RootDir = trimmed
		updated = append(updated, "root_dir")
		config.AppConfig.SetupComplete = trimmed != ""
		updated = append(updated, "setup_complete")
	}

	// openai_api_key: secret field with debug logging
	if val, ok := cus.ExtractStringField(payload, "openai_api_key"); ok {
		log.Printf("[DEBUG] UpdateConfig: Updating OpenAI API key (length: %d, last 4: ***%s)", len(val), func() string {
			if len(val) > 4 {
				return val[len(val)-4:]
			}
			return val
		}())
		config.AppConfig.OpenAIAPIKey = val
		updated = append(updated, "openai_api_key")
	}

	// --- Bool fields ---
	boolFields := map[string]*bool{
		"setup_complete":           &config.AppConfig.SetupComplete,
		"scan_on_startup":         &config.AppConfig.ScanOnStartup,
		"auto_organize":           &config.AppConfig.AutoOrganize,
		"auto_scan_enabled":       &config.AppConfig.AutoScanEnabled,
		"create_backups":          &config.AppConfig.CreateBackups,
		"enable_disk_quota":       &config.AppConfig.EnableDiskQuota,
		"enable_user_quotas":      &config.AppConfig.EnableUserQuotas,
		"enable_ai_parsing":       &config.AppConfig.EnableAIParsing,
		"enable_auth":             &config.AppConfig.EnableAuth,
		"basic_auth_enabled":      &config.AppConfig.BasicAuthEnabled,
		"auto_fetch_metadata":      &config.AppConfig.AutoFetchMetadata,
		"write_back_metadata":      &config.AppConfig.WriteBackMetadata,
		"openlibrary_dump_enabled": &config.AppConfig.OpenLibraryDumpEnabled,
		"auto_update_enabled":      &config.AppConfig.AutoUpdateEnabled,
	}
	for key, ptr := range boolFields {
		if val, ok := cus.ExtractBoolField(payload, key); ok {
			*ptr = val
			updated = append(updated, key)
		}
	}

	// --- Int fields ---
	intFields := map[string]*int{
		"concurrent_scans":           &config.AppConfig.ConcurrentScans,
		"auto_scan_debounce_seconds": &config.AppConfig.AutoScanDebounceSeconds,
		"operation_timeout_minutes":  &config.AppConfig.OperationTimeoutMinutes,
		"api_rate_limit_per_minute":  &config.AppConfig.APIRateLimitPerMinute,
		"auth_rate_limit_per_minute": &config.AppConfig.AuthRateLimitPerMinute,
		"json_body_limit_mb":         &config.AppConfig.JSONBodyLimitMB,
		"upload_body_limit_mb":       &config.AppConfig.UploadBodyLimitMB,
		"disk_quota_percent":         &config.AppConfig.DiskQuotaPercent,
		"default_user_quota_gb":      &config.AppConfig.DefaultUserQuotaGB,
		"cache_size":                 &config.AppConfig.CacheSize,
		"memory_limit_percent":       &config.AppConfig.MemoryLimitPercent,
		"memory_limit_mb":            &config.AppConfig.MemoryLimitMB,
		"auto_update_check_minutes":  &config.AppConfig.AutoUpdateCheckMinutes,
		"auto_update_window_start":   &config.AppConfig.AutoUpdateWindowStart,
		"auto_update_window_end":     &config.AppConfig.AutoUpdateWindowEnd,
	}
	for key, ptr := range intFields {
		if val, ok := cus.ExtractIntField(payload, key); ok {
			*ptr = val
			updated = append(updated, key)
		}
	}

	// --- Slice fields ---
	if exts, ok := extractStringSlice(payload, "supported_extensions"); ok {
		config.AppConfig.SupportedExtensions = exts
		updated = append(updated, "supported_extensions")
	}
	if patterns, ok := extractStringSlice(payload, "exclude_patterns"); ok {
		config.AppConfig.ExcludePatterns = patterns
		updated = append(updated, "exclude_patterns")
	}

	// Persist to database
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

// ApplyUpdates applies config updates and persists them. This is a convenience
// wrapper around UpdateConfig for callers that prefer an error return.
// Deprecated: prefer UpdateConfig directly.
func (cus *ConfigUpdateService) ApplyUpdates(payload map[string]any) error {
	status, resp := cus.UpdateConfig(payload)
	if status >= 400 {
		if errMsg, ok := resp["error"].(string); ok {
			return fmt.Errorf("%s", errMsg)
		}
		return fmt.Errorf("config update failed with status %d", status)
	}
	return nil
}
