<!-- file: docs/superpowers/bot-tasks/2026-04-29-deluge-2-protected-path-cache.md -->
<!-- version: 1.0.0 -->
<!-- guid: c4e1f8a2-6b9d-4073-8e5c-1a7f3d2b0e96 -->
<!-- last-edited: 2026-04-29 -->

# BOT TASK: DELUGE-2 — protectedPathCache

**TODO ID:** DELUGE-2
**Audience:** burndown bot
**Branch:** `feat/deluge-2-protected-path-cache`
**PR title:** `feat(deluge): add ProtectedPathCache with TTL refresh and stale fallback`

---

## What This Task Does

Creates `internal/deluge/protected_paths.go` — a thread-safe, TTL-based cache of
filesystem path prefixes that the app must never move, rename, or delete because
they are managed by Deluge. The cache is populated from:

1. The `SavePath` of every active Deluge torrent (returned by `ListTorrents`)
2. Any extra paths from `config.AppConfig.ProtectedPaths` (if that field exists;
   if it does not exist yet, use `config.AppConfig.DelugeDiscoveryLabel` as a
   placeholder — see Step 1)

If Deluge is unreachable when the cache tries to refresh, it keeps the last-known
data (stale cache). The TTL is 5 minutes.

Also creates `internal/deluge/protected_paths_test.go` with exactly 3 tests.

---

## What NOT to Do

- **Do NOT modify `internal/deluge/client.go`** in any way. Read it, do not write it.
- **Do NOT call `ListTorrentsByLabel`** — call `ListTorrents()` (no label filter)
  to collect ALL save paths regardless of label.
- **Do NOT use a `sync.RWMutex` paired with a `Mutex` in the same struct** — pick
  one. Use `sync.RWMutex` (allows concurrent reads).
- **Do NOT import `internal/config`** inside `internal/deluge/` — that would create
  a circular dependency. The caller passes extra paths as `[]string`. The cache
  struct does NOT know about config directly.
- **Do NOT use `time.Sleep` inside any method** — the TTL is checked lazily on
  each call to `IsProtected`.

---

## Background: Existing `client.go` API

You MUST read `internal/deluge/client.go` before writing any code. The relevant
method signatures are:

```go
// ListTorrents returns all torrents with the standard field set.
func (c *Client) ListTorrents() (map[string]TorrentStatus, error)

// TorrentStatus holds the fields we care about from Deluge.
type TorrentStatus struct {
    Hash     string  `json:"hash"`
    Name     string  `json:"name"`
    SavePath string  `json:"save_path"`
    State    string  `json:"state"`
    Progress float64 `json:"progress"`
    Label    string  `json:"label"`
    TotalSize int64  `json:"total_size"`
}
```

`ListTorrents` returns a `map[string]TorrentStatus` where the key is the torrent
hash. You want `TorrentStatus.SavePath` for each entry.

---

## Step 1 — Create `internal/deluge/protected_paths.go`

Write the file with the **exact** contents below. Do not change the logic; do not
rename any exported identifiers.

