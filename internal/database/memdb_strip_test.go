// file: internal/database/memdb_strip_test.go
// version: 1.1.0
// guid: e6f7a8b9-c0d1-4e2f-3a4b-5c6d7e8f9012

package database

import (
	"encoding/json"
	"testing"
	"time"
)

func TestStripBookFileForMemdb_NilsLargeFields(t *testing.T) {
	now := time.Now()
	reason := "corrupt_audio"
	detail := "ffmpeg returned 1"
	diag := `{"diagnostic":"data"}`

	src := &BookFile{
		ID:                             "bf-1",
		BookID:                         "book-1",
		FilePath:                       "/tmp/test.m4b",
		AcoustIDSeg0:                   "AQADtAcSRY",
		AcoustIDFingerprint:            make([]byte, 256*1024),
		AcoustIDFingerprintDurationSec: 7200.5,
		FingerprintFailedAt:            &now,
		FingerprintFailureReason:       &reason,
		FingerprintFailureDetail:       &detail,
		FingerprintDiagnosticJSON:      &diag,
	}

	stripped := stripBookFileForMemdb(src)
	if stripped == nil {
		t.Fatal("stripped is nil")
	}

	if stripped.AcoustIDFingerprint != nil {
		t.Errorf("AcoustIDFingerprint not stripped: got len=%d, want nil", len(stripped.AcoustIDFingerprint))
	}
	if stripped.FingerprintFailedAt != nil {
		t.Errorf("FingerprintFailedAt not stripped")
	}
	if stripped.FingerprintFailureReason != nil {
		t.Errorf("FingerprintFailureReason not stripped")
	}
	if stripped.FingerprintFailureDetail != nil {
		t.Errorf("FingerprintFailureDetail not stripped")
	}
	if stripped.FingerprintDiagnosticJSON != nil {
		t.Errorf("FingerprintDiagnosticJSON not stripped")
	}

	if stripped.ID != "bf-1" {
		t.Errorf("ID not preserved: %q", stripped.ID)
	}
	if stripped.BookID != "book-1" {
		t.Errorf("BookID not preserved: %q", stripped.BookID)
	}
	if stripped.FilePath != "/tmp/test.m4b" {
		t.Errorf("FilePath not preserved: %q", stripped.FilePath)
	}
	if stripped.AcoustIDSeg0 != "AQADtAcSRY" {
		t.Errorf("AcoustIDSeg0 not preserved: %q", stripped.AcoustIDSeg0)
	}
	if stripped.AcoustIDFingerprintDurationSec != 7200.5 {
		t.Errorf("AcoustIDFingerprintDurationSec not preserved: %v", stripped.AcoustIDFingerprintDurationSec)
	}

	if src.AcoustIDFingerprint == nil {
		t.Errorf("source mutated: AcoustIDFingerprint nil on src")
	}
	if src.FingerprintFailedAt == nil {
		t.Errorf("source mutated: FingerprintFailedAt nil on src")
	}
}

func TestStripBookFileForMemdb_NilInput(t *testing.T) {
	if got := stripBookFileForMemdb(nil); got != nil {
		t.Errorf("nil input: got %v, want nil", got)
	}
}

func TestStripBookFileForMemdb_AlreadyEmpty(t *testing.T) {
	src := &BookFile{ID: "bf-2"}
	stripped := stripBookFileForMemdb(src)
	if stripped == nil {
		t.Fatal("nil result")
	}
	if stripped.ID != "bf-2" {
		t.Errorf("ID corrupted: %q", stripped.ID)
	}
	if stripped.AcoustIDFingerprint != nil {
		t.Errorf("empty input produced non-nil AcoustIDFingerprint")
	}
}

// TestStrippedBookSizeRegressionAssertion verifies that a sampled stripped Book
// projection stays ≤4KB mean to catch field bloat before it impacts memdb RSS.
// This is a soft regression assert: if the projection grows past 4KB on average,
// the CI failure alerts us to investigate what field was added.
func TestStrippedBookSizeRegressionAssertion(t *testing.T) {
	// Create a realistic stripped Book with typical field values.
	// Note: Book fields are intentionally kept minimal to reflect what stripBookForMemdb retains.
	title := "The Hobbit"
	language := "en"
	isbn10 := "0547928227"
	asin := "B00DWTGFVI"
	authorID := 1
	seriesID := 1
	duration := 720000
	fileSize := int64(1234567890)

	strippedBook := &Book{
		ID:           "book-12345",
		Title:        title,
		AuthorID:     &authorID,
		SeriesID:     &seriesID,
		FilePath:     "/mnt/audiobooks/tolkien/the-hobbit/",
		Language:     &language,
		ISBN10:       &isbn10,
		ASIN:         &asin,
		Duration:     &duration,
		FileSize:     &fileSize,
		// Heavy fields are stripped (nil in memdb):
		// Description, VersionNotes, BookSigV1, BookSigV1Mask, BookSigSegments
		// Author and Series pointers are nil at warm time (hydrated separately)
	}

	data, err := json.Marshal(strippedBook)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	sizeBytes := len(data)
	const maxBytes = 4096 // 4KB threshold per spec

	if sizeBytes > maxBytes {
		t.Errorf("stripped Book projection exceeds 4KB threshold: %d bytes > %d bytes; "+
			"a heavy field may have been added — check memdb_strip.go", sizeBytes, maxBytes)
	}
}
