package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

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

	// Initialize provider store
	// Store data files in a persistent location (not /tmp which gets cleared)
	dataDir := os.Getenv("AXIOS_DATA_DIR")
	if dataDir == "" {
		homeDir, _ := os.UserHomeDir()
		dataDir = homeDir + "/.axios"
	}
	os.MkdirAll(dataDir, 0755)

	providersFilePath := dataDir + "/providers.json"
	providerStore := claused.NewProviderStore(providersFilePath)
	if err := providerStore.LoadFromFile(); err != nil {
		logger.Warn("failed to load saved providers", "error", err)
	}

	// If Anthropic credentials exist from env/config, auto-set them on the "anthropic" provider
	if hasCloudAuth {
		if err := providerStore.SetAPIKey("anthropic", token); err != nil {
			logger.Warn("failed to set anthropic key on provider store", "error", err)
		}
		// Set anthropic as active if no other provider is already active
		if providerStore.GetActive() == nil {
			if err := providerStore.SetActive("anthropic", cfg.Anthropic.Model); err != nil {
				logger.Warn("failed to set anthropic as active provider", "error", err)
			}
		}
	}

	// Start HTTP/WebSocket server
	server := claused.NewServer(anthropic, router, mcpManager, logger)
	server.SetProviderStore(providerStore)
	if ollamaClient != nil {
		server.SetOllama(ollamaClient)
	}

	// Initialize host store with a callback that updates the server's Ollama client
	hostsFilePath := dataDir + "/hosts.json"
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

	// Re-activate the previously active host, or fall back to any online host
	hostStore.CheckAllHealth()
	activated := false

	// Try to restore the saved active host
	for _, h := range hostStore.GetHosts() {
		if h.Active && h.Status == "online" {
			if err := hostStore.SetActive(h.ID); err == nil {
				logger.Info("restored active Ollama host", "id", h.ID, "host", h.Host)
				activated = true
			}
			break
		}
	}

	// If saved active host is offline, try any online host
	if !activated {
		for _, h := range hostStore.GetHosts() {
			if h.Status == "online" {
				if err := hostStore.SetActive(h.ID); err == nil {
					logger.Info("activated first online Ollama host", "id", h.ID, "host", h.Host)
					activated = true
				}
				break
			}
		}
	}

	if !activated {
		logger.Warn("no Ollama hosts are online")
	}

	// Save state to disk
	saveAll := func() {
		if err := hostStore.SaveToFile(hostsFilePath); err != nil {
			logger.Error("failed to save hosts", "error", err)
		} else {
			logger.Info("hosts saved", "path", hostsFilePath)
		}
		if err := providerStore.SaveToFile(); err != nil {
			logger.Error("failed to save providers", "error", err)
		} else {
			logger.Info("providers saved", "path", providersFilePath)
		}
	}

	// Handle shutdown signals — save state before exit
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("shutdown signal received, saving state...")
		saveAll()
		os.Exit(0)
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
		saveAll()
		os.Exit(1)
	}
}
