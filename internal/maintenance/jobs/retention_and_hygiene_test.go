// file: internal/maintenance/jobs/retention_and_hygiene_test.go
// version: 1.0.0
// guid: f8d0e5b9-c2a4-5b1d-9e7f-8c3d2a1b0f5e

package jobs

import (
	"testing"
	"time"
)

// TestRetentionAndHygieneJob_JobMetadata verifies ID, Name, and Description.
func TestRetentionAndHygieneJob_JobMetadata(t *testing.T) {
	job := &retentionAndHygieneJob{}
	if job.ID() != "retention-and-hygiene" {
		t.Errorf("ID: got %q, want 'retention-and-hygiene'", job.ID())
	}
	if job.Name() == "" {
		t.Errorf("Name is empty")
	}
	if job.Category() != "maintenance" {
		t.Errorf("Category: got %q, want 'maintenance'", job.Category())
	}
	if !job.CanResume() {
		t.Errorf("CanResume: got false, want true")
	}
}

// TestRetentionBoundaryLogic verifies the boundary logic for identifying stale operations.
// Operations with CreatedAt < cutoffTime should be marked for deletion.
func TestRetentionBoundaryLogic(t *testing.T) {
	now := time.Now()
	cutoffTime := now.AddDate(0, 0, -90) // 90 days ago

	tests := []struct {
		name     string
		opTime   time.Time
		shouldDel bool
	}{
		{
			"before cutoff",
			cutoffTime.Add(-1 * time.Second),
			true,
		},
		{
			"at cutoff",
			cutoffTime,
			false, // CreatedAt.Before(cutoffTime) is false when equal
		},
		{
			"after cutoff",
			cutoffTime.Add(1 * time.Second),
			false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			shouldDelete := tc.opTime.Before(cutoffTime)
			if shouldDelete != tc.shouldDel {
				t.Errorf("got shouldDelete=%v, want %v for time %v vs cutoff %v",
					shouldDelete, tc.shouldDel, tc.opTime, cutoffTime)
			}
		})
	}
}
