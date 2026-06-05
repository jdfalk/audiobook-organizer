// file: internal/database/embedding_store.go
// version: 2.0.0
// last-edited: 2026-05-11
// guid: 7c4a9b2e-d831-4f5c-a07e-3b8d6e1f9c42

package database

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/pebble/v2"
	"github.com/falkcorp/audiobook-organizer/internal/metrics"
)

// Key-space layout (PebbleDB):
//
//	emb:v:<entityType>:<entityID>   → embRec JSON      (vector + metadata)
//	emb:c:<model>:<textHash>        → raw float32 blob  (content-hash cache)
//	dedup:r:<id16hex>               → candRec JSON      (dedup candidate)
//	dedup:p:<type>:<aID>:<bID>      → id16hex           (pair uniqueness index)
//	dedup:seq                       → [8]byte LE int64  (auto-increment counter)
const (
	embVecPfx    = "emb:v:"
	embCachePfx  = "emb:c:"
	dedupRecPfx  = "dedup:r:"
	dedupPairPfx = "dedup:p:"
	dedupSeqKey  = "dedup:seq"
)

// embRec is the stored value for a vector embedding.
type embRec struct {
	TextHash  string `json:"h"`
	Vector    []byte `json:"v"` // encodeVector output (little-endian float32)
	Model     string `json:"m"`
	CreatedAt int64  `json:"c"` // Unix nanoseconds
	UpdatedAt int64  `json:"u"` // Unix nanoseconds
}

// candRec is the stored value for a dedup candidate.
type candRec struct {
	EntityType string   `json:"et"`
	EntityAID  string   `json:"a"`
	EntityBID  string   `json:"b"`
	Layer      string   `json:"l"`
	Similarity *float64 `json:"sim,omitempty"`
	LLMVerdict string   `json:"lv,omitempty"`
	LLMReason  string   `json:"lr,omitempty"`
	Status     string   `json:"s"`
	CreatedAt  int64    `json:"c"` // Unix nanoseconds
	UpdatedAt  int64    `json:"u"` // Unix nanoseconds
}

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
	MaxSimilarity *float64
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

// EmbeddingHealthStats contains diagnostic counts for the embedding store.
type EmbeddingHealthStats struct {
	VectorCount int64 `json:"vector_count"`
	SizeBytes   int64 `json:"size_bytes"`
}

// EmbeddingStore is a PebbleDB-backed store for vector embeddings, text-hash
// cache, and dedup candidates. It replaces the former SQLite sidecar (embeddings.db).
type EmbeddingStore struct {
	db        *pebble.DB
	owned     bool        // if true, Close() shuts down the DB
	mu        sync.Mutex  // serialises counter + pair-uniqueness operations
	closeOnce sync.Once   // guards owned Close against double-call
	closed    atomic.Bool // set on Close; makes post-close ops return errors, not panic
}

// NewEmbeddingStore creates an EmbeddingStore backed by the given PebbleDB.
// The DB is shared with the main store; Close() is a no-op.
func NewEmbeddingStore(db *pebble.DB) *EmbeddingStore {
	return &EmbeddingStore{db: db, owned: false}
}

// Close releases resources. A no-op when the DB is shared (owned=false).
// Safe to call more than once when owned — subsequent calls are no-ops.
func (s *EmbeddingStore) Close() error {
	if !s.owned {
		return nil
	}
	var closeErr error
	s.closeOnce.Do(func() {
		s.closed.Store(true)
		closeErr = s.db.Close()
	})
	return closeErr
}

// PebbleDB returns the underlying Pebble handle. This is provided so
// adjacent stores (e.g. the split-book candidate keyspace) can share
// the same shared-database connection without re-opening it. Returns
// nil if the receiver is nil or already closed.
func (s *EmbeddingStore) PebbleDB() *pebble.DB {
	if s == nil || s.closed.Load() {
		return nil
	}
	return s.db
}

// errClosed is returned by all operations when the store has been closed.
var errClosed = fmt.Errorf("embedding store closed")

func (s *EmbeddingStore) checkClosed() error {
	if s.closed.Load() {
		return errClosed
	}
	return nil
}

// ─── Vector methods ───────────────────────────────────────────────────────────

func embVecKey(entityType, entityID string) []byte {
	return []byte(embVecPfx + entityType + ":" + entityID)
}

