// file: internal/itunes/itl_le_verify_test.go
// version: 1.0.0
// guid: 9e0f1a2b-3c4d-5e6f-7a8b-9c0d1e2f3a4b

package itunes

import (
	"os"
	"strings"
	"testing"
)

// TestVerifyDanglingRefs_RealCorruption asserts the verifier detects the
// 2026-05-02 corruption (TrackID 129719 dangling) when the production damaged
// library is available locally. Skipped in CI where the file isn't present.
func TestVerifyDanglingRefs_RealCorruption(t *testing.T) {
	const lastGood = "/tmp/last-good.itl"
	const damaged = "/tmp/damaged.itl"
	if _, err := os.Stat(damaged); err != nil {
		t.Skip("damaged ITL fixture not present locally")
	}
	if _, err := os.Stat(lastGood); err != nil {
		t.Skip("last-good ITL fixture not present locally")
	}

	lgRaw, err := os.ReadFile(lastGood)
	if err != nil {
		t.Fatal(err)
	}
	dmRaw, err := os.ReadFile(damaged)
	if err != nil {
		t.Fatal(err)
	}
	lgLib, err := ParseITLBytes(lgRaw)
	if err != nil {
		t.Fatal(err)
	}
	dmLib, err := ParseITLBytes(dmRaw)
	if err != nil {
		t.Fatal(err)
	}
	lgDec := lgLib.RawData()
	dmDec := dmLib.RawData()

	// last-good should have at least 1 pre-existing orphan that iTunes tolerated
	tids := CollectMasterTrackIDsLE(lgDec)
	if len(tids) == 0 {
		t.Fatal("could not collect master TIDs from last-good")
	}
	preExist := FindDanglingMtphRefsLE(lgDec, tids)
	if len(preExist) == 0 {
		t.Log("note: last-good has no orphan mtph refs (test environment may have changed)")
	}

	// VerifyITLNoNewDanglingRefsLE(lastGood, damaged) MUST fail because
	// damaged introduced TID 129719 as a new orphan.
	err = VerifyITLNoNewDanglingRefsLE(lgDec, dmDec)
	if err == nil {
		t.Fatal("verifier did not detect introduced dangling refs in damaged ITL")
	}
	if !strings.Contains(err.Error(), "129719") {
		t.Fatalf("verifier error did not name the expected TID 129719: %v", err)
	}

	// And conversely, last-good against itself must NOT fail.
	if err := VerifyITLNoNewDanglingRefsLE(lgDec, lgDec); err != nil {
		t.Fatalf("verifier flagged baseline as introducing new orphans: %v", err)
	}
}

// TestRemoveTracksByPIDLE_SafePath verifies the v1.2.0 safe removal:
// removing one real track from last-good produces a payload with no NEW
// dangling refs (pre-existing orphans tolerated by iTunes are preserved).
func TestRemoveTracksByPIDLE_SafePath(t *testing.T) {
	const lastGood = "/tmp/last-good.itl"
	if _, err := os.Stat(lastGood); err != nil {
		t.Skip("last-good ITL fixture not present locally")
	}
	raw, err := os.ReadFile(lastGood)
	if err != nil {
		t.Fatal(err)
	}
	lib, err := ParseITLBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	dec := lib.RawData()

	// Pick the first track's PID by scanning master mith chunks
	pid := firstMithPID(dec)
	if pid == "" {
		t.Skip("could not locate a real PID to remove")
	}
	beforeMaster := CollectMasterTrackIDsLE(dec)

	out, removed := RemoveTracksByPIDLE(dec, map[string]bool{pid: true})
	if removed != 1 {
		t.Fatalf("expected 1 removal, got %d", removed)
	}
	afterMaster := CollectMasterTrackIDsLE(out)
	if len(afterMaster) != len(beforeMaster)-1 {
		t.Fatalf("master count: before=%d after=%d (want %d)", len(beforeMaster), len(afterMaster), len(beforeMaster)-1)
	}
	if err := VerifyITLNoNewDanglingRefsLE(dec, out); err != nil {
		t.Fatalf("safe removal introduced new dangling refs: %v", err)
	}
}

// firstMithPID returns the PID of the first mith chunk in the master track
// list, or "" if the structure can't be walked.
func firstMithPID(data []byte) string {
	msdhOffset, msdhHeaderLen, _ := findMsdhByType(data, 1)
	if msdhOffset < 0 {
		return ""
	}
	off := msdhOffset + msdhHeaderLen
	if off+12 > len(data) {
		return ""
	}
	if readTag(data, off) == "mlth" {
		off += int(readUint32LE(data, off+4))
	}
	for off+12 < len(data) {
		t := readTag(data, off)
		if t == "" {
			return ""
		}
		hdr := int(readUint32LE(data, off+4))
		tot := int(readUint32LE(data, off+8))
		size := hdr
		if (t == "mith" || t == "mhoh" || t == "miah") && tot > hdr {
			size = tot
		}
		if t == "mith" && off+136 <= len(data) {
			return extractMithPIDLE(data, off)
		}
		if size < 8 {
			return ""
		}
		off += size
	}
	return ""
}
