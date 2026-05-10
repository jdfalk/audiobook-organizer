// file: internal/server/server.go
// version: 2.10.0
// guid: 4c5d6e7f-8a9b-0c1d-2e3f-4a5b6c7d8e9f
// last-edited: 2026-05-08

package server

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/activity"
	"github.com/jdfalk/audiobook-organizer/internal/ai"
	"github.com/jdfalk/audiobook-organizer/internal/aiscan"
	audiobookspkg "github.com/jdfalk/audiobook-organizer/internal/audiobooks"
	"github.com/jdfalk/audiobook-organizer/internal/cache"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/dedup"
	"github.com/jdfalk/audiobook-organizer/internal/deluge"
	"github.com/jdfalk/audiobook-organizer/internal/diagnostics"
	"github.com/jdfalk/audiobook-organizer/internal/itunes"
	itunesservice "github.com/jdfalk/audiobook-organizer/internal/itunes/service"
	"github.com/jdfalk/audiobook-organizer/internal/logger"
	"github.com/jdfalk/audiobook-organizer/internal/maintenance"
	_ "github.com/jdfalk/audiobook-organizer/internal/maintenance/jobs"
	"github.com/jdfalk/audiobook-organizer/internal/merge"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
	"github.com/jdfalk/audiobook-organizer/internal/metafetch"
	"github.com/jdfalk/audiobook-organizer/internal/metrics"
	"github.com/jdfalk/audiobook-organizer/internal/importer"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	opsregistry "github.com/jdfalk/audiobook-organizer/internal/operations/registry"
	acoustidplugin "github.com/jdfalk/audiobook-organizer/internal/plugins/acoustid"
	dedupplugin "github.com/jdfalk/audiobook-organizer/internal/plugins/dedup"
	delugeplug "github.com/jdfalk/audiobook-organizer/internal/plugins/deluge"
	itunesplug "github.com/jdfalk/audiobook-organizer/internal/plugins/itunes"
	maintenanceplugin "github.com/jdfalk/audiobook-organizer/internal/plugins/maintenance"
	"github.com/jdfalk/audiobook-organizer/internal/organizer"
	"github.com/jdfalk/audiobook-organizer/internal/plugin"
	"github.com/jdfalk/audiobook-organizer/internal/quarantine"
	"github.com/jdfalk/audiobook-organizer/internal/realtime"
	"github.com/jdfalk/audiobook-organizer/internal/scanner"
	"github.com/jdfalk/audiobook-organizer/internal/search"
	servermiddleware "github.com/jdfalk/audiobook-organizer/internal/server/middleware"
	"github.com/jdfalk/audiobook-organizer/internal/fileops"
	"github.com/jdfalk/audiobook-organizer/internal/sysinfo"
	"github.com/jdfalk/audiobook-organizer/internal/tagger"
	"github.com/jdfalk/audiobook-organizer/internal/updater"
	"github.com/quic-go/quic-go/http3"
)

// Cached library and import path sizes to avoid expensive recalculation on frequent status checks
var cachedLibrarySize int64
var cachedImportSize int64
var cachedSizeComputedAt time.Time
var cacheLock sync.RWMutex

const librarySizeCacheTTL = 24 * time.Hour

// appVersion is set at startup via SetVersion(), injected from main.version
var appVersion = "dev"

// SetVersion sets the application version string.

// resetLibrarySizeCache resets the library size cache (for testing)

// Helper functions for pointer conversions

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
	return ai.NewOpenAIParser(&config.AppConfig, apiKey, enabled)
}

