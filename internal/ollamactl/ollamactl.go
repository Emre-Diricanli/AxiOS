// Package ollamactl wraps the Ollama HTTP API for model MANAGEMENT —
// listing, inspecting, pulling, and deleting models. It is shared by the
// axiosd REST handlers (internal/axiosd/models.go) and the axios-ollama MCP
// server so Ollama logic lives in one place. Chat goes through the provider
// layer (pkg/providers, ollama profile).
package ollamactl

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client talks to an Ollama server's management API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// New creates a management client for the Ollama server at baseURL
// (e.g. "http://localhost:11434"). A nil httpClient falls back to a default
// client; tests inject httptest clients/URLs.
func New(baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
	}
}

// BaseURL returns the Ollama server base URL.
func (c *Client) BaseURL() string {
	return c.baseURL
}

// --- Data types ---

// Model represents a locally installed Ollama model.
type Model struct {
	Name         string `json:"name"`
	Size         int64  `json:"size"`
	SizeHuman    string `json:"size_human"`
	Modified     string `json:"modified"`
	Family       string `json:"family"`
	Parameters   string `json:"parameters"`
	Quantization string `json:"quantization"`
}

// ModelDetails describes a model's format/family/size metadata as returned
// by Ollama in both /api/tags and /api/show responses.
type ModelDetails struct {
	Format            string   `json:"format"`
	Family            string   `json:"family"`
	Families          []string `json:"families"`
	ParameterSize     string   `json:"parameter_size"`
	QuantizationLevel string   `json:"quantization_level"`
}

// ShowResponse is the response from Ollama's POST /api/show endpoint.
type ShowResponse struct {
	License    string       `json:"license"`
	Modelfile  string       `json:"modelfile"`
	Parameters string       `json:"parameters"`
	Template   string       `json:"template"`
	Details    ModelDetails `json:"details"`
}

// PullProgress represents progress of a model pull operation.
type PullProgress struct {
	Status    string  `json:"status"`
	Digest    string  `json:"digest,omitempty"`
	Total     int64   `json:"total,omitempty"`
	Completed int64   `json:"completed,omitempty"`
	Percent   float64 `json:"percent"`
}

// FormatSize converts bytes to a human-readable string.
func FormatSize(bytes int64) string {
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

// --- Wire types ---

// tagsResponse is the response from Ollama's GET /api/tags endpoint.
type tagsResponse struct {
	Models []tagsModelEntry `json:"models"`
}

type tagsModelEntry struct {
	Name       string       `json:"name"`
	ModifiedAt time.Time    `json:"modified_at"`
	Size       int64        `json:"size"`
	Digest     string       `json:"digest"`
	Details    ModelDetails `json:"details"`
}

// pullRequest is the request body for Ollama's POST /api/pull endpoint.
type pullRequest struct {
	Name   string `json:"name"`
	Stream bool   `json:"stream"`
}

// pullProgressLine is a single progress line from Ollama's pull stream.
type pullProgressLine struct {
	Status    string `json:"status"`
	Digest    string `json:"digest"`
	Total     int64  `json:"total"`
	Completed int64  `json:"completed"`
}

// deleteRequest is the request body for Ollama's DELETE /api/delete endpoint.
type deleteRequest struct {
	Name string `json:"name"`
}

// --- API methods ---

// Ping checks if the Ollama server is reachable.
func (c *Client) Ping() error {
	resp, err := c.httpClient.Get(c.baseURL + "/api/tags")
	if err != nil {
		return fmt.Errorf("ollama unreachable: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama returned %d", resp.StatusCode)
	}
	return nil
}

// Version returns the Ollama server version from GET /api/version.
func (c *Client) Version() (string, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/version")
	if err != nil {
		return "", fmt.Errorf("failed to reach Ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Ollama returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to parse Ollama response: %w", err)
	}
	return result.Version, nil
}

// ListModelNames returns the names of all locally available models.
func (c *Client) ListModelNames() ([]string, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/tags")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result tagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var names []string
	for _, m := range result.Models {
		names = append(names, m.Name)
	}
	return names, nil
}

// ListModels returns the locally installed models with size and family
// details. When /api/tags omits details, /api/show is queried as a fallback.
func (c *Client) ListModels() ([]Model, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/tags")
	if err != nil {
		return nil, fmt.Errorf("failed to reach Ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Ollama returned %d: %s", resp.StatusCode, string(body))
	}

	var tagsResp tagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tagsResp); err != nil {
		return nil, fmt.Errorf("failed to parse Ollama response: %w", err)
	}

	models := make([]Model, 0, len(tagsResp.Models))
	for _, m := range tagsResp.Models {
		model := Model{
			Name:         m.Name,
			Size:         m.Size,
			SizeHuman:    FormatSize(m.Size),
			Modified:     m.ModifiedAt.Format(time.RFC3339),
			Family:       m.Details.Family,
			Parameters:   m.Details.ParameterSize,
			Quantization: m.Details.QuantizationLevel,
		}

		// If details are missing from the tags response, try /api/show
		if model.Family == "" || model.Parameters == "" || model.Quantization == "" {
			info, err := c.ModelInfo(m.Name)
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

// ModelInfo fetches detailed information about a specific model via
// POST /api/show.
func (c *Client) ModelInfo(name string) (*ShowResponse, error) {
	reqBody, err := json.Marshal(map[string]string{"name": name})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal show request: %w", err)
	}

	resp, err := c.httpClient.Post(c.baseURL+"/api/show", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to reach Ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Ollama returned %d: %s", resp.StatusCode, string(body))
	}

	var showResp ShowResponse
	if err := json.NewDecoder(resp.Body).Decode(&showResp); err != nil {
		return nil, fmt.Errorf("failed to parse Ollama response: %w", err)
	}

	return &showResp, nil
}

// PullModel pulls a model from the Ollama registry, blocking until the pull
// completes. Each streamed progress line is passed to onProgress when it is
// non-nil; pass nil to discard progress.
func (c *Client) PullModel(name string, onProgress func(PullProgress)) error {
	reqBody, err := json.Marshal(pullRequest{
		Name:   name,
		Stream: true,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal pull request: %w", err)
	}

	resp, err := c.httpClient.Post(c.baseURL+"/api/pull", "application/json", bytes.NewReader(reqBody))
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
		var line pullProgressLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}

		var percent float64
		if line.Total > 0 {
			percent = float64(line.Completed) / float64(line.Total) * 100
		}

		if onProgress != nil {
			onProgress(PullProgress{
				Status:    line.Status,
				Digest:    line.Digest,
				Total:     line.Total,
				Completed: line.Completed,
				Percent:   percent,
			})
		}
	}

	return scanner.Err()
}

// DeleteModel deletes a locally installed model via DELETE /api/delete.
func (c *Client) DeleteModel(name string) error {
	reqBody, err := json.Marshal(deleteRequest{Name: name})
	if err != nil {
		return fmt.Errorf("failed to marshal delete request: %w", err)
	}

	req, err := http.NewRequest(http.MethodDelete, c.baseURL+"/api/delete", bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create delete request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
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
