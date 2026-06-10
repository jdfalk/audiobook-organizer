// file: internal/database/pebble_activity_store.go
// version: 1.0.0
// guid: d4e5f6a7-b8c9-0004-def0-000000000004

// Package database — PebbleDB-backed activity log store.
//
// WHY a Pebble backend:
//   - The NutsDB activity store (nuts_activity_store.go) works but carries an
//     entire extra storage engine dependency (nutsdb/nutsdb). Pebble is already
//     the primary database engine. Removing NutsDB requires a Pebble backend
//     that satisfies the same ActivityStorer interface.
//   - Key layout mirrors NutsDB verbatim so lexicographic ordering and range
//     scans behave identically (no behavioral change to callers).
//
// Key layout (all keys are []byte; prefixes end with ':'):
//
//	act:<tier>:<20d-unix-nano>:<ulid>        = JSON(ActivityEntry)   primary
//	act:op:<op_id>:<20d-unix-nano>:<ulid>    = []byte("<tier>:<20d-unix-nano>:<ulid>")  op index
//	act:bk:<book_id>:<20d-unix-nano>:<ulid>  = []byte("<tier>:<20d-unix-nano>:<ulid>")  book index
//
// Compared with NutsDB, Pebble is a single shared key-space so every key is
// scoped with "act:" to avoid collisions with other prefixes. The "tier" component
// in the primary key separates tiers without needing separate buckets; Pebble range
// scans over ["act:<tier>:", "act:<tier>;") return exactly that tier's entries in
// timestamp order because ';' (0x3B) is one above ':' (0x3A) in ASCII.
package database

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/pebble/v2"
	"github.com/oklog/ulid/v2"
)

// PebbleActivityStore persists activity log entries in a shared PebbleDB database.
// It satisfies the ActivityStorer interface and is a drop-in replacement for both
// ActivityStore (SQLite) and NutsActivityStore.
//
// The caller retains ownership of the *pebble.DB — Close() on this store is a no-op.
type PebbleActivityStore struct {
	db      *pebble.DB
	counter atomic.Int64
}

// NewPebbleActivityStore creates a PebbleActivityStore backed by the provided DB.
// The caller retains ownership of db; Close() on this store does NOT close db.
func NewPebbleActivityStore(db *pebble.DB) *PebbleActivityStore {
	s := &PebbleActivityStore{db: db}
	// Seed counter from current time so IDs don't collide across restarts.
	s.counter.Store(time.Now().UnixNano())
	return s
}

// Close is a no-op: the caller owns the PebbleDB instance.
func (s *PebbleActivityStore) Close() error { return nil }

// DB returns the underlying *pebble.DB. Used by callers (e.g., backfill, registry wiring)
// that need to check the backfill sentinel key directly.
func (s *PebbleActivityStore) DB() *pebble.DB { return s.db }

// ── Key construction ──────────────────────────────────────────────────────────

// pactPrimaryKey builds the primary key for a tier entry:
//
//	act:<tier>:<20d-unix-nano>:<ulid>
func pactPrimaryKey(tier string, t time.Time, id string) []byte {
	return []byte(fmt.Sprintf("act:%s:%020d:%s", tier, t.UnixNano(), id))
}

// pactPrimaryPrefix returns the inclusive lower-bound prefix for a tier range scan:
//
//	act:<tier>:
func pactPrimaryPrefix(tier string) []byte {
	return []byte("act:" + tier + ":")
}

// pactPrimaryUpperBound returns the exclusive upper-bound for a tier range scan.
// ';' is ASCII 0x3B — one above ':' (0x3A) — so the range covers exactly all
// entries for the tier.
func pactPrimaryUpperBound(tier string) []byte {
	return []byte("act:" + tier + ";")
}

// pactIndexRef encodes the cross-reference value stored in secondary indexes:
// "<tier>:<20d-unix-nano>:<ulid>" — enough to reconstruct the primary key.
func pactIndexRef(tier string, t time.Time, id string) []byte {
	return []byte(fmt.Sprintf("%s:%020d:%s", tier, t.UnixNano(), id))
}

// pactPrimaryKeyFromRef reconstructs the primary key from an index reference value.
// ref = "<tier>:<20d-unix-nano>:<ulid>"
func pactPrimaryKeyFromRef(ref []byte) ([]byte, bool) {
	s := string(ref)
	if !strings.Contains(s, ":") {
		return nil, false
	}
	return []byte("act:" + s), true
}

