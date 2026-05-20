// file: tools/cmd/reconcile-paths/main.go
// version: 1.0.0
// last-edited: 2026-05-20
//
// reconcile-paths is a READ-ONLY dry-run CLI tool that identifies audiobooks
// whose FilePath no longer exists on disk and finds their candidate location
// via the "Title - Title" doubled-folder pattern.
//
// No DB writes. No file moves. Output is a CSV report only.
//
// Usage:
//
//	reconcile-paths [-api URL] [-key KEY] [-out FILE] [-limit N] [-page-size N] [-verbose]
//
// SSH mode (dev box → prod):
//
//	RECONCILE_VIA_SSH=172.16.2.30 reconcile-paths ...
package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// audioExt lists extensions to check for single-file books.
var audioExt = []string{".m4b", ".mp3", ".m4a", ".ogg", ".flac", ".aac", ".opus"}

// sshBatchSize controls how many paths are checked in a single ssh invocation.
const sshBatchSize = 50

// ----- API response types (mirrors internal/audiobooks types) -----

type book struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	FilePath string `json:"file_path"`
}

type listData struct {
	Items  []book `json:"items"`
	Count  int    `json:"count"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}

// listResponse handles both wrapped {"data": {...}} and unwrapped shapes.
type listResponse struct {
	Data *listData `json:"data"`
	// Flat fields (fallback if API returns top-level directly)
	Items  []book `json:"items"`
	Count  int    `json:"count"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}

// ----- Result record -----

type matchRecord struct {
	bookID        string
	title         string
	dbPath        string
	candidatePath string
	candidateKind string // "doubled-folder" | "single-file-<ext>"
	audioCount    int    // files in folder (0 for single-file hits)
}

// ----- main -----

func main() {
	apiURL := flag.String("api", "https://172.16.2.30:8484", "API base URL")
	outFile := flag.String("out", "/tmp/reconcile_dry_run.csv", "Output CSV path (use '-' for stdout)")
	limit := flag.Int("limit", 0, "Max books to inspect (0 = all)")
	apiKey := flag.String("key", "", "API key (or set AUDIOBOOK_API_KEY env)")
	pageSize := flag.Int("page-size", 200, "Pagination page size")
	verbose := flag.Bool("verbose", false, "Verbose progress logging")
	stdout := flag.Bool("stdout", false, "Write CSV to stdout instead of -out file")
	flag.Parse()

	// Resolve API key.
	key := *apiKey
	if key == "" {
		key = os.Getenv("AUDIOBOOK_API_KEY")
	}
	if key == "" {
		log.Fatal("API key required: use -key flag or AUDIOBOOK_API_KEY env var")
	}

	sshHost := os.Getenv("RECONCILE_VIA_SSH")

	// Build HTTP client (TLS skip for self-signed cert).
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
	}
	client := &http.Client{Transport: transport}

	// Fetch all books.
	books, err := fetchAllBooks(client, *apiURL, key, *pageSize, *limit, *verbose)
	if err != nil {
		log.Fatalf("fetch books: %v", err)
	}
	log.Printf("Fetched %d books from API", len(books))

	// Stat all db paths to find missing ones.
	missing := filterMissing(books, sshHost, *verbose)
	log.Printf("%d books have missing FilePath on disk", len(missing))

	// For each missing book, probe candidates.
	var records []matchRecord
	noMatch := 0
	for _, b := range missing {
		rec, found := findCandidate(b, sshHost, *verbose)
		if found {
			records = append(records, rec)
		} else {
			noMatch++
		}
	}

	// Write CSV.
	var w io.Writer
	if *stdout || *outFile == "-" {
		w = os.Stdout
	} else {
		f, err := os.Create(*outFile)
		if err != nil {
			log.Fatalf("create output file: %v", err)
		}
		defer f.Close()
		w = f
		log.Printf("Writing CSV to %s", *outFile)
	}
	if err := writeCSV(w, records); err != nil {
		log.Fatalf("write CSV: %v", err)
	}

	// Summary.
	doubledFolderCount := 0
	singleFileCount := 0
	for _, r := range records {
		if r.candidateKind == "doubled-folder" {
			doubledFolderCount++
		} else {
			singleFileCount++
		}
	}
	fmt.Fprintf(os.Stderr, "\n=== SUMMARY ===\n")
	fmt.Fprintf(os.Stderr, "Total books inspected:    %d\n", len(books))
	fmt.Fprintf(os.Stderr, "Missing on disk:          %d\n", len(missing))
	fmt.Fprintf(os.Stderr, "Matched (doubled-folder): %d\n", doubledFolderCount)
	fmt.Fprintf(os.Stderr, "Matched (single-file):    %d\n", singleFileCount)
	fmt.Fprintf(os.Stderr, "No match found:           %d\n", noMatch)
}

