package opencode_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/axios-os/axios/pkg/opencode"
)

// recvEvent reads the next event or fails the test after a timeout.
func recvEvent(t *testing.T, ch <-chan opencode.Event) opencode.Event {
	t.Helper()
	select {
	case ev, ok := <-ch:
		if !ok {
			t.Fatal("event channel closed unexpectedly")
		}
		return ev
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for event")
	}
	return opencode.Event{}
}

// waitClosed drains ch until it closes or the test times out.
func waitClosed(t *testing.T, ch <-chan opencode.Event) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for event channel to close")
		}
	}
}

func flushSSE(t *testing.T, w http.ResponseWriter, frame string) {
	t.Helper()
	if _, err := io.WriteString(w, frame); err != nil {
		return
	}
	w.(http.Flusher).Flush()
}

func TestEventsStreamParsing(t *testing.T) {
	const bigSize = 100 * 1024 // > bufio.Scanner's 64KB default
	big := strings.Repeat("x", bigSize)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/event" {
			t.Errorf("path = %q, want /event", r.URL.Path)
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != "opencode" || pass != testPassword {
			t.Errorf("basic auth = (%q, %q, ok=%v), want (opencode, %q, ok=true)", user, pass, ok, testPassword)
		}
		if accept := r.Header.Get("Accept"); accept != "text/event-stream" {
			t.Errorf("Accept = %q, want text/event-stream", accept)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		// First event after connect: server.connected.
		flushSSE(t, w, "data: {\"type\":\"server.connected\",\"properties\":{}}\n\n")
		// Comment / keep-alive lines must be ignored.
		flushSSE(t, w, ": keep-alive\n\n")
		// Multi-line data frame with an id field, split mid-JSON.
		flushSSE(t, w, "id: 42\r\n"+
			"data: {\"type\":\"permission.asked\",\n"+
			"data: \"properties\":{\"id\":\"per_01HXYZ\",\"sessionID\":\"ses_01ABC\","+
			"\"permission\":\"bash\",\"patterns\":[\"git push*\"],"+
			"\"metadata\":{\"command\":\"git push origin main\"},"+
			"\"title\":\"git push origin main\",\"messageID\":\"msg_09\",\"callID\":\"call_7\","+
			"\"time\":{\"created\":1752130000000}}}\n\n")
		// Malformed frame: must be skipped without killing the stream.
		flushSSE(t, w, "data: this is not json\n\n")
		// Oversized frame (>64KB).
		flushSSE(t, w, fmt.Sprintf("data: {\"type\":\"message.part.delta\",\"properties\":{\"sessionID\":\"ses_01ABC\",\"delta\":%q}}\n\n", big))
		// Hold the stream open until the client goes away.
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := opencode.NewClient(srv.URL, testPassword, srv.Client())
	events, err := c.Events(ctx)
	if err != nil {
		t.Fatalf("Events() error: %v", err)
	}

	ev := recvEvent(t, events)
	if ev.Type != opencode.EventServerConnected {
		t.Fatalf("first event type = %q, want %q", ev.Type, opencode.EventServerConnected)
	}

	ev = recvEvent(t, events)
	if ev.Type != opencode.EventPermissionAsked {
		t.Fatalf("second event type = %q, want %q", ev.Type, opencode.EventPermissionAsked)
	}
	if ev.ID != "42" {
		t.Errorf("event ID = %q, want %q", ev.ID, "42")
	}
	var perm opencode.PermissionAsked
	if err := json.Unmarshal(ev.Properties, &perm); err != nil {
		t.Fatalf("decode PermissionAsked: %v", err)
	}
	if perm.ID != "per_01HXYZ" || perm.SessionID != "ses_01ABC" || perm.Permission != "bash" {
		t.Errorf("PermissionAsked = %+v", perm)
	}
	if len(perm.Patterns) != 1 || perm.Patterns[0] != "git push*" {
		t.Errorf("Patterns = %v, want [git push*]", perm.Patterns)
	}
	var meta map[string]any
	if err := json.Unmarshal(perm.Metadata, &meta); err != nil {
		t.Fatalf("decode Metadata: %v", err)
	}
	if meta["command"] != "git push origin main" {
		t.Errorf("Metadata command = %v", meta["command"])
	}

	// The malformed frame is skipped; the next event is the oversized delta.
	ev = recvEvent(t, events)
	if ev.Type != opencode.EventMessagePartDelta {
		t.Fatalf("third event type = %q, want %q", ev.Type, opencode.EventMessagePartDelta)
	}
	var delta struct {
		Delta string `json:"delta"`
	}
	if err := json.Unmarshal(ev.Properties, &delta); err != nil {
		t.Fatalf("decode oversized delta: %v", err)
	}
	if len(delta.Delta) != bigSize {
		t.Errorf("delta length = %d, want %d", len(delta.Delta), bigSize)
	}

	cancel()
	waitClosed(t, events)
}

func TestEventsReconnectAfterServerClose(t *testing.T) {
	var mu sync.Mutex
	conns := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		conns++
		n := conns
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flushSSE(t, w, fmt.Sprintf("data: {\"type\":\"server.connected\",\"properties\":{\"connection\":%d}}\n\n", n))
		if n == 1 {
			return // drop the first stream: the client must reconnect
		}
		flushSSE(t, w, "data: {\"type\":\"session.idle\",\"properties\":{\"sessionID\":\"ses_01ABC\"}}\n\n")
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := opencode.NewClient(srv.URL, testPassword, srv.Client())
	events, err := c.Events(ctx)
	if err != nil {
		t.Fatalf("Events() error: %v", err)
	}

	connOf := func(ev opencode.Event) int {
		var p struct {
			Connection int `json:"connection"`
		}
		if err := json.Unmarshal(ev.Properties, &p); err != nil {
			t.Fatalf("decode connection: %v", err)
		}
		return p.Connection
	}

	ev := recvEvent(t, events)
	if ev.Type != opencode.EventServerConnected || connOf(ev) != 1 {
		t.Fatalf("first event = %+v, want server.connected from connection 1", ev)
	}
	// After the server drops the stream, the client reconnects and the first
	// event on the new stream is server.connected again.
	ev = recvEvent(t, events)
	if ev.Type != opencode.EventServerConnected || connOf(ev) != 2 {
		t.Fatalf("second event = %+v, want server.connected from connection 2", ev)
	}
	ev = recvEvent(t, events)
	if ev.Type != opencode.EventSessionIdle {
		t.Fatalf("third event type = %q, want %q", ev.Type, opencode.EventSessionIdle)
	}

	mu.Lock()
	got := conns
	mu.Unlock()
	if got != 2 {
		t.Errorf("connection count = %d, want 2", got)
	}

	cancel()
	waitClosed(t, events)
}

func TestEventsRetriesAfterErrorStatus(t *testing.T) {
	var mu sync.Mutex
	conns := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		conns++
		n := conns
		mu.Unlock()

		if n == 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flushSSE(t, w, "data: {\"type\":\"server.connected\",\"properties\":{}}\n\n")
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := opencode.NewClient(srv.URL, testPassword, srv.Client())
	events, err := c.Events(ctx)
	if err != nil {
		t.Fatalf("Events() error: %v", err)
	}

	ev := recvEvent(t, events)
	if ev.Type != opencode.EventServerConnected {
		t.Fatalf("event type = %q, want %q", ev.Type, opencode.EventServerConnected)
	}

	mu.Lock()
	got := conns
	mu.Unlock()
	if got < 2 {
		t.Errorf("connection count = %d, want at least 2 (retry after 401)", got)
	}

	cancel()
	waitClosed(t, events)
}

func TestEventsContextCancelClosesChannel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flushSSE(t, w, "data: {\"type\":\"server.connected\",\"properties\":{}}\n\n")
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := opencode.NewClient(srv.URL, testPassword, srv.Client())
	events, err := c.Events(ctx)
	if err != nil {
		t.Fatalf("Events() error: %v", err)
	}

	ev := recvEvent(t, events)
	if ev.Type != opencode.EventServerConnected {
		t.Fatalf("event type = %q, want %q", ev.Type, opencode.EventServerConnected)
	}

	cancel()
	waitClosed(t, events)
}

