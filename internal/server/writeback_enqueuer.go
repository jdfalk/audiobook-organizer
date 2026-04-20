// file: internal/server/writeback_enqueuer.go
// version: 2.0.0
// guid: 5c255544-6862-47a8-bb9f-cce7630ecba5

package server

import itunesservice "github.com/jdfalk/audiobook-organizer/internal/itunes/service"

// Enqueuer is a type alias for itunesservice.Enqueuer. The canonical
// interface moved to internal/itunes/service/enqueuer.go during Phase 2
// M1 step 1 so sub-components inside that package can depend on it
// without importing server (which already imports itunesservice — would
// be a cycle). Server-package callers continue to use `server.Enqueuer`
// because Go type aliases make the names interchangeable.
type Enqueuer = itunesservice.Enqueuer

// Compile-time proof *itunesservice.WriteBackBatcher satisfies Enqueuer.
// If the batcher gains or renames methods this assertion catches it.
// (The batcher moved under internal/itunes/service/ during M1 step 2;
// the assertion can eventually move alongside it once the server alias
// disappears.)
var _ Enqueuer = (*itunesservice.WriteBackBatcher)(nil)
