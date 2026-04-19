// file: internal/config/config_unit_test.go
// version: 1.0.0

package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHasBalancedBraces(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"empty string", "", true},
		{"no braces", "hello world", true},
		{"balanced single", "{author}", true},
		{"balanced multiple", "{author}/{title}", true},
		{"unbalanced open", "{author/{title}", false},
		{"unbalanced close", "author}/{title}", false},
		{"nested balanced", "{{nested}}", true},
		{"extra open", "{a}{b}{", false},
		{"extra close", "{a}}", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, hasBalancedBraces(tt.input))
		})
	}
}

func TestValidateNamingPattern(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		wantErr string
	}{
		{"valid simple", "{title}", ""},
		{"valid with separators", "{author}/{series}/{title}", ""},
		{"valid with literal text", "{title} ({print_year})", ""},
		{"empty pattern", "", "pattern cannot be empty"},
		{"whitespace only", "   ", "pattern cannot be empty"},
		{"unbalanced braces", "{author/{title}", "unbalanced braces"},
		{"invalid placeholder", "{author}/{bad placeholder}", "invalid placeholder format"},
		{"bare open brace", "{author}/{ }/{title}", "invalid placeholder format"},
		{"bare close brace", "{author}/}/{title}", "unbalanced braces"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateNamingPattern(tt.pattern)
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

func TestValidateParentDirExists(t *testing.T) {
	t.Run("empty path is ok", func(t *testing.T) {
		assert.NoError(t, validateParentDirExists("", "test_field"))
	})

	t.Run("whitespace path is ok", func(t *testing.T) {
		assert.NoError(t, validateParentDirExists("   ", "test_field"))
	})

	t.Run("existing parent dir", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "somefile.db")
		assert.NoError(t, validateParentDirExists(path, "database_path"))
	})

	t.Run("non-existent parent dir", func(t *testing.T) {
		path := filepath.Join("/nonexistent_dir_12345", "subdir", "file.db")
		err := validateParentDirExists(path, "database_path")
		assert.ErrorContains(t, err, "does not exist")
	})

	t.Run("parent is a file not directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		fakeParent := filepath.Join(tmpDir, "notadir")
		require.NoError(t, os.WriteFile(fakeParent, []byte("x"), 0644))
		path := filepath.Join(fakeParent, "file.db")
		err := validateParentDirExists(path, "database_path")
		assert.ErrorContains(t, err, "is not a directory")
	})
}

func TestValidateParentDirWritable(t *testing.T) {
	t.Run("empty path is ok", func(t *testing.T) {
		assert.NoError(t, validateParentDirWritable("", "test_field"))
	})

	t.Run("whitespace path is ok", func(t *testing.T) {
		assert.NoError(t, validateParentDirWritable("   ", "test_field"))
	})

	t.Run("writable parent dir", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "somefile.db")
		assert.NoError(t, validateParentDirWritable(path, "database_path"))
	})

	t.Run("non-existent parent dir", func(t *testing.T) {
		path := filepath.Join("/nonexistent_dir_12345", "file.db")
		err := validateParentDirWritable(path, "database_path")
		assert.ErrorContains(t, err, "is not writable")
	})
}

