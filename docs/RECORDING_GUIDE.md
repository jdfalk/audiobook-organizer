<!-- file: docs/RECORDING_GUIDE.md -->
<!-- version: 1.0.0 -->
<!-- guid: e5f6a7b8-c9d0-e1f2-3a4b-5c6d7e8f9a0b -->
<!-- last-edited: 2026-02-05 -->

# Automated Demo Video Recording Guide

## Overview

This guide explains how to use the automated Playwright-based recording script to capture a complete end-to-end video of the audiobook organizer workflow.

The script:
- ✅ Automatically starts the API server
- ✅ Records the entire workflow as a video
- ✅ Tests all 5 workflow phases
- ✅ Captures screenshots at key moments
- ✅ Produces a WebM video file
- ✅ Displays a summary with timing and results

**Total recording time:** 2-3 minutes

---

## Quick Start (One Command)

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer

# Run the complete demo recording
bash scripts/run_demo_recording.sh
```

That's it! The script will:
1. Build the project
2. Start the API server
3. Record the entire workflow
4. Save the video to `demo_recordings/audiobook-demo.webm`
5. Display a summary

---

## What Gets Recorded

### Phase 1: Import Files (30 seconds)
- Creating an import path
- Importing an audiobook file
- Verification of import in database

### Phase 2: Fetch Metadata (45 seconds)
- Bulk metadata fetch from Open Library
- API populating title, author, description
- Display of enriched metadata

### Phase 3: Organize Files (30 seconds)
- File organization to disk
- Folder structure creation
- File renaming with metadata

### Phase 4: Edit Metadata (45 seconds)
- Manual metadata editing
- Updating narrator and publisher
- Saving changes

### Phase 5: Verification (30 seconds)
- Confirming all changes persisted
- Displaying final library view
- Summary of completed workflow

---

## Output Files

After running the script, you'll find:

```
demo_recordings/
├── audiobook-demo.webm          # Main demo video (WebM format)
└── screenshots/
    ├── 01-imported-book.png      # After import
    ├── 02-with-metadata.png      # After metadata fetch
    ├── 03-organized-files.png    # After file organization
    ├── 04-edited-metadata.png    # After manual edit
    └── 05-final-library-view.png # Final state
```

---

## Customization

### Change Output Directory

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
OUTPUT_DIR=/custom/path bash scripts/run_demo_recording.sh
```

### Use Different API Port

```bash
API_PORT=9000 bash scripts/run_demo_recording.sh
```

### Both Custom Port and Directory

```bash
API_PORT=9000 OUTPUT_DIR=/tmp/demo bash scripts/run_demo_recording.sh
```

---

## Video Format & Specifications

| Property | Value |
|----------|-------|
| Format | WebM (VP8 codec) |
| Duration | 2-3 minutes |
| Quality | 1280x720 (HD) |
| Frame Rate | 30 fps |
| Codec | VP8 |

---

## Post-Recording Tasks

### 1. Convert to MP4 (if needed)

```bash
# Install ffmpeg if not already installed
# macOS: brew install ffmpeg
# Linux: sudo apt-get install ffmpeg
# Windows: Download from ffmpeg.org

ffmpeg -i demo_recordings/audiobook-demo.webm \
       -c:v libx264 -crf 23 \
       -c:a aac -b:a 128k \
       demo_recordings/audiobook-demo.mp4
```

### 2. Add Voiceover

```bash
# Record voiceover using Audacity or similar tool
# Then use ffmpeg to mix audio:
ffmpeg -i demo_recordings/audiobook-demo.webm \
       -i voiceover.mp3 \
       -c:v copy \
       -c:a aac \
       -map 0:v:0 -map 1:a:0 \
       demo_recordings/audiobook-demo-with-voiceover.mp4
```

### 3. Add Captions/Subtitles

Use tools like:
- **Subtitle Edit** (Windows/Linux)
- **Aegisub** (Cross-platform)
- **YouTube Auto-captions** (After upload)

### 4. Edit Video

Use video editing software:
- **DaVinci Resolve** (Free, professional)
- **OBS Studio** (Free, streaming-focused)
- **Adobe Premiere Pro** (Professional)
- **Final Cut Pro** (Mac)

---

## Troubleshooting

### "API server did not start"

```bash
# Check if port 8080 is already in use
lsof -i :8080

# If port is in use, either:
# 1. Kill the process: kill -9 <PID>
# 2. Use a different port: API_PORT=9000 bash scripts/run_demo_recording.sh
```

### "Node.js is not installed"

```bash
# Install Node.js from nodejs.org or use package manager
# macOS: brew install node
# Linux: sudo apt-get install nodejs npm
# Windows: Download from nodejs.org
```

### "Playwright not found"

```bash
# Install Playwright dependency
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
npm install --save-dev playwright axios
```

### "Browser window won't open"

Ensure you're not running in headless mode:
- The script sets `headless: false`
- Make sure X11 (Linux) or display server is available
- On macOS/Windows, this should work automatically

### "Video file is empty or too small"

