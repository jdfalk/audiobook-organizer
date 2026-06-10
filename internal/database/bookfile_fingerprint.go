// file: internal/database/bookfile_fingerprint.go
// version: 1.1.0
// guid: d1e2f3g4-h5i6-7890-jkml-no1234567890
// last-edited: 2026-06-10

package database

import "time"

// GetAcoustIDSeg0 satisfies fingerprint.FileWithFingerprint. Returns a
// non-empty string when the file has any fingerprint data, allowing
// fingerprint.ComputeFingerprintFields to compute the per-book
// fingerprint_status badge.
//
// After fable5 T019, AcoustIDSeg0..6 are stripped from memdb rows before
// insertion (memdb_strip.go). To preserve the badge for memdb-sourced callers
// (GetBookFilesForIDs → ComputeFingerprintFields), we fall back to
// AcoustIDFingerprintDurationSec: the duration is retained in memdb (it is a
// float64 scalar, never stripped) and is non-zero only when a whole-file
// chromaprint was successfully computed.
//
// Pebble-direct callers are unaffected — their BookFile copies have the
// original AcoustIDSeg0 value populated from storage.
func (bf *BookFile) GetAcoustIDSeg0() string {
	if bf == nil {
		return ""
	}
	if bf.AcoustIDSeg0 != "" {
		return bf.AcoustIDSeg0
	}
	// Fallback: whole-file fingerprint present (AcoustIDSeg0 stripped from
	// memdb rows by stripBookFileForMemdb — use duration as presence proxy).
	if bf.AcoustIDFingerprintDurationSec > 0 {
		return "wf" // non-empty sentinel; callers only test != ""
	}
	return ""
}

// GetUpdatedAt returns the UpdatedAt field for use with fingerprint.FileWithFingerprint interface.
func (bf *BookFile) GetUpdatedAt() time.Time {
	if bf == nil {
		return time.Time{}
	}
	return bf.UpdatedAt
}
