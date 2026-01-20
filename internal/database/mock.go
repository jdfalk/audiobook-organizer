// file: internal/database/mock.go
// version: 1.0.0
// guid: 2b3c4d5e-6f7a-8b9c-0d1e-2f3a4b5c6d7e

package database

import (
	"database/sql"
	"errors"
	"fmt"
	"sync"
)

// MockDB is a mock implementation of DBInterface for testing
type MockDB struct {
	mu sync.RWMutex

	// QueryFunc allows tests to override Query behavior
	QueryFunc func(query string, args ...interface{}) (*sql.Rows, error)

	// QueryRowFunc allows tests to override QueryRow behavior
	QueryRowFunc func(query string, args ...interface{}) *sql.Row

	// ExecFunc allows tests to override Exec behavior
	ExecFunc func(query string, args ...interface{}) (sql.Result, error)

	// PrepareFunc allows tests to override Prepare behavior
	PrepareFunc func(query string) (*sql.Stmt, error)

	// BeginFunc allows tests to override Begin behavior
	BeginFunc func() (*sql.Tx, error)

	// CloseFunc allows tests to override Close behavior
	CloseFunc func() error

	// QueryCalls tracks all Query calls
	QueryCalls []MockCall

	// QueryRowCalls tracks all QueryRow calls
	QueryRowCalls []MockCall

	// ExecCalls tracks all Exec calls
	ExecCalls []MockCall

	// PrepareCalls tracks all Prepare calls
	PrepareCalls []MockCall

	// BeginCalls tracks all Begin calls
	BeginCalls []MockCall

	// CloseCalls tracks all Close calls
	CloseCalls int

	// ErrorMode can be set to trigger specific error scenarios
	ErrorMode string
}

// MockCall represents a recorded database call
type MockCall struct {
	Query string
	Args  []interface{}
}

// MockResult implements sql.Result for testing
type MockResult struct {
	LastID          int64
	AffectedRows    int64
	Err             error
}

// LastInsertId returns the last insert ID
func (r *MockResult) LastInsertId() (int64, error) {
	return r.LastID, r.Err
}

// RowsAffected returns the number of rows affected
func (r *MockResult) RowsAffected() (int64, error) {
	return r.AffectedRows, r.Err
}

// NewMockDB creates a new mock database
func NewMockDB() *MockDB {
	return &MockDB{
		QueryCalls:    make([]MockCall, 0),
		QueryRowCalls: make([]MockCall, 0),
		ExecCalls:     make([]MockCall, 0),
		PrepareCalls:  make([]MockCall, 0),
		BeginCalls:    make([]MockCall, 0),
	}
}

// Query executes a query and records the call
func (m *MockDB) Query(query string, args ...interface{}) (*sql.Rows, error) {
	m.mu.Lock()
	m.QueryCalls = append(m.QueryCalls, MockCall{Query: query, Args: args})
	m.mu.Unlock()

	if m.ErrorMode == "query_error" {
		return nil, errors.New("mock query error")
	}

	if m.QueryFunc != nil {
		return m.QueryFunc(query, args...)
	}

	return nil, errors.New("QueryFunc not set - must provide QueryFunc for mock behavior")
}

// QueryRow executes a query and records the call
func (m *MockDB) QueryRow(query string, args ...interface{}) *sql.Row {
	m.mu.Lock()
	m.QueryRowCalls = append(m.QueryRowCalls, MockCall{Query: query, Args: args})
	m.mu.Unlock()

	if m.QueryRowFunc != nil {
		return m.QueryRowFunc(query, args...)
	}

	// Return a row from an in-memory database for testing
	// Tests should set QueryRowFunc for specific behavior
	return nil
}

// Exec executes a command and records the call
func (m *MockDB) Exec(query string, args ...interface{}) (sql.Result, error) {
	m.mu.Lock()
	m.ExecCalls = append(m.ExecCalls, MockCall{Query: query, Args: args})
	m.mu.Unlock()

	if m.ErrorMode == "exec_error" {
		return nil, errors.New("mock exec error")
	}

	if m.ExecFunc != nil {
		return m.ExecFunc(query, args...)
	}

	// Default: return success with 1 row affected
	return &MockResult{LastID: 1, AffectedRows: 1}, nil
}

// Prepare prepares a statement and records the call
func (m *MockDB) Prepare(query string) (*sql.Stmt, error) {
	m.mu.Lock()
	m.PrepareCalls = append(m.PrepareCalls, MockCall{Query: query})
	m.mu.Unlock()

	if m.ErrorMode == "prepare_error" {
		return nil, errors.New("mock prepare error")
	}

	if m.PrepareFunc != nil {
		return m.PrepareFunc(query)
	}

	return nil, errors.New("PrepareFunc not set")
}

// Begin starts a transaction and records the call
func (m *MockDB) Begin() (*sql.Tx, error) {
	m.mu.Lock()
	m.BeginCalls = append(m.BeginCalls, MockCall{})
	m.mu.Unlock()

	if m.ErrorMode == "begin_error" {
		return nil, errors.New("mock begin error")
	}

	if m.BeginFunc != nil {
		return m.BeginFunc()
	}

	return nil, errors.New("BeginFunc not set")
}

// Close closes the database and records the call
func (m *MockDB) Close() error {
	m.mu.Lock()
	m.CloseCalls++
	m.mu.Unlock()

	if m.ErrorMode == "close_error" {
		return errors.New("mock close error")
	}

	if m.CloseFunc != nil {
		return m.CloseFunc()
	}

	return nil
}

// Reset clears all recorded calls and resets state
func (m *MockDB) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.QueryCalls = make([]MockCall, 0)
	m.QueryRowCalls = make([]MockCall, 0)
	m.ExecCalls = make([]MockCall, 0)
	m.PrepareCalls = make([]MockCall, 0)
	m.BeginCalls = make([]MockCall, 0)
	m.CloseCalls = 0
	m.ErrorMode = ""
}

// GetQueryCallCount returns the number of Query calls
func (m *MockDB) GetQueryCallCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.QueryCalls)
}

// GetExecCallCount returns the number of Exec calls
func (m *MockDB) GetExecCallCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.ExecCalls)
}

// GetLastQuery returns the last Query call
func (m *MockDB) GetLastQuery() (string, []interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.QueryCalls) == 0 {
		return "", nil, errors.New("no queries recorded")
	}
	last := m.QueryCalls[len(m.QueryCalls)-1]
	return last.Query, last.Args, nil
}

// GetLastExec returns the last Exec call
func (m *MockDB) GetLastExec() (string, []interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.ExecCalls) == 0 {
		return "", nil, errors.New("no exec calls recorded")
	}
	last := m.ExecCalls[len(m.ExecCalls)-1]
	return last.Query, last.Args, nil
}

// VerifyQuery checks if a specific query was called
func (m *MockDB) VerifyQuery(expectedQuery string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, call := range m.QueryCalls {
		if call.Query == expectedQuery {
			return nil
		}
	}
	return fmt.Errorf("query not found: %s", expectedQuery)
}

// VerifyExec checks if a specific exec was called
func (m *MockDB) VerifyExec(expectedQuery string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, call := range m.ExecCalls {
		if call.Query == expectedQuery {
			return nil
		}
	}
	return fmt.Errorf("exec not found: %s", expectedQuery)
}
