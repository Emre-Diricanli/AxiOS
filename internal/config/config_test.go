package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "axiosd.yaml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func hasWarningContaining(warnings []string, substr string) bool {
	for _, w := range warnings {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}

func TestLoadDaemonDefaults(t *testing.T) {
	cfg, err := LoadDaemon(writeConfig(t, "routing:\n  mode: auto\n"))
	if err != nil {
		t.Fatalf("LoadDaemon: %v", err)
	}

	if cfg.Server.Listen != "127.0.0.1:3000" {
		t.Errorf("Server.Listen = %q, want loopback default", cfg.Server.Listen)
	}
	if cfg.Model.Provider != "auto" {
		t.Errorf("Model.Provider = %q, want auto", cfg.Model.Provider)
	}
	if cfg.Opencode.Binary != "opencode" || cfg.Opencode.Port != 4097 || cfg.Opencode.Workspace != "~/axios-workspace" {
		t.Errorf("Opencode defaults wrong: %+v", cfg.Opencode)
	}
	if cfg.Ollama.Host != "localhost" || cfg.Ollama.Port != 11434 {
		t.Errorf("Ollama defaults wrong: %+v", cfg.Ollama)
	}
	if cfg.Routing.Mode != "auto" {
		t.Errorf("Routing.Mode = %q", cfg.Routing.Mode)
	}
	if cfg.MCP.SocketDir != "/run/axios/mcp" {
		t.Errorf("MCP.SocketDir = %q", cfg.MCP.SocketDir)
	}
	if cfg.Permissions.Path != "/etc/axios/permissions.yaml" {
		t.Errorf("Permissions.Path = %q, want /etc/axios/permissions.yaml", cfg.Permissions.Path)
	}
	if cfg.Permissions.ApprovalTimeoutSeconds != 120 {
		t.Errorf("Permissions.ApprovalTimeoutSeconds = %d, want 120", cfg.Permissions.ApprovalTimeoutSeconds)
	}
	if len(cfg.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", cfg.Warnings)
	}
}

func TestLoadDaemonPermissionsOverrides(t *testing.T) {
	cfg, err := LoadDaemon(writeConfig(t, `
permissions:
  path: /opt/axios/perms.yaml
  approval_timeout_seconds: 30
`))
	if err != nil {
		t.Fatalf("LoadDaemon: %v", err)
	}
	if cfg.Permissions.Path != "/opt/axios/perms.yaml" {
		t.Errorf("Permissions.Path = %q", cfg.Permissions.Path)
	}
	if cfg.Permissions.ApprovalTimeoutSeconds != 30 {
		t.Errorf("Permissions.ApprovalTimeoutSeconds = %d", cfg.Permissions.ApprovalTimeoutSeconds)
	}
}

func TestLoadDaemonNewSchema(t *testing.T) {
	cfg, err := LoadDaemon(writeConfig(t, `
server:
  listen: "0.0.0.0:8443"
model:
  provider: openrouter
  id: anthropic/claude-sonnet-4
fallback_providers:
  - provider: groq
    model: llama-3.1-70b-versatile
  - provider: ollama
    model: llama3.1:8b
opencode:
  enabled: true
  port: 5000
`))
	if err != nil {
		t.Fatalf("LoadDaemon: %v", err)
	}

	if cfg.Server.Listen != "0.0.0.0:8443" {
		t.Errorf("Server.Listen = %q", cfg.Server.Listen)
	}
	if cfg.Model.Provider != "openrouter" || cfg.Model.ID != "anthropic/claude-sonnet-4" {
		t.Errorf("Model = %+v", cfg.Model)
	}
	if len(cfg.FallbackProviders) != 2 {
		t.Fatalf("FallbackProviders = %+v", cfg.FallbackProviders)
	}
	if cfg.FallbackProviders[0].Provider != "groq" || cfg.FallbackProviders[0].Model != "llama-3.1-70b-versatile" {
		t.Errorf("FallbackProviders[0] = %+v", cfg.FallbackProviders[0])
	}
	if cfg.FallbackProviders[1].Provider != "ollama" || cfg.FallbackProviders[1].Model != "llama3.1:8b" {
		t.Errorf("FallbackProviders[1] = %+v", cfg.FallbackProviders[1])
	}
	if !cfg.Opencode.Enabled || cfg.Opencode.Port != 5000 {
		t.Errorf("Opencode = %+v", cfg.Opencode)
	}
}

func TestLoadDaemonLegacyAnthropicMapping(t *testing.T) {
	cfg, err := LoadDaemon(writeConfig(t, `
anthropic:
  api_key: sk-ant-api03-legacy
  model: claude-sonnet-4-6
`))
	if err != nil {
		t.Fatalf("LoadDaemon: %v", err)
	}

	if cfg.Model.Provider != "anthropic" {
		t.Errorf("Model.Provider = %q, want anthropic (legacy mapping)", cfg.Model.Provider)
	}
	if cfg.Model.ID != "claude-sonnet-4-6" {
		t.Errorf("Model.ID = %q, want legacy model", cfg.Model.ID)
	}
	if cfg.LegacyAnthropicCredential() != "sk-ant-api03-legacy" {
		t.Errorf("LegacyAnthropicCredential = %q", cfg.LegacyAnthropicCredential())
	}
	if !hasWarningContaining(cfg.Warnings, "anthropic: section is deprecated") {
		t.Errorf("missing deprecation warning, got %v", cfg.Warnings)
	}
}

