// file: internal/database/memdb_schema.go
// version: 1.0.0
// guid: a1b2c3d4-mema-aaaa-aaaa-000000000002

package database

import "github.com/hashicorp/go-memdb"

// Table names for the in-memory query store.
const (
	memTableBooks            = "books"
	memTableAuthors          = "authors"
	memTableSeries           = "series"
	memTableBookFiles        = "book_files"
	memTableNarrators        = "narrators"
	memTableBookAuthors      = "book_authors"
	memTableBookNarrators    = "book_narrators"
	memTableImportPaths      = "import_paths"
	memTableAuthorAliases    = "author_aliases"
	memTableBlockedHashes    = "blocked_hashes"
	memTableWorks            = "works"
)

// Index names.
const (
	memIdxID                = "id"
	memIdxName              = "name"
	memIdxAuthorID          = "author_id"
	memIdxSeriesID          = "series_id"
	memIdxBookID            = "book_id"
	memIdxNarratorID        = "narrator_id"
	memIdxFilePath          = "file_path"
	memIdxFileHash          = "file_hash"
	memIdxMissing           = "missing"
	memIdxIsPrimaryVersion  = "is_primary_version"
	memIdxMarkedForDeletion = "marked_for_deletion"
	memIdxVersionGroupID    = "version_group_id"
	memIdxTitle             = "title"
	memIdxPath              = "path"
	memIdxEnabled           = "enabled"
	memIdxAliasName         = "alias_name"
	memIdxHash              = "hash"
	memIdxWorkAuthor        = "author_id"
)

