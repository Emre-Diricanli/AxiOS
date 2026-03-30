package claused

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gorilla/websocket"
)

// Server is the main HTTP/WebSocket server for claused.
type Server struct {
	anthropic *AnthropicClient
	router    *Router
	sessions  *SessionStore
	upgrader  websocket.Upgrader
	logger    *slog.Logger
	system    string
}

// NewServer creates a new claused HTTP server.
func NewServer(anthropic *AnthropicClient, router *Router, logger *slog.Logger) *Server {
	return &Server{
		anthropic: anthropic,
		router:    router,
		sessions:  NewSessionStore(),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		logger: logger,
		system: `You are the AI assistant running on AxiOS, an AI-native operating system. You have direct access to the system hardware and software through MCP tools. You help the user manage their system, run commands, manage files, and accomplish creative work.

Be concise and direct. When the user asks you to do something on the system, do it — don't just explain how.`,
	}
}

// ChatMessage is the WebSocket message format between the web UI and claused.
type ChatMessage struct {
	Type      string `json:"type"`      // "user", "assistant", "error", "status"
	Content   string `json:"content"`
	SessionID string `json:"sessionId"`
	Model     string `json:"model,omitempty"` // Which backend handled this
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
		"backend": string(s.router.Route()),
		"routing": string(s.router.mode),
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

		session := s.sessions.Get(msg.SessionID)
		session.AddMessage(Message{Role: "user", Content: msg.Content})

		// Route to appropriate backend
		backend := s.router.Route()

		switch backend {
		case BackendCloud:
			s.handleCloudMessage(conn, session, backend)
		case BackendLocal:
			// TODO: implement Ollama backend
			s.sendMessage(conn, ChatMessage{
				Type:    "error",
				Content: "Local model backend not yet implemented",
			})
		}
	}
}

func (s *Server) handleCloudMessage(conn *websocket.Conn, session *Session, backend Backend) {
	messages := session.GetMessages()

	stream, err := s.anthropic.StreamMessage(s.system, messages, nil)
	if err != nil {
		s.logger.Error("anthropic stream failed", "error", err)
		s.sendMessage(conn, ChatMessage{
			Type:    "error",
			Content: fmt.Sprintf("Failed to connect to Claude: %v", err),
		})

		// If cloud fails, try local fallback
		if s.router.mode == RouteAuto {
			s.router.SetCloudAvailable(false)
		}
		return
	}
	defer stream.Close()

	var fullResponse string
	err = ParseSSEStream(stream, func(eventType string, data []byte) {
		switch eventType {
		case "content_block_delta":
			var delta struct {
				Delta struct {
					Text string `json:"text"`
				} `json:"delta"`
			}
			if err := json.Unmarshal(data, &delta); err == nil && delta.Delta.Text != "" {
				fullResponse += delta.Delta.Text
				s.sendMessage(conn, ChatMessage{
					Type:    "assistant",
					Content: delta.Delta.Text,
					Model:   "claude",
				})
			}
		case "message_stop":
			session.AddMessage(Message{Role: "assistant", Content: fullResponse})
		}
	})
	if err != nil {
		s.logger.Error("stream parse error", "error", err)
	}
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
