// file: internal/fingerprint/calculator.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890
// last-edited: 2026-05-19

package fingerprint

import (
	"time"
)

// Constants for fingerprint status
const (
	FingerprintStatusNone     = "none"
	FingerprintStatusPartial  = "partial"
	FingerprintStatusComplete = "complete"
)

// FileWithFingerprint is a minimal interface for files that have fingerprinting data.
// This avoids a circular import with the database package.
type FileWithFingerprint interface {
	GetAcoustIDSeg0() string
	GetUpdatedAt() time.Time
}

// ComputeFingerprintFields calculates fingerprinting status and coverage for a book
// given its files. Returns FingerprintStatus, FingerprintedFileCount, CoveragePercent,
// and the most recent LastFingerprintedAt timestamp.
//
// Files are considered fingerprinted if GetAcoustIDSeg0() returns a non-empty string.
func ComputeFingerprintFields(files []FileWithFingerprint) (status string, fingerprintedCount, coveragePercent int, lastFingerprintedAt *time.Time) {
	if len(files) == 0 {
		return FingerprintStatusNone, 0, 0, nil
	}

	fingerprintedCount = 0
	var maxTime *time.Time

	for _, f := range files {
		// A file is considered fingerprinted if AcoustIDSeg0 is populated
		if f.GetAcoustIDSeg0() != "" {
			fingerprintedCount++
			// Track the most recent update time
			updatedAt := f.GetUpdatedAt()
			if maxTime == nil || updatedAt.After(*maxTime) {
				maxTime = &updatedAt
			}
		}
	}

	// Determine status
	switch {
	case fingerprintedCount == 0:
		status = FingerprintStatusNone
	case fingerprintedCount == len(files):
		status = FingerprintStatusComplete
	default:
		status = FingerprintStatusPartial
	}

	// Calculate coverage percentage
	coveragePercent = (fingerprintedCount * 100) / len(files)
	if coveragePercent == 0 && fingerprintedCount > 0 {
		coveragePercent = 1 // Avoid showing 0% for partial fingerprints
	}

	lastFingerprintedAt = maxTime

	return status, fingerprintedCount, coveragePercent, lastFingerprintedAt
}
