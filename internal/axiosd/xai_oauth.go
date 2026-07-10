package axiosd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// xAI subscription OAuth (SuperGrok / X Premium+), implemented as the same
// device-code flow xAI's Grok Build CLI and sanctioned third-party agents
// (opencode, hermes-agent, Warp) use, via xAI's shared public OAuth client.
// The resulting tokens are written into opencode's auth store, so delegated
// coding tasks run on the user's subscription quota instead of API billing.
//
// Note: xAI enforces a server-side allowlist on this surface — some standard
// SuperGrok accounts receive HTTP 403 despite an active subscription. The
// console.x.ai API-key path is the ungated fallback.
const (
	xaiOAuthIssuer   = "https://auth.x.ai"
	xaiOAuthClientID = "b1a00492-073a-47ea-816f-4c329264a828" // xAI's shared public "Grok Build" client
	xaiOAuthScope    = "openid profile email offline_access grok-cli:access api:access"

	// xaiTokenFallbackTTL is assumed when the token response omits
	// expires_in; opencode refreshes expired entries itself, so
	// underestimating is safe.
	xaiTokenFallbackTTL = time.Hour
)

// XAIOAuthState is the lifecycle of one device-code flow.
type XAIOAuthState string

const (
	XAIOAuthIdle      XAIOAuthState = "idle"
	XAIOAuthPending   XAIOAuthState = "pending"
	XAIOAuthConnected XAIOAuthState = "connected"
	XAIOAuthError     XAIOAuthState = "error"
)

// XAIOAuthStatus is the JSON shape of GET /api/providers/xai/oauth/status.
type XAIOAuthStatus struct {
	State           XAIOAuthState `json:"state"`
	UserCode        string        `json:"user_code,omitempty"`
	VerificationURI string        `json:"verification_uri,omitempty"`
	ExpiresAt       *time.Time    `json:"expires_at,omitempty"` // pending flow expiry
	Error           string        `json:"error,omitempty"`
	// Connected reports whether opencode's auth store currently holds an
	// xAI OAuth entry, regardless of any in-flight flow.
	Connected bool `json:"connected"`
}

// XAIOAuth runs xAI device-code login flows, one at a time.
type XAIOAuth struct {
	hc       *http.Client
	issuer   string // OAuth issuer base URL; tests point it at httptest
	authPath string // opencode auth.json; "" = resolve default lazily
	logger   *slog.Logger
	// onConnected is invoked after tokens are stored (e.g. to restart the
	// managed opencode server so it picks up the new credentials).
	onConnected func()

	mu        sync.Mutex
	state     XAIOAuthState
	userCode  string
	verifyURI string
	expiresAt time.Time
	lastErr   string
	cancel    context.CancelFunc
}

// NewXAIOAuth creates the flow manager. hc nil uses a 30s-timeout client;
// authPath "" resolves opencode's default auth store location.
func NewXAIOAuth(hc *http.Client, authPath string, logger *slog.Logger) *XAIOAuth {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &XAIOAuth{hc: hc, issuer: xaiOAuthIssuer, authPath: authPath, logger: logger, state: XAIOAuthIdle}
}

