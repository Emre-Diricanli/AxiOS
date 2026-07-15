package axiosd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/axios-os/axios/internal/obsidianctl"
)

// newObsidianTestMux wires a Server (with an obsidian manager over a fresh
// data dir) into a mux built by SetupRoutes, exercising the real routing.
func newObsidianTestMux(t *testing.T, seedVault string) (*Server, *http.ServeMux) {
	t.Helper()
	s := &Server{logger: testLogger()}
	s.SetObsidian(obsidianctl.NewManager(t.TempDir(), seedVault))
	mux := http.NewServeMux()
	s.SetupRoutes(mux)
	return s, mux
}

// newObsidianTestVault stages a small vault on disk and returns its root.
func newObsidianTestVault(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"welcome.md":         "---\ntags:\n  - intro\n---\nWelcome to the vault. #start\n",
		"Work/plan.md":       "Quarterly plan with milestones.\n",
		".obsidian/app.json": "{}",
	}
	for rel, content := range files {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	return root
}

// doJSON performs a request against the mux and decodes the JSON body into a
// map (nil body responses return an empty map).
func doJSON(t *testing.T, mux *http.ServeMux, method, target, body string) (int, map[string]any) {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(method, target, reader))
	out := map[string]any{}
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("%s %s: decode body %q: %v", method, target, rec.Body, err)
		}
	}
	return rec.Code, out
}

func TestObsidianUnconfigured(t *testing.T) {
	_, mux := newObsidianTestMux(t, "")

	t.Run("status reports unconfigured", func(t *testing.T) {
		code, body := doJSON(t, mux, http.MethodGet, "/api/obsidian/status", "")
		if code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
		if body["configured"] != false {
			t.Errorf("configured = %v, want false", body["configured"])
		}
		if _, ok := body["notes"]; ok {
			t.Error("unconfigured status must omit stats")
		}
	})

	t.Run("nil manager status reports unconfigured", func(t *testing.T) {
		s := &Server{logger: testLogger()}
		mux := http.NewServeMux()
		s.SetupRoutes(mux)
		code, body := doJSON(t, mux, http.MethodGet, "/api/obsidian/status", "")
		if code != http.StatusOK || body["configured"] != false {
			t.Errorf("status = %d, configured = %v; want 200/false", code, body["configured"])
		}
	})

	t.Run("data endpoints answer 409", func(t *testing.T) {
		targets := []struct {
			method string
			target string
		}{
			{http.MethodGet, "/api/obsidian/notes"},
			{http.MethodGet, "/api/obsidian/note?path=x.md"},
			{http.MethodPut, "/api/obsidian/note"},
			{http.MethodDelete, "/api/obsidian/note?path=x.md"},
			{http.MethodGet, "/api/obsidian/search?q=x"},
		}
		for _, tt := range targets {
			code, body := doJSON(t, mux, tt.method, tt.target, `{"path":"x.md","content":"y"}`)
			if code != http.StatusConflict {
				t.Errorf("%s %s = %d, want 409", tt.method, tt.target, code)
			}
			if body["error"] != "no vault configured" {
				t.Errorf("%s %s error = %v, want %q", tt.method, tt.target, body["error"], "no vault configured")
			}
		}
	})
}

func TestObsidianVaultPut(t *testing.T) {
	root := newObsidianTestVault(t)
	_, mux := newObsidianTestMux(t, "")

	t.Run("valid vault returns full status", func(t *testing.T) {
		code, body := doJSON(t, mux, http.MethodPut, "/api/obsidian/vault", fmt.Sprintf(`{"path":%q}`, root))
		if code != http.StatusOK {
			t.Fatalf("status = %d, body %v", code, body)
		}
		if body["configured"] != true || body["vault_path"] != root {
			t.Errorf("payload = %v, want configured with vault_path %q", body, root)
		}
		if body["looks_like_vault"] != true {
			t.Errorf("looks_like_vault = %v, want true (.obsidian present)", body["looks_like_vault"])
		}
		if body["notes"] != float64(2) || body["folders"] != float64(1) {
			t.Errorf("notes/folders = %v/%v, want 2/1", body["notes"], body["folders"])
		}
		if body["name"] != filepath.Base(root) {
			t.Errorf("name = %v, want %q", body["name"], filepath.Base(root))
		}
	})

	t.Run("status reflects the persisted vault", func(t *testing.T) {
		code, body := doJSON(t, mux, http.MethodGet, "/api/obsidian/status", "")
		if code != http.StatusOK || body["configured"] != true {
			t.Errorf("status = %d, configured = %v; want 200/true", code, body["configured"])
		}
	})

	t.Run("directory without .obsidian warns via looks_like_vault", func(t *testing.T) {
		bare := t.TempDir()
		code, body := doJSON(t, mux, http.MethodPut, "/api/obsidian/vault", fmt.Sprintf(`{"path":%q}`, bare))
		if code != http.StatusOK {
			t.Fatalf("status = %d, body %v", code, body)
		}
		if body["looks_like_vault"] != false {
			t.Errorf("looks_like_vault = %v, want false", body["looks_like_vault"])
		}
	})

	t.Run("invalid paths are 400", func(t *testing.T) {
		bodies := []string{
			`{"path":""}`,
			`{"path":"relative/vault"}`,
			fmt.Sprintf(`{"path":%q}`, filepath.Join(root, "does-not-exist")),
			`not json`,
			`{}`,
		}
		for _, b := range bodies {
			if code, _ := doJSON(t, mux, http.MethodPut, "/api/obsidian/vault", b); code != http.StatusBadRequest {
				t.Errorf("PUT vault %s = %d, want 400", b, code)
			}
		}
	})

	t.Run("method check", func(t *testing.T) {
		if code, _ := doJSON(t, mux, http.MethodGet, "/api/obsidian/vault", ""); code != http.StatusMethodNotAllowed {
			t.Errorf("GET vault = %d, want 405", code)
		}
	})
}

