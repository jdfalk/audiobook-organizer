// file: internal/database/sqlite_acoustid_stats.go
// version: 1.0.0
// guid: bbccddeeff-0011-2233-4455-66778899aabb
// last-edited: 2026-06-04

package database

import "database/sql"

func hasAcoustIDNullSegments(segments ...sql.NullString) bool {
	for _, seg := range segments {
		if seg.Valid && seg.String != "" {
			return true
		}
	}
	return false
}
