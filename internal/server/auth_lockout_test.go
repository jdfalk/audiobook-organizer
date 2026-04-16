// file: internal/server/auth_lockout_test.go
// version: 1.0.0
// guid: 8c4e5f3a-9b5a-4a70-b8c5-3d7e0f1b9a99

package server

import "testing"

func TestLockout_TriggersAfterMaxFailures(t *testing.T) {
	uid := "test-lockout-user"
	clearFailedLogins(uid)
	defer clearFailedLogins(uid)

	for i := 0; i < maxFailedLogins; i++ {
		if isLockedOut(uid) {
			t.Fatalf("locked out after only %d attempts, want %d", i, maxFailedLogins)
		}
		recordFailedLogin(uid)
	}
	if !isLockedOut(uid) {
		t.Errorf("should be locked out after %d failures", maxFailedLogins)
	}
}

func TestLockout_ClearedOnSuccess(t *testing.T) {
	uid := "test-clear-user"
	clearFailedLogins(uid)
	defer clearFailedLogins(uid)

	for i := 0; i < maxFailedLogins-1; i++ {
		recordFailedLogin(uid)
	}
	clearFailedLogins(uid)
	if isLockedOut(uid) {
		t.Error("should not be locked out after clear")
	}
}

func TestLockout_NoLockoutForUnknownUser(t *testing.T) {
	if isLockedOut("never-seen-user-123") {
		t.Error("unknown user should not be locked out")
	}
}
