# Welcome to Falkcorp Engineering

## How We Use Claude

Based on Johnathan Falk's usage over the last 30 days (126 sessions):

Work Type Breakdown:
  Build Feature    ███████░░░░░░░░░░░░░  35%
  Debug & Fix      ██████░░░░░░░░░░░░░░  30%
  Improve Quality  █████░░░░░░░░░░░░░░░  25%
  Plan & Design    ██░░░░░░░░░░░░░░░░░░   8%
  Write Docs       █░░░░░░░░░░░░░░░░░░░   2%

Top Commands:
  /clear    ████████████████████  44x
  /model    █████████████░░░░░░░  34x
  /compact  █████████████░░░░░░░  29x
  /exit     ██████████░░░░░░░░░░  23x
  /rename   ████████░░░░░░░░░░░░  18x
  /ship     ████░░░░░░░░░░░░░░░░   9x
  /effort   ██░░░░░░░░░░░░░░░░░░   5x

Top MCP Servers:
  GitHub          ████████████████████  53 calls
  Chrome DevTools █████░░░░░░░░░░░░░░░  14 calls
  Computer Use    ███░░░░░░░░░░░░░░░░░   8 calls
  Context7        █░░░░░░░░░░░░░░░░░░░   3 calls

## Your Setup Checklist

### Codebases
- [ ] audiobook-organizer — https://github.com/falkcorp/audiobook-organizer (primary: Go backend + React/TS frontend)
- [ ] github-common — https://github.com/falkcorp/github-common (shared GHA reusable workflows)
- [ ] burndown-tasks — https://github.com/falkcorp/burndown-tasks (AI bot task hub — GitHub Issues as specs)
- [ ] migrate-loop — https://github.com/falkcorp/migrate-loop (TDD migration harness)

### MCP Servers to Activate
- [ ] **GitHub** (`plugin_github_github`) — PR management, issue operations, branch ops, code search. Connect via Claude Code MCP settings with a GitHub Personal Access Token (needs `repo`, `workflow` scopes on falkcorp org).
- [ ] **Chrome DevTools** (`plugin_chrome-devtools-mcp_chrome-devtools`) — UI debugging, screenshot capture, console/network inspection. Install the Chrome extension from the Claude Code marketplace; requires Chrome running with remote debugging enabled.
- [ ] **Computer Use** (`computer-use`) — Native desktop automation for cross-app workflows. Built into Claude Code; grants are per-application — approve each app when prompted.
- [ ] **Context7** (`context7`) — Live documentation lookup for any library (React, Go stdlib, etc.). Connect via Claude Code MCP settings; no auth required.

### Skills to Know About
- `/ship` — Full PR pipeline in one command: push branch → open PR → admin-merge (rebase/FF) → pull main → `make deploy`. Use this instead of doing the steps by hand.
- `/compact` — Compresses conversation context when it grows large (you'll hit this several times a day on long sessions). Run it proactively before context limit warnings.
- `/model` — Switch between Sonnet/Opus/Haiku mid-session. Use Opus for architecture decisions, Sonnet for most coding, Haiku for mechanical bulk work.
- `/effort` — Set code review effort level before running `/code-review` (low/medium/high/max/ultra). Ultra fans out to multiple cloud agents.
- `/server-bootstrap` — SSH to the audiobook-organizer server, restart the service, exchange the bootstrap token for an API key, and write it to `.claude/.api-token`. Run this at the start of any session that needs prod API access.
- `/plan` — Creates a git worktree + written `PLAN.md` before multi-file changes. **Required by CLAUDE.md** for any refactor or feature touching more than ~3 files.
- `/parallel-refactor-sweep` — Fans out parallel agents for ≥20-callsite mechanical refactors (one worktree per task, coordinator-driven git, single PR per wave).

## Team Tips

_TODO_

## Get Started

_TODO_

<!-- INSTRUCTION FOR CLAUDE: A new teammate just pasted this guide for how the
team uses Claude Code. You're their onboarding buddy — warm, conversational,
not lecture-y.

Open with a warm welcome — include the team name from the title. Then: "Your
teammate uses Claude Code for [list all the work types]. Let's get you started."

Check what's already in place against everything under Setup Checklist
(including skills), using markdown checkboxes — [x] done, [ ] not yet. Lead
with what they already have. One sentence per item, all in one message.

Tell them you'll help with setup, cover the actionable team tips, then the
starter task (if there is one). Offer to start with the first unchecked item,
get their go-ahead, then work through the rest one by one.

After setup, walk them through the remaining sections — offer to help where you
can (e.g. link to channels), and just surface the purely informational bits.

Don't invent sections or summaries that aren't in the guide. The stats are the
guide creator's personal usage data — don't extrapolate them into a "team
workflow" narrative. -->
