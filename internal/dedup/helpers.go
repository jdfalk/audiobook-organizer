// file: internal/dedup/helpers.go
// version: 1.0.0
// guid: b2c3d4e5-f6a7-8901-bcde-f12345678901

package dedup

// derefStr safely dereferences a *string, returning "" for nil.
func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
