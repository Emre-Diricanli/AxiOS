package axiosd

// The /api/obsidian endpoints expose the user's Obsidian vault (a plain
// directory of Markdown notes) to the web UI. Path containment and note
// parsing live in internal/obsidianctl; the manager re-reads the obsidian.json
// state file on every call, so a vault switch takes effect immediately in the
// daemon and the axios-obsidian MCP server alike. Full request/response
// contract (for the frontend): docs/obsidian-api.md.

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"strconv"
	"strings"

	"github.com/axios-os/axios/internal/obsidianctl"
)

// SetObsidian wires the Obsidian vault manager into the server.
func (s *Server) SetObsidian(m *obsidianctl.Manager) {
	s.obsidian = m
}

// obsidianVault resolves the active vault for a request, answering the
// standard 409 itself when no usable vault is configured. The bool reports
// whether the caller may proceed.
func (s *Server) obsidianVault(w http.ResponseWriter) (*obsidianctl.Vault, bool) {
	if s.obsidian == nil {
		s.jsonError(w, "no vault configured", http.StatusConflict)
		return nil, false
	}
	v, err := s.obsidian.Vault()
	if err != nil {
		msg := "no vault configured"
		if !errors.Is(err, obsidianctl.ErrNotConfigured) {
			msg = "vault unavailable: " + err.Error()
		}
		s.jsonError(w, msg, http.StatusConflict)
		return nil, false
	}
	return v, true
}

// obsidianError maps vault-engine errors onto HTTP statuses: invalid or
// hidden paths → 400, missing notes → 404, existing notes (overwrite=false)
// → 409, everything else → 500.
func (s *Server) obsidianError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, obsidianctl.ErrInvalidPath):
		status = http.StatusBadRequest
	case errors.Is(err, fs.ErrNotExist):
		status = http.StatusNotFound
	case errors.Is(err, fs.ErrExist):
		status = http.StatusConflict
	}
	s.jsonError(w, err.Error(), status)
}

// obsidianStatusPayload builds the GET /api/obsidian/status body, which
// PUT /api/obsidian/vault returns as well. Unconfigured is a normal state:
// {"configured": false} with no stats.
func (s *Server) obsidianStatusPayload() map[string]any {
	if s.obsidian == nil {
		return map[string]any{"configured": false}
	}
	path, err := s.obsidian.Path()
	if errors.Is(err, obsidianctl.ErrNotConfigured) {
		return map[string]any{"configured": false}
	}
	if err != nil {
		return map[string]any{"configured": false, "error": err.Error()}
	}

	payload := map[string]any{"configured": true, "vault_path": path}
	vault, err := obsidianctl.Open(path)
	if err != nil {
		// Configured but unusable (e.g. the directory was moved): keep the
		// path so the UI can show what is broken, omit the stats.
		payload["looks_like_vault"] = false
		payload["error"] = err.Error()
		return payload
	}
	payload["looks_like_vault"] = vault.LooksLikeVault()

	info, err := vault.Info()
	if err != nil {
		payload["error"] = err.Error()
		return payload
	}
	payload["name"] = info.Name
	payload["notes"] = info.Notes
	payload["folders"] = info.Folders
	payload["size_bytes"] = info.TotalSizeBytes
	return payload
}

// handleObsidianStatus serves GET /api/obsidian/status: whether a vault is
// configured plus its stats. Never 409s — the UI gates on this endpoint.
func (s *Server) handleObsidianStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.obsidianStatusPayload())
}

// handleObsidianVault serves PUT /api/obsidian/vault {"path":"/abs/dir"}:
// validate the directory, persist it to $AXIOS_DATA_DIR/obsidian.json, and
// return the refreshed status payload.
func (s *Server) handleObsidianVault(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		s.jsonError(w, "PUT required", http.StatusMethodNotAllowed)
		return
	}
	if s.obsidian == nil {
		s.jsonError(w, "obsidian integration not available", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Path) == "" {
		s.jsonError(w, "path is required", http.StatusBadRequest)
		return
	}

	if _, err := s.obsidian.SetVault(req.Path); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, obsidianctl.ErrInvalidPath) || errors.Is(err, fs.ErrNotExist) {
			status = http.StatusBadRequest
		}
		s.jsonError(w, err.Error(), status)
		return
	}
	s.logger.Info("obsidian vault configured", "path", req.Path)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.obsidianStatusPayload())
}

// handleObsidianNotes serves GET /api/obsidian/notes?folder=&recursive=true:
// the notes and folders under a vault-relative folder ("" = the root).
func (s *Server) handleObsidianNotes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	vault, ok := s.obsidianVault(w)
	if !ok {
		return
	}

	entries, err := vault.ListNotes(r.URL.Query().Get("folder"), r.URL.Query().Get("recursive") == "true")
	if err != nil {
		s.obsidianError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"entries": entries})
}

// handleObsidianNote serves /api/obsidian/note: GET reads a note (?path=),
// PUT writes one ({"path","content","overwrite"}, overwrite defaults to
// true for the UI's editor), DELETE removes one (?path=) with 204.
func (s *Server) handleObsidianNote(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		vault, ok := s.obsidianVault(w)
		if !ok {
			return
		}
		path := r.URL.Query().Get("path")
		if path == "" {
			s.jsonError(w, "path parameter required", http.StatusBadRequest)
			return
		}
		note, err := vault.ReadNote(path)
		if err != nil {
			s.obsidianError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(note)

	case http.MethodPut:
		vault, ok := s.obsidianVault(w)
		if !ok {
			return
		}
		var req struct {
			Path      string `json:"path"`
			Content   string `json:"content"`
			Overwrite *bool  `json:"overwrite"` // nil → true: the UI editor saves over existing notes
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.jsonError(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if req.Path == "" {
			s.jsonError(w, "path is required", http.StatusBadRequest)
			return
		}
		overwrite := req.Overwrite == nil || *req.Overwrite
		if err := vault.WriteNote(req.Path, req.Content, overwrite); err != nil {
			s.obsidianError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true})

	case http.MethodDelete:
		vault, ok := s.obsidianVault(w)
		if !ok {
			return
		}
		path := r.URL.Query().Get("path")
		if path == "" {
			s.jsonError(w, "path parameter required", http.StatusBadRequest)
			return
		}
		if err := vault.DeleteNote(path); err != nil {
			s.obsidianError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		s.jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleObsidianSearch serves GET /api/obsidian/search?q=&tag=&limit=:
// case-insensitive substring search over note names and content, optionally
// restricted to notes carrying a tag.
func (s *Server) handleObsidianSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	vault, ok := s.obsidianVault(w)
	if !ok {
		return
	}

	q := r.URL.Query().Get("q")
	tag := r.URL.Query().Get("tag")
	if strings.TrimSpace(q) == "" && strings.TrimSpace(tag) == "" {
		s.jsonError(w, "q or tag parameter required", http.StatusBadRequest)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	hits, err := vault.Search(q, tag, limit)
	if err != nil {
		s.obsidianError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"hits": hits})
}
