// file: internal/database/embedding_store_test.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

package database

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestEmbeddingStore creates a temporary EmbeddingStore for use in tests.
func newTestEmbeddingStore(t *testing.T) *EmbeddingStore {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "embeddings.db")
	store, err := NewEmbeddingStore(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// TestEmbeddingStore_ContentHashCache locks in the contract that
// GetCachedEmbedding returns (nil, nil) on a miss, PutCachedEmbedding
// round-trips a vector by text hash + model, and different models
// for the same hash are isolated. This is the cache path that
// EmbedBatch relies on to avoid burning the OpenAI quota on
// identical-content re-embeds.
func TestEmbeddingStore_ContentHashCache(t *testing.T) {
	store := newTestEmbeddingStore(t)

	const hash = "deadbeef"
	const model = "text-embedding-3-large"

	// Miss on a fresh store.
	got, err := store.GetCachedEmbedding(hash, model)
	require.NoError(t, err)
	assert.Nil(t, got, "miss should return nil, nil")

	// Round-trip a vector.
	want := []float32{0.1, 0.2, 0.3, 0.4}
	require.NoError(t, store.PutCachedEmbedding(hash, model, want))
	got, err = store.GetCachedEmbedding(hash, model)
	require.NoError(t, err)
	assert.Equal(t, want, got)

	// Same hash, different model → separate row.
	smaller := []float32{1, 2}
	require.NoError(t, store.PutCachedEmbedding(hash, "text-embedding-3-small", smaller))
	got, err = store.GetCachedEmbedding(hash, "text-embedding-3-small")
	require.NoError(t, err)
	assert.Equal(t, smaller, got)
	// The original entry at the original model is still intact.
	got, err = store.GetCachedEmbedding(hash, model)
	require.NoError(t, err)
	assert.Equal(t, want, got)

	// Empty arguments short-circuit cleanly.
	got, err = store.GetCachedEmbedding("", model)
	require.NoError(t, err)
	assert.Nil(t, got)
	require.NoError(t, store.PutCachedEmbedding("", model, want))
	require.NoError(t, store.PutCachedEmbedding(hash, model, nil))
}

func TestEmbeddingStore_UpsertAndGet(t *testing.T) {
	store := newTestEmbeddingStore(t)

	e := Embedding{
		EntityType: "book",
		EntityID:   "book-001",
		TextHash:   "abc123",
		Vector:     []float32{0.1, 0.2, 0.3},
		Model:      "text-embedding-3-small",
	}
	require.NoError(t, store.Upsert(e))

	got, err := store.Get("book", "book-001")
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, "book", got.EntityType)
	assert.Equal(t, "book-001", got.EntityID)
	assert.Equal(t, "abc123", got.TextHash)
	assert.Equal(t, "text-embedding-3-small", got.Model)
	require.Len(t, got.Vector, 3)
	assert.InDelta(t, 0.1, got.Vector[0], 1e-6)
	assert.InDelta(t, 0.2, got.Vector[1], 1e-6)
	assert.InDelta(t, 0.3, got.Vector[2], 1e-6)
	assert.False(t, got.CreatedAt.IsZero())
	assert.False(t, got.UpdatedAt.IsZero())
}

func TestEmbeddingStore_UpsertOverwrites(t *testing.T) {
	store := newTestEmbeddingStore(t)

	first := Embedding{
		EntityType: "book",
		EntityID:   "book-001",
		TextHash:   "hash-v1",
		Vector:     []float32{1.0, 0.0, 0.0},
		Model:      "model-v1",
	}
	require.NoError(t, store.Upsert(first))

	second := Embedding{
		EntityType: "book",
		EntityID:   "book-001",
		TextHash:   "hash-v2",
		Vector:     []float32{0.0, 1.0, 0.0},
		Model:      "model-v2",
	}
	require.NoError(t, store.Upsert(second))

	got, err := store.Get("book", "book-001")
	require.NoError(t, err)
	require.NotNil(t, got)

	// Second upsert must win.
	assert.Equal(t, "hash-v2", got.TextHash)
	assert.Equal(t, "model-v2", got.Model)
	assert.InDelta(t, 0.0, got.Vector[0], 1e-6)
	assert.InDelta(t, 1.0, got.Vector[1], 1e-6)

	// Only one row should exist.
	n, err := store.CountByType("book")
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}

