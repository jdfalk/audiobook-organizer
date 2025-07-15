# file: .github/linters/README.md

# version: 1.0.0

# guid: c2d3e4f5-a6b7-89cd-ef01-23456789cdef

# Linter Configuration Files

This directory contains configuration files for Super Linter, which provides comprehensive code quality checks across multiple languages.

## Configuration Files

| File                 | Purpose                       | Language/Tool   |
| -------------------- | ----------------------------- | --------------- |
| `.python-black`      | Python code formatting        | Black formatter |
| `.pylintrc`          | Python code analysis          | PyLint          |
| `ruff.toml`          | Python linting and formatting | Ruff            |
| `.markdownlint.json` | Markdown linting              | markdownlint    |
| `.yaml-lint.yml`     | YAML linting                  | yamllint        |
| `.eslintrc.json`     | JavaScript/TypeScript linting | ESLint          |
| `.stylelintrc.json`  | CSS linting                   | StyleLint       |

## Style Guide Compliance

These configurations enforce the coding standards defined in our style guides:

- **Python**: Follows Google Python Style Guide with 80-character line length
- **Shell**: Follows Google Shell Style Guide
- **Markdown**: Follows Google Markdown Style Guide with 100-character line length
- **JavaScript/TypeScript**: Modern ES2022 standards with TypeScript support
- **CSS**: Standard CSS formatting with consistent spacing and naming
- **YAML**: Standard YAML formatting with proper indentation
- **JSON**: Standard JSON formatting and validation

## Usage

These files are automatically used by the Super Linter workflow (`reusable-super-linter.yml`) when linting is enabled in CI/CD pipelines.

### Local Development

You can use these configurations locally by:

1. **Python**:

   ```bash
   black --config .github/linters/.python-black .
   pylint --rcfile .github/linters/.pylintrc your_file.py
   ruff check --config .github/linters/ruff.toml .
   ```

2. **Markdown**:

   ```bash
   markdownlint --config .github/linters/.markdownlint.json **/*.md
   ```

3. **JavaScript/TypeScript**:

   ```bash
   eslint --config .github/linters/.eslintrc.json **/*.{js,ts,jsx,tsx}
   ```

4. **CSS**:

   ```bash
   stylelint --config .github/linters/.stylelintrc.json **/*.css
   ```

5. **YAML**:
   ```bash
   yamllint --config .github/linters/.yaml-lint.yml .
   ```

## Integration

The Super Linter workflow integrates these configurations automatically and provides:

- ✅ Comprehensive multi-language linting
- 🔧 **Auto-fixing for supported languages**
- 📊 Detailed error reporting
- 🚀 PR comment integration
- 🎯 Configurable language support
- 🔒 Security scanning (secrets, Dockerfile)
- 💾 **Automatic commit and push of fixes**

## Auto-Fix Support

The following linters support automatic fixing:

| Language/Tool         | Linter            | Auto-fix Available | What Gets Fixed                             |
| --------------------- | ----------------- | ------------------ | ------------------------------------------- |
| Python                | Black             | ✅                 | Code formatting, line length, quotes        |
| Python                | Ruff              | ✅                 | Import sorting, unused imports, basic style |
| JavaScript/TypeScript | ESLint            | ✅                 | Syntax, formatting, import organization     |
| CSS                   | StyleLint         | ✅                 | Property ordering, formatting, syntax       |
| JSON                  | jq/Prettier       | ✅                 | Formatting, indentation                     |
| Markdown              | markdownlint      | ✅                 | Heading structure, list formatting          |
| YAML                  | yamllint/Prettier | ✅                 | Indentation, formatting                     |
| Go                    | gofmt/goimports   | ✅                 | Code formatting, import organization        |
| Shell                 | shfmt             | ✅                 | Script formatting, indentation              |

### Auto-fix Configuration

Auto-fixing is enabled by default but can be controlled with these inputs:

```yaml
- name: Super Linter with Auto-fix
  uses: jdfalk/ghcommon/.github/workflows/reusable-super-linter.yml@main
  with:
    enable-auto-fix: true # Enable auto-fixing
    auto-commit-fixes: true # Commit fixes automatically
    commit-message: "style: auto-fix [skip ci]" # Custom commit message
```

### When Auto-fixes Are Applied

- **Pull Requests**: Fixes are committed to the PR branch
- **Main Branch**: Fixes are committed directly to main
- **Commits include `[skip ci]`** to prevent infinite loops

## Customization

To customize linting for specific projects:

1. Override configurations in your repository's `.github/linters/` directory
2. Modify the Super Linter environment file (`.github/super-linter.env`)
3. Adjust inputs in your workflow that calls `reusable-super-linter.yml`

For more information, see the [Super Linter documentation](https://github.com/super-linter/super-linter).
