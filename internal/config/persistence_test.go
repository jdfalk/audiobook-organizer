// file: internal/config/persistence_test.go
// version: 1.3.1
// guid: 5e6f7a8b-9c0d-1e2f-3a4b-5c6d7e8f9a0b

package config

import (
	"fmt"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/database/mocks"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/mock"
)

func resetConfigTestState() {
	viper.Reset()
	AppConfig = Config{}
}

func TestLoadConfigFromDatabase(t *testing.T) {
	resetConfigTestState()
	t.Cleanup(resetConfigTestState)

	t.Run("returns error for nil store", func(t *testing.T) {
		err := LoadConfigFromDatabase(nil)
		if err == nil {
			t.Error("expected error for nil store")
		}
	})

	t.Run("returns nil when store GetAllSettings errors", func(t *testing.T) {
		store := mocks.NewMockStore(t)
		store.EXPECT().GetAllSettings().Return(nil, fmt.Errorf("boom")).Once()

		err := LoadConfigFromDatabase(store)
		if err != nil {
			t.Fatalf("expected nil error when GetAllSettings fails, got %v", err)
		}
	})

	t.Run("handles empty settings gracefully", func(t *testing.T) {
		store := mocks.NewMockStore(t)
		store.EXPECT().GetAllSettings().Return([]database.Setting{}, nil).Once()
		err := LoadConfigFromDatabase(store)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("loads string settings", func(t *testing.T) {
		store := mocks.NewMockStore(t)
		store.EXPECT().GetAllSettings().Return([]database.Setting{
			{
				Key:   "root_dir",
				Value: "/test/audiobooks",
				Type:  "string",
			},
			{
				Key:   "organization_strategy",
				Value: "copy",
				Type:  "string",
			},
		}, nil).Once()

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
		store := mocks.NewMockStore(t)
		store.EXPECT().GetAllSettings().Return([]database.Setting{
			{
				Key:   "scan_on_startup",
				Value: "true",
				Type:  "bool",
			},
			{
				Key:   "auto_organize",
				Value: "false",
				Type:  "bool",
			},
		}, nil).Once()

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
		store := mocks.NewMockStore(t)
		store.EXPECT().GetAllSettings().Return([]database.Setting{
			{
				Key:   "concurrent_scans",
				Value: "8",
				Type:  "int",
			},
			{
				Key:   "cache_size",
				Value: "2000",
				Type:  "int",
			},
			{
				Key:   "disk_quota_percent",
				Value: "90",
				Type:  "int",
			},
		}, nil).Once()

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

	t.Run("skips secret setting when decrypt fails", func(t *testing.T) {
		store := mocks.NewMockStore(t)
		store.EXPECT().GetAllSettings().Return([]database.Setting{
			{
				Key:      "openai_api_key",
				Value:    "not-base64",
				Type:     "string",
				IsSecret: true,
			},
		}, nil).Once()

		AppConfig = Config{OpenAIAPIKey: "keep-me"}

		err := LoadConfigFromDatabase(store)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if AppConfig.OpenAIAPIKey != "keep-me" {
			t.Fatalf("expected OpenAIAPIKey to remain unchanged, got %q", AppConfig.OpenAIAPIKey)
		}
	})

	t.Run("handles invalid boolean gracefully", func(t *testing.T) {
		store := mocks.NewMockStore(t)
		store.EXPECT().GetAllSettings().Return([]database.Setting{
			{
				Key:   "scan_on_startup",
				Value: "not-a-bool",
				Type:  "bool",
			},
		}, nil).Once()

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
		store := mocks.NewMockStore(t)
		store.EXPECT().GetAllSettings().Return([]database.Setting{
			{
				Key:   "concurrent_scans",
				Value: "not-an-int",
				Type:  "int",
			},
		}, nil).Once()

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
	resetConfigTestState()
	t.Cleanup(resetConfigTestState)

	tests := []struct {
		name    string
		key     string
		value   string
		typ     string
		check   func() bool
		setup   func()
		wantErr bool
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
	resetConfigTestState()
	t.Cleanup(resetConfigTestState)

	t.Run("returns error for nil store", func(t *testing.T) {
		err := SaveConfigToDatabase(nil)
		if err == nil {
			t.Error("expected error for nil store")
		}
	})

	t.Run("saves all config values", func(t *testing.T) {
		store := mocks.NewMockStore(t)
		seen := map[string]struct{}{}
		store.EXPECT().SetSetting(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Run(func(key string, value string, typ string, isSecret bool) {
				seen[key] = struct{}{}
			}).
			Return(nil)

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
		err := SaveConfigToDatabase(store)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for _, key := range []string{"root_dir", "database_path", "organization_strategy", "concurrent_scans"} {
			if _, ok := seen[key]; !ok {
				t.Fatalf("expected %q to be saved", key)
			}
		}
		for _, secretKey := range []string{"openai_api_key"} {
			if _, ok := seen[secretKey]; !ok {
				t.Fatalf("expected secret %q to be saved when non-empty", secretKey)
			}
		}
	})

	t.Run("skips empty secrets", func(t *testing.T) {
		store := mocks.NewMockStore(t)
		seen := map[string]struct{}{}
		store.EXPECT().SetSetting(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Run(func(key string, value string, typ string, isSecret bool) {
				seen[key] = struct{}{}
			}).
			Return(nil)

		AppConfig = Config{
			OpenAIAPIKey: "",
		}

		err := SaveConfigToDatabase(store)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, ok := seen["openai_api_key"]; ok {
			t.Fatalf("did not expect openai_api_key to be saved when empty")
		}
		if _, ok := seen["root_dir"]; !ok {
			t.Fatalf("expected root_dir to be saved")
		}
	})
}

func TestSyncConfigFromEnv(t *testing.T) {
	t.Run("overrides only set values", func(t *testing.T) {
		resetConfigTestState()
		t.Cleanup(resetConfigTestState)

		AppConfig = Config{
			RootDir:         "/existing/root",
			OpenAIAPIKey:    "existing-key",
			EnableAIParsing: false,
		}

		viper.Set("root_dir", "/env/root")
		viper.Set("openai_api_key", "env-key")
		viper.Set("enable_ai_parsing", true)

		SyncConfigFromEnv()

		if AppConfig.RootDir != "/env/root" {
			t.Errorf("expected RootDir to be overridden, got %q", AppConfig.RootDir)
		}
		if AppConfig.OpenAIAPIKey != "env-key" {
			t.Errorf("expected OpenAIAPIKey to be overridden, got %q", AppConfig.OpenAIAPIKey)
		}
		if !AppConfig.EnableAIParsing {
			t.Errorf("expected EnableAIParsing to be overridden to true")
		}
	})

	t.Run("does not change unset values", func(t *testing.T) {
		resetConfigTestState()
		t.Cleanup(resetConfigTestState)

		AppConfig = Config{RootDir: "/keep"}

		SyncConfigFromEnv()

		if AppConfig.RootDir != "/keep" {
			t.Errorf("expected RootDir to remain unchanged, got %q", AppConfig.RootDir)
		}
	})
}

func TestLifecycleRetentionSettings(t *testing.T) {
	t.Run("lifecycle settings not yet implemented in applySetting", func(t *testing.T) {
		resetConfigTestState()
		t.Cleanup(resetConfigTestState)

		InitConfig()

		// These settings are currently saved but not applied in applySetting.
		// Verify that loading them does not override defaults.
		store := mocks.NewMockStore(t)
		store.EXPECT().GetAllSettings().Return([]database.Setting{
			{
				Key:   "purge_soft_deleted_after_days",
				Value: "60",
				Type:  "int",
			},
			{
				Key:   "purge_soft_deleted_delete_files",
				Value: "true",
				Type:  "bool",
			},
		}, nil).Once()

		defaultDays := AppConfig.PurgeSoftDeletedAfterDays
		defaultDeleteFiles := AppConfig.PurgeSoftDeletedDeleteFiles

		err := LoadConfigFromDatabase(store)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if AppConfig.PurgeSoftDeletedAfterDays != defaultDays {
			t.Errorf("expected PurgeSoftDeletedAfterDays to remain %d, got %d", defaultDays, AppConfig.PurgeSoftDeletedAfterDays)
		}
		if AppConfig.PurgeSoftDeletedDeleteFiles != defaultDeleteFiles {
			t.Errorf("expected PurgeSoftDeletedDeleteFiles to remain %v, got %v", defaultDeleteFiles, AppConfig.PurgeSoftDeletedDeleteFiles)
		}
	})
}
