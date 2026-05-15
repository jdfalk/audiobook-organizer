// file: tools/cmd/sdkguard/main.go
// version: 1.0.0
// guid: e8f9a0b1-c2d3-4567-e890-f12345678901
// last-edited: 2026-05-08

// Package main implements sdkguard, a CI tool that asserts pkg/plugin/sdk has
// no unexpected internal/ dependencies.
//
// The SDK is a stable public contract backed by type aliases into
// internal/operations/registry and internal/auth. Those "allowed internals" are
// part of the SDK's own implementation contract and are explicitly whitelisted.
// Any OTHER internal/ import appearing in the dependency tree of pkg/plugin/sdk
// is a violation — it means a new internal dependency was added without review.
//
// Usage:
//
//	go run ./tools/cmd/sdkguard/main.go            # check pkg/plugin/sdk
//	go run ./tools/cmd/sdkguard/main.go -module=./pkg/plugin/sdk/...
//
// Exit codes:
//
//	0 — all clear
//	1 — one or more forbidden internal/ packages found in the dep tree
//	2 — usage / invocation error
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// module is the Go pattern passed to `go list -deps`.
const defaultModule = "./pkg/plugin/sdk/..."

// allowedInternals lists the internal/ packages that are part of the SDK's
// stable backplane. These are the type-alias targets (internal/operations/registry,
// internal/auth) and their own transitive dependencies. Any internal/ package
// NOT in this set is a forbidden accretion.
//
// To update this list: run `go list -deps ./pkg/plugin/sdk/... | grep '/internal/'`
// and verify each entry is an approved backplane package, then add it here.
var allowedInternals = map[string]bool{
	"github.com/jdfalk/audiobook-organizer/internal/operations/registry": true,
	"github.com/jdfalk/audiobook-organizer/internal/auth":                true,
	"github.com/jdfalk/audiobook-organizer/internal/models":              true,
	"github.com/jdfalk/audiobook-organizer/internal/database":            true,
	"github.com/jdfalk/audiobook-organizer/internal/metrics":             true,
	"github.com/jdfalk/audiobook-organizer/internal/util":                true,
	"github.com/jdfalk/audiobook-organizer/internal/fingerprint":         true,
	"github.com/jdfalk/audiobook-organizer/internal/matcher":             true,
	// Added to allow pkg/plugin/sdk to reference the service registry backplane
	// which is part of the SDK's supported stable surface.
	"github.com/jdfalk/audiobook-organizer/internal/serviceregistry":     true,
}

func main() {
	moduleFlag := flag.String("module", defaultModule, "Go package pattern to inspect")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: sdkguard [-module=<pattern>]\n\n")
		fmt.Fprintf(os.Stderr, "Asserts that pkg/plugin/sdk has no unexpected internal/ dependencies.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	violations, err := run(*moduleFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sdkguard: error: %v\n", err)
		os.Exit(2)
	}

	if len(violations) > 0 {
		fmt.Fprintf(os.Stderr, "sdkguard: FAIL — forbidden internal/ packages in %s dep tree:\n\n", *moduleFlag)
		for _, v := range violations {
			fmt.Fprintf(os.Stderr, "  %s\n", v)
		}
		fmt.Fprintf(os.Stderr, "\nTo fix: remove the import or add it to the allowedInternals list in\n")
		fmt.Fprintf(os.Stderr, "tools/cmd/sdkguard/main.go (with a comment explaining why it is allowed).\n")
		os.Exit(1)
	}

	fmt.Printf("sdkguard: OK — %s has no forbidden internal/ dependencies\n", *moduleFlag)
}

// run shells out to `go list -deps <module>` and scans the output for
// project-local internal/ packages not on the allowlist. It returns a slice
// of violation strings (one per forbidden package).
func run(module string) ([]string, error) {
	cmd := exec.Command("go", "list", "-deps", module)
	cmd.Env = os.Environ()
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list -deps %s: %w\n%s", module, err, string(out))
	}

	const modulePrefix = "github.com/jdfalk/audiobook-organizer/internal/"
	var violations []string
	for _, line := range strings.Split(string(out), "\n") {
		pkg := strings.TrimSpace(line)
		if !strings.HasPrefix(pkg, modulePrefix) {
			continue
		}
		if !allowedInternals[pkg] {
			violations = append(violations, pkg)
		}
	}
	return violations, nil
}
