// file: internal/database/pebble_store_ops_v2.go
// version: 3.0.0
// guid: c3d4e5f6-a7b8-9c0d-1e2f-3a4b5c6d7e8f

// pebble_store_ops_v2 implements OpsV2Store for PebbleDB (the primary production
// database). Key schema (all prefixed with "opv2:"):
//
//	opv2:def:{def_id}                               → JSON(OpDefinitionV2Row)
//	opv2:op:{op_id}                                 → JSON(OperationV2Row)
//	opv2:q:{999-priority:03d}:{ts_nano:020d}:{op_id} → op_id  (queue index)
//	opv2:act:{op_id}                                → ""      (active: queued|running)
//	opv2:state:{op_id}                              → JSON(OpStateV2Row)
//	opv2:log:{op_id}:{ts_nano:020d}:{seq:010d}      → JSON(OpLogV2Row)
//	opv2:err:{op_id}:{ts_nano:020d}                 → JSON(OpErrorV2Row)
//	opv2:strike:{def_id}:{ts_nano:020d}:{op_id}     → JSON(OpStrikeV2Row)

package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/pebble/v2"
)

// key builders

func opv2DefKey(defID string) []byte {
	return []byte("opv2:def:" + defID)
}

func opv2OpKey(opID string) []byte {
	return []byte("opv2:op:" + opID)
}

// opv2QueueKey encodes priority DESC (inverted), queued_at ASC into a
// lexicographically sortable key so a simple prefix scan returns ops in
// dispatch order without a secondary sort.
func opv2QueueKey(priority int, queuedAt time.Time, opID string) []byte {
	// 999-priority gives higher priorities a smaller prefix → sorts first.
	return []byte(fmt.Sprintf("opv2:q:%03d:%020d:%s", 999-priority, queuedAt.UnixNano(), opID))
}

func opv2ActKey(opID string) []byte {
	return []byte("opv2:act:" + opID)
}

func opv2StateKey(opID string) []byte {
	return []byte("opv2:state:" + opID)
}

func opv2LogKey(opID string, ts time.Time, seq int64) []byte {
	return []byte(fmt.Sprintf("opv2:log:%s:%020d:%010d", opID, ts.UnixNano(), seq))
}

func opv2ErrKey(opID string, ts time.Time) []byte {
	return []byte(fmt.Sprintf("opv2:err:%s:%020d", opID, ts.UnixNano()))
}

func opv2StrikeKey(defID string, ts time.Time, opID string) []byte {
	return []byte(fmt.Sprintf("opv2:strike:%s:%020d:%s", defID, ts.UnixNano(), opID))
}

// pebbleGet reads a single key and JSON-decodes into dst. Returns nil, nil if not found.
func (p *PebbleStore) pebbleGetJSON(key []byte, dst any) error {
	val, closer, err := p.db.Get(key)
	if errors.Is(err, pebble.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	defer closer.Close()
	return json.Unmarshal(val, dst)
}

func (p *PebbleStore) pebbleSetJSON(key []byte, src any) error {
	data, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return p.db.Set(key, data, pebble.Sync)
}

// UpsertOpDefinitionV2 inserts or replaces a definition row.
func (p *PebbleStore) UpsertOpDefinitionV2(row OpDefinitionV2Row) error {
	return p.pebbleSetJSON(opv2DefKey(row.ID), &row)
}

// DeleteOrphanOpDefsV2 removes definition rows whose ID is not in keepIDs.
func (p *PebbleStore) DeleteOrphanOpDefsV2(keepIDs []string) error {
	keep := make(map[string]bool, len(keepIDs))
	for _, id := range keepIDs {
		keep[id] = true
	}

	prefix := []byte("opv2:def:")
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: prefixEnd(prefix),
	})
	if err != nil {
		return err
	}
	defer iter.Close()

	var toDelete [][]byte
	for iter.First(); iter.Valid(); iter.Next() {
		k := iter.Key()
		defID := strings.TrimPrefix(string(k), "opv2:def:")
		if !keep[defID] {
			cp := make([]byte, len(k))
			copy(cp, k)
			toDelete = append(toDelete, cp)
		}
	}
	if err := iter.Error(); err != nil {
		return err
	}

	for _, k := range toDelete {
		if err := p.db.Delete(k, pebble.Sync); err != nil {
			return err
		}
	}
	return nil
}