// opencodeAuthPath resolves opencode's credential store:
// $XDG_DATA_HOME/opencode/auth.json, defaulting to ~/.local/share.
func opencodeAuthPath() string {
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return filepath.Join(dir, "opencode", "auth.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "opencode", "auth.json")
}

func (x *XAIOAuth) resolveAuthPath() string {
	if x.authPath != "" {
		return x.authPath
	}
	return opencodeAuthPath()
}

// Status reports the current flow state plus whether stored credentials exist.
func (x *XAIOAuth) Status() XAIOAuthStatus {
	x.mu.Lock()
	defer x.mu.Unlock()
	st := XAIOAuthStatus{
		State:     x.state,
		Error:     x.lastErr,
		Connected: hasXAIOAuthEntry(x.resolveAuthPath()),
	}
	if x.state == XAIOAuthPending {
		st.UserCode = x.userCode
		st.VerificationURI = x.verifyURI
		exp := x.expiresAt
		st.ExpiresAt = &exp
	}
	return st
}

// Start begins a device-code flow (or returns the still-pending one) and
// polls for approval in the background. The returned status carries the
// verification URL and user code to show the user.
func (x *XAIOAuth) Start() (XAIOAuthStatus, error) {
	x.mu.Lock()
	if x.state == XAIOAuthPending && time.Now().Before(x.expiresAt) {
		st := XAIOAuthStatus{State: x.state, UserCode: x.userCode, VerificationURI: x.verifyURI}
		exp := x.expiresAt
		st.ExpiresAt = &exp
		x.mu.Unlock()
		return st, nil
	}
	if x.cancel != nil {
		x.cancel()
		x.cancel = nil
	}
	x.mu.Unlock()

	tokenEndpoint, err := x.discoverTokenEndpoint()
	if err != nil {
		return x.fail(fmt.Errorf("xAI OAuth discovery failed: %w", err))
	}

	dev, err := x.requestDeviceCode()
	if err != nil {
		return x.fail(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	x.mu.Lock()
	x.state = XAIOAuthPending
	x.userCode = dev.UserCode
	x.verifyURI = dev.verificationURL()
	x.expiresAt = time.Now().Add(time.Duration(dev.ExpiresIn) * time.Second)
	x.lastErr = ""
	x.cancel = cancel
	st := XAIOAuthStatus{State: x.state, UserCode: x.userCode, VerificationURI: x.verifyURI}
	exp := x.expiresAt
	st.ExpiresAt = &exp
	x.mu.Unlock()

	go x.pollUntilDone(ctx, tokenEndpoint, dev)
	return st, nil
}

func (x *XAIOAuth) fail(err error) (XAIOAuthStatus, error) {
	x.mu.Lock()
	x.state = XAIOAuthError
	x.lastErr = err.Error()
	x.mu.Unlock()
	return x.Status(), err
}

// deviceCodeResponse is the /oauth2/device/code payload.
type deviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

func (d *deviceCodeResponse) verificationURL() string {
	if d.VerificationURIComplete != "" {
		return d.VerificationURIComplete
	}
	return d.VerificationURI
}

// discoverTokenEndpoint reads the OIDC discovery document.
func (x *XAIOAuth) discoverTokenEndpoint() (string, error) {
	discoveryURL := strings.TrimRight(x.issuer, "/") + "/.well-known/openid-configuration"
	resp, err := x.hc.Get(discoveryURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("discovery returned HTTP %d", resp.StatusCode)
	}
	var doc struct {
		TokenEndpoint string `json:"token_endpoint"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return "", err
	}
	if doc.TokenEndpoint == "" {
		return "", fmt.Errorf("discovery document has no token_endpoint")
	}
	return doc.TokenEndpoint, nil
}

// requestDeviceCode asks xAI for a device authorization.
func (x *XAIOAuth) requestDeviceCode() (*deviceCodeResponse, error) {
	deviceURL := strings.TrimRight(x.issuer, "/") + "/oauth2/device/code"
	form := url.Values{"client_id": {xaiOAuthClientID}, "scope": {xaiOAuthScope}}
	resp, err := x.hc.Post(deviceURL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("xAI device-code request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("xAI device-code request failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var dev deviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&dev); err != nil {
		return nil, fmt.Errorf("xAI device-code response undecodable: %w", err)
	}
	if dev.DeviceCode == "" || dev.UserCode == "" || dev.verificationURL() == "" {
		return nil, fmt.Errorf("xAI device-code response missing required fields")
	}
	if dev.Interval <= 0 {
		dev.Interval = 5
	}
	if dev.ExpiresIn <= 0 {
		dev.ExpiresIn = 600
	}
	return &dev, nil
}

// pollUntilDone polls the token endpoint per RFC 8628 semantics until the
// user approves, denies, or the code expires.
func (x *XAIOAuth) pollUntilDone(ctx context.Context, tokenEndpoint string, dev *deviceCodeResponse) {
	interval := time.Duration(dev.Interval) * time.Second
	deadline := time.Now().Add(time.Duration(dev.ExpiresIn) * time.Second)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}

		form := url.Values{
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
			"client_id":   {xaiOAuthClientID},
			"device_code": {dev.DeviceCode},
		}
		resp, err := x.hc.Post(tokenEndpoint, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
		if err != nil {
			x.finish(XAIOAuthError, fmt.Sprintf("token polling failed: %v", err))
			return
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			var tok struct {
				AccessToken  string `json:"access_token"`
				RefreshToken string `json:"refresh_token"`
				ExpiresIn    int    `json:"expires_in"`
			}
			if err := json.Unmarshal(body, &tok); err != nil || tok.AccessToken == "" || tok.RefreshToken == "" {
				x.finish(XAIOAuthError, "token response missing access or refresh token")
				return
			}
			ttl := time.Duration(tok.ExpiresIn) * time.Second
			if ttl <= 0 {
				ttl = xaiTokenFallbackTTL
			}
			if err := writeOpencodeXAIAuth(x.resolveAuthPath(), tok.AccessToken, tok.RefreshToken, time.Now().Add(ttl)); err != nil {
				x.finish(XAIOAuthError, fmt.Sprintf("failed to store credentials: %v", err))
				return
			}
			x.logger.Info("xAI SuperGrok OAuth connected — credentials stored for opencode")
			x.finish(XAIOAuthConnected, "")
			if x.onConnected != nil {
				x.onConnected()
			}
			return
		}

		var oauthErr struct {
			Error       string `json:"error"`
			Description string `json:"error_description"`
		}
		_ = json.Unmarshal(body, &oauthErr)
		switch oauthErr.Error {
		case "authorization_pending":
			continue
		case "slow_down":
			if interval < 30*time.Second {
				interval += time.Second
			}
			continue
		default:
			msg := oauthErr.Description
			if msg == "" {
				msg = oauthErr.Error
			}
			if msg == "" {
				msg = fmt.Sprintf("HTTP %d", resp.StatusCode)
			}
			if resp.StatusCode == http.StatusForbidden {
				msg += " — xAI gates this surface per subscription tier; if your SuperGrok plan is rejected, use a console.x.ai API key instead"
			}
			x.finish(XAIOAuthError, "authorization failed: "+msg)
			return
		}
	}
	x.finish(XAIOAuthError, "device authorization timed out — start again")
}

func (x *XAIOAuth) finish(state XAIOAuthState, errMsg string) {
	x.mu.Lock()
	x.state = state
	x.lastErr = errMsg
	x.userCode = ""
	x.verifyURI = ""
	x.cancel = nil
	x.mu.Unlock()
	if errMsg != "" {
		x.logger.Warn("xAI SuperGrok OAuth flow failed", "error", errMsg)
	}
}

// opencodeAuthEntry matches opencode's auth.json OAuth entry shape (verified
// against a live v1.17.0 store: {"type":"oauth","access","refresh","expires"}).
type opencodeAuthEntry struct {
	Type    string `json:"type"`
	Access  string `json:"access"`
	Refresh string `json:"refresh"`
	Expires int64  `json:"expires"` // unix milliseconds
}

// writeOpencodeXAIAuth merges an xai OAuth entry into opencode's auth.json,
// preserving every other provider's credentials.
func writeOpencodeXAIAuth(path, access, refresh string, expires time.Time) error {
	if path == "" {
		return fmt.Errorf("cannot resolve opencode auth store path")
	}
	entries := map[string]json.RawMessage{}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &entries); err != nil {
			return fmt.Errorf("existing auth store is not valid JSON (refusing to overwrite): %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	entry, err := json.Marshal(opencodeAuthEntry{
		Type:    "oauth",
		Access:  access,
		Refresh: refresh,
		Expires: expires.UnixMilli(),
	})
	if err != nil {
		return err
	}
	entries["xai"] = entry

	out, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o600)
}

// hasXAIOAuthEntry reports whether the auth store holds an xai OAuth entry.
func hasXAIOAuthEntry(path string) bool {
	if path == "" {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var entries map[string]opencodeAuthEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return false
	}
	e, ok := entries["xai"]
	return ok && e.Type == "oauth" && e.Refresh != ""
}
