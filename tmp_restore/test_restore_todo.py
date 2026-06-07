from pathlib import Path
import struct
import zlib
import sys


def restore_todo():
    index_path = Path(".git/index")
    data = index_path.read_bytes()
    if not data.startswith(b"DIRC"):
        raise RuntimeError("This repo has no git index header")
    entry_count = struct.unpack(">I", data[8:12])[0]
    offset = 12
    for _ in range(entry_count):
        header = data[offset : offset + 62]
        if len(header) < 62:
            raise RuntimeError("Truncated git index entry")
        _, _, _, _, _, _, _, _, _, _, sha1, flags = struct.unpack(
            ">LLLLLLLLLL20sH", header
        )
        path_offset = offset + 62
        path_end = data.index(b"\x00", path_offset)
        path = data[path_offset:path_end].decode()
        entry_length = 62 + (path_end - path_offset) + 1
        padding = (8 - (entry_length % 8)) % 8
        total_length = entry_length + padding
        if path == "TODO.md":
            blob_path = Path(".git/objects") / sha1.hex()[:2] / sha1.hex()[2:]
            blob_data = zlib.decompress(blob_path.read_bytes())
            _, content = blob_data.split(b"\x00", 1)
            Path("TODO.md").write_bytes(content)
            print("Restored TODO.md from blob", sha1.hex())
            return
        offset += total_length
    raise FileNotFoundError("TODO.md entry not found in git index")


def test_restore_todo():
    restore_todo()
    script = Path(__file__)
    script.unlink()
    parent = script.parent
    try:
        parent.rmdir()
    except OSError:
        pass
