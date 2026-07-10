package axiosd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeXAIAuthServer simulates auth.x.ai: discovery, device-code issuance,
// and a token endpoint whose behavior is scripted per test.
func fakeXAIAuthServer(t *testing.T, tokenHandler http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"token_endpoint": srv.URL + "/oauth2/token"})
	})
	mux.HandleFunc("/oauth2/device/code", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil || r.Form.Get("client_id") != xaiOAuthClientID {
			http.Error(w, "bad client", http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"device_code":               "dev-123",
			"user_code":                 "ABCD-1234",
			"verification_uri":          "https://accounts.x.ai/activate",
			"verification_uri_complete": "https://accounts.x.ai/activate?code=ABCD-1234",
			"expires_in":                60,
			"interval":                  1, // fast polling keeps tests quick
		})
	})
	mux.HandleFunc("/oauth2/token", tokenHandler)
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// newTestXAIOAuth points the flow at a fake issuer and a temp auth store.
func newTestXAIOAuth(t *testing.T, srv *httptest.Server) (*XAIOAuth, string) {
	t.Helper()
	authPath := filepath.Join(t.TempDir(), "auth.json")
	x := NewXAIOAuth(srv.Client(), authPath, testLogger())
	x.issuer = srv.URL
	return x, authPath
}

func waitForState(t *testing.T, x *XAIOAuth, want XAIOAuthState) XAIOAuthStatus {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		st := x.Status()
		if st.State == want {
			return st
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("flow never reached state %q (last: %+v)", want, x.Status())
	return XAIOAuthStatus{}
}

func TestXAIOAuthHappyPath(t *testing.T) {
	var polls atomic.Int32
	srv := fakeXAIAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:device_code" ||
			r.Form.Get("device_code") != "dev-123" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		// pending twice, then approved.
		if polls.Add(1) < 3 {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at-xyz", "refresh_token": "rt-xyz", "expires_in": 21600,
		})
	})
	x, authPath := newTestXAIOAuth(t, srv)

	var restarted atomic.Bool
	x.onConnected = func() { restarted.Store(true) }

	st, err := x.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if st.State != XAIOAuthPending || st.UserCode != "ABCD-1234" ||
		st.VerificationURI != "https://accounts.x.ai/activate?code=ABCD-1234" {
		t.Fatalf("pending status = %+v", st)
	}

	done := waitForState(t, x, XAIOAuthConnected)
	if !done.Connected {
		t.Error("Connected flag not set after success")
	}
	if !restarted.Load() {
		t.Error("onConnected hook not invoked")
	}

	// Tokens stored in opencode's shape, file mode 0600.
	data, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatalf("auth store not written: %v", err)
	}
	var entries map[string]opencodeAuthEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("auth store not valid JSON: %v", err)
	}
	e := entries["xai"]
	if e.Type != "oauth" || e.Access != "at-xyz" || e.Refresh != "rt-xyz" || e.Expires <= time.Now().UnixMilli() {
		t.Errorf("stored entry = %+v", e)
	}
	if info, _ := os.Stat(authPath); info.Mode().Perm() != 0o600 {
		t.Errorf("auth store mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestXAIOAuthDenied(t *testing.T) {
	srv := fakeXAIAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "access_denied", "error_description": "user denied"})
	})
	x, authPath := newTestXAIOAuth(t, srv)

	if _, err := x.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	st := waitForState(t, x, XAIOAuthError)
	if st.Error == "" || st.Connected {
		t.Errorf("denied status = %+v", st)
	}
	if _, err := os.Stat(authPath); !os.IsNotExist(err) {
		t.Error("auth store must not be written on denial")
	}
}

func TestXAIOAuthTierGate403(t *testing.T) {
	srv := fakeXAIAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "forbidden"})
	})
	x, _ := newTestXAIOAuth(t, srv)
	if _, err := x.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	st := waitForState(t, x, XAIOAuthError)
	if !strings.Contains(st.Error, "console.x.ai API key") {
		t.Errorf("403 error should point at the API-key fallback, got %q", st.Error)
	}
}

