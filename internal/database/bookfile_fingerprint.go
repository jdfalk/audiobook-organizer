// file: internal/database/bookfile_fingerprint.go
// version: 1.0.0
// guid: d1e2f3g4-h5i6-7890-jkml-no1234567890
// last-edited: 2026-05-19

package database

import "time"

// GetAcoustIDSeg0 returns the AcoustIDSeg0 field for use with fingerprint.FileWithFingerprint interface.
func (bf *BookFile) GetAcoustIDSeg0() string {
	if bf == nil {
		return ""
	}
	return bf.AcoustIDSeg0
}

// GetUpdatedAt returns the UpdatedAt field for use with fingerprint.FileWithFingerprint interface.
func (bf *BookFile) GetUpdatedAt() time.Time {
	if bf == nil {
		return time.Time{}
	}
	return bf.UpdatedAt
}
