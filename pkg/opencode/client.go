// Package opencode is an HTTP + SSE client for a locally running opencode
// server (`opencode serve`), pinned to the legacy API surface verified
// against opencode v1.17.0 (NOT the newer /api/* routes).
//
// The package speaks the wire protocol only; process supervision lives in
// internal/axiosd. Every request authenticates with HTTP Basic auth as user
// "opencode" and the server password (OPENCODE_SERVER_PASSWORD).
package opencode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/axios-os/axios/pkg/logging"
)

// basicAuthUser is the fixed username opencode expects for HTTP Basic auth.
const basicAuthUser = "opencode"

// Permission responses accepted by ReplyPermission.
const (
	PermissionOnce   = "once"
	PermissionAlways = "always"
	PermissionReject = "reject"
)

// Client talks to a running opencode server over HTTP. It is safe for
// concurrent use.
type Client struct {
	baseURL  string
	password string
	hc       *http.Client
	log      *slog.Logger
}

// NewClient returns a client for the opencode server at baseURL (e.g.
// "http://127.0.0.1:4097"). password is sent as HTTP Basic auth
// ("opencode:<password>") on every request. hc is the injected HTTP client;
// nil falls back to a client with a 30s timeout.
func NewClient(baseURL, password string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		baseURL:  strings.TrimRight(baseURL, "/"),
		password: password,
		hc:       hc,
		log:      logging.New("opencode-client"),
	}
}

// APIError is returned when the opencode server responds with a non-2xx
// status. Body holds up to 4KB of the response for diagnostics.
type APIError struct {
	StatusCode int
	Method     string
	Path       string
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("opencode: %s %s: status %d: %s", e.Method, e.Path, e.StatusCode, e.Body)
}

// Health checks GET /global/health; nil means the server is up and the
// password is accepted.
func (c *Client) Health() error {
	return c.do(http.MethodGet, "/global/health", nil, nil, nil)
}

