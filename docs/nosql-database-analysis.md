<!-- file: docs/nosql-database-analysis.md -->
<!-- version: 1.0.0 -->
<!-- guid: 7a8b9c0d-1e2f-3a4b-5c6d-7e8f9a0b1c2d -->
<!-- last-edited: 2026-03-22 -->

# NoSQL Database Analysis for Audiobook Organizer

## Context

The audiobook organizer currently supports two database backends behind a unified
`Store` interface (~270 methods, ~4,900 lines for PebbleDB, ~4,300 lines for
SQLite). This document evaluates 11 NoSQL databases as potential additional or
replacement backends.

### Current Architecture

- **Store interface**: 270+ methods across 30+ entity types
- **PebbleDB (default)**: Embedded LSM key-value store, pure Go, prefix-scanned
  key schema with JSON values, manual secondary indexes
- **SQLite (opt-in)**: Relational with CGO dependency, full SQL with JOINs and
  FTS, 36+ migrations

### Workload Profile

| Dimension | Value |
|---|---|
| Books | ~11,000 |
| Authors | ~3,000 |
| Series | ~8,500 |
| External ID mappings | ~97,000 |
| Read/write ratio | ~90/10 (read-heavy browsing, bursty writes) |
| Concurrent users | 1-5 (self-hosted household) |
| Full-text search | Title, author, narrator |
| Relationship complexity | Many-to-many (books-authors, books-narrators, books-tags) |
| Key-value patterns | User prefs, scan cache, operation state |
| Time-series patterns | Playback events, changelog, version history |
| Deployment | Single Linux server, self-hosted, must be simple |

---

## 1. MongoDB

### Overview

MongoDB is the most widely deployed document database. It stores data as BSON
(binary JSON) documents in collections. It is mature (first released 2009),
commercially backed by MongoDB Inc., and available under the Server Side Public
License (SSPL) for community edition.

### Go Driver Quality

The official `go.mongodb.org/mongo-driver/v2` is first-party, well-maintained,
and idiomatic Go. It supports generics-based codec, connection pooling, change
streams, transactions, and aggregation pipelines. The driver is one of the best
in the NoSQL ecosystem for Go.

### Embeddability

MongoDB requires a separate `mongod` server process. There is no embedded mode.
For self-hosting, this means either:
- Running mongod as a systemd service alongside the app
- Using a Docker sidecar
- Using MongoDB Atlas (cloud, defeats self-hosting purpose)

This is a significant operational burden for a single-binary self-hosted app.

### Fit for Our Workload

MongoDB is overkill for this scale but would handle it effortlessly. Document
model maps well to our Book struct (which is already JSON-serialized in PebbleDB).
Rich query language supports filtering, sorting, and pagination natively.
Aggregation pipeline handles dashboard stats well. Change streams could replace
our changelog polling.

### Many-to-Many Support

Two common patterns:
1. **Embedded arrays**: Store author IDs inside book documents. Works well for
   books-authors (small arrays), but querying "all books by author X" requires
   an index on the array field.
2. **Junction collections**: Separate `book_authors` collection, similar to
   relational junction tables. More normalized but requires $lookup (join).

MongoDB handles both patterns. Array indexing with `multikey indexes` makes
embedded arrays efficient for our cardinality (~3 authors per book).

### Full-Text Search

MongoDB has built-in text indexes (`db.books.createIndex({title: "text", author:
"text"})`). For more advanced search, MongoDB Atlas Search (Lucene-based) exists
but requires Atlas. The built-in text search is adequate for our needs.

### Migration Effort

**High-Medium**. The document model maps naturally to our existing JSON
serialization. Many methods would become simpler (MongoDB queries replace manual
prefix scanning). Estimated 3,000-4,000 lines. The main work is translating
270+ methods to MongoDB query/update operations and handling transactions where
needed.

### Self-Hosting Complexity

**High**. Requires running mongod separately, configuring auth, managing data
directory, backup strategy. Users must install MongoDB or use Docker. This is a
dealbreaker for the "download one binary and run" experience.

### Performance at Our Scale

Trivial. MongoDB handles millions of documents easily. 11K books would fit
entirely in the WiredTiger cache. Queries would be sub-millisecond with proper
indexes.

### Pros

1. Best-in-class document query language, aggregation, and indexing
2. Excellent Go driver with full feature parity
3. Built-in text search, TTL indexes, change streams

### Cons

1. Requires separate server process (no embedding)
2. SSPL license may concern some users
3. Heavy resource footprint (~200MB+ RAM) for a tiny dataset

### Verdict: No

The operational complexity of running a separate MongoDB server is incompatible
with a self-hosted single-binary application. The power of MongoDB is wasted at
our scale. If the app ever becomes multi-user SaaS, revisit.

---

## 2. Redis / Valkey / KeyDB

### Overview

