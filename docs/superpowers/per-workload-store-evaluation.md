# Per-workload store evaluation: Pebble vs SQLite vs PostgreSQL vs Go-native NoSQL

This is the research companion for TODO item **4.7** ([`TODO.md`](../../TODO.md#L212)). The goal is to understand how different storage engines behave across the distinct workloads that the Audiobook Organizer database stack already carries: structured metadata and relationships, the dedup/change log, the embedding (vector) store, and the write-back/outbox journal.

## Workload characteristics

1. **Structured metadata & relationships** – tables like `books`, `authors`, `playlists`, and their join tables demand indexes, foreign-key semantics (logical or documented), and flexible slices of deeply nested records for the UI and its APIs.
2. **Dedup/change log** – the dedup engine, queue processing, and its supporting journal favor append-oriented writes, range scans over the most recent entries, and low-latency reads without complex joins.
3. **Embedding store** – vectors for similarity search are large binary blobs keyed by book or passage; reads are mostly by ID with occasional batch scans, and writes tend to be sequential as embeddings refresh for entire pieces.
4. **Write-back/outbox durability** – the `database.WriteBackQueue` acts as a replay log for remote sync; it needs transactional durability, reliable replay, and a compact-able record store for in-flight operations.

Each workload has unique needs around transactions, concurrency, schema richness, compaction/garbage collection, and operational complexity.

## Candidate storage engines

### Pebble (CockroachDB’s LSM key-value)
- **Model:** ordered byte-key/byte-value with explicit column families or prefix buckets when layered on top of a typed wrapper.
- **Concurrency:** multiple readers, a single writer per DB (with manual coordination) but very fast sequential appends and compaction.
- **Durability & compaction:** WAL-backed, compacts in background, handles large volumes of write-ahead data without blocking readers.
- **Query surface:** no SQL; every secondary index or relationship must be built by the application.
- **Operational burden:** comes as a Go dependency, no external process or network, but the tooling is low-level.

### SQLite (embedded SQL)
- **Model:** ACID relational with `CREATE TABLE`, indexes, and SQL queries for joins, filters, and schemas that can evolve via migrations.
- **Concurrency:** WAL mode lets many readers coexist with a single writer; heavier concurrency can hit the writer lock.
- **Durability & compaction:** file-based with autovacuum; easy to backup by copying the file, but large write workloads can trigger checkpointing stalls.
- **Query surface:** rich SQL, foreign-key checks, triggers, and extensions (`fts5`, JSON, etc.).
- **Operational burden:** minimal—no server to deploy—and the same binary can ship with a local DB file for clients or CLI.

### PostgreSQL (remote relational)
- **Model:** full SQL server with stored procedures, schemas, and rich indexing. Perfect for relational metadata, materialized views, and analytics.
- **Concurrency:** highly concurrent multi-writer/multi-reader with configurable isolation levels.
- **Durability & compaction:** WAL, background checkpoints, and `VACUUM/ANALYZE` to reclaim space; heavier ops may require DBA attention.
- **Query surface:** same as SQLite plus extensions (e.g., `pgvector`, parallel queries, JSONB indexing).
- **Operational burden:** requires a running server (local or remote) and migrations, but scales well for shared deployments.

### Go-native NoSQL (bbolt/Badger-style embedded KV)
- **Model:** simplified key-value (maps, B-tree, or LSM, depending on the library) with optional buckets but no SQL.
- **Concurrency & durability:** typically allows multiple readers, a single writer; durability varies by library but is often good enough for local stores.
- **Query surface:** only key-based operations, which is fine as long as the schema is flattened.
- **Operational burden:** zero external dependencies, low latency; can be tuned in Go code and is easy to embed inside the server binary.
- **Variants:** `bbolt` for B-tree semantics, `Badger` for pure LSM, and others like `Pebble` itself.

## Workload-by-engine evaluation

| Workload / criteria | Pebble | SQLite | PostgreSQL | Go-native NoSQL |
|---------------------|--------|--------|------------|-----------------|
| Structured metadata & relationships | ✶ Poor if you need SQL joins, but you can maintain manual indexes. Might fit for name-value tables if schema stays flat. | ✅ Very good: embedded SQL, indexes, migrations, low ops for local CLI/desktop deployments. | ✅✅ Best for server deployments that need multi-client writes, complex queries, and analytics. | ⚠︎ Works only if the schema stays extremely flat; lacks join/support for constraints. |
| Dedup/change log | ✅ Ideal: sequential writes, durable WAL, compaction-friendly for huge journals. | ⚠︎ OK for low-traffic journals, but single-writer checkpointing can stall high throughput. | ✅✅ Still handles high throughput, but heavier to operate, and local-only cases still need a lightweight option. | ✅ Good for simple append-only logs within the Go process, especially if the store is small. |
| Embedding store | ✅ Excellent for storing large blobs with deterministic keys; range scans for batch rebuilds work well. | ⚠︎ Blob storage with SQL is possible, but ballooning size and single writer may become bottlenecks. | ✅ Works, and extensions like `pgvector` help, but requires client-server setup for vector search. | ✅✅ Lightest-weight for per-node embedding caches, but lacks native vector search. |
| Write-back/outbox durability | ✅ Strong: WAL ensures replay, compaction keeps disk bounded, manual TTL possible. | ⚠︎ Works for small queues, but frequent checkpointing complicates durability guarantees. | ✅✅ Most recoverable and manageable for distributed deployments needing audit trails. | ✅ Simple and very fast if the store is kept small; lacks visibility for ops. |

▷ **Legend:** ✅ fits well, ⚠︎ workable with trade-offs, ✶ awkward/needs more work.

## Recommendations

1. **Split responsibilities.** Keep SQLite (or PostgreSQL) for structured metadata because the REST APIs rely on joins and the Admin UI benefits from SQL exports. PostgreSQL is the aspirational long-term backend for centralized deployments; SQLite can be the local single-user fallback.
2. **Keep Pebble for append-heavy workloads.** The dedup/change log and write-back queue already behave like LSM-friendly streams, so Pebble provides the best durability and compaction without extra processes.
3. **Treat embeddings as a dedicated KV store.** Pebble or another Go-native NoSQL engine (Badger/bbolt) is better than SQLite when embeddings grow in gigabytes and only require keyed access.
4. **Use Go-native NoSQL for quick experiments or single-file caches.** When we need zero external dependencies but don't want to manage compaction, a library like `bbolt` or `Badger` can host a per-node cache; this is also the easiest path for features that should stay in the Go process without daemonizing a full database.

## Next steps

- Surface this evaluation in `docs/backlog-2026-04-10.md` so PM/UX stakeholders can see the trade-offs when we split `database.Store` (#372–#382).
- Build small benchmarks that exercise the dedup log and embedding store against each candidate to validate throughput assumptions before adding support for a new backend.
- Once the interface split (#4.8) lands, wire each workload to a dedicated store implementation and continue adjusting the plan in `docs/superpowers/plans/2026-04-17-store-iface-sweep.md`.
