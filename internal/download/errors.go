// file: internal/download/errors.go
// version: 1.0.0
// guid: c82e3b94-2ab9-469d-a2ed-16a28525b03d

package download

import "errors"

// ErrNotImplemented signals that a client adapter is not implemented yet.
var ErrNotImplemented = errors.New("download client not implemented")
