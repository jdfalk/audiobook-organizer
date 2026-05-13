// file: internal/server/registry_wire.go
// version: 1.2.0

package server

import (
	"github.com/jdfalk/audiobook-organizer/internal/activity"
	audiobookspkg "github.com/jdfalk/audiobook-organizer/internal/audiobooks"
	"github.com/jdfalk/audiobook-organizer/internal/batch"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/fileops"
	"github.com/jdfalk/audiobook-organizer/internal/importer"
	"github.com/jdfalk/audiobook-organizer/internal/merge"
	"github.com/jdfalk/audiobook-organizer/internal/metafetch"
	"github.com/jdfalk/audiobook-organizer/internal/plugin"
	"github.com/jdfalk/audiobook-organizer/internal/quarantine"
	"github.com/jdfalk/audiobook-organizer/internal/scanner"
	"github.com/jdfalk/audiobook-organizer/internal/serviceregistry"
	"github.com/jdfalk/audiobook-organizer/internal/sysinfo"
	"github.com/jdfalk/audiobook-organizer/internal/work"
)

// init registers the `system` service. It lives here (not in
// internal/sysinfo) because NewSystemService needs appVersion (a
// package-level var in this package) and calculateLibrarySizes (a
// function in this package). Both stay where they are; the registry
// closure just captures them.
func init() {
	serviceregistry.Register(serviceregistry.ServiceDef{
		Name:  "system",
		Needs: []string{"store"},
		Build: func(c *serviceregistry.Container) (any, error) {
			store := serviceregistry.Get[database.Store](c, "store")
			return sysinfo.NewSystemService(store, appVersion, calculateLibrarySizes), nil
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
}
