// file: internal/server/deluge_discovery.go
// version: 2.3.1
// guid: e6f7a8b9-c0d1-2e3f-4a5b-6c7d8e9f0a1b
//
// Deluge label-based audiobook discovery.
//
// Four-tier matching to decide if a labeled Deluge torrent is already in
// the library — run in order, stop on first hit:
//
//  1. Hash    — GetBookVersionByTorrentHash: exact, works regardless of where
//               the file moved after organize. Requires the torrent was
//               previously imported via the Deluge flow (hash stored then).
//  2. Path    — filepath.Join(save_path, name) is a prefix of Book.FilePath:
//               catches books still sitting in the download directory.
//  3. Title   — parse the torrent name into candidate title strings, check
//               against a normalised set of Book.Title values. When a title
//               match fires, Tier 4 SHA-verifies the actual files.
//  4. SHA256  — SHA256 each audio file found under content_path, query
//               GetBookByFileHash for each. A hash hit confirms the fuzzy
//               title match; a miss means it's a different edition/version
//               and the torrent IS a new candidate even though titles match.
//
// A torrent that passes all four tiers is returned as a discovery candidate.

package server

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	delugeclient "github.com/jdfalk/audiobook-organizer/internal/deluge"
	"github.com/jdfalk/audiobook-organizer/internal/fingerprint"
)

// DiscoveredTorrent is a Deluge torrent not yet tracked in the library.
type DiscoveredTorrent struct {
	Hash        string  `json:"hash"`
	Name        string  `json:"name"`
	SavePath    string  `json:"save_path"`
	ContentPath string  `json:"content_path"` // filepath.Join(save_path, name) — import this
	Label       string  `json:"label"`
	State       string  `json:"state"`
	Progress    float64 `json:"progress"`
	TotalSize   int64   `json:"total_size"`
	MatchTier   string  `json:"match_tier,omitempty"` // debug: which tier matched (empty = new)
}

// libraryIndex is a pre-built lookup structure used by all three match tiers.
type libraryIndex struct {
	// Tier 2: normalised current file paths → present
	paths map[string]struct{}
	// Tier 3: normalised book titles → present
	titles map[string]struct{}
}

func (s *Server) buildLibraryIndex() libraryIndex {
	idx := libraryIndex{
		paths:  make(map[string]struct{}),
		titles: make(map[string]struct{}),
	}
	books, err := s.Store().GetAllBooks(100000, 0)
	if err != nil {
		log.Printf("[WARN] deluge discovery: failed to load books: %v", err)
		return idx
	}
	for _, b := range books {
		if b.FilePath != "" {
			idx.paths[b.FilePath] = struct{}{}
		}
		if b.Title != "" {
			idx.titles[normalizeTitle(b.Title)] = struct{}{}
		}
	}
	return idx
}

// discoverUnimported fetches labeled torrents and returns those not already
// in the library according to the three-tier matching strategy.
func (s *Server) discoverUnimported(client *delugeclient.Client, label string) ([]DiscoveredTorrent, error) {
	torrents, err := client.ListTorrentsByLabel(label)
	if err != nil {
		return nil, err
	}
	if len(torrents) == 0 {
		return []DiscoveredTorrent{}, nil
	}

	idx := s.buildLibraryIndex()

	var unimported []DiscoveredTorrent
	for _, t := range torrents {
		// Tier 1: torrent hash lookup (O(1), authoritative).
		if t.Hash != "" {
			if ver, _ := s.Store().GetBookVersionByTorrentHash(t.Hash); ver != nil {
				continue // already tracked
			}
		}

		// Tier 2: content path prefix against current file paths.
		contentPath := filepath.Join(t.SavePath, t.Name)
		if isPathTracked(contentPath, idx.paths) {
			continue
		}

		// Tier 3: torrent name → title candidates against known titles.
		// When a title match fires, Tier 4 verifies actual file content.
		if isTitleTracked(t.Name, idx.titles) {
			if s.isContentFingerprintTracked(contentPath) {
				continue // same audio stream — already in library
			}
			// Title matched but audio differs → different edition, surface it.
		}

		unimported = append(unimported, DiscoveredTorrent{
			Hash:        t.Hash,
			Name:        t.Name,
			SavePath:    t.SavePath,
			ContentPath: contentPath,
			Label:       t.Label,
			State:       t.State,
			Progress:    t.Progress,
			TotalSize:   t.TotalSize,
		})
	}
	return unimported, nil
}

