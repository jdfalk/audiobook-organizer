<!-- file: docs/plans/download-client-integration.md -->
<!-- version: 1.2.0 -->
<!-- guid: e4f5a6b7-c8d9-0e1f-2a3b-4c5d6e7f8a9b -->
<!-- last-edited: 2026-01-31 -->

# Download Client Integration

## Overview

Integrate with popular torrent and Usenet clients to automatically detect,
import, and manage audiobook downloads. Includes seeding-preservation support
via shadow directories.

**Priority**: Post-MVP (vNext v1.2)

---

## Deluge Torrent Integration

- Connect to Deluge RPC API
- Monitor downloads folder for completed torrents
- Auto-import completed audiobooks into the library
- Show torrent status in Book Detail (seeding / completed / removed)
- Optional: remove torrent after successful import (configurable)

## SABnzbd NZB Integration

- Connect to SABnzbd API
- Monitor downloads folder for completed NZBs
- Auto-import completed audiobooks
- Link back to NZB source for re-download if needed
- Show download history and repair status

## qBittorrent Web API Integration

- Connect via qBittorrent Web API
- Same auto-import and status tracking as Deluge

---

## Torrent Client API Abstraction

All three download clients (Deluge, qBittorrent, SABnzbd) are hidden behind a
single Go interface. This lets the rest of the system (shadow creation, cleanup
job, status reporting) operate on one code path regardless of which client the
user has configured. Each concrete implementation lives in its own file under
`internal/download/`.

### Interface definition

```go
// internal/download/client.go

package download

import (
    "context"
    "time"
)

// TorrentInfo is the read-only view of a single torrent that the organizer
// needs. Fields map directly to the native API responses of each client;
// the concrete adapters translate.
type TorrentInfo struct {
    ID              string        // Client-opaque identifier (hash or numeric ID)
    Name            string        // User-visible name / directory name
    DownloadDir     string        // Current download path on disk
    Status          TorrentStatus // Normalized state
    Progress        float64       // 0.0 – 1.0, download completion
    TotalUploaded   int64         // Lifetime bytes uploaded (for activity tracking)
    TotalDownloaded int64         // Lifetime bytes downloaded
    Files           []TorrentFile // Individual files inside this torrent
    CreatedAt       time.Time     // When the torrent was added to the client
    IsPaused        bool
}

type TorrentFile struct {
    Path string // Relative path inside the torrent
    Size int64  // File size in bytes
}

type TorrentStatus int

const (
    StatusDownloading TorrentStatus = iota
    StatusSeeding
    StatusPaused
    StatusStopped    // Finished but not seeding (client-specific)
    StatusNotFound   // Torrent no longer exists in client
)

// UploadStats is a lightweight snapshot for the cleanup job.
// Returned by GetUploadStats to avoid pulling the full TorrentInfo
// on every poll cycle.
type UploadStats struct {
    TotalUploaded int64
    IsPaused      bool
    Exists        bool // false when the torrent has been removed from the client
}

// TorrentClient abstracts a download client. All methods accept a context so
// network calls respect operation cancellation from the queue system.
type TorrentClient interface {
    // Connect validates credentials and returns an error if the client
    // is unreachable. Called once at startup and on config change.
    Connect(ctx context.Context) error

    // GetTorrent returns full info for a single torrent by its client ID.
    // Returns nil, nil when the torrent does not exist (not an error).
    GetTorrent(ctx context.Context, id string) (*TorrentInfo, error)

    // GetUploadStats is a lightweight poll used by the shadow cleanup job.
    // It returns only the fields the cleanup loop needs, avoiding the cost
    // of unmarshaling the full torrent object on every cycle.
    GetUploadStats(ctx context.Context, id string) (*UploadStats, error)

    // SetDownloadPath relocates a torrent to a new directory on disk.
    // Used after shadow link creation to point the client at the shadow dir.
    SetDownloadPath(ctx context.Context, id, newPath string) error

    // RemoveTorrent removes the torrent from the client. If deleteFiles is
    // true, the client also deletes the associated files from disk.
    RemoveTorrent(ctx context.Context, id string, deleteFiles bool) error

    // ListCompleted returns all torrents that have reached 100% download
    // completion. Used by the auto-import poller.
    ListCompleted(ctx context.Context) ([]TorrentInfo, error)

    // ClientType returns a human-readable label ("deluge", "qbittorrent",
    // "sabnzbd") for logging and config disambiguation.
    ClientType() string
}
```

### Concrete adapter sketch (Deluge)

