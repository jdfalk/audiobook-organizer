// file: internal/openlibrary/store.go
// version: 2.0.0
// guid: c3d4e5f6-a7b8-9c0d-1e2f-3a4b5c6d7e8f

package openlibrary

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/pebble"
)

// OLStore provides local lookup of Open Library data dump records stored in PebbleDB.
type OLStore struct {
	db *pebble.DB
}

// NewOLStore opens or creates a PebbleDB instance for Open Library dump data.
func NewOLStore(path string) (*OLStore, error) {
	db, err := pebble.Open(path, &pebble.Options{})
	if err != nil {
		return nil, fmt.Errorf("failed to open OL store: %w", err)
	}
	return &OLStore{db: db}, nil
}

// Close closes the underlying PebbleDB.
func (s *OLStore) Close() error {
	return s.db.Close()
}

// Key prefixes
const (
	prefixEdition       = "ol:edition:"
	prefixEditionISBN10 = "ol:edition:isbn10:"
	prefixEditionISBN13 = "ol:edition:isbn13:"
	prefixWork          = "ol:work:"
	prefixWorkTitle     = "ol:work:title:"
	prefixAuthor        = "ol:author:"
	prefixAuthorName    = "ol:author:name:"
	prefixMetaStatus    = "ol:meta:status"
)

func normalizeForIndex(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// indexEntry holds pre-parsed key-value pairs ready for batch writing.
type indexEntry struct {
	keys [][]byte
	vals [][]byte
}

// ImportDump stream-parses a TSV.gz dump file using parallel workers and batch-writes to PebbleDB.
// A reader goroutine feeds raw lines to N CPU-bound JSON-parsing workers.
// A single writer goroutine collects results and batch-writes to Pebble (keeping writes single-threaded).
// Progress callback receives the number of records imported so far.
func (s *OLStore) ImportDump(dumpType, filePath string, progress func(int)) error {
	switch dumpType {
	case "editions", "authors", "works":
	default:
		return fmt.Errorf("unknown dump type: %s", dumpType)
	}

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open dump file: %w", err)
	}
	defer f.Close()

	// Get file size for progress estimation
	fi, _ := f.Stat()
	var estimatedTotal int64
	if fi != nil {
		switch dumpType {
		case "editions":
			estimatedTotal = fi.Size() / 800
		case "authors":
			estimatedTotal = fi.Size() / 400
		case "works":
			estimatedTotal = fi.Size() / 500
		}
	}

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gz.Close()

	numWorkers := max(runtime.NumCPU(), 2)

	lineCh := make(chan []byte, 1000)
	entryCh := make(chan indexEntry, 1000)
	errCh := make(chan error, 1)

	// Reader goroutine: decompress + scan lines → lineCh
	go func() {
		defer close(lineCh)
		scanner := bufio.NewScanner(gz)
		scanner.Buffer(make([]byte, 0, 4*1024*1024), 16*1024*1024)
		for scanner.Scan() {
			// Copy the line since scanner reuses its buffer
			line := make([]byte, len(scanner.Bytes()))
			copy(line, scanner.Bytes())
			lineCh <- line
		}
		if err := scanner.Err(); err != nil {
			select {
			case errCh <- fmt.Errorf("scanner error: %w", err):
			default:
			}
		}
	}()

	// Worker goroutines: parse JSON → entryCh
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for line := range lineCh {
				parts := strings.SplitN(string(line), "\t", 5)
				if len(parts) < 5 {
					continue
				}
				jsonData := []byte(parts[4])
				entry, err := s.parseEntry(dumpType, jsonData)
				if err != nil {
					continue // skip malformed
				}
				entryCh <- entry
			}
		}()
	}

	// Close entryCh when all workers done
	go func() {
		wg.Wait()
		close(entryCh)
	}()

	// Writer goroutine (single): batch-write to Pebble
	batch := s.db.NewBatch()
	count := 0
	const batchSize = 5000

	for entry := range entryCh {
		for i := range entry.keys {
			if err := batch.Set(entry.keys[i], entry.vals[i], pebble.NoSync); err != nil {
				return fmt.Errorf("batch set failed: %w", err)
			}
		}
		count++
		if count%batchSize == 0 {
			if err := batch.Commit(pebble.NoSync); err != nil {
				return fmt.Errorf("batch commit failed at record %d: %w", count, err)
			}
			batch = s.db.NewBatch()
			if progress != nil {
				progress(count)
			}
			if count%50000 == 0 {
				s.updateImportProgress(dumpType, count, estimatedTotal)
			}
		}
	}

	// Check for reader errors
	select {
	case err := <-errCh:
		return err
	default:
	}

	// Commit remaining
	if err := batch.Commit(pebble.NoSync); err != nil {
		return fmt.Errorf("final batch commit failed: %w", err)
	}
	if progress != nil {
		progress(count)
	}

	// Final sync to ensure durability
	s.updateStatus(dumpType, count)

	return nil
}

