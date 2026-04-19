// file: internal/server/file_io_pool.go
// version: 2.2.0
// guid: c4d5e6f7-a8b9-0c1d-2e3f-4a5b6c7d8e9f
//
// Bounded worker pool for file I/O operations (cover embed, tag write,
// rename). Tracks pending jobs in PebbleDB so they survive restarts.
//
// Tracking key schema: pending_file_op:{bookID}:{opType}. Multiple op
// types for the same book coexist without clobbering. Recovery looks
// each opType up in recoveryDispatch.

package server

import (
	"encoding/json"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

const pendingFileOpPrefix = "pending_file_op:"

// FileIOJob tracks a pending file I/O operation persistently.
type FileIOJob struct {
	BookID    string    `json:"book_id"`
	OpType    string    `json:"op_type"`
	CreatedAt time.Time `json:"created_at"`
}

// FileIOPool manages a bounded pool of workers for slow file operations.
// Jobs are tracked in PebbleDB so interrupted ones can be recovered on restart.
type FileIOPool struct {
	ch       chan fileIOJobEntry
	wg       sync.WaitGroup
	stopped  int32
	pending  sync.Map      // "{bookID}:{opType}" -> FileIOJob, for in-memory tracking
	overflow chan struct{} // semaphore to limit overflow goroutines
}

type fileIOJobEntry struct {
	bookID string
	opType string
	fn     func()
}

var (
	GlobalFileIOPool   *FileIOPool
	globalFileIOPoolMu sync.Mutex

	// globalServer holds a Server reference used by the default recovery handler.
	globalServer *Server

	recoveryDispatch   = map[string]FileOpRecoveryFunc{}
	recoveryDispatchMu sync.RWMutex
)

// FileOpRecoveryFunc re-runs a specific file-op type for one book.
type FileOpRecoveryFunc func(bookID string)

// RegisterFileOpRecovery registers a recovery handler for a given op type.
// Overwrites any previous registration for the same type.
func RegisterFileOpRecovery(opType string, fn FileOpRecoveryFunc) {
	recoveryDispatchMu.Lock()
	recoveryDispatch[opType] = fn
	recoveryDispatchMu.Unlock()
}

func lookupFileOpRecovery(opType string) (FileOpRecoveryFunc, bool) {
	recoveryDispatchMu.RLock()
	fn, ok := recoveryDispatch[opType]
	recoveryDispatchMu.RUnlock()
	return fn, ok
}

// GetGlobalFileIOPool returns the pool safely.
func GetGlobalFileIOPool() *FileIOPool {
	globalFileIOPoolMu.Lock()
	p := GlobalFileIOPool
	globalFileIOPoolMu.Unlock()
	return p
}

// SetGlobalFileIOPool sets the pool safely.
func SetGlobalFileIOPool(p *FileIOPool) {
	globalFileIOPoolMu.Lock()
	GlobalFileIOPool = p
	globalFileIOPoolMu.Unlock()
}

// NewFileIOPool creates a pool with the given number of workers.
func NewFileIOPool(workers int) *FileIOPool {
	p := &FileIOPool{
		ch:       make(chan fileIOJobEntry, 500),
		overflow: make(chan struct{}, workers),
	}
	for i := 0; i < workers; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
	log.Printf("[INFO] file I/O pool started with %d workers, buffer 500", workers)
	return p
}

func (p *FileIOPool) worker(id int) {
	defer p.wg.Done()
	for job := range p.ch {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[ERROR] file I/O worker %d panicked on book %s (op=%s): %v", id, job.bookID, job.opType, r)
				}
			}()
			job.fn()
			p.pending.Delete(pendingKey(job.bookID, job.opType))
			removePendingFileOp(job.bookID, job.opType)
		}()
	}
}

// Submit queues a file I/O job with the default "apply_metadata" op type.
func (p *FileIOPool) Submit(bookID string, fn func()) {
	p.SubmitTyped(bookID, "apply_metadata", fn)
}

// SubmitTyped queues a file I/O job with a specific operation type.
func (p *FileIOPool) SubmitTyped(bookID, opType string, fn func()) {
	if atomic.LoadInt32(&p.stopped) == 1 {
		log.Printf("[WARN] file I/O pool stopped, dropping job for book %s (op=%s)", bookID, opType)
		return
	}
	job := FileIOJob{BookID: bookID, OpType: opType, CreatedAt: time.Now()}
	p.pending.Store(pendingKey(bookID, opType), job)
	storePendingFileOp(job)

	select {
	case p.ch <- fileIOJobEntry{bookID: bookID, opType: opType, fn: fn}:
	default:
		p.overflow <- struct{}{}
		log.Printf("[WARN] file I/O pool buffer full, running overflow for book %s (op=%s)", bookID, opType)
		go func() {
			defer func() { <-p.overflow }()
			fn()
			p.pending.Delete(pendingKey(bookID, opType))
			removePendingFileOp(bookID, opType)
		}()
	}
}

// Pending returns the number of queued jobs.
func (p *FileIOPool) Pending() int {
	return len(p.ch)
}

// PendingJobs returns all in-flight / queued jobs for observability.
func (p *FileIOPool) PendingJobs() []FileIOJob {
	var jobs []FileIOJob
	p.pending.Range(func(_, v any) bool {
		if job, ok := v.(FileIOJob); ok {
			jobs = append(jobs, job)
		}
		return true
	})
	return jobs
}

