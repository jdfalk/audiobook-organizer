// file: internal/operations/registry/register.go
// version: 1.0.0

package registry

import (
	"context"
	"log/slog"

	"github.com/falkcorp/audiobook-organizer/internal/database"
	"github.com/falkcorp/audiobook-organizer/internal/serviceregistry"
)

// RegistryWrapper wraps *Registry to adapt its Start(ctx) (void) method to
// the Starter interface's Start(ctx) error signature. The wrapper is registered
// in the service registry and consumers can Get[*Registry] from the container.
type RegistryWrapper struct {
	*Registry
}

// Start satisfies serviceregistry.Starter by converting Registry.Start's void
// return to an error return.
func (w *RegistryWrapper) Start(ctx context.Context) error {
	w.Registry.Start(ctx)
	return nil
}

// Stop satisfies serviceregistry.Stopper by delegating to Registry.Shutdown.
func (w *RegistryWrapper) Stop(ctx context.Context) error {
	return w.Registry.Shutdown(ctx)
}

func init() {
	serviceregistry.Register(serviceregistry.ServiceDef{
		Name:   "ophub",
		Needs:  []string{},
		Groups: []string{"scheduler"},
		Build: func(c *serviceregistry.Container) (any, error) {
			return NewEventHub(), nil
		},
	})

	serviceregistry.Register(serviceregistry.ServiceDef{
		Name:   "opregistry",
		Needs:  []string{"store", "ophub"},
		Groups: []string{"scheduler"},
		Build: func(c *serviceregistry.Container) (any, error) {
			store := serviceregistry.Get[database.OpsV2Store](c, "store")
			hub := serviceregistry.Get[*EventHub](c, "ophub")
			reg := New(store, slog.Default(), 8, hub)
			return &RegistryWrapper{Registry: reg}, nil
		},
	})
}
