// Code generated - supplemental manual additions for new Store method
// DO NOT EDIT (this file is intentionally hand-written to augment generated mocks)

package mocks

import (
	"context"
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
