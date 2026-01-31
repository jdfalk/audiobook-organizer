#!/bin/bash
# file: testdata/scripts/create_test_audiobooks.sh
# version: 1.0.0
# guid: f7e6d5c4-b3a2-1098-7654-3210fedcba98
# description: Create M4B and M4A test audiobook files from MP3s with chapters and metadata

set -e

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}Creating Test Audiobook Files${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

# Check if ffmpeg is installed
if ! command -v ffmpeg &> /dev/null; then
    echo -e "${YELLOW}Error: ffmpeg is not installed${NC}"
    echo "Install with: brew install ffmpeg"
    exit 1
fi

# Base directory
TESTDATA_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
AUDIO_DIR="$TESTDATA_DIR/audio/librivox"

# Function to create M4B/M4A from MP3s
create_audiobook() {
    local book_dir="$1"
    local title="$2"
    local author="$3"
    local series="$4"
    local narrator="$5"
    local year="$6"

    echo -e "${GREEN}Processing: $title${NC}"

    # Get sorted list of MP3 files
    mp3_files=$(find "$book_dir" -type f -name "*.mp3" | sort)

    if [ -z "$mp3_files" ]; then
        echo -e "${YELLOW}  No MP3 files found in $book_dir${NC}"
        return
    fi

    # Count files
    file_count=$(echo "$mp3_files" | wc -l | tr -d ' ')
    echo "  Found $file_count MP3 files"

    # Create chapters file for ffmpeg
    chapter_file=$(mktemp)
    echo ";FFMETADATA1" > "$chapter_file"

    # Calculate chapter timing
    current_time=0
    chapter_num=1

    while IFS= read -r mp3_file; do
        # Get duration of this MP3
        duration=$(ffprobe -v error -show_entries format=duration -of default=noprint_wrappers=1:nokey=1 "$mp3_file" 2>/dev/null)
        duration_ms=$(echo "$duration * 1000 / 1" | bc)

        # Add chapter marker
        echo "" >> "$chapter_file"
        echo "[CHAPTER]" >> "$chapter_file"
        echo "TIMEBASE=1/1000" >> "$chapter_file"
        echo "START=$current_time" >> "$chapter_file"

        # Calculate end time
        end_time=$((current_time + duration_ms))
        echo "END=$end_time" >> "$chapter_file"

        # Chapter title from filename
        chapter_title=$(basename "$mp3_file" .mp3)
        echo "title=Chapter $chapter_num: $chapter_title" >> "$chapter_file"

        current_time=$end_time
        ((chapter_num++))
    done <<< "$mp3_files"

    # Create concat file for ffmpeg
    concat_file=$(mktemp)
    echo "$mp3_files" | while read -r mp3_file; do
        echo "file '$mp3_file'" >> "$concat_file"
    done

    # Output filenames
    output_base="$book_dir/$(echo "$title" | tr ' ' '_' | tr '[:upper:]' '[:lower:]')"
    m4b_file="${output_base}.m4b"
    m4a_file="${output_base}.m4a"

    # Create M4B with chapters and metadata
    echo "  Creating M4B file..."
    ffmpeg -f concat -safe 0 -i "$concat_file" \
        -i "$chapter_file" \
        -map_metadata 1 \
        -c:a aac -b:a 64k \
        -metadata title="$title" \
        -metadata artist="$author" \
        -metadata album_artist="$author" \
        -metadata album="$series" \
        -metadata composer="$narrator" \
        -metadata date="$year" \
        -metadata genre="Audiobook" \
        -metadata copyright="Public Domain" \
        -f ipod \
        "$m4b_file" \
        -y -v error -stats

    # Create M4A variant (same content, different extension)
    echo "  Creating M4A file..."
    ffmpeg -f concat -safe 0 -i "$concat_file" \
        -i "$chapter_file" \
        -map_metadata 1 \
        -c:a aac -b:a 64k \
        -metadata title="$title" \
        -metadata artist="$author" \
        -metadata album_artist="$author" \
        -metadata album="$series" \
        -metadata composer="$narrator" \
        -metadata date="$year" \
        -metadata genre="Audiobook" \
        -metadata copyright="Public Domain" \
        "$m4a_file" \
        -y -v error -stats

    # Clean up temp files
    rm "$chapter_file" "$concat_file"

    echo -e "${GREEN}  ✓ Created: $(basename "$m4b_file")${NC}"
    echo -e "${GREEN}  ✓ Created: $(basename "$m4a_file")${NC}"
    echo ""
}

# Process each test book
create_audiobook \
    "$AUDIO_DIR/odyssey_butler_librivox" \
    "The Odyssey" \
    "Homer" \
    "Homer's Epics" \
    "Samuel Butler" \
    "800BC"

create_audiobook \
    "$AUDIO_DIR/moby_dick_librivox" \
    "Moby Dick" \
    "Herman Melville" \
    "" \
    "LibriVox Community" \
    "1851"

create_audiobook \
    "$AUDIO_DIR/illiad_0801_librivox3" \
    "The Iliad" \
    "Homer" \
    "Homer's Epics" \
    "LibriVox Volunteers" \
    "800BC"

create_audiobook \
    "$AUDIO_DIR/iliadv2_2407_librivox" \
    "The Iliad (Version 2)" \
    "Homer" \
    "Homer's Epics" \
    "LibriVox Community" \
    "800BC"

echo -e "${BLUE}========================================${NC}"
echo -e "${GREEN}✓ Test audiobook files created successfully!${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""
echo "M4B and M4A files have been created in each book directory:"
echo "  - testdata/audio/librivox/odyssey_butler_librivox/"
echo "  - testdata/audio/librivox/moby_dick_librivox/"
echo "  - testdata/audio/librivox/illiad_0801_librivox3/"
echo "  - testdata/audio/librivox/iliadv2_2407_librivox/"
echo ""
echo "Each M4B/M4A file includes:"
echo "  ✓ All MP3s as chapters with chapter markers"
echo "  ✓ Full metadata (title, author, series, narrator, year)"
echo "  ✓ Proper audiobook format"
echo ""
