// file: internal/server/activity_writer.go
// version: 1.0.0
// guid: c3d4e5f6-a7b8-4c9d-0e1f-2a3b4c5d6e7f

package server

import (
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// activityWriter is an io.Writer that tees log output to stdout AND sends
// parsed entries through a buffered channel to an ActivityStore.
type activityWriter struct {
	stdout   io.Writer
	ch       chan database.ActivityEntry
	store    *database.ActivityStore
	done     chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
	mu       sync.Mutex
	partial  string // incomplete line buffer
	closed   atomic.Bool
}

// newActivityWriter creates a new activityWriter backed by store.
// chanSize controls the depth of the internal entry buffer.
func newActivityWriter(store *database.ActivityStore, chanSize int) *activityWriter {
	return &activityWriter{
		stdout: os.Stdout,
		ch:     make(chan database.ActivityEntry, chanSize),
		store:  store,
		done:   make(chan struct{}),
	}
}

// Start launches the background drain goroutine. Call once before writing.
func (w *activityWriter) Start() {
	w.wg.Add(1)
	go w.drain()
}

// Write implements io.Writer. Always writes to stdout first, then parses
// complete lines and sends ActivityEntry values to the background drain.
func (w *activityWriter) Write(p []byte) (n int, err error) {
	n, err = w.stdout.Write(p)

	w.mu.Lock()
	defer w.mu.Unlock()

	data := w.partial + string(p)
	w.partial = ""

	for {
		idx := strings.IndexByte(data, '\n')
		if idx < 0 {
			w.partial = data
			break
		}
		line := strings.TrimRight(data[:idx], "\r")
		data = data[idx+1:]
		if line != "" {
			w.sendEntry(line)
		}
	}
	return n, nil
}

// sendEntry parses a single log line and enqueues an ActivityEntry.
// Debug entries are silently dropped when the channel is full.
// Non-debug entries emit a warning to stdout when dropped.
func (w *activityWriter) sendEntry(line string) {
	if w.closed.Load() {
		return
	}
	level, source, message := parseLogLine(line)
	entry := database.ActivityEntry{
		Tier:    "debug",
		Type:    "system",
		Level:   level,
		Source:  source,
		Summary: message,
	}
	select {
	case w.ch <- entry:
	default:
		if level != "debug" {
			w.stdout.Write([]byte("[WARN] activity channel full, dropped: " + message + "\n")) //nolint:errcheck
		}
	}
}

// drain reads from the channel and persists entries in batches of up to 100,
// flushing at least every 500 ms. It stops when the done signal is received.
func (w *activityWriter) drain() {
	defer w.wg.Done()
	batch := make([]database.ActivityEntry, 0, 100)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case entry := <-w.ch:
			batch = append(batch, entry)
			if len(batch) >= 100 {
				w.writeBatch(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				w.writeBatch(batch)
				batch = batch[:0]
			}
		case <-w.done:
			// Drain remaining entries before exiting.
		drainLoop:
			for {
				select {
				case entry := <-w.ch:
					batch = append(batch, entry)
				default:
					break drainLoop
				}
			}
			if len(batch) > 0 {
				w.writeBatch(batch)
			}
			return
		}
	}
}

// writeBatch persists a slice of entries to the store, ignoring individual errors.
func (w *activityWriter) writeBatch(entries []database.ActivityEntry) {
	for _, e := range entries {
		w.store.Record(e) //nolint:errcheck
	}
}

// Flush synchronously drains any entries currently in the channel without
// stopping the background goroutine.
func (w *activityWriter) Flush() {
	for {
		select {
		case e := <-w.ch:
			w.store.Record(e) //nolint:errcheck
		default:
			return
		}
	}
}

// Stop marks the writer as closed, signals the drain goroutine to finish,
// and waits for it to flush all remaining entries. Safe to call multiple times.
func (w *activityWriter) Stop() {
	w.closed.Store(true)
	w.stopOnce.Do(func() { close(w.done) })
	w.wg.Wait()
}

// ── log line parser ───────────────────────────────────────────────────────────

// parseLogLine extracts (level, source, message) from a single log line.
//
// Recognised formats:
//   - GIN: "[GIN] YYYY/MM/DD - HH:MM:SS | STATUS | ..."
//   - Go standard log: "YYYY/MM/DD HH:MM:SS file.go:NNN: [level] source: message"
//   - Bare text: returned as-is with level=info, source=server.
func parseLogLine(line string) (level, source, message string) {
	// GIN logs: [GIN] YYYY/MM/DD - HH:MM:SS | STATUS | ...
	if strings.HasPrefix(line, "[GIN]") {
		rest := line[5:]
		if idx := strings.Index(rest, "| "); idx >= 0 {
			message = strings.TrimSpace(rest[idx+2:])
		} else {
			message = strings.TrimSpace(rest)
		}
		return "info", "gin", message
	}

	work := line

	// Strip date/time prefix: "YYYY/MM/DD HH:MM:SS" (19 chars + space)
	if len(work) > 20 && work[4] == '/' && work[7] == '/' && work[10] == ' ' {
		// Find "file:line: " part after timestamp and skip past it.
		if idx := strings.Index(work[20:], ": "); idx >= 0 {
			work = work[20+idx+2:]
		} else {
			work = work[20:]
		}
	}

	work = strings.TrimSpace(work)

	// Check for [level] prefix
	if len(work) > 2 && work[0] == '[' {
		if end := strings.Index(work, "] "); end > 0 {
			level = strings.ToLower(work[1:end])
			work = work[end+2:]
			// Check for "source: message" — source name is short (< 30 chars)
			if idx := strings.Index(work, ": "); idx > 0 && idx < 30 {
				source = work[:idx]
				message = work[idx+2:]
				return level, source, message
			}
			return level, "server", work
		}
	}

	return "info", "server", work
}
