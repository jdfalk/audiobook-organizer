// Code generated - supplemental manual additions for new Store method
// DO NOT EDIT (this file is intentionally hand-written to augment generated mocks)

package mocks

import (
	"context"

	mock "github.com/stretchr/testify/mock"
)

// MarkFileImportedFromDeluge provides a mock function for the type MockBookFileStore
func (_mock *MockBookFileStore) MarkFileImportedFromDeluge(ctx context.Context, originalPath, libraryPath, torrentHash string) error {
	ret := _mock.Called(ctx, originalPath, libraryPath, torrentHash)

	if len(ret) == 0 {
		panic("no return value specified for MarkFileImportedFromDeluge")
	}

	if rf, ok := ret.Get(0).(func(context.Context, string, string, string) error); ok {
		return rf(ctx, originalPath, libraryPath, torrentHash)
	}

	return ret.Error(0)
}

// MarkFileImportedFromDeluge provides a mock function for the type MockStore
func (_mock *MockStore) MarkFileImportedFromDeluge(ctx context.Context, originalPath, libraryPath, torrentHash string) error {
	ret := _mock.Called(ctx, originalPath, libraryPath, torrentHash)

	if len(ret) == 0 {
		panic("no return value specified for MarkFileImportedFromDeluge")
	}

	if rf, ok := ret.Get(0).(func(context.Context, string, string, string) error); ok {
		return rf(ctx, originalPath, libraryPath, torrentHash)
	}

	return ret.Error(0)
}

// MockBookFileStore_MarkFileImportedFromDeluge_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'MarkFileImportedFromDeluge'
type MockBookFileStore_MarkFileImportedFromDeluge_Call struct {
	*mock.Call
}

// MarkFileImportedFromDeluge is a helper method to define mock.On call
//   - ctx context.Context
//   - originalPath string
//   - libraryPath string
//   - torrentHash string
func (_e *MockBookFileStore_Expecter) MarkFileImportedFromDeluge(ctx interface{}, originalPath interface{}, libraryPath interface{}, torrentHash interface{}) *MockBookFileStore_MarkFileImportedFromDeluge_Call {
	return &MockBookFileStore_MarkFileImportedFromDeluge_Call{Call: _e.mock.On("MarkFileImportedFromDeluge", ctx, originalPath, libraryPath, torrentHash)}
}

func (_c *MockBookFileStore_MarkFileImportedFromDeluge_Call) Run(run func(ctx context.Context, originalPath string, libraryPath string, torrentHash string)) *MockBookFileStore_MarkFileImportedFromDeluge_Call {
	_c.Call.Run(func(args mock.Arguments) {
		var arg0 context.Context
		if args[0] != nil {
			arg0 = args[0].(context.Context)
		}
		var arg1 string
		if args[1] != nil {
			arg1 = args[1].(string)
		}
		var arg2 string
		if args[2] != nil {
			arg2 = args[2].(string)
		}
		var arg3 string
		if args[3] != nil {
			arg3 = args[3].(string)
		}
		run(
			arg0,
			arg1,
			arg2,
			arg3,
		)
	})
	return _c
}

func (_c *MockBookFileStore_MarkFileImportedFromDeluge_Call) Return(err error) *MockBookFileStore_MarkFileImportedFromDeluge_Call {
	_c.Call.Return(err)
	return _c
}

func (_c *MockBookFileStore_MarkFileImportedFromDeluge_Call) RunAndReturn(run func(ctx context.Context, originalPath string, libraryPath string, torrentHash string) error) *MockBookFileStore_MarkFileImportedFromDeluge_Call {
	_c.Call.Return(run)
	return _c
}

// MockStore_MarkFileImportedFromDeluge_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'MarkFileImportedFromDeluge'
type MockStore_MarkFileImportedFromDeluge_Call struct {
	*mock.Call
}

// MarkFileImportedFromDeluge is a helper method to define mock.On call
//   - ctx context.Context
//   - originalPath string
//   - libraryPath string
//   - torrentHash string
func (_e *MockStore_Expecter) MarkFileImportedFromDeluge(ctx interface{}, originalPath interface{}, libraryPath interface{}, torrentHash interface{}) *MockStore_MarkFileImportedFromDeluge_Call {
	return &MockStore_MarkFileImportedFromDeluge_Call{Call: _e.mock.On("MarkFileImportedFromDeluge", ctx, originalPath, libraryPath, torrentHash)}
}

func (_c *MockStore_MarkFileImportedFromDeluge_Call) Run(run func(ctx context.Context, originalPath string, libraryPath string, torrentHash string)) *MockStore_MarkFileImportedFromDeluge_Call {
	_c.Call.Run(func(args mock.Arguments) {
		var arg0 context.Context
		if args[0] != nil {
			arg0 = args[0].(context.Context)
		}
		var arg1 string
		if args[1] != nil {
			arg1 = args[1].(string)
		}
		var arg2 string
		if args[2] != nil {
			arg2 = args[2].(string)
		}
		var arg3 string
		if args[3] != nil {
			arg3 = args[3].(string)
		}
		run(
			arg0,
			arg1,
			arg2,
			arg3,
		)
	})
	return _c
}

func (_c *MockStore_MarkFileImportedFromDeluge_Call) Return(err error) *MockStore_MarkFileImportedFromDeluge_Call {
	_c.Call.Return(err)
	return _c
}

func (_c *MockStore_MarkFileImportedFromDeluge_Call) RunAndReturn(run func(ctx context.Context, originalPath string, libraryPath string, torrentHash string) error) *MockStore_MarkFileImportedFromDeluge_Call {
	_c.Call.Return(run)
	return _c
}
