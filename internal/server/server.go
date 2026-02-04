// file: internal/server/server.go
// version: 1.54.0
// guid: 4c5d6e7f-8a9b-0c1d-2e3f-4a5b6c7d8e9f

package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/ai"
	"github.com/jdfalk/audiobook-organizer/internal/backup"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
	"github.com/jdfalk/audiobook-organizer/internal/metrics"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	"github.com/jdfalk/audiobook-organizer/internal/organizer"
	"github.com/jdfalk/audiobook-organizer/internal/realtime"
	"github.com/jdfalk/audiobook-organizer/internal/scanner"
	ulid "github.com/oklog/ulid/v2"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/quic-go/quic-go/http3"
	"golang.org/x/net/http2"
)

// Cached library and import path sizes to avoid expensive recalculation on frequent status checks
var cachedLibrarySize int64
var cachedImportSize int64
var cachedSizeComputedAt time.Time
var cacheLock sync.RWMutex

const librarySizeCacheTTL = 60 * time.Second

// resetLibrarySizeCache resets the library size cache (for testing)
func resetLibrarySizeCache() {
	cacheLock.Lock()
	defer cacheLock.Unlock()
	cachedLibrarySize = 0
	cachedImportSize = 0
	cachedSizeComputedAt = time.Time{}
}

// Helper functions for pointer conversions
func stringPtr(s string) *string {
	return &s
}

func intPtrHelper(i int) *int {
	return &i
}

func boolPtr(b bool) *bool {
	return &b
}

type aiParser interface {
	IsEnabled() bool
	ParseFilename(ctx context.Context, filename string) (*ai.ParsedMetadata, error)
	TestConnection(ctx context.Context) error
}

var newAIParser = func(apiKey string, enabled bool) aiParser {
	return ai.NewOpenAIParser(apiKey, enabled)
}

func metadataStateKey(bookID string) string {
	return fmt.Sprintf("metadata_state_%s", bookID)
}

func decodeMetadataValue(raw *string) any {
	if raw == nil || *raw == "" {
		return nil
	}
	var value any
	if err := json.Unmarshal([]byte(*raw), &value); err != nil {
		return *raw
	}
	return value
}

func encodeMetadataValue(value any) (*string, error) {
	if value == nil {
		return nil, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	encoded := string(data)
	return &encoded, nil
}

func loadLegacyMetadataState(bookID string) (map[string]metadataFieldState, error) {
	state := map[string]metadataFieldState{}

	pref, err := database.GlobalStore.GetUserPreference(metadataStateKey(bookID))
	if err != nil {
		return state, err
	}
	if pref == nil || pref.Value == nil || *pref.Value == "" {
		return state, nil
	}

	if err := json.Unmarshal([]byte(*pref.Value), &state); err != nil {
		return state, fmt.Errorf("failed to parse metadata state: %w", err)
	}
	return state, nil
}

func loadMetadataState(bookID string) (map[string]metadataFieldState, error) {
	state := map[string]metadataFieldState{}
	if database.GlobalStore == nil {
		return state, fmt.Errorf("database not initialized")
	}

	stored, err := database.GlobalStore.GetMetadataFieldStates(bookID)
	if err != nil {
		return state, err
	}
	for _, entry := range stored {
		state[entry.Field] = metadataFieldState{
			FetchedValue:   decodeMetadataValue(entry.FetchedValue),
			OverrideValue:  decodeMetadataValue(entry.OverrideValue),
			OverrideLocked: entry.OverrideLocked,
			UpdatedAt:      entry.UpdatedAt,
		}
	}
	if len(state) > 0 {
		return state, nil
	}

	legacy, err := loadLegacyMetadataState(bookID)
	if err != nil {
		return state, err
	}
	if len(legacy) == 0 {
		return state, nil
	}

	if err := saveMetadataState(bookID, legacy); err != nil {
		log.Printf("[WARN] failed to migrate legacy metadata state for %s: %v", bookID, err)
	}
	return legacy, nil
}

func saveMetadataState(bookID string, state map[string]metadataFieldState) error {
	if database.GlobalStore == nil {
		return fmt.Errorf("database not initialized")
	}

	existing, err := database.GlobalStore.GetMetadataFieldStates(bookID)
	if err != nil {
		return err
	}
	existingFields := map[string]struct{}{}
	for _, entry := range existing {
		existingFields[entry.Field] = struct{}{}
	}

	now := time.Now()
	for field, entry := range state {
		fetched, err := encodeMetadataValue(entry.FetchedValue)
		if err != nil {
			return fmt.Errorf("failed to encode fetched metadata for %s: %w", field, err)
		}
		override, err := encodeMetadataValue(entry.OverrideValue)
		if err != nil {
			return fmt.Errorf("failed to encode override metadata for %s: %w", field, err)
		}
		if entry.UpdatedAt.IsZero() {
			entry.UpdatedAt = now
		}

		dbState := database.MetadataFieldState{
			BookID:         bookID,
			Field:          field,
			FetchedValue:   fetched,
			OverrideValue:  override,
			OverrideLocked: entry.OverrideLocked,
			UpdatedAt:      entry.UpdatedAt,
		}

		if err := database.GlobalStore.UpsertMetadataFieldState(&dbState); err != nil {
			return fmt.Errorf("failed to persist metadata state for %s: %w", field, err)
		}
		delete(existingFields, field)
	}

	for field := range existingFields {
		if err := database.GlobalStore.DeleteMetadataFieldState(bookID, field); err != nil {
			return fmt.Errorf("failed to clean up metadata state for %s: %w", field, err)
		}
	}

	return nil
}

func decodeRawValue(raw json.RawMessage) any {
	if raw == nil {
		return nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return string(raw)
	}
	return value
}

func updateFetchedMetadataState(bookID string, values map[string]any) error {
	state, err := loadMetadataState(bookID)
	if err != nil {
		return err
	}
	if state == nil {
		state = map[string]metadataFieldState{}
	}
	for field, value := range values {
		entry := state[field]
		entry.FetchedValue = value
		entry.UpdatedAt = time.Now()
		state[field] = entry
	}
	return saveMetadataState(bookID, state)
}

func stringVal(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

func intVal(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func resolveAuthorAndSeriesNames(book *database.Book) (string, string) {
	authorName := ""
	if book.Author != nil {
		authorName = book.Author.Name
	} else if book.AuthorID != nil {
		if author, err := database.GlobalStore.GetAuthorByID(*book.AuthorID); err == nil && author != nil {
			authorName = author.Name
		}
	}

	seriesName := ""
	if book.Series != nil {
		seriesName = book.Series.Name
	} else if book.SeriesID != nil {
		if series, err := database.GlobalStore.GetSeriesByID(*book.SeriesID); err == nil && series != nil {
			seriesName = series.Name
		}
	}

	return authorName, seriesName
}

func buildMetadataProvenance(book *database.Book, state map[string]metadataFieldState, meta metadata.Metadata, authorName, seriesName string) map[string]database.MetadataProvenanceEntry {
	if state == nil {
		state = map[string]metadataFieldState{}
	}

	provenance := map[string]database.MetadataProvenanceEntry{}

	addEntry := func(field string, fileValue any, storedValue any) {
		entryState := state[field]
		effectiveSource := ""
		var effectiveValue any
		switch {
		case entryState.OverrideValue != nil:
			effectiveSource = "override"
			effectiveValue = entryState.OverrideValue
		case storedValue != nil:
			effectiveSource = "stored"
			effectiveValue = storedValue
		case entryState.FetchedValue != nil:
			effectiveSource = "fetched"
			effectiveValue = entryState.FetchedValue
		case fileValue != nil:
			effectiveSource = "file"
			effectiveValue = fileValue
		}

		var updatedAt *time.Time
		if !entryState.UpdatedAt.IsZero() {
			ts := entryState.UpdatedAt.UTC()
			updatedAt = &ts
		}

		provenance[field] = database.MetadataProvenanceEntry{
			FileValue:       fileValue,
			FetchedValue:    entryState.FetchedValue,
			StoredValue:     storedValue,
			OverrideValue:   entryState.OverrideValue,
			OverrideLocked:  entryState.OverrideLocked,
			EffectiveValue:  effectiveValue,
			EffectiveSource: effectiveSource,
			UpdatedAt:       updatedAt,
		}
	}

	addEntry("title", meta.Title, book.Title)
	addEntry("author_name", meta.Artist, authorName)
	addEntry("narrator", meta.Narrator, stringVal(book.Narrator))
	addEntry("series_name", meta.Series, seriesName)
	addEntry("publisher", meta.Publisher, stringVal(book.Publisher))
	addEntry("language", meta.Language, stringVal(book.Language))
	addEntry("audiobook_release_year", meta.Year, intVal(book.AudiobookReleaseYear))
	addEntry("isbn10", meta.ISBN10, stringVal(book.ISBN10))
	addEntry("isbn13", meta.ISBN13, stringVal(book.ISBN13))

	return provenance
}

func stringFromSeries(series *database.Series) any {
	if series == nil {
		return nil
	}
	return series.Name
}

func applyOrganizedFileMetadata(book *database.Book, newPath string) {
	hash, err := scanner.ComputeFileHash(newPath)
	if err != nil {
		log.Printf("[WARN] failed to compute organized hash for %s: %v", newPath, err)
	} else if hash != "" {
		book.FileHash = stringPtr(hash)
		book.OrganizedFileHash = stringPtr(hash)
		if book.OriginalFileHash == nil {
			book.OriginalFileHash = stringPtr(hash)
		}
	}
	if info, err := os.Stat(newPath); err == nil {
		size := info.Size()
		book.FileSize = &size
	}
}

// calculateLibrarySizes computes library and import path sizes with caching
func calculateLibrarySizes(rootDir string, importFolders []database.ImportPath) (librarySize, importSize int64) {
	cacheLock.RLock()
	if time.Since(cachedSizeComputedAt) < librarySizeCacheTTL {
		librarySize = cachedLibrarySize
		importSize = cachedImportSize
		cacheLock.RUnlock()
		log.Printf("[DEBUG] Using cached sizes: library=%d, import=%d", librarySize, importSize)
		return
	}
	cacheLock.RUnlock()

	// Cache expired, recalculate
	cacheLock.Lock()
	defer cacheLock.Unlock()

	// Double-check in case another goroutine just updated
	if time.Since(cachedSizeComputedAt) < librarySizeCacheTTL {
		return cachedLibrarySize, cachedImportSize
	}

	log.Printf("[DEBUG] Recalculating library sizes (cache expired)")

	// Calculate library size
	librarySize = 0
	if rootDir != "" {
		if info, err := os.Stat(rootDir); err == nil && info.IsDir() {
			filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
				if err == nil && !info.IsDir() {
					librarySize += info.Size()
				}
				return nil
			})
		}
	}

	// Calculate import path sizes independently (not by subtraction)
	importSize = 0
	for _, folder := range importFolders {
		if !folder.Enabled {
			continue
		}
		if info, err := os.Stat(folder.Path); err == nil && info.IsDir() {
			filepath.Walk(folder.Path, func(path string, info os.FileInfo, err error) error {
				if err == nil && !info.IsDir() {
					// Skip files that are under rootDir to avoid double counting
					if rootDir != "" && strings.HasPrefix(path, rootDir) {
						return nil
					}
					importSize += info.Size()
				}
				return nil
			})
		}
	}

	// Update cache
	cachedLibrarySize = librarySize
	cachedImportSize = importSize
	cachedSizeComputedAt = time.Now()

	log.Printf("[DEBUG] Calculated new sizes: library=%d, import=%d", librarySize, importSize)
	return
}

// Server represents the HTTP server
type Server struct {
	httpServer               *http.Server
	router                   *gin.Engine
	audiobookService         *AudiobookService
	audiobookUpdateService   *AudiobookUpdateService
	batchService             *BatchService
	workService              *WorkService
	authorSeriesService      *AuthorSeriesService
	filesystemService        *FilesystemService
	importPathService        *ImportPathService
	importService            *ImportService
	scanService              *ScanService
	organizeService          *OrganizeService
	metadataFetchService     *MetadataFetchService
	configUpdateService      *ConfigUpdateService
	systemService            *SystemService
	metadataStateService     *MetadataStateService
	dashboardService         *DashboardService
}

// ServerConfig holds server configuration
type ServerConfig struct {
	Port         string
	Host         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
	TLSCertFile  string // Optional TLS certificate file for HTTPS/HTTP2/HTTP3
	TLSKeyFile   string // Optional TLS key file for HTTPS/HTTP2/HTTP3
	HTTP3Port    string // Optional HTTP/3 port (UDP). If set with TLS, enables HTTP/3
}

// NewServer creates a new server instance
func NewServer() *Server {
	router := gin.Default()

	// Set up middleware
	router.Use(gin.Logger())
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())

	// Register metrics (idempotent)
	metrics.Register()

	server := &Server{
		router:                   router,
		audiobookService:         NewAudiobookService(database.GlobalStore),
		audiobookUpdateService:   NewAudiobookUpdateService(database.GlobalStore),
		batchService:             NewBatchService(database.GlobalStore),
		workService:              NewWorkService(database.GlobalStore),
		authorSeriesService:      NewAuthorSeriesService(database.GlobalStore),
		filesystemService:        NewFilesystemService(),
		importPathService:        NewImportPathService(database.GlobalStore),
		importService:            NewImportService(database.GlobalStore),
		scanService:              NewScanService(database.GlobalStore),
		organizeService:          NewOrganizeService(database.GlobalStore),
		metadataFetchService:     NewMetadataFetchService(database.GlobalStore),
		configUpdateService:      NewConfigUpdateService(database.GlobalStore),
		systemService:            NewSystemService(database.GlobalStore),
		metadataStateService:     NewMetadataStateService(database.GlobalStore),
		dashboardService:         NewDashboardService(database.GlobalStore),
	}

	server.setupRoutes()

	return server
}

