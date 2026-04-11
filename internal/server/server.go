// file: internal/server/server.go
// version: 1.155.0
// guid: 4c5d6e7f-8a9b-0c1d-2e3f-4a5b6c7d8e9f

package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/ai"
	"github.com/jdfalk/audiobook-organizer/internal/backup"
	"github.com/jdfalk/audiobook-organizer/internal/cache"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/fileops"
	"github.com/jdfalk/audiobook-organizer/internal/itunes"
	"github.com/jdfalk/audiobook-organizer/internal/logger"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
	"github.com/jdfalk/audiobook-organizer/internal/metrics"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	"github.com/jdfalk/audiobook-organizer/internal/organizer"
	"github.com/jdfalk/audiobook-organizer/internal/realtime"
	"github.com/jdfalk/audiobook-organizer/internal/scanner"
	servermiddleware "github.com/jdfalk/audiobook-organizer/internal/server/middleware"
	"github.com/jdfalk/audiobook-organizer/internal/transcode"
	"github.com/jdfalk/audiobook-organizer/internal/updater"
	"github.com/jdfalk/audiobook-organizer/internal/watcher"
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

const librarySizeCacheTTL = 5 * time.Minute

// appVersion is set at startup via SetVersion(), injected from main.version
var appVersion = "dev"

// SetVersion sets the application version string.
func SetVersion(v string) {
	appVersion = v
}

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
	ParseAudiobook(ctx context.Context, abCtx ai.AudiobookContext) (*ai.ParsedMetadata, error)
	ParseCoverArt(ctx context.Context, imageBytes []byte, mimeType string) (*ai.ParsedMetadata, error)
	ReviewAuthorDuplicates(ctx context.Context, groups []ai.AuthorDedupInput) ([]ai.AuthorDedupSuggestion, error)
	DiscoverAuthorDuplicates(ctx context.Context, inputs []ai.AuthorDiscoveryInput) ([]ai.AuthorDiscoverySuggestion, error)
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

// enrichedBookResponse wraps a Book with resolved names for JSON responses.
type enrichedBookResponse struct {
	*database.Book
	AuthorName *string         `json:"author_name,omitempty"`
	SeriesName *string         `json:"series_name,omitempty"`
	Authors    []authorEntry   `json:"authors,omitempty"`
	Narrators  []narratorEntry `json:"narrators,omitempty"`
	FileExists *bool           `json:"file_exists,omitempty"`
}

type authorEntry struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Position int    `json:"position"`
}

type narratorEntry struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Position int    `json:"position"`
}

// enrichBookForResponse resolves author, series, and narrator names from join
// tables so the JSON response contains all the fields the frontend expects.
func enrichBookForResponse(book *database.Book) enrichedBookResponse {
	authorName, seriesName := resolveAuthorAndSeriesNames(book)
	resp := enrichedBookResponse{Book: book}
	if authorName != "" {
		resp.AuthorName = &authorName
	}
	if seriesName != "" {
		resp.SeriesName = &seriesName
	}

	// Check if the book's file exists on disk (single-file books only).
	if book.FilePath != "" {
		_, statErr := os.Stat(book.FilePath)
		exists := statErr == nil
		resp.FileExists = &exists
	}

	if database.GlobalStore != nil {
		if bookAuthors, err := database.GlobalStore.GetBookAuthors(book.ID); err == nil && len(bookAuthors) > 0 {
			for _, ba := range bookAuthors {
				if author, err := database.GlobalStore.GetAuthorByID(ba.AuthorID); err == nil && author != nil {
					resp.Authors = append(resp.Authors, authorEntry{
						ID: author.ID, Name: author.Name, Role: ba.Role, Position: ba.Position,
					})
				}
			}
			if resp.AuthorName == nil && len(resp.Authors) > 0 {
				names := make([]string, len(resp.Authors))
				for i, a := range resp.Authors {
					names[i] = a.Name
				}
				combined := strings.Join(names, " & ")
				resp.AuthorName = &combined
			}
		}

		if bookNarrators, err := database.GlobalStore.GetBookNarrators(book.ID); err == nil && len(bookNarrators) > 0 {
			for _, bn := range bookNarrators {
				if narrator, err := database.GlobalStore.GetNarratorByID(bn.NarratorID); err == nil && narrator != nil {
					resp.Narrators = append(resp.Narrators, narratorEntry{
						ID: narrator.ID, Name: narrator.Name, Role: bn.Role, Position: bn.Position,
					})
				}
			}
			if (book.Narrator == nil || *book.Narrator == "") && len(resp.Narrators) > 0 {
				names := make([]string, len(resp.Narrators))
				for i, n := range resp.Narrators {
					names[i] = n.Name
				}
				combined := strings.Join(names, " & ")
				book.Narrator = &combined
			}
		}
	}

	return resp
}

func buildComparisonValuesFromMetadata(comparisonMeta *metadata.Metadata) map[string]any {
	if comparisonMeta == nil {
		return nil
	}

	compMap := map[string]any{
		"title":           nonEmpty(comparisonMeta.Title),
		"author_name":     nonEmpty(comparisonMeta.Artist),
		"narrator":        nonEmpty(comparisonMeta.Narrator),
		"series_name":     nonEmpty(comparisonMeta.Series),
		"publisher":       nonEmpty(comparisonMeta.Publisher),
		"language":        nonEmpty(comparisonMeta.Language),
		"isbn10":          nonEmpty(comparisonMeta.ISBN10),
		"isbn13":          nonEmpty(comparisonMeta.ISBN13),
		"genre":           nonEmpty(comparisonMeta.Genre),
		"album":           nonEmpty(comparisonMeta.Album),
		"asin":            nonEmpty(comparisonMeta.ASIN),
		"edition":         nonEmpty(comparisonMeta.Edition),
		"print_year":      nonEmpty(comparisonMeta.PrintYear),
		"description":     nonEmpty(comparisonMeta.Comments),
		"book_id":         nonEmpty(comparisonMeta.BookOrganizerID),
		"open_library_id": nonEmpty(comparisonMeta.OpenLibraryID),
		"hardcover_id":    nonEmpty(comparisonMeta.HardcoverID),
		"google_books_id": nonEmpty(comparisonMeta.GoogleBooksID),
	}
	if comparisonMeta.Year > 0 {
		compMap["audiobook_release_year"] = comparisonMeta.Year
	}
	if comparisonMeta.SeriesIndex > 0 {
		compMap["series_index"] = comparisonMeta.SeriesIndex
	}
	return compMap
}

func buildComparisonValuesFromBook(book *database.Book, authorName, seriesName string) map[string]any {
	if book == nil {
		return nil
	}

	compMap := map[string]any{
		"title":           nonEmpty(book.Title),
		"author_name":     nonEmpty(authorName),
		"narrator":        nonEmpty(ptrStr(book.Narrator)),
		"series_name":     nonEmpty(seriesName),
		"publisher":       nonEmpty(ptrStr(book.Publisher)),
		"language":        nonEmpty(ptrStr(book.Language)),
		"isbn10":          nonEmpty(ptrStr(book.ISBN10)),
		"isbn13":          nonEmpty(ptrStr(book.ISBN13)),
		"genre":           nonEmpty(ptrStr(book.Genre)),
		"album":           nonEmpty(book.Title),
		"asin":            nonEmpty(ptrStr(book.ASIN)),
		"edition":         nonEmpty(ptrStr(book.Edition)),
		"description":     nonEmpty(ptrStr(book.Description)),
		"book_id":         nonEmpty(book.ID),
		"open_library_id": nonEmpty(ptrStr(book.OpenLibraryID)),
		"hardcover_id":    nonEmpty(ptrStr(book.HardcoverID)),
		"google_books_id": nonEmpty(ptrStr(book.GoogleBooksID)),
	}
	if book.AudiobookReleaseYear != nil && *book.AudiobookReleaseYear > 0 {
		compMap["audiobook_release_year"] = *book.AudiobookReleaseYear
	}
	if book.SeriesSequence != nil && *book.SeriesSequence > 0 {
		compMap["series_index"] = *book.SeriesSequence
	}
	if book.PrintYear != nil && *book.PrintYear > 0 {
		compMap["print_year"] = *book.PrintYear
	}
	return compMap
}

// buildComparisonValuesFromActivityLog reconstructs a "before" tag snapshot by
// querying the activity log for metadata_apply entries for the given book
// recorded within a ±5 second window of ts. For each field found, the
// old_value (i.e. the value BEFORE that operation) is used as the comparison
// value. This is the fallback when GetBookAtVersion is unavailable (SQLite) or
// when the exact version key is not present in PebbleDB.
func buildComparisonValuesFromActivityLog(as *ActivityService, bookID string, ts time.Time) map[string]any {
	window := 5 * time.Second
	since := ts.Add(-window)
	until := ts.Add(window)

	entries, _, err := as.Query(database.ActivityFilter{
		BookID: bookID,
		Type:   "metadata_apply",
		Since:  &since,
		Until:  &until,
		Limit:  200,
	})
	if err != nil || len(entries) == 0 {
		return nil
	}

	compMap := map[string]any{}
	for _, e := range entries {
		if e.Details == nil {
			continue
		}
		field, _ := e.Details["field"].(string)
		if field == "" {
			continue
		}
		// old_value is the state BEFORE this operation — that's what we want
		// to show as the "snapshot" comparison row.
		if oldVal, ok := e.Details["old_value"]; ok && oldVal != nil {
			if s, ok := oldVal.(string); ok && s != "" {
				compMap[field] = s
			} else if oldVal != nil {
				compMap[field] = oldVal
			}
		}
	}
	if len(compMap) == 0 {
		return nil
	}
	return compMap
}

func buildMetadataProvenance(book *database.Book, state map[string]metadataFieldState, meta metadata.Metadata, authorName, seriesName string, comparisonValues map[string]any) map[string]database.MetadataProvenanceEntry {
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

		entry := database.MetadataProvenanceEntry{
			FileValue:       fileValue,
			FetchedValue:    entryState.FetchedValue,
			StoredValue:     storedValue,
			OverrideValue:   entryState.OverrideValue,
			OverrideLocked:  entryState.OverrideLocked,
			EffectiveValue:  effectiveValue,
			EffectiveSource: effectiveSource,
			UpdatedAt:       updatedAt,
		}

		if comparisonValues != nil {
			if cv, ok := comparisonValues[field]; ok {
				entry.ComparisonValue = cv
			}
		}

		provenance[field] = entry
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
	addEntry("genre", meta.Genre, stringVal(book.Genre))
	addEntry("album", meta.Album, book.Title)
	addEntry("asin", nonEmpty(meta.ASIN), stringVal(book.ASIN))
	var seriesIdx any
	if meta.SeriesIndex > 0 {
		seriesIdx = meta.SeriesIndex
	}
	addEntry("series_index", seriesIdx, intVal(book.SeriesSequence))
	addEntry("print_year", nonEmpty(meta.PrintYear), intVal(book.PrintYear))
	addEntry("edition", nonEmpty(meta.Edition), stringVal(book.Edition))
	addEntry("description", nonEmpty(meta.Comments), stringVal(book.Description))
	addEntry("book_id", nonEmpty(meta.BookOrganizerID), book.ID)
	addEntry("open_library_id", nonEmpty(meta.OpenLibraryID), stringVal(book.OpenLibraryID))
	addEntry("hardcover_id", nonEmpty(meta.HardcoverID), stringVal(book.HardcoverID))
	addEntry("google_books_id", nonEmpty(meta.GoogleBooksID), stringVal(book.GoogleBooksID))

	return provenance
}

// nonEmpty returns the string if non-empty, nil otherwise (for comparison map building).
func nonEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
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
		// cached sizes used
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

	// Recalculating library sizes (cache expired)

	// Calculate library size
	librarySize = 0
	if rootDir != "" {
		if info, err := os.Stat(rootDir); err == nil && info.IsDir() {
			filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
				if err == nil && !info.IsDir() {
					librarySize += filePhysicalSize(info)
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
					importSize += filePhysicalSize(info)
				}
				return nil
			})
		}
	}

	// Update cache
	cachedLibrarySize = librarySize
	cachedImportSize = importSize
	cachedSizeComputedAt = time.Now()

	// sizes recalculated
	return
}

// Server represents the HTTP server
type Server struct {
	httpServer             *http.Server
	router                 *gin.Engine
	audiobookService       *AudiobookService
	audiobookUpdateService *AudiobookUpdateService
	batchService           *BatchService
	workService            *WorkService
	authorSeriesService    *AuthorSeriesService
	filesystemService      *FilesystemService
	importPathService      *ImportPathService
	importService          *ImportService
	scanService            *ScanService
	organizeService        *OrganizeService
	metadataFetchService   *MetadataFetchService
	configUpdateService    *ConfigUpdateService
	systemService          *SystemService
	metadataStateService   *MetadataStateService
	dashboardService       *DashboardService
	dashboardCache         *cache.Cache[gin.H]
	olService              *OpenLibraryService
	dedupCache             *cache.Cache[gin.H]
	listCache              *cache.Cache[gin.H]
	libraryWatcher         *itunes.LibraryWatcher
	updater                *updater.Updater
	updateScheduler        *updater.Scheduler
	scheduler              *TaskScheduler
	aiScanStore            *database.AIScanStore
	pipelineManager        *PipelineManager
	batchPoller            *BatchPoller
	mergeService           *MergeService
	diagnosticsService     *DiagnosticsService
	changelogService       *ChangelogService
	activityService        *ActivityService
	embeddingStore         *database.EmbeddingStore
	dedupEngine            *DedupEngine
	activityWriter         *activityWriter
	http3Server            *http3.Server
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
	router := gin.New() // don't use gin.Default() — we add our own middleware

	// Custom logger that skips noisy polling endpoints
	router.Use(gin.LoggerWithConfig(gin.LoggerConfig{
		SkipPaths: []string{"/api/v1/operations/active"},
	}))
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())
	router.Use(servermiddleware.BasicAuth())

	// Register metrics (idempotent)
	metrics.Register()

	store := database.GetGlobalStore()
	server := &Server{
		router:                 router,
		audiobookService:       NewAudiobookService(store),
		audiobookUpdateService: NewAudiobookUpdateService(store),
		batchService:           NewBatchService(store),
		workService:            NewWorkService(store),
		authorSeriesService:    NewAuthorSeriesService(store),
		filesystemService:      NewFilesystemService(),
		importPathService:      NewImportPathService(store),
		importService:          NewImportService(store),
		scanService:            NewScanService(store),
		organizeService:        NewOrganizeService(store),
		metadataFetchService:   NewMetadataFetchService(store),
		configUpdateService:    NewConfigUpdateService(store),
		systemService:          NewSystemService(store),
		metadataStateService:   NewMetadataStateService(store),
		dashboardService:       NewDashboardService(store),
		dashboardCache:         cache.New[gin.H](30 * time.Second),
		dedupCache:             cache.New[gin.H](5 * time.Minute),
		listCache:              cache.New[gin.H](30 * time.Second),
		olService:              NewOpenLibraryService(),
		updater:                updater.NewUpdater(appVersion),
		mergeService:           NewMergeService(store),
		diagnosticsService:     NewDiagnosticsService(store, nil, config.AppConfig.ITunesLibraryReadPath),
		changelogService:       NewChangelogService(store),
	}

	// Initialize update scheduler
	server.updateScheduler = updater.NewScheduler(server.updater, func() updater.SchedulerConfig {
		return updater.SchedulerConfig{
			Enabled:     config.AppConfig.AutoUpdateEnabled,
			Channel:     config.AppConfig.AutoUpdateChannel,
			CheckMins:   config.AppConfig.AutoUpdateCheckMinutes,
			WindowStart: config.AppConfig.AutoUpdateWindowStart,
			WindowEnd:   config.AppConfig.AutoUpdateWindowEnd,
		}
	})
	server.updateScheduler.Start()

	// Wire OL dump store into metadata fetch service for local-first lookups
	if server.olService != nil && server.olService.Store() != nil {
		server.metadataFetchService.SetOLStore(server.olService.Store())
	}

	// Wire ISBN enrichment service into metadata fetch service
	isbnSources := server.metadataFetchService.BuildSourceChain()
	if len(isbnSources) > 0 {
		server.metadataFetchService.SetISBNEnrichment(
			NewISBNEnrichmentService(database.GlobalStore, isbnSources),
		)
	}

	// Open AI scan store alongside main DB
	if dbPath := config.AppConfig.DatabasePath; dbPath != "" {
		aiScanDBPath := filepath.Join(filepath.Dir(dbPath), "ai_scans.db")
		aiScanStore, err := database.NewAIScanStore(aiScanDBPath)
		if err != nil {
			log.Printf("[WARN] Failed to open AI scan store: %v", err)
		} else {
			server.aiScanStore = aiScanStore
			aiParserInst := newAIParser(config.AppConfig.OpenAIAPIKey, config.AppConfig.EnableAIParsing)
			if p, ok := aiParserInst.(*ai.OpenAIParser); ok {
				server.pipelineManager = NewPipelineManager(aiScanStore, database.GlobalStore, p, server)
				server.batchPoller = NewBatchPoller(database.GlobalStore, p)
				server.registerBatchPollerHandlers()
			}
		}
	}

	// Open activity log store alongside main DB
	if dbPath := config.AppConfig.DatabasePath; dbPath != "" {
		activityDBPath := filepath.Join(filepath.Dir(dbPath), "activity.db")
		activityStore, err := database.NewActivityStore(activityDBPath)
		if err != nil {
			log.Printf("[WARN] Failed to open activity log store: %v", err)
		} else {
			server.activityService = NewActivityService(activityStore)
		}
	}

	// Open embedding store for dedup
	if dbPath := config.AppConfig.DatabasePath; dbPath != "" {
		embeddingDBPath := filepath.Join(filepath.Dir(dbPath), "embeddings.db")
		embeddingStore, err := database.NewEmbeddingStore(embeddingDBPath)
		if err != nil {
			log.Printf("[WARN] Failed to open embedding store: %v", err)
		} else {
			server.embeddingStore = embeddingStore
			if config.AppConfig.OpenAIAPIKey != "" && config.AppConfig.EmbeddingEnabled {
				embedClient := ai.NewEmbeddingClient(config.AppConfig.OpenAIAPIKey)
				// Dedup Layer 3 uses a dedicated chat parser so it can call
				// OpenAIParser.ReviewDedupPairs during maintenance runs.
				llmParser := ai.NewOpenAIParser(config.AppConfig.OpenAIAPIKey, config.AppConfig.EnableAIParsing)
				server.dedupEngine = NewDedupEngine(
					embeddingStore,
					database.GlobalStore,
					embedClient,
					llmParser,
					server.mergeService,
				)
				server.dedupEngine.BookHighThreshold = config.AppConfig.DedupBookHighThreshold
				server.dedupEngine.BookLowThreshold = config.AppConfig.DedupBookLowThreshold
				server.dedupEngine.AuthorHighThreshold = config.AppConfig.DedupAuthorHighThreshold
				server.dedupEngine.AuthorLowThreshold = config.AppConfig.DedupAuthorLowThreshold
				server.dedupEngine.AutoMergeEnabled = config.AppConfig.DedupAutoMergeEnabled
				log.Println("[INFO] Embedding store and dedup engine initialized")
				server.metadataFetchService.SetDedupEngine(server.dedupEngine)

				// Wire the embedding-based metadata candidate scorer. The
				// scorer reuses the same embedClient + embeddingStore as the
				// dedup engine; it's a lightweight wrapper exposing the
				// MetadataCandidateScorer interface. Any failure at search
				// time falls back to the F1 path inside scoreBaseCandidates,
				// so this is safe to leave wired up unconditionally once
				// the embedding infra is available.
				if config.AppConfig.MetadataEmbeddingScoringEnabled {
					server.metadataFetchService.SetMetadataScorer(
						ai.NewEmbeddingScorer(embedClient, embeddingStore),
					)
					log.Println("[INFO] Metadata candidate scoring: embedding tier enabled")
				}

				// Wire the LLM rerank scorer. It reuses the same llmParser
				// the dedup engine uses for Layer 3 review. The scorer is
				// injected unconditionally — the per-search use_rerank flag
				// and the MetadataLLMScoringEnabled config key together gate
				// whether it actually fires.
				server.metadataFetchService.SetMetadataLLMScorer(ai.NewLLMScorer(llmParser))
				if config.AppConfig.MetadataLLMScoringEnabled {
					log.Println("[INFO] Metadata candidate scoring: LLM rerank tier enabled (opt-in per search)")
				} else {
					log.Println("[INFO] Metadata candidate scoring: LLM rerank tier wired but disabled in config")
				}
			} else {
				log.Println("[INFO] Embedding store opened (dedup engine disabled — no API key or embedding_enabled=false)")
			}
		}
	}

	// Start embedding backfill if dedup engine is ready
	if server.dedupEngine != nil {
		go server.runEmbeddingBackfill()
	}

	// Wire activity log dual-write hooks
	if server.activityService != nil {
		// Task 10: Operation changes → activity log
		operations.ActivityRecorder = func(entry database.ActivityEntry) {
			_ = server.activityService.Record(entry)
		}

		// Task 11/14: Metadata fetch service → activity log
		server.metadataFetchService.SetActivityService(server.activityService)

		// Wire activity service into audiobook service for snapshot comparison fallback
		server.audiobookService.SetActivityService(server.activityService)

		// Global log capture via teeWriter — replaces globalActivityRecorder
		aw := newActivityWriter(server.activityService.Store(), 10000)
		aw.Start()
		server.activityWriter = aw
		log.SetOutput(aw)

		// Task 15: iTunes sync → activity log
		itunesActivityRecorder = func(entry database.ActivityEntry) {
			_ = server.activityService.Record(entry)
		}

		// Task 16: Scanner → activity log
		scanner.ScanActivityRecorder = func(bookID, title string) {
			_ = server.activityService.Record(database.ActivityEntry{
				Tier:    "change",
				Type:    "scan",
				Level:   "info",
				Source:  "background",
				BookID:  bookID,
				Summary: fmt.Sprintf("Scan found: %s", title),
			})
		}

		// Record server startup in activity log
		_ = server.activityService.Record(database.ActivityEntry{
			Tier:    "debug",
			Type:    "system",
			Level:   "info",
			Source:  "server",
			Summary: "Server started, activity log initialized",
		})
		log.Println("[INFO] Activity log service initialized and recording")
	}

	server.setupRoutes()

	// Initialize the iTunes auto write-back batcher
	InitWriteBackBatcher()

	// Initialize the file I/O worker pool (bounded concurrency for embed/tag/rename)
	// Set global server ref for recovery of interrupted file ops
	globalServer = server
	InitFileIOPool()

	return server
}

// resumeInterruptedOperations checks for operations left in running/queued state
// from a previous server lifecycle and re-enqueues them.
func (s *Server) resumeInterruptedOperations() {
	store := database.GlobalStore
	if store == nil || operations.GlobalQueue == nil {
		return
	}

	interrupted, err := store.GetInterruptedOperations()
	if err != nil {
		log.Printf("[WARN] Failed to query interrupted operations: %v", err)
		return
	}

	oq, ok := operations.GlobalQueue.(*operations.OperationQueue)
	if !ok {
		return
	}

	for _, op := range interrupted {
		// Mark as interrupted in DB
		_ = store.UpdateOperationStatus(op.ID, "interrupted", op.Progress, op.Total, "server restarted")

		checkpoint, _ := operations.LoadCheckpoint(store, op.ID)
		phaseInfo := ""
		if checkpoint != nil {
			phaseInfo = fmt.Sprintf(" from %s at %d/%d", checkpoint.Phase, checkpoint.PhaseIndex, checkpoint.PhaseTotal)
		}
		log.Printf("[INFO] Resuming interrupted operation %s (%s)%s", op.ID, op.Type, phaseInfo)

		opID := op.ID
		opType := op.Type

		var resumeFn operations.OperationFunc
		switch opType {
		case "itunes_import":
			params, _ := operations.LoadParams[operations.ITunesImportParams](store, opID)
			if params == nil {
				log.Printf("[WARN] No params found for interrupted iTunes import %s, marking as failed", opID)
				_ = store.UpdateOperationError(opID, "no saved params, cannot resume")
				continue
			}
			resumeFn = func(ctx context.Context, progress operations.ProgressReporter) error {
				// Rebuild the ITunesImportRequest from saved params
				var mappings []itunes.PathMapping
				for from, to := range params.PathMappings {
					mappings = append(mappings, itunes.PathMapping{From: from, To: to})
				}
				return executeITunesImport(ctx, operations.LoggerFromReporter(progress), opID, ITunesImportRequest{
					LibraryPath:      params.LibraryXMLPath,
					ImportMode:       params.ImportMode,
					PathMappings:     mappings,
					SkipDuplicates:   params.SkipDuplicates,
					FetchMetadata:    params.EnrichMetadata,
					PreserveLocation: !params.AutoOrganize,
				})
			}
		case "scan":
			params, _ := operations.LoadParams[operations.ScanParams](store, opID)
			if params == nil {
				log.Printf("[WARN] No params found for interrupted scan %s, marking as failed", opID)
				_ = store.UpdateOperationError(opID, "no saved params, cannot resume")
				continue
			}
			resumeFn = func(ctx context.Context, progress operations.ProgressReporter) error {
				forceUpdate := params.ForceUpdate
				return s.scanService.PerformScanWithID(ctx, opID, &ScanRequest{
					FolderPath:  params.FolderPath,
					ForceUpdate: &forceUpdate,
				}, operations.LoggerFromReporter(progress))
			}
		case "organize":
			resumeFn = func(ctx context.Context, progress operations.ProgressReporter) error {
				return s.organizeService.PerformOrganizeWithID(ctx, opID, &OrganizeRequest{}, operations.LoggerFromReporter(progress))
			}
		case "reconcile_scan", "transcode", "diagnostics_export", "diagnostics_ai",
			"bulk_write_back", "cleanup_activity_log", "purge_old_logs",
			"purge-deleted", "tombstone-cleanup", "isbn-enrichment",
			"author-dedup-scan", "author-split-scan", "series-prune",
			"db-optimize", "cleanup-old-backups", "batch_poller",
			"itunes_sync", "metadata-refresh":
			// These are not resumable — mark as failed silently
			_ = store.UpdateOperationError(opID, fmt.Sprintf("interrupted during %s, please retry", opType))
			_ = operations.ClearState(store, opID)
			continue
		default:
			// Unknown type — mark as failed without noisy logging
			_ = store.UpdateOperationError(opID, "interrupted, cannot resume")
			_ = operations.ClearState(store, opID)
			continue
		}

		if err := oq.EnqueueResume(opID, opType, operations.PriorityNormal, resumeFn); err != nil {
			log.Printf("[WARN] Failed to re-enqueue operation %s: %v", opID, err)
		}
	}
}