func TestXAIOAuthStartIsIdempotentWhilePending(t *testing.T) {
	srv := fakeXAIAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
	})
	x, _ := newTestXAIOAuth(t, srv)
	first, err := x.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	second, err := x.Start()
	if err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if second.UserCode != first.UserCode {
		t.Errorf("pending Start returned a new flow: %q vs %q", second.UserCode, first.UserCode)
	}
}

func TestWriteOpencodeXAIAuthMergesExistingProviders(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	existing := `{"anthropic":{"type":"api","key":"sk-ant-x"},"openai":{"type":"oauth","access":"a","refresh":"r","expires":1}}`
	if err := os.WriteFile(path, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := writeOpencodeXAIAuth(path, "at", "rt", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("writeOpencodeXAIAuth: %v", err)
	}

	var entries map[string]json.RawMessage
	data, _ := os.ReadFile(path)
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"anthropic", "openai", "xai"} {
		if _, ok := entries[key]; !ok {
			t.Errorf("entry %q missing after merge (got keys %v)", key, keysOf(entries))
		}
	}

	// Corrupt store must be refused, not clobbered.
	bad := filepath.Join(t.TempDir(), "auth.json")
	os.WriteFile(bad, []byte("not json"), 0o600)
	if err := writeOpencodeXAIAuth(bad, "at", "rt", time.Now()); err == nil {
		t.Error("corrupt auth store should refuse the write")
	}
	if data, _ := os.ReadFile(bad); string(data) != "not json" {
		t.Error("corrupt auth store was overwritten")
	}
}

func keysOf(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestHasXAIOAuthEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	if hasXAIOAuthEntry(path) {
		t.Error("missing file should not report connected")
	}
	writeOpencodeXAIAuth(path, "at", "rt", time.Now().Add(time.Hour))
	if !hasXAIOAuthEntry(path) {
		t.Error("stored entry should report connected")
	}
}

func TestXAIOAuthRESTEndpoints(t *testing.T) {
	srv := fakeXAIAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at", "refresh_token": "rt", "expires_in": 3600,
		})
	})
	s := &Server{logger: testLogger()}
	s.xaiOAuthOnce.Do(func() {}) // pre-arm Once so we can inject the test flow
	flow, _ := newTestXAIOAuth(t, srv)
	s.xaiOAuthFlow = flow

	rec := httptest.NewRecorder()
	s.handleXAIOAuthStart(rec, httptest.NewRequest(http.MethodPost, "/api/providers/xai/oauth/start", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("start status = %d, body %s", rec.Code, rec.Body)
	}
	var st XAIOAuthStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st.State != XAIOAuthPending || st.UserCode == "" || st.VerificationURI == "" {
		t.Errorf("start response = %+v", st)
	}

	waitForState(t, flow, XAIOAuthConnected)
	rec = httptest.NewRecorder()
	s.handleXAIOAuthStatus(rec, httptest.NewRequest(http.MethodGet, "/api/providers/xai/oauth/status", nil))
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st.State != XAIOAuthConnected || !st.Connected {
		t.Errorf("status response = %+v", st)
	}

	// Method guards.
	rec = httptest.NewRecorder()
	s.handleXAIOAuthStart(rec, httptest.NewRequest(http.MethodGet, "/api/providers/xai/oauth/start", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET on start = %d, want 405", rec.Code)
	}
}

func TestDelegateUsesConfiguredDefaultModel(t *testing.T) {
	client := &fakeOpencodeClient{}
	m := newTestOpencodeManager(t, client)
	m.opts.Model = "xai/grok-build-0.1"

	if _, err := m.Delegate("do it", "", nil); err != nil {
		t.Fatalf("Delegate: %v", err)
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if len(client.promptModels) != 1 || client.promptModels[0] != "xai/grok-build-0.1" {
		t.Errorf("prompt models = %v, want the configured default", client.promptModels)
	}
}
