// file: internal/server/metadata_scoring_refactor_test.go
// version: 1.0.0
// guid: 3a7c2b1d-e84f-4d59-9f16-0e5a8b2c4d7e

package server

import (
	"testing"

	"github.com/stretchr/testify/assert"

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
