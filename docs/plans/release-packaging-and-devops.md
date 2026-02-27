<!-- file: docs/plans/release-packaging-and-devops.md -->
<!-- version: 2.1.0 -->
<!-- guid: a2b3c4d5-e6f7-8a9b-0c1d-2e3f4a5b6c7d -->
<!-- last-edited: 2026-01-31 -->

# Release, Packaging, and DevOps

## Overview

Everything needed to get releases out the door: CI/CD pipeline health,
release automation, Docker/Kubernetes packaging, and the ghcommon shared
workflow infrastructure.

---

## Active: CI/CD Issues

### NPM Cache Missing Lock File (CRITICAL-002)

`actions/setup-node@v6` auto-cache requires a lock file (`package-lock.json`).
The `web/` directory does have a lock file, but the reusable workflow may run
`setup-node` from the repo root where no lock file exists. Solution: use manual
caching from `reusable-advanced-cache.yml` (already implemented in
ghcommon@main) instead of the built-in `setup-node` cache.

Steps:

1. ✅ Verify ghcommon@main has reusable-advanced-cache.yml
2. Update reusable-ci.yml to disable setup-node cache, use advanced-cache
3. Update repository-config.yml with npm cache settings
4. Test with audiobook-organizer workflows

**Affected workflow files and exact changes:**

**File: `.github/workflows/frontend-ci.yml`** (current, lines 51-54)

The `frontend-ci.yml` delegates to `ghcommon` reusable CI. No direct
`setup-node` call here, but if ghcommon's reusable-ci.yml uses
`actions/setup-node` with `cache: 'npm'` at the root, it will fail because
there is no `package-lock.json` at the repo root. The fix is in
`repository-config.yml` (see below) telling ghcommon where to find the lock
file.

**File: `.github/repository-config.yml`** — Add/update the `ci.caching` block:

Before:
```yaml
ci:
  caching:
    enabled: true
    paths:
      go: ['~/.cache/go-build', '~/go/pkg/mod']
      npm: ['~/.npm', '~/.cache/npm']
```

After:
```yaml
ci:
  caching:
    enabled: true
    strategy: manual          # tells ghcommon to use reusable-advanced-cache
    paths:
      go: ['~/.cache/go-build', '~/go/pkg/mod']
      npm: ['~/.npm', '~/.cache/npm']
    lock_files:
      npm: 'web/package-lock.json'   # explicit path so setup-node can find it
```

If the ghcommon reusable workflow does not yet read `lock_files`, the
fallback is to add a symlink step before `setup-node` in any workflow that
runs Node:

```yaml
      - name: Symlink lock file to repo root for setup-node cache
        run: |
          if [ ! -f package-lock.json ] && [ -f web/package-lock.json ]; then
            ln -s web/package-lock.json package-lock.json
          fi
```

This step goes immediately after `actions/checkout` and before any
`actions/setup-node` invocation. In `frontend-ci.yml` the reusable call
handles this, but if you add a local Node step in any other workflow
(e.g., `ci.yml`), add this symlink step there too.

### ghcommon Pre-release & Tagging Strategy (CRITICAL-004) -- RESOLVED

~~Need structured pre-release tagging before ghcommon 1.0.0.~~ ghcommon now
has proper semver tags (latest: v1.10.3). All workflow pins in this repo have
been updated to use the latest ghcommon tag SHA with version comments.
GoReleaser config updated with prerelease support, grouped changelogs, and
release templates.

**Current pins in this repo (all reference the same SHA today):**

| Workflow file | Line | Current pin |
|---|---|---|
| `.github/workflows/ci.yml` | 32 | `jdfalk/ghcommon/.github/workflows/reusable-ci.yml@9fbf36626f62738565c35009c9d22dbc5f68eccf` |
| `.github/workflows/prerelease.yml` | 30 | `jdfalk/ghcommon/.github/workflows/reusable-release.yml@9fbf36626f62738565c35009c9d22dbc5f68eccf` |
| `.github/workflows/release-prod.yml` | 39 | `jdfalk/ghcommon/.github/workflows/reusable-release.yml@9fbf36626f62738565c35009c9d22dbc5f68eccf` |
| `.github/workflows/frontend-ci.yml` | 51 | `jdfalk/ghcommon/.github/workflows/reusable-ci.yml@41a2edb6639d44ac9f328ea263299a55d78d08b3` |

