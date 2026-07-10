package axiosd

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"sync"

	"github.com/axios-os/axios/pkg/providers"
	"github.com/axios-os/axios/pkg/secrets"
)

// CloudProvider represents an AI API provider (Anthropic, OpenAI, Google, etc.).
type CloudProvider struct {
	ID         string   `json:"id"`         // "anthropic", "openai", "google", etc.
	Name       string   `json:"name"`       // Display name
	BaseURL    string   `json:"base_url"`   // API endpoint
	APIKey     string   `json:"-"`          // Never expose in JSON
	HasKey     bool     `json:"has_key"`    // Whether a key is configured
	Models     []string `json:"models"`     // Available models
	Active     bool     `json:"active"`     // Currently selected
	Compatible string   `json:"compatible"` // "anthropic" or "openai" (API format)
}

// providerCatalog returns the hardcoded list of supported providers.
func providerCatalog() []*CloudProvider {
	return []*CloudProvider{
		{
			ID:         "anthropic",
			Name:       "Anthropic",
			BaseURL:    "https://api.anthropic.com",
			Models:     []string{"claude-sonnet-4-6", "claude-opus-4-6", "claude-haiku-4-5"},
			Compatible: "anthropic",
		},
		{
			ID:         "openai",
			Name:       "OpenAI",
			BaseURL:    "https://api.openai.com",
			Models:     []string{"gpt-4o", "gpt-4o-mini", "gpt-4-turbo", "o1", "o1-mini", "o3-mini"},
			Compatible: "openai",
		},
		{
			ID:         "google",
			Name:       "Google (Gemini)",
			BaseURL:    "https://generativelanguage.googleapis.com/v1beta/openai",
			Models:     []string{"gemini-2.5-pro", "gemini-2.5-flash", "gemini-2.0-flash"},
			Compatible: "openai",
		},
		{
			ID:         "mistral",
			Name:       "Mistral",
			BaseURL:    "https://api.mistral.ai",
			Models:     []string{"mistral-large-latest", "mistral-medium-latest", "mistral-small-latest", "codestral-latest"},
			Compatible: "openai",
		},
		{
			ID:         "groq",
			Name:       "Groq",
			BaseURL:    "https://api.groq.com/openai",
			Models:     []string{"llama-3.1-70b-versatile", "llama-3.1-8b-instant", "mixtral-8x7b-32768", "gemma2-9b-it"},
			Compatible: "openai",
		},
		{
			ID:         "together",
			Name:       "Together AI",
			BaseURL:    "https://api.together.xyz",
			Models:     []string{"meta-llama/Llama-3.1-70B-Instruct", "meta-llama/Llama-3.1-8B-Instruct", "mistralai/Mixtral-8x7B-Instruct-v0.1"},
			Compatible: "openai",
		},
		{
			ID:         "openrouter",
			Name:       "OpenRouter",
			BaseURL:    "https://openrouter.ai/api",
			Models:     []string{"anthropic/claude-sonnet-4", "openai/gpt-4o", "google/gemini-2.5-pro", "meta-llama/llama-3.1-70b-instruct"},
			Compatible: "openai",
		},
		{
			ID:         "deepseek",
			Name:       "DeepSeek",
			BaseURL:    "https://api.deepseek.com",
			Models:     []string{"deepseek-chat", "deepseek-coder", "deepseek-reasoner"},
			Compatible: "openai",
		},
		{
			ID:         "xai",
			Name:       "xAI (Grok)",
			BaseURL:    "https://api.x.ai",
			Models:     []string{"grok-2", "grok-2-mini"},
			Compatible: "openai",
		},
		{
			ID:         "cohere",
			Name:       "Cohere",
			BaseURL:    "https://api.cohere.com/compatibility/v1",
			Models:     []string{"command-r-plus", "command-r", "command-light"},
			Compatible: "openai",
		},
		{
			ID:         "perplexity",
			Name:       "Perplexity",
			BaseURL:    "https://api.perplexity.ai",
			Models:     []string{"sonar-pro", "sonar", "sonar-reasoning"},
			Compatible: "openai",
		},
	}
}