// ── ActivityStorer implementation ─────────────────────────────────────────────

// Record inserts an ActivityEntry and returns a synthetic int64 ID.
func (s *PebbleActivityStore) Record(e ActivityEntry) (int64, error) {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}
	if e.Level == "" {
		e.Level = "info"
	}
	if e.Tier == "" {
		e.Tier = "change"
	}

	id := s.counter.Add(1)
	e.ID = id

	entryID := ulid.Make().String()
	primaryKey := pactPrimaryKey(e.Tier, e.Timestamp, entryID)

	b, err := json.Marshal(e)
	if err != nil {
		return 0, fmt.Errorf("pebble_activity_store: marshal: %w", err)
	}

	batch := s.db.NewBatch()
	defer batch.Close()

	if err := batch.Set(primaryKey, b, nil); err != nil {
		return 0, fmt.Errorf("pebble_activity_store: set primary: %w", err)
	}

	// Secondary index: op_id → primary ref
	if e.OperationID != "" {
		opKey := []byte(fmt.Sprintf("act:op:%s:%020d:%s", e.OperationID, e.Timestamp.UnixNano(), entryID))
		ref := pactIndexRef(e.Tier, e.Timestamp, entryID)
		if err := batch.Set(opKey, ref, nil); err != nil {
			return 0, fmt.Errorf("pebble_activity_store: set op index: %w", err)
		}
	}

	// Secondary index: book_id → primary ref
	if e.BookID != "" {
		bkKey := []byte(fmt.Sprintf("act:bk:%s:%020d:%s", e.BookID, e.Timestamp.UnixNano(), entryID))
		ref := pactIndexRef(e.Tier, e.Timestamp, entryID)
		if err := batch.Set(bkKey, ref, nil); err != nil {
			return 0, fmt.Errorf("pebble_activity_store: set book index: %w", err)
		}
	}

	if err := batch.Commit(pebble.Sync); err != nil {
		return 0, fmt.Errorf("pebble_activity_store: commit: %w", err)
	}

	return id, nil
}

// Query returns entries matching f, newest-first, plus the total matching count.
func (s *PebbleActivityStore) Query(f ActivityFilter) ([]ActivityEntry, int, error) {
	if f.Limit == 0 {
		f.Limit = 50
	}

	// Fast path: op_id or book_id filter → use secondary index.
	if f.OperationID != "" {
		return s.queryByIndexPrefix(fmt.Sprintf("act:op:%s:", f.OperationID), f)
	}
	if f.BookID != "" {
		return s.queryByIndexPrefix(fmt.Sprintf("act:bk:%s:", f.BookID), f)
	}

	// General path: scan tier bucket(s) by time range.
	tiers := actTiers
	if f.Tier != "" {
		tiers = []string{f.Tier}
	}

	var all []ActivityEntry
	for _, tier := range tiers {
		entries, err := s.scanTier(tier, f.Since, f.Until)
		if err != nil {
			return nil, 0, err
		}
		all = append(all, entries...)
	}

	// Apply remaining filters in Go.
	filtered := make([]ActivityEntry, 0, len(all))
	for _, e := range all {
		if matchesFilter(e, f) {
			filtered = append(filtered, e)
		}
	}

	// Sort newest-first; digest entries sort last (matching SQL ORDER BY compacted ASC, timestamp DESC).
	sort.Slice(filtered, func(i, j int) bool {
		ci := boolInt(filtered[i].Tier == "digest")
		cj := boolInt(filtered[j].Tier == "digest")
		if ci != cj {
			return ci < cj
		}
		return filtered[i].Timestamp.After(filtered[j].Timestamp)
	})

	total := len(filtered)

	// Paginate.
	start := f.Offset
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + f.Limit
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[start:end], total, nil
}

