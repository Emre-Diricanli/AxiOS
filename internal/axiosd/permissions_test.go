package axiosd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/axios-os/axios/pkg/mcp"
	"github.com/axios-os/axios/pkg/permissions"
)

// fakeExecutor records dispatched tool calls and returns a scripted result.
type fakeExecutor struct {
	mu     sync.Mutex
	calls  []string // "server/tool"
	result *mcp.ToolResult
	err    error
}

func (f *fakeExecutor) CallTool(serverName, toolName string, params map[string]any) (*mcp.ToolResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, serverName+"/"+toolName)
	if f.err != nil {
		return nil, f.err
	}
	if f.result != nil {
		return f.result, nil
	}
	return &mcp.ToolResult{Content: "ok"}, nil
}

func (f *fakeExecutor) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// newPermTestServer builds a Server with the built-in policy, a fake tool
// executor, and a short approval timeout suitable for tests.
func newPermTestServer(t *testing.T, exec toolExecutor) *Server {
	t.Helper()
	return &Server{
		logger:          testLogger(),
		permissions:     permissions.Default(),
		executor:        exec,
		approvalTimeout: 500 * time.Millisecond,
	}
}

// waitForApprovalRequest polls the sink until an approval_request shows up.
func waitForApprovalRequest(t *testing.T, sink *fakeSink) ChatMessage {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if reqs := sink.byType("approval_request"); len(reqs) > 0 {
			return reqs[0]
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("no approval_request arrived on the sink")
	return ChatMessage{}
}

func TestExecuteToolSynchronousPaths(t *testing.T) {
	tests := []struct {
		name         string
		tool         string
		input        string
		wantResult   string // substring
		wantCalls    int
		wantApproval int
	}{
		{
			name:       "trusted passthrough",
			tool:       "axios-fs__read_file",
			input:      `{"path":"/tmp/x"}`,
			wantResult: "ok",
			wantCalls:  1,
		},
		{
			name:       "prohibited blocked without executing",
			tool:       "axios-fs__write_file",
			input:      `{"path":"/etc/axios/axiosd.yaml"}`,
			wantResult: "blocked by AxiOS permission policy",
			wantCalls:  0,
		},
		{
			name:       "invalid tool name",
			tool:       "no-separator",
			input:      `{}`,
			wantResult: "invalid tool name format",
			wantCalls:  0,
		},
		{
			name:       "invalid tool input",
			tool:       "axios-fs__read_file",
			input:      `{not json`,
			wantResult: "invalid tool input",
			wantCalls:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := &fakeExecutor{}
			s := newPermTestServer(t, exec)
			sink := &fakeSink{}

			got := s.executeTool(context.Background(), sink, tt.tool, "tc-1", json.RawMessage(tt.input))

			if !strings.Contains(got, tt.wantResult) {
				t.Errorf("executeTool result = %q, want it to contain %q", got, tt.wantResult)
			}
			if exec.callCount() != tt.wantCalls {
				t.Errorf("executor calls = %d, want %d", exec.callCount(), tt.wantCalls)
			}
			if got := len(sink.byType("approval_request")); got != tt.wantApproval {
				t.Errorf("approval_request messages = %d, want %d", got, tt.wantApproval)
			}
		})
	}
}

func TestExecuteToolApprovalFlow(t *testing.T) {
	tests := []struct {
		name       string
		approve    bool
		wantCalls  int
		wantResult string // substring
	}{
		{name: "approved executes", approve: true, wantCalls: 1, wantResult: "ok"},
		{name: "denied returns error result", approve: false, wantCalls: 0, wantResult: "denied by AxiOS permission policy"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := &fakeExecutor{}
			s := newPermTestServer(t, exec)
			sink := &fakeSink{}
			rawInput := json.RawMessage(`{"command":"ls -la"}`)

			resultCh := make(chan string, 1)
			go func() {
				resultCh <- s.executeTool(context.Background(), sink, "axios-system__run_command", "tc-1", rawInput)
			}()

			req := waitForApprovalRequest(t, sink)
			if req.ID == "" {
				t.Fatalf("approval_request carries no id: %+v", req)
			}
			if req.Tool != "axios-system__run_command" {
				t.Errorf("approval_request.Tool = %q", req.Tool)
			}
			if string(req.Params) != string(rawInput) {
				t.Errorf("approval_request.Params = %s, want %s", req.Params, rawInput)
			}

			if !s.approvals.resolve(req.ID, tt.approve) {
				t.Fatalf("resolve(%q) found no pending approval", req.ID)
			}

			var result string
			select {
			case result = <-resultCh:
			case <-time.After(2 * time.Second):
				t.Fatal("executeTool did not return after approval_response")
			}

			if !strings.Contains(result, tt.wantResult) {
				t.Errorf("executeTool result = %q, want it to contain %q", result, tt.wantResult)
			}
			if exec.callCount() != tt.wantCalls {
				t.Errorf("executor calls = %d, want %d", exec.callCount(), tt.wantCalls)
			}
		})
	}
}

