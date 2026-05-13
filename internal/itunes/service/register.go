// file: internal/itunes/service/register.go
// version: 2.0.0
// guid: f1e2d3c4-b5a6-7890-1a2b-3c4d5e6f7a8b
//
// Registers the "writebackbatcher" service with the global serviceregistry.
//
// As of PROMOTE-STUB-REGISTRATIONS (May 13, 2026) this is no longer a stub:
// itunesservice.Service is container-built (see internal/server/registry_wire.go
// "itunes" ServiceDef), so Build can pull the parent service and return an
// adapter wrapping its real Batcher.
//
// Behavior on test paths (cfg.Enabled false → itunesservice.NewDisabled):
// svc.Batcher is nil. The adapter still wraps nil-safely (Stop is a no-op),
// matching the pre-promotion behavior where callers had to nil-check.

package itunesservice

import (
	"context"

	"github.com/jdfalk/audiobook-organizer/internal/serviceregistry"
)

// batcherAdapter wraps *WriteBackBatcher so it satisfies the
// serviceregistry.Starter and serviceregistry.Stopper interfaces.
//
// WriteBackBatcher.Stop() has signature Stop() (no ctx, no error). The
// registry Stopper interface requires Stop(ctx context.Context) error.
// Similarly there is no Start method on the batcher (it is stateless until
// the first Enqueue call), so Start is a no-op here.
type batcherAdapter struct {
	b *WriteBackBatcher
}

// Batcher returns the underlying *WriteBackBatcher. May be nil when the
// parent itunesservice was constructed via NewDisabled (test paths).
func (a *batcherAdapter) Batcher() *WriteBackBatcher {
	if a == nil {
		return nil
	}
	return a.b
}

// Start implements serviceregistry.Starter. WriteBackBatcher has no explicit
// start: it begins processing on the first Enqueue. This is a no-op.
func (a *batcherAdapter) Start(_ context.Context) error {
	return nil
}

// Stop implements serviceregistry.Stopper. Delegates to WriteBackBatcher.Stop()
// which flushes pending writes before returning. Nil-safe.
func (a *batcherAdapter) Stop(_ context.Context) error {
	if a != nil && a.b != nil {
		a.b.Stop()
	}
	return nil
}

func init() {
	// "writebackbatcher" — adapter around itunesservice.Service.Batcher.
	// Depends on "itunes" so Build order is well-defined regardless of
	// group ordering. The adapter's Stop hooks into Container.Stop()
	// once SERVER-LIFECYCLE-FLIP wires it.
	serviceregistry.Register(serviceregistry.ServiceDef{
		Name:   "writebackbatcher",
		Needs:  []string{"itunes"},
		Groups: []string{"scheduler"},
		Build: func(c *serviceregistry.Container) (any, error) {
			svc := serviceregistry.Get[*Service](c, "itunes")
			if svc == nil {
				return &batcherAdapter{}, nil
			}
			return &batcherAdapter{b: svc.Batcher}, nil
		},
	})
}
