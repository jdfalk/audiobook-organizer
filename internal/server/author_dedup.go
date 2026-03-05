// file: internal/server/author_dedup.go
// version: 1.4.0
// guid: d4e5f6a7-b8c9-0d1e-2f3a-4b5c6d7e8f90

package server

import (
	"math"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// AuthorDedupGroup represents a group of potentially duplicate authors.
type AuthorDedupGroup struct {
	Canonical database.Author   `json:"canonical"`
	Variants  []database.Author `json:"variants"`
	BookCount int               `json:"book_count"`
}

// NormalizeAuthorName normalizes whitespace around initials and trims.
// "James S. A. Corey" and "James S.A. Corey" both become "James S. A. Corey"
func NormalizeAuthorName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return name
	}

	// Normalize multiple spaces to single
	spaceRe := regexp.MustCompile(`\s+`)
	name = spaceRe.ReplaceAllString(name, " ")

	// Expand collapsed initials: "S.A." → "S. A."
	initialsRe := regexp.MustCompile(`([A-Z]\.)([A-Z])`)
	for initialsRe.MatchString(name) {
		name = initialsRe.ReplaceAllString(name, "$1 $2")
	}

	return strings.TrimSpace(name)
}

// splitAuthorParts splits "First Middle Last" into (first+middle, last).
// Handles "Last, First" format too.
func splitAuthorParts(name string) (first, last string) {
	name = strings.TrimSpace(name)

	// Handle "Last, First" format
	if idx := strings.Index(name, ","); idx > 0 {
		return strings.TrimSpace(name[idx+1:]), strings.TrimSpace(name[:idx])
	}

	parts := strings.Fields(name)
	if len(parts) == 0 {
		return "", ""
	}
	if len(parts) == 1 {
		return "", parts[0]
	}
	return strings.Join(parts[:len(parts)-1], " "), parts[len(parts)-1]
}

// extractBaseAuthor strips narrator/co-author suffixes like "Author/Narrator"
// or "Author (Narrator Name)" and returns the base author name.
func extractBaseAuthor(name string) string {
	// Strip " (anything)" parenthetical that looks like a role
	parenRe := regexp.MustCompile(`\s*\([^)]*\)\s*$`)
	name = parenRe.ReplaceAllString(name, "")

	// If name contains "/" and isn't just initials, take the first part
	if idx := strings.Index(name, "/"); idx > 0 {
		name = strings.TrimSpace(name[:idx])
	}

	return strings.TrimSpace(name)
}

// isDirtyAuthorName returns true if the name is obviously not a real author
func isDirtyAuthorName(name string) bool {
	if strings.Contains(name, " - ") {
		return true
	}

	lower := strings.ToLower(name)
	publisherSuffixes := []string{"production", "productions", "publishing", "publishers",
		"press", "studios", "studio", "media", "entertainment", "books", "audio",
		"house", "group", "company", "records", "recordings"}
	for _, suffix := range publisherSuffixes {
		if strings.HasSuffix(lower, " "+suffix) {
			return true
		}
	}

	publisherPrefixes := []string{"bbc ", "penguin ", "harpercollins", "hachette", "simon & schuster"}
	for _, prefix := range publisherPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}

	return false
}

// isCompositeAuthorName returns true if the name contains multiple real authors
func isCompositeAuthorName(name string) bool {
	if regexp.MustCompile(`(?i)\(aka\s`).MatchString(name) {
		return false
	}

	if idx := strings.Index(name, "/"); idx > 0 {
		left := strings.TrimSpace(name[:idx])
		right := strings.TrimSpace(name[idx+1:])
		if len(left) > 2 && len(right) > 2 {
			return true
		}
	}

	parts := strings.SplitN(name, ",", 2)
	if len(parts) == 2 {
		left := strings.TrimSpace(parts[0])
		right := strings.TrimSpace(parts[1])
		if strings.Contains(right, " ") && strings.Contains(left, " ") {
			return true
		}
	}

	return false
}

