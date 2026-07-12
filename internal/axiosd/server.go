package axiosd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"

	"github.com/axios-os/axios/pkg/mcp"
	"github.com/axios-os/axios/pkg/permissions"
	"github.com/axios-os/axios/pkg/providers"
)

// defaultApprovalTimeout bounds how long an approval_required tool call
// waits for the user's verdict before being denied (config
// permissions.approval_timeout_seconds overrides it).
const defaultApprovalTimeout = 120 * time.Second

// toolExecutor performs the actual tool call once the permission middleware
// allows it. *MCPManager satisfies it; tests supply fakes.
type toolExecutor interface {
	CallTool(serverName, toolName string, params map[string]any) (*mcp.ToolResult, error)
}

// Server is the main HTTP/WebSocket server for axiosd.
type Server struct {
	runtime       *ProviderRuntime
	ollama        *OllamaClient // model management/marketplace only — chat goes through the provider layer
	providerStore *ProviderStore
	router        *Router
	sessions      *SessionStore
	mcpManager    *MCPManager
	hostStore     *HostStore
	upgrader      websocket.Upgrader
	logger        *slog.Logger
	system        string
	toolDefs      []providers.ToolDef // canonical tool definitions from MCP servers
	fallbacks     []FallbackSpec      // provider fallback chain from config

	// Permission enforcement on model-initiated tool calls.
	permissions     PermissionChecker // never nil; defaults to the built-in policy
	executor        toolExecutor      // dispatches allowed tool calls (MCPManager)
	approvals       approvalRegistry  // in-flight approval_required requests
	approvalTimeout time.Duration     // deny approval_required calls after this long

	// Background coding agent (opencode) and the connected-UI registry its
	// permission bridge uses to reach a user for approvals.
	opencodeMgr *OpencodeManager
	sinksMu     sync.Mutex
	sinks       []wsSink // active websocket chat connections, oldest first

	// xAI SuperGrok subscription OAuth (device-code flow), created lazily.
	xaiOAuthOnce sync.Once
	xaiOAuthFlow *XAIOAuth
}

// NewServer creates a new axiosd HTTP server.
func NewServer(router *Router, mcpManager *MCPManager, logger *slog.Logger) *Server {
	return &Server{
		router:     router,
		sessions:   NewSessionStore(logger),
		mcpManager: mcpManager,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		logger:          logger,
		permissions:     permissions.Default(),
		executor:        mcpManager,
		approvalTimeout: defaultApprovalTimeout,
		system: `You are AxiOS System Intelligence — the AI brain of an AI-native operating system called AxiOS. You are NOT a chatbot. You ARE the operating system's intelligence layer. You live inside the machine and have direct hardware access.

# Identity
- Your name is "AxiOS" when asked. Never say "I'm Claude", "I'm GPT", "I'm Llama", or any model name.
- You are the system itself speaking. Talk like a knowledgeable sysadmin who happens to live inside the computer.
- Be confident and direct. You own this machine.

# Tools & Capabilities
You have access to system tools that let you:
- axios-system > run_command: Execute any bash/shell command on the host
- axios-system > system_info: Get CPU, memory, OS, kernel details
- axios-system > disk_usage: Check storage across all mounted drives
- axios-system > process_list: See top processes by memory usage
- axios-system > service_status: Check systemd service status
- axios-fs > read_file: Read any file on the system
- axios-fs > write_file: Create or modify files
- axios-fs > list_directory: Browse directories
- axios-fs > search_files: Find files by pattern
- axios-fs > file_info: Get file metadata (size, permissions, dates)

IMPORTANT: When the user asks you to DO something, USE THE TOOLS TO DO IT. Never just show commands or explain how — actually execute them using run_command or the appropriate tool.

# Examples of what to do vs not do
- User: "kill all my containers" → Use run_command with "docker stop $(docker ps -q) && docker rm $(docker ps -aq)". Do NOT just print the command.
- User: "what's using my memory" → Use process_list or system_info, then summarize. Do NOT tell them to run htop.
- User: "create a file called test.txt" → Use write_file. Do NOT show them an echo command.
- User: "install nginx" → Use run_command with "brew install nginx" or "apt install nginx". Just do it.

# Response Rules
1. NEVER dump raw tool output. Always interpret and summarize in plain English.
2. Use proper human-readable units: GB not bytes, percentages for usage.
3. Be concise: 2-4 sentences for simple questions. Use markdown formatting.
4. Use tables, bold, bullet points, and code blocks where they help readability.
5. If a tool fails, explain the error simply and suggest what to do.
6. For casual conversation (hey, what's up), respond naturally without running tools.
7. You have FULL system access. The user trusts you to execute commands. Don't be cautious — act.
8. The user is the admin of this machine. Be helpful, fast, and accurate.`,
	}
}

