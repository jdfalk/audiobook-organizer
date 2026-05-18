# INFRA-OPENTELEMETRY — Add Observability Instrumentation

## Goal
Integrate OpenTelemetry (OTEL) across the audiobook-organizer backend to enable detailed observability: metrics (request rate, latency, error rate, library size, operation queue depth), distributed tracing (end-to-end operation spans), and structured logging. Enable identification of slow code paths, operation bottlenecks, DB query latency, HTTP handler latency, and external AI call costs.

---

## Affected Files

### New Files
- `internal/telemetry/telemetry.go` — OTEL init, meter/tracer setup, per-category span/metric helper functions
- `internal/telemetry/config.go` — OTEL configuration (endpoint, sampler, exporter selection from env)

### Modified Files
- `go.mod` — add `go.opentelemetry.io/otel`, SDK, GCP/Jaeger exporters
- `cmd/audiobook-organizer/main.go` — initialize OTEL on startup (otelStartup())
- `internal/server/server.go` — wire OTEL services into server init
- `internal/server/http.go` or Gin middleware — add `otelgin` middleware for HTTP layer (per-handler spans + metrics)
- `internal/database/pebble_store.go` — wrap store methods (Get, Put, Range, etc.) with spans + metric counters
- `internal/database/sqlite_store.go` — same (if still used in tests/legacy paths)
- `internal/operations/operation_registry.go` — wrap `op.Run(ctx)` with root span (op_id attribute)
- `internal/openai/openai.go` — wrap embed/parse/batch calls with child spans + token/cost metrics
- `internal/dedup/engine.go` — add spans to FullScan, CheckBook, PurgeStaleCandidates phases
- `.env` / server startup — expose `OTEL_EXPORTER_OTLP_ENDPOINT` env var (disabled by default, no-op when unset)
- `Makefile` — optional: add `run-with-otel` target for local Jaeger/Prometheus setup

---

## Steps

### Step 1: Add OTEL Dependencies and Init Package
1. Update `go.mod`: add `go.opentelemetry.io/otel/...` libraries (otel, SDK, OTLP exporter, Jaeger exporter, Prometheus exporter)
2. Create `internal/telemetry/telemetry.go` with:
   - `InitOTEL()` function that reads `OTEL_EXPORTER_OTLP_ENDPOINT` env var
   - Returns (shutdown func, error)
   - If endpoint is empty, return no-op shutdown
   - If endpoint is set, init OTEL exporter (OTLP gRPC), meter, and tracer
3. Create `internal/telemetry/config.go` with OTEL config struct and env var parsing

### Step 2: Wire OTEL into Server Startup
1. Modify `cmd/audiobook-organizer/main.go`:
   - Call `telemetry.InitOTEL()` before NewServer()
   - Defer shutdown (handle errors)
   - Log startup: "[INFO] OpenTelemetry enabled (endpoint: ...)" if active
2. Update `internal/server/server.go` to store global tracer/meter references for later use

### Step 3: HTTP Layer Instrumentation
1. Add `otelgin` middleware to the Gin router (wraps all HTTP handlers):
   - Creates span per handler with handler name + method + path as attributes
   - Records request count + latency histogram
2. Middleware should be added in `server.setupRouter()` or equivalent

### Step 4: Database Layer Instrumentation
1. Create helper functions in `internal/telemetry/db.go`:
   - `WrapStorageOp(ctx, opName, fn)` — executes fn with a span + counter
   - Examples: `RecordDBGet(ctx, key string)`, `RecordDBPut(ctx, key string)`, etc.
2. Modify Pebble store methods to call helper:
   - Example: `Get(ctx, key)` wraps with span "db.get" (key attr, error attr if present)
   - Same for Put, Range, Delete, Scan
3. Same for SQLite store (if still tested/used)

### Step 5: Operation Instrumentation
1. Modify `internal/operations/operation_registry.go`:
   - In `op.Run(ctx)` wrapper, create root span with op_id, op_type as attributes
   - Emit event at start ("operation.start")
   - Emit event at completion ("operation.end" + status)
   - Record operation duration (latency histogram)

### Step 6: AI/External Call Instrumentation
1. Modify `internal/openai/openai.go`:
   - Wrap `EmbedText()`, `ParseMetadata()`, and batch request calls with child spans
   - Record token count + estimated cost as attributes (if available from response)
   - Track error rate for failed calls

### Step 7: Dedup Engine Instrumentation
1. Modify `internal/dedup/engine.go`:
   - Add spans for `FullScan()`, `CheckBook()`, `PurgeStaleCandidates()` phases
   - Record phase duration + count of items processed (as span events)

### Step 8: Prometheus Metrics Endpoint
1. Add `/metrics` handler (or reuse existing if present) that exposes Prometheus metrics
   - Enables Grafana scraping
   - Metrics exposed: request rate, p50/p95/p99 latency, error rate, library size, op queue depth

### Step 9: Configuration and Optional Local Setup
1. Document OTEL_EXPORTER_OTLP_ENDPOINT env var in README / .env example
2. Optional: Add `make run-with-otel` target that:
   - Spins up local Jaeger (docker) or uses existing instance
   - Runs the server with OTEL_EXPORTER_OTLP_ENDPOINT set

---

## Test Strategy

### Unit Tests
- `internal/telemetry/telemetry_test.go`:
  - Test InitOTEL() with missing endpoint (no-op)
  - Test InitOTEL() with valid endpoint (returns shutdown func)
  - Test shutdown func closes cleanly

### Integration Tests
- Spin up local OTEL collector/Jaeger
- Run server with OTEL enabled
- Trigger operation (scan, dedup, metadata apply)
- Verify spans in Jaeger:
  - Root span with op_id attribute
  - Child spans for DB ops, AI calls
  - Latency recorded correctly
- Verify Prometheus metrics at `/metrics`:
  - request_count, request_duration_seconds, error_count, etc.

### Manual Verification
- Commands:
  ```bash
  docker run -d -p 16686:16686 jaegertracing/all-in-one:latest
  OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317 make run
  # Browse to http://localhost:16686 and search for traces
  curl http://localhost:8484/metrics | grep audiobook_organizer
  ```

---

## Rollback
- `git revert` to before OTEL changes
- Remove `go.mod` dependencies
- OTEL is additive; no schema changes needed
- If OTEL_EXPORTER_OTLP_ENDPOINT is not set, system operates as before

---

## Notes
- OTEL as a feature is optional: if endpoint is not configured, overhead is negligible (no-op tracer/meter)
- Spans should include context-specific attributes (book_id, op_id, user_id where applicable)
- Do NOT instrument hot loops (e.g., per-file processing in FullScan) excessively — batch span events instead
- Dedup scan can emit periodic progress spans ("scanned X files, Y dedup candidates") rather than per-file
