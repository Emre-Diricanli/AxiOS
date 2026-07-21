package axiosd

import (
	"net/http/httptest"
	"testing"
)

func TestModelClientForRequestTargetsExplicitHost(t *testing.T) {
	store := NewHostStore(nil)
	store.hosts["local"] = &OllamaHost{ID: "local", Host: "127.0.0.1", Port: 11434, Status: "online"}
	store.hosts["remote"] = &OllamaHost{ID: "remote", Host: "100.64.0.10", Port: 22434, Status: "online"}
	store.activeID = "local"
	server := &Server{hostStore: store, ollama: NewOllamaClient("127.0.0.1", 11434)}

	request := httptest.NewRequest("GET", "/api/models/installed?host_id=remote", nil)
	client, err := server.modelClientForRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	if got := client.BaseURL(); got != "http://100.64.0.10:22434" {
		t.Fatalf("base URL = %q, want remote host", got)
	}
	if store.GetActive().ID != "local" {
		t.Fatal("browsing a remote marketplace changed the active inference host")
	}
}

func TestModelClientForRequestRejectsOfflineHost(t *testing.T) {
	store := NewHostStore(nil)
	store.hosts["remote"] = &OllamaHost{ID: "remote", Host: "100.64.0.10", Port: 11434, Status: "offline"}
	server := &Server{hostStore: store}
	request := httptest.NewRequest("GET", "/api/models/installed?host_id=remote", nil)
	if _, err := server.modelClientForRequest(request); err == nil {
		t.Fatal("expected offline host to be rejected")
	}
}
