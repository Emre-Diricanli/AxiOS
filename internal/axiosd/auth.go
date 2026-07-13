package axiosd

// Admin authentication for the axiosd HTTP/WebSocket surface.
//
// Model (Jupyter-style): on first start the daemon generates a random admin
// token ("axsk_" + base64url) and prints it ONCE to the daemon log. Only its
// SHA-256 hash is persisted, in $AXIOS_DATA_DIR/auth.json (0600), alongside a
// separate HMAC key used to sign stateless session cookies and an integer
// epoch. `axiosd --reset-auth` rotates token + key and bumps the epoch, which
// invalidates every outstanding session. The plaintext token can never be
// recovered — only reset.
//
// Enforcement is a single middleware wrapped around the entire mux (fail
// closed): everything under /api, /ws and /v1 requires a valid session
// cookie or an `Authorization: Bearer axsk_...` header, with a tiny public
// allowlist (/api/auth/login, /api/auth/status) so the SPA can render its
// login screen. The same origin policy guards websocket upgrades and
// state-changing HTTP requests against cross-site attacks.
//
// Frontend/API contract: docs/auth-api.md.

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// authTokenPrefix marks admin tokens ("axsk" = AxiOS secret key).
	authTokenPrefix = "axsk_"

	// sessionCookieName carries the HMAC-signed session between requests.
	sessionCookieName = "axios_session"

	// sessionVersion is the leading field of every session cookie value.
	sessionVersion = "v1"

	// defaultSessionTTL applies when config omits session_ttl_hours.
	defaultSessionTTL = 168 * time.Hour

	// authKeySize is the byte length of both the admin token entropy and the
	// HMAC session-signing key.
	authKeySize = 32

	// sessionNonceSize makes every issued session value unique.
	sessionNonceSize = 16

	// loginFailureLimit / loginFailureWindow: at most 5 failed login attempts
	// per remote IP per sliding minute before /api/auth/login returns 429.
	loginFailureLimit  = 5
	loginFailureWindow = time.Minute
)

// authState is the on-disk auth.json format. It contains no plaintext
// secrets: the token is stored only as a SHA-256 hash. The session key is a
// signing key, not a credential — anyone who can read this file can already
// run code as the AxiOS user (same threat model as pkg/secrets).
type authState struct {
	Version     int       `json:"version"`
	TokenSHA256 string    `json:"token_sha256"` // hex sha256 of the full "axsk_..." string
	SessionKey  string    `json:"session_key"`  // base64url (no padding), 32 bytes
	Epoch       int64     `json:"epoch"`        // bumped by --reset-auth; sessions embed it
	CreatedAt   time.Time `json:"created_at"`
}

// validate rejects malformed state files with an error, never a panic.
func (st authState) validate() error {
	if st.Version != 1 {
		return fmt.Errorf("unsupported version %d", st.Version)
	}
	hash, err := hex.DecodeString(st.TokenSHA256)
	if err != nil || len(hash) != sha256.Size {
		return errors.New("malformed token hash")
	}
	key, err := base64.RawURLEncoding.DecodeString(st.SessionKey)
	if err != nil || len(key) != authKeySize {
		return errors.New("malformed session key")
	}
	if st.Epoch < 1 {
		return fmt.Errorf("epoch %d out of range", st.Epoch)
	}
	return nil
}

// AuthStore verifies admin tokens and signs/verifies session values against
// the persisted auth state. Safe for concurrent use.
type AuthStore struct {
	mu         sync.Mutex
	path       string
	tokenHash  []byte // sha256 of the plaintext token
	sessionKey []byte // HMAC-SHA256 signing key for session values
	epoch      int64
}

// LoadOrCreateAuthState loads auth.json from path, generating fresh state
// (mode 0600, parent dir 0700) when none exists. The returned string is the
// plaintext admin token and is non-empty ONLY at generation time — this is
// the caller's single chance to show it to the user (PrintAuthTokenBanner).
// A corrupt state file is an error, never silently regenerated: recovery is
// the explicit --reset-auth path.
func LoadOrCreateAuthState(statePath string) (*AuthStore, string, error) {
	st, err := readAuthState(statePath)
	if err == nil {
		store, serr := storeFromState(statePath, st)
		return store, "", serr
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, "", err
	}

	st, token, err := newAuthState(1)
	if err != nil {
		return nil, "", err
	}
	if err := createAuthState(statePath, st); err != nil {
		if errors.Is(err, fs.ErrExist) {
			// Another process won the creation race; adopt its state. Our
			// token was never persisted, so it must not be printed.
			existing, rerr := readAuthState(statePath)
			if rerr != nil {
				return nil, "", rerr
			}
			store, serr := storeFromState(statePath, existing)
			return store, "", serr
		}
		return nil, "", err
	}
	store, err := storeFromState(statePath, st)
	return store, token, err
}

