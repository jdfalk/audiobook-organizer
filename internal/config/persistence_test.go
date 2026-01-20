// file: internal/config/persistence_test.go
// version: 1.0.0
// guid: 5e6f7a8b-9c0d-1e2f-3a4b-5c6d7e8f9a0b

package config

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

func TestLoadConfigFromDatabase(t *testing.T) {
	t.Run("returns error for nil store", func(t *testing.T) {
		err := LoadConfigFromDatabase(nil)
		if err == nil {
			t.Error("expected error for nil store")
		}
	})

	t.Run("handles empty settings gracefully", func(t *testing.T) {
		store := database.NewMockStore()
		err := LoadConfigFromDatabase(store)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("loads string settings", func(t *testing.T) {
		store := database.NewMockStore()
		store.Settings["root_dir"] = &database.Setting{
			Key:   "root_dir",
			Value: "/test/audiobooks",
			Type:  "string",
		}
		store.Settings["organization_strategy"] = &database.Setting{
			Key:   "organization_strategy",
			Value: "copy",
			Type:  "string",
		}

		// Reset AppConfig
		AppConfig = Config{}

		err := LoadConfigFromDatabase(store)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if AppConfig.RootDir != "/test/audiobooks" {
			t.Errorf("expected RootDir='/test/audiobooks', got '%s'", AppConfig.RootDir)
		}
		if AppConfig.OrganizationStrategy != "copy" {
			t.Errorf("expected OrganizationStrategy='copy', got '%s'", AppConfig.OrganizationStrategy)
		}
	})

	t.Run("loads boolean settings", func(t *testing.T) {
		store := database.NewMockStore()
		store.Settings["scan_on_startup"] = &database.Setting{
			Key:   "scan_on_startup",
			Value: "true",
			Type:  "bool",
		}
		store.Settings["auto_organize"] = &database.Setting{
			Key:   "auto_organize",
			Value: "false",
			Type:  "bool",
		}

		AppConfig = Config{}

		err := LoadConfigFromDatabase(store)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !AppConfig.ScanOnStartup {
			t.Error("expected ScanOnStartup=true")
		}
		if AppConfig.AutoOrganize {
			t.Error("expected AutoOrganize=false")
		}
	})

	t.Run("loads integer settings", func(t *testing.T) {
		store := database.NewMockStore()
		store.Settings["concurrent_scans"] = &database.Setting{
			Key:   "concurrent_scans",
			Value: "8",
			Type:  "int",
		}
		store.Settings["cache_size"] = &database.Setting{
			Key:   "cache_size",
			Value: "2000",
			Type:  "int",
		}
		store.Settings["disk_quota_percent"] = &database.Setting{
			Key:   "disk_quota_percent",
			Value: "90",
			Type:  "int",
		}

		AppConfig = Config{}

		err := LoadConfigFromDatabase(store)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if AppConfig.ConcurrentScans != 8 {
			t.Errorf("expected ConcurrentScans=8, got %d", AppConfig.ConcurrentScans)
		}
		if AppConfig.CacheSize != 2000 {
			t.Errorf("expected CacheSize=2000, got %d", AppConfig.CacheSize)
		}
		if AppConfig.DiskQuotaPercent != 90 {
			t.Errorf("expected DiskQuotaPercent=90, got %d", AppConfig.DiskQuotaPercent)
		}
	})

	t.Run("handles invalid boolean gracefully", func(t *testing.T) {
		store := database.NewMockStore()
		store.Settings["scan_on_startup"] = &database.Setting{
			Key:   "scan_on_startup",
			Value: "not-a-bool",
			Type:  "bool",
		}

		AppConfig = Config{ScanOnStartup: true}

		err := LoadConfigFromDatabase(store)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should remain unchanged due to parse error
		if !AppConfig.ScanOnStartup {
			t.Error("ScanOnStartup should not have changed on parse error")
		}
	})

	t.Run("handles invalid integer gracefully", func(t *testing.T) {
		store := database.NewMockStore()
		store.Settings["concurrent_scans"] = &database.Setting{
			Key:   "concurrent_scans",
			Value: "not-an-int",
			Type:  "int",
		}

		AppConfig = Config{ConcurrentScans: 4}

		err := LoadConfigFromDatabase(store)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should remain unchanged due to parse error
		if AppConfig.ConcurrentScans != 4 {
			t.Errorf("ConcurrentScans should not have changed on parse error, got %d", AppConfig.ConcurrentScans)
		}
	})
}

func TestApplySetting(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    string
		typ      string
		check    func() bool
		setup    func()
		wantErr  bool
	}{
		{
			name:  "root_dir",
			key:   "root_dir",
			value: "/new/path",
			typ:   "string",
			setup: func() { AppConfig.RootDir = "" },
			check: func() bool { return AppConfig.RootDir == "/new/path" },
		},
		{
			name:  "database_path",
			key:   "database_path",
			value: "/data/db.pebble",
			typ:   "string",
			setup: func() { AppConfig.DatabasePath = "" },
			check: func() bool { return AppConfig.DatabasePath == "/data/db.pebble" },
		},
		{
			name:  "playlist_dir",
			key:   "playlist_dir",
			value: "/playlists",
			typ:   "string",
			setup: func() { AppConfig.PlaylistDir = "" },
			check: func() bool { return AppConfig.PlaylistDir == "/playlists" },
		},
		{
			name:  "organization_strategy",
			key:   "organization_strategy",
			value: "hardlink",
			typ:   "string",
			setup: func() { AppConfig.OrganizationStrategy = "" },
			check: func() bool { return AppConfig.OrganizationStrategy == "hardlink" },
		},
		{
			name:  "scan_on_startup",
			key:   "scan_on_startup",
			value: "true",
			typ:   "bool",
			setup: func() { AppConfig.ScanOnStartup = false },
			check: func() bool { return AppConfig.ScanOnStartup },
		},
		{
			name:  "auto_organize",
			key:   "auto_organize",
			value: "false",
			typ:   "bool",
			setup: func() { AppConfig.AutoOrganize = true },
			check: func() bool { return !AppConfig.AutoOrganize },
		},
		{
			name:  "folder_naming_pattern",
			key:   "folder_naming_pattern",
			value: "{author}/{title}",
			typ:   "string",
			setup: func() { AppConfig.FolderNamingPattern = "" },
			check: func() bool { return AppConfig.FolderNamingPattern == "{author}/{title}" },
		},
		{
			name:  "file_naming_pattern",
			key:   "file_naming_pattern",
			value: "{title}",
			typ:   "string",
			setup: func() { AppConfig.FileNamingPattern = "" },
			check: func() bool { return AppConfig.FileNamingPattern == "{title}" },
		},
		{
			name:  "create_backups",
			key:   "create_backups",
			value: "false",
			typ:   "bool",
			setup: func() { AppConfig.CreateBackups = true },
			check: func() bool { return !AppConfig.CreateBackups },
		},
		{
			name:  "enable_disk_quota",
			key:   "enable_disk_quota",
			value: "true",
			typ:   "bool",
			setup: func() { AppConfig.EnableDiskQuota = false },
			check: func() bool { return AppConfig.EnableDiskQuota },
		},
		{
			name:  "disk_quota_percent",
			key:   "disk_quota_percent",
			value: "95",
			typ:   "int",
			setup: func() { AppConfig.DiskQuotaPercent = 0 },
			check: func() bool { return AppConfig.DiskQuotaPercent == 95 },
		},
		{
			name:  "enable_user_quotas",
			key:   "enable_user_quotas",
			value: "true",
			typ:   "bool",
			setup: func() { AppConfig.EnableUserQuotas = false },
			check: func() bool { return AppConfig.EnableUserQuotas },
		},
		{
			name:  "default_user_quota_gb",
			key:   "default_user_quota_gb",
			value: "50",
			typ:   "int",
			setup: func() { AppConfig.DefaultUserQuotaGB = 0 },
			check: func() bool { return AppConfig.DefaultUserQuotaGB == 50 },
		},
		{
			name:  "auto_fetch_metadata",
			key:   "auto_fetch_metadata",
			value: "false",
			typ:   "bool",
			setup: func() { AppConfig.AutoFetchMetadata = true },
			check: func() bool { return !AppConfig.AutoFetchMetadata },
		},
		{
			name:  "language",
			key:   "language",
			value: "de",
			typ:   "string",
			setup: func() { AppConfig.Language = "" },
			check: func() bool { return AppConfig.Language == "de" },
		},
		{
			name:  "enable_ai_parsing",
			key:   "enable_ai_parsing",
			value: "true",
			typ:   "bool",
			setup: func() { AppConfig.EnableAIParsing = false },
			check: func() bool { return AppConfig.EnableAIParsing },
		},
		{
			name:  "openai_api_key",
			key:   "openai_api_key",
			value: "sk-test-key",
			typ:   "string",
			setup: func() { AppConfig.OpenAIAPIKey = "" },
			check: func() bool { return AppConfig.OpenAIAPIKey == "sk-test-key" },
		},
		{
			name:  "concurrent_scans",
			key:   "concurrent_scans",
			value: "16",
			typ:   "int",
			setup: func() { AppConfig.ConcurrentScans = 0 },
			check: func() bool { return AppConfig.ConcurrentScans == 16 },
		},
		{
			name:  "memory_limit_type",
			key:   "memory_limit_type",
			value: "percent",
			typ:   "string",
			setup: func() { AppConfig.MemoryLimitType = "" },
			check: func() bool { return AppConfig.MemoryLimitType == "percent" },
		},
		{
			name:  "cache_size",
			key:   "cache_size",
			value: "5000",
			typ:   "int",
			setup: func() { AppConfig.CacheSize = 0 },
			check: func() bool { return AppConfig.CacheSize == 5000 },
		},
		{
			name:  "memory_limit_percent",
			key:   "memory_limit_percent",
			value: "50",
			typ:   "int",
			setup: func() { AppConfig.MemoryLimitPercent = 0 },
			check: func() bool { return AppConfig.MemoryLimitPercent == 50 },
		},
		{
			name:  "memory_limit_mb",
			key:   "memory_limit_mb",
			value: "1024",
			typ:   "int",
			setup: func() { AppConfig.MemoryLimitMB = 0 },
			check: func() bool { return AppConfig.MemoryLimitMB == 1024 },
		},
		{
			name:  "log_level",
			key:   "log_level",
			value: "debug",
			typ:   "string",
			setup: func() { AppConfig.LogLevel = "" },
			check: func() bool { return AppConfig.LogLevel == "debug" },
		},
		{
			name:  "log_format",
			key:   "log_format",
			value: "json",
			typ:   "string",
			setup: func() { AppConfig.LogFormat = "" },
			check: func() bool { return AppConfig.LogFormat == "json" },
		},
		{
			name:  "enable_json_logging",
			key:   "enable_json_logging",
			value: "true",
			typ:   "bool",
			setup: func() { AppConfig.EnableJsonLogging = false },
			check: func() bool { return AppConfig.EnableJsonLogging },
		},
		{
			name:  "goodreads_api_key",
			key:   "goodreads_api_key",
			value: "goodreads-key",
			typ:   "string",
			setup: func() { AppConfig.APIKeys.Goodreads = "" },
			check: func() bool { return AppConfig.APIKeys.Goodreads == "goodreads-key" },
		},
		{
			name:    "unknown_key",
			key:     "unknown_key",
			value:   "value",
			typ:     "string",
			setup:   func() {},
			check:   func() bool { return true },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			err := applySetting(tt.key, tt.value, tt.typ)
			if (err != nil) != tt.wantErr {
				t.Errorf("applySetting() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !tt.check() {
				t.Errorf("applySetting() did not apply setting correctly")
			}
		})
	}
}

func TestSaveConfigToDatabase(t *testing.T) {
	t.Run("returns error for nil store", func(t *testing.T) {
		err := SaveConfigToDatabase(nil)
		if err == nil {
			t.Error("expected error for nil store")
		}
	})

	t.Run("saves all config values", func(t *testing.T) {
		store := database.NewMockStore()

		AppConfig = Config{
			RootDir:              "/test/audiobooks",
			DatabasePath:         "/data/db.pebble",
			PlaylistDir:          "/playlists",
			OrganizationStrategy: "copy",
			ScanOnStartup:        true,
			AutoOrganize:         false,
			FolderNamingPattern:  "{author}/{title}",
			FileNamingPattern:    "{title}",
			CreateBackups:        true,
			EnableDiskQuota:      true,
			DiskQuotaPercent:     90,
			EnableUserQuotas:     true,
			DefaultUserQuotaGB:   50,
			AutoFetchMetadata:    true,
			Language:             "de",
			EnableAIParsing:      true,
			OpenAIAPIKey:         "sk-test",
			ConcurrentScans:      8,
			MemoryLimitType:      "percent",
			CacheSize:            2000,
			MemoryLimitPercent:   50,
			MemoryLimitMB:        1024,
			LogLevel:             "debug",
			LogFormat:            "json",
			EnableJsonLogging:    true,
		}
		AppConfig.APIKeys.Goodreads = "gr-key"

		err := SaveConfigToDatabase(store)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify some settings were saved
		if s, ok := store.Settings["root_dir"]; !ok || s.Value != "/test/audiobooks" {
			t.Error("root_dir not saved correctly")
		}
		if s, ok := store.Settings["concurrent_scans"]; !ok || s.Value != "8" {
			t.Error("concurrent_scans not saved correctly")
		}
		if s, ok := store.Settings["scan_on_startup"]; !ok || s.Value != "true" {
			t.Error("scan_on_startup not saved correctly")
		}
		// Secret should be saved
		if s, ok := store.Settings["openai_api_key"]; !ok || s.Value != "sk-test" {
			t.Error("openai_api_key not saved correctly")
		}
	})

	t.Run("skips empty secrets", func(t *testing.T) {
		store := database.NewMockStore()

		AppConfig = Config{
			OpenAIAPIKey: "",
		}
		AppConfig.APIKeys.Goodreads = ""

		err := SaveConfigToDatabase(store)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, ok := store.Settings["openai_api_key"]; ok {
			t.Error("empty openai_api_key should not be saved")
		}
		if _, ok := store.Settings["goodreads_api_key"]; ok {
			t.Error("empty goodreads_api_key should not be saved")
		}
	})
}

func TestSyncConfigFromEnv(t *testing.T) {
	// This function uses viper, which is already tested in config_test.go
	// Here we just verify it doesn't panic
	t.Run("does not panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("SyncConfigFromEnv panicked: %v", r)
			}
		}()
		SyncConfigFromEnv()
	})
}

func TestLifecycleRetentionSettings(t *testing.T) {
	t.Run("lifecycle settings not yet implemented in applySetting", func(t *testing.T) {
		// These settings are not currently handled in applySetting
		// They are saved but cannot be loaded back
		store := database.NewMockStore()
		store.Settings["purge_soft_deleted_after_days"] = &database.Setting{
			Key:   "purge_soft_deleted_after_days",
			Value: "60",
			Type:  "int",
		}
		store.Settings["purge_soft_deleted_delete_files"] = &database.Setting{
			Key:   "purge_soft_deleted_delete_files",
			Value: "true",
			Type:  "bool",
		}

		AppConfig = Config{}
		// This will log warnings about unknown settings
		err := LoadConfigFromDatabase(store)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Values should remain at defaults since applySetting doesn't handle them
		if AppConfig.PurgeSoftDeletedAfterDays != 0 {
			t.Error("expected PurgeSoftDeletedAfterDays to remain at default")
		}
	})
}
