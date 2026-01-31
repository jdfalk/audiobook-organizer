#!/bin/bash
# file: scripts/create-test-audiobooks.sh
# version: 1.0.0
# guid: f1e2d3c4-b5a6-7980-1234-567890abcdef
# description: Create M4B and M4A test audiobook files from existing MP3 files

set -e

echo "🎧 Creating M4B/M4A Test Audiobooks"
echo "===================================="
echo ""

# Base directory for test audio files
TESTDATA_DIR="testdata/audio/librivox"

# Function to create M4B/M4A from MP3 files in a directory
create_audiobook() {
    local input_dir="$1"
    local output_name="$2"
    local title="$3"
    local author="$4"
    local album="$5"
    local year="$6"

    echo "📚 Processing: $title by $author"
    echo "   Input: $input_dir"

    # Find all MP3 files and sort them
    local mp3_files=()
    while IFS= read -r -d '' file; do
        mp3_files+=("$file")
    done < <(find "$input_dir" -name "*.mp3" -print0 | sort -z)

    if [ ${#mp3_files[@]} -eq 0 ]; then
        echo "   ⚠️  No MP3 files found, skipping"
        return
    fi

    echo "   Found ${#mp3_files[@]} MP3 files"

    # Create concat demuxer file
    local concat_file=$(mktemp)
    local chapter_file=$(mktemp)

    # Build concat list and chapter metadata
    local start_time=0
    local chapter_num=1

    echo ";FFMETADATA1" > "$chapter_file"
    echo "title=$title" >> "$chapter_file"
    echo "artist=$author" >> "$chapter_file"
    echo "album=$album" >> "$chapter_file"
    echo "date=$year" >> "$chapter_file"
    echo "genre=Audiobook" >> "$chapter_file"
    echo "" >> "$chapter_file"

    for mp3_file in "${mp3_files[@]}"; do
        # Use absolute path for concat demuxer
        local abs_path=$(cd "$(dirname "$mp3_file")" && pwd)/$(basename "$mp3_file")
        echo "file '$abs_path'" >> "$concat_file"

        # Get duration of this MP3 file
        local duration=$(ffprobe -v error -show_entries format=duration -of default=noprint_wrappers=1:nokey=1 "$mp3_file")
        local duration_ms=$(echo "$duration * 1000" | bc | cut -d. -f1)

        # Add chapter marker
        local chapter_title=$(basename "$mp3_file" .mp3)
        echo "[CHAPTER]" >> "$chapter_file"
        echo "TIMEBASE=1/1000" >> "$chapter_file"
        echo "START=$start_time" >> "$chapter_file"
        local end_time=$((start_time + duration_ms))
        echo "END=$end_time" >> "$chapter_file"
        echo "title=Chapter $chapter_num: $chapter_title" >> "$chapter_file"
        echo "" >> "$chapter_file"

        start_time=$end_time
        ((chapter_num++))
    done

    # Create M4B with chapters
    local m4b_output="$input_dir/${output_name}.m4b"
    echo "   Creating M4B: $m4b_output"

    ffmpeg -f concat -safe 0 -i "$concat_file" \
           -i "$chapter_file" \
           -map 0:a \
           -map_metadata 1 \
           -c:a aac \
           -b:a 128k \
           -f ipod \
           -movflags +faststart \
           -y \
           "$m4b_output" \
           2>&1 | grep -E "(Input|Output|Duration|time=)" || true

    echo "   ✅ Created M4B: $(basename "$m4b_output")"

    # Create M4A variant (without chapter metadata, simpler)
    local m4a_output="$input_dir/${output_name}.m4a"
    echo "   Creating M4A: $m4a_output"

    ffmpeg -f concat -safe 0 -i "$concat_file" \
           -map 0:a \
           -metadata title="$title" \
           -metadata artist="$author" \
           -metadata album="$album" \
           -metadata date="$year" \
           -metadata genre="Audiobook" \
           -c:a aac \
           -b:a 128k \
           -movflags +faststart \
           -y \
           "$m4a_output" \
           2>&1 | grep -E "(Input|Output|Duration|time=)" || true

    echo "   ✅ Created M4A: $(basename "$m4a_output")"

    # Cleanup temp files
    rm -f "$concat_file" "$chapter_file"

    echo ""
}

# Check if testdata directory exists
if [ ! -d "$TESTDATA_DIR" ]; then
    echo "❌ Error: $TESTDATA_DIR not found"
    exit 1
fi

# Create M4B/M4A files for each test book
echo "Processing test audiobooks..."
echo ""

# Book 1: The Odyssey
if [ -d "$TESTDATA_DIR/odyssey_butler_librivox" ]; then
    create_audiobook \
        "$TESTDATA_DIR/odyssey_butler_librivox" \
        "odyssey_complete" \
        "The Odyssey" \
        "Homer" \
        "The Odyssey" \
        "800BC"
fi

# Book 2: Moby Dick
if [ -d "$TESTDATA_DIR/moby_dick_librivox" ]; then
    create_audiobook \
        "$TESTDATA_DIR/moby_dick_librivox" \
        "moby_dick_complete" \
        "Moby-Dick" \
        "Herman Melville" \
        "Moby-Dick; or, The Whale" \
        "1851"
fi

# Book 3: The Iliad (version 1)
if [ -d "$TESTDATA_DIR/illiad_0801_librivox3" ]; then
    create_audiobook \
        "$TESTDATA_DIR/illiad_0801_librivox3" \
        "iliad_complete" \
        "The Iliad" \
        "Homer" \
        "The Iliad" \
        "762BC"
fi

# Book 4: The Iliad (version 2 - different recording)
if [ -d "$TESTDATA_DIR/iliadv2_2407_librivox" ]; then
    create_audiobook \
        "$TESTDATA_DIR/iliadv2_2407_librivox" \
        "iliad_v2_complete" \
        "The Iliad (Version 2)" \
        "Homer" \
        "The Iliad" \
        "762BC"
fi

echo "=================================="
echo "✨ M4B/M4A Test Files Created!"
echo ""
echo "Created files:"
find "$TESTDATA_DIR" -name "*.m4b" -o -name "*.m4a" | while read file; do
    echo "  - $(basename "$file") ($(du -h "$file" | cut -f1))"
done
echo ""
echo "These files include:"
echo "  ✓ Chapter markers"
echo "  ✓ Embedded metadata (title, author, album, year)"
echo "  ✓ AAC audio codec @ 128kbps"
echo "  ✓ Ready for testing import and playback"
echo ""
echo "Next: Add cover art using download-test-covers.sh"
