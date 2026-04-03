package claused

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
)

// handleSessionsList returns all sessions with metadata.
// GET /api/chat/sessions
func (s *Server) handleSessionsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"sessions": s.sessions.List(),
	})
}

// handleSessionsCreate creates a new session.
// POST /api/chat/sessions
func (s *Server) handleSessionsCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	id := generateID()
	s.sessions.Create(id)
	if err := s.sessions.Save(); err != nil {
		s.logger.Error("failed to save sessions", "error", err)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": id})
}

// handleSessionsDelete deletes a session by ID.
// DELETE /api/chat/sessions?id=X
func (s *Server) handleSessionsDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		s.jsonError(w, "DELETE required", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		s.jsonError(w, "missing id parameter", http.StatusBadRequest)
		return
	}
	if !s.sessions.Delete(id) {
		s.jsonError(w, fmt.Sprintf("session %q not found", id), http.StatusNotFound)
		return
	}
	if err := s.sessions.Save(); err != nil {
		s.logger.Error("failed to save sessions after delete", "error", err)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// handleSessionsMessages returns all messages for a session.
// GET /api/chat/sessions/messages?id=X
func (s *Server) handleSessionsMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		s.jsonError(w, "missing id parameter", http.StatusBadRequest)
		return
	}

	// Check if the session exists by listing sessions
	found := false
	for _, meta := range s.sessions.List() {
		if meta.ID == id {
			found = true
			break
		}
	}
	if !found {
		s.jsonError(w, fmt.Sprintf("session %q not found", id), http.StatusNotFound)
		return
	}

	session := s.sessions.Get(id)
	messages := session.GetMessages()

	// Convert messages to a frontend-friendly format
	type msgOut struct {
		Role     string `json:"role"`
		Content  any    `json:"content"`
		ToolName string `json:"toolName,omitempty"`
	}

	out := make([]msgOut, 0, len(messages))
	for _, m := range messages {
		out = append(out, msgOut{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"messages": out,
	})
}

// handleSessionsRouter multiplexes GET/POST/DELETE on /api/chat/sessions.
func (s *Server) handleSessionsRouter(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleSessionsList(w, r)
	case http.MethodPost:
		s.handleSessionsCreate(w, r)
	case http.MethodDelete:
		s.handleSessionsDelete(w, r)
	default:
		s.jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// generateID creates a random UUID v4 string.
func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

