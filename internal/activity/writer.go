// file: internal/activity/writer.go
// version: 1.6.0
// guid: c3d4e5f6-a7b8-4c9d-0e1f-2a3b4c5d6e7f

package activity

import (
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/falkcorp/audiobook-organizer/internal/database"
)

// Writer is an io.Writer that tees log output to stdout AND sends
// parsed entries through a buffered channel to an ActivityStore.
type Writer struct {
	stdout      io.Writer
	ch          chan database.ActivityEntry
	store       database.ActivityStorer
	batcher     *ActivityBatcher
	done        chan struct{}
	stopOnce    sync.Once
	wg          sync.WaitGroup
	mu          sync.Mutex
	partial     string // incomplete line buffer
	closed      atomic.Bool
	skipSources map[string]struct{}
}

// NewWriter creates a new Writer backed by store.
// chanSize controls the depth of the internal entry buffer.
// By default the "gin" source is skipped — HTTP request logs are not
// useful as persistent activity entries and would flood the database.
func NewWriter(store database.ActivityStorer, chanSize int) *Writer {
	w := &Writer{
		stdout:      os.Stdout,
		ch:          make(chan database.ActivityEntry, chanSize),
		store:       store,
		done:        make(chan struct{}),
		skipSources: map[string]struct{}{"gin": {}},
	}
	w.batcher = NewActivityBatcher(w.ch)
	return w
}

// SetSkipSources replaces the set of log sources that are dropped before
// being written to the activity store. Entries are still printed to stdout.
// Call before Start(). Pass no arguments to disable all skipping.
func (w *Writer) SetSkipSources(sources ...string) {
	m := make(map[string]struct{}, len(sources))
	for _, s := range sources {
		m[s] = struct{}{}
	}
	w.skipSources = m
}

// Start launches the background drain goroutine. Call once before writing.
// Implements the Starter interface for serviceregistry.
func (w *Writer) Start(ctx context.Context) error {
	w.wg.Add(1)
	go w.drain()
	return nil
}

