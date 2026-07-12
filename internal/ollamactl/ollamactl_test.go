package ollamactl

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeOllama spins up an httptest server that fakes the Ollama management
// endpoints used by the client. Handlers are optional; missing routes 404.
func fakeOllama(t *testing.T, handlers map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for route, h := range handlers {
		mux.HandleFunc(route, h)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// unreachableURL returns a base URL that refuses connections.
func unreachableURL(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	return url
}

func TestListModels(t *testing.T) {
	tagsJSON := `{"models":[
		{"name":"llama3.1:8b","modified_at":"2026-01-02T03:04:05Z","size":4661224676,
		 "details":{"format":"gguf","family":"llama","parameter_size":"8.0B","quantization_level":"Q4_0"}},
		{"name":"bare:latest","modified_at":"2026-02-03T04:05:06Z","size":1048576,"details":{}}
	]}`

	tests := []struct {
		name     string
		handlers map[string]http.HandlerFunc
		wantErr  string
		check    func(t *testing.T, models []Model)
	}{
		{
			name: "parses installed models with details",
			handlers: map[string]http.HandlerFunc{
				"/api/tags": func(w http.ResponseWriter, r *http.Request) {
					fmt.Fprint(w, tagsJSON)
				},
				"/api/show": func(w http.ResponseWriter, r *http.Request) {
					fmt.Fprint(w, `{"details":{"family":"bert","parameter_size":"23M","quantization_level":"F16"}}`)
				},
			},
			check: func(t *testing.T, models []Model) {
				if len(models) != 2 {
					t.Fatalf("got %d models, want 2", len(models))
				}
				m := models[0]
				if m.Name != "llama3.1:8b" || m.Family != "llama" || m.Parameters != "8.0B" || m.Quantization != "Q4_0" {
					t.Errorf("unexpected first model: %+v", m)
				}
				if m.Size != 4661224676 || m.SizeHuman != "4.3 GB" {
					t.Errorf("size = %d / %q, want 4661224676 / 4.3 GB", m.Size, m.SizeHuman)
				}
				if m.Modified != "2026-01-02T03:04:05Z" {
					t.Errorf("modified = %q", m.Modified)
				}
				// Second model has no details in /api/tags: filled from /api/show.
				b := models[1]
				if b.Family != "bert" || b.Parameters != "23M" || b.Quantization != "F16" {
					t.Errorf("show fallback not applied: %+v", b)
				}
			},
		},
		{
			name: "show fallback failure leaves details empty",
			handlers: map[string]http.HandlerFunc{
				"/api/tags": func(w http.ResponseWriter, r *http.Request) {
					fmt.Fprint(w, `{"models":[{"name":"bare:latest","size":10,"details":{}}]}`)
				},
			},
			check: func(t *testing.T, models []Model) {
				if len(models) != 1 {
					t.Fatalf("got %d models, want 1", len(models))
				}
				if models[0].Family != "" || models[0].Parameters != "" {
					t.Errorf("expected empty details, got %+v", models[0])
				}
			},
		},
		{
			name: "empty list",
			handlers: map[string]http.HandlerFunc{
				"/api/tags": func(w http.ResponseWriter, r *http.Request) {
					fmt.Fprint(w, `{"models":[]}`)
				},
			},
			check: func(t *testing.T, models []Model) {
				if len(models) != 0 {
					t.Fatalf("got %d models, want 0", len(models))
				}
			},
		},
		{
			name: "non-200 response",
			handlers: map[string]http.HandlerFunc{
				"/api/tags": func(w http.ResponseWriter, r *http.Request) {
					http.Error(w, "boom", http.StatusInternalServerError)
				},
			},
			wantErr: "Ollama returned 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := fakeOllama(t, tt.handlers)
			models, err := New(srv.URL, srv.Client()).ListModels()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ListModels: %v", err)
			}
			tt.check(t, models)
		})
	}
}

func TestListModelsUnreachable(t *testing.T) {
	_, err := New(unreachableURL(t), nil).ListModels()
	if err == nil || !strings.Contains(err.Error(), "failed to reach Ollama") {
		t.Fatalf("err = %v, want failed-to-reach error", err)
	}
}

