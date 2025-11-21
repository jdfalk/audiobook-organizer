// file: scripts/cleanup_invalid_books.go
// version: 1.0.1
// guid: 9b0c1d2e-3f4a-5b6c-7d8e-9f0a1b2c3d4e

package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

func main() {
	// Initialize database
	if err := database.Initialize("audiobooks.pebble"); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

	fmt.Println("Scanning for invalid book records...")

	// Get all books
	allBooks, err := database.GlobalStore.GetAllBooks(10000, 0)
	if err != nil {
		log.Fatalf("Failed to get books: %v", err)
	}

	fmt.Printf("Found %d total books\n", len(allBooks))

	// Find books with template variables in file paths
	var invalidBooks []database.Book
	for _, book := range allBooks {
		if strings.Contains(book.FilePath, "{series}") ||
			strings.Contains(book.FilePath, "{narrator}") ||
			strings.Contains(book.FilePath, "{author}") ||
			strings.Contains(book.FilePath, "{title}") {
			invalidBooks = append(invalidBooks, book)
		}
	}

	fmt.Printf("Found %d invalid book records with template variables in file paths\n", len(invalidBooks))

	if len(invalidBooks) == 0 {
		fmt.Println("No invalid books found. Database is clean.")
		return
	}

	// Show invalid books
	fmt.Println("\nInvalid books:")
	for i, book := range invalidBooks {
		fmt.Printf("%d. ID: %d, Title: %s\n", i+1, book.ID, book.Title)
		fmt.Printf("   FilePath: %s\n", book.FilePath)
	}

	// Ask for confirmation
	fmt.Print("\nDelete these invalid records? (yes/no): ")
	var response string
	fmt.Scanln(&response)

	if strings.ToLower(response) != "yes" {
		fmt.Println("Aborted. No records deleted.")
		return
	}

	// Delete invalid books
	deleted := 0
	for _, book := range invalidBooks {
		if err := database.GlobalStore.DeleteBook(book.ID); err != nil {
			log.Printf("Failed to delete book %d: %v", book.ID, err)
		} else {
			deleted++
		}
	}

	fmt.Printf("\nDeleted %d invalid book records\n", deleted)
	fmt.Println("Done. You should now rescan your library to add books with correct file paths.")
}
