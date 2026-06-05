from pathlib import Path

def test_patch_todo():
    path = Path("TODO.md")
    text = path.read_text()
    old = "- [ ] **ACOUSTID-STATS-2** `GET /maintenance/acoustid-stats` handler + route."
    new = "- [x] **ACOUSTID-STATS-2** `GET /maintenance/acoustid-stats` handler + route."
    if old not in text:
        raise RuntimeError("pattern not found")
    path.write_text(text.replace(old, new, 1))
    Path(__file__).unlink()
