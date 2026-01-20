// file: internal/database/mock_test.go
// version: 1.0.0
// guid: 3c4d5e6f-7a8b-9c0d-1e2f-3a4b5c6d7e8f

package database

import (
	"database/sql"
	"errors"
	"testing"
)

func TestNewMockDB(t *testing.T) {
	mock := NewMockDB()
	if mock == nil {
		t.Fatal("NewMockDB returned nil")
	}
	if mock.QueryCalls == nil {
		t.Error("QueryCalls not initialized")
	}
	if mock.ExecCalls == nil {
		t.Error("ExecCalls not initialized")
	}
}

func TestMockDB_Exec(t *testing.T) {
	mock := NewMockDB()

	// Test default behavior (returns success)
	result, err := mock.Exec("INSERT INTO test VALUES (?)", "value1")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if result == nil {
		t.Fatal("Expected result, got nil")
	}

	// Verify call was recorded
	if len(mock.ExecCalls) != 1 {
		t.Errorf("Expected 1 exec call, got %d", len(mock.ExecCalls))
	}
	if mock.ExecCalls[0].Query != "INSERT INTO test VALUES (?)" {
		t.Errorf("Expected query 'INSERT INTO test VALUES (?)', got '%s'", mock.ExecCalls[0].Query)
	}

	// Test custom ExecFunc
	mock.Reset()
	mock.ExecFunc = func(query string, args ...interface{}) (sql.Result, error) {
		return &MockResult{LastID: 42, AffectedRows: 10}, nil
	}
	result, err = mock.Exec("UPDATE test SET val=?", "newval")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	lastID, _ := result.LastInsertId()
	if lastID != 42 {
		t.Errorf("Expected LastID 42, got %d", lastID)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected != 10 {
		t.Errorf("Expected RowsAffected 10, got %d", rowsAffected)
	}
}

func TestMockDB_ExecError(t *testing.T) {
	mock := NewMockDB()
	mock.ErrorMode = "exec_error"

	_, err := mock.Exec("INSERT INTO test VALUES (?)", "value")
	if err == nil {
		t.Error("Expected error, got nil")
	}
	if err.Error() != "mock exec error" {
		t.Errorf("Expected 'mock exec error', got '%s'", err.Error())
	}
}

func TestMockDB_Query(t *testing.T) {
	mock := NewMockDB()

	// Test with custom QueryFunc
	mock.QueryFunc = func(query string, args ...interface{}) (*sql.Rows, error) {
		// Return nil rows for test (in real usage, would return actual rows)
		return nil, nil
	}

	_, err := mock.Query("SELECT * FROM test WHERE id=?", 1)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Verify call was recorded
	if len(mock.QueryCalls) != 1 {
		t.Errorf("Expected 1 query call, got %d", len(mock.QueryCalls))
	}
	if mock.QueryCalls[0].Query != "SELECT * FROM test WHERE id=?" {
		t.Errorf("Expected query 'SELECT * FROM test WHERE id=?', got '%s'", mock.QueryCalls[0].Query)
	}
	if len(mock.QueryCalls[0].Args) != 1 || mock.QueryCalls[0].Args[0] != 1 {
		t.Errorf("Expected args [1], got %v", mock.QueryCalls[0].Args)
	}
}

func TestMockDB_QueryError(t *testing.T) {
	mock := NewMockDB()
	mock.ErrorMode = "query_error"

	_, err := mock.Query("SELECT * FROM test")
	if err == nil {
		t.Error("Expected error, got nil")
	}
	if err.Error() != "mock query error" {
		t.Errorf("Expected 'mock query error', got '%s'", err.Error())
	}
}

func TestMockDB_QueryRow(t *testing.T) {
	mock := NewMockDB()

	// Test QueryRow recording
	_ = mock.QueryRow("SELECT * FROM test WHERE id=?", 1)

	// Verify call was recorded
	if len(mock.QueryRowCalls) != 1 {
		t.Errorf("Expected 1 query row call, got %d", len(mock.QueryRowCalls))
	}
	if mock.QueryRowCalls[0].Query != "SELECT * FROM test WHERE id=?" {
		t.Errorf("Expected query 'SELECT * FROM test WHERE id=?', got '%s'", mock.QueryRowCalls[0].Query)
	}
}

func TestMockDB_Prepare(t *testing.T) {
	mock := NewMockDB()
	mock.PrepareFunc = func(query string) (*sql.Stmt, error) {
		return nil, nil
	}

	_, err := mock.Prepare("SELECT * FROM test WHERE id=?")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Verify call was recorded
	if len(mock.PrepareCalls) != 1 {
		t.Errorf("Expected 1 prepare call, got %d", len(mock.PrepareCalls))
	}
}

func TestMockDB_PrepareError(t *testing.T) {
	mock := NewMockDB()
	mock.ErrorMode = "prepare_error"

	_, err := mock.Prepare("SELECT * FROM test")
	if err == nil {
		t.Error("Expected error, got nil")
	}
}

func TestMockDB_Begin(t *testing.T) {
	mock := NewMockDB()
	mock.BeginFunc = func() (*sql.Tx, error) {
		return nil, nil
	}

	_, err := mock.Begin()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Verify call was recorded
	if len(mock.BeginCalls) != 1 {
		t.Errorf("Expected 1 begin call, got %d", len(mock.BeginCalls))
	}
}

func TestMockDB_BeginError(t *testing.T) {
	mock := NewMockDB()
	mock.ErrorMode = "begin_error"

	_, err := mock.Begin()
	if err == nil {
		t.Error("Expected error, got nil")
	}
}

func TestMockDB_Close(t *testing.T) {
	mock := NewMockDB()

	err := mock.Close()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Verify call was recorded
	if mock.CloseCalls != 1 {
		t.Errorf("Expected 1 close call, got %d", mock.CloseCalls)
	}

	// Close again
	err = mock.Close()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if mock.CloseCalls != 2 {
		t.Errorf("Expected 2 close calls, got %d", mock.CloseCalls)
	}
}

func TestMockDB_CloseError(t *testing.T) {
	mock := NewMockDB()
	mock.ErrorMode = "close_error"

	err := mock.Close()
	if err == nil {
		t.Error("Expected error, got nil")
	}
}

func TestMockDB_Reset(t *testing.T) {
	mock := NewMockDB()

	// Make some calls
	mock.ExecFunc = func(query string, args ...interface{}) (sql.Result, error) {
		return &MockResult{}, nil
	}
	mock.Exec("INSERT INTO test VALUES (?)", "value")
	mock.QueryRow("SELECT * FROM test")
	mock.Close()
	mock.ErrorMode = "exec_error"

	// Verify calls recorded
	if len(mock.ExecCalls) != 1 {
		t.Error("Expected exec calls before reset")
	}
	if len(mock.QueryRowCalls) != 1 {
		t.Error("Expected query row calls before reset")
	}
	if mock.CloseCalls != 1 {
		t.Error("Expected close calls before reset")
	}

	// Reset
	mock.Reset()

	// Verify reset
	if len(mock.ExecCalls) != 0 {
		t.Errorf("Expected 0 exec calls after reset, got %d", len(mock.ExecCalls))
	}
	if len(mock.QueryRowCalls) != 0 {
		t.Errorf("Expected 0 query row calls after reset, got %d", len(mock.QueryRowCalls))
	}
	if mock.CloseCalls != 0 {
		t.Errorf("Expected 0 close calls after reset, got %d", mock.CloseCalls)
	}
	if mock.ErrorMode != "" {
		t.Errorf("Expected empty error mode after reset, got '%s'", mock.ErrorMode)
	}
}

func TestMockDB_GetQueryCallCount(t *testing.T) {
	mock := NewMockDB()
	mock.QueryFunc = func(query string, args ...interface{}) (*sql.Rows, error) {
		return nil, nil
	}

	if mock.GetQueryCallCount() != 0 {
		t.Error("Expected 0 query calls initially")
	}

	mock.Query("SELECT 1")
	mock.Query("SELECT 2")
	mock.Query("SELECT 3")

	if mock.GetQueryCallCount() != 3 {
		t.Errorf("Expected 3 query calls, got %d", mock.GetQueryCallCount())
	}
}

func TestMockDB_GetExecCallCount(t *testing.T) {
	mock := NewMockDB()

	if mock.GetExecCallCount() != 0 {
		t.Error("Expected 0 exec calls initially")
	}

	mock.Exec("INSERT INTO test VALUES (1)")
	mock.Exec("INSERT INTO test VALUES (2)")

	if mock.GetExecCallCount() != 2 {
		t.Errorf("Expected 2 exec calls, got %d", mock.GetExecCallCount())
	}
}

func TestMockDB_GetLastQuery(t *testing.T) {
	mock := NewMockDB()
	mock.QueryFunc = func(query string, args ...interface{}) (*sql.Rows, error) {
		return nil, nil
	}

	// Test with no calls
	_, _, err := mock.GetLastQuery()
	if err == nil {
		t.Error("Expected error when no queries, got nil")
	}

	// Make some calls
	mock.Query("SELECT 1")
	mock.Query("SELECT * FROM test WHERE id=?", 42)

	query, args, err := mock.GetLastQuery()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if query != "SELECT * FROM test WHERE id=?" {
		t.Errorf("Expected 'SELECT * FROM test WHERE id=?', got '%s'", query)
	}
	if len(args) != 1 || args[0] != 42 {
		t.Errorf("Expected args [42], got %v", args)
	}
}

func TestMockDB_GetLastExec(t *testing.T) {
	mock := NewMockDB()

	// Test with no calls
	_, _, err := mock.GetLastExec()
	if err == nil {
		t.Error("Expected error when no execs, got nil")
	}

	// Make some calls
	mock.Exec("INSERT INTO test VALUES (1)")
	mock.Exec("UPDATE test SET val=? WHERE id=?", "newval", 5)

	query, args, err := mock.GetLastExec()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if query != "UPDATE test SET val=? WHERE id=?" {
		t.Errorf("Expected 'UPDATE test SET val=? WHERE id=?', got '%s'", query)
	}
	if len(args) != 2 || args[0] != "newval" || args[1] != 5 {
		t.Errorf("Expected args ['newval', 5], got %v", args)
	}
}

func TestMockDB_VerifyQuery(t *testing.T) {
	mock := NewMockDB()
	mock.QueryFunc = func(query string, args ...interface{}) (*sql.Rows, error) {
		return nil, nil
	}

	mock.Query("SELECT * FROM test")
	mock.Query("SELECT * FROM users")

	// Verify existing query
	err := mock.VerifyQuery("SELECT * FROM test")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Verify non-existent query
	err = mock.VerifyQuery("SELECT * FROM nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent query, got nil")
	}
}

