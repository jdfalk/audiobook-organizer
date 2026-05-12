// file: internal/serviceregistry/registry_test.go
// version: 1.0.0

package serviceregistry

import (
	"strings"
	"testing"
)

func TestRegister_DuplicatePanics(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	Register(ServiceDef{Name: "x", Build: func(c *Container) (any, error) { return nil, nil }})

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on duplicate Register")
		}
		msg, ok := r.(string)
		if !ok || !strings.Contains(msg, "duplicate") {
			t.Errorf("panic msg = %v, expected duplicate", r)
		}
	}()
	Register(ServiceDef{Name: "x", Build: func(c *Container) (any, error) { return nil, nil }})
}

func TestRegister_RequiresName(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	defer func() { _ = recover() }()
	Register(ServiceDef{Build: func(c *Container) (any, error) { return nil, nil }})
	t.Fatal("expected panic on missing Name")
}

func TestRegister_RequiresBuild(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	defer func() { _ = recover() }()
	Register(ServiceDef{Name: "x"})
	t.Fatal("expected panic on missing Build")
}
