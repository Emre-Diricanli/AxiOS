package secrets

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(filepath.Join(t.TempDir(), "master.key"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return s
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	s := newTestStore(t)

	tests := []struct {
		name      string
		plaintext []byte
	}{
		{"empty", []byte{}},
		{"short api key", []byte("sk-ant-api03-abc123")},
		{"long", bytes.Repeat([]byte("credential-material-"), 200)},
		{"binary", []byte{0x00, 0xff, 0x10, 0x7f, 0x00, 0x01}},
		{"unicode", []byte("anahtar-şifre-秘密-🔑")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc, err := s.Encrypt(tt.plaintext)
			if err != nil {
				t.Fatalf("Encrypt: %v", err)
			}
			if !strings.HasPrefix(enc, "axsec1:") {
				t.Errorf("Encrypt output %q missing axsec1: prefix", enc)
			}
			dec, err := s.Decrypt(enc)
			if err != nil {
				t.Fatalf("Decrypt: %v", err)
			}
			if !bytes.Equal(dec, tt.plaintext) {
				t.Errorf("round trip mismatch: got %q, want %q", dec, tt.plaintext)
			}
		})
	}
}

func TestEncryptUsesRandomNonce(t *testing.T) {
	s := newTestStore(t)
	plaintext := []byte("same input")

	a, err := s.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	b, err := s.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if a == b {
		t.Errorf("two encryptions of the same plaintext produced identical output %q", a)
	}
}

func TestDecryptWrongKeyFails(t *testing.T) {
	s1 := newTestStore(t)
	s2 := newTestStore(t)

	enc, err := s1.Encrypt([]byte("secret value"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if _, err := s2.Decrypt(enc); !errors.Is(err, ErrDecryptFailed) {
		t.Errorf("Decrypt with wrong key: got err %v, want ErrDecryptFailed", err)
	}
}

func TestDecryptRejectsBadInput(t *testing.T) {
	s := newTestStore(t)
	enc, err := s.Encrypt([]byte("secret value"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Flip one byte in the middle of the sealed payload.
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(enc, "axsec1:"))
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	raw[len(raw)/2] ^= 0x01
	tampered := "axsec1:" + base64.StdEncoding.EncodeToString(raw)

	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		{"tampered ciphertext", tampered, ErrDecryptFailed},
		{"truncated below nonce size", "axsec1:" + base64.StdEncoding.EncodeToString(raw[:4]), ErrDecryptFailed},
		{"malformed base64", "axsec1:!!!not-base64!!!", ErrDecryptFailed},
		{"missing prefix", strings.TrimPrefix(enc, "axsec1:"), ErrNotEncrypted},
		{"legacy base64 value", base64.StdEncoding.EncodeToString([]byte("sk-legacy")), ErrNotEncrypted},
		{"empty string", "", ErrNotEncrypted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.Decrypt(tt.input)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Decrypt(%q): got err %v, want %v", tt.name, err, tt.wantErr)
			}
			if got != nil {
				t.Errorf("Decrypt(%q): got plaintext %q on failure, want nil", tt.name, got)
			}
		})
	}
}

func TestIsEncrypted(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"axsec1 value", "axsec1:c29tZSBjaXBoZXJ0ZXh0", true},
		{"legacy base64 api key", base64.StdEncoding.EncodeToString([]byte("sk-ant-api03-abc")), false},
		{"plain text", "sk-ant-api03-abc", false},
		{"empty", "", false},
		{"prefix only", "axsec1:", true},
		{"different version prefix", "axsec2:c29tZQ==", false},
		{"prefix not at start", " axsec1:c29tZQ==", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsEncrypted(tt.input); got != tt.want {
				t.Errorf("IsEncrypted(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}

	// Real Encrypt output must always be detected as encrypted.
	s := newTestStore(t)
	enc, err := s.Encrypt([]byte("value"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if !IsEncrypted(enc) {
		t.Errorf("IsEncrypted(Encrypt output) = false, want true")
	}
}

func TestKeyFileCreatedWithModeAndReused(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "data", "master.key")

	s1, err := NewStore(keyPath)
	if err != nil {
		t.Fatalf("NewStore (create): %v", err)
	}

	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat key file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("key file mode = %o, want 0600", perm)
	}
	dirInfo, err := os.Stat(filepath.Dir(keyPath))
	if err != nil {
		t.Fatalf("stat key dir: %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Errorf("key dir mode = %o, want 0700", perm)
	}
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("read key file: %v", err)
	}
	if len(keyBytes) != 32 {
		t.Errorf("key file size = %d, want 32", len(keyBytes))
	}

	enc, err := s1.Encrypt([]byte("persisted secret"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// A second store on the same path must reuse the key, not regenerate it.
	s2, err := NewStore(keyPath)
	if err != nil {
		t.Fatalf("NewStore (reuse): %v", err)
	}
	dec, err := s2.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt with reused key: %v", err)
	}
	if string(dec) != "persisted secret" {
		t.Errorf("Decrypt with reused key = %q, want %q", dec, "persisted secret")
	}
	keyBytesAfter, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("re-read key file: %v", err)
	}
	if !bytes.Equal(keyBytes, keyBytesAfter) {
		t.Errorf("key file changed across NewStore calls")
	}
}

func TestNewStoreRejectsInvalidKeyFile(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "master.key")
	if err := os.WriteFile(keyPath, []byte("too short"), 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}
	if _, err := NewStore(keyPath); err == nil {
		t.Errorf("NewStore with %d-byte key file: got nil error, want failure", len("too short"))
	}
}

func TestConcurrentEncrypt(t *testing.T) {
	s := newTestStore(t)

	const goroutines = 16
	const perGoroutine = 50
	plaintext := []byte("concurrent secret")

	results := make([][]string, goroutines)
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			out := make([]string, 0, perGoroutine)
			for i := 0; i < perGoroutine; i++ {
				enc, err := s.Encrypt(plaintext)
				if err != nil {
					t.Errorf("goroutine %d: Encrypt: %v", g, err)
					return
				}
				out = append(out, enc)
			}
			results[g] = out
		}(g)
	}
	wg.Wait()

	seen := make(map[string]bool, goroutines*perGoroutine)
	for g, out := range results {
		for _, enc := range out {
			if seen[enc] {
				t.Fatalf("duplicate ciphertext produced under concurrency")
			}
			seen[enc] = true
			dec, err := s.Decrypt(enc)
			if err != nil {
				t.Fatalf("goroutine %d output failed to decrypt: %v", g, err)
			}
			if !bytes.Equal(dec, plaintext) {
				t.Fatalf("goroutine %d output decrypted to %q, want %q", g, dec, plaintext)
			}
		}
	}
	if len(seen) != goroutines*perGoroutine {
		t.Errorf("got %d ciphertexts, want %d", len(seen), goroutines*perGoroutine)
	}
}
