// file: internal/server/file_ops_handlers_test.go
// version: 1.0.0
// guid: 2c1f8b4d-9e3a-4f50-a7d6-8b1c5e0f9a23

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHandleListPendingFileOps_NoPool(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prev := GetGlobalFileIOPool()
	SetGlobalFileIOPool(nil)
	t.Cleanup(func() { SetGlobalFileIOPool(prev) })

	srv := &Server{router: gin.New()}
	srv.router.GET("/api/v1/file-ops/pending", srv.handleListPendingFileOps)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/file-ops/pending", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Count      int             `json:"count"`
		Operations []pendingFileOp `json:"operations"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Count != 0 || len(resp.Operations) != 0 {
		t.Errorf("expected empty result, got count=%d ops=%v", resp.Count, resp.Operations)
	}
}

func TestHandleListPendingFileOps_PopulatedPool(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Pool with no workers — Submit will overflow but we just want to populate
	// the in-memory `pending` map; we don't need the work to actually run.
	pool := &FileIOPool{
		ch:       make(chan fileIOJobEntry, 8),
		overflow: make(chan struct{}, 1),
	}
	prev := GetGlobalFileIOPool()
	SetGlobalFileIOPool(pool)
	t.Cleanup(func() { SetGlobalFileIOPool(prev) })

	pool.SubmitTyped("book-A", "apply_metadata", func() {})
	pool.SubmitTyped("book-B", "tag_writeback", func() {})

	srv := &Server{router: gin.New()}
	srv.router.GET("/api/v1/file-ops/pending", srv.handleListPendingFileOps)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/file-ops/pending", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Count      int             `json:"count"`
		Operations []pendingFileOp `json:"operations"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Count != 2 {
		t.Fatalf("count = %d, want 2 (response: %s)", resp.Count, w.Body.String())
	}

	seen := map[string]string{}
	for _, op := range resp.Operations {
		seen[op.BookID] = op.OpType
	}
	if seen["book-A"] != "apply_metadata" {
		t.Errorf("book-A op_type = %q, want apply_metadata", seen["book-A"])
	}
	if seen["book-B"] != "tag_writeback" {
		t.Errorf("book-B op_type = %q, want tag_writeback", seen["book-B"])
	}
}

func TestHandleListPendingFileOps_SortedByStartedAt(t *testing.T) {
	gin.SetMode(gin.TestMode)

	pool := &FileIOPool{
		ch:       make(chan fileIOJobEntry, 8),
		overflow: make(chan struct{}, 1),
	}
	prev := GetGlobalFileIOPool()
	SetGlobalFileIOPool(pool)
	t.Cleanup(func() { SetGlobalFileIOPool(prev) })

	// Submit A first — should sort earlier than B since started_at is set in SubmitTyped.
	pool.SubmitTyped("first", "apply_metadata", func() {})
	pool.SubmitTyped("second", "apply_metadata", func() {})
	pool.SubmitTyped("third", "apply_metadata", func() {})

	srv := &Server{router: gin.New()}
	srv.router.GET("/api/v1/file-ops/pending", srv.handleListPendingFileOps)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/file-ops/pending", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "first") || !strings.Contains(body, "second") || !strings.Contains(body, "third") {
		t.Fatalf("missing expected book IDs in response: %s", body)
	}

	var resp struct {
		Count      int             `json:"count"`
		Operations []pendingFileOp `json:"operations"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Operations) != 3 {
		t.Fatalf("got %d ops, want 3", len(resp.Operations))
	}
	for i := 1; i < len(resp.Operations); i++ {
		if resp.Operations[i].StartedAt.Before(resp.Operations[i-1].StartedAt) {
			t.Errorf("operations not sorted by started_at: %v before %v",
				resp.Operations[i].StartedAt, resp.Operations[i-1].StartedAt)
		}
	}
}