// ResetAuthState is the --reset-auth entry point: it writes brand-new state
// (fresh token + session key, epoch+1) and returns the new plaintext token.
// It succeeds even when the existing file is missing or corrupt, because
// reset is the recovery path and must never dead-end.
func ResetAuthState(statePath string) (string, error) {
	var epoch int64
	if st, err := readAuthState(statePath); err == nil {
		epoch = st.Epoch
	}
	st, token, err := newAuthState(epoch + 1)
	if err != nil {
		return "", err
	}
	if err := overwriteAuthState(statePath, st); err != nil {
		return "", err
	}
	return token, nil
}

// Reset rotates the token and session key on a live store and bumps the
// epoch, invalidating every outstanding session. Returns the new plaintext
// token — the only moment it is available.
func (s *AuthStore) Reset() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	st, token, err := newAuthState(s.epoch + 1)
	if err != nil {
		return "", err
	}
	if err := overwriteAuthState(s.path, st); err != nil {
		return "", err
	}
	hash, err := hex.DecodeString(st.TokenSHA256)
	if err != nil {
		return "", fmt.Errorf("auth: decode token hash: %w", err)
	}
	key, err := base64.RawURLEncoding.DecodeString(st.SessionKey)
	if err != nil {
		return "", fmt.Errorf("auth: decode session key: %w", err)
	}
	s.tokenHash, s.sessionKey, s.epoch = hash, key, st.Epoch
	return token, nil
}

// VerifyToken reports whether presented is the admin token, comparing
// sha256(presented) against the stored hash in constant time.
func (s *AuthStore) VerifyToken(presented string) bool {
	sum := sha256.Sum256([]byte(presented))
	s.mu.Lock()
	defer s.mu.Unlock()
	return subtle.ConstantTimeCompare(sum[:], s.tokenHash) == 1
}

// IssueSession signs a fresh session value valid for ttl:
// "v1.<epoch>.<expiresUnix>.<nonceB64>.<hmacB64>" where the HMAC-SHA256
// covers everything before the last dot.
func (s *AuthStore) IssueSession(ttl time.Duration) (string, error) {
	if ttl <= 0 {
		ttl = defaultSessionTTL
	}
	nonce := make([]byte, sessionNonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("auth: generate session nonce: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.signSessionLocked(s.epoch, time.Now().Add(ttl).Unix(), nonce), nil
}

// signSessionLocked builds and signs a session value. Callers hold s.mu.
func (s *AuthStore) signSessionLocked(epoch, expiresUnix int64, nonce []byte) string {
	payload := fmt.Sprintf("%s.%d.%d.%s",
		sessionVersion, epoch, expiresUnix, base64.RawURLEncoding.EncodeToString(nonce))
	mac := hmac.New(sha256.New, s.sessionKey)
	mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// VerifySession checks a session value: version, HMAC (constant time via
// hmac.Equal), epoch equal to the current one, and not expired.
func (s *AuthStore) VerifySession(value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) != 5 || parts[0] != sessionVersion {
		return false
	}
	mac, err := base64.RawURLEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	want := hmac.New(sha256.New, s.sessionKey)
	want.Write([]byte(strings.Join(parts[:4], ".")))
	if !hmac.Equal(want.Sum(nil), mac) {
		return false
	}
	epoch, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || epoch != s.epoch {
		return false // epoch bumped by --reset-auth → session invalid
	}
	expiresUnix, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return false
	}
	return time.Now().Unix() < expiresUnix
}

// newAuthState generates fresh state plus the plaintext token it hashes.
func newAuthState(epoch int64) (authState, string, error) {
	raw := make([]byte, authKeySize)
	if _, err := rand.Read(raw); err != nil {
		return authState{}, "", fmt.Errorf("auth: generate token: %w", err)
	}
	token := authTokenPrefix + base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))

	key := make([]byte, authKeySize)
	if _, err := rand.Read(key); err != nil {
		return authState{}, "", fmt.Errorf("auth: generate session key: %w", err)
	}

	return authState{
		Version:     1,
		TokenSHA256: hex.EncodeToString(sum[:]),
		SessionKey:  base64.RawURLEncoding.EncodeToString(key),
		Epoch:       epoch,
		CreatedAt:   time.Now().UTC(),
	}, token, nil
}

