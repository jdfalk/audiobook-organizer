// file: scripts/query_books.go
// version: 1.1.0
// guid: 8a9b0c1d-2e3f-4a5b-6c7d-8e9f0a1b2c3d

package main

import (
	"fmt"
	"log"

	"github.com/cockroachdb/pebble"
)

func main() {
	db, err := pebble.Open("audiobooks.pebble", &pebble.Options{})
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Iterate through all keys with "book:" prefix
	iter, err := db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("book:"),
		UpperBound: []byte("book;"), // Next character after ':'
	})
	if err != nil {
		log.Fatal(err)
	}
	defer iter.Close()

	count := 0
	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		value := iter.Value()

		fmt.Printf("Key: %s\n", key)
		fmt.Printf("Value length: %d bytes\n", len(value))

		// Show first 500 chars of value if it contains file_path
		if len(value) > 0 {
			valStr := string(value)
			if len(valStr) > 500 {
				valStr = valStr[:500] + "..."
			}
			fmt.Printf("Value: %s\n", valStr)
		}
		fmt.Println("---")

		count++
		if count >= 5 {
			break
		}
	}

	if err := iter.Error(); err != nil {
		log.Fatal(err)
	}

	if count == 0 {
		fmt.Println("No books found in database")

		// Try listing all keys
		fmt.Println("\nAll keys in database:")
		iter2, err := db.NewIter(nil)
		if err != nil {
			log.Fatal(err)
		}
		defer iter2.Close()

		keyCount := 0
		for iter2.First(); iter2.Valid(); iter2.Next() {
			fmt.Printf("%s\n", string(iter2.Key()))
			keyCount++
			if keyCount >= 20 {
				fmt.Println("(showing first 20 keys)")
				break
			}
		}
	}
}