// Start starts the HTTP server
func (s *Server) Start(cfg ServerConfig) error {
	s.httpServer = &http.Server{
		Addr:              fmt.Sprintf("%s:%s", cfg.Host, cfg.Port),
		Handler:           s.router,
		ReadHeaderTimeout: cfg.ReadTimeout, // Only limit header read, not body (allows large uploads)
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		MaxHeaderBytes:    1 << 20, // 1MB
	}

	if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
		if _, err := os.Stat(cfg.TLSCertFile); err != nil {
			log.Printf("[WARN] TLS certificate not available (%s): %v. Falling back to HTTP-only mode.", cfg.TLSCertFile, err)
			cfg.TLSCertFile = ""
			cfg.TLSKeyFile = ""
			cfg.HTTP3Port = ""
		} else if _, err := os.Stat(cfg.TLSKeyFile); err != nil {
			log.Printf("[WARN] TLS key not available (%s): %v. Falling back to HTTP-only mode.", cfg.TLSKeyFile, err)
			cfg.TLSCertFile = ""
			cfg.TLSKeyFile = ""
			cfg.HTTP3Port = ""
		}
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
				log.Printf("Failed to start HTTPS server: %v", err)
			}
		}()

		// Start HTTP/3 server if configured
		if cfg.HTTP3Port != "" {
			s.http3Server = &http3.Server{
				Addr:      fmt.Sprintf("%s:%s", cfg.Host, cfg.HTTP3Port),
				Handler:   s.router,
				TLSConfig: tlsConfig,
			}
			go func() {
				log.Printf("Starting HTTP/3 (QUIC) server on UDP %s:%s", cfg.Host, cfg.HTTP3Port)
				if err := s.http3Server.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile); err != nil {
					log.Printf("Failed to start HTTP/3 server: %v", err)
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
				log.Printf("Failed to start server: %v", err)
			}
		}()
	}

	// Resume any operations that were interrupted by a previous shutdown/crash
	s.resumeInterruptedOperations()

	// Recover interrupted file I/O operations (cover embed, tag write, rename)
	RecoverInterruptedFileOps()

	// Resume interrupted metadata candidate fetch operations
	s.resumeInterruptedMetadataFetch()

	// Backfill external ID mappings from existing iTunes PIDs (one-time, idempotent)
	go s.backfillExternalIDs()

	// Start periodic cleanup of stale transcode temp files
	if database.GlobalStore != nil {
		if paths, err := database.GlobalStore.GetAllImportPaths(); err == nil {
			for _, p := range paths {
				stopCleanup := transcode.StartCleanupTicker(p.Path, 1*time.Hour, 2*time.Hour)
				defer stopCleanup()
			}
		}
	}

	// Heartbeat: push periodic system.status events via SSE (every 5s) while running
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	shutdown := make(chan struct{})
	var backgroundWG sync.WaitGroup

	// Start unified task scheduler (replaces individual iTunes sync and purge tickers)
	s.scheduler = NewTaskScheduler(s)
	s.scheduler.Start(shutdown, &backgroundWG)

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
						}
						if folders, err := database.GlobalStore.GetAllImportPaths(); err == nil {
							folderCount = len(folders)
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

	// Start auto-scan file watcher if enabled
	var fileWatcher *watcher.Watcher
	if config.AppConfig.AutoScanEnabled && database.GlobalStore != nil {
		importPaths, err := database.GlobalStore.GetAllImportPaths()
		if err == nil && len(importPaths) > 0 {
			var watchPaths []string
			for _, ip := range importPaths {
				if ip.Enabled {
					watchPaths = append(watchPaths, ip.Path)
				}
			}
			if len(watchPaths) > 0 {
				debounce := 5 * time.Second
				if config.AppConfig.AutoScanDebounceSeconds > 0 {
					debounce = time.Duration(config.AppConfig.AutoScanDebounceSeconds) * time.Second
				}
				watchLog := logger.NewWithActivityLog("auto-scan", database.GlobalStore)
				fileWatcher = watcher.New(func(path string) {
					watchLog.Info("Auto-scan triggered for: %s", path)
					if hub := realtime.GetGlobalHub(); hub != nil {
						hub.Broadcast(&realtime.Event{
							Type: "scan.auto_triggered",
							Data: map[string]any{"path": path},
						})
					}
					if s.scanService != nil && operations.GlobalQueue != nil {
						go func() {
							scanPath := path
							id := ulid.Make().String()
							op, opErr := database.GlobalStore.CreateOperation(id, "scan", &scanPath)
							if opErr != nil {
								watchLog.Error("Auto-scan: failed to create operation: %v", opErr)
								return
							}
							scanReq := &ScanRequest{FolderPath: &scanPath}
							opFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
								return s.scanService.PerformScan(ctx, scanReq, operations.LoggerFromReporter(progress))
							}
							if enqueueErr := operations.GlobalQueue.Enqueue(op.ID, "scan", operations.PriorityLow, opFunc); enqueueErr != nil {
								watchLog.Error("Auto-scan: failed to enqueue: %v", enqueueErr)
							}
						}()
					}
				}, debounce)
				// Start watching the first import path (primary)
				if startErr := fileWatcher.Start(watchPaths[0]); startErr != nil {
					watchLog.Warn("Failed to start file watcher: %v", startErr)
					fileWatcher = nil
				} else {
					watchLog.Info("Auto-scan file watcher started for %s", watchPaths[0])
				}
			}
		}
	}

	// Periodic cleanup of expired/revoked auth sessions.
	if database.GlobalStore != nil {
		sessionLog := logger.NewWithActivityLog("session-cleanup", database.GlobalStore)
		sessionCleanupTicker := time.NewTicker(10 * time.Minute)
		backgroundWG.Add(1)
		go func() {
			defer backgroundWG.Done()
			defer sessionCleanupTicker.Stop()
			for {
				select {
				case <-sessionCleanupTicker.C:
					if deleted, err := database.GlobalStore.DeleteExpiredSessions(time.Now()); err != nil {
						sessionLog.Warn("failed to clean up expired sessions: %v", err)
					} else if deleted > 0 {
						sessionLog.Info("cleaned up %d expired/revoked sessions", deleted)
					}
				case <-shutdown:
					return
				}
			}
		}()
	}

	// Periodically mark stale operations as failed.
	if database.GlobalStore != nil && config.AppConfig.OperationTimeoutMinutes > 0 {
		staleTimeout := time.Duration(config.AppConfig.OperationTimeoutMinutes) * time.Minute
		staleTicker := time.NewTicker(1 * time.Minute)
		backgroundWG.Add(1)
		go func() {
			defer backgroundWG.Done()
			defer staleTicker.Stop()
			for {
				select {
				case <-staleTicker.C:
					s.failStaleOperations(staleTimeout)
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

	// Broadcast shutdown event to all connected clients FIRST
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

	// Stop accepting HTTP requests BEFORE closing any stores.
	// This prevents panics from requests hitting closed PebbleDB instances.
	log.Println("[INFO] Stopping HTTP servers...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if s.http3Server != nil {
		if err := s.http3Server.Close(); err != nil {
			log.Printf("[WARN] HTTP/3 server close error: %v", err)
		}
	}
	if err := s.httpServer.Shutdown(ctx); err != nil {
		log.Printf("[WARN] HTTP server forced shutdown: %v", err)
	}

	// Stop the file I/O pool — waits for in-flight jobs to finish
	if p := GetGlobalFileIOPool(); p != nil {
		log.Println("[INFO] Waiting for file I/O operations to complete...")
		p.Stop()
	}

	// Flush the ITL write-back batcher
	if GlobalWriteBackBatcher != nil {
		log.Println("[INFO] Flushing iTunes write-back batcher...")
		GlobalWriteBackBatcher.Stop()
	}

	// Stop activity writer before closing store
	if s.activityWriter != nil {
		s.activityWriter.Stop()
	}

	// Close activity log store
	if s.activityService != nil {
		if err := s.activityService.Store().Close(); err != nil {
			log.Printf("[WARN] Failed to close activity log store: %v", err)
		} else {
			log.Println("[INFO] Activity log store closed")
		}
	}

	// Stop file watcher
	if fileWatcher != nil {
		fileWatcher.Stop()
		log.Println("[INFO] File watcher stopped")
	}

	// Close embedding store
	if s.embeddingStore != nil {
		if err := s.embeddingStore.Close(); err != nil {
			log.Printf("[WARN] Failed to close embedding store: %v", err)
		} else {
			log.Println("[INFO] Embedding store closed")
		}
	}

	// Close AI scan store
	if s.aiScanStore != nil {
		if err := s.aiScanStore.Close(); err != nil {
			log.Printf("[WARN] Failed to close AI scan store: %v", err)
		} else {
			log.Println("[INFO] AI scan store closed")
		}
		s.aiScanStore = nil
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
	s.router.GET("/health", s.healthCheck)
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

	jsonLimitBytes := int64(config.AppConfig.JSONBodyLimitMB) * 1024 * 1024
	uploadLimitBytes := int64(config.AppConfig.UploadBodyLimitMB) * 1024 * 1024

	// Rate limiting is opt-in. Default 0 means disabled (local/single-user server).
	apiRateLimiter := gin.HandlerFunc(func(c *gin.Context) { c.Next() })
	authRateLimiter := gin.HandlerFunc(func(c *gin.Context) { c.Next() })
	if rpm := config.AppConfig.APIRateLimitPerMinute; rpm > 0 {
		burst := rpm / 5
		if burst < 10 {
			burst = 10
		}
		apiRateLimiter = servermiddleware.NewIPRateLimiter(rpm, burst).Middleware()
	}
	if rpm := config.AppConfig.AuthRateLimitPerMinute; rpm > 0 {
		burst := rpm / 5
		if burst < 5 {
			burst = 5
		}
		authRateLimiter = servermiddleware.NewIPRateLimiter(rpm, burst).Middleware()
	}
	bodyLimitMiddleware := servermiddleware.MaxRequestBodySize(jsonLimitBytes, uploadLimitBytes)
	authMiddleware := gin.HandlerFunc(func(c *gin.Context) {
		c.Next()
	})
	if config.AppConfig.EnableAuth {
		authMiddleware = servermiddleware.RequireAuth(database.GlobalStore)
	}

	// API routes (auth + rate limits + request-size limits)
	api := s.router.Group("/api/v1")
	api.Use(apiRateLimiter, bodyLimitMiddleware)
	{
		authGroup := api.Group("/auth")
		authGroup.Use(authRateLimiter)
		{
			authGroup.GET("/status", s.getAuthStatus)
			authGroup.POST("/setup", s.setupInitialAdmin)
			authGroup.POST("/login", s.login)
		}

		authProtected := authGroup.Group("")
		authProtected.Use(authMiddleware)
		{
			authProtected.GET("/me", s.me)
			authProtected.POST("/logout", s.logout)
			authProtected.GET("/sessions", s.listMySessions)
			authProtected.DELETE("/sessions/:id", s.revokeMySession)
		}

		protected := api.Group("")
		protected.Use(authMiddleware)
		{
			// Audiobook routes
			protected.GET("/audiobooks", s.listAudiobooks)
			// /audiobooks/search removed — use GET /audiobooks?search= instead
			protected.GET("/audiobooks/count", s.countAudiobooks)
			protected.GET("/audiobooks/duplicates", s.listDuplicateAudiobooks)
			protected.GET("/audiobooks/duplicates/scan-results", s.listBookDuplicateScanResults)
			protected.POST("/audiobooks/duplicates/scan", s.scanBookDuplicates)
			protected.POST("/audiobooks/duplicates/merge", s.mergeBookDuplicatesAsVersions)
			protected.POST("/audiobooks/duplicates/dismiss", s.dismissBookDuplicateGroup)
			protected.GET("/audiobooks/soft-deleted", s.listSoftDeletedAudiobooks)
			protected.DELETE("/audiobooks/purge-soft-deleted", s.purgeSoftDeletedAudiobooks)
			protected.POST("/audiobooks/:id/restore", s.restoreAudiobook)
			protected.GET("/audiobooks/:id", s.getAudiobook)
			protected.GET("/audiobooks/:id/tags", s.getAudiobookTags)
			protected.PUT("/audiobooks/:id", s.updateAudiobook)
			protected.DELETE("/audiobooks/:id", s.deleteAudiobook)
			protected.GET("/audiobooks/:id/cover", s.serveAudiobookCover)
			protected.GET("/audiobooks/:id/segments", s.listAudiobookSegments)
			protected.GET("/audiobooks/:id/segments/:segmentId/tags", s.getSegmentTags)
			protected.GET("/audiobooks/:id/files", s.listBookFiles)
			protected.GET("/audiobooks/:id/changelog", s.getBookChangelog)
			protected.GET("/audiobooks/:id/path-history", s.getBookPathHistory)
			protected.GET("/audiobooks/:id/external-ids", s.getAudiobookExternalIDs)
			protected.POST("/audiobooks/:id/extract-track-info", s.extractTrackInfo)
			protected.POST("/audiobooks/:id/relocate", s.relocateBookFiles)
			protected.POST("/audiobooks/batch", s.batchUpdateAudiobooks)
			protected.POST("/audiobooks/batch-write-back", s.batchWriteBackAudiobooks)
			protected.POST("/audiobooks/bulk-write-back", s.handleBulkWriteBack)
			protected.POST("/audiobooks/batch-operations", s.batchOperations)

			// User tag routes
			protected.GET("/tags", s.listAllUserTags)
			protected.GET("/audiobooks/:id/user-tags", s.getBookUserTags)
			protected.POST("/audiobooks/batch-tags", s.batchUpdateTags)

			// User preferences
			protected.GET("/preferences/:key", s.getUserPreference)
			protected.PUT("/preferences/:key", s.setUserPreference)
			protected.DELETE("/preferences/:key", s.deleteUserPreference)

			// Metadata change history
			protected.GET("/audiobooks/:id/metadata-history", s.getBookMetadataHistory)
			protected.GET("/audiobooks/:id/metadata-history/:field", s.getFieldMetadataHistory)
			protected.POST("/audiobooks/:id/metadata-history/:field/undo", s.undoMetadataChange)
			protected.POST("/audiobooks/:id/undo-last-apply", s.undoLastApply)
			protected.GET("/audiobooks/:id/field-states", s.getAudiobookFieldStates)
			protected.GET("/audiobooks/:id/changes", s.getBookChanges)

			// Author, narrator, and series routes
			protected.GET("/authors", s.listAuthors)
			protected.GET("/authors/count", s.countAuthors)
			protected.GET("/authors/duplicates", s.listDuplicateAuthors)
			protected.POST("/authors/duplicates/refresh", s.refreshDuplicateAuthors)
			protected.POST("/authors/duplicates/ai-review", s.aiReviewDuplicateAuthors)
			protected.POST("/authors/duplicates/ai-review/apply", s.applyAIAuthorReview)
			protected.POST("/authors/merge", s.mergeAuthors)
			protected.POST("/authors/:id/reclassify-as-narrator", s.reclassifyAuthorAsNarrator)
			protected.PUT("/authors/:id/name", s.renameAuthor)
			protected.POST("/authors/:id/split", s.splitCompositeAuthor)
			protected.POST("/authors/:id/resolve-production", s.resolveProductionAuthor)
			protected.GET("/authors/:id/aliases", s.getAuthorAliases)
			protected.POST("/authors/:id/aliases", s.createAuthorAlias)
			protected.DELETE("/authors/:id/aliases/:aliasId", s.deleteAuthorAlias)
			protected.POST("/audiobooks/merge", s.mergeBooks)
			protected.GET("/narrators", s.listNarrators)
			protected.GET("/narrators/count", s.countNarrators)
			protected.GET("/audiobooks/:id/narrators", s.listAudiobookNarrators)
			protected.PUT("/audiobooks/:id/narrators", s.setAudiobookNarrators)
			protected.GET("/series", s.listSeries)
			protected.GET("/series/count", s.countSeries)
			protected.GET("/series/duplicates", s.listSeriesDuplicates)
			protected.POST("/series/duplicates/refresh", s.refreshSeriesDuplicates)
			protected.POST("/series/deduplicate", s.deduplicateSeriesHandler)
			protected.POST("/series/merge", s.mergeSeriesGroup)
			protected.GET("/series/prune/preview", s.seriesPrunePreview)
			protected.POST("/series/prune", s.seriesPrune)
			protected.PATCH("/series/:id", s.updateSeriesName)
			protected.GET("/series/:id/books", s.getSeriesBooks)
			protected.PUT("/series/:id/name", s.renameSeriesHandler)
			protected.POST("/series/:id/split", s.splitSeriesHandler)
			protected.DELETE("/series/:id", s.deleteEmptySeries)
			protected.GET("/authors/:id/books", s.getAuthorBooks)
			protected.DELETE("/authors/:id", s.deleteAuthorHandler)
			protected.POST("/authors/bulk-delete", s.bulkDeleteAuthors)
			protected.POST("/series/bulk-delete", s.bulkDeleteSeries)
			protected.POST("/dedup/validate", s.validateDedupEntry)

			// Embedding-based dedup
			protected.GET("/dedup/candidates", s.listDedupCandidates)
			protected.GET("/dedup/stats", s.getDedupStats)
			protected.POST("/dedup/candidates/:id/merge", s.mergeDedupCandidate)
			protected.POST("/dedup/candidates/:id/dismiss", s.dismissDedupCandidate)
			protected.POST("/dedup/candidates/bulk-merge", s.bulkMergeDedupCandidates)
			protected.POST("/dedup/candidates/merge-cluster", s.mergeDedupCluster)
			protected.POST("/dedup/candidates/dismiss-cluster", s.dismissDedupCluster)
			protected.POST("/dedup/scan", s.triggerDedupScan)
			protected.POST("/dedup/scan-llm", s.triggerDedupLLM)
			protected.POST("/dedup/refresh", s.triggerDedupRefresh)

			// File system routes
			protected.GET("/filesystem/home", s.getHomeDirectory)
			protected.GET("/filesystem/browse", s.browseFilesystem)
			protected.POST("/filesystem/exclude", s.createExclusion)
			protected.DELETE("/filesystem/exclude", s.removeExclusion)

			// Import path routes
			protected.GET("/import-paths", s.listImportPaths)
			protected.POST("/import-paths", s.addImportPath)
			protected.DELETE("/import-paths/:id", s.removeImportPath)

			// Operation routes
			protected.GET("/operations", s.listOperations)
			protected.GET("/operations/active", s.listActiveOperations)
			protected.GET("/operations/stale", s.listStaleOperations)
			protected.POST("/operations/scan", s.startScan)
			protected.POST("/operations/organize", s.startOrganize)
			protected.POST("/operations/transcode", s.startTranscode)
			protected.GET("/operations/recent", s.handleGetRecentOperations)
			protected.GET("/operations/:id/results", s.handleGetOperationResults)
			protected.GET("/operations/:id/status", s.getOperationStatus)
			protected.GET("/operations/:id/logs", s.getOperationLogs)
			protected.GET("/operations/:id/result", s.getOperationResult)
			protected.DELETE("/operations/:id", s.cancelOperation)
			protected.POST("/operations/clear-stale", s.clearStaleOperations)
			protected.DELETE("/operations/history", s.deleteOperationHistory)
			protected.POST("/operations/optimize-database", s.optimizeDatabase)
			protected.POST("/operations/sweep-tombstones", s.sweepTombstones)
			protected.GET("/operations/audit-files", s.auditFileConsistency)
			protected.GET("/operations/reconcile/preview", s.reconcilePreview)
			protected.POST("/operations/reconcile", s.startReconcile)
			protected.POST("/operations/reconcile/scan", s.startReconcileScan)
			protected.GET("/operations/reconcile/scan/latest", s.latestReconcileScan)
			protected.POST("/operations/cleanup-version-groups", s.cleanupDuplicateVersionGroupsHandler)
			protected.POST("/operations/mark-broken-segments", s.markBrokenSegmentBooksHandler)
			protected.POST("/operations/merge-novg-duplicates", s.mergeNoVGDuplicatesHandler)
			protected.POST("/operations/assign-orphan-vgs", s.assignOrphanVGsHandler)
			protected.GET("/operations/:id/changes", s.getOperationChanges)
			protected.POST("/operations/:id/revert", s.revertOperation)

			// Import routes
			protected.POST("/import/file", s.importFile)

			// iTunes import routes
			itunesGroup := protected.Group("/itunes")
			{
				itunesGroup.POST("/validate", s.handleITunesValidate)
				itunesGroup.POST("/test-mapping", s.handleITunesTestMapping)
				itunesGroup.POST("/import", s.handleITunesImport)
				itunesGroup.POST("/write-back", s.handleITunesWriteBack)
				itunesGroup.POST("/write-back-all", s.handleITunesWriteBackAll)
				itunesGroup.POST("/write-back/preview", s.handleITunesWriteBackPreview)
				itunesGroup.GET("/books", s.handleListITunesBooks)
				itunesGroup.GET("/import-status/:id", s.handleITunesImportStatus)
				itunesGroup.POST("/import-status/bulk", s.handleITunesImportStatusBulk)
				itunesGroup.GET("/library-status", s.handleITunesLibraryStatus)
				itunesGroup.POST("/sync", s.handleITunesSync)
			}

			// Cover art
			protected.GET("/covers/proxy", s.handleCoverProxy)
			protected.GET("/covers/local/:filename", s.handleLocalCover)

			// Unified task/scheduler routes
			protected.GET("/tasks", s.listTasks)
			protected.POST("/tasks/:name/run", s.runTask)
			protected.PUT("/tasks/:name", s.updateTaskConfig)
			protected.POST("/maintenance-window/run", s.runMaintenanceWindowNow)
			protected.POST("/maintenance/fix-read-by-narrator", s.handleFixReadByNarrator)
			protected.POST("/maintenance/cleanup-series", s.handleCleanupSeries)
			protected.POST("/maintenance/backfill-book-files", s.handleBackfillBookFiles)
			protected.POST("/maintenance/cleanup-empty-folders", s.handleCleanupEmptyFolders)
			protected.POST("/maintenance/cleanup-organize-mess", s.handleCleanupOrganizeMess)
			protected.POST("/maintenance/fix-author-narrator-swap", s.handleFixAuthorNarratorSwap)
			protected.POST("/maintenance/fix-version-groups", s.handleFixVersionGroups)
			protected.POST("/maintenance/fix-library-states", s.handleFixLibraryStates)
			protected.POST("/maintenance/enrich-book-files", s.handleEnrichBookFiles)
			protected.POST("/maintenance/dedup-books", s.handleDedupBooks)
			protected.POST("/maintenance/fix-book-file-paths", s.handleFixBookFilePaths)
			protected.POST("/maintenance/refetch-missing-authors", s.handleRefetchMissingAuthors)
			protected.POST("/maintenance/recompute-itunes-paths", s.handleRecomputeITunesPaths)
			protected.POST("/maintenance/generate-itl-tests", s.handleGenerateITLTests)

			// Admin-only destructive endpoints
			adminOnly := protected.Group("")
			adminOnly.Use(servermiddleware.RequireAdmin())
			{
				adminOnly.POST("/maintenance/wipe", s.handleWipe)
			}

			// Unified activity log
			protected.GET("/activity", s.listActivity)
			protected.GET("/activity/sources", s.listActivitySources)
			protected.POST("/activity/compact", s.compactActivity)

			// System routes
			protected.GET("/system/status", s.getSystemStatus)
			protected.GET("/system/announcements", s.getSystemAnnouncements)
			protected.GET("/system/storage", s.getSystemStorage)
			protected.GET("/system/logs", s.getSystemLogs)
			protected.GET("/system/activity-log", s.getSystemActivityLog)
			protected.POST("/system/reset", s.resetSystem)
			protected.POST("/system/factory-reset", s.factoryReset)
			protected.GET("/config", s.getConfig)
			protected.PUT("/config", s.updateConfig)
			protected.GET("/dashboard", s.getDashboard)

			// Backup routes
			protected.POST("/backup/create", s.createBackup)
			protected.GET("/backup/list", s.listBackups)
			protected.POST("/backup/restore", s.restoreBackup)
			protected.DELETE("/backup/:filename", s.deleteBackup)

			// Enhanced metadata routes
			protected.POST("/metadata/batch-update", s.batchUpdateMetadata)
			protected.POST("/metadata/validate", s.validateMetadata)
			protected.GET("/metadata/export", s.exportMetadata)
			protected.POST("/metadata/import", s.importMetadata)
			protected.GET("/metadata/search", s.searchMetadata)
			protected.GET("/metadata/fields", s.getMetadataFields)
			protected.POST("/metadata/bulk-fetch", s.bulkFetchMetadata)
			protected.POST("/metadata/batch-fetch-candidates", s.handleBatchFetchCandidates)
			protected.POST("/metadata/batch-apply-candidates", s.handleBatchApplyCandidates)
			protected.POST("/metadata/batch-reject-candidates", s.handleRejectCandidates)
			protected.POST("/metadata/batch-unreject-candidates", s.handleUnrejectCandidates)
			protected.POST("/audiobooks/:id/fetch-metadata", s.fetchAudiobookMetadata)
			protected.POST("/audiobooks/:id/search-metadata", s.searchAudiobookMetadata)
			protected.POST("/audiobooks/:id/apply-metadata", s.applyAudiobookMetadata)
			protected.POST("/audiobooks/:id/mark-no-match", s.markAudiobookNoMatch)
			protected.POST("/audiobooks/:id/revert-metadata", s.revertAudiobookMetadata)
			protected.GET("/audiobooks/:id/cow-versions", s.listBookCOWVersions)
			protected.POST("/audiobooks/:id/cow-versions/prune", s.pruneBookCOWVersions)
			protected.POST("/audiobooks/:id/write-back", s.writeBackAudiobookMetadata)

			// Rename preview and apply
			protected.POST("/audiobooks/:id/rename/preview", s.previewRename)
			protected.POST("/audiobooks/:id/rename/apply", s.applyRename)

			// Organize preview and execute (single book)
			protected.GET("/audiobooks/:id/preview-organize", s.previewOrganize)
			protected.POST("/audiobooks/:id/organize", s.organizeBook)

			// AI-powered parsing routes
			protected.POST("/ai/parse-filename", s.parseFilenameWithAI)
			protected.POST("/ai/test-connection", s.testAIConnection)

			// AI Scan Pipeline
			aiScans := protected.Group("/ai/scans")
			aiScans.POST("", s.startAIScan)
			aiScans.GET("", s.listAIScans)
			aiScans.GET("/compare", s.compareAIScans) // Must be before /:id to avoid conflict
			aiScans.GET("/:id", s.getAIScan)
			aiScans.GET("/:id/results", s.getAIScanResults)
			aiScans.POST("/:id/apply", s.applyAIScanResults)
			aiScans.POST("/:id/cancel", s.cancelAIScan)
			aiScans.DELETE("/:id", s.deleteAIScan)
			protected.POST("/metadata-sources/test", s.testMetadataSource)
			protected.POST("/audiobooks/:id/parse-with-ai", s.parseAudiobookWithAI)

			// Open Library dump routes
			protected.GET("/openlibrary/status", s.getOLStatus)
			protected.POST("/openlibrary/download", s.startOLDownload)
			protected.POST("/openlibrary/import", s.startOLImport)
			protected.POST("/openlibrary/upload", s.uploadOLDump)
			protected.DELETE("/openlibrary/data", s.deleteOLData)

			// Work routes (logical title-level grouping)
			protected.GET("/works", s.listWorks)
			protected.POST("/works", s.createWork)
			protected.GET("/works/:id", s.getWork)
			protected.PUT("/works/:id", s.updateWork)
			protected.DELETE("/works/:id", s.deleteWork)
			protected.GET("/works/:id/books", s.listWorkBooks)

			// Version management routes
			protected.GET("/audiobooks/:id/versions", s.listAudiobookVersions)
			protected.POST("/audiobooks/:id/versions", s.linkAudiobookVersion)
			protected.PUT("/audiobooks/:id/set-primary", s.setAudiobookPrimary)
			protected.POST("/audiobooks/:id/split-version", s.splitVersion)
			protected.POST("/audiobooks/:id/split-to-books", s.splitSegmentsToBooks)
			protected.POST("/audiobooks/:id/move-segments", s.moveSegments)
			protected.GET("/version-groups/:id", s.getVersionGroup)

			// Work queue routes (alternative singular form for compatibility)
			protected.GET("/work", s.listWork)
			protected.GET("/work/stats", s.getWorkStats)

			// Update routes
			protected.GET("/update/status", s.getUpdateStatus)
			protected.POST("/update/check", s.checkForUpdate)
			protected.POST("/update/apply", s.applyUpdate)

			// Blocked hashes management routes
			protected.GET("/blocked-hashes", s.listBlockedHashes)
			protected.POST("/blocked-hashes", s.addBlockedHash)
			protected.DELETE("/blocked-hashes/:hash", s.removeBlockedHash)

			// Diagnostics routes
			protected.POST("/diagnostics/export", s.startDiagnosticsExport)
			protected.GET("/diagnostics/export/:operationId/download", s.downloadDiagnosticsExport)
			protected.POST("/diagnostics/submit-ai", s.submitDiagnosticsAI)
			protected.GET("/diagnostics/ai-results/:operationId", s.getDiagnosticsAIResults)
			protected.POST("/diagnostics/apply-suggestions", s.applyDiagnosticsSuggestions)

			// Bench routes (only available with -tags bench)
			s.setupUserTagRoutes(protected)
			s.setupBenchRoutes(protected)
		}
	}

	// Serve static files (React frontend)
	// Implementation is in static_embed.go or static_nonembed.go depending on build tags
	s.setupStaticFiles()
}

// corsMiddleware adds restrictive CORS headers.
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := strings.TrimSpace(c.GetHeader("Origin"))
		allowedOrigin := ""
		isDevMode := gin.Mode() == gin.DebugMode

		if origin != "" {
			// Dev-mode CORS: allow Vite dev server only.
			if isDevMode && (origin == "http://localhost:5173" || origin == "https://localhost:5173") {
				allowedOrigin = origin
			}

			// Always allow same-origin requests.
			host := strings.TrimSpace(c.Request.Host)
			if host != "" {
				if origin == "http://"+host || origin == "https://"+host {
					allowedOrigin = origin
				}
			}
		}

		if allowedOrigin != "" {
			c.Header("Access-Control-Allow-Origin", allowedOrigin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, Authorization, Cache-Control, X-Requested-With")
			c.Header("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH")
		}

		if c.Request.Method == http.MethodOptions {
			if origin != "" && allowedOrigin == "" {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.AbortWithStatus(http.StatusNoContent)
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
		// Playlist count intentionally omitted — no reliable counting method yet
	}
	resp := gin.H{
		"status":        "ok",
		"timestamp":     time.Now().Unix(),
		"version":       appVersion,
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
	// Build cache key from the full query string
	cacheKey := "list:" + c.Request.URL.RawQuery
	if cached, ok := s.listCache.Get(cacheKey); ok {
		c.JSON(http.StatusOK, cached)
		return
	}

	// Parse pagination parameters
	params := ParsePaginationParams(c)
	authorID := ParseQueryIntPtr(c, "author_id")
	seriesID := ParseQueryIntPtr(c, "series_id")

	// Parse optional filters
	sortOrder := ParseQueryString(c, "sort_order")
	if sortOrder != "" && sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "asc"
	}
	filters := ListFilters{
		IsPrimaryVersion: ParseQueryBoolPtr(c, "is_primary_version"),
		LibraryState:     ParseQueryString(c, "library_state"),
		Tag:              ParseQueryString(c, "tag"),
		SortBy:           ParseQueryString(c, "sort_by"),
		SortOrder:        sortOrder,
	}

	// Parse field filters from JSON query param
	if filtersJSON := c.Query("filters"); filtersJSON != "" {
		var fieldFilters []FieldFilter
		if err := json.Unmarshal([]byte(filtersJSON), &fieldFilters); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid filters parameter: " + err.Error()})
			return
		}
		filters.FieldFilters = fieldFilters
	}

	// Call service
	books, err := s.audiobookService.GetAudiobooks(c.Request.Context(), params.Limit, params.Offset, params.Search, authorID, seriesID, filters)
	if err != nil {
		internalError(c, "failed to list audiobooks", err)
		return
	}

	// Enrich with author and series names
	enriched := s.audiobookService.EnrichAudiobooksWithNames(books)

	// Get total count for proper pagination
	totalCount := len(enriched)
	hasFilters := filters.IsPrimaryVersion != nil || filters.LibraryState != "" || filters.Tag != ""
	if params.Search == "" && authorID == nil && seriesID == nil {
		if hasFilters {
			if tc, err := s.audiobookService.CountAudiobooksFiltered(c.Request.Context(), filters); err == nil {
				totalCount = tc
			}
		} else {
			if tc, err := s.audiobookService.CountAudiobooks(c.Request.Context()); err == nil {
				totalCount = tc
			}
		}
	}

	resp := gin.H{"items": enriched, "count": totalCount, "limit": params.Limit, "offset": params.Offset}
	s.listCache.Set(cacheKey, resp)
	c.JSON(http.StatusOK, resp)
}

func (s *Server) listDuplicateAudiobooks(c *gin.Context) {
	if cached, ok := s.dedupCache.Get("book-duplicates"); ok {
		c.JSON(http.StatusOK, cached)
		return
	}

	result, err := s.audiobookService.GetDuplicateBooks(c.Request.Context())
	if err != nil {
		internalError(c, "failed to list duplicate audiobooks", err)
		return
	}

	resp := gin.H{
		"groups":          result.Groups,
		"group_count":     result.GroupCount,
		"duplicate_count": result.DuplicateCount,
	}
	s.dedupCache.Set("book-duplicates", resp)
	c.JSON(http.StatusOK, resp)
}

// listBookDuplicateScanResults returns cached results from the last async book-dedup scan.
func (s *Server) listBookDuplicateScanResults(c *gin.Context) {
	if cached, ok := s.dedupCache.Get("book-dedup-scan"); ok {
		c.JSON(http.StatusOK, cached)
		return
	}
	c.JSON(http.StatusOK, gin.H{"groups": []any{}, "group_count": 0, "duplicate_count": 0, "needs_refresh": true})
}

// scanBookDuplicates triggers an async scan for book duplicates using metadata matching.
func (s *Server) scanBookDuplicates(c *gin.Context) {
	if operations.GlobalQueue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	store := database.GlobalStore
	opID := ulid.Make().String()
	op, err := store.CreateOperation(opID, "book-dedup-scan", nil)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		_ = progress.UpdateProgress(0, 100, "Scanning for duplicate books...")

		// Step 1: Hash-based duplicates (high confidence)
		_ = progress.UpdateProgress(10, 100, "Finding hash-based duplicates...")
		hashGroups, err := store.GetDuplicateBooks()
		if err != nil {
			return fmt.Errorf("hash-based dedup failed: %w", err)
		}

		// Step 2: Folder duplicates (same title in same folder)
		_ = progress.UpdateProgress(30, 100, "Finding folder-based duplicates...")
		folderGroups, err := store.GetFolderDuplicates()
		if err != nil {
			log.Printf("[WARN] folder dedup failed: %v", err)
			folderGroups = nil
		}

		// Step 3: Metadata-based fuzzy matching
		_ = progress.UpdateProgress(50, 100, "Finding metadata-based duplicates...")
		metadataGroups, err := store.GetDuplicateBooksByMetadata(0.85)
		if err != nil {
			log.Printf("[WARN] metadata dedup failed: %v", err)
			metadataGroups = nil
		}

		_ = progress.UpdateProgress(80, 100, "Merging results...")

		// Load dismissed groups
		dismissed := loadDismissedDedupGroups(store)

		// Combine all groups, deduplicating by book ID
		seenBookIDs := map[string]bool{}
		type dupGroup struct {
			Books      []database.Book `json:"books"`
			Confidence string          `json:"confidence"` // "high", "medium", "low"
			Reason     string          `json:"reason"`
			GroupKey   string          `json:"group_key"`
		}
		var allGroups []dupGroup

		addGroups := func(groups [][]database.Book, confidence, reason string) {
			for _, group := range groups {
				allSeen := true
				for _, b := range group {
					if !seenBookIDs[b.ID] {
						allSeen = false
						break
					}
				}
				if allSeen {
					continue
				}
				// Generate a stable group key from sorted book IDs
				ids := make([]string, len(group))
				for i, b := range group {
					ids[i] = b.ID
				}
				groupKey := strings.Join(ids, "+")
				if dismissed[groupKey] {
					continue
				}
				allGroups = append(allGroups, dupGroup{
					Books:      group,
					Confidence: confidence,
					Reason:     reason,
					GroupKey:   groupKey,
				})
				for _, b := range group {
					seenBookIDs[b.ID] = true
				}
			}
		}

		addGroups(hashGroups, "high", "Identical file hash")
		addGroups(folderGroups, "medium", "Same title in same folder")
		addGroups(metadataGroups, "low", "Similar title and author")

		totalDuplicates := 0
		for _, g := range allGroups {
			totalDuplicates += len(g.Books) - 1
		}

		result := gin.H{
			"groups":          allGroups,
			"group_count":     len(allGroups),
			"duplicate_count": totalDuplicates,
		}
		s.dedupCache.SetWithTTL("book-dedup-scan", result, 30*time.Minute)

		_ = progress.UpdateProgress(100, 100, fmt.Sprintf("Found %d duplicate groups (%d duplicates)", len(allGroups), totalDuplicates))
		return nil
	}

	if err := operations.GlobalQueue.Enqueue(opID, "book-dedup-scan", operations.PriorityNormal, operationFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	c.JSON(http.StatusAccepted, op)
}

// mergeBookDuplicatesAsVersions merges a group of duplicate books into a version group.
func (s *Server) mergeBookDuplicatesAsVersions(c *gin.Context) {
	var req struct {
		BookIDs []string `json:"book_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.BookIDs) < 2 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "need at least 2 book IDs"})
		return
	}

	ms := s.mergeService
	if ms == nil {
		ms = NewMergeService(database.GlobalStore)
	}

	result, err := ms.MergeBooks(req.BookIDs, "")
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		} else {
			internalError(c, "failed to merge duplicate books", err)
		}
		return
	}

	s.dedupCache.Invalidate("book-dedup-scan")
	s.dedupCache.Invalidate("book-duplicates")

	c.JSON(http.StatusOK, gin.H{
		"message":          fmt.Sprintf("Merged %d books into version group", result.MergedCount),
		"version_group_id": result.VersionGroupID,
		"primary_id":       result.PrimaryID,
	})
}

// filesCommonDir finds the common parent directory of all BookFile file paths.
func filesCommonDir(files []database.BookFile) string {
	if len(files) == 0 {
		return ""
	}
	common := filepath.Dir(files[0].FilePath)
	for _, f := range files[1:] {
		fDir := filepath.Dir(f.FilePath)
		for common != fDir && !strings.HasPrefix(fDir, common+string(filepath.Separator)) {
			common = filepath.Dir(common)
			if common == "/" || common == "." {
				return common
			}
		}
	}
	return common
}

// isProtectedPath returns true if the given file path is under an import path
// or the iTunes library folder. Files in protected paths must NEVER be moved,
// renamed, or deleted — only hardlinked or copied to the organized library.
func isProtectedPath(filePath string) bool {
	absPath, _ := filepath.Abs(filePath)

	// Check import paths
	if database.GlobalStore != nil {
		importPaths, err := database.GlobalStore.GetAllImportPaths()
		if err == nil {
			for _, ip := range importPaths {
				ipAbs, _ := filepath.Abs(ip.Path)
				if strings.HasPrefix(absPath, ipAbs+"/") || absPath == ipAbs {
					return true
				}
			}
		}
	}

	// Check iTunes library paths
	if config.AppConfig.ITunesLibraryReadPath != "" {
		itunesDir := filepath.Dir(config.AppConfig.ITunesLibraryReadPath)
		itunesAbs, _ := filepath.Abs(itunesDir)
		if strings.HasPrefix(absPath, itunesAbs+"/") || absPath == itunesAbs {
			return true
		}
	}
	if config.AppConfig.ITunesLibraryWritePath != "" {
		itunesDir := filepath.Dir(config.AppConfig.ITunesLibraryWritePath)
		itunesAbs, _ := filepath.Abs(itunesDir)
		if strings.HasPrefix(absPath, itunesAbs+"/") || absPath == itunesAbs {
			return true
		}
	}

	// Also check if path contains "iTunes Media" as a safety net
	if strings.Contains(absPath, "iTunes Media") || strings.Contains(absPath, "iTunes%20Media") {
		return true
	}

	return false
}

// isITunesGhostPath reports whether a book's file path points at the
// iTunes media folder rather than the managed audiobook-organizer library.
// Such books are "ghost" references — iTunes knows about them but they
// live outside the library we actually manage, so they should never be
// chosen as the primary version of a merge.
func isITunesGhostPath(p string) bool {
	if p == "" {
		return false
	}
	lower := strings.ToLower(p)
	return strings.Contains(lower, "/itunes media/") || strings.Contains(lower, "/itunes/itunes")
}

// bookIsBetter returns true if a is a "better" primary version than b.
// Preference: managed library path > iTunes-ghost path, M4B > other formats,
// higher bitrate, larger file.
func bookIsBetter(a, b *database.Book) bool {
	// Path origin trumps everything else — an organized-library copy is
	// always a better primary than an iTunes ghost, regardless of format or
	// bitrate. Otherwise a high-bitrate iTunes import would steal the
	// primary slot from the file the user has actually organized.
	aGhost := isITunesGhostPath(a.FilePath)
	bGhost := isITunesGhostPath(b.FilePath)
	if aGhost != bGhost {
		return !aGhost
	}

	aM4B := strings.EqualFold(a.Format, "m4b")
	bM4B := strings.EqualFold(b.Format, "m4b")
	if aM4B != bM4B {
		return aM4B
	}
	aBitrate := 0
	if a.Bitrate != nil {
		aBitrate = *a.Bitrate
	}
	bBitrate := 0
	if b.Bitrate != nil {
		bBitrate = *b.Bitrate
	}
	if aBitrate != bBitrate {
		return aBitrate > bBitrate
	}
	aSize := int64(0)
	if a.FileSize != nil {
		aSize = *a.FileSize
	}
	bSize := int64(0)
	if b.FileSize != nil {
		bSize = *b.FileSize
	}
	return aSize > bSize
}

// dismissBookDuplicateGroup marks a book duplicate group as not-duplicates.
func (s *Server) dismissBookDuplicateGroup(c *gin.Context) {
	var req struct {
		GroupKey string `json:"group_key" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	store := database.GlobalStore
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Load existing dismissed groups
	dismissed := loadDismissedDedupGroups(store)
	dismissed[req.GroupKey] = true
	saveDismissedDedupGroups(store, dismissed)

	s.dedupCache.Invalidate("book-dedup-scan")

	c.JSON(http.StatusOK, gin.H{"message": "Group dismissed"})
}

// loadDismissedDedupGroups loads the set of dismissed dedup group keys from user preferences.
func loadDismissedDedupGroups(store database.Store) map[string]bool {
	dismissed := map[string]bool{}
	pref, err := store.GetUserPreference("dedup_dismissed_groups")
	if err != nil || pref == nil || pref.Value == nil || *pref.Value == "" {
		return dismissed
	}
	var keys []string
	if err := json.Unmarshal([]byte(*pref.Value), &keys); err != nil {
		return dismissed
	}
	for _, k := range keys {
		dismissed[k] = true
	}
	return dismissed
}

// saveDismissedDedupGroups saves the set of dismissed dedup group keys to user preferences.
func saveDismissedDedupGroups(store database.Store, dismissed map[string]bool) {
	keys := make([]string, 0, len(dismissed))
	for k := range dismissed {
		keys = append(keys, k)
	}
	data, err := json.Marshal(keys)
	if err != nil {
		log.Printf("[WARN] failed to marshal dismissed dedup groups: %v", err)
		return
	}
	if err := store.SetUserPreference("dedup_dismissed_groups", string(data)); err != nil {
		log.Printf("[WARN] failed to save dismissed dedup groups: %v", err)
	}
}

func (s *Server) listSoftDeletedAudiobooks(c *gin.Context) {
	params := ParsePaginationParams(c)
	olderThanDays := ParseQueryIntPtr(c, "older_than_days")

	books, err := s.audiobookService.GetSoftDeletedBooks(c.Request.Context(), params.Limit, params.Offset, olderThanDays)
	if err != nil {
		internalError(c, "failed to list deleted audiobooks", err)
		return
	}

	// Get total count (unpaginated) for proper pagination support
	allBooks, _ := s.audiobookService.GetSoftDeletedBooks(c.Request.Context(), 10000, 0, olderThanDays)
	total := len(allBooks)

	c.JSON(http.StatusOK, gin.H{
		"items":  books,
		"count":  len(books),
		"total":  total,
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
		internalError(c, "failed to purge deleted audiobooks", err)
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

// triggerITunesSync finds the library path from DB and enqueues a sync if the file changed.
func (s *Server) triggerITunesSync() {
	if database.GlobalStore == nil || operations.GlobalQueue == nil {
		return
	}

	libraryPath := discoverITunesLibraryPath()
	if libraryPath == "" {
		return
	}

	// Check fingerprint — skip if unchanged (quick mtime+size check)
	if rec, err := database.GlobalStore.GetLibraryFingerprint(libraryPath); err == nil && rec != nil {
		if info, statErr := os.Stat(libraryPath); statErr == nil {
			if info.Size() == rec.Size && info.ModTime().Equal(rec.ModTime) {
				return // No changes
			}
		}
	}

	itunesTriggerLog := logger.NewWithActivityLog("itunes-scheduler", database.GlobalStore)
	opID := ulid.Make().String()
	op, err := database.GlobalStore.CreateOperation(opID, "itunes_sync", &libraryPath)
	if err != nil {
		itunesTriggerLog.Warn("iTunes sync scheduler: failed to create operation: %v", err)
		return
	}

	// Load path mappings from config for the scheduled sync
	var scheduledMappings []itunes.PathMapping
	for _, m := range config.AppConfig.ITunesPathMappings {
		scheduledMappings = append(scheduledMappings, itunes.PathMapping{From: m.From, To: m.To})
	}

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		return executeITunesSync(ctx, operations.LoggerFromReporter(progress), libraryPath, scheduledMappings)
	}

	if err := operations.GlobalQueue.Enqueue(op.ID, "itunes_sync", operations.PriorityNormal, operationFunc); err != nil {
		itunesTriggerLog.Warn("iTunes sync scheduler: failed to enqueue: %v", err)
		return
	}

	itunesTriggerLog.Info("iTunes sync scheduler: enqueued sync operation %s", op.ID)
}

func isStaleOperationStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "running", "queued", "in_progress":
		return true
	default:
		return false
	}
}

func operationStartedOrCreatedAt(op database.Operation) time.Time {
	if op.StartedAt != nil && !op.StartedAt.IsZero() {
		return *op.StartedAt
	}
	return op.CreatedAt
}

func (s *Server) collectStaleOperations(timeout time.Duration) ([]database.Operation, error) {
	if timeout <= 0 {
		return []database.Operation{}, nil
	}
	if database.GlobalStore == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	ops, err := database.GlobalStore.GetRecentOperations(500)
	if err != nil {
		return nil, err
	}
	threshold := time.Now().Add(-timeout)
	stale := make([]database.Operation, 0)
	for _, op := range ops {
		if !isStaleOperationStatus(op.Status) {
			continue
		}
		started := operationStartedOrCreatedAt(op)
		if started.IsZero() || started.After(threshold) {
			continue
		}
		stale = append(stale, op)
	}
	return stale, nil
}

func (s *Server) failStaleOperations(timeout time.Duration) {
	staleLog := logger.NewWithActivityLog("reaper", database.GlobalStore)
	stale, err := s.collectStaleOperations(timeout)
	if err != nil {
		staleLog.Warn("stale operation check failed: %v", err)
		return
	}
	if len(stale) == 0 {
		return
	}

	for _, op := range stale {
		msg := fmt.Sprintf("operation timed out after %s", timeout)
		if err := database.GlobalStore.UpdateOperationError(op.ID, msg); err != nil {
			staleLog.Warn("failed to mark stale operation %s as failed: %v", op.ID, err)
			continue
		}
		if hub := realtime.GetGlobalHub(); hub != nil {
			hub.SendOperationStatus(op.ID, "failed", map[string]any{
				"error": msg,
			})
		}
		staleLog.Warn("marked stale operation as failed: id=%s type=%s", op.ID, op.Type)
	}
}

func (s *Server) getSystemActivityLog(c *gin.Context) {
	source := c.Query("source")
	limit := 50
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 {
		limit = l
	}
	logs, err := database.GlobalStore.GetSystemActivityLogs(source, limit)
	if err != nil {
		internalError(c, "failed to get activity log", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": logs, "count": len(logs)})
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
		internalError(c, "failed to count audiobooks", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"count": count})
}

func (s *Server) serveAudiobookCover(c *gin.Context) {
	id := c.Param("id")
	if config.AppConfig.RootDir == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "root_dir not configured"})
		return
	}
	coverPath := metadata.CoverPathForBook(config.AppConfig.RootDir, id)
	if coverPath == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "no cover art found"})
		return
	}
	c.File(coverPath)
}

