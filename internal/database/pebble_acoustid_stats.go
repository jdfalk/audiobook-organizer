// file: internal/database/pebble_acoustid_stats.go
// version: 1.0.0
// guid: aabbccdd-eeff-0011-2233-445566778899
// last-edited: 2026-06-04

package database

import "sort"

func flattenAcoustIDStats(byLib map[string]*AcoustIDStatsByLibrary) []AcoustIDStatsByLibrary {
	libs := make([]AcoustIDStatsByLibrary, 0, len(byLib))
	for _, entry := range byLib {
		libs = append(libs, *entry)
	}
	sort.Slice(libs, func(i, j int) bool {
		return libs[i].LibraryRoot < libs[j].LibraryRoot
	})
	return libs
}

func bookFileHasAcoustIDSegments(f *BookFile) bool {
	if f == nil {
		return false
	}
	return f.AcoustIDSeg0 != "" || f.AcoustIDSeg1 != "" || f.AcoustIDSeg2 != "" ||
		f.AcoustIDSeg3 != "" || f.AcoustIDSeg4 != "" || f.AcoustIDSeg5 != "" || f.AcoustIDSeg6 != ""
}
