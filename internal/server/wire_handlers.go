// file: internal/server/wire_handlers.go
// version: 2.0.0
// guid: f7a8b9c0-d1e2-3456-7890-abcdef012345
// last-edited: 2026-06-02

package server

import (
	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/auth"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	dedupengine "github.com/jdfalk/audiobook-organizer/internal/dedup"
	"github.com/jdfalk/audiobook-organizer/internal/server/handlers"
	servermiddleware "github.com/jdfalk/audiobook-organizer/internal/server/middleware"
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
