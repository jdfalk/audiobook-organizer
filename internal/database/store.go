// file: internal/database/store.go
// version: 2.60.1
// guid: 8a9b0c1d-2e3f-4a5b-6c7d-8e9f0a1b2c3d

package database

import (
	"fmt"
	"sync"
	"time"
)

// Store defines the full database surface. Most services should depend
// on a narrower sub-interface defined in iface_*.go; Store itself is
// used by the server bootstrap and test fixtures that genuinely need
// wide access. See docs/superpowers/specs/2026-04-17-store-interface-segregation-design.md.
type Store interface {
	LifecycleStore
	BookStore
	AuthorStore
	SeriesStore
	UserStore
	NarratorStore
	WorkStore
	SessionStore
	RoleStore
	APIKeyStore
	InviteStore
	UserPreferenceStore
	UserPositionStore
	BookVersionStore
	BookFileStore
	BookSegmentStore
	PlaylistStore
	UserPlaylistStore
	ImportPathStore
	OperationStore
	TagStore
	UserTagStore
	MetadataStore
	HashBlocklistStore
	ITunesStateStore
	PathHistoryStore
	ExternalIDStore
	RawKVStore
	PlaybackStore
	SettingsStore
	StatsStore
	MaintenanceStore
	SystemActivityStore
}
...