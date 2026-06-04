// file: internal/database/chromem_embedding_store.go
// version: 1.1.0
// guid: 2d0e1f9a-3b4c-4a70-b8c5-3d7e0f1b9a99
//
// chromem-go backed embedding vector store (DES-2).
//
// Replaces the linear-scan FindSimilar from the SQLite embedding
// store with exhaustive cosine-similarity search (brute-force O(n)).
// chromem-go v0.7.0 does not implement HNSW; that is a roadmap item.
// Supports metadata filtering at query time (primary-version,
// series exclusion, etc.).
//
// The SQLite/Pebble EmbeddingStore stays as the source of truth for
// embedding vectors — this only handles the in-memory ANN index.
//
// MAYDEPLOY-D2 (2026-05): Switched from chromem.NewPersistentDB to
// chromem.NewDB (in-memory only). Background:
//
//   - chromem-go v0.7.0 persists synchronously by writing one gob file
//     per document on every AddDocument call. With ~50K books × 3072-dim
//     vectors that is 50K+ files (~600MB-1.2GB) and 50K fsync-heavy
//     writes per hydrate cycle — slow and inode-heavy.
//   - In production the persistent dir was observed as empty (1KB) and
//     the dedup engine already re-hydrates from the SQLite/Pebble
//     embedding store on every startup (see dedup/lifecycle.go and
//     engine.HydrateChromem). The hydrate is the canonical mirror path.
//   - DEDUP_CHROMEM_LAZY=true (MAYDEPLOY-D1 / PR #1169) is the operator
//     escape hatch when hydrate is too costly; without persistence the
//     intent is now unambiguous: chromem is a derived in-memory index,
//     not a second source of truth.
//
// If we ever want on-disk ANN persistence again, the right answer is
// a different backing store (e.g. HNSW with mmap), not chromem-go's
// per-document gob files.

package database

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	chromem "github.com/philippgille/chromem-go"
)

const (
	chromemStoreModeInMemory = "in-memory"
	chromemStoreModeUnknown  = "unknown"
)

// ChromemEmbeddingStore wraps chromem-go collections for ANN vector search.
type ChromemEmbeddingStore struct {
	db          *chromem.DB
	collections map[string]*chromem.Collection
	mu          sync.RWMutex
	dims        int
	mode        string
}

// NewChromemEmbeddingStore returns an in-memory chromem-go store.
func NewChromemEmbeddingStore(dir string, dims int) (*ChromemEmbeddingStore, error) {
	_ = dir
	db := chromem.NewDB()
	if db == nil {
		return nil, fmt.Errorf("chromem.NewDB returned nil")
	}
	return &ChromemEmbeddingStore{
		db:          db,
		collections: make(map[string]*chromem.Collection),
		dims:        dims,
		mode:        chromemStoreModeInMemory,
	}, nil
}

// NewInMemoryChromemStore creates an in-memory chromem store for tests.
func NewInMemoryChromemStore(dims int) *ChromemEmbeddingStore {
	db := chromem.NewDB()
	return &ChromemEmbeddingStore{
		db:          db,
		collections: make(map[string]*chromem.Collection),
		dims:        dims,
		mode:        chromemStoreModeInMemory,
	}
}

// StoreMode reports the mode used to construct the chromem store.
func (s *ChromemEmbeddingStore) StoreMode() string {
	if s == nil || s.mode == "" {
		return chromemStoreModeUnknown
	}
	return s.mode
}

func (s *ChromemEmbeddingStore) getOrCreateCollection(entityType string) (*chromem.Collection, error) {
	s.mu.RLock()
	col, ok := s.collections[entityType]
	s.mu.RUnlock()
	if ok {
		return col, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if col, ok := s.collections[entityType]; ok {
		return col, nil
	}
	col, err := s.db.GetOrCreateCollection(entityType, nil, nil)
	if err != nil {
		return nil, err
	}
	s.collections[entityType] = col
	return col, nil
}

// Upsert stores or replaces an embedding with metadata.
func (s *ChromemEmbeddingStore) Upsert(ctx context.Context, entityType, entityID string, vec []float32, meta map[string]string) error {
	col, err := s.getOrCreateCollection(entityType)
	if err != nil {
		return err
	}
	doc := chromem.Document{
		ID:        entityID,
		Embedding: vec,
		Metadata:  meta,
	}
	return col.AddDocument(ctx, doc)
}

// Get returns metadata for a single embedding.
func (s *ChromemEmbeddingStore) Get(ctx context.Context, entityType, entityID string) (map[string]string, error) {
	col, err := s.getOrCreateCollection(entityType)
	if err != nil {
		return nil, err
	}
	doc, err := col.GetByID(ctx, entityID)
	if err != nil {
		return nil, nil
	}
	if doc.ID == "" {
		return nil, nil
	}
	return doc.Metadata, nil
}

// Delete removes an embedding with the given ID.
func (s *ChromemEmbeddingStore) Delete(ctx context.Context, entityType, entityID string) error {
	col, err := s.getOrCreateCollection(entityType)
	if err != nil {
		return err
	}
	return col.Delete(ctx, nil, nil, entityID)
}

// ChromemSimilarityResult is a single ANN match.
type ChromemSimilarityResult struct {
	EntityID   string
	Similarity float32
	Metadata   map[string]string
}

// FindSimilar performs an ANN query with optional metadata filters.
func (s *ChromemEmbeddingStore) FindSimilar(ctx context.Context, entityType string, query []float32, maxResults int, filter map[string]string) ([]ChromemSimilarityResult, error) {
	col, err := s.getOrCreateCollection(entityType)
	if err != nil {
		return nil, err
	}
	if maxResults <= 0 {
		maxResults = 20
	}
	count := col.Count()
	if maxResults > count {
		maxResults = count
	}
	if maxResults <= 0 {
		return nil, nil
	}
	res, err := col.QueryEmbedding(ctx, query, maxResults, filter, nil)
	if err != nil {
		return nil, fmt.Errorf("chromem query: %w", err)
	}
	out := make([]ChromemSimilarityResult, 0, len(res))
	for _, r := range res {
		out = append(out, ChromemSimilarityResult{
			EntityID:   r.ID,
			Similarity: r.Similarity,
			Metadata:   r.Metadata,
		})
	}
	return out, nil
}

// CountByType returns the document count for a collection.
func (s *ChromemEmbeddingStore) CountByType(ctx context.Context, entityType string) (int, error) {
	col, err := s.getOrCreateCollection(entityType)
	if err != nil {
		return 0, err
	}
	return col.Count(), nil
}

// Close releases chromem resources (no-op for in-memory store).
func (s *ChromemEmbeddingStore) Close() error {
	return nil
}

// MetaBool reads a boolean metadata key.
func MetaBool(meta map[string]string, key string) bool {
	return meta[key] == "true"
}

// MetaInt reads an integer metadata key.
func MetaInt(meta map[string]string, key string) int {
	n, _ := strconv.Atoi(meta[key])
	return n
}
