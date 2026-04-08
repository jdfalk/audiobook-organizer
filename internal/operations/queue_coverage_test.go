// file: internal/operations/queue_coverage_test.go
// version: 1.0.0

package operations

import (
	"context"
	"testing"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/logger"
)

// --- Coverage tests for functions not exercised by existing tests ---

func TestCoverage_truncateStr(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"short string", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"needs truncation", "hello world", 5, "hello…"},
		{"empty string", "", 5, ""},
		{"zero maxLen", "hello", 0, "…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateStr(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateStr(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestCoverage_formatChangeSummary(t *testing.T) {
	t.Run("with field name", func(t *testing.T) {
		c := &database.OperationChange{
			ChangeType: "metadata_update",
			FieldName:  "title",
			OldValue:   "Old Title",
			NewValue:   "New Title",
		}
		got := formatChangeSummary(c)
		if got != "title: Old Title → New Title" {
			t.Errorf("unexpected summary: %s", got)
		}
	})

	t.Run("without field name", func(t *testing.T) {
		c := &database.OperationChange{
			ChangeType: "book_create",
		}
		got := formatChangeSummary(c)
		if got != "book_create" {
			t.Errorf("unexpected summary: %s", got)
		}
	})

	t.Run("truncates long values", func(t *testing.T) {
		longValue := "This is a very long string that exceeds fifty characters in total length for testing purposes"
		c := &database.OperationChange{
			ChangeType: "metadata_update",
			FieldName:  "description",
			OldValue:   longValue,
			NewValue:   "short",
		}
		got := formatChangeSummary(c)
		if len(got) > 200 {
			t.Errorf("summary too long: %d chars", len(got))
		}
		// Should contain the field name
		if got == "metadata_update" {
			t.Error("should have field-based summary, not just change type")
		}
	})
}

