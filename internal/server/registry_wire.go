// file: internal/server/registry_wire.go
// version: 1.4.0

package server

import (
	"log"
	"path/filepath"

	"github.com/jdfalk/audiobook-organizer/internal/activity"
	"github.com/jdfalk/audiobook-organizer/internal/ai"
	audiobookspkg "github.com/jdfalk/audiobook-organizer/internal/audiobooks"
	"github.com/jdfalk/audiobook-organizer/internal/batch"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/dedup"
	"github.com/jdfalk/audiobook-organizer/internal/fileops"
	"github.com/jdfalk/audiobook-organizer/internal/importer"
	itunesservice "github.com/jdfalk/audiobook-organizer/internal/itunes/service"
	"github.com/jdfalk/audiobook-organizer/internal/merge"
	"github.com/jdfalk/audiobook-organizer/internal/metafetch"
	opsregistry "github.com/jdfalk/audiobook-organizer/internal/operations/registry"
	"github.com/jdfalk/audiobook-organizer/internal/organizer"
	"github.com/jdfalk/audiobook-organizer/internal/plugin"
	"github.com/jdfalk/audiobook-organizer/internal/quarantine"
	"github.com/jdfalk/audiobook-organizer/internal/scanner"
	"github.com/jdfalk/audiobook-organizer/internal/serviceregistry"
	"github.com/jdfalk/audiobook-organizer/internal/sysinfo"
	"github.com/jdfalk/audiobook-organizer/internal/work"
)