// isContentFingerprintTracked walks contentPath for the first audio file,
// fingerprints it with fpcalc, and checks the library via exact then fuzzy
// AcoustID match. Returns true if the audio stream is already tracked.
//
// Falls back to SHA-256 walking when fpcalc is not installed so the pipeline
// is never blocked by a missing dependency.
func (s *Server) isContentFingerprintTracked(contentPath string) bool {
	if !fingerprint.Available() {
		// fpcalc not installed — fall back to SHA-256 content walk.
		hashLookup := func(hash string) bool {
			b, _ := s.Store().GetBookByFileHash(hash)
			return b != nil
		}
		return isContentHashTracked(contentPath, hashLookup)
	}

	// Find the first audio file under contentPath to fingerprint.
	var firstAudio string
	_ = filepath.Walk(contentPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || firstAudio != "" {
			return nil
		}
		if _, ok := audioExtensions[strings.ToLower(filepath.Ext(path))]; ok {
			firstAudio = path
		}
		return nil
	})
	if firstAudio == "" {
		return false
	}

	segs, err := fingerprint.FileSegments(firstAudio, 0)
	if err != nil {
		log.Printf("[WARN] deluge discovery: fingerprint %s: %v", firstAudio, err)
		return false
	}
	// Use the intro segment (seg[0]) for exact and fuzzy lookups.
	introFP := segs[0]
	if introFP == "" {
		return false
	}

	// Exact match first (O(1) index lookup).
	if f, _ := s.Store().GetBookFileByAcoustID(introFP); f != nil {
		return true
	}

	// Fuzzy fallback — catches minor encoding variations.
	f, _ := s.Store().GetBookFileByAcoustIDFuzzy(introFP, fingerprint.FuzzyMinSimilarity)
	return f != nil
}

// isPathTracked returns true if contentPath is a prefix of any known file path.
//
// Callers MUST pass filepath.Join(save_path, torrent_name) — NOT save_path
// alone. A shared download directory is a prefix of everything in it, so
// using raw save_path would mark every torrent as tracked once any file from
// that directory exists in the DB.
func isPathTracked(contentPath string, known map[string]struct{}) bool {
	if contentPath == "" {
		return false
	}
	prefix := strings.TrimRight(contentPath, "/") + "/"
	for p := range known {
		if strings.HasPrefix(p, prefix) || p == contentPath {
			return true
		}
	}
	return false
}

// isTitleTracked parses a torrent name into candidate titles and checks each
// against the normalised DB title set.
func isTitleTracked(torrentName string, titles map[string]struct{}) bool {
	for _, candidate := range parseTorrentNameCandidates(torrentName) {
		if _, ok := titles[candidate]; ok {
			return true
		}
	}
	return false
}

// audioExtensions is the set of file extensions we hash for content matching.
var audioExtensions = map[string]struct{}{
	".m4b": {}, ".m4a": {}, ".mp3": {}, ".flac": {}, ".aax": {},
	".aac": {}, ".ogg": {}, ".opus": {}, ".wav": {},
}

// isContentHashTracked walks contentPath, SHA256s each audio file, and calls
// lookup for each hash. Returns true as soon as any hash is found in the DB.
func isContentHashTracked(contentPath string, lookup func(string) bool) bool {
	found := false
	_ = filepath.Walk(contentPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || found {
			return nil
		}
		if _, ok := audioExtensions[strings.ToLower(filepath.Ext(path))]; !ok {
			return nil
		}
		hash, hashErr := sha256File(path)
		if hashErr != nil {
			log.Printf("[WARN] deluge discovery: sha256 %s: %v", path, hashErr)
			return nil
		}
		if lookup(hash) {
			found = true
		}
		return nil
	})
	return found
}

// sha256File returns the hex-encoded SHA256 of a file's contents.
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// stripBrackets removes common annotation brackets: [Unabridged], {MP3}, (2023).
var stripBrackets = regexp.MustCompile(`[\[\]{}(][^\[\]{}()]*[\]\})]`)

// metaSuffixes are noise tokens appended to torrent names.
var metaSuffixes = regexp.MustCompile(`(?i)\b(unabridged|abridged|m4b|mp3|aax|flac|aac|ogg|retail|repack|\d{4})\b.*$`)

// parseTorrentNameCandidates returns a set of normalised title strings derived
// from a torrent name. The typical formats handled:
//
//	"Author - Title"              → ["title", "author"]
//	"Title - Author"              → ["title", "author"]
//	"Title by Author [M4B]"       → ["title"]
//	"Author.Title.Year.M4B"       → ["author title"] (dots-as-spaces)
//	"Title (Author) [Unabridged]" → ["title"]
func parseTorrentNameCandidates(name string) []string {
	seen := make(map[string]struct{})
	add := func(s string) {
		s = normalizeTitle(s)
		if len(s) >= 3 {
			seen[s] = struct{}{}
		}
	}

	// Strip bracket annotations first.
	clean := stripBrackets.ReplaceAllString(name, " ")
	// Remove file-format / year suffixes.
	clean = metaSuffixes.ReplaceAllString(clean, "")
	clean = strings.TrimSpace(clean)

	// Try dash-separated "Author - Title" or "Title - Author".
	if parts := strings.SplitN(clean, " - ", 2); len(parts) == 2 {
		add(parts[0])
		add(parts[1])
	}

	// Try "Title by Author".
	if idx := strings.Index(strings.ToLower(clean), " by "); idx > 0 {
		add(clean[:idx])
	}

	// Try dot-separated names (common for scene releases).
	if strings.ContainsRune(clean, '.') && !strings.ContainsRune(clean, ' ') {
		add(strings.ReplaceAll(clean, ".", " "))
	}

	// Always include the whole cleaned name as a fallback candidate.
	add(clean)

	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	return out
}

