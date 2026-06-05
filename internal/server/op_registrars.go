// file: internal/server/op_registrars.go
// version: 1.0.0
// guid: e1f2a3b4-c5d6-7e8f-9a0b-1c2d3e4f5a6b

// op_registrars provides a zero-conflict mechanism for registering v2 OperationDefs.
// Each op file calls addOpRegistrar in an init() function; server.go iterates the
// slice at startup. New ops never require touching server.go.

package server

import opsregistry "github.com/falkcorp/audiobook-organizer/internal/operations/registry"

// opRegistrar is a function that registers one or more OperationDefs with the
// UOS-02 registry.
type opRegistrar func(s *Server, reg *opsregistry.Registry) error

var opRegistrars []opRegistrar

// addOpRegistrar appends a registration function to the global slice.
// Call from init() in any server package file to wire up a new op without
// modifying server.go.
func addOpRegistrar(fn opRegistrar) {
	opRegistrars = append(opRegistrars, fn)
}
