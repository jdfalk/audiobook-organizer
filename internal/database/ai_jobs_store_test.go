// file: internal/database/ai_jobs_store_test.go
// version: 1.0.0
// guid: 6da64e72-5521-4eb8-a378-384ec245f31f

package database

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestAIJobsStore(t *testing.T) *SQLiteStore {
	t.Helper()
	store, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	require.NoError(t, RunMigrations(store))
	return store
}

func TestAIJobs_CreateAndGet(t *testing.T) {
	store := newTestAIJobsStore(t)
	defer store.Close()

	job := AIJob{
		ID:             "01TEST",
		Type:           "dedup_review",
		CustomIDPrefix: "01TEST",
		Status:         "pending",
		ItemCount:      5,
		CreatedAt:      time.Now(),
	}
	err := store.CreateAIJob(job, []byte(`[{"idx":1}]`))
	require.NoError(t, err)

	got, err := store.GetAIJob("01TEST")
	require.NoError(t, err)
	assert.Equal(t, "dedup_review", got.Type)
	assert.Equal(t, "pending", got.Status)
	assert.Equal(t, 5, got.ItemCount)

	payload, err := store.GetAIJobPayload("01TEST")
	require.NoError(t, err)
	assert.JSONEq(t, `[{"idx":1}]`, string(payload))
}

func TestAIJobs_UpdateStatus(t *testing.T) {
	store := newTestAIJobsStore(t)
	defer store.Close()

	job := AIJob{ID: "01TEST", Type: "x", CustomIDPrefix: "01TEST", Status: "pending", ItemCount: 1, CreatedAt: time.Now()}
	require.NoError(t, store.CreateAIJob(job, []byte("[]")))

	require.NoError(t, store.MarkAIJobSubmitted("01TEST", "batch_abc123"))
	got, err := store.GetAIJob("01TEST")
	require.NoError(t, err)
	assert.Equal(t, "submitted", got.Status)
	assert.Equal(t, "batch_abc123", got.BatchID)
	assert.False(t, got.SubmittedAt.IsZero())

	require.NoError(t, store.MarkAIJobCompleted("01TEST", "completed_with_errors", 3, 2, []AIJobRowError{
		{CustomID: "01TEST-4", Error: "boom"},
	}))
	got, err = store.GetAIJob("01TEST")
	require.NoError(t, err)
	assert.Equal(t, "completed_with_errors", got.Status)
	assert.Equal(t, 3, got.SuccessCount)
	assert.Equal(t, 2, got.ErrorCount)
	var errs []AIJobRowError
	require.NoError(t, json.Unmarshal([]byte(got.RowErrors), &errs))
	assert.Len(t, errs, 1)
	assert.Equal(t, "01TEST-4", errs[0].CustomID)
}

func TestAIJobs_MarkFailed(t *testing.T) {
	store := newTestAIJobsStore(t)
	defer store.Close()

	job := AIJob{ID: "01TEST", Type: "x", CustomIDPrefix: "01TEST", Status: "pending", ItemCount: 1, CreatedAt: time.Now()}
	require.NoError(t, store.CreateAIJob(job, []byte("[]")))

	require.NoError(t, store.MarkAIJobFailed("01TEST", "quota exceeded"))
	got, err := store.GetAIJob("01TEST")
	require.NoError(t, err)
	assert.Equal(t, "failed", got.Status)
	assert.Equal(t, "quota exceeded", got.ErrorMsg)
}

func TestAIJobs_LookupByBatchID(t *testing.T) {
	store := newTestAIJobsStore(t)
	defer store.Close()

	job := AIJob{ID: "01TEST", Type: "x", CustomIDPrefix: "01TEST", Status: "pending", ItemCount: 1, CreatedAt: time.Now()}
	require.NoError(t, store.CreateAIJob(job, []byte("[]")))
	require.NoError(t, store.MarkAIJobSubmitted("01TEST", "batch_xyz"))

	got, err := store.GetAIJobByBatchID("batch_xyz")
	require.NoError(t, err)
	assert.Equal(t, "01TEST", got.ID)
}

func TestAIJobs_List(t *testing.T) {
	store := newTestAIJobsStore(t)
	defer store.Close()

	for i, status := range []string{"pending", "submitted", "completed"} {
		job := AIJob{
			ID:             string(rune('A'+i)) + "1",
			Type:           "dedup_review",
			CustomIDPrefix: "x",
			Status:         status,
			ItemCount:      1,
			CreatedAt:      time.Now(),
		}
		require.NoError(t, store.CreateAIJob(job, []byte("[]")))
	}

	all, err := store.ListAIJobs("", "", 10, 0)
	require.NoError(t, err)
	assert.Len(t, all, 3)

	pending, err := store.ListAIJobs("dedup_review", "pending", 10, 0)
	require.NoError(t, err)
	assert.Len(t, pending, 1)
}
