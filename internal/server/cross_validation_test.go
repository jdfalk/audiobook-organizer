// file: internal/server/cross_validation_test.go
// version: 1.0.0
// guid: e1f7a3b5-8c9d-0e1f-2a3b-4c5d6e7f8a9b

package server

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/stretchr/testify/require"
)

func TestCrossValidateAgreed(t *testing.T) {
	groups := []database.ScanSuggestion{
		{Action: "merge", CanonicalName: "J. N. Chaney", Confidence: "high", AuthorIDs: []int{10, 20}, Reason: "name variants"},
	}
	full := []database.ScanSuggestion{
		{Action: "merge", CanonicalName: "J.N. Chaney", Confidence: "medium", AuthorIDs: []int{10, 20}, Reason: "same author"},
	}

	results := CrossValidate(1, groups, full)
	require.Len(t, results, 1)
	require.Equal(t, "agreed", results[0].Agreement)
	require.Equal(t, "high", results[0].Suggestion.Confidence)            // higher of the two
	require.Equal(t, "J. N. Chaney", results[0].Suggestion.CanonicalName) // groups' canonical preferred
	require.Equal(t, "cross_validate", results[0].Suggestion.Source)
}

func TestCrossValidateDisagreed(t *testing.T) {
	groups := []database.ScanSuggestion{
		{Action: "merge", CanonicalName: "GraphicAudio", Confidence: "high", AuthorIDs: []int{100, 200}},
	}
	full := []database.ScanSuggestion{
		{Action: "reclassify", CanonicalName: "GraphicAudio", Confidence: "high", AuthorIDs: []int{100, 200}},
	}

	results := CrossValidate(1, groups, full)
	require.Len(t, results, 2) // both disagreed results
	require.Equal(t, "disagreed", results[0].Agreement)
	require.Equal(t, "disagreed", results[1].Agreement)
	require.Equal(t, "merge", results[0].Suggestion.Action)
	require.Equal(t, "reclassify", results[1].Suggestion.Action)
}

func TestCrossValidateOneSided(t *testing.T) {
	groups := []database.ScanSuggestion{
		{Action: "merge", CanonicalName: "Stephen King", AuthorIDs: []int{1, 2}},
	}
	full := []database.ScanSuggestion{
		{Action: "reclassify", CanonicalName: "BBC Studios", AuthorIDs: []int{50}},
	}

	results := CrossValidate(1, groups, full)
	require.Len(t, results, 2)

	// Find groups_only and full_only
	var groupsOnly, fullOnly *database.ScanResult
	for i := range results {
		switch results[i].Agreement {
		case "groups_only":
			groupsOnly = &results[i]
		case "full_only":
			fullOnly = &results[i]
		}
	}
	require.NotNil(t, groupsOnly)
	require.NotNil(t, fullOnly)
	require.Equal(t, "Stephen King", groupsOnly.Suggestion.CanonicalName)
	require.Equal(t, "BBC Studios", fullOnly.Suggestion.CanonicalName)
}

func TestCrossValidateIDMerge(t *testing.T) {
	groups := []database.ScanSuggestion{
		{Action: "merge", CanonicalName: "Test", AuthorIDs: []int{1, 2, 3}},
	}
	full := []database.ScanSuggestion{
		{Action: "merge", CanonicalName: "Test", AuthorIDs: []int{2, 3, 4}},
	}

	results := CrossValidate(1, groups, full)
	require.Len(t, results, 1)
	require.Equal(t, "agreed", results[0].Agreement)
	require.ElementsMatch(t, []int{1, 2, 3, 4}, results[0].Suggestion.AuthorIDs)
}

func TestCrossValidateEmpty(t *testing.T) {
	results := CrossValidate(1, nil, nil)
	require.Empty(t, results)
}

func TestHigherConfidence(t *testing.T) {
	require.Equal(t, "high", higherConfidence("high", "low"))
	require.Equal(t, "high", higherConfidence("low", "high"))
	require.Equal(t, "medium", higherConfidence("medium", "low"))
	require.Equal(t, "medium", higherConfidence("medium", "medium"))
}