func (s *Server) getAudiobook(c *gin.Context) {
	id := c.Param("id")

	book, err := s.audiobookService.GetAudiobook(c.Request.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		internalError(c, "failed to get audiobook", err)
		return
	}

	c.JSON(http.StatusOK, enrichBookForResponse(book))
}

// listAudiobookSegments returns file segments for a multi-file audiobook.
// Backward-compatible: internally queries book_files and returns data in the
// legacy BookSegment JSON shape so the frontend continues to work.
func (s *Server) listAudiobookSegments(c *gin.Context) {
	id := c.Param("id")
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	book, err := database.GlobalStore.GetBookByID(id)
	if err != nil || book == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
		return
	}

	files, err := database.GlobalStore.GetBookFiles(book.ID)
	if err != nil {
		internalError(c, "failed to list book files", err)
		return
	}
	if files == nil {
		files = []database.BookFile{}
	}

	// Convert BookFile to legacy segment JSON shape with file_exists
	result := make([]gin.H, 0, len(files))
	for _, f := range files {
		_, statErr := os.Stat(f.FilePath)
		result = append(result, gin.H{
			"id":               f.ID,
			"book_id":          int(crc32.ChecksumIEEE([]byte(f.BookID))),
			"file_path":        f.FilePath,
			"format":           f.Format,
			"size_bytes":       f.FileSize,
			"duration_seconds": f.Duration / 1000, // BookFile stores ms
			"track_number":     f.TrackNumber,
			"total_tracks":     f.TrackCount,
			"segment_title":    f.Title,
			"file_hash":        f.FileHash,
			"active":           !f.Missing,
			"superseded_by":    nil,
			"created_at":       f.CreatedAt,
			"updated_at":       f.UpdatedAt,
			"file_exists":      statErr == nil,
		})
	}

	c.JSON(http.StatusOK, result)
}

// listBookFiles returns all book_files rows for a book with live disk-existence check.
func (s *Server) listBookFiles(c *gin.Context) {
	bookID := c.Param("id")
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	files, err := database.GlobalStore.GetBookFiles(bookID)
	if err != nil {
		internalError(c, "failed to get book files", err)
		return
	}
	if files == nil {
		files = []database.BookFile{}
	}
	results := make([]gin.H, 0, len(files))
	for _, f := range files {
		_, statErr := os.Stat(f.FilePath)
		results = append(results, gin.H{
			"id":                   f.ID,
			"book_id":              f.BookID,
			"file_path":            f.FilePath,
			"original_filename":    f.OriginalFilename,
			"itunes_path":          f.ITunesPath,
			"itunes_persistent_id": f.ITunesPersistentID,
			"track_number":         f.TrackNumber,
			"track_count":          f.TrackCount,
			"disc_number":          f.DiscNumber,
			"disc_count":           f.DiscCount,
			"title":                f.Title,
			"format":               f.Format,
			"codec":                f.Codec,
			"duration":             f.Duration,
			"file_size":            f.FileSize,
			"bitrate_kbps":         f.BitrateKbps,
			"sample_rate_hz":       f.SampleRateHz,
			"channels":             f.Channels,
			"bit_depth":            f.BitDepth,
			"file_hash":            f.FileHash,
			"missing":              f.Missing,
			"file_exists":          statErr == nil,
			"created_at":           f.CreatedAt,
			"updated_at":           f.UpdatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"files": results, "count": len(results)})
}

// extractTrackInfo parses track/disk numbers from segment filenames and updates segments.
func (s *Server) extractTrackInfo(c *gin.Context) {
	id := c.Param("id")
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	book, err := database.GlobalStore.GetBookByID(id)
	if err != nil || book == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
		return
	}

	files, err := database.GlobalStore.GetBookFiles(book.ID)
	if err != nil {
		internalError(c, "failed to list book files", err)
		return
	}

	filePaths := make([]string, len(files))
	for i, f := range files {
		filePaths[i] = f.FilePath
	}

	trackInfos := metadata.ExtractTrackInfoBatch(filePaths)

	// Second pass: normalize track numbers to be 1-indexed and fill gaps
	// Some players/files use 0-based numbering (0-50); we always want 1-based (1-51)
	hasZero := false
	for _, info := range trackInfos {
		if info.TrackNumber != nil && *info.TrackNumber == 0 {
			hasZero = true
			break
		}
	}
	if hasZero {
		for i := range trackInfos {
			if trackInfos[i].TrackNumber != nil {
				n := *trackInfos[i].TrackNumber + 1
				trackInfos[i].TrackNumber = &n
			}
		}
	}

	// Assign sequential numbers to files that had no parseable track number
	usedNumbers := map[int]bool{}
	for _, info := range trackInfos {
		if info.TrackNumber != nil {
			usedNumbers[*info.TrackNumber] = true
		}
	}
	nextNum := 1
	total := len(files)
	for i := range trackInfos {
		if trackInfos[i].TrackNumber == nil {
			for usedNumbers[nextNum] {
				nextNum++
			}
			n := nextNum
			trackInfos[i].TrackNumber = &n
			usedNumbers[nextNum] = true
			nextNum++
		}
		// Ensure TotalTracks is set for all entries
		trackInfos[i].TotalTracks = &total
	}

	updated := 0
	for i, info := range trackInfos {
		oldTrack := files[i].TrackNumber
		if info.TrackNumber != nil {
			files[i].TrackNumber = *info.TrackNumber
		}
		if info.TotalTracks != nil {
			files[i].TrackCount = *info.TotalTracks
		}
		if err := database.GlobalStore.UpdateBookFile(files[i].ID, &files[i]); err != nil {
			log.Printf("WARN: failed to update book file %s track info: %v", files[i].ID, err)
			continue
		}
		updated++

		// Record the track number change in history
		var prevVal, newVal string
		if oldTrack != 0 {
			prevVal = strconv.Itoa(oldTrack)
		}
		if info.TrackNumber != nil {
			newVal = strconv.Itoa(*info.TrackNumber)
		}
		prev := prevVal
		nv := newVal
		_ = database.GlobalStore.RecordMetadataChange(&database.MetadataChangeRecord{
			BookID:        id,
			Field:         "track_number",
			PreviousValue: &prev,
			NewValue:      &nv,
			ChangeType:    "auto_number",
			Source:        "filename_extraction",
			ChangedAt:     time.Now(),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"updated": updated,
		"total":   len(files),
		"files":   files,
	})
}

// relocateBookFiles updates segment file paths when files have been moved.
func (s *Server) relocateBookFiles(c *gin.Context) {
	id := c.Param("id")
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	book, err := database.GlobalStore.GetBookByID(id)
	if err != nil || book == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
		return
	}

	var req RelocateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	files, err := database.GlobalStore.GetBookFiles(book.ID)
	if err != nil {
		internalError(c, "failed to list book files", err)
		return
	}

	result := RelocateResult{}

	if req.SegmentID != "" && req.NewPath != "" {
		// Individual mode: update one file (SegmentID maps to file ID)
		for i, f := range files {
			if f.ID == req.SegmentID {
				if _, statErr := os.Stat(req.NewPath); os.IsNotExist(statErr) {
					c.JSON(http.StatusBadRequest, gin.H{"error": "new path does not exist on disk"})
					return
				}
				files[i].FilePath = req.NewPath
				if err := database.GlobalStore.UpdateBookFile(files[i].ID, &files[i]); err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("update file %s: %v", f.ID, err))
				} else {
					result.Updated++
				}
				break
			}
		}
	} else if req.FolderPath != "" {
		// Folder mode: scan folder and match files by name
		dirEntries, err := os.ReadDir(req.FolderPath)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("cannot read folder: %v", err)})
			return
		}

		// Build map of filename -> full path in the new folder
		fileMap := make(map[string]string)
		for _, de := range dirEntries {
			if !de.IsDir() {
				fileMap[de.Name()] = filepath.Join(req.FolderPath, de.Name())
			}
		}

		for i, f := range files {
			oldName := filepath.Base(f.FilePath)
			if newPath, ok := fileMap[oldName]; ok {
				files[i].FilePath = newPath
				if err := database.GlobalStore.UpdateBookFile(files[i].ID, &files[i]); err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("update file %s: %v", f.ID, err))
				} else {
					result.Updated++
				}
			}
		}
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "must provide segment_id+new_path or folder_path"})
		return
	}

	// Update book's file_path to match first file
	if result.Updated > 0 && len(files) > 0 {
		book.FilePath = files[0].FilePath
		if _, err := database.GlobalStore.UpdateBook(book.ID, book); err != nil {
			log.Printf("[WARN] failed to update book file_path: %v", err)
		}
	}

	c.JSON(http.StatusOK, result)
}

// getSegmentTags returns raw metadata tags for a specific segment file.
func (s *Server) getSegmentTags(c *gin.Context) {
	id := c.Param("id")
	segmentId := c.Param("segmentId")
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	book, err := database.GlobalStore.GetBookByID(id)
	if err != nil || book == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
		return
	}

	found, err := database.GlobalStore.GetBookFileByID(book.ID, segmentId)
	if err != nil {
		internalError(c, "failed to get book file", err)
		return
	}
	if found == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "segment not found"})
		return
	}

	tags := map[string]string{}
	usedFallback := false
	tagsReadError := ""

	meta, err := metadata.ExtractMetadata(found.FilePath, nil)
	if err != nil {
		tagsReadError = err.Error()
	} else {
		usedFallback = meta.UsedFilenameFallback
		if meta.Title != "" {
			tags["title"] = meta.Title
		}
		if meta.Artist != "" {
			tags["artist"] = meta.Artist
		}
		if meta.Album != "" {
			tags["album"] = meta.Album
		}
		if meta.Genre != "" {
			tags["genre"] = meta.Genre
		}
		if meta.Series != "" {
			tags["series"] = meta.Series
		}
		if meta.SeriesIndex != 0 {
			tags["series_index"] = strconv.Itoa(meta.SeriesIndex)
		}
		if meta.Comments != "" {
			tags["comments"] = meta.Comments
		}
		if meta.Year != 0 {
			tags["year"] = strconv.Itoa(meta.Year)
		}
		if meta.Narrator != "" {
			tags["narrator"] = meta.Narrator
		}
		if meta.Language != "" {
			tags["language"] = meta.Language
		}
		if meta.Publisher != "" {
			tags["publisher"] = meta.Publisher
		}
		if meta.ISBN10 != "" {
			tags["isbn10"] = meta.ISBN10
		}
		if meta.ISBN13 != "" {
			tags["isbn13"] = meta.ISBN13
		}
	}

	resp := gin.H{
		"segment_id":             found.ID,
		"file_path":              found.FilePath,
		"format":                 found.Format,
		"size_bytes":             found.FileSize,
		"duration_sec":           found.Duration / 1000,
		"track_number":           found.TrackNumber,
		"total_tracks":           found.TrackCount,
		"tags":                   tags,
		"used_filename_fallback": usedFallback,
	}
	if tagsReadError != "" {
		resp["tags_read_error"] = tagsReadError
	}

	c.JSON(http.StatusOK, resp)
}

func (s *Server) getBookMetadataHistory(c *gin.Context) {
	id := c.Param("id")
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	limit := 100
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	records, err := database.GlobalStore.GetBookChangeHistory(id, limit)
	if err != nil {
		internalError(c, "failed to get metadata history", err)
		return
	}
	if records == nil {
		records = []database.MetadataChangeRecord{}
	}
	c.JSON(http.StatusOK, gin.H{"items": records, "count": len(records)})
}

func (s *Server) getAudiobookFieldStates(c *gin.Context) {
	id := c.Param("id")
	states, err := s.metadataStateService.LoadMetadataState(id)
	if err != nil {
		internalError(c, "failed to get field states", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"field_states": states})
}

func (s *Server) getFieldMetadataHistory(c *gin.Context) {
	id := c.Param("id")
	field := c.Param("field")
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	limit := 50
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	records, err := database.GlobalStore.GetMetadataChangeHistory(id, field, limit)
	if err != nil {
		internalError(c, "failed to get field history", err)
		return
	}
	if records == nil {
		records = []database.MetadataChangeRecord{}
	}
	c.JSON(http.StatusOK, gin.H{"items": records, "count": len(records)})
}

func (s *Server) undoMetadataChange(c *gin.Context) {
	id := c.Param("id")
	field := c.Param("field")
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Get the latest change for this field
	records, err := database.GlobalStore.GetMetadataChangeHistory(id, field, 1)
	if err != nil {
		internalError(c, "failed to get field history", err)
		return
	}
	if len(records) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "no change history found for this field"})
		return
	}

	latest := records[0]

	// Apply the previous value back via metadata state service
	if latest.PreviousValue != nil {
		var prevValue any
		if err := json.Unmarshal([]byte(*latest.PreviousValue), &prevValue); err != nil {
			prevValue = *latest.PreviousValue
		}
		if err := s.metadataStateService.SetOverride(id, field, prevValue, false); err != nil {
			internalError(c, "failed to apply undo", err)
			return
		}
	} else {
		// Previous value was nil, so clear the override
		if err := s.metadataStateService.ClearOverride(id, field); err != nil {
			// Ignore "not found" errors when clearing
			if !strings.Contains(err.Error(), "not found") {
				internalError(c, "failed to clear override", err)
				return
			}
		}
	}

	// Record the undo itself
	undoRecord := &database.MetadataChangeRecord{
		BookID:        id,
		Field:         field,
		PreviousValue: latest.NewValue,
		NewValue:      latest.PreviousValue,
		ChangeType:    "undo",
		Source:        "manual",
		ChangedAt:     time.Now(),
	}
	if err := database.GlobalStore.RecordMetadataChange(undoRecord); err != nil {
		log.Printf("[WARN] failed to record undo change for %s/%s: %v", id, field, err)
	}

	c.JSON(http.StatusOK, gin.H{"message": "undo applied", "field": field, "reverted_to": latest.PreviousValue})
}

// undoLastApply reverts all fields changed in the most recent metadata apply for a book.
func (s *Server) undoLastApply(c *gin.Context) {
	id := c.Param("id")
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Get recent history for this book (enough to find the last apply batch)
	history, err := database.GlobalStore.GetBookChangeHistory(id, 50)
	if err != nil {
		internalError(c, "failed to get change history", err)
		return
	}
	if len(history) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "no change history found"})
		return
	}

	// Find the most recent non-undo change timestamp to identify the batch
	var batchTime time.Time
	for _, rec := range history {
		if rec.ChangeType != "undo" {
			batchTime = rec.ChangedAt
			break
		}
	}
	if batchTime.IsZero() {
		c.JSON(http.StatusNotFound, gin.H{"error": "no changes to undo"})
		return
	}

	// Collect all changes from this batch (within 2 seconds of each other)
	var batchRecords []*database.MetadataChangeRecord
	for i := range history {
		rec := &history[i]
		if rec.ChangeType == "undo" {
			continue
		}
		diff := batchTime.Sub(rec.ChangedAt)
		if diff < 0 {
			diff = -diff
		}
		if diff <= 2*time.Second {
			batchRecords = append(batchRecords, rec)
		}
	}

	if len(batchRecords) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "no changes to undo"})
		return
	}

	// Undo each field in the batch
	undoneFields := []string{}
	for _, rec := range batchRecords {
		if rec.PreviousValue != nil {
			var prevValue any
			if jsonErr := json.Unmarshal([]byte(*rec.PreviousValue), &prevValue); jsonErr != nil {
				prevValue = *rec.PreviousValue
			}
			if setErr := s.metadataStateService.SetOverride(id, rec.Field, prevValue, false); setErr != nil {
				log.Printf("[WARN] undo-last-apply: failed to revert %s for %s: %v", rec.Field, id, setErr)
				continue
			}
		} else {
			if clrErr := s.metadataStateService.ClearOverride(id, rec.Field); clrErr != nil {
				if !strings.Contains(clrErr.Error(), "not found") {
					log.Printf("[WARN] undo-last-apply: failed to clear %s for %s: %v", rec.Field, id, clrErr)
					continue
				}
			}
		}
		undoneFields = append(undoneFields, rec.Field)

		// Record the undo
		undoRec := &database.MetadataChangeRecord{
			BookID:        id,
			Field:         rec.Field,
			PreviousValue: rec.NewValue,
			NewValue:      rec.PreviousValue,
			ChangeType:    "undo",
			Source:        "bulk-search-undo",
			ChangedAt:     time.Now(),
		}
		if recErr := database.GlobalStore.RecordMetadataChange(undoRec); recErr != nil {
			log.Printf("[WARN] undo-last-apply: failed to record undo for %s/%s: %v", id, rec.Field, recErr)
		}
	}

	// Re-write tags to files if write-back is enabled, restoring original values
	if len(undoneFields) > 0 && GlobalWriteBackBatcher != nil {
		GlobalWriteBackBatcher.Enqueue(id)
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       fmt.Sprintf("Undid %d field(s)", len(undoneFields)),
		"undone_fields": undoneFields,
	})
}

func (s *Server) getBookPathHistory(c *gin.Context) {
	id := c.Param("id")
	history, err := database.GlobalStore.GetBookPathHistory(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"history": []any{}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"history": history})
}

func (s *Server) getAudiobookExternalIDs(c *gin.Context) {
	id := c.Param("id")
	eidStore := asExternalIDStore(database.GlobalStore)
	if eidStore == nil {
		c.JSON(http.StatusOK, gin.H{"external_ids": []any{}, "itunes_linked": false})
		return
	}
	extIDs, err := eidStore.GetExternalIDsForBook(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"external_ids": []any{}, "itunes_linked": false})
		return
	}
	itunesLinked := false
	for _, eid := range extIDs {
		if eid.Source == "itunes" && !eid.Tombstoned {
			itunesLinked = true
			break
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"external_ids":  extIDs,
		"itunes_linked": itunesLinked,
		"total":         len(extIDs),
	})
}

