// file: internal/server/handlers/auth_test.go
// version: 1.0.0
// guid: d5e6f7a8-b9c0-1234-5678-90abcdef0123
// last-edited: 2026-06-01

package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/server/handlers"
	handlersmocks "github.com/jdfalk/audiobook-organizer/internal/server/handlers/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func newAuthCtx(method, path string, body any) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	var b []byte
	if body != nil {
		b, _ = json.Marshal(body)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	return c, w
}

func setAuthUser(c *gin.Context, user *database.User) {
	c.Set("auth_user", user)
}

func setAuthSession(c *gin.Context, session *database.Session) {
	c.Set("auth_session", session)
}

func testBcryptHash(t *testing.T, password string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	require.NoError(t, err)
	return string(hash)
}

func TestAuthHandler_GetStatus_NoUsers(t *testing.T) {
	store := handlersmocks.NewMockAuthStore(t)
	store.EXPECT().CountUsers().Return(0, nil)

	h := handlers.NewAuthHandler(store, true)
	c, w := newAuthCtx("GET", "/auth/status", nil)
	h.GetStatus(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	assert.Equal(t, false, data["has_users"])
	assert.Equal(t, true, data["bootstrap_ready"])
	assert.Equal(t, false, data["requires_auth"])
}

func TestAuthHandler_GetStatus_HasUsers(t *testing.T) {
	store := handlersmocks.NewMockAuthStore(t)
	store.EXPECT().CountUsers().Return(3, nil)

	h := handlers.NewAuthHandler(store, true)
	c, w := newAuthCtx("GET", "/auth/status", nil)
	h.GetStatus(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	assert.Equal(t, true, data["has_users"])
	assert.Equal(t, true, data["requires_auth"])
	assert.Equal(t, false, data["bootstrap_ready"])
}

func TestAuthHandler_GetStatus_AuthDisabled(t *testing.T) {
	store := handlersmocks.NewMockAuthStore(t)
	store.EXPECT().CountUsers().Return(5, nil)

	h := handlers.NewAuthHandler(store, false) // enableAuth=false
	c, w := newAuthCtx("GET", "/auth/status", nil)
	h.GetStatus(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	assert.Equal(t, false, data["requires_auth"]) // auth disabled → no auth required
}

func TestAuthHandler_Login_Success(t *testing.T) {
	hash := testBcryptHash(t, "password123")
	user := &database.User{
		ID:           "user-1",
		Username:     "alice",
		PasswordHash: hash,
		Roles:        []string{"user"},
		Status:       "active",
		CreatedAt:    time.Now(),
	}
	session := &database.Session{
		ID:        "sess-abc",
		UserID:    "user-1",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	store := handlersmocks.NewMockAuthStore(t)
	store.EXPECT().GetUserByUsername("alice").Return(user, nil)
	store.EXPECT().CreateSession("user-1", mock.Anything, mock.Anything, mock.Anything).Return(session, nil)

	h := handlers.NewAuthHandler(store, true)
	c, w := newAuthCtx("POST", "/auth/login", map[string]any{
		"username": "alice",
		"password": "password123",
	})
	h.Login(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	assert.NotNil(t, data["user"])
	assert.NotNil(t, data["session"])
}

func TestAuthHandler_Login_WrongPassword(t *testing.T) {
	hash := testBcryptHash(t, "correctpassword")
	user := &database.User{ID: "user-1", Username: "alice", PasswordHash: hash}

	store := handlersmocks.NewMockAuthStore(t)
	store.EXPECT().GetUserByUsername("alice").Return(user, nil)

	h := handlers.NewAuthHandler(store, true)
	c, w := newAuthCtx("POST", "/auth/login", map[string]any{
		"username": "alice",
		"password": "wrongpassword",
	})
	h.Login(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthHandler_Login_UserNotFound(t *testing.T) {
	store := handlersmocks.NewMockAuthStore(t)
	store.EXPECT().GetUserByUsername("nobody").Return(nil, nil)

	h := handlers.NewAuthHandler(store, true)
	c, w := newAuthCtx("POST", "/auth/login", map[string]any{
		"username": "nobody",
		"password": "anything",
	})
	h.Login(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthHandler_Login_LockedOut(t *testing.T) {
	hash := testBcryptHash(t, "password123")
	user := &database.User{ID: "user-1", Username: "alice", PasswordHash: hash}

	store := handlersmocks.NewMockAuthStore(t)
	// 10 failed attempts + 1 lockout check (11 GetUserByUsername calls)
	store.EXPECT().GetUserByUsername("alice").Return(user, nil).Times(11)

	h := handlers.NewAuthHandler(store, true)

	for i := 0; i < 10; i++ {
		c, _ := newAuthCtx("POST", "/auth/login", map[string]any{
			"username": "alice", "password": "wrong",
		})
		h.Login(c)
	}

	c, w := newAuthCtx("POST", "/auth/login", map[string]any{
		"username": "alice", "password": "password123",
	})
	h.Login(c)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

func TestAuthHandler_SetupInitialAdmin_AlreadyExists(t *testing.T) {
	store := handlersmocks.NewMockAuthStore(t)
	store.EXPECT().CountUsers().Return(1, nil)

	h := handlers.NewAuthHandler(store, true)
	c, w := newAuthCtx("POST", "/auth/setup", map[string]any{
		"username": "admin", "password": "password123",
	})
	h.SetupInitialAdmin(c)

	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestAuthHandler_SetupInitialAdmin_Success(t *testing.T) {
	created := &database.User{
		ID:        "user-1",
		Username:  "admin",
		Email:     "admin@local",
		Roles:     []string{"admin"},
		Status:    "active",
		CreatedAt: time.Now(),
	}

	store := handlersmocks.NewMockAuthStore(t)
	store.EXPECT().CountUsers().Return(0, nil)
	store.EXPECT().CreateUser("admin", "admin@local", "bcrypt", mock.Anything, []string{"admin"}, "active").Return(created, nil)

	h := handlers.NewAuthHandler(store, true)
	c, w := newAuthCtx("POST", "/auth/setup", map[string]any{
		"username": "admin", "password": "password123",
	})
	h.SetupInitialAdmin(c)

	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestAuthHandler_Logout_RevokesSession(t *testing.T) {
	session := &database.Session{ID: "sess-1", UserID: "user-1"}

	store := handlersmocks.NewMockAuthStore(t)
	store.EXPECT().RevokeSession("sess-1").Return(nil)

	h := handlers.NewAuthHandler(store, true)
	c, w := newAuthCtx("POST", "/auth/logout", nil)
	setAuthSession(c, session)
	h.Logout(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuthHandler_Me_NotAuthenticated(t *testing.T) {
	store := handlersmocks.NewMockAuthStore(t)

	h := handlers.NewAuthHandler(store, true)
	c, w := newAuthCtx("GET", "/auth/me", nil)
	// no user set in context
	h.Me(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthHandler_Me_Authenticated(t *testing.T) {
	user := &database.User{
		ID:       "user-1",
		Username: "alice",
		Roles:    []string{"user"},
		Status:   "active",
	}
	store := handlersmocks.NewMockAuthStore(t)

	h := handlers.NewAuthHandler(store, true)
	c, w := newAuthCtx("GET", "/auth/me", nil)
	setAuthUser(c, user)
	h.Me(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	userField := data["user"].(map[string]any)
	assert.Equal(t, "alice", userField["username"])
}