// readAuthState loads and validates auth.json. Missing files surface as an
// error wrapping fs.ErrNotExist; corrupt files as descriptive errors.
func readAuthState(statePath string) (authState, error) {
	data, err := os.ReadFile(statePath)
	if err != nil {
		return authState{}, fmt.Errorf("auth: read state file: %w", err)
	}
	var st authState
	if err := json.Unmarshal(data, &st); err != nil {
		return authState{}, fmt.Errorf("auth: parse state file %s: %w", statePath, err)
	}
	if err := st.validate(); err != nil {
		return authState{}, fmt.Errorf("auth: invalid state file %s: %w", statePath, err)
	}
	return st, nil
}

// storeFromState builds an AuthStore from validated state.
func storeFromState(statePath string, st authState) (*AuthStore, error) {
	hash, err := hex.DecodeString(st.TokenSHA256)
	if err != nil {
		return nil, fmt.Errorf("auth: decode token hash: %w", err)
	}
	key, err := base64.RawURLEncoding.DecodeString(st.SessionKey)
	if err != nil {
		return nil, fmt.Errorf("auth: decode session key: %w", err)
	}
	return &AuthStore{path: statePath, tokenHash: hash, sessionKey: key, epoch: st.Epoch}, nil
}

// createAuthState writes brand-new state with O_EXCL so a concurrent creator
// is detected (the returned error wraps fs.ErrExist), mirroring
// pkg/secrets' key-file handling.
func createAuthState(statePath string, st authState) error {
	if dir := filepath.Dir(statePath); dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("auth: create state directory: %w", err)
		}
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("auth: encode state: %w", err)
	}
	f, err := os.OpenFile(statePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("auth: create state file: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(statePath)
		return fmt.Errorf("auth: write state file: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(statePath)
		return fmt.Errorf("auth: close state file: %w", err)
	}
	return nil
}

// overwriteAuthState atomically replaces the state file (temp + rename) so a
// crash mid-reset never leaves a corrupt auth.json behind.
func overwriteAuthState(statePath string, st authState) error {
	if dir := filepath.Dir(statePath); dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("auth: create state directory: %w", err)
		}
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("auth: encode state: %w", err)
	}
	tmp := statePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("auth: write state file: %w", err)
	}
	if err := os.Rename(tmp, statePath); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("auth: replace state file: %w", err)
	}
	return nil
}

// PrintAuthTokenBanner writes the plaintext admin token to w, prominently
// framed (Jupyter-style). This is the ONLY place the token may ever be
// emitted — at generation or reset — and callers write it to the daemon log
// (stderr), never through the structured logger.
func PrintAuthTokenBanner(w io.Writer, token string, reset bool) {
	headline := "AxiOS admin token generated (first start)."
	if reset {
		headline = "AxiOS admin token RESET — the previous token and ALL sessions are now invalid."
	}
	fmt.Fprintf(w, `
================================================================================

    %s

    Save it now — it cannot be recovered, only reset (axiosd --reset-auth):

        %s

    Sign in to the web UI with it, or use it as an API key:

        Authorization: Bearer %s

================================================================================

`, headline, token, token)
}

// --- Middleware -------------------------------------------------------------

// authClass is the credential class a request path requires.
type authClass int

const (
	// authPublic paths need no credentials: the SPA shell and assets (the
	// login screen must render), /api/auth/login and /api/auth/status.
	authPublic authClass = iota

	// authAdmin paths require a valid session cookie or the admin token as
	// an Authorization: Bearer header. Everything under /api, /ws and /v1
	// defaults to this class — fail closed for routes added tomorrow.
	authAdmin

	// authMachine is reserved for machine endpoints (the per-host telemetry
	// token scheme owned by Codex). Until that verifier exists, machine
	// paths still require admin credentials.
	authMachine
)

// machineAuthEndpoints assigns specific paths a non-default auth class.
// Deliberately EMPTY for now: /api/hosts/stats and friends stay
// admin-protected until the per-host telemetry token scheme lands. To
// register a machine endpoint, add its exact (cleaned) path here mapped to
// authMachine — see docs/auth-api.md, "Machine endpoints".
var machineAuthEndpoints = map[string]authClass{}