// Start starts the HTTP server
func (s *Server) Start(cfg ServerConfig) error {
	s.httpServer = &http.Server{
		Addr:           fmt.Sprintf("%s:%s", cfg.Host, cfg.Port),
		Handler:        s.router,
		ReadTimeout:    cfg.ReadTimeout,
		WriteTimeout:   cfg.WriteTimeout,
		IdleTimeout:    cfg.IdleTimeout,
		MaxHeaderBytes: 1 << 20, // 1MB
	}

	// Enable HTTP/2 if TLS is configured
	if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
		// Configure TLS with HTTP/2 (and optionally HTTP/3)
		nextProtos := []string{"h2", "http/1.1"}
		if cfg.HTTP3Port != "" {
			// Add h3 to advertised protocols
			nextProtos = append([]string{"h3"}, nextProtos...)
		}
		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS12,
			NextProtos: nextProtos,
		}
		s.httpServer.TLSConfig = tlsConfig

		// Explicitly configure HTTP/2
		if err := http2.ConfigureServer(s.httpServer, &http2.Server{}); err != nil {
			return fmt.Errorf("failed to configure HTTP/2: %w", err)
		}

		// Add Alt-Svc header to advertise HTTP/3 if enabled
		if cfg.HTTP3Port != "" {
			s.router.Use(func(c *gin.Context) {
				c.Header("Alt-Svc", fmt.Sprintf(`h3=":%s"; ma=2592000`, cfg.HTTP3Port))
				c.Next()
			})
		}

		// Start HTTPS server with HTTP/2
		go func() {
			protocols := "HTTPS/HTTP2"
			if cfg.HTTP3Port != "" {
				protocols = "HTTPS/HTTP2 (HTTP/3 on UDP port " + cfg.HTTP3Port + ")"
			}
			log.Printf("Starting %s server on %s", protocols, s.httpServer.Addr)
			if err := s.httpServer.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile); err != nil && err != http.ErrServerClosed {
				log.Fatalf("Failed to start HTTPS server: %v", err)
			}
		}()

		// Start HTTP/3 server if configured
		if cfg.HTTP3Port != "" {
			go func() {
				http3Server := &http3.Server{
					Addr:      fmt.Sprintf("%s:%s", cfg.Host, cfg.HTTP3Port),
					Handler:   s.router,
					TLSConfig: tlsConfig,
				}
				log.Printf("Starting HTTP/3 (QUIC) server on UDP %s:%s", cfg.Host, cfg.HTTP3Port)
				if err := http3Server.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile); err != nil {
					log.Fatalf("Failed to start HTTP/3 server: %v", err)
				}
			}()
		}

		// Start HTTP to HTTPS redirect server on port 80
		go func() {
			redirectAddr := fmt.Sprintf("%s:80", cfg.Host)
			httpsPort := cfg.Port
			if httpsPort == "80" {
				httpsPort = "443" // Don't redirect 80->80
			}

			redirectHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Build HTTPS URL
				target := "https://" + r.Host
				// Add port if not default HTTPS port
				if httpsPort != "443" {
					target = fmt.Sprintf("https://%s:%s", cfg.Host, httpsPort)
				}
				target += r.URL.RequestURI()

				log.Printf("HTTP->HTTPS redirect: %s -> %s", r.URL.String(), target)
				http.Redirect(w, r, target, http.StatusMovedPermanently)
			})

			log.Printf("Starting HTTP->HTTPS redirect server on %s (redirects to :%s)", redirectAddr, httpsPort)
			httpRedirectServer := &http.Server{
				Addr:    redirectAddr,
				Handler: redirectHandler,
			}
			if err := httpRedirectServer.ListenAndServe(); err != nil {
				// Don't fatal - port 80 might require sudo
				log.Printf("Warning: HTTP redirect server failed (port 80 may require sudo): %v", err)
			}
		}()
	} else {
		// Start HTTP/1.1 server without TLS
		go func() {
			log.Printf("Starting HTTP/1.1 server on %s (use --tls-cert and --tls-key for HTTP/2, add --http3-port for HTTP/3)", s.httpServer.Addr)
			if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("Failed to start server: %v", err)
			}
		}()
	}

	// Heartbeat: push periodic system.status events via SSE (every 5s) while running
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	shutdown := make(chan struct{})
	var backgroundWG sync.WaitGroup
	ticker := time.NewTicker(5 * time.Second)
	backgroundWG.Add(1)
	go func() {
		defer backgroundWG.Done()
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if hub := realtime.GetGlobalHub(); hub != nil {
					// Gather lightweight metrics
					var alloc runtime.MemStats
					runtime.ReadMemStats(&alloc)
					bookCount := 0
					folderCount := 0
					if database.GlobalStore != nil {
						if bc, err := database.GlobalStore.CountBooks(); err == nil {
							bookCount = bc
						} else {
							log.Printf("[DEBUG] Heartbeat: Failed to count books: %v", err)
						}
						if folders, err := database.GlobalStore.GetAllImportPaths(); err == nil {
							folderCount = len(folders)
							log.Printf("[DEBUG] Heartbeat: Got %d import paths", folderCount)
						} else {
							log.Printf("[DEBUG] Heartbeat: Failed to get import paths: %v", err)
						}
					}

					// Update Prometheus metrics
					metrics.SetBooks(bookCount)
					metrics.SetFolders(folderCount)
					metrics.SetMemoryAlloc(alloc.Alloc)
					metrics.SetGoroutines(runtime.NumGoroutine())

					hub.SendSystemStatus(map[string]any{
						"books":        bookCount,
						"folders":      folderCount,
						"memory_alloc": alloc.Alloc,
						"goroutines":   runtime.NumGoroutine(),
						"timestamp":    time.Now().Unix(),
					})
				}
			case <-shutdown:
				return
			}
		}
	}()

	// Background purge of soft-deleted books (configurable retention)
	if config.AppConfig.PurgeSoftDeletedAfterDays > 0 {
		purgeTicker := time.NewTicker(6 * time.Hour)
		backgroundWG.Add(1)
		go func() {
			defer backgroundWG.Done()
			defer purgeTicker.Stop()
			// Initial run on startup
			s.runAutoPurgeSoftDeleted()
			for {
				select {
				case <-purgeTicker.C:
					s.runAutoPurgeSoftDeleted()
				case <-shutdown:
					return
				}
			}
		}()
	}

	// Wait for interrupt signal to gracefully shutdown the server
	<-quit
	close(shutdown)
	signal.Stop(quit)

	log.Println("Shutting down server...")

	// Broadcast shutdown event to all connected clients
	if hub := realtime.GetGlobalHub(); hub != nil {
		hub.Broadcast(&realtime.Event{
			Type: "system.shutdown",
			Data: map[string]any{
				"message": "Server is shutting down",
			},
		})
		// Give clients a moment to receive the event
		time.Sleep(500 * time.Millisecond)
	}

	// Give outstanding requests a deadline for completion
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.httpServer.Shutdown(ctx); err != nil {
		backgroundWG.Wait()
		return fmt.Errorf("server forced to shutdown: %w", err)
	}

	backgroundWG.Wait()
	log.Println("Server exited")
	return nil
}

