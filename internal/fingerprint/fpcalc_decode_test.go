// file: internal/fingerprint/fpcalc_decode_test.go
// version: 1.0.0
// guid: e2c4a6b8-9d0e-4f1a-8b2c-3d4e5f6a7b8c

package fingerprint

import (
	"encoding/base64"
	"encoding/binary"
	"strings"
	"testing"
)

// TestDecodeAnyFingerprint_URLSafeBase64 ensures fingerprints encoded with the
// URL-safe base64 alphabet (using '-' and '_' instead of '+' and '/') decode
// successfully. Regression test for log spam:
//
//	[WARN] acoustid backfill: synthesize book signature for ...:
//	  synthesize signature: decode segment: invalid character '-' in fingerprint
func TestDecodeAnyFingerprint_URLSafeBase64(t *testing.T) {
	// Build a payload guaranteed to round-trip through URL-safe base64
	// and contain at least one '-' or '_' when encoded.
	payload := make([]byte, 0, 64)
	// Header (4 bytes) + 15 uint32s = 64 bytes.
	header := []byte{0xfb, 0x01, 0x00, 0x00}
	payload = append(payload, header...)
	for i := uint32(0); i < 15; i++ {
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], 0xFFFFFFF0|i)
		payload = append(payload, b[:]...)
	}

	stdEncoded := base64.StdEncoding.EncodeToString(payload)
	urlEncoded := base64.URLEncoding.EncodeToString(payload)
	rawURLEncoded := base64.RawURLEncoding.EncodeToString(payload)

	// Sanity: URL-safe encoding should differ from std for this payload.
	if !strings.ContainsAny(urlEncoded, "-_") {
		t.Skip("payload happens not to contain URL-safe chars; pick another payload")
	}

	for name, fp := range map[string]string{
		"std":    stdEncoded,
		"url":    urlEncoded,
		"rawurl": rawURLEncoded,
	} {
		t.Run(name, func(t *testing.T) {
			ints, err := decodeAnyFingerprint(fp)
			if err != nil {
				t.Fatalf("decodeAnyFingerprint(%s) err: %v", name, err)
			}
			if len(ints) != 15 {
				t.Fatalf("expected 15 ints, got %d", len(ints))
			}
		})
	}
}
