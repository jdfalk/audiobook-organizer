// file: internal/database/dedup_label.go
// version: 1.0.1
// guid: 5a0319bd-8bc4-4135-91e6-dfd43628dcc5
// last-edited: 2026-06-13

package database

import (
	"encoding/json"
	"fmt"

	"github.com/cockroachdb/pebble/v2"
)

// dedupLabelPfx is the Pebble keyspace for labeled dedup examples.
// Key layout: dedup:label:<candidateID, 16-hex> → LabeledExample JSON.
const dedupLabelPfx = "dedup:label:"

// dedupLabelKey renders the fixed-width key for a candidate ID so that range
// scans over the prefix return rows in a stable order.
func dedupLabelKey(candidateID int64) []byte {
	return []byte(fmt.Sprintf("%s%016x", dedupLabelPfx, uint64(candidateID)))
}

// BookFeatures captures the per-book evidence a judge needs. Computed by the
// dataset feature builder, snapshotted at capture time.
type BookFeatures struct {
	Title               string   `json:"title"`
	Author              string   `json:"author"`
	PrimaryPath         string   `json:"primary_path"`
	TotalDurationSec    float64  `json:"total_duration_sec"`
	FileCount           int      `json:"file_count"`
	HasCover            bool     `json:"has_cover"`
	FilesExist          bool     `json:"files_exist"`
	RecordingIDs        []string `json:"recording_ids,omitempty"`
	ITunesPIDPresent    bool     `json:"itunes_pid_present"`
	WholeBookSigPresent bool     `json:"whole_book_sig_present"`
}

// LabeledExample is one labeled dedup candidate pair plus the features behind
// the label. Stored at dedup:label:<candidateID>.
type LabeledExample struct {
	CandidateID int64  `json:"candidate_id"`
	EntityAID   string `json:"entity_a_id"`
	EntityBID   string `json:"entity_b_id"`

	Layer          string          `json:"layer"`
	Band           string          `json:"band,omitempty"`
	Score          float64         `json:"score"`
	ScoreBreakdown json.RawMessage `json:"score_breakdown,omitempty"`
	Similarity     *float64        `json:"similarity,omitempty"`

	A BookFeatures `json:"a"`
	B BookFeatures `json:"b"`

	DurationRatio     float64 `json:"duration_ratio"`
	FolderRelation    string  `json:"folder_relation"` // unrelated|same_dir|a_ancestor_of_b|b_ancestor_of_a|sibling_parts
	SharesRecordingID bool    `json:"shares_recording_id"`
	SignatureRelation string  `json:"signature_relation"` // unknown|match|disjoint|a_contains_b|b_contains_a

	Label          string `json:"label"`        // true_dup|not_dup|unsure
	LabelSource    string `json:"label_source"` // rule|itunes_attr|human|llm_judge
	LabelReason    string `json:"label_reason"`
	DecidedAt      string `json:"decided_at,omitempty"` // RFC3339; caller-stamped
	FormulaVersion string `json:"formula_version,omitempty"`
}

// LabeledExampleFilter narrows ListLabeledExamples / CountLabeledExamples.
// Empty fields are ignored. Filtering is in-memory over the prefix scan, which
// is fine at dataset scale (tens of thousands of rows).
type LabeledExampleFilter struct {
	Label             string
	LabelSource       string
	Band              string
	FolderRelation    string
	SignatureRelation string
	Limit             int
	Offset            int
}

func (f LabeledExampleFilter) matches(ex *LabeledExample) bool {
	if f.Label != "" && ex.Label != f.Label {
		return false
	}
	if f.LabelSource != "" && ex.LabelSource != f.LabelSource {
		return false
	}
	if f.Band != "" && ex.Band != f.Band {
		return false
	}
	if f.FolderRelation != "" && ex.FolderRelation != f.FolderRelation {
		return false
	}
	if f.SignatureRelation != "" && ex.SignatureRelation != f.SignatureRelation {
		return false
	}
	return true
}

// UpsertLabeledExample writes (or overwrites) a labeled example.
func (s *EmbeddingStore) UpsertLabeledExample(ex LabeledExample) error {
	if err := s.checkClosed(); err != nil {
		return err
	}
	data, err := json.Marshal(ex)
	if err != nil {
		return fmt.Errorf("marshal labeled example %d: %w", ex.CandidateID, err)
	}
	return s.db.Set(dedupLabelKey(ex.CandidateID), data, pebble.Sync)
}

// GetLabeledExample returns the example for a candidate, or nil if absent.
func (s *EmbeddingStore) GetLabeledExample(candidateID int64) (*LabeledExample, error) {
	if err := s.checkClosed(); err != nil {
		return nil, err
	}
	val, closer, err := s.db.Get(dedupLabelKey(candidateID))
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get labeled example %d: %w", candidateID, err)
	}
	defer func() { _ = closer.Close() }()
	var ex LabeledExample
	if err := json.Unmarshal(val, &ex); err != nil {
		return nil, fmt.Errorf("unmarshal labeled example %d: %w", candidateID, err)
	}
	return &ex, nil
}

// ListLabeledExamples returns examples matching the filter (prefix scan).
func (s *EmbeddingStore) ListLabeledExamples(f LabeledExampleFilter) ([]LabeledExample, error) {
	if err := s.checkClosed(); err != nil {
		return nil, err
	}
	prefix := []byte(dedupLabelPfx)
	upper := prefixUpperBound(prefix)
	iter, err := s.db.NewIter(&pebble.IterOptions{LowerBound: prefix, UpperBound: upper})
	if err != nil {
		return nil, fmt.Errorf("list labeled examples: %w", err)
	}
	defer func() { _ = iter.Close() }()

	var out []LabeledExample
	skipped := 0
	for iter.First(); iter.Valid(); iter.Next() {
		var ex LabeledExample
		if err := json.Unmarshal(iter.Value(), &ex); err != nil {
			continue // skip a corrupt row rather than abort the scan
		}
		if !f.matches(&ex) {
			continue
		}
		if f.Offset > 0 && skipped < f.Offset {
			skipped++
			continue
		}
		out = append(out, ex)
		if f.Limit > 0 && len(out) >= f.Limit {
			break
		}
	}
	return out, iter.Error()
}

// CountLabeledExamples counts examples matching the filter (Limit/Offset ignored).
func (s *EmbeddingStore) CountLabeledExamples(f LabeledExampleFilter) (int, error) {
	if err := s.checkClosed(); err != nil {
		return 0, err
	}
	cf := f
	cf.Limit, cf.Offset = 0, 0
	list, err := s.ListLabeledExamples(cf)
	if err != nil {
		return 0, err
	}
	return len(list), nil
}
