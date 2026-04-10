// file: internal/ai/metadata_scorer_test.go
// version: 1.0.0
// guid: f38c92ad-67f7-4820-b5d5-a1d25d7426b8

package ai

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubScorer is a tiny MetadataCandidateScorer implementation used to lock
// in the interface shape. It always returns 0.5 for every candidate, except
// that an empty input slice returns (nil, nil) per the interface contract.
type stubScorer struct{ name string }

func (s *stubScorer) Score(ctx context.Context, q Query, cands []Candidate) ([]float64, error) {
	if len(cands) == 0 {
		return nil, nil
	}
	out := make([]float64, len(cands))
	for i := range out {
		out[i] = 0.5
	}
	return out, nil
}

func (s *stubScorer) Name() string { return s.name }

// TestMetadataCandidateScorer_InterfaceShape verifies that a concrete
// implementation satisfies the interface and can be assigned to the type.
// This is a compile-time check in disguise; if the interface changes in a
// breaking way, this test stops compiling.
func TestMetadataCandidateScorer_InterfaceShape(t *testing.T) {
	var scorer MetadataCandidateScorer = &stubScorer{name: "stub"}
	assert.Equal(t, "stub", scorer.Name())

	scores, err := scorer.Score(context.Background(), Query{Title: "Dune"}, []Candidate{
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
	scorer := &stubScorer{name: "stub"}
	scores, err := scorer.Score(context.Background(), Query{Title: "Dune"}, nil)
	require.NoError(t, err)
	assert.Nil(t, scores)
}
