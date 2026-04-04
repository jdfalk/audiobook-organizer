package main

import (
	"fmt"
	"os"
	"github.com/jdfalk/audiobook-organizer/internal/itunes"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: itl-roundtrip <input.itl> <output.itl>")
		os.Exit(1)
	}
	// Use a dummy update with a PID that won't match anything
	// This forces the full decrypt→decompress→rewrite→compress→encrypt cycle
	dummyUpdates := []itunes.ITLLocationUpdate{
		{PersistentID: "0000000000000000", NewLocation: "file://localhost/dummy"},
	}
	result, err := itunes.UpdateITLLocations(os.Args[1], os.Args[2], dummyUpdates)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	fmt.Printf("Updated: %d\n", result.UpdatedCount)
	
	orig, _ := os.Stat(os.Args[1])
	out, _ := os.Stat(os.Args[2])
	fmt.Printf("Original: %d bytes\nOutput:   %d bytes\nDiff:     %d bytes (%.1f%%)\n", 
		orig.Size(), out.Size(), orig.Size()-out.Size(),
		float64(orig.Size()-out.Size())/float64(orig.Size())*100)
}
