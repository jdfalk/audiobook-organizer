package database

// version: 1.0.0
// guid: b2c3d4e5-f6a7-48b9-c0d1-e2f3g4h5i6j7
// last-edited: 2026-05-24

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/cockroachdb/pebble/v2"
	_ "github.com/chaisql/chai"
)

// ChaiDB wraps a Chai database instance with schema management
// It provides a minimal integration layer for initializing Chai with a Pebble backend
type ChaiDB struct {
	db     *sql.DB
	schema *ChaiSchema
	path   string
}

// NewChaiDB creates and initializes a new Chai database with Pebble backend
// The pebbleDB parameter is passed to enable future integration
// (currently Chai uses its own embedded storage, but this allows for future consolidation)
func NewChaiDB(ctx context.Context, dbPath string) (*ChaiDB, error) {
	if dbPath == "" {
		return nil, fmt.Errorf("database path cannot be empty")
	}

	// Open Chai database at the specified path
	// Chai uses sql.Open with "chai" driver and a file path
	sqlDB, err := sql.Open("chai", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open chai database at %s: %w", dbPath, err)
	}

	// Verify the connection is working
	if err := sqlDB.PingContext(ctx); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("failed to ping chai database: %w", err)
	}

	// Create the wrapper
	chaiDB := &ChaiDB{
		db:   sqlDB,
		path: dbPath,
	}

	// Initialize schema
	chaiDB.schema = NewChaiSchema(sqlDB)
	if err := chaiDB.schema.InitializeSchema(ctx); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return chaiDB, nil
}

// NewChaiDBFromPebble creates a Chai database alongside an existing Pebble instance
// This is provided for Phase 1 integration where both stores run in parallel
func NewChaiDBFromPebble(ctx context.Context, pebbleDB *pebble.DB, chaiPath string) (*ChaiDB, error) {
	if pebbleDB == nil {
		return nil, fmt.Errorf("pebbleDB cannot be nil")
	}

	// For now, we create a separate Chai database
	// In future phases, this could be tightly integrated with Pebble
	return NewChaiDB(ctx, chaiPath)
}

// DB returns the underlying *sql.DB for executing queries
func (c *ChaiDB) DB() *sql.DB {
	return c.db
}

// QueryContext executes a query with context
func (c *ChaiDB) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	if c.db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	return c.db.QueryContext(ctx, query, args...)
}

// QueryRowContext executes a query that returns a single row
func (c *ChaiDB) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	if c.db == nil {
		return nil
	}
	return c.db.QueryRowContext(ctx, query, args...)
}

// ExecContext executes a statement with context
func (c *ChaiDB) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	if c.db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	return c.db.ExecContext(ctx, query, args...)
}

// BeginTx begins a new transaction
func (c *ChaiDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	if c.db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	return c.db.BeginTx(ctx, opts)
}

// Close closes the database connection
func (c *ChaiDB) Close() error {
	if c.db == nil {
		return nil
	}
	return c.db.Close()
}

// Health checks if the database is healthy
func (c *ChaiDB) Health(ctx context.Context) error {
	if c.db == nil {
		return fmt.Errorf("database is not initialized")
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return c.db.PingContext(ctx)
}

// Stats returns database statistics
func (c *ChaiDB) Stats() sql.DBStats {
	if c.db == nil {
		return sql.DBStats{}
	}
	return c.db.Stats()
}

// Path returns the database file path
func (c *ChaiDB) Path() string {
	return c.path
}

// ResetSchema drops and recreates all tables (for testing)
func (c *ChaiDB) ResetSchema(ctx context.Context) error {
	if c.schema == nil {
		return fmt.Errorf("schema not initialized")
	}

	// Drop all tables
	if err := c.schema.DropAllTables(ctx); err != nil {
		return err
	}

	// Recreate them
	return c.schema.InitializeSchema(ctx)
}

// GetSchema returns the schema handler
func (c *ChaiDB) GetSchema() *ChaiSchema {
	return c.schema
}
