// file: internal/logger/logger_test.go
// version: 1.0.0
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
