// file: internal/itunes/service/enqueuer.go
// version: 1.0.0
// guid: 2b66355b-ff2a-48fa-854d-e38711835d91

package itunesservice

import "github.com/jdfalk/audiobook-organizer/internal/itunes"

// Enqueuer is the narrow slice of *WriteBackBatcher that callers use.
// Defined here so sub-components inside internal/itunes/service/ (e.g.
// TrackProvisioner, PlaylistSync, PositionSync) can depend on the
// interface without importing internal/server — server already imports
// this package, so a reverse import would create a cycle.
//
// The server package keeps a parallel declaration (internal/server/
// writeback_enqueuer.go) as a type alias so existing server-package
// callers don't need to update their field/param types until Phase 2
// M1 moves them naturally.
type Enqueuer interface {
	Enqueue(bookID string)
	EnqueueAdd(track itunes.ITLNewTrack)
	EnqueueRemove(pid string)
}
