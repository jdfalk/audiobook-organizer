// file: internal/plugins/acoustid/register.go
// version: 1.1.1

// Service registry registration for the acoustid UOS plugin (W5/W7).
//
// Mirrors the dedup plugin's registration shape — needs the dedup engine
// + embedding store. Build returns nil when deps are unavailable.
// PostInit self-registers the plugin's op-defs.

package acoustid

import (
	"context"
	"log/slog"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	dedupengine "github.com/jdfalk/audiobook-organizer/internal/dedup"
	opsregistry "github.com/jdfalk/audiobook-organizer/internal/operations/registry"
	"github.com/jdfalk/audiobook-organizer/internal/serviceregistry"
)

func init() {
	serviceregistry.Register(serviceregistry.ServiceDef{
		Name:   "acoustidplugin",
		Needs:  []string{"store", "dedup", "embeddingstore"},
		Groups: []string{"plugins"},
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

func (p *Plugin) PostInit(ctx context.Context, c *serviceregistry.Container) error {
	if p == nil {
		return nil
	}
	wrapper, ok := serviceregistry.TryGet[*opsregistry.RegistryWrapper](c, "opregistry")
	if !ok || wrapper == nil {
		slog.Warn("PostInit: opregistry not available, skipping op-def registration")
		return nil
	}
	return p.Register(wrapper.Registry)
}
