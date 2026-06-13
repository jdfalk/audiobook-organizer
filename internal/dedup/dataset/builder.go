// file: internal/dedup/dataset/builder.go
// version: 1.0.0
// guid: 4a91c7e0-6d83-4b25-9f10-2c5a8e7d4b31
// last-edited: 2026-06-13

// Package dataset builds labeled dedup examples and runs deterministic catchers
// over them. Pure: a store interface in, a database.LabeledExample out, no
// side effects. This is the audit CLI's per-pair logic promoted to a reusable,
// unit-tested package (spec C1/C2).
package dataset

import (
	"path/filepath"
	"strings"

	"github.com/falkcorp/audiobook-organizer/internal/database"
)

// BuilderStore is the narrow store surface BuildExample needs.
type BuilderStore interface {
	GetBook(id string) (*database.Book, error)
	GetBookFiles(id string) ([]database.BookFile, error)
}

// BuildExample loads both books and computes every feature for the candidate pair.
// It is pure: it reads from the store and returns a LabeledExample with no side
// effects. Label fields are left empty — callers should run Classify to populate
// them.
func BuildExample(store BuilderStore, cand database.DedupCandidate) (database.LabeledExample, error) {
	ex := database.LabeledExample{
		CandidateID: cand.ID,
		EntityAID:   cand.EntityAID,
		EntityBID:   cand.EntityBID,
		Layer:       cand.Layer,
		Band:        cand.Band,
		Similarity:  cand.Similarity,
	}

	a, aFiles, err := loadSide(store, cand.EntityAID)
	if err != nil {
		return ex, err
	}
	b, bFiles, err := loadSide(store, cand.EntityBID)
	if err != nil {
		return ex, err
	}
	ex.A = buildFeatures(a, aFiles)
	ex.B = buildFeatures(b, bFiles)

	ex.DurationRatio = durationRatio(ex.A.TotalDurationSec, ex.B.TotalDurationSec)
	ex.FolderRelation = folderRelation(ex.A.PrimaryPath, ex.B.PrimaryPath)
	ex.SharesRecordingID = sharesAny(ex.A.RecordingIDs, ex.B.RecordingIDs)
	ex.SignatureRelation = signatureRelation(a, b)
	return ex, nil
}

func loadSide(store BuilderStore, id string) (*database.Book, []database.BookFile, error) {
	bk, err := store.GetBook(id)
	if err != nil {
		return nil, nil, err
	}
	files, err := store.GetBookFiles(id)
	if err != nil {
		return bk, nil, err
	}
	return bk, files, nil
}

// buildFeatures computes the per-book feature snapshot from a book and its files.
// Note: BookFeatures.Author is left empty — BuilderStore provides only Book and
// BookFile records; author name resolution would require a separate store method.
func buildFeatures(bk *database.Book, files []database.BookFile) database.BookFeatures {
	f := database.BookFeatures{
		FileCount:  len(files),
		FilesExist: len(files) > 0,
	}
	if bk != nil {
		f.Title = bk.Title
		f.WholeBookSigPresent = bk.BookSigV1 != nil && *bk.BookSigV1 != ""
		if bk.CoverURL != nil && *bk.CoverURL != "" {
			f.HasCover = true
		}
	}
	var total float64
	for i := range files {
		fl := &files[i]
		if f.PrimaryPath == "" && fl.FilePath != "" {
			f.PrimaryPath = fl.FilePath
		}
		// Prefer fpcalc-measured duration; fall back to container duration (int seconds).
		if fl.AcoustIDFingerprintDurationSec > 0 {
			total += fl.AcoustIDFingerprintDurationSec
		} else if fl.Duration > 0 {
			total += float64(fl.Duration)
		}
		if fl.AcoustIDOnlineRecordingID != "" {
			f.RecordingIDs = append(f.RecordingIDs, fl.AcoustIDOnlineRecordingID)
		}
		if fl.ITunesPersistentID != "" {
			f.ITunesPIDPresent = true
		}
	}
	f.TotalDurationSec = total
	return f
}

// durationRatio returns min/max of the two durations, or 0 if either is ≤0.
func durationRatio(a, b float64) float64 {
	if a <= 0 || b <= 0 {
		return 0
	}
	lo, hi := a, b
	if lo > hi {
		lo, hi = hi, lo
	}
	return lo / hi
}

// folderRelation classifies how two primary file paths sit relative to each other.
// Returns one of: unrelated, same_dir, a_ancestor_of_b, b_ancestor_of_a.
func folderRelation(a, b string) string {
	if a == "" || b == "" {
		return "unrelated"
	}
	da, db := filepath.Dir(a), filepath.Dir(b)
	if da == db {
		return "same_dir"
	}
	if isAncestor(da, db) {
		return "a_ancestor_of_b"
	}
	if isAncestor(db, da) {
		return "b_ancestor_of_a"
	}
	return "unrelated"
}

// isAncestor reports whether anc is a strict path ancestor of desc.
func isAncestor(anc, desc string) bool {
	anc = strings.TrimRight(anc, "/")
	return anc != "" && strings.HasPrefix(desc, anc+"/")
}

// sharesAny reports whether the two recording-ID slices share any element.
func sharesAny(a, b []string) bool {
	set := make(map[string]struct{}, len(a))
	for _, x := range a {
		set[x] = struct{}{}
	}
	for _, y := range b {
		if _, ok := set[y]; ok {
			return true
		}
	}
	return false
}

// signatureRelation reports the whole-book-signature relationship between two books.
// Task 5 (M1 initial): uses raw byte equality as a definite match; sim-threshold
// comparison is wired in Task 6 via fingerprint.BookSignatureSimilarity.
// Returns one of: match, disjoint, unknown.
func signatureRelation(a, b *database.Book) string {
	if a == nil || b == nil {
		return "unknown"
	}
	if a.BookSigV1 == nil || *a.BookSigV1 == "" || b.BookSigV1 == nil || *b.BookSigV1 == "" {
		return "unknown"
	}
	// Exact string equality is a definite match. Task 6 replaces this with the
	// real similarity comparison so near-identical sigs also return "match".
	if *a.BookSigV1 == *b.BookSigV1 {
		return "match"
	}
	return "unknown"
}