// Summarize groups old entries by (operation_id, type), writes a summary row,
// and deletes the originals. Returns count of deleted rows.
func (s *PebbleActivityStore) Summarize(ctx context.Context, olderThan time.Time, tier string) (int, error) {
	kvs, err := s.scanTierKVs(tier, nil, &olderThan)
	if err != nil {
		return 0, err
	}

	type groupKey struct{ opID, typ string }
	type group struct{ kvs []pactKV }
	groups := make(map[groupKey]*group)

	for _, kv := range kvs {
		if kv.entry.PrunedAt != nil {
			continue
		}
		k := groupKey{opID: kv.entry.OperationID, typ: kv.entry.Type}
		if groups[k] == nil {
			groups[k] = &group{}
		}
		groups[k].kvs = append(groups[k].kvs, kv)
	}
	if len(groups) == 0 {
		return 0, nil
	}

	totalDeleted := 0
	now := time.Now().UTC()

	for gk, g := range groups {
		select {
		case <-ctx.Done():
			return totalDeleted, ctx.Err()
		default:
		}

		entries := make([]ActivityEntry, len(g.kvs))
		for i, kv := range g.kvs {
			entries[i] = kv.entry
		}

		first := entries[0].Timestamp
		last := entries[len(entries)-1].Timestamp
		summaryText := fmt.Sprintf("Summary: %d %s entries (%s to %s)",
			len(entries), gk.typ,
			first.Format(time.RFC3339), last.Format(time.RFC3339),
		)

		prunedAt := now
		summary := ActivityEntry{
			ID:          s.counter.Add(1),
			Timestamp:   now,
			Tier:        tier,
			Type:        gk.typ,
			Level:       "info",
			Source:      "summarize",
			OperationID: gk.opID,
			Summary:     summaryText,
			PrunedAt:    &prunedAt,
		}

		summaryID := ulid.Make().String()
		summaryKey := pactPrimaryKey(tier, now, summaryID)
		summaryBytes, err := json.Marshal(summary)
		if err != nil {
			return totalDeleted, err
		}

		batch := s.db.NewBatch()
		if setErr := batch.Set(summaryKey, summaryBytes, nil); setErr != nil {
			batch.Close()
			return totalDeleted, setErr
		}
		for _, kv := range g.kvs {
			if delErr := batch.Delete(kv.key, nil); delErr != nil {
				batch.Close()
				return totalDeleted, delErr
			}
		}
		if commitErr := batch.Commit(pebble.Sync); commitErr != nil {
			batch.Close()
			return totalDeleted, fmt.Errorf("pebble_activity_store: summarize commit: %w", commitErr)
		}
		batch.Close()
		totalDeleted += len(g.kvs)
	}
	return totalDeleted, nil
}

// Prune hard-deletes all entries of the given tier older than olderThan.
func (s *PebbleActivityStore) Prune(olderThan time.Time, tier string) (int, error) {
	kvs, err := s.scanTierKVs(tier, nil, &olderThan)
	if err != nil {
		return 0, err
	}
	if len(kvs) == 0 {
		return 0, nil
	}

	deleted := 0
	// Delete in batches of 500 to keep batch size reasonable.
	for i := 0; i < len(kvs); i += 500 {
		end := i + 500
		if end > len(kvs) {
			end = len(kvs)
		}
		batch := s.db.NewBatch()
		for _, kv := range kvs[i:end] {
			if err := batch.Delete(kv.key, nil); err != nil {
				batch.Close()
				return deleted, fmt.Errorf("pebble_activity_store: prune batch delete: %w", err)
			}
		}
		if err := batch.Commit(pebble.Sync); err != nil {
			batch.Close()
			return deleted, fmt.Errorf("pebble_activity_store: prune batch commit: %w", err)
		}
		batch.Close()
		deleted += end - i
	}
	return deleted, nil
}

// GetDistinctSources returns unique sources with entry counts, ordered by count desc.
func (s *PebbleActivityStore) GetDistinctSources(f ActivityFilter) ([]SourceCount, error) {
	tiers := actTiers
	if f.Tier != "" {
		tiers = []string{f.Tier}
	}
	counts := make(map[string]int)
	for _, tier := range tiers {
		entries, err := s.scanTier(tier, f.Since, f.Until)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if matchesFilter(e, f) {
				counts[e.Source]++
			}
		}
	}
	out := make([]SourceCount, 0, len(counts))
	for src, cnt := range counts {
		out = append(out, SourceCount{Source: src, Count: cnt})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	return out, nil
}

