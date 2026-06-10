// file: cmd/itl-check/main.go
// version: 2.0.0
// guid: 3a4b5c6d-7e8f-9a0b-1c2d-3e4f5a6b7c8d
//
// itl-check — audit an iTunes Library .itl file against the ITLSafetyContract.
//
// Usage:
//
//	itl-check <library.itl>
//
// Reads the library, runs AuditITL (all eight named guards from T003), and
// prints a per-guard verdict table plus per-guard violation counts.
//
// Exit codes:
//
//	0 — all guards passed
//	1 — one or more guards reported violations
//	2 — usage error or file could not be read/parsed

package main

import (
	"fmt"
	"os"

	"github.com/falkcorp/audiobook-organizer/internal/itunes"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "Usage: itl-check <library.itl>")
		os.Exit(2)
	}
	path := os.Args[1]

	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", path, err)
		os.Exit(2)
	}

	verdict := itunes.AuditITL(data)

	// Print summary header.
	fmt.Printf("itl-check: %s\n", path)
	lib, parseErr := itunes.ParseITL(path)
	if parseErr == nil {
		fmt.Printf("Tracks: %d  Playlists: %d  Version: %s\n",
			len(lib.Tracks), len(lib.Playlists), lib.Version)
		locs := 0
		for _, t := range lib.Tracks {
			if t.Location != "" {
				locs++
			}
		}
		fmt.Printf("Tracks with Location: %d\n", locs)
	}
	fmt.Println()

	// Per-guard table.
	fmt.Printf("%-24s  %s\n", "Guard", "Result")
	fmt.Printf("%-24s  %s\n", "------------------------", "------")
	for _, r := range verdict.Results {
		status := "PASS"
		detail := ""
		if !r.Pass() {
			status = fmt.Sprintf("FAIL (%d violation(s))", len(r.Violations))
			// Show the first violation message as a hint.
			if len(r.Violations) > 0 {
				v := r.Violations[0]
				detail = fmt.Sprintf("  -> [%s@%d] %s", v.Chunk, v.Offset, v.Message)
				if len(r.Violations) > 1 {
					detail += fmt.Sprintf(" (+%d more)", len(r.Violations)-1)
				}
			}
		}
		fmt.Printf("%-24s  %s%s\n", r.Guard, status, detail)
	}
	fmt.Println()

	if verdict.Pass {
		fmt.Println("Verdict: PASS — no violations found")
		os.Exit(0)
	}

	failed := verdict.FailedGuards()
	fmt.Printf("Verdict: FAIL — %d guard(s) violated: %v\n", len(failed), failed)
	os.Exit(1)
}
