// file: internal/dedup/dataset/builder_test.go
// version: 1.0.0
// guid: b3e7f2a1-9c45-4d80-8e62-5f1a3d6c7b90
// last-edited: 2026-06-13

package dataset

import (
	"testing"

	"github.com/falkcorp/audiobook-organizer/internal/database"
)

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
