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
	ollama     *OllamaClient
	router     *Router
	sessions   *SessionStore
	mcpManager *MCPManager
	upgrader   websocket.Upgrader
	logger     *slog.Logger
	system     string
	tools      []any         // Tool definitions for Claude
	ollamaTools []ollamaTool // Tool definitions for Ollama
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
	s.ollamaTools = ConvertToolsForOllama(tools)
	return tools
}

// SetOllama sets the Ollama client on the server.
func (s *Server) SetOllama(client *OllamaClient) {
	s.ollama = client
}

// SetupRoutes registers HTTP handlers.
func (s *Server) SetupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/ws/terminal", s.handleTerminalWebSocket)
	mux.HandleFunc("/ws", s.handleWebSocket)
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/status", s.handleStatus)

	// System stats endpoint (gathered directly, no MCP)
	mux.HandleFunc("/api/system/stats", s.handleSystemStats)

	// Model management
	mux.HandleFunc("/api/models", s.handleListModels)
	mux.HandleFunc("/api/models/current", s.handleCurrentModel)
	mux.HandleFunc("/api/models/switch", s.handleSwitchModel)

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
	status := map[string]any{
		"backend": string(s.router.Route()),
		"routing": string(s.router.mode),
	}
	if s.anthropic != nil {
		status["authType"] = string(s.anthropic.authType)
		status["cloudModel"] = s.anthropic.model
	}
	if s.ollama != nil {
		status["localModel"] = s.ollama.model
	}
	json.NewEncoder(w).Encode(status)
}

func (s *Server) handleListModels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	type modelInfo struct {
		Name     string `json:"name"`
		Backend  string `json:"backend"` // "cloud" or "local"
		Active   bool   `json:"active"`
	}

	var models []modelInfo

	// Cloud models
	if s.anthropic != nil {
		active := s.router.Route() == BackendCloud
		models = append(models, modelInfo{Name: s.anthropic.model, Backend: "cloud", Active: active})
	}

	// Local models from Ollama
	if s.ollama != nil {
		ollamaModels, err := s.ollama.ListModels()
		if err == nil {
			activeLocal := s.router.Route() == BackendLocal
			for _, m := range ollamaModels {
				models = append(models, modelInfo{
					Name:    m,
					Backend: "local",
					Active:  activeLocal && m == s.ollama.model,
				})
			}
		}
	}

	json.NewEncoder(w).Encode(map[string]any{"models": models})
}

func (s *Server) handleCurrentModel(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	backend := s.router.Route()
	model := ""
	if backend == BackendCloud && s.anthropic != nil {
		model = s.anthropic.model
	} else if backend == BackendLocal && s.ollama != nil {
		model = s.ollama.model
	}
	json.NewEncoder(w).Encode(map[string]any{
		"model":   model,
		"backend": string(backend),
	})
}

func (s *Server) handleSwitchModel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Model   string `json:"model"`
		Backend string `json:"backend"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	switch req.Backend {
	case "cloud":
		if s.anthropic == nil {
			s.jsonError(w, "no cloud credentials configured", http.StatusBadRequest)
			return
		}
		s.anthropic.model = req.Model
		s.router.mode = RouteCloudOnly
		s.logger.Info("switched to cloud model", "model", req.Model)
	case "local":
		if s.ollama == nil {
			s.jsonError(w, "Ollama not available", http.StatusBadRequest)
			return
		}
		s.ollama.model = req.Model
		s.router.mode = RouteLocalOnly
		s.logger.Info("switched to local model", "model", req.Model)
	default:
		s.jsonError(w, "backend must be 'cloud' or 'local'", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "model": req.Model, "backend": req.Backend})
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
			if s.ollama == nil {
				s.sendMessage(conn, ChatMessage{
					Type:    "error",
					Content: "Ollama not configured. Install Ollama and set ollama.enabled=true in config.",
				})
			} else {
				s.handleLocalMessage(conn, session)
			}
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

		s.logger.Info("anthropic response", "stop_reason", apiResp.StopReason, "content_blocks", len(apiResp.Content))

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

		if !hasToolUse || apiResp.StopReason != "tool_use" {
			// No tool use or Claude signaled end_turn — we're done
			s.logger.Info("loop done", "hasToolUse", hasToolUse, "stop_reason", apiResp.StopReason, "text_len", len(textContent))
			return
		}

		s.logger.Info("continuing agentic loop with tool results", "tool_count", len(toolResults))

		// Add tool results to session and loop again
		session.AddMessage(Message{Role: "user", Content: toolResults})
	}

	s.logger.Warn("hit max agentic loop iterations")
}

func (s *Server) handleLocalMessage(conn *websocket.Conn, session *Session) {
	messages := session.GetMessages()
	ollamaMessages := ConvertMessagesForOllama(messages)

	s.logger.Info("sending to ollama", "message_count", len(ollamaMessages))

	// Try with tools first, fall back to no tools if the model doesn't support them
	resp, err := s.ollama.Chat(s.system, ollamaMessages, s.ollamaTools)
	if err != nil {
		s.logger.Error("ollama request failed", "error", err)

		// Try streaming without tools as fallback
		stream, streamErr := s.ollama.StreamChat(s.system, ollamaMessages)
		if streamErr != nil {
			s.sendMessage(conn, ChatMessage{
				Type:    "error",
				Content: fmt.Sprintf("Ollama error: %v", err),
			})
			return
		}
		defer stream.Close()

		var fullResponse string
		ParseOllamaStream(stream, func(text string, done bool) {
			if text != "" {
				fullResponse += text
				s.sendMessage(conn, ChatMessage{
					Type:    "assistant",
					Content: text,
					Model:   s.ollama.model,
				})
			}
		})
		session.AddMessage(Message{Role: "assistant", Content: fullResponse})
		return
	}

	// Handle tool calls from Ollama
	if len(resp.Message.ToolCalls) > 0 {
		for _, tc := range resp.Message.ToolCalls {
			toolName := tc.Function.Name
			s.logger.Info("ollama tool call", "tool", toolName)

			inputJSON, _ := json.Marshal(tc.Function.Arguments)
			s.sendMessage(conn, ChatMessage{
				Type:     "tool_use",
				ToolName: toolName,
				Content:  string(inputJSON),
			})

			result := s.executeTool(toolName, "", inputJSON)
			s.sendMessage(conn, ChatMessage{
				Type:     "tool_result",
				ToolName: toolName,
				Content:  result,
			})

			// Add tool result as context and ask Ollama to summarize
			ollamaMessages = append(ollamaMessages, ollamaChatMessage{
				Role:    "assistant",
				Content: fmt.Sprintf("I called %s and got: %s", toolName, result),
			})
		}

		// Follow up call for Ollama to summarize
		followUp, err := s.ollama.Chat(s.system, append(ollamaMessages, ollamaChatMessage{
			Role:    "user",
			Content: "Now summarize the tool results in a clear, concise response.",
		}), nil)
		if err == nil && followUp.Message.Content != "" {
			s.sendMessage(conn, ChatMessage{
				Type:    "assistant",
				Content: followUp.Message.Content,
				Model:   s.ollama.model,
			})
			session.AddMessage(Message{Role: "assistant", Content: followUp.Message.Content})
		}
		return
	}

	// Simple text response
	if resp.Message.Content != "" {
		s.sendMessage(conn, ChatMessage{
			Type:    "assistant",
			Content: resp.Message.Content,
			Model:   s.ollama.model,
		})
		session.AddMessage(Message{Role: "assistant", Content: resp.Message.Content})
	}
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
