package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/axios-os/axios/internal/netctl"
	"github.com/axios-os/axios/pkg/logging"
	"github.com/axios-os/axios/pkg/mcp"
)

func main() {
	socketPath := flag.String("socket", mcp.SocketPath("axios-network"), "Unix socket path")
	flag.Parse()

	logger := logging.New("axios-network")

	server := mcp.NewServer("axios-network", "0.1.0")

	// --- list_interfaces ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "list_interfaces",
		Description: "List network interfaces with state, MTU, hardware address, and assigned IPs",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Permission: "trusted",
	}, handleListInterfaces)

	// --- dns_lookup ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "dns_lookup",
		Description: "Resolve a hostname to its IP addresses (and CNAME when present)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"host": map[string]any{
					"type":        "string",
					"description": "Hostname to resolve, e.g. example.com",
				},
			},
			"required": []string{"host"},
		},
		Permission: "trusted",
	}, handleDNSLookup)

	// --- ping_host ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "ping_host",
		Description: "Ping a host to check reachability and latency",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"host": map[string]any{
					"type":        "string",
					"description": "Hostname or IP address to ping",
				},
				"count": map[string]any{
					"type":        "integer",
					"description": "Number of pings to send, 1-5 (default 3)",
				},
			},
			"required": []string{"host"},
		},
		Permission: "trusted",
	}, handlePingHost)

	// --- tailscale_status ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "tailscale_status",
		Description: "Get Tailscale VPN status: this machine and its peers with IPs and online state",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Permission: "trusted",
	}, handleTailscaleStatus)

	// --- tailscale_up ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "tailscale_up",
		Description: "Connect this machine to the Tailscale VPN",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Permission: "approval_required",
	}, handleTailscaleUp)

	// --- tailscale_down ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "tailscale_down",
		Description: "Disconnect this machine from the Tailscale VPN",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Permission: "approval_required",
	}, handleTailscaleDown)

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.Info("received signal, shutting down", "signal", sig.String())
		server.Close()
		os.Exit(0)
	}()

	logger.Info("starting axios-network MCP server", "socket", *socketPath)
	if err := server.Serve(*socketPath); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}

// --- Tool handlers ---
// Handler errors become IsError tool results in pkg/mcp, so a missing
// tailscale CLI or an invalid host surfaces as a clear error message instead
// of crashing the server.

// requireString extracts a required non-empty string parameter.
func requireString(params map[string]any, key string) (string, error) {
	v, ok := params[key].(string)
	if !ok || v == "" {
		return "", fmt.Errorf("missing required parameter: %s", key)
	}
	return v, nil
}

func handleListInterfaces(params map[string]any) (string, error) {
	ifaces, err := netctl.ListInterfaces()
	if err != nil {
		return "", err
	}
	if len(ifaces) == 0 {
		return "no network interfaces found", nil
	}

	out, err := json.Marshal(ifaces)
	if err != nil {
		return "", fmt.Errorf("marshal interfaces: %w", err)
	}
	return fmt.Sprintf("%d interfaces:\n%s", len(ifaces), out), nil
}

func handleDNSLookup(params map[string]any) (string, error) {
	host, err := requireString(params, "host")
	if err != nil {
		return "", err
	}

	result, err := netctl.DNSLookup(host)
	if err != nil {
		return "", err
	}

	msg := fmt.Sprintf("%s resolves to: %s", result.Host, strings.Join(result.Addresses, ", "))
	if result.CNAME != "" {
		msg += fmt.Sprintf(" (CNAME: %s)", result.CNAME)
	}
	return msg, nil
}

func handlePingHost(params map[string]any) (string, error) {
	host, err := requireString(params, "host")
	if err != nil {
		return "", err
	}

	count := 3
	if c, ok := params["count"].(float64); ok && int(c) > 0 {
		count = int(c)
	}

	output, err := netctl.PingHost(host, count)
	if err != nil {
		return "", err
	}
	if output == "" {
		return fmt.Sprintf("no ping output for host %s", host), nil
	}
	return output, nil
}

func handleTailscaleStatus(params map[string]any) (string, error) {
	state, err := netctl.TailscaleStatus()
	if err != nil {
		return "", err
	}

	out, err := json.Marshal(state)
	if err != nil {
		return "", fmt.Errorf("marshal tailscale status: %w", err)
	}
	return fmt.Sprintf("tailscale status (%d peers):\n%s", len(state.Peers), out), nil
}

func handleTailscaleUp(params map[string]any) (string, error) {
	output, err := netctl.TailscaleUp()
	if err != nil {
		return "", err
	}
	if output == "" {
		return "tailscale is up", nil
	}
	return output, nil
}

func handleTailscaleDown(params map[string]any) (string, error) {
	output, err := netctl.TailscaleDown()
	if err != nil {
		return "", err
	}
	if output == "" {
		return "tailscale is down", nil
	}
	return output, nil
}