// ChatMessage is the WebSocket message format between the web UI and axiosd.
type ChatMessage struct {
	Type      string `json:"type"` // "user", "assistant", "error", "status", "tool_use", "tool_result", "approval_request", "approval_response"
	Content   string `json:"content"`
	SessionID string `json:"sessionId,omitempty"`
	Model     string `json:"model,omitempty"`
	Provider  string `json:"provider,omitempty"`
	ToolName  string `json:"toolName,omitempty"`
	ToolID    string `json:"toolId,omitempty"`

	// Mode selects the chat backend for user messages: "" = the provider
	// chat loop; "code" = an interactive opencode session. Directory
	// optionally sets the project directory when a code session is created.
	Mode      string `json:"mode,omitempty"`
	Directory string `json:"directory,omitempty"`

	// Approval flow fields. approval_request (daemon → UI) carries ID, Tool,
	// and Params; approval_response (UI → daemon) carries ID and Approve.
	ID      string          `json:"id,omitempty"`
	Tool    string          `json:"tool,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Approve bool            `json:"approve,omitempty"`
}

// BuildTools generates canonical tool definitions from connected MCP servers.
// Transports convert them to each provider's wire format.
func (s *Server) BuildTools() []providers.ToolDef {
	allTools := s.mcpManager.ListTools()
	var tools []providers.ToolDef

	for serverName, serverTools := range allTools {
		for _, t := range serverTools {
			// The InputSchema from MCP servers is already a complete JSON Schema object
			// with "type", "properties", and "required" fields
			tools = append(tools, providers.ToolDef{
				Name:        serverName + "__" + t.Name,
				Description: t.Description,
				InputSchema: t.InputSchema,
			})
		}
	}

	// Built-in coding-delegation tools ride the same permission pipeline as
	// MCP tools (approval_required by default via the "opencode__*" policy).
	if s.opencodeMgr != nil {
		tools = append(tools, opencodeToolDefs()...)
	}

	s.logger.Info("built tool definitions", "count", len(tools))
	s.toolDefs = tools
	return tools
}

// SetOllama sets the Ollama client on the server (model management APIs only).
func (s *Server) SetOllama(client *OllamaClient) {
	s.ollama = client
}

// SetProviderRuntime sets the provider runtime on the server.
func (s *Server) SetProviderRuntime(rt *ProviderRuntime) {
	s.runtime = rt
}

// SetFallbackProviders sets the provider fallback chain used by the chat loop.
func (s *Server) SetFallbackProviders(chain []FallbackSpec) {
	s.fallbacks = chain
}

// SetPermissionChecker replaces the permission policy enforced on
// model-initiated tool calls. Nil checkers are ignored — enforcement can
// never be switched off.
func (s *Server) SetPermissionChecker(pc PermissionChecker) {
	if pc != nil {
		s.permissions = pc
	}
}

// SetApprovalTimeout overrides how long approval_required tool calls wait
// for the user's verdict. Non-positive values are ignored.
func (s *Server) SetApprovalTimeout(d time.Duration) {
	if d > 0 {
		s.approvalTimeout = d
	}
}

// SetOpencodeManager attaches the background coding agent and binds its
// permission bridge to this server's policy and approval flow. Call after
// SetPermissionChecker and before BuildTools.
func (s *Server) SetOpencodeManager(m *OpencodeManager) {
	s.opencodeMgr = m
	if m == nil {
		return
	}
	m.bind(
		func(toolName string, args map[string]any) permissions.Tier {
			return s.permissions.Check(toolName, args)
		},
		s.requestGlobalApproval,
	)
	// Persist finished code-chat turns into the AxiOS chat history so the
	// transcript survives reloads.
	m.bindTurnComplete(func(chatSessionID, text string) {
		s.sessions.Get(chatSessionID).AddMessage(providers.Message{Role: "assistant", Content: text})
		if err := s.sessions.Save(); err != nil {
			s.logger.Error("failed to save sessions after code turn", "error", err)
		}
	})
}

// registerSink tracks an active websocket chat connection so out-of-band
// approval requests (the opencode permission bridge) can reach a user.
func (s *Server) registerSink(sink wsSink) {
	s.sinksMu.Lock()
	s.sinks = append(s.sinks, sink)
	s.sinksMu.Unlock()
}

// unregisterSink removes a disconnected websocket connection.
func (s *Server) unregisterSink(sink wsSink) {
	s.sinksMu.Lock()
	for i, candidate := range s.sinks {
		if candidate == sink {
			s.sinks = append(s.sinks[:i], s.sinks[i+1:]...)
			break
		}
	}
	s.sinksMu.Unlock()
}

// latestSink returns the most recently connected websocket client, or nil.
func (s *Server) latestSink() wsSink {
	s.sinksMu.Lock()
	defer s.sinksMu.Unlock()
	if len(s.sinks) == 0 {
		return nil
	}
	return s.sinks[len(s.sinks)-1]
}

// requestGlobalApproval routes an out-of-band approval request (not tied to
// a chat turn) to the most recently connected UI client. With no client
// connected there is nobody to ask, so the request is denied immediately —
// fail closed rather than leaving the caller blocked on an unanswerable ask.
func (s *Server) requestGlobalApproval(ctx context.Context, tool string, params json.RawMessage) bool {
	sink := s.latestSink()
	if sink == nil {
		s.logger.Warn("approval required but no UI client is connected — denying", "tool", tool)
		return false
	}
	approved, outcome := s.awaitApproval(ctx, sink, tool, params)
	s.logPermissionDecision(tool, permissions.ApprovalRequired, outcome, 0)
	return approved
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
	mux.HandleFunc("/api/fs/write", s.handleFSWrite)
	mux.HandleFunc("/api/fs/raw", s.handleFSRaw)
	mux.HandleFunc("/api/fs/info", s.handleFSInfo)

	// Filesystem management endpoints (rename, copy, move, delete, mkdir)
	mux.HandleFunc("/api/fs/rename", s.handleFSRename)
	mux.HandleFunc("/api/fs/copy", s.handleFSCopy)
	mux.HandleFunc("/api/fs/move", s.handleFSMove)
	mux.HandleFunc("/api/fs/delete", s.handleFSDelete)
	mux.HandleFunc("/api/fs/mkdir", s.handleFSMkdir)
	mux.HandleFunc("/api/fs/upload", s.handleFSUpload)
	mux.HandleFunc("/api/fs/bulk-delete", s.handleFSBulkDelete)

	// Model marketplace
	mux.HandleFunc("/api/models/installed", s.handleModelsInstalled)
	mux.HandleFunc("/api/models/marketplace", s.handleModelsMarketplace)
	mux.HandleFunc("/api/models/pull", s.handleModelPull)
	mux.HandleFunc("/api/models/delete", s.handleModelDelete)
	mux.HandleFunc("/api/models/info", s.handleModelInfo)

	// Ollama host management
	mux.HandleFunc("/api/hosts", s.handleHosts)
	mux.HandleFunc("/api/hosts/activate", s.handleHostAction)
	mux.HandleFunc("/api/hosts/health", s.handleHostHealth)

	// Cloud provider management
	mux.HandleFunc("/api/providers", s.handleProviders)
	mux.HandleFunc("/api/providers/key", s.handleProviderKey)
	mux.HandleFunc("/api/providers/activate", s.handleProviderActivate)

	// xAI SuperGrok subscription OAuth (funds delegated opencode tasks)
	mux.HandleFunc("/api/providers/xai/oauth/start", s.handleXAIOAuthStart)
	mux.HandleFunc("/api/providers/xai/oauth/status", s.handleXAIOAuthStatus)

	// Chat session management
	mux.HandleFunc("/api/chat/sessions", s.handleSessionsRouter)
	mux.HandleFunc("/api/chat/sessions/messages", s.handleSessionsMessages)

	// AI quick-ask endpoint (non-streaming, one-shot)
	mux.HandleFunc("/api/ai/ask", s.handleAIAsk)

	// Docker management
	mux.HandleFunc("/api/docker/containers", s.handleDockerContainers)
	mux.HandleFunc("/api/docker/containers/inspect", s.handleDockerContainer)
	mux.HandleFunc("/api/docker/containers/action", s.handleDockerContainerAction)
	mux.HandleFunc("/api/docker/containers/logs", s.handleDockerContainerLogs)
	mux.HandleFunc("/api/docker/images", s.handleDockerImages)
	mux.HandleFunc("/api/docker/images/pull", s.handleDockerImagePull)
	mux.HandleFunc("/api/docker/compose", s.handleDockerCompose)
	mux.HandleFunc("/api/docker/stats", s.handleDockerStats)

	// Delegated coding tasks (background opencode agent)
	mux.HandleFunc("/api/code/tasks", s.handleCodeTasks)
	mux.HandleFunc("/api/code/tasks/", s.handleCodeTaskByID)
	mux.HandleFunc("/api/code/models", s.handleCodeModels)
	mux.HandleFunc("/api/code/model", s.handleCodeModel)

	// First-boot setup wizard
	mux.HandleFunc("/api/setup/status", s.handleSetupStatus)
	mux.HandleFunc("/api/setup/complete", s.handleSetupComplete)

	// OpenAI-compatible API (for Open WebUI, LangChain, etc.)
	mux.HandleFunc("/v1/models", s.handleV1Models)
	mux.HandleFunc("/v1/chat/completions", s.handleV1ChatCompletions)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	backend := s.router.Route()
	status := map[string]any{
		"backend": string(backend),
		"routing": string(s.router.mode),
	}
	if s.runtime != nil {
		if provider, model := s.runtime.Current(backend); provider != "" {
			status["provider"] = provider
			status["model"] = model
		}
		status["localModel"] = s.runtime.LocalModel()
	}
	json.NewEncoder(w).Encode(status)
}

func (s *Server) handleListModels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	type modelInfo struct {
		Name    string `json:"name"`
		Backend string `json:"backend"` // "cloud" or "local"
		Active  bool   `json:"active"`
	}

	var models []modelInfo

	// Cloud models from every provider with credentials
	if s.providerStore != nil {
		activeCloud := s.router.Route() == BackendCloud
		activeModel := s.providerStore.GetActiveModel()
		for _, p := range s.providerStore.GetProviders() {
			if !p.HasKey {
				continue
			}
			for _, m := range p.Models {
				models = append(models, modelInfo{
					Name:    m,
					Backend: "cloud",
					Active:  activeCloud && p.Active && m == activeModel,
				})
			}
		}
	}

	// SuperGrok chat models served through the opencode bridge
	if s.opencodeMgr != nil && s.opencodeMgr.Enabled() {
		if avail, err := s.opencodeMgr.AvailableModels(); err == nil {
			pinned := s.opencodeMgr.ChatModel()
			for _, id := range avail {
				if !strings.HasPrefix(id, "xai/") || strings.Contains(id, "imagine") {
					continue
				}
				models = append(models, modelInfo{
					Name:    strings.TrimPrefix(id, "xai/"),
					Backend: "supergrok",
					Active:  id == pinned,
				})
			}
		}
	}

	// Local models from Ollama
	if s.ollama != nil {
		ollamaModels, err := s.ollama.ListModels()
		if err == nil {
			activeLocal := s.router.Route() == BackendLocal
			localModel := ""
			if s.runtime != nil {
				localModel = s.runtime.LocalModel()
			}
			for _, m := range ollamaModels {
				models = append(models, modelInfo{
					Name:    m,
					Backend: "local",
					Active:  activeLocal && m == localModel,
				})
			}
		}
	}

	json.NewEncoder(w).Encode(map[string]any{"models": models})
}

func (s *Server) handleCurrentModel(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// A pinned SuperGrok chat model overrides the provider-layer routing.
	if s.opencodeMgr != nil {
		if pinned := s.opencodeMgr.ChatModel(); pinned != "" {
			json.NewEncoder(w).Encode(map[string]any{
				"model":    strings.TrimPrefix(pinned, "xai/"),
				"backend":  "supergrok",
				"provider": "xAI (SuperGrok)",
			})
			return
		}
	}

	backend := s.router.Route()
	model := ""
	providerName := ""

	if backend == BackendCloud {
		if s.providerStore != nil {
			if active := s.providerStore.GetActive(); active != nil {
				model = s.providerStore.GetActiveModel()
				providerName = active.Name
			}
		}
	} else if backend == BackendLocal {
		providerName = "Ollama"
		if s.runtime != nil {
			model = s.runtime.LocalModel()
		}
	}

	json.NewEncoder(w).Encode(map[string]any{
		"model":    model,
		"backend":  string(backend),
		"provider": providerName,
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

	// Switching to a provider-layer backend releases any SuperGrok chat pin.
	clearChatPin := func() {
		if s.opencodeMgr != nil {
			if err := s.opencodeMgr.SetChatModel(""); err != nil {
				s.logger.Warn("failed to clear SuperGrok chat pin", "error", err)
			}
		}
	}

	switch req.Backend {
	case "supergrok":
		if s.opencodeMgr == nil || !s.opencodeMgr.Enabled() {
			s.jsonError(w, "opencode integration disabled", http.StatusBadRequest)
			return
		}
		model := req.Model
		if !strings.Contains(model, "/") {
			model = "xai/" + model
		}
		if err := s.opencodeMgr.SetChatModel(model); err != nil {
			s.jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.logger.Info("chat pinned to SuperGrok model via opencode", "model", model)
	case "cloud":
		if s.providerStore == nil {
			s.jsonError(w, "no cloud credentials configured", http.StatusBadRequest)
			return
		}
		// Resolve which provider serves this model (prefer the active one).
		providerID := s.providerStore.ProviderForModel(req.Model)
		if providerID == "" {
			if active := s.providerStore.GetActive(); active != nil {
				providerID = active.ID // custom model on the active provider
			}
		}
		if providerID == "" {
			s.jsonError(w, "no cloud credentials configured", http.StatusBadRequest)
			return
		}
		if err := s.providerStore.SetActive(providerID, req.Model); err != nil {
			s.jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
		if s.runtime != nil {
			s.runtime.Rebuild()
		}
		s.router.SetCloudAvailable(true)
		s.router.mode = RouteCloudOnly
		clearChatPin()
		s.logger.Info("switched to cloud model", "provider", providerID, "model", req.Model)
	case "local":
		if s.ollama == nil {
			s.jsonError(w, "Ollama not available", http.StatusBadRequest)
			return
		}
		if s.runtime != nil {
			s.runtime.SetLocalModel(req.Model)
		}
		s.router.mode = RouteLocalOnly
		clearChatPin()
		s.logger.Info("switched to local model", "model", req.Model)
	default:
		s.jsonError(w, "backend must be 'cloud' or 'local'", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "model": req.Model, "backend": req.Backend})
}

// lockedSink serializes concurrent WriteJSON calls onto one websocket
// connection (gorilla/websocket allows only one concurrent writer). The
// chat worker goroutine and the read loop both write through it.
type lockedSink struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

func (l *lockedSink) WriteJSON(v any) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.conn.WriteJSON(v)
}

// maxQueuedUserMessages bounds the per-connection backlog of unprocessed
// user messages so the read loop never blocks (it must stay free to route
// approval_response frames to the pending-approvals map).
const maxQueuedUserMessages = 16

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("websocket upgrade failed", "error", err)
		return
	}
	defer conn.Close()

	s.logger.Info("websocket client connected")

	sink := &lockedSink{conn: conn}
	s.registerSink(sink)
	defer s.unregisterSink(sink)

	// Chat turns run on a worker goroutine so this read loop keeps draining
	// frames — otherwise an approval_required tool call would deadlock
	// waiting for an approval_response the loop never reads. Cancellation
	// unblocks any in-flight approval wait when the client disconnects.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	userMsgs := make(chan ChatMessage, maxQueuedUserMessages)
	defer close(userMsgs)

	go func() {
		for msg := range userMsgs {
			s.handleUserChatMessage(ctx, sink, msg)
		}
	}()

	for {
		var msg ChatMessage
		if err := conn.ReadJSON(&msg); err != nil {
			s.logger.Info("websocket client disconnected", "error", err)
			return
		}

		switch msg.Type {
		case "approval_response":
			// Route to the pending-approvals map — never treated as chat input.
			if !s.approvals.resolve(msg.ID, msg.Approve) {
				s.logger.Warn("approval response for unknown or expired request", "id", msg.ID)
			}
		case "user":
			select {
			case userMsgs <- msg:
			default:
				s.sendMessage(sink, ChatMessage{Type: "error", Content: "too many queued messages — wait for the current response to finish"})
			}
		}
	}
}

// handleUserChatMessage processes one user chat turn: append to the session,
// resolve the backend's provider client, and run the agentic loop.
func (s *Server) handleUserChatMessage(ctx context.Context, sink wsSink, msg ChatMessage) {
	s.logger.Info("received user message", "content", msg.Content, "session", msg.SessionID)

	sessionID := msg.SessionID
	if sessionID == "" {
		sessionID = "default"
	}

	session := s.sessions.Get(sessionID)
	session.AddMessage(providers.Message{Role: "user", Content: msg.Content})

	// Code mode: the turn runs on an interactive opencode session; progress
	// streams back via the manager's event bridge, which also emits the
	// closing "done" status when the session goes idle. A pinned SuperGrok
	// chat model (picker selection) routes every turn this way.
	if msg.Mode == "code" || (s.opencodeMgr != nil && s.opencodeMgr.ChatModel() != "") {
		if s.opencodeMgr == nil {
			s.sendMessage(sink, ChatMessage{Type: "error", Content: "code mode requires the opencode integration"})
			s.sendMessage(sink, ChatMessage{Type: "status", Content: "done"})
			return
		}
		if err := s.opencodeMgr.ChatPrompt(sessionID, msg.Content, msg.Directory, sink); err != nil {
			s.sendMessage(sink, ChatMessage{Type: "error", Content: err.Error()})
			s.sendMessage(sink, ChatMessage{Type: "status", Content: "done"})
		}
		if err := s.sessions.Save(); err != nil {
			s.logger.Error("failed to auto-save sessions", "error", err)
		}
		return
	}

	// Route to the appropriate backend and resolve its provider client.
	backend := s.router.Route()
	s.logger.Info("routing to backend", "backend", string(backend))

	client, err := s.resolveChatClient(backend)
	if err != nil {
		s.sendMessage(sink, ChatMessage{Type: "error", Content: err.Error()})
	} else {
		s.runChatLoop(ctx, sink, session, client)
	}

	// Signal to the UI that the response is complete
	s.sendMessage(sink, ChatMessage{
		Type:    "status",
		Content: "done",
	})

	// Auto-save sessions after each message exchange
	if err := s.sessions.Save(); err != nil {
		s.logger.Error("failed to auto-save sessions", "error", err)
	}
}

// resolveChatClient returns the provider client for a backend, with
// user-facing error messages when nothing is configured.
func (s *Server) resolveChatClient(backend Backend) (*providers.Client, error) {
	if s.runtime == nil {
		return nil, fmt.Errorf("no AI provider configured. Complete setup in the web UI.")
	}
	if backend == BackendLocal {
		client, err := s.runtime.LocalClient()
		if err != nil {
			return nil, fmt.Errorf("Ollama not configured. Install Ollama and set ollama.enabled=true in config.")
		}
		return client, nil
	}
	return s.runtime.CloudClient()
}

// executeTool is the permission middleware in front of every model-initiated
// tool call: trusted → execute; prohibited → error ToolResult back to the
// model without executing; approval_required → ask the user over the
// session's sink and block until approval_response or timeout (timeout =
// deny). Denials return an error result so the chat loop continues.
func (s *Server) executeTool(ctx context.Context, sink wsSink, toolName, toolID string, rawInput json.RawMessage) string {
	serverName, mcpToolName, ok := splitToolName(toolName)
	if !ok {
		return fmt.Sprintf("error: invalid tool name format: %s", toolName)
	}

	var params map[string]any
	if err := json.Unmarshal(rawInput, &params); err != nil {
		return fmt.Sprintf("error: invalid tool input: %v", err)
	}

	switch tier := s.permissions.Check(toolName, params); tier {
	case permissions.Trusted:
		return s.dispatchTool(serverName, mcpToolName, params)

	case permissions.Prohibited:
		s.logPermissionDecision(toolName, tier, "blocked", 0)
		return "error: blocked by AxiOS permission policy"

	default: // permissions.ApprovalRequired (and any unknown tier: fail closed)
		start := time.Now()
		approved, outcome := s.awaitApproval(ctx, sink, toolName, rawInput)
		s.logPermissionDecision(toolName, tier, outcome, time.Since(start))
		if !approved {
			return fmt.Sprintf("error: denied by AxiOS permission policy — %s", denialReason(outcome))
		}
		return s.dispatchTool(serverName, mcpToolName, params)
	}
}

// awaitApproval sends an approval_request over the sink and blocks on the
// pending-approvals map until the websocket read loop routes the matching
// approval_response back, the timeout elapses, or the connection's context
// is canceled. It returns the verdict and an outcome label for logging.
func (s *Server) awaitApproval(ctx context.Context, sink wsSink, toolName string, params json.RawMessage) (bool, string) {
	id := newApprovalID()
	ch := s.approvals.register(id)
	defer s.approvals.remove(id)

	if err := sink.WriteJSON(ChatMessage{
		Type:   "approval_request",
		ID:     id,
		Tool:   toolName,
		Params: params,
	}); err != nil {
		s.logger.Error("failed to send approval request", "tool", toolName, "id", id, "error", err)
		return false, "request_failed"
	}

	timer := time.NewTimer(s.approvalTimeout)
	defer timer.Stop()

	select {
	case approve := <-ch:
		if approve {
			return true, "approved"
		}
		return false, "denied"
	case <-timer.C:
		return false, "timeout"
	case <-ctx.Done():
		return false, "canceled"
	}
}

// dispatchTool performs the actual (already permitted) tool call. The
// "opencode" pseudo-server routes to the background coding agent; everything
// else goes to its MCP socket.
func (s *Server) dispatchTool(serverName, mcpToolName string, params map[string]any) string {
	if serverName == opencodeServerName {
		if s.opencodeMgr == nil {
			return "error: opencode integration is not enabled"
		}
		return s.opencodeMgr.ExecuteChatTool(mcpToolName, params)
	}

	s.logger.Info("executing MCP tool", "server", serverName, "tool", mcpToolName)

	result, err := s.executor.CallTool(serverName, mcpToolName, params)
	if err != nil {
		s.logger.Error("MCP tool call failed", "error", err)
		return fmt.Sprintf("error: %v", err)
	}

	if result.IsError {
		return fmt.Sprintf("error: %s", result.Content)
	}

	return result.Content
}

// logPermissionDecision emits one concise slog line per non-trusted tool
// call: the tool, its tier, the outcome, and how long the decision took.
func (s *Server) logPermissionDecision(tool string, tier permissions.Tier, outcome string, latency time.Duration) {
	s.logger.Info("permission decision",
		"tool", tool,
		"tier", string(tier),
		"outcome", outcome,
		"latency", latency,
	)
}

// denialReason maps an approval outcome to the explanation carried in the
// error ToolResult returned to the model.
func denialReason(outcome string) string {
	switch outcome {
	case "denied":
		return "the user denied the approval request"
	case "timeout":
		return "the approval request timed out"
	case "canceled":
		return "the session ended before the request was approved"
	default:
		return "the approval request could not be delivered"
	}
}

// splitToolName splits a runtime tool name ("serverName__toolName") into its
// MCP server and tool parts.
func splitToolName(toolName string) (serverName, mcpToolName string, ok bool) {
	if i := strings.Index(toolName, "__"); i > 0 {
		return toolName[:i], toolName[i+2:], true
	}
	return "", "", false
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

func (s *Server) sendMessage(sink wsSink, msg ChatMessage) {
	if err := sink.WriteJSON(msg); err != nil {
		s.logger.Error("websocket write failed", "error", err)
	}
}

// --- Filesystem REST endpoints ---

func expandHome(path string) string {
	if len(path) >= 1 && path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		if len(path) == 1 {
			return home
		}
		return home + path[1:]
	}
	return path
}

func (s *Server) handleFSList(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/"
	}
	path = expandHome(path)

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
	path = expandHome(path)

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

func (s *Server) handleFSWrite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Path == "" {
		s.jsonError(w, "path is required", http.StatusBadRequest)
		return
	}
	req.Path = expandHome(req.Path)

	result, err := s.mcpManager.CallTool("axios-fs", "write_file", map[string]any{
		"path":    req.Path,
		"content": req.Content,
	})
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}
	if result.IsError {
		s.jsonError(w, result.Content, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *Server) handleFSRaw(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		s.jsonError(w, "path parameter required", http.StatusBadRequest)
		return
	}
	path = expandHome(path)

	// Serve the file directly with proper MIME type
	http.ServeFile(w, r, path)
}

func (s *Server) handleFSInfo(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		s.jsonError(w, "path parameter required", http.StatusBadRequest)
		return
	}
	path = expandHome(path)

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

// --- Filesystem management endpoints (rename, copy, move, delete, mkdir) ---

func (s *Server) handleFSRename(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Path    string `json:"path"`
		NewName string `json:"new_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Path == "" || req.NewName == "" {
		s.jsonError(w, "path and new_name are required", http.StatusBadRequest)
		return
	}
	if strings.Contains(req.NewName, "/") || strings.Contains(req.NewName, string(filepath.Separator)) {
		s.jsonError(w, "new_name must not contain path separators", http.StatusBadRequest)
		return
	}

	oldPath := expandHome(req.Path)
	newPath := filepath.Join(filepath.Dir(oldPath), req.NewName)

	if err := os.Rename(oldPath, newPath); err != nil {
		s.jsonError(w, fmt.Sprintf("rename failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "new_path": newPath})
}

func (s *Server) handleFSCopy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Source      string `json:"source"`
		Destination string `json:"destination"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Source == "" || req.Destination == "" {
		s.jsonError(w, "source and destination are required", http.StatusBadRequest)
		return
	}

	src := expandHome(req.Source)
	dst := expandHome(req.Destination)

	info, err := os.Stat(src)
	if err != nil {
		s.jsonError(w, fmt.Sprintf("source not found: %v", err), http.StatusBadRequest)
		return
	}

	if info.IsDir() {
		// Use cp -r for directories
		cmd := exec.Command("cp", "-r", src, dst)
		if out, err := cmd.CombinedOutput(); err != nil {
			s.jsonError(w, fmt.Sprintf("copy failed: %v — %s", err, string(out)), http.StatusInternalServerError)
			return
		}
	} else {
		// Read and write for files
		data, err := os.ReadFile(src)
		if err != nil {
			s.jsonError(w, fmt.Sprintf("read source failed: %v", err), http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(dst, data, info.Mode()); err != nil {
			s.jsonError(w, fmt.Sprintf("write destination failed: %v", err), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *Server) handleFSMove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Source      string `json:"source"`
		Destination string `json:"destination"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Source == "" || req.Destination == "" {
		s.jsonError(w, "source and destination are required", http.StatusBadRequest)
		return
	}

	src := expandHome(req.Source)
	dst := expandHome(req.Destination)

	if err := os.Rename(src, dst); err != nil {
		s.jsonError(w, fmt.Sprintf("move failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *Server) handleFSDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Path == "" {
		s.jsonError(w, "path is required", http.StatusBadRequest)
		return
	}

	p := expandHome(req.Path)

	info, err := os.Stat(p)
	if err != nil {
		s.jsonError(w, fmt.Sprintf("path not found: %v", err), http.StatusBadRequest)
		return
	}

	if info.IsDir() {
		err = os.RemoveAll(p)
	} else {
		err = os.Remove(p)
	}
	if err != nil {
		s.jsonError(w, fmt.Sprintf("delete failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *Server) handleFSMkdir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Path == "" {
		s.jsonError(w, "path is required", http.StatusBadRequest)
		return
	}

	p := expandHome(req.Path)

	if err := os.MkdirAll(p, 0755); err != nil {
		s.jsonError(w, fmt.Sprintf("mkdir failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *Server) handleFSUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	destDir := r.URL.Query().Get("path")
	if destDir == "" {
		destDir = "/"
	}
	destDir = expandHome(destDir)

	// Verify destination is a directory
	info, err := os.Stat(destDir)
	if err != nil {
		s.jsonError(w, fmt.Sprintf("destination not found: %v", err), http.StatusBadRequest)
		return
	}
	if !info.IsDir() {
		s.jsonError(w, "destination is not a directory", http.StatusBadRequest)
		return
	}

	// 32 MB max memory
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		s.jsonError(w, fmt.Sprintf("parse multipart failed: %v", err), http.StatusBadRequest)
		return
	}

	var savedFiles []string

	for _, fileHeaders := range r.MultipartForm.File {
		for _, fh := range fileHeaders {
			src, err := fh.Open()
			if err != nil {
				s.jsonError(w, fmt.Sprintf("open uploaded file failed: %v", err), http.StatusInternalServerError)
				return
			}

			// Sanitize filename — strip any path components
			filename := filepath.Base(fh.Filename)
			destPath := filepath.Join(destDir, filename)

			dst, err := os.Create(destPath)
			if err != nil {
				src.Close()
				s.jsonError(w, fmt.Sprintf("create file failed: %v", err), http.StatusInternalServerError)
				return
			}

			if _, err := io.Copy(dst, src); err != nil {
				src.Close()
				dst.Close()
				s.jsonError(w, fmt.Sprintf("write file failed: %v", err), http.StatusInternalServerError)
				return
			}

			src.Close()
			dst.Close()
			savedFiles = append(savedFiles, filename)
		}
	}

	s.logger.Info("files uploaded", "count", len(savedFiles), "dest", destDir)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "files": savedFiles})
}

func (s *Server) handleFSBulkDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Paths []string `json:"paths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if len(req.Paths) == 0 {
		s.jsonError(w, "paths array is required", http.StatusBadRequest)
		return
	}

	var errors []string
	deleted := 0

	for _, p := range req.Paths {
		p = expandHome(p)
		info, err := os.Stat(p)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: not found", p))
			continue
		}
		if info.IsDir() {
			err = os.RemoveAll(p)
		} else {
			err = os.Remove(p)
		}
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", p, err))
		} else {
			deleted++
		}
	}

	s.logger.Info("bulk delete", "deleted", deleted, "errors", len(errors))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"ok":      len(errors) == 0,
		"deleted": deleted,
		"errors":  errors,
	})
}

// handleAIAsk handles one-shot AI queries without going through the chat WebSocket.
// POST /api/ai/ask — body: {"prompt": "...", "context": "..."}
// Returns: {"response": "..."}
func (s *Server) handleAIAsk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Prompt  string `json:"prompt"`
		Context string `json:"context"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Prompt == "" {
		s.jsonError(w, "prompt is required", http.StatusBadRequest)
		return
	}

	// Build the user message: prompt + optional context
	userContent := req.Prompt
	if req.Context != "" {
		userContent = req.Prompt + "\n\n" + req.Context
	}

	systemPrompt := "You are a helpful code assistant integrated into a file editor. Provide clear, concise, and accurate responses. When showing code, use markdown code blocks with the appropriate language tag."

	backend := s.router.Route()
	s.logger.Info("ai/ask request", "backend", string(backend), "prompt_len", len(req.Prompt), "context_len", len(req.Context))

	client, err := s.resolveChatClient(backend)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp, err := client.Complete(r.Context(), systemPrompt, []providers.Message{
		{Role: "user", Content: userContent},
	}, nil)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"response": resp.Content})
}

// axiosConfigDir returns the path to ~/.axios, creating it if needed.
func axiosConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".axios")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// handleSetupStatus returns whether the first-boot setup has been completed.
func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	dir, err := axiosConfigDir()
	if err != nil {
		s.jsonError(w, "failed to resolve config dir: "+err.Error(), http.StatusInternalServerError)
		return
	}

	markerPath := filepath.Join(dir, "setup-complete")
	completed := false
	if _, err := os.Stat(markerPath); err == nil {
		completed = true
	}

	json.NewEncoder(w).Encode(map[string]bool{"completed": completed})
}

// handleSetupComplete marks the first-boot setup as done.
func (s *Server) handleSetupComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	dir, err := axiosConfigDir()
	if err != nil {
		s.jsonError(w, "failed to resolve config dir: "+err.Error(), http.StatusInternalServerError)
		return
	}

	markerPath := filepath.Join(dir, "setup-complete")

	// Read the optional body so we can persist it as the marker content.
	body, _ := io.ReadAll(r.Body)
	if len(body) == 0 {
		body = []byte("{}")
	}

	if err := os.WriteFile(markerPath, body, 0o644); err != nil {
		s.jsonError(w, "failed to write setup marker: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
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
