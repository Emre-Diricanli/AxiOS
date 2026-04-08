// Package config handles loading and managing AxiOS configuration.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Claused holds the claused daemon configuration.
type Claused struct {
	Server    ServerConfig    `yaml:"server"`
	Anthropic AnthropicConfig `yaml:"anthropic"`
	Ollama    OllamaConfig    `yaml:"ollama"`
	Routing   RoutingConfig   `yaml:"routing"`
	MCP       MCPConfig       `yaml:"mcp"`
	Gateway   GatewayConfig   `yaml:"gateway"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type AnthropicConfig struct {
	APIKey     string `yaml:"api_key"`      // Standard API key (sk-ant-api03-...)
	OAuthToken string `yaml:"oauth_token"`  // OAuth token from `claude setup-token` (sk-ant-oat01-...)
	Model      string `yaml:"model"`
}

type OllamaConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Model    string `yaml:"model"`
}

type RoutingConfig struct {
	Mode string `yaml:"mode"` // "auto", "cloud_only", "local_only", "cost_aware"
}

type MCPConfig struct {
	SocketDir string   `yaml:"socket_dir"`
	Servers   []string `yaml:"servers"`
}

type GatewayConfig struct {
	Enabled bool   `yaml:"enabled"` // Enable CasaOS-Gateway registration
	URL     string `yaml:"url"`     // Gateway URL (auto-discovered if empty)
}

// LoadClaused reads and parses the claused daemon configuration.
func LoadClaused(path string) (*Claused, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read claused config: %w", err)
	}

	var cfg Claused
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse claused config: %w", err)
	}

	// Defaults
	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 443
	}
	if cfg.Anthropic.Model == "" {
		cfg.Anthropic.Model = "claude-sonnet-4-6"
	}
	if cfg.Ollama.Host == "" {
		cfg.Ollama.Host = "localhost"
	}
	if cfg.Ollama.Port == 0 {
		cfg.Ollama.Port = 11434
	}
	if cfg.Routing.Mode == "" {
		cfg.Routing.Mode = "auto"
	}
	if cfg.MCP.SocketDir == "" {
		cfg.MCP.SocketDir = "/run/axios/mcp"
	}

	return &cfg, nil
}
