<!-- file: docs/QA_CHECKLIST.md -->
<!-- version: 1.0.0 -->
<!-- guid: a1b2c3d4-e5f6-7890-abcd-ef0123456789 -->

# Manual QA Verification Checklist

Run through before any release. Check each box when verified.

## Startup

- [ ] `make build` completes without errors
- [ ] `./audiobook-organizer serve` starts and serves UI at configured port
- [ ] Welcome wizard appears on first run (no config/DB)
- [ ] Dashboard loads with system status

## Scanning & Import

- [ ] Add import path via Settings
- [ ] Trigger scan — books appear in Library
- [ ] Multi-file audiobook detected as single book with segments
- [ ] M4B, MP3, FLAC formats all recognized
- [ ] Duplicate files (same hash) deduplicated
- [ ] iTunes XML import via wizard works

## Library & Book Detail

- [ ] Library page lists all books with pagination
- [ ] Search filters books by title/author
- [ ] Book detail page shows metadata, segments, cover art
- [ ] Files tab shows individual segments with durations
- [ ] Auto-fill Track Numbers works on Files tab
- [ ] Tags tab shows file tags vs book metadata comparison
- [ ] Inline tag editing works (edit → save)

## Metadata

- [ ] Fetch Metadata pulls from OpenLibrary
- [ ] Parse with AI extracts metadata from filenames
- [ ] History tab shows all metadata changes
- [ ] Undo reverts a metadata change

## Versions (CoW)

- [ ] Versions tab shows linked versions
- [ ] Link Another Version — search and link works
- [ ] Star icon sets primary version
- [ ] Unlink removes version from group
- [ ] Creating a snapshot preserves previous state

## Transcode

- [ ] Transcode to M4B starts and shows in Active Operations
- [ ] Progress updates in real-time (percentage increments)
- [ ] Chapter markers added for multi-file books
- [ ] Output file created at correct path
- [ ] Temp files cleaned up on success
- [ ] Canceling a transcode cleans up temp files

## Operations & Background Jobs

- [ ] Active Operations panel shows running jobs
- [ ] SSE events update progress in real-time
- [ ] Completed operations show in history
- [ ] Cancel button stops a running operation

## Settings

- [ ] All settings categories load
- [ ] Changes persist after save and restart
- [ ] Import paths can be added/removed
- [ ] Blocklist management works

## Backup & Restore

- [ ] Create backup produces .tar.gz file
- [ ] Restore from backup restores database state

## Error Handling

- [ ] Missing file paths show clear error
- [ ] Network errors show toast notifications
- [ ] Invalid operations return appropriate error messages

## Deployment

- [ ] `docker build .` succeeds
- [ ] Container starts and serves UI
- [ ] Data persists across container restarts (volume mount)
