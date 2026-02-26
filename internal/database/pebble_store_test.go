// file: internal/database/pebble_store_test.go
// version: 1.0.3
// guid: 4d5e6f7a-8b9c-0d1e-2f3a-4b5c6d7e8f9a

package database

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cockroachdb/pebble/v2"
	ulid "github.com/oklog/ulid/v2"
)

// setupPebbleTestDB creates a temporary PebbleDB database for testing
// Returns the store and a cleanup function
func setupPebbleTestDB(t *testing.T) (Store, func()) {
	// Create temporary database directory with unique name
	tmpdir := "/tmp/test_pebble_" + ulid.Make().String()

	// Create the store
	store, err := NewPebbleStore(tmpdir)
	if err != nil {
		t.Fatalf("Failed to create test Pebble database: %v", err)
	}

	// Cleanup function removes the database directory
	cleanup := func() {
		store.Close()
		os.RemoveAll(tmpdir)
	}

	return store, cleanup
}

// TestNewPebbleStore tests Pebble store creation
func TestNewPebbleStore(t *testing.T) {
	// Arrange-Act
	store, cleanup := setupPebbleTestDB(t)
	defer cleanup()

	// Assert
	if store == nil {
		t.Fatal("Expected non-nil store")
	}
}

// TestPebbleCreateAndGetBook tests basic book CRUD operations
func TestPebbleCreateAndGetBook(t *testing.T) {
	// Arrange
	store, cleanup := setupPebbleTestDB(t)
	defer cleanup()

	// Create test author
	createdAuthor, err := store.CreateAuthor("Test Author")
	if err != nil {
		t.Fatalf("Failed to create author: %v", err)
	}

	// Create test book
	book := &Book{
		Title:    "Test Book",
		AuthorID: &createdAuthor.ID,
		FilePath: "/test/path/book.mp3",
	}

	// Act
	createdBook, err := store.CreateBook(book)
	if err != nil {
		t.Fatalf("Failed to create book: %v", err)
	}

	// Assert
	if createdBook.ID == "" {
		t.Error("Expected non-empty book ID (ULID)")
	}

	// Retrieve the book by ID
	retrievedBook, err := store.GetBookByID(createdBook.ID)
	if err != nil {
		t.Fatalf("Failed to get book by ID: %v", err)
	}

	if retrievedBook.Title != "Test Book" {
		t.Errorf("Expected title 'Test Book', got '%s'", retrievedBook.Title)
	}

	if retrievedBook.AuthorID == nil || *retrievedBook.AuthorID != *book.AuthorID {
		t.Error("Author ID mismatch")
	}

	// Test GetBookByFilePath
	bookByPath, err := store.GetBookByFilePath("/test/path/book.mp3")
	if err != nil {
		t.Fatalf("Failed to get book by file path: %v", err)
	}

	if bookByPath.ID != createdBook.ID {
		t.Error("Expected same book when retrieved by file path")
	}
}

// TestPebbleUpdateBook tests book update operations
func TestPebbleUpdateBook(t *testing.T) {
	// Arrange
	store, cleanup := setupPebbleTestDB(t)
	defer cleanup()

	// Create test book
	book := &Book{
		Title:    "Original Title",
		FilePath: "/test/path/book.mp3",
	}
	createdBook, err := store.CreateBook(book)
	if err != nil {
		t.Fatalf("Failed to create book: %v", err)
	}

	// Act - Update the book
	createdBook.Title = "Updated Title"
	narrator := "Test Narrator"
	createdBook.Narrator = &narrator

	updatedBook, err := store.UpdateBook(createdBook.ID, createdBook)
	if err != nil {
		t.Fatalf("Failed to update book: %v", err)
	}

	// Assert
	if updatedBook.Title != "Updated Title" {
		t.Errorf("Expected title 'Updated Title', got '%s'", updatedBook.Title)
	}

	if updatedBook.Narrator == nil || *updatedBook.Narrator != "Test Narrator" {
		t.Error("Narrator not updated correctly")
	}
}

