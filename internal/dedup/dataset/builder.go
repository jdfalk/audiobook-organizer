// file: internal/dedup/dataset/builder.go
// version: 1.1.3
// guid: 4a91c7e0-6d83-4b25-9f10-2c5a8e7d4b31
// last-edited: 2026-06-13

// Package dataset builds labeled dedup examples and runs deterministic catchers
// over them. Pure: a store interface in, a database.LabeledExample out, no
// side effects. This is the audit CLI's per-pair logic promoted to a reusable,
// unit-tested package (spec C1/C2).
package dataset

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/falkcorp/audiobook-organizer/internal/database"
	"github.com/falkcorp/audiobook-organizer/internal/fingerprint"
)

// sigMatchThreshold is the BookSignatureSimilarity score at/above which two
// whole-book signatures are treated as a content match (spec: 0.95).
const sigMatchThreshold = 0.95

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

	// Populate Score and ScoreBreakdown snapshot from the candidate's unified
	// score when present. On current production data these are nil (Experiment 0
	// found 100% empty) — this is forward-correctness for rows produced by the
	// T015/T016 unified pipeline. A nil ScoreBreakdown leaves Score at 0 and
	// ex.ScoreBreakdown nil, which is the safe/zero value.
	if cand.ScoreBreakdown != nil {
		ex.Score = cand.ScoreBreakdown.Score
		if raw, err := json.Marshal(cand.ScoreBreakdown); err == nil {
			ex.ScoreBreakdown = raw
		}
		// On marshal error: Score is still set; ScoreBreakdown is left nil.
		// BuildExample never fails due to a snapshot marshal error.
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
		// Book-level size as a baseline; the per-file max below can exceed it.
		if bk.FileSize != nil && *bk.FileSize > f.FileSizeBytes {
			f.FileSizeBytes = *bk.FileSize
		}
	}
	var total float64
	for i := range files {
		fl := &files[i]
		if f.PrimaryPath == "" && fl.FilePath != "" {
			f.PrimaryPath = fl.FilePath
		}
		// Largest file size across the book's files — the signal for "has real
		// audio" vs a stub. A genuine unscanned copy keeps a large size here.
		if fl.FileSize > f.FileSizeBytes {
			f.FileSizeBytes = fl.FileSize
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
// Note: currently produces only these four values; sibling_parts is planned but not yet returned.
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
// Uses fingerprint.BookSignatureSimilarity with a 0.95 threshold for "match".
// Returns one of: match, disjoint, unknown.
// "unknown" is returned when either signature is absent or the comparator errors
// (e.g. corrupt/short base64). Offset/subsequence containment (a_contains_b /
// b_contains_a) is deferred to a later spec milestone.
func signatureRelation(a, b *database.Book) string {
	if a == nil || b == nil {
		return "unknown"
	}
	if a.BookSigV1 == nil || *a.BookSigV1 == "" || b.BookSigV1 == nil || *b.BookSigV1 == "" {
		return "unknown"
	}
	sim, err := fingerprint.BookSignatureSimilarity(*a.BookSigV1, *b.BookSigV1)
	if err != nil {
		return "unknown"
	}
	if sim >= sigMatchThreshold {
		return "match"
	}
	return "disjoint"
}