func TestEmbeddingStore_FindSimilar(t *testing.T) {
	store := newTestEmbeddingStore(t)

	// Two vectors pointing in roughly the same direction.
	similar1 := Embedding{EntityType: "book", EntityID: "sim-a", TextHash: "h1", Vector: []float32{1, 0, 0}, Model: "m"}
	similar2 := Embedding{EntityType: "book", EntityID: "sim-b", TextHash: "h2", Vector: []float32{0.9, 0.1, 0}, Model: "m"}
	// One vector pointing in an orthogonal direction.
	different := Embedding{EntityType: "book", EntityID: "diff-c", TextHash: "h3", Vector: []float32{0, 0, 1}, Model: "m"}

	require.NoError(t, store.Upsert(similar1))
	require.NoError(t, store.Upsert(similar2))
	require.NoError(t, store.Upsert(different))

	query := []float32{1, 0, 0}
	results, err := store.FindSimilar("book", query, 0.5, 10)
	require.NoError(t, err)

	// Should find sim-a and sim-b, not diff-c.
	ids := make([]string, len(results))
	for i, r := range results {
		ids[i] = r.EntityID
	}
	assert.Contains(t, ids, "sim-a")
	assert.Contains(t, ids, "sim-b")
	assert.NotContains(t, ids, "diff-c")

	// Results must be sorted descending by similarity.
	if len(results) >= 2 {
		assert.GreaterOrEqual(t, results[0].Similarity, results[1].Similarity)
	}
}

func TestCosineSimilarity(t *testing.T) {
	t.Run("identical vectors produce 1.0", func(t *testing.T) {
		v := []float32{1, 2, 3}
		assert.InDelta(t, 1.0, CosineSimilarity(v, v), 1e-6)
	})

	t.Run("orthogonal vectors produce 0.0", func(t *testing.T) {
		a := []float32{1, 0, 0}
		b := []float32{0, 1, 0}
		assert.InDelta(t, 0.0, CosineSimilarity(a, b), 1e-6)
	})

	t.Run("opposite vectors produce -1.0", func(t *testing.T) {
		a := []float32{1, 0, 0}
		b := []float32{-1, 0, 0}
		assert.InDelta(t, -1.0, CosineSimilarity(a, b), 1e-6)
	})
}

func TestEmbeddingStore_Delete(t *testing.T) {
	store := newTestEmbeddingStore(t)

	e := Embedding{
		EntityType: "author",
		EntityID:   "author-001",
		TextHash:   "hash1",
		Vector:     []float32{0.5, 0.5},
		Model:      "m",
	}
	require.NoError(t, store.Upsert(e))

	// Verify it exists.
	got, err := store.Get("author", "author-001")
	require.NoError(t, err)
	require.NotNil(t, got)

	// Delete and verify it is gone.
	require.NoError(t, store.Delete("author", "author-001"))

	got, err = store.Get("author", "author-001")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestEmbeddingStore_ListByType(t *testing.T) {
	store := newTestEmbeddingStore(t)

	books := []Embedding{
		{EntityType: "book", EntityID: "b1", TextHash: "h1", Vector: []float32{1}, Model: "m"},
		{EntityType: "book", EntityID: "b2", TextHash: "h2", Vector: []float32{2}, Model: "m"},
	}
	authors := []Embedding{
		{EntityType: "author", EntityID: "a1", TextHash: "h3", Vector: []float32{3}, Model: "m"},
	}

	for _, e := range append(books, authors...) {
		require.NoError(t, store.Upsert(e))
	}

	gotBooks, err := store.ListByType("book")
	require.NoError(t, err)
	assert.Len(t, gotBooks, 2)
	for _, b := range gotBooks {
		assert.Equal(t, "book", b.EntityType)
	}

	gotAuthors, err := store.ListByType("author")
	require.NoError(t, err)
	assert.Len(t, gotAuthors, 1)
	assert.Equal(t, "a1", gotAuthors[0].EntityID)
}
