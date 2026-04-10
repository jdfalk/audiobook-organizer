// file: internal/ai/embedding_scorer_test.go
// version: 1.0.0
// guid: 9c3e5b17-6f8a-4d2e-b091-5a7c8d4e2f6a

package ai

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeEmbedAPI is an in-process stand-in for the OpenAI embeddings endpoint.
// Tests install a textToVec function that maps a text to a deterministic
// vector, so cosine math is predictable without real API calls.
type fakeEmbedAPI struct {
	textToVec  func(string) []float32
	embedOne   int // call counts for assertions
	embedBatch int
	failNext   error
}

func (f *fakeEmbedAPI) EmbedOne(ctx context.Context, text string) ([]float32, error) {
	f.embedOne++
	if f.failNext != nil {
		err := f.failNext
		f.failNext = nil
		return nil, err
	}
	return f.textToVec(text), nil
}

func (f *fakeEmbedAPI) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	f.embedBatch++
	if f.failNext != nil {
		err := f.failNext
		f.failNext = nil
		return nil, err
	}
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = f.textToVec(t)
	}
	return out, nil
}

// oneHotByPrefix returns a 4-dim vector where the hot index depends on the
// first character of the text. Two texts sharing a first letter get identical
// vectors (cosine = 1.0), different letters get orthogonal vectors
// (cosine = 0.0). This makes the test assertions trivial to read.
func oneHotByPrefix(text string) []float32 {
	if text == "" {
		return []float32{0, 0, 0, 0}
	}
	switch text[0] {
	case 'a', 'A':
		return []float32{1, 0, 0, 0}
	case 'b', 'B':
		return []float32{0, 1, 0, 0}
	case 'c', 'C':
		return []float32{0, 0, 1, 0}
	default:
		return []float32{0, 0, 0, 1}
	}
}

func newFakeScorer(t *testing.T) (*EmbeddingScorer, *fakeEmbedAPI) {
	t.Helper()
	api := &fakeEmbedAPI{textToVec: oneHotByPrefix}
	scorer := NewEmbeddingScorerWithAPI(api, nil)
	return scorer, api
}

func TestEmbeddingScorer_Name(t *testing.T) {
	scorer, _ := newFakeScorer(t)
	assert.Equal(t, "embedding", scorer.Name())
}

func TestEmbeddingScorer_EmptyCandidates(t *testing.T) {
	scorer, api := newFakeScorer(t)
	scores, err := scorer.Score(context.Background(), Query{Title: "Dune"}, nil)
	require.NoError(t, err)
	assert.Nil(t, scores)
	assert.Equal(t, 0, api.embedOne, "empty candidates should not trigger query embedding")
	assert.Equal(t, 0, api.embedBatch, "empty candidates should not trigger candidate batch")
}

func TestEmbeddingScorer_CosineRanking(t *testing.T) {
	scorer, api := newFakeScorer(t)

	// Query title "Dune" starts with 'd' → hot index 3 (the default branch).
	// Candidates use known prefixes that give orthogonal or identical vectors.
	scores, err := scorer.Score(context.Background(), Query{Title: "Dune by Frank Herbert"}, []Candidate{
		{Title: "abyss", Author: "X"},     // different prefix → cosine 0
		{Title: "different", Author: "X"}, // 'd' prefix → same vector as query → cosine 1
		{Title: "boring", Author: "X"},    // different prefix → cosine 0
	})
	require.NoError(t, err)
	require.Len(t, scores, 3)
	assert.InDelta(t, 0.0, scores[0], 0.01, "candidate 0 should be orthogonal to query")
	assert.InDelta(t, 1.0, scores[1], 0.01, "candidate 1 should match query perfectly")
	assert.InDelta(t, 0.0, scores[2], 0.01, "candidate 2 should be orthogonal to query")

	assert.Equal(t, 1, api.embedOne, "query should be embedded once")
	assert.Equal(t, 1, api.embedBatch, "candidates should be batch-embedded once")
}

func TestEmbeddingScorer_ClampsNegativeCosine(t *testing.T) {
	// Force an opposite-direction vector to produce cosine = -1, verify it
	// clamps to 0.
	api := &fakeEmbedAPI{
		textToVec: func(text string) []float32 {
			if text[0] == 'q' {
				return []float32{1, 0, 0, 0}
			}
			return []float32{-1, 0, 0, 0}
		},
	}
	scorer := NewEmbeddingScorerWithAPI(api, nil)
	scores, err := scorer.Score(context.Background(), Query{Title: "query"}, []Candidate{
		{Title: "other"},
	})
	require.NoError(t, err)
	assert.Equal(t, 0.0, scores[0], "negative cosine should clamp to 0")
}

func TestEmbeddingScorer_QueryEmbedError(t *testing.T) {
	api := &fakeEmbedAPI{textToVec: oneHotByPrefix, failNext: errors.New("boom")}
	scorer := NewEmbeddingScorerWithAPI(api, nil)

	scores, err := scorer.Score(context.Background(), Query{Title: "Dune"}, []Candidate{
		{Title: "Dune"},
	})
	require.Error(t, err)
	assert.Nil(t, scores, "partial results are never returned")
}

func TestEmbeddingScorer_CandidateBatchError(t *testing.T) {
	api := &fakeEmbedAPI{textToVec: oneHotByPrefix}
	scorer := NewEmbeddingScorerWithAPI(api, nil)

	// First call succeeds (query), next call (batch) fails.
	_, _ = scorer.Score(context.Background(), Query{Title: "Dune"}, []Candidate{{Title: "Dune"}})
	api.failNext = errors.New("batch failure")

	scores, err := scorer.Score(context.Background(), Query{Title: "Dune"}, []Candidate{
		{Title: "Dune"},
		{Title: "Dune Messiah"},
	})
	require.Error(t, err)
	assert.Nil(t, scores)
}
