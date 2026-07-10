package axiosd

import (
	"path/filepath"
	"sync"
	"testing"
)

func TestProviderRuntimeCloudClient(t *testing.T) {
	ps := NewProviderStore(filepath.Join(t.TempDir(), "providers.json"), nil)
	rt := NewProviderRuntime(ps, nil, testLogger())

	// No active provider yet.
	if _, err := rt.CloudClient(); err == nil {
		t.Fatal("CloudClient without active provider should fail")
	}

	ps.SetAPIKey("openai", "sk-test")
	ps.SetActive("openai", "gpt-4o")
	rt.Rebuild()

	client, err := rt.CloudClient()
	if err != nil {
		t.Fatalf("CloudClient: %v", err)
	}
	if client.Name() != "openai" || client.Model() != "gpt-4o" {
		t.Errorf("client = %s/%s, want openai/gpt-4o", client.Name(), client.Model())
	}

	// Cached until Rebuild.
	again, _ := rt.CloudClient()
	if again != client {
		t.Error("CloudClient should return the cached client")
	}

	// Provider/model switch rebuilds the client.
	ps.SetAPIKey("groq", "sk-groq")
	ps.SetActive("groq", "llama-3.1-8b-instant")
	rt.Rebuild()
	switched, err := rt.CloudClient()
	if err != nil {
		t.Fatalf("CloudClient after switch: %v", err)
	}
	if switched.Name() != "groq" || switched.Model() != "llama-3.1-8b-instant" {
		t.Errorf("switched client = %s/%s", switched.Name(), switched.Model())
	}
}

func TestProviderRuntimeClientFor(t *testing.T) {
	ps := NewProviderStore(filepath.Join(t.TempDir(), "providers.json"), nil)
	ps.SetAPIKey("perplexity", "sk-pplx") // catalog-only provider, no registered profile
	rt := NewProviderRuntime(ps, nil, testLogger())

	// Registered profile provider without a key → error.
	if _, err := rt.ClientFor("openai", "gpt-4o"); err == nil {
		t.Error("ClientFor without key should fail")
	}

	// Catalog-only provider gets a synthesized openai-compatible profile.
	client, err := rt.ClientFor("perplexity", "sonar-pro")
	if err != nil {
		t.Fatalf("ClientFor(perplexity): %v", err)
	}
	if client.Name() != "perplexity" || client.Model() != "sonar-pro" {
		t.Errorf("client = %s/%s", client.Name(), client.Model())
	}

	// Ollama needs no API key.
	local, err := rt.ClientFor("ollama", "llama3.1:8b")
	if err != nil {
		t.Fatalf("ClientFor(ollama): %v", err)
	}
	if local.Name() != "ollama" || local.Model() != "llama3.1:8b" {
		t.Errorf("local client = %s/%s", local.Name(), local.Model())
	}
}

func TestProviderRuntimeLocalModelOverride(t *testing.T) {
	rt := NewProviderRuntime(nil, nil, testLogger())

	// Falls back to the ollama profile default.
	if got := rt.LocalModel(); got == "" {
		t.Error("LocalModel should fall back to the profile default")
	}

	rt.SetLocalModel("qwen2.5:14b")
	if got := rt.LocalModel(); got != "qwen2.5:14b" {
		t.Errorf("LocalModel = %q, want override", got)
	}

	client, err := rt.LocalClient()
	if err != nil {
		t.Fatalf("LocalClient: %v", err)
	}
	if client.Model() != "qwen2.5:14b" {
		t.Errorf("local client model = %q", client.Model())
	}
}

// TestProviderRuntimeHostSwitchNoDeadlock exercises the lock ordering between
// HostStore (which invokes the switch callback under its own lock) and the
// runtime's client accessors. Run with -race; a regression here deadlocks.
func TestProviderRuntimeHostSwitchNoDeadlock(t *testing.T) {
	var rt *ProviderRuntime
	hs := NewHostStore(func(client *OllamaClient) {
		if rt != nil {
			rt.Rebuild()
		}
	})
	rt = NewProviderRuntime(nil, hs, testLogger())

	// Register a host directly (AddHost probes the network; keep it hermetic).
	hs.mu.Lock()
	hs.hosts["local"] = &OllamaHost{ID: "local", Name: "Local", Host: "127.0.0.1", Port: 11434, Status: "online"}
	hs.mu.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = hs.SetActive("local")
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_, _ = rt.LocalClient()
				_ = rt.LocalModel()
			}
		}()
	}
	wg.Wait()
}