Redis is the dominant in-memory key-value store (BSD-licensed until 2024, then
dual RSALv2/SSPL). **Valkey** is the Linux Foundation fork (BSD-3-Clause)
created after the Redis license change. **KeyDB** is a multithreaded Redis fork
(BSD-3-Clause). All three are protocol-compatible.

### Go Driver Quality

`github.com/redis/go-redis/v9` is the official Go client, supporting all three
servers. It is excellent: type-safe, supports pipelining, Lua scripting, pub/sub,
streams, and cluster mode. Community support is enormous.

### Embeddability

None of these are embeddable. They all require a separate server process. There
are some Go-native Redis-compatible servers (e.g., `tidwall/redcon` for building
one) but no drop-in embedded Redis.

**Miniredis** (`alicebob/miniredis`) exists for testing but is not suitable for
production data persistence.

### Fit for Our Workload

Redis excels at key-value lookups, caching, and pub/sub. It can model our data
using hashes (one hash per book), sets (for relationships), and sorted sets (for
ordering). However:
- No native document querying (filtering books by author AND format AND year
  requires RediSearch module or application-side filtering)
- Persistence is secondary (RDB snapshots + AOF). Data loss window exists.
- All data must fit in RAM. At our scale this is fine (~50MB), but it is a
  philosophical mismatch for a primary datastore.

### Many-to-Many Support

Modeled via Redis Sets:
- `book:{id}:authors` = set of author IDs
- `author:{id}:books` = set of book IDs
- Both sides must be maintained manually on every write.

This is error-prone and lacks transactional guarantees across multiple keys
(MULTI/EXEC helps but is not true ACID).

### Full-Text Search

Requires **RediSearch** module (now called Redis Search, part of Redis Stack).
This adds another dependency. Without it, only exact key lookups are possible.

### Migration Effort

**Very High**. Redis has no query language for complex filtering. Every method
that does "find books where X and Y" must be reimplemented using secondary index
sets or Lua scripts. Estimated 5,000+ lines with significant complexity.

### Self-Hosting Complexity

**High**. Requires running redis-server separately. While Redis is easy to
install, it is another process to manage, configure, and back up.

### Performance at Our Scale

Sub-millisecond for everything. Redis is absurdly fast at this scale. But speed
is not our bottleneck.

### Pros

1. Fastest possible reads and writes (in-memory)
2. Pub/sub and streams for real-time features
3. Huge ecosystem, every ops team knows Redis

### Cons

1. Not embeddable, requires separate server
2. Poor fit as primary datastore (no complex queries, persistence is secondary)
3. Many-to-many relationships require extensive manual index maintenance

### Verdict: No

Redis is a cache and message broker, not a primary document store. Using it as
the sole backend would require reimplementing query capabilities that PebbleDB
already handles via prefix scanning and that SQLite handles natively. The
operational burden of a separate server adds no value for our use case.

---

## 3. BadgerDB

### Overview

BadgerDB is a Go-native LSM-based key-value store created by Dgraph Labs.
Licensed under Apache 2.0. It is the most direct competitor to PebbleDB in the
Go embedded database space. Mature (v4.x), but development has slowed since
Dgraph pivoted.

### Go Driver Quality

BadgerDB IS the Go library. `github.com/dgraph-io/badger/v4` is the only way to
use it. API is clean and idiomatic: `txn.Get()`, `txn.Set()`, iterators with
prefix seeking. Supports ACID transactions, TTL on keys, and value log garbage
collection.

### Embeddability

Fully embedded, in-process. Single directory on disk. This is identical to
PebbleDB's deployment model. Zero operational overhead.

### Fit for Our Workload

Almost identical to PebbleDB. Both are LSM key-value stores. The key differences:
- BadgerDB separates keys and values (SSTables for keys, value log for values).
  This is better for large values and key-only iteration, but adds GC complexity.
- PebbleDB keeps keys and values together in SSTables (simpler, better for our
  small-to-medium value sizes).
- BadgerDB's value log GC must be triggered manually or via goroutine.

For our workload (JSON documents averaging 500B-2KB), PebbleDB's architecture is
slightly more efficient. BadgerDB's value separation shines with larger values
(>1KB keys, >4KB values).

### Many-to-Many Support

Identical to PebbleDB: manual secondary index keys. No improvement here.

### Full-Text Search

None built-in. Same limitation as PebbleDB. Would require the same external
approach (Bleve, application-side filtering).

### Migration Effort

**Low**. The API is structurally very similar to PebbleDB. Key schema can be
reused almost verbatim. The main changes are:
- `pebble.DB` -> `badger.DB`
- `db.Get(key)` -> `txn.Get(key)` (BadgerDB requires explicit transactions)
- Iterator API differences (BadgerDB uses `item.Value(func(val []byte) error)`)
- Add value log GC goroutine

