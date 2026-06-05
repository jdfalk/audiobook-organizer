import os
import pathlib
import sys

target = "stripChapterPrefix"
root = pathlib.Path(__file__).parent
found = []
for path in root.rglob("*.go"):
    try:
        text = path.read_text()
    except Exception:
        continue
    if target in text:
        found.append(str(path.relative_to(root)))
print("found:")
for entry in found:
    print(entry)
if not found:
    raise SystemExit(1)
