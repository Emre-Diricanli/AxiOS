// Package secrets encrypts credentials at rest with AES-256-GCM.
//
// Threat model (honest limits): this package protects secrets such as
// providers.json against at-rest exfiltration — backups, copied disks, and
// other local users reading the file. It does NOT protect against a
// compromised daemon or anything that can run code as the AxiOS user,
// because the master key lives on disk beside the data it protects.
// Keychain/TPM-backed key storage is a later upgrade.
//
// Wire format: "axsec1:" + base64(nonce || ciphertext), where ciphertext
// includes the GCM authentication tag. The nonce is random per encryption.
// The version prefix makes encrypted values distinguishable from the legacy
// plain-base64 storage format: the standard base64 alphabet contains no ':',
// so a legacy value can never start with "axsec1:".
//
// GCM authentication uses a constant-time tag comparison internally, and this
// package never places key material, plaintext, or caller-supplied input in
// error messages.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// prefix is the versioned marker prepended to every encrypted value.
const prefix = "axsec1:"

// keySize is the AES-256 key length in bytes.
const keySize = 32

var (
	// ErrNotEncrypted is returned by Decrypt when the input does not carry
	// the "axsec1:" prefix (e.g. a legacy base64 value).
	ErrNotEncrypted = errors.New("secrets: value is not an axsec1-encrypted secret")

	// ErrDecryptFailed is returned when authentication fails: the ciphertext
	// was tampered with, truncated, or encrypted under a different key.
	ErrDecryptFailed = errors.New("secrets: decryption failed (wrong key or tampered ciphertext)")
)

// Store encrypts and decrypts secrets using a persistent 32-byte master key.
// A Store is safe for concurrent use by multiple goroutines.
type Store struct {
	aead cipher.AEAD
}

// NewStore loads the master key from keyPath, creating a random 32-byte key
// there if the file does not exist. The key file is written with mode 0600
// and its parent directory is created with mode 0700 if missing. Typical
// keyPath: $AXIOS_DATA_DIR/master.key.
func NewStore(keyPath string) (*Store, error) {
	key, err := loadOrCreateKey(keyPath)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secrets: init cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secrets: init GCM: %w", err)
	}
	return &Store{aead: aead}, nil
}

// Encrypt seals plaintext with AES-256-GCM under a fresh random nonce and
// returns "axsec1:" + base64(nonce || ciphertext).
func (s *Store) Encrypt(plaintext []byte) (string, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("secrets: generate nonce: %w", err)
	}
	sealed := s.aead.Seal(nonce, nonce, plaintext, nil)
	return prefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt reverses Encrypt. It returns ErrNotEncrypted if the value lacks the
// "axsec1:" prefix, and ErrDecryptFailed if authentication fails (wrong key
// or tampered data).
func (s *Store) Decrypt(v string) ([]byte, error) {
	if !IsEncrypted(v) {
		return nil, ErrNotEncrypted
	}
	raw, err := base64.StdEncoding.DecodeString(v[len(prefix):])
	if err != nil {
		return nil, fmt.Errorf("%w: malformed base64 payload", ErrDecryptFailed)
	}
	nonceSize := s.aead.NonceSize()
	if len(raw) < nonceSize {
		return nil, fmt.Errorf("%w: payload shorter than nonce", ErrDecryptFailed)
	}
	nonce, ciphertext := raw[:nonceSize], raw[nonceSize:]
	plaintext, err := s.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		// Deliberately generic: never echo key material or payloads.
		return nil, ErrDecryptFailed
	}
	return plaintext, nil
}

// IsEncrypted reports whether v is an axsec1-encrypted value, as opposed to a
// legacy plain-base64 value. The check is unambiguous because the standard
// base64 alphabet does not contain ':'.
func IsEncrypted(v string) bool {
	return strings.HasPrefix(v, prefix)
}

// loadOrCreateKey reads a 32-byte key from keyPath, generating and persisting
// a fresh one (mode 0600, parent dir 0700) if the file does not exist.
func loadOrCreateKey(keyPath string) ([]byte, error) {
	key, err := os.ReadFile(keyPath)
	if err == nil {
		return validateKey(key, keyPath)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("secrets: read key file: %w", err)
	}

	if dir := filepath.Dir(keyPath); dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("secrets: create key directory: %w", err)
		}
	}

	key = make([]byte, keySize)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("secrets: generate key: %w", err)
	}

	f, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			// Another process won the creation race; use its key.
			existing, readErr := os.ReadFile(keyPath)
			if readErr != nil {
				return nil, fmt.Errorf("secrets: read key file: %w", readErr)
			}
			return validateKey(existing, keyPath)
		}
		return nil, fmt.Errorf("secrets: create key file: %w", err)
	}
	if _, err := f.Write(key); err != nil {
		f.Close()
		os.Remove(keyPath)
		return nil, fmt.Errorf("secrets: write key file: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(keyPath)
		return nil, fmt.Errorf("secrets: close key file: %w", err)
	}
	return key, nil
}

// validateKey checks the key length and tightens overly permissive file modes
// left behind by external tooling. Error messages name the path only — never
// key bytes.
func validateKey(key []byte, keyPath string) ([]byte, error) {
	if len(key) != keySize {
		return nil, fmt.Errorf("secrets: key file %s has invalid size %d (want %d bytes)", keyPath, len(key), keySize)
	}
	if info, err := os.Stat(keyPath); err == nil && info.Mode().Perm()&0o077 != 0 {
		if err := os.Chmod(keyPath, 0o600); err != nil {
			return nil, fmt.Errorf("secrets: tighten key file permissions: %w", err)
		}
	}
	return key, nil
}
