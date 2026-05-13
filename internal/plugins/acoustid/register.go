// file: internal/plugins/acoustid/register.go
// version: 1.0.0

// Service registry registration for the acoustid UOS plugin (W5).
//
// Mirrors the dedup plugin's registration shape — needs the dedup engine
// + embedding store. Build returns nil when deps are unavailable.

package acoustid

import (
	"github.com/jdfalk/audiobook-organizer/internal/database"
	dedupengine "github.com/jdfalk/audiobook-organizer/internal/dedup"
	"github.com/jdfalk/audiobook-organizer/internal/serviceregistry"
)

func init() {
	serviceregistry.Register(serviceregistry.ServiceDef{
		Name:  "acoustidplugin",
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