```go
// internal/download/deluge.go

type DelugeClient struct {
    host     string
    port     int
    username string
    password string
    // connection pool or session reuse handled internally
}

func NewDelugeClient(cfg DelugeConfig) *DelugeClient { ... }

func (d *DelugeClient) Connect(ctx context.Context) error {
    // Authenticate via Deluge RPC JSON-RPC protocol.
    // Store session cookie / auth token for subsequent calls.
    ...
}

func (d *DelugeClient) GetUploadStats(ctx context.Context, id string) (*UploadStats, error) {
    // RPC call: ("core.get_torrent_data", id, ["total_uploaded", "paused", "state"])
    // Map response into UploadStats. Return Exists=false on 404 / not-found.
    ...
}

// ClientType implements TorrentClient.
func (d *DelugeClient) ClientType() string { return "deluge" }
```

### Factory and registration

```go
// internal/download/factory.go

func NewClientFromConfig(cfg *config.Config) (TorrentClient, error) {
    switch cfg.DownloadClient.Type {
    case "deluge":
        return NewDelugeClient(cfg.DownloadClient.Deluge), nil
    case "qbittorrent":
        return NewQBittorrentClient(cfg.DownloadClient.QBittorrent), nil
    case "sabnzbd":
        return NewSABnzbdClient(cfg.DownloadClient.SABnzbd), nil
    case "":
        return nil, nil // download integration disabled
    default:
        return nil, fmt.Errorf("unsupported download client type: %s", cfg.DownloadClient.Type)
    }
}
```

---

## Seeding Preservation (Shadow Directory)

After organizing audiobooks into the library, torrent clients need the original
file structure to continue seeding. The shadow directory holds the original
(unmodified) files so the torrent client can keep seeding while the library
copy gets metadata written to it.

### How it works

1. Torrent completes download to e.g. `/downloads/torrent-name/file.m4b`
2. We organize: copy the file into the library at
   `/library/Author/Book/file.m4b`
