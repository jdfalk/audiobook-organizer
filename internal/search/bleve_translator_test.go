// file: internal/search/bleve_translator_test.go
// version: 1.0.0
// guid: 1a8c2f4d-5b9e-4f70-a7d6-2e8d0f1b9a57

package search

import (
	"path/filepath"
	"testing"
)

func translate(t *testing.T, q string) (hits []SearchResult, total uint64, filters []PerUserFilter) {
	t.Helper()
	idx, err := Open(filepath.Join(t.TempDir(), "b"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = idx.Close() })

	// Seed a small library.
	docs := []BookDocument{
		{BookID: "b1", Title: "The Way of Kings", Author: "Brandon Sanderson", Series: "Stormlight Archive", SeriesNumber: 1, Year: 2010, Format: "m4b", Tags: []string{"epic", "favorite"}},
		{BookID: "b2", Title: "Words of Radiance", Author: "Brandon Sanderson", Series: "Stormlight Archive", SeriesNumber: 2, Year: 2014, Format: "m4b"},
		{BookID: "b3", Title: "Vampire Hunter", Author: "Jane Smith", Year: 2020, Format: "mp3", Tags: []string{"horror"}},
		{BookID: "b4", Title: "New Dawn", Author: "Jane Smyth", Year: 1995, Format: "mp3"},
	}
	for _, d := range docs {
		if err := idx.IndexBook(d); err != nil {
			t.Fatalf("index %s: %v", d.BookID, err)
		}
	}

	ast, err := ParseQuery(q)
	if err != nil {
		t.Fatalf("parse %q: %v", q, err)
	}
	bq, pu, err := Translate(ast)
	if err != nil {
		t.Fatalf("translate %q: %v", q, err)
	}
	hits, total, err = idx.SearchNative(bq, 0, 50)
	if err != nil {
		t.Fatalf("search %q: %v", q, err)
	}
	return hits, total, pu
}

func TestTranslate_SimpleMatch(t *testing.T) {
	hits, _, _ := translate(t, "author:sanderson")
	if len(hits) != 2 {
		t.Errorf("author:sanderson → %d hits, want 2", len(hits))
	}
}

func TestTranslate_And(t *testing.T) {
	hits, _, _ := translate(t, "author:sanderson series:stormlight")
	if len(hits) != 2 {
		t.Errorf("AND → %d hits, want 2", len(hits))
	}
}

func TestTranslate_Or(t *testing.T) {
	hits, _, _ := translate(t, "author:sanderson || author:smith")
	if len(hits) != 3 {
		t.Errorf("OR → %d hits, want 3 (both Sandersons + Smith)", len(hits))
	}
}

func TestTranslate_Not(t *testing.T) {
	// Everything except Sanderson.
	hits, _, _ := translate(t, "-author:sanderson")
	if len(hits) != 2 {
		t.Errorf("NOT → %d hits, want 2", len(hits))
	}
}

func TestTranslate_ValueAlt(t *testing.T) {
	hits, _, _ := translate(t, `format:(m4b|mp3)`)
	if len(hits) != 4 {
		t.Errorf("value-alt → %d hits, want 4 (all books)", len(hits))
	}
}

func TestTranslate_NumericRange(t *testing.T) {
	hits, _, _ := translate(t, "year:>2000 year:<2020")
	// b1 (2010) + b2 (2014)
	if len(hits) != 2 {
		t.Errorf("range → %d hits, want 2; got %v", len(hits), hitIDs(hits))
	}
}

func TestTranslate_NumericInclusiveRange(t *testing.T) {
	hits, _, _ := translate(t, "year:[2010 TO 2020]")
	// b1 (2010), b2 (2014), b3 (2020)
	if len(hits) != 3 {
		t.Errorf("[2010 TO 2020] → %d hits, want 3", len(hits))
	}
}

func TestTranslate_Prefix(t *testing.T) {
	hits, _, _ := translate(t, "author:sand*")
	if len(hits) != 2 {
		t.Errorf("prefix → %d hits, want 2 (Sandersons); got %v", len(hits), hitIDs(hits))
	}
}

func TestTranslate_Fuzzy(t *testing.T) {
	// smith~ should match both "Smith" and "Smyth" (edit distance 2).
	hits, _, _ := translate(t, "author:smith~")
	if len(hits) < 1 {
		t.Errorf("fuzzy → %d hits, want ≥1", len(hits))
	}
}

func TestTranslate_FreeText(t *testing.T) {
	hits, _, _ := translate(t, "vampire")
	if len(hits) < 1 {
		t.Errorf("free text → %d hits, want ≥1 (Vampire Hunter)", len(hits))
	}
}

func TestTranslate_Group(t *testing.T) {
	// (-author:sanderson) && year:>2000 — excludes both Sandersons AND
	// excludes the 1995 book, so only "Vampire Hunter" (2020).
	hits, _, _ := translate(t, "-author:sanderson && year:>2000")
	if len(hits) != 1 || hits[0].BookID != "b3" {
		t.Errorf("group → %v, want [b3]", hitIDs(hits))
	}
}

func TestTranslate_PerUserFieldSplit(t *testing.T) {
	// read_status is per-user → goes to post-filter, NOT Bleve.
	// Remaining Bleve query: author:sanderson (2 hits).
	hits, _, filters := translate(t, "author:sanderson read_status:finished")
	if len(hits) != 2 {
		t.Errorf("Bleve hits = %d, want 2 (per-user field split out)", len(hits))
	}
	if len(filters) != 1 {
		t.Fatalf("PerUserFilter count = %d, want 1", len(filters))
	}
	if filters[0].Node.Field != "read_status" {
		t.Errorf("filter field = %q, want read_status", filters[0].Node.Field)
	}
	if filters[0].Negated {
		t.Error("filter should not be negated")
	}
}

func TestTranslate_PerUserNegated(t *testing.T) {
	_, _, filters := translate(t, "-read_status:finished")
	if len(filters) != 1 {
		t.Fatalf("PerUserFilter count = %d, want 1", len(filters))
	}
	if !filters[0].Negated {
		t.Error("filter should be negated by surrounding NOT")
	}
}

func TestTranslate_PhraseMatch(t *testing.T) {
	hits, _, _ := translate(t, `title:"New Dawn"`)
	if len(hits) != 1 || hits[0].BookID != "b4" {
		t.Errorf("phrase → %v, want [b4]", hitIDs(hits))
	}
}

func TestTranslate_Empty(t *testing.T) {
	// A completely empty / match-all query through the translator.
	q, pu, err := Translate(nil)
	if err != nil {
		t.Fatalf("translate nil: %v", err)
	}
	if q == nil {
		t.Error("nil AST should produce match-all, not nil")
	}
	if len(pu) != 0 {
		t.Errorf("per-user = %d, want 0", len(pu))
	}
}
