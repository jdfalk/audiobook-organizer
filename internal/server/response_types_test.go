// file: internal/server/response_types_test.go
// version: 1.1.0
// guid: 8a9b0c1d-2e3f-4a5b-6c7d-8e9f0a1b2c3d

package server

import (
	"encoding/json"
	"testing"
)

func TestNewListResponse(t *testing.T) {
	items := []string{"item1", "item2"}
	resp := NewListResponse(items, 2, 50, 0)

	if resp.Count != 2 {
		t.Errorf("expected count 2, got %d", resp.Count)
	}
	if resp.Limit != 50 {
		t.Errorf("expected limit 50, got %d", resp.Limit)
	}
	if resp.Offset != 0 {
		t.Errorf("expected offset 0, got %d", resp.Offset)
	}
	if resp.Total != 2 {
		t.Errorf("expected total 2, got %d", resp.Total)
	}
}

func TestNewListResponseWithTotal(t *testing.T) {
	items := []string{"item1"}
	resp := NewListResponseWithTotal(items, 1, 50, 0, 100)

	if resp.Count != 1 {
		t.Errorf("expected count 1, got %d", resp.Count)
	}
	if resp.Total != 100 {
		t.Errorf("expected total 100, got %d", resp.Total)
	}
}

func TestNewBulkResponse(t *testing.T) {
	results := []BulkItem{
		{ID: "1", Status: "success"},
		{ID: "2", Status: "success"},
		{ID: "3", Status: "failed", Error: "not found"},
	}
	resp := NewBulkResponse(3, results)

	if resp.Total != 3 {
		t.Errorf("expected total 3, got %d", resp.Total)
	}
	if resp.Succeeded != 2 {
		t.Errorf("expected succeeded 2, got %d", resp.Succeeded)
	}
	if resp.Failed != 1 {
		t.Errorf("expected failed 1, got %d", resp.Failed)
	}
}

func TestNewMessageResponse(t *testing.T) {
	resp := NewMessageResponse("test message", "TEST_CODE")

	if resp.Message != "test message" {
		t.Errorf("expected 'test message', got %q", resp.Message)
	}
	if resp.Code != "TEST_CODE" {
		t.Errorf("expected 'TEST_CODE', got %q", resp.Code)
	}
}

func TestListResponseJSON(t *testing.T) {
	items := []int{1, 2, 3}
	resp := NewListResponse(items, 3, 10, 0)

	data, err := json.Marshal(resp)
	if err != nil {
		t.Errorf("failed to marshal: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Errorf("failed to unmarshal: %v", err)
	}

	if result["count"] != float64(3) {
		t.Errorf("expected count 3 in JSON, got %v", result["count"])
	}
}

func TestAudiobookResponseJSON(t *testing.T) {
	rating := 4.5
	year := 2024
	resp := &AudiobookResponse{
		ID:          "123",
		Title:       "Test Book",
		Author:      "Test Author",
		Rating:      &rating,
		ReleaseYear: &year,
		IsAudiobook: true,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Errorf("failed to marshal: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Errorf("failed to unmarshal: %v", err)
	}

	if result["title"] != "Test Book" {
		t.Errorf("expected title 'Test Book', got %v", result["title"])
	}
	if result["rating"] != 4.5 {
		t.Errorf("expected rating 4.5, got %v", result["rating"])
	}
}

func TestDuplicatesResponse(t *testing.T) {
	groups := []DuplicateGroup{
		{
			Key:   "hash-123",
			Items: 2,
			Details: []DuplicateItem{
				{ID: "1", Title: "Book 1", FilePath: "/path/1"},
				{ID: "2", Title: "Book 1", FilePath: "/path/2"},
			},
		},
	}
	resp := &DuplicatesResponse{
		Groups:         groups,
		GroupCount:     1,
		DuplicateCount: 2,
	}

	if resp.GroupCount != 1 {
		t.Errorf("expected group count 1, got %d", resp.GroupCount)
	}
	if resp.DuplicateCount != 2 {
		t.Errorf("expected duplicate count 2, got %d", resp.DuplicateCount)
	}
}

func TestNewStatusResponse(t *testing.T) {
	t.Run("ok status with nil data", func(t *testing.T) {
		resp := NewStatusResponse("ok", nil)

		if resp.Status != "ok" {
			t.Errorf("expected status %q, got %q", "ok", resp.Status)
		}
		if resp.Data != nil {
			t.Errorf("expected data nil, got %v", resp.Data)
		}
	})

	t.Run("error status with string data", func(t *testing.T) {
		resp := NewStatusResponse("error", "something went wrong")

		if resp.Status != "error" {
			t.Errorf("expected status %q, got %q", "error", resp.Status)
		}
		if resp.Data != "something went wrong" {
			t.Errorf("expected data %q, got %v", "something went wrong", resp.Data)
		}
	})

	t.Run("degraded status with map data", func(t *testing.T) {
		data := map[string]any{"service": "database", "latency": 500}
		resp := NewStatusResponse("degraded", data)

		if resp.Status != "degraded" {
			t.Errorf("expected status %q, got %q", "degraded", resp.Status)
		}
		// Verify the data is not nil (can't directly compare maps)
		if resp.Data == nil {
			t.Error("expected data to be set, got nil")
		}
		// Type assert to verify it's a map
		if _, ok := resp.Data.(map[string]any); !ok {
			t.Errorf("expected data to be map[string]any, got %T", resp.Data)
		}
	})
}
