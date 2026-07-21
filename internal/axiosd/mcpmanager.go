package axiosd

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"

	"github.com/axios-os/axios/pkg/mcp"
)

// MCPManager manages connections to MCP servers.
type MCPManager struct {
	clients   map[string]*mcp.Client
	socketDir string
	mu        sync.RWMutex
	logger    *slog.Logger
}

// NewMCPManager creates a new MCP server manager.
func NewMCPManager(socketDir string, logger *slog.Logger) *MCPManager {
	if socketDir == "" {
		socketDir = mcp.SocketDir
	}
	return &MCPManager{
		clients:   make(map[string]*mcp.Client),
		socketDir: socketDir,
		logger:    logger,
	}
}

// Connect establishes a connection to an MCP server.
func (m *MCPManager) Connect(serverName string) error {
	socketPath := filepath.Join(m.socketDir, serverName+".sock")

	client, err := mcp.Dial(socketPath)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", serverName, err)
	}

	info, err := client.Initialize()
	if err != nil {
		client.Close()
		return fmt.Errorf("initialize %s: %w", serverName, err)
	}

	m.mu.Lock()
	old := m.clients[serverName]
	m.clients[serverName] = client
	m.mu.Unlock()
	if old != nil {
		// Replaced an existing client (e.g. a duplicate concurrent
		// reconnect) — don't leak its connection.
		old.Close()
	}

	m.logger.Info("connected to MCP server",
		"server", serverName,
		"version", info.Version,
		"tools", len(info.Tools),
	)
	return nil
}

// CallTool invokes a tool on the appropriate MCP server. When the server is
// not connected, or its connection has died (server crashed or restarted),
// the dead client is dropped and one reconnect (re-dial + re-Initialize) plus
// a single retry is attempted. A server that is genuinely down fails fast
// with a clear error; protocol-level errors (unknown tool, bad params) are
// returned as-is since reconnecting would not help.
func (m *MCPManager) CallTool(serverName, toolName string, params map[string]any) (*mcp.ToolResult, error) {
	m.mu.RLock()
	client, ok := m.clients[serverName]
	m.mu.RUnlock()

	if ok {
		result, err := client.CallTool(toolName, params)
		if err == nil {
			return result, nil
		}
		if !mcp.IsConnError(err) {
			return nil, err
		}
		m.logger.Warn("MCP server connection lost, attempting reconnect", "server", serverName, "error", err)
		m.dropClient(serverName, client)
	}

	if err := m.Connect(serverName); err != nil {
		if ok {
			return nil, fmt.Errorf("reconnect to MCP server %s: %w", serverName, err)
		}
		return nil, fmt.Errorf("MCP server %s not connected: %w", serverName, err)
	}

	m.mu.RLock()
	client, ok = m.clients[serverName]
	m.mu.RUnlock()
	if !ok {
		// The server died again between our reconnect and this retry and a
		// concurrent caller already dropped the fresh client.
		return nil, fmt.Errorf("MCP server %s not connected: connection lost during reconnect", serverName)
	}
	return client.CallTool(toolName, params)
}

// dropClient removes client from the map only if it is still the registered
// client for serverName — a concurrent reconnect may already have replaced
// it. The dead connection is closed either way.
func (m *MCPManager) dropClient(serverName string, client *mcp.Client) {
	m.mu.Lock()
	if m.clients[serverName] == client {
		delete(m.clients, serverName)
	}
	m.mu.Unlock()
	client.Close()
}

// ListTools returns all tools across all connected MCP servers.
func (m *MCPManager) ListTools() map[string][]mcp.ToolDefinition {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string][]mcp.ToolDefinition)
	for name, client := range m.clients {
		tools, err := client.ListTools()
		if err != nil {
			m.logger.Error("list tools failed", "server", name, "error", err)
			continue
		}
		result[name] = tools
	}
	return result
}

// Close disconnects from all MCP servers.
func (m *MCPManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, client := range m.clients {
		if err := client.Close(); err != nil {
			m.logger.Error("close MCP client failed", "server", name, "error", err)
		}
	}
}
