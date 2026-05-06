// file: internal/database/pebble_store_ops_v2.go
// version: 2.0.0
// guid: c3d4e5f6-a7b8-9c0d-1e2f-3a4b5c6d7e8f
// last-edited: 2026-05-06

package database

import (
	"errors"
	"time"
)

// ErrOpsV2NotSupported is returned by PebbleStore for UOS v2 operations.
// The v2 schema lives in SQLite; PebbleStore is the production store for
// library data only.
var ErrOpsV2NotSupported = errors.New("OpsV2Store not supported by PebbleStore")

func (p *PebbleStore) UpsertOpDefinitionV2(_ OpDefinitionV2Row) error {
	return ErrOpsV2NotSupported
}

func (p *PebbleStore) DeleteOrphanOpDefsV2(_ []string) error {
	return ErrOpsV2NotSupported
}

func (p *PebbleStore) InsertOperationV2(_ OperationV2Row) error {
	return ErrOpsV2NotSupported
}

func (p *PebbleStore) ListQueuedOperationsV2() ([]OperationV2Row, error) {
	return nil, ErrOpsV2NotSupported
}

func (p *PebbleStore) GetOperationV2(_ string) (*OperationV2Row, error) {
	return nil, ErrOpsV2NotSupported
}

func (p *PebbleStore) UpdateOperationV2Status(_ string, _ string, _, _ *time.Time, _ *string) error {
	return ErrOpsV2NotSupported
}

func (p *PebbleStore) SetOperationV2StatusIfQueued(_, _ string) (bool, error) {
	return false, ErrOpsV2NotSupported
}

func (p *PebbleStore) CountRunningByPluginV2(_ string) (int, error) {
	return 0, ErrOpsV2NotSupported
}

func (p *PebbleStore) ListActiveOperationsV2() ([]OperationV2Row, error) {
	return nil, ErrOpsV2NotSupported
}

func (p *PebbleStore) IncrementResumeCountV2(_ string) error {
	return ErrOpsV2NotSupported
}

func (p *PebbleStore) InsertOpStrikeV2(_ OpStrikeV2Row) error {
	return ErrOpsV2NotSupported
}

func (p *PebbleStore) GetOpStateV2(_ string) (*OpStateV2Row, error) {
	return nil, ErrOpsV2NotSupported
}

func (p *PebbleStore) DeleteOpStateV2(_ string) error {
	return ErrOpsV2NotSupported
}
