// file: internal/operations/selection.go
// version: 1.0.0
// guid: a2b3c4d5-e6f7-8901-bcde-f12345678901
// last-edited: 2026-05-11
//
// ResolveBookIDs resolves a SelectionSpec to a concrete list of book IDs.
// When the spec carries a FilterSpec the caller-provided resolve func is
// invoked; otherwise the explicit BookIDs slice is returned as-is.

package operations

// ResolveBookIDs resolves a SelectionSpec to a list of book IDs.
// The resolve func must return IDs of primary-version books matching the
// FilterSpec. It is only called when spec.Filter is non-nil; callers that
// supply a filter must ensure the func applies IsPrimaryVersion=true.
func ResolveBookIDs(spec SelectionSpec, resolve func(FilterSpec) ([]string, error)) ([]string, error) {
	if spec.Filter != nil {
		return resolve(*spec.Filter)
	}
	return spec.BookIDs, nil
}
