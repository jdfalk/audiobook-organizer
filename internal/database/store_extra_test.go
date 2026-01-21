// file: internal/database/store_extra_test.go
// version: 1.0.0
// guid: 68b2b2f9-2b8f-4f7f-9d8f-26e6306a3c8e

package database

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStoreAdditionalCoverageSQLite(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	if err := RunMigrations(store); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	exerciseStoreCommon(t, store)
}

func TestStoreAdditionalCoveragePebble(t *testing.T) {
	store, cleanup := setupPebbleTestDB(t)
	defer cleanup()

	exerciseStoreCommon(t, store)
	exercisePebbleAdvanced(t, store.(*PebbleStore))
}

func TestSQLiteUnsupportedFeatures(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()
	sqliteStore := store.(*SQLiteStore)

	if user, err := sqliteStore.CreateUser("user", "user@example.com", "algo", "hash", []string{"user"}, "active"); err == nil || user != nil {
		t.Fatal("expected CreateUser to be unsupported")
	}
	if user, err := sqliteStore.GetUserByID("missing"); err != nil || user != nil {
		t.Fatal("expected GetUserByID to return nil without error")
	}
	if user, err := sqliteStore.GetUserByUsername("missing"); err != nil || user != nil {
		t.Fatal("expected GetUserByUsername to return nil without error")
	}
	if user, err := sqliteStore.GetUserByEmail("missing@example.com"); err != nil || user != nil {
		t.Fatal("expected GetUserByEmail to return nil without error")
	}
	if err := sqliteStore.UpdateUser(&User{ID: "id"}); err == nil {
		t.Fatal("expected UpdateUser to be unsupported")
	}

	if sess, err := sqliteStore.CreateSession("user", "127.0.0.1", "agent", time.Minute); err == nil || sess != nil {
		t.Fatal("expected CreateSession to be unsupported")
	}
	if sess, err := sqliteStore.GetSession("missing"); err != nil || sess != nil {
		t.Fatal("expected GetSession to return nil without error")
	}
	if err := sqliteStore.RevokeSession("missing"); err == nil {
		t.Fatal("expected RevokeSession to be unsupported")
	}
	if sessions, err := sqliteStore.ListUserSessions("user"); err != nil || len(sessions) != 0 {
		t.Fatal("expected ListUserSessions to return empty slice")
	}

	if err := sqliteStore.SetUserPreferenceForUser("user", "theme", "dark"); err == nil {
		t.Fatal("expected SetUserPreferenceForUser to be unsupported")
	}
	if pref, err := sqliteStore.GetUserPreferenceForUser("user", "theme"); err != nil || pref != nil {
		t.Fatal("expected GetUserPreferenceForUser to return nil without error")
	}
	if prefs, err := sqliteStore.GetAllPreferencesForUser("user"); err != nil || len(prefs) != 0 {
		t.Fatal("expected GetAllPreferencesForUser to return empty slice")
	}

	if seg, err := sqliteStore.CreateBookSegment(1, &BookSegment{FilePath: "/tmp/seg.mp3"}); err == nil || seg != nil {
		t.Fatal("expected CreateBookSegment to be unsupported")
	}
	if segs, err := sqliteStore.ListBookSegments(1); err != nil || len(segs) != 0 {
		t.Fatal("expected ListBookSegments to return empty slice")
	}
	if err := sqliteStore.MergeBookSegments(1, &BookSegment{FilePath: "/tmp/seg.mp3"}, []string{"seg"}); err == nil {
		t.Fatal("expected MergeBookSegments to be unsupported")
	}

	if err := sqliteStore.AddPlaybackEvent(&PlaybackEvent{UserID: "user"}); err == nil {
		t.Fatal("expected AddPlaybackEvent to be unsupported")
	}
	if events, err := sqliteStore.ListPlaybackEvents("user", 1, 10); err != nil || len(events) != 0 {
		t.Fatal("expected ListPlaybackEvents to return empty slice")
	}
	if err := sqliteStore.UpdatePlaybackProgress(&PlaybackProgress{UserID: "user"}); err == nil {
		t.Fatal("expected UpdatePlaybackProgress to be unsupported")
	}
	if progress, err := sqliteStore.GetPlaybackProgress("user", 1); err != nil || progress != nil {
		t.Fatal("expected GetPlaybackProgress to return nil without error")
	}
	if err := sqliteStore.IncrementBookPlayStats(1, 10); err == nil {
		t.Fatal("expected IncrementBookPlayStats to be unsupported")
	}
	if stats, err := sqliteStore.GetBookStats(1); err != nil || stats != nil {
		t.Fatal("expected GetBookStats to return nil without error")
	}
	if err := sqliteStore.IncrementUserListenStats("user", 10); err == nil {
		t.Fatal("expected IncrementUserListenStats to be unsupported")
	}
	if stats, err := sqliteStore.GetUserStats("user"); err != nil || stats != nil {
		t.Fatal("expected GetUserStats to return nil without error")
	}
}

