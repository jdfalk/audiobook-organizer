// file: internal/fingerprint/book_signature_test.go
// version: 1.0.0
// guid: 8f9e0a1b-2c3d-4e5f-6a7b-8c9d0e1f2a3b
// last-edited: 2026-05-03

package fingerprint

import (
	"math/rand"
	"strings"
	"testing"
)

// makeTestSegment creates a valid fingerprint segment from uint32 values.
func makeTestSegment(values ...uint32) string {
	return encodeUint32SliceToBase64(values)
}

func TestSynthesizeBookSignature_Deterministic(t *testing.T) {
	// Use simple uint32 arrays encoded to base64
	files := []FileSegmentData{
		{
			Seg0: makeTestSegment(1, 2, 3, 4),
			Seg1: makeTestSegment(5, 6, 7, 8),
			Seg2: makeTestSegment(9, 10, 11, 12),
			Seg3: makeTestSegment(13, 14, 15, 16),
			Seg4: makeTestSegment(17, 18, 19, 20),
			Seg5: makeTestSegment(21, 22, 23, 24),
			Seg6: makeTestSegment(25, 26, 27, 28),
		},
	}

	sig1, segCount1, err := SynthesizeBookSignature(files)
	if err != nil {
		t.Fatalf("SynthesizeBookSignature failed: %v", err)
	}
	if sig1 == "" {
		t.Fatal("signature is empty")
	}
	if segCount1 <= 0 {
		t.Fatalf("segment count should be > 0, got %d", segCount1)
	}

	sig2, segCount2, err := SynthesizeBookSignature(files)
	if err != nil {
		t.Fatalf("second SynthesizeBookSignature failed: %v", err)
	}
	if sig1 != sig2 {
		t.Errorf("signature not deterministic: run1=%s, run2=%s", sig1, sig2)
	}
	if segCount1 != segCount2 {
		t.Errorf("segment count not deterministic: %d vs %d", segCount1, segCount2)
	}
}

func TestBookSignatureSimilarity_Identical(t *testing.T) {
	files := []FileSegmentData{
		{
			Seg0: makeTestSegment(1, 2, 3, 4),
			Seg1: makeTestSegment(5, 6, 7, 8),
			Seg2: makeTestSegment(9, 10, 11, 12),
			Seg3: makeTestSegment(13, 14, 15, 16),
			Seg4: makeTestSegment(17, 18, 19, 20),
			Seg5: makeTestSegment(21, 22, 23, 24),
			Seg6: makeTestSegment(25, 26, 27, 28),
		},
	}

	sig, _, err := SynthesizeBookSignature(files)
	if err != nil {
		t.Fatalf("SynthesizeBookSignature failed: %v", err)
	}

	sim, err := BookSignatureSimilarity(sig, sig)
	if err != nil {
		t.Fatalf("BookSignatureSimilarity failed: %v", err)
	}
	if sim != 1.0 {
		t.Errorf("identical signatures should have similarity 1.0, got %f", sim)
	}
}

