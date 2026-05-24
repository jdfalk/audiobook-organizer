# Server Login & API Access

## Web UI Dashboard

Once you have credentials from `server-logs` skill, open the web UI:

```
http://<server-ip>:8484
```

### Logging In

1. **With API Key** (from `.api-token`):
   - Leave username blank or enter `admin`
   - Paste the API key in the password field
   - Click Login

2. **With Password** (if set during bootstrap):
   - Username: `admin`
   - Password: From `server-bootstrap` output (one-time, should change after first login)

### What You Can Do

- **Library**: Browse, search, filter audiobooks
- **Activity Log**: See all operations (scans, imports, metadata updates)
- **Diagnostics**: View batch operations, AI analysis results
- **Settings**: Configure scanner paths, maintenance tasks
- **Users & API Keys**: Manage access (admin only)

## API Access

### Authentication

All API requests require the `Authorization: Bearer` header:

```bash
API_KEY="abbs_xxxxxxxxxxxxx"

curl -H "Authorization: Bearer $API_KEY" \
  http://<server-ip>:8484/api/v1/audiobooks
```

### Common Endpoints

```bash
# List all audiobooks
GET /api/v1/audiobooks

# Get a specific book
GET /api/v1/audiobooks/{book-id}

# Batch operations
POST /api/v1/audiobooks/batch-operations

# Scan status
GET /api/v1/audiobooks/scan/status

# Activity log
GET /api/v1/activity/digest
```

See `.github/copilot-instructions.md` or API docs in project for full endpoint reference.

### API Key Scopes

Bootstrap keys have **all scopes** enabled (`scopes: ["all"]`), meaning:
- Read all data
- Write/modify audiobooks
- Run operations
- Manage users and API keys

## SSH Access (for debugging)

If you need direct server access:

```bash
# SSH to server
ssh root@<server-ip>

# Check service status
sudo systemctl status audiobook-organizer.service

# View live logs
sudo journalctl -fu audiobook-organizer.service

# Restart service
sudo systemctl restart audiobook-organizer.service

# Check disk usage
df -h
zfs list  # If using ZFS

# Check open files
lsof | grep audiobook-organizer
```

## Credentials File (.api-token)

The `.api-token` file (created by `server-bootstrap` skill) contains:

```bash
api_key=abbs_xxxxxxxxxxxxx
key_id=ulid-...
username=admin
server_ip=<server-ip>
api_port=8484
expires_at=1716470400
```

**Important:**
- This file is shared across all worktrees in the repo
- Added to `.gitignore` — never commit
- Auto-cleaned after 8 hours
- Read it with: `source .api-token && echo $api_key`

## Common Tasks

### Test API Connection

```bash
# After getting credentials
source .api-token
curl -H "Authorization: Bearer $api_key" \
  http://$server_ip:$api_port/api/v1/audiobooks | head -20
```

### Restart Service and Wait for Startup

```bash
ssh root@<server-ip> 'sudo systemctl restart audiobook-organizer.service'
sleep 5
./scripts/fetch-logs.sh status
```

### Check for Errors During Operation

```bash
# Terminal 1: Watch logs
./scripts/fetch-logs.sh stream

# Terminal 2: Trigger operation
curl -X POST http://<server-ip>:8484/api/v1/audiobooks/scan
```

### Export Activity Log

```bash
source .api-token
curl -H "Authorization: Bearer $api_key" \
  'http://<server-ip>:8484/api/v1/activity/digest?limit=1000' \
  | jq . > activity.json
```

## Troubleshooting

### "Invalid bootstrap token"
- Token is 10 minutes old
- Service was restarted, generating a new token
- Run `server-bootstrap` skill again

### "401 Unauthorized" on API call
- API key has expired (8 hours old)
- Run `server-bootstrap` skill again to get fresh key

### "Connection refused"
- Service is down: Check logs or restart
- Wrong IP/port: Check `.api-token` file
- Network issue: SSH test first

### "No admin user found"
- First bootstrap creates the `admin` user
- Check if database was reset
- Run bootstrap again to create fresh admin

## See Also

- `server-bootstrap` — Generate fresh credentials
- `server-logs` — Fetch service logs and login info
- `build-deploy` — Build and deploy new version
