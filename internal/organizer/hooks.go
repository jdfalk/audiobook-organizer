// file: internal/organizer/hooks.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

package organizer

// OrganizeHooks provides optional callbacks for organize-time side effects.
type OrganizeHooks interface {
	OnCollision(currentBookID, occupantPath string)
}
