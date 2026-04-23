// file: internal/server/bootstrap.go
// version: 1.3.0
// guid: 3e7c9a12-4f6b-4d8e-b5a1-2c8f0e3d9b47

package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/auth"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	ulid "github.com/oklog/ulid/v2"
)

const (
	bootstrapTokenKey    = "bootstrap_token_hash"
	bootstrapExpiresKey  = "bootstrap_token_expires_at"
	bootstrapTokenTTL    = 10 * time.Minute
)

// SettingsReadWriter is the minimal store surface needed by the bootstrap subsystem.
type SettingsReadWriter interface {
	GetSetting(key string) (*database.Setting, error)
	SetSetting(key, value, typ string, isSecret bool) error
	DeleteSetting(key string) error
}

// bootstrapMu prevents two concurrent exchange attempts from both succeeding.
var bootstrapMu sync.Mutex

// BootstrapTokenPath returns the path to the on-disk plaintext bootstrap token file.
func BootstrapTokenPath(dataDir string) string {
	return filepath.Join(dataDir, ".bootstrap-token")
}

// InitBootstrapToken generates a fresh bootstrap token on every startup.
// The token expires after bootstrapTokenTTL (10 min). A restart is required
// to get a new one — so an unexpected restart is visible in the logs.
func InitBootstrapToken(store SettingsReadWriter, dataDir string) error {
	// Always replace — each restart gets a fresh 10-minute window.
	_ = store.DeleteSetting(bootstrapTokenKey)
	_ = store.DeleteSetting(bootstrapExpiresKey)

	raw, hash, err := generateBootstrapToken()
	if err != nil {
		return fmt.Errorf("bootstrap: generate token: %w", err)
	}

	expiresAt := time.Now().Add(bootstrapTokenTTL)
	if err := store.SetSetting(bootstrapTokenKey, hash, "string", false); err != nil {
		return fmt.Errorf("bootstrap: persist token hash: %w", err)
	}
	if err := store.SetSetting(bootstrapExpiresKey, fmt.Sprintf("%d", expiresAt.Unix()), "string", false); err != nil {
		return fmt.Errorf("bootstrap: persist token expiry: %w", err)
	}

	tokenPath := BootstrapTokenPath(dataDir)
	if err := os.WriteFile(tokenPath, []byte(raw+"\n"), 0600); err != nil {
		log.Printf("[BOOTSTRAP] WARNING: could not write token file %s: %v", tokenPath, err)
	}

	log.Printf("[BOOTSTRAP] Emergency access token: %s", raw)
	log.Printf("[BOOTSTRAP] Token expires in 10 minutes. POST /api/v1/auth/bootstrap to exchange for an API key. Restart required to generate a new token.")
	return nil
}

// ConsumeBootstrapToken validates plaintext against the stored hash, checks
// the 10-minute expiry, then atomically deletes both settings and the on-disk
// file. Returns (valid, error). Thread-safe.
func ConsumeBootstrapToken(store SettingsReadWriter, dataDir, plaintext string) (bool, error) {
	bootstrapMu.Lock()
	defer bootstrapMu.Unlock()

	setting, err := store.GetSetting(bootstrapTokenKey)
	if err != nil {
		return false, fmt.Errorf("bootstrap: read token hash: %w", err)
	}
	if setting == nil || setting.Value == "" {
		return false, nil
	}

	// Check expiry before doing any hash work.
	if expSetting, err := store.GetSetting(bootstrapExpiresKey); err == nil && expSetting != nil {
		var expUnix int64
		fmt.Sscanf(expSetting.Value, "%d", &expUnix)
		if expUnix > 0 && time.Now().Unix() > expUnix {
			log.Printf("[BOOTSTRAP] Token exchange attempted but token has expired (restart required to generate a new one)")
			_ = store.DeleteSetting(bootstrapTokenKey)
			_ = store.DeleteSetting(bootstrapExpiresKey)
			_ = os.Remove(BootstrapTokenPath(dataDir))
			return false, nil
		}
	}

	if hashBootstrapToken(plaintext) != setting.Value {
		return false, nil
	}

	_ = store.DeleteSetting(bootstrapTokenKey)
	_ = store.DeleteSetting(bootstrapExpiresKey)
	_ = os.Remove(BootstrapTokenPath(dataDir))

	return true, nil
}

func generateBootstrapToken() (raw, hash string, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("read random bytes: %w", err)
	}
	raw = "abbs_" + base64.RawURLEncoding.EncodeToString(buf)
	hash = hashBootstrapToken(raw)
	return raw, hash, nil
}

func hashBootstrapToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// --- rate-limit state for the bootstrap endpoint ---

const (
	bootstrapMaxAttempts    = 5
	bootstrapWindowDuration = time.Hour
)

type bootstrapAttempt struct {
	count   int
	firstAt time.Time
}

var (
	bootstrapRateMu sync.Mutex
	bootstrapRate   = map[string]*bootstrapAttempt{}
)

func bootstrapIsRateLimited(ip string) bool {
	bootstrapRateMu.Lock()
	defer bootstrapRateMu.Unlock()
	a, ok := bootstrapRate[ip]
	if !ok {
		return false
	}
	if time.Since(a.firstAt) > bootstrapWindowDuration {
		delete(bootstrapRate, ip)
		return false
	}
	return a.count >= bootstrapMaxAttempts
}

func bootstrapRecordAttempt(ip string) {
	bootstrapRateMu.Lock()
	defer bootstrapRateMu.Unlock()
	a, ok := bootstrapRate[ip]
	if !ok || time.Since(a.firstAt) > bootstrapWindowDuration {
		bootstrapRate[ip] = &bootstrapAttempt{count: 1, firstAt: time.Now()}
		return
	}
	a.count++
}

// --- handler ---

type bootstrapRequest struct {
	Token   string `json:"token"`
	KeyName string `json:"key_name"`
}

// handleBootstrap exchanges a one-time bootstrap token for a full-privilege API key.
// Wired to a public (unauthenticated) route.
func (s *Server) handleBootstrap(c *gin.Context) {
	ip := strings.TrimSpace(c.ClientIP())

	if bootstrapIsRateLimited(ip) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many bootstrap attempts — try again later"})
		return
	}
	bootstrapRecordAttempt(ip)

	var req bootstrapRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Token = strings.TrimSpace(req.Token)
	if req.Token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "token is required"})
		return
	}

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	dataDir := filepath.Dir(config.AppConfig.DatabasePath)

	valid, err := ConsumeBootstrapToken(store, dataDir, req.Token)
	if err != nil {
		log.Printf("[BOOTSTRAP] consume error ip=%s err=%v", ip, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	if !valid {
		time.Sleep(500 * time.Millisecond)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid bootstrap token"})
		return
	}

	adminUser, err := findAdminUser(store)
	if err != nil || adminUser == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "no admin user found — complete setup first"})
		return
	}

	keyName := strings.TrimSpace(req.KeyName)
	if keyName == "" {
		keyName = "Bootstrap recovery key"
	}

	raw, hash, err := database.GenerateAPIKeyToken()
	if err != nil {
		log.Printf("[BOOTSTRAP] generate api key error ip=%s err=%v", ip, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate API key"})
		return
	}

	scopes := auth.All()

	key := &database.APIKey{
		ID:          ulid.Make().String(),
		UserID:      adminUser.ID,
		Name:        keyName,
		Description: "Generated via bootstrap token exchange",
		TokenHash:   hash,
		Scopes:      scopes,
		Status:      "active",
		CreatedAt:   time.Now(),
	}

	created, err := store.CreateAPIKey(key)
	if err != nil {
		log.Printf("[BOOTSTRAP] create api key error user=%s ip=%s err=%v", adminUser.ID, ip, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create API key"})
		return
	}

	log.Printf("[BOOTSTRAP] Token consumed: new API key created user=%s key_id=%s ip=%s", adminUser.Username, created.ID, ip)

	c.JSON(http.StatusOK, gin.H{
		"api_key":  raw,
		"key_id":   created.ID,
		"user_id":  adminUser.ID,
		"username": adminUser.Username,
		"scopes":   scopes,
		"message":  "Bootstrap token consumed. This key will not be shown again.",
	})
}

// findAdminUser returns the first user whose assigned role carries PermUsersManage.
func findAdminUser(store database.Store) (*database.User, error) {
	users, err := store.ListUsers()
	if err != nil {
		return nil, err
	}
	for i := range users {
		u := &users[i]
		for _, roleName := range u.Roles {
			role, err := store.GetRoleByName(roleName)
			if err != nil || role == nil {
				role, _ = store.GetRoleByID(roleName)
			}
			if role == nil {
				continue
			}
			for _, perm := range role.Permissions {
				if perm == auth.PermUsersManage {
					return u, nil
				}
			}
		}
	}
	return nil, nil
}