// Upsert inserts or replaces a vector embedding. Created-at is preserved on updates.
func (s *EmbeddingStore) Upsert(e Embedding) error {
	if err := s.checkClosed(); err != nil {
		return err
	}
	key := embVecKey(e.EntityType, e.EntityID)
	now := time.Now().UnixNano()
	createdAt := now

	existing, err := s.getEmbRec(key)
	if err != nil {
		return err
	}
	if existing != nil {
		createdAt = existing.CreatedAt
	}

	return s.setJSON(key, embRec{
		TextHash:  e.TextHash,
		Vector:    encodeVector(e.Vector),
		Model:     e.Model,
		CreatedAt: createdAt,
		UpdatedAt: now,
	})
}

// Get retrieves an embedding by entity type and ID. Returns nil, nil when not found.
func (s *EmbeddingStore) Get(entityType, entityID string) (*Embedding, error) {
	if err := s.checkClosed(); err != nil {
		return nil, err
	}
	key := embVecKey(entityType, entityID)
	rec, err := s.getEmbRec(key)
	if err != nil || rec == nil {
		return nil, err
	}
	return &Embedding{
		EntityType: entityType,
		EntityID:   entityID,
		TextHash:   rec.TextHash,
		Vector:     decodeVector(rec.Vector),
		Model:      rec.Model,
		CreatedAt:  time.Unix(0, rec.CreatedAt),
		UpdatedAt:  time.Unix(0, rec.UpdatedAt),
	}, nil
}

// Delete removes an embedding. It is a no-op if the entity does not exist.
func (s *EmbeddingStore) Delete(entityType, entityID string) error {
	if err := s.checkClosed(); err != nil {
		return err
	}
	return s.db.Delete(embVecKey(entityType, entityID), pebble.Sync)
}

// ListByType returns all embeddings for the given entity type.
func (s *EmbeddingStore) ListByType(entityType string) ([]Embedding, error) {
	if err := s.checkClosed(); err != nil {
		return nil, err
	}
	prefix := []byte(embVecPfx + entityType + ":")
	upper := prefixUpperBound(prefix)

	iter, err := s.db.NewIter(&pebble.IterOptions{LowerBound: prefix, UpperBound: upper})
	if err != nil {
		return nil, fmt.Errorf("list by type %s: %w", entityType, err)
	}
	defer iter.Close()

	typePrefix := embVecPfx + entityType + ":"
	var results []Embedding
	for iter.First(); iter.Valid(); iter.Next() {
		var rec embRec
		if err := json.Unmarshal(iter.Value(), &rec); err != nil {
			continue
		}
		entityID := string(iter.Key())[len(typePrefix):]
		results = append(results, Embedding{
			EntityType: entityType,
			EntityID:   entityID,
			TextHash:   rec.TextHash,
			Vector:     decodeVector(rec.Vector),
			Model:      rec.Model,
			CreatedAt:  time.Unix(0, rec.CreatedAt),
			UpdatedAt:  time.Unix(0, rec.UpdatedAt),
		})
	}
	return results, iter.Error()
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
		if sim := CosineSimilarity(query, e.Vector); sim >= minSimilarity {
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
	if err := s.checkClosed(); err != nil {
		return 0, err
	}
	prefix := []byte(embVecPfx + entityType + ":")
	upper := prefixUpperBound(prefix)

	iter, err := s.db.NewIter(&pebble.IterOptions{LowerBound: prefix, UpperBound: upper})
	if err != nil {
		return 0, fmt.Errorf("count by type %s: %w", entityType, err)
	}
	defer iter.Close()

	n := 0
	for iter.First(); iter.Valid(); iter.Next() {
		n++
	}
	return n, iter.Error()
}

// ─── Cache methods ────────────────────────────────────────────────────────────

// GetCachedEmbedding looks up a cached embedding by text hash and model.
// Returns nil, nil on a cache miss.
func (s *EmbeddingStore) GetCachedEmbedding(textHash, model string) ([]float32, error) {
	if textHash == "" || model == "" {
		return nil, nil
	}
	start := time.Now()
	val, closer, err := s.db.Get([]byte(embCachePfx + model + ":" + textHash))
	metrics.ObserveCacheGetDuration("embedding", time.Since(start))
	if err == pebble.ErrNotFound {
		metrics.RecordCacheMiss("embedding", "not_found")
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get cached embedding: %w", err)
	}
	vec := decodeVector(val)
	closer.Close()
	metrics.RecordCacheHit("embedding")
	return vec, nil
}

// PutCachedEmbedding stores a vector keyed by text hash and model.
// A write failure is never fatal — callers log and continue.
func (s *EmbeddingStore) PutCachedEmbedding(textHash, model string, vector []float32) error {
	if textHash == "" || model == "" || len(vector) == 0 {
		return nil
	}
	if err := s.db.Set([]byte(embCachePfx+model+":"+textHash), encodeVector(vector), pebble.Sync); err != nil {
		return err
	}
	metrics.RecordCacheSet("embedding")
	return nil
}

// ─── Dedup candidate methods ──────────────────────────────────────────────────

func dedupRecKey(id int64) []byte {
	return []byte(fmt.Sprintf("%s%016x", dedupRecPfx, id))
}

func dedupPairKey(entityType, aID, bID string) []byte {
	return []byte(dedupPairPfx + entityType + ":" + aID + ":" + bID)
}

// nextID reads and increments the sequential counter. Must be called with s.mu held.
// The new counter value is written into the supplied batch so counter + record land atomically.
func (s *EmbeddingStore) nextID(b *pebble.Batch) (int64, error) {
	val, closer, err := s.db.Get([]byte(dedupSeqKey))
	var id int64
	if err == pebble.ErrNotFound {
		id = 1
	} else if err != nil {
		return 0, fmt.Errorf("read dedup seq: %w", err)
	} else {
		id = int64(binary.LittleEndian.Uint64(val)) + 1
		closer.Close()
	}
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], uint64(id))
	return id, b.Set([]byte(dedupSeqKey), buf[:], nil)
}