func TestObsidianNotesList(t *testing.T) {
	root := newObsidianTestVault(t)
	_, mux := newObsidianTestMux(t, root)

	entriesOf := func(body map[string]any) []any {
		entries, _ := body["entries"].([]any)
		return entries
	}

	t.Run("root listing", func(t *testing.T) {
		code, body := doJSON(t, mux, http.MethodGet, "/api/obsidian/notes", "")
		if code != http.StatusOK {
			t.Fatalf("status = %d", code)
		}
		entries := entriesOf(body)
		if len(entries) != 2 { // Work/ + welcome.md; .obsidian hidden
			t.Fatalf("entries = %v, want 2", entries)
		}
		first := entries[0].(map[string]any)
		if first["path"] != "Work" || first["is_folder"] != true {
			t.Errorf("first entry = %v, want the Work folder", first)
		}
	})

	t.Run("recursive listing of a folder", func(t *testing.T) {
		code, body := doJSON(t, mux, http.MethodGet, "/api/obsidian/notes?folder=Work&recursive=true", "")
		if code != http.StatusOK {
			t.Fatalf("status = %d", code)
		}
		entries := entriesOf(body)
		if len(entries) != 1 || entries[0].(map[string]any)["path"] != "Work/plan.md" {
			t.Errorf("entries = %v, want only Work/plan.md", entries)
		}
	})

	t.Run("bad folder is 400, missing folder 404", func(t *testing.T) {
		if code, _ := doJSON(t, mux, http.MethodGet, "/api/obsidian/notes?folder=../etc", ""); code != http.StatusBadRequest {
			t.Errorf("traversal folder = %d, want 400", code)
		}
		if code, _ := doJSON(t, mux, http.MethodGet, "/api/obsidian/notes?folder=Nope", ""); code != http.StatusNotFound {
			t.Errorf("missing folder = %d, want 404", code)
		}
	})

	t.Run("method check", func(t *testing.T) {
		if code, _ := doJSON(t, mux, http.MethodPost, "/api/obsidian/notes", "{}"); code != http.StatusMethodNotAllowed {
			t.Errorf("POST notes = %d, want 405", code)
		}
	})
}