// memdbSchema returns the complete schema for the in-memory query layer.
// PebbleDB remains source of truth; this schema is derived/rebuilt from Pebble
// on startup and kept in sync via write-through.
func memdbSchema() *memdb.DBSchema {
	return &memdb.DBSchema{
		Tables: map[string]*memdb.TableSchema{
			memTableBooks: {
				Name: memTableBooks,
				Indexes: map[string]*memdb.IndexSchema{
					memIdxID: {
						Name:    memIdxID,
						Unique:  true,
						Indexer: &memdb.StringFieldIndex{Field: "ID"},
					},
					memIdxAuthorID: {
						Name:         memIdxAuthorID,
						AllowMissing: true,
						Indexer:      &nullableIntFieldIndex{Field: "AuthorID"},
					},
					memIdxSeriesID: {
						Name:         memIdxSeriesID,
						AllowMissing: true,
						Indexer:      &nullableIntFieldIndex{Field: "SeriesID"},
					},
					memIdxIsPrimaryVersion: {
						// Default nil → true to match SQL semantics (column default true)
						Name: memIdxIsPrimaryVersion,
						Indexer: &effectiveBoolFieldIndex{
							Field:   "IsPrimaryVersion",
							Default: true,
						},
					},
					memIdxMarkedForDeletion: {
						// Default nil → false (column default false)
						Name: memIdxMarkedForDeletion,
						Indexer: &effectiveBoolFieldIndex{
							Field:   "MarkedForDeletion",
							Default: false,
						},
					},
					memIdxVersionGroupID: {
						Name:         memIdxVersionGroupID,
						AllowMissing: true,
						Indexer:      &nullableStringFieldIndex{Field: "VersionGroupID"},
					},
					memIdxFilePath: {
						// NOT unique. Pebble doesn't enforce file_path uniqueness;
						// real data has duplicates (soft-deleted versions, dedup
						// candidates). Declaring Unique here caused warmup to
						// abort on the first conflict and silently leave memdb
						// empty — which made the library list look empty.
						Name:         memIdxFilePath,
						AllowMissing: true,
						Indexer:      &memdb.StringFieldIndex{Field: "FilePath"},
					},
					memIdxTitle: {
						Name:    memIdxTitle,
						Indexer: &memdb.StringFieldIndex{Field: "Title", Lowercase: true},
					},
				},
			},

			memTableAuthors: {
				Name: memTableAuthors,
				Indexes: map[string]*memdb.IndexSchema{
					memIdxID: {
						Name:    memIdxID,
						Unique:  true,
						Indexer: &memdb.IntFieldIndex{Field: "ID"},
					},
					memIdxName: {
						Name:    memIdxName,
						Indexer: &memdb.StringFieldIndex{Field: "Name", Lowercase: true},
					},
				},
			},

			memTableSeries: {
				Name: memTableSeries,
				Indexes: map[string]*memdb.IndexSchema{
					memIdxID: {
						Name:    memIdxID,
						Unique:  true,
						Indexer: &memdb.IntFieldIndex{Field: "ID"},
					},
					memIdxName: {
						Name:    memIdxName,
						Indexer: &memdb.StringFieldIndex{Field: "Name", Lowercase: true},
					},
					memIdxAuthorID: {
						Name:         memIdxAuthorID,
						AllowMissing: true,
						Indexer:      &nullableIntFieldIndex{Field: "AuthorID"},
					},
				},
			},

			memTableBookFiles: {
				Name: memTableBookFiles,
				Indexes: map[string]*memdb.IndexSchema{
					memIdxID: {
						Name:    memIdxID,
						Unique:  true,
						Indexer: &memdb.StringFieldIndex{Field: "ID"},
					},
					memIdxBookID: {
						Name:    memIdxBookID,
						Indexer: &memdb.StringFieldIndex{Field: "BookID"},
					},
					memIdxFileHash: {
						Name:         memIdxFileHash,
						AllowMissing: true,
						Indexer:      &memdb.StringFieldIndex{Field: "FileHash"},
					},
					memIdxMissing: {
						Name:    memIdxMissing,
						Indexer: &plainBoolFieldIndex{Field: "Missing"},
					},
					memIdxFilePath: {
						Name:         memIdxFilePath,
						AllowMissing: true,
						Indexer:      &memdb.StringFieldIndex{Field: "FilePath"},
					},
				},
			},

			memTableNarrators: {
				Name: memTableNarrators,
				Indexes: map[string]*memdb.IndexSchema{
					memIdxID: {
						Name:    memIdxID,
						Unique:  true,
						Indexer: &memdb.IntFieldIndex{Field: "ID"},
					},
					memIdxName: {
						// NOT unique. Pebble may legitimately have multiple
						// Narrator rows with the same name (case-insensitive
						// matching is best-effort, not enforced). Unique would
						// abort warmup on the first collision.
						Name:    memIdxName,
						Indexer: &memdb.StringFieldIndex{Field: "Name", Lowercase: true},
					},
				},
			},

			memTableBookAuthors: {
				Name: memTableBookAuthors,
				Indexes: map[string]*memdb.IndexSchema{
					memIdxID: {
						// Composite primary key: book_id + author_id
						Name:   memIdxID,
						Unique: true,
						Indexer: &memdb.CompoundIndex{
							Indexes: []memdb.Indexer{
								&memdb.StringFieldIndex{Field: "BookID"},
								&memdb.IntFieldIndex{Field: "AuthorID"},
							},
						},
					},
					memIdxBookID: {
						Name:    memIdxBookID,
						Indexer: &memdb.StringFieldIndex{Field: "BookID"},
					},
					memIdxAuthorID: {
						Name:    memIdxAuthorID,
						Indexer: &memdb.IntFieldIndex{Field: "AuthorID"},
					},
				},
			},

			memTableBookNarrators: {
				Name: memTableBookNarrators,
				Indexes: map[string]*memdb.IndexSchema{
					memIdxID: {
						Name:   memIdxID,
						Unique: true,
						Indexer: &memdb.CompoundIndex{
							Indexes: []memdb.Indexer{
								&memdb.StringFieldIndex{Field: "BookID"},
								&memdb.IntFieldIndex{Field: "NarratorID"},
							},
						},
					},
					memIdxBookID: {
						Name:    memIdxBookID,
						Indexer: &memdb.StringFieldIndex{Field: "BookID"},
					},
					memIdxNarratorID: {
						Name:    memIdxNarratorID,
						Indexer: &memdb.IntFieldIndex{Field: "NarratorID"},
					},
				},
			},

			memTableImportPaths: {
				Name: memTableImportPaths,
				Indexes: map[string]*memdb.IndexSchema{
					memIdxID: {
						Name:    memIdxID,
						Unique:  true,
						Indexer: &memdb.IntFieldIndex{Field: "ID"},
					},
					memIdxPath: {
						// NOT unique (warmup-safety). Pebble already de-dupes
						// import paths at write time via path-keyed lookup, but
						// historical data may have collisions and we don't want
						// warmup to abort.
						Name:    memIdxPath,
						Indexer: &memdb.StringFieldIndex{Field: "Path"},
					},
					memIdxEnabled: {
						Name:    memIdxEnabled,
						Indexer: &memdb.BoolFieldIndex{Field: "Enabled"},
					},
				},
			},

			memTableAuthorAliases: {
				Name: memTableAuthorAliases,
				Indexes: map[string]*memdb.IndexSchema{
					memIdxID: {
						Name:    memIdxID,
						Unique:  true,
						Indexer: &memdb.IntFieldIndex{Field: "ID"},
					},
					memIdxAuthorID: {
						Name:    memIdxAuthorID,
						Indexer: &memdb.IntFieldIndex{Field: "AuthorID"},
					},
					memIdxAliasName: {
						Name:    memIdxAliasName,
						Indexer: &memdb.StringFieldIndex{Field: "AliasName", Lowercase: true},
					},
				},
			},

			memTableBlockedHashes: {
				Name: memTableBlockedHashes,
				Indexes: map[string]*memdb.IndexSchema{
					// go-memdb requires every table to have an "id" index.
					memIdxID: {
						Name:    memIdxID,
						Unique:  true,
						Indexer: &memdb.StringFieldIndex{Field: "Hash"},
					},
					memIdxHash: {
						Name:    memIdxHash,
						Unique:  true,
						Indexer: &memdb.StringFieldIndex{Field: "Hash"},
					},
				},
			},

			memTableWorks: {
				Name: memTableWorks,
				Indexes: map[string]*memdb.IndexSchema{
					memIdxID: {
						Name:    memIdxID,
						Unique:  true,
						Indexer: &memdb.StringFieldIndex{Field: "ID"},
					},
					memIdxTitle: {
						Name:    memIdxTitle,
						Indexer: &memdb.StringFieldIndex{Field: "Title", Lowercase: true},
					},
					memIdxWorkAuthor: {
						Name:         memIdxWorkAuthor,
						AllowMissing: true,
						Indexer:      &nullableIntFieldIndex{Field: "AuthorID"},
					},
					memIdxSeriesID: {
						Name:         memIdxSeriesID,
						AllowMissing: true,
						Indexer:      &nullableIntFieldIndex{Field: "SeriesID"},
					},
				},
			},
		},
	}
}