func (s *Server) getAudiobookTags(c *gin.Context) {
	id := c.Param("id")
	compareID := c.Query("compare_id")
	snapshotTS := c.Query("snapshot_ts")
	if snapshotTS != "" {
		if _, err := time.Parse(time.RFC3339Nano, snapshotTS); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid snapshot_ts format, use RFC3339Nano"})
			return
		}
	}
	resp, err := s.audiobookService.GetAudiobookTags(c.Request.Context(), id, compareID, snapshotTS)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		internalError(c, "failed to get tags", err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// --- User tag handlers ---

func (s *Server) listAllUserTags(c *gin.Context) {
	tags, err := s.audiobookService.ListAllUserTags()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if tags == nil {
		tags = []database.TagWithCount{}
	}
	c.JSON(http.StatusOK, gin.H{"tags": tags})
}

func (s *Server) getBookUserTags(c *gin.Context) {
	id := c.Param("id")
	tags, err := s.audiobookService.GetBookUserTags(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if tags == nil {
		tags = []string{}
	}
	c.JSON(http.StatusOK, gin.H{"tags": tags})
}

func (s *Server) batchUpdateTags(c *gin.Context) {
	var body struct {
		BookIDs    []string `json:"book_ids"`
		AddTags    []string `json:"add_tags"`
		RemoveTags []string `json:"remove_tags"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if len(body.BookIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "book_ids is required"})
		return
	}
	if len(body.AddTags) == 0 && len(body.RemoveTags) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one of add_tags or remove_tags is required"})
		return
	}
	// Filter out empty strings from tag arrays
	filterEmpty := func(tags []string) []string {
		out := make([]string, 0, len(tags))
		for _, t := range tags {
			if strings.TrimSpace(t) != "" {
				out = append(out, t)
			}
		}
		return out
	}
	body.AddTags = filterEmpty(body.AddTags)
	body.RemoveTags = filterEmpty(body.RemoveTags)
	updated, err := s.audiobookService.BatchUpdateUserTags(body.BookIDs, body.AddTags, body.RemoveTags)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"updated": updated})
}

func (s *Server) getBookChangelog(c *gin.Context) {
	id := c.Param("id")
	entries, err := s.changelogService.GetBookChangelog(id)
	if err != nil {
		internalError(c, "failed to get changelog", err)
		return
	}
	if entries == nil {
		entries = []ChangeLogEntry{}
	}
	c.JSON(http.StatusOK, gin.H{"entries": entries})
}

func (s *Server) updateAudiobook(c *gin.Context) {
	id := c.Param("id")

	var payload map[string]any
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Fetch old book for change history comparison
	var oldBook *database.Book
	if database.GlobalStore != nil {
		oldBook, _ = database.GlobalStore.GetBookByID(id)
	}

	updatedBook, err := s.audiobookUpdateService.UpdateAudiobook(id, payload)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		internalError(c, "failed to update audiobook", err)
		return
	}

	// Record metadata change history for manual edits
	if oldBook != nil && database.GlobalStore != nil {
		now := time.Now()
		manualChanges := []struct {
			field  string
			oldVal string
			newVal string
		}{
			{"title", oldBook.Title, updatedBook.Title},
			{"narrator", ptrStr(oldBook.Narrator), ptrStr(updatedBook.Narrator)},
			{"publisher", ptrStr(oldBook.Publisher), ptrStr(updatedBook.Publisher)},
			{"language", ptrStr(oldBook.Language), ptrStr(updatedBook.Language)},
		}
		// Compare author names
		oldAuthor := ""
		if oldBook.AuthorID != nil {
			if a, err := database.GlobalStore.GetAuthorByID(*oldBook.AuthorID); err == nil && a != nil {
				oldAuthor = a.Name
			}
		}
		newAuthor := ""
		if updatedBook.AuthorID != nil {
			if a, err := database.GlobalStore.GetAuthorByID(*updatedBook.AuthorID); err == nil && a != nil {
				newAuthor = a.Name
			}
		}
		manualChanges = append(manualChanges, struct {
			field  string
			oldVal string
			newVal string
		}{"author_name", oldAuthor, newAuthor})
		// Compare year
		oldYear := ""
		if oldBook.AudiobookReleaseYear != nil {
			oldYear = strconv.Itoa(*oldBook.AudiobookReleaseYear)
		}
		newYear := ""
		if updatedBook.AudiobookReleaseYear != nil {
			newYear = strconv.Itoa(*updatedBook.AudiobookReleaseYear)
		}
		manualChanges = append(manualChanges, struct {
			field  string
			oldVal string
			newVal string
		}{"audiobook_release_year", oldYear, newYear})

		for _, c := range manualChanges {
			if c.newVal == "" || c.newVal == c.oldVal {
				continue
			}
			oldJSON, _ := json.Marshal(c.oldVal)
			newJSON, _ := json.Marshal(c.newVal)
			oldStr := string(oldJSON)
			newStr := string(newJSON)
			record := &database.MetadataChangeRecord{
				BookID:        id,
				Field:         c.field,
				PreviousValue: &oldStr,
				NewValue:      &newStr,
				ChangeType:    "manual",
				Source:        "manual",
				ChangedAt:     now,
			}
			if err := database.GlobalStore.RecordMetadataChange(record); err != nil {
				log.Printf("[WARN] failed to record manual metadata change for %s.%s: %v", id, c.field, err)
			}
		}
	}

	// Write updated metadata back to the audio file
	if updatedBook.FilePath != "" {
		tagMap := make(map[string]interface{})
		if v, ok := payload["title"].(string); ok && v != "" {
			tagMap["title"] = v
		}
		if v, ok := payload["author_name"].(string); ok && v != "" {
			tagMap["artist"] = v
		}
		if v, ok := payload["publisher"].(string); ok && v != "" {
			tagMap["publisher"] = v
		}
		if v, ok := payload["narrator"].(string); ok && v != "" {
			tagMap["album_artist"] = v
		}
		if v, ok := payload["audiobook_release_year"].(float64); ok && v != 0 {
			tagMap["year"] = int(v)
		}
		// If we have multiple authors in join table, combine with " & " for file tags
		if _, hasAuthor := tagMap["artist"]; !hasAuthor && database.GlobalStore != nil {
			if authors, err := database.GlobalStore.GetBookAuthors(id); err == nil && len(authors) > 1 {
				names := make([]string, 0, len(authors))
				for _, ba := range authors {
					if a, err := database.GlobalStore.GetAuthorByID(ba.AuthorID); err == nil && a != nil {
						names = append(names, a.Name)
					}
				}
				if len(names) > 0 {
					tagMap["artist"] = strings.Join(names, ", ")
				}
			}
		}
		// If we have multiple narrators in join table, combine with " & " for file tags
		if _, hasNarr := tagMap["album_artist"]; !hasNarr && database.GlobalStore != nil {
			if narrators, err := database.GlobalStore.GetBookNarrators(id); err == nil && len(narrators) > 1 {
				names := make([]string, 0, len(narrators))
				for _, bn := range narrators {
					if n, err := database.GlobalStore.GetNarratorByID(bn.NarratorID); err == nil && n != nil {
						names = append(names, n.Name)
					}
				}
				if len(names) > 0 {
					tagMap["album_artist"] = strings.Join(names, " & ")
				}
			}
		}
		if len(tagMap) > 0 {
			if isProtectedPath(updatedBook.FilePath) {
				log.Printf("[INFO] skipping write-back for protected path: %s", updatedBook.FilePath)
			} else {
				opConfig := fileops.OperationConfig{VerifyChecksums: true}
				if writeErr := metadata.WriteMetadataToFile(updatedBook.FilePath, tagMap, opConfig); writeErr != nil {
					log.Printf("[WARN] write-back failed for %s: %v", updatedBook.FilePath, writeErr)
				} else {
					// Stamp last_written_at after successful write-back.
					if stampErr := database.GlobalStore.SetLastWrittenAt(updatedBook.ID, time.Now()); stampErr != nil {
						log.Printf("[WARN] failed to stamp last_written_at for book %s: %v", updatedBook.ID, stampErr)
					}
				}
			}
		}
	}

	// Enqueue for iTunes auto write-back if enabled
	if GlobalWriteBackBatcher != nil {
		GlobalWriteBackBatcher.Enqueue(id)
	}

	c.JSON(http.StatusOK, enrichBookForResponse(updatedBook))
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

	// Enqueue all updated books for iTunes auto write-back
	if GlobalWriteBackBatcher != nil && resp != nil {
		for _, item := range resp.Results {
			if item.Success {
				GlobalWriteBackBatcher.Enqueue(item.ID)
			}
		}
	}

	c.JSON(http.StatusOK, resp)
}

// handleBulkWriteBack handles POST /api/v1/audiobooks/bulk-write-back.
// It creates an async operation that writes metadata tags and renames files
// for all books matching the provided filters (or all organized/imported books).
func (s *Server) handleBulkWriteBack(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	if operations.GlobalQueue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	var req struct {
		Filter struct {
			LibraryState *string `json:"library_state"`
			AuthorID     *int    `json:"author_id"`
			SeriesID     *int    `json:"series_id"`
		} `json:"filter"`
		DryRun bool `json:"dry_run"`
		Rename bool `json:"rename"`
	}
	_ = c.ShouldBindJSON(&req)

	store := database.GlobalStore

	// Gather matching books based on filters
	var books []database.Book
	var err error

	if req.Filter.AuthorID != nil {
		books, err = store.GetBooksByAuthorID(*req.Filter.AuthorID)
	} else if req.Filter.SeriesID != nil {
		books, err = store.GetBooksBySeriesID(*req.Filter.SeriesID)
	} else {
		// Get all books, then filter by library_state
		books, err = store.GetAllBooks(1_000_000, 0)
	}
	if err != nil {
		internalError(c, "failed to query books", err)
		return
	}

	// Filter by library_state if specified, otherwise default to organized+imported
	targetStates := map[string]bool{"organized": true, "imported": true}
	if req.Filter.LibraryState != nil && *req.Filter.LibraryState != "" {
		targetStates = map[string]bool{*req.Filter.LibraryState: true}
	}

	var filtered []database.Book
	for _, book := range books {
		// Skip soft-deleted books
		if book.MarkedForDeletion != nil && *book.MarkedForDeletion {
			continue
		}
		// Skip books with empty file paths
		if book.FilePath == "" {
			continue
		}
		// Skip protected paths
		if isProtectedPath(book.FilePath) {
			continue
		}
		// Filter by library state (only when not filtering by author/series exclusively)
		if book.LibraryState != nil {
			if !targetStates[*book.LibraryState] {
				continue
			}
		} else if req.Filter.AuthorID == nil && req.Filter.SeriesID == nil {
			// No library_state set and no author/series filter: skip
			continue
		}
		filtered = append(filtered, book)
	}

	estimatedBooks := len(filtered)

	// Dry run: just return the count
	if req.DryRun {
		c.JSON(http.StatusOK, gin.H{
			"estimated_books": estimatedBooks,
			"dry_run":         true,
		})
		return
	}

	if estimatedBooks == 0 {
		c.JSON(http.StatusOK, gin.H{
			"estimated_books": 0,
			"message":         "no books match the given filters",
		})
		return
	}

	// Create the operation
	opID := ulid.Make().String()
	op, err := store.CreateOperation(opID, "bulk_write_back", nil)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	doRename := req.Rename
	mfs := s.metadataFetchService
	bookIDs := make([]string, len(filtered))
	for i, b := range filtered {
		bookIDs[i] = b.ID
	}

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		total := len(bookIDs)
		written := 0
		failed := 0

		for i, bookID := range bookIDs {
			if progress.IsCanceled() {
				msg := fmt.Sprintf("canceled after %d/%d books (%d written, %d failed)", i, total, written, failed)
				_ = progress.Log("info", msg, nil)
				return nil
			}

			// Check context cancellation
			select {
			case <-ctx.Done():
				msg := fmt.Sprintf("context canceled after %d/%d books (%d written, %d failed)", i, total, written, failed)
				_ = progress.Log("info", msg, nil)
				return ctx.Err()
			default:
			}

			book, err := store.GetBookByID(bookID)
			if err != nil || book == nil {
				failed++
				detail := fmt.Sprintf("book %s: not found", bookID)
				_ = progress.Log("warn", detail, nil)
				_ = progress.UpdateProgress(i+1, total, fmt.Sprintf("processing %d/%d (skipped: not found)", i+1, total))
				continue
			}

			// Skip protected paths (re-check in case data changed)
			if isProtectedPath(book.FilePath) {
				detail := fmt.Sprintf("book %s: skipping protected path %s", bookID, book.FilePath)
				_ = progress.Log("info", detail, nil)
				_ = progress.UpdateProgress(i+1, total, fmt.Sprintf("processing %d/%d (skipped: protected)", i+1, total))
				continue
			}

			// Step 1: Rename if requested
			if doRename {
				if renameErr := mfs.RunApplyPipelineRenameOnly(bookID, book); renameErr != nil {
					detail := fmt.Sprintf("book %s: rename failed: %v", bookID, renameErr)
					_ = progress.Log("warn", detail, nil)
				}
			}

			// Step 2: Write tags
			count, writeErr := mfs.WriteBackMetadataForBook(bookID)
			if writeErr != nil {
				failed++
				detail := fmt.Sprintf("book %s: write-back failed: %v", bookID, writeErr)
				_ = progress.Log("warn", detail, nil)
			} else {
				written++
				if count > 0 {
					detail := fmt.Sprintf("book %s: wrote %d file(s)", bookID, count)
					_ = progress.Log("debug", detail, nil)
				}
			}

			_ = progress.UpdateProgress(i+1, total, fmt.Sprintf("processing %d/%d (%d written, %d failed)", i+1, total, written, failed))
		}

		summary := fmt.Sprintf("bulk write-back complete: %d written, %d failed out of %d", written, failed, total)
		_ = progress.Log("info", summary, nil)
		return nil
	}

	if err := operations.GlobalQueue.Enqueue(op.ID, "bulk_write_back", operations.PriorityNormal, operationFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"operation_id":    op.ID,
		"estimated_books": estimatedBooks,
	})
}

func (s *Server) batchWriteBackAudiobooks(c *gin.Context) {
	var req struct {
		BookIDs  []string `json:"book_ids"`
		Rename   bool     `json:"rename"`
		Organize bool     `json:"organize"`
		Force    bool     `json:"force"` // skip change detection, rewrite everything
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.BookIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "book_ids is required"})
		return
	}

	store := database.GlobalStore
	doOrganize := req.Organize || req.Rename

	// Create a supervisor operation for tracking
	opID := ulid.Make().String()
	if _, err := store.CreateOperation(opID, "batch_save_to_files", nil); err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	bookIDs := make([]string, len(req.BookIDs))
	copy(bookIDs, req.BookIDs)
	totalBooks := len(bookIDs)
	force := req.Force
	mfs := s.metadataFetchService
	orgSvc := s.organizeService

	opFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		_ = progress.UpdateProgress(0, totalBooks, "starting save to files")

		written, organized, failed, skipped := 0, 0, 0, 0
		org := organizer.NewOrganizer(&config.AppConfig)
		log2 := logger.NewWithActivityLog("batch-write-back", store)

		for i, id := range bookIDs {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			book, err := store.GetBookByID(id)
			if err != nil || book == nil {
				failed++
				_ = store.AddOperationLog(opID, "warn", fmt.Sprintf("book %s not found", id), nil)
				continue
			}

			// Skip if already written and metadata hasn't changed since last write
			if !force && book.LastWrittenAt != nil && !book.UpdatedAt.After(*book.LastWrittenAt) {
				skipped++
				_ = progress.UpdateProgress(i+1, totalBooks,
					fmt.Sprintf("processed %d/%d (skipped: %d — already up to date)", i+1, totalBooks, skipped))
				continue
			}

			// Write tags
			_, wbErr := mfs.WriteBackMetadataForBook(id)
			if wbErr != nil {
				failed++
				detail := wbErr.Error()
				_ = store.AddOperationLog(opID, "warn", fmt.Sprintf("write-back failed for %s", book.Title), &detail)
				continue
			}
			written++
			// Stamp last_written_at on the book the user sees (may differ from library copy)
			_ = store.SetLastWrittenAt(id, time.Now())

			// Organize
			if doOrganize {
				book, _ = store.GetBookByID(id)
				if book != nil {
					oldPath := book.FilePath
					alreadyInRoot := config.AppConfig.RootDir != "" && strings.HasPrefix(oldPath, config.AppConfig.RootDir)
					var newPath string
					var orgErr error
					if alreadyInRoot {
						newPath, orgErr = orgSvc.reOrganizeInPlace(book, log2)
					} else {
						bookFiles, _ := store.GetBookFiles(id)
						isDir := len(bookFiles) > 1
						if !isDir {
							if info, statErr := os.Stat(oldPath); statErr == nil && info.IsDir() {
								isDir = true
							}
						}
						if isDir {
							newPath, orgErr = orgSvc.organizeDirectoryBook(org, book, log2)
						} else {
							newPath, _, orgErr = org.OrganizeBook(book)
						}
					}
					if orgErr != nil {
						detail := orgErr.Error()
						_ = store.AddOperationLog(opID, "warn", fmt.Sprintf("organize failed for %s", book.Title), &detail)
					} else if newPath != "" && newPath != oldPath {
						organized++
					}
				}
			}

			// Enqueue ITL write-back
			if GlobalWriteBackBatcher != nil {
				GlobalWriteBackBatcher.Enqueue(id)
			}

			_ = progress.UpdateProgress(i+1, totalBooks,
				fmt.Sprintf("processed %d/%d (written: %d, organized: %d, failed: %d)",
					i+1, totalBooks, written, organized, failed))
		}

		_ = progress.UpdateProgress(totalBooks, totalBooks,
			fmt.Sprintf("complete: written %d, organized %d, skipped %d, failed %d", written, organized, skipped, failed))
		return nil
	}

	if err := operations.GlobalQueue.Enqueue(opID, "batch_save_to_files", operations.PriorityNormal, opFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"operation_id": opID,
		"message":      fmt.Sprintf("Save to files queued for %d books", totalBooks),
		"book_count":   totalBooks,
	})
}

func (s *Server) batchOperations(c *gin.Context) {
	var req BatchOperationsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.Operations) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no operations provided"})
		return
	}
	if len(req.Operations) > 10000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "max 10000 operations per request"})
		return
	}

	resp := s.batchService.ExecuteOperations(&req)

	if GlobalWriteBackBatcher != nil {
		for _, r := range resp.Results {
			if r.Success {
				GlobalWriteBackBatcher.Enqueue(r.ID)
			}
		}
	}

	c.JSON(http.StatusOK, resp)
}

// ---- Work handlers ----

func (s *Server) listWorks(c *gin.Context) {
	resp, err := s.workService.ListWorks()
	if err != nil {
		internalError(c, "failed to list works", err)
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
		internalError(c, "failed to update work", err)
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
		internalError(c, "failed to delete work", err)
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
		internalError(c, "failed to list work books", err)
		return
	}
	if books == nil {
		books = []database.Book{}
	}
	c.JSON(http.StatusOK, gin.H{"items": books, "count": len(books)})
}

func (s *Server) listAuthors(c *gin.Context) {
	resp, err := s.authorSeriesService.ListAuthorsWithCounts()
	if err != nil {
		internalError(c, "failed to list authors", err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (s *Server) countAuthors(c *gin.Context) {
	count, err := database.GlobalStore.CountAuthors()
	if err != nil {
		internalError(c, "failed to count authors", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": count})
}

func (s *Server) listDuplicateAuthors(c *gin.Context) {
	if cached, ok := s.dedupCache.Get("author-duplicates"); ok {
		c.JSON(http.StatusOK, cached)
		return
	}

	// No cache — return empty with needs_refresh flag so frontend triggers async scan
	c.JSON(http.StatusOK, gin.H{"groups": []any{}, "count": 0, "needs_refresh": true})
}

func (s *Server) refreshDuplicateAuthors(c *gin.Context) {
	if operations.GlobalQueue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	store := database.GlobalStore
	opID := ulid.Make().String()
	op, err := store.CreateOperation(opID, "author-dedup-scan", nil)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		_ = progress.UpdateProgress(0, 100, "Fetching authors...")

		authors, err := store.GetAllAuthors()
		if err != nil {
			return err
		}
		_ = progress.UpdateProgress(10, 100, fmt.Sprintf("Loaded %d authors, fetching book counts...", len(authors)))

		bookCounts, err := store.GetAllAuthorBookCounts()
		if err != nil {
			return err
		}
		bookCountFn := func(authorID int) int { return bookCounts[authorID] }
		_ = progress.UpdateProgress(20, 100, "Finding duplicate authors...")

		progressFn := func(current, total int, message string) {
			// Map author comparison progress to 20-90% range
			pct := 20 + (current*70)/max(total, 1)
			_ = progress.UpdateProgress(pct, 100, message)
		}

		groups := FindDuplicateAuthors(authors, 0.9, bookCountFn, progressFn)

		// Filter out groups already reviewed by AI scans
		groups = s.filterReviewedAuthorGroups(groups)

		result := gin.H{"groups": groups, "count": len(groups)}
		s.dedupCache.SetWithTTL("author-duplicates", result, 30*time.Minute)

		_ = progress.UpdateProgress(100, 100, fmt.Sprintf("Found %d duplicate groups (after filtering reviewed)", len(groups)))
		return nil
	}

	if err := operations.GlobalQueue.Enqueue(opID, "author-dedup-scan", operations.PriorityNormal, operationFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	c.JSON(http.StatusAccepted, op)
}

// filterReviewedAuthorGroups removes author dedup groups where all author IDs
// have already been reviewed via AI scans (applied results with skip/split/merge).
func (s *Server) filterReviewedAuthorGroups(groups []AuthorDedupGroup) []AuthorDedupGroup {
	if s.aiScanStore == nil {
		return groups
	}
	applied, err := s.aiScanStore.GetAllAppliedResults()
	if err != nil || len(applied) == 0 {
		return groups
	}

	// Build set of reviewed author ID sets (key = sorted comma-joined IDs)
	reviewedSets := make(map[string]bool)
	for _, r := range applied {
		if len(r.Suggestion.AuthorIDs) < 2 {
			continue
		}
		ids := make([]int, len(r.Suggestion.AuthorIDs))
		copy(ids, r.Suggestion.AuthorIDs)
		sort.Ints(ids)
		parts := make([]string, len(ids))
		for i, id := range ids {
			parts[i] = strconv.Itoa(id)
		}
		reviewedSets[strings.Join(parts, ",")] = true
	}

	if len(reviewedSets) == 0 {
		return groups
	}

	// Filter: exclude groups whose author IDs match a reviewed set
	filtered := make([]AuthorDedupGroup, 0, len(groups))
	for _, g := range groups {
		ids := make([]int, 0, 1+len(g.Variants))
		ids = append(ids, g.Canonical.ID)
		for _, v := range g.Variants {
			ids = append(ids, v.ID)
		}
		sort.Ints(ids)
		parts := make([]string, len(ids))
		for i, id := range ids {
			parts[i] = strconv.Itoa(id)
		}
		key := strings.Join(parts, ",")
		if !reviewedSets[key] {
			filtered = append(filtered, g)
		}
	}
	return filtered
}

func (s *Server) reclassifyAuthorAsNarrator(c *gin.Context) {
	authorID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid author ID"})
		return
	}

	store := database.GlobalStore
	author, err := store.GetAuthorByID(authorID)
	if err != nil || author == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "author not found"})
		return
	}

	// Create or find narrator with same name
	narrator, err := store.GetNarratorByName(author.Name)
	if err != nil || narrator == nil {
		narrator, err = store.CreateNarrator(author.Name)
		if err != nil {
			internalError(c, "failed to create narrator", err)
			return
		}
	}

	// Get all books linked to this author
	books, err := store.GetBooksByAuthorIDWithRole(authorID)
	if err != nil {
		internalError(c, "failed to get author books", err)
		return
	}

	booksUpdated := 0
	for _, book := range books {
		// Remove author link
		bookAuthors, err := store.GetBookAuthors(book.ID)
		if err != nil {
			continue
		}
		var newAuthors []database.BookAuthor
		for _, ba := range bookAuthors {
			if ba.AuthorID != authorID {
				newAuthors = append(newAuthors, ba)
			}
		}
		if err := store.SetBookAuthors(book.ID, newAuthors); err != nil {
			continue
		}

		// Add narrator link if not already present
		bookNarrators, err := store.GetBookNarrators(book.ID)
		if err != nil {
			continue
		}
		hasNarrator := false
		for _, bn := range bookNarrators {
			if bn.NarratorID == narrator.ID {
				hasNarrator = true
				break
			}
		}
		if !hasNarrator {
			bookNarrators = append(bookNarrators, database.BookNarrator{
				BookID:     book.ID,
				NarratorID: narrator.ID,
				Role:       "narrator",
				Position:   len(bookNarrators),
			})
			if err := store.SetBookNarrators(book.ID, bookNarrators); err != nil {
				continue
			}
		}
		booksUpdated++
	}

	// Delete the author record
	if err := store.DeleteAuthor(authorID); err != nil {
		internalError(c, "failed to delete author", err)
		return
	}

	s.dedupCache.Invalidate("author-duplicates")
	c.JSON(http.StatusOK, gin.H{"narrator_id": narrator.ID, "books_updated": booksUpdated})
}

func (s *Server) renameAuthor(c *gin.Context) {
	authorID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid author ID"})
		return
	}

	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name must not be empty"})
		return
	}

	store := database.GlobalStore
	if err := store.UpdateAuthorName(authorID, name); err != nil {
		internalError(c, "failed to rename author", err)
		return
	}

	s.dedupCache.Invalidate("author-duplicates")
	c.JSON(http.StatusOK, gin.H{"id": authorID, "name": name})
}

// splitCompositeAuthor splits an author like "Author1 / Author2" or "Author1, Author2"
// into individual author records, relinking all books to each new author.
func (s *Server) splitCompositeAuthor(c *gin.Context) {
	authorID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid author ID"})
		return
	}

	store := database.GlobalStore
	author, err := store.GetAuthorByID(authorID)
	if err != nil || author == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "author not found"})
		return
	}

	// Optional: caller can provide explicit names to split into
	var req struct {
		Names []string `json:"names"`
	}
	_ = c.ShouldBindJSON(&req)

	// If no explicit names, auto-detect split
	names := req.Names
	if len(names) == 0 {
		names = SplitCompositeAuthorName(author.Name)
	}
	if len(names) <= 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "author name does not appear to be composite"})
		return
	}

	// Create or find each individual author
	var newAuthors []database.Author
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		existing, err := store.GetAuthorByName(name)
		if err == nil && existing != nil {
			newAuthors = append(newAuthors, *existing)
			continue
		}
		created, err := store.CreateAuthor(name)
		if err != nil {
			internalError(c, "failed to create author", err)
			return
		}
		newAuthors = append(newAuthors, *created)
	}

	// Get all books linked to the composite author
	books, err := store.GetBooksByAuthorIDWithRole(authorID)
	if err != nil {
		internalError(c, "failed to get author books", err)
		return
	}

	booksUpdated := 0
	for _, book := range books {
		bookAuthors, err := store.GetBookAuthors(book.ID)
		if err != nil {
			continue
		}

		// Find the role/position of the composite author entry
		role := "author"
		for _, ba := range bookAuthors {
			if ba.AuthorID == authorID {
				role = ba.Role
				break
			}
		}

		// Remove composite author, add individual authors
		var updated []database.BookAuthor
		for _, ba := range bookAuthors {
			if ba.AuthorID != authorID {
				updated = append(updated, ba)
			}
		}
		for i, na := range newAuthors {
			// Check not already linked
			alreadyLinked := false
			for _, ba := range updated {
				if ba.AuthorID == na.ID {
					alreadyLinked = true
					break
				}
			}
			if !alreadyLinked {
				updated = append(updated, database.BookAuthor{
					BookID:   book.ID,
					AuthorID: na.ID,
					Role:     role,
					Position: len(updated) + i,
				})
			}
		}
		if err := store.SetBookAuthors(book.ID, updated); err != nil {
			continue
		}
		booksUpdated++
	}

	// Delete the composite author
	if err := store.DeleteAuthor(authorID); err != nil {
		internalError(c, "failed to delete author", err)
		return
	}

	result := make([]gin.H, len(newAuthors))
	for i, a := range newAuthors {
		result[i] = gin.H{"id": a.ID, "name": a.Name}
	}

	s.dedupCache.Invalidate("author-duplicates")
	c.JSON(http.StatusOK, gin.H{"authors": result, "books_updated": booksUpdated})
}

// resolveProductionAuthor attempts to find real authors for books attributed to
// a production company by searching metadata sources by title only and optionally
// using AI cover art analysis.
func (s *Server) resolveProductionAuthor(c *gin.Context) {
	authorID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid author ID"})
		return
	}

	store := database.GlobalStore
	author, err := store.GetAuthorByID(authorID)
	if err != nil || author == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "author not found"})
		return
	}

	if !isProductionCompany(author.Name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("%q is not a recognized production company", author.Name)})
		return
	}

	opID := ulid.Make().String()
	op, err := store.CreateOperation(opID, "resolve-production-author", nil)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	prodAuthorName := author.Name
	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		books, err := store.GetBooksByAuthorIDWithRole(authorID)
		if err != nil {
			return fmt.Errorf("failed to get books: %w", err)
		}
		_ = progress.Log("info", fmt.Sprintf("Resolving %d books for production company %q", len(books), prodAuthorName), nil)

		resolved := 0
		failed := 0
		for i, book := range books {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			_ = progress.UpdateProgress(i, len(books), fmt.Sprintf("Processing %d/%d: %s", i+1, len(books), book.Title))

			// Try metadata fetch by title only
			resp, fetchErr := s.metadataFetchService.FetchMetadataForBookByTitle(book.ID)
			if fetchErr == nil && resp != nil && resp.Book != nil && resp.Book.AuthorID != nil {
				// Check if the found author is different from the production company
				newAuthor, _ := store.GetAuthorByID(*resp.Book.AuthorID)
				if newAuthor != nil && !isProductionCompany(newAuthor.Name) {
					_ = progress.Log("info", fmt.Sprintf("Resolved %q → author %q (source: %s)", book.Title, newAuthor.Name, resp.Source), nil)
					// Reclassify production company as publisher
					if book.Publisher == nil || *book.Publisher == "" {
						pub := prodAuthorName
						book.Publisher = &pub
						store.UpdateBook(book.ID, &database.Book{Publisher: &pub})
					}
					resolved++
					continue
				}
			}

			// If metadata failed and AI is enabled, try cover art analysis
			aiParser := newAIParser(config.AppConfig.OpenAIAPIKey, config.AppConfig.EnableAIParsing)
			if aiParser.IsEnabled() && book.FilePath != "" {
				imgData, mime, imgErr := metadata.ExtractCoverArtBytes(book.FilePath)
				if imgErr == nil && len(imgData) > 0 {
					parsed, aiErr := aiParser.ParseCoverArt(ctx, imgData, mime)
					if aiErr == nil && parsed != nil && parsed.Author != "" && parsed.Confidence != "low" {
						_ = progress.Log("info", fmt.Sprintf("AI cover analysis for %q found author: %q (confidence: %s)", book.Title, parsed.Author, parsed.Confidence), nil)
						// Look up or create the discovered author
						existing, _ := store.GetAuthorByName(parsed.Author)
						if existing == nil {
							existing, _ = store.CreateAuthor(parsed.Author)
						}
						if existing != nil {
							aid := existing.ID
							book.AuthorID = &aid
							store.UpdateBook(book.ID, &database.Book{AuthorID: &aid})
							// Update book_authors
							bookAuthors, _ := store.GetBookAuthors(book.ID)
							var updated []database.BookAuthor
							for _, ba := range bookAuthors {
								if ba.AuthorID != authorID {
									updated = append(updated, ba)
								}
							}
							updated = append(updated, database.BookAuthor{
								BookID:   book.ID,
								AuthorID: existing.ID,
								Role:     "author",
								Position: 0,
							})
							store.SetBookAuthors(book.ID, updated)
							resolved++
							continue
						}
					}
				}
			}

			failed++
			_ = progress.Log("debug", fmt.Sprintf("Could not resolve author for %q", book.Title), nil)
		}

		if s.dedupCache != nil {
			s.dedupCache.Invalidate("author-duplicates")
		}

		resultMsg := fmt.Sprintf("Resolved %d/%d books for %q (%d unresolved)", resolved, len(books), prodAuthorName, failed)
		_ = progress.Log("info", resultMsg, nil)
		_ = progress.UpdateProgress(len(books), len(books), resultMsg)
		return nil
	}

	if err := operations.GlobalQueue.Enqueue(opID, "resolve-production-author", operations.PriorityNormal, operationFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"operation": op})
}

func (s *Server) mergeAuthors(c *gin.Context) {
	var req struct {
		KeepID   int   `json:"keep_id" binding:"required"`
		MergeIDs []int `json:"merge_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.MergeIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "merge_ids must not be empty"})
		return
	}

	store := database.GlobalStore
	keepAuthor, err := store.GetAuthorByID(req.KeepID)
	if err != nil || keepAuthor == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "keep author not found"})
		return
	}

	if operations.GlobalQueue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	opID := ulid.Make().String()
	detail := fmt.Sprintf("merge-authors:keep=%d,merge=%v", req.KeepID, req.MergeIDs)
	op, err := store.CreateOperation(opID, "author-merge", &detail)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	keepID := req.KeepID
	mergeIDs := req.MergeIDs
	keepName := keepAuthor.Name

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		_ = progress.Log("info", fmt.Sprintf("Merging %d author(s) into \"%s\"", len(mergeIDs), keepName), nil)
		_ = progress.UpdateProgress(0, len(mergeIDs), "Starting author merge...")

		merged := 0
		var mergeErrors []string
		for i, mergeID := range mergeIDs {
			if progress.IsCanceled() {
				return fmt.Errorf("cancelled")
			}
			if mergeID == keepID {
				continue
			}
			books, err := store.GetBooksByAuthorIDWithRole(mergeID)
			if err != nil {
				mergeErrors = append(mergeErrors, fmt.Sprintf("failed to get books for author %d: %v", mergeID, err))
				continue
			}

			mergeAuthor, _ := store.GetAuthorByID(mergeID)
			mergeAuthorName := ""
			if mergeAuthor != nil {
				mergeAuthorName = mergeAuthor.Name
			}

			for _, book := range books {
				bookAuthors, err := store.GetBookAuthors(book.ID)
				if err != nil {
					continue
				}
				hasKeep := false
				for _, ba := range bookAuthors {
					if ba.AuthorID == keepID {
						hasKeep = true
						break
					}
				}
				var newAuthors []database.BookAuthor
				for _, ba := range bookAuthors {
					if ba.AuthorID == mergeID {
						if !hasKeep {
							ba.AuthorID = keepID
							newAuthors = append(newAuthors, ba)
							hasKeep = true
						}
					} else {
						newAuthors = append(newAuthors, ba)
					}
				}
				if err := store.SetBookAuthors(book.ID, newAuthors); err != nil {
					mergeErrors = append(mergeErrors, fmt.Sprintf("failed to update book %s: %v", book.ID, err))
				} else {
					_ = store.CreateOperationChange(&database.OperationChange{
						ID:          ulid.Make().String(),
						OperationID: opID,
						BookID:      book.ID,
						ChangeType:  "author_reassign",
						FieldName:   "book_authors",
						OldValue:    fmt.Sprintf("author_id:%d (%s)", mergeID, mergeAuthorName),
						NewValue:    fmt.Sprintf("author_id:%d (%s)", keepID, keepName),
					})
				}
			}

			if err := store.DeleteAuthor(mergeID); err != nil {
				mergeErrors = append(mergeErrors, fmt.Sprintf("failed to delete author %d: %v", mergeID, err))
			} else {
				_ = store.CreateAuthorTombstone(mergeID, keepID)
				_ = store.CreateOperationChange(&database.OperationChange{
					ID:          ulid.Make().String(),
					OperationID: opID,
					BookID:      "",
					ChangeType:  "author_delete",
					FieldName:   "author",
					OldValue:    fmt.Sprintf("%d:%s", mergeID, mergeAuthorName),
					NewValue:    fmt.Sprintf("merged_into:%d:%s", keepID, keepName),
				})
				merged++
			}

			_ = progress.UpdateProgress(i+1, len(mergeIDs),
				fmt.Sprintf("Merged %d/%d authors", i+1, len(mergeIDs)))
		}

		resultMsg := fmt.Sprintf("Author merge complete: merged %d, %d errors", merged, len(mergeErrors))
		_ = progress.Log("info", resultMsg, nil)
		if len(mergeErrors) > 0 {
			errDetail := strings.Join(mergeErrors[:min(len(mergeErrors), 10)], "; ")
			_ = progress.Log("warn", fmt.Sprintf("Errors: %s", errDetail), nil)
		}
		s.dedupCache.InvalidateAll()
		return nil
	}

	if err := operations.GlobalQueue.Enqueue(op.ID, "author-merge", operations.PriorityNormal, operationFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	c.JSON(http.StatusAccepted, op)
}

func (s *Server) updateSeriesName(c *gin.Context) {
	idStr := c.Param("id")
	id := 0
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid series id"})
		return
	}
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name cannot be empty"})
		return
	}
	store := database.GlobalStore
	if err := store.UpdateSeriesName(id, name); err != nil {
		internalError(c, "failed to update series", err)
		return
	}
	s.dedupCache.Invalidate("series-duplicates")
	series, _ := store.GetSeriesByID(id)
	c.JSON(http.StatusOK, series)
}