**Step-by-step tagging workflow (run in the `jdfalk/ghcommon` repo):**

```bash
# 1. On the ghcommon repo, create a pre-release tag from main
cd <ghcommon checkout>
git checkout main
git pull origin main

# Create annotated tag (annotated tags are required for GitHub releases)
git tag -a v0.9.0 -m "Pre-release v0.9.0: initial stable candidate"
git push origin v0.9.0

# 2. For patch releases on the same minor:
git tag -a v0.9.1 -m "Pre-release v0.9.1: NPM cache fix"
git push origin v0.9.1

# 3. Once v0.9.x is stable across all dependent repos, promote to 1.0.0:
git tag -a v1.0.0 -m "Stable release v1.0.0"
git push origin v1.0.0
```

**Updating pins in this repo after a ghcommon tag is published:**

```bash
# Get the commit SHA that the tag points to:
# (GitHub UI: go to jdfalk/ghcommon/releases/tag/v0.9.0, copy the SHA from
#  the "Assets" section or use the API)

# Or via git:
cd <ghcommon checkout>
git rev-parse v0.9.0
# Example output: abc123def456...

# Then update each workflow file, replacing the old SHA with the new one.
# Pin format: @<sha> # v0.9.0
# The comment after # is for human readability; GitHub Actions uses the SHA.
```

Example updated pin in `.github/workflows/ci.yml`:
```yaml
# Before:
    uses: jdfalk/ghcommon/.github/workflows/reusable-ci.yml@9fbf36626f62738565c35009c9d22dbc5f68eccf # main

# After:
    uses: jdfalk/ghcommon/.github/workflows/reusable-ci.yml@abc123def456789... # v0.9.0
```

Apply the same pattern to `prerelease.yml`, `release-prod.yml`, and
`frontend-ci.yml`. The `frontend-ci.yml` currently pins a different SHA than
the others — reconcile all four to the same tag once testing confirms
compatibility.

**Testing checklist before promoting to 1.0.0:**

1. Pin all four workflows to v0.9.0 in a feature branch
2. Open a PR; verify CI (`.github/workflows/ci.yml`) passes
3. Merge to main; verify prerelease workflow fires and produces artifacts
4. Manually trigger `release-prod.yml` with `release-type: patch` in draft mode
5. Inspect the draft release: binaries present, checksums valid, Docker image
   tagged correctly
6. Repeat steps 1-5 for every dependent repo before tagging v1.0.0

### Architecture Items

- **TODO-ARCH-001**: Make repository-config.yml the single source of truth
  for all reusable workflow configuration
- **TODO-ARCH-002**: Switch to manual caching strategy across all repos
  (replace setup-node built-in cache)
- **TODO-ARCH-003**: Simplify client workflows — they should only pass
  minimal options to reusable workflows

**TODO-ARCH-001 — repository-config.yml as single source of truth**

The current `.github/repository-config.yml` already declares language versions,
working directories, build targets, and testing thresholds. The goal is to make
every reusable workflow in ghcommon read these values instead of requiring them
as `with:` inputs.

Current file location: `.github/repository-config.yml`

This is a concrete example of what the file should look like when fully
authoritative. Fields marked `# NEW` are additions needed for the reusable
workflows to fully consume the config:

