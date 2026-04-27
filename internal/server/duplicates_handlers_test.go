// file: internal/server/duplicates_handlers_test.go
// version: 1.0.0
// guid: 9c1e2f3a-4b5d-6e7f-8a9b-0c1d2e3f4a5b

package server

import (
	"fmt"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

func TestComputeSeriesNormalizeActions_Basic(t *testing.T) {
	authorID := 1
	store := &database.MockStore{}
	store.GetAllSeriesFunc = func() ([]database.Series, error) {
		return []database.Series{
			{ID: 1, Name: "The Long Earth One", AuthorID: &authorID},
			{ID: 2, Name: "The Long Earth Two", AuthorID: &authorID},
			{ID: 3, Name: "Discworld", AuthorID: &authorID},
		}, nil
	}
	store.GetBooksBySeriesIDFunc = func(id int) ([]database.Book, error) {
		return []database.Book{{ID: fmt.Sprintf("book-%d", id)}}, nil
	}

	actions := computeSeriesNormalizeActions(store)

	for _, a := range actions {
		if a.OldName == "Discworld" {
			t.Errorf("clean series Discworld should not appear in actions")
		}
	}
	if len(actions) != 2 {
		t.Errorf("expected 2 actions, got %d", len(actions))
	}
}