// TestPebbleDeleteBook tests book deletion
func TestPebbleDeleteBook(t *testing.T) {
	// Arrange
	store, cleanup := setupPebbleTestDB(t)
	defer cleanup()

	// Create test book
	book := &Book{
		Title:    "Book to Delete",
		FilePath: "/test/path/book.mp3",
	}
	createdBook, err := store.CreateBook(book)
	if err != nil {
		t.Fatalf("Failed to create book: %v", err)
	}

	// Act - Delete the book
	err = store.DeleteBook(createdBook.ID)
	if err != nil {
		t.Fatalf("Failed to delete book: %v", err)
	}

	// Assert - Verify deletion (GetBookByID returns nil book, not error)
	deletedBook, err := store.GetBookByID(createdBook.ID)
	if err != nil {
		t.Fatalf("Unexpected error when getting deleted book: %v", err)
	}
	if deletedBook != nil {
		t.Error("Expected book to be nil after deletion")
	}
}

// TestPebbleGetAllBooks tests book listing with pagination
func TestPebbleGetAllBooks(t *testing.T) {
	// Arrange
	store, cleanup := setupPebbleTestDB(t)
	defer cleanup()

	// Create multiple books
	for i := 0; i < 5; i++ {
		book := &Book{
			Title:    "Book " + string(rune('A'+i)),
			FilePath: "/test/path/book" + string(rune('A'+i)) + ".mp3",
		}
		_, err := store.CreateBook(book)
		if err != nil {
			t.Fatalf("Failed to create book %d: %v", i, err)
		}
	}

	// Act - List books with pagination
	books, err := store.GetAllBooks(10, 0)
	if err != nil {
		t.Fatalf("Failed to list books: %v", err)
	}

	// Assert
	if len(books) != 5 {
		t.Errorf("Expected 5 books, got %d", len(books))
	}
}

// TestPebbleGetBooksBySeriesID tests filtering books by series
func TestPebbleGetBooksBySeriesID(t *testing.T) {
	// Arrange
	store, cleanup := setupPebbleTestDB(t)
	defer cleanup()

	// Create author
	author, err := store.CreateAuthor("Series Author")
	if err != nil {
		t.Fatalf("Failed to create author: %v", err)
	}

	// Create series
	series, err := store.CreateSeries("Test Series", &author.ID)
	if err != nil {
		t.Fatalf("Failed to create series: %v", err)
	}

	// Create books in series
	for i := 0; i < 3; i++ {
		seq := i + 1
		book := &Book{
			Title:          "Series Book " + string(rune('A'+i)),
			FilePath:       "/test/series/book" + string(rune('A'+i)) + ".mp3",
			SeriesID:       &series.ID,
			SeriesSequence: &seq,
		}
		_, err := store.CreateBook(book)
		if err != nil {
			t.Fatalf("Failed to create book %d: %v", i, err)
		}
	}

	// Act - Get books by series
	seriesBooks, err := store.GetBooksBySeriesID(series.ID)
	if err != nil {
		t.Fatalf("Failed to get books by series: %v", err)
	}

	// Assert
	if len(seriesBooks) != 3 {
		t.Errorf("Expected 3 books in series, got %d", len(seriesBooks))
	}

	// Verify books are in series
	for _, book := range seriesBooks {
		if book.SeriesID == nil || *book.SeriesID != series.ID {
			t.Error("Book not properly associated with series")
		}
	}
}