// areAuthorsDuplicate determines if two author names refer to the same person.
// Much stricter than raw Jaro-Winkler — requires last name match.
func areAuthorsDuplicate(name1, name2 string) bool {
	// Skip dirty names (book titles, publishers)
	if isDirtyAuthorName(name1) || isDirtyAuthorName(name2) {
		return false
	}

	norm1 := strings.ToLower(NormalizeAuthorName(name1))
	norm2 := strings.ToLower(NormalizeAuthorName(name2))

	// Exact normalized match
	if norm1 == norm2 {
		return true
	}

	// Check if one contains the other (e.g. "David Kushner" vs "David Kushner/Wil Wheaton")
	base1 := strings.ToLower(extractBaseAuthor(NormalizeAuthorName(name1)))
	base2 := strings.ToLower(extractBaseAuthor(NormalizeAuthorName(name2)))
	if base1 == base2 {
		return true
	}

	// If after base extraction they still differ, compare parts
	first1, last1 := splitAuthorParts(base1)
	first2, last2 := splitAuthorParts(base2)

	// Both must have a last name
	if last1 == "" || last2 == "" {
		return false
	}

	// Last names must be very similar (>= 0.95) or exact match
	lastSim := jaroWinklerSimilarity(last1, last2)
	if lastSim < 0.95 {
		return false
	}

	// If one has no first name, only match if last names are identical
	if first1 == "" || first2 == "" {
		return last1 == last2
	}

	// First names/initials must also be similar
	// Handle initial vs full name: "J." matches "James", "J. K." matches "Joanne K."
	if isInitialMatch(first1, first2) {
		return true
	}

	firstSim := jaroWinklerSimilarity(first1, first2)
	return firstSim >= 0.85
}

// isInitialMatch checks if one name is an initial form of the other.
// "J." matches "James", "J. K." matches "J. K.", "J.K." matches "J. K."
func isInitialMatch(a, b string) bool {
	aParts := strings.Fields(a)
	bParts := strings.Fields(b)

	// Must have same number of name parts
	if len(aParts) != len(bParts) {
		return false
	}

	for i := range aParts {
		ap := strings.TrimRight(aParts[i], ".")
		bp := strings.TrimRight(bParts[i], ".")

		// If both are single char (initials), they must match
		if len(ap) == 1 && len(bp) == 1 {
			if !strings.EqualFold(ap, bp) {
				return false
			}
			continue
		}

		// If one is an initial and the other is a full name, initial must match first letter
		if len(ap) == 1 {
			if !strings.HasPrefix(strings.ToLower(bp), strings.ToLower(ap)) {
				return false
			}
			continue
		}
		if len(bp) == 1 {
			if !strings.HasPrefix(strings.ToLower(ap), strings.ToLower(bp)) {
				return false
			}
			continue
		}

		// Both are full names — must be very similar
		if jaroWinklerSimilarity(ap, bp) < 0.92 {
			return false
		}
	}
	return true
}

// jaroWinklerSimilarity computes the Jaro-Winkler similarity between two strings.
func jaroWinklerSimilarity(s1, s2 string) float64 {
	s1 = strings.ToLower(s1)
	s2 = strings.ToLower(s2)

	if s1 == s2 {
		return 1.0
	}

	len1 := utf8.RuneCountInString(s1)
	len2 := utf8.RuneCountInString(s2)

	if len1 == 0 || len2 == 0 {
		return 0.0
	}

	r1 := []rune(s1)
	r2 := []rune(s2)

	matchDistance := int(math.Max(float64(len1), float64(len2)))/2 - 1
	if matchDistance < 0 {
		matchDistance = 0
	}

	s1Matches := make([]bool, len1)
	s2Matches := make([]bool, len2)

	matches := 0
	transpositions := 0

	for i := 0; i < len1; i++ {
		start := int(math.Max(0, float64(i-matchDistance)))
		end := int(math.Min(float64(len2-1), float64(i+matchDistance)))

		for j := start; j <= end; j++ {
			if s2Matches[j] || r1[i] != r2[j] {
				continue
			}
			s1Matches[i] = true
			s2Matches[j] = true
			matches++
			break
		}
	}

	if matches == 0 {
		return 0.0
	}

	k := 0
	for i := 0; i < len1; i++ {
		if !s1Matches[i] {
			continue
		}
		for !s2Matches[k] {
			k++
		}
		if r1[i] != r2[k] {
			transpositions++
		}
		k++
	}

	jaro := (float64(matches)/float64(len1) +
		float64(matches)/float64(len2) +
		float64(matches-transpositions/2)/float64(matches)) / 3.0

	// Winkler modification: boost for common prefix (up to 4 chars)
	prefixLen := 0
	for i := 0; i < int(math.Min(4, math.Min(float64(len1), float64(len2)))); i++ {
		if r1[i] == r2[i] {
			prefixLen++
		} else {
			break
		}
	}

	return jaro + float64(prefixLen)*0.1*(1-jaro)
}

