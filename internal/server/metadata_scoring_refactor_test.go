// file: internal/server/metadata_scoring_refactor_test.go
// version: 1.1.0
// guid: 3a7c2b1d-e84f-4d59-9f16-0e5a8b2c4d7e

package server

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jdfalk/audiobook-organizer/internal/ai"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
)

// TestScoreOneResult_RefactorEquivalence locks in the current output of
// scoreOneResult against representative inputs so the split into base +
// non-base halves can't accidentally change the combined result.
func TestScoreOneResult_RefactorEquivalence(t *testing.T) {
	searchWords := significantWords("The Way of Kings")

	cases := []struct {
		name   string
		input  metadata.BookMetadata
		minExp float64
		maxExp float64
	}{
		{
			name: "exact title match with rich metadata",
			input: metadata.BookMetadata{
				Title:       "The Way of Kings",
				Description: "long description",
				CoverURL:    "https://example/cover.jpg",
				Narrator:    "Kate Reading",
				ISBN:        "9780765326355",
			},
			// F1 = 1.0, full rich-metadata bonus (+0.15), no penalties
			minExp: 1.10, maxExp: 1.20,
		},
		{
			name: "compilation penalty fires",
			input: metadata.BookMetadata{
				Title: "The Way of Kings (Stormlight Archive Omnibus)",
			},
			// Contains "omnibus" → compilation multiplier 0.15 → tiny score
			minExp: 0.0, maxExp: 0.20,
		},
		{
			name: "unrelated title",
			input: metadata.BookMetadata{
				Title: "Completely Different Book",
			},
			// F1 ~ 0 (no overlap) + no bonus
			minExp: 0.0, maxExp: 0.05,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := scoreOneResult(tc.input, searchWords)
			assert.GreaterOrEqual(t, got, tc.minExp, "score below expected range")
			assert.LessOrEqual(t, got, tc.maxExp, "score above expected range")
		})
	}
}

// scorerStub is a controllable MetadataCandidateScorer for tests.
type scorerStub struct {
	name      string
	scores    []float64
	err       error
	callCount int
}

func (s *scorerStub) Score(ctx context.Context, q ai.Query, cands []ai.Candidate) ([]float64, error) {
	s.callCount++
	if s.err != nil {
		return nil, s.err
	}
	return s.scores, nil
}

func (s *scorerStub) Name() string { return s.name }

func TestScoreBaseCandidates_EmbeddingTierUsed(t *testing.T) {
	mfs := &MetadataFetchService{metadataScorer: &scorerStub{
		name:   "embedding",
		scores: []float64{0.9, 0.7, 0.3},
	}}
	prev := config.AppConfig.MetadataEmbeddingScoringEnabled
	config.AppConfig.MetadataEmbeddingScoringEnabled = true
	defer func() { config.AppConfig.MetadataEmbeddingScoringEnabled = prev }()

	results := []metadata.BookMetadata{
		{Title: "A"}, {Title: "B"}, {Title: "C"},
	}
	searchWords := significantWords("A")
	book := &database.Book{ID: "BOOK1", Title: "A"}

	scores, tier := mfs.scoreBaseCandidates(context.Background(), book, results, searchWords)
	assert.Equal(t, "embedding", tier)
	assert.Equal(t, []float64{0.9, 0.7, 0.3}, scores)
}

func TestScoreBaseCandidates_ConfigDisabledFallsBackToF1(t *testing.T) {
	mfs := &MetadataFetchService{metadataScorer: &scorerStub{
		name:   "embedding",
		scores: []float64{1.0, 1.0, 1.0},
	}}
	prev := config.AppConfig.MetadataEmbeddingScoringEnabled
	config.AppConfig.MetadataEmbeddingScoringEnabled = false
	defer func() { config.AppConfig.MetadataEmbeddingScoringEnabled = prev }()

	results := []metadata.BookMetadata{
		{Title: "The Way of Kings"},
		{Title: "Completely Unrelated Book"},
	}
	searchWords := significantWords("The Way of Kings")
	book := &database.Book{ID: "BOOK1", Title: "The Way of Kings"}

	scores, tier := mfs.scoreBaseCandidates(context.Background(), book, results, searchWords)
	assert.Equal(t, "f1", tier)
	assert.Len(t, scores, 2)
	assert.InDelta(t, 1.0, scores[0], 0.01)
	assert.InDelta(t, 0.0, scores[1], 0.1)
}

func TestScoreBaseCandidates_ScorerErrorFallsBackToF1(t *testing.T) {
	stub := &scorerStub{name: "embedding", err: errors.New("api boom")}
	mfs := &MetadataFetchService{metadataScorer: stub}
	prev := config.AppConfig.MetadataEmbeddingScoringEnabled
	config.AppConfig.MetadataEmbeddingScoringEnabled = true
	defer func() { config.AppConfig.MetadataEmbeddingScoringEnabled = prev }()

	results := []metadata.BookMetadata{{Title: "The Way of Kings"}}
	searchWords := significantWords("The Way of Kings")
	book := &database.Book{ID: "BOOK1", Title: "The Way of Kings"}

	scores, tier := mfs.scoreBaseCandidates(context.Background(), book, results, searchWords)
	assert.Equal(t, "f1", tier, "scorer error should fall back to F1 tier")
	assert.Equal(t, 1, stub.callCount, "scorer should be called exactly once")
	assert.InDelta(t, 1.0, scores[0], 0.01)
}

func TestScoreBaseCandidates_NilScorerFallsBackSilently(t *testing.T) {
	mfs := &MetadataFetchService{metadataScorer: nil}
	prev := config.AppConfig.MetadataEmbeddingScoringEnabled
	config.AppConfig.MetadataEmbeddingScoringEnabled = true
	defer func() { config.AppConfig.MetadataEmbeddingScoringEnabled = prev }()

	results := []metadata.BookMetadata{{Title: "The Way of Kings"}}
	searchWords := significantWords("The Way of Kings")
	book := &database.Book{ID: "BOOK1", Title: "The Way of Kings"}

	scores, tier := mfs.scoreBaseCandidates(context.Background(), book, results, searchWords)
	assert.Equal(t, "f1", tier)
	assert.Len(t, scores, 1)
}
