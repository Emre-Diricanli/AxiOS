package claused

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// CasaOS Gateway integration
//
// When running inside CasaOS, claused registers its API routes with the
// CasaOS-Gateway so they're accessible through the central reverse proxy.
// This is optional — claused works standalone without the gateway.

// GatewayConfig holds CasaOS-Gateway connection details.
type GatewayConfig struct {
	Enabled    bool   // Whether to attempt gateway registration
	GatewayURL string // e.g., "http://localhost:80"
}

// gatewayRoute is a route registration request for CasaOS-Gateway.
type gatewayRoute struct {
	Path   string `json:"path"`
	Target string `json:"target"`
}

// RegisterWithGateway registers claused's API routes with the CasaOS-Gateway.
// It discovers the gateway URL from the runtime file or environment, then
// registers all route prefixes that claused serves.
//
// This is a best-effort operation — if the gateway isn't running (e.g.,
// standalone dev mode), it logs a warning and continues.
func (s *Server) RegisterWithGateway(selfAddr string, gatewayCfg GatewayConfig) {
	if !gatewayCfg.Enabled {
		return
	}

	gatewayURL := gatewayCfg.GatewayURL
	if gatewayURL == "" {
		gatewayURL = discoverGatewayURL()
	}
	if gatewayURL == "" {
		s.logger.Warn("CasaOS-Gateway not found, skipping registration")
		return
	}

	// Normalize self address to a full URL
	target := selfAddr
	if !strings.HasPrefix(target, "http") {
		target = "http://" + target
	}

	// Routes that claused serves and should be exposed through the gateway
	routes := []gatewayRoute{
		{Path: "/v1/models", Target: target},
		{Path: "/v1/chat/completions", Target: target},
		{Path: "/api/health", Target: target},
		{Path: "/api/status", Target: target},
		{Path: "/api/models", Target: target},
		{Path: "/api/providers", Target: target},
		{Path: "/api/hosts", Target: target},
		{Path: "/api/system/stats", Target: target},
		{Path: "/api/docker", Target: target},
		{Path: "/api/fs", Target: target},
		{Path: "/api/chat", Target: target},
		{Path: "/api/ai", Target: target},
		{Path: "/ws", Target: target},
	}

	client := &http.Client{Timeout: 5 * time.Second}
	registered := 0

	for _, route := range routes {
		body, _ := json.Marshal(route)
		req, err := http.NewRequest("POST", gatewayURL+"/v1/gateway/routes", bytes.NewReader(body))
		if err != nil {
			s.logger.Error("failed to create gateway registration request", "path", route.Path, "error", err)
			continue
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			s.logger.Warn("gateway registration failed", "path", route.Path, "error", err)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			registered++
		} else {
			s.logger.Warn("gateway rejected route", "path", route.Path, "status", resp.StatusCode)
		}
	}

	if registered > 0 {
		s.logger.Info("registered with CasaOS-Gateway", "routes", registered, "gateway", gatewayURL)
	} else {
		s.logger.Warn("no routes registered with gateway")
	}
}

// discoverGatewayURL finds the CasaOS-Gateway address.
// Priority: CASAOS_GATEWAY_URL env > runtime file > default localhost:80
func discoverGatewayURL() string {
	// 1. Environment variable
	if url := os.Getenv("CASAOS_GATEWAY_URL"); url != "" {
		return url
	}

	// 2. CasaOS runtime file (written by the gateway on startup)
	runtimePaths := []string{
		"/var/run/casaos/gateway.url",
		"/run/casaos/gateway.url",
	}
	for _, path := range runtimePaths {
		data, err := os.ReadFile(path)
		if err == nil {
			url := strings.TrimSpace(string(data))
			if url != "" {
				return url
			}
		}
	}

	// 3. Try common gateway ports
	client := &http.Client{Timeout: 2 * time.Second}
	for _, port := range []string{"80", "8080"} {
		url := fmt.Sprintf("http://localhost:%s", port)
		resp, err := client.Get(url + "/v1/gateway/routes")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode < 500 {
				return url
			}
		}
	}

	return ""
}
