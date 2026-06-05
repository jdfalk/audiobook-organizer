// file: cmd/itl-repair/main.go
// version: 1.0.0
//
// itl-repair: surgical, non-destructive repair for ITL files corrupted by
// the May-2026 RemoveTracksByPIDLE bug. Strips orphaned `mtph` playlist
// track items whose TrackID is not present in the master track list,
// updating the enclosing `miph` playlist headers and the playlist-list
// `msdh` container so the file remains internally consistent.
//
// Usage:
//
//	itl-repair <input.itl> <output.itl>

package main

import (
	"fmt"
	"os"
	"sort"

	itunes "github.com/falkcorp/audiobook-organizer/internal/itunes"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: itl-repair <input.itl> <output.itl>")
		os.Exit(2)
	}
	in, out := os.Args[1], os.Args[2]

	raw, err := os.ReadFile(in)
	if err != nil {
		fail("read %s: %v", in, err)
	}
	lib, err := itunes.ParseITLBytes(raw)
	if err != nil {
		fail("parse %s: %v", in, err)
	}
	dec := lib.RawData()
	if !itunes.LooksLikeLE(dec) {
		fail("not an LE-format ITL payload")
	}

	master := itunes.CollectMasterTrackIDsLE(dec)
	if master == nil {
		fail("could not locate master track list (msdh blockType=1)")
	}
	fmt.Printf("master track count: %d\n", len(master))

	hits := itunes.LocateDanglingMtphLE(dec, master)
	fmt.Printf("dangling mtph items: %d (across %d miph parents)\n", len(hits), countParents(hits))
	if len(hits) == 0 {
		fmt.Println("nothing to repair — payload is already consistent")
		if err := os.WriteFile(out, raw, 0o644); err != nil {
			fail("copy: %v", err)
		}
		return
	}

	repaired := itunes.RepairITLDropDanglingMtphLE(dec, hits)

	if err := itunes.VerifyITLNoNewDanglingRefsLE(dec, repaired); err != nil {
		fail("repaired payload still has new dangling refs: %v", err)
	}
	post := itunes.FindDanglingMtphRefsLE(repaired, itunes.CollectMasterTrackIDsLE(repaired))
	fmt.Printf("dangling refs after repair: %d\n", len(post))

	if err := itunes.WriteITLBytes(in, out, repaired); err != nil {
		fail("write %s: %v", out, err)
	}
	fmt.Printf("wrote repaired ITL: %s (%d bytes decompressed)\n", out, len(repaired))

	rawOut, err := os.ReadFile(out)
	if err != nil {
		fail("read back %s: %v", out, err)
	}
	libOut, err := itunes.ParseITLBytes(rawOut)
	if err != nil {
		fail("parse repaired output: %v", err)
	}
	finalDec := libOut.RawData()
	finalDangling := itunes.FindDanglingMtphRefsLE(finalDec, itunes.CollectMasterTrackIDsLE(finalDec))
	fmt.Printf("post-write tracks=%d  remaining dangling refs=%d\n", len(itunes.CollectMasterTrackIDsLE(finalDec)), len(finalDangling))
}

func countParents(hits []itunes.MtphHitLE) int {
	seen := map[int]struct{}{}
	for _, h := range hits {
		seen[h.ParentMiphOffset] = struct{}{}
	}
	keys := make([]int, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return len(keys)
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "itl-repair: "+format+"\n", args...)
	os.Exit(1)
}
