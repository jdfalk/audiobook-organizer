// file: internal/fingerprint/fpcalc.go
// version: 1.0.0
// guid: b1c2d3e4-f5a6-7b8c-9d0e-1f2a3b4c5d6e

// Package fingerprint wraps the fpcalc CLI (from the Chromaprint project) to
// generate AcoustID fingerprints for audio files. Fingerprints are used for
// content-based library matching — they survive metadata rewrites, file moves,
// and format/container changes as long as the audio stream is unchanged.
//
// fpcalc must be installed on the system (e.g. `apt install libchromaprint-tools`
// or `brew install chromaprint`). If not found, all functions return ErrNotAvailable
// so callers can gracefully degrade.
package fingerprint

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// ErrNotAvailable is returned when fpcalc is not found on PATH.
var ErrNotAvailable = errors.New("fpcalc not found — install chromaprint-tools")

// Result holds the output of a single fpcalc run.
type Result struct {
	// Duration is the audio duration in seconds as reported by fpcalc.
	Duration float64 `json:"duration"`
	// Fingerprint is the raw AcoustID fingerprint string.
	// For exact matching, compare this string directly.
	// For fuzzy matching, decode via DecodeFingerprint and compute Hamming distance.
	Fingerprint string `json:"fingerprint"`
}

// File generates an AcoustID fingerprint for the given audio file.
// Returns ErrNotAvailable if fpcalc is not on PATH.
// The fingerprint covers the first ~120 seconds of audio (fpcalc default) —
// enough for reliable identification without reading the entire file.
func File(path string) (*Result, error) {
	fpcalc, err := exec.LookPath("fpcalc")
	if err != nil {
		return nil, ErrNotAvailable
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.Command(fpcalc, "-json", path)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("fpcalc %s: %s", path, msg)
	}

	var r Result
	if err := json.NewDecoder(&stdout).Decode(&r); err != nil {
		return nil, fmt.Errorf("fpcalc parse %s: %w", path, err)
	}
	if r.Fingerprint == "" {
		return nil, fmt.Errorf("fpcalc returned empty fingerprint for %s", path)
	}
	return &r, nil
}

// Available reports whether fpcalc is present on PATH.
func Available() bool {
	_, err := exec.LookPath("fpcalc")
	return err == nil
}

// HammingSimilarity returns the fraction of bits that agree between two
// AcoustID fingerprint strings (0.0–1.0). Used for fuzzy fallback matching.
// Returns 0 and an error if either fingerprint cannot be decoded.
func HammingSimilarity(a, b string) (float64, error) {
	ints_a, err := decodeFingerprint(a)
	if err != nil {
		return 0, fmt.Errorf("decode a: %w", err)
	}
	ints_b, err := decodeFingerprint(b)
	if err != nil {
		return 0, fmt.Errorf("decode b: %w", err)
	}

	// Compare over the shorter length.
	n := len(ints_a)
	if len(ints_b) < n {
		n = len(ints_b)
	}
	if n == 0 {
		return 0, errors.New("empty fingerprint")
	}

	var matching uint32
	var total uint32
	for i := 0; i < n; i++ {
		xor := ints_a[i] ^ ints_b[i]
		// Count bits that match (32 - popcount(xor)).
		matching += 32 - popcount(xor)
		total += 32
	}
	return float64(matching) / float64(total), nil
}

// FuzzyMinSimilarity is the default threshold for fuzzy matching.
// 0.80 means 80% of bits agree — tolerant of minor encoding differences
// while rejecting different recordings of the same title.
const FuzzyMinSimilarity = 0.80

// decodeFingerprint base62-decodes an AcoustID fingerprint string into its
// underlying int32 array. AcoustID uses a custom base62 alphabet.
func decodeFingerprint(fp string) ([]uint32, error) {
	const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	lookup := [128]int{}
	for i := range lookup {
		lookup[i] = -1
	}
	for i, c := range alphabet {
		lookup[c] = i
	}

	var bits uint64
	numBits := 0
	var result []uint32

	for _, c := range fp {
		if int(c) >= 128 || lookup[c] < 0 {
			return nil, fmt.Errorf("invalid character %q in fingerprint", c)
		}
		bits |= uint64(lookup[c]) << numBits
		numBits += 6
		for numBits >= 32 {
			result = append(result, uint32(bits))
			bits >>= 32
			numBits -= 32
		}
	}
	return result, nil
}

// popcount counts the number of set bits in a uint32.
func popcount(x uint32) uint32 {
	x = x - ((x >> 1) & 0x55555555)
	x = (x & 0x33333333) + ((x >> 2) & 0x33333333)
	x = (x + (x >> 4)) & 0x0f0f0f0f
	return (x * 0x01010101) >> 24
}
