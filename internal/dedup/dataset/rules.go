// file: internal/dedup/dataset/rules.go
// version: 1.0.0
// guid: 9e2b4c71-3a85-4d60-8f29-1b7c6a4e5d02
// last-edited: 2026-06-13

package dataset

import (
	"fmt"

	"github.com/falkcorp/audiobook-organizer/internal/database"
)

// partVsWholeRatioMax is the duration-ratio ceiling below which a pair is
// classified as a part matched against a whole book (not a duplicate).
// A ratio below 0.5 means the shorter side is less than half the longer side.
const partVsWholeRatioMax = 0.5

// Classify runs the deterministic catchers in priority order and returns the
// first firing rule's (label, reason, fires=true). If no rule fires, it returns
// ("", "", false). The caller (backfill op) uses fires=false as "unsure" and
// leaves the example unlabeled for human or ML review.
//
// Priority order (highest first):
//  1. wholeBookSignatureMatch → true_dup  (strong positive oracle)
//  2. missingFile             → not_dup  (hard negative: never merge a ghost)
//  3. partVsWhole             → not_dup  (hard negative: duration mismatch)
func Classify(ex database.LabeledExample) (label, reason string, fires bool) {
	if l, r, ok := wholeBookSignatureMatch(ex); ok {
		return l, r, true
	}
	if l, r, ok := missingFile(ex); ok {
		return l, r, true
	}
	if l, r, ok := partVsWhole(ex); ok {
		return l, r, true
	}
	return "", "", false
}

// wholeBookSignatureMatch is the positive oracle: both sides have a computed
// whole-book signature and signatureRelation is "match" (sim ≥ 0.95 per Task 6
// wiring in builder.go). Returns true_dup.
func wholeBookSignatureMatch(ex database.LabeledExample) (string, string, bool) {
	if ex.A.WholeBookSigPresent && ex.B.WholeBookSigPresent && ex.SignatureRelation == "match" {
		return "true_dup", "whole-book signatures match", true
	}
	return "", "", false
}

// missingFile fires when either side has no resolvable files. We must never
// merge a book whose files are gone — there is nothing to consolidate into.
func missingFile(ex database.LabeledExample) (string, string, bool) {
	if !ex.A.FilesExist {
		return "not_dup", "side A has no resolvable files", true
	}
	if !ex.B.FilesExist {
		return "not_dup", "side B has no resolvable files", true
	}
	return "", "", false
}

// partVsWhole fires when both durations are known and the min/max ratio is below
// partVsWholeRatioMax, indicating the shorter entry is a chapter or excerpt of
// the longer one rather than a full duplicate.
func partVsWhole(ex database.LabeledExample) (string, string, bool) {
	if ex.A.TotalDurationSec > 0 && ex.B.TotalDurationSec > 0 &&
		ex.DurationRatio > 0 && ex.DurationRatio < partVsWholeRatioMax {
		return "not_dup", fmt.Sprintf("duration ratio %.3f — part vs whole", ex.DurationRatio), true
	}
	return "", "", false
}
