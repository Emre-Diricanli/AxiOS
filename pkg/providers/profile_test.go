package providers

import (
	"testing"
)

// restoreBuiltins re-registers the pristine built-in profiles so registry
// mutations from a test never leak into other tests.
func restoreBuiltins(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		for _, p := range Builtins() {
			Register(p)
		}
	})
}

func TestRegistryBuiltins(t *testing.T) {
	wantNames := []string{
		"anthropic", "openai", "openrouter", "groq", "deepseek", "xai",
		"mistral", "together", "fireworks", "google", "ollama", "custom",
	}
	for _, name := range wantNames {
		p, ok := Get(name)
		if !ok {
			t.Errorf("builtin %q not registered", name)
			continue
		}
		if p.Name != name {
			t.Errorf("Get(%q).Name = %q", name, p.Name)
		}
	}
	if len(List()) < len(wantNames) {
		t.Errorf("List() returned %d profiles, want >= %d", len(List()), len(wantNames))
	}
}

func TestRegistryAliasResolution(t *testing.T) {
	tests := []struct {
		alias string
		want  string
	}{
		{"claude", "anthropic"},
		{"CLAUDE", "anthropic"}, // case-insensitive
		{"gemini", "google"},
		{"grok", "xai"},
		{"anthropic", "anthropic"}, // canonical name still works
	}
	for _, tt := range tests {
		t.Run(tt.alias, func(t *testing.T) {
			p, ok := Get(tt.alias)
			if !ok {
				t.Fatalf("Get(%q) not found", tt.alias)
			}
			if p.Name != tt.want {
				t.Errorf("Get(%q).Name = %q, want %q", tt.alias, p.Name, tt.want)
			}
		})
	}

	if _, ok := Get("no-such-provider"); ok {
		t.Error("Get of unknown name should fail")
	}
}

func TestRegistryLastWriterWins(t *testing.T) {
	restoreBuiltins(t)

	override := &Profile{
		Name:         "anthropic",
		Aliases:      []string{"claude", "my-claude"},
		APIMode:      APIModeChatCompletions, // deliberately different
		BaseURL:      "http://localhost:9999/v1",
		DefaultModel: "test-model",
	}
	Register(override)

	got, ok := Get("anthropic")
	if !ok {
		t.Fatal("override not found by name")
	}
	if got != override {
		t.Errorf("Get returned %+v, want the override", got)
	}

	// Aliases follow the last writer too, including new ones.
	for _, alias := range []string{"claude", "my-claude"} {
		p, ok := Get(alias)
		if !ok || p != override {
			t.Errorf("Get(%q) = %+v, want the override", alias, p)
		}
	}

	// A second override wins again.
	second := &Profile{Name: "anthropic", APIMode: APIModeAnthropicMessages, BaseURL: "http://other:1/v1"}
	Register(second)
	if p, _ := Get("anthropic"); p != second {
		t.Errorf("second override did not win: %+v", p)
	}
	// Alias registered by the first override still resolves to the canonical name.
	if p, ok := Get("my-claude"); !ok || p != second {
		t.Errorf("alias should resolve to latest registration, got %+v", p)
	}
}

func TestRegistryListSorted(t *testing.T) {
	profiles := List()
	for i := 1; i < len(profiles); i++ {
		if profiles[i-1].Name > profiles[i].Name {
			t.Fatalf("List not sorted: %q before %q", profiles[i-1].Name, profiles[i].Name)
		}
	}
}

func TestProfileAuthHeaderDefault(t *testing.T) {
	p := &Profile{Name: "x"}
	h, v := p.authHeader("key123")
	if h != "Authorization" || v != "Bearer key123" {
		t.Errorf("default auth header = (%q, %q)", h, v)
	}

	anthropic := mustGet(t, "anthropic")
	if h, v := anthropic.authHeader("sk-ant-api03-k"); h != "x-api-key" || v != "sk-ant-api03-k" {
		t.Errorf("anthropic api key auth = (%q, %q)", h, v)
	}
	if h, v := anthropic.authHeader("sk-ant-oat01-k"); h != "Authorization" || v != "Bearer sk-ant-oat01-k" {
		t.Errorf("anthropic oauth auth = (%q, %q)", h, v)
	}
}
