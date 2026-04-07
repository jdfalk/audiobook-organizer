// file: internal/server/file_io_pool.go
// version: 2.1.0
// guid: c4d5e6f7-a8b9-0c1d-2e3f-4a5b6c7d8e9f
//
// Bounded worker pool for file I/O operations (cover embed, tag write,
// rename). Tracks pending jobs in PebbleDB so they survive restarts.

package server

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// FileIOJob tracks a pending file I/O operation persistently.
type FileIOJob struct {
	BookID    string    `json:"book_id"`
	OpType    string    `json:"op_type"` // "apply_metadata", "write_back", etc.
	CreatedAt time.Time `json:"created_at"`
}

// FileIOPool manages a bounded pool of workers for slow file operations.
// Jobs are tracked in PebbleDB so interrupted ones can be recovered on restart.
type FileIOPool struct {
	ch       chan fileIOJobEntry
	wg       sync.WaitGroup
	stopped  int32
	pending  sync.Map // bookID -> true, for in-memory tracking
	overflow chan struct{} // semaphore to limit overflow goroutines
}

type fileIOJobEntry struct {
	bookID string
	opType string
	fn     func()
}

// GlobalFileIOPool is the singleton pool.
var GlobalFileIOPool *FileIOPool

// globalServer holds a reference to the Server for recovery of interrupted file ops.
var globalServer *Server

// NewFileIOPool creates a pool with the given number of workers.
func NewFileIOPool(workers int) *FileIOPool {
	p := &FileIOPool{
		ch:       make(chan fileIOJobEntry, 500),
		overflow: make(chan struct{}, workers), // cap overflow goroutines at worker count
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
					log.Printf("[ERROR] file I/O worker %d panicked on book %s: %v", id, job.bookID, r)
				}
			}()
			job.fn()
			// Mark completed — remove from persistent store
			p.pending.Delete(job.bookID)
			removePendingFileOp(job.bookID)
		}()
	}
}

// Submit queues a file I/O job with persistent tracking.
func (p *FileIOPool) Submit(bookID string, fn func()) {
	p.SubmitTyped(bookID, "apply_metadata", fn)
}

// SubmitTyped queues a file I/O job with a specific operation type.
func (p *FileIOPool) SubmitTyped(bookID, opType string, fn func()) {
	if atomic.LoadInt32(&p.stopped) == 1 {
		log.Printf("[WARN] file I/O pool stopped, dropping job for book %s", bookID)
		return
	}
	// Track persistently so we can recover on restart
	p.pending.Store(bookID, true)
	storePendingFileOp(bookID, opType)

	select {
	case p.ch <- fileIOJobEntry{bookID: bookID, opType: opType, fn: fn}:
	default:
		// Buffer full — use semaphore to limit overflow concurrency
		p.overflow <- struct{}{} // blocks if too many overflow goroutines
		log.Printf("[WARN] file I/O pool buffer full, running overflow for book %s", bookID)
		go func() {
			defer func() { <-p.overflow }()
			fn()
			p.pending.Delete(bookID)
			removePendingFileOp(bookID)
		}()
	}
}

// Pending returns the number of queued + in-flight jobs.
func (p *FileIOPool) Pending() int {
	return len(p.ch)
}

// PendingBookIDs returns all book IDs with pending file operations.
func (p *FileIOPool) PendingBookIDs() []string {
	var ids []string
	p.pending.Range(func(key, _ any) bool {
		ids = append(ids, key.(string))
		return true
	})
	return ids
}

// Stop drains the queue and waits for in-flight jobs to finish,
// with a 30-second timeout to prevent blocking shutdown indefinitely.
func (p *FileIOPool) Stop() {
	atomic.StoreInt32(&p.stopped, 1)
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

// InitFileIOPool creates the global pool.
func InitFileIOPool() {
	GlobalFileIOPool = NewFileIOPool(4)
}

// RecoverInterruptedFileOps re-queues any interrupted file I/O jobs.
// Called explicitly from the server startup sequence, after all services are ready.
func RecoverInterruptedFileOps() {
	recoverInterruptedFileOps()
}

// --- Persistent tracking via PebbleDB ---

func storePendingFileOp(bookID, opType string) {
	store := database.GlobalStore
	if store == nil {
		return
	}
	job := FileIOJob{BookID: bookID, OpType: opType, CreatedAt: time.Now()}
	data, _ := json.Marshal(job)
	key := fmt.Sprintf("pending_file_op:%s", bookID)
	_ = store.SetRaw(key, data)
}

func removePendingFileOp(bookID string) {
	store := database.GlobalStore
	if store == nil {
		return
	}
	key := fmt.Sprintf("pending_file_op:%s", bookID)
	_ = store.DeleteRaw(key)
}

// recoverInterruptedFileOps re-queues any file I/O jobs that were in-flight
// when the server last shut down (or crashed).
func recoverInterruptedFileOps() {
	store := database.GlobalStore
	if store == nil {
		return
	}

	keys, err := store.ScanPrefix("pending_file_op:")
	if err != nil || len(keys) == 0 {
		// No interrupted ops or store doesn't support ScanPrefix (e.g. in tests)
		return
	}

	log.Printf("[INFO] recovering %d interrupted file I/O operations", len(keys))

	for _, kv := range keys {
		var job FileIOJob
		if err := json.Unmarshal(kv.Value, &job); err != nil {
			_ = store.DeleteRaw(kv.Key)
			continue
		}

		bookID := job.BookID
		log.Printf("[INFO] re-queuing file I/O for book %s (type=%s, started=%s)", bookID, job.OpType, job.CreatedAt.Format(time.RFC3339))

		GlobalFileIOPool.SubmitTyped(bookID, job.OpType, func() {
			// Re-run the full apply pipeline
			srv := globalServer
			if srv == nil {
				log.Printf("[WARN] no server instance for recovery of book %s", bookID)
				return
			}
			srv.metadataFetchService.ApplyMetadataFileIO(bookID)
			if _, wbErr := srv.metadataFetchService.WriteBackMetadataForBook(bookID); wbErr != nil {
				log.Printf("[WARN] recovery write-back for %s: %v", bookID, wbErr)
			}
			if GlobalWriteBackBatcher != nil {
				GlobalWriteBackBatcher.Enqueue(bookID)
			}
		})
	}
}
