#!/usr/bin/env bash
# Run AFTER migrate-bsr.sh and verifying buf.build/falkcorp/gcommon works.
set -euo pipefail

if [[ -z "${BUF_TOKEN:-}" ]]; then
    echo "ERROR: BUF_TOKEN not set"
    exit 1
fi
export BUF_TOKEN

buf registry module deprecate buf.build/jdfalk/gcommon \
    --message "Moved to buf.build/falkcorp/gcommon — update your buf.yaml deps"
echo "Old BSR module buf.build/jdfalk/gcommon marked deprecated."