// mergeSeriesGroup merges multiple series into one, reassigning all books.
func (s *Server) mergeSeriesGroup(c *gin.Context) {
	var req struct {
		KeepID     int    `json:"keep_id" binding:"required"`
		MergeIDs   []int  `json:"merge_ids" binding:"required"`
		CustomName string `json:"custom_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.MergeIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "merge_ids must not be empty"})
		return
	}

	store := database.GlobalStore
	keepSeries, err := store.GetSeriesByID(req.KeepID)
	if err != nil || keepSeries == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "keep series not found"})
		return
	}

	if operations.GlobalQueue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	opID := ulid.Make().String()
	detail := fmt.Sprintf("merge-series:keep=%d,merge=%v", req.KeepID, req.MergeIDs)
	op, err := store.CreateOperation(opID, "series-merge", &detail)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	keepID := req.KeepID
	mergeIDs := req.MergeIDs
	customName := strings.TrimSpace(req.CustomName)
	keepName := keepSeries.Name
	if customName != "" {
		keepName = customName
	}

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		// Rename the kept series if a custom name was provided
		if customName != "" {
			oldName := keepSeries.Name
			if err := store.UpdateSeriesName(keepID, customName); err != nil {
				return fmt.Errorf("failed to rename series to %q: %w", customName, err)
			}
			_ = store.CreateOperationChange(&database.OperationChange{
				ID:          ulid.Make().String(),
				OperationID: opID,
				ChangeType:  "metadata_update",
				FieldName:   "series_name",
				OldValue:    oldName,
				NewValue:    customName,
			})
			_ = progress.Log("info", fmt.Sprintf("Renamed series from %q to %q", oldName, customName), nil)
		}

		_ = progress.Log("info", fmt.Sprintf("Merging %d series into \"%s\"", len(mergeIDs), keepName), nil)
		_ = progress.UpdateProgress(0, len(mergeIDs), "Starting series merge...")

		// Collect all unique author IDs from all series being merged (including keep)
		allAuthorIDs := make(map[int]bool)
		allSeriesIDs := append([]int{keepID}, mergeIDs...)
		for _, sid := range allSeriesIDs {
			s, err := store.GetSeriesByID(sid)
			if err == nil && s != nil && s.AuthorID != nil {
				allAuthorIDs[*s.AuthorID] = true
			}
		}

		merged := 0
		var mergeErrors []string
		for i, mergeID := range mergeIDs {
			if progress.IsCanceled() {
				return fmt.Errorf("cancelled")
			}
			if mergeID == keepID {
				continue
			}
			books, err := store.GetBooksBySeriesID(mergeID)
			if err != nil {
				mergeErrors = append(mergeErrors, fmt.Sprintf("failed to get books for series %d: %v", mergeID, err))
				continue
			}

			for _, book := range books {
				oldSeriesID := ""
				if book.SeriesID != nil {
					oldSeriesID = fmt.Sprintf("%d", *book.SeriesID)
				}
				book.SeriesID = &keepID
				if _, err := store.UpdateBook(book.ID, &book); err != nil {
					mergeErrors = append(mergeErrors, fmt.Sprintf("failed to reassign book %s: %v", book.ID, err))
				} else {
					_ = store.CreateOperationChange(&database.OperationChange{
						ID:          ulid.Make().String(),
						OperationID: opID,
						BookID:      book.ID,
						ChangeType:  "metadata_update",
						FieldName:   "series_id",
						OldValue:    oldSeriesID,
						NewValue:    fmt.Sprintf("%d", keepID),
					})
				}
			}

			// Record the series deletion
			mergeSeries, _ := store.GetSeriesByID(mergeID)
			mergeSeriesName := ""
			if mergeSeries != nil {
				mergeSeriesName = mergeSeries.Name
			}
			if err := store.DeleteSeries(mergeID); err != nil {
				mergeErrors = append(mergeErrors, fmt.Sprintf("failed to delete series %d: %v", mergeID, err))
			} else {
				merged++
				_ = store.CreateOperationChange(&database.OperationChange{
					ID:          ulid.Make().String(),
					OperationID: opID,
					BookID:      "",
					ChangeType:  "series_delete",
					FieldName:   "series",
					OldValue:    fmt.Sprintf("%d:%s", mergeID, mergeSeriesName),
					NewValue:    fmt.Sprintf("merged_into:%d", keepID),
				})
			}

			_ = progress.UpdateProgress(i+1, len(mergeIDs),
				fmt.Sprintf("Merged %d/%d series", i+1, len(mergeIDs)))
		}

		// Link all books in the kept series to all unique authors
		if len(allAuthorIDs) > 1 {
			_ = progress.Log("info", fmt.Sprintf("Linking books to %d authors", len(allAuthorIDs)), nil)
			allBooks, err := store.GetBooksBySeriesID(keepID)
			if err == nil {
				for _, book := range allBooks {
					existing, _ := store.GetBookAuthors(book.ID)
					existingMap := make(map[int]bool)
					for _, ba := range existing {
						existingMap[ba.AuthorID] = true
					}
					authors := existing
					var addedAuthors []int
					for aid := range allAuthorIDs {
						if !existingMap[aid] {
							authors = append(authors, database.BookAuthor{BookID: book.ID, AuthorID: aid})
							addedAuthors = append(addedAuthors, aid)
						}
					}
					if len(authors) > len(existing) {
						if err := store.SetBookAuthors(book.ID, authors); err != nil {
							mergeErrors = append(mergeErrors, fmt.Sprintf("failed to set authors for book %s: %v", book.ID, err))
						} else {
							_ = store.CreateOperationChange(&database.OperationChange{
								ID:          ulid.Make().String(),
								OperationID: opID,
								BookID:      book.ID,
								ChangeType:  "author_link",
								FieldName:   "book_authors",
								OldValue:    fmt.Sprintf("%d authors", len(existing)),
								NewValue:    fmt.Sprintf("%d authors (added %v)", len(authors), addedAuthors),
							})
						}
					}
				}
			}
		}

		resultMsg := fmt.Sprintf("Series merge complete: merged %d, %d errors", merged, len(mergeErrors))
		_ = progress.Log("info", resultMsg, nil)
		if len(mergeErrors) > 0 {
			errDetail := strings.Join(mergeErrors[:min(len(mergeErrors), 10)], "; ")
			_ = progress.Log("warn", fmt.Sprintf("Errors: %s", errDetail), nil)
		}
		s.dedupCache.InvalidateAll()
		return nil
	}

	if err := operations.GlobalQueue.Enqueue(op.ID, "series-merge", operations.PriorityNormal, operationFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	c.JSON(http.StatusAccepted, op)
}

func (s *Server) mergeBooks(c *gin.Context) {
	var req struct {
		KeepID   string   `json:"keep_id" binding:"required"`
		MergeIDs []string `json:"merge_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.MergeIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "merge_ids must not be empty"})
		return
	}

	store := database.GlobalStore
	keepBook, err := store.GetBookByID(req.KeepID)
	if err != nil || keepBook == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "keep book not found"})
		return
	}

	if operations.GlobalQueue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	opID := ulid.Make().String()
	detail := fmt.Sprintf("merge-books:keep=%s,merge=%d", req.KeepID, len(req.MergeIDs))
	op, err := store.CreateOperation(opID, "book-merge", &detail)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	keepID := req.KeepID
	mergeIDs := req.MergeIDs

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		_ = progress.Log("info", fmt.Sprintf("Merging %d book(s) into \"%s\"", len(mergeIDs), keepBook.Title), nil)
		_ = progress.UpdateProgress(0, len(mergeIDs), "Starting book merge...")

		kBook, err := store.GetBookByID(keepID)
		if err != nil || kBook == nil {
			return fmt.Errorf("keep book %s not found", keepID)
		}

		merged := 0
		var mergeErrors []string
		for i, mergeID := range mergeIDs {
			if progress.IsCanceled() {
				return fmt.Errorf("cancelled")
			}
			if mergeID == keepID {
				continue
			}
			mergeBook, err := store.GetBookByID(mergeID)
			if err != nil || mergeBook == nil {
				mergeErrors = append(mergeErrors, fmt.Sprintf("book %s not found", mergeID))
				continue
			}

			// Transfer useful metadata
			if (kBook.ITunesPersistentID == nil || *kBook.ITunesPersistentID == "") &&
				mergeBook.ITunesPersistentID != nil && *mergeBook.ITunesPersistentID != "" {
				kBook.ITunesPersistentID = mergeBook.ITunesPersistentID
			}
			if kBook.ITunesPlayCount == nil && mergeBook.ITunesPlayCount != nil {
				kBook.ITunesPlayCount = mergeBook.ITunesPlayCount
			}
			if kBook.ITunesRating == nil && mergeBook.ITunesRating != nil {
				kBook.ITunesRating = mergeBook.ITunesRating
			}
			if kBook.ITunesDateAdded == nil && mergeBook.ITunesDateAdded != nil {
				kBook.ITunesDateAdded = mergeBook.ITunesDateAdded
			}
			if kBook.ITunesLastPlayed == nil && mergeBook.ITunesLastPlayed != nil {
				kBook.ITunesLastPlayed = mergeBook.ITunesLastPlayed
			}
			if kBook.ITunesBookmark == nil && mergeBook.ITunesBookmark != nil {
				kBook.ITunesBookmark = mergeBook.ITunesBookmark
			}

			if err := store.DeleteBook(mergeID); err != nil {
				mergeErrors = append(mergeErrors, fmt.Sprintf("failed to delete book %s: %v", mergeID, err))
			} else {
				_ = store.CreateOperationChange(&database.OperationChange{
					ID:          ulid.Make().String(),
					OperationID: opID,
					BookID:      mergeID,
					ChangeType:  "book_delete",
					FieldName:   "book",
					OldValue:    fmt.Sprintf("%s (%s)", mergeBook.Title, mergeBook.FilePath),
					NewValue:    fmt.Sprintf("merged_into:%s", keepID),
				})
				merged++
			}

			_ = progress.UpdateProgress(i+1, len(mergeIDs),
				fmt.Sprintf("Merged %d/%d books", i+1, len(mergeIDs)))
		}

		if _, err := store.UpdateBook(kBook.ID, kBook); err != nil {
			mergeErrors = append(mergeErrors, fmt.Sprintf("failed to update keep book: %v", err))
		}

		resultMsg := fmt.Sprintf("Book merge complete: merged %d, %d errors", merged, len(mergeErrors))
		_ = progress.Log("info", resultMsg, nil)
		s.dedupCache.InvalidateAll()
		return nil
	}

	if err := operations.GlobalQueue.Enqueue(op.ID, "book-merge", operations.PriorityNormal, operationFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	c.JSON(http.StatusAccepted, op)
}

// extractSeriesNameForDedup tries to extract the actual series name from patterns like
// "Book Title: Series Name" or "Series Name, Book 3". Returns the suggested
// series name and whether a suggestion was made.
func extractSeriesNameForDedup(name string) (string, bool) {
	// Pattern: "Book Title: Series Name" — the part after colon is often the series
	if idx := strings.LastIndex(name, ": "); idx > 0 {
		after := strings.TrimSpace(name[idx+2:])
		before := strings.TrimSpace(name[:idx])
		// Heuristic: if after-colon part looks like a series (shorter, no numbers at end),
		// prefer it. If before-colon part looks like a series, prefer that.
		// Series names are typically the longer, more generic portion.
		if len(after) > 3 && len(after) < len(before) {
			return after, true
		}
		if len(before) > 3 && len(before) < len(after) {
			return before, true
		}
	}
	// Pattern: "Series Name, Book N" or "Series Name, Vol N"
	commaPatterns := []string{", book ", ", vol ", ", volume ", ", #"}
	lower := strings.ToLower(name)
	for _, pat := range commaPatterns {
		if idx := strings.Index(lower, pat); idx > 0 {
			return strings.TrimSpace(name[:idx]), true
		}
	}
	return "", false
}

func (s *Server) listSeriesDuplicates(c *gin.Context) {
	if cached, ok := s.dedupCache.Get("series-duplicates"); ok {
		c.JSON(http.StatusOK, cached)
		return
	}

	// No cache — return empty with needs_refresh flag so frontend triggers async scan
	c.JSON(http.StatusOK, gin.H{"groups": []any{}, "count": 0, "total_series": 0, "needs_refresh": true})
}

func (s *Server) refreshSeriesDuplicates(c *gin.Context) {
	if operations.GlobalQueue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	store := database.GlobalStore
	opID := ulid.Make().String()
	op, err := store.CreateOperation(opID, "series-dedup-scan", nil)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		_ = progress.UpdateProgress(0, 100, "Fetching series...")

		allSeries, err := store.GetAllSeries()
		if err != nil {
			return err
		}
		_ = progress.UpdateProgress(10, 100, fmt.Sprintf("Loaded %d series, grouping...", len(allSeries)))

		// Reuse the same logic from listSeriesDuplicates
		isGarbageSeries := func(name string) bool {
			trimmed := strings.TrimSpace(name)
			if len(trimmed) == 0 {
				return true
			}
			for _, r := range trimmed {
				if r < '0' || r > '9' {
					return false
				}
			}
			return true
		}

		exactGroups := make(map[string][]database.Series)
		for _, s := range allSeries {
			if isGarbageSeries(s.Name) {
				continue
			}
			key := strings.ToLower(strings.TrimSpace(s.Name))
			exactGroups[key] = append(exactGroups[key], s)
		}

		_ = progress.UpdateProgress(20, 100, "Building author lookup...")

		type seriesBookSummary struct {
			ID       string `json:"id"`
			Title    string `json:"title"`
			CoverURL string `json:"cover_url,omitempty"`
		}
		type seriesWithBooks struct {
			database.Series
			Books      []seriesBookSummary `json:"books"`
			AuthorName string              `json:"author_name,omitempty"`
		}

		allAuthors, _ := store.GetAllAuthors()
		authorNameMap := make(map[int]string, len(allAuthors))
		for _, a := range allAuthors {
			authorNameMap[a.ID] = a.Name
		}

		type seriesDupGroup struct {
			Name          string            `json:"name"`
			Count         int               `json:"count"`
			Series        []seriesWithBooks `json:"series"`
			SuggestedName string            `json:"suggested_name,omitempty"`
			MatchType     string            `json:"match_type"`
		}

		enrichSeries := func(seriesList []database.Series) []seriesWithBooks {
			result := make([]seriesWithBooks, 0, len(seriesList))
			for _, s := range seriesList {
				authorName := ""
				if s.AuthorID != nil {
					authorName = authorNameMap[*s.AuthorID]
				}
				sw := seriesWithBooks{Series: s, AuthorName: authorName}
				if books, err := store.GetBooksBySeriesID(s.ID); err == nil {
					limit := 5
					if len(books) < limit {
						limit = len(books)
					}
					for _, b := range books[:limit] {
						cover := ""
						if b.CoverURL != nil {
							cover = *b.CoverURL
						}
						sw.Books = append(sw.Books, seriesBookSummary{
							ID:       b.ID,
							Title:    b.Title,
							CoverURL: cover,
						})
					}
				}
				result = append(result, sw)
			}
			return result
		}

		var result []seriesDupGroup
		seen := make(map[int]bool)

		_ = progress.UpdateProgress(30, 100, "Finding exact duplicates...")

		groupKeys := make([]string, 0, len(exactGroups))
		for k := range exactGroups {
			groupKeys = append(groupKeys, k)
		}

		processed := 0
		totalGroups := len(groupKeys)
		for _, k := range groupKeys {
			group := exactGroups[k]
			if len(group) < 2 {
				continue
			}
			for _, s := range group {
				seen[s.ID] = true
			}
			suggested, _ := extractSeriesNameForDedup(group[0].Name)
			result = append(result, seriesDupGroup{
				Name:          group[0].Name,
				Count:         len(group),
				Series:        enrichSeries(group),
				SuggestedName: suggested,
				MatchType:     "exact",
			})
			processed++
			if processed%10 == 0 {
				pct := 30 + (processed*40)/max(totalGroups, 1)
				_ = progress.UpdateProgress(min(pct, 70), 100, fmt.Sprintf("Processing groups... (%d/%d)", processed, totalGroups))
			}
		}

		_ = progress.UpdateProgress(70, 100, "Finding sub-series patterns...")

		seriesByNormalizedName := make(map[string][]database.Series)
		for _, s := range allSeries {
			seriesByNormalizedName[strings.ToLower(strings.TrimSpace(s.Name))] = append(
				seriesByNormalizedName[strings.ToLower(strings.TrimSpace(s.Name))], s)
		}

		for _, s := range allSeries {
			if seen[s.ID] || isGarbageSeries(s.Name) {
				continue
			}
			suggested, ok := extractSeriesNameForDedup(s.Name)
			if !ok {
				continue
			}
			suggestedKey := strings.ToLower(strings.TrimSpace(suggested))
			if matches, exists := seriesByNormalizedName[suggestedKey]; exists {
				group := []database.Series{s}
				seen[s.ID] = true
				for _, m := range matches {
					if !seen[m.ID] {
						group = append(group, m)
						seen[m.ID] = true
					}
				}
				if len(group) >= 2 {
					result = append(result, seriesDupGroup{
						Name:          s.Name,
						Count:         len(group),
						Series:        enrichSeries(group),
						SuggestedName: suggested,
						MatchType:     "subseries",
					})
				}
			}
		}

		resp := gin.H{"groups": result, "count": len(result), "total_series": len(allSeries)}
		s.dedupCache.Set("series-duplicates", resp)

		_ = progress.UpdateProgress(100, 100, fmt.Sprintf("Found %d duplicate groups", len(result)))
		return nil
	}

	if err := operations.GlobalQueue.Enqueue(opID, "series-dedup-scan", operations.PriorityNormal, operationFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	c.JSON(http.StatusAccepted, op)
}

// validateDedupEntry searches metadata sources (OpenLibrary, Audible, etc.) to validate
// a series name, author name, or book title during dedup review.
func (s *Server) validateDedupEntry(c *gin.Context) {
	var req struct {
		Query string `json:"query" binding:"required"`
		Type  string `json:"type"` // "series", "author", "book"
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query is required"})
		return
	}
	if req.Type == "" {
		req.Type = "series"
	}

	chain := s.metadataFetchService.BuildSourceChain()
	if len(chain) == 0 {
		c.JSON(http.StatusOK, gin.H{"results": []interface{}{}, "message": "no metadata sources configured"})
		return
	}

	type validationResult struct {
		Source         string `json:"source"`
		Title          string `json:"title"`
		Author         string `json:"author"`
		Series         string `json:"series,omitempty"`
		SeriesPosition string `json:"series_position,omitempty"`
		CoverURL       string `json:"cover_url,omitempty"`
		ISBN           string `json:"isbn,omitempty"`
	}

	var results []validationResult
	for _, src := range chain {
		matches, err := src.SearchByTitle(req.Query)
		if err != nil {
			continue
		}
		for _, m := range matches {
			r := validationResult{
				Source:         src.Name(),
				Title:          m.Title,
				Author:         m.Author,
				Series:         m.Series,
				SeriesPosition: m.SeriesPosition,
				CoverURL:       m.CoverURL,
				ISBN:           m.ISBN,
			}
			// For series validation, prioritize results that have series info
			if req.Type == "series" && m.Series == "" {
				continue
			}
			results = append(results, r)
		}
		// Limit total results
		if len(results) >= 20 {
			results = results[:20]
			break
		}
	}

	if results == nil {
		results = []validationResult{}
	}
	c.JSON(http.StatusOK, gin.H{"results": results, "query": req.Query, "type": req.Type})
}

func (s *Server) deduplicateSeriesHandler(c *gin.Context) {
	store := database.GlobalStore
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	if operations.GlobalQueue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	opID := ulid.Make().String()
	detail := "series-deduplicate"
	op, err := store.CreateOperation(opID, "series-dedup", &detail)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		_ = progress.Log("info", "Starting series deduplication...", nil)

		allSeries, err := store.GetAllSeries()
		if err != nil {
			return fmt.Errorf("failed to get series: %w", err)
		}

		_ = progress.UpdateProgress(0, len(allSeries), fmt.Sprintf("Scanning %d series for duplicates...", len(allSeries)))

		// Group by normalized name only
		groups := make(map[string][]database.Series)
		for _, s := range allSeries {
			key := strings.ToLower(strings.TrimSpace(s.Name))
			groups[key] = append(groups[key], s)
		}

		// Count total duplicate groups
		var dupGroups [][]database.Series
		for _, group := range groups {
			if len(group) >= 2 {
				dupGroups = append(dupGroups, group)
			}
		}

		msg := fmt.Sprintf("Found %d duplicate groups to merge", len(dupGroups))
		_ = progress.Log("info", msg, nil)
		_ = progress.UpdateProgress(0, len(dupGroups), msg)

		totalMerged := 0
		var mergeErrors []string
		for gi, group := range dupGroups {
			if progress.IsCanceled() {
				_ = progress.Log("warn", "Operation cancelled by user", nil)
				return fmt.Errorf("cancelled")
			}

			keepIdx := 0
			for i, s := range group {
				if s.AuthorID != nil && group[keepIdx].AuthorID == nil {
					keepIdx = i
				} else if (s.AuthorID != nil) == (group[keepIdx].AuthorID != nil) && s.ID < group[keepIdx].ID {
					keepIdx = i
				}
			}
			keepID := group[keepIdx].ID

			for i, s := range group {
				if i == keepIdx {
					continue
				}
				books, err := store.GetBooksBySeriesID(s.ID)
				if err != nil {
					mergeErrors = append(mergeErrors, fmt.Sprintf("failed to get books for series %d: %v", s.ID, err))
					continue
				}
				for _, book := range books {
					book.SeriesID = &keepID
					if _, err := store.UpdateBook(book.ID, &book); err != nil {
						mergeErrors = append(mergeErrors, fmt.Sprintf("failed to reassign book %s: %v", book.ID, err))
					}
				}
				if err := store.DeleteSeries(s.ID); err != nil {
					mergeErrors = append(mergeErrors, fmt.Sprintf("failed to delete series %d: %v", s.ID, err))
				} else {
					totalMerged++
				}
			}

			_ = progress.UpdateProgress(gi+1, len(dupGroups),
				fmt.Sprintf("Merged %d/%d groups (%d series merged)", gi+1, len(dupGroups), totalMerged))
		}

		resultMsg := fmt.Sprintf("Series deduplication complete: merged %d duplicates, %d errors", totalMerged, len(mergeErrors))
		_ = progress.Log("info", resultMsg, nil)
		if len(mergeErrors) > 0 {
			errDetail := strings.Join(mergeErrors[:min(len(mergeErrors), 10)], "; ")
			_ = progress.Log("warn", fmt.Sprintf("Merge errors: %s", errDetail), nil)
		}
		s.dedupCache.InvalidateAll()
		return nil
	}

	if err := operations.GlobalQueue.Enqueue(op.ID, "series-dedup", operations.PriorityNormal, operationFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	c.JSON(http.StatusAccepted, op)
}

// seriesPruneResult holds the result of a series prune operation.
type seriesPruneResult struct {
	DuplicatesMerged int `json:"duplicates_merged"`
	OrphansDeleted   int `json:"orphans_deleted"`
	TotalCleaned     int `json:"total_cleaned"`
}

// seriesPrunePreviewGroup describes a duplicate group or orphan for the preview endpoint.
type seriesPrunePreviewGroup struct {
	Name        string `json:"name"`
	CanonicalID int    `json:"canonical_id"`
	MergeIDs    []int  `json:"merge_ids"`
	BookCount   int    `json:"book_count"`
	Type        string `json:"type"` // "duplicate" or "orphan"
}

// seriesPrunePreviewResult holds the dry-run result.
type seriesPrunePreviewResult struct {
	Groups         []seriesPrunePreviewGroup `json:"groups"`
	DuplicateCount int                       `json:"duplicate_count"`
	OrphanCount    int                       `json:"orphan_count"`
	TotalCount     int                       `json:"total_count"`
}

// computeSeriesPrunePreview builds the preview of what a series prune would do.
func computeSeriesPrunePreview(store database.Store) (*seriesPrunePreviewResult, error) {
	allSeries, err := store.GetAllSeries()
	if err != nil {
		return nil, fmt.Errorf("failed to get series: %w", err)
	}

	// Group by LOWER(TRIM(name)) + author_id
	type groupKey struct {
		name     string
		authorID int // 0 means nil
	}
	groups := make(map[groupKey][]database.Series)
	for _, s := range allSeries {
		aid := 0
		if s.AuthorID != nil {
			aid = *s.AuthorID
		}
		key := groupKey{name: strings.ToLower(strings.TrimSpace(s.Name)), authorID: aid}
		groups[key] = append(groups[key], s)
	}

	result := &seriesPrunePreviewResult{}

	// Find duplicate groups (>1 entry with same normalized name + author_id)
	for _, group := range groups {
		if len(group) < 2 {
			continue
		}

		// Pick canonical: most books attached, then lowest ID
		canonicalIdx := 0
		canonicalBookCount := 0
		for i, s := range group {
			books, err := store.GetBooksBySeriesID(s.ID)
			if err != nil {
				continue
			}
			bc := len(books)
			if bc > canonicalBookCount || (bc == canonicalBookCount && s.ID < group[canonicalIdx].ID) {
				canonicalIdx = i
				canonicalBookCount = bc
			}
		}

		var mergeIDs []int
		totalBooks := 0
		for i, s := range group {
			if i == canonicalIdx {
				continue
			}
			mergeIDs = append(mergeIDs, s.ID)
			books, _ := store.GetBooksBySeriesID(s.ID)
			totalBooks += len(books)
		}
		books, _ := store.GetBooksBySeriesID(group[canonicalIdx].ID)
		totalBooks += len(books)

		result.Groups = append(result.Groups, seriesPrunePreviewGroup{
			Name:        group[canonicalIdx].Name,
			CanonicalID: group[canonicalIdx].ID,
			MergeIDs:    mergeIDs,
			BookCount:   totalBooks,
			Type:        "duplicate",
		})
		result.DuplicateCount += len(mergeIDs)
	}

	// Find orphan series with 0 books
	for _, s := range allSeries {
		books, err := store.GetBooksBySeriesID(s.ID)
		if err != nil {
			continue
		}
		if len(books) == 0 {
			result.Groups = append(result.Groups, seriesPrunePreviewGroup{
				Name:        s.Name,
				CanonicalID: s.ID,
				MergeIDs:    nil,
				BookCount:   0,
				Type:        "orphan",
			})
			result.OrphanCount++
		}
	}

	result.TotalCount = result.DuplicateCount + result.OrphanCount
	return result, nil
}

func (s *Server) seriesPrunePreview(c *gin.Context) {
	store := database.GlobalStore
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	preview, err := computeSeriesPrunePreview(store)
	if err != nil {
		internalError(c, "failed to compute series prune preview", err)
		return
	}

	c.JSON(http.StatusOK, preview)
}

func (s *Server) seriesPrune(c *gin.Context) {
	store := database.GlobalStore
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	if operations.GlobalQueue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	opID := ulid.Make().String()
	detail := "series-prune"
	op, err := store.CreateOperation(opID, "series-prune", &detail)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		return s.executeSeriesPrune(ctx, store, progress, op.ID)
	}

	if err := operations.GlobalQueue.Enqueue(op.ID, "series-prune", operations.PriorityNormal, operationFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	c.JSON(http.StatusAccepted, op)
}

// executeSeriesPrune performs the actual series prune logic (used by both HTTP handler and scheduler).
func (s *Server) executeSeriesPrune(ctx context.Context, store database.Store, progress operations.ProgressReporter, operationID string) error {
	_ = progress.Log("info", "Starting series auto-prune...", nil)

	allSeries, err := store.GetAllSeries()
	if err != nil {
		return fmt.Errorf("failed to get series: %w", err)
	}

	_ = progress.UpdateProgress(0, len(allSeries), fmt.Sprintf("Scanning %d series...", len(allSeries)))

	// Group by LOWER(TRIM(name)) + author_id
	type groupKey struct {
		name     string
		authorID int
	}
	groups := make(map[groupKey][]database.Series)
	for _, s := range allSeries {
		aid := 0
		if s.AuthorID != nil {
			aid = *s.AuthorID
		}
		key := groupKey{name: strings.ToLower(strings.TrimSpace(s.Name)), authorID: aid}
		groups[key] = append(groups[key], s)
	}

	// Phase 1: Merge duplicates
	totalMerged := 0
	var mergeErrors []string
	dupGroupCount := 0

	for _, group := range groups {
		if len(group) < 2 {
			continue
		}
		dupGroupCount++

		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Pick canonical: most books, then lowest ID
		canonicalIdx := 0
		canonicalBookCount := 0
		for i, s := range group {
			books, err := store.GetBooksBySeriesID(s.ID)
			if err != nil {
				continue
			}
			bc := len(books)
			if bc > canonicalBookCount || (bc == canonicalBookCount && s.ID < group[canonicalIdx].ID) {
				canonicalIdx = i
				canonicalBookCount = bc
			}
		}
		keepID := group[canonicalIdx].ID

		for i, ser := range group {
			if i == canonicalIdx {
				continue
			}
			books, err := store.GetBooksBySeriesID(ser.ID)
			if err != nil {
				mergeErrors = append(mergeErrors, fmt.Sprintf("failed to get books for series %d: %v", ser.ID, err))
				continue
			}
			for _, book := range books {
				oldSeriesID := ser.ID
				book.SeriesID = &keepID
				if _, err := store.UpdateBook(book.ID, &book); err != nil {
					mergeErrors = append(mergeErrors, fmt.Sprintf("failed to reassign book %s: %v", book.ID, err))
				} else if operationID != "" {
					_ = store.CreateOperationChange(&database.OperationChange{
						ID:          ulid.Make().String(),
						OperationID: operationID,
						BookID:      book.ID,
						ChangeType:  "series_merge",
						FieldName:   "series_id",
						OldValue:    fmt.Sprintf("%d (%s)", oldSeriesID, ser.Name),
						NewValue:    fmt.Sprintf("%d (%s)", keepID, group[canonicalIdx].Name),
					})
				}
			}
			if err := store.DeleteSeries(ser.ID); err != nil {
				mergeErrors = append(mergeErrors, fmt.Sprintf("failed to delete series %d: %v", ser.ID, err))
			} else {
				totalMerged++
				if operationID != "" {
					_ = store.CreateOperationChange(&database.OperationChange{
						ID:          ulid.Make().String(),
						OperationID: operationID,
						ChangeType:  "series_delete",
						FieldName:   "series",
						OldValue:    fmt.Sprintf("%d: %s", ser.ID, ser.Name),
						NewValue:    fmt.Sprintf("merged into %d: %s", keepID, group[canonicalIdx].Name),
					})
				}
			}
		}
	}

	_ = progress.Log("info", fmt.Sprintf("Phase 1 complete: merged %d duplicate series from %d groups", totalMerged, dupGroupCount), nil)
	_ = progress.UpdateProgress(50, 100, "Scanning for orphan series...")

	// Phase 2: Delete orphan series (0 books)
	orphansDeleted := 0
	// Re-fetch series to account for merges
	refreshedSeries, err := store.GetAllSeries()
	if err != nil {
		_ = progress.Log("warn", fmt.Sprintf("Failed to refresh series list: %v", err), nil)
	} else {
		for _, ser := range refreshedSeries {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			books, err := store.GetBooksBySeriesID(ser.ID)
			if err != nil {
				continue
			}
			if len(books) == 0 {
				if err := store.DeleteSeries(ser.ID); err != nil {
					mergeErrors = append(mergeErrors, fmt.Sprintf("failed to delete orphan series %d: %v", ser.ID, err))
				} else {
					orphansDeleted++
					if operationID != "" {
						_ = store.CreateOperationChange(&database.OperationChange{
							ID:          ulid.Make().String(),
							OperationID: operationID,
							ChangeType:  "series_delete",
							FieldName:   "orphan_series",
							OldValue:    fmt.Sprintf("%d: %s", ser.ID, ser.Name),
							NewValue:    "deleted (0 books)",
						})
					}
				}
			}
		}
	}

	totalCleaned := totalMerged + orphansDeleted
	resultMsg := fmt.Sprintf("Series prune complete: %d duplicates merged, %d orphans deleted (%d total cleaned, %d errors)",
		totalMerged, orphansDeleted, totalCleaned, len(mergeErrors))
	_ = progress.Log("info", resultMsg, nil)

	// Record summary change
	if operationID != "" {
		_ = store.CreateOperationChange(&database.OperationChange{
			ID:          ulid.Make().String(),
			OperationID: operationID,
			ChangeType:  "series_prune_summary",
			FieldName:   "summary",
			OldValue:    fmt.Sprintf("%d total series scanned", len(allSeries)),
			NewValue:    resultMsg,
		})
	}
	if len(mergeErrors) > 0 {
		errDetail := strings.Join(mergeErrors[:min(len(mergeErrors), 10)], "; ")
		_ = progress.Log("warn", fmt.Sprintf("Errors: %s", errDetail), nil)
	}
	_ = progress.UpdateProgress(100, 100, resultMsg)

	if s.dedupCache != nil {
		s.dedupCache.InvalidateAll()
	}

	return nil
}

func (s *Server) listNarrators(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	narrators, err := database.GlobalStore.ListNarrators()
	if err != nil {
		internalError(c, "failed to list narrators", err)
		return
	}
	c.JSON(http.StatusOK, narrators)
}

