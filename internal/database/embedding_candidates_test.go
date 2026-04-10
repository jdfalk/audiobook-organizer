// file: internal/database/embedding_candidates_test.go
// version: 1.2.0
// guid: f3e2d1c0-b9a8-4765-8e7d-6f5c4b3a2190

package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// floatPtr is a test helper that returns a pointer to a float64 value.
func floatPtr(f float64) *float64 { return &f }

func TestDedupCandidates_CreateAndList(t *testing.T) {
	store := newTestEmbeddingStore(t)

	c1 := DedupCandidate{
		EntityType: "book",
		EntityAID:  "b1",
		EntityBID:  "b2",
		Layer:      "embedding",
		Similarity: floatPtr(0.95),
		Status:     "pending",
	}
	c2 := DedupCandidate{
		EntityType: "book",
		EntityAID:  "b3",
		EntityBID:  "b4",
		Layer:      "embedding",
		Similarity: floatPtr(0.80),
		Status:     "pending",
	}

	require.NoError(t, store.UpsertCandidate(c1))
	require.NoError(t, store.UpsertCandidate(c2))

	results, total, err := store.ListCandidates(CandidateFilter{
		EntityType: "book",
		Status:     "pending",
	})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, results, 2)

	// Highest similarity should come first.
	assert.Equal(t, "b1", results[0].EntityAID)
	assert.Equal(t, "b3", results[1].EntityAID)
}

func TestDedupCandidates_UpdateStatus(t *testing.T) {
	store := newTestEmbeddingStore(t)

	c := DedupCandidate{
		EntityType: "book",
		EntityAID:  "b1",
		EntityBID:  "b2",
		Layer:      "embedding",
		Status:     "pending",
	}
	require.NoError(t, store.UpsertCandidate(c))

	// Retrieve so we have the auto-assigned ID.
	results, _, err := store.ListCandidates(CandidateFilter{})
	require.NoError(t, err)
	require.Len(t, results, 1)

	id := results[0].ID
	require.NoError(t, store.UpdateCandidateStatus(id, "merged"))

	got, err := store.GetCandidateByID(id)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "merged", got.Status)
}

func TestDedupCandidates_UpsertIdempotent(t *testing.T) {
	store := newTestEmbeddingStore(t)

	base := DedupCandidate{
		EntityType: "book",
		EntityAID:  "b1",
		EntityBID:  "b2",
		Layer:      "embedding",
		Similarity: floatPtr(0.90),
		Status:     "pending",
	}
	require.NoError(t, store.UpsertCandidate(base))

	// Second upsert with updated similarity.
	updated := base
	updated.Similarity = floatPtr(0.99)
	require.NoError(t, store.UpsertCandidate(updated))

	results, total, err := store.ListCandidates(CandidateFilter{})
	require.NoError(t, err)
	assert.Equal(t, 1, total, "upsert must not create a second row")
	require.Len(t, results, 1)
	require.NotNil(t, results[0].Similarity)
	assert.InDelta(t, 0.99, *results[0].Similarity, 1e-9)
}

func TestDedupCandidates_Stats(t *testing.T) {
	store := newTestEmbeddingStore(t)

	candidates := []DedupCandidate{
		{EntityType: "book", EntityAID: "b1", EntityBID: "b2", Layer: "embedding", Status: "pending"},
		{EntityType: "book", EntityAID: "b3", EntityBID: "b4", Layer: "embedding", Status: "merged"},
		{EntityType: "author", EntityAID: "a1", EntityBID: "a2", Layer: "metadata", Status: "pending"},
	}
	for _, c := range candidates {
		require.NoError(t, store.UpsertCandidate(c))
	}

	stats, err := store.GetCandidateStats()
	require.NoError(t, err)
	assert.NotEmpty(t, stats)

	// Build a lookup map for easier assertion.
	type key struct{ entityType, layer, status string }
	lookup := make(map[key]int)
	for _, s := range stats {
		lookup[key{s.EntityType, s.Layer, s.Status}] = s.Count
	}

	assert.Equal(t, 1, lookup[key{"book", "embedding", "pending"}])
	assert.Equal(t, 1, lookup[key{"book", "embedding", "merged"}])
	assert.Equal(t, 1, lookup[key{"author", "metadata", "pending"}])
}

