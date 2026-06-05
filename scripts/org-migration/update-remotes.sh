#!/usr/bin/env bash
# Phase 8: Update all local git remotes to point to falkcorp.
# Run from any directory. Reads local clone dirs under ~/repos/github.com/jdfalk/.
set -eo pipefail

BASE="${HOME}/repos/github.com/jdfalk"
FALKCORP_DIR="${HOME}/repos/github.com/falkcorp"

# Map of old-name → new-name for repos that were renamed
declare -A RENAMES=(
    ["ghcommon"]="github-common"
    ["release-go-action"]="gha-release-go"
    ["release-python-action"]="gha-release-python"
    ["release-rust-action"]="gha-release-rust"
    ["release-docker-action"]="gha-release-docker"
    ["release-frontend-action"]="gha-release-frontend"
    ["release-protobuf-action"]="gha-release-protobuf-base"
    ["auto-module-tagging-action"]="gha-auto-module-tagging"
    ["ci-workflow-helpers-action"]="gha-ci-workflow-helpers"
    ["generate-version-action"]="gha-generate-version"
    ["get-frontend-config-action"]="gha-get-frontend-config"
    ["package-assets-action"]="gha-package-assets"
    ["docs-generator-action"]="gha-docs-generator"
    ["release-strategy-action"]="gha-release-strategy"
    ["update-action-docker-ref-action"]="gha-update-action-docker-ref"
    ["load-config-action"]="gha-load-config"
    ["detect-languages-action"]="gha-detect-languages"
    ["ci-generate-matrices-action"]="gha-ci-generate-matrices"
    ["security-summary-action"]="gha-security-summary"
    ["pr-auto-label-action"]="gha-pr-auto-label"
    ["jft-github-actions"]="gha-template-repo"
)

mkdir -p "${FALKCORP_DIR}"

for dir in "${BASE}"/*/; do
    [[ -d "${dir}/.git" ]] || continue
    repo=$(basename "${dir}")
    new_name="${RENAMES[$repo]:-$repo}"
    new_remote="https://github.com/falkcorp/${new_name}.git"
    current_remote=$(git -C "${dir}" remote get-url origin 2>/dev/null || echo "")

    if [[ -z "${current_remote}" ]]; then
        echo "SKIP  ${repo} (no remote)"
        continue
    fi

    if [[ "${current_remote}" == *"github.com/jdfalk/"* ]] || [[ "${current_remote}" == *"github.com/falkcorp/"* && "${current_remote}" != "${new_remote}" ]]; then
        echo "UPDATE ${repo} → ${new_remote}"
        git -C "${dir}" remote set-url origin "${new_remote}"
    else
        echo "OK    ${repo} (already ${current_remote})"
    fi

    # Create symlink in falkcorp dir pointing to jdfalk dir
    symlink="${FALKCORP_DIR}/${new_name}"
    if [[ ! -e "${symlink}" ]]; then
        ln -s "${dir}" "${symlink}"
        echo "LINK  ${FALKCORP_DIR}/${new_name} → ${dir}"
    fi
done

echo ""
echo "Done. Run 'git -C <repo> fetch' in each repo to verify connectivity."
echo "Symlinks created at ${FALKCORP_DIR}/ for IDE and shell history compatibility."
