package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/axios-os/axios/internal/ollamactl"
	"github.com/axios-os/axios/pkg/logging"
	"github.com/axios-os/axios/pkg/mcp"
)

// client is the shared Ollama management client, set in main before serving.
var client *ollamactl.Client

func main() {
	defaultHost := "http://localhost:11434"
	if env := os.Getenv("OLLAMA_HOST"); env != "" {
		defaultHost = env
	}

	socketPath := flag.String("socket", mcp.SocketPath("axios-ollama"), "Unix socket path")
	host := flag.String("host", defaultHost, "Ollama base URL (default overridden by OLLAMA_HOST)")
	flag.Parse()

	logger := logging.New("axios-ollama")

	client = ollamactl.New(*host, nil)

	server := mcp.NewServer("axios-ollama", "0.1.0")

	// --- list_models ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "list_models",
		Description: "List locally installed Ollama models with size and family details",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Permission: "trusted",
	}, handleListModels)

	// --- model_info ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "model_info",
		Description: "Show detailed information about an installed Ollama model (license, parameters, template, details)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Model name, e.g. llama3.1:8b",
				},
			},
			"required": []string{"name"},
		},
		Permission: "trusted",
	}, handleModelInfo)

	// --- ollama_status ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "ollama_status",
		Description: "Check whether the Ollama server is reachable and report its version",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Permission: "trusted",
	}, handleOllamaStatus)

	// --- pull_model ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "pull_model",
		Description: "Pull a model from the Ollama registry (blocks until the download completes)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Model name, e.g. llama3.1:8b",
				},
			},
			"required": []string{"name"},
		},
		Permission: "approval_required",
	}, handlePullModel)

	// --- delete_model ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "delete_model",
		Description: "Delete a locally installed Ollama model",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Model name, e.g. llama3.1:8b",
				},
			},
			"required": []string{"name"},
		},
		Permission: "approval_required",
	}, handleDeleteModel)

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.Info("received signal, shutting down", "signal", sig.String())
		server.Close()
		os.Exit(0)
	}()

	logger.Info("starting axios-ollama MCP server", "socket", *socketPath, "host", *host)
	if err := server.Serve(*socketPath); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}

// --- Tool handlers ---
// Handler errors become IsError tool results in pkg/mcp, so an unreachable
// Ollama server surfaces as a clear error message instead of crashing the
// server.

// requireString extracts a required non-empty string parameter.
func requireString(params map[string]any, key string) (string, error) {
	v, ok := params[key].(string)
	if !ok || v == "" {
		return "", fmt.Errorf("missing required parameter: %s", key)
	}
	return v, nil
}

func handleListModels(params map[string]any) (string, error) {
	models, err := client.ListModels()
	if err != nil {
		return "", err
	}
	if len(models) == 0 {
		return "no models installed (use pull_model to download one)", nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d installed models:\n", len(models))
	for _, m := range models {
		fmt.Fprintf(&b, "- %s (size: %s, family: %s, parameters: %s, quantization: %s, modified: %s)\n",
			m.Name, m.SizeHuman, orDash(m.Family), orDash(m.Parameters), orDash(m.Quantization), m.Modified)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

func handleModelInfo(params map[string]any) (string, error) {
	name, err := requireString(params, "name")
	if err != nil {
		return "", err
	}

	info, err := client.ModelInfo(name)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "model %s:\n", name)
	fmt.Fprintf(&b, "- family: %s\n", orDash(info.Details.Family))
	if len(info.Details.Families) > 0 {
		fmt.Fprintf(&b, "- families: %s\n", strings.Join(info.Details.Families, ", "))
	}
	fmt.Fprintf(&b, "- format: %s\n", orDash(info.Details.Format))
	fmt.Fprintf(&b, "- parameter size: %s\n", orDash(info.Details.ParameterSize))
	fmt.Fprintf(&b, "- quantization: %s\n", orDash(info.Details.QuantizationLevel))
	if info.Parameters != "" {
		fmt.Fprintf(&b, "- parameters:\n%s\n", info.Parameters)
	}
	if info.Template != "" {
		fmt.Fprintf(&b, "- template:\n%s\n", info.Template)
	}
	if info.License != "" {
		fmt.Fprintf(&b, "- license: %s\n", firstLine(info.License))
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

func handleOllamaStatus(params map[string]any) (string, error) {
	if err := client.Ping(); err != nil {
		return "", fmt.Errorf("Ollama is not reachable at %s: %w", client.BaseURL(), err)
	}

	version, err := client.Version()
	if err != nil {
		return fmt.Sprintf("Ollama is reachable at %s (version unknown: %v)", client.BaseURL(), err), nil
	}
	return fmt.Sprintf("Ollama is reachable at %s (version %s)", client.BaseURL(), version), nil
}

func handlePullModel(params map[string]any) (string, error) {
	name, err := requireString(params, "name")
	if err != nil {
		return "", err
	}

	// Blocking single-shot pull; stream progress is tracked only to report a
	// completion summary.
	var last ollamactl.PullProgress
	if err := client.PullModel(name, func(p ollamactl.PullProgress) {
		last = p
	}); err != nil {
		return "", fmt.Errorf("pull %s failed: %w", name, err)
	}

	summary := fmt.Sprintf("model %s pulled successfully", name)
	if last.Status != "" {
		summary += fmt.Sprintf(" (last status: %s)", last.Status)
	}
	if last.Total > 0 {
		summary += fmt.Sprintf(", downloaded %s", ollamactl.FormatSize(last.Total))
	}
	return summary, nil
}

func handleDeleteModel(params map[string]any) (string, error) {
	name, err := requireString(params, "name")
	if err != nil {
		return "", err
	}

	if err := client.DeleteModel(name); err != nil {
		return "", fmt.Errorf("delete %s failed: %w", name, err)
	}
	return fmt.Sprintf("model %s deleted", name), nil
}

// orDash substitutes "-" for empty metadata fields in human-readable output.
func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// firstLine truncates multi-line text (e.g. license blobs) to its first line.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i]) + " …"
	}
	return s
}
