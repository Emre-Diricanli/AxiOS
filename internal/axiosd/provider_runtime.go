package axiosd

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/axios-os/axios/pkg/providers"
)

// ProviderRuntime resolves the active *providers.Client for the daemon from
// the ProviderStore (cloud providers) and HostStore (ollama base URL). Clients
// are cached and rebuilt when the active provider, model, or host changes
// (callers signal changes via Rebuild / SetLocalModel).
type ProviderRuntime struct {
	store      *ProviderStore
	hosts      *HostStore
	logger     *slog.Logger
	httpClient *http.Client

	mu         sync.Mutex
	cloud      *providers.Client
	local      *providers.Client
	localModel string
}

// NewProviderRuntime creates a runtime over the given stores. The injected
// http.Client is shared by every provider client (nil gets a default without
// a timeout, since chat responses stream).
func NewProviderRuntime(store *ProviderStore, hosts *HostStore, logger *slog.Logger) *ProviderRuntime {
	if logger == nil {
		logger = slog.Default()
	}
	return &ProviderRuntime{
		store:      store,
		hosts:      hosts,
		logger:     logger,
		httpClient: &http.Client{},
	}
}

// Rebuild drops the cached clients so the next access reflects the current
// provider store / host store state. Call after provider or model switches.
func (rt *ProviderRuntime) Rebuild() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.cloud = nil
	rt.local = nil
}

// SetLocalModel overrides the model used by the local (ollama) client.
func (rt *ProviderRuntime) SetLocalModel(model string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.localModel = model
	rt.local = nil
}

// LocalModel returns the effective local model name (override, else the
// active host's first model, else the ollama profile default).
func (rt *ProviderRuntime) LocalModel() string {
	rt.mu.Lock()
	override := rt.localModel
	rt.mu.Unlock()
	if override != "" {
		return override
	}
	if rt.hosts != nil {
		if h := rt.hosts.GetActive(); h != nil && len(h.Models) > 0 {
			return h.Models[0]
		}
	}
	if p, ok := providers.Get("ollama"); ok {
		return p.DefaultModel
	}
	return ""
}

// ClientForBackend returns the active client for the routed backend.
func (rt *ProviderRuntime) ClientForBackend(backend Backend) (*providers.Client, error) {
	if backend == BackendLocal {
		return rt.LocalClient()
	}
	return rt.CloudClient()
}

// Lock ordering note: rt.mu is never held across calls into the ProviderStore
// or HostStore. The HostStore invokes the daemon's host-switch callback (which
// calls Rebuild → rt.mu) while holding its own lock, so acquiring store locks
// under rt.mu would risk an AB/BA deadlock.

// CloudClient returns the client for the active cloud provider, building and
// caching it on first access.
func (rt *ProviderRuntime) CloudClient() (*providers.Client, error) {
	rt.mu.Lock()
	if rt.cloud != nil {
		client := rt.cloud
		rt.mu.Unlock()
		return client, nil
	}
	rt.mu.Unlock()

	if rt.store == nil {
		return nil, fmt.Errorf("no provider store configured")
	}
	active := rt.store.GetActive()
	if active == nil {
		return nil, fmt.Errorf("no cloud provider configured — add an API key in Settings")
	}
	key, ok := rt.store.Credential(active.ID)
	if !ok {
		return nil, fmt.Errorf("provider %q has no API key configured", active.ID)
	}
	model := rt.store.GetActiveModel()

	client := providers.NewClient(rt.profileFor(active.ID), key, "", model, rt.httpClient)

	rt.mu.Lock()
	if rt.cloud == nil {
		rt.cloud = client
	} else {
		client = rt.cloud // another goroutine won the build race
	}
	rt.mu.Unlock()
	return client, nil
}

// LocalClient returns a client speaking to the active Ollama host, building
// and caching it on first access.
func (rt *ProviderRuntime) LocalClient() (*providers.Client, error) {
	rt.mu.Lock()
	if rt.local != nil {
		client := rt.local
		rt.mu.Unlock()
		return client, nil
	}
	model := rt.localModel
	rt.mu.Unlock()

	profile, ok := providers.Get("ollama")
	if !ok {
		return nil, fmt.Errorf("ollama profile not registered")
	}

	baseURL := rt.localBaseURL()
	if model == "" {
		if rt.hosts != nil {
			if h := rt.hosts.GetActive(); h != nil && len(h.Models) > 0 {
				model = h.Models[0]
			}
		}
	}

	client := providers.NewClient(profile, "", baseURL, model, rt.httpClient)

	rt.mu.Lock()
	if rt.local == nil {
		rt.local = client
	} else {
		client = rt.local // another goroutine won the build race
	}
	rt.mu.Unlock()
	return client, nil
}

// ClientFor builds a client for an explicitly named provider and model — used
// for the fallback chain and the /v1 facade. It is never cached: it does not
// change the runtime's notion of the active provider.
func (rt *ProviderRuntime) ClientFor(providerID, model string) (*providers.Client, error) {
	if strings.EqualFold(providerID, "ollama") {
		profile, ok := providers.Get("ollama")
		if !ok {
			return nil, fmt.Errorf("ollama profile not registered")
		}
		return providers.NewClient(profile, "", rt.localBaseURL(), model, rt.httpClient), nil
	}

	if rt.store == nil {
		return nil, fmt.Errorf("no provider store configured")
	}
	key, ok := rt.store.Credential(providerID)
	if !ok {
		return nil, fmt.Errorf("provider %q has no API key configured", providerID)
	}
	return providers.NewClient(rt.profileFor(providerID), key, "", model, rt.httpClient), nil
}

// Current returns the provider and model names for the given backend, or
// empty strings when that backend has nothing configured.
func (rt *ProviderRuntime) Current(backend Backend) (provider, model string) {
	if backend == BackendLocal {
		return "ollama", rt.LocalModel()
	}
	if rt.store == nil {
		return "", ""
	}
	active := rt.store.GetActive()
	if active == nil {
		return "", ""
	}
	return active.ID, rt.store.GetActiveModel()
}

// localBaseURL resolves the Ollama base URL from the active host, falling
// back to the profile default. Must be called WITHOUT rt.mu held (it takes
// the host store's lock — see the lock ordering note above).
func (rt *ProviderRuntime) localBaseURL() string {
	if rt.hosts != nil {
		if h := rt.hosts.GetActive(); h != nil {
			return fmt.Sprintf("http://%s:%d", h.Host, h.Port)
		}
	}
	return "" // NewClient falls back to the profile's BaseURL
}

// profileFor resolves the registered profile for a provider ID. Catalog-only
// providers without a registered profile (e.g. cohere, perplexity) get a
// synthesized OpenAI-compatible profile using the catalog base URL.
func (rt *ProviderRuntime) profileFor(providerID string) *providers.Profile {
	if p, ok := providers.Get(providerID); ok {
		return p
	}

	base := ""
	if rt.store != nil {
		if cp := rt.store.Provider(providerID); cp != nil {
			base = cp.BaseURL
		}
	}
	// The transports append only the final path segment (/chat/completions),
	// so make sure the base URL carries a version prefix.
	if base != "" && !strings.Contains(base, "/v1") && !strings.HasSuffix(base, "/openai") {
		base = strings.TrimRight(base, "/") + "/v1"
	}
	rt.logger.Debug("synthesized openai-compatible profile", "provider", providerID, "base_url", base)
	return &providers.Profile{
		Name:    providerID,
		APIMode: providers.APIModeChatCompletions,
		BaseURL: base,
	}
}
