// file: internal/activity/api.go
// version: 1.0.0
// guid: 9a4f2e1b-3c7d-4b8e-a6f0-5d2c8e1b7a3f

package activity

import "github.com/jdfalk/audiobook-organizer/internal/database"

// batchableTypes is the set of type strings that route through the ActivityBatcher.
// Only entries with both a non-empty OperationID and a registered type are batched.
var batchableTypes = map[string]bool{
	"embedded-tag-load": true,
	"tag-scan":          true,
	"metadata-apply":    true,
	"path-repair":       true,
	"isbn-enrich":       true,
}

// LogBatch submits a single BatchItem to the batcher inside w for the given
// operationID and batchType. If operationID is empty or batchType is not
// registered as batchable, the item is emitted as a plain debug ActivityEntry
// instead (non-blocking, best-effort). Safe to call from multiple goroutines.
// w may be nil — in that case the call is a no-op.
func LogBatch(w *Writer, operationID, batchType, source string, item BatchItem) {
	if w == nil {
		return
	}
	if operationID == "" || !batchableTypes[batchType] {
		// Fallback: emit as a plain debug entry; non-blocking.
		select {
		case w.ch <- database.ActivityEntry{
			Tier:        "debug",
			Type:        batchType,
			Level:       "info",
			Source:      source,
			OperationID: operationID,
			Summary:     item.Name,
		}:
		default:
			// channel full — drop silently, same policy as sendEntry for debug
		}
		return
	}
	w.batcher.Submit(BatchKey{
		Type:        batchType,
		Source:      source,
		OperationID: operationID,
	}, item)
}

// FlushOperation immediately flushes all pending batches whose OperationID
// matches operationID. Call this just before recording an operation's
// completion event, so the batch rows land before the completion row.
// Safe to call from any goroutine. w may be nil.
func FlushOperation(w *Writer, operationID string) {
	if w == nil || operationID == "" {
		return
	}
	w.batcher.mu.Lock()
	keys := make([]BatchKey, 0)
	for k := range w.batcher.pending {
		if k.OperationID == operationID {
			keys = append(keys, k)
		}
	}
	w.batcher.mu.Unlock()
	for _, k := range keys {
		w.batcher.flushKey(k)
	}
}