// setupRoutes configures all the routes
func (s *Server) setupRoutes() {
	// Health check endpoint
	// Prometheus metrics endpoint (standard path)
	s.router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Health check endpoint (both paths for compatibility)
	s.router.GET("/api/health", s.healthCheck)
	s.router.GET("/api/v1/health", s.healthCheck)

	// Real-time events (SSE)
	s.router.GET("/api/events", s.handleEvents)

	// Redirect /api/* to /api/v1/* for v1 compatibility
	s.router.Use(func(c *gin.Context) {
		path := c.Request.URL.Path
		// If path starts with /api/ but not /api/v1/ and not /api/health and not /api/events
		if strings.HasPrefix(path, "/api/") &&
			!strings.HasPrefix(path, "/api/v1/") &&
			!strings.HasPrefix(path, "/api/health") &&
			!strings.HasPrefix(path, "/api/events") &&
			!strings.HasPrefix(path, "/api/metrics") {
			// Redirect to /api/v1/
			newPath := strings.Replace(path, "/api/", "/api/v1/", 1)
			c.Redirect(http.StatusMovedPermanently, newPath)
			c.Abort()
			return
		}
		c.Next()
	})

	// API routes
	api := s.router.Group("/api/v1")
	{
		// Audiobook routes
		api.GET("/audiobooks", s.listAudiobooks)
		api.GET("/audiobooks/count", s.countAudiobooks)
		api.GET("/audiobooks/duplicates", s.listDuplicateAudiobooks)
		api.GET("/audiobooks/soft-deleted", s.listSoftDeletedAudiobooks)
		api.DELETE("/audiobooks/purge-soft-deleted", s.purgeSoftDeletedAudiobooks)
		api.POST("/audiobooks/:id/restore", s.restoreAudiobook)
		api.GET("/audiobooks/:id", s.getAudiobook)
		api.GET("/audiobooks/:id/tags", s.getAudiobookTags)
		api.PUT("/audiobooks/:id", s.updateAudiobook)
		api.DELETE("/audiobooks/:id", s.deleteAudiobook)
		api.POST("/audiobooks/batch", s.batchUpdateAudiobooks)

		// Author and series routes
		api.GET("/authors", s.listAuthors)
		api.GET("/series", s.listSeries)

		// File system routes
		api.GET("/filesystem/home", s.getHomeDirectory)
		api.GET("/filesystem/browse", s.browseFilesystem)
		api.POST("/filesystem/exclude", s.createExclusion)
		api.DELETE("/filesystem/exclude", s.removeExclusion)

		// Import path routes
		api.GET("/import-paths", s.listImportPaths)
		api.POST("/import-paths", s.addImportPath)
		api.DELETE("/import-paths/:id", s.removeImportPath)

		// Operation routes
		api.POST("/operations/scan", s.startScan)
		api.POST("/operations/organize", s.startOrganize)
		api.GET("/operations/:id/status", s.getOperationStatus)
		api.GET("/operations/:id/logs", s.getOperationLogs)
		api.DELETE("/operations/:id", s.cancelOperation)
		api.GET("/operations/active", s.listActiveOperations)

		// Import routes
		api.POST("/import/file", s.importFile)

		// iTunes import routes
		itunesGroup := api.Group("/itunes")
		{
			itunesGroup.POST("/validate", s.handleITunesValidate)
			itunesGroup.POST("/import", s.handleITunesImport)
			itunesGroup.POST("/write-back", s.handleITunesWriteBack)
			itunesGroup.GET("/import-status/:id", s.handleITunesImportStatus)
		}

		// System routes
		api.GET("/system/status", s.getSystemStatus)
		api.GET("/system/logs", s.getSystemLogs)
		api.POST("/system/reset", s.resetSystem)
		api.GET("/config", s.getConfig)
		api.PUT("/config", s.updateConfig)
		api.GET("/dashboard", s.getDashboard)

		// Backup routes
		api.POST("/backup/create", s.createBackup)
		api.GET("/backup/list", s.listBackups)
		api.POST("/backup/restore", s.restoreBackup)
		api.DELETE("/backup/:filename", s.deleteBackup)

		// Enhanced metadata routes
		api.POST("/metadata/batch-update", s.batchUpdateMetadata)
		api.POST("/metadata/validate", s.validateMetadata)
		api.GET("/metadata/export", s.exportMetadata)
		api.POST("/metadata/import", s.importMetadata)
		api.GET("/metadata/search", s.searchMetadata)
		api.GET("/metadata/fields", s.getMetadataFields)
		api.POST("/metadata/bulk-fetch", s.bulkFetchMetadata)
		api.POST("/audiobooks/:id/fetch-metadata", s.fetchAudiobookMetadata)

		// AI-powered parsing routes
		api.POST("/ai/parse-filename", s.parseFilenameWithAI)
		api.POST("/ai/test-connection", s.testAIConnection)
		api.POST("/audiobooks/:id/parse-with-ai", s.parseAudiobookWithAI)

		// Work routes (logical title-level grouping)
		api.GET("/works", s.listWorks)
		api.POST("/works", s.createWork)
		api.GET("/works/:id", s.getWork)
		api.PUT("/works/:id", s.updateWork)
		api.DELETE("/works/:id", s.deleteWork)
		api.GET("/works/:id/books", s.listWorkBooks)

		// Version management routes
		api.GET("/audiobooks/:id/versions", s.listAudiobookVersions)
		api.POST("/audiobooks/:id/versions", s.linkAudiobookVersion)
		api.PUT("/audiobooks/:id/set-primary", s.setAudiobookPrimary)
		api.GET("/version-groups/:id", s.getVersionGroup)

		// Work queue routes (alternative singular form for compatibility)
		api.GET("/work", s.listWork)
		api.GET("/work/stats", s.getWorkStats)

		// Blocked hashes management routes
		api.GET("/blocked-hashes", s.listBlockedHashes)
		api.POST("/blocked-hashes", s.addBlockedHash)
		api.DELETE("/blocked-hashes/:hash", s.removeBlockedHash)
	}

	// Serve static files (React frontend)
	// Implementation is in static_embed.go or static_nonembed.go depending on build tags
	s.setupStaticFiles()
}