// TestPebbleGetBooksByAuthorID tests filtering books by author
func TestPebbleGetBooksByAuthorID(t *testing.T) {
	// Arrange
	store, cleanup := setupPebbleTestDB(t)
	defer cleanup()

	// Create author
	author, err := store.CreateAuthor("Author with Books")
	if err != nil {
		t.Fatalf("Failed to create author: %v", err)
	}

	// Create books by author
	for i := 0; i < 3; i++ {
		book := &Book{
			Title:    "Author Book " + string(rune('A'+i)),
			FilePath: "/test/author/book" + string(rune('A'+i)) + ".mp3",
			AuthorID: &author.ID,
		}
		_, err := store.CreateBook(book)
		if err != nil {
			t.Fatalf("Failed to create book %d: %v", i, err)
		}
	}

	// Act - Get books by author
	authorBooks, err := store.GetBooksByAuthorID(author.ID)
	if err != nil {
		t.Fatalf("Failed to get books by author: %v", err)
	}

	// Assert
	if len(authorBooks) != 3 {
		t.Errorf("Expected 3 books by author, got %d", len(authorBooks))
	}

	// Verify books are by author
	for _, book := range authorBooks {
		if book.AuthorID == nil || *book.AuthorID != author.ID {
			t.Error("Book not properly associated with author")
		}
	}
}

// TestPebbleVersionManagement tests book version grouping
func TestPebbleVersionManagement(t *testing.T) {
	// Arrange
	store, cleanup := setupPebbleTestDB(t)
	defer cleanup()

	// Create two books
	book1 := &Book{
		Title:    "Book Version 1",
		FilePath: "/test/path/book_v1.mp3",
	}
	createdBook1, err := store.CreateBook(book1)
	if err != nil {
		t.Fatalf("Failed to create book 1: %v", err)
	}

	book2 := &Book{
		Title:    "Book Version 2",
		FilePath: "/test/path/book_v2.mp3",
	}
	createdBook2, err := store.CreateBook(book2)
	if err != nil {
		t.Fatalf("Failed to create book 2: %v", err)
	}

	// Act - Link books as versions
	groupID := ulid.Make().String()
	createdBook1.VersionGroupID = &groupID
	isPrimary := true
	createdBook1.IsPrimaryVersion = &isPrimary

	_, err = store.UpdateBook(createdBook1.ID, createdBook1)
	if err != nil {
		t.Fatalf("Failed to update book 1 with version group: %v", err)
	}

	createdBook2.VersionGroupID = &groupID
	isPrimaryFalse := false
	createdBook2.IsPrimaryVersion = &isPrimaryFalse

	_, err = store.UpdateBook(createdBook2.ID, createdBook2)
	if err != nil {
		t.Fatalf("Failed to update book 2 with version group: %v", err)
	}

	// Assert - Get books by version group
	versions, err := store.GetBooksByVersionGroup(groupID)
	if err != nil {
		t.Fatalf("Failed to get books by version group: %v", err)
	}

	if len(versions) != 2 {
		t.Errorf("Expected 2 versions, got %d", len(versions))
	}

	// Verify primary version
	foundPrimary := false
	for _, v := range versions {
		if v.IsPrimaryVersion != nil && *v.IsPrimaryVersion {
			foundPrimary = true
			if v.ID != createdBook1.ID {
				t.Error("Wrong book marked as primary")
			}
		}
	}
	if !foundPrimary {
		t.Error("No primary version found")
	}
}

// TestPebbleCreateAndGetAuthor tests author CRUD operations
func TestPebbleCreateAndGetAuthor(t *testing.T) {
	// Arrange
	store, cleanup := setupPebbleTestDB(t)
	defer cleanup()

	// Act
	createdAuthor, err := store.CreateAuthor("J.R.R. Tolkien")
	if err != nil {
		t.Fatalf("Failed to create author: %v", err)
	}

	// Assert
	if createdAuthor.ID == 0 {
		t.Error("Expected non-zero author ID")
	}

	// Get author by ID
	retrievedAuthor, err := store.GetAuthorByID(createdAuthor.ID)
	if err != nil {
		t.Fatalf("Failed to get author: %v", err)
	}

	if retrievedAuthor.Name != "J.R.R. Tolkien" {
		t.Errorf("Expected name 'J.R.R. Tolkien', got '%s'", retrievedAuthor.Name)
	}

	// Get author by name
	authorByName, err := store.GetAuthorByName("J.R.R. Tolkien")
	if err != nil {
		t.Fatalf("Failed to get author by name: %v", err)
	}

	if authorByName.ID != createdAuthor.ID {
		t.Error("Expected same author ID when retrieved by name")
	}
}

