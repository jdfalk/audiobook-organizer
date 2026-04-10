// file: internal/ai/metadata_llm_review_test.go
// version: 1.0.0
// guid: a7d31f84-6b52-4e89-c0d2-5f8a19e47b35

package ai

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMetadataLLMScore_JSONShape locks in the struct shape the
// ScoreMetadataCandidates response is expected to unmarshal into. It's a
// compile-time check wrapped in a runtime assertion: if the types change
// in a breaking way this test stops compiling.
func TestMetadataLLMScore_JSONShape(t *testing.T) {
	score := MetadataLLMScore{
		Index:  3,
		Score:  0.92,
		Reason: "Same book, different subtitle format",
	}
	assert.Equal(t, 3, score.Index)
	assert.InDelta(t, 0.92, score.Score, 0.0001)
	assert.Equal(t, "Same book, different subtitle format", score.Reason)
}

// TestMetadataLLMQuery_FieldMapping verifies the Query/Candidate field
// layout the caller uses.
func TestMetadataLLMQuery_FieldMapping(t *testing.T) {
	q := MetadataLLMQuery{
		Title:    "Dune",
		Author:   "Frank Herbert",
		Narrator: "Scott Brick",
	}
	assert.Equal(t, "Dune", q.Title)
	assert.Equal(t, "Frank Herbert", q.Author)
	assert.Equal(t, "Scott Brick", q.Narrator)
}