// ----- API pagination -----

func fetchAllBooks(client *http.Client, apiURL, key string, pageSize, limit int, verbose bool) ([]book, error) {
	var all []book
	offset := 0
	for {
		url := fmt.Sprintf("%s/api/v1/audiobooks?page_size=%d&offset=%d", apiURL, pageSize, offset)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+key)

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("GET %s: %w", url, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, body)
		}

		var lr listResponse
		if err := json.Unmarshal(body, &lr); err != nil {
			return nil, fmt.Errorf("decode response: %w", err)
		}
		// Unwrap data envelope if present.
		items := lr.Items
		count := lr.Count
		if lr.Data != nil {
			items = lr.Data.Items
			count = lr.Data.Count
		}
		if verbose {
			log.Printf("Page offset=%d: got %d items (total count=%d)", offset, len(items), count)
		}
		all = append(all, items...)

		if limit > 0 && len(all) >= limit {
			all = all[:limit]
			break
		}
		if len(items) < pageSize || len(all) >= count {
			break
		}
		offset += len(items)
	}
	return all, nil
}

// ----- Stat helpers -----

// filterMissing returns books whose FilePath does not exist on disk.
func filterMissing(books []book, sshHost string, verbose bool) []book {
	if sshHost != "" {
		return filterMissingSSH(books, sshHost, verbose)
	}
	var missing []book
	for _, b := range books {
		if b.FilePath == "" {
			continue
		}
		if _, err := os.Stat(b.FilePath); os.IsNotExist(err) {
			if verbose {
				log.Printf("MISSING: %s", b.FilePath)
			}
			missing = append(missing, b)
		}
	}
	return missing
}

// filterMissingSSH batches stat calls over SSH.
func filterMissingSSH(books []book, host string, verbose bool) []book {
	// Build index: path → book.
	pathToBook := make(map[string]book, len(books))
	paths := make([]string, 0, len(books))
	for _, b := range books {
		if b.FilePath == "" {
			continue
		}
		pathToBook[b.FilePath] = b
		paths = append(paths, b.FilePath)
	}

	hits := sshStatBatch(host, paths, verbose)

	var missing []book
	for _, p := range paths {
		if !hits[p] {
			missing = append(missing, pathToBook[p])
		}
	}
	return missing
}

// sshStatBatch checks a list of paths via SSH, returns a map path→exists.
func sshStatBatch(host string, paths []string, verbose bool) map[string]bool {
	result := make(map[string]bool, len(paths))
	for i := 0; i < len(paths); i += sshBatchSize {
		end := i + sshBatchSize
		if end > len(paths) {
			end = len(paths)
		}
		batch := paths[i:end]
		hits := runSSHBatch(host, batch, verbose)
		for k, v := range hits {
			result[k] = v
		}
	}
	return result
}

