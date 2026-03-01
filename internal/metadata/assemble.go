// file: internal/metadata/assemble.go
// version: 1.0.0
// guid: 1b2c3d4e-5f6a-7b8c-9d0e-1f2a3b4c5d6e

package metadata

import (
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// AssembledMetadata is the combined, priority-resolved metadata for an audiobook.
type AssembledMetadata struct {
	Title          string
	Authors        []string
	SeriesName     string
	SeriesPosition int
	Narrator       string
	Year           int
	Genre          string
	Language       string
	Publisher      string
	ISBN13         string
	ISBN10         string
	FileCount      int
	TotalDuration  float64

	TitleSource    string
	AuthorSource   string
	SeriesSource   string
	NarratorSource string
}

// AssembleBookMetadata builds a BookMetadata from folder path hierarchy + first file tags.
func AssembleBookMetadata(dirPath, firstFilePath string, fileCount int, totalDuration float64) (*AssembledMetadata, error) {
	bm := &AssembledMetadata{
		FileCount:     fileCount,
		TotalDuration: totalDuration,
	}

	fm, err := ExtractMetadataFromFolder(dirPath)
	if err != nil {
		log.Printf("[WARN] assemble: folder parser error for %s: %v", dirPath, err)
		fm = &FolderMetadata{}
	}

	var tagMeta *Metadata
	if firstFilePath != "" && firstFilePath != dirPath {
		info, statErr := os.Stat(firstFilePath)
		if statErr == nil && !info.IsDir() {
			m, tagErr := ExtractMetadata(firstFilePath)
			if tagErr == nil {
				tagMeta = &m
			} else {
				log.Printf("[WARN] assemble: tag extraction failed for %s: %v", firstFilePath, tagErr)
			}
		}
	}

	bm.Title, bm.TitleSource = resolveTitle(tagMeta, fm, firstFilePath)
	bm.Authors, bm.AuthorSource = resolveAuthors(tagMeta, fm)
	bm.SeriesName, bm.SeriesPosition, bm.SeriesSource = resolveSeries(tagMeta, fm)
	bm.Narrator, bm.NarratorSource = resolveNarrator(tagMeta, fm)

	if tagMeta != nil && tagMeta.Year > 0 {
		bm.Year = tagMeta.Year
	}
	if tagMeta != nil {
		bm.Genre = tagMeta.Genre
		bm.Language = tagMeta.Language
		bm.Publisher = tagMeta.Publisher
		bm.ISBN13 = tagMeta.ISBN13
		bm.ISBN10 = tagMeta.ISBN10
	}

	log.Printf(
		"[INFO] assemble: %s → title=%q authors=%v series=%q pos=%d narrator=%q",
		dirPath, bm.Title, bm.Authors, bm.SeriesName, bm.SeriesPosition, bm.Narrator,
	)
	return bm, nil
}

func resolveTitle(tag *Metadata, fm *FolderMetadata, firstFilePath string) (string, string) {
	if tag != nil && tag.Title != "" {
		if !isGenericTitle(tag.Title) {
			return tag.Title, "tag.Title"
		}
		log.Printf("[DEBUG] assemble: tag title %q looks generic; trying folder parser", tag.Title)
	}
	if fm.Title != "" {
		return fm.Title, "folder.Title"
	}
	dirName := filepath.Base(firstFilePath)
	if dirName != "" && dirName != "." {
		dirName = strings.TrimSuffix(dirName, filepath.Ext(dirName))
		if !IsGenericPartFilename(dirName) {
			return dirName, "filename"
		}
	}
	return "", "unknown"
}

func isGenericTitle(title string) bool {
	lower := strings.ToLower(strings.TrimSpace(title))
	genericPrefixes := []string{
		"part ", "chapter ", "track ", "disc ", "disk ",
	}
	for _, pfx := range genericPrefixes {
		if strings.HasPrefix(lower, pfx) {
			return true
		}
	}
	return IsGenericPartFilename(title + ".mp3")
}

func resolveAuthors(tag *Metadata, fm *FolderMetadata) ([]string, string) {
	if tag != nil && tag.Artist != "" {
		authors := splitMultipleAuthors(tag.Artist)
		if len(authors) > 0 {
			return authors, "tag.Artist"
		}
	}
	if len(fm.Authors) > 0 {
		return fm.Authors, "folder.Authors"
	}
	return nil, "unknown"
}

func resolveSeries(tag *Metadata, fm *FolderMetadata) (string, int, string) {
	if tag != nil {
		if tag.Series != "" {
			return tag.Series, tag.SeriesIndex, "tag.Series"
		}
		if tag.Album != "" && fm.SeriesName != "" && strings.EqualFold(tag.Album, fm.SeriesName) {
			return fm.SeriesName, fm.SeriesPosition, "folder.Series(album-confirmed)"
		}
	}
	if fm.SeriesName != "" {
		return fm.SeriesName, fm.SeriesPosition, "folder.Series"
	}
	return "", 0, "unknown"
}

func resolveNarrator(tag *Metadata, fm *FolderMetadata) (string, string) {
	if tag != nil && tag.Narrator != "" {
		return tag.Narrator, "tag.Narrator"
	}
	if fm.Narrator != "" {
		return fm.Narrator, "folder.Narrator"
	}
	if tag != nil && tag.Comments != "" {
		if n := extractNarratorFromComment(tag.Comments); n != "" {
			return n, "tag.Comment"
		}
	}
	return "", "unknown"
}

func extractNarratorFromComment(comment string) string {
	prefixes := []string{"narrator:", "read by:", "narrated by:", "reader:"}
	lower := strings.ToLower(comment)
	for _, pfx := range prefixes {
		if idx := strings.Index(lower, pfx); idx >= 0 {
			rest := strings.TrimSpace(comment[idx+len(pfx):])
			end := strings.IndexAny(rest, "\n\r,;")
			if end > 0 {
				return strings.TrimSpace(rest[:end])
			}
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// PrimaryAuthor returns the first author from the Authors slice, or empty string.
func (bm *AssembledMetadata) PrimaryAuthor() string {
	if len(bm.Authors) == 0 {
		return ""
	}
	return bm.Authors[0]
}

// FindFirstAudioFile returns the alphabetically first audio file in dirPath.
func FindFirstAudioFile(dirPath string, supportedExts []string) string {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return ""
	}

	extSet := make(map[string]bool, len(supportedExts))
	for _, e := range supportedExts {
		extSet[strings.ToLower(e)] = true
	}

	var audioFiles []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if extSet[ext] {
			audioFiles = append(audioFiles, filepath.Join(dirPath, e.Name()))
		}
	}

	if len(audioFiles) == 0 {
		return ""
	}
	sort.Strings(audioFiles)
	return audioFiles[0]
}
