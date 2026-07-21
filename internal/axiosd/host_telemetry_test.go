package axiosd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/axios-os/axios/pkg/secrets"
)

func testServerAddress(t *testing.T, rawURL string) (string, int) {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatal(err)
	}
	return parsed.Hostname(), port
}

func TestCollectHostTelemetryFromAgent(t *testing.T) {
	const telemetryToken = "test-token-that-is-long-enough-for-auth"
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/version":
			json.NewEncoder(w).Encode(map[string]string{"version": "0.9.1"})
		case "/api/ps":
			json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{
				"name": "qwen:27b", "size": 18_000_000_000, "size_vram": 17_000_000_000,
			}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ollama.Close()

	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+telemetryToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/api/system/stats" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(SystemStats{
			Hostname: "gpu-node",
			CPU:      CPUStats{Model: "Test CPU", Cores: 16, Threads: 32},
			Memory:   MemStats{TotalBytes: 64 << 30},
			GPU:      []GPUStats{{Index: 0, Name: "RTX Test", MemoryTotalBytes: 24 << 30}},
		})
	}))
	defer agent.Close()

	hostName, ollamaPort := testServerAddress(t, ollama.URL)
	_, agentPort := testServerAddress(t, agent.URL)
	telemetry := collectHostTelemetry(&OllamaHost{
		ID: "remote", Name: "Remote", Host: hostName, Port: ollamaPort, TelemetryPort: agentPort, TelemetryToken: telemetryToken,
	})

	if telemetry.Source != "agent" || telemetry.System == nil {
		t.Fatalf("telemetry = %+v, want full agent telemetry", telemetry)
	}
	if telemetry.System.Hostname != "gpu-node" || len(telemetry.System.GPU) != 1 {
		t.Fatalf("system = %+v", telemetry.System)
	}
	if telemetry.OllamaVersion != "0.9.1" || len(telemetry.RunningModels) != 1 {
		t.Fatalf("runtime telemetry = %+v", telemetry)
	}
}

func TestCollectHostTelemetryRejectsMissingToken(t *testing.T) {
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(SystemStats{Hostname: "should-not-load"})
	}))
	defer agent.Close()
	hostName, agentPort := testServerAddress(t, agent.URL)
	telemetry := collectHostTelemetry(&OllamaHost{ID: "remote", Host: hostName, Port: 1, TelemetryPort: agentPort})
	if telemetry.System != nil || !strings.Contains(telemetry.Message, "token is required") {
		t.Fatalf("telemetry = %+v", telemetry)
	}
}

func TestCollectHostTelemetryFallsBackToOllama(t *testing.T) {
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/version":
			json.NewEncoder(w).Encode(map[string]string{"version": "0.9.2"})
		case "/api/ps":
			json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{
				"name": "qwen:27b", "size": 18_000_000_000, "size_vram": 12_000_000_000,
			}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ollama.Close()

	hostName, ollamaPort := testServerAddress(t, ollama.URL)
	telemetry := collectHostTelemetry(&OllamaHost{
		ID: "remote", Name: "Remote", Host: hostName, Port: ollamaPort, TelemetryPort: 1,
	})

	if telemetry.Source != "ollama" || telemetry.System != nil {
		t.Fatalf("telemetry = %+v, want Ollama fallback", telemetry)
	}
	if telemetry.OllamaVersion != "0.9.2" || len(telemetry.RunningModels) != 1 {
		t.Fatalf("runtime telemetry = %+v", telemetry)
	}
	if telemetry.Message == "" {
		t.Fatal("expected setup guidance when full telemetry is unavailable")
	}
}

func TestHostStoreUpdatesTelemetryPort(t *testing.T) {
	store := NewHostStore(nil)
	store.hosts["remote"] = &OllamaHost{ID: "remote", TelemetryPort: 3000}
	if err := store.SetTelemetryPort("remote", 3210); err != nil {
		t.Fatal(err)
	}
	if got := store.GetHosts()[0].TelemetryPort; got != 3210 {
		t.Fatalf("telemetry port = %d, want 3210", got)
	}
	if err := store.SetTelemetryPort("remote", 70000); err == nil {
		t.Fatal("expected invalid port to fail")
	}
}

func TestHostStoreEncryptsTelemetryTokens(t *testing.T) {
	directory := t.TempDir()
	secretStore, err := secrets.NewStore(filepath.Join(directory, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "hosts.json")
	store := NewHostStore(nil)
	store.ConfigurePersistence(path, secretStore)
	store.hosts["remote"] = &OllamaHost{ID: "remote", Name: "Remote", TelemetryToken: "remote-secret-token", HasTelemetryToken: true}
	if err := store.Save(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "remote-secret-token") || !strings.Contains(string(raw), "axsec1:") {
		t.Fatalf("token was not encrypted: %s", raw)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Fatalf("hosts file mode = %o, want 600", mode)
	}

	reloaded := NewHostStore(nil)
	reloaded.ConfigurePersistence(path, secretStore)
	if err := reloaded.LoadFromFile(path); err != nil {
		t.Fatal(err)
	}
	host := reloaded.hosts["remote"]
	if host == nil || host.TelemetryToken != "remote-secret-token" || !host.HasTelemetryToken {
		t.Fatalf("reloaded host = %+v", host)
	}
	encoded, err := json.Marshal(host)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "remote-secret-token") {
		t.Fatal("telemetry token leaked through host JSON")
	}
}

func TestHostStoreRejectsShortTelemetryToken(t *testing.T) {
	store := NewHostStore(nil)
	store.hosts["remote"] = &OllamaHost{ID: "remote"}
	if err := store.SetTelemetryToken("remote", "too-short"); err == nil {
		t.Fatal("expected short token to fail")
	}
}

func TestHostStoreRefusesPlaintextTokenPersistence(t *testing.T) {
	store := NewHostStore(nil)
	store.ConfigurePersistence(filepath.Join(t.TempDir(), "hosts.json"), nil)
	store.hosts["remote"] = &OllamaHost{ID: "remote", TelemetryToken: "a-valid-token-that-must-not-be-plaintext", HasTelemetryToken: true}
	if err := store.Save(); err == nil {
		t.Fatal("expected persistence without a secrets store to fail")
	}
}
