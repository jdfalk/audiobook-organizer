// file: internal/dedup/dataset/builder_test.go
// version: 1.1.1
// guid: b3e7f2a1-9c45-4d80-8e62-5f1a3d6c7b90
// last-edited: 2026-06-13

package dataset

import (
	"encoding/base64"
	"encoding/binary"
	"testing"

	"github.com/falkcorp/audiobook-organizer/internal/database"
)

// makeTestSig builds a valid BookSigV1 string: 4096 uint32 words encoded as
// little-endian bytes then base64. All words are set to the given seed value
// so two sigs with the same seed are identical (sim=1.0) and two with different
// seeds are fully dissimilar (sim=0.0 when seed bits are all-1 vs all-0).
func makeTestSig(seed uint32) string {
	const n = 4096
	buf := make([]byte, n*4)
	for i := 0; i < n; i++ {
		binary.LittleEndian.PutUint32(buf[i*4:], seed)
	}
	return base64.StdEncoding.EncodeToString(buf)
}

// fakeStore implements BuilderStore for tests.
type fakeStore struct {
	books map[string]*database.Book
	files map[string][]database.BookFile
}

func (f *fakeStore) GetBook(id string) (*database.Book, error)           { return f.books[id], nil }
func (f *fakeStore) GetBookFiles(id string) ([]database.BookFile, error) { return f.files[id], nil }

func TestBuildExample_PartVsWholeFeatures(t *testing.T) {
	whole := &database.Book{ID: "whole", Title: "The Crafter's Defense"}
	part := &database.Book{ID: "part", Title: "The Crafter's Defense"}
	fs := &fakeStore{
		books: map[string]*database.Book{"whole": whole, "part": part},
		files: map[string][]database.BookFile{
			"whole": {{BookID: "whole", FilePath: "/lib/Crafter/whole.m4b", Duration: 36000}},
			"part":  {{BookID: "part", FilePath: "/lib/Crafter/part-1.m4b", Duration: 1200}},
		},
	}
	cand := database.DedupCandidate{ID: 7, EntityAID: "whole", EntityBID: "part", Layer: "exact"}

	ex, err := BuildExample(fs, cand)
	if err != nil {
		t.Fatalf("BuildExample: %v", err)
	}
	if ex.A.TotalDurationSec != 36000 || ex.B.TotalDurationSec != 1200 {
		t.Fatalf("durations: a=%v b=%v", ex.A.TotalDurationSec, ex.B.TotalDurationSec)
	}
	wantRatio := 1200.0 / 36000.0
	if ex.DurationRatio < wantRatio-1e-9 || ex.DurationRatio > wantRatio+1e-9 {
		t.Fatalf("duration ratio = %v, want %v", ex.DurationRatio, wantRatio)
	}
	if ex.FolderRelation != "same_dir" {
		t.Fatalf("folder relation = %q, want same_dir", ex.FolderRelation)
	}
}

func TestBuildExample_CandidateFieldsCarried(t *testing.T) {
	sim := 0.97
	bk := &database.Book{ID: "a", Title: "Test"}
	fs := &fakeStore{
		books: map[string]*database.Book{"a": bk, "b": bk},
		files: map[string][]database.BookFile{},
	}
	cand := database.DedupCandidate{ID: 99, EntityAID: "a", EntityBID: "b", Layer: "lsh", Band: "HIGH", Similarity: &sim}

	ex, err := BuildExample(fs, cand)
	if err != nil {
		t.Fatalf("BuildExample: %v", err)
	}
	if ex.CandidateID != 99 || ex.Layer != "lsh" || ex.Band != "HIGH" {
		t.Fatalf("candidate fields not carried: %+v", ex)
	}
	if ex.Similarity == nil || *ex.Similarity != sim {
		t.Fatalf("similarity not carried: %v", ex.Similarity)
	}
}

func TestBuildExample_RecordingIDSharing(t *testing.T) {
	bkA := &database.Book{ID: "a", Title: "Shared"}
	bkB := &database.Book{ID: "b", Title: "Shared"}
	fs := &fakeStore{
		books: map[string]*database.Book{"a": bkA, "b": bkB},
		files: map[string][]database.BookFile{
			"a": {{BookID: "a", FilePath: "/x/a.m4b", Duration: 3600, AcoustIDOnlineRecordingID: "mbid-123"}},
			"b": {{BookID: "b", FilePath: "/y/b.m4b", Duration: 3600, AcoustIDOnlineRecordingID: "mbid-123"}},
		},
	}
	cand := database.DedupCandidate{ID: 1, EntityAID: "a", EntityBID: "b", Layer: "lsh"}

	ex, err := BuildExample(fs, cand)
	if err != nil {
		t.Fatalf("BuildExample: %v", err)
	}
	if !ex.SharesRecordingID {
		t.Fatal("expected SharesRecordingID=true when both sides have the same AcoustID recording ID")
	}
}

