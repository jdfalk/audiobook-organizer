#!/usr/bin/env bash
# Phase 9: Post-migration health checks.
set -euo pipefail

PASS=0
FAIL=0
WARN=0

check() {
    local label="$1"
    local cmd="$2"
    if eval "${cmd}" &>/dev/null; then
        echo "  PASS  ${label}"
        ((PASS++))
    else
        echo "  FAIL  ${label}"
        ((FAIL++))
    fi
}

warn_if_found() {
    local label="$1"
    local cmd="$2"
    local result
    result=$(eval "${cmd}" 2>/dev/null || true)
    if [[ -n "${result}" ]]; then
        echo "  WARN  ${label}"
        echo "${result}" | head -5 | sed 's/^/         /'
        ((WARN++))
    else
        echo "  PASS  ${label}"
        ((PASS++))
    fi
}

echo "=== GitHub Reachability ==="
check "falkcorp/github-common reachable" "gh repo view falkcorp/github-common --json name"
check "falkcorp/audiobook-organizer reachable" "gh repo view falkcorp/audiobook-organizer --json name"
check "falkcorp/burndown-tasks reachable" "gh repo view falkcorp/burndown-tasks --json name"
check "falkcorp/overnight-burndown reachable" "gh repo view falkcorp/overnight-burndown --json name"
check "falkcorp/gha-load-config reachable" "gh repo view falkcorp/gha-load-config --json name"
check "falkcorp/gha-release-go reachable" "gh repo view falkcorp/gha-release-go --json name"
check "falkcorp/safe-ai-util reachable" "gh repo view falkcorp/safe-ai-util --json name"
check "falkcorp/burndown-runner-image reachable" "gh repo view falkcorp/burndown-runner-image --json name"

echo ""
echo "=== Stale jdfalk/ References in Local Clones ==="
BASE="${HOME}/repos/github.com/jdfalk"
warn_if_found "No jdfalk/ in workflow uses:" \
    "grep -r 'uses: jdfalk/' ${BASE}/*/.github/workflows/ 2>/dev/null"
warn_if_found "No ghcr.io/jdfalk/ image refs in action.yml files" \
    "grep -r 'ghcr.io/jdfalk/' ${BASE}/*/action.yml 2>/dev/null"
warn_if_found "No github.com/jdfalk/gcommon in go.mod files" \
    "grep -r 'github.com/jdfalk/gcommon' ${BASE}/*/go.mod 2>/dev/null"

echo ""
echo "=== GitHub App Secrets ==="
check "BURNDOWN_BOT_APP_ID exists in falkcorp" \
    "gh secret list --org falkcorp | grep -q BURNDOWN_BOT_APP_ID"
check "BURNDOWN_BOT_PRIVATE_KEY exists in falkcorp" \
    "gh secret list --org falkcorp | grep -q BURNDOWN_BOT_PRIVATE_KEY"
check "ANTHROPIC_API_KEY exists in falkcorp" \
    "gh secret list --org falkcorp | grep -q ANTHROPIC_API_KEY"
check "BUF_TOKEN exists in falkcorp" \
    "gh secret list --org falkcorp | grep -q BUF_TOKEN"

echo ""
echo "=== Local Git Remotes ==="
for repo in audiobook-organizer burndown-tasks overnight-burndown github-common; do
    dir="${HOME}/repos/github.com/jdfalk/${repo}"
    [[ -d "${dir}" ]] || dir="${HOME}/repos/github.com/falkcorp/${repo}"
    if [[ -d "${dir}" ]]; then
        remote=$(git -C "${dir}" remote get-url origin 2>/dev/null || echo "")
        if [[ "${remote}" == *"falkcorp"* ]]; then
            echo "  PASS  ${repo} remote: ${remote}"
            ((PASS++))
        else
            echo "  FAIL  ${repo} remote still points to: ${remote}"
            ((FAIL++))
        fi
    fi
done

echo ""
echo "=== BSR ==="
check "buf.build/falkcorp/gcommon exists" \
    "buf registry module info buf.build/falkcorp/gcommon"

echo ""
echo "==============================="
echo "PASS: ${PASS}  FAIL: ${FAIL}  WARN: ${WARN}"
[[ ${FAIL} -eq 0 ]]
