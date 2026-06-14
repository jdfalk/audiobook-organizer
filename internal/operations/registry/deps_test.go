// file: internal/operations/registry/deps_test.go
// version: 1.0.0
// guid: e1f2a3b4-c5d6-7e8f-9a0b-1c2d3e4f5a6b
// last-edited: 2026-06-13

package registry

import (
	"fmt"
	"testing"

	"github.com/falkcorp/audiobook-organizer/internal/database"
)

// fakeDepStore is a minimal in-memory DepStore for unit testing the evaluator.
type fakeDepStore struct {
	depRev         map[string]uint64            // key(sub) → rev
	completion     map[string]uint64            // ckey(sub, opType) → rev
	fileCompletion map[string]map[string]uint64 // ckey(sub, opType) → fileID→rev
	files          map[string][]string          // bookID → file IDs
}

func newFakeDepStore() *fakeDepStore {
	return &fakeDepStore{
		depRev:         make(map[string]uint64),
		completion:     make(map[string]uint64),
		fileCompletion: make(map[string]map[string]uint64),
		files:          make(map[string][]string),
	}
}

func depStoreSubKey(sub Subject) string {
	return sub.Type + ":" + sub.ID
}

func depStoreCKey(sub Subject, opType string) string {
	return sub.Type + ":" + sub.ID + ":" + opType
}

func (f *fakeDepStore) GetDepRev(sub database.OpSubject) (uint64, error) {
	return f.depRev[sub.Type+":"+sub.ID], nil
}

func (f *fakeDepStore) GetOpCompletion(sub database.OpSubject, opType string) (uint64, bool, error) {
	key := sub.Type + ":" + sub.ID + ":" + opType
	rev, ok := f.completion[key]
	return rev, ok, nil
}

func (f *fakeDepStore) ListFileCompletions(sub database.OpSubject, opType string) (map[string]uint64, error) {
	key := sub.Type + ":" + sub.ID + ":" + opType
	m, ok := f.fileCompletion[key]
	if !ok {
		return nil, nil
	}
	// Return a copy so callers can't mutate the test state.
	out := make(map[string]uint64, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out, nil
}

func (f *fakeDepStore) BookFiles(bookID string) ([]string, error) {
	return f.files[bookID], nil
}

// TestEvaluate_OpCompleted_FreshVsStale verifies the core freshness check:
// a completion at a stale rev does NOT satisfy; at the current rev it does.
func TestEvaluate_OpCompleted_FreshVsStale(t *testing.T) {
	st := newFakeDepStore()
	sub := Subject{Type: "book", ID: "b1"}
	st.depRev[depStoreSubKey(sub)] = 2
	req := Requirement{Kind: ReqOpCompleted, OpType: "fp"}

	// no completion record → unmet
	ok, reason, err := Satisfied(st, req, sub)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("should be unmet without any completion record")
	}
	if reason == "" {
		t.Fatal("expected non-empty reason when unmet")
	}

	// completion at rev 1 but current dep_rev is 2 → stale → unmet
	st.completion[depStoreCKey(sub, "fp")] = 1
	ok, _, err = Satisfied(st, req, sub)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("stale completion (rev 1 < current 2) must not satisfy")
	}

	// completion at rev 2 → fresh → satisfied
	st.completion[depStoreCKey(sub, "fp")] = 2
	ok, _, err = Satisfied(st, req, sub)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("fresh completion (rev 2 == current 2) must satisfy")
	}
}

// TestEvaluate_AllFiles_Coverage verifies that AllFiles=true requires every file
// of the book to have a fresh completion record.
func TestEvaluate_AllFiles_Coverage(t *testing.T) {
	st := newFakeDepStore()
	sub := Subject{Type: "book", ID: "b1"}
	st.depRev[depStoreSubKey(sub)] = 1
	st.files["b1"] = []string{"f1", "f2"}
	req := Requirement{Kind: ReqOpCompleted, OpType: "fp", AllFiles: true}

	// Only f1 complete → partial → unmet.
	st.fileCompletion[depStoreCKey(sub, "fp")] = map[string]uint64{"f1": 1}
	ok, _, err := Satisfied(st, req, sub)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("partial file coverage (f1 only) must not satisfy")
	}

	// Both files at current rev → satisfied.
	st.fileCompletion[depStoreCKey(sub, "fp")]["f2"] = 1
	ok, _, err = Satisfied(st, req, sub)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("full file coverage must satisfy")
	}

	// One file at stale rev → not satisfied.
	st.fileCompletion[depStoreCKey(sub, "fp")]["f1"] = 0
	ok, _, err = Satisfied(st, req, sub)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("file with stale rev must not satisfy AllFiles requirement")
	}
}