// UpsertCandidate inserts or updates a dedup candidate pair.
//
// Layer precedence (exact > llm > embedding): a more-specific layer never gets
// downgraded by a less-specific one. LLM fields are always updated when non-empty.
// Pairs are canonicalised before insert (lexicographically smaller ID is always A).
func (s *EmbeddingStore) UpsertCandidate(c DedupCandidate) error {
	if err := s.checkClosed(); err != nil {
		return err
	}
	if c.EntityAID > c.EntityBID {
		c.EntityAID, c.EntityBID = c.EntityBID, c.EntityAID
	}
	if c.Status == "" {
		c.Status = "pending"
	}
	now := time.Now().UnixNano()

	s.mu.Lock()
	defer s.mu.Unlock()

	pairKey := dedupPairKey(c.EntityType, c.EntityAID, c.EntityBID)
	existingIDBytes, closer, err := s.db.Get(pairKey)
	if err != nil && err != pebble.ErrNotFound {
		return fmt.Errorf("upsert candidate pair lookup: %w", err)
	}

	b := s.db.NewBatch()
	defer b.Close()

	if err == pebble.ErrNotFound {
		// New pair — assign sequential ID.
		id, err := s.nextID(b)
		if err != nil {
			return err
		}
		rec := candRec{
			EntityType: c.EntityType,
			EntityAID:  c.EntityAID,
			EntityBID:  c.EntityBID,
			Layer:      c.Layer,
			Similarity: c.Similarity,
			LLMVerdict: c.LLMVerdict,
			LLMReason:  c.LLMReason,
			Status:     c.Status,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		data, err := json.Marshal(rec)
		if err != nil {
			return fmt.Errorf("marshal candidate: %w", err)
		}
		if err := b.Set(dedupRecKey(id), data, nil); err != nil {
			return err
		}
		if err := b.Set(pairKey, []byte(fmt.Sprintf("%016x", id)), nil); err != nil {
			return err
		}
		return b.Commit(pebble.Sync)
	}

	// Existing pair — update in-place.
	idHex := string(existingIDBytes)
	closer.Close()
	id, err := strconv.ParseInt(idHex, 16, 64)
	if err != nil {
		return fmt.Errorf("parse candidate id %q: %w", idHex, err)
	}

	val, existingCloser, err := s.db.Get(dedupRecKey(id))
	if err != nil {
		return fmt.Errorf("read existing candidate %d: %w", id, err)
	}
	var existing candRec
	if err := json.Unmarshal(val, &existing); err != nil {
		existingCloser.Close()
		return fmt.Errorf("unmarshal existing candidate %d: %w", id, err)
	}
	existingCloser.Close()

	// Layer precedence mirrors the SQL ON CONFLICT logic:
	//   exact  → never overwritten (regardless of incoming layer)
	//   llm    → protected against embedding, but exact still upgrades it
	//   others → always overwritten with incoming layer + similarity
	protected := existing.Layer == "exact" ||
		(existing.Layer == "llm" && c.Layer != "exact")
	if !protected {
		existing.Layer = c.Layer
		existing.Similarity = c.Similarity
	}
	if c.LLMVerdict != "" {
		existing.LLMVerdict = c.LLMVerdict
	}
	if c.LLMReason != "" {
		existing.LLMReason = c.LLMReason
	}
	existing.Status = c.Status
	existing.UpdatedAt = now

	data, err := json.Marshal(existing)
	if err != nil {
		return fmt.Errorf("marshal updated candidate: %w", err)
	}
	if err := b.Set(dedupRecKey(id), data, nil); err != nil {
		return err
	}
	return b.Commit(pebble.Sync)
}

// GetCandidateByID retrieves a single candidate by its ID.
// Returns nil, nil when not found.
func (s *EmbeddingStore) GetCandidateByID(id int64) (*DedupCandidate, error) {
	val, closer, err := s.db.Get(dedupRecKey(id))
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get candidate %d: %w", id, err)
	}
	var rec candRec
	if err := json.Unmarshal(val, &rec); err != nil {
		closer.Close()
		return nil, fmt.Errorf("unmarshal candidate %d: %w", id, err)
	}
	closer.Close()
	c := candRecToCandidate(id, rec)
	return &c, nil
}

