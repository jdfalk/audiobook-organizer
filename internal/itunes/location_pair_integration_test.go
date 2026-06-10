// file: internal/itunes/location_pair_integration_test.go
// version: 1.0.0
// guid: 2f7a9c14-8b30-4e62-a1d5-6c0e3b94d287

// Integration tests for the LocationPair writer wiring (fable5 TASK-006, CRIT-2).
//
// These exercise the FULL writer paths (UpdateMetadataLE replace/append and the
// rewriteChunksLE location-update path) against a generated fixture, then re-run
// the T003 ITLSafetyContract location-form guard on the output. This is the
// acceptance criterion: every writer output round-trips the guard, 0x0D holds a
// backslash Windows path, and a URL fed as a location is normalized (never
// written raw into 0x0D).
//
// The fxTrack/buildPayloadFromTracks/winPathToLocalURL/RunSafetyContract helpers
// live in itl_safety_contract_test.go (same package).

package itunes

import (
	"strings"
	"testing"
)

// fixturePID mirrors the fixture's PID encoding (buildMith): the persistent ID is
// BigEndian(tid | 0x1000000000000000), hex-encoded lowercase — the exact string
// both the metadata-update and location-rewrite paths extract.
func fixturePID(tid uint32) string {
	v := uint64(tid) | 0x1000000000000000
	const hexdigits = "0123456789abcdef"
	var b [16]byte
	for i := 15; i >= 0; i-- {
		b[i] = hexdigits[v&0xF]
		v >>= 4
	}
	return string(b[:])
}

// runLocationFormGuard returns the location-form GuardResult for a payload.
func runLocationFormGuard(t *testing.T, payload []byte) GuardResult {
	t.Helper()
	hdr := buildHeaderFor(payload)
	v := RunSafetyContract(payload, payload, hdr, defCfg())
	for _, r := range v.Results {
		if r.Guard == "location-form" {
			return r
		}
	}
	t.Fatal("location-form guard did not run")
	return GuardResult{}
}

// TestIntegration_UpdateMetadataLocation_PassesGuard: a metadata update that
// changes a track's Location writes BOTH 0x0D (WinPath) and 0x0B (URL) from one
// LocationPair, so the output round-trips the location-form guard.
func TestIntegration_UpdateMetadataLocation_PassesGuard(t *testing.T) {
	clean := buildPayloadFromTracks([]fxTrack{
		{tid: 10, name: "Chapter One", location: `W:\itunes\Media\Old\01.mp3`},
		{tid: 20, name: "Chapter Two", location: `W:\itunes\Media\Old\02.mp3`},
	})

	newWin := `W:\itunes\Media\Audiobooks\New Author\01 New Title - 1.mp3`
	after, n := UpdateMetadataLE(clean, []ITLMetadataUpdate{
		{PersistentID: fixturePID(10), Location: newWin},
	})
	if n != 1 {
		t.Fatalf("expected 1 track updated, got %d", n)
	}

	// Guard must pass: 0x0D = backslash path, sibling 0x0B round-trips.
	if res := runLocationFormGuard(t, after); !res.Pass() {
		t.Fatalf("location-form guard failed after metadata update: %+v", res.Violations)
	}

	// 0x0D must contain the backslash path, and NOT a URL.
	loc0D := firstTrackLocation(t, after, fixturePID(10), 0x0D)
	if loc0D != newWin {
		t.Errorf("0x0D = %q, want %q", loc0D, newWin)
	}
	if strings.Contains(loc0D, "file://") || strings.Contains(loc0D, "/") {
		t.Errorf("0x0D must be a plain backslash path, got %q", loc0D)
	}
	// 0x0B must be the percent-escaped URL derived from the same path.
	loc0B := firstTrackLocation(t, after, fixturePID(10), 0x0B)
	if want := winPathToLocalURL(newWin); loc0B != want {
		t.Errorf("0x0B = %q, want %q", loc0B, want)
	}
}

// TestIntegration_UpdateMetadataLocation_URLIn0x0D_Regression: feeding a URL as
// the Location must be NORMALIZED — 0x0D ends up a backslash path, never the raw
// URL. This is the direct CRIT-2 regression.
func TestIntegration_UpdateMetadataLocation_URLIn0x0D_Regression(t *testing.T) {
	clean := buildPayloadFromTracks([]fxTrack{
		{tid: 10, name: "Chapter One", location: `W:\itunes\Media\Old\01.mp3`},
	})

	// Caller (wrongly) passes a URL-shaped value, as f.ITunesPath historically did.
	urlVal := "file://localhost/W:/itunes/Media/Fixed/01%20Track.mp3"
	after, n := UpdateMetadataLE(clean, []ITLMetadataUpdate{
		{PersistentID: fixturePID(10), Location: urlVal},
	})
	if n != 1 {
		t.Fatalf("expected 1 track updated, got %d", n)
	}

	loc0D := firstTrackLocation(t, after, fixturePID(10), 0x0D)
	if strings.Contains(loc0D, "file://") {
		t.Fatalf("CRIT-2 regression: URL written raw into 0x0D: %q", loc0D)
	}
	if loc0D != `W:\itunes\Media\Fixed\01 Track.mp3` {
		t.Errorf("0x0D not normalized to backslash path: %q", loc0D)
	}
	if res := runLocationFormGuard(t, after); !res.Pass() {
		t.Fatalf("location-form guard failed: %+v", res.Violations)
	}
}

