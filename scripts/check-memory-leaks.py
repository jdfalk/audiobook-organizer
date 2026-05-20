#!/usr/bin/env python3
"""
Detects common memory leak patterns in React/TypeScript code.
Scans for:
- Untracked setTimeout/setInterval
- Untracked addEventListener/removeEventListener pairs
- Untracked fetch/subscriptions in handlers
"""

import re
import sys
from pathlib import Path
from typing import List, Tuple, Optional

class MemoryLeakDetector:
    def __init__(self, web_src_dir: Optional[str] = None):
        if web_src_dir is None:
            # Find web/src by looking up from current directory
            current = Path.cwd()
            for parent in [current] + list(current.parents):
                candidate = parent / "web" / "src"
                if candidate.exists():
                    web_src_dir = str(candidate)
                    break
            if web_src_dir is None:
                web_src_dir = "web/src"

        self.web_src = Path(web_src_dir)
        self.issues: List[Tuple[str, int, str]] = []
        # Lines that indicate proper cleanup
        self.ignore_patterns = [
            r"clearTimeout|clearInterval",
            r"removeEventListener",
            r"useRef.*current",
            r"return\s*\(\)",
            r"cancelled\s*=\s*true",
            r"AbortController",
        ]

    def should_ignore(self, line: str) -> bool:
        """Check if line should be ignored"""
        # Ignore empty lines and comments
        stripped = line.strip()
        if not stripped or stripped.startswith('//'):
            return True

        for pattern in self.ignore_patterns:
            if re.search(pattern, line):
                return True
        return False

    def check_untracked_timers(self, filepath: str, content: str) -> None:
        """Find setTimeout/setInterval without proper tracking"""
        lines = content.split('\n')

        for i, line in enumerate(lines, 1):
            if self.should_ignore(line):
                continue

            # Look for setTimeout/setInterval
            match = re.search(r'(setTimeout|setInterval)\s*\(', line)
            if not match:
                continue

            timer_type = match.group(1)

            # Check if it's being assigned to a ref or variable
            if re.search(r'(Ref\.current\s*=|const\s+\w*[Rr]ef\s*=|let\s+\w*[Ii]d\s*=)', line):
                continue  # Already tracked

            # Check if we're in a useEffect with cleanup
            in_use_effect = False
            has_cleanup = False

            # Look backwards for useEffect
            for j in range(i-1, max(0, i-30), -1):
                if 'useEffect' in lines[j]:
                    in_use_effect = True
                    # Check if there's a return statement (cleanup) before the setTimeout
                    for k in range(j, i):
                        if re.search(r'return\s*\(\s*\)\s*=>|return\s*function', lines[k]):
                            has_cleanup = True
                            break
                    break

            if in_use_effect and has_cleanup:
                continue  # Cleanup exists

            # This is an untracked timer
            self.issues.append((filepath, i, f"Untracked {timer_type} (may fire after unmount)"))

    def check_untracked_listeners(self, filepath: str, content: str) -> None:
        """Find addEventListener without removeEventListener"""
        lines = content.split('\n')

        for i, line in enumerate(lines, 1):
            if 'addEventListener' not in line or self.should_ignore(line):
                continue

            # Get the listener name for matching
            listener_match = re.search(r"addEventListener\s*\(\s*['\"](\w+)['\"]", line)
            if not listener_match:
                continue

            event_name = listener_match.group(1)

            # Look ahead for removeEventListener with same event
            found_remove = False
            scope_depth = 0

            for j in range(i, min(i + 50, len(lines))):
                line_j = lines[j]

                # Track scope to avoid false positives
                scope_depth += line_j.count('{') - line_j.count('}')

                if f"removeEventListener('{event_name}'" in line_j or f'removeEventListener("{event_name}"' in line_j:
                    found_remove = True
                    break

                # If we hit end of function/component, stop looking
                if scope_depth < 0:
                    break

            if not found_remove:
                self.issues.append((filepath, i, f"addEventListener('{event_name}') without removeEventListener"))

    def check_poll_without_cleanup(self, filepath: str, content: str) -> None:
        """Find polling functions created in handlers without cleanup tracking"""
        lines = content.split('\n')

        for i, line in enumerate(lines, 1):
            if self.should_ignore(line):
                continue

            # Look for patterns like: const poll = async () => { ... while true or setTimeout(poll, ...)
            if re.search(r'const\s+\w*[Pp]oll\s*=\s*async', line):
                # Check if it has proper cleanup pattern
                for j in range(i, min(i + 50, len(lines))):
                    if 'cancelled' in lines[j] or 'AbortController' in lines[j]:
                        break
                    if re.search(r'setTimeout\(\w*[Pp]oll', lines[j]):
                        # Found recursive setTimeout without cancellation
                        if 'cancelled' not in ''.join(lines[max(0, j-20):j+5]):
                            self.issues.append((filepath, j+1, "Recursive polling without cancellation flag"))
                        break

    def scan(self) -> bool:
        """Scan all TSX/TS files"""
        if not self.web_src.exists():
            print(f"Warning: {self.web_src} not found, skipping memory leak scan")
            return True

        tsx_files = sorted(list(self.web_src.rglob("*.tsx")) + list(self.web_src.rglob("*.ts")))
        # Exclude test files
        tsx_files = [f for f in tsx_files if '.test.' not in f.name and 'setup.ts' not in f.name]

        print(f"🔍 Scanning {len(tsx_files)} files for memory leaks...")

        for filepath in tsx_files:
            try:
                content = filepath.read_text(encoding='utf-8')
                rel_path = filepath.relative_to(self.web_src.parent)

                self.check_untracked_timers(str(rel_path), content)
                self.check_untracked_listeners(str(rel_path), content)
                self.check_poll_without_cleanup(str(rel_path), content)
            except Exception as e:
                print(f"Error scanning {filepath}: {e}", file=sys.stderr)

        return len(self.issues) == 0

    def report(self) -> None:
        """Print findings"""
        if not self.issues:
            print("✅ No memory leak patterns detected\n")
            return

        print(f"\n⚠️  Found {len(self.issues)} potential memory leaks:\n")

        by_file = {}
        for filepath, line_no, issue in self.issues:
            if filepath not in by_file:
                by_file[filepath] = []
            by_file[filepath].append((line_no, issue))

        for filepath in sorted(by_file.keys()):
            print(f"  {filepath}")
            for line_no, issue in sorted(by_file[filepath]):
                print(f"    Line {line_no}: {issue}")

        print()


def main():
    detector = MemoryLeakDetector()
    success = detector.scan()
    detector.report()

    return 0 if success else 1


if __name__ == "__main__":
    sys.exit(main())
