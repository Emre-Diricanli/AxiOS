package axiosd

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// --- helpers -----------------------------------------------------------------

// newTestAuthStore creates fresh auth state in a temp dir and returns the
// store plus the one-time plaintext token.
func newTestAuthStore(t *testing.T) (*AuthStore, string) {
	t.Helper()
	store, token, err := LoadOrCreateAuthState(filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatalf("LoadOrCreateAuthState: %v", err)
	}
	if token == "" {
		t.Fatal("expected a plaintext token at generation")
	}
	return store, token
}

// authStack is the middleware-wrapped mux with stand-in protected routes and
// the real /api/auth handlers, assembled the same way ListenAndServe does.
type authStack struct {
	store   *AuthStore
	token   string
	manager *AuthManager
	handler http.Handler
}

func newAuthStack(t *testing.T, enabled bool, origins []string) *authStack {
	t.Helper()
	store, token := newTestAuthStore(t)
	m := NewAuthManager(store, AuthOptions{
		Enabled:        enabled,
		SessionTTL:     time.Hour,
		AllowedOrigins: origins,
	}, testLogger())

	srv := &Server{logger: testLogger()}
	srv.SetAuth(m)

	ok := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }
	mux := http.NewServeMux()
	mux.HandleFunc("/", ok)         // SPA shell: index.html + static assets
	mux.HandleFunc("/api/ping", ok) // stand-in for any admin API route
	mux.HandleFunc("/ws", ok)
	mux.HandleFunc("/ws/terminal", ok)
	mux.HandleFunc("/v1/models", ok)
	mux.HandleFunc("/api/auth/login", srv.handleAuthLogin)
	mux.HandleFunc("/api/auth/logout", srv.handleAuthLogout)
	mux.HandleFunc("/api/auth/status", srv.handleAuthStatus)

	return &authStack{store: store, token: token, manager: m, handler: m.Middleware(mux)}
}

func (st *authStack) do(req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	st.handler.ServeHTTP(rec, req)
	return rec
}

// login exchanges the admin token for a session cookie.
func (st *authStack) login(t *testing.T) *http.Cookie {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login",
		strings.NewReader(fmt.Sprintf(`{"token":%q}`, st.token)))
	rec := st.do(req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, body %s", rec.Code, rec.Body)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName {
			return c
		}
	}
	t.Fatal("login response did not set the session cookie")
	return nil
}

// errorBody decodes the {"error": "..."} JSON error shape.
func errorBody(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response body %q is not an error JSON: %v", rec.Body, err)
	}
	return body.Error
}

// signSession crafts a session value with an arbitrary epoch/expiry (for
// expired- and wrong-epoch-session tests).
func signSession(t *testing.T, store *AuthStore, epoch, expiresUnix int64) string {
	t.Helper()
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.signSessionLocked(epoch, expiresUnix, []byte("test-nonce-0123!"))
}

// tamperMAC flips a character inside the HMAC segment of a session value.
func tamperMAC(t *testing.T, value string) string {
	t.Helper()
	parts := strings.Split(value, ".")
	if len(parts) != 5 {
		t.Fatalf("session value %q does not have 5 parts", value)
	}
	mac := []byte(parts[4])
	if mac[0] == 'A' {
		mac[0] = 'B'
	} else {
		mac[0] = 'A'
	}
	parts[4] = string(mac)
	return strings.Join(parts, ".")
}

// --- store tests -------------------------------------------------------------

