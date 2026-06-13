// file: internal/database/dedup_label_test.go
// version: 1.0.0
// guid: 28cfcafd-ac95-4175-8fe7-b0fc46bd05bb
// last-edited: 2026-06-13

package database

import (
	"os"
	"testing"
)

func newTestLabelStore(t *testing.T) *EmbeddingStore {
	t.Helper()
	dir, err := os.MkdirTemp("", "abk-label-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	ps, err := NewPebbleStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ps.Close() })
	return NewEmbeddingStore(ps.DB())
}

func TestLabeledExample_RoundTripAndFilter(t *testing.T) {
	es := newTestLabelStore(t)

	ex := LabeledExample{
		CandidateID:       42,
		EntityAID:         "a",
		EntityBID:         "b",
		Layer:             "exact",
		Label:             "not_dup",
		LabelSource:       "rule",
		LabelReason:       "duration ratio 0.02 — part vs whole",
		FolderRelation:    "sibling_parts",
		SignatureRelation: "unknown",
	}
	if err := es.UpsertLabeledExample(ex); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := es.GetLabeledExample(42)
	if err != nil || got == nil {
		t.Fatalf("get: %v (nil=%v)", err, got == nil)
	}
	if got.LabelReason != ex.LabelReason || got.Label != "not_dup" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}

	list, err := es.ListLabeledExamples(LabeledExampleFilter{Label: "not_dup", Limit: 10})
	if err != nil || len(list) != 1 {
		t.Fatalf("list by label: err=%v len=%d", err, len(list))
	}
	n, err := es.CountLabeledExamples(LabeledExampleFilter{LabelSource: "rule"})
	if err != nil || n != 1 {
		t.Fatalf("count by source: err=%v n=%d", err, n)
	}
}