// TestIntegration_UpdateMetadataLocation_Unmappable_SkipsRaw: an unmappable
// location (staging-dir leak) is skipped — neither 0x0D nor 0x0B is rewritten
// with the bad value, and the original (clean) blocks are preserved so the guard
// still passes.
func TestIntegration_UpdateMetadataLocation_Unmappable_SkipsRaw(t *testing.T) {
	orig := `W:\itunes\Media\Old\01.mp3`
	clean := buildPayloadFromTracks([]fxTrack{
		{tid: 10, name: "Chapter One", location: orig},
	})

	bad := `W:\audiobook-organizer\.itunes-writeback\iTunes Media\01.mp3`
	after, _ := UpdateMetadataLE(clean, []ITLMetadataUpdate{
		{PersistentID: fixturePID(10), Location: bad},
	})

	loc0D := firstTrackLocation(t, after, fixturePID(10), 0x0D)
	if strings.Contains(loc0D, ".itunes-writeback") {
		t.Fatalf("staging-dir leak written into 0x0D: %q", loc0D)
	}
	if loc0D != orig {
		t.Errorf("0x0D should be unchanged (original preserved), got %q", loc0D)
	}
	if res := runLocationFormGuard(t, after); !res.Pass() {
		t.Fatalf("location-form guard failed: %+v", res.Violations)
	}
}

// TestIntegration_RewriteChunksLE_LocationUpdate: the location-update path
// (rewriteChunksLE / shouldUpdateMhohLE) rewrites both 0x0D and 0x0B from a single
// WinPath map value and passes the guard.
func TestIntegration_RewriteChunksLE_LocationUpdate(t *testing.T) {
	clean := buildPayloadFromTracks([]fxTrack{
		{tid: 10, name: "Chapter One", location: `W:\itunes\Media\Old\01.mp3`},
		{tid: 20, name: "Chapter Two", location: `W:\itunes\Media\Old\02.mp3`},
	})

	newWin := `W:\itunes\Media\Audiobooks\Author\02 Title.mp3`
	updateMap := map[string]string{fixturePID(20): newWin}
	after, n := rewriteChunksLE(clean, updateMap)
	if n < 2 {
		// One update touches both the 0x0D and 0x0B blocks of the track.
		t.Fatalf("expected >=2 mhoh rewrites (0x0D + 0x0B), got %d", n)
	}

	if res := runLocationFormGuard(t, after); !res.Pass() {
		t.Fatalf("location-form guard failed after rewriteChunksLE: %+v", res.Violations)
	}
	loc0D := firstTrackLocation(t, after, fixturePID(20), 0x0D)
	if loc0D != newWin {
		t.Errorf("0x0D = %q, want %q", loc0D, newWin)
	}
	loc0B := firstTrackLocation(t, after, fixturePID(20), 0x0B)
	if want := winPathToLocalURL(newWin); loc0B != want {
		t.Errorf("0x0B = %q, want %q", loc0B, want)
	}
}

// TestIntegration_RewriteChunksLE_URLValue_Normalized: a URL-shaped map value
// (the historical f.ITunesPath shape) is normalized — 0x0D gets a backslash path.
func TestIntegration_RewriteChunksLE_URLValue_Normalized(t *testing.T) {
	clean := buildPayloadFromTracks([]fxTrack{
		{tid: 10, name: "Chapter One", location: `W:\itunes\Media\Old\01.mp3`},
	})
	url := "file://localhost/W:/itunes/Media/New/01%20Fixed.mp3"
	after, _ := rewriteChunksLE(clean, map[string]string{fixturePID(10): url})

	loc0D := firstTrackLocation(t, after, fixturePID(10), 0x0D)
	if strings.Contains(loc0D, "file://") {
		t.Fatalf("CRIT-2 regression in rewriteChunksLE: URL in 0x0D: %q", loc0D)
	}
	if loc0D != `W:\itunes\Media\New\01 Fixed.mp3` {
		t.Errorf("0x0D not normalized: %q", loc0D)
	}
	if res := runLocationFormGuard(t, after); !res.Pass() {
		t.Fatalf("location-form guard failed: %+v", res.Violations)
	}
}

