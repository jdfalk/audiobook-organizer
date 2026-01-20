// file: internal/database/mock_store_test.go
// version: 1.0.0
// guid: 4d5e6f7a-8b9c-0d1e-2f3a-4b5c6d7e8f9a

package database

import (
	"errors"
	"testing"
	"time"
)

func TestNewMockStore(t *testing.T) {
	store := NewMockStore()
	if store == nil {
		t.Fatal("NewMockStore returned nil")
	}
	if store.Books == nil {
		t.Error("Books map not initialized")
	}
	if store.Authors == nil {
		t.Error("Authors map not initialized")
	}
	if store.NextAuthorID != 1 {
		t.Error("NextAuthorID not initialized to 1")
	}
}

func TestMockStore_Close(t *testing.T) {
	store := NewMockStore()

	err := store.Close()
	if err != nil {
		t.Errorf("Close returned error: %v", err)
	}

	// Test error injection
	store.ErrorOnNext["Close"] = errors.New("close error")
	err = store.Close()
	if err == nil {
		t.Error("expected error from Close")
	}
}

func TestMockStore_Authors(t *testing.T) {
	store := NewMockStore()

	t.Run("CreateAuthor", func(t *testing.T) {
		author, err := store.CreateAuthor("Test Author")
		if err != nil {
			t.Fatalf("CreateAuthor failed: %v", err)
		}
		if author.Name != "Test Author" {
			t.Errorf("expected name 'Test Author', got '%s'", author.Name)
		}
		if author.ID != 1 {
			t.Errorf("expected ID 1, got %d", author.ID)
		}
	})

	t.Run("GetAuthorByID", func(t *testing.T) {
		author, err := store.GetAuthorByID(1)
		if err != nil {
			t.Fatalf("GetAuthorByID failed: %v", err)
		}
		if author.Name != "Test Author" {
			t.Errorf("expected name 'Test Author', got '%s'", author.Name)
		}
	})

	t.Run("GetAuthorByID_NotFound", func(t *testing.T) {
		_, err := store.GetAuthorByID(999)
		if err == nil {
			t.Error("expected error for non-existent author")
		}
	})

	t.Run("GetAuthorByName", func(t *testing.T) {
		author, err := store.GetAuthorByName("Test Author")
		if err != nil {
			t.Fatalf("GetAuthorByName failed: %v", err)
		}
		if author.ID != 1 {
			t.Errorf("expected ID 1, got %d", author.ID)
		}
	})

	t.Run("GetAuthorByName_NotFound", func(t *testing.T) {
		_, err := store.GetAuthorByName("Non-existent")
		if err == nil {
			t.Error("expected error for non-existent author")
		}
	})

	t.Run("GetAllAuthors", func(t *testing.T) {
		authors, err := store.GetAllAuthors()
		if err != nil {
			t.Fatalf("GetAllAuthors failed: %v", err)
		}
		if len(authors) != 1 {
			t.Errorf("expected 1 author, got %d", len(authors))
		}
	})

	t.Run("Error injection", func(t *testing.T) {
		store.ErrorOnNext["CreateAuthor"] = errors.New("create error")
		_, err := store.CreateAuthor("Another")
		if err == nil {
			t.Error("expected error from CreateAuthor")
		}
	})
}

func TestMockStore_Series(t *testing.T) {
	store := NewMockStore()
	authorID := 1

	t.Run("CreateSeries", func(t *testing.T) {
		series, err := store.CreateSeries("Test Series", &authorID)
		if err != nil {
			t.Fatalf("CreateSeries failed: %v", err)
		}
		if series.Name != "Test Series" {
			t.Errorf("expected name 'Test Series', got '%s'", series.Name)
		}
	})

	t.Run("GetSeriesByID", func(t *testing.T) {
		series, err := store.GetSeriesByID(1)
		if err != nil {
			t.Fatalf("GetSeriesByID failed: %v", err)
		}
		if series.Name != "Test Series" {
			t.Errorf("expected name 'Test Series', got '%s'", series.Name)
		}
	})

	t.Run("GetSeriesByName", func(t *testing.T) {
		series, err := store.GetSeriesByName("Test Series", nil)
		if err != nil {
			t.Fatalf("GetSeriesByName failed: %v", err)
		}
		if series.ID != 1 {
			t.Errorf("expected ID 1, got %d", series.ID)
		}
	})

	t.Run("GetAllSeries", func(t *testing.T) {
		seriesList, err := store.GetAllSeries()
		if err != nil {
			t.Fatalf("GetAllSeries failed: %v", err)
		}
		if len(seriesList) != 1 {
			t.Errorf("expected 1 series, got %d", len(seriesList))
		}
	})
}

