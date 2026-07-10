package axiosd

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/axios-os/axios/pkg/opencode"
)

// codeTaskRequest is the POST /api/code/tasks body.
type codeTaskRequest struct {
	Prompt    string `json:"prompt"`
	Directory string `json:"directory,omitempty"`
	Model     string `json:"model,omitempty"` // "provider/model" (optional)
}

// codeTaskDetail is the GET /api/code/tasks/{id} response: the task plus the
// session's file changes once it finished.
type codeTaskDetail struct {
	OpencodeTask
	Diff []opencode.FileDiff `json:"diff,omitempty"`
}

// opencodeEnabled guards the /api/code endpoints; when the integration is off
// they answer 503 instead of exposing half-working state.
func (s *Server) opencodeEnabled(w http.ResponseWriter) bool {
	if s.opencodeMgr == nil || !s.opencodeMgr.Enabled() {
		s.jsonError(w, "opencode integration disabled", http.StatusServiceUnavailable)
		return false
	}
	return true
}

// handleCodeTasks serves /api/code/tasks: GET lists delegated tasks, POST
// creates one.
func (s *Server) handleCodeTasks(w http.ResponseWriter, r *http.Request) {
	if !s.opencodeEnabled(w) {
		return
	}

	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"tasks": s.opencodeMgr.Tasks()})

	case http.MethodPost:
		var req codeTaskRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Prompt) == "" {
			s.jsonError(w, "prompt is required", http.StatusBadRequest)
			return
		}
		task, err := s.opencodeMgr.Delegate(req.Prompt, req.Directory, parseModelRef(req.Model))
		if err != nil {
			s.jsonError(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"task_id": task.ID, "status": task.Status})

	default:
		s.jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleCodeTaskByID serves /api/code/tasks/{id}: GET returns status, result
// and (for finished tasks) the diff; DELETE aborts a running task.
func (s *Server) handleCodeTaskByID(w http.ResponseWriter, r *http.Request) {
	if !s.opencodeEnabled(w) {
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/code/tasks/")
	if id == "" || strings.Contains(id, "/") {
		s.jsonError(w, "invalid task id", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		task, ok := s.opencodeMgr.Task(id)
		if !ok {
			s.jsonError(w, "unknown task", http.StatusNotFound)
			return
		}
		detail := codeTaskDetail{OpencodeTask: task}
		if task.Status == TaskDone {
			if diff, err := s.opencodeMgr.TaskDiff(id); err == nil {
				detail.Diff = diff
			} else {
				s.logger.Warn("failed to fetch opencode task diff", "task", id, "error", err)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(detail)

	case http.MethodDelete:
		if err := s.opencodeMgr.AbortTask(id); err != nil {
			s.jsonError(w, err.Error(), http.StatusConflict)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"task_id": id, "status": TaskAborted})

	default:
		s.jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// parseModelRef splits an optional "provider/model" string into opencode's
// addressing scheme; empty or malformed values fall back to opencode's default.
func parseModelRef(s string) *opencode.ModelRef {
	provider, model, ok := strings.Cut(s, "/")
	if !ok || provider == "" || model == "" {
		return nil
	}
	return &opencode.ModelRef{ProviderID: provider, ModelID: model}
}
