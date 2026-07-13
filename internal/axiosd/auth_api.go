package axiosd

// The three /api/auth endpoints. Full request/response contract (for the
// frontend and API clients): docs/auth-api.md.

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// handleAuthLogin exchanges the admin token for an HMAC-signed session
// cookie.
//
//	POST /api/auth/login {"token":"axsk_..."}
//	  200 {"ok":true}  + Set-Cookie: axios_session=...
//	  400 {"error":"invalid JSON"}
//	  401 {"error":"invalid token"}
//	  429 {"error":"too many attempts"}   (5 failures/minute per remote IP)
func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	m := s.auth
	if m == nil || m.store == nil {
		// No auth state wired — nothing to verify a token against.
		s.jsonError(w, "invalid token", http.StatusUnauthorized)
		return
	}

	ip := remoteIP(r)
	if m.limiter.tooManyFailures(ip) {
		s.jsonError(w, "too many attempts", http.StatusTooManyRequests)
		return
	}

	var req struct {
		Token string `json:"token"`
	}
	// Tokens are ~50 bytes; cap the unauthenticated body far above that.
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if !m.store.VerifyToken(req.Token) {
		m.limiter.recordFailure(ip)
		s.jsonError(w, "invalid token", http.StatusUnauthorized)
		return
	}
	m.limiter.clear(ip)

	value, err := m.store.IssueSession(m.sessionTTL)
	if err != nil {
		s.logger.Error("failed to issue session", "error", err)
		s.jsonError(w, "failed to issue session", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, m.sessionCookie(value, r))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

// handleAuthLogout expires the session cookie.
//
//	POST /api/auth/logout → 204 (no body)
//
// Sessions are stateless (HMAC-signed), so logout clears the browser's copy;
// revoking every outstanding session is `axiosd --reset-auth`.
func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	http.SetCookie(w, expiredSessionCookie(r))
	w.WriteHeader(http.StatusNoContent)
}

// handleAuthStatus is the public probe the SPA gates on.
//
//	GET /api/auth/status → 200 {"auth_required":bool,"authenticated":bool}
//
// Always public and never leaks more: auth_required reflects config,
// authenticated whether this request carried a valid session or bearer.
func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	authRequired := s.auth != nil && s.auth.enabled
	authenticated := s.auth != nil && s.auth.authenticated(r)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{
		"auth_required": authRequired,
		"authenticated": authenticated,
	})
}

// sessionCookie builds the login session cookie: HttpOnly, SameSite=Lax,
// Path=/, Secure when the request arrived over TLS (directly or behind a
// TLS-terminating proxy such as `tailscale serve`).
func (m *AuthManager) sessionCookie(value string, r *http.Request) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   int(m.sessionTTL / time.Second),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestIsSecure(r),
	}
}

// expiredSessionCookie clears the session cookie (Max-Age=-1).
func expiredSessionCookie(r *http.Request) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestIsSecure(r),
	}
}

// requestIsSecure reports whether the client reached us over TLS, directly
// or via a TLS-terminating reverse proxy (X-Forwarded-Proto).
func requestIsSecure(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
