// file: internal/quarantine/lifecycle.go
// version: 1.0.0

// PostInit method on *QuarantineService. Pulls the iTunes write-back
// enqueuer from the container. Replaces the inline
// server.quarantineSvc.SetWriteBackBatcher call in NewServer.

package quarantine

import (
	"context"

	"github.com/jdfalk/audiobook-organizer/internal/serviceregistry"
)

// PostInit wires the iTunes write-back enqueuer (WriteBackEnqueuer
// interface; itunes-disabled path → typed-nil; left unset).
func (qs *QuarantineService) PostInit(_ context.Context, c *serviceregistry.Container) error {
	if qs == nil {
		return nil
	}
	if enq, ok := serviceregistry.TryGet[WriteBackEnqueuer](c, "writebackbatcher"); ok && enq != nil {
		qs.SetWriteBackBatcher(enq)
	}
	return nil
}