// CreateSession creates a new session via POST /session?directory=<dir>.
// dir is the working directory for the session (empty = server default);
// title is an optional human-readable label.
func (c *Client) CreateSession(dir, title string) (*Session, error) {
	q := url.Values{}
	if dir != "" {
		q.Set("directory", dir)
	}
	var s Session
	if err := c.do(http.MethodPost, "/session", q, createSessionRequest{Title: title}, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// PromptAsync submits a text prompt to POST /session/{id}/prompt_async and
// returns immediately; progress and completion arrive on the /event stream
// (message.part.delta, session.idle, session.error). model selects the
// provider/model for this prompt; nil uses opencode's own default.
func (c *Client) PromptAsync(sessionID string, model *ModelRef, text string) error {
	body := promptRequest{
		Model: model,
		Parts: []textPart{{Type: "text", Text: text}},
	}
	return c.do(http.MethodPost, "/session/"+url.PathEscape(sessionID)+"/prompt_async", nil, body, nil)
}

// Messages returns the session transcript from GET /session/{id}/message.
func (c *Client) Messages(sessionID string) ([]Message, error) {
	var msgs []Message
	if err := c.do(http.MethodGet, "/session/"+url.PathEscape(sessionID)+"/message", nil, nil, &msgs); err != nil {
		return nil, err
	}
	return msgs, nil
}

// ReplyPermission answers a permission.asked event via
// POST /session/{sid}/permissions/{pid}. response must be PermissionOnce,
// PermissionAlways or PermissionReject.
func (c *Client) ReplyPermission(sessionID, permID, response string) error {
	switch response {
	case PermissionOnce, PermissionAlways, PermissionReject:
	default:
		return fmt.Errorf("opencode: invalid permission response %q (want %q, %q or %q)",
			response, PermissionOnce, PermissionAlways, PermissionReject)
	}
	return c.do(http.MethodPost,
		"/session/"+url.PathEscape(sessionID)+"/permissions/"+url.PathEscape(permID),
		nil, permissionReply{Response: response}, nil)
}

// Abort cancels the session's in-flight work via POST /session/{id}/abort.
func (c *Client) Abort(sessionID string) error {
	return c.do(http.MethodPost, "/session/"+url.PathEscape(sessionID)+"/abort", nil, nil, nil)
}

// DeleteSession removes a session via DELETE /session/{id}.
func (c *Client) DeleteSession(sessionID string) error {
	return c.do(http.MethodDelete, "/session/"+url.PathEscape(sessionID), nil, nil, nil)
}

// Status returns per-session status from GET /session/status, keyed by
// session ID.
func (c *Client) Status() (map[string]SessionStatus, error) {
	var st map[string]SessionStatus
	if err := c.do(http.MethodGet, "/session/status", nil, nil, &st); err != nil {
		return nil, err
	}
	return st, nil
}

// Diff returns the session's accumulated file changes from
// GET /session/{id}/diff.
func (c *Client) Diff(sessionID string) ([]FileDiff, error) {
	var diffs []FileDiff
	if err := c.do(http.MethodGet, "/session/"+url.PathEscape(sessionID)+"/diff", nil, nil, &diffs); err != nil {
		return nil, err
	}
	return diffs, nil
}

// Providers returns the providers and models currently usable by the server
// (i.e. with working credentials) from GET /config/providers. Verified shape
// in v1.17.0: {"providers":[{"id":"xai","models":{"grok-4.5":{...},...}}],"default":...}.
func (c *Client) Providers() ([]ProviderModels, error) {
	var resp struct {
		Providers []struct {
			ID     string                     `json:"id"`
			Models map[string]json.RawMessage `json:"models"`
		} `json:"providers"`
	}
	if err := c.do(http.MethodGet, "/config/providers", nil, nil, &resp); err != nil {
		return nil, err
	}
	out := make([]ProviderModels, 0, len(resp.Providers))
	for _, p := range resp.Providers {
		models := make([]string, 0, len(p.Models))
		for name := range p.Models {
			models = append(models, name)
		}
		sort.Strings(models)
		out = append(out, ProviderModels{ID: p.ID, Models: models})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// ReplyQuestion answers a question.asked event via
// POST /question/{requestID}/reply. answers holds, per question in order,
// the selected option labels (or a free-text answer when the question
// allows custom input).
func (c *Client) ReplyQuestion(requestID string, answers [][]string) error {
	if answers == nil {
		answers = [][]string{}
	}
	return c.do(http.MethodPost, "/question/"+url.PathEscape(requestID)+"/reply",
		nil, questionReply{Answers: answers}, nil)
}

// RejectQuestion declines a question.asked event via
// POST /question/{requestID}/reject.
func (c *Client) RejectQuestion(requestID string) error {
	return c.do(http.MethodPost, "/question/"+url.PathEscape(requestID)+"/reject", nil, nil, nil)
}

// Request bodies (legacy v1.17.0 shapes).

type createSessionRequest struct {
	Title string `json:"title,omitempty"`
}

type promptRequest struct {
	Model *ModelRef  `json:"model,omitempty"`
	Parts []textPart `json:"parts"`
}

type textPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type permissionReply struct {
	Response string `json:"response"`
}

type questionReply struct {
	Answers [][]string `json:"answers"`
}

// do performs one authenticated JSON request. body is marshaled when non-nil;
// out, when non-nil, receives the decoded 2xx response body (unknown JSON
// fields are ignored).
func (c *Client) do(method, path string, query url.Values, body, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("opencode: encode %s %s body: %w", method, path, err)
		}
		reader = bytes.NewReader(data)
	}

	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequest(method, u, reader)
	if err != nil {
		return fmt.Errorf("opencode: build %s %s request: %w", method, path, err)
	}
	req.SetBasicAuth(basicAuthUser, c.password)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("opencode: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return &APIError{
			StatusCode: resp.StatusCode,
			Method:     method,
			Path:       path,
			Body:       strings.TrimSpace(string(snippet)),
		}
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("opencode: decode %s %s response: %w", method, path, err)
		}
		return nil
	}
	// Drain so the connection can be reused.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	return nil
}