// isMultiAuthorString returns true if the name looks like multiple authors
// (more than 2 comma-separated parts, suggesting "Author1, Author2, Author3").
func isMultiAuthorString(name string) bool {
	parts := strings.Split(name, ",")
	return len(parts) > 3
}

// authorNameScore returns a penalty score for a name. Lower = cleaner/better.
func authorNameScore(name string) int {
	score := 0
	if strings.Contains(name, "/") {
		score += 10
	}
	if strings.Contains(name, "(") {
		score += 10
	}
	if strings.Contains(name, " - ") {
		score += 20
	}
	score += len(name)
	return score
}

// pickCanonicalAuthor selects the cleanest author name from a group.
func pickCanonicalAuthor(authors []database.Author, bookCountFn func(int) int) database.Author {
	if len(authors) == 0 {
		return database.Author{}
	}
	best := 0
	bestScore := authorNameScore(authors[0].Name)
	bestBooks := bookCountFn(authors[0].ID)

	for i := 1; i < len(authors); i++ {
		score := authorNameScore(authors[i].Name)
		books := bookCountFn(authors[i].ID)
		if score < bestScore || (score == bestScore && books > bestBooks) {
			best = i
			bestScore = score
			bestBooks = books
		}
	}
	return authors[best]
}

// FindDuplicateAuthors groups authors by similarity using structured name comparison.
// The threshold parameter is kept for API compatibility but the actual matching
// uses areAuthorsDuplicate which compares first/last names separately.
func FindDuplicateAuthors(authors []database.Author, threshold float64, bookCountFn func(int) int) []AuthorDedupGroup {
	used := make(map[int]bool)
	var groups []AuthorDedupGroup

	for i := 0; i < len(authors); i++ {
		if used[authors[i].ID] || isMultiAuthorString(authors[i].Name) || isCompositeAuthorName(authors[i].Name) || isDirtyAuthorName(authors[i].Name) {
			continue
		}

		group := AuthorDedupGroup{
			Canonical: authors[i],
		}

		for j := i + 1; j < len(authors); j++ {
			if used[authors[j].ID] || isMultiAuthorString(authors[j].Name) || isCompositeAuthorName(authors[j].Name) || isDirtyAuthorName(authors[j].Name) {
				continue
			}

			if areAuthorsDuplicate(authors[i].Name, authors[j].Name) {
				group.Variants = append(group.Variants, authors[j])
				used[authors[j].ID] = true
			}
		}

		if len(group.Variants) > 0 {
			// Pick the cleanest name as canonical
			allInGroup := make([]database.Author, 0, 1+len(group.Variants))
			allInGroup = append(allInGroup, authors[i])
			allInGroup = append(allInGroup, group.Variants...)
			canonical := pickCanonicalAuthor(allInGroup, bookCountFn)
			group.Canonical = canonical
			// Rebuild variants excluding canonical
			var variants []database.Author
			for _, a := range allInGroup {
				if a.ID != canonical.ID {
					variants = append(variants, a)
				}
			}
			group.Variants = variants

			used[authors[i].ID] = true
			totalBooks := bookCountFn(canonical.ID)
			for _, v := range group.Variants {
				totalBooks += bookCountFn(v.ID)
			}
			group.BookCount = totalBooks
			groups = append(groups, group)
		}
	}

	return groups
}
