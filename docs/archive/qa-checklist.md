<!-- file: docs/qa-checklist.md -->
<!-- version: 1.0.0 -->
<!-- guid: 9c8b7a6d-5e4f-4321-a0b9-c8d7e6f5a4b3 -->

# QA Checklist

## First Run
- [ ] Fresh binary starts without errors
- [ ] Welcome wizard appears on first visit
- [ ] Can set library path in wizard
- [ ] Can skip optional steps (AI key, iTunes)
- [ ] Dashboard loads after wizard completion

## Authentication
- [ ] First run allows creating admin account
- [ ] Login succeeds with valid credentials
- [ ] Login fails with invalid credentials
- [ ] Logout redirects to login
- [ ] Unauthenticated requests to protected API return 401

## Import & Scan
- [ ] Can scan a directory with M4B files
- [ ] Can scan a directory with MP3 files
- [ ] Can scan mixed format directories
- [ ] Progress shows in UI via SSE
- [ ] Duplicate files detected and skipped
- [ ] Special characters in filenames handled

## iTunes Import
- [ ] Can browse for Library.xml
- [ ] Validation shows track count
- [ ] Import creates book records
- [ ] Metadata fetch runs after import
- [ ] Files organized into author/title structure

## Library View
- [ ] Books display in grid
- [ ] Books display in list view
- [ ] Pagination works (next/prev)
- [ ] Search filters results
- [ ] Sort by title/author/date works
- [ ] Cover art displays when available

## Book Detail
- [ ] All metadata fields display
- [ ] Can edit metadata fields
- [ ] Multi-author display works
- [ ] Narrator display works
- [ ] File list shows all audio files

## Organize
- [ ] Copy strategy works
- [ ] Hardlink strategy works
- [ ] Naming pattern applied correctly
- [ ] Multi-author books filed under primary author

## Settings
- [ ] Can change library path
- [ ] Can change naming pattern
- [ ] Can toggle AI features
- [ ] Can configure download clients
- [ ] Settings persist across restarts

## Auto-scan (fsnotify)
- [ ] Dropping file in import dir triggers scan within debounce window
- [ ] SSE notification fires
- [ ] New book appears in library

## Backup/Restore
- [ ] Can create backup
- [ ] Can restore from backup
- [ ] Auto-cleanup of old backups works

## Download Clients
- [ ] Deluge connection works (if available)
- [ ] qBittorrent connection works (if available)
- [ ] SABnzbd connection works (if available)

## Edge Cases
- [ ] 10,000+ book library performs acceptably
- [ ] Very long filenames handled
- [ ] Unicode filenames handled
- [ ] Network disconnection handled gracefully
- [ ] Disk full handled gracefully
- [ ] Concurrent operations do not conflict
