// Package credentials provides encryption and decryption of sensitive node
// credentials (passwords, API keys, tokens) stored outside the main flow file.
//
// Encryption scheme (planned):
//   - Algorithm: AES-256-GCM (authenticated encryption)
//   - Key: 256-bit key stored in a separate key file (loopze.key)
//   - Nonce: 96-bit random nonce prepended to each ciphertext
//   - Format: nonce (12 bytes) || ciphertext || GCM tag (16 bytes)
//
// The key file is auto-generated on first run if it does not exist.
// It MUST be excluded from version control and backups should be handled
// separately by the operator.
package credentials

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
)

// keySize is the required AES-256 key length in bytes.
const keySize = 32

// CredentialManager handles encryption and decryption of node credentials.
// It holds the path to the key file and caches the derived AES-GCM cipher
// after the first successful key load.
type CredentialManager struct {
	keyFile string
	mu      sync.RWMutex
	aead    cipher.AEAD
}

// NewCredentialManager creates a new CredentialManager that reads its
// encryption key from the given keyFile path.
func NewCredentialManager(keyFile string) *CredentialManager {
	return &CredentialManager{
		keyFile: keyFile,
	}
}

// EnsureKeyFile checks whether the key file exists. If it does not, a new
// 256-bit random key is generated and written with restrictive permissions
// (0600). This should be called during application startup.
func (cm *CredentialManager) EnsureKeyFile() error {
	if _, err := os.Stat(cm.keyFile); err == nil {
		slog.Debug("credential key file already exists", "path", cm.keyFile)
		return nil
	}

	slog.Info("generating new credential encryption key", "path", cm.keyFile)

	key := make([]byte, keySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return fmt.Errorf("credentials: failed to generate random key: %w", err)
	}

	// Write with owner-only read/write permissions.
	if err := os.WriteFile(cm.keyFile, key, 0600); err != nil {
		return fmt.Errorf("credentials: failed to write key file: %w", err)
	}

	slog.Info("credential encryption key generated successfully", "path", cm.keyFile)
	return nil
}

// loadCipher lazily initialises the AES-GCM AEAD cipher from the key file.
// It is safe for concurrent use.
func (cm *CredentialManager) loadCipher() (cipher.AEAD, error) {
	cm.mu.RLock()
	if cm.aead != nil {
		defer cm.mu.RUnlock()
		return cm.aead, nil
	}
	cm.mu.RUnlock()

	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Double-check after acquiring write lock.
	if cm.aead != nil {
		return cm.aead, nil
	}

	key, err := os.ReadFile(cm.keyFile)
	if err != nil {
		return nil, fmt.Errorf("credentials: failed to read key file %q: %w", cm.keyFile, err)
	}
	if len(key) != keySize {
		return nil, fmt.Errorf("credentials: invalid key length %d (expected %d)", len(key), keySize)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("credentials: failed to create AES cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("credentials: failed to create GCM: %w", err)
	}

	cm.aead = aead
	return cm.aead, nil
}

// Encrypt encrypts the given plaintext using AES-256-GCM. The returned byte
// slice contains a random nonce prepended to the authenticated ciphertext:
//
//	[ nonce (12 bytes) | ciphertext + GCM tag ]
func (cm *CredentialManager) Encrypt(data []byte) ([]byte, error) {
	aead, err := cm.loadCipher()
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("credentials: failed to generate nonce: %w", err)
	}

	// Seal appends the ciphertext to nonce so the result is nonce||ciphertext.
	ciphertext := aead.Seal(nonce, nonce, data, nil)
	return ciphertext, nil
}

// Decrypt decrypts data that was previously encrypted with Encrypt. It expects
// the nonce to be prepended to the ciphertext (as produced by Encrypt).
func (cm *CredentialManager) Decrypt(data []byte) ([]byte, error) {
	aead, err := cm.loadCipher()
	if err != nil {
		return nil, err
	}

	nonceSize := aead.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("credentials: ciphertext too short (len=%d, need at least %d)", len(data), nonceSize)
	}

	nonce := data[:nonceSize]
	ciphertext := data[nonceSize:]

	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("credentials: decryption failed: %w", err)
	}

	return plaintext, nil
}

// EncryptJSON is a convenience wrapper that marshals the given value to JSON
// and then encrypts the result.
func (cm *CredentialManager) EncryptJSON(v any) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("credentials: failed to marshal JSON: %w", err)
	}
	return cm.Encrypt(data)
}

// DecryptJSON is a convenience wrapper that decrypts data and then unmarshals
// the resulting JSON into the given target.
func (cm *CredentialManager) DecryptJSON(data []byte, target any) error {
	plaintext, err := cm.Decrypt(data)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(plaintext, target); err != nil {
		return fmt.Errorf("credentials: failed to unmarshal JSON: %w", err)
	}
	return nil
}