func (s *Server) countNarrators(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	narrators, err := database.GlobalStore.ListNarrators()
	if err != nil {
		internalError(c, "failed to count narrators", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": len(narrators)})
}

func (s *Server) listAudiobookNarrators(c *gin.Context) {
	id := c.Param("id")
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	narrators, err := database.GlobalStore.GetBookNarrators(id)
	if err != nil {
		internalError(c, "failed to list audiobook narrators", err)
		return
	}
	if narrators == nil {
		narrators = []database.BookNarrator{}
	}
	c.JSON(http.StatusOK, narrators)
}

func (s *Server) setAudiobookNarrators(c *gin.Context) {
	id := c.Param("id")
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	var narrators []database.BookNarrator
	if err := c.ShouldBindJSON(&narrators); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := database.GlobalStore.SetBookNarrators(id, narrators); err != nil {
		internalError(c, "failed to set audiobook narrators", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) countSeries(c *gin.Context) {
	count, err := database.GlobalStore.CountSeries()
	if err != nil {
		internalError(c, "failed to count series", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": count})
}

func (s *Server) listSeries(c *gin.Context) {
	resp, err := s.authorSeriesService.ListSeriesWithCounts()
	if err != nil {
		internalError(c, "failed to list series", err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (s *Server) getSeriesBooks(c *gin.Context) {
	seriesID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid series ID"})
		return
	}
	store := database.GlobalStore
	books, err := store.GetBooksBySeriesID(seriesID)
	if err != nil {
		internalError(c, "failed to get series books", err)
		return
	}
	enriched := make([]enrichedBookResponse, len(books))
	for i := range books {
		enriched[i] = enrichBookForResponse(&books[i])
	}
	c.JSON(http.StatusOK, gin.H{"items": enriched, "count": len(enriched)})
}

func (s *Server) renameSeriesHandler(c *gin.Context) {
	seriesID, err := strconv.Atoi(c.Param("id"))
	if err != nil || seriesID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid series ID"})
		return
	}
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name must not be empty"})
		return
	}
	store := database.GlobalStore
	if err := store.UpdateSeriesName(seriesID, name); err != nil {
		internalError(c, "failed to rename series", err)
		return
	}
	if s.dedupCache != nil {
		s.dedupCache.Invalidate("series-duplicates")
	}
	series, _ := store.GetSeriesByID(seriesID)
	c.JSON(http.StatusOK, series)
}

func (s *Server) splitSeriesHandler(c *gin.Context) {
	seriesID, err := strconv.Atoi(c.Param("id"))
	if err != nil || seriesID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid series ID"})
		return
	}
	var req struct {
		BookIDs []string `json:"book_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.BookIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "book_ids must not be empty"})
		return
	}
	store := database.GlobalStore
	oldSeries, err := store.GetSeriesByID(seriesID)
	if err != nil || oldSeries == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "series not found"})
		return
	}
	newSeries, err := store.CreateSeries(oldSeries.Name+" (Split)", oldSeries.AuthorID)
	if err != nil {
		internalError(c, "failed to create new series", err)
		return
	}
	moved := 0
	for _, bookID := range req.BookIDs {
		book, err := store.GetBookByID(bookID)
		if err != nil || book == nil {
			continue
		}
		if book.SeriesID == nil || *book.SeriesID != seriesID {
			continue
		}
		book.SeriesID = &newSeries.ID
		if _, err := store.UpdateBook(book.ID, book); err != nil {
			continue
		}
		moved++
	}
	if s.dedupCache != nil {
		s.dedupCache.Invalidate("series-duplicates")
	}
	c.JSON(http.StatusOK, gin.H{"new_series": newSeries, "books_moved": moved})
}

func (s *Server) deleteEmptySeries(c *gin.Context) {
	seriesID, err := strconv.Atoi(c.Param("id"))
	if err != nil || seriesID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid series ID"})
		return
	}
	store := database.GlobalStore
	books, err := store.GetBooksBySeriesID(seriesID)
	if err != nil {
		internalError(c, "failed to get series books", err)
		return
	}
	if len(books) > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "cannot delete series with books"})
		return
	}
	if err := store.DeleteSeries(seriesID); err != nil {
		internalError(c, "failed to delete series", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "series deleted"})
}

func (s *Server) getAuthorBooks(c *gin.Context) {
	authorID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid author ID"})
		return
	}
	store := database.GlobalStore
	books, err := store.GetBooksByAuthorID(authorID)
	if err != nil {
		internalError(c, "failed to get author books", err)
		return
	}
	enriched := make([]enrichedBookResponse, len(books))
	for i := range books {
		enriched[i] = enrichBookForResponse(&books[i])
	}
	c.JSON(http.StatusOK, gin.H{"items": enriched, "count": len(enriched)})
}

func (s *Server) deleteAuthorHandler(c *gin.Context) {
	authorID, err := strconv.Atoi(c.Param("id"))
	if err != nil || authorID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid author ID"})
		return
	}
	store := database.GlobalStore
	books, err := store.GetBooksByAuthorID(authorID)
	if err != nil {
		internalError(c, "failed to get author books", err)
		return
	}
	if len(books) > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "cannot delete author with books"})
		return
	}
	if err := store.DeleteAuthor(authorID); err != nil {
		internalError(c, "failed to delete author", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "author deleted"})
}

// bulkDeleteAuthors deletes multiple zero-book authors at once.
func (s *Server) bulkDeleteAuthors(c *gin.Context) {
	var req struct {
		IDs []int `json:"ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	store := database.GlobalStore
	deleted := 0
	skipped := 0
	var errors []string
	for _, id := range req.IDs {
		books, err := store.GetBooksByAuthorID(id)
		if err != nil {
			errors = append(errors, fmt.Sprintf("author %d: %v", id, err))
			continue
		}
		if len(books) > 0 {
			skipped++
			continue
		}
		if err := store.DeleteAuthor(id); err != nil {
			errors = append(errors, fmt.Sprintf("author %d: %v", id, err))
			continue
		}
		deleted++
	}
	c.JSON(http.StatusOK, gin.H{
		"deleted": deleted,
		"skipped": skipped,
		"errors":  errors,
		"total":   len(req.IDs),
	})
}

// bulkDeleteSeries deletes multiple empty series at once.
func (s *Server) bulkDeleteSeries(c *gin.Context) {
	var req struct {
		IDs []int `json:"ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	store := database.GlobalStore
	deleted := 0
	skipped := 0
	var errors []string
	for _, id := range req.IDs {
		books, err := store.GetBooksBySeriesID(id)
		if err != nil {
			errors = append(errors, fmt.Sprintf("series %d: %v", id, err))
			continue
		}
		if len(books) > 0 {
			skipped++
			continue
		}
		if err := store.DeleteSeries(id); err != nil {
			errors = append(errors, fmt.Sprintf("series %d: %v", id, err))
			continue
		}
		deleted++
	}
	c.JSON(http.StatusOK, gin.H{
		"deleted": deleted,
		"skipped": skipped,
		"errors":  errors,
		"total":   len(req.IDs),
	})
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
		internalError(c, "failed to list import paths", err)
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
				scanLog := operations.LoggerFromReporter(progress)
				books, err := scanner.ScanDirectoryParallel(folderPath, workers, scanLog)
				if err != nil {
					return fmt.Errorf("failed to scan folder: %w", err)
				}

				scanLog.Info("Found %d audiobook files", len(books))

				// Process the books to extract metadata (parallel)
				if len(books) > 0 {
					scanLog.Info("Processing metadata for %d books using %d workers", len(books), workers)
					if err := scanner.ProcessBooksParallel(ctx, books, workers, nil, scanLog); err != nil {
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
							newPath, _, err := org.OrganizeBook(dbBook)
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

				// Trigger dedup check on newly scanned books
				if s.dedupEngine != nil && len(books) > 0 {
					go func() {
						for _, b := range books {
							dbBook, err := database.GlobalStore.GetBookByFilePath(b.FilePath)
							if err != nil || dbBook == nil {
								continue
							}
							if _, err := s.dedupEngine.CheckBook(context.Background(), dbBook.ID); err != nil {
								log.Printf("[WARN] dedup check failed for scanned book %s: %v", dbBook.ID, err)
							}
						}
					}()
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
			books, err := scanner.ScanDirectory(folder.Path, nil)
			if err == nil {
				if len(books) > 0 {
					_ = scanner.ProcessBooks(books, nil) // ignore individual processing errors (already logged internally)
					// Auto-organize if enabled
					if config.AppConfig.AutoOrganize && config.AppConfig.RootDir != "" {
						org := organizer.NewOrganizer(&config.AppConfig)
						for _, b := range books {
							dbBook, err := database.GlobalStore.GetBookByFilePath(b.FilePath)
							if err != nil || dbBook == nil {
								continue
							}
							newPath, _, err := org.OrganizeBook(dbBook)
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
		internalError(c, "failed to remove import path", err)
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
		internalError(c, "failed to create operation", err)
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
		return s.scanService.PerformScan(ctx, scanReq, operations.LoggerFromReporter(progress))
	}

	// Enqueue the operation
	if err := operations.GlobalQueue.Enqueue(op.ID, "scan", priority, operationFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
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
		FolderPath         *string  `json:"folder_path"`
		Priority           *int     `json:"priority"`
		BookIDs            []string `json:"book_ids"`
		FetchMetadataFirst bool     `json:"fetch_metadata_first"`
		SyncITunesFirst    bool     `json:"sync_itunes_first"`
	}
	_ = c.ShouldBindJSON(&req)

	id := ulid.Make().String()
	op, err := database.GlobalStore.CreateOperation(id, "organize", req.FolderPath)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	// Determine priority (default to normal)
	priority := operations.PriorityNormal
	if req.Priority != nil {
		priority = *req.Priority
	}

	// Create operation function that delegates to service
	organizeReq := &OrganizeRequest{
		FolderPath:         req.FolderPath,
		Priority:           req.Priority,
		BookIDs:            req.BookIDs,
		FetchMetadataFirst: req.FetchMetadataFirst,
		SyncITunesFirst:    req.SyncITunesFirst,
		OperationID:        op.ID,
	}

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		return s.organizeService.PerformOrganize(ctx, organizeReq, operations.LoggerFromReporter(progress))
	}

	// Enqueue the operation
	if err := operations.GlobalQueue.Enqueue(op.ID, "organize", priority, operationFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	c.JSON(http.StatusAccepted, op)
}

func (s *Server) startTranscode(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	if operations.GlobalQueue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	var req struct {
		BookID       string `json:"book_id"`
		OutputFormat string `json:"output_format"`
		Bitrate      int    `json:"bitrate"`
		KeepOriginal *bool  `json:"keep_original"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.BookID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "book_id is required"})
		return
	}

	// Verify the book exists
	if _, err := database.GlobalStore.GetBookByID(req.BookID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "book not found"})
		return
	}

	id := ulid.Make().String()
	op, err := database.GlobalStore.CreateOperation(id, "transcode", nil)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	keepOriginal := true
	if req.KeepOriginal != nil {
		keepOriginal = *req.KeepOriginal
	}

	opts := transcode.TranscodeOpts{
		BookID:       req.BookID,
		OutputFormat: req.OutputFormat,
		Bitrate:      req.Bitrate,
		KeepOriginal: keepOriginal,
	}

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		outputPath, err := transcode.Transcode(ctx, opts, database.GlobalStore, progress)
		if err != nil {
			return err
		}

		// Get the original book to preserve its data
		originalBook, err := database.GlobalStore.GetBookByID(req.BookID)
		if err != nil {
			return fmt.Errorf("failed to get original book: %w", err)
		}

		// Set up version group if not already set
		groupID := ""
		if originalBook.VersionGroupID != nil && *originalBook.VersionGroupID != "" {
			groupID = *originalBook.VersionGroupID
		} else {
			groupID = ulid.Make().String()
		}

		// Mark original as non-primary version (modify fetched book to preserve all fields)
		notPrimary := false
		origNotes := "Original format"
		originalBook.IsPrimaryVersion = &notPrimary
		originalBook.VersionGroupID = &groupID
		originalBook.VersionNotes = &origNotes
		if _, err := database.GlobalStore.UpdateBook(req.BookID, originalBook); err != nil {
			progress.Log("warn", fmt.Sprintf("Failed to update original book version info: %v", err), nil)
		}

		// Create a new book record for the M4B version
		m4bFormat := "m4b"
		aacCodec := "aac"
		bitrateVal := opts.Bitrate
		if bitrateVal <= 0 {
			bitrateVal = 128
		}
		isPrimary := true
		m4bNotes := "Transcoded to M4B"

		newBook := &database.Book{
			ID:                   ulid.Make().String(),
			Title:                originalBook.Title,
			FilePath:             outputPath,
			Format:               m4bFormat,
			Codec:                &aacCodec,
			Bitrate:              &bitrateVal,
			AuthorID:             originalBook.AuthorID,
			SeriesID:             originalBook.SeriesID,
			SeriesSequence:       originalBook.SeriesSequence,
			Duration:             originalBook.Duration,
			Narrator:             originalBook.Narrator,
			Publisher:            originalBook.Publisher,
			PrintYear:            originalBook.PrintYear,
			AudiobookReleaseYear: originalBook.AudiobookReleaseYear,
			ISBN10:               originalBook.ISBN10,
			ISBN13:               originalBook.ISBN13,
			ASIN:                 originalBook.ASIN,
			Language:             originalBook.Language,
			CoverURL:             originalBook.CoverURL,
			IsPrimaryVersion:     &isPrimary,
			VersionGroupID:       &groupID,
			VersionNotes:         &m4bNotes,
		}
		if _, err := database.GlobalStore.CreateBook(newBook); err != nil {
			// Fallback: update original in-place but preserve all existing fields
			progress.Log("warn", fmt.Sprintf("Failed to create M4B version record, updating original: %v", err), nil)
			isPrim := true
			fallbackNotes := fmt.Sprintf("Transcoded to M4B (in-place, original was at %s)", originalBook.FilePath)
			originalBook.FilePath = outputPath
			originalBook.Format = m4bFormat
			originalBook.Codec = &aacCodec
			originalBook.Bitrate = &bitrateVal
			originalBook.IsPrimaryVersion = &isPrim
			originalBook.VersionGroupID = &groupID
			originalBook.VersionNotes = &fallbackNotes
			if _, updateErr := database.GlobalStore.UpdateBook(req.BookID, originalBook); updateErr != nil {
				return updateErr
			}
			return nil
		}

		progress.Log("info", fmt.Sprintf("Created M4B version %s (group %s), original %s demoted to non-primary", newBook.ID, groupID, req.BookID), nil)

		// If iTunes write-back is disabled and the original book came from iTunes,
		// store a deferred update so the path change is applied on the next sync.
		if !config.AppConfig.ITLWriteBackEnabled &&
			originalBook.ITunesPersistentID != nil &&
			*originalBook.ITunesPersistentID != "" {
			if err := database.GlobalStore.CreateDeferredITunesUpdate(
				originalBook.ID,
				*originalBook.ITunesPersistentID,
				originalBook.FilePath,
				newBook.FilePath,
				"transcode",
			); err != nil {
				progress.Log("warn", fmt.Sprintf("Failed to create deferred iTunes update: %v", err), nil)
			} else {
				progress.Log("info", "M4B created. iTunes library update deferred until write-back is enabled.", nil)
			}
		}

		return nil
	}

	if err := operations.GlobalQueue.Enqueue(op.ID, "transcode", operations.PriorityNormal, operationFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
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

	id := c.Param("id")

	// Check if this is an AI scan operation — cancel via pipeline manager
	if s.pipelineManager != nil && s.aiScanStore != nil {
		scans, _ := s.aiScanStore.ListScans()
		for _, scan := range scans {
			if scan.OperationID == id {
				if err := s.pipelineManager.CancelScan(scan.ID); err != nil {
					log.Printf("[cancelOperation] AI scan %d cancel warning: %v", scan.ID, err)
				}
				c.Status(http.StatusNoContent)
				return
			}
		}
	}

	// Try cancel via queue (for running queue operations)
	if operations.GlobalQueue != nil {
		if err := operations.GlobalQueue.Cancel(id); err == nil {
			c.Status(http.StatusNoContent)
			return
		}
	}

	// Fallback: force-update DB status (e.g., stale after restart)
	if dbErr := database.GlobalStore.UpdateOperationStatus(id, "canceled", 0, 0, "force canceled (stale operation)"); dbErr != nil {
		internalError(c, "failed to cancel operation", dbErr)
		return
	}
	c.Status(http.StatusNoContent)
}

// clearStaleOperations force-marks all pending/running/queued operations as failed.
func (s *Server) clearStaleOperations(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	ops, err := database.GlobalStore.GetRecentOperations(500)
	if err != nil {
		internalError(c, "failed to get operations", err)
		return
	}

	cleared := 0
	for _, op := range ops {
		if op.Status == "pending" || op.Status == "running" || op.Status == "queued" {
			_ = database.GlobalStore.UpdateOperationStatus(op.ID, "failed", 0, 0, "force cleared by user")
			cleared++
		}
	}

	c.JSON(http.StatusOK, gin.H{"cleared": cleared})
}

// deleteOperationHistory deletes operations matching the given status(es).
// Query param: ?status=completed or ?status=failed or ?status=completed,failed
func (s *Server) deleteOperationHistory(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	statusParam := c.Query("status")
	if statusParam == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status parameter required"})
		return
	}

	statuses := strings.Split(statusParam, ",")
	// Only allow deleting terminal statuses
	allowed := map[string]bool{"completed": true, "failed": true, "canceled": true}
	for _, s := range statuses {
		if !allowed[s] {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("cannot delete operations with status %q", s)})
			return
		}
	}

	deleted, err := database.GlobalStore.DeleteOperationsByStatus(statuses)
	if err != nil {
		internalError(c, "failed to delete operations", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"deleted": deleted})
}

// optimizeDatabase splits &-delimited author/narrator strings and re-extracts empty media info.
func (s *Server) optimizeDatabase(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	books, err := database.GlobalStore.GetAllBooks(10000, 0)
	if err != nil {
		internalError(c, "failed to get audiobooks", err)
		return
	}

	authorsSplit := 0
	narratorsSplit := 0

	for _, book := range books {
		// Split compound author names into individual book_authors
		if book.AuthorID != nil {
			author, err := database.GlobalStore.GetAuthorByID(*book.AuthorID)
			if err == nil && author != nil && strings.Contains(author.Name, " & ") {
				names := splitMultipleNames(author.Name)
				if len(names) > 1 {
					var bookAuthors []database.BookAuthor
					for _, name := range names {
						a, err := database.GlobalStore.GetAuthorByName(name)
						if err != nil || a == nil {
							a, err = database.GlobalStore.CreateAuthor(name)
							if err != nil {
								continue
							}
						}
						bookAuthors = append(bookAuthors, database.BookAuthor{
							AuthorID: a.ID,
							Role:     "author",
						})
					}
					if len(bookAuthors) > 0 {
						if err := database.GlobalStore.SetBookAuthors(book.ID, bookAuthors); err == nil {
							authorsSplit++
						}
					}
				}
			}
		}

		// Split compound narrator names into individual book_narrators
		if book.Narrator != nil && strings.Contains(*book.Narrator, " & ") {
			names := splitMultipleNames(*book.Narrator)
			if len(names) > 1 {
				var bookNarrators []database.BookNarrator
				for _, name := range names {
					n, err := database.GlobalStore.GetNarratorByName(name)
					if err != nil || n == nil {
						n, err = database.GlobalStore.CreateNarrator(name)
						if err != nil {
							continue
						}
					}
					bookNarrators = append(bookNarrators, database.BookNarrator{
						NarratorID: n.ID,
					})
				}
				if len(bookNarrators) > 0 {
					if err := database.GlobalStore.SetBookNarrators(book.ID, bookNarrators); err == nil {
						narratorsSplit++
					}
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"books_processed": len(books),
		"authors_split":   authorsSplit,
		"narrators_split": narratorsSplit,
	})
}

func (s *Server) sweepTombstones(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	result, err := SweepTombstones(database.GlobalStore)
	if err != nil {
		internalError(c, "failed to sweep tombstones", err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) auditFileConsistency(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	result, err := AuditFileConsistency(database.GlobalStore)
	if err != nil {
		internalError(c, "failed to audit file consistency", err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// listActiveOperations returns a snapshot of currently queued/running operations with basic progress
func (s *Server) listOperations(c *gin.Context) {
	params := ParsePaginationParams(c)
	store := database.GlobalStore
	if store == nil {
		c.JSON(http.StatusOK, gin.H{"items": []database.Operation{}, "total": 0, "limit": params.Limit, "offset": params.Offset})
		return
	}
	ops, total, err := store.ListOperations(params.Limit, params.Offset)
	if err != nil {
		internalError(c, "failed to list operations", err)
		return
	}
	if ops == nil {
		ops = []database.Operation{}
	}
	c.JSON(http.StatusOK, gin.H{"items": ops, "total": total, "limit": params.Limit, "offset": params.Offset})
}

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

func (s *Server) listStaleOperations(c *gin.Context) {
	timeoutMinutes := config.AppConfig.OperationTimeoutMinutes
	if timeoutMinutes <= 0 {
		timeoutMinutes = 30
	}
	if raw := strings.TrimSpace(c.Query("timeout_minutes")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			timeoutMinutes = parsed
		}
	}

	stale, err := s.collectStaleOperations(time.Duration(timeoutMinutes) * time.Minute)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list stale operations"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"timeout_minutes": timeoutMinutes,
		"count":           len(stale),
		"operations":      stale,
	})
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
		internalError(c, "failed to get system status", err)
		return
	}

	c.JSON(http.StatusOK, status)
}

func (s *Server) getSystemAnnouncements(c *gin.Context) {
	type Announcement struct {
		ID       string `json:"id"`
		Severity string `json:"severity"` // info, warning, error
		Message  string `json:"message"`
		Link     string `json:"link,omitempty"`
	}

	var announcements []Announcement

	// Check for duplicate authors
	authors, err := database.GlobalStore.GetAllAuthors()
	if err == nil {
		bookCountFn := func(authorID int) int {
			books, err := database.GlobalStore.GetBooksByAuthorIDWithRole(authorID)
			if err != nil {
				return 0
			}
			return len(books)
		}
		groups := s.filterReviewedAuthorGroups(FindDuplicateAuthors(authors, 0.9, bookCountFn))
		if len(groups) > 0 {
			announcements = append(announcements, Announcement{
				ID:       "duplicate-authors",
				Severity: "warning",
				Message:  fmt.Sprintf("You have %d group(s) of duplicate authors to review", len(groups)),
				Link:     "/dedup?tab=authors",
			})
		}
	}

	// Check for missing files (sample first 100 books)
	books, err := database.GlobalStore.GetAllBooks(100, 0)
	if err == nil {
		missingCount := 0
		for _, book := range books {
			if book.FilePath != "" {
				if _, statErr := os.Stat(book.FilePath); os.IsNotExist(statErr) {
					missingCount++
				}
			}
		}
		if missingCount > 0 {
			announcements = append(announcements, Announcement{
				ID:       "missing-files",
				Severity: "warning",
				Message:  fmt.Sprintf("%d book(s) have missing files on disk", missingCount),
				Link:     "/library",
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{"announcements": announcements})
}

func (s *Server) getSystemStorage(c *gin.Context) {
	rootDir := strings.TrimSpace(config.AppConfig.RootDir)
	if rootDir == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "root_dir is not configured"})
		return
	}

	totalBytes, freeBytes, err := getDiskStats(rootDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read filesystem stats"})
		return
	}

	usedBytes := totalBytes - freeBytes
	percentUsed := 0.0
	if totalBytes > 0 {
		percentUsed = (float64(usedBytes) / float64(totalBytes)) * 100.0
	}

	c.JSON(http.StatusOK, gin.H{
		"path":                rootDir,
		"total_bytes":         totalBytes,
		"used_bytes":          usedBytes,
		"free_bytes":          freeBytes,
		"percent_used":        percentUsed,
		"quota_enabled":       config.AppConfig.EnableDiskQuota,
		"quota_percent":       config.AppConfig.DiskQuotaPercent,
		"user_quotas_enabled": config.AppConfig.EnableUserQuotas,
	})
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
		internalError(c, "failed to get system logs", err)
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
		internalError(c, "failed to reset database", err)
		return
	}

	// Reset config to defaults
	config.ResetToDefaults()

	// Reset caches
	resetLibrarySizeCache()
	s.dashboardCache.InvalidateAll()

	RespondWithOK(c, gin.H{"message": "System reset successfully"})
}

func (s *Server) factoryReset(c *gin.Context) {
	var req struct {
		Confirm string `json:"confirm"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Confirm != "RESET" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "request body must contain {\"confirm\": \"RESET\"}"})
		return
	}

	log.Printf("[INFO] Factory reset initiated")

	// Reset database (books, authors, series, settings)
	if err := database.GlobalStore.Reset(); err != nil {
		log.Printf("[ERROR] Factory reset: database reset failed: %v", err)
		internalError(c, "failed to reset database", err)
		return
	}
	log.Printf("[INFO] Factory reset: database cleared")

	// Delete OL data (pebble store + dump files)
	if s.olService != nil {
		s.olService.mu.Lock()
		if s.olService.store != nil {
			s.olService.store.Close()
			s.olService.store = nil
		}
		s.olService.mu.Unlock()

		targetDir := getOLDumpDir()
		if targetDir != "" {
			if err := os.RemoveAll(targetDir); err != nil {
				log.Printf("[WARN] Factory reset: failed to remove OL data dir: %v", err)
			} else {
				log.Printf("[INFO] Factory reset: OL data deleted")
			}
		}
	}

	// Clear library folder contents (organized audiobooks)
	if config.AppConfig.RootDir != "" {
		libraryDir := config.AppConfig.RootDir
		entries, err := os.ReadDir(libraryDir)
		if err == nil {
			for _, entry := range entries {
				entryPath := filepath.Join(libraryDir, entry.Name())
				if err := os.RemoveAll(entryPath); err != nil {
					log.Printf("[WARN] Factory reset: failed to remove %s: %v", entryPath, err)
				}
			}
			log.Printf("[INFO] Factory reset: library folder cleared (%s)", libraryDir)
		}
	}

	// Reset config to defaults, then clear paths so wizard re-shows
	config.ResetToDefaults()
	config.AppConfig.RootDir = ""
	config.AppConfig.SetupComplete = false
	if err := config.SaveConfigToDatabase(database.GlobalStore); err != nil {
		log.Printf("[WARN] Factory reset: failed to persist config: %v", err)
	}

	// Reset caches
	resetLibrarySizeCache()
	s.dashboardCache.InvalidateAll()

	log.Printf("[INFO] Factory reset complete")
	c.JSON(http.StatusOK, gin.H{"message": "factory reset complete"})
}

func (s *Server) getConfig(c *gin.Context) {
	// Create a copy of config with masked secrets
	maskedConfig := config.AppConfig
	if maskedConfig.OpenAIAPIKey != "" {
		maskedConfig.OpenAIAPIKey = database.MaskSecret(maskedConfig.OpenAIAPIKey)
	}
	c.JSON(http.StatusOK, gin.H{"config": maskedConfig})
}

func (s *Server) updateConfig(c *gin.Context) {
	var payload map[string]any
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	previousConfig := config.AppConfig
	status, resp := s.configUpdateService.UpdateConfig(payload)
	if status >= 400 {
		config.AppConfig = previousConfig
		c.JSON(status, resp)
		return
	}

	if err := config.AppConfig.Validate(); err != nil {
		config.AppConfig = previousConfig
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	maskedConfig := s.configUpdateService.MaskSecrets(config.AppConfig)
	response := gin.H{"config": maskedConfig}
	if raw, err := json.Marshal(maskedConfig); err == nil {
		var flat map[string]any
		if err := json.Unmarshal(raw, &flat); err == nil {
			for k, v := range flat {
				response[k] = v
			}
		}
	}
	c.JSON(http.StatusOK, response)
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
		internalError(c, "failed to get operation logs", err)
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

	// Resolve backup dir relative to database directory so it's always absolute
	if dbPath != "" && !filepath.IsAbs(backupConfig.BackupDir) {
		backupConfig.BackupDir = filepath.Join(filepath.Dir(dbPath), backupConfig.BackupDir)
	}

	info, err := backup.CreateBackup(dbPath, dbType, backupConfig)
	if err != nil {
		internalError(c, "failed to create backup", err)
		return
	}

	c.JSON(http.StatusOK, info)
}

// listBackups lists all available backups
func (s *Server) listBackups(c *gin.Context) {
	backupConfig := backup.DefaultBackupConfig()
	if dbPath := config.AppConfig.DatabasePath; dbPath != "" && !filepath.IsAbs(backupConfig.BackupDir) {
		backupConfig.BackupDir = filepath.Join(filepath.Dir(dbPath), backupConfig.BackupDir)
	}

	backups, err := backup.ListBackups(backupConfig.BackupDir)
	if err != nil {
		internalError(c, "failed to list backups", err)
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
	if dbPath := config.AppConfig.DatabasePath; dbPath != "" && !filepath.IsAbs(backupConfig.BackupDir) {
		backupConfig.BackupDir = filepath.Join(filepath.Dir(dbPath), backupConfig.BackupDir)
	}
	backupPath := filepath.Join(backupConfig.BackupDir, req.BackupFilename)

	// Use current database path as target if not specified
	targetPath := req.TargetPath
	if targetPath == "" {
		targetPath = filepath.Dir(config.AppConfig.DatabasePath)
	}

	if err := backup.RestoreBackup(backupPath, targetPath, req.Verify); err != nil {
		internalError(c, "failed to restore backup", err)
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
	if dbPath := config.AppConfig.DatabasePath; dbPath != "" && !filepath.IsAbs(backupConfig.BackupDir) {
		backupConfig.BackupDir = filepath.Join(filepath.Dir(dbPath), backupConfig.BackupDir)
	}
	// Sanitize filename to prevent path traversal
	filename = filepath.Base(filename)
	backupPath := filepath.Join(backupConfig.BackupDir, filename)

	if err := backup.DeleteBackup(backupPath); err != nil {
		internalError(c, "failed to delete backup", err)
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
		internalError(c, "failed to get audiobooks", err)
		return
	}

	// Export metadata
	exportData, err := metadata.ExportMetadata(books)
	if err != nil {
		internalError(c, "failed to export metadata", err)
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
		Validate bool           `json:"validate"`
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
		internalError(c, "metadata search failed", err)
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
	cleaned := title

	// Strip leading track/disc number prefixes from filenames
	// e.g. "01 - Title", "01. Title", "1 - Title", "123 - Title"
	trackNumPrefix := regexp.MustCompile(`^\d{1,3}\s*[-–.]\s*`)
	cleaned = trackNumPrefix.ReplaceAllString(cleaned, "")
	// e.g. "01 Title" (bare number prefix followed by non-numeric text)
	bareNumPrefix := regexp.MustCompile(`^\d{1,3}\s+`)
	if stripped := strings.TrimSpace(bareNumPrefix.ReplaceAllString(cleaned, "")); stripped != "" {
		cleaned = stripped
	}
	// e.g. "Track 01 - Title", "Track01 - Title"
	trackWordPrefix := regexp.MustCompile(`(?i)^[Tt]rack\s*\d+\s*[-–.]\s*`)
	cleaned = trackWordPrefix.ReplaceAllString(cleaned, "")
	// e.g. "Disc 1 - Title", "Disc01 - Title"
	discWordPrefix := regexp.MustCompile(`(?i)^[Dd]is[ck]\s*\d+\s*[-–.]\s*`)
	cleaned = discWordPrefix.ReplaceAllString(cleaned, "")

	// Strip leading bracketed series info like "[The Expanse 9.0]" or "[Series Name]"
	bracketPrefix := regexp.MustCompile(`^\[.*?\]\s*[-–]?\s*`)
	cleaned = bracketPrefix.ReplaceAllString(cleaned, "")

	// Strip trailing bracketed info like "Title [Unabridged]"
	bracketSuffix := regexp.MustCompile(`\s*\[.*?\]\s*$`)
	cleaned = bracketSuffix.ReplaceAllString(cleaned, "")

	// Common patterns for chapters/books/parts/volumes
	patterns := []string{
		`(?i)[,:\s]*-?\s*(?:Book|Chapter|Part|Volume|Vol\.?|Pt\.?)\s*\d+[\.\d]*\s*$`,
		`(?i)\s*\((?:Book|Chapter|Part|Volume)\s*\d+[\.\d]*\)`,
		`(?i)\s*#\d+[\.\d]*\s*$`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		cleaned = re.ReplaceAllString(cleaned, "")
	}

	// Strip audiobook qualifiers like "(Unabridged)", "(Abridged)", etc.
	qualifiers := regexp.MustCompile(`(?i)\s*\((un)?abridged\)`)
	cleaned = qualifiers.ReplaceAllString(cleaned, "")

	// Strip leading/trailing " - " artifacts from removals
	cleaned = strings.TrimLeft(cleaned, " -–")
	cleaned = strings.TrimRight(cleaned, " -–")
	cleaned = strings.TrimSpace(cleaned)

	// If stripping removed everything, return the original title
	if cleaned == "" {
		return strings.TrimSpace(title)
	}
	return cleaned
}

// stripSubtitle removes subtitle portions from a title, e.g.
// "Title: A Subtitle" → "Title", "Title - A Subtitle" → "Title".
// Returns the original title if no subtitle separator is found.
func stripSubtitle(title string) string {
	// Try colon separator first: "Title: Subtitle"
	if idx := strings.Index(title, ": "); idx > 0 {
		return strings.TrimSpace(title[:idx])
	}
	// Try dash separator: "Title - Subtitle"
	if idx := strings.Index(title, " - "); idx > 0 {
		return strings.TrimSpace(title[:idx])
	}
	// Try em-dash: "Title — Subtitle"
	if idx := strings.Index(title, " — "); idx > 0 {
		return strings.TrimSpace(title[:idx])
	}
	return title
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

	// Enqueue for iTunes auto write-back if metadata was updated
	if GlobalWriteBackBatcher != nil {
		GlobalWriteBackBatcher.Enqueue(id)
	}

	// Re-fetch to get fully enriched book with author/series/narrator names
	enrichedBook := resp.Book
	if fresh, err := database.GlobalStore.GetBookByID(id); err == nil && fresh != nil {
		enrichedBook = fresh
	}
	c.JSON(http.StatusOK, gin.H{
		"message": resp.Message,
		"book":    enrichBookForResponse(enrichedBook),
		"source":  resp.Source,
	})
}

// searchAudiobookMetadata handles POST /api/v1/audiobooks/:id/search-metadata.
func (s *Server) searchAudiobookMetadata(c *gin.Context) {
	id := c.Param("id")
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	var body struct {
		Query     string `json:"query"`
		Author    string `json:"author"`
		Narrator  string `json:"narrator"`
		Series    string `json:"series"`
		UseRerank bool   `json:"use_rerank"`
	}
	_ = c.ShouldBindJSON(&body)

	// Cache metadata search results for 60s — external API calls are expensive.
	// use_rerank is part of the cache key so a rerank result and a non-rerank
	// result for the same search don't clobber each other.
	cacheKey := fmt.Sprintf("meta_search:%s:%s:%s:%s:%s:%t",
		id, body.Query, body.Author, body.Narrator, body.Series, body.UseRerank)
	if cached, ok := s.listCache.Get(cacheKey); ok {
		c.JSON(http.StatusOK, cached)
		return
	}

	resp, err := s.metadataFetchService.SearchMetadataForBookWithOptions(
		id, body.Query, body.Author, body.Narrator, body.Series,
		SearchOptions{UseRerank: body.UseRerank},
	)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	// Cache as gin.H wrapper
	respH := gin.H{"results": resp.Results, "query": resp.Query, "sources_tried": resp.SourcesTried, "sources_failed": resp.SourcesFailed}
	s.listCache.Set(cacheKey, respH)
	c.JSON(http.StatusOK, resp)
}

// applyAudiobookMetadata handles POST /api/v1/audiobooks/:id/apply-metadata.
func (s *Server) applyAudiobookMetadata(c *gin.Context) {
	id := c.Param("id")
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	var body struct {
		Candidate MetadataCandidate `json:"candidate"`
		Fields    []string          `json:"fields"`
		WriteBack *bool             `json:"write_back"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	resp, err := s.metadataFetchService.ApplyMetadataCandidate(id, body.Candidate, body.Fields)
	if err != nil {
		internalError(c, "failed to apply metadata", err)
		return
	}

	// Kick off slow file I/O (cover embed, tags, rename) in background.
	// Cover download is already done inline so the response has the URL.
	shouldWriteBack := body.WriteBack == nil || *body.WriteBack
	if pool := GetGlobalFileIOPool(); pool != nil {
		bookID := id
		mfs := s.metadataFetchService
		pool.Submit(bookID, func() {
			mfs.ApplyMetadataFileIO(bookID)
			if shouldWriteBack {
				if _, wbErr := mfs.WriteBackMetadataForBook(bookID); wbErr != nil {
					log.Printf("[WARN] background write-back for %s: %v", bookID, wbErr)
				}
				if GlobalWriteBackBatcher != nil {
					GlobalWriteBackBatcher.Enqueue(bookID)
				}
			}
		})
	}

	// Re-fetch to get fully enriched book with author/series/narrator names
	enrichedBook := resp.Book
	if fresh, err := database.GlobalStore.GetBookByID(id); err == nil && fresh != nil {
		enrichedBook = fresh
	}
	c.JSON(http.StatusOK, gin.H{
		"message": resp.Message,
		"book":    enrichBookForResponse(enrichedBook),
		"source":  resp.Source,
	})
}

// markAudiobookNoMatch handles POST /api/v1/audiobooks/:id/mark-no-match.
func (s *Server) markAudiobookNoMatch(c *gin.Context) {
	id := c.Param("id")
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	if err := s.metadataFetchService.MarkNoMatch(id); err != nil {
		internalError(c, "failed to mark no match", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Book marked as no match"})
}

// revertAudiobookMetadata handles POST /api/v1/audiobooks/:id/revert-metadata.
// It restores a book to a previous CoW version snapshot via the store layer.
func (s *Server) revertAudiobookMetadata(c *gin.Context) {
	id := c.Param("id")
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	var body struct {
		Timestamp string `json:"timestamp"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Timestamp == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "timestamp is required"})
		return
	}
	ts, err := time.Parse(time.RFC3339Nano, body.Timestamp)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid timestamp format, use RFC3339Nano"})
		return
	}
	book, err := database.GlobalStore.RevertBookToVersion(id, ts)
	if err != nil {
		internalError(c, "failed to revert metadata", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Book reverted to version", "book": book})
}

// listBookCOWVersions handles GET /api/v1/audiobooks/:id/cow-versions.
// Returns copy-on-write version snapshots from the store layer.
func (s *Server) listBookCOWVersions(c *gin.Context) {
	id := c.Param("id")
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	limit := 50
	if q := c.Query("limit"); q != "" {
		if v, err := strconv.Atoi(q); err == nil && v > 0 {
			limit = v
		}
	}
	versions, err := database.GlobalStore.GetBookVersions(id, limit)
	if err != nil {
		internalError(c, "failed to list versions", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"versions": versions})
}

// pruneBookCOWVersions handles POST /api/v1/audiobooks/:id/cow-versions/prune.
// Prunes old copy-on-write version snapshots, keeping the most recent N.
func (s *Server) pruneBookCOWVersions(c *gin.Context) {
	id := c.Param("id")
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	var body struct {
		KeepCount int `json:"keep_count"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.KeepCount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "keep_count must be a positive integer"})
		return
	}
	pruned, err := database.GlobalStore.PruneBookVersions(id, body.KeepCount)
	if err != nil {
		internalError(c, "failed to prune versions", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"pruned": pruned})
}

// writeBackAudiobookMetadata handles POST /api/v1/audiobooks/:id/write-back.
// It writes current DB metadata to audio files AND renames files if AutoRenameOnApply is enabled.
func (s *Server) writeBackAudiobookMetadata(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "book id is required"})
		return
	}

	// Parse optional segment filter and rename flag from request body
	var body struct {
		SegmentIDs []string `json:"segment_ids"`
		Rename     *bool    `json:"rename"`
	}
	_ = c.ShouldBindJSON(&body)

	book, err := database.GlobalStore.GetBookByID(id)
	if err != nil || book == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
		return
	}

	// Step 1: Rename files if requested or AutoRenameOnApply is on
	renamed := 0
	doRename := (body.Rename != nil && *body.Rename) || config.AppConfig.AutoRenameOnApply
	if doRename && len(body.SegmentIDs) == 0 {
		if err := s.metadataFetchService.RunApplyPipelineRenameOnly(id, book); err != nil {
			log.Printf("[WARN] rename failed for book %s: %v", id, err)
		} else {
			renamed = 1
			// Re-fetch book after rename since file_path may have changed
			book, _ = database.GlobalStore.GetBookByID(id)
		}
	}

	// Step 2: Write tags to files
	var writtenCount int
	if len(body.SegmentIDs) > 0 {
		writtenCount, err = s.metadataFetchService.WriteBackMetadataForBook(id, body.SegmentIDs)
	} else {
		writtenCount, err = s.metadataFetchService.WriteBackMetadataForBook(id)
	}
	if err != nil {
		internalError(c, "failed to write back metadata", err)
		return
	}

	msg := fmt.Sprintf("metadata written to %d file(s)", writtenCount)
	if writtenCount == 0 {
		msg = "no files needed tag updates (tags already match DB values)"
	}
	if renamed > 0 {
		msg += ", files renamed"
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       msg,
		"written_count": writtenCount,
		"renamed":       renamed > 0,
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

	sourceChain := s.metadataFetchService.BuildSourceChain()
	if len(sourceChain) == 0 {
		// Fallback to Audible if no sources configured (best for audiobooks)
		sourceChain = []metadata.MetadataSource{metadata.NewAudibleClient()}
	}
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

		// Resolve current author for post-search verification (NOT for search query)
		currentAuthor := ""
		if book.Author != nil {
			currentAuthor = book.Author.Name
		} else if book.AuthorID != nil {
			if author, err := database.GlobalStore.GetAuthorByID(*book.AuthorID); err == nil && author != nil {
				currentAuthor = author.Name
			}
		}

		// Clean title: strip track number prefixes like "01 - ", chapter markers, etc.
		searchTitle := stripChapterFromTitle(book.Title)

		// Search using both title and author (like the manual search dialog does)
		// for better match quality. Author is used as a filter, not as the primary query.
		var metaResults []metadata.BookMetadata
		var sourceName string
		for _, src := range sourceChain {
			// If we have an author, try title+author search first for more precise results
			if currentAuthor != "" {
				metaResults, err = src.SearchByTitleAndAuthor(searchTitle, currentAuthor)
				if err == nil && len(metaResults) > 0 {
					sourceName = src.Name()
					break
				}
			}
			// Fall back to title-only search
			metaResults, err = src.SearchByTitle(searchTitle)
			if err == nil && len(metaResults) > 0 {
				sourceName = src.Name()
				break
			}
			// Try original title if stripped version returned nothing
			if searchTitle != book.Title {
				if currentAuthor != "" {
					metaResults, err = src.SearchByTitleAndAuthor(book.Title, currentAuthor)
					if err == nil && len(metaResults) > 0 {
						sourceName = src.Name()
						break
					}
				}
				metaResults, err = src.SearchByTitle(book.Title)
				if err == nil && len(metaResults) > 0 {
					sourceName = src.Name()
					break
				}
			}
			log.Printf("[DEBUG] bulkFetchMetadata: source %s returned no results for %q, trying next", src.Name(), searchTitle)
		}
		if len(metaResults) == 0 {
			result.Status = "not_found"
			result.Message = "no metadata found from any source"
			results = append(results, result)
			continue
		}

		// Pick best match: prefer result whose author matches current author if known
		meta := metaResults[0]
		if currentAuthor != "" && len(metaResults) > 1 {
			lowerAuthor := strings.ToLower(currentAuthor)
			for _, r := range metaResults {
				if strings.EqualFold(r.Author, currentAuthor) || strings.Contains(strings.ToLower(r.Author), lowerAuthor) {
					meta = r
					break
				}
			}
		}
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

		if meta.Title != "" && !isGarbageValue(meta.Title) {
			addFetched("title", meta.Title)
			if shouldApply("title", hasBookValue("title")) {
				book.Title = meta.Title
				appliedFields = append(appliedFields, "title")
				didUpdate = true
			}
		}

		if meta.Author != "" && !isGarbageValue(meta.Author) {
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

		if meta.Publisher != "" && !isGarbageValue(meta.Publisher) {
			addFetched("publisher", meta.Publisher)
			if shouldApply("publisher", hasBookValue("publisher")) {
				book.Publisher = stringPtr(meta.Publisher)
				appliedFields = append(appliedFields, "publisher")
				didUpdate = true
			}
		}

		if meta.Language != "" && !isGarbageValue(meta.Language) {
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
			// Record change history before applying
			s.metadataFetchService.recordChangeHistory(book, meta, sourceName)

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
	if err != nil || book == nil {
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

// splitVersion moves selected segments from a book into a new version (a new book
// in the same version group).
func (s *Server) splitVersion(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		SegmentIDs []string `json:"segment_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.SegmentIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "segment_ids must not be empty"})
		return
	}

	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// 1. Get source book
	sourceBook, err := database.GlobalStore.GetBookByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
		return
	}

	// 2. Ensure source book has a version group
	versionGroupID := ""
	if sourceBook.VersionGroupID != nil && *sourceBook.VersionGroupID != "" {
		versionGroupID = *sourceBook.VersionGroupID
	} else {
		versionGroupID = ulid.Make().String()
		sourceBook.VersionGroupID = &versionGroupID
		if _, err := database.GlobalStore.UpdateBook(id, sourceBook); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update source book version group"})
			return
		}
	}

	// 3. Count existing versions to determine suffix
	existingVersions, _ := database.GlobalStore.GetBooksByVersionGroup(versionGroupID)
	versionNum := len(existingVersions) + 1

	// 4. Create new book entry (copy metadata from source, but NOT FilePath —
	// the new version's path will be derived from its segments after they're moved)
	newTitle := fmt.Sprintf("%s (Version %d)", sourceBook.Title, versionNum)
	primaryFlag := false
	newBook := &database.Book{
		Title:            newTitle,
		AuthorID:         sourceBook.AuthorID,
		SeriesID:         sourceBook.SeriesID,
		SeriesSequence:   sourceBook.SeriesSequence,
		FilePath:         "", // Will be set from segments below
		Format:           sourceBook.Format,
		WorkID:           sourceBook.WorkID,
		Narrator:         sourceBook.Narrator,
		Language:         sourceBook.Language,
		Publisher:        sourceBook.Publisher,
		VersionGroupID:   &versionGroupID,
		IsPrimaryVersion: &primaryFlag,
	}

	createdBook, err := database.GlobalStore.CreateBook(newBook)
	if err != nil {
		internalError(c, "failed to create new version", err)
		return
	}

	// 5. Move files to the new book (DB-only, does NOT touch files on disk)
	if err := database.GlobalStore.MoveBookFilesToBook(req.SegmentIDs, sourceBook.ID, createdBook.ID); err != nil {
		internalError(c, "failed to move files", err)
		return
	}

	// 6. Derive the new book's FilePath from its files.
	// For multi-file books, FilePath is the common parent directory.
	// For single-file books, FilePath is the file path itself.
	newFiles, _ := database.GlobalStore.GetBookFiles(createdBook.ID)
	if len(newFiles) > 0 {
		if len(newFiles) == 1 {
			createdBook.FilePath = newFiles[0].FilePath
		} else {
			createdBook.FilePath = filesCommonDir(newFiles)
		}
		database.GlobalStore.UpdateBook(createdBook.ID, createdBook)
	}

	// 7. Also update the source book's FilePath from its remaining files
	remainingFiles, _ := database.GlobalStore.GetBookFiles(sourceBook.ID)
	if len(remainingFiles) > 0 {
		if len(remainingFiles) == 1 {
			sourceBook.FilePath = remainingFiles[0].FilePath
		} else {
			sourceBook.FilePath = filesCommonDir(remainingFiles)
		}
		database.GlobalStore.UpdateBook(sourceBook.ID, sourceBook)
	}

	c.JSON(http.StatusOK, gin.H{
		"book":             createdBook,
		"version_group_id": versionGroupID,
		"segments_moved":   len(req.SegmentIDs),
	})
}