// corsMiddleware adds CORS headers
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Header("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

// Handler functions (stubs for now)
func (s *Server) healthCheck(c *gin.Context) {
	// Gather basic metrics; tolerate errors (don't fail health entirely)
	var bookCount, authorCount, seriesCount, playlistCount int
	var dbErr error
	if database.GlobalStore != nil {
		if bc, err := database.GlobalStore.CountBooks(); err == nil {
			bookCount = bc
		} else {
			dbErr = err
		}
		if authors, err := database.GlobalStore.GetAllAuthors(); err == nil {
			authorCount = len(authors)
		} else if dbErr == nil {
			dbErr = err
		}
		if series, err := database.GlobalStore.GetAllSeries(); err == nil {
			seriesCount = len(series)
		} else if dbErr == nil {
			dbErr = err
		}
		if playlists, err := database.GlobalStore.GetPlaylistBySeriesID(0); err == nil && playlists != nil { // legacy placeholder (0 unlikely valid series)
			playlistCount = 1 // minimal indicator; real playlist counting not yet implemented
		}
	}
	resp := gin.H{
		"status":        "ok",
		"timestamp":     time.Now().Unix(),
		"version":       "1.1.0",
		"database_type": config.AppConfig.DatabaseType,
		"metrics": gin.H{
			"books":     bookCount,
			"authors":   authorCount,
			"series":    seriesCount,
			"playlists": playlistCount,
		},
	}
	if dbErr != nil {
		resp["partial_error"] = dbErr.Error()
	}
	c.JSON(http.StatusOK, resp)
}

func (s *Server) listAudiobooks(c *gin.Context) {
	// Parse pagination parameters
	params := ParsePaginationParams(c)
	authorID := ParseQueryIntPtr(c, "author_id")
	seriesID := ParseQueryIntPtr(c, "series_id")

	// Call service
	books, err := s.audiobookService.GetAudiobooks(c.Request.Context(), params.Limit, params.Offset, params.Search, authorID, seriesID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Enrich with author and series names
	enriched := s.audiobookService.EnrichAudiobooksWithNames(books)
	c.JSON(http.StatusOK, gin.H{"items": enriched, "count": len(enriched), "limit": params.Limit, "offset": params.Offset})
}

func (s *Server) listDuplicateAudiobooks(c *gin.Context) {
	result, err := s.audiobookService.GetDuplicateBooks(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"groups":          result.Groups,
		"group_count":     result.GroupCount,
		"duplicate_count": result.DuplicateCount,
	})
}

func (s *Server) listSoftDeletedAudiobooks(c *gin.Context) {
	params := ParsePaginationParams(c)
	olderThanDays := ParseQueryIntPtr(c, "older_than_days")

	books, err := s.audiobookService.GetSoftDeletedBooks(c.Request.Context(), params.Limit, params.Offset, olderThanDays)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items":  books,
		"count":  len(books),
		"total":  len(books),
		"limit":  params.Limit,
		"offset": params.Offset,
	})
}

func (s *Server) purgeSoftDeletedAudiobooks(c *gin.Context) {
	deleteFiles := c.Query("delete_files") == "true"
	olderThanStr := c.Query("older_than_days")

	var olderThanDays *int
	if olderThanStr != "" {
		if days, err := strconv.Atoi(olderThanStr); err == nil && days > 0 {
			olderThanDays = &days
		}
	}

	result, err := s.audiobookService.PurgeSoftDeletedBooks(c.Request.Context(), deleteFiles, olderThanDays)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (s *Server) runAutoPurgeSoftDeleted() {
	if config.AppConfig.PurgeSoftDeletedAfterDays <= 0 {
		return
	}
	if database.GlobalStore == nil {
		log.Printf("[DEBUG] Auto-purge skipped: database not initialized")
		return
	}

	days := config.AppConfig.PurgeSoftDeletedAfterDays
	result, err := s.audiobookService.PurgeSoftDeletedBooks(context.Background(), config.AppConfig.PurgeSoftDeletedDeleteFiles, &days)
	if err != nil {
		log.Printf("[WARN] Auto-purge failed: %v", err)
		return
	}

	if result.Attempted > 0 {
		log.Printf("[INFO] Auto-purge soft-deleted books: attempted=%d purged=%d files_deleted=%d errors=%d",
			result.Attempted, result.Purged, result.FilesDeleted, len(result.Errors))
		if len(result.Errors) > 0 {
			for _, e := range result.Errors {
				log.Printf("[WARN] Auto-purge error: %s", e)
			}
		}
	}
}

func (s *Server) restoreAudiobook(c *gin.Context) {
	id := c.Param("id")
	updated, err := s.audiobookService.RestoreAudiobook(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "audiobook restored",
		"book":    updated,
	})
}

func (s *Server) countAudiobooks(c *gin.Context) {
	count, err := s.audiobookService.CountAudiobooks(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"count": count})
}

