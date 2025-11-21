# Test Audio Fixtures

Tiny audio files for integration testing, managed via Git LFS.

## Files

- `test_sample.mp3` - 0.1s MP3 @ 32kbps (~748 bytes)
- `test_sample.m4b` - 0.1s M4B/AAC @ 32kbps (~1.4KB)
- `test_sample.flac` - 0.1s FLAC (~9.3KB)

## Purpose

These minimal audio files enable:

- Metadata read/write integration tests
- Scanner functionality tests
- Tag library validation
- Format detection tests

All files contain a 440Hz sine wave tone for 0.1 seconds.

## Git LFS

Audio files are tracked by Git LFS. To clone with samples:

```bash
git lfs pull
```
