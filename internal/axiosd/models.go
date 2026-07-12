package axiosd

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/axios-os/axios/internal/ollamactl"
)

// InstalledModel represents a locally installed Ollama model. It is an alias
// of the shared ollamactl.Model so the REST JSON shape stays unchanged.
type InstalledModel = ollamactl.Model

// MarketplaceModel represents a model available for download from the Ollama registry.
type MarketplaceModel struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Category    string   `json:"category"`
	Parameters  string   `json:"parameters"`
	Recommended bool     `json:"recommended"`
}

// PullProgress represents progress of a model pull operation. It is an alias
// of the shared ollamactl.PullProgress so the SSE JSON shape stays unchanged.
type PullProgress = ollamactl.PullProgress

// getInstalledModels fetches the list of locally installed models from Ollama.
func getInstalledModels(ollamaURL string) ([]InstalledModel, error) {
	return ollamactl.New(ollamaURL, nil).ListModels()
}

// getMarketplaceModels returns a curated list of popular models available from the Ollama registry.
func getMarketplaceModels() []MarketplaceModel {
	return []MarketplaceModel{
		// General
		{
			Name:        "llama3.1",
			Description: "Meta's Llama 3.1 — high-quality general-purpose model with strong reasoning and instruction following",
			Tags:        []string{"8b", "70b"},
			Category:    "general",
			Parameters:  "8B",
			Recommended: true,
		},
		{
			Name:        "llama3.2",
			Description: "Meta's Llama 3.2 — compact and efficient models for edge and mobile deployments",
			Tags:        []string{"1b", "3b"},
			Category:    "general",
			Parameters:  "3B",
			Recommended: false,
		},
		{
			Name:        "mistral",
			Description: "Mistral 7B — fast and capable open-weight model from Mistral AI",
			Tags:        []string{"7b"},
			Category:    "general",
			Parameters:  "7B",
			Recommended: false,
		},
		{
			Name:        "mixtral",
			Description: "Mixtral 8x7B — mixture-of-experts model offering strong performance with efficient inference",
			Tags:        []string{"8x7b"},
			Category:    "general",
			Parameters:  "8x7B",
			Recommended: false,
		},
		{
			Name:        "gemma2",
			Description: "Google's Gemma 2 — lightweight and efficient models built from Gemini research",
			Tags:        []string{"2b", "9b", "27b"},
			Category:    "general",
			Parameters:  "9B",
			Recommended: false,
		},
		{
			Name:        "phi3",
			Description: "Microsoft's Phi-3 — small language models with surprisingly strong reasoning capabilities",
			Tags:        []string{"3.8b", "14b"},
			Category:    "general",
			Parameters:  "3.8B",
			Recommended: false,
		},
		{
			Name:        "qwen2.5",
			Description: "Alibaba's Qwen 2.5 — multilingual model with excellent coding and math abilities",
			Tags:        []string{"7b", "14b", "32b", "72b"},
			Category:    "general",
			Parameters:  "14B",
			Recommended: true,
		},
		{
			Name:        "deepseek-v2.5",
			Description: "DeepSeek V2.5 — strong open model combining chat and coding capabilities",
			Tags:        []string{"latest"},
			Category:    "general",
			Parameters:  "236B",
			Recommended: false,
		},

		// Code
		{
			Name:        "codellama",
			Description: "Meta's Code Llama — specialized for code generation, completion, and understanding",
			Tags:        []string{"7b", "13b", "34b"},
			Category:    "code",
			Parameters:  "7B",
			Recommended: true,
		},
		{
			Name:        "deepseek-coder-v2",
			Description: "DeepSeek Coder V2 — high-performance coding model supporting 300+ languages",
			Tags:        []string{"latest"},
			Category:    "code",
			Parameters:  "236B",
			Recommended: false,
		},
		{
			Name:        "starcoder2",
			Description: "BigCode's StarCoder2 — trained on The Stack v2 for strong code completion",
			Tags:        []string{"3b", "7b", "15b"},
			Category:    "code",
			Parameters:  "7B",
			Recommended: false,
		},
		{
			Name:        "qwen2.5-coder",
			Description: "Qwen 2.5 Coder — code-specialized variant with strong generation and understanding",
			Tags:        []string{"7b", "14b", "32b"},
			Category:    "code",
			Parameters:  "14B",
			Recommended: false,
		},

		// Vision
		{
			Name:        "llava",
			Description: "LLaVA — multimodal model that can understand and reason about images",
			Tags:        []string{"7b", "13b"},
			Category:    "vision",
			Parameters:  "7B",
			Recommended: false,
		},
		{
			Name:        "llama3.2-vision",
			Description: "Meta's Llama 3.2 Vision — multimodal model with native image understanding",
			Tags:        []string{"11b", "90b"},
			Category:    "vision",
			Parameters:  "11B",
			Recommended: false,
		},
		{
			Name:        "moondream",
			Description: "Moondream — tiny but capable vision-language model for resource-constrained environments",
			Tags:        []string{"1.8b"},
			Category:    "vision",
			Parameters:  "1.8B",
			Recommended: false,
		},

		// Embedding
		{
			Name:        "nomic-embed-text",
			Description: "Nomic Embed Text — high-quality text embeddings with long context support",
			Tags:        []string{"latest"},
			Category:    "embedding",
			Parameters:  "137M",
			Recommended: false,
		},
		{
			Name:        "mxbai-embed-large",
			Description: "Mixedbread Embed Large — state-of-the-art embedding model for retrieval tasks",
			Tags:        []string{"latest"},
			Category:    "embedding",
			Parameters:  "335M",
			Recommended: false,
		},
		{
			Name:        "all-minilm",
			Description: "All-MiniLM — lightweight and fast sentence embedding model",
			Tags:        []string{"latest"},
			Category:    "embedding",
			Parameters:  "23M",
			Recommended: false,
		},
	}
}