func (s *Server) getAudiobook(c *gin.Context) {
	id := c.Param("id")
	book, err := s.audiobookService.GetAudiobook(c.Request.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Add author and series names to response
	type bookResponse struct {
		*database.Book
		AuthorName *string `json:"author_name,omitempty"`
		SeriesName *string `json:"series_name,omitempty"`
	}

	authorName, seriesName := resolveAuthorAndSeriesNames(book)
	resp := bookResponse{Book: book}
	if authorName != "" {
		resp.AuthorName = &authorName
	}
	if seriesName != "" {
		resp.SeriesName = &seriesName
	}

	c.JSON(http.StatusOK, resp)
}

func (s *Server) getAudiobookTags(c *gin.Context) {
	id := c.Param("id")
	resp, err := s.audiobookService.GetAudiobookTags(c.Request.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (s *Server) updateAudiobook(c *gin.Context) {
	id := c.Param("id")

	var payload map[string]any
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updatedBook, err := s.audiobookUpdateService.UpdateAudiobook(id, payload)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, updatedBook)
}

func (s *Server) deleteAudiobook(c *gin.Context) {
	id := c.Param("id")
	blockHash := c.Query("block_hash") == "true"
	softDelete := c.Query("soft_delete") == "true"

	opts := &DeleteAudiobookOptions{
		SoftDelete: softDelete,
		BlockHash:  blockHash,
	}

	result, err := s.audiobookService.DeleteAudiobook(c.Request.Context(), id, opts)
	if err != nil {
		if strings.Contains(err.Error(), "already soft deleted") {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (s *Server) batchUpdateAudiobooks(c *gin.Context) {
	var req BatchUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp := s.batchService.UpdateAudiobooks(&req)
	c.JSON(http.StatusOK, resp)
}

// ---- Work handlers ----

func (s *Server) listWorks(c *gin.Context) {
	resp, err := s.workService.ListWorks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (s *Server) createWork(c *gin.Context) {
	var work database.Work
	if err := c.ShouldBindJSON(&work); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	created, err := s.workService.CreateWork(&work)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, created)
}

func (s *Server) getWork(c *gin.Context) {
	id := c.Param("id")
	work, err := s.workService.GetWork(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, work)
}

func (s *Server) updateWork(c *gin.Context) {
	id := c.Param("id")
	var work database.Work
	if err := c.ShouldBindJSON(&work); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(work.Title) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title is required"})
		return
	}
	updated, err := s.workService.UpdateWork(id, &work)
	if err != nil {
		if err.Error() == "work not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, updated)
}

func (s *Server) deleteWork(c *gin.Context) {
	id := c.Param("id")
	if err := s.workService.DeleteWork(id); err != nil {
		if err.Error() == "work not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) listWorkBooks(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	id := c.Param("id")
	books, err := database.GlobalStore.GetBooksByWorkID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if books == nil {
		books = []database.Book{}
	}
	c.JSON(http.StatusOK, gin.H{"items": books, "count": len(books)})
}

func (s *Server) listAuthors(c *gin.Context) {
	resp, err := s.authorSeriesService.ListAuthors()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (s *Server) listSeries(c *gin.Context) {
	resp, err := s.authorSeriesService.ListSeries()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// getHomeDirectory returns the server user's home directory path.
func (s *Server) getHomeDirectory(c *gin.Context) {
	homeDir, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(homeDir) == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to determine home directory"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"path": homeDir})
}

func (s *Server) browseFilesystem(c *gin.Context) {
	path := c.Query("path")
	result, err := s.filesystemService.BrowseDirectory(path)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) createExclusion(c *gin.Context) {
	var req struct {
		Path string `json:"path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.filesystemService.CreateExclusion(req.Path); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "exclusion created"})
}

func (s *Server) removeExclusion(c *gin.Context) {
	var req struct {
		Path string `json:"path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.filesystemService.RemoveExclusion(req.Path); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) listImportPaths(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	folders, err := database.GlobalStore.GetAllImportPaths()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Ensure we never return null - always return empty array
	if folders == nil {
		folders = []database.ImportPath{}
	}

	c.JSON(http.StatusOK, gin.H{"importPaths": folders, "count": len(folders)})
}

func (s *Server) addImportPath(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	var req struct {
		Path    string `json:"path" binding:"required"`
		Name    string `json:"name" binding:"required"`
		Enabled *bool  `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	createdPath, err := s.importPathService.CreateImportPath(req.Path, req.Name)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	folder := createdPath
	if req.Enabled != nil && !*req.Enabled {
		folder.Enabled = false
		if err := database.GlobalStore.UpdateImportPath(folder.ID, folder); err != nil {
			// Non-fatal; return created folder anyway with note
			c.JSON(http.StatusCreated, gin.H{"importPath": folder, "warning": "created but could not update enabled flag"})
			return
		}
	}

	// Auto-scan the newly added folder if enabled and operation queue is available
	if folder.Enabled && operations.GlobalQueue != nil {
		opID := ulid.Make().String()
		folderPath := folder.Path
		op, err := database.GlobalStore.CreateOperation(opID, "scan", &folderPath)
		if err == nil {
			// Create scan operation function
			operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
				_ = progress.Log("info", fmt.Sprintf("Auto-scanning newly added folder: %s", folderPath), nil)

				// Check if folder exists
				if _, err := os.Stat(folderPath); os.IsNotExist(err) {
					return fmt.Errorf("folder does not exist: %s", folderPath)
				}

				// Scan directory for audiobook files (parallel)
				workers := config.AppConfig.ConcurrentScans
				if workers < 1 {
					workers = 4
				}
				books, err := scanner.ScanDirectoryParallel(folderPath, workers)
				if err != nil {
					return fmt.Errorf("failed to scan folder: %w", err)
				}

				_ = progress.Log("info", fmt.Sprintf("Found %d audiobook files", len(books)), nil)

				// Process the books to extract metadata (parallel)
				if len(books) > 0 {
					_ = progress.Log("info", fmt.Sprintf("Processing metadata for %d books using %d workers", len(books), workers), nil)
					if err := scanner.ProcessBooksParallel(ctx, books, workers, nil); err != nil {
						return fmt.Errorf("failed to process books: %w", err)
					}
					// Auto-organize if enabled
					if config.AppConfig.AutoOrganize && config.AppConfig.RootDir != "" {
						org := organizer.NewOrganizer(&config.AppConfig)
						organized := 0
						for _, b := range books {
							// Lookup DB book by file path
							dbBook, err := database.GlobalStore.GetBookByFilePath(b.FilePath)
							if err != nil || dbBook == nil {
								continue
							}
							newPath, err := org.OrganizeBook(dbBook)
							if err != nil {
								_ = progress.Log("warn", fmt.Sprintf("Organize failed for %s: %v", dbBook.Title, err), nil)
								continue
							}
							if newPath != dbBook.FilePath {
								dbBook.FilePath = newPath
								applyOrganizedFileMetadata(dbBook, newPath)
								if _, err := database.GlobalStore.UpdateBook(dbBook.ID, dbBook); err != nil {
									_ = progress.Log("warn", fmt.Sprintf("Failed to update path for %s: %v", dbBook.Title, err), nil)
								} else {
									organized++
								}
							}
						}
						_ = progress.Log("info", fmt.Sprintf("Auto-organize complete: %d organized", organized), nil)
					} else if config.AppConfig.AutoOrganize && config.AppConfig.RootDir == "" {
						_ = progress.Log("warn", "Auto-organize enabled but root_dir not set", nil)
					}
				}

				// Update book count for this import path
				folder.BookCount = len(books)
				now := time.Now()
				folder.LastScan = &now
				if err := database.GlobalStore.UpdateImportPath(folder.ID, folder); err != nil {
					_ = progress.Log("warn", fmt.Sprintf("Failed to update book count: %v", err), nil)
				}

				_ = progress.Log("info", fmt.Sprintf("Auto-scan completed. Total books: %d", len(books)), nil)
				return nil
			}

			// Enqueue the scan operation with normal priority
			_ = operations.GlobalQueue.Enqueue(op.ID, "scan", operations.PriorityNormal, operationFunc)

			c.JSON(http.StatusCreated, gin.H{"importPath": folder, "scan_operation_id": op.ID})
			return
		}
	}

	// Fallback: if enabled but queue unavailable OR operation creation failed, run synchronous scan
	if folder.Enabled && operations.GlobalQueue == nil {
		// Basic scan without progress reporter
		if _, err := os.Stat(folder.Path); err == nil {
			books, err := scanner.ScanDirectory(folder.Path)
			if err == nil {
				if len(books) > 0 {
					_ = scanner.ProcessBooks(books) // ignore individual processing errors (already logged internally)
					// Auto-organize if enabled
					if config.AppConfig.AutoOrganize && config.AppConfig.RootDir != "" {
						org := organizer.NewOrganizer(&config.AppConfig)
						for _, b := range books {
							dbBook, err := database.GlobalStore.GetBookByFilePath(b.FilePath)
							if err != nil || dbBook == nil {
								continue
							}
							newPath, err := org.OrganizeBook(dbBook)
							if err != nil {
								continue
							}
							if newPath != dbBook.FilePath {
								dbBook.FilePath = newPath
								applyOrganizedFileMetadata(dbBook, newPath)
								_, _ = database.GlobalStore.UpdateBook(dbBook.ID, dbBook)
							}
						}
					} else if config.AppConfig.AutoOrganize && config.AppConfig.RootDir == "" {
						log.Printf("auto-organize enabled but root_dir not set")
					}
				}
				folder.BookCount = len(books)
				now := time.Now()
				folder.LastScan = &now
				_ = database.GlobalStore.UpdateImportPath(folder.ID, folder)
			}
		}
	}

	c.JSON(http.StatusCreated, gin.H{"importPath": folder})
}

func (s *Server) removeImportPath(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid import path id"})
		return
	}
	if err := database.GlobalStore.DeleteImportPath(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) startScan(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	if operations.GlobalQueue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	var req struct {
		FolderPath  *string `json:"folder_path"`
		Priority    *int    `json:"priority"`
		ForceUpdate *bool   `json:"force_update"`
	}
	_ = c.ShouldBindJSON(&req)

	id := ulid.Make().String()
	op, err := database.GlobalStore.CreateOperation(id, "scan", req.FolderPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Determine priority (default to normal)
	priority := operations.PriorityNormal
	if req.Priority != nil {
		priority = *req.Priority
	}

	// Create operation function that delegates to service
	scanReq := &ScanRequest{
		FolderPath:  req.FolderPath,
		Priority:    req.Priority,
		ForceUpdate: req.ForceUpdate,
	}

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		return s.scanService.PerformScan(ctx, scanReq, progress)
	}

	// Enqueue the operation
	if err := operations.GlobalQueue.Enqueue(op.ID, "scan", priority, operationFunc); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, op)
}

func (s *Server) startOrganize(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	if operations.GlobalQueue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	var req struct {
		FolderPath *string `json:"folder_path"`
		Priority   *int    `json:"priority"`
	}
	_ = c.ShouldBindJSON(&req)

	id := ulid.Make().String()
	op, err := database.GlobalStore.CreateOperation(id, "organize", req.FolderPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Determine priority (default to normal)
	priority := operations.PriorityNormal
	if req.Priority != nil {
		priority = *req.Priority
	}

	// Create operation function that delegates to service
	organizeReq := &OrganizeRequest{
		FolderPath: req.FolderPath,
		Priority:   req.Priority,
	}

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		return s.organizeService.PerformOrganize(ctx, organizeReq, progress)
	}

	// Enqueue the operation
	if err := operations.GlobalQueue.Enqueue(op.ID, "organize", priority, operationFunc); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, op)
}

func (s *Server) getOperationStatus(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	id := c.Param("id")
	op, err := database.GlobalStore.GetOperationByID(id)
	if err != nil || op == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "operation not found"})
		return
	}
	c.JSON(http.StatusOK, op)
}

func (s *Server) cancelOperation(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	if operations.GlobalQueue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	id := c.Param("id")

	// Cancel via queue (which will update database)
	if err := operations.GlobalQueue.Cancel(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// listActiveOperations returns a snapshot of currently queued/running operations with basic progress
func (s *Server) listActiveOperations(c *gin.Context) {
	if operations.GlobalQueue == nil {
		c.JSON(http.StatusOK, gin.H{"operations": []gin.H{}})
		return
	}
	active := operations.GlobalQueue.ActiveOperations()
	results := make([]gin.H, 0, len(active))
	for _, a := range active {
		status := "queued"
		progress := 0
		total := 0
		message := ""
		if database.GlobalStore != nil {
			if op, err := database.GlobalStore.GetOperationByID(a.ID); err == nil && op != nil {
				status = op.Status
				progress = op.Progress
				total = op.Total
				message = op.Message
			}
		}
		results = append(results, gin.H{
			"id":       a.ID,
			"type":     a.Type,
			"status":   status,
			"progress": progress,
			"total":    total,
			"message":  message,
		})
	}
	c.JSON(http.StatusOK, gin.H{"operations": results})
}

func (s *Server) importFile(c *gin.Context) {
	var req ImportFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := s.importService.ImportFile(&req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, result)
}

func (s *Server) getSystemStatus(c *gin.Context) {
	status, err := s.systemService.CollectSystemStatus()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, status)
}

func (s *Server) getSystemLogs(c *gin.Context) {
	// For operation-specific logs, redirect to getOperationLogs
	if id := c.Query("operation_id"); id != "" {
		s.getOperationLogs(c)
		return
	}

	level := c.Query("level")
	params := ParsePaginationParams(c)

	logs, total, err := s.systemService.CollectSystemLogs(level, params.Search, params.Limit, params.Offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"logs":   logs,
		"limit":  params.Limit,
		"offset": params.Offset,
		"total":  total,
	})
}

func (s *Server) resetSystem(c *gin.Context) {
	// Reset database
	if err := database.GlobalStore.Reset(); err != nil {
		RespondWithInternalError(c, "failed to reset database: "+err.Error())
		return
	}

	// Reset config to defaults
	config.ResetToDefaults()

	// Reset library size cache
	resetLibrarySizeCache()

	RespondWithOK(c, gin.H{"message": "System reset successfully"})
}

func (s *Server) getConfig(c *gin.Context) {
	// Create a copy of config with masked secrets
	maskedConfig := config.AppConfig
	if maskedConfig.OpenAIAPIKey != "" {
		maskedConfig.OpenAIAPIKey = database.MaskSecret(maskedConfig.OpenAIAPIKey)
	}
	if maskedConfig.APIKeys.Goodreads != "" {
		maskedConfig.APIKeys.Goodreads = database.MaskSecret(maskedConfig.APIKeys.Goodreads)
	}
	c.JSON(http.StatusOK, gin.H{"config": maskedConfig})
}

func (s *Server) updateConfig(c *gin.Context) {
	var payload map[string]any
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := s.configUpdateService.ApplyUpdates(payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	maskedConfig := s.configUpdateService.MaskSecrets(config.AppConfig)
	c.JSON(http.StatusOK, maskedConfig)
}

// getOperationLogs returns logs for a given operation
func (s *Server) getOperationLogs(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	id := c.Param("id")
	logs, err := database.GlobalStore.GetOperationLogs(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Optional tail parameter for last N log lines
	if tailStr := c.Query("tail"); tailStr != "" {
		if n, convErr := strconv.Atoi(tailStr); convErr == nil && n > 0 && n < len(logs) {
			logs = logs[len(logs)-n:]
		}
	}
	c.JSON(http.StatusOK, gin.H{"items": logs, "count": len(logs)})
}

// handleEvents handles Server-Sent Events (SSE) for real-time updates
func (s *Server) handleEvents(c *gin.Context) {
	hub := realtime.GetGlobalHub()
	if hub == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "event hub not initialized"})
		return
	}
	hub.HandleSSE(c)
}

// createBackup creates a database backup
func (s *Server) createBackup(c *gin.Context) {
	var req struct {
		MaxBackups *int `json:"max_backups"`
	}
	_ = c.ShouldBindJSON(&req)

	backupConfig := backup.DefaultBackupConfig()
	if req.MaxBackups != nil {
		backupConfig.MaxBackups = *req.MaxBackups
	}

	// Get database path and type from app config
	dbPath := config.AppConfig.DatabasePath
	dbType := config.AppConfig.DatabaseType

	info, err := backup.CreateBackup(dbPath, dbType, backupConfig)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, info)
}

// listBackups lists all available backups
func (s *Server) listBackups(c *gin.Context) {
	backupConfig := backup.DefaultBackupConfig()

	backups, err := backup.ListBackups(backupConfig.BackupDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Ensure we never return null - always return empty array
	if backups == nil {
		backups = []backup.BackupInfo{}
	}

	c.JSON(http.StatusOK, gin.H{
		"backups": backups,
		"count":   len(backups),
	})
}

// restoreBackup restores from a backup file
func (s *Server) restoreBackup(c *gin.Context) {
	var req struct {
		BackupFilename string `json:"backup_filename" binding:"required"`
		TargetPath     string `json:"target_path"`
		Verify         bool   `json:"verify"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	backupConfig := backup.DefaultBackupConfig()
	backupPath := filepath.Join(backupConfig.BackupDir, req.BackupFilename)

	// Use current database path as target if not specified
	targetPath := req.TargetPath
	if targetPath == "" {
		targetPath = filepath.Dir(config.AppConfig.DatabasePath)
	}

	if err := backup.RestoreBackup(backupPath, targetPath, req.Verify); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "backup restored successfully",
		"target":  targetPath,
	})
}

// deleteBackup deletes a backup file
func (s *Server) deleteBackup(c *gin.Context) {
	filename := c.Param("filename")
	if filename == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "filename required"})
		return
	}

	backupConfig := backup.DefaultBackupConfig()
	backupPath := filepath.Join(backupConfig.BackupDir, filename)

	if err := backup.DeleteBackup(backupPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "backup deleted successfully"})
}

// batchUpdateMetadata handles batch metadata updates with validation
func (s *Server) batchUpdateMetadata(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	var req struct {
		Updates  []metadata.MetadataUpdate `json:"updates" binding:"required"`
		Validate bool                      `json:"validate"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	errors, successCount := metadata.BatchUpdateMetadata(req.Updates, database.GlobalStore, req.Validate)

	response := gin.H{
		"success_count": successCount,
		"total_count":   len(req.Updates),
	}

	if len(errors) > 0 {
		errorMessages := make([]string, len(errors))
		for i, err := range errors {
			errorMessages[i] = err.Error()
		}
		response["errors"] = errorMessages
		c.JSON(http.StatusPartialContent, response)
	} else {
		c.JSON(http.StatusOK, response)
	}
}

// validateMetadata validates metadata updates without applying them
func (s *Server) validateMetadata(c *gin.Context) {
	var req struct {
		Updates map[string]any `json:"updates" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rules := metadata.DefaultValidationRules()
	errors := metadata.ValidateMetadata(req.Updates, rules)

	if len(errors) > 0 {
		errorMessages := make([]string, len(errors))
		for i, err := range errors {
			errorMessages[i] = err.Error()
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"valid":  false,
			"errors": errorMessages,
		})
	} else {
		c.JSON(http.StatusOK, gin.H{
			"valid":   true,
			"message": "metadata is valid",
		})
	}
}

// exportMetadata exports all audiobook metadata
func (s *Server) exportMetadata(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Get all books
	books, err := database.GlobalStore.GetAllBooks(0, 0) // No limit/offset
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Export metadata
	exportData, err := metadata.ExportMetadata(books)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, exportData)
}

// importMetadata imports audiobook metadata
func (s *Server) importMetadata(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	var req struct {
		Data     map[string]any `json:"data" binding:"required"`
		Validate bool                   `json:"validate"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	importCount, errors := metadata.ImportMetadata(req.Data, database.GlobalStore, req.Validate)

	response := gin.H{
		"import_count": importCount,
	}

	if len(errors) > 0 {
		errorMessages := make([]string, len(errors))
		for i, err := range errors {
			errorMessages[i] = err.Error()
		}
		response["errors"] = errorMessages
		c.JSON(http.StatusPartialContent, response)
	} else {
		c.JSON(http.StatusOK, response)
	}
}

// searchMetadata searches external metadata sources
func (s *Server) searchMetadata(c *gin.Context) {
	title := c.Query("title")
	author := c.Query("author")

	if title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title parameter required"})
		return
	}

	// Use Open Library for now
	client := metadata.NewOpenLibraryClient()

	var results []metadata.BookMetadata
	var err error

	if author != "" {
		results, err = client.SearchByTitleAndAuthor(title, author)
	} else {
		results, err = client.SearchByTitle(title)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("metadata search failed: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"results": results,
		"source":  "Open Library",
	})
}

// stripChapterFromTitle removes chapter/book numbers from titles to improve search results
// Examples: "The Odyssey: Book 01" -> "The Odyssey", "Harry Potter - Chapter 5" -> "Harry Potter"
func stripChapterFromTitle(title string) string {
	// Common patterns for chapters/books
	patterns := []string{
		`: Book \d+`,
		`: Chapter \d+`,
		` - Book \d+`,
		` - Chapter \d+`,
		`, Book \d+`,
		`, Chapter \d+`,
		`\(Book \d+\)`,
		`\(Chapter \d+\)`,
		` Book \d+$`,
		` Chapter \d+$`,
	}

	cleaned := title
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		cleaned = re.ReplaceAllString(cleaned, "")
	}

	return strings.TrimSpace(cleaned)
}

// fetchAudiobookMetadata fetches and applies metadata to an audiobook
func (s *Server) fetchAudiobookMetadata(c *gin.Context) {
	id := c.Param("id")

	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	resp, err := s.metadataFetchService.FetchMetadataForBook(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": resp.Message,
		"book":    resp.Book,
		"source":  resp.Source,
	})
}

type bulkFetchMetadataRequest struct {
	BookIDs     []string `json:"book_ids" binding:"required"`
	OnlyMissing *bool    `json:"only_missing,omitempty"`
}

type bulkFetchMetadataResult struct {
	BookID        string   `json:"book_id"`
	Status        string   `json:"status"`
	Message       string   `json:"message,omitempty"`
	AppliedFields []string `json:"applied_fields,omitempty"`
	FetchedFields []string `json:"fetched_fields,omitempty"`
}

// bulkFetchMetadata fetches external metadata for multiple audiobooks and applies
// fields only when they are missing and not manually overridden or locked.
func (s *Server) bulkFetchMetadata(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	var req bulkFetchMetadataRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.BookIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "book_ids is required"})
		return
	}

	onlyMissing := true
	if req.OnlyMissing != nil {
		onlyMissing = *req.OnlyMissing
	}

	client := metadata.NewOpenLibraryClient()
	results := make([]bulkFetchMetadataResult, 0, len(req.BookIDs))
	updatedCount := 0

	for _, bookID := range req.BookIDs {
		result := bulkFetchMetadataResult{
			BookID: bookID,
			Status: "skipped",
		}

		book, err := database.GlobalStore.GetBookByID(bookID)
		if err != nil || book == nil {
			result.Status = "not_found"
			result.Message = "audiobook not found"
			results = append(results, result)
			continue
		}

		if strings.TrimSpace(book.Title) == "" {
			result.Message = "missing title"
			results = append(results, result)
			continue
		}

		state, err := loadMetadataState(bookID)
		if err != nil {
			result.Status = "error"
			result.Message = "failed to load metadata state"
			results = append(results, result)
			continue
		}
		if state == nil {
			state = map[string]metadataFieldState{}
		}

		authorName := ""
		if book.Author != nil {
			authorName = book.Author.Name
		} else if book.AuthorID != nil {
			if author, err := database.GlobalStore.GetAuthorByID(*book.AuthorID); err == nil && author != nil {
				authorName = author.Name
			}
		}

		var metaResults []metadata.BookMetadata
		if authorName != "" {
			metaResults, err = client.SearchByTitleAndAuthor(book.Title, authorName)
		} else {
			metaResults, err = client.SearchByTitle(book.Title)
		}
		if err != nil || len(metaResults) == 0 {
			result.Status = "not_found"
			result.Message = "no metadata found"
			results = append(results, result)
			continue
		}

		meta := metaResults[0]
		fetchedValues := map[string]any{}
		appliedFields := []string{}
		fetchedFields := []string{}

		addFetched := func(field string, value any) {
			fetchedValues[field] = value
			fetchedFields = append(fetchedFields, field)
		}

		shouldApply := func(field string, hasValue bool) bool {
			entry := state[field]
			if entry.OverrideLocked || entry.OverrideValue != nil {
				return false
			}
			if onlyMissing && hasValue {
				return false
			}
			return true
		}

		hasBookValue := func(field string) bool {
			switch field {
			case "title":
				return strings.TrimSpace(book.Title) != ""
			case "author_name":
				return book.AuthorID != nil || book.Author != nil
			case "publisher":
				return book.Publisher != nil && strings.TrimSpace(*book.Publisher) != ""
			case "language":
				return book.Language != nil && strings.TrimSpace(*book.Language) != ""
			case "audiobook_release_year":
				return book.AudiobookReleaseYear != nil && *book.AudiobookReleaseYear != 0
			case "isbn10":
				return book.ISBN10 != nil && strings.TrimSpace(*book.ISBN10) != ""
			case "isbn13":
				return book.ISBN13 != nil && strings.TrimSpace(*book.ISBN13) != ""
			default:
				return false
			}
		}

		didUpdate := false

		if meta.Title != "" {
			addFetched("title", meta.Title)
			if shouldApply("title", hasBookValue("title")) {
				book.Title = meta.Title
				appliedFields = append(appliedFields, "title")
				didUpdate = true
			}
		}

		if meta.Author != "" {
			addFetched("author_name", meta.Author)
			if shouldApply("author_name", hasBookValue("author_name")) {
				author, err := database.GlobalStore.GetAuthorByName(meta.Author)
				if err != nil {
					result.Status = "error"
					result.Message = "failed to resolve author"
					results = append(results, result)
					continue
				}
				if author == nil {
					author, err = database.GlobalStore.CreateAuthor(meta.Author)
					if err != nil {
						result.Status = "error"
						result.Message = "failed to create author"
						results = append(results, result)
						continue
					}
				}
				book.AuthorID = &author.ID
				appliedFields = append(appliedFields, "author_name")
				didUpdate = true
			}
		}

		if meta.Publisher != "" {
			addFetched("publisher", meta.Publisher)
			if shouldApply("publisher", hasBookValue("publisher")) {
				book.Publisher = stringPtr(meta.Publisher)
				appliedFields = append(appliedFields, "publisher")
				didUpdate = true
			}
		}

		if meta.Language != "" {
			addFetched("language", meta.Language)
			if shouldApply("language", hasBookValue("language")) {
				book.Language = stringPtr(meta.Language)
				appliedFields = append(appliedFields, "language")
				didUpdate = true
			}
		}

		if meta.PublishYear != 0 {
			addFetched("audiobook_release_year", meta.PublishYear)
			if shouldApply("audiobook_release_year", hasBookValue("audiobook_release_year")) {
				year := meta.PublishYear
				book.AudiobookReleaseYear = &year
				appliedFields = append(appliedFields, "audiobook_release_year")
				didUpdate = true
			}
		}

		if meta.ISBN != "" {
			if len(meta.ISBN) == 10 {
				addFetched("isbn10", meta.ISBN)
				if shouldApply("isbn10", hasBookValue("isbn10")) {
					book.ISBN10 = stringPtr(meta.ISBN)
					appliedFields = append(appliedFields, "isbn10")
					didUpdate = true
				}
			} else {
				addFetched("isbn13", meta.ISBN)
				if shouldApply("isbn13", hasBookValue("isbn13")) {
					book.ISBN13 = stringPtr(meta.ISBN)
					appliedFields = append(appliedFields, "isbn13")
					didUpdate = true
				}
			}
		}

		if len(fetchedValues) > 0 {
			if err := updateFetchedMetadataState(bookID, fetchedValues); err != nil {
				log.Printf("[WARN] bulkFetchMetadata: failed to persist fetched metadata state for %s: %v", bookID, err)
			}
		}

		if didUpdate {
			if _, err := database.GlobalStore.UpdateBook(bookID, book); err != nil {
				result.Status = "error"
				result.Message = fmt.Sprintf("failed to update book: %v", err)
				results = append(results, result)
				continue
			}
			updatedCount++
			result.Status = "updated"
		} else if len(fetchedValues) > 0 {
			result.Status = "fetched"
		}

		result.AppliedFields = appliedFields
		result.FetchedFields = fetchedFields
		results = append(results, result)
	}

	c.JSON(http.StatusOK, gin.H{
		"updated_count": updatedCount,
		"total_count":   len(req.BookIDs),
		"results":       results,
		"source":        "Open Library",
	})
}

// Version Management Handlers

// listAudiobookVersions lists all versions of an audiobook
func (s *Server) listAudiobookVersions(c *gin.Context) {
	id := c.Param("id")

	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	book, err := database.GlobalStore.GetBookByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
		return
	}

	if book.VersionGroupID == nil {
		c.JSON(http.StatusOK, gin.H{"versions": []any{book}})
		return
	}

	books, err := database.GlobalStore.GetBooksByVersionGroup(*book.VersionGroupID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch versions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"versions": books})
}

// linkAudiobookVersion links an audiobook as another version
func (s *Server) linkAudiobookVersion(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		OtherID string `json:"other_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	book1, err := database.GlobalStore.GetBookByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
		return
	}

	book2, err := database.GlobalStore.GetBookByID(req.OtherID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "other audiobook not found"})
		return
	}

	versionGroupID := ""
	if book1.VersionGroupID != nil {
		versionGroupID = *book1.VersionGroupID
	} else if book2.VersionGroupID != nil {
		versionGroupID = *book2.VersionGroupID
	} else {
		versionGroupID = ulid.Make().String()
	}

	book1.VersionGroupID = &versionGroupID
	book2.VersionGroupID = &versionGroupID

	if _, err := database.GlobalStore.UpdateBook(id, book1); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update audiobook"})
		return
	}

	if _, err := database.GlobalStore.UpdateBook(req.OtherID, book2); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update other audiobook"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"version_group_id": versionGroupID})
}

