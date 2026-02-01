<!-- file: .github/instructions/python.instructions.md -->
<!-- version: 2.0.0 -->
<!-- guid: 2a5b7c8d-9e1f-4a2b-8c3d-6e9f1a5b7c8d -->
<!-- last-edited: 2026-01-31 -->

<!-- prettier-ignore-start -->
<!-- markdownlint-disable -->
---
applyTo: "**/*.py"
description: |
  Python coding rules. Extends general-coding.instructions.md. Full reference: Google Python Style Guide.
---
<!-- markdownlint-enable -->
<!-- prettier-ignore-end -->

# Python Coding Instructions

- Follow [general coding instructions](general-coding.instructions.md).
- Full reference: [Google Python Style Guide](https://google.github.io/styleguide/pyguide.html).
- All Python files must begin with the required file header (after shebang if present).

## Version

- **Python 3.13+** required. Specify `requires-python = ">=3.13"` in `pyproject.toml`.

## Naming

- **Modules/packages:** lowercase, no underscores (`mymodule`)
- **Functions/variables:** snake_case
- **Classes:** CapWords (PascalCase)
- **Constants:** UPPER_CASE
- **Private:** leading underscore (`_helper`). Double underscore for name mangling only.

## Imports

- Standard library → third-party → local. Blank line between groups.
- One import per line (not `import os, sys`).
- Use `from x import y` for frequently used names.

## Functions

- Type-hint all parameters and return types.
- Docstrings on all public functions/classes using Google style:

```python
def fetch_metadata(url: str, timeout: int = 30) -> dict:
    """Fetches audiobook metadata from the given URL.

    Args:
        url: The metadata endpoint URL.
        timeout: Request timeout in seconds.

    Returns:
        Parsed metadata as a dictionary.

    Raises:
        RequestError: If the request fails or times out.
    """
```

## Error Handling

- Use specific exception types. Never bare `except:`.
- Catch narrow exceptions. Re-raise or wrap with context.
- Use `finally` for cleanup, not bare `except` + cleanup.

```python
try:
    data = fetch(url)
except ConnectionError as e:
    raise MetadataError(f"failed to fetch {url}") from e
```

## Classes

- Prefer composition over inheritance.
- Use `@dataclass` for simple data containers.
- Use `@property` for computed attributes.
- Implement `__repr__` for debuggability.

## Style

- 4 spaces indentation. Max 100 chars per line.
- Single quotes preferred (be consistent within a file).
- Trailing commas in multiline structures.
- Use f-strings for string formatting (not `.format()` or `%`).

## Testing

- Use `pytest`. Name tests `test_<what>_<condition>_<expected>`.
- AAA pattern: Arrange, Act, Assert.
- Mock external dependencies (`unittest.mock` or `pytest-mock`).
- Use `tmp_path` fixture for filesystem tests.

## Scripts

This repo uses Python for automation scripts. All scripts should:
- Have `#!/usr/bin/env python3` shebang + file header.
- Use `argparse` for CLI arguments.
- Exit with appropriate codes (`sys.exit(1)` on error).
- Include a `if __name__ == "__main__":` guard.

## Required File Header

```python
#!/usr/bin/env python3
# file: path/to/script.py
# version: 1.0.0
# guid: 123e4567-e89b-12d3-a456-426614174000
```
