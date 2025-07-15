#!/bin/bash
# file: copilot/setup-repository.sh
# version: 1.0.0
# guid: 2e8f7a1b-4c9d-5e2f-8a1b-7e4c2e8f7a1b

# Repository Setup Script
# This script helps automate the setup of a repository to use the reusable workflows

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if we're in a git repository
check_git_repo() {
    if ! git rev-parse --git-dir > /dev/null 2>&1; then
        log_error "Not in a git repository. Please run this script from the root of your git repository."
        exit 1
    fi
}

# Create .github/workflows directory if it doesn't exist
create_workflows_dir() {
    if [ ! -d ".github/workflows" ]; then
        log_info "Creating .github/workflows directory..."
        mkdir -p .github/workflows
        log_success "Created .github/workflows directory"
    fi
}

# Download workflow template
download_template() {
    local template_name="$1"
    local output_file="$2"
    local base_url="https://raw.githubusercontent.com/jdfalk/ghcommon/main/templates/workflows"

    log_info "Downloading $template_name template..."

    if curl -fsSL "$base_url/$template_name" -o ".github/workflows/$output_file"; then
        log_success "Downloaded $template_name to .github/workflows/$output_file"
    else
        log_error "Failed to download $template_name"
        return 1
    fi
}

# Create example Dockerfile
create_dockerfile() {
    if [ ! -f "Dockerfile" ]; then
        log_info "Creating example Dockerfile..."
        cat > Dockerfile << 'EOF'
# Multi-stage build for optimal image size
FROM node:20-alpine AS builder

WORKDIR /app

# Copy package files
COPY package*.json ./

# Install dependencies
RUN npm ci --only=production && npm cache clean --force

# Production stage
FROM node:20-alpine

# Create non-root user
RUN addgroup -g 1001 -S nodejs && \
    adduser -S nextjs -u 1001

WORKDIR /app

# Copy dependencies from builder stage
COPY --from=builder /app/node_modules ./node_modules
COPY --chown=nextjs:nodejs . .

# Switch to non-root user
USER nextjs

# Expose port
EXPOSE 3000

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD curl -f http://localhost:3000/health || exit 1

# Start application
CMD ["npm", "start"]
EOF
        log_success "Created example Dockerfile"
        log_warning "Please customize the Dockerfile for your specific application"
    else
        log_info "Dockerfile already exists, skipping creation"
    fi
}

# Create version file
create_version_file() {
    if [ ! -f "version.txt" ] && [ ! -f "package.json" ]; then
        log_info "Creating version.txt file..."
        echo "1.0.0" > version.txt
        log_success "Created version.txt with initial version 1.0.0"
    fi
}

# Create GitHub issue templates
create_issue_templates() {
    if [ ! -d ".github/ISSUE_TEMPLATE" ]; then
        log_info "Creating GitHub issue templates..."
        mkdir -p .github/ISSUE_TEMPLATE

        # Bug report template
        cat > .github/ISSUE_TEMPLATE/bug_report.yml << 'EOF'
name: Bug Report
description: File a bug report
title: "[Bug]: "
labels: ["bug", "triage"]
body:
  - type: markdown
    attributes:
      value: |
        Thanks for taking the time to fill out this bug report!
  - type: textarea
    id: what-happened
    attributes:
      label: What happened?
      description: Also tell us, what did you expect to happen?
      placeholder: Tell us what you see!
    validations:
      required: true
  - type: textarea
    id: reproduction
    attributes:
      label: Steps to reproduce
      description: How can we reproduce this issue?
      placeholder: |
        1. Go to '...'
        2. Click on '....'
        3. Scroll down to '....'
        4. See error
    validations:
      required: true
  - type: textarea
    id: logs
    attributes:
      label: Relevant log output
      description: Please copy and paste any relevant log output.
      render: shell
EOF

        # Feature request template
        cat > .github/ISSUE_TEMPLATE/feature_request.yml << 'EOF'
name: Feature Request
description: Suggest an idea for this project
title: "[Feature]: "
labels: ["enhancement", "triage"]
body:
  - type: markdown
    attributes:
      value: |
        Thanks for suggesting a new feature!
  - type: textarea
    id: problem
    attributes:
      label: Is your feature request related to a problem?
      description: A clear and concise description of what the problem is.
      placeholder: I'm always frustrated when [...]
  - type: textarea
    id: solution
    attributes:
      label: Describe the solution you'd like
      description: A clear and concise description of what you want to happen.
    validations:
      required: true
  - type: textarea
    id: alternatives
    attributes:
      label: Describe alternatives you've considered
      description: A clear and concise description of any alternative solutions or features you've considered.
  - type: textarea
    id: context
    attributes:
      label: Additional context
      description: Add any other context or screenshots about the feature request here.
EOF

        log_success "Created GitHub issue templates"
    fi
}

