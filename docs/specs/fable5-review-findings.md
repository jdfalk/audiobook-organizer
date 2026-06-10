<!-- file: docs/specs/fable5-review-findings.md -->
<!-- version: 1.0.0 -->
<!-- guid: 3f2b8c1d-9e4a-4f6b-8a2c-5d7e9f0a1b2c -->

# FINDINGS: Security & Bugs — Fable 5 Full-System Review (2026-06-09)

Classification: SECURITY / BUG / PERFORMANCE / DEBT. Severity: CRITICAL / HIGH / MEDIUM / LOW.

Every iTunes finding below is grounded in empirical analysis of the six libraries in
`/tmp/itunes-libraries/` (golden 30.8MB / writeback 1.0MB / damaged-1..4 at 29.0–29.7MB),
performed with `cmd/itl-check`, `cmd/itl-diff`, `cmd/itl-repair`, and direct binary
inspection of the decrypted/inflated payloads. Evidence provenance is tagged:

- **OBSERVED** — seen directly in the damaged-file evidence, with file + counts cited
- **VERIFIED** — confirmed by reading/running the current code in this repo
- **REPORTED** — surfaced by a read-only review agent and spot-checked where load-bearing

Baseline facts (OBSERVED, all six libraries):

| Library | Tracks (header @0x44) | Tracks (mlth/mith) | Playlists | Dangling mtph | iTunes verdict |
|---|---|---|---|---|---|
| golden `iTunes Library.itl` | 94,575 | 94,575 | 338 | 0 | opens fine |
| `writeback-iTunes Library.itl` | 374 | 374 | 14 | 0 | (test lib) |
| damaged-1 | **90,900** | **90,898** | 335 | **6** (3 playlists) | Damaged |
| damaged-2 | **90,900** | **90,898** | 335 | 0 | Damaged |
| damaged-3 | 90,900 | 90,900 | 335 | 0 | Damaged |
| damaged-4 | 90,863 | 90,863 | 335 | 0 | Damaged |

Key implication: **the current safety verifier (`VerifyITLNoDanglingRefsLE`) passes 3 of the
4 libraries that iTunes itself rejected as damaged.** The dangling-mtph class is real but it
is the *minority* corruption class in the evidence. damaged-2 is byte-consistent with
"damaged-1 minus its 6 dangling mtph items" (Δ = 504 bytes = 6 × 84-byte mtph) — i.e. a
dangling-ref-repaired library is still iTunes-rejected.

---

## CRITICAL

### CRIT-1 — Writeback emits mhoh encoding-flag bytes iTunes never produces (BUG, iTunes corruption, LIVE)

**OBSERVED.** In the golden library, every one of its 281,790 string `mhoh` blocks sampled
across types 0x02 (Name), 0x0B (LocalURL), 0x0D (Location) has byte `+27` (what our code
calls the "encoding flag") equal to `0x00`. The damaged libraries contain large numbers of
blocks with `+27 ∈ {1, 3}` — values that only our writer produces:

| Library | flag=3 location blocks (0x0B+0x0D) | flag=3/1 name blocks (0x02) |
|---|---|---|
| golden | 0 | 0 |
| damaged-1 | 167,562 | 81,195 (flag 3) + 2,586 (flag 1) |
| damaged-3 | 167,566 | 5,770 (flag 3) + 5 (flag 1) |
| damaged-4 | 68 | 34 |

**VERIFIED root cause:** `encodeHohmString` (`internal/itunes/itl.go:373`) returns flag 3
(Windows-1252) for Latin strings and flag 1 (UTF-16BE) otherwise. Both `buildMhohLE`
(`internal/itunes/itl_le_mutate.go:272`) and `rewriteHohmLocationLE`
(`internal/itunes/itl_le.go:656`) stamp that flag into byte `+27`. iTunes appears to encode
strings via a field at byte `+24` (golden shows values 1/2/3 there with `+27` always 0) —
our `+27` semantics are an invention of our parser, faithfully round-tripped by our own
tools but foreign to iTunes/Apple Devices.

**Impact:** every metadata/location writeback stamps tens of thousands of blocks with byte
patterns real iTunes never writes. damaged-4 was rejected by iTunes with only ~34 such
blocks — tolerance is near-zero. This is the prime suspect for the Apple Devices crash
class. Fix direction: byte-level corpus study of golden (how iTunes really encodes
non-ASCII), then make the writer emit byte-identical encodings, with a write-guard that
rejects any block whose `+24..+27` pattern doesn't occur in iTunes-authored libraries.
Full treatment: SPEC 2 (`fable5-spec-itunes-writeback-hardening.md`).

