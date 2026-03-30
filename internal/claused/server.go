package claused

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/gorilla/websocket"
)

// Server is the main HTTP/WebSocket server for claused.
type Server struct {
	anthropic  *AnthropicClient
	router     *Router
	sessions   *SessionStore
	mcpManager *MCPManager
	upgrader   websocket.Upgrader
	logger     *slog.Logger
	system     string
	tools      []any // Tool definitions for Claude
}

// NewServer creates a new claused HTTP server.
func NewServer(anthropic *AnthropicClient, router *Router, mcpManager *MCPManager, logger *slog.Logger) *Server {
	return &Server{
		anthropic:  anthropic,
		router:     router,
		sessions:   NewSessionStore(),
		mcpManager: mcpManager,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		logger: logger,
		system: `You are the AI assistant running on AxiOS, an AI-native operating system. You have direct access to the system hardware and software through tools. You help the user manage their system, run commands, manage files, and accomplish creative work.

Be concise and direct. When the user asks you to do something on the system, do it — don't just explain how. Use the tools available to you to interact with the system.`,
	}
}

// ChatMessage is the WebSocket message format between the web UI and claused.
type ChatMessage struct {
	Type      string `json:"type"`                // "user", "assistant", "error", "status", "tool_use", "tool_result"
	Content   string `json:"content"`
	SessionID string `json:"sessionId,omitempty"`
	Model     string `json:"model,omitempty"`
	ToolName  string `json:"toolName,omitempty"`
	ToolID    string `json:"toolId,omitempty"`
}

// BuildTools generates Claude tool definitions from connected MCP servers.
func (s *Server) BuildTools() []any {
	allTools := s.mcpManager.ListTools()
	var tools []any

	for serverName, serverTools := range allTools {
		for _, t := range serverTools {
			// The InputSchema from MCP servers is already a complete JSON Schema object
			// with "type", "properties", and "required" fields
			tool := map[string]any{
				"name":         serverName + "__" + t.Name,
				"description":  t.Description,
				"input_schema": t.InputSchema,
			}
			tools = append(tools, tool)
		}
	}

	s.logger.Info("built tool definitions", "count", len(tools))
	s.tools = tools
	return tools
}

// SetupRoutes registers HTTP handlers.
func (s *Server) SetupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/ws", s.handleWebSocket)
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/status", s.handleStatus)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"backend":  string(s.router.Route()),
		"routing":  string(s.router.mode),
		"authType": string(s.anthropic.authType),
		"model":    s.anthropic.model,
	})
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("websocket upgrade failed", "error", err)
		return
	}
	defer conn.Close()

	s.logger.Info("websocket client connected")

	for {
		var msg ChatMessage
		if err := conn.ReadJSON(&msg); err != nil {
			s.logger.Info("websocket client disconnected", "error", err)
			return
		}

		if msg.Type != "user" {
			continue
		}

		s.logger.Info("received user message", "content", msg.Content, "session", msg.SessionID)

		sessionID := msg.SessionID
		if sessionID == "" {
			sessionID = "default"
		}

		session := s.sessions.Get(sessionID)
		session.AddMessage(Message{Role: "user", Content: msg.Content})

		// Route to appropriate backend
		backend := s.router.Route()
		s.logger.Info("routing to backend", "backend", string(backend))

		switch backend {
		case BackendCloud:
			s.handleCloudMessage(conn, session)
		case BackendLocal:
			s.sendMessage(conn, ChatMessage{
				Type:    "error",
				Content: "Local model backend not yet implemented",
			})
		}

		// Signal to the UI that the response is complete
		s.sendMessage(conn, ChatMessage{
			Type:    "status",
			Content: "done",
		})
	}
}

// anthropicResponse is the response body from the Anthropic Messages API.
type anthropicResponse struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Role      string         `json:"role"`
	Content   []contentBlock `json:"content"`
	StopReason string        `json:"stop_reason"`
}

type contentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

