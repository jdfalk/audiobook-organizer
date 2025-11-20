// file: cmd/root.go
// version: 1.4.0
// guid: 6a7b8c9d-0e1f-2a3b-4c5d-6e7f8a9b0c1d

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	"github.com/jdfalk/audiobook-organizer/internal/playlist"
	"github.com/jdfalk/audiobook-organizer/internal/realtime"
	"github.com/jdfalk/audiobook-organizer/internal/scanner"
	"github.com/jdfalk/audiobook-organizer/internal/server"
	"github.com/jdfalk/audiobook-organizer/internal/tagger"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string
var rootDir string
var databasePath string
var databaseType string
var enableSQLite bool
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
		if err := database.InitializeStore(config.AppConfig.DatabaseType, config.AppConfig.DatabasePath, config.AppConfig.EnableSQLite); err != nil {
			return fmt.Errorf("failed to initialize database: %w", err)
		}
		defer database.CloseStore()

		fmt.Printf("Using database: %s (%s)\n", config.AppConfig.DatabasePath, config.AppConfig.DatabaseType)
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
		if err := database.InitializeStore(config.AppConfig.DatabaseType, config.AppConfig.DatabasePath, config.AppConfig.EnableSQLite); err != nil {
			return fmt.Errorf("failed to initialize database: %w", err)
		}
		defer database.CloseStore()

		fmt.Printf("Using database: %s (%s)\n", config.AppConfig.DatabasePath, config.AppConfig.DatabaseType)
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
		if err := database.InitializeStore(config.AppConfig.DatabaseType, config.AppConfig.DatabasePath, config.AppConfig.EnableSQLite); err != nil {
			return fmt.Errorf("failed to initialize database: %w", err)
		}
		defer database.CloseStore()

		fmt.Printf("Using database: %s (%s)\n", config.AppConfig.DatabasePath, config.AppConfig.DatabaseType)
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
		if err := database.InitializeStore(config.AppConfig.DatabaseType, config.AppConfig.DatabasePath, config.AppConfig.EnableSQLite); err != nil {
			return fmt.Errorf("failed to initialize database: %w", err)
		}
		defer database.CloseStore()

		// Step 1: Scan files
		fmt.Printf("Using database: %s (%s)\n", config.AppConfig.DatabasePath, config.AppConfig.DatabaseType)
		fmt.Printf("Scanning directory: %s\n", config.AppConfig.RootDir)
		books, err := scanner.ScanDirectory(config.AppConfig.RootDir)
		if err != nil {
			return fmt.Errorf("scan error: %w", err)
		}
		fmt.Printf("Found %d audiobooks\n", len(books))

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

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the web server",
	Long:  `Start the web server to provide a web interface for audiobook management.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Initialize database
		if err := database.InitializeStore(config.AppConfig.DatabaseType, config.AppConfig.DatabasePath, config.AppConfig.EnableSQLite); err != nil {
			return fmt.Errorf("failed to initialize database: %w", err)
		}
		defer database.CloseStore()

		fmt.Printf("Using database: %s (%s)\n", config.AppConfig.DatabasePath, config.AppConfig.DatabaseType)

		// Initialize encryption for settings (generates key if needed)
		dbDir := filepath.Dir(config.AppConfig.DatabasePath)
		if err := database.InitEncryption(dbDir); err != nil {
			return fmt.Errorf("failed to initialize encryption: %w", err)
		}
		fmt.Println("Settings encryption initialized")

		// Load configuration from database (overrides defaults with persisted values)
		if err := config.LoadConfigFromDatabase(database.GlobalStore); err != nil {
			fmt.Printf("Warning: Could not load config from database: %v\n", err)
		}

		// Apply env var overrides (command line takes precedence over DB)
		config.SyncConfigFromEnv()

		fmt.Println("Starting audiobook organizer web server...")

		// Initialize real-time event hub
		realtime.InitializeEventHub()
		fmt.Println("Real-time event hub initialized")

		// Initialize operation queue with 2 workers
		workers := 2
		if w := cmd.Flag("workers").Value.String(); w != "" {
			fmt.Sscanf(w, "%d", &workers)
		}
		operations.InitializeQueue(database.GlobalStore, workers)
		defer func() {
			fmt.Println("Shutting down operation queue...")
			if err := operations.ShutdownQueue(30 * time.Second); err != nil {
				fmt.Printf("Warning: operation queue shutdown error: %v\n", err)
			}
		}()
		fmt.Printf("Operation queue initialized with %d workers\n", workers)

		// Create and start server
		srv := server.NewServer()
		cfg := server.GetDefaultServerConfig()

		// Override with command line flags if provided
		if port := cmd.Flag("port").Value.String(); port != "" {
			cfg.Port = port
		}
		if host := cmd.Flag("host").Value.String(); host != "" {
			cfg.Host = host
		}
		if rt := cmd.Flag("read-timeout").Value.String(); rt != "" {
			if d, err := time.ParseDuration(rt); err == nil {
				cfg.ReadTimeout = d
			}
		}
		if wt := cmd.Flag("write-timeout").Value.String(); wt != "" {
			if d, err := time.ParseDuration(wt); err == nil {
				cfg.WriteTimeout = d
			}
		}
		if it := cmd.Flag("idle-timeout").Value.String(); it != "" {
			if d, err := time.ParseDuration(it); err == nil {
				cfg.IdleTimeout = d
			}
		}

		return srv.Start(cfg)
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
	rootCmd.PersistentFlags().StringVar(&databasePath, "db", "audiobooks.pebble", "path to database (default: audiobooks.pebble for PebbleDB)")
	rootCmd.PersistentFlags().StringVar(&databaseType, "db-type", "pebble", "database type: pebble (default) or sqlite")
	rootCmd.PersistentFlags().BoolVar(&enableSQLite, "enable-sqlite3-i-know-the-risks", false, "enable SQLite3 database (WARNING: cross-compilation issues, PebbleDB recommended)")
	rootCmd.PersistentFlags().StringVar(&playlistDir, "playlists", "playlists", "directory to store generated playlists")

	viper.BindPFlag("root_dir", rootCmd.PersistentFlags().Lookup("dir"))
	viper.BindPFlag("database_path", rootCmd.PersistentFlags().Lookup("db"))
	viper.BindPFlag("database_type", rootCmd.PersistentFlags().Lookup("db-type"))
	viper.BindPFlag("enable_sqlite3_i_know_the_risks", rootCmd.PersistentFlags().Lookup("enable-sqlite3-i-know-the-risks"))
	viper.BindPFlag("playlist_dir", rootCmd.PersistentFlags().Lookup("playlists"))

	rootCmd.AddCommand(scanCmd)
	rootCmd.AddCommand(playlistCmd)
	rootCmd.AddCommand(tagCmd)
	rootCmd.AddCommand(organizeCmd)
	rootCmd.AddCommand(serveCmd)

	// Add serve command specific flags
	serveCmd.Flags().String("port", "8080", "port to run the web server on")
	serveCmd.Flags().String("host", "localhost", "host to bind the web server to")
	serveCmd.Flags().String("read-timeout", "15s", "read timeout (e.g. 15s, 1m)")
	serveCmd.Flags().String("write-timeout", "15s", "write timeout (e.g. 15s, 1m)")
	serveCmd.Flags().String("idle-timeout", "60s", "idle timeout (e.g. 60s, 2m)")
	serveCmd.Flags().Int("workers", 2, "number of background operation workers")
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
