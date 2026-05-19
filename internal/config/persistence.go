// file: internal/config/persistence.go
// version: 1.18.1
// guid: 9c8d7e6f-5a4b-3c2d-1e0f-9a8b7c6d5e4f

package config

import (
	"log/slog"
	"encoding/json"
	"fmt"
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
		slog.Warn("Failed to parse config file %s: %v", path, err)
		return nil
	}

	applied := 0

	// Only fill in values that are currently empty/default.
	// This is the fallback path when DB decryption fails for secrets.
	stringFallbacks := map[string]*string{
		"openai_api_key":       &AppConfig.OpenAIAPIKey,
		"google_books_api_key": &AppConfig.GoogleBooksAPIKey,
		"hardcover_api_token":  &AppConfig.HardcoverAPIToken,
		"root_dir":             &AppConfig.RootDir,
		"language":             &AppConfig.Language,
	}
	for key, ptr := range stringFallbacks {
		if *ptr == "" {
			if val, ok := fileConfig[key].(string); ok && val != "" {
				*ptr = val
				applied++
				slog.Info("Loaded %s from config file", key)
			}
		}
	}

	if !AppConfig.EnableAIParsing {
		if val, ok := fileConfig["enable_ai_parsing"].(bool); ok && val {
			AppConfig.EnableAIParsing = true
			applied++
			slog.Info("Loaded enable_ai_parsing from config file")
		}
	}

	if applied > 0 {
		slog.Info("Applied %d settings from config file %s", applied, path)
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

	if err := os.MkdirAll(filepath.Dir(path), 0o775); err != nil {
		return fmt.Errorf("failed to create config dir: %w", err)
	}

	// Write with restrictive permissions since it may contain secrets
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	slog.Info("Configuration saved to file: %s", path)
	return nil
}

