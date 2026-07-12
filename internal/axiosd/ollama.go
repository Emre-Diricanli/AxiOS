package axiosd

import (
	"fmt"

	"github.com/axios-os/axios/internal/ollamactl"
)

// OllamaClient talks to an Ollama server for model MANAGEMENT only —
// listing, pulling, deleting, and inspecting models (see models.go and the
// /api/models* endpoints). Chat goes through the provider layer
// (pkg/providers, ollama profile). It wraps the shared internal/ollamactl
// client, which is also used by the axios-ollama MCP server.
type OllamaClient struct {
	inner *ollamactl.Client
}

// NewOllamaClient creates a management client for an Ollama server.
func NewOllamaClient(host string, port int) *OllamaClient {
	return &OllamaClient{
		inner: ollamactl.New(fmt.Sprintf("http://%s:%d", host, port), nil),
	}
}

// BaseURL returns the Ollama server base URL.
func (c *OllamaClient) BaseURL() string {
	return c.inner.BaseURL()
}

// ListModels returns the names of all locally available models.
func (c *OllamaClient) ListModels() ([]string, error) {
	return c.inner.ListModelNames()
}

// Ping checks if the Ollama server is reachable.
func (c *OllamaClient) Ping() error {
	return c.inner.Ping()
}
