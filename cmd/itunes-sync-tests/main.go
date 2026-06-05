// file: cmd/itunes-sync-tests/main.go
// version: 1.0.0
// guid: 7c8d9e0f-1a2b-3c4d-5e6f-7a8b9c0d1e2f
//
// CLI that emits the iTunes/Apple Devices sync-diagnostic ITL suite.
// See internal/itunes/sync_diagnostic_tests.go for the variant catalog.
//
// Usage:
//
//	go run ./cmd/itunes-sync-tests \
//	    -baseline "/Users/jdfalk/Downloads/itunes-investigation/iTunes Library.itl" \
//	    -out      "/Users/jdfalk/Downloads/itunes-investigation/sync-tests"

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/falkcorp/audiobook-organizer/internal/itunes"
)

func main() {
	baseline := flag.String("baseline", "", "Path to known-good iTunes Library.itl from before audiobook-organizer touched the library")
	out := flag.String("out", "", "Output directory for the test suite")
	flag.Parse()

	if *baseline == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "Usage: itunes-sync-tests -baseline <path> -out <dir>")
		os.Exit(2)
	}

	if err := itunes.GenerateSyncDiagnosticSuite(*baseline, *out); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	fmt.Println("Generated sync-diagnostic suite at", *out)
}
