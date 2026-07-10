// Package config handles loading and managing AxiOS configuration.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Daemon holds the axiosd daemon configuration.
type Daemon struct {
	Server            ServerConfig       `yaml:"server"`
	Model             ModelConfig        `yaml:"model"`
	FallbackProviders []FallbackProvider `yaml:"fallback_providers"`
	Opencode          OpencodeConfig     `yaml:"opencode"`
	Ollama            OllamaConfig       `yaml:"ollama"`
	Routing           RoutingConfig      `yaml:"routing"`
	MCP               MCPConfig          `yaml:"mcp"`
	Gateway           GatewayConfig      `yaml:"gateway"`
	Permissions       PermissionsConfig  `yaml:"permissions"`

	// Anthropic is the deprecated legacy credential section. It is still
	// parsed and mapped onto the model-agnostic schema; new configs should
	// use `model:` plus provider env vars or the provider store instead.
	Anthropic AnthropicConfig `yaml:"anthropic"`

	// Warnings collects deprecation notices produced during load, so the
	// caller can log them through its own logger.
	Warnings []string `yaml:"-"`
}

// ServerConfig controls the HTTP/WebSocket listener.
type ServerConfig struct {
	// Listen is the bind address, e.g. "127.0.0.1:3000". The default is
	// loopback-only; exposing the daemon on the LAN requires explicit opt-in.
	Listen string `yaml:"listen"`

	// Host and Port are the deprecated pre-Phase-1 fields; when set (and
	// Listen is empty) they are joined into Listen with a warning.
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

// ModelConfig selects the active provider and model.
type ModelConfig struct {
	Provider string `yaml:"provider"` // auto | anthropic | openai | openrouter | ollama | custom...
	ID       string `yaml:"id"`       // empty = provider default
}

// FallbackProvider is one entry in the provider fallback chain.
type FallbackProvider struct {
	Provider string `yaml:"provider"`
	Model    string `yaml:"model"`
}

// OpencodeConfig controls the managed opencode background coding agent.
type OpencodeConfig struct {
	Enabled   bool   `yaml:"enabled"`
	Binary    string `yaml:"binary"`
	Port      int    `yaml:"port"`
	Workspace string `yaml:"workspace"`
	// Model is the default "provider/model" for delegated coding tasks,
	// e.g. "xai/grok-build-0.1" to run them on a SuperGrok subscription
	// connected via /api/providers/xai/oauth. Empty = opencode's default.
	Model string `yaml:"model"`
}

// AnthropicConfig is the deprecated legacy credential section.
type AnthropicConfig struct {
	APIKey     string `yaml:"api_key"`     // Standard API key (sk-ant-api03-...)
	OAuthToken string `yaml:"oauth_token"` // OAuth token from `claude setup-token` (sk-ant-oat01-...)
	Model      string `yaml:"model"`
}

type OllamaConfig struct {
	Enabled bool   `yaml:"enabled"`
	Host    string `yaml:"host"`
	Port    int    `yaml:"port"`
	Model   string `yaml:"model"`
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

// PermissionsConfig controls tiered-trust enforcement on model-initiated
// tool calls.
type PermissionsConfig struct {
	// Path is the permissions.yaml location. When the file is missing the
	// daemon falls back to the repo's configs/permissions.yaml (dev) and
	// finally to the built-in default policy.
	Path string `yaml:"path"`
	// ApprovalTimeoutSeconds bounds how long an approval_required tool call
	// waits for the user's approval_response before being denied.
	ApprovalTimeoutSeconds int `yaml:"approval_timeout_seconds"`
}

// LegacyAnthropicCredential returns the credential from the deprecated
// anthropic: section, preferring the OAuth token, or "" when unset.
func (d *Daemon) LegacyAnthropicCredential() string {
	if d.Anthropic.OAuthToken != "" {
		return d.Anthropic.OAuthToken
	}
	return d.Anthropic.APIKey
}

// LoadDaemon reads and parses the axiosd daemon configuration, applies
// defaults, and maps deprecated legacy fields onto the current schema.
// Deprecation notices are collected in Daemon.Warnings.
func LoadDaemon(path string) (*Daemon, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read axiosd config: %w", err)
	}

	var cfg Daemon
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse axiosd config: %w", err)
	}

	applyDefaults(&cfg)
	return &cfg, nil
}

// applyDefaults fills zero values and maps legacy fields onto the new schema.
func applyDefaults(cfg *Daemon) {
	// Legacy server.host/server.port → server.listen.
	if cfg.Server.Listen == "" && (cfg.Server.Host != "" || cfg.Server.Port != 0) {
		host := cfg.Server.Host
		if host == "" {
			host = "127.0.0.1"
		}
		port := cfg.Server.Port
		if port == 0 {
			port = 3000
		}
		cfg.Server.Listen = fmt.Sprintf("%s:%d", host, port)
		cfg.Warnings = append(cfg.Warnings,
			"config: server.host/server.port are deprecated — use server.listen instead")
	}
	if cfg.Server.Listen == "" {
		cfg.Server.Listen = "127.0.0.1:3000"
	}

	// Legacy anthropic: section → model.provider/model.id.
	if cfg.Anthropic.APIKey != "" || cfg.Anthropic.OAuthToken != "" || cfg.Anthropic.Model != "" {
		cfg.Warnings = append(cfg.Warnings,
			"config: the anthropic: section is deprecated — use model.provider/model.id and provider env vars or the provider store")
		if (cfg.Model.Provider == "" || cfg.Model.Provider == "auto") && cfg.LegacyAnthropicCredential() != "" {
			cfg.Model.Provider = "anthropic"
		}
		if cfg.Model.Provider == "anthropic" && cfg.Model.ID == "" && cfg.Anthropic.Model != "" {
			cfg.Model.ID = cfg.Anthropic.Model
		}
	}

	if cfg.Model.Provider == "" {
		cfg.Model.Provider = "auto"
	}

	if cfg.Opencode.Binary == "" {
		cfg.Opencode.Binary = "opencode"
	}
	if cfg.Opencode.Port == 0 {
		cfg.Opencode.Port = 4097
	}
	if cfg.Opencode.Workspace == "" {
		cfg.Opencode.Workspace = "~/axios-workspace"
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
	if cfg.Permissions.Path == "" {
		cfg.Permissions.Path = "/etc/axios/permissions.yaml"
	}
	if cfg.Permissions.ApprovalTimeoutSeconds <= 0 {
		cfg.Permissions.ApprovalTimeoutSeconds = 120
	}
}
