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

	// Initialize host store with a callback that updates the server's Ollama client
	hostsFilePath := "/tmp/axios-hosts.json"
	hostStore := claused.NewHostStore(func(client *claused.OllamaClient) {
		server.SetOllama(client)
		router.SetLocalAvailable(true)
		logger.Info("Ollama host switched", "url", client.BaseURL())
	})

	// Add the local host from config
	localHost, err := hostStore.AddHost("Local", cfg.Ollama.Host, cfg.Ollama.Port)
	if err != nil {
		logger.Warn("failed to add local Ollama host", "error", err)
	} else {
		logger.Info("local Ollama host registered", "status", localHost.Status)
	}

	// Load saved hosts from file (adds any previously saved remote hosts)
	if err := hostStore.LoadFromFile(hostsFilePath); err != nil {
		logger.Warn("failed to load saved hosts", "error", err)
	}

	// Set the local host as active if Ollama is enabled and reachable
	if ollamaEnabled && localHost != nil && localHost.Status == "online" {
		if err := hostStore.SetActive("local"); err != nil {
			logger.Warn("failed to set local host as active", "error", err)
		}
	}

	// Save hosts on shutdown
	defer func() {
		if err := hostStore.SaveToFile(hostsFilePath); err != nil {
			logger.Error("failed to save hosts", "error", err)
		} else {
			logger.Info("hosts saved", "path", hostsFilePath)
		}
	}()

	server.SetHostStore(hostStore)

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
