// file: internal/server/indexed_store.go
// version: 1.0.0
// guid: 5d2e4f3a-7b5a-4a70-b8c5-3d7e0f1b9a79
//
// indexedStore decorates a database.Store so that every successful
// book mutation (create / update / delete) schedules an async
// Bleve index update. This keeps the search index in sync without
// threading explicit index calls through every handler and service
// that touches books.
//
// Indexing is async via a bounded channel. If the channel fills up
// (worker stuck, Bleve slow) new requests are dropped silently —
// the library search rebuilds on startup (see buildSearchIndexIfEmpty)
// so stale entries eventually get repaired.

package server

import (
	"log"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// indexedStore wraps an inner Store and fires index-update events on
// book mutations. The embedded interface forwards every other method
// to the underlying store transparently.
type indexedStore struct {
	database.Store
	server *Server
}

// CreateBook writes to the inner store and schedules an index
// refresh for the newly-assigned book ID on success.
func (s *indexedStore) CreateBook(b *database.Book) (*database.Book, error) {
	created, err := s.Store.CreateBook(b)
	if err == nil && created != nil {
		s.server.enqueueIndex(created.ID, false)
	}
	return created, err
}

// UpdateBook schedules a re-index on success. The update may be a
// narrow field change but we reindex the full document to keep
// things simple — BookToDoc is cheap relative to Bleve's cost.
func (s *indexedStore) UpdateBook(id string, b *database.Book) (*database.Book, error) {
	updated, err := s.Store.UpdateBook(id, b)
	if err == nil {
		s.server.enqueueIndex(id, false)
	}
	return updated, err
}

// DeleteBook removes the row and schedules a Bleve delete on success.
func (s *indexedStore) DeleteBook(id string) error {
	if err := s.Store.DeleteBook(id); err != nil {
		return err
	}
	s.server.enqueueIndex(id, true)
	return nil
}

// indexRequest is the payload carried on the index worker channel.
// Delete=true removes from Bleve; otherwise a reindex is performed
// by reading the current book state from the store.
type indexRequest struct {
	bookID string
	delete bool
}

// enqueueIndex submits an index event. Full queue drops the event
// silently — a startup reindex will heal any gaps. Safe to call
// concurrently with Shutdown: the mutex + closed flag prevents
// sending on a closed channel during teardown.
func (s *Server) enqueueIndex(bookID string, del bool) {
	if bookID == "" {
		return
	}
	s.indexQueueMu.RLock()
	defer s.indexQueueMu.RUnlock()
	if s.indexQueueClosed || s.indexQueue == nil {
		return
	}
	select {
	case s.indexQueue <- indexRequest{bookID: bookID, delete: del}:
	default:
		log.Printf("[WARN] search index queue full, dropped %s (delete=%v)", bookID, del)
	}
}

// closeIndexQueue takes the write lock, closes the channel, and
// flips the closed flag so subsequent enqueueIndex calls no-op.
// Called exactly once from Shutdown.
func (s *Server) closeIndexQueue() {
	s.indexQueueMu.Lock()
	defer s.indexQueueMu.Unlock()
	if s.indexQueueClosed || s.indexQueue == nil {
		return
	}
	s.indexQueueClosed = true
	close(s.indexQueue)
}

// runIndexWorker drains the index queue. Designed as a single
// long-lived goroutine so Bleve sees serialized writes and we don't
// need to protect BookToDoc-style reads against concurrent DB state.
// Exits when the queue is closed by Shutdown.
func (s *Server) runIndexWorker() {
	if s.indexQueue == nil {
		return
	}
	for req := range s.indexQueue {
		if req.delete {
			if err := s.DeleteIndexedBook(req.bookID); err != nil {
				log.Printf("[WARN] delete index %s: %v", req.bookID, err)
			}
			continue
		}
		if err := s.IndexBookByID(req.bookID); err != nil {
			log.Printf("[WARN] index %s: %v", req.bookID, err)
		}
	}
}