func TestMockStore_Books(t *testing.T) {
	store := NewMockStore()

	t.Run("CreateBook", func(t *testing.T) {
		book := &Book{Title: "Test Book", FilePath: "/test/book.m4b"}
		created, err := store.CreateBook(book)
		if err != nil {
			t.Fatalf("CreateBook failed: %v", err)
		}
		if created.Title != "Test Book" {
			t.Errorf("expected title 'Test Book', got '%s'", created.Title)
		}
		if created.ID == "" {
			t.Error("expected ID to be assigned")
		}
	})

	t.Run("GetBookByID", func(t *testing.T) {
		book, err := store.GetBookByID("book-1")
		if err != nil {
			t.Fatalf("GetBookByID failed: %v", err)
		}
		if book.Title != "Test Book" {
			t.Errorf("expected title 'Test Book', got '%s'", book.Title)
		}
	})

	t.Run("GetBookByFilePath", func(t *testing.T) {
		book, err := store.GetBookByFilePath("/test/book.m4b")
		if err != nil {
			t.Fatalf("GetBookByFilePath failed: %v", err)
		}
		if book.Title != "Test Book" {
			t.Errorf("expected title 'Test Book', got '%s'", book.Title)
		}
	})

	t.Run("UpdateBook", func(t *testing.T) {
		book := &Book{Title: "Updated Title"}
		updated, err := store.UpdateBook("book-1", book)
		if err != nil {
			t.Fatalf("UpdateBook failed: %v", err)
		}
		if updated.Title != "Updated Title" {
			t.Errorf("expected title 'Updated Title', got '%s'", updated.Title)
		}
	})

	t.Run("GetAllBooks", func(t *testing.T) {
		books, err := store.GetAllBooks(10, 0)
		if err != nil {
			t.Fatalf("GetAllBooks failed: %v", err)
		}
		if len(books) != 1 {
			t.Errorf("expected 1 book, got %d", len(books))
		}
	})

	t.Run("CountBooks", func(t *testing.T) {
		count, err := store.CountBooks()
		if err != nil {
			t.Fatalf("CountBooks failed: %v", err)
		}
		if count != 1 {
			t.Errorf("expected count 1, got %d", count)
		}
	})

	t.Run("DeleteBook", func(t *testing.T) {
		err := store.DeleteBook("book-1")
		if err != nil {
			t.Fatalf("DeleteBook failed: %v", err)
		}
		_, err = store.GetBookByID("book-1")
		if err == nil {
			t.Error("expected error after deletion")
		}
	})

	t.Run("SearchBooks", func(t *testing.T) {
		books, err := store.SearchBooks("test", 10, 0)
		if err != nil {
			t.Fatalf("SearchBooks failed: %v", err)
		}
		// Returns empty by default
		if books == nil {
			t.Error("expected non-nil result")
		}
	})
}

func TestMockStore_BooksByHash(t *testing.T) {
	store := NewMockStore()
	hash := "abc123"
	originalHash := "orig123"
	organizedHash := "org123"

	book := &Book{
		Title:             "Hash Test",
		FilePath:          "/test/hash.m4b",
		FileHash:          &hash,
		OriginalFileHash:  &originalHash,
		OrganizedFileHash: &organizedHash,
	}
	_, _ = store.CreateBook(book)

	t.Run("GetBookByFileHash", func(t *testing.T) {
		found, err := store.GetBookByFileHash("abc123")
		if err != nil {
			t.Fatalf("GetBookByFileHash failed: %v", err)
		}
		if found.Title != "Hash Test" {
			t.Error("wrong book returned")
		}
	})

	t.Run("GetBookByOriginalHash", func(t *testing.T) {
		found, err := store.GetBookByOriginalHash("orig123")
		if err != nil {
			t.Fatalf("GetBookByOriginalHash failed: %v", err)
		}
		if found.Title != "Hash Test" {
			t.Error("wrong book returned")
		}
	})

	t.Run("GetBookByOrganizedHash", func(t *testing.T) {
		found, err := store.GetBookByOrganizedHash("org123")
		if err != nil {
			t.Fatalf("GetBookByOrganizedHash failed: %v", err)
		}
		if found.Title != "Hash Test" {
			t.Error("wrong book returned")
		}
	})
}

