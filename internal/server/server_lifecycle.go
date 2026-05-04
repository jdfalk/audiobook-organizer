// file: internal/server/server_lifecycle.go
// version: 1.4.0
// guid: 2f98675b-61e1-45a0-94e9-e7fdeb8f273e
// last-edited: 2026-05-04

package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/auth"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	itunesservice "github.com/jdfalk/audiobook-organizer/internal/itunes/service"
	"github.com/jdfalk/audiobook-organizer/internal/logger"
	"github.com/jdfalk/audiobook-organizer/internal/maintenance"
	"github.com/jdfalk/audiobook-organizer/internal/metrics"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	"github.com/jdfalk/audiobook-organizer/internal/realtime"
	"github.com/jdfalk/audiobook-organizer/internal/search"
	servermiddleware "github.com/jdfalk/audiobook-organizer/internal/server/middleware"
	"github.com/jdfalk/audiobook-organizer/internal/scanner"
	"github.com/jdfalk/audiobook-organizer/internal/transcode"
	"github.com/jdfalk/audiobook-organizer/internal/watcher"
	ulid "github.com/oklog/ulid/v2"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/quic-go/quic-go/http3"
	"golang.org/x/net/http2"
	"github.com/jdfalk/audiobook-organizer/internal/httputil"
)

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
				var mappings []itunesservice.PathMapping
				for from, to := range params.PathMappings {
					mappings = append(mappings, itunesservice.PathMapping{From: from, To: to})
				}
				return s.itunesSvc.Importer.Execute(ctx, opID, itunesservice.ImportRequest{
					LibraryPath:      params.LibraryXMLPath,
					ImportMode:       params.ImportMode,
					PathMappings:     mappings,
					SkipDuplicates:   params.SkipDuplicates,
					FetchMetadata:    params.EnrichMetadata,
					PreserveLocation: !params.AutoOrganize,
				}, operations.LoggerFromReporter(progress))
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
				return s.scanService.PerformScanWithID(ctx, opID, &scanner.ScanRequest{
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
			isbnOpID := opID
			resumeFn = func(ctx context.Context, progress operations.ProgressReporter) error {
				return s.runIsbnEnrichment(ctx, progress, isbnOpID)
			}
		case "metadata-refresh":
			resumeFn = s.runMetadataRefreshScan
		case "itunes_path_reconcile":
			reconcileOpID := opID
			resumeFn = func(ctx context.Context, progress operations.ProgressReporter) error {
				return s.itunesSvc.Paths.Reconcile(ctx, reconcileOpID, progress)
			}
		case "itunes_path_repair":
			repairOpID := opID
			// Resume always defaults to dry-run for safety — operator
			// can re-trigger with apply=true once they confirm the report.
			resumeFn = func(ctx context.Context, progress operations.ProgressReporter) error {
				return s.itunesSvc.Repair.Repair(ctx, repairOpID, true, progress)
			}
		case "transcode", "diagnostics_export", "diagnostics_ai",
			"cleanup_activity_log", "purge_old_logs",
			"purge-deleted", "tombstone-cleanup",
			"author-dedup-scan", "author-split-scan", "series-prune", "series-normalize",
			"db-optimize", "cleanup-old-backups", "batch_poller",
			"itunes_sync",
			// reconcile_scan: a 271K-file hash sweep that ignores ctx, runs
			// nightly via the scheduler, and pins both queue workers for ~45min
			// when auto-resumed. Repeated quick deploys produced a queue jam
			// where new ops (AcoustID, embed, etc.) sat queued behind two
			// stuck reconcile_scans that the cancel API couldn't actually
			// kill. Letting the scheduler re-run it tomorrow is fine.
			"reconcile_scan":
			// These are not resumable — mark as failed silently
			_ = store.UpdateOperationError(opID, fmt.Sprintf("interrupted during %s, please retry", opType))
			_ = operations.ClearState(store, opID)
			continue
		default:
			// Try to resume as a maintenance job
			if j, jobErr := maintenance.Get(strings.TrimPrefix(opType, "maintenance:")); jobErr == nil && j.CanResume() {
				capturedOpID := opID
				capturedJob := j
				capturedStore := store
				resumeFn = func(ctx context.Context, progress operations.ProgressReporter) error {
					ctx = maintenance.WithOperationID(ctx, capturedOpID)
					adapter := &progressAdapter{ops: progress}
					return capturedJob.Run(ctx, capturedStore, adapter, false)
				}
			} else {
				_ = store.UpdateOperationError(opID, "interrupted, cannot resume")
				_ = operations.ClearState(store, opID)
				continue
			}
		}

		if err := oq.EnqueueResume(opID, opType, operations.PriorityNormal, resumeFn); err != nil {
			log.Printf("[WARN] Failed to re-enqueue operation %s: %v", opID, err)
		}
	}
}

