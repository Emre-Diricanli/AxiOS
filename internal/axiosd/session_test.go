package axiosd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/axios-os/axios/pkg/providers"
)

func TestSessionStoreV2SaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")

	ss := newSessionStoreAt(path, testLogger())
	sess := ss.Get("s1")
	sess.AddMessage(providers.Message{Role: "user", Content: "check disk usage"})
	sess.AddMessage(providers.Message{
		Role:    "assistant",
		Content: "Checking.",
		ToolCalls: []providers.ToolCall{
			{ID: "call_1", Name: "axios-system__disk_usage", Arguments: `{}`},
		},
	})
	sess.AddMessage(providers.Message{Role: "tool", ToolCallID: "call_1", Name: "axios-system__disk_usage", Content: "42% used"})

	if err := ss.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// File must be 0600 and carry version 2.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat sessions file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("sessions file mode = %o, want 0600", perm)
	}
	raw, _ := os.ReadFile(path)
	var probe struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil || probe.Version != 2 {
		t.Fatalf("sessions file version = %d (err %v), want 2", probe.Version, err)
	}

	// Reload and compare.
	ss2 := newSessionStoreAt(path, testLogger())
	got := ss2.Get("s1").GetMessages()
	if len(got) != 3 {
		t.Fatalf("reloaded %d messages, want 3: %+v", len(got), got)
	}
	if got[1].Role != "assistant" || len(got[1].ToolCalls) != 1 || got[1].ToolCalls[0].ID != "call_1" {
		t.Errorf("assistant message lost tool calls: %+v", got[1])
	}
	if got[2].Role != "tool" || got[2].ToolCallID != "call_1" || got[2].Content != "42% used" {
		t.Errorf("tool message mangled: %+v", got[2])
	}
	if title := ss2.Get("s1").Title; title != "check disk usage" {
		t.Errorf("session title = %q", title)
	}
}

func TestSessionStoreLegacyConversion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")

	// Pre-version-2 format: top-level map, content either a string or an
	// array of Anthropic content blocks.
	legacy := `{
	  "old": {
	    "id": "old",
	    "title": "Legacy chat",
	    "created_at": "2025-01-01T00:00:00Z",
	    "updated_at": "2025-01-02T00:00:00Z",
	    "messages": [
	      {"role": "user", "content": "hello"},
	      {"role": "assistant", "content": [
	        {"type": "text", "text": "Hi! "},
	        {"type": "tool_use", "id": "t1", "name": "axios-system__system_info", "input": {}},
	        {"type": "text", "text": "Checking your system."}
	      ]},
	      {"role": "user", "content": [
	        {"type": "tool_result", "tool_use_id": "t1", "content": "macOS 15"}
	      ]},
	      {"role": "assistant", "content": "You are on macOS 15."}
	    ]
	  }
	}`
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}

	ss := newSessionStoreAt(path, testLogger())
	sess := ss.Get("old")
	if sess.Title != "Legacy chat" {
		t.Errorf("title = %q", sess.Title)
	}
	msgs := sess.GetMessages()

	// user "hello", assistant text-only merge, final assistant string.
	// The tool_use and tool_result blocks are dropped.
	if len(msgs) != 3 {
		t.Fatalf("converted %d messages, want 3: %+v", len(msgs), msgs)
	}
	if msgs[0].Role != "user" || msgs[0].Content != "hello" {
		t.Errorf("msg[0] = %+v", msgs[0])
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != "Hi! Checking your system." {
		t.Errorf("msg[1] = %+v", msgs[1])
	}
	if msgs[2].Role != "assistant" || msgs[2].Content != "You are on macOS 15." {
		t.Errorf("msg[2] = %+v", msgs[2])
	}

	// Saving upgrades the file to version 2.
	if err := ss.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	raw, _ := os.ReadFile(path)
	var probe struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil || probe.Version != 2 {
		t.Fatalf("upgraded file version = %d (err %v), want 2", probe.Version, err)
	}
}

func TestConvertLegacyMessagesSkipCounting(t *testing.T) {
	tests := []struct {
		name        string
		in          []legacyMessage
		wantMsgs    int
		wantSkipped int
	}{
		{
			name:        "plain strings",
			in:          []legacyMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}},
			wantMsgs:    1,
			wantSkipped: 0,
		},
		{
			name: "tool blocks only dropped entirely",
			in: []legacyMessage{
				{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"x","content":"y"}]`)},
			},
			wantMsgs:    0,
			wantSkipped: 1,
		},
		{
			name: "mixed text and tool blocks",
			in: []legacyMessage{
				{Role: "assistant", Content: json.RawMessage(`[{"type":"text","text":"a"},{"type":"tool_use","id":"t"}]`)},
			},
			wantMsgs:    1,
			wantSkipped: 1,
		},
		{
			name:        "unparseable content skipped",
			in:          []legacyMessage{{Role: "user", Content: json.RawMessage(`12345`)}},
			wantMsgs:    0,
			wantSkipped: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msgs, skipped := convertLegacyMessages(tt.in)
			if len(msgs) != tt.wantMsgs {
				t.Errorf("messages = %d, want %d (%+v)", len(msgs), tt.wantMsgs, msgs)
			}
			if skipped != tt.wantSkipped {
				t.Errorf("skipped = %d, want %d", skipped, tt.wantSkipped)
			}
		})
	}
}

// TestNewSessionStoreExplicitDataDir verifies that NewSessionStore honors an
// explicit data directory (the AXIOS_DATA_DIR-resolved path) instead of
// hardcoding ~/.axios.
func TestNewSessionStoreExplicitDataDir(t *testing.T) {
	dir := t.TempDir()

	ss := NewSessionStore(dir, testLogger())
	sess := ss.Create("s1")
	sess.AddMessage(providers.Message{Role: "user", Content: "hello"})
	if err := ss.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	path := filepath.Join(dir, "sessions.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("sessions file not in explicit data dir: %v", err)
	}

	// A new store over the same dir loads the saved session back.
	ss2 := NewSessionStore(dir, testLogger())
	got := ss2.Get("s1")
	if got.MessageCount() != 1 || got.GetMessages()[0].Content != "hello" {
		t.Errorf("reloaded session = %+v, want 1 message %q", got.GetMessages(), "hello")
	}
}
