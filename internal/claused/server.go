package claused

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
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
		system: `You are Claude, the AI assistant powering AxiOS — an AI-native operating system. You have direct access to the system hardware and software through tools.

CRITICAL RULES:
1. When you get tool results, ALWAYS interpret and summarize them in a clear, human-friendly response. NEVER dump raw tool output to the user. For example, if system_info returns memory stats, say "You have 16GB RAM, 8GB in use" — not the raw /proc/meminfo output.
2. Be concise. Short sentences, no filler. Format with markdown when helpful.
3. When asked to do something, do it with tools — don't just explain how.
4. If a tool returns an error, explain what went wrong and suggest a fix.
5. When listing files or processes, present them in a clean formatted way, not raw command output.
6. You are part of the OS. Speak as the system's intelligence, not as an external chatbot.`,
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
	mux.HandleFunc("/ws/terminal", s.handleTerminalWebSocket)
	mux.HandleFunc("/ws", s.handleWebSocket)
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/status", s.handleStatus)

	// System stats endpoint (gathered directly, no MCP)
	mux.HandleFunc("/api/system/stats", s.handleSystemStats)

	// Filesystem REST endpoints (call axios-fs MCP server directly)
	mux.HandleFunc("/api/fs/list", s.handleFSList)
	mux.HandleFunc("/api/fs/read", s.handleFSRead)
	mux.HandleFunc("/api/fs/info", s.handleFSInfo)
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

// terminalResizeMsg is the JSON message format for terminal resize events.
type terminalResizeMsg struct {
	Type string `json:"type"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

func (s *Server) handleTerminalWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("terminal websocket upgrade failed", "error", err)
		return
	}
	defer conn.Close()

	s.logger.Info("terminal websocket client connected")

	// Determine shell to use
	shell := "/bin/bash"
	if _, err := os.Stat("/bin/zsh"); err == nil {
		shell = "/bin/zsh"
	}

	cmd := exec.Command(shell)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptmx, err := pty.Start(cmd)
	if err != nil {
		s.logger.Error("failed to start pty", "error", err)
		conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Failed to start shell: %v\r\n", err)))
		return
	}
	defer func() {
		_ = ptmx.Close()
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	// Set initial size
	_ = pty.Setsize(ptmx, &pty.Winsize{Rows: 24, Cols: 80})

	var once sync.Once
	done := make(chan struct{})

	// PTY -> WebSocket
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if err != nil {
				once.Do(func() { close(done) })
				return
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
				once.Do(func() { close(done) })
				return
			}
		}
	}()

	// WebSocket -> PTY
	go func() {
		for {
			msgType, msg, err := conn.ReadMessage()
			if err != nil {
				once.Do(func() { close(done) })
				return
			}

			// Check if it's a JSON resize message
			if msgType == websocket.TextMessage {
				var resize terminalResizeMsg
				if json.Unmarshal(msg, &resize) == nil && resize.Type == "resize" {
					_ = pty.Setsize(ptmx, &pty.Winsize{
						Rows: resize.Rows,
						Cols: resize.Cols,
					})
					continue
				}
			}

			// Otherwise forward raw data to PTY
			if _, err := ptmx.Write(msg); err != nil {
				once.Do(func() { close(done) })
				return
			}
		}
	}()

	<-done
	s.logger.Info("terminal websocket client disconnected")
}

func (s *Server) sendMessage(conn *websocket.Conn, msg ChatMessage) {
	if err := conn.WriteJSON(msg); err != nil {
		s.logger.Error("websocket write failed", "error", err)
	}
}

// --- Filesystem REST endpoints ---

func (s *Server) handleFSList(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/"
	}

	result, err := s.mcpManager.CallTool("axios-fs", "list_directory", map[string]any{"path": path})
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}
	if result.IsError {
		s.jsonError(w, result.Content, http.StatusInternalServerError)
		return
	}

	// result.Content is already a JSON array from the MCP server
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"entries":%s}`, result.Content)
}

func (s *Server) handleFSRead(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		s.jsonError(w, "path parameter required", http.StatusBadRequest)
		return
	}

	result, err := s.mcpManager.CallTool("axios-fs", "read_file", map[string]any{"path": path})
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}
	if result.IsError {
		s.jsonError(w, result.Content, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	data, _ := json.Marshal(map[string]string{"content": result.Content})
	w.Write(data)
}

func (s *Server) handleFSInfo(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		s.jsonError(w, "path parameter required", http.StatusBadRequest)
		return
	}

	result, err := s.mcpManager.CallTool("axios-fs", "file_info", map[string]any{"path": path})
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}
	if result.IsError {
		s.jsonError(w, result.Content, http.StatusInternalServerError)
		return
	}

	// result.Content is already JSON from the MCP server
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(result.Content))
}

func (s *Server) jsonError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	data, _ := json.Marshal(map[string]string{"error": msg})
	w.Write(data)
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe(addr string) error {
	mux := http.NewServeMux()
	s.SetupRoutes(mux)

	s.logger.Info("starting server", "addr", addr)
	return http.ListenAndServe(addr, mux)
}
