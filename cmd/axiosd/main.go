package main

import (
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/axios-os/axios/internal/axiosd"
	"github.com/axios-os/axios/internal/config"
	"github.com/axios-os/axios/pkg/logging"
	"github.com/axios-os/axios/pkg/permissions"
	"github.com/axios-os/axios/pkg/providers"
	"github.com/axios-os/axios/pkg/secrets"
)

const (
	defaultConfigPath = "/etc/axios/axiosd.yaml"
	legacyConfigPath  = "/etc/axios/claused.yaml"

	// devPermissionsPath is the repo-relative fallback so `go run ./cmd/axiosd`
	// from a checkout picks up the shipped policy without an install step.
	devPermissionsPath = "configs/permissions.yaml"
)

func main() {
	configPath := flag.String("config", defaultConfigPath, "path to axiosd config")
	flag.Parse()

	logger := logging.New("axiosd")

	// If the default config path is absent, fall back to the deprecated
	// legacy claused.yaml path so existing installs keep working.
	path := *configPath
	if path == defaultConfigPath {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if _, lerr := os.Stat(legacyConfigPath); lerr == nil {
				logger.Warn("config not found at default path; using deprecated legacy config — rename it to axiosd.yaml",
					"default_path", defaultConfigPath,
					"legacy_path", legacyConfigPath,
				)
				path = legacyConfigPath
			}
		}
	}

	cfg, err := config.LoadDaemon(path)
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}
	for _, warning := range cfg.Warnings {
		logger.Warn(warning)
	}

	// Resolve the persistent data directory (not /tmp, which gets cleared).
	dataDir := os.Getenv("AXIOS_DATA_DIR")
	if dataDir == "" {
		homeDir, _ := os.UserHomeDir()
		dataDir = filepath.Join(homeDir, ".axios")
	}
	os.MkdirAll(dataDir, 0755)

	// Master key for credential encryption at rest (AES-256-GCM).
	secretsStore, err := secrets.NewStore(filepath.Join(dataDir, "master.key"))
	if err != nil {
		logger.Error("failed to initialize secrets store — API keys will NOT be encrypted at rest", "error", err)
		secretsStore = nil
	}

	// Provider store: saved keys (encrypted) + active provider/model.
	providersFilePath := filepath.Join(dataDir, "providers.json")
	providerStore := axiosd.NewProviderStore(providersFilePath, secretsStore)
	if err := providerStore.LoadFromFile(); err != nil {
		logger.Warn("failed to load saved providers", "error", err)
	}

	// Legacy anthropic credentials (config section or old env var) seed the
	// provider store like any other key — anthropic is not privileged.
	if cred := legacyAnthropicCredential(cfg); cred != "" {
		logger.Warn("legacy anthropic credentials detected — prefer provider env vars or the provider store")
		if err := providerStore.SetAPIKey("anthropic", cred); err != nil {
			logger.Warn("failed to seed anthropic key from legacy config", "error", err)
		}
	}

	// Environment credentials: every registered profile's env vars, in
	// deterministic registry order. The first hit also drives auto-selection.
	envProvider := seedEnvCredentials(providerStore, logger)

	// Resolve the active provider. Order: explicit config → env var profile →
	// saved active provider → reachable local ollama → none (setup wizard).
	resolveActiveProvider(cfg, providerStore, envProvider, logger)

	cloudAvailable := providerStore.GetActive() != nil

	// Initialize the Ollama model-management client (chat goes through the
	// provider layer's ollama profile).
	var ollamaClient *axiosd.OllamaClient
	ollamaEnabled := cfg.Ollama.Enabled
	if ollamaEnabled {
		ollamaClient = axiosd.NewOllamaClient(cfg.Ollama.Host, cfg.Ollama.Port)
		if err := ollamaClient.Ping(); err != nil {
			logger.Warn("Ollama not reachable, disabling", "error", err)
			ollamaEnabled = false
			ollamaClient = nil
		} else {
			logger.Info("Ollama connected", "host", cfg.Ollama.Host, "port", cfg.Ollama.Port)
		}
	}

	if !cloudAvailable && !ollamaEnabled {
		logger.Warn("no AI provider configured yet — complete setup in the web UI")
	}

	// Initialize router
	router := axiosd.NewRouter(axiosd.RoutingMode(cfg.Routing.Mode), logger)
	router.SetCloudAvailable(cloudAvailable)
	router.SetLocalAvailable(ollamaEnabled)

	// Initialize MCP manager
	mcpManager := axiosd.NewMCPManager(cfg.MCP.SocketDir, logger)
	defer mcpManager.Close()

	for _, serverName := range cfg.MCP.Servers {
		if err := mcpManager.Connect(serverName); err != nil {
			logger.Warn("MCP server not available", "server", serverName, "error", err)
		}
	}

	// Start HTTP/WebSocket server
	server := axiosd.NewServer(router, mcpManager, logger)
	server.SetProviderStore(providerStore)
	if ollamaClient != nil {
		server.SetOllama(ollamaClient)
	}

	// Permission policy: tiered trust enforced on every model-initiated
	// tool call, with a WebSocket approval flow for approval_required tiers.
	server.SetPermissionChecker(loadPermissionPolicy(cfg, logger))
	server.SetApprovalTimeout(time.Duration(cfg.Permissions.ApprovalTimeoutSeconds) * time.Second)

	// Background coding agent: a supervised `opencode serve` instance for
	// delegated self-coding tasks, with its permission asks bridged into the
	// AxiOS policy above. Must bind after SetPermissionChecker and before
	// BuildTools (which registers the opencode__ chat tools).
	opencodeMgr := axiosd.NewOpencodeManager(axiosd.OpencodeOptions{
		Enabled:   cfg.Opencode.Enabled,
		Binary:    cfg.Opencode.Binary,
		Port:      cfg.Opencode.Port,
		Workspace: cfg.Opencode.Workspace,
	}, providerStore, filepath.Join(dataDir, "opencode_tasks.json"), logger)
	server.SetOpencodeManager(opencodeMgr)
	if err := opencodeMgr.Start(); err != nil {
		logger.Warn("failed to start opencode manager", "error", err)
	}

	// Initialize host store; switching hosts swaps the management client and
	// invalidates the provider runtime's cached local client.
	var runtime *axiosd.ProviderRuntime
	hostsFilePath := filepath.Join(dataDir, "hosts.json")
	hostStore := axiosd.NewHostStore(func(client *axiosd.OllamaClient) {
		server.SetOllama(client)
		router.SetLocalAvailable(true)
		if runtime != nil {
			runtime.Rebuild()
		}
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

	server.SetHostStore(hostStore)

	// Provider runtime: resolves the active provider client from the stores.
	runtime = axiosd.NewProviderRuntime(providerStore, hostStore, logger)
	if localModel := resolveLocalModel(cfg); localModel != "" {
		runtime.SetLocalModel(localModel)
	}
	server.SetProviderRuntime(runtime)

	// Fallback chain from config.
	fallbacks := make([]axiosd.FallbackSpec, 0, len(cfg.FallbackProviders))
	for _, fb := range cfg.FallbackProviders {
		fallbacks = append(fallbacks, axiosd.FallbackSpec{Provider: fb.Provider, Model: fb.Model})
	}
	server.SetFallbackProviders(fallbacks)

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
		opencodeMgr.Stop()
		saveAll()
		os.Exit(0)
	}()

	server.BuildTools()
	addr := cfg.Server.Listen

	// Register with CasaOS-Gateway (if enabled)
	go server.RegisterWithGateway(addr, axiosd.GatewayConfig{
		Enabled:    cfg.Gateway.Enabled,
		GatewayURL: cfg.Gateway.URL,
	})

	activeProvider, activeModel := "", ""
	if active := providerStore.GetActive(); active != nil {
		activeProvider = active.ID
		activeModel = providerStore.GetActiveModel()
	}
	logger.Info("AxiOS axiosd starting",
		"addr", addr,
		"provider", activeProvider,
		"model", activeModel,
		"ollama", ollamaEnabled,
		"routing", cfg.Routing.Mode,
	)

	if err := server.ListenAndServe(addr); err != nil {
		logger.Error("server failed", "error", err)
		saveAll()
		os.Exit(1)
	}
}