// ListCandidates returns a paginated list of dedup candidates matching the filter
// along with the total count of matching rows.
func (s *EmbeddingStore) ListCandidates(f CandidateFilter) ([]DedupCandidate, int, error) {
	if err := s.checkClosed(); err != nil {
		return nil, 0, err
	}
	prefix := []byte(dedupRecPfx)
	upper := prefixUpperBound(prefix)

	iter, err := s.db.NewIter(&pebble.IterOptions{LowerBound: prefix, UpperBound: upper})
	if err != nil {
		return nil, 0, fmt.Errorf("list candidates: %w", err)
	}
	defer iter.Close()

	var all []DedupCandidate
	for iter.First(); iter.Valid(); iter.Next() {
		idHex := string(iter.Key())[len(dedupRecPfx):]
		id, err := strconv.ParseInt(idHex, 16, 64)
		if err != nil {
			continue
		}
		var rec candRec
		if err := json.Unmarshal(iter.Value(), &rec); err != nil {
			continue
		}
		c := candRecToCandidate(id, rec)

		if f.EntityType != "" && c.EntityType != f.EntityType {
			continue
		}
		if f.Status != "" && c.Status != f.Status {
			continue
		}
		if f.Layer != "" && c.Layer != f.Layer {
			continue
		}
		if f.MinSimilarity != nil && (c.Similarity == nil || *c.Similarity < *f.MinSimilarity) {
			continue
		}
		if f.MaxSimilarity != nil && (c.Similarity == nil || *c.Similarity > *f.MaxSimilarity) {
			continue
		}
		all = append(all, c)
	}
	if err := iter.Error(); err != nil {
		return nil, 0, fmt.Errorf("list candidates scan: %w", err)
	}

	// Sort by similarity descending (mirrors SQL ORDER BY COALESCE(similarity,0) DESC).
	sort.Slice(all, func(i, j int) bool {
		si, sj := 0.0, 0.0
		if all[i].Similarity != nil {
			si = *all[i].Similarity
		}
		if all[j].Similarity != nil {
			sj = *all[j].Similarity
		}
		return si > sj
	})

	total := len(all)
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	start := f.Offset
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	return all[start:end], total, nil
}

// UpdateCandidateStatus updates the status of a single candidate by ID.
func (s *EmbeddingStore) UpdateCandidateStatus(id int64, status string) error {
	return s.updateCandidate(id, func(rec *candRec) {
		rec.Status = status
		rec.UpdatedAt = time.Now().UnixNano()
	})
}

// UpdateCandidateLLM stores the LLM verdict and reason for a candidate and
// sets the layer to 'llm'.
func (s *EmbeddingStore) UpdateCandidateLLM(id int64, verdict, reason string) error {
	return s.updateCandidate(id, func(rec *candRec) {
		rec.LLMVerdict = verdict
		rec.LLMReason = reason
		rec.Layer = "llm"
		rec.UpdatedAt = time.Now().UnixNano()
	})
}

