// file: internal/fingerprint/book_signature.go
// version: 1.0.0
// guid: 7f8e9d0c-1b2a-3f4e-5d6c-7e8f9a0b1c2d
// last-edited: 2026-05-03

package fingerprint

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"math/bits"
	"sort"
)

// ErrIncompleteFingerprint is returned when synthesizing a book signature from
// files that have not all been fingerprinted yet.
var ErrIncompleteFingerprint = errors.New("incomplete fingerprint: one or more files missing acoustid segments")

// BookSignatureFixedLength is the target down-sampled length in uint32 words.
// 4096 * 4 bytes = 16 KiB raw, ~22 KiB base64.
const BookSignatureFixedLength = 4096

// FileSegmentData holds the 7 acoustid segments for a single file.
type FileSegmentData struct {
	Seg0 string
	Seg1 string
	Seg2 string
	Seg3 string
	Seg4 string
	Seg5 string
	Seg6 string
}

// SynthesizeBookSignature synthesizes a unified per-book fingerprint from the
// per-file 7-segment chromaprint fingerprints. Files must be ordered by
// sort_order or filename. Returns (base64-encoded signature, pre-downsample
// segment count, error).
//
// Algorithm:
//  1. Concatenate all [seg0..seg6] from each file in order.
//  2. Decode base64 segments into one big []uint32.
//  3. Down-sample to BookSignatureFixedLength (4096) via max-pooling.
//  4. Re-encode to base64.
//
// Returns ErrIncompleteFingerprint if any file has an empty seg0.
func SynthesizeBookSignature(files []FileSegmentData) (string, int, error) {
	if len(files) == 0 {
		return "", 0, ErrIncompleteFingerprint
	}

	var allInts []uint32
	for _, f := range files {
		segs := []string{f.Seg0, f.Seg1, f.Seg2, f.Seg3, f.Seg4, f.Seg5, f.Seg6}
		for i, seg := range segs {
			if seg == "" {
				if i == 0 {
					return "", 0, ErrIncompleteFingerprint
				}
				continue
			}
			decoded, err := decodeAnyFingerprint(seg)
			if err != nil {
				return "", 0, fmt.Errorf("decode segment: %w", err)
			}
			allInts = append(allInts, decoded...)
		}
	}

	if len(allInts) == 0 {
		return "", 0, ErrIncompleteFingerprint
	}

	originalLen := len(allInts)
	downsampled := downsampleMaxPool(allInts, BookSignatureFixedLength)
	encoded := encodeUint32SliceToBase64(downsampled)
	return encoded, originalLen, nil
}

// downsampleMaxPool down-samples a uint32 slice to targetLen by max-pooling
// consecutive non-overlapping windows. For input shorter than targetLen, pads
// with zeros.
func downsampleMaxPool(data []uint32, targetLen int) []uint32 {
	if len(data) == 0 {
		out := make([]uint32, targetLen)
		return out
	}
	if len(data) <= targetLen {
		out := make([]uint32, targetLen)
		copy(out, data)
		return out
	}

	windowSize := (len(data) + targetLen - 1) / targetLen
	out := make([]uint32, targetLen)
	for i := 0; i < targetLen; i++ {
		start := i * windowSize
		end := start + windowSize
		if end > len(data) {
			end = len(data)
		}
		var maxVal uint32
		for j := start; j < end; j++ {
			if data[j] > maxVal {
				maxVal = data[j]
			}
		}
		out[i] = maxVal
	}
	return out
}

// encodeUint32SliceToBase64 encodes a []uint32 slice to base64 (little-endian).
func encodeUint32SliceToBase64(data []uint32) string {
	buf := make([]byte, len(data)*4)
	for i, val := range data {
		binary.LittleEndian.PutUint32(buf[i*4:], val)
	}
	return base64.StdEncoding.EncodeToString(buf)
}

// BookSignatureSimilarity compares two book signatures and returns a
// similarity score [0.0–1.0]. Both signatures must be base64-encoded
// BookSignatureFixedLength uint32 arrays.
//
// Algorithm:
//  1. Decode both to []uint32.
//  2. Compute Hamming distance per uint32 word (bits.OnesCount32(a[i] ^ b[i])).
//  3. Normalize: similarity = 1.0 - (totalBitDifferences / (4096*32)).
func BookSignatureSimilarity(a, b string) (float64, error) {
	intsA, err := decodeBase64Uint32Slice(a)
	if err != nil {
		return 0, fmt.Errorf("decode signature a: %w", err)
	}
	intsB, err := decodeBase64Uint32Slice(b)
	if err != nil {
		return 0, fmt.Errorf("decode signature b: %w", err)
	}
	if len(intsA) != BookSignatureFixedLength || len(intsB) != BookSignatureFixedLength {
		return 0, fmt.Errorf("invalid book signature length (expected %d, got %d and %d)", BookSignatureFixedLength, len(intsA), len(intsB))
	}

	var totalBitDiff int
	for i := 0; i < BookSignatureFixedLength; i++ {
		xor := intsA[i] ^ intsB[i]
		totalBitDiff += bits.OnesCount32(xor)
	}

	totalBits := BookSignatureFixedLength * 32
	errorRate := float64(totalBitDiff) / float64(totalBits)
	return 1.0 - errorRate, nil
}

// decodeBase64Uint32Slice decodes a base64 string into a []uint32 slice
// (little-endian).
func decodeBase64Uint32Slice(encoded string) ([]uint32, error) {
	b, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}
	if len(b)%4 != 0 {
		return nil, fmt.Errorf("decoded length %d is not a multiple of 4", len(b))
	}
	ints := make([]uint32, len(b)/4)
	for i := range ints {
		ints[i] = binary.LittleEndian.Uint32(b[i*4:])
	}
	return ints, nil
}

// OrderFileSegmentsByDefault sorts FileSegmentData in place by the provided
// sort keys. This is a helper for callers that need to order files by
// sort_order or original_filename before passing them to SynthesizeBookSignature.
type FileWithSegments struct {
	SortOrder int
	Filename  string
	Segments  FileSegmentData
}

// SortFilesByOrder sorts files by sort_order, falling back to filename.
func SortFilesByOrder(files []FileWithSegments) {
	sort.Slice(files, func(i, j int) bool {
		if files[i].SortOrder != files[j].SortOrder {
			return files[i].SortOrder < files[j].SortOrder
		}
		return files[i].Filename < files[j].Filename
	})
}
