// file: internal/database/settings.go
// version: 1.0.1
// guid: 8a7b6c5d-4e3f-2a1b-0c9d-8e7f6a5b4c3d

package database

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/cockroachdb/pebble"
)

// Setting represents a stored configuration setting
type Setting struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Type     string `json:"type"`      // "string", "int", "bool", "json"
	IsSecret bool   `json:"is_secret"` // If true, value is encrypted
}

// Encryption key derivation and storage
var encryptionKey []byte

// InitEncryption initializes or loads the encryption key
func InitEncryption(dataDir string) error {
	keyPath := filepath.Join(dataDir, ".encryption_key")

	// Try to load existing key
	if data, err := os.ReadFile(keyPath); err == nil {
		encryptionKey = data
		if len(encryptionKey) != 32 {
			return fmt.Errorf("invalid encryption key length: %d", len(encryptionKey))
		}
		return nil
	}

	// Generate new key
	encryptionKey = make([]byte, 32) // AES-256
	if _, err := io.ReadFull(rand.Reader, encryptionKey); err != nil {
		return fmt.Errorf("failed to generate encryption key: %w", err)
	}

	// Save key with restrictive permissions
	if err := os.WriteFile(keyPath, encryptionKey, 0600); err != nil {
		return fmt.Errorf("failed to save encryption key: %w", err)
	}

	return nil
}

// DeriveKeyFromPassword can be used as alternative to random key
func DeriveKeyFromPassword(password string) []byte {
	hash := sha256.Sum256([]byte(password))
	return hash[:]
}

// EncryptValue encrypts a plaintext value
func EncryptValue(plaintext string) (string, error) {
	if encryptionKey == nil {
		return "", fmt.Errorf("encryption key not initialized")
	}

	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptValue decrypts an encrypted value
func DecryptValue(encrypted string) (string, error) {
	if encryptionKey == nil {
		return "", fmt.Errorf("encryption key not initialized")
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// MaskSecret returns a masked version of a secret (for display)
func MaskSecret(secret string) string {
	if len(secret) < 8 {
		return "****"
	}
	return secret[:3] + "****" + secret[len(secret)-4:]
}

// PebbleDB implementation
func (s *PebbleStore) GetSetting(key string) (*Setting, error) {
	data, closer, err := s.db.Get([]byte("setting:" + key))
	if err != nil {
		if err == pebble.ErrNotFound {
			return nil, fmt.Errorf("setting not found: %s", key)
		}
		return nil, err
	}
	defer closer.Close()

	var setting Setting
	if err := json.Unmarshal(data, &setting); err != nil {
		return nil, err
	}

	return &setting, nil
}

func (s *PebbleStore) SetSetting(key, value, typ string, isSecret bool) error {
	// Encrypt if secret
	storedValue := value
	if isSecret && value != "" {
		encrypted, err := EncryptValue(value)
		if err != nil {
			return fmt.Errorf("encryption failed: %w", err)
		}
		storedValue = encrypted
	}

	setting := Setting{
		Key:      key,
		Value:    storedValue,
		Type:     typ,
		IsSecret: isSecret,
	}

	data, err := json.Marshal(setting)
	if err != nil {
		return err
	}

	return s.db.Set([]byte("setting:"+key), data, nil)
}

func (s *PebbleStore) GetAllSettings() ([]Setting, error) {
	var settings []Setting

	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("setting:"),
		UpperBound: []byte("setting:\xff"),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		var setting Setting
		if err := json.Unmarshal(iter.Value(), &setting); err != nil {
			continue
		}

		// Mask secrets in list view
		if setting.IsSecret && setting.Value != "" {
			setting.Value = MaskSecret(setting.Value)
		}

		settings = append(settings, setting)
	}

	return settings, nil
}

func (s *PebbleStore) DeleteSetting(key string) error {
	return s.db.Delete([]byte("setting:"+key), nil)
}

// SQLite implementation
func (s *SQLiteStore) GetSetting(key string) (*Setting, error) {
	var setting Setting
	var isSecret int

	err := s.db.QueryRow(`
		SELECT key, value, type, is_secret
		FROM settings
		WHERE key = ?
	`, key).Scan(&setting.Key, &setting.Value, &setting.Type, &isSecret)

	if err != nil {
		return nil, err
	}

	setting.IsSecret = isSecret == 1
	return &setting, nil
}

func (s *SQLiteStore) SetSetting(key, value, typ string, isSecret bool) error {
	// Encrypt if secret
	storedValue := value
	if isSecret && value != "" {
		encrypted, err := EncryptValue(value)
		if err != nil {
			return fmt.Errorf("encryption failed: %w", err)
		}
		storedValue = encrypted
	}

	isSecretInt := 0
	if isSecret {
		isSecretInt = 1
	}

	_, err := s.db.Exec(`
		INSERT INTO settings (key, value, type, is_secret, updated_at)
		VALUES (?, ?, ?, ?, datetime('now'))
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			type = excluded.type,
			is_secret = excluded.is_secret,
			updated_at = excluded.updated_at
	`, key, storedValue, typ, isSecretInt)

	return err
}

func (s *SQLiteStore) GetAllSettings() ([]Setting, error) {
	rows, err := s.db.Query(`
		SELECT key, value, type, is_secret
		FROM settings
		ORDER BY key
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var settings []Setting
	for rows.Next() {
		var setting Setting
		var isSecret int

		if err := rows.Scan(&setting.Key, &setting.Value, &setting.Type, &isSecret); err != nil {
			continue
		}

		setting.IsSecret = isSecret == 1

		// Mask secrets in list view
		if setting.IsSecret && setting.Value != "" {
			setting.Value = MaskSecret(setting.Value)
		}

		settings = append(settings, setting)
	}

	return settings, rows.Err()
}

func (s *SQLiteStore) DeleteSetting(key string) error {
	_, err := s.db.Exec("DELETE FROM settings WHERE key = ?", key)
	return err
}

// Helper functions to get decrypted setting value from stores
func GetDecryptedSetting(store Store, key string) (string, error) {
	setting, err := store.GetSetting(key)
	if err != nil {
		return "", err
	}

	if !setting.IsSecret {
		return setting.Value, nil
	}

	return DecryptValue(setting.Value)
}
