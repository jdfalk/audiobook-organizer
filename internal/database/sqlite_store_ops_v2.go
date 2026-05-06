// file: internal/database/sqlite_store_ops_v2.go
// version: 1.0.0
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
