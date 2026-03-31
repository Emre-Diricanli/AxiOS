package claused

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// OllamaHost represents a remote or local machine running the Ollama server.
type OllamaHost struct {
	ID      string   `json:"id"`       // unique identifier (slug from name)
	Name    string   `json:"name"`     // display name, e.g., "Jetson Nano", "Mac Studio"
	Host    string   `json:"host"`     // hostname or IP
	Port    int      `json:"port"`     // default 11434
	Status  string   `json:"status"`   // "online", "offline", "checking"
	Models  []string `json:"models"`   // list of model names available on this host
	Active  bool     `json:"active"`   // is this the currently selected host
	GPUInfo string   `json:"gpu_info"` // GPU description if available
}

// HostStore manages a collection of Ollama host connections.
type HostStore struct {
	hosts    map[string]*OllamaHost
	activeID string
	mu       sync.RWMutex
	onSwitch func(client *OllamaClient) // called when active host changes
}

// NewHostStore creates a new host store with a callback for when the active host switches.
// The onSwitch callback receives the new OllamaClient when SetActive is called.
func NewHostStore(onSwitch func(client *OllamaClient)) *HostStore {
	return &HostStore{
		hosts:    make(map[string]*OllamaHost),
		onSwitch: onSwitch,
	}
}

// slugify converts a display name into a URL-friendly identifier.
func slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, " ", "-")
	// Remove anything that isn't alphanumeric or dash
	var b strings.Builder
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// AddHost registers a new Ollama host and probes it for availability.
func (hs *HostStore) AddHost(name, host string, port int) (*OllamaHost, error) {
	if name == "" {
		return nil, fmt.Errorf("host name is required")
	}
	if host == "" {
		return nil, fmt.Errorf("host address is required")
	}
	if port <= 0 {
		port = 11434
	}

	id := slugify(name)
	if id == "" {
		return nil, fmt.Errorf("name produces an empty identifier")
	}

	hs.mu.Lock()
	if _, exists := hs.hosts[id]; exists {
		hs.mu.Unlock()
		return nil, fmt.Errorf("host with id %q already exists", id)
	}
	hs.mu.Unlock()

	h := &OllamaHost{
		ID:     id,
		Name:   name,
		Host:   host,
		Port:   port,
		Status: "checking",
	}

	// Probe the host
	client := NewOllamaClient(host, port, "")
	if err := pingWithTimeout(client); err != nil {
		h.Status = "offline"
	} else {
		h.Status = "online"
		if models, err := client.ListModels(); err == nil {
			h.Models = models
		}
	}

	hs.mu.Lock()
	hs.hosts[id] = h
	hs.mu.Unlock()

	return h, nil
}

// RemoveHost removes a host by ID.
// Cannot remove the active host. Cannot remove "local" if it is the only host.
func (hs *HostStore) RemoveHost(id string) error {
	hs.mu.Lock()
	defer hs.mu.Unlock()

	if _, exists := hs.hosts[id]; !exists {
		return fmt.Errorf("host %q not found", id)
	}

	if hs.activeID == id {
		return fmt.Errorf("cannot remove the active host — switch to another host first")
	}

	if id == "local" && len(hs.hosts) == 1 {
		return fmt.Errorf("cannot remove the last remaining host")
	}

	delete(hs.hosts, id)
	return nil
}

// GetHosts returns all hosts with current active flag set correctly.
func (hs *HostStore) GetHosts() []*OllamaHost {
	hs.mu.RLock()
	defer hs.mu.RUnlock()

	result := make([]*OllamaHost, 0, len(hs.hosts))
	for _, h := range hs.hosts {
		// Copy so we don't expose internal pointers in a racy way
		copy := *h
		copy.Active = (h.ID == hs.activeID)
		result = append(result, &copy)
	}
	return result
}

// SetActive sets the active host by ID. It creates a new OllamaClient
// pointing to the host and invokes the onSwitch callback.
func (hs *HostStore) SetActive(id string) error {
	hs.mu.Lock()
	defer hs.mu.Unlock()

	h, exists := hs.hosts[id]
	if !exists {
		return fmt.Errorf("host %q not found", id)
	}

	// Determine a model to use — pick the first available if any
	model := ""
	if len(h.Models) > 0 {
		model = h.Models[0]
	}

	client := NewOllamaClient(h.Host, h.Port, model)

	// Mark old active host as inactive
	if old, ok := hs.hosts[hs.activeID]; ok {
		old.Active = false
	}

	hs.activeID = id
	h.Active = true

	if hs.onSwitch != nil {
		hs.onSwitch(client)
	}

	return nil
}

// CheckHealth pings a specific host and updates its status and model list.
func (hs *HostStore) CheckHealth(id string) {
	hs.mu.RLock()
	h, exists := hs.hosts[id]
	hs.mu.RUnlock()

	if !exists {
		return
	}

	hs.mu.Lock()
	h.Status = "checking"
	hs.mu.Unlock()

	client := NewOllamaClient(h.Host, h.Port, "")

	if err := pingWithTimeout(client); err != nil {
		hs.mu.Lock()
		h.Status = "offline"
		h.Models = nil
		hs.mu.Unlock()
		return
	}

	models, _ := client.ListModels()

	hs.mu.Lock()
	h.Status = "online"
	h.Models = models
	hs.mu.Unlock()
}