# Create pull request template
create_pr_template() {
    if [ ! -f ".github/pull_request_template.md" ]; then
        log_info "Creating pull request template..."
        cat > .github/pull_request_template.md << 'EOF'
## Description

Brief description of the changes made in this PR.

## Type of Change

- [ ] Bug fix (non-breaking change which fixes an issue)
- [ ] New feature (non-breaking change which adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to not work as expected)
- [ ] Documentation update
- [ ] Performance improvement
- [ ] Code refactoring

## Testing

- [ ] I have tested these changes locally
- [ ] I have added tests that prove my fix is effective or that my feature works
- [ ] New and existing unit tests pass locally with my changes
- [ ] I have checked that my code follows the project's style guidelines

## Documentation

- [ ] I have updated the documentation accordingly
- [ ] I have updated the CHANGELOG.md (if applicable)

## Screenshots (if applicable)

## Additional Notes

Any additional information or context about this PR.
EOF
        log_success "Created pull request template"
    fi
}

# Create .gitignore if it doesn't exist
create_gitignore() {
    if [ ! -f ".gitignore" ]; then
        log_info "Creating .gitignore file..."
        cat > .gitignore << 'EOF'
# Dependencies
node_modules/
*.log
npm-debug.log*
yarn-debug.log*
yarn-error.log*

# Runtime data
pids
*.pid
*.seed
*.pid.lock

# Coverage directory used by tools like istanbul
coverage/
*.lcov

# Build outputs
dist/
build/
*.tgz

# Environment variables
.env
.env.local
.env.development.local
.env.test.local
.env.production.local

# IDE
.vscode/
.idea/
*.swp
*.swo

# OS
.DS_Store
Thumbs.db

# Temporary files
*.tmp
*.temp
EOF
        log_success "Created .gitignore file"
    fi
}

# Show next steps
show_next_steps() {
    echo
    log_success "Repository setup completed!"
    echo
    log_info "Next steps:"
    echo "  1. Review and customize the workflow file in .github/workflows/"
    echo "  2. Update the Dockerfile for your specific application"
    echo "  3. Add any required secrets to your repository settings"
    echo "  4. Configure branch protection rules"
    echo "  5. Start using conventional commit messages"
    echo
    log_info "For detailed setup instructions, see:"
    echo "  https://github.com/jdfalk/ghcommon/blob/main/copilot/repository-setup.md"
}

# Main function
main() {
    local workflow_type="${1:-complete}"

    echo "🚀 Setting up repository for reusable workflows..."
    echo

    # Validate arguments
    case "$workflow_type" in
        complete|container|library)
            ;;
        *)
            log_error "Invalid workflow type. Use: complete, container, or library"
            echo "Usage: $0 [complete|container|library]"
            exit 1
            ;;
    esac

    # Run setup steps
    check_git_repo
    create_workflows_dir

    # Download appropriate template
    case "$workflow_type" in
        complete)
            download_template "complete-ci-cd.yml" "ci-cd.yml"
            create_dockerfile
            ;;
        container)
            download_template "container-only.yml" "container.yml"
            create_dockerfile
            ;;
        library)
            download_template "library-release.yml" "release.yml"
            ;;
    esac

    # Create supporting files
    create_version_file
    create_issue_templates
    create_pr_template
    create_gitignore

    # Show next steps
    show_next_steps
}

# Run main function with all arguments
main "$@"