func TestSynthesizeBookSignature_MultiFileSplit(t *testing.T) {
	// Simulate a 30-file split: each file has the same 7 segments
	// (in reality they'd differ, but this tests the concatenation logic).
	multiFile := make([]FileSegmentData, 30)
	for i := range multiFile {
		multiFile[i] = FileSegmentData{
			Seg0: makeTestSegment(1, 2, 3, 4),
			Seg1: makeTestSegment(5, 6, 7, 8),
			Seg2: makeTestSegment(9, 10, 11, 12),
			Seg3: makeTestSegment(13, 14, 15, 16),
			Seg4: makeTestSegment(17, 18, 19, 20),
			Seg5: makeTestSegment(21, 22, 23, 24),
			Seg6: makeTestSegment(25, 26, 27, 28),
		}
	}

	// Simulate a 1-file version with the "same" segments
	singleFile := []FileSegmentData{
		{
			Seg0: makeTestSegment(1, 2, 3, 4),
			Seg1: makeTestSegment(5, 6, 7, 8),
			Seg2: makeTestSegment(9, 10, 11, 12),
			Seg3: makeTestSegment(13, 14, 15, 16),
			Seg4: makeTestSegment(17, 18, 19, 20),
			Seg5: makeTestSegment(21, 22, 23, 24),
			Seg6: makeTestSegment(25, 26, 27, 28),
		},
	}

	sigMulti, _, err := SynthesizeBookSignature(multiFile)
	if err != nil {
		t.Fatalf("SynthesizeBookSignature (multi-file) failed: %v", err)
	}
	sigSingle, _, err := SynthesizeBookSignature(singleFile)
	if err != nil {
		t.Fatalf("SynthesizeBookSignature (single-file) failed: %v", err)
	}

	sim, err := BookSignatureSimilarity(sigMulti, sigSingle)
	if err != nil {
		t.Fatalf("BookSignatureSimilarity failed: %v", err)
	}

	// The threshold in the spec is 0.85. Because we're using identical segments
	// repeated, the similarity should be very high. If the segments were actually
	// different across the split, we'd tune the threshold. For now, expect >= 0.85.
	if sim < 0.85 {
		t.Errorf("multi-file vs single-file similarity too low: %f (expected >= 0.85)", sim)
	}
	t.Logf("Multi-file vs single-file similarity: %f", sim)
}

func TestSynthesizeBookSignature_UnrelatedBooks(t *testing.T) {
	// Generate large test data with truly random values using different seeds
	segRandom := func(seed int64) string {
		rng := rand.New(rand.NewSource(seed))
		vals := make([]uint32, 1000)
		for i := range vals {
			vals[i] = rng.Uint32()
		}
		return encodeUint32SliceToBase64(vals)
	}

	bookA := []FileSegmentData{
		{
			Seg0: segRandom(1),
			Seg1: segRandom(2),
			Seg2: segRandom(3),
			Seg3: segRandom(4),
			Seg4: segRandom(5),
			Seg5: segRandom(6),
			Seg6: segRandom(7),
		},
	}

	bookB := []FileSegmentData{
		{
			Seg0: segRandom(1000),
			Seg1: segRandom(2000),
			Seg2: segRandom(3000),
			Seg3: segRandom(4000),
			Seg4: segRandom(5000),
			Seg5: segRandom(6000),
			Seg6: segRandom(7000),
		},
	}

	sigA, _, err := SynthesizeBookSignature(bookA)
	if err != nil {
		t.Fatalf("SynthesizeBookSignature (bookA) failed: %v", err)
	}
	sigB, _, err := SynthesizeBookSignature(bookB)
	if err != nil {
		t.Fatalf("SynthesizeBookSignature (bookB) failed: %v", err)
	}

	sim, err := BookSignatureSimilarity(sigA, sigB)
	if err != nil {
		t.Fatalf("BookSignatureSimilarity failed: %v", err)
	}

	// For synthetic random test data, we expect ~50% similarity due to random
	// bit patterns. Real chromaprint data has structure that allows lower
	// thresholds (~0.4). For this test, we verify that unrelated books have
	// *significantly lower* similarity than the related-books test (which is ~0.99).
	// In production, empirical tuning based on real audio fingerprints may allow
	// a lower threshold (the spec suggests 0.4).
	if sim > 0.7 {
		t.Errorf("unrelated books similarity too high: %f (expected <= 0.7 for synthetic test data)", sim)
	}
	t.Logf("Unrelated books similarity: %f", sim)
}

func TestSynthesizeBookSignature_IncompleteFingerprint(t *testing.T) {
	files := []FileSegmentData{
		{
			Seg0: "",
			Seg1: makeTestSegment(5, 6, 7, 8),
			Seg2: makeTestSegment(9, 10, 11, 12),
			Seg3: makeTestSegment(13, 14, 15, 16),
			Seg4: makeTestSegment(17, 18, 19, 20),
			Seg5: makeTestSegment(21, 22, 23, 24),
			Seg6: makeTestSegment(25, 26, 27, 28),
		},
	}

	sig, _, err := SynthesizeBookSignature(files)
	if err != ErrIncompleteFingerprint {
		t.Errorf("expected ErrIncompleteFingerprint when seg0 is empty, got: %v (sig=%s)", err, sig)
	}
}

