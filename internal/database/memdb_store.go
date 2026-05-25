// file: internal/database/memdb_store.go
// version: 1.0.0
// guid: a1b2c3d4-mema-aaaa-aaaa-000000000003

package database

import (
	"fmt"

	"github.com/hashicorp/go-memdb"
)

// MemStore is an in-memory query/index layer over PebbleDB. PebbleDB remains
// the source of truth and durable store; MemStore is rebuilt from Pebble on
// startup and kept in sync via write-through.
//
// Reads use snapshot transactions (no locking, MVCC via immutable radix
// trees). Writes are serialized by go-memdb's single-writer model but never
// block readers.
type MemStore struct {
	db *memdb.MemDB
}

// NewMemStore allocates an empty MemStore with the full schema applied.
// Call WarmFromPebble after construction to populate it from a PebbleStore.
func NewMemStore() (*MemStore, error) {
	db, err := memdb.NewMemDB(memdbSchema())
	if err != nil {
		return nil, fmt.Errorf("memdb: failed to build schema: %w", err)
	}
	return &MemStore{db: db}, nil
}

// Txn begins a transaction. Pass write=true for mutations.
// Always defer Abort(); call Commit() to publish writes.
func (m *MemStore) Txn(write bool) *memdb.Txn {
	return m.db.Txn(write)
}

// Snapshot returns a point-in-time snapshot view. Useful for long-running
// reads that should see a consistent state without holding back writers.
func (m *MemStore) Snapshot() *MemStore {
	return &MemStore{db: m.db.Snapshot()}
}