// enrichedBookResponse wraps a Book with resolved names for JSON responses.
type enrichedBookResponse struct {
	*database.Book
	AuthorName                       *string         `json:"author_name,omitempty"`
	SeriesName                       *string         `json:"series_name,omitempty"`
	Authors                          []authorEntry   `json:"authors,omitempty"`
	Narrators                        []narratorEntry `json:"narrators,omitempty"`
	FileExists                       *bool           `json:"file_exists,omitempty"`
	MetadataSourceHashDuplicateCount *int            `json:"metadata_source_hash_duplicate_count,omitempty"`
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

// batchFetchBookAuthorsAndNarrators pre-fetches author and narrator join table
// entries plus their full details for all given books. Returns maps keyed by
// book ID for join entries, plus maps keyed by author/narrator ID for details.
// Nil maps are returned if the global store is not available.

// enrichBookForResponseSingle enriches a single book by pre-fetching its
// author and narrator data. Convenience wrapper for single-book endpoints.

// enrichBookForResponse resolves author, series, and narrator names from join
// tables so the JSON response contains all the fields the frontend expects.
// Pre-fetched maps of authors and narrators (by book ID) are used instead of
// per-book DB calls to eliminate N+1 queries.

// buildComparisonValuesFromActivityLog reconstructs a "before" tag snapshot by
// querying the activity log for metadata_apply entries for the given book
// recorded within a ±5 second window of ts. For each field found, the
// old_value (i.e. the value BEFORE that operation) is used as the comparison
// value. This is the fallback when GetBookAtVersion is unavailable (SQLite) or
// when the exact version key is not present in PebbleDB.

// nonEmpty returns the string if non-empty, nil otherwise (for comparison map building).

// calculateLibrarySizes computes library and import path sizes with caching

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
	audiobookService       *audiobookspkg.AudiobookService
	audiobookUpdateService *AudiobookUpdateService
	batchService           *BatchService
	workService            *WorkService
	authorSeriesService    *audiobookspkg.AuthorSeriesService
	filesystemService      *fileops.FilesystemService
	importPathService      *importer.ImportPathService
	importService          *importer.ImportService
	scanService            *scanner.ScanService
	organizeService        *OrganizeService
	metadataFetchService   *metafetch.Service
	configUpdateService    *ConfigUpdateService
	systemService          *sysinfo.SystemService
	metadataStateService   *MetadataStateService
	dashboardService       *DashboardService
	olService              *metafetch.OpenLibraryService
	dedupCache             *cache.Cache[gin.H]
	listCache              *cache.Cache[gin.H]
	facetsCache            *cache.Cache[gin.H]
	libraryWatcher         *itunes.LibraryWatcher
	itunesSvc              *itunesservice.Service
	updater                *updater.Updater
	updateScheduler        *updater.Scheduler
	scheduler              *TaskScheduler
	aiScanStore            *database.AIScanStore
	pipelineManager        *aiscan.PipelineManager
	batchPoller            *BatchPoller
	mergeService           *merge.Service
	diagnosticsService     *diagnostics.Service
	changelogService       *activity.ChangelogService
	activityService        *activity.Service
	embeddingStore         *database.EmbeddingStore
	metricsStore           *database.MetricsStore
	dedupEngine            *dedup.Engine
	activityWriter         *activity.Writer
	itunesActivityFn       func(entry database.ActivityEntry)
	eventBus               *plugin.EventBus
	pluginRegistry         *plugin.Registry
	quarantineSvc          *quarantine.QuarantineService
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
	writeBackBatcher *itunesservice.WriteBackBatcher
	fileIOPool       *FileIOPool
	// opRegistry is the UOS-02 registry. Plugins register their OperationDefs
	// here; the registry owns dispatch and worker pool lifecycle.
	// No plugins are registered until their own bot-tasks wire them in.
	opRegistry *opsregistry.Registry

	// opHub is the UOS-06 SSE event bus for operations events.
	// Created in NewServer, wired to opRegistry via SetBus before Start().
	opHub *opsregistry.EventHub

	// protectedPathCache holds the union of Deluge save_paths and
	// config.ProtectedPaths. Consulted before any in-place tag write.
	// Nil when Deluge is not configured (extra paths only, or no Deluge URL).
	protectedPathCache *deluge.ProtectedPathCache

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

// publishEvent publishes a lifecycle event to the plugin event bus.
func (s *Server) publishEvent(ctx context.Context, event plugin.Event) {
	if s.eventBus != nil {
		s.eventBus.Publish(ctx, event)
	}
}

// NewServer constructs a Server with an explicit Store dependency.
// s.Store() is still assigned at startup for code that hasn't
// been migrated to use s.Store() yet (see DI migration plan 4.4).
func NewServer(store database.Store) *Server {
	router := gin.New() // don't use gin.Default() — we add our own middleware

	// Custom logger that skips noisy polling endpoints
	// (UOS-14: /operations/active removed; SkipPaths entry removed)
	router.Use(gin.LoggerWithConfig(gin.LoggerConfig{
		SkipPaths: []string{"/api/v1/operations/events"},
	}))
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())
	router.Use(servermiddleware.BasicAuth())
	router.Use(gzip.Gzip(gzip.DefaultCompression, gzip.WithExcludedPaths([]string{"/api/events"})))

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
		audiobookService:       audiobookspkg.NewAudiobookService(resolvedStore),
		audiobookUpdateService: NewAudiobookUpdateService(resolvedStore),
		batchService:           NewBatchService(resolvedStore),
		workService:            NewWorkService(resolvedStore),
		authorSeriesService:    audiobookspkg.NewAuthorSeriesService(resolvedStore),
		filesystemService:      fileops.NewFilesystemService(resolvedStore),
		importPathService:      importer.NewImportPathService(resolvedStore),
		importService:          importer.NewImportService(resolvedStore),
		scanService:            scanner.NewScanService(resolvedStore),
		organizeService:        NewOrganizeService(resolvedStore),
		metadataFetchService:   metafetch.NewService(resolvedStore),
		configUpdateService:    NewConfigUpdateService(resolvedStore),
		systemService:          sysinfo.NewSystemService(resolvedStore, appVersion, calculateLibrarySizes),
		metadataStateService:   NewMetadataStateService(resolvedStore),
		dashboardService:       NewDashboardService(resolvedStore),
		dedupCache:             cache.New[gin.H]("dedup", 24*time.Hour),
		listCache:              cache.New[gin.H]("list", 24*time.Hour),
		facetsCache:            cache.New[gin.H]("facets", 24*time.Hour),
		olService:              metafetch.NewOpenLibraryService(),
		updater:                updater.NewUpdater(appVersion),
		mergeService:           merge.NewService(resolvedStore),
		diagnosticsService:     diagnostics.NewService(resolvedStore, nil, config.AppConfig.ITunesLibraryReadPath),
		changelogService:       activity.NewChangelogService(resolvedStore),
	}

	// Propagate rootDir into the store so LibraryStats can split organized vs unorganized.
	resolvedStore.SetRootDir(config.AppConfig.RootDir)

	// Inject the store into the maintenance package so jobs can access it.
	maintenance.InjectStore(resolvedStore)
	if server.writeBackBatcher != nil {
		maintenance.InjectEnqueuer(server.writeBackBatcher)
	}

	// Initialize plugin event bus and registry
	server.eventBus = plugin.NewEventBus()
	server.pluginRegistry = plugin.Global()
	server.quarantineSvc = quarantine.NewQuarantineService(resolvedStore, &config.AppConfig, server.eventBus)

	// Initialize the UOS-02 operations registry. The registry holds the
	// OperationDef registration table, dispatcher, and worker pool.
	// Plugins register their defs in their own bot-tasks (UOS-03+).
	server.opHub = opsregistry.NewEventHub()
	server.opRegistry = opsregistry.New(resolvedStore, slog.Default(), 8, server.opHub)

	// UOS-12: maintenance plugin — 26 ops migrated from scheduler_tasks.go.
	// Guard on RootDir: tests don't configure AppConfig, so RootDir is ""
	// and the mock store has no UpsertOpDefinitionV2 expectations.
	if config.AppConfig.RootDir != "" {
		if err := maintenanceplugin.New(server).Register(server.opRegistry); err != nil {
			log.Printf("[server] maintenance plugin register: %v", err)
		}
		if err := server.RegisterBulkMetadataFetchOp(server.opRegistry); err != nil {
			log.Printf("[server] bulk-metadata-fetch op register: %v", err)
		}
		if err := server.RegisterLibraryScanOp(server.opRegistry); err != nil {
			log.Printf("[server] library.scan op register: %v", err)
		}
		if err := server.RegisterLibraryOrganizeOp(server.opRegistry); err != nil {
			log.Printf("[server] library.organize op register: %v", err)
		}
		if err := server.RegisterLibraryTranscodeOp(server.opRegistry); err != nil {
			log.Printf("[server] library.transcode op register: %v", err)
		}
		if err := server.RegisterBulkWriteBackOp(server.opRegistry); err != nil {
			log.Printf("[server] bulk-write-back op register: %v", err)
		}
	}

	// Construct the iTunes service. Phase 2 M1 step 1 enables it via New()
	// so the real TrackProvisioner gets wired into the import pipeline;
	// remaining sub-components stay as empty-struct placeholders until
	// they move in later M1 steps. Disabled fallback is preserved for
	// construction failures or (future) test paths that explicitly opt out.
	itunesCfg := itunesservice.Config{
		Enabled:             true,
		LibraryReadPath:     config.AppConfig.ITunesLibraryReadPath,
		LibraryWritePath:    config.AppConfig.ITunesLibraryWritePath,
		AutoWriteBack:       config.AppConfig.ITunesAutoWriteBack,
		ITLWriteBackEnabled: config.AppConfig.ITLWriteBackEnabled,
	}
	// Build the operation queue before constructing the iTunes service
	// so PathReconciler / PathRepairer get a real handle. The duplicate
	// later in this function (kept for the GlobalQueue back-compat) is
	// a no-op once this assignment lands.
	if server.queue == nil {
		innerQ := operations.NewOperationQueue(resolvedStore, 8, nil, server.hub)
		server.queue = operations.NewBridgeQueue(innerQ, resolvedStore)
		// Long-running maintenance ops (path repair scans 80K+ files
		// across many spinning disks) need more than the 2-hour default.
		server.queue.SetOperationTimeout(6 * time.Hour)
		operations.GlobalQueue = server.queue
	}
	itunesSvc, err := itunesservice.New(itunesservice.Deps{
		Store:         resolvedStore,
		Config:        itunesCfg,
		OpQueue:       server.queue,
		AudiobookRoot: config.AppConfig.RootDir,
		ReportDir:     filepath.Join(config.AppConfig.RootDir, "reports"),
		OnBookCreated: func(bookID string) {
			// Resolved lazily via closure so server.fireDedupOnImport is available.
			server.fireDedupOnImport(bookID)
		},
		Metafetch: server.metadataFetchService,
		OrganizerFactory: func() itunesservice.BookOrganizer {
			return organizer.NewOrganizer(&config.AppConfig)
		},
	})
	if err != nil {
		log.Printf("[WARN] iTunes service construction failed, falling back to disabled: %v", err)
		itunesSvc = itunesservice.NewDisabled()
	}
	server.itunesSvc = itunesSvc

	// Register iTunes plugin (UOS-10)
	// Guard on RootDir: tests don't configure AppConfig, so RootDir="" and the
	// mock store has no UpsertOpDefinitionV2 expectations.
	if config.AppConfig.RootDir != "" && itunesSvc != nil && itunesSvc.Enabled() {
		itunesPlugin := itunesplug.New(itunesSvc, resolvedStore)
		if err := itunesPlugin.Register(server.opRegistry); err != nil {
			log.Printf("[server] iTunes plugin register: %v", err)
		}
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
			metafetch.NewISBNService(database.GetGlobalStore(), isbnSources),
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
				server.pipelineManager = aiscan.NewPipelineManager(aiScanStore, database.GetGlobalStore(), p)
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

	// Open metrics sidecar store for cache stats history. Always SQLite,
	// independent of the primary store backend (PebbleDB or SQLite), so
	// /api/v1/cache/stats/history works everywhere.
	if dbPath := config.AppConfig.DatabasePath; dbPath != "" {
		metricsDBPath := filepath.Join(filepath.Dir(dbPath), "metrics.db")
		metricsStore, err := database.NewMetricsStore(metricsDBPath)
		if err != nil {
			log.Printf("[WARN] Failed to open metrics store: %v", err)
		} else {
			server.metricsStore = metricsStore
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
				llmParser := ai.NewOpenAIParser(&config.AppConfig, config.AppConfig.OpenAIAPIKey, config.AppConfig.EnableAIParsing)
				server.dedupEngine = dedup.NewEngine(
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

					// Hydrate chromem from the SQLite embeddings table in
					// the background. Without this, an empty or stale
					// chromem dir means Layer 2 returns zero matches even
					// though tens of thousands of embeddings exist on disk.
					// Run async so it doesn't block startup; the dedup
					// engine works (slowly) before hydration completes
					// because mirrorBookToChromem populates entries on
					// demand whenever EmbedBook is called.
					go func() {
						hCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
						defer cancel()
						books, authors, err := server.dedupEngine.HydrateChromem(hCtx)
						if err != nil {
							log.Printf("[WARN] chromem hydrate finished with error: %v (books=%d authors=%d)", err, books, authors)
							return
						}
						log.Printf("[INFO] chromem hydrate complete: books=%d authors=%d", books, authors)
					}()
				}

				// Wire aijobs store for async dedup review batches.
				if aiJobsStore, ok := database.GetGlobalStore().(database.AIJobsStore); ok {
					server.dedupEngine.SetAIJobsStore(aiJobsStore)
					// Register the engine as the dedup verdict applier for batch callbacks.
					ai.SetDedupVerdictApplier(server.dedupEngine)
					log.Println("[INFO] Dedup async review (aijobs) wired")
				} else {
					log.Println("[WARN] Global store does not implement AIJobsStore; dedup async review disabled")
				}

				log.Println("[INFO] Embedding store and dedup engine initialized")
				server.metadataFetchService.SetDedupEngine(server.dedupEngine)

				// Register UOS dedup plugin (UOS-07). Done here so the engine
				// and embeddingStore are both available.
				if err := dedupplugin.New(server.dedupEngine, resolvedStore, server.embeddingStore).Register(server.opRegistry); err != nil {
					log.Printf("[server] dedup plugin register: %v", err)
				}

				// Register acoustid plugin so acoustid.fingerprint-rescan
				// and acoustid.scan op-defs are available via the UOS registry.
				if err := acoustidplugin.New(server.dedupEngine, resolvedStore, server.embeddingStore).Register(server.opRegistry); err != nil {
					log.Printf("[server] acoustid plugin register: %v", err)
				}

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

	if server.queue == nil {
		innerQ := operations.NewOperationQueue(resolvedStore, 8, nil, server.hub)
		server.queue = operations.NewBridgeQueue(innerQ, resolvedStore)
		operations.GlobalQueue = server.queue
	}

	// The batcher moved under itunesservice.Service in Phase 2 M1 step 2.
	// Server still keeps a typed field for back-compat with the many call
	// sites that were already using server.writeBackBatcher — but it now
	// points at the service-owned instance. When the service is nil (test
	// paths), the field stays nil and enqueues are silent no-ops via the
	// `if batcher != nil` guards already in place.
	server.writeBackBatcher = server.itunesSvc.Batcher
	server.fileIOPool = NewFileIOPool(4)

	// Wire writeBackBatcher into services that need it
	server.metadataFetchService.SetWriteBackBatcher(server.writeBackBatcher)
	server.organizeService.SetWriteBackBatcher(server.writeBackBatcher)
	server.organizeService.SetQueue(server.queue)
	server.mergeService.SetWriteBackBatcher(server.writeBackBatcher)
	server.quarantineSvc.SetWriteBackBatcher(server.writeBackBatcher)
	if server.audiobookService != nil && server.writeBackBatcher != nil {
		server.audiobookService.SetITunesEnqueuer(server.writeBackBatcher)
	}

	// Wire iTunes-specific organizer callbacks now that itunesSvc is ready.
	if server.itunesSvc.Enabled() {
		server.organizeService.DiscoverITunesLibraryPath = func(_ database.Store) string {
			return server.itunesSvc.Importer.DiscoverLibraryPath()
		}
		server.organizeService.ExecuteITunesSync = func(ctx context.Context, _ database.Store, log logger.Logger, libraryPath string) error {
			return server.itunesSvc.Importer.Sync(ctx, libraryPath, nil, server.itunesActivityFn, log)
		}
	}

	// ImportService uses the iTunes service's TrackProvisioner (moved
	// during Phase 2 M1 step 1). Nil provisioner → ITL track provisioning
	// is skipped (service is disabled or construction failed above).
	server.importService.SetTrackProvisioner(server.itunesSvc.Provisioner)
	// After M1 step 2, the batcher is owned by itunesservice.Service and
	// Provisioner was wired with the real Enqueuer at Service.New() time.
	// No SetEnqueuer hop needed.

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
		server.queue.SetActivityLogger(&activityServiceLogger{svc: server.activityService})

		// Task 11/14: Metadata fetch service → activity log
		server.metadataFetchService.SetActivityService(server.activityService)

		// Wire activity service into audiobook service for snapshot comparison fallback
		server.audiobookService.SetActivityService(server.activityService)

		// Global log capture via teeWriter — replaces globalActivityRecorder
		aw := activity.NewWriter(server.activityService.Store(), 10000)
		aw.Start()
		server.activityWriter = aw
		server.scanService.SetActivityWriter(aw)
		log.SetOutput(aw)
		if server.itunesSvc != nil && server.itunesSvc.Enabled() {
			server.itunesSvc.Repair.SetActivityWriter(aw)
		}

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

	// Wire post-scan auto-quarantine hook.
	server.scanService.PostScanFn = server.quarantineSvc.AutoQuarantineFailedScans

	// Wire post-folder auto-organize hook (breaks scanner→organizer import cycle).
	server.scanService.AutoOrganizeFn = func(ctx context.Context, books []scanner.Book, l logger.Logger) {
		if len(books) == 0 {
			return
		}
		if !config.AppConfig.AutoOrganize || config.AppConfig.RootDir == "" {
			if config.AppConfig.AutoOrganize {
				l.Warn("Auto-organize enabled but root_dir not set")
			}
			return
		}
		org := organizer.NewOrganizer(&config.AppConfig)
		organized := 0
		for i := range books {
			if l.IsCanceled() {
				break
			}
			dbBook, err := server.store.GetBookByFilePath(books[i].FilePath)
			if err != nil || dbBook == nil {
				continue
			}
			newPath, _, err := org.OrganizeBook(dbBook)
			if err != nil {
				l.Warn("Organize failed for %s: %v", dbBook.Title, err)
				continue
			}
			if newPath != dbBook.FilePath {
				oldPath := dbBook.FilePath
				dbBook.FilePath = newPath
				scanner.ApplyOrganizedFileMetadata(dbBook, newPath)
				if _, err := server.store.UpdateBook(dbBook.ID, dbBook); err != nil {
					l.Error("Failed to update path for %s: %v — rolling back", dbBook.Title, err)
					if rbErr := os.Rename(newPath, oldPath); rbErr != nil {
						l.Error("CRITICAL: rollback failed for %s: file at %s, DB expects %s", dbBook.ID, newPath, oldPath)
					}
				} else {
					organized++
				}
			}
		}
		l.Info("Auto-organize complete: %d organized", organized)
	}

	// Note: the search index is opened in Start(), not here, so
	// tests that construct a Server without calling Start don't
	// leak Bleve file handles.

	// Initialize the protected-path cache. The cache merges Deluge save_paths
	// (fetched lazily on first IsProtected call) with any static prefixes from
	// config.ProtectedPaths. If Deluge is not configured, the cache still works
	// for the static paths (e.g. iTunes library root).
	{
		dc := getDelugeClient()
		server.protectedPathCache = deluge.NewProtectedPathCache(dc, config.AppConfig.ProtectedPaths)
		log.Printf("[INFO] ProtectedPathCache initialized (%d static extra paths)", len(config.AppConfig.ProtectedPaths))

		// Wire the pre-flight safe-write guard into the metadata package so that
		// all taglib writes (metadata apply, single-tag patch) check for Deluge-
		// protected paths before writing. This uses the same ProtectedPathCache
		// and a LibraryImporterAdapter backed by the server's store.
		importer := NewLibraryImporterAdapter(resolvedStore, dc, &config.AppConfig)
		deps := tagger.SafeWriteDeps{
			ProtectedCache: server.protectedPathCache,
			Importer:       importer,
		}
		metadata.SetSafeWriteDeps(deps)
		log.Printf("[INFO] metadata.SetSafeWriteDeps wired (Deluge pre-flight guard active)")

		// Also wire into the metafetch service so cover-art embeds use the guard.
		server.metadataFetchService.SetSafeWriteDeps(deps)
		log.Printf("[INFO] metafetch.Service.SetSafeWriteDeps wired (cover embed guard active)")

		// Register the Deluge plugin (UOS-11).
		// Guard on RootDir: tests don't configure AppConfig, so RootDir="" and the
		// mock store has no UpsertOpDefinitionV2 expectations.
		if config.AppConfig.RootDir != "" && dc != nil && server.protectedPathCache != nil {
			delugePlugin := delugeplug.New(dc, server.protectedPathCache, resolvedStore)
			if err := delugePlugin.Register(server.opRegistry); err != nil {
				log.Printf("[server] deluge plugin register: %v", err)
			}
		}
	}

	server.setupRoutes()

	// Initialize plugins after routes are set up so plugins can register
	// their own sub-routes under /api/v1/plugins/{id}/.
	server.initPlugins(bgCtx)

	return server
}

// SearchIndex returns the server's Bleve index, or nil if none is
// open. Handlers use this to decide whether to route queries
// through Bleve or fall back to the legacy SearchBooks path.

// safeWriteDeps builds a tagger.SafeWriteDeps from the server's wired
// dependencies. Used by movement_atom_cleanup and any other server-package
// code that calls tag-writing functions directly (outside the metadata
// package path that has its own package-level deps).

// buildSearchIndexIfEmpty runs a full reindex of the library when
// the search index has zero documents. Honors s.bgCtx so shutdown
// stops the backfill cleanly. Page size matches the existing
// backfill code to keep memory bounded.

// IndexBookByID reads a book (plus its related rows) and upserts
// the flat BookDocument into the search index. Best-effort: logs
// and returns nil if the index isn't open or the book is missing.
// Callers: handlers that create or update a book, plus the startup
// full-build goroutine.

// DeleteIndexedBook removes a book from the search index. Called
// after a book delete (soft or hard). Safe when the index isn't
// open.

// serverScanHooks implements scanner.ScanHooks, bridging scanner
// callbacks to the server's activity service and dedup engine.
type serverScanHooks struct {
	activityService *activity.Service
	dedupFn         func(bookID string)
}

// serverOrganizeHooks implements organizer.OrganizeHooks, bridging
// collision callbacks to the server's dedup engine.
type serverOrganizeHooks struct {
	server *Server
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

// resumeInterruptedOperations checks for operations left in running/queued state
// from a previous server lifecycle and re-enqueues them.

// Start starts the HTTP server

// perm returns a Gin middleware that checks the calling user has the
// given permission. It's a thin wrapper around RequirePermission from
// the middleware package, curried with the server's Store. Used inline
// in route registration: `protected.GET("/path", s.perm(P), s.handler)`.

// itunesSvcGuard returns a gin handler that checks s.itunesSvc is non-nil
// and enabled before delegating to fn. Any route that directly dereferences
// a sub-component (Paths.Start, Transfer.*) must be wrapped here so that
// setupRoutes doesn't panic when itunesSvc is nil or disabled.

// setupRoutes configures all the routes

// corsMiddleware adds restrictive CORS headers.

// filesCommonDir finds the common parent directory of all BookFile file paths.

// isProtectedPath returns true if the given file path is under an import path
// or the iTunes library folder. Files in protected paths must NEVER be moved,
// renamed, or deleted — only hardlinked or copied to the organized library.

// loadDismissedDedupGroups loads the set of dismissed dedup group keys from user preferences.

// saveDismissedDedupGroups saves the set of dismissed dedup group keys to user preferences.

// triggerITunesSync finds the library path from DB and enqueues a sync if the file changed.

// --- User tag handlers ---

// ---- Work handlers ----

// extractSeriesNameForDedup tries to extract the actual series name from patterns like
// "Book Title: Series Name" or "Series Name, Book 3". Returns the suggested
// series name and whether a suggestion was made.

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

// stripChapterFromTitle removes chapter/book numbers from titles to improve search results
// Examples: "The Odyssey: Book 01" -> "The Odyssey", "Harry Potter - Chapter 5" -> "Harry Potter"

// stripSubtitle removes subtitle portions from a title, e.g.
// "Title: A Subtitle" → "Title", "Title - A Subtitle" → "Title".
// Returns the original title if no subtitle separator is found.

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

// reassignExternalIDsForFiles moves external ID mappings (iTunes PIDs) from
// sourceBookID to targetBookID for the given files. It matches by file_path or
// ITunesPersistentID on the external_id_map entries.

// GetDefaultServerConfig returns default server configuration

// Author alias handlers

// --- AI Scan Pipeline Handlers ---

// --- Preview Rename & Metadata Writeback Handlers ---

// --- Preview Organize & Single-Book Organize Handlers ---
