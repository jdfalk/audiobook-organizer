// file: internal/itunes/service/register.go
// version: 3.0.0
// guid: f1e2d3c4-b5a6-7890-1a2b-3c4d5e6f7a8b
//
// Registers the "writebackbatcher" service with the global serviceregistry.
//
// Build pulls *itunesservice.Service from the container and returns
// svc.Batcher directly. *WriteBackBatcher implements Start(ctx)/Stop(ctx)
// matching the serviceregistry Starter/Stopper interfaces (since v5.1.0),
// so no adapter wrap is needed — the container will drive lifecycle
// directly once SERVER-LIFECYCLE-FLIP wires Container.Start/Stop.
//
// On test paths (cfg.Enabled false → itunesservice.NewDisabled()) svc.Batcher
// is nil. Build returns typed-nil; callers must nil-check before Enqueue.

package itunesservice

import (
	"github.com/jdfalk/audiobook-organizer/internal/serviceregistry"
)

func init() {
	serviceregistry.Register(serviceregistry.ServiceDef{
		Name:   "writebackbatcher",
		Needs:  []string{"itunes"},
		Groups: []string{"scheduler"},
		Build: func(c *serviceregistry.Container) (any, error) {
			svc := serviceregistry.Get[*Service](c, "itunes")
			if svc == nil {
				return (*WriteBackBatcher)(nil), nil
			}
			return svc.Batcher, nil
		},
	})
}