// InsertOperationV2 inserts a new queued operation row and adds it to the queue index.
func (p *PebbleStore) InsertOperationV2(row OperationV2Row) error {
	if err := p.pebbleSetJSON(opv2OpKey(row.ID), &row); err != nil {
		return err
	}
	if row.Status == "queued" {
		if err := p.db.Set(opv2QueueKey(row.Priority, row.QueuedAt, row.ID), []byte(row.ID), pebble.Sync); err != nil {
			return err
		}
		if err := p.db.Set(opv2ActKey(row.ID), nil, pebble.Sync); err != nil {
			return err
		}
	}
	return nil
}

// ListQueuedOperationsV2 returns queued ops ordered by priority DESC, queued_at ASC.
func (p *PebbleStore) ListQueuedOperationsV2() ([]OperationV2Row, error) {
	prefix := []byte("opv2:q:")
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: prefixEnd(prefix),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var result []OperationV2Row
	for iter.First(); iter.Valid(); iter.Next() {
		val := iter.Value()
		opID := string(val)
		if opID == "" {
			continue
		}
		var row OperationV2Row
		if err := p.pebbleGetJSON(opv2OpKey(opID), &row); err != nil {
			continue
		}
		if row.Status != "queued" {
			continue
		}
		result = append(result, row)
	}
	return result, iter.Error()
}

// GetOperationV2 returns a single operation by id.
func (p *PebbleStore) GetOperationV2(id string) (*OperationV2Row, error) {
	var row OperationV2Row
	if err := p.pebbleGetJSON(opv2OpKey(id), &row); err != nil {
		return nil, err
	}
	if row.ID == "" {
		return nil, nil
	}
	return &row, nil
}

// UpdateOperationV2Status updates status and optional timestamps on an operation,
// maintaining the queue/active indexes as status transitions occur.
func (p *PebbleStore) UpdateOperationV2Status(id, status string, startedAt, completedAt *time.Time, errMsg *string) error {
	p.opsMu.Lock()
	defer p.opsMu.Unlock()

	var row OperationV2Row
	if err := p.pebbleGetJSON(opv2OpKey(id), &row); err != nil {
		return err
	}
	if row.ID == "" {
		return fmt.Errorf("opv2: operation not found: %s", id)
	}

	oldStatus := row.Status
	row.Status = status
	if startedAt != nil {
		row.StartedAt = startedAt
	}
	if completedAt != nil {
		row.CompletedAt = completedAt
	}
	if errMsg != nil {
		row.ErrorMessage = errMsg
	}

	if err := p.pebbleSetJSON(opv2OpKey(id), &row); err != nil {
		return err
	}

	// Remove from queue index if it was queued.
	if oldStatus == "queued" {
		_ = p.db.Delete(opv2QueueKey(row.Priority, row.QueuedAt, id), pebble.Sync)
	}
	// Maintain active set.
	if status == "running" {
		_ = p.db.Set(opv2ActKey(id), nil, pebble.Sync)
	} else if status != "queued" {
		_ = p.db.Delete(opv2ActKey(id), pebble.Sync)
	}
	return nil
}