func (s *EmbeddingStore) updateCandidate(id int64, mutFn func(*candRec)) error {
	val, closer, err := s.db.Get(dedupRecKey(id))
	if err == pebble.ErrNotFound {
		return nil
	}
	if err != nil {
		return fmt.Errorf("update candidate read %d: %w", id, err)
	}
	var rec candRec
	if err := json.Unmarshal(val, &rec); err != nil {
		closer.Close()
		return fmt.Errorf("update candidate unmarshal %d: %w", id, err)
	}
	closer.Close()

	mutFn(&rec)

	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("update candidate marshal %d: %w", id, err)
	}
	return s.db.Set(dedupRecKey(id), data, pebble.Sync)
}

// DeleteCandidate removes a single candidate row by ID.
func (s *EmbeddingStore) DeleteCandidate(id int64) error {
	val, closer, err := s.db.Get(dedupRecKey(id))
	if err == pebble.ErrNotFound {
		return nil
	}
	if err != nil {
		return fmt.Errorf("delete candidate read %d: %w", id, err)
	}
	var rec candRec
	if err := json.Unmarshal(val, &rec); err != nil {
		closer.Close()
		return fmt.Errorf("delete candidate unmarshal %d: %w", id, err)
	}
	closer.Close()

	b := s.db.NewBatch()
	defer b.Close()
	_ = b.Delete(dedupRecKey(id), nil)
	_ = b.Delete(dedupPairKey(rec.EntityType, rec.EntityAID, rec.EntityBID), nil)
	return b.Commit(pebble.Sync)
}

// MarkCandidatesAsMergedForEntity sets status="merged" on every candidate row
// that references the given entity on either side (entity_a_id OR entity_b_id),
// regardless of current layer or status — *except* rows whose status is already
// "merged", which are left untouched so the returned count reflects only newly
// transitioned rows.
//
// Use case (MAYDEPLOY-B3): when a Merge operation collapses book B into book A,
// any other dedup candidate that still references book B (e.g. a separate row
// comparing book B vs book C) becomes a stale "orphan" — clicking Merge on it
// would fail because book B is gone. Marking those rows as merged here causes
// the candidates UI to drop them on its next refresh, instead of the user
// having to dismiss each one manually.
//
// Returns the number of rows whose status was newly changed.
func (s *EmbeddingStore) MarkCandidatesAsMergedForEntity(entityType, entityID string) (int, error) {
	if entityType == "" || entityID == "" {
		return 0, nil
	}

	prefix := []byte(dedupRecPfx)
	upper := prefixUpperBound(prefix)

	iter, err := s.db.NewIter(&pebble.IterOptions{LowerBound: prefix, UpperBound: upper})
	if err != nil {
		return 0, fmt.Errorf("mark candidates merged for entity: %w", err)
	}

	type target struct {
		id  int64
		rec candRec
	}
	var targets []target
	for iter.First(); iter.Valid(); iter.Next() {
		idHex := string(iter.Key())[len(dedupRecPfx):]
		id, err := strconv.ParseInt(idHex, 16, 64)
		if err != nil {
			continue
		}
		var rec candRec
		if err := json.Unmarshal(iter.Value(), &rec); err != nil {
			continue
		}
		if rec.EntityType != entityType {
			continue
		}
		if rec.EntityAID != entityID && rec.EntityBID != entityID {
			continue
		}
		if rec.Status == "merged" {
			continue
		}
		targets = append(targets, target{id: id, rec: rec})
	}
	iter.Close()
	if err := iter.Error(); err != nil {
		return 0, err
	}

	now := time.Now().UnixNano()
	b := s.db.NewBatch()
	defer b.Close()
	for _, t := range targets {
		t.rec.Status = "merged"
		t.rec.UpdatedAt = now
		data, err := json.Marshal(t.rec)
		if err != nil {
			return 0, fmt.Errorf("mark candidates merged marshal %d: %w", t.id, err)
		}
		if err := b.Set(dedupRecKey(t.id), data, nil); err != nil {
			return 0, fmt.Errorf("mark candidates merged set %d: %w", t.id, err)
		}
	}
	if err := b.Commit(pebble.Sync); err != nil {
		return 0, fmt.Errorf("mark candidates merged commit: %w", err)
	}
	return len(targets), nil
}