// splitSegmentsToBooks splits selected segments out of a multi-file book into
// independent new books (one per segment), extracting titles from filenames.
// Unlike splitVersion, the new books are NOT version-linked to the source.
func (s *Server) splitSegmentsToBooks(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		SegmentIDs []string `json:"segment_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.SegmentIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "segment_ids must not be empty"})
		return
	}

	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	sourceBook, err := database.GlobalStore.GetBookByID(id)
	if err != nil || sourceBook == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
		return
	}

	// Build a lookup of file ID → BookFile
	allFiles, err := database.GlobalStore.GetBookFiles(sourceBook.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list book files"})
		return
	}
	fileMap := make(map[string]database.BookFile, len(allFiles))
	for _, f := range allFiles {
		fileMap[f.ID] = f
	}

	// Create one new book per selected file
	var createdBooks []interface{}
	for _, fileID := range req.SegmentIDs {
		f, ok := fileMap[fileID]
		if !ok {
			continue
		}

		// Extract title from file name
		// e.g. "01 ASoIaF 1 - A Game of Thrones.m4b" → "A Game of Thrones"
		title := extractTitleFromSegmentFilename(filepath.Base(f.FilePath))
		if title == "" {
			title = sourceBook.Title + " (split)"
		}

		durationSec := f.Duration / 1000 // BookFile stores ms
		newBook := &database.Book{
			Title:     title,
			AuthorID:  sourceBook.AuthorID,
			SeriesID:  sourceBook.SeriesID,
			FilePath:  f.FilePath,
			Format:    f.Format,
			Narrator:  sourceBook.Narrator,
			Language:  sourceBook.Language,
			Publisher: sourceBook.Publisher,
			Duration:  &durationSec,
			FileSize:  &f.FileSize,
		}

		created, createErr := database.GlobalStore.CreateBook(newBook)
		if createErr != nil {
			log.Printf("[WARN] splitSegmentsToBooks: failed to create book for file %s: %v", fileID, createErr)
			continue
		}

		// Copy book_authors from source
		if authors, aErr := database.GlobalStore.GetBookAuthors(sourceBook.ID); aErr == nil && len(authors) > 0 {
			var newAuthors []database.BookAuthor
			for _, ba := range authors {
				newAuthors = append(newAuthors, database.BookAuthor{
					BookID:   created.ID,
					AuthorID: ba.AuthorID,
					Role:     ba.Role,
				})
			}
			_ = database.GlobalStore.SetBookAuthors(created.ID, newAuthors)
		}

		// Move the file to the new book
		_ = database.GlobalStore.MoveBookFilesToBook([]string{fileID}, sourceBook.ID, created.ID)

		// Reassign external ID mappings (iTunes PIDs) that belong to the moved file
		reassignExternalIDsForFiles(sourceBook.ID, created.ID, []database.BookFile{f})

		createdBooks = append(createdBooks, created)
	}

	// Update source book's FilePath from remaining files
	remainingFiles, _ := database.GlobalStore.GetBookFiles(sourceBook.ID)
	if len(remainingFiles) > 0 {
		if len(remainingFiles) == 1 {
			sourceBook.FilePath = remainingFiles[0].FilePath
		} else {
			sourceBook.FilePath = filesCommonDir(remainingFiles)
		}
		database.GlobalStore.UpdateBook(sourceBook.ID, sourceBook)
	}

	c.JSON(http.StatusOK, gin.H{
		"created_books": createdBooks,
		"count":         len(createdBooks),
	})
}

// extractTitleFromSegmentFilename tries to extract a meaningful book title
// from a segment filename like "01 ASoIaF 1 - A Game of Thrones.m4b".
func extractTitleFromSegmentFilename(filename string) string {
	// Strip extension
	name := strings.TrimSuffix(filename, filepath.Ext(filename))

	// Try to find title after " - " separator (common pattern)
	if idx := strings.Index(name, " - "); idx >= 0 {
		title := strings.TrimSpace(name[idx+3:])
		if title != "" {
			return title
		}
	}

	// Try after " – " (em dash)
	if idx := strings.Index(name, " – "); idx >= 0 {
		title := strings.TrimSpace(name[idx+len(" – "):])
		if title != "" {
			return title
		}
	}

	// Strip leading track numbers like "01 ", "01. "
	stripped := regexp.MustCompile(`^\d{1,3}[\s.\-]+`).ReplaceAllString(name, "")
	if stripped != "" {
		return strings.TrimSpace(stripped)
	}

	return name
}

// moveSegments moves segments from one book to another within the same version group.
func (s *Server) moveSegments(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		SegmentIDs   []string `json:"segment_ids" binding:"required"`
		TargetBookID string   `json:"target_book_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.SegmentIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "segment_ids must not be empty"})
		return
	}

	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// 1. Get source and target books
	sourceBook, err := database.GlobalStore.GetBookByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "source audiobook not found"})
		return
	}

	targetBook, err := database.GlobalStore.GetBookByID(req.TargetBookID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "target audiobook not found"})
		return
	}

	// 2. Verify both books are in the same version group
	if sourceBook.VersionGroupID == nil || targetBook.VersionGroupID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "both books must be in a version group"})
		return
	}
	if *sourceBook.VersionGroupID != *targetBook.VersionGroupID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "books must be in the same version group"})
		return
	}

	// 3. Verify the files belong to the source book
	sourceFiles, err := database.GlobalStore.GetBookFiles(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list source book files"})
		return
	}
	sourceFileMap := make(map[string]bool, len(sourceFiles))
	for _, f := range sourceFiles {
		sourceFileMap[f.ID] = true
	}
	for _, segID := range req.SegmentIDs {
		if !sourceFileMap[segID] {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("file %s does not belong to source book", segID)})
			return
		}
	}

	// 4. Collect the files being moved (for external ID reassignment)
	var movedFiles []database.BookFile
	movedSet := make(map[string]bool, len(req.SegmentIDs))
	for _, sid := range req.SegmentIDs {
		movedSet[sid] = true
	}
	for _, f := range sourceFiles {
		if movedSet[f.ID] {
			movedFiles = append(movedFiles, f)
		}
	}

	// 5. Move files
	if err := database.GlobalStore.MoveBookFilesToBook(req.SegmentIDs, id, req.TargetBookID); err != nil {
		internalError(c, "failed to move files", err)
		return
	}

	// 6. Reassign external ID mappings (iTunes PIDs) for moved files
	reassignExternalIDsForFiles(id, req.TargetBookID, movedFiles)

	c.JSON(http.StatusOK, gin.H{
		"segments_moved": len(req.SegmentIDs),
		"source_book_id": id,
		"target_book_id": req.TargetBookID,
	})
}

// reassignExternalIDsForFiles moves external ID mappings (iTunes PIDs) from
// sourceBookID to targetBookID for the given files. It matches by file_path or
// ITunesPersistentID on the external_id_map entries.
func reassignExternalIDsForFiles(sourceBookID, targetBookID string, files []database.BookFile) {
	eidStore := asExternalIDStore(database.GlobalStore)
	if eidStore == nil {
		return
	}

	mappings, err := eidStore.GetExternalIDsForBook(sourceBookID)
	if err != nil || len(mappings) == 0 {
		return
	}

	// Build lookup sets from the moved files
	movedPaths := make(map[string]bool, len(files))
	movedPIDs := make(map[string]bool, len(files))
	for _, f := range files {
		if f.FilePath != "" {
			movedPaths[f.FilePath] = true
		}
		if f.ITunesPersistentID != "" {
			movedPIDs[f.ITunesPersistentID] = true
		}
	}

	// Collect only the mappings that belong to the moved files
	var toMove []database.ExternalIDMapping
	for _, m := range mappings {
		if (m.FilePath != "" && movedPaths[m.FilePath]) ||
			(m.ExternalID != "" && movedPIDs[m.ExternalID]) {
			toMove = append(toMove, m)
		}
	}
	if len(toMove) == 0 {
		return
	}

	// Reassign each mapping: delete old reverse key, update primary, add new reverse key
	for _, m := range toMove {
		oldReverseKey := fmt.Sprintf("ext_id:book:%s:%s:%s", sourceBookID, m.Source, m.ExternalID)
		_ = database.GlobalStore.DeleteRaw(oldReverseKey)

		m.BookID = targetBookID
		if createErr := eidStore.CreateExternalIDMapping(&m); createErr != nil {
			log.Printf("[WARN] reassignExternalIDsForFiles: failed to reassign %s:%s to %s: %v",
				m.Source, m.ExternalID, targetBookID, createErr)
		}
	}

	log.Printf("[INFO] reassigned %d external ID mapping(s) from book %s to %s",
		len(toMove), sourceBookID, targetBookID)
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
		internalError(c, "failed to parse filename", err)
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
		log.Printf("[ERROR] connection test failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "connection test failed", "success": false})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "OpenAI connection successful"})
}

// testMetadataSource tests a metadata source API key by performing a simple search.
func (s *Server) testMetadataSource(c *gin.Context) {
	var req struct {
		SourceID string `json:"source_id"`
		APIKey   string `json:"api_key"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.SourceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source_id is required", "success": false})
		return
	}
	if req.APIKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "api_key is required", "success": false})
		return
	}

	testQuery := "The Hobbit" // well-known book for test queries

	switch req.SourceID {
	case "google-books":
		client := metadata.NewGoogleBooksClient(req.APIKey)
		results, err := client.SearchByTitle(testQuery)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "error": fmt.Sprintf("Google Books API error: %v", err)})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": fmt.Sprintf("Google Books connection successful (%d results)", len(results))})

	case "hardcover":
		client := metadata.NewHardcoverClient(req.APIKey)
		results, err := client.SearchByTitle(testQuery)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "error": fmt.Sprintf("Hardcover API error: %v", err)})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": fmt.Sprintf("Hardcover connection successful (%d results)", len(results))})

	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("unknown source: %s", req.SourceID), "success": false})
	}
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
	parser := newAIParser(config.AppConfig.OpenAIAPIKey, config.AppConfig.EnableAIParsing)
	if !parser.IsEnabled() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "AI parsing is not enabled or API key not configured"})
		return
	}

	// Build rich context for the AI parser
	abCtx := ai.AudiobookContext{
		FilePath: book.FilePath,
		Title:    book.Title,
	}
	if book.Narrator != nil {
		abCtx.Narrator = *book.Narrator
	}
	if book.Duration != nil {
		abCtx.TotalDuration = *book.Duration
	}
	// Resolve author name from author_id
	if book.AuthorID != nil {
		if author, err := database.GlobalStore.GetAuthorByID(*book.AuthorID); err == nil {
			abCtx.AuthorName = author.Name
		}
	}

	// Parse with AI using full context
	metadata, err := parser.ParseAudiobook(c.Request.Context(), abCtx)
	if err != nil {
		internalError(c, "failed to parse audiobook", err)
		return
	}

	// Build payload for the update service (routes through AudiobookService
	// which handles "&" splitting for authors/narrators, junction tables, etc.)
	payload := map[string]any{}
	if metadata.Title != "" {
		payload["title"] = metadata.Title
	}
	if metadata.Author != "" {
		payload["author_name"] = metadata.Author
	}
	if metadata.Narrator != "" {
		payload["narrator"] = metadata.Narrator
	}
	if metadata.Publisher != "" {
		payload["publisher"] = metadata.Publisher
	}
	if metadata.Year > 0 {
		payload["audiobook_release_year"] = metadata.Year
	}
	if metadata.Series != "" {
		payload["series_name"] = metadata.Series
	}
	if metadata.SeriesNum > 0 {
		payload["series_sequence"] = metadata.SeriesNum
	}

	// Route through the service layer for proper multi-author/narrator handling
	updatedBook, err := s.audiobookUpdateService.UpdateAudiobook(id, payload)
	if err != nil {
		internalError(c, "failed to update audiobook", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "audiobook updated with AI-parsed metadata",
		"book":       enrichBookForResponse(updatedBook),
		"confidence": metadata.Confidence,
	})
}

// getDashboard returns dashboard statistics with size and format distributions
func (s *Server) getDashboard(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Check cache first
	if cached, ok := s.dashboardCache.Get("dashboard"); ok {
		LogServiceCacheHit("Dashboard", "dashboard")
		c.JSON(http.StatusOK, cached)
		return
	}
	LogServiceCacheMiss("Dashboard", "dashboard")

	// Use SQL aggregation instead of loading all books
	stats, err := database.GlobalStore.GetDashboardStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve dashboard stats"})
		return
	}

	// Get recent operations
	recentOps, err := database.GlobalStore.GetRecentOperations(5)
	if err != nil {
		recentOps = []database.Operation{}
	}

	result := gin.H{
		"formatDistribution": stats.FormatDistribution,
		"stateDistribution":  stats.StateDistribution,
		"recentOperations":   recentOps,
		"totalSize":          stats.TotalSize,
		"totalBooks":         stats.TotalBooks,
		"totalDuration":      stats.TotalDuration,
	}

	s.dashboardCache.Set("dashboard", result)
	c.JSON(http.StatusOK, result)
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
		internalError(c, "failed to get blocked hashes", err)
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
		internalError(c, "failed to add blocked hash", err)
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
		internalError(c, "failed to remove blocked hash", err)
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
		Port:         "8484",
		Host:         "localhost",
		ReadTimeout:  15 * time.Second,  // Allow slow clients without stalling forever
		WriteTimeout: 0,                 // Disable write timeout so SSE streams stay open
		IdleTimeout:  120 * time.Second, // 2 minute idle timeout
	}
}

func ptrStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// listTasks returns all registered tasks with their status and schedule.
func (s *Server) listTasks(c *gin.Context) {
	if s.scheduler == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "scheduler not initialized"})
		return
	}
	c.JSON(http.StatusOK, s.scheduler.ListTasks())
}

// runTask triggers a task by name.
func (s *Server) runTask(c *gin.Context) {
	if s.scheduler == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "scheduler not initialized"})
		return
	}
	name := c.Param("name")
	op, err := s.scheduler.RunTask(name)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if op == nil {
		c.JSON(http.StatusAccepted, gin.H{"message": "task triggered"})
		return
	}
	c.JSON(http.StatusAccepted, op)
}

// updateTaskConfig updates schedule config for a task.
func (s *Server) updateTaskConfig(c *gin.Context) {
	name := c.Param("name")

	var req struct {
		Enabled                *bool `json:"enabled"`
		IntervalMinutes        *int  `json:"interval_minutes"`
		RunOnStartup           *bool `json:"run_on_startup"`
		RunInMaintenanceWindow *bool `json:"run_in_maintenance_window"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Map task name to config fields and apply
	switch name {
	case "dedup_refresh":
		if req.Enabled != nil {
			config.AppConfig.ScheduledDedupRefreshEnabled = *req.Enabled
		}
		if req.IntervalMinutes != nil {
			config.AppConfig.ScheduledDedupRefreshInterval = *req.IntervalMinutes
		}
		if req.RunOnStartup != nil {
			config.AppConfig.ScheduledDedupRefreshOnStartup = *req.RunOnStartup
		}
		if req.RunInMaintenanceWindow != nil {
			config.AppConfig.MaintenanceWindowDedupRefresh = *req.RunInMaintenanceWindow
		}
	case "author_split_scan":
		if req.Enabled != nil {
			config.AppConfig.ScheduledAuthorSplitEnabled = *req.Enabled
		}
		if req.IntervalMinutes != nil {
			config.AppConfig.ScheduledAuthorSplitInterval = *req.IntervalMinutes
		}
		if req.RunOnStartup != nil {
			config.AppConfig.ScheduledAuthorSplitOnStartup = *req.RunOnStartup
		}
		if req.RunInMaintenanceWindow != nil {
			config.AppConfig.MaintenanceWindowAuthorSplit = *req.RunInMaintenanceWindow
		}
	case "db_optimize":
		if req.Enabled != nil {
			config.AppConfig.ScheduledDbOptimizeEnabled = *req.Enabled
		}
		if req.IntervalMinutes != nil {
			config.AppConfig.ScheduledDbOptimizeInterval = *req.IntervalMinutes
		}
		if req.RunOnStartup != nil {
			config.AppConfig.ScheduledDbOptimizeOnStartup = *req.RunOnStartup
		}
		if req.RunInMaintenanceWindow != nil {
			config.AppConfig.MaintenanceWindowDbOptimize = *req.RunInMaintenanceWindow
		}
	case "metadata_refresh":
		if req.Enabled != nil {
			config.AppConfig.ScheduledMetadataRefreshEnabled = *req.Enabled
		}
		if req.IntervalMinutes != nil {
			config.AppConfig.ScheduledMetadataRefreshInterval = *req.IntervalMinutes
		}
		if req.RunOnStartup != nil {
			config.AppConfig.ScheduledMetadataRefreshOnStartup = *req.RunOnStartup
		}
		if req.RunInMaintenanceWindow != nil {
			config.AppConfig.MaintenanceWindowMetadataRefresh = *req.RunInMaintenanceWindow
		}
	case "itunes_sync":
		if req.Enabled != nil {
			config.AppConfig.ITunesSyncEnabled = *req.Enabled
		}
		if req.IntervalMinutes != nil {
			config.AppConfig.ITunesSyncInterval = *req.IntervalMinutes
		}
	case "series_prune":
		if req.Enabled != nil {
			config.AppConfig.ScheduledSeriesPruneEnabled = *req.Enabled
		}
		if req.IntervalMinutes != nil {
			config.AppConfig.ScheduledSeriesPruneInterval = *req.IntervalMinutes
		}
		if req.RunOnStartup != nil {
			config.AppConfig.ScheduledSeriesPruneOnStartup = *req.RunOnStartup
		}
		if req.RunInMaintenanceWindow != nil {
			config.AppConfig.MaintenanceWindowSeriesPrune = *req.RunInMaintenanceWindow
		}
	case "purge_deleted":
		if req.IntervalMinutes != nil {
			// purge interval is fixed at 6h, but we can update retention days
		}
		if req.RunInMaintenanceWindow != nil {
			config.AppConfig.MaintenanceWindowPurgeDeleted = *req.RunInMaintenanceWindow
		}
	case "tombstone_cleanup":
		if req.RunInMaintenanceWindow != nil {
			config.AppConfig.MaintenanceWindowTombstoneCleanup = *req.RunInMaintenanceWindow
		}
	case "reconcile_scan":
		if req.Enabled != nil {
			config.AppConfig.ScheduledReconcileEnabled = *req.Enabled
		}
		if req.RunInMaintenanceWindow != nil {
			config.AppConfig.MaintenanceWindowReconcile = *req.RunInMaintenanceWindow
		}
	case "purge_old_logs":
		if req.RunInMaintenanceWindow != nil {
			config.AppConfig.MaintenanceWindowPurgeOldLogs = *req.RunInMaintenanceWindow
		}
	case "library_scan":
		if req.RunInMaintenanceWindow != nil {
			config.AppConfig.MaintenanceWindowLibraryScan = *req.RunInMaintenanceWindow
		}
	case "library_organize":
		if req.RunInMaintenanceWindow != nil {
			config.AppConfig.MaintenanceWindowLibraryOrganize = *req.RunInMaintenanceWindow
		}
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("task %q config is not configurable", name)})
		return
	}

	// Persist to database
	if database.GlobalStore != nil {
		if err := config.SaveConfigToDatabase(database.GlobalStore); err != nil {
			log.Printf("[WARN] Failed to save task config: %v", err)
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "task config updated"})
}

