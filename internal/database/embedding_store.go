// file: internal/database/embedding_store.go
// version: 1.0.0
// guid: 7c4a9b2e-d831-4f5c-a07e-3b8d6e1f9c42

package database

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Embedding holds a vector embedding for a single entity.
type Embedding struct {
	EntityType string
	EntityID   string
	TextHash   string
	Vector     []float32
	Model      string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// SimilarityResult pairs an entity ID with its cosine similarity score.
type SimilarityResult struct {
	EntityID   string
	Similarity float32
}

// EmbeddingStore is a SQLite-backed sidecar for vector embeddings and dedup candidates.
type EmbeddingStore struct {
	db *sql.DB
}

// NewEmbeddingStore opens (or creates) the SQLite database at dbPath and
// initialises the schema.
func NewEmbeddingStore(dbPath string) (*EmbeddingStore, error) {
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=off", dbPath)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open embedding store: %w", err)
	}

	s := &EmbeddingStore{db: db}
	if err := s.createSchema(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create embedding schema: %w", err)
	}
	return s, nil
}

// Close releases the underlying database handle.
func (s *EmbeddingStore) Close() error {
	return s.db.Close()
}

// createSchema creates all tables and indexes if they do not already exist.
func (s *EmbeddingStore) createSchema() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS embeddings (
    id TEXT PRIMARY KEY,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    text_hash TEXT NOT NULL,
    vector BLOB NOT NULL,
    model TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_embeddings_type   ON embeddings(entity_type);
CREATE INDEX IF NOT EXISTS idx_embeddings_entity ON embeddings(entity_id);
CREATE INDEX IF NOT EXISTS idx_embeddings_hash   ON embeddings(text_hash);

CREATE TABLE IF NOT EXISTS dedup_candidates (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    entity_type TEXT NOT NULL,
    entity_a_id TEXT NOT NULL,
    entity_b_id TEXT NOT NULL,
    layer TEXT NOT NULL,
    similarity REAL,
    llm_verdict TEXT,
    llm_reason TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    UNIQUE(entity_type, entity_a_id, entity_b_id)
);
CREATE INDEX IF NOT EXISTS idx_dedup_status      ON dedup_candidates(status);
CREATE INDEX IF NOT EXISTS idx_dedup_type_status ON dedup_candidates(entity_type, status);
CREATE INDEX IF NOT EXISTS idx_dedup_entity_a    ON dedup_candidates(entity_a_id);
CREATE INDEX IF NOT EXISTS idx_dedup_entity_b    ON dedup_candidates(entity_b_id);
`)
	return err
}

// compositeKey returns the primary key for an embedding row.
func compositeKey(entityType, entityID string) string {
	return entityType + ":" + entityID
}

// Upsert inserts or replaces an embedding in the store.
func (s *EmbeddingStore) Upsert(e Embedding) error {
	id := compositeKey(e.EntityType, e.EntityID)
	now := time.Now().UTC()

	// Preserve created_at when updating.
	var createdAt time.Time
	err := s.db.QueryRow(`SELECT created_at FROM embeddings WHERE id = ?`, id).Scan(&createdAt)
	if err == sql.ErrNoRows {
		createdAt = now
	} else if err != nil {
		return fmt.Errorf("upsert check created_at: %w", err)
	}

	_, err = s.db.Exec(`
INSERT INTO embeddings (id, entity_type, entity_id, text_hash, vector, model, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    text_hash  = excluded.text_hash,
    vector     = excluded.vector,
    model      = excluded.model,
    updated_at = excluded.updated_at
`,
		id,
		e.EntityType,
		e.EntityID,
		e.TextHash,
		encodeVector(e.Vector),
		e.Model,
		createdAt.Format(time.RFC3339Nano),
		now.Format(time.RFC3339Nano),
	)
	return err
}

// Get retrieves an embedding by entity type and ID. Returns nil, nil when not found.
func (s *EmbeddingStore) Get(entityType, entityID string) (*Embedding, error) {
	id := compositeKey(entityType, entityID)
	row := s.db.QueryRow(`
SELECT entity_type, entity_id, text_hash, vector, model, created_at, updated_at
FROM embeddings WHERE id = ?`, id)

	var (
		e           Embedding
		vectorBlob  []byte
		createdAtS  string
		updatedAtS  string
	)
	err := row.Scan(&e.EntityType, &e.EntityID, &e.TextHash, &vectorBlob, &e.Model, &createdAtS, &updatedAtS)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get embedding: %w", err)
	}
	e.Vector = decodeVector(vectorBlob)
	if e.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAtS); err != nil {
		e.CreatedAt, _ = time.Parse("2006-01-02T15:04:05Z", createdAtS)
	}
	if e.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAtS); err != nil {
		e.UpdatedAt, _ = time.Parse("2006-01-02T15:04:05Z", updatedAtS)
	}
	return &e, nil
}

// Delete removes an embedding from the store. It is a no-op if the entity does not exist.
func (s *EmbeddingStore) Delete(entityType, entityID string) error {
	id := compositeKey(entityType, entityID)
	_, err := s.db.Exec(`DELETE FROM embeddings WHERE id = ?`, id)
	return err
}

// ListByType returns all embeddings for the given entity type.
func (s *EmbeddingStore) ListByType(entityType string) ([]Embedding, error) {
	rows, err := s.db.Query(`
SELECT entity_type, entity_id, text_hash, vector, model, created_at, updated_at
FROM embeddings WHERE entity_type = ?`, entityType)
	if err != nil {
		return nil, fmt.Errorf("list by type: %w", err)
	}
	defer rows.Close()

	var results []Embedding
	for rows.Next() {
		var (
			e          Embedding
			vectorBlob []byte
			createdAtS string
			updatedAtS string
		)
		if err := rows.Scan(&e.EntityType, &e.EntityID, &e.TextHash, &vectorBlob, &e.Model, &createdAtS, &updatedAtS); err != nil {
			return nil, fmt.Errorf("scan embedding: %w", err)
		}
		e.Vector = decodeVector(vectorBlob)
		if e.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAtS); err != nil {
			e.CreatedAt, _ = time.Parse("2006-01-02T15:04:05Z", createdAtS)
		}
		if e.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAtS); err != nil {
			e.UpdatedAt, _ = time.Parse("2006-01-02T15:04:05Z", updatedAtS)
		}
		results = append(results, e)
	}
	return results, rows.Err()
}

// FindSimilar loads all embeddings of entityType, computes cosine similarity
// against query, filters by minSimilarity, and returns up to maxResults sorted
// by similarity descending.
func (s *EmbeddingStore) FindSimilar(entityType string, query []float32, minSimilarity float32, maxResults int) ([]SimilarityResult, error) {
	all, err := s.ListByType(entityType)
	if err != nil {
		return nil, err
	}

	var candidates []SimilarityResult
	for _, e := range all {
		sim := CosineSimilarity(query, e.Vector)
		if sim >= minSimilarity {
			candidates = append(candidates, SimilarityResult{EntityID: e.EntityID, Similarity: sim})
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Similarity > candidates[j].Similarity
	})

	if maxResults > 0 && len(candidates) > maxResults {
		candidates = candidates[:maxResults]
	}
	return candidates, nil
}

// CountByType returns the number of embeddings stored for the given entity type.
func (s *EmbeddingStore) CountByType(entityType string) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM embeddings WHERE entity_type = ?`, entityType).Scan(&n)
	return n, err
}

// CosineSimilarity computes the cosine similarity between two vectors using
// float64 internally for precision. Returns 0 when either vector has zero norm.
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		ai := float64(a[i])
		bi := float64(b[i])
		dot += ai * bi
		normA += ai * ai
		normB += bi * bi
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return float32(dot / denom)
}

// encodeVector serialises a float32 slice to little-endian bytes.
func encodeVector(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// decodeVector deserialises little-endian bytes back to a float32 slice.
func decodeVector(b []byte) []float32 {
	n := len(b) / 4
	v := make([]float32, n)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}