// ProviderStore manages the collection of cloud API providers. API keys are
// held in memory as plaintext and encrypted at rest via pkg/secrets.
type ProviderStore struct {
	providers   map[string]*CloudProvider
	activeID    string
	activeModel string
	mu          sync.RWMutex
	filePath    string
	secrets     *secrets.Store
}

// NewProviderStore creates a new provider store initialized with the full
// catalog. sec may be nil (keys then persist as legacy base64 — only tests
// should do this).
func NewProviderStore(filePath string, sec *secrets.Store) *ProviderStore {
	ps := &ProviderStore{
		providers: make(map[string]*CloudProvider),
		filePath:  filePath,
		secrets:   sec,
	}
	for _, p := range providerCatalog() {
		ps.providers[p.ID] = p
	}
	return ps
}

// SetAPIKey sets the API key for a provider.
func (ps *ProviderStore) SetAPIKey(providerID, apiKey string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	p, exists := ps.providers[providerID]
	if !exists {
		return fmt.Errorf("unknown provider %q", providerID)
	}

	p.APIKey = apiKey
	p.HasKey = apiKey != ""
	return nil
}

// RemoveAPIKey removes the API key for a provider.
// If the provider is currently active, it is deactivated.
func (ps *ProviderStore) RemoveAPIKey(providerID string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	p, exists := ps.providers[providerID]
	if !exists {
		return fmt.Errorf("unknown provider %q", providerID)
	}

	p.APIKey = ""
	p.HasKey = false

	// Deactivate if this was the active provider
	if ps.activeID == providerID {
		p.Active = false
		ps.activeID = ""
		ps.activeModel = ""
	}

	return nil
}

// Credential returns the decrypted API key for a provider. This is the
// sanctioned accessor — callers must never reach into the store's internals.
func (ps *ProviderStore) Credential(providerID string) (string, bool) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	p, exists := ps.providers[providerID]
	if !exists || !p.HasKey {
		return "", false
	}
	return p.APIKey, true
}

// Provider returns a copy of one catalog entry (API key blanked), or nil.
func (ps *ProviderStore) Provider(providerID string) *CloudProvider {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	p, exists := ps.providers[providerID]
	if !exists {
		return nil
	}
	cp := *p
	cp.APIKey = ""
	cp.Active = (p.ID == ps.activeID)
	return &cp
}

// GetProviders returns all providers with API keys masked.
func (ps *ProviderStore) GetProviders() []*CloudProvider {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	result := make([]*CloudProvider, 0, len(ps.providers))
	for _, p := range ps.providers {
		cp := *p
		cp.APIKey = "" // never expose
		cp.Active = (p.ID == ps.activeID)
		result = append(result, &cp)
	}

	// Sort by ID for stable ordering
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})

	return result
}

// ProviderForModel returns the ID of a provider (with a key) whose catalog
// lists the given model, preferring the currently active provider. Returns ""
// when no provider matches.
func (ps *ProviderStore) ProviderForModel(model string) string {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	// Prefer the active provider when it lists the model.
	if active, ok := ps.providers[ps.activeID]; ok && active.HasKey {
		for _, m := range active.Models {
			if m == model {
				return active.ID
			}
		}
	}

	// Otherwise scan in stable ID order.
	ids := make([]string, 0, len(ps.providers))
	for id := range ps.providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		p := ps.providers[id]
		if !p.HasKey {
			continue
		}
		for _, m := range p.Models {
			if m == model {
				return id
			}
		}
	}
	return ""
}

// SetActive sets the active cloud provider and model. The model is free-form
// (custom model IDs are allowed); it only has to be non-empty.
func (ps *ProviderStore) SetActive(providerID, model string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	p, exists := ps.providers[providerID]
	if !exists {
		return fmt.Errorf("unknown provider %q", providerID)
	}

	if !p.HasKey {
		return fmt.Errorf("provider %q has no API key configured", providerID)
	}

	if model == "" {
		return fmt.Errorf("model is required")
	}

	// Deactivate old
	if old, ok := ps.providers[ps.activeID]; ok {
		old.Active = false
	}

	ps.activeID = providerID
	ps.activeModel = model
	p.Active = true

	return nil
}

