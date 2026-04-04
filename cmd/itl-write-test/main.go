package main

import (
	"fmt"
	"os"
	"github.com/jdfalk/audiobook-organizer/internal/itunes"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: itl-write-test <input.itl> <output.itl>")
		os.Exit(1)
	}
	
	// First verify we can read it
	lib, err := itunes.ParseITL(os.Args[1])
	if err != nil {
		fmt.Printf("Parse error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Before: %d tracks, %d playlists\n", len(lib.Tracks), len(lib.Playlists))
	
	// Write one location update using a fake PID that won't match
	// This tests the full round-trip without actually changing data
	updates := []itunes.ITLLocationUpdate{
		{
			PersistentID: "AABBCCDDEEFF1122",
			NewLocation:  "file://localhost/W:/audiobook-organizer/Test Author/Test Book/Test Book.m4b",
		},
	}
	
	result, err := itunes.UpdateITLLocations(os.Args[1], os.Args[2], updates)
	if err != nil {
		fmt.Printf("Write error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Updated %d tracks\n", result.UpdatedCount)
	
	// Verify we can re-read the output
	lib2, err := itunes.ParseITL(os.Args[2])
	if err != nil {
		fmt.Printf("Re-parse error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("After: %d tracks, %d playlists\n", len(lib2.Tracks), len(lib2.Playlists))
	
	orig, _ := os.Stat(os.Args[1])
	out, _ := os.Stat(os.Args[2])
	fmt.Printf("Size: %d → %d (diff: %d)\n", orig.Size(), out.Size(), out.Size()-orig.Size())
}