3. We create a shadow hard link at `/seeding/torrent-name/file.m4b` pointing to
   the original downloaded file (preserving the torrent's directory structure)
4. We tell the torrent client to update its path to `/seeding/torrent-name/`
5. We write metadata to the library copy — this breaks the hard link (metadata
   writers use temp + rename), leaving the shadow with the original unmodified
   bytes that the torrent client needs for hash verification
6. Result: library has metadata-enriched copy, shadow has original for seeding,
   storage cost is 2x until shadow is cleaned up

### Cross-filesystem handling

Hard links only work within the same filesystem. Strategy in priority order:

1. **Reflink (CoW)** — try first. Works on btrfs, APFS, XFS. Shares blocks on
   disk until one side is modified, so effectively 1x storage until the metadata
   write happens. Already supported by the existing safe file ops pipeline
   (`reflink → hardlink → copy`).
2. **Hard link** — works if shadow dir and downloads are on the same filesystem.
3. **Copy** — last resort fallback. Full 2x storage from the start.
4. **Warn and skip** — if the user's layout makes all of the above impractical,
   surface a warning during setup and skip shadow creation for that torrent.

### Shadow directory creation — Go implementation

The shadow link is created immediately after `OrganizeBook` succeeds. The
creation function mirrors the project's existing `reflink → hardlink → copy`
fallback chain (the same chain used in `internal/organizer/organizer.go`
`OrganizeBook` when `strategy == "auto"`). It also follows the SHA256
verification pattern from `internal/fileops/safe_operations.go`.

```go
// internal/download/shadow.go

package download

import (
    "crypto/sha256"
    "fmt"
    "io"
    "os"
    "path/filepath"

    "github.com/jdfalk/audiobook-organizer/internal/fileops"
)

// ShadowLinkResult records what happened when we created (or failed to create)
// a shadow link for one file.
type ShadowLinkResult struct {
    OriginalPath string
    ShadowPath   string
    Method       string // "reflink", "hardlink", "copy", or "skipped"
    Err          error
}

// CreateShadowLinks creates the shadow directory structure for a completed
// torrent. downloadDir is the original torrent download path (e.g.
// /downloads/torrent-name/). shadowBase is the configured shadow root (e.g.
// /seeding/). torrentName is the directory name the torrent client expects.
// files is the list of files inside the torrent (from TorrentInfo.Files).
//
// The function preserves the exact relative directory structure that the
// torrent client needs for hash verification.
func CreateShadowLinks(downloadDir, shadowBase, torrentName string, files []TorrentFile) []ShadowLinkResult {
    shadowTorrentDir := filepath.Join(shadowBase, torrentName)
    results := make([]ShadowLinkResult, 0, len(files))

    for _, f := range files {
        srcPath := filepath.Join(downloadDir, f.Path)
        dstPath := filepath.Join(shadowTorrentDir, f.Path)

        // Ensure the target directory exists (torrents can have nested folders)
        if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
            results = append(results, ShadowLinkResult{
                OriginalPath: srcPath, ShadowPath: dstPath,
                Method: "skipped", Err: fmt.Errorf("mkdir: %w", err),
            })
            continue
        }

        result := createSingleShadowLink(srcPath, dstPath)
        results = append(results, result)
    }

    return results
}

// createSingleShadowLink attempts reflink → hardlink → copy for one file,
// then verifies the result with SHA256.
func createSingleShadowLink(src, dst string) ShadowLinkResult {
    base := ShadowLinkResult{OriginalPath: src, ShadowPath: dst}

    // 1. Try reflink (copy-on-write). Uses clone_file on macOS / FICLONE
    //    ioctl on Linux. The organizer already has platform-specific
    //    reflinkFilePlatform(); reuse that pattern here.
    if err := reflinkFile(src, dst); err == nil {
        base.Method = "reflink"
        return base
    }

    // 2. Try hard link — zero storage cost, same inode.
    if err := os.Link(src, dst); err == nil {
        base.Method = "hardlink"
        return base
    }

    // 3. Fall back to full copy with SHA256 post-verification.
    if err := safeCopyWithVerify(src, dst); err != nil {
        base.Method = "skipped"
        base.Err = fmt.Errorf("all link strategies failed: %w", err)
        return base
    }

    base.Method = "copy"
    return base
}

// safeCopyWithVerify copies src → dst and verifies the SHA256 matches.
// On mismatch, the destination is removed and an error is returned.
// This mirrors the pattern in fileops.SafeCopy / FileOperation.Execute.
func safeCopyWithVerify(src, dst string) error {
    // Compute source hash first
    srcHash, err := fileops.ComputeFileHash(src)
    if err != nil {
        return fmt.Errorf("source hash: %w", err)
    }

    // Copy
    srcFile, err := os.Open(src)
    if err != nil {
        return err
    }
    defer srcFile.Close()

    dstFile, err := os.Create(dst)
    if err != nil {
        return err
    }

    if _, err := io.Copy(dstFile, srcFile); err != nil {
        dstFile.Close()
        os.Remove(dst)
        return fmt.Errorf("copy: %w", err)
    }
    if err := dstFile.Sync(); err != nil {
        dstFile.Close()
        os.Remove(dst)
        return fmt.Errorf("sync: %w", err)
    }
    dstFile.Close()

    // Verify
    dstHash, err := fileops.ComputeFileHash(dst)
    if err != nil {
        os.Remove(dst)
        return fmt.Errorf("destination hash: %w", err)
    }
    if srcHash != dstHash {
        os.Remove(dst)
        return fmt.Errorf("checksum mismatch: src=%s dst=%s", srcHash, dstHash)
    }

    return nil
}

// reflinkFile is the platform-specific reflink attempt. Thin wrapper over
// the same ioctl / clone_file calls used in organizer.reflinkFilePlatform.
func reflinkFile(src, dst string) error {
    // Delegate to the organizer's platform implementation.
    // In practice this is compiled per-platform (reflink_darwin.go, reflink_linux.go).
    return organizer.ReflinkFile(src, dst)
}
```

### Shadow cleanup job — Go implementation

The cleanup job runs as a long-lived goroutine, started during server init
alongside the operation queue workers. It owns a ticker and a small in-memory
state map that tracks `last_activity_at` and `last_uploaded_bytes` per shadow.
The persistent shadow metadata (created_at, torrent ID, status) is stored in
PebbleDB under the `shadow:` key prefix (see Per-torrent override storage
below).

```go
// internal/download/shadow_cleanup.go

package download

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "os"
    "time"

    "github.com/cockroachdb/pebble"
    "github.com/jdfalk/audiobook-organizer/internal/database"
)

// ShadowRecord is persisted in PebbleDB and represents one active shadow.
// Key: shadow:<torrent_id>
type ShadowRecord struct {
    TorrentID       string    `json:"torrent_id"`
    TorrentName     string    `json:"torrent_name"`
    ShadowDir       string    `json:"shadow_dir"`       // Absolute path to shadow torrent dir
    CreatedAt       time.Time `json:"created_at"`
    LastActivityAt  time.Time `json:"last_activity_at"`
    LastUploadBytes int64     `json:"last_upload_bytes"`
    PauseStartedAt  *time.Time `json:"pause_started_at,omitempty"`
    Status          string    `json:"status"` // "active", "inactive", "pending_removal"
}

// ShadowCleanupConfig holds the resolved thresholds for one shadow.
// The caller resolves the override chain (global → per-client → per-torrent)
// before passing this in.
type ShadowCleanupConfig struct {
    InactivityWindowDays       int
    UploadActivityThresholdPct int
    PauseTimeoutDays           int
    MaxLifetimeEnabled         bool
    MaxLifetimeDays            int
}

// ShadowCleanupJob polls all active shadows and applies the cleanup logic.
type ShadowCleanupJob struct {
    client   TorrentClient
    store    database.Store
    interval time.Duration // How often to poll (e.g. 5 minutes)
}

// NewShadowCleanupJob creates the cleanup job. store is the global PebbleDB
// store (database.GlobalStore). client is the active TorrentClient instance.
func NewShadowCleanupJob(client TorrentClient, store database.Store, interval time.Duration) *ShadowCleanupJob {
    return &ShadowCleanupJob{client: client, store: store, interval: interval}
}

// Run blocks and polls until ctx is canceled (server shutdown).
func (j *ShadowCleanupJob) Run(ctx context.Context) {
    ticker := time.NewTicker(j.interval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            j.tick(ctx)
        }
    }
}

func (j *ShadowCleanupJob) tick(ctx context.Context) {
    shadows, err := j.loadAllShadows()
    if err != nil {
        log.Printf("[WARN] shadow cleanup: failed to load shadows: %v", err)
        return
    }

    for _, shadow := range shadows {
        if shadow.Status == "pending_removal" {
            j.removeShadow(shadow)
            continue
        }

        cfg := j.resolveConfig(shadow.TorrentID)
        j.evaluateShadow(ctx, &shadow, cfg)
    }
}

func (j *ShadowCleanupJob) evaluateShadow(ctx context.Context, shadow *ShadowRecord, cfg ShadowCleanupConfig) {
    // --- Check 1: Max lifetime hard cap (runs first, unconditionally) ---
    if cfg.MaxLifetimeEnabled {
        hardDeadline := shadow.CreatedAt.Add(time.Duration(cfg.MaxLifetimeDays) * 24 * time.Hour)
        if time.Now().After(hardDeadline) {
            log.Printf("[INFO] shadow cleanup: %s exceeded max_lifetime (%d days), marking for removal",
                shadow.TorrentID, cfg.MaxLifetimeDays)
            shadow.Status = "pending_removal"
            j.persistShadow(shadow)
            return
        }
    }

    // --- Poll the torrent client ---
    stats, err := j.client.GetUploadStats(ctx, shadow.TorrentID)
    if err != nil {
        // Client unreachable — fail safe, do nothing (except max lifetime above)
        log.Printf("[WARN] shadow cleanup: client unreachable for %s: %v", shadow.TorrentID, err)
        return
    }

    // --- Check 2: Torrent removed from client ---
    if !stats.Exists {
        log.Printf("[INFO] shadow cleanup: torrent %s no longer in client, marking for removal",
            shadow.TorrentID)
        shadow.Status = "pending_removal"
        j.persistShadow(shadow)
        return
    }

    // --- Check 3: Pause handling ---
    if stats.IsPaused {
        if shadow.PauseStartedAt == nil {
            now := time.Now()
            shadow.PauseStartedAt = &now
            j.persistShadow(shadow)
        }
        pauseDeadline := shadow.PauseStartedAt.Add(time.Duration(cfg.PauseTimeoutDays) * 24 * time.Hour)
        if time.Now().After(pauseDeadline) {
            log.Printf("[INFO] shadow cleanup: %s paused for %d days, marking for removal",
                shadow.TorrentID, cfg.PauseTimeoutDays)
            shadow.Status = "pending_removal"
            j.persistShadow(shadow)
        }
        return // No activity check while paused
    }

    // Torrent is not paused — clear any pause timer
    if shadow.PauseStartedAt != nil {
        shadow.PauseStartedAt = nil
    }

    // --- Check 4: Activity check ---
    uploadIncrease := stats.TotalUploaded - shadow.LastUploadBytes
    // Compute the total size of all files in this shadow (sum of file sizes).
    // In practice this is cached from the original TorrentInfo at shadow creation time.
    totalFileSize := j.getShadowTotalSize(shadow)
    threshold := int64(float64(totalFileSize) * float64(cfg.UploadActivityThresholdPct) / 100.0)

    if uploadIncrease >= threshold && threshold > 0 {
        // Meaningful activity detected — reset inactivity clock
        shadow.LastActivityAt = time.Now()
        shadow.LastUploadBytes = stats.TotalUploaded
        shadow.Status = "active"
    } else {
        // No meaningful activity this cycle
        shadow.LastUploadBytes = stats.TotalUploaded
        inactivityDeadline := shadow.LastActivityAt.Add(
            time.Duration(cfg.InactivityWindowDays) * 24 * time.Hour)
        if time.Now().After(inactivityDeadline) {
            log.Printf("[INFO] shadow cleanup: %s inactive for %d days, marking for removal",
                shadow.TorrentID, cfg.InactivityWindowDays)
            shadow.Status = "pending_removal"
        }
    }

    j.persistShadow(shadow)
}

// removeShadow deletes the shadow directory from disk and removes the
// PebbleDB record.
func (j *ShadowCleanupJob) removeShadow(shadow ShadowRecord) {
    if err := os.RemoveAll(shadow.ShadowDir); err != nil {
        log.Printf("[WARN] shadow cleanup: failed to remove %s: %v", shadow.ShadowDir, err)
        return
    }
    // Delete PebbleDB key: shadow:<torrent_id>
    if ps, ok := j.store.(*database.PebbleStore); ok {
        key := []byte(fmt.Sprintf("shadow:%s", shadow.TorrentID))
        _ = ps.Delete(key)
    }
    log.Printf("[INFO] shadow cleanup: removed shadow for %s at %s", shadow.TorrentID, shadow.ShadowDir)
}

// persistShadow writes the updated ShadowRecord back to PebbleDB.
func (j *ShadowCleanupJob) persistShadow(shadow *ShadowRecord) {
    data, err := json.Marshal(shadow)
    if err != nil {
        log.Printf("[WARN] shadow cleanup: marshal error for %s: %v", shadow.TorrentID, err)
        return
    }
    key := []byte(fmt.Sprintf("shadow:%s", shadow.TorrentID))
    if ps, ok := j.store.(*database.PebbleStore); ok {
        _ = ps.Set(key, data)
    }
}

// loadAllShadows scans PebbleDB for all shadow: keys and deserializes them.
func (j *ShadowCleanupJob) loadAllShadows() ([]ShadowRecord, error) {
    // Iterate over prefix "shadow:" using the same pattern as GetAllImportPaths
    // in pebble_store.go: NewIter with LowerBound/UpperBound.
    // PebbleDB key range: "shadow:" to "shadow;" (semicolon is next byte after colon)
    ...
}

// resolveConfig resolves the override chain for a torrent:
//   global default → per-client override → per-torrent override.
// Per-torrent overrides are stored under key: shadow:override:<torrent_id>
func (j *ShadowCleanupJob) resolveConfig(torrentID string) ShadowCleanupConfig {
    // Start with global defaults from config.AppConfig.DownloadClient.Shadow
    cfg := ShadowCleanupConfig{
        InactivityWindowDays:       config.AppConfig.DownloadClient.Shadow.InactivityWindowDays,
        UploadActivityThresholdPct: config.AppConfig.DownloadClient.Shadow.UploadActivityThresholdPct,
        PauseTimeoutDays:           config.AppConfig.DownloadClient.Shadow.PauseTimeoutDays,
        MaxLifetimeEnabled:         config.AppConfig.DownloadClient.Shadow.MaxLifetimeEnabled,
        MaxLifetimeDays:            config.AppConfig.DownloadClient.Shadow.MaxLifetimeDays,
    }

    // Check for per-torrent override in PebbleDB
    // Key: shadow:override:<torrent_id>
    override := j.loadTorrentOverride(torrentID)
    if override != nil {
        if override.InactivityWindowDays != nil {
            cfg.InactivityWindowDays = *override.InactivityWindowDays
        }
        if override.UploadActivityThresholdPct != nil {
            cfg.UploadActivityThresholdPct = *override.UploadActivityThresholdPct
        }
        // ... same pattern for remaining fields
    }

    return cfg
}
```

### Shadow cleanup job startup

The cleanup job is started in `internal/server/server.go` alongside the
existing heartbeat goroutine, after the operation queue is initialized:

```go
// In the server Run() function, after queue initialization:
if downloadClient != nil && config.AppConfig.DownloadClient.Shadow.Dir != "" {
    cleanupJob := download.NewShadowCleanupJob(
        downloadClient,
        database.GlobalStore,
        5*time.Minute, // poll interval
    )
    go cleanupJob.Run(serverCtx) // serverCtx is canceled on SIGINT/SIGTERM
}
```

### Shadow cleanup policy

Shadow copies are temporary. A background job polls each shadow's torrent and
applies three checks. All thresholds are configurable (see below).

**Activity check** (primary signal):

- Poll `total_uploaded` from the torrent client API each cycle
- Activity only counts if the increase since the last check is ≥
  `upload_activity_threshold_pct` of the file size (default 20%). Filters out
  trickle uploads that don't meaningfully help the network.
- If meaningful activity is detected → update `last_activity_at`, reset
- If no meaningful activity for `inactivity_window_days` → mark for removal

**Pause handling:**

- If the torrent is paused AND fully downloaded: start the `pause_timeout_days`
  countdown
- If it gets unpaused before timeout → cancel countdown, resume
  activity-based tracking
- If it stays paused past timeout → mark for removal
- Handles torrent clients that start torrents paused by default without
  punishing short intentional pauses

**Max lifetime (hard cap):**

- Off by default. When enabled, the shadow is removed on its hard removal date
  regardless of seeding activity, pause state, or anything else.
- Hard removal date = `created_at + max_lifetime_days`
- This check runs first in the cleanup logic. If max lifetime is enabled and
  the date has passed, the shadow is marked for removal immediately — no other
  checks apply.
- Use case: aggressive cleanup when you want to bound your storage risk and
  don't care about preserving seeding past a certain point.

**Client unreachable:**

- If the torrent client is offline or unreachable, do not clean up. Fail safe —
  shadow stays until we can confirm actual status. Prevents data loss from
  temporary connectivity issues.
- Exception: max lifetime hard cap still applies even when client is
  unreachable. It's a hard date, not a status check.

**Torrent removed from client:**

- If the torrent no longer exists in the client at all → mark for removal
  immediately. No grace period needed.

---

## Configurable Settings

All cleanup thresholds follow the same override chain: **global default →
per-client override → per-torrent override**. The most specific value wins.

| Setting | Default | Description |
| --- | --- | --- |
| `shadow_dir` | (none) | Path to shadow seeding directory. Must be configured to enable shadow seeding. |
| `inactivity_window_days` | 30 | Days with no meaningful upload activity before shadow is removed |
| `upload_activity_threshold_pct` | 20 | Minimum upload increase as % of file size to count as activity |
| `pause_timeout_days` | 30 | Days a fully-downloaded torrent can stay paused before shadow is removed |
| `max_lifetime_enabled` | false | When true, shadows are removed on their hard removal date regardless of activity |
| `max_lifetime_days` | 90 | Hard removal date = shadow created_at + this value. Only applies when `max_lifetime_enabled` is true |
| `removal_policy` | `keep_seeding` | `remove` (no shadow), `keep_seeding` (shadow + cleanup), `archive` (keep torrent file for re-download) |

---

## Configuration — Config Struct and YAML

The download client settings are added to `internal/config/config.go` as a
nested struct on `Config`, following the same pattern as `MetadataSources`
(nested struct, loaded via `viper.UnmarshalKey`). Each sub-client has its own
struct for connection details. The shadow settings mirror the table above.

### Go struct additions to `internal/config/config.go`

```go
// Add to the Config struct in internal/config/config.go:

type Config struct {
    // ... existing fields ...

    // Download client integration
    DownloadClient DownloadClientConfig `json:"download_client"`
}

// DownloadClientConfig holds all download-client settings.
type DownloadClientConfig struct {
    Type   string              `json:"type"`   // "deluge", "qbittorrent", "sabnzbd", or ""
    Deluge DelugeConfig        `json:"deluge"`
    QBit   QBittorrentConfig   `json:"qbittorrent"`
    SAB    SABnzbdConfig       `json:"sabnzbd"`
    Shadow ShadowConfig        `json:"shadow"`
}

type DelugeConfig struct {
    Host     string `json:"host"`
    Port     int    `json:"port"`
    Username string `json:"username"`
    Password string `json:"password"` // stored via encrypted setting; see database/settings.go
}

type QBittorrentConfig struct {
    URL      string `json:"url"`      // e.g. "http://localhost:8080"
    Username string `json:"username"`
    Password string `json:"password"`
}

type SABnzbdConfig struct {
    URL    string `json:"url"`    // e.g. "http://localhost:8080"
    APIKey string `json:"api_key"`
}

type ShadowConfig struct {
    Dir                        string `json:"dir"`                          // Absolute path to shadow root. Empty = disabled.
    InactivityWindowDays       int    `json:"inactivity_window_days"`       // default 30
    UploadActivityThresholdPct int    `json:"upload_activity_threshold_pct"` // default 20
    PauseTimeoutDays           int    `json:"pause_timeout_days"`           // default 30
    MaxLifetimeEnabled         bool   `json:"max_lifetime_enabled"`         // default false
    MaxLifetimeDays            int    `json:"max_lifetime_days"`            // default 90
    RemovalPolicy              string `json:"removal_policy"`               // "remove", "keep_seeding", "archive"
}
```

### Viper defaults (add to `InitConfig()`)

```go
// Download client defaults — add inside InitConfig()
viper.SetDefault("download_client.type", "")
viper.SetDefault("download_client.shadow.dir", "")
viper.SetDefault("download_client.shadow.inactivity_window_days", 30)
viper.SetDefault("download_client.shadow.upload_activity_threshold_pct", 20)
viper.SetDefault("download_client.shadow.pause_timeout_days", 30)
viper.SetDefault("download_client.shadow.max_lifetime_enabled", false)
viper.SetDefault("download_client.shadow.max_lifetime_days", 90)
viper.SetDefault("download_client.shadow.removal_policy", "keep_seeding")

// Then in the AppConfig assignment block:
// viper.UnmarshalKey("download_client", &AppConfig.DownloadClient)
```

### YAML configuration example

```yaml
# config.yaml

download_client:
  type: deluge                     # or "qbittorrent", "sabnzbd", or "" to disable

  deluge:
    host: "127.0.0.1"
    port: 58846
    username: "localoftp"
    password: "localoftp"          # In production, store via encrypted setting API

  qbittorrent:
    url: "http://localhost:8080"
    username: "admin"
    password: "adminadmin"

  sabnzbd:
    url: "http://localhost:8080"
    api_key: "abc123def456"

  shadow:
    dir: "/data/seeding"           # Must exist and be writable
    inactivity_window_days: 30
    upload_activity_threshold_pct: 20
    pause_timeout_days: 30
    max_lifetime_enabled: false
    max_lifetime_days: 90
    removal_policy: keep_seeding   # "remove" | "keep_seeding" | "archive"
```

### Credential security

Download client passwords and API keys are secrets. They should be stored via
`database.SetSetting(key, value, "string", true)` (the `isSecret=true` flag
triggers AES-256-GCM encryption as implemented in `internal/database/settings.go`).
The YAML values above are only used for initial setup; the server reads them
from the encrypted settings store at runtime via `database.GetDecryptedSetting`.

---

## Per-Torrent Override Storage

Each torrent can override any shadow cleanup threshold independently. The
override is a sparse struct — only fields that are explicitly set take effect;
nil fields fall through to the global default.

### PebbleDB key pattern

```
shadow:override:<torrent_id>   →  JSON-encoded TorrentOverride
```

This follows the project's key-prefix convention seen in `pebble_store.go`
(e.g. `book:path:<path>`, `operationlog:<op_id>:<timestamp>:<seq>`). The
`shadow:` prefix keeps all shadow-related keys in a contiguous keyspace,
making iteration and cleanup straightforward.

### Go struct

```go
// internal/download/shadow.go

// TorrentOverride holds per-torrent cleanup overrides.
// All fields are pointers; nil means "use the global/client default."
type TorrentOverride struct {
    InactivityWindowDays       *int `json:"inactivity_window_days,omitempty"`
    UploadActivityThresholdPct *int `json:"upload_activity_threshold_pct,omitempty"`
    PauseTimeoutDays           *int `json:"pause_timeout_days,omitempty"`
    MaxLifetimeEnabled         *bool `json:"max_lifetime_enabled,omitempty"`
    MaxLifetimeDays            *int  `json:"max_lifetime_days,omitempty"`
}

// SetTorrentOverride persists (or replaces) the override for a torrent.
func SetTorrentOverride(store database.Store, torrentID string, override *TorrentOverride) error {
    data, err := json.Marshal(override)
    if err != nil {
        return err
    }
    key := []byte(fmt.Sprintf("shadow:override:%s", torrentID))
    // Use the PebbleStore directly (same pattern as persistShadow above)
    if ps, ok := store.(*database.PebbleStore); ok {
        return ps.Set(key, data)
    }
    return fmt.Errorf("override storage requires PebbleDB")
}

// GetTorrentOverride loads the override. Returns nil if none is set.
func GetTorrentOverride(store database.Store, torrentID string) (*TorrentOverride, error) {
    key := []byte(fmt.Sprintf("shadow:override:%s", torrentID))
    if ps, ok := store.(*database.PebbleStore); ok {
        value, closer, err := ps.Get(key)
        if err != nil { // pebble.ErrNotFound → nil, nil
            return nil, nil
        }
        defer closer.Close()
        var override TorrentOverride
        if err := json.Unmarshal(value, &override); err != nil {
            return nil, err
        }
        return &override, nil
    }
    return nil, fmt.Errorf("override storage requires PebbleDB")
}
```

### API endpoint (future)

A `PUT /api/v1/torrents/:id/shadow-override` endpoint will be added to
`internal/server/server.go` following the same handler pattern used by the
existing book update endpoints. The handler validates the JSON body against
`TorrentOverride`, calls `SetTorrentOverride`, and returns 200 on success.

---

## Integration with Organize Flow

The shadow link creation hook lives inside `internal/organizer/organizer.go`,
in the `OrganizeBook` method, immediately after the file copy/link succeeds
and before the method returns the new path. This placement is deliberate:
the shadow must be created while the original downloaded file still exists at
its pre-organize location.

### Where in the code

In `OrganizeBook`, after the `switch strategy { ... }` block resolves
successfully (all branches that return `targetPath, nil`), add:

```go
// internal/organizer/organizer.go — inside OrganizeBook, after successful link/copy

func (o *Organizer) OrganizeBook(book *database.Book) (string, error) {
    // ... existing code up through the strategy switch ...

    // At this point targetPath is set and the file is in the library.
    // If download client integration is enabled, create the shadow link
    // BEFORE we return — the original file at book.FilePath still exists.
    if config.AppConfig.DownloadClient.Shadow.Dir != "" {
        torrentInfo := o.lookupTorrentForBook(book)
        if torrentInfo != nil {
            results := download.CreateShadowLinks(
                torrentInfo.DownloadDir,
                config.AppConfig.DownloadClient.Shadow.Dir,
                torrentInfo.Name,
                torrentInfo.Files,
            )
            // Log results; failures are non-fatal (the organize itself succeeded)
            for _, r := range results {
                if r.Err != nil {
                    log.Printf("[WARN] organizer: shadow link failed for %s: %v (method=%s)",
                        r.OriginalPath, r.Err, r.Method)
                } else {
                    log.Printf("[DEBUG] organizer: shadow link created for %s via %s",
                        r.ShadowPath, r.Method)
                }
            }

            // Persist the ShadowRecord so the cleanup job can track it
            record := &download.ShadowRecord{
                TorrentID:       torrentInfo.ID,
                TorrentName:     torrentInfo.Name,
                ShadowDir:       filepath.Join(config.AppConfig.DownloadClient.Shadow.Dir, torrentInfo.Name),
                CreatedAt:       time.Now(),
                LastActivityAt:  time.Now(),
                LastUploadBytes: torrentInfo.TotalUploaded,
                Status:          "active",
            }
            download.PersistShadowRecord(database.GlobalStore, record)

            // Tell the torrent client to point at the shadow dir
            if activeClient != nil {
                shadowTorrentDir := filepath.Join(
                    config.AppConfig.DownloadClient.Shadow.Dir, torrentInfo.Name)
                if err := activeClient.SetDownloadPath(context.Background(), torrentInfo.ID, shadowTorrentDir); err != nil {
                    log.Printf("[WARN] organizer: SetDownloadPath failed for %s: %v", torrentInfo.ID, err)
                }
            }
        }
    }

    return targetPath, nil
}

// lookupTorrentForBook finds which torrent (if any) produced this book.
// Strategy: match book.FilePath against the download directories of all
// completed torrents. This lookup is cached per organize batch.
func (o *Organizer) lookupTorrentForBook(book *database.Book) *download.TorrentInfo {
    // Query the active TorrentClient for completed torrents.
    // Check if book.FilePath starts with any torrent's DownloadDir + file path.
    // Return the matching TorrentInfo, or nil if no match.
    ...
}
```

### Sequence summary

```
1. Torrent completes download     →  /downloads/torrent-name/file.m4b
2. Auto-import scanner detects it →  enqueues organize operation
3. OrganizeBook runs              →  copies/links to /library/Author/Book/file.m4b
4. Shadow hook fires              →  creates /seeding/torrent-name/file.m4b (hardlink/reflink/copy)
5. SetDownloadPath called         →  torrent client now points at /seeding/torrent-name/
6. Metadata writer runs later     →  writes tags to library copy (temp+rename breaks hardlink)
7. Result                         →  library has enriched copy; shadow has pristine original
8. Cleanup job (periodic)         →  monitors upload activity; removes shadow when done seeding
```

---

## Dependencies

- Requires stable organize workflow (files must be reliably moved/linked)
- Shadow directory feature depends on safe file operations (already
  implemented in `internal/fileops/safe_operations.go`)
- Each client integration is independent and can be shipped separately
- Per-torrent overrides require PebbleDB (the default store)
- Credential encryption requires `database.InitEncryption` to have run
  (already called during server startup)

## References

- Safe file operations: `internal/fileops/safe_operations.go` (copy-first with SHA256 verification)
- File hashing: `internal/fileops/hash.go`
- Library organization: `internal/organizer/organizer.go` (reflink → hardlink → copy chain)
- Config structure: `internal/config/config.go`
- PebbleDB key patterns: `internal/database/pebble_store.go`
- Encrypted settings: `internal/database/settings.go`
- Library organization plan:
  [`library-organization-and-transcoding.md`](library-organization-and-transcoding.md)