Estimated: 1-2 weeks to port, ~4,500 lines, mostly mechanical translation.

### Self-Hosting Complexity

**None** (same as PebbleDB). Embedded in the binary. User sees a data directory.

### Performance at Our Scale

Comparable to PebbleDB. At 11K records, both are effectively instant. BadgerDB
may use slightly more disk space due to value log fragmentation but this is
negligible.

### Pros

1. Drop-in replacement for PebbleDB (same paradigm, similar API)
2. Supports TTL on keys natively (useful for session expiry, operation cleanup)
3. Fully embedded, pure Go, no CGO

### Cons

1. Value log GC adds operational complexity (must run periodically)
2. Development has slowed; PebbleDB is more actively maintained (CockroachDB)
3. No meaningful improvement over PebbleDB for our workload profile

### Verdict: No

BadgerDB is a lateral move from PebbleDB, not an upgrade. PebbleDB has stronger
backing (CockroachDB), more active development, simpler value management, and is
already integrated. There is no compelling reason to add or switch to BadgerDB
unless PebbleDB is abandoned.

---

## 4. BoltDB / bbolt

### Overview

BoltDB was a pure Go B+tree key-value store inspired by LMDB, created by Ben
Johnson. The original `boltdb/bolt` is archived. **bbolt**
(`go.etcd.io/bbolt`) is the maintained fork by the etcd team. Licensed under
MIT. Very mature and battle-tested (used by etcd, Consul, InfluxDB).

### Go Driver Quality

bbolt IS the Go library. The API is minimal and elegant: buckets (namespaces),
read-only and read-write transactions, cursor iteration. It is one of the most
well-designed Go database APIs.

### Embeddability

Fully embedded, single-file database. Even simpler than PebbleDB (one file vs.
a directory of SSTables). Memory-mapped I/O for reads.

### Fit for Our Workload

bbolt uses a B+tree (not LSM), which means:
- **Reads are faster** than LSM for point lookups and range scans (no
  compaction-related read amplification)
- **Writes are slower** for bulk operations (copy-on-write B+tree, single
  writer at a time)
- Single-writer constraint means batch imports would serialize

For our read-heavy workload, bbolt's B+tree is actually a better fit than an
LSM tree. The write bottleneck during batch imports is the main concern, but at
our scale (importing 11K books takes seconds either way) it is unlikely to be
noticeable.

### Many-to-Many Support

bbolt has **buckets** (like tables/namespaces), which is cleaner than PebbleDB's
prefix-based approach. You can have:
- Bucket `books` with book ID keys
- Bucket `book_authors` with composite keys
- Bucket `author_books` (reverse index)
- Nested buckets for hierarchical organization

This is a slight ergonomic improvement over PebbleDB's flat keyspace.

### Full-Text Search

None built-in. Same limitation as PebbleDB.

### Migration Effort

**Medium-Low**. The concepts map well:
- PebbleDB prefix groups -> bbolt buckets
- PebbleDB prefix scan -> bbolt cursor within bucket
- PebbleDB Get/Set -> bbolt txn.Bucket().Get/Put

The main differences:
- bbolt requires explicit transactions for all operations (even reads)
- Single-writer model means write methods need careful transaction scoping
- Bucket creation must happen in a write transaction

Estimated: 2-3 weeks, ~4,500 lines. The bucket model would actually make the
code slightly cleaner than PebbleDB's prefix strings.

### Self-Hosting Complexity

**None**. Single file, embedded. Arguably simpler than PebbleDB since it is one
file instead of a directory.

### Performance at Our Scale

Excellent for reads. At 11K records the entire database would be memory-mapped
and cached by the OS. Writes are serialized but at our scale this is
imperceptible (microseconds per write, even batch imports of 1,000 books would
take well under a second).

### Pros

1. Single-file database, simplest possible deployment
2. B+tree gives excellent read performance for our read-heavy workload
3. Buckets provide cleaner namespacing than prefix-based key schemes

### Cons

1. Single-writer model (one write transaction at a time)
2. Less actively developed than PebbleDB (maintenance mode, not feature development)
3. No built-in compaction; file size only grows (requires periodic rebuild)

### Verdict: Maybe

bbolt is a legitimate alternative to PebbleDB, especially for read-heavy
workloads. The bucket model is arguably more ergonomic. However, the
single-writer limitation and lack of active feature development make this a
sideways move rather than an upgrade. Worth considering if PebbleDB's write
amplification or directory-based storage becomes problematic, or if code clarity
from buckets is valued.

---

## 5. DynamoDB (Local)

### Overview

Amazon DynamoDB is a fully managed NoSQL key-value/document service on AWS.
**DynamoDB Local** is a downloadable Java application that simulates DynamoDB for
development. Licensed: proprietary (AWS). DynamoDB Local is free but requires a
JVM.

### Go Driver Quality