// AuthOptions configures NewAuthManager from the server.auth config section.
type AuthOptions struct {
	Enabled        bool          // middleware enforcement on/off (default true in config)
	SessionTTL     time.Duration // login session lifetime; <= 0 → defaultSessionTTL
	AllowedOrigins []string      // extra browser origins beyond same-host/loopback
}

// AuthManager glues the token store, session settings, origin policy and
// login rate limiter together for the middleware and the /api/auth handlers.
type AuthManager struct {
	store          *AuthStore
	enabled        bool
	sessionTTL     time.Duration
	allowedOrigins map[string]struct{} // normalized (lowercase, no trailing /)
	limiter        *loginLimiter
	logger         *slog.Logger
}

// NewAuthManager builds the manager. store may be nil only when auth is
// disabled — with enforcement on and no store, every protected request would
// fail closed with 401.
func NewAuthManager(store *AuthStore, opts AuthOptions, logger *slog.Logger) *AuthManager {
	ttl := opts.SessionTTL
	if ttl <= 0 {
		ttl = defaultSessionTTL
	}
	origins := make(map[string]struct{}, len(opts.AllowedOrigins))
	for _, o := range opts.AllowedOrigins {
		if norm := normalizeOrigin(o); norm != "" {
			origins[norm] = struct{}{}
		}
	}
	return &AuthManager{
		store:          store,
		enabled:        opts.Enabled,
		sessionTTL:     ttl,
		allowedOrigins: origins,
		limiter:        newLoginLimiter(),
		logger:         logger,
	}
}

// Middleware wraps next (the complete route mux) in the admin auth policy.
// It is applied exactly once, around the entire mux, so every current and
// future route is protected by default.
//
// Order: disabled bypass → origin check on state-changing requests (CSRF) →
// path classification → credential check. Unauthenticated API/WS requests
// get 401 JSON, never a redirect.
func (m *AuthManager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !m.enabled {
			next.ServeHTTP(w, r)
			return
		}

		// CSRF guard: state-changing cross-origin requests are rejected even
		// with valid credentials, because browsers attach cookies on their
		// own. Safe methods skip this; responses stay unreadable cross-origin
		// (no CORS headers are ever set).
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if r.Header.Get("Origin") != "" && !m.originAllowed(r) {
				m.logger.Warn("cross-origin request rejected",
					"origin", r.Header.Get("Origin"), "host", r.Host, "path", r.URL.Path)
				writeAuthError(w, http.StatusForbidden, "origin not allowed")
				return
			}
		}

		switch m.classify(r.URL.Path) {
		case authPublic:
			next.ServeHTTP(w, r)
		case authAdmin, authMachine:
			// authMachine entries keep requiring admin credentials until the
			// per-host telemetry verifier exists (see machineAuthEndpoints).
			if m.authenticated(r) {
				next.ServeHTTP(w, r)
				return
			}
			writeAuthError(w, http.StatusUnauthorized, "unauthorized")
		default:
			// Unknown class: fail closed.
			writeAuthError(w, http.StatusUnauthorized, "unauthorized")
		}
	})
}

// classify maps a request path to its auth class. The path is cleaned first
// so "//api/x" or "/foo/../api/x" cannot dodge the /api prefix check.
func (m *AuthManager) classify(requestPath string) authClass {
	p := path.Clean(requestPath)
	if !strings.HasPrefix(p, "/") {
		return authAdmin // malformed target — fail closed
	}
	if class, ok := machineAuthEndpoints[p]; ok {
		return class
	}
	switch p {
	case "/api/auth/login", "/api/auth/status":
		return authPublic
	}
	if isProtectedPath(p) {
		return authAdmin
	}
	// Everything else is the SPA shell: index.html and static assets must
	// load without credentials so the login screen can render.
	return authPublic
}

// isProtectedPath reports whether a cleaned path lives under one of the
// credentialed prefixes.
func isProtectedPath(p string) bool {
	for _, prefix := range []string{"/api", "/ws", "/v1"} {
		if p == prefix || strings.HasPrefix(p, prefix+"/") {
			return true
		}
	}
	return false
}

// authenticated reports whether the request carries a valid session cookie
// or the admin token as a bearer header (both verified in constant time).
func (m *AuthManager) authenticated(r *http.Request) bool {
	if m.store == nil {
		return false
	}
	if c, err := r.Cookie(sessionCookieName); err == nil && m.store.VerifySession(c.Value) {
		return true
	}
	if token, ok := bearerToken(r); ok {
		return m.store.VerifyToken(token)
	}
	return false
}