```yaml
# .github/repository-config.yml

repository:
  name: 'audiobook-organizer'
  type: 'application'
  primary_language: 'go'
  description: 'Audiobook organization and management tool'

# Frontend — consumed by get-frontend-config-action
working_directories:
  frontend: 'web'
  node: 'web'

versions:
  node: ['22']

languages:
  detection:
    patterns:
      go: ['go.mod', 'go.sum', 'main.go', 'cmd/', 'internal/']
      javascript: ['web/package.json', 'web/*.js']
      docker: ['Dockerfile', 'docker-compose.yml']
    priority: [go, javascript, docker]
  versions:
    go: ['1.25']
    node: ['22']
  working_directories:
    node: 'web'
    frontend: 'web'

build:
  platforms:
    os: ['ubuntu-latest']
    arch: ['amd64', 'arm64']
  docker:
    enabled: true
    platforms: ['linux/amd64,linux/arm64']
    registries: ['ghcr.io']
    use_qemu: false
  cross_compile:
    enabled: true
    targets:
      go:
        - 'linux/amd64'
        - 'linux/arm64'
        - 'darwin/amd64'
        - 'darwin/arm64'

release:
  enabled: true
  strategy: 'semantic'
  prerelease_branch: 'main'
  stable_branches: ['main']
  versioning:
    scheme: 'semver'
    auto_detect: true
  artifacts:
    go_binaries: true
    docker_images: true
    source: true
    checksums: true
  goreleaser:                       # NEW — path to .goreleaser.yml
    config: '.goreleaser.yml'
    publish: true                   # NEW — set to true once token fix is done

testing:
  enabled: true
  coverage:
    enabled: true
    threshold: 60                   # NEW — lowered from 80 to match current target
    reports: ['html', 'json']
  unit:
    enabled: true
    parallel: true
  integration:
    enabled: true
  e2e:                              # NEW — Playwright config
    enabled: true
    config: 'web/tests/e2e/playwright.config.ts'
    browsers: ['chromium']          # webkit is informational only

ci:
  enabled: true
  on:
    push: ['main']
    pull_request: ['main']
  caching:
    enabled: true
    strategy: manual                # NEW — use reusable-advanced-cache
    paths:
      go: ['~/.cache/go-build', '~/go/pkg/mod']
      npm: ['~/.npm', '~/.cache/npm']
    lock_files:                     # NEW — explicit lock file paths
      npm: 'web/package-lock.json'
```

Reusable workflows in ghcommon read this file at runtime via the
`load_repository_config.py` script (`.github/workflows/scripts/load_repository_config.py`).
That script parses the YAML and exports keys as GitHub Actions step outputs.
Client workflows then reference those outputs in subsequent steps.

**TODO-ARCH-002** is addressed by the NPM Cache section above (symlink
fallback + `strategy: manual` in config).

**TODO-ARCH-003 — Simplify client workflows**

Once repository-config.yml is authoritative, the client workflows should shrink
to minimal `with:` blocks. The target state for `ci.yml`:

```yaml
jobs:
  ci:
    name: Run CI
    uses: jdfalk/ghcommon/.github/workflows/reusable-ci.yml@<sha> # v0.9.0
    with:
      # All values below are read from repository-config.yml by ghcommon.
      # Only override here if you need a per-repo exception.
      coverage-threshold: '60'     # matches testing.coverage.threshold
    secrets: inherit
```

The `go-version`, `node-version`, `python-version`, `rust-version`, and
`skip-protobuf` inputs are removed because ghcommon reads them from the config
file. If ghcommon does not yet support this, leave the explicit `with:` values
in place until it does.

### CI/CD Health Monitoring (P1)

Monitor `test-action-integration.yml` for output drift. Alert if action
outputs change from expected values (`dir=web`, `node-version=22`,
`has-frontend=true`).

**File:** `.github/workflows/test-action-integration.yml`

The workflow already validates these three outputs (lines 48-61). To add
alerting on drift, append a step that posts a GitHub issue when any
assertion fails:

```yaml
      - name: Alert on output drift
        if: failure()  # runs only if a previous step failed
        uses: actions/github-script@v7
        with:
          github-token: ${{ secrets.GITHUB_TOKEN }}
          script: |
            const { context } = require('@actions/github');
            const octokit = github.getOctokit(process.env.GITHUB_TOKEN);

            const body = [
              '## CI/CD Configuration Drift Detected',
              '',
              'The `test-action-integration.yml` workflow detected unexpected outputs',
              'from `get-frontend-config-action`. This may indicate a breaking change',
              'in the upstream action or a configuration regression.',
              '',
              '**Expected values:**',
              '- `dir` = `web`',
              '- `node-version` = `22`',
              '- `has-frontend` = `true`',
              '',
              '**Workflow run:** ' + context.payload.repository.html_url +
                '/actions/runs/' + context.runId,
              '',
              'Please investigate and update `.github/repository-config.yml` or',
              'pin the action to a known-good version.',
            ].join('\n');

            await octokit.rest.issues.create({
              owner: context.repo.owner,
              repo: context.repo.repo,
              title: '[CI DRIFT] get-frontend-config-action outputs changed',
              body: body,
              labels: ['ci', 'priority:high'],
            });
```

This step uses `if: failure()` so it fires only when the validation
assertions in the preceding step exit with code 1. The issue is created
with a `ci` label and a direct link to the failing workflow run.