// GetActive returns the active provider, or nil if none is set.
func (ps *ProviderStore) GetActive() *CloudProvider {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	if ps.activeID == "" {
		return nil
	}
	p, exists := ps.providers[ps.activeID]
	if !exists {
		return nil
	}
	cp := *p
	cp.Active = true
	return &cp
}

// GetActiveModel returns the name of the currently active model.
func (ps *ProviderStore) GetActiveModel() string {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.activeModel
}

// --- Persistence ---

// providersFile is the JSON structure saved to disk. Key values are either
// "axsec1:..." (AES-256-GCM via pkg/secrets) or legacy plain base64.
type providersFile struct {
	ActiveID    string            `json:"active_id"`
	ActiveModel string            `json:"active_model"`
	Keys        map[string]string `json:"keys"` // provider ID -> encrypted API key
}

// SaveToFile persists API keys to a JSON file, encrypting each key with the
// secrets store. Legacy base64 values loaded earlier are re-encrypted here
// (transparent upgrade).
func (ps *ProviderStore) SaveToFile() error {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	data := providersFile{
		ActiveID:    ps.activeID,
		ActiveModel: ps.activeModel,
		Keys:        make(map[string]string),
	}

	for id, p := range ps.providers {
		if p.APIKey == "" {
			continue
		}
		if ps.secrets != nil {
			enc, err := ps.secrets.Encrypt([]byte(p.APIKey))
			if err != nil {
				return fmt.Errorf("encrypt key for %s: %w", id, err)
			}
			data.Keys[id] = enc
		} else {
			data.Keys[id] = base64.StdEncoding.EncodeToString([]byte(p.APIKey))
		}
	}

	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal providers: %w", err)
	}
	if err := os.WriteFile(ps.filePath, raw, 0600); err != nil {
		return fmt.Errorf("write providers file: %w", err)
	}
	return nil
}

// LoadFromFile loads API keys from a JSON file. Values with the "axsec1:"
// prefix are decrypted via pkg/secrets; anything else is treated as legacy
// base64 and will be re-encrypted on the next SaveToFile.
func (ps *ProviderStore) LoadFromFile() error {
	raw, err := os.ReadFile(ps.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No file — nothing to load
		}
		return fmt.Errorf("read providers file: %w", err)
	}

	var data providersFile
	if err := json.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("parse providers file: %w", err)
	}

	ps.mu.Lock()
	defer ps.mu.Unlock()

	for id, stored := range data.Keys {
		p, exists := ps.providers[id]
		if !exists {
			continue
		}

		var key string
		if secrets.IsEncrypted(stored) {
			if ps.secrets == nil {
				continue // cannot decrypt without a secrets store
			}
			plaintext, err := ps.secrets.Decrypt(stored)
			if err != nil {
				continue // skip corrupted/foreign-key entries
			}
			key = string(plaintext)
		} else {
			decoded, err := base64.StdEncoding.DecodeString(stored)
			if err != nil {
				continue // skip corrupted entries
			}
			key = string(decoded) // legacy value: upgraded on next save
		}

		p.APIKey = key
		p.HasKey = key != ""
	}

	// Restore active provider/model only if valid
	if p, exists := ps.providers[data.ActiveID]; exists && p.HasKey {
		ps.activeID = data.ActiveID
		ps.activeModel = data.ActiveModel
		p.Active = true
	}

	return nil
}

// --- Server integration ---

// SetProviderStore sets the provider store on the server.
func (s *Server) SetProviderStore(store *ProviderStore) {
	s.providerStore = store
}