```bash
# Check if recording actually captured:
ls -lh demo_recordings/audiobook-demo.webm

# If file is small (<1MB), the recording may have failed
# Check the browser console for errors
# Increase timeouts in the script if needed
```

---

## Script Internals

The recording script (`scripts/record_demo.js`) performs these steps:

1. **Health Check**: Verifies API server is running
2. **Setup**: Creates import paths and test files
3. **Phase 1**: Tests import functionality
4. **Phase 2**: Tests metadata fetching
5. **Phase 3**: Tests file organization
6. **Phase 4**: Tests metadata editing
7. **Phase 5**: Verifies persistence
8. **Summary**: Displays results and saves video

Each phase includes:
- API calls to test the functionality
- Console output showing progress
- Screenshots at key points
- Automatic pauses to ensure visibility

---

## Video Formats & Sharing

### YouTube Upload

WebM format is supported by YouTube:
1. Go to YouTube Studio
2. Click "CREATE" → "Upload Video"
3. Select `demo_recordings/audiobook-demo.webm`
4. Add title, description, and thumbnails
5. Set to "Unlisted" or "Public"

### Embed in Documentation

```markdown
## Demo Video

[Watch the demo on YouTube](https://youtube.com/watch?v=YOUR_VIDEO_ID)

Or view the local recording:
- WebM: `demo_recordings/audiobook-demo.webm`
- MP4: `demo_recordings/audiobook-demo.mp4` (if converted)
```

### Social Media

Convert to MP4 and upload to:
- LinkedIn (mp4, up to 10GB)
- Twitter/X (mp4, up to 512MB)
- Facebook (mp4, up to 4GB)
- TikTok (mp4, up to 1GB)

---

## Advanced: Running Multiple Tests

To record multiple scenarios:

```bash
# Record scenario 1
OUTPUT_DIR=demo_recordings/scenario1 bash scripts/run_demo_recording.sh

# Record scenario 2 (different metadata)
OUTPUT_DIR=demo_recordings/scenario2 bash scripts/run_demo_recording.sh

# Record scenario 3 (error cases)
OUTPUT_DIR=demo_recordings/errors bash scripts/run_demo_recording.sh
```

Then combine videos using ffmpeg:

```bash
# Create a concat file
cat > concat.txt << EOF
file 'demo_recordings/scenario1/audiobook-demo.webm'
file 'demo_recordings/scenario2/audiobook-demo.webm'
file 'demo_recordings/scenario3/audiobook-demo.webm'
EOF

# Combine into single video
ffmpeg -f concat -safe 0 -i concat.txt -c copy demo_recordings/combined-demo.webm
```

---

## Performance Metrics

Recording performance varies by system:

| Phase | Duration | API Calls | Screenshots |
|-------|----------|-----------|-------------|
| Import Files | 30 sec | 3-4 | 1 |
| Fetch Metadata | 45 sec | 2-3 | 1 |
| Organize Files | 30 sec | 1-2 | 1 |
| Edit Metadata | 45 sec | 1-2 | 1 |
| Verify | 30 sec | 2 | 1 |
| **Total** | **3 min** | **10-13** | **5** |

---

## Example Complete Video Workflow

```bash
# 1. Generate recording
bash scripts/run_demo_recording.sh

# 2. Convert to MP4
ffmpeg -i demo_recordings/audiobook-demo.webm \
       -c:v libx264 -crf 23 \
       demo_recordings/audiobook-demo.mp4

# 3. Add watermark (optional)
ffmpeg -i demo_recordings/audiobook-demo.mp4 \
       -vf "drawtext=text='Audiobook Organizer Demo':x=10:y=10:fontsize=24:fontcolor=white" \
       demo_recordings/audiobook-demo-watermarked.mp4

# 4. Upload to YouTube
# Use YouTube Studio or youtube-upload CLI tool

# 5. Share the link
echo "Demo available at: https://youtube.com/watch?v=YOUR_ID"
```

---

## Tips for Professional Recording

1. **Clean System**: Close other applications to reduce distractions
2. **Stable Network**: Ensure good connectivity for API calls
3. **Full Screenshot**: Maximize browser window for better visibility
4. **Multiple Takes**: Record multiple times to get the best take
5. **Consistent Speed**: Maintain steady pace throughout recording
6. **Test Beforehand**: Run once without recording to verify workflow

---

## Support

If you encounter issues:

1. Check `/tmp/api_server.log` for server errors
2. Review the generated screenshots in `demo_recordings/screenshots/`
3. Check browser console in the browser window that opens
4. Run individual API calls manually using `scripts/api_examples.sh`
5. Review the full demo guide: `docs/END_TO_END_DEMO.md`

---

## Summary

The automated recording script provides:
- ✅ Full end-to-end workflow testing
- ✅ Professional video output
- ✅ Automatic screenshot capture
- ✅ API call verification
- ✅ Complete demo in one command

**Ready to record? Run:**

```bash
bash scripts/run_demo_recording.sh
```

Your demo video will be saved to `demo_recordings/audiobook-demo.webm` 🎬