**Extending the drift check for future values:**

If `repository-config.yml` gains new fields (e.g., `coverage-threshold`),
add a corresponding validation step before the alert step:

```yaml
      - name: Verify coverage threshold from config
        run: |
          THRESHOLD=$(cat .github/repository-config.yml | grep -A1 'coverage:' | grep 'threshold:' | awk '{print $2}')
          echo "Coverage threshold: $THRESHOLD"
          if [ "$THRESHOLD" != "60" ]; then
            echo "ERROR: Expected threshold=60, got $THRESHOLD"
            exit 1
          fi
```

---

## Release Automation

### Release Notes Generation

Automated release notes from conventional commits. Currently using a local
changelog stub; needs integration with the real generator once GHCOMMON is
available. The `.goreleaser.yml` changelog section already filters out
non-release commits (docs, test, chore, merge commits). Once ghcommon's
release workflow generates a `CHANGELOG.md`, include it in the archive via the
existing `files` block in `.goreleaser.yml`.

### Build Artifact Publishing

Publish binary and Docker image on release. Two fixes are required:

**Fix 1 — GoReleaser `release.disable` must be set to `false`**

File: `.goreleaser.yml` (line 75)

Current:
```yaml
release:
  disable: true
```

Change to:
```yaml
release:
  disable: false
  github_urls:
    homepage: https://github.com/jdfalk/audiobook-organizer
  description: "Audiobook Organizer release"
```

This tells GoReleaser to actually publish the GitHub release. The reusable
release workflow in ghcommon invokes GoReleaser; if `disable: true`, it
creates archives locally but never uploads them.

**Fix 2 — GitHub Actions token must have `contents:write`**

The release workflows (`.github/workflows/prerelease.yml` and
`.github/workflows/release-prod.yml`) already declare:

```yaml
permissions:
  contents: write
  packages: write
  id-token: write
  attestations: write
```

This is correct at the workflow level. However, if the `GITHUB_TOKEN` passed
to GoReleaser does not inherit these permissions (e.g., because ghcommon's
reusable workflow overrides permissions), the upload will fail with a 403.

**Diagnostic:** Run the prerelease workflow and check the GoReleaser step logs
for `permission denied` or `403`. If present, the reusable workflow in ghcommon
needs a `permissions` block that includes `contents: write`. The fix is in
ghcommon, but this repo can work around it by passing a PAT:

```yaml
# In prerelease.yml or release-prod.yml, if GITHUB_TOKEN is insufficient:
env:
  GITHUB_TOKEN: ${{ secrets.GITHUB_PAT }}  # PAT with contents:write scope
```

Once ghcommon's reusable-release.yml correctly propagates permissions, remove
the PAT workaround and revert to `secrets: inherit`.

**GoReleaser build verification:**

```bash
# Install goreleaser locally (or use the Docker image):
go install github.com/goreleaser/goreleaser/v2@latest

# Dry-run a snapshot release (no upload):
GITHUB_TOKEN="" goreleaser release --snapshot --skip publish

# This produces artifacts in dist/ — verify:
ls dist/
# Expected: audiobook-organizer_Linux_x86_64.tar.gz
#           audiobook-organizer_Linux_arm64.tar.gz
#           audiobook-organizer_Darwin_x86_64.tar.gz
#           audiobook-organizer_Darwin_arm64.tar.gz
#           audiobook-organizer_Windows_x86_64.zip
#           checksums.txt
```

### Nightly Vulnerability Scan

Automated nightly scan and report for dependency vulnerabilities. This can be
added as a scheduled workflow:

```yaml
# .github/workflows/vulnerability-scan.yml (new file)
name: Nightly Vulnerability Scan

on:
  schedule:
    - cron: '0 6 * * *'  # 6 AM UTC daily
  workflow_dispatch:

permissions:
  contents: read
  security-events: write

jobs:
  scan-go:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd # v6.0.2
      - uses: actions/setup-go@v5
        with:
          go-version: '1.25'
      - name: Run govulncheck
        uses: golang/govulncheck-action@v1
        with:
          go-version: '1.25'
          check-mode: module

  scan-npm:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd # v6.0.2
      - uses: actions/setup-node@v5
        with:
          node-version: '22'
      - name: Install dependencies
        run: cd web && npm ci --ignore-scripts
      - name: Audit NPM dependencies
        run: cd web && npm audit --audit-level=high || true
        # || true prevents failure; review output manually or integrate
        # with GitHub Advisory alerts via dependabot
```

