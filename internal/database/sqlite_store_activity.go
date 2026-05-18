// file: internal/database/sqlite_store_activity.go
// version: 1.0.1
// guid: c1d2e3f4-5678-90ab-cdef-123456789012
// last-edited: 2026-05-02

package database

import (
	"database/sql"
	"fmt"
	"time"

	ulid "github.com/oklog/ulid/v2"
)

func (s *SQLiteStore) CreateOperation(id, opType string, folderPath *string) (*Operation, error) {
	now := time.Now()
	_, err := s.db.Exec(`INSERT INTO operations (id, type, status, folder_path, created_at)
		VALUES (?, ?, ?, ?, ?)`, id, opType, "pending", folderPath, now)
	if err != nil {
		return nil, err
	}
	return &Operation{
		ID:         id,
		Type:       opType,
		Status:     "pending",
		Progress:   0,
		Total:      0,
		Message:    "",
		FolderPath: folderPath,
		CreatedAt:  now,
	}, nil
}

func (s *SQLiteStore) GetOperationByID(id string) (*Operation, error) {
	var op Operation
	query := `SELECT id, type, status, progress, total, message, folder_path,
			  created_at, started_at, completed_at, error_message, result_data
			  FROM operations WHERE id = ?`
	err := s.db.QueryRow(query, id).Scan(&op.ID, &op.Type, &op.Status, &op.Progress,
		&op.Total, &op.Message, &op.FolderPath, &op.CreatedAt, &op.StartedAt,
		&op.CompletedAt, &op.ErrorMessage, &op.ResultData)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &op, nil
}

func (s *SQLiteStore) GetRecentOperations(limit int) ([]Operation, error) {
	query := `SELECT id, type, status, progress, total, message, folder_path,
			  created_at, started_at, completed_at, error_message, result_data
			  FROM operations ORDER BY created_at DESC LIMIT ?`
	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var operations []Operation
	for rows.Next() {
		var op Operation
		if err := rows.Scan(&op.ID, &op.Type, &op.Status, &op.Progress, &op.Total,
			&op.Message, &op.FolderPath, &op.CreatedAt, &op.StartedAt,
			&op.CompletedAt, &op.ErrorMessage, &op.ResultData); err != nil {
			return nil, err
		}
		operations = append(operations, op)
	}
	return operations, rows.Err()
}

func (s *SQLiteStore) ListOperations(limit, offset int) ([]Operation, int, error) {
	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM operations").Scan(&total); err != nil {
		return nil, 0, err
	}
	query := `SELECT id, type, status, progress, total, message, folder_path,
			  created_at, started_at, completed_at, error_message, result_data
			  FROM operations ORDER BY created_at DESC LIMIT ? OFFSET ?`
	rows, err := s.db.Query(query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var operations []Operation
	for rows.Next() {
		var op Operation
		if err := rows.Scan(&op.ID, &op.Type, &op.Status, &op.Progress, &op.Total,
			&op.Message, &op.FolderPath, &op.CreatedAt, &op.StartedAt,
			&op.CompletedAt, &op.ErrorMessage, &op.ResultData); err != nil {
			return nil, 0, err
		}
		operations = append(operations, op)
	}
	return operations, total, rows.Err()
}

func (s *SQLiteStore) UpdateOperationStatus(id, status string, progress, total int, message string) error {
	now := time.Now()
	var startedAt *time.Time
	var completedAt *time.Time

	if status == "running" {
		startedAt = &now
	} else if status == "completed" || status == "failed" {
		completedAt = &now
	}

	_, err := s.db.Exec(`UPDATE operations SET status = ?, progress = ?, total = ?,
		message = ?, started_at = COALESCE(started_at, ?), completed_at = ? WHERE id = ?`,
		status, progress, total, message, startedAt, completedAt, id)
	return err
}

func (s *SQLiteStore) UpdateOperationError(id, errorMessage string) error {
	_, err := s.db.Exec("UPDATE operations SET error_message = ?, status = 'failed' WHERE id = ?",
		errorMessage, id)
	return err
}

// Operation Log operations

