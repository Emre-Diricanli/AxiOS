package providers

import (
	"sort"
	"strings"
	"sync"
)

// Profile declaratively describes one provider (hermes-agent ProviderProfile).
// It carries no connection state; a Client binds a Profile to credentials and
// an http.Client at runtime.
type Profile struct {
	Name           string
	Aliases        []string
	APIMode        string   // chat_completions|anthropic_messages|ollama
	BaseURL        string   // includes API version prefix (e.g. https://api.openai.com/v1)
	EnvVars        []string // credential env vars, priority order
	DefaultModel   string
	FallbackModels []string
	DefaultHeaders map[string]string
	// Hooks (nil = default behavior):
	PrepareRequest func(req map[string]any, model string)  // last-mile quirks
	AuthHeader     func(key string) (header, value string) // default: Authorization: Bearer
}

// authHeader resolves the auth header for the given key, applying the
// default (Authorization: Bearer) when the profile has no AuthHeader hook.
func (p *Profile) authHeader(key string) (string, string) {
	if p.AuthHeader != nil {
		return p.AuthHeader(key)
	}
	return "Authorization", "Bearer " + key
}

// --- Registry ---

var registry = struct {
	mu       sync.RWMutex
	profiles map[string]*Profile // canonical name -> profile
	aliases  map[string]string   // alias -> canonical name
}{
	profiles: make(map[string]*Profile),
	aliases:  make(map[string]string),
}

// Register adds a profile to the global registry. Registration is
// last-writer-wins for both the canonical name and every alias, so tests and
// users can override built-ins.
func Register(p *Profile) {
	if p == nil || p.Name == "" {
		return
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()

	name := strings.ToLower(p.Name)
	registry.profiles[name] = p
	for _, a := range p.Aliases {
		registry.aliases[strings.ToLower(a)] = name
	}
}

// Get looks up a profile by canonical name or alias (case-insensitive).
func Get(nameOrAlias string) (*Profile, bool) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	key := strings.ToLower(nameOrAlias)
	if p, ok := registry.profiles[key]; ok {
		return p, true
	}
	if canonical, ok := registry.aliases[key]; ok {
		p, ok := registry.profiles[canonical]
		return p, ok
	}
	return nil, false
}

// List returns all registered profiles sorted by canonical name.
func List() []*Profile {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	result := make([]*Profile, 0, len(registry.profiles))
	for _, p := range registry.profiles {
		result = append(result, p)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}
