// file: internal/config/config.go
// version: 1.2.0
// guid: 7b8c9d0e-1f2a-3b4c-5d6e-7f8a9b0c1d2e

package config

import (
	"github.com/spf13/viper"
)

// Config holds application configuration
type Config struct {
	RootDir      string
	DatabasePath string
	DatabaseType string // "pebble" (default) or "sqlite"
	EnableSQLite bool   // Must be true to use SQLite (safety flag)
	PlaylistDir  string
	APIKeys      struct {
		Goodreads string
	}
	SupportedExtensions []string
}

var AppConfig Config

// InitConfig initializes the application configuration
func InitConfig() {
	// Set defaults
	viper.SetDefault("database_type", "pebble")
	viper.SetDefault("enable_sqlite3_i_know_the_risks", false)
	
	AppConfig = Config{
		RootDir:      viper.GetString("root_dir"),
		DatabasePath: viper.GetString("database_path"),
		DatabaseType: viper.GetString("database_type"),
		EnableSQLite: viper.GetBool("enable_sqlite3_i_know_the_risks"),
		PlaylistDir:  viper.GetString("playlist_dir"),
		SupportedExtensions: []string{
			".m4b", ".mp3", ".m4a", ".aac", ".ogg", ".flac", ".wma",
		},
	}

	// API Keys
	AppConfig.APIKeys.Goodreads = viper.GetString("api_keys.goodreads")
	
	// Normalize database type
	if AppConfig.DatabaseType == "sqlite3" {
		AppConfig.DatabaseType = "sqlite"
	}
	if AppConfig.DatabaseType == "" {
		AppConfig.DatabaseType = "pebble"
	}
}