// SetOperationV2StatusIfQueued atomically transitions status only when current status is 'queued'.
// Returns true if the row was updated.
func (p *PebbleStore) SetOperationV2StatusIfQueued(id, newStatus string) (bool, error) {
	p.opsMu.Lock()
	defer p.opsMu.Unlock()

	var row OperationV2Row
	if err := p.pebbleGetJSON(opv2OpKey(id), &row); err != nil {
		return false, err
	}
	if row.ID == "" || row.Status != "queued" {
		return false, nil
	}

	row.Status = newStatus
	if err := p.pebbleSetJSON(opv2OpKey(id), &row); err != nil {
		return false, err
	}
	_ = p.db.Delete(opv2QueueKey(row.Priority, row.QueuedAt, id), pebble.Sync)
	if newStatus != "running" {
		_ = p.db.Delete(opv2ActKey(id), pebble.Sync)
	}
	return true, nil
}

// ListActiveOperationsV2 returns ops with status 'queued' or 'running'.
func (p *PebbleStore) ListActiveOperationsV2() ([]OperationV2Row, error) {
	prefix := []byte("opv2:act:")
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: prefixEnd(prefix),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var result []OperationV2Row
	for iter.First(); iter.Valid(); iter.Next() {
		opID := strings.TrimPrefix(string(iter.Key()), "opv2:act:")
		var row OperationV2Row
		if err := p.pebbleGetJSON(opv2OpKey(opID), &row); err != nil || row.ID == "" {
			continue
		}
		result = append(result, row)
	}
	return result, iter.Error()
}

// CountRunningByPluginV2 returns the count of running ops for a plugin.
func (p *PebbleStore) CountRunningByPluginV2(plugin string) (int, error) {
	active, err := p.ListActiveOperationsV2()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range active {
		if r.Plugin == plugin && r.Status == "running" {
			n++
		}
	}
	return n, nil
}

// IncrementResumeCountV2 atomically increments resume_count for the given op.
func (p *PebbleStore) IncrementResumeCountV2(id string) error {
	p.opsMu.Lock()
	defer p.opsMu.Unlock()

	var row OperationV2Row
	if err := p.pebbleGetJSON(opv2OpKey(id), &row); err != nil {
		return err
	}
	row.ResumeCount++
	return p.pebbleSetJSON(opv2OpKey(id), &row)
}

// UpdateOpProgressV2 updates the progress fields and last_progress_at.
func (p *PebbleStore) UpdateOpProgressV2(id string, current, total int, message string) error {
	p.opsMu.Lock()
	defer p.opsMu.Unlock()

	var row OperationV2Row
	if err := p.pebbleGetJSON(opv2OpKey(id), &row); err != nil {
		return err
	}
	now := time.Now().UTC()
	row.ProgressCurrent = current
	row.ProgressTotal = total
	row.ProgressMessage = message
	row.LastProgressAt = &now
	return p.pebbleSetJSON(opv2OpKey(id), &row)
}

// UpdateOpPhaseV2 sets or clears current_phase on an operation.
func (p *PebbleStore) UpdateOpPhaseV2(id string, phase *string) error {
	p.opsMu.Lock()
	defer p.opsMu.Unlock()

	var row OperationV2Row
	if err := p.pebbleGetJSON(opv2OpKey(id), &row); err != nil {
		return err
	}
	row.CurrentPhase = phase
	return p.pebbleSetJSON(opv2OpKey(id), &row)
}

// UpdateOpCheckpointV2 sets last_checkpoint_at and updates high_water_progress.
func (p *PebbleStore) UpdateOpCheckpointV2(id string, newHWM int) error {
	p.opsMu.Lock()
	defer p.opsMu.Unlock()

	var row OperationV2Row
	if err := p.pebbleGetJSON(opv2OpKey(id), &row); err != nil {
		return err
	}
	now := time.Now().UTC()
	row.LastCheckpointAt = &now
	if newHWM > row.HighWaterProgress {
		row.HighWaterProgress = newHWM
	}
	return p.pebbleSetJSON(opv2OpKey(id), &row)
}

// UpsertOpStateV2 inserts or replaces the checkpoint state for an operation.
func (p *PebbleStore) UpsertOpStateV2(row OpStateV2Row) error {
	return p.pebbleSetJSON(opv2StateKey(row.OperationID), &row)
}