// WipeAllActivity deletes every entry from all tier buckets. Returns total count.
func (s *PebbleActivityStore) WipeAllActivity() (int64, error) {
	var total int64
	for _, tier := range actTiers {
		kvs, err := s.scanTierKVs(tier, nil, nil)
		if err != nil {
			return total, err
		}
		for i := 0; i < len(kvs); i += 500 {
			end := i + 500
			if end > len(kvs) {
				end = len(kvs)
			}
			batch := s.db.NewBatch()
			for _, kv := range kvs[i:end] {
				if err := batch.Delete(kv.key, nil); err != nil {
					batch.Close()
					return total, fmt.Errorf("pebble_activity_store: wipe batch delete: %w", err)
				}
			}
			if err := batch.Commit(pebble.Sync); err != nil {
				batch.Close()
				return total, fmt.Errorf("pebble_activity_store: wipe batch commit: %w", err)
			}
			batch.Close()
			total += int64(end - i)
		}
	}
	return total, nil
}

// CompactByDay collapses all compactable tier entries into daily digest rows.
// Every tier except "digest" is eligible — newly-introduced tiers are automatically
// compacted without an allowlist update (denylist approach).
// Each day is processed atomically.
func (s *PebbleActivityStore) CompactByDay(ctx context.Context, olderThan time.Time) (CompactResult, error) {
	var result CompactResult

	// Load all compactable entries (all tiers except "digest").
	var all []pactKV
	for _, tier := range actCompactableTiers() {
		kvs, err := s.scanTierKVs(tier, nil, &olderThan)
		if err != nil {
			return result, err
		}
		all = append(all, kvs...)
	}
	if len(all) == 0 {
		return result, nil
	}

	// Group by date.
	type dayGroup struct{ kvs []pactKV }
	days := make(map[string]*dayGroup)
	var dayOrder []string
	for _, kv := range all {
		dk := kv.entry.Timestamp.UTC().Format("2006-01-02")
		if _, ok := days[dk]; !ok {
			days[dk] = &dayGroup{}
			dayOrder = append(dayOrder, dk)
		}
		days[dk].kvs = append(days[dk].kvs, kv)
	}
	sort.Strings(dayOrder)

	for _, dateKey := range dayOrder {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		dg := days[dateKey]
		entries := make([]ActivityEntry, len(dg.kvs))
		for i, kv := range dg.kvs {
			entries[i] = kv.entry
		}

		counts := make(map[string]int)
		for _, e := range entries {
			counts[e.Type]++
		}

		var auditItems, errItems, normalItems []DigestItem
		for _, e := range entries {
			item := DigestItem{
				Type:        e.Type,
				Tier:        e.Tier,
				Book:        extractBookName(e),
				BookID:      e.BookID,
				OperationID: e.OperationID,
				Summary:     extractItemSummary(e),
				Timestamp:   e.Timestamp,
				Tags:        e.Tags,
			}
			switch {
			case e.Tier == "audit":
				auditItems = append(auditItems, item)
			case e.Level == "error" || e.Level == "warn":
				item.Details = extractErrorDetails(e)
				errItems = append(errItems, item)
			default:
				normalItems = append(normalItems, item)
			}
		}
		items := append(auditItems, errItems...)
		items = append(items, normalItems...)

		truncated := false
		truncatedCount := 0
		if len(items) > maxDigestItems {
			truncatedCount = len(items) - maxDigestItems
			items = items[:maxDigestItems]
			truncated = true
		}

		dd := DigestDetails{
			Date:           dateKey,
			OriginalCount:  len(entries),
			Counts:         counts,
			Items:          items,
			Truncated:      truncated,
			TruncatedCount: truncatedCount,
		}

		// Check for existing digest for this date to merge into.
		existing, existingKey, err := s.findExistingDigest(dateKey)
		if err != nil {
			return result, err
		}
		if existingKey != nil {
			// Merge existing digest into dd.
			for k, v := range existing.Counts {
				dd.Counts[k] += v
			}
			dd.OriginalCount += existing.OriginalCount
			combined := append(existing.Items, dd.Items...)
			if existing.Truncated {
				dd.Truncated = true
				dd.TruncatedCount += existing.TruncatedCount
			}
			if len(combined) > maxDigestItems {
				dd.TruncatedCount += len(combined) - maxDigestItems
				combined = combined[:maxDigestItems]
				dd.Truncated = true
			}
			dd.Items = combined
		}

		detailsBytes, err := json.Marshal(dd)
		if err != nil {
			return result, fmt.Errorf("pebble_activity_store: compact marshal: %w", err)
		}

		startOfDay, err := time.Parse("2006-01-02", dateKey)
		if err != nil {
			return result, fmt.Errorf("pebble_activity_store: compact parse date: %w", err)
		}

		// Populate Details from the DigestDetails map so it survives the ActivityEntry round-trip.
		var ddMap map[string]any
		if mapErr := json.Unmarshal(detailsBytes, &ddMap); mapErr != nil {
			return result, fmt.Errorf("pebble_activity_store: compact unmarshal dd map: %w", mapErr)
		}
		digestID := ulid.Make().String()
		digest := ActivityEntry{
			ID:        s.counter.Add(1),
			Timestamp: startOfDay,
			Tier:      "digest",
			Type:      "daily_digest",
			Level:     "info",
			Source:    "compaction",
			Summary:   fmt.Sprintf("Daily digest for %s (%d entries)", dateKey, dd.OriginalCount),
			Details:   ddMap,
		}
		digestKey := pactPrimaryKey("digest", startOfDay, digestID)
		digestBytes, err := json.Marshal(digest)
		if err != nil {
			return result, fmt.Errorf("pebble_activity_store: compact marshal digest: %w", err)
		}

		batch := s.db.NewBatch()

		// Delete old digest if present.
		if existingKey != nil {
			if err := batch.Delete(existingKey, nil); err != nil {
				batch.Close()
				return result, fmt.Errorf("pebble_activity_store: compact delete old digest: %w", err)
			}
		}

		// Write new digest.
		if err := batch.Set(digestKey, digestBytes, nil); err != nil {
			batch.Close()
			return result, fmt.Errorf("pebble_activity_store: compact set digest: %w", err)
		}

		// Delete originals.
		for _, kv := range dg.kvs {
			if err := batch.Delete(kv.key, nil); err != nil {
				batch.Close()
				return result, fmt.Errorf("pebble_activity_store: compact delete original: %w", err)
			}
		}

		if err := batch.Commit(pebble.Sync); err != nil {
			batch.Close()
			return result, fmt.Errorf("pebble_activity_store: compact commit day %s: %w", dateKey, err)
		}
		batch.Close()

		result.DaysCompacted++
		result.EntriesDeleted += len(dg.kvs)
	}
	return result, nil
}

