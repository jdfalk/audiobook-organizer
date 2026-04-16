// file: internal/server/file_ops_handlers.go
// version: 1.1.0
// guid: 5a2e0c6b-1d4f-4a9e-9c3b-6f1a2d7e8b01
//
// HTTP handlers for in-flight file I/O operations. Exposes the
// FileIOPool's pending jobs so the UI can display a "N books writing
// tags..." indicator and list per-book progress.

package server

import (
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
)

type pendingFileOp struct {
	BookID    string    `json:"book_id"`
	OpType    string    `json:"op_type"`
	StartedAt time.Time `json:"started_at"`
	BookTitle string    `json:"book_title,omitempty"`
}

// handleListPendingFileOps returns currently-queued + in-flight file I/O jobs.
// Used by the frontend toast + Operations page + Activity Log page.
func (s *Server) handleListPendingFileOps(c *gin.Context) {
	pool := GetGlobalFileIOPool()
	if pool == nil {
		c.JSON(http.StatusOK, gin.H{"operations": []pendingFileOp{}, "count": 0})
		return
	}

	jobs := pool.PendingJobs()
	out := make([]pendingFileOp, 0, len(jobs))
	store := s.Store()
	for _, j := range jobs {
		op := pendingFileOp{
			BookID:    j.BookID,
			OpType:    j.OpType,
			StartedAt: j.CreatedAt,
		}
		if store != nil {
			if b, err := store.GetBookByID(j.BookID); err == nil && b != nil {
				op.BookTitle = b.Title
			}
		}
		out = append(out, op)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].StartedAt.Before(out[j].StartedAt)
	})

	c.JSON(http.StatusOK, gin.H{"operations": out, "count": len(out)})
}
