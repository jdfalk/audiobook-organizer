<!-- file: docs/superpowers/bot-tasks/2026-05-01-sec-1-remove-committed-secret.md -->
<!-- version: 1.0.0 -->
<!-- guid: d3e4f5a6-b7c8-9d0e-1f2a-3b4c5d6e7f8a -->
<!-- last-edited: 2026-05-01 -->

# BOT TASK: SEC-1 — Remove committed OpenAI API key placeholder from config.yaml

**TODO ID:** SEC-1 (re-audit; original was C-5)  
**Audience:** burndown bot  
**Branch:** `fix/sec-remove-config-secret`  
**PR title:** `fix(config): remove committed API key placeholder from config.yaml`

---

## What This Task Does

Removes the `openai_api_key: sk-test12345678` placeholder from `config.yaml` and
replaces it with an empty value plus a comment directing users to use environment
variables or a separate secrets file. This was identified in the 2026-04-30 audit
as C-5 but was NOT actioned in PRs #587–#627.

---

## What NOT to Do

- **Do NOT** replace with a real API key of any kind.
- **Do NOT** add `config.yaml` to `.gitignore` — it is a documentation template.
- **Do NOT** add any `sk-…` or `pk-…` or similar secrets to any config file.
- **Do NOT** change the key name — only the value.

---

## Read First

1. Verify the secret is still present:

```bash
grep -n 'openai_api_key\|sk-' config.yaml
```

2. Read the surrounding config documentation to understand the field:

```bash
sed -n '1,30p' config.yaml
```

3. Check how `openai_api_key` is loaded — is env-var override documented?

```bash
grep -rn 'openai_api_key\|OPENAI_API_KEY' internal/ --include='*.go' | grep -v test | head -10
grep -n 'openai_api_key\|OPENAI_API_KEY' config.yaml README.md QUICKSTART.md 2>/dev/null | head -10
```

---

## Steps

### Step 1 — Remove the placeholder value

In `config.yaml`, change:
```yaml
openai_api_key: sk-test12345678
```
to:
```yaml
# Set via OPENAI_API_KEY environment variable or a local secrets file excluded from git.
openai_api_key: ""
```

### Step 2 — Update README / QUICKSTART if needed

If `README.md` or `QUICKSTART.md` reference setting `openai_api_key` in `config.yaml`,
add a note that the value must not be committed:

```markdown
> **Security:** Never commit your real OpenAI API key to `config.yaml`.
> Use the `OPENAI_API_KEY` environment variable or a local override file
> that is listed in `.gitignore`.
```

### Step 3 — Verify the project still builds and tests pass

```bash
go build ./...
go test ./internal/config/... -timeout 30s 2>&1 | tail -5
```

### Step 4 — Commit and open PR

```bash
git checkout -b fix/sec-remove-config-secret
git add config.yaml README.md QUICKSTART.md  # only files actually changed
git commit -m "fix(config): remove committed API key placeholder from config.yaml

Replaces 'sk-test12345678' with an empty string and a comment directing
users to set OPENAI_API_KEY via environment variable. Prevents contributors
from normalising the practice of committing real keys. Re-audit finding R-3
(original audit C-5).

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin fix/sec-remove-config-secret
gh pr create \
  --title "fix(config): remove committed API key placeholder from config.yaml" \
  --body "Removes sk-test12345678 placeholder. Directs users to use environment variables. Re-audit finding R-3."
```

---

## Checklist

- [ ] `config.yaml` no longer contains any `sk-…` value
- [ ] A comment explains how to set the key via env var
- [ ] `go build ./...` and `go test ./internal/config/...` pass
- [ ] PR opened with correct branch and title
