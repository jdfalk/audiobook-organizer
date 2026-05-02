# Code Quality Cleanup Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Fix remaining code quality issues: remove unused test variables, standardize error handling patterns, and replace `interface{}` with `any` for Go 1.25 compatibility.

**Architecture:** Three independent cleanup tasks: (1) Fix unused test variables, (2) Standardize test error handling, (3) Modernize type keywords. Each task is isolated and can be executed independently.

**Tech Stack:** Go 1.25.0, testing framework, json package

---

## Task 1: Remove Unused Test Variables in server_test.go

**Files:**
- Modify: `internal/server/server_test.go:219,874-880`

**Step 1: Fix unused otherFile variable at line 219**

The variable `otherFile` is created but never used. Search the test to confirm it's not referenced elsewhere.

Current code at line 219:
```go
otherFile := filepath.Join(tempDir, "other.m4b")
require.NoError(t, os.WriteFile(otherFile, []byte("audio"), 0o644))
```

Remove the entire line 219 and the corresponding WriteFile call at line 221. The test creates `tempFile` which is sufficient for the test.

Replace lines 219-221:
```go
tempFile := filepath.Join(tempDir, "book.m4b")
otherFile := filepath.Join(tempDir, "other.m4b")
require.NoError(t, os.WriteFile(tempFile, []byte("audio"), 0o644))
require.NoError(t, os.WriteFile(otherFile, []byte("audio"), 0o644))
```

With:
```go
tempFile := filepath.Join(tempDir, "book.m4b")
require.NoError(t, os.WriteFile(tempFile, []byte("audio"), 0o644))
```

**Step 2: Fix unused start and maxDuration variables at lines 874-880**

The test has incomplete timing measurement code:

Current code at lines 874-880:
```go
start := httptest.NewRecorder()
server.router.ServeHTTP(w, req)

// Note: In actual test, we'd measure time properly
// This is a placeholder to show the pattern
_ = start
_ = maxDuration
```

The `start` variable is incorrectly assigned a `*httptest.ResponseRecorder` (should be time.Time). The `maxDuration` variable is undefined. Remove these unused lines:

Replace lines 874-880:
```go
		start := httptest.NewRecorder()
		server.router.ServeHTTP(w, req)

		// Note: In actual test, we'd measure time properly
		// This is a placeholder to show the pattern
		_ = start
		_ = maxDuration
```

With:
```go
		server.router.ServeHTTP(w, req)
```

**Step 3: Run tests to verify nothing broke**

Run: `make test`
Expected: All tests pass, no failures

**Step 4: Commit**

```bash
git add internal/server/server_test.go
git commit -m "test: remove unused variables in endpoint tests"
```

---

## Task 2: Standardize Test Error Handling Patterns

**Files:**
- Modify: `internal/server/server_test.go:201,334,525,573,664,1140,647,650`
- Modify: `internal/server/server_more_test.go:328,332`

**Step 1: Replace ignored json.Marshal errors at line 201**

Current code at line 201:
```go
body, _ := json.Marshal(updateData)
```

Replace with:
```go
body, err := json.Marshal(updateData)
require.NoError(t, err)
```

**Step 2: Replace ignored json.Marshal errors at line 334**

Current code at line 334:
```go
body, _ := json.Marshal(batchData)
```

Replace with:
```go
body, err := json.Marshal(batchData)
require.NoError(t, err)
```

**Step 3: Replace ignored json.Marshal errors in table-driven tests**

Lines 525 and 573 are in a loop. Current code:
```go
body, _ := json.Marshal(tt.requestBody)
```

Replace with:
```go
body, err := json.Marshal(tt.requestBody)
require.NoError(t, err, "failed to marshal request body for test case %s", tt.name)
```

(The `tt.name` is available in the loop context)

**Step 4: Replace ignored json.Marshal errors at line 664**

Current code at line 664:
```go
body, _ := json.Marshal(payload)
```

Replace with:
```go
body, err := json.Marshal(payload)
require.NoError(t, err)
```

