// file: internal/watcher/watcher.go
// version: 1.0.0
// guid: b2c3d4e5-f6a7-8901-bcde-f23456789012

package watcher

import (
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// audioExtensions are the file extensions we watch for
var audioExtensions = map[string]bool{
	".m4b":  true,
	".mp3":  true,
	".flac": true,
	".m4a":  true,
	".aac":  true,
	".ogg":  true,
	".wma":  true,
}

// ScanFunc is called when a scan should be triggered for a path
type ScanFunc func(path string)

// Watcher monitors directories for new audio files and triggers scans
type Watcher struct {
	fsWatcher      *fsnotify.Watcher
	paths          []string
	debounceDelay  time.Duration
	onScan         ScanFunc
	stop           chan struct{}
	wg             sync.WaitGroup
	mu             sync.Mutex
	pendingPaths   map[string]time.Time
	debounceTimers map[string]*time.Timer
}

// New creates a new file watcher
func New(paths []string, debounceSec int, onScan ScanFunc) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	delay := time.Duration(debounceSec) * time.Second
	if delay <= 0 {
		delay = 30 * time.Second
	}

	return &Watcher{
		fsWatcher:      fsw,
		paths:          paths,
		debounceDelay:  delay,
		onScan:         onScan,
		stop:           make(chan struct{}),
		pendingPaths:   make(map[string]time.Time),
		debounceTimers: make(map[string]*time.Timer),
	}, nil
}

// Start begins watching the configured paths
func (w *Watcher) Start() error {
	for _, p := range w.paths {
		if err := w.fsWatcher.Add(p); err != nil {
			log.Printf("[WARN] watcher: failed to watch %s: %v", p, err)
		} else {
			log.Printf("[INFO] watcher: watching %s for new audiobooks", p)
		}
	}

	w.wg.Add(1)
	go w.eventLoop()
	return nil
}

// Stop gracefully shuts down the watcher
func (w *Watcher) Stop() {
	close(w.stop)
	w.fsWatcher.Close()
	w.wg.Wait()

	// Cancel any pending timers
	w.mu.Lock()
	for _, timer := range w.debounceTimers {
		timer.Stop()
	}
	w.mu.Unlock()
}

func (w *Watcher) eventLoop() {
	defer w.wg.Done()

	for {
		select {
		case <-w.stop:
			return
		case event, ok := <-w.fsWatcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Create|fsnotify.Write) == 0 {
				continue
			}
			if !isAudioFile(event.Name) {
				continue
			}
			w.scheduleSccan(filepath.Dir(event.Name))

		case err, ok := <-w.fsWatcher.Errors:
			if !ok {
				return
			}
			log.Printf("[ERROR] watcher: %v", err)
		}
	}
}

func (w *Watcher) scheduleSccan(dir string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Reset or create debounce timer for this directory
	if timer, exists := w.debounceTimers[dir]; exists {
		timer.Reset(w.debounceDelay)
	} else {
		w.debounceTimers[dir] = time.AfterFunc(w.debounceDelay, func() {
			log.Printf("[INFO] watcher: triggering scan for %s", dir)
			w.onScan(dir)

			w.mu.Lock()
			delete(w.debounceTimers, dir)
			w.mu.Unlock()
		})
	}
}

func isAudioFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return audioExtensions[ext]
}