func (s *Server) handleCloudMessage(conn *websocket.Conn, session *Session) {
	// Agentic loop: keep calling Claude until we get a response with no tool use
	for i := 0; i < 20; i++ { // Max 20 iterations to prevent infinite loops
		messages := session.GetMessages()
		s.logger.Info("sending to anthropic", "message_count", len(messages), "iteration", i)

		resp, err := s.anthropic.SendMessage(s.system, messages, s.tools)
		if err != nil {
			s.logger.Error("anthropic request failed", "error", err)
			s.sendMessage(conn, ChatMessage{
				Type:    "error",
				Content: fmt.Sprintf("Failed to connect to Claude: %v", err),
			})
			if s.router.mode == RouteAuto {
				s.router.SetCloudAvailable(false)
			}
			return
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			s.logger.Error("read response body failed", "error", err)
			return
		}

		if resp.StatusCode != http.StatusOK {
			s.logger.Error("anthropic API error", "status", resp.StatusCode, "body", string(body))
			s.sendMessage(conn, ChatMessage{
				Type:    "error",
				Content: fmt.Sprintf("Claude API error %d: %s", resp.StatusCode, string(body)),
			})
			return
		}

		var apiResp anthropicResponse
		if err := json.Unmarshal(body, &apiResp); err != nil {
			s.logger.Error("unmarshal response failed", "error", err)
			return
		}

		// Process content blocks
		var hasToolUse bool
		var textContent string
		var toolResults []any

		for _, block := range apiResp.Content {
			switch block.Type {
			case "text":
				textContent += block.Text
				s.sendMessage(conn, ChatMessage{
					Type:    "assistant",
					Content: block.Text,
					Model:   "claude",
				})

			case "tool_use":
				hasToolUse = true
				s.logger.Info("tool use requested", "tool", block.Name, "id", block.ID)

				// Tell the UI what tool is being called
				inputStr, _ := json.Marshal(json.RawMessage(block.Input))
				s.sendMessage(conn, ChatMessage{
					Type:     "tool_use",
					ToolName: block.Name,
					ToolID:   block.ID,
					Content:  string(inputStr),
				})

				// Execute the tool via MCP
				result := s.executeTool(block.Name, block.ID, block.Input)

				// Tell the UI the result
				s.sendMessage(conn, ChatMessage{
					Type:     "tool_result",
					ToolID:   block.ID,
					ToolName: block.Name,
					Content:  result,
				})

				toolResults = append(toolResults, map[string]any{
					"type":       "tool_result",
					"tool_use_id": block.ID,
					"content":    result,
				})
			}
		}

		// Add assistant message to session (full content blocks as-is)
		session.AddMessage(Message{Role: "assistant", Content: apiResp.Content})

		if !hasToolUse {
			// No tool use — we're done
			return
		}

		// Add tool results to session and loop again
		session.AddMessage(Message{Role: "user", Content: toolResults})
	}

	s.logger.Warn("hit max agentic loop iterations")
}

// executeTool routes a tool call to the appropriate MCP server.
func (s *Server) executeTool(toolName, toolID string, rawInput json.RawMessage) string {
	// Tool names are formatted as "serverName__toolName"
	var serverName, mcpToolName string
	for i := 0; i < len(toolName)-1; i++ {
		if toolName[i] == '_' && toolName[i+1] == '_' {
			serverName = toolName[:i]
			mcpToolName = toolName[i+2:]
			break
		}
	}

	if serverName == "" {
		return fmt.Sprintf("error: invalid tool name format: %s", toolName)
	}

	var params map[string]any
	if err := json.Unmarshal(rawInput, &params); err != nil {
		return fmt.Sprintf("error: invalid tool input: %v", err)
	}

	s.logger.Info("executing MCP tool", "server", serverName, "tool", mcpToolName)

	result, err := s.mcpManager.CallTool(serverName, mcpToolName, params)
	if err != nil {
		s.logger.Error("MCP tool call failed", "error", err)
		return fmt.Sprintf("error: %v", err)
	}

	if result.IsError {
		return fmt.Sprintf("error: %s", result.Content)
	}

	return result.Content
}

func (s *Server) sendMessage(conn *websocket.Conn, msg ChatMessage) {
	if err := conn.WriteJSON(msg); err != nil {
		s.logger.Error("websocket write failed", "error", err)
	}
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe(addr string) error {
	mux := http.NewServeMux()
	s.SetupRoutes(mux)

	s.logger.Info("starting server", "addr", addr)
	return http.ListenAndServe(addr, mux)
}