func TestMockStore_Operations(t *testing.T) {
	store := NewMockStore()

	t.Run("CreateOperation", func(t *testing.T) {
		folder := "/test/folder"
		op, err := store.CreateOperation("op-1", "scan", &folder)
		if err != nil {
			t.Fatalf("CreateOperation failed: %v", err)
		}
		if op.Type != "scan" {
			t.Error("wrong operation type")
		}
	})

	t.Run("GetOperationByID", func(t *testing.T) {
		op, err := store.GetOperationByID("op-1")
		if err != nil {
			t.Fatalf("GetOperationByID failed: %v", err)
		}
		if op.Type != "scan" {
			t.Error("wrong operation type")
		}
	})

	t.Run("UpdateOperationStatus", func(t *testing.T) {
		err := store.UpdateOperationStatus("op-1", "running", 5, 10, "in progress")
		if err != nil {
			t.Fatalf("UpdateOperationStatus failed: %v", err)
		}
		op, _ := store.GetOperationByID("op-1")
		if op.Status != "running" {
			t.Error("status not updated")
		}
		if op.Progress != 5 {
			t.Error("progress not updated")
		}
	})

	t.Run("UpdateOperationStatus creates if missing", func(t *testing.T) {
		err := store.UpdateOperationStatus("new-op", "queued", 0, 0, "starting")
		if err != nil {
			t.Fatalf("UpdateOperationStatus failed: %v", err)
		}
		op, err := store.GetOperationByID("new-op")
		if err != nil {
			t.Fatalf("operation should exist: %v", err)
		}
		if op.Status != "queued" {
			t.Error("status not set")
		}
	})

	t.Run("UpdateOperationError", func(t *testing.T) {
		err := store.UpdateOperationError("op-1", "test error")
		if err != nil {
			t.Fatalf("UpdateOperationError failed: %v", err)
		}
		op, _ := store.GetOperationByID("op-1")
		if op.Status != "failed" {
			t.Error("status should be failed")
		}
	})

	t.Run("GetRecentOperations", func(t *testing.T) {
		ops, err := store.GetRecentOperations(10)
		if err != nil {
			t.Fatalf("GetRecentOperations failed: %v", err)
		}
		if len(ops) == 0 {
			t.Error("expected operations")
		}
	})

	t.Run("AddOperationLog", func(t *testing.T) {
		details := "some details"
		err := store.AddOperationLog("op-1", "info", "test message", &details)
		if err != nil {
			t.Fatalf("AddOperationLog failed: %v", err)
		}
	})

	t.Run("GetOperationLogs", func(t *testing.T) {
		logs, err := store.GetOperationLogs("op-1")
		if err != nil {
			t.Fatalf("GetOperationLogs failed: %v", err)
		}
		if len(logs) != 1 {
			t.Errorf("expected 1 log, got %d", len(logs))
		}
	})
}

func TestMockStore_Settings(t *testing.T) {
	store := NewMockStore()

	t.Run("SetSetting", func(t *testing.T) {
		err := store.SetSetting("test_key", "test_value", "string", false)
		if err != nil {
			t.Fatalf("SetSetting failed: %v", err)
		}
	})

	t.Run("GetSetting", func(t *testing.T) {
		setting, err := store.GetSetting("test_key")
		if err != nil {
			t.Fatalf("GetSetting failed: %v", err)
		}
		if setting.Value != "test_value" {
			t.Error("wrong value")
		}
	})

	t.Run("GetAllSettings", func(t *testing.T) {
		settings, err := store.GetAllSettings()
		if err != nil {
			t.Fatalf("GetAllSettings failed: %v", err)
		}
		if len(settings) != 1 {
			t.Errorf("expected 1 setting, got %d", len(settings))
		}
	})

	t.Run("DeleteSetting", func(t *testing.T) {
		err := store.DeleteSetting("test_key")
		if err != nil {
			t.Fatalf("DeleteSetting failed: %v", err)
		}
		_, err = store.GetSetting("test_key")
		if err == nil {
			t.Error("expected error after deletion")
		}
	})
}

