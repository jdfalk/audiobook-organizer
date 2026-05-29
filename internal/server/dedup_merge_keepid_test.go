// file: internal/server/dedup_merge_keepid_test.go
// version: 1.0.0
// guid: e8a5ab90-9d5c-4cbb-8a19-2f3b5cc8a401

package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/cockroachdb/pebble/v2"
	"github.com/gin-gonic/gin"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// newDedupMergeKeepIDTestServer wires up the minimal Server needed to exercise
// mergeDedupCandidate's keep_id validation path. mergeService is left nil on
// purpose — that branch is skipped, leaving just the input-validation logic
// (which is what this fix changed) and the status update.
func newDedupMergeKeepIDTestServer(t *testing.T) (*Server, *database.EmbeddingStore) {
	t.Helper()
	db, err := pebble.Open(t.TempDir(), &pebble.Options{})
	if err != nil {
		t.Fatalf("pebble.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	es := database.NewEmbeddingStore(db)
	srv := &Server{embeddingStore: es}
	return srv, es
}

// insertCandidate creates a pending book dedup candidate and returns its
// numeric id, plus the canonicalised A/B ids (UpsertCandidate swaps so A < B).
func insertCandidate(t *testing.T, es *database.EmbeddingStore, aID, bID string) (int64, string, string) {
	t.Helper()
	sim := 0.95
	if err := es.UpsertCandidate(database.DedupCandidate{
		EntityType: "book",
		EntityAID:  aID,
		EntityBID:  bID,
		Layer:      "embedding",
		Similarity: &sim,
		Status:     "pending",
	}); err != nil {
		t.Fatalf("UpsertCandidate: %v", err)
	}

	// Canonicalise: UpsertCandidate swaps so smaller ID is A.
	cA, cB := aID, bID
	if cA > cB {
		cA, cB = cB, cA
	}

	cands, _, err := es.ListCandidates(database.CandidateFilter{
		EntityType: "book",
		Status:     "pending",
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("ListCandidates: %v", err)
	}
	for _, c := range cands {
		if c.EntityAID == cA && c.EntityBID == cB {
			return c.ID, cA, cB
		}
	}
	t.Fatalf("inserted candidate not found in list")
	return 0, "", ""
}

func doMergeRequest(t *testing.T, srv *Server, candidateID int64, body any) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	var reqBody []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reqBody = b
	}
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/dedup/candidates/"+strconv.FormatInt(candidateID, 10)+"/merge",
		bytes.NewReader(reqBody))
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: strconv.FormatInt(candidateID, 10)}}

	srv.mergeDedupCandidate(c)
	return w
}

// TestMergeDedupCandidate_KeepID_Invalid asserts a keep_id that matches
// neither side returns 400.
func TestMergeDedupCandidate_KeepID_Invalid(t *testing.T) {
	srv, es := newDedupMergeKeepIDTestServer(t)
	id, _, _ := insertCandidate(t, es, "book-aaa", "book-bbb")

	w := doMergeRequest(t, srv, id, map[string]string{"keep_id": "book-not-in-pair"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid keep_id: status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

// TestMergeDedupCandidate_KeepID_A asserts keep_id=A is accepted and echoed.
// (mergeService is nil so MergeBooks is skipped; we're validating the
// boundary between handler input parsing and the merge call.)
func TestMergeDedupCandidate_KeepID_A(t *testing.T) {
	srv, es := newDedupMergeKeepIDTestServer(t)
	id, aID, _ := insertCandidate(t, es, "book-aaa", "book-bbb")

	w := doMergeRequest(t, srv, id, map[string]string{"keep_id": aID})
	if w.Code != http.StatusOK {
		t.Fatalf("keep_id=A: status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			Status string `json:"status"`
			KeepID string `json:"keep_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode resp: %v; body=%s", err, w.Body.String())
	}
	if resp.Data.KeepID != aID {
		t.Errorf("keep_id echoed = %q, want %q", resp.Data.KeepID, aID)
	}
}

// TestMergeDedupCandidate_KeepID_B asserts keep_id=B is accepted and echoed.
func TestMergeDedupCandidate_KeepID_B(t *testing.T) {
	srv, es := newDedupMergeKeepIDTestServer(t)
	id, _, bID := insertCandidate(t, es, "book-aaa", "book-bbb")

	w := doMergeRequest(t, srv, id, map[string]string{"keep_id": bID})
	if w.Code != http.StatusOK {
		t.Fatalf("keep_id=B: status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			KeepID string `json:"keep_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode resp: %v; body=%s", err, w.Body.String())
	}
	if resp.Data.KeepID != bID {
		t.Errorf("keep_id echoed = %q, want %q", resp.Data.KeepID, bID)
	}
}

// TestMergeDedupCandidate_KeepID_Empty asserts back-compat: no body / no
// keep_id is still accepted (auto-select path).
func TestMergeDedupCandidate_KeepID_Empty(t *testing.T) {
	srv, es := newDedupMergeKeepIDTestServer(t)
	id, _, _ := insertCandidate(t, es, "book-aaa", "book-bbb")

	w := doMergeRequest(t, srv, id, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("empty body: status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			KeepID string `json:"keep_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode resp: %v; body=%s", err, w.Body.String())
	}
	if resp.Data.KeepID != "" {
		t.Errorf("keep_id with empty body = %q, want empty", resp.Data.KeepID)
	}
}
