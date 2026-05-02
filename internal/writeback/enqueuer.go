// file: internal/writeback/enqueuer.go
// version: 1.0.0
// guid: 5c255544-6862-47a8-bb9f-cce7630ecba5
// last-edited: 2026-05-01

package writeback

import itunesservice "github.com/jdfalk/audiobook-organizer/internal/itunes/service"

// Enqueuer is a type alias for itunesservice.Enqueuer. The canonical
// interface lives in internal/itunes/service/enqueuer.go; this alias
// lets writeback-package callers use `writeback.Enqueuer` without
// importing the itunes/service package directly.
type Enqueuer = itunesservice.Enqueuer

// Compile-time proof *itunesservice.WriteBackBatcher satisfies Enqueuer.
var _ Enqueuer = (*itunesservice.WriteBackBatcher)(nil)
