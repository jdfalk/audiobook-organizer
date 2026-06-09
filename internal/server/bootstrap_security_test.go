// file: internal/server/bootstrap_security_test.go
// version: 1.0.1
// guid: 2b9d4e6a-1c3f-4a8b-bd72-5e0a9c4f1d36
// last-edited: 2026-06-09

package server

import (
	"fmt"
	"testing"

	"github.com/falkcorp/audiobook-organizer/internal/database"
)

// fakeSettingsStore is a minimal in-memory SettingsReadWriter that mirrors the
// real backends' contract: a missing key returns ErrSettingNotFound (wrapped).
type fakeSettingsStore struct {
	m map[string]string
}

func newFakeSettingsStore() *fakeSettingsStore {
	return &fakeSettingsStore{m: map[string]string{}}
}

func (f *fakeSettingsStore) GetSetting(key string) (*database.Setting, error) {
	v, ok := f.m[key]
	if !ok {
		return nil, fmt.Errorf("setting not found: %s: %w", key, database.ErrSettingNotFound)
	}
	return &database.Setting{Key: key, Value: v}, nil
}

func (f *fakeSettingsStore) SetSetting(key, value, _ string, _ bool) error {
	f.m[key] = value
	return nil
}

func (f *fakeSettingsStore) DeleteSetting(key string) error {
	delete(f.m, key)
	return nil
}

// HIGH-4a: once the one-time token is consumed (key deleted), GetSetting returns
// ErrSettingNotFound. ConsumeBootstrapToken must treat that as an invalid token
// — (false, nil) — so the handler returns 401, not 500.
func TestConsumeBootstrapToken_ConsumedTokenIsUnauthorizedNotError(t *testing.T) {
	store := newFakeSettingsStore() // empty — token key absent (already consumed)
	dataDir := t.TempDir()

	valid, err := ConsumeBootstrapToken(store, dataDir, "abbs_anything")
	if err != nil {
		t.Fatalf("expected nil error for a consumed token, got: %v", err)
	}
	if valid {
		t.Fatal("expected valid=false for a consumed token")
	}
}

// A wrong token (key present, hash mismatch) is also (false, nil), not an error.
func TestConsumeBootstrapToken_WrongTokenIsUnauthorized(t *testing.T) {
	store := newFakeSettingsStore()
	_ = store.SetSetting(bootstrapTokenKey, hashBootstrapToken("abbs_correct"), "string", false)
	dataDir := t.TempDir()

	valid, err := ConsumeBootstrapToken(store, dataDir, "abbs_wrong")
	if err != nil {
		t.Fatalf("expected nil error for a wrong token, got: %v", err)
	}
	if valid {
		t.Fatal("expected valid=false for a wrong token")
	}
}

// The happy path still works: a matching token consumes successfully.
func TestConsumeBootstrapToken_CorrectTokenConsumes(t *testing.T) {
	store := newFakeSettingsStore()
	_ = store.SetSetting(bootstrapTokenKey, hashBootstrapToken("abbs_correct"), "string", false)
	dataDir := t.TempDir()

	valid, err := ConsumeBootstrapToken(store, dataDir, "abbs_correct")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !valid {
		t.Fatal("expected valid=true for the correct token")
	}
	// Token must be gone after consumption (single-use).
	if _, err := store.GetSetting(bootstrapTokenKey); err == nil {
		t.Fatal("expected token key to be deleted after consumption")
	}
}
