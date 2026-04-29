from pathlib import Path
import re

needle = "discovery"
for path in Path(".\").rglob("*.ts*"):
    try:
        text = path.read_text(errors="ignore")
    except Exception:
        continue
    if needle in text.lower():
        index = text.lower().index(needle)
        print(f"--- {path} ---")
        start = max(0, index - 200)
        end = min(len(text), index + 200)
        snippet = text[start:end]
        print(snippet)
        print()
