package axiosd

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// OllamaClient talks to an Ollama server for model MANAGEMENT only —
// listing, pulling, deleting, and inspecting models (see models.go and the
// /api/models* endpoints). Chat goes through the provider layer
// (pkg/providers, ollama profile).
type OllamaClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewOllamaClient creates a management client for an Ollama server.
func NewOllamaClient(host string, port int) *OllamaClient {
	return &OllamaClient{
		baseURL:    fmt.Sprintf("http://%s:%d", host, port),
		httpClient: &http.Client{},
	}
}

// BaseURL returns the Ollama server base URL.
func (c *OllamaClient) BaseURL() string {
	return c.baseURL
}

// ListModels returns the names of all locally available models.
func (c *OllamaClient) ListModels() ([]string, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/tags")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var names []string
	for _, m := range result.Models {
		names = append(names, m.Name)
	}
	return names, nil
}

// Ping checks if the Ollama server is reachable.
func (c *OllamaClient) Ping() error {
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