// MigrateSystemActivityLogs is a no-op for PebbleActivityStore since it's not backed by SQLite.
func (s *PebbleActivityStore) MigrateSystemActivityLogs() (int, error) {
	return 0, nil
}

// RecompactDigests re-derives type, tier, and tags on every stored daily-digest
// entry whose items were compacted before enrichment was added.
//
// Algorithm:
//  1. Range-scan the entire "act:digest:" prefix (all digest keys).
//  2. Decode each entry's Details map as DigestDetails.
//  3. Skip entries where no items are legacy (idempotent guard).
//  4. For each legacy item: call deriveTypeFromMessage + enrichLegacyLogTags.
//  5. Rebuild Counts and TagCounts, marshal back, and overwrite with the same key.
func (s *PebbleActivityStore) RecompactDigests(ctx context.Context) (RecompactResult, error) {
	var result RecompactResult

	type digestKV struct {
		key   []byte
		entry ActivityEntry
		dd    DigestDetails
	}

	var candidates []digestKV

	// Scan all digest entries.
	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: pactPrimaryPrefix("digest"),
		UpperBound: pactPrimaryUpperBound("digest"),
	})
	if err != nil {
		return result, fmt.Errorf("pebble_activity_store: recompact new iter: %w", err)
	}
	for iter.First(); iter.Valid(); iter.Next() {
		var e ActivityEntry
		if jsonErr := json.Unmarshal(iter.Value(), &e); jsonErr != nil {
			continue
		}
		if e.Type != "daily_digest" {
			continue
		}
		var dd DigestDetails
		if e.Details != nil {
			if b, merr := json.Marshal(e.Details); merr == nil {
				_ = json.Unmarshal(b, &dd)
			}
		}
		keyCopy := make([]byte, len(iter.Key()))
		copy(keyCopy, iter.Key())
		candidates = append(candidates, digestKV{key: keyCopy, entry: e, dd: dd})
	}
	if err := iter.Close(); err != nil {
		return result, fmt.Errorf("pebble_activity_store: recompact iter close: %w", err)
	}

	slog.Info("[activity] recompact: starting digest re-derivation (pebble)",
		"digest_count", len(candidates))

	for _, c := range candidates {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		// Check if any items need updating.
		needsUpdate := false
		for _, item := range c.dd.Items {
			if isLegacyItem(item) {
				needsUpdate = true
				break
			}
		}
		if !needsUpdate {
			result.Skipped++
			continue
		}

		// Re-derive type, tier, and tags on each legacy item.
		for i, item := range c.dd.Items {
			if !isLegacyItem(item) {
				continue
			}
			derivedType, derivedTier := deriveTypeFromMessage(item.Summary, "")
			derivedTags := enrichLegacyLogTags(item.Summary, "", "info")
			c.dd.Items[i].Type = derivedType
			c.dd.Items[i].Tier = derivedTier
			c.dd.Items[i].Tags = derivedTags
		}

		// Rebuild Counts from updated items.
		newCounts := make(map[string]int)
		for _, item := range c.dd.Items {
			newCounts[item.Type]++
		}
		c.dd.Counts = newCounts

		// Rebuild TagCounts from updated items (action: and source: namespaces only).
		newTagCounts := make(map[string]map[string]int)
		for _, item := range c.dd.Items {
			for _, tag := range item.Tags {
				colonIdx := strings.Index(tag, ":")
				if colonIdx < 1 {
					continue
				}
				ns := tag[:colonIdx]
				val := tag[colonIdx+1:]
				if ns != "action" && ns != "source" {
					continue
				}
				if newTagCounts[ns] == nil {
					newTagCounts[ns] = make(map[string]int)
				}
				newTagCounts[ns][val]++
			}
		}
		if len(newTagCounts) > 0 {
			c.dd.TagCounts = newTagCounts
		}

		// Merge updated DigestDetails back into the entry's Details map.
		ddBytes, merr := json.Marshal(c.dd)
		if merr != nil {
			return result, fmt.Errorf("pebble_activity_store: recompact marshal digest key=%s: %w", c.key, merr)
		}
		var detailsMap map[string]any
		if err := json.Unmarshal(ddBytes, &detailsMap); err != nil {
			return result, fmt.Errorf("pebble_activity_store: recompact unmarshal detailsMap key=%s: %w", c.key, err)
		}
		c.entry.Details = detailsMap
		c.entry.Summary = fmt.Sprintf("Daily digest for %s (%d entries)", c.dd.Date, c.dd.OriginalCount)

		entryBytes, merr := json.Marshal(c.entry)
		if merr != nil {
			return result, fmt.Errorf("pebble_activity_store: recompact marshal entry key=%s: %w", c.key, merr)
		}

		if err := s.db.Set(c.key, entryBytes, pebble.Sync); err != nil {
			return result, fmt.Errorf("pebble_activity_store: recompact write key=%s: %w", c.key, err)
		}

		slog.Info("[activity] recompact: updated digest (pebble)",
			"key", string(c.key), "date", c.dd.Date, "items", len(c.dd.Items))
		result.Touched++
	}

	slog.Info("[activity] recompact: complete (pebble)",
		"touched", result.Touched, "skipped", result.Skipped)
	return result, nil
}

