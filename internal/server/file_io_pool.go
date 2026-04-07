// file: internal/server/file_io_pool.go
// version: 1.0.0
// guid: c4d5e6f7-a8b9-0c1d-2e3f-4a5b6c7d8e9f
//
// Bounded worker pool for file I/O operations (cover embed, tag write,
// rename). Prevents unbounded goroutine spawning when many metadata
// applies happen in rapid succession.

package server

import (
	"log"
	"sync"
	"sync/atomic"
)

// FileIOPool manages a bounded pool of workers for slow file operations.
type FileIOPool struct {
	ch      chan fileIOJob
	wg      sync.WaitGroup
	stopped int32
}

type fileIOJob struct {
	bookID string
	fn     func()
}

// GlobalFileIOPool is the singleton pool.
var GlobalFileIOPool *FileIOPool

// NewFileIOPool creates a pool with the given number of workers.
func NewFileIOPool(workers int) *FileIOPool {
	p := &FileIOPool{
		ch: make(chan fileIOJob, 200), // buffer up to 200 pending jobs
	}
	for i := 0; i < workers; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
	log.Printf("[INFO] file I/O pool started with %d workers", workers)
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
		}()
	}
}

// Submit queues a file I/O job. Non-blocking if buffer isn't full.
func (p *FileIOPool) Submit(bookID string, fn func()) {
	if atomic.LoadInt32(&p.stopped) == 1 {
		log.Printf("[WARN] file I/O pool stopped, dropping job for book %s", bookID)
		return
	}
	select {
	case p.ch <- fileIOJob{bookID: bookID, fn: fn}:
	default:
		// Buffer full — run inline as fallback
		log.Printf("[WARN] file I/O pool buffer full, running inline for book %s", bookID)
		go fn()
	}
}

// Pending returns the number of queued jobs.
func (p *FileIOPool) Pending() int {
	return len(p.ch)
}

// Stop drains the queue and waits for in-flight jobs to finish.
// Called during graceful shutdown.
func (p *FileIOPool) Stop() {
	atomic.StoreInt32(&p.stopped, 1)
	close(p.ch)
	p.wg.Wait()
	log.Printf("[INFO] file I/O pool stopped, all jobs complete")
}

// InitFileIOPool creates the global pool.
func InitFileIOPool() {
	GlobalFileIOPool = NewFileIOPool(4)
}
