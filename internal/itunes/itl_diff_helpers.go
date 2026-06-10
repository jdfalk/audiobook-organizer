// file: internal/itunes/itl_diff_helpers.go
// version: 1.0.0
// guid: d1e2f3a4-b5c6-7d8e-9f0a-1b2c3d4e5f6a
//
// Testable helpers for itl-diff: msdh container inventory and playlist
// membership diff. Keeping the diffable walks in this package (rather than
// in cmd/itl-diff) makes them unit-testable against generated fixtures.
//
// Used by: cmd/itl-diff (inventory + membership sections), cmd/itl-check
// (indirectly via AuditITL which operates on the same decrypted payload).

package itunes

import (
	"encoding/hex"
	"sort"
)

// ---------------------------------------------------------------------------
// msdh container inventory
// ---------------------------------------------------------------------------

// MsdhEntry describes one top-level msdh container in a decrypted ITL payload.
type MsdhEntry struct {
	// BlockType is the value at msdh+12 (LE uint32). Well-known values:
	//   1 = master track list, 2 = playlist list, 9 = album list, 11 = artist list.
	BlockType int
	// HeaderLen is the fixed header portion length (msdh+4, LE uint32).
	HeaderLen int
	// TotalLen is the full container length including all children (msdh+8, LE uint32).
	TotalLen int
}

// BlockTypeName returns a human-readable name for known msdh block types.
func (e MsdhEntry) BlockTypeName() string {
	switch e.BlockType {
	case 1:
		return "track-list"
	case 2:
		return "playlist-list"
	case 9:
		return "album-list"
	case 11:
		return "artist-list"
	default:
		return "unknown"
	}
}

// CollectMsdhInventory walks all top-level msdh containers in a decrypted LE
// ITL payload and returns an ordered slice of MsdhEntry (one per container).
// The payload must be the result of DecryptAndInflateITL or equivalent; it
// starts with the first "msdh" tag.
//
// Returns an empty slice (not nil) if the payload is not LE format.
func CollectMsdhInventory(payload []byte) []MsdhEntry {
	if !detectLE(payload) {
		return []MsdhEntry{}
	}
	var entries []MsdhEntry
	offset := 0
	for offset+16 <= len(payload) {
		tag := readTag(payload, offset)
		if tag != "msdh" {
			break
		}
		hdrLen := int(readUint32LE(payload, offset+4))
		totalLen := int(readUint32LE(payload, offset+8))
		blockType := int(readUint32LE(payload, offset+12))
		if totalLen < 16 || offset+totalLen > len(payload) {
			break
		}
		entries = append(entries, MsdhEntry{
			BlockType: blockType,
			HeaderLen: hdrLen,
			TotalLen:  totalLen,
		})
		offset += totalLen
	}
	return entries
}

// MsdhInventoryDiff describes the difference in msdh inventories between two
// ITL payloads. For containers present in both, the entry records the before/after
// sizes; containers present in only one side are listed in OnlyA or OnlyB.
type MsdhInventoryDiff struct {
	// Changed lists containers present in both A and B but with different sizes.
	Changed []MsdhInventoryChange
	// OnlyA lists block types present only in A.
	OnlyA []MsdhEntry
	// OnlyB lists block types present only in B.
	OnlyB []MsdhEntry
}

// MsdhInventoryChange records a container whose sizes changed between A and B.
type MsdhInventoryChange struct {
	BlockType int
	A         MsdhEntry
	B         MsdhEntry
}

// DiffMsdhInventory computes the inventory diff between two decrypted ITL payloads.
func DiffMsdhInventory(payloadA, payloadB []byte) MsdhInventoryDiff {
	invA := CollectMsdhInventory(payloadA)
	invB := CollectMsdhInventory(payloadB)

	indexA := make(map[int]MsdhEntry, len(invA))
	for _, e := range invA {
		indexA[e.BlockType] = e
	}
	indexB := make(map[int]MsdhEntry, len(invB))
	for _, e := range invB {
		indexB[e.BlockType] = e
	}

	var diff MsdhInventoryDiff
	for _, ea := range invA {
		eb, ok := indexB[ea.BlockType]
		if !ok {
			diff.OnlyA = append(diff.OnlyA, ea)
			continue
		}
		if ea.HeaderLen != eb.HeaderLen || ea.TotalLen != eb.TotalLen {
			diff.Changed = append(diff.Changed, MsdhInventoryChange{
				BlockType: ea.BlockType, A: ea, B: eb,
			})
		}
	}
	for _, eb := range invB {
		if _, ok := indexA[eb.BlockType]; !ok {
			diff.OnlyB = append(diff.OnlyB, eb)
		}
	}

	// Sort for deterministic output.
	sort.Slice(diff.Changed, func(i, j int) bool {
		return diff.Changed[i].BlockType < diff.Changed[j].BlockType
	})
	sort.Slice(diff.OnlyA, func(i, j int) bool {
		return diff.OnlyA[i].BlockType < diff.OnlyA[j].BlockType
	})
	sort.Slice(diff.OnlyB, func(i, j int) bool {
		return diff.OnlyB[i].BlockType < diff.OnlyB[j].BlockType
	})
	return diff
}

