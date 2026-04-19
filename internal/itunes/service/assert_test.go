// file: internal/itunes/service/assert_test.go
// version: 1.0.0
// guid: bdd01af3-56b2-4412-93b5-25c65ddb02ab

package itunesservice_test

import (
	"github.com/jdfalk/audiobook-organizer/internal/database"
	itunesservice "github.com/jdfalk/audiobook-organizer/internal/itunes/service"
)

// Compile-time proof that *database.PebbleStore satisfies
// itunesservice.Store. If a method is renamed or removed from PebbleStore,
// the assertion below fails to build — we find out here rather than at
// the Server wiring step.
var _ itunesservice.Store = (*database.PebbleStore)(nil)