// parseEntry unmarshals JSON and builds index key-value pairs without touching Pebble.
func (s *OLStore) parseEntry(dumpType string, jsonData []byte) (indexEntry, error) {
	switch dumpType {
	case "editions":
		return s.parseEdition(jsonData)
	case "authors":
		return s.parseAuthor(jsonData)
	case "works":
		return s.parseWork(jsonData)
	default:
		return indexEntry{}, fmt.Errorf("unknown dump type: %s", dumpType)
	}
}

func (s *OLStore) parseEdition(data []byte) (indexEntry, error) {
	var ed OLEdition
	if err := json.Unmarshal(data, &ed); err != nil {
		return indexEntry{}, err
	}
	if ed.Key == "" {
		return indexEntry{}, fmt.Errorf("missing key")
	}
	e := indexEntry{}
	e.keys = append(e.keys, []byte(prefixEdition+ed.Key))
	e.vals = append(e.vals, data)
	for _, isbn := range ed.ISBN10 {
		e.keys = append(e.keys, []byte(prefixEditionISBN10+isbn))
		e.vals = append(e.vals, []byte(ed.Key))
	}
	for _, isbn := range ed.ISBN13 {
		e.keys = append(e.keys, []byte(prefixEditionISBN13+isbn))
		e.vals = append(e.vals, []byte(ed.Key))
	}
	return e, nil
}

func (s *OLStore) parseAuthor(data []byte) (indexEntry, error) {
	var author OLAuthor
	if err := json.Unmarshal(data, &author); err != nil {
		return indexEntry{}, err
	}
	if author.Key == "" {
		return indexEntry{}, fmt.Errorf("missing key")
	}
	e := indexEntry{}
	e.keys = append(e.keys, []byte(prefixAuthor+author.Key))
	e.vals = append(e.vals, data)
	if author.Name != "" {
		e.keys = append(e.keys, []byte(prefixAuthorName+normalizeForIndex(author.Name)))
		e.vals = append(e.vals, []byte(author.Key))
	}
	return e, nil
}

func (s *OLStore) parseWork(data []byte) (indexEntry, error) {
	var work OLWork
	if err := json.Unmarshal(data, &work); err != nil {
		return indexEntry{}, err
	}
	if work.Key == "" {
		return indexEntry{}, fmt.Errorf("missing key")
	}
	e := indexEntry{}
	e.keys = append(e.keys, []byte(prefixWork+work.Key))
	e.vals = append(e.vals, data)
	if work.Title != "" {
		e.keys = append(e.keys, []byte(prefixWorkTitle+normalizeForIndex(work.Title)))
		e.vals = append(e.vals, []byte(work.Key))
	}
	return e, nil
}

// LookupByISBN finds an edition by ISBN (tries ISBN-13 first, then ISBN-10).
func (s *OLStore) LookupByISBN(isbn string) (*OLEdition, error) {
	isbn = strings.TrimSpace(isbn)

	prefixes := []string{prefixEditionISBN13, prefixEditionISBN10}
	if len(isbn) == 10 {
		prefixes = []string{prefixEditionISBN10, prefixEditionISBN13}
	}

	for _, prefix := range prefixes {
		val, closer, err := s.db.Get([]byte(prefix + isbn))
		if err == pebble.ErrNotFound {
			continue
		}
		if err != nil {
			return nil, err
		}
		key := string(val)
		closer.Close()

		return s.getEdition(key)
	}
	return nil, fmt.Errorf("ISBN not found: %s", isbn)
}

