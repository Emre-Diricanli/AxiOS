package axiosd

import (
	"encoding/json"
	"net/http"
)

// xaiOAuth lazily creates the flow manager. Connecting restarts the managed
// opencode server (when idle) so it picks up the new credentials immediately.
func (s *Server) xaiOAuth() *XAIOAuth {
	s.xaiOAuthOnce.Do(func() {
		s.xaiOAuthFlow = NewXAIOAuth(nil, "", s.logger)
		s.xaiOAuthFlow.onConnected = func() {
			if s.opencodeMgr != nil {
				s.opencodeMgr.RestartIfIdle()
			}
		}
	})
	return s.xaiOAuthFlow
}

// handleXAIOAuthStart serves POST /api/providers/xai/oauth/start: begins (or
// returns the pending) device-code flow. The response carries the
// verification URL and user code for the UI to display.
func (s *Server) handleXAIOAuthStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status, err := s.xaiOAuth().Start()
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
	}
	json.NewEncoder(w).Encode(status)
}

// handleXAIOAuthStatus serves GET /api/providers/xai/oauth/status.
func (s *Server) handleXAIOAuthStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.xaiOAuth().Status())
}
