package main

import (
	"fmt"
	"os"
	"github.com/jdfalk/audiobook-organizer/internal/itunes"
)

func main() {
	lib, err := itunes.ParseITL(os.Args[1])
	if err != nil { fmt.Println("Error:", err); os.Exit(1) }
	fmt.Printf("Tracks: %d, Playlists: %d, Version: %s\n", len(lib.Tracks), len(lib.Playlists), lib.Version)
	locs := 0
	for _, t := range lib.Tracks {
		if t.Location != "" { locs++ }
	}
	fmt.Printf("Tracks with Location: %d\n", locs)
}