// TestPebbleGetAllAuthors tests listing all authors
func TestPebbleGetAllAuthors(t *testing.T) {
	// Arrange
	store, cleanup := setupPebbleTestDB(t)
	defer cleanup()

	// Create multiple authors
	authorNames := []string{"Author A", "Author B", "Author C"}
	for _, name := range authorNames {
		_, err := store.CreateAuthor(name)
		if err != nil {
			t.Fatalf("Failed to create author '%s': %v", name, err)
		}
	}

	// Act
	authors, err := store.GetAllAuthors()
	if err != nil {
		t.Fatalf("Failed to get all authors: %v", err)
	}

	// Assert
	if len(authors) != 3 {
		t.Errorf("Expected 3 authors, got %d", len(authors))
	}
}

// TestPebbleCreateAndGetSeries tests series CRUD operations
func TestPebbleCreateAndGetSeries(t *testing.T) {
	// Arrange
	store, cleanup := setupPebbleTestDB(t)
	defer cleanup()

	// Create author
	author, err := store.CreateAuthor("Series Author")
	if err != nil {
		t.Fatalf("Failed to create author: %v", err)
	}

	// Act
	createdSeries, err := store.CreateSeries("Test Series", &author.ID)
	if err != nil {
		t.Fatalf("Failed to create series: %v", err)
	}

	// Assert
	if createdSeries.ID == 0 {
		t.Error("Expected non-zero series ID")
	}

	// Get series by ID
	retrievedSeries, err := store.GetSeriesByID(createdSeries.ID)
	if err != nil {
		t.Fatalf("Failed to get series: %v", err)
	}

	if retrievedSeries.Name != "Test Series" {
		t.Errorf("Expected series name 'Test Series', got '%s'", retrievedSeries.Name)
	}

	// Get series by name
	seriesByName, err := store.GetSeriesByName("Test Series", &author.ID)
	if err != nil {
		t.Fatalf("Failed to get series by name: %v", err)
	}

	if seriesByName.ID != createdSeries.ID {
		t.Error("Expected same series ID when retrieved by name")
	}
}

// TestPebbleGetAllSeries tests listing all series
func TestPebbleGetAllSeries(t *testing.T) {
	// Arrange
	store, cleanup := setupPebbleTestDB(t)
	defer cleanup()

	// Create author
	author, err := store.CreateAuthor("Prolific Author")
	if err != nil {
		t.Fatalf("Failed to create author: %v", err)
	}

	// Create multiple series
	seriesNames := []string{"Series A", "Series B", "Series C"}
	for _, name := range seriesNames {
		_, err := store.CreateSeries(name, &author.ID)
		if err != nil {
			t.Fatalf("Failed to create series '%s': %v", name, err)
		}
	}

	// Act
	series, err := store.GetAllSeries()
	if err != nil {
		t.Fatalf("Failed to get all series: %v", err)
	}

	// Assert
	if len(series) != 3 {
		t.Errorf("Expected 3 series, got %d", len(series))
	}
}

// TestPebbleSearchBooks tests book search functionality
func TestPebbleSearchBooks(t *testing.T) {
	// Arrange
	store, cleanup := setupPebbleTestDB(t)
	defer cleanup()

	// Create test books with searchable titles
	testBooks := []string{
		"The Hobbit",
		"The Lord of the Rings",
		"The Silmarillion",
		"Harry Potter",
	}

	for i, title := range testBooks {
		book := &Book{
			Title:    title,
			FilePath: "/test/search/book" + string(rune('0'+i)) + ".mp3",
		}
		_, err := store.CreateBook(book)
		if err != nil {
			t.Fatalf("Failed to create book '%s': %v", title, err)
		}
	}

	// Act - Search for "The"
	results, err := store.SearchBooks("The", 10, 0)
	if err != nil {
		t.Fatalf("Failed to search books: %v", err)
	}

	// Assert
	if len(results) < 3 {
		t.Errorf("Expected at least 3 results for 'The', got %d", len(results))
	}
}