func (s *OLStore) getEdition(key string) (*OLEdition, error) {
	val, closer, err := s.db.Get([]byte(prefixEdition + key))
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	var ed OLEdition
	if err := json.Unmarshal(val, &ed); err != nil {
		return nil, err
	}
	return &ed, nil
}

// SearchByTitle searches for works by normalized title prefix.
func (s *OLStore) SearchByTitle(title string) ([]OLEdition, error) {
	normalized := normalizeForIndex(title)
	if normalized == "" {
		return nil, nil
	}

	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte(prefixWorkTitle + normalized),
		UpperBound: []byte(prefixWorkTitle + normalized + "\xff"),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var results []OLEdition
	for iter.First(); iter.Valid() && len(results) < 10; iter.Next() {
		workKey := string(iter.Value())
		editions := s.editionsForWork(workKey)
		results = append(results, editions...)
	}

	return results, nil
}

func (s *OLStore) editionsForWork(workKey string) []OLEdition {
	work, err := s.LookupWork(workKey)
	if err != nil {
		return nil
	}

	return []OLEdition{{
		Key:   work.Key,
		Title: work.Title,
		Authors: func() []OLRef {
			if work.Authors == nil {
				return nil
			}
			return work.Authors
		}(),
		Covers: work.Covers,
		Works:  []OLRef{{Key: work.Key}},
	}}
}

// LookupAuthor retrieves an author by OL key.
func (s *OLStore) LookupAuthor(key string) (*OLAuthor, error) {
	val, closer, err := s.db.Get([]byte(prefixAuthor + key))
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	var author OLAuthor
	if err := json.Unmarshal(val, &author); err != nil {
		return nil, err
	}
	return &author, nil
}

// LookupWork retrieves a work by OL key.
func (s *OLStore) LookupWork(key string) (*OLWork, error) {
	val, closer, err := s.db.Get([]byte(prefixWork + key))
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	var work OLWork
	if err := json.Unmarshal(val, &work); err != nil {
		return nil, err
	}
	return &work, nil
}

// GetStatus returns the current dump import status.
func (s *OLStore) GetStatus() (*DumpStatus, error) {
	val, closer, err := s.db.Get([]byte(prefixMetaStatus))
	if err == pebble.ErrNotFound {
		return &DumpStatus{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	var status DumpStatus
	if err := json.Unmarshal(val, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

func (s *OLStore) updateStatus(dumpType string, recordCount int) {
	status, _ := s.GetStatus()
	if status == nil {
		status = &DumpStatus{}
	}

	ts := DumpTypeStatus{
		RecordCount:    int64(recordCount),
		ImportProgress: 1.0,
		LastUpdated:    time.Now(),
	}

	switch dumpType {
	case "editions":
		status.Editions = ts
	case "authors":
		status.Authors = ts
	case "works":
		status.Works = ts
	}

	data, err := json.Marshal(status)
	if err != nil {
		return
	}
	s.db.Set([]byte(prefixMetaStatus), data, pebble.Sync)
}

// updateImportProgress writes fractional progress during import.
func (s *OLStore) updateImportProgress(dumpType string, count int, estimatedTotal int64) {
	status, _ := s.GetStatus()
	if status == nil {
		status = &DumpStatus{}
	}

	var prog float64
	if estimatedTotal > 0 {
		prog = float64(count) / float64(estimatedTotal)
		if prog > 0.99 {
			prog = 0.99 // reserve 1.0 for completion
		}
	}

	ts := DumpTypeStatus{
		RecordCount:    int64(count),
		ImportProgress: prog,
		LastUpdated:    time.Now(),
	}

	switch dumpType {
	case "editions":
		status.Editions = ts
	case "authors":
		status.Authors = ts
	case "works":
		status.Works = ts
	}

	data, err := json.Marshal(status)
	if err != nil {
		return
	}
	s.db.Set([]byte(prefixMetaStatus), data, pebble.NoSync)
}
