// file: internal/ai/metadata_llm_review_aijobs_test.go
// version: 1.0.0
// guid: 3d4e5f6a-7b8c-9d0e-1f2a-3b4c5d6e7f8a

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/ai/aijobs"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeMetadataApplier records scores for inspection in tests.
type fakeMetadataApplier struct {
	appliedScores map[string][]MetadataLLMScore // keyed by query title
}

func newFakeMetadataApplier() *fakeMetadataApplier {
	return &fakeMetadataApplier{
		appliedScores: make(map[string][]MetadataLLMScore),
	}
}

func (f *fakeMetadataApplier) ApplyMetadataScores(query MetadataLLMQuery, scores []MetadataLLMScore) int {
	key := query.Title
	f.appliedScores[key] = append(f.appliedScores[key], scores...)
	return len(scores)
}

// fakeStoreForMetadata implements AIJobsStore for metadata tests.
type fakeStoreForMetadata struct {
	jobs     map[string]database.AIJob
	payloads map[string][]byte
}

func newFakeStoreForMetadata() *fakeStoreForMetadata {
	return &fakeStoreForMetadata{
		jobs:     map[string]database.AIJob{},
		payloads: map[string][]byte{},
	}
}

func (f *fakeStoreForMetadata) CreateAIJob(j database.AIJob, p []byte) error {
	f.jobs[j.ID] = j
	f.payloads[j.ID] = p
	return nil
}
func (f *fakeStoreForMetadata) GetAIJob(id string) (database.AIJob, error)           { return f.jobs[id], nil }
func (f *fakeStoreForMetadata) GetAIJobByBatchID(b string) (database.AIJob, error) {
	for _, j := range f.jobs {
		if j.BatchID == b {
			return j, nil
		}
	}
	return database.AIJob{}, errors.New("not found")
}
func (f *fakeStoreForMetadata) GetAIJobPayload(id string) ([]byte, error) { return f.payloads[id], nil }
func (f *fakeStoreForMetadata) MarkAIJobSubmitted(id, b string) error {
	j := f.jobs[id]
	j.Status = "submitted"
	j.BatchID = b
	f.jobs[id] = j
	return nil
}
func (f *fakeStoreForMetadata) MarkAIJobCompleted(id, status string, s, e int, re []database.AIJobRowError) error {
	j := f.jobs[id]
	j.Status = status
	j.SuccessCount = s
	j.ErrorCount = e
	if len(re) > 0 {
		b, _ := json.Marshal(re)
		j.RowErrors = string(b)
	}
	f.jobs[id] = j
	return nil
}
func (f *fakeStoreForMetadata) MarkAIJobFailed(id, msg string) error {
	j := f.jobs[id]
	j.Status = "failed"
	j.ErrorMsg = msg
	f.jobs[id] = j
	return nil
}
func (f *fakeStoreForMetadata) ListAIJobs(t, s string, l, o int) ([]database.AIJob, error) {
	var out []database.AIJob
	for _, j := range f.jobs {
		if t != "" && j.Type != t {
			continue
		}
		if s != "" && j.Status != s {
			continue
		}
		out = append(out, j)
	}
	return out, nil
}

// fakeBatchClientForMetadata records batch submissions.
type fakeBatchClientForMetadata struct {
	uploadCalls int
	createCalls int
	lastJSONL   []byte
}

func (f *fakeBatchClientForMetadata) UploadBatchFile(ctx context.Context, data []byte) (string, error) {
	f.uploadCalls++
	f.lastJSONL = append([]byte(nil), data...)
	return "file_metadata_test", nil
}
func (f *fakeBatchClientForMetadata) CreateBatchWithMetadata(ctx context.Context, fileID, batchType string) (string, error) {
	f.createCalls++
	return "batch_metadata_test", nil
}

// TestMetadataReviewCallback_HappyPath tests the callback with one successful row.
func TestMetadataReviewCallback_HappyPath(t *testing.T) {
	applier := newFakeMetadataApplier()

	query := MetadataLLMQuery{Title: "Dune", Author: "Frank Herbert"}
	payload := metadataReviewPayload{
		Query: query,
		Inputs: []MetadataLLMCandidate{
			{Index: 0, Title: "Dune", Author: "Frank Herbert"},
			{Index: 1, Title: "Dune Messiah", Author: "Frank Herbert"},
		},
	}
	payloadJSON, err := json.Marshal(payload)
	require.NoError(t, err)

	scoreJSON, err := json.Marshal(struct {
		Scores []MetadataLLMScore `json:"scores"`
	}{
		Scores: []MetadataLLMScore{
			{Index: 0, Score: 0.95, Reason: "exact match"},
			{Index: 1, Score: 0.42, Reason: "different book"},
		},
	})
	require.NoError(t, err)

	results := []aijobs.RowResult{
		{CustomID: "job1-0", Content: string(scoreJSON)},
	}

	// Set the applier before calling the callback.
	oldApplier := metadataReviewApplier
	metadataReviewApplier = applier
	defer func() { metadataReviewApplier = oldApplier }()

	success, fail, rowErrors, fatalErr := metadataReviewCallback(context.Background(), payloadJSON, results)

	require.NoError(t, fatalErr)
	assert.Equal(t, 1, success, "expected 1 success")
	assert.Equal(t, 0, fail, "expected 0 failures")
	assert.Len(t, rowErrors, 0, "expected no row errors")
	assert.Len(t, applier.appliedScores[query.Title], 2, "expected 2 scores applied")
	assert.InDelta(t, 0.95, applier.appliedScores[query.Title][0].Score, 0.0001)
	assert.InDelta(t, 0.42, applier.appliedScores[query.Title][1].Score, 0.0001)
}