// GetOpStateV2 returns the state blob for an op, or nil if not found.
func (p *PebbleStore) GetOpStateV2(opID string) (*OpStateV2Row, error) {
	var row OpStateV2Row
	if err := p.pebbleGetJSON(opv2StateKey(opID), &row); err != nil {
		return nil, err
	}
	if row.OperationID == "" {
		return nil, nil
	}
	return &row, nil
}

// DeleteOpStateV2 removes the state blob for an op.
func (p *PebbleStore) DeleteOpStateV2(opID string) error {
	return p.db.Delete(opv2StateKey(opID), pebble.Sync)
}

// AppendOpLogsV2 bulk-inserts log rows.
func (p *PebbleStore) AppendOpLogsV2(rows []OpLogV2Row) error {
	if len(rows) == 0 {
		return nil
	}
	batch := p.db.NewBatch()
	defer batch.Close()
	for _, row := range rows {
		seq := atomic.AddInt64(&p.opsLogSeq, 1)
		key := opv2LogKey(row.OperationID, row.CreatedAt, seq)
		data, err := json.Marshal(&row)
		if err != nil {
			return err
		}
		if err := batch.Set(key, data, nil); err != nil {
			return err
		}
	}
	return batch.Commit(pebble.Sync)
}

// InsertOpErrorV2 inserts a single error record.
func (p *PebbleStore) InsertOpErrorV2(row OpErrorV2Row) error {
	return p.pebbleSetJSON(opv2ErrKey(row.OperationID, row.OccurredAt), &row)
}

// InsertOpStrikeV2 appends a row to op_strikes_v2.
func (p *PebbleStore) InsertOpStrikeV2(row OpStrikeV2Row) error {
	return p.pebbleSetJSON(opv2StrikeKey(row.DefID, row.OccurredAt, row.OperationID), &row)
}

// ListOperationsV2Since returns operations queued at or after `since`, ordered
// by started_at DESC NULLS LAST, queued_at DESC, up to `limit` rows.
func (p *PebbleStore) ListOperationsV2Since(since time.Time, limit int) ([]OperationV2Row, error) {
	if limit <= 0 {
		limit = 200
	}
	prefix := []byte("opv2:op:")
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: prefixEnd(prefix),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var all []OperationV2Row
	for iter.First(); iter.Valid(); iter.Next() {
		var row OperationV2Row
		if err := json.Unmarshal(iter.Value(), &row); err != nil {
			continue
		}
		if !row.QueuedAt.Before(since) {
			all = append(all, row)
		}
	}
	if err := iter.Error(); err != nil {
		return nil, err
	}

	sort.Slice(all, func(i, j int) bool {
		si, sj := all[i].StartedAt, all[j].StartedAt
		// NULLS LAST: nil StartedAt sorts after non-nil.
		if si == nil && sj == nil {
			return all[i].QueuedAt.After(all[j].QueuedAt)
		}
		if si == nil {
			return false
		}
		if sj == nil {
			return true
		}
		if !si.Equal(*sj) {
			return si.After(*sj)
		}
		return all[i].QueuedAt.After(all[j].QueuedAt)
	})

	if len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

// GetOpLogsV2 returns up to `limit` log lines for the given operation, ordered by created_at ASC.
// A limit ≤ 0 returns all rows.
func (p *PebbleStore) GetOpLogsV2(opID string, limit int) ([]OpLogV2Row, error) {
	prefix := []byte("opv2:log:" + opID + ":")
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: prefixEnd(prefix),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var result []OpLogV2Row
	for iter.First(); iter.Valid(); iter.Next() {
		var row OpLogV2Row
		if err := json.Unmarshal(iter.Value(), &row); err != nil {
			continue
		}
		result = append(result, row)
	}
	if err := iter.Error(); err != nil {
		return nil, err
	}

	if limit > 0 && len(result) > limit {
		result = result[len(result)-limit:]
	}
	return result, nil
}