// ── internal helpers ──────────────────────────────────────────────────────────

// pactKV holds a Pebble key and its decoded ActivityEntry.
type pactKV struct {
	key   []byte
	entry ActivityEntry
}

// scanTier returns all entries from a tier within [since, until].
// nil bounds mean "no bound". Results are in ascending timestamp order (Pebble
// lexicographic order over the time-keyed prefix).
func (s *PebbleActivityStore) scanTier(tier string, since, until *time.Time) ([]ActivityEntry, error) {
	kvs, err := s.scanTierKVs(tier, since, until)
	if err != nil {
		return nil, err
	}
	entries := make([]ActivityEntry, len(kvs))
	for i, kv := range kvs {
		entries[i] = kv.entry
	}
	return entries, nil
}

// scanTierKVs returns key+entry pairs for a tier within the time range.
func (s *PebbleActivityStore) scanTierKVs(tier string, since, until *time.Time) ([]pactKV, error) {
	// Default lower/upper bounds cover the entire tier prefix.
	lower := pactPrimaryPrefix(tier)
	upper := pactPrimaryUpperBound(tier)

	// Narrow bounds when time constraints are given.
	if since != nil {
		lower = pactPrimaryKey(tier, *since, "")
	}
	if until != nil {
		upper = pactPrimaryKey(tier, *until, "\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff")
	}

	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: lower,
		UpperBound: upper,
	})
	if err != nil {
		return nil, fmt.Errorf("pebble_activity_store: scanTierKVs new iter (tier=%s): %w", tier, err)
	}
	defer iter.Close()

	var out []pactKV
	for iter.First(); iter.Valid(); iter.Next() {
		var e ActivityEntry
		if jsonErr := json.Unmarshal(iter.Value(), &e); jsonErr != nil {
			continue
		}
		keyCopy := make([]byte, len(iter.Key()))
		copy(keyCopy, iter.Key())
		out = append(out, pactKV{key: keyCopy, entry: e})
	}
	return out, nil
}

