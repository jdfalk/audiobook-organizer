<!-- file: docs/superpowers/bot-tasks/2026-04-27-itunes-tests-4-13c-validate.md -->
<!-- version: 1.0.0 -->
<!-- guid: ce7f9136-bc8d-4e20-ad3f-013aed7d8ab9 -->

# BOT TASK: 4.13c — Tests for internal/itunes/service/validate.go

**TODO ID:** 4.13c
**Companion human design:** [`docs/superpowers/specs/2026-04-27-itunes-test-suite-design.md`](../specs/2026-04-27-itunes-test-suite-design.md)
**Pattern reference:** [`4.13a`](2026-04-27-itunes-tests-4-13a-status.md) — read first.

## Branch

```
test/4-13c-itunes-validate
```

## Files

- **Read:** `internal/itunes/service/validate.go` (121 LOC)
- **Read:** `internal/itunes/service/validate_mock_test.go` (mocks exist)
- **Create:** `internal/itunes/service/validate_test.go`

## What this code does

Validation of iTunes inputs — likely things like:
- ITL file size / format sanity
- Path string sanity (no NUL bytes, length limits, illegal Windows chars on remote target)
- Track metadata consistency (title not empty, duration sane)
- Persistent-ID format

This is the easiest of the three zero-coverage files to test because validation functions are pure (no store, no network). Aim for **table-driven tests**.

## Required test pattern

For each exported validator function:

```go
func TestValidate<FunctionName>(t *testing.T) {
    cases := []struct{
        name    string
        input   <InputType>
        wantErr string  // substring; "" means no error
    }{
        {"happy_path",            valid,                   ""},
        {"empty_required_field",  withEmptyTitle,          "title"},
        {"too_long",              withOversize,            "exceeds"},
        {"illegal_char",          withNullByte,            "invalid"},
        // ... per-validator-specific cases
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            err := Validate<FunctionName>(tc.input)
            if tc.wantErr == "" {
                assert.NoError(t, err)
            } else {
                assert.ErrorContains(t, err, tc.wantErr)
            }
        })
    }
}
```

Aim for 8–12 cases per validator. Pure functions are cheap; cover thoroughly.

## Validator-specific edge cases

If the file validates **paths**: cover Windows reserved names (CON, PRN, AUX, NUL, COM1, LPT1), trailing dots, leading/trailing spaces, drive-letter mismatches.

If the file validates **persistent IDs**: cover empty, non-hex, wrong-length, mixed case (iTunes uses uppercase hex).

If the file validates **track metadata**: cover empty title, oversized title (>255 char on Windows), non-UTF8 bytes if input is `[]byte`.

## Step-by-step

Same as 4.13a Step 2–5.

## Definition of done

- [ ] Every exported function in `validate.go` has a table-driven test with ≥ 5 cases
- [ ] Edge cases listed above are covered where applicable
- [ ] Package coverage rises
- [ ] `make ci` green
- [ ] CHANGELOG prepended
- [ ] TODO.md `4.13c` flipped to `[x]`

## When to STOP

NEEDS_REVIEW if:

- A validator depends on store / network state. Pure validators expected; impure ones suggest the file is misnamed. Surface for human review.
