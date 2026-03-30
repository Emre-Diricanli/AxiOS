package mcp

import "path/filepath"

const (
	// SocketDir is the default directory for MCP server Unix sockets.
	SocketDir = "/run/axios/mcp"
)

// SocketPath returns the Unix socket path for a given MCP server name.
// Example: SocketPath("axios-fs") -> "/run/axios/mcp/axios-fs.sock"
func SocketPath(serverName string) string {
	return filepath.Join(SocketDir, serverName+".sock")
}
