<!-- file: docs/superpowers/bot-tasks/2026-04-30-sec-1-browse-allowlist.md -->
<!-- version: 1.1.0 -->
<!-- guid: c7d8e9f0-a1b2-3456-cdef-789012345abc -->
<!-- last-edited: 2026-04-30 -->

# BOT TASK: SEC-1 — Restrict BrowseDirectory to Configured Import Paths

**TODO ID:** SEC-1
**Audience:** burndown bot
**Branch:** `fix/browse-dir-allowlist`
**PR title:** `fix(security): restrict BrowseDirectory to configured import paths`

---

## What This Task Does

Adds an allowlist check to `BrowseDirectory` in
`internal/server/filesystem_service.go` so that only paths under `config.RootDir`
or `config.ImportPaths` can be browsed. Any other path returns HTTP 403.

---

## What NOT to Do

- **Do NOT break** browsing for paths that ARE within the allowed directories.
- **Do NOT change** the response format for valid paths.
- **Do NOT add** authentication — that is a separate issue (SEC-2).
- **Do NOT use** `strings.HasPrefix(absPath, allowedPrefix)` without accounting for
  directory separator — `/etc` must not match `/etc-evil`.

---

## Read First

1. `internal/server/filesystem_service.go:35–79` — read `BrowseDirectory` fully.
   Understand: how is the `path` param received? How is `absPath` computed?
   Where does the function return results?
2. `internal/server/filesystem_handlers.go:43` — the handler that calls
   `BrowseDirectory`. Understand how config is accessed (is it a field on the
   server struct? a global?).
3. `internal/config/config.go` — find the fields for:
   - `RootDir` (or equivalent library root path)
   - `ImportPaths` (a `[]string` of paths users can browse/import from)

---

## Steps

### Step 1 — Understand the current code

Read `BrowseDirectory` (lines 35–79). Note:
- Where `absPath` is set (via `filepath.Abs` or similar)
- Where the function returns a result vs. an error

### Step 2 — Build the allowlist

The allowlist has two layers — hardcoded safe prefixes (cover the common cases
with zero config) plus the user's configured paths.

After `absPath` is computed, add a check. The check must handle:
- `allowedPrefix` == `absPath` (exact match — browsing the root itself is OK)
- `absPath` starts with `allowedPrefix + "/"` (subdirectory is OK)
- Anything else → 403

```go
// defaultBrowseAllowPrefixes are safe root paths that work out-of-the-box.
// /home, /media, /mnt cover typical Linux desktop/server layouts.
// /audiobooks and /data cover Docker volume conventions.
var defaultBrowseAllowPrefixes = []string{
    "/home",
    "/media",
    "/mnt",
    "/audiobooks",
    "/data",
}

func isAllowedPath(absPath string, cfg *config.Config) bool {
    // Build full allowlist: hardcoded defaults + config-derived paths.
    allowed := make([]string, 0, len(defaultBrowseAllowPrefixes)+len(cfg.ImportPaths)+1)
    allowed = append(allowed, defaultBrowseAllowPrefixes...)
    if cfg.RootDir != "" {
        allowed = append(allowed, cfg.RootDir)
    }
    allowed = append(allowed, cfg.ImportPaths...)

    for _, prefix := range allowed {
        if prefix == "" {
            continue
        }
        // Normalize: no trailing slash
        prefix = strings.TrimRight(prefix, string(os.PathSeparator))
        if absPath == prefix ||
            strings.HasPrefix(absPath, prefix+string(os.PathSeparator)) {
            return true
        }
    }
    return false
}
```

### Step 3 — Insert the check in BrowseDirectory

After the line that sets `absPath`, add:

```go
if !isAllowedPath(absPath, s.config) { // adjust s.config to the actual config accessor
    c.JSON(http.StatusForbidden, gin.H{"error": "path not in allowed directories"})
    return
}
```

Import `"net/http"`, `"strings"`, `"os"` if not already imported.

### Step 4 — Verify

```bash
cd /Users/jdfalk/.worktrees/audiobook-eval
go build ./...
go vet ./...
```

Manual verification (if server can be run locally):
```bash
# Should return 403
curl -s "http://localhost:PORT/api/v1/filesystem/browse?path=/etc" | jq .

# Should return 200 (use an actual configured import path)
curl -s "http://localhost:PORT/api/v1/filesystem/browse?path=/path/to/import" | jq .
```

### Step 5 — Commit and open PR

```bash
git checkout -b fix/browse-dir-allowlist
git add internal/server/filesystem_service.go
git commit -m "fix(security): restrict BrowseDirectory to configured import paths

Adds isAllowedPath check after filepath.Abs resolution. Returns HTTP 403
if the requested path is not under config.RootDir or config.ImportPaths.
Uses separator-aware prefix check to prevent /etc matching /etc-evil.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin fix/browse-dir-allowlist
gh pr create \
  --title "fix(security): restrict BrowseDirectory to configured import paths" \
  --body "Prevents traversal to arbitrary filesystem paths via the browse API. Returns 403 for paths outside RootDir and ImportPaths. Security fix S-1."
```

---

## Checklist

- [ ] `isAllowedPath` helper function added
- [ ] Check inserted in `BrowseDirectory` after `absPath` is computed
- [ ] Returns HTTP 403 with `{"error": "path not in allowed directories"}` for disallowed paths
- [ ] Separator-aware prefix check (no `/etc` matching `/etc-evil`)
- [ ] `go build ./...` passes
- [ ] `go vet ./...` clean
- [ ] PR opened with correct branch and title
