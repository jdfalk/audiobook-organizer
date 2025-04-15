package config

import (
	"github.com/spf13/viper"
)

// Config holds application configuration
type Config struct {
	RootDir      string
	DatabasePath string
	PlaylistDir  string
	APIKeys      struct {
		Goodreads string
	}
	SupportedExtensions []string
}

var AppConfig Config

// InitConfig initializes the application configuration
func InitConfig() {
	AppConfig = Config{
		RootDir:      viper.GetString("root_dir"),
		DatabasePath: viper.GetString("database_path"),
		PlaylistDir:  viper.GetString("playlist_dir"),
		SupportedExtensions: []string{
			".m4b", ".mp3", ".m4a", ".aac", ".ogg", ".flac", ".wma",
		},
	}

	// API Keys
	AppConfig.APIKeys.Goodreads = viper.GetString("api_keys.goodreads")
}