// TestMetadataReviewCallback_PerRowErrorsIsolated tests that errors in one row don't affect others.
func TestMetadataReviewCallback_PerRowErrorsIsolated(t *testing.T) {
	applier := newFakeMetadataApplier()

	query := MetadataLLMQuery{Title: "The Way of Kings"}
	payload := metadataReviewPayload{
		Query: query,
		Inputs: []MetadataLLMCandidate{
			{Index: 0, Title: "The Way of Kings"},
			{Index: 1, Title: "Words of Radiance"},
			{Index: 2, Title: "Oathbringer"},
		},
	}
	payloadJSON, err := json.Marshal(payload)
	require.NoError(t, err)

	goodScoreJSON, err := json.Marshal(struct {
		Scores []MetadataLLMScore `json:"scores"`
	}{
		Scores: []MetadataLLMScore{
			{Index: 0, Score: 0.98, Reason: "perfect match"},
		},
	})
	require.NoError(t, err)

	results := []aijobs.RowResult{
		{CustomID: "job1-0", Content: string(goodScoreJSON)}, // Success
		{CustomID: "job1-1", Err: "api error"},               // Error from OpenAI
		{CustomID: "job1-2", Content: "not valid json"},      // Parse error
	}

	oldApplier := metadataReviewApplier
	metadataReviewApplier = applier
	defer func() { metadataReviewApplier = oldApplier }()

	success, fail, rowErrors, fatalErr := metadataReviewCallback(context.Background(), payloadJSON, results)

	require.NoError(t, fatalErr)
	assert.Equal(t, 1, success, "expected 1 success")
	assert.Equal(t, 2, fail, "expected 2 failures")
	assert.Len(t, rowErrors, 2, "expected 2 row errors")
	assert.Len(t, applier.appliedScores[query.Title], 1, "expected only 1 score despite 3 rows")
}

// TestSubmitMetadataReviewJob_SplitsIntoSubBatches tests that 51 candidates split into 3 JSONL rows (25+25+1).
func TestSubmitMetadataReviewJob_SplitsIntoSubBatches(t *testing.T) {
	store := newFakeStoreForMetadata()
	client := &fakeBatchClientForMetadata{}

	deps := aijobs.Deps{
		Store:  store,
		Client: client,
	}

	// Build 51 candidates.
	query := MetadataLLMQuery{Title: "Test Book", Author: "Test Author"}
	candidates := make([]MetadataLLMCandidate, 51)
	for i := 0; i < 51; i++ {
		candidates[i] = MetadataLLMCandidate{
			Index:  i,
			Title:  "Book " + string(rune('A'+i%26)),
			Author: "Author " + string(rune('A'+i%26)),
		}
	}

	jobID, err := SubmitMetadataReviewJob(context.Background(), deps, "gpt-5-mini", query, candidates)
	require.NoError(t, err)
	require.NotEmpty(t, jobID)

	// Verify JSONL has 3 lines (sub-batches).
	lines := 0
	jsonlBytes := client.lastJSONL
	for _, b := range jsonlBytes {
		if b == '\n' {
			lines++
		}
	}
	assert.Equal(t, 3, lines, "expected 3 JSONL lines (sub-batches)")

	// Verify the job was created.
	job, err := store.GetAIJob(jobID)
	require.NoError(t, err)
	assert.Equal(t, "metadata_review", job.Type)
	assert.Equal(t, 3, job.ItemCount, "expected ItemCount=3 sub-batches")
}

// TestMetadataReviewCallback_NoApplier tests graceful handling when no applier is set.
func TestMetadataReviewCallback_NoApplier(t *testing.T) {
	query := MetadataLLMQuery{Title: "Test"}
	payload := metadataReviewPayload{
		Query: query,
		Inputs: []MetadataLLMCandidate{
			{Index: 0, Title: "Test"},
		},
	}
	payloadJSON, err := json.Marshal(payload)
	require.NoError(t, err)

	scoreJSON, err := json.Marshal(struct {
		Scores []MetadataLLMScore `json:"scores"`
	}{
		Scores: []MetadataLLMScore{
			{Index: 0, Score: 0.9, Reason: "match"},
		},
	})
	require.NoError(t, err)

	results := []aijobs.RowResult{
		{CustomID: "job1-0", Content: string(scoreJSON)},
	}

	// Ensure no applier is set.
	oldApplier := metadataReviewApplier
	metadataReviewApplier = nil
	defer func() { metadataReviewApplier = oldApplier }()

	success, fail, rowErrors, fatalErr := metadataReviewCallback(context.Background(), payloadJSON, results)

	// Should succeed even without applier.
	require.NoError(t, fatalErr)
	assert.Equal(t, 1, success)
	assert.Equal(t, 0, fail)
	assert.Len(t, rowErrors, 0)
}
