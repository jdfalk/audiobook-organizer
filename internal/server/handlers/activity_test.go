package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

func TestActivityHandler_ListOperationActivity_ReturnsActivityEntries(t *testing.T) {
	gin.SetMode(gin.TestMode)

	now := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	svc := &mockActivityService{
		QueryFn: func(filter database.ActivityFilter) ([]database.ActivityEntry, int, error) {
			return []database.ActivityEntry{{
				OperationID: "op-123",
				Level:       "info",
				Type:        "metadata-apply",
				Source:      "metafetch",
				Summary:     "metadata applied",
				Timestamp:   now,
				Tags:        []string{"op:op-123", "source:metafetch"},
			}}, 1, nil
		},
	}

	handler := NewActivityHandler(svc, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/operations/op-123/activity", nil)
	c.Params = gin.Params{gin.Param{Key: "id", Value: "op-123"}}

	handler.ListOperationActivity(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	var resp struct {
		Data struct {
			OperationID string                    `json:"operation_id"`
			Entries     []OperationActivityEntry `json:"entries"`
			Total       int                       `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Data.OperationID != "op-123" {
		t.Fatalf("operation_id mismatch: want %q, got %q", "op-123", resp.Data.OperationID)
	}
	if resp.Data.Total != 1 {
		t.Fatalf("expected total 1, got %d", resp.Data.Total)
	}
	if len(resp.Data.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(resp.Data.Entries))
	}

	entry := resp.Data.Entries[0]
	if entry.Message != "metadata applied" {
		t.Errorf("unexpected message: %q", entry.Message)
	}
	if entry.OperationType != "metadata-apply" {
		t.Errorf("unexpected operation type: %q", entry.OperationType)
	}
}

func TestActivityHandler_ListOperationActivity_FallbackToOpLogs(t *testing.T) {
	gin.SetMode(gin.TestMode)

	opsStore := &mockActivityOpsStore{
		logs: []database.OpLogV2Row{{
			OperationID: "op-metadata",
			Level:       "info",
			Message:     "metadata fetch started",
			Attrs:       `{"def_id":"metadata_fetch","plugin":"metafetch"}`,
			CreatedAt:   time.Date(2026, 6, 4, 12, 5, 0, 0, time.UTC),
		}},
	}

	svc := &mockActivityService{
		QueryFn: func(filter database.ActivityFilter) ([]database.ActivityEntry, int, error) {
			return nil, 0, nil
		},
	}

	handler := NewActivityHandler(svc, opsStore)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/operations/op-metadata/activity", nil)
	c.Params = gin.Params{gin.Param{Key: "id", Value: "op-metadata"}}

	handler.ListOperationActivity(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	var resp struct {
		Data struct {
			OperationID string                    `json:"operation_id"`
			Entries     []OperationActivityEntry `json:"entries"`
			Total       int                       `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Data.OperationID != "op-metadata" {
		t.Fatalf("operation_id mismatch: want %q, got %q", "op-metadata", resp.Data.OperationID)
	}
	if resp.Data.Total != 1 {
		t.Fatalf("expected total 1, got %d", resp.Data.Total)
	}
	if len(resp.Data.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(resp.Data.Entries))
	}

	entry := resp.Data.Entries[0]
	if entry.OperationType != "metadata_fetch" {
		t.Fatalf("unexpected operation type: %q", entry.OperationType)
	}
	if entry.Message != "metadata fetch started" {
		t.Errorf("message mismatch: %q", entry.Message)
	}
	if entry.Details != `{"def_id":"metadata_fetch","plugin":"metafetch"}` {
		t.Errorf("details mismatch: %s", entry.Details)
	}

	found := false
	for _, tag := range entry.Tags {
		if tag == "def:metadata_fetch" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected def:metadata_fetch tag, got %v", entry.Tags)
	}
}

type mockActivityService struct {
	QueryFn            func(database.ActivityFilter) ([]database.ActivityEntry, int, error)
	DistinctSourcesFn  func(database.ActivityFilter) ([]database.SourceCount, error)
	RecompactDigestsFn func(context.Context) (database.RecompactResult, error)
	CompactByDayFn     func(context.Context, time.Time) (database.CompactResult, error)
}

func (m *mockActivityService) Query(filter database.ActivityFilter) ([]database.ActivityEntry, int, error) {
	if m.QueryFn != nil {
		return m.QueryFn(filter)
	}
	return nil, 0, nil
}

func (m *mockActivityService) GetDistinctSources(filter database.ActivityFilter) ([]database.SourceCount, error) {
	if m.DistinctSourcesFn != nil {
		return m.DistinctSourcesFn(filter)
	}
	return nil, nil
}

func (m *mockActivityService) RecompactDigests(ctx context.Context) (database.RecompactResult, error) {
	if m.RecompactDigestsFn != nil {
		return m.RecompactDigestsFn(ctx)
	}
	return database.RecompactResult{}, nil
}

func (m *mockActivityService) CompactByDay(ctx context.Context, cutoff time.Time) (database.CompactResult, error) {
	if m.CompactByDayFn != nil {
		return m.CompactByDayFn(ctx, cutoff)
	}
	return database.CompactResult{}, nil
}

type mockActivityOpsStore struct {
	logs []database.OpLogV2Row
	err  error
}

func (m *mockActivityOpsStore) GetOpLogsV2(id string, limit int) ([]database.OpLogV2Row, error) {
	return m.logs, m.err
}
