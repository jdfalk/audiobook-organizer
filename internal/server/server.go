// file: internal/server/server.go
// version: 1.180.0
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
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/activity"
	"github.com/jdfalk/audiobook-organizer/internal/ai"
	"github.com/jdfalk/audiobook-organizer/internal/auth"
	"github.com/jdfalk/audiobook-organizer/internal/cache"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/merge"
	"github.com/jdfalk/audiobook-organizer/internal/search"
	"github.com/jdfalk/audiobook-organizer/internal/itunes"
	"github.com/jdfalk/audiobook-organizer/internal/logger"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
	"github.com/jdfalk/audiobook-organizer/internal/metrics"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
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

	pref, err := database.GetGlobalStore().GetUserPreference(metadataStateKey(bookID))
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
	if database.GetGlobalStore() == nil {
		return state, fmt.Errorf("database not initialized")
	}

	stored, err := database.GetGlobalStore().GetMetadataFieldStates(bookID)
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
	if database.GetGlobalStore() == nil {
		return fmt.Errorf("database not initialized")
	}

	existing, err := database.GetGlobalStore().GetMetadataFieldStates(bookID)
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

		if err := database.GetGlobalStore().UpsertMetadataFieldState(&dbState); err != nil {
			return fmt.Errorf("failed to persist metadata state for %s: %w", field, err)
		}
		delete(existingFields, field)
	}

	for field := range existingFields {
		if err := database.GetGlobalStore().DeleteMetadataFieldState(bookID, field); err != nil {
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
		if author, err := database.GetGlobalStore().GetAuthorByID(*book.AuthorID); err == nil && author != nil {
			authorName = author.Name
		}
	}

	seriesName := ""
	if book.Series != nil {
		seriesName = book.Series.Name
	} else if book.SeriesID != nil {
		if series, err := database.GetGlobalStore().GetSeriesByID(*book.SeriesID); err == nil && series != nil {
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

	if database.GetGlobalStore() != nil {
		if bookAuthors, err := database.GetGlobalStore().GetBookAuthors(book.ID); err == nil && len(bookAuthors) > 0 {
			for _, ba := range bookAuthors {
				if author, err := database.GetGlobalStore().GetAuthorByID(ba.AuthorID); err == nil && author != nil {
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

		if bookNarrators, err := database.GetGlobalStore().GetBookNarrators(book.ID); err == nil && len(bookNarrators) > 0 {
			for _, bn := range bookNarrators {
				if narrator, err := database.GetGlobalStore().GetNarratorByID(bn.NarratorID); err == nil && narrator != nil {
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
func buildComparisonValuesFromActivityLog(as *activity.Service, bookID string, ts time.Time) map[string]any {
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
// activityServiceLogger adapts activity.Service to the operations.ActivityLogger interface.
type activityServiceLogger struct {
	svc *activity.Service
}

func (a *activityServiceLogger) RecordActivity(entry database.ActivityEntry) {
	_ = a.svc.Record(entry)
}

type Server struct {
	store                  database.Store
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
	mergeService           *merge.Service
	diagnosticsService     *DiagnosticsService
	changelogService       *activity.ChangelogService
	activityService        *activity.Service
	embeddingStore         *database.EmbeddingStore
	dedupEngine            *DedupEngine
	activityWriter         *activity.Writer
	itunesActivityFn       func(entry database.ActivityEntry)
	// searchIndex is the Bleve library search index (spec DES-1).
	// Opened at startup, nil if DB path isn't set yet.
	searchIndex *search.BleveIndex
	// indexQueue feeds the single index worker goroutine. Allocated
	// when searchIndex opens, closed in Shutdown. Bounded channel —
	// a full queue drops events and the startup reindex heals gaps.
	// indexQueueMu guards against concurrent close vs. send races.
	indexQueue       chan indexRequest
	indexQueueMu     sync.RWMutex
	indexQueueClosed bool
	http3Server      *http3.Server

	queue            operations.Queue
	hub              *realtime.EventHub
	writeBackBatcher *WriteBackBatcher
	fileIOPool       *FileIOPool

	// Shutdown coordination. bgCtx is canceled when Shutdown() runs, and
	// bgWG tracks every fire-and-forget background goroutine (embedding
	// backfill, async dedup scans, etc.) so Shutdown can wait for them to
	// finish BEFORE the database is closed. Without this the embedding
	// backfill goroutine would still be holding Pebble iterators when
	// database.CloseStore() ran, and Pebble would panic with "element has
	// outstanding references" during FileCache.Unref. Every goroutine that
	// touches the store must: (1) call bgWG.Add(1) before starting,
	// (2) defer bgWG.Done(), (3) honor bgCtx.Done() for cancellation.
	bgCtx    context.Context
	bgCancel context.CancelFunc
	bgWG     sync.WaitGroup
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
// Store returns the database.Store dependency the server was constructed
// with. Handlers should prefer s.Store() over database.GetGlobalStore(); the
// global is being phased out per the 4.4 DI migration.
func (s *Server) Store() database.Store {
	if s.store != nil {
		return s.store
	}
	// Fallback during migration: if s.store wasn't set (older construction
	// paths, tests that build Server literals), fall back to the package
	// global so behavior is unchanged.
	return database.GetGlobalStore()
}

// NewServer constructs a Server with an explicit Store dependency.
// s.Store() is still assigned at startup for code that hasn't
// been migrated to use s.Store() yet (see DI migration plan 4.4).
func NewServer(store database.Store) *Server {
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

	// Services below need a concrete Store at construction time, so
	// resolve a non-nil value here for them. But we only pin the
	// passed-in store on s.store when the caller provided one — if
	// the caller passed nil, s.Store() falls through to the package
	// global dynamically, which matters for tests that swap
	// database.GetGlobalStore() mid-test.
	resolvedStore := store
	if resolvedStore == nil {
		resolvedStore = database.GetGlobalStore()
	}
	bgCtx, bgCancel := context.WithCancel(context.Background())
	server := &Server{
		store:                  store,
		bgCtx:                  bgCtx,
		bgCancel:               bgCancel,
		router:                 router,
		audiobookService:       NewAudiobookService(resolvedStore),
		audiobookUpdateService: NewAudiobookUpdateService(resolvedStore),
		batchService:           NewBatchService(resolvedStore),
		workService:            NewWorkService(resolvedStore),
		authorSeriesService:    NewAuthorSeriesService(resolvedStore),
		filesystemService:      NewFilesystemService(),
		importPathService:      NewImportPathService(resolvedStore),
		importService:          NewImportService(resolvedStore),
		scanService:            NewScanService(resolvedStore),
		organizeService:        NewOrganizeService(resolvedStore),
		metadataFetchService:   NewMetadataFetchService(resolvedStore),
		configUpdateService:    NewConfigUpdateService(resolvedStore),
		systemService:          NewSystemService(resolvedStore),
		metadataStateService:   NewMetadataStateService(resolvedStore),
		dashboardService:       NewDashboardService(resolvedStore),
		dashboardCache:         cache.New[gin.H](30 * time.Second),
		dedupCache:             cache.New[gin.H](5 * time.Minute),
		listCache:              cache.New[gin.H](30 * time.Second),
		olService:              NewOpenLibraryService(),
		updater:                updater.NewUpdater(appVersion),
		mergeService:           merge.NewService(resolvedStore),
		diagnosticsService:     NewDiagnosticsService(resolvedStore, nil, config.AppConfig.ITunesLibraryReadPath),
		changelogService:       activity.NewChangelogService(resolvedStore),
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
			NewISBNEnrichmentService(database.GetGlobalStore(), isbnSources),
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
				server.pipelineManager = NewPipelineManager(aiScanStore, database.GetGlobalStore(), p, server)
				server.batchPoller = NewBatchPoller(database.GetGlobalStore(), p)
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
			server.activityService = activity.NewService(activityStore)
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
				// Wire the embedding store as a content-hash cache so
				// repeated embeds of identical text (e.g. "Foundation
				// by Isaac Asimov" appearing as a candidate across
				// many metadata fetches) return instantly without
				// re-hitting OpenAI. Added after the 2026-04-11 quota
				// incident where a single bulk fetch burned the
				// entire monthly budget by re-embedding every
				// candidate on every fetch.
				embedClient := ai.NewEmbeddingClient(config.AppConfig.OpenAIAPIKey).
					WithCache(embeddingStore)
				// Dedup Layer 3 uses a dedicated chat parser so it can call
				// OpenAIParser.ReviewDedupPairs during maintenance runs.
				llmParser := ai.NewOpenAIParser(config.AppConfig.OpenAIAPIKey, config.AppConfig.EnableAIParsing)
				server.dedupEngine = NewDedupEngine(
					embeddingStore,
					database.GetGlobalStore(),
					embedClient,
					llmParser,
					server.mergeService,
				)
				server.dedupEngine.BookHighThreshold = config.AppConfig.DedupBookHighThreshold
				server.dedupEngine.BookLowThreshold = config.AppConfig.DedupBookLowThreshold
				server.dedupEngine.AuthorHighThreshold = config.AppConfig.DedupAuthorHighThreshold
				server.dedupEngine.AuthorLowThreshold = config.AppConfig.DedupAuthorLowThreshold
				server.dedupEngine.AutoMergeEnabled = config.AppConfig.DedupAutoMergeEnabled

				// Wire chromem-go ANN store if available.
				chromemDir := filepath.Dir(embeddingDBPath)
				chromemStore, chromemErr := database.NewChromemEmbeddingStore(chromemDir, 3072)
				if chromemErr != nil {
					log.Printf("[WARN] chromem-go init failed (falling back to SQLite linear scan): %v", chromemErr)
				} else {
					server.dedupEngine.SetChromemStore(chromemStore)
					log.Println("[INFO] chromem-go ANN store active for dedup Layer 2")
				}

				log.Println("[INFO] Embedding store and dedup engine initialized")
				server.metadataFetchService.SetDedupEngine(server.dedupEngine)

				// Dedup-on-import is now wired via SetScanHooks below
				// (together with the activity recorder).
				log.Println("[INFO] Dedup-on-import hook wired via SetScanHooks")

				// Wire the organize collision hook. When OrganizeBook
				// hits ErrTargetOccupied (two books with identical
				// metadata producing the same target path, or a re-organize
				// of a content-duplicate), this hook creates a pending
				// "exact" dedup candidate between the current book and the
				// book that already owns the target. Without it, the
				// collision would surface only as an opaque error and the
				// user would have no trail to follow.
				//
				// Runs inside a bgWG-tracked goroutine so it doesn't block
				// the organize caller and shutdown drains it cleanly.
				server.organizeService.SetOrganizeHooks(&serverOrganizeHooks{server: server})
				log.Println("[INFO] Organize collision hook wired via OrganizeService")

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

	// Start embedding backfill if dedup engine is ready. Tracked via
	// bgWG so Shutdown() can wait for it to finish before the database
	// closes — without this, a backfill still iterating Pebble when the
	// server stops will leave iterators open and panic inside Pebble's
	// FileCache.Unref during Close().
	if server.dedupEngine != nil {
		server.bgWG.Add(1)
		go func() {
			defer server.bgWG.Done()
			server.runEmbeddingBackfill()
		}()
	}

	// Create hub, queue, batcher, and file I/O pool as Server fields
	server.hub = realtime.NewEventHub()
	// Also set the global for backward compatibility during migration
	realtime.SetGlobalHub(server.hub)

	server.queue = operations.NewOperationQueue(resolvedStore, 2, nil, server.hub)
	// Also set the global for backward compatibility during migration
	operations.GlobalQueue = server.queue

	server.writeBackBatcher = NewWriteBackBatcher(5 * time.Second)
	server.fileIOPool = NewFileIOPool(4)

	// Wire writeBackBatcher into services that need it
	server.metadataFetchService.SetWriteBackBatcher(server.writeBackBatcher)
	server.organizeService.SetWriteBackBatcher(server.writeBackBatcher)
	server.organizeService.SetQueue(server.queue)
	server.mergeService.SetWriteBackBatcher(server.writeBackBatcher)
	server.importService.SetWriteBackBatcher(server.writeBackBatcher)

	// Register file-op recovery handler (uses server closure instead of globalServer)
	RegisterFileOpRecovery("apply_metadata", func(bookID string) {
		if server.metadataFetchService == nil {
			log.Printf("[WARN] no server instance for apply_metadata recovery of book %s", bookID)
			return
		}
		server.metadataFetchService.ApplyMetadataFileIO(bookID)
		if _, err := server.metadataFetchService.WriteBackMetadataForBook(bookID); err != nil {
			log.Printf("[WARN] recovery write-back for %s: %v", bookID, err)
		}
		if server.writeBackBatcher != nil {
			server.writeBackBatcher.Enqueue(bookID)
		}
	})

	// Wire activity log dual-write hooks
	if server.activityService != nil {
		// Task 10: Operation changes → activity log (injected via interface)
		if oq, ok := server.queue.(*operations.OperationQueue); ok {
			oq.SetActivityLogger(&activityServiceLogger{svc: server.activityService})
		}

		// Task 11/14: Metadata fetch service → activity log
		server.metadataFetchService.SetActivityService(server.activityService)

		// Wire activity service into audiobook service for snapshot comparison fallback
		server.audiobookService.SetActivityService(server.activityService)

		// Global log capture via teeWriter — replaces globalActivityRecorder
		aw := activity.NewWriter(server.activityService.Store(), 10000)
		aw.Start()
		server.activityWriter = aw
		log.SetOutput(aw)

		// Task 15: iTunes sync → activity log
		server.itunesActivityFn = func(entry database.ActivityEntry) {
			_ = server.activityService.Record(entry)
		}

		// Task 16: Scanner → activity log (via ScanHooks interface)
		scanner.SetScanHooks(&serverScanHooks{
			activityService: server.activityService,
			dedupFn:         server.fireDedupOnImport,
		})

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

	// Note: the search index is opened in Start(), not here, so
	// tests that construct a Server without calling Start don't
	// leak Bleve file handles.

	server.setupRoutes()

	return server
}

// SearchIndex returns the server's Bleve index, or nil if none is
// open. Handlers use this to decide whether to route queries
// through Bleve or fall back to the legacy SearchBooks path.
func (s *Server) SearchIndex() *search.BleveIndex {
	return s.searchIndex
}

// buildSearchIndexIfEmpty runs a full reindex of the library when
// the search index has zero documents. Honors s.bgCtx so shutdown
// stops the backfill cleanly. Page size matches the existing
// backfill code to keep memory bounded.
func (s *Server) buildSearchIndexIfEmpty() {
	if s.searchIndex == nil {
		return
	}
	count, err := s.searchIndex.DocCount()
	if err != nil {
		log.Printf("[WARN] search index DocCount: %v", err)
		return
	}
	if count > 0 {
		return
	}
	store := s.Store()
	if store == nil {
		return
	}
	log.Printf("[INFO] Search index empty — starting full backfill")
	start := time.Now()
	indexed := 0
	const pageSize = 500
	offset := 0
	for {
		select {
		case <-s.bgCtx.Done():
			log.Printf("[INFO] Search backfill canceled at %d books (bgCtx)", indexed)
			return
		default:
		}
		books, err := store.GetAllBooks(pageSize, offset)
		if err != nil {
			log.Printf("[WARN] search backfill GetAllBooks: %v", err)
			return
		}
		if len(books) == 0 {
			break
		}
		for i := range books {
			select {
			case <-s.bgCtx.Done():
				log.Printf("[INFO] Search backfill canceled at %d books", indexed)
				return
			default:
			}
			doc := search.BookToDoc(store, &books[i])
			if err := s.searchIndex.IndexBook(doc); err != nil {
				log.Printf("[WARN] search backfill index %s: %v", books[i].ID, err)
				continue
			}
			indexed++
		}
		offset += len(books)
		if len(books) < pageSize {
			break
		}
	}
	log.Printf("[INFO] Search backfill complete: %d books in %s", indexed, time.Since(start))
}

// IndexBookByID reads a book (plus its related rows) and upserts
// the flat BookDocument into the search index. Best-effort: logs
// and returns nil if the index isn't open or the book is missing.
// Callers: handlers that create or update a book, plus the startup
// full-build goroutine.
func (s *Server) IndexBookByID(bookID string) error {
	if s.searchIndex == nil || bookID == "" {
		return nil
	}
	book, err := s.Store().GetBookByID(bookID)
	if err != nil || book == nil {
		return err
	}
	return s.searchIndex.IndexBook(search.BookToDoc(s.Store(), book))
}

// DeleteIndexedBook removes a book from the search index. Called
// after a book delete (soft or hard). Safe when the index isn't
// open.
func (s *Server) DeleteIndexedBook(bookID string) error {
	if s.searchIndex == nil || bookID == "" {
		return nil
	}
	return s.searchIndex.DeleteBook(bookID)
}

// serverScanHooks implements scanner.ScanHooks, bridging scanner
// callbacks to the server's activity service and dedup engine.
type serverScanHooks struct {
	activityService *activity.Service
	dedupFn         func(bookID string)
}

func (h *serverScanHooks) OnBookScanned(bookID, title string) {
	if h.activityService != nil {
		_ = h.activityService.Record(database.ActivityEntry{
			Tier:    "change",
			Type:    "scan",
			Level:   "info",
			Source:  "background",
			BookID:  bookID,
			Summary: fmt.Sprintf("Scan found: %s", title),
		})
	}
}

func (h *serverScanHooks) OnImportDedup(bookID string) {
	if h.dedupFn != nil {
		h.dedupFn(bookID)
	}
}

// serverOrganizeHooks implements organizer.OrganizeHooks, bridging
// collision callbacks to the server's dedup engine.
type serverOrganizeHooks struct {
	server *Server
}

func (h *serverOrganizeHooks) OnCollision(currentBookID, occupantPath string) {
	if h.server.embeddingStore == nil || h.server.store == nil {
		return
	}
	h.server.bgWG.Add(1)
	go func() {
		defer h.server.bgWG.Done()
		occupant, err := h.server.store.GetBookByFilePath(occupantPath)
		if err != nil {
			log.Printf("[WARN] organize-collision hook: lookup %s failed: %v", occupantPath, err)
			return
		}
		if occupant == nil || occupant.ID == currentBookID {
			return
		}
		sim := 1.0
		if err := h.server.embeddingStore.UpsertCandidate(database.DedupCandidate{
			EntityType: "book",
			EntityAID:  currentBookID,
			EntityBID:  occupant.ID,
			Layer:      "exact",
			Similarity: &sim,
			Status:     "pending",
		}); err != nil {
			log.Printf("[WARN] organize-collision hook: upsert candidate %s/%s failed: %v",
				currentBookID, occupant.ID, err)
			return
		}
		log.Printf("[INFO] organize-collision: created dedup candidate between %s and %s (occupant of %s)",
			currentBookID, occupant.ID, occupantPath)
	}()
}

// fireDedupOnImport runs the dedup engine's Layer 1 + Layer 2 checks for
// a freshly created book, in a bgWG-tracked goroutine so it doesn't
// block the caller and shutdown drains it before closing Pebble.
//
// This is the single entry point used by every CreateBook path —
// scanner imports (via ScanHooks.OnImportDedup), iTunes sync, manual
// book creation, etc. Having every create path fire the hook means new
// books get exact-match hash/ISBN/title checks against the whole
// library immediately, instead of waiting for a user-triggered Re-scan.
//
// In particular this catches the "iTunes sync creates a parallel row
// for a book we already have under audiobook-organizer/" bug — the
// Layer 1 file-hash check fires inside CheckBook, sees the match, and
// records a pending dedup candidate that surfaces in the UI.
//
// Safe to call even when the dedup engine is disabled — it's a no-op.
func (s *Server) fireDedupOnImport(bookID string) {
	if s.dedupEngine == nil || bookID == "" {
		return
	}
	s.bgWG.Add(1)
	go func() {
		defer s.bgWG.Done()
		if _, err := s.dedupEngine.CheckBook(s.bgCtx, bookID); err != nil {
			log.Printf("[WARN] dedup-on-import CheckBook(%s): %v", bookID, err)
		}
	}()
}

// resumeInterruptedOperations checks for operations left in running/queued state
// from a previous server lifecycle and re-enqueues them.
func (s *Server) resumeInterruptedOperations() {
	store := s.Store()
	if store == nil || s.queue == nil {
		return
	}

	interrupted, err := store.GetInterruptedOperations()
	if err != nil {
		log.Printf("[WARN] Failed to query interrupted operations: %v", err)
		return
	}

	oq, ok := s.queue.(*operations.OperationQueue)
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
				return executeITunesImport(ctx, s.Store(), operations.LoggerFromReporter(progress), opID, ITunesImportRequest{
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
		case "bulk_write_back":
			params, _ := operations.LoadParams[operations.BulkWriteBackParams](store, opID)
			if params == nil {
				log.Printf("[WARN] No params for interrupted bulk_write_back %s, marking failed", opID)
				_ = store.UpdateOperationError(opID, "no saved params, cannot resume")
				continue
			}
			startIdx := 0
			if checkpoint != nil {
				startIdx = checkpoint.PhaseIndex
			}
			bookIDs := params.BookIDs
			doRename := params.Rename
			resumeFn = func(ctx context.Context, progress operations.ProgressReporter) error {
				return s.runBulkWriteBack(ctx, opID, bookIDs, doRename, startIdx, progress)
			}
		case "isbn-enrichment":
			resumeFn = s.runIsbnEnrichment
		case "metadata-refresh":
			resumeFn = s.runMetadataRefreshScan
		case "reconcile_scan":
			scanOpID := opID
			resumeFn = func(ctx context.Context, progress operations.ProgressReporter) error {
				return s.runReconcileScan(ctx, scanOpID, progress)
			}
		case "itunes_path_reconcile":
			reconcileOpID := opID
			resumeFn = func(ctx context.Context, progress operations.ProgressReporter) error {
				return s.runITunesPathReconcile(ctx, reconcileOpID, progress)
			}
		case "transcode", "diagnostics_export", "diagnostics_ai",
			"cleanup_activity_log", "purge_old_logs",
			"purge-deleted", "tombstone-cleanup",
			"author-dedup-scan", "author-split-scan", "series-prune",
			"db-optimize", "cleanup-old-backups", "batch_poller",
			"itunes_sync":
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

	// Seed / refresh the multi-user roles (spec 3.7). Idempotent: if
	// the permission set in auth.SeedRoles has grown since last boot,
	// existing roles pick up the new entries automatically.
	if created, updated, err := auth.SeedRoles(s.Store()); err != nil {
		log.Printf("[WARN] seed roles: %v", err)
	} else if created > 0 || updated > 0 {
		log.Printf("[INFO] seed roles: %d created, %d updated", created, updated)
	}
	if err := auth.SeedSystemUser(s.Store()); err != nil {
		log.Printf("[WARN] seed system user: %v", err)
	}

	// Resume any operations that were interrupted by a previous shutdown/crash
	s.resumeInterruptedOperations()

	// Recover interrupted file I/O operations (cover embed, tag write, rename)
	RecoverInterruptedFileOps(s.fileIOPool)

	// Resume interrupted metadata candidate fetch operations
	s.resumeInterruptedMetadataFetch()

	// Backfill external ID mappings from existing iTunes PIDs (one-time,
	// idempotent). Tracked via bgWG for the same reason as the embedding
	// backfill: we can't let it hold Pebble iterators while CloseStore runs.
	s.bgWG.Add(1)
	go func() {
		defer s.bgWG.Done()
		s.backfillExternalIDs()
	}()

	// Open the library search index (Bleve, spec DES-1). Opened here
	// rather than in NewServer so tests that skip Start don't leak
	// Bleve handles. Failures are non-fatal — server runs without
	// search until the index comes back.
	if dbPath := config.AppConfig.DatabasePath; dbPath != "" && s.searchIndex == nil {
		indexPath := filepath.Join(filepath.Dir(dbPath), "library.bleve")
		idx, err := search.Open(indexPath)
		if err != nil {
			log.Printf("[WARN] Failed to open search index: %v", err)
		} else {
			s.searchIndex = idx
			log.Printf("[INFO] Search index opened at %s", indexPath)
		}
	}

	// Install the indexing store decorator + worker once the index is
	// open. Every downstream book mutation flows through s.Store()
	// (or the package-level global) so wrapping both keeps the index
	// in sync regardless of whether a caller has migrated to DI yet.
	if s.searchIndex != nil {
		s.indexQueue = make(chan indexRequest, 1024)
		inner := s.Store()
		wrapped := &indexedStore{Store: inner, server: s}
		s.store = wrapped
		database.SetGlobalStore(wrapped)
		s.bgWG.Add(1)
		go func() {
			defer s.bgWG.Done()
			s.runIndexWorker()
		}()
		// Route the /audiobooks?search= path through Bleve.
		if s.audiobookService != nil {
			s.audiobookService.SetSearchIndex(s.searchIndex)
		}
	}

	// Build the search index on first startup (or if it got wiped).
	// Tracked via bgWG so shutdown can wait for in-flight indexing
	// instead of letting it run under a closing DB.
	if s.searchIndex != nil {
		s.bgWG.Add(1)
		go func() {
			defer s.bgWG.Done()
			s.buildSearchIndexIfEmpty()
		}()
	}

	// Start periodic cleanup of stale transcode temp files
	if s.Store() != nil {
		if paths, err := s.Store().GetAllImportPaths(); err == nil {
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
				if s.hub != nil {
					// Gather lightweight metrics
					var alloc runtime.MemStats
					runtime.ReadMemStats(&alloc)
					bookCount := 0
					folderCount := 0
					if s.Store() != nil {
						if bc, err := s.Store().CountBooks(); err == nil {
							bookCount = bc
						}
						if folders, err := s.Store().GetAllImportPaths(); err == nil {
							folderCount = len(folders)
						}
					}

					// Update Prometheus metrics
					metrics.SetBooks(bookCount)
					metrics.SetFolders(folderCount)
					metrics.SetMemoryAlloc(alloc.Alloc)
					metrics.SetGoroutines(runtime.NumGoroutine())

					s.hub.SendSystemStatus(map[string]any{
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

	// Start auto-scan file watchers if enabled. ONE watcher per enabled
	// import path — previously only the first enabled path was watched,
	// so users with multiple import locations had silent blind spots on
	// every path after the first.
	var fileWatchers []*watcher.Watcher
	if config.AppConfig.AutoScanEnabled && s.Store() != nil {
		importPaths, err := s.Store().GetAllImportPaths()
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
				watchLog := logger.NewWithActivityLog("auto-scan", s.Store())
				// The same callback is reused across watchers because
				// each watcher invokes it with its own root path, so
				// the scan target is correct per event.
				cb := func(path string) {
					watchLog.Info("Auto-scan triggered for: %s", path)
					if s.hub != nil {
						s.hub.Broadcast(&realtime.Event{
							Type: "scan.auto_triggered",
							Data: map[string]any{"path": path},
						})
					}
					if s.scanService != nil && s.queue != nil {
						go func() {
							scanPath := path
							id := ulid.Make().String()
							op, opErr := s.Store().CreateOperation(id, "scan", &scanPath)
							if opErr != nil {
								watchLog.Error("Auto-scan: failed to create operation: %v", opErr)
								return
							}
							scanReq := &ScanRequest{FolderPath: &scanPath}
							opFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
								return s.scanService.PerformScan(ctx, scanReq, operations.LoggerFromReporter(progress))
							}
							if enqueueErr := s.queue.Enqueue(op.ID, "scan", operations.PriorityLow, opFunc); enqueueErr != nil {
								watchLog.Error("Auto-scan: failed to enqueue: %v", enqueueErr)
							}
						}()
					}
				}
				for _, wp := range watchPaths {
					fw := watcher.New(cb, debounce)
					if startErr := fw.Start(wp); startErr != nil {
						watchLog.Warn("Failed to start file watcher for %s: %v", wp, startErr)
						continue
					}
					fileWatchers = append(fileWatchers, fw)
					watchLog.Info("Auto-scan file watcher started for %s", wp)
				}
			}
		}
	}

	// Periodic cleanup of expired/revoked auth sessions.
	if s.Store() != nil {
		sessionLog := logger.NewWithActivityLog("session-cleanup", s.Store())
		sessionCleanupTicker := time.NewTicker(10 * time.Minute)
		backgroundWG.Add(1)
		go func() {
			defer backgroundWG.Done()
			defer sessionCleanupTicker.Stop()
			for {
				select {
				case <-sessionCleanupTicker.C:
					if deleted, err := s.Store().DeleteExpiredSessions(time.Now()); err != nil {
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
	if s.Store() != nil && config.AppConfig.OperationTimeoutMinutes > 0 {
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
	if s.hub != nil {
		s.hub.Broadcast(&realtime.Event{
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

	// Cancel fire-and-forget background work (embedding backfill, async
	// dedup scans) and wait for it to return. This MUST happen before
	// embeddingStore.Close() and before Start() returns (which triggers
	// the deferred closeStore() in cmd/root.go). Without it, the backfill
	// goroutine keeps iterating Pebble while CloseStore runs, and Pebble's
	// FileCache.Unref panics with "element has outstanding references"
	// during shutdown — which has been killing every restart mid-cycle.
	if s.bgCancel != nil {
		log.Println("[INFO] Canceling background goroutines...")
		s.bgCancel()
	}
	// Close the index queue so the index worker goroutine can
	// finish its range loop and decrement bgWG. Leaving it open
	// would deadlock the wait below because the worker doesn't
	// listen on bgCtx — its termination signal is the queue close.
	s.closeIndexQueue()
	bgDone := make(chan struct{})
	go func() {
		s.bgWG.Wait()
		close(bgDone)
	}()
	select {
	case <-bgDone:
		log.Println("[INFO] Background goroutines stopped")
	case <-time.After(30 * time.Second):
		log.Println("[WARN] Background goroutines did not stop within 30s — proceeding with shutdown anyway")
	}

	// Stop the file I/O pool — waits for in-flight jobs to finish
	if p := s.fileIOPool; p != nil {
		log.Println("[INFO] Waiting for file I/O operations to complete...")
		p.Stop()
	}

	// Flush the ITL write-back batcher
	if s.writeBackBatcher != nil {
		log.Println("[INFO] Flushing iTunes write-back batcher...")
		s.writeBackBatcher.Stop()
	}

	// Close the search index before the DB goes away — the index is
	// independent storage but closing it here keeps shutdown order
	// predictable and avoids Bleve holding file handles after the
	// process starts tearing down.
	if s.searchIndex != nil {
		if err := s.searchIndex.Close(); err != nil {
			log.Printf("[WARN] Failed to close search index: %v", err)
		} else {
			log.Println("[INFO] Search index closed")
		}
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

	// Stop every file watcher (one per import path).
	for _, fw := range fileWatchers {
		fw.Stop()
	}
	if len(fileWatchers) > 0 {
		log.Printf("[INFO] File watchers stopped (%d)", len(fileWatchers))
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

// perm returns a Gin middleware that checks the calling user has the
// given permission. It's a thin wrapper around RequirePermission from
// the middleware package, curried with the server's Store. Used inline
// in route registration: `protected.GET("/path", s.perm(P), s.handler)`.
func (s *Server) perm(p auth.Permission) gin.HandlerFunc {
	if !config.AppConfig.EnableAuth {
		return func(c *gin.Context) { c.Next() }
	}
	return servermiddleware.RequirePermission(s.Store(), p)
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
		authMiddleware = servermiddleware.RequireAuth(s.Store())
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
			authGroup.POST("/accept-invite", s.handleAcceptInvite)
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
			protected.GET("/audiobooks", s.perm(auth.PermLibraryView), s.listAudiobooks)
			// /audiobooks/search removed — use GET /audiobooks?search= instead
			protected.GET("/audiobooks/count", s.perm(auth.PermLibraryView), s.countAudiobooks)
			protected.GET("/audiobooks/duplicates", s.perm(auth.PermLibraryView), s.listDuplicateAudiobooks)
			protected.GET("/audiobooks/duplicates/scan-results", s.perm(auth.PermLibraryView), s.listBookDuplicateScanResults)
			protected.POST("/audiobooks/duplicates/scan", s.perm(auth.PermLibraryEditMetadata), s.scanBookDuplicates)
			protected.POST("/audiobooks/duplicates/merge", s.perm(auth.PermLibraryEditMetadata), s.mergeBookDuplicatesAsVersions)
			protected.POST("/audiobooks/duplicates/dismiss", s.perm(auth.PermLibraryEditMetadata), s.dismissBookDuplicateGroup)
			protected.GET("/audiobooks/soft-deleted", s.perm(auth.PermLibraryView), s.listSoftDeletedAudiobooks)
			protected.DELETE("/audiobooks/purge-soft-deleted", s.perm(auth.PermLibraryDelete), s.purgeSoftDeletedAudiobooks)
			protected.POST("/audiobooks/:id/restore", s.perm(auth.PermLibraryOrganize), s.restoreAudiobook)
			protected.GET("/audiobooks/:id", s.perm(auth.PermLibraryView), s.getAudiobook)
			protected.GET("/audiobooks/:id/tags", s.perm(auth.PermLibraryView), s.getAudiobookTags)
			protected.PUT("/audiobooks/:id", s.perm(auth.PermLibraryEditMetadata), s.updateAudiobook)
			protected.DELETE("/audiobooks/:id", s.perm(auth.PermLibraryDelete), s.deleteAudiobook)
			protected.GET("/audiobooks/:id/cover", s.perm(auth.PermLibraryView), s.serveAudiobookCover)
			protected.GET("/audiobooks/:id/segments", s.perm(auth.PermLibraryView), s.listAudiobookSegments)
			protected.GET("/audiobooks/:id/segments/:segmentId/tags", s.perm(auth.PermLibraryView), s.getSegmentTags)
			protected.GET("/audiobooks/:id/files", s.perm(auth.PermLibraryView), s.listBookFiles)
			protected.GET("/audiobooks/:id/changelog", s.perm(auth.PermLibraryView), s.getBookChangelog)
			protected.GET("/audiobooks/:id/path-history", s.perm(auth.PermLibraryView), s.getBookPathHistory)
			protected.GET("/audiobooks/:id/external-ids", s.perm(auth.PermLibraryView), s.getAudiobookExternalIDs)
			protected.POST("/audiobooks/:id/extract-track-info", s.perm(auth.PermLibraryEditMetadata), s.extractTrackInfo)
			protected.POST("/audiobooks/:id/relocate", s.perm(auth.PermLibraryOrganize), s.relocateBookFiles)
			protected.POST("/audiobooks/batch", s.perm(auth.PermLibraryEditMetadata), s.batchUpdateAudiobooks)
			protected.POST("/audiobooks/batch-write-back", s.perm(auth.PermLibraryEditMetadata), s.batchWriteBackAudiobooks)
			protected.POST("/audiobooks/bulk-write-back", s.perm(auth.PermLibraryEditMetadata), s.handleBulkWriteBack)
			protected.POST("/audiobooks/batch-operations", s.perm(auth.PermLibraryEditMetadata), s.batchOperations)

			// User tag routes
			protected.GET("/tags", s.perm(auth.PermLibraryView), s.listAllUserTags)
			protected.GET("/audiobooks/:id/user-tags", s.perm(auth.PermLibraryView), s.getBookUserTags)
			// Detailed tag route: returns tag+source pairs so the
			// UI can render system-applied tags (dedup:*,
			// metadata:source:*, etc.) differently from user tags.
			protected.GET("/audiobooks/:id/tags-detailed", s.perm(auth.PermLibraryView), s.getBookTagsDetailed)
			protected.POST("/audiobooks/batch-tags", s.perm(auth.PermLibraryEditMetadata), s.batchUpdateTags)

			// Book alternative titles
			protected.GET("/audiobooks/:id/alternative-titles", s.perm(auth.PermLibraryView), s.getBookAlternativeTitles)
			protected.POST("/audiobooks/:id/alternative-titles", s.perm(auth.PermLibraryEditMetadata), s.addBookAlternativeTitle)
			protected.DELETE("/audiobooks/:id/alternative-titles", s.perm(auth.PermLibraryDelete), s.removeBookAlternativeTitle)

			// User preferences
			protected.GET("/preferences/:key", s.perm(auth.PermLibraryView), s.getUserPreference)
			protected.PUT("/preferences/:key", s.perm(auth.PermLibraryEditMetadata), s.setUserPreference)
			protected.DELETE("/preferences/:key", s.perm(auth.PermLibraryDelete), s.deleteUserPreference)

			// Metadata change history
			protected.GET("/audiobooks/:id/metadata-history", s.perm(auth.PermLibraryView), s.getBookMetadataHistory)
			protected.GET("/audiobooks/:id/metadata-history/:field", s.perm(auth.PermLibraryView), s.getFieldMetadataHistory)
			protected.POST("/audiobooks/:id/metadata-history/:field/undo", s.perm(auth.PermLibraryEditMetadata), s.undoMetadataChange)
			protected.POST("/audiobooks/:id/undo-last-apply", s.perm(auth.PermLibraryEditMetadata), s.undoLastApply)
			protected.GET("/audiobooks/:id/field-states", s.perm(auth.PermLibraryView), s.getAudiobookFieldStates)
			protected.GET("/audiobooks/:id/changes", s.perm(auth.PermLibraryView), s.getBookChanges)

			// Author, narrator, and series routes
			protected.GET("/authors", s.perm(auth.PermLibraryView), s.listAuthors)
			protected.GET("/authors/count", s.perm(auth.PermLibraryView), s.countAuthors)
			protected.GET("/authors/duplicates", s.perm(auth.PermLibraryView), s.listDuplicateAuthors)
			protected.POST("/authors/duplicates/refresh", s.perm(auth.PermLibraryEditMetadata), s.refreshDuplicateAuthors)
			protected.POST("/authors/duplicates/ai-review", s.perm(auth.PermLibraryEditMetadata), s.aiReviewDuplicateAuthors)
			protected.POST("/authors/duplicates/ai-review/apply", s.perm(auth.PermLibraryEditMetadata), s.applyAIAuthorReview)
			protected.POST("/authors/merge", s.perm(auth.PermLibraryEditMetadata), s.mergeAuthors)
			protected.POST("/authors/:id/reclassify-as-narrator", s.perm(auth.PermLibraryEditMetadata), s.reclassifyAuthorAsNarrator)
			protected.PUT("/authors/:id/name", s.perm(auth.PermLibraryEditMetadata), s.renameAuthor)
			protected.POST("/authors/:id/split", s.perm(auth.PermLibraryEditMetadata), s.splitCompositeAuthor)
			protected.POST("/authors/:id/resolve-production", s.perm(auth.PermLibraryEditMetadata), s.resolveProductionAuthor)
			protected.GET("/authors/:id/aliases", s.perm(auth.PermLibraryView), s.getAuthorAliases)
			protected.POST("/authors/:id/aliases", s.perm(auth.PermLibraryEditMetadata), s.createAuthorAlias)
			protected.DELETE("/authors/:id/aliases/:aliasId", s.perm(auth.PermLibraryDelete), s.deleteAuthorAlias)
			protected.POST("/audiobooks/merge", s.perm(auth.PermLibraryEditMetadata), s.mergeBooks)
			protected.GET("/narrators", s.perm(auth.PermLibraryView), s.listNarrators)
			protected.GET("/narrators/count", s.perm(auth.PermLibraryView), s.countNarrators)
			protected.GET("/audiobooks/:id/narrators", s.perm(auth.PermLibraryView), s.listAudiobookNarrators)
			protected.PUT("/audiobooks/:id/narrators", s.perm(auth.PermLibraryEditMetadata), s.setAudiobookNarrators)
			protected.GET("/series", s.perm(auth.PermLibraryView), s.listSeries)
			protected.GET("/series/count", s.perm(auth.PermLibraryView), s.countSeries)
			protected.GET("/series/duplicates", s.perm(auth.PermLibraryView), s.listSeriesDuplicates)
			protected.POST("/series/duplicates/refresh", s.perm(auth.PermLibraryEditMetadata), s.refreshSeriesDuplicates)
			protected.POST("/series/deduplicate", s.perm(auth.PermLibraryEditMetadata), s.deduplicateSeriesHandler)
			protected.POST("/series/merge", s.perm(auth.PermLibraryEditMetadata), s.mergeSeriesGroup)
			protected.GET("/series/prune/preview", s.perm(auth.PermLibraryView), s.seriesPrunePreview)
			protected.POST("/series/prune", s.perm(auth.PermLibraryEditMetadata), s.seriesPrune)
			protected.PATCH("/series/:id", s.perm(auth.PermLibraryEditMetadata), s.updateSeriesName)
			protected.GET("/series/:id/books", s.perm(auth.PermLibraryView), s.getSeriesBooks)
			protected.PUT("/series/:id/name", s.perm(auth.PermLibraryEditMetadata), s.renameSeriesHandler)
			protected.POST("/series/:id/split", s.perm(auth.PermLibraryEditMetadata), s.splitSeriesHandler)
			protected.DELETE("/series/:id", s.perm(auth.PermLibraryDelete), s.deleteEmptySeries)
			protected.GET("/authors/:id/books", s.perm(auth.PermLibraryView), s.getAuthorBooks)
			protected.DELETE("/authors/:id", s.perm(auth.PermLibraryDelete), s.deleteAuthorHandler)
			protected.POST("/authors/bulk-delete", s.perm(auth.PermLibraryDelete), s.bulkDeleteAuthors)
			protected.POST("/series/bulk-delete", s.perm(auth.PermLibraryDelete), s.bulkDeleteSeries)
			protected.POST("/dedup/validate", s.perm(auth.PermLibraryEditMetadata), s.validateDedupEntry)

			// Embedding-based dedup
			protected.GET("/dedup/candidates", s.perm(auth.PermLibraryView), s.listDedupCandidates)
			protected.GET("/dedup/candidates/export", s.perm(auth.PermLibraryView), s.exportDedupCandidates)
			protected.GET("/dedup/stats", s.perm(auth.PermLibraryView), s.getDedupStats)
			protected.POST("/dedup/candidates/:id/merge", s.perm(auth.PermLibraryEditMetadata), s.mergeDedupCandidate)
			protected.POST("/dedup/candidates/:id/dismiss", s.perm(auth.PermLibraryEditMetadata), s.dismissDedupCandidate)
			protected.POST("/dedup/candidates/bulk-merge", s.perm(auth.PermLibraryEditMetadata), s.bulkMergeDedupCandidates)
			protected.POST("/dedup/candidates/merge-cluster", s.perm(auth.PermLibraryEditMetadata), s.mergeDedupCluster)
			protected.POST("/dedup/candidates/dismiss-cluster", s.perm(auth.PermLibraryEditMetadata), s.dismissDedupCluster)
			protected.POST("/dedup/candidates/remove-from-cluster", s.perm(auth.PermLibraryEditMetadata), s.removeFromDedupCluster)
			protected.GET("/dedup/candidates/series-summary", s.perm(auth.PermLibraryView), s.listDedupCandidateSeries)
			protected.POST("/dedup/candidates/merge-series", s.perm(auth.PermLibraryEditMetadata), s.mergeDedupCandidateSeries)
			protected.POST("/dedup/scan", s.perm(auth.PermScanTrigger), s.triggerDedupScan)
			protected.POST("/dedup/scan-llm", s.perm(auth.PermScanTrigger), s.triggerDedupLLM)
			protected.POST("/dedup/refresh", s.perm(auth.PermScanTrigger), s.triggerDedupRefresh)

			// File system routes
			protected.GET("/filesystem/home", s.perm(auth.PermSettingsManage), s.getHomeDirectory)
			protected.GET("/filesystem/browse", s.perm(auth.PermSettingsManage), s.browseFilesystem)
			protected.POST("/filesystem/exclude", s.perm(auth.PermSettingsManage), s.createExclusion)
			protected.DELETE("/filesystem/exclude", s.perm(auth.PermSettingsManage), s.removeExclusion)

			// Import path routes
			protected.GET("/import-paths", s.perm(auth.PermSettingsManage), s.listImportPaths)
			protected.POST("/import-paths", s.perm(auth.PermSettingsManage), s.addImportPath)
			protected.DELETE("/import-paths/:id", s.perm(auth.PermSettingsManage), s.removeImportPath)

			// Operation routes
			protected.GET("/operations", s.perm(auth.PermLibraryView), s.listOperations)
			protected.GET("/operations/active", s.perm(auth.PermLibraryView), s.listActiveOperations)
			protected.GET("/operations/stale", s.perm(auth.PermLibraryView), s.listStaleOperations)
			protected.POST("/operations/scan", s.perm(auth.PermScanTrigger), s.startScan)
			protected.POST("/operations/organize", s.perm(auth.PermScanTrigger), s.startOrganize)
			protected.POST("/operations/transcode", s.perm(auth.PermScanTrigger), s.startTranscode)
			protected.GET("/operations/recent", s.perm(auth.PermLibraryView), s.handleGetRecentOperations)
			protected.GET("/file-ops/pending", s.perm(auth.PermLibraryView), s.handleListPendingFileOps)
			protected.GET("/operations/:id/results", s.perm(auth.PermLibraryView), s.handleGetOperationResults)
			protected.GET("/operations/:id/status", s.perm(auth.PermLibraryView), s.getOperationStatus)
			protected.GET("/operations/:id/logs", s.perm(auth.PermLibraryView), s.getOperationLogs)
			protected.GET("/operations/:id/result", s.perm(auth.PermLibraryView), s.getOperationResult)
			protected.DELETE("/operations/:id", s.perm(auth.PermSettingsManage), s.cancelOperation)
			protected.POST("/operations/clear-stale", s.perm(auth.PermSettingsManage), s.clearStaleOperations)
			protected.DELETE("/operations/history", s.perm(auth.PermSettingsManage), s.deleteOperationHistory)
			protected.POST("/operations/optimize-database", s.perm(auth.PermSettingsManage), s.optimizeDatabase)
			protected.POST("/operations/sweep-tombstones", s.perm(auth.PermSettingsManage), s.sweepTombstones)
			protected.GET("/operations/audit-files", s.perm(auth.PermSettingsManage), s.auditFileConsistency)
			protected.GET("/operations/reconcile/preview", s.perm(auth.PermLibraryView), s.reconcilePreview)
			protected.POST("/operations/reconcile", s.perm(auth.PermScanTrigger), s.startReconcile)
			protected.POST("/operations/reconcile/scan", s.perm(auth.PermScanTrigger), s.startReconcileScan)
			protected.GET("/operations/reconcile/scan/latest", s.perm(auth.PermLibraryView), s.latestReconcileScan)
			protected.POST("/operations/itunes-path-reconcile", s.perm(auth.PermScanTrigger), s.startITunesPathReconcile)
			protected.POST("/operations/cleanup-version-groups", s.perm(auth.PermSettingsManage), s.cleanupDuplicateVersionGroupsHandler)
			protected.POST("/operations/mark-broken-segments", s.perm(auth.PermSettingsManage), s.markBrokenSegmentBooksHandler)
			protected.POST("/operations/merge-novg-duplicates", s.perm(auth.PermSettingsManage), s.mergeNoVGDuplicatesHandler)
			protected.POST("/operations/assign-orphan-vgs", s.perm(auth.PermSettingsManage), s.assignOrphanVGsHandler)
			protected.GET("/operations/:id/changes", s.perm(auth.PermLibraryView), s.getOperationChanges)
			protected.GET("/operations/:id/undo/preflight", s.perm(auth.PermLibraryView), s.undoPreflightHandler)
			protected.POST("/operations/:id/revert", s.perm(auth.PermLibraryOrganize), s.revertOperation)

			// Import routes
			protected.POST("/import/file", s.perm(auth.PermScanTrigger), s.importFile)
			protected.POST("/import/collision-preview", s.perm(auth.PermLibraryView), s.handleImportCollisionPreview)

			// iTunes import routes
			itunesGroup := protected.Group("/itunes")
			{
				itunesGroup.POST("/validate", s.perm(auth.PermLibraryEditMetadata), s.handleITunesValidate)
				itunesGroup.POST("/test-mapping", s.perm(auth.PermLibraryEditMetadata), s.handleITunesTestMapping)
				itunesGroup.POST("/import", s.perm(auth.PermLibraryEditMetadata), s.handleITunesImport)
				itunesGroup.POST("/write-back", s.perm(auth.PermLibraryEditMetadata), s.handleITunesWriteBack)
				itunesGroup.POST("/write-back-all", s.perm(auth.PermLibraryEditMetadata), s.handleITunesWriteBackAll)
				itunesGroup.POST("/write-back/preview", s.perm(auth.PermLibraryEditMetadata), s.handleITunesWriteBackPreview)
				itunesGroup.GET("/books", s.perm(auth.PermLibraryView), s.handleListITunesBooks)
				itunesGroup.GET("/import-status/:id", s.perm(auth.PermLibraryView), s.handleITunesImportStatus)
				itunesGroup.POST("/import-status/bulk", s.perm(auth.PermLibraryEditMetadata), s.handleITunesImportStatusBulk)
				itunesGroup.GET("/library-status", s.perm(auth.PermLibraryView), s.handleITunesLibraryStatus)
				itunesGroup.POST("/sync", s.perm(auth.PermLibraryEditMetadata), s.handleITunesSync)
				// Diff-and-batch rebuild: computes the full diff
				// between the DB and the current ITL file, then
				// applies all adds/removes/updates in one atomic
				// safeWriteITL call. Supports dry_run=true to
				// preview without applying. Backlog 7.9.
				itunesGroup.POST("/rebuild", s.perm(auth.PermLibraryEditMetadata), s.rebuildITLHandler)

				// ITL file transfer (6.4)
				itunesGroup.GET("/library/download", s.perm(auth.PermIntegrationsManage), s.handleITLDownload)
				itunesGroup.POST("/library/upload", s.perm(auth.PermIntegrationsManage), s.handleITLUpload)
				itunesGroup.GET("/library/backups", s.perm(auth.PermIntegrationsManage), s.handleITLBackupList)
				itunesGroup.POST("/library/restore", s.perm(auth.PermIntegrationsManage), s.handleITLRestore)
			}

			// Cover art
			protected.GET("/covers/proxy", s.perm(auth.PermLibraryView), s.handleCoverProxy)
			protected.GET("/covers/local/:filename", s.perm(auth.PermLibraryView), s.handleLocalCover)
			protected.GET("/audiobooks/:id/cover-history", s.perm(auth.PermLibraryView), s.handleListCoverHistory)
			protected.POST("/audiobooks/:id/cover-history/restore", s.perm(auth.PermLibraryEditMetadata), s.handleRestoreCover)

			// Unified task/scheduler routes
			protected.GET("/tasks", s.perm(auth.PermSettingsManage), s.listTasks)
			protected.POST("/tasks/:name/run", s.perm(auth.PermSettingsManage), s.runTask)
			protected.PUT("/tasks/:name", s.perm(auth.PermSettingsManage), s.updateTaskConfig)
			protected.POST("/maintenance-window/run", s.perm(auth.PermSettingsManage), s.runMaintenanceWindowNow)
			protected.POST("/maintenance/fix-read-by-narrator", s.perm(auth.PermSettingsManage), s.handleFixReadByNarrator)
			protected.POST("/maintenance/cleanup-series", s.perm(auth.PermSettingsManage), s.handleCleanupSeries)
			protected.POST("/maintenance/backfill-book-files", s.perm(auth.PermSettingsManage), s.handleBackfillBookFiles)
			protected.POST("/maintenance/cleanup-empty-folders", s.perm(auth.PermSettingsManage), s.handleCleanupEmptyFolders)
			protected.POST("/maintenance/cleanup-backups", s.perm(auth.PermSettingsManage), s.handleCleanupBackups)
			protected.POST("/maintenance/cleanup-organize-mess", s.perm(auth.PermSettingsManage), s.handleCleanupOrganizeMess)
			protected.POST("/maintenance/fix-author-narrator-swap", s.perm(auth.PermSettingsManage), s.handleFixAuthorNarratorSwap)
			protected.POST("/maintenance/fix-version-groups", s.perm(auth.PermSettingsManage), s.handleFixVersionGroups)
			protected.POST("/maintenance/fix-library-states", s.perm(auth.PermSettingsManage), s.handleFixLibraryStates)
			protected.POST("/maintenance/enrich-book-files", s.perm(auth.PermSettingsManage), s.handleEnrichBookFiles)
			protected.POST("/maintenance/dedup-books", s.perm(auth.PermSettingsManage), s.handleDedupBooks)
			protected.POST("/maintenance/fix-book-file-paths", s.perm(auth.PermSettingsManage), s.handleFixBookFilePaths)
			protected.POST("/maintenance/refetch-missing-authors", s.perm(auth.PermSettingsManage), s.handleRefetchMissingAuthors)
			protected.POST("/maintenance/recompute-itunes-paths", s.perm(auth.PermSettingsManage), s.handleRecomputeITunesPaths)
			protected.POST("/maintenance/generate-itl-tests", s.perm(auth.PermSettingsManage), s.handleGenerateITLTests)

			// Admin-only destructive endpoints
			adminOnly := protected.Group("")
			adminOnly.Use(servermiddleware.RequireAdmin())
			{
				adminOnly.POST("/maintenance/wipe", s.handleWipe)
			}

			// Unified activity log
			protected.GET("/activity", s.perm(auth.PermLibraryView), s.listActivity)
			protected.GET("/activity/sources", s.perm(auth.PermLibraryView), s.listActivitySources)
			protected.POST("/activity/compact", s.perm(auth.PermSettingsManage), s.compactActivity)

			// System routes
			protected.GET("/system/status", s.perm(auth.PermSettingsManage), s.getSystemStatus)
			protected.GET("/system/announcements", s.perm(auth.PermSettingsManage), s.getSystemAnnouncements)
			protected.GET("/system/storage", s.perm(auth.PermSettingsManage), s.getSystemStorage)
			protected.GET("/system/logs", s.perm(auth.PermSettingsManage), s.getSystemLogs)
			protected.GET("/system/activity-log", s.perm(auth.PermSettingsManage), s.getSystemActivityLog)
			protected.POST("/system/reset", s.perm(auth.PermSettingsManage), s.resetSystem)
			protected.POST("/system/factory-reset", s.perm(auth.PermSettingsManage), s.factoryReset)
			protected.GET("/config", s.perm(auth.PermSettingsManage), s.getConfig)
			protected.PUT("/config", s.perm(auth.PermSettingsManage), s.updateConfig)
			protected.GET("/dashboard", s.perm(auth.PermLibraryView), s.getDashboard)

			// Backup routes
			protected.POST("/backup/create", s.perm(auth.PermSettingsManage), s.createBackup)
			protected.GET("/backup/list", s.perm(auth.PermSettingsManage), s.listBackups)
			protected.POST("/backup/restore", s.perm(auth.PermSettingsManage), s.restoreBackup)
			protected.DELETE("/backup/:filename", s.perm(auth.PermSettingsManage), s.deleteBackup)

			// Enhanced metadata routes
			protected.POST("/metadata/batch-update", s.perm(auth.PermLibraryEditMetadata), s.batchUpdateMetadata)
			protected.POST("/metadata/validate", s.perm(auth.PermLibraryEditMetadata), s.validateMetadata)
			protected.GET("/metadata/export", s.perm(auth.PermLibraryView), s.exportMetadata)
			protected.POST("/metadata/import", s.perm(auth.PermLibraryEditMetadata), s.importMetadata)
			protected.GET("/metadata/search", s.perm(auth.PermLibraryView), s.searchMetadata)
			protected.GET("/metadata/fields", s.perm(auth.PermLibraryView), s.getMetadataFields)
			protected.POST("/metadata/bulk-fetch", s.perm(auth.PermLibraryEditMetadata), s.bulkFetchMetadata)
			protected.POST("/metadata/batch-fetch-candidates", s.perm(auth.PermLibraryEditMetadata), s.handleBatchFetchCandidates)
			protected.GET("/metadata/recent-fetches", s.perm(auth.PermLibraryView), s.handleGetLatestMetadataFetch)
			protected.POST("/metadata/batch-apply-candidates", s.perm(auth.PermLibraryEditMetadata), s.handleBatchApplyCandidates)
			protected.POST("/metadata/batch-reject-candidates", s.perm(auth.PermLibraryEditMetadata), s.handleRejectCandidates)
			protected.POST("/metadata/batch-unreject-candidates", s.perm(auth.PermLibraryEditMetadata), s.handleUnrejectCandidates)
			protected.POST("/audiobooks/:id/fetch-metadata", s.perm(auth.PermLibraryEditMetadata), s.fetchAudiobookMetadata)
			protected.POST("/audiobooks/:id/search-metadata", s.perm(auth.PermLibraryEditMetadata), s.searchAudiobookMetadata)
			protected.POST("/audiobooks/:id/apply-metadata", s.perm(auth.PermLibraryEditMetadata), s.applyAudiobookMetadata)
			protected.POST("/audiobooks/:id/mark-no-match", s.perm(auth.PermLibraryEditMetadata), s.markAudiobookNoMatch)
			protected.POST("/audiobooks/:id/revert-metadata", s.perm(auth.PermLibraryEditMetadata), s.revertAudiobookMetadata)
			protected.GET("/audiobooks/:id/similar", s.perm(auth.PermLibraryView), s.handleSimilarBooks)
			protected.GET("/audiobooks/:id/cow-versions", s.perm(auth.PermLibraryView), s.listBookCOWVersions)
			protected.POST("/audiobooks/:id/cow-versions/prune", s.perm(auth.PermLibraryEditMetadata), s.pruneBookCOWVersions)
			protected.POST("/audiobooks/:id/write-back", s.perm(auth.PermLibraryEditMetadata), s.writeBackAudiobookMetadata)

			// Rename preview and apply
			protected.POST("/audiobooks/:id/rename/preview", s.perm(auth.PermLibraryOrganize), s.previewRename)
			protected.POST("/audiobooks/:id/rename/apply", s.perm(auth.PermLibraryOrganize), s.applyRename)

			// Organize preview and execute (single book)
			protected.GET("/audiobooks/:id/preview-organize", s.perm(auth.PermLibraryOrganize), s.previewOrganize)
			protected.POST("/audiobooks/:id/organize", s.perm(auth.PermLibraryOrganize), s.organizeBook)

			// AI-powered parsing routes
			protected.POST("/ai/parse-filename", s.perm(auth.PermLibraryEditMetadata), s.parseFilenameWithAI)
			protected.POST("/ai/test-connection", s.perm(auth.PermLibraryEditMetadata), s.testAIConnection)

			// AI Scan Pipeline
			aiScans := protected.Group("/ai/scans")
			aiScans.POST("", s.perm(auth.PermLibraryEditMetadata), s.startAIScan)
			aiScans.GET("", s.perm(auth.PermLibraryView), s.listAIScans)
			aiScans.GET("/compare", s.compareAIScans) // Must be before /:id to avoid conflict
			aiScans.GET("/:id", s.perm(auth.PermLibraryView), s.getAIScan)
			aiScans.GET("/:id/results", s.perm(auth.PermLibraryView), s.getAIScanResults)
			aiScans.POST("/:id/apply", s.perm(auth.PermLibraryEditMetadata), s.applyAIScanResults)
			aiScans.POST("/:id/cancel", s.perm(auth.PermLibraryEditMetadata), s.cancelAIScan)
			aiScans.DELETE("/:id", s.perm(auth.PermLibraryDelete), s.deleteAIScan)
			protected.POST("/metadata-sources/test", s.perm(auth.PermSettingsManage), s.testMetadataSource)
			protected.POST("/audiobooks/:id/parse-with-ai", s.perm(auth.PermLibraryEditMetadata), s.parseAudiobookWithAI)

			// Open Library dump routes
			protected.GET("/openlibrary/status", s.perm(auth.PermIntegrationsManage), s.getOLStatus)
			protected.POST("/openlibrary/download", s.perm(auth.PermIntegrationsManage), s.startOLDownload)
			protected.POST("/openlibrary/import", s.perm(auth.PermIntegrationsManage), s.startOLImport)
			protected.POST("/openlibrary/upload", s.perm(auth.PermIntegrationsManage), s.uploadOLDump)
			protected.DELETE("/openlibrary/data", s.perm(auth.PermIntegrationsManage), s.deleteOLData)

			// Work routes (logical title-level grouping)
			protected.GET("/works", s.perm(auth.PermLibraryView), s.listWorks)
			protected.POST("/works", s.perm(auth.PermLibraryEditMetadata), s.createWork)
			protected.GET("/works/:id", s.perm(auth.PermLibraryView), s.getWork)
			protected.PUT("/works/:id", s.perm(auth.PermLibraryEditMetadata), s.updateWork)
			protected.DELETE("/works/:id", s.perm(auth.PermLibraryDelete), s.deleteWork)
			protected.GET("/works/:id/books", s.perm(auth.PermLibraryView), s.listWorkBooks)

			// Version management routes
			protected.GET("/audiobooks/:id/versions", s.perm(auth.PermLibraryView), s.listAudiobookVersions)
			protected.POST("/audiobooks/:id/versions", s.perm(auth.PermLibraryEditMetadata), s.linkAudiobookVersion)
			protected.PUT("/audiobooks/:id/set-primary", s.perm(auth.PermLibraryEditMetadata), s.setAudiobookPrimary)
			protected.POST("/audiobooks/:id/split-version", s.perm(auth.PermLibraryEditMetadata), s.splitVersion)
			protected.POST("/audiobooks/:id/split-to-books", s.perm(auth.PermLibraryEditMetadata), s.splitSegmentsToBooks)
			protected.POST("/audiobooks/:id/move-segments", s.perm(auth.PermLibraryEditMetadata), s.moveSegments)
			protected.GET("/version-groups/:id", s.perm(auth.PermLibraryView), s.getVersionGroup)

			// Work queue routes (alternative singular form for compatibility)
			protected.GET("/work", s.perm(auth.PermLibraryView), s.listWork)
			protected.GET("/work/stats", s.perm(auth.PermLibraryView), s.getWorkStats)

			// Update routes
			protected.GET("/update/status", s.perm(auth.PermSettingsManage), s.getUpdateStatus)
			protected.POST("/update/check", s.perm(auth.PermSettingsManage), s.checkForUpdate)
			protected.POST("/update/apply", s.perm(auth.PermSettingsManage), s.applyUpdate)

			// Blocked hashes management routes
			protected.GET("/blocked-hashes", s.perm(auth.PermLibraryView), s.listBlockedHashes)
			protected.POST("/blocked-hashes", s.perm(auth.PermLibraryEditMetadata), s.addBlockedHash)
			protected.DELETE("/blocked-hashes/:hash", s.perm(auth.PermLibraryDelete), s.removeBlockedHash)

			// Diagnostics routes
			protected.POST("/diagnostics/export", s.perm(auth.PermSettingsManage), s.startDiagnosticsExport)
			protected.GET("/diagnostics/export/:operationId/download", s.perm(auth.PermSettingsManage), s.downloadDiagnosticsExport)
			protected.POST("/diagnostics/submit-ai", s.perm(auth.PermSettingsManage), s.submitDiagnosticsAI)
			protected.GET("/diagnostics/ai-results/:operationId", s.perm(auth.PermSettingsManage), s.getDiagnosticsAIResults)
			protected.POST("/diagnostics/apply-suggestions", s.perm(auth.PermSettingsManage), s.applyDiagnosticsSuggestions)

			// Bench routes (only available with -tags bench)
			s.setupUserTagRoutes(protected)
			s.registerReadingRoutes(protected)
			s.registerPlaylistRoutes(protected)
			s.registerUserAdminRoutes(protected)
			s.registerVersionLifecycleRoutes(protected)
			s.registerEntityTagRoutes(protected)
			s.registerDelugeRoutes(protected)
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
	if database.GetGlobalStore() != nil {
		importPaths, err := database.GetGlobalStore().GetAllImportPaths()
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

// triggerITunesSync finds the library path from DB and enqueues a sync if the file changed.
func (s *Server) triggerITunesSync() {
	if s.Store() == nil || s.queue == nil {
		return
	}

	libraryPath := discoverITunesLibraryPath(s.Store())
	if libraryPath == "" {
		return
	}

	// Check fingerprint — skip if unchanged (quick mtime+size check)
	if rec, err := s.Store().GetLibraryFingerprint(libraryPath); err == nil && rec != nil {
		if info, statErr := os.Stat(libraryPath); statErr == nil {
			if info.Size() == rec.Size && info.ModTime().Equal(rec.ModTime) {
				return // No changes
			}
		}
	}

	itunesTriggerLog := logger.NewWithActivityLog("itunes-scheduler", s.Store())
	opID := ulid.Make().String()
	op, err := s.Store().CreateOperation(opID, "itunes_sync", &libraryPath)
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
		return executeITunesSync(ctx, s.Store(), operations.LoggerFromReporter(progress), libraryPath, scheduledMappings, s.itunesActivityFn)
	}

	if err := s.queue.Enqueue(op.ID, "itunes_sync", operations.PriorityNormal, operationFunc); err != nil {
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
	if s.Store() == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	ops, err := s.Store().GetRecentOperations(500)
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
	staleLog := logger.NewWithActivityLog("reaper", s.Store())
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
		if err := s.Store().UpdateOperationError(op.ID, msg); err != nil {
			staleLog.Warn("failed to mark stale operation %s as failed: %v", op.ID, err)
			continue
		}
		if s.hub != nil {
			s.hub.SendOperationStatus(op.ID, "failed", map[string]any{
				"error": msg,
			})
		}
		staleLog.Warn("marked stale operation as failed: id=%s type=%s", op.ID, op.Type)
	}
}

// --- User tag handlers ---

// ---- Work handlers ----

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

// Version Management Handlers

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

// reassignExternalIDsForFiles moves external ID mappings (iTunes PIDs) from
// sourceBookID to targetBookID for the given files. It matches by file_path or
// ITunesPersistentID on the external_id_map entries.
func reassignExternalIDsForFiles(sourceBookID, targetBookID string, files []database.BookFile) {
	eidStore := asExternalIDStore(database.GetGlobalStore())
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
		_ = database.GetGlobalStore().DeleteRaw(oldReverseKey)

		m.BookID = targetBookID
		if createErr := eidStore.CreateExternalIDMapping(&m); createErr != nil {
			log.Printf("[WARN] reassignExternalIDsForFiles: failed to reassign %s:%s to %s: %v",
				m.Source, m.ExternalID, targetBookID, createErr)
		}
	}

	log.Printf("[INFO] reassigned %d external ID mapping(s) from book %s to %s",
		len(toMove), sourceBookID, targetBookID)
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

// Author alias handlers

// --- AI Scan Pipeline Handlers ---

// --- Preview Rename & Metadata Writeback Handlers ---

// --- Preview Organize & Single-Book Organize Handlers ---