// LoadConfigFromDatabase loads settings from database and applies them to AppConfig.
//
// Load order (blob-first):
//  1. If "config_blob" exists: unmarshal the full non-secret Config JSON directly onto
//     AppConfig — every field is restored automatically, no registration needed.
//  2. Always load individual secret rows (they are NOT included in the blob).
//  3. If no blob is found (existing install): fall back to individual applySetting keys
//     so existing data is preserved.
func LoadConfigFromDatabase(store database.SettingsStore) error {
	if store == nil {
		return fmt.Errorf("store is nil")
	}

	slog.Info("Loading configuration from database...")

	settings, err := store.GetAllSettings()
	if err != nil {
		slog.Info("Note: Could not load settings from database: %v", err)
		return nil
	}

	// Index settings for O(1) lookup
	settingsMap := make(map[string]*database.Setting, len(settings))
	for i := range settings {
		settingsMap[settings[i].Key] = &settings[i]
	}

	var corruptSecrets []string

	// --- Blob path (new installs and post-first-save upgrades) ---
	blobFound := false
	if blob, ok := settingsMap["config_blob"]; ok && blob.Value != "" {
		// Preserve immutable fields — they must come from the runtime environment,
		// not from a stored blob that could have been created under different flags.
		savedDBType := AppConfig.DatabaseType

		var loaded Config
		if err := json.Unmarshal([]byte(blob.Value), &loaded); err == nil {
			AppConfig = loaded
			AppConfig.DatabaseType = savedDBType
			blobFound = true
			slog.Info("Loaded config from blob (%d bytes)", len(blob.Value))
		} else {
			slog.Warn("Failed to parse config_blob: %v — falling back to individual keys", err)
		}
	}

	// --- Secret loading (always, blob or legacy) ---
	// Secrets are never stored in the blob; they live as individually encrypted rows.
	for _, setting := range settings {
		if !setting.IsSecret {
			continue
		}
		decrypted, err := database.DecryptValue(setting.Value)
		if err != nil {
			slog.Info("WARNING: Failed to decrypt setting %q — will try config file fallback. (error: %v)",
				setting.Key, err)
			corruptSecrets = append(corruptSecrets, setting.Key)
			continue
		}
		if err := applySetting(setting.Key, decrypted, setting.Type); err != nil {
			slog.Warn("Failed to apply secret setting %s: %v", setting.Key, err)
		}
		slog.Debug("LoadConfigFromDatabase: found setting %s (isSecret=true, valueLen=%d)",
			setting.Key, len(decrypted))
	}

	// --- Legacy path (existing installs without a blob) ---
	if !blobFound {
		applied := 0
		for _, setting := range settings {
			if setting.Key == "config_blob" || setting.IsSecret {
				continue // blob already handled; secrets handled above
			}
			if err := applySetting(setting.Key, setting.Value, setting.Type); err != nil {
				slog.Warn("Failed to apply setting %s: %v", setting.Key, err)
				continue
			}
			applied++
		}
		slog.Info("Applied %d settings from database (legacy individual keys)", applied)
	}

	// Fall back to config file for anything not yet loaded (e.g. corrupted secrets)
	if err := LoadConfigFromFile(); err != nil {
		slog.Warn("Config file fallback failed: %v", err)
	}

	// Re-encrypt secrets that failed to decrypt but were recovered from the config file
	if len(corruptSecrets) > 0 {
		slog.Info("Re-encrypting %d corrupt secret(s) recovered from config file...", len(corruptSecrets))
		for _, key := range corruptSecrets {
			var plaintext string
			switch key {
			case "openai_api_key":
				plaintext = AppConfig.OpenAIAPIKey
			case "google_books_api_key":
				plaintext = AppConfig.GoogleBooksAPIKey
			case "hardcover_api_token":
				plaintext = AppConfig.HardcoverAPIToken
			case "basic_auth_password":
				plaintext = AppConfig.BasicAuthPassword
			}
			if plaintext != "" {
				if err := store.SetSetting(key, plaintext, "string", true); err != nil {
					slog.Warn("Failed to re-encrypt setting %q: %v", key, err)
				} else {
					slog.Info("Re-encrypted setting %q successfully", key)
				}
			} else {
				if err := store.DeleteSetting(key); err != nil {
					slog.Warn("Could not clear corrupt secret %q from DB: %v", key, err)
				} else {
					slog.Info("Cleared corrupt secret %q — re-enter via Settings", key)
				}
			}
		}
	}

	slog.Debug("After config load: EnableAIParsing=%v, OpenAIAPIKey length=%d",
		AppConfig.EnableAIParsing, len(AppConfig.OpenAIAPIKey))

	// Migrate auto-update window → maintenance window (idempotent)
	MigrateMaintenanceWindow(store)

	// Re-derive defaults that depend on RootDir
	if AppConfig.OpenLibraryDumpDir == "" && AppConfig.RootDir != "" {
		AppConfig.OpenLibraryDumpDir = filepath.Join(AppConfig.RootDir, "openlibrary-dumps")
	}

	return nil
}