// defaultModelFor returns a sensible default model for a provider: the
// registered profile's default, else the first catalog model.
func (s *Server) defaultModelFor(providerID string) string {
	if p, ok := providers.Get(providerID); ok && p.DefaultModel != "" {
		return p.DefaultModel
	}
	if s.providerStore != nil {
		if cp := s.providerStore.Provider(providerID); cp != nil && len(cp.Models) > 0 {
			return cp.Models[0]
		}
	}
	return ""
}

// --- HTTP Handlers ---

// handleProviders handles GET /api/providers — list all providers with has_key status.
func (s *Server) handleProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, "GET required", http.StatusMethodNotAllowed)
		return
	}

	if s.providerStore == nil {
		s.jsonError(w, "provider management not initialized", http.StatusServiceUnavailable)
		return
	}

	providers := s.providerStore.GetProviders()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"providers":    providers,
		"active_model": s.providerStore.GetActiveModel(),
	})
}

// handleProviderKey handles POST /api/providers/key (set key) and DELETE /api/providers/key (remove key).
func (s *Server) handleProviderKey(w http.ResponseWriter, r *http.Request) {
	if s.providerStore == nil {
		s.jsonError(w, "provider management not initialized", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodPost:
		var req struct {
			Provider string `json:"provider"`
			APIKey   string `json:"api_key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.jsonError(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.Provider == "" || req.APIKey == "" {
			s.jsonError(w, "provider and api_key are required", http.StatusBadRequest)
			return
		}

		if err := s.providerStore.SetAPIKey(req.Provider, req.APIKey); err != nil {
			s.jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}

		// If nothing is active yet, activate this provider with its default
		// model so chat works immediately after the first key is added.
		if s.providerStore.GetActive() == nil {
			if model := s.defaultModelFor(req.Provider); model != "" {
				if err := s.providerStore.SetActive(req.Provider, model); err != nil {
					s.logger.Warn("failed to auto-activate provider", "provider", req.Provider, "error", err)
				}
			}
		}

		if s.runtime != nil {
			s.runtime.Rebuild()
		}
		if s.providerStore.GetActive() != nil {
			s.router.SetCloudAvailable(true)
		}

		s.logger.Info("API key set", "provider", req.Provider)

		// Auto-save
		if err := s.providerStore.SaveToFile(); err != nil {
			s.logger.Error("failed to save providers", "error", err)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "provider": req.Provider})

	case http.MethodDelete:
		providerID := r.URL.Query().Get("provider")
		if providerID == "" {
			s.jsonError(w, "provider query parameter required", http.StatusBadRequest)
			return
		}

		if err := s.providerStore.RemoveAPIKey(providerID); err != nil {
			s.jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}

		if s.runtime != nil {
			s.runtime.Rebuild()
		}
		if s.providerStore.GetActive() == nil {
			s.router.SetCloudAvailable(false)
		}

		s.logger.Info("API key removed", "provider", providerID)

		// Auto-save
		if err := s.providerStore.SaveToFile(); err != nil {
			s.logger.Error("failed to save providers", "error", err)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "removed": providerID})

	default:
		s.jsonError(w, "POST or DELETE required", http.StatusMethodNotAllowed)
	}
}

// handleProviderActivate handles POST /api/providers/activate — activate a provider and model.
func (s *Server) handleProviderActivate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	if s.providerStore == nil {
		s.jsonError(w, "provider management not initialized", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Provider == "" || req.Model == "" {
		s.jsonError(w, "provider and model are required", http.StatusBadRequest)
		return
	}

	if err := s.providerStore.SetActive(req.Provider, req.Model); err != nil {
		s.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Rebuild the runtime client for the new provider/model — no
	// provider-name special-casing; the profile registry handles the rest.
	if s.runtime != nil {
		s.runtime.Rebuild()
	}
	s.router.SetCloudAvailable(true)
	s.router.mode = RouteCloudOnly

	s.logger.Info("provider activated", "provider", req.Provider, "model", req.Model)

	// Auto-save
	if err := s.providerStore.SaveToFile(); err != nil {
		s.logger.Error("failed to save providers", "error", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"ok":       true,
		"provider": req.Provider,
		"model":    req.Model,
	})
}