The official AWS SDK v2 (`github.com/aws/aws-sdk-go-v2/service/dynamodb`) is
well-maintained. However, it is designed for AWS service calls (HTTP-based),
meaning every operation goes through an HTTP API layer even when talking to
localhost. The API is verbose (attribute values must be marshaled into
DynamoDB-specific types).

### Embeddability

DynamoDB Local is a Java application (~300MB). It requires a JVM to run. It is
emphatically not embeddable in a Go process. Even as a local server, it is
heavyweight.

### Fit for Our Workload

DynamoDB's single-table design pattern requires pre-planning all access patterns.
Our 270-method interface with diverse query patterns would require a complex
GSI (Global Secondary Index) strategy. Key-value lookups are fast, but
DynamoDB's query model (partition key + sort key only) makes ad-hoc filtering
painful. You cannot do `WHERE author = X AND year > 2020 AND format = 'M4B'`
without a scan or GSI.

### Many-to-Many Support

DynamoDB's recommended pattern is "single-table design" with composite sort keys:
- PK=`BOOK#123`, SK=`AUTHOR#456` (book-author relationship)
- PK=`AUTHOR#456`, SK=`BOOK#123` (reverse, requires GSI)

This is workable but extraordinarily complex for our relationship count. Each new
relationship pattern requires a new GSI (limit: 20 per table).

### Full-Text Search

None. DynamoDB has no text search capability. Would need OpenSearch integration
(another heavy dependency).

### Migration Effort

**Very High**. DynamoDB's data model is fundamentally different from both our
current backends. Single-table design requires rethinking every access pattern.
The attribute value marshaling adds boilerplate. Estimated 6,000+ lines, 4-6
weeks.

### Self-Hosting Complexity

**Very High**. Requires JVM, 300MB+ download, separate process, and AWS SDK
configuration (even for local). No user would want to install Java to run an
audiobook organizer.

### Performance at Our Scale

Adequate but irrelevant. DynamoDB Local is not optimized for performance; it is a
development simulator. Real DynamoDB on AWS would be fast but defeats self-hosting.

### Pros

1. If migrating to cloud later, the backend is ready
2. Excellent at pure key-value and single-partition queries
3. AWS SDK is well-maintained

### Cons

1. Requires JVM (massive dependency for self-hosted app)
2. Data model is extremely restrictive for our diverse query patterns
3. DynamoDB Local is a development tool, not a production-grade local database

### Verdict: No

DynamoDB Local is a development simulator, not a real embedded database. The
operational complexity (JVM dependency), restrictive query model, and
self-hosting burden make this a poor fit. Only makes sense if the app were
migrating to AWS-hosted SaaS.

---

## 6. CouchDB / PouchDB

### Overview

Apache CouchDB is a document database focused on reliability and sync. It uses
HTTP/JSON for its API and implements the CouchDB Replication Protocol for
multi-master sync. Licensed under Apache 2.0. Mature (since 2005).

PouchDB is a JavaScript client-side database that syncs with CouchDB. It runs in
browsers and Node.js but has no Go equivalent.

### Go Driver Quality

No official Go driver. The best community option is
`github.com/go-kivik/kivik` (Apache 2.0), which provides a `database/sql`-like
interface for CouchDB. It is maintained but niche. CouchDB's HTTP API means you
could also use raw HTTP calls, but that is tedious.

### Embeddability

CouchDB requires a separate Erlang-based server. It is not embeddable. PouchDB
is JavaScript-only and irrelevant for a Go backend.

There are no Go-native CouchDB-compatible embedded databases.

### Fit for Our Workload

CouchDB's killer feature is multi-master replication and conflict resolution.
This is useful for:
- Syncing between devices (phone app <-> server)
- Offline-first applications

Our app is a single-server deployment. The sync capability adds complexity
without benefit. CouchDB's views (MapReduce-based indexes) are powerful but have
a steep learning curve and are slow to build.

### Many-to-Many Support

CouchDB views can emit multiple keys per document, enabling many-to-many
queries. But views must be pre-defined and are computed lazily (first query after
data change is slow). The pattern:

```javascript
// View: books_by_author
function(doc) {
  if (doc.type === "book") {
    doc.author_ids.forEach(id => emit(id, doc._id));
  }
}
```

### Full-Text Search

CouchDB has optional Lucene-based search via `dreyfus` (built into CouchDB 3.x).
Adequate for our needs.

### Migration Effort

**High**. CouchDB's HTTP API is fundamentally different from embedded key-value
access. Views must be designed for each query pattern. The kivik driver adds a
layer but cannot hide the impedance mismatch. Estimated 4,000-5,000 lines.

### Self-Hosting Complexity

**High**. Requires Erlang runtime, CouchDB daemon, configuration. Docker makes
it manageable but it is still a separate service.

### Performance at Our Scale