func TestConfigValidate(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		var c *Config
		err := c.Validate()
		assert.ErrorContains(t, err, "config is nil")
	})

	t.Run("valid defaults", func(t *testing.T) {
		c := &Config{
			DatabaseType:         "pebble",
			OrganizationStrategy: "auto",
			ConcurrentScans:      4,
			FolderNamingPattern:  "{author}/{title}",
			FileNamingPattern:    "{title}",
			SupportedExtensions:  []string{".m4b", ".mp3"},
		}
		assert.NoError(t, c.Validate())
	})

	t.Run("invalid database type", func(t *testing.T) {
		c := &Config{DatabaseType: "mysql"}
		err := c.Validate()
		assert.ErrorContains(t, err, "database_type must be 'pebble' or 'sqlite'")
	})

	t.Run("sqlite is valid", func(t *testing.T) {
		c := &Config{DatabaseType: "sqlite", OrganizationStrategy: "auto"}
		assert.NoError(t, c.Validate())
	})

	t.Run("negative concurrent scans", func(t *testing.T) {
		c := &Config{DatabaseType: "pebble", ConcurrentScans: -1}
		err := c.Validate()
		assert.ErrorContains(t, err, "concurrent_scans must be >= 0")
	})

	t.Run("negative debounce seconds", func(t *testing.T) {
		c := &Config{DatabaseType: "pebble", AutoScanDebounceSeconds: -5}
		err := c.Validate()
		assert.ErrorContains(t, err, "auto_scan_debounce_seconds must be >= 0")
	})

	t.Run("negative operation timeout", func(t *testing.T) {
		c := &Config{DatabaseType: "pebble", OperationTimeoutMinutes: -1}
		err := c.Validate()
		assert.ErrorContains(t, err, "operation_timeout_minutes must be >= 0")
	})

	t.Run("negative API rate limit", func(t *testing.T) {
		c := &Config{DatabaseType: "pebble", APIRateLimitPerMinute: -1}
		err := c.Validate()
		assert.ErrorContains(t, err, "api_rate_limit_per_minute must be >= 0")
	})

	t.Run("negative auth rate limit", func(t *testing.T) {
		c := &Config{DatabaseType: "pebble", AuthRateLimitPerMinute: -1}
		err := c.Validate()
		assert.ErrorContains(t, err, "auth_rate_limit_per_minute must be >= 0")
	})

	t.Run("negative JSON body limit", func(t *testing.T) {
		c := &Config{DatabaseType: "pebble", JSONBodyLimitMB: -1}
		err := c.Validate()
		assert.ErrorContains(t, err, "json_body_limit_mb must be >= 0")
	})

	t.Run("negative upload body limit", func(t *testing.T) {
		c := &Config{DatabaseType: "pebble", UploadBodyLimitMB: -1}
		err := c.Validate()
		assert.ErrorContains(t, err, "upload_body_limit_mb must be >= 0")
	})

	t.Run("disk quota out of range", func(t *testing.T) {
		c := &Config{
			DatabaseType:    "pebble",
			EnableDiskQuota: true,
			DiskQuotaPercent: 0,
		}
		err := c.Validate()
		assert.ErrorContains(t, err, "disk_quota_percent must be between 1 and 100")

		c.DiskQuotaPercent = 101
		err = c.Validate()
		assert.ErrorContains(t, err, "disk_quota_percent must be between 1 and 100")
	})

	t.Run("disk quota disabled ignores range", func(t *testing.T) {
		c := &Config{
			DatabaseType:     "pebble",
			EnableDiskQuota:  false,
			DiskQuotaPercent: 0,
		}
		assert.NoError(t, c.Validate())
	})

	t.Run("invalid organization strategy", func(t *testing.T) {
		c := &Config{DatabaseType: "pebble", OrganizationStrategy: "magic"}
		err := c.Validate()
		assert.ErrorContains(t, err, "organization_strategy must be one of")
	})

	t.Run("all valid strategies", func(t *testing.T) {
		for _, strategy := range []string{"auto", "copy", "hardlink", "reflink", "symlink"} {
			c := &Config{DatabaseType: "pebble", OrganizationStrategy: strategy}
			assert.NoError(t, c.Validate(), "strategy %q should be valid", strategy)
		}
	})

	t.Run("invalid folder naming pattern", func(t *testing.T) {
		c := &Config{
			DatabaseType:        "pebble",
			FolderNamingPattern: "{bad pattern",
		}
		err := c.Validate()
		assert.ErrorContains(t, err, "folder_naming_pattern")
	})

	t.Run("invalid file naming pattern", func(t *testing.T) {
		c := &Config{
			DatabaseType:      "pebble",
			FileNamingPattern: "{bad pattern",
		}
		err := c.Validate()
		assert.ErrorContains(t, err, "file_naming_pattern")
	})

	t.Run("extension without dot", func(t *testing.T) {
		c := &Config{
			DatabaseType:        "pebble",
			SupportedExtensions: []string{"m4b"},
		}
		err := c.Validate()
		assert.ErrorContains(t, err, "must start with '.'")
	})

	t.Run("empty extension is skipped", func(t *testing.T) {
		c := &Config{
			DatabaseType:        "pebble",
			SupportedExtensions: []string{"", ".mp3"},
		}
		assert.NoError(t, c.Validate())
	})

	t.Run("multiple errors collected", func(t *testing.T) {
		c := &Config{
			DatabaseType:    "invalid",
			ConcurrentScans: -1,
		}
		err := c.Validate()
		assert.ErrorContains(t, err, "database_type")
		assert.ErrorContains(t, err, "concurrent_scans")
	})

	t.Run("database path with existing parent", func(t *testing.T) {
		tmpDir := t.TempDir()
		c := &Config{
			DatabaseType: "pebble",
			DatabasePath: filepath.Join(tmpDir, "test.db"),
		}
		assert.NoError(t, c.Validate())
	})

	t.Run("database path with non-existent parent", func(t *testing.T) {
		c := &Config{
			DatabaseType: "pebble",
			DatabasePath: "/nonexistent_xyz_12345/sub/test.db",
		}
		err := c.Validate()
		assert.ErrorContains(t, err, "does not exist")
	})

	t.Run("playlist dir with non-existent parent", func(t *testing.T) {
		c := &Config{
			DatabaseType: "pebble",
			PlaylistDir:  "/nonexistent_xyz_12345/playlists",
		}
		err := c.Validate()
		assert.ErrorContains(t, err, "does not exist")
	})
}

