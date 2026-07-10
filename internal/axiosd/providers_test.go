package axiosd

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/axios-os/axios/pkg/secrets"
)

func newTestSecrets(t *testing.T, dir string) *secrets.Store {
	t.Helper()
	sec, err := secrets.NewStore(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatalf("secrets.NewStore: %v", err)
	}
	return sec
}

func TestProviderStoreSecretsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	sec := newTestSecrets(t, dir)
	path := filepath.Join(dir, "providers.json")

	ps := NewProviderStore(path, sec)
	if err := ps.SetAPIKey("openai", "sk-test-roundtrip"); err != nil {
		t.Fatalf("SetAPIKey: %v", err)
	}
	if err := ps.SetActive("openai", "gpt-4o"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	if err := ps.SaveToFile(); err != nil {
		t.Fatalf("SaveToFile: %v", err)
	}

	// The on-disk value must be encrypted (axsec1: prefix), never plaintext
	// or plain base64 of the key.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read providers file: %v", err)
	}
	var onDisk struct {
		ActiveID    string            `json:"active_id"`
		ActiveModel string            `json:"active_model"`
		Keys        map[string]string `json:"keys"`
	}
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatalf("parse providers file: %v", err)
	}
	stored := onDisk.Keys["openai"]
	if !strings.HasPrefix(stored, "axsec1:") {
		t.Fatalf("stored key %q lacks axsec1: prefix", stored)
	}
	if strings.Contains(string(raw), "sk-test-roundtrip") {
		t.Fatal("plaintext key leaked into providers.json")
	}
	legacyB64 := base64.StdEncoding.EncodeToString([]byte("sk-test-roundtrip"))
	if strings.Contains(string(raw), legacyB64) {
		t.Fatal("key stored as legacy base64 despite secrets store")
	}

	// A fresh store with the same key file must decrypt transparently.
	ps2 := NewProviderStore(path, newTestSecrets(t, dir))
	if err := ps2.LoadFromFile(); err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	key, ok := ps2.Credential("openai")
	if !ok || key != "sk-test-roundtrip" {
		t.Fatalf("Credential = %q, %v", key, ok)
	}
	active := ps2.GetActive()
	if active == nil || active.ID != "openai" {
		t.Fatalf("active provider not restored: %+v", active)
	}
	if got := ps2.GetActiveModel(); got != "gpt-4o" {
		t.Fatalf("active model = %q, want gpt-4o", got)
	}
}

func TestProviderStoreLegacyBase64Upgrade(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "providers.json")

	// Simulate a pre-encryption providers.json with plain base64 keys.
	legacy := map[string]any{
		"active_id":    "anthropic",
		"active_model": "claude-sonnet-4-6",
		"keys": map[string]string{
			"anthropic": base64.StdEncoding.EncodeToString([]byte("sk-ant-legacy")),
		},
	}
	raw, _ := json.MarshalIndent(legacy, "", "  ")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}

	sec := newTestSecrets(t, dir)
	ps := NewProviderStore(path, sec)
	if err := ps.LoadFromFile(); err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}

	// Legacy value decoded transparently.
	key, ok := ps.Credential("anthropic")
	if !ok || key != "sk-ant-legacy" {
		t.Fatalf("Credential after legacy load = %q, %v", key, ok)
	}
	if active := ps.GetActive(); active == nil || active.ID != "anthropic" {
		t.Fatalf("active provider not restored from legacy file: %+v", active)
	}

	// The next save re-encrypts (transparent upgrade).
	if err := ps.SaveToFile(); err != nil {
		t.Fatalf("SaveToFile: %v", err)
	}
	upgraded, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read upgraded file: %v", err)
	}
	var onDisk struct {
		Keys map[string]string `json:"keys"`
	}
	if err := json.Unmarshal(upgraded, &onDisk); err != nil {
		t.Fatalf("parse upgraded file: %v", err)
	}
	if !strings.HasPrefix(onDisk.Keys["anthropic"], "axsec1:") {
		t.Fatalf("legacy key not upgraded on save: %q", onDisk.Keys["anthropic"])
	}

	// And it still round-trips.
	ps2 := NewProviderStore(path, newTestSecrets(t, dir))
	if err := ps2.LoadFromFile(); err != nil {
		t.Fatalf("LoadFromFile after upgrade: %v", err)
	}
	key, ok = ps2.Credential("anthropic")
	if !ok || key != "sk-ant-legacy" {
		t.Fatalf("Credential after upgrade = %q, %v", key, ok)
	}
}

func TestProviderStoreSetActiveRequiresKey(t *testing.T) {
	ps := NewProviderStore(filepath.Join(t.TempDir(), "providers.json"), nil)

	if err := ps.SetActive("openai", "gpt-4o"); err == nil {
		t.Fatal("SetActive without key should fail")
	}
	if err := ps.SetActive("nope", "x"); err == nil {
		t.Fatal("SetActive with unknown provider should fail")
	}

	if err := ps.SetAPIKey("openai", "sk-x"); err != nil {
		t.Fatalf("SetAPIKey: %v", err)
	}
	// Custom (non-catalog) model IDs are allowed.
	if err := ps.SetActive("openai", "gpt-custom-preview"); err != nil {
		t.Fatalf("SetActive with custom model: %v", err)
	}
	if err := ps.SetActive("openai", ""); err == nil {
		t.Fatal("SetActive with empty model should fail")
	}
}

func TestProviderStoreProviderForModel(t *testing.T) {
	ps := NewProviderStore(filepath.Join(t.TempDir(), "providers.json"), nil)
	ps.SetAPIKey("openai", "sk-1")
	ps.SetAPIKey("groq", "sk-2")

	if got := ps.ProviderForModel("gpt-4o"); got != "openai" {
		t.Errorf("ProviderForModel(gpt-4o) = %q, want openai", got)
	}
	if got := ps.ProviderForModel("llama-3.1-8b-instant"); got != "groq" {
		t.Errorf("ProviderForModel(llama-3.1-8b-instant) = %q, want groq", got)
	}
	// No key → not eligible.
	if got := ps.ProviderForModel("claude-sonnet-4-6"); got != "" {
		t.Errorf("ProviderForModel(claude-sonnet-4-6) = %q, want empty", got)
	}
	if got := ps.ProviderForModel("unknown-model"); got != "" {
		t.Errorf("ProviderForModel(unknown-model) = %q, want empty", got)
	}
}
