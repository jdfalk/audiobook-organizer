// file: internal/dedup/backfill_progress_test.go
// version: 1.0.0
// guid: a7b8c9d0-e1f2-a3b4-c5d6-e7f8a9b0c1d2

package dedup

import (
	"testing"
)

func TestNewDedupScanProgressLoggerLogsAtIntervals(t *testing.T) {
	var logs []string
	logf := func(format string, args ...any) {
		logs = append(logs, "logged")
	}

	progressFn := NewDedupScanProgressLogger(10, logf)

	// At done=10, should log
	progressFn(10, 100)
	if len(logs) != 1 {
		t.Errorf("expected 1 log at done=10, got %d", len(logs))
	}

	// At done=15, no new log
	progressFn(15, 100)
	if len(logs) != 1 {
		t.Errorf("expected no new log at done=15, got %d total logs", len(logs))
	}

	// At done=20, should log
	progressFn(20, 100)
	if len(logs) != 2 {
		t.Errorf("expected 2 logs at done=20, got %d", len(logs))
	}
}

func TestNewDedupScanProgressLoggerCompletion(t *testing.T) {
	var logs []string
	logf := func(format string, args ...any) {
		logs = append(logs, "logged")
	}

	progressFn := NewDedupScanProgressLogger(100, logf)

	// At total=100, done=100, should log completion
	progressFn(100, 100)
	if len(logs) != 1 {
		t.Errorf("expected 1 log at completion, got %d", len(logs))
	}
}

func TestBackfillVersionMarkerConstant(t *testing.T) {
	if BackfillVersionMarker == "" {
		t.Error("BackfillVersionMarker should not be empty")
	}
	if !contains(BackfillVersionMarker, "v") {
		t.Error("BackfillVersionMarker should contain version marker")
	}
}

func contains(s, substr string) bool {
	for i := 0; i < len(s); i++ {
		if s[i:] >= substr && len(s[i:]) >= len(substr) {
			match := true
			for j := 0; j < len(substr); j++ {
				if s[i+j] != substr[j] {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
	}
	return false
}
