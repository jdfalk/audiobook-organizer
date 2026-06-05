// file: internal/merge/lifecycle.go
// version: 1.0.0

// PostInit method on *Service. Pulls the iTunes write-back enqueuer
// from the container so merge operations can clean up stale PIDs.
// Replaces the inline server.mergeService.SetWriteBackBatcher call in
// NewServer.

package merge

import (
	"context"

	"github.com/falkcorp/audiobook-organizer/internal/serviceregistry"
)

// PostInit wires the iTunes write-back enqueuer (WriteBackEnqueuer
// interface; itunes-disabled path → typed-nil; left unset).
func (ms *Service) PostInit(_ context.Context, c *serviceregistry.Container) error {
	if ms == nil {
		return nil
	}
	if enq, ok := serviceregistry.TryGet[WriteBackEnqueuer](c, "writebackbatcher"); ok && enq != nil {
		ms.SetWriteBackBatcher(enq)
	}
	return nil
}
