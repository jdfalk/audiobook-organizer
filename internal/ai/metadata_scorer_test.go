// file: internal/ai/metadata_scorer_test.go
// version: 1.2.0
// guid: f38c92ad-67f7-4820-b5d5-a1d25d7426b8

// Lives in package ai_test (black-box) so it can import the
// mockery-generated mocks from internal/ai/mocks without creating
// an import cycle (ai → ai/mocks → ai).

package ai_test

import (
	"context"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/ai"
	"github.com/jdfalk/audiobook-organizer/internal/ai/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// newStubScorer returns a mockery-generated MockMetadataCandidateScorer
// configured to return a constant 0.5 score per candidate, or (nil, nil)
// when the input slice is empty (per the interface contract).
func newStubScorer(t *testing.T, name string) *mocks.MockMetadataCandidateScorer {
	t.Helper()
	m := mocks.NewMockMetadataCandidateScorer(t)
	m.EXPECT().Name().Return(name).Maybe()
	m.EXPECT().Score(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, q ai.Query, cands []ai.Candidate) ([]float64, error) {
			if len(cands) == 0 {
				return nil, nil
			}
			out := make([]float64, len(cands))
			for i := range out {
				out[i] = 0.5
			}
			return out, nil
		}).Maybe()
	return m
}

// TestMetadataCandidateScorer_InterfaceShape verifies that a concrete
// implementation satisfies the interface and can be assigned to the type.
// This is a compile-time check in disguise; if the interface changes in a
// breaking way, this test stops compiling.
func TestMetadataCandidateScorer_InterfaceShape(t *testing.T) {
	var scorer ai.MetadataCandidateScorer = newStubScorer(t, "stub")
	assert.Equal(t, "stub", scorer.Name())

	scores, err := scorer.Score(context.Background(), ai.Query{Title: "Dune"}, []ai.Candidate{
		{Title: "Dune"},
		{Title: "Dune Messiah"},
	})
	require.NoError(t, err)
	assert.Len(t, scores, 2)
	for _, s := range scores {
		assert.GreaterOrEqual(t, s, 0.0)
		assert.LessOrEqual(t, s, 1.0)
	}
}

// TestMetadataCandidateScorer_EmptyCandidates verifies the documented
// "nil candidates, nil score slice" behavior.
func TestMetadataCandidateScorer_EmptyCandidates(t *testing.T) {
	scorer := newStubScorer(t, "stub")
	scores, err := scorer.Score(context.Background(), ai.Query{Title: "Dune"}, nil)
	require.NoError(t, err)
	assert.Nil(t, scores)
}
