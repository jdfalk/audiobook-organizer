// file: internal/auth/context.go
// version: 1.0.0
// guid: 8c4a2f1d-9b3e-4f60-a8d5-2c7e0f1b9a47
//
// Request-scoped auth state plumbing (spec 3.7). Long-lived deps
// (database.Store, services) live on the Server struct; per-request
// state (who is calling, what they can do) flows through
// context.Context using typed helpers so handlers can't accidentally
// read them as strings.

package auth

import (
	"context"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

type ctxKey int

const (
	userKey ctxKey = iota
	permissionsKey
)

// WithUser attaches the calling user to ctx. Typically set by the
// authenticate middleware after session/JWT verification.
func WithUser(ctx context.Context, u *database.User) context.Context {
	if u == nil {
		return ctx
	}
	return context.WithValue(ctx, userKey, u)
}

// UserFromContext returns the user attached to ctx by WithUser, or
// (nil, false) if no user is set (unauthenticated request or a
// handler that bypassed auth middleware).
func UserFromContext(ctx context.Context) (*database.User, bool) {
	if ctx == nil {
		return nil, false
	}
	u, ok := ctx.Value(userKey).(*database.User)
	return u, ok && u != nil
}

// WithPermissions attaches the calling user's flattened permission
// set to ctx. Computed at session creation by unioning the role
// permissions and cached on the session blob; middleware copies it
// into request context so Can() is a cheap map lookup.
func WithPermissions(ctx context.Context, perms []Permission) context.Context {
	if len(perms) == 0 {
		return ctx
	}
	set := make(map[Permission]struct{}, len(perms))
	for _, p := range perms {
		set[p] = struct{}{}
	}
	return context.WithValue(ctx, permissionsKey, set)
}

// PermissionsFromContext returns the permission set attached to ctx,
// or nil if no permissions have been loaded (which effectively means
// the caller has none).
func PermissionsFromContext(ctx context.Context) map[Permission]struct{} {
	if ctx == nil {
		return nil
	}
	set, _ := ctx.Value(permissionsKey).(map[Permission]struct{})
	return set
}

// Can reports whether the caller in ctx has permission p.
//
// Returns false for an unauthenticated caller (no permissions set)
// and for an authenticated caller whose roles do not include p.
// Never panics — unset context or nil permission set both yield
// false, which is the safe default.
func Can(ctx context.Context, p Permission) bool {
	set := PermissionsFromContext(ctx)
	if set == nil {
		return false
	}
	_, ok := set[p]
	return ok
}
