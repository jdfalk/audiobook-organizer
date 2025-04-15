package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/playlist"
	"github.com/jdfalk/audiobook-organizer/internal/scanner"
	"github.com/jdfalk/audiobook-organizer/internal/tagger"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string
var rootDir string
var databasePath string
var playlistDir string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "audiobook-organizer",
	Short: "Organize audiobooks into series and generate playlists",
	Long: `Audiobook Organizer scans your audiobook files, identifies series
using metadata and filenames, and generates iTunes-compatible playlists.

It also updates metadata tags in the audio files to include series information.`,
}

// scanCmd represents the scan command
var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan audiobook directories",
	Long:  `Scan audiobook directories to identify books and series.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if config.AppConfig.RootDir == "" {
			return fmt.Errorf("root directory not specified")
		}

		// Initialize database
		if err := database.Initialize(config.AppConfig.DatabasePath); err != nil {
			return fmt.Errorf("failed to initialize database: %w", err)
		}
		defer database.Close()

		fmt.Printf("Scanning directory: %s\n", config.AppConfig.RootDir)

		// Start scanning
		books, err := scanner.ScanDirectory(config.AppConfig.RootDir)
		if err != nil {
			return fmt.Errorf("scan error: %w", err)
		}

		fmt.Printf("Found %d audiobooks\n", len(books))

		// Process books and identify series
		if err := scanner.ProcessBooks(books); err != nil {
			return fmt.Errorf("processing error: %w", err)
		}

		return nil
	},
}

// playlistCmd represents the playlist command
var playlistCmd = &cobra.Command{
	Use:   "playlist",
	Short: "Generate playlists for audiobook series",
	Long:  `Generate iTunes-compatible playlists for each audiobook series.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Initialize database
		if err := database.Initialize(config.AppConfig.DatabasePath); err != nil {
			return fmt.Errorf("failed to initialize database: %w", err)
		}
		defer database.Close()

		fmt.Println("Generating playlists for audiobook series...")

		// Generate playlists
		if err := playlist.GeneratePlaylistsForSeries(); err != nil {
			return fmt.Errorf("failed to generate playlists: %w", err)
		}

		fmt.Printf("Playlists saved to: %s\n", config.AppConfig.PlaylistDir)
		return nil
	},
}

// tagCmd represents the tag command
var tagCmd = &cobra.Command{
	Use:   "tag",
	Short: "Update audio file tags with series information",
	Long:  `Update the metadata tags of audio files to include series information.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Initialize database
		if err := database.Initialize(config.AppConfig.DatabasePath); err != nil {
			return fmt.Errorf("failed to initialize database: %w", err)
		}
		defer database.Close()

		fmt.Println("Updating audio file tags with series information...")

		// Update tags
		if err := tagger.UpdateSeriesTags(); err != nil {
			return fmt.Errorf("failed to update tags: %w", err)
		}

		return nil
	},
}

// organizeCmd represents the organize command
var organizeCmd = &cobra.Command{
	Use:   "organize",
	Short: "Run the complete organization process",
	Long:  `Scan audiobooks, identify series, generate playlists, and update tags.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if config.AppConfig.RootDir == "" {
			return fmt.Errorf("root directory not specified")
		}

		// Initialize database
		if err := database.Initialize(config.AppConfig.DatabasePath); err != nil {
			return fmt.Errorf("failed to initialize database: %w", err)
		}
		defer database.Close()

		// Step 1: Scan files
		fmt.Printf("Scanning directory: %s\n", config.AppConfig.RootDir)
		books, err := scanner.ScanDirectory(config.AppConfig.RootDir)
		if err != nil {
			return fmt.Errorf("scan error: %w", err)
		}
		fmt.Printf("Found %d audiobooks\n", len(books)

		// Step 2: Process books and identify series
		fmt.Println("Processing audiobooks and identifying series...")
		if err := scanner.ProcessBooks(books); err != nil {
			return fmt.Errorf("processing error: %w", err)
		}

		// Step 3: Generate playlists
		fmt.Println("Generating playlists...")
		if err := playlist.GeneratePlaylistsForSeries(); err != nil {
			return fmt.Errorf("playlist generation error: %w", err)
		}

		// Step 4: Update tags
		fmt.Println("Updating audio file tags...")
		if err := tagger.UpdateSeriesTags(); err != nil {
			return fmt.Errorf("tag update error: %w", err)
		}

		fmt.Println("\nAudiobook organization complete!")
		fmt.Printf("- Database: %s\n", config.AppConfig.DatabasePath)
		fmt.Printf("- Playlists: %s\n", config.AppConfig.PlaylistDir)

		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.audiobook-organizer.yaml)")
	rootCmd.PersistentFlags().StringVar(&rootDir, "dir", "", "root directory containing audiobooks")
	rootCmd.PersistentFlags().StringVar(&databasePath, "db", "audiobooks.db", "path to SQLite database")
	rootCmd.PersistentFlags().StringVar(&playlistDir, "playlists", "playlists", "directory to store generated playlists")

	viper.BindPFlag("root_dir", rootCmd.PersistentFlags().Lookup("dir"))
	viper.BindPFlag("database_path", rootCmd.PersistentFlags().Lookup("db"))
	viper.BindPFlag("playlist_dir", rootCmd.PersistentFlags().Lookup("playlists"))

	rootCmd.AddCommand(scanCmd)
	rootCmd.AddCommand(playlistCmd)
	rootCmd.AddCommand(tagCmd)
	rootCmd.AddCommand(organizeCmd)
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".audiobook-organizer")
	}

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}

	// Create playlist directory if it doesn't exist
	if playlistDir != "" {
		if err := os.MkdirAll(playlistDir, 0755); err != nil {
			fmt.Printf("Error creating playlist directory: %v\n", err)
		}
	}

	// Ensure database directory exists
	if databasePath != "" {
		dbDir := filepath.Dir(databasePath)
		if dbDir != "." {
			if err := os.MkdirAll(dbDir, 0755); err != nil {
				fmt.Printf("Error creating database directory: %v\n", err)
			}
		}
	}

	config.InitConfig()
}