func exerciseStoreCommon(t *testing.T, store Store) {
	t.Helper()

	author, err := store.CreateAuthor("Coverage Author")
	if err != nil {
		t.Fatalf("CreateAuthor failed: %v", err)
	}
	if _, err := store.GetAuthorByID(author.ID); err != nil {
		t.Fatalf("GetAuthorByID failed: %v", err)
	}
	if _, err := store.GetAuthorByName(author.Name); err != nil {
		t.Fatalf("GetAuthorByName failed: %v", err)
	}
	if _, err := store.GetAllAuthors(); err != nil {
		t.Fatalf("GetAllAuthors failed: %v", err)
	}

	series, err := store.CreateSeries("Coverage Series", &author.ID)
	if err != nil {
		t.Fatalf("CreateSeries failed: %v", err)
	}
	if _, err := store.GetSeriesByID(series.ID); err != nil {
		t.Fatalf("GetSeriesByID failed: %v", err)
	}
	if _, err := store.GetSeriesByName(series.Name, &author.ID); err != nil {
		t.Fatalf("GetSeriesByName failed: %v", err)
	}
	if _, err := store.GetAllSeries(); err != nil {
		t.Fatalf("GetAllSeries failed: %v", err)
	}

	work, err := store.CreateWork(&Work{Title: "Coverage Work"})
	if err != nil {
		t.Fatalf("CreateWork failed: %v", err)
	}
	if _, err := store.GetAllWorks(); err != nil {
		t.Fatalf("GetAllWorks failed: %v", err)
	}
	if _, err := store.GetWorkByID(work.ID); err != nil {
		t.Fatalf("GetWorkByID failed: %v", err)
	}
	if _, err := store.UpdateWork(work.ID, &Work{Title: "Coverage Work Updated"}); err != nil {
		t.Fatalf("UpdateWork failed: %v", err)
	}

	fileHash := "hash-1"
	origHash := "orig-1"
	orgHash := fileHash
	bookPath := filepath.Join(t.TempDir(), "book.mp3")
	book, err := store.CreateBook(&Book{
		Title:             "Coverage Book",
		AuthorID:          &author.ID,
		SeriesID:          &series.ID,
		WorkID:            &work.ID,
		FilePath:          bookPath,
		FileHash:          &fileHash,
		OriginalFileHash:  &origHash,
		OrganizedFileHash: &orgHash,
	})
	if err != nil {
		t.Fatalf("CreateBook failed: %v", err)
	}
	if _, err := store.GetBooksByWorkID(work.ID); err != nil {
		t.Fatalf("GetBooksByWorkID failed: %v", err)
	}
	if _, err := store.GetBookByFilePath(book.FilePath); err != nil {
		t.Fatalf("GetBookByFilePath failed: %v", err)
	}
	if _, err := store.GetBookByFileHash(fileHash); err != nil {
		t.Fatalf("GetBookByFileHash failed: %v", err)
	}
	if _, err := store.GetBookByOriginalHash(origHash); err != nil {
		t.Fatalf("GetBookByOriginalHash failed: %v", err)
	}
	if _, err := store.GetBookByOrganizedHash(orgHash); err != nil {
		t.Fatalf("GetBookByOrganizedHash failed: %v", err)
	}

	book2Path := filepath.Join(t.TempDir(), "book2.mp3")
	if _, err := store.CreateBook(&Book{
		Title:             "Coverage Book 2",
		FilePath:          book2Path,
		FileHash:          &fileHash,
		OrganizedFileHash: &orgHash,
	}); err != nil {
		t.Fatalf("CreateBook (dup) failed: %v", err)
	}
	if dups, err := store.GetDuplicateBooks(); err != nil {
		t.Fatalf("GetDuplicateBooks failed: %v", err)
	} else if len(dups) == 0 {
		if _, ok := store.(*PebbleStore); !ok {
			t.Fatal("expected duplicates for sqlite store")
		}
	}

	marked := true
	ts := time.Now().Add(-48 * time.Hour)
	book.MarkedForDeletion = &marked
	book.MarkedForDeletionAt = &ts
	if _, err := store.UpdateBook(book.ID, book); err != nil {
		t.Fatalf("UpdateBook (soft delete) failed: %v", err)
	}
	if books, err := store.ListSoftDeletedBooks(10, 0, nil); err != nil || len(books) == 0 {
		t.Fatalf("ListSoftDeletedBooks failed: %v", err)
	}
	if count, err := store.CountBooks(); err != nil || count < 1 {
		t.Fatalf("CountBooks failed: %v", err)
	}
	if _, err := store.GetBooksBySeriesID(series.ID); err != nil {
		t.Fatalf("GetBooksBySeriesID failed: %v", err)
	}
	if _, err := store.GetBooksByAuthorID(author.ID); err != nil {
		t.Fatalf("GetBooksByAuthorID failed: %v", err)
	}
	if _, err := store.SearchBooks("Coverage", 10, 0); err != nil {
		t.Fatalf("SearchBooks failed: %v", err)
	}

	importPath, err := store.CreateImportPath("/tmp/coverage", "Coverage Import")
	if err != nil {
		t.Fatalf("CreateImportPath failed: %v", err)
	}
	if _, err := store.GetImportPathByID(importPath.ID); err != nil {
		t.Fatalf("GetImportPathByID failed: %v", err)
	}
	if _, err := store.GetImportPathByPath(importPath.Path); err != nil {
		t.Fatalf("GetImportPathByPath failed: %v", err)
	}
	importPath.Enabled = false
	importPath.BookCount = 2
	if err := store.UpdateImportPath(importPath.ID, importPath); err != nil {
		t.Fatalf("UpdateImportPath failed: %v", err)
	}
	if _, err := store.GetAllImportPaths(); err != nil {
		t.Fatalf("GetAllImportPaths failed: %v", err)
	}
	if err := store.DeleteImportPath(importPath.ID); err != nil {
		t.Fatalf("DeleteImportPath failed: %v", err)
	}

	folderPath := "/tmp/coverage"
	op, err := store.CreateOperation("op-coverage", "scan", &folderPath)
	if err != nil {
		t.Fatalf("CreateOperation failed: %v", err)
	}
	if err := store.UpdateOperationStatus(op.ID, "running", 0, 10, "start"); err != nil {
		t.Fatalf("UpdateOperationStatus failed: %v", err)
	}
	if err := store.UpdateOperationStatus(op.ID, "completed", 10, 10, "done"); err != nil {
		t.Fatalf("UpdateOperationStatus completed failed: %v", err)
	}
	if err := store.UpdateOperationError(op.ID, "err"); err != nil {
		t.Fatalf("UpdateOperationError failed: %v", err)
	}
	if _, err := store.GetOperationByID(op.ID); err != nil {
		t.Fatalf("GetOperationByID failed: %v", err)
	}
	if _, err := store.GetRecentOperations(5); err != nil {
		t.Fatalf("GetRecentOperations failed: %v", err)
	}
	detail := "detail"
	if err := store.AddOperationLog(op.ID, "info", "message", &detail); err != nil {
		t.Fatalf("AddOperationLog failed: %v", err)
	}
	if _, err := store.GetOperationLogs(op.ID); err != nil {
		t.Fatalf("GetOperationLogs failed: %v", err)
	}

	state := &MetadataFieldState{
		BookID:         book.ID,
		Field:          "title",
		OverrideLocked: true,
		UpdatedAt:      time.Now(),
	}
	if err := store.UpsertMetadataFieldState(state); err != nil {
		t.Fatalf("UpsertMetadataFieldState failed: %v", err)
	}
	if _, err := store.GetMetadataFieldStates(book.ID); err != nil {
		t.Fatalf("GetMetadataFieldStates failed: %v", err)
	}
	if err := store.DeleteMetadataFieldState(book.ID, "title"); err != nil {
		t.Fatalf("DeleteMetadataFieldState failed: %v", err)
	}

	if err := store.SetUserPreference("theme", "dark"); err != nil {
		t.Fatalf("SetUserPreference failed: %v", err)
	}
	if _, err := store.GetUserPreference("theme"); err != nil {
		t.Fatalf("GetUserPreference failed: %v", err)
	}
	if _, err := store.GetAllUserPreferences(); err != nil {
		t.Fatalf("GetAllUserPreferences failed: %v", err)
	}

	playlist, err := store.CreatePlaylist("Coverage Playlist", &series.ID, "/tmp/list.m3u")
	if err != nil {
		t.Fatalf("CreatePlaylist failed: %v", err)
	}
	if _, err := store.GetPlaylistByID(playlist.ID); err != nil {
		t.Fatalf("GetPlaylistByID failed: %v", err)
	}
	if _, err := store.GetPlaylistBySeriesID(series.ID); err != nil {
		t.Fatalf("GetPlaylistBySeriesID failed: %v", err)
	}
	if err := store.AddPlaylistItem(playlist.ID, 1, 1); err != nil {
		t.Fatalf("AddPlaylistItem failed: %v", err)
	}
	if _, err := store.GetPlaylistItems(playlist.ID); err != nil {
		t.Fatalf("GetPlaylistItems failed: %v", err)
	}

	if blocked, err := store.IsHashBlocked("abc"); err != nil || blocked {
		t.Fatalf("IsHashBlocked failed: %v", err)
	}
	if err := store.AddBlockedHash("abc", "reason"); err != nil {
		t.Fatalf("AddBlockedHash failed: %v", err)
	}
	if _, err := store.GetBlockedHashByHash("abc"); err != nil {
		t.Fatalf("GetBlockedHashByHash failed: %v", err)
	}
	if _, err := store.GetAllBlockedHashes(); err != nil {
		t.Fatalf("GetAllBlockedHashes failed: %v", err)
	}
	if err := store.RemoveBlockedHash("abc"); err != nil {
		t.Fatalf("RemoveBlockedHash failed: %v", err)
	}

	if err := store.DeleteBook("missing-book"); err != nil {
		if _, ok := store.(*PebbleStore); ok {
			t.Fatalf("expected Pebble DeleteBook to ignore missing book: %v", err)
		}
	} else {
		if _, ok := store.(*PebbleStore); !ok {
			t.Fatal("expected SQLite DeleteBook to error for missing book")
		}
	}

	if err := store.DeleteWork(work.ID); err != nil {
		t.Fatalf("DeleteWork failed: %v", err)
	}
	if _, err := store.UpdateWork("missing-work", &Work{Title: "Missing"}); err == nil {
		t.Fatal("expected UpdateWork to fail for missing work")
	}
	if err := store.DeleteWork("missing-work"); err != nil {
		if _, ok := store.(*PebbleStore); ok {
			t.Fatalf("expected Pebble DeleteWork to ignore missing work: %v", err)
		}
	} else {
		if _, ok := store.(*PebbleStore); !ok {
			t.Fatal("expected SQLite DeleteWork to error for missing work")
		}
	}
}

