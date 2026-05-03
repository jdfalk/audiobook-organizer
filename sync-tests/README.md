<!-- file: sync-tests/README.md -->
<!-- version: 1.0.0 -->
<!-- guid: e3a91b27-4f6c-4d8a-9e15-7b2c8d4f9a01 -->
<!-- last-edited: 2026-05-02 -->

# iTunes / Apple Devices Sync Test Harness

Reproducible test harness for the "Apple Devices fails at step 3 — determining
tracks to sync" bug. Generates a series of ITL files, each with one isolated
mutation applied to a verified-good baseline, then walks you through opening
each one in iTunes and Apple Devices on Windows. You record the outcome
(works / failed / won't open). The harness writes the results to a JSON file
so the failure pattern can be analyzed.

## Files

| Path                              | Purpose                                                                    |
| --------------------------------- | -------------------------------------------------------------------------- |
| `scripts/Generate-Suite.ps1`      | Wrapper around `cmd/itunes-sync-tests`. Builds the binary and runs it.     |
| `scripts/Run-Tests.ps1`           | Interactive Windows runner. Opens each variant; prompts works/fail/wont.   |
| `scripts/Restore-Baseline.ps1`    | One-click safety net: copies the baseline back to the iTunes library path. |
| `scripts/Analyze-Results.ps1`     | Groups `results.json` by mutation tag; flags suspects for sync-fail-step-3. |
| `manifest.example.json`           | Reference for the `index.json` the generator writes.                       |
| `results.example.json`            | Reference for the per-run output the runner writes.                        |

## Variant catalog

Defined in `internal/itunes/sync_diagnostic_tests.go::GenerateSyncDiagnosticSuite`.
About 30 variants today, grouped by hypothesis:

- **Baseline integrity** — untouched copy, round-trip, minus-1-track.
- **Adds** — single synthetic track, real-file location, deterministic PID, etc.
- **Genre / Kind** — missing, plural, empty, podcast.
- **Location encoding** — bare Windows path, `file://localhost/`, `file:///`.
- **Audio fields** — populated Size / TotalTime / BitRate / SampleRate.
- **Mutation stubs** — Media Kind byte, Bookmarkable flag, Date Added/Modified
  (these are documentation stubs awaiting mith-offset reverse-engineering).

Each variant directory contains:

- `iTunes Library.itl` — the variant.
- `info.json` — `{id, hypothesis, description, mutations[]}`.
- `README.md` — human-readable hypothesis writeup.

The output dir contains `index.json` listing every variant.

## Workflow

### 1. (Mac, in repo) Generate the suite

```bash
# Use the LIVE golden master as baseline. Treat as read-only.
go run ./cmd/itunes-sync-tests \
  -baseline "/path/to/iTunes Library.itl" \
  -out      "./sync-tests-out"
```

Or via PowerShell on Windows after building the binary:

```powershell
.\sync-tests\scripts\Generate-Suite.ps1 `
  -Baseline "C:\Users\you\Music\iTunes\iTunes Library.itl.golden" `
  -OutputDir "C:\sync-tests-out"
```

### 2. (Windows) Pre-flight

- **STOP** the audiobook-organizer service if it's running anywhere that can
  write to the iTunes library you're about to test against.
- Make a backup of your live library before starting:
  ```powershell
  Copy-Item "C:\Users\you\Music\iTunes\iTunes Library.itl" `
            "C:\Users\you\Music\iTunes\iTunes Library.itl.preflight-bak"
  ```
- Close iTunes and Apple Devices.

### 3. (Windows) Run the harness

```powershell
.\sync-tests\scripts\Run-Tests.ps1 `
  -SuiteDir       "C:\sync-tests-out" `
  -ITunesLibPath  "C:\Users\you\Music\iTunes\iTunes Library.itl" `
  -ResultsPath    "C:\sync-tests-out\results.json" `
  -ITunesExe      "C:\Program Files\iTunes\iTunes.exe" `
  -AppleDevicesExe "C:\Program Files\WindowsApps\AppleInc.AppleDevices_*\AppleDevices.exe"
```

For each variant the runner will:

1. Show the hypothesis + description from `info.json`.
2. Copy the variant ITL to `ITunesLibPath`.
3. Open iTunes. Prompt you: "Did iTunes open the library?" (yes / no / crash).
4. Open Apple Devices. Plug in your iPhone if not connected. Try to sync.
   Prompt you: "What happened?" (sync-ok / sync-fail-step-3 /
   sync-fail-other / wont-open / crash).
5. Append the result to `results.json` immediately (so partial runs are safe).
6. Restore the original library before moving on (configurable; default is on).

You can resume an interrupted run — the runner skips variants already in the
results file. Pass `-Resume:$false` to force re-test.

### 4. Analyze

The results file is a JSON array of:

```jsonc
{
  "variant_id":   "18-aborg-add-1-no-genre",
  "hypothesis":   "Apple Devices uses Genre to decide audiobook membership; ...",
  "started_at":   "2026-05-02T20:14:33-04:00",
  "finished_at":  "2026-05-02T20:15:02-04:00",
  "itunes":       "ok" | "wont-open" | "crash",
  "apple_devices":"sync-ok" | "sync-fail-step-3" | "sync-fail-other" | "wont-open" | "crash",
  "notes":        "free-text user notes"
}
```

Cross-reference variant IDs against the catalog (or `info.json` per variant)
to map outcomes to mutations.

Or just run the analyzer:

```powershell
.\sync-tests\scripts\Analyze-Results.ps1 `
  -ResultsPath C:\sync-tests-out\results.json `
  -SuiteDir    C:\sync-tests-out
```

It prints a per-variant table, a per-mutation-tag outcome breakdown,
and a "suspects" list ranked by step-3 failure rate. Pass
`-OutputJson C:\sync-tests-out\analysis.json` to also dump structured
output for sharing back.

## Safety

- Always work from a known-good baseline backup. The included
  `Restore-Baseline.ps1` is a one-liner you can keep handy.
- The runner writes `results.json` after each variant, so a power loss or
  crash never destroys results from earlier variants.
- The runner refuses to start unless the baseline backup file you point it
  at exists. It will not let you orphan your live library.