func TestInitConfigDefaults(t *testing.T) {
	viper.Reset()
	InitConfig()

	t.Run("core defaults", func(t *testing.T) {
		assert.Equal(t, "pebble", AppConfig.DatabaseType)
		assert.False(t, AppConfig.EnableSQLite)
		assert.False(t, AppConfig.SetupComplete)
	})

	t.Run("library organization defaults", func(t *testing.T) {
		assert.Equal(t, "auto", AppConfig.OrganizationStrategy)
		assert.False(t, AppConfig.ScanOnStartup)
		assert.True(t, AppConfig.AutoOrganize)
		assert.False(t, AppConfig.AutoScanEnabled)
		assert.Equal(t, 30, AppConfig.AutoScanDebounceSeconds)
		assert.True(t, AppConfig.CreateBackups)
	})

	t.Run("metadata defaults", func(t *testing.T) {
		assert.True(t, AppConfig.AutoFetchMetadata)
		assert.False(t, AppConfig.WriteBackMetadata)
		assert.False(t, AppConfig.EmbedCoverArt)
		assert.Equal(t, "en", AppConfig.Language)
	})

	t.Run("performance defaults", func(t *testing.T) {
		expectedWorkers := runtime.NumCPU()
		if expectedWorkers < 4 {
			expectedWorkers = 4
		}
		assert.Equal(t, expectedWorkers, AppConfig.ConcurrentScans)
		assert.Equal(t, 30, AppConfig.OperationTimeoutMinutes)
	})

	t.Run("embedding dedup defaults", func(t *testing.T) {
		assert.True(t, AppConfig.EmbeddingEnabled)
		assert.Equal(t, "text-embedding-3-large", AppConfig.EmbeddingModel)
		assert.InDelta(t, 0.95, AppConfig.DedupBookHighThreshold, 0.001)
		assert.InDelta(t, 0.85, AppConfig.DedupBookLowThreshold, 0.001)
		assert.InDelta(t, 0.92, AppConfig.DedupAuthorHighThreshold, 0.001)
		assert.InDelta(t, 0.80, AppConfig.DedupAuthorLowThreshold, 0.001)
		assert.True(t, AppConfig.DedupAutoMergeEnabled)
		assert.False(t, AppConfig.DedupLLMAutoMergeHighConfidence)
	})

	t.Run("metadata scoring defaults", func(t *testing.T) {
		assert.True(t, AppConfig.MetadataEmbeddingScoringEnabled)
		assert.InDelta(t, 0.50, AppConfig.MetadataEmbeddingMinScore, 0.001)
		assert.InDelta(t, 0.70, AppConfig.MetadataEmbeddingBestMatchMin, 0.001)
		assert.False(t, AppConfig.MetadataLLMScoringEnabled)
		assert.InDelta(t, 0.01, AppConfig.MetadataLLMRerankEpsilon, 0.001)
		assert.Equal(t, 5, AppConfig.MetadataLLMRerankTopK)
	})

	t.Run("tag write backup default", func(t *testing.T) {
		assert.False(t, AppConfig.WriteBackupBeforeTagWrite)
	})

	t.Run("API and auth defaults", func(t *testing.T) {
		assert.Equal(t, 0, AppConfig.APIRateLimitPerMinute)
		assert.Equal(t, 10, AppConfig.AuthRateLimitPerMinute)
		assert.Equal(t, 1, AppConfig.JSONBodyLimitMB)
		assert.Equal(t, 10, AppConfig.UploadBodyLimitMB)
		assert.True(t, AppConfig.EnableAuth)
		assert.False(t, AppConfig.BasicAuthEnabled)
	})

	t.Run("memory management defaults", func(t *testing.T) {
		assert.Equal(t, "items", AppConfig.MemoryLimitType)
		assert.Equal(t, 1000, AppConfig.CacheSize)
		assert.Equal(t, 25, AppConfig.MemoryLimitPercent)
		assert.Equal(t, 512, AppConfig.MemoryLimitMB)
	})

	t.Run("logging defaults", func(t *testing.T) {
		assert.Equal(t, "info", AppConfig.LogLevel)
		assert.Equal(t, "text", AppConfig.LogFormat)
		assert.False(t, AppConfig.EnableJsonLogging)
	})

	t.Run("maintenance window defaults", func(t *testing.T) {
		assert.True(t, AppConfig.MaintenanceWindowEnabled)
		assert.Equal(t, 1, AppConfig.MaintenanceWindowStart)
		assert.Equal(t, 4, AppConfig.MaintenanceWindowEnd)
		assert.True(t, AppConfig.MaintenanceWindowDedupRefresh)
		assert.True(t, AppConfig.MaintenanceWindowDbOptimize)
		assert.False(t, AppConfig.MaintenanceWindowLibraryScan)
		assert.False(t, AppConfig.MaintenanceWindowLibraryOrganize)
		assert.False(t, AppConfig.MaintenanceWindowMetadataRefresh)
	})

	t.Run("iTunes defaults", func(t *testing.T) {
		assert.True(t, AppConfig.ITunesSyncEnabled)
		assert.Equal(t, 30, AppConfig.ITunesSyncInterval)
		assert.False(t, AppConfig.ITunesAutoWriteBack)
	})

	t.Run("auto-update defaults", func(t *testing.T) {
		assert.False(t, AppConfig.AutoUpdateEnabled)
		assert.Equal(t, "stable", AppConfig.AutoUpdateChannel)
		assert.Equal(t, 60, AppConfig.AutoUpdateCheckMinutes)
	})

	t.Run("default metadata sources", func(t *testing.T) {
		assert.GreaterOrEqual(t, len(AppConfig.MetadataSources), 3)
		assert.Equal(t, "audible", AppConfig.MetadataSources[0].ID)
		assert.True(t, AppConfig.MetadataSources[0].Enabled)
		assert.Equal(t, "openlibrary", AppConfig.MetadataSources[1].ID)
	})

	t.Run("supported extensions default", func(t *testing.T) {
		assert.Contains(t, AppConfig.SupportedExtensions, ".m4b")
		assert.Contains(t, AppConfig.SupportedExtensions, ".mp3")
		assert.Contains(t, AppConfig.SupportedExtensions, ".flac")
	})

	t.Run("path formatting defaults", func(t *testing.T) {
		assert.Equal(t, "{author}/{series_prefix}{title}/{track_title}.{ext}", AppConfig.PathFormat)
		assert.True(t, AppConfig.AutoRenameOnApply)
		assert.True(t, AppConfig.AutoWriteTagsOnApply)
		assert.True(t, AppConfig.VerifyAfterWrite)
	})
}

