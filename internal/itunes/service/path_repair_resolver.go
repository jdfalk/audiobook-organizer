// file: internal/itunes/service/path_repair_resolver.go
// version: 1.0.0
// guid: 7d4f25a1-8e29-4b8b-9a02-3c5e1f9d4b27
//
// Pure-function resolvers for the path-repair operation. Each tier
// takes a narrow store interface and an existsFn so tests can drive
// them without a real filesystem.

package itunesservice

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/matcher"
)

// audioExt is the set of file extensions tier B inspects. Anything
// outside the set is skipped during the walk.
var audioExt = map[string]struct{}{
	".m4b": {}, ".m4a": {}, ".mp3": {}, ".ogg": {}, ".flac": {},
	".opus": {}, ".aac": {}, ".wav": {},
}

// bookIDExtractor pulls the AUDIOBOOK_ORGANIZER_ID tag from one
// audio file. Returns "" when the tag is absent. Production wires
// this to metadata.ExtractMetadata; tests inject a deterministic fake.
type bookIDExtractor func(audioFilePath string) (string, error)

// noopTagScanner returns no matches. Used when tier B is misconfigured
// (no audiobook root, no extractor) so the worker can stay on a single
// code path without nil-checks.
type noopTagScanner struct{}

func (noopTagScanner) bookIDToPaths(string) []string { return nil }
func (noopTagScanner) allPaths() []string            { return nil }

// fsTagScanner walks the audiobook root once, lazily, the first time
// bookIDToPaths or allPaths is called. Subsequent calls hit the
// in-memory state.
type fsTagScanner struct {
	root    string
	extract bookIDExtractor
	once    sync.Once
	index   map[string][]string
	all     []string
}

func newFSTagScanner(root string, extract bookIDExtractor) *fsTagScanner {
	return &fsTagScanner{root: root, extract: extract}
}

func (s *fsTagScanner) bookIDToPaths(bookID string) []string {
	s.once.Do(s.scan)
	return s.index[bookID]
}

func (s *fsTagScanner) allPaths() []string {
	s.once.Do(s.scan)
	return s.all
}

func (s *fsTagScanner) scan() {
	s.index = make(map[string][]string)
	if s.root == "" {
		return
	}
	_ = filepath.WalkDir(s.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if _, ok := audioExt[ext]; !ok {
			return nil
		}
		s.all = append(s.all, path)
		if s.extract != nil {
			if bookID, err := s.extract(path); err == nil && bookID != "" {
				s.index[bookID] = append(s.index[bookID], path)
			}
		}
		return nil
	})
}

// tierAStore is the slice tier A needs after the bookID has been
// resolved at the worker level (see lookupBookID).
type tierAStore interface {
	GetBookByID(id string) (*database.Book, error)
	GetBookFiles(bookID string) ([]database.BookFile, error)
}

// pidLookup is the worker-level PID → bookID hop. Hoisted so tier A
// and tier B share a single DB call per missing track.
type pidLookup interface {
	GetBookByExternalID(source, externalID string) (string, error)
}

// lookupBookID returns the bookID for an iTunes PID, or "" when the
// mapping is absent or the store errors. Errors are intentionally
// swallowed here — at the tier resolver level "no mapping" and
// "lookup failed" lead to the same fall-through path.
func lookupBookID(s pidLookup, pid string) string {
	bookID, err := s.GetBookByExternalID("itunes", pid)
	if err != nil {
		return ""
	}
	return bookID
}

// tagScanner exposes a lazy, cached lookup from
// AUDIOBOOK_ORGANIZER_ID tag value (the audiobook-organizer book ID)
// to the on-disk paths whose audio files carry that tag, plus the
// flat list of every audio file the walk found (for tier C scoring).
// Tests inject a fake; production walks the audiobook root.
type tagScanner interface {
	bookIDToPaths(bookID string) []string
	allPaths() []string
}

// resolveTierB resolves a missing PID via the embedded
// AUDIOBOOK_ORGANIZER_ID tag scan: bookID (already looked up at the
// worker level) → unique on-disk path. Multi-segment books with
// multiple disk matches are deliberately returned unresolved — those
// go to tier C for human review.
func resolveTierB(scanner tagScanner, bookID string, existsFn func(string) bool) (string, bool) {
	if bookID == "" {
		return "", false
	}
	paths := scanner.bookIDToPaths(bookID)
	if len(paths) != 1 {
		return "", false
	}
	if !existsFn(paths[0]) {
		return "", false
	}
	return paths[0], true
}

// trackInfo carries the iTunes-side hints tier C scores against.
type trackInfo struct {
	Title       string
	OldBasename string
}

// tierCCandidate is one ranked match emitted to the needs_review list.
type tierCCandidate struct {
	Path  string `json:"path"`
	Score int    `json:"score"`
}

// resolveTierC scores every candidate path against the iTunes track
// title and the old basename, then returns the top-N candidates whose
// score meets the threshold. Never auto-applies — caller emits to the
// needs_review list for human confirmation.
//
// We score against both the title and the old basename and take the
// max so e.g. a file renamed to use the title still scores well, and
// a file whose basename was preserved across a directory move also
// scores well.
func resolveTierC(candidates []string, info trackInfo, threshold, topN int) []tierCCandidate {
	if len(candidates) == 0 || (info.Title == "" && info.OldBasename == "") {
		return nil
	}
	scored := make([]tierCCandidate, 0, len(candidates))
	for _, p := range candidates {
		base := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
		var s int
		if info.Title != "" {
			if v := matcher.ScoreMatch(info.Title, base); v > s {
				s = v
			}
		}
		if info.OldBasename != "" {
			oldBase := strings.TrimSuffix(info.OldBasename, filepath.Ext(info.OldBasename))
			if v := matcher.ScoreMatch(oldBase, base); v > s {
				s = v
			}
		}
		if s < threshold {
			continue
		}
		scored = append(scored, tierCCandidate{Path: p, Score: s})
	}
	sort.SliceStable(scored, func(i, j int) bool { return scored[i].Score > scored[j].Score })
	if topN > 0 && len(scored) > topN {
		scored = scored[:topN]
	}
	return scored
}

// resolveTierA returns the on-disk path the DB thinks the file is at
// for a given (pid, bookID) — preferring the matching BookFile over
// Book.FilePath. Returns ok=false when the DB-known path doesn't
// exist on disk.
//
// The caller is responsible for resolving PID → bookID via
// lookupBookID first, so tier A and tier B share that DB call.
func resolveTierA(s tierAStore, pid, bookID string, existsFn func(string) bool) (string, bool) {
	if bookID == "" {
		return "", false
	}
	if files, err := s.GetBookFiles(bookID); err == nil {
		for _, bf := range files {
			if bf.ITunesPersistentID == pid && bf.FilePath != "" && existsFn(bf.FilePath) {
				return bf.FilePath, true
			}
		}
	}
	if book, err := s.GetBookByID(bookID); err == nil && book != nil && book.FilePath != "" && existsFn(book.FilePath) {
		return book.FilePath, true
	}
	return "", false
}