func TestBuildExample_HasCover(t *testing.T) {
	coverURL := "https://example.com/cover.jpg"
	bkA := &database.Book{ID: "a", Title: "Cover Book", CoverURL: &coverURL}
	bkB := &database.Book{ID: "b", Title: "No Cover Book"}
	fs := &fakeStore{
		books: map[string]*database.Book{"a": bkA, "b": bkB},
		files: map[string][]database.BookFile{},
	}
	cand := database.DedupCandidate{ID: 5, EntityAID: "a", EntityBID: "b", Layer: "exact"}

	ex, err := BuildExample(fs, cand)
	if err != nil {
		t.Fatalf("BuildExample: %v", err)
	}
	if !ex.A.HasCover {
		t.Fatal("expected A.HasCover=true")
	}
	if ex.B.HasCover {
		t.Fatal("expected B.HasCover=false")
	}
}

func TestBuildExample_SignatureRelation_Match(t *testing.T) {
	// Two books with identical 4096-uint32 sigs → sim=1.0 → "match"
	sig := makeTestSig(0xDEADBEEF)
	bkA := &database.Book{ID: "a", Title: "Same Book", BookSigV1: &sig}
	bkB := &database.Book{ID: "b", Title: "Same Book", BookSigV1: &sig}
	fs := &fakeStore{
		books: map[string]*database.Book{"a": bkA, "b": bkB},
		files: map[string][]database.BookFile{},
	}
	cand := database.DedupCandidate{ID: 10, EntityAID: "a", EntityBID: "b", Layer: "lsh"}

	ex, err := BuildExample(fs, cand)
	if err != nil {
		t.Fatalf("BuildExample: %v", err)
	}
	if ex.SignatureRelation != "match" {
		t.Fatalf("SignatureRelation = %q, want match (identical sigs)", ex.SignatureRelation)
	}
}

func TestBuildExample_SignatureRelation_Disjoint(t *testing.T) {
	// seed=0x00000000 gives all-zero words; seed=0xFFFFFFFF gives all-one words.
	// XOR of these is all-ones → every bit differs → sim=0.0 → "disjoint"
	sigA := makeTestSig(0x00000000)
	sigB := makeTestSig(0xFFFFFFFF)
	bkA := &database.Book{ID: "a", Title: "Book A", BookSigV1: &sigA}
	bkB := &database.Book{ID: "b", Title: "Book B", BookSigV1: &sigB}
	fs := &fakeStore{
		books: map[string]*database.Book{"a": bkA, "b": bkB},
		files: map[string][]database.BookFile{},
	}
	cand := database.DedupCandidate{ID: 11, EntityAID: "a", EntityBID: "b", Layer: "lsh"}

	ex, err := BuildExample(fs, cand)
	if err != nil {
		t.Fatalf("BuildExample: %v", err)
	}
	if ex.SignatureRelation != "disjoint" {
		t.Fatalf("SignatureRelation = %q, want disjoint (fully dissimilar sigs)", ex.SignatureRelation)
	}
}

func TestBuildExample_SignatureRelation_Unknown_NoSig(t *testing.T) {
	// Book with no BookSigV1 → "unknown"
	bkA := &database.Book{ID: "a", Title: "No Sig"}
	bkB := &database.Book{ID: "b", Title: "No Sig"}
	fs := &fakeStore{
		books: map[string]*database.Book{"a": bkA, "b": bkB},
		files: map[string][]database.BookFile{},
	}
	cand := database.DedupCandidate{ID: 12, EntityAID: "a", EntityBID: "b", Layer: "lsh"}

	ex, err := BuildExample(fs, cand)
	if err != nil {
		t.Fatalf("BuildExample: %v", err)
	}
	if ex.SignatureRelation != "unknown" {
		t.Fatalf("SignatureRelation = %q, want unknown (no sigs)", ex.SignatureRelation)
	}
}