// CheckAllHealth pings all hosts concurrently and updates their statuses.
func (hs *HostStore) CheckAllHealth() {
	hs.mu.RLock()
	ids := make([]string, 0, len(hs.hosts))
	for id := range hs.hosts {
		ids = append(ids, id)
	}
	hs.mu.RUnlock()

	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(hostID string) {
			defer wg.Done()
			hs.CheckHealth(hostID)
		}(id)
	}
	wg.Wait()
}

// GetActive returns the currently active host, or nil if none is set.
func (hs *HostStore) GetActive() *OllamaHost {
	hs.mu.RLock()
	defer hs.mu.RUnlock()

	if hs.activeID == "" {
		return nil
	}
	h, exists := hs.hosts[hs.activeID]
	if !exists {
		return nil
	}
	copy := *h
	copy.Active = true
	return &copy
}

// --- Persistence ---

// hostsFile is the JSON structure saved to disk.
type hostsFile struct {
	Hosts    []*OllamaHost `json:"hosts"`
	ActiveID string        `json:"active_id"`
}

// SaveToFile persists the host list to a JSON file.
func (hs *HostStore) SaveToFile(path string) error {
	hs.mu.RLock()
	defer hs.mu.RUnlock()

	data := hostsFile{
		Hosts:    make([]*OllamaHost, 0, len(hs.hosts)),
		ActiveID: hs.activeID,
	}
	for _, h := range hs.hosts {
		copy := *h
		copy.Active = (h.ID == hs.activeID)
		data.Hosts = append(data.Hosts, &copy)
	}

	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal hosts: %w", err)
	}
	if err := os.WriteFile(path, raw, 0644); err != nil {
		return fmt.Errorf("write hosts file: %w", err)
	}
	return nil
}

// LoadFromFile loads the host list from a JSON file.
// Hosts loaded from file are re-probed for current status.
func (hs *HostStore) LoadFromFile(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No file — nothing to load
		}
		return fmt.Errorf("read hosts file: %w", err)
	}

	var data hostsFile
	if err := json.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("parse hosts file: %w", err)
	}

	hs.mu.Lock()
	for _, h := range data.Hosts {
		// Don't overwrite hosts that are already registered (e.g., "local")
		if _, exists := hs.hosts[h.ID]; exists {
			continue
		}
		h.Status = "offline" // will be refreshed by CheckAllHealth
		hs.hosts[h.ID] = h
	}
	// Restore active ID only if the host exists
	if _, exists := hs.hosts[data.ActiveID]; exists {
		hs.activeID = data.ActiveID
	}
	hs.mu.Unlock()

	return nil
}

// pingWithTimeout pings an Ollama host with a short timeout.
func pingWithTimeout(client *OllamaClient) error {
	// Use a dedicated http client with a timeout for health checks
	hc := &http.Client{Timeout: 5 * time.Second}
	resp, err := hc.Get(client.BaseURL() + "/api/tags")
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama returned %d", resp.StatusCode)
	}
	return nil
}

// --- Server integration ---

// SetHostStore sets the host store on the server.
func (s *Server) SetHostStore(store *HostStore) {
	s.hostStore = store
}

// --- HTTP Handlers ---

// handleHosts handles GET (list) and POST (add) for /api/hosts.
func (s *Server) handleHosts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleHostsList(w, r)
	case http.MethodPost:
		s.handleHostsAdd(w, r)
	case http.MethodDelete:
		s.handleHostsRemove(w, r)
	default:
		s.jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleHostsList(w http.ResponseWriter, r *http.Request) {
	if s.hostStore == nil {
		s.jsonError(w, "host management not initialized", http.StatusServiceUnavailable)
		return
	}

	hosts := s.hostStore.GetHosts()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"hosts":    hosts,
		"active":   s.hostStore.GetActive(),
	})
}

func (s *Server) handleHostsAdd(w http.ResponseWriter, r *http.Request) {
	if s.hostStore == nil {
		s.jsonError(w, "host management not initialized", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Name string `json:"name"`
		Host string `json:"host"`
		Port int    `json:"port"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	host, err := s.hostStore.AddHost(req.Name, req.Host, req.Port)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.logger.Info("added Ollama host", "id", host.ID, "host", host.Host, "port", host.Port, "status", host.Status)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(host)
}

func (s *Server) handleHostsRemove(w http.ResponseWriter, r *http.Request) {
	if s.hostStore == nil {
		s.jsonError(w, "host management not initialized", http.StatusServiceUnavailable)
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		s.jsonError(w, "id parameter required", http.StatusBadRequest)
		return
	}

	if err := s.hostStore.RemoveHost(id); err != nil {
		s.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.logger.Info("removed Ollama host", "id", id)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"ok": "true", "removed": id})
}

// handleHostAction handles POST /api/hosts/activate?id=X to set the active host.
func (s *Server) handleHostAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if s.hostStore == nil {
		s.jsonError(w, "host management not initialized", http.StatusServiceUnavailable)
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		s.jsonError(w, "id parameter required", http.StatusBadRequest)
		return
	}

	if err := s.hostStore.SetActive(id); err != nil {
		s.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.logger.Info("switched active Ollama host", "id", id)

	active := s.hostStore.GetActive()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "active": active})
}

// handleHostHealth handles POST /api/hosts/health to check host health.
// Use ?id=X for a single host, or no query to check all.
func (s *Server) handleHostHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if s.hostStore == nil {
		s.jsonError(w, "host management not initialized", http.StatusServiceUnavailable)
		return
	}

	id := r.URL.Query().Get("id")
	if id != "" {
		s.hostStore.CheckHealth(id)
	} else {
		s.hostStore.CheckAllHealth()
	}

	hosts := s.hostStore.GetHosts()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"hosts": hosts})
}