Fine. CouchDB is not the fastest but 11K documents would be trivial. View builds
are the main performance concern (seconds for initial build, fast after).

### Pros

1. Best-in-class replication (if multi-device sync is ever needed)
2. HTTP API makes it accessible from any language
3. Built-in full-text search in CouchDB 3.x

### Cons

1. Requires separate Erlang-based server
2. No Go-native embedded option
3. MapReduce views are complex and slow to develop/iterate

### Verdict: No

CouchDB's strength (replication) is irrelevant for a single-server audiobook
organizer. The operational burden of an Erlang-based server, lack of an embedded
option, and niche Go driver make this a poor fit.

---

## 7. FoundationDB

### Overview

FoundationDB is a distributed, ACID-compliant key-value store created by Apple
(acquired 2015, open-sourced 2018). Licensed under Apache 2.0. It is the
foundation for Apple's CloudKit, Snowflake, and other large-scale systems.
Extremely mature and reliable.

### Go Driver Quality

The official Go binding (`github.com/apple/foundationdb/bindings/go`) requires
the FoundationDB C client library (CGO). It is well-maintained but the CGO
dependency undermines one of PebbleDB's key advantages (pure Go builds).

### Embeddability

Not embeddable. FoundationDB requires a cluster of `fdbserver` processes (minimum
1 for development, recommended 3+ for production). Even single-node setup
requires the FDB daemon, FDB client library, and cluster file configuration.

### Fit for Our Workload

FoundationDB provides an ordered key-value store with strong ACID guarantees and
up to 5-second transactions. Its "layer" concept allows building higher-level
data models (document, graph, SQL) on top. However:
- At our scale, the distributed architecture is pure overhead
- The 5-second transaction limit complicates long-running batch operations
- Key-value API is lower-level than PebbleDB (no built-in iteration helpers)

### Many-to-Many Support

Same as any key-value store: manual secondary indexes. FoundationDB's
directory and subspace layers provide namespacing similar to bbolt buckets.
The `foundationdb/tuple` package provides structured key encoding.

### Full-Text Search

None built-in. Would need external index.

### Migration Effort

**Very High**. CGO dependency, external server requirement, transaction model
differences, tuple encoding for keys. Estimated 5,000+ lines, 4-6 weeks.

### Self-Hosting Complexity

**Very High**. Installing FoundationDB requires:
- Platform-specific packages (`.deb`/`.rpm`)
- FDB server daemon configuration
- Cluster file setup
- Client library installation

This is enterprise infrastructure, not consumer software.

### Performance at Our Scale

Absurdly over-provisioned. FoundationDB is designed for billions of keys across
dozens of machines. At 11K records on one server, it would work but with
unnecessary overhead (network layer, distributed commit protocol).

### Pros

1. Strongest ACID guarantees of any distributed KV store
2. Ordered keys with efficient range scans
3. Used at massive scale (Apple, Snowflake) -- bulletproof

### Cons

1. Requires separate server cluster (not embeddable)
2. CGO dependency for Go bindings
3. Extreme overkill for single-server, 11K record dataset

### Verdict: No

FoundationDB is enterprise-grade distributed infrastructure. Using it for a
self-hosted audiobook organizer is like using a cruise ship to cross a pond.
The operational complexity, CGO requirement, and distributed overhead provide
zero benefit at our scale.

---

## 8. TiKV

### Overview

TiKV is a distributed, transactional key-value store. It is part of the TiDB
ecosystem (PingCAP). Licensed under Apache 2.0. Uses RocksDB as its storage
engine and Raft for consensus. CNCF graduated project.

### Go Driver Quality

The official Go client is `github.com/tikv/client-go/v2`. It is maintained by
PingCAP. The API is lower-level than most databases: raw key-value or
transactional key-value modes. Not as polished as MongoDB or Redis drivers.

### Embeddability

Not embeddable. TiKV requires:
- TiKV server nodes (minimum 3 for production)
- PD (Placement Driver) nodes for cluster management
- Raft groups for replication

Even single-node development setup requires PD + TiKV processes.

### Fit for Our Workload

TiKV is designed for multi-terabyte, multi-datacenter workloads. Its distributed
transaction protocol (Percolator-based) adds latency to every operation. At our
scale, every operation would be network-bound despite running locally.

### Many-to-Many Support

Manual secondary indexes, same as any key-value store. TiKV's range partitioning
provides good locality for prefix-based key schemes similar to our PebbleDB
approach.

### Full-Text Search

None built-in. TiDB (the SQL layer) can integrate with TiFlash for analytics
but not full-text search.

### Migration Effort

**Very High**. Distributed system setup, two-phase commit semantics, PD
dependency. Estimated 5,000+ lines.

### Self-Hosting Complexity

**Extreme**. Minimum 3 processes (PD + TiKV + optional TiDB). Requires
understanding of distributed systems for troubleshooting. Not suitable for
consumer deployment.