func TestAuthStateCreate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	store, token, err := LoadOrCreateAuthState(path)
	if err != nil {
		t.Fatalf("LoadOrCreateAuthState: %v", err)
	}

	if !strings.HasPrefix(token, authTokenPrefix) {
		t.Errorf("token %q missing %q prefix", token, authTokenPrefix)
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(token, authTokenPrefix))
	if err != nil || len(raw) != authKeySize {
		t.Errorf("token payload = %d bytes (err %v), want %d bytes of base64url", len(raw), err, authKeySize)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat auth.json: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("auth.json mode = %o, want 0600", perm)
	}
	if !store.VerifyToken(token) {
		t.Error("freshly generated token must verify")
	}
	if strings.Contains(readFileString(t, path), strings.TrimPrefix(token, authTokenPrefix)) {
		t.Error("auth.json must not contain the plaintext token")
	}

	// Reload: the plaintext token is available exactly once (at generation).
	store2, token2, err := LoadOrCreateAuthState(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if token2 != "" {
		t.Errorf("reload returned a token %q, want empty (printed once only)", token2)
	}
	if !store2.VerifyToken(token) {
		t.Error("reloaded store must verify the original token")
	}
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestAuthStateResetBumpsEpoch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	store, token, err := LoadOrCreateAuthState(path)
	if err != nil {
		t.Fatalf("LoadOrCreateAuthState: %v", err)
	}
	if store.epoch != 1 {
		t.Fatalf("initial epoch = %d, want 1", store.epoch)
	}
	session, err := store.IssueSession(time.Hour)
	if err != nil {
		t.Fatalf("IssueSession: %v", err)
	}
	if !store.VerifySession(session) {
		t.Fatal("fresh session must verify")
	}

	newToken, err := store.Reset()
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if newToken == token {
		t.Error("Reset must generate a different token")
	}
	if store.epoch != 2 {
		t.Errorf("epoch after Reset = %d, want 2", store.epoch)
	}
	if store.VerifyToken(token) {
		t.Error("old token must be invalid after Reset")
	}
	if !store.VerifyToken(newToken) {
		t.Error("new token must verify after Reset")
	}
	if store.VerifySession(session) {
		t.Error("epoch bump must invalidate outstanding sessions")
	}

	// The reset persisted: a reload sees the new epoch and token hash.
	reloaded, printable, err := LoadOrCreateAuthState(path)
	if err != nil {
		t.Fatalf("reload after Reset: %v", err)
	}
	if printable != "" {
		t.Error("reload after Reset must not re-print a token")
	}
	if reloaded.epoch != 2 || !reloaded.VerifyToken(newToken) {
		t.Errorf("reloaded epoch = %d (token valid %v), want 2/true", reloaded.epoch, reloaded.VerifyToken(newToken))
	}

	// ResetAuthState (the --reset-auth entry point) bumps again on disk.
	third, err := ResetAuthState(path)
	if err != nil {
		t.Fatalf("ResetAuthState: %v", err)
	}
	final, _, err := LoadOrCreateAuthState(path)
	if err != nil {
		t.Fatalf("reload after ResetAuthState: %v", err)
	}
	if final.epoch != 3 || !final.VerifyToken(third) {
		t.Errorf("epoch after ResetAuthState = %d (token valid %v), want 3/true", final.epoch, final.VerifyToken(third))
	}
}

func TestAuthStateCorruptFile(t *testing.T) {
	validHash := strings.Repeat("ab", 32)
	validKey := base64.RawURLEncoding.EncodeToString(make([]byte, authKeySize))

	tests := []struct {
		name    string
		content string
	}{
		{"not json", "definitely not json"},
		{"empty object", "{}"},
		{"wrong version", fmt.Sprintf(`{"version":9,"token_sha256":%q,"session_key":%q,"epoch":1}`, validHash, validKey)},
		{"bad token hash", fmt.Sprintf(`{"version":1,"token_sha256":"zz","session_key":%q,"epoch":1}`, validKey)},
		{"short session key", fmt.Sprintf(`{"version":1,"token_sha256":%q,"session_key":"AAAA","epoch":1}`, validHash)},
		{"epoch out of range", fmt.Sprintf(`{"version":1,"token_sha256":%q,"session_key":%q,"epoch":0}`, validHash, validKey)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "auth.json")
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatalf("write corrupt file: %v", err)
			}
			if _, _, err := LoadOrCreateAuthState(path); err == nil {
				t.Error("LoadOrCreateAuthState accepted a corrupt state file, want error")
			}
		})
	}
}

func TestVerifySessionRejectsBadValues(t *testing.T) {
	store, _ := newTestAuthStore(t)
	valid, err := store.IssueSession(time.Hour)
	if err != nil {
		t.Fatalf("IssueSession: %v", err)
	}
	if !store.VerifySession(valid) {
		t.Fatal("fresh session must verify")
	}

	tests := []struct {
		name  string
		value string
	}{
		{"empty", ""},
		{"wrong part count", "v1.1.12345"},
		{"wrong version", "v0" + strings.TrimPrefix(valid, "v1")},
		{"tampered hmac", tamperMAC(t, valid)},
		{"mac not base64", strings.Join(strings.Split(valid, ".")[:4], ".") + ".!!!"},
		{"expired", signSession(t, store, store.epoch, time.Now().Add(-time.Minute).Unix())},
		{"future epoch", signSession(t, store, store.epoch+1, time.Now().Add(time.Hour).Unix())},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if store.VerifySession(tt.value) {
				t.Errorf("VerifySession(%q) = true, want false", tt.value)
			}
		})
	}
}