// setAudiobookPrimary sets an audiobook as the primary version
func (s *Server) setAudiobookPrimary(c *gin.Context) {
	id := c.Param("id")

	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	book, err := database.GlobalStore.GetBookByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
		return
	}

	if book.VersionGroupID == nil {
		primaryFlag := true
		book.IsPrimaryVersion = &primaryFlag
		if _, err := database.GlobalStore.UpdateBook(id, book); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update audiobook"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "audiobook set as primary"})
		return
	}

	books, err := database.GlobalStore.GetBooksByVersionGroup(*book.VersionGroupID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch versions"})
		return
	}

	for i := range books {
		primaryFlag := books[i].ID == id
		books[i].IsPrimaryVersion = &primaryFlag
		if _, err := database.GlobalStore.UpdateBook(books[i].ID, &books[i]); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update version"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "audiobook set as primary"})
}

// getVersionGroup gets all audiobooks in a version group
func (s *Server) getVersionGroup(c *gin.Context) {
	groupID := c.Param("id")

	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	books, err := database.GlobalStore.GetBooksByVersionGroup(groupID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch version group"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"audiobooks": books})
}

// parseFilenameWithAI uses OpenAI to parse a filename into structured metadata
func (s *Server) parseFilenameWithAI(c *gin.Context) {
	var req struct {
		Filename string `json:"filename" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "filename is required"})
		return
	}

	// Create AI parser
	parser := ai.NewOpenAIParser(config.AppConfig.OpenAIAPIKey, config.AppConfig.EnableAIParsing)
	if !parser.IsEnabled() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "AI parsing is not enabled or API key not configured"})
		return
	}

	// Parse filename
	metadata, err := parser.ParseFilename(c.Request.Context(), req.Filename)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to parse filename: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"metadata": metadata})
}

// testAIConnection tests the OpenAI API connection
func (s *Server) testAIConnection(c *gin.Context) {
	// Parse request body for API key (allows testing without saving)
	var req struct {
		APIKey string `json:"api_key"`
	}

	// Try to get API key from request body first, fall back to config
	apiKey := config.AppConfig.OpenAIAPIKey
	if err := c.ShouldBindJSON(&req); err == nil && req.APIKey != "" {
		apiKey = req.APIKey
	}

	if apiKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "API key not provided", "success": false})
		return
	}

	// Create parser with the provided/configured API key
	parser := ai.NewOpenAIParser(apiKey, true)
	if err := parser.TestConnection(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("connection test failed: %v", err), "success": false})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "OpenAI connection successful"})
}

// parseAudiobookWithAI parses an audiobook's filename with AI and updates its metadata
func (s *Server) parseAudiobookWithAI(c *gin.Context) {
	id := c.Param("id")

	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Get the book
	book, err := database.GlobalStore.GetBookByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
		return
	}

	// Create AI parser
	parser := ai.NewOpenAIParser(config.AppConfig.OpenAIAPIKey, config.AppConfig.EnableAIParsing)
	if !parser.IsEnabled() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "AI parsing is not enabled or API key not configured"})
		return
	}

	// Extract filename from path
	filename := filepath.Base(book.FilePath)

	// Parse with AI
	metadata, err := parser.ParseFilename(c.Request.Context(), filename)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to parse filename: %v", err)})
		return
	}

	// Update book with parsed metadata
	if metadata.Title != "" {
		book.Title = metadata.Title
	}
	if metadata.Narrator != "" {
		book.Narrator = &metadata.Narrator
	}
	if metadata.Publisher != "" {
		book.Publisher = &metadata.Publisher
	}
	if metadata.Year > 0 {
		book.PrintYear = &metadata.Year
	}

	// Handle author
	if metadata.Author != "" {
		author, err := database.GlobalStore.GetAuthorByName(metadata.Author)
		if err != nil || author == nil {
			// Create new author
			author, err = database.GlobalStore.CreateAuthor(metadata.Author)
			if err == nil && author != nil {
				book.AuthorID = &author.ID
			}
		} else {
			book.AuthorID = &author.ID
		}
	}

	// Handle series
	if metadata.Series != "" {
		series, err := database.GlobalStore.GetSeriesByName(metadata.Series, book.AuthorID)
		if err != nil || series == nil {
			// Create new series
			series, err = database.GlobalStore.CreateSeries(metadata.Series, book.AuthorID)
			if err == nil && series != nil {
				book.SeriesID = &series.ID
			}
		} else {
			book.SeriesID = &series.ID
		}

		if metadata.SeriesNum > 0 {
			book.SeriesSequence = &metadata.SeriesNum
		}
	}

	// Update in database
	updatedBook, err := database.GlobalStore.UpdateBook(id, book)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update audiobook"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "audiobook updated with AI-parsed metadata",
		"book":       updatedBook,
		"confidence": metadata.Confidence,
	})
}

// getDashboard returns dashboard statistics with size and format distributions
func (s *Server) getDashboard(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Get all books
	allBooks, err := database.GlobalStore.GetAllBooks(100000, 0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve books"})
		return
	}

	// Calculate size distribution
	sizeDistribution := map[string]int{
		"0-100MB":   0,
		"100-500MB": 0,
		"500MB-1GB": 0,
		"1GB+":      0,
	}

	// Calculate format distribution and total size
	formatDistribution := make(map[string]int)
	var totalSize int64 = 0

	for _, book := range allBooks {
		// Size distribution
		if book.FileSize != nil && *book.FileSize > 0 {
			totalSize += *book.FileSize
			sizeMB := float64(*book.FileSize) / (1024 * 1024)
			sizeGB := sizeMB / 1024

			if sizeMB < 100 {
				sizeDistribution["0-100MB"]++
			} else if sizeMB < 500 {
				sizeDistribution["100-500MB"]++
			} else if sizeGB < 1 {
				sizeDistribution["500MB-1GB"]++
			} else {
				sizeDistribution["1GB+"]++
			}
		}

		// Format distribution
		ext := strings.ToLower(filepath.Ext(book.FilePath))
		if ext != "" {
			ext = strings.TrimPrefix(ext, ".")
			formatDistribution[ext]++
		}
	}

	// Get recent operations
	recentOps, err := database.GlobalStore.GetRecentOperations(5)
	if err != nil {
		recentOps = []database.Operation{}
	}

	c.JSON(http.StatusOK, gin.H{
		"sizeDistribution":   sizeDistribution,
		"formatDistribution": formatDistribution,
		"recentOperations":   recentOps,
		"totalSize":          totalSize,
		"totalBooks":         len(allBooks),
	})
}

// getMetadataFields returns available metadata fields with their types and validation rules
func (s *Server) getMetadataFields(c *gin.Context) {
	fields := []map[string]any{
		{
			"name":        "title",
			"type":        "string",
			"required":    true,
			"maxLength":   500,
			"description": "Book title",
		},
		{
			"name":        "author",
			"type":        "string",
			"required":    false,
			"description": "Author name",
		},
		{
			"name":        "narrator",
			"type":        "string",
			"required":    false,
			"description": "Narrator name",
		},
		{
			"name":        "publisher",
			"type":        "string",
			"required":    false,
			"description": "Publisher name",
		},
		{
			"name":        "publishDate",
			"type":        "integer",
			"required":    false,
			"min":         1000,
			"max":         9999,
			"description": "Publication year",
		},
		{
			"name":        "series",
			"type":        "string",
			"required":    false,
			"description": "Series name",
		},
		{
			"name":        "language",
			"type":        "string",
			"required":    false,
			"pattern":     "^[a-z]{2}$",
			"description": "ISO 639-1 language code (e.g., 'en', 'es')",
		},
		{
			"name":        "isbn10",
			"type":        "string",
			"required":    false,
			"pattern":     "^[0-9]{9}[0-9X]$",
			"description": "ISBN-10",
		},
		{
			"name":        "isbn13",
			"type":        "string",
			"required":    false,
			"pattern":     "^97[89][0-9]{10}$",
			"description": "ISBN-13",
		},
		{
			"name":        "series_sequence",
			"type":        "integer",
			"required":    false,
			"min":         1,
			"description": "Position in series",
		},
	}

	c.JSON(http.StatusOK, gin.H{
		"fields": fields,
	})
}

// listWork returns all work items (audiobooks grouped by work entity)
func (s *Server) listWork(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Get all works
	works, err := database.GlobalStore.GetAllWorks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve works"})
		return
	}

	// For each work, get associated books
	items := make([]map[string]any, 0, len(works))
	for _, work := range works {
		books, err := database.GlobalStore.GetBooksByWorkID(work.ID)
		if err != nil {
			books = []database.Book{}
		}

		items = append(items, map[string]any{
			"id":         work.ID,
			"title":      work.Title,
			"author_id":  work.AuthorID,
			"book_count": len(books),
			"books":      books,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"items": items,
		"total": len(items),
	})
}

// getWorkStats returns statistics about work items
func (s *Server) getWorkStats(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	works, err := database.GlobalStore.GetAllWorks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve works"})
		return
	}

	totalWorks := len(works)
	totalBooks := 0
	worksWithMultipleEditions := 0

	for _, work := range works {
		books, err := database.GlobalStore.GetBooksByWorkID(work.ID)
		if err != nil {
			continue
		}
		bookCount := len(books)
		totalBooks += bookCount
		if bookCount > 1 {
			worksWithMultipleEditions++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"total_works":                  totalWorks,
		"total_books":                  totalBooks,
		"works_with_multiple_editions": worksWithMultipleEditions,
		"average_editions_per_work":    float64(totalBooks) / float64(max(totalWorks, 1)),
	})
}

// listBlockedHashes returns all blocked hashes
func (s *Server) listBlockedHashes(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	hashes, err := database.GlobalStore.GetAllBlockedHashes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to get blocked hashes: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items": hashes,
		"total": len(hashes),
	})
}

// addBlockedHash adds a hash to the blocklist
func (s *Server) addBlockedHash(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	var req struct {
		Hash   string `json:"hash" binding:"required"`
		Reason string `json:"reason" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate hash format (should be 64 character hex string for SHA256)
	if len(req.Hash) != 64 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "hash must be 64 characters (SHA256)"})
		return
	}

	err := database.GlobalStore.AddBlockedHash(req.Hash, req.Reason)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to add blocked hash: %v", err)})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "hash blocked successfully",
		"hash":    req.Hash,
		"reason":  req.Reason,
	})
}

// removeBlockedHash removes a hash from the blocklist
func (s *Server) removeBlockedHash(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	hash := c.Param("hash")
	if hash == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "hash parameter required"})
		return
	}

	err := database.GlobalStore.RemoveBlockedHash(hash)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to remove blocked hash: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "hash unblocked successfully",
		"hash":    hash,
	})
}

// GetDefaultServerConfig returns default server configuration
func GetDefaultServerConfig() ServerConfig {
	return ServerConfig{
		Port:         "8080",
		Host:         "localhost",
		ReadTimeout:  15 * time.Second,  // Allow slow clients without stalling forever
		WriteTimeout: 0,                 // Disable write timeout so SSE streams stay open
		IdleTimeout:  120 * time.Second, // 2 minute idle timeout
	}
}
