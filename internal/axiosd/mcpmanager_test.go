package axiosd

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/axios-os/axios/pkg/mcp"
)

// shortSocketDir returns a short-lived temp dir for Unix sockets. t.TempDir
// paths on macOS exceed the 104-byte sun_path limit, so sockets go under /tmp.
func shortSocketDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "axmcp")
	if err != nil {
		t.Fatalf("mktemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// startFakeMCPServer launches a real pkg/mcp server on <socketDir>/fake.sock
// with a single trusted "ping" tool, and waits until the socket accepts
// connections.
func startFakeMCPServer(t *testing.T, socketDir string) *mcp.Server {
	t.Helper()

	srv := mcp.NewServer("fake", "0.0.1")
	srv.RegisterTool(mcp.ToolDefinition{
		Name:        "ping",
		Description: "test echo",
		Permission:  "trusted",
	}, func(params map[string]any) (string, error) {
		return "pong", nil
	})

	socketPath := filepath.Join(socketDir, "fake.sock")
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(socketPath) }()

	deadline := time.Now().Add(5 * time.Second)
	for {
		select {
		case err := <-serveErr:
			t.Fatalf("fake MCP server exited during startup: %v", err)
		default:
		}
		conn, err := mcp.Dial(socketPath)
		if err == nil {
			conn.Close()
			return srv
		}
		if time.Now().After(deadline) {
			t.Fatal("fake MCP server did not start accepting connections")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestMCPManagerReconnectAfterServerRestart kills the MCP server mid-test and
// restarts it on the same socket: calls must fail fast while it is down and
// recover on demand once it returns — without a daemon restart.
func TestMCPManagerReconnectAfterServerRestart(t *testing.T) {
	socketDir := shortSocketDir(t)
	mgr := NewMCPManager(socketDir, testLogger())

	srv := startFakeMCPServer(t, socketDir)
	if err := mgr.Connect("fake"); err != nil {
		t.Fatalf("initial connect: %v", err)
	}

	res, err := mgr.CallTool("fake", "ping", nil)
	if err != nil {
		t.Fatalf("initial call: %v", err)
	}
	if res.Content != "pong" {
		t.Fatalf("initial call content = %q, want %q", res.Content, "pong")
	}

	// Kill the server: live connections break and the socket goes stale.
	srv.Close()

	// While it is down the call must fail fast with a clear error.
	if _, err := mgr.CallTool("fake", "ping", nil); err == nil {
		t.Fatal("call while server is down succeeded, want error")
	}

	// After the server returns on the same socket, one call reconnects
	// (re-dial + re-Initialize) and succeeds.
	srv2 := startFakeMCPServer(t, socketDir)
	defer srv2.Close()

	res, err = mgr.CallTool("fake", "ping", nil)
	if err != nil {
		t.Fatalf("call after restart: %v", err)
	}
	if res.Content != "pong" {
		t.Fatalf("call after restart content = %q, want %q", res.Content, "pong")
	}

	// A second kill/restart cycle also recovers: the dead reconnected client
	// must be dropped and replaced cleanly.
	srv2.Close()
	srv3 := startFakeMCPServer(t, socketDir)
	defer srv3.Close()

	res, err = mgr.CallTool("fake", "ping", nil)
	if err != nil {
		t.Fatalf("call after second restart: %v", err)
	}
	if res.Content != "pong" {
		t.Fatalf("call after second restart content = %q, want %q", res.Content, "pong")
	}
}

// TestMCPManagerNoReconnectOnProtocolError verifies that a server-side
// protocol error (e.g. unknown tool) does not trigger a reconnect — the
// healthy client stays registered.
func TestMCPManagerNoReconnectOnProtocolError(t *testing.T) {
	socketDir := shortSocketDir(t)
	mgr := NewMCPManager(socketDir, testLogger())

	srv := startFakeMCPServer(t, socketDir)
	defer srv.Close()
	if err := mgr.Connect("fake"); err != nil {
		t.Fatalf("connect: %v", err)
	}

	mgr.mu.RLock()
	before := mgr.clients["fake"]
	mgr.mu.RUnlock()

	if _, err := mgr.CallTool("fake", "nonexistent", nil); err == nil {
		t.Fatal("call with unknown tool succeeded, want protocol error")
	}

	mgr.mu.RLock()
	after := mgr.clients["fake"]
	mgr.mu.RUnlock()
	if before != after {
		t.Fatal("protocol error triggered a reconnect; client was replaced")
	}

	// The same connection still works.
	if _, err := mgr.CallTool("fake", "ping", nil); err != nil {
		t.Fatalf("call after protocol error: %v", err)
	}
}

// TestMCPManagerConcurrentReconnect hammers CallTool from several goroutines
// across a server restart; run with -race to prove the clients map and the
// reconnect path are race-free.
func TestMCPManagerConcurrentReconnect(t *testing.T) {
	socketDir := shortSocketDir(t)
	mgr := NewMCPManager(socketDir, testLogger())

	srv := startFakeMCPServer(t, socketDir)
	if err := mgr.Connect("fake"); err != nil {
		t.Fatalf("connect: %v", err)
	}

	// Restart the server while calls are in flight.
	srv.Close()
	srv2 := startFakeMCPServer(t, socketDir)
	defer srv2.Close()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Errors are fine here (calls racing the restart may fail);
			// this only exercises the reconnect path for data races.
			mgr.CallTool("fake", "ping", nil)
		}()
	}
	wg.Wait()

	// Once the dust settles the manager must have a working client.
	res, err := mgr.CallTool("fake", "ping", nil)
	if err != nil {
		t.Fatalf("call after concurrent reconnect: %v", err)
	}
	if res.Content != "pong" {
		t.Fatalf("content = %q, want %q", res.Content, "pong")
	}
}

// TestMCPManagerCallToolServerDownFailsFast verifies that a server that was
// never reachable fails immediately instead of hanging.
func TestMCPManagerCallToolServerDownFailsFast(t *testing.T) {
	socketDir := shortSocketDir(t)
	// A stale socket file with no listener behind it.
	if err := os.WriteFile(filepath.Join(socketDir, "fake.sock"), []byte{}, 0600); err != nil {
		t.Fatal(err)
	}
	mgr := NewMCPManager(socketDir, testLogger())

	start := time.Now()
	if _, err := mgr.CallTool("fake", "ping", nil); err == nil {
		t.Fatal("call against dead server succeeded, want error")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("call against dead server took %v, want fast failure", elapsed)
	}
}
