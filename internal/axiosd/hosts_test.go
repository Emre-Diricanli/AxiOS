package axiosd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHostMutationsPersist(t *testing.T) {
	tests := []struct {
		name         string
		method       string
		target       string
		handle       func(*Server, http.ResponseWriter, *http.Request)
		wantHosts    int
		wantActiveID string
	}{
		{
			name:   "remove host",
			method: http.MethodDelete,
			target: "/api/hosts?id=remote",
			handle: func(s *Server, w http.ResponseWriter, r *http.Request) {
				s.handleHostsRemove(w, r)
			},
			wantHosts:    1,
			wantActiveID: "local",
		},
		{
			name:   "activate host",
			method: http.MethodPost,
			target: "/api/hosts/activate?id=remote",
			handle: func(s *Server, w http.ResponseWriter, r *http.Request) {
				s.handleHostAction(w, r)
			},
			wantHosts:    2,
			wantActiveID: "remote",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "hosts.json")
			store := NewHostStore(nil)
			store.hosts = map[string]*OllamaHost{
				"local":  {ID: "local", Name: "Local", Host: "localhost", Port: 11434, Active: true},
				"remote": {ID: "remote", Name: "Remote", Host: "remote", Port: 11434},
			}
			store.activeID = "local"
			store.ConfigurePersistence(path, nil)
			s := &Server{hostStore: store, logger: testLogger()}

			req := httptest.NewRequest(tt.method, tt.target, nil)
			rec := httptest.NewRecorder()
			tt.handle(s, rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
			}

			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read persisted hosts: %v", err)
			}
			var saved hostsFile
			if err := json.Unmarshal(raw, &saved); err != nil {
				t.Fatalf("decode persisted hosts: %v", err)
			}
			if len(saved.Hosts) != tt.wantHosts {
				t.Errorf("persisted host count = %d, want %d", len(saved.Hosts), tt.wantHosts)
			}
			if saved.ActiveID != tt.wantActiveID {
				t.Errorf("persisted active ID = %q, want %q", saved.ActiveID, tt.wantActiveID)
			}
		})
	}
}

func TestHostMutationsHandleSaveErrors(t *testing.T) {
	tests := []struct {
		name   string
		method string
		target string
		handle func(*Server, http.ResponseWriter, *http.Request)
	}{
		{
			name:   "remove host",
			method: http.MethodDelete,
			target: "/api/hosts?id=remote",
			handle: func(s *Server, w http.ResponseWriter, r *http.Request) {
				s.handleHostsRemove(w, r)
			},
		},
		{
			name:   "activate host",
			method: http.MethodPost,
			target: "/api/hosts/activate?id=remote",
			handle: func(s *Server, w http.ResponseWriter, r *http.Request) {
				s.handleHostAction(w, r)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewHostStore(nil)
			store.hosts = map[string]*OllamaHost{
				"local":  {ID: "local", Name: "Local", Host: "localhost", Port: 11434, Active: true},
				"remote": {ID: "remote", Name: "Remote", Host: "remote", Port: 11434},
			}
			store.activeID = "local"
			store.ConfigurePersistence(t.TempDir(), nil)
			s := &Server{hostStore: store, logger: testLogger()}

			req := httptest.NewRequest(tt.method, tt.target, nil)
			rec := httptest.NewRecorder()
			tt.handle(s, rec, req)
			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
			}
			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if got := body["error"]; got != "failed to save host configuration" {
				t.Errorf("error = %q, want %q", got, "failed to save host configuration")
			}
		})
	}
}