// TestIntegration_NonASCIILocation_ThroughEncoderPassesGuard: a non-ASCII WinPath
// is written by the REAL writer (UpdateMetadataLE → encodeMhohITunes, which emits
// UTF-16LE for the 0x0D path per T005) and must still pass the T003 location-form
// guard, with the decoded 0x0D equal to the input and the 0x0B sibling equal to
// the percent-escaped URL. This is the cross-task interaction the task calls out:
// T005 UTF-16LE encode → T006 pair → T003 guard byte comparison. The fixture is
// built ASCII and mutated by the real writer (NOT hand-built) so the encoding is
// exactly what production would emit.
func TestIntegration_NonASCIILocation_ThroughEncoderPassesGuard(t *testing.T) {
	clean := buildPayloadFromTracks([]fxTrack{
		{tid: 10, name: "Chapter One", location: `W:\itunes\Media\Old\01.mp3`},
	})

	nonASCII := `W:\itunes\Media\日本語\01 章 - 1.mp3`

	// Replace path: UpdateMetadataLE.
	after, n := UpdateMetadataLE(clean, []ITLMetadataUpdate{
		{PersistentID: fixturePID(10), Location: nonASCII},
	})
	if n != 1 {
		t.Fatalf("expected 1 track updated, got %d", n)
	}
	if res := runLocationFormGuard(t, after); !res.Pass() {
		t.Fatalf("location-form guard failed for non-ASCII path: %+v", res.Violations)
	}
	if loc0D := firstTrackLocation(t, after, fixturePID(10), 0x0D); loc0D != nonASCII {
		t.Errorf("non-ASCII 0x0D decoded wrong: got %q want %q", loc0D, nonASCII)
	}
	if loc0B := firstTrackLocation(t, after, fixturePID(10), 0x0B); loc0B != winPathToLocalURL(nonASCII) {
		t.Errorf("non-ASCII 0x0B = %q, want %q", loc0B, winPathToLocalURL(nonASCII))
	}

	// Rewrite path: rewriteChunksLE on a fresh fixture.
	clean2 := buildPayloadFromTracks([]fxTrack{
		{tid: 10, name: "Chapter One", location: `W:\itunes\Media\Old\01.mp3`},
	})
	after2, _ := rewriteChunksLE(clean2, map[string]string{fixturePID(10): nonASCII})
	if res := runLocationFormGuard(t, after2); !res.Pass() {
		t.Fatalf("location-form guard failed for non-ASCII via rewriteChunksLE: %+v", res.Violations)
	}
	if loc0D := firstTrackLocation(t, after2, fixturePID(10), 0x0D); loc0D != nonASCII {
		t.Errorf("rewriteChunksLE non-ASCII 0x0D wrong: got %q want %q", loc0D, nonASCII)
	}
}

// firstTrackLocation decodes the string of the first mhoh of hohmType under the
// mith whose PID == wantPID. Returns "" if not found.
func firstTrackLocation(t *testing.T, payload []byte, wantPID string, hohmType uint32) string {
	t.Helper()
	got := ""
	found := false

	// Manual mith walk to match by PID and decode the requested hohm type.
	msdhOff, hdrLen, totalLen := findMsdhByType(payload, 1)
	if msdhOff < 0 {
		return ""
	}
	off := msdhOff + hdrLen
	end := msdhOff + totalLen
	for off+8 <= end && !found {
		tag := readTag(payload, off)
		if tag != "mith" {
			span := int(readUint32LE(payload, off+4))
			if tt := int(readUint32LE(payload, off+8)); tt > span && off+tt <= end {
				span = tt
			}
			if span < 8 {
				break
			}
			off += span
			continue
		}
		mithHdr := int(readUint32LE(payload, off+4))
		mithTotal := int(readUint32LE(payload, off+8))
		span := mithHdr
		if mithTotal > mithHdr && off+mithTotal <= end {
			span = mithTotal
		}
		pid := extractMithPIDLE(payload, off)
		if pid == strings.ToLower(wantPID) {
			child := off + mithHdr
			for child+8 <= off+span {
				if readTag(payload, child) != "mhoh" {
					break
				}
				cspan := int(readUint32LE(payload, child+8))
				if cspan < 40 || child+cspan > off+span {
					break
				}
				if readUint32LE(payload, child+12) == hohmType {
					s, _ := decodeMhohBlock(payload[child : child+cspan])
					got = s
					found = true
					break
				}
				child += cspan
			}
		}
		off += span
	}
	return got
}