### CRIT-2 — Location (0x0D) written as `file://` URL; iTunes stores a native Windows path (BUG, iTunes corruption, LIVE)

**OBSERVED.** Golden: all 93,014 type-0x0D blocks contain Windows paths
(`W:\itunes\iTunes Media\Audiobooks\...`); the URL form lives only in type-0x0B
(`file://localhost/W:/itunes/iTunes%20Media/...`). damaged-1/3 contain 83,783 type-0x0D
blocks holding **URLs** (`file://localhost/W:/audiobook-organizer/...`); damaged-4 has 34.
damaged-4 also shows locations pointing into our staging dir
(`.../audiobook-organizer/.itunes-writeback/iTunes%20Media/...`).

**VERIFIED root cause:** the location-update path (`internal/itunes/itl_le.go:640-654`)
adds a `file://localhost/` prefix only for 0x0B and writes the caller's value into 0x0D
verbatim; callers pass `f.ITunesPath` / URL-shaped values
(`internal/itunes/service/writeback_batcher.go:341-345`, `UpdateMetadataLE` `Location`
field). There is no "0x0D must be a Windows path, 0x0B must be a percent-escaped URL"
normalization or guard anywhere.

### CRIT-3 — `hdfm` header count fields never updated by mutations (BUG, iTunes corruption, LIVE)

**OBSERVED.** The unencrypted `hdfm` header carries BE count fields at 0x44 (tracks), 0x48
(playlists), 0x4C (albums = `miah` count), 0x54 (artists = `miih` count) — verified by exact
match against payload counts in golden, damaged-3, damaged-4. In damaged-1 and damaged-2
the header says 90,900 tracks while the payload (`mlth` count and actual `mith` blocks) has
90,898 — a desync of exactly the 2 tracks our `RemoveTracksByPIDLE` removed after iTunes
last saved.

**VERIFIED root cause:** `RemoveTracksByPIDLE` (`internal/itunes/itl_le_remove_by_pid.go`)
updates `mlth` count, `miph` counts, and msdh totalLens, but no code path touches the
header remainder (grep for `hdfm|remainder|0x44` in the mutate/remove files: zero hits).
`buildHdfmHeader` (`internal/itunes/itl.go:475`) reuses the stale `headerRemainder`
verbatim on every write. Also note `itl-repair` does not fix this class: damaged-2 (the
repaired twin of damaged-1) still carries the desync and is still iTunes-rejected.

---

## HIGH

### HIGH-1 — Safety contract has blind spots covering 3 of 4 observed damaged libraries (BUG, iTunes safety)

**OBSERVED + VERIFIED.** The only structural guard run on writeback is the dangling-mtph
check (`VerifyITLNoNewDanglingRefsLE` / `VerifyITLNoDanglingRefsLE`,
`internal/itunes/itl_le_verify.go`). Running the full detector over the evidence: damaged-2,
-3, -4 all report **zero** dangling refs and pass, yet all were renamed "(Damaged)" by
iTunes. Missing guards include: mhoh format validation (headerLen==24, valid `+24..+27`
encoding patterns, totalLen == 40+strLen), header-vs-payload count agreement, 0x0D
path-form validation, and 0x0B URL-escape validation. Additionally both verifiers
**fail open**: if `CollectMasterTrackIDsLE` returns nil (unparseable master list), they
return nil ("don't fail-closed on parse surprises", itl_le_verify.go:80-85) — precisely
when the library is most damaged. Full guard inventory: SPEC 2.

### HIGH-2 — LE parser never reads track string metadata; all string-field diagnostics are vacuous (BUG)

