// file: internal/server/activity_integration_test.go
// version: 1.0.0
// guid: f8a3b2c1-d4e5-6f7a-8b9c-0d1e2f3a4b5c

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

func TestActivity_Integration_RecordAndHTTPQuery(t *testing.T) {
	// Setup: create temp store, service, and router
	dbPath := filepath.Join(t.TempDir(), "activity_integ.db")
	store, err := database.NewActivityStore(dbPath)
	if err != nil {
		t.Fatalf("NewActivityStore: %v", err)
	}
	defer store.Close()

	svc := NewActivityService(store)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	srv := &Server{activityService: svc}
	r.GET("/api/v1/activity", srv.listActivity)

	// Simulate an iTunes sync writing activity
	_ = svc.Record(database.ActivityEntry{
		Tier:        "change",
		Type:        "itunes_sync",
		Source:      "scheduler",
		OperationID: "op-sync-1",
		Summary:     "Sync: 312 updated, 39 new",
		Details:     map[string]any{"updated": 312, "new": 39},
		Tags:        []string{"scheduled", "itunes"},
	})

	// Simulate a metadata apply
	_ = svc.Record(database.ActivityEntry{
		Tier:    "change",
		Type:    "metadata_apply",
		Source:  "api",
		BookID:  "book-1",
		Summary: "Applied title: old → new",
	})

	// Simulate debug progress
	_ = svc.Record(database.ActivityEntry{
		Tier:        "debug",
		Type:        "progress",
		Source:      "background",
		OperationID: "op-sync-1",
		Summary:     "Processing book 45 of 312...",
	})

	type activityResp struct {
		Entries []database.ActivityEntry `json:"entries"`
		Total   int                      `json:"total"`
	}

	// Query all
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/activity?limit=100", nil)
	r.ServeHTTP(w, req)

	var resp activityResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Total != 3 {
		t.Fatalf("expected 3, got %d", resp.Total)
	}

	// Query by operation
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/activity?operation_id=op-sync-1", nil)
	r.ServeHTTP(w, req)
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 2 {
		t.Errorf("expected 2 entries for op-sync-1, got %d", resp.Total)
	}

	// Query by book
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/activity?book_id=book-1&tier=change", nil)
	r.ServeHTTP(w, req)
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 1 {
		t.Errorf("expected 1 entry for book-1, got %d", resp.Total)
	}

	// Query by tags
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/activity?tags=itunes", nil)
	r.ServeHTTP(w, req)
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 1 {
		t.Errorf("expected 1 entry with tag=itunes, got %d", resp.Total)
	}

	// Verify details round-trip
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/activity?type=itunes_sync", nil)
	r.ServeHTTP(w, req)
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 1 {
		t.Fatalf("expected 1 itunes_sync, got %d", resp.Total)
	}
	if v, ok := resp.Entries[0].Details["updated"]; !ok || v != float64(312) {
		t.Errorf("details.updated mismatch: %v", resp.Entries[0].Details)
	}
}
