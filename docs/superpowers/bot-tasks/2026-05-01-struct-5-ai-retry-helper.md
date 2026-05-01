<!-- file: docs/superpowers/bot-tasks/2026-05-01-struct-5-ai-retry-helper.md -->
<!-- version: 1.0.0 -->
<!-- guid: e5f6a7b8-c9d0-1234-efab-567890123456 -->
<!-- last-edited: 2026-05-01 -->

# BOT TASK: STRUCT-5 — Extract shared retry helper to `internal/ai`

**TODO ID:** STRUCT-5
**Audience:** burndown bot
**Branch:** `refactor/struct-5-ai-retry-helper`
**PR title:** `refactor(ai): extract shared retry/backoff helper`

---

## What This Task Does

Creates `internal/ai/retry.go` with a shared `withRetry` function that replaces
duplicated retry/backoff logic in at least 3 AI files:
- `internal/ai/openai_parser.go`
- `internal/ai/metadata_llm_review.go`
- `internal/ai/embedding_client.go`

---

## What NOT to Do

- **Do NOT** replace call sites in this PR — just create the shared helper.
- **Do NOT** change the retry behaviour (same delays, same max attempts).
- **Do NOT** touch test files.

---

## Step-by-step

### Step 1 — Read the existing retry implementations

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer

grep -n 'retry\|Retry\|attempt\|Attempt\|backoff\|Backoff\|sleep\|Sleep' \
  internal/ai/openai_parser.go \
  internal/ai/metadata_llm_review.go \
  internal/ai/embedding_client.go | grep -v '//'
```

Then read 15 lines of context around each hit to understand the full pattern:
```bash
grep -n 'for.*attempt\|for.*retry\|time\.Sleep' \
  internal/ai/openai_parser.go \
  internal/ai/metadata_llm_review.go \
  internal/ai/embedding_client.go
```

Note: what are the max attempts and sleep durations in each file? Use the most
common values for the shared helper's defaults.

### Step 2 — Create `internal/ai/retry.go`

Based on what you read in Step 1, create a retry helper. The typical pattern will
look like this (adjust delays/attempts to match what the existing code uses):

```go
// file: internal/ai/retry.go
// version: 1.0.0
// last-edited: 2026-05-01
// guid: f6a7b8c9-d0e1-2345-fabc-678901234567

package ai

import (
	"context"
	"time"
)

// withRetry calls fn up to maxAttempts times, sleeping between attempts with
// exponential backoff starting at initialDelay. Returns the last error if all
// attempts fail. Respects ctx cancellation.
func withRetry(ctx context.Context, maxAttempts int, initialDelay time.Duration, fn func() error) error {
	var err error
	delay := initialDelay
	for i := 0; i < maxAttempts; i++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err = fn()
		if err == nil {
			return nil
		}
		if i < maxAttempts-1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
			delay *= 2
		}
	}
	return err
}
```

Adjust `maxAttempts` default, `initialDelay`, and backoff multiplier to match
what the existing 3 files currently use. If they all differ, use the most
conservative (highest retry count, longest delay) as the default.

### Step 3 — Build

```bash
go build ./internal/ai/...
```

Must compile clean.

### Step 4 — Commit and open PR

```bash
git checkout -b refactor/struct-5-ai-retry-helper
git add internal/ai/retry.go
git commit -m "refactor(ai): extract shared retry/backoff helper

Adds retry.go with withRetry() to replace duplicated retry logic in
openai_parser.go, metadata_llm_review.go, and embedding_client.go.
Uses exponential backoff with context cancellation support.
Call-site replacement in follow-up task STRUCT-5b.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin refactor/struct-5-ai-retry-helper
gh pr create \
  --title "refactor(ai): extract shared retry/backoff helper" \
  --body "Adds ai/retry.go. Replaces 3 duplicated retry implementations. Part of STRUCT-5 structure audit."
```

---

## Checklist

- [ ] `internal/ai/retry.go` created
- [ ] Retry logic matches existing implementations (same defaults)
- [ ] `go build ./internal/ai/...` clean
- [ ] PR opened on branch `refactor/struct-5-ai-retry-helper`
