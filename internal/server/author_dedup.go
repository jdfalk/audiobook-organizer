// file: internal/server/author_dedup.go
// version: 1.0.0
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
	// Pattern: uppercase letter followed by dot followed by uppercase letter
	initialsRe := regexp.MustCompile(`([A-Z]\.)([A-Z])`)
	for initialsRe.MatchString(name) {
		name = initialsRe.ReplaceAllString(name, "$1 $2")
	}

	return strings.TrimSpace(name)
}

// jaroWinklerSimilarity computes the Jaro-Winkler similarity between two strings.
// Returns a value between 0 (no similarity) and 1 (identical).
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

// FindDuplicateAuthors groups authors by similarity.
// threshold is the Jaro-Winkler similarity threshold (e.g. 0.9).
func FindDuplicateAuthors(authors []database.Author, threshold float64, bookCountFn func(int) int) []AuthorDedupGroup {
	if threshold <= 0 {
		threshold = 0.9
	}

	used := make(map[int]bool)
	var groups []AuthorDedupGroup

	for i := 0; i < len(authors); i++ {
		if used[authors[i].ID] || isMultiAuthorString(authors[i].Name) {
			continue
		}

		norm1 := NormalizeAuthorName(authors[i].Name)
		group := AuthorDedupGroup{
			Canonical: authors[i],
		}

		for j := i + 1; j < len(authors); j++ {
			if used[authors[j].ID] || isMultiAuthorString(authors[j].Name) {
				continue
			}

			norm2 := NormalizeAuthorName(authors[j].Name)

			// Check normalized exact match first
			if strings.EqualFold(norm1, norm2) || jaroWinklerSimilarity(norm1, norm2) >= threshold {
				group.Variants = append(group.Variants, authors[j])
				used[authors[j].ID] = true
			}
		}

		if len(group.Variants) > 0 {
			used[authors[i].ID] = true
			totalBooks := bookCountFn(authors[i].ID)
			for _, v := range group.Variants {
				totalBooks += bookCountFn(v.ID)
			}
			group.BookCount = totalBooks
			groups = append(groups, group)
		}
	}

	return groups
}
