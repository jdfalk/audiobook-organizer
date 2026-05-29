// file: internal/server/split_book_handlers.go
// version: 1.0.0
// guid: 5d7e9f1a-4b6c-8d0e-2f3a-5b7c9d1e3f4a
// last-edited: 2026-05-29

// HTTP handlers for the split-book backfill (MAYDEPLOY-G2 + G4).
//
// Endpoints:
//
//	POST /api/v1/dedup/split-book-scan                   — enqueue scan
//	GET  /api/v1/dedup/split-book-candidates             — list candidates
//	POST /api/v1/dedup/split-book-candidates/:id/merge   — execute merge

package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	dedupengine "github.com/jdfalk/audiobook-organizer/internal/dedup"
	"github.com/jdfalk/audiobook-organizer/internal/httputil"
)

// triggerSplitBookScan handles POST /api/v1/dedup/split-book-scan.
// Delegates to the UOS registry (dedup.split-book-scan op).
func (s *Server) triggerSplitBookScan(c *gin.Context) {
	if s.opRegistry == nil {
		httputil.RespondWithInternalError(c, "operation registry not initialized")
		return
	}
	opID, err := s.opRegistry.EnqueueOp(c.Request.Context(), "dedup.split-book-scan", nil)
	if err != nil {
		httputil.InternalError(c, "failed to enqueue split-book scan", err)
		return
	}
	httputil.RespondWithSuccess(c, http.StatusAccepted, map[string]string{"op_id": opID})
}

// listSplitBookCandidates handles GET /api/v1/dedup/split-book-candidates.
//
// Returns every persisted split-book candidate cluster. Pagination is
// not applied — the candidate set is expected to be small (hundreds at
// most) and operator-facing tooling reads the whole list at once.
func (s *Server) listSplitBookCandidates(c *gin.Context) {
	store, ok := s.splitBookStore()
	if !ok {
		httputil.RespondWithServiceUnavailable(c, "split-book store not available")
		return
	}
	cands, err := store.List()
	if err != nil {
		httputil.InternalError(c, "failed to list split-book candidates", err)
		return
	}
	if cands == nil {
		cands = []dedupengine.SplitBookCandidate{}
	}
	httputil.RespondWithOK(c, gin.H{
		"candidates": cands,
		"total":      len(cands),
	})
}

// mergeSplitBookCandidate handles POST /api/v1/dedup/split-book-candidates/:id/merge.
//
// Body (optional):
//
//	{ "keep_id": "<bookID>" }
//
// If keep_id is omitted, the suggested keep-ID from the candidate
// (BookIDs[0], earliest ULID) is used. On success the candidate row is
// deleted so subsequent list calls don't surface it again.
func (s *Server) mergeSplitBookCandidate(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		httputil.RespondWithBadRequest(c, "candidate id required")
		return
	}
	store, ok := s.splitBookStore()
	if !ok {
		httputil.RespondWithServiceUnavailable(c, "split-book store not available")
		return
	}
	cand, err := store.Get(id)
	if err != nil {
		httputil.InternalError(c, "failed to load split-book candidate", err)
		return
	}
	if cand == nil {
		httputil.RespondWithNotFound(c, "split_book_candidate", id)
		return
	}
	if len(cand.BookIDs) < 2 {
		httputil.RespondWithBadRequest(c, "candidate has fewer than 2 books")
		return
	}

	var body struct {
		KeepID string `json:"keep_id"`
	}
	_ = c.ShouldBindJSON(&body) // body is optional

	keepID := body.KeepID
	if keepID == "" {
		keepID = cand.BookIDs[0]
	}

	// Validate keepID is in the candidate's BookIDs.
	srcIDs := make([]string, 0, len(cand.BookIDs)-1)
	keepFound := false
	for _, bid := range cand.BookIDs {
		if bid == keepID {
			keepFound = true
			continue
		}
		srcIDs = append(srcIDs, bid)
	}
	if !keepFound {
		httputil.RespondWithBadRequest(c, "keep_id not in candidate book_ids")
		return
	}

	result, err := dedupengine.MergeSplitBookCluster(s.Store(), keepID, srcIDs, cand.SuggestedTitle)
	if err != nil {
		httputil.InternalError(c, "split-book merge failed", err)
		return
	}

	// Delete candidate so it's not surfaced again. Best-effort: a
	// stale row left behind only causes a re-attempt that no-ops on
	// already-soft-deleted srcs.
	if delErr := store.Delete(id); delErr != nil {
		c.Error(delErr) //nolint:errcheck
	}

	httputil.RespondWithOK(c, result)
}

// splitBookStore is a small helper that returns the candidate store
// backed by the shared embedding-store Pebble handle. Returns
// (nil, false) when the embedding store isn't wired up (e.g. tests).
func (s *Server) splitBookStore() (*dedupengine.SplitBookStore, bool) {
	if s.embeddingStore == nil {
		return nil, false
	}
	db := s.embeddingStore.PebbleDB()
	if db == nil {
		return nil, false
	}
	return dedupengine.NewSplitBookStore(db), true
}
