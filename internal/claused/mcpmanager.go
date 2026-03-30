package claused

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
	m.clients[serverName] = client
	m.mu.Unlock()

	m.logger.Info("connected to MCP server",
		"server", serverName,
		"version", info.Version,
		"tools", len(info.Tools),
	)
	return nil
}

// CallTool invokes a tool on the appropriate MCP server.
func (m *MCPManager) CallTool(serverName, toolName string, params map[string]any) (*mcp.ToolResult, error) {
	m.mu.RLock()
	client, ok := m.clients[serverName]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("MCP server %s not connected", serverName)
	}

	return client.CallTool(toolName, params)
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