func TestListModelNames(t *testing.T) {
	srv := fakeOllama(t, map[string]http.HandlerFunc{
		"/api/tags": func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"models":[{"name":"a"},{"name":"b"}]}`)
		},
	})
	names, err := New(srv.URL, srv.Client()).ListModelNames()
	if err != nil {
		t.Fatalf("ListModelNames: %v", err)
	}
	if len(names) != 2 || names[0] != "a" || names[1] != "b" {
		t.Fatalf("names = %v, want [a b]", names)
	}
}

func TestModelInfo(t *testing.T) {
	tests := []struct {
		name     string
		handlers map[string]http.HandlerFunc
		wantErr  string
	}{
		{
			name: "success",
			handlers: map[string]http.HandlerFunc{
				"/api/show": func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodPost {
						t.Errorf("method = %s, want POST", r.Method)
					}
					var req map[string]string
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req["name"] != "llama3.1:8b" {
						t.Errorf("bad request body: %v / %v", req, err)
					}
					fmt.Fprint(w, `{"license":"MIT","modelfile":"FROM llama","parameters":"stop \"<|eot|>\"",
						"template":"{{ .Prompt }}",
						"details":{"format":"gguf","family":"llama","families":["llama"],
						           "parameter_size":"8.0B","quantization_level":"Q4_0"}}`)
				},
			},
		},
		{
			name: "model not found",
			handlers: map[string]http.HandlerFunc{
				"/api/show": func(w http.ResponseWriter, r *http.Request) {
					http.Error(w, `{"error":"model not found"}`, http.StatusNotFound)
				},
			},
			wantErr: "Ollama returned 404",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := fakeOllama(t, tt.handlers)
			info, err := New(srv.URL, srv.Client()).ModelInfo("llama3.1:8b")
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ModelInfo: %v", err)
			}
			if info.License != "MIT" || info.Details.Family != "llama" || info.Details.ParameterSize != "8.0B" {
				t.Errorf("unexpected info: %+v", info)
			}
			if len(info.Details.Families) != 1 || info.Details.Families[0] != "llama" {
				t.Errorf("families = %v", info.Details.Families)
			}
		})
	}
}

func TestPullModel(t *testing.T) {
	tests := []struct {
		name         string
		handlers     map[string]http.HandlerFunc
		baseURL      string // overrides fake server when set
		wantErr      string
		wantProgress int
		checkLast    func(t *testing.T, last PullProgress)
	}{
		{
			name: "success streams progress",
			handlers: map[string]http.HandlerFunc{
				"/api/pull": func(w http.ResponseWriter, r *http.Request) {
					var req pullRequest
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name != "llama3.1:8b" || !req.Stream {
						t.Errorf("bad pull request: %+v / %v", req, err)
					}
					fmt.Fprintln(w, `{"status":"pulling manifest"}`)
					fmt.Fprintln(w, `{"status":"downloading","digest":"sha256:abc","total":100,"completed":50}`)
					fmt.Fprintln(w, `not-json`) // malformed lines are skipped
					fmt.Fprintln(w, `{"status":"success","total":100,"completed":100}`)
				},
			},
			wantProgress: 3,
			checkLast: func(t *testing.T, last PullProgress) {
				if last.Status != "success" || last.Percent != 100 {
					t.Errorf("last progress = %+v", last)
				}
			},
		},
		{
			name: "registry error",
			handlers: map[string]http.HandlerFunc{
				"/api/pull": func(w http.ResponseWriter, r *http.Request) {
					http.Error(w, `{"error":"pull model manifest: file does not exist"}`, http.StatusInternalServerError)
				},
			},
			wantErr: "Ollama returned 500",
		},
		{
			name:    "unreachable server",
			baseURL: "unreachable",
			wantErr: "failed to reach Ollama",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c *Client
			if tt.baseURL == "unreachable" {
				c = New(unreachableURL(t), nil)
			} else {
				srv := fakeOllama(t, tt.handlers)
				c = New(srv.URL, srv.Client())
			}

			var got []PullProgress
			err := c.PullModel("llama3.1:8b", func(p PullProgress) { got = append(got, p) })
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("PullModel: %v", err)
			}
			if len(got) != tt.wantProgress {
				t.Fatalf("got %d progress updates, want %d: %+v", len(got), tt.wantProgress, got)
			}
			if got[1].Percent != 50 {
				t.Errorf("mid progress percent = %v, want 50", got[1].Percent)
			}
			tt.checkLast(t, got[len(got)-1])
		})
	}
}

func TestPullModelNilProgress(t *testing.T) {
	srv := fakeOllama(t, map[string]http.HandlerFunc{
		"/api/pull": func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, `{"status":"success"}`)
		},
	})
	if err := New(srv.URL, srv.Client()).PullModel("x", nil); err != nil {
		t.Fatalf("PullModel with nil progress: %v", err)
	}
}

func TestDeleteModel(t *testing.T) {
	tests := []struct {
		name     string
		handlers map[string]http.HandlerFunc
		wantErr  string
	}{
		{
			name: "success",
			handlers: map[string]http.HandlerFunc{
				"/api/delete": func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodDelete {
						t.Errorf("method = %s, want DELETE", r.Method)
					}
					var req deleteRequest
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name != "llama3.1:8b" {
						t.Errorf("bad delete request: %+v / %v", req, err)
					}
					w.WriteHeader(http.StatusOK)
				},
			},
		},
		{
			name: "model not found",
			handlers: map[string]http.HandlerFunc{
				"/api/delete": func(w http.ResponseWriter, r *http.Request) {
					http.Error(w, `{"error":"model not found"}`, http.StatusNotFound)
				},
			},
			wantErr: "Ollama returned 404",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := fakeOllama(t, tt.handlers)
			err := New(srv.URL, srv.Client()).DeleteModel("llama3.1:8b")
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("DeleteModel: %v", err)
			}
		})
	}
}

func TestPingAndVersion(t *testing.T) {
	tests := []struct {
		name        string
		handlers    map[string]http.HandlerFunc
		unreachable bool
		wantPingErr string
		wantVersion string
		wantVerErr  string
	}{
		{
			name: "healthy server",
			handlers: map[string]http.HandlerFunc{
				"/api/tags": func(w http.ResponseWriter, r *http.Request) {
					fmt.Fprint(w, `{"models":[]}`)
				},
				"/api/version": func(w http.ResponseWriter, r *http.Request) {
					fmt.Fprint(w, `{"version":"0.5.7"}`)
				},
			},
			wantVersion: "0.5.7",
		},
		{
			name: "ping non-200",
			handlers: map[string]http.HandlerFunc{
				"/api/tags": func(w http.ResponseWriter, r *http.Request) {
					http.Error(w, "nope", http.StatusServiceUnavailable)
				},
			},
			wantPingErr: "ollama returned 503",
			wantVerErr:  "Ollama returned 404", // no /api/version route registered
		},
		{
			name:        "unreachable",
			unreachable: true,
			wantPingErr: "ollama unreachable",
			wantVerErr:  "failed to reach Ollama",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c *Client
			if tt.unreachable {
				c = New(unreachableURL(t), nil)
			} else {
				srv := fakeOllama(t, tt.handlers)
				c = New(srv.URL, srv.Client())
			}

			err := c.Ping()
			if tt.wantPingErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantPingErr) {
					t.Fatalf("Ping err = %v, want containing %q", err, tt.wantPingErr)
				}
			} else if err != nil {
				t.Fatalf("Ping: %v", err)
			}

			v, err := c.Version()
			if tt.wantVerErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantVerErr) {
					t.Fatalf("Version err = %v, want containing %q", err, tt.wantVerErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Version: %v", err)
			}
			if v != tt.wantVersion {
				t.Errorf("version = %q, want %q", v, tt.wantVersion)
			}
		})
	}
}

func TestBaseURLTrimsTrailingSlash(t *testing.T) {
	c := New("http://localhost:11434/", nil)
	if got := c.BaseURL(); got != "http://localhost:11434" {
		t.Errorf("BaseURL = %q, want trailing slash trimmed", got)
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{512, "512 B"},
		{2048, "2.0 KB"},
		{5 * 1024 * 1024, "5.0 MB"},
		{4661224676, "4.3 GB"},
		{2199023255552, "2.0 TB"},
	}
	for _, tt := range tests {
		if got := FormatSize(tt.in); got != tt.want {
			t.Errorf("FormatSize(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
