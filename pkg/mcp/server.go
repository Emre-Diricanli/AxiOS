package mcp

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
)

// ToolHandler is a function that handles a tool invocation.
type ToolHandler func(params map[string]any) (string, error)

// Server is an MCP server that listens on a Unix socket and handles tool calls.
type Server struct {
	info     ServerInfo
	handlers map[string]ToolHandler
	listener net.Listener
	logger   *slog.Logger
}

// NewServer creates a new MCP server with the given name and version.
func NewServer(name, version string) *Server {
	return &Server{
		info: ServerInfo{
			Name:    name,
			Version: version,
		},
		handlers: make(map[string]ToolHandler),
		logger:   slog.Default().With("server", name),
	}
}

// RegisterTool adds a tool to the server with its handler.
func (s *Server) RegisterTool(def ToolDefinition, handler ToolHandler) {
	s.info.Tools = append(s.info.Tools, def)
	s.handlers[def.Name] = handler
}

// Serve starts listening on the given Unix socket path.
func (s *Server) Serve(socketPath string) error {
	// Remove stale socket file if it exists
	os.Remove(socketPath)

	var err error
	s.listener, err = net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", socketPath, err)
	}
	defer s.listener.Close()

	s.logger.Info("listening", "socket", socketPath)

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			s.logger.Error("accept failed", "error", err)
			continue
		}
		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)

	for {
		var req Request
		if err := decoder.Decode(&req); err != nil {
			return // Connection closed
		}

		resp := s.handleRequest(req)
		if err := encoder.Encode(resp); err != nil {
			s.logger.Error("encode response failed", "error", err)
			return
		}
	}
}

func (s *Server) handleRequest(req Request) Response {
	switch req.Method {
	case "initialize":
		return Response{ID: req.ID, Result: s.info}

	case "tools/list":
		return Response{ID: req.ID, Result: s.info.Tools}

	case "tools/call":
		return s.handleToolCall(req)

	default:
		return Response{
			ID:    req.ID,
			Error: &Error{Code: -32601, Message: "method not found: " + req.Method},
		}
	}
}

func (s *Server) handleToolCall(req Request) Response {
	toolName, _ := req.Params["name"].(string)
	toolParams, _ := req.Params["params"].(map[string]any)

	handler, ok := s.handlers[toolName]
	if !ok {
		return Response{
			ID:    req.ID,
			Error: &Error{Code: -32602, Message: "unknown tool: " + toolName},
		}
	}

	s.logger.Info("tool call", "tool", toolName)

	result, err := handler(toolParams)
	if err != nil {
		return Response{
			ID: req.ID,
			Result: ToolResult{
				ID:      req.ID,
				Content: err.Error(),
				IsError: true,
			},
		}
	}

	return Response{
		ID: req.ID,
		Result: ToolResult{
			ID:      req.ID,
			Content: result,
		},
	}
}

// Close stops the server.
func (s *Server) Close() error {
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}
