// file: internal/database/iface_author.go
// version: 1.1.0
// guid: 2e3b78c0-c989-48c0-a324-b88ea52b1ccd
// last-edited: 2026-04-30

package database

import "context"

// AuthorReader is the read-only author slice (authors + aliases + book-author joins).
type AuthorReader interface {
	GetAllAuthors() ([]Author, error)
	GetAuthorByID(id int) (*Author, error)
	GetAuthorByName(name string) (*Author, error)
	GetAuthorAliases(authorID int) ([]AuthorAlias, error)
	GetAllAuthorAliases() ([]AuthorAlias, error)
	FindAuthorByAlias(aliasName string) (*Author, error)
	GetBookAuthors(bookID string) ([]BookAuthor, error)
	GetBooksByAuthorIDWithRole(authorID int) ([]Book, error)
	GetAllAuthorBookCounts() (map[int]int, error)
	GetAllAuthorFileCounts() (map[int]int, error)
	GetAuthorTombstone(oldID int) (int, error)
	// GetAuthorsByBookIDs returns a map from bookID → []Author for all given book IDs.
	// Returns an empty map (not nil) if bookIDs is empty.
	GetAuthorsByBookIDs(ctx context.Context, bookIDs []string) (map[string][]Author, error)
}

// AuthorWriter is the write-only author slice.
type AuthorWriter interface {
	CreateAuthor(name string) (*Author, error)
	DeleteAuthor(id int) error
	UpdateAuthorName(id int, name string) error
	CreateAuthorAlias(authorID int, aliasName string, aliasType string) (*AuthorAlias, error)
	DeleteAuthorAlias(id int) error
	SetBookAuthors(bookID string, authors []BookAuthor) error
	CreateAuthorTombstone(oldID, canonicalID int) error
	ResolveTombstoneChains() (int, error)
}

// AuthorStore combines both halves.
type AuthorStore interface {
	AuthorReader
	AuthorWriter
}
