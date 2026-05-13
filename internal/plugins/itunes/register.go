// file: internal/plugins/itunes/register.go
// version: 1.0.0

// Service registry registration for the iTunes UOS plugin (W5).
//
// **Stub registration.** Mirrors maintenance/register.go: the iTunes
// plugin's constructor needs `*itunesservice.Service`, which carries
// server-bound closures (OnBookCreated → server.fireDedupOnImport,
// OrganizerFactory referencing config.AppConfig, etc.). That coupling
// blocks clean container registration today; see internal/itunes/
// service/register.go for the parallel writebackbatcher discussion.
//
// Build returns nil so the plugin slot exists in the container.
// Production path stays inline in NewServer.
//
// W7 (or follow-up): decouple itunesservice.Service from server-bound
// closures so this plugin (and W3.1's writebackbatcher stub) can build
// for real.

package itunes

import (
	"github.com/jdfalk/audiobook-organizer/internal/serviceregistry"
)

func init() {
	serviceregistry.Register(serviceregistry.ServiceDef{
		Name:  "itunesplugin",
		Needs: []string{},
		Groups: []string{"plugins"},
		Build: func(c *serviceregistry.Container) (any, error) {
			// Stub — see file header.
			return (*Plugin)(nil), nil
		},
	})
}
