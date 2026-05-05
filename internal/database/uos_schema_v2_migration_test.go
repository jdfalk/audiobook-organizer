// file: internal/database/uos_schema_v2_migration_test.go
// version: 1.0.0
// guid: 8f9a0b1c-2d3e-4f5a-9b0c-1d2e3f4a5b6c
// last-edited: 2026-05-05

package database

import "testing"

func TestMigration059UOSSchemaV2(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()
	sqliteStore := store.(*SQLiteStore)

	if err := migration059Up(store); err != nil {
		t.Fatalf("migration059Up failed: %v", err)
	}

	expectedTables := map[string]map[string]string{
		"op_definitions_v2": {
			"id":              "TEXT",
			"plugin":          "TEXT",
			"display_name":    "TEXT",
			"description":     "TEXT",
			"capabilities":    "TEXT",
			"permissions":     "TEXT",
			"cancellable":     "BOOLEAN",
			"isolate":         "BOOLEAN",
			"resume_policy":   "TEXT",
			"schedule_cron":   "TEXT",
			"triggers":        "TEXT",
			"depends_on":      "TEXT",
			"phases":          "TEXT",
			"timeout_seconds": "INTEGER",
			"registered_at":   "TIMESTAMP",
		},
		"operations_v2": {
			"id":                  "TEXT",
			"def_id":              "TEXT",
			"plugin":              "TEXT",
			"parent_id":           "TEXT",
			"actor_user_id":       "TEXT",
			"trace_id":            "TEXT",
			"span_id":             "TEXT",
			"parent_span_id":      "TEXT",
			"status":              "TEXT",
			"priority":            "INTEGER",
			"progress_current":    "INTEGER",
			"progress_total":      "INTEGER",
			"progress_message":    "TEXT",
			"current_phase":       "TEXT",
			"params":              "TEXT",
			"error_message":       "TEXT",
			"result_data":         "TEXT",
			"queued_at":           "TIMESTAMP",
			"started_at":          "TIMESTAMP",
			"completed_at":        "TIMESTAMP",
			"last_progress_at":    "TIMESTAMP",
			"last_checkpoint_at":  "TIMESTAMP",
			"high_water_progress": "INTEGER",
			"resume_count":        "INTEGER",
		},
		"op_logs_v2": {
			"id":           "INTEGER",
			"operation_id": "TEXT",
			"level":        "TEXT",
			"message":      "TEXT",
			"attrs":        "TEXT",
			"created_at":   "TIMESTAMP",
		},
		"op_errors_v2": {
			"id":           "INTEGER",
			"operation_id": "TEXT",
			"plugin":       "TEXT",
			"def_id":       "TEXT",
			"message":      "TEXT",
			"attrs":        "TEXT",
			"occurred_at":  "TIMESTAMP",
		},
		"op_state_v2": {
			"operation_id":   "TEXT",
			"phase":          "TEXT",
			"state_blob":     "BLOB",
			"schema_version": "INTEGER",
			"written_at":     "TIMESTAMP",
		},
		"op_strikes_v2": {
			"id":           "INTEGER",
			"def_id":       "TEXT",
			"operation_id": "TEXT",
			"kind":         "TEXT",
			"details":      "TEXT",
			"occurred_at":  "TIMESTAMP",
		},
		"plugin_schema_v2": {
			"plugin":            "TEXT",
			"migration_version": "INTEGER",
			"applied_at":        "TIMESTAMP",
		},
		"core_schema_meta_v2": {
			"id":                  "INTEGER",
			"core_schema_version": "INTEGER",
		},
	}

	for tableName, expectedColumns := range expectedTables {
		actualColumns := tableColumns(t, sqliteStore, tableName)
		if len(actualColumns) != len(expectedColumns) {
			t.Fatalf("%s: expected %d columns, got %d (%v)", tableName, len(expectedColumns), len(actualColumns), actualColumns)
		}
		for columnName, expectedType := range expectedColumns {
			actualType, ok := actualColumns[columnName]
			if !ok {
				t.Fatalf("%s: missing column %s", tableName, columnName)
			}
			if actualType != expectedType {
				t.Fatalf("%s.%s: expected type %s, got %s", tableName, columnName, expectedType, actualType)
			}
		}
	}

	expectedIndexes := []string{
		"idx_operations_v2_status",
		"idx_operations_v2_parent",
		"idx_operations_v2_def",
		"idx_op_logs_v2_op_time",
		"idx_op_errors_v2_def",
		"idx_op_errors_v2_plugin",
		"idx_op_strikes_v2_def_time",
	}
	for _, indexName := range expectedIndexes {
		if !sqliteObjectExists(t, sqliteStore, "index", indexName) {
			t.Fatalf("expected index %s to exist", indexName)
		}
	}

	var coreSchemaVersion int
	if err := sqliteStore.db.QueryRow(`SELECT core_schema_version FROM core_schema_meta_v2 WHERE id = 1`).Scan(&coreSchemaVersion); err != nil {
		t.Fatalf("core_schema_meta_v2 seed row missing: %v", err)
	}
	if coreSchemaVersion != 1 {
		t.Fatalf("expected core_schema_version 1, got %d", coreSchemaVersion)
	}

	if _, err := sqliteStore.db.Exec(`DELETE FROM core_schema_meta_v2`); err != nil {
		t.Fatalf("failed to clear core_schema_meta_v2 for check test: %v", err)
	}
	if _, err := sqliteStore.db.Exec(`INSERT INTO core_schema_meta_v2 (id, core_schema_version) VALUES (1, 1)`); err != nil {
		t.Fatalf("expected first core_schema_meta_v2 insert to succeed: %v", err)
	}
	if _, err := sqliteStore.db.Exec(`INSERT INTO core_schema_meta_v2 (id, core_schema_version) VALUES (2, 1)`); err == nil {
		t.Fatal("expected second core_schema_meta_v2 insert to fail")
	}

	if err := migration059Down(store); err != nil {
		t.Fatalf("migration059Down failed: %v", err)
	}
	for tableName := range expectedTables {
		if sqliteObjectExists(t, sqliteStore, "table", tableName) {
			t.Fatalf("expected table %s to be dropped", tableName)
		}
	}
	for _, indexName := range expectedIndexes {
		if sqliteObjectExists(t, sqliteStore, "index", indexName) {
			t.Fatalf("expected index %s to be dropped", indexName)
		}
	}
}

func tableColumns(t *testing.T, store *SQLiteStore, tableName string) map[string]string {
	t.Helper()

	rows, err := store.db.Query(`PRAGMA table_info(` + tableName + `)`)
	if err != nil {
		t.Fatalf("failed to inspect %s columns: %v", tableName, err)
	}
	defer rows.Close()

	columns := make(map[string]string)
	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal any
			pk         int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &pk); err != nil {
			t.Fatalf("failed to scan %s column info: %v", tableName, err)
		}
		columns[name] = columnType
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("failed reading %s column info: %v", tableName, err)
	}
	return columns
}

func sqliteObjectExists(t *testing.T, store *SQLiteStore, objectType, objectName string) bool {
	t.Helper()

	var count int
	if err := store.db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = ? AND name = ?`,
		objectType,
		objectName,
	).Scan(&count); err != nil {
		t.Fatalf("failed to inspect sqlite_master for %s %s: %v", objectType, objectName, err)
	}
	return count > 0
}
