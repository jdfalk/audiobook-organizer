// file: internal/database/web.go
// version: 1.1.1
// guid: 5d6e7f8a-9b0c-1d2e-3f4a-5b6c7d8e9f0a

package database

import (
	"database/sql"
	"time"
)

// Note: Type definitions for ImportPath, Operation, OperationLog, and UserPreference
// have been moved to store.go to avoid circular dependencies

// Import path operations

// GetImportPaths returns all import paths
func GetImportPaths() ([]ImportPath, error) {
	query := `
		SELECT id, path, name, enabled, created_at, last_scan, book_count
		FROM import_paths
		ORDER BY name
	`
	rows, err := DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var folders []ImportPath
	for rows.Next() {
		var folder ImportPath
		err := rows.Scan(
			&folder.ID, &folder.Path, &folder.Name, &folder.Enabled,
			&folder.CreatedAt, &folder.LastScan, &folder.BookCount,
		)
		if err != nil {
			return nil, err
		}
		folders = append(folders, folder)
	}

	return folders, rows.Err()
}

// AddImportPath adds a new import path
func AddImportPath(path, name string) (*ImportPath, error) {
	query := `
		INSERT INTO import_paths (path, name)
		VALUES (?, ?)
	`
	result, err := DB.Exec(query, path, name)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return GetImportPathByID(int(id))
}

// GetImportPathByID returns an import path by ID
func GetImportPathByID(id int) (*ImportPath, error) {
	query := `
		SELECT id, path, name, enabled, created_at, last_scan, book_count
		FROM import_paths
		WHERE id = ?
	`
	row := DB.QueryRow(query, id)

	var folder ImportPath
	err := row.Scan(
		&folder.ID, &folder.Path, &folder.Name, &folder.Enabled,
		&folder.CreatedAt, &folder.LastScan, &folder.BookCount,
	)
	if err != nil {
		return nil, err
	}

	return &folder, nil
}

// UpdateImportPath updates an existing import path
func UpdateImportPath(id int, enabled bool, lastScan *time.Time, bookCount int) error {
	query := `
		UPDATE import_paths
		SET enabled = ?, last_scan = ?, book_count = ?
		WHERE id = ?
	`
	_, err := DB.Exec(query, enabled, lastScan, bookCount, id)
	return err
}

// RemoveImportPath removes an import path
func RemoveImportPath(id int) error {
	query := `DELETE FROM import_paths WHERE id = ?`
	_, err := DB.Exec(query, id)
	return err
}

// Operation operations

// CreateOperation creates a new operation
func CreateOperation(id, opType, folderPath string) (*Operation, error) {
	query := `
		INSERT INTO operations (id, type, folder_path)
		VALUES (?, ?, ?)
	`
	_, err := DB.Exec(query, id, opType, folderPath)
	if err != nil {
		return nil, err
	}

	return GetOperationByID(id)
}

// GetOperationByID returns an operation by ID
func GetOperationByID(id string) (*Operation, error) {
	query := `
		SELECT id, type, status, progress, total, message, folder_path,
		       created_at, started_at, completed_at, error_message
		FROM operations
		WHERE id = ?
	`
	row := DB.QueryRow(query, id)

	var op Operation
	err := row.Scan(
		&op.ID, &op.Type, &op.Status, &op.Progress, &op.Total, &op.Message,
		&op.FolderPath, &op.CreatedAt, &op.StartedAt, &op.CompletedAt, &op.ErrorMessage,
	)
	if err != nil {
		return nil, err
	}

	return &op, nil
}

// UpdateOperationStatus updates an operation's status
func UpdateOperationStatus(id, status string, progress, total int, message string) error {
	var query string
	var args []interface{}

	if status == "running" && progress == 0 {
		// Starting the operation
		query = `
			UPDATE operations
			SET status = ?, progress = ?, total = ?, message = ?, started_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`
		args = []interface{}{status, progress, total, message, id}
	} else if status == "completed" || status == "failed" {
		// Completing the operation
		query = `
			UPDATE operations
			SET status = ?, progress = ?, total = ?, message = ?, completed_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`
		args = []interface{}{status, progress, total, message, id}
	} else {
		// Regular progress update
		query = `
			UPDATE operations
			SET status = ?, progress = ?, total = ?, message = ?
			WHERE id = ?
		`
		args = []interface{}{status, progress, total, message, id}
	}

	_, err := DB.Exec(query, args...)
	return err
}

// UpdateOperationError updates an operation with an error
func UpdateOperationError(id, errorMessage string) error {
	query := `
		UPDATE operations
		SET status = 'failed', error_message = ?, completed_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`
	_, err := DB.Exec(query, errorMessage, id)
	return err
}

// GetRecentOperations returns recent operations
func GetRecentOperations(limit int) ([]Operation, error) {
	query := `
		SELECT id, type, status, progress, total, message, folder_path,
		       created_at, started_at, completed_at, error_message
		FROM operations
		ORDER BY created_at DESC
		LIMIT ?
	`
	rows, err := DB.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var operations []Operation
	for rows.Next() {
		var op Operation
		err := rows.Scan(
			&op.ID, &op.Type, &op.Status, &op.Progress, &op.Total, &op.Message,
			&op.FolderPath, &op.CreatedAt, &op.StartedAt, &op.CompletedAt, &op.ErrorMessage,
		)
		if err != nil {
			return nil, err
		}
		operations = append(operations, op)
	}

	return operations, rows.Err()
}

// Operation log operations

// AddOperationLog adds a log entry for an operation
func AddOperationLog(operationID, level, message string, details *string) error {
	query := `
		INSERT INTO operation_logs (operation_id, level, message, details)
		VALUES (?, ?, ?, ?)
	`
	_, err := DB.Exec(query, operationID, level, message, details)
	return err
}

// GetOperationLogs returns logs for an operation
func GetOperationLogs(operationID string) ([]OperationLog, error) {
	query := `
		SELECT id, operation_id, level, message, details, created_at
		FROM operation_logs
		WHERE operation_id = ?
		ORDER BY created_at
	`
	rows, err := DB.Query(query, operationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []OperationLog
	for rows.Next() {
		var log OperationLog
		err := rows.Scan(
			&log.ID, &log.OperationID, &log.Level, &log.Message,
			&log.Details, &log.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}

	return logs, rows.Err()
}

// User preference operations

// GetUserPreference gets a user preference value
func GetUserPreference(key string) (*UserPreference, error) {
	query := `
		SELECT id, key, value, updated_at
		FROM user_preferences
		WHERE key = ?
	`
	row := DB.QueryRow(query, key)

	var pref UserPreference
	err := row.Scan(&pref.ID, &pref.Key, &pref.Value, &pref.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Not found, not an error
		}
		return nil, err
	}

	return &pref, nil
}

// SetUserPreference sets a user preference value
func SetUserPreference(key, value string) error {
	query := `
		INSERT OR REPLACE INTO user_preferences (key, value, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
	`
	_, err := DB.Exec(query, key, value)
	return err
}

// GetAllUserPreferences returns all user preferences
func GetAllUserPreferences() ([]UserPreference, error) {
	query := `
		SELECT id, key, value, updated_at
		FROM user_preferences
		ORDER BY key
	`
	rows, err := DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var preferences []UserPreference
	for rows.Next() {
		var pref UserPreference
		err := rows.Scan(&pref.ID, &pref.Key, &pref.Value, &pref.UpdatedAt)
		if err != nil {
			return nil, err
		}
		preferences = append(preferences, pref)
	}

	return preferences, rows.Err()
}