// runMaintenanceWindowNow triggers the full maintenance window sequence immediately.
func (s *Server) runMaintenanceWindowNow(c *gin.Context) {
	if s.scheduler == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "scheduler not initialized"})
		return
	}
	ctx := context.WithValue(c.Request.Context(), ignoreWindowKey, true)
	if err := s.scheduler.RunMaintenanceWindow(ctx); err != nil {
		internalError(c, "failed to run maintenance", err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"message": "maintenance window triggered"})
}

func (s *Server) aiReviewDuplicateAuthors(c *gin.Context) {
	parser := newAIParser(config.AppConfig.OpenAIAPIKey, config.AppConfig.EnableAIParsing)
	if !parser.IsEnabled() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "AI parsing is not enabled"})
		return
	}

	if operations.GlobalQueue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	// Parse optional mode from request body
	var reqBody struct {
		Mode string `json:"mode"`
	}
	_ = c.ShouldBindJSON(&reqBody)
	mode := reqBody.Mode
	if mode == "" {
		mode = "groups"
	}
	if mode != "full" && mode != "groups" {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid mode %q; must be full or groups", mode)})
		return
	}

	store := database.GlobalStore

	// Check for an already-running ai-author-review of the same mode — block concurrent same-mode runs
	opType := "ai-author-review-" + mode
	recentOps, _, _ := store.ListOperations(50, 0)
	for _, existing := range recentOps {
		if existing.Type == opType && (existing.Status == "pending" || existing.Status == "running") {
			c.JSON(http.StatusAccepted, existing)
			return
		}
	}

	// For groups mode, we need dedup groups — use cache if available, otherwise compute inline
	var dedupGroups []AuthorDedupGroup
	if mode == "groups" {
		cached, ok := s.dedupCache.Get("author-duplicates")
		if ok {
			groupsRaw, ok2 := cached["groups"]
			if ok2 {
				groupsJSON, err := json.Marshal(groupsRaw)
				if err == nil {
					_ = json.Unmarshal(groupsJSON, &dedupGroups)
				}
			}
		}
		if len(dedupGroups) == 0 {
			// Cache is cold — compute dedup groups inline instead of requiring a separate refresh
			authors, err := store.GetAllAuthors()
			if err != nil {
				internalError(c, "failed to fetch authors", err)
				return
			}
			bookCounts, err := store.GetAllAuthorBookCounts()
			if err != nil {
				internalError(c, "failed to fetch book counts", err)
				return
			}
			bookCountFn := func(authorID int) int { return bookCounts[authorID] }
			dedupGroups = FindDuplicateAuthors(authors, 0.9, bookCountFn, nil)
			// Warm the cache for subsequent requests
			result := gin.H{"groups": dedupGroups, "count": len(dedupGroups)}
			s.dedupCache.SetWithTTL("author-duplicates", result, 30*time.Minute)
		}
		if len(dedupGroups) == 0 {
			c.JSON(http.StatusOK, gin.H{"message": "no duplicate groups to review"})
			return
		}
	}

	opID := ulid.Make().String()
	op, err := store.CreateOperation(opID, opType, nil)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		switch mode {
		case "groups":
			return s.aiReviewGroupsMode(ctx, progress, parser, store, opID, dedupGroups)
		case "full":
			return s.aiReviewFullMode(ctx, progress, parser, store, opID)
		}
		return fmt.Errorf("unknown mode: %s", mode)
	}

	if err := operations.GlobalQueue.Enqueue(opID, opType, operations.PriorityNormal, operationFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	c.JSON(http.StatusAccepted, op)
}

// aiReviewGroupsMode is the existing Groups mode — local heuristics build groups, AI validates.
func (s *Server) aiReviewGroupsMode(ctx context.Context, progress operations.ProgressReporter, parser aiParser, store database.Store, opID string, dedupGroups []AuthorDedupGroup) error {
	_ = progress.Log("info", fmt.Sprintf("Starting AI review (groups mode) of %d duplicate author groups", len(dedupGroups)), nil)
	_ = progress.UpdateProgress(0, len(dedupGroups), "Building AI review input...")

	var inputs []ai.AuthorDedupInput
	for i, group := range dedupGroups {
		var variantNames []string
		for _, v := range group.Variants {
			variantNames = append(variantNames, v.Name)
		}
		var sampleTitles []string
		if group.Canonical.ID > 0 {
			books, err := store.GetBooksByAuthorIDWithRole(group.Canonical.ID)
			if err == nil {
				for j, b := range books {
					if j >= 3 {
						break
					}
					sampleTitles = append(sampleTitles, b.Title)
				}
			}
		}
		inputs = append(inputs, ai.AuthorDedupInput{
			Index:         i,
			CanonicalName: NormalizeAuthorName(group.Canonical.Name),
			VariantNames:  variantNames,
			BookCount:     group.BookCount,
			SampleTitles:  sampleTitles,
		})
	}

	_ = progress.UpdateProgress(10, 100, fmt.Sprintf("Sending %d groups to AI for review...", len(inputs)))

	suggestions, err := parser.ReviewAuthorDuplicates(ctx, inputs)
	if err != nil {
		return fmt.Errorf("AI review failed: %w", err)
	}

	// Normalize initials formatting in AI-returned canonical names
	for i := range suggestions {
		suggestions[i].CanonicalName = NormalizeAuthorName(suggestions[i].CanonicalName)
	}

	_ = progress.Log("info", fmt.Sprintf("Received %d suggestions from AI", len(suggestions)), nil)

	resultPayload := map[string]interface{}{
		"mode":        "groups",
		"suggestions": suggestions,
		"groups":      dedupGroups,
	}
	resultJSON, err := json.Marshal(resultPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal suggestions: %w", err)
	}
	if err := store.UpdateOperationResultData(opID, string(resultJSON)); err != nil {
		return fmt.Errorf("failed to store results: %w", err)
	}

	_ = progress.UpdateProgress(100, 100, fmt.Sprintf("AI review complete: %d suggestions", len(suggestions)))
	return nil
}

// aiReviewFullMode sends all authors to AI for duplicate discovery.
func (s *Server) aiReviewFullMode(ctx context.Context, progress operations.ProgressReporter, parser aiParser, store database.Store, opID string) error {
	_ = progress.Log("info", "Starting AI review (full mode) — discovering duplicates from all authors", nil)
	_ = progress.UpdateProgress(0, 100, "Loading all authors...")

	allAuthors, err := store.GetAllAuthors()
	if err != nil {
		return fmt.Errorf("failed to get authors: %w", err)
	}

	_ = progress.Log("info", fmt.Sprintf("Building discovery input for %d authors", len(allAuthors)), nil)
	_ = progress.UpdateProgress(5, 100, fmt.Sprintf("Building input for %d authors...", len(allAuthors)))

	var inputs []ai.AuthorDiscoveryInput
	for _, author := range allAuthors {
		var sampleTitles []string
		books, err := store.GetBooksByAuthorIDWithRole(author.ID)
		if err == nil {
			for j, b := range books {
				if j >= 3 {
					break
				}
				sampleTitles = append(sampleTitles, b.Title)
			}
		}
		inputs = append(inputs, ai.AuthorDiscoveryInput{
			ID:           author.ID,
			Name:         author.Name,
			BookCount:    len(books),
			SampleTitles: sampleTitles,
		})
	}

	_ = progress.UpdateProgress(10, 100, fmt.Sprintf("Sending %d authors to AI for discovery...", len(inputs)))

	discoveries, err := parser.DiscoverAuthorDuplicates(ctx, inputs)
	if err != nil {
		return fmt.Errorf("AI discovery failed: %w", err)
	}

	_ = progress.Log("info", fmt.Sprintf("AI discovered %d duplicate groups", len(discoveries)), nil)

	// Build author ID→Author map for lookup
	authorMap := make(map[int]database.Author)
	for _, a := range allAuthors {
		authorMap[a.ID] = a
	}

	// Convert discovery suggestions to standard AuthorDedupSuggestion + AuthorDedupGroup format
	var suggestions []ai.AuthorDedupSuggestion
	var groups []AuthorDedupGroup
	for _, disc := range discoveries {
		if len(disc.AuthorIDs) < 2 && disc.Action != "rename" {
			continue
		}
		// First ID = canonical, rest = variants
		canonicalID := disc.AuthorIDs[0]
		canonical, ok := authorMap[canonicalID]
		if !ok {
			continue
		}
		var variants []database.Author
		for _, aid := range disc.AuthorIDs[1:] {
			if a, ok := authorMap[aid]; ok {
				variants = append(variants, a)
			}
		}
		groups = append(groups, AuthorDedupGroup{
			Canonical: canonical,
			Variants:  variants,
			BookCount: disc.AuthorIDs[0], // placeholder; we just need a count
		})
		// Fix book count — count books for all authors in the group
		totalBooks := 0
		for _, aid := range disc.AuthorIDs {
			bks, err := store.GetBooksByAuthorIDWithRole(aid)
			if err == nil {
				totalBooks += len(bks)
			}
		}
		groups[len(groups)-1].BookCount = totalBooks

		suggestions = append(suggestions, ai.AuthorDedupSuggestion{
			GroupIndex:    len(groups) - 1, // index into groups slice, not discoveries
			Action:        disc.Action,
			CanonicalName: NormalizeAuthorName(disc.CanonicalName),
			Reason:        disc.Reason,
			Confidence:    disc.Confidence,
			IsNarrator:    disc.IsNarrator,
			IsPublisher:   disc.IsPublisher,
		})
	}

	resultPayload := map[string]interface{}{
		"mode":        "full",
		"suggestions": suggestions,
		"groups":      groups,
	}
	resultJSON, err := json.Marshal(resultPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal results: %w", err)
	}
	if err := store.UpdateOperationResultData(opID, string(resultJSON)); err != nil {
		return fmt.Errorf("failed to store results: %w", err)
	}

	_ = progress.UpdateProgress(100, 100, fmt.Sprintf("AI discovery complete: %d groups found", len(groups)))
	return nil
}

// Author alias handlers

func (s *Server) getAuthorAliases(c *gin.Context) {
	authorID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid author ID"})
		return
	}
	aliases, err := database.GlobalStore.GetAuthorAliases(authorID)
	if err != nil {
		internalError(c, "failed to get author aliases", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"aliases": aliases})
}

func (s *Server) createAuthorAlias(c *gin.Context) {
	authorID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid author ID"})
		return
	}
	var req struct {
		AliasName string `json:"alias_name"`
		AliasType string `json:"alias_type"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.AliasName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "alias_name is required"})
		return
	}
	if req.AliasType == "" {
		req.AliasType = "alias"
	}
	alias, err := database.GlobalStore.CreateAuthorAlias(authorID, req.AliasName, req.AliasType)
	if err != nil {
		internalError(c, "failed to create author alias", err)
		return
	}
	c.JSON(http.StatusCreated, alias)
}

func (s *Server) deleteAuthorAlias(c *gin.Context) {
	aliasID, err := strconv.Atoi(c.Param("aliasId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid alias ID"})
		return
	}
	if err := database.GlobalStore.DeleteAuthorAlias(aliasID); err != nil {
		internalError(c, "failed to delete author alias", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (s *Server) getOperationResult(c *gin.Context) {
	id := c.Param("id")
	store := database.GlobalStore
	op, err := store.GetOperationByID(id)
	if err != nil {
		internalError(c, "failed to get operation", err)
		return
	}
	if op == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "operation not found"})
		return
	}

	if op.ResultData == nil {
		c.JSON(http.StatusOK, gin.H{"result_data": nil})
		return
	}

	// Parse the JSON result data to return as structured JSON
	var resultData json.RawMessage
	if err := json.Unmarshal([]byte(*op.ResultData), &resultData); err != nil {
		c.JSON(http.StatusOK, gin.H{"result_data": *op.ResultData})
		return
	}

	c.JSON(http.StatusOK, gin.H{"result_data": resultData})
}

func (s *Server) applyAIAuthorReview(c *gin.Context) {
	var req struct {
		Suggestions []struct {
			GroupIndex    int    `json:"group_index"`
			Action        string `json:"action"`
			CanonicalName string `json:"canonical_name"`
			KeepID        int    `json:"keep_id"`
			MergeIDs      []int  `json:"merge_ids"`
			Rename        bool   `json:"rename"`
		} `json:"suggestions"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.Suggestions) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no suggestions provided"})
		return
	}

	if operations.GlobalQueue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	store := database.GlobalStore
	opID := ulid.Make().String()
	op, err := store.CreateOperation(opID, "ai-author-merge-apply", nil)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	suggestions := req.Suggestions

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		total := len(suggestions)
		applied := 0
		var applyErrors []string

		_ = progress.Log("info", fmt.Sprintf("Starting AI author review apply: %d suggestion(s)", total), nil)

		for i, sug := range suggestions {
			if progress.IsCanceled() {
				_ = progress.Log("warn", "Operation cancelled by user", nil)
				return fmt.Errorf("cancelled")
			}

			_ = progress.UpdateProgress(i, total, fmt.Sprintf("Applying suggestion %d/%d...", i+1, total))

			switch sug.Action {
			case "skip":
				_ = progress.Log("info", fmt.Sprintf("Skipped group %d", sug.GroupIndex), nil)
				continue

			case "rename":
				if sug.KeepID > 0 && sug.CanonicalName != "" {
					if err := store.UpdateAuthorName(sug.KeepID, NormalizeAuthorName(sug.CanonicalName)); err != nil {
						applyErrors = append(applyErrors, fmt.Sprintf("rename author %d: %v", sug.KeepID, err))
					} else {
						applied++
						_ = progress.Log("info", fmt.Sprintf("Renamed author %d to \"%s\"", sug.KeepID, sug.CanonicalName), nil)
					}
				}

			case "merge":
				// Rename canonical if needed
				if sug.Rename && sug.KeepID > 0 && sug.CanonicalName != "" {
					if err := store.UpdateAuthorName(sug.KeepID, NormalizeAuthorName(sug.CanonicalName)); err != nil {
						applyErrors = append(applyErrors, fmt.Sprintf("rename before merge %d: %v", sug.KeepID, err))
					}
				}

				// Merge variant authors
				for _, mergeID := range sug.MergeIDs {
					if mergeID == sug.KeepID {
						continue
					}
					books, err := store.GetBooksByAuthorIDWithRole(mergeID)
					if err != nil {
						applyErrors = append(applyErrors, fmt.Sprintf("get books for author %d: %v", mergeID, err))
						continue
					}

					// Snapshot affected books
					_ = progress.Log("info", fmt.Sprintf("Snapshotting %d books before merge of author %d", len(books), mergeID), nil)

					for _, book := range books {
						bookAuthors, err := store.GetBookAuthors(book.ID)
						if err != nil {
							continue
						}
						hasKeep := false
						for _, ba := range bookAuthors {
							if ba.AuthorID == sug.KeepID {
								hasKeep = true
								break
							}
						}
						var newAuthors []database.BookAuthor
						for _, ba := range bookAuthors {
							if ba.AuthorID == mergeID {
								if !hasKeep {
									ba.AuthorID = sug.KeepID
									newAuthors = append(newAuthors, ba)
									hasKeep = true
								}
							} else {
								newAuthors = append(newAuthors, ba)
							}
						}
						if err := store.SetBookAuthors(book.ID, newAuthors); err != nil {
							applyErrors = append(applyErrors, fmt.Sprintf("update book %s: %v", book.ID, err))
						}
					}

					if err := store.DeleteAuthor(mergeID); err != nil {
						applyErrors = append(applyErrors, fmt.Sprintf("delete author %d: %v", mergeID, err))
					} else {
						_ = store.CreateAuthorTombstone(mergeID, sug.KeepID)
					}
				}
				applied++
				_ = progress.Log("info", fmt.Sprintf("Merged group %d: %d variants into \"%s\"", sug.GroupIndex, len(sug.MergeIDs), sug.CanonicalName), nil)

			case "alias":
				// Keep canonical author, add variants as aliases instead of merging
				if sug.KeepID > 0 && sug.CanonicalName != "" {
					if sug.Rename {
						if err := store.UpdateAuthorName(sug.KeepID, NormalizeAuthorName(sug.CanonicalName)); err != nil {
							applyErrors = append(applyErrors, fmt.Sprintf("rename for alias %d: %v", sug.KeepID, err))
						}
					}
					for _, mergeID := range sug.MergeIDs {
						if mergeID == sug.KeepID {
							continue
						}
						variant, err := store.GetAuthorByID(mergeID)
						if err != nil || variant == nil {
							continue
						}
						if _, err := store.CreateAuthorAlias(sug.KeepID, variant.Name, "pen_name"); err != nil {
							applyErrors = append(applyErrors, fmt.Sprintf("create alias for author %d: %v", sug.KeepID, err))
						}
						// Re-link books and delete the variant author
						books, err := store.GetBooksByAuthorIDWithRole(mergeID)
						if err != nil {
							continue
						}
						for _, book := range books {
							bookAuthors, err := store.GetBookAuthors(book.ID)
							if err != nil {
								continue
							}
							hasKeep := false
							for _, ba := range bookAuthors {
								if ba.AuthorID == sug.KeepID {
									hasKeep = true
									break
								}
							}
							var newAuthors []database.BookAuthor
							for _, ba := range bookAuthors {
								if ba.AuthorID == mergeID {
									if !hasKeep {
										ba.AuthorID = sug.KeepID
										newAuthors = append(newAuthors, ba)
										hasKeep = true
									}
								} else {
									newAuthors = append(newAuthors, ba)
								}
							}
							if err := store.SetBookAuthors(book.ID, newAuthors); err != nil {
								applyErrors = append(applyErrors, fmt.Sprintf("update book %s for alias: %v", book.ID, err))
							}
						}
						if err := store.DeleteAuthor(mergeID); err != nil {
							applyErrors = append(applyErrors, fmt.Sprintf("delete aliased author %d: %v", mergeID, err))
						} else {
							_ = store.CreateAuthorTombstone(mergeID, sug.KeepID)
						}
					}
					applied++
					_ = progress.Log("info", fmt.Sprintf("Created aliases for group %d: canonical \"%s\"", sug.GroupIndex, sug.CanonicalName), nil)
				}

			case "split":
				_ = progress.Log("info", fmt.Sprintf("Split action for group %d — manual intervention needed", sug.GroupIndex), nil)
				applied++
			}
		}

		s.dedupCache.InvalidateAll()

		resultMsg := fmt.Sprintf("AI review applied: %d actions, %d errors", applied, len(applyErrors))
		_ = progress.Log("info", resultMsg, nil)
		if len(applyErrors) > 0 {
			errDetail := strings.Join(applyErrors[:min(len(applyErrors), 10)], "; ")
			_ = progress.Log("warn", fmt.Sprintf("Errors: %s", errDetail), nil)
		}

		_ = progress.UpdateProgress(total, total, resultMsg)
		return nil
	}

	if err := operations.GlobalQueue.Enqueue(opID, "ai-author-merge-apply", operations.PriorityNormal, operationFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	c.JSON(http.StatusAccepted, op)
}

// getOperationChanges returns change tracking records for an operation.
func (s *Server) getOperationChanges(c *gin.Context) {
	id := c.Param("id")
	changes, err := database.GlobalStore.GetOperationChanges(id)
	if err != nil {
		internalError(c, "failed to get operation changes", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"changes": changes})
}

// revertOperation undoes all changes from a given operation.
func (s *Server) revertOperation(c *gin.Context) {
	id := c.Param("id")
	revertSvc := NewRevertService(database.GlobalStore)
	if err := revertSvc.RevertOperation(id); err != nil {
		internalError(c, "failed to revert operation", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "operation reverted successfully"})
}

// getBookChanges returns change tracking records for a book.
func (s *Server) getBookChanges(c *gin.Context) {
	id := c.Param("id")
	changes, err := database.GlobalStore.GetBookChanges(id)
	if err != nil {
		internalError(c, "failed to get book changes", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"changes": changes})
}

// --- AI Scan Pipeline Handlers ---

// startAIScan kicks off a new multi-pass AI author dedup scan.
func (s *Server) startAIScan(c *gin.Context) {
	if s.pipelineManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "AI scan pipeline not configured"})
		return
	}
	var req struct {
		Mode string `json:"mode"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		req.Mode = "realtime"
	}
	if req.Mode != "batch" && req.Mode != "realtime" {
		req.Mode = "realtime"
	}
	scan, err := s.pipelineManager.StartScan(c.Request.Context(), req.Mode)
	if err != nil {
		internalError(c, "failed to start AI scan", err)
		return
	}
	c.JSON(http.StatusAccepted, scan)
}

// listAIScans returns all AI scan pipeline runs.
func (s *Server) listAIScans(c *gin.Context) {
	if s.aiScanStore == nil {
		c.JSON(http.StatusOK, gin.H{"scans": []interface{}{}})
		return
	}
	scans, err := s.aiScanStore.ListScans()
	if err != nil {
		internalError(c, "failed to list AI scans", err)
		return
	}
	if scans == nil {
		scans = []database.Scan{}
	}
	c.JSON(http.StatusOK, gin.H{"scans": scans})
}

// getAIScan returns a single scan with its phases.
func (s *Server) getAIScan(c *gin.Context) {
	if s.aiScanStore == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "scan store not configured"})
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scan ID"})
		return
	}
	scan, err := s.aiScanStore.GetScan(id)
	if err != nil {
		internalError(c, "failed to get AI scan", err)
		return
	}
	if scan == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "scan not found"})
		return
	}
	phases, _ := s.aiScanStore.GetPhases(id)
	c.JSON(http.StatusOK, gin.H{"scan": scan, "phases": phases})
}

// getAIScanResults returns results for a scan, with optional agreement filter.
func (s *Server) getAIScanResults(c *gin.Context) {
	if s.aiScanStore == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "scan store not configured"})
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scan ID"})
		return
	}
	results, err := s.aiScanStore.GetScanResults(id)
	if err != nil {
		internalError(c, "failed to get AI scan results", err)
		return
	}

	// Optional agreement filter
	agreement := c.Query("agreement")
	if agreement != "" {
		var filtered []database.ScanResult
		for _, r := range results {
			if r.Agreement == agreement {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}

	if results == nil {
		results = []database.ScanResult{}
	}
	c.JSON(http.StatusOK, gin.H{"results": results})
}

// applyAIScanResults marks selected scan results as applied.
func (s *Server) applyAIScanResults(c *gin.Context) {
	if s.aiScanStore == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "scan store not configured"})
		return
	}
	scanID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scan ID"})
		return
	}
	var req struct {
		ResultIDs []int `json:"result_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	applied := 0
	var errors []string
	for _, resultID := range req.ResultIDs {
		if err := s.aiScanStore.MarkResultApplied(scanID, resultID); err != nil {
			errors = append(errors, fmt.Sprintf("result %d: %v", resultID, err))
		} else {
			applied++
		}
	}

	c.JSON(http.StatusOK, gin.H{"applied": applied, "errors": errors})
}

// deleteAIScan removes a scan and all its associated data.
func (s *Server) deleteAIScan(c *gin.Context) {
	if s.aiScanStore == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "scan store not configured"})
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scan ID"})
		return
	}
	if err := s.aiScanStore.DeleteScan(id); err != nil {
		internalError(c, "failed to delete AI scan", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// cancelAIScan cancels a running AI scan, including any in-flight batch jobs.
func (s *Server) cancelAIScan(c *gin.Context) {
	if s.pipelineManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "AI scan pipeline not configured"})
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scan ID"})
		return
	}
	if err := s.pipelineManager.CancelScan(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "canceled"})
}

// compareAIScans compares results between two scans.
func (s *Server) compareAIScans(c *gin.Context) {
	if s.aiScanStore == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "scan store not configured"})
		return
	}
	aID, err := strconv.Atoi(c.Query("a"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scan ID 'a'"})
		return
	}
	bID, err := strconv.Atoi(c.Query("b"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scan ID 'b'"})
		return
	}

	resultsA, _ := s.aiScanStore.GetScanResults(aID)
	resultsB, _ := s.aiScanStore.GetScanResults(bID)

	// Build comparison: new in B, resolved from A, unchanged
	aMap := make(map[string]database.ScanResult)
	for _, r := range resultsA {
		key := fmt.Sprintf("%s:%s", r.Suggestion.Action, r.Suggestion.CanonicalName)
		aMap[key] = r
	}

	var newInB, unchanged []database.ScanResult
	bSeen := make(map[string]bool)
	for _, r := range resultsB {
		key := fmt.Sprintf("%s:%s", r.Suggestion.Action, r.Suggestion.CanonicalName)
		bSeen[key] = true
		if _, found := aMap[key]; found {
			unchanged = append(unchanged, r)
		} else {
			newInB = append(newInB, r)
		}
	}

	var resolvedFromA []database.ScanResult
	for key, r := range aMap {
		if !bSeen[key] {
			resolvedFromA = append(resolvedFromA, r)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"new_in_b":        newInB,
		"resolved_from_a": resolvedFromA,
		"unchanged":       unchanged,
	})
}

// --- Preview Rename & Metadata Writeback Handlers ---

// previewRename returns current path, proposed path, and tag diff for a book.
func (s *Server) previewRename(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "book id is required"})
		return
	}

	svc := NewRenameService(database.GlobalStore)
	preview, err := svc.PreviewRename(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		internalError(c, "failed to preview rename", err)
		return
	}

	c.JSON(http.StatusOK, preview)
}

// applyRename executes the rename + tag write + DB update for a book.
func (s *Server) applyRename(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "book id is required"})
		return
	}

	// Create an operation for tracking and undo support
	opID := ulid.Make().String()
	op, err := database.GlobalStore.CreateOperation(opID, "rename", stringPtr(id))
	if err != nil {
		log.Printf("[ERROR] rename: failed to create operation: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create operation record"})
		return
	}

	svc := NewRenameService(database.GlobalStore)
	result, err := svc.ApplyRename(id, op.ID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		internalError(c, "failed to apply rename", err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// --- Preview Organize & Single-Book Organize Handlers ---

// previewOrganize returns a step-by-step preview of what organizing a single book would do.
func (s *Server) previewOrganize(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "book id is required"})
		return
	}

	svc := NewOrganizePreviewService(database.GlobalStore)
	preview, err := svc.PreviewOrganize(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		internalError(c, "failed to preview organize", err)
		return
	}

	c.JSON(http.StatusOK, preview)
}

// organizeBook executes the full organize pipeline for a single book.
// It uses the same logic as the batch organize: book_files for multi-file
// books, organizeDirectoryBook for directory-based books, and
// createOrganizedVersion for version-aware DB tracking. This correctly handles
// directory books and author-flat directories used by iTunes.
func (s *Server) organizeBook(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "book id is required"})
		return
	}

	// Create an operation for tracking and undo support
	opID := ulid.Make().String()
	op, err := database.GlobalStore.CreateOperation(opID, "organize", stringPtr(id))
	if err != nil {
		log.Printf("[ERROR] organize: failed to create operation: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create operation record"})
		return
	}

	book, err := database.GlobalStore.GetBookByID(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "book not found"})
			return
		}
		internalError(c, "failed to fetch book", err)
		return
	}

	oldPath := book.FilePath
	org := organizer.NewOrganizer(&config.AppConfig)
	log2 := logger.NewWithActivityLog("organize", database.GlobalStore)

	// Determine whether this is a directory-based (multi-file) book.
	// Prefer book_files count; fall back to os.Stat only when necessary.
	bookFiles, _ := database.GlobalStore.GetBookFiles(id)
	isDir := false
	if len(bookFiles) > 1 {
		isDir = true
	} else if len(bookFiles) == 0 {
		if info, statErr := os.Stat(oldPath); statErr == nil && info.IsDir() {
			isDir = true
		}
	} else if len(bookFiles) == 1 {
		// Single book_file entry — treat as file unless it has no extension
		if info, statErr := os.Stat(oldPath); statErr == nil && info.IsDir() {
			isDir = true
		}
	}

	alreadyInRoot := config.AppConfig.RootDir != "" && strings.HasPrefix(oldPath, config.AppConfig.RootDir)

	var newPath string
	if alreadyInRoot {
		newPath, err = s.organizeService.reOrganizeInPlace(book, log2)
	} else if isDir {
		newPath, err = s.organizeService.organizeDirectoryBook(org, book, log2)
	} else {
		newPath, _, err = org.OrganizeBook(book)
	}

	if err != nil {
		internalError(c, "failed to organize book", err)
		return
	}

	if oldPath == newPath {
		c.JSON(http.StatusOK, gin.H{
			"message":      "already organized",
			"book_id":      book.ID,
			"old_path":     oldPath,
			"new_path":     newPath,
			"operation_id": op.ID,
		})
		return
	}

	if alreadyInRoot {
		// Re-organized in place — stamp the existing record
		now := time.Now()
		book.LastOrganizeOperationID = &opID
		book.LastOrganizedAt = &now
		if _, updateErr := database.GlobalStore.UpdateBook(book.ID, book); updateErr != nil {
			log.Printf("[WARN] organize: failed to stamp book %s: %v", book.ID, updateErr)
		}
		_ = database.GlobalStore.CreateOperationChange(&database.OperationChange{
			ID:          ulid.Make().String(),
			OperationID: op.ID,
			BookID:      book.ID,
			ChangeType:  "organize_rename",
			FieldName:   "file_path",
			OldValue:    oldPath,
			NewValue:    newPath,
		})
		c.JSON(http.StatusOK, gin.H{
			"message":      fmt.Sprintf("re-organized: %s → %s", oldPath, newPath),
			"book_id":      book.ID,
			"old_path":     oldPath,
			"new_path":     newPath,
			"operation_id": op.ID,
		})
		return
	}

	// Version-aware organize: create a new organized book record linked to the original
	createdBook, createErr := s.organizeService.createOrganizedVersion(org, book, newPath, isDir, op.ID, log2)
	if createErr != nil {
		internalError(c, "failed to create organized version", createErr)
		return
	}

	now := time.Now()
	createdBook.LastOrganizeOperationID = &opID
	createdBook.LastOrganizedAt = &now
	if _, updateErr := database.GlobalStore.UpdateBook(createdBook.ID, createdBook); updateErr != nil {
		log.Printf("[WARN] organize: failed to stamp organized book %s: %v", createdBook.ID, updateErr)
	}

	c.JSON(http.StatusOK, gin.H{
		"message":          fmt.Sprintf("organized: %s → %s", oldPath, newPath),
		"book_id":          createdBook.ID,
		"original_book_id": book.ID,
		"old_path":         oldPath,
		"new_path":         newPath,
		"operation_id":     op.ID,
	})
}

// getUserPreference returns a single user preference by key.
func (s *Server) getUserPreference(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key is required"})
		return
	}
	pref, err := database.GlobalStore.GetUserPreference(key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get preference"})
		return
	}
	if pref == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "preference not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"key": pref.Key, "value": pref.Value})
}

// setUserPreference creates or updates a user preference.
func (s *Server) setUserPreference(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key is required"})
		return
	}
	var body struct {
		Value string `json:"value"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if err := database.GlobalStore.SetUserPreference(key, body.Value); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save preference"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"key": key, "value": body.Value})
}

// deleteUserPreference removes a user preference by setting it to empty.
func (s *Server) deleteUserPreference(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key is required"})
		return
	}
	// Set to empty string to "delete" (store doesn't have a delete method)
	if err := database.GlobalStore.SetUserPreference(key, ""); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete preference"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "preference deleted"})
}
