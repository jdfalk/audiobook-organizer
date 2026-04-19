// file: internal/itunes/service/store.go
// version: 1.0.0
// guid: 4f9bbf9f-0d28-46d5-be9c-e9ce3a422593

// Package itunesservice contains the iTunes integration: import pipeline,
// ITL write-back batcher, position sync, path reconcile, playlist sync,
// track provisioner, and ITL transfer. The low-level ITL parser, fingerprint,
// path mapping, and smart-criteria translator live in the parent package
// internal/itunes and are untouched by this extraction.
//
// See docs/superpowers/specs/2026-04-18-itunes-service-extraction-design.md.
package itunesservice

import "github.com/jdfalk/audiobook-organizer/internal/database"

// Store is the narrow slice of database.Store that the iTunes service
// uses. Wide because iTunes is a hub — books, authors, series, files,
// tags, external IDs, operations, preferences, playlists, fingerprints
// — but still smaller than full database.Store.
type Store interface {
	database.BookStore
	database.AuthorStore
	database.SeriesStore
	database.NarratorStore
	database.BookFileStore
	database.HashBlocklistStore
	database.ITunesStateStore
	database.ExternalIDStore
	database.UserPositionStore
	database.UserPlaylistStore
	database.UserPreferenceStore
	database.OperationStore
	database.SettingsStore
	database.MetadataStore
	database.TagStore
	database.RawKVStore
}