func TestCoverage_queueStoreAdapter_NilStore(t *testing.T) {
	adapter := &queueStoreAdapter{store: nil}

	t.Run("AddOperationLog nil store", func(t *testing.T) {
		err := adapter.AddOperationLog("op1", "info", "msg", nil)
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("CreateOperationChange nil store", func(t *testing.T) {
		err := adapter.CreateOperationChange(&database.OperationChange{})
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("CreateOperationChange wrong type", func(t *testing.T) {
		err := adapter.CreateOperationChange("not a change")
		if err != nil {
			t.Errorf("expected nil error for wrong type, got %v", err)
		}
	})

	t.Run("UpdateOperationProgress nil store", func(t *testing.T) {
		err := adapter.UpdateOperationProgress("op1", 5, 10, "msg")
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})
}

func TestCoverage_SetOperationTimeout(t *testing.T) {
	t.Run("sets timeout", func(t *testing.T) {
		q := &OperationQueue{}
		q.SetOperationTimeout(5 * time.Minute)
		if q.timeout != 5*time.Minute {
			t.Errorf("expected 5m timeout, got %v", q.timeout)
		}
	})

	t.Run("nil queue does not panic", func(t *testing.T) {
		var q *OperationQueue
		q.SetOperationTimeout(5 * time.Minute) // should not panic
	})

	t.Run("zero disables timeout", func(t *testing.T) {
		q := &OperationQueue{timeout: 5 * time.Minute}
		q.SetOperationTimeout(0)
		if q.timeout != 0 {
			t.Errorf("expected 0 timeout, got %v", q.timeout)
		}
	})
}

func TestCoverage_SetGlobalOperationTimeout(t *testing.T) {
	oldQueue := GlobalQueue
	defer func() { GlobalQueue = oldQueue }()

	store := newMockStore(t)
	GlobalQueue = NewOperationQueue(store, 1)
	defer GlobalQueue.Shutdown(time.Second)

	SetGlobalOperationTimeout(10 * time.Minute)

	oq := GlobalQueue.(*OperationQueue)
	if oq.timeout != 10*time.Minute {
		t.Errorf("expected 10m timeout, got %v", oq.timeout)
	}
}

func TestCoverage_ReporterFromLogger(t *testing.T) {
	log := logger.New("test")
	reporter := ReporterFromLogger(log)

	t.Run("UpdateProgress does not error", func(t *testing.T) {
		err := reporter.UpdateProgress(5, 10, "halfway")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("Log levels", func(t *testing.T) {
		if err := reporter.Log("info", "info msg", nil); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if err := reporter.Log("warn", "warn msg", nil); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if err := reporter.Log("error", "error msg", nil); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if err := reporter.Log("debug", "debug msg", nil); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if err := reporter.Log("unknown", "default msg", nil); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("IsCanceled returns false", func(t *testing.T) {
		if reporter.IsCanceled() {
			t.Error("expected not canceled for StandardLogger")
		}
	})
}

func TestCoverage_LoggerFromReporter(t *testing.T) {
	t.Run("non-loggerProgressReporter returns default logger", func(t *testing.T) {
		reporter := &operationProgressReporter{operationID: "test"}
		l := LoggerFromReporter(reporter)
		if l == nil {
			t.Error("expected non-nil logger")
		}
	})
}

func TestCoverage_EnqueueResume(t *testing.T) {
	store := newMockStore(t)
	q := NewOperationQueue(store, 1)
	defer q.Shutdown(time.Second)

	t.Run("resume operation executes", func(t *testing.T) {
		done := make(chan bool, 1)
		fn := func(ctx context.Context, progress ProgressReporter) error {
			done <- true
			return nil
		}

		err := q.EnqueueResume("resume-1", "test", PriorityNormal, fn)
		if err != nil {
			t.Fatalf("EnqueueResume failed: %v", err)
		}

		select {
		case <-done:
			// success
		case <-time.After(2 * time.Second):
			t.Fatal("resumed operation did not execute")
		}
	})

	t.Run("duplicate resume rejected", func(t *testing.T) {
		blocker := make(chan struct{})
		fn := func(ctx context.Context, progress ProgressReporter) error {
			<-blocker
			return nil
		}
		err := q.EnqueueResume("resume-dup", "test", PriorityNormal, fn)
		if err != nil {
			t.Fatalf("first EnqueueResume failed: %v", err)
		}

		err = q.EnqueueResume("resume-dup", "test", PriorityNormal, fn)
		if err == nil {
			t.Fatal("expected error for duplicate resume")
		}
		close(blocker)
	})
}

// --- OperationState coverage tests ---

func TestCoverage_OperationState_Structs(t *testing.T) {
	t.Run("ITunesImportParams", func(t *testing.T) {
		params := ITunesImportParams{
			LibraryXMLPath: "/path/to/xml",
			LibraryPath:    "/path/to/lib",
			ImportMode:     "full",
			PathMappings:   map[string]string{"from": "to"},
			SkipDuplicates: true,
			EnrichMetadata: true,
			AutoOrganize:   false,
		}
		if params.LibraryXMLPath != "/path/to/xml" {
			t.Error("field not set correctly")
		}
	})

	t.Run("ScanParams", func(t *testing.T) {
		path := "/scan/path"
		params := ScanParams{
			FolderPath:  &path,
			ForceUpdate: true,
		}
		if *params.FolderPath != "/scan/path" {
			t.Error("field not set correctly")
		}
	})

	t.Run("OrganizeParams", func(t *testing.T) {
		params := OrganizeParams{Strategy: "copy"}
		if params.Strategy != "copy" {
			t.Error("field not set correctly")
		}
	})
}

func TestCoverage_OperationProgress_Struct(t *testing.T) {
	p := OperationProgress{
		Current: 5,
		Total:   10,
		Message: "halfway",
	}
	if p.Current != 5 || p.Total != 10 || p.Message != "halfway" {
		t.Error("OperationProgress fields not set correctly")
	}
}
