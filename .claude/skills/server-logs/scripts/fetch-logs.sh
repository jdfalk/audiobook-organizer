#!/bin/bash
# file: .claude/skills/server-logs/scripts/fetch-logs.sh
# version: 1.0.0
# Fetch logs from audiobook-organizer service and display login credentials

set -euo pipefail

TOKEN_FILE="./.api-token"
COMMAND="${1:-status}"

# Check if token file exists
if [ ! -f "$TOKEN_FILE" ]; then
    echo "❌ No .api-token file found"
    echo ""
    echo "Run 'server-bootstrap' skill first to create it:"
    echo "  /server-bootstrap"
    exit 1
fi

# Source the token file
source "$TOKEN_FILE"

if [ -z "${api_key:-}" ] || [ -z "${server_ip:-}" ]; then
    echo "❌ .api-token file is incomplete or invalid"
    exit 1
fi

# Helper: run command on server via SSH
run_ssh() {
    ssh "root@$server_ip" "$@"
}

# Helper: run sudo command
run_sudo() {
    ssh "root@$server_ip" "sudo $@"
}

case "$COMMAND" in
    status)
        echo "📋 Last 50 log lines from audiobook-organizer.service:"
        echo ""
        run_sudo "journalctl -u audiobook-organizer.service -n 50"
        ;;

    stream)
        echo "🔴 Streaming logs (Ctrl+C to stop)..."
        echo ""
        run_sudo "journalctl -fu audiobook-organizer.service"
        ;;

    errors)
        echo "⚠️  Log entries with ERROR level:"
        echo ""
        run_sudo "journalctl -u audiobook-organizer.service" | grep "level=ERROR" || echo "(No errors found)"
        ;;

    warnings)
        echo "⚠️  Log entries with WARN or ERROR level:"
        echo ""
        run_sudo "journalctl -u audiobook-organizer.service" | grep -E "level=(WARN|ERROR)" || echo "(No warnings or errors found)"
        ;;

    since:*)
        # Extract duration (e.g., "since:1h" → "1 hour ago")
        DURATION="${COMMAND#since:}"
        echo "📋 Logs since $DURATION:"
        echo ""
        run_sudo "journalctl -u audiobook-organizer.service --since '$DURATION ago'"
        ;;

    login)
        echo "🔐 Web UI Login Credentials"
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo ""
        echo "URL:      http://$server_ip:$api_port"
        echo "Username: $username"
        echo "API Key:  ${api_key:0:20}...${api_key: -10}"
        echo ""
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo ""
        echo "To log in:"
        echo "  1. Open http://$server_ip:$api_port in a browser"
        echo "  2. Username: $username"
        echo "  3. Use the API key or password from bootstrap"
        echo ""
        echo "To use API:"
        echo "  curl -H \"Authorization: Bearer $api_key\" \\"
        echo "    http://$server_ip:$api_port/api/v1/audiobooks"
        echo ""

        # Show expiry if available
        if [ -n "${expires_at:-}" ]; then
            EXPIRES_DATE=$(date -d "@$expires_at" '+%Y-%m-%d %H:%M:%S')
            EXPIRES_IN=$(( expires_at - $(date +%s) ))
            if [ "$EXPIRES_IN" -lt 0 ]; then
                echo "⚠️  Token EXPIRED at $EXPIRES_DATE"
                echo "   Run 'server-bootstrap' skill to get a fresh token"
            else
                HOURS=$(( EXPIRES_IN / 3600 ))
                MINS=$(( (EXPIRES_IN % 3600) / 60 ))
                echo "✓ Token expires in ~${HOURS}h ${MINS}m"
            fi
        fi
        ;;

    *)
        echo "Usage: $0 <command>"
        echo ""
        echo "Commands:"
        echo "  status              Last 50 log lines"
        echo "  stream              Live stream (Ctrl+C to stop)"
        echo "  errors              Lines with level=ERROR"
        echo "  warnings            Lines with level=WARN or ERROR"
        echo "  since:<duration>    Logs from duration ago (e.g., since:1h, since:10m)"
        echo "  login               Show login credentials + web UI instructions"
        echo ""
        exit 1
        ;;
esac