### Performance at Our Scale

Overkill. Cross-process Raft consensus adds milliseconds to every write, which
is pure overhead for a single-node deployment.

### Pros

1. Horizontal scalability if the app ever needed it
2. Strong consistency with distributed transactions
3. CNCF graduated, strong community

### Cons

1. Requires multi-process cluster (PD + TiKV minimum)
2. Distributed transaction overhead for a single-node use case
3. Operational complexity far exceeds our needs

### Verdict: No

Same reasoning as FoundationDB. TiKV is distributed infrastructure for
planet-scale systems. No benefit for a self-hosted single-server application
with 11K records.

---

## 9. ScyllaDB

### Overview

ScyllaDB is a C++ rewrite of Apache Cassandra, offering Cassandra compatibility
with much better performance. Licensed under AGPL-3.0 (open-source edition).
It is a wide-column store optimized for time-series and high-throughput workloads.

### Go Driver Quality

`github.com/gocql/gocql` (Cassandra driver) works with ScyllaDB.
`github.com/scylladb/gocql` is ScyllaDB's optimized fork with shard-aware
routing. Both are mature. CQL (Cassandra Query Language) is SQL-like but with
significant limitations (no JOINs, no subqueries, limited WHERE clauses).

### Embeddability

Not embeddable. ScyllaDB is a cluster database requiring its own server process.
Minimum viable deployment is a single-node cluster, but it still needs 2GB+ RAM
and its own process.

### Fit for Our Workload

ScyllaDB/Cassandra's data model requires denormalization. Every query pattern
needs its own table. For example:
- `books_by_id` (partition key: book_id)
- `books_by_author` (partition key: author_id, clustering key: book_id)
- `books_by_series` (partition key: series_id, clustering key: sequence)

This means maintaining multiple copies of data and ensuring consistency across
them. CQL has no JOINs, so "get book with author and series info" requires
multiple queries.

Our read-heavy, low-volume workload is the exact opposite of ScyllaDB's sweet
spot (high-throughput writes, time-series ingestion).

### Many-to-Many Support

Must be fully denormalized. Each access pattern gets its own materialized view or
table. Book-author relationships require:
- `book_authors_by_book` table (lookup authors for a book)
- `book_authors_by_author` table (lookup books for an author)
- Application-level consistency maintenance

### Full-Text Search

ScyllaDB has secondary indexes and materialized views but no full-text search.
Would need Elasticsearch sidecar.

### Migration Effort

**Extreme**. Complete data model redesign for denormalization. Every query pattern
becomes a separate table. The 270-method interface would need fundamental
rethinking. Estimated 6,000+ lines, 6-8 weeks, with ongoing maintenance burden.

### Self-Hosting Complexity

**Very High**. ScyllaDB requires dedicated server resources, tuning of memory and
I/O schedulers, and understanding of Cassandra data modeling. Far beyond consumer
deployment.

### Performance at Our Scale

Wasted. ScyllaDB is designed for millions of operations per second across a
cluster. At 11K records it would use orders of magnitude more resources than
needed.

### Pros

1. Exceptional performance for time-series / high-throughput writes
2. CQL provides SQL-like familiarity
3. Linear horizontal scalability

### Cons

1. Requires separate server, significant resources (2GB+ RAM)
2. Denormalized data model is painful for our many-to-many relationships
3. No JOINs, no full-text search, no transactions across partitions

### Verdict: No

ScyllaDB is purpose-built for massive write-throughput time-series workloads.
Our read-heavy, relationship-rich, 11K-record dataset is the worst possible
fit. The denormalization burden alone makes this impractical.

---

## 10. SurrealDB

### Overview

SurrealDB is a multi-model database supporting documents, graph relations, and
key-value patterns in a single system. It has its own query language (SurrealQL)
that blends SQL with graph traversal. Licensed under Business Source License 1.1
(converts to Apache 2.0 after 4 years). Relatively new (v1.0 released 2023),
written in Rust.

### Go Driver Quality

The official Go SDK is `github.com/surrealdb/surrealdb.go`. It is maintained
by the SurrealDB team but is less mature than drivers for established databases.
The SDK uses WebSocket or HTTP connections. API ergonomics are reasonable but
documentation is thinner than MongoDB or Redis drivers.

### Embeddability

SurrealDB supports an embedded mode via its Rust library, but the Go SDK
connects via WebSocket/HTTP to a `surreal start` server process. There is no
in-process embedded mode for Go. However, SurrealDB can run with a
**RocksDB or in-memory backend** as a single-binary server with persistent
storage, which is relatively lightweight.

### Fit for Our Workload

This is where SurrealDB gets interesting. Its multi-model approach addresses
several of our pain points simultaneously:

- **Document model**: Books, operations, preferences stored as documents
  (like MongoDB)