func (s *SQLiteStore) AddOperationLog(operationID, level, message string, details *string) error {
	_, err := s.db.Exec(`INSERT INTO operation_logs (operation_id, level, message, details)
		VALUES (?, ?, ?, ?)`, operationID, level, message, details)
	return err
}

func (s *SQLiteStore) GetOperationLogs(operationID string) ([]OperationLog, error) {
	query := `SELECT id, operation_id, level, message, details, created_at
			  FROM operation_logs WHERE operation_id = ? ORDER BY created_at`
	rows, err := s.db.Query(query, operationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []OperationLog
	for rows.Next() {
		var log OperationLog
		if err := rows.Scan(&log.ID, &log.OperationID, &log.Level, &log.Message,
			&log.Details, &log.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

// ---- Operation Summary Logs (persistent across restarts) ----

func (s *SQLiteStore) SaveOperationSummaryLog(op *OperationSummaryLog) error {
	now := time.Now()
	_, err := s.db.Exec(`INSERT INTO operation_summary_logs (id, type, status, progress, result, error, created_at, updated_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET status=excluded.status, progress=excluded.progress,
		result=excluded.result, error=excluded.error, updated_at=excluded.updated_at,
		completed_at=excluded.completed_at`,
		op.ID, op.Type, op.Status, op.Progress, op.Result, op.Error, op.CreatedAt, now, op.CompletedAt)
	return err
}

func (s *SQLiteStore) GetOperationSummaryLog(id string) (*OperationSummaryLog, error) {
	var op OperationSummaryLog
	err := s.db.QueryRow(`SELECT id, type, status, progress, result, error, created_at, updated_at, completed_at
		FROM operation_summary_logs WHERE id = ?`, id).Scan(
		&op.ID, &op.Type, &op.Status, &op.Progress, &op.Result, &op.Error,
		&op.CreatedAt, &op.UpdatedAt, &op.CompletedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &op, nil
}

func (s *SQLiteStore) ListOperationSummaryLogs(limit, offset int) ([]OperationSummaryLog, error) {
	rows, err := s.db.Query(`SELECT id, type, status, progress, result, error, created_at, updated_at, completed_at
		FROM operation_summary_logs ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []OperationSummaryLog
	for rows.Next() {
		var op OperationSummaryLog
		if err := rows.Scan(&op.ID, &op.Type, &op.Status, &op.Progress, &op.Result, &op.Error,
			&op.CreatedAt, &op.UpdatedAt, &op.CompletedAt); err != nil {
			return nil, err
		}
		logs = append(logs, op)
	}
	return logs, rows.Err()
}

// ---- Operation Results (structured per-book output) ----

func (s *SQLiteStore) SetRaw(key string, value []byte) error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS kv_store (key TEXT PRIMARY KEY, value BLOB)`); err != nil {
		return fmt.Errorf("create kv_store table: %w", err)
	}
	_, err := s.db.Exec(`INSERT OR REPLACE INTO kv_store (key, value) VALUES (?, ?)`, key, value)
	return err
}

// GetRaw reads a single kv_store row. Returns (nil, nil) on miss so
// callers can handle cache-style lookups with a two-valued result
// rather than a sentinel error.
func (s *SQLiteStore) GetRaw(key string) ([]byte, error) {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS kv_store (key TEXT PRIMARY KEY, value BLOB)`); err != nil {
		return nil, fmt.Errorf("create kv_store table: %w", err)
	}
	var value []byte
	err := s.db.QueryRow(`SELECT value FROM kv_store WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return value, nil
}

func (s *SQLiteStore) DeleteRaw(key string) error {
	_, err := s.db.Exec(`DELETE FROM kv_store WHERE key = ?`, key)
	return err
}

func (s *SQLiteStore) ScanPrefix(prefix string) ([]KVPair, error) {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS kv_store (key TEXT PRIMARY KEY, value BLOB)`); err != nil {
		return nil, fmt.Errorf("create kv_store table: %w", err)
	}
	rows, err := s.db.Query(`SELECT key, value FROM kv_store WHERE key LIKE ?`, prefix+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pairs []KVPair
	for rows.Next() {
		var kv KVPair
		if err := rows.Scan(&kv.Key, &kv.Value); err != nil {
			continue
		}
		pairs = append(pairs, kv)
	}
	return pairs, nil
}

func (s *SQLiteStore) CountPrefix(prefix string) (int64, error) {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS kv_store (key TEXT PRIMARY KEY, value BLOB)`); err != nil {
		return 0, fmt.Errorf("create kv_store table: %w", err)
	}
	var n int64
	err := s.db.QueryRow(`SELECT COUNT(*) FROM kv_store WHERE key LIKE ?`, prefix+"%").Scan(&n)
	return n, err
}

func (s *SQLiteStore) CreateOperationResult(result *OperationResult) error {
	_, err := s.db.Exec(
		`INSERT INTO operation_results (operation_id, book_id, result_json, status) VALUES (?, ?, ?, ?)`,
		result.OperationID, result.BookID, result.ResultJSON, result.Status,
	)
	return err
}

func (s *SQLiteStore) GetOperationResults(operationID string) ([]OperationResult, error) {
	rows, err := s.db.Query(
		`SELECT id, operation_id, book_id, result_json, status, created_at FROM operation_results WHERE operation_id = ? ORDER BY id`,
		operationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []OperationResult
	for rows.Next() {
		var r OperationResult
		if err := rows.Scan(&r.ID, &r.OperationID, &r.BookID, &r.ResultJSON, &r.Status, &r.CreatedAt); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (s *SQLiteStore) GetOperationResultsPage(operationID string, limit, offset int) ([]OperationResult, int, error) {
	var total int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM operation_results WHERE operation_id = ?`, operationID,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `SELECT id, operation_id, book_id, result_json, status, created_at FROM operation_results WHERE operation_id = ? ORDER BY id`
	args := []any{operationID}
	if limit > 0 {
		query += ` LIMIT ? OFFSET ?`
		args = append(args, limit, offset)
	} else if offset > 0 {
		query += ` LIMIT -1 OFFSET ?`
		args = append(args, offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, total, err
	}
	defer rows.Close()

	var results []OperationResult
	for rows.Next() {
		var r OperationResult
		if err := rows.Scan(&r.ID, &r.OperationID, &r.BookID, &r.ResultJSON, &r.Status, &r.CreatedAt); err != nil {
			return nil, total, err
		}
		results = append(results, r)
	}
	return results, total, rows.Err()
}

func (s *SQLiteStore) GetRecentCompletedOperations(limit int) ([]Operation, error) {
	rows, err := s.db.Query(
		`SELECT id, type, status, progress, total, message, folder_path, created_at, started_at, completed_at, error_message, result_data
		 FROM operations WHERE status IN ('completed', 'failed') ORDER BY completed_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ops []Operation
	for rows.Next() {
		var op Operation
		if err := rows.Scan(&op.ID, &op.Type, &op.Status, &op.Progress, &op.Total, &op.Message, &op.FolderPath,
			&op.CreatedAt, &op.StartedAt, &op.CompletedAt, &op.ErrorMessage, &op.ResultData); err != nil {
			return nil, err
		}
		ops = append(ops, op)
	}
	return ops, rows.Err()
}

// ---- Operation State Persistence (resumable operations) ----

func (s *SQLiteStore) ensureOpStateTable() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS operation_state (
		op_id TEXT NOT NULL,
		key_suffix TEXT NOT NULL DEFAULT '',
		data BLOB NOT NULL,
		PRIMARY KEY (op_id, key_suffix)
	)`)
	return err
}

func (s *SQLiteStore) SaveOperationState(opID string, state []byte) error {
	if err := s.ensureOpStateTable(); err != nil {
		return fmt.Errorf("ensure operation state table: %w", err)
	}
	_, err := s.db.Exec(`INSERT INTO operation_state (op_id, key_suffix, data) VALUES (?, '', ?)
		ON CONFLICT(op_id, key_suffix) DO UPDATE SET data = ?`, opID, state, state)
	return err
}

func (s *SQLiteStore) GetOperationState(opID string) ([]byte, error) {
	if err := s.ensureOpStateTable(); err != nil {
		return nil, fmt.Errorf("ensure operation state table: %w", err)
	}
	var data []byte
	err := s.db.QueryRow(`SELECT data FROM operation_state WHERE op_id = ? AND key_suffix = ''`, opID).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return data, err
}

func (s *SQLiteStore) SaveOperationParams(opID string, params []byte) error {
	if err := s.ensureOpStateTable(); err != nil {
		return fmt.Errorf("ensure operation state table: %w", err)
	}
	_, err := s.db.Exec(`INSERT INTO operation_state (op_id, key_suffix, data) VALUES (?, 'params', ?)
		ON CONFLICT(op_id, key_suffix) DO UPDATE SET data = ?`, opID, params, params)
	return err
}

func (s *SQLiteStore) GetOperationParams(opID string) ([]byte, error) {
	if err := s.ensureOpStateTable(); err != nil {
		return nil, fmt.Errorf("ensure operation state table: %w", err)
	}
	var data []byte
	err := s.db.QueryRow(`SELECT data FROM operation_state WHERE op_id = ? AND key_suffix = 'params'`, opID).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return data, err
}

func (s *SQLiteStore) DeleteOperationState(opID string) error {
	if err := s.ensureOpStateTable(); err != nil {
		return fmt.Errorf("ensure operation state table: %w", err)
	}
	_, err := s.db.Exec(`DELETE FROM operation_state WHERE op_id = ?`, opID)
	return err
}

func (s *SQLiteStore) DeleteOperationsByStatus(statuses []string) (int, error) {
	if len(statuses) == 0 {
		return 0, nil
	}
	placeholders := ""
	args := make([]interface{}, len(statuses))
	for i, s := range statuses {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args[i] = s
	}
	result, err := s.db.Exec(`DELETE FROM operations WHERE status IN (`+placeholders+`)`, args...)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	// Also clean up summary logs
	if _, err := s.db.Exec(`DELETE FROM operation_summary_logs WHERE status IN (`+placeholders+`)`, args...); err != nil {
		return int(n), fmt.Errorf("delete operation summary logs: %w", err)
	}
	return int(n), nil
}

func (s *SQLiteStore) UpdateOperationResultData(id string, resultData string) error {
	_, err := s.db.Exec("UPDATE operations SET result_data = ? WHERE id = ?", resultData, id)
	return err
}

func (s *SQLiteStore) GetInterruptedOperations() ([]Operation, error) {
	query := `SELECT id, type, status, progress, total, message, folder_path,
		created_at, started_at, completed_at, error_message, result_data
		FROM operations WHERE status IN ('running', 'queued', 'interrupted')`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ops []Operation
	for rows.Next() {
		var op Operation
		if err := rows.Scan(&op.ID, &op.Type, &op.Status, &op.Progress, &op.Total,
			&op.Message, &op.FolderPath, &op.CreatedAt, &op.StartedAt,
			&op.CompletedAt, &op.ErrorMessage, &op.ResultData); err != nil {
			return nil, err
		}
		ops = append(ops, op)
	}
	return ops, rows.Err()
}

func (s *SQLiteStore) CreateOperationChange(change *OperationChange) error {
	if change.ID == "" {
		change.ID = ulid.Make().String()
	}
	_, err := s.db.Exec(
		`INSERT INTO operation_changes (id, operation_id, book_id, change_type, field_name, old_value, new_value, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		change.ID, change.OperationID, change.BookID, change.ChangeType, change.FieldName, change.OldValue, change.NewValue,
	)
	return err
}

// GetOperationChanges returns all changes for a given operation.
func (s *SQLiteStore) GetOperationChanges(operationID string) ([]*OperationChange, error) {
	rows, err := s.db.Query(
		`SELECT id, operation_id, book_id, change_type, field_name, old_value, new_value, reverted_at, created_at
		 FROM operation_changes WHERE operation_id = ? ORDER BY created_at ASC`, operationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanOperationChanges(rows)
}

// GetBookChanges returns all changes for a given book.
func (s *SQLiteStore) GetBookChanges(bookID string) ([]*OperationChange, error) {
	rows, err := s.db.Query(
		`SELECT id, operation_id, book_id, change_type, field_name, old_value, new_value, reverted_at, created_at
		 FROM operation_changes WHERE book_id = ? ORDER BY created_at DESC`, bookID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanOperationChanges(rows)
}

// RevertOperationChanges marks all changes for an operation as reverted.
func (s *SQLiteStore) RevertOperationChanges(operationID string) error {
	_, err := s.db.Exec(
		`UPDATE operation_changes SET reverted_at = CURRENT_TIMESTAMP WHERE operation_id = ? AND reverted_at IS NULL`,
		operationID,
	)
	return err
}

// CreateAuthorTombstone is a no-op for SQLite (uses SQL foreign keys instead).
func (s *SQLiteStore) CreateAuthorTombstone(oldID, canonicalID int) error {
	return nil
}

// GetAuthorTombstone is a no-op for SQLite (uses SQL foreign keys instead).
func (s *SQLiteStore) GetAuthorTombstone(oldID int) (int, error) {
	return 0, nil
}

// ResolveTombstoneChains is a no-op for SQLite (uses SQL foreign keys instead).
func (s *SQLiteStore) ResolveTombstoneChains() (int, error) {
	return 0, nil
}

// AddSystemActivityLog inserts a log entry from a housekeeping goroutine.
func (s *SQLiteStore) AddSystemActivityLog(source, level, message string) error {
	_, err := s.db.Exec(
		"INSERT INTO system_activity_log (source, level, message) VALUES (?, ?, ?)",
		source, level, message,
	)
	return err
}

// GetSystemActivityLogs retrieves recent system activity log entries.
func (s *SQLiteStore) GetSystemActivityLogs(source string, limit int) ([]SystemActivityLog, error) {
	query := "SELECT id, source, level, message, created_at FROM system_activity_log"
	args := []interface{}{}
	if source != "" {
		query += " WHERE source = ?"
		args = append(args, source)
	}
	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []SystemActivityLog
	for rows.Next() {
		var l SystemActivityLog
		if err := rows.Scan(&l.ID, &l.Source, &l.Level, &l.Message, &l.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

// GetAllSystemActivityLogRows retrieves all system activity log entries, ordered by created_at ASC.
// Used for one-time migration to the unified ActivityStore.
func (s *SQLiteStore) GetAllSystemActivityLogRows() ([]SystemActivityLog, error) {
	query := "SELECT id, source, level, message, created_at FROM system_activity_log ORDER BY created_at ASC"
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []SystemActivityLog
	for rows.Next() {
		var l SystemActivityLog
		if err := rows.Scan(&l.ID, &l.Source, &l.Level, &l.Message, &l.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

// PruneOperationLogs deletes operation log entries older than the given time.
func (s *SQLiteStore) PruneOperationLogs(olderThan time.Time) (int, error) {
	result, err := s.db.Exec("DELETE FROM operation_logs WHERE created_at < ?", olderThan)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// PruneOperationChanges deletes operation change entries older than the given time.
func (s *SQLiteStore) PruneOperationChanges(olderThan time.Time) (int, error) {
	result, err := s.db.Exec("DELETE FROM operation_changes WHERE created_at < ?", olderThan)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}
func (s *SQLiteStore) GetScanFailCount(pathHash string) (int, error) {
	key := "scan_fail:" + pathHash
	setting, err := s.GetSetting(key)
	if err != nil || setting == nil {
		return 0, nil
	}
	n := 0
	_, _ = fmt.Sscanf(setting.Value, "%d", &n)
	return n, nil
}

// IncrScanFailCount increments the scan-fail counter and returns the new count.
func (s *SQLiteStore) IncrScanFailCount(pathHash string) (int, error) {
	n, _ := s.GetScanFailCount(pathHash)
	n++
	key := "scan_fail:" + pathHash
	return n, s.SetSetting(key, fmt.Sprintf("%d", n), "int", false)
}

// ResetScanFailCount resets the scan-fail counter to zero.
func (s *SQLiteStore) ResetScanFailCount(pathHash string) error {
	key := "scan_fail:" + pathHash
	return s.SetSetting(key, "0", "int", false)
}

// MergeChapterBooks absorbs srcIDs into primaryID in a single transaction:
