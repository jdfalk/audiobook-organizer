// file: internal/plugins/maintenance/register.go
// version: 1.0.0

// Service registry registration for the maintenance UOS plugin (W5).
//
// **Stub registration.** The maintenance plugin's constructor takes a
// `ServerDeps` struct populated with multiple typed references to
// *server.Server fields (scheduler, dedup engine, ai scan store,
// activity writer, openlibrary service, ...). That coupling makes the
// plugin unbuildable from the registry at present — every dep would
// need to be registered first and the ServerDeps struct populated by
// pulling them all from the container.
//
// For now this Build returns nil so the plugin is "present" in the
// container (consumers can TryGet) but inert. Inline NewServer
// construction continues to be the production path.
//
// SERVER-PLUGIN-REG W7 (or a follow-up sweep) needs to decouple
// ServerDeps from *Server and register its constituent services in the
// container before this plugin can build for real.

package maintenance

import (
	"github.com/jdfalk/audiobook-organizer/internal/serviceregistry"
)

func init() {
	serviceregistry.Register(serviceregistry.ServiceDef{
		Name:  "maintenanceplugin",
		Needs: []string{},
		Build: func(c *serviceregistry.Container) (any, error) {
			// Stub — see file header. Inline construction remains the
			// production path until ServerDeps is decoupled from *Server.
			return (*Plugin)(nil), nil
		},
	})
}
