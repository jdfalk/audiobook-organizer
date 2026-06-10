// file: internal/itunes/itl_combined_mutate.go
// version: 1.2.0
// guid: f7a8b9c0-d1e2-3f4a-5b6c-7d8e9f0a1b2c
//
// Combined ITL mutation: applies removes, adds, and location patches in a
// single read-modify-write pass. This avoids redundant decrypt/compress
// cycles on the 29MB production ITL.

package itunes

import (
	"fmt"
	"os"
	"strings"
)

// ITLOperationSet holds all pending mutations to apply in one pass.
type ITLOperationSet struct {
	Removes         map[string]bool     // PID hex strings to remove
	Adds            []ITLNewTrack       // New tracks to insert
	LocationUpdates []ITLLocationUpdate // Location changes for existing tracks
	MetadataUpdates []ITLMetadataUpdate // Metadata changes for existing tracks
}

// IsEmpty returns true if there are no operations to apply.
func (ops *ITLOperationSet) IsEmpty() bool {
	return len(ops.Removes) == 0 && len(ops.Adds) == 0 &&
		len(ops.LocationUpdates) == 0 && len(ops.MetadataUpdates) == 0
}

// ApplyITLOperations reads the ITL file, applies all mutations to the
// decompressed payload in one pass, and writes the result.
// Order: removes first, then adds, then location patches.
func ApplyITLOperations(inputPath, outputPath string, ops ITLOperationSet) (*ITLWriteBackResult, error) {
	if ops.IsEmpty() {
		return &ITLWriteBackResult{OutputPath: outputPath}, nil
	}

	data, err := os.ReadFile(inputPath)
	if err != nil {
		return nil, fmt.Errorf("reading ITL: %w", err)
	}

	hdr, err := parseHdfmHeader(data)
	if err != nil {
		return nil, err
	}

	payload := data[hdr.headerLen:]
	decrypted := itlDecrypt(hdr, payload)
	decompressed, wasCompressed, err := itlInflate(decrypted)
	if err != nil {
		return nil, fmt.Errorf("decompressing ITL payload: %w", err)
	}

	isLE := detectLE(decompressed)
	totalUpdated := 0

	// Phase 1: Removes
	if len(ops.Removes) > 0 {
		if isLE {
			var removed int
			decompressed, removed = RemoveTracksByPIDLE(decompressed, ops.Removes)
			totalUpdated += removed
		}
		// BE remove not implemented (production is LE)
	}

	// Phase 2: Adds
	if len(ops.Adds) > 0 {
		if isLE {
			decompressed = AddTracksLE(decompressed, ops.Adds)
			totalUpdated += len(ops.Adds)
		}
		// BE add not implemented (production is LE)
	}

	// Phase 3: Location patches
	if len(ops.LocationUpdates) > 0 {
		updateMap := make(map[string]string, len(ops.LocationUpdates))
		for _, u := range ops.LocationUpdates {
			updateMap[strings.ToLower(u.PersistentID)] = u.NewLocation
		}

		var patched int
		if isLE {
			decompressed, patched = rewriteChunksLE(decompressed, updateMap)
		} else {
			decompressed, patched = rewriteChunksBE(decompressed, updateMap)
		}
		totalUpdated += patched
	}

	// Phase 4: Metadata updates (title, artist, album, genre, etc.)
	if len(ops.MetadataUpdates) > 0 {
		if isLE {
			var metaUpdated int
			decompressed, metaUpdated = UpdateMetadataLE(decompressed, ops.MetadataUpdates)
			totalUpdated += metaUpdated
		}
	}

	// Safety gate: re-read the original payload and diff dangling playlist
	// refs. If our writes introduced any new orphans, refuse to write the
	// file rather than corrupt the iTunes library. Re-decoding the input
	// is intentional — we want to compare against an immutable baseline,
	// not whatever the in-memory `decompressed` started as before mutation.
	if isLE {
		origPayload := data[hdr.headerLen:]
		origDec := itlDecrypt(hdr, origPayload)
		origInflated, _, err := itlInflate(origDec)
		if err != nil {
			return nil, fmt.Errorf("decompressing original ITL for verification: %w", err)
		}
		if err := VerifyITLNoNewDanglingRefsLE(origInflated, decompressed); err != nil {
			return nil, fmt.Errorf("aborting ITL write to %s: %w", outputPath, err)
		}
	}

	return writeITLFile(outputPath, hdr, decompressed, wasCompressed, totalUpdated)
}

// ApplyITLOperationsInMemory applies the same mutations as ApplyITLOperations
// but returns the resulting ITL bytes instead of writing to disk.
// Used by the partial-export path (Task 033 / ARCH-6-4).
func ApplyITLOperationsInMemory(inputPath string, ops ITLOperationSet) ([]byte, error) {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return nil, fmt.Errorf("reading ITL: %w", err)
	}

	hdr, err := parseHdfmHeader(data)
	if err != nil {
		return nil, err
	}

	payload := data[hdr.headerLen:]
	decrypted := itlDecrypt(hdr, payload)
	decompressed, wasCompressed, err := itlInflate(decrypted)
	if err != nil {
		return nil, fmt.Errorf("decompressing ITL payload: %w", err)
	}
	isLE := detectLE(decompressed)

	if len(ops.Removes) > 0 && isLE {
		decompressed, _ = RemoveTracksByPIDLE(decompressed, ops.Removes)
	}
	if len(ops.Adds) > 0 && isLE {
		decompressed = AddTracksLE(decompressed, ops.Adds)
	}
	if len(ops.LocationUpdates) > 0 {
		updateMap := make(map[string]string, len(ops.LocationUpdates))
		for _, u := range ops.LocationUpdates {
			updateMap[strings.ToLower(u.PersistentID)] = u.NewLocation
		}
		if isLE {
			decompressed, _ = rewriteChunksLE(decompressed, updateMap)
		} else {
			decompressed, _ = rewriteChunksBE(decompressed, updateMap)
		}
	}
	if len(ops.MetadataUpdates) > 0 && isLE {
		decompressed, _ = UpdateMetadataLE(decompressed, ops.MetadataUpdates)
	}

	return encodeITLPayload(hdr, decompressed, wasCompressed)
}