func TestExecuteToolApprovalTimeoutDenies(t *testing.T) {
	exec := &fakeExecutor{}
	s := newPermTestServer(t, exec)
	s.approvalTimeout = 20 * time.Millisecond
	sink := &fakeSink{}

	result := s.executeTool(context.Background(), sink, "axios-system__run_command", "tc-1", json.RawMessage(`{"command":"rm -rf /"}`))

	if !strings.Contains(result, "denied by AxiOS permission policy") || !strings.Contains(result, "timed out") {
		t.Errorf("executeTool result = %q, want timeout denial", result)
	}
	if exec.callCount() != 0 {
		t.Errorf("executor calls = %d, want 0 after timeout", exec.callCount())
	}
	if got := len(sink.byType("approval_request")); got != 1 {
		t.Errorf("approval_request messages = %d, want 1", got)
	}

	// The pending entry must be cleaned up after the timeout.
	req := sink.byType("approval_request")[0]
	if s.approvals.resolve(req.ID, true) {
		t.Error("pending approval survived its timeout")
	}
}

func TestExecuteToolApprovalCanceledContextDenies(t *testing.T) {
	exec := &fakeExecutor{}
	s := newPermTestServer(t, exec)
	sink := &fakeSink{}

	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan string, 1)
	go func() {
		resultCh <- s.executeTool(ctx, sink, "axios-fs__write_file", "tc-1", json.RawMessage(`{"path":"/tmp/out.txt","content":"x"}`))
	}()

	waitForApprovalRequest(t, sink)
	cancel()

	var result string
	select {
	case result = <-resultCh:
	case <-time.After(2 * time.Second):
		t.Fatal("executeTool did not return after context cancellation")
	}

	if !strings.Contains(result, "denied by AxiOS permission policy") {
		t.Errorf("executeTool result = %q, want cancellation denial", result)
	}
	if exec.callCount() != 0 {
		t.Errorf("executor calls = %d, want 0 after cancellation", exec.callCount())
	}
}

func TestApprovalRegistryResolveUnknownID(t *testing.T) {
	var r approvalRegistry
	if r.resolve("nope", true) {
		t.Error("resolve of unknown id reported success")
	}
}

// TestWebSocketRoutesApprovalResponse verifies the websocket read loop routes
// approval_response frames to the pending-approvals map instead of treating
// them as chat input.
func TestWebSocketRoutesApprovalResponse(t *testing.T) {
	s := newPermTestServer(t, &fakeExecutor{})
	s.sessions = NewSessionStore(testLogger())

	srv := httptest.NewServer(http.HandlerFunc(s.handleWebSocket))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	approveCh := s.approvals.register("appr-approve")
	denyCh := s.approvals.register("appr-deny")

	// An unknown id must be tolerated (logged, not fatal, not chat input).
	frames := []ChatMessage{
		{Type: "approval_response", ID: "appr-unknown", Approve: true},
		{Type: "approval_response", ID: "appr-approve", Approve: true},
		{Type: "approval_response", ID: "appr-deny", Approve: false},
	}
	for _, f := range frames {
		if err := conn.WriteJSON(f); err != nil {
			t.Fatalf("write frame %+v: %v", f, err)
		}
	}

	select {
	case got := <-approveCh:
		if !got {
			t.Error("approve channel received false, want true")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("approval_response(approve=true) was not routed to the pending map")
	}

	select {
	case got := <-denyCh:
		if got {
			t.Error("deny channel received true, want false")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("approval_response(approve=false) was not routed to the pending map")
	}
}
