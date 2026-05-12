// file: internal/serviceregistry/errors.go
// version: 1.0.0

package serviceregistry

import "errors"

var (
	ErrCycle          = errors.New("serviceregistry: dependency cycle")
	ErrUnknownService = errors.New("serviceregistry: unknown service")
	ErrUndeclaredDep  = errors.New("serviceregistry: undeclared dependency")
	ErrNotBuilt       = errors.New("serviceregistry: service not built")
	ErrTypeMismatch   = errors.New("serviceregistry: type mismatch")
	ErrWrongPhase     = errors.New("serviceregistry: operation called in wrong phase")
)