func TestBuildExample_FolderRelation_Ancestor(t *testing.T) {
	bkA := &database.Book{ID: "a", Title: "Parent"}
	bkB := &database.Book{ID: "b", Title: "Child"}
	fs := &fakeStore{
		books: map[string]*database.Book{"a": bkA, "b": bkB},
		files: map[string][]database.BookFile{
			"a": {{BookID: "a", FilePath: "/lib/Series/whole.m4b", Duration: 36000}},
			"b": {{BookID: "b", FilePath: "/lib/Series/Part1/part.m4b", Duration: 3600}},
		},
	}
	cand := database.DedupCandidate{ID: 2, EntityAID: "a", EntityBID: "b", Layer: "lsh"}

	ex, err := BuildExample(fs, cand)
	if err != nil {
		t.Fatalf("BuildExample: %v", err)
	}
	if ex.FolderRelation != "a_ancestor_of_b" {
		t.Fatalf("folder relation = %q, want a_ancestor_of_b", ex.FolderRelation)
	}
}

func TestBuildExample_FolderRelation_BAncestorOfA(t *testing.T) {
	// B's directory is an ancestor of A's path → b_ancestor_of_a.
	bkA := &database.Book{ID: "a", Title: "Child"}
	bkB := &database.Book{ID: "b", Title: "Parent"}
	fs := &fakeStore{
		books: map[string]*database.Book{"a": bkA, "b": bkB},
		files: map[string][]database.BookFile{
			"a": {{BookID: "a", FilePath: "/lib/Series/Part1/part.m4b", Duration: 3600}},
			"b": {{BookID: "b", FilePath: "/lib/Series/whole.m4b", Duration: 36000}},
		},
	}
	cand := database.DedupCandidate{ID: 3, EntityAID: "a", EntityBID: "b", Layer: "lsh"}

	ex, err := BuildExample(fs, cand)
	if err != nil {
		t.Fatalf("BuildExample: %v", err)
	}
	if ex.FolderRelation != "b_ancestor_of_a" {
		t.Fatalf("folder relation = %q, want b_ancestor_of_a", ex.FolderRelation)
	}
}

func TestBuildExample_FolderRelation_Unrelated(t *testing.T) {
	// Two completely disjoint directories → unrelated.
	bkA := &database.Book{ID: "a", Title: "Book A"}
	bkB := &database.Book{ID: "b", Title: "Book B"}
	fs := &fakeStore{
		books: map[string]*database.Book{"a": bkA, "b": bkB},
		files: map[string][]database.BookFile{
			"a": {{BookID: "a", FilePath: "/lib/AuthorOne/book.m4b", Duration: 3600}},
			"b": {{BookID: "b", FilePath: "/lib/AuthorTwo/book.m4b", Duration: 3600}},
		},
	}
	cand := database.DedupCandidate{ID: 4, EntityAID: "a", EntityBID: "b", Layer: "lsh"}

	ex, err := BuildExample(fs, cand)
	if err != nil {
		t.Fatalf("BuildExample: %v", err)
	}
	if ex.FolderRelation != "unrelated" {
		t.Fatalf("folder relation = %q, want unrelated", ex.FolderRelation)
	}
}

func TestBuildExample_FolderRelation_PrefixNotAncestor(t *testing.T) {
	// /lib/Series is NOT an ancestor of /lib/Series-Extra — the trailing-slash
	// guard in isAncestor must prevent a false positive here.
	bkA := &database.Book{ID: "a", Title: "Book"}
	bkB := &database.Book{ID: "b", Title: "Book Extra"}
	fs := &fakeStore{
		books: map[string]*database.Book{"a": bkA, "b": bkB},
		files: map[string][]database.BookFile{
			"a": {{BookID: "a", FilePath: "/lib/Series/book.m4b", Duration: 3600}},
			"b": {{BookID: "b", FilePath: "/lib/Series-Extra/book.m4b", Duration: 3600}},
		},
	}
	cand := database.DedupCandidate{ID: 5, EntityAID: "a", EntityBID: "b", Layer: "lsh"}

	ex, err := BuildExample(fs, cand)
	if err != nil {
		t.Fatalf("BuildExample: %v", err)
	}
	if ex.FolderRelation != "unrelated" {
		t.Fatalf("folder relation = %q, want unrelated (Series is not an ancestor of Series-Extra)", ex.FolderRelation)
	}
}