// Write implements io.Writer. Always writes to stdout first, then parses
// complete lines and sends ActivityEntry values to the background drain.
func (w *Writer) Write(p []byte) (n int, err error) {
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

// isBatchable returns true if e should be routed through the ActivityBatcher
// rather than written directly to the channel. Entries are batchable only when
// they come from a structured LogBatch call (operationID non-empty) AND their
// type is registered as a high-volume batch type.
func isBatchable(e database.ActivityEntry) bool {
	if e.OperationID == "" || e.Tier != "debug" {
		return false
	}
	switch e.Type {
	case "embedded-tag-load", "tag-scan", "metadata-apply", "path-repair", "isbn-enrich":
		return true
	}
	return false
}

// sendEntry parses a single log line and enqueues an ActivityEntry.
// Debug entries are silently dropped when the channel is full.
// Non-debug entries emit a warning to stdout when dropped.
func (w *Writer) sendEntry(line string) {
	if w.closed.Load() {
		return
	}
	parsed := ParseLogLineFull(line)
	if _, skip := w.skipSources[parsed.Source]; skip {
		return
	}
	// Tier is derived from level. The Activity Log UI defaults to
	// "debug tier excluded", so persisting every line at tier=debug
	// meant the page always showed 0 entries even though writes were
	// happening. info/warn/error → change so the user sees actual
	// progress; debug stays debug for the firehose.
	tier := "change"
	switch parsed.Level {
	case "debug":
		tier = "debug"
	case "warn", "warning", "error":
		tier = "change"
	}
	entry := database.ActivityEntry{
		Tier:        tier,
		Type:        "system",
		Level:       parsed.Level,
		Source:      parsed.Source,
		Summary:     parsed.Message,
		OperationID: parsed.OpID,
	}
	// Propagate component into Details so EnrichTags can produce
	// a component: tag without requiring a DB schema change.
	if parsed.Component != "" {
		entry.Details = map[string]any{"component": parsed.Component}
	}
	if isBatchable(entry) {
		w.batcher.Submit(BatchKey{
			Type:        entry.Type,
			Source:      entry.Source,
			OperationID: entry.OperationID,
		}, BatchItem{Name: entry.Summary})
		return
	}
	select {
	case w.ch <- entry:
	default:
		if parsed.Level != "debug" {
			w.stdout.Write([]byte("[WARN] activity channel full, dropped: " + parsed.Message + "\n")) //nolint:errcheck
		}
	}
}

// drain reads from the channel and persists entries in batches of up to 100,
// flushing at least every 500 ms. It stops when the done signal is received.
func (w *Writer) drain() {
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
// Each entry is enriched with derived tags (outcome:, source:, action:,
// lifecycle: for system entries) before persistence so the Activity Log UI
// has structured tags on every row — not just rows that went through
// Service.Record.
func (w *Writer) writeBatch(entries []database.ActivityEntry) {
	for _, e := range entries {
		EnrichTags(&e)
		w.store.Record(e) //nolint:errcheck
	}
}

// Flush synchronously drains any entries currently in the channel without
// stopping the background goroutine.
func (w *Writer) Flush() {
	w.batcher.FlushAll()
	for {
		select {
		case e := <-w.ch:
			EnrichTags(&e)
			w.store.Record(e) //nolint:errcheck
		default:
			return
		}
	}
}

// Chan returns the underlying entry channel. Intended for use in tests only —
// callers should not write to this channel directly.
func (w *Writer) Chan() <-chan database.ActivityEntry {
	return w.ch
}

// Stop marks the writer as closed, signals the drain goroutine to finish,
// and waits for it to flush all remaining entries. Safe to call multiple times.
// Implements the Stopper interface for serviceregistry.
func (w *Writer) Stop(ctx context.Context) error {
	w.batcher.Close()
	w.closed.Store(true)
	w.stopOnce.Do(func() { close(w.done) })
	w.wg.Wait()
	return nil
}

// ── log line parser ───────────────────────────────────────────────────────────

// ParsedLogLine holds all fields extracted from a single log line.
type ParsedLogLine struct {
	Level     string // "info", "warn", "error", "debug"
	Source    string // subsystem name, e.g. "scanner", "server"
	Message   string // human-readable message text
	OpID      string // operation_id / op_id slog attribute, if present
	Component string // component / subsystem slog attribute or source-path derived name
}

// ParseLogLineFull extracts all structured fields from a single log line,
// including op_id and component attributes when the line is in slog text format.
// ParseLogLine is a thin wrapper for callers that only need level/source/message.
func ParseLogLineFull(line string) ParsedLogLine {
	p := ParsedLogLine{}
	p.Level, p.Source, p.Message = parseLogLineCore(line)

	// For slog text lines, also extract op_id, component, and subsystem attrs.
	if strings.HasPrefix(line, "time=") && strings.Contains(line, " level=") && strings.Contains(line, " msg=") {
		p.OpID = extractSlogAttr(line, "op_id", "operation_id", "opID")
		p.Component = extractSlogAttr(line, "component", "subsystem", "pkg")
	}

	// If no explicit component, derive one from the source path field when the
	// slog Source attr includes a file path (e.g., "internal/plugins/acoustid/scan.go").
	if p.Component == "" {
		p.Component = deriveComponentFromSource(p.Source)
	}
	return p
}

// extractSlogAttr scans the slog key=value attrs in a text-format log line for
// any of the supplied key names and returns the first matching value. Values may
// be quoted or bare. Returns "" if none match.
func extractSlogAttr(line string, keys ...string) string {
	for _, key := range keys {
		needle := " " + key + "="
		idx := strings.Index(line, needle)
		if idx < 0 {
			continue
		}
		rest := line[idx+len(needle):]
		if rest == "" {
			continue
		}
		if rest[0] == '"' {
			// Quoted value: find the closing quote (skip escaped quotes).
			i := 1
			for i < len(rest) {
				if rest[i] == '\\' {
					i += 2
					continue
				}
				if rest[i] == '"' {
					return rest[1:i]
				}
				i++
			}
			continue
		}
		// Bare value: ends at next space.
		if sp := strings.IndexByte(rest, ' '); sp > 0 {
			return rest[:sp]
		}
		return rest
	}
	return ""
}

// deriveComponentFromSource maps known source file path segments to a
// component name. Returns "" when no known prefix matches, so we don't
// emit a misleading tag for unrecognised sources.
func deriveComponentFromSource(source string) string {
	lower := strings.ToLower(source)
	switch {
	case strings.Contains(lower, "acoustid") || strings.Contains(lower, "acousticid"):
		return "acoustid"
	case strings.Contains(lower, "itunes"):
		return "itunes_sync"
	case strings.Contains(lower, "scanner"):
		return "scanner"
	case strings.Contains(lower, "dedup"):
		return "dedup"
	case strings.Contains(lower, "isbn"):
		return "isbn_enrich"
	case strings.Contains(lower, "embed"):
		return "embedding"
	case strings.Contains(lower, "scheduler"):
		return "scheduler"
	case strings.Contains(lower, "maintenance"):
		return "maintenance"
	default:
		return ""
	}
}

// ParseLogLine extracts (level, source, message) from a single log line.
//
// Recognised formats:
//   - GIN: "[GIN] YYYY/MM/DD - HH:MM:SS | STATUS | ..."
//   - slog text: `time=... level=INFO msg="..."`
//   - Go standard log: "YYYY/MM/DD HH:MM:SS file.go:NNN: [level] source: message"
//   - Bare text: returned as-is with level=info, source=server.
func ParseLogLine(line string) (level, source, message string) {
	p := ParseLogLineFull(line)
	return p.Level, p.Source, p.Message
}

func parseLogLineCore(line string) (level, source, message string) {
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

	// slog TextHandler: time=... level=INFO msg="..." [attrs...]
	// We only care about level + msg; drop time and attrs. After
	// extracting msg, recurse so the wrapped "[INFO] source: message"
	// payload parses through the standard [level] branch and gets a
	// proper source.
	if strings.HasPrefix(line, "time=") && strings.Contains(line, " level=") && strings.Contains(line, " msg=") {
		lvl := "info"
		if li := strings.Index(line, " level="); li >= 0 {
			rest := line[li+len(" level="):]
			if sp := strings.IndexByte(rest, ' '); sp > 0 {
				lvl = strings.ToLower(rest[:sp])
			}
		}
		mi := strings.Index(line, " msg=")
		if mi >= 0 {
			msg := line[mi+len(" msg="):]
			// msg is quoted text; trim surrounding quotes and unescape \"
			if len(msg) > 0 && msg[0] == '"' {
				if end := strings.LastIndexByte(msg, '"'); end > 0 {
					msg = msg[1:end]
				}
				msg = strings.ReplaceAll(msg, `\"`, `"`)
			}
			// Recurse: msg often starts with "[INFO] source: ..." in our
			// code so the standard branch can extract a real source.
			if msg != "" && msg != line {
				rlvl, rsrc, rmsg := ParseLogLine(msg)
				if rsrc != "server" || rlvl != "info" {
					return rlvl, rsrc, rmsg
				}
				return lvl, "server", msg
			}
			return lvl, "server", msg
		}
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
			// Check for "source: message" — source must look like a subsystem name:
			// short (< 30 chars), no spaces, typically lowercase with hyphens/underscores
			if idx := strings.Index(work, ": "); idx > 0 && idx < 30 && !strings.Contains(work[:idx], " ") {
				source = work[:idx]
				message = work[idx+2:]
				return level, source, message
			}
			return level, "server", work
		}
	}

	return "info", "server", work
}