// loadPermissionPolicy loads permissions.yaml for tool-call enforcement.
// Resolution order: the configured path (default /etc/axios/permissions.yaml)
// → the repo's configs/permissions.yaml (dev checkouts) → the built-in
// default policy (logged as a warning, never disabled).
func loadPermissionPolicy(cfg *config.Daemon, logger *slog.Logger) *permissions.Config {
	paths := []string{cfg.Permissions.Path}
	if cfg.Permissions.Path == "/etc/axios/permissions.yaml" {
		paths = append(paths, devPermissionsPath)
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			continue
		}
		permCfg, err := permissions.LoadConfig(path)
		if err != nil {
			logger.Error("failed to parse permissions config", "path", path, "error", err)
			continue
		}
		if permCfg.Deprecation != "" {
			logger.Warn(permCfg.Deprecation, "path", path)
		}
		logger.Info("permission policy loaded", "path", path, "version", permCfg.Version, "default_tier", string(permCfg.DefaultTier))
		return permCfg
	}

	logger.Warn("no permissions config found — using the built-in default policy",
		"configured_path", cfg.Permissions.Path,
		"dev_fallback", devPermissionsPath,
	)
	return permissions.Default()
}

// legacyAnthropicCredential resolves the deprecated anthropic credential
// sources: the ANTHROPIC_OAUTH_TOKEN env var and the legacy config section.
// (Current env vars — ANTHROPIC_API_KEY etc. — are handled generically via
// the profile registry in seedEnvCredentials.)
func legacyAnthropicCredential(cfg *config.Daemon) string {
	if v := os.Getenv("ANTHROPIC_OAUTH_TOKEN"); v != "" {
		return v
	}
	return cfg.LegacyAnthropicCredential()
}

