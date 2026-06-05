package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/falkcorp/audiobook-organizer/internal/itunes"
)

func main() {
	root := "/Users/jdfalk/Downloads/itunes-investigation/sync-tests"
	baselineLib, err := itunes.ParseITL("/Users/jdfalk/Downloads/itunes-investigation/iTunes Library.itl")
	if err != nil {
		fmt.Println("baseline parse err:", err)
		os.Exit(1)
	}
	baseN := len(baselineLib.Tracks)
	fmt.Printf("baseline tracks=%d playlists=%d\n\n", baseN, len(baselineLib.Playlists))

	entries, _ := os.ReadDir(root)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		itlPath := filepath.Join(root, e.Name(), "iTunes Library.itl")
		infoPath := filepath.Join(root, e.Name(), "test-info.json")
		var info struct {
			ExpectedTrackDelta int `json:"expected_track_delta"`
		}
		b, _ := os.ReadFile(infoPath)
		_ = json.Unmarshal(b, &info)
		st, _ := os.Stat(itlPath)

		lib, err := itunes.ParseITL(itlPath)
		status := "ok"
		actualTracks := -1
		actualPL := -1
		if err != nil {
			status = "PARSE_ERROR: " + err.Error()
		} else {
			actualTracks = len(lib.Tracks)
			actualPL = len(lib.Playlists)
		}
		expected := baseN + info.ExpectedTrackDelta
		match := "MATCH"
		if actualTracks != expected {
			match = fmt.Sprintf("MISMATCH (expected=%d)", expected)
		}
		fmt.Printf("%-42s sz=%9d tracks=%6d pl=%4d %s %s\n",
			e.Name(), st.Size(), actualTracks, actualPL, match, status)
	}
}
