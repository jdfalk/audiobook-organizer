# Linter Configuration Files

This directory contains configuration files for various linters used by Super Linter in the CI/CD
pipeline.

## Files

- `.markdownlint.json` - Markdown linting rules
- `.yaml-lint.yml` - YAML linting rules
- `.python-black` - Python Black formatter configuration
- `.pylintrc` - Pylint configuration for Python code analysis
- `ruff.toml` - Ruff linter configuration for Python

## Usage

These configurations are automatically used by Super Linter when running in GitHub Actions. The
configurations enforce our coding standards as defined in the `.github/instructions/` directory.

## Customization

When modifying these files, ensure they align with:

- General coding instructions in `.github/instructions/general-coding.instructions.md`
- Language-specific instructions in `.github/instructions/<language>.instructions.md`
- Project coding standards and style guides