// bearerToken extracts the credential from an "Authorization: Bearer ..."
// header (scheme match is case-insensitive per RFC 6750).
func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	const scheme = "bearer "
	if len(h) <= len(scheme) || !strings.EqualFold(h[:len(scheme)], scheme) {
		return "", false
	}
	return strings.TrimSpace(h[len(scheme):]), true
}

// writeAuthError emits the middleware's JSON error body (the Server-level
// jsonError helper is not reachable from here).
func writeAuthError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// --- Origin policy ----------------------------------------------------------

// originAllowed is the shared origin policy for websocket upgrades
// (Upgrader.CheckOrigin — wired in Server.SetAuth) and state-changing HTTP
// requests. It returns true when:
//
//	(a) there is no Origin header — non-browser clients, which still have to
//	    pass token/cookie auth;
//	(b) the origin host equals the request Host, ignoring ports;
//	(c) BOTH hosts are loopback (localhost/127.0.0.1/::1) — keeps the Vite
//	    dev server on :5173 talking to axiosd on :3000;
//	(d) the origin is listed in server.auth.allowed_origins.
//
// Malformed and "null" origins fail closed.
func (m *AuthManager) originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // (a)
	}
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" {
		return false
	}
	originHost := u.Hostname()
	requestHost := hostWithoutPort(r.Host)
	if strings.EqualFold(originHost, requestHost) {
		return true // (b)
	}
	if isLoopbackHost(originHost) && isLoopbackHost(requestHost) {
		return true // (c)
	}
	_, ok := m.allowedOrigins[normalizeOrigin(origin)]
	return ok // (d)
}

// hostWithoutPort strips an optional :port (and IPv6 brackets) from a Host
// header value.
func hostWithoutPort(hostport string) string {
	if host, _, err := net.SplitHostPort(hostport); err == nil {
		return host
	}
	return strings.Trim(hostport, "[]")
}

// isLoopbackHost reports whether host is localhost or a loopback IP.
func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// normalizeOrigin canonicalizes an origin for allowlist comparison:
// lowercase, no trailing slash.
func normalizeOrigin(origin string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(origin)), "/")
}

// --- Login rate limiting ----------------------------------------------------

// loginLimiter enforces a per-remote-IP sliding window on FAILED login
// attempts. Only the connection's RemoteAddr is consulted — proxy headers
// like X-Forwarded-For are attacker-controlled and deliberately ignored.
type loginLimiter struct {
	mu        sync.Mutex
	limit     int
	window    time.Duration
	failures  map[string][]time.Time
	lastSweep time.Time
	now       func() time.Time // injectable for tests
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{
		limit:    loginFailureLimit,
		window:   loginFailureWindow,
		failures: make(map[string][]time.Time),
		now:      time.Now,
	}
}

// tooManyFailures reports whether ip has exhausted its failure budget in the
// current window. It also runs the periodic cleanup of stale entries.
func (l *loginLimiter) tooManyFailures(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.sweepLocked(now)
	recent := pruneBefore(l.failures[ip], now.Add(-l.window))
	if len(recent) == 0 {
		delete(l.failures, ip)
	} else {
		l.failures[ip] = recent
	}
	return len(recent) >= l.limit
}

// recordFailure notes one failed attempt for ip.
func (l *loginLimiter) recordFailure(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failures[ip] = append(l.failures[ip], l.now())
}

// clear forgets ip's failures (called after a successful login).
func (l *loginLimiter) clear(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, ip)
}

// sweepLocked drops expired records across all IPs at most once per window,
// keeping the map bounded without a background goroutine. Callers hold l.mu.
func (l *loginLimiter) sweepLocked(now time.Time) {
	if now.Sub(l.lastSweep) < l.window {
		return
	}
	l.lastSweep = now
	cutoff := now.Add(-l.window)
	for ip, times := range l.failures {
		if recent := pruneBefore(times, cutoff); len(recent) == 0 {
			delete(l.failures, ip)
		} else {
			l.failures[ip] = recent
		}
	}
}

// pruneBefore returns only the timestamps after cutoff.
func pruneBefore(times []time.Time, cutoff time.Time) []time.Time {
	var kept []time.Time
	for _, t := range times {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	return kept
}

// remoteIP extracts the connection's IP from RemoteAddr for rate limiting.
func remoteIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
