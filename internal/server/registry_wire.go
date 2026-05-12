// file: internal/server/registry_wire.go
// version: 1.0.0

package server

import (
	audiobookspkg "github.com/jdfalk/audiobook-organizer/internal/audiobooks"
	"github.com/jdfalk/audiobook-organizer/internal/batch"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/fileops"
	"github.com/jdfalk/audiobook-organizer/internal/importer"
	"github.com/jdfalk/audiobook-organizer/internal/metafetch"
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
func wireServerFromContainer(s *Server, c *serviceregistry.Container) {
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
}