func TestEventsCancelDuringBackoff(t *testing.T) {
	// No server at all: the client keeps retrying with backoff until the
	// context is cancelled, then closes the channel.
	c := opencode.NewClient("http://127.0.0.1:1", testPassword, &http.Client{Timeout: time.Second})

	ctx, cancel := context.WithCancel(context.Background())
	events, err := c.Events(ctx)
	if err != nil {
		t.Fatalf("Events() error: %v", err)
	}

	time.Sleep(50 * time.Millisecond) // let it fail at least one connect
	cancel()
	waitClosed(t, events)
}

func TestEventsInvalidBaseURL(t *testing.T) {
	c := opencode.NewClient("://not-a-url", testPassword, nil)
	if _, err := c.Events(context.Background()); err == nil {
		t.Fatal("Events() error = nil, want request-build error")
	}
}

// TestPermissionAskedDecodeRealistic decodes a verbatim permission.asked
// payload as emitted by opencode v1.17.0, ensuring unknown fields are
// ignored and Metadata stays raw.
func TestPermissionAskedDecodeRealistic(t *testing.T) {
	payload := `{
		"id": "per_01J9ZC2V4X8YQW3E5R6T7Y8U9I",
		"sessionID": "ses_01J9ZC0A1B2C3D4E5F6G7H8J9K",
		"permission": "bash",
		"patterns": ["rm -rf node_modules"],
		"metadata": {"command": "rm -rf node_modules", "description": "Remove installed dependencies"},
		"messageID": "msg_01J9ZC1Q2W3E4R5T6Y7U8I9O0P",
		"callID": "toolu_01AbCdEfGhIjKlMnOpQrStUv",
		"title": "rm -rf node_modules",
		"type": "bash",
		"pattern": "rm -rf *",
		"time": {"created": 1752130000000}
	}`

	var perm opencode.PermissionAsked
	if err := json.Unmarshal([]byte(payload), &perm); err != nil {
		t.Fatalf("decode PermissionAsked: %v", err)
	}
	if perm.ID != "per_01J9ZC2V4X8YQW3E5R6T7Y8U9I" {
		t.Errorf("ID = %q", perm.ID)
	}
	if perm.SessionID != "ses_01J9ZC0A1B2C3D4E5F6G7H8J9K" {
		t.Errorf("SessionID = %q", perm.SessionID)
	}
	if perm.Permission != "bash" {
		t.Errorf("Permission = %q, want bash", perm.Permission)
	}
	if len(perm.Patterns) != 1 || perm.Patterns[0] != "rm -rf node_modules" {
		t.Errorf("Patterns = %v", perm.Patterns)
	}
	var meta struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(perm.Metadata, &meta); err != nil {
		t.Fatalf("decode Metadata: %v", err)
	}
	if meta.Command != "rm -rf node_modules" {
		t.Errorf("Metadata.Command = %q", meta.Command)
	}
}