func TestLoadDaemonLegacyAnthropicOAuthPreferred(t *testing.T) {
	cfg, err := LoadDaemon(writeConfig(t, `
anthropic:
  api_key: sk-ant-api03-x
  oauth_token: sk-ant-oat01-y
`))
	if err != nil {
		t.Fatalf("LoadDaemon: %v", err)
	}
	if cfg.LegacyAnthropicCredential() != "sk-ant-oat01-y" {
		t.Errorf("LegacyAnthropicCredential = %q, want oauth token", cfg.LegacyAnthropicCredential())
	}
}

func TestLoadDaemonLegacyAnthropicDoesNotOverrideExplicitProvider(t *testing.T) {
	cfg, err := LoadDaemon(writeConfig(t, `
model:
  provider: openai
anthropic:
  api_key: sk-ant-api03-legacy
  model: claude-sonnet-4-6
`))
	if err != nil {
		t.Fatalf("LoadDaemon: %v", err)
	}
	if cfg.Model.Provider != "openai" {
		t.Errorf("Model.Provider = %q, explicit provider must win", cfg.Model.Provider)
	}
	if cfg.Model.ID != "" {
		t.Errorf("Model.ID = %q, legacy model must not leak onto other providers", cfg.Model.ID)
	}
}

func TestLoadDaemonLegacyServerHostPort(t *testing.T) {
	cfg, err := LoadDaemon(writeConfig(t, `
server:
  host: "0.0.0.0"
  port: 3000
`))
	if err != nil {
		t.Fatalf("LoadDaemon: %v", err)
	}
	if cfg.Server.Listen != "0.0.0.0:3000" {
		t.Errorf("Server.Listen = %q, want mapped legacy host:port", cfg.Server.Listen)
	}
	if !hasWarningContaining(cfg.Warnings, "server.host/server.port are deprecated") {
		t.Errorf("missing deprecation warning, got %v", cfg.Warnings)
	}
}

func TestLoadDaemonListenWinsOverLegacyHostPort(t *testing.T) {
	cfg, err := LoadDaemon(writeConfig(t, `
server:
  listen: "127.0.0.1:9000"
  host: "0.0.0.0"
  port: 3000
`))
	if err != nil {
		t.Fatalf("LoadDaemon: %v", err)
	}
	if cfg.Server.Listen != "127.0.0.1:9000" {
		t.Errorf("Server.Listen = %q, explicit listen must win", cfg.Server.Listen)
	}
}

func TestLoadDaemonAuthDefaults(t *testing.T) {
	cfg, err := LoadDaemon(writeConfig(t, "routing:\n  mode: auto\n"))
	if err != nil {
		t.Fatalf("LoadDaemon: %v", err)
	}
	if !cfg.Server.AuthEnabled() {
		t.Error("AuthEnabled() = false with server.auth omitted, want enabled by default")
	}
	if cfg.Server.Auth.SessionTTLHours != 168 {
		t.Errorf("Auth.SessionTTLHours = %d, want 168", cfg.Server.Auth.SessionTTLHours)
	}
	if len(cfg.Server.Auth.AllowedOrigins) != 0 {
		t.Errorf("Auth.AllowedOrigins = %v, want empty", cfg.Server.Auth.AllowedOrigins)
	}
}

func TestLoadDaemonAuthOverrides(t *testing.T) {
	cfg, err := LoadDaemon(writeConfig(t, `
server:
  auth:
    enabled: false
    session_ttl_hours: 24
    allowed_origins:
      - https://axios.example.ts.net
`))
	if err != nil {
		t.Fatalf("LoadDaemon: %v", err)
	}
	if cfg.Server.AuthEnabled() {
		t.Error("AuthEnabled() = true, explicit enabled: false must win")
	}
	if cfg.Server.Auth.SessionTTLHours != 24 {
		t.Errorf("Auth.SessionTTLHours = %d, want 24", cfg.Server.Auth.SessionTTLHours)
	}
	if len(cfg.Server.Auth.AllowedOrigins) != 1 || cfg.Server.Auth.AllowedOrigins[0] != "https://axios.example.ts.net" {
		t.Errorf("Auth.AllowedOrigins = %v", cfg.Server.Auth.AllowedOrigins)
	}
}

func TestLoadDaemonAuthExplicitEnabled(t *testing.T) {
	cfg, err := LoadDaemon(writeConfig(t, `
server:
  auth:
    enabled: true
`))
	if err != nil {
		t.Fatalf("LoadDaemon: %v", err)
	}
	if !cfg.Server.AuthEnabled() {
		t.Error("AuthEnabled() = false with enabled: true")
	}
}
