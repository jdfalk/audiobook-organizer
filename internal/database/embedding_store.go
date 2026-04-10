// file: internal/database/embedding_store.go
// version: 1.1.0
// guid: 7c4a9b2e-d831-4f5c-a07e-3b8d6e1f9c42

package database

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"strings"
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

// DedupCandidate represents a potential duplicate pair.
type DedupCandidate struct {
	ID         int64     `json:"id"`
	EntityType string    `json:"entity_type"`
	EntityAID  string    `json:"entity_a_id"`
	EntityBID  string    `json:"entity_b_id"`
	Layer      string    `json:"layer"`
	Similarity *float64  `json:"similarity,omitempty"`
	LLMVerdict string    `json:"llm_verdict,omitempty"`
	LLMReason  string    `json:"llm_reason,omitempty"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// CandidateFilter controls ListCandidates queries.
type CandidateFilter struct {
	EntityType    string
	Status        string
	Layer         string
	MinSimilarity *float64
	Limit         int
	Offset        int
}

// CandidateStat holds a count for one grouping.
type CandidateStat struct {
	EntityType string `json:"entity_type"`
	Layer      string `json:"layer"`
	Status     string `json:"status"`
	Count      int    `json:"count"`
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

// nullStr returns nil for empty strings, otherwise the string value.
// Used to store NULL instead of empty string in nullable TEXT columns.
func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// joinAnd joins a slice of SQL WHERE clause fragments with " AND ".
func joinAnd(clauses []string) string {
	return strings.Join(clauses, " AND ")
}

// UpsertCandidate inserts or updates a dedup candidate pair.
// On conflict (entity_type, entity_a_id, entity_b_id) the row is updated.
func (s *EmbeddingStore) UpsertCandidate(c DedupCandidate) error {
	now := time.Now().UTC()

	// Preserve created_at for existing rows.
	var createdAt time.Time
	err := s.db.QueryRow(
		`SELECT created_at FROM dedup_candidates WHERE entity_type=? AND entity_a_id=? AND entity_b_id=?`,
		c.EntityType, c.EntityAID, c.EntityBID,
	).Scan(&createdAt)
	if err == sql.ErrNoRows {
		createdAt = now
	} else if err != nil {
		return fmt.Errorf("upsert candidate check created_at: %w", err)
	}

	status := c.Status
	if status == "" {
		status = "pending"
	}

	_, err = s.db.Exec(`
INSERT INTO dedup_candidates
    (entity_type, entity_a_id, entity_b_id, layer, similarity, llm_verdict, llm_reason, status, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(entity_type, entity_a_id, entity_b_id) DO UPDATE SET
    layer       = excluded.layer,
    similarity  = excluded.similarity,
    llm_verdict = excluded.llm_verdict,
    llm_reason  = excluded.llm_reason,
    status      = excluded.status,
    updated_at  = excluded.updated_at
`,
		c.EntityType,
		c.EntityAID,
		c.EntityBID,
		c.Layer,
		c.Similarity,
		nullStr(c.LLMVerdict),
		nullStr(c.LLMReason),
		status,
		createdAt.Format(time.RFC3339Nano),
		now.Format(time.RFC3339Nano),
	)
	return err
}

// scanCandidate scans a single dedup_candidates row.
func scanCandidate(scan func(...any) error) (DedupCandidate, error) {
	var (
		c          DedupCandidate
		sim        sql.NullFloat64
		verdict    sql.NullString
		reason     sql.NullString
		createdAtS string
		updatedAtS string
	)
	if err := scan(
		&c.ID, &c.EntityType, &c.EntityAID, &c.EntityBID, &c.Layer,
		&sim, &verdict, &reason, &c.Status, &createdAtS, &updatedAtS,
	); err != nil {
		return c, err
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
	var err error
	if c.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAtS); err != nil {
		c.CreatedAt, _ = time.Parse("2006-01-02T15:04:05Z", createdAtS)
	}
	if c.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAtS); err != nil {
		c.UpdatedAt, _ = time.Parse("2006-01-02T15:04:05Z", updatedAtS)
	}
	return c, nil
}

// ListCandidates returns a paginated list of dedup candidates matching the filter
// along with the total count of matching rows.
func (s *EmbeddingStore) ListCandidates(f CandidateFilter) ([]DedupCandidate, int, error) {
	var clauses []string
	var args []any

	if f.EntityType != "" {
		clauses = append(clauses, "entity_type = ?")
		args = append(args, f.EntityType)
	}
	if f.Status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, f.Status)
	}
	if f.Layer != "" {
		clauses = append(clauses, "layer = ?")
		args = append(args, f.Layer)
	}
	if f.MinSimilarity != nil {
		clauses = append(clauses, "similarity >= ?")
		args = append(args, *f.MinSimilarity)
	}

	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + joinAnd(clauses)
	}

	// Total count.
	var total int
	if err := s.db.QueryRow(
		"SELECT COUNT(*) FROM dedup_candidates "+where, args...,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("list candidates count: %w", err)
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}

	query := fmt.Sprintf(`
SELECT id, entity_type, entity_a_id, entity_b_id, layer,
       similarity, llm_verdict, llm_reason, status, created_at, updated_at
FROM dedup_candidates
%s
ORDER BY COALESCE(similarity, 0) DESC
LIMIT ? OFFSET ?`, where)

	rows, err := s.db.Query(query, append(args, limit, f.Offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("list candidates query: %w", err)
	}
	defer rows.Close()

	var results []DedupCandidate
	for rows.Next() {
		c, err := scanCandidate(rows.Scan)
		if err != nil {
			return nil, 0, fmt.Errorf("scan candidate: %w", err)
		}
		results = append(results, c)
	}
	return results, total, rows.Err()
}

// UpdateCandidateStatus updates the status of a single candidate by ID.
func (s *EmbeddingStore) UpdateCandidateStatus(id int64, status string) error {
	_, err := s.db.Exec(
		`UPDATE dedup_candidates SET status=?, updated_at=? WHERE id=?`,
		status, time.Now().UTC().Format(time.RFC3339Nano), id,
	)
	return err
}

// UpdateCandidateLLM stores the LLM verdict and reason for a candidate and
// sets the layer to 'llm'.
func (s *EmbeddingStore) UpdateCandidateLLM(id int64, verdict, reason string) error {
	_, err := s.db.Exec(
		`UPDATE dedup_candidates SET llm_verdict=?, llm_reason=?, layer='llm', updated_at=? WHERE id=?`,
		nullStr(verdict), nullStr(reason), time.Now().UTC().Format(time.RFC3339Nano), id,
	)
	return err
}

// RemoveCandidatesForEntity deletes all candidate rows that involve the given
// entity (as either entity_a or entity_b). Returns the number of rows deleted.
func (s *EmbeddingStore) RemoveCandidatesForEntity(entityType, entityID string) (int, error) {
	res, err := s.db.Exec(
		`DELETE FROM dedup_candidates WHERE entity_type=? AND (entity_a_id=? OR entity_b_id=?)`,
		entityType, entityID, entityID,
	)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}

// GetCandidateStats returns row counts grouped by entity_type, layer, and status.
func (s *EmbeddingStore) GetCandidateStats() ([]CandidateStat, error) {
	rows, err := s.db.Query(`
SELECT entity_type, layer, status, COUNT(*) AS cnt
FROM dedup_candidates
GROUP BY entity_type, layer, status
ORDER BY entity_type, layer, status`)
	if err != nil {
		return nil, fmt.Errorf("get candidate stats: %w", err)
	}
	defer rows.Close()

	var stats []CandidateStat
	for rows.Next() {
		var st CandidateStat
		if err := rows.Scan(&st.EntityType, &st.Layer, &st.Status, &st.Count); err != nil {
			return nil, fmt.Errorf("scan candidate stat: %w", err)
		}
		stats = append(stats, st)
	}
	return stats, rows.Err()
}

// GetCandidateByID retrieves a single candidate by its integer ID.
// Returns nil, nil when not found.
func (s *EmbeddingStore) GetCandidateByID(id int64) (*DedupCandidate, error) {
	row := s.db.QueryRow(`
SELECT id, entity_type, entity_a_id, entity_b_id, layer,
       similarity, llm_verdict, llm_reason, status, created_at, updated_at
FROM dedup_candidates WHERE id=?`, id)

	c, err := scanCandidate(row.Scan)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get candidate by id: %w", err)
	}
	return &c, nil
}