// TestAllSatisfied_CombinesReqs verifies AllSatisfied returns false on first
// unmet requirement and true when all are met.
func TestAllSatisfied_CombinesReqs(t *testing.T) {
	st := newFakeDepStore()
	sub := Subject{Type: "book", ID: "b2"}
	st.depRev[depStoreSubKey(sub)] = 1

	reqs := []Requirement{
		{Kind: ReqOpCompleted, OpType: "fp"},
		{Kind: ReqOpCompleted, OpType: "scan"},
	}

	// Neither satisfied yet.
	ok, unmet, err := AllSatisfied(st, reqs, sub)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected not satisfied")
	}
	if unmet == "" {
		t.Fatal("expected non-empty firstUnmet")
	}

	// Satisfy only fp.
	st.completion[depStoreCKey(sub, "fp")] = 1
	ok, unmet, err = AllSatisfied(st, reqs, sub)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("scan still unmet, expected not satisfied")
	}
	if unmet == "" {
		t.Fatal("expected non-empty firstUnmet when scan is unmet")
	}

	// Satisfy both.
	st.completion[depStoreCKey(sub, "scan")] = 1
	ok, _, err = AllSatisfied(st, reqs, sub)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("both requirements met, expected satisfied")
	}
}

// TestReqFieldSet_NotImplemented_M1 verifies that field_set requirements
// always return (false, "not implemented in M1", nil) — they should not block
// compilation and should give a clear non-error signal.
func TestReqFieldSet_NotImplemented_M1(t *testing.T) {
	st := newFakeDepStore()
	sub := Subject{Type: "book", ID: "b3"}
	req := Requirement{Kind: ReqFieldSet, Field: "narrator"}

	ok, reason, err := Satisfied(st, req, sub)
	if err != nil {
		t.Fatalf("field_set should not return an error in M1, got: %v", err)
	}
	if ok {
		t.Fatal("field_set must not satisfy in M1")
	}
	if reason == "" {
		t.Fatal("expected non-empty reason for field_set not-implemented stub")
	}
}

// TestCycleDetection verifies that CheckRequirementCycle catches a direct A→B→A cycle.
func TestCycleDetection(t *testing.T) {
	defs := map[string][]Requirement{
		"a": {{Kind: ReqOpCompleted, OpType: "b"}},
		"b": {{Kind: ReqOpCompleted, OpType: "a"}},
	}
	if err := CheckRequirementCycle(defs); err == nil {
		t.Fatal("expected cycle error for A→B→A")
	}
}

// TestCycleDetection_NoCycle verifies a valid linear chain does not trigger cycle error.
func TestCycleDetection_NoCycle(t *testing.T) {
	defs := map[string][]Requirement{
		"a": {},
		"b": {{Kind: ReqOpCompleted, OpType: "a"}},
		"c": {{Kind: ReqOpCompleted, OpType: "b"}},
	}
	if err := CheckRequirementCycle(defs); err != nil {
		t.Fatalf("expected no cycle, got: %v", err)
	}
}

// TestCycleDetection_SelfLoop verifies that a self-referential op is caught.
func TestCycleDetection_SelfLoop(t *testing.T) {
	defs := map[string][]Requirement{
		"a": {{Kind: ReqOpCompleted, OpType: "a"}},
	}
	if err := CheckRequirementCycle(defs); err == nil {
		t.Fatal("expected cycle error for self-loop A→A")
	}
}

// TestCycleDetection_LongChain verifies a longer cycle is detected.
func TestCycleDetection_LongChain(t *testing.T) {
	// a→b→c→d→a is a cycle of length 4.
	defs := map[string][]Requirement{
		"a": {{Kind: ReqOpCompleted, OpType: "b"}},
		"b": {{Kind: ReqOpCompleted, OpType: "c"}},
		"c": {{Kind: ReqOpCompleted, OpType: "d"}},
		"d": {{Kind: ReqOpCompleted, OpType: "a"}},
	}
	if err := CheckRequirementCycle(defs); err == nil {
		t.Fatal("expected cycle error for a→b→c→d→a")
	}
}

// TestCycleDetection_IgnoresFieldSet verifies that ReqFieldSet requirements
// are not traversed in cycle detection (they have no OpType graph edge).
func TestCycleDetection_IgnoresFieldSet(t *testing.T) {
	defs := map[string][]Requirement{
		"a": {{Kind: ReqFieldSet, Field: "narrator"}},
		"b": {{Kind: ReqOpCompleted, OpType: "a"}},
	}
	if err := CheckRequirementCycle(defs); err != nil {
		t.Fatalf("field_set req should not create a graph edge, got: %v", err)
	}
}

// TestCycleDetection_CycleErrorNamesNode verifies the error message includes
// one of the cycle nodes for debuggability.
func TestCycleDetection_CycleErrorNamesNode(t *testing.T) {
	defs := map[string][]Requirement{
		"x": {{Kind: ReqOpCompleted, OpType: "y"}},
		"y": {{Kind: ReqOpCompleted, OpType: "x"}},
	}
	err := CheckRequirementCycle(defs)
	if err == nil {
		t.Fatal("expected cycle error")
	}
	errStr := err.Error()
	if errStr == "" {
		t.Fatal("cycle error must have a non-empty message")
	}
	// The message must mention "cycle" or one of the involved node names.
	found := false
	for _, word := range []string{"cycle", "x", "y"} {
		if len(errStr) > 0 && contains(errStr, word) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("cycle error message %q should mention the cycle or node names", errStr)
	}
}

// contains is a simple substring check (avoid importing strings just for tests).
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsHelper(s, sub))
}

func containsHelper(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// Compile-time guard: fakeDepStore must implement DepStore.
var _ DepStore = (*fakeDepStore)(nil)

// Compile-time guard: Satisfied, AllSatisfied, CheckRequirementCycle must exist.
var _ = fmt.Sprintf // suppress unused import if guards are removed