// PendingBookIDs returns all book IDs with at least one pending file op.
// Deduped across op types.
func (p *FileIOPool) PendingBookIDs() []string {
	seen := map[string]struct{}{}
	p.pending.Range(func(_, v any) bool {
		if job, ok := v.(FileIOJob); ok {
			seen[job.BookID] = struct{}{}
		}
		return true
	})
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	return ids
}

// Stop drains the queue and waits for in-flight jobs to finish,
// with a 30-second timeout to prevent blocking shutdown indefinitely.
// Safe to call multiple times.
func (p *FileIOPool) Stop() {
	if !atomic.CompareAndSwapInt32(&p.stopped, 0, 1) {
		return
	}
	close(p.ch)

	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("[INFO] file I/O pool stopped, all jobs complete")
	case <-time.After(30 * time.Second):
		log.Printf("[WARN] file I/O pool shutdown timed out after 30s, %d jobs may be incomplete", p.Pending())
	}
}

// InitFileIOPool creates the global pool and registers the default recovery handler.
func InitFileIOPool() {
	GlobalFileIOPool = NewFileIOPool(4)
	RegisterFileOpRecovery("apply_metadata", func(bookID string) {
		srv := globalServer
		if srv == nil || srv.metadataFetchService == nil {
			log.Printf("[WARN] no server instance for apply_metadata recovery of book %s", bookID)
			return
		}
		srv.metadataFetchService.ApplyMetadataFileIO(bookID)
		if _, err := srv.metadataFetchService.WriteBackMetadataForBook(bookID); err != nil {
			log.Printf("[WARN] recovery write-back for %s: %v", bookID, err)
		}
		if srv.writeBackBatcher != nil {
			srv.writeBackBatcher.Enqueue(bookID)
		}
	})
}

// RecoverInterruptedFileOps re-queues any interrupted file I/O jobs.
// Called from the server startup sequence after services are ready.
func RecoverInterruptedFileOps(pool *FileIOPool) {
	recoverInterruptedFileOps(pool)
}

// --- Persistent tracking via PebbleDB ---

func pendingKey(bookID, opType string) string {
	return bookID + ":" + opType
}

func pebbleKey(bookID, opType string) string {
	return pendingFileOpPrefix + bookID + ":" + opType
}

// parsePebbleKey splits a stored key back into (bookID, opType).
// Accepts legacy keys without opType ("pending_file_op:{bookID}"), treating them as apply_metadata.
func parsePebbleKey(key string) (bookID, opType string, ok bool) {
	rest := strings.TrimPrefix(key, pendingFileOpPrefix)
	if rest == key {
		return "", "", false
	}
	// Last ":" separates bookID from opType. Book IDs shouldn't contain ":"
	// but be defensive: split on the last colon only.
	idx := strings.LastIndex(rest, ":")
	if idx < 0 {
		return rest, "apply_metadata", true
	}
	return rest[:idx], rest[idx+1:], true
}

func storePendingFileOp(job FileIOJob) {
	store := database.GetGlobalStore()
	if store == nil {
		return
	}
	data, _ := json.Marshal(job)
	_ = store.SetRaw(pebbleKey(job.BookID, job.OpType), data)
}

func removePendingFileOp(bookID, opType string) {
	store := database.GetGlobalStore()
	if store == nil {
		return
	}
	_ = store.DeleteRaw(pebbleKey(bookID, opType))
}

// recoverInterruptedFileOps re-queues any file I/O jobs that were in-flight
// when the server last shut down (or crashed).
func recoverInterruptedFileOps(pool *FileIOPool) {
	store := database.GetGlobalStore()
	if store == nil {
		return
	}

	keys, err := store.ScanPrefix(pendingFileOpPrefix)
	if err != nil || len(keys) == 0 {
		return
	}

	log.Printf("[INFO] recovering %d interrupted file I/O operations", len(keys))

	for _, kv := range keys {
		var job FileIOJob
		if err := json.Unmarshal(kv.Value, &job); err != nil {
			_ = store.DeleteRaw(kv.Key)
			continue
		}

		// Backfill fields for legacy entries that predate the opType split.
		if job.BookID == "" || job.OpType == "" {
			if bid, op, ok := parsePebbleKey(kv.Key); ok {
				if job.BookID == "" {
					job.BookID = bid
				}
				if job.OpType == "" {
					job.OpType = op
				}
			}
		}
		if job.OpType == "" {
			job.OpType = "apply_metadata"
		}
		if job.BookID == "" {
			_ = store.DeleteRaw(kv.Key)
			continue
		}

		fn, ok := lookupFileOpRecovery(job.OpType)
		if !ok {
			log.Printf("[WARN] no recovery handler for op type %q (book %s), removing stale key", job.OpType, job.BookID)
			_ = store.DeleteRaw(kv.Key)
			continue
		}

		bookID := job.BookID
		opType := job.OpType
		log.Printf("[INFO] re-queuing file I/O for book %s (type=%s, started=%s)", bookID, opType, job.CreatedAt.Format(time.RFC3339))
		if pool != nil {
			pool.SubmitTyped(bookID, opType, func() { fn(bookID) })
		}
	}
}
