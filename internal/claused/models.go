package claused

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// InstalledModel represents a locally installed Ollama model.
type InstalledModel struct {
	Name         string `json:"name"`
	Size         int64  `json:"size"`
	SizeHuman    string `json:"size_human"`
	Modified     string `json:"modified"`
	Family       string `json:"family"`
	Parameters   string `json:"parameters"`
	Quantization string `json:"quantization"`
}

// MarketplaceModel represents a model available for download from the Ollama registry.
type MarketplaceModel struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Category    string   `json:"category"`
	Parameters  string   `json:"parameters"`
	Recommended bool     `json:"recommended"`
}

// PullProgress represents progress of a model pull operation.
type PullProgress struct {
	Status    string  `json:"status"`
	Digest    string  `json:"digest,omitempty"`
	Total     int64   `json:"total,omitempty"`
	Completed int64   `json:"completed,omitempty"`
	Percent   float64 `json:"percent"`
}

// formatSize converts bytes to a human-readable string.
func formatSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)
	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.1f TB", float64(bytes)/float64(TB))
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// ollamaTagsResponse is the response from Ollama's GET /api/tags endpoint.
type ollamaTagsResponse struct {
	Models []ollamaModelEntry `json:"models"`
}

type ollamaModelEntry struct {
	Name       string    `json:"name"`
	ModifiedAt time.Time `json:"modified_at"`
	Size       int64     `json:"size"`
	Digest     string    `json:"digest"`
	Details    struct {
		Format            string `json:"format"`
		Family            string `json:"family"`
		Families          []string `json:"families"`
		ParameterSize     string `json:"parameter_size"`
		QuantizationLevel string `json:"quantization_level"`
	} `json:"details"`
}

// ollamaShowResponse is the response from Ollama's POST /api/show endpoint.
type ollamaShowResponse struct {
	License    string `json:"license"`
	Modelfile  string `json:"modelfile"`
	Parameters string `json:"parameters"`
	Template   string `json:"template"`
	Details    struct {
		Format            string   `json:"format"`
		Family            string   `json:"family"`
		Families          []string `json:"families"`
		ParameterSize     string   `json:"parameter_size"`
		QuantizationLevel string   `json:"quantization_level"`
	} `json:"details"`
}

// ollamaPullRequest is the request body for Ollama's POST /api/pull endpoint.
type ollamaPullRequest struct {
	Name   string `json:"name"`
	Stream bool   `json:"stream"`
}

// ollamaPullProgress is a single progress line from Ollama's pull stream.
type ollamaPullProgress struct {
	Status    string `json:"status"`
	Digest    string `json:"digest"`
	Total     int64  `json:"total"`
	Completed int64  `json:"completed"`
}

// ollamaDeleteRequest is the request body for Ollama's DELETE /api/delete endpoint.
type ollamaDeleteRequest struct {
	Name string `json:"name"`
}

// getInstalledModels fetches the list of locally installed models from Ollama.
func getInstalledModels(ollamaURL string) ([]InstalledModel, error) {
	resp, err := http.Get(ollamaURL + "/api/tags")
	if err != nil {
		return nil, fmt.Errorf("failed to reach Ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Ollama returned %d: %s", resp.StatusCode, string(body))
	}

	var tagsResp ollamaTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tagsResp); err != nil {
		return nil, fmt.Errorf("failed to parse Ollama response: %w", err)
	}

	models := make([]InstalledModel, 0, len(tagsResp.Models))
	for _, m := range tagsResp.Models {
		model := InstalledModel{
			Name:         m.Name,
			Size:         m.Size,
			SizeHuman:    formatSize(m.Size),
			Modified:     m.ModifiedAt.Format(time.RFC3339),
			Family:       m.Details.Family,
			Parameters:   m.Details.ParameterSize,
			Quantization: m.Details.QuantizationLevel,
		}

		// If details are missing from the tags response, try /api/show
		if model.Family == "" || model.Parameters == "" || model.Quantization == "" {
			info, err := getModelInfo(ollamaURL, m.Name)
			if err == nil {
				if model.Family == "" {
					model.Family = info.Details.Family
				}
				if model.Parameters == "" {
					model.Parameters = info.Details.ParameterSize
				}
				if model.Quantization == "" {
					model.Quantization = info.Details.QuantizationLevel
				}
			}
		}

		models = append(models, model)
	}

	return models, nil
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

	reqBody, err := json.Marshal(ollamaPullRequest{
		Name:   modelName,
		Stream: true,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal pull request: %w", err)
	}

	resp, err := http.Post(ollamaURL+"/api/pull", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("failed to reach Ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Ollama returned %d: %s", resp.StatusCode, string(body))
	}

	scanner := bufio.NewScanner(resp.Body)
	// Increase buffer size for potentially large lines
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		var progress ollamaPullProgress
		if err := json.Unmarshal(scanner.Bytes(), &progress); err != nil {
			continue
		}

		var percent float64
		if progress.Total > 0 {
			percent = float64(progress.Completed) / float64(progress.Total) * 100
		}

		progressChan <- PullProgress{
			Status:    progress.Status,
			Digest:    progress.Digest,
			Total:     progress.Total,
			Completed: progress.Completed,
			Percent:   percent,
		}
	}

	return scanner.Err()
}

// deleteModel deletes a locally installed model from Ollama.
func deleteModel(ollamaURL, modelName string) error {
	reqBody, err := json.Marshal(ollamaDeleteRequest{Name: modelName})
	if err != nil {
		return fmt.Errorf("failed to marshal delete request: %w", err)
	}

	req, err := http.NewRequest(http.MethodDelete, ollamaURL+"/api/delete", bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create delete request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to reach Ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Ollama returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// getModelInfo fetches detailed information about a specific model from Ollama.
func getModelInfo(ollamaURL, modelName string) (*ollamaShowResponse, error) {
	reqBody, err := json.Marshal(map[string]string{"name": modelName})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal show request: %w", err)
	}

	resp, err := http.Post(ollamaURL+"/api/show", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to reach Ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Ollama returned %d: %s", resp.StatusCode, string(body))
	}

	var showResp ollamaShowResponse
	if err := json.NewDecoder(resp.Body).Decode(&showResp); err != nil {
		return nil, fmt.Errorf("failed to parse Ollama response: %w", err)
	}

	return &showResp, nil
}

// --- HTTP Handlers ---

// handleModelsInstalled returns the list of locally installed models.
func (s *Server) handleModelsInstalled(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, "GET required", http.StatusMethodNotAllowed)
		return
	}

	if s.ollama == nil {
		s.jsonError(w, "Ollama is not configured", http.StatusServiceUnavailable)
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
