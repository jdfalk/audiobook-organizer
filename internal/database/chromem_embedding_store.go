// file: internal/database/chromem_embedding_store.go
// version: 1.0.0
// guid: 2d0e1f9a-3b4c-4a70-b8c5-3d7e0f1b9a99
//
// chromem-go backed embedding vector store (DES-2).
//
// Replaces the linear-scan FindSimilar from the SQLite embedding
// store with HNSW-based approximate nearest neighbor search.
// Supports metadata filtering at query time (primary-version,
// series exclusion, etc.).
//
// The SQLite EmbeddingStore stays for DedupCandidate CRUD — this
// only handles the vector operations.

package database

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"sync"

	chromem "github.com/philippgille/chromem-go"
)

// ChromemEmbeddingStore wraps chromem-go collections for ANN vector
// search. One collection per entity type (books, authors).
type ChromemEmbeddingStore struct {
	db          *chromem.DB
	collections map[string]*chromem.Collection
	mu          sync.RWMutex
	dims        int
}

// NewChromemEmbeddingStore opens or creates a persistent chromem-go
// database at the given directory.
func NewChromemEmbeddingStore(dir string, dims int) (*ChromemEmbeddingStore, error) {
	dbPath := filepath.Join(dir, "chromem")
	db, err := chromem.NewPersistentDB(dbPath, false)
	if err != nil {
		return nil, fmt.Errorf("open chromem at %s: %w", dbPath, err)
	}
	return &ChromemEmbeddingStore{
		db:          db,
		collections: make(map[string]*chromem.Collection),
		dims:        dims,
	}, nil
}

// NewInMemoryChromemStore creates an in-memory store for tests.
func NewInMemoryChromemStore(dims int) *ChromemEmbeddingStore {
	db := chromem.NewDB()
	return &ChromemEmbeddingStore{
		db:          db,
		collections: make(map[string]*chromem.Collection),
		dims:        dims,
	}
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
	// Double-check.
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

// Get returns a single embedding's metadata by ID.
func (s *ChromemEmbeddingStore) Get(ctx context.Context, entityType, entityID string) (map[string]string, error) {
	col, err := s.getOrCreateCollection(entityType)
	if err != nil {
		return nil, err
	}
	doc, err := col.GetByID(ctx, entityID)
	if err != nil {
		return nil, nil // not found
	}
	if doc.ID == "" {
		return nil, nil
	}
	return doc.Metadata, nil
}

// Delete removes an embedding.
func (s *ChromemEmbeddingStore) Delete(ctx context.Context, entityType, entityID string) error {
	col, err := s.getOrCreateCollection(entityType)
	if err != nil {
		return err
	}
	return col.Delete(ctx, nil, nil, entityID)
}

// ChromemSimilarityResult is a scored match from FindSimilar.
type ChromemSimilarityResult struct {
	EntityID   string
	Similarity float32
	Metadata   map[string]string
}

// FindSimilar performs an ANN query with optional metadata filter.
func (s *ChromemEmbeddingStore) FindSimilar(
	ctx context.Context,
	entityType string,
	query []float32,
	maxResults int,
	filter map[string]string,
) ([]ChromemSimilarityResult, error) {
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
	results, err := col.QueryEmbedding(ctx, query, maxResults, filter, nil)
	if err != nil {
		return nil, fmt.Errorf("chromem query: %w", err)
	}

	out := make([]ChromemSimilarityResult, 0, len(results))
	for _, r := range results {
		out = append(out, ChromemSimilarityResult{
			EntityID:   r.ID,
			Similarity: r.Similarity,
			Metadata:   r.Metadata,
		})
	}
	return out, nil
}

// CountByType returns the document count in a collection.
func (s *ChromemEmbeddingStore) CountByType(ctx context.Context, entityType string) (int, error) {
	col, err := s.getOrCreateCollection(entityType)
	if err != nil {
		return 0, err
	}
	return col.Count(), nil
}

// Close is a no-op for chromem-go (persistence is automatic).
func (s *ChromemEmbeddingStore) Close() error {
	return nil
}

// Helper to convert metadata to typed values.

// MetaBool reads a boolean metadata value.
func MetaBool(meta map[string]string, key string) bool {
	return meta[key] == "true"
}

// MetaInt reads an integer metadata value.
func MetaInt(meta map[string]string, key string) int {
	n, _ := strconv.Atoi(meta[key])
	return n
}
