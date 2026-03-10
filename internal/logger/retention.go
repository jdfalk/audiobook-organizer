// file: internal/logger/retention.go
// version: 1.0.0
// guid: 7c9e2f14-3a5b-4d8c-b1e6-9f2047a85c30

package logger

import (
	"fmt"
	"time"
)

// RetentionStore is the subset of the store needed for log pruning.
type RetentionStore interface {
	PruneOperationLogs(olderThan time.Time) (int, error)
	PruneOperationChanges(olderThan time.Time) (int, error)
	PruneSystemActivityLogs(olderThan time.Time) (int, error)
}

// PruneOldLogs deletes logs, changes, and activity entries older than retentionDays.
// Returns total records pruned.
func PruneOldLogs(store RetentionStore, retentionDays int, log Logger) (int, error) {
	if retentionDays <= 0 {
		log.Info("log retention disabled (0 days), skipping prune")
		return 0, nil
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	total := 0

	n, err := store.PruneOperationLogs(cutoff)
	if err != nil {
		return total, fmt.Errorf("prune operation logs: %w", err)
	}
	total += n
	if n > 0 {
		log.Info("pruned %d operation log entries", n)
	}

	n, err = store.PruneOperationChanges(cutoff)
	if err != nil {
		return total, fmt.Errorf("prune operation changes: %w", err)
	}
	total += n
	if n > 0 {
		log.Info("pruned %d operation change entries", n)
	}

	n, err = store.PruneSystemActivityLogs(cutoff)
	if err != nil {
		return total, fmt.Errorf("prune system activity logs: %w", err)
	}
	total += n
	if n > 0 {
		log.Info("pruned %d system activity log entries", n)
	}

	return total, nil
}