func exercisePebbleAdvanced(t *testing.T, store *PebbleStore) {
	t.Helper()

	user, err := store.CreateUser("User", "user@example.com", "algo", "hash", []string{"admin"}, "active")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	if _, err := store.GetUserByID(user.ID); err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}
	if _, err := store.GetUserByUsername(user.Username); err != nil {
		t.Fatalf("GetUserByUsername failed: %v", err)
	}
	if _, err := store.GetUserByEmail(user.Email); err != nil {
		t.Fatalf("GetUserByEmail failed: %v", err)
	}
	user.Status = "disabled"
	if err := store.UpdateUser(user); err != nil {
		t.Fatalf("UpdateUser failed: %v", err)
	}

	sess, err := store.CreateSession(user.ID, "127.0.0.1", "agent", time.Minute)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	if _, err := store.GetSession(sess.ID); err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if err := store.RevokeSession(sess.ID); err != nil {
		t.Fatalf("RevokeSession failed: %v", err)
	}
	if _, err := store.ListUserSessions(user.ID); err != nil {
		t.Fatalf("ListUserSessions failed: %v", err)
	}

	if err := store.SetUserPreferenceForUser(user.ID, "theme", "dark"); err != nil {
		t.Fatalf("SetUserPreferenceForUser failed: %v", err)
	}
	if _, err := store.GetUserPreferenceForUser(user.ID, "theme"); err != nil {
		t.Fatalf("GetUserPreferenceForUser failed: %v", err)
	}
	if _, err := store.GetAllPreferencesForUser(user.ID); err != nil {
		t.Fatalf("GetAllPreferencesForUser failed: %v", err)
	}

	segment, err := store.CreateBookSegment(1, &BookSegment{FilePath: "/tmp/segment.mp3", DurationSec: 60})
	if err != nil {
		t.Fatalf("CreateBookSegment failed: %v", err)
	}
	if _, err := store.ListBookSegments(1); err != nil {
		t.Fatalf("ListBookSegments failed: %v", err)
	}
	if err := store.MergeBookSegments(1, &BookSegment{FilePath: "/tmp/segment2.mp3", DurationSec: 40}, []string{segment.ID}); err != nil {
		t.Fatalf("MergeBookSegments failed: %v", err)
	}

	if err := store.AddPlaybackEvent(&PlaybackEvent{UserID: user.ID, BookID: 1}); err != nil {
		t.Fatalf("AddPlaybackEvent failed: %v", err)
	}
	if _, err := store.ListPlaybackEvents(user.ID, 1, 10); err != nil {
		t.Fatalf("ListPlaybackEvents failed: %v", err)
	}
	if err := store.UpdatePlaybackProgress(&PlaybackProgress{UserID: user.ID, BookID: 1, PositionSec: 10}); err != nil {
		t.Fatalf("UpdatePlaybackProgress failed: %v", err)
	}
	if _, err := store.GetPlaybackProgress(user.ID, 1); err != nil {
		t.Fatalf("GetPlaybackProgress failed: %v", err)
	}

	if err := store.IncrementBookPlayStats(1, 120); err != nil {
		t.Fatalf("IncrementBookPlayStats failed: %v", err)
	}
	if _, err := store.GetBookStats(1); err != nil {
		t.Fatalf("GetBookStats failed: %v", err)
	}
	if err := store.IncrementUserListenStats(user.ID, 30); err != nil {
		t.Fatalf("IncrementUserListenStats failed: %v", err)
	}
	if _, err := store.GetUserStats(user.ID); err != nil {
		t.Fatalf("GetUserStats failed: %v", err)
	}
}