func TestMockStore_Preferences(t *testing.T) {
	store := NewMockStore()

	t.Run("SetUserPreference", func(t *testing.T) {
		err := store.SetUserPreference("theme", "dark")
		if err != nil {
			t.Fatalf("SetUserPreference failed: %v", err)
		}
	})

	t.Run("GetUserPreference", func(t *testing.T) {
		pref, err := store.GetUserPreference("theme")
		if err != nil {
			t.Fatalf("GetUserPreference failed: %v", err)
		}
		if *pref.Value != "dark" {
			t.Error("wrong value")
		}
	})

	t.Run("GetAllUserPreferences", func(t *testing.T) {
		prefs, err := store.GetAllUserPreferences()
		if err != nil {
			t.Fatalf("GetAllUserPreferences failed: %v", err)
		}
		if len(prefs) != 1 {
			t.Errorf("expected 1 preference, got %d", len(prefs))
		}
	})
}

func TestMockStore_ImportPaths(t *testing.T) {
	store := NewMockStore()

	t.Run("CreateImportPath", func(t *testing.T) {
		path, err := store.CreateImportPath("/media/audiobooks", "Main Library")
		if err != nil {
			t.Fatalf("CreateImportPath failed: %v", err)
		}
		if path.Path != "/media/audiobooks" {
			t.Error("wrong path")
		}
	})

	t.Run("GetImportPathByID", func(t *testing.T) {
		path, err := store.GetImportPathByID(1)
		if err != nil {
			t.Fatalf("GetImportPathByID failed: %v", err)
		}
		if path.Name != "Main Library" {
			t.Error("wrong name")
		}
	})

	t.Run("GetImportPathByPath", func(t *testing.T) {
		path, err := store.GetImportPathByPath("/media/audiobooks")
		if err != nil {
			t.Fatalf("GetImportPathByPath failed: %v", err)
		}
		if path.ID != 1 {
			t.Error("wrong ID")
		}
	})

	t.Run("GetAllImportPaths", func(t *testing.T) {
		paths, err := store.GetAllImportPaths()
		if err != nil {
			t.Fatalf("GetAllImportPaths failed: %v", err)
		}
		if len(paths) != 1 {
			t.Errorf("expected 1 path, got %d", len(paths))
		}
	})

	t.Run("UpdateImportPath", func(t *testing.T) {
		path := &ImportPath{Name: "Updated Name", Path: "/updated/path"}
		err := store.UpdateImportPath(1, path)
		if err != nil {
			t.Fatalf("UpdateImportPath failed: %v", err)
		}
	})

	t.Run("DeleteImportPath", func(t *testing.T) {
		err := store.DeleteImportPath(1)
		if err != nil {
			t.Fatalf("DeleteImportPath failed: %v", err)
		}
	})
}

func TestMockStore_Works(t *testing.T) {
	store := NewMockStore()

	t.Run("CreateWork", func(t *testing.T) {
		work := &Work{Title: "Test Work"}
		created, err := store.CreateWork(work)
		if err != nil {
			t.Fatalf("CreateWork failed: %v", err)
		}
		if created.ID == "" {
			t.Error("ID should be assigned")
		}
	})

	t.Run("GetWorkByID", func(t *testing.T) {
		work, err := store.GetWorkByID("work-1")
		if err != nil {
			t.Fatalf("GetWorkByID failed: %v", err)
		}
		if work.Title != "Test Work" {
			t.Error("wrong title")
		}
	})

	t.Run("UpdateWork", func(t *testing.T) {
		work := &Work{Title: "Updated Work"}
		updated, err := store.UpdateWork("work-1", work)
		if err != nil {
			t.Fatalf("UpdateWork failed: %v", err)
		}
		if updated.Title != "Updated Work" {
			t.Error("title not updated")
		}
	})

	t.Run("GetAllWorks", func(t *testing.T) {
		works, err := store.GetAllWorks()
		if err != nil {
			t.Fatalf("GetAllWorks failed: %v", err)
		}
		if len(works) != 1 {
			t.Errorf("expected 1 work, got %d", len(works))
		}
	})

	t.Run("DeleteWork", func(t *testing.T) {
		err := store.DeleteWork("work-1")
		if err != nil {
			t.Fatalf("DeleteWork failed: %v", err)
		}
	})
}

