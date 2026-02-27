<!-- file: GHCOMMON_VERSIONING.md -->
<!-- version: 1.1.0 -->
<!-- guid: 9f8e7d6c-5b4a-3210-fedc-ba9876543210 -->
<!-- last-edited: 2026-01-19 -->

# ghcommon Workflow Versioning and Standards

This document outlines the versioning scheme and standards used by ghcommon
reusable workflows.

## Version Standards

### Language Versions (from ghcommon defaults)

| Language | Default Version      | Notes                                    |
| -------- | -------------------- | ---------------------------------------- |
| Go       | 1.24 (default input) | Actual setup uses 1.23 for compatibility |
| Node.js  | 22                   |                                          |
| Python   | 3.13                 |                                          |

### Workflow Reference Pattern

```yaml
uses: jdfalk/ghcommon/.github/workflows/reusable-ci.yml@6ed97545daef53051405663133e18fde036645e1 # v1.10.3
```

**Always pin to a specific SHA with a version comment** for reproducibility.
Use `@<sha> # v1.10.3` format. Never use `@main` in production workflows.

## reusable-ci.yml

### Inputs

```yaml
inputs:
  go-version:
    description: 'Go version to use'
    required: false
    type: string
    default: '1.24'

  node-version:
    description: 'Node.js version to use'
    required: false
    type: string
    default: '22'

  python-version:
    description: 'Python version to use'
    required: false
    type: string
    default: '3.13'
```

### Usage Example

```yaml
jobs:
  frontend:
    name: Frontend Build and Test
    uses: jdfalk/ghcommon/.github/workflows/reusable-ci.yml@6ed97545daef53051405663133e18fde036645e1 # v1.10.3
    with:
      node-version: '22'
    secrets: inherit
```

## reusable-release.yml

### Inputs

```yaml
inputs:
  release-type:
    description: 'Type of release (auto, major, minor, patch, prerelease)'
    required: false
    type: string
    default: 'auto'

  prerelease:
    description: 'Create a prerelease'
    required: false
    type: boolean
    default: false

  draft:
    description: 'Create a draft release'
    required: false
    type: boolean
    default: false

  go-enabled:
    description: 'Enable Go build'
    required: false
    type: boolean
    default: false

  frontend-enabled:
    description: 'Enable frontend build'
    required: false
    type: boolean
    default: false

  docker-enabled:
    description: 'Enable Docker build'
    required: false
    type: boolean
    default: false
```

### Usage Example

```yaml
jobs:
  prerelease:
    name: Create Prerelease Build
    uses: jdfalk/ghcommon/.github/workflows/reusable-release.yml@6ed97545daef53051405663133e18fde036645e1 # v1.10.3
    with:
      release-type: 'auto'
      prerelease: true
      draft: false
      go-enabled: true
      frontend-enabled: true
      docker-enabled: true
    secrets: inherit
```

## Version Scheme for Releases

The ghcommon workflows implement automatic semantic versioning:

### Release Types

| Type         | Behavior                                | Example                     |
| ------------ | --------------------------------------- | --------------------------- |
| `auto`       | Determines version from commit messages | feat: → minor, fix: → patch |
| `major`      | Forces major version bump               | 1.2.3 → 2.0.0               |
| `minor`      | Forces minor version bump               | 1.2.3 → 1.3.0               |
| `patch`      | Forces patch version bump               | 1.2.3 → 1.2.4               |
| `prerelease` | Creates prerelease version              | 1.2.3 → 1.2.4-rc.1          |

### Conventional Commits

The workflows use conventional commits to determine version bumps:

```text
feat: new feature     → minor version bump
fix: bug fix         → patch version bump
docs: documentation  → no version bump (unless only change)
chore: maintenance   → no version bump
BREAKING CHANGE:     → major version bump
```

## Docker Build Configuration

### Platform Matrix

```yaml
docker-matrix:
  platform:
    - linux/amd64,linux/arm64
```

### Dockerfile Requirements

1. **Multi-stage builds** for optimization
2. **Platform arguments** for cross-compilation:

   ```dockerfile
   ARG TARGETOS
   ARG TARGETARCH
   ARG BUILDPLATFORM
   ```

3. **Non-root user** for security
4. **Health checks** for container orchestration

## Required Permissions

### For CI Workflows

```yaml
permissions:
  contents: write
  actions: write
  checks: write
  packages: write
  security-events: write
  id-token: write
  attestations: write
```

### For Release Workflows

```yaml
permissions:
  contents: write
  packages: write
  id-token: write
  attestations: write
  security-events: write
```

## Common Issues and Solutions

### Issue 1: Wrong Go Version

**Problem**: Using Go version that doesn't exist (e.g., 1.25)

**Solution**: Use Go 1.23 as specified in ghcommon

```dockerfile
# ❌ Wrong
FROM golang:1.25-alpine

# ✅ Correct
FROM golang:1.23-alpine
```

```go.mod
// ❌ Wrong
go 1.25

// ✅ Correct
go 1.23
```

### Issue 2: Incorrect Workflow Reference

**Problem**: Using wrong branch or tag

**Solution**: Always use `@main`

```yaml
# ✅ Correct
uses: jdfalk/ghcommon/.github/workflows/reusable-ci.yml@6ed97545daef53051405663133e18fde036645e1 # v1.10.3
```

### Issue 3: Missing Permissions

**Problem**: Workflow fails with permission errors

**Solution**: Include all required permissions in calling workflow

```yaml
permissions:
  contents: write
  packages: write
  id-token: write
  attestations: write
```

## Update Checklist

When updating to latest ghcommon standards:

- [x] Update workflow references to pinned SHA with version comment
- [ ] Verify Go version is 1.23
- [ ] Verify Node version is 22
- [ ] Verify Python version is 3.13 (if used)
- [ ] Update Dockerfile Go version
- [ ] Update go.mod version
- [ ] Verify all required permissions are set
- [ ] Test workflows in PR before merging

## Additional Resources

- [ghcommon Repository](https://github.com/jdfalk/ghcommon)
- [Conventional Commits](https://www.conventionalcommits.org/)
- [Semantic Versioning](https://semver.org/)
- [GitHub Actions Documentation](https://docs.github.com/en/actions)
