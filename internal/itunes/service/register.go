// file: internal/itunes/service/register.go
// version: 1.0.0
// guid: f1e2d3c4-b5a6-7890-1a2b-3c4d5e6f7a8b
//
// Registers the writebackbatcher service with the global serviceregistry.
//
// WHY THIS IS A STUB
// ──────────────────
// itunesservice.Service is constructed in NewServer (internal/server/server.go
// ~line 395) with several server-bound closures:
//
//	OnBookCreated: func(bookID string) { server.fireDedupOnImport(bookID) }
//	Metafetch:     server.metadataFetchService
//	OrganizerFactory: func() BookOrganizer { return organizer.NewOrganizer(&config.AppConfig) }
//
// Those closures capture a live *Server pointer that cannot be obtained at
// registry Build time (Build runs before NewServer returns). Therefore the
// "itunes" parent service cannot be fully registered; the Batcher that lives
// inside it (svc.Batcher) is likewise unavailable at Build time.
//
// The "writebackbatcher" entry is registered as a typed-nil stub so that:
//   - TryGet[*WriteBackBatcher](c, "writebackbatcher") returns (nil, true),
//     signalling "service registered but not yet wired". Callers must nil-check
//     the value.
//   - The name is reserved in the registry to prevent accidental reuse.
//   - Once the server-binding closures are moved to PostInit (a future W-series
//     step), the Build func here can be upgraded to call New() properly.
//
// Coordinator note: registering the parent "itunes" service fully is blocked by
// the OnBookCreated / Metafetch / OrganizerFactory server-bound fields. When
// those are moved to PostInit, remove the stub and replace it with the real
// construction.

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

// Start implements serviceregistry.Starter. WriteBackBatcher has no explicit
// start: it begins processing on the first Enqueue. This is a no-op.
func (a *batcherAdapter) Start(_ context.Context) error {
	return nil
}

// Stop implements serviceregistry.Stopper. Delegates to WriteBackBatcher.Stop()
// which flushes pending writes before returning.
func (a *batcherAdapter) Stop(_ context.Context) error {
	if a.b != nil {
		a.b.Stop()
	}
	return nil
}

func init() {
	// "writebackbatcher" — stub registration.
	//
	// The real *WriteBackBatcher lives inside itunesservice.Service.Batcher,
	// which is constructed by NewServer using server-bound closures (see file
	// header). Build cannot create a real instance here.
	//
	// Returns a typed nil (*WriteBackBatcher)(nil) so TryGet returns
	// (nil, true) — callers know the service is registered but must
	// nil-check before use. The server's PostInit (or equivalent wiring)
	// is expected to Override "writebackbatcher" with a real *batcherAdapter
	// once itunesSvc.Batcher is available.
	serviceregistry.Register(serviceregistry.ServiceDef{
		Name:  "writebackbatcher",
		Needs: []string{},
		Groups: []string{"scheduler"},
		Build: func(c *serviceregistry.Container) (any, error) {
			// Stub: the batcher is server-constructed; see file header.
			return (*batcherAdapter)(nil), nil
		},
	})
}