// ---------------------------------------------------------------------------
// Playlist membership diff
// ---------------------------------------------------------------------------

// PlaylistMembership records the track IDs (as TrackIDs, integer) in a playlist
// identified by its PID (hex-encoded, BE/MSB-first, matching XML format).
type PlaylistMembership struct {
	PID   string // hex-encoded 8-byte playlist persistent ID
	Title string // decoded title mhoh (may be empty if not present)
	TIDs  []int  // ordered track IDs in playlist order
}

// PlaylistMembershipDiff describes what changed for a single playlist.
type PlaylistMembershipDiff struct {
	PID     string
	Title   string
	Added   []int // TIDs in B but not A
	Removed []int // TIDs in A but not B
}

// MembershipDiffResult is the full result of DiffPlaylistMembership.
type MembershipDiffResult struct {
	// Changed lists playlists present in both A and B with different membership.
	Changed []PlaylistMembershipDiff
	// OnlyA lists playlists present only in A (by PID).
	OnlyA []PlaylistMembership
	// OnlyB lists playlists present only in B (by PID).
	OnlyB []PlaylistMembership
}

// CollectPlaylistMemberships extracts playlist membership from a parsed library.
func CollectPlaylistMemberships(lib *ITLLibrary) []PlaylistMembership {
	out := make([]PlaylistMembership, 0, len(lib.Playlists))
	for _, p := range lib.Playlists {
		pid := hex.EncodeToString(p.PersistentID[:])
		pm := PlaylistMembership{
			PID:   pid,
			Title: p.Title,
			TIDs:  make([]int, len(p.Items)),
		}
		copy(pm.TIDs, p.Items)
		out = append(out, pm)
	}
	return out
}

// DiffPlaylistMembership computes playlist membership differences between two
// parsed ITL libraries. Two playlists are matched by persistent ID.
func DiffPlaylistMembership(libA, libB *ITLLibrary) MembershipDiffResult {
	msA := CollectPlaylistMemberships(libA)
	msB := CollectPlaylistMemberships(libB)

	indexA := make(map[string]PlaylistMembership, len(msA))
	for _, m := range msA {
		indexA[m.PID] = m
	}
	indexB := make(map[string]PlaylistMembership, len(msB))
	for _, m := range msB {
		indexB[m.PID] = m
	}

	var result MembershipDiffResult

	for _, ma := range msA {
		mb, ok := indexB[ma.PID]
		if !ok {
			result.OnlyA = append(result.OnlyA, ma)
			continue
		}
		diff := membershipDiff(ma, mb)
		if len(diff.Added) > 0 || len(diff.Removed) > 0 {
			result.Changed = append(result.Changed, diff)
		}
	}
	for _, mb := range msB {
		if _, ok := indexA[mb.PID]; !ok {
			result.OnlyB = append(result.OnlyB, mb)
		}
	}

	// Sort for deterministic output.
	sort.Slice(result.Changed, func(i, j int) bool {
		return result.Changed[i].PID < result.Changed[j].PID
	})
	sort.Slice(result.OnlyA, func(i, j int) bool { return result.OnlyA[i].PID < result.OnlyA[j].PID })
	sort.Slice(result.OnlyB, func(i, j int) bool { return result.OnlyB[i].PID < result.OnlyB[j].PID })
	return result
}

// membershipDiff computes added/removed TIDs between two playlists with the same PID.
func membershipDiff(a, b PlaylistMembership) PlaylistMembershipDiff {
	setA := make(map[int]struct{}, len(a.TIDs))
	for _, tid := range a.TIDs {
		setA[tid] = struct{}{}
	}
	setB := make(map[int]struct{}, len(b.TIDs))
	for _, tid := range b.TIDs {
		setB[tid] = struct{}{}
	}

	var added, removed []int
	for _, tid := range b.TIDs {
		if _, ok := setA[tid]; !ok {
			added = append(added, tid)
		}
	}
	for _, tid := range a.TIDs {
		if _, ok := setB[tid]; !ok {
			removed = append(removed, tid)
		}
	}
	sort.Ints(added)
	sort.Ints(removed)

	title := a.Title
	if title == "" {
		title = b.Title
	}
	return PlaylistMembershipDiff{PID: a.PID, Title: title, Added: added, Removed: removed}
}
