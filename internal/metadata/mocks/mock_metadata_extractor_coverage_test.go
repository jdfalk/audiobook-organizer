package mocks

import (
	"reflect"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/metadata"
	"github.com/stretchr/testify/mock"
)

func TestMockMetadataExtractor_Coverage(t *testing.T) {
	m := NewMockMetadataExtractor(t)
	exp := m.EXPECT()
	ifaceType := reflect.TypeOf((*metadata.MetadataExtractor)(nil)).Elem()

	for i := 0; i < ifaceType.NumMethod(); i++ {
		method := ifaceType.Method(i)
		name := method.Name

		matcherArgs := make([]reflect.Value, 0, method.Type.NumIn())
		for j := 0; j < method.Type.NumIn(); j++ {
			matcherArgs = append(matcherArgs, reflect.ValueOf(mock.Anything))
		}

		expMethod := reflect.ValueOf(exp).MethodByName(name)
		if !expMethod.IsValid() {
			t.Fatalf("EXPECT() missing method %s", name)
		}
		callVals := expMethod.Call(matcherArgs)
		if len(callVals) != 1 {
			t.Fatalf("EXPECT().%s returned %d values, expected 1", name, len(callVals))
		}
		call := callVals[0]

		runMethod := call.MethodByName("Run")
		if runMethod.IsValid() {
			runFuncType := runMethod.Type().In(0)
			runFn := reflect.MakeFunc(runFuncType, func(_ []reflect.Value) []reflect.Value { return nil })
			runMethod.Call([]reflect.Value{runFn})
		}

		returnMethod := call.MethodByName("Return")
		if !returnMethod.IsValid() {
			t.Fatalf("EXPECT().%s missing Return(...)", name)
		}
		returnArgs := make([]reflect.Value, 0, method.Type.NumOut())
		for j := 0; j < method.Type.NumOut(); j++ {
			returnArgs = append(returnArgs, reflect.Zero(method.Type.Out(j)))
		}
		returnMethod.Call(returnArgs)

		methodVal := reflect.ValueOf(m).MethodByName(name)
		if !methodVal.IsValid() {
			t.Fatalf("MockMetadataExtractor missing method %s", name)
		}
		callArgs := make([]reflect.Value, 0, method.Type.NumIn())
		for j := 0; j < method.Type.NumIn(); j++ {
			callArgs = append(callArgs, reflect.Zero(method.Type.In(j)))
		}
		_ = methodVal.Call(callArgs)
	}
}