// init registers services that can't live in their domain packages due
// to import cycles or because they need package-private symbols from
// internal/server.
//
//   - `system` — needs appVersion + calculateLibrarySizes from this pkg.
//   - `embeddingstore`, `chromemstore`, `aijobsstore` — live in
//     internal/database which can't import internal/config (cycle).
//   - `dedup` (the engine) — needs *config.Config to read thresholds;
//     internal/dedup doesn't already import internal/config, so registering
//     here avoids forcing a new dependency on that pkg.
func init() {
	serviceregistry.Register(serviceregistry.ServiceDef{
		Name:   "system",
		Needs:  []string{"store"},
		Groups: []string{"core"},
		Build: func(c *serviceregistry.Container) (any, error) {
			store := serviceregistry.Get[database.Store](c, "store")
			return sysinfo.NewSystemService(store, appVersion, calculateLibrarySizes), nil
		},
	})

	// embeddingstore — Pebble-backed key namespace for dedup embeddings.
	// Returns nil if the underlying store isn't *PebbleStore (e.g. tests).
	serviceregistry.Register(serviceregistry.ServiceDef{
		Name:   "embeddingstore",
		Needs:  []string{"store"},
		Groups: []string{"ai"},
		Build: func(c *serviceregistry.Container) (any, error) {
			store := serviceregistry.Get[database.Store](c, "store")
			ps, ok := store.(*database.PebbleStore)
			if !ok {
				return (*database.EmbeddingStore)(nil), nil
			}
			return database.NewEmbeddingStore(ps.DB()), nil
		},
	})

	// chromemstore — chromem-go ANN vector store for dedup Layer 2.
	// Optional; failure logs a warning + returns nil so dedup falls back
	// to the Pebble linear scan.
	serviceregistry.Register(serviceregistry.ServiceDef{
		Name:   "chromemstore",
		Needs:  []string{"config"},
		Groups: []string{"ai"},
		Build: func(c *serviceregistry.Container) (any, error) {
			cfg := serviceregistry.Get[*config.Config](c, "config")
			if cfg.DatabasePath == "" {
				return (*database.ChromemEmbeddingStore)(nil), nil
			}
			dir := filepath.Dir(cfg.DatabasePath)
			store, err := database.NewChromemEmbeddingStore(dir, 3072)
			if err != nil {
				return (*database.ChromemEmbeddingStore)(nil), nil
			}
			return store, nil
		},
	})

	// aijobsstore — interface assertion on the main store.
	serviceregistry.Register(serviceregistry.ServiceDef{
		Name:   "aijobsstore",
		Needs:  []string{"store"},
		Groups: []string{"ai"},
		Build: func(c *serviceregistry.Container) (any, error) {
			store := serviceregistry.Get[database.Store](c, "store")
			if s, ok := store.(database.AIJobsStore); ok {
				return s, nil
			}
			return database.AIJobsStore(nil), nil
		},
	})

	// dedup — the duplicate detection engine.
	// Returns nil if any required dep is missing (no API key, no embed
	// client, etc.) — matches the existing inline conditional construction
	// in NewServer.
	serviceregistry.Register(serviceregistry.ServiceDef{
		Name:   "dedup",
		Needs:  []string{"store", "config", "embeddingstore", "embedclient", "llmparser", "merge"},
		Groups: []string{"ai"},
		Build: func(c *serviceregistry.Container) (any, error) {
			cfg := serviceregistry.Get[*config.Config](c, "config")
			if cfg.OpenAIAPIKey == "" || !cfg.EmbeddingEnabled {
				return (*dedup.Engine)(nil), nil
			}
			embStore, _ := serviceregistry.TryGet[*database.EmbeddingStore](c, "embeddingstore")
			embClient, _ := serviceregistry.TryGet[*ai.EmbeddingClient](c, "embedclient")
			llmParser, _ := serviceregistry.TryGet[*ai.OpenAIParser](c, "llmparser")
			if embStore == nil || embClient == nil || llmParser == nil {
				return (*dedup.Engine)(nil), nil
			}
			store := serviceregistry.Get[database.Store](c, "store")
			mergeSvc := serviceregistry.Get[*merge.Service](c, "merge")
			engine := dedup.NewEngine(embStore, store, embClient, llmParser, mergeSvc)
			engine.BookHighThreshold = cfg.DedupBookHighThreshold
			engine.BookLowThreshold = cfg.DedupBookLowThreshold
			engine.AuthorHighThreshold = cfg.DedupAuthorHighThreshold
			engine.AuthorLowThreshold = cfg.DedupAuthorLowThreshold
			engine.AutoMergeEnabled = cfg.DedupAutoMergeEnabled
			return engine, nil
		},
	})

	// itunes — the iTunes integration service. Registered here (rather
	// than in internal/itunes/service/register.go) because the
	// OrganizerFactory closure needs internal/organizer + internal/config,
	// and itunesservice deliberately avoids importing internal/organizer
	// (see internal/itunes/service/types.go BookOrganizer comment).
	//
	// Construction never returns an error in practice: itunesservice.New
	// returns NewDisabled() when cfg.Enabled is false. The "Enabled: true"
	// flag here mirrors the pre-container inline construction in NewServer
	// — the per-feature toggles (AutoWriteBack, ITLWriteBackEnabled) come
	// from AppConfig.
	serviceregistry.Register(serviceregistry.ServiceDef{
		Name:   "itunes",
		Needs:  []string{"store", "config", "eventbus", "metafetch"},
		Groups: []string{"core"},
		Build: func(c *serviceregistry.Container) (any, error) {
			store := serviceregistry.Get[database.Store](c, "store")
			cfg := serviceregistry.Get[*config.Config](c, "config")
			bus := serviceregistry.Get[*plugin.EventBus](c, "eventbus")
			mf := serviceregistry.Get[*metafetch.Service](c, "metafetch")
			svc, err := itunesservice.New(itunesservice.Deps{
				Store: store,
				Config: itunesservice.Config{
					Enabled:             true,
					LibraryReadPath:     cfg.ITunesLibraryReadPath,
					LibraryWritePath:    cfg.ITunesLibraryWritePath,
					AutoWriteBack:       cfg.ITunesAutoWriteBack,
					ITLWriteBackEnabled: cfg.ITLWriteBackEnabled,
				},
				AudiobookRoot: cfg.RootDir,
				ReportDir:     filepath.Join(cfg.RootDir, "reports"),
				EventBus:      bus,
				Metafetch:     mf,
				OrganizerFactory: func() itunesservice.BookOrganizer {
					return organizer.NewOrganizer(cfg)
				},
			})
			if err != nil {
				log.Printf("[WARN] iTunes service construction failed, falling back to disabled: %v", err)
				return itunesservice.NewDisabled(), nil
			}
			return svc, nil
		},
	})
}