// RemoveCandidatesForEntity deletes all candidate rows that involve the given
// entity (as either entity_a or entity_b). Returns the number of rows deleted.
func (s *EmbeddingStore) RemoveCandidatesForEntity(entityType, entityID string) (int, error) {
	prefix := []byte(dedupRecPfx)
	upper := prefixUpperBound(prefix)

	iter, err := s.db.NewIter(&pebble.IterOptions{LowerBound: prefix, UpperBound: upper})
	if err != nil {
		return 0, fmt.Errorf("remove candidates for entity: %w", err)
	}

	type target struct {
		id  int64
		rec candRec
	}
	var targets []target
	for iter.First(); iter.Valid(); iter.Next() {
		idHex := string(iter.Key())[len(dedupRecPfx):]
		id, err := strconv.ParseInt(idHex, 16, 64)
		if err != nil {
			continue
		}
		var rec candRec
		if err := json.Unmarshal(iter.Value(), &rec); err != nil {
			continue
		}
		if rec.EntityType == entityType && (rec.EntityAID == entityID || rec.EntityBID == entityID) {
			targets = append(targets, target{id: id, rec: rec})
		}
	}
	iter.Close()
	if err := iter.Error(); err != nil {
		return 0, err
	}

	b := s.db.NewBatch()
	defer b.Close()
	for _, t := range targets {
		_ = b.Delete(dedupRecKey(t.id), nil)
		_ = b.Delete(dedupPairKey(t.rec.EntityType, t.rec.EntityAID, t.rec.EntityBID), nil)
	}
	if err := b.Commit(pebble.Sync); err != nil {
		return 0, err
	}
	return len(targets), nil
}

// CanonicalizeCandidates rewrites existing rows so each logical pair has
// exactly one entry with entity_a_id < entity_b_id lexicographically.
// Returns (rewritten, deleted) counts for logging.
func (s *EmbeddingStore) CanonicalizeCandidates() (rewritten, deleted int, err error) {
	prefix := []byte(dedupRecPfx)
	upper := prefixUpperBound(prefix)

	iter, iterErr := s.db.NewIter(&pebble.IterOptions{LowerBound: prefix, UpperBound: upper})
	if iterErr != nil {
		return 0, 0, fmt.Errorf("canonicalize scan: %w", iterErr)
	}

	type nonCanon struct {
		id  int64
		rec candRec
	}
	var targets []nonCanon
	for iter.First(); iter.Valid(); iter.Next() {
		idHex := string(iter.Key())[len(dedupRecPfx):]
		id, err := strconv.ParseInt(idHex, 16, 64)
		if err != nil {
			continue
		}
		var rec candRec
		if err := json.Unmarshal(iter.Value(), &rec); err != nil {
			continue
		}
		if rec.EntityAID > rec.EntityBID {
			targets = append(targets, nonCanon{id: id, rec: rec})
		}
	}
	iter.Close()
	if err := iter.Error(); err != nil {
		return 0, 0, err
	}

	for _, t := range targets {
		canonA, canonB := t.rec.EntityBID, t.rec.EntityAID // swapped = canonical order
		canonPairKey := dedupPairKey(t.rec.EntityType, canonA, canonB)
		oldPairKey := dedupPairKey(t.rec.EntityType, t.rec.EntityAID, t.rec.EntityBID)

		_, existingCloser, existingErr := s.db.Get(canonPairKey)
		if existingErr == pebble.ErrNotFound {
			// No canonical row — swap in place.
			t.rec.EntityAID = canonA
			t.rec.EntityBID = canonB
			t.rec.UpdatedAt = time.Now().UnixNano()
			data, err := json.Marshal(t.rec)
			if err != nil {
				return rewritten, deleted, fmt.Errorf("canonicalize marshal: %w", err)
			}
			b := s.db.NewBatch()
			_ = b.Delete(oldPairKey, nil)
			_ = b.Set(canonPairKey, []byte(fmt.Sprintf("%016x", t.id)), nil)
			_ = b.Set(dedupRecKey(t.id), data, nil)
			if err := b.Commit(pebble.Sync); err != nil {
				b.Close()
				return rewritten, deleted, fmt.Errorf("canonicalize swap: %w", err)
			}
			b.Close()
			rewritten++
		} else if existingErr == nil {
			// Canonical row already exists — delete the non-canonical duplicate.
			existingCloser.Close()
			b := s.db.NewBatch()
			_ = b.Delete(dedupRecKey(t.id), nil)
			_ = b.Delete(oldPairKey, nil)
			if err := b.Commit(pebble.Sync); err != nil {
				b.Close()
				return rewritten, deleted, fmt.Errorf("canonicalize delete: %w", err)
			}
			b.Close()
			deleted++
		} else {
			return rewritten, deleted, fmt.Errorf("canonicalize check canonical: %w", existingErr)
		}
	}
	return rewritten, deleted, nil
}

