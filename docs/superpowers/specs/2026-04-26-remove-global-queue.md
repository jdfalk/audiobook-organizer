---
title: Remove GlobalQueue package global (migrate-loop dogfood #2)
slug: remove-global-queue
target_packages:
  - internal/operations
test_runner: "go test -race -json ./internal/operations/..."
---

# Remove GlobalQueue package global

## THE PLAN PHASE MUST ADD EXACTLY THIS TEST

Add this function verbatim to `internal/operations/queue_test.go` as the
**very first action** in the PLAN phase. Do not skip it. Do not paraphrase it.
Copy it word for word:

```go
func TestGlobalQueueMustBeGone(t *testing.T) {
	src, err := os.ReadFile("queue.go")
	if err != nil {
		t.Fatalf("cannot read queue.go: %v", err)
	}
	if bytes.Contains(src, []byte("var GlobalQueue")) {
		t.Fatal("GlobalQueue package var still exists — delete it and its three helper functions")
	}
}
```

Add the required imports (`"bytes"`, `"os"`) to the test file's import block if
not already present.

**This test MUST fail before the CODE phase starts.** Run `go test
./internal/operations/...` at the end of PLAN to verify it fails. If it
passes, the var is already gone and this migration is complete. If it fails
(expected), proceed to CODE.

## Success criteria

- `TestGlobalQueueMustBeGone` passes (i.e. `var GlobalQueue` is gone from queue.go)
- `var GlobalQueue Queue` is deleted from queue.go
- `func InitializeQueue(...)` is deleted
- `func ShutdownQueue(...)` is deleted
- `func SetGlobalOperationTimeout(...)` is deleted
- All tests in `./internal/operations/...` pass with `-race`

## CODE phase: what to delete

In `internal/operations/queue.go`, delete:
1. `var GlobalQueue Queue`
2. `func InitializeQueue(...)`
3. `func ShutdownQueue(...)`
4. `func SetGlobalOperationTimeout(...)`

Then fix any compile errors **within `internal/operations/` only** that result
from the deletion.

In `internal/operations/queue_test.go`, rewrite `TestGlobalQueueFunctions` to
not reference `GlobalQueue`, `InitializeQueue`, `ShutdownQueue`, or
`SetGlobalOperationTimeout`. Use `NewOperationQueue()` directly instead. The
test must still exercise: double-construction of independent queues, shutdown,
and SetOperationTimeout on an instance.

## Out of scope

Do NOT modify `main.go`, `cmd/root.go`, `internal/server/server.go`, or
`internal/testutil/integration.go`. Those callers will break at compile time —
that is expected and intentional. This migration only fixes the operations
package itself.
