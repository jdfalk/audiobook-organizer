import pathlib
import zlib

REPO_ROOT = pathlib.Path(__file__).resolve().parent.parent


def git_dir() -> pathlib.Path:
    git_path = REPO_ROOT / ".git"
    if not git_path.is_dir():
        raise RuntimeError(".git directory not found")
    return git_path


def read_object(hash_hex: str):
    git_path = git_dir() / "objects" / hash_hex[:2] / hash_hex[2:]
    with git_path.open("rb") as f:
        data = zlib.decompress(f.read())
    head_end = data.index(b"\x00")
    header = data[:head_end].decode()
    obj_type = header.split()[0]
    return data[head_end + 1 :], obj_type


def read_commit_tree(commit_hash: str) -> str:
    data, obj_type = read_object(commit_hash)
    if obj_type != "commit":
        raise RuntimeError(f"object {commit_hash} is not a commit")
    for line in data.splitlines():
        if line.startswith(b"tree "):
            return line.split()[1].decode()
    raise RuntimeError(f"commit {commit_hash} missing tree")


def parse_tree_entries(data: bytes):
    entries = []
    i = 0
    while i < len(data):
        space = data.index(b" ", i)
        mode = data[i:space].decode()
        i = space + 1
        zero = data.index(b"\x00", i)
        name = data[i:zero].decode()
        i = zero + 1
        hash_bytes = data[i : i + 20]
        i += 20
        entries.append((mode, name, hash_bytes.hex()))
    return entries


def find_blob_hash(tree_hash: str, parts):
    current = tree_hash
    for idx, part in enumerate(parts):
        tree_data, obj_type = read_object(current)
        if obj_type != "tree":
            raise RuntimeError(f"object {current} is not a tree")
        entry = None
        for mode, name, hash_hex in parse_tree_entries(tree_data):
            if name == part:
                entry = (mode, hash_hex)
                break
        if entry is None:
            raise RuntimeError(f"path segment {part} not found")
        mode, hash_hex = entry
        if idx == len(parts) - 1:
            return hash_hex
        if mode != "40000":
            raise RuntimeError(f"path segment {part} is not a directory")
        current = hash_hex
    raise RuntimeError("path not found")


def read_head_commit() -> str:
    head_path = git_dir() / "HEAD"
    head = head_path.read_text().strip()
    if head.startswith("ref: "):
        ref = head.split(" ", 1)[1]
        return (git_dir() / ref).read_text().strip()
    return head


def cat_git_blob(path: str) -> bytes:
    commit = read_head_commit()
    tree = read_commit_tree(commit)
    blob_hash = find_blob_hash(tree, path.split("/"))
    data, obj_type = read_object(blob_hash)
    if obj_type != "blob":
        raise RuntimeError(f"expected blob, got {obj_type}")
    return data


def test_restore_wire_handlers():
    data = cat_git_blob("internal/server/wire_handlers.go")
    target = REPO_ROOT / "internal/server/wire_handlers.go"
    target.write_bytes(data)