func TestObsidianNoteCRUD(t *testing.T) {
	root := newObsidianTestVault(t)
	_, mux := newObsidianTestMux(t, root)

	t.Run("read returns content, frontmatter, and tags", func(t *testing.T) {
		code, body := doJSON(t, mux, http.MethodGet, "/api/obsidian/note?path=welcome.md", "")
		if code != http.StatusOK {
			t.Fatalf("status = %d, body %v", code, body)
		}
		if body["path"] != "welcome.md" {
			t.Errorf("path = %v, want welcome.md", body["path"])
		}
		if !strings.Contains(body["content"].(string), "Welcome to the vault") {
			t.Errorf("content = %v, want the raw note", body["content"])
		}
		if body["frontmatter"] == nil {
			t.Error("frontmatter missing from read payload")
		}
		tags, _ := body["tags"].([]any)
		if len(tags) != 2 || tags[0] != "intro" || tags[1] != "start" {
			t.Errorf("tags = %v, want [intro start]", tags)
		}
	})

	t.Run("write defaults to overwrite for the UI", func(t *testing.T) {
		for i := 0; i < 2; i++ { // second write overwrites without error
			code, body := doJSON(t, mux, http.MethodPut, "/api/obsidian/note", `{"path":"Journal/today","content":"# Today\n"}`)
			if code != http.StatusOK || body["ok"] != true {
				t.Fatalf("write #%d = %d %v, want 200 ok", i+1, code, body)
			}
		}
		code, body := doJSON(t, mux, http.MethodGet, "/api/obsidian/note?path=Journal/today.md", "")
		if code != http.StatusOK || body["content"] != "# Today\n" {
			t.Errorf("read back = %d %v", code, body)
		}
	})

	t.Run("explicit overwrite false conflicts on existing notes", func(t *testing.T) {
		code, _ := doJSON(t, mux, http.MethodPut, "/api/obsidian/note", `{"path":"welcome.md","content":"clobber","overwrite":false}`)
		if code != http.StatusConflict {
			t.Errorf("status = %d, want 409", code)
		}
	})

	t.Run("delete answers 204 and the note is gone", func(t *testing.T) {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/obsidian/note?path=Journal/today.md", nil))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("delete status = %d, want 204", rec.Code)
		}
		if code, _ := doJSON(t, mux, http.MethodGet, "/api/obsidian/note?path=Journal/today.md", ""); code != http.StatusNotFound {
			t.Errorf("read after delete = %d, want 404", code)
		}
	})

	t.Run("validation and error mapping", func(t *testing.T) {
		tests := []struct {
			name   string
			method string
			target string
			body   string
			want   int
		}{
			{"read without path", http.MethodGet, "/api/obsidian/note", "", http.StatusBadRequest},
			{"read traversal", http.MethodGet, "/api/obsidian/note?path=../secret.md", "", http.StatusBadRequest},
			{"read hidden", http.MethodGet, "/api/obsidian/note?path=.obsidian/app.json", "", http.StatusBadRequest},
			{"read missing", http.MethodGet, "/api/obsidian/note?path=ghost.md", "", http.StatusNotFound},
			{"write without path", http.MethodPut, "/api/obsidian/note", `{"content":"x"}`, http.StatusBadRequest},
			{"write invalid json", http.MethodPut, "/api/obsidian/note", `nope`, http.StatusBadRequest},
			{"write traversal", http.MethodPut, "/api/obsidian/note", `{"path":"a/../../x","content":"y"}`, http.StatusBadRequest},
			{"delete without path", http.MethodDelete, "/api/obsidian/note", "", http.StatusBadRequest},
			{"delete missing", http.MethodDelete, "/api/obsidian/note?path=ghost.md", "", http.StatusNotFound},
			{"post not allowed", http.MethodPost, "/api/obsidian/note", `{}`, http.StatusMethodNotAllowed},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if code, _ := doJSON(t, mux, tt.method, tt.target, tt.body); code != tt.want {
					t.Errorf("%s %s = %d, want %d", tt.method, tt.target, code, tt.want)
				}
			})
		}
	})
}

func TestObsidianSearchAPI(t *testing.T) {
	root := newObsidianTestVault(t)
	_, mux := newObsidianTestMux(t, root)

	t.Run("query hits", func(t *testing.T) {
		code, body := doJSON(t, mux, http.MethodGet, "/api/obsidian/search?q=quarterly", "")
		if code != http.StatusOK {
			t.Fatalf("status = %d", code)
		}
		hits, _ := body["hits"].([]any)
		if len(hits) != 1 {
			t.Fatalf("hits = %v, want 1", hits)
		}
		hit := hits[0].(map[string]any)
		if hit["path"] != "Work/plan.md" || !strings.Contains(hit["snippet"].(string), "Quarterly") {
			t.Errorf("hit = %v, want Work/plan.md with a snippet", hit)
		}
	})

	t.Run("tag filter", func(t *testing.T) {
		code, body := doJSON(t, mux, http.MethodGet, "/api/obsidian/search?q=vault&tag=intro&limit=5", "")
		if code != http.StatusOK {
			t.Fatalf("status = %d", code)
		}
		hits, _ := body["hits"].([]any)
		if len(hits) != 1 || hits[0].(map[string]any)["path"] != "welcome.md" {
			t.Errorf("hits = %v, want only welcome.md", hits)
		}
	})

	t.Run("no query or tag is 400", func(t *testing.T) {
		if code, _ := doJSON(t, mux, http.MethodGet, "/api/obsidian/search", ""); code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", code)
		}
	})

	t.Run("method check", func(t *testing.T) {
		if code, _ := doJSON(t, mux, http.MethodPost, "/api/obsidian/search?q=x", "{}"); code != http.StatusMethodNotAllowed {
			t.Errorf("POST search = %d, want 405", code)
		}
	})
}
