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

// TestRemoveTracksByPIDLE_IsNoOp pins the safety behavior: production code
// must not destructively remove tracks until the full reference-cleanup is
// implemented.
func TestRemoveTracksByPIDLE_IsNoOp(t *testing.T) {
	calls := 0
	prev := logRemoveSkipped
	logRemoveSkipped = func(int) { calls++ }
	defer func() { logRemoveSkipped = prev }()

	in := []byte("payload-bytes")
	out, removed := RemoveTracksByPIDLE(in, map[string]bool{"deadbeefdeadbeef": true})
	if removed != 0 {
		t.Fatalf("RemoveTracksByPIDLE must report 0 removed, got %d", removed)
	}
	if string(out) != string(in) {
		t.Fatal("RemoveTracksByPIDLE must return input unchanged")
	}
	if calls != 1 {
		t.Fatalf("expected 1 dropped-remove warning, got %d", calls)
	}
}
