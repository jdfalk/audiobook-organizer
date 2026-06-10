// file: internal/itunes/itl_decrypt_export.go
// version: 1.1.0
// guid: 9c8d7e6f-5a4b-3c2d-1e0f-9a8b7c6d5e4f
// last-edited: 2026-06-09

package itunes

import "fmt"

// DecryptAndInflateITL is a convenience helper that takes a raw ITL file's
// bytes (header + encrypted/compressed payload) and returns the fully
// decrypted, inflated payload — the same in-memory representation the
// internal ITL helpers (CollectMasterTrackIDsLE, FindDanglingMtphRefsLE,
// etc.) operate on.
//
// Used by the lightweight HTTP diagnostics endpoint (library-stats) so
// callers don't need to replicate the parseHdfmHeader → itlDecrypt →
// itlInflate dance themselves.
//
// Returns explicit error if decompression fails or payload exceeds cap (fail-closed).
func DecryptAndInflateITL(data []byte) ([]byte, error) {
	hdr, err := parseHdfmHeader(data)
	if err != nil {
		return nil, err
	}
	payload := data[hdr.headerLen:]
	decrypted := itlDecrypt(hdr, payload)
	decompressed, _, err := itlInflate(decrypted)
	if err != nil {
		return nil, fmt.Errorf("decompressing ITL payload: %w", err)
	}
	return decompressed, nil
}