- **Graph relations**: `RELATE book:123->authored_by->author:456` provides
  native many-to-many without junction tables or manual index keys
- **Key-value**: Simple `SET` / `GET` for preferences and cache
- **Live queries**: Push notifications for real-time UI updates

SurrealQL can do what currently requires complex prefix scanning in PebbleDB:
```sql
SELECT * FROM book WHERE ->authored_by->author.name = "Brandon Sanderson"
  AND format = "M4B" ORDER BY title LIMIT 20 START 0;
```

### Many-to-Many Support

**Best-in-class for our use case.** Graph edges are first-class:
```sql
RELATE book:123->authored_by->author:456 SET role = "author", position = 0;
RELATE book:123->narrated_by->narrator:789 SET role = "narrator";
RELATE book:123->tagged_with->tag:scifi;
```

Querying relationships is natural:
```sql
SELECT ->authored_by->author FROM book:123;       -- book's authors
SELECT <-authored_by<-book FROM author:456;       -- author's books
```

This eliminates the manual secondary index maintenance that is the biggest
pain point of our PebbleDB implementation.

### Full-Text Search

SurrealDB supports full-text search indexes:
```sql
DEFINE INDEX book_search ON book FIELDS title, description SEARCH ANALYZER ascii BM25;
SELECT * FROM book WHERE title @@ "mistborn";
```

This is built-in and adequate for our needs.

### Migration Effort

**High-Medium**. While SurrealQL maps well to our domain, the ~270 methods still
need translation. The graph model would simplify many-to-many methods but the
query language is unfamiliar. Key considerations:
- Document operations are straightforward
- Relationship methods become much simpler (graph edges vs manual index keys)
- Transactions are supported but the model differs
- The Go SDK's type marshaling needs verification for our complex structs

Estimated: 3,500-4,500 lines, 3-4 weeks. Many methods would be simpler than
current PebbleDB equivalents.

### Self-Hosting Complexity

**Medium**. SurrealDB can run as a single binary with file-backed storage:
```bash
surreal start --log info --user root --pass root file:audiobooks.db
```

This is heavier than embedded PebbleDB (separate process) but lighter than
MongoDB or CouchDB (no Erlang, no JVM, single ~30MB binary). Could potentially
be bundled or launched as a subprocess.

### Performance at Our Scale

More than adequate. SurrealDB is young so benchmarks are less established, but
11K records is trivial for any database. Query performance depends heavily on
index design.

### Pros

1. Native graph edges solve our many-to-many relationship modeling elegantly
2. Multi-model (document + graph + KV) means one backend handles all patterns
3. Built-in full-text search, live queries, and permissions

### Cons

1. Young project (v2.x), less battle-tested than PebbleDB/bbolt
2. Requires separate server process (no Go-embedded mode)
3. BSL license may concern some users (though it converts to Apache 2.0)

### Verdict: Maybe

SurrealDB is the most architecturally interesting option on this list. Its graph
model would eliminate the worst parts of our PebbleDB implementation (manual
secondary indexes for many-to-many relationships). The main concerns are
maturity (it is young), the separate server process, and the BSL license. If
the project matures and gains an embedded Go mode, it would become a strong
candidate. Worth watching closely.

---

## 11. FerretDB

### Overview

FerretDB provides a MongoDB-compatible API on top of PostgreSQL (or SQLite).
It translates MongoDB wire protocol commands into SQL queries. Licensed under
Apache 2.0. The goal is to be a drop-in MongoDB replacement using proven
relational backends.

### Go Driver Quality

FerretDB uses the standard MongoDB Go driver (`go.mongodb.org/mongo-driver`),
which is excellent. From the application's perspective, it IS MongoDB. The
translation happens on the server side.

### Embeddability

FerretDB itself is a Go binary that runs as a proxy server. However, it recently
added an **embedded mode** that can run in-process. With the SQLite backend, this
means you can get MongoDB API semantics on an embedded SQLite database within
your Go process. This is interesting.

However, the SQLite backend reintroduces the CGO dependency that PebbleDB
was chosen to avoid. The PostgreSQL backend requires an external server.

### Fit for Our Workload

FerretDB gives us MongoDB's document query language on top of SQLite's proven
storage. This combination could be appealing:
- MongoDB-style queries for filtering, sorting, aggregation
- SQLite's reliability and single-file storage
- No need to run a real MongoDB server

However, there is a performance overhead from query translation (MongoDB query
-> SQL -> results -> MongoDB response format). At our scale this is unlikely to
be noticeable, but it adds a layer of abstraction and potential bugs.

### Many-to-Many Support

Same as MongoDB (embedded arrays or junction collections). The underlying
PostgreSQL/SQLite stores documents as JSONB/JSON, so array queries work but may
not be as optimized as native MongoDB.

### Full-Text Search