// TestPebbleCountBooks tests book counting
func TestPebbleCountBooks(t *testing.T) {
	// Arrange
	store, cleanup := setupPebbleTestDB(t)
	defer cleanup()

	initialCount, err := store.CountBooks()
	if err != nil {
		t.Fatalf("Failed to count books: %v", err)
	}

	// Create books
	for i := 0; i < 5; i++ {
		book := &Book{
			Title:    "Count Book " + string(rune('A'+i)),
			FilePath: "/test/count/book" + string(rune('A'+i)) + ".mp3",
		}
		_, err := store.CreateBook(book)
		if err != nil {
			t.Fatalf("Failed to create book %d: %v", i, err)
		}
	}

	// Act
	newCount, err := store.CountBooks()
	if err != nil {
		t.Fatalf("Failed to count books after creation: %v", err)
	}

	// Assert
	if newCount != initialCount+5 {
		t.Errorf("Expected count to increase by 5, got %d -> %d", initialCount, newCount)
	}
}

// TestPebbleImportPaths tests import path management
func TestPebbleImportPaths(t *testing.T) {
	// Arrange
	store, cleanup := setupPebbleTestDB(t)
	defer cleanup()

	// Act - Create import path
	folder, err := store.CreateImportPath("/media/audiobooks", "Main Library")
	if err != nil {
		t.Fatalf("Failed to create import path: %v", err)
	}

	// Assert
	if folder.ID == 0 {
		t.Error("Expected non-zero import path ID")
	}

	// Get import path by ID
	retrievedFolder, err := store.GetImportPathByID(folder.ID)
	if err != nil {
		t.Fatalf("Failed to get import path: %v", err)
	}

	if retrievedFolder.Path != "/media/audiobooks" {
		t.Errorf("Expected path '/media/audiobooks', got '%s'", retrievedFolder.Path)
	}

	// Get import path by path
	folderByPath, err := store.GetImportPathByPath("/media/audiobooks")
	if err != nil {
		t.Fatalf("Failed to get import path by path: %v", err)
	}

	if folderByPath.ID != folder.ID {
		t.Error("Expected same folder ID when retrieved by path")
	}

	// List all import paths
	folders, err := store.GetAllImportPaths()
	if err != nil {
		t.Fatalf("Failed to get all import paths: %v", err)
	}

	if len(folders) != 1 {
		t.Errorf("Expected 1 import path, got %d", len(folders))
	}
}

