// file: internal/ai/llm_scorer_test.go
// version: 1.1.0
// guid: b2e4f817-6c0a-4d93-a8e5-3f1b7d2c9045

package ai

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// newFakeBackend returns a mockery-generated mockMetadataLLMBackend
// configured with the given scores / error. Returns the mock plus a
// pointer to a call counter so tests can assert call counts (the old
// hand-written fakeLLMBackend had a `calls int` field that tests
// inspected directly — preserving that contract).
func newFakeBackend(t *testing.T, scores []MetadataLLMScore, err error) (*mockMetadataLLMBackend, *int) {
	t.Helper()
	calls := 0
	m := newMockMetadataLLMBackend(t)
	m.EXPECT().
		ScoreMetadataCandidates(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, q MetadataLLMQuery, cands []MetadataLLMCandidate) ([]MetadataLLMScore, error) {
			calls++
			if err != nil {
				return nil, err
			}
			return scores, nil
		}).Maybe()
	return m, &calls
}

func TestLLMScorer_Name(t *testing.T) {
	backend, _ := newFakeBackend(t, nil, nil)
	scorer := NewLLMScorerWithBackend(backend)
	assert.Equal(t, "llm", scorer.Name())
}

func TestLLMScorer_EmptyCandidates(t *testing.T) {
	backend, calls := newFakeBackend(t, nil, nil)
	scorer := NewLLMScorerWithBackend(backend)
	scores, err := scorer.Score(context.Background(), Query{Title: "Dune"}, nil)
	require.NoError(t, err)
	assert.Nil(t, scores)
	assert.Equal(t, 0, *calls, "empty input should not call the backend")
}

func TestLLMScorer_ScoresInOrder(t *testing.T) {
	backend, calls := newFakeBackend(t, []MetadataLLMScore{
		{Index: 0, Score: 0.91, Reason: "exact match"},
		{Index: 1, Score: 0.42, Reason: "different edition"},
		{Index: 2, Score: 0.15, Reason: "different book"},
	}, nil)
	scorer := NewLLMScorerWithBackend(backend)

	scores, err := scorer.Score(context.Background(),
		Query{Title: "Dune", Author: "Frank Herbert"},
		[]Candidate{
			{Title: "Dune", Author: "Frank Herbert"},
			{Title: "Dune Messiah", Author: "Frank Herbert"},
			{Title: "Dune: The Butlerian Jihad", Author: "Brian Herbert"},
		},
	)
	require.NoError(t, err)
	require.Len(t, scores, 3)
	assert.InDelta(t, 0.91, scores[0], 0.0001)
	assert.InDelta(t, 0.42, scores[1], 0.0001)
	assert.InDelta(t, 0.15, scores[2], 0.0001)
	assert.Equal(t, 1, *calls)
}

func TestLLMScorer_OutOfOrderIndices(t *testing.T) {
	// LLM returns scores in a different order than the input — the scorer
	// must route them back to their input slot by Index.
	backend, _ := newFakeBackend(t, []MetadataLLMScore{
		{Index: 2, Score: 0.15},
		{Index: 0, Score: 0.91},
		{Index: 1, Score: 0.42},
	}, nil)
	scorer := NewLLMScorerWithBackend(backend)

	scores, err := scorer.Score(context.Background(),
		Query{Title: "Dune"},
		[]Candidate{{Title: "A"}, {Title: "B"}, {Title: "C"}},
	)
	require.NoError(t, err)
	assert.InDelta(t, 0.91, scores[0], 0.0001)
	assert.InDelta(t, 0.42, scores[1], 0.0001)
	assert.InDelta(t, 0.15, scores[2], 0.0001)
}

func TestLLMScorer_MissingIndexDefaultsToZero(t *testing.T) {
	// LLM skipped index 1 — the scorer should fill it with 0.0 rather
	// than shifting the remaining scores.
	backend, _ := newFakeBackend(t, []MetadataLLMScore{
		{Index: 0, Score: 0.91},
		{Index: 2, Score: 0.15},
	}, nil)
	scorer := NewLLMScorerWithBackend(backend)

	scores, err := scorer.Score(context.Background(),
		Query{Title: "Dune"},
		[]Candidate{{Title: "A"}, {Title: "B"}, {Title: "C"}},
	)
	require.NoError(t, err)
	require.Len(t, scores, 3)
	assert.InDelta(t, 0.91, scores[0], 0.0001)
	assert.InDelta(t, 0.0, scores[1], 0.0001, "missing index should default to 0")
	assert.InDelta(t, 0.15, scores[2], 0.0001)
}

func TestLLMScorer_ClampsOutOfRange(t *testing.T) {
	// LLM returns 1.2 and -0.3 — scorer clamps to [0, 1].
	backend, _ := newFakeBackend(t, []MetadataLLMScore{
		{Index: 0, Score: 1.2},
		{Index: 1, Score: -0.3},
	}, nil)
	scorer := NewLLMScorerWithBackend(backend)
	scores, _ := scorer.Score(context.Background(), Query{}, []Candidate{{}, {}})
	assert.Equal(t, 1.0, scores[0])
	assert.Equal(t, 0.0, scores[1])
}

func TestLLMScorer_BackendError(t *testing.T) {
	backend, _ := newFakeBackend(t, nil, errors.New("openai 503"))
	scorer := NewLLMScorerWithBackend(backend)
	scores, err := scorer.Score(context.Background(),
		Query{Title: "Dune"},
		[]Candidate{{Title: "Dune"}},
	)
	require.Error(t, err)
	assert.Nil(t, scores, "partial results are never returned")
}
