---
name: create-migration
description: Create a new PebbleDB/SQLite database migration following the project pattern
disable-model-invocation: true
---

# Create Database Migration

Create a new migration in `internal/database/migrations.go` following the established pattern.

## Arguments

- First argument: short description of the migration (e.g., "add genre field to books")

## Steps

1. **Find the current highest migration version:**
   ```bash
   grep 'Version:' internal/database/migrations.go | grep -oP '\d+' | sort -n | tail -1
   ```

2. **Create the migration function** at the end of migrations.go (before the closing of the file), following this pattern:

   ```go
   func migration<NNN>Up(store Store) error {
       // For PebbleDB operations:
       ps, ok := store.(*PebbleStore)
       if !ok {
           return nil // Skip for non-Pebble stores
       }
       _ = ps

       // For SQLite operations:
       ss, ok := store.(*SQLiteStore)
       if !ok {
           return nil
       }
       _, err := ss.db.Exec(`ALTER TABLE books ADD COLUMN new_field TEXT DEFAULT ''`)
       return err
   }
   ```

3. **Register the migration** in the `migrations` slice:
   ```go
   {
       Version:     <next_version>,
       Description: "<description>",
       Up:          migration<NNN>Up,
       Down:        nil,
   },
   ```

4. **Update the file version header** at the top of migrations.go

## Rules

- Migration version numbers must be sequential (no gaps)
- Always handle both PebbleStore and SQLiteStore if both need changes
- Use `Down: nil` unless rollback is critical (most migrations are additive)
- PebbleDB uses key-value patterns; SQLite uses SQL DDL
- Test with `go build ./internal/database/...` after adding
- For backfill operations, use versioned keys (e.g., `my_backfill_v1_done`) to prevent re-runs