**Step 5: Replace ignored json.Marshal errors at line 1140**

Current code at line 1140:
```go
body, _ := json.Marshal(requestBody)
```

Replace with:
```go
body, err := json.Marshal(requestBody)
require.NoError(t, err, "failed to marshal request body for test case %s", tt.name)
```

**Step 6: Replace ignored write errors at lines 647, 650**

Current code at lines 647, 650:
```go
_, _ = w.Write([]byte(`{"numFound":1,...}`))
_, _ = w.Write([]byte(`{"numFound":0,...}`))
```

Replace with:
```go
_, err := w.Write([]byte(`{"numFound":1,...}`))
require.NoError(t, err)
_, err = w.Write([]byte(`{"numFound":0,...}`))
require.NoError(t, err)
```

**Step 7: Fix double-blank assignments in server_more_test.go at lines 328, 332**

Current code at line 328:
```go
_, _ = database.GlobalStore.CreateBook(...)
```

Replace with:
```go
_, err := database.GlobalStore.CreateBook(...)
require.NoError(t, err)
```

Current code at line 332:
```go
_, _ = database.GlobalStore.CreateImportPath(...)
```

Replace with:
```go
_, err := database.GlobalStore.CreateImportPath(...)
require.NoError(t, err)
```

**Step 8: Run tests to verify nothing broke**

Run: `make test`
Expected: All tests pass, no failures. If tests were failing before and caught by error checking, fix the underlying issues in the test setup.

**Step 9: Commit**

```bash
git add internal/server/server_test.go internal/server/server_more_test.go
git commit -m "test: standardize error handling patterns in all tests"
```

---

## Task 3: Replace interface{} with any Keyword

**Files:**
- Modify: `internal/server/server.go` (10+ occurrences)
- Modify: `internal/server/audiobook_service.go` (multiple occurrences)
- Modify: `internal/server/batch_service.go` (1 occurrence)
- Modify: `internal/server/server_test.go` (40+ occurrences)
- Modify: `internal/server/server_handlers_test.go` (7 occurrences)

**Step 1: Replace interface{} in metadataFieldState struct**

File: `internal/server/server.go`, lines 76-77

Current code:
```go
type metadataFieldState struct {
	FetchedValue   interface{} `json:"fetched_value,omitempty"`
	OverrideValue  interface{} `json:"override_value,omitempty"`
	OverrideLocked bool        `json:"override_locked"`
	UpdatedAt      time.Time   `json:"updated_at,omitempty"`
}
```

Replace with:
```go
type metadataFieldState struct {
	FetchedValue   any `json:"fetched_value,omitempty"`
	OverrideValue  any `json:"override_value,omitempty"`
	OverrideLocked bool        `json:"override_locked"`
	UpdatedAt      time.Time   `json:"updated_at,omitempty"`
}
```

**Step 2: Replace function return types and parameters in server.go**

Line 96 - decodeMetadataValue:
```go
func decodeMetadataValue(raw *string) interface{} {
```
Replace with:
```go
func decodeMetadataValue(raw *string) any {
```

Line 100 - variable declaration:
```go
var value interface{}
```
Replace with:
```go
var value any
```

Line 107 - encodeMetadataValue:
```go
func encodeMetadataValue(value interface{}) (*string, error) {
```
Replace with:
```go
func encodeMetadataValue(value any) (*string, error) {
```

Line 224 - decodeRawValue:
```go
func decodeRawValue(raw json.RawMessage) interface{} {
```
Replace with:
```go
func decodeRawValue(raw json.RawMessage) any {
```

Line 228 - variable declaration:
```go
var value interface{}
```
Replace with:
```go
var value any
```

Line 235 - updateFetchedMetadataState:
```go
func updateFetchedMetadataState(bookID string, values map[string]interface{}) error {
```
Replace with:
```go
func updateFetchedMetadataState(bookID string, values map[string]any) error {
```