// wireServerFromContainer populates the typed service fields on *Server
// from the built container. Called once during NewServer after
// Container.Build() and Container.PostInit() succeed. Adding a future
// service is one new line here + one new register.go in the domain pkg.
//
// W2 services use TryGet because "activity" / "activitystore" are only
// Included when config.DatabasePath is set (the NutsDB sidecar can't open
// without a path). All other W1+W2 services are unconditional and Get
// could safely be used — TryGet is used consistently here to keep the
// wire-up uniform and tolerant of further phased Include() decisions.
func wireServerFromContainer(s *Server, c *serviceregistry.Container) {
	// W1 services (unconditional)
	s.audiobookService = serviceregistry.Get[*audiobookspkg.AudiobookService](c, "audiobook")
	s.batchService = serviceregistry.Get[*batch.BatchService](c, "batch")
	s.workService = serviceregistry.Get[*work.WorkService](c, "work")
	s.filesystemService = serviceregistry.Get[*fileops.FilesystemService](c, "filesystem")
	s.importPathService = serviceregistry.Get[*importer.ImportPathService](c, "importpath")
	s.scanService = serviceregistry.Get[*scanner.ScanService](c, "scan")
	s.dashboardService = serviceregistry.Get[*sysinfo.DashboardService](c, "dashboard")
	s.systemService = serviceregistry.Get[*sysinfo.SystemService](c, "system")
	s.configUpdateService = serviceregistry.Get[*config.UpdateService](c, "configupdate")
	s.metadataStateService = serviceregistry.Get[*metafetch.MetadataStateService](c, "metadatastate")

	// W2 services
	s.metadataFetchService = serviceregistry.Get[*metafetch.Service](c, "metafetch")
	s.mergeService = serviceregistry.Get[*merge.Service](c, "merge")
	s.organizeService = serviceregistry.Get[*OrganizeService](c, "organize")
	s.quarantineSvc = serviceregistry.Get[*quarantine.QuarantineService](c, "quarantine")
	s.eventBus = serviceregistry.Get[*plugin.EventBus](c, "eventbus")
	// activity is conditional on config.DatabasePath — pull via TryGet so
	// tests that don't configure a DB path still build.
	if svc, ok := serviceregistry.TryGet[*activity.Service](c, "activity"); ok {
		s.activityService = svc
	}

	// W3 services
	// batchpoller is conditional on OpenAI config — pull via TryGet.
	if poller, ok := serviceregistry.TryGet[*BatchPoller](c, "batchpoller"); ok {
		s.batchPoller = poller
	}
	// opRegistry — Get'd via the RegistryWrapper that exposes Start/Stop;
	// callers use the embedded *opsregistry.Registry. Always present.
	if wrapper, ok := serviceregistry.TryGet[*opsregistry.RegistryWrapper](c, "opregistry"); ok && wrapper != nil {
		s.opRegistry = wrapper.Registry
	}
	if hub, ok := serviceregistry.TryGet[*opsregistry.EventHub](c, "ophub"); ok {
		s.opHub = hub
	}

	// W4 services — embedding/AI cluster.
	if embStore, ok := serviceregistry.TryGet[*database.EmbeddingStore](c, "embeddingstore"); ok {
		s.embeddingStore = embStore
	}
	if engine, ok := serviceregistry.TryGet[*dedup.Engine](c, "dedup"); ok {
		s.dedupEngine = engine
	}

	// itunesservice.Service — container-built since PLUGIN-DECOUPLE-CLOSURES
	// (May 13, 2026). Replaces the prior inline itunesservice.New(...) call
	// in NewServer. Always present (Build returns NewDisabled() on error).
	s.itunesSvc = serviceregistry.Get[*itunesservice.Service](c, "itunes")
}