func TestPebbleMigrateImportPathKeys(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pebble-migrate")
	db, err := pebble.Open(dbPath, &pebble.Options{})
	if err != nil {
		t.Fatalf("Failed to open Pebble: %v", err)
	}
	defer db.Close()

	legacyFolder := []byte(`{"id":1,"path":"/legacy","name":"Legacy","enabled":true,"book_count":0}`)
	if err := db.Set([]byte("library:1"), legacyFolder, pebble.Sync); err != nil {
		t.Fatalf("Failed to write legacy key: %v", err)
	}
	if err := db.Set([]byte("library:path:/legacy"), []byte("1"), pebble.Sync); err != nil {
		t.Fatalf("Failed to write legacy index key: %v", err)
	}
	if err := db.Set([]byte("counter:library"), []byte("2"), pebble.Sync); err != nil {
		t.Fatalf("Failed to write legacy counter: %v", err)
	}

	store := &PebbleStore{db: db}
	if err := store.migrateImportPathKeys(); err != nil {
		t.Fatalf("migrateImportPathKeys failed: %v", err)
	}

	if _, closer, err := db.Get([]byte("import_path:1")); err != nil {
		t.Fatalf("expected migrated import_path key: %v", err)
	} else {
		closer.Close()
	}
	if _, closer, err := db.Get([]byte("import_path:path:/legacy")); err != nil {
		t.Fatalf("expected migrated import_path index: %v", err)
	} else {
		closer.Close()
	}

	if _, _, err := db.Get([]byte("library:1")); err != pebble.ErrNotFound {
		t.Fatalf("expected legacy key removed, got %v", err)
	}
	if _, _, err := db.Get([]byte("library:path:/legacy")); err != pebble.ErrNotFound {
		t.Fatalf("expected legacy index removed, got %v", err)
	}

	if val, closer, err := db.Get([]byte("counter:import_path")); err != nil {
		t.Fatalf("expected migrated counter: %v", err)
	} else {
		if string(val) != "2" {
			t.Fatalf("unexpected counter value: %s", string(val))
		}
		closer.Close()
	}
	if _, _, err := db.Get([]byte("counter:library")); err != pebble.ErrNotFound {
		t.Fatalf("expected legacy counter removed, got %v", err)
	}

	if err := store.migrateImportPathKeys(); err != nil {
		t.Fatalf("idempotent migration failed: %v", err)
	}
}

// TestPebbleOperations tests operation tracking
func TestPebbleOperations(t *testing.T) {
	// Arrange
	store, cleanup := setupPebbleTestDB(t)
	defer cleanup()

	operationID := ulid.Make().String()
	folderPath := "/media/audiobooks"

	// Act - Create operation
	op, err := store.CreateOperation(operationID, "scan", &folderPath)
	if err != nil {
		t.Fatalf("Failed to create operation: %v", err)
	}

	// Assert
	if op.ID != operationID {
		t.Errorf("Expected operation ID '%s', got '%s'", operationID, op.ID)
	}

	// Get operation by ID
	retrievedOp, err := store.GetOperationByID(operationID)
	if err != nil {
		t.Fatalf("Failed to get operation: %v", err)
	}

	if retrievedOp.Type != "scan" {
		t.Errorf("Expected operation type 'scan', got '%s'", retrievedOp.Type)
	}

	// Update operation status
	err = store.UpdateOperationStatus(operationID, "completed", 100, 100, "Scan completed")
	if err != nil {
		t.Fatalf("Failed to update operation status: %v", err)
	}

	// Verify update
	updatedOp, err := store.GetOperationByID(operationID)
	if err != nil {
		t.Fatalf("Failed to get updated operation: %v", err)
	}

	if updatedOp.Status != "completed" {
		t.Errorf("Expected status 'completed', got '%s'", updatedOp.Status)
	}
}

// TestPebbleUserPreferences tests user preference storage
func TestPebbleUserPreferences(t *testing.T) {
	// Arrange
	store, cleanup := setupPebbleTestDB(t)
	defer cleanup()

	// Act - Set preference
	err := store.SetUserPreference("theme", "dark")
	if err != nil {
		t.Fatalf("Failed to set user preference: %v", err)
	}

	// Get preference
	pref, err := store.GetUserPreference("theme")
	if err != nil {
		t.Fatalf("Failed to get user preference: %v", err)
	}

	// Assert
	if pref.Value == nil || *pref.Value != "dark" {
		if pref.Value == nil {
			t.Error("Expected preference value to be set")
		} else {
			t.Errorf("Expected preference value 'dark', got '%s'", *pref.Value)
		}
	}

	// Get all preferences
	prefs, err := store.GetAllUserPreferences()
	if err != nil {
		t.Fatalf("Failed to get all preferences: %v", err)
	}

	if len(prefs) != 1 {
		t.Errorf("Expected 1 preference, got %d", len(prefs))
	}
}
