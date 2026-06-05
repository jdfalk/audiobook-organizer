#!/usr/bin/env bash
# Phase 10: Migrate gcommon-proto from buf.build/jdfalk to buf.build/falkcorp.
# Requires: buf CLI + BUF_TOKEN env var
set -euo pipefail

OLD_ORG="jdfalk"
NEW_ORG="falkcorp"
MODULES=("gcommon")
GCOMMON_PROTO_DIR="${HOME}/repos/github.com/jdfalk/gcommon-proto"

if [[ -z "${BUF_TOKEN:-}" ]]; then
    echo "ERROR: BUF_TOKEN environment variable not set"
    exit 1
fi

export BUF_TOKEN

echo "=== Step 1: Create BSR org buf.build/${NEW_ORG} ==="
if buf registry organization create "buf.build/${NEW_ORG}" 2>&1 | grep -q "already_exists"; then
    echo "  SKIP org already exists"
else
    echo "  OK created buf.build/${NEW_ORG}"
fi

echo ""
echo "=== Step 2: Create modules ==="
for mod in "${MODULES[@]}"; do
    if buf registry module get "buf.build/${NEW_ORG}/${mod}" &>/dev/null; then
        echo "  SKIP buf.build/${NEW_ORG}/${mod} already exists"
    else
        buf registry module create "buf.build/${NEW_ORG}/${mod}" \
            --visibility public \
            --default-label-name main
        echo "  OK created buf.build/${NEW_ORG}/${mod}"
    fi
done

echo ""
echo "=== Step 3: Update buf.yaml in gcommon-proto ==="
if [[ -f "${GCOMMON_PROTO_DIR}/buf.yaml" ]]; then
    sed -i.bak "s|buf.build/${OLD_ORG}/|buf.build/${NEW_ORG}/|g" "${GCOMMON_PROTO_DIR}/buf.yaml"
    echo "  OK updated ${GCOMMON_PROTO_DIR}/buf.yaml"
    diff "${GCOMMON_PROTO_DIR}/buf.yaml.bak" "${GCOMMON_PROTO_DIR}/buf.yaml" || true
    rm "${GCOMMON_PROTO_DIR}/buf.yaml.bak"
else
    echo "  WARN ${GCOMMON_PROTO_DIR}/buf.yaml not found — update manually"
fi

echo ""
echo "=== Step 4: Push to new BSR path ==="
echo "  Run from ${GCOMMON_PROTO_DIR}:"
echo "    cd ${GCOMMON_PROTO_DIR} && buf push --label main"
echo ""
echo "  (CI will auto-push after buf.yaml update is merged)"

echo ""
echo "=== Next: Run deprecate-bsr-jdfalk.sh after verifying new path works ==="
