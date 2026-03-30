package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/axios-os/axios/internal/claused"
	"github.com/axios-os/axios/internal/config"
	"github.com/axios-os/axios/pkg/logging"
)

func main() {
	configPath := flag.String("config", "/etc/axios/claused.yaml", "path to claused config")
	flag.Parse()

	logger := logging.New("claused")

	cfg, err := config.LoadClaused(*configPath)
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Initialize Anthropic client
	apiKey := cfg.Anthropic.APIKey
	if envKey := os.Getenv("ANTHROPIC_API_KEY"); envKey != "" {
		apiKey = envKey
	}
	if apiKey == "" {
		logger.Error("no Anthropic API key configured")
		os.Exit(1)
	}

	anthropic := claused.NewAnthropicClient(apiKey, cfg.Anthropic.Model)

	// Initialize router
	router := claused.NewRouter(claused.RoutingMode(cfg.Routing.Mode), logger)
	router.SetCloudAvailable(true)
	router.SetLocalAvailable(cfg.Ollama.Enabled)

	// Initialize MCP manager
	mcpManager := claused.NewMCPManager(logger)
	defer mcpManager.Close()

	// Connect to configured MCP servers
	for _, serverName := range cfg.MCP.Servers {
		if err := mcpManager.Connect(serverName); err != nil {
			logger.Warn("MCP server not available", "server", serverName, "error", err)
		}
	}

	// Start HTTP/WebSocket server
	server := claused.NewServer(anthropic, router, logger)
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)

	logger.Info("AxiOS claused starting",
		"addr", addr,
		"model", cfg.Anthropic.Model,
		"routing", cfg.Routing.Mode,
		"ollama", cfg.Ollama.Enabled,
	)

	if err := server.ListenAndServe(addr); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}
