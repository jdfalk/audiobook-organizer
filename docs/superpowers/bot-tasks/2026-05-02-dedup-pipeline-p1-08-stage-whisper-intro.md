<!-- file: docs/superpowers/bot-tasks/2026-05-02-dedup-pipeline-p1-08-stage-whisper-intro.md -->
<!-- version: 1.0.0 -->
<!-- guid: 6451979c-8c8b-431b-8546-8d01e36b517a -->
<!-- last-edited: 2026-05-02 -->

# BOT TASK: Pipeline stage: Whisper transcription of first 2 minutes

**Pipeline phase:** P1
**Audience:** burndown bot
**Master spec:** [`docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md`](../specs/2026-05-02-audiobook-identification-dedup-pipeline.md)

## Prerequisites

- `task:P0-04` — must be merged before this task starts

```bash
count=$(gh pr list --label "task:P0-04" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:P0-04"; exit 0; }
```


## Branch

```
feat/dedup-pipeline-p1-08-stage-whisper-intro
```

## Label

```bash
gh label create "task:PIPE-P1-08" --color "1d76db" --description "Bot task: Pipeline stage: Whisper transcription of first 2 minutes" 2>/dev/null || true
```

## What This Does

Extracts the first 120 s of audio with ffmpeg, sends to Whisper (OpenAI API by default; local whisper.cpp if `settings.whisper.local=true`), fuzzy-matches transcript against `"<title> by <author>"`. Only runs when identity_score from earlier stages is < 0.85 (gating happens in the coordinator P3-01; this stage assumes it's been called).

## Files to Create / Edit

1. **Create** `internal/maintenance/jobs/stage_whisper_intro.go`
2. **Create** `internal/maintenance/jobs/stage_whisper_intro_test.go`

## Implementation outline

- Use `ffmpeg -ss 0 -t 120 -i in -ar 16000 -ac 1 -c:a pcm_s16le out.wav`.
- Cache transcript on FingerprintStore (a new `whisper_intro_text` column may
  be added in a follow-up; for now stash inside `EvidenceJSON`).
- Match score: token-set ratio (use `github.com/agnivade/levenshtein` or the
  existing helper) of normalized transcript vs `"<title> by <author>"`.
- Score ≥ 0.55 → `KindWhisperIntroMatch`; score < 0.20 with
  duration ≥ 90s actually heard → `KindWhisperIntroNegative`.
- Failure: Whisper API down → no signal; coordinator caps identity_score at
  0.85 to surface "needs verification".


## Verify

```bash
go test ./internal/maintenance/jobs/... -run StageWhisperIntro
go vet ./internal/maintenance/jobs/...
```

## Definition of Done

- [ ] Job registers itself via `init()` (`maintenance.Register(&Job{})`)
- [ ] Job emits exactly one `signals.Signal` per `(book_id, kind, value)` it sees
- [ ] Failure mode (see master spec §3.4) is handled — never panics, never blocks the pipeline
- [ ] Test asserts: (a) no signal on empty input, (b) correct signal on happy path, (c) graceful no-op on missing dependency
- [ ] `make build-api` succeeds


## PR Instructions

```bash
gh pr create \
  --title "feat(dedup): pipeline stage: whisper transcription of first 2 minutes (PIPE-P1-08)" \
  --body "Implements PIPE-P1-08 from docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md. See task file for details." \
  --label "task:PIPE-P1-08"
```