// normalizeTitle lowercases, strips punctuation, and collapses whitespace so
// that "The Way of Kings" and "the way of kings!" both normalise to the same
// string for comparison.
func normalizeTitle(s string) string {
	var b strings.Builder
	prevSpace := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevSpace = false
		} else if !prevSpace {
			b.WriteRune(' ')
			prevSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

// ---------------------------------------------------------------------------
// HTTP handlers
// ---------------------------------------------------------------------------

// handleDelugeDiscover returns Deluge torrents not yet in the library.
// GET /api/v1/deluge/discover?label=audiobooks
func (s *Server) handleDelugeDiscover(c *gin.Context) {
	if !config.AppConfig.DelugeDiscoveryEnabled {
		c.JSON(http.StatusForbidden, gin.H{"error": "deluge discovery is disabled (set deluge_discovery_enabled=true to enable)"})
		return
	}
	client := getDelugeClient()
	if client == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "deluge not configured"})
		return
	}

	label := c.Query("label")
	if label == "" {
		label = config.AppConfig.DelugeDiscoveryLabel
	}

	unimported, err := s.discoverUnimported(client, label)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"label":      label,
		"candidates": unimported,
		"count":      len(unimported),
	})
}

// handleDelugeListLabels returns all labels from the Deluge Label plugin.
// GET /api/v1/deluge/labels
func (s *Server) handleDelugeListLabels(c *gin.Context) {
	client := getDelugeClient()
	if client == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "deluge not configured"})
		return
	}
	labels, err := client.ListLabels()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"labels": labels, "count": len(labels)})
}

// handleDelugeDiscoverImport triggers an import of a discovered torrent's
// content_path into the library. Reuses the existing ImportFile pipeline.
// POST /api/v1/deluge/discover/import
// Body: { "content_path": "/mnt/downloads/Dune by Frank Herbert", "torrent_hash": "abc123" }
func (s *Server) handleDelugeDiscoverImport(c *gin.Context) {
	if !config.AppConfig.DelugeDiscoveryEnabled {
		c.JSON(http.StatusForbidden, gin.H{"error": "deluge discovery is disabled (set deluge_discovery_enabled=true to enable)"})
		return
	}
	var req struct {
		ContentPath string `json:"content_path" binding:"required"`
		TorrentHash string `json:"torrent_hash"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondWithBadRequest(c, err.Error())
		return
	}
	if s.importService == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "import service not initialized"})
		return
	}

	resp, err := s.importService.ImportFile(&ImportFileRequest{
		FilePath: req.ContentPath,
		Organize: false,
	})
	if err != nil {
		RespondWithInternalError(c, err.Error())
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"book":         resp,
		"torrent_hash": req.TorrentHash,
	})
}

// handleDiscoveryImport triggers importToLibrary for all book_files that
// have a deluge_hash but have not yet been imported (imported_from_deluge_at IS NULL).
// This is the bulk-import trigger called from the Settings UI.
// POST /api/v1/discovery/import
// Optional body: { "dry_run": true, "max_books": 100 }
func (s *Server) handleDiscoveryImport(c *gin.Context) {
	var req struct {
		DryRun   bool `json:"dry_run"`
		MaxBooks int  `json:"max_books"`
	}
	_ = c.ShouldBindJSON(&req) // optional body

	store := s.Store()
	if store == nil {
		RespondWithInternalError(c, "database not initialized")
		return
	}
	client := getDelugeClient()
	if client == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "deluge not configured"})
		return
	}

	files, err := store.GetAllBookFiles()
	if err != nil {
		internalError(c, "failed to load book files", err)
		return
	}

	var pending []database.BookFile
	for i := range files {
		f := &files[i]
		if f.DelugeHash != "" && f.ImportedFromDelugeAt == nil {
			pending = append(pending, *f)
		}
	}

	if req.MaxBooks > 0 && len(pending) > req.MaxBooks {
		pending = pending[:req.MaxBooks]
	}

	type result struct {
		FileID  string `json:"file_id"`
		Path    string `json:"path"`
		NewPath string `json:"new_path,omitempty"`
		Error   string `json:"error,omitempty"`
	}

	var results []result
	imported, skipped, failed := 0, 0, 0

	for i := range pending {
		f := &pending[i]
		if req.DryRun {
			results = append(results, result{FileID: f.ID, Path: f.FilePath})
			skipped++
			continue
		}
		newPath, importErr := importToLibrary(&config.AppConfig, client, store, f)
		if importErr != nil {
			results = append(results, result{FileID: f.ID, Path: f.FilePath, Error: importErr.Error()})
			failed++
		} else {
			results = append(results, result{FileID: f.ID, Path: f.FilePath, NewPath: newPath})
			imported++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"total":    len(pending),
		"imported": imported,
		"skipped":  skipped,
		"failed":   failed,
		"dry_run":  req.DryRun,
		"results":  results,
	})
}