// queryByIndexPrefix reads ref values from an index prefix, then fetches primary entries.
// Handles both op and book secondary indexes.
func (s *PebbleActivityStore) queryByIndexPrefix(prefix string, f ActivityFilter) ([]ActivityEntry, int, error) {
	// Upper bound: replace trailing ':' with ';' to cover the entire id sub-namespace.
	upperPrefix := prefix[:len(prefix)-1] + ";"

	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte(prefix),
		UpperBound: []byte(upperPrefix),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("pebble_activity_store: queryByIndex new iter: %w", err)
	}
	defer iter.Close()

	var refs [][]byte
	for iter.First(); iter.Valid(); iter.Next() {
		valCopy := make([]byte, len(iter.Value()))
		copy(valCopy, iter.Value())
		refs = append(refs, valCopy)
	}

	var all []ActivityEntry
	for _, ref := range refs {
		primaryKey, ok := pactPrimaryKeyFromRef(ref)
		if !ok {
			continue
		}
		val, closer, err := s.db.Get(primaryKey)
		if err != nil {
			// Entry may have been deleted (e.g., pruned); skip stale index refs.
			continue
		}
		var entry ActivityEntry
		jsonErr := json.Unmarshal(val, &entry)
		closer.Close()
		if jsonErr != nil {
			continue
		}
		if matchesFilter(entry, f) {
			all = append(all, entry)
		}
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].Timestamp.After(all[j].Timestamp)
	})

	total := len(all)
	start := f.Offset
	if start > len(all) {
		start = len(all)
	}
	end := start + f.Limit
	if end > len(all) {
		end = len(all)
	}
	return all[start:end], total, nil
}

// findExistingDigest looks for a digest row for the given date string ("2006-01-02").
// Returns the DigestDetails, the Pebble key, and any error.
func (s *PebbleActivityStore) findExistingDigest(dateKey string) (DigestDetails, []byte, error) {
	day, err := time.Parse("2006-01-02", dateKey)
	if err != nil {
		return DigestDetails{}, nil, err
	}
	dayEnd := day.Add(24 * time.Hour)

	lower := pactPrimaryKey("digest", day, "")
	upper := pactPrimaryKey("digest", dayEnd, "")

	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: lower,
		UpperBound: upper,
	})
	if err != nil {
		return DigestDetails{}, nil, fmt.Errorf("pebble_activity_store: findExistingDigest new iter: %w", err)
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		var row struct {
			ActivityEntry
			DigestDetails json.RawMessage `json:"digest_details,omitempty"`
		}
		if jsonErr := json.Unmarshal(iter.Value(), &row); jsonErr != nil {
			continue
		}
		if row.ActivityEntry.Type != "daily_digest" {
			continue
		}

		var dd DigestDetails
		if row.DigestDetails != nil {
			// Old format: digest_details at top level.
			_ = json.Unmarshal(row.DigestDetails, &dd)
		} else if row.ActivityEntry.Details != nil {
			// New format: stored in ActivityEntry.Details.
			if b, merr := json.Marshal(row.ActivityEntry.Details); merr == nil {
				_ = json.Unmarshal(b, &dd)
			}
		}
		keyCopy := make([]byte, len(iter.Key()))
		copy(keyCopy, iter.Key())
		return dd, keyCopy, nil
	}
	return DigestDetails{}, nil, nil
}