func TestInitConfigDatabaseTypeNormalization(t *testing.T) {
	t.Run("sqlite3 normalized to sqlite", func(t *testing.T) {
		viper.Reset()
		viper.Set("database_type", "sqlite3")
		InitConfig()
		assert.Equal(t, "sqlite", AppConfig.DatabaseType)
	})

	t.Run("empty normalized to pebble", func(t *testing.T) {
		viper.Reset()
		viper.Set("database_type", "")
		InitConfig()
		assert.Equal(t, "pebble", AppConfig.DatabaseType)
	})
}

func TestInitConfigITunesBackwardCompat(t *testing.T) {
	t.Run("old itunes_library_itl_path maps to write path", func(t *testing.T) {
		viper.Reset()
		viper.Set("itunes_library_itl_path", "/old/path.itl")
		InitConfig()
		assert.Equal(t, "/old/path.itl", AppConfig.ITunesLibraryWritePath)
	})

	t.Run("old itunes_library_xml_path maps to read path", func(t *testing.T) {
		viper.Reset()
		viper.Set("itunes_library_xml_path", "/old/library.xml")
		InitConfig()
		assert.Equal(t, "/old/library.xml", AppConfig.ITunesLibraryReadPath)
	})

	t.Run("new path takes precedence over old", func(t *testing.T) {
		viper.Reset()
		viper.Set("itunes_library_write_path", "/new/path.itl")
		viper.Set("itunes_library_itl_path", "/old/path.itl")
		InitConfig()
		assert.Equal(t, "/new/path.itl", AppConfig.ITunesLibraryWritePath)
	})
}

