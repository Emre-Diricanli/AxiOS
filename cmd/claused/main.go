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
	token := ""
	hasCloudAuth := true
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
		hasCloudAuth = false
		logger.Warn("no Anthropic credentials configured — cloud backend unavailable")
	}

	var anthropic *claused.AnthropicClient
	if hasCloudAuth {
		authType := claused.DetectAuthType(token)
		logger.Info("auth type detected", "type", string(authType))
		anthropic = claused.NewAnthropicClient(token, cfg.Anthropic.Model)
	}

	// Initialize Ollama client
	var ollamaClient *claused.OllamaClient
	ollamaEnabled := cfg.Ollama.Enabled
	if ollamaEnabled {
		ollamaClient = claused.NewOllamaClient(cfg.Ollama.Host, cfg.Ollama.Port, cfg.Ollama.Model)
		if err := ollamaClient.Ping(); err != nil {
			logger.Warn("Ollama not reachable, disabling", "error", err)
			ollamaEnabled = false
			ollamaClient = nil
		} else {
			logger.Info("Ollama connected", "model", cfg.Ollama.Model, "host", cfg.Ollama.Host)
		}
	}

	// Must have at least one backend
	if !hasCloudAuth && !ollamaEnabled {
		logger.Error("no AI backend available. Set ANTHROPIC_API_KEY or enable Ollama in config.")
		os.Exit(1)
	}

	// Initialize router
	router := claused.NewRouter(claused.RoutingMode(cfg.Routing.Mode), logger)
	router.SetCloudAvailable(hasCloudAuth)
	router.SetLocalAvailable(ollamaEnabled)

	// Initialize MCP manager
	mcpManager := claused.NewMCPManager(cfg.MCP.SocketDir, logger)
	defer mcpManager.Close()

	for _, serverName := range cfg.MCP.Servers {
		if err := mcpManager.Connect(serverName); err != nil {
			logger.Warn("MCP server not available", "server", serverName, "error", err)
		}
	}

	// Start HTTP/WebSocket server
	server := claused.NewServer(anthropic, router, mcpManager, logger)
	if ollamaClient != nil {
		server.SetOllama(ollamaClient)
	}

	server.BuildTools()
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)

	logger.Info("AxiOS claused starting",
		"addr", addr,
		"cloud", hasCloudAuth,
		"ollama", ollamaEnabled,
		"routing", cfg.Routing.Mode,
	)

	if err := server.ListenAndServe(addr); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}