**VERIFIED empirically.** `walkMsdhTracksLE` (`internal/itunes/itl_le.go:49-91`) advances
by the `mith` chunk's totalLen — which *includes* its child `mhoh` blocks — so the `case
"mhoh"` branch is unreachable for track metadata. Every `ITLTrack` string field (Name,
Album, Artist, Genre, Kind, Location, LocalURL) is empty after parse. Confirmed:
`itl-diff -v` prints `""  by ""` for every track in the golden library; `itl-check` reports
"Tracks with Location: 0" on a library with 93,014 location blocks. Consequence:
`itl-diff`'s "Tracks changed: 0" between golden and damaged was **vacuous** for all string
fields — the very fields our writeback rewrites. Any production logic reading
`ITLTrack.Location` (path repair, import mapping) sees empty strings on LE libraries.

### HIGH-3 — Writeback unconditionally rewrites metadata for every mapped track (BUG/PERFORMANCE, corruption amplifier)

**VERIFIED + OBSERVED.** `writeback_batcher.go:346` ("Always push metadata so iTunes has
current values") appends an `ITLMetadataUpdate` for every book file with an iTunes PID on
every sync — there is no changed-value check. The observed result in the evidence:
damaged-1 has ~81K rewritten Name blocks and ~167K rewritten location blocks. Combined
with CRIT-1/2, every full sync re-stamps nearly the whole library with non-iTunes byte
patterns; blast radius is total instead of incremental. Diff-before-write is the single
highest-leverage hardening change after the encoders are fixed.

### HIGH-4 — `POST /api/v1/auth/accept-invite` returns `{"error":"EOF"}` under HTTP/2 (SECURITY/BUG, pen-test June 4 2026)

**VERIFIED handler code.** `internal/server/auth_accept_invite.go:28` feeds
`c.ShouldBindJSON(&req)` errors straight into the 400 response; an empty/streamed HTTP/2
body yields the raw Go `EOF` error string. (Contrast: `internal/server/fingerprint_rescan.go:45`
already tolerates EOF for optional bodies — but accept-invite's body is *required*, so the
right fix is mapping EOF/empty-body to a clear "request body required" message, not
ignoring it.) Root cause of the pen-test finding is error-message passthrough, not a
binding failure. REPORTED middleware context: `request_size.go` only pre-rejects on
`ContentLength > 0`, so chunked/HTTP/2 bodies skip the early 413 and surface as
EOF-flavored errors from `MaxBytesReader` (see MED-1).

### HIGH-5 — Dedup candidates carry no fingerprint provenance; stale 100%-similarity rows survive recompute (BUG, dedup correctness)

**VERIFIED.** `DedupCandidate` (`internal/database/embedding_store.go:79-91`) has
`Layer`/`Similarity` but no record of *which fingerprint version/algorithm* produced the
match. `PurgeStaleCandidates` (`internal/dedup/engine.go:1521-1649`) prunes for
missing/non-primary/version-group/series reasons only — nothing invalidates candidates when
fingerprints are recomputed. Production carries ~14K 100%-matches created from the old
per-segment fingerprints (count from project records; not re-verified against prod in this
review). Whole-file fields exist (`store.go:676-684`, segments deprecated) but no
`fingerprint_version` marker exists anywhere (grep negative). Fix: provenance fields +
purge-by-provenance migration (SPEC 1, tasks in plan).

### HIGH-6 — `headerLen=totalLen` mhoh corruption: fixed at the writer, but no detector for already-corrupt libraries (BUG, partially mitigated)

**OBSERVED.** damaged-1 contains ~60K type-0x02 blocks with headerLen 41–210 (legal value
is 24 — golden is 100% headerLen=24); damaged-3 has ~3.4K, damaged-4 ~33. **VERIFIED**
that the cause (`buildMhohLE` writing headerLen=totalLen) is already fixed
(`internal/itunes/itl_le_mutate.go:266-276`, `mhohFixedHeaderLen = 24`, regression test
`TestRewriteHohmLocationLE_PreservesHeaderLen`). Remaining gap: nothing *detects* such
blocks at read/verify time, so a library corrupted by an old version (or any external
cause) sails through every current guard and gets mutated/re-shipped.

---

## MEDIUM

### MED-1 — Request-size middleware early-413 bypass for chunked/HTTP/2 bodies (SECURITY, hardening)

**REPORTED, spot-checked.** `internal/server/middleware/request_size.go:52-58` pre-rejects
only when `ContentLength > limit && ContentLength > 0`; bodies without Content-Length fall
through to `http.MaxBytesReader`, which truncates with an opaque error instead of a clean
413. Safe (no OOM) but produces the EOF-style failure surface seen in HIGH-4.

### MED-2 — `Book.Duration` / `Book.FileSize` are snapshots, not aggregates of BookFiles (BUG/DEBT)

**REPORTED, consistent with grep.** Fields at `internal/database/store.go:128,170` are set
at import and never recomputed from `BookFile` rows; no sum-over-files path exists. Known
backlog item (project memory `duration_filesize_aggregation`); UI shows stale snapshots for
multi-file books.

### MED-3 — `FilterUnchangedTags` treats custom `AUDIOBOOK_ORGANIZER_*` fields as always-changed (BUG)

**REPORTED.** `internal/metafetch/service_writeback.go:353-423` compares a fixed set of
standard fields; unknown keys fall through to "write". Effect: every write-back rewrites
all custom tags even when unchanged — defeats the skip-detection the function exists for,
and inflates copy-on-write `.bak-*` churn.

### MED-4 — Legacy SQLite store (~7.9K lines) still compiled and opened at startup (DEBT)

**VERIFIED.** `internal/database/database.go:20` does `sql.Open("sqlite3", databasePath)`
unconditionally; `sqlite_store_*.go` totals ~7,938 lines implementing a parallel Store that
production (PebbleDB-primary) shouldn't reach. Risk: drift, accidental use (e.g.
`sqlite_store_books.go:872` implements `GetDuplicateBooks` alongside the Pebble one),
single-writer lock if any path lands there. Note the review brief's claim "embeddings.db
SQLite" is stale: embeddings live in PebbleDB (`emb:v:*` keys,
`internal/database/embedding_store.go`). Removal plan: SPEC 3.

### MED-5 — Diagnostic tools claim more than they do (DEBT, tooling trust)

**VERIFIED.** `cmd/itl-diff/main.go` docstring promises an msdh container inventory that is
not implemented; it diffs only the 96-byte header hex and per-track parsed fields (which
HIGH-2 makes vacuous for strings). `cmd/itl-check` prints counts only. Neither inspects
playlist membership. During an actual corruption incident these tools said "0 changed" on
libraries with 167K rewritten blocks.

### MED-6 — `ValidateITL` validates almost nothing (BUG, iTunes safety)

**VERIFIED.** `internal/itunes/itl.go:626` checks header decrypt + non-zero track count
only. It is not a structural validator and must not be treated as one by callers
(`library_watcher.go`, service validate paths). Superseded by the SPEC 2 safety contract.

### MED-7 — Oversized-payload inflate fails *silently* and verifiers fail open — a bad composition (BUG, iTunes safety)

**VERIFIED.** `itlInflate` (`internal/itunes/itl.go:302-320`) returns `(data, false)` —
i.e. "treat as uncompressed" — when payload exceeds the 512MB decompression cap (golden
already inflates to 236MB; a 2.2× larger library trips this). Downstream, parse yields no
master list, and `VerifyITLNoDanglingRefsLE` returns nil when the master list can't be
located. Net effect: beyond ~500MB decompressed, every guard silently passes while parse
sees garbage. Fail-closed behavior is required on both ends.

### MED-8 — Chromem hydration is fire-and-forget with no shutdown join (BUG, concurrency, minor)

**REPORTED, lifecycle reviewed.** `internal/dedup/lifecycle.go:103-112` starts hydration
under `bgCtx` with a 30-min timeout but no WaitGroup; `Stop()` cancels correctly
(mutex-guarded, nil-safe — the PR #1239 pattern) but does not wait, so shutdown can race a
final Pebble read. Concurrency sweep found no other unsafe cancel patterns: registry
shutdown (`internal/operations/registry/registry.go:398-450`), acoustid heartbeat
(`internal/plugins/acoustid/fingerprint_rescan.go:142-175`), and warmup contexts all follow
the safe pattern.

---

## LOW

### LOW-1 — Zero-test packages (DEBT)

**REPORTED.** `internal/quarantine` (354 lines), `internal/httputil` (337), 
`internal/operations` (318) have no `_test.go` files. `go vet ./...` is clean. Candidates
for the existing test-coverage burndown queue (#79–#109) rather than new plan tasks.

### LOW-2 — BE-format writer lacks the LE safeguards (DEBT, iTunes)

**VERIFIED.** All verify functions are LE-only and silently no-op on BE payloads
(`detectLE` gate, `itl_le_verify.go:31,75`); `itl_be.go:528` shares the same
`encodeHohmString` flag problem as CRIT-1. Production libraries are LE (v12.13), so this is
exposure only if a PowerPC-era library is ever written. Recommend: refuse BE writeback
outright instead of writing unguarded.

---

## Defenses confirmed present (for balance)

- Path traversal: `internal/security/pathvalidation` (`CleanAbsolutePath`, `SecureJoin*`)
  used at user-facing path inputs; recent CodeQL sanitizer commits verified.
- Auth: bcrypt at default cost; session cookies HttpOnly+Secure+SameSite=Strict; API keys
  stored as SHA-256 hashes; per-IP and per-account login throttles; bootstrap token
  rate-limited, written 0600, never logged.
- SSRF: cover-art fetch restricted to an allowlist (openlibrary.org, books.google.com,
  amazon.com); other outbound HTTP targets fixed hosts.
- iTunes path mapping inputs come from the DB (not user-supplied) and the write target is
  the configured library path; no traversal vector found in `ReverseRemapPath`.
- Dedup engine background lifecycle (`bgMu`/`bgCtx`) follows the post-#1239 safe pattern.

## Severity totals

- **CRITICAL: 3** (CRIT-1 encoding flags, CRIT-2 0x0D URL-vs-path, CRIT-3 header count desync)
- **HIGH: 6** (verifier blind spots; vacuous LE string parse; unconditional metadata push; accept-invite EOF; candidate provenance; headerLen detector gap)
- **MEDIUM: 8** | **LOW: 2**
