// file: internal/plugins/dedup/register.go
// version: 1.0.0

// Service registry registration for the dedup UOS plugin (W5).
//
// Build returns the constructed *Plugin when all required services are
// available, or nil when any dep is unavailable (no API key → no dedup
// engine → no plugin). Inline NewServer construction continues to be
// the production path that actually calls Plugin.Register(opRegistry);
// W7 cleanup migrates that registration to PostInit.

package dedup

import (
	"github.com/jdfalk/audiobook-organizer/internal/database"
	dedupengine "github.com/jdfalk/audiobook-organizer/internal/dedup"
	"github.com/jdfalk/audiobook-organizer/internal/serviceregistry"
)

func init() {
	serviceregistry.Register(serviceregistry.ServiceDef{
		Name:  "dedupplugin",
		Needs: []string{"store", "dedup", "embeddingstore"},
		Build: func(c *serviceregistry.Container) (any, error) {
			engine, _ := serviceregistry.TryGet[*dedupengine.Engine](c, "dedup")
			embStore, _ := serviceregistry.TryGet[*database.EmbeddingStore](c, "embeddingstore")
			if engine == nil || embStore == nil {
				return (*Plugin)(nil), nil
			}
			store := serviceregistry.Get[database.Store](c, "store")
			return New(engine, store, embStore), nil
		},
	})
}
