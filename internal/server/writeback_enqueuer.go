// file: internal/server/writeback_enqueuer.go
// version: 1.0.0
// guid: 5c255544-6862-47a8-bb9f-cce7630ecba5

package server

import "github.com/jdfalk/audiobook-organizer/internal/itunes"

// Enqueuer is the narrow slice of *WriteBackBatcher that callers actually
// need. Kept deliberately small so tests can mock it without spinning up a
// batcher goroutine, and so services can depend on the interface rather
// than the concrete type — which lets the concrete move packages (see the
// iTunes service extraction, spec 2026-04-18) without churning every caller.
//
// Matches the per-package WriteBackEnqueuer interfaces already declared in
// internal/merge and internal/organizer. Those packages keep their local
// declarations because their surfaces are narrower (only EnqueueRemove or
// only Enqueue); Enqueuer is the umbrella with all three methods for
// callers that need the full batcher surface.
type Enqueuer interface {
	Enqueue(bookID string)
	EnqueueAdd(track itunes.ITLNewTrack)
	EnqueueRemove(pid string)
}

// Compile-time proof *WriteBackBatcher satisfies Enqueuer. If the batcher
// gains or renames methods this assertion catches it.
var _ Enqueuer = (*WriteBackBatcher)(nil)
