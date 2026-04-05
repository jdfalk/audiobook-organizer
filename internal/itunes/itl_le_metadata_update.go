// file: internal/itunes/itl_le_metadata_update.go
// version: 1.0.0
// guid: b2c3d4e5-f6a7-8901-bcde-f01234567890
//
// Update track metadata (title, artist, album, genre, etc.) in LE-format
// ITL payloads by persistent ID. This writes directly to the ITL's mhoh
// chunks so iTunes sees the changes without needing to re-read audio files.

package itunes

import (
	"bytes"
	"strings"
)

// ITLMetadataUpdate describes metadata changes for a single track.
// Only non-empty fields are written; empty fields are left unchanged.
type ITLMetadataUpdate struct {
	PersistentID string // hex-encoded 8-byte PID (required)
	Name         string // hohm type 0x02
	Album        string // hohm type 0x03
	Artist       string // hohm type 0x04
	Genre        string // hohm type 0x05
	Kind         string // hohm type 0x06
	Composer     string // hohm type 0x0C
	Location     string // hohm type 0x0D
}

// UpdateMetadataLE rewrites mhoh chunks for tracks matching the given updates.
// For each matching PID, it replaces existing mhoh chunks of the specified types
// and appends new ones for types that don't exist yet.
// Returns the modified payload and count of tracks updated.
func UpdateMetadataLE(data []byte, updates []ITLMetadataUpdate) ([]byte, int) {
	if len(updates) == 0 {
		return data, 0
	}

	// Build lookup by PID
	updateMap := make(map[string]*ITLMetadataUpdate, len(updates))
	for i := range updates {
		updateMap[strings.ToLower(updates[i].PersistentID)] = &updates[i]
	}

	msdhOffset, msdhHeaderLen, msdhTotalLen := findMsdhByType(data, 1)
	if msdhOffset < 0 {
		return data, 0
	}

	contentStart := msdhOffset + msdhHeaderLen
	contentEnd := msdhOffset + msdhTotalLen

	// Skip mlth header
	mlthHeaderLen := 0
	if contentStart+12 <= contentEnd && readTag(data, contentStart) == "mlth" {
		mlthHeaderLen = int(readUint32LE(data, contentStart+4))
	}

	// Walk tracks, rebuild the track section with updated mhoh chunks
	var result bytes.Buffer
	// Write everything before the track content
	result.Write(data[:contentStart+mlthHeaderLen])

	offset := contentStart + mlthHeaderLen
	updatedCount := 0

	for offset+8 <= contentEnd {
		// Find next mith (track start)
		tag := readTag(data, offset)
		if tag == "" {
			break
		}
		if tag != "mith" {
			// Non-track chunk (shouldn't happen inside track section, but be safe)
			headerLen := int(readUint32LE(data, offset+4))
			totalLen := int(readUint32LE(data, offset+8))
			length := headerLen
			if totalLen > headerLen && totalLen <= contentEnd-offset {
				length = totalLen
			}
			if length < 8 || offset+length > contentEnd {
				break
			}
			result.Write(data[offset : offset+length])
			offset += length
			continue
		}

		// Found a mith — collect its span (mith + all following mhoh)
		mithHeaderLen := int(readUint32LE(data, offset+4))
		mithTotalLen := int(readUint32LE(data, offset+8))
		mithLen := mithHeaderLen
		if mithTotalLen > mithHeaderLen && mithTotalLen <= contentEnd-offset {
			mithLen = mithTotalLen
		}
		if mithLen < 8 || offset+mithLen > contentEnd {
			break
		}

		// Extract PID
		pid := ""
		if offset+136 <= len(data) {
			pid = extractMithPIDLE(data, offset)
		}

		update, hasUpdate := updateMap[pid]
		if !hasUpdate {
			// No update for this track — copy as-is
			result.Write(data[offset : offset+mithLen])
			offset += mithLen
			continue
		}

		// Track needs updating — write mith header, then rebuild mhoh chunks
		mithHeader := make([]byte, mithHeaderLen)
		copy(mithHeader, data[offset:offset+mithHeaderLen])

		// Collect existing mhoh chunks and their types
		type mhohChunk struct {
			hohmType uint32
			data     []byte
		}
		var existingMhohs []mhohChunk
		mhohOffset := offset + mithHeaderLen
		for mhohOffset+8 <= offset+mithLen {
			mhohTag := readTag(data, mhohOffset)
			if mhohTag != "mhoh" {
				break
			}
			mhohHdrLen := int(readUint32LE(data, mhohOffset+4))
			mhohTotalLen := int(readUint32LE(data, mhohOffset+8))
			mhohLen := mhohHdrLen
			if mhohTotalLen > mhohHdrLen && mhohTotalLen <= (offset+mithLen)-mhohOffset {
				mhohLen = mhohTotalLen
			}
			if mhohLen < 8 || mhohOffset+mhohLen > offset+mithLen {
				break
			}
			ht := readUint32LE(data, mhohOffset+12)
			existingMhohs = append(existingMhohs, mhohChunk{
				hohmType: ht,
				data:     data[mhohOffset : mhohOffset+mhohLen],
			})
			mhohOffset += mhohLen
		}

		// Build replacement mhoh list
		replacements := map[uint32]string{}
		if update.Location != "" {
			replacements[0x0D] = update.Location
		}
		if update.Name != "" {
			replacements[0x02] = update.Name
		}
		if update.Album != "" {
			replacements[0x03] = update.Album
		}
		if update.Artist != "" {
			replacements[0x04] = update.Artist
		}
		if update.Genre != "" {
			replacements[0x05] = update.Genre
		}
		if update.Kind != "" {
			replacements[0x06] = update.Kind
		}
		if update.Composer != "" {
			replacements[0x0C] = update.Composer
		}

		// Rebuild mhoh list: replace existing, track which were replaced
		replaced := make(map[uint32]bool)
		var newMhohs bytes.Buffer
		for _, existing := range existingMhohs {
			if newVal, ok := replacements[existing.hohmType]; ok {
				// Replace with new value
				newMhohs.Write(buildMhohLE(existing.hohmType, newVal))
				replaced[existing.hohmType] = true
			} else {
				// Keep existing
				newMhohs.Write(existing.data)
			}
		}

		// Append new mhoh chunks for types that didn't exist
		// Order: location first, then metadata
		appendOrder := []uint32{0x0D, 0x02, 0x03, 0x04, 0x05, 0x06, 0x0C}
		for _, ht := range appendOrder {
			if val, ok := replacements[ht]; ok && !replaced[ht] {
				newMhohs.Write(buildMhohLE(ht, val))
			}
		}

		// Update mith totalLen to include new mhoh data
		newTotalLen := mithHeaderLen + newMhohs.Len()
		writeUint32LE(mithHeader, 8, uint32(newTotalLen))

		result.Write(mithHeader)
		result.Write(newMhohs.Bytes())
		updatedCount++

		offset += mithLen
	}

	// Write everything after the track section
	result.Write(data[contentEnd:])

	// Update msdh totalLen
	out := result.Bytes()
	newContentLen := result.Len() - contentStart - (len(data) - contentEnd)
	newMsdhTotal := msdhHeaderLen + newContentLen
	writeUint32LE(out, msdhOffset+8, uint32(newMsdhTotal))

	return out, updatedCount
}