func TestMockStore_Playlists(t *testing.T) {
	store := NewMockStore()
	seriesID := 1

	t.Run("CreatePlaylist", func(t *testing.T) {
		playlist, err := store.CreatePlaylist("Test Playlist", &seriesID, "/playlist.m3u")
		if err != nil {
			t.Fatalf("CreatePlaylist failed: %v", err)
		}
		if playlist.Name != "Test Playlist" {
			t.Error("wrong name")
		}
	})

	t.Run("GetPlaylistByID", func(t *testing.T) {
		playlist, err := store.GetPlaylistByID(1)
		if err != nil {
			t.Fatalf("GetPlaylistByID failed: %v", err)
		}
		if playlist.Name != "Test Playlist" {
			t.Error("wrong name")
		}
	})

	t.Run("GetPlaylistBySeriesID", func(t *testing.T) {
		playlist, err := store.GetPlaylistBySeriesID(1)
		if err != nil {
			t.Fatalf("GetPlaylistBySeriesID failed: %v", err)
		}
		if playlist.ID != 1 {
			t.Error("wrong ID")
		}
	})

	t.Run("AddPlaylistItem", func(t *testing.T) {
		err := store.AddPlaylistItem(1, 1, 1)
		if err != nil {
			t.Fatalf("AddPlaylistItem failed: %v", err)
		}
	})

	t.Run("GetPlaylistItems", func(t *testing.T) {
		items, err := store.GetPlaylistItems(1)
		if err != nil {
			t.Fatalf("GetPlaylistItems failed: %v", err)
		}
		// Returns empty by default
		if items == nil {
			t.Error("expected non-nil result")
		}
	})
}

func TestMockStore_BlockedHashes(t *testing.T) {
	store := NewMockStore()

	t.Run("AddBlockedHash", func(t *testing.T) {
		err := store.AddBlockedHash("blockedhash123", "duplicate")
		if err != nil {
			t.Fatalf("AddBlockedHash failed: %v", err)
		}
	})

	t.Run("IsHashBlocked", func(t *testing.T) {
		blocked, err := store.IsHashBlocked("blockedhash123")
		if err != nil {
			t.Fatalf("IsHashBlocked failed: %v", err)
		}
		if !blocked {
			t.Error("hash should be blocked")
		}
	})

	t.Run("GetBlockedHashByHash", func(t *testing.T) {
		record, err := store.GetBlockedHashByHash("blockedhash123")
		if err != nil {
			t.Fatalf("GetBlockedHashByHash failed: %v", err)
		}
		if record.Reason != "duplicate" {
			t.Error("wrong reason")
		}
	})

	t.Run("GetAllBlockedHashes", func(t *testing.T) {
		hashes, err := store.GetAllBlockedHashes()
		if err != nil {
			t.Fatalf("GetAllBlockedHashes failed: %v", err)
		}
		if len(hashes) != 1 {
			t.Errorf("expected 1 blocked hash, got %d", len(hashes))
		}
	})

	t.Run("RemoveBlockedHash", func(t *testing.T) {
		err := store.RemoveBlockedHash("blockedhash123")
		if err != nil {
			t.Fatalf("RemoveBlockedHash failed: %v", err)
		}
		blocked, _ := store.IsHashBlocked("blockedhash123")
		if blocked {
			t.Error("hash should not be blocked after removal")
		}
	})
}

func TestMockStore_MetadataFieldStates(t *testing.T) {
	store := NewMockStore()

	t.Run("UpsertMetadataFieldState", func(t *testing.T) {
		state := &MetadataFieldState{
			BookID:         "book-1",
			Field:          "title",
			OverrideLocked: true,
			UpdatedAt:      time.Now(),
		}
		err := store.UpsertMetadataFieldState(state)
		if err != nil {
			t.Fatalf("UpsertMetadataFieldState failed: %v", err)
		}
	})

	t.Run("GetMetadataFieldStates", func(t *testing.T) {
		states, err := store.GetMetadataFieldStates("book-1")
		if err != nil {
			t.Fatalf("GetMetadataFieldStates failed: %v", err)
		}
		if len(states) != 1 {
			t.Errorf("expected 1 state, got %d", len(states))
		}
	})

	t.Run("Upsert updates existing", func(t *testing.T) {
		state := &MetadataFieldState{
			BookID:         "book-1",
			Field:          "title",
			OverrideLocked: false,
			UpdatedAt:      time.Now(),
		}
		err := store.UpsertMetadataFieldState(state)
		if err != nil {
			t.Fatalf("UpsertMetadataFieldState failed: %v", err)
		}
		states, _ := store.GetMetadataFieldStates("book-1")
		if len(states) != 1 {
			t.Error("should update not insert")
		}
		if states[0].OverrideLocked {
			t.Error("should be updated to false")
		}
	})

	t.Run("DeleteMetadataFieldState", func(t *testing.T) {
		err := store.DeleteMetadataFieldState("book-1", "title")
		if err != nil {
			t.Fatalf("DeleteMetadataFieldState failed: %v", err)
		}
		states, _ := store.GetMetadataFieldStates("book-1")
		if len(states) != 0 {
			t.Error("state should be deleted")
		}
	})
}