func TestDownsampleMaxPool_Short(t *testing.T) {
	data := []uint32{1, 2, 3, 4, 5}
	out := downsampleMaxPool(data, 10)
	if len(out) != 10 {
		t.Errorf("expected length 10, got %d", len(out))
	}
	for i := 0; i < 5; i++ {
		if out[i] != data[i] {
			t.Errorf("mismatch at %d: expected %d, got %d", i, data[i], out[i])
		}
	}
	for i := 5; i < 10; i++ {
		if out[i] != 0 {
			t.Errorf("expected 0 at %d, got %d", i, out[i])
		}
	}
}

func TestDownsampleMaxPool_Long(t *testing.T) {
	data := make([]uint32, 10000)
	for i := range data {
		data[i] = uint32(i)
	}
	out := downsampleMaxPool(data, 100)
	if len(out) != 100 {
		t.Errorf("expected length 100, got %d", len(out))
	}
	// Each window should contain the max value from that window.
	// window 0: data[0..99] → max=99
	// window 1: data[100..199] → max=199, etc.
	if out[0] < 99 {
		t.Errorf("window 0 max should be >= 99, got %d", out[0])
	}
}

func TestSortFilesByOrder(t *testing.T) {
	files := []FileWithSegments{
		{SortOrder: 3, Filename: "c.mp3"},
		{SortOrder: 1, Filename: "a.mp3"},
		{SortOrder: 2, Filename: "b.mp3"},
		{SortOrder: 0, Filename: "z.mp3"},
	}

	SortFilesByOrder(files)

	expected := []string{"z.mp3", "a.mp3", "b.mp3", "c.mp3"}
	for i, f := range files {
		if f.Filename != expected[i] {
			t.Errorf("index %d: expected %s, got %s", i, expected[i], f.Filename)
		}
	}
}

func TestSortFilesByOrder_FallbackToFilename(t *testing.T) {
	files := []FileWithSegments{
		{SortOrder: 0, Filename: "c.mp3"},
		{SortOrder: 0, Filename: "a.mp3"},
		{SortOrder: 0, Filename: "b.mp3"},
	}

	SortFilesByOrder(files)

	expected := []string{"a.mp3", "b.mp3", "c.mp3"}
	for i, f := range files {
		if f.Filename != expected[i] {
			t.Errorf("index %d: expected %s, got %s", i, expected[i], f.Filename)
		}
	}
}

// TestEncodeDecodeRoundTrip ensures that encoding and decoding preserves data.
func TestEncodeDecodeRoundTrip(t *testing.T) {
	data := make([]uint32, 100)
	for i := range data {
		data[i] = uint32(i * 13)
	}

	encoded := encodeUint32SliceToBase64(data)
	decoded, err := decodeBase64Uint32Slice(encoded)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if len(decoded) != len(data) {
		t.Fatalf("length mismatch: %d != %d", len(decoded), len(data))
	}
	for i := range data {
		if decoded[i] != data[i] {
			t.Errorf("mismatch at %d: %d != %d", i, decoded[i], data[i])
		}
	}
}

// TestBookSignatureSimilarity_InvalidLength ensures we reject non-4096 signatures.
func TestBookSignatureSimilarity_InvalidLength(t *testing.T) {
	short := encodeUint32SliceToBase64(make([]uint32, 10))
	valid := encodeUint32SliceToBase64(make([]uint32, BookSignatureFixedLength))

	_, err := BookSignatureSimilarity(short, valid)
	if err == nil {
		t.Error("expected error for mismatched signature length")
	}
	if !strings.Contains(err.Error(), "invalid book signature length") {
		t.Errorf("unexpected error message: %v", err)
	}
}