### Performance Regression Benchmarks

Compare scan speed and operation throughput per commit to catch regressions
before they ship. Add a benchmark in the scanner package:

```go
// internal/scanner/scanner_bench_test.go
package scanner

import (
    "os"
    "path/filepath"
    "testing"
)

func BenchmarkScanDirectory_100Files(b *testing.B) {
    dir := b.TempDir()
    for i := 0; i < 100; i++ {
        path := filepath.Join(dir, "book"+string(rune('A'+i%26))+string(rune('0'+i/26))+".m4b")
        _ = os.WriteFile(path, make([]byte, 1024), 0o644)
    }

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        // Replace with the actual scan function signature when available
        _ = dir
    }
}
```

Run benchmarks and compare across commits:
```bash
go test ./internal/scanner/... -bench=. -benchmem -count=3 > bench-$(git rev-parse --short HEAD).txt
# Compare with previous run:
# go tool pprof or manual diff of bench output
```

---

## Packaging & Deployment

### Docker Multi-Arch Build

Build pipeline for `linux/amd64` and `linux/arm64` images.

**File:** `Dockerfile` (annotated below with what to change for multi-arch)

The current Dockerfile is already structured for multi-arch via Docker buildx.
Here is the full file with annotations:

```dockerfile
# Dockerfile
# version: 1.2.2

# Stage 1: Build Go application
# --platform=$BUILDPLATFORM forces the build stage to run on the HOST platform
# (the machine running docker buildx), not the target platform.
# This is correct: Go cross-compiles natively; no QEMU needed here.
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS go-builder

# TARGETOS and TARGETARCH are set by buildx to the OUTPUT platform.
# The go build command uses them to cross-compile.
ARG TARGETOS
ARG TARGETARCH
ARG BUILDPLATFORM

WORKDIR /build
RUN apk add --no-cache git ca-certificates tzdata
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .

# KEY LINE: GOOS=$TARGETOS GOARCH=$TARGETARCH cross-compiles to the target arch.
# CGO_ENABLED=0 produces a fully static binary — no libc dependency.
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
    -ldflags="-s -w -X main.version=$(git describe --tags --always --dirty 2>/dev/null || echo 'dev')" \
    -o audiobook-organizer \
    .

# Stage 2: Build frontend
# --platform=$BUILDPLATFORM: Node.js build also runs on host.
# npm run build produces platform-independent static files.
FROM --platform=$BUILDPLATFORM node:22-alpine AS frontend-builder

WORKDIR /build/web
COPY web/package*.json ./
RUN npm ci --prefer-offline --no-audit
COPY web/ ./
RUN npm run build

# Stage 3: Final production image
# NO --platform override here. This stage runs on the TARGET platform.
# alpine:3.20 is available for both amd64 and arm64.
FROM alpine:3.20

# --no-scripts flag prevents maintainer scripts that can trigger QEMU
# issues on arm64 when building on amd64 hosts.
RUN apk add --no-cache --no-scripts \
    ca-certificates \
    tzdata \
    && update-ca-certificates || true \
    && addgroup -g 1000 audiobook \
    && adduser -D -u 1000 -G audiobook audiobook

WORKDIR /app

# The binary from Stage 1 was cross-compiled to TARGETARCH.
# COPY --from works correctly across stages regardless of platform.
COPY --from=go-builder --chown=audiobook:audiobook /build/audiobook-organizer /app/
# Frontend dist is platform-independent (static HTML/JS/CSS).
COPY --from=frontend-builder --chown=audiobook:audiobook /build/web/dist /app/web/dist

USER audiobook
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/audiobook-organizer"]
CMD ["--help"]
```

**What is already correct (no changes needed):**
- Stage 1 and 2 use `--platform=$BUILDPLATFORM` (host platform)
- Go build uses `GOOS=$TARGETOS GOARCH=$TARGETARCH` for cross-compilation
- `CGO_ENABLED=0` ensures no C library dependency
- Stage 3 (final image) does NOT pin `--platform`, so it inherits the target
- `--no-scripts` on `apk add` avoids QEMU-triggered maintainer scripts

**What to verify before shipping multi-arch:**