// MigrateMaintenanceWindow migrates auto-update window fields to maintenance window.
// Idempotent — safe to call multiple times.
func MigrateMaintenanceWindow(store database.SettingsStore) {
	migrated, _ := store.GetSetting("maintenance_window_migrated")
	if migrated != nil && migrated.Value == "true" {
		return
	}

	// Migrate auto-update window start/end if maintenance window not yet configured
	if AppConfig.MaintenanceWindowStart == 0 && AppConfig.AutoUpdateWindowStart > 0 {
		AppConfig.MaintenanceWindowStart = AppConfig.AutoUpdateWindowStart
	}
	if AppConfig.MaintenanceWindowEnd == 0 && AppConfig.AutoUpdateWindowEnd > 0 {
		AppConfig.MaintenanceWindowEnd = AppConfig.AutoUpdateWindowEnd
	}
	// Ensure sensible defaults
	if AppConfig.MaintenanceWindowStart == 0 && AppConfig.MaintenanceWindowEnd == 0 {
		AppConfig.MaintenanceWindowStart = 1
		AppConfig.MaintenanceWindowEnd = 4
	}

	_ = store.SetSetting("maintenance_window_migrated", "true", "bool", false)
	slog.Info("Maintenance window migration complete (start=%d, end=%d)",
		AppConfig.MaintenanceWindowStart, AppConfig.MaintenanceWindowEnd)
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
	case "metadata_review_default_view":
		AppConfig.MetadataReviewDefaultView = value
	case "metadata_sources":
		var sources []MetadataSource
		if err := json.Unmarshal([]byte(value), &sources); err == nil && len(sources) > 0 {
			AppConfig.MetadataSources = sources
		}

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
	case "cache_invalidate_on_book_update":
		AppConfig.CacheInvalidateOnBookUpdate = value == "true"
	case "metadata_fetch_cache_ttl_days":
		if i, err := strconv.Atoi(value); err == nil {
			AppConfig.MetadataFetchCacheTTLDays = i
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
	case "itunes_library_write_path", "itunes_library_itl_path":
		AppConfig.ITunesLibraryWritePath = value
	case "itunes_library_read_path", "itunes_library_xml_path":
		AppConfig.ITunesLibraryReadPath = value
	case "itunes_auto_write_back":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.ITunesAutoWriteBack = b
		}
	case "itunes_path_trim_enabled":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.ITunesPathTrimEnabled = b
		}
	case "itunes_windows_root_path":
		AppConfig.ITunesWindowsRootPath = value
	case "itunes_media_root":
		AppConfig.ITunesMediaRoot = value
	case "itunes_path_mappings":
		var mappings []ITunesPathMap
		if err := json.Unmarshal([]byte(value), &mappings); err == nil {
			AppConfig.ITunesPathMappings = mappings
		}

	// Maintenance window
	case "maintenance_window_enabled":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.MaintenanceWindowEnabled = b
		}
	case "maintenance_window_start":
		if i, err := strconv.Atoi(value); err == nil {
			AppConfig.MaintenanceWindowStart = i
		}
	case "maintenance_window_end":
		if i, err := strconv.Atoi(value); err == nil {
			AppConfig.MaintenanceWindowEnd = i
		}
	case "maintenance_window_dedup_refresh":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.MaintenanceWindowDedupRefresh = b
		}
	case "maintenance_window_series_prune":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.MaintenanceWindowSeriesPrune = b
		}
	case "maintenance_window_author_split":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.MaintenanceWindowAuthorSplit = b
		}
	case "maintenance_window_tombstone_cleanup":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.MaintenanceWindowTombstoneCleanup = b
		}
	case "maintenance_window_reconcile":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.MaintenanceWindowReconcile = b
		}
	case "maintenance_window_purge_deleted":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.MaintenanceWindowPurgeDeleted = b
		}
	case "maintenance_window_purge_old_logs":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.MaintenanceWindowPurgeOldLogs = b
		}
	case "maintenance_window_db_optimize":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.MaintenanceWindowDbOptimize = b
		}
	case "maintenance_window_library_scan":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.MaintenanceWindowLibraryScan = b
		}
	case "maintenance_window_library_organize":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.MaintenanceWindowLibraryOrganize = b
		}
	case "maintenance_window_metadata_refresh":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.MaintenanceWindowMetadataRefresh = b
		}

	// Scheduled maintenance tasks
	case "scheduled_dedup_refresh_enabled":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.ScheduledDedupRefreshEnabled = b
		}
	case "scheduled_dedup_refresh_interval":
		if i, err := strconv.Atoi(value); err == nil {
			AppConfig.ScheduledDedupRefreshInterval = i
		}
	case "scheduled_dedup_refresh_on_startup":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.ScheduledDedupRefreshOnStartup = b
		}
	case "scheduled_author_split_enabled":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.ScheduledAuthorSplitEnabled = b
		}
	case "scheduled_author_split_interval":
		if i, err := strconv.Atoi(value); err == nil {
			AppConfig.ScheduledAuthorSplitInterval = i
		}
	case "scheduled_author_split_on_startup":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.ScheduledAuthorSplitOnStartup = b
		}
	case "scheduled_db_optimize_enabled":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.ScheduledDbOptimizeEnabled = b
		}
	case "scheduled_db_optimize_interval":
		if i, err := strconv.Atoi(value); err == nil {
			AppConfig.ScheduledDbOptimizeInterval = i
		}
	case "scheduled_db_optimize_on_startup":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.ScheduledDbOptimizeOnStartup = b
		}
	case "scheduled_metadata_refresh_enabled":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.ScheduledMetadataRefreshEnabled = b
		}
	case "scheduled_metadata_refresh_interval":
		if i, err := strconv.Atoi(value); err == nil {
			AppConfig.ScheduledMetadataRefreshInterval = i
		}
	case "scheduled_metadata_refresh_on_startup":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.ScheduledMetadataRefreshOnStartup = b
		}

	case "scheduled_resolve_production_authors_enabled":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.ScheduledResolveProductionAuthorsEnabled = b
		}
	case "scheduled_resolve_production_authors_interval":
		if i, err := strconv.Atoi(value); err == nil {
			AppConfig.ScheduledResolveProductionAuthorsInterval = i
		}

	case "scheduled_series_prune_enabled":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.ScheduledSeriesPruneEnabled = b
		}
	case "scheduled_series_prune_interval":
		if i, err := strconv.Atoi(value); err == nil {
			AppConfig.ScheduledSeriesPruneInterval = i
		}
	case "scheduled_series_prune_on_startup":
		if b, err := strconv.ParseBool(value); err == nil {
			AppConfig.ScheduledSeriesPruneOnStartup = b
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

	case "maintenance_window_migrated", "maintenance_window_last_run", "maintenance_window_update_completed":
		// Internal state keys — not mapped to AppConfig fields
		return nil

	default:
		return fmt.Errorf("unknown setting key: %s", key)
	}

	return nil
}