func (s *Server) Start(cfg ServerConfig) error {
	if err := s.itunesSvc.Start(s.bgCtx); err != nil {
		return fmt.Errorf("itunes service start: %w", err)
	}

	// Pre-warm the facets cache in the background so the first Library page
	// load doesn't block on a full PebbleDB scan.
	go s.warmFacetsCache()

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

	// Initialize the one-time bootstrap token for emergency admin access.
	if dbPath := config.AppConfig.DatabasePath; dbPath != "" {
		dataDir := filepath.Dir(dbPath)
		if err := InitBootstrapToken(s.Store(), dataDir); err != nil {
			log.Printf("[BOOTSTRAP] Failed to init bootstrap token: %v", err)
		}
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

	s.bgWG.Add(1)
	go func() {
		defer s.bgWG.Done()
		s.backfillAcoustIDs()
	}()

	// Strip shwm/©mvi/©mvn atoms from audiobook files (one-time). These
	// classical-music atoms crash Apple Devices for Windows at sync.
	s.bgWG.Add(1)
	go func() {
		defer s.bgWG.Done()
		s.stripMovementAtoms()
	}()

	// Re-mux M4B/M4A files with malformed atom structures so taglib,
	// AtomicParsley, and Apple Devices can read them (one-time).
	s.bgWG.Add(1)
	go func() {
		defer s.bgWG.Done()
		s.remuxMalformedM4BFiles()
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

	// One-time startup jobs: transcode malformed M4B files, then quarantine any
	// that remained permanently unreadable. Run sequentially in a bgWG goroutine
	// so shutdown waits for them and they don't race against the HTTP server.
	s.bgWG.Add(1)
	go func() {
		defer s.bgWG.Done()
		s.transcodeMalformedM4BFiles()
		s.quarantineKnownBadFiles()
	}()

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

	// Persist cache observability snapshots to SQLite every 5 minutes so
	// hit/miss trends survive restarts. PebbleDB-backed deployments skip
	// persistence inside runCacheStatsSnapshotter.
	backgroundWG.Add(1)
	go func() {
		defer backgroundWG.Done()
		s.runCacheStatsSnapshotter(shutdown)
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
							scanReq := &scanner.ScanRequest{FolderPath: &scanPath}
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

	// Shut down the iTunes service (no-op in PR 1 since NewDisabled is
	// always used; PR 2 onward may have live sub-components to flush).
	if s.itunesSvc != nil {
		if err := s.itunesSvc.Shutdown(30 * time.Second); err != nil {
			log.Printf("[WARN] itunes service shutdown: %v", err)
		}
	}

	// Shut down all plugins before closing stores
	if s.pluginRegistry != nil {
		log.Println("[INFO] Shutting down plugins...")
		s.pluginRegistry.ShutdownAll(ctx)
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

func (s *Server) perm(p auth.Permission) gin.HandlerFunc {
	if !config.AppConfig.EnableAuth {
		return func(c *gin.Context) { c.Next() }
	}
	return servermiddleware.RequirePermission(s.Store(), p)
}

func (s *Server) itunesSvcGuard(fn gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.itunesSvc == nil || !s.itunesSvc.Enabled() {
			httputil.RespondWithServiceUnavailable(c, "iTunes service is disabled")
			return
		}
		fn(c)
	}
}

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
	if rpm := config.AppConfig.APIRateLimitPerMinute; rpm > 0 {
		burst := rpm / 5
		if burst < 10 {
			burst = 10
		}
		apiRateLimiter = servermiddleware.NewIPRateLimiter(rpm, burst).Middleware()
	}
	bodyLimitMiddleware := servermiddleware.MaxRequestBodySize(jsonLimitBytes, uploadLimitBytes)
	authMiddleware := gin.HandlerFunc(func(c *gin.Context) {
		c.Next()
	})
	if config.AppConfig.EnableAuth {
		authMiddleware = servermiddleware.RequireAuth(s.Store())
	} else {
		log.Printf("[WARN] authentication is disabled (enable_auth=false) — do not expose this server to untrusted networks")
	}
	if !config.AppConfig.EnableRateLimit {
		log.Printf("[WARN] rate limiting is disabled (enable_rate_limit=false) — the API is vulnerable to abuse. Set enable_rate_limit: true in config.yaml for production deployments")
	}

	// API routes (auth + rate limits + request-size limits)
	api := s.router.Group("/api/v1")
	api.Use(apiRateLimiter, bodyLimitMiddleware)
	{
		authGroup := api.Group("/auth")
		{
			authGroup.GET("/status", s.getAuthStatus)
			authGroup.POST("/setup", s.setupInitialAdmin)
			authGroup.POST("/login", s.login)
			authGroup.POST("/accept-invite", s.handleAcceptInvite)
			authGroup.POST("/bootstrap", s.handleBootstrap)
		}

		authProtected := authGroup.Group("")
		authProtected.Use(authMiddleware)
		{
			authProtected.GET("/me", s.me)
			authProtected.PATCH("/me", s.updateMe)
			authProtected.POST("/logout", s.logout)
			authProtected.GET("/sessions", s.listMySessions)
			authProtected.DELETE("/sessions/:id", s.revokeMySession)
			authProtected.PUT("/me/password", s.changePassword)

			// API key management
			authProtected.POST("/api-keys", s.createAPIKey)
			authProtected.GET("/api-keys", s.listAPIKeys)
			authProtected.GET("/api-keys/:id", s.getAPIKey)
			authProtected.PATCH("/api-keys/:id", s.updateAPIKeyStatus)
			authProtected.DELETE("/api-keys/:id", s.revokeAPIKey)
		}

		// Public cache stats endpoint (no auth required)
		api.GET("/cache/stats", s.handleCacheStats)
		api.GET("/cache/stats/history", s.handleCacheStatsHistory)

		protected := api.Group("")
		protected.Use(authMiddleware)
		{
			// Audiobook routes
			protected.GET("/audiobooks", s.perm(auth.PermLibraryView), s.listAudiobooks)
			// /audiobooks/search removed — use GET /audiobooks?search= instead
			protected.GET("/audiobooks/count", s.perm(auth.PermLibraryView), s.countAudiobooks)
			protected.GET("/audiobooks/facets", s.perm(auth.PermLibraryView), s.audiobookFacets)
			protected.GET("/audiobooks/duplicates", s.perm(auth.PermLibraryView), s.listDuplicateAudiobooks)
			protected.GET("/audiobooks/duplicates/scan-results", s.perm(auth.PermLibraryView), s.listBookDuplicateScanResults)
			protected.POST("/audiobooks/duplicates/scan", s.perm(auth.PermLibraryEditMetadata), s.scanBookDuplicates)
			protected.POST("/audiobooks/duplicates/merge", s.perm(auth.PermLibraryEditMetadata), s.mergeBookDuplicatesAsVersions)
			protected.POST("/audiobooks/duplicates/dismiss", s.perm(auth.PermLibraryEditMetadata), s.dismissBookDuplicateGroup)
			protected.GET("/audiobooks/quarantined", s.perm(auth.PermLibraryView), s.listQuarantinedBooks)
			protected.GET("/audiobooks/soft-deleted", s.perm(auth.PermLibraryView), s.listSoftDeletedAudiobooks)
			protected.DELETE("/audiobooks/purge-soft-deleted", s.perm(auth.PermLibraryDelete), s.purgeSoftDeletedAudiobooks)
			protected.POST("/audiobooks/:id/restore", s.perm(auth.PermLibraryOrganize), s.restoreAudiobook)
			protected.POST("/audiobooks/:id/quarantine", s.perm(auth.PermSettingsManage), s.quarantineBook)
			protected.DELETE("/audiobooks/:id/quarantine", s.perm(auth.PermSettingsManage), s.unquarantineBook)
			protected.GET("/audiobooks/:id", s.perm(auth.PermLibraryView), s.getAudiobook)
			protected.GET("/audiobooks/:id/tags", s.perm(auth.PermLibraryView), s.getAudiobookTags)
			protected.PUT("/audiobooks/:id", s.perm(auth.PermLibraryEditMetadata), s.updateAudiobook)
			protected.PATCH("/audiobooks/:id/rating", s.perm(auth.PermLibraryEditMetadata), s.handleUpdateBookRating)
			protected.DELETE("/audiobooks/:id", s.perm(auth.PermLibraryDelete), s.deleteAudiobook)
			protected.GET("/audiobooks/:id/cover", s.perm(auth.PermLibraryView), s.serveAudiobookCover)
			protected.GET("/audiobooks/:id/sample", s.perm(auth.PermLibraryView), s.handleAudioSample)
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
			protected.GET("/series/normalize/preview", s.perm(auth.PermLibraryView), s.seriesNormalizePreview)
			protected.POST("/series/normalize", s.perm(auth.PermLibraryEditMetadata), s.seriesNormalize)
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
			protected.POST("/dedup/scan-acoustid", s.perm(auth.PermScanTrigger), s.triggerDedupAcoustID)
			protected.POST("/dedup/scan-book-signature", s.perm(auth.PermScanTrigger), s.triggerBookSignatureScan)
			protected.POST("/dedup/fingerprint-rescan", s.perm(auth.PermScanTrigger), s.triggerFingerprintRescan)
			protected.POST("/dedup/refresh", s.perm(auth.PermScanTrigger), s.triggerDedupRefresh)
			protected.POST("/dedup/embed", s.perm(auth.PermScanTrigger), s.triggerEmbedScan)

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
			protected.POST("/operations/set-internal-flag", s.perm(auth.PermSettingsManage), s.setInternalFlag)
			protected.GET("/operations/audit-files", s.perm(auth.PermSettingsManage), s.auditFileConsistency)
			protected.GET("/operations/reconcile/preview", s.perm(auth.PermLibraryView), s.reconcilePreview)
			protected.POST("/operations/reconcile", s.perm(auth.PermScanTrigger), s.startReconcile)
			protected.POST("/operations/reconcile/scan", s.perm(auth.PermScanTrigger), s.startReconcileScan)
			protected.GET("/operations/reconcile/scan/latest", s.perm(auth.PermLibraryView), s.latestReconcileScan)
			protected.POST("/operations/itunes-path-reconcile", s.perm(auth.PermScanTrigger), s.itunesSvcGuard(func(c *gin.Context) { s.itunesSvc.Paths.Start(c) }))
			protected.POST("/operations/itunes-path-repair", s.perm(auth.PermScanTrigger), s.itunesSvcGuard(func(c *gin.Context) { s.itunesSvc.Repair.Start(c) }))
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
				// REMOVED in v5: cleanup-orphans was a bulk-remove
				// endpoint that inferred "what should not be in iTunes"
				// from the DB. With a stale or partially-cleared DB
				// (or with manually-managed iTunes content), it would
				// wipe legitimate tracks. Targeted-only removes (one
				// PID per explicit user delete via the per-book delete
				// path) are the only safe pattern. Any future bulk
				// reconciliation must be opt-in, dry-run-by-default,
				// preview-required, and reviewed item-by-item.
				itunesGroup.GET("/library-stats", s.perm(auth.PermLibraryView), s.handleITunesLibraryStats)
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
				itunesGroup.GET("/library/download", s.perm(auth.PermIntegrationsManage), s.itunesSvcGuard(func(c *gin.Context) { s.itunesSvc.Transfer.HandleDownload(c) }))
				itunesGroup.POST("/library/upload", s.perm(auth.PermIntegrationsManage), s.itunesSvcGuard(func(c *gin.Context) { s.itunesSvc.Transfer.HandleUpload(c) }))
				itunesGroup.GET("/library/backups", s.perm(auth.PermIntegrationsManage), s.itunesSvcGuard(func(c *gin.Context) { s.itunesSvc.Transfer.HandleBackupList(c) }))
				itunesGroup.POST("/library/restore", s.perm(auth.PermIntegrationsManage), s.itunesSvcGuard(func(c *gin.Context) { s.itunesSvc.Transfer.HandleRestore(c) }))
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
			protected.GET("/maintenance-window/status", s.perm(auth.PermSettingsManage), s.getMaintenanceWindowStatus)
			protected.PUT("/maintenance-window/config", s.perm(auth.PermSettingsManage), s.updateMaintenanceWindowConfig)
			// Result-getter GETs (not job triggers — these poll async results)
			protected.GET("/maintenance/scan-composer-tags/:id", s.perm(auth.PermSettingsManage), s.handleGetComposerScanResults)
			protected.GET("/maintenance/repair-missing-files/:id", s.perm(auth.PermSettingsManage), s.handleGetMissingFileRepairResults)
			// Hash stats endpoints
			protected.GET("/maintenance/book-file-hash-stats", s.perm(auth.PermSettingsManage), s.handleGetBookFileHashStats)
			protected.GET("/maintenance/book-metadata-hash-stats", s.perm(auth.PermSettingsManage), s.handleGetBookMetadataHashStats)
			// Unified maintenance job dispatcher
			protected.GET("/maintenance/jobs", s.perm(auth.PermSettingsManage), s.listMaintenanceJobs)
			protected.POST("/maintenance/jobs/:job_id", s.runMaintenanceJob)

			// Admin-only destructive endpoints
			adminOnly := protected.Group("")
			adminOnly.Use(servermiddleware.RequireAdmin())
			{
				adminOnly.POST("/maintenance/wipe", s.handleWipe)
				adminOnly.GET("/cache/stats/keys", s.handleCacheKeysIntrospection)
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
			protected.GET("/audiobooks/:id/metadata-rejections", s.perm(auth.PermLibraryView), s.handleGetMetadataRejections)
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
			protected.GET("/diagnostics/db-health", s.perm(auth.PermSettingsManage), s.getDBHealth)
			protected.POST("/diagnostics/export", s.perm(auth.PermSettingsManage), s.startDiagnosticsExport)
			protected.GET("/diagnostics/export/:operationId/download", s.perm(auth.PermSettingsManage), s.downloadDiagnosticsExport)
			protected.POST("/diagnostics/submit-ai", s.perm(auth.PermSettingsManage), s.submitDiagnosticsAI)
			protected.GET("/diagnostics/ai-results/:operationId", s.perm(auth.PermSettingsManage), s.getDiagnosticsAIResults)
			protected.POST("/diagnostics/apply-suggestions", s.perm(auth.PermSettingsManage), s.applyDiagnosticsSuggestions)

			// AI Jobs observability routes
			protected.GET("/ai-jobs", s.perm(auth.PermSettingsManage), s.handleListAIJobs)

			// Bench routes (only available with -tags bench)
			s.setupUserTagRoutes(protected)
			s.registerReadingRoutes(protected)
			s.registerPlaylistRoutes(protected)
			s.registerUserAdminRoutes(protected)
			s.registerVersionLifecycleRoutes(protected)
			s.registerEntityTagRoutes(protected)
			s.registerDelugeRoutes(protected)
			s.setupBenchRoutes(protected)

			// Plugin management API
			plugins := protected.Group("/plugins")
			{
				plugins.GET("", s.perm(auth.PermSettingsManage), s.listPlugins)
				plugins.GET("/:id", s.perm(auth.PermSettingsManage), s.getPlugin)
				plugins.POST("/:id/enable", s.perm(auth.PermSettingsManage), s.enablePlugin)
				plugins.POST("/:id/disable", s.perm(auth.PermSettingsManage), s.disablePlugin)
				plugins.GET("/:id/health", s.perm(auth.PermSettingsManage), s.pluginHealth)
				plugins.PUT("/:id/settings", s.perm(auth.PermSettingsManage), s.updatePluginSettings)
			}
		}
	}

	// Serve static files (React frontend)
	// Implementation is in static_embed.go or static_nonembed.go depending on build tags
	s.setupStaticFiles()
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

func GetDefaultServerConfig() ServerConfig {
	return ServerConfig{
		Port:         "8484",
		Host:         "localhost",
		ReadTimeout:  15 * time.Second,  // Allow slow clients without stalling forever
		WriteTimeout: 0,                 // Disable write timeout so SSE streams stay open
		IdleTimeout:  120 * time.Second, // 2 minute idle timeout
	}
}