// TestVerifyTokenCorrectness pins the constant-time comparison's observable
// behavior (equal → true, everything else → false); the timing property
// itself comes from crypto/subtle and is not testable here.
func TestVerifyTokenCorrectness(t *testing.T) {
	store, token := newTestAuthStore(t)

	tests := []struct {
		name      string
		presented string
		want      bool
	}{
		{"exact token", token, true},
		{"empty", "", false},
		{"truncated", token[:len(token)-1], false},
		{"appended", token + "x", false},
		{"same length different bytes", authTokenPrefix + strings.Repeat("A", len(token)-len(authTokenPrefix)), false},
		{"missing prefix", strings.TrimPrefix(token, authTokenPrefix), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := store.VerifyToken(tt.presented); got != tt.want {
				t.Errorf("VerifyToken = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- login flow --------------------------------------------------------------

func TestLoginFlow(t *testing.T) {
	st := newAuthStack(t, true, nil)

	// Before login, API access is rejected.
	if rec := st.do(httptest.NewRequest(http.MethodGet, "/api/ping", nil)); rec.Code != http.StatusUnauthorized {
		t.Fatalf("pre-login /api/ping status = %d, want 401", rec.Code)
	}

	cookie := st.login(t)
	if !cookie.HttpOnly {
		t.Error("session cookie must be HttpOnly")
	}
	if cookie.Path != "/" {
		t.Errorf("session cookie Path = %q, want /", cookie.Path)
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("session cookie SameSite = %v, want Lax", cookie.SameSite)
	}
	if cookie.Secure {
		t.Error("plain-HTTP login must not set the Secure flag")
	}

	// The cookie authenticates subsequent API requests.
	req := httptest.NewRequest(http.MethodGet, "/api/ping", nil)
	req.AddCookie(cookie)
	if rec := st.do(req); rec.Code != http.StatusOK {
		t.Errorf("authed /api/ping status = %d, want 200", rec.Code)
	}

	// Wrong token → 401 {"error":"invalid token"}.
	rec := st.do(httptest.NewRequest(http.MethodPost, "/api/auth/login",
		strings.NewReader(`{"token":"axsk_definitely-wrong"}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad login status = %d, want 401", rec.Code)
	}
	if got := errorBody(t, rec); got != "invalid token" {
		t.Errorf("bad login error = %q, want %q", got, "invalid token")
	}

	// Malformed JSON → 400, and GET → 405.
	if rec := st.do(httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader("{"))); rec.Code != http.StatusBadRequest {
		t.Errorf("malformed login status = %d, want 400", rec.Code)
	}
	if rec := st.do(httptest.NewRequest(http.MethodGet, "/api/auth/login", nil)); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET login status = %d, want 405", rec.Code)
	}
}

func TestLoginSecureCookieBehindTLSProxy(t *testing.T) {
	st := newAuthStack(t, true, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login",
		strings.NewReader(fmt.Sprintf(`{"token":%q}`, st.token)))
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := st.do(req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d", rec.Code)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName {
			if !c.Secure {
				t.Error("session cookie behind a TLS-terminating proxy must be Secure")
			}
			return
		}
	}
	t.Fatal("no session cookie set")
}

func TestLoginRateLimit(t *testing.T) {
	st := newAuthStack(t, true, nil)

	badLogin := func(remoteAddr string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login",
			strings.NewReader(`{"token":"axsk_wrong"}`))
		req.RemoteAddr = remoteAddr
		return st.do(req)
	}

	for i := 0; i < loginFailureLimit; i++ {
		if rec := badLogin("10.0.0.1:4000"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("failed attempt %d status = %d, want 401", i+1, rec.Code)
		}
	}

	// The next attempt from the same IP is throttled — even with the RIGHT
	// token, and regardless of source port.
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login",
		strings.NewReader(fmt.Sprintf(`{"token":%q}`, st.token)))
	req.RemoteAddr = "10.0.0.1:9999"
	rec := st.do(req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("throttled login status = %d, want 429", rec.Code)
	}
	if got := errorBody(t, rec); got != "too many attempts" {
		t.Errorf("throttled login error = %q, want %q", got, "too many attempts")
	}

	// A distinct IP is unaffected.
	req = httptest.NewRequest(http.MethodPost, "/api/auth/login",
		strings.NewReader(fmt.Sprintf(`{"token":%q}`, st.token)))
	req.RemoteAddr = "10.9.9.9:4000"
	if rec := st.do(req); rec.Code != http.StatusOK {
		t.Errorf("distinct-IP login status = %d, want 200", rec.Code)
	}
}

func TestLoginRateLimitWindowSlides(t *testing.T) {
	l := newLoginLimiter()
	now := time.Unix(1_000_000, 0)
	l.now = func() time.Time { return now }

	for i := 0; i < loginFailureLimit; i++ {
		l.recordFailure("192.0.2.7")
	}
	if !l.tooManyFailures("192.0.2.7") {
		t.Fatal("limit must engage after 5 failures")
	}

	now = now.Add(loginFailureWindow + time.Second)
	if l.tooManyFailures("192.0.2.7") {
		t.Fatal("window must slide: old failures expire after a minute")
	}
	// The periodic sweep also dropped the stale map entry entirely.
	if len(l.failures) != 0 {
		t.Errorf("failures map = %v, want swept empty", l.failures)
	}
}

// --- middleware table --------------------------------------------------------

func TestMiddlewareTable(t *testing.T) {
	st := newAuthStack(t, true, nil)
	cookie := st.login(t)
	expired := signSession(t, st.store, st.store.epoch, time.Now().Add(-time.Minute).Unix())
	tampered := tamperMAC(t, cookie.Value)

	withCookie := func(value string) func(*http.Request) {
		return func(r *http.Request) {
			r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: value})
		}
	}
	withBearer := func(value string) func(*http.Request) {
		return func(r *http.Request) { r.Header.Set("Authorization", value) }
	}

	tests := []struct {
		name    string
		method  string
		path    string
		prepare func(*http.Request)
		want    int
	}{
		{"static index public", http.MethodGet, "/", nil, http.StatusOK},
		{"static asset public", http.MethodGet, "/assets/app.js", nil, http.StatusOK},
		{"auth status public", http.MethodGet, "/api/auth/status", nil, http.StatusOK},
		{"api without credentials", http.MethodGet, "/api/ping", nil, http.StatusUnauthorized},
		{"api with session cookie", http.MethodGet, "/api/ping", withCookie(cookie.Value), http.StatusOK},
		{"api with bearer token", http.MethodGet, "/api/ping", withBearer("Bearer " + st.token), http.StatusOK},
		{"bearer scheme is case-insensitive", http.MethodGet, "/api/ping", withBearer("bearer " + st.token), http.StatusOK},
		{"api with wrong bearer", http.MethodGet, "/api/ping", withBearer("Bearer axsk_bogus"), http.StatusUnauthorized},
		{"chat ws without credentials", http.MethodGet, "/ws", nil, http.StatusUnauthorized},
		{"terminal ws without credentials", http.MethodGet, "/ws/terminal", nil, http.StatusUnauthorized},
		{"openai compat without credentials", http.MethodGet, "/v1/models", nil, http.StatusUnauthorized},
		{"openai compat with bearer", http.MethodGet, "/v1/models", withBearer("Bearer " + st.token), http.StatusOK},
		{"unregistered api path fails closed", http.MethodGet, "/api/route/added/tomorrow", nil, http.StatusUnauthorized},
		{"expired session", http.MethodGet, "/api/ping", withCookie(expired), http.StatusUnauthorized},
		{"tampered session hmac", http.MethodGet, "/api/ping", withCookie(tampered), http.StatusUnauthorized},
		{"path traversal cannot dodge the api prefix", http.MethodGet, "/static/../api/ping", nil, http.StatusUnauthorized},
		{"logout requires auth", http.MethodPost, "/api/auth/logout", nil, http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.prepare != nil {
				tt.prepare(req)
			}
			rec := st.do(req)
			if rec.Code != tt.want {
				t.Fatalf("%s %s status = %d, want %d (body %s)", tt.method, tt.path, rec.Code, tt.want, rec.Body)
			}
			if tt.want == http.StatusUnauthorized {
				if got := errorBody(t, rec); got != "unauthorized" {
					t.Errorf("401 body error = %q, want %q", got, "unauthorized")
				}
			}
		})
	}
}

func TestMiddlewareEpochBumpInvalidatesSessions(t *testing.T) {
	st := newAuthStack(t, true, nil)
	cookie := st.login(t)

	req := httptest.NewRequest(http.MethodGet, "/api/ping", nil)
	req.AddCookie(cookie)
	if rec := st.do(req); rec.Code != http.StatusOK {
		t.Fatalf("pre-reset status = %d, want 200", rec.Code)
	}

	if _, err := st.store.Reset(); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/ping", nil)
	req.AddCookie(cookie)
	if rec := st.do(req); rec.Code != http.StatusUnauthorized {
		t.Errorf("post-reset session status = %d, want 401", rec.Code)
	}

	// The old admin token dies with the reset too.
	req = httptest.NewRequest(http.MethodGet, "/api/ping", nil)
	req.Header.Set("Authorization", "Bearer "+st.token)
	if rec := st.do(req); rec.Code != http.StatusUnauthorized {
		t.Errorf("post-reset bearer status = %d, want 401", rec.Code)
	}
}

func TestMiddlewareAuthDisabled(t *testing.T) {
	st := newAuthStack(t, false, nil)

	// Everything passes through, credentials or not.
	if rec := st.do(httptest.NewRequest(http.MethodGet, "/api/ping", nil)); rec.Code != http.StatusOK {
		t.Errorf("/api/ping status = %d, want 200 passthrough", rec.Code)
	}
	if rec := st.do(httptest.NewRequest(http.MethodGet, "/ws", nil)); rec.Code != http.StatusOK {
		t.Errorf("/ws status = %d, want 200 passthrough", rec.Code)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/ping", nil)
	req.Header.Set("Origin", "http://evil.example")
	if rec := st.do(req); rec.Code != http.StatusOK {
		t.Errorf("cross-origin POST status = %d, want 200 passthrough when disabled", rec.Code)
	}

	// And status reports that no auth is required.
	rec := st.do(httptest.NewRequest(http.MethodGet, "/api/auth/status", nil))
	var body map[string]bool
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("status body: %v", err)
	}
	if body["auth_required"] {
		t.Error("auth_required = true with auth disabled, want false")
	}
}

func TestAuthStatus(t *testing.T) {
	st := newAuthStack(t, true, nil)
	cookie := st.login(t)

	status := func(prepare func(*http.Request)) map[string]bool {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
		if prepare != nil {
			prepare(req)
		}
		rec := st.do(req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status code = %d, want 200 (always public)", rec.Code)
		}
		var body map[string]bool
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("status body: %v", err)
		}
		return body
	}

	if got := status(nil); !got["auth_required"] || got["authenticated"] {
		t.Errorf("anonymous status = %v, want auth_required=true authenticated=false", got)
	}
	if got := status(func(r *http.Request) { r.AddCookie(cookie) }); !got["authenticated"] {
		t.Errorf("cookie status = %v, want authenticated=true", got)
	}
	if got := status(func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+st.token) }); !got["authenticated"] {
		t.Errorf("bearer status = %v, want authenticated=true", got)
	}
}

func TestLogout(t *testing.T) {
	st := newAuthStack(t, true, nil)
	cookie := st.login(t)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.AddCookie(cookie)
	rec := st.do(req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, want 204", rec.Code)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName {
			if c.MaxAge >= 0 {
				t.Errorf("logout cookie MaxAge = %d, want negative (expired)", c.MaxAge)
			}
			return
		}
	}
	t.Fatal("logout did not clear the session cookie")
}

// --- origin policy -----------------------------------------------------------

func TestOriginEnforcement(t *testing.T) {
	st := newAuthStack(t, true, []string{"https://axios.example.net"})
	cookie := st.login(t)

	tests := []struct {
		name   string
		method string
		origin string
		host   string // "" keeps httptest's default (example.com)
		want   int
	}{
		{"evil origin blocked despite valid cookie", http.MethodPost, "http://evil.example", "", http.StatusForbidden},
		{"same host origin allowed", http.MethodPost, "http://example.com", "", http.StatusOK},
		{"same host different port allowed", http.MethodPost, "http://example.com:8443", "", http.StatusOK},
		{"no origin header allowed", http.MethodPost, "", "", http.StatusOK},
		{"loopback cross-port allowed (vite dev)", http.MethodPost, "http://localhost:5173", "127.0.0.1:3000", http.StatusOK},
		{"config allowlisted origin", http.MethodPost, "https://axios.example.net", "", http.StatusOK},
		{"GET skips the origin rule", http.MethodGet, "http://evil.example", "", http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/api/ping", nil)
			req.AddCookie(cookie)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if tt.host != "" {
				req.Host = tt.host
			}
			rec := st.do(req)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.want, rec.Body)
			}
			if tt.want == http.StatusForbidden {
				if got := errorBody(t, rec); got != "origin not allowed" {
					t.Errorf("403 body error = %q, want %q", got, "origin not allowed")
				}
			}
		})
	}

	// Login CSRF: a cross-origin login is rejected before token verification.
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login",
		strings.NewReader(fmt.Sprintf(`{"token":%q}`, st.token)))
	req.Header.Set("Origin", "http://evil.example")
	if rec := st.do(req); rec.Code != http.StatusForbidden {
		t.Errorf("cross-origin login status = %d, want 403", rec.Code)
	}
}

// TestWSCheckOrigin exercises the shared origin helper exactly as the
// websocket Upgrader consumes it, and proves SetAuth replaced the permissive
// default CheckOrigin on the shared upgrader.
func TestWSCheckOrigin(t *testing.T) {
	store, _ := newTestAuthStore(t)
	m := NewAuthManager(store, AuthOptions{
		Enabled:        true,
		AllowedOrigins: []string{"https://Axios.Example.net/"}, // normalized on load
	}, testLogger())

	srv := &Server{
		logger:   testLogger(),
		upgrader: websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
	}
	evil := &http.Request{Host: "localhost:3000", Header: http.Header{"Origin": []string{"http://evil.example"}}}
	if !srv.upgrader.CheckOrigin(evil) {
		t.Fatal("precondition: default CheckOrigin is permissive")
	}
	srv.SetAuth(m)
	if srv.upgrader.CheckOrigin(evil) {
		t.Fatal("SetAuth must replace the permissive CheckOrigin with the origin policy")
	}

	tests := []struct {
		name   string
		origin string
		host   string
		want   bool
	}{
		{"no origin (curl, wscat)", "", "127.0.0.1:3000", true},
		{"same host ignoring ports", "http://example.com:9999", "example.com:3000", true},
		{"vite dev proxy loopback", "http://localhost:5173", "127.0.0.1:3000", true},
		{"ipv6 loopback", "http://[::1]:5173", "localhost:3000", true},
		{"cross site", "http://evil.example", "localhost:3000", false},
		{"subdomain is not same host", "http://api.example.com", "example.com:3000", false},
		{"allowlisted origin (case/slash-insensitive)", "https://axios.example.net", "unrelated.host:3000", true},
		{"malformed origin", "http://", "localhost:3000", false},
		{"null origin", "null", "localhost:3000", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &http.Request{Host: tt.host, Header: http.Header{}}
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if got := srv.upgrader.CheckOrigin(req); got != tt.want {
				t.Errorf("CheckOrigin(origin=%q, host=%q) = %v, want %v", tt.origin, tt.host, got, tt.want)
			}
		})
	}
}

// --- token banner ------------------------------------------------------------

func TestPrintAuthTokenBanner(t *testing.T) {
	var buf bytes.Buffer
	PrintAuthTokenBanner(&buf, "axsk_test-token-123", false)
	out := buf.String()
	if !strings.Contains(out, "axsk_test-token-123") {
		t.Error("banner must contain the plaintext token")
	}
	if !strings.Contains(out, strings.Repeat("=", 80)) {
		t.Error("banner must be prominently framed")
	}
	if !strings.Contains(out, "--reset-auth") {
		t.Error("banner must point at --reset-auth for recovery")
	}

	var resetBuf bytes.Buffer
	PrintAuthTokenBanner(&resetBuf, "axsk_new", true)
	if !strings.Contains(resetBuf.String(), "RESET") {
		t.Error("reset banner must announce the reset")
	}
}