func TestMockStore_BookQueries(t *testing.T) {
	store := NewMockStore()

	// Create test data
	authorID := 1
	seriesID := 1
	workID := "work-1"
	groupID := "group-1"

	store.CreateAuthor("Test Author")
	store.CreateSeries("Test Series", &authorID)

	book1 := &Book{
		Title:          "Book 1",
		FilePath:       "/book1.m4b",
		AuthorID:       &authorID,
		SeriesID:       &seriesID,
		WorkID:         &workID,
		VersionGroupID: &groupID,
	}
	store.CreateBook(book1)

	book2 := &Book{
		Title:          "Book 2",
		FilePath:       "/book2.m4b",
		AuthorID:       &authorID,
		SeriesID:       &seriesID,
		WorkID:         &workID,
		VersionGroupID: &groupID,
	}
	store.CreateBook(book2)

	t.Run("GetBooksByAuthorID", func(t *testing.T) {
		books, err := store.GetBooksByAuthorID(authorID)
		if err != nil {
			t.Fatalf("GetBooksByAuthorID failed: %v", err)
		}
		if len(books) != 2 {
			t.Errorf("expected 2 books, got %d", len(books))
		}
	})

	t.Run("GetBooksBySeriesID", func(t *testing.T) {
		books, err := store.GetBooksBySeriesID(seriesID)
		if err != nil {
			t.Fatalf("GetBooksBySeriesID failed: %v", err)
		}
		if len(books) != 2 {
			t.Errorf("expected 2 books, got %d", len(books))
		}
	})

	t.Run("GetBooksByWorkID", func(t *testing.T) {
		books, err := store.GetBooksByWorkID(workID)
		if err != nil {
			t.Fatalf("GetBooksByWorkID failed: %v", err)
		}
		if len(books) != 2 {
			t.Errorf("expected 2 books, got %d", len(books))
		}
	})

	t.Run("GetBooksByVersionGroup", func(t *testing.T) {
		books, err := store.GetBooksByVersionGroup(groupID)
		if err != nil {
			t.Fatalf("GetBooksByVersionGroup failed: %v", err)
		}
		if len(books) != 2 {
			t.Errorf("expected 2 books, got %d", len(books))
		}
	})

	t.Run("GetDuplicateBooks", func(t *testing.T) {
		dupes, err := store.GetDuplicateBooks()
		if err != nil {
			t.Fatalf("GetDuplicateBooks failed: %v", err)
		}
		// Returns empty by default
		if dupes == nil {
			t.Error("expected non-nil result")
		}
	})

	t.Run("ListSoftDeletedBooks", func(t *testing.T) {
		books, err := store.ListSoftDeletedBooks(10, 0, nil)
		if err != nil {
			t.Fatalf("ListSoftDeletedBooks failed: %v", err)
		}
		if books == nil {
			t.Error("expected non-nil result")
		}
	})
}

func TestMockStore_CallTracking(t *testing.T) {
	store := NewMockStore()

	store.Close()
	store.GetAllAuthors()
	store.CreateAuthor("Test")

	if len(store.Calls) != 3 {
		t.Errorf("expected 3 calls, got %d", len(store.Calls))
	}

	if store.Calls[0].Method != "Close" {
		t.Error("first call should be Close")
	}
	if store.Calls[1].Method != "GetAllAuthors" {
		t.Error("second call should be GetAllAuthors")
	}
	if store.Calls[2].Method != "CreateAuthor" {
		t.Error("third call should be CreateAuthor")
	}
}
