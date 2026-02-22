// file: internal/itunes/library_watcher.go
// version: 1.0.0
// guid: e9f0a1b2-c3d4-5e6f-7a8b-9c0d1e2f3a4b

package itunes

import (
	"log"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// LibraryWatcher monitors an iTunes Library.xml file for external changes.
type LibraryWatcher struct {
	path      string
	watcher   *fsnotify.Watcher
	mu        sync.RWMutex
	changed   bool
	changedAt time.Time
	stop      chan struct{}
}

// NewLibraryWatcher creates a watcher for the given library file path.
func NewLibraryWatcher(path string) (*LibraryWatcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	if err := fsw.Add(path); err != nil {
		fsw.Close()
		return nil, err
	}

	w := &LibraryWatcher{
		path:    path,
		watcher: fsw,
		stop:    make(chan struct{}),
	}

	go w.loop()
	return w, nil
}

func (w *LibraryWatcher) loop() {
	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Rename) {
				w.mu.Lock()
				w.changed = true
				w.changedAt = time.Now()
				w.mu.Unlock()
				log.Printf("iTunes library file changed: %s (op: %s)", w.path, event.Op)
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("iTunes library watcher error: %v", err)
		case <-w.stop:
			return
		}
	}
}

// HasChanged returns true if the file has been modified since last ClearChanged.
func (w *LibraryWatcher) HasChanged() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.changed
}

// ChangedAt returns when the last change was detected.
func (w *LibraryWatcher) ChangedAt() time.Time {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.changedAt
}

// ClearChanged resets the changed flag (call after import/write-back).
func (w *LibraryWatcher) ClearChanged() {
	w.mu.Lock()
	w.changed = false
	w.changedAt = time.Time{}
	w.mu.Unlock()
}

// Close stops watching.
func (w *LibraryWatcher) Close() error {
	close(w.stop)
	return w.watcher.Close()
}