Line 252 - stringVal:
```go
func stringVal(p *string) interface{} {
```
Replace with:
```go
func stringVal(p *string) any {
```

Line 259 - intVal:
```go
func intVal(p *int) interface{} {
```
Replace with:
```go
func intVal(p *int) any {
```

Line 295 - buildMetadataProvenance (in function):
```go
addEntry := func(field string, fileValue interface{}, storedValue interface{}) {
```
Replace with:
```go
addEntry := func(field string, fileValue any, storedValue any) {
```

Line 298 - variable in buildMetadataProvenance:
```go
var effectiveValue interface{}
```
Replace with:
```go
var effectiveValue any
```

Line 345 - stringFromSeries:
```go
func stringFromSeries(series *database.Series) interface{} {
```
Replace with:
```go
func stringFromSeries(series *database.Series) any {
```

Lines 630, 675 - map[string]interface{} usage (realtime notifications):
```go
Data: map[string]interface{}{
```
Replace with:
```go
Data: map[string]any{
```

Line 1110 - payloadMap:
```go
var payloadMap map[string]interface{}
```
Replace with:
```go
var payloadMap map[string]any
```

Line 1179 - type assertion:
```go
if overridesMap, ok := payloadMap["overrides"].(map[string]interface{}); ok {
```
Replace with:
```go
if overridesMap, ok := payloadMap["overrides"].(map[string]any); ok {
```

Line 1182 - nested type assertion:
```go
if vm, ok := v.(map[string]interface{}); ok {
```
Replace with:
```go
if vm, ok := v.(map[string]any); ok {
```

Line 1206 - slice of interface{}:
```go
if unlockOverridesRaw, ok := payloadMap["unlock_overrides"].([]interface{}); ok {
```
Replace with:
```go
if unlockOverridesRaw, ok := payloadMap["unlock_overrides"].([]any); ok {
```

Line 2333 - updates map:
```go
var updates map[string]interface{}
```
Replace with:
```go
var updates map[string]any
```

**Step 3: Replace interface{} in audiobook_service.go**

Replace all occurrences following the same pattern as above:
- Function parameters: `interface{}` → `any`
- Function return types: `interface{}` → `any`
- Variable declarations: `interface{}` → `any`
- Map types: `map[string]interface{}` → `map[string]any`
- Slice types: `[]interface{}` → `[]any`

**Step 4: Replace interface{} in batch_service.go at line 21**

Current code:
```go
Updates map[string]interface{} `json:"updates"`
```
Replace with:
```go
Updates map[string]any `json:"updates"`
```

**Step 5: Replace interface{} in test files**

In `server_test.go` and `server_handlers_test.go`, apply the same replacements for all `interface{}` occurrences. This affects approximately 50+ lines across both files.

Examples:
- `map[string]interface{}` → `map[string]any`
- `[]interface{}` → `[]any`
- Function signatures with `interface{}` parameters

Use Find & Replace to systematically replace all occurrences.

**Step 6: Run tests to verify nothing broke**

Run: `make test`
Expected: All tests pass, no failures

**Step 7: Run build to verify compilation**

Run: `make build-api`
Expected: Build succeeds with no errors

**Step 8: Commit**

```bash
git add internal/server/server.go internal/server/audiobook_service.go internal/server/batch_service.go internal/server/server_test.go internal/server/server_handlers_test.go
git commit -m "refactor: replace interface{} with any for Go 1.25 compatibility"
```

---

## Verification

After all tasks complete:

1. **No Compiler Errors**: `make build-api` should succeed
2. **All Tests Pass**: `make test` should show all tests passing
3. **Code Quality**: The codebase should be cleaner and more idiomatic Go
4. **Consistency**: Error handling patterns should be uniform across test files

---

## Notes

- The `interface{}` to `any` replacement is a Go 1.18+ feature and improves code readability
- Your codebase specifies `go 1.25.0` in go.mod, so `any` is fully supported
- Test error handling standardization prevents silent test failures
- All changes maintain backward compatibility - this is purely refactoring