func TestMockDB_VerifyExec(t *testing.T) {
	mock := NewMockDB()

	mock.Exec("INSERT INTO test VALUES (1)")
	mock.Exec("UPDATE test SET val='x'")

	// Verify existing exec
	err := mock.VerifyExec("INSERT INTO test VALUES (1)")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Verify non-existent exec
	err = mock.VerifyExec("DELETE FROM test")
	if err == nil {
		t.Error("Expected error for non-existent exec, got nil")
	}
}

func TestMockResult(t *testing.T) {
	result := &MockResult{
		LastID:       100,
		AffectedRows: 5,
	}

	lastID, err := result.LastInsertId()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if lastID != 100 {
		t.Errorf("Expected LastID 100, got %d", lastID)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if rowsAffected != 5 {
		t.Errorf("Expected RowsAffected 5, got %d", rowsAffected)
	}
}

func TestMockResult_WithError(t *testing.T) {
	testErr := errors.New("test error")
	result := &MockResult{
		LastID:       0,
		AffectedRows: 0,
		Err:          testErr,
	}

	_, err := result.LastInsertId()
	if err != testErr {
		t.Errorf("Expected test error, got %v", err)
	}

	_, err = result.RowsAffected()
	if err != testErr {
		t.Errorf("Expected test error, got %v", err)
	}
}

func TestMockDB_ConcurrentAccess(t *testing.T) {
	mock := NewMockDB()
	mock.QueryFunc = func(query string, args ...interface{}) (*sql.Rows, error) {
		return nil, nil
	}

	// Test concurrent access (should not panic or race)
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			mock.Query("SELECT ?", id)
			mock.Exec("INSERT INTO test VALUES (?)", id)
			mock.QueryRow("SELECT * FROM test WHERE id=?", id)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all calls recorded
	if mock.GetQueryCallCount() != 10 {
		t.Errorf("Expected 10 query calls, got %d", mock.GetQueryCallCount())
	}
	if mock.GetExecCallCount() != 10 {
		t.Errorf("Expected 10 exec calls, got %d", mock.GetExecCallCount())
	}
	if len(mock.QueryRowCalls) != 10 {
		t.Errorf("Expected 10 query row calls, got %d", len(mock.QueryRowCalls))
	}
}
