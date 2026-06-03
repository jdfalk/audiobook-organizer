// file: internal/server/wire_handlers.go
// version: 2.4.0
// guid: f7a8b9c0-d1e2-3456-7890-abcdef012345
// last-edited: 2026-06-03

package server

import (
	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/ai"
	"github.com/jdfalk/audiobook-organizer/internal/auth"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	dedupengine "github.com/jdfalk/audiobook-organizer/internal/dedup"
	"github.com/jdfalk/audiobook-organizer/internal/server/handlers"
	entities "github.com/jdfalk/audiobook-organizer/internal/server/handlers/entities"
	operations "github.com/jdfalk/audiobook-organizer/internal/server/handlers/operations"
	system "github.com/jdfalk/audiobook-organizer/internal/server/handlers/system"
	servermiddleware "github.com/jdfalk/audiobook-organizer/internal/server/middleware"
	"github.com/jdfalk/audiobook-organizer/internal/undo"
)

// wireHandlers instantiates handler structs and registers their routes.
// Called from Start() after the protected group is created.
func (s *Server) wireHandlers(api *gin.RouterGroup, authMiddleware gin.HandlerFunc, protected *gin.RouterGroup) {
	authH := handlers.NewAuthHandler(s.Store(), config.AppConfig.EnableAuth)
	apiKeyH := handlers.NewAPIKeyHandler(s.Store())

	authGroup := api.Group("/auth")
	{
		authGroup.GET("/status", authH.GetStatus)
		authGroup.POST("/setup", authH.SetupInitialAdmin)
		authGroup.POST("/login", authH.Login)
		authGroup.POST("/accept-invite", s.handleAcceptInvite)
		authGroup.POST("/bootstrap", s.handleBootstrap)
	}

	authProtected := authGroup.Group("")
	authProtected.Use(authMiddleware)
	{
		authProtected.GET("/me", authH.Me)
		authProtected.PATCH("/me", authH.UpdateMe)
		authProtected.POST("/logout", authH.Logout)
		authProtected.GET("/sessions", authH.ListMySessions)
		authProtected.DELETE("/sessions/:id", authH.RevokeMySession)
		authProtected.PUT("/me/password", authH.ChangePassword)
		authProtected.POST("/temp-tokens", s.perm(permTempLoginMint()), s.createTempLoginToken)

		authProtected.POST("/api-keys", apiKeyH.Create)
		authProtected.GET("/api-keys", apiKeyH.List)
		authProtected.GET("/api-keys/:id", apiKeyH.Get)
		authProtected.PATCH("/api-keys/:id", apiKeyH.UpdateStatus)
		authProtected.DELETE("/api-keys/:id", apiKeyH.Revoke)
	}

	// ── Build split-book candidate store ─────────────────────────────────────
	var splitBookCands handlers.SplitBookCandidateStore
	if s.embeddingStore != nil {
		if db := s.embeddingStore.PebbleDB(); db != nil {
			splitBookCands = dedupengine.NewSplitBookStore(db)
		}
	}

	// ── Instantiate Phase 2 handlers ─────────────────────────────────────────
	cacheH := handlers.NewCacheHandler(s.metricsStore, s.Store())
	activityH := handlers.NewActivityHandler(s.activityService, s.Store())
	readingH := handlers.NewReadingHandler(s.Store())
	userH := handlers.NewUserHandler(s.Store())
	splitBookH := handlers.NewSplitBookHandler(s.opRegistry, splitBookCands, s.Store())
	metaCacheH := handlers.NewMetadataCacheHandler(s.Store(), s.metadataFetchService, s.writeBackBatcher)
	organizeH := handlers.NewOrganizeHandler(
		s.Store(),
		NewRenameService(s.Store()),
		NewOrganizePreviewService(s.Store()),
		s.organizeService,
		s.writeBackBatcher,
		s.eventBus,
		config.AppConfig.RootDir,
		config.AppConfig.AutoOrganize,
	)
	filesystemH := handlers.NewFilesystemHandler(
		s.Store(),
		s.filesystemService,
		s.importPathService,
		s.importService,
		s.opRegistry,
		s.eventBus,
		config.AppConfig.RootDir,
		config.AppConfig.AutoOrganize,
	)
	playlistH := handlers.NewPlaylistHandlerWithGetter(s.Store(), s.SearchIndex)
	pluginsH := handlers.NewPluginsHandler(s.pluginRegistry, config.AppConfig.Plugins)
	versionsH := handlers.NewVersionsHandler(s.Store())

	// Entities domain handler (authors/series/narrators/works). Guard typed-nil
	// boxing for each interface-typed dep so the handler's nil checks (and the
	// concrete pointers' own nil semantics) are preserved. workService and
	// authorSeriesService are concrete *struct pointers on Server that are
	// always constructed in NewServer, but the guards keep parity with the
	// established wiring pattern and are harmless.
	var entWorkSvc entities.WorkService
	if s.workService != nil {
		entWorkSvc = s.workService
	}
	var entAuthorSeriesSvc entities.AuthorSeriesService
	if s.authorSeriesService != nil {
		entAuthorSeriesSvc = s.authorSeriesService
	}
	var entOpReg entities.OperationsRegistry
	if s.opRegistry != nil {
		entOpReg = s.opRegistry
	}
	// enrichBooks mirrors the original getAuthorBooks/getSeriesBooks loop: one
	// batch fetch, then per-book enrichment in order. Returns a non-nil slice so
	// the JSON "items" field is [] (never null) for an empty book list.
	enrichBooks := func(books []database.Book) []any {
		bookIDs := make([]string, len(books))
		for i, b := range books {
			bookIDs[i] = b.ID
		}
		bookAuthorsMap, authorsByID, bookNarratorsMap, narratorsByID := s.batchFetchBookAuthorsAndNarrators(bookIDs)
		out := make([]any, len(books))
		for i := range books {
			out[i] = s.enrichBookForResponse(&books[i], bookAuthorsMap, authorsByID, bookNarratorsMap, narratorsByID)
		}
		return out
	}
	entitiesH := entities.New(
		s.Store(),
		entWorkSvc,
		entAuthorSeriesSvc,
		entOpReg,
		s.authorsCache,
		s.seriesCache,
		s.dedupCache,
		enrichBooks,
	)

	// Guard typed-nil boxing of the operations registry and event hub. Both are
	// concrete pointers on Server (*opsregistry.Registry / *opsregistry.EventHub)
	// that can legitimately be nil (e.g. container without an opregistry entry;
	// see server_queue_test.go). Boxing a nil pointer into an interface yields a
	// non-nil interface, which would defeat the handlers' `h.registry == nil` /
	// `h.hub == nil` guards (they mirror the old `s.opRegistry == nil` checks on
	// the concrete pointers) and panic instead of returning a clean 500/503.
	var opReg handlers.OperationsRegistry
	if s.opRegistry != nil {
		opReg = s.opRegistry
	}
	var opEventHub handlers.OperationsEventHub
	if s.opHub != nil {
		opEventHub = s.opHub
	}

	// Resolve the opsV2 store from the composite store (nil if unsupported).
	var opsV2 database.OpsV2Store
	if st := s.Store(); st != nil {
		if v2, ok := st.(database.OpsV2Store); ok {
			opsV2 = v2
		}
	}
	opsV2H := handlers.NewOperationsV2Handler(opsV2, opReg, opEventHub)

	// Operations domain handler (scan/organize/optimize/transcode triggers,
	// operation status/logs/result/changes/revert, stale-op management, DB
	// optimize, tasks, maintenance window). Guard typed-nil boxing for each
	// interface-typed concrete-pointer dep: s.opRegistry (*opsregistry.Registry),
	// s.pipelineManager (*aiscan.PipelineManager) and s.aiScanStore
	// (*database.AIScanStore) can all legitimately be nil; boxing a nil concrete
	// pointer into the interface would yield a non-nil interface and defeat the
	// handler's in-method nil guards (which mirror the old `s.opRegistry == nil` /
	// `s.pipelineManager != nil && s.aiScanStore != nil` checks). opRegistry,
	// pipelineManager and aiScanStore are all wired before setupRoutes
	// (wireServerFromContainer) and aiScanStore is only re-nilled during
	// shutdown, so snapshotting them here is safe. s.scheduler is the exception:
	// it is assigned in Start() AFTER this runs, so it is passed as a lazy
	// provider closure (below) instead of a snapshot.
	var opsOpReg operations.OperationsRegistry
	if s.opRegistry != nil {
		opsOpReg = s.opRegistry
	}
	var opsPipeline operations.ScanCanceler
	if s.pipelineManager != nil {
		opsPipeline = s.pipelineManager
	}
	var opsScanStore operations.AIScanLister
	if s.aiScanStore != nil {
		opsScanStore = s.aiScanStore
	}
	// collectStale stays in package server (also called from server_lifecycle.go).
	// preflightUndo / revert wrap server-private re-export helpers that consume a
	// full database.Store opaquely; the controller closes over s.Store().
	operationsH := operations.New(
		s.Store(),
		opsOpReg,
		// Lazy scheduler provider: s.scheduler is assigned in Start() (after this
		// wire-time runs), so resolve it at request time. Guard inside the
		// closure so a nil *scheduler.TaskScheduler is not boxed into a non-nil
		// interface (which would defeat the handler's nil check).
		func() operations.Scheduler {
			if s.scheduler == nil {
				return nil
			}
			return s.scheduler
		},
		opsPipeline,
		opsScanStore,
		s.collectStaleOperations,
		func(id string) (*undo.UndoConflictReport, error) {
			return undo.PreflightUndoConflicts(s.Store(), id)
		},
		func(id string) error {
			return NewRevertService(s.Store()).RevertOperation(id)
		},
	)
	// getSystemLogs (system handler) delegates its operation_id branch to
	// operationsH.GetOperationLogs; stash it on the Server for that call.
	s.operationsHandler = operationsH

	// System domain handler (health/status/announcements/storage/logs/
	// activity-log/reset/factory-reset/config/SSE events/backups/dashboard/
	// blocked-hashes/user-preferences/policy-tags/quick-queries). Guard typed-nil
	// boxing for each interface-typed concrete-pointer dep so the handler's
	// in-method nil guards (which mirror the old `s.X == nil` / `s.X != nil`
	// checks on the concrete pointers) are preserved: boxing a nil concrete
	// pointer into an interface yields a non-nil interface and would defeat them.
	// s.systemService / s.configUpdateService are concrete *struct pointers always
	// constructed in NewServer, but the guards keep parity with the established
	// pattern and are harmless. s.pluginRegistry can legitimately be nil
	// (HealthCheckAll skipped). s.hub is passed as a LAZY provider closure (not a
	// snapshot): handleEvents read s.hub at request time, and a test nils s.hub
	// AFTER wiring to drive the SSE 503 guard — snapshotting it here would capture
	// a live hub and invoke HandleSSE instead of 503 (mirrors the operations
	// getScheduler seam). s.olService is passed as a CONCRETE
	// pointer (factoryReset reaches its .Mu / .OLStore fields, which an interface
	// cannot abstract); the handler nil-checks it directly. s.operationsHandler
	// (set just above) backs OperationLogsProvider. The store is passed as a LAZY
	// provider closure (not a snapshot): the original handlers read s.Store() at
	// request time, and a router-integration test swaps server.store post-wire to
	// inject a mock — snapshotting would miss it. s.Store() returns the
	// database.Store interface, so a nil store stays a nil interface.
	var sysSvc system.SystemService
	if s.systemService != nil {
		sysSvc = s.systemService
	}
	var sysConfigUpdate system.ConfigUpdateService
	if s.configUpdateService != nil {
		sysConfigUpdate = s.configUpdateService
	}
	var sysPlugins system.PluginHealthChecker
	if s.pluginRegistry != nil {
		sysPlugins = s.pluginRegistry
	}
	var sysOpLogs system.OperationLogsProvider
	if s.operationsHandler != nil {
		sysOpLogs = s.operationsHandler
	}
	systemH := system.New(
		// Lazy store provider: resolve s.Store() at request time (late binding, as
		// the original handlers did). A test swaps server.store post-wire, so a
		// snapshot would miss it. s.Store() returns the database.Store interface,
		// so a nil store stays a nil interface and the handler's store==nil guards
		// hold.
		func() system.SystemStore { return s.Store() },
		sysSvc,
		sysConfigUpdate,
		sysPlugins,
		// Lazy hub provider: resolve s.hub at request time with a typed-nil guard
		// so a nil *realtime.EventHub is never boxed into a non-nil interface.
		func() system.EventStreamer {
			if s.hub == nil {
				return nil
			}
			return s.hub
		},
		sysOpLogs,
		s.olService, // concrete pointer; handler nil-checks it for field access
		getDiskStats,
		resetLibrarySizeCache,
		func() string { return appVersion },
		s.filterReviewedAuthorGroups,
	)
	s.systemHandler = systemH

	// iTunes handlers. Guard the service/importer wiring: s.itunesSvc is set
	// from the service registry and may be nil (iTunes disabled / not
	// configured). Boxing a typed-nil *Service into the interface would make
	// the handler's `h.svc == nil` guard read false, so only assign when the
	// concrete service is non-nil.
	var itSvc handlers.ITunesService
	var itImporter handlers.ITunesImporter
	if s.itunesSvc != nil {
		itSvc = s.itunesSvc
		itImporter = s.itunesSvc.Importer
	}
	itunesH := handlers.NewITunesHandler(itSvc, itImporter, opReg, s.Store())

	// AI handlers. Guard each concrete dependency so a typed-nil pointer is not
	// boxed into the handler's interface fields — that would defeat the
	// `h.scanStore == nil` / `h.pipeline == nil` guards (which mirror the old
	// `s.aiScanStore == nil` checks on the concrete pointers).
	var aiScanStore handlers.AIScanStore
	if s.aiScanStore != nil {
		aiScanStore = s.aiScanStore
	}
	var aiPipeline handlers.AIPipeline
	if s.pipelineManager != nil {
		aiPipeline = s.pipelineManager
	}
	var aiUpdater handlers.AudiobookUpdater
	if s.audiobookUpdateService != nil {
		aiUpdater = s.audiobookUpdateService
	}
	aiH := handlers.NewAIHandler(
		s.Store(),
		aiScanStore,
		aiPipeline,
		aiUpdater,
		s.dedupCache,
		opReg,
		func(b *database.Book) any { return s.enrichBookForResponseSingle(b) },
	)

	// Diagnostics handlers. Resolve the AI batch parser from the (unexported)
	// batchPoller field — the handler cannot import package server, so the
	// controller reads parser here and passes it in. Guard typed-nil boxing of
	// the diagnostics/merge services so the handler's nil-fallback (lazy
	// construction) fires correctly.
	var diagParser *ai.OpenAIParser
	if s.batchPoller != nil {
		diagParser = s.batchPoller.parser
	}
	var diagSvc handlers.DiagnosticsService
	if s.diagnosticsService != nil {
		diagSvc = s.diagnosticsService
	}
	var diagMergeSvc handlers.MergeService
	if s.mergeService != nil {
		diagMergeSvc = s.mergeService
	}
	diagH := handlers.NewDiagnosticsHandler(
		s.Store(),
		diagSvc,
		diagMergeSvc,
		s.embeddingStore,
		s.aiScanStore,
		opReg,
		diagParser,
	)

	// ── Public cache routes (no auth) ────────────────────────────────────────
	api.GET("/cache/stats", cacheH.HandleCacheStats)
	api.GET("/cache/stats/history", cacheH.HandleCacheStatsHistory)

	// ── Protected routes ─────────────────────────────────────────────────────

	// Activity log
	protected.GET("/activity", s.perm(auth.PermLibraryView), activityH.ListActivity)
	protected.GET("/activity/sources", s.perm(auth.PermLibraryView), activityH.ListActivitySources)
	protected.POST("/activity/compact", s.perm(auth.PermSettingsManage), activityH.CompactActivity)
	protected.GET("/operations/:id/activity", s.perm(auth.PermLibraryView), activityH.ListOperationActivity)

	// Split-book dedup
	protected.POST("/dedup/split-book-scan", s.perm(auth.PermScanTrigger), splitBookH.TriggerSplitBookScan)
	protected.GET("/dedup/split-book-candidates", s.perm(auth.PermLibraryView), splitBookH.ListSplitBookCandidates)
	protected.POST("/dedup/split-book-candidates/:id/merge", s.perm(auth.PermLibraryEditMetadata), splitBookH.MergeSplitBookCandidate)

	// Filesystem + import paths
	protected.GET("/filesystem/home", s.perm(auth.PermSettingsManage), filesystemH.GetHomeDirectory)
	protected.GET("/filesystem/browse", s.perm(auth.PermSettingsManage), filesystemH.BrowseFilesystem)
	protected.POST("/filesystem/exclude", s.perm(auth.PermSettingsManage), filesystemH.CreateExclusion)
	protected.DELETE("/filesystem/exclude", s.perm(auth.PermSettingsManage), filesystemH.RemoveExclusion)
	protected.GET("/import-paths", s.perm(auth.PermSettingsManage), filesystemH.ListImportPaths)
	protected.POST("/import-paths", s.perm(auth.PermSettingsManage), filesystemH.AddImportPath)
	protected.DELETE("/import-paths/:id", s.perm(auth.PermSettingsManage), filesystemH.RemoveImportPath)
	protected.POST("/import/file", s.perm(auth.PermScanTrigger), filesystemH.ImportFile)

	// Organize + rename
	protected.POST("/audiobooks/:id/rename/preview", s.perm(auth.PermLibraryOrganize), organizeH.PreviewRename)
	protected.POST("/audiobooks/:id/rename/apply", s.perm(auth.PermLibraryOrganize), organizeH.ApplyRename)
	protected.GET("/audiobooks/:id/preview-organize", s.perm(auth.PermLibraryOrganize), organizeH.PreviewOrganize)
	protected.POST("/audiobooks/:id/organize", s.perm(auth.PermLibraryOrganize), organizeH.OrganizeBook)

	// Metadata cache
	protected.GET("/audiobooks/metadata/cached", s.perm(auth.PermLibraryView), metaCacheH.ListCachedCandidates)
	protected.GET("/audiobooks/metadata/cache/review", s.perm(auth.PermLibraryView), metaCacheH.GetCacheReviewResults)
	protected.POST("/audiobooks/metadata/batch-apply-cached", s.perm(auth.PermLibraryEditMetadata), metaCacheH.BatchApplyFromCache)
	protected.POST("/audiobooks/:id/clear-no-match", s.perm(auth.PermLibraryEditMetadata), metaCacheH.ClearMetadataNoMatch)

	// Reading progress
	protected.POST("/books/:id/position", readingH.SetPosition)
	protected.GET("/books/:id/position", readingH.GetPosition)
	protected.GET("/books/:id/state", readingH.GetBookState)
	protected.PATCH("/books/:id/status", readingH.SetBookStatus)
	protected.DELETE("/books/:id/status", readingH.ClearBookStatus)
	protected.GET("/me/:status", readingH.ListByStatus)

	// Playlists
	protected.GET("/playlists", s.perm(auth.PermLibraryView), playlistH.ListPlaylists)
	protected.POST("/playlists", playlistH.CreatePlaylist)
	protected.GET("/playlists/:id", playlistH.GetPlaylist)
	protected.PUT("/playlists/:id", playlistH.UpdatePlaylist)
	protected.DELETE("/playlists/:id", playlistH.DeletePlaylist)
	protected.POST("/playlists/:id/books", playlistH.AddBooksToPlaylist)
	protected.DELETE("/playlists/:id/books/:bookID", playlistH.RemoveBookFromPlaylist)
	protected.POST("/playlists/:id/reorder", playlistH.ReorderPlaylist)
	protected.POST("/playlists/:id/materialize", playlistH.MaterializePlaylist)

	// User management
	users := protected.Group("/users")
	{
		users.GET("", s.perm("users.manage"), userH.ListUsers)
		users.POST("/invite", s.perm("users.manage"), userH.CreateInvite)
		users.GET("/invites", s.perm("users.manage"), userH.ListInvites)
		users.DELETE("/invites/:token", s.perm("users.manage"), userH.DeleteInvite)
		users.POST("/:id/deactivate", s.perm("users.manage"), userH.DeactivateUser)
		users.POST("/:id/reactivate", s.perm("users.manage"), userH.ReactivateUser)
		users.POST("/:id/reset-password", s.perm("users.manage"), userH.ResetPassword)
	}

	// Version groups
	protected.GET("/audiobooks/:id/versions", s.perm(auth.PermLibraryView), versionsH.ListAudiobookVersions)
	protected.POST("/audiobooks/:id/versions", s.perm(auth.PermLibraryEditMetadata), versionsH.LinkAudiobookVersion)
	protected.PUT("/audiobooks/:id/set-primary", s.perm(auth.PermLibraryEditMetadata), versionsH.SetAudiobookPrimary)
	protected.POST("/audiobooks/:id/split-version", s.perm(auth.PermLibraryEditMetadata), versionsH.SplitVersion)
	protected.POST("/audiobooks/:id/split-to-books", s.perm(auth.PermLibraryEditMetadata), versionsH.SplitSegmentsToBooks)
	protected.POST("/audiobooks/:id/move-segments", s.perm(auth.PermLibraryEditMetadata), versionsH.MoveSegments)
	protected.GET("/version-groups/:id", s.perm(auth.PermLibraryView), versionsH.GetVersionGroup)

	// iTunes (12 migrated routes; survivors stay in server_lifecycle.go).
	// Two protected.Group("/itunes") blocks (here + survivors) is fine in Gin
	// since there is no duplicate method+path.
	itunesG := protected.Group("/itunes")
	{
		itunesG.POST("/validate", s.perm(auth.PermLibraryEditMetadata), itunesH.Validate)
		itunesG.POST("/test-mapping", s.perm(auth.PermLibraryEditMetadata), itunesH.TestMapping)
		itunesG.POST("/import", s.perm(auth.PermLibraryEditMetadata), itunesH.Import)
		itunesG.POST("/write-back", s.perm(auth.PermLibraryEditMetadata), itunesH.WriteBack)
		itunesG.POST("/write-back-all", s.perm(auth.PermLibraryEditMetadata), itunesH.WriteBackAll)
		itunesG.GET("/library-stats", s.perm(auth.PermLibraryView), itunesH.LibraryStats)
		itunesG.POST("/write-back/preview", s.perm(auth.PermLibraryEditMetadata), itunesH.WriteBackPreview)
		itunesG.GET("/books", s.perm(auth.PermLibraryView), itunesH.ListBooks)
		itunesG.GET("/import-status/:id", s.perm(auth.PermLibraryView), itunesH.ImportStatus)
		itunesG.POST("/import-status/bulk", s.perm(auth.PermLibraryEditMetadata), itunesH.ImportStatusBulk)
		itunesG.GET("/library-status", s.perm(auth.PermLibraryView), itunesH.LibraryStatus)
		itunesG.POST("/sync", s.perm(auth.PermLibraryEditMetadata), itunesH.Sync)
	}

	// AI domain (migrated from server_lifecycle.go).
	protected.POST("/authors/duplicates/ai-review", s.perm(auth.PermLibraryEditMetadata), aiH.ReviewDuplicateAuthors)
	protected.POST("/authors/duplicates/ai-review/apply", s.perm(auth.PermLibraryEditMetadata), aiH.ApplyAuthorReview)
	protected.POST("/ai/parse-filename", s.perm(auth.PermLibraryEditMetadata), aiH.ParseFilename)
	protected.POST("/ai/test-connection", s.perm(auth.PermLibraryEditMetadata), aiH.TestConnection)
	aiScans := protected.Group("/ai/scans")
	{
		aiScans.POST("", s.perm(auth.PermLibraryEditMetadata), aiH.StartScan)
		aiScans.GET("", s.perm(auth.PermLibraryView), aiH.ListScans)
		aiScans.GET("/compare", aiH.CompareScans) // Must be before /:id to avoid conflict
		aiScans.GET("/:id", s.perm(auth.PermLibraryView), aiH.GetScan)
		aiScans.GET("/:id/results", s.perm(auth.PermLibraryView), aiH.GetScanResults)
		aiScans.POST("/:id/apply", s.perm(auth.PermLibraryEditMetadata), aiH.ApplyScanResults)
		aiScans.POST("/:id/cancel", s.perm(auth.PermLibraryEditMetadata), aiH.CancelScan)
		aiScans.DELETE("/:id", s.perm(auth.PermLibraryDelete), aiH.DeleteScan)
	}
	protected.POST("/metadata-sources/test", s.perm(auth.PermSettingsManage), aiH.TestMetadataSource)
	protected.POST("/audiobooks/:id/parse-with-ai", s.perm(auth.PermLibraryEditMetadata), aiH.ParseAudiobook)
	protected.GET("/ai-jobs", s.perm(auth.PermSettingsManage), aiH.ListAIJobs)

	// Entities domain (migrated from server_lifecycle.go): authors, narrators,
	// series, and works. Paths + permission guards copied verbatim. Sibling
	// /authors/duplicates*, /series/duplicates*, /authors/duplicates/ai-review*
	// (now aiH.*) and the entity-tag routes stay on *Server / their own handlers.
	protected.GET("/authors", s.perm(auth.PermLibraryView), entitiesH.ListAuthors)
	protected.GET("/authors/count", s.perm(auth.PermLibraryView), entitiesH.CountAuthors)
	protected.POST("/authors/merge", s.perm(auth.PermLibraryEditMetadata), entitiesH.MergeAuthors)
	protected.POST("/authors/:id/reclassify-as-narrator", s.perm(auth.PermLibraryEditMetadata), entitiesH.ReclassifyAuthorAsNarrator)
	protected.PUT("/authors/:id/name", s.perm(auth.PermLibraryEditMetadata), entitiesH.RenameAuthor)
	protected.POST("/authors/:id/split", s.perm(auth.PermLibraryEditMetadata), entitiesH.SplitCompositeAuthor)
	protected.POST("/authors/:id/resolve-production", s.perm(auth.PermLibraryEditMetadata), entitiesH.ResolveProductionAuthor)
	protected.GET("/authors/:id/aliases", s.perm(auth.PermLibraryView), entitiesH.GetAuthorAliases)
	protected.POST("/authors/:id/aliases", s.perm(auth.PermLibraryEditMetadata), entitiesH.CreateAuthorAlias)
	protected.DELETE("/authors/:id/aliases/:aliasId", s.perm(auth.PermLibraryDelete), entitiesH.DeleteAuthorAlias)
	protected.GET("/authors/:id/books", s.perm(auth.PermLibraryView), entitiesH.GetAuthorBooks)
	protected.DELETE("/authors/:id", s.perm(auth.PermLibraryDelete), entitiesH.DeleteAuthor)
	protected.POST("/authors/bulk-delete", s.perm(auth.PermLibraryDelete), entitiesH.BulkDeleteAuthors)

	protected.GET("/narrators", s.perm(auth.PermLibraryView), entitiesH.ListNarrators)
	protected.GET("/narrators/count", s.perm(auth.PermLibraryView), entitiesH.CountNarrators)
	protected.GET("/audiobooks/:id/narrators", s.perm(auth.PermLibraryView), entitiesH.ListAudiobookNarrators)
	protected.PUT("/audiobooks/:id/narrators", s.perm(auth.PermLibraryEditMetadata), entitiesH.SetAudiobookNarrators)

	protected.GET("/series", s.perm(auth.PermLibraryView), entitiesH.ListSeries)
	protected.GET("/series/count", s.perm(auth.PermLibraryView), entitiesH.CountSeries)
	protected.PATCH("/series/:id", s.perm(auth.PermLibraryEditMetadata), entitiesH.UpdateSeriesName)
	protected.GET("/series/:id/books", s.perm(auth.PermLibraryView), entitiesH.GetSeriesBooks)
	protected.PUT("/series/:id/name", s.perm(auth.PermLibraryEditMetadata), entitiesH.RenameSeries)
	protected.POST("/series/:id/split", s.perm(auth.PermLibraryEditMetadata), entitiesH.SplitSeries)
	protected.DELETE("/series/:id", s.perm(auth.PermLibraryDelete), entitiesH.DeleteEmptySeries)
	protected.POST("/series/bulk-delete", s.perm(auth.PermLibraryDelete), entitiesH.BulkDeleteSeries)

	protected.GET("/works", s.perm(auth.PermLibraryView), entitiesH.ListWorks)
	protected.POST("/works", s.perm(auth.PermLibraryEditMetadata), entitiesH.CreateWork)
	protected.GET("/works/:id", s.perm(auth.PermLibraryView), entitiesH.GetWork)
	protected.PUT("/works/:id", s.perm(auth.PermLibraryEditMetadata), entitiesH.UpdateWork)
	protected.DELETE("/works/:id", s.perm(auth.PermLibraryDelete), entitiesH.DeleteWork)
	protected.GET("/works/:id/books", s.perm(auth.PermLibraryView), entitiesH.ListWorkBooks)
	protected.GET("/work", s.perm(auth.PermLibraryView), entitiesH.ListWork)
	protected.GET("/work/stats", s.perm(auth.PermLibraryView), entitiesH.GetWorkStats)

	// Diagnostics (migrated from server_lifecycle.go).
	protected.GET("/diagnostics/db-health", s.perm(auth.PermSettingsManage), diagH.GetDBHealth)
	protected.POST("/diagnostics/export", s.perm(auth.PermSettingsManage), diagH.StartExport)
	protected.GET("/diagnostics/export/:operationId/download", s.perm(auth.PermSettingsManage), diagH.DownloadExport)
	protected.POST("/diagnostics/submit-ai", s.perm(auth.PermSettingsManage), diagH.SubmitAI)
	protected.GET("/diagnostics/ai-results/:operationId", s.perm(auth.PermSettingsManage), diagH.GetAIResults)
	protected.POST("/diagnostics/apply-suggestions", s.perm(auth.PermSettingsManage), diagH.ApplySuggestions)

	// Operations v2 (UOS-06)
	protected.GET("/operations/timeline", s.perm(auth.PermLibraryView), opsV2H.GetOperationTimeline)
	protected.GET("/operations/events", s.perm(auth.PermLibraryView), opsV2H.OperationsSSE)
	protected.GET("/operations/v2/:id", s.perm(auth.PermLibraryView), opsV2H.GetOperationV2)
	protected.DELETE("/operations/v2/:id", s.perm(auth.PermSettingsManage), opsV2H.CancelOperationV2)
	protected.POST("/operations/v2", s.perm(auth.PermScanTrigger), opsV2H.TriggerOperationV2)
	protected.GET("/op-defs", s.perm(auth.PermLibraryView), opsV2H.ListOpDefs)
	protected.GET("/op-defs/:id", s.perm(auth.PermLibraryView), opsV2H.GetOpDef)

	// Operations domain (migrated from server_lifecycle.go). Paths + permission
	// guards copied verbatim. These share the /operations path prefix with the
	// operations_v2 routes above (timeline/events/v2/op-defs) and the survivors
	// that stay in server_lifecycle.go (active/recent/reconcile/itunes-path-*/
	// cleanup-version-groups/results/file-ops) — all distinct method+path pairs,
	// all using the identical `:id` param name, so Gin registers them cleanly.
	protected.GET("/operations", s.perm(auth.PermLibraryView), operationsH.ListOperations)
	protected.GET("/operations/stale", s.perm(auth.PermLibraryView), operationsH.ListStaleOperations)
	protected.POST("/operations/scan", s.perm(auth.PermScanTrigger), operationsH.StartScan)
	protected.POST("/operations/organize", s.perm(auth.PermScanTrigger), operationsH.StartOrganize)
	protected.POST("/operations/transcode", s.perm(auth.PermScanTrigger), operationsH.StartTranscode)
	protected.POST("/operations/optimize", s.perm(auth.PermScanTrigger), operationsH.StartOptimize)
	protected.GET("/operations/:id/status", s.perm(auth.PermLibraryView), operationsH.GetOperationStatus)
	protected.GET("/operations/:id/logs", s.perm(auth.PermLibraryView), operationsH.GetOperationLogs)
	protected.GET("/operations/:id/result", s.perm(auth.PermLibraryView), operationsH.GetOperationResult)
	protected.DELETE("/operations/:id", s.perm(auth.PermSettingsManage), operationsH.CancelOperation)
	protected.POST("/operations/clear-stale", s.perm(auth.PermSettingsManage), operationsH.ClearStaleOperations)
	protected.DELETE("/operations/history", s.perm(auth.PermSettingsManage), operationsH.DeleteOperationHistory)
	protected.POST("/operations/optimize-database", s.perm(auth.PermSettingsManage), operationsH.OptimizeDatabase)
	protected.POST("/operations/sweep-tombstones", s.perm(auth.PermSettingsManage), operationsH.SweepTombstones)
	protected.POST("/operations/set-internal-flag", s.perm(auth.PermSettingsManage), operationsH.SetInternalFlag)
	protected.GET("/operations/audit-files", s.perm(auth.PermSettingsManage), operationsH.AuditFileConsistency)
	protected.GET("/operations/:id/changes", s.perm(auth.PermLibraryView), operationsH.GetOperationChanges)
	protected.GET("/operations/:id/undo/preflight", s.perm(auth.PermLibraryView), operationsH.UndoPreflightHandler)
	protected.POST("/operations/:id/revert", s.perm(auth.PermLibraryOrganize), operationsH.RevertOperation)
	protected.GET("/tasks", s.perm(auth.PermSettingsManage), operationsH.ListTasks)
	protected.POST("/tasks/:name/run", s.perm(auth.PermSettingsManage), operationsH.RunTask)
	protected.PUT("/tasks/:name", s.perm(auth.PermSettingsManage), operationsH.UpdateTaskConfig)
	protected.POST("/maintenance-window/run", s.perm(auth.PermSettingsManage), operationsH.RunMaintenanceWindowNow)
	protected.GET("/maintenance-window/status", s.perm(auth.PermSettingsManage), operationsH.GetMaintenanceWindowStatus)
	protected.PUT("/maintenance-window/config", s.perm(auth.PermSettingsManage), operationsH.UpdateMaintenanceWindowConfig)

	// System domain (migrated from server_lifecycle.go). Paths + permission
	// guards copied verbatim. The public /health (x3) and /api/events routes stay
	// in setupRoutes — they are registered on s.router BEFORE the /api/* redirect
	// middleware, so re-registering them here would change their middleware
	// ordering; they delegate to systemH via closures instead.
	protected.GET("/policy/tags", s.perm(auth.PermLibraryView), systemH.HandlePolicyTags)
	protected.GET("/system/status", s.perm(auth.PermSettingsManage), systemH.GetSystemStatus)
	protected.GET("/system/announcements", s.perm(auth.PermSettingsManage), systemH.GetSystemAnnouncements)
	protected.GET("/system/storage", s.perm(auth.PermSettingsManage), systemH.GetSystemStorage)
	protected.GET("/system/logs", s.perm(auth.PermSettingsManage), systemH.GetSystemLogs)
	protected.GET("/system/activity-log", s.perm(auth.PermSettingsManage), systemH.GetSystemActivityLog)
	protected.POST("/system/reset", s.perm(auth.PermSettingsManage), systemH.ResetSystem)
	protected.POST("/system/factory-reset", s.perm(auth.PermSettingsManage), systemH.FactoryReset)
	protected.GET("/config", s.perm(auth.PermSettingsManage), systemH.GetConfig)
	protected.PUT("/config", s.perm(auth.PermSettingsManage), systemH.UpdateConfig)
	protected.GET("/dashboard", s.perm(auth.PermLibraryView), systemH.GetDashboard)
	protected.POST("/backup/create", s.perm(auth.PermSettingsManage), systemH.CreateBackup)
	protected.GET("/backup/list", s.perm(auth.PermSettingsManage), systemH.ListBackups)
	protected.POST("/backup/restore", s.perm(auth.PermSettingsManage), systemH.RestoreBackup)
	protected.DELETE("/backup/:filename", s.perm(auth.PermSettingsManage), systemH.DeleteBackup)
	protected.GET("/library/quick-queries", s.perm(auth.PermLibraryView), systemH.GetQuickQueries)
	protected.GET("/blocked-hashes", s.perm(auth.PermLibraryView), systemH.ListBlockedHashes)
	protected.POST("/blocked-hashes", s.perm(auth.PermLibraryEditMetadata), systemH.AddBlockedHash)
	protected.DELETE("/blocked-hashes/:hash", s.perm(auth.PermLibraryDelete), systemH.RemoveBlockedHash)
	protected.GET("/preferences/:key", s.perm(auth.PermLibraryView), systemH.GetUserPreference)
	protected.PUT("/preferences/:key", s.perm(auth.PermLibraryEditMetadata), systemH.SetUserPreference)
	protected.DELETE("/preferences/:key", s.perm(auth.PermLibraryDelete), systemH.DeleteUserPreference)

	// Plugins
	plugins := protected.Group("/plugins")
	{
		plugins.GET("", s.perm(auth.PermSettingsManage), pluginsH.ListPlugins)
		plugins.GET("/:id", s.perm(auth.PermSettingsManage), pluginsH.GetPlugin)
		plugins.POST("/:id/enable", s.perm(auth.PermSettingsManage), pluginsH.EnablePlugin)
		plugins.POST("/:id/disable", s.perm(auth.PermSettingsManage), pluginsH.DisablePlugin)
		plugins.GET("/:id/health", s.perm(auth.PermSettingsManage), pluginsH.PluginHealth)
		plugins.PUT("/:id/settings", s.perm(auth.PermSettingsManage), pluginsH.UpdatePluginSettings)
	}

	// Admin-only Phase 2 routes
	adminOnly := protected.Group("")
	adminOnly.Use(servermiddleware.RequireAdmin())
	{
		adminOnly.GET("/cache/stats/keys", cacheH.HandleCacheKeysIntrospection)
		adminOnly.POST("/admin/recompact-digests", activityH.RecompactDigests)
	}
}