// runSSHBatch runs one ssh invocation for a slice of paths.
func runSSHBatch(host string, paths []string, verbose bool) map[string]bool {
	// Build a shell script: for each path emit HIT:<path> or MISS:<path>.
	var sb strings.Builder
	for _, p := range paths {
		// Quote path for shell safety (single-quote, escape any single quotes).
		safe := strings.ReplaceAll(p, "'", "'\\''")
		fmt.Fprintf(&sb, "[ -e '%s' ] && echo 'HIT:%s' || echo 'MISS:%s'\n", safe, safe, safe)
	}
	script := sb.String()

	if verbose {
		log.Printf("SSH batch (%d paths) to %s", len(paths), host)
	}

	cmd := exec.Command("ssh", "-o", "StrictHostKeyChecking=no", "-o", "BatchMode=yes", host, script)
	out, err := cmd.Output()
	if err != nil {
		log.Printf("WARN: ssh batch error: %v (marking paths as unknown/missing)", err)
		result := make(map[string]bool, len(paths))
		for _, p := range paths {
			result[p] = false
		}
		return result
	}

	result := make(map[string]bool, len(paths))
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "HIT:") {
			result[strings.TrimPrefix(line, "HIT:")] = true
		} else if strings.HasPrefix(line, "MISS:") {
			result[strings.TrimPrefix(line, "MISS:")] = false
		}
	}
	return result
}

// statExists checks a single path, either locally or via SSH.
func statExists(path, sshHost string) bool {
	if sshHost != "" {
		hits := runSSHBatch(sshHost, []string{path}, false)
		return hits[path]
	}
	_, err := os.Stat(path)
	return err == nil
}

// ----- Candidate probing -----

func findCandidate(b book, sshHost string, verbose bool) (matchRecord, bool) {
	parent := filepath.Dir(b.FilePath)
	title := b.Title

	// 1. Doubled-folder pattern: parent/<title> - <title>/
	doubled := filepath.Join(parent, title+" - "+title)
	if statExists(doubled, sshHost) {
		count := countAudioFiles(doubled, sshHost)
		if verbose {
			log.Printf("MATCH doubled-folder: %s %q → %s (%d audio files)", b.ID, title, doubled, count)
		}
		return matchRecord{
			bookID:        b.ID,
			title:         title,
			dbPath:        b.FilePath,
			candidatePath: doubled,
			candidateKind: "doubled-folder",
			audioCount:    count,
		}, true
	}

	// 2. Single-file variants: parent/<title>.<ext>
	for _, ext := range audioExt {
		candidate := filepath.Join(parent, title+ext)
		if statExists(candidate, sshHost) {
			if verbose {
				log.Printf("MATCH single-file: %s %q → %s", b.ID, title, candidate)
			}
			return matchRecord{
				bookID:        b.ID,
				title:         title,
				dbPath:        b.FilePath,
				candidatePath: candidate,
				candidateKind: "single-file" + ext,
				audioCount:    1,
			}, true
		}
	}

	if verbose {
		log.Printf("NO MATCH: %s %q (db_path=%s)", b.ID, title, b.FilePath)
	}
	return matchRecord{}, false
}

// countAudioFiles counts audio files in a directory (local or SSH).
func countAudioFiles(dir, sshHost string) int {
	if sshHost != "" {
		return countAudioFilesSSH(dir, sshHost)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		for _, ae := range audioExt {
			if ext == ae {
				count++
				break
			}
		}
	}
	return count
}

func countAudioFilesSSH(dir, host string) int {
	safe := strings.ReplaceAll(dir, "'", "'\\''")
	script := fmt.Sprintf("ls -1 '%s' 2>/dev/null | grep -cEi '\\.(m4b|mp3|m4a|ogg|flac|aac|opus)$' || echo 0", safe)
	cmd := exec.Command("ssh", "-o", "StrictHostKeyChecking=no", "-o", "BatchMode=yes", host, script)
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	n := 0
	fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &n)
	return n
}

// ----- CSV output -----

func writeCSV(w io.Writer, records []matchRecord) error {
	cw := csv.NewWriter(w)
	// Header.
	if err := cw.Write([]string{
		"book_id", "title", "db_path", "candidate_path", "candidate_kind", "audio_file_count",
	}); err != nil {
		return err
	}
	for _, r := range records {
		if err := cw.Write([]string{
			r.bookID,
			r.title,
			r.dbPath,
			r.candidatePath,
			r.candidateKind,
			fmt.Sprintf("%d", r.audioCount),
		}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