// GetCandidateStats returns row counts grouped by entity_type, layer, and status.
func (s *EmbeddingStore) GetCandidateStats() ([]CandidateStat, error) {
	prefix := []byte(dedupRecPfx)
	upper := prefixUpperBound(prefix)

	iter, err := s.db.NewIter(&pebble.IterOptions{LowerBound: prefix, UpperBound: upper})
	if err != nil {
		return nil, fmt.Errorf("get candidate stats: %w", err)
	}
	defer iter.Close()

	type statKey struct{ entityType, layer, status string }
	counts := map[statKey]int{}

	for iter.First(); iter.Valid(); iter.Next() {
		var rec candRec
		if err := json.Unmarshal(iter.Value(), &rec); err != nil {
			continue
		}
		counts[statKey{rec.EntityType, rec.Layer, rec.Status}]++
	}
	if err := iter.Error(); err != nil {
		return nil, err
	}

	stats := make([]CandidateStat, 0, len(counts))
	for k, cnt := range counts {
		stats = append(stats, CandidateStat{EntityType: k.entityType, Layer: k.layer, Status: k.status, Count: cnt})
	}
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].EntityType != stats[j].EntityType {
			return stats[i].EntityType < stats[j].EntityType
		}
		if stats[i].Layer != stats[j].Layer {
			return stats[i].Layer < stats[j].Layer
		}
		return stats[i].Status < stats[j].Status
	})
	return stats, nil
}

// HealthStats returns diagnostic counters for the embedding store.
func (s *EmbeddingStore) HealthStats() (EmbeddingHealthStats, error) {
	books, err := s.CountByType("book")
	if err != nil {
		return EmbeddingHealthStats{}, err
	}
	authors, err := s.CountByType("author")
	if err != nil {
		return EmbeddingHealthStats{}, err
	}
	return EmbeddingHealthStats{VectorCount: int64(books + authors)}, nil
}

// ─── Internal helpers ─────────────────────────────────────────────────────────

func (s *EmbeddingStore) getEmbRec(key []byte) (*embRec, error) {
	val, closer, err := s.db.Get(key)
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read embedding record: %w", err)
	}
	var rec embRec
	if err := json.Unmarshal(val, &rec); err != nil {
		closer.Close()
		return nil, fmt.Errorf("unmarshal embedding record: %w", err)
	}
	closer.Close()
	return &rec, nil
}

func (s *EmbeddingStore) setJSON(key []byte, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return s.db.Set(key, data, pebble.Sync)
}

func candRecToCandidate(id int64, rec candRec) DedupCandidate {
	return DedupCandidate{
		ID:         id,
		EntityType: rec.EntityType,
		EntityAID:  rec.EntityAID,
		EntityBID:  rec.EntityBID,
		Layer:      rec.Layer,
		Similarity: rec.Similarity,
		LLMVerdict: rec.LLMVerdict,
		LLMReason:  rec.LLMReason,
		Status:     rec.Status,
		CreatedAt:  time.Unix(0, rec.CreatedAt),
		UpdatedAt:  time.Unix(0, rec.UpdatedAt),
	}
}

// prefixUpperBound returns the smallest key strictly greater than all keys with
// the given prefix, suitable as PebbleDB UpperBound.
func prefixUpperBound(prefix []byte) []byte {
	upper := make([]byte, len(prefix))
	copy(upper, prefix)
	for i := len(upper) - 1; i >= 0; i-- {
		upper[i]++
		if upper[i] != 0 {
			return upper[:i+1]
		}
	}
	return nil // all bytes were 0xFF; no upper bound
}

// ─── Math helpers (package-level, shared with dedup engine) ──────────────────

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
