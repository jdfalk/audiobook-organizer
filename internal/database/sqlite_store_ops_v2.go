// file: internal/database/sqlite_store_ops_v2.go
// version: 2.2.0
// guid: b2c3d4e5-f6a7-8b9c-0d1e-2f3a4b5c6d71
// last-edited: 2026-05-06

package database

import (
	"fmt"
	"strings"
	"time"
)

// UpsertOpDefinitionV2 inserts or replaces a definition row in op_definitions_v2.
func (s *SQLiteStore) UpsertOpDefinitionV2(row OpDefinitionV2Row) error {
	_, err := s.db.Exec(`
		INSERT INTO op_definitions_v2
			(id, plugin, display_name, description, capabilities, permissions,
			 cancellable, isolate, resume_policy, schedule_cron, triggers,
			 depends_on, phases, timeout_seconds, registered_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			plugin=excluded.plugin,
			display_name=excluded.display_name,
			description=excluded.description,
			capabilities=excluded.capabilities,
			permissions=excluded.permissions,
			cancellable=excluded.cancellable,
			isolate=excluded.isolate,
			resume_policy=excluded.resume_policy,
			schedule_cron=excluded.schedule_cron,
			triggers=excluded.triggers,
			depends_on=excluded.depends_on,
			phases=excluded.phases,
			timeout_seconds=excluded.timeout_seconds,
			registered_at=excluded.registered_at
	`,
		row.ID, row.Plugin, row.DisplayName, row.Description,
		row.Capabilities, row.Permissions,
		row.Cancellable, row.Isolate, row.ResumePolicy,
		row.ScheduleCron,
		row.Triggers, row.DependsOn, row.Phases,
		row.TimeoutSeconds, row.RegisteredAt,
	)
	return err
}

// DeleteOrphanOpDefsV2 removes op_definitions_v2 rows whose id is not in keepIDs.
func (s *SQLiteStore) DeleteOrphanOpDefsV2(keepIDs []string) error {
	if len(keepIDs) == 0 {
		_, err := s.db.Exec(`DELETE FROM op_definitions_v2`)
		return err
	}
	placeholders := make([]string, len(keepIDs))
	args := make([]any, len(keepIDs))
	for i, id := range keepIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	q := fmt.Sprintf(`DELETE FROM op_definitions_v2 WHERE id NOT IN (%s)`,
		strings.Join(placeholders, ","))
	_, err := s.db.Exec(q, args...)
	return err
}

