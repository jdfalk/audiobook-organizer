// file: internal/logger/logger_test.go
// version: 1.1.0
// guid: 9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d

package logger

import "testing"

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  Level
	}{
		{"trace", LevelTrace},
		{"debug", LevelDebug},
		{"info", LevelInfo},
		{"warn", LevelWarn},
		{"error", LevelError},
		{"garbage", LevelInfo}, // default
		{"", LevelInfo},
	}
	for _, tc := range tests {
		if got := ParseLevel(tc.input); got != tc.want {
			t.Errorf("ParseLevel(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestLevelString(t *testing.T) {
	if LevelDebug.String() != "debug" {
		t.Errorf("LevelDebug.String() = %q, want 'debug'", LevelDebug.String())
	}
}

func TestStandardLogger_With(t *testing.T) {
	log := New("parent")
	child := log.With("child")
	sl, ok := child.(*StandardLogger)
	if !ok {
		t.Fatal("expected *StandardLogger from With()")
	}
	if sl.subsystem != "parent.child" {
		t.Errorf("subsystem = %q, want 'parent.child'", sl.subsystem)
	}
}

func TestStandardLogger_NoOps(t *testing.T) {
	log := New("test")
	log.UpdateProgress(1, 10, "msg")
	log.RecordChange(Change{ChangeType: "test"})
	if log.IsCanceled() {
		t.Error("StandardLogger.IsCanceled() should return false")
	}
	if log.ChangeCounters() != nil {
		t.Error("StandardLogger.ChangeCounters() should return nil")
	}
}

func TestStandardLogger_ActivityWriter(t *testing.T) {
	var captured []string
	writer := &mockActivityWriter{
		addFunc: func(source, level, message string) error {
			captured = append(captured, level+":"+message)
			return nil
		},
	}
	log := NewWithActivityLog("test", writer)
	log.Debug("should not be captured")
	log.Info("should be captured")
	log.Warn("also captured")

	if len(captured) != 2 {
		t.Errorf("expected 2 captured, got %d", len(captured))
	}
}

type mockActivityWriter struct {
	addFunc func(source, level, message string) error
}

func (m *mockActivityWriter) AddSystemActivityLog(source, level, message string) error {
	if m.addFunc != nil {
		return m.addFunc(source, level, message)
	}
	return nil
}
