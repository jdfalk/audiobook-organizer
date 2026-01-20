// file: internal/database/interface.go
// version: 1.0.0
// guid: 1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d

package database

import "database/sql"

// DBInterface defines the interface for database operations
// This allows for mocking in tests and decouples code from the global DB variable
type DBInterface interface {
	// Query executes a query that returns rows
	Query(query string, args ...interface{}) (*sql.Rows, error)

	// QueryRow executes a query that is expected to return at most one row
	QueryRow(query string, args ...interface{}) *sql.Row

	// Exec executes a query without returning any rows
	Exec(query string, args ...interface{}) (sql.Result, error)

	// Prepare creates a prepared statement for later queries or executions
	Prepare(query string) (*sql.Stmt, error)

	// Begin starts a transaction
	Begin() (*sql.Tx, error)

	// Close closes the database connection
	Close() error
}

// sqlDBWrapper wraps a *sql.DB to implement DBInterface
type sqlDBWrapper struct {
	db *sql.DB
}

// NewDBInterface creates a DBInterface from a *sql.DB
func NewDBInterface(db *sql.DB) DBInterface {
	return &sqlDBWrapper{db: db}
}

// Query executes a query that returns rows
func (w *sqlDBWrapper) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return w.db.Query(query, args...)
}

// QueryRow executes a query that is expected to return at most one row
func (w *sqlDBWrapper) QueryRow(query string, args ...interface{}) *sql.Row {
	return w.db.QueryRow(query, args...)
}

// Exec executes a query without returning any rows
func (w *sqlDBWrapper) Exec(query string, args ...interface{}) (sql.Result, error) {
	return w.db.Exec(query, args...)
}

// Prepare creates a prepared statement for later queries or executions
func (w *sqlDBWrapper) Prepare(query string) (*sql.Stmt, error) {
	return w.db.Prepare(query)
}

// Begin starts a transaction
func (w *sqlDBWrapper) Begin() (*sql.Tx, error) {
	return w.db.Begin()
}

// Close closes the database connection
func (w *sqlDBWrapper) Close() error {
	return w.db.Close()
}

// GetDBInterface returns a DBInterface wrapping the global DB
// This provides a migration path from the global DB to the interface
func GetDBInterface() DBInterface {
	if DB == nil {
		return nil
	}
	return NewDBInterface(DB)
}