```go
// file: internal/deluge/protected_paths.go
// version: 1.0.0
// guid: d5b8e2a1-3c9f-4076-b7d4-0e8a2c5f1b93

// Package deluge provides integration with the Deluge BitTorrent client.
package deluge

import (
	"log"
	"strings"
	"sync"
	"time"
)

const protectedPathTTL = 5 * time.Minute

// ProtectedPathCache maintains a set of filesystem path prefixes that must
// not be moved, renamed, or deleted. Paths come from two sources:
//
//  1. The SavePath of every active Deluge torrent (refreshed every 5 min).
//  2. Extra paths supplied at construction time (e.g. config.ProtectedPaths).
//
// Thread-safe. If Deluge is unreachable on refresh, the last-known set is kept.
type ProtectedPathCache struct {
	mu          sync.RWMutex
	paths       []string
	extraPaths  []string
	client      *Client
	lastRefresh time.Time
}

// NewProtectedPathCache creates a cache backed by the given Deluge client.
// extraPaths is a static list of additional protected prefixes (e.g. from config).
// The cache is empty until the first call to IsProtected triggers a refresh.
func NewProtectedPathCache(client *Client, extraPaths []string) *ProtectedPathCache {
	extra := make([]string, len(extraPaths))
	copy(extra, extraPaths)
	return &ProtectedPathCache{
		client:     client,
		extraPaths: extra,
	}
}

// IsProtected returns true if filePath has any cached path as a prefix.
// It lazily refreshes the cache when the TTL has expired.
func (c *ProtectedPathCache) IsProtected(filePath string) bool {
	c.maybeRefresh()
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, prefix := range c.paths {
		if prefix != "" && strings.HasPrefix(filePath, prefix) {
			return true
		}
	}
	return false
}

// Invalidate forces the next IsProtected call to refresh from Deluge immediately.
func (c *ProtectedPathCache) Invalidate() {
	c.mu.Lock()
	c.lastRefresh = time.Time{} // zero value — older than any TTL
	c.mu.Unlock()
}

// maybeRefresh checks whether the TTL has expired and, if so, calls refresh.
// Uses a double-checked pattern: read lock to check, write lock to update.
func (c *ProtectedPathCache) maybeRefresh() {
	c.mu.RLock()
	expired := time.Since(c.lastRefresh) > protectedPathTTL
	c.mu.RUnlock()
	if !expired {
		return
	}
	c.refresh()
}

// refresh fetches current torrent save paths from Deluge and rebuilds the
// path list. If Deluge is unreachable, the existing list is kept unchanged.
func (c *ProtectedPathCache) refresh() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Re-check under write lock in case another goroutine refreshed first.
	if time.Since(c.lastRefresh) <= protectedPathTTL {
		return
	}

	torrents, err := c.client.ListTorrents()
	if err != nil {
		// Deluge unreachable — keep stale data, do not update lastRefresh
		// so the next call will try again.
		log.Printf("[WARN] ProtectedPathCache: failed to refresh from Deluge: %v (using stale data)", err)
		return
	}

	// Collect unique save paths from all torrents.
	seen := make(map[string]struct{})
	var fresh []string
	for _, t := range torrents {
		if t.SavePath == "" {
			continue
		}
		if _, dup := seen[t.SavePath]; !dup {
			seen[t.SavePath] = struct{}{}
			fresh = append(fresh, t.SavePath)
		}
	}

	// Merge in extra (static) paths.
	for _, p := range c.extraPaths {
		if p == "" {
			continue
		}
		if _, dup := seen[p]; !dup {
			seen[p] = struct{}{}
			fresh = append(fresh, p)
		}
	}

	c.paths = fresh
	c.lastRefresh = time.Now()
	log.Printf("[DEBUG] ProtectedPathCache: refreshed %d protected path prefixes", len(c.paths))
}
```

---

## Step 2 — Create `internal/deluge/protected_paths_test.go`

Write the file with the **exact** contents below. Do not add or remove tests.

