// file: internal/database/pebble_store_versiongroup_backfill.go
// version: 1.0.1
// PERF-VERSIONS: one-time backfill that writes the
// book:versiongroup:<gid>:<id> secondary index for every existing book
// that has a VersionGroupID. Without this, /audiobooks/:id/versions
// falls back to a 10K-row full scan (~15s in prod). After the backfill
// runs once, the fast path in GetBooksByVersionGroup serves all reads.

package database

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/cockroachdb/pebble/v2"
)

const versionGroupBackfillKey = "system:backfill:versiongroup_index_v1_done"

// BackfillVersionGroupIndex writes the secondary index for every book with
// a non-empty VersionGroupID. Idempotent — gated by a sentinel key so
// repeated calls after the first successful run are cheap no-ops.
func (p *PebbleStore) BackfillVersionGroupIndex() error {
	if _, closer, err := p.db.Get([]byte(versionGroupBackfillKey)); err == nil {
		closer.Close()
		return nil
	}

	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("book:0"),
		UpperBound: []byte("book:;"),
	})
	if err != nil {
		return err
	}

	batch := p.db.NewBatch()
	indexed := 0
	scanned := 0
	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		// Only the primary `book:<id>` rows carry full JSON payloads;
		// skip every secondary index prefix.
		if strings.Contains(key, ":path:") || strings.Contains(key, ":series:") ||
			strings.Contains(key, ":author:") || strings.Contains(key, ":version:") ||
			strings.Contains(key, ":versiongroup:") || strings.Contains(key, ":hash:") ||
			strings.Contains(key, ":originalhash:") || strings.Contains(key, ":organizedhash:") ||
			strings.Contains(key, ":organizedhash:") {
			continue
		}

		var book Book
		if err := json.Unmarshal(iter.Value(), &book); err != nil {
			continue
		}
		scanned++
		if book.VersionGroupID == nil || *book.VersionGroupID == "" {
			continue
		}
		vgKey := []byte(fmt.Sprintf("book:versiongroup:%s:%s", *book.VersionGroupID, book.ID))
		if err := batch.Set(vgKey, []byte(book.ID), nil); err != nil {
			iter.Close()
			batch.Close()
			return err
		}
		indexed++
	}
	iter.Close()

	if err := batch.Set([]byte(versionGroupBackfillKey), []byte("1"), nil); err != nil {
		batch.Close()
		return err
	}
	if err := batch.Commit(pebble.Sync); err != nil {
		return err
	}
	slog.Info("versiongroup-backfill: scanned= indexed=", "scanned", scanned, "indexed", indexed)
	return nil
}
