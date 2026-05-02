// file: cmd/itl-diff/main.go
// version: 1.0.0
// guid: 9d0e1f2a-3b4c-5d6e-7f8a-9b0c1d2e3f4a
//
// Structural diff between two iTunes Library.itl files. Reports:
//   - hdfm header comparison (version, header length, file length, max_crypt_size,
//     full byte-diff of the version "remainder" header trailing bytes)
//   - msdh container inventory (block-type → header-len, total-len)
//   - mlth track count
//   - per-track metadata diff (by Persistent ID): which mhoh fields changed,
//     which fields are present in A but missing in B (and vice versa)
//
// Usage: itl-diff <a.itl> <b.itl>

package main

import (
	"bytes"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/jdfalk/audiobook-organizer/internal/itunes"
)

func main() {
	verbose := flag.Bool("v", false, "Verbose: list per-track diffs")
	max := flag.Int("max", 20, "Max per-track diffs to print in verbose mode")
	flag.Parse()
	if flag.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "Usage: itl-diff [-v] [-max N] <a.itl> <b.itl>")
		os.Exit(2)
	}

	a, err := itunes.ParseITL(flag.Arg(0))
	must(err, "parse A")
	b, err := itunes.ParseITL(flag.Arg(1))
	must(err, "parse B")

	fmt.Printf("A: %s\nB: %s\n\n", flag.Arg(0), flag.Arg(1))

	// File bytes for low-level comparison
	rawA, _ := os.ReadFile(flag.Arg(0))
	rawB, _ := os.ReadFile(flag.Arg(1))
	fmt.Printf("File size:        A=%d  B=%d  (Δ=%+d)\n",
		len(rawA), len(rawB), len(rawB)-len(rawA))
	fmt.Printf("ITL version:      A=%q B=%q\n", a.Version, b.Version)
	fmt.Printf("Track count:      A=%d B=%d (Δ=%+d)\n",
		len(a.Tracks), len(b.Tracks), len(b.Tracks)-len(a.Tracks))
	fmt.Printf("Playlist count:   A=%d B=%d (Δ=%+d)\n\n",
		len(a.Playlists), len(b.Playlists), len(b.Playlists)-len(a.Playlists))

	// hdfm header comparison
	if len(rawA) >= 96 && len(rawB) >= 96 {
		fmt.Println("hdfm header (first 96 bytes):")
		printHexDiff(rawA[:96], rawB[:96])
		fmt.Println()
	}

	// Per-track diff by PID
	indexA := map[string]itunes.ITLTrack{}
	for _, t := range a.Tracks {
		indexA[hex.EncodeToString(t.PersistentID[:])] = t
	}
	indexB := map[string]itunes.ITLTrack{}
	for _, t := range b.Tracks {
		indexB[hex.EncodeToString(t.PersistentID[:])] = t
	}

	var onlyA, onlyB, changed []string
	for pid, ta := range indexA {
		tb, ok := indexB[pid]
		if !ok {
			onlyA = append(onlyA, pid)
			continue
		}
		if !tracksEqual(ta, tb) {
			changed = append(changed, pid)
		}
	}
	for pid := range indexB {
		if _, ok := indexA[pid]; !ok {
			onlyB = append(onlyB, pid)
		}
	}
	sort.Strings(onlyA)
	sort.Strings(onlyB)
	sort.Strings(changed)

	fmt.Printf("Tracks only in A:  %d\n", len(onlyA))
	fmt.Printf("Tracks only in B:  %d\n", len(onlyB))
	fmt.Printf("Tracks changed:    %d\n", len(changed))

	if *verbose {
		printList("only-in-A", onlyA, indexA, *max)
		printList("only-in-B", onlyB, indexB, *max)
		fmt.Println()
		fmt.Printf("Per-track diffs (first %d):\n", *max)
		for i, pid := range changed {
			if i >= *max {
				fmt.Printf("...and %d more\n", len(changed)-*max)
				break
			}
			ta := indexA[pid]
			tb := indexB[pid]
			fmt.Printf("\n  PID %s:\n", pid)
			diffField("Name", ta.Name, tb.Name)
			diffField("Album", ta.Album, tb.Album)
			diffField("Artist", ta.Artist, tb.Artist)
			diffField("Genre", ta.Genre, tb.Genre)
			diffField("Kind", ta.Kind, tb.Kind)
			diffField("Location", ta.Location, tb.Location)
			diffInt("Size", ta.Size, tb.Size)
			diffInt("TotalTime", ta.TotalTime, tb.TotalTime)
			diffInt("BitRate", ta.BitRate, tb.BitRate)
			diffInt("SampleRate", ta.SampleRate, tb.SampleRate)
			diffInt("PlayCount", ta.PlayCount, tb.PlayCount)
			diffInt("Rating", ta.Rating, tb.Rating)
			diffInt("Year", ta.Year, tb.Year)
		}
	}
}

func tracksEqual(a, b itunes.ITLTrack) bool {
	return a.Name == b.Name && a.Album == b.Album && a.Artist == b.Artist &&
		a.Genre == b.Genre && a.Kind == b.Kind && a.Location == b.Location &&
		a.Size == b.Size && a.TotalTime == b.TotalTime &&
		a.BitRate == b.BitRate && a.SampleRate == b.SampleRate &&
		a.PlayCount == b.PlayCount && a.Rating == b.Rating && a.Year == b.Year
}

func diffField(name, a, b string) {
	if a != b {
		fmt.Printf("    %-12s A=%q B=%q\n", name, a, b)
	}
}

func diffInt(name string, a, b int) {
	if a != b {
		fmt.Printf("    %-12s A=%d B=%d\n", name, a, b)
	}
}

func printList(label string, pids []string, idx map[string]itunes.ITLTrack, max int) {
	if len(pids) == 0 {
		return
	}
	fmt.Printf("\n%s (showing up to %d):\n", label, max)
	for i, pid := range pids {
		if i >= max {
			fmt.Printf("  ...and %d more\n", len(pids)-max)
			break
		}
		t := idx[pid]
		fmt.Printf("  %s  %q  by %q\n", pid, t.Name, t.Artist)
	}
}

func printHexDiff(a, b []byte) {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i += 16 {
		end := i + 16
		if end > n {
			end = n
		}
		ra := a[i:end]
		rb := b[i:end]
		marker := " "
		if !bytes.Equal(ra, rb) {
			marker = "*"
		}
		fmt.Printf("  %s %04x  A: %-48s | B: %-48s\n", marker, i,
			hex.EncodeToString(ra), hex.EncodeToString(rb))
	}
}

func must(err error, what string) {
	if err != nil {
		fmt.Fprintln(os.Stderr, what+":", err)
		os.Exit(1)
	}
}