FerretDB's text search support is limited. It does not fully implement MongoDB's
`$text` operator. This is a known gap. You would need to use the underlying
database's FTS directly (PostgreSQL's tsvector or SQLite's FTS5), which defeats
the purpose of the abstraction.

### Migration Effort

**Medium**. If writing a MongoDB-style backend, the document model maps well
to our existing JSON structures. But we would be writing against the MongoDB
driver API, not SQL, which is a different paradigm from both current backends.
Estimated: 3,500-4,500 lines.

### Self-Hosting Complexity

**Medium-Low (embedded SQLite mode)** or **High (PostgreSQL mode)**.

The embedded mode with SQLite is compelling: FerretDB runs in-process, stores
data in a local SQLite file, and provides MongoDB query semantics. The CGO
dependency is the main drawback.

### Performance at Our Scale

Adequate. The translation layer adds overhead per query but at 11K records this
is negligible. Benchmarks show FerretDB is 2-10x slower than native MongoDB for
most operations, but native MongoDB speed is not needed at our scale.

### Pros

1. MongoDB API on embedded SQLite (best of both worlds, in theory)
2. Uses battle-tested MongoDB Go driver
3. Apache 2.0 license, open-source, backed by FerretDB Inc.

### Cons

1. Translation layer adds complexity and potential bugs
2. Incomplete MongoDB compatibility (text search gaps, no change streams)
3. SQLite backend requires CGO (same issue our PebbleDB adoption solved)

### Verdict: Maybe (Weak)

FerretDB's embedded SQLite mode is a creative solution: MongoDB query ergonomics
on top of SQLite storage. However, the CGO dependency, incomplete MongoDB
compatibility, and translation overhead introduce more problems than they solve.
If we were starting fresh and wanted document-query semantics without running
MongoDB, it would be worth prototyping. Given that we already have two working
backends, the marginal benefit does not justify the integration effort.

---

## Summary Comparison

| Database | Embeddable | Pure Go | Self-Host Ease | Query Power | M2M Support | FTS | Migration Effort | Verdict |
|---|---|---|---|---|---|---|---|---|
| MongoDB | No | N/A | Hard | Excellent | Good | Built-in | Medium | **No** |
| Redis/Valkey | No | N/A | Hard | Poor | Manual | Module | Very High | **No** |
| BadgerDB | Yes | Yes | Easy | KV only | Manual | None | Low | **No** |
| BoltDB/bbolt | Yes | Yes | Easy | KV only | Manual | None | Medium-Low | **Maybe** |
| DynamoDB Local | No | N/A | Very Hard | Limited | Complex | None | Very High | **No** |
| CouchDB | No | N/A | Hard | Views | Views | Built-in | High | **No** |
| FoundationDB | No | CGO | Very Hard | KV only | Manual | None | Very High | **No** |
| TiKV | No | N/A | Very Hard | KV only | Manual | None | Very High | **No** |
| ScyllaDB | No | N/A | Very Hard | CQL | Denorm | None | Extreme | **No** |
| SurrealDB | No* | N/A | Medium | Excellent | Native Graph | Built-in | High-Medium | **Maybe** |
| FerretDB | Partial | CGO | Medium | Good | Good | Partial | Medium | **Maybe (Weak)** |

\* SurrealDB has an embedded Rust mode but no Go-embedded mode.

## Recommendations

### Keep Current Architecture (Recommended)

PebbleDB remains the best choice for this application. The reasons it was
selected still hold:
- Pure Go, no CGO, trivial cross-compilation
- Embedded (zero operational overhead)
- Adequate performance at our scale
- Mature (CockroachDB-backed)

SQLite as an opt-in backend covers users who want SQL query capabilities and
are willing to accept the CGO dependency.

### If Adding a Third Backend

**bbolt** is the only option that clearly fits the "embedded, pure Go, simple"
criteria while offering a tangible benefit (B+tree for read-heavy workloads,
bucket-based namespacing). However, the benefit over PebbleDB is marginal.

### If Rearchitecting

**SurrealDB** is the most interesting option for a future rearchitecture. Its
native graph model would eliminate the most complex parts of the PebbleDB
implementation (manual secondary indexes for many-to-many relationships) and
provide built-in full-text search. The trade-off is maturity and requiring a
separate process. Worth revisiting in 1-2 years as the project matures.

### What Would Actually Help More Than a New Backend

Rather than adding another backend, the effort would be better spent on:

1. **Adding Bleve or Tantivy-based full-text search** to the existing PebbleDB
   backend (addresses the biggest functional gap)
2. **Reducing the Store interface surface** -- 270+ methods is unwieldy; some
   methods could be composed from primitives
3. **Adding a query builder** that generates prefix scans for PebbleDB and SQL
   for SQLite, reducing per-method implementation burden
4. **Implementing batch transaction support** in the Store interface to improve
   bulk import performance on PebbleDB

These improvements would deliver more user-visible value than any backend swap.