// seedEnvCredentials walks the profile registry in deterministic order and
// seeds the provider store with any credentials found in the environment.
// It returns the name of the first profile whose env var was set (used for
// auto-selection), or "".
func seedEnvCredentials(store *axiosd.ProviderStore, logger *slog.Logger) string {
	first := ""
	for _, profile := range providers.List() {
		for _, envVar := range profile.EnvVars {
			v := os.Getenv(envVar)
			if v == "" {
				continue
			}
			if err := store.SetAPIKey(profile.Name, v); err != nil {
				// Profile without a catalog entry (e.g. "custom") — skip.
				logger.Debug("env credential for unknown catalog provider", "provider", profile.Name, "env", envVar)
				break
			}
			logger.Info("provider credential loaded from environment", "provider", profile.Name, "env", envVar)
			if first == "" {
				first = profile.Name
			}
			break
		}
	}
	return first
}

// resolveActiveProvider applies the boot resolution order:
// explicit model.provider in config → first profile with an env credential →
// saved active provider (already restored by LoadFromFile) → local ollama
// (handled by the router's local availability) → none.
func resolveActiveProvider(cfg *config.Daemon, store *axiosd.ProviderStore, envProvider string, logger *slog.Logger) {
	explicit := strings.ToLower(cfg.Model.Provider)
	if explicit != "" && explicit != "auto" {
		if explicit == "ollama" {
			// Local provider: nothing to activate in the cloud store; the
			// runtime's local client picks up model.id via resolveLocalModel.
			logger.Info("configured provider is ollama (local)", "model", cfg.Model.ID)
			return
		}
		model := cfg.Model.ID
		if model == "" {
			model = defaultModelForProfile(explicit)
		}
		if err := store.SetActive(explicit, model); err != nil {
			logger.Warn("configured model.provider could not be activated", "provider", explicit, "error", err)
			return
		}
		logger.Info("active provider set from config", "provider", explicit, "model", model)
		return
	}

	// auto: prefer an environment-configured provider.
	if envProvider != "" {
		model := defaultModelForProfile(envProvider)
		// Keep the saved model when the saved active provider matches.
		if active := store.GetActive(); active != nil && active.ID == envProvider && store.GetActiveModel() != "" {
			model = store.GetActiveModel()
		}
		if err := store.SetActive(envProvider, model); err != nil {
			logger.Warn("env provider could not be activated", "provider", envProvider, "error", err)
		} else {
			logger.Info("active provider set from environment", "provider", envProvider, "model", model)
		}
		return
	}

	// Saved active provider (restored by LoadFromFile) or nothing.
	if active := store.GetActive(); active != nil {
		logger.Info("restored saved active provider", "provider", active.ID, "model", store.GetActiveModel())
	}
}

// resolveLocalModel picks the local (ollama) model override from config.
func resolveLocalModel(cfg *config.Daemon) string {
	if strings.EqualFold(cfg.Model.Provider, "ollama") && cfg.Model.ID != "" {
		return cfg.Model.ID
	}
	return cfg.Ollama.Model
}

// defaultModelForProfile returns the registered profile's default model.
func defaultModelForProfile(name string) string {
	if p, ok := providers.Get(name); ok {
		return p.DefaultModel
	}
	return ""
}
