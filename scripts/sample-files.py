#!/usr/bin/env python3
# file: scripts/sample-files.py
# version: 1.0.0
# guid: 1a2b3c4d-5e6f-7890-abcd-ef1234567890

"""
Sample random files from the full file list for testing.
Creates a representative sample of 1000 files.
"""

import os
import random
import sys
from pathlib import Path


def main():
    # Use environment variable or default path
    scratch_dir = Path(os.getenv("SCRATCH_DIR", str(Path.home() / "scratch")))
    input_file = scratch_dir / "file-list-books"
    script_dir = Path(__file__).parent.resolve()
    output_file = script_dir.parent / "testdata" / "sample-1000-files.txt"

    # Ensure testdata directory exists
    output_file.parent.mkdir(parents=True, exist_ok=True)

    # Read all lines
    print(f"Reading file list from: {input_file}")
    with open(input_file, "r", encoding="utf-8", errors="ignore") as f:
        all_files = [line.strip() for line in f if line.strip()]

    print(f"Total files: {len(all_files)}")

    # Sample 1000 random files
    sample_size = min(1000, len(all_files))
    sampled_files = random.sample(all_files, sample_size)

    # Sort for consistent output
    sampled_files.sort()

    # Write to output file
    print(f"Writing {len(sampled_files)} sampled files to: {output_file}")
    with open(output_file, "w", encoding="utf-8") as f:
        for file_path in sampled_files:
            f.write(f"{file_path}\n")

    print(f"Done! Sampled {len(sampled_files)} files")

    # Print some statistics
    extensions = {}
    for file_path in sampled_files:
        ext = Path(file_path).suffix.lower()
        extensions[ext] = extensions.get(ext, 0) + 1

    print("\nFile format distribution:")
    for ext, count in sorted(extensions.items(), key=lambda x: x[1], reverse=True)[:10]:
        print(f"  {ext:10s}: {count:5d}")


if __name__ == "__main__":
    main()
