// file: internal/server/version_lifecycle_prop_test.go
// version: 1.2.0
// guid: d4c4cd2b-c578-4a11-8229-83a516271b1b

// Property-based tests for BookVersion lifecycle transitions (spec 4.5 task 6).
//
// These tests are kept in the server package as integration tests that verify
// the handler-level behavior still works with the extracted versions package.
// The canonical property tests now live in internal/versions/lifecycle_prop_test.go.
//
// This file is retained for backward compatibility and can be removed once
// the versions package tests are confirmed sufficient.

package server

// Property tests have been moved to internal/versions/lifecycle_prop_test.go.
// The server package tests in version_lifecycle_test.go cover the HTTP handler
// integration with the versions package.