// InsertOperationV2 inserts a new row into operations_v2.
func (s *SQLiteStore) InsertOperationV2(row OperationV2Row) error {
	_, err := s.db.Exec(`
		INSERT INTO operations_v2
			(id, def_id, plugin, parent_id, actor_user_id, trace_id, span_id, parent_span_id,
			 status, priority, params, queued_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		row.ID, row.DefID, row.Plugin, row.ParentID, row.ActorUserID,
		row.TraceID, row.SpanID, row.ParentSpanID,
		row.Status, row.Priority, row.Params, row.QueuedAt,
	)
	return err
}

// ListQueuedOperationsV2 returns queued ops ordered by priority DESC, queued_at ASC.
func (s *SQLiteStore) ListQueuedOperationsV2() ([]OperationV2Row, error) {
	rows, err := s.db.Query(`
		SELECT id, def_id, plugin, parent_id, actor_user_id, trace_id, span_id,
		       parent_span_id, status, priority, params, queued_at
		FROM operations_v2
		WHERE status = 'queued'
		ORDER BY priority DESC, queued_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []OperationV2Row
	for rows.Next() {
		var r OperationV2Row
		if err := rows.Scan(
			&r.ID, &r.DefID, &r.Plugin, &r.ParentID, &r.ActorUserID,
			&r.TraceID, &r.SpanID, &r.ParentSpanID,
			&r.Status, &r.Priority, &r.Params, &r.QueuedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// GetOperationV2 returns a single run by id.
func (s *SQLiteStore) GetOperationV2(id string) (*OperationV2Row, error) {
	row := &OperationV2Row{}
	err := s.db.QueryRow(`
		SELECT id, def_id, plugin, parent_id, actor_user_id, trace_id, span_id,
		       parent_span_id, status, priority, params, queued_at,
		       started_at, completed_at, error_message
		FROM operations_v2
		WHERE id = ?
	`, id).Scan(
		&row.ID, &row.DefID, &row.Plugin, &row.ParentID, &row.ActorUserID,
		&row.TraceID, &row.SpanID, &row.ParentSpanID,
		&row.Status, &row.Priority, &row.Params, &row.QueuedAt,
		&row.StartedAt, &row.CompletedAt, &row.ErrorMessage,
	)
	if err != nil {
		return nil, err
	}
	return row, nil
}

// UpdateOperationV2Status updates the status and optional timestamps of an op.
func (s *SQLiteStore) UpdateOperationV2Status(id, status string, startedAt, completedAt *time.Time, errMsg *string) error {
	_, err := s.db.Exec(`
		UPDATE operations_v2
		SET status        = ?,
		    started_at    = COALESCE(?, started_at),
		    completed_at  = COALESCE(?, completed_at),
		    error_message = COALESCE(?, error_message)
		WHERE id = ?
	`, status, startedAt, completedAt, errMsg, id)
	return err
}

// SetOperationV2StatusIfQueued atomically sets status only if the row is currently queued.
func (s *SQLiteStore) SetOperationV2StatusIfQueued(id, newStatus string) (bool, error) {
	res, err := s.db.Exec(`
		UPDATE operations_v2
		SET status = ?
		WHERE id = ? AND status = 'queued'
	`, newStatus, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// CountRunningByPluginV2 returns the count of running ops for a plugin.
func (s *SQLiteStore) CountRunningByPluginV2(plugin string) (int, error) {
	var n int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM operations_v2
		WHERE plugin = ? AND status = 'running'
	`, plugin).Scan(&n)
	return n, err
}

// UpdateOpProgressV2 updates progress_current, progress_total, progress_message, last_progress_at.
func (s *SQLiteStore) UpdateOpProgressV2(id string, current, total int, message string) error {
	_, err := s.db.Exec(`
		UPDATE operations_v2
		SET progress_current   = ?,
		    progress_total     = ?,
		    progress_message   = ?,
		    last_progress_at   = ?
		WHERE id = ?
	`, current, total, message, time.Now().UTC(), id)
	return err
}

// UpdateOpPhaseV2 sets or clears the current_phase column.
func (s *SQLiteStore) UpdateOpPhaseV2(id string, phase *string) error {
	_, err := s.db.Exec(`UPDATE operations_v2 SET current_phase = ? WHERE id = ?`, phase, id)
	return err
}

// UpdateOpCheckpointV2 sets last_checkpoint_at and updates high_water_progress to MAX(old, newHWM).
func (s *SQLiteStore) UpdateOpCheckpointV2(id string, newHWM int) error {
	_, err := s.db.Exec(`
		UPDATE operations_v2
		SET last_checkpoint_at  = ?,
		    high_water_progress = MAX(high_water_progress, ?)
		WHERE id = ?
	`, time.Now().UTC(), newHWM, id)
	return err
}

// AppendOpLogsV2 bulk-inserts log rows into op_logs_v2.
func (s *SQLiteStore) AppendOpLogsV2(rows []OpLogV2Row) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`
		INSERT INTO op_logs_v2 (operation_id, level, message, attrs, created_at)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, row := range rows {
		if _, err := stmt.Exec(row.OperationID, row.Level, row.Message, row.Attrs, row.CreatedAt); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// InsertOpErrorV2 inserts a single error record into op_errors_v2.
func (s *SQLiteStore) InsertOpErrorV2(row OpErrorV2Row) error {
	_, err := s.db.Exec(`
		INSERT INTO op_errors_v2 (operation_id, plugin, def_id, message, attrs, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, row.OperationID, row.Plugin, row.DefID, row.Message, row.Attrs, row.OccurredAt)
	return err
}

// UpsertOpStateV2 inserts or replaces the checkpoint row for an operation.
func (s *SQLiteStore) UpsertOpStateV2(row OpStateV2Row) error {
	_, err := s.db.Exec(`
		INSERT INTO op_state_v2 (operation_id, phase, state_blob, schema_version, written_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(operation_id) DO UPDATE SET
			phase          = excluded.phase,
			state_blob     = excluded.state_blob,
			schema_version = excluded.schema_version,
			written_at     = excluded.written_at
	`, row.OperationID, row.Phase, row.StateBlob, row.SchemaVersion, row.WrittenAt)
	return err
}

// GetOpStateV2 returns the latest checkpoint row for an operation, or nil if not found.
func (s *SQLiteStore) GetOpStateV2(opID string) (*OpStateV2Row, error) {
	row := &OpStateV2Row{}
	err := s.db.QueryRow(`
		SELECT operation_id, phase, state_blob, schema_version, written_at
		FROM op_state_v2
		WHERE operation_id = ?
	`, opID).Scan(&row.OperationID, &row.Phase, &row.StateBlob, &row.SchemaVersion, &row.WrittenAt)
	if err != nil {
		// Not found is treated as nil, no state.
		return nil, nil //nolint:nilerr
	}
	return row, nil
}

// DeleteOpStateV2 removes the state blob for an op.
func (s *SQLiteStore) DeleteOpStateV2(opID string) error {
	_, err := s.db.Exec(`DELETE FROM op_state_v2 WHERE operation_id = ?`, opID)
	return err
}

// ListActiveOperationsV2 returns ops with status 'queued' or 'running'.
func (s *SQLiteStore) ListActiveOperationsV2() ([]OperationV2Row, error) {
	rows, err := s.db.Query(`
		SELECT id, def_id, plugin, parent_id, actor_user_id, trace_id, span_id,
		       parent_span_id, status, priority, params, queued_at,
		       started_at, high_water_progress, resume_count
		FROM operations_v2
		WHERE status IN ('queued', 'running')
		ORDER BY queued_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []OperationV2Row
	for rows.Next() {
		var r OperationV2Row
		if err := rows.Scan(
			&r.ID, &r.DefID, &r.Plugin, &r.ParentID, &r.ActorUserID,
			&r.TraceID, &r.SpanID, &r.ParentSpanID,
			&r.Status, &r.Priority, &r.Params, &r.QueuedAt,
			&r.StartedAt, &r.HighWaterProgress, &r.ResumeCount,
		); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// IncrementResumeCountV2 atomically increments resume_count for the given op.
func (s *SQLiteStore) IncrementResumeCountV2(id string) error {
	_, err := s.db.Exec(`
		UPDATE operations_v2 SET resume_count = resume_count + 1 WHERE id = ?
	`, id)
	return err
}

// InsertOpStrikeV2 appends a row to op_strikes_v2.
func (s *SQLiteStore) InsertOpStrikeV2(row OpStrikeV2Row) error {
	_, err := s.db.Exec(`
		INSERT INTO op_strikes_v2 (def_id, operation_id, kind, details, occurred_at)
		VALUES (?, ?, ?, ?, ?)
	`, row.DefID, row.OperationID, row.Kind, row.Details, row.OccurredAt)
	return err
}

// ListOperationsV2Since returns operations queued at or after the given time.
// Results are ordered by started_at DESC NULLS LAST, queued_at DESC.
// A limit of 0 uses a safe default of 200.
func (s *SQLiteStore) ListOperationsV2Since(since time.Time, limit int) ([]OperationV2Row, error) {
	if limit <= 0 {
		limit = 200
	}
	// SQLite does not support NULLS LAST directly.
	// "started_at IS NULL" evaluates to 1 for NULL rows (1 > 0), so ordering
	// ASC on that column sorts non-NULL rows first, giving us NULLS LAST behaviour.
	rows, err := s.db.Query(`
		SELECT id, def_id, plugin, parent_id, actor_user_id, trace_id, span_id,
		       parent_span_id, status, priority, params, queued_at,
		       started_at, completed_at, error_message,
		       progress_current, progress_total, progress_message,
		       current_phase, high_water_progress, resume_count
		FROM operations_v2
		WHERE queued_at >= ?
		ORDER BY started_at IS NULL, started_at DESC, queued_at DESC
		LIMIT ?
	`, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []OperationV2Row
	for rows.Next() {
		var r OperationV2Row
		if err := rows.Scan(
			&r.ID, &r.DefID, &r.Plugin, &r.ParentID, &r.ActorUserID,
			&r.TraceID, &r.SpanID, &r.ParentSpanID,
			&r.Status, &r.Priority, &r.Params, &r.QueuedAt,
			&r.StartedAt, &r.CompletedAt, &r.ErrorMessage,
			&r.ProgressCurrent, &r.ProgressTotal, &r.ProgressMessage,
			&r.CurrentPhase, &r.HighWaterProgress, &r.ResumeCount,
		); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// GetOpLogsV2 returns the last limit log lines for the given operation,
// ordered by created_at ASC. A limit ≤ 0 returns all rows.
func (s *SQLiteStore) GetOpLogsV2(opID string, limit int) ([]OpLogV2Row, error) {
	query := `
		SELECT operation_id, level, message, attrs, created_at
		FROM op_logs_v2
		WHERE operation_id = ?
		ORDER BY created_at ASC
	`
	args := []any{opID}

	if limit > 0 {
		// Fetch the last N rows by wrapping in a sub-query so they come back
		// in chronological order (oldest-first) but we only keep the newest limit lines.
		query = `
			SELECT operation_id, level, message, attrs, created_at
			FROM (
				SELECT operation_id, level, message, attrs, created_at
				FROM op_logs_v2
				WHERE operation_id = ?
				ORDER BY created_at DESC
				LIMIT ?
			)
			ORDER BY created_at ASC
		`
		args = []any{opID, limit}
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []OpLogV2Row
	for rows.Next() {
		var r OpLogV2Row
		if err := rows.Scan(&r.OperationID, &r.Level, &r.Message, &r.Attrs, &r.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}
