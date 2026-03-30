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

	// Resolve Anthropic token — priority: env vars > config file
	// Supports both standard API keys and OAuth tokens from `claude setup-token`
	token := ""
	switch {
	case os.Getenv("ANTHROPIC_OAUTH_TOKEN") != "":
		token = os.Getenv("ANTHROPIC_OAUTH_TOKEN")
		logger.Info("using OAuth token from ANTHROPIC_OAUTH_TOKEN env var")
	case os.Getenv("ANTHROPIC_API_KEY") != "":
		token = os.Getenv("ANTHROPIC_API_KEY")
		logger.Info("using API key from ANTHROPIC_API_KEY env var")
	case cfg.Anthropic.OAuthToken != "":
		token = cfg.Anthropic.OAuthToken
		logger.Info("using OAuth token from config")
	case cfg.Anthropic.APIKey != "":
		token = cfg.Anthropic.APIKey
		logger.Info("using API key from config")
	default:
		logger.Error("no Anthropic credentials configured. Set ANTHROPIC_API_KEY, ANTHROPIC_OAUTH_TOKEN, or run: claude setup-token")
		os.Exit(1)
	}

	authType := claused.DetectAuthType(token)
	logger.Info("auth type detected", "type", string(authType))

	anthropic := claused.NewAnthropicClient(token, cfg.Anthropic.Model)

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
	server := claused.NewServer(anthropic, router, mcpManager, logger)

	// Build tool definitions from connected MCP servers
	server.BuildTools()
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
