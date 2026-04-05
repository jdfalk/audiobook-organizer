// file: internal/itunes/itl_le_remove_by_pid.go
// version: 1.0.0
// guid: e6f7a8b9-c0d1-2e3f-4a5b-6c7d8e9f0a1b
//
// Remove tracks from LE-format ITL payloads by persistent ID.

package itunes

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// RemoveTracksByPIDLE removes tracks whose persistent ID matches any key in pids.
// pids keys are lowercase hex strings (16 chars, no "0x" prefix).
// Returns the modified payload and the number of tracks removed.
func RemoveTracksByPIDLE(data []byte, pids map[string]bool) ([]byte, int) {
	if len(pids) == 0 {
		return data, 0
	}

	msdhOffset, msdhHeaderLen, msdhTotalLen := findMsdhByType(data, 1)
	if msdhOffset < 0 {
		return data, 0
	}

	contentStart := msdhOffset + msdhHeaderLen
	contentEnd := msdhOffset + msdhTotalLen

	// Find mlth header
	mlthOffset := -1
	mlthHeaderLen := 0
	if contentStart+12 <= contentEnd && readTag(data, contentStart) == "mlth" {
		mlthOffset = contentStart
		mlthHeaderLen = int(readUint32LE(data, contentStart+4))
	}

	// Walk tracks, collect spans to remove
	type span struct{ start, end int }
	var removeSpans []span
	removedCount := 0

	offset := contentStart + mlthHeaderLen
	var currentSpanStart int
	inTrack := false
	currentPIDMatch := false

	for offset+8 <= contentEnd {
		tag := readTag(data, offset)
		if tag == "" {
			break
		}
		headerLen := int(readUint32LE(data, offset+4))
		totalLen := int(readUint32LE(data, offset+8))
		length := headerLen
		if (tag == "mith" || tag == "mhoh" || tag == "miah") && totalLen > headerLen && totalLen <= contentEnd-offset {
			length = totalLen
		}
		if length < 8 || offset+length > contentEnd {
			break
		}

		if tag == "mith" {
			// Finish previous track span if it was a match
			if inTrack && currentPIDMatch {
				removeSpans = append(removeSpans, span{currentSpanStart, offset})
				removedCount++
			}
			// Start new track
			currentSpanStart = offset
			inTrack = true
			currentPIDMatch = false

			// Extract PID from mith (bytes 128-135, reversed for LE)
			if offset+136 <= len(data) {
				pid := extractMithPIDLE(data, offset)
				if pids[pid] {
					currentPIDMatch = true
				}
			}
		}

		offset += length
	}
	// Handle last track
	if inTrack && currentPIDMatch {
		removeSpans = append(removeSpans, span{currentSpanStart, offset})
		removedCount++
	}

	if removedCount == 0 {
		return data, 0
	}

	// Sort spans in descending order so we can splice from back to front
	// without invalidating earlier offsets
	sort.Slice(removeSpans, func(i, j int) bool {
		return removeSpans[i].start > removeSpans[j].start
	})

	result := make([]byte, len(data))
	copy(result, data)

	totalRemoved := 0
	for _, s := range removeSpans {
		size := s.end - s.start
		// Remove [s.start : s.end] from result
		result = append(result[:s.start], result[s.end:]...)
		totalRemoved += size
	}

	// Update mlth track count
	if mlthOffset >= 0 && mlthOffset+12 <= len(result) {
		oldCount := int(readUint32LE(result, mlthOffset+8))
		newCount := oldCount - removedCount
		if newCount < 0 {
			newCount = 0
		}
		writeUint32LE(result, mlthOffset+8, uint32(newCount))
	}

	// Update msdh totalLen
	if msdhOffset+12 <= len(result) {
		writeUint32LE(result, msdhOffset+8, uint32(msdhTotalLen-totalRemoved))
	}

	return result, removedCount
}

// extractMithPIDLE reads the 8-byte persistent ID from an LE mith block.
// In LE format, PIDs are stored in reversed byte order at offset 128-135.
// Returns a lowercase 16-char hex string matching the canonical format.
func extractMithPIDLE(data []byte, mithOffset int) string {
	var pid [8]byte
	for i := 0; i < 8; i++ {
		pid[i] = data[mithOffset+135-i]
	}
	return strings.ToLower(hex.EncodeToString(pid[:]))
}

// ExtractTrackPIDLE is the exported version of extractMithPIDLE for use by
// other packages that need to read PIDs from parsed track data.
func ExtractTrackPIDLE(data []byte, mithOffset int) string {
	return extractMithPIDLE(data, mithOffset)
}

// GeneratePIDHex generates a random 8-byte persistent ID and returns it
// as a lowercase 16-char hex string. Use this when provisioning new tracks
// for non-iTunes books.
func GeneratePIDHex() string {
	pid := randomPID()
	return fmt.Sprintf("%016x", pid)
}
