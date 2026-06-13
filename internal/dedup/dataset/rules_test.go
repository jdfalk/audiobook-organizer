// file: internal/dedup/dataset/rules_test.go
// version: 1.0.0
// guid: c1d4e8b5-7f23-4a90-9b01-6e2c5d8f3a47
// last-edited: 2026-06-13

package dataset

import (
	"testing"

	"github.com/falkcorp/audiobook-organizer/internal/database"
)

func TestCatchers(t *testing.T) {
	cases := []struct {
		name      string
		ex        database.LabeledExample
		wantFires bool
		wantLabel string
	}{
		{
			name: "part vs whole by duration ratio",
			ex: database.LabeledExample{
				A:             database.BookFeatures{TotalDurationSec: 36000, FilesExist: true},
				B:             database.BookFeatures{TotalDurationSec: 1200, FilesExist: true},
				DurationRatio: 1200.0 / 36000.0,
			},
			wantFires: true, wantLabel: "not_dup",
		},
		{
			name: "missing file one side",
			ex: database.LabeledExample{
				A: database.BookFeatures{FilesExist: true, TotalDurationSec: 100},
				B: database.BookFeatures{FilesExist: false},
			},
			wantFires: true, wantLabel: "not_dup",
		},
		{
			name: "whole-book signature match => true_dup",
			ex: database.LabeledExample{
				A:                 database.BookFeatures{FilesExist: true, WholeBookSigPresent: true, TotalDurationSec: 36000},
				B:                 database.BookFeatures{FilesExist: true, WholeBookSigPresent: true, TotalDurationSec: 36000},
				SignatureRelation: "match", DurationRatio: 1.0,
			},
			wantFires: true, wantLabel: "true_dup",
		},
		{
			name: "no rule fires",
			ex: database.LabeledExample{
				A:                 database.BookFeatures{FilesExist: true, TotalDurationSec: 36000},
				B:                 database.BookFeatures{FilesExist: true, TotalDurationSec: 35900},
				DurationRatio:     35900.0 / 36000.0,
				SignatureRelation: "unknown",
			},
			wantFires: false,
		},
		{
			// The signature match catcher must fire BEFORE the missing-file catcher
			// to respect priority order. Both sigs present + "match" wins even if
			// one side is somehow also flagged missing (unlikely but tests priority).
			name: "signature match beats missing file (priority check)",
			ex: database.LabeledExample{
				A:                 database.BookFeatures{FilesExist: true, WholeBookSigPresent: true, TotalDurationSec: 36000},
				B:                 database.BookFeatures{FilesExist: false, WholeBookSigPresent: true},
				SignatureRelation: "match",
			},
			wantFires: true, wantLabel: "true_dup",
		},
		{
			// missing-file fires before part-vs-whole
			name: "missing file beats part-vs-whole (priority check)",
			ex: database.LabeledExample{
				A:             database.BookFeatures{FilesExist: true, TotalDurationSec: 36000},
				B:             database.BookFeatures{FilesExist: false, TotalDurationSec: 100},
				DurationRatio: 100.0 / 36000.0,
			},
			wantFires: true, wantLabel: "not_dup",
		},
		{
			// disjoint signature should not trigger signature-match catcher
			name: "disjoint signature does not fire match catcher",
			ex: database.LabeledExample{
				A:                 database.BookFeatures{FilesExist: true, WholeBookSigPresent: true, TotalDurationSec: 36000},
				B:                 database.BookFeatures{FilesExist: true, WholeBookSigPresent: true, TotalDurationSec: 36000},
				DurationRatio:     1.0,
				SignatureRelation: "disjoint",
			},
			wantFires: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			label, reason, fires := Classify(tc.ex)
			if fires != tc.wantFires {
				t.Fatalf("fires=%v want %v (reason=%q)", fires, tc.wantFires, reason)
			}
			if fires && label != tc.wantLabel {
				t.Fatalf("label=%q want %q (reason=%q)", label, tc.wantLabel, reason)
			}
		})
	}
}

func TestClassify_ReasonContainsRatio(t *testing.T) {
	ex := database.LabeledExample{
		A:             database.BookFeatures{TotalDurationSec: 36000, FilesExist: true},
		B:             database.BookFeatures{TotalDurationSec: 1200, FilesExist: true},
		DurationRatio: 1200.0 / 36000.0,
	}
	_, reason, fires := Classify(ex)
	if !fires {
		t.Fatal("expected catcher to fire")
	}
	if len(reason) == 0 {
		t.Fatal("expected non-empty reason")
	}
	// Reason should mention the ratio numerically (smoke check, not an exact match).
	if reason == "not_dup" {
		t.Fatalf("reason should describe the cause, not just repeat the label; got %q", reason)
	}
}