// pullModel pulls a model from the Ollama registry with streaming progress updates.
func pullModel(ollamaURL, modelName string, progressChan chan<- PullProgress) error {
	defer close(progressChan)

	return ollamactl.New(ollamaURL, nil).PullModel(modelName, func(p ollamactl.PullProgress) {
		progressChan <- p
	})
}

// deleteModel deletes a locally installed model from Ollama.
func deleteModel(ollamaURL, modelName string) error {
	return ollamactl.New(ollamaURL, nil).DeleteModel(modelName)
}

// getModelInfo fetches detailed information about a specific model from Ollama.
func getModelInfo(ollamaURL, modelName string) (*ollamactl.ShowResponse, error) {
	return ollamactl.New(ollamaURL, nil).ModelInfo(modelName)
}

// --- HTTP Handlers ---

// handleModelsInstalled returns the list of locally installed models.
func (s *Server) handleModelsInstalled(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, "GET required", http.StatusMethodNotAllowed)
		return
	}

	// No reachable Ollama host is a normal state (cloud-only setups) — render
	// an empty library instead of failing the whole Models page.
	if s.ollama == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"models": []any{}, "warning": "Ollama is not configured"})
		return
	}

	models, err := getInstalledModels(s.ollama.BaseURL())
	if err != nil {
		s.logger.Error("failed to get installed models", "error", err)
		s.jsonError(w, fmt.Sprintf("Failed to get installed models: %v", err), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"models": models})
}

// handleModelsMarketplace returns the curated list of available models.
func (s *Server) handleModelsMarketplace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, "GET required", http.StatusMethodNotAllowed)
		return
	}

	models := getMarketplaceModels()

	// If Ollama is connected, mark which models are already installed
	var installed map[string]bool
	if s.ollama != nil {
		installedModels, err := getInstalledModels(s.ollama.BaseURL())
		if err == nil {
			installed = make(map[string]bool, len(installedModels))
			for _, m := range installedModels {
				installed[m.Name] = true
			}
		}
	}

	type marketplaceEntry struct {
		MarketplaceModel
		Installed bool `json:"installed"`
	}

	entries := make([]marketplaceEntry, 0, len(models))
	for _, m := range models {
		entry := marketplaceEntry{
			MarketplaceModel: m,
			Installed:        installed[m.Name],
		}
		entries = append(entries, entry)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"models": entries})
}

// handleModelPull pulls a model from the Ollama registry with SSE streaming progress.
func (s *Server) handleModelPull(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	if s.ollama == nil {
		s.jsonError(w, "Ollama is not configured", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		s.jsonError(w, "model name is required", http.StatusBadRequest)
		return
	}

	s.logger.Info("pulling model", "name", req.Name)

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.jsonError(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	progressChan := make(chan PullProgress, 16)

	// Start the pull in a goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- pullModel(s.ollama.BaseURL(), req.Name, progressChan)
	}()

	// Stream progress updates as SSE events
	for progress := range progressChan {
		data, err := json.Marshal(progress)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	// Check for errors from the pull operation
	if err := <-errChan; err != nil {
		s.logger.Error("model pull failed", "name", req.Name, "error", err)
		errData, _ := json.Marshal(PullProgress{
			Status:  fmt.Sprintf("error: %v", err),
			Percent: -1,
		})
		fmt.Fprintf(w, "data: %s\n\n", errData)
		flusher.Flush()
		return
	}

	// Send completion event
	doneData, _ := json.Marshal(PullProgress{
		Status:  "success",
		Percent: 100,
	})
	fmt.Fprintf(w, "data: %s\n\n", doneData)
	flusher.Flush()

	s.logger.Info("model pull completed", "name", req.Name)
}

// handleModelDelete deletes a locally installed model.
func (s *Server) handleModelDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		s.jsonError(w, "DELETE required", http.StatusMethodNotAllowed)
		return
	}

	if s.ollama == nil {
		s.jsonError(w, "Ollama is not configured", http.StatusServiceUnavailable)
		return
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		s.jsonError(w, "name query parameter is required", http.StatusBadRequest)
		return
	}

	s.logger.Info("deleting model", "name", name)

	if err := deleteModel(s.ollama.BaseURL(), name); err != nil {
		s.logger.Error("model delete failed", "name", name, "error", err)
		s.jsonError(w, fmt.Sprintf("Failed to delete model: %v", err), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "deleted": name})

	s.logger.Info("model deleted", "name", name)
}

// handleModelInfo returns detailed information about a specific model.
func (s *Server) handleModelInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, "GET required", http.StatusMethodNotAllowed)
		return
	}

	if s.ollama == nil {
		s.jsonError(w, "Ollama is not configured", http.StatusServiceUnavailable)
		return
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		s.jsonError(w, "name query parameter is required", http.StatusBadRequest)
		return
	}

	info, err := getModelInfo(s.ollama.BaseURL(), name)
	if err != nil {
		s.logger.Error("model info failed", "name", name, "error", err)
		s.jsonError(w, fmt.Sprintf("Failed to get model info: %v", err), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"name":       name,
		"license":    info.License,
		"modelfile":  info.Modelfile,
		"parameters": info.Parameters,
		"template":   info.Template,
		"details": map[string]any{
			"format":             info.Details.Format,
			"family":             info.Details.Family,
			"families":           info.Details.Families,
			"parameter_size":     info.Details.ParameterSize,
			"quantization_level": info.Details.QuantizationLevel,
		},
	})
}
