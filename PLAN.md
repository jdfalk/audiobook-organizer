<!-- file: PLAN.md -->
<!-- version: 2.0.0 -->
<!-- guid: 7d8e9f10-2345-4abc-9def-0123456789ab -->
<!-- last-edited: 2026-05-05 -->

# Plan: UOS-01 Schema Migrations

## Goal

Implement bot-task `docs/superpowers/bot-tasks/2026-05-04-uos-01-schema.md`
on branch `feat/uos-01-schema` by adding the exact `*_v2` core schema from
spec §2.1 as a reversible migration. No runtime code should use the new tables
in this task.

## Files To Change

- `internal/database/migrations.go`
  - Add the next migration entry.
  - Add `migrationNNNUp` and `migrationNNNDown` helpers, or follow the local
    migration pattern if migrations are split elsewhere.
- `internal/database/migrations_extra_test.go` or a new focused test file under
  `internal/database/`
  - Verify every `*_v2` table and required column type.
  - Verify every named index.
  - Verify `core_schema_meta_v2` rejects a second row.
  - Verify down migration drops all new tables and indexes.

## Ordered Steps

1. Inspect the existing migration registration and migration test helpers.
2. Pick the next migration number after the existing list.
3. Add the schema SQL exactly from spec §2.1, plus the single
   `core_schema_meta_v2` seed row.
4. Add the reversible down migration in dependency-safe drop order.
5. Add focused tests for table columns, indexes, single-row meta behavior, and
   down migration cleanup.
6. Run the required task checks as far as practical locally:
   `go test ./internal/database/...`, `make build`, and `make ci`.

## Rollback

Revert the migration entry, migration functions, and focused tests. The down
migration also drops every new `*_v2` table and index added by this task.
