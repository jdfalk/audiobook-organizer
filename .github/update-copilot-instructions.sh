#!/bin/bash
# file: copilot/update-copilot-instructions.sh
# version: 1.0.0
# guid: 3f9e8a2c-5d0e-6f3f-9a2c-8f5d3f9e8a2c

# Update Copilot Instructions
# This script updates the repository's Copilot instructions based on the common workflows

set -euo pipefail

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

# Check if we're in a git repository
if ! git rev-parse --git-dir > /dev/null 2>&1; then
    echo "Error: Not in a git repository"
    exit 1
fi

# Create .github directory if it doesn't exist
mkdir -p .github

log_info "Updating repository Copilot instructions..."

# Create or update copilot instructions
cat > .github/copilot-instructions.md << 'EOF'
# file: .github/copilot-instructions.md

# Copilot Instructions for GitHub Common Workflows

This repository is configured to use the GitHub Common Workflows from `jdfalk/ghcommon`.

## Workflow Integration

When suggesting or implementing workflows, always:

1. **Use the reusable workflows** from `jdfalk/ghcommon` instead of creating custom ones
2. **Follow conventional commit format** for automatic versioning
3. **Include security best practices** from the common security guidelines
4. **Validate configuration** using the provided validation scripts

## Available Workflows

### Semantic Versioning
```yaml
versioning:
  uses: jdfalk/ghcommon/.github/workflows/reusable-semantic-versioning.yml@main
  with:
    version-files: '["package.json", "version.txt"]'
    update-pr-title: true
    dry-run: ${{ github.event_name == 'pull_request' }}
```

### Container Builds
```yaml
container:
  uses: jdfalk/ghcommon/.github/workflows/buildah-multiarch.yml@main
  with:
    image-name: ${{ github.event.repository.name }}
    platforms: linux/amd64,linux/arm64
    generate-sbom: true
    generate-attestation: true
    scan-vulnerability: true
```

### Automatic Releases
```yaml
release:
  uses: jdfalk/ghcommon/.github/workflows/automatic-release.yml@main
  with:
    release-type: auto
    include-artifacts: true
    container-image: ${{ needs.container.outputs.image-url }}
```

## Security Guidelines

Always follow these security practices:
- Use least privilege permissions
- Pin action versions to specific commits
- Generate SBOMs for containers
- Include vulnerability scanning
- Sign container images
- Validate all inputs

## Conventional Commits

Use conventional commit format for automatic versioning:
- `feat:` - New features (minor version bump)
- `fix:` - Bug fixes (patch version bump)
- `feat!:` or `fix!:` - Breaking changes (major version bump)
- `docs:`, `style:`, `refactor:`, `test:`, `chore:` - No version bump

## Validation

Validate your setup with:
```bash
curl -sSL https://raw.githubusercontent.com/jdfalk/ghcommon/main/copilot/validate-setup.sh | bash
```

For more information, see: https://github.com/jdfalk/ghcommon
EOF

log_success "Created .github/copilot-instructions.md"

# Update existing copilot instructions if they exist
if [ -f ".github/copilot-instructions.md" ]; then
    log_success "Updated existing Copilot instructions"
else
    log_success "Created new Copilot instructions"
fi

# Create workflow starter if none exists
if [ ! -d ".github/workflows" ] || [ -z "$(ls -A .github/workflows 2>/dev/null)" ]; then
    log_info "No workflows detected. Creating starter workflow..."

    mkdir -p .github/workflows

    # Determine project type based on files present
    if [ -f "Dockerfile" ]; then
        WORKFLOW_TYPE="container"
    elif [ -f "package.json" ] || [ -f "setup.py" ] || [ -f "Cargo.toml" ]; then
        WORKFLOW_TYPE="library"
    else
        WORKFLOW_TYPE="complete"
    fi

    log_info "Detected project type: $WORKFLOW_TYPE"

    # Download appropriate template
    case "$WORKFLOW_TYPE" in
        container)
            curl -fsSL "https://raw.githubusercontent.com/jdfalk/ghcommon/main/templates/workflows/container-only.yml" \
                -o ".github/workflows/ci-cd.yml"
            ;;
        library)
            curl -fsSL "https://raw.githubusercontent.com/jdfalk/ghcommon/main/templates/workflows/library-release.yml" \
                -o ".github/workflows/release.yml"
            ;;
        *)
            curl -fsSL "https://raw.githubusercontent.com/jdfalk/ghcommon/main/templates/workflows/complete-ci-cd.yml" \
                -o ".github/workflows/ci-cd.yml"
            ;;
    esac

    log_success "Created starter workflow for $WORKFLOW_TYPE project"
fi

echo
log_success "Repository updated with GitHub Common Workflows integration!"
echo
log_info "Next steps:"
echo "  1. Review the workflow file in .github/workflows/"
echo "  2. Customize for your specific project needs"
echo "  3. Start using conventional commit messages"
echo "  4. Test with a pull request"
echo
log_info "For help: https://github.com/jdfalk/ghcommon"