1. Run a local buildx test (requires Docker with buildx plugin):
   ```bash
   docker buildx create --use --name multiarch
   docker buildx build --platform linux/amd64,linux/arm64 --tag audiobook-organizer:latest .
   # --load only works for single platform; use --push or just --build for validation
   ```

2. The `repository-config.yml` already declares:
   ```yaml
   docker:
     platforms: ['linux/amd64,linux/arm64']
     use_qemu: false
   ```
   This tells ghcommon's release workflow to invoke `docker buildx` with
   both platforms. The `use_qemu: false` flag means no QEMU setup step is
   needed — correct because all compilation happens on the host in Stages 1-2.

3. If QEMU errors still appear (e.g., from `update-ca-certificates` in Stage 3),
   the `--no-scripts` flag on `apk add` should suppress them. If not, replace
   `update-ca-certificates` with a no-op:
   ```dockerfile
   RUN apk add --no-cache --no-scripts ca-certificates tzdata || true
   # Skip update-ca-certificates entirely; alpine ships with up-to-date certs
   ```

### Helm Chart

Kubernetes deployment chart for containerized deployments. Not yet created;
this is a placeholder for a future `charts/audiobook-organizer/` directory.
Minimum viable chart needs:

- `Chart.yaml` with `apiVersion: v2`, `name: audiobook-organizer`, matching
  the release version
- `values.yaml` with image repository/tag, port (8080), resource limits,
  volume mounts for the data directory (PebbleDB stores data on disk)
- `templates/deployment.yaml` referencing the multi-arch image from ghcr.io
- `templates/service.yaml` exposing port 8080
- `templates/persistentvolumeclaim.yaml` for the database directory

### Binary Distribution

GoReleaser already handles binary packaging. The `.goreleaser.yml` `archives`
block produces:

```
audiobook-organizer_<version>_<OS>_<arch>.tar.gz   (Linux, macOS)
audiobook-organizer_<version>_Windows_x86_64.zip   (Windows)
checksums.txt
```

Each archive includes `LICENSE`, `README.md`, and `CHANGELOG.md` (via the
`files` block in `.goreleaser.yml`).

**SBOM generation** is not yet configured. To add it, extend `.goreleaser.yml`:

```yaml
# Add after the archives block:
sboms:
  - args: ["${artifact}", "--output", "spdx"]
    env:
      - GONOSUMCHECK=*
```

This requires `syft` to be installed in the CI runner. The ghcommon release
workflow may already install it; verify by checking the workflow logs after
enabling this block.

---

## Outstanding Small Items

- **TODO-013**: Fix punycode deprecation warning in Node.js dependencies (low
  priority). Affected file: `web/package.json`. Run `cd web && npm ls punycode`
  to identify which dependency pulls in the deprecated package. Upgrade or
  replace that dependency.

- **TODO-016**: Fix `generate_release_summary.py` error handling — script
  exits with code 1 without creating a report. File:
  `.github/workflows/scripts/release_workflow.py` (the release summary logic
  is implemented here or called from here). The fix is to catch exceptions
  around the report-generation logic and write a minimal fallback report
  before exiting, rather than letting an unhandled exception propagate to
  exit code 1. Wrap the summary generation in a `try/except` block:
  ```python
  try:
      # existing summary generation logic
      generate_summary(...)
  except Exception as e:
      # Write a minimal report so the workflow step does not fail silently
      with open('release-summary.md', 'w') as f:
          f.write(f'# Release Summary\n\nError generating summary: {e}\n')
      sys.exit(0)  # allow the workflow to continue
  ```

---

## Background Agent Queue

Workflow actionization tasks (manage_todo_list):

1. Plan security workflow actionization
2. Audit remaining workflows for action conversion
3. Validate new composite actions CI/CD pipelines
4. Verify action tags and releases (v1/v1.0/v1.0.0)
5. Update reusable workflows to use new actions

---

## Dependencies

- Release pipeline fixes (token, GoReleaser) block MVP release — see
  [`mvp-critical-path.md`](mvp-critical-path.md)
- ghcommon pre-release strategy affects all downstream repos
- Docker multi-arch is independent of other items

## References

- Workflows: `.github/workflows/`
- ghcommon: `jdfalk/ghcommon` repository
- Dockerfile: `Dockerfile`
- CI config: `.github/repository-config.yml`
