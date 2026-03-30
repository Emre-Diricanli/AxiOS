// Package mcp implements the Model Context Protocol for AxiOS.
// MCP servers expose system capabilities as tools that AI models can call.
package mcp

// ToolDefinition describes a tool that an MCP server exposes.
type ToolDefinition struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	InputSchema map[string]any      `json:"inputSchema"`
	Permission  string              `json:"permission"` // "trusted", "approval_required", "prohibited"
}

// ToolCall represents a request to invoke a tool.
type ToolCall struct {
	ID     string         `json:"id"`
	Name   string         `json:"name"`
	Params map[string]any `json:"params"`
}

// ToolResult represents the outcome of a tool invocation.
type ToolResult struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	IsError bool   `json:"isError"`
}

// ServerInfo describes an MCP server's identity and capabilities.
type ServerInfo struct {
	Name    string           `json:"name"`
	Version string           `json:"version"`
	Tools   []ToolDefinition `json:"tools"`
}

// Request is a JSON-RPC style request from claused to an MCP server.
type Request struct {
	Method string         `json:"method"` // "initialize", "tools/list", "tools/call"
	ID     string         `json:"id"`
	Params map[string]any `json:"params,omitempty"`
}

// Response is a JSON-RPC style response from an MCP server to claused.
type Response struct {
	ID     string `json:"id"`
	Result any    `json:"result,omitempty"`
	Error  *Error `json:"error,omitempty"`
}

// Error represents an MCP protocol error.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
