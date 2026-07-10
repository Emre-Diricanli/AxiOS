package opencode_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/axios-os/axios/pkg/opencode"
)

const testPassword = "test-password-0123456789abcdef"

// recordedRequest captures what the fake server saw, guarded for -race.
type recordedRequest struct {
	mu          sync.Mutex
	seen        bool
	method      string
	path        string
	query       url.Values
	body        string
	contentType string
	user        string
	pass        string
	authOK      bool
}

func (r *recordedRequest) record(req *http.Request) {
	body, _ := io.ReadAll(req.Body)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen = true
	r.method = req.Method
	r.path = req.URL.Path
	r.query = req.URL.Query()
	r.body = string(body)
	r.contentType = req.Header.Get("Content-Type")
	r.user, r.pass, r.authOK = req.BasicAuth()
}

// jsonEqual compares two JSON documents structurally.
func jsonEqual(t *testing.T, want, got string) bool {
	t.Helper()
	var w, g any
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("bad want JSON %q: %v", want, err)
	}
	if err := json.Unmarshal([]byte(got), &g); err != nil {
		return false
	}
	return reflect.DeepEqual(w, g)
}

func TestClientMethods(t *testing.T) {
	tests := []struct {
		name       string
		call       func(t *testing.T, c *opencode.Client)
		wantMethod string
		wantPath   string
		wantQuery  url.Values
		wantBody   string // JSON; "" means no request body expected
		respBody   string
	}{
		{
			name: "Health",
			call: func(t *testing.T, c *opencode.Client) {
				if err := c.Health(); err != nil {
					t.Fatalf("Health() error: %v", err)
				}
			},
			wantMethod: http.MethodGet,
			wantPath:   "/global/health",
			respBody:   `{"healthy":true,"version":"1.17.0"}`,
		},
		{
			name: "CreateSession",
			call: func(t *testing.T, c *opencode.Client) {
				s, err := c.CreateSession("/home/axios/axios-workspace", "fix the build")
				if err != nil {
					t.Fatalf("CreateSession() error: %v", err)
				}
				if s.ID != "ses_01ABC" {
					t.Errorf("session ID = %q, want %q", s.ID, "ses_01ABC")
				}
				if s.Directory != "/home/axios/axios-workspace" {
					t.Errorf("session Directory = %q", s.Directory)
				}
				if s.Time.Created != 1752130000000 {
					t.Errorf("session Time.Created = %v", s.Time.Created)
				}
			},
			wantMethod: http.MethodPost,
			wantPath:   "/session",
			wantQuery:  url.Values{"directory": {"/home/axios/axios-workspace"}},
			wantBody:   `{"title":"fix the build"}`,
			respBody: `{"id":"ses_01ABC","projectID":"prj_01","directory":"/home/axios/axios-workspace",` +
				`"title":"fix the build","version":"1.17.0",` +
				`"time":{"created":1752130000000,"updated":1752130000000},"unknownField":42}`,
		},
		{
			name: "CreateSession empty dir and title",
			call: func(t *testing.T, c *opencode.Client) {
				if _, err := c.CreateSession("", ""); err != nil {
					t.Fatalf("CreateSession() error: %v", err)
				}
			},
			wantMethod: http.MethodPost,
			wantPath:   "/session",
			wantBody:   `{}`,
			respBody:   `{"id":"ses_02","time":{"created":1,"updated":1}}`,
		},
		{
			name: "PromptAsync with model",
			call: func(t *testing.T, c *opencode.Client) {
				m := &opencode.ModelRef{ProviderID: "anthropic", ModelID: "claude-sonnet-4-5"}
				if err := c.PromptAsync("ses_01ABC", m, "add table-driven tests"); err != nil {
					t.Fatalf("PromptAsync() error: %v", err)
				}
			},
			wantMethod: http.MethodPost,
			wantPath:   "/session/ses_01ABC/prompt_async",
			wantBody: `{"model":{"providerID":"anthropic","modelID":"claude-sonnet-4-5"},` +
				`"parts":[{"type":"text","text":"add table-driven tests"}]}`,
			respBody: `{}`,
		},
		{
			name: "PromptAsync without model omits model field",
			call: func(t *testing.T, c *opencode.Client) {
				if err := c.PromptAsync("ses_01ABC", nil, "hello"); err != nil {
					t.Fatalf("PromptAsync() error: %v", err)
				}
			},
			wantMethod: http.MethodPost,
			wantPath:   "/session/ses_01ABC/prompt_async",
			wantBody:   `{"parts":[{"type":"text","text":"hello"}]}`,
			respBody:   `{}`,
		},
		{
			name: "Messages",
			call: func(t *testing.T, c *opencode.Client) {
				msgs, err := c.Messages("ses_01ABC")
				if err != nil {
					t.Fatalf("Messages() error: %v", err)
				}
				if len(msgs) != 2 {
					t.Fatalf("len(msgs) = %d, want 2", len(msgs))
				}
				info := msgs[0].Info
				if info.Role != "assistant" || info.ID != "msg_01" {
					t.Errorf("info = %+v", info)
				}
				if info.Cost != 0.0123 {
					t.Errorf("Cost = %v, want 0.0123", info.Cost)
				}
				if info.Tokens.Input != 100 || info.Tokens.Output != 50 || info.Tokens.Cache.Read != 10 {
					t.Errorf("Tokens = %+v", info.Tokens)
				}
				if info.Time.Created != 1752130000000 || info.Time.Completed != 1752130009000 {
					t.Errorf("Time = %+v", info.Time)
				}
				if info.Error != nil {
					t.Errorf("Error = %s, want nil", info.Error)
				}
				if len(msgs[0].Parts) != 1 || msgs[0].Parts[0].Type != "text" || msgs[0].Parts[0].Text != "done" {
					t.Errorf("Parts = %+v", msgs[0].Parts)
				}
				if msgs[1].Info.Error == nil || !strings.Contains(string(msgs[1].Info.Error), "ProviderAuthError") {
					t.Errorf("msgs[1].Info.Error = %s, want raw ProviderAuthError payload", msgs[1].Info.Error)
				}
			},
			wantMethod: http.MethodGet,
			wantPath:   "/session/ses_01ABC/message",
			respBody: `[` +
				`{"info":{"id":"msg_01","sessionID":"ses_01ABC","role":"assistant",` +
				`"time":{"created":1752130000000,"completed":1752130009000},"cost":0.0123,` +
				`"tokens":{"input":100,"output":50,"reasoning":0,"cache":{"read":10,"write":5}},` +
				`"providerID":"anthropic","modelID":"claude-sonnet-4-5","mode":"build"},` +
				`"parts":[{"id":"prt_01","sessionID":"ses_01ABC","messageID":"msg_01","type":"text","text":"done"}]},` +
				`{"info":{"id":"msg_02","sessionID":"ses_01ABC","role":"assistant",` +
				`"time":{"created":1752130010000},"tokens":{"input":0,"output":0,"reasoning":0,"cache":{"read":0,"write":0}},` +
				`"error":{"name":"ProviderAuthError","data":{"message":"invalid api key"}}},` +
				`"parts":[]}` +
				`]`,
		},
		{
			name: "ReplyPermission",
			call: func(t *testing.T, c *opencode.Client) {
				if err := c.ReplyPermission("ses_01ABC", "per_01XYZ", opencode.PermissionOnce); err != nil {
					t.Fatalf("ReplyPermission() error: %v", err)
				}
			},
			wantMethod: http.MethodPost,
			wantPath:   "/session/ses_01ABC/permissions/per_01XYZ",
			wantBody:   `{"response":"once"}`,
			respBody:   `true`,
		},
		{
			name: "Abort",
			call: func(t *testing.T, c *opencode.Client) {
				if err := c.Abort("ses_01ABC"); err != nil {
					t.Fatalf("Abort() error: %v", err)
				}
			},
			wantMethod: http.MethodPost,
			wantPath:   "/session/ses_01ABC/abort",
			respBody:   `true`,
		},
		{
			name: "DeleteSession",
			call: func(t *testing.T, c *opencode.Client) {
				if err := c.DeleteSession("ses_01ABC"); err != nil {
					t.Fatalf("DeleteSession() error: %v", err)
				}
			},
			wantMethod: http.MethodDelete,
			wantPath:   "/session/ses_01ABC",
			respBody:   `true`,
		},
		{
			name: "Status",
			call: func(t *testing.T, c *opencode.Client) {
				st, err := c.Status()
				if err != nil {
					t.Fatalf("Status() error: %v", err)
				}
				want := map[string]opencode.SessionStatus{
					"ses_01ABC": {Type: "busy"},
					"ses_02DEF": {Type: "idle"},
				}
				if !reflect.DeepEqual(st, want) {
					t.Errorf("Status() = %+v, want %+v", st, want)
				}
			},
			wantMethod: http.MethodGet,
			wantPath:   "/session/status",
			respBody:   `{"ses_01ABC":{"type":"busy"},"ses_02DEF":{"type":"idle"}}`,
		},
		{
			name: "Diff",
			call: func(t *testing.T, c *opencode.Client) {
				diffs, err := c.Diff("ses_01ABC")
				if err != nil {
					t.Fatalf("Diff() error: %v", err)
				}
				want := []opencode.FileDiff{
					{File: "cmd/axiosd/main.go", Patch: "@@ -1 +1 @@", Additions: 3, Deletions: 1, Status: "modified"},
				}
				if !reflect.DeepEqual(diffs, want) {
					t.Errorf("Diff() = %+v, want %+v", diffs, want)
				}
			},
			wantMethod: http.MethodGet,
			wantPath:   "/session/ses_01ABC/diff",
			respBody:   `[{"file":"cmd/axiosd/main.go","patch":"@@ -1 +1 @@","additions":3,"deletions":1,"status":"modified"}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recordedRequest{}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				rec.record(r)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, tt.respBody)
			}))
			defer srv.Close()

			c := opencode.NewClient(srv.URL, testPassword, srv.Client())
			tt.call(t, c)

			rec.mu.Lock()
			defer rec.mu.Unlock()
			if !rec.seen {
				t.Fatal("server received no request")
			}
			if !rec.authOK || rec.user != "opencode" || rec.pass != testPassword {
				t.Errorf("basic auth = (%q, %q, ok=%v), want (opencode, %q, ok=true)",
					rec.user, rec.pass, rec.authOK, testPassword)
			}
			if rec.method != tt.wantMethod {
				t.Errorf("method = %q, want %q", rec.method, tt.wantMethod)
			}
			if rec.path != tt.wantPath {
				t.Errorf("path = %q, want %q", rec.path, tt.wantPath)
			}
			wantQuery := tt.wantQuery
			if wantQuery == nil {
				wantQuery = url.Values{}
			}
			if !reflect.DeepEqual(rec.query, wantQuery) {
				t.Errorf("query = %v, want %v", rec.query, wantQuery)
			}
			if tt.wantBody == "" {
				if rec.body != "" {
					t.Errorf("unexpected request body: %q", rec.body)
				}
			} else {
				if rec.contentType != "application/json" {
					t.Errorf("Content-Type = %q, want application/json", rec.contentType)
				}
				if !jsonEqual(t, tt.wantBody, rec.body) {
					t.Errorf("body = %s, want %s", rec.body, tt.wantBody)
				}
			}
		})
	}
}

func TestClientTrimsTrailingSlash(t *testing.T) {
	rec := &recordedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		_, _ = io.WriteString(w, `{"healthy":true}`)
	}))
	defer srv.Close()

	c := opencode.NewClient(srv.URL+"/", testPassword, srv.Client())
	if err := c.Health(); err != nil {
		t.Fatalf("Health() error: %v", err)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.path != "/global/health" {
		t.Errorf("path = %q, want /global/health", rec.path)
	}
}

func TestClientNonSuccessStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
	}))
	defer srv.Close()

	c := opencode.NewClient(srv.URL, testPassword, srv.Client())
	_, err := c.Messages("ses_missing")
	if err == nil {
		t.Fatal("Messages() error = nil, want *APIError")
	}
	var apiErr *opencode.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T (%v), want *APIError", err, err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want 404", apiErr.StatusCode)
	}
	if !strings.Contains(apiErr.Body, "session not found") {
		t.Errorf("Body = %q, want it to contain the server error", apiErr.Body)
	}
	if !strings.Contains(apiErr.Error(), "/session/ses_missing/message") {
		t.Errorf("Error() = %q, want it to mention the path", apiErr.Error())
	}
}

func TestReplyPermissionRejectsInvalidResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be reached for an invalid response value")
	}))
	defer srv.Close()

	c := opencode.NewClient(srv.URL, testPassword, srv.Client())
	if err := c.ReplyPermission("ses_01", "per_01", "maybe"); err == nil {
		t.Fatal("ReplyPermission(..., \"maybe\") error = nil, want validation error")
	}
}
