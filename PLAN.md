# Structured Logging Migration – Wave 11 (Format String Fix)

**Branch:** `feat/slog-w11`
**Scope:** Fix all format string issues in slog calls (674 calls across 119 files)
**Status:** Planning (awaiting approval)

## Goal

Fix the format string corruption introduced in W10. The W10 conversion was mechanical (just swapping `log.Printf` → `slog.Info`), but didn't account for slog's requirement for structured key-value pairs instead of printf-style format strings.

Production logs now show: `msg="Startup task %s started: operation %s" !BADKEY=taskID !BADKEY=opID`

This wave converts all format strings to proper key-value format.

## Problem Statement

**Current State (W10 output):**
```go
slog.Info("Startup task %s started: operation %s", taskID, opID)
// Produces: msg="Startup task %s started: operation %s" !BADKEY=taskID !BADKEY=opID
```

**Desired State (W11 output):**
```go
slog.Info("Startup task started", "task", taskID, "op", opID)
// Produces: msg="Startup task started" task=taskID op=opID
```

## Scope

- **674 slog calls** across 119 files with format strings (`%s`, `%d`, `%v`, `%f`)
- Affects: internal/, cmd/ packages
- Pattern categories:
  1. Single format string arg: `slog.Info("msg: %s", val)` → `slog.Info("msg", "key", val)`
  2. Multiple format string args: `slog.Info("msg %s %d", val1, val2)` → `slog.Info("msg", "key1", val1, "key2", val2)`
  3. Mixed (format string + KV): `slog.Error("msg %v", val, "existing", "kv")` → needs careful parsing

## Conversion Strategy

### Approach

Build a Go program (`w11-fix-format-strings.go`) that:

1. **Identifies problematic patterns** via regex matching `slog\.(Info|Warn|Error|Debug)\([^)]*%[sdvf]`
2. **Extracts components**:
   - Method name (Info, Warn, Error, Debug)
   - Message string with format specifiers
   - Format args (the positional values after the message)
   - Any trailing key-value pairs
3. **Generates key names** automatically (auto-increment: key0, key1, key2 or smarter naming based on context)
4. **Reconstructs the call** with structured format

### Rules for Key Naming

When format string has positional args, generate keys intelligently:
- If a variable name is available (e.g., `taskID`), use it: `"task", taskID`
- If a function call (e.g., `len(items)`), use generic: `"count", len(items)`
- If a complex expression, use generic: `"value0"`, `"value1"`, etc.

Examples:
```go
// Input: slog.Info("file %s processed", filepath)
// Output: slog.Info("file processed", "filepath", filepath)

// Input: slog.Info("total: %d bytes, %d items", totalBytes, itemCount)
// Output: slog.Info("total", "totalBytes", totalBytes, "itemCount", itemCount)

// Input: slog.Error("error: %v", err)
// Output: slog.Error("error", "err", err)
```

## Execution Steps

1. **Build converter** — write `w11-fix-format-strings.go`
   - Parse slog calls with format strings
   - Extract method, message, args
   - Generate key names
   - Output fixed calls

2. **Test converter** on a single file first (e.g., `settings.go`)

3. **Apply to all 119 files**
   - Run converter on all internal/cmd files
   - Manual review of high-risk files (database, itunes, metadata)

4. **Verify** 
   ```bash
   grep -rn 'slog\.\(Info\|Warn\|Error\|Debug\).*%[sdvf]' internal cmd
   # Should return 0 results
   ```

5. **Run tests**
   ```bash
   make ci
   ```

6. **Commit**
   ```bash
   git commit -m "fix(slog): W11 format string to key-value conversion (674 calls)"
   ```

7. **Ship**
   ```bash
   /ship
   ```

## Risk Assessment

**Low Risk**
- All changes are mechanical (format string → KV transformation)
- No behavioral changes (just log output format)
- Easy to verify: grep for `%[sdvf]` patterns post-conversion

## Timeline Estimate

- **Converter development:** ~30 min
- **Single file test:** ~5 min
- **Batch apply:** ~10 min (automated)
- **Testing:** ~15 min (make ci)
- **Ship:** ~5 min (/ship)
- **Total:** ~1.5 hours

## Rollback Plan

If issues arise post-commit:
- Revert commit: `git revert <commit-hash>`
- Rollback via `/ship` (auto-merge revert)

---

**Approval Required:** Proceed with W11 conversion? [y/n]
