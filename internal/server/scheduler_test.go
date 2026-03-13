// file: internal/server/scheduler_test.go
// version: 1.0.0
// guid: f3a7c2d1-8b4e-4f09-a5c6-1d2e3f4a5b6c

package server

import (
	"testing"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestIsInMaintenanceWindowAt(t *testing.T) {
	tests := []struct {
		name    string
		start   int
		end     int
		hour    int
		enabled bool
		want    bool
	}{
		{"disabled", 1, 4, 2, false, false},
		{"in window", 1, 4, 2, true, true},
		{"at start (inclusive)", 1, 4, 1, true, true},
		{"before window", 1, 4, 0, true, false},
		{"at end (exclusive)", 1, 4, 4, true, false},
		{"midnight span in midnight", 23, 2, 0, true, true},
		{"midnight span in 1am", 23, 2, 1, true, true},
		{"midnight span at start", 23, 2, 23, true, true},
		{"midnight span out 2am", 23, 2, 2, true, false},
		{"midnight span out 3am", 23, 2, 3, true, false},
		{"midnight span out noon", 23, 2, 12, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.AppConfig.MaintenanceWindowEnabled = tt.enabled
			config.AppConfig.MaintenanceWindowStart = tt.start
			config.AppConfig.MaintenanceWindowEnd = tt.end
			got := isInMaintenanceWindowAt(tt.hour)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHasRunToday(t *testing.T) {
	ts := &TaskScheduler{lastMaintenanceRun: time.Time{}}
	assert.False(t, ts.hasRunToday(), "zero time should not count as today")

	ts.lastMaintenanceRun = time.Now()
	assert.True(t, ts.hasRunToday(), "current time should count as today")

	ts.lastMaintenanceRun = time.Now().AddDate(0, 0, -1)
	assert.False(t, ts.hasRunToday(), "yesterday should not count as today")
}
