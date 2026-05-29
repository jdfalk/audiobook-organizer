// file: internal/itunes/service/strip_chapter_prefix.go
// version: 1.0.0
// guid: 4d9e2f1a-7b6c-4e5f-8a3b-2c1d4e5f6a7b

package itunesservice

import (
	"regexp"
	"strings"
)

// chapterPrefixPatterns matches a leading chapter/track marker on an iTunes
// per-chapter track Name. Order matters — most-specific first. Each pattern
// is anchored at the start of the string; the matched span is stripped.
//
// Examples this catches:
//
//	"(76/85) Tarkin: Star Wars (Unabridged)"   → "Tarkin: Star Wars (Unabridged)"
//	"(76 of 85) Tarkin: Star Wars"             → "Tarkin: Star Wars"
//	"Chapter 03 - The Storm"                   → "The Storm"
//	"Chapter 03: The Storm"                    → "The Storm"
//	"Track 12 - Foo"                           → "Foo"
//	"Part 4 - Bar"                             → "Bar"
//	"03 - Foo"                                 → "Foo"
//
// Does NOT touch titles without a leading marker (e.g. "The Hobbit").
var chapterPrefixPatterns = []*regexp.Regexp{
	// "(76 of 85)" / "(76/85)" / "(76-85)" / "(76_85)" with trailing space
	regexp.MustCompile(`^\(\s*\d{1,4}\s*(?:of|[\s_\-\/])\s*\d{1,4}\s*\)\s+`),
	// "Chapter 03 - " / "Chapter 03: " / "Chapter 03 "
	regexp.MustCompile(`(?i)^chapter[\s_\-]+\d{1,4}\s*[\-:\s]\s*`),
	// "Track 12 - " / "Track 12: "
	regexp.MustCompile(`(?i)^track[\s_\-]+\d{1,4}\s*[\-:\s]\s*`),
	// "Part 4 - " / "Part 4 of 8 - "
	regexp.MustCompile(`(?i)^part[\s_\-]+\d{1,4}(?:\s+of\s+\d{1,4})?\s*[\-:\s]\s*`),
	// Leading bare number with delimiter: "03 - " / "002. " / "1: "
	regexp.MustCompile(`^\d{1,4}\s*[\-:\.]\s+`),
}

// stripChapterPrefix removes a leading chapter/track marker from an iTunes
// per-chapter track Name, so the residue can be used as a Book.Title without
// the "(76/85)" / "Chapter 03" prefix leaking in. Idempotent; safe to call on
// titles that have no prefix.
//
// Called by buildBookFromAlbumGroup ONLY when falling back from an empty
// Album tag to track.Name — when Album is present we trust it verbatim.
//
// See docs/perf-audit-2026-05-29-g5-title-mismatch.md for the root-cause
// analysis that motivated this helper (MAYDEPLOY-G5a).
func stripChapterPrefix(title string) string {
	s := strings.TrimSpace(title)
	if s == "" {
		return s
	}
	// Apply each pattern at most once; first match wins. The patterns are
	// mutually exclusive in practice (a title can't start with both "(N/M)"
	// and "Chapter N"), so one pass is sufficient.
	for _, re := range chapterPrefixPatterns {
		if loc := re.FindStringIndex(s); loc != nil && loc[0] == 0 {
			s = strings.TrimSpace(s[loc[1]:])
			break
		}
	}
	return s
}