// SaveConfigToDatabase persists current AppConfig to database AND config file.
//
// Storage format (v2, blob-based):
//   - "config_blob": full Config JSON with secrets zeroed — automatically includes
//     every field in config.Config with no manual registration.
//   - Individual encrypted rows for each secret (openai_api_key, etc.).
//
// Existing installs that have never saved under v2 still load correctly via the
// legacy applySetting fallback in LoadConfigFromDatabase.
func SaveConfigToDatabase(store database.SettingsStore) error {
	if store == nil {
		return fmt.Errorf("store is nil")
	}

	slog.Info("Saving configuration to database...")

	// Build a safe copy with secrets zeroed — they are saved separately (encrypted).
	safeConfig := AppConfig
	safeConfig.OpenAIAPIKey = ""
	safeConfig.GoogleBooksAPIKey = ""
	safeConfig.HardcoverAPIToken = ""
	safeConfig.BasicAuthPassword = ""

	blobJSON, err := json.Marshal(safeConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal config blob: %w", err)
	}
	if err := store.SetSetting("config_blob", string(blobJSON), "json", false); err != nil {
		return fmt.Errorf("failed to save config blob: %w", err)
	}

	// Persist secrets individually (encrypted).
	// If the current AppConfig value is empty, preserve the existing DB entry
	// so a page-load that clears the field doesn't wipe a previously saved key.
	type secretEntry struct {
		key   string
		value string
	}
	secrets := []secretEntry{
		{"openai_api_key", AppConfig.OpenAIAPIKey},
		{"google_books_api_key", AppConfig.GoogleBooksAPIKey},
		{"hardcover_api_token", AppConfig.HardcoverAPIToken},
		{"basic_auth_password", AppConfig.BasicAuthPassword},
	}
	for _, s := range secrets {
		if s.value == "" {
			existing, err := store.GetSetting(s.key)
			if err == nil && existing != nil && existing.Value != "" {
				slog.Debug("Preserving existing secret %s (current value empty)", s.key)
				continue
			}
		}
		if err := store.SetSetting(s.key, s.value, "string", true); err != nil {
			slog.Warn("Failed to save secret %s: %v", s.key, err)
		}
	}

	slog.Info("Configuration saved to database (blob + %d secrets)", len(secrets))

	// Also save to config file as a reliable fallback
	if err := SaveConfigToFile(); err != nil {
		slog.Warn("Failed to save config file: %v", err)
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
			slog.Debug("SyncConfigFromEnv: overriding OpenAI API key from env/config (length: %d)", len(val))
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
