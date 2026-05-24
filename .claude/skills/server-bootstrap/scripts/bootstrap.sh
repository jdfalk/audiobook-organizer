#!/bin/bash
# file: .claude/skills/server-bootstrap/scripts/bootstrap.sh
# version: 1.0.0
# Restart audiobook-organizer service, extract bootstrap token, exchange for API key

set -euo pipefail

SERVER_IP="${1:?Server IP required (e.g., 172.16.2.30)}"
API_PORT="${2:-8484}"
TOKEN_FILE="./.api-token"
EXPIRES_IN_SECONDS=$((8 * 3600))

# Step 1: Restart service and wait for token to appear in logs
echo "[1/4] Connecting to $SERVER_IP and restarting audiobook-organizer service..."
ssh "root@$SERVER_IP" 'sudo systemctl restart audiobook-organizer.service' || {
    echo "❌ SSH failed or service restart failed"
    exit 1
}

# Step 2: Extract bootstrap token from journalctl (waits up to 10 seconds)
echo "[2/4] Waiting for bootstrap token in logs..."
BOOTSTRAP_TOKEN=""
for i in {1..20}; do
    BOOTSTRAP_TOKEN=$(ssh "root@$SERVER_IP" 'sudo journalctl -u audiobook-organizer.service -n 50' 2>/dev/null | grep 'msg="Emergency access token"' | grep -oP 'raw=\K[^ ]+' | head -1)
    if [ -n "$BOOTSTRAP_TOKEN" ]; then
        break
    fi
    sleep 0.5
done

if [ -z "$BOOTSTRAP_TOKEN" ]; then
    echo "❌ Could not extract bootstrap token from logs"
    exit 1
fi
echo "✓ Got bootstrap token: ${BOOTSTRAP_TOKEN:0:15}..."

# Step 3: Exchange token for API key via /api/v1/auth/bootstrap
echo "[3/4] Exchanging bootstrap token for API key..."
RESPONSE=$(curl -s -X POST "http://$SERVER_IP:$API_PORT/api/v1/auth/bootstrap" \
    -H "Content-Type: application/json" \
    -d "{\"token\":\"$BOOTSTRAP_TOKEN\", \"key_name\":\"Claude-workspace-$(date +%s)\"}" || echo "")

if [ -z "$RESPONSE" ]; then
    echo "❌ Bootstrap endpoint request failed"
    exit 1
fi

# Parse response JSON
API_KEY=$(echo "$RESPONSE" | grep -oP '"api_key":"\K[^"]+' || true)
KEY_ID=$(echo "$RESPONSE" | grep -oP '"key_id":"\K[^"]+' || true)
USERNAME=$(echo "$RESPONSE" | grep -oP '"username":"\K[^"]+' || true)

if [ -z "$API_KEY" ]; then
    echo "❌ Failed to exchange token for API key"
    echo "Response: $RESPONSE"
    exit 1
fi
echo "✓ Got API key: ${API_KEY:0:15}..."

# Step 4: Write token file
echo "[4/4] Writing token to .claude/.api-token..."
EXPIRES_AT=$(($(date +%s) + EXPIRES_IN_SECONDS))
mkdir -p .claude

cat > "$TOKEN_FILE" <<EOF
api_key=$API_KEY
key_id=$KEY_ID
username=$USERNAME
server_ip=$SERVER_IP
api_port=$API_PORT
expires_at=$EXPIRES_AT
EOF

chmod 600 "$TOKEN_FILE"
echo "✓ Token written to $TOKEN_FILE"

# Step 5: Schedule cleanup
echo "[5/5] Scheduling cleanup in ${EXPIRES_IN_SECONDS}s..."
nohup sh -c "sleep $EXPIRES_IN_SECONDS && rm -f '$TOKEN_FILE' && echo 'Cleaned up $TOKEN_FILE'" > /dev/null 2>&1 &

echo ""
echo "✅ Bootstrap complete!"
echo "   API Key: ${API_KEY:0:20}...${API_KEY: -5}"
echo "   Expires: $(date -d @$EXPIRES_AT '+%Y-%m-%d %H:%M:%S')"
echo ""
echo "This API key is shared across all worktrees. Other worktrees can now use it."