func TestDedupCandidates_RemoveForEntity(t *testing.T) {
	store := newTestEmbeddingStore(t)

	// b1 is involved in two pairs; b3/b4 do not involve b1.
	candidates := []DedupCandidate{
		{EntityType: "book", EntityAID: "b1", EntityBID: "b2", Layer: "embedding", Status: "pending"},
		{EntityType: "book", EntityAID: "b3", EntityBID: "b1", Layer: "embedding", Status: "pending"},
		{EntityType: "book", EntityAID: "b3", EntityBID: "b4", Layer: "embedding", Status: "pending"},
	}
	for _, c := range candidates {
		require.NoError(t, store.UpsertCandidate(c))
	}

	n, err := store.RemoveCandidatesForEntity("book", "b1")
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	remaining, total, err := store.ListCandidates(CandidateFilter{})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, remaining, 1)
	assert.Equal(t, "b3", remaining[0].EntityAID)
	assert.Equal(t, "b4", remaining[0].EntityBID)
}

// TestDedupCandidates_LayerPrecedence verifies that an upsert with a
// lower-confidence layer does not downgrade an existing higher-confidence
// row. Precedence: exact > llm > embedding. This locks in the fix for a
// bug where FullScan would silently erase the `exact` bucket because
// findSimilarBooks re-upserted the same pair as `embedding` after
// checkExactTitle had just flagged it as `exact`.
func TestDedupCandidates_LayerPrecedence(t *testing.T) {
	store := newTestEmbeddingStore(t)

	// Seed the pair as exact (no similarity — Layer 1 doesn't use one).
	require.NoError(t, store.UpsertCandidate(DedupCandidate{
		EntityType: "book",
		EntityAID:  "book_a",
		EntityBID:  "book_b",
		Layer:      "exact",
		Status:     "pending",
	}))

	// Attempt to overwrite as embedding with a similarity score — this is
	// exactly what findSimilarBooks does during a FullScan pass.
	require.NoError(t, store.UpsertCandidate(DedupCandidate{
		EntityType: "book",
		EntityAID:  "book_a",
		EntityBID:  "book_b",
		Layer:      "embedding",
		Similarity: floatPtr(0.94),
		Status:     "pending",
	}))

	got, _, err := store.ListCandidates(CandidateFilter{EntityType: "book"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "exact", got[0].Layer, "exact should not be downgraded to embedding")
	assert.Nil(t, got[0].Similarity, "exact layer should keep its nil similarity, not adopt the embedding's 0.94")

	// Overwriting as llm should also leave exact in place.
	require.NoError(t, store.UpsertCandidate(DedupCandidate{
		EntityType: "book",
		EntityAID:  "book_a",
		EntityBID:  "book_b",
		Layer:      "llm",
		LLMVerdict: "duplicate",
		LLMReason:  "same book",
		Status:     "pending",
	}))
	got, _, _ = store.ListCandidates(CandidateFilter{EntityType: "book"})
	require.Len(t, got, 1)
	assert.Equal(t, "exact", got[0].Layer, "exact should not be downgraded to llm")
	// LLM verdict and reason are still persisted even when layer stays exact,
	// so future reviewers see the LLM's take as supplementary evidence.
	assert.Equal(t, "duplicate", got[0].LLMVerdict)
	assert.Equal(t, "same book", got[0].LLMReason)
}

// TestDedupCandidates_LayerUpgrade verifies the opposite direction: an
// embedding row correctly gets upgraded to llm (when the LLM reranker
// processes it) and to exact (if Layer 1 later catches the pair).
func TestDedupCandidates_LayerUpgrade(t *testing.T) {
	store := newTestEmbeddingStore(t)

	// Seed as embedding.
	require.NoError(t, store.UpsertCandidate(DedupCandidate{
		EntityType: "book",
		EntityAID:  "book_a",
		EntityBID:  "book_b",
		Layer:      "embedding",
		Similarity: floatPtr(0.88),
		Status:     "pending",
	}))

	// Upgrade to llm.
	require.NoError(t, store.UpsertCandidate(DedupCandidate{
		EntityType: "book",
		EntityAID:  "book_a",
		EntityBID:  "book_b",
		Layer:      "llm",
		LLMVerdict: "duplicate",
		LLMReason:  "same book, different subtitle",
		Status:     "pending",
	}))
	got, _, _ := store.ListCandidates(CandidateFilter{EntityType: "book"})
	require.Len(t, got, 1)
	assert.Equal(t, "llm", got[0].Layer, "llm should upgrade over embedding")
	assert.Equal(t, "duplicate", got[0].LLMVerdict)

	// Upgrade to exact.
	require.NoError(t, store.UpsertCandidate(DedupCandidate{
		EntityType: "book",
		EntityAID:  "book_a",
		EntityBID:  "book_b",
		Layer:      "exact",
		Status:     "pending",
	}))
	got, _, _ = store.ListCandidates(CandidateFilter{EntityType: "book"})
	require.Len(t, got, 1)
	assert.Equal(t, "exact", got[0].Layer, "exact should upgrade over llm")
}

// TestDedupCandidates_UpsertCanonicalizes verifies that inserting the same
// logical pair with swapped entity IDs produces exactly one row (in
// canonical form: smaller ID first). Before this fix, FullScan would
// discover a pair once as (A,B) while processing book A, then again as
// (B,A) while processing book B, and each went into its own row because
// the UNIQUE constraint treats (A,B) and (B,A) as distinct — which is
// why the UI showed the same "Foundation and Empire" pair twice.
func TestDedupCandidates_UpsertCanonicalizes(t *testing.T) {
	store := newTestEmbeddingStore(t)

	// Insert the pair as (book_z, book_a) — non-canonical direction.
	require.NoError(t, store.UpsertCandidate(DedupCandidate{
		EntityType: "book",
		EntityAID:  "book_z",
		EntityBID:  "book_a",
		Layer:      "embedding",
		Similarity: floatPtr(0.92),
		Status:     "pending",
	}))
	// Insert the same pair in canonical direction — should update the
	// existing row, not create a new one.
	require.NoError(t, store.UpsertCandidate(DedupCandidate{
		EntityType: "book",
		EntityAID:  "book_a",
		EntityBID:  "book_z",
		Layer:      "embedding",
		Similarity: floatPtr(0.93),
		Status:     "pending",
	}))

	got, total, err := store.ListCandidates(CandidateFilter{EntityType: "book"})
	require.NoError(t, err)
	assert.Equal(t, 1, total, "exactly one row should exist for the pair")
	require.Len(t, got, 1)
	// Canonical form: smaller ID first
	assert.Equal(t, "book_a", got[0].EntityAID)
	assert.Equal(t, "book_z", got[0].EntityBID)
	// Second upsert's similarity should have taken effect.
	require.NotNil(t, got[0].Similarity)
	assert.InDelta(t, 0.93, *got[0].Similarity, 0.0001)
}

// TestCanonicalizeCandidates_Cleanup verifies the one-time cleanup that
// removes duplicate (A,B)/(B,A) rows from deployments that accumulated
// them before UpsertCandidate started canonicalizing on insert. The
// cleanup must (a) swap non-canonical rows in place when no canonical
// sibling exists, and (b) delete the non-canonical row when a canonical
// sibling already exists.
func TestCanonicalizeCandidates_Cleanup(t *testing.T) {
	store := newTestEmbeddingStore(t)

	// Bypass the upsert canonicalization by using raw SQL so we can
	// simulate a pre-fix database state.
	rawInsert := func(typ, a, b, layer string) {
		t.Helper()
		_, err := store.db.Exec(`
			INSERT INTO dedup_candidates
				(entity_type, entity_a_id, entity_b_id, layer, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'pending', '2026-04-10', '2026-04-10')
		`, typ, a, b, layer)
		require.NoError(t, err)
	}

	// Case A: non-canonical row with no canonical sibling — should be
	// swapped in place.
	rawInsert("book", "zebra", "apple", "embedding")

	// Case B: canonical row already exists alongside a non-canonical
	// duplicate — the non-canonical duplicate should be deleted.
	rawInsert("book", "hello", "world", "embedding") // canonical (h < w)
	rawInsert("book", "world", "hello", "exact")     // non-canonical duplicate

	// Case C: pair already in canonical form — untouched.
	rawInsert("book", "aaa", "bbb", "embedding")

	rewritten, deleted, err := store.CanonicalizeCandidates()
	require.NoError(t, err)
	assert.Equal(t, 1, rewritten, "case A should be swapped in place")
	assert.Equal(t, 1, deleted, "case B's non-canonical duplicate should be deleted")

	// Verify final state: 3 rows, all in canonical order.
	got, total, err := store.ListCandidates(CandidateFilter{EntityType: "book", Limit: 100})
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	require.Len(t, got, 3)
	for _, c := range got {
		assert.LessOrEqual(t, c.EntityAID, c.EntityBID,
			"all rows should have entity_a_id <= entity_b_id after canonicalize, got (%s, %s)",
			c.EntityAID, c.EntityBID)
	}
}
