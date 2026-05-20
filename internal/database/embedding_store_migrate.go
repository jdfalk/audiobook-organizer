// file: internal/database/embedding_store_migrate.go
// version: 1.0.1
// last-edited: 2026-05-11
// guid: a3c1f2e8-b947-4d6a-9e5c-0f1a8b7c3d2e
//
// One-shot migration: copies all rows from the legacy SQLite embeddings.db
// (entity vectors + text-hash cache + dedup candidates) into PebbleDB.
//
// Called from server.go once at startup. A flag key "emb:migrated_v1" is
// written to PebbleDB on success so the migration never runs twice.
// If embeddings.db does not exist the function returns immediately (fresh install).
//
// This file can be deleted (along with embeddings.db) after the migration has
// been confirmed in production.

package database

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/cockroachdb/pebble/v2"
	_ "github.com/mattn/go-sqlite3"
)

const embMigratedKey = "emb:migrated_v1"

// MigrateEmbeddingsFromSQLite reads all rows from the legacy embeddings.db at
// sqlitePath and writes them into db. It is idempotent: a second call is a no-op.
// If sqlitePath does not exist the function returns nil immediately.
func MigrateEmbeddingsFromSQLite(db *pebble.DB, sqlitePath string) error {
	// Check migration flag.
	_, closer, err := db.Get([]byte(embMigratedKey))
	if err == nil {
		closer.Close()
		return nil // already done
	}
	if err != pebble.ErrNotFound {
		return fmt.Errorf("check migration flag: %w", err)
	}

	// Skip gracefully if the old file does not exist (fresh deploy).
	if _, statErr := os.Stat(sqlitePath); os.IsNotExist(statErr) {
		return markMigrated(db)
	}

	slog.Info("Migrating embeddings.db → PebbleDB from", "sqlitePath", sqlitePath)
	start := time.Now()

	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=off&mode=ro", sqlitePath)
	sdb, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return fmt.Errorf("open legacy embeddings.db: %w", err)
	}
	defer sdb.Close()

	store := &EmbeddingStore{db: db, owned: false}

	vectors, cache, err := migrateEmbeddingsTable(sdb, store)
	if err != nil {
		return fmt.Errorf("migrate embeddings table: %w", err)
	}

	candidates, err := migrateDedupCandidatesTable(sdb, store)
	if err != nil {
		return fmt.Errorf("migrate dedup_candidates table: %w", err)
	}

	if err := markMigrated(db); err != nil {
		return err
	}

	slog.Info("Embeddings migration complete in vectors cache candidates", "value0", time.Since(start).Round(time.Millisecond), "vectors", vectors, "cache", cache, "candidates", candidates)
	return nil
}

func migrateEmbeddingsTable(sdb *sql.DB, store *EmbeddingStore) (vectors, cache int, err error) {
	rows, err := sdb.Query(`
SELECT entity_type, entity_id, text_hash, vector, model, created_at, updated_at
FROM embeddings ORDER BY created_at`)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			entityType string
			entityID   string
			textHash   string
			vectorBlob []byte
			model      string
			createdAtS string
			updatedAtS string
		)
		if err := rows.Scan(&entityType, &entityID, &textHash, &vectorBlob, &model, &createdAtS, &updatedAtS); err != nil {
			slog.Warn("migrate embeddings scan row", "err", err)
			continue
		}

		vec := decodeVector(vectorBlob)
		createdAt := parseTime(createdAtS)
		updatedAt := parseTime(updatedAtS)

		if entityType == "cache" {
			// Cache entries were stored as entity_type='cache', entity_id='<model>:<hash>'.
			// Reconstruct model and hash from entity_id.
			m, h := splitModelHash(entityID)
			if m == "" {
				m = model
				h = textHash
			}
			if err := store.PutCachedEmbedding(h, m, vec); err != nil {
				slog.Warn("migrate embeddings put cache", "entityID", entityID, "err", err)
				continue
			}
			cache++
		} else {
			key := embVecKey(entityType, entityID)
			rec := embRec{
				TextHash:  textHash,
				Vector:    encodeVector(vec),
				Model:     model,
				CreatedAt: createdAt.UnixNano(),
				UpdatedAt: updatedAt.UnixNano(),
			}
			if err := store.setJSON(key, rec); err != nil {
				slog.Warn("migrate embeddings write vector", "entityType", entityType, "entityID", entityID, "err", err)
				continue
			}
			vectors++
		}
	}
	return vectors, cache, rows.Err()
}

func migrateDedupCandidatesTable(sdb *sql.DB, store *EmbeddingStore) (int, error) {
	rows, err := sdb.Query(`
SELECT entity_type, entity_a_id, entity_b_id, layer,
       similarity, llm_verdict, llm_reason, status, created_at, updated_at
FROM dedup_candidates ORDER BY id`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	n := 0
	for rows.Next() {
		var (
			entityType string
			entityAID  string
			entityBID  string
			layer      string
			sim        sql.NullFloat64
			verdict    sql.NullString
			reason     sql.NullString
			status     string
			createdAtS string
			updatedAtS string
		)
		if err := rows.Scan(&entityType, &entityAID, &entityBID, &layer,
			&sim, &verdict, &reason, &status, &createdAtS, &updatedAtS); err != nil {
			slog.Warn("migrate dedup_candidates scan row", "err", err)
			continue
		}

		c := DedupCandidate{
			EntityType: entityType,
			EntityAID:  entityAID,
			EntityBID:  entityBID,
			Layer:      layer,
			Status:     status,
			CreatedAt:  parseTime(createdAtS),
			UpdatedAt:  parseTime(updatedAtS),
		}
		if sim.Valid {
			v := sim.Float64
			c.Similarity = &v
		}
		if verdict.Valid {
			c.LLMVerdict = verdict.String
		}
		if reason.Valid {
			c.LLMReason = reason.String
		}

		if err := store.UpsertCandidate(c); err != nil {
			slog.Warn("migrate dedup_candidates upsert /", "entityType", entityType, "entityAID", entityAID, "entityBID", entityBID, "err", err)
			continue
		}
		n++
	}
	return n, rows.Err()
}

func markMigrated(db *pebble.DB) error {
	return db.Set([]byte(embMigratedKey), []byte("1"), pebble.Sync)
}

func parseTime(s string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05Z", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// splitModelHash splits an entity_id of the form "<model>:<hash>" into its parts.
// Returns ("", "") if no colon is found.
func splitModelHash(entityID string) (model, hash string) {
	for i, c := range entityID {
		if c == ':' {
			return entityID[:i], entityID[i+1:]
		}
	}
	return "", ""
}