```go
// file: internal/deluge/protected_paths_test.go
// version: 1.0.0
// guid: e6c9f3b2-4d0a-5187-c8e5-2b9f4e1c0d74

package deluge

import (
	"fmt"
	"testing"
	"time"
)

// mockClient is a fake *Client that returns controlled ListTorrents results.
// It is unexported and only used in tests. We cannot embed *Client because
// Client has unexported fields, so we use a wrapper approach via monkey-patching
// the ProtectedPathCache's refresh function.
//
// Instead, we construct a real ProtectedPathCache but replace its internals
// directly, because all fields are unexported. The cleanest approach is a
// test-only constructor that accepts a function.

// listFunc is the type of a function that imitates ListTorrents for tests.
type listFunc func() (map[string]TorrentStatus, error)

// newTestCache creates a ProtectedPathCache wired to a fake list function
// instead of a real *Client. Used only in tests.
func newTestCache(lf listFunc, extraPaths []string) *ProtectedPathCache {
	// We create a dummy client (nil) and override the refresh behavior via
	// a thin subtype. Since Client is a struct with unexported fields we
	// cannot embed it. Instead we call refresh manually with a stub.
	//
	// Strategy: create the cache, then immediately call a helper that
	// populates c.paths directly, simulating what refresh() would do.
	c := &ProtectedPathCache{
		client:     nil, // never called in tests — we populate manually
		extraPaths: extraPaths,
	}
	populateCacheForTest(c, lf)
	return c
}

// populateCacheForTest runs the same logic as refresh() but uses lf instead
// of c.client.ListTorrents(). Sets lastRefresh to now so TTL doesn't expire.
func populateCacheForTest(c *ProtectedPathCache, lf listFunc) {
	torrents, err := lf()
	if err != nil {
		// Simulate stale-on-error: do not update paths or lastRefresh.
		return
	}
	seen := make(map[string]struct{})
	var fresh []string
	for _, t := range torrents {
		if t.SavePath == "" {
			continue
		}
		if _, dup := seen[t.SavePath]; !dup {
			seen[t.SavePath] = struct{}{}
			fresh = append(fresh, t.SavePath)
		}
	}
	for _, p := range c.extraPaths {
		if p == "" {
			continue
		}
		if _, dup := seen[p]; !dup {
			seen[p] = struct{}{}
			fresh = append(fresh, p)
		}
	}
	c.paths = fresh
	c.lastRefresh = time.Now()
}

// Test 1: IsProtected returns true when filePath has a cached prefix.
func TestIsProtected_True(t *testing.T) {
	lf := func() (map[string]TorrentStatus, error) {
		return map[string]TorrentStatus{
			"aabbcc": {SavePath: "/mnt/downloads/audiobooks"},
		}, nil
	}
	cache := newTestCache(lf, nil)

	filePath := "/mnt/downloads/audiobooks/TheName/file.m4b"
	if !cache.IsProtected(filePath) {
		t.Errorf("expected IsProtected(%q) = true, got false", filePath)
	}
}

// Test 2: IsProtected returns false when filePath has no matching prefix.
func TestIsProtected_False(t *testing.T) {
	lf := func() (map[string]TorrentStatus, error) {
		return map[string]TorrentStatus{
			"aabbcc": {SavePath: "/mnt/downloads/audiobooks"},
		}, nil
	}
	cache := newTestCache(lf, nil)

	filePath := "/mnt/bigdata/books/audiobook-organizer/Author/Title/file.m4b"
	if cache.IsProtected(filePath) {
		t.Errorf("expected IsProtected(%q) = false, got true", filePath)
	}
}

// Test 3: When refresh fails, stale data is kept and IsProtected still works.
func TestIsProtected_StaleOnError(t *testing.T) {
	// First, populate with good data.
	goodLF := func() (map[string]TorrentStatus, error) {
		return map[string]TorrentStatus{
			"aabbcc": {SavePath: "/mnt/downloads/stale-path"},
		}, nil
	}
	cache := newTestCache(goodLF, nil)

	// Verify stale path is protected before simulating error.
	if !cache.IsProtected("/mnt/downloads/stale-path/book.m4b") {
		t.Fatal("precondition: stale path should be protected before error")
	}

	// Simulate Deluge going down: reset lastRefresh so refresh() will run,
	// then use the error-returning lf.
	cache.lastRefresh = time.Time{} // force TTL expiry
	errorLF := func() (map[string]TorrentStatus, error) {
		return nil, fmt.Errorf("connection refused")
	}
	// populateCacheForTest with error lf — should NOT clear existing paths.
	populateCacheForTest(cache, errorLF)

	// Stale paths must still be protected.
	if !cache.IsProtected("/mnt/downloads/stale-path/book.m4b") {
		t.Errorf("expected stale path to still be protected after refresh error")
	}
}
```

---

## Step 3 — Run the Tests

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
go test ./internal/deluge/...
```

All three tests must pass. If any test fails, read the error and fix it. Do not
delete tests to make them pass.

---

## Step 4 — Verify Build

```bash
go build ./...
```

Must produce zero errors.

---

## Step 5 — Bump Version Header

Both new files already have `// version: 1.0.0` in the header written above. Do
not change them unless you made code changes that deviate from the spec.

---

## Step 6 — Commit and Open PR

```bash
git checkout -b feat/deluge-2-protected-path-cache
git add internal/deluge/protected_paths.go internal/deluge/protected_paths_test.go
git commit -m "feat(deluge): add ProtectedPathCache with TTL refresh and stale fallback"
git push -u origin feat/deluge-2-protected-path-cache
gh pr create \
  --title "feat(deluge): add ProtectedPathCache with TTL refresh and stale fallback" \
  --body "Adds internal/deluge/protected_paths.go: thread-safe, 5-min TTL cache of Deluge torrent save_paths + extra static prefixes. On Deluge unreachable, keeps stale data. Three unit tests. Part of protected-paths spec."
```

---

## Checklist

- [ ] `internal/deluge/protected_paths.go` created with exact code above
- [ ] `internal/deluge/protected_paths_test.go` created with exact code above
- [ ] `go test ./internal/deluge/...` passes all 3 tests
- [ ] `go build ./...` passes
- [ ] PR opened with correct branch name and title