func TestInitConfigITLWriteBackAutoEnable(t *testing.T) {
	viper.Reset()
	viper.Set("itunes_library_write_path", "/some/path.itl")
	viper.Set("itl_write_back_enabled", false)
	InitConfig()
	assert.True(t, AppConfig.ITLWriteBackEnabled, "should auto-enable when write path is set")
}

func TestInitConfigOpenLibraryDumpDir(t *testing.T) {
	t.Run("auto-set when root_dir present", func(t *testing.T) {
		viper.Reset()
		viper.Set("root_dir", "/media/audiobooks")
		InitConfig()
		assert.Equal(t, "/media/audiobooks/openlibrary-dumps", AppConfig.OpenLibraryDumpDir)
	})

	t.Run("not set when root_dir empty", func(t *testing.T) {
		viper.Reset()
		InitConfig()
		assert.Equal(t, "", AppConfig.OpenLibraryDumpDir)
	})

	t.Run("explicit value preserved", func(t *testing.T) {
		viper.Reset()
		viper.Set("root_dir", "/media/audiobooks")
		viper.Set("openlibrary_dump_dir", "/custom/dumps")
		InitConfig()
		assert.Equal(t, "/custom/dumps", AppConfig.OpenLibraryDumpDir)
	})
}

func TestResetToDefaultsPreservesPaths(t *testing.T) {
	// Set paths that should be preserved
	AppConfig = Config{
		RootDir:      "/media/audiobooks",
		DatabasePath: "/data/audiobooks.pebble",
		PlaylistDir:  "/media/playlists",
		LogLevel:     "debug",
		CacheSize:    5000,
	}

	ResetToDefaults()

	// Paths should be preserved
	assert.Equal(t, "/media/audiobooks", AppConfig.RootDir)
	assert.Equal(t, "/data/audiobooks.pebble", AppConfig.DatabasePath)
	assert.Equal(t, "/media/playlists", AppConfig.PlaylistDir)

	// Other fields should be reset
	assert.Equal(t, "pebble", AppConfig.DatabaseType)
	assert.Equal(t, "info", AppConfig.LogLevel)
	assert.Equal(t, 1000, AppConfig.CacheSize)
	assert.True(t, AppConfig.AutoOrganize)
	assert.Equal(t, "auto", AppConfig.OrganizationStrategy)
}
